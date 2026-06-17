package portal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/nlp"
)

var testLastCrawlRequest struct {
	sync.Mutex
	templateID string
	sourceType string
	keyword    string
}

func TestUserIDFromMap(t *testing.T) {
	tests := []struct {
		name string
		user any
		want int64
	}{
		{name: "float64", user: map[string]any{"id": float64(7)}, want: 7},
		{name: "int64", user: map[string]any{"id": int64(9)}, want: 9},
		{name: "json number", user: map[string]any{"id": json.Number("11")}, want: 11},
		{name: "invalid type", user: map[string]any{"id": "abc"}, want: 0},
		{name: "not map", user: "abc", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := userIDFromMap(tc.user); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestNonEmpty(t *testing.T) {
	if got := nonEmpty("", "   ", " alpha ", "beta"); got != "alpha" {
		t.Fatalf("expected first non-empty trimmed value, got %q", got)
	}
	if got := nonEmpty("", "   "); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestArticleBodyTextSkipsTitleOnlyContent(t *testing.T) {
	item := model.Item{
		Title:   "金十图示：2026年06月16日（周二）亚盘市场行情",
		Summary: "金十图示：2026年06月16日（周二）亚盘市场行情",
		Content: "",
	}

	if got := articleBodyText(item); got != "" {
		t.Fatalf("expected empty body for title-only article, got %q", got)
	}

	item.Content = "真实正文内容"
	if got := articleBodyText(item); got != "真实正文内容" {
		t.Fatalf("expected real content, got %q", got)
	}
}

func TestMonitorCompatGetArticle(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/search/full":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{{
						"id":           101,
						"source_type":  "headline",
						"source_key":   "monitor-key-101",
						"title":        "钢铁行业快讯",
						"content":      "钢铁行业正文",
						"summary":      "钢铁行业摘要",
						"publish_time": "2026-06-10 10:00:00",
						"source_url":   "https://example.com/101",
						"from_text":    "金十数据",
						"tag_flags":    "1",
						"project_ids":  []int64{7},
					}},
					"total":     1,
					"page":      1,
					"page_size": 10,
				},
			})
		case "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
				"id":         7,
				"group_id":   3,
				"group_name": "钢铁组",
				"name":       "钢铁项目",
			}}})
		case "/api/v1/project-groups":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 3, "name": "钢铁组"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer content.Close()

	srv := &Server{
		cfg:       config.Config{ContentURL: content.URL},
		client:    resty.New(),
		templates: NewServer(config.Config{}).templates,
	}

	req := httptest.NewRequest(http.MethodPost, "/monitor/getarticle", strings.NewReader(`{"projectid":"7","searchkeyword":"钢铁","page":1,"pageSize":10}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleMonitorCompat(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Status int `json:"status"`
		Data   struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal monitor getarticle: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("unexpected monitor getarticle payload: %+v", envelope)
	}
	if len(envelope.Data.List) > 0 && (envelope.Data.List[0]["projectid"] != float64(7) || envelope.Data.List[0]["groupid"] != float64(3)) {
		t.Fatalf("expected project/group ids in payload, got %+v", envelope.Data.List[0])
	}
}

func TestMonitorCompatWarningSettingAndStatus(t *testing.T) {
	var statusCalls []string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 7, "group_id": 3, "group_name": "钢铁组", "name": "钢铁项目"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/warning-settings/7":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"project_id":         7,
				"warning_status":     1,
				"warning_word":       "钢铁",
				"warning_name":       "钢铁预警",
				"channels":           "mail",
				"enabled":            true,
				"warning_setting_id": 11,
			}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/articles/101/status":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			statusCalls = append(statusCalls, req["status"].(string))
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"status": req["status"]}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer content.Close()

	srv := &Server{
		cfg:       config.Config{ContentURL: content.URL},
		client:    resty.New(),
		templates: NewServer(config.Config{}).templates,
	}

	warnReq := httptest.NewRequest(http.MethodGet, "/monitor/warningSetting/7", nil)
	warnRR := httptest.NewRecorder()
	srv.handleMonitorCompat(warnRR, warnReq, map[string]any{"id": 1})
	if warnRR.Code != http.StatusOK {
		t.Fatalf("expected warning setting 200, got %d", warnRR.Code)
	}

	statusReq := httptest.NewRequest(http.MethodPost, "/monitor/edit/status", strings.NewReader(`{"articleId":"101","type":"1"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusRR := httptest.NewRecorder()
	srv.handleMonitorCompat(statusRR, statusReq, map[string]any{"id": 9})
	if statusRR.Code != http.StatusOK {
		t.Fatalf("expected edit status 200, got %d", statusRR.Code)
	}
	if len(statusCalls) != 1 || statusCalls[0] != "invalid" {
		t.Fatalf("expected invalid status call, got %+v", statusCalls)
	}
}

func TestCryptoPageUsesSharedNavAndFriendlyFallback(t *testing.T) {
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "binance unavailable", "data": nil})
	}))
	defer analysis.Close()

	srv := &Server{
		cfg:       config.Config{AnalysisURL: analysis.URL},
		client:    resty.New(),
		templates: NewServer(config.Config{}).templates,
	}

	req := httptest.NewRequest(http.MethodGet, "/crypto?pair=BTCUSDT", nil)
	rr := httptest.NewRecorder()
	srv.handleCryptoPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "500 Internal Server Error") {
		t.Fatalf("expected friendly fallback, got %s", body)
	}
	if !strings.Contains(body, "/crawl-templates/manage") || !strings.Contains(body, "/crypto") {
		t.Fatalf("expected shared nav links in crypto page, got %s", body)
	}
}

func TestAStockPageUsesSharedNavAndEmptyState(t *testing.T) {
	srv := NewServer(config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-15", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	aStockIndex := strings.Index(body, `href="/a-stock"`)
	auctionIndex := strings.Index(body, `href="/a-stock/auction"`)
	researchIndex := strings.Index(body, `href="/stock-research"`)
	cryptoIndex := strings.Index(body, `href="/crypto"`)
	if aStockIndex < 0 || auctionIndex < 0 || researchIndex < 0 || cryptoIndex < 0 || aStockIndex > researchIndex || researchIndex > auctionIndex || auctionIndex > cryptoIndex {
		t.Fatalf("expected A股, 研报调研, 集合竞价 nav links before Crypto, got %s", body)
	}
	for _, want := range []string{
		"A股策略工作台",
		"08:00-09:30",
		"09:26-12:50",
		"每日 09:30",
		"12:50 自动抓取",
		"上午推荐",
		"下午推荐",
		"热点归纳",
		"推荐股票",
		"集合竞价",
		"研报调研",
		"昨日收盘价",
		"昨日涨跌幅",
		"30天涨跌幅",
		"60天涨跌幅",
		"现价",
		"今日跌幅",
		"当日开盘价",
		"T+1 收盘价",
		"T+1 收益",
		"T+2 收益",
		"T+3 收益",
		"T+4 收益",
		"T+5 收益",
		`colspan="10"`,
		"抓取全部财经信息",
		"重新生成上午推荐",
		"重新生成下午推荐",
		"金十全站信息",
		"jin10_full",
		"东方财富网",
		"eastmoney_kuaixun",
		"暂无数据",
		"仅供策略研究和回测，不构成投资建议",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 page to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"T+2 收盘价", "T+3 收盘价", "T+4 收盘价", "T+5 收盘价"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected A股 page not to contain removed backtest column %q, got %s", notWant, body)
		}
	}
	if strings.Contains(body, `body[data-page='a-stock'] header`) {
		t.Fatalf("expected A股 page to keep shared header width, got %s", body)
	}
}

func TestAStockAuctionPageLoadsSummaryAndRows(t *testing.T) {
	fetchedAt := time.Date(2026, 6, 16, 1, 30, 0, 0, time.UTC)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/auction" {
			t.Fatalf("unexpected auction content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("date") != "2026-06-16" || r.URL.Query().Get("keyword") != "科" {
			t.Fatalf("unexpected auction query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.AStockAuctionListResult{
				Date:         "2026-06-16",
				Keyword:      "科",
				LatestDate:   "2026-06-16",
				Dates:        []string{"2026-06-16", "2026-06-15"},
				SummaryCount: 2,
				Total:        1,
				Page:         1,
				PageSize:     50,
				TotalAmount:  151000000,
				MaxItem: &model.AStockAuctionAmount{
					TradeDate:     "2026-06-16",
					Code:          "002230",
					Name:          "科大讯飞",
					AuctionAmount: 100000000,
					FetchedAt:     fetchedAt,
				},
				FetchedAt: &fetchedAt,
				Items: []model.AStockAuctionAmount{{
					TradeDate:     "2026-06-16",
					Code:          "002230",
					Name:          "科大讯飞",
					AuctionPrice:  41.2,
					AuctionVolume: 123400,
					AuctionAmount: 5084080,
					Source:        "akshare_pre_min",
					Status:        "ok",
					FetchedAt:     fetchedAt,
				}},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/auction?date=2026-06-16&keyword=科", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockAuctionPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected auction page 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"集合竞价", "操作区", "获取今日集合竞价金额", "当日汇总", "2026-06-16", "科大讯飞", "股票数", "2", "1.51亿", "508.41万", "12.34万", "akshare_pre_min", `value="科"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected auction page to contain %q, got %s", want, body)
		}
	}
}

func TestAStockAuctionPagePostTriggersSchedulerJob(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/jobs/a-stock-auction-crawl/run" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"status":"triggered"}}`))
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodPost, "/a-stock/auction", strings.NewReader("action=fetch_today_auction"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockAuctionPage(rr, req, map[string]any{"id": 1})

	if !schedulerCalled {
		t.Fatal("expected scheduler to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after auction trigger, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "/a-stock/auction?") || !strings.Contains(loc, "date=") || !strings.Contains(loc, "msg=") {
		t.Fatalf("expected redirect to auction page with date and msg, got %q", loc)
	}
}

func TestAStockAuctionPageExplainsDisabledSchedulerJob(t *testing.T) {
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"data":null,"message":"scheduler job a-stock-auction-crawl is disabled"}`))
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL})
	req := httptest.NewRequest(http.MethodPost, "/a-stock/auction", strings.NewReader("action=fetch_today_auction"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockAuctionPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after auction trigger, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"集合竞价抓取任务未启用", "YUQING_ASTOCK_AUCTION_URL", "scheduler-service"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected disabled scheduler explanation %q, got %q", want, loc)
		}
	}
}

func TestStockResearchPageLoadsFiltersAndRows(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected stock research content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("company") != "科大" || r.URL.Query().Get("institution") != "中金" || r.URL.Query().Get("source") != "sina_finance_report" {
			t.Fatalf("unexpected stock research query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.StockResearchListResult{
				Page:        1,
				PageSize:    50,
				Total:       1,
				Company:     "科大",
				Institution: "中金",
				Source:      "sina_finance_report",
				Sources:     []string{"sina_finance_report", "sohu_finance_report"},
				Items: []model.StockResearchSurvey{{
					Code:         "002230",
					Name:         "科大讯飞",
					Kind:         "report",
					Title:        "科大讯飞深度研究",
					Institution:  "中金公司",
					Analyst:      "张三",
					Rating:       "买入",
					TargetPrice:  "50.00",
					ResearchDate: "2026-06-16",
					SourceType:   "sina_finance_report",
					SourceURL:    "https://sina.example.com/1",
				}},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/stock-research?company=科大&institution=中金&source=sina_finance_report", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stock research page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"研报调研", "科大讯飞深度研究", "中金公司", "张三", "买入", "50.00", "新浪财经", "搜狐财经", "回补近一年", `value="科大"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected stock research page to contain %q, got %s", want, body)
		}
	}
}

func TestStockResearchPagePostTriggersBackfill(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/stock-research/backfill" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.URL.Query().Get("code") != "002230" || r.URL.Query().Get("company") != "科大讯飞" || r.URL.Query().Get("start") == "" || r.URL.Query().Get("end") == "" {
			t.Fatalf("unexpected scheduler query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "triggered"}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodPost, "/stock-research", strings.NewReader("action=backfill_year&code=002230&company=%E7%A7%91%E5%A4%A7%E8%AE%AF%E9%A3%9E"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if !schedulerCalled {
		t.Fatal("expected scheduler backfill to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after stock research backfill, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"/stock-research?", "code=002230", "company=科大讯飞", "回补任务已触发"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect to keep filters and message %q, got %q", want, loc)
		}
	}
}

func TestAStockHoldingsPageLoadsSummaryRowsAndFilters(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/holdings":
			if r.URL.Query().Get("code") != "002230" || r.URL.Query().Get("period") != "20260331" || r.URL.Query().Get("holder_type") != "fund" {
				t.Fatalf("unexpected holdings list query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingListResult{
				Total:       1,
				Page:        1,
				PageSize:    50,
				Periods:     []string{"20260331"},
				HolderTypes: []string{"fund"},
				Sources:     []string{"stock_institute_hold_detail"},
				Items: []model.StockInstitutionHolding{{
					StockCode:    "002230",
					StockName:    "科大讯飞",
					ReportPeriod: "20260331",
					HolderName:   "易方达基金",
					HolderType:   "fund",
					Shares:       1000,
					FloatRatio:   1.5,
					SourceType:   "stock_institute_hold_detail",
				}},
			}})
		case "/api/v1/a-stock/holdings/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{
				StockCode:       "002230",
				ReportPeriod:    "20260331",
				HolderCount:     2,
				FundCount:       1,
				HolderTypeCount: 2,
				TotalShares:     3000,
				TotalFloatRatio: 3.5,
				MaxHolderName:   "易方达基金",
				MaxHolderType:   "fund",
				MaxHolderShares: 1000,
			}})
		default:
			t.Fatalf("unexpected holdings content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/holdings?code=002230&period=20260331&holder_type=fund", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockHoldingsPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected holdings page 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"机构持仓", "共持摘要", "2026-Q1", "易方达基金", "基金", "3.50%"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected holdings page to contain %q, got %s", want, body)
		}
	}
}

func TestAStockHoldingsPagePostTriggersBackfill(t *testing.T) {
	var called bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/holdings/backfill" {
			t.Fatalf("unexpected scheduler holdings request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("code") != "002230" || r.URL.Query().Get("period") != "20260331" {
			t.Fatalf("unexpected scheduler holdings query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "triggered"}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	form := url.Values{"action": {"backfill"}, "code": {"002230"}, "period": {"20260331"}, "holder_type": {"fund"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock/holdings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockHoldingsPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected scheduler holdings backfill call")
	}
	if location := rr.Header().Get("Location"); !strings.Contains(location, "/a-stock/holdings?") || !strings.Contains(location, "msg=") {
		t.Fatalf("expected redirect back to holdings with msg, got %s", location)
	}
}

func TestAStockStockGenerateActionsSelectPeriod(t *testing.T) {
	srv := NewServer(config.Config{})
	tests := []struct {
		name       string
		fromPeriod string
		action     string
		wantPeriod string
		wantMsg    string
		wantIgnore bool
	}{
		{
			name:       "switch to afternoon",
			fromPeriod: "morning",
			action:     "generate_afternoon_stock",
			wantPeriod: "afternoon",
			wantMsg:    "已切换到下午窗口，按 09:26-12:50 历史新闻重新计算推荐。",
		},
		{
			name:       "regenerate morning from afternoon",
			fromPeriod: "afternoon",
			action:     "generate_morning_stock",
			wantPeriod: "morning",
			wantMsg:    "已切换到上午窗口，按 08:00-09:30 历史新闻重新计算推荐。",
		},
		{
			name:       "ignore recent filter",
			fromPeriod: "morning",
			action:     "generate_ignore_recent_stock",
			wantPeriod: "morning",
			wantMsg:    "上午推荐已忽略近15日重复推荐过滤，按当前新闻窗口重新计算推荐。",
			wantIgnore: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{"date": {"2026-06-16"}, "period": {tc.fromPeriod}, "action": {tc.action}}
			req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()
			srv.handleAStockPage(rr, req, map[string]any{"id": 1})

			if rr.Code != http.StatusSeeOther {
				t.Fatalf("expected redirect, got %d", rr.Code)
			}
			loc := rr.Header().Get("Location")
			if !strings.Contains(loc, "date=2026-06-16") || !strings.Contains(loc, "period="+tc.wantPeriod) {
				t.Fatalf("expected %s redirect, got %q", tc.wantPeriod, loc)
			}
			if tc.wantIgnore && !strings.Contains(loc, "ignore_recent=1") {
				t.Fatalf("expected ignore_recent redirect, got %q", loc)
			}
			decoded, _ := url.QueryUnescape(loc)
			if !strings.Contains(decoded, tc.wantMsg) {
				t.Fatalf("expected generation message %q, got %q", tc.wantMsg, decoded)
			}
		})
	}
}

