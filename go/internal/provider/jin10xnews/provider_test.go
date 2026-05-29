package jin10xnews

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseHTML(t *testing.T) {
	html := mustReadFixture(t, "headline_list.html")
	items, err := ParseHTML(html, "https://xnews.jin10.com/", time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseHTML error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "订阅存储市场动态" {
		t.Fatalf("unexpected title: %s", items[0].Title)
	}
	if items[0].PublishTimeText != "5分钟前" {
		t.Fatalf("unexpected publish time text: %s", items[0].PublishTimeText)
	}
	if items[0].TagFlags != "NEW/原创" {
		t.Fatalf("unexpected tag flags: %s", items[0].TagFlags)
	}
	if items[1].ExternalSourceHost != "mp.weixin.qq.com" {
		t.Fatalf("unexpected external host: %s", items[1].ExternalSourceHost)
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
