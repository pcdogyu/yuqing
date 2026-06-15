package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	host := flag.String("host", "10.15.0.19", "PostgreSQL host")
	port := flag.String("port", "5432", "PostgreSQL port")
	database := flag.String("database", "yuqing", "database name")
	user := flag.String("user", "postgres", "PostgreSQL user")
	password := flag.String("password", "", "PostgreSQL password")
	sslMode := flag.String("sslmode", "disable", "PostgreSQL sslmode")
	schemaPath := flag.String("schema", "", "schema SQL path")
	flag.Parse()

	if !validDatabaseName(*database) {
		fatalf("database name must contain only letters, digits, and underscore")
	}
	if strings.TrimSpace(*schemaPath) == "" {
		*schemaPath = filepath.Join("db", "postgres_schema.sql")
	}

	schema, err := os.ReadFile(*schemaPath)
	if err != nil {
		fatalf("read schema: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetDSN := postgresDSN(*host, *port, *database, *user, *password, *sslMode)
	if err := ping(ctx, targetDSN); err != nil {
		fmt.Printf("database %q is not reachable, trying to create it from postgres database...\n", *database)
		if createErr := createDatabase(ctx, postgresDSN(*host, *port, "postgres", *user, *password, *sslMode), *database); createErr != nil {
			fatalf("create database %q: %v (initial connection error: %v)", *database, createErr, err)
		}
	}

	if err := execSQL(ctx, targetDSN, string(schema)); err != nil {
		fatalf("apply schema: %v", err)
	}
	fmt.Printf("postgres schema applied: host=%s port=%s database=%s user=%s schema=%s\n", *host, *port, *database, *user, *schemaPath)
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

func ping(ctx context.Context, dsn string) error {
	conn, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return conn.Ping(ctx)
}

func createDatabase(ctx context.Context, dsn, database string) error {
	conn, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `CREATE DATABASE "`+database+`"`)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P04" {
		return nil
	}
	return err
}

func execSQL(ctx context.Context, dsn, sqlText string) error {
	conn, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, sqlText)
	return err
}

func connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	return pgx.ConnectConfig(ctx, cfg)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