func TestAStockPageShowsBackfillCurrentWindowAction(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items:    []model.Item{},
				Page:     1,
				PageSize: 200,
				Total:    0,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"补抓当前窗口新闻", `name="action" value="backfill_window_news"`, "补录上午新闻并生成推荐", `name="action" value="backfill_morning_stock"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected backfill action %q, got %s", want, body)
		}
	}
}

func TestAStockBackfillWindowActionPassesMorningWindow(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" || r.URL.Query().Get("end") != "2026-06-16 09:30:59" || r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected backfill window query: %s", r.URL.RawQuery)
		}
		mu.Lock()
		seen[r.URL.Query().Get("source_type")] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok"})
	}))
	defer crawler.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" || r.URL.Query().Get("end") != "2026-06-16 09:30:59" {
			t.Fatalf("unexpected content window query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items: []model.Item{{
					ID:         1,
					SourceType: "flash",
					Title:      "上午财经新闻",
					CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC),
				}},
				Page:     1,
				PageSize: 200,
				Total:    1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{CrawlerURL: crawler.URL, ContentURL: content.URL})
	form := url.Values{"date": {"2026-06-16"}, "period": {"morning"}, "action": {"backfill_window_news"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	for _, sourceType := range []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun"} {
		if !seen[sourceType] {
			t.Fatalf("expected source %s to be backfilled, got %+v", sourceType, seen)
		}
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-16", "period=morning", "已补抓 2026-06-16 上午 08:00-09:30", "当前窗口已有 1 条财经新闻"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect message %q, got %q", want, loc)
		}
	}
}

func TestAStockBackfillMorningStockActionSelectsMorningWindow(t *testing.T) {
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-15 08:00:00" || r.URL.Query().Get("end") != "2026-06-15 09:30:59" {
			t.Fatalf("unexpected morning backfill query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok"})
	}))
	defer crawler.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data":    model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{CrawlerURL: crawler.URL, ContentURL: content.URL})
	form := url.Values{"date": {"2026-06-15"}, "period": {"afternoon"}, "action": {"backfill_morning_stock"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-15", "period=morning", "已补抓 2026-06-15 上午 08:00-09:30", "上午推荐已按补录后的新闻窗口重新计算"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect message %q, got %q", want, loc)
		}
	}
}

func TestAStockPageExplainsMorningNoNews(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") != "publish_time" || r.URL.Query().Get("start") != "2026-06-16 08:00:00" || r.URL.Query().Get("end") != "2026-06-16 09:30:59" {
			t.Fatalf("unexpected A股 morning window query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items:    []model.Item{},
				Page:     1,
				PageSize: 200,
				Total:    0,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"上午推荐", "08:00-09:30", "没有历史新闻", "请先抓取或补抓财经信息"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected no-news explanation %q, got %s", want, body)
		}
	}
}

func TestAStockPageExplainsNewsWithoutHotspots(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items: []model.Item{
					{ID: 901, SourceType: "flash", Title: "普通市场消息", Summary: "未匹配词典", CapturedAt: time.Date(2026, 6, 16, 1, 10, 0, 0, time.UTC)},
				},
				Page:     1,
				PageSize: 200,
				Total:    1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"有 1 条新闻", "未命中 A股热点关键词"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected no-hotspot explanation %q, got %s", want, body)
		}
	}
}

func TestNormalizeAStockPeriodAcceptsAfterAlias(t *testing.T) {
	if got := normalizeAStockPeriod("after"); got.Key != "afternoon" || got.Label != "下午推荐" {
		t.Fatalf("expected after alias to normalize to afternoon, got %+v", got)
	}
}

func TestAStockPageLoadsAfternoonWindow(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 afternoon window query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("start") != "2026-06-16 09:26:00" || r.URL.Query().Get("end") != "2026-06-16 12:50:59" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data":    model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items: []model.Item{
					{ID: 601, SourceType: "eastmoney_kuaixun", Title: "午间低空经济订单增加", Summary: "无人机产业链升温", CapturedAt: time.Date(2026, 6, 16, 4, 42, 0, 0, time.UTC)},
				},
				Page:     1,
				PageSize: 200,
				Total:    1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=afternoon", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"下午推荐", "09:26-12:50 财经新闻", "午间低空经济订单增加", "万丰奥威"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 afternoon page to contain %q, got %s", want, body)
		}
	}
}

func TestAStockPageOffersTodayNavigationAndAfterAlias(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 16, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") != "publish_time" || r.URL.Query().Get("start") != "2026-06-11 09:26:00" || r.URL.Query().Get("end") != "2026-06-11 12:50:59" {
			t.Fatalf("expected after alias to use afternoon window, got query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items:    []model.Item{{ID: 701, SourceType: "flash", Title: "普通财经新闻", CapturedAt: time.Date(2026, 6, 11, 2, 0, 0, 0, time.UTC)}},
				Page:     1,
				PageSize: 200,
				Total:    1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-11&period=after", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"下午推荐",
		"回到今天",
		"今日 2026-06-16 上午",
		"今日 2026-06-16 下午",
		`href="/a-stock?date=2026-06-16&period=afternoon"`,
		"前5日 2026-06-11 下午",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 page to contain %q, got %s", want, body)
		}
	}
}

func setAStockNowForTest(t *testing.T, now time.Time) {
	t.Helper()
	previous := aStockNow
	aStockNow = func() time.Time {
		return now
	}
	t.Cleanup(func() {
		aStockNow = previous
	})
}

func setAStockEastmoneyKlineURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	previous := aStockEastmoneyKlineURL
	aStockEastmoneyKlineURL = rawURL
	t.Cleanup(func() {
		aStockEastmoneyKlineURL = previous
	})
}

func setAStockYahooChartURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	previous := aStockYahooChartURL
	aStockYahooChartURL = rawURL
	t.Cleanup(func() {
		aStockYahooChartURL = previous
	})
}

func TestAStockNewsSectionPaginatesTenItems(t *testing.T) {
	items := make([]model.Item, 0, 12)
	for i := 1; i <= 12; i++ {
		items = append(items, model.Item{
			ID:         int64(700 + i),
			SourceType: "flash",
			Title:      fmt.Sprintf("分页新闻%02d", i),
			Summary:    "普通财经新闻",
			CapturedAt: time.Date(2026, 6, 11, 1, 26+i, 0, 0, time.UTC),
		})
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") != "publish_time" || r.URL.Query().Get("start") != "2026-06-11 09:26:00" || r.URL.Query().Get("end") != "2026-06-11 12:50:59" {
			t.Fatalf("unexpected A股 afternoon window query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items:    items,
				Page:     1,
				PageSize: 200,
				Total:    12,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	firstReq := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-11&period=afternoon", nil)
	firstRR := httptest.NewRecorder()
	srv.handleAStockPage(firstRR, firstReq, map[string]any{"id": 1})
	if firstRR.Code != http.StatusOK {
		t.Fatalf("expected first page 200, got %d", firstRR.Code)
	}
	firstBody := firstRR.Body.String()
	for _, want := range []string{"分页新闻01", "分页新闻10", "新闻分页：1/2，共12条", `/a-stock?date=2026-06-11&period=afternoon&news_page=2`, `<strong style="display:block;margin-top:8px;font-size:24px">12</strong>`} {
		if !strings.Contains(firstBody, want) {
			t.Fatalf("expected first news page to contain %q, got %s", want, firstBody)
		}
	}
	for _, notWant := range []string{"分页新闻11", "分页新闻12"} {
		if strings.Contains(firstBody, notWant) {
			t.Fatalf("expected first news page not to contain %q, got %s", notWant, firstBody)
		}
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-11&period=afternoon&news_page=2", nil)
	secondRR := httptest.NewRecorder()
	srv.handleAStockPage(secondRR, secondReq, map[string]any{"id": 1})
	if secondRR.Code != http.StatusOK {
		t.Fatalf("expected second page 200, got %d", secondRR.Code)
	}
	secondBody := secondRR.Body.String()
	for _, want := range []string{"分页新闻11", "分页新闻12", "新闻分页：2/2，共12条"} {
		if !strings.Contains(secondBody, want) {
			t.Fatalf("expected second news page to contain %q, got %s", want, secondBody)
		}
	}
	if strings.Contains(secondBody, "分页新闻10") {
		t.Fatalf("expected second news page not to contain page one item, got %s", secondBody)
	}
}

func TestAStockPageLoadsNewsAndRecommendations(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 16, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("date") != "2026-06-16" {
			t.Fatalf("unexpected market date: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.Query().Get("codes"), "002230") || !strings.Contains(r.URL.Query().Get("codes"), "688981") {
			t.Fatalf("unexpected market codes: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "002230", "date": "2026-04-17", "open": 12.10, "close": 12.00, "pct": -0.40},
					{"code": "002230", "date": "2026-05-16", "open": 9.90, "close": 10.00, "pct": 0.20},
					{"code": "002230", "date": "2026-06-15", "open": 10.10, "close": 10.50, "pct": 1.25},
					{"code": "002230", "date": "2026-06-16", "open": 10.60, "close": 10.90, "pct": 3.81},
					{"code": "002230", "date": "2026-06-17", "open": 10.95, "close": 11.00, "pct": 0.92},
					{"code": "002230", "date": "2026-06-18", "open": 11.05, "close": 10.80, "pct": -1.82},
					{"code": "002230", "date": "2026-06-19", "open": 10.82, "close": 11.20, "pct": 3.70},
					{"code": "002230", "date": "2026-06-22", "open": 11.25, "close": 11.40, "pct": 1.79},
					{"code": "002230", "date": "2026-06-23", "open": 11.42, "close": 11.10, "pct": -2.63},
					{"code": "688981", "date": "2026-06-15", "open": 51.00, "close": 50.20, "pct": -0.60},
					{"code": "688981", "date": "2026-06-16", "open": 50.10, "close": 50.60, "pct": 0.80},
					{"code": "688981", "date": "2026-06-17", "open": 50.70, "close": 51.00, "pct": 0.79},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 window query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" || r.URL.Query().Get("end") != "2026-06-16 09:30:59" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data":    model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items: []model.Item{
					{ID: 501, SourceType: "flash", Title: "AI 算力政策加码", Summary: "人工智能产业链活跃", CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC)},
					{ID: 502, SourceType: "headline", Title: "半导体先进封装景气度提升", Summary: "芯片设备需求回暖", CapturedAt: time.Date(2026, 6, 16, 1, 12, 0, 0, time.UTC)},
				},
				Page:     1,
				PageSize: 200,
				Total:    2,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"AI 算力政策加码", "半导体先进封装景气度提升", "人工智能", "半导体", "科大讯飞", "中芯国际", "财经新闻数", "昨日收盘价", "昨日涨跌幅", "30天涨跌幅", "60天涨跌幅", "现价", "今日跌幅", "推荐历史", "补抓并重新生成当前窗口", "重新生成当前推荐", "忽略15日重复过滤重新生成", "刷新当前回测", `name="action" value="backfill_window_news"`, `name="action" value="generate_morning_stock"`, `name="action" value="generate_ignore_recent_stock"`, `name="action" value="refresh_backtest"`, "今日 2026-06-16 上午", "今日 2026-06-16 下午", "前1日 2026-06-15 上午", "前1日 2026-06-15 下午", "前5日 2026-06-11 上午", "前5日 2026-06-11 下午", "astock-recommendation-table", "10.50", "+1.25%", "+5.00%", "-12.50%", "10.90", "+3.81%", "50.20", "-0.60%", "50.60", "+0.80%", "002230 科大讯飞", "+7.55%", "已回测", "已回测T+1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 page to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"T+2 收盘价", "T+3 收盘价", "T+4 收盘价", "T+5 收盘价"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected A股 page not to contain removed backtest column %q, got %s", notWant, body)
		}
	}
	if strings.Contains(body, "astock-history-card") || strings.Contains(body, "astock-history-stocks") {
		t.Fatalf("expected old recommendation history stock card to be removed, got %s", body)
	}
	if !strings.Contains(body, `/a-stock?date=2026-06-15&period=morning`) {
		t.Fatalf("expected recommendation history tab to link previous day, got %s", body)
	}
	if !strings.Contains(body, `class="astock-up"`) || !strings.Contains(body, `class="astock-down"`) {
		t.Fatalf("expected A股 page to color上涨/下跌 percentages, got %s", body)
	}
}

func TestAStockRecommendationHistoryRendersDateTabs(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 16, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var b strings.Builder
	renderAStockRecommendationHistoryTabs(&b, "2026-06-18", "afternoon", false)

	body := b.String()
	for _, want := range []string{
		"推荐历史",
		"今日 2026-06-16 上午",
		"今日 2026-06-16 下午",
		"前1日 2026-06-15 上午",
		"前1日 2026-06-15 下午",
		"前5日 2026-06-11 上午",
		"前5日 2026-06-11 下午",
		`/a-stock?date=2026-06-16&period=morning`,
		`/a-stock?date=2026-06-16&period=afternoon`,
		`/a-stock?date=2026-06-15&period=morning`,
		`/a-stock?date=2026-06-15&period=afternoon`,
		`data-preserve-scroll="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected history tabs to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "astock-history-card") || strings.Contains(body, "山东黄金") {
		t.Fatalf("expected history stock table/card to be removed, got %s", body)
	}
}

func TestAStockRecommendationHistoryActionsUseSelectedPeriod(t *testing.T) {
	var b strings.Builder
	renderAStockRecommendationHistoryActions(&b, "2026-06-16", "afternoon", false)
	body := b.String()
	for _, want := range []string{
		`name="date" value="2026-06-16"`,
		`name="period" value="afternoon"`,
		`name="action" value="backfill_window_news"`,
		`name="action" value="generate_afternoon_stock"`,
		`name="action" value="generate_ignore_recent_stock"`,
		`name="action" value="refresh_backtest"`,
		`data-preserve-scroll="1"`,
		"补抓并重新生成当前窗口",
		"重新生成当前推荐",
		"忽略15日重复过滤重新生成",
		"刷新当前回测",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected history actions to contain %q, got %s", want, body)
		}
	}
}

func TestAStockIgnoreRecentStatePersistsInHistoryNavigation(t *testing.T) {
	var tabs strings.Builder
	renderAStockRecommendationHistoryTabs(&tabs, "2026-06-16", "morning", true)
	tabsBody := tabs.String()
	for _, want := range []string{
		`/a-stock?date=2026-06-16&period=morning&ignore_recent=1`,
		`/a-stock?date=2026-06-16&period=afternoon&ignore_recent=1`,
	} {
		if !strings.Contains(tabsBody, want) {
			t.Fatalf("expected history tab to preserve ignore_recent %q, got %s", want, tabsBody)
		}
	}

	var actions strings.Builder
	renderAStockRecommendationHistoryActions(&actions, "2026-06-16", "morning", true)
	actionsBody := actions.String()
	if !strings.Contains(actionsBody, `name="ignore_recent" value="1"`) {
		t.Fatalf("expected history actions to preserve ignore_recent, got %s", actionsBody)
	}
}

func TestFilterRecentAStockRecommendationsDropsPast15DayCodes(t *testing.T) {
	recommendations := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞"},
		{Rank: 2, Hotspot: "半导体", Code: "688981", Name: "中芯国际"},
		{Rank: 3, Hotspot: "低空经济", Code: "000099", Name: "保留股票"},
	}
	filtered, skipped := filterRecentAStockRecommendations(recommendations, map[string]struct{}{
		"002230": {},
		"688981": {},
	})
	if skipped != 2 || len(filtered) != 1 {
		t.Fatalf("expected two recent recommendations to be filtered, got skipped=%d filtered=%+v", skipped, filtered)
	}
	if filtered[0].Rank != 1 || filtered[0].Code != "000099" {
		t.Fatalf("expected remaining recommendation to be reranked, got %+v", filtered)
	}
}

func TestAStockContextCanIgnoreRecentRecommendationFilter(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []map[string]any{
				{"code": "002230", "date": "2026-06-15", "open": 10.10, "close": 10.50, "pct": 1.25},
				{"code": "002230", "date": "2026-06-16", "open": 10.60, "close": 10.90, "pct": 3.81},
			}},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.ItemListResult{
				Items: []model.Item{{ID: 900, SourceType: "flash", Title: "AI 算力政策加码", Summary: "人工智能产业链活跃", CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC)}},
				Page:  1, PageSize: 200, Total: 1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	filtered := srv.loadAStockContext("2026-06-16", "morning", 1, false)
	if filtered.RecentFiltered == 0 || len(filtered.Recommendations) != 0 {
		t.Fatalf("expected recent filter to remove recommendations, got filtered=%d recommendations=%+v", filtered.RecentFiltered, filtered.Recommendations)
	}
	ignored := srv.loadAStockContext("2026-06-16", "morning", 1, true)
	if ignored.RecentFiltered != 0 || len(ignored.Recommendations) == 0 {
		t.Fatalf("expected ignoreRecent to keep recommendations, got filtered=%d recommendations=%+v", ignored.RecentFiltered, ignored.Recommendations)
	}
}

func TestAStockMarketViewFiltersDeepDrawdownsAndPenalizesSector(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "000001", Name: "回撤过滤", HotspotScore: 80, MarketScore: 80, Reason: "热度分 80"},
		{Rank: 2, Hotspot: "人工智能", Code: "000002", Name: "保留扣分", HotspotScore: 80, MarketScore: 80, Reason: "热度分 80"},
		{Rank: 3, Hotspot: "金融券商", Code: "000003", Name: "正常板块", HotspotScore: 75, MarketScore: 75, Reason: "热度分 75"},
		{Rank: 4, Hotspot: "金融券商", Code: "000004", Name: "停牌过滤", HotspotScore: 75, MarketScore: 75, Reason: "热度分 75"},
	})
	bars := []aStockMarketBar{
		{Code: "000001", Date: "2026-04-17", Close: 100},
		{Code: "000001", Date: "2026-05-16", Close: 100},
		{Code: "000001", Date: "2026-06-15", Close: 84, Pct: -1},
		{Code: "000001", Date: "2026-06-16", Open: 84, Close: 85, Pct: 1},
		{Code: "000002", Date: "2026-04-17", Close: 100},
		{Code: "000002", Date: "2026-05-16", Close: 100},
		{Code: "000002", Date: "2026-06-15", Close: 92, Pct: 1},
		{Code: "000002", Date: "2026-06-16", Open: 93, Close: 94, Pct: 2},
		{Code: "000003", Date: "2026-04-17", Close: 100},
		{Code: "000003", Date: "2026-05-16", Close: 100},
		{Code: "000003", Date: "2026-06-15", Close: 98, Pct: 2},
		{Code: "000003", Date: "2026-06-16", Open: 99, Close: 100, Pct: 1},
		{Code: "000004", Date: "2026-04-17", Close: 100},
		{Code: "000004", Date: "2026-05-16", Close: 100},
		{Code: "000004", Date: "2026-06-15", Close: 98, Pct: 2},
	}

	filtered, rows, status := applyAStockMarketBars("2026-06-16", recommendations, bars)

	if len(filtered) != 2 {
		t.Fatalf("expected one deep-drawdown recommendation to be filtered, got %+v", filtered)
	}
	for _, rec := range filtered {
		if rec.Code == "000001" {
			t.Fatalf("expected 30/60 day drawdown stock to be filtered, got %+v", filtered)
		}
		if rec.Code == "000004" {
			t.Fatalf("expected stock without entry price to be filtered, got %+v", filtered)
		}
	}
	if filtered[0].Code != "000003" || filtered[0].Rank != 1 {
		t.Fatalf("expected unpenalized sector to rank first, got %+v", filtered)
	}
	if filtered[0].CurrentPrice != "100.00" || filtered[0].TodayPct != "+1.00%" || filtered[0].TodayPctClass != "astock-up" {
		t.Fatalf("expected current day market fields to be filled, got %+v", filtered[0])
	}
	if filtered[1].Code != "000002" || filtered[1].MarketScore != 65 || !strings.Contains(filtered[1].Reason, "板块回撤减分 15") {
		t.Fatalf("expected remaining AI stock to carry sector penalty, got %+v", filtered[1])
	}
	if filtered[1].CurrentPrice != "94.00" || filtered[1].TodayPct != "+2.00%" || filtered[1].TodayPctClass != "astock-up" {
		t.Fatalf("expected penalized current day market fields to be filled, got %+v", filtered[1])
	}
	if len(rows) != 2 || strings.Contains(rows[0].Stock+rows[1].Stock, "000001") || strings.Contains(rows[0].Stock+rows[1].Stock, "000004") {
		t.Fatalf("expected backtest rows to follow filtered recommendations, got %+v", rows)
	}
	if !strings.Contains(status, "过滤回撤股票 1") || !strings.Contains(status, "过滤无当日行情股票 1") {
		t.Fatalf("expected status to mention drawdown and missing price filtering, got %q", status)
	}
}

func TestAStockMarketBarsFallbackToEastmoneyWhenCustomEndpointEmpty(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"items": []any{}}})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.000001" {
			t.Fatalf("unexpected eastmoney secid: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"klines": []string{
					"2026-06-15,10.00,11.00,0,0,0,0,0,1.00",
					"2026-06-16,12.00,13.00,0,0,0,0,0,2.00",
				},
			},
		})
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-16", []string{"000001"}, custom.URL)
	if err != nil {
		t.Fatalf("expected fallback market bars, got error: %v", err)
	}
	if len(bars) != 2 || bars[1].Code != "000001" || bars[1].Open != 12 || bars[1].Close != 13 {
		t.Fatalf("unexpected fallback bars: %+v", bars)
	}
}

func TestAStockMarketBarsFallbackToYahooWhenEastmoneyUnavailable(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"items": []any{}}})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "eastmoney unavailable", http.StatusBadGateway)
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	location := aStockLocation()
	unixDay := func(day string) int64 {
		parsed, err := time.ParseInLocation("2006-01-02", day, location)
		if err != nil {
			t.Fatalf("parse test date: %v", err)
		}
		return parsed.Unix()
	}
	yahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/002230.SZ" {
			t.Fatalf("unexpected yahoo path: %s", r.URL.String())
		}
		if r.URL.Query().Get("interval") != "1d" || r.URL.Query().Get("period1") == "" || r.URL.Query().Get("period2") == "" {
			t.Fatalf("unexpected yahoo query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"chart":{"result":[{"timestamp":[%d,%d,%d,%d],"indicators":{"quote":[{"open":[10,11,12,13],"close":[10.5,11.5,13,14]}]}}],"error":null}}`,
			unixDay("2026-06-14"), unixDay("2026-06-15"), unixDay("2026-06-16"), unixDay("2026-06-17"))
	}))
	defer yahoo.Close()
	setAStockYahooChartURLForTest(t, yahoo.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-16", []string{"002230"}, custom.URL)
	if err != nil {
		t.Fatalf("expected yahoo fallback market bars, got error: %v", err)
	}
	if len(bars) != 4 {
		t.Fatalf("expected four yahoo bars, got %+v", bars)
	}
	if bars[2].Code != "002230" || bars[2].Date != "2026-06-16" || bars[2].Open != 12 || bars[2].Close != 13 {
		t.Fatalf("unexpected yahoo strategy day bar: %+v", bars[2])
	}
	if bars[2].Pct < 13.03 || bars[2].Pct > 13.05 {
		t.Fatalf("expected yahoo fallback to compute pct change, got %+v", bars[2])
	}
}

func TestAStockRecommendationsUseTopThreeHotspotIndustries(t *testing.T) {
	recommendations := buildAStockRecommendations([]aStockHotspot{
		{Name: "黄金有色", Keywords: []string{"黄金"}, Score: 32, Evidence: 2},
		{Name: "消费电子", Keywords: []string{"华为"}, Score: 26, Evidence: 2},
		{Name: "新能源", Keywords: []string{"储能"}, Score: 25, Evidence: 1},
		{Name: "医药生物", Keywords: []string{"医药"}, Score: 19, Evidence: 1},
	})

	if len(recommendations) != 9 {
		t.Fatalf("expected 9 recommendations from top 3 industries, got %d", len(recommendations))
	}
	for _, rec := range recommendations {
		if rec.Hotspot == "医药生物" {
			t.Fatalf("expected fourth industry to be excluded, got %+v", rec)
		}
	}
	for _, want := range []string{"黄金有色", "消费电子", "新能源"} {
		found := false
		for _, rec := range recommendations {
			if rec.Hotspot == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected recommendations to include hotspot %q, got %+v", want, recommendations)
		}
	}
}

