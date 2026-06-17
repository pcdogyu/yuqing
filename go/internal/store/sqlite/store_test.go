package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
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

func TestListItemsCanFilterByPublishTime(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	morning := sampleItem("flash", "flash-key-morning", "上午 AI 新闻")
	morning.PublishTime = "2026-06-16 09:10:00"
	morning.CapturedAt = time.Date(2026, 6, 16, 8, 30, 0, 0, time.UTC)
	morning.CreatedAt = morning.CapturedAt
	morning.UpdatedAt = morning.CapturedAt
	afternoon := sampleItem("flash", "flash-key-afternoon", "下午 AI 新闻")
	afternoon.PublishTime = "2026-06-16 15:10:00"
	afternoon.CapturedAt = morning.CapturedAt
	afternoon.CreatedAt = morning.CapturedAt
	afternoon.UpdatedAt = morning.CapturedAt

	if _, _, err := store.UpsertItems(ctx, []model.Item{morning, afternoon}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	list, err := store.ListItems(ctx, model.ArticleFilter{
		Page:      1,
		PageSize:  10,
		TimeField: "publish_time",
		Start:     "2026-06-16 08:00:00",
		End:       "2026-06-16 09:30:59",
	})
	if err != nil {
		t.Fatalf("ListItems by publish_time error: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].SourceKey != "flash-key-morning" {
		t.Fatalf("expected only morning publish_time item, got %+v", list)
	}

	defaultList, err := store.ListItems(ctx, model.ArticleFilter{
		Page:     1,
		PageSize: 10,
		Start:    "2026-06-16 08:00:00",
		End:      "2026-06-16 09:30:59",
	})
	if err != nil {
		t.Fatalf("ListItems by default captured_at error: %v", err)
	}
	if defaultList.Total != 0 {
		t.Fatalf("expected default filter to keep using captured_at, got %+v", defaultList)
	}
}

func TestAStockAuctionAmountsUpsertAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 6, 16, 1, 30, 0, 0, time.UTC)

	first, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-16", []model.AStockAuctionAmount{
		{Code: "002230", Name: "科大讯飞", AuctionPrice: 41.2, AuctionVolume: 123400, AuctionAmount: 5084080, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "600000", Name: "浦发银行", AuctionPrice: 8.8, AuctionVolume: 90000, AuctionAmount: 792000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
	})
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts insert error: %v", err)
	}
	if first.Inserted != 2 || first.Updated != 0 {
		t.Fatalf("unexpected insert result: %+v", first)
	}

	second, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-16", []model.AStockAuctionAmount{
		{Code: "002230", Name: "科大讯飞", AuctionPrice: 42, AuctionVolume: 100000, AuctionAmount: 4200000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
	})
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts update error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 1 {
		t.Fatalf("unexpected update result: %+v", second)
	}

	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-15", []model.AStockAuctionAmount{
		{Code: "000001", Name: "平安银行", AuctionPrice: 12, AuctionVolume: 10000, AuctionAmount: 120000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt.AddDate(0, 0, -1)},
	}); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts previous day error: %v", err)
	}

	latest, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts latest error: %v", err)
	}
	if latest.Date != "2026-06-16" || latest.LatestDate != "2026-06-16" || latest.Total != 2 || len(latest.Items) != 2 {
		t.Fatalf("unexpected latest auction list: %+v", latest)
	}
	if latest.Items[0].Code != "002230" || latest.Items[0].AuctionAmount != 4200000 || latest.TotalAmount != 4992000 {
		t.Fatalf("expected updated highest amount row and total amount, got %+v", latest)
	}
	if latest.MaxItem == nil || latest.MaxItem.Code != "002230" {
		t.Fatalf("expected max item, got %+v", latest.MaxItem)
	}

	filtered, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-06-16", Keyword: "浦发", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts filtered error: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].Code != "600000" {
		t.Fatalf("expected keyword filtered row, got %+v", filtered)
	}
}

