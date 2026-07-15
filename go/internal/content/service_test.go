package content

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
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

func TestSystemFeedbackAPIUpsertsAndLists(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"user_id":7,"title":"列表建议","content":"右侧显示这条建议"}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/feedback", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected feedback upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/feedback?limit=20", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected feedback list 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var envelope struct {
		Data []model.Feedback `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal feedback list: %v", err)
	}
	if len(envelope.Data) != 1 || envelope.Data[0].UserID != 7 || envelope.Data[0].Title != "列表建议" || envelope.Data[0].Content != "右侧显示这条建议" {
		t.Fatalf("unexpected feedback list payload: %+v", envelope.Data)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/system/feedback/"+strconv.FormatInt(envelope.Data[0].ID, 10), nil)
	deleteRR := httptest.NewRecorder()
	router.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected feedback delete 200, got %d body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	listAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/feedback?limit=20", nil)
	listAfterDeleteRR := httptest.NewRecorder()
	router.ServeHTTP(listAfterDeleteRR, listAfterDeleteReq)
	if listAfterDeleteRR.Code != http.StatusOK {
		t.Fatalf("expected feedback list after delete 200, got %d body=%s", listAfterDeleteRR.Code, listAfterDeleteRR.Body.String())
	}
	envelope = struct {
		Data []model.Feedback `json:"data"`
	}{}
	if err := json.Unmarshal(listAfterDeleteRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal feedback list after delete: %v", err)
	}
	if len(envelope.Data) != 0 {
		t.Fatalf("expected feedback list empty after delete, got %+v", envelope.Data)
	}
}

func TestAStockAuctionAmountAPIUpsertsAndLists(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"date":"2026-06-16","items":[{"code":"002230","name":"科大讯飞","auction_price":41.2,"auction_volume":123400,"auction_amount":5084080,"source":"akshare_pre_min","status":"ok"},{"code":"600000","name":"浦发银行","auction_price":8.8,"auction_volume":90000,"auction_amount":792000,"source":"akshare_pre_min","status":"ok"}]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/a-stock/auction", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected auction upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/auction?date=2026-06-16&keyword=讯飞&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected auction list 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var envelope struct {
		Data model.AStockAuctionListResult `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal auction list: %v", err)
	}
	if envelope.Data.Date != "2026-06-16" || envelope.Data.Total != 1 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Code != "002230" {
		t.Fatalf("unexpected auction list payload: %+v", envelope.Data)
	}
	if envelope.Data.CaptureSlot != "0929" || envelope.Data.Items[0].CaptureSlot != "0929" {
		t.Fatalf("expected default 0929 capture slot, got %+v", envelope.Data)
	}
	if envelope.Data.TotalAmount != 5876080 || envelope.Data.MaxItem == nil || envelope.Data.MaxItem.Code != "002230" {
		t.Fatalf("expected date summary independent of keyword filter, got %+v", envelope.Data)
	}
	if len(envelope.Data.Trend) != 1 || envelope.Data.Trend[0].TotalAmount != 5876080 || envelope.Data.Trend[0].TotalVolume != 213400 {
		t.Fatalf("expected auction trend totals, got %+v", envelope.Data.Trend)
	}
	if len(envelope.Data.TrendSeries["0929"]) != 1 {
		t.Fatalf("expected 0929 trend series, got %+v", envelope.Data.TrendSeries)
	}
	if len(envelope.Data.Trend[0].MarketTop) != 2 ||
		envelope.Data.Trend[0].MarketTop[0].Market != "沪市" ||
		envelope.Data.Trend[0].MarketTop[0].Items[0].Code != "600000" ||
		envelope.Data.Trend[0].MarketTop[1].Market != "深市" ||
		envelope.Data.Trend[0].MarketTop[1].Items[0].Code != "002230" {
		t.Fatalf("expected per-market auction amount top stocks, got %+v", envelope.Data.Trend[0].MarketTop)
	}

	slotPayload := `{"date":"2026-06-16","capture_slot":"0925","items":[{"code":"002230","name":"科大讯飞","auction_price":40,"auction_volume":100000,"auction_amount":4000000,"source":"eastmoney_clist","status":"ok"}],"replace":true}`
	slotPostReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/a-stock/auction", strings.NewReader(slotPayload))
	slotPostRR := httptest.NewRecorder()
	router.ServeHTTP(slotPostRR, slotPostReq)
	if slotPostRR.Code != http.StatusOK {
		t.Fatalf("expected 0925 auction upsert 200, got %d body=%s", slotPostRR.Code, slotPostRR.Body.String())
	}
	slotListReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/auction?date=2026-06-16&capture_slot=0925&page=1&page_size=10", nil)
	slotListRR := httptest.NewRecorder()
	router.ServeHTTP(slotListRR, slotListReq)
	if slotListRR.Code != http.StatusOK {
		t.Fatalf("expected 0925 auction list 200, got %d body=%s", slotListRR.Code, slotListRR.Body.String())
	}
	if err := json.Unmarshal(slotListRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal 0925 auction list: %v", err)
	}
	if envelope.Data.CaptureSlot != "0925" || envelope.Data.TotalAmount != 4000000 || envelope.Data.Items[0].CaptureSlot != "0925" {
		t.Fatalf("expected 0925 auction list, got %+v", envelope.Data)
	}
	if len(envelope.Data.Trend) != 1 || envelope.Data.Trend[0].CaptureSlot != "0929" || envelope.Data.Trend[0].TotalAmount != 5876080 {
		t.Fatalf("expected 0925 list response to keep 0929 history trend, got %+v", envelope.Data.Trend)
	}
}

func TestAStockRecommendationSnapshotAPIUpsertsAndGets(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"strategy_date":"2026-06-22","period":"afternoon","ignore_recent":false,"recommendations_json":"[{\"Code\":\"600000\"}]","backtests_json":"null","backtest_status":"已回测 1/1","generated_count":1,"limit_up_filter_enabled":true,"limit_up_filtered":2,"today_market_filter_enabled":true,"no_today_market_count":4,"fund_flow_filter_enabled":true,"fund_flow_filtered":3,"fund_flow_missing_count":1,"market_candidate_count":9}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/recommendations", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected recommendation snapshot upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	disabledPayload := `{"strategy_date":"2026-06-22","period":"afternoon","ignore_recent":false,"recommendations_json":"[{\"Code\":\"000001\"}]","backtests_json":"[]","backtest_status":"资金过滤关闭","generated_count":2,"limit_up_filter_enabled":true,"today_market_filter_enabled":true,"fund_flow_filter_enabled":false,"fund_flow_filtered":0}`
	disabledPostReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/recommendations", strings.NewReader(disabledPayload))
	disabledPostRR := httptest.NewRecorder()
	router.ServeHTTP(disabledPostRR, disabledPostReq)
	if disabledPostRR.Code != http.StatusOK {
		t.Fatalf("expected disabled fund-flow snapshot upsert 200, got %d body=%s", disabledPostRR.Code, disabledPostRR.Body.String())
	}

	defaultEnabledPayload := `{"strategy_date":"2026-06-22","period":"afternoon","ignore_recent":false,"recommendations_json":"[{\"Code\":\"600001\"}]","backtests_json":"[]","backtest_status":"网页默认资金过滤开启","generated_count":1,"limit_up_filter_enabled":false,"today_market_filter_enabled":false,"fund_flow_filter_enabled":true,"fund_flow_filtered":1}`
	defaultEnabledPostReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/recommendations", strings.NewReader(defaultEnabledPayload))
	defaultEnabledPostRR := httptest.NewRecorder()
	router.ServeHTTP(defaultEnabledPostRR, defaultEnabledPostReq)
	if defaultEnabledPostRR.Code != http.StatusOK {
		t.Fatalf("expected default enabled fund-flow snapshot upsert 200, got %d body=%s", defaultEnabledPostRR.Code, defaultEnabledPostRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendations?date=2026-06-22&period=afternoon&limit_up_filter_enabled=1&today_market_filter_enabled=1&fund_flow_filter_enabled=1", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected recommendation snapshot get 200, got %d body=%s", getRR.Code, getRR.Body.String())
	}
	var envelope struct {
		Data model.AStockRecommendationSnapshot `json:"data"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode snapshot response error: %v", err)
	}
	if !envelope.Data.Found || envelope.Data.StrategyDate != "2026-06-22" || envelope.Data.Period != "afternoon" || envelope.Data.GeneratedCount != 1 || !envelope.Data.LimitUpFilterEnabled || envelope.Data.LimitUpFiltered != 2 || !envelope.Data.TodayMarketFilterEnabled || envelope.Data.NoTodayMarketCount != 4 || !envelope.Data.FundFlowFilterEnabled || envelope.Data.FundFlowFiltered != 3 || envelope.Data.FundFlowMissingCount != 1 {
		t.Fatalf("unexpected snapshot response: %+v", envelope.Data)
	}
	if envelope.Data.BacktestsJSON != "[]" {
		t.Fatalf("expected null backtests json to be normalized to [], got %q", envelope.Data.BacktestsJSON)
	}
	if !strings.Contains(envelope.Data.RecommendationsJSON, "600000") {
		t.Fatalf("expected enabled fund-flow exact snapshot, got %s", envelope.Data.RecommendationsJSON)
	}

	defaultGetReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendations?date=2026-06-22&period=afternoon", nil)
	defaultGetRR := httptest.NewRecorder()
	router.ServeHTTP(defaultGetRR, defaultGetReq)
	if defaultGetRR.Code != http.StatusOK {
		t.Fatalf("expected default recommendation snapshot get 200, got %d body=%s", defaultGetRR.Code, defaultGetRR.Body.String())
	}
	var defaultEnvelope struct {
		Data model.AStockRecommendationSnapshot `json:"data"`
	}
	if err := json.Unmarshal(defaultGetRR.Body.Bytes(), &defaultEnvelope); err != nil {
		t.Fatalf("decode default snapshot response error: %v", err)
	}
	if !defaultEnvelope.Data.Found || !defaultEnvelope.Data.FundFlowFilterEnabled || !strings.Contains(defaultEnvelope.Data.RecommendationsJSON, "600001") {
		t.Fatalf("expected default get to use fund-flow enabled snapshot, got %+v", defaultEnvelope.Data)
	}

	disabledGetReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendations?date=2026-06-22&period=afternoon&limit_up_filter_enabled=1&today_market_filter_enabled=1&fund_flow_filter_enabled=0", nil)
	disabledGetRR := httptest.NewRecorder()
	router.ServeHTTP(disabledGetRR, disabledGetReq)
	if disabledGetRR.Code != http.StatusOK {
		t.Fatalf("expected disabled fund-flow snapshot get 200, got %d body=%s", disabledGetRR.Code, disabledGetRR.Body.String())
	}
	var disabledEnvelope struct {
		Data model.AStockRecommendationSnapshot `json:"data"`
	}
	if err := json.Unmarshal(disabledGetRR.Body.Bytes(), &disabledEnvelope); err != nil {
		t.Fatalf("decode disabled snapshot response error: %v", err)
	}
	if !disabledEnvelope.Data.Found || disabledEnvelope.Data.FundFlowFilterEnabled || disabledEnvelope.Data.GeneratedCount != 2 || !strings.Contains(disabledEnvelope.Data.RecommendationsJSON, "000001") {
		t.Fatalf("expected disabled fund-flow exact snapshot, got %+v", disabledEnvelope.Data)
	}
}