func TestAStockRecommendationsApplyHoldingSummaryBonus(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/holdings/summary" || r.URL.Query().Get("code") != "002230" {
			t.Fatalf("unexpected holdings summary request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{
			StockCode:       "002230",
			HolderCount:     6,
			HolderTypeCount: 3,
			TotalFloatRatio: 8.4,
		}})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	recommendations := srv.applyAStockHoldingSummaries([]aStockRecommendation{{
		Rank:         1,
		Hotspot:      "人工智能",
		Code:         "002230",
		Name:         "科大讯飞",
		HotspotScore: 40,
		MarketScore:  40,
		Reason:       "命中 AI，热度分 40",
	}})
	if len(recommendations) != 1 {
		t.Fatalf("expected one recommendation, got %+v", recommendations)
	}
	got := recommendations[0]
	if got.MarketScore != 60 || got.HoldingSummary != "6家/3类" || got.HoldingRatio != "8.40%" {
		t.Fatalf("expected holdings bonus and display fields, got %+v", got)
	}
	if !strings.Contains(got.Reason, "机构共持 6 家") || !strings.Contains(got.Reason, "持仓加分 20") {
		t.Fatalf("expected holdings reason, got %s", got.Reason)
	}
}

func TestAStockCrawlActionTriggersAllJin10Sources(t *testing.T) {
	var sources []string
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		sources = append(sources, r.URL.Query().Get("source_type"))
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.CrawlSummary{SourceType: r.URL.Query().Get("source_type")}})
	}))
	defer crawler.Close()

	srv := NewServer(config.Config{CrawlerURL: crawler.URL})
	form := url.Values{"date": {"2026-06-16"}, "action": {"crawl"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	sort.Strings(sources)
	if strings.Join(sources, ",") != "eastmoney_kuaixun,flash,headline,jin10_full" {
		t.Fatalf("expected flash, headline, jin10_full and eastmoney_kuaixun crawl, got %v", sources)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/a-stock?") || !strings.Contains(loc, "date=2026-06-16") {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
}

func TestAStockRouteRequiresSession(t *testing.T) {
	srv := NewServer(config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/a-stock", nil)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for unauthenticated A股 route, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestCryptoPageDefaultsToBTCAndETHCards(t *testing.T) {
	var requested []string
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pair := r.URL.Query().Get("pair")
		requested = append(requested, pair)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.CryptoInsightResponse{
				Pair:       pair,
				BaseAsset:  strings.TrimSuffix(pair, "USDT"),
				QuoteAsset: "USDT",
				PriceSnapshot: model.CryptoPriceSnapshot{
					Symbol:    pair,
					Price:     123.45,
					Change1H:  1.23,
					Change4H:  2.34,
					Change24H: 3.45,
					UpdatedAt: time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
				},
				UpdatedAt: time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
			},
		})
	}))
	defer analysis.Close()

	srv := &Server{
		cfg:       config.Config{AnalysisURL: analysis.URL},
		client:    resty.New(),
		templates: NewServer(config.Config{}).templates,
	}

	req := httptest.NewRequest(http.MethodGet, "/crypto", nil)
	rr := httptest.NewRecorder()
	srv.handleCryptoPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"默认价格看板", "BTCUSDT", "ETHUSDT"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected default crypto page to contain %q, got %s", want, body)
		}
	}
	if len(requested) != 2 || requested[0] != "BTCUSDT" || requested[1] != "ETHUSDT" {
		t.Fatalf("expected default requests for BTCUSDT/ETHUSDT, got %+v", requested)
	}
}

func TestHandleRulesCreatesProjectWhenProjectListIsEmpty(t *testing.T) {
	var createdProject model.Project
	var createdRule model.MonitorRule
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			if err := json.NewDecoder(r.Body).Decode(&createdProject); err != nil {
				t.Fatalf("decode project body: %v", err)
			}
			createdProject.ID = 42
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": createdProject})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/monitor-rules":
			if err := json.NewDecoder(r.Body).Decode(&createdRule); err != nil {
				t.Fatalf("decode rule body: %v", err)
			}
			createdRule.ID = 99
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": createdRule})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer content.Close()

	srv := &Server{
		cfg:       config.Config{ContentURL: content.URL},
		client:    resty.New(),
		templates: NewServer(config.Config{}).templates,
	}
	form := url.Values{}
	form.Set("name", "AI 监测")
	form.Set("include_keywords", "AI,大模型")
	form.Set("channels", "flash,headline")
	form.Set("severity", "high")
	req := httptest.NewRequest(http.MethodPost, "/monitor-rules", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "/monitor-rules")
	rr := httptest.NewRecorder()

	srv.handleRules(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if createdProject.ID != 42 || createdProject.Name != "监测项目：AI,大模型" || createdProject.Keywords != "AI,大模型" {
		t.Fatalf("unexpected created project: %+v", createdProject)
	}
	if createdRule.ProjectID != 42 || createdRule.Name != "AI 监测" || createdRule.Channels != "flash,headline" || createdRule.Severity != "high" {
		t.Fatalf("unexpected created rule: %+v", createdRule)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "%E8%A7%84%E5%88%99%E5%88%9B%E5%BB%BA%E6%88%90%E5%8A%9F") {
		t.Fatalf("expected success redirect, got %s", loc)
	}
}

func TestCryptoPageQueriesRequestedPairOnly(t *testing.T) {
	var requested []string
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pair := r.URL.Query().Get("pair")
		requested = append(requested, pair)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.CryptoInsightResponse{
				Pair:       pair,
				BaseAsset:  "SOL",
				QuoteAsset: "USDT",
				PriceSnapshot: model.CryptoPriceSnapshot{
					Symbol:    pair,
					Price:     150.12,
					Change1H:  0.8,
					Change4H:  1.5,
					Change24H: 6.2,
					UpdatedAt: time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
				},
				Signals: model.CryptoSignalSet{
					H4:  model.CryptoSignal{Horizon: "4h", Direction: "bullish", Confidence: 0.62, Bullish: 0.55, Neutral: 0.28, Bearish: 0.17},
					H24: model.CryptoSignal{Horizon: "24h", Direction: "bullish", Confidence: 0.58, Bullish: 0.52, Neutral: 0.31, Bearish: 0.17},
				},
				TopReasons: []model.CryptoReason{
					{Label: "机构/ETF", Direction: "bullish", Score: 1.2, EvidenceCount: 2, Summary: "ETF 资金流入"},
				},
				EvidenceArticles: []model.CryptoEvidenceArticle{
					{Title: "SOL ETF 预期升温", Summary: "市场关注资金流入", SourceType: "headline", Direction: "bullish", ReasonLabel: "机构/ETF", RelevanceScore: 8.8, PublishTime: "2026-06-04 10:00"},
				},
				SocialSentiment: model.CryptoSocialSentiment{Direction: "bullish", Confidence: 0.41},
				AIExplanation:   "SOL/USDT 价格维持走强。",
				UpdatedAt:       time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
				CacheTTLSeconds: 300,
			},
		})
	}))
	defer analysis.Close()

	srv := &Server{
		cfg:       config.Config{AnalysisURL: analysis.URL},
		client:    resty.New(),
		templates: NewServer(config.Config{}).templates,
	}

	req := httptest.NewRequest(http.MethodGet, "/crypto?pair=SOLUSDT", nil)
	rr := httptest.NewRecorder()
	srv.handleCryptoPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "默认价格看板") {
		t.Fatalf("expected queried page not to render default cards, got %s", body)
	}
	for _, want := range []string{"SOLUSDT", "SOL/USDT 价格维持走强。", "相关新闻搜索", "相似 / 相关线索", "后市价格预判与置信度", "62%"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected queried page to contain %q, got %s", want, body)
		}
	}
	if !strings.Contains(body, "SOL ETF 预期升温") {
		t.Fatalf("expected queried page content, got %s", body)
	}
	if len(requested) != 1 || requested[0] != "SOLUSDT" {
		t.Fatalf("expected single request for SOLUSDT, got %+v", requested)
	}
}

func TestCryptoPageEmptyStateIncludesOperationsDiagnostics(t *testing.T) {
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/crypto/insights" || r.URL.Query().Get("pair") != "eth" {
			t.Fatalf("unexpected analysis request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.CryptoInsightResponse{
				Pair:       "ETHUSDT",
				BaseAsset:  "ETH",
				QuoteAsset: "USDT",
				Signals: model.CryptoSignalSet{
					H4:  model.CryptoSignal{Horizon: "4h", Direction: "neutral"},
					H24: model.CryptoSignal{Horizon: "24h", Direction: "neutral"},
				},
				SocialSentiment: model.CryptoSocialSentiment{Direction: "neutral", Neutral: 1},
				AIExplanation:   "ETH/USDT 当前证据不足。",
				UpdatedAt:       time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC),
				CacheTTLSeconds: 300,
			},
		})
	}))
	defer analysis.Close()

	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/tasks/crawl/runs" {
			t.Fatalf("unexpected crawler request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		sourceType := r.URL.Query().Get("source_type")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": []model.CrawlRun{{
				SourceType:    sourceType,
				Status:        "failed",
				FetchedCount:  0,
				InsertedCount: 0,
				ErrorText:     "endpoint empty",
				StartedAt:     time.Date(2026, 6, 15, 7, 0, 0, 0, time.UTC),
			}},
		})
	}))
	defer crawler.Close()

	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/scheduler/jobs" {
			t.Fatalf("unexpected scheduler request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": []model.OperationSchedulerJob{
				{Name: "crypto-x-crawl", Enabled: false, LastStatus: "disabled", LastMessage: "endpoint empty"},
				{Name: "crypto-telegram-crawl", Enabled: true, LastStatus: "success", NextRunAt: ptrTime(time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC))},
				{Name: "foresight-newsflash-crawl", Enabled: true, LastStatus: "failed", LastMessage: "parse empty"},
				{Name: "coindesk-zh-latest-crawl", Enabled: true, LastStatus: "success", NextRunAt: ptrTime(time.Date(2026, 6, 15, 9, 5, 0, 0, time.UTC))},
				{Name: "panews-newsflash-crawl", Enabled: true, LastStatus: "success", NextRunAt: ptrTime(time.Date(2026, 6, 15, 9, 6, 0, 0, time.UTC))},
			},
		})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{
		AnalysisURL:           analysis.URL,
		CrawlerURL:            crawler.URL,
		SchedulerURL:          scheduler.URL,
		CryptoTelegramURL:     "https://crypto.example.com/tg",
		ForesightNewsflashURL: "https://foresightnews.pro/news",
		CoinDeskZHLatestURL:   "https://www.coindesk.com/zh/latest-crypto-news",
		PANewsNewsflashURL:    "https://www.panewslab.com/rss.xml?lang=zh&type=NEWS",
		ServiceToken:          "test-token",
		HTTPTimeout:           time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/crypto?pair=eth", nil)
	rr := httptest.NewRecorder()
	srv.handleCryptoPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"抓取 X", "抓取 Telegram", "抓取 Foresight", "抓取 CoinDesk 中文", "抓取 PANews", "刷新分析", "数据诊断", "X 未配置 / Telegram 已配置 / Foresight 已配置 / CoinDesk 中文 已配置 / PANews 已配置", "crypto-x-crawl", "foresight-newsflash-crawl", "coindesk-zh-latest-crawl", "panews-newsflash-crawl", "暂无相关新闻命中", "endpoint empty", "parse empty"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected crypto empty state to contain %q, got %s", want, body)
		}
	}
}

func TestCryptoPagePostActionsTriggerBackendsAndPreservePair(t *testing.T) {
	var crawlSources []string
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Token") != "test-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.Path)
		}
		crawlSources = append(crawlSources, r.URL.Query().Get("source_type"))
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": map[string]any{}})
	}))
	defer crawler.Close()

	var analysisRefreshes int
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Token") != "test-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/analysis/refresh" {
			t.Fatalf("unexpected analysis request: %s %s", r.Method, r.URL.Path)
		}
		analysisRefreshes++
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": map[string]any{}})
	}))
	defer analysis.Close()

	srv := NewServer(config.Config{
		CrawlerURL:   crawler.URL,
		AnalysisURL:  analysis.URL,
		ServiceToken: "test-token",
		HTTPTimeout:  time.Second,
	})
	actions := []struct {
		action     string
		wantSource string
		wantMsg    string
	}{
		{action: "crawl_x", wantSource: "crypto_x", wantMsg: "Crypto X 抓取已触发"},
		{action: "crawl_telegram", wantSource: "crypto_telegram", wantMsg: "Crypto Telegram 抓取已触发"},
		{action: "crawl_foresight_newsflash", wantSource: "foresight_newsflash", wantMsg: "Foresight News 抓取已触发"},
		{action: "crawl_coindesk_zh_latest", wantSource: "coindesk_zh_latest", wantMsg: "CoinDesk 中文抓取已触发"},
		{action: "crawl_panews_newsflash", wantSource: "panews_newsflash", wantMsg: "PANews 抓取已触发"},
		{action: "analysis", wantMsg: "分析刷新已触发"},
	}
	for _, tc := range actions {
		form := url.Values{"pair": {"eth"}, "action": {tc.action}}
		req := httptest.NewRequest(http.MethodPost, "/crypto", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleCryptoPage(rr, req, map[string]any{"id": 1})
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect for %s, got %d", tc.action, rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "pair=eth") || !strings.Contains(loc, url.QueryEscape(tc.wantMsg)) {
			t.Fatalf("expected pair-preserving redirect with message, got %s", loc)
		}
		if tc.wantSource != "" && crawlSources[len(crawlSources)-1] != tc.wantSource {
			t.Fatalf("expected crawl source %q, got %+v", tc.wantSource, crawlSources)
		}
	}
	if len(crawlSources) != 5 || analysisRefreshes != 1 {
		t.Fatalf("unexpected backend calls: crawl=%+v analysis=%d", crawlSources, analysisRefreshes)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestLegacySearchTarget(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/fullsearch/result?searchword=钢铁&project_id=7&source_type=headline&industry=能源&province=上海&city=浦东&read=read&favorite=favorited&start=2026-01-01&end=2026-01-31", nil)

	got := s.legacySearchTarget("full", req)
	gotURL, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if gotURL.Path != "/articles" {
		t.Fatalf("expected /articles, got %q", gotURL.Path)
	}

	query := gotURL.Query()
	if query.Get("mode") != "full" {
		t.Fatalf("expected mode full, got %q", query.Get("mode"))
	}
	if query.Get("keyword") != "钢铁" {
		t.Fatalf("expected keyword 钢铁, got %q", query.Get("keyword"))
	}
	if query.Get("project_id") != "7" || query.Get("source_type") != "headline" {
		t.Fatalf("expected search filters preserved, got %q", got)
	}
	if query.Get("industry") != "能源" || query.Get("province") != "上海" || query.Get("city") != "浦东" {
		t.Fatalf("expected location filters preserved, got %q", got)
	}
	if query.Get("read") != "read" || query.Get("favorite") != "favorited" {
		t.Fatalf("expected state filters preserved, got %q", got)
	}
}

func TestLegacySearchTargetRedirectsCryptoTerms(t *testing.T) {
	s := &Server{}
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "symbol", path: "/fullsearch/result?searchword=btc", want: "/crypto?pair=BTCUSDT"},
		{name: "cn alias", path: "/fullsearch/result?searchword=比特币", want: "/crypto?pair=BTCUSDT"},
		{name: "slash pair", path: "/fullsearch/result?searchword=btc/usdt", want: "/crypto?pair=BTCUSDT"},
		{name: "cross pair", path: "/fullsearch/result?searchword=ethbtc", want: "/crypto?pair=ETHBTC"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if got := s.legacySearchTarget("full", req); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestLegacySearchBucketsIndustry(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/full" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		result := model.SearchResult{Total: 3, PageSize: 1000}
		switch page {
		case "1":
			result.Page = 1
			result.Items = []model.Item{
				{ID: 1, RawPayload: `{"industry":"能源","province":"上海","city":"浦东","eventlable":"风电"}`},
				{ID: 2, RawPayload: `{"industry":["能源","科技"],"province":"上海","city":"徐汇","eventIndex":["风电","新能源"]}`},
			}
		case "2":
			result.Page = 2
			result.Items = []model.Item{
				{ID: 3, RawPayload: `{"industry":"金融","province":"北京","city":"朝阳","eventlable":"资本市场"}`},
			}
		default:
			result.Page = 3
			result.Items = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data":    result,
		})
	}))
	defer content.Close()

	s := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	body := `{"searchword":"能源","industryIndex":["能源"],"province":["上海"],"city":["浦东","徐汇"],"eventIndex":["风电"],"timeType":8,"times":"2026-06-01 00:00:00","timee":"2026-06-03 23:59:59"}`
	req := httptest.NewRequest(http.MethodPost, "/industry", strings.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleLegacySearchBuckets("industry")(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Data []legacyFacetBucket `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if envelope.Msg != "行业标签列表成功" {
		t.Fatalf("unexpected msg %q", envelope.Msg)
	}
	if len(envelope.Data.Data) != 4 {
		t.Fatalf("expected 4 buckets including total, got %d", len(envelope.Data.Data))
	}
	buckets := map[string]int{}
	for _, bucket := range envelope.Data.Data {
		buckets[bucket.Key] = bucket.DocCount
	}
	if buckets["能源"] != 2 || buckets["科技"] != 1 || buckets["金融"] != 1 {
		t.Fatalf("unexpected industry buckets: %+v", buckets)
	}
	if buckets["total"] != 3 {
		t.Fatalf("expected total 3, got %+v", buckets)
	}
}

func TestLegacySearchBucketsEvent(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		result := model.SearchResult{Total: 2, PageSize: 1000, Page: 1}
		result.Items = []model.Item{
			{ID: 1, RawPayload: `{"industry":"能源","province":"上海","city":"浦东","eventlable":"风电"}`, TagFlags: "风电"},
			{ID: 2, RawPayload: `{"industry":"能源","province":"上海","city":"徐汇","eventIndex":["风电","新能源"]}`, TagFlags: "新能源"},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data":    result,
		})
	}))
	defer content.Close()

	s := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	body := `{"eventIndex":["风电"],"timeType":8,"times":"2026-06-01 00:00:00","timee":"2026-06-03 23:59:59"}`
	req := httptest.NewRequest(http.MethodPost, "/getevent", strings.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleLegacySearchBuckets("event")(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Data struct {
			Data []legacyFacetBucket `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(envelope.Data.Data) != 3 {
		t.Fatalf("expected 3 buckets including total, got %d", len(envelope.Data.Data))
	}
	buckets := map[string]int{}
	for _, bucket := range envelope.Data.Data {
		buckets[bucket.Key] = bucket.DocCount
	}
	if buckets["风电"] != 2 || buckets["新能源"] != 1 {
		t.Fatalf("unexpected event buckets: %+v", buckets)
	}
	if buckets["total"] != 2 {
		t.Fatalf("expected total 2, got %+v", buckets)
	}
}

func TestHotPageCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/hot/hotpage?limit=2", nil)
	rr := httptest.NewRecorder()
	srv.handleHotPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "AI") {
		t.Fatalf("expected hot page to include analysis keyword, got %s", body)
	}
	if !strings.Contains(body, "/articles?mode=full&keyword=AI") {
		t.Fatalf("expected hot page to link to article search, got %s", body)
	}
	if !strings.Contains(body, "热点数据") {
		t.Fatalf("expected page title, got %s", body)
	}
}

