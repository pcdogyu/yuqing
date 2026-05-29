package jin10flash

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseHTMLUsesDataList(t *testing.T) {
	html := mustReadFixture(t, "flash_datalist.html")
	items, err := ParseHTML(html, "https://www.jin10.com/", time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseHTML error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "美国总统称将推动税改法案" {
		t.Fatalf("unexpected title: %s", items[0].Title)
	}
	if items[0].DetailURL != "https://flash.jin10.com/detail/20260529101000100" {
		t.Fatalf("unexpected detail url: %s", items[0].DetailURL)
	}
	if !items[1].IsVIP {
		t.Fatalf("expected vip item")
	}
}

func TestParseHTMLFallsBackToLinks(t *testing.T) {
	html := mustReadFixture(t, "flash_fallback.html")
	items, err := ParseHTML(html, "https://www.jin10.com/", time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseHTML error: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected fallback items")
	}
	if items[0].DetailURL == "" {
		t.Fatal("expected detail url")
	}
}

func mustReadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}