func TestAStockRecommendationPerformanceAPIUsesOfficialAndShadowSnapshots(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	officialPayload := `{"strategy_date":"2026-06-23","period":"morning","recommendations_json":"[{\"Rank\":1,\"Hotspot\":\"人工智能\",\"Code\":\"600001\",\"Name\":\"正式一号\",\"MarketScore\":260,\"FundFlow5D\":\"+1.00亿\",\"Change30\":\"+5.00%\",\"Change60\":\"+8.00%\"}]","backtests_json":"[{\"Stock\":\"600001 正式一号\",\"Days\":[{\"Return\":\"+1.00%\"}]}]","backtest_status":"已回测","generated_count":1}`
	officialReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/recommendations", strings.NewReader(officialPayload))
	officialRR := httptest.NewRecorder()
	router.ServeHTTP(officialRR, officialReq)
	if officialRR.Code != http.StatusOK {
		t.Fatalf("expected official upsert 200, got %d body=%s", officialRR.Code, officialRR.Body.String())
	}

	shadowPayload := `{"strategy_key":"t1_shadow_v1","strategy_date":"2026-06-23","period":"morning","recommendations_json":"[{\"Rank\":1,\"Hotspot\":\"人工智能\",\"Code\":\"600002\",\"Name\":\"影子一号\",\"MarketScore\":300,\"FundFlow5D\":\"+2.00亿\",\"Change30\":\"+6.00%\",\"Change60\":\"+9.00%\"}]","backtests_json":"[{\"Stock\":\"600002 影子一号\",\"Days\":[{\"Return\":\"-0.50%\"}]}]","backtest_status":"已回测","generated_count":1}`
	shadowReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/recommendation-shadow-snapshots", strings.NewReader(shadowPayload))
	shadowRR := httptest.NewRecorder()
	router.ServeHTTP(shadowRR, shadowReq)
	if shadowRR.Code != http.StatusOK {
		t.Fatalf("expected shadow upsert 200, got %d body=%s", shadowRR.Code, shadowRR.Body.String())
	}

	officialPerfReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendation-performance?start=2026-06-01&end=2026-06-30&period=all&strategy=official", nil)
	officialPerfRR := httptest.NewRecorder()
	router.ServeHTTP(officialPerfRR, officialPerfReq)
	if officialPerfRR.Code != http.StatusOK {
		t.Fatalf("expected official performance 200, got %d body=%s", officialPerfRR.Code, officialPerfRR.Body.String())
	}
	var officialEnvelope struct {
		Data model.AStockRecommendationPerformanceSummary `json:"data"`
	}
	if err := json.Unmarshal(officialPerfRR.Body.Bytes(), &officialEnvelope); err != nil {
		t.Fatalf("decode official performance: %v", err)
	}
	if officialEnvelope.Data.SampleCount != 1 || officialEnvelope.Data.WinCount != 1 || officialEnvelope.Data.WinRate != 1 {
		t.Fatalf("unexpected official performance: %+v", officialEnvelope.Data)
	}

	shadowPerfReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendation-performance?start=2026-06-01&end=2026-06-30&period=all&strategy=t1_shadow_v1", nil)
	shadowPerfRR := httptest.NewRecorder()
	router.ServeHTTP(shadowPerfRR, shadowPerfReq)
	if shadowPerfRR.Code != http.StatusOK {
		t.Fatalf("expected shadow performance 200, got %d body=%s", shadowPerfRR.Code, shadowPerfRR.Body.String())
	}
	var shadowEnvelope struct {
		Data model.AStockRecommendationPerformanceSummary `json:"data"`
	}
	if err := json.Unmarshal(shadowPerfRR.Body.Bytes(), &shadowEnvelope); err != nil {
		t.Fatalf("decode shadow performance: %v", err)
	}
	if shadowEnvelope.Data.Strategy != "t1_shadow_v1" || shadowEnvelope.Data.SampleCount != 1 || shadowEnvelope.Data.WinCount != 0 || shadowEnvelope.Data.WinRate != 0 {
		t.Fatalf("unexpected shadow performance: %+v", shadowEnvelope.Data)
	}
}

func TestAStockBacktestRefreshPriceAPIProxiesToGateway(t *testing.T) {
	var gatewayCalled bool
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/internal/a-stock/backtests/refresh-price" {
			t.Fatalf("unexpected gateway request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret" {
			t.Fatalf("expected service token to be forwarded, got %q", r.Header.Get("X-Service-Token"))
		}
		var payload aStockBacktestRefreshPriceRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode gateway payload: %v", err)
		}
		if payload.Date != "2026-07-10" || payload.Period != "morning" || payload.Code != "300394" {
			t.Fatalf("unexpected gateway payload: %+v", payload)
		}
		apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
			"summary": "价格已刷新",
			"detail":  "明细",
			"snapshot": model.AStockRecommendationSnapshot{
				Found:               true,
				StrategyDate:        "2026-07-10",
				Period:              "morning",
				RecommendationsJSON: "[]",
				BacktestsJSON:       "[]",
			},
		})
	}))
	defer gateway.Close()

	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{GatewayWebURL: gateway.URL, ServiceToken: "secret"}, store)
	router := svc.Router()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/a-stock/backtests/refresh-price", strings.NewReader(`{"date":"2026-07-10","period":"morning","code":"300394"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected refresh proxy 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !gatewayCalled {
		t.Fatal("expected gateway to be called")
	}
	var envelope struct {
		Data struct {
			Summary  string                             `json:"summary"`
			Snapshot model.AStockRecommendationSnapshot `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}
	if envelope.Data.Summary != "价格已刷新" || !envelope.Data.Snapshot.Found || envelope.Data.Snapshot.Period != "morning" {
		t.Fatalf("unexpected proxy response: %+v", envelope.Data)
	}
}

func TestAStockRecommendationSelectionsAPIUpsertsAndLists(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"strategy_date":"2026-06-23","period":"afternoon","items":[{"rank":1,"code":"002008","name":"大族激光","hotspot":"机器人","market_score":91,"reason":"locked-1","entry_time":"10:30"},{"rank":2,"code":"688367","name":"工大高科","hotspot":"机器人","market_score":87,"reason":"locked-2","entry_time":"13:01"}]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/recommendation-selections", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected recommendation selections upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendation-selections?date=2026-06-23&period=afternoon", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected recommendation selections get 200, got %d body=%s", getRR.Code, getRR.Body.String())
	}
	var envelope struct {
		Data model.AStockRecommendationSelectionListResult `json:"data"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode selection response error: %v", err)
	}
	if !envelope.Data.Found || envelope.Data.StrategyDate != "2026-06-23" || envelope.Data.Period != "afternoon" || len(envelope.Data.Items) != 2 {
		t.Fatalf("unexpected selection response: %+v", envelope.Data)
	}
	if envelope.Data.Items[0].Code != "002008" || envelope.Data.Items[0].EntryTime != "10:30" || envelope.Data.Items[1].Code != "688367" || envelope.Data.Items[1].EntryTime != "13:01" {
		t.Fatalf("unexpected selection items: %+v", envelope.Data.Items)
	}
}

func TestAStockRecommendationLatestDatesAPIListsBatchHits(t *testing.T) {
	store := newContentSearchTestStore(t)
	if _, err := store.UpsertAStockRecommendationSelections(context.Background(), model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-23",
		Period:       "afternoon",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人"},
		},
	}); err != nil {
		t.Fatalf("upsert old selection: %v", err)
	}
	if _, err := store.UpsertAStockRecommendationSelections(context.Background(), model.AStockRecommendationSelectionSet{
		StrategyDate: "2026-06-24",
		Period:       "morning",
		Items: []model.AStockRecommendationSelection{
			{Rank: 1, Code: "688367", Name: "工大高科", Hotspot: "机器人"},
		},
	}); err != nil {
		t.Fatalf("upsert current selection: %v", err)
	}
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/recommendation-latest-dates?date=2026-06-24&period=morning&codes=002008,688367,000000", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected recommendation latest dates get 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data model.AStockRecommendationLatestDateListResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode latest dates response error: %v", err)
	}
	if envelope.Data.StrategyDate != "2026-06-24" || envelope.Data.Period != "morning" || len(envelope.Data.Items) != 1 {
		t.Fatalf("unexpected latest dates response: %+v", envelope.Data)
	}
	if envelope.Data.Items[0].Code != "002008" || envelope.Data.Items[0].LatestDate != "2026-06-23" {
		t.Fatalf("unexpected latest date item: %+v", envelope.Data.Items)
	}
}