func TestArticlesPagePresentationUsesShanghaiTimeAndNoFavoriteAction(t *testing.T) {
	oldCommit, oldBuildTime, oldBranch := app.GitCommit, app.BuildTime, app.BranchName
	app.GitCommit = "abcdef1"
	app.BuildTime = "2026-06-12T06:17:25Z"
	app.BranchName = "golang-jin10-sqlite"
	t.Cleanup(func() {
		app.GitCommit = oldCommit
		app.BuildTime = oldBuildTime
		app.BranchName = oldBranch
	})

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.ItemListResult{
					Total:    4,
					PageSize: 20,
					Page:     1,
					Items: []model.Item{
						{
							ID:         1,
							Title:      "上海时间测试文章",
							SourceType: "flash",
							CapturedAt: time.Date(2026, 6, 12, 6, 17, 25, 0, time.UTC),
							Read:       false,
							Favorited:  true,
						},
						{
							ID:         2,
							Title:      "PANews 测试文章",
							SourceType: "panews_newsflash",
							CapturedAt: time.Date(2026, 6, 12, 7, 17, 25, 0, time.UTC),
							Read:       true,
						},
						{
							ID:         3,
							Title:      "CoinDesk 测试文章",
							SourceType: "coindesk_zh_latest",
							CapturedAt: time.Date(2026, 6, 12, 8, 17, 25, 0, time.UTC),
						},
						{
							ID:         4,
							Title:      "Foresight 测试文章",
							SourceType: "foresight_newsflash",
							CapturedAt: time.Date(2026, 6, 12, 9, 17, 25, 0, time.UTC),
						},
					},
				},
			})
		case "/api/v1/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": []model.Project{}})
		case "/api/v1/search/options":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": model.SearchOptions{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, HTTPTimeout: time.Second})
	req := httptest.NewRequest(http.MethodGet, "/articles", nil)
	rr := httptest.NewRecorder()

	srv.handleArticles(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, unexpected := range []string{
		`class="col-status"`,
		`name="action" value="favorite"`,
		`<button type="submit">收藏</button>`,
		`<button type="submit">取消收藏</button>`,
	} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("unexpected article list UI fragment %q in body: %s", unexpected, body)
		}
	}
	renderedText := strings.ReplaceAll(html.UnescapeString(body), "&#43;", "+")
	for _, expected := range []string{
		`<th class="col-title">标题</th><th class="col-source">来源</th><th class="col-time">时间</th><th class="col-actions">操作</th>`,
		`金十`,
		`PANews`,
		`CoinDesk`,
		`Foresight`,
		`2026-06-12 14:17`,
		`Code By Yuhao@jiansutech.com - 2026-06-12 14:17:25 UTC+8 - abcdef1 - golang-jin10-sqlite`,
	} {
		if !strings.Contains(renderedText, expected) {
			t.Fatalf("expected article list UI fragment %q in body: %s", expected, body)
		}
	}
}

func TestPortalPageTemplatesUseCommonFooter(t *testing.T) {
	templates := map[string]string{
		"login":                   loginTemplate,
		"dashboard":               dashboardTemplate,
		"crawl_templates":         crawlTemplatesTemplate,
		"projects":                projectsTemplate,
		"project":                 projectTemplate,
		"rules":                   rulesTemplate,
		"rule":                    ruleTemplate,
		"articles":                articlesTemplate,
		"article":                 articleTemplate,
		"reports":                 reportsTemplate,
		"report":                  reportTemplate,
		"system_logs":             systemLogsTemplate,
		"logs":                    logsTemplate,
		"system":                  systemTemplate,
		"platform_bindings_work":  platformBindingsWorkbenchTemplate,
		"platform_workbench_v3":   platformWorkbenchTemplateV3,
		"public_option_workbench": publicOptionWorkbenchTemplate,
		"platform_bindings":       platformBindingsTemplate,
	}
	for name, source := range templates {
		if !strings.Contains(source, `{{template "footer" .}}`) {
			t.Fatalf("expected %s template to include common footer", name)
		}
		if strings.Contains(source, `</main></body></html>{{end}}`) {
			t.Fatalf("expected %s template to close through common footer", name)
		}
	}
}

func TestPortalNavPositionsLogoutTopRight(t *testing.T) {
	if !strings.Contains(portalNavHTML, `class="logout-link" href="/logout"`) {
		t.Fatalf("expected logout link to use fixed-position class")
	}
	if !strings.Contains(baseStyles, `.logout-link{position:fixed;top:14px;right:18px;`) {
		t.Fatalf("expected logout link to be positioned at top right")
	}
	if !strings.Contains(portalNavHTML, `<a href="/system">系统</a><a href="/logs">日志</a>`) {
		t.Fatalf("expected logs menu to appear immediately after system menu")
	}
}

func TestArticlesTemplateUsesSharedHeaderNavDirectly(t *testing.T) {
	for _, unexpected := range []string{
		`top-menu-card`,
		`<section class="top-menu-card">`,
	} {
		if strings.Contains(articlesTemplate, unexpected) {
			t.Fatalf("expected articles template to avoid menu card wrapper %q", unexpected)
		}
	}
	if !strings.Contains(articlesTemplate, `<header><div class="page-title"><div><h1>文章中心</h1></div></div>{{template "nav" .}}</header>`) {
		t.Fatal("expected articles template to render shared nav directly in header")
	}
}

func TestDashboardTemplateOmitsServiceStatusTable(t *testing.T) {
	for _, unexpected := range []string{
		`<h2>服务状态</h2>`,
		`{{range .Services}}`,
		`暂无服务状态`,
	} {
		if strings.Contains(dashboardTemplate, unexpected) {
			t.Fatalf("expected dashboard template to omit service status fragment %q", unexpected)
		}
	}
}

func TestSystemTemplateIncludesServiceRestartActions(t *testing.T) {
	for _, expected := range []string{
		`body>main,body>.site-footer{max-width:none;width:100%;box-sizing:border-box}`,
		`<th>操作</th>`,
		`class="service-actions"`,
		`.service-actions .service-button{display:inline-flex;align-items:center;justify-content:center;width:42px;min-width:42px;max-width:42px`,
		`name="form_type" value="restart_service"`,
		`name="service_name" value="{{.Name}}"`,
		`<button class="service-button" type="submit">重启</button>`,
		`<form method="get" action="/system/logs" class="service-action-form">`,
		`<button class="service-button" type="submit">日志</button>`,
	} {
		if !strings.Contains(systemTemplate, expected) {
			t.Fatalf("expected system template to include %q", expected)
		}
	}
}

func TestSystemTemplateGroupsRepeatedPanelsBySection(t *testing.T) {
	serviceTab := `href="/system?section=services">服务状态</a><a class="{{if eq .SectionKey "legacy"}}active{{end}}" href="/system?section=legacy">Legacy注册表`
	opActionsTab := `href="/system?section=operations">生产运行</a><a class="{{if eq .SectionKey "opactions"}}active{{end}}" href="/system?section=opactions">运营操作`
	contractsTab := `href="/system?section=opactions">运营操作</a><a class="{{if eq .SectionKey "contracts"}}active{{end}}" href="/system?section=contracts">外部契约与审计`
	announcementTab := `href="/system?section=contracts">外部契约与审计</a><a class="{{if eq .SectionKey "announcements"}}active{{end}}" href="/system?section=announcements">公告与任务`
	for _, expected := range []string{
		serviceTab,
		opActionsTab,
		contractsTab,
		announcementTab,
		`{{if eq .SectionKey "services"}}<section><h2>服务状态</h2>`,
		`{{if eq .SectionKey "legacy"}}<section class="section-block"><h2>Legacy 注册表</h2>`,
		`name="section" value="services"`,
		`{{if eq .SectionKey "operations"}}<section class="section-block"><h2>生产运行</h2>`,
		`{{if eq .SectionKey "opactions"}}<section class="section-block"><h2>运营操作</h2>`,
		`name="section" value="opactions"`,
		`{{if eq .SectionKey "contracts"}}<section class="section-block"><h2>外部契约与审计</h2>`,
		`{{if eq .SectionKey "announcements"}}<section class="section-block"><h2>公告与任务</h2>`,
		`.feedback-textarea{min-height:168px;resize:vertical}`,
		`<textarea class="feedback-textarea" name="content" placeholder="问题描述或需求"></textarea>`,
	} {
		if !strings.Contains(systemTemplate, expected) {
			t.Fatalf("expected system template to include grouped fragment %q", expected)
		}
	}
	if strings.Contains(systemTemplate, `</section><section><h2>服务状态</h2>`) {
		t.Fatal("expected service status panel to be scoped to services section")
	}
	if strings.Contains(systemTemplate, `{{end}}<section class="section-block"><h2>运营操作</h2>`) {
		t.Fatal("expected operations action panel to be scoped to operations section")
	}
	if strings.Contains(systemTemplate, `{{if eq .SectionKey "operations"}}<section class="section-block"><h2>运营操作</h2>`) {
		t.Fatal("expected operation action panel to move out of production operations section")
	}
	if strings.Contains(systemTemplate, `<div><h3>Legacy 注册表</h3>`) {
		t.Fatal("expected legacy registry card to move out of operations grid")
	}
	if strings.Contains(systemTemplate, `</section><section class="section-block"><h2>外部契约与审计</h2>`) {
		t.Fatal("expected external contract audit panel to move out of operations section")
	}
}

func TestPortalFooterDoesNotAppendSystemServiceLogLinks(t *testing.T) {
	if strings.Contains(portalFooterHTML, `service-log-link`) {
		t.Fatal("expected service log buttons to be rendered by system template, not appended by footer script")
	}
	if !strings.Contains(portalFooterHTML, `{{.FooterBranch}} - <a class="footer-feedback-link" href="/system?section=feedback">反馈建议</a>`) {
		t.Fatal("expected footer to link to feedback after branch")
	}
	if !strings.Contains(baseStyles, `.site-footer a{color:#214e34;font-weight:600;text-decoration:none}`) {
		t.Fatal("expected footer feedback link styling")
	}
}

func TestParseServiceLogEntries(t *testing.T) {
	entries := parseServiceLogEntries(strings.Join([]string{
		`2026-06-15T09:45:20+08:00 INF scheduler service ready service=scheduler-service`,
		`continuation detail`,
		`2026/06/15 09:46:05.014134 WARN RESTY Post "http://127.0.0.1:8083": context deadline exceeded`,
		`retry detail`,
		"\x1b[90m2026-06-15T14:22:38+08:00\x1b[0m \x1b[36mDBG\x1b[0m startup debug info \x1b[36mservice=\x1b[0mauth-service",
	}, "\n"))

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	if entries[0].Time != "2026-06-15T09:45:20+08:00" || entries[0].Level != "INF" {
		t.Fatalf("unexpected first entry metadata: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Message, "scheduler service ready") {
		t.Fatalf("unexpected first entry message: %q", entries[0].Message)
	}
	if entries[1].Time != "" || entries[1].Level != "" || entries[1].Message != "continuation detail" {
		t.Fatalf("expected standalone second line entry, got %+v", entries[1])
	}
	if entries[2].Time != "2026/06/15 09:46:05.014134" || entries[2].Level != "WARN" {
		t.Fatalf("unexpected third entry metadata: %+v", entries[2])
	}
	if !strings.Contains(entries[2].Message, "context deadline exceeded") {
		t.Fatalf("unexpected third entry message: %q", entries[2].Message)
	}
	if entries[3].Message != "retry detail" {
		t.Fatalf("expected standalone fourth line entry, got %+v", entries[3])
	}
	if entries[4].Time != "2026-06-15T14:22:38+08:00" || entries[4].Level != "DBG" {
		t.Fatalf("unexpected ANSI entry metadata: %+v", entries[4])
	}
	if strings.Contains(entries[4].Message, "\x1b") || !strings.Contains(entries[4].Message, "service=auth-service") {
		t.Fatalf("unexpected ANSI entry message: %q", entries[4].Message)
	}
}

func TestSystemLogsPageRendersTableAndPagination(t *testing.T) {
	lines := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		lines = append(lines, `2026-06-15T09:45:20+08:00 INF entry-`+strconv.Itoa(i))
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/services/content-service/logs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": map[string]string{
				"service": "content-service",
				"log":     strings.Join(lines, "\n"),
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, ServiceToken: "test-token", HTTPTimeout: time.Second})
	req := httptest.NewRequest(http.MethodGet, "/system/logs?service=content-service&page=2", nil)
	rr := httptest.NewRecorder()

	srv.handleSystemLogs(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, expected := range []string{
		`<th class="log-time">时间</th>`,
		`<th class="log-level">Level</th>`,
		`<th>具体内容</th>`,
		`第 2 / 2 页`,
		`entry-9`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected logs page to include %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, `entry-10`) {
		t.Fatalf("expected second page to omit first page entries, got %s", body)
	}
}

func TestLogsPageSummarizesServiceLogs(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service := strings.TrimPrefix(r.URL.Path, "/api/v1/system/services/")
		service = strings.TrimSuffix(service, "/logs")
		if service == r.URL.Path {
			http.NotFound(w, r)
			return
		}
		logText := ""
		if service == "crawler-service" {
			logText = strings.Join([]string{
				`2026-06-16T08:58:01+08:00 ERR crawler failed`,
				`2026-06-16T08:59:01+08:00 INF crawler ready`,
			}, "\n")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": map[string]string{
				"service": service,
				"log":     logText,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, ServiceToken: "test-token", HTTPTimeout: time.Second})
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	rr := httptest.NewRecorder()

	srv.handleLogsPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, expected := range []string{
		`日志中心`,
		`gateway-web`,
		`crawler-service`,
		`查看详情`,
		`/system/logs?service=crawler-service`,
		`crawler ready`,
		`<th class="log-time">时间</th>`,
		`<th class="log-level">Level</th>`,
		`<th>具体内容</th>`,
		`暂无日志或读取失败`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected logs page to include %q, got %s", expected, body)
		}
	}
}

func TestDisplayBoardCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/displayboard?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleDisplayBoard(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "综合看板") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "旧版主题区") || !strings.Contains(body, "头条热点") {
		t.Fatalf("expected legacy themed sections on board, got %s", body)
	}
	if !strings.Contains(body, "summary-strip") && !strings.Contains(body, "核心指标") {
		t.Fatalf("expected dashboard layout cues on board, got %s", body)
	}
	if !strings.Contains(body, "综合热点") || !strings.Contains(body, "微博热点") || !strings.Contains(body, "政策热点") {
		t.Fatalf("expected synthesize sections on board, got %s", body)
	}
	if !strings.Contains(body, "热点关键词") || !strings.Contains(body, "AI") {
		t.Fatalf("expected hotspot keywords on board, got %s", body)
	}
	if !strings.Contains(body, "项目快捷入口") || !strings.Contains(body, "项目一") {
		t.Fatalf("expected project links on board, got %s", body)
	}
	if !strings.Contains(body, "/fullsearch/result?searchword=AI&menuStyle=1&fulltype=8&page=1") {
		t.Fatalf("expected hot keyword search entry, got %s", body)
	}
}

