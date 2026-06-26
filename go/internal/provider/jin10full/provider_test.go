package jin10full

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

type memoryStateStore struct {
	values map[string]string
}

func (s *memoryStateStore) GetCrawlState(context.Context, string, string) (string, error) {
	return "", nil
}

func (s *memoryStateStore) UpsertCrawlState(_ context.Context, sourceType, cursorKey, cursorValue string, _ time.Time) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[sourceType+"|"+cursorKey] = cursorValue
	return nil
}

func TestFetchCollectsFlashHeadlineAndSitemap(t *testing.T) {
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /blocked\n"))
		case "/":
			_, _ = w.Write([]byte(`<div id="jin_slide_news" data-list='[{"title":"黄金上涨","content":"避险情绪升温","url":"` + baseURL + `/flash/detail/1"}]'></div>`))
		case "/xnews":
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`<html><body></body></html>`))
				return
			}
			_, _ = w.Write([]byte(`<div class="jin10-news-list-item news"><a href="` + baseURL + `/details/2"></a><p class="jin10-news-list-item-title">美联储观察</p><div class="jin10-news-list-item-introduction">降息预期变化</div><span class="jin10-news-list-item-display_datetime">刚刚</span></div>`))
		case "/flash/detail/1":
			_, _ = w.Write([]byte(`<html><title>黄金上涨 - 金十数据</title><div class="content-title"><div class="flash-title">黄金上涨</div><div>避险情绪升温推动金价走高。</div></div></html>`))
		case "/details/2":
			_, _ = w.Write([]byte(`<html><h1>美联储观察</h1><div class="jin10-news-cdetails-content"><p>市场重新定价降息路径。</p></div></html>`))
		case "/details/3":
			_, _ = w.Write([]byte(`<html><h1>原油快讯</h1><article><p>供应扰动带来油价波动。</p></article></html>`))
		case "/blocked/4":
			_, _ = w.Write([]byte(`<html><h1>不应抓取</h1></html>`))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset><url><loc>` + baseURL + `/details/3</loc><lastmod>2026-06-04</lastmod></url><url><loc>` + baseURL + `/blocked/4</loc><lastmod>2026-06-04</lastmod></url></urlset>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	state := &memoryStateStore{}
	prov := NewProvider(resty.New().SetTimeout(2*time.Second), state, Options{
		FlashURL:       server.URL + "/",
		HeadlineURL:    server.URL + "/xnews",
		BackfillDays:   3650,
		MaxPagesPerRun: 5,
		RateLimit:      time.Millisecond,
		IncludeSitemap: true,
	})

	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %#v", len(items), items)
	}
	for _, item := range items {
		if item.SourceType != provider.SourceTypeJin10Full {
			t.Fatalf("expected jin10_full source type, got %q", item.SourceType)
		}
		if item.DetailURL == server.URL+"/blocked/4" {
			t.Fatalf("robots-disallowed detail was fetched: %#v", item)
		}
	}
	if state.values[provider.SourceTypeJin10Full+"|"+stateLastRunAt] == "" {
		t.Fatalf("expected last run state to be recorded")
	}
}

func TestFetchWithOptionsUsesWindowedFlashAndSkipsSitemap(t *testing.T) {
	var baseURL string
	var sitemapHits int
	var requestedMaxTime string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/get_flash_list":
			requestedMaxTime = r.URL.Query().Get("max_time")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"data":[{"id":"full-flash-1","time":"2026-06-26 09:20:00","data":{"content":"窗口内金十快讯","source":"金十数据"}}]}`))
		case "/xnews":
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`<html><body></body></html>`))
				return
			}
			_, _ = w.Write([]byte(`<div class="jin10-news-list-item news"><a href="` + baseURL + `/details/2"></a><p class="jin10-news-list-item-title">窗口内金十资讯</p><div class="jin10-news-list-item-introduction">相对时间已解析</div><span class="jin10-news-list-item-display_datetime">2026-06-26 09:28:00</span></div>`))
		case "/details/2":
			_, _ = w.Write([]byte(`<html><h1>窗口内金十资讯</h1><div class="jin10-news-cdetails-content"><p>资讯正文。</p></div></html>`))
		case "/sitemap.xml":
			sitemapHits++
			http.Error(w, "sitemap should be skipped for window fetch", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	prov := NewProvider(resty.New().SetTimeout(2*time.Second), &memoryStateStore{}, Options{
		FlashURL:       server.URL + "/get_flash_list",
		HeadlineURL:    server.URL + "/xnews",
		BackfillDays:   3650,
		MaxPagesPerRun: 3,
		RateLimit:      time.Millisecond,
		IncludeSitemap: true,
	})

	items, err := prov.FetchWithOptions(context.Background(), model.CrawlOptions{
		Start:     "2026-06-26 09:20:00",
		End:       "2026-06-26 09:30:00",
		TimeField: "publish_time",
	})
	if err != nil {
		t.Fatalf("FetchWithOptions returned error: %v", err)
	}
	if requestedMaxTime != "2026-06-26 09:30:00" {
		t.Fatalf("expected flash API window max_time, got %q", requestedMaxTime)
	}
	if sitemapHits != 0 {
		t.Fatalf("expected sitemap to be skipped for window fetch, hits=%d", sitemapHits)
	}
	if len(items) != 2 {
		t.Fatalf("expected flash and headline items, got %d: %+v", len(items), items)
	}
	for _, item := range items {
		if item.SourceType != provider.SourceTypeJin10Full {
			t.Fatalf("expected jin10_full source type, got %q", item.SourceType)
		}
	}
}

func TestParseDisallow(t *testing.T) {
	rules := parseDisallow("User-agent: *\nDisallow: /private\nAllow: /\n")
	if len(rules) != 1 || rules[0] != "/private" {
		t.Fatalf("unexpected rules: %#v", rules)
	}
}
