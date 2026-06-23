package sqlite

import (
	"context"
	"path/filepath"
	"strings"
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

	if _, _, err := store.UpsertItems(ctx, []model.Item{recrawledOld, latestPublished}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	defaultList, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListItems default sort error: %v", err)
	}
	if len(defaultList.Items) != 2 || defaultList.Items[0].SourceKey != recrawledOld.SourceKey {
		t.Fatalf("expected default captured_at sort to keep recrawled old item first, got %+v", defaultList.Items)
	}

	publishTimeList, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Sort: "publish_time_desc"})
	if err != nil {
		t.Fatalf("ListItems publish_time_desc error: %v", err)
	}
	if len(publishTimeList.Items) != 2 || publishTimeList.Items[0].SourceKey != latestPublished.SourceKey {
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
		{Code: "920118", Name: "太湖远大", AuctionPrice: 18, AuctionVolume: 10000, AuctionAmount: 700000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
		{Code: "831526", Name: "凯华材料", AuctionPrice: 15, AuctionVolume: 12000, AuctionAmount: 600000, Source: "akshare_pre_min", Status: "ok", FetchedAt: fetchedAt},
	})
	if err != nil {
		t.Fatalf("UpsertAStockAuctionAmounts insert error: %v", err)
	}
	if first.Inserted != 7 || first.Updated != 0 {
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
	if latest.Date != "2026-06-16" || latest.LatestDate != "2026-06-16" || latest.Total != 5 || len(latest.Items) != 5 {
		t.Fatalf("unexpected latest auction list: %+v", latest)
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
}

func TestAStockRecommendationSnapshotUpsertAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate:        "2026-06-22",
		Period:              "afternoon",
		RecommendationsJSON: `[{"Code":"600000"}]`,
		BacktestsJSON:       `[]`,
		BacktestStatus:      "已回测 1/1",
		GeneratedCount:      1,
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSnapshot insert error: %v", err)
	}
	if first.Inserted != 1 || first.Updated != 0 {
		t.Fatalf("unexpected first upsert result: %+v", first)
	}

	second, err := store.UpsertAStockRecommendationSnapshot(ctx, model.AStockRecommendationSnapshot{
		StrategyDate:         "2026-06-22",
		Period:               "afternoon",
		RecommendationsJSON:  `[{"Code":"000001"}]`,
		BacktestsJSON:        `[]`,
		BacktestStatus:       "已回测 1/1",
		GeneratedCount:       1,
		LimitUpFilterEnabled: true,
		LimitUpFiltered:      2,
	})
	if err != nil {
		t.Fatalf("UpsertAStockRecommendationSnapshot update error: %v", err)
	}
	if second.Inserted != 0 || second.Updated != 1 {
		t.Fatalf("unexpected second upsert result: %+v", second)
	}

	snapshot, found, err := store.GetAStockRecommendationSnapshot(ctx, "2026-06-22", "afternoon", false)
	if err != nil {
		t.Fatalf("GetAStockRecommendationSnapshot error: %v", err)
	}
	if !found || snapshot.RecommendationsJSON != `[{"Code":"000001"}]` || snapshot.GeneratedCount != 1 || !snapshot.LimitUpFilterEnabled || snapshot.LimitUpFiltered != 2 {
		t.Fatalf("unexpected snapshot: found=%v %+v", found, snapshot)
	}
}

func TestStockResearchSurveysUpsertAndFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.UpsertStockResearchSurveys(ctx, []model.StockResearchSurvey{
		{Code: "002230", Name: "科大讯飞", Kind: "report", Title: "科大讯飞深度研究", Institution: "中金公司", Analyst: "张三", Rating: "买入", TargetPrice: "50.00", ResearchDate: "2026-06-16", SourceType: "sina_finance_report", SourceKey: "sina-1", SourceURL: "https://sina.example.com/1", RawPayload: "{}"},
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
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", AnnounceDate: "2026-04-30", HolderName: "易方达基金", HolderType: "fund", HolderCode: "110001", HolderRank: "1", Shares: 1000, SharesChange: 100, ChangeRatio: 10, FloatRatio: 1.5, MarketValue: 50000, SourceType: "stock_institute_hold_detail", SourceURL: "https://example.com/1", RawPayload: `{"id":1}`, FetchedAt: fetchedAt},
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
		{StockCode: "002230", StockName: "科大讯飞", ReportPeriod: "20260331", HolderName: "易方达基金", HolderType: "fund", HolderCode: "110001", Shares: 1500, FloatRatio: 1.8, MarketValue: 80000, SourceType: "stock_institute_hold_detail", RawPayload: `{"id":2}`, FetchedAt: fetchedAt},
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
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Shares != 1500 || list.Items[0].HolderType != "fund" {
		t.Fatalf("unexpected holdings list: %+v", list)
	}
	if len(list.Periods) != 1 || len(list.HolderTypes) != 3 || len(list.Sources) != 3 {
		t.Fatalf("expected distinct filters, got periods=%v types=%v sources=%v", list.Periods, list.HolderTypes, list.Sources)
	}

	summary, err := store.GetStockInstitutionHoldingSummary(ctx, "002230", "20260331")
	if err != nil {
		t.Fatalf("GetStockInstitutionHoldingSummary error: %v", err)
	}
	if summary.HolderCount != 2 || summary.FundCount != 1 || summary.HolderTypeCount != 2 {
		t.Fatalf("unexpected holder counts: %+v", summary)
	}
	if summary.TotalShares != 3500 || summary.TotalFloatRatio != 4.3 || summary.MaxHolderName != "社保基金一一八组合" {
		t.Fatalf("unexpected summary totals: %+v", summary)
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