func TestDisplayBoardCollection2Compat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/displayboard/collection2", nil)
	rr := httptest.NewRecorder()
	srv.handleDisplayBoardCollection2(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Data struct {
			Data []model.Item `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(envelope.Data.Data) == 0 {
		t.Fatalf("expected collection data, got %+v", envelope)
	}
}

func TestMonitorDetailCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/monitor/detail/100?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleMonitorCompat(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "监测详情") || !strings.Contains(body, "AI 观察日报") {
		t.Fatalf("expected monitor detail content, got %s", body)
	}
	if !strings.Contains(body, "/mobile/monitor?groupid=1&amp;projectid=1") {
		t.Fatalf("expected return link to mobile monitor, got %s", body)
	}
}

func TestProductManualCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	pageReq := httptest.NewRequest(http.MethodGet, "/system/productmanual/online", nil)
	pageRR := httptest.NewRecorder()
	srv.handleSystemProductManualOnline(pageRR, pageReq, map[string]any{"id": 1})
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", pageRR.Code)
	}
	body := pageRR.Body.String()
	if !strings.Contains(body, "产品手册") || !strings.Contains(body, "/system/uploadProductManual") {
		t.Fatalf("expected product manual page content, got %s", body)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/system/uploadProductManual", nil)
	downloadRR := httptest.NewRecorder()
	srv.handleSystemUploadProductManual(downloadRR, downloadReq, map[string]any{"id": 1})
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", downloadRR.Code)
	}
	if ct := downloadRR.Header().Get("Content-Type"); !strings.Contains(ct, "application/pdf") {
		t.Fatalf("expected pdf content type, got %q", ct)
	}
	if downloadRR.Body.Len() == 0 {
		t.Fatalf("expected pdf body, got empty response")
	}
}

func TestMonitorWxGroupCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/monitor/wxGroup", nil)
	rr := httptest.NewRecorder()
	srv.handleMonitorWxGroup(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"联系我们", "/assets/images/users/wxOfficialAccount.jpg", "/assets/images/users/wxGroup.jpg", "/assets/images/expireCode.jpg", "www.stonedt.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %s", want, body)
		}
	}
}

func TestVolumePageCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/volume?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleVolume(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "声量监测") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "AI") {
		t.Fatalf("expected volume page to include hot keyword, got %s", body)
	}
	if !strings.Contains(body, "/volume/getproject?groupid=1&projectid=1") {
		t.Fatalf("expected project link, got %s", body)
	}
}

func TestCrawlTemplatesPage(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/crawl-templates/manage", nil)
	rr := httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(rr, req, map[string]any{"id": 7})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "抓取模板管理") || !strings.Contains(body, "X BTC 热门账号模板") {
		t.Fatalf("expected template management page content, got %s", body)
	}
	if !strings.Contains(body, "/crawl-templates/manage") {
		t.Fatalf("expected crawl-templates manage nav link, got %s", body)
	}
	if !strings.Contains(body, `<select class="js-source-type-input" name="source_type">`) {
		t.Fatalf("expected source_type select on template management page, got %s", body)
	}
	if !strings.Contains(body, `name="config_method"`) || !strings.Contains(body, `name="config_base_url"`) || !strings.Contains(body, `name="config_list_selector"`) || !strings.Contains(body, `name="config_detail_url_field"`) {
		t.Fatalf("expected common config fields on template management page, got %s", body)
	}
	for _, expected := range []string{"抓取参数", "网站", "URL", "抓取区域", "href 字段", "x.com", "https://x.com/search?q=btc", ".tweet-card", "detail_url: a.tweet-link@href"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected crawl template parameter %q to render, got %s", expected, body)
		}
	}
	for _, expected := range []string{"模板列表（共 3 个）", "/crawl-templates/manage?template_id=2", "查看抓取页面"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected crawl template list entry %q to render, got %s", expected, body)
		}
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/crawl-templates/manage?template_id=2", nil)
	detailRR := httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(detailRR, detailReq, map[string]any{"id": 7})
	detailBody := detailRR.Body.String()
	for _, expected := range []string{"模板详情", "抓取页面", `href="https://x.com/search?q=btc"`, "X BTC 热门账号模板"} {
		if !strings.Contains(detailBody, expected) {
			t.Fatalf("expected crawl template detail %q to render, got %s", expected, detailBody)
		}
	}

	createForm := url.Values{
		"action":                  {"create"},
		"name":                    {"BTC 资讯模板"},
		"website":                 {"example.com"},
		"source_type":             {"crypto_x"},
		"config_method":           {"POST"},
		"config_base_url":         {"https://example.com/feed"},
		"config_list_selector":    {".feed-item"},
		"config_detail_url_field": {"href"},
		"config_json":             {`{"source_type":"crypto_x","base_url":"https://example.com"}`},
	}
	createReq := httptest.NewRequest(http.MethodPost, "/crawl-templates/manage", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRR := httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(createRR, createReq, map[string]any{"id": 7})
	if createRR.Code != http.StatusSeeOther {
		t.Fatalf("expected create redirect, got %d", createRR.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/crawl-templates/manage", nil)
	rr = httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(rr, req, map[string]any{"id": 7})
	if !strings.Contains(rr.Body.String(), "BTC 资讯模板") || !strings.Contains(rr.Body.String(), `&#34;website&#34;:&#34;example.com&#34;`) || !strings.Contains(rr.Body.String(), `&#34;method&#34;:&#34;POST&#34;`) || !strings.Contains(rr.Body.String(), `&#34;base_url&#34;:&#34;https://example.com/feed&#34;`) || !strings.Contains(rr.Body.String(), `&#34;list_selector&#34;:&#34;.feed-item&#34;`) || !strings.Contains(rr.Body.String(), `&#34;detail_url_field&#34;:&#34;href&#34;`) {
		t.Fatalf("expected created template to render, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "https://example.com/feed") || !strings.Contains(rr.Body.String(), ".feed-item") {
		t.Fatalf("expected created template parameters to render, got %s", rr.Body.String())
	}

	updateForm := url.Values{
		"action":                  {"update"},
		"template_id":             {"1"},
		"name":                    {"X BTC 热门账号模板 - 停用"},
		"website":                 {"x.com"},
		"source_type":             {"crypto_x"},
		"config_method":           {"POST"},
		"config_base_url":         {"https://example.com/updated"},
		"config_list_selector":    {".updated-item"},
		"config_detail_url_field": {"data-url"},
		"config_json":             {`{"source_type":"crypto_x","base_url":"https://example.com/updated"}`},
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/crawl-templates/manage", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRR := httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(updateRR, updateReq, map[string]any{"id": 7})
	if updateRR.Code != http.StatusSeeOther {
		t.Fatalf("expected update redirect, got %d", updateRR.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/crawl-templates/manage", nil)
	rr = httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(rr, req, map[string]any{"id": 7})
	if !strings.Contains(rr.Body.String(), "X BTC 热门账号模板 - 停用") || !strings.Contains(rr.Body.String(), `&#34;website&#34;:&#34;x.com&#34;`) || !strings.Contains(rr.Body.String(), `&#34;method&#34;:&#34;POST&#34;`) || !strings.Contains(rr.Body.String(), `&#34;detail_url_field&#34;:&#34;data-url&#34;`) {
		t.Fatalf("expected updated template name to render, got %s", rr.Body.String())
	}

	deleteForm := url.Values{
		"action":      {"delete"},
		"template_id": {"3"},
	}
	deleteReq := httptest.NewRequest(http.MethodPost, "/crawl-templates/manage", strings.NewReader(deleteForm.Encode()))
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteRR := httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(deleteRR, deleteReq, map[string]any{"id": 7})
	if deleteRR.Code != http.StatusSeeOther {
		t.Fatalf("expected delete redirect, got %d", deleteRR.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/crawl-templates/manage", nil)
	rr = httptest.NewRecorder()
	srv.handleCrawlTemplatesPage(rr, req, map[string]any{"id": 7})
	if strings.Contains(rr.Body.String(), "Telegram 交易所公告模板") {
		t.Fatalf("expected deleted template to disappear, got %s", rr.Body.String())
	}
}

func TestMobileMonitorCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/mobile/monitor", nil)
	rr := httptest.NewRecorder()
	srv.handleMobileMonitor(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "移动端监测") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "项目分组") || !strings.Contains(body, "项目一") {
		t.Fatalf("expected project group content, got %s", body)
	}
	if !strings.Contains(body, "/mobile/monitor/detail?groupid=1&projectid=1") {
		t.Fatalf("expected detail link, got %s", body)
	}
}

func TestMobileMonitorDetailCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/mobile/monitor/detail?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleMobileMonitorDetail(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "移动端详情") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "项目一") || !strings.Contains(body, "AI") {
		t.Fatalf("expected project details and articles, got %s", body)
	}
	if !strings.Contains(body, "预警消息") || !strings.Contains(body, "新能源 研判") {
		t.Fatalf("expected warning summary on mobile detail, got %s", body)
	}
}

func TestAnalysisCompatPage(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/analysis?projectid=1&timePeriod=7", nil)
	rr := httptest.NewRecorder()
	srv.handleAnalysisEntry(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "监测分析") {
		t.Fatalf("expected analysis page title, got %s", body)
	}
	if !strings.Contains(body, "AI 观察日报") || !strings.Contains(body, "新能源 研判") {
		t.Fatalf("expected analysis page to include latest news, got %s", body)
	}
	if !strings.Contains(body, "AI") {
		t.Fatalf("expected analysis page to include analysis keyword, got %s", body)
	}
}

func TestAnalysisCompatJSON(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	latestReq := httptest.NewRequest(http.MethodPost, "/analysis/latestnews", strings.NewReader("projectid=1&timePeriod=7"))
	latestReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	latestRR := httptest.NewRecorder()
	srv.handleAnalysisCompatJSON(latestRR, latestReq, map[string]any{"id": 1})
	if latestRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", latestRR.Code)
	}
	var latestEnvelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(latestRR.Body.Bytes(), &latestEnvelope); err != nil {
		t.Fatalf("unmarshal latestnews response: %v", err)
	}
	if len(latestEnvelope.Data) == 0 || latestEnvelope.Data[0]["title"] == "" {
		t.Fatalf("unexpected latestnews response: %+v", latestEnvelope)
	}

	refreshReq := httptest.NewRequest(http.MethodGet, "/analysis/updateanalysisdata?projectid=1", nil)
	refreshRR := httptest.NewRecorder()
	srv.handleAnalysisCompatJSON(refreshRR, refreshReq, map[string]any{"id": 1})
	if refreshRR.Code != http.StatusOK {
		t.Fatalf("expected 200 refresh, got %d", refreshRR.Code)
	}
	if !strings.Contains(refreshRR.Body.String(), "success") {
		t.Fatalf("unexpected refresh response: %s", refreshRR.Body.String())
	}
}

func TestPublicOptionCompatPages(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	listRR := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/publicoption", nil)
	srv.handlePublicOptionEntry(listRR, listReq, user)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected publicoption list 200, got %d", listRR.Code)
	}
	listBody := listRR.Body.String()
	if !strings.Contains(listBody, "事件分析工作台") || !strings.Contains(listBody, "AI 舆情研判") {
		t.Fatalf("expected publicoption list content, got %s", listBody)
	}
	if !strings.Contains(listBody, "/publicoption?id=1") {
		t.Fatalf("expected detail link on publicoption list, got %s", listBody)
	}

	detailRR := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/publicoption/reportdetail/1", nil)
	srv.handlePublicOptionCompat(detailRR, detailReq, user)
	if detailRR.Code != http.StatusGone {
		t.Fatalf("expected publicoption detail legacy route 410, got %d", detailRR.Code)
	}

	workbenchRR := httptest.NewRecorder()
	workbenchReq := httptest.NewRequest(http.MethodGet, "/publicoption?id=1", nil)
	srv.handlePublicOptionEntry(workbenchRR, workbenchReq, user)
	if workbenchRR.Code != http.StatusOK {
		t.Fatalf("expected publicoption workbench 200, got %d", workbenchRR.Code)
	}
	detailBody := workbenchRR.Body.String()
	if !strings.Contains(detailBody, "任务总览") || !strings.Contains(detailBody, "事件脉络内容") {
		t.Fatalf("expected publicoption detail content, got %s", detailBody)
	}

	analysisRR := httptest.NewRecorder()
	analysisReq := httptest.NewRequest(http.MethodGet, "/publicoption/backanalysis?id=1", nil)
	srv.handlePublicOptionCompat(analysisRR, analysisReq, user)
	if analysisRR.Code != http.StatusGone {
		t.Fatalf("expected publicoption analysis legacy route 410, got %d", analysisRR.Code)
	}

	routerGoneRR := httptest.NewRecorder()
	routerGoneReq := httptest.NewRequest(http.MethodGet, "/publicoption/eventTrace?id=1", nil)
	routerGoneReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	srv.Router().ServeHTTP(routerGoneRR, routerGoneReq)
	if routerGoneRR.Code != http.StatusGone {
		t.Fatalf("expected publicoption legacy route 410 through router, got %d", routerGoneRR.Code)
	}

	analysisPageRR := httptest.NewRecorder()
	analysisPageReq := httptest.NewRequest(http.MethodGet, "/publicoption?id=1&section=backanalysis", nil)
	srv.handlePublicOptionEntry(analysisPageRR, analysisPageReq, user)
	if analysisPageRR.Code != http.StatusOK {
		t.Fatalf("expected publicoption analysis page 200, got %d", analysisPageRR.Code)
	}
	analysisBody := analysisPageRR.Body.String()
	if !strings.Contains(analysisBody, "分析结果") || !strings.Contains(analysisBody, "回溯分析内容") {
		t.Fatalf("expected publicoption analysis content, got %s", analysisBody)
	}
}

func TestLegacyRouteRegistryStrategies(t *testing.T) {
	fullResult, ok := legacyRouteSpecForPath("/fullsearch/result")
	if !ok || fullResult.Strategy != legacyStrategyGone || fullResult.RemovalGate != legacyRemovalGateUIMigrated {
		t.Fatalf("unexpected fullsearch result legacy spec: %+v ok=%v", fullResult, ok)
	}

	fullDetail, ok := legacyRouteSpecForPath("/fullsearch/lawyerDetail/101")
	if !ok || fullDetail.Strategy != legacyStrategyGone || fullDetail.RemovalGate != legacyRemovalGateUIMigrated {
		t.Fatalf("unexpected fullsearch detail legacy spec: %+v ok=%v", fullDetail, ok)
	}

	fullJSON, ok := legacyRouteSpecForPath("/fullsearch/informationListpost")
	if !ok || fullJSON.Strategy != legacyStrategyGone || fullJSON.RemovalGate != legacyRemovalGateClientMigrated {
		t.Fatalf("unexpected fullsearch JSON legacy spec: %+v ok=%v", fullJSON, ok)
	}

	fullSearchResult, ok := legacyRouteSpecForPath("/fullsearch/getSearchResult")
	if !ok || fullSearchResult.Strategy != legacyStrategyGone || fullSearchResult.RemovalGate != legacyRemovalGateClientMigrated {
		t.Fatalf("unexpected fullsearch getSearchResult legacy spec: %+v ok=%v", fullSearchResult, ok)
	}

	lsearch, ok := legacyRouteSpecForPath("/industry")
	if !ok || lsearch.Strategy != legacyStrategyGone || lsearch.RemovalGate != legacyRemovalGateClientMigrated {
		t.Fatalf("unexpected lsearch legacy spec: %+v ok=%v", lsearch, ok)
	}

	publicOptionLoad, ok := legacyRouteSpecForPath("/publicoption/loadInformation")
	if !ok || publicOptionLoad.Strategy != legacyStrategyGone || publicOptionLoad.RemovalGate != legacyRemovalGateClientMigrated {
		t.Fatalf("unexpected publicoption loadInformation legacy spec: %+v ok=%v", publicOptionLoad, ok)
	}

	router := NewServer(config.Config{}).Router()
	routerGoneRR := httptest.NewRecorder()
	router.ServeHTTP(routerGoneRR, httptest.NewRequest(http.MethodGet, "/fullsearch/informationListpost?searchword=AI", nil))
	if routerGoneRR.Code != http.StatusGone {
		t.Fatalf("expected fullsearch JSON legacy route 410 through router, got %d", routerGoneRR.Code)
	}
	searchResultGoneRR := httptest.NewRecorder()
	router.ServeHTTP(searchResultGoneRR, httptest.NewRequest(http.MethodGet, "/fullsearch/getSearchResult?searchword=AI", nil))
	if searchResultGoneRR.Code != http.StatusGone {
		t.Fatalf("expected fullsearch getSearchResult legacy route 410 through router, got %d", searchResultGoneRR.Code)
	}

	detail, ok := legacyRouteSpecForPath("/publicoption/reportdetail/1")
	if !ok || detail.Strategy != legacyStrategyGone || detail.RemovalGate != legacyRemovalGateUIMigrated {
		t.Fatalf("unexpected publicoption detail legacy spec: %+v ok=%v", detail, ok)
	}

	platform, ok := legacyRouteSpecForPath("/platform/xie/report")
	if !ok || platform.Strategy != legacyStrategyGone || platform.RemovalGate != legacyRemovalGateExternalContract {
		t.Fatalf("unexpected platform legacy spec: %+v ok=%v", platform, ok)
	}

	if !isRemovedLegacyPortalPath("/user/save") {
		t.Fatalf("expected /user/save to be treated as removed")
	}
}

func TestFinalLegacyCompatRoutesAreGone(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	paths := []string{
		"/timelysearch/result?keyword=AI",
		"/platform/nlp/ocr",
		"/platform/xie/report",
		"/mobile/monitor",
		"/mobile/getGroupAndProject",
		"/mobile/mobileQRCode",
		"/mobile/uuid/1-1000/test",
		"/displayboard",
		"/displayboard/collection2",
		"/volume",
		"/volume/getproject",
		"/volume/projectname",
		"/hot/hotpage",
		"/hot/hotlist",
		"/dist/monitor",
		"/dist/yqapply",
		"/dist/applydatainfo",
		"/img/code",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
		rr := httptest.NewRecorder()
		srv.Router().ServeHTTP(rr, req)
		if rr.Code != http.StatusGone {
			t.Fatalf("expected 410 for final removed legacy route %s, got %d", path, rr.Code)
		}
	}

	if live := collectLegacyLiveRoutes(); len(live) != 0 {
		t.Fatalf("expected no live legacy routes after final cleanup, got %+v", live)
	}

	counts := map[legacyRouteStrategy]int{}
	for _, spec := range portalLegacyRoutes {
		counts[spec.Strategy]++
	}
	if counts[legacyStrategyProxy] != 0 || counts[legacyStrategyPreserve] != 0 || counts[legacyStrategyGone] != 76 || counts[legacyStrategyDelete] != 40 {
		t.Fatalf("unexpected legacy route counts after final cleanup: %+v", counts)
	}
}

func TestPublicOptionCompatMutations(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	createReq := httptest.NewRequest(http.MethodPost, "/publicoption/addpublicoptiondata", strings.NewReader("eventname=AI%E6%96%B0%E5%AE%9E%E9%AA%8C&eventkeywords=AI&eventstarttime=2026-06-01%2000:00:00&eventendtime=2026-06-04%2023:59:59&eventstopwords=%E5%9C%A8%E7%BA%BF"))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRR := httptest.NewRecorder()
	srv.handlePublicOptionCompat(createRR, createReq, user)
	if createRR.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d", createRR.Code)
	}
	var createEnvelope struct {
		Code int                `json:"code"`
		Msg  string             `json:"msg"`
		Data model.PublicOption `json:"data"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createEnvelope.Code != http.StatusOK || createEnvelope.Data.ID == 0 || createEnvelope.Data.EventName != "AI新实验" {
		t.Fatalf("unexpected create response: %+v", createEnvelope)
	}

	updateReq := httptest.NewRequest(http.MethodPost, "/publicoption/updatedatabyid", strings.NewReader("id=1&eventname=AI%E8%88%86%E6%83%85%E7%A0%94%E5%88%A4%E6%9B%B4%E6%96%B0&eventkeywords=AI&eventstarttime=2026-06-01%2000:00:00&eventendtime=2026-06-05%2023:59:59&eventstopwords=%E6%97%A0%E5%85%B3%E8%AF%8D"))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRR := httptest.NewRecorder()
	srv.handlePublicOptionCompat(updateRR, updateReq, user)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d", updateRR.Code)
	}
	var updateEnvelope struct {
		Code int                `json:"code"`
		Msg  string             `json:"msg"`
		Data model.PublicOption `json:"data"`
	}
	if err := json.Unmarshal(updateRR.Body.Bytes(), &updateEnvelope); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updateEnvelope.Data.ID != 1 || updateEnvelope.Data.EventName != "AI舆情研判更新" {
		t.Fatalf("unexpected update response: %+v", updateEnvelope)
	}
	if updateEnvelope.Data.BackAnalysis == "" || updateEnvelope.Data.EventContext == "" {
		t.Fatalf("expected enriched analyses on update, got %+v", updateEnvelope.Data)
	}

	loadReq := httptest.NewRequest(http.MethodPost, "/publicoption/loadInformation?page=1", strings.NewReader("eventname=AI%E8%88%86%E6%83%85%E7%A0%94%E5%88%A4%E6%9B%B4%E6%96%B0&eventkeywords=AI&eventstarttime=2026-06-01%2000:00:00&eventendtime=2026-06-05%2023:59:59&eventstopwords=%E6%97%A0%E5%85%B3%E8%AF%8D"))
	loadReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loadRR := httptest.NewRecorder()
	srv.handlePublicOptionCompat(loadRR, loadReq, user)
	if loadRR.Code != http.StatusGone {
		t.Fatalf("expected removed loadInformation 410, got %d", loadRR.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/publicoption/deletepublicoptioninfo", strings.NewReader("Ids=1"))
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteRR := httptest.NewRecorder()
	srv.handlePublicOptionCompat(deleteRR, deleteReq, user)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", deleteRR.Code)
	}
	var deleteEnvelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Deleted bool `json:"deleted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(deleteRR.Body.Bytes(), &deleteEnvelope); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if deleteEnvelope.Code != http.StatusOK || !deleteEnvelope.Data.Deleted {
		t.Fatalf("unexpected delete response: %+v", deleteEnvelope)
	}
}

func TestCrawlTemplateCompatPage(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/crawl-templates", nil)
	srv.handleCrawlTemplates(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected crawl templates page 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "模板中心") || !strings.Contains(body, "示例模板") {
		t.Fatalf("expected crawl templates page content, got %s", body)
	}
	if !strings.Contains(body, "最近执行记录") || !strings.Contains(body, "success") {
		t.Fatalf("expected crawl template run history, got %s", body)
	}
}

