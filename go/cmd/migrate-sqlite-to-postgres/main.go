package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var tableLinePattern = regexp.MustCompile(`(?m)^CREATE TABLE IF NOT EXISTS ([A-Za-z0-9_]+) \(`)

type columnInfo struct {
	Name     string
	DataType string
	UDTName  string
}

func main() {
	sqlitePath := flag.String("sqlite", filepath.Join("data", "yuqing.db"), "source SQLite database path")
	host := flag.String("host", "10.15.0.19", "PostgreSQL host")
	port := flag.String("port", "5432", "PostgreSQL port")
	database := flag.String("database", "yuqing", "PostgreSQL database")
	user := flag.String("user", "postgres", "PostgreSQL user")
	password := flag.String("password", "", "PostgreSQL password")
	sslMode := flag.String("sslmode", "disable", "PostgreSQL sslmode")
	schemaPath := flag.String("schema", filepath.Join("db", "postgres_schema.sql"), "PostgreSQL schema SQL path")
	truncate := flag.Bool("truncate", false, "delete target table rows before copying")
	flag.Parse()

	if !validDatabaseName(*database) {
		fatalf("database name must contain only letters, digits, and underscore")
	}

	tables, err := schemaTables(*schemaPath)
	if err != nil {
		fatalf("read schema tables: %v", err)
	}
	if len(tables) == 0 {
		fatalf("no tables found in schema %s", *schemaPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pgDSN := postgresDSN(*host, *port, *database, *user, *password, *sslMode)
	if err := ensurePostgresDatabase(ctx, postgresDSN(*host, *port, "postgres", *user, *password, *sslMode), pgDSN, *database); err != nil {
		fatalf("ensure postgres database: %v", err)
	}
	if err := applySchema(ctx, pgDSN, *schemaPath); err != nil {
		fatalf("apply postgres schema: %v", err)
	}

	source, err := sql.Open("sqlite", *sqlitePath)
	if err != nil {
		fatalf("open sqlite: %v", err)
	}
	defer source.Close()
	if err := source.PingContext(ctx); err != nil {
		fatalf("ping sqlite: %v", err)
	}

	target, err := sql.Open("pgx", pgDSN)
	if err != nil {
		fatalf("open postgres: %v", err)
	}
	defer target.Close()
	if err := target.PingContext(ctx); err != nil {
		fatalf("ping postgres: %v", err)
	}

	total := 0
	for _, table := range tables {
		copied, err := copyTable(ctx, source, target, table, *truncate)
		if err != nil {
			fatalf("copy table %s: %v", table, err)
		}
		total += copied
		fmt.Printf("copied table=%s rows=%d\n", table, copied)
	}
	if err := refreshSequences(ctx, target, tables); err != nil {
		fatalf("refresh sequences: %v", err)
	}
	fmt.Printf("sqlite to postgres migration complete: sqlite=%s postgres=%s:%s/%s tables=%d inserted=%d truncate=%v\n", *sqlitePath, *host, *port, *database, len(tables), total, *truncate)
}

func schemaTables(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := tableLinePattern.FindAllStringSubmatch(string(content), -1)
	tables := make([]string, 0, len(matches))
	for _, match := range matches {
		tables = append(tables, match[1])
	}
	return tables, nil
}

func copyTable(ctx context.Context, source, target *sql.DB, table string, truncate bool) (int, error) {
	columns, err := postgresColumns(ctx, target, table)
	if err != nil {
		return 0, err
	}
	if len(columns) == 0 {
		return 0, nil
	}
	columnNames := make([]string, len(columns))
	for i, column := range columns {
		columnNames[i] = column.Name
	}
	if truncate {
		if _, err := target.ExecContext(ctx, `DELETE FROM `+quoteIdent(table)); err != nil {
			return 0, err
		}
	}

	selectSQL := `SELECT ` + quoteIdentList(columnNames) + ` FROM ` + quoteIdent(table)
	rows, err := source.QueryContext(ctx, selectSQL)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	insertSQL := postgresInsertSQL(table, columnNames)
	count := 0
	cleaned := 0
	for rows.Next() {
		values := make([]any, len(columns))
		scans := make([]any, len(columns))
		for i := range values {
			scans[i] = &values[i]
		}
		if err := rows.Scan(scans...); err != nil {
			return count, err
		}
		for i, value := range values {
			cleanValue, wasCleaned := normalizeSQLiteValue(value, columns[i])
			values[i] = cleanValue
			if wasCleaned {
				cleaned++
			}
		}
		tag, err := target.ExecContext(ctx, insertSQL, values...)
		if err != nil {
			return count, err
		}
		affected, _ := tag.RowsAffected()
		count += int(affected)
	}
	if cleaned > 0 {
		fmt.Printf("cleaned invalid utf8 table=%s values=%d\n", table, cleaned)
	}
	return count, rows.Err()
}

func postgresColumns(ctx context.Context, db *sql.DB, table string) ([]columnInfo, error) {
	rows, err := db.QueryContext(ctx, `
SELECT column_name, data_type, udt_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []columnInfo
	for rows.Next() {
		var column columnInfo
		if err := rows.Scan(&column.Name, &column.DataType, &column.UDTName); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}

func normalizeSQLiteValue(value any, column columnInfo) (any, bool) {
	switch v := value.(type) {
	case string:
		if utf8.ValidString(v) {
			return v, false
		}
		return strings.ToValidUTF8(v, "\uFFFD"), true
	case []byte:
		if strings.EqualFold(column.DataType, "bytea") || strings.EqualFold(column.UDTName, "bytea") {
			return v, false
		}
		text := string(v)
		if utf8.ValidString(text) {
			return text, false
		}
		return strings.ToValidUTF8(text, "\uFFFD"), true
	default:
		return value, false
	}
}

func postgresInsertSQL(table string, columns []string) string {
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return `INSERT INTO ` + quoteIdent(table) + ` (` + quoteIdentList(columns) + `) VALUES (` + strings.Join(placeholders, ", ") + `) ON CONFLICT DO NOTHING`
}

func quoteIdentList(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = quoteIdent(value)
	}
	return strings.Join(quoted, ", ")
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func refreshSequences(ctx context.Context, db *sql.DB, tables []string) error {
	for _, table := range tables {
		hasID, err := tableHasColumn(ctx, db, table, "id")
		if err != nil {
			return err
		}
		if !hasID {
			continue
		}
		var sequence sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT pg_get_serial_sequence($1, 'id')`, "public."+table).Scan(&sequence); err != nil {
			return err
		}
		if !sequence.Valid || strings.TrimSpace(sequence.String) == "" {
			continue
		}
		query := fmt.Sprintf(`SELECT setval($1, COALESCE((SELECT MAX(id) FROM %s), 1), COALESCE((SELECT MAX(id) FROM %s), 0) > 0)`, quoteIdent(table), quoteIdent(table))
		if _, err := db.ExecContext(ctx, query, sequence.String); err != nil {
			return err
		}
	}
	return nil
}

func tableHasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
)`, table, column).Scan(&exists)
	return exists, err
}

func ensurePostgresDatabase(ctx context.Context, adminDSN, targetDSN, database string) error {
	target, err := sql.Open("pgx", targetDSN)
	if err != nil {
		return err
	}
	if pingErr := target.PingContext(ctx); pingErr == nil {
		return target.Close()
	}
	_ = target.Close()

	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		return err
	}
	_, err = admin.ExecContext(ctx, `CREATE DATABASE `+quoteIdent(database))
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "SQLSTATE 42P04") {
		return nil
	}
	return err
}

func applySchema(ctx context.Context, dsn, schemaPath string) error {
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, string(schema))
	return err
}

func postgresDSN(host, port, database, user, password, sslMode string) string {
	if strings.TrimSpace(port) == "" {
		port = "5432"
	}
	if strings.TrimSpace(sslMode) == "" {
		sslMode = "disable"
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: "/" + database}
	if password != "" {
		u.User = url.UserPassword(user, password)
	} else if user != "" {
		u.User = url.User(user)
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func validDatabaseName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func fatalf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	if !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