func TestNewStoreSetsBusyTimeout(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	var timeout int
	if err := store.db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != busyTimeoutMillis {
		t.Fatalf("expected busy_timeout %d, got %d", busyTimeoutMillis, timeout)
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

func TestWarningAndOpinionConditions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	warning, err := store.UpsertWarningSetting(ctx, model.WarningSetting{
		ProjectID:            11,
		WarningStatus:        1,
		WarningName:          "预警",
		WarningWord:          "钢铁,能源",
		WarningClassify:      "1,2,3",
		WarningContent:       1,
		WarningSimilar:       1,
		WarningMatch:         2,
		WarningDeduplication: 1,
		WarningSource:        `{"type":"2","email":"ops@example.com"}`,
		WarningReceiveTime:   `{"start":"08:00","end":"18:00"}`,
		WeekendWarning:       1,
		WarningInterval:      `{"type":"2","time":"30"}`,
		Enabled:              true,
		Channels:             "1,2,3",
		Threshold:            30,
		Recipients:           "ops@example.com",
		Description:          "预警",
	})
	if err != nil {
		t.Fatalf("UpsertWarningSetting error: %v", err)
	}
	if warning.WarningWord != "钢铁,能源" || warning.WarningStatus != 1 || !warning.Enabled {
		t.Fatalf("unexpected warning setting: %+v", warning)
	}

	condition, err := store.UpsertOpinionCondition(ctx, model.OpinionCondition{
		ProjectID:          11,
		OpinionConditionID: 101,
		Time:               8,
		Precise:            1,
		Emotion:            `[1,3]`,
		Similar:            1,
		Sort:               2,
		Matchs:             3,
		Times:              "2026-06-01",
		Timee:              "2026-06-03",
		Province:           "上海",
		City:               "上海",
		CreateTime:         "2026-06-03 10:00:00",
	})
	if err != nil {
		t.Fatalf("UpsertOpinionCondition error: %v", err)
	}
	if condition.ProjectID != 11 || condition.OpinionConditionID != 101 || condition.Time != 8 || condition.Emotion != `[1,3]` {
		t.Fatalf("unexpected opinion condition: %+v", condition)
	}

	loaded, err := store.GetOpinionCondition(ctx, 11)
	if err != nil {
		t.Fatalf("GetOpinionCondition error: %v", err)
	}
	if loaded.Sort != 2 || loaded.Matchs != 3 || loaded.Province != "上海" {
		t.Fatalf("unexpected loaded opinion condition: %+v", loaded)
	}
}

func TestUpdateUserPassword(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.EnsureDefaultAdmin(ctx, "admin", "old-secret"); err != nil {
		t.Fatalf("EnsureDefaultAdmin error: %v", err)
	}

	user, err := store.AuthenticateUser(ctx, "admin", "old-secret")
	if err != nil {
		t.Fatalf("AuthenticateUser old password error: %v", err)
	}
	if user.Username != "admin" {
		t.Fatalf("unexpected user: %+v", user)
	}

	if err := store.UpdateUserPassword(ctx, user.ID, "new-secret"); err != nil {
		t.Fatalf("UpdateUserPassword error: %v", err)
	}

	if _, err := store.AuthenticateUser(ctx, "admin", "old-secret"); err == nil {
		t.Fatal("expected old password to stop working")
	}
	if _, err := store.AuthenticateUser(ctx, "admin", "new-secret"); err != nil {
		t.Fatalf("AuthenticateUser new password error: %v", err)
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

func TestSearchWordHistory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	words := []string{"钢铁", "能源", "钢铁"}
	for _, word := range words {
		if err := store.SaveSearchWord(ctx, 42, word); err != nil {
			t.Fatalf("SaveSearchWord error: %v", err)
		}
	}

	stats, err := store.ListSearchWords(ctx, 42, 10)
	if err != nil {
		t.Fatalf("ListSearchWords error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 unique search words, got %+v", stats)
	}
	if stats[0].SearchWord != "钢铁" || stats[0].WordCount != 2 {
		t.Fatalf("unexpected first history item: %+v", stats[0])
	}
	if stats[1].SearchWord != "能源" || stats[1].WordCount != 1 {
		t.Fatalf("unexpected second history item: %+v", stats[1])
	}
}

func TestSearchWordSuggestionsAndHotKeywords(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seed := []struct {
		userID int64
		word   string
	}{
		{42, "钢铁"},
		{42, "钢铁"},
		{42, "钢材"},
		{42, "能源"},
		{7, "钢铁"},
		{7, "科技"},
	}
	for _, item := range seed {
		if err := store.SaveSearchWord(ctx, item.userID, item.word); err != nil {
			t.Fatalf("SaveSearchWord error: %v", err)
		}
	}

	suggestions, err := store.ListSearchWordSuggestions(ctx, 42, "钢", 10)
	if err != nil {
		t.Fatalf("ListSearchWordSuggestions error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %+v", suggestions)
	}
	if suggestions[0].SearchWord != "钢铁" || suggestions[0].WordCount != 2 {
		t.Fatalf("unexpected first suggestion: %+v", suggestions[0])
	}
	if suggestions[1].SearchWord != "钢材" || suggestions[1].WordCount != 1 {
		t.Fatalf("unexpected second suggestion: %+v", suggestions[1])
	}

	hot, err := store.ListHotSearchWords(ctx, 10)
	if err != nil {
		t.Fatalf("ListHotSearchWords error: %v", err)
	}
	if len(hot) < 2 {
		t.Fatalf("expected hot keywords, got %+v", hot)
	}
	if hot[0].SearchWord != "钢铁" || hot[0].WordCount != 3 {
		t.Fatalf("unexpected hot keyword ranking: %+v", hot[0])
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
