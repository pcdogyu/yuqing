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

	"github.com/stonedt-yuqing/go-jin10/internal/model"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/cryptosocial"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
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
