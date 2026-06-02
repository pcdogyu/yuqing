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

func TestPreferencesPopupAndMailConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	pref, err := store.UpsertUserPreference(ctx, model.UserPreference{
		UserID:             1,
		Language:           "zh-CN",
		Theme:              "light",
		DefaultSearchMode:  "full",
		ArticlePageSize:    50,
		EmailNotifications: true,
	})
	if err != nil {
		t.Fatalf("UpsertUserPreference error: %v", err)
	}
	if pref.DefaultSearchMode != "full" || pref.ArticlePageSize != 50 {
		t.Fatalf("unexpected preference: %+v", pref)
	}

	popup, err := store.UpsertPopupState(ctx, model.PopupState{UserID: 1, Key: "system-announcement", Dismissed: true})
	if err != nil {
		t.Fatalf("UpsertPopupState error: %v", err)
	}
	if !popup.Dismissed || popup.DismissedAt == nil {
		t.Fatalf("unexpected popup state: %+v", popup)
	}

	mailCfg, err := store.UpsertMailConfig(ctx, model.MailConfig{
		Enabled:     true,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    465,
		Username:    "robot",
		Password:    "secret",
		SenderName:  "Bot",
		SenderEmail: "bot@example.com",
	})
	if err != nil {
		t.Fatalf("UpsertMailConfig error: %v", err)
	}
	if mailCfg.SMTPHost != "smtp.example.com" || mailCfg.SMTPPort != 465 {
		t.Fatalf("unexpected mail config: %+v", mailCfg)
	}
}

func TestAdvancedSearchAndAnalysisHelpers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	items := []model.Item{
		sampleItemWithPayload("headline", "meta-1", "AI 芯片产业上涨", `{"industry":"AI","province":"广东","city":"深圳"}`),
		sampleItemWithPayload("flash", "meta-2", "新能源下跌风险", `{"industry":"新能源","province":"上海","city":"上海"}`),
	}
	if _, _, err := store.UpsertItems(ctx, items); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	search, err := store.SearchItemsAdvanced(ctx, model.ArticleFilter{
		Page:     1,
		PageSize: 20,
		Province: "广东",
	})
	if err != nil {
		t.Fatalf("SearchItemsAdvanced error: %v", err)
	}
	if search.Total != 1 || len(search.Items) != 1 {
		t.Fatalf("unexpected search result: %+v", search)
	}

	options, err := store.ListSearchOptions(ctx)
	if err != nil {
		t.Fatalf("ListSearchOptions error: %v", err)
	}
	if len(options.Industries) == 0 || options.Industries[0] == "" {
		t.Fatalf("expected industries in options, got %+v", options)
	}

	emotions, err := store.BuildEmotionAnalysis(ctx, 0)
	if err != nil {
		t.Fatalf("BuildEmotionAnalysis error: %v", err)
	}
	if emotions.Total != 2 {
		t.Fatalf("expected 2 items in emotion analysis, got %+v", emotions)
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

func sampleItemWithPayload(sourceType, key, title, payload string) model.Item {
	item := sampleItem(sourceType, key, title)
	item.RawPayload = payload
	return item
}
