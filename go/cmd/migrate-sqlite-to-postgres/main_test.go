package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaTablesExtractsCreateTableNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.sql")
	schema := strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS articles (`,
		`    id BIGSERIAL PRIMARY KEY`,
		`);`,
		`CREATE INDEX idx_articles_title ON articles(title);`,
		`CREATE TABLE IF NOT EXISTS a_stock_recommendations (`,
		`    id BIGSERIAL PRIMARY KEY`,
		`);`,
	}, "\n")
	if err := os.WriteFile(path, []byte(schema), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	got, err := schemaTables(path)
	if err != nil {
		t.Fatalf("schema tables: %v", err)
	}
	want := []string{"articles", "a_stock_recommendations"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tables = %#v, want %#v", got, want)
	}
}

func TestNormalizeSQLiteValuePreservesByteaAndCleansText(t *testing.T) {
	raw := []byte{0xff, 'a'}
	if got, cleaned := normalizeSQLiteValue(raw, columnInfo{DataType: "bytea"}); cleaned || !reflect.DeepEqual(got, raw) {
		t.Fatalf("bytea normalize = %#v cleaned=%v", got, cleaned)
	}
	got, cleaned := normalizeSQLiteValue(raw, columnInfo{DataType: "text"})
	if !cleaned {
		t.Fatalf("text normalize cleaned=false, want true")
	}
	if text, ok := got.(string); !ok || !strings.Contains(text, "\uFFFD") {
		t.Fatalf("text normalize = %#v", got)
	}
	if got, cleaned := normalizeSQLiteValue("valid", columnInfo{DataType: "text"}); cleaned || got != "valid" {
		t.Fatalf("valid text normalize = %#v cleaned=%v", got, cleaned)
	}
}

func TestPostgresInsertSQLQuotesIdentifiers(t *testing.T) {
	got := postgresInsertSQL(`weird"table`, []string{"id", `bad"name`})
	want := `INSERT INTO "weird""table" ("id", "bad""name") VALUES ($1, $2) ON CONFLICT DO NOTHING`
	if got != want {
		t.Fatalf("insert sql = %q, want %q", got, want)
	}
}

func TestMigrationValidDatabaseName(t *testing.T) {
	if !validDatabaseName("yuqing_2026") {
		t.Fatalf("expected database name to be valid")
	}
	if validDatabaseName("yuqing-prod") {
		t.Fatalf("expected database name with dash to be invalid")
	}
}
