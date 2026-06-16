package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