func TestStockResearchAPIUpsertsAndLists(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"items":[{"code":"002230","name":"科大讯飞","kind":"report","title":"科大讯飞深度研究","institution":"中金公司","analyst":"张三","rating":"买入","target_price":"50.00","research_date":"2026-06-16","source_type":"sina_finance_report","source_key":"sina-1","source_url":"https://sina.example.com/1","source_text":"已入库原文","source_fetch_status":"parsed","source_fetched_at":"2026-06-16T01:00:00Z"},{"code":"300059","name":"东方财富","kind":"survey","title":"东方财富机构调研","institution":"华泰证券","research_date":"2026-06-15","source_type":"sohu_finance_report","source_key":"sohu-1"}]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/stock-research/batch", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected stock research upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/stock-research?company=科大&institution=中金&source=sina_finance_report&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected stock research list 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var envelope struct {
		Data model.StockResearchListResult `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stock research list: %v", err)
	}
	if envelope.Data.Total != 1 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Code != "002230" {
		t.Fatalf("unexpected stock research list payload: %+v", envelope.Data)
	}
	if envelope.Data.Items[0].Rating != "买入" || envelope.Data.Items[0].TargetPrice != "50.00" {
		t.Fatalf("expected rating and target price, got %+v", envelope.Data.Items[0])
	}
	if envelope.Data.Items[0].SourceText != "已入库原文" || envelope.Data.Items[0].SourceFetchStatus != "parsed" {
		t.Fatalf("expected source fields in list response, got %+v", envelope.Data.Items[0])
	}
	itemID := envelope.Data.Items[0].ID

	skipReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/internal/stock-research/%d/source", itemID), strings.NewReader(`{"source_text":"新正文","source_fetch_status":"parsed","source_fetched_at":"2026-06-16T02:00:00Z"}`))
	skipRR := httptest.NewRecorder()
	router.ServeHTTP(skipRR, skipReq)
	if skipRR.Code != http.StatusOK {
		t.Fatalf("expected source skip update 200, got %d body=%s", skipRR.Code, skipRR.Body.String())
	}
	var sourceEnvelope struct {
		Data model.StockResearchSurvey `json:"data"`
	}
	if err := json.Unmarshal(skipRR.Body.Bytes(), &sourceEnvelope); err != nil {
		t.Fatalf("unmarshal source skip response: %v", err)
	}
	if sourceEnvelope.Data.SourceText != "已入库原文" {
		t.Fatalf("expected default source update to skip existing text, got %+v", sourceEnvelope.Data)
	}

	forceReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/internal/stock-research/%d/source", itemID), strings.NewReader(`{"source_text":"新正文","source_fetch_status":"parsed","source_fetched_at":"2026-06-16T02:00:00Z","force":true}`))
	forceRR := httptest.NewRecorder()
	router.ServeHTTP(forceRR, forceReq)
	if forceRR.Code != http.StatusOK {
		t.Fatalf("expected source force update 200, got %d body=%s", forceRR.Code, forceRR.Body.String())
	}
	if err := json.Unmarshal(forceRR.Body.Bytes(), &sourceEnvelope); err != nil {
		t.Fatalf("unmarshal source force response: %v", err)
	}
	if sourceEnvelope.Data.SourceText != "新正文" || sourceEnvelope.Data.SourceFetchedAt != "2026-06-16T02:00:00Z" {
		t.Fatalf("expected force source update to overwrite existing text, got %+v", sourceEnvelope.Data)
	}

	defaultPageReq := httptest.NewRequest(http.MethodGet, "/api/v1/stock-research?company=科大&page=1", nil)
	defaultPageRR := httptest.NewRecorder()
	router.ServeHTTP(defaultPageRR, defaultPageReq)
	if defaultPageRR.Code != http.StatusOK {
		t.Fatalf("expected stock research default page list 200, got %d body=%s", defaultPageRR.Code, defaultPageRR.Body.String())
	}
	envelope = struct {
		Data model.StockResearchListResult `json:"data"`
	}{}
	if err := json.Unmarshal(defaultPageRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stock research default page list: %v", err)
	}
	if envelope.Data.PageSize != 20 {
		t.Fatalf("expected stock research default page size 20, got %+v", envelope.Data)
	}
}

func TestStockResearchPDFAPIUpdatesDownloadsAndReadsText(t *testing.T) {
	store := newContentSearchTestStore(t)
	pdfRoot := t.TempDir()
	svc := NewService(config.Config{StockResearchPDFDir: pdfRoot}, store)
	router := svc.Router()

	payload := `{"items":[{"code":"002230","name":"科大讯飞","kind":"report","title":"科大讯飞深度研究","institution":"中金公司","research_date":"2026-06-16","source_type":"sina_finance_report","source_key":"sina-pdf-1","source_url":"https://sina.example.com/1"}]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/stock-research/batch", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected stock research upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/stock-research?source=sina_finance_report&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected stock research list 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var listEnvelope struct {
		Data model.StockResearchListResult `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("unmarshal stock research list: %v", err)
	}
	if len(listEnvelope.Data.Items) != 1 {
		t.Fatalf("expected one stock research item, got %+v", listEnvelope.Data)
	}
	itemID := listEnvelope.Data.Items[0].ID
	pdfPath := filepath.Join(pdfRoot, "sina_finance_report", "sina-pdf-1.pdf")
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		t.Fatalf("mkdir pdf dir: %v", err)
	}
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nfake\n"), 0o644); err != nil {
		t.Fatalf("write pdf file: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/internal/stock-research/%d/pdf", itemID), strings.NewReader(fmt.Sprintf(`{"pdf_url":"https://sina.example.com/1.pdf","pdf_file_path":%q,"pdf_status":"parsed","pdf_text":"科大讯飞研报正文","pdf_fetched_at":"2026-06-16T01:00:00Z","pdf_parsed_at":"2026-06-16T01:01:00Z"}`, pdfPath)))
	updateRR := httptest.NewRecorder()
	router.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("expected pdf update 200, got %d body=%s", updateRR.Code, updateRR.Body.String())
	}
	var updateEnvelope struct {
		Data model.StockResearchSurvey `json:"data"`
	}
	if err := json.Unmarshal(updateRR.Body.Bytes(), &updateEnvelope); err != nil {
		t.Fatalf("unmarshal pdf update response: %v", err)
	}
	if updateEnvelope.Data.SourceText != "科大讯飞研报正文" || updateEnvelope.Data.SourceFetchStatus != "parsed" {
		t.Fatalf("expected pdf update to sync source text, got %+v", updateEnvelope.Data)
	}

	skipPDFReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/internal/stock-research/%d/pdf", itemID), strings.NewReader(fmt.Sprintf(`{"pdf_url":"https://sina.example.com/1.pdf","pdf_file_path":%q,"pdf_status":"parsed","pdf_text":"新版研报正文","pdf_fetched_at":"2026-06-16T02:00:00Z","pdf_parsed_at":"2026-06-16T02:01:00Z"}`, pdfPath)))
	skipPDFRR := httptest.NewRecorder()
	router.ServeHTTP(skipPDFRR, skipPDFReq)
	if skipPDFRR.Code != http.StatusOK {
		t.Fatalf("expected pdf skip update 200, got %d body=%s", skipPDFRR.Code, skipPDFRR.Body.String())
	}
	if err := json.Unmarshal(skipPDFRR.Body.Bytes(), &updateEnvelope); err != nil {
		t.Fatalf("unmarshal pdf skip update response: %v", err)
	}
	if updateEnvelope.Data.PDFText != "新版研报正文" || updateEnvelope.Data.SourceText != "科大讯飞研报正文" {
		t.Fatalf("expected pdf update without force to preserve existing source text, got %+v", updateEnvelope.Data)
	}

	forcePDFReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/internal/stock-research/%d/pdf", itemID), strings.NewReader(fmt.Sprintf(`{"pdf_url":"https://sina.example.com/1.pdf","pdf_file_path":%q,"pdf_status":"parsed","pdf_text":"新版研报正文\n第二段","pdf_fetched_at":"2026-06-16T03:00:00Z","pdf_parsed_at":"2026-06-16T03:01:00Z","force":true}`, pdfPath)))
	forcePDFRR := httptest.NewRecorder()
	router.ServeHTTP(forcePDFRR, forcePDFReq)
	if forcePDFRR.Code != http.StatusOK {
		t.Fatalf("expected pdf force update 200, got %d body=%s", forcePDFRR.Code, forcePDFRR.Body.String())
	}
	if err := json.Unmarshal(forcePDFRR.Body.Bytes(), &updateEnvelope); err != nil {
		t.Fatalf("unmarshal pdf force update response: %v", err)
	}
	if updateEnvelope.Data.SourceText != "新版研报正文\n第二段" || updateEnvelope.Data.SourceFetchedAt != "2026-06-16T03:01:00Z" {
		t.Fatalf("expected pdf force update to overwrite source text, got %+v", updateEnvelope.Data)
	}

	nlpReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/internal/stock-research/%d/nlp", itemID), strings.NewReader(`{"nlp_score":86.5,"nlp_rating":"积极","nlp_reason":"油运景气提升","nlp_scored_at":"2026-06-16T04:00:00Z"}`))
	nlpRR := httptest.NewRecorder()
	router.ServeHTTP(nlpRR, nlpReq)
	if nlpRR.Code != http.StatusOK {
		t.Fatalf("expected nlp update 200, got %d body=%s", nlpRR.Code, nlpRR.Body.String())
	}
	if err := json.Unmarshal(nlpRR.Body.Bytes(), &updateEnvelope); err != nil {
		t.Fatalf("unmarshal nlp update response: %v", err)
	}
	if updateEnvelope.Data.NLPScore != 86.5 || updateEnvelope.Data.NLPRating != "积极" || updateEnvelope.Data.NLPReason != "油运景气提升" || updateEnvelope.Data.NLPScoredAt != "2026-06-16T04:00:00Z" {
		t.Fatalf("expected nlp fields to update, got %+v", updateEnvelope.Data)
	}
	if updateEnvelope.Data.SourceText != "新版研报正文\n第二段" || updateEnvelope.Data.PDFText != "新版研报正文\n第二段" {
		t.Fatalf("expected nlp update to preserve source and pdf text, got %+v", updateEnvelope.Data)
	}

	textReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stock-research/%d/pdf/text", itemID), nil)
	textRR := httptest.NewRecorder()
	router.ServeHTTP(textRR, textReq)
	if textRR.Code != http.StatusOK || !strings.Contains(textRR.Body.String(), "新版研报正文") {
		t.Fatalf("expected parsed pdf text, got status=%d body=%s", textRR.Code, textRR.Body.String())
	}

	pdfReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stock-research/%d/pdf", itemID), nil)
	pdfRR := httptest.NewRecorder()
	router.ServeHTTP(pdfRR, pdfReq)
	if pdfRR.Code != http.StatusOK || !strings.Contains(pdfRR.Body.String(), "%PDF-1.4") {
		t.Fatalf("expected pdf file response, got status=%d body=%s", pdfRR.Code, pdfRR.Body.String())
	}
}

func TestStockInstitutionHoldingAPIUpsertsListsAndSummarizes(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"items":[{"stock_code":"SH.002230","stock_name":"科大讯飞","report_period":"2026-Q1","announce_date":"2026-04-30","holder_name":"易方达基金","holder_type":"基金","holder_code":"110001","holder_rank":"1","shares":1000,"float_ratio":1.5,"market_value":50000,"source_type":"stock_institute_hold_detail","raw_payload":"{\"id\":1}"},{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"社保基金一一八组合","holder_type":"社保基金","shares":2000,"float_ratio":2.5,"market_value":100000,"source_type":"stock_gdfx_free_holding_detail_em"}]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/holdings/batch", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected holdings upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/holdings?code=002230&period=2026Q1&holder_type=fund&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected holdings list 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var listEnvelope struct {
		Data model.StockInstitutionHoldingListResult `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("unmarshal holdings list: %v", err)
	}
	if listEnvelope.Data.Total != 1 || len(listEnvelope.Data.Items) != 1 {
		t.Fatalf("unexpected holdings list payload: %+v", listEnvelope.Data)
	}
	item := listEnvelope.Data.Items[0]
	if item.StockCode != "002230" || item.ReportPeriod != "20260331" || item.HolderType != "fund" || !strings.Contains(item.RawPayload, `"id":1`) {
		t.Fatalf("expected normalized holding row, got %+v", item)
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/holdings/summary?code=002230&period=20260331", nil)
	summaryRR := httptest.NewRecorder()
	router.ServeHTTP(summaryRR, summaryReq)
	if summaryRR.Code != http.StatusOK {
		t.Fatalf("expected holdings summary 200, got %d body=%s", summaryRR.Code, summaryRR.Body.String())
	}
	var summaryEnvelope struct {
		Data model.StockInstitutionHoldingSummary `json:"data"`
	}
	if err := json.Unmarshal(summaryRR.Body.Bytes(), &summaryEnvelope); err != nil {
		t.Fatalf("unmarshal holdings summary: %v", err)
	}
	if summaryEnvelope.Data.HolderCount != 2 || summaryEnvelope.Data.FundCount != 1 || summaryEnvelope.Data.HolderTypeCount != 2 || summaryEnvelope.Data.TotalFloatRatio != 4 {
		t.Fatalf("unexpected holdings summary: %+v", summaryEnvelope.Data)
	}
}

func TestAStockSectorFundFlowAPIUpsertsAndLists(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"date":"2026-07-01","sector_type":"行业资金流","indicator":"今日","replace":true,"items":[{"rank":1,"name":"半导体","change_pct":2.5,"main_net_inflow":120000000,"main_net_inflow_pct":4.2,"top_stock":"中芯国际"}]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/sector-fund-flows", strings.NewReader(payload))
	postReq.Header.Set("Content-Type", "application/json")
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected sector fund flow upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/sector-fund-flows?date=2026-07-01&sector_type=行业资金流&indicator=今日&keyword=半导&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected sector fund flow list 200, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	var envelope struct {
		Data model.AStockSectorFundFlowListResult `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode sector fund flow list: %v", err)
	}
	if envelope.Data.Total != 1 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Name != "半导体" || envelope.Data.Items[0].TradeDate != "2026-07-01" {
		t.Fatalf("unexpected sector fund flow list: %+v", envelope.Data)
	}
	if envelope.Data.SectorType != "行业资金流" || envelope.Data.Indicator != "今日" || envelope.Data.Items[0].SourceType != "akshare_sector_fund_flow" {
		t.Fatalf("unexpected normalized sector fund flow fields: %+v", envelope.Data)
	}

	sourcePayload := `{"date":"2026-07-02","sector_type":"行业资金流","indicator":"今日","source_type":"eastmoney","replace":true,"items":[{"rank":1,"name":"电机","main_net_inflow":100,"field_counts_json":"{\"main_net_inflow\":1}"}]}`
	sourcePostReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/sector-fund-flow-sources", strings.NewReader(sourcePayload))
	sourcePostReq.Header.Set("Content-Type", "application/json")
	sourcePostRR := httptest.NewRecorder()
	router.ServeHTTP(sourcePostRR, sourcePostReq)
	if sourcePostRR.Code != http.StatusOK {
		t.Fatalf("expected sector fund flow source upsert 200, got %d body=%s", sourcePostRR.Code, sourcePostRR.Body.String())
	}
	sourceListReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/sector-fund-flows?date=2026-07-02&sector_type=行业资金流&indicator=今日&source=eastmoney", nil)
	sourceListRR := httptest.NewRecorder()
	router.ServeHTTP(sourceListRR, sourceListReq)
	if sourceListRR.Code != http.StatusOK {
		t.Fatalf("expected sector source list 200, got %d body=%s", sourceListRR.Code, sourceListRR.Body.String())
	}
	var sourceEnvelope struct {
		Data model.AStockSectorFundFlowListResult `json:"data"`
	}
	if err := json.Unmarshal(sourceListRR.Body.Bytes(), &sourceEnvelope); err != nil {
		t.Fatalf("decode sector source list: %v", err)
	}
	if sourceEnvelope.Data.Total != 1 || sourceEnvelope.Data.Items[0].SourceType != "eastmoney" || sourceEnvelope.Data.Items[0].Name != "电机" {
		t.Fatalf("unexpected sector source list: %+v", sourceEnvelope.Data)
	}

	stockPayload := `{"date":"2026-07-02","indicator":"今日","source_type":"sina","replace":true,"items":[{"rank":1,"code":"sz300502","name":"新易盛","price":520,"main_net_inflow":200,"field_counts_json":"{\"price\":1,\"main_net_inflow\":1}"}]}`
	stockPostReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/stock-fund-flow-sources", strings.NewReader(stockPayload))
	stockPostReq.Header.Set("Content-Type", "application/json")
	stockPostRR := httptest.NewRecorder()
	router.ServeHTTP(stockPostRR, stockPostReq)
	if stockPostRR.Code != http.StatusOK {
		t.Fatalf("expected stock fund flow source upsert 200, got %d body=%s", stockPostRR.Code, stockPostRR.Body.String())
	}
	stockListReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/stock-fund-flows?date=2026-07-02&indicator=今日&codes=sz300502,688981&page=1&page_size=10", nil)
	stockListRR := httptest.NewRecorder()
	router.ServeHTTP(stockListRR, stockListReq)
	if stockListRR.Code != http.StatusOK {
		t.Fatalf("expected stock fund flow list 200, got %d body=%s", stockListRR.Code, stockListRR.Body.String())
	}
	var stockEnvelope struct {
		Data model.AStockStockFundFlowListResult `json:"data"`
	}
	if err := json.Unmarshal(stockListRR.Body.Bytes(), &stockEnvelope); err != nil {
		t.Fatalf("decode stock fund flow list: %v", err)
	}
	if stockEnvelope.Data.Total != 1 || stockEnvelope.Data.Items[0].Code != "300502" || stockEnvelope.Data.Items[0].SourceCount != 1 {
		t.Fatalf("unexpected stock fund flow list: %+v", stockEnvelope.Data)
	}

	constituentPayload := `{"sector_type":"行业资金流","sector_name":"石油石化","replace":true,"items":[{"code":"sh600028","name":"中国石化","source":"eastmoney"},{"code":"601857","name":"中国石油","source":"eastmoney"}]}`
	constituentPostReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/sector-constituents", strings.NewReader(constituentPayload))
	constituentPostReq.Header.Set("Content-Type", "application/json")
	constituentPostRR := httptest.NewRecorder()
	router.ServeHTTP(constituentPostRR, constituentPostReq)
	if constituentPostRR.Code != http.StatusOK {
		t.Fatalf("expected sector constituent upsert 200, got %d body=%s", constituentPostRR.Code, constituentPostRR.Body.String())
	}
	constituentListReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/sector-constituents?sector_type=行业资金流&sector_name=石油石化&keyword=中国&limit=1", nil)
	constituentListRR := httptest.NewRecorder()
	router.ServeHTTP(constituentListRR, constituentListReq)
	if constituentListRR.Code != http.StatusOK {
		t.Fatalf("expected sector constituent list 200, got %d body=%s", constituentListRR.Code, constituentListRR.Body.String())
	}
	var constituentEnvelope struct {
		Data model.AStockSectorConstituentListResult `json:"data"`
	}
	if err := json.Unmarshal(constituentListRR.Body.Bytes(), &constituentEnvelope); err != nil {
		t.Fatalf("decode sector constituent list: %v", err)
	}
	if constituentEnvelope.Data.Total != 2 || len(constituentEnvelope.Data.Items) != 1 || constituentEnvelope.Data.Items[0].Code != "600028" {
		t.Fatalf("unexpected constituent list: %+v", constituentEnvelope.Data)
	}

	for _, date := range []string{"2026-07-03", "2026-07-04"} {
		sectorPayload := fmt.Sprintf(`{"date":"%s","sector_type":"行业资金流","indicator":"今日","replace":true,"items":[{"rank":1,"name":"石油石化","main_net_inflow":300}]}`, date)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/sector-fund-flows", strings.NewReader(sectorPayload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected sector trend seed 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		stockPayload := fmt.Sprintf(`{"date":"%s","indicator":"今日","replace":false,"items":[{"rank":1,"code":"600028","name":"中国石化","main_net_inflow":400}]}`, date)
		stockReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/stock-fund-flows", strings.NewReader(stockPayload))
		stockReq.Header.Set("Content-Type", "application/json")
		stockRR := httptest.NewRecorder()
		router.ServeHTTP(stockRR, stockReq)
		if stockRR.Code != http.StatusOK {
			t.Fatalf("expected stock trend seed 200, got %d body=%s", stockRR.Code, stockRR.Body.String())
		}
	}
	sectorTrendReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/sector-fund-flow-trend?end_date=2026-07-04&sector_type=行业资金流&sector_name=石油石化&indicator=今日&days=5", nil)
	sectorTrendRR := httptest.NewRecorder()
	router.ServeHTTP(sectorTrendRR, sectorTrendReq)
	if sectorTrendRR.Code != http.StatusOK {
		t.Fatalf("expected sector trend 200, got %d body=%s", sectorTrendRR.Code, sectorTrendRR.Body.String())
	}
	var sectorTrendEnvelope struct {
		Data model.AStockSectorFundFlowTrendResult `json:"data"`
	}
	if err := json.Unmarshal(sectorTrendRR.Body.Bytes(), &sectorTrendEnvelope); err != nil {
		t.Fatalf("decode sector trend: %v", err)
	}
	if sectorTrendEnvelope.Data.Total != 2 || sectorTrendEnvelope.Data.Items[0].TradeDate != "2026-07-04" {
		t.Fatalf("unexpected sector trend: %+v", sectorTrendEnvelope.Data)
	}
	stockTrendReq := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/stock-fund-flow-trend?end_date=2026-07-04&code=sh600028&indicator=今日&days=5", nil)
	stockTrendRR := httptest.NewRecorder()
	router.ServeHTTP(stockTrendRR, stockTrendReq)
	if stockTrendRR.Code != http.StatusOK {
		t.Fatalf("expected stock trend 200, got %d body=%s", stockTrendRR.Code, stockTrendRR.Body.String())
	}
	var stockTrendEnvelope struct {
		Data model.AStockStockFundFlowTrendResult `json:"data"`
	}
	if err := json.Unmarshal(stockTrendRR.Body.Bytes(), &stockTrendEnvelope); err != nil {
		t.Fatalf("decode stock trend: %v", err)
	}
	if stockTrendEnvelope.Data.Total != 2 || stockTrendEnvelope.Data.Code != "600028" || stockTrendEnvelope.Data.Items[0].TradeDate != "2026-07-04" {
		t.Fatalf("unexpected stock trend: %+v", stockTrendEnvelope.Data)
	}
}

func TestStockInstitutionHoldingSignalsAPI(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"items":[
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20251231","holder_name":"易方达基金","holder_type":"基金","holder_code":"old-1","shares":1000,"float_ratio":1,"market_value":10000,"source_type":"stock_institute_hold_detail"},
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"易方达基金","holder_type":"基金","holder_code":"old-1","shares":1200,"float_ratio":1.2,"market_value":12000,"source_type":"stock_institute_hold_detail"},
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"南方基金","holder_type":"基金","holder_code":"new-1","shares":1000,"float_ratio":0.8,"market_value":20000,"source_type":"stock_institute_hold_detail"},
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"嘉实基金","holder_type":"基金","holder_code":"new-2","shares":1000,"float_ratio":0.8,"market_value":19000,"source_type":"stock_institute_hold_detail"},
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"社保基金一一八组合","holder_type":"社保基金","holder_code":"new-3","shares":1000,"float_ratio":1.1,"market_value":50000,"source_type":"stock_institute_hold_detail"},
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"QFII Alpha","holder_type":"QFII","holder_code":"new-4","shares":1000,"float_ratio":0.7,"market_value":18000,"source_type":"stock_institute_hold_detail"},
		{"stock_code":"002230","stock_name":"科大讯飞","report_period":"20260331","holder_name":"保险资管","holder_type":"保险","holder_code":"new-5","shares":1000,"float_ratio":0.4,"market_value":17000,"source_type":"stock_institute_hold_detail"}
	]}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/a-stock/holdings/batch", strings.NewReader(payload))
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected holdings upsert 200, got %d body=%s", postRR.Code, postRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/a-stock/holdings/signals?code=SH.002230&page=1&page_size=20", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected holdings signals 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data model.StockInstitutionHoldingSignalListResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal holdings signals: %v", err)
	}
	if envelope.Data.Total != 1 || envelope.Data.CurrentPeriod != "20260331" || envelope.Data.PreviousPeriod != "20251231" {
		t.Fatalf("unexpected holdings signals metadata: %+v", envelope.Data)
	}
	signal := envelope.Data.Items[0]
	if signal.StockCode != "002230" || signal.HolderCountChange != 5 || signal.FloatRatioChange != 4 {
		t.Fatalf("unexpected holdings signal payload: %+v", signal)
	}
}

func TestAuditMiddlewareWritesSanitizedAccessLog(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects?token=secret&keyword=alpha", nil)
	req.Header.Set("X-User-ID", "42")
	req.Header.Set("X-User-Name", "auditor")
	req.Header.Set("User-Agent", "audit-test")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected projects 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	logs, err := store.ListAuditLogs(context.Background(), 5, 42, "http.get")
	if err != nil {
		t.Fatalf("ListAuditLogs error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one audit log, got %+v", logs)
	}
	if logs[0].Resource != "/api/v1/projects" || !strings.Contains(logs[0].DetailJSON, `"status":200`) {
		t.Fatalf("unexpected audit log: %+v", logs[0])
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(logs[0].DetailJSON), &detail); err != nil {
		t.Fatalf("unmarshal audit detail: %v", err)
	}
	query, _ := detail["query"].(string)
	if strings.Contains(query, "secret") || !strings.Contains(query, "token=<redacted>") {
		t.Fatalf("expected sanitized query, got %s", query)
	}
}

func TestOperationsAndAlertsAPI(t *testing.T) {
	ctx := context.Background()
	store := newContentSearchTestStore(t)
	started := time.Now().UTC().Add(-time.Minute)
	finished := time.Now().UTC()
	if err := store.RecordTaskRun(ctx, "release-check", "failed", "boom", started, &finished); err != nil {
		t.Fatalf("RecordTaskRun error: %v", err)
	}
	if err := store.RecordTaskRun(ctx, "release-check:smoke", "failed", "still broken", started.Add(time.Second), &finished); err != nil {
		t.Fatalf("RecordTaskRun second error: %v", err)
	}
	if _, err := store.CreateAuditLog(ctx, model.AuditLog{Username: "ops", Action: "ops.test", Resource: "/ops", DetailJSON: `{}`}); err != nil {
		t.Fatalf("CreateAuditLog error: %v", err)
	}
	crawlStarted := time.Now().UTC().Add(-7 * time.Hour)
	crawlFinished := crawlStarted.Add(time.Minute)
	crawlRunID, err := store.StartCrawlRun(ctx, "crypto_x", crawlStarted)
	if err != nil {
		t.Fatalf("StartCrawlRun error: %v", err)
	}
	if err := store.FinishCrawlRun(ctx, crawlRunID, "success", 5, 0, 0, "", crawlFinished); err != nil {
		t.Fatalf("FinishCrawlRun error: %v", err)
	}
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/healthy", "/api/v1/nlp/capabilities":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"status":"ok"}}`))
		case "/api/v1/scheduler/jobs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":[{"name":"analysis-refresh","java_quartz_name":"AnalysisQuartz","cron":"0 0/2 * * * ?","enabled":true,"last_status":"success"}]}`))
		default:
			http.Error(w, http.StatusText(http.StatusGone), http.StatusGone)
		}
	}))
	defer healthy.Close()

	svc := NewService(config.Config{
		DatabasePath:   filepath.Join(t.TempDir(), "yuqing.db"),
		GatewayWebURL:  healthy.URL,
		AuthURL:        healthy.URL,
		WechatURL:      healthy.URL,
		ContentURL:     healthy.URL,
		CrawlerURL:     healthy.URL,
		AnalysisURL:    healthy.URL,
		NLPURL:         healthy.URL,
		SchedulerURL:   healthy.URL,
		ReleaseURL:     healthy.URL,
		CryptoXURL:     healthy.URL + "/crypto-x",
		BinanceBaseURL: "https://binance.example.com",
		CoinLoreURL:    "https://coinlore.example.com",
		CoinGeckoURL:   "https://coingecko.example.com",
		HTTPTimeout:    time.Second,
	}, store)
	router := svc.Router()

	opsReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/operations", nil)
	opsRR := httptest.NewRecorder()
	router.ServeHTTP(opsRR, opsReq)
	if opsRR.Code != http.StatusOK {
		t.Fatalf("expected operations 200, got %d body=%s", opsRR.Code, opsRR.Body.String())
	}
	var opsEnvelope struct {
		Data model.OperationsSummary `json:"data"`
	}
	if err := json.Unmarshal(opsRR.Body.Bytes(), &opsEnvelope); err != nil {
		t.Fatalf("unmarshal operations: %v", err)
	}
	if len(opsEnvelope.Data.Services) != 9 || len(opsEnvelope.Data.FailedTaskRuns) != 2 {
		t.Fatalf("unexpected operations summary: %+v", opsEnvelope.Data)
	}
	serviceStatuses := map[string]model.OperationServiceStatus{}
	for _, service := range opsEnvelope.Data.Services {
		serviceStatuses[service.Name] = service
	}
	if !serviceStatuses["gateway-web"].Healthy || serviceStatuses["gateway-web"].Status != "ok" {
		t.Fatalf("expected gateway-web healthy ok status, got %+v", serviceStatuses["gateway-web"])
	}
	if !serviceStatuses["release-service"].Healthy || serviceStatuses["release-service"].Status != "ok" {
		t.Fatalf("expected release-service healthy ok status, got %+v", serviceStatuses["release-service"])
	}
	if len(opsEnvelope.Data.SchedulerJobs) != 1 || opsEnvelope.Data.SchedulerJobs[0].Name != "analysis-refresh" {
		t.Fatalf("expected scheduler job summary, got %+v", opsEnvelope.Data.SchedulerJobs)
	}
	if opsEnvelope.Data.TaskSummary.ConsecutiveFailures != 2 {
		t.Fatalf("expected two consecutive failures, got %+v", opsEnvelope.Data.TaskSummary)
	}
	if len(opsEnvelope.Data.LegacyRouteProbes) == 0 || opsEnvelope.Data.LegacyRouteProbes[0].Status != "gone" {
		t.Fatalf("expected legacy 410 probes, got %+v", opsEnvelope.Data.LegacyRouteProbes)
	}
	if opsEnvelope.Data.LegacyRegistry[2].Strategy != "preserve" || opsEnvelope.Data.LegacyRegistry[2].Count != 0 {
		t.Fatalf("expected preserve legacy count to be zero, got %+v", opsEnvelope.Data.LegacyRegistry)
	}

	alertReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/alerts", nil)
	alertRR := httptest.NewRecorder()
	router.ServeHTTP(alertRR, alertReq)
	if alertRR.Code != http.StatusOK || !strings.Contains(alertRR.Body.String(), "failed_task_runs") || !strings.Contains(alertRR.Body.String(), "consecutive_task_failures") || !strings.Contains(alertRR.Body.String(), "crypto_social_no_recent_insert") {
		t.Fatalf("expected failed task alert, got status=%d body=%s", alertRR.Code, alertRR.Body.String())
	}

	healthyReq := httptest.NewRequest(http.MethodGet, "/healthy", nil)
	healthyRR := httptest.NewRecorder()
	router.ServeHTTP(healthyRR, healthyReq)
	if healthyRR.Code != http.StatusOK || !strings.Contains(healthyRR.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected healthy endpoint response, got status=%d body=%s", healthyRR.Code, healthyRR.Body.String())
	}
}

func TestAndroidBootstrapModulesAndDashboard(t *testing.T) {
	ctx := context.Background()
	store := newContentSearchTestStore(t)
	if _, err := store.CreateProject(ctx, model.Project{Name: "mobile project", Keywords: "android", Status: "active"}); err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if _, err := store.CreateMonitorRule(ctx, model.MonitorRule{ProjectID: 1, Name: "mobile warning", IncludeKeywords: "android", Status: "active"}); err != nil {
		t.Fatalf("CreateMonitorRule error: %v", err)
	}
	if _, err := store.CreateReport(ctx, model.Report{ProjectID: 1, Title: "mobile report", Content: "content", Status: "generated"}); err != nil {
		t.Fatalf("CreateReport error: %v", err)
	}
	now := time.Date(2026, 6, 23, 3, 50, 0, 0, time.UTC)
	if _, _, err := store.UpsertItems(ctx, []model.Item{{
		SourceType: "headline",
		SourceKey:  "android-1",
		Title:      "Android article",
		Content:    "content",
		Summary:    "summary",
		SourceURL:  "https://example.com/android",
		CapturedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	if _, _, err := store.UpsertItems(ctx, []model.Item{
		{
			SourceType:  "flash",
			SourceKey:   "android-recrawled-old",
			Title:       "旧快讯",
			Content:     "content",
			Summary:     "summary",
			SourceURL:   "https://example.com/old",
			PublishTime: "2026-06-19 16:57:53",
			CapturedAt:  time.Date(2026, 6, 23, 4, 9, 16, 0, time.UTC),
			CreatedAt:   time.Date(2026, 6, 19, 8, 58, 21, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 6, 23, 4, 9, 16, 0, time.UTC),
		},
		{
			SourceType:  "panews_newsflash",
			SourceKey:   "android-latest-news",
			Title:       "最新新闻",
			Content:     "content",
			Summary:     "summary",
			SourceURL:   "https://example.com/latest",
			PublishTime: "2026-06-23T03:58:00Z",
			CapturedAt:  time.Date(2026, 6, 23, 4, 0, 0, 0, time.UTC),
			CreatedAt:   time.Date(2026, 6, 23, 4, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 6, 23, 4, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}
	svc := NewService(config.Config{HTTPTimeout: time.Second}, store)
	router := svc.Router()

	bootstrapReq := httptest.NewRequest(http.MethodGet, "/api/v1/android/bootstrap", nil)
	bootstrapReq.Header.Set("X-User-ID", "42")
	bootstrapReq.Header.Set("X-User-Name", "admin")
	bootstrapRR := httptest.NewRecorder()
	router.ServeHTTP(bootstrapRR, bootstrapReq)
	if bootstrapRR.Code != http.StatusOK {
		t.Fatalf("expected bootstrap 200, got %d body=%s", bootstrapRR.Code, bootstrapRR.Body.String())
	}
	var bootstrapEnvelope struct {
		Data androidBootstrap `json:"data"`
	}
	if err := json.Unmarshal(bootstrapRR.Body.Bytes(), &bootstrapEnvelope); err != nil {
		t.Fatalf("unmarshal bootstrap: %v", err)
	}
	if bootstrapEnvelope.Data.PackageName != androidPackageName || bootstrapEnvelope.Data.User.ID != 42 {
		t.Fatalf("unexpected bootstrap payload: %+v", bootstrapEnvelope.Data)
	}
	if len(bootstrapEnvelope.Data.Modules) < 10 || len(bootstrapEnvelope.Data.Actions) == 0 {
		t.Fatalf("expected full portal modules and actions, got %+v", bootstrapEnvelope.Data)
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/api/v1/android/dashboard", nil)
	dashboardRR := httptest.NewRecorder()
	router.ServeHTTP(dashboardRR, dashboardReq)
	if dashboardRR.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d body=%s", dashboardRR.Code, dashboardRR.Body.String())
	}
	var dashboardEnvelope struct {
		Data androidDashboard `json:"data"`
	}
	if err := json.Unmarshal(dashboardRR.Body.Bytes(), &dashboardEnvelope); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}
	if dashboardEnvelope.Data.Overview.ArticleCount != 3 || dashboardEnvelope.Data.Overview.ProjectCount != 1 || dashboardEnvelope.Data.Overview.ReportCount != 1 {
		t.Fatalf("unexpected dashboard overview: %+v", dashboardEnvelope.Data.Overview)
	}
	if len(dashboardEnvelope.Data.Articles.Items) != 3 || len(dashboardEnvelope.Data.Projects) != 1 {
		t.Fatalf("expected dashboard lists, got %+v", dashboardEnvelope.Data)
	}
	if dashboardEnvelope.Data.Articles.Items[0].SourceKey != "android-recrawled-old" {
		t.Fatalf("expected android dashboard to show latest synced news first, got %+v", dashboardEnvelope.Data.Articles.Items)
	}
}

func TestAndroidActionProxiesSchedulerWithServiceToken(t *testing.T) {
	store := newContentSearchTestStore(t)
	var gotToken string
	var gotPath string
	var gotQuery string
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Service-Token")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "triggered"})
	}))
	defer scheduler.Close()

	svc := NewService(config.Config{
		ServiceToken: "secret",
		SchedulerURL: scheduler.URL,
		HTTPTimeout:  time.Second,
	}, store)
	router := svc.Router()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/android/actions/stock_research_backfill", strings.NewReader(`{"params":{"code":"002230","start":"2026-06-01","end":"2026-06-18"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected action 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if gotToken != "secret" || gotPath != "/api/v1/scheduler/stock-research/backfill" {
		t.Fatalf("unexpected proxied request token=%q path=%q", gotToken, gotPath)
	}
	if !strings.Contains(gotQuery, "code=002230") || !strings.Contains(gotQuery, "start=2026-06-01") || !strings.Contains(gotQuery, "end=2026-06-18") {
		t.Fatalf("unexpected proxied query: %s", gotQuery)
	}
}

func TestDatabaseConfigAPI(t *testing.T) {
	store := newContentSearchTestStore(t)
	dbPath := filepath.Join(t.TempDir(), "yuqing.db")
	svc := NewService(config.Config{
		DatabaseDriver:   "sqlite",
		DatabasePath:     dbPath,
		DatabaseURL:      "postgres://dbuser:secret@127.0.0.1:5432/yuqing?sslmode=disable",
		PostgresHost:     "127.0.0.1",
		PostgresPort:     "5432",
		PostgresDatabase: "yuqing",
		PostgresUser:     "dbuser",
		PostgresSSLMode:  "disable",
		HTTPTimeout:      time.Second,
	}, store)
	router := svc.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/database-config", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected database config 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret") || !strings.Contains(rr.Body.String(), "redacted") || !strings.Contains(rr.Body.String(), `"driver":"sqlite"`) {
		t.Fatalf("expected masked sqlite database config, got %s", rr.Body.String())
	}

	missingSvc := NewService(config.Config{DatabaseDriver: "sqlite", DatabasePath: dbPath, HTTPTimeout: time.Second}, store)
	missingRouter := missingSvc.Router()
	checkReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/database-config/check", strings.NewReader(`{"driver":"postgres"}`))
	checkReq.Header.Set("Content-Type", "application/json")
	checkRR := httptest.NewRecorder()
	missingRouter.ServeHTTP(checkRR, checkReq)
	if checkRR.Code != http.StatusOK || !strings.Contains(checkRR.Body.String(), "PostgreSQL config is incomplete") {
		t.Fatalf("expected incomplete postgres config warning, got status=%d body=%s", checkRR.Code, checkRR.Body.String())
	}
}

