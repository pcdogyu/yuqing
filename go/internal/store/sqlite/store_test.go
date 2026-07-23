package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
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
	canonicalList, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Keyword: "华尔街", SourceType: provider.SourceTypeHeadline})
	if err != nil {
		t.Fatalf("ListItems canonical source error: %v", err)
	}
	if canonicalList.Total != 1 || len(canonicalList.Items) != 1 || canonicalList.Items[0].SourceType != "headline" {
		t.Fatalf("expected canonical source filter to include legacy headline row, got %+v", canonicalList)
	}
}

func TestListItemsLiteOmitsLargeFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	item := sampleItemWithPayload("flash", "flash-lite-1", "AI 算力消息", `{"stockList":[{"code":"002230"}],"body":"large"}`)
	item.TagFlags = "0.002230"

	if _, _, err := store.UpsertItems(ctx, []model.Item{item}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	full, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListItems full error: %v", err)
	}
	if len(full.Items) != 1 || full.Items[0].Content == "" || full.Items[0].RawPayload == "" {
		t.Fatalf("expected full list to include large fields, got %+v", full.Items)
	}
	lite, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Lite: true})
	if err != nil {
		t.Fatalf("ListItems lite error: %v", err)
	}
	if len(lite.Items) != 1 {
		t.Fatalf("expected one lite item, got %+v", lite)
	}
	got := lite.Items[0]
	if got.Content != "" || got.RawPayload != "" {
		t.Fatalf("expected lite list to omit large fields, got content=%q raw=%q", got.Content, got.RawPayload)
	}
	if got.Title != item.Title || got.Summary != item.Summary || got.TagFlags != item.TagFlags || got.SourceType != item.SourceType {
		t.Fatalf("expected lite list to keep match fields, got %+v", got)
	}
}

func TestUpsertItemsSkipsRecentDuplicateTitles(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	recent := sampleItem("flash", "recent-title-1", "重复标题")
	recent.CapturedAt = now.Add(-time.Hour)
	recent.CreatedAt = recent.CapturedAt
	recent.UpdatedAt = recent.CapturedAt
	inserted, updated, err := store.UpsertItems(ctx, []model.Item{recent})
	if err != nil {
		t.Fatalf("UpsertItems recent error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("unexpected recent insert/update counts: %d/%d", inserted, updated)
	}

	recent.Summary = "同 source_key 仍允许更新"
	recent.UpdatedAt = now
	inserted, updated, err = store.UpsertItems(ctx, []model.Item{recent})
	if err != nil {
		t.Fatalf("UpsertItems same source_key update error: %v", err)
	}
	if inserted != 0 || updated != 1 {
		t.Fatalf("expected same source_key to update, got %d/%d", inserted, updated)
	}

	dbDuplicate := sampleItem("headline", "recent-title-2", "重复标题")
	dbDuplicate.CapturedAt = now
	dbDuplicate.CreatedAt = now
	dbDuplicate.UpdatedAt = now
	batchFirst := sampleItem("flash", "batch-title-1", "批内重复标题")
	batchFirst.CapturedAt = now
	batchFirst.CreatedAt = now
	batchFirst.UpdatedAt = now
	batchDuplicate := sampleItem("headline", "batch-title-2", "批内重复标题")
	batchDuplicate.CapturedAt = now
	batchDuplicate.CreatedAt = now
	batchDuplicate.UpdatedAt = now
	inserted, updated, err = store.UpsertItems(ctx, []model.Item{dbDuplicate, batchFirst, batchDuplicate})
	if err != nil {
		t.Fatalf("UpsertItems duplicates error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("expected only first batch title to insert, got %d/%d", inserted, updated)
	}

	assertItemTitleCount(t, store, ctx, "重复标题", 1)
	assertItemTitleCount(t, store, ctx, "批内重复标题", 1)
}

func TestUpsertItemsSkipsJin10FullDuplicateTitle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	flash := sampleItem("flash", "flash-same-title", "同标题金十快讯")
	flash.CapturedAt = now
	flash.CreatedAt = now
	flash.UpdatedAt = now
	full := sampleItem("jin10_full", "jin10-full-same-title", "同标题金十快讯")
	full.CapturedAt = now
	full.CreatedAt = now
	full.UpdatedAt = now

	inserted, updated, err := store.UpsertItems(ctx, []model.Item{flash, full})
	if err != nil {
		t.Fatalf("UpsertItems duplicate jin10_full error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("expected jin10_full duplicate title to be skipped, got %d/%d", inserted, updated)
	}
	assertItemTitleCount(t, store, ctx, "同标题金十快讯", 1)
}

func TestUpsertItemsAllowsDuplicateTitleAfterTwentyFourHours(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	old := sampleItem("flash", "old-title-1", "跨日标题")
	old.CapturedAt = now.Add(-25 * time.Hour)
	old.CreatedAt = old.CapturedAt
	old.UpdatedAt = old.CapturedAt
	fresh := sampleItem("headline", "old-title-2", "跨日标题")
	fresh.CapturedAt = now
	fresh.CreatedAt = now
	fresh.UpdatedAt = now

	inserted, updated, err := store.UpsertItems(ctx, []model.Item{old})
	if err != nil {
		t.Fatalf("UpsertItems old title error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("expected old title to insert, got %d/%d", inserted, updated)
	}
	inserted, updated, err = store.UpsertItems(ctx, []model.Item{fresh})
	if err != nil {
		t.Fatalf("UpsertItems fresh duplicate title error: %v", err)
	}
	if inserted != 1 || updated != 0 {
		t.Fatalf("expected fresh title to insert after 24 hours, got %d/%d", inserted, updated)
	}
	assertItemTitleCount(t, store, ctx, "跨日标题", 2)
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

func TestStoreCreatesItemTimeRangeIndexes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, name := range []string{
		"idx_items_captured_at",
		"idx_items_publish_time",
	} {
		var found string
		if err := store.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&found); err != nil {
			t.Fatalf("expected index %s to exist: %v", name, err)
		}
	}
}

func TestListItemsCanSortByPublishTimeDesc(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	recrawledOld := sampleItem("flash", "flash-key-recrawled-old", "旧快讯被重复抓取")
	recrawledOld.PublishTime = "2026-06-19 16:57:53"
	recrawledOld.CapturedAt = time.Date(2026, 6, 23, 4, 9, 16, 0, time.UTC)
	recrawledOld.CreatedAt = time.Date(2026, 6, 19, 8, 58, 21, 0, time.UTC)
	recrawledOld.UpdatedAt = recrawledOld.CapturedAt

	latestPublished := sampleItem("panews_newsflash", "panews-key-latest", "真正的最新新闻")
	latestPublished.PublishTime = "2026-06-23T03:58:00Z"
	latestPublished.CapturedAt = time.Date(2026, 6, 23, 4, 0, 0, 0, time.UTC)
	latestPublished.CreatedAt = latestPublished.CapturedAt
	latestPublished.UpdatedAt = latestPublished.CapturedAt

	relativeLatest := sampleItem("headline", "headline-key-relative-latest", "相对时间最新新闻")
	relativeLatest.PublishTime = ""
	relativeLatest.PublishTimeText = "7分钟前"
	relativeLatest.CapturedAt = time.Date(2026, 6, 23, 4, 8, 0, 0, time.UTC)
	relativeLatest.CreatedAt = relativeLatest.CapturedAt
	relativeLatest.UpdatedAt = relativeLatest.CapturedAt

	if _, _, err := store.UpsertItems(ctx, []model.Item{recrawledOld, latestPublished, relativeLatest}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	defaultList, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListItems default sort error: %v", err)
	}
	if len(defaultList.Items) != 3 || defaultList.Items[0].SourceKey != recrawledOld.SourceKey {
		t.Fatalf("expected default captured_at sort to keep recrawled old item first, got %+v", defaultList.Items)
	}

	publishTimeList, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Sort: "publish_time_desc"})
	if err != nil {
		t.Fatalf("ListItems publish_time_desc error: %v", err)
	}
	if len(publishTimeList.Items) != 3 || publishTimeList.Items[0].SourceKey != relativeLatest.SourceKey {
		t.Fatalf("expected publish_time_desc to prefer latest published item, got %+v", publishTimeList.Items)
	}
}

func TestListCrawlRunsAllowsNullErrorText(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	startedAt := time.Date(2026, 6, 22, 5, 0, 0, 0, time.UTC).Format(time.RFC3339)

	if _, err := store.db.ExecContext(ctx, `
INSERT INTO crawl_runs (source_type, template_id, template_name, template_snapshot, started_at, status, fetched_count, inserted_count, updated_count, error_text)
VALUES (?, 0, '', '', ?, 'failed', 0, 0, 0, NULL)`, "jin10_full", startedAt); err != nil {
		t.Fatalf("insert crawl run error: %v", err)
	}

	runs, err := store.ListCrawlRuns(ctx, 10, "jin10_full")
	if err != nil {
		t.Fatalf("ListCrawlRuns should allow NULL error_text, got %v", err)
	}
	if len(runs) != 1 || runs[0].SourceType != "jin10_full" || runs[0].ErrorText != "" {
		t.Fatalf("unexpected crawl runs: %+v", runs)
	}
}

func TestFailRunningCrawlRunsMarksInterrupted(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	startedAt := time.Date(2026, 6, 26, 1, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 26, 1, 5, 0, 0, time.UTC)

	runningID, err := store.StartCrawlRun(ctx, "jin10_full", startedAt)
	if err != nil {
		t.Fatalf("StartCrawlRun running error: %v", err)
	}
	successID, err := store.StartCrawlRun(ctx, "flash", startedAt)
	if err != nil {
		t.Fatalf("StartCrawlRun success error: %v", err)
	}
	if err := store.FinishCrawlRun(ctx, successID, "success", 1, 1, 0, "", startedAt.Add(time.Minute)); err != nil {
		t.Fatalf("FinishCrawlRun success error: %v", err)
	}

	affected, err := store.FailRunningCrawlRuns(ctx, "interrupted by service restart", finishedAt)
	if err != nil {
		t.Fatalf("FailRunningCrawlRuns error: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected one affected running row, got %d", affected)
	}

	runs, err := store.ListCrawlRuns(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListCrawlRuns error: %v", err)
	}
	byID := map[int64]model.CrawlRun{}
	for _, run := range runs {
		byID[run.ID] = run
	}
	running := byID[runningID]
	if running.Status != "failed" || running.ErrorText != "interrupted by service restart" || running.FinishedAt == nil {
		t.Fatalf("expected running row to be marked failed, got %+v", running)
	}
	success := byID[successID]
	if success.Status != "success" || success.ErrorText != "" {
		t.Fatalf("expected success row to remain unchanged, got %+v", success)
	}
}

func TestFeedbackCreateAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.CreateFeedback(ctx, model.Feedback{UserID: 7, Title: "第一个建议", Content: "先提交的内容"})
	if err != nil {
		t.Fatalf("CreateFeedback first error: %v", err)
	}
	second, err := store.CreateFeedback(ctx, model.Feedback{UserID: 8, Title: "第二个建议", Content: "后提交的内容"})
	if err != nil {
		t.Fatalf("CreateFeedback second error: %v", err)
	}
	if first.ID <= 0 || second.ID <= first.ID {
		t.Fatalf("expected increasing feedback ids, got first=%+v second=%+v", first, second)
	}

	items, err := store.ListFeedback(ctx, 1)
	if err != nil {
		t.Fatalf("ListFeedback error: %v", err)
	}
	if len(items) != 1 || items[0].ID != second.ID || items[0].Title != "第二个建议" || items[0].UserID != 8 || items[0].CreatedAt.IsZero() {
		t.Fatalf("expected latest feedback first with limit applied, got %+v", items)
	}

	if err := store.DeleteFeedback(ctx, second.ID); err != nil {
		t.Fatalf("DeleteFeedback error: %v", err)
	}
	items, err = store.ListFeedback(ctx, 10)
	if err != nil {
		t.Fatalf("ListFeedback after delete error: %v", err)
	}
	if len(items) != 1 || items[0].ID != first.ID || items[0].Title != "第一个建议" {
		t.Fatalf("expected deleted feedback removed from list, got %+v", items)
	}
}

func TestAStockAuctionAmountsUpsertAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 6, 16, 1, 30, 0, 0, time.UTC)

	first, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-16", []model.AStockAuctionAmount{
		{Code: "002230", Name: "科大讯飞", AuctionPrice: 41.2, AuctionVolume: 123400, AuctionAmount: 5084080, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "600000", Name: "浦发银行", AuctionPrice: 8.8, AuctionVolume: 90000, AuctionAmount: 792000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "601398", Name: "工商银行", AuctionPrice: 5.2, AuctionVolume: 120000, AuctionAmount: 900000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "000001", Name: "平安银行", AuctionPrice: 12, AuctionVolume: 100000, AuctionAmount: 1200000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "300750", Name: "宁德时代", AuctionPrice: 210, AuctionVolume: 20000, AuctionAmount: 800000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "012322", Name: "012322", AuctionPrice: 1.1, AuctionVolume: 120000, AuctionAmount: 132000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "011631", Name: "基金测试", AuctionPrice: 1.2, AuctionVolume: 110000, AuctionAmount: 132000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "920118", Name: "太湖远大", AuctionPrice: 18, AuctionVolume: 10000, AuctionAmount: 700000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "831526", Name: "凯华材料", AuctionPrice: 15, AuctionVolume: 12000, AuctionAmount: 600000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
	}, false)
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts insert error: %v", err)
	}
	if first.Inserted != 5 || first.Updated != 0 {
		t.Fatalf("unexpected insert result: %+v", first)
	}

	second, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-16", []model.AStockAuctionAmount{
		{Code: "002230", Name: "科大讯飞", AuctionPrice: 42, AuctionVolume: 100000, AuctionAmount: 4200000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
	}, false)
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts update error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 1 {
		t.Fatalf("unexpected update result: %+v", second)
	}

	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-15", []model.AStockAuctionAmount{
		{Code: "000001", Name: "平安银行", AuctionPrice: 12, AuctionVolume: 10000, AuctionAmount: 120000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt.AddDate(0, 0, -1)},
	}, false); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts previous day error: %v", err)
	}

	latest, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts latest error: %v", err)
	}
	if latest.Date != "2026-06-16" || latest.LatestDate != "2026-06-16" || latest.Total != 5 || len(latest.Items) != 5 {
		t.Fatalf("unexpected latest auction list: %+v", latest)
	}
	if latest.CaptureSlot != "0929" || latest.Items[0].CaptureSlot != "0929" {
		t.Fatalf("expected default 0929 capture slot, got %+v", latest)
	}
	if latest.Items[0].Code != "002230" || latest.Items[0].AuctionAmount != 4200000 || latest.TotalAmount != 7892000 {
		t.Fatalf("expected updated highest amount row and total amount, got %+v", latest)
	}
	if latest.MaxItem == nil || latest.MaxItem.Code != "002230" {
		t.Fatalf("expected max item, got %+v", latest.MaxItem)
	}
	if len(latest.Trend) != 2 || latest.Trend[0].Date != "2026-06-15" || latest.Trend[1].Date != "2026-06-16" || latest.Trend[1].TotalVolume != 430000 {
		t.Fatalf("expected two-day auction trend, got %+v", latest.Trend)
	}
	if len(latest.TrendSeries["0929"]) != 2 {
		t.Fatalf("expected 0929 trend series, got %+v", latest.TrendSeries)
	}
	marketTop := func(market string) []model.AStockAuctionAmount {
		for _, group := range latest.Trend[1].MarketTop {
			if group.Market == market {
				return group.Items
			}
		}
		return nil
	}
	if got := marketTop("沪市"); len(got) != 2 || got[0].Code != "601398" || got[1].Code != "600000" {
		t.Fatalf("expected Shanghai market top stocks by auction amount, got %+v", got)
	}
	if got := marketTop("深市"); len(got) != 3 || got[0].Code != "002230" || got[1].Code != "000001" || got[2].Code != "300750" {
		t.Fatalf("expected Shenzhen market top 3 stocks by auction amount, got %+v", got)
	}
	if got := marketTop("北交所"); got != nil {
		t.Fatalf("expected Beijing market top stocks to be excluded, got %+v", got)
	}

	filtered, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-06-16", Keyword: "浦发", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts filtered error: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].Code != "600000" {
		t.Fatalf("expected keyword filtered row, got %+v", filtered)
	}

	replaced, err := store.UpsertAStockAuctionAmounts(ctx, "2026-06-16", []model.AStockAuctionAmount{
		{Code: "300750", Name: "宁德时代", AuctionPrice: 215, AuctionVolume: 30000, AuctionAmount: 6450000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt.Add(5 * time.Minute)},
		{Code: "000001", Name: "平安银行", AuctionPrice: 11.8, AuctionVolume: 110000, AuctionAmount: 1298000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt.Add(5 * time.Minute)},
	}, true)
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts replace error: %v", err)
	}
	if replaced.Inserted != 2 || replaced.Updated != 0 {
		t.Fatalf("unexpected replace result: %+v", replaced)
	}
	replacedList, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-06-16", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts replaced error: %v", err)
	}
	if replacedList.Total != 2 || len(replacedList.Items) != 2 || replacedList.Items[0].Code != "300750" {
		t.Fatalf("expected replace snapshot to discard stale rows, got %+v", replacedList)
	}
}

func TestAStockAuctionAmountsKeepCaptureSlots(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 14, 1, 25, 5, 0, time.UTC)

	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-14", []model.AStockAuctionAmount{
		{CaptureSlot: "0920", Code: "002230", Name: "科大讯飞", AuctionVolume: 9000, AuctionAmount: 900000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt.Add(-5 * time.Minute)},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts 0920 error: %v", err)
	}
	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-14", []model.AStockAuctionAmount{
		{CaptureSlot: "0925", Code: "002230", Name: "科大讯飞", AuctionVolume: 10000, AuctionAmount: 1000000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts 0925 error: %v", err)
	}
	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-14", []model.AStockAuctionAmount{
		{CaptureSlot: "0929", Code: "002230", Name: "科大讯飞", AuctionVolume: 20000, AuctionAmount: 2000000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt.Add(4*time.Minute + 54*time.Second)},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts 0929 error: %v", err)
	}
	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-14", []model.AStockAuctionAmount{
		{CaptureSlot: "0925", Code: "600000", Name: "浦发银行", AuctionVolume: 30000, AuctionAmount: 3000000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts replace 0925 error: %v", err)
	}

	defaultList, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-14", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts default error: %v", err)
	}
	if defaultList.CaptureSlot != "0929" || defaultList.Total != 1 || defaultList.Items[0].Code != "002230" || defaultList.TotalAmount != 2000000 {
		t.Fatalf("expected default 0929 snapshot to remain intact, got %+v", defaultList)
	}
	slot0925, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-14", CaptureSlot: "0925", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts 0925 error: %v", err)
	}
	if slot0925.CaptureSlot != "0925" || slot0925.Total != 1 || slot0925.Items[0].Code != "600000" || slot0925.TotalAmount != 3000000 {
		t.Fatalf("expected replaced 0925 snapshot only, got %+v", slot0925)
	}
	if len(slot0925.Trend) != 1 || slot0925.Trend[0].CaptureSlot != "0929" || slot0925.Trend[0].TotalAmount != 2000000 {
		t.Fatalf("expected 0925 detail query to keep 0929 history trend, got %+v", slot0925.Trend)
	}
	if len(defaultList.TrendSeries["0920"]) != 1 || len(defaultList.TrendSeries["0925"]) != 1 || len(defaultList.TrendSeries["0929"]) != 1 {
		t.Fatalf("expected three current trend series, got %+v", defaultList.TrendSeries)
	}
}

func TestAStockAuctionAmountsDefaultFallsBackToLegacy0930(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 14, 1, 30, 5, 0, time.UTC)

	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-14", []model.AStockAuctionAmount{
		{CaptureSlot: "0930", Code: "002230", Name: "科大讯飞", AuctionVolume: 20000, AuctionAmount: 2000000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts legacy 0930 error: %v", err)
	}
	list, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-14", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts legacy fallback error: %v", err)
	}
	if list.CaptureSlot != "0930" || list.Total != 1 || list.Items[0].CaptureSlot != "0930" {
		t.Fatalf("expected default query to fall back to legacy 0930, got %+v", list)
	}
}

func TestAStockAuctionAmountsFinalTrendMergesLegacy0930(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 15, 1, 30, 5, 0, time.UTC)

	for _, snapshot := range []struct {
		date   string
		slot   string
		code   string
		amount float64
	}{
		{date: "2026-07-13", slot: "0930", code: "600000", amount: 1300000},
		{date: "2026-07-14", slot: "0930", code: "600000", amount: 1400000},
		{date: "2026-07-14", slot: "0929", code: "002230", amount: 1429000},
		{date: "2026-07-15", slot: "0929", code: "002230", amount: 1529000},
	} {
		if _, err := store.UpsertAStockAuctionAmounts(ctx, snapshot.date, []model.AStockAuctionAmount{
			{CaptureSlot: snapshot.slot, Code: snapshot.code, Name: "测试股份", AuctionVolume: 10000, AuctionAmount: snapshot.amount, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
		}, true); err != nil {
			t.Fatalf("UpsertAStockAuctionAmounts %s %s error: %v", snapshot.date, snapshot.slot, err)
		}
	}

	list, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-15", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts merged final trend error: %v", err)
	}
	final := list.TrendSeries["0929"]
	if len(final) != 3 {
		t.Fatalf("expected merged final 0929 trend to include legacy and current dates, got %+v", final)
	}
	assertFinal := func(i int, date string, slot string, amount float64) {
		t.Helper()
		if final[i].Date != date || final[i].CaptureSlot != slot || final[i].TotalAmount != amount {
			t.Fatalf("unexpected merged final point %d: got %+v want date=%s slot=%s amount=%.0f", i, final[i], date, slot, amount)
		}
	}
	assertFinal(0, "2026-07-13", "0930", 1300000)
	assertFinal(1, "2026-07-14", "0929", 1429000)
	assertFinal(2, "2026-07-15", "0929", 1529000)
	if len(list.Trend) != 3 || list.Trend[0].CaptureSlot != "0930" || list.Trend[1].CaptureSlot != "0929" {
		t.Fatalf("expected Trend to use merged final snapshots, got %+v", list.Trend)
	}
	if rawLegacy := list.TrendSeries["0930"]; len(rawLegacy) != 2 {
		t.Fatalf("expected raw legacy 0930 series to remain available, got %+v", rawLegacy)
	}
}

