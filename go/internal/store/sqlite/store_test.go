package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func TestUpsertAndListItems(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first := sampleItem("flash", "flash-key-1", "美元指数快速拉升")
	second := sampleItem("headline", "headline-key-1", "华尔街热议")

	inserted, updated, err := store.UpsertItems(ctx, []model.Item{first, second})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	if inserted != 2 || updated != 0 {
		t.Fatalf("unexpected insert/update counts: %d/%d", inserted, updated)
	}

	first.Title = "美元指数刷新日内高点"
	inserted, updated, err = store.UpsertItems(ctx, []model.Item{first})
	if err != nil {
		t.Fatalf("UpsertItems update error: %v", err)
	}
	if inserted != 0 || updated != 1 {
		t.Fatalf("unexpected second insert/update counts: %d/%d", inserted, updated)
	}

	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Keyword: "华尔街", SourceType: "headline"})
	if err != nil {
		t.Fatalf("ListItems error: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("unexpected list result: %+v", list)
	}
	if list.Items[0].SourceType != "headline" {
		t.Fatalf("unexpected source type: %s", list.Items[0].SourceType)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jin10.db")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func sampleItem(sourceType, key, title string) model.Item {
	now := time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC)
	return model.Item{
		SourceType:      sourceType,
		SourceKey:       key,
		Title:           title,
		Content:         title + " 内容",
		Summary:         title + " 摘要",
		PublishTime:     "2026-05-29 11:00:00",
		PublishTimeText: "5分钟前",
		DetailURL:       "https://example.com/" + key,
		SourceURL:       "https://example.com",
		CapturedAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
