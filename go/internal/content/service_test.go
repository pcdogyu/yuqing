package content

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

func TestArticleFilterFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=2&page_size=50&keyword=%20alpha%20&source_type=flash&project_id=12&user_id=99&start=2026-05-01&end=2026-05-31", nil)

	filter := articleFilterFromRequest(req)

	if filter.Page != 2 || filter.PageSize != 50 {
		t.Fatalf("unexpected pagination filter: %+v", filter)
	}
	if filter.Keyword != "alpha" || filter.SourceType != "flash" {
		t.Fatalf("unexpected text filter: %+v", filter)
	}
	if filter.ProjectID != 12 || filter.UserID != 99 {
		t.Fatalf("unexpected id filter: %+v", filter)
	}
	if filter.Start != "2026-05-01" || filter.End != "2026-05-31" {
		t.Fatalf("unexpected range filter: %+v", filter)
	}

	req = httptest.NewRequest(http.MethodGet, "/?industry=finance&province=guangdong&city=shenzhen&sort=captured_at_desc&read=read&favorite=favorited", nil)
	filter = articleFilterFromRequest(req)
	if filter.Industry != "finance" || filter.Province != "guangdong" || filter.City != "shenzhen" {
		t.Fatalf("unexpected advanced filters: %+v", filter)
	}
	if filter.Sort != "captured_at_desc" || filter.Read != "read" || filter.Favorite != "favorited" {
		t.Fatalf("unexpected sort/state filters: %+v", filter)
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Run("valid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"title":"hello"}`))
		recorder := httptest.NewRecorder()
		var payload struct {
			Title string `json:"title"`
		}

		if ok := decodeJSON(recorder, req, &payload); !ok {
			t.Fatal("expected decodeJSON to succeed")
		}
		if payload.Title != "hello" {
			t.Fatalf("expected decoded title, got %+v", payload)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
		recorder := httptest.NewRecorder()

		if ok := decodeJSON(recorder, req, &struct{}{}); ok {
			t.Fatal("expected decodeJSON to fail")
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
		}

		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if payload["message"] != "invalid body" {
			t.Fatalf("expected invalid body message, got %+v", payload)
		}
	})
}

func TestParseID(t *testing.T) {
	newRequest := func(id string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		return req.WithContext(contextWithRoute(req, rctx))
	}

	t.Run("valid id", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		id, ok := parseID(recorder, newRequest("42"), "id")
		if !ok || id != 42 {
			t.Fatalf("expected valid parsed id, got id=%d ok=%v", id, ok)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		id, ok := parseID(recorder, newRequest("bad"), "id")
		if ok || id != 0 {
			t.Fatalf("expected parse failure, got id=%d ok=%v", id, ok)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
		}
	})
}

func TestSummarizeTextAndNonEmpty(t *testing.T) {
	short := " short text "
	if got := summarizeText(short); got != "short text" {
		t.Fatalf("expected trimmed short text, got %q", got)
	}

	long := strings.Repeat("中", 141)
	got := summarizeText(long)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis for long text, got %q", got)
	}
	if len([]rune(got)) != 143 {
		t.Fatalf("expected 140 runes plus ellipsis, got %d runes", len([]rune(got)))
	}

	if got := nonEmpty("", "  ", " alpha ", "beta"); got != "alpha" {
		t.Fatalf("expected first non-empty trimmed value, got %q", got)
	}
}

func TestSyncTemplateWebsiteConfig(t *testing.T) {
	got := syncTemplateWebsiteConfig(`{"source_type":"flash","base_url":"https://example.com"}`, "flash.example.com")
	if !strings.Contains(got, `"website":"flash.example.com"`) {
		t.Fatalf("expected website injected into config_json, got %s", got)
	}

	got = syncTemplateWebsiteConfig(`{"source_type":"flash","website":"old.example.com"}`, "")
	if strings.Contains(got, `"website"`) {
		t.Fatalf("expected website removed from config_json, got %s", got)
	}
}

