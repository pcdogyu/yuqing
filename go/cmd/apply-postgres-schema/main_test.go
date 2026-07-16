package main

import (
	"net/url"
	"testing"
)

func TestValidDatabaseName(t *testing.T) {
	tests := map[string]bool{
		"":            false,
		"yuqing":      true,
		"yuqing_2026": true,
		"yuqing-prod": false,
		"yuqing db":   false,
		"舆情":          false,
	}
	for name, want := range tests {
		if got := validDatabaseName(name); got != want {
			t.Fatalf("validDatabaseName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestPostgresDSNBuildsEscapedURL(t *testing.T) {
	dsn := postgresDSN("db.example.com", "", "yuqing", "postgres", "pa ss", "")
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn %q: %v", dsn, err)
	}
	if parsed.Scheme != "postgres" {
		t.Fatalf("scheme = %q", parsed.Scheme)
	}
	if parsed.Host != "db.example.com:5432" {
		t.Fatalf("host = %q", parsed.Host)
	}
	if parsed.Path != "/yuqing" {
		t.Fatalf("path = %q", parsed.Path)
	}
	if user := parsed.User.Username(); user != "postgres" {
		t.Fatalf("user = %q", user)
	}
	if password, ok := parsed.User.Password(); !ok || password != "pa ss" {
		t.Fatalf("password = %q ok=%v", password, ok)
	}
	if sslMode := parsed.Query().Get("sslmode"); sslMode != "disable" {
		t.Fatalf("sslmode = %q", sslMode)
	}
}
