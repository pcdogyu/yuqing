package jin10flash

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
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

func TestParseAPIMapsFlashRows(t *testing.T) {
	body := []byte(`{
		"status": 200,
		"data": [{
			"id": "20260626130000123",
			"time": "2026-06-26 13:00:00",
			"data": {
				"title": "金十期货6月26日讯",
				"content": "<p>上海出口集装箱结算运价指数上涨。</p>",
				"source": "金十期货",
				"source_link": "https://example.com/news/1",
				"pic": "https://example.com/a.png"
			},
			"tags": [{"name":"A股"}],
			"remark": [{"title":"沪深300","symbol":"IF"}],
			"extras": {"vip": true}
		}]
	}`)

	items, rows, err := ParseAPI(body, "https://www.jin10.com/", time.Date(2026, 6, 26, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseAPI error: %v", err)
	}
	if len(rows) != 1 || len(items) != 1 {
		t.Fatalf("expected one API row and item, got rows=%d items=%d", len(rows), len(items))
	}
	item := items[0]
	if item.DetailURL != "https://flash.jin10.com/detail/20260626130000123" || item.PublishTime != "2026-06-26 13:00:00" {
		t.Fatalf("unexpected mapped detail/time: %+v", item)
	}
	if item.Content != "上海出口集装箱结算运价指数上涨。" || item.FromText != "来自：金十期货" {
		t.Fatalf("unexpected mapped content/source: %+v", item)
	}
	if item.SourceURL != "https://example.com/news/1" || item.ExternalSourceHost != "example.com" {
		t.Fatalf("unexpected source url/host: %+v", item)
	}
	if !item.IsVIP || !item.HasImage || item.TagFlags != "A股/沪深300 IF" {
		t.Fatalf("unexpected flags: %+v", item)
	}
}

func TestProviderFetchWithOptionsPaginatesFlashAPIWindow(t *testing.T) {
	requestedMaxTimes := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get_flash_list" {
			http.NotFound(w, r)
			return
		}
		maxTime := r.URL.Query().Get("max_time")
		requestedMaxTimes = append(requestedMaxTimes, maxTime)
		w.Header().Set("Content-Type", "application/json")
		switch maxTime {
		case "2026-06-26 09:30:00":
			_, _ = w.Write([]byte(`{"status":200,"data":[
				{"id":"a","time":"2026-06-26 09:29:30","data":{"content":"窗口内新快讯"}},
				{"id":"b","time":"2026-06-26 09:25:00","data":{"content":"继续向前翻页"}}
			]}`))
		case "2026-06-26 09:24:59":
			_, _ = w.Write([]byte(`{"status":200,"data":[
				{"id":"c","time":"2026-06-26 09:20:00","data":{"content":"窗口起点快讯"}},
				{"id":"d","time":"2026-06-26 09:19:30","data":{"content":"窗口外停止锚点"}}
			]}`))
		default:
			t.Fatalf("unexpected max_time %q", maxTime)
		}
	}))
	defer server.Close()

	prov := NewProvider(resty.New(), server.URL+"/get_flash_list")
	items, err := prov.FetchWithOptions(context.Background(), model.CrawlOptions{
		Start:     "2026-06-26 09:20:00",
		End:       "2026-06-26 09:30:00",
		TimeField: "publish_time",
	})
	if err != nil {
		t.Fatalf("FetchWithOptions error: %v", err)
	}
	expectedMaxTimes := []string{"2026-06-26 09:30:00", "2026-06-26 09:24:59"}
	if !reflect.DeepEqual(requestedMaxTimes, expectedMaxTimes) {
		t.Fatalf("unexpected max_time sequence: got %#v want %#v", requestedMaxTimes, expectedMaxTimes)
	}
	if len(items) != 4 {
		t.Fatalf("expected collected rows from both pages, got %d: %+v", len(items), items)
	}
	if items[2].Title != "窗口起点快讯" || items[2].PublishTime != "2026-06-26 09:20:00" {
		t.Fatalf("expected second page item, got %+v", items[2])
	}
}

func TestProviderFetchFallsBackToHTMLWhenAPIFails(t *testing.T) {
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_flash_list":
			http.Error(w, "api unavailable", http.StatusBadGateway)
		case "/":
			_, _ = w.Write([]byte(`<div id="jin_slide_news" data-list='[{"title":"HTML兜底快讯","content":"旧页面仍可解析","url":"` + baseURL + `/detail/1"}]'></div>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	prov := &Provider{
		client:          resty.New(),
		pageURL:         server.URL + "/",
		apiURL:          server.URL + "/get_flash_list",
		fallbackHTMLURL: server.URL + "/",
	}
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(items) != 1 || items[0].Title != "HTML兜底快讯" {
		t.Fatalf("expected HTML fallback item, got %+v", items)
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
