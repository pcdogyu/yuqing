package jin10xnews

import (
	"os"
	"path/filepath"
	"strings"
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
	if items[0].PublishTime != "2026-05-29 10:55:00" {
		t.Fatalf("unexpected publish time: %s", items[0].PublishTime)
	}
	if items[0].TagFlags != "NEW/原创" {
		t.Fatalf("unexpected tag flags: %s", items[0].TagFlags)
	}
	if items[1].ExternalSourceHost != "mp.weixin.qq.com" {
		t.Fatalf("unexpected external host: %s", items[1].ExternalSourceHost)
	}
}

func TestResolvePublishTimeFromText(t *testing.T) {
	capturedAt := time.Date(2026, 6, 26, 4, 27, 0, 0, time.UTC)
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "just now", text: "刚刚", want: "2026-06-26 12:27:00"},
		{name: "minutes", text: "19分钟前", want: "2026-06-26 12:08:00"},
		{name: "hours", text: "2小时前", want: "2026-06-26 10:27:00"},
		{name: "same day clock", text: "09:16", want: "2026-06-26 09:16:00"},
		{name: "full time", text: "2026-06-26 09:16", want: "2026-06-26 09:16:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePublishTimeFromText(tt.text, capturedAt); got != tt.want {
				t.Fatalf("resolvePublishTimeFromText(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseDetailHTML(t *testing.T) {
	html := mustReadFixture(t, "headline_detail.html")

	content, summary, shareURL := parseDetailHTML(html)

	if summary == "" {
		t.Fatal("expected summary from detail page")
	}
	if content == "" {
		t.Fatal("expected content from detail page")
	}
	if shareURL == "" || !strings.Contains(shareURL, "webapp/details.html") {
		t.Fatalf("expected share url, got %q", shareURL)
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
