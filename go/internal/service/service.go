package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

type Store interface {
	StartCrawlRun(context.Context, string, time.Time) (int64, error)
	FinishCrawlRun(context.Context, int64, string, int, int, int, string, time.Time) error
	UpsertItems(context.Context, []model.Item) (int, int, error)
	ListItems(context.Context, int, int, string, string) (model.ItemListResult, error)
	LatestItems(context.Context, int, string) ([]model.Item, error)
	GetItem(context.Context, int64) (model.Item, error)
	ListCrawlRuns(context.Context, int, string) ([]model.CrawlRun, error)
}

type Crawler struct {
	store     Store
	providers provider.Registry
}

func NewCrawler(store Store, providers provider.Registry) *Crawler {
	return &Crawler{store: store, providers: providers}
}

func (c *Crawler) Run(ctx context.Context, sourceType string) (model.CrawlSummary, error) {
	prov := c.providers.Resolve(sourceType)
	if prov == nil {
		return model.CrawlSummary{}, errors.New("unsupported source_type")
	}

	startedAt := time.Now().UTC()
	runID, err := c.store.StartCrawlRun(ctx, sourceType, startedAt)
	if err != nil {
		return model.CrawlSummary{}, err
	}

	items, fetchErr := prov.Fetch(ctx)
	summary := model.CrawlSummary{
		SourceType:   sourceType,
		FetchedCount: len(items),
		RunID:        runID,
	}

	if fetchErr != nil {
		summary.ErrorText = fetchErr.Error()
		_ = c.store.FinishCrawlRun(ctx, runID, "failed", 0, 0, 0, fetchErr.Error(), time.Now().UTC())
		return summary, fetchErr
	}

	now := time.Now().UTC()
	for index := range items {
		items[index].SourceType = sourceType
		items[index].CapturedAt = now
		if items[index].CreatedAt.IsZero() {
			items[index].CreatedAt = now
		}
		items[index].UpdatedAt = now
		items[index].SourceKey = buildSourceKey(items[index])
		items[index].Title = strings.TrimSpace(items[index].Title)
		items[index].Content = strings.TrimSpace(items[index].Content)
		items[index].Summary = strings.TrimSpace(items[index].Summary)
	}

	inserted, updated, err := c.store.UpsertItems(ctx, items)
	if err != nil {
		summary.ErrorText = err.Error()
		_ = c.store.FinishCrawlRun(ctx, runID, "failed", len(items), 0, 0, err.Error(), time.Now().UTC())
		return summary, err
	}

	summary.InsertedCount = inserted
	summary.UpdatedCount = updated
	if err := c.store.FinishCrawlRun(ctx, runID, "success", len(items), inserted, updated, "", time.Now().UTC()); err != nil {
		return summary, err
	}
	return summary, nil
}

func (c *Crawler) RunAll(ctx context.Context) ([]model.CrawlSummary, error) {
	summaries := make([]model.CrawlSummary, 0, 2)
	for _, prov := range c.providers.All() {
		if prov == nil {
			continue
		}
		summary, err := c.Run(ctx, prov.SourceType())
		summaries = append(summaries, summary)
		if err != nil {
			return summaries, err
		}
	}
	return summaries, nil
}

func (c *Crawler) ListItems(ctx context.Context, page, pageSize int, keyword, sourceType string) (model.ItemListResult, error) {
	return c.store.ListItems(ctx, page, pageSize, keyword, sourceType)
}

func (c *Crawler) LatestItems(ctx context.Context, limit int, sourceType string) ([]model.Item, error) {
	return c.store.LatestItems(ctx, limit, sourceType)
}

func (c *Crawler) GetItem(ctx context.Context, id int64) (model.Item, error) {
	return c.store.GetItem(ctx, id)
}

func (c *Crawler) ListCrawlRuns(ctx context.Context, limit int, sourceType string) ([]model.CrawlRun, error) {
	return c.store.ListCrawlRuns(ctx, limit, sourceType)
}

func IsNotFound(err error) bool {
	return errors.Is(err, sqlitestore.ErrNotFound)
}

func buildSourceKey(item model.Item) string {
	if strings.TrimSpace(item.DetailURL) != "" {
		return item.DetailURL
	}
	payload := item.SourceType + "|" + item.Title + "|" + item.PublishTimeText + "|" + item.PublishTime + "|" + item.Content
	digest := sha1.Sum([]byte(payload))
	return hex.EncodeToString(digest[:])
}