func TestAStockAuctionAmountsDefaultTrendWindowIsTwoWeeks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 6, 15, 1, 30, 5, 0, time.UTC)

	for day := 1; day <= 15; day++ {
		date := fmt.Sprintf("2026-06-%02d", day)
		if _, err := store.UpsertAStockAuctionAmounts(ctx, date, []model.AStockAuctionAmount{{
			Code:          "600000",
			Name:          "浦发银行",
			AuctionVolume: float64(day * 10000),
			AuctionAmount: float64(day * 1000000),
			Source:        "eastmoney_clist",
			Status:        "ok",
			FetchedAt:     fetchedAt.AddDate(0, 0, day-15),
		}}, true); err != nil {
			t.Fatalf("UpsertAStockAuctionAmounts %s error: %v", date, err)
		}
	}

	defaultList, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-06-15", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts default trend error: %v", err)
	}
	if len(defaultList.Trend) != 14 || defaultList.Trend[0].Date != "2026-06-02" || defaultList.Trend[13].Date != "2026-06-15" {
		t.Fatalf("expected default two-week auction trend, got %+v", defaultList.Trend)
	}

	sevenDayList, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-06-15", Page: 1, PageSize: 10, TrendDays: 7})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts explicit 7-day trend error: %v", err)
	}
	if len(sevenDayList.Trend) != 7 || sevenDayList.Trend[0].Date != "2026-06-09" || sevenDayList.Trend[6].Date != "2026-06-15" {
		t.Fatalf("expected explicit 7-day auction trend, got %+v", sevenDayList.Trend)
	}
}

func TestAStockAuctionAmountsRequested0929FallsBackToLegacy0930Only(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 13, 1, 30, 5, 0, time.UTC)

	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-13", []model.AStockAuctionAmount{
		{CaptureSlot: "0930", Code: "600000", Name: "浦发银行", AuctionVolume: 20000, AuctionAmount: 2000000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts legacy 0930 error: %v", err)
	}

	finalList, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-13", CaptureSlot: "0929", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts requested 0929 error: %v", err)
	}
	if finalList.CaptureSlot != "0930" || finalList.Total != 1 || len(finalList.Items) != 1 || finalList.Items[0].CaptureSlot != "0930" {
		t.Fatalf("expected requested 0929 detail to fall back to legacy 0930, got %+v", finalList)
	}

	slot0925, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-13", CaptureSlot: "0925", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts requested 0925 error: %v", err)
	}
	if slot0925.CaptureSlot != "0925" || slot0925.Total != 0 || len(slot0925.Items) != 0 {
		t.Fatalf("expected requested 0925 to avoid legacy fallback, got %+v", slot0925)
	}

	slot0920, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-13", CaptureSlot: "0920", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts requested 0920 error: %v", err)
	}
	if slot0920.CaptureSlot != "0920" || slot0920.Total != 0 || len(slot0920.Items) != 0 {
		t.Fatalf("expected requested 0920 to avoid legacy fallback, got %+v", slot0920)
	}
}

func TestAStockAuctionAmountsDefaultFallsBackTo0925(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 15, 1, 25, 5, 0, time.UTC)
	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-15", []model.AStockAuctionAmount{
		{CaptureSlot: "0925", Code: "002230", Name: "科大讯飞", AuctionVolume: 10000, AuctionAmount: 1000000, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
	}, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts 0925 error: %v", err)
	}
	list, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-15", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts fallback error: %v", err)
	}
	if list.CaptureSlot != "0925" || list.Total != 1 || len(list.Items) != 1 || list.Items[0].CaptureSlot != "0925" {
		t.Fatalf("expected default query to fall back to 0925, got %+v", list)
	}
	if len(list.Trend) != 0 {
		t.Fatalf("expected history trend to stay empty without 0930 data, got %+v", list.Trend)
	}
}

func TestAStockAuctionAmountsResolveNamesFromCodeDictionary(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 3, 1, 30, 0, 0, time.UTC)

	dictionary, err := store.UpsertAStockCodeNames(ctx, []model.AStockCodeName{
		{Code: "000034", Name: "神州数码", Source: "akshare_code_name", UpdatedAt: fetchedAt},
		{Code: "601995", Name: "中金公司", Source: "akshare_code_name", UpdatedAt: fetchedAt},
		{Code: "301696", Name: "三瑞智能", Source: "akshare_code_name", UpdatedAt: fetchedAt},
		{Code: "000001", Name: "金十数据整理", Source: "akshare_code_name", UpdatedAt: fetchedAt},
	})
	if err != nil {
		t.Fatalf("UpsertAStockCodeNames error: %v", err)
	}
	if dictionary.Inserted != 3 || dictionary.Updated != 0 {
		t.Fatalf("expected three valid dictionary rows, got %+v", dictionary)
	}

	names, err := store.ListAStockCodeNames(ctx, []string{"000034", "601995", "301696", "000001"})
	if err != nil {
		t.Fatalf("ListAStockCodeNames error: %v", err)
	}
	if names.Total != 3 {
		t.Fatalf("expected placeholder name to be excluded from dictionary, got %+v", names)
	}

	result, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-03", []model.AStockAuctionAmount{
		{Code: "000034", Name: "金十数据整理", AuctionPrice: 28.25, AuctionVolume: 10000, AuctionAmount: 282500, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "601995", Name: "中金", AuctionPrice: 36.38, AuctionVolume: 10000, AuctionAmount: 363800, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "301696", Name: "301696", AuctionPrice: 136.11, AuctionVolume: 10000, AuctionAmount: 1361100, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "002179", Name: "金十数据整理", AuctionPrice: 60, AuctionVolume: 10000, AuctionAmount: 600000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
	}, false)
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts with dictionary error: %v", err)
	}
	if result.Inserted != 3 || result.Updated != 0 {
		t.Fatalf("expected dictionary to repair three auction rows and skip unresolved placeholder, got %+v", result)
	}

	list, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-03", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts error: %v", err)
	}
	got := map[string]string{}
	for _, item := range list.Items {
		got[item.Code] = item.Name
	}
	for code, want := range map[string]string{"000034": "神州数码", "601995": "中金公司", "301696": "三瑞智能"} {
		if got[code] != want {
			t.Fatalf("expected %s name %s, got %q in %+v", code, want, got[code], list.Items)
		}
	}
	if _, exists := got["002179"]; exists {
		t.Fatalf("expected unresolved placeholder row to be skipped, got %+v", list.Items)
	}
}

func TestAStockAuctionAmountsNormalizeOverscaledSnapshotUnits(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 10, 1, 26, 0, 0, time.UTC)
	items := make([]model.AStockAuctionAmount, 0, 1001)
	items = append(items,
		model.AStockAuctionAmount{Code: "603986", Name: "兆易创新", AuctionPrice: 612, AuctionVolume: 887457, AuctionAmount: 59381239201, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
		model.AStockAuctionAmount{Code: "000725", Name: "京东方Ａ", AuctionPrice: 7.59, AuctionVolume: 38046199, AuctionAmount: 30277260673.71, Source: "eastmoney_clist", Status: "ok", FetchedAt: fetchedAt},
	)
	for i := 0; i < 999; i++ {
		items = append(items, model.AStockAuctionAmount{
			Code:          fmt.Sprintf("30%04d", i),
			Name:          fmt.Sprintf("测试股份%d", i),
			AuctionPrice:  10,
			AuctionVolume: 1400000,
			AuctionAmount: 3500000000,
			Source:        "eastmoney_clist",
			Status:        "ok",
			FetchedAt:     fetchedAt,
		})
	}

	if _, err := store.UpsertAStockAuctionAmounts(ctx, "2026-07-10", items, true); err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts oversized snapshot error: %v", err)
	}
	list, err := store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Date: "2026-07-10", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockAuctionAmounts oversized snapshot error: %v", err)
	}
	if list.MaxItem == nil || list.MaxItem.Code != "603986" {
		t.Fatalf("expected normalized max item, got %+v", list.MaxItem)
	}
	if list.MaxItem.AuctionVolume != 8874.57 || list.MaxItem.AuctionAmount != 593812392.01 {
		t.Fatalf("expected 100x unit normalization, got %+v", list.MaxItem)
	}
	if list.TotalAmount >= 1000000000000 {
		t.Fatalf("expected normalized daily total below overscaled threshold, got %.2f", list.TotalAmount)
	}
}

func TestAStockRecommendationSnapshotUpsertAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate:                "2026-06-22",
		Period:                      "afternoon",
		RecommendationsJSON:         `[{"Code":"600000"}]`,
		FilteredRecommendationsJSON: `[{"reason":"today_high_pct","recommendation":{"Code":"600010"}}]`,
		BacktestsJSON:               `[]`,
		NewsSummaryJSON:             `{"articles":[],"news_articles":[],"hotspots":[]}`,
		BacktestStatus:              "已回测 1/1",
		GeneratedCount:              1,
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSnapshot insert error: %v", err)
	}
	if first.Inserted != 1 || first.Updated != 0 {
		t.Fatalf("unexpected first upsert result: %+v", first)
	}

	second, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate:             "2026-06-22",
		Period:                   "afternoon",
		RecommendationsJSON:      `[{"Code":"000001"}]`,
		BacktestsJSON:            `null`,
		NewsSummaryJSON:          `{"articles":[{"id":1}],"news_articles":[{"id":1}],"hotspots":[{"Name":"人工智能"}]}`,
		BacktestStatus:           "已回测 1/1",
		GeneratedCount:           1,
		LimitUpFilterEnabled:     true,
		LimitUpFiltered:          2,
		TodayMarketFilterEnabled: true,
		NoTodayMarketCount:       4,
		FundFlowFilterEnabled:    true,
		FundFlowFiltered:         3,
		FundFlowMissingCount:     1,
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSnapshot update error: %v", err)
	}
	if second.Inserted != 1 || second.Updated != 0 {
		t.Fatalf("unexpected second upsert result: %+v", second)
	}

	defaultSnapshot, found, err := store.GetAStockRecommendationSnapshotWithFilter(ctx, "2026-06-22", "afternoon", model.AStockRecommendationSnapshotFilter{
		HasLimitUpFilterEnabled:     true,
		LimitUpFilterEnabled:        false,
		HasTodayMarketFilterEnabled: true,
		TodayMarketFilterEnabled:    false,
		HasFundFlowFilterEnabled:    true,
		FundFlowFilterEnabled:       false,
	})
	if err != nil {
		t.Fatalf("GetAStockRecommendationSnapshotWithFilter default error: %v", err)
	}
	if !found || defaultSnapshot.RecommendationsJSON != `[{"Code":"600000"}]` || !strings.Contains(defaultSnapshot.FilteredRecommendationsJSON, "600010") || defaultSnapshot.GeneratedCount != 1 || defaultSnapshot.LimitUpFilterEnabled || defaultSnapshot.TodayMarketFilterEnabled || defaultSnapshot.FundFlowFilterEnabled {
		t.Fatalf("unexpected default snapshot: found=%v %+v", found, defaultSnapshot)
	}

	snapshot, found, err := store.GetAStockRecommendationSnapshotWithFilter(ctx, "2026-06-22", "afternoon", model.AStockRecommendationSnapshotFilter{
		HasLimitUpFilterEnabled:     true,
		LimitUpFilterEnabled:        true,
		HasTodayMarketFilterEnabled: true,
		TodayMarketFilterEnabled:    true,
		HasFundFlowFilterEnabled:    true,
		FundFlowFilterEnabled:       true,
	})
	if err != nil {
		t.Fatalf("GetAStockRecommendationSnapshotWithFilter exact error: %v", err)
	}
	if !found || snapshot.RecommendationsJSON != `[{"Code":"000001"}]` || snapshot.FilteredRecommendationsJSON != `[]` || snapshot.BacktestsJSON != `[]` || !strings.Contains(snapshot.NewsSummaryJSON, `"人工智能"`) || snapshot.GeneratedCount != 1 || !snapshot.LimitUpFilterEnabled || snapshot.LimitUpFiltered != 2 || !snapshot.TodayMarketFilterEnabled || snapshot.NoTodayMarketCount != 4 || !snapshot.FundFlowFilterEnabled || snapshot.FundFlowFiltered != 3 || snapshot.FundFlowMissingCount != 1 {
		t.Fatalf("unexpected exact snapshot: found=%v %+v", found, snapshot)
	}
}

