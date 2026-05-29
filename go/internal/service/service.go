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
	ListItems(context.Context, model.ArticleFilter) (model.ItemListResult, error)
	LatestItems(context.Context, int, string) ([]model.Item, error)
	GetItem(context.Context, int64) (model.Item, error)
	GetRelatedItems(context.Context, int64, int) ([]model.Item, error)
	ListCrawlRuns(context.Context, int, string) ([]model.CrawlRun, error)
	ListActiveMonitorRules(context.Context) ([]model.MonitorRule, error)
	LinkItemsToProjects(context.Context, []string, []int64, int64) error
	RecordTaskRun(context.Context, string, string, string, time.Time, *time.Time) error
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
		finishedAt := time.Now().UTC()
		_ = c.store.RecordTaskRun(ctx, "crawl:"+sourceType, "failed", fetchErr.Error(), startedAt, &finishedAt)
		return summary, fetchErr
	}

	now := time.Now().UTC()
	sourceKeys := make([]string, 0, len(items))
	matches := make(map[int64][]string)
	rules, _ := c.store.ListActiveMonitorRules(ctx)
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
		sourceKeys = append(sourceKeys, items[index].SourceKey)
		for _, rule := range rules {
			if ruleMatches(rule, sourceType, items[index]) {
				matches[rule.ID] = append(matches[rule.ID], items[index].SourceKey)
			}
		}
	}

	inserted, updated, err := c.store.UpsertItems(ctx, items)
	if err != nil {
		summary.ErrorText = err.Error()
		_ = c.store.FinishCrawlRun(ctx, runID, "failed", len(items), 0, 0, err.Error(), time.Now().UTC())
		finishedAt := time.Now().UTC()
		_ = c.store.RecordTaskRun(ctx, "crawl:"+sourceType, "failed", err.Error(), startedAt, &finishedAt)
		return summary, err
	}

	for _, rule := range rules {
		keys := matches[rule.ID]
		if len(keys) == 0 {
			continue
		}
		_ = c.store.LinkItemsToProjects(ctx, keys, []int64{rule.ProjectID}, rule.ID)
	}

	summary.InsertedCount = inserted
	summary.UpdatedCount = updated
	if err := c.store.FinishCrawlRun(ctx, runID, "success", len(items), inserted, updated, "", time.Now().UTC()); err != nil {
		return summary, err
	}
	finishedAt := time.Now().UTC()
	_ = c.store.RecordTaskRun(ctx, "crawl:"+sourceType, "success", "crawl completed", startedAt, &finishedAt)
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

func (c *Crawler) ListItems(ctx context.Context, filter model.ArticleFilter) (model.ItemListResult, error) {
	return c.store.ListItems(ctx, filter)
}

func (c *Crawler) LatestItems(ctx context.Context, limit int, sourceType string) ([]model.Item, error) {
	return c.store.LatestItems(ctx, limit, sourceType)
}

func (c *Crawler) GetItem(ctx context.Context, id int64) (model.Item, error) {
	return c.store.GetItem(ctx, id)
}

func (c *Crawler) GetRelatedItems(ctx context.Context, id int64, limit int) ([]model.Item, error) {
	return c.store.GetRelatedItems(ctx, id, limit)
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

func ruleMatches(rule model.MonitorRule, sourceType string, item model.Item) bool {
	if strings.TrimSpace(rule.Status) != "" && !strings.EqualFold(rule.Status, "active") {
		return false
	}
	if !channelMatches(rule.Channels, sourceType) {
		return false
	}
	body := strings.ToLower(item.Title + " " + item.Summary + " " + item.Content)
	includes := splitCSV(rule.IncludeKeywords)
	excludes := splitCSV(rule.ExcludeKeywords)
	if len(includes) > 0 {
		match := false
		for _, keyword := range includes {
			if strings.Contains(body, strings.ToLower(keyword)) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	for _, keyword := range excludes {
		if strings.Contains(body, strings.ToLower(keyword)) {
			return false
		}
	}
	return true
}

func channelMatches(channels, sourceType string) bool {
	parts := splitCSV(channels)
	if len(parts) == 0 {
		return true
	}
	for _, part := range parts {
		if strings.EqualFold(part, sourceType) || strings.EqualFold(part, "all") {
			return true
		}
	}
	return false
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '|' || r == ';' || r == '，' })
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}
