package app

import (
	"os"
	"path/filepath"
	"testing"
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