func TestSearchSuggestionAndHotKeywordHandlers(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)

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
		if err := store.SaveSearchWord(context.Background(), item.userID, item.word); err != nil {
			t.Fatalf("SaveSearchWord error: %v", err)
		}
	}

	suggestionReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/suggestions?user_id=42&q=%E9%92%A2&limit=5", nil)
	suggestionRR := httptest.NewRecorder()
	svc.handleSearchSuggestions(suggestionRR, suggestionReq)
	if suggestionRR.Code != http.StatusOK {
		t.Fatalf("expected suggestions 200, got %d", suggestionRR.Code)
	}
	var suggestionEnvelope struct {
		Code int                    `json:"code"`
		Data []model.SearchWordStat `json:"data"`
	}
	if err := json.Unmarshal(suggestionRR.Body.Bytes(), &suggestionEnvelope); err != nil {
		t.Fatalf("unmarshal suggestions: %v", err)
	}
	if suggestionEnvelope.Code != http.StatusOK || len(suggestionEnvelope.Data) != 2 {
		t.Fatalf("unexpected suggestions payload: %+v", suggestionEnvelope)
	}
	if suggestionEnvelope.Data[0].SearchWord != "钢铁" || suggestionEnvelope.Data[0].WordCount != 2 {
		t.Fatalf("unexpected first suggestion: %+v", suggestionEnvelope.Data[0])
	}

	hotReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/hot-keywords?limit=5", nil)
	hotRR := httptest.NewRecorder()
	svc.handleHotKeywords(hotRR, hotReq)
	if hotRR.Code != http.StatusOK {
		t.Fatalf("expected hot keywords 200, got %d", hotRR.Code)
	}
	var hotEnvelope struct {
		Code int                    `json:"code"`
		Data []model.SearchWordStat `json:"data"`
	}
	if err := json.Unmarshal(hotRR.Body.Bytes(), &hotEnvelope); err != nil {
		t.Fatalf("unmarshal hot keywords: %v", err)
	}
	if hotEnvelope.Code != http.StatusOK || len(hotEnvelope.Data) < 2 {
		t.Fatalf("unexpected hot keywords payload: %+v", hotEnvelope)
	}
	if hotEnvelope.Data[0].SearchWord != "钢铁" || hotEnvelope.Data[0].WordCount != 3 {
		t.Fatalf("unexpected hot keyword ranking: %+v", hotEnvelope.Data[0])
	}
}

