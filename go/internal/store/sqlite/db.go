package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

type DB struct {
	raw    *sql.DB
	driver string
}

type Tx struct {
	raw    *sql.Tx
	driver string
}

type Stmt struct {
	raw *sql.Stmt
}

type insertResult struct {
	id           int64
	rowsAffected int64
}

func newDB(raw *sql.DB, driver string) *DB {
	return &DB{raw: raw, driver: driver}
}

func (db *DB) Close() error {
	return db.raw.Close()
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if db.driver != "postgres" {
		return db.raw.ExecContext(ctx, query, args...)
	}
	return execPostgres(ctx, db.raw, query, args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if db.driver != "postgres" {
		return db.raw.QueryContext(ctx, query, args...)
	}
	return db.raw.QueryContext(ctx, postgresQuery(query), args...)
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if db.driver != "postgres" {
		return db.raw.QueryRowContext(ctx, query, args...)
	}
	return db.raw.QueryRowContext(ctx, postgresQuery(query), args...)
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := db.raw.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{raw: tx, driver: db.driver}, nil
}

func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if tx.driver != "postgres" {
		return tx.raw.ExecContext(ctx, query, args...)
	}
	return execPostgres(ctx, tx.raw, query, args...)
}

func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if tx.driver != "postgres" {
		return tx.raw.QueryRowContext(ctx, query, args...)
	}
	return tx.raw.QueryRowContext(ctx, postgresQuery(query), args...)
}

func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if tx.driver != "postgres" {
		return tx.raw.QueryContext(ctx, query, args...)
	}
	return tx.raw.QueryContext(ctx, postgresQuery(query), args...)
}

func (tx *Tx) PrepareContext(ctx context.Context, query string) (*Stmt, error) {
	if tx.driver == "postgres" {
		query = postgresQuery(query)
	}
	stmt, err := tx.raw.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &Stmt{raw: stmt}, nil
}

func (tx *Tx) Commit() error {
	return tx.raw.Commit()
}

func (tx *Tx) Rollback() error {
	return tx.raw.Rollback()
}

func (stmt *Stmt) ExecContext(ctx context.Context, args ...any) (sql.Result, error) {
	return stmt.raw.ExecContext(ctx, args...)
}

func (stmt *Stmt) Close() error {
	return stmt.raw.Close()
}

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func execPostgres(ctx context.Context, q queryer, query string, args ...any) (sql.Result, error) {
	rewritten := postgresQuery(query)
	if postgresInsertNeedsID(rewritten) {
		var id int64
		if err := q.QueryRowContext(ctx, rewritten+" RETURNING id", args...).Scan(&id); err != nil {
			return nil, err
		}
		return insertResult{id: id, rowsAffected: 1}, nil
	}
	return q.ExecContext(ctx, rewritten, args...)
}

func (r insertResult) LastInsertId() (int64, error) {
	return r.id, nil
}

func (r insertResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

func postgresQuery(query string) string {
	query = strings.Replace(query, "INSERT OR IGNORE INTO", "INSERT INTO", 1)
	if strings.Contains(query, "INSERT INTO") && strings.Contains(query, " OR IGNORE ") {
		query = strings.Replace(query, " OR IGNORE ", " ", 1)
	}
	if strings.Contains(query, "INSERT INTO") && strings.Contains(query, "DO NOTHING") == false && strings.Contains(query, "OR IGNORE") {
		query += " ON CONFLICT DO NOTHING"
	}
	if strings.Contains(query, "INSERT INTO") && strings.Contains(query, "ON CONFLICT") == false && strings.Contains(query, "INSERT OR IGNORE") {
		query += " ON CONFLICT DO NOTHING"
	}
	query = strings.Replace(query, "INSERT INTO item_reads (user_id, item_id, created_at) VALUES (?, ?, ?)", "INSERT INTO item_reads (user_id, item_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", 1)
	query = strings.Replace(query, "INSERT INTO item_shares (user_id, item_id, channel, created_at) VALUES (?, ?, ?, ?)", "INSERT INTO item_shares (user_id, item_id, channel, created_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING", 1)
	query = strings.Replace(query, "INSERT INTO item_tags (item_id, tag, created_at) VALUES (?, ?, ?)", "INSERT INTO item_tags (item_id, tag, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", 1)
	query = strings.Replace(query, "INSERT INTO item_tags (item_id, tag, created_at) VALUES (?, 'deleted', ?)", "INSERT INTO item_tags (item_id, tag, created_at) VALUES (?, 'deleted', ?) ON CONFLICT DO NOTHING", 1)
	query = strings.Replace(query, "INSERT INTO item_relations (item_id, project_id, rule_id, created_at) VALUES (?, ?, ?, ?)", "INSERT INTO item_relations (item_id, project_id, rule_id, created_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING", 1)
	return rebindPostgresPlaceholders(query)
}

func postgresInsertNeedsID(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(upper, "INSERT INTO ") || strings.Contains(upper, " ON CONFLICT") || strings.Contains(upper, " RETURNING ") {
		return false
	}
	for _, table := range []string{
		"CRAWL_RUNS",
		"CRAWL_TEMPLATES",
		"USERS",
		"PROJECT_GROUPS",
		"PROJECTS",
		"MONITOR_RULES",
		"REPORTS",
		"REPORT_SECTIONS",
		"FEEDBACK",
		"AUDIT_LOGS",
		"SEARCH_WORDS",
		"PUBLIC_OPTIONS",
		"SYSTEM_NOTICES",
	} {
		if strings.HasPrefix(upper, "INSERT INTO "+table+" ") || strings.HasPrefix(upper, "INSERT INTO "+table+"(") {
			return true
		}
	}
	return false
}

func rebindPostgresPlaceholders(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 8)
	arg := 1
	inSingle := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			b.WriteByte(ch)
			if inSingle && i+1 < len(query) && query[i+1] == '\'' {
				i++
				b.WriteByte(query[i])
				continue
			}
			inSingle = !inSingle
			continue
		}
		if ch == '?' && !inSingle {
			b.WriteByte('$')
			b.WriteString(intString(arg))
			arg++
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func intString(value int) string {
	switch value {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	case 4:
		return "4"
	case 5:
		return "5"
	case 6:
		return "6"
	case 7:
		return "7"
	case 8:
		return "8"
	case 9:
		return "9"
	default:
		return strconv.Itoa(value)
	}
}