func TestReleaseSettingsAPISavesServerPublishConfig(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{HTTPTimeout: time.Second}, store)
	router := svc.Router()

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/release-settings", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected release settings 200, got status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `C:\\yuqing\\release`) || !strings.Contains(getRR.Body.String(), `D:\\yuqing\\release`) {
		t.Fatalf("expected default release directories, got %s", getRR.Body.String())
	}

	saveReq := httptest.NewRequest(http.MethodPut, "/api/v1/system/release-settings", strings.NewReader(`{"release_addr":":8100","release_url":"http://10.15.0.7:8100/","release_dir":"C:\\yuqing\\release2","dev_release_dir":"D:\\yuqing\\release2","server_share_path":"\\\\10.15.0.7\\yuqing-release2\\","server_user":"10.15.0.7\\hyuser"}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRR := httptest.NewRecorder()
	router.ServeHTTP(saveRR, saveReq)
	if saveRR.Code != http.StatusOK {
		t.Fatalf("expected release settings save 200, got status=%d body=%s", saveRR.Code, saveRR.Body.String())
	}
	body := saveRR.Body.String()
	if !strings.Contains(body, `"release_addr":":8100"`) || !strings.Contains(body, `"release_url":"http://10.15.0.7:8100"`) || !strings.Contains(body, `\\\\10.15.0.7\\yuqing-release2`) {
		t.Fatalf("unexpected release settings response: %s", body)
	}
}

func TestAStockRecommendationAlgorithmSettingsAPI(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{HTTPTimeout: time.Second}, store)
	router := svc.Router()

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/a-stock-recommendation-algorithm", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK || !strings.Contains(getRR.Body.String(), `"recommendation_limit":5`) || !strings.Contains(getRR.Body.String(), `"extreme_score":50`) {
		t.Fatalf("expected default algorithm settings, got status=%d body=%s", getRR.Code, getRR.Body.String())
	}

	settings := model.DefaultAStockRecommendationAlgorithmSettings()
	settings.Auction.RecommendationLimit = 4
	settings.Fund.ExtremeScore = 45
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	saveReq := httptest.NewRequest(http.MethodPut, "/api/v1/system/a-stock-recommendation-algorithm", strings.NewReader(string(raw)))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRR := httptest.NewRecorder()
	router.ServeHTTP(saveRR, saveReq)
	if saveRR.Code != http.StatusOK || !strings.Contains(saveRR.Body.String(), `"recommendation_limit":4`) || !strings.Contains(saveRR.Body.String(), `"extreme_score":45`) {
		t.Fatalf("expected saved algorithm settings, got status=%d body=%s", saveRR.Code, saveRR.Body.String())
	}

	invalid := settings
	invalid.Fund.ScoreMin = 10
	invalid.Fund.ScoreMax = -10
	raw, err = json.Marshal(invalid)
	if err != nil {
		t.Fatalf("marshal invalid settings: %v", err)
	}
	invalidReq := httptest.NewRequest(http.MethodPut, "/api/v1/system/a-stock-recommendation-algorithm", strings.NewReader(string(raw)))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRR := httptest.NewRecorder()
	router.ServeHTTP(invalidRR, invalidReq)
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid settings 400, got status=%d body=%s", invalidRR.Code, invalidRR.Body.String())
	}
}

func TestDatabaseSwitchAPISavesRuntimeConfig(t *testing.T) {
	store := newContentSearchTestStore(t)
	restartSubmitted := false
	previousRestartAll := startAllServicesRestart
	startAllServicesRestart = func() error {
		restartSubmitted = true
		return nil
	}
	t.Cleanup(func() { startAllServicesRestart = previousRestartAll })
	configPath := filepath.Join(t.TempDir(), "database-config.json")
	dbPath := filepath.Join(t.TempDir(), "local.db")
	svc := NewService(config.Config{
		DatabaseDriver:     "sqlite",
		DatabasePath:       dbPath,
		DatabaseConfigPath: configPath,
		PostgresHost:       "10.15.0.19",
		PostgresPort:       "5432",
		PostgresDatabase:   "yuqing",
		PostgresUser:       "admin",
		PostgresSSLMode:    "disable",
		HTTPTimeout:        time.Second,
	}, store)
	router := svc.Router()

	switchReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/database-config/switch", strings.NewReader(`{"driver":"sqlite","sqlite_path":"data/local.db"}`))
	switchReq.Header.Set("Content-Type", "application/json")
	switchRR := httptest.NewRecorder()
	router.ServeHTTP(switchRR, switchReq)
	if switchRR.Code != http.StatusOK {
		t.Fatalf("expected database switch 200, got status=%d body=%s", switchRR.Code, switchRR.Body.String())
	}
	body := switchRR.Body.String()
	if !strings.Contains(body, `"configured_driver":"sqlite"`) || !strings.Contains(body, `"restart_required":true`) || !strings.Contains(body, "restarting all services") {
		t.Fatalf("expected switch response to include selected sqlite and restart hint, got %s", body)
	}
	if !restartSubmitted {
		t.Fatal("expected database switch to submit all-services restart")
	}
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected runtime database config file: %v", err)
	}
	if !strings.Contains(string(payload), `"driver": "sqlite"`) || !strings.Contains(string(payload), `"sqlite_path": "data/local.db"`) {
		t.Fatalf("unexpected runtime database config payload: %s", payload)
	}
}

func TestDatabaseSaveAPISavesRuntimeConfigWithoutRestart(t *testing.T) {
	store := newContentSearchTestStore(t)
	restartSubmitted := false
	previousRestartAll := startAllServicesRestart
	startAllServicesRestart = func() error {
		restartSubmitted = true
		return nil
	}
	t.Cleanup(func() { startAllServicesRestart = previousRestartAll })
	configPath := filepath.Join(t.TempDir(), "database-config.json")
	svc := NewService(config.Config{
		DatabaseDriver:     "sqlite",
		DatabasePath:       filepath.Join(t.TempDir(), "local.db"),
		DatabaseConfigPath: configPath,
		HTTPTimeout:        time.Second,
	}, store)
	router := svc.Router()

	saveReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/database-config/save", strings.NewReader(`{"driver":"postgres","postgres_host":"127.0.0.1","postgres_port":"5432","postgres_database":"yuqing","postgres_user":"postgres","postgres_password":"secret","postgres_sslmode":"disable","sqlite_path":"data/yuqing.db"}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRR := httptest.NewRecorder()
	router.ServeHTTP(saveRR, saveReq)
	if saveRR.Code != http.StatusOK {
		t.Fatalf("expected database save 200, got status=%d body=%s", saveRR.Code, saveRR.Body.String())
	}
	if restartSubmitted {
		t.Fatal("expected database save not to restart services")
	}
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected runtime database config file: %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"driver": "postgres"`) || !strings.Contains(body, `"postgres_password": "secret"`) {
		t.Fatalf("unexpected saved database config payload: %s", body)
	}
}

func TestRestartServiceAPIRequiresTokenAndSubmitsKnownService(t *testing.T) {
	store := newContentSearchTestStore(t)
	var restarted serviceRestartSpec
	previous := startServiceRestart
	startServiceRestart = func(spec serviceRestartSpec) error {
		restarted = spec
		return nil
	}
	t.Cleanup(func() { startServiceRestart = previous })

	svc := NewService(config.Config{
		ServiceToken: "secret",
		CrawlerAddr:  ":8083",
		HTTPTimeout:  time.Second,
	}, store)
	router := svc.Router()

	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/services/crawler-service/restart", nil)
	unauthorizedRR := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRR, unauthorizedReq)
	if unauthorizedRR.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized restart to be rejected, got %d", unauthorizedRR.Code)
	}

	unknownReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/services/unknown/restart", nil)
	unknownReq.Header.Set("X-Service-Token", "secret")
	unknownRR := httptest.NewRecorder()
	router.ServeHTTP(unknownRR, unknownReq)
	if unknownRR.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown service to be rejected, got %d", unknownRR.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/services/crawler-service/restart", nil)
	req.Header.Set("X-Service-Token", "secret")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected restart accepted, got %d body=%s", rr.Code, rr.Body.String())
	}
	if restarted.Name != "crawler-service" || restarted.Port != 8083 || restarted.Path != ".\\cmd\\crawler-service" {
		t.Fatalf("unexpected restart spec: %+v", restarted)
	}
	if !strings.HasSuffix(filepath.Clean(restarted.BinaryPath), filepath.Clean("bin/crawler-service.exe")) {
		t.Fatalf("expected crawler restart to prefer built binary, got %+v", restarted)
	}
	if !testIntSliceContains(restarted.Ports, 8083) {
		t.Fatalf("expected crawler restart ports to include 8083, got %+v", restarted.Ports)
	}

	gateway := NewService(config.Config{
		ServiceToken:        "secret",
		GatewayWebAddr:      ":8079",
		GatewayWebHTTPAddrs: []string{":8079", ":80"},
		HTTPTimeout:         time.Second,
	}, store)
	gatewayReq := httptest.NewRequest(http.MethodPost, "/api/v1/system/services/gateway-web/restart", nil)
	gatewayReq.Header.Set("X-Service-Token", "secret")
	gatewayRR := httptest.NewRecorder()
	gateway.Router().ServeHTTP(gatewayRR, gatewayReq)
	if gatewayRR.Code != http.StatusAccepted {
		t.Fatalf("expected gateway restart accepted, got %d body=%s", gatewayRR.Code, gatewayRR.Body.String())
	}
	if restarted.Name != "gateway-web" || restarted.Port != 8079 || restarted.Path != ".\\cmd\\gateway-web" {
		t.Fatalf("unexpected gateway restart spec: %+v", restarted)
	}
	if !strings.HasSuffix(filepath.Clean(restarted.BinaryPath), filepath.Clean("bin/gateway-web.exe")) {
		t.Fatalf("expected gateway restart to prefer built binary, got %+v", restarted)
	}
	if !testIntSliceContains(restarted.Ports, 8079) || !testIntSliceContains(restarted.Ports, 80) {
		t.Fatalf("expected gateway restart ports to include 8079 and 80, got %+v", restarted.Ports)
	}
}

func testIntSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestServiceLogsAPIRequiresTokenAndReadsTail(t *testing.T) {
	store := newContentSearchTestStore(t)
	root := t.TempDir()
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir error: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	if err := os.MkdirAll(filepath.Join(root, "runtime-logs"), 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	logPath := filepath.Join(root, "runtime-logs", "crawler-service.out.log")
	if err := os.WriteFile(logPath, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	svc := NewService(config.Config{
		ServiceToken: "secret",
		CrawlerAddr:  ":8083",
		HTTPTimeout:  time.Second,
	}, store)
	router := svc.Router()

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/api/v1/system/services/crawler-service/logs", nil)
	unauthorizedRR := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRR, unauthorizedReq)
	if unauthorizedRR.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized logs request to be rejected, got %d", unauthorizedRR.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/services/crawler-service/logs?lines=2", nil)
	req.Header.Set("X-Service-Token", "secret")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected logs 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"service":"crawler-service"`) || !strings.Contains(rr.Body.String(), "line2\\nline3") || strings.Contains(rr.Body.String(), "line1") {
		t.Fatalf("unexpected log body: %s", rr.Body.String())
	}
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

func TestCryptoNewsAndSocialFallbackMatchesETHInRawFields(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	now := time.Now().UTC()
	_, _, err := store.UpsertItems(context.Background(), []model.Item{
		{
			SourceType: "headline", SourceKey: "eth-raw-news", Title: "ETF flow watch",
			Summary: "institution desk update", Content: "approval chatter grows",
			SourceURL:  "https://example.com/eth-news",
			RawPayload: `{"symbols":["ETHUSDT"],"asset":"Ethereum","cn":"以太坊"}`,
			CapturedAt: now.Add(-30 * time.Minute), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			SourceType: "foresight_newsflash", SourceKey: "eth-foresight-tags", Title: "链上资金异动",
			Summary: "资金流入增强", Content: "巨鲸买入推动市场情绪回暖",
			SourceURL:  "https://foresightnews.pro/news/detail/1",
			TagFlags:   "以太坊/ETH",
			RawPayload: `{"tags":[{"name":"ETH"}],"wikis":[{"symbol":"ETHUSDT"}]}`,
			CapturedAt: now.Add(-25 * time.Minute), CreatedAt: now.Add(-25 * time.Minute), UpdatedAt: now.Add(-25 * time.Minute),
		},
		{
			SourceType: "coindesk_zh_latest", SourceKey: "eth-coindesk-title", Title: "华尔街正逐步深入布局以太坊",
			Summary: "ETH 基础设施已基本建立", Content: "采用规模扩大",
			SourceURL:  "https://www.coindesk.com/zh/markets/2026/06/15/ethereum-wall-street",
			CapturedAt: now.Add(-22 * time.Minute), CreatedAt: now.Add(-22 * time.Minute), UpdatedAt: now.Add(-22 * time.Minute),
		},
		{
			SourceType: "panews_newsflash", SourceKey: "eth-panews-rss", Title: "巨鲸以5倍杠杆开设以太坊多单",
			Summary: "PANews 快讯", Content: "链上资金流入增强",
			SourceURL:  "https://www.panewslab.com/zh/articles/eth-long",
			RawPayload: `{"guid":"eth-long","description":"ETH 多头仓位"}`,
			CapturedAt: now.Add(-18 * time.Minute), CreatedAt: now.Add(-18 * time.Minute), UpdatedAt: now.Add(-18 * time.Minute),
		},
		{
			SourceType: "crypto_x", SourceKey: "eth-raw-social", Title: "Whale transfer alert",
			Content: "large wallet movement with bullish sentiment", Summary: "on-chain alert",
			SourceURL: "https://x.com/example/eth", ExternalSourceHost: "x.com", FromText: "@onchain",
			TagFlags:   "ETH,USDT",
			RawPayload: `{"tickers":["ETHUSDT"],"message":"Ethereum whale inflow"}`,
			CapturedAt: now.Add(-20 * time.Minute), CreatedAt: now.Add(-20 * time.Minute), UpdatedAt: now.Add(-20 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	newsReq := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/news?pair=eth&page=1&page_size=10", nil)
	newsRR := httptest.NewRecorder()
	svc.handleCryptoNews(newsRR, newsReq)
	if newsRR.Code != http.StatusOK {
		t.Fatalf("expected news 200, got %d body=%s", newsRR.Code, newsRR.Body.String())
	}
	var newsEnvelope struct {
		Data model.CryptoNewsResult `json:"data"`
	}
	if err := json.Unmarshal(newsRR.Body.Bytes(), &newsEnvelope); err != nil {
		t.Fatalf("unmarshal crypto news response: %v", err)
	}
	if newsEnvelope.Data.Resolution.Pair != "ETHUSDT" || newsEnvelope.Data.Total < 4 {
		t.Fatalf("expected ETH fallback news match, got %+v", newsEnvelope.Data)
	}

	socialReq := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/social?pair=eth&page=1&page_size=10", nil)
	socialRR := httptest.NewRecorder()
	svc.handleCryptoSocial(socialRR, socialReq)
	if socialRR.Code != http.StatusOK {
		t.Fatalf("expected social 200, got %d body=%s", socialRR.Code, socialRR.Body.String())
	}
	var socialEnvelope struct {
		Data model.CryptoSocialResult `json:"data"`
	}
	if err := json.Unmarshal(socialRR.Body.Bytes(), &socialEnvelope); err != nil {
		t.Fatalf("unmarshal crypto social response: %v", err)
	}
	if socialEnvelope.Data.Resolution.Pair != "ETHUSDT" || socialEnvelope.Data.Total != 1 {
		t.Fatalf("expected one ETH fallback social match, got %+v", socialEnvelope.Data)
	}
	if socialEnvelope.Data.Items[0].Platform != "X" {
		t.Fatalf("expected X platform fallback match, got %+v", socialEnvelope.Data.Items[0])
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

func TestHotspotSwitchingBuildsRisingCoolingAndSwitch(t *testing.T) {
	ctx := context.Background()
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, loc)
	items := make([]model.Item, 0)
	addItem := func(key string, dayOffset int, title, content string) {
		ts := now.AddDate(0, 0, dayOffset)
		items = append(items, model.Item{
			SourceType: "headline",
			SourceKey:  key,
			Title:      title,
			Content:    content,
			Summary:    content,
			SourceURL:  "https://example.com/" + key,
			CapturedAt: ts,
			CreatedAt:  ts,
			UpdatedAt:  ts,
		})
	}
	for i, offset := range []int{-13, -12, -11, -10} {
		addItem(fmt.Sprintf("gold-prev-%d", i), offset, fmt.Sprintf("黄金%d", i), "黄金 金价 贵金属")
	}
	for i, offset := range []int{-4, -3, -2, -1, 0} {
		addItem(fmt.Sprintf("ai-recent-%d", i), offset, fmt.Sprintf("AI%d", i), "人工智能 大模型 生成式AI")
	}
	for i, offset := range []int{-1, 0} {
		addItem(fmt.Sprintf("robot-new-%d", i), offset, fmt.Sprintf("机器人%d", i), "人形机器人 具身智能")
	}
	if _, _, err := store.UpsertItems(ctx, items); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	result, err := svc.buildHotspotSwitching(ctx, 14, now)
	if err != nil {
		t.Fatalf("buildHotspotSwitching error: %v", err)
	}
	if result.Days != 14 || result.TotalArticles != len(items) {
		t.Fatalf("unexpected hotspot window summary: %+v", result)
	}
	if got := hotspotItemByKeyword(result.Rising, "AI"); got == nil || got.CountRecent7D != 5 || got.CountPrev7D != 0 {
		t.Fatalf("expected AI to be rising from recent articles, got %+v", got)
	}
	if got := hotspotItemByKeyword(result.Cooling, "黄金"); got == nil || got.CountPrev7D != 4 || got.CountRecent7D != 0 {
		t.Fatalf("expected gold to be cooling from previous window, got %+v", got)
	}
	if got := hotspotItemByKeyword(result.New, "机器人"); got == nil || got.CountRecent7D != 2 {
		t.Fatalf("expected robot to be new recent hotspot, got %+v", got)
	}
	if len(result.Switches) == 0 || result.Switches[0].From != "黄金" || result.Switches[0].To != "AI" {
		t.Fatalf("expected hotspot switch from gold to AI, got %+v", result.Switches)
	}
}

func TestExtractArticleHotspotKeywordsNormalizesMarketMoveFragments(t *testing.T) {
	keywords := extractArticleHotspotKeywords(model.Item{Title: "新叶股份盘中快速上涨，5分钟内涨幅达2%，AI机器人活跃"})
	for _, want := range []string{"涨幅", "AI", "机器人"} {
		if _, ok := keywords[want]; !ok {
			t.Fatalf("expected keyword %q in %+v", want, keywords)
		}
	}
	for _, bad := range []string{"5分钟内涨幅达2", "涨幅达2", "幅达2", "达2"} {
		if _, ok := keywords[bad]; ok {
			t.Fatalf("did not expect numeric fragment %q in %+v", bad, keywords)
		}
	}

	keywords = extractArticleHotspotKeywords(model.Item{Title: "风电板块回调，跌幅超过5%，资金观望"})
	if _, ok := keywords["跌幅"]; !ok {
		t.Fatalf("expected 跌幅 keyword in %+v", keywords)
	}
	for _, bad := range []string{"跌幅超过5", "幅超过5", "达5"} {
		if _, ok := keywords[bad]; ok {
			t.Fatalf("did not expect numeric fragment %q in %+v", bad, keywords)
		}
	}
}

func TestNormalizeHotspotTitleTokenHandlesMarketMoveFragments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "simple rise phrase", in: "涨幅", want: "涨幅", ok: true},
		{name: "simple fall phrase", in: "跌幅", want: "跌幅", ok: true},
		{name: "rise numeric phrase", in: "涨幅达2", want: "涨幅", ok: true},
		{name: "fall numeric phrase", in: "跌幅超过5", want: "跌幅", ok: true},
		{name: "rise phrase in longer token", in: "5分钟内涨幅达2", want: "涨幅", ok: true},
		{name: "broken suffix fragment", in: "幅达2", ok: false},
		{name: "broken reach fragment", in: "达2", ok: false},
		{name: "leading time fragment", in: "5分钟内", ok: false},
		{name: "theme stays usable", in: "机器人", want: "机器人", ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeHotspotTitleToken(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("normalizeHotspotTitleToken(%q) = %q, %t; want %q, %t", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestHotspotSwitchingFiltersMarketMoveFragmentsFromRankings(t *testing.T) {
	ctx := context.Background()
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, loc)
	items := []model.Item{
		{
			SourceType: "headline",
			SourceKey:  "rise-fragment",
			Title:      "新叶股份盘中快速上涨5分钟内涨幅达2",
			SourceURL:  "https://example.com/rise-fragment",
			CapturedAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			SourceType: "headline",
			SourceKey:  "fall-fragment",
			Title:      "风电板块盘中回调跌幅达5",
			SourceURL:  "https://example.com/fall-fragment",
			CapturedAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	if _, _, err := store.UpsertItems(ctx, items); err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	result, err := svc.buildHotspotSwitching(ctx, 14, now)
	if err != nil {
		t.Fatalf("buildHotspotSwitching error: %v", err)
	}
	if got := hotspotItemByKeyword(result.TodayTop, "涨幅"); got == nil || got.TodayCount == 0 {
		t.Fatalf("expected normalized 涨幅 in today ranking, got %+v", result.TodayTop)
	}
	if got := hotspotItemByKeyword(result.TodayTop, "跌幅"); got == nil || got.TodayCount == 0 {
		t.Fatalf("expected normalized 跌幅 in today ranking, got %+v", result.TodayTop)
	}
	assertHotspotResultMissingKeywords(t, result, []string{"5分钟内涨幅达2", "涨幅达2", "幅达2", "达2", "跌幅达5", "幅达5", "达5"})
}

func TestHotspotSwitchingEndpointReturnsEmptyListsWithoutArticles(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hotspots/switching?days=14", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected hotspot endpoint 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data model.HotspotSwitchingResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal hotspot response: %v", err)
	}
	if envelope.Data.Days != 14 || len(envelope.Data.Top) != 0 || len(envelope.Data.Rising) != 0 || len(envelope.Data.Cooling) != 0 {
		t.Fatalf("expected empty hotspot lists, got %+v", envelope.Data)
	}
}

func hotspotItemByKeyword(items []model.HotspotSwitchingItem, keyword string) *model.HotspotSwitchingItem {
	for idx := range items {
		if items[idx].Keyword == keyword {
			return &items[idx]
		}
	}
	return nil
}

func assertHotspotResultMissingKeywords(t *testing.T, result model.HotspotSwitchingResult, badKeywords []string) {
	t.Helper()
	bad := make(map[string]struct{}, len(badKeywords))
	for _, keyword := range badKeywords {
		bad[keyword] = struct{}{}
	}
	collections := [][]model.HotspotSwitchingItem{
		result.TodayTop,
		result.Top,
		result.Rising,
		result.Cooling,
		result.New,
		result.ContinuousRising,
	}
	for _, collection := range collections {
		for _, item := range collection {
			if _, ok := bad[item.Keyword]; ok {
				t.Fatalf("did not expect numeric fragment keyword %q in hotspot result: %+v", item.Keyword, result)
			}
		}
	}
	for _, day := range result.Daily {
		for _, item := range day.Items {
			if _, ok := bad[item.Keyword]; ok {
				t.Fatalf("did not expect numeric fragment keyword %q in daily hotspot result: %+v", item.Keyword, result.Daily)
			}
		}
	}
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