func TestAStockRecommendationPerformanceBuildsT1Metrics(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, ok := parseAStockPerformancePct("+1.23%"); !ok {
		t.Fatal("expected positive percent to parse")
	}
	if value, ok := parseAStockPerformancePct("0.00%"); !ok || value != 0 {
		t.Fatalf("expected zero percent to parse, got value=%v ok=%v", value, ok)
	}
	if value, ok := parseAStockPerformancePct("-0.01%"); !ok || value >= 0 {
		t.Fatalf("expected negative percent to parse, got value=%v ok=%v", value, ok)
	}
	if _, ok := parseAStockPerformancePct("--"); ok {
		t.Fatal("expected missing percent not to parse")
	}

	_, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate: "2026-06-22",
		Period:       "morning",
		RecommendationsJSON: `[
			{"Rank":1,"Hotspot":"人工智能","Code":"600001","Name":"胜率一号","MarketScore":260,"FundFlow5D":"+1.00亿","Change30":"+5.00%","Change60":"+8.00%"},
			{"Rank":4,"Hotspot":"人工智能","Code":"600002","Name":"持平二号","MarketScore":210,"FundFlow5D":"-1.00亿","Change30":"-11.00%","Change60":"+1.00%"},
			{"Rank":7,"Hotspot":"机器人","Code":"600003","Name":"未成熟三号","MarketScore":160,"FundFlow5D":"--","Change30":"--","Change60":"--"},
			{"Rank":8,"Hotspot":"机器人","Code":"600004","Name":"无回测四号","MarketScore":120,"FundFlow5D":"+0.00亿","Change30":"+1.00%","Change60":"+2.00%"}
		]`,
		BacktestsJSON: `[
			{"Stock":"600001 胜率一号","Days":[{"Return":"+1.23%"}]},
			{"Stock":"600002 持平二号","Days":[{"Return":"0.00%"}]},
			{"Stock":"600003 未成熟三号","Days":[{"Return":"--"}]}
		]`,
		BacktestStatus: "已回测",
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSnapshot error: %v", err)
	}

	summary, err := store.BuildAStockRecommendationPerformance(ctx, model.AStockRecommendationPerformanceFilter{
		StartDate: "2026-06-01",
		EndDate:   "2026-06-30",
		Period:    "all",
		Strategy:  "official",
	})
	if err != nil {
		t.Fatalf("BuildAStockRecommendationPerformance error: %v", err)
	}
	if summary.RecommendationCount != 4 || summary.SampleCount != 2 || summary.WinCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.WinRate != 0.5 || summary.AverageReturn < 0.61 || summary.AverageReturn > 0.62 || summary.RecommendationCover != 0.5 {
		t.Fatalf("unexpected summary metrics: %+v", summary)
	}
	groups := map[string]model.AStockRecommendationPerformanceGroup{}
	for _, group := range summary.Groups {
		groups[group.Dimension+"|"+group.Key] = group
	}
	if groups["period|morning"].SampleCount != 2 || groups["fund_flow|净流出"].SampleCount != 1 || groups["drawdown|回撤<-10%"].SampleCount != 1 {
		t.Fatalf("unexpected grouped metrics: %+v", summary.Groups)
	}
}

func TestAStockRecommendationPerformanceUsesShadowSnapshots(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	result, err := store.UpsertAStockRecommendationShadowSnapshot(ctx, model.AStockRecommendationShadowSnapshot{
		StrategyKey: "t1_shadow_v1",
		AStockRecommendationSnapshot: model.AStockRecommendationSnapshot{
			StrategyDate:        "2026-06-23",
			Period:              "afternoon",
			RecommendationsJSON: `[{"Rank":1,"Hotspot":"机器人","Code":"600010","Name":"影子一号","MarketScore":300,"FundFlow5D":"+2.00亿","Change30":"+6.00%","Change60":"+9.00%"}]`,
			BacktestsJSON:       `[{"Stock":"600010 影子一号","Days":[{"Return":"+2.00%"}]}]`,
			BacktestStatus:      "已回测",
			GeneratedCount:      1,
		},
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationShadowSnapshot error: %v", err)
	}
	if result.Inserted != 1 || result.Updated != 0 {
		t.Fatalf("unexpected shadow upsert result: %+v", result)
	}
	summary, err := store.BuildAStockRecommendationPerformance(ctx, model.AStockRecommendationPerformanceFilter{
		StartDate: "2026-06-01",
		EndDate:   "2026-06-30",
		Period:    "all",
		Strategy:  "t1_shadow_v1",
	})
	if err != nil {
		t.Fatalf("BuildAStockRecommendationPerformance shadow error: %v", err)
	}
	if summary.Strategy != "t1_shadow_v1" || summary.RecommendationCount != 1 || summary.SampleCount != 1 || summary.WinCount != 1 || summary.WinRate != 1 {
		t.Fatalf("unexpected shadow summary: %+v", summary)
	}
}

func TestAStockRecommendationSelectionsUpsertAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.UpsertAStockRecommendationSelections(ctx, model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-23",
		Period:       "afternoon",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 91, Reason: "first", EntryTime: "10:30"},
			{Rank: 2, Code: "688367", Name: "工大高科", Hotspot: "机器人", MarketScore: 87, Reason: "second", EntryTime: "13:01"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSelections insert error: %v", err)
	}
	if first.Inserted != 2 || first.Updated != 0 || first.Total != 2 {
		t.Fatalf("unexpected first selection upsert result: %+v", first)
	}

	second, err := store.UpsertAStockRecommendationSelections(ctx, model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-23",
		Period:       "afternoon",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 93, Reason: "kept", EntryTime: "10:31"},
			{Rank: 2, Code: "688367", Name: "工大高科", Hotspot: "机器人", MarketScore: 88, Reason: "kept-too", EntryTime: "13:01"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSelections update error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 2 || second.Total != 2 {
		t.Fatalf("unexpected second selection upsert result: %+v", second)
	}

	list, err := store.ListAStockRecommendationSelections(ctx, "2026-06-23", "afternoon")
	if err != nil {
		t.Fatalf("ListAStockRecommendationSelections error: %v", err)
	}
	if !list.Found || len(list.Items) != 2 {
		t.Fatalf("expected two persisted selections, got %+v", list)
	}
	if list.Items[0].Code != "002008" || list.Items[0].MarketScore != 93 || list.Items[0].EntryTime != "10:31" || list.Items[1].Code != "688367" || list.Items[1].EntryTime != "13:01" {
		t.Fatalf("unexpected persisted selections: %+v", list.Items)
	}
}

func TestAStockRecommendationLatestDatesUseSelectionsAndSnapshots(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.UpsertAStockRecommendationSelections(ctx, model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-23",
		Period:       "morning",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "688367", Name: "工大高科", Hotspot: "机器人"},
		},
	}); err != nil {
		t.Fatalf("upsert old morning selections: %v", err)
	}
	if _, err := store.UpsertAStockRecommendationSelections(ctx, model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-24",
		Period:       "afternoon",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人"},
		},
	}); err != nil {
		t.Fatalf("upsert old afternoon selections: %v", err)
	}
	if _, err := store.UpsertAStockRecommendationSelections(ctx, model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-25",
		Period:       "morning",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "688367", Name: "工大高科", Hotspot: "机器人"},
		},
	}); err != nil {
		t.Fatalf("upsert same-day morning selections: %v", err)
	}
	if _, err := store.UpsertAStockRecommendationSelections(ctx, model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-25",
		Period:       "afternoon",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "600030", Name: "中信证券", Hotspot: "金融券商"},
		},
	}); err != nil {
		t.Fatalf("upsert current afternoon selections: %v", err)
	}
	if _, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate:        "2026-06-24",
		Period:              "morning",
		RecommendationsJSON: `[{"Rank":1,"Code":"300024","Name":"机器人"}]`,
	}); err != nil {
		t.Fatalf("upsert old snapshot: %v", err)
	}
	if _, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate:        "2026-06-25",
		Period:              "afternoon",
		RecommendationsJSON: `[{"Rank":1,"Code":"300024","Name":"机器人"},{"Rank":2,"Code":"600030","Name":"中信证券"}]`,
	}); err != nil {
		t.Fatalf("upsert current snapshot: %v", err)
	}

	afternoon, err := store.ListAStockRecommendationLatestDates(ctx, "2026-06-25", "afternoon", []string{"002008", "688367", "600030", "300024", "999999"})
	if err != nil {
		t.Fatalf("ListAStockRecommendationLatestDates afternoon error: %v", err)
	}
	afternoonDates := aStockLatestDateTestMap(afternoon.Items)
	if afternoonDates["002008"] != "2026-06-24" || afternoonDates["688367"] != "2026-06-25" || afternoonDates["300024"] != "2026-06-24" {
		t.Fatalf("unexpected afternoon latest dates: %+v", afternoon.Items)
	}
	if _, ok := afternoonDates["600030"]; ok {
		t.Fatalf("expected current afternoon recommendation to be excluded, got %+v", afternoon.Items)
	}
	if _, ok := afternoonDates["999999"]; ok {
		t.Fatalf("expected missing code to be omitted, got %+v", afternoon.Items)
	}

	morning, err := store.ListAStockRecommendationLatestDates(ctx, "2026-06-25", "morning", []string{"688367", "600030"})
	if err != nil {
		t.Fatalf("ListAStockRecommendationLatestDates morning error: %v", err)
	}
	morningDates := aStockLatestDateTestMap(morning.Items)
	if morningDates["688367"] != "2026-06-23" {
		t.Fatalf("expected morning query to exclude same-day morning recommendation, got %+v", morning.Items)
	}
	if _, ok := morningDates["600030"]; ok {
		t.Fatalf("expected morning query to exclude same-day afternoon recommendation, got %+v", morning.Items)
	}
}

