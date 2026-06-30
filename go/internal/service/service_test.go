package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptosocial"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

func TestCrawlerRunsCryptoSocialSourceWithDedup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"url":"https://x.com/a/status/1","content":"BTC ETF approval chatter","author":"@alpha","created_at":"2026-06-04T10:00:00Z"}]}`))
	}))
	defer server.Close()

	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "crawler-social.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	client := resty.New().SetTimeout(5 * time.Second)
	crawler := NewCrawler(store, provider.Registry{
		CryptoX: cryptosocial.NewXProvider(client, server.URL, ""),
	}, nil)

	first, err := crawler.Run(context.Background(), provider.SourceTypeCryptoX)
	if err != nil {
		t.Fatalf("first crawl error: %v", err)
	}
	second, err := crawler.Run(context.Background(), provider.SourceTypeCryptoX)
	if err != nil {
		t.Fatalf("second crawl error: %v", err)
	}

	if first.InsertedCount != 1 || first.UpdatedCount != 0 {
		t.Fatalf("unexpected first summary: %+v", first)
	}
	if second.InsertedCount != 0 || second.UpdatedCount != 1 {
		t.Fatalf("unexpected second summary: %+v", second)
	}

	items, err := store.LatestItems(context.Background(), 10, provider.SourceTypeCryptoX)
	if err != nil {
		t.Fatalf("LatestItems error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one persisted social item, got %d", len(items))
	}
}

func TestCrawlerRunWithOptionsFiltersPublishTimeWindow(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "crawler-window.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	crawler := NewCrawler(store, provider.Registry{
		Flash: fakeProvider{
			sourceType: provider.SourceTypeFlash,
			items: []model.Item{
				{Title: "上午新闻", PublishTime: "2026-06-16 09:10:00"},
				{Title: "下午新闻", PublishTime: "2026-06-16 13:10:00"},
				{Title: "旧发布时间新抓取", PublishTime: "2026-06-15 15:10:00", CapturedAt: time.Date(2026, 6, 16, 1, 10, 0, 0, time.UTC)},
				{Title: "无发布时间新闻"},
			},
		},
	}, nil)

	summary, err := crawler.RunWithOptions(context.Background(), provider.SourceTypeFlash, model.CrawlOptions{
		Start:     "2026-06-16 08:00:00",
		End:       "2026-06-16 09:30:59",
		TimeField: "publish_time",
	})
	if err != nil {
		t.Fatalf("RunWithOptions error: %v", err)
	}
	if summary.FetchedCount != 4 || summary.InsertedCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	items, err := store.ListItems(context.Background(), model.ArticleFilter{
		Page:      1,
		PageSize:  10,
		Start:     "2026-06-16 08:00:00",
		End:       "2026-06-16 09:30:59",
		TimeField: "publish_time",
	})
	if err != nil {
		t.Fatalf("ListItems error: %v", err)
	}
	if len(items.Items) != 1 || items.Items[0].Title != "上午新闻" {
		t.Fatalf("expected only morning item, got %+v", items.Items)
	}
}

func TestChannelMatchesJin10LegacySourceTypes(t *testing.T) {
	if !channelMatches("flash,headline", provider.SourceTypeFlash) {
		t.Fatal("expected legacy flash channel to match jin10_kuaixun")
	}
	if !channelMatches("flash,headline", provider.SourceTypeHeadline) {
		t.Fatal("expected legacy headline channel to match jin10_资讯")
	}
	if !channelMatches("jin10_kuaixun,jin10_资讯", provider.LegacySourceTypeHeadline) {
		t.Fatal("expected canonical channels to match legacy headline")
	}
}

func TestCrawlerStartupMarksRunningCrawlRunsInterrupted(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "crawler-restart.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	runID, err := store.StartCrawlRun(context.Background(), provider.SourceTypeJin10Full, time.Date(2026, 6, 26, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("StartCrawlRun error: %v", err)
	}

	_ = NewCrawler(store, provider.Registry{}, nil)

	runs, err := store.ListCrawlRuns(context.Background(), 10, provider.SourceTypeJin10Full)
	if err != nil {
		t.Fatalf("ListCrawlRuns error: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != runID {
		t.Fatalf("expected one crawl run %d, got %+v", runID, runs)
	}
	if runs[0].Status != "failed" || runs[0].ErrorText != "interrupted by service restart" || runs[0].FinishedAt == nil {
		t.Fatalf("expected interrupted failed run, got %+v", runs[0])
	}
}

func TestBuildSourceKeyKeepsJin10FullSeparateFromFlash(t *testing.T) {
	detailURL := "https://flash.jin10.com/detail/20260626093000123"
	flashKey := buildSourceKey(model.Item{
		SourceType: provider.SourceTypeFlash,
		DetailURL:  detailURL,
	})
	fullKey := buildSourceKey(model.Item{
		SourceType: provider.SourceTypeJin10Full,
		DetailURL:  detailURL,
	})
	if flashKey != detailURL {
		t.Fatalf("expected flash key to remain detail url, got %q", flashKey)
	}
	if fullKey != provider.SourceTypeJin10Full+"|"+detailURL {
		t.Fatalf("unexpected jin10_full key: %q", fullKey)
	}
	if flashKey == fullKey {
		t.Fatalf("expected flash and jin10_full keys to be distinct")
	}
}

func TestCrawlerRunTemplateByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><article><h1>BTC ETF update</h1><p>Positive template item</p></article></body></html>`))
	}))
	defer server.Close()

	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "crawler-template.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	tpl, err := store.CreateCrawlTemplate(context.Background(), model.CrawlTemplate{
		Name:       "X BTC 热门账号模板",
		SourceType: provider.SourceTypeCryptoX,
		Enabled:    true,
		ConfigJSON: mustTemplateConfigJSON(t, server.URL),
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	crawler := NewCrawler(store, provider.Registry{}, resty.New().SetTimeout(5*time.Second))
	summary, err := crawler.RunTemplateByID(context.Background(), tpl.ID, "BTC")
	if err != nil {
		t.Fatalf("RunTemplateByID error: %v", err)
	}
	if summary.InsertedCount != 1 || summary.FetchedCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	items, err := store.LatestItems(context.Background(), 10, provider.SourceTypeCryptoX)
	if err != nil {
		t.Fatalf("LatestItems error: %v", err)
	}
	if len(items) != 1 || items[0].Title != "BTC ETF update" {
		t.Fatalf("unexpected persisted template item: %+v", items)
	}
}

type fakeProvider struct {
	sourceType string
	items      []model.Item
}

func (p fakeProvider) SourceType() string {
	return p.sourceType
}

func (p fakeProvider) Fetch(context.Context) ([]model.Item, error) {
	return p.items, nil
}

func mustTemplateConfigJSON(t *testing.T, baseURL string) string {
	t.Helper()
	cfg := model.CrawlTemplateConfig{
		SourceType:   provider.SourceTypeCryptoX,
		BaseURL:      baseURL,
		Method:       http.MethodGet,
		ListSelector: "article",
		Fields: []model.CrawlTemplateField{
			{Name: "title", Selector: "h1"},
			{Name: "content", Selector: "p"},
		},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal template config: %v", err)
	}
	return string(raw)
}
