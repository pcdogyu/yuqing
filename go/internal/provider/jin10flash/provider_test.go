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

func TestParseDetailHTML(t *testing.T) {
	html := mustReadFixture(t, "flash_detail.html")

	title, content, publishTime, sourceURL, fromText := parseDetailHTML(html)

	if title == "" {
		t.Fatal("expected title from detail page")
	}
	if content == "" {
		t.Fatal("expected content from detail page")
	}
	if publishTime != "2026-06-01 14:21:41" {
		t.Fatalf("unexpected publish time: %q", publishTime)
	}
	if sourceURL != "https://example.com/source" {
		t.Fatalf("unexpected source url: %q", sourceURL)
	}
	if fromText != "来自：新华社" {
		t.Fatalf("unexpected from text: %q", fromText)
	}
	if content != "金十数据6月1日讯，正文内容在这里。（新华社）" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestParseDetailHTMLFallsBackToNuxtContent(t *testing.T) {
	html := `<html><body><div class="content-title"><div class="flash-title">中科创达：创通联达首发TurboX C7790开发套件 填补高通平台20+TOPS算力模组空白</div><div></div></div><script>window.__NUXT__=(function(){return {data:[{flash:{data:{title:"中科创达：创通联达首发TurboX C7790开发套件 填补高通平台20+TOPS算力模组空白",content:"\u003Cp\u003E中科创达表示，该套件面向边缘AI部署场景。\u003C\/p\u003E\u003Cp\u003E产品填补高通平台20+TOPS算力模组空白。\u003C\/p\u003E"}}}]}})();</script></body></html>`

	title, content, publishTime, sourceURL, fromText := parseDetailHTML(html)

	if title != "中科创达：创通联达首发TurboX C7790开发套件 填补高通平台20+TOPS算力模组空白" {
		t.Fatalf("unexpected title: %q", title)
	}
	if content != "中科创达表示，该套件面向边缘AI部署场景。 产品填补高通平台20+TOPS算力模组空白。" {
		t.Fatalf("unexpected content: %q", content)
	}
	if publishTime != "" || sourceURL != "" || fromText != "" {
		t.Fatalf("expected empty metadata, got publishTime=%q sourceURL=%q fromText=%q", publishTime, sourceURL, fromText)
	}
}

func TestParseDetailHTMLDoesNotUseTitleAsContent(t *testing.T) {
	html := `<html><head><title>金十图示：2026年06月16日（周二）亚盘市场行情 - 金十数据</title></head><body><div class="content-title"><div class="flash-title">金十图示：2026年06月16日（周二）亚盘市场行情</div><div></div></div></body></html>`

	title, content, publishTime, sourceURL, fromText := parseDetailHTML(html)

	if title != "金十图示：2026年06月16日（周二）亚盘市场行情" {
		t.Fatalf("unexpected title: %q", title)
	}
	if content != "" {
		t.Fatalf("expected empty content when detail body is missing, got %q", content)
	}
	if publishTime != "" || sourceURL != "" || fromText != "" {
		t.Fatalf("expected empty metadata, got publishTime=%q sourceURL=%q fromText=%q", publishTime, sourceURL, fromText)
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
