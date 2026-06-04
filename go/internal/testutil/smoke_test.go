package testutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssertFileContains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	AssertFileContains(t, path, "alpha", "gamma")
}

func TestAssertFileContainsFailsWhenSnippetMissing(t *testing.T) {
	if os.Getenv("TESTUTIL_HELPER") == "1" {
		AssertFileContains(t, os.Getenv("TESTUTIL_PATH"), "delta")
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestAssertFileContainsFailsWhenSnippetMissing$")
	cmd.Env = append(os.Environ(),
		"TESTUTIL_HELPER=1",
		"TESTUTIL_PATH="+path,
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("AssertFileContains should fail when the snippet is missing")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
}
