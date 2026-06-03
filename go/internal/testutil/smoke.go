package testutil

import (
	"os"
	"strings"
	"testing"
)

func AssertFileContains(t *testing.T, path string, snippets ...string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	content := string(data)
	for _, snippet := range snippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to contain %q", path, snippet)
		}
	}
}
