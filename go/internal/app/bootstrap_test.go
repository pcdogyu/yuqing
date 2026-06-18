package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pcdogyu/yuqing/go/internal/config"
)

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(tempFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if !fileExists(tempFile) {
		t.Fatal("expected fileExists to return true for existing file")
	}
	if fileExists(filepath.Join(tempDir, "missing.txt")) {
		t.Fatal("expected fileExists to return false for missing file")
	}
}

func TestPostgresDSNBuildsFromConfigParts(t *testing.T) {
	dsn := postgresDSN(config.Config{
		PostgresHost:     "10.15.0.19",
		PostgresPort:     "5432",
		PostgresDatabase: "yuqing",
		PostgresUser:     "admin",
		PostgresPassword: "secret",
		PostgresSSLMode:  "disable",
	})
	for _, want := range []string{"postgres://admin:secret@10.15.0.19:5432/yuqing", "sslmode=disable"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("expected dsn to contain %q, got %s", want, dsn)
		}
	}
}

func TestPostgresAdminDSNFromConfigParts(t *testing.T) {
	dsn, database, err := postgresAdminDSN(config.Config{
		PostgresHost:     "127.0.0.1",
		PostgresPort:     "5432",
		PostgresDatabase: "yuqing",
		PostgresUser:     "postgres",
		PostgresPassword: "secret",
		PostgresSSLMode:  "disable",
	})
	if err != nil {
		t.Fatalf("postgresAdminDSN() error = %v", err)
	}
	if database != "yuqing" {
		t.Fatalf("expected target database yuqing, got %q", database)
	}
	for _, want := range []string{"postgres://postgres:secret@127.0.0.1:5432/postgres", "sslmode=disable"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("expected admin dsn to contain %q, got %s", want, dsn)
		}
	}
}

func TestPostgresAdminDSNFromDatabaseURL(t *testing.T) {
	dsn, database, err := postgresAdminDSN(config.Config{
		DatabaseURL: "postgres://app:secret@db.example.com:6543/yuqing_prod?sslmode=require",
	})
	if err != nil {
		t.Fatalf("postgresAdminDSN() error = %v", err)
	}
	if database != "yuqing_prod" {
		t.Fatalf("expected target database yuqing_prod, got %q", database)
	}
	if dsn != "postgres://app:secret@db.example.com:6543/postgres?sslmode=require" {
		t.Fatalf("unexpected admin dsn: %s", dsn)
	}
}

func TestPostgresDatabaseNameValidation(t *testing.T) {
	for _, value := range []string{"yuqing", "yuqing_prod", "db2026"} {
		if !validPostgresDatabaseName(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "yuqing-prod", "yuqing.prod", `yuqing"prod`} {
		if validPostgresDatabaseName(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestPostgresErrorCodeDetection(t *testing.T) {
	missing := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "3D000"})
	if !isPostgresDatabaseMissing(missing) {
		t.Fatal("expected wrapped pg error to be detected as missing database")
	}
	if !isPostgresDatabaseMissing(fmt.Errorf("server error (SQLSTATE 3D000)")) {
		t.Fatal("expected SQLSTATE text to be detected as missing database")
	}
	duplicate := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "42P04"})
	if !isPostgresDuplicateDatabase(duplicate) {
		t.Fatal("expected wrapped pg error to be detected as duplicate database")
	}
}
