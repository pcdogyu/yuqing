package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestListItemsCanSortByPublishTimeDescInPostgresMode(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	recrawledOld := sampleItem("flash", "flash-key-recrawled-old-pg", "旧快讯被重复抓取")
	recrawledOld.PublishTime = "2026-06-19 16:57:53"
	recrawledOld.CapturedAt = time.Date(2026, 6, 23, 4, 9, 16, 0, time.UTC)
	recrawledOld.CreatedAt = time.Date(2026, 6, 19, 8, 58, 21, 0, time.UTC)
	recrawledOld.UpdatedAt = recrawledOld.CapturedAt

	latestPublished := sampleItem("panews_newsflash", "panews-key-latest-pg", "真正的最新新闻")
	latestPublished.PublishTime = "2026-06-23T03:58:00Z"
	latestPublished.CapturedAt = time.Date(2026, 6, 23, 4, 0, 0, 0, time.UTC)
	latestPublished.CreatedAt = latestPublished.CapturedAt
	latestPublished.UpdatedAt = latestPublished.CapturedAt

	if _, _, err := store.UpsertItems(ctx, []model.Item{recrawledOld, latestPublished}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	store.driver = "postgres"
	store.db.driver = "postgres"

	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Sort: "publish_time_desc"})
	if err != nil {
		t.Fatalf("ListItems publish_time_desc postgres mode error: %v", err)
	}
	if len(list.Items) != 2 || list.Items[0].SourceKey != latestPublished.SourceKey {
		t.Fatalf("expected publish_time_desc to prefer latest published item in postgres mode, got %+v", list.Items)
	}
}