func TestSearchDetailHandler(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	ctx := context.Background()

	now := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC)
	_, _, err := store.UpsertItems(ctx, []model.Item{{
		SourceType:      "investment",
		SourceKey:       "detail-key-1",
		Title:           "Alpha AI 完成 A 轮融资",
		Content:         "正文内容",
		Summary:         "摘要内容",
		DetailURL:       "https://example.com/detail/1",
		SourceURL:       "https://example.com/source/1",
		PublishTimeText: "2026-06-12 11:00:00",
		RawPayload:      `{"companyName":"Alpha AI","historyArray":"[{\"history_rounds\":\"天使轮\"}]","detailUrl":"https://example.com/payload-detail/1"}`,
		CapturedAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("ListItems error: %v %+v", err, list)
	}
	itemID := list.Items[0].ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/details/1", nil)
	req = req.WithContext(contextWithRoute(req, routeContextWithID(itemID)))
	rr := httptest.NewRecorder()
	svc.handleSearchDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d", rr.Code)
	}
	var envelope struct {
		Code int                `json:"code"`
		Data model.SearchDetail `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Payload["companyName"] != "Alpha AI" {
		t.Fatalf("expected payload fields preserved, got %+v", envelope.Data.Payload)
	}
	if envelope.Data.DetailURL != "https://example.com/payload-detail/1" {
		t.Fatalf("expected detail URL to prefer payload field, got %+v", envelope.Data)
	}
	if envelope.Data.URL != "https://example.com/source/1" {
		t.Fatalf("expected canonical URL fallback, got %+v", envelope.Data)
	}
}

func TestSearchMetadataAndSpecialHandlers(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	ctx := context.Background()

	now := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC)
	_, _, err := store.UpsertItems(ctx, []model.Item{
		{
			SourceType:      "lawyer",
			SourceKey:       "lawyer-1",
			Title:           "张三律师",
			Content:         "擅长公司法与投融资",
			Summary:         "南京律师",
			SourceURL:       "https://example.com/lawyer/1",
			PublishTimeText: "2026-06-12 11:00:00",
			RawPayload:      `{"name":"张三","lawfirm":"金陵律师事务所","goods":"公司法","city":"南京"}`,
			CapturedAt:      now,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		{
			SourceType:      "company",
			SourceKey:       "company-1",
			Title:           "星云科技有限公司",
			Content:         "企业信息与股东结构",
			Summary:         "高新技术企业",
			SourceURL:       "https://example.com/company/1",
			PublishTimeText: "2026-06-12 11:10:00",
			RawPayload:      `{"name":"星云科技有限公司","industry_involved":"人工智能","legal_person":"李四","location":"上海市浦东新区"}`,
			CapturedAt:      now.Add(time.Minute),
			CreatedAt:       now.Add(time.Minute),
			UpdatedAt:       now.Add(time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	typeReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/metadata/types?level=1", nil)
	typeRR := httptest.NewRecorder()
	svc.handleSearchMetadataTypes(typeRR, typeReq)
	if typeRR.Code != http.StatusOK {
		t.Fatalf("expected metadata types 200, got %d", typeRR.Code)
	}
	var typeEnvelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(typeRR.Body.Bytes(), &typeEnvelope); err != nil {
		t.Fatalf("decode metadata types: %v", err)
	}
	if len(typeEnvelope.Data) == 0 {
		t.Fatalf("expected metadata types, got %+v", typeEnvelope.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/special/lawyer?q=%E5%BC%A0%E4%B8%89%E5%BE%8B%E5%B8%88&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	svc.handleSearchSpecialList(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected special list 200, got %d", listRR.Code)
	}
	var listEnvelope struct {
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode special list: %v", err)
	}
	if len(listEnvelope.Data.List) != 1 || listEnvelope.Data.List[0]["lawfirm"] != "金陵律师事务所" {
		t.Fatalf("unexpected lawyer special list: %+v", listEnvelope.Data.List)
	}

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/special/company/options", nil)
	optionsRR := httptest.NewRecorder()
	svc.handleSearchSpecialOptions(optionsRR, optionsReq)
	if optionsRR.Code != http.StatusOK {
		t.Fatalf("expected special options 200, got %d", optionsRR.Code)
	}
	var optionsEnvelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(optionsRR.Body.Bytes(), &optionsEnvelope); err != nil {
		t.Fatalf("decode special options: %v", err)
	}
	if len(optionsEnvelope.Data) < 2 {
		t.Fatalf("expected dynamic company options, got %+v", optionsEnvelope.Data)
	}

	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListItems error: %v", err)
	}
	var companyID int64
	for _, item := range list.Items {
		if item.SourceType == "company" {
			companyID = item.ID
			break
		}
	}
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/special/company/details/1", nil)
	detailReq = detailReq.WithContext(contextWithRoute(detailReq, routeContextWithID(companyID)))
	detailRR := httptest.NewRecorder()
	svc.handleSearchSpecialDetail(detailRR, detailReq)
	if detailRR.Code != http.StatusOK {
		t.Fatalf("expected special detail 200, got %d", detailRR.Code)
	}
	var detailEnvelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(detailRR.Body.Bytes(), &detailEnvelope); err != nil {
		t.Fatalf("decode special detail: %v", err)
	}
	if detailEnvelope.Data["name"] != "星云科技有限公司" || detailEnvelope.Data["industry_involved"] != "人工智能" {
		t.Fatalf("unexpected company detail payload: %+v", detailEnvelope.Data)
	}
}

func TestArticleEmotionDeleteAndShareHandlers(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	ctx := context.Background()

	now := time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC)
	_, _, err := store.UpsertItems(ctx, []model.Item{{
		SourceType:  "headline",
		SourceKey:   "handler-key-1",
		Title:       "handler article",
		Content:     "handler content",
		Summary:     "handler summary",
		SourceURL:   "https://www.jin10.com/",
		CapturedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishTime: "2026-05-29 11:00:00",
	}})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("ListItems error: %v %+v", err, list)
	}
	itemID := list.Items[0].ID

	emotionReq := httptest.NewRequest(http.MethodPost, "/api/v1/articles/1/emotion?flag=2", strings.NewReader("flag=2"))
	emotionReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	emotionReq = emotionReq.WithContext(contextWithRoute(emotionReq, routeContextWithID(itemID)))
	emotionRR := httptest.NewRecorder()
	svc.handleSetArticleEmotion(emotionRR, emotionReq)
	if emotionRR.Code != http.StatusOK {
		t.Fatalf("expected emotion handler success, got %d", emotionRR.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/articles/1", nil)
	deleteReq = deleteReq.WithContext(contextWithRoute(deleteReq, routeContextWithID(itemID)))
	deleteRR := httptest.NewRecorder()
	svc.handleDeleteArticle(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected delete handler success, got %d", deleteRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/articles/1", nil)
	getReq = getReq.WithContext(contextWithRoute(getReq, routeContextWithID(itemID)))
	getRR := httptest.NewRecorder()
	svc.handleGetArticle(getRR, getReq)
	if getRR.Code != http.StatusNotFound {
		t.Fatalf("expected deleted article to return 404, got %d", getRR.Code)
	}

	shareReq := httptest.NewRequest(http.MethodPost, "/api/v1/articles/1/share?user_id=42", strings.NewReader(`{"channel":"project:7"}`))
	shareReq.Header.Set("Content-Type", "application/json")
	shareReq = shareReq.WithContext(contextWithRoute(shareReq, routeContextWithID(itemID)))
	shareRR := httptest.NewRecorder()
	svc.handleShareArticle(shareRR, shareReq)
	if shareRR.Code != http.StatusOK {
		t.Fatalf("expected share handler success, got %d", shareRR.Code)
	}
}

func TestArticleStatusHandler(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	_, _, err := store.UpsertItems(ctx, []model.Item{{
		SourceType: "headline",
		SourceKey:  "status-key-1",
		Title:      "status article",
		Content:    "content",
		Summary:    "summary",
		SourceURL:  "https://example.com/status",
		CapturedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("ListItems error: %v %+v", err, list)
	}
	itemID := list.Items[0].ID

	statusReq := httptest.NewRequest(http.MethodPut, "/api/v1/articles/1/status", strings.NewReader(`{"status":"invalid"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusReq = statusReq.WithContext(contextWithRoute(statusReq, routeContextWithID(itemID)))
	statusRR := httptest.NewRecorder()
	svc.handleSetArticleStatus(statusRR, statusReq)
	if statusRR.Code != http.StatusOK {
		t.Fatalf("expected status handler success, got %d", statusRR.Code)
	}
}

func TestReportBatchHandlers(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	ctx := context.Background()

	report1, err := store.CreateReport(ctx, model.Report{ProjectID: 1, Title: "report 1", Content: "content 1", Status: "draft"})
	if err != nil {
		t.Fatalf("CreateReport report1 error: %v", err)
	}
	report2, err := store.CreateReport(ctx, model.Report{ProjectID: 1, Title: "report 2", Content: "content 2", Status: "draft"})
	if err != nil {
		t.Fatalf("CreateReport report2 error: %v", err)
	}

	statusReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/batch-status", strings.NewReader(fmt.Sprintf(`{"report_ids":[%d,%d],"status":"generated"}`, report1.ID, report2.ID)))
	statusReq.Header.Set("Content-Type", "application/json")
	statusRR := httptest.NewRecorder()
	svc.handleBatchUpdateReportStatus(statusRR, statusReq)
	if statusRR.Code != http.StatusOK {
		t.Fatalf("expected batch status 200, got %d body=%s", statusRR.Code, statusRR.Body.String())
	}

	updated1, err := store.GetReport(ctx, report1.ID)
	if err != nil {
		t.Fatalf("GetReport report1 error: %v", err)
	}
	updated2, err := store.GetReport(ctx, report2.ID)
	if err != nil {
		t.Fatalf("GetReport report2 error: %v", err)
	}
	if updated1.Status != "generated" || updated2.Status != "generated" {
		t.Fatalf("expected generated statuses, got %q and %q", updated1.Status, updated2.Status)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/batch-delete", strings.NewReader(fmt.Sprintf(`{"report_ids":[%d,%d]}`, report1.ID, report2.ID)))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRR := httptest.NewRecorder()
	svc.handleBatchDeleteReports(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected batch delete 200, got %d body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	archived1, _ := store.GetReport(ctx, report1.ID)
	archived2, _ := store.GetReport(ctx, report2.ID)
	if archived1.Status != "archived" || archived2.Status != "archived" {
		t.Fatalf("expected archived statuses, got %q and %q", archived1.Status, archived2.Status)
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/batch-status", strings.NewReader(`{"report_ids":[1],"status":"invalid"}`))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRR := httptest.NewRecorder()
	svc.handleBatchUpdateReportStatus(invalidRR, invalidReq)
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid status 400, got %d", invalidRR.Code)
	}
}

func TestHandleCryptoPairResolve(t *testing.T) {
	svc := NewService(config.Config{}, newContentSearchTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/pairs/resolve?q=btc/usdt", nil)
	rr := httptest.NewRecorder()

	svc.handleCryptoPairResolve(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Data model.CryptoPairResolution `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal resolve response: %v", err)
	}
	if envelope.Data.Pair != "BTCUSDT" || envelope.Data.BaseAsset != "BTC" || envelope.Data.QuoteAsset != "USDT" {
		t.Fatalf("unexpected resolution: %+v", envelope.Data)
	}
}

func TestHandleCryptoNews(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	now := time.Now().UTC()
	_, _, err := store.UpsertItems(context.Background(), []model.Item{
		{
			SourceType: "headline", SourceKey: "btc-news-1", Title: "Bitcoin ETF approval boosts BTC",
			Summary: "比特币走强，市场情绪偏多", SourceURL: "https://example.com/1",
			CapturedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			SourceType: "flash", SourceKey: "btc-news-2", Title: "比特币短线回落，市场担忧监管风险",
			Summary: "BTC 出现回调", SourceURL: "https://example.com/2",
			CapturedAt: now.Add(-4 * time.Hour), CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour),
		},
		{
			SourceType: "headline", SourceKey: "eth-news-1", Title: "Ethereum ecosystem update",
			Summary: "ETH 相关新闻", SourceURL: "https://example.com/3",
			CapturedAt: now.Add(-1 * time.Hour), CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/news?pair=BTCUSDT&page=1&page_size=10", nil)
	rr := httptest.NewRecorder()
	svc.handleCryptoNews(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data model.CryptoNewsResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal crypto news response: %v", err)
	}
	if envelope.Data.Resolution.Pair != "BTCUSDT" {
		t.Fatalf("unexpected pair resolution: %+v", envelope.Data.Resolution)
	}
	if envelope.Data.Total < 2 {
		t.Fatalf("expected BTC items, got %+v", envelope.Data)
	}
	if envelope.Data.Items[0].RelevanceScore < envelope.Data.Items[1].RelevanceScore {
		t.Fatalf("expected items sorted by score: %+v", envelope.Data.Items)
	}
}

func TestHandleCryptoSocial(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	now := time.Now().UTC()
	_, _, err := store.UpsertItems(context.Background(), []model.Item{
		{
			SourceType: "crypto_x", SourceKey: "btc-social-1", Title: "BTC whale transfer sparks bullish chatter",
			Content: "X.com traders expect breakout after whale inflow", SourceURL: "https://x.com/example/1",
			ExternalSourceHost: "x.com", FromText: "@cryptoalpha",
			CapturedAt: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			SourceType: "crypto_telegram", SourceKey: "btc-social-2", Title: "社区担忧监管风险，BTC 短线承压",
			Content: "Telegram 社群讨论监管与清算风险", SourceURL: "https://t.me/example/1",
			ExternalSourceHost: "t.me", FromText: "链上观察员",
			CapturedAt: now.Add(-90 * time.Minute), CreatedAt: now.Add(-90 * time.Minute), UpdatedAt: now.Add(-90 * time.Minute),
		},
		{
			SourceType: "headline", SourceKey: "btc-news-ignore", Title: "Bitcoin ETF update",
			Content: "This should not appear in social evidence", SourceURL: "https://example.com/news",
			CapturedAt: now.Add(-45 * time.Minute), CreatedAt: now.Add(-45 * time.Minute), UpdatedAt: now.Add(-45 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/social?pair=BTCUSDT&page=1&page_size=10", nil)
	rr := httptest.NewRecorder()
	svc.handleCryptoSocial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data model.CryptoSocialResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal crypto social response: %v", err)
	}
	if envelope.Data.Total != 2 {
		t.Fatalf("expected 2 social items, got %+v", envelope.Data)
	}
	if envelope.Data.Items[0].Platform != "X" {
		t.Fatalf("expected first platform X, got %+v", envelope.Data.Items[0])
	}
	if envelope.Data.Items[1].Platform != "Telegram" {
		t.Fatalf("expected second platform Telegram, got %+v", envelope.Data.Items[1])
	}
}

func contextWithRoute(req *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
}

func routeContextWithID(id int64) *chi.Context {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	return rctx
}

func newContentSearchTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "content-search.db")
	store, err := sqlitestore.New(path)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