func TestCrawlTemplateCompatMutations(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	createForm := url.Values{}
	createForm.Set("form_type", "template")
	createForm.Set("name", "手填模板")
	createForm.Set("website", "news.example.com")
	createForm.Set("source_type", "headline")
	createForm.Set("config_method", "POST")
	createForm.Set("config_base_url", "https://example.com/api")
	createForm.Set("config_list_selector", ".article-card")
	createForm.Set("config_detail_url_field", "data-href")
	createForm.Set("enabled", "on")
	createForm.Set("config_json", `{"source_type":"headline","method":"GET","base_url":"https://example.com"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/crawl-templates", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRR := httptest.NewRecorder()
	srv.handleCrawlTemplates(createRR, createReq, user)
	if createRR.Code != http.StatusSeeOther {
		t.Fatalf("expected create redirect, got %d", createRR.Code)
	}

	listRR := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/crawl-templates", nil)
	srv.handleCrawlTemplates(listRR, listReq, user)
	if !strings.Contains(listRR.Body.String(), "手填模板") || !strings.Contains(listRR.Body.String(), `&#34;website&#34;:&#34;news.example.com&#34;`) || !strings.Contains(listRR.Body.String(), `&#34;method&#34;:&#34;POST&#34;`) || !strings.Contains(listRR.Body.String(), `&#34;base_url&#34;:&#34;https://example.com/api&#34;`) || !strings.Contains(listRR.Body.String(), `&#34;list_selector&#34;:&#34;.article-card&#34;`) || !strings.Contains(listRR.Body.String(), `&#34;detail_url_field&#34;:&#34;data-href&#34;`) {
		t.Fatalf("expected created template to appear on page, got %s", listRR.Body.String())
	}

	updateForm := url.Values{}
	updateForm.Set("form_type", "template")
	updateForm.Set("action", "update")
	updateForm.Set("template_id", "2")
	updateForm.Set("name", "手填模板更新")
	updateForm.Set("website", "flash.example.org")
	updateForm.Set("source_type", "flash")
	updateForm.Set("config_method", "POST")
	updateForm.Set("config_base_url", "https://example.org/feed")
	updateForm.Set("config_list_selector", ".flash-item")
	updateForm.Set("config_detail_url_field", "href")
	updateForm.Set("config_json", `{"source_type":"flash","method":"POST","base_url":"https://example.org"}`)
	updateReq := httptest.NewRequest(http.MethodPost, "/crawl-templates", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRR := httptest.NewRecorder()
	srv.handleCrawlTemplates(updateRR, updateReq, user)
	if updateRR.Code != http.StatusSeeOther {
		t.Fatalf("expected update redirect, got %d", updateRR.Code)
	}

	listRR = httptest.NewRecorder()
	srv.handleCrawlTemplates(listRR, listReq, user)
	listBody := listRR.Body.String()
	if !strings.Contains(listBody, "手填模板更新") || !strings.Contains(listBody, "flash.example.org") || !strings.Contains(listBody, `&#34;base_url&#34;:&#34;https://example.org/feed&#34;`) || !strings.Contains(listBody, `&#34;list_selector&#34;:&#34;.flash-item&#34;`) || !strings.Contains(listBody, "POST") {
		t.Fatalf("expected updated template to appear on page, got %s", listBody)
	}

	deleteForm := url.Values{}
	deleteForm.Set("form_type", "template")
	deleteForm.Set("action", "delete")
	deleteForm.Set("template_id", "2")
	deleteReq := httptest.NewRequest(http.MethodPost, "/crawl-templates", strings.NewReader(deleteForm.Encode()))
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteRR := httptest.NewRecorder()
	srv.handleCrawlTemplates(deleteRR, deleteReq, user)
	if deleteRR.Code != http.StatusSeeOther {
		t.Fatalf("expected delete redirect, got %d", deleteRR.Code)
	}

	listRR = httptest.NewRecorder()
	srv.handleCrawlTemplates(listRR, listReq, user)
	if strings.Contains(listRR.Body.String(), "手填模板更新") {
		t.Fatalf("expected deleted template to disappear, got %s", listRR.Body.String())
	}
}

func TestCrawlTemplateCompatExecution(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	previewReq := httptest.NewRequest(http.MethodPost, "/crawl-templates/1?action=preview", nil)
	previewRR := httptest.NewRecorder()
	srv.handleCrawlTemplates(previewRR, previewReq, user)
	if previewRR.Code != http.StatusSeeOther {
		t.Fatalf("expected preview redirect, got %d", previewRR.Code)
	}
	if loc := previewRR.Header().Get("Location"); !strings.Contains(loc, "msg=") {
		t.Fatalf("expected preview redirect message, got %s", loc)
	}

	runReq := httptest.NewRequest(http.MethodPost, "/crawl-templates/1?action=run", nil)
	runRR := httptest.NewRecorder()
	srv.handleCrawlTemplates(runRR, runReq, user)
	if runRR.Code != http.StatusSeeOther {
		t.Fatalf("expected run redirect, got %d", runRR.Code)
	}
	if loc := runRR.Header().Get("Location"); !strings.Contains(loc, "msg=") {
		t.Fatalf("expected run redirect message, got %s", loc)
	}
}

func TestLegacySystemAndUserCompat(t *testing.T) {
	srv := &Server{}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/preference?projectid=12&page=3", nil)
	srv.handleSystemSectionRedirect("preferences")(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "section=preferences") || !strings.Contains(loc, "project_id=12") || !strings.Contains(loc, "page=3") {
		t.Fatalf("unexpected redirect location: %s", loc)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/warningmsg?project_id=7&page=2&openFlag=1&keyword=%E9%92%A2%E9%93%81", nil)
	srv.handleSystemWarningMessage(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected warningmsg redirect, got %d", rr.Code)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse warningmsg redirect: %v", err)
	}
	if loc.Path != "/system" {
		t.Fatalf("unexpected redirect path: %s", loc.Path)
	}
	query := loc.Query()
	if query.Get("section") != "warningmsg" || query.Get("project_id") != "7" || query.Get("page") != "2" || query.Get("openFlag") != "1" || query.Get("keyword") != "钢铁" {
		t.Fatalf("unexpected warningmsg redirect query: %s", loc.RawQuery)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/warning?projectid=7&page=2", nil)
	srv.handleSystemWarningEdit(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected warning redirect, got %d", rr.Code)
	}
	loc, err = url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse warning redirect: %v", err)
	}
	if loc.Path != "/system" {
		t.Fatalf("unexpected warning redirect path: %s", loc.Path)
	}
	query = loc.Query()
	if query.Get("section") != "warning" || query.Get("project_id") != "7" || query.Get("page") != "2" {
		t.Fatalf("unexpected warning redirect query: %s", loc.RawQuery)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/user/123", nil)
	srv.handleUserCompat(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected user redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "section=account") {
		t.Fatalf("unexpected user redirect location: %s", loc)
	}
}

func TestLegacyUserSaveCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	body := strings.NewReader("telephone=13800000000&password=secret&display_name=运营账号&email=ops@example.com&status=1")
	req := httptest.NewRequest(http.MethodPost, "/user/save", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removed legacy route, got %d", rr.Code)
	}
}

func TestPlatformNLPCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	imageBytes := buildTestPNGBytes(t)

	ocrReq := httptest.NewRequest(http.MethodPost, "/platform/nlp/ocr", strings.NewReader("imageUrl="+url.QueryEscape(srv.cfg.GatewayWebURL+"/image/screenshot.png")))
	ocrReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ocrRR := httptest.NewRecorder()
	srv.handlePlatformCompat(ocrRR, ocrReq, map[string]any{"id": 1})
	if ocrRR.Code != http.StatusOK {
		t.Fatalf("expected OCR 200, got %d", ocrRR.Code)
	}
	var ocrEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   []struct {
			Data []struct {
				Text string `json:"text"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ocrRR.Body.Bytes(), &ocrEnvelope); err != nil {
		t.Fatalf("decode OCR response: %v", err)
	}
	if ocrEnvelope.Status != http.StatusOK || len(ocrEnvelope.Data) != 1 || len(ocrEnvelope.Data[0].Data) != 1 {
		t.Fatalf("unexpected OCR payload: %+v", ocrEnvelope)
	}
	if got := ocrEnvelope.Data[0].Data[0].Text; !strings.Contains(got, "screenshot") {
		t.Fatalf("expected OCR text to mention screenshot, got %q", got)
	}

	imageReq := httptest.NewRequest(http.MethodPost, "/platform/nlp/image", strings.NewReader("imageUrl="+url.QueryEscape(srv.cfg.GatewayWebURL+"/image/screenshot.png")))
	imageReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	imageRR := httptest.NewRecorder()
	srv.handlePlatformCompat(imageRR, imageReq, map[string]any{"id": 1})
	if imageRR.Code != http.StatusOK {
		t.Fatalf("expected image 200, got %d", imageRR.Code)
	}
	var imageEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			Result []struct {
				Keyword string `json:"keyword"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(imageRR.Body.Bytes(), &imageEnvelope); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if imageEnvelope.Status != http.StatusOK || len(imageEnvelope.Data.Result) == 0 {
		t.Fatalf("unexpected image payload: %+v", imageEnvelope)
	}

	multipartOCRReq := newMultipartNLPRequest(t, "/platform/nlp/ocr", "images", "screenshot.png", imageBytes)
	multipartOCRRR := httptest.NewRecorder()
	srv.handlePlatformCompat(multipartOCRRR, multipartOCRReq, map[string]any{"id": 1})
	if multipartOCRRR.Code != http.StatusOK {
		t.Fatalf("expected multipart OCR 200, got %d", multipartOCRRR.Code)
	}
	var multipartOCREnvelope struct {
		Data []struct {
			Data []struct {
				Text string `json:"text"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(multipartOCRRR.Body.Bytes(), &multipartOCREnvelope); err != nil {
		t.Fatalf("decode multipart OCR response: %v", err)
	}
	if len(multipartOCREnvelope.Data) != 1 || len(multipartOCREnvelope.Data[0].Data) != 1 {
		t.Fatalf("unexpected multipart OCR payload: %+v", multipartOCREnvelope)
	}
	if got := multipartOCREnvelope.Data[0].Data[0].Text; !strings.Contains(got, "screenshot") {
		t.Fatalf("expected multipart OCR text to mention screenshot, got %q", got)
	}

	multipartImageReq := newMultipartNLPRequest(t, "/platform/nlp/image", "file", "chart.png", imageBytes)
	multipartImageRR := httptest.NewRecorder()
	srv.handlePlatformCompat(multipartImageRR, multipartImageReq, map[string]any{"id": 1})
	if multipartImageRR.Code != http.StatusOK {
		t.Fatalf("expected multipart image 200, got %d", multipartImageRR.Code)
	}
	var multipartImageEnvelope struct {
		Data struct {
			Result []struct {
				Keyword string `json:"keyword"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(multipartImageRR.Body.Bytes(), &multipartImageEnvelope); err != nil {
		t.Fatalf("decode multipart image response: %v", err)
	}
	if len(multipartImageEnvelope.Data.Result) == 0 {
		t.Fatalf("unexpected multipart image payload: %+v", multipartImageEnvelope)
	}
}

func TestPlatformBindingsPage(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	pageRR := httptest.NewRecorder()
	pageReq := httptest.NewRequest(http.MethodGet, "/platform/bindings", nil)
	srv.handlePlatformCompat(pageRR, pageReq, user)
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected bindings page 200, got %d", pageRR.Code)
	}
	body := pageRR.Body.String()
	if !strings.Contains(body, "平台工作台") || !strings.Contains(body, "NLP 绑定") || !strings.Contains(body, "写作绑定") {
		t.Fatalf("expected bindings page content, got %s", body)
	}
	if !strings.Contains(body, "已绑定") {
		t.Fatalf("expected binding status, got %s", body)
	}

	form := url.Values{}
	form.Set("kind", "xie")
	form.Set("secret_id", "new-secret")
	form.Set("secret_key", "new-key")
	form.Set("bound", "on")
	postRR := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/platform/bindings", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handlePlatformCompat(postRR, postReq, user)
	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("expected bindings save redirect, got %d", postRR.Code)
	}
	if loc := postRR.Header().Get("Location"); !strings.Contains(loc, "msg=") {
		t.Fatalf("expected redirect message, got %s", loc)
	}
}

func TestPlatformBindingsWorkbenchShowsAuditLogs(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	form := url.Values{}
	form.Set("kind", "xie")
	form.Set("secret_id", "new-secret")
	form.Set("secret_key", "new-key")
	form.Set("bound", "on")
	postRR := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/platform/bindings", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handlePlatformCompat(postRR, postReq, user)
	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("expected bindings save redirect, got %d", postRR.Code)
	}

	pageRR := httptest.NewRecorder()
	pageReq := httptest.NewRequest(http.MethodGet, "/platform/bindings", nil)
	srv.handlePlatformCompat(pageRR, pageReq, user)
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected bindings page 200, got %d", pageRR.Code)
	}
	body := pageRR.Body.String()
	if !strings.Contains(body, "平台工作台") {
		t.Fatalf("expected platform workbench heading, got %s", body)
	}
	if !strings.Contains(body, "平台公告") || !strings.Contains(body, "最近平台操作") {
		t.Fatalf("expected platform notice and audit sections, got %s", body)
	}
	if !strings.Contains(body, "platform.binding.save") {
		t.Fatalf("expected platform audit log entry, got %s", body)
	}
}

func TestPlatformWorkbenchActions(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	rootRR := httptest.NewRecorder()
	rootReq := httptest.NewRequest(http.MethodGet, "/platform/", nil)
	srv.handlePlatformCompat(rootRR, rootReq, user)
	if rootRR.Code != http.StatusOK {
		t.Fatalf("expected platform root 200, got %d", rootRR.Code)
	}
	rootBody := rootRR.Body.String()
	if !strings.Contains(rootBody, "平台工作台") || !strings.Contains(rootBody, "OCR 识别") || !strings.Contains(rootBody, "写作报告预览") {
		t.Fatalf("expected upgraded platform workbench, got %s", rootBody)
	}

	ocrForm := url.Values{}
	ocrForm.Set("form_type", "nlp_ocr")
	ocrForm.Set("imageUrl", srv.cfg.GatewayWebURL+"/image/screenshot.png")
	ocrRR := httptest.NewRecorder()
	ocrReq := httptest.NewRequest(http.MethodPost, "/platform/bindings", strings.NewReader(ocrForm.Encode()))
	ocrReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handlePlatformCompat(ocrRR, ocrReq, user)
	if ocrRR.Code != http.StatusOK {
		t.Fatalf("expected workbench ocr 200, got %d", ocrRR.Code)
	}
	ocrBody := ocrRR.Body.String()
	if !strings.Contains(ocrBody, "OCR 识别已完成") || !strings.Contains(ocrBody, "screenshot") {
		t.Fatalf("expected OCR result in workbench, got %s", ocrBody)
	}

	titleForm := url.Values{}
	titleForm.Set("form_type", "xie_title")
	titleForm.Set("article_id", "101")
	titleForm.Set("text", "这是用于平台工作台生成标题的测试内容。")
	titleRR := httptest.NewRecorder()
	titleReq := httptest.NewRequest(http.MethodPost, "/platform/bindings", strings.NewReader(titleForm.Encode()))
	titleReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handlePlatformCompat(titleRR, titleReq, user)
	if titleRR.Code != http.StatusOK {
		t.Fatalf("expected workbench title 200, got %d", titleRR.Code)
	}
	titleBody := titleRR.Body.String()
	if !strings.Contains(titleBody, "标题已生成") {
		t.Fatalf("expected title generation message, got %s", titleBody)
	}

	reportForm := url.Values{}
	reportForm.Set("form_type", "xie_report")
	reportForm.Set("article_id", "101")
	reportForm.Set("title", "测试标题")
	reportForm.Set("relatedword", "AI")
	reportForm.Set("publishTime", "2026-06-10 12:00:00")
	reportForm.Set("text", "平台工作台报告预览正文。")
	reportRR := httptest.NewRecorder()
	reportReq := httptest.NewRequest(http.MethodPost, "/platform/bindings", strings.NewReader(reportForm.Encode()))
	reportReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handlePlatformCompat(reportRR, reportReq, user)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("expected workbench report 200, got %d", reportRR.Code)
	}
	reportBody := reportRR.Body.String()
	if !strings.Contains(reportBody, "报告预览已生成") || !strings.Contains(reportBody, "测试标题") || !strings.Contains(reportBody, "平台工作台报告预览正文") {
		t.Fatalf("expected report preview content, got %s", reportBody)
	}
	if !strings.Contains(reportBody, "platform.xie.report") && !strings.Contains(reportBody, "platform.nlp.ocr") {
		t.Fatalf("expected workbench audit log entries, got %s", reportBody)
	}
}

func TestPlatformXieCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	checkReq := httptest.NewRequest(http.MethodGet, "/platform/xie/checkBind", nil)
	checkRR := httptest.NewRecorder()
	srv.handlePlatformCompat(checkRR, checkReq, user)
	if checkRR.Code != http.StatusOK {
		t.Fatalf("expected checkBind 200, got %d", checkRR.Code)
	}
	var checkEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(checkRR.Body.Bytes(), &checkEnvelope); err != nil {
		t.Fatalf("decode checkBind response: %v", err)
	}
	if checkEnvelope.Status != http.StatusOK || checkEnvelope.Msg != "OK" {
		t.Fatalf("unexpected checkBind payload: %+v", checkEnvelope)
	}

	titleBody := `{"params":{"text":"<p>这是用于生成标题的测试内容，覆盖旧版写作宝兼容接口。</p>"}}`
	titleReq := httptest.NewRequest(http.MethodPost, "/platform/xie/title/101", strings.NewReader(titleBody))
	titleReq.Header.Set("Content-Type", "application/json")
	titleRR := httptest.NewRecorder()
	srv.handlePlatformCompat(titleRR, titleReq, user)
	if titleRR.Code != http.StatusOK {
		t.Fatalf("expected xie title 200, got %d", titleRR.Code)
	}
	var titleEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   string `json:"data"`
	}
	if err := json.Unmarshal(titleRR.Body.Bytes(), &titleEnvelope); err != nil {
		t.Fatalf("decode xie title response: %v", err)
	}
	if titleEnvelope.Status != http.StatusOK || titleEnvelope.Msg != "OK" || strings.TrimSpace(titleEnvelope.Data) == "" {
		t.Fatalf("unexpected xie title payload: %+v", titleEnvelope)
	}

	reportReq := httptest.NewRequest(http.MethodGet, "/platform/xie/report?articleId=101&projectId=7&relatedword=AI&publishTime=2026-06-10+12:00:00&title=%E6%B5%8B%E8%AF%95%E6%A0%87%E9%A2%98", nil)
	reportRR := httptest.NewRecorder()
	srv.handlePlatformCompat(reportRR, reportReq, user)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("expected xie report 200, got %d", reportRR.Code)
	}
	if contentType := reportRR.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %s", contentType)
	}
	body := reportRR.Body.String()
	if !strings.Contains(body, "event: start") || !strings.Contains(body, "event: message") || !strings.Contains(body, "event: end") {
		t.Fatalf("unexpected xie report stream: %s", body)
	}
	if !strings.Contains(body, "测试标题") && !strings.Contains(body, "AI 市场震荡") {
		t.Fatalf("expected generated report content, got %s", body)
	}
}

func TestPlatformNoticeCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/platform/notice", nil)
	rr := httptest.NewRecorder()
	srv.handlePlatformCompat(rr, req, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected notice 200, got %d", rr.Code)
	}
	var envelope struct {
		Status int                  `json:"status"`
		Msg    string               `json:"msg"`
		Data   []model.SystemNotice `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode notice response: %v", err)
	}
	if envelope.Status != http.StatusOK || envelope.Msg != "OK" || len(envelope.Data) == 0 {
		t.Fatalf("unexpected notice payload: %+v", envelope)
	}
}

func TestTemplateCrawlActionsAcrossPages(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	t.Run("project", func(t *testing.T) {
		form := url.Values{}
		form.Set("form_type", "crawl")
		form.Set("template_id", "1")
		form.Set("keyword", "AI")
		req := httptest.NewRequest(http.MethodPost, "/projects/1", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleProjectDetail(rr, req, user)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/projects/1") {
			t.Fatalf("expected project redirect, got %s", loc)
		}
		assertLastCrawlRequest(t, srv, "1", "", "AI")
	})

	t.Run("rule", func(t *testing.T) {
		form := url.Values{}
		form.Set("form_type", "crawl")
		form.Set("project_id", "1")
		form.Set("template_id", "1")
		form.Set("keyword", "AI")
		req := httptest.NewRequest(http.MethodPost, "/monitor-rules/1", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleRuleDetail(rr, req, user)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/monitor-rules/1") {
			t.Fatalf("expected rule redirect, got %s", loc)
		}
		assertLastCrawlRequest(t, srv, "1", "", "AI")
	})

	t.Run("report", func(t *testing.T) {
		form := url.Values{}
		form.Set("form_type", "crawl")
		form.Set("template_id", "1")
		form.Set("keyword", "AI")
		req := httptest.NewRequest(http.MethodPost, "/reports/1?return_to=/reports", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleReportDetail(rr, req, user)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/reports/1") {
			t.Fatalf("expected report redirect, got %s", loc)
		}
		assertLastCrawlRequest(t, srv, "1", "", "AI")
	})
}

func TestRemovedLegacyAPIRoutesReturn404(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	for _, path := range []string{
		"/api/getToken",
		"/api/getArticle",
		"/api/getMergeArticle",
		"/api/detail",
		"/monitor/exportarticle",
		"/project/names",
		"/project/groupandproject",
		"/project/mkdirgroup",
		"/project/getProjectCountByGroupId",
		"/project/editgroup",
		"/project/listproject",
		"/project/getGroupAndProject",
		"/project/verifygroup",
		"/project/getedit",
		"/project/commitproject",
		"/project/detail",
		"/project/commiteditproject",
		"/project/delProject",
		"/project/updateSolutionGroupStatus",
		"/project/batchUpdateProject",
		"/project/keywords",
		"/mail/checkMailConfig",
		"/mail/saveMailConfig",
		"/mail/getMailConfig",
		"/popUp/needPopUp",
		"/popUp/close",
		"/popUp/needContact",
		"/popUp/closeContact",
		"/datamonitor/addfavoritedata",
		"/datamonitor/isread",
		"/datamonitor/selectreadsign",
		"/datamonitor/copytext",
		"/datamonitor/updateemtion",
		"/datamonitor/sending",
		"/datamonitor/deletedata",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
		rr := httptest.NewRecorder()
		srv.Router().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for removed legacy route %s, got %d", path, rr.Code)
		}
	}
}

func TestLegacyLoginCompatRoutes(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	loginReq := httptest.NewRequest(http.MethodGet, "/login?reference=%2Fmonitor%3Fprojectid%3D1", nil)
	loginRR := httptest.NewRecorder()
	srv.Router().ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("expected login page 200, got %d", loginRR.Code)
	}
	if body := loginRR.Body.String(); !strings.Contains(body, `name="reference" value="/monitor?projectid=1"`) {
		t.Fatalf("expected login reference hidden field, got %s", body)
	}

	loginPostReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=secret&reference=%2Fmonitor%3Fprojectid%3D1"))
	loginPostReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginPostRR := httptest.NewRecorder()
	srv.Router().ServeHTTP(loginPostRR, loginPostReq)
	if loginPostRR.Code != http.StatusSeeOther {
		t.Fatalf("expected login post redirect, got %d", loginPostRR.Code)
	}
	if loc := loginPostRR.Header().Get("Location"); loc != "/monitor?projectid=1" {
		t.Fatalf("unexpected login redirect location: %s", loc)
	}

	loginBakReq := httptest.NewRequest(http.MethodGet, "/loginbak", nil)
	loginBakRR := httptest.NewRecorder()
	srv.Router().ServeHTTP(loginBakRR, loginBakReq)
	if loginBakRR.Code != http.StatusOK || !strings.Contains(loginBakRR.Body.String(), "Go 舆情系统") {
		t.Fatalf("unexpected loginbak response: code=%d body=%s", loginBakRR.Code, loginBakRR.Body.String())
	}

	forgotReq := httptest.NewRequest(http.MethodGet, "/forgotpwd", nil)
	forgotRR := httptest.NewRecorder()
	srv.Router().ServeHTTP(forgotRR, forgotReq)
	if forgotRR.Code != http.StatusOK || !strings.Contains(forgotRR.Body.String(), "忘记密码") {
		t.Fatalf("unexpected forgotpwd response: code=%d body=%s", forgotRR.Code, forgotRR.Body.String())
	}
	if !strings.Contains(forgotRR.Body.String(), "Code By Yuhao@jiansutech.com") {
		t.Fatalf("expected forgotpwd simple page to include common footer, got %s", forgotRR.Body.String())
	}
}

func TestLegacyJumpLoginFallbackRoutes(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	for _, path := range []string{"/jumpLogin?b64=test", "/wechatJumpLogin?userId=1&sha=test"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.Router().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for removed legacy route %s, got %d", path, rr.Code)
		}
	}
}

func TestLegacyOnlineStatisticalCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/onlinestatistical", nil)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removed legacy route, got %d", rr.Code)
	}
}

func TestLegacyReportCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	user := map[string]any{"id": int64(1), "username": "admin"}

	t.Run("report page alias", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/report?projectid=1&search=%E6%AF%8F%E6%97%A5&type=1&page=2", nil)
		rr := httptest.NewRecorder()
		srv.handleLegacyReportPage(rr, req, user)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "报告中心") || !strings.Contains(body, "AI 每日简报") {
			t.Fatalf("unexpected report page body: %s", body)
		}
		if !strings.Contains(body, "当前筛选已生效") {
			t.Fatalf("expected legacy filters to map into report page, got %s", body)
		}
	})

	t.Run("report detail alias", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/report/1?projectid=1&search=%E6%AF%8F%E6%97%A5&type=1&page=2", nil)
		rr := httptest.NewRecorder()
		srv.handleLegacyReportCompat(rr, req, user)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "报告详情") || !strings.Contains(body, "AI 每日简报") {
			t.Fatalf("unexpected report detail body: %s", body)
		}
		if !strings.Contains(body, "/reports?keyword=%e6%af%8f%e6%97%a5&amp;page=2&amp;project_id=1&amp;status=generated") {
			t.Fatalf("expected legacy return_to mapping on detail page, got %s", body)
		}
	})

	t.Run("report detail json", func(t *testing.T) {
		form := url.Values{}
		form.Set("reportId", "1")
		req := httptest.NewRequest(http.MethodPost, "/report/reportDetail", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleLegacyReportCustomDetailJSON(rr, req, user)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal report detail json: %v", err)
		}
		if payload["title"] != "AI 每日简报" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})

	t.Run("report list json", func(t *testing.T) {
		form := url.Values{}
		form.Set("pageNum", "1")
		form.Set("projectId", "1")
		form.Set("reportType", "1")
		form.Set("nameSearch", "AI")
		req := httptest.NewRequest(http.MethodPost, "/report/listReportCustom", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleLegacyReportCustomList(rr, req, user)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var envelope struct {
			Code int    `json:"code"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("unmarshal report list envelope: %v", err)
		}
		if envelope.Code != http.StatusOK || !strings.Contains(envelope.Data, "AI 每日简报") {
			t.Fatalf("unexpected report list payload: %+v", envelope)
		}
	})

	t.Run("batch archive", func(t *testing.T) {
		form := url.Values{}
		form.Set("reportIds", "1,2")
		req := httptest.NewRequest(http.MethodPost, "/report/batchUpdateReportCustom", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleLegacyBatchUpdateReportCustom(rr, req, user)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal batch archive: %v", err)
		}
		if payload["status"] != true {
			t.Fatalf("unexpected batch archive payload: %+v", payload)
		}
	})

	t.Run("batch status", func(t *testing.T) {
		form := url.Values{}
		form.Set("reportIds", "2,3")
		form.Set("reportType", "1")
		req := httptest.NewRequest(http.MethodPost, "/report/batchUpdateReportCustomStatus", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.handleLegacyBatchUpdateReportCustomStatus(rr, req, user)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal batch status: %v", err)
		}
		if payload["status"] != true {
			t.Fatalf("unexpected batch status payload: %+v", payload)
		}
	})
}

