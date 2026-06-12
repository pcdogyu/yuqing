package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10flash"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10xnews"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

func TestLiveCrawlJin10Sources(t *testing.T) {
	if os.Getenv("RUN_LIVE_JIN10") != "1" {
		t.Skip("set RUN_LIVE_JIN10=1 to enable live integration test")
	}

	dbPath := filepath.Join(t.TempDir(), "jin10-live.db")
	store, err := sqlitestore.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	client := resty.New().
		SetTimeout(20*time.Second).
		SetRetryCount(1).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")

	crawler := NewCrawler(store, provider.Registry{
		Flash:    jin10flash.NewProvider(client, "https://www.jin10.com/"),
		Headline: jin10xnews.NewProvider(client, "https://xnews.jin10.com/"),
	}, nil)

	ctx := context.Background()
	flashSummary, err := crawler.Run(ctx, provider.SourceTypeFlash)
	if err != nil {
		t.Fatalf("flash crawl failed: %v", err)
	}
	headlineSummary, err := crawler.Run(ctx, provider.SourceTypeHeadline)
	if err != nil {
		t.Fatalf("headline crawl failed: %v", err)
	}

	if flashSummary.FetchedCount == 0 {
		t.Fatal("expected flash items from live crawl")
	}
	if headlineSummary.FetchedCount == 0 {
		t.Fatal("expected headline items from live crawl")
	}

	flashItems, err := store.LatestItems(ctx, 5, provider.SourceTypeFlash)
	if err != nil {
		t.Fatalf("list flash items: %v", err)
	}
	headlineItems, err := store.LatestItems(ctx, 5, provider.SourceTypeHeadline)
	if err != nil {
		t.Fatalf("list headline items: %v", err)
	}
	if len(flashItems) == 0 || len(headlineItems) == 0 {
		t.Fatalf("expected both source types persisted, got flash=%d headline=%d", len(flashItems), len(headlineItems))
	}
}