func aStockLatestDateTestMap(items []model.AStockRecommendationLatestDate) map[string]string {
	result := make(map[string]string, len(items))
	for _, item := range items {
		result[item.Code] = item.LatestDate
	}
	return result
}

func TestStockResearchSurveysUpsertAndFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{
		{Code: "002230", Name: "科大讯飞", Kind: "report", Title: "科大讯飞深度研究", Institution: "中金公司", Analyst: "张三", Rating: "买入", TargetPrice: "50.00", ResearchDate: "2026-06-16", SourceType: "sina_finance_report", SourceKey: "sina-1", SourceURL: "https://sina.example.com/1", SourceText: "已入库正文", SourceFetchStatus: "parsed", SourceFetchedAt: "2026-06-16T01:00:00Z", RawPayload: "{}"},
		{Code: "300059", Name: "东方财富", Kind: "survey", Title: "东方财富机构调研", Institution: "华泰证券", ResearchDate: "2026-06-15", SourceType: "sohu_finance_report", SourceKey: "sohu-1", RawPayload: "{}"},
	})
	if err != nil {
		t.Fatalf("UpsertStockResearchSurveys insert error: %v", err)
	}
	if first.Inserted != 2 || first.Updated != 0 {
		t.Fatalf("unexpected insert result: %+v", first)
	}
	second, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{
		{Code: "002230", Name: "科大讯飞", Kind: "report", Title: "科大讯飞深度研究更新", Institution: "中金公司", ResearchDate: "2026-06-16", SourceType: "sina_finance_report", SourceKey: "sina-1", RawPayload: "{}"},
	})
	if err != nil {
		t.Fatalf("UpsertStockResearchSurveys update error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 1 {
		t.Fatalf("unexpected update result: %+v", second)
	}

	list, err := store.ListStockResearchSurveys(ctx, model.StockResearchFilter{Code: "002230", Institution: "中金", Source: "sina_finance_report", Start: "2026-06-01", End: "2026-06-30", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListStockResearchSurveys error: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Title != "科大讯飞深度研究更新" {
		t.Fatalf("unexpected stock research list: %+v", list)
	}
	if list.Items[0].SourceText != "已入库正文" || list.Items[0].SourceFetchStatus != "parsed" {
		t.Fatalf("expected source fields to be stored and listed, got %+v", list.Items[0])
	}
	if len(list.Sources) != 2 {
		t.Fatalf("expected source options, got %+v", list.Sources)
	}
	pdfUpdated, err := store.UpdateStockResearchPDF(ctx, list.Items[0].ID, model.StockResearchPDFUpdate{
		PDFURL:       "https://sina.example.com/1.pdf",
		PDFFilePath:  filepath.Join("data", "stock-research-pdfs", "sina_finance_report", "sina-1.pdf"),
		PDFStatus:    "parsed",
		PDFText:      "科大讯飞研报正文",
		PDFFetchedAt: "2026-06-16T01:00:00Z",
		PDFParsedAt:  "2026-06-16T01:01:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateStockResearchPDF error: %v", err)
	}
	if pdfUpdated.PDFStatus != "parsed" || !strings.Contains(pdfUpdated.PDFText, "研报正文") {
		t.Fatalf("unexpected pdf update result: %+v", pdfUpdated)
	}
	if _, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{
		{Code: "002230", Name: "科大讯飞", Kind: "report", Title: "科大讯飞深度研究再次更新", Institution: "中金公司", ResearchDate: "2026-06-16", SourceType: "sina_finance_report", SourceKey: "sina-1", RawPayload: "{}"},
	}); err != nil {
		t.Fatalf("UpsertStockResearchSurveys preserve pdf error: %v", err)
	}
	loaded, err := store.GetStockResearchSurvey(ctx, list.Items[0].ID)
	if err != nil {
		t.Fatalf("GetStockResearchSurvey error: %v", err)
	}
	if loaded.Title != "科大讯飞深度研究再次更新" || loaded.PDFStatus != "parsed" || loaded.PDFText != "科大讯飞研报正文" {
		t.Fatalf("expected normal upsert to preserve parsed PDF fields, got %+v", loaded)
	}
	if loaded.SourceText != "已入库正文" {
		t.Fatalf("expected normal upsert to preserve source text, got %+v", loaded)
	}
	skippedSource, err := store.UpdateStockResearchSource(ctx, list.Items[0].ID, model.StockResearchSourceUpdate{
		SourceText:        "新正文",
		SourceFetchStatus: "parsed",
		SourceFetchedAt:   "2026-06-16T02:00:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateStockResearchSource skip error: %v", err)
	}
	if skippedSource.SourceText != "已入库正文" {
		t.Fatalf("expected source update without force to skip existing text, got %+v", skippedSource)
	}
	forcedSource, err := store.UpdateStockResearchSource(ctx, list.Items[0].ID, model.StockResearchSourceUpdate{
		SourceText:        "新正文",
		SourceFetchStatus: "parsed",
		SourceFetchedAt:   "2026-06-16T02:00:00Z",
		Force:             true,
	})
	if err != nil {
		t.Fatalf("UpdateStockResearchSource force error: %v", err)
	}
	if forcedSource.SourceText != "新正文" || forcedSource.SourceFetchedAt != "2026-06-16T02:00:00Z" {
		t.Fatalf("expected forced source update to overwrite text, got %+v", forcedSource)
	}
	nlpUpdated, err := store.UpdateStockResearchNLP(ctx, list.Items[0].ID, model.StockResearchNLPUpdate{
		NLPScore:    82.5,
		NLPRating:   "积极",
		NLPReason:   "AI订单增长",
		NLPScoredAt: "2026-06-16T04:00:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateStockResearchNLP error: %v", err)
	}
	if nlpUpdated.NLPScore != 82.5 || nlpUpdated.NLPRating != "积极" || nlpUpdated.NLPReason != "AI订单增长" || nlpUpdated.NLPScoredAt != "2026-06-16T04:00:00Z" {
		t.Fatalf("expected nlp fields to update, got %+v", nlpUpdated)
	}
	if nlpUpdated.SourceText != "新正文" || nlpUpdated.PDFText != "科大讯飞研报正文" {
		t.Fatalf("expected nlp update to preserve source and pdf text, got %+v", nlpUpdated)
	}

	sohuList, err := store.ListStockResearchSurveys(ctx, model.StockResearchFilter{Code: "300059", Page: 1, PageSize: 10})
	if err != nil || len(sohuList.Items) != 1 {
		t.Fatalf("list sohu stock research error=%v list=%+v", err, sohuList)
	}
	pdfSynced, err := store.UpdateStockResearchPDF(ctx, sohuList.Items[0].ID, model.StockResearchPDFUpdate{
		PDFURL:      "https://sohu.example.com/1.pdf",
		PDFStatus:   "parsed",
		PDFText:     "PDF解析正文",
		PDFParsedAt: "2026-06-16T03:00:00Z",
	})
	if err != nil {
		t.Fatalf("UpdateStockResearchPDF source sync error: %v", err)
	}
	if pdfSynced.SourceText != "PDF解析正文" || pdfSynced.SourceFetchStatus != "parsed" || pdfSynced.SourceFetchedAt != "2026-06-16T03:00:00Z" {
		t.Fatalf("expected pdf update to sync source text, got %+v", pdfSynced)
	}
}

func TestStockResearchUpsertMergesEquivalentReportsPreferringEastMoney(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{{
		Code:         "603100",
		Name:         "川仪股份",
		Kind:         "report",
		Title:        "川仪股份(603100)：工业自动化仪表龙头，国产替代持续推进",
		Institution:  "国投证券股份有限公司",
		ResearchDate: "2026-06-21",
		SourceType:   "sina_finance_report",
		SourceKey:    "sina-603100",
		SourceURL:    "https://stock.finance.sina.com.cn/report/603100.html",
		RawPayload:   "{}",
	}}); err != nil {
		t.Fatalf("insert sina stock research: %v", err)
	}
	before, err := store.ListStockResearchSurveys(ctx, model.StockResearchFilter{Code: "603100", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list before eastmoney upsert: %v", err)
	}
	if before.Total != 1 {
		t.Fatalf("expected one sina row before merge, got %+v", before)
	}
	firstID := before.Items[0].ID

	if _, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{{
		Code:         "603100",
		Name:         "川仪股份",
		Kind:         "report",
		Title:        "工业自动化仪表龙头，国产替代持续推进",
		Institution:  "国投证券股份有限公司",
		ResearchDate: "2026-06-21",
		SourceType:   "eastmoney_report",
		SourceKey:    "AP202606211823706310",
		SourceURL:    "https://data.eastmoney.com/report/info/AP202606211823706310.html",
		PDFURL:       "https://pdf.dfcfw.com/pdf/H3_AP202606211823706310_1.pdf?1782037122000.pdf",
		PDFStatus:    "pending",
		RawPayload:   "{}",
	}}); err != nil {
		t.Fatalf("upsert equivalent eastmoney stock research: %v", err)
	}
	merged, err := store.ListStockResearchSurveys(ctx, model.StockResearchFilter{Code: "603100", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list after eastmoney merge: %v", err)
	}
	if merged.Total != 1 || len(merged.Items) != 1 {
		t.Fatalf("expected one merged row, got %+v", merged)
	}
	item := merged.Items[0]
	if item.ID != firstID || item.SourceType != "eastmoney_report" || item.SourceKey != "AP202606211823706310" || !strings.Contains(item.SourceURL, "data.eastmoney.com/report/info/AP202606211823706310.html") || !strings.Contains(item.PDFURL, "AP202606211823706310") {
		t.Fatalf("expected existing row upgraded to eastmoney with PDF, got %+v", item)
	}

	if _, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{{
		Code:         "603100",
		Name:         "川仪股份",
		Kind:         "report",
		Title:        "川仪股份(603100)：工业自动化仪表龙头，国产替代持续推进",
		Institution:  "国投证券股份有限公司",
		ResearchDate: "2026-06-21",
		SourceType:   "sina_finance_report",
		SourceKey:    "sina-603100-late",
		SourceURL:    "https://stock.finance.sina.com.cn/report/603100-late.html",
		RawPayload:   "{}",
	}}); err != nil {
		t.Fatalf("upsert late sina equivalent stock research: %v", err)
	}
	afterLateSina, err := store.ListStockResearchSurveys(ctx, model.StockResearchFilter{Code: "603100", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list after late sina upsert: %v", err)
	}
	if afterLateSina.Total != 1 || afterLateSina.Items[0].SourceType != "eastmoney_report" || afterLateSina.Items[0].SourceKey != "AP202606211823706310" {
		t.Fatalf("expected late sina equivalent not to downgrade eastmoney row, got %+v", afterLateSina)
	}
}

func TestStockInstitutionHoldingsUpsertListAndSummary(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 6, 17, 2, 35, 0, 0, time.UTC)

	first, err := store.UpsertStockInstitutionHoldings(ctx, []model.StockInstitutionHolding{
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", AnnounceDate: "2026-04-30", HolderName: "易方达基金", HolderType: "fund", HolderCode: "110001", FundCompany: "易方达基金管理有限公司", FundCode: "110001", DisclosureScope: "fund_quarterly", HolderRank: "1", Shares: 1000, SharesChange: 100, ChangeRatio: 10, FloatRatio: 1.5, MarketValue: 50000, SourceType: "stock_institute_hold_detail", SourceURL: "https://example.com/1", RawPayload: `{"id":1}`, FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "社保基金一一八组合", HolderType: "social_security", Shares: 2000, FloatRatio: 2.5, MarketValue: 100000, SourceType: "stock_gdfx_free_holding_detail_em", RawPayload: `{}`, FetchedAt: fetchedAt},
		{StockCode: "300059", StockName: "东方财富", ReportPeriod: "20260331", HolderName: "香港中央结算有限公司", HolderType: "institution", Shares: 3000, FloatRatio: 3, SourceType: "stock_gdfx_holding_detail_em", RawPayload: `{}`, FetchedAt: fetchedAt},
	})
	if err != nil {
		t.Fatalf("UpsertStockInstitutionHoldings insert error: %v", err)
	}
	if first.Inserted != 3 || first.Updated != 0 || first.Total != 3 {
		t.Fatalf("unexpected first upsert result: %+v", first)
	}

	second, err := store.UpsertStockInstitutionHoldings(ctx, []model.StockInstitutionHolding{
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "易方达基金", HolderType: "fund", HolderCode: "110001", FundCompany: "易方达基金管理有限公司", FundCode: "110001", Shares: 1500, FloatRatio: 1.8, MarketValue: 80000, SourceType: "stock_institute_hold_detail", RawPayload: `{"id":2}`, FetchedAt: fetchedAt},
	})
	if err != nil {
		t.Fatalf("UpsertStockInstitutionHoldings update error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 1 || second.Total != 1 {
		t.Fatalf("unexpected second upsert result: %+v", second)
	}

	list, err := store.ListStockInstitutionHoldings(ctx, model.StockInstitutionHoldingFilter{Code: "002230", Period: "20260331", HolderType: "fund", Holder: "易方达", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListStockInstitutionHoldings error: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Shares != 1500 || list.Items[0].HolderType != "fund" || list.Items[0].FundCompany != "易方达基金管理有限公司" {
		t.Fatalf("unexpected holdings list: %+v", list)
	}
	if len(list.Periods) != 1 || len(list.HolderTypes) != 3 || len(list.Sources) != 3 {
		t.Fatalf("expected distinct filters, got periods=%v types=%v sources=%v", list.Periods, list.HolderTypes, list.Sources)
	}

	summary, err := store.GetStockInstitutionHoldingSummary(ctx, "002230", "20260331")
	if err != nil {
		t.Fatalf("GetStockInstitutionHoldingSummary error: %v", err)
	}
	if summary.HolderCount != 2 || summary.FundCount != 1 || summary.FundCompanyCount != 1 || summary.HolderTypeCount != 2 {
		t.Fatalf("unexpected holder counts: %+v", summary)
	}
	if summary.TotalShares != 3500 || summary.TotalFloatRatio != 4.3 || summary.MaxHolderName != "社保基金一一八组合" {
		t.Fatalf("unexpected summary totals: %+v", summary)
	}

	reportResult, err := store.UpsertStockHoldingReportDocuments(ctx, []model.StockHoldingReportDocument{{
		SourceType:        "tiantian_fund_regular_report",
		SourceKey:         "110001-20260331",
		ReportPeriod:      "20260331",
		FundCode:          "110001",
		FundName:          "易方达蓝筹精选",
		FundCompany:       "易方达基金管理有限公司",
		AnnouncementTitle: "易方达蓝筹精选2026年第1季度报告",
		AnnouncementDate:  "2026-04-22",
		SourceURL:         "https://example.com/report",
		ParseStatus:       "indexed",
		RawPayload:        "{}",
		FetchedAt:         fetchedAt,
	}})
	if err != nil {
		t.Fatalf("UpsertStockHoldingReportDocuments error: %v", err)
	}
	if reportResult.Inserted != 1 || reportResult.Updated != 0 || reportResult.Total != 1 {
		t.Fatalf("unexpected report upsert result: %+v", reportResult)
	}
	reportList, err := store.ListStockHoldingReportDocuments(ctx, model.StockHoldingReportDocumentFilter{Period: "20260331", FundCompany: "易方达", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListStockHoldingReportDocuments error: %v", err)
	}
	if reportList.Total != 1 || len(reportList.Items) != 1 || reportList.Items[0].FundCode != "110001" || len(reportList.Periods) != 1 || len(reportList.Sources) != 1 {
		t.Fatalf("unexpected report list: %+v", reportList)
	}
}

func TestAStockSectorFundFlowsUpsertReplaceAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 1, 2, 30, 0, 0, time.UTC)

	first, err := store.UpsertAStockSectorFundFlows(ctx, "2026-07-01", []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-01", SectorType: "行业资金流", Indicator: "今日", Rank: 1, Name: "半导体", ChangePct: 2.5, MainNetInflow: 120000000, MainNetInflowPct: 4.2, TopStock: "中芯国际", SourceType: "akshare_sector_fund_flow", FetchedAt: fetchedAt},
		{TradeDate: "2026-07-01", SectorType: "行业资金流", Indicator: "今日", Rank: 2, Name: "证券", ChangePct: -1.2, MainNetInflow: -30000000, MainNetInflowPct: -1.1, TopStock: "中信证券", SourceType: "akshare_sector_fund_flow", FetchedAt: fetchedAt},
	}, true)
	if err != nil {
		t.Fatalf("UpsertAStockSectorFundFlows insert error: %v", err)
	}
	if first.Inserted != 2 || first.Updated != 0 || first.Total != 2 {
		t.Fatalf("unexpected first sector fund flow upsert result: %+v", first)
	}

	second, err := store.UpsertAStockSectorFundFlows(ctx, "2026-07-01", []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-01", SectorType: "行业资金流", Indicator: "今日", Rank: 1, Name: "机器人", ChangePct: 3.1, MainNetInflow: 210000000, MainNetInflowPct: 5.6, TopStock: "埃斯顿", SourceType: "akshare_sector_fund_flow", FetchedAt: fetchedAt.Add(time.Minute)},
	}, true)
	if err != nil {
		t.Fatalf("UpsertAStockSectorFundFlows replace error: %v", err)
	}
	if second.Inserted != 1 || second.Total != 1 {
		t.Fatalf("unexpected replacement upsert result: %+v", second)
	}

	if _, err := store.UpsertAStockSectorFundFlows(ctx, "2026-07-01", []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-01", SectorType: "概念资金流", Indicator: "5日", Rank: 1, Name: "人工智能", MainNetInflow: 300000000, SourceType: "akshare_sector_fund_flow", FetchedAt: fetchedAt},
	}, true); err != nil {
		t.Fatalf("UpsertAStockSectorFundFlows concept error: %v", err)
	}

	list, err := store.ListAStockSectorFundFlows(ctx, model.AStockSectorFundFlowFilter{Date: "2026-07-01", SectorType: "行业资金流", Indicator: "今日", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockSectorFundFlows error: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Name != "机器人" || list.Items[0].MainNetInflow != 210000000 {
		t.Fatalf("unexpected sector fund flow list: %+v", list)
	}
	if len(list.Dates) != 1 || list.LatestDate != "2026-07-01" || len(list.SectorTypes) < 2 || len(list.Indicators) < 3 {
		t.Fatalf("unexpected sector fund flow filter options: %+v", list)
	}

	keyword, err := store.ListAStockSectorFundFlows(ctx, model.AStockSectorFundFlowFilter{Date: "2026-07-01", SectorType: "概念资金流", Indicator: "5日", Keyword: "智能", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("keyword ListAStockSectorFundFlows error: %v", err)
	}
	if keyword.Total != 1 || keyword.Items[0].Name != "人工智能" {
		t.Fatalf("unexpected keyword sector fund flow list: %+v", keyword)
	}
}

func TestAStockSectorFundFlowIntradaySnapshotAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 14, 2, 30, 0, 0, time.UTC)

	if _, err := store.UpsertAStockSectorFundFlows(ctx, "2026-07-14", []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-14", SectorType: "概念资金流", Indicator: "今日", Rank: 1, Name: "创新药", MainNetInflow: 1000000000, TopStock: "药企A", SourceType: "average", FetchedAt: fetchedAt},
		{TradeDate: "2026-07-14", SectorType: "概念资金流", Indicator: "今日", Rank: 2, Name: "半导体", MainNetInflow: -2000000000, TopStock: "芯片A", SourceType: "average", FetchedAt: fetchedAt},
	}, true); err != nil {
		t.Fatalf("UpsertAStockSectorFundFlows first error: %v", err)
	}
	first, err := store.SnapshotAStockSectorFundFlowIntraday(ctx, "2026-07-14", "09:30", "概念资金流", "今日")
	if err != nil {
		t.Fatalf("SnapshotAStockSectorFundFlowIntraday first error: %v", err)
	}
	if first.Inserted != 2 || first.Updated != 0 || first.Total != 2 {
		t.Fatalf("unexpected first intraday snapshot result: %+v", first)
	}

	if _, err := store.UpsertAStockSectorFundFlows(ctx, "2026-07-14", []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-14", SectorType: "概念资金流", Indicator: "今日", Rank: 1, Name: "创新药", MainNetInflow: 3000000000, TopStock: "药企A", SourceType: "average", FetchedAt: fetchedAt.Add(time.Minute)},
		{TradeDate: "2026-07-14", SectorType: "概念资金流", Indicator: "今日", Rank: 2, Name: "半导体", MainNetInflow: -1000000000, TopStock: "芯片A", SourceType: "average", FetchedAt: fetchedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("UpsertAStockSectorFundFlows second error: %v", err)
	}
	second, err := store.SnapshotAStockSectorFundFlowIntraday(ctx, "2026-07-14", "09:30", "概念资金流", "今日")
	if err != nil {
		t.Fatalf("SnapshotAStockSectorFundFlowIntraday second error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 2 || second.Total != 2 {
		t.Fatalf("unexpected second intraday snapshot result: %+v", second)
	}
	if _, err := store.SnapshotAStockSectorFundFlowIntraday(ctx, "2026-07-14", "13:11", "概念资金流", "今日"); err != nil {
		t.Fatalf("SnapshotAStockSectorFundFlowIntraday 13:11 error: %v", err)
	}

	result, err := store.ListAStockSectorFundFlowIntraday(ctx, model.AStockSectorFundFlowIntradayFilter{Date: "2026-07-14", SectorType: "概念资金流", Indicator: "今日", Limit: 2})
	if err != nil {
		t.Fatalf("ListAStockSectorFundFlowIntraday error: %v", err)
	}
	if result.LatestTime != "13:11" || len(result.Times) != 2 || result.Times[0] != "09:30" || result.Times[1] != "13:11" {
		t.Fatalf("unexpected intraday times: %+v", result)
	}
	if result.Total != 2 || len(result.Top) != 2 || result.Top[0].Name != "创新药" || result.Top[0].MainNetInflow != 3000000000 {
		t.Fatalf("unexpected intraday top ranking: %+v", result)
	}
	if len(result.Series) != 2 || result.Series[0].Name != "创新药" || len(result.Series[0].Points) != 2 {
		t.Fatalf("unexpected intraday series: %+v", result.Series)
	}
}

func TestAStockSectorFundFlowSourceRowsAverageAndSourceFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 2, 2, 30, 0, 0, time.UTC)

	if _, err := store.UpsertAStockSectorFundFlowSourceRows(ctx, "2026-07-02", []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-02", SectorType: "行业资金流", Indicator: "今日", SourceType: "eastmoney", Rank: 1, Name: "电机", MainNetInflow: 100, LargeNetInflow: 50, TopStock: "电机A", FieldCountsJSON: `{"main_net_inflow":1,"large_net_inflow":1}`, FetchedAt: fetchedAt},
		{TradeDate: "2026-07-02", SectorType: "行业资金流", Indicator: "今日", SourceType: "ths", Rank: 2, Name: "电机", MainNetInflow: 200, LargeNetInflow: 0, TopStock: "电机B", FieldCountsJSON: `{"main_net_inflow":1}`, FetchedAt: fetchedAt.Add(time.Minute)},
		{TradeDate: "2026-07-02", SectorType: "行业资金流", Indicator: "今日", SourceType: "sina", Rank: 3, Name: "电机", MainNetInflow: 300, LargeNetInflow: 0, TopStock: "电机C；电机D", FieldCountsJSON: `{"main_net_inflow":1}`, FetchedAt: fetchedAt.Add(2 * time.Minute)},
	}, true); err != nil {
		t.Fatalf("UpsertAStockSectorFundFlowSourceRows error: %v", err)
	}

	average, err := store.ListAStockSectorFundFlows(ctx, model.AStockSectorFundFlowFilter{Date: "2026-07-02", SectorType: "行业资金流", Indicator: "今日", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockSectorFundFlows average error: %v", err)
	}
	if average.Total != 1 || len(average.Items) != 1 {
		t.Fatalf("unexpected average sector list: %+v", average)
	}
	item := average.Items[0]
	if item.MainNetInflow != 200 || item.LargeNetInflow != 50 || item.SourceCount != 3 || item.SourceTypes != "eastmoney,sina,ths" || item.TopStock != "电机C、电机D、电机B" {
		t.Fatalf("unexpected averaged sector item: %+v", item)
	}
	counts := parseAStockFundFlowFieldCounts(item.FieldCountsJSON)
	if counts["main_net_inflow"] != 3 || counts["large_net_inflow"] != 1 {
		t.Fatalf("unexpected averaged sector field counts: %s", item.FieldCountsJSON)
	}

	sourceRows, err := store.ListAStockSectorFundFlows(ctx, model.AStockSectorFundFlowFilter{Date: "2026-07-02", SectorType: "行业资金流", Indicator: "今日", SourceType: "ths", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockSectorFundFlows source error: %v", err)
	}
	if sourceRows.Total != 1 || sourceRows.Items[0].SourceType != "ths" || sourceRows.Items[0].MainNetInflow != 200 {
		t.Fatalf("unexpected source sector rows: %+v", sourceRows)
	}
}

func TestAStockStockFundFlowSourceRowsAverageAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 2, 2, 35, 0, 0, time.UTC)

	if _, err := store.UpsertAStockStockFundFlowSourceRows(ctx, "2026-07-02", []model.AStockStockFundFlow{
		{TradeDate: "2026-07-02", Indicator: "今日", SourceType: "eastmoney", Rank: 1, Code: "sz300502", Name: "新易盛", Price: 500, MainNetInflow: 100, SuperLargeNetInflow: 40, FieldCountsJSON: `{"price":1,"main_net_inflow":1,"super_large_net_inflow":1}`, FetchedAt: fetchedAt},
		{TradeDate: "2026-07-02", Indicator: "今日", SourceType: "sina", Rank: 2, Code: "300502", Name: "新易盛", Price: 520, MainNetInflow: 200, SuperLargeNetInflow: 0, FieldCountsJSON: `{"price":1,"main_net_inflow":1}`, FetchedAt: fetchedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("UpsertAStockStockFundFlowSourceRows error: %v", err)
	}

	average, err := store.ListAStockStockFundFlows(ctx, model.AStockStockFundFlowFilter{Date: "2026-07-02", Indicator: "今日", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockStockFundFlows average error: %v", err)
	}
	if average.Total != 1 || len(average.Items) != 1 {
		t.Fatalf("unexpected stock fund flow list: %+v", average)
	}
	item := average.Items[0]
	if item.Code != "300502" || item.Price != 510 || item.MainNetInflow != 150 || item.SuperLargeNetInflow != 40 || item.SourceCount != 2 || item.SourceTypes != "eastmoney,sina" {
		t.Fatalf("unexpected averaged stock item: %+v", item)
	}
	counts := parseAStockFundFlowFieldCounts(item.FieldCountsJSON)
	if counts["price"] != 2 || counts["main_net_inflow"] != 2 || counts["super_large_net_inflow"] != 1 {
		t.Fatalf("unexpected averaged stock field counts: %s", item.FieldCountsJSON)
	}

	sourceRows, err := store.ListAStockStockFundFlows(ctx, model.AStockStockFundFlowFilter{Date: "2026-07-02", Indicator: "今日", SourceType: "sina", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockStockFundFlows source error: %v", err)
	}
	if sourceRows.Total != 1 || sourceRows.Items[0].Code != "300502" || sourceRows.Items[0].SourceType != "sina" {
		t.Fatalf("unexpected source stock rows: %+v", sourceRows)
	}

	if _, err := store.UpsertAStockStockFundFlowSourceRows(ctx, "2026-07-02", []model.AStockStockFundFlow{
		{TradeDate: "2026-07-02", Indicator: "今日", SourceType: "eastmoney", Rank: 3, Code: "sh688981", Name: "中芯国际", Price: 88, MainNetInflow: 80, FieldCountsJSON: `{"price":1,"main_net_inflow":1}`, FetchedAt: fetchedAt},
	}, false); err != nil {
		t.Fatalf("UpsertAStockStockFundFlowSourceRows extra error: %v", err)
	}
	filtered, err := store.ListAStockStockFundFlows(ctx, model.AStockStockFundFlowFilter{Date: "2026-07-02", Indicator: "今日", Codes: []string{"sz300502", "688981", "688981"}, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockStockFundFlows code filter error: %v", err)
	}
	if filtered.Total != 2 || len(filtered.Items) != 2 {
		t.Fatalf("expected two code-filtered stock rows, got %+v", filtered)
	}
	sourceFiltered, err := store.ListAStockStockFundFlows(ctx, model.AStockStockFundFlowFilter{Date: "2026-07-02", Indicator: "今日", SourceType: "eastmoney", Codes: []string{"688981"}, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAStockStockFundFlows source code filter error: %v", err)
	}
	if sourceFiltered.Total != 1 || sourceFiltered.Items[0].Code != "688981" {
		t.Fatalf("expected one source-filtered stock row for 688981, got %+v", sourceFiltered)
	}
}

func TestAStockSectorConstituentsAndFundFlowTrends(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 1, 2, 35, 0, 0, time.UTC)

	first, err := store.UpsertAStockSectorConstituents(ctx, "行业资金流", "石油石化", []model.AStockSectorConstituent{
		{SectorType: "行业资金流", SectorName: "石油石化", Code: "600028", Name: "中国石化", Source: "eastmoney", FetchedAt: fetchedAt},
		{SectorType: "行业资金流", SectorName: "石油石化", Code: "601857", Name: "中国石油", Source: "eastmoney", FetchedAt: fetchedAt},
	}, true)
	if err != nil {
		t.Fatalf("UpsertAStockSectorConstituents insert error: %v", err)
	}
	if first.Inserted != 2 || first.Total != 2 {
		t.Fatalf("unexpected constituent insert result: %+v", first)
	}
	if _, err := store.UpsertAStockSectorConstituents(ctx, "行业资金流", "石油石化", []model.AStockSectorConstituent{
		{SectorType: "行业资金流", SectorName: "石油石化", Code: "600028", Name: "中国石化", Source: "cached", FetchedAt: fetchedAt.Add(time.Minute)},
	}, false); err != nil {
		t.Fatalf("UpsertAStockSectorConstituents update error: %v", err)
	}
	list, err := store.ListAStockSectorConstituents(ctx, model.AStockSectorConstituentFilter{SectorType: "行业资金流", SectorName: "石油石化", Keyword: "中国", Limit: 1})
	if err != nil {
		t.Fatalf("ListAStockSectorConstituents error: %v", err)
	}
	if list.Total != 2 || len(list.Items) != 1 || list.Items[0].Code != "600028" || list.Items[0].Source != "cached" {
		t.Fatalf("unexpected constituent list: %+v", list)
	}

	for idx, date := range []string{"2026-07-01", "2026-07-02", "2026-07-03"} {
		if _, err := store.UpsertAStockSectorFundFlows(ctx, date, []model.AStockSectorFundFlow{
			{TradeDate: date, SectorType: "行业资金流", Indicator: "今日", Rank: idx + 1, Name: "石油石化", MainNetInflow: float64(100 + idx), FetchedAt: fetchedAt.Add(time.Duration(idx) * time.Hour)},
			{TradeDate: date, SectorType: "概念资金流", Indicator: "今日", Rank: 1, Name: "石油概念", MainNetInflow: 999, FetchedAt: fetchedAt},
		}, true); err != nil {
			t.Fatalf("UpsertAStockSectorFundFlows %s error: %v", date, err)
		}
		if _, err := store.UpsertAStockStockFundFlows(ctx, date, []model.AStockStockFundFlow{
			{TradeDate: date, Indicator: "今日", Rank: idx + 1, Code: "600028", Name: "中国石化", MainNetInflow: float64(200 + idx), FetchedAt: fetchedAt.Add(time.Duration(idx) * time.Hour)},
			{TradeDate: date, Indicator: "今日", Rank: 2, Code: "601857", Name: "中国石油", MainNetInflow: 50, FetchedAt: fetchedAt},
		}, true); err != nil {
			t.Fatalf("UpsertAStockStockFundFlows %s error: %v", date, err)
		}
	}
	sectorTrend, err := store.ListAStockSectorFundFlowTrend(ctx, model.AStockFundFlowTrendFilter{EndDate: "2026-07-03", SectorType: "行业资金流", SectorName: "石油石化", Indicator: "今日", Days: 5})
	if err != nil {
		t.Fatalf("ListAStockSectorFundFlowTrend error: %v", err)
	}
	if sectorTrend.Total != 3 || sectorTrend.Items[0].TradeDate != "2026-07-03" || sectorTrend.Items[1].TradeDate != "2026-07-02" || sectorTrend.Items[0].MainNetInflow != 102 {
		t.Fatalf("unexpected sector trend: %+v", sectorTrend)
	}
	stockTrend, err := store.ListAStockStockFundFlowTrend(ctx, model.AStockFundFlowTrendFilter{EndDate: "2026-07-03", Code: "sh600028", Indicator: "今日", Days: 5})
	if err != nil {
		t.Fatalf("ListAStockStockFundFlowTrend error: %v", err)
	}
	if stockTrend.Total != 3 || stockTrend.Code != "600028" || stockTrend.Items[0].TradeDate != "2026-07-03" || stockTrend.Items[0].MainNetInflow != 202 {
		t.Fatalf("unexpected stock trend: %+v", stockTrend)
	}
}

func TestStockInstitutionHoldingSignalsComparePeriods(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 6, 17, 2, 35, 0, 0, time.UTC)
	items := []model.StockInstitutionHolding{
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20251231", HolderName: "易方达基金", HolderType: "fund", HolderCode: "old-1", Shares: 1000, FloatRatio: 1, MarketValue: 10000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "300059", StockName: "东方财富", ReportPeriod: "20251231", HolderName: "华夏基金", HolderType: "fund", HolderCode: "old-2", Shares: 1000, FloatRatio: 1, MarketValue: 10000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "易方达基金", HolderType: "fund", HolderCode: "old-1", Shares: 1200, FloatRatio: 1.2, MarketValue: 12000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "南方基金", HolderType: "fund", HolderCode: "new-1", Shares: 1000, FloatRatio: 0.8, MarketValue: 20000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "嘉实基金", HolderType: "fund", HolderCode: "new-2", Shares: 1000, FloatRatio: 0.8, MarketValue: 19000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "社保基金一一八组合", HolderType: "social_security", HolderCode: "new-3", Shares: 1000, FloatRatio: 1.1, MarketValue: 50000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "QFII Alpha", HolderType: "qfii", HolderCode: "new-4", Shares: 1000, FloatRatio: 0.7, MarketValue: 18000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "保险资管", HolderType: "insurance", HolderCode: "new-5", Shares: 1000, FloatRatio: 0.4, MarketValue: 17000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "300059", StockName: "东方财富", ReportPeriod: "20260331", HolderName: "华夏基金", HolderType: "fund", HolderCode: "old-2", Shares: 2000, FloatRatio: 4, MarketValue: 60000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "300059", StockName: "东方财富", ReportPeriod: "20260331", HolderName: "香港中央结算有限公司", HolderType: "institution", HolderCode: "new-6", Shares: 2000, FloatRatio: 3, MarketValue: 70000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
	}
	if _, err := store.UpsertStockInstitutionHoldings(ctx, items); err != nil {
		t.Fatalf("UpsertStockInstitutionHoldings error: %v", err)
	}

	signals, err := store.ListStockInstitutionHoldingSignals(ctx, model.StockInstitutionHoldingSignalFilter{Period: "20260331", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListStockInstitutionHoldingSignals error: %v", err)
	}
	if signals.Total != 2 || signals.CurrentPeriod != "20260331" || signals.PreviousPeriod != "20251231" {
		t.Fatalf("unexpected signal metadata: %+v", signals)
	}
	if signals.Items[0].StockCode != "300059" || signals.Items[0].Level != "high" || signals.Items[0].FloatRatioChange != 6 {
		t.Fatalf("expected high float-ratio signal first, got %+v", signals.Items[0])
	}
	second := signals.Items[1]
	if second.StockCode != "002230" || second.HolderCountChange != 5 || second.FundCountChange != 2 || second.FloatRatioChange != 4 {
		t.Fatalf("unexpected 002230 signal: %+v", second)
	}
	if len(second.NewMajorHolders) == 0 || second.NewMajorHolders[0] != "社保基金一一八组合" {
		t.Fatalf("expected new major holders sorted by market value, got %+v", second.NewMajorHolders)
	}

	filtered, err := store.ListStockInstitutionHoldingSignals(ctx, model.StockInstitutionHoldingSignalFilter{Code: "002230", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("filtered ListStockInstitutionHoldingSignals error: %v", err)
	}
	if filtered.Total != 1 || filtered.Items[0].StockCode != "002230" {
		t.Fatalf("expected one filtered signal for 002230, got %+v", filtered)
	}
}

func TestStockInstitutionHoldingSignalsDetectExitDisclosure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	fetchedAt := time.Date(2026, 7, 23, 2, 35, 0, 0, time.UTC)
	items := []model.StockInstitutionHolding{
		{StockCode: "600000", StockName: "浦发银行", ReportPeriod: "20260331", HolderName: "易方达基金", HolderType: "fund", HolderCode: "exit-1", FundCompany: "易方达基金管理有限公司", Shares: 1000, FloatRatio: 1, MarketValue: 10000, SourceType: "stock_fund_stock_holder", FetchedAt: fetchedAt},
		{StockCode: "600000", StockName: "浦发银行", ReportPeriod: "20260331", HolderName: "华夏基金", HolderType: "fund", HolderCode: "exit-2", FundCompany: "华夏基金管理有限公司", Shares: 900, FloatRatio: 0.9, MarketValue: 9000, SourceType: "stock_fund_stock_holder", FetchedAt: fetchedAt},
		{StockCode: "600000", StockName: "浦发银行", ReportPeriod: "20260331", HolderName: "南方基金", HolderType: "fund", HolderCode: "exit-3", FundCompany: "南方基金管理股份有限公司", Shares: 800, FloatRatio: 0.8, MarketValue: 8000, SourceType: "stock_fund_stock_holder", FetchedAt: fetchedAt},
		{StockCode: "600000", StockName: "浦发银行", ReportPeriod: "20260331", HolderName: "社保基金一一八组合", HolderType: "social_security", HolderCode: "exit-4", Shares: 700, FloatRatio: 0.7, MarketValue: 7000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "600000", StockName: "浦发银行", ReportPeriod: "20260331", HolderName: "QFII Alpha", HolderType: "qfii", HolderCode: "exit-5", Shares: 600, FloatRatio: 0.6, MarketValue: 6000, SourceType: "stock_institute_hold_detail", FetchedAt: fetchedAt},
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260630", HolderName: "易方达基金", HolderType: "fund", HolderCode: "keep-1", FundCompany: "易方达基金管理有限公司", Shares: 100, FloatRatio: 0.1, MarketValue: 1000, SourceType: "stock_fund_stock_holder", FetchedAt: fetchedAt},
	}
	if _, err := store.UpsertStockInstitutionHoldings(ctx, items); err != nil {
		t.Fatalf("UpsertStockInstitutionHoldings error: %v", err)
	}

	signals, err := store.ListStockInstitutionHoldingSignals(ctx, model.StockInstitutionHoldingSignalFilter{Period: "20260630", SignalType: "exit_disclosure", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListStockInstitutionHoldingSignals exit error: %v", err)
	}
	if signals.Total != 1 || signals.Items[0].StockCode != "600000" || signals.Items[0].SignalType != "exit_disclosure" {
		t.Fatalf("expected one exit disclosure signal for 600000, got %+v", signals)
	}
	signal := signals.Items[0]
	if signal.CurrentHolderCount != 0 || signal.PreviousHolderCount != 5 || signal.ExitedHolderCount != 5 || signal.ExitedFundCount != 3 || signal.ExitedFundCompanyCount != 3 {
		t.Fatalf("unexpected exit disclosure stats: %+v", signal)
	}
	if !strings.Contains(signal.Reason, "退出披露名单") || strings.Contains(signal.Reason, "清仓") {
		t.Fatalf("expected cautious exit disclosure reason, got %q", signal.Reason)
	}
	if len(signal.ExitedMajorHolders) == 0 || signal.ExitedMajorHolders[0] != "易方达基金" {
		t.Fatalf("expected exited holders sorted by market value, got %+v", signal.ExitedMajorHolders)
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

	defaultRelease, err := store.GetReleaseSettings(ctx)
	if err != nil {
		t.Fatalf("GetReleaseSettings error: %v", err)
	}
	if defaultRelease.ReleaseDir != `C:\yuqing\release` || defaultRelease.DevReleaseDir != `D:\yuqing\release` || defaultRelease.ServerSharePath != `\\10.15.0.7\yuqing-release` {
		t.Fatalf("unexpected default release settings: %+v", defaultRelease)
	}
	releaseSettings, err := store.UpsertReleaseSettings(ctx, model.ReleaseSettings{
		ReleaseAddr:     ":8100",
		ReleaseURL:      "http://10.15.0.7:8100/",
		ReleaseDir:      `C:\yuqing\release2`,
		DevReleaseDir:   `D:\yuqing\release2`,
		ServerSharePath: `\\10.15.0.7\yuqing-release2\`,
		ServerUser:      `10.15.0.7\hyuser`,
	})
	if err != nil {
		t.Fatalf("UpsertReleaseSettings error: %v", err)
	}
	if releaseSettings.ReleaseURL != "http://10.15.0.7:8100" || releaseSettings.ServerSharePath != `\\10.15.0.7\yuqing-release2` {
		t.Fatalf("unexpected release settings: %+v", releaseSettings)
	}

	defaultAlgorithm, err := store.GetAStockRecommendationAlgorithmSettings(ctx)
	if err != nil {
		t.Fatalf("GetAStockRecommendationAlgorithmSettings error: %v", err)
	}
	if defaultAlgorithm.Auction.RecommendationLimit != 5 || defaultAlgorithm.Fund.ExtremeScore != 120 || defaultAlgorithm.Emotion.FactorScoreCap != 300 || !defaultAlgorithm.Candidate.RequireHotspotLink || defaultAlgorithm.Auction.LowOpenStrongPenalty != 65 {
		t.Fatalf("unexpected default A stock algorithm settings: %+v", defaultAlgorithm)
	}
	updatedAlgorithm := defaultAlgorithm
	updatedAlgorithm.Auction.RecommendationLimit = 4
	updatedAlgorithm.Auction.LowOpenPenalty2 = 22
	updatedAlgorithm.Fund.ExtremeScore = 45
	savedAlgorithm, err := store.UpsertAStockRecommendationAlgorithmSettings(ctx, updatedAlgorithm)
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationAlgorithmSettings error: %v", err)
	}
	if savedAlgorithm.Auction.RecommendationLimit != 4 || savedAlgorithm.Auction.LowOpenPenalty2 != 22 || savedAlgorithm.Fund.ExtremeScore != 45 || savedAlgorithm.UpdatedAt.IsZero() {
		t.Fatalf("unexpected saved A stock algorithm settings: %+v", savedAlgorithm)
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

func assertItemTitleCount(t *testing.T, store *Store, ctx context.Context, title string, expected int) {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE title = ?`, title).Scan(&count); err != nil {
		t.Fatalf("count title %q error: %v", title, err)
	}
	if count != expected {
		t.Fatalf("expected title %q count %d, got %d", title, expected, count)
	}
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
