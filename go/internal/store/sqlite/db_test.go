package sqlite

import (
	"strings"
	"testing"
)

func TestPostgresQueryRewritesPlaceholdersAndInsertIgnore(t *testing.T) {
	query := postgresQuery(`INSERT OR IGNORE INTO item_tags (item_id, tag, created_at) VALUES (?, 'deleted', ?)`)
	if !strings.Contains(query, "ON CONFLICT DO NOTHING") {
		t.Fatalf("expected insert ignore rewrite, got %s", query)
	}
	if !strings.Contains(query, "$1") || !strings.Contains(query, "$2") {
		t.Fatalf("expected postgres placeholders, got %s", query)
	}
	if strings.Contains(query, "?") {
		t.Fatalf("expected all bind placeholders rewritten, got %s", query)
	}
}

func TestPostgresInsertNeedsID(t *testing.T) {
	if !postgresInsertNeedsID(`INSERT INTO users (username) VALUES ($1)`) {
		t.Fatal("expected users insert to request returning id")
	}
	if postgresInsertNeedsID(`INSERT INTO sessions (token) VALUES ($1)`) {
		t.Fatal("expected token-table insert to skip returning id")
	}
	if postgresInsertNeedsID(`INSERT INTO users (username) VALUES ($1) ON CONFLICT(username) DO UPDATE SET username = excluded.username`) {
		t.Fatal("expected upsert to skip returning id")
	}
}