func TestSystemDatabaseSectionRendersPostgresConfig(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/system?section=database", nil)
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected system database page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, snippet := range []string{"数据库配置", "数据库连接检测与切换", "已选择驱动", "切换到 PostgreSQL", "切回 SQLite", "database_switch_postgres", "database_switch_sqlite", "YUQING_DB_DRIVER", "postgres_dsn"} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected database section to contain %q, got %s", snippet, body)
		}
	}
}

func TestSystemDatabaseSwitchRedirectsWithSuccess(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	form := url.Values{}
	form.Set("section", "database")
	form.Set("form_type", "database_switch_postgres")
	form.Set("driver", "sqlite")
	form.Set("sqlite_path", "data/yuqing.db")
	req := httptest.NewRequest(http.MethodPost, "/system?section=database", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after database switch, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "section=database") || !strings.Contains(location, url.QueryEscape("数据库切换配置已保存，正在自动重启全部服务")) {
		t.Fatalf("expected database switch success redirect, got %q", location)
	}

	form.Set("form_type", "database_switch_sqlite")
	req = httptest.NewRequest(http.MethodPost, "/system?section=database", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after sqlite switch, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func assertLastCrawlRequest(t *testing.T, srv *Server, wantTemplateID, wantSourceType, wantKeyword string) {
	t.Helper()
	_ = srv
	testLastCrawlRequest.Lock()
	defer testLastCrawlRequest.Unlock()
	if testLastCrawlRequest.templateID != wantTemplateID || testLastCrawlRequest.sourceType != wantSourceType || testLastCrawlRequest.keyword != wantKeyword {
		t.Fatalf("unexpected crawl request: template_id=%q source_type=%q keyword=%q", testLastCrawlRequest.templateID, testLastCrawlRequest.sourceType, testLastCrawlRequest.keyword)
	}
}

func newPortalCompatServer(t *testing.T) (*Server, func()) {
	t.Helper()
	var mu sync.Mutex
	var projectMu sync.Mutex
	var authMu sync.Mutex
	var bindingMu sync.Mutex
	mailCfg := model.MailConfig{}
	popupStates := map[string]model.PopupState{}
	deletedArticles := map[int64]bool{}
	emotions := map[int64]string{}
	shareChannels := map[int64][]string{}
	opinionConditions := map[int64]model.OpinionCondition{}
	warningSettings := map[int64]model.WarningSetting{}
	createdUsers := map[string]model.User{}
	auditLogs := []model.AuditLog{}
	apiTokens := map[string]model.APIToken{
		"legacy-token": {Token: "legacy-token", UserID: 1, Name: "legacy-api", CreatedAt: time.Now().UTC()},
	}
	sessionUsers := map[string]map[string]any{
		"session-admin": {"id": int64(1), "username": "admin", "display_name": "管理员", "role": "admin"},
	}
	publicOptions := map[int64]model.PublicOption{
		1: {
			ID:                  1,
			UserID:              1,
			EventName:           "AI 舆情研判",
			EventKeywords:       "AI,大模型",
			EventStopWords:      "无关词",
			EventStartTime:      "2026-06-01 00:00:00",
			EventEndTime:        "2026-06-04 23:59:59",
			CreateTime:          time.Now().UTC().Add(-48 * time.Hour),
			Updatetime:          time.Now().UTC().Add(-12 * time.Hour),
			Status:              1,
			DetailStatus:        1,
			EmotionalIndex:      "0.76",
			BackAnalysis:        "回溯分析内容",
			EventContext:        "事件脉络内容",
			EventTrace:          "事件跟踪内容",
			HotAnalysis:         "热点分析内容",
			NetizensAnalysis:    "网民分析内容",
			Statistics:          "统计内容",
			PropagationAnalysis: "传播分析内容",
			ThematicAnalysis:    "专题分析内容",
			UnscrambleContent:   "解读内容",
			ContentAnalysis:     "内容分析内容",
		},
	}
	crawlTemplatesMu := sync.Mutex{}
	crawlTemplates := []model.CrawlTemplate{
		{
			ID:         1,
			Name:       "示例模板",
			SourceType: "flash",
			Enabled:    true,
			ConfigJSON: `{"source_type":"flash","method":"GET"}`,
			CreatedAt:  time.Now().UTC().Add(-72 * time.Hour),
			UpdatedAt:  time.Now().UTC().Add(-6 * time.Hour),
		},
		{
			ID:         2,
			Name:       "X BTC 热门账号模板",
			Website:    "x.com",
			SourceType: "crypto_x",
			Enabled:    true,
			ConfigJSON: `{"source_type":"crypto_x","website":"x.com","method":"GET","base_url":"https://x.com/search?q=btc","list_selector":".tweet-card","detail_selector":"a.tweet-link","detail_url_field":"href","fields":[{"name":"title","selector":".tweet-text","scope":"list","required":true},{"name":"detail_url","selector":"a.tweet-link","attr":"href","scope":"list"}]}`,
			CreatedAt:  time.Now().UTC().Add(-48 * time.Hour),
			UpdatedAt:  time.Now().UTC().Add(-12 * time.Hour),
		},
		{
			ID:         3,
			Name:       "Telegram 交易所公告模板",
			Website:    "t.me",
			SourceType: "crypto_telegram",
			Enabled:    true,
			ConfigJSON: `{"source_type":"crypto_telegram","website":"t.me","base_url":"https://t.me/s/exchange_news","list_selector":".tgme_widget_message","detail_url_field":"href"}`,
			CreatedAt:  time.Now().UTC().Add(-24 * time.Hour),
			UpdatedAt:  time.Now().UTC().Add(-2 * time.Hour),
		},
	}
	nextCrawlTemplateID := int64(4)
	projectGroups := []model.ProjectGroup{
		{ID: 1, Name: "组一", Description: "测试项目组", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}
	projects := []model.Project{
		{ID: 1, GroupID: 1, GroupName: "组一", Name: "项目一", Keywords: "AI,新能源", Description: "测试项目", Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}
	reports := []model.Report{
		{ID: 1, ProjectID: 1, Title: "AI 每日简报", Summary: "AI 摘要", Content: "AI 正文", Status: "generated", CreatedAt: time.Now().UTC().Add(-3 * time.Hour), UpdatedAt: time.Now().UTC().Add(-2 * time.Hour)},
		{ID: 2, ProjectID: 1, Title: "AI 草稿报告", Summary: "草稿摘要", Content: "草稿正文", Status: "draft", CreatedAt: time.Now().UTC().Add(-90 * time.Minute), UpdatedAt: time.Now().UTC().Add(-80 * time.Minute)},
		{ID: 3, ProjectID: 1, Title: "AI 归档报告", Summary: "归档摘要", Content: "归档正文", Status: "archived", CreatedAt: time.Now().UTC().Add(-48 * time.Hour), UpdatedAt: time.Now().UTC().Add(-24 * time.Hour)},
	}
	nextGroupID := int64(2)
	nextProjectID := int64(2)
	platformBindings := map[string]model.PlatformBinding{
		"nlp:1": {
			UserID:    1,
			Kind:      "nlp",
			SecretID:  "secret-id",
			SecretKey: "secret-key",
			Bound:     true,
		},
		"xie:1": {
			UserID:    1,
			Kind:      "xie",
			SecretID:  "write-secret",
			SecretKey: sha1Hex("write-secret"),
			Bound:     true,
		},
	}
	nextUserID := int64(3)
	articles := map[int64]model.Item{
		99: {
			ID:         99,
			Title:      "测试标题",
			Content:    "测试内容",
			Summary:    "测试摘要",
			SourceType: "flash",
		},
		100: {
			ID:                 100,
			Title:              "AI 观察日报",
			Content:            "AI 相关内容",
			Summary:            "AI 摘要",
			SourceType:         "headline",
			FromText:           "新闻",
			PublishTimeText:    "2026-06-04 10:00:00",
			CapturedAt:         time.Now().UTC().Add(-2 * time.Hour),
			ProjectIDs:         []int64{1},
			ExternalSourceHost: "news.example.com",
		},
		102: {
			ID:              102,
			Title:           "微博热点追踪",
			Content:         "微博 热点 内容",
			Summary:         "微博 摘要",
			SourceType:      "weibo",
			FromText:        "微博",
			PublishTimeText: "2026-06-04 08:30:00",
			CapturedAt:      time.Now().UTC().Add(-90 * time.Minute),
		},
		103: {
			ID:              103,
			Title:           "抖音热评速览",
			Content:         "抖音 热点 内容",
			Summary:         "抖音 摘要",
			SourceType:      "douyin",
			FromText:        "抖音",
			PublishTimeText: "2026-06-04 08:20:00",
			CapturedAt:      time.Now().UTC().Add(-80 * time.Minute),
		},
		104: {
			ID:              104,
			Title:           "B站财经解读",
			Content:         "B站 热点 内容",
			Summary:         "B站 摘要",
			SourceType:      "bilibili",
			FromText:        "B站",
			PublishTimeText: "2026-06-04 08:10:00",
			CapturedAt:      time.Now().UTC().Add(-70 * time.Minute),
		},
		105: {
			ID:                 105,
			Title:              "36氪创业观察",
			Content:            "36kr 热点 内容",
			Summary:            "36kr 摘要",
			SourceType:         "36kr",
			FromText:           "36氪",
			PublishTimeText:    "2026-06-04 08:00:00",
			CapturedAt:         time.Now().UTC().Add(-60 * time.Minute),
			ExternalSourceHost: "36kr.com",
		},
		106: {
			ID:                 106,
			Title:              "国务院政策解读",
			Content:            "政策 热点 内容",
			Summary:            "政策 摘要",
			SourceType:         "gov",
			FromText:           "政策",
			PublishTimeText:    "2026-06-04 07:50:00",
			CapturedAt:         time.Now().UTC().Add(-50 * time.Minute),
			ExternalSourceHost: "gov.cn",
		},
		101: {
			ID:              101,
			Title:           "新能源 研判",
			Content:         "新能源 观察",
			Summary:         "新能源 摘要",
			SourceType:      "wechat",
			FromText:        "微信",
			PublishTimeText: "2026-06-04 09:30:00",
			CapturedAt:      time.Now().UTC().Add(-time.Hour),
			ProjectIDs:      []int64{1},
		},
	}
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope := func(code int, message string, data any) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    code,
				"message": message,
				"data":    data,
			})
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/keywords":
			writeEnvelope(http.StatusOK, "ok", []model.KeywordHotspot{
				{Keyword: "AI", Count: 12},
				{Keyword: "新能源", Count: 8},
				{Keyword: "港股", Count: 5},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/overview":
			writeEnvelope(http.StatusOK, "ok", model.DashboardSnapshot{
				Overview: model.Overview{ArticleCount: 3, ProjectCount: 1, ReportCount: 1, AlertRuleCount: 1},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/emotions":
			writeEnvelope(http.StatusOK, "ok", model.EmotionAnalysis{ProjectID: 1, Total: 3, Buckets: []model.EmotionBucket{{Name: "positive", Count: 2, Ratio: 0.66}, {Name: "neutral", Count: 1, Ratio: 0.33}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/event-overview":
			writeEnvelope(http.StatusOK, "ok", []model.EventOverview{{Keyword: "AI", Count: 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/propagation":
			writeEnvelope(http.StatusOK, "ok", model.PropagationAnalysis{ProjectID: 1, SourceFlow: []model.PropagationNode{{Label: "flash", Count: 2}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/themes":
			writeEnvelope(http.StatusOK, "ok", []model.ThemeInsight{{Name: "AI", Count: 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-opinion/events":
			writeEnvelope(http.StatusOK, "ok", []model.PublicOpinionEvent{{Title: "AI 舆情", Keyword: "AI", Count: 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-opinion/reports":
			writeEnvelope(http.StatusOK, "ok", []model.PublicOpinionReport{{Title: "AI 报告"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-opinion/enrich":
			writeEnvelope(http.StatusOK, "ok", model.PublicOpinionAnalysisBundle{
				EventName:           "AI 舆情研判",
				EventKeywords:       "AI,大模型",
				EventStopWords:      "无关词",
				EventStartTime:      "2026-06-01 00:00:00",
				EventEndTime:        "2026-06-04 23:59:59",
				EmotionalIndex:      "0.76",
				ArticleCount:        2,
				BackAnalysis:        "回溯分析内容",
				EventContext:        "事件脉络内容",
				EventTrace:          "事件跟踪内容",
				HotAnalysis:         "热点分析内容",
				NetizensAnalysis:    "网民分析内容",
				Statistics:          "统计内容",
				PropagationAnalysis: "传播分析内容",
				ThematicAnalysis:    "专题分析内容",
				UnscrambleContent:   "解读内容",
				ContentAnalysis:     "内容分析内容",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-opinion/analysis":
			writeEnvelope(http.StatusOK, "ok", model.PublicOpinionAnalysisView{
				Status:  "ok",
				Message: "analysis built from 2 articles",
				Bundle: model.PublicOpinionAnalysisBundle{
					EventName:           "AI 舆情研判",
					EventKeywords:       "AI,大模型",
					EventStopWords:      "无关词",
					EventStartTime:      "2026-06-01 00:00:00",
					EventEndTime:        "2026-06-04 23:59:59",
					EmotionalIndex:      "0.76",
					ArticleCount:        2,
					BackAnalysis:        "回溯分析内容",
					EventContext:        "事件脉络内容",
					EventTrace:          "事件跟踪内容",
					HotAnalysis:         "热点分析内容",
					NetizensAnalysis:    "网民分析内容",
					Statistics:          "统计内容",
					PropagationAnalysis: "传播分析内容",
					ThematicAnalysis:    "专题分析内容",
					UnscrambleContent:   "解读内容",
					ContentAnalysis:     "内容分析内容",
				},
				EventOverview: []model.EventOverview{{Keyword: "AI", Count: 2}},
				Emotions:      model.EmotionAnalysis{ProjectID: 1, Total: 2, Buckets: []model.EmotionBucket{{Name: "positive", Count: 1, Ratio: 0.5}, {Name: "negative", Count: 1, Ratio: 0.5}}},
				Propagation:   model.PropagationAnalysis{ProjectID: 1, SourceFlow: []model.PropagationNode{{Label: "headline", Count: 1}, {Label: "weibo", Count: 1}}},
				Themes:        []model.ThemeInsight{{Name: "AI", Count: 2}},
				Events:        []model.PublicOpinionEvent{{Title: "AI 舆情", Keyword: "AI", Count: 2}},
				Reports:       []model.PublicOpinionReport{{Title: "AI 报告"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/tasks/analysis/refresh":
			writeEnvelope(http.StatusOK, "ok", model.DashboardSnapshot{})
		default:
			writeEnvelope(http.StatusNotFound, "not found", nil)
		}
	}))
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope := func(code int, message string, data any) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    code,
				"message": message,
				"data":    data,
			})
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/project-groups":
			writeEnvelope(http.StatusOK, "ok", projectGroups)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/project-groups":
			var group model.ProjectGroup
			if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			group.ID = nextGroupID
			nextGroupID++
			group.CreatedAt = time.Now().UTC()
			group.UpdatedAt = group.CreatedAt
			projectGroups = append(projectGroups, group)
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", group)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/project-groups/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/project-groups/"))
			var group model.ProjectGroup
			if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			updated := false
			for idx, candidate := range projectGroups {
				if candidate.ID == id {
					group.ID = id
					group.CreatedAt = candidate.CreatedAt
					group.UpdatedAt = time.Now().UTC()
					projectGroups[idx] = group
					updated = true
					break
				}
			}
			projectMu.Unlock()
			if !updated {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", group)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/project-groups/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/project-groups/"))
			projectMu.Lock()
			kept := projectGroups[:0]
			removed := false
			for _, candidate := range projectGroups {
				if candidate.ID == id {
					removed = true
					continue
				}
				kept = append(kept, candidate)
			}
			projectGroups = kept
			projectMu.Unlock()
			if !removed {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			projectMu.Lock()
			items := append([]model.Project(nil), projects...)
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/reports":
			projectID := parseTestInt64(strings.TrimSpace(r.URL.Query().Get("project_id")))
			items := make([]model.Report, 0, len(reports))
			for _, report := range reports {
				if projectID > 0 && report.ProjectID != projectID {
					continue
				}
				items = append(items, report)
			}
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/reports/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/reports/"))
			for _, report := range reports {
				if report.ID == id {
					writeEnvelope(http.StatusOK, "ok", report)
					return
				}
			}
			writeEnvelope(http.StatusNotFound, "not found", nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/reports/batch-delete":
			var req struct {
				ReportIDs []int64 `json:"report_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			for i := range reports {
				for _, id := range req.ReportIDs {
					if reports[i].ID == id {
						reports[i].Status = "archived"
					}
				}
			}
			writeEnvelope(http.StatusOK, "ok", map[string]any{"updated": len(req.ReportIDs), "status": "archived"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/reports/batch-status":
			var req struct {
				ReportIDs []int64 `json:"report_ids"`
				Status    string  `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			for i := range reports {
				for _, id := range req.ReportIDs {
					if reports[i].ID == id {
						reports[i].Status = req.Status
					}
				}
			}
			writeEnvelope(http.StatusOK, "ok", map[string]any{"updated": len(req.ReportIDs), "status": req.Status})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates":
			writeEnvelope(http.StatusOK, "ok", crawlTemplates)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates/1":
			writeEnvelope(http.StatusOK, "ok", crawlTemplates[0])
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates/2":
			writeEnvelope(http.StatusOK, "ok", crawlTemplates[1])
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/crawl-templates":
			var tpl model.CrawlTemplate
			if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			if tpl.ConfigJSON == "" {
				tpl.ConfigJSON = "{}"
			}
			now := time.Now().UTC()
			tpl.ID = nextCrawlTemplateID
			nextCrawlTemplateID++
			tpl.Enabled = true
			tpl.CreatedAt = now
			tpl.UpdatedAt = now
			crawlTemplates = append([]model.CrawlTemplate{tpl}, crawlTemplates...)
			writeEnvelope(http.StatusOK, "ok", tpl)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/crawl-templates/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/crawl-templates/"))
			var tpl model.CrawlTemplate
			if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			updated := false
			for idx := range crawlTemplates {
				if crawlTemplates[idx].ID != id {
					continue
				}
				tpl.ID = id
				tpl.CreatedAt = crawlTemplates[idx].CreatedAt
				tpl.UpdatedAt = time.Now().UTC()
				crawlTemplates[idx] = tpl
				updated = true
				break
			}
			if !updated {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", tpl)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/crawl-templates/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/crawl-templates/"))
			kept := crawlTemplates[:0]
			removed := false
			for _, tpl := range crawlTemplates {
				if tpl.ID == id {
					removed = true
					continue
				}
				kept = append(kept, tpl)
			}
			crawlTemplates = kept
			if !removed {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			var project model.Project
			if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			project.ID = nextProjectID
			nextProjectID++
			project.CreatedAt = time.Now().UTC()
			project.UpdatedAt = project.CreatedAt
			if project.Status == "" {
				project.Status = "active"
			}
			for _, group := range projectGroups {
				if group.ID == project.GroupID {
					project.GroupName = group.Name
					break
				}
			}
			projects = append(projects, project)
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", project)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			projectMu.Lock()
			var item model.Project
			for _, candidate := range projects {
				if candidate.ID == id {
					item = candidate
					break
				}
			}
			projectMu.Unlock()
			if item.ID == 0 {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			var project model.Project
			if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			updated := false
			for idx, candidate := range projects {
				if candidate.ID == id {
					project.ID = id
					project.CreatedAt = candidate.CreatedAt
					project.UpdatedAt = time.Now().UTC()
					if project.Status == "" {
						project.Status = candidate.Status
					}
					for _, group := range projectGroups {
						if group.ID == project.GroupID {
							project.GroupName = group.Name
							break
						}
					}
					projects[idx] = project
					updated = true
					break
				}
			}
			projectMu.Unlock()
			if !updated {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", project)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			projectMu.Lock()
			kept := projects[:0]
			removed := false
			for _, candidate := range projects {
				if candidate.ID == id {
					removed = true
					continue
				}
				kept = append(kept, candidate)
			}
			projects = kept
			projectMu.Unlock()
			if !removed {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"))
			projectMu.Lock()
			item, ok := opinionConditions[id]
			projectMu.Unlock()
			if !ok {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"))
			var condition model.OpinionCondition
			if err := json.NewDecoder(r.Body).Decode(&condition); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			condition.ProjectID = id
			if condition.Time == 0 {
				condition.Time = 4
			}
			if condition.Emotion == "" {
				condition.Emotion = "[1,2,3]"
			}
			if condition.Sort == 0 {
				condition.Sort = 1
			}
			if condition.Matchs == 0 {
				condition.Matchs = 1
			}
			projectMu.Lock()
			opinionConditions[id] = condition
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", condition)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/system/warning-settings/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/warning-settings/"))
			projectMu.Lock()
			item, ok := warningSettings[id]
			projectMu.Unlock()
			if !ok {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/system/warning-settings/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/warning-settings/"))
			var setting model.WarningSetting
			if err := json.NewDecoder(r.Body).Decode(&setting); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			setting.ProjectID = id
			setting.Enabled = setting.Enabled || setting.WarningStatus == 1
			projectMu.Lock()
			warningSettings[id] = setting
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", setting)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-options":
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
			items := make([]model.PublicOption, 0, len(publicOptions))
			for _, option := range publicOptions {
				if userID > 0 && option.UserID != userID {
					continue
				}
				if keyword != "" && !strings.Contains(strings.ToLower(option.EventName+" "+option.EventKeywords), keyword) {
					continue
				}
				items = append(items, option)
			}
			sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/public-options":
			var option model.PublicOption
			if err := json.NewDecoder(r.Body).Decode(&option); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			option.ID = int64(len(publicOptions) + 1)
			option.CreateTime = time.Now().UTC()
			option.Updatetime = option.CreateTime
			publicOptions[option.ID] = option
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", option)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/public-options/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/public-options/"))
			projectMu.Lock()
			option, ok := publicOptions[id]
			projectMu.Unlock()
			if !ok {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", option)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/public-options/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/public-options/"))
			var option model.PublicOption
			if err := json.NewDecoder(r.Body).Decode(&option); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			if _, ok := publicOptions[id]; !ok {
				projectMu.Unlock()
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			option.ID = id
			option.CreateTime = publicOptions[id].CreateTime
			option.Updatetime = time.Now().UTC()
			publicOptions[id] = option
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", option)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/public-options/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/public-options/"))
			projectMu.Lock()
			_, ok := publicOptions[id]
			if ok {
				delete(publicOptions, id)
			}
			projectMu.Unlock()
			if !ok {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates":
			crawlTemplatesMu.Lock()
			items := append([]model.CrawlTemplate(nil), crawlTemplates...)
			crawlTemplatesMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/crawl-templates":
			var tpl model.CrawlTemplate
			if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			if strings.TrimSpace(tpl.ConfigJSON) == "" {
				tpl.ConfigJSON = "{}"
			}
			crawlTemplatesMu.Lock()
			tpl.ID = nextCrawlTemplateID
			nextCrawlTemplateID++
			tpl.CreatedAt = time.Now().UTC()
			tpl.UpdatedAt = tpl.CreatedAt
			crawlTemplates = append(crawlTemplates, tpl)
			crawlTemplatesMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", tpl)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/crawl-templates/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/crawl-templates/"))
			var tpl model.CrawlTemplate
			if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			if strings.TrimSpace(tpl.ConfigJSON) == "" {
				tpl.ConfigJSON = "{}"
			}
			crawlTemplatesMu.Lock()
			updated := false
			for idx, candidate := range crawlTemplates {
				if candidate.ID == id {
					tpl.ID = id
					tpl.CreatedAt = candidate.CreatedAt
					tpl.UpdatedAt = time.Now().UTC()
					crawlTemplates[idx] = tpl
					updated = true
					break
				}
			}
			crawlTemplatesMu.Unlock()
			if !updated {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", tpl)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/crawl-templates/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/crawl-templates/"))
			crawlTemplatesMu.Lock()
			kept := crawlTemplates[:0]
			removed := false
			for _, candidate := range crawlTemplates {
				if candidate.ID == id {
					removed = true
					continue
				}
				kept = append(kept, candidate)
			}
			crawlTemplates = kept
			crawlTemplatesMu.Unlock()
			if !removed {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/search/full":
			query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
			items := make([]model.Item, 0, len(articles))
			for _, item := range articles {
				blob := strings.ToLower(item.Title + " " + item.Content + " " + item.Summary + " " + item.FromText + " " + item.SourceType)
				if query != "" && !strings.Contains(blob, query) {
					continue
				}
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
			writeEnvelope(http.StatusOK, "ok", model.SearchResult{Total: len(items), PageSize: 10, Page: 1, Items: items})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles":
			projectID := parseTestInt64(r.URL.Query().Get("project_id"))
			items := make([]model.Item, 0, len(articles))
			for _, item := range articles {
				if projectID > 0 && len(item.ProjectIDs) > 0 {
					matched := false
					for _, pid := range item.ProjectIDs {
						if pid == projectID {
							matched = true
							break
						}
					}
					if !matched {
						continue
					}
				}
				items = append(items, item)
			}
			writeEnvelope(http.StatusOK, "ok", model.ItemListResult{Items: items, Page: 1, PageSize: 20, Total: len(items)})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/mail-config":
			mu.Lock()
			cfg := mailCfg
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", cfg)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/mail-config":
			var cfg model.MailConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			cfg.UpdatedAt = time.Now().UTC()
			mu.Lock()
			mailCfg = cfg
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", cfg)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/database-config":
			writeEnvelope(http.StatusOK, "ok", model.DatabaseConfigStatus{
				Driver:           "sqlite",
				ConfiguredDriver: "sqlite",
				RuntimeDriver:    "sqlite",
				Status:           "ok",
				Message:          "sqlite ready: data/yuqing.db",
				ConfigPath:       "data/database-config.json",
				SQLitePath:       "data/yuqing.db",
				PostgresHost:     "127.0.0.1",
				PostgresPort:     "5432",
				PostgresDatabase: "yuqing",
				PostgresUser:     "postgres",
				PostgresSSLMode:  "disable",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/database-config/check":
			writeEnvelope(http.StatusOK, "postgresql connection ok", model.DatabaseConfigStatus{Driver: "postgres", Status: "ok", Message: "postgresql connection ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/database-config/switch":
			writeEnvelope(http.StatusOK, "database switch saved; restarting all services to apply", model.DatabaseConfigStatus{
				Driver:           "sqlite",
				ConfiguredDriver: "sqlite",
				RuntimeDriver:    "sqlite",
				Status:           "ok",
				Message:          "database switch saved; restarting all services to apply",
				ConfigPath:       "data/database-config.json",
				RestartRequired:  true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			key := r.URL.Query().Get("key")
			mu.Lock()
			state, ok := popupStates[popupStateMapKey(userID, key)]
			mu.Unlock()
			if !ok {
				state = model.PopupState{UserID: userID, Key: key}
			}
			writeEnvelope(http.StatusOK, "ok", state)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/popup":
			var state model.PopupState
			if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			state.UpdatedAt = time.Now().UTC()
			if state.Dismissed && state.DismissedAt == nil {
				now := state.UpdatedAt
				state.DismissedAt = &now
			}
			mu.Lock()
			popupStates[popupStateMapKey(state.UserID, state.Key)] = state
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", state)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/audit-logs":
			if r.Header.Get("X-Service-Token") != "test-token" {
				writeEnvelope(http.StatusUnauthorized, "unauthorized", nil)
				return
			}
			var entry model.AuditLog
			if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			entry.ID = int64(len(auditLogs) + 1)
			entry.CreatedAt = time.Now().UTC()
			mu.Lock()
			auditLogs = append(auditLogs, entry)
			mu.Unlock()
			writeEnvelope(http.StatusCreated, "ok", entry)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/audit-logs":
			mu.Lock()
			items := append([]model.AuditLog(nil), auditLogs...)
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/notices":
			writeEnvelope(http.StatusOK, "ok", []model.SystemNotice{
				{ID: 1, Title: "系统公告", Content: "兼容测试公告", CreatedAt: time.Now().UTC()},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"))
			mu.Lock()
			item, ok := articles[id]
			deleted := deletedArticles[id]
			emotion := emotions[id]
			shares := append([]string(nil), shareChannels[id]...)
			mu.Unlock()
			if !ok || deleted {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			if userID > 0 {
				item.Favorited = item.Favorited
				item.Read = item.Read
			}
			if emotion != "" {
				item.TagFlags = emotion
			}
			if len(shares) > 0 {
				item.FromText = strings.Join(shares, ",")
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/related"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/related"))
			mu.Lock()
			items := make([]model.Item, 0, len(articles))
			for itemID, item := range articles {
				if itemID == id || deletedArticles[itemID] {
					continue
				}
				items = append(items, item)
			}
			mu.Unlock()
			if len(items) > 3 {
				items = items[:3]
			}
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/emotion"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/emotion"))
			emotion := nonEmpty(r.URL.Query().Get("emotion"), r.URL.Query().Get("flag"))
			mu.Lock()
			emotions[id] = emotion
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]any{"emotion": emotion})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"))
			mu.Lock()
			deletedArticles[id] = true
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/favorite"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/favorite"))
			mu.Lock()
			item := articles[id]
			item.Favorited = true
			articles[id] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"favorited": true})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/read"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/read"))
			mu.Lock()
			item := articles[id]
			item.Read = true
			articles[id] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"read": true})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/read"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/read"))
			mu.Lock()
			item := articles[id]
			item.Read = false
			articles[id] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"read": false})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/share"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/share"))
			var payload struct {
				Channel string `json:"channel"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			mu.Lock()
			shareChannels[id] = append(shareChannels[id], payload.Channel)
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]any{"shared": true, "channel": payload.Channel})
		case strings.HasPrefix(r.URL.Path, "/api/v1/platform/bindings/"):
			kind := strings.TrimPrefix(r.URL.Path, "/api/v1/platform/bindings/")
			key := kind + ":" + strconv.FormatInt(parseTestInt64(r.URL.Query().Get("user_id")), 10)
			if r.Method == http.MethodGet {
				bindingMu.Lock()
				binding, ok := platformBindings[key]
				bindingMu.Unlock()
				if !ok {
					writeEnvelope(http.StatusNotFound, "not found", nil)
					return
				}
				writeEnvelope(http.StatusOK, "ok", binding)
				return
			}
			if r.Method == http.MethodPost {
				var binding model.PlatformBinding
				if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
					writeEnvelope(http.StatusBadRequest, err.Error(), nil)
					return
				}
				binding.Kind = kind
				binding.Bound = true
				bindingMu.Lock()
				platformBindings[key] = binding
				bindingMu.Unlock()
				writeEnvelope(http.StatusOK, "ok", binding)
				return
			}
			writeEnvelope(http.StatusMethodNotAllowed, "method not allowed", nil)
		default:
			writeEnvelope(http.StatusNotFound, "not found", nil)
		}
	}))

	ocrImage := createTestImageServer(t)
	nlpServer := httptest.NewServer(nlp.NewService().Router())
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Service-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "unauthorized", "data": nil})
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/tasks/crawl/templates/preview":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": map[string]any{
					"source_type":   "flash",
					"fetched_count": 1,
					"run_id":        0,
					"items": []model.Item{
						{Title: "预览文章", Summary: "预览摘要", Content: "预览内容"},
					},
				},
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/tasks/crawl/templates/run":
			var tpl model.CrawlTemplate
			_ = json.NewDecoder(r.Body).Decode(&tpl)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": map[string]any{
					"source_type":    "flash",
					"fetched_count":  1,
					"inserted_count": 1,
					"updated_count":  0,
					"run_id":         9,
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": []model.CrawlRun{{ID: 8, TemplateID: 1, TemplateName: "示例模板", SourceType: "flash", Status: "success", FetchedCount: 1, InsertedCount: 1, StartedAt: time.Now().UTC().Add(-time.Hour)}}})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/tasks/crawl":
			testLastCrawlRequest.Lock()
			testLastCrawlRequest.templateID = r.URL.Query().Get("template_id")
			testLastCrawlRequest.sourceType = r.URL.Query().Get("source_type")
			testLastCrawlRequest.keyword = r.URL.Query().Get("keyword")
			testLastCrawlRequest.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": map[string]any{
					"source_type":    "flash",
					"fetched_count":  1,
					"inserted_count": 1,
					"updated_count":  0,
					"run_id":         8,
				},
			})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusNotFound, "message": "not found", "data": nil})
		}
	}))

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Service-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "unauthorized", "data": nil})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login" {
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": err.Error(), "data": nil})
				return
			}
			if strings.TrimSpace(req.Username) != "admin" || strings.TrimSpace(req.Password) != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "invalid credentials", "data": nil})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data":    map[string]any{"session_token": "session-admin"},
			})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/tokens" {
			if strings.TrimSpace(r.URL.Query().Get("session_token")) != "session-admin" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "invalid session", "data": nil})
				return
			}
			token := apiTokens["legacy-token"]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusCreated,
				"message": "ok",
				"data":    token,
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me" {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader != "Bearer legacy-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "invalid token", "data": nil})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data":    sessionUsers["session-admin"],
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/session" {
			user, ok := sessionUsers[strings.TrimSpace(r.URL.Query().Get("session_token"))]
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "invalid session", "data": nil})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data":    map[string]any{"user": user},
			})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/users" {
			var req struct {
				Username       string `json:"username"`
				Telephone      string `json:"telephone"`
				Password       string `json:"password"`
				DisplayName    string `json:"display_name"`
				Email          string `json:"email"`
				Role           string `json:"role"`
				Status         *int   `json:"status"`
				TermOfValidity string `json:"term_of_validity"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": err.Error(), "data": nil})
				return
			}
			username := nonEmpty(req.Username, req.Telephone)
			if username == "" || strings.TrimSpace(req.Password) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": "invalid body", "data": nil})
				return
			}
			authMu.Lock()
			if _, ok := createdUsers[username]; ok {
				authMu.Unlock()
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusConflict, "message": "user already exists", "data": nil})
				return
			}
			id := nextUserID
			nextUserID++
			user := model.User{
				ID:          id,
				Username:    username,
				DisplayName: nonEmpty(req.DisplayName, username),
				Email:       req.Email,
				Role:        nonEmpty(req.Role, "user"),
				Status:      1,
			}
			if req.Status != nil {
				user.Status = *req.Status
			}
			if req.TermOfValidity != "" {
				if parsed, err := time.Parse(time.RFC3339, req.TermOfValidity); err == nil {
					user.TermOfValidity = parsed
				}
			}
			createdUsers[username] = user
			authMu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusCreated,
				"message": "ok",
				"data":    map[string]any{"user": user},
			})
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/users/") && strings.HasSuffix(r.URL.Path, "/password") {
			if strings.TrimSpace(r.URL.Query().Get("session_token")) != "session-admin" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "invalid session", "data": nil})
				return
			}
			var req struct {
				OldPassword string `json:"old_password"`
				NewPassword string `json:"new_password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": err.Error(), "data": nil})
				return
			}
			if strings.TrimSpace(req.OldPassword) != "password123" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": "old password mismatch", "data": nil})
				return
			}
			if strings.TrimSpace(req.NewPassword) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": "new password required", "data": nil})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data":    map[string]any{"updated": true},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/wechat/getQrCode" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": map[string]any{
					"qrcodeUrl": "https://example.com/qr",
					"sceneStr":  "scene-1",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusNotFound, "message": "not found", "data": nil})
	}))

	srv := &Server{cfg: config.Config{ContentURL: content.URL, AuthURL: auth.URL, WechatURL: auth.URL, NLPURL: nlpServer.URL, GatewayWebURL: ocrImage.URL, AnalysisURL: analysis.URL, CrawlerURL: crawler.URL, ServiceToken: "test-token"}, client: resty.New().SetHeader("X-Service-Token", "test-token"), templates: NewServer(config.Config{}).templates}
	return srv, func() {
		ocrImage.Close()
		nlpServer.Close()
		analysis.Close()
		auth.Close()
		crawler.Close()
		content.Close()
	}
}

func createTestImageServer(t *testing.T) *httptest.Server {
	t.Helper()
	body := buildTestPNGBytes(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func buildTestPNGBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func newMultipartNLPRequest(t *testing.T, path, field, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create multipart form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func seedPopupState(t *testing.T, srv *Server, state model.PopupState) {
	t.Helper()
	if _, err := srv.putPopupState(state); err != nil {
		t.Fatalf("seed popup state: %v", err)
	}
}

func getStoredPopupState(t *testing.T, srv *Server, userID int64, key string) model.PopupState {
	t.Helper()
	state, ok, err := srv.getPopupState(userID, key)
	if err != nil {
		t.Fatalf("get popup state: %v", err)
	}
	if !ok {
		t.Fatalf("expected popup state to exist for user %d key %q", userID, key)
	}
	return state
}

func popupStateMapKey(userID int64, key string) string {
	return strconv.FormatInt(userID, 10) + ":" + key
}

func parseTestInt64(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func timePtr(t time.Time) *time.Time {
	return &t
}
