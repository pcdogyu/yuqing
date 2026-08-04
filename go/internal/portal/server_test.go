package portal

import (
	"bytes"
	"context"
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
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/astocknews"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/nlp"
	"github.com/pcdogyu/yuqing/go/internal/provider"
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

func TestRouterHealthy(t *testing.T) {
	srv := NewServer(config.Config{})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthy", nil)

	srv.Router().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var envelope struct {
		Data struct {
			Service string `json:"service"`
			Status  string `json:"status"`
			Healthy bool   `json:"healthy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal healthy response: %v", err)
	}
	if envelope.Data.Service != "gateway-web" || envelope.Data.Status != "ok" || !envelope.Data.Healthy {
		t.Fatalf("unexpected healthy payload: %+v", envelope.Data)
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
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
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
	if !strings.Contains(body, "/hotspots") || !strings.Contains(body, "/crypto") {
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
	backtestIndex := strings.Index(body, `href="/a-stock/backtest"`)
	sectorIndex := strings.Index(body, `href="/sector-fund-flow"`)
	auctionIndex := strings.Index(body, `href="/a-stock/auction"`)
	researchIndex := strings.Index(body, `href="/stock-research"`)
	cryptoIndex := strings.Index(body, `href="/crypto"`)
	if aStockIndex < 0 || backtestIndex < 0 || sectorIndex < 0 || auctionIndex < 0 || researchIndex < 0 || cryptoIndex < 0 || aStockIndex > backtestIndex || backtestIndex > sectorIndex || sectorIndex > researchIndex || researchIndex > auctionIndex || auctionIndex > cryptoIndex {
		t.Fatalf("expected A股, 回测, 版块资金, 研报调研, 集合竞价 nav links before Crypto, got %s", body)
	}
	if strings.Contains(body, `href="/investor-relations"`) {
		t.Fatalf("expected investor relations to be removed from shared nav, got %s", body)
	}
	for _, want := range []string{
		`class="astock-overview-header"`,
		`id="astock-page-content"`,
		`class="astock-overview-summary"`,
		`class="astock-overview-table"`,
		`body[data-page='a-stock'] table{width:100%;min-width:100%;font-size:12px}`,
		`.astock-overview-header{display:flex;align-items:flex-start;justify-content:space-between;gap:24px;margin-bottom:14px}`,
		`.astock-overview-summary{display:flex;justify-content:flex-start;gap:24px;flex-wrap:wrap;text-align:left;font-size:11px}`,
		`.astock-overview-summary strong{display:block;font-size:17px;line-height:1.25;white-space:nowrap}`,
		`.astock-overview-table{width:100%;min-width:1920px;table-layout:fixed;font-size:11px}`,
		`.astock-overview-table .astock-muted{display:block;margin-bottom:7px;font-size:11px;white-space:nowrap;word-break:keep-all}`,
		`.astock-overview-period{width:6.4%;min-width:110px}`,
		`.astock-overview-period strong{white-space:nowrap}`,
		`.astock-overview-metric{width:6.1%;min-width:82px}`,
		`.astock-overview-metric .astock-muted,.astock-overview-metric strong{white-space:nowrap}`,
		`.astock-overview-recent-filter{width:7.2%;min-width:128px}`,
		`.astock-overview-recent-filter .astock-muted,.astock-overview-recent-filter strong{white-space:nowrap}`,
		`.astock-overview-limit-filter{width:6.6%;min-width:116px}`,
		`.astock-overview-market-filter{width:7.8%;min-width:138px}`,
		`.astock-overview-fund-filter{width:7.8%;min-width:138px}`,
		`.astock-overview-recalculate{width:8.2%;min-width:146px}`,
		`.astock-overview-recalculate .astock-muted,.astock-overview-recalculate strong{white-space:nowrap}`,
		`.astock-overview-window{width:11.2%;min-width:196px}`,
		`.astock-overview-table strong{display:block;font-size:17px;line-height:1.25}`,
		`.astock-overview-status{width:25.4%;min-width:418px}`,
		`.astock-overview-table .astock-filter-toggle{width:100%;min-height:28px;white-space:nowrap}`,
		`body[data-page='a-stock'] main{max-width:none;width:100%;box-sizing:border-box}`,
		`.astock-scroll{width:100%;overflow:auto}`,
		`.astock-news-grid{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:16px;align-items:start}`,
		`.astock-help:hover .astock-help-text,.astock-help:focus .astock-help-text{display:block}`,
		`.astock-hotspot-stocks{display:grid;grid-template-columns:repeat(3,minmax(0,1fr))`,
		`.astock-hotspot-stock{display:block;min-width:0;white-space:nowrap}`,
		`.astock-hotspot-date{color:#7a7064;font-size:12px}`,
		`class="astock-overview-period"`,
		`class="astock-overview-metric"`,
		`class="astock-overview-recent-filter"`,
		`class="astock-overview-limit-filter"`,
		`class="astock-overview-market-filter"`,
		"08:00-09:30",
		"09:30-13:00",
		"上午推荐",
		"下午推荐",
		"推荐生成窗口",
		"新闻统计窗口",
		"08:00-09:26:59",
		"09:30-13:00:59",
		"热点归纳",
		"推荐股票",
		"集合竞价",
		"研报调研",
		"昨日收盘价",
		"昨日涨跌幅",
		"30天涨跌幅",
		"60天涨跌幅",
		"现价",
		"今日涨跌幅",
		"5日资金动向",
		"资金过滤",
		"启用资金过滤",
		".astock-table th{white-space:nowrap}",
		".astock-recommendation-table{width:100%;min-width:1460px;table-layout:fixed}",
		".astock-recommendation-table th:nth-child(2),.astock-recommendation-table td:nth-child(2){width:7.5%;white-space:nowrap}",
		".astock-recommendation-table th:last-child,.astock-recommendation-table td:last-child{width:47%}",
		".astock-recommendation-detail-cell{white-space:normal}",
		".astock-recommendation-fundflow{margin-bottom:6px;font-weight:700;line-height:1.25;white-space:nowrap}",
		".astock-score-table th:nth-child(3),.astock-score-table td:nth-child(3){width:42%}",
		"推荐窗口",
		`colspan="12"`,
		"抓取全部财经信息",
		"重新生成上午推荐",
		"重新生成下午推荐",
		`class="astock-action-form"`,
		`class="astock-action-grid"`,
		"astock-action-running",
		`aria-busy`,
		`button.disabled=true`,
		`u.searchParams.set("partial","1")`,
		`fetch(partialURL(raw)`,
		`fallback(raw)`,
		`form[method='get']`,
		`history.pushState`,
		`window.addEventListener("popstate"`,
		"金十全站信息",
		"jin10_full",
		"东方财富快讯",
		"eastmoney_kuaixun",
		"东方财富全站",
		"eastmoney_full",
		"华尔街见闻",
		"wallstreetcn_a_stock",
		"财联社",
		"cls_telegraph",
		"新浪财经",
		"sina_finance_7x24",
		"暂无数据",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 page to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"机构共持</th>", "持仓占比</th>", "消息回测", "推荐历史", "T+0 收益", "T+1 收益", "T+2 收盘价", "T+3 收盘价", "T+4 收盘价", "T+5 收盘价", "astock-overview-meta", "财经新闻来源统计", "08:00-09:30 财经新闻", "09:30-13:00 财经新闻"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected A股 page not to contain moved backtest content %q, got %s", notWant, body)
		}
	}
	for _, notWant := range []string{"A股策略工作台", "回到今天", "每日 09:30", "12:50 自动抓取"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected removed A股 hero content %q to be absent, got %s", notWant, body)
		}
	}
	if strings.Contains(body, `body[data-page='a-stock'] header`) {
		t.Fatalf("expected A股 page to keep shared header width, got %s", body)
	}
	if strings.Contains(body, `<th>说明</th>`) {
		t.Fatalf("expected explanation column to move into tooltip, got %s", body)
	}
}

func TestAStockPagePartialReturnsFragmentJSON(t *testing.T) {
	srv := NewServer(config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-15&period=morning&partial=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected JSON content type, got %q body=%s", ct, rr.Body.String())
	}
	var payload aStockPartialPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode partial payload: %v body=%s", err, rr.Body.String())
	}
	if payload.Date != "2026-06-15" || payload.Period != "morning" {
		t.Fatalf("expected normalized date/period, got %+v", payload)
	}
	if payload.CanonicalURL != "/a-stock?date=2026-06-15&period=morning" {
		t.Fatalf("expected canonical URL, got %q", payload.CanonicalURL)
	}
	for _, want := range []string{`id="astock-page-content"`, `data-astock-date="2026-06-15"`, "上午推荐"} {
		if !strings.Contains(payload.HTML, want) {
			t.Fatalf("expected partial HTML to contain %q, got %s", want, payload.HTML)
		}
	}
	for _, notWant := range []string{"<style>", "<script>", "<title>", "body data-page='a-stock'"} {
		if strings.Contains(payload.HTML, notWant) {
			t.Fatalf("expected partial HTML not to contain full-page marker %q, got %s", notWant, payload.HTML)
		}
	}
}

func TestAStockPageFragmentCacheKeysAndBypass(t *testing.T) {
	srv := NewServer(config.Config{})
	key := aStockPageFragmentCacheKey("2026-06-15", "morning", 1, false, false, true, false)
	srv.storeCachedAStockPageFragment(key, aStockPartialPayload{
		HTML:         `<div id="astock-page-content">cached morning</div>`,
		CanonicalURL: "/a-stock?date=2026-06-15&period=morning",
		Date:         "2026-06-15",
		Period:       "morning",
	})

	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-15&period=morning&partial=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})
	var cached aStockPartialPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &cached); err != nil {
		t.Fatalf("decode cached partial: %v", err)
	}
	if !strings.Contains(cached.HTML, "cached morning") {
		t.Fatalf("expected fragment cache hit, got %+v", cached)
	}

	req = httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-15&period=afternoon&partial=1", nil)
	rr = httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})
	var afternoon aStockPartialPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &afternoon); err != nil {
		t.Fatalf("decode afternoon partial: %v", err)
	}
	if afternoon.Period != "afternoon" || strings.Contains(afternoon.HTML, "cached morning") {
		t.Fatalf("expected period-specific cache key, got %+v", afternoon)
	}

	for _, rawURL := range []string{
		"/a-stock?date=2026-06-15&period=morning&partial=1&msg=fresh",
		"/a-stock?date=2026-06-15&period=morning&partial=1&refresh_recommendations=1",
		"/a-stock?date=2026-06-15&period=morning&partial=1&refresh_all_backtests=1",
	} {
		req = httptest.NewRequest(http.MethodGet, rawURL, nil)
		rr = httptest.NewRecorder()
		srv.handleAStockPage(rr, req, map[string]any{"id": 1})
		var payload aStockPartialPayload
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode bypass partial %s: %v", rawURL, err)
		}
		if strings.Contains(payload.HTML, "cached morning") {
			t.Fatalf("expected %s to bypass fragment cache, got %+v", rawURL, payload)
		}
	}

	baseKey := aStockPageFragmentCacheKey("2026-06-15", "morning", 1, false, false, true, false)
	for name, otherKey := range map[string]string{
		"date":                aStockPageFragmentCacheKey("2026-06-16", "morning", 1, false, false, true, false),
		"period":              aStockPageFragmentCacheKey("2026-06-15", "afternoon", 1, false, false, true, false),
		"news_page":           aStockPageFragmentCacheKey("2026-06-15", "morning", 2, false, false, true, false),
		"ignore_recent":       aStockPageFragmentCacheKey("2026-06-15", "morning", 1, true, false, true, false),
		"ignore_limit_up":     aStockPageFragmentCacheKey("2026-06-15", "morning", 1, false, true, true, false),
		"filter_fund_flow":    aStockPageFragmentCacheKey("2026-06-15", "morning", 1, false, false, false, false),
		"filter_today_market": aStockPageFragmentCacheKey("2026-06-15", "morning", 1, false, false, true, true),
	} {
		if otherKey == baseKey {
			t.Fatalf("expected %s to be part of fragment cache key %q", name, baseKey)
		}
	}
}

func TestAStockPagePostClearsSameDateCaches(t *testing.T) {
	srv := NewServer(config.Config{})
	fragmentKey := aStockPageFragmentCacheKey("2026-06-15", "morning", 1, false, false, false, false)
	articleStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	articleEnd := articleStart.Add(time.Hour)
	articleKey := aStockArticlePagesCacheKey("publish_time", articleStart, articleEnd)
	srv.storeCachedAStockPageFragment(fragmentKey, aStockPartialPayload{HTML: `<div>cached</div>`, Date: "2026-06-15", Period: "morning"})
	srv.storeCachedAStockArticlePages(articleKey, "2026-06-15", []model.Item{{ID: 1, Title: "cached"}})
	srv.aStockCacheMu.Lock()
	srv.aStockAuctions[aStockAuctionCandidateCacheKey("2026-06-15")] = aStockServerAuctionCacheEntry{expiresAt: time.Now().Add(time.Hour)}
	srv.aStockAuctions["__latest__"] = aStockServerAuctionCacheEntry{expiresAt: time.Now().Add(time.Hour)}
	srv.aStockCacheMu.Unlock()

	form := url.Values{"date": {"2026-06-15"}, "period": {"morning"}, "action": {"unknown"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if _, ok := srv.loadCachedAStockPageFragment(fragmentKey); ok {
		t.Fatal("expected same-date fragment cache to be cleared")
	}
	if _, _, ok := srv.loadCachedAStockArticlePages(articleKey); ok {
		t.Fatal("expected same-date article cache to be cleared")
	}
	srv.aStockCacheMu.Lock()
	_, dateAuctionOK := srv.aStockAuctions[aStockAuctionCandidateCacheKey("2026-06-15")]
	_, latestAuctionOK := srv.aStockAuctions["__latest__"]
	srv.aStockCacheMu.Unlock()
	if dateAuctionOK || latestAuctionOK {
		t.Fatalf("expected auction caches to be cleared, date=%t latest=%t", dateAuctionOK, latestAuctionOK)
	}
}

func TestAStockAuctionPageLoadsSummaryAndRows(t *testing.T) {
	fetchedAt := time.Date(2026, 6, 16, 1, 30, 0, 0, time.UTC)
	trend0929 := []model.AStockAuctionTrend{
		{Date: "2026-06-15", CaptureSlot: "0929", StockCount: 2, TotalVolume: 180000, TotalAmount: 4500000, MaxStockCode: "600000", MaxStockName: "浦发银行"},
		{
			Date:         "2026-06-16",
			CaptureSlot:  "0929",
			StockCount:   2,
			TotalVolume:  213400,
			TotalAmount:  5876080,
			MaxStockCode: "002230",
			MaxStockName: "科大讯飞",
			MarketTop: []model.AStockAuctionMarketTop{
				{
					Market: "沪市",
					Items: []model.AStockAuctionAmount{{
						Code:          "600000",
						Name:          "浦发银行",
						AuctionAmount: 792000,
					}},
				},
				{
					Market: "深市",
					Items: []model.AStockAuctionAmount{{
						Code:          "002230",
						Name:          "科大讯飞",
						AuctionAmount: 5084080,
					}},
				},
			},
		},
	}
	trend0925 := []model.AStockAuctionTrend{
		{Date: "2026-06-15", CaptureSlot: "0925", StockCount: 2, TotalVolume: 170000, TotalAmount: 4000000, MaxStockCode: "600000", MaxStockName: "浦发银行"},
		{Date: "2026-06-16", CaptureSlot: "0925", StockCount: 2, TotalVolume: 200000, TotalAmount: 5000000, MaxStockCode: "002230", MaxStockName: "科大讯飞"},
	}
	trend0920 := []model.AStockAuctionTrend{
		{Date: "2026-06-15", CaptureSlot: "0920", StockCount: 2, TotalVolume: 160000, TotalAmount: 3900000, MaxStockCode: "600000", MaxStockName: "浦发银行"},
		{Date: "2026-06-16", CaptureSlot: "0920", StockCount: 2, TotalVolume: 190000, TotalAmount: 4800000, MaxStockCode: "002230", MaxStockName: "科大讯飞"},
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/auction" {
			t.Fatalf("unexpected auction content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("date") != "2026-06-16" || r.URL.Query().Get("keyword") != "科" {
			t.Fatalf("unexpected auction query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("page_size") != "6000" {
			t.Fatalf("expected auction page to request full-market page_size=6000, got %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("trend_days") != "14" {
			t.Fatalf("expected auction page to request default 14-day trend, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.AStockAuctionListResult{
				Date:         "2026-06-16",
				CaptureSlot:  "0929",
				Keyword:      "科",
				LatestDate:   "2026-06-16",
				Dates:        []string{"2026-06-16", "2026-06-15"},
				SummaryCount: 2,
				Total:        1,
				Page:         1,
				PageSize:     6000,
				TotalAmount:  151000000,
				MaxItem: &model.AStockAuctionAmount{
					TradeDate:     "2026-06-16",
					CaptureSlot:   "0929",
					Code:          "002230",
					Name:          "科大讯飞",
					AuctionAmount: 100000000,
					FetchedAt:     fetchedAt,
				},
				FetchedAt: &fetchedAt,
				Items: []model.AStockAuctionAmount{{
					TradeDate:     "2026-06-16",
					CaptureSlot:   "0929",
					Code:          "002230",
					Name:          "科大讯飞",
					AuctionPrice:  41.2,
					AuctionVolume: 123400,
					AuctionAmount: 5084080,
					Source:        "akshare_pre_min",
					Status:        "ok",
					FetchedAt:     fetchedAt,
				}},
				Trend:       trend0929,
				TrendSeries: map[string][]model.AStockAuctionTrend{"0920": trend0920, "0925": trend0925, "0929": trend0929},
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
	for _, want := range []string{"集合竞价", "09:20:00", "09:25:00", "09:29:59", "操作区", "获取最新交易日集合竞价金额", "回溯近7天集合竞价", "当日汇总", "当前快照", "0929", "近2周资金趋势", "最近7天", "最近2周", "最近30天", "沪市金额前三", "深市金额前三", "2026-06-16", "科大讯飞", "浦发银行", "股票数", "2", "1.51亿", "508.41万", "79.20万", "12.34万", "akshare_pre_min", `value="科"`, `<svg class="auction-chart"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected auction page to contain %q, got %s", want, body)
		}
	}
	for _, want := range []string{
		`.auction-chart-grid-x{stroke:#d6ccbb;stroke-width:1}`,
		`.auction-chart{width:100%;height:auto;min-height:286px}`,
		`viewBox="0 0 1120 308"`,
		`class="auction-chart-grid-x" x1="56.0"`,
		`class="auction-chart-grid-x" x1="1048.0"`,
		`<title>2026-06-15</title>`,
		`<title>2026-06-16</title>`,
		`class="auction-chart-label" x="56.0" text-anchor="start" y="296.0">2026-06-15</text>`,
		`class="auction-chart-label" x="1048.0" text-anchor="end" y="296.0">2026-06-16</text>`,
		`class="auction-chart-label" text-anchor="end" x="48.0" y="28.0">587.61万</text>`,
		`class="auction-chart-label" text-anchor="start" x="1056.0" y="28.0">587.61万</text>`,
		`class="auction-chart-label" text-anchor="end" x="48.0" y="260.0">0</text>`,
		`class="auction-chart-label" text-anchor="start" x="1056.0" y="260.0">0</text>`,
		`class="auction-chart-line auction-chart-line-0920"`,
		`class="auction-chart-line auction-chart-line-0925"`,
		`class="auction-chart-line auction-chart-line-0929"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected auction chart daily grid to contain %q, got %s", want, body)
		}
	}
	if got := strings.Count(body, `class="auction-chart-grid-x"`); got != 2 {
		t.Fatalf("expected one x-axis grid line per trend day, got %d in %s", got, body)
	}
	for _, notWant := range []string{"近7日资金趋势", "近30日资金趋势", "北交所金额前三", "每个市场集合竞价金额最高的3只股票"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected auction page not to contain %q, got %s", notWant, body)
		}
	}
}

func TestAStockAuctionPageKeepsExplicitSevenDayTrend(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/auction" {
			t.Fatalf("unexpected auction content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("trend_days") != "7" {
			t.Fatalf("expected auction page to pass explicit 7-day trend, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.AStockAuctionListResult{
				Date:       "2026-06-15",
				LatestDate: "2026-06-15",
				Page:       1,
				PageSize:   6000,
				Trend: []model.AStockAuctionTrend{{
					Date:        "2026-06-15",
					TotalAmount: 1000000,
				}},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/auction?trend_days=7", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockAuctionPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected auction page 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, "近7日资金趋势") || !strings.Contains(body, `href="/a-stock/auction?date=2026-06-15&amp;trend_days=7">最近7天`) {
		t.Fatalf("expected explicit 7-day auction trend page, got %s", body)
	}
}

func TestAStockAuctionPageSwitchesTrendPeriod(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/auction" {
			t.Fatalf("unexpected auction content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("trend_days") != "14" {
			t.Fatalf("expected auction page to pass selected 14-day trend, got %s", r.URL.RawQuery)
		}
		trend := make([]model.AStockAuctionTrend, 0, 15)
		for day := 1; day <= 15; day++ {
			trend = append(trend, model.AStockAuctionTrend{
				Date:         fmt.Sprintf("2026-06-%02d", day),
				StockCount:   day,
				TotalVolume:  float64(day * 10000),
				TotalAmount:  float64(day * 1000000),
				MaxStockCode: "600000",
				MaxStockName: "浦发银行",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.AStockAuctionListResult{
				Date:       "2026-06-15",
				LatestDate: "2026-06-15",
				Page:       1,
				PageSize:   6000,
				Trend:      trend,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/auction?trend_days=14", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockAuctionPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected auction page 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"近2周资金趋势", `href="/a-stock/auction?date=2026-06-15&amp;trend_days=14">最近2周`, "2026-06-02", "2026-06-15"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected 14-day auction trend page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "2026-06-01") {
		t.Fatalf("expected 14-day trend window to exclude oldest 15th point, got %s", body)
	}
}

func TestAStockAuctionTrendTablePointsPrefers0929Series(t *testing.T) {
	points := aStockAuctionTrendTablePoints(model.AStockAuctionListResult{
		Trend: []model.AStockAuctionTrend{{
			Date:        "2026-07-14",
			CaptureSlot: "0925",
			TotalAmount: 925,
		}},
		TrendSeries: map[string][]model.AStockAuctionTrend{
			"0925": {{
				Date:        "2026-07-14",
				CaptureSlot: "0925",
				TotalAmount: 925,
			}},
			"0929": {
				{
					Date:        "2026-07-14",
					CaptureSlot: "0929",
					TotalAmount: 929,
				},
				{
					Date:        "2026-07-15",
					CaptureSlot: "0929",
					TotalAmount: 92915,
				},
			},
			"0930": {
				{
					Date:        "2026-07-13",
					CaptureSlot: "0930",
					TotalAmount: 93013,
				},
				{
					Date:        "2026-07-14",
					CaptureSlot: "0930",
					TotalAmount: 93014,
				},
			},
		},
	}, 7)

	if len(points) != 3 {
		t.Fatalf("expected history table to merge legacy 0930 and current 0929 series, got %+v", points)
	}
	if points[0].Date != "2026-07-13" || points[0].CaptureSlot != "0930" || points[0].TotalAmount != 93013 {
		t.Fatalf("expected first merged point to come from legacy 0930, got %+v", points[0])
	}
	if points[1].Date != "2026-07-14" || points[1].CaptureSlot != "0929" || points[1].TotalAmount != 929 {
		t.Fatalf("expected same-day 0929 point to override legacy 0930, got %+v", points[1])
	}
	if points[2].Date != "2026-07-15" || points[2].CaptureSlot != "0929" || points[2].TotalAmount != 92915 {
		t.Fatalf("expected latest merged point to come from current 0929, got %+v", points[2])
	}
}

func TestAStockAuctionTrendTablePointsFallsBackToLegacy0930Series(t *testing.T) {
	points := aStockAuctionTrendTablePoints(model.AStockAuctionListResult{
		TrendSeries: map[string][]model.AStockAuctionTrend{
			"0930": {{
				Date:        "2026-07-14",
				CaptureSlot: "0930",
				TotalAmount: 930,
			}},
		},
	}, 7)

	if len(points) != 1 || points[0].CaptureSlot != "0930" || points[0].TotalAmount != 930 {
		t.Fatalf("expected history table to fall back to legacy 0930 series, got %+v", points)
	}
}

func TestAStockAuctionPagePostTriggersSchedulerJob(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/auction/latest" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"status":"triggered","result":{"date":"2026-06-16","total":2,"ok":1}}}`))
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
	decoded, _ := url.QueryUnescape(loc)
	if !strings.Contains(decoded, "/a-stock/auction?") || strings.Contains(decoded, "date=") || !strings.Contains(decoded, "最新交易日集合竞价已写入：2026-06-16") {
		t.Fatalf("expected redirect to auction page with latest message and no forced date, got %q", decoded)
	}
}

func TestAStockAuctionPagePostTriggersBackfill7Days(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/auction/backfill" || r.URL.Query().Get("days") != "7" {
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
	req := httptest.NewRequest(http.MethodPost, "/a-stock/auction", strings.NewReader("action=backfill_7d_auction"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockAuctionPage(rr, req, map[string]any{"id": 1})

	if !schedulerCalled {
		t.Fatal("expected scheduler backfill to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after auction backfill, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	if !strings.Contains(loc, "近7天集合竞价回溯任务已触发") {
		t.Fatalf("expected backfill success message, got %q", loc)
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
	var requests int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected stock research content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("company") != "科大" || r.URL.Query().Get("institution") != "中金" || r.URL.Query().Get("source") != "" {
			t.Fatalf("unexpected stock research query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.StockResearchListResult{
				Page:        1,
				PageSize:    50,
				Total:       3,
				Company:     "科大",
				Institution: "中金",
				Sources:     []string{"sina_finance_report", "eastmoney_report", "sohu_finance_report", investorRelationsSourceType},
				Items: []model.StockResearchSurvey{
					{
						ID:           7,
						Code:         "002230",
						Name:         "科大讯飞",
						Kind:         "report",
						Title:        "科大讯飞深度研究",
						Institution:  "中金公司",
						Analyst:      "张三",
						Rating:       "买入",
						TargetPrice:  "50.123456",
						ResearchDate: "2026-06-16",
						SourceType:   "sina_finance_report",
						SourceURL:    "https://sina.example.com/1",
						PDFFilePath:  "data/stock-research-pdfs/sina/sina-1.pdf",
						PDFStatus:    "parsed",
						PDFText:      "科大讯飞研报正文",
					},
					{
						ID:           8,
						Code:         "000001",
						Name:         "平安银行",
						Kind:         "report",
						Title:        "银行业务点评",
						Institution:  "中金公司",
						Analyst:      "李四",
						ResearchDate: "2026-06-15",
						SourceType:   "sina_finance_report",
						SourceURL:    "https://sina.example.com/2",
						PDFStatus:    "",
					},
					{
						ID:           9,
						Code:         "300123",
						Name:         "测试公司",
						Kind:         "survey",
						Title:        "投资者关系管理信息20260617",
						ResearchDate: "2026-06-14",
						SourceType:   investorRelationsSourceType,
					},
				},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/stock-research?company=科大&institution=中金", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stock research page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if requests != 1 {
		t.Fatalf("expected one merged stock research request, got %d", requests)
	}
	body := rr.Body.String()
	for _, want := range []string{"研报调研", "深度研究", "中金公司", "张三", "新浪财经", "东方财富", "搜狐财经", "回补近一年", "解析当前筛选研报PDF", "抓取投资者关系近一年并解析PDF", "补抓当前页原文入库", "下载PDF", "查看文本", "已解析", "无PDF", "重新解析", "抓取原文", `value="科大"`, `href="/stock-research/7?return_to=`, `href="/a-stock">A股</a><a href="/a-stock/backtest">回测</a><a href="/sector-fund-flow">版块资金</a><a href="/stock-research">研报调研</a><a href="/a-stock/holdings">机构持仓</a>`, "body[data-page='stock-research'] main,body[data-page='stock-research'] .site-footer{max-width:none;width:100%;box-sizing:border-box}", "body[data-page='stock-research'] main{font-size:14px;line-height:1.45}", "body[data-page='stock-research'] section{width:100%;box-sizing:border-box}", ".research-scroll{width:100%;overflow:auto}", ".research-table{width:100%;min-width:0;table-layout:fixed}", ".research-col-title{width:22.8%}", ".research-col-institution{width:12.8%}", ".research-col-analyst{width:10.8%}", ".research-col-pdf{width:8.4%}", ".research-col-status{width:18.2%}", ".research-col-date{width:7.5%}", ".research-col-stock{width:8.5%}", ".research-col-source{width:6%}", ".research-col-link{width:5%}", ".research-status-actions{display:flex;align-items:center;gap:8px;white-space:nowrap;flex-wrap:wrap}", ".research-status-actions .research-action-form{flex:1 1 auto;min-width:0}", ".research-status-actions button{width:90%;height:90%;min-height:32px;margin:0;padding:7px 10px}", "<th>日期</th><th>股票</th><th>标题</th>", `<option value="cninfo_investor_relation">互动易投资者关系</option>`, "互动易投资者关系", "投资者关系管理信息20260617"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected stock research page to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{`href="/investor-relations"`, "投资者关系列表", "投资者关系筛选", `name="ir_code"`, "ir_page=", `id="investor-relations"`, "#investor-relations"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected stock research page not to contain %q, got %s", notWant, body)
		}
	}
	if strings.Contains(body, "body[data-page='stock-research'] header,body[data-page='stock-research'] main") {
		t.Fatalf("expected stock research header to keep the shared homepage alignment, got %s", body)
	}
	if strings.Index(body, `class="research-status-actions"`) < 0 || strings.Index(body, `class="research-status-actions"`) > strings.Index(body, `重新解析`) {
		t.Fatalf("expected status and reparse button to render in one action row, got %s", body)
	}
	for _, notWant := range []string{"科大讯飞深度研究", "买入", "50.12", "50.123456", "<th>评级</th>", "<th>目标价</th>", "<th>日期</th><th>股票</th><th>类型</th>"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected stock research page not to contain %q, got %s", notWant, body)
		}
	}
}

func TestSectorFundFlowPageLoadsFiltersRowsAndRefreshAction(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchedAt := time.Date(2026, 7, 1, 2, 35, 0, 0, time.UTC)
		switch r.URL.Path {
		case "/api/v1/a-stock/sector-fund-flows":
			if r.URL.Query().Get("sector_type") != "概念资金流" || r.URL.Query().Get("indicator") != "5日" || r.URL.Query().Get("keyword") != "AI" {
				t.Fatalf("unexpected sector fund flow query: %s", r.URL.RawQuery)
			}
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorFundFlowListResult{
					Items: []model.AStockSectorFundFlow{{
						TradeDate:              "2026-07-01",
						SectorType:             "概念资金流",
						Indicator:              "5日",
						Rank:                   1,
						Name:                   "人工智能",
						ChangePct:              2.34,
						MainNetInflow:          230000000,
						MainNetInflowPct:       5.6,
						SuperLargeNetInflow:    120000000,
						SuperLargeNetInflowPct: 3.4,
						LargeNetInflow:         110000000,
						LargeNetInflowPct:      2.2,
						MediumNetInflow:        -30000000,
						SmallNetInflow:         -90000000,
						TopStock:               "中科曙光；新易盛；寒武纪；第四名",
						FetchedAt:              fetchedAt,
					}},
					Page:        1,
					PageSize:    200,
					Total:       1,
					Date:        "2026-07-01",
					LatestDate:  "2026-07-01",
					SectorType:  "概念资金流",
					Indicator:   "5日",
					Keyword:     "AI",
					Dates:       []string{"2026-07-01"},
					SectorTypes: []string{"行业资金流", "概念资金流"},
					Indicators:  []string{"今日", "5日", "10日"},
					FetchedAt:   &fetchedAt,
				},
			})
		case "/api/v1/a-stock/stock-fund-flows":
			if r.URL.Query().Get("keyword") == "AI" {
				writeRawJSON(w, http.StatusOK, map[string]any{
					"code":    http.StatusOK,
					"message": "ok",
					"data": model.AStockStockFundFlowListResult{
						Items:     nil,
						Page:      1,
						PageSize:  50,
						Total:     0,
						Date:      "2026-07-01",
						Indicator: "5日",
						FetchedAt: &fetchedAt,
					},
				})
				return
			}
			if r.URL.Query().Get("indicator") != "5日" || r.URL.Query().Get("codes") != "300502,603019" {
				t.Fatalf("unexpected stock fund flow query: %s", r.URL.RawQuery)
			}
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockStockFundFlowListResult{
					Items: []model.AStockStockFundFlow{{
						TradeDate:           "2026-07-01",
						Indicator:           "5日",
						Rank:                8,
						Code:                "300502",
						Name:                "新易盛",
						Price:               529.04,
						ChangePct:           3.94,
						MainNetInflow:       810968272,
						MainNetInflowPct:    8.95,
						SuperLargeNetInflow: 600000000,
						LargeNetInflow:      210968272,
						MediumNetInflow:     -100000000,
						SmallNetInflow:      -710968272,
						FetchedAt:           fetchedAt,
					}},
					Page:      1,
					PageSize:  500,
					Total:     1,
					Date:      "2026-07-01",
					Indicator: "5日",
					FetchedAt: &fetchedAt,
				},
			})
		case "/api/v1/a-stock/sector-constituents":
			if r.URL.Query().Get("sector_type") != "概念资金流" || r.URL.Query().Get("sector_name") != "人工智能" {
				t.Fatalf("unexpected cached constituents query: %s", r.URL.RawQuery)
			}
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorConstituentListResult{
					Items: []model.AStockSectorConstituent{
						{SectorType: "概念资金流", SectorName: "人工智能", Code: "300502", Name: "新易盛", Source: "cached", FetchedAt: fetchedAt},
						{SectorType: "概念资金流", SectorName: "人工智能", Code: "603019", Name: "中科曙光", Source: "cached", FetchedAt: fetchedAt},
					},
					Total:      2,
					SectorType: "概念资金流",
					SectorName: "人工智能",
					FetchedAt:  &fetchedAt,
				},
			})
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("akshare should not be called when cached constituents exist: %s", r.URL.String())
	}))
	defer akshare.Close()
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/sector-fund-flow/latest" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.Path)
		}
		writeRawJSON(w, http.StatusOK, map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": map[string]any{
				"status": "completed",
				"result": map[string]any{"date": "2026-07-01", "groups": 6, "items": 6},
			},
		})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL, AStockAuctionURL: akshare.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodGet, "/sector-fund-flow?sector_type=概念资金流&indicator=5日&keyword=AI&sector_name=人工智能", nil)
	rr := httptest.NewRecorder()
	srv.handleSectorFundFlowPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected sector fund flow page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"版块资金", "人工智能", "中科曙光、新易盛、寒武纪", "刷新版块资金", "行业资金流", "概念资金流", "今日", "5日", "10日", "+2.30亿", "+5.60%", "人工智能 个股资金流", "成分股 2 只，本地资金流命中 1 只", "300502", "新易盛", "+8.11亿", "sector_name", "趋势", "trend=sector", `href="/a-stock">A股</a><a href="/a-stock/backtest">回测</a><a href="/sector-fund-flow">版块资金</a><a href="/stock-research">研报调研</a>`, `body[data-page='sector-fund-flow'] main,body[data-page='sector-fund-flow'] .site-footer{max-width:none;width:100%;box-sizing:border-box}`, `.sector-table th:nth-child(1),.sector-table td:nth-child(1){width:54px;text-align:center}`, `.sector-name-link`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected sector fund flow page to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"<th>中单</th>", "<th>小单</th>", "第四名"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected sector fund flow page not to contain %q, got %s", notWant, body)
		}
	}

	form := url.Values{}
	form.Set("action", "refresh_sector_fund_flow")
	form.Set("sector_type", "概念资金流")
	form.Set("indicator", "5日")
	form.Set("keyword", "AI")
	postReq := httptest.NewRequest(http.MethodPost, "/sector-fund-flow", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	srv.handleSectorFundFlowPage(postRR, postReq, map[string]any{"id": 1})
	if postRR.Code != http.StatusSeeOther || !strings.Contains(postRR.Header().Get("Location"), "/sector-fund-flow?") || !strings.Contains(postRR.Header().Get("Location"), "sector_type=") {
		t.Fatalf("expected sector fund flow refresh redirect, status=%d location=%s", postRR.Code, postRR.Header().Get("Location"))
	}
}

func TestSectorFundFlowPageRendersTrendAndStockSearch(t *testing.T) {
	fetchedAt := time.Date(2026, 7, 6, 7, 0, 0, 0, time.UTC)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/sector-fund-flows":
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorFundFlowListResult{
					Items: []model.AStockSectorFundFlow{
						{TradeDate: "2026-07-06", SectorType: "行业资金流", Indicator: "今日", Rank: 47, Name: "石油石化", MainNetInflow: 447406048, TopStock: "中国石化", FetchedAt: fetchedAt},
						{TradeDate: "2026-07-06", SectorType: "行业资金流", Indicator: "今日", Rank: 62, Name: "石油加工贸易", MainNetInflow: 299000000, TopStock: "东方盛虹", FetchedAt: fetchedAt},
						{TradeDate: "2026-07-06", SectorType: "行业资金流", Indicator: "今日", Rank: 78, Name: "石油行业", MainNetInflow: 201844911, TopStock: "ST洲际", FetchedAt: fetchedAt},
					},
					Page:        1,
					PageSize:    200,
					Total:       3,
					Date:        "2026-07-06",
					LatestDate:  "2026-07-06",
					SectorType:  "行业资金流",
					Indicator:   "今日",
					Keyword:     "600028",
					Dates:       []string{"2026-07-06"},
					SectorTypes: []string{"行业资金流", "概念资金流"},
					Indicators:  []string{"今日", "5日", "10日"},
					FetchedAt:   &fetchedAt,
				},
			})
		case "/api/v1/a-stock/stock-fund-flows":
			if r.URL.Query().Get("codes") == "600028" {
				writeRawJSON(w, http.StatusOK, map[string]any{
					"code":    http.StatusOK,
					"message": "ok",
					"data": model.AStockStockFundFlowListResult{
						Items: []model.AStockStockFundFlow{{TradeDate: "2026-07-06", Indicator: "今日", Rank: 95, Code: "600028", Name: "中国石化", Price: 4.83, MainNetInflow: 217735152, FetchedAt: fetchedAt}},
						Page:  1, PageSize: 500, Total: 1, Date: "2026-07-06", Indicator: "今日", FetchedAt: &fetchedAt,
					},
				})
				return
			}
			if r.URL.Query().Get("keyword") != "600028" {
				t.Fatalf("unexpected stock search query: %s", r.URL.RawQuery)
			}
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockStockFundFlowListResult{
					Items: []model.AStockStockFundFlow{{TradeDate: "2026-07-06", Indicator: "今日", Rank: 95, Code: "600028", Name: "中国石化", Price: 4.83, MainNetInflow: 217735152, FetchedAt: fetchedAt}},
					Page:  1, PageSize: 50, Total: 1, Date: "2026-07-06", Indicator: "今日", Keyword: "600028", FetchedAt: &fetchedAt,
				},
			})
		case "/api/v1/a-stock/sector-constituents":
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorConstituentListResult{
					Items: []model.AStockSectorConstituent{{SectorType: "行业资金流", SectorName: "石油石化", Code: "600028", Name: "中国石化", Source: "cached", FetchedAt: fetchedAt}},
					Total: 1, SectorType: "行业资金流", SectorName: "石油石化", FetchedAt: &fetchedAt,
				},
			})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockStockFundFlowTrendResult{
					Items: []model.AStockStockFundFlow{
						{TradeDate: "2026-07-06", Indicator: "今日", Rank: 95, Code: "600028", Name: "中国石化", MainNetInflow: 217735152, FetchedAt: fetchedAt},
						{TradeDate: "2026-07-03", Indicator: "今日", Rank: 90, Code: "600028", Name: "中国石化", MainNetInflow: -10000000, FetchedAt: fetchedAt.Add(-24 * time.Hour)},
					},
					Total: 2, EndDate: "2026-07-06", Indicator: "今日", Code: "600028", Days: 5,
				},
			})
		case "/api/v1/a-stock/sector-fund-flow-trend":
			if r.URL.Query().Get("sector_name") != "石油石化" || r.URL.Query().Get("days") != "5" {
				t.Fatalf("unexpected sector trend query: %s", r.URL.RawQuery)
			}
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorFundFlowTrendResult{
					Items: []model.AStockSectorFundFlow{
						{TradeDate: "2026-07-06", SectorType: "行业资金流", Indicator: "今日", Rank: 47, Name: "石油石化", MainNetInflow: 447406048, FetchedAt: fetchedAt},
						{TradeDate: "2026-07-03", SectorType: "行业资金流", Indicator: "今日", Rank: 42, Name: "石油石化", MainNetInflow: -120000000, FetchedAt: fetchedAt.Add(-24 * time.Hour)},
					},
					Total: 2, EndDate: "2026-07-06", SectorType: "行业资金流", SectorName: "石油石化", Indicator: "今日", Days: 5,
				},
			})
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()
	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/sector-fund-flow?date=2026-07-06&sector_type=行业资金流&indicator=今日&keyword=600028&trend=sector&sector_name=石油石化", nil)
	rr := httptest.NewRecorder()
	srv.handleSectorFundFlowPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected sector trend page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"石油石化 版块主力资金趋势", "当前仅有 2 个交易日数据", "5日", "10日", "30日", "个股资金流搜索", "600028", "中国石化", "trend=stock", "+2.18亿"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected trend/search page to contain %q, got %s", want, body)
		}
	}
}

func TestSectorFundFlowPageConstituentFallbackTimeoutIsFriendly(t *testing.T) {
	fetchedAt := time.Date(2026, 7, 6, 7, 0, 0, 0, time.UTC)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/sector-fund-flows":
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code": http.StatusOK, "message": "ok",
				"data": model.AStockSectorFundFlowListResult{
					Items: []model.AStockSectorFundFlow{{TradeDate: "2026-07-06", SectorType: "行业资金流", Indicator: "今日", Rank: 47, Name: "石油石化", MainNetInflow: 447406048, TopStock: "中国石化", FetchedAt: fetchedAt}},
					Page:  1, PageSize: 200, Total: 1, Date: "2026-07-06", SectorType: "行业资金流", Indicator: "今日", Dates: []string{"2026-07-06"}, FetchedAt: &fetchedAt,
				},
			})
		case "/api/v1/a-stock/sector-constituents":
			writeRawJSON(w, http.StatusOK, map[string]any{
				"code": http.StatusOK, "message": "ok",
				"data": model.AStockSectorConstituentListResult{Items: nil, Total: 0, SectorType: "行业资金流", SectorName: "石油石化"},
			})
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		writeRawJSON(w, http.StatusOK, map[string]any{"items": []map[string]string{{"code": "600028", "name": "中国石化"}}})
	}))
	defer akshare.Close()
	srv := NewServer(config.Config{ContentURL: content.URL, AStockAuctionURL: akshare.URL})
	req := httptest.NewRequest(http.MethodGet, "/sector-fund-flow?date=2026-07-06&sector_type=行业资金流&indicator=今日&sector_name=石油石化", nil)
	rr := httptest.NewRecorder()
	srv.handleSectorFundFlowPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected fallback timeout page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "暂无成分股缓存，实时刷新失败") {
		t.Fatalf("expected friendly constituent timeout message, got %s", body)
	}
	if strings.Contains(body, "context deadline exceeded") || strings.Contains(body, "Client.Timeout") {
		t.Fatalf("expected raw timeout to be hidden, got %s", body)
	}
}

func TestCleanStockResearchTitleRemovesLeadingMarkers(t *testing.T) {
	tests := []struct {
		name string
		item model.StockResearchSurvey
		want string
	}{
		{
			name: "leading zero colon",
			item: model.StockResearchSurvey{Title: "0：预计天气扰动短期节奏 看好其他饮料放量"},
			want: "预计天气扰动短期节奏 看好其他饮料放量",
		},
		{
			name: "parenthesized prefix",
			item: model.StockResearchSurvey{Title: "（公司点评）：经营韧性凸显 底部逐步夯实"},
			want: "经营韧性凸显 底部逐步夯实",
		},
		{
			name: "zero company comment prefix",
			item: model.StockResearchSurvey{Title: "0公司点评：经营韧性凸显 底部逐步夯实"},
			want: "经营韧性凸显 底部逐步夯实",
		},
		{
			name: "zero report comment prefix",
			item: model.StockResearchSurvey{Title: "02026年一季报点评：低猪价使公司业绩承压 养殖成本持续下降"},
			want: "低猪价使公司业绩承压 养殖成本持续下降",
		},
		{
			name: "stock identity removed",
			item: model.StockResearchSurvey{Code: "000885", Name: "城发环境", Title: "000885 城发环境 2025年报点评，归母净利同比增+8.3%"},
			want: "2025年报点评，归母净利同比增+8.3%",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanStockResearchTitle(tc.item); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
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

func TestStockResearchPagePostTriggersPDFParse(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/stock-research/pdf/parse" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.URL.Query().Get("id") != "7" {
			t.Fatalf("expected single item parse id, got query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "triggered", "result": map[string]int{"total": 1, "parsed": 1}}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodPost, "/stock-research", strings.NewReader("action=parse_pdf_one&id=7&code=002230&company=%E7%A7%91%E5%A4%A7%E8%AE%AF%E9%A3%9E"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if !schedulerCalled {
		t.Fatal("expected scheduler pdf parse to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after stock research pdf parse, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"/stock-research?", "code=002230", "company=科大讯飞", "PDF 解析任务已触发"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect to keep filters and message %q, got %q", want, loc)
		}
	}
}

func TestStockResearchPagePostTriggersNLPParse(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/stock-research/nlp/parse" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.URL.Query().Get("id") != "4900" {
			t.Fatalf("expected single item nlp parse id, got query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "triggered", "result": map[string]int{"total": 1, "scored": 1}}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodPost, "/stock-research", strings.NewReader("action=parse_nlp_one&id=4900&code=600026&company=%E4%B8%AD%E8%BF%9C%E6%B5%B7%E8%83%BD"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if !schedulerCalled {
		t.Fatal("expected scheduler nlp parse to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after stock research nlp parse, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"/stock-research?", "code=600026", "company=中远海能", "NLP 解析完成", "评分 1 条"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect to keep filters and message %q, got %q", want, loc)
		}
	}
}

func TestInvestorRelationsPageRedirectsToMergedStockResearchPage(t *testing.T) {
	srv := NewServer(config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/investor-relations?code=300250&company=%E5%88%9D%E7%81%B5&page=2", nil)
	rr := httptest.NewRecorder()
	srv.handleInvestorRelationsPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusFound {
		t.Fatalf("expected investor relations page redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"/stock-research?", "code=300250", "company=初灵", "kind=survey", "source=cninfo_investor_relation", "page=2"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected investor relations redirect to contain %q, got %q", want, loc)
		}
	}
	for _, notWant := range []string{"ir_code=", "ir_company=", "ir_page=", "#investor-relations"} {
		if strings.Contains(loc, notWant) {
			t.Fatalf("expected investor relations redirect not to contain %q, got %q", notWant, loc)
		}
	}
}

func TestInvestorRelationsPagePostTriggersBackfill(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/investor-relations/backfill" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("code") != "300250" || r.URL.Query().Get("start") == "" || r.URL.Query().Get("end") == "" {
			t.Fatalf("unexpected investor relations scheduler query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "triggered"}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodPost, "/investor-relations", strings.NewReader("action=backfill_year&code=300250"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleInvestorRelationsPage(rr, req, map[string]any{"id": 1})
	if !schedulerCalled {
		t.Fatal("expected investor relations scheduler backfill to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after investor relations backfill, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"/stock-research?", "code=300250", "kind=survey", "source=cninfo_investor_relation", "page=1", "抓取和 PDF 解析任务已触发"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect to keep filters and message %q, got %q", want, loc)
		}
	}
	for _, notWant := range []string{"ir_code=", "ir_page=", "#investor-relations"} {
		if strings.Contains(loc, notWant) {
			t.Fatalf("expected redirect not to contain %q, got %q", notWant, loc)
		}
	}
}

func TestStockResearchPageUsesSingleMergedPagination(t *testing.T) {
	var requests int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected stock research content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("page") != "4" || r.URL.Query().Get("source") != investorRelationsSourceType || r.URL.Query().Get("kind") != "survey" || r.URL.Query().Get("code") != "300250" {
			t.Fatalf("unexpected stock research query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{Page: 4, PageSize: 20, Total: 100, Source: investorRelationsSourceType, Kind: "survey", Code: "300250"}})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/stock-research?page=4&source=cninfo_investor_relation&kind=survey&code=300250", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stock research page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if requests != 1 {
		t.Fatalf("expected one merged stock research request, got %d", requests)
	}
	body := rr.Body.String()
	for _, want := range []string{`page=3`, `page=5`, `source=cninfo_investor_relation`, `kind=survey`, `code=300250`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected merged page pagination to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{`ir_page=`, `ir_code=`, `#investor-relations`} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected merged page pagination not to contain %q, got %s", notWant, body)
		}
	}
}

func TestStockResearchPagePostTriggersInvestorRelationsBackfill(t *testing.T) {
	var schedulerCalled bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerCalled = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/investor-relations/backfill" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("code") != "300250" || r.URL.Query().Get("start") == "" || r.URL.Query().Get("end") == "" {
			t.Fatalf("unexpected investor relations scheduler query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "triggered"}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	req := httptest.NewRequest(http.MethodPost, "/stock-research", strings.NewReader("scope=investor_relations&action=backfill_year&code=300250"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if !schedulerCalled {
		t.Fatal("expected investor relations scheduler backfill to be called")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after investor relations backfill, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"/stock-research?", "code=300250", "kind=survey", "source=cninfo_investor_relation", "page=1", "抓取和 PDF 解析任务已触发"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect to keep investor relations filters and message %q, got %q", want, loc)
		}
	}
	for _, notWant := range []string{"ir_code=", "ir_page=", "#investor-relations"} {
		if strings.Contains(loc, notWant) {
			t.Fatalf("expected redirect not to contain %q, got %q", notWant, loc)
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
					StockCode:       "002230",
					StockName:       "科大讯飞",
					ReportPeriod:    "20260331",
					HolderName:      "易方达基金",
					HolderType:      "fund",
					FundCompany:     "易方达基金管理有限公司",
					DisclosureScope: "fund_quarterly",
					Shares:          1000,
					SharesChange:    -250,
					ChangeRatio:     -1.25,
					FloatRatio:      1.5,
					SourceType:      "stock_institute_hold_detail",
					SourceURL:       "https://example.com/holding",
				}},
			}})
		case "/api/v1/a-stock/holdings/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{
				StockCode:        "002230",
				ReportPeriod:     "20260331",
				HolderCount:      2,
				FundCount:        1,
				HolderTypeCount:  2,
				FundCompanyCount: 1,
				TotalShares:      3000,
				TotalFloatRatio:  3.5,
				MaxHolderName:    "易方达基金",
				MaxHolderType:    "fund",
				MaxHolderShares:  1000,
			}})
		case "/api/v1/a-stock/holdings/signals":
			if r.URL.Query().Get("code") != "002230" || r.URL.Query().Get("period") != "20260331" {
				t.Fatalf("unexpected holdings signals query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSignalListResult{
				Page:           1,
				PageSize:       20,
				Total:          1,
				Code:           "002230",
				Period:         "20260331",
				CurrentPeriod:  "20260331",
				PreviousPeriod: "20251231",
				Thresholds: model.StockInstitutionSignalThreshold{
					HolderCountChange: 5,
					FundCountChange:   3,
					FloatRatioChange:  3,
				},
				Items: []model.StockInstitutionHoldingSignal{{
					StockCode:              "002230",
					StockName:              "科大讯飞",
					SignalType:             "new_entry",
					CurrentPeriod:          "20260331",
					PreviousPeriod:         "20251231",
					HolderCountChange:      5,
					NewHolderCount:         5,
					FundCountChange:        3,
					NewFundCount:           3,
					FundCompanyCountChange: 1,
					NewFundCompanyCount:    1,
					FloatRatioChange:       4,
					SharesChange:           6000,
					MarketValueChange:      120000,
					NewMajorHolders:        []string{"社保基金一一八组合"},
					SourceTypes:            []string{"stock_institute_hold_detail"},
					Level:                  "medium",
					Reason:                 "新进披露机构 5 家，新进基金 3 只，流通占比 +4.00%",
				}},
			}})
		case "/api/v1/a-stock/holding-reports":
			if r.URL.Query().Get("period") != "20260331" {
				t.Fatalf("unexpected holdings reports query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockHoldingReportDocumentListResult{
				Page:     1,
				PageSize: 10,
				Total:    1,
				Items: []model.StockHoldingReportDocument{{
					ReportPeriod:      "20260331",
					FundCode:          "110001",
					FundName:          "易方达蓝筹精选",
					FundCompany:       "易方达基金管理有限公司",
					AnnouncementTitle: "易方达蓝筹精选2026年第1季度报告",
					AnnouncementDate:  "2026-04-22",
					SourceType:        "tiantian_fund_regular_report",
					SourceURL:         "https://example.com/report",
					ParseStatus:       "indexed",
				}},
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
	for _, want := range []string{"机构持仓", "机构持仓异动", "新进", "退出披露名单", "2025-Q4 -> 2026-Q1", "社保基金一一八组合", "+4.00%", "共持摘要", "基金公司数", "2026-Q1", "易方达基金", "易方达基金管理有限公司", "基金季报索引", "易方达蓝筹精选2026年第1季度报告", "已索引", "3.50%", "回补近一年全市场", "body[data-page='a-stock-holdings'] main,body[data-page='a-stock-holdings'] .site-footer{max-width:none;width:98%;box-sizing:border-box}", ".holding-table,.holding-signal-table{font-size:12px;line-height:1.25}", ".holding-table th,.holding-table td,.holding-signal-table th,.holding-signal-table td{white-space:nowrap;padding:7px 10px}", ".holding-change-up{color:#c5221f;font-weight:700}.holding-change-down{color:#087a3d;font-weight:700}", `<span class="holding-change-up">+6000</span>`, `<span class="holding-change-up">+12.00万</span>`, `<span class="holding-change-down">-250</span>`, `<span class="holding-change-down">-1.25%</span>`, "<th>代表退出披露</th><th>原因</th>", "<th>公告日</th></tr>", "<th>解析状态</th></tr>"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected holdings page to contain %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"<th>来源</th>", "https://example.com/holding", "https://example.com/report"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected holdings page to hide %q, got %s", notWant, body)
		}
	}
	if strings.Contains(body, "body[data-page='a-stock-holdings'] header,body[data-page='a-stock-holdings'] main") {
		t.Fatalf("expected holdings header to keep the shared homepage alignment, got %s", body)
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

func TestAStockHoldingsPagePostTriggersFullMarketBackfill(t *testing.T) {
	var called bool
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/holdings/backfill" {
			t.Fatalf("unexpected scheduler holdings request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("expected full-market holdings backfill without filters, got query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "triggered"}})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	form := url.Values{"action": {"backfill_all"}, "code": {"002230"}, "period": {"20260331"}, "holder_type": {"fund"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock/holdings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockHoldingsPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("expected scheduler holdings full-market backfill call")
	}
	location, _ := url.QueryUnescape(rr.Header().Get("Location"))
	if !strings.Contains(location, "/a-stock/holdings?") || strings.Contains(location, "code=002230") || !strings.Contains(location, "近一年全市场机构持仓回补任务已触发") {
		t.Fatalf("expected redirect back to unfiltered holdings with msg, got %s", location)
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
			wantMsg:    "已切换到下午窗口，按 09:30-13:00:59 推荐生成窗口重新计算推荐。",
		},
		{
			name:       "regenerate morning from afternoon",
			fromPeriod: "afternoon",
			action:     "generate_morning_stock",
			wantPeriod: "morning",
			wantMsg:    "已切换到上午窗口，按 08:00-09:26:59 推荐生成窗口重新计算推荐。",
		},
		{
			name:       "ignore recent filter",
			fromPeriod: "morning",
			action:     "generate_ignore_recent_stock",
			wantPeriod: "morning",
			wantMsg:    "上午推荐已忽略90个交易日内重复推荐过滤，按当前新闻窗口重新计算推荐。",
			wantIgnore: true,
		},
		{
			name:       "refresh all backtests",
			fromPeriod: "morning",
			action:     "refresh_backtest",
			wantPeriod: "morning",
			wantMsg:    "上午、下午和晚间消息回测已按当前推荐股票、13:01价格和行情收益重新刷新。",
		},
		{
			name:       "refresh current backtest",
			fromPeriod: "afternoon",
			action:     "refresh_current_backtest",
			wantPeriod: "afternoon",
			wantMsg:    "下午推荐行情收益未补齐：未找到当前推荐股票。",
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
			if strings.Contains(loc, "refresh_recommendations=1") || strings.Contains(loc, "refresh_all_backtests=1") {
				t.Fatalf("expected POST action to persist before redirect instead of delegating writes to GET, got %q", loc)
			}
			decoded, _ := url.QueryUnescape(loc)
			if !strings.Contains(decoded, tc.wantMsg) {
				t.Fatalf("expected generation message %q, got %q", tc.wantMsg, decoded)
			}
		})
	}
}

func TestAStockGenerateAfternoonActionRebuildsAndPersistsSnapshot(t *testing.T) {
	currentSelectionGets := 0
	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          16,
					SourceType:  "flash",
					Title:       "下午AI算力活跃，科大讯飞走强",
					Summary:     "人工智能产业链活跃",
					PublishTime: "2026-06-16 10:05:00",
					TagFlags:    "0.002230",
					CapturedAt:  time.Date(2026, 6, 16, 2, 5, 0, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("date") == "2026-06-16" && r.URL.Query().Get("period") == "afternoon" {
				currentSelectionGets++
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "999999", Name: "旧锁定", Hotspot: "旧热点", Reason: "locked"}},
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	form := url.Values{"date": {"2026-06-16"}, "period": {"morning"}, "action": {"generate_afternoon_stock"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if currentSelectionGets != 0 {
		t.Fatalf("expected rebuild action to skip current locked afternoon selections, got %d reads", currentSelectionGets)
	}
	if savedSnapshot.StrategyDate != "2026-06-16" || savedSnapshot.Period != "afternoon" || strings.TrimSpace(savedSnapshot.NewsSummaryJSON) == "" {
		t.Fatalf("expected rebuilt afternoon snapshot with news summary to be persisted, got %+v", savedSnapshot)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-16", "period=afternoon", "已切换到下午窗口，按 09:30-13:00:59 推荐生成窗口重新计算推荐"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect message %q, got %q", want, loc)
		}
	}
}

func TestAStockBacktestPageRendersStandaloneBacktestAndNavigation(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	srv := NewServer(config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-06-16&period=afternoon&ignore_recent=1&filter_today_market=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"<title>A股回测</title>",
		"body data-page='a-stock-backtest'",
		`href="/a-stock">A股</a><a href="/a-stock/backtest">回测</a><a href="/sector-fund-flow">版块资金</a>`,
		"消息回测",
		"推荐历史",
		"上午推荐",
		"下午推荐",
		"实时价",
		"T+0 收益",
		"五日内最高收益",
		`/a-stock/backtest?date=2026-06-16&period=afternoon&ignore_recent=1&filter_today_market=1`,
		`/a-stock/backtest?date=2026-06-15&period=afternoon&ignore_recent=1&filter_today_market=1`,
		`href="/a-stock/backtest?date=2026-06-16&amp;period=afternoon&amp;filter_today_market=1"`,
		`action="/a-stock/backtest"`,
		`name="action" value="repair_stock_names"`,
		`name="action" value="refresh_current_backtest"`,
		`name="action" value="refresh_backtest"`,
		"补股票名称",
		"补行情收益",
		"刷新全部回测",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股回测 page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, `href="/a-stock?date=2026-06-16&period=afternoon`) {
		t.Fatalf("expected backtest page date and period links to stay on /a-stock/backtest, got %s", body)
	}
	if strings.Contains(body, `href="/a-stock/backtest?date=2026-06-16&amp;period=morning`) {
		t.Fatalf("expected backtest page to hide morning/afternoon period switch buttons, got %s", body)
	}
	if strings.Contains(body, "实时收益") {
		t.Fatalf("expected backtest table to hide realtime return column, got %s", body)
	}
}

func TestAStockBacktestCurrentPriceClassUsesCurrentReturn(t *testing.T) {
	row := aStockBacktestRow{
		CurrentPrice:          "277.29",
		CurrentReturn:         "-1.32%",
		CurrentReturnClass:    "astock-down",
		CurrentMarketPct:      "+2.88%",
		CurrentMarketPctClass: "astock-up",
	}
	if got := aStockBacktestCurrentReturnClass(row); got != "astock-down" {
		t.Fatalf("expected realtime price color to follow current return, got %q", got)
	}
	row.CurrentReturn = "--"
	row.CurrentReturnClass = ""
	if got := aStockBacktestCurrentReturnClass(row); got != "astock-up" {
		t.Fatalf("expected realtime price color to fall back to market pct, got %q", got)
	}
}

func TestAStockFundFlowFilterSessionSyncsBetweenRecommendationAndBacktest(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 9, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	snapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-07-03",
		Period:                "morning",
		FundFlowFilterEnabled: false,
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "人工智能", Code: "601995", Name: "中金公司"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "601995 中金公司", EntryOpen: "36.38", T0Return: "+0.00%", T0Close: "36.38", Status: "已回测T+2"}}),
		BacktestStatus:        "已锁定推荐股票，已回测 1/1",
	}
	var queryMu sync.Mutex
	fundFlowSnapshotQueries := make([]string, 0)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			queryMu.Lock()
			fundFlowSnapshotQueries = append(fundFlowSnapshotQueries, r.URL.RawQuery)
			queryMu.Unlock()
			if r.URL.Query().Get("date") == "2026-07-03" && r.URL.Query().Get("period") == "morning" && r.URL.Query().Get("fund_flow_filter_enabled") == "0" {
				writeEnvelope(w, http.StatusOK, "ok", snapshot)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 1000, Total: 0})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	pageReq := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-07-03&period=morning&ignore_fund_flow=1", nil)
	pageRR := httptest.NewRecorder()
	srv.handleAStockPage(pageRR, pageReq, map[string]any{"id": 1})
	disabledCookie := aStockCookieByName(pageRR.Result().Cookies(), aStockFundFlowFilterCookieName)
	if disabledCookie == nil || disabledCookie.Value != aStockFundFlowFilterCookieDisabled {
		t.Fatalf("expected disabled fund-flow cookie, got %+v", pageRR.Result().Cookies())
	}

	backtestReq := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-07-03&period=morning", nil)
	backtestReq.AddCookie(disabledCookie)
	backtestRR := httptest.NewRecorder()
	srv.handleAStockBacktestPage(backtestRR, backtestReq, map[string]any{"id": 1})
	if backtestRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", backtestRR.Code, backtestRR.Body.String())
	}
	backtestBody := backtestRR.Body.String()
	if !strings.Contains(backtestBody, "601995 中金公司") || strings.Contains(backtestBody, "暂无回测结果") {
		t.Fatalf("expected backtest page to use disabled fund-flow session state, got %s", backtestBody)
	}

	enableReq := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-07-03&period=morning&filter_fund_flow=1", nil)
	enableReq.AddCookie(disabledCookie)
	enableRR := httptest.NewRecorder()
	srv.handleAStockBacktestPage(enableRR, enableReq, map[string]any{"id": 1})
	enabledCookie := aStockCookieByName(enableRR.Result().Cookies(), aStockFundFlowFilterCookieName)
	if enabledCookie == nil || enabledCookie.Value != aStockFundFlowFilterCookieEnabled {
		t.Fatalf("expected enabled fund-flow cookie, got %+v", enableRR.Result().Cookies())
	}
	queryMu.Lock()
	defer queryMu.Unlock()
	if !slices.ContainsFunc(fundFlowSnapshotQueries, func(raw string) bool {
		values, err := url.ParseQuery(raw)
		return err == nil && values.Get("period") == "morning" && values.Get("fund_flow_filter_enabled") == "0"
	}) {
		t.Fatalf("expected snapshot requests to include disabled fund-flow exact filter, got %v", fundFlowSnapshotQueries)
	}
}

func TestAStockJuneDefaultFundFlowDisabledIgnoresEnabledCookie(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 10, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	snapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-06-18",
		Period:                "morning",
		FundFlowFilterEnabled: false,
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞", Reason: "六月关闭资金过滤快照"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "002230 科大讯飞", EntryOpen: "11.05", T0Return: "+1.23%", T0Close: "11.19", Status: "已回测T+1"}}),
		BacktestStatus:        "已读取六月关闭资金过滤快照",
		GeneratedCount:        1,
	}
	var mu sync.Mutex
	snapshotQueries := make([]url.Values, 0)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			mu.Lock()
			snapshotQueries = append(snapshotQueries, r.URL.Query())
			mu.Unlock()
			if r.URL.Query().Get("date") == "2026-06-18" && r.URL.Query().Get("period") == "morning" && r.URL.Query().Get("fund_flow_filter_enabled") == "0" {
				writeEnvelope(w, http.StatusOK, "ok", snapshot)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 1000, Total: 0})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	enabledCookie := &http.Cookie{Name: aStockFundFlowFilterCookieName, Value: aStockFundFlowFilterCookieEnabled, Path: "/"}
	pageReq := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-18&period=morning", nil)
	pageReq.AddCookie(enabledCookie)
	pageRR := httptest.NewRecorder()
	srv.handleAStockPage(pageRR, pageReq, map[string]any{"id": 1})
	pageBody := pageRR.Body.String()
	if pageRR.Code != http.StatusOK || !strings.Contains(pageBody, "002230") || !strings.Contains(pageBody, "科大讯飞") {
		t.Fatalf("expected June page to read disabled fund-flow snapshot, code=%d body=%s", pageRR.Code, pageRR.Body.String())
	}

	backtestReq := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-06-18&period=morning", nil)
	backtestReq.AddCookie(enabledCookie)
	backtestRR := httptest.NewRecorder()
	srv.handleAStockBacktestPage(backtestRR, backtestReq, map[string]any{"id": 1})
	backtestBody := backtestRR.Body.String()
	if backtestRR.Code != http.StatusOK || !strings.Contains(backtestBody, "002230") || !strings.Contains(backtestBody, "科大讯飞") {
		t.Fatalf("expected June backtest to read disabled fund-flow snapshot, code=%d body=%s", backtestRR.Code, backtestRR.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.ContainsFunc(snapshotQueries, func(values url.Values) bool {
		return values.Get("date") == "2026-06-18" && values.Get("period") == "morning" && values.Get("fund_flow_filter_enabled") == "0"
	}) {
		t.Fatalf("expected June snapshot queries to request fund_flow_filter_enabled=0, got %v", snapshotQueries)
	}
}

func TestAStockExplicitFundFlowAndJulyCookieDefaults(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 10, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var mu sync.Mutex
	snapshotQueries := make([]url.Values, 0)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			query := r.URL.Query()
			mu.Lock()
			snapshotQueries = append(snapshotQueries, query)
			mu.Unlock()
			fundFlowEnabled := query.Get("fund_flow_filter_enabled") != "0"
			snapshot := model.AStockRecommendationSnapshot{
				Found:                 true,
				StrategyDate:          query.Get("date"),
				Period:                normalizeAStockPeriod(query.Get("period")).Key,
				FundFlowFilterEnabled: fundFlowEnabled,
				RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "测试热点", Code: "600000", Name: "测试银行"}}),
				BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600000 测试银行", EntryOpen: "10.00", T0Return: "+0.00%", T0Close: "10.00", Status: "已回测T+0"}}),
				BacktestStatus:        "已读取测试快照",
				GeneratedCount:        1,
			}
			writeEnvelope(w, http.StatusOK, "ok", snapshot)
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 1000, Total: 0})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	for _, tc := range []struct {
		name       string
		targetURL  string
		cookie     *http.Cookie
		wantDate   string
		wantFilter string
	}{
		{name: "june explicit enabled", targetURL: "/a-stock?date=2026-06-18&period=morning&filter_fund_flow=1", wantDate: "2026-06-18", wantFilter: "1"},
		{name: "july default enabled", targetURL: "/a-stock?date=2026-07-09&period=morning", wantDate: "2026-07-09", wantFilter: "1"},
		{name: "july disabled cookie", targetURL: "/a-stock?date=2026-07-09&period=morning", cookie: &http.Cookie{Name: aStockFundFlowFilterCookieName, Value: aStockFundFlowFilterCookieDisabled, Path: "/"}, wantDate: "2026-07-09", wantFilter: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.targetURL, nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rr := httptest.NewRecorder()
			srv.handleAStockPage(rr, req, map[string]any{"id": 1})
			body := rr.Body.String()
			if rr.Code != http.StatusOK || !strings.Contains(body, "600000") || !strings.Contains(body, "测试银行") {
				t.Fatalf("expected page to render test snapshot, code=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []struct {
		date   string
		filter string
	}{
		{date: "2026-06-18", filter: "1"},
		{date: "2026-07-09", filter: "1"},
		{date: "2026-07-09", filter: "0"},
	} {
		if !slices.ContainsFunc(snapshotQueries, func(values url.Values) bool {
			return values.Get("date") == want.date && values.Get("period") == "morning" && values.Get("fund_flow_filter_enabled") == want.filter
		}) {
			t.Fatalf("expected snapshot query date=%s fund_flow_filter_enabled=%s, got %v", want.date, want.filter, snapshotQueries)
		}
	}
}

func TestAStockJuneDateLinksDoNotLeakFundFlowDisabledToJuly(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 10, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	srv := NewServer(config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-30&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `/a-stock?date=2026-07-01&period=morning`) {
		t.Fatalf("expected June page to render July date link without fund-flow parameter, got %s", body)
	}
	if strings.Contains(body, `/a-stock?date=2026-07-01&period=morning&ignore_fund_flow=1`) {
		t.Fatalf("expected June page not to leak ignore_fund_flow=1 into July date link, got %s", body)
	}
}

func TestAStockBacktestPageGetUsesSnapshotOnly(t *testing.T) {
	morningSnapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-06-30",
		Period:                "morning",
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "快照热点", Code: "603986", Name: "兆易创新", Reason: "snapshot morning"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "603986 兆易创新", EntryOpen: "721.00", T0Return: "-3.63%", T0Close: "694.80", T0ReturnClass: "astock-down", BestReturn: "-3.63%", BestReturnClass: "astock-down", Status: "已回测T+1"}}),
		BacktestStatus:        "已读取上午快照",
		GeneratedCount:        1,
		FundFlowFilterEnabled: true,
	}
	afternoonSnapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-06-30",
		Period:                "afternoon",
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "快照热点", Code: "600519", Name: "贵州茅台", Reason: "snapshot afternoon"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600519 贵州茅台", AfternoonOpen: "1418.00", T0Return: "+1.25%", T0Close: "1435.73", T0ReturnClass: "astock-up", BestReturn: "+1.25%", BestReturnClass: "astock-up", Status: "已回测T+1"}}),
		BacktestStatus:        "已读取下午快照",
		GeneratedCount:        1,
		LimitUpFilterEnabled:  true,
		FundFlowFilterEnabled: true,
	}
	var mu sync.Mutex
	requests := []string{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.String())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/recommendation-performance" {
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationPerformanceSummary{
				Strategy: r.URL.Query().Get("strategy"),
				Period:   r.URL.Query().Get("period"),
			})
			return
		}
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			http.Error(w, "unexpected endpoint", http.StatusInternalServerError)
			return
		}
		period := r.URL.Query().Get("period")
		switch period {
		case "morning":
			if got := r.URL.Query().Get("date"); got != "2026-06-30" {
				http.Error(w, "unexpected date", http.StatusBadRequest)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", morningSnapshot)
		case "afternoon":
			if got := r.URL.Query().Get("date"); got != "2026-06-30" {
				http.Error(w, "unexpected date", http.StatusBadRequest)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", afternoonSnapshot)
		case "evening":
			if got := r.URL.Query().Get("date"); got != "2026-06-29" {
				http.Error(w, "unexpected evening date", http.StatusBadRequest)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		default:
			http.Error(w, "unexpected period", http.StatusBadRequest)
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-06-30&period=morning&refresh_recommendations=1&refresh_all_backtests=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"消息回测", "上午推荐", "下午推荐", "603986 兆易创新", "600519 贵州茅台", "721.00", "1418.00"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected snapshot-only backtest page to contain %q, got %s", want, body)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 6 {
		t.Fatalf("expected morning/afternoon/evening snapshots plus three performance requests, got %d: %v", len(requests), requests)
	}
	periodHits := map[string]int{}
	performanceHits := 0
	for _, raw := range requests {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse request URL %q: %v", raw, err)
		}
		if u.Path == "/api/v1/a-stock/recommendation-performance" {
			performanceHits++
			continue
		}
		if u.Path != "/api/v1/a-stock/recommendations" {
			t.Fatalf("expected snapshot/performance endpoint, got %q in %v", u.Path, requests)
		}
		periodHits[u.Query().Get("period")]++
	}
	if periodHits["morning"] != 1 || periodHits["afternoon"] != 1 || periodHits["evening"] != 1 {
		t.Fatalf("expected one morning, afternoon, and evening snapshot request, got %v from %v", periodHits, requests)
	}
	if performanceHits != 3 {
		t.Fatalf("expected official and shadow performance requests, got %d from %v", performanceHits, requests)
	}
}

func TestAStockBacktestPageShowsHistoricalSnapshotWhenFundFlowStateDiffers(t *testing.T) {
	snapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-07-03",
		Period:                "morning",
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "人工智能", Code: "601995", Name: "中金公司", Reason: "snapshot"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "601995 中金公司", EntryOpen: "36.38", T0Return: "+0.00%", T0Close: "36.38", T0ReturnClass: "astock-flat", BestReturn: "+2.36%", BestReturnClass: "astock-up", Status: "已回测T+2"}}),
		BacktestStatus:        "已锁定推荐股票，已回测 1/1",
		GeneratedCount:        1,
		FundFlowFilterEnabled: false,
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			http.Error(w, "unexpected endpoint", http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("period") == "morning" {
			writeEnvelope(w, http.StatusOK, "ok", snapshot)
			return
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-07-03&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "601995 中金公司") || strings.Contains(body, "暂无回测结果") {
		t.Fatalf("expected historical snapshot to render despite current fund-flow state, got %s", body)
	}
	if !strings.Contains(body, `name="ignore_fund_flow" value="1"`) {
		t.Fatalf("expected rendered actions to reflect disabled fund-flow snapshot state, got %s", body)
	}
}

func TestAStockBacktestPageShowsRecommendationsWithoutBacktestRows(t *testing.T) {
	snapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-07-08",
		Period:                "afternoon",
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "机器人", Code: "300024", Name: "机器人", Reason: "snapshot recommendation"}}),
		BacktestsJSON:         "null",
		BacktestStatus:        "等待行情同步",
		GeneratedCount:        1,
		LimitUpFilterEnabled:  true,
		FundFlowFilterEnabled: true,
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			http.Error(w, "unexpected endpoint", http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("period") == "afternoon" {
			writeEnvelope(w, http.StatusOK, "ok", snapshot)
			return
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
			Found:                 true,
			StrategyDate:          "2026-07-08",
			Period:                "morning",
			RecommendationsJSON:   "[]",
			BacktestsJSON:         "[]",
			BacktestStatus:        "无推荐股票",
			FundFlowFilterEnabled: true,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-07-08&period=afternoon", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"下午推荐", "300024 机器人", "等待行情同步"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected backtest page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "暂无回测结果，等待行情同步。") {
		t.Fatalf("expected recommendation placeholder row instead of empty backtest table, got %s", body)
	}
}

func TestAStockBacktestPageFallsBackToTodayReadOnlyRecommendations(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 8, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": []map[string]any{
			{"code": "002230", "date": "2026-07-07", "open": 40.00, "close": 41.00, "pct": 1.10},
			{"code": "002230", "date": "2026-07-08", "open": 42.00, "close": 43.00, "pct": 4.88, "entry_price": 42.00},
		}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	internalWrites := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          1,
					SourceType:  "flash",
					Title:       "人工智能产业链活跃 科大讯飞走强",
					Summary:     "AI 算力需求增长",
					PublishTime: "2026-07-08 09:05:00",
					TagFlags:    "0.002230",
					CapturedAt:  time.Date(2026, 7, 8, 1, 5, 0, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date:       "2026-07-08",
				LatestDate: "2026-07-08",
				Page:       1,
				PageSize:   5000,
				Items: []model.AStockAuctionAmount{{
					TradeDate:     "2026-07-08",
					Code:          "002230",
					Name:          "科大讯飞",
					AuctionAmount: 100000000,
				}},
			})
		case "/api/v1/a-stock/recommendation-latest-dates":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations", "/api/v1/internal/a-stock/recommendation-selections":
			internalWrites++
			http.Error(w, "GET backtest must not write", http.StatusInternalServerError)
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-07-08&period=morning&ignore_fund_flow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"上午推荐", "精确快照待生成"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected missing exact snapshot backtest page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "<tr><td>上午推荐</td><td>") || !strings.Contains(body, "精确快照待生成") {
		t.Fatalf("expected missing exact snapshot backtest page to stay read-only empty, got %s", body)
	}
	if internalWrites != 0 {
		t.Fatalf("expected GET missing snapshot path to avoid internal writes, got %d", internalWrites)
	}
}

func TestAStockBacktestSnapshotMissingDoesNotFallback(t *testing.T) {
	requests := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			http.Error(w, "unexpected endpoint", http.StatusInternalServerError)
			return
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockBacktestSnapshotContextWithCache("2026-06-30", "afternoon", 1, false, false, false, false, newAStockRequestCache())

	if ctx.BacktestStatus != "无推荐快照" || !strings.Contains(ctx.EmptyReason, "下午推荐精确快照待生成") {
		t.Fatalf("expected missing snapshot empty state, got status=%q reason=%q", ctx.BacktestStatus, ctx.EmptyReason)
	}
	if len(ctx.Recommendations) != 0 || len(ctx.Backtests) != 0 {
		t.Fatalf("expected missing snapshot to stay empty, got recommendations=%+v backtests=%+v", ctx.Recommendations, ctx.Backtests)
	}
	if requests != 1 {
		t.Fatalf("expected only one snapshot request, got %d", requests)
	}
}

func TestAStockBacktestPagePostRedirectsBackToBacktest(t *testing.T) {
	srv := NewServer(config.Config{})
	form := url.Values{"date": {"2026-06-16"}, "period": {"afternoon"}, "action": {"refresh_backtest"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock/backtest", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/a-stock/backtest?") || !strings.Contains(loc, "date=2026-06-16") || !strings.Contains(loc, "period=afternoon") {
		t.Fatalf("expected redirect back to /a-stock/backtest with state, got %q", loc)
	}
	decoded, _ := url.QueryUnescape(loc)
	if !strings.Contains(decoded, "上午、下午和晚间消息回测已按当前推荐股票、13:01价格和行情收益重新刷新。") {
		t.Fatalf("expected refresh backtest message, got %q", decoded)
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
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"astock-action-grid", "抓取全部财经信息", "补录上午新闻", "重新生成上午推荐", "同步行情", "补录集合竞价", "刷新回测结果", "生成全部推荐股票", "补录下午新闻", "重新生成下午推荐", `name="action" value="backfill_window_news"`, `name="period" value="morning"`, `name="period" value="afternoon"`, `name="action" value="backfill_auction"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected backfill action %q, got %s", want, body)
		}
	}
	actionStart := strings.Index(body, `<section><h2>操作区</h2>`)
	if actionStart < 0 {
		t.Fatalf("expected action section, got %s", body)
	}
	actionEnd := strings.Index(body[actionStart:], `<p class="astock-muted">`)
	if actionEnd < 0 {
		t.Fatalf("expected action section footer, got %s", body)
	}
	actionHTML := body[actionStart : actionStart+actionEnd]
	for _, notWant := range []string{"astock-action-table", "<table", "<tbody", "<tr", "<td"} {
		if strings.Contains(actionHTML, notWant) {
			t.Fatalf("expected action section to use buttons without table markup %q, got %s", notWant, actionHTML)
		}
	}
	expectedOrder := []string{"抓取全部财经信息", "补录上午新闻", "重新生成上午推荐", "同步行情", "补录集合竞价", "刷新回测结果", "生成全部推荐股票", "补录下午新闻", "重新生成下午推荐"}
	last := -1
	for _, want := range expectedOrder {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Fatalf("expected action %q, got %s", want, body)
		}
		if idx < last {
			t.Fatalf("expected action %q to render after previous action, got %s", want, body)
		}
		last = idx
	}
}

func TestAStockBackfillAuctionActionUsesSelectedDate(t *testing.T) {
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/auction/backfill" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("days") != "1" || r.URL.Query().Get("start") != "2026-06-17" || r.URL.Query().Get("end") != "2026-06-17" {
			t.Fatalf("unexpected auction backfill query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"result": map[string]any{
				"succeeded": 1,
				"skipped":   0,
				"failed":    0,
			}},
		})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	form := url.Values{"date": {"2026-06-17"}, "period": {"afternoon"}, "action": {"backfill_auction"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-17", "period=afternoon", "已补录 2026-06-17 集合竞价", "请重新生成推荐"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected auction backfill redirect message %q, got %q", want, loc)
		}
	}
}

func TestAStockBackfillAuctionActionExplainsHistoryUnavailable(t *testing.T) {
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/scheduler/a-stock/auction/backfill" {
			t.Fatalf("unexpected scheduler request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "a-stock auction backfill skipped all dates without usable data: 2026-06-17: AKShare auction adapter only serves the latest trading day (2026-06-18) without a usable local cache for the requested date.",
		})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{SchedulerURL: scheduler.URL, ServiceToken: "secret-token"})
	form := url.Values{"date": {"2026-06-17"}, "period": {"afternoon"}, "action": {"backfill_auction"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-17", "period=afternoon", "集合竞价无可补录数据", "历史集合竞价金额保持 --"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected history unavailable message %q, got %q", want, loc)
		}
	}
	if strings.Contains(loc, "集合竞价补录失败") {
		t.Fatalf("expected history unavailable message not to be marked as failure, got %q", loc)
	}
}

func TestAStockBackfillWindowActionPassesMorningWindow(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": []model.CrawlRun{}})
			return
		}
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
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			items := []model.Item{}
			if r.URL.Query().Get("start") == "2026-06-16 08:00:00" && r.URL.Query().Get("end") == "2026-06-16 09:30:59" {
				items = []model.Item{{
					ID:         1,
					SourceType: "flash",
					Title:      "上午财经新闻",
					CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC),
				}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data":    model.ItemListResult{Items: items, Page: 1, PageSize: 200, Total: len(items)},
			})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{})
		case "/api/v1/internal/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
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
	for _, sourceType := range []string{provider.SourceTypeFlash, provider.SourceTypeHeadline, provider.SourceTypeJin10Full, provider.SourceTypeEastMoneyKuaixun, provider.SourceTypeWallStreetCNAStock, provider.SourceTypeCLSTelegraph, provider.SourceTypeSinaFinance7x24} {
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

func TestAStockBackfillWindowActionPassesAfternoonWindowAndPersistsSnapshot(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	var savedSnapshot model.AStockRecommendationSnapshot
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": []model.CrawlRun{}})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16 09:30:00" || r.URL.Query().Get("end") != "2026-06-16 13:00:59" || r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected afternoon backfill window query: %s", r.URL.RawQuery)
		}
		mu.Lock()
		seen[r.URL.Query().Get("source_type")] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok"})
	}))
	defer crawler.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			items := []model.Item{}
			if r.URL.Query().Get("start") == "2026-06-16 09:30:00" && r.URL.Query().Get("end") == "2026-06-16 13:00:59" {
				items = []model.Item{{
					ID:          16,
					SourceType:  "flash",
					Title:       "下午AI算力活跃",
					Summary:     "人工智能产业链活跃",
					PublishTime: "2026-06-16 10:05:00",
					TagFlags:    "0.002230",
					CapturedAt:  time.Date(2026, 6, 16, 2, 5, 0, 0, time.UTC),
				}}
			}
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: items, Page: 1, PageSize: 200, Total: len(items)})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{CrawlerURL: crawler.URL, ContentURL: content.URL})
	form := url.Values{"date": {"2026-06-16"}, "period": {"afternoon"}, "action": {"backfill_window_news"}}
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	for _, sourceType := range []string{provider.SourceTypeFlash, provider.SourceTypeHeadline, provider.SourceTypeJin10Full, provider.SourceTypeEastMoneyKuaixun, provider.SourceTypeWallStreetCNAStock, provider.SourceTypeCLSTelegraph, provider.SourceTypeSinaFinance7x24} {
		if !seen[sourceType] {
			t.Fatalf("expected source %s to be backfilled, got %+v", sourceType, seen)
		}
	}
	if savedSnapshot.StrategyDate != "2026-06-16" || savedSnapshot.Period != "afternoon" || strings.TrimSpace(savedSnapshot.NewsSummaryJSON) == "" {
		t.Fatalf("expected afternoon snapshot with news summary to be persisted, got %+v", savedSnapshot)
	}
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-16", "period=afternoon", "已补抓 2026-06-16 下午 09:30-13:00", "当前窗口已有 1 条财经新闻"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect message %q, got %q", want, loc)
		}
	}
}

func TestAStockBackfillWindowActionExplainsZeroWindowNews(t *testing.T) {
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": []model.CrawlRun{}})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" || r.URL.Query().Get("end") != "2026-06-16 09:30:59" || r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected backfill window query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.CrawlSummary{
				SourceType: r.URL.Query().Get("source_type"),
			},
		})
	}))
	defer crawler.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data":    model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
			})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{})
		case "/api/v1/internal/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
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
	loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
	for _, want := range []string{"date=2026-06-16", "period=morning", "源站返回 0 条，窗口内入库 0 条、更新 0 条", "当前窗口已有 0 条财经新闻", "没有新闻：本次补抓没有写入 08:00-09:30 窗口内带 publish_time 的可用新闻"} {
		if !strings.Contains(loc, want) {
			t.Fatalf("expected redirect message %q, got %q", want, loc)
		}
	}
}

func TestAStockBackfillMorningStockActionSelectsMorningWindow(t *testing.T) {
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": []model.CrawlRun{}})
			return
		}
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
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
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
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"上午推荐", "08:00-09:30", "没有新闻", "请先抓取或补抓财经信息"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected no-news explanation %q, got %s", want, body)
		}
	}
}

func TestAStockPageDoesNotBuildRecommendationsWithoutAuctionOrStockMentions(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/v1/a-stock/holdings/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{}})
		case "/api/v1/a-stock/auction":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.AStockAuctionListResult{
				Date:  r.URL.Query().Get("date"),
				Items: []model.AStockAuctionAmount{},
			}})
		case "/api/v1/articles":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{
				Items: []model.Item{{ID: 1, SourceType: "flash", Title: "人工智能产业链活跃", Summary: "AI 算力需求增长", PublishTime: "2026-06-16 09:05:00"}},
				Page:  1, PageSize: 200, Total: 1,
			}})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"人工智能", "暂无推荐股票", "没有集合竞价候选数据", "补录集合竞价"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected no-candidate recommendation explanation %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"科大讯飞"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected old static stock/content to be absent %q, got %s", notWant, body)
		}
	}
}

func TestAStockPageExplainsNewsWithoutHotspots(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
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
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning&refresh_recommendations=1", nil)
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

func TestAStockRecentFilterDefaultsOn(t *testing.T) {
	if normalizeAStockIgnoreRecent(url.Values{}) {
		t.Fatal("expected recent recommendation filter to be enabled by default")
	}
	if normalizeAStockIgnoreRecent(url.Values{"filter_recent": {"1"}}) {
		t.Fatal("expected filter_recent=1 to enable the recent recommendation filter")
	}
	if !normalizeAStockIgnoreRecent(url.Values{"ignore_recent": {"1"}}) {
		t.Fatal("expected ignore_recent=1 to keep ignoring the recent recommendation filter")
	}
}

func TestAStockLimitUpFilterDefaultsOnForAfternoon(t *testing.T) {
	if normalizeAStockIgnoreLimitUp(url.Values{}) {
		t.Fatal("expected limit-up filter to be enabled by default")
	}
	if normalizeAStockIgnoreLimitUp(url.Values{"filter_limit_up": {"1"}}) {
		t.Fatal("expected filter_limit_up=1 to enable the limit-up filter")
	}
	if !normalizeAStockIgnoreLimitUp(url.Values{"ignore_limit_up": {"1"}}) {
		t.Fatal("expected ignore_limit_up=1 to disable the limit-up filter")
	}
}

func TestAStockTodayMarketFilterDefaultsOff(t *testing.T) {
	if normalizeAStockFilterTodayMarket(url.Values{}) {
		t.Fatal("expected today-market filter to default disabled")
	}
	if !normalizeAStockFilterTodayMarket(url.Values{"filter_today_market": {"1"}}) {
		t.Fatal("expected filter_today_market=1 to enable today-market filter")
	}
	if !normalizeAStockFilterTodayMarket(url.Values{"require_today_market": {"1"}}) {
		t.Fatal("expected require_today_market=1 to enable today-market filter")
	}
}

func TestAStockPageLoadsAfternoonWindow(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []map[string]any{
				{"code": "300777", "date": "2026-04-17", "open": 20.00, "close": 20.00, "pct": 0},
				{"code": "300777", "date": "2026-05-16", "open": 20.00, "close": 20.00, "pct": 0},
				{"code": "300777", "date": "2026-06-15", "open": 20.80, "close": 21.00, "pct": 1.00},
				{"code": "300777", "date": "2026-06-16", "open": 21.60, "afternoon_entry_price": 21.60, "close": 22.00, "pct": 4.76},
				{"code": "300777", "date": "2026-06-17", "open": 22.20, "close": 23.00, "pct": 4.55},
			}},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/stock-fund-flow-trend" {
			handleEmptyAStockAuctionTestEndpoint(w, r)
			return
		}
		if r.URL.Path == "/api/v1/a-stock/auction" {
			if r.URL.Query().Get("date") != "2026-06-16" {
				t.Fatalf("unexpected auction date: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("page_size") != "5000" {
				t.Fatalf("expected A股 page to request full-market auction candidates, got %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.AStockAuctionListResult{
					Date:     "2026-06-16",
					Page:     1,
					PageSize: 200,
					Total:    1,
					Items: []model.AStockAuctionAmount{
						{TradeDate: "2026-06-16", Code: "300777", Name: "无人机龙头", AuctionPrice: 21.60, AuctionVolume: 1800000, AuctionAmount: 38880000, Status: "ok"},
					},
				},
			})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 afternoon window query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("start") != "2026-06-16 09:30:00" || r.URL.Query().Get("end") != "2026-06-16 13:00:59" {
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
					{ID: 601, SourceType: "eastmoney_kuaixun", Title: "午间低空经济订单增加", Summary: "无人机产业链升温", TagFlags: "0.300777", CapturedAt: time.Date(2026, 6, 16, 4, 42, 0, 0, time.UTC)},
				},
				Page:     1,
				PageSize: 200,
				Total:    1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=afternoon&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"下午推荐", "09:30-13:00", `财经新闻数</span><strong>1</strong>`, "无人机龙头"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 afternoon page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "午间低空经济订单增加") {
		t.Fatalf("expected A股 afternoon page not to render individual news title, got %s", body)
	}
	if strings.Contains(body, "中信海直") {
		t.Fatalf("expected static low-altitude stock to be absent from recommendation table, got %s", body)
	}
}

func TestAStockPageOffersTodayNavigationAndAfterAlias(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 16, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("expected after alias to use afternoon window, got query: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.Query().Get("start"), "09:30") {
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
			return
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
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-11&period=after&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"下午推荐",
		"2026-06-09 周二",
		"2026-06-10 周三",
		"2026-06-11 周四",
		"2026-06-12 周五",
		"2026-06-15 周一",
		`href="/a-stock?date=2026-06-15&period=afternoon"`,
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

func writeEnvelope(w http.ResponseWriter, status int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    status,
		"message": message,
		"data":    data,
	})
}

func TestAStockReadOnlySnapshotWithNewsSummarySkipsArticleAPI(t *testing.T) {
	summaryJSON, err := json.Marshal(aStockSnapshotNewsSummary{
		Articles: []model.Item{{
			ID:          1,
			SourceType:  "flash",
			Title:       "AI 算力新闻",
			PublishTime: "2026-06-15 08:30:00",
		}},
		NewsArticles: []model.Item{{
			ID:          1,
			SourceType:  "flash",
			Title:       "AI 算力新闻",
			PublishTime: "2026-06-15 08:30:00",
		}},
		Hotspots: []aStockHotspot{{Name: "人工智能", Score: 1, Evidence: 1}},
	})
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/articles" {
			t.Fatalf("expected snapshot news_summary_json to avoid articles API, got %s", r.URL.String())
		}
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
			Found:                 true,
			StrategyDate:          "2026-06-15",
			Period:                r.URL.Query().Get("period"),
			RecommendationsJSON:   "[]",
			BacktestsJSON:         "[]",
			NewsSummaryJSON:       string(summaryJSON),
			BacktestStatus:        "无推荐股票",
			FundFlowFilterEnabled: true,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx, ok := srv.loadAStockReadOnlySnapshotContextWithCache("2026-06-15", "morning", 1, false, false, false, false, newAStockRequestCache())
	if !ok {
		t.Fatal("expected snapshot context")
	}
	if ctx.NewsTotal != 1 || len(ctx.Hotspots) != 1 || ctx.Hotspots[0].Name != "人工智能" {
		t.Fatalf("expected snapshot summary to populate article stats, got %+v", ctx)
	}
}

func TestAStockReadOnlySnapshotDoesNotApplyOutputFilters(t *testing.T) {
	recommendationsJSON, err := json.Marshal([]aStockRecommendation{
		{Rank: 1, Hotspot: "金融券商", Code: "300475", Name: "香农芯创"},
		{Rank: 2, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞"},
	})
	if err != nil {
		t.Fatalf("marshal recommendations: %v", err)
	}
	summaryJSON, err := json.Marshal(aStockSnapshotNewsSummary{
		Articles: []model.Item{{
			ID:          1,
			SourceType:  "flash",
			Title:       "人工智能新闻",
			PublishTime: "2026-07-13 09:00:00",
		}},
		NewsArticles: []model.Item{{
			ID:          1,
			SourceType:  "flash",
			Title:       "人工智能新闻",
			PublishTime: "2026-07-13 09:00:00",
		}},
		Hotspots: []aStockHotspot{{Name: "人工智能", Score: 1, Evidence: 1}},
	})
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/articles" {
			t.Fatalf("expected snapshot news_summary_json to avoid articles API, got %s", r.URL.String())
		}
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
			Found:                 true,
			StrategyDate:          "2026-07-13",
			Period:                r.URL.Query().Get("period"),
			RecommendationsJSON:   string(recommendationsJSON),
			BacktestsJSON:         "[]",
			NewsSummaryJSON:       string(summaryJSON),
			BacktestStatus:        "已锁定推荐股票，已回测 0/2",
			FundFlowFilterEnabled: true,
		})
	}))
	defer content.Close()

	dividendHits := 0
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dividendHits++
		if r.URL.Path != "/api/a-stock/dividend-events" {
			t.Fatalf("unexpected akshare path: %s", r.URL.String())
		}
		writeEnvelope(w, http.StatusOK, "ok", aStockDividendEventResult{
			Items: []aStockDividendEvent{
				{Code: "002230", Name: "科大讯飞", ExDate: "2026-07-13", Description: "10派1元(含税)"},
			},
			Date: "2026-07-13",
		})
	}))
	defer akshare.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, AStockAuctionURL: akshare.URL})
	ctx, ok := srv.loadAStockReadOnlySnapshotContextWithCache("2026-07-13", "morning", 1, false, false, false, false, newAStockRequestCache())
	if !ok {
		t.Fatal("expected snapshot context")
	}
	if dividendHits != 0 {
		t.Fatalf("expected read-only snapshot to skip output filters, dividend hits=%d", dividendHits)
	}
	if len(ctx.Recommendations) != 2 {
		t.Fatalf("expected snapshot recommendations to remain unchanged, got %+v", ctx.Recommendations)
	}
	if ctx.Recommendations[0].Code != "300475" || ctx.Recommendations[1].Code != "002230" {
		t.Fatalf("expected original snapshot recommendation order, got %+v", ctx.Recommendations)
	}
	if ctx.ExDividendFiltered != 0 || ctx.SameDayMorningFiltered != 0 {
		t.Fatalf("expected no read-time output filtering counters, got ex=%d daily=%d", ctx.ExDividendFiltered, ctx.SameDayMorningFiltered)
	}
	if strings.Contains(ctx.BacktestStatus, "除权除息过滤") || strings.Contains(ctx.BacktestStatus, "日内推荐上限") {
		t.Fatalf("expected snapshot status without read-time filter suffix, got %q", ctx.BacktestStatus)
	}
}

func TestAStockOldSnapshotMissingNewsSummaryUsesArticleWindowCache(t *testing.T) {
	var mu sync.Mutex
	articleHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                 true,
				StrategyDate:          "2026-06-15",
				Period:                r.URL.Query().Get("period"),
				RecommendationsJSON:   "[]",
				BacktestsJSON:         "[]",
				BacktestStatus:        "无推荐股票",
				FundFlowFilterEnabled: true,
			})
		case "/api/v1/articles":
			mu.Lock()
			articleHits++
			mu.Unlock()
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          int64(articleHits),
					SourceType:  "flash",
					Title:       "AI 算力产业链活跃",
					Summary:     "人工智能热点",
					PublishTime: "2026-06-15 08:30:00",
					CapturedAt:  time.Date(2026, 6, 15, 0, 30, 0, 0, time.UTC),
				}},
				Page:     1,
				PageSize: 200,
				Total:    1,
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx, ok := srv.loadAStockReadOnlySnapshotContextWithCache("2026-06-15", "morning", 1, false, false, false, false, newAStockRequestCache())
	if !ok || ctx.NewsTotal == 0 {
		t.Fatalf("expected old snapshot to populate articles, ok=%t ctx=%+v", ok, ctx)
	}
	mu.Lock()
	firstHits := articleHits
	mu.Unlock()
	if firstHits == 0 {
		t.Fatal("expected first old snapshot request to hit articles API")
	}

	ctx, ok = srv.loadAStockReadOnlySnapshotContextWithCache("2026-06-15", "morning", 1, false, false, false, false, newAStockRequestCache())
	if !ok || ctx.NewsTotal == 0 {
		t.Fatalf("expected second old snapshot to use cached articles, ok=%t ctx=%+v", ok, ctx)
	}
	mu.Lock()
	secondHits := articleHits
	mu.Unlock()
	if secondHits != firstHits {
		t.Fatalf("expected second same-window request to hit service article cache, hits %d -> %d", firstHits, secondHits)
	}
}

func TestAStockEmptySnapshotNewsSummaryFallsBackToArticleWindow(t *testing.T) {
	emptySummary, err := json.Marshal(aStockSnapshotNewsSummary{
		Articles:     []model.Item{},
		NewsArticles: []model.Item{},
		Hotspots:     []aStockHotspot{},
	})
	if err != nil {
		t.Fatalf("marshal empty summary: %v", err)
	}
	articleHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                 true,
				StrategyDate:          "2026-06-15",
				Period:                r.URL.Query().Get("period"),
				RecommendationsJSON:   "[]",
				BacktestsJSON:         "[]",
				NewsSummaryJSON:       string(emptySummary),
				BacktestStatus:        "无推荐股票",
				LimitUpFilterEnabled:  true,
				FundFlowFilterEnabled: true,
			})
		case "/api/v1/articles":
			articleHits++
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          1,
					SourceType:  "sina_finance_7x24",
					Title:       "半导体板块活跃",
					Summary:     "芯片产业链活跃",
					PublishTime: "2026-06-15 09:35:00",
					CapturedAt:  time.Date(2026, 6, 15, 1, 35, 0, 0, time.UTC),
				}},
				Page:     1,
				PageSize: 200,
				Total:    1,
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx, ok := srv.loadAStockReadOnlySnapshotContextWithCache("2026-06-15", "afternoon", 1, false, false, false, false, newAStockRequestCache())
	if !ok {
		t.Fatal("expected snapshot context")
	}
	if articleHits == 0 || ctx.NewsTotal != 1 || ctx.RecommendationNewsTotal != 1 || len(ctx.Hotspots) == 0 {
		t.Fatalf("expected empty snapshot summary to fall back to article stats, hits=%d ctx=%+v", articleHits, ctx)
	}
}

func newAStockTradingDayServer(t *testing.T, isTradingDay bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/scheduler/a-stock/trading-day" {
			t.Fatalf("unexpected trading-day path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		date := r.URL.Query().Get("date")
		if date == "" {
			date = "2026-06-16"
		}
		reason := "trading_day"
		message := "open"
		if !isTradingDay {
			reason = "market_closed"
			message = "该日 A 股休市，不生成股票推荐。"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": map[string]any{
				"date":                 date,
				"is_trading_day":       isTradingDay,
				"latest_trading_day":   "2026-06-18",
				"previous_trading_day": "2026-06-18",
				"next_trading_day":     "2026-06-22",
				"source":               "test",
				"reason":               reason,
				"message":              message,
			},
		})
	}))
}

func TestAStockPageReusesTradingDayStatusWithinRequest(t *testing.T) {
	var mu sync.Mutex
	tradingDayCalls := 0
	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/scheduler/a-stock/trading-day" {
			t.Fatalf("unexpected trading-day path: %s", r.URL.String())
		}
		mu.Lock()
		tradingDayCalls++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"date":                 r.URL.Query().Get("date"),
				"is_trading_day":       true,
				"latest_trading_day":   "2026-06-16",
				"previous_trading_day": "2026-06-15",
				"next_trading_day":     "2026-06-17",
				"source":               "test",
				"reason":               "trading_day",
				"message":              "open",
			},
		})
	}))
	defer scheduler.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&period=morning&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	mu.Lock()
	got := tradingDayCalls
	mu.Unlock()
	if got != 1 {
		t.Fatalf("expected one trading-day request within page render, got %d", got)
	}
}

func setAStockEastmoneyKlineURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	previous := aStockEastmoneyKlineURL
	aStockEastmoneyKlineURL = rawURL
	t.Cleanup(func() {
		aStockEastmoneyKlineURL = previous
	})
}

func setAStockEastmoneyQuoteURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	previous := aStockEastmoneyQuoteURL
	aStockEastmoneyQuoteURL = rawURL
	t.Cleanup(func() {
		aStockEastmoneyQuoteURL = previous
	})
}

func setAStockTencentMinuteURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	previous := aStockTencentMinuteURL
	aStockTencentMinuteURL = rawURL
	t.Cleanup(func() {
		aStockTencentMinuteURL = previous
	})
}

func setAStockSinaMinuteURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	previous := aStockSinaMinuteURL
	aStockSinaMinuteURL = rawURL
	t.Cleanup(func() {
		aStockSinaMinuteURL = previous
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

func TestAStockWindowArticlesFallbackToCapturedAt(t *testing.T) {
	var seenPublishQuery bool
	var seenCapturedQuery bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		switch r.URL.Query().Get("time_field") {
		case "publish_time":
			seenPublishQuery = true
			if r.URL.Query().Get("start") != "2026-06-22 09:30:00" || r.URL.Query().Get("end") != "2026-06-22 13:00:59" {
				t.Fatalf("unexpected publish_time query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
			})
		case "captured_at":
			seenCapturedQuery = true
			if r.URL.Query().Get("start") != "2026-06-22T01:30:00Z" || r.URL.Query().Get("end") != "2026-06-22T05:00:59Z" {
				t.Fatalf("unexpected captured_at fallback query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{
					Items: []model.Item{{
						ID:         701,
						SourceType: "flash",
						Title:      "午间机器人板块活跃",
						Summary:    "机器人产业链升温",
						CapturedAt: time.Date(2026, 6, 22, 3, 15, 0, 0, time.UTC),
					}},
					Page: 1, PageSize: 200, Total: 1,
				},
			})
		default:
			t.Fatalf("unexpected article time_field: %s", r.URL.RawQuery)
		}
	}))
	defer content.Close()

	start, end := aStockWindow("2026-06-22", "afternoon")
	srv := NewServer(config.Config{ContentURL: content.URL})
	items, err := srv.loadAStockWindowArticles(start, end)
	if err != nil {
		t.Fatalf("loadAStockWindowArticles error: %v", err)
	}
	if !seenPublishQuery || !seenCapturedQuery {
		t.Fatalf("expected publish_time and captured_at queries, got publish=%v captured=%v", seenPublishQuery, seenCapturedQuery)
	}
	if len(items) != 1 || items[0].ID != 701 {
		t.Fatalf("expected captured_at fallback item, got %+v", items)
	}
}

func TestAStockWindowArticlesSupplementsMissingPublishSourceFromCapturedAt(t *testing.T) {
	var seenCapturedQuery bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		switch r.URL.Query().Get("time_field") {
		case "publish_time":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{
					Items: []model.Item{{
						ID:          801,
						SourceType:  "eastmoney_kuaixun",
						Title:       "东方财富窗口新闻",
						PublishTime: "2026-06-26 09:20:00",
						CapturedAt:  time.Date(2026, 6, 26, 1, 20, 0, 0, time.UTC),
					}},
					Page: 1, PageSize: 200, Total: 1,
				},
			})
		case "captured_at":
			seenCapturedQuery = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{
					Items: []model.Item{
						{
							ID:         802,
							SourceType: "headline",
							Title:      "金十资讯相对时间新闻",
							CapturedAt: time.Date(2026, 6, 26, 4, 8, 0, 0, time.UTC),
						},
						{
							ID:          803,
							SourceType:  "flash",
							Title:       "旧发布日期金十快讯",
							PublishTime: "2026-06-19 15:51:43",
							CapturedAt:  time.Date(2026, 6, 26, 4, 9, 0, 0, time.UTC),
						},
					},
					Page: 1, PageSize: 200, Total: 2,
				},
			})
		default:
			t.Fatalf("unexpected article time_field: %s", r.URL.RawQuery)
		}
	}))
	defer content.Close()

	start, end, _ := aStockRecommendationPhaseWindow("2026-06-26", "morning", aStockRecommendationPhaseFinal)
	srv := NewServer(config.Config{ContentURL: content.URL})
	items, err := srv.loadAStockWindowArticles(start, end)
	if err != nil {
		t.Fatalf("loadAStockWindowArticles error: %v", err)
	}
	if !seenCapturedQuery {
		t.Fatal("expected captured_at source supplement query")
	}
	gotIDs := make([]int64, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	if len(items) != 2 || items[0].ID != 801 || items[1].ID != 802 {
		t.Fatalf("expected publish item plus headline captured_at supplement, got ids=%v items=%+v", gotIDs, items)
	}
}

func TestAStockWindowArticlesFetchesAllPublishTimePages(t *testing.T) {
	firstPage := make([]model.Item, 0, aStockArticleFetchPageSize)
	for i := 0; i < aStockArticleFetchPageSize; i++ {
		firstPage = append(firstPage, model.Item{
			ID:          int64(9000 + i),
			SourceType:  "eastmoney_kuaixun",
			Title:       fmt.Sprintf("东方财富分页新闻%03d", i),
			PublishTime: "2026-06-26 09:00:00",
			CapturedAt:  time.Date(2026, 6, 26, 1, 0, i%60, 0, time.UTC),
		})
	}
	secondPage := []model.Item{
		{ID: 9301, SourceType: "flash", Title: "第二页金十快讯", PublishTime: "2026-06-26 09:10:00", CapturedAt: time.Date(2026, 6, 26, 1, 10, 0, 0, time.UTC)},
		{ID: 9302, SourceType: "headline", Title: "第二页金十资讯", PublishTime: "2026-06-26 09:11:00", CapturedAt: time.Date(2026, 6, 26, 1, 11, 0, 0, time.UTC)},
		{ID: 9303, SourceType: "jin10_full", Title: "第二页金十全站", PublishTime: "2026-06-26 09:12:00", CapturedAt: time.Date(2026, 6, 26, 1, 12, 0, 0, time.UTC)},
	}
	seenPublishPages := map[string]bool{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if got := r.URL.Query().Get("page_size"); got != fmt.Sprint(aStockArticleFetchPageSize) {
			t.Fatalf("expected article page_size=%d, got %q", aStockArticleFetchPageSize, got)
		}
		switch r.URL.Query().Get("time_field") {
		case "publish_time":
			page := r.URL.Query().Get("page")
			seenPublishPages[page] = true
			items := firstPage
			if page == "2" {
				items = secondPage
			}
			pageNumber, _ := strconv.Atoi(page)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{Items: items, Page: pageNumber, PageSize: aStockArticleFetchPageSize, Total: aStockArticleFetchPageSize + len(secondPage)},
			})
		case "captured_at":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: aStockArticleFetchPageSize, Total: 0},
			})
		default:
			t.Fatalf("unexpected article time_field: %s", r.URL.RawQuery)
		}
	}))
	defer content.Close()

	start, end := aStockWindow("2026-06-26", "morning")
	srv := NewServer(config.Config{ContentURL: content.URL})
	items, err := srv.loadAStockWindowArticles(start, end)
	if err != nil {
		t.Fatalf("loadAStockWindowArticles error: %v", err)
	}
	if !seenPublishPages["1"] || !seenPublishPages["2"] {
		t.Fatalf("expected publish_time pagination to fetch pages 1 and 2, got %+v", seenPublishPages)
	}
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.SourceType]++
	}
	if len(items) != aStockArticleFetchPageSize+len(secondPage) || counts["flash"] != 1 || counts["headline"] != 1 || counts["jin10_full"] != 1 {
		t.Fatalf("expected all paged items including three Jin10 sources, total=%d counts=%+v", len(items), counts)
	}
}

func TestAStockContextUsesLiteralNewsWindowForSourceStats(t *testing.T) {
	var seenRecommendationWindow bool
	var seenStatsWindow bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0},
			})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected article time_field: %s", r.URL.RawQuery)
		}
		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")
		switch {
		case start == "2026-06-26 08:00:00" && end == "2026-06-26 09:26:59":
			seenRecommendationWindow = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{
					Items: []model.Item{{
						ID:          9401,
						SourceType:  "headline",
						Title:       "策略窗口金十资讯",
						PublishTime: "2026-06-26 09:20:00",
						CapturedAt:  time.Date(2026, 6, 26, 1, 20, 0, 0, time.UTC),
					}},
					Page: 1, PageSize: 200, Total: 1,
				},
			})
		case start == "2026-06-26 08:00:00" && end == "2026-06-26 09:30:59":
			seenStatsWindow = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.ItemListResult{
					Items: []model.Item{
						{
							ID:          9402,
							SourceType:  "headline",
							Title:       "统计窗口金十资讯",
							PublishTime: "2026-06-26 09:20:00",
							CapturedAt:  time.Date(2026, 6, 26, 1, 20, 0, 0, time.UTC),
						},
						{
							ID:          9403,
							SourceType:  "flash",
							Title:       "09:28 金十快讯",
							PublishTime: "2026-06-26 09:28:00",
							CapturedAt:  time.Date(2026, 6, 26, 1, 28, 0, 0, time.UTC),
						},
					},
					Page: 1, PageSize: 200, Total: 2,
				},
			})
		default:
			t.Fatalf("unexpected publish_time window query: %s", r.URL.RawQuery)
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithRecommendationPhase("2026-06-26", "morning", 1, false, false, false, false, true, aStockRecommendationPhaseFinal, newAStockRequestCache())
	if seenRecommendationWindow || !seenStatsWindow {
		t.Fatalf("expected stats window to cover recommendation window without duplicate query, recommendation=%v stats=%v", seenRecommendationWindow, seenStatsWindow)
	}
	if ctx.WindowLabel != "08:00-09:30" || ctx.RecommendationWindowLabel != "08:00-09:26:59" {
		t.Fatalf("unexpected window labels: stats=%q recommendation=%q", ctx.WindowLabel, ctx.RecommendationWindowLabel)
	}
	if len(ctx.Articles) != 1 || len(ctx.NewsArticles) != 2 || ctx.RecommendationNewsTotal != 1 || ctx.NewsTotal != 2 {
		t.Fatalf("expected recommendation and source stats article sets to stay separate, ctx=%+v", ctx)
	}
}

func TestAStockPageOmitsNewsSourceStatsSection(t *testing.T) {
	items := make([]model.Item, 0, 12)
	for i := 1; i <= 12; i++ {
		sourceType := "flash"
		if i > 7 {
			sourceType = "eastmoney_kuaixun"
		}
		items = append(items, model.Item{
			ID:         int64(700 + i),
			SourceType: sourceType,
			Title:      fmt.Sprintf("分页新闻%02d", i),
			Summary:    "普通财经新闻",
			CapturedAt: time.Date(2026, 6, 11, 1, 26+i, 0, 0, time.UTC),
		})
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 afternoon window query: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.Query().Get("start"), "09:30") {
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
			return
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
	firstReq := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-11&period=afternoon&refresh_recommendations=1", nil)
	firstRR := httptest.NewRecorder()
	srv.handleAStockPage(firstRR, firstReq, map[string]any{"id": 1})
	if firstRR.Code != http.StatusOK {
		t.Fatalf("expected first page 200, got %d", firstRR.Code)
	}
	firstBody := firstRR.Body.String()
	for _, want := range []string{`财经新闻数</span><strong>12</strong>`} {
		if !strings.Contains(firstBody, want) {
			t.Fatalf("expected A股 overview to contain %q, got %s", want, firstBody)
		}
	}
	for _, notWant := range []string{"财经新闻来源统计", "08:00-09:30 财经新闻", "09:30-13:00 财经新闻", "新闻条数", "最近抓取", "08:00-09:30 0 / 09:30-13:00 12", "08:00-09:26:59 0 / 09:30-13:00 12", "astock-overview-meta"} {
		if strings.Contains(firstBody, notWant) {
			t.Fatalf("expected A股 page source stats to be absent, got %q in %s", notWant, firstBody)
		}
	}
	for _, notWant := range []string{"分页新闻01", "分页新闻10", "分页新闻11", "分页新闻12", "新闻分页：", `news_page=2`} {
		if strings.Contains(firstBody, notWant) {
			t.Fatalf("expected news summary not to contain detail %q, got %s", notWant, firstBody)
		}
	}
}

func TestSystemNewsStatsSectionRendersAStockSourceStats(t *testing.T) {
	t.Setenv("YUQING_A_STOCK_NEWS_SOURCE_CONFIG", filepath.Join(t.TempDir(), "sources.json"))
	items := make([]model.Item, 0, 12)
	for i := 1; i <= 12; i++ {
		sourceType := "flash"
		if i > 7 {
			sourceType = "eastmoney_kuaixun"
		}
		items = append(items, model.Item{
			ID:         int64(700 + i),
			SourceType: sourceType,
			Title:      fmt.Sprintf("分页新闻%02d", i),
			Summary:    "普通财经新闻",
			CapturedAt: time.Date(2026, 6, 11, 1, 26+i, 0, 0, time.UTC),
		})
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/articles" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": []any{}})
			return
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 news window query: %s", r.URL.RawQuery)
		}
		resultItems := []model.Item{}
		total := 0
		if strings.Contains(r.URL.Query().Get("start"), "09:30") {
			resultItems = items
			total = len(items)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items:    resultItems,
				Page:     1,
				PageSize: 200,
				Total:    total,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/system?section=newsstats&strategy_date=2026-06-11", nil)
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected system page 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{`href="/system?section=newsstats">新闻统计</a>`, `name="section" value="newsstats"`, `value="2026-06-11"`, "财经新闻来源统计", "08:00-09:30 财经新闻", "09:30-13:00 财经新闻", "来源", "原始地址", "新闻条数", "最近抓取", "开关", "推荐", "金十快讯", "7条", "东方财富快讯", "5条", "东方财富全站", "财联社", "0条", "暂无数据", `href="https://kuaixun.eastmoney.com/"`, `name="form_type" value="astock_news_source_setting"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected news stats section to contain %q, got %s", want, body)
		}
	}
	morningNewsIndex := strings.Index(body, "08:00-09:30 财经新闻")
	afternoonNewsIndex := strings.Index(body, "09:30-13:00 财经新闻")
	if morningNewsIndex < 0 || afternoonNewsIndex < 0 || morningNewsIndex > afternoonNewsIndex {
		t.Fatalf("expected morning source table before afternoon source table, got %s", body)
	}
	if strings.Contains(body, `<th>说明</th>`) {
		t.Fatalf("expected news stats to keep explanation in tooltip, got %s", body)
	}
}

func TestSystemNewsStatsSourceSettingPostPersistsConfig(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "sources.json")
	t.Setenv("YUQING_A_STOCK_NEWS_SOURCE_CONFIG", settingsPath)

	srv := NewServer(config.Config{})
	form := url.Values{}
	form.Set("form_type", "astock_news_source_setting")
	form.Set("section", "newsstats")
	form.Set("strategy_date", "2026-06-11")
	form.Set("source_type", "eastmoney_kuaixun")
	form.Set("setting", "recommendation")
	form.Set("enabled", "0")
	req := httptest.NewRequest(http.MethodPost, "/system?section=newsstats", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after source setting save, got %d", rr.Code)
	}
	if location := rr.Header().Get("Location"); !strings.Contains(location, "section=newsstats") || !strings.Contains(location, "strategy_date=2026-06-11") {
		t.Fatalf("unexpected redirect location: %s", location)
	}
	settings, err := astocknews.LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if settings["eastmoney_kuaixun"].RecommendationEnabled {
		t.Fatalf("expected eastmoney recommendation to be disabled, got %+v", settings["eastmoney_kuaixun"])
	}
}

func TestAStockNewsSectionShowsSourceRunDiagnostics(t *testing.T) {
	t.Setenv("YUQING_A_STOCK_NEWS_SOURCE_CONFIG", filepath.Join(t.TempDir(), "sources.json"))
	setAStockNowForTest(t, time.Date(2026, 6, 22, 12, 20, 0, 0, aStockLocation()))
	ctx := aStockContext{
		Date:            "2026-06-22",
		WindowLabel:     "09:30-13:00",
		NewsWindowStart: time.Date(2026, 6, 22, 9, 30, 0, 0, aStockLocation()),
		NewsWindowEnd:   time.Date(2026, 6, 22, 13, 0, 59, 0, aStockLocation()),
		Articles: []model.Item{
			{SourceType: "flash", Title: "金十新闻"},
		},
		SourceRuns: []aStockSourceRun{
			{SourceType: "flash", Status: "success", FetchedCount: 18, InsertedCount: 18, StartedAt: time.Date(2026, 6, 22, 5, 1, 0, 0, time.UTC)},
			{SourceType: "eastmoney_kuaixun", Status: "success", FetchedCount: 2600, InsertedCount: 20, UpdatedCount: 2300, StartedAt: time.Date(2026, 6, 22, 1, 33, 0, 0, time.UTC)},
			{SourceType: "sina_finance_7x24", Status: "failed", ErrorText: "upstream timeout", StartedAt: time.Date(2026, 6, 22, 5, 2, 0, 0, time.UTC)},
		},
	}
	var b strings.Builder
	renderAStockNewsWindow(&b, ctx)
	body := b.String()
	for _, want := range []string{"金十快讯", "1条", "success", "18/18/0", "新浪财经", "failed", `title="upstream timeout"`, `role="tooltip">upstream timeout`, "东方财富快讯", "0条", "东方财富全站", "最近抓取早于统计截止，可能未覆盖后续新闻", "源站抓取数 / 入库新增数 / 更新数", "原始地址", "开关", "推荐"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 news diagnostics to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, `<th>说明</th>`) {
		t.Fatalf("expected A股 news diagnostics to move explanation into tooltip, got %s", body)
	}
}

func TestAStockNewsSectionFutureWindowDoesNotShowStaleDiagnostic(t *testing.T) {
	t.Setenv("YUQING_A_STOCK_NEWS_SOURCE_CONFIG", filepath.Join(t.TempDir(), "sources.json"))
	setAStockNowForTest(t, time.Date(2026, 7, 29, 9, 13, 0, 0, aStockLocation()))
	ctx := aStockContext{
		Date:            "2026-07-29",
		WindowLabel:     "09:30-13:00",
		NewsWindowStart: time.Date(2026, 7, 29, 9, 30, 0, 0, aStockLocation()),
		NewsWindowEnd:   time.Date(2026, 7, 29, 13, 0, 59, 0, aStockLocation()),
		SourceRuns: []aStockSourceRun{
			{SourceType: "flash", Status: "success", FetchedCount: 18, InsertedCount: 1, UpdatedCount: 15, StartedAt: time.Date(2026, 7, 29, 1, 9, 0, 0, time.UTC)},
		},
	}
	var b strings.Builder
	renderAStockNewsWindow(&b, ctx)
	body := b.String()
	if !strings.Contains(body, "窗口尚未开始") {
		t.Fatalf("expected future window empty state, got %s", body)
	}
	if strings.Contains(body, "最近抓取早于统计截止") {
		t.Fatalf("future window should not show stale coverage diagnostic, got %s", body)
	}
	if !strings.Contains(body, "18/1/15") {
		t.Fatalf("expected latest crawl counts to remain visible, got %s", body)
	}
}

func TestLoadAStockSourceRunsShowsUnavailableWhenCrawlerURLMissing(t *testing.T) {
	srv := NewServer(config.Config{})
	runs := srv.loadAStockSourceRuns()
	if len(runs) == 0 {
		t.Fatal("expected unavailable source runs")
	}
	for _, run := range runs {
		if run.SourceType == "" || run.Status != "unavailable" || !strings.Contains(run.ErrorText, "抓取状态接口未配置") {
			t.Fatalf("unexpected unavailable run: %+v", run)
		}
	}
}

func TestLoadAStockSourceRunsBackfillsMissingSourcesBySourceType(t *testing.T) {
	var sourceSpecificRequests int
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/tasks/crawl/runs" {
			t.Fatalf("unexpected crawler path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		sourceType := r.URL.Query().Get("source_type")
		if sourceType == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": []model.CrawlRun{{
					SourceType:    "flash",
					Status:        "success",
					FetchedCount:  23,
					InsertedCount: 23,
					StartedAt:     time.Date(2026, 6, 23, 1, 16, 0, 0, time.UTC),
				}},
			})
			return
		}
		sourceSpecificRequests++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": []model.CrawlRun{{
				SourceType:    sourceType,
				Status:        "success",
				FetchedCount:  5,
				InsertedCount: 4,
				UpdatedCount:  1,
				StartedAt:     time.Date(2026, 6, 23, 1, 25, 0, 0, time.UTC),
			}},
		})
	}))
	defer crawler.Close()

	srv := NewServer(config.Config{CrawlerURL: crawler.URL})
	runs := srv.loadAStockSourceRuns()
	if sourceSpecificRequests == 0 {
		t.Fatal("expected source-specific fallback requests")
	}
	var eastmoney aStockSourceRun
	for _, run := range runs {
		if run.SourceType == "eastmoney_kuaixun" {
			eastmoney = run
			break
		}
	}
	if eastmoney.Status != "success" || eastmoney.FetchedCount != 5 || eastmoney.InsertedCount != 4 || eastmoney.UpdatedCount != 1 {
		t.Fatalf("expected source-specific eastmoney run, got %+v", eastmoney)
	}
}

func handleAStockRecommendationSnapshotTestEndpoint(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/a-stock/recommendations":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationSnapshot{Found: false},
		})
		return true
	case "/api/v1/a-stock/recommendation-selections":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationSelectionListResult{Found: false},
		})
		return true
	case "/api/v1/a-stock/recommendation-latest-dates":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}},
		})
		return true
	case "/api/v1/a-stock/recommendation-performance":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": model.AStockRecommendationPerformanceSummary{
				Strategy:    r.URL.Query().Get("strategy"),
				StartDate:   r.URL.Query().Get("start"),
				EndDate:     r.URL.Query().Get("end"),
				Period:      r.URL.Query().Get("period"),
				SampleCount: 0,
			},
		})
		return true
	case "/api/v1/internal/a-stock/recommendations":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationSnapshotUpsertResult{Inserted: 1},
		})
		return true
	case "/api/v1/internal/a-stock/recommendation-shadow-snapshots":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationSnapshotUpsertResult{Inserted: 1},
		})
		return true
	case "/api/v1/internal/a-stock/recommendation-selections":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationSelectionUpsertResult{Inserted: 1, Total: 1},
		})
		return true
	default:
		return false
	}
}

func handleEmptyAStockAuctionTestEndpoint(w http.ResponseWriter, r *http.Request) bool {
	if handleAStockAlgorithmSettingsTestEndpoint(w, r) {
		return true
	}
	if r.URL.Path == "/api/v1/a-stock/recommendation-performance" {
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationPerformanceSummary{
			Strategy:  r.URL.Query().Get("strategy"),
			StartDate: r.URL.Query().Get("start"),
			EndDate:   r.URL.Query().Get("end"),
			Period:    r.URL.Query().Get("period"),
		})
		return true
	}
	if r.URL.Path == "/api/v1/a-stock/recommendation-latest-dates" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}},
		})
		return true
	}
	if r.URL.Path == "/api/v1/a-stock/stock-fund-flow-trend" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": model.AStockStockFundFlowTrendResult{
				Items: []model.AStockStockFundFlow{{
					Code:          r.URL.Query().Get("code"),
					TradeDate:     r.URL.Query().Get("end_date"),
					Indicator:     r.URL.Query().Get("indicator"),
					MainNetInflow: 0,
				}},
				Total:     0,
				EndDate:   r.URL.Query().Get("end_date"),
				Indicator: r.URL.Query().Get("indicator"),
				Days:      parseIntDefault(r.URL.Query().Get("days"), 0),
			},
		})
		return true
	}
	if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
		writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
		return true
	}
	if r.URL.Path == "/api/v1/a-stock/sector-fund-flow-trend" {
		writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowTrendResult{})
		return true
	}
	if r.URL.Path == "/api/v1/a-stock/auction" {
		writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{})
		return true
	}
	return false
}

func handleAStockAlgorithmSettingsTestEndpoint(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/api/v1/system/a-stock-recommendation-algorithm" {
		writeEnvelope(w, http.StatusOK, "ok", model.DefaultAStockRecommendationAlgorithmSettings())
		return true
	}
	return false
}

func mustAStockTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal A股测试数据: %v", err)
	}
	return string(raw)
}

func aStockCookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie != nil && cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func writeAStockTestMarketBars(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	codes := strings.Split(r.URL.Query().Get("codes"), ",")
	items := make([]map[string]any, 0, len(codes))
	for _, rawCode := range codes {
		code := normalizeAStockCode(rawCode)
		if code == "" {
			continue
		}
		items = append(items, map[string]any{
			"code":                  code,
			"date":                  r.URL.Query().Get("date"),
			"open":                  10.00,
			"close":                 10.50,
			"pct":                   5.00,
			"entry_price":           10.00,
			"afternoon_entry_price": 10.20,
		})
	}
	writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": items})
}

func writeAStockSnapshotOverviewArticleFixture(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	items := []model.Item{}
	switch {
	case strings.Contains(start, "08:00"):
		items = []model.Item{
			{ID: 1, SourceType: "jin10_kuaixun", Title: "人工智能产业链活跃", Summary: "AI 算力需求增长", PublishTime: "2026-06-24 09:05:00"},
			{ID: 2, SourceType: "eastmoney_kuaixun", Title: "算力板块继续走强", Summary: "人工智能投资升温", PublishTime: "2026-06-24 09:12:00"},
		}
	case strings.Contains(start, "09:30"):
		items = []model.Item{
			{ID: 3, SourceType: "cls_telegraph", Title: "人工智能应用端活跃", Summary: "AI 终端放量", PublishTime: "2026-06-24 10:30:00"},
		}
	}
	writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: items, Page: 1, PageSize: 200, Total: len(items)})
}

func mustAStockSnapshotNewsSummaryJSON(t *testing.T, articles []model.Item, newsArticles []model.Item, hotspots []aStockHotspot) string {
	t.Helper()
	return mustAStockTestJSON(t, aStockSnapshotNewsSummary{
		Articles:     articles,
		NewsArticles: newsArticles,
		Hotspots:     hotspots,
	})
}

func newAStockRefreshBacktestPostRequest(strategyDate string, period string) *http.Request {
	form := url.Values{}
	form.Set("date", strategyDate)
	form.Set("period", period)
	form.Set("action", "refresh_backtest")
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func newAStockRefreshCurrentBacktestPostRequest(strategyDate string, period string) *http.Request {
	form := url.Values{}
	form.Set("date", strategyDate)
	form.Set("period", period)
	form.Set("action", "refresh_current_backtest")
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestAStockPageUsesValidSnapshotsBeforeSelections(t *testing.T) {
	marketHits := 0
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marketHits++
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": []map[string]any{}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	morningArticles := []model.Item{
		{ID: 1, SourceType: "jin10_kuaixun", Title: "人工智能产业链活跃", Summary: "AI 算力需求增长", PublishTime: "2026-06-24 09:05:00"},
		{ID: 2, SourceType: "eastmoney_kuaixun", Title: "算力板块继续走强", Summary: "人工智能投资升温", PublishTime: "2026-06-24 09:12:00"},
	}
	afternoonArticles := []model.Item{
		{ID: 3, SourceType: "cls_telegraph", Title: "人工智能应用端活跃", Summary: "AI 终端放量", PublishTime: "2026-06-24 10:30:00"},
	}
	morningHotspots := []aStockHotspot{{Name: "人工智能", Keywords: []string{"人工智能"}, Score: 26, Evidence: 2, MatchedItems: morningArticles, TopStocks: []aStockHotspotStock{{Rank: 1, Code: "600001", Name: "快照上午"}}}}
	afternoonHotspots := []aStockHotspot{{Name: "人工智能", Keywords: []string{"人工智能"}, Score: 13, Evidence: 1, MatchedItems: afternoonArticles, TopStocks: []aStockHotspotStock{{Rank: 1, Code: "600002", Name: "快照下午"}}}}
	morningSnapshot := model.AStockRecommendationSnapshot{
		Found:               true,
		StrategyDate:        "2026-06-24",
		Period:              "morning",
		RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "快照热点", Code: "600001", Name: "快照上午", Reason: "snapshot morning"}}),
		BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{{
			Stock:           "600001 快照上午",
			EntryOpen:       "10.00",
			AfternoonOpen:   "--",
			T0Return:        "+2.00%",
			T0Close:         "10.20",
			T0ReturnClass:   "astock-up",
			Days:            []aStockBacktestCell{{Close: "10.50", Return: "+5.00%", ReturnClass: "astock-up"}},
			BestReturn:      "+5.00%",
			BestReturnClass: "astock-up",
			Status:          "已回测T+1",
		}}),
		NewsSummaryJSON:       mustAStockSnapshotNewsSummaryJSON(t, morningArticles, morningArticles, morningHotspots),
		BacktestStatus:        "已读取上午快照",
		GeneratedCount:        1,
		FundFlowFilterEnabled: true,
	}
	afternoonSnapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-06-24",
		Period:                "afternoon",
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "快照热点", Code: "600002", Name: "快照下午", Reason: "snapshot afternoon"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600002 快照下午", EntryOpen: "--", AfternoonOpen: "20.00", T0Return: "+1.00%", T0Close: "20.20", T0ReturnClass: "astock-up", Days: []aStockBacktestCell{{Close: "20.60", Return: "+3.00%", ReturnClass: "astock-up"}}, BestReturn: "+3.00%", BestReturnClass: "astock-up", Status: "已回测T+1"}}),
		NewsSummaryJSON:       mustAStockSnapshotNewsSummaryJSON(t, afternoonArticles, afternoonArticles, afternoonHotspots),
		BacktestStatus:        "已读取下午快照",
		GeneratedCount:        1,
		LimitUpFilterEnabled:  true,
		FundFlowFilterEnabled: true,
	}
	selectionHits := 0
	holdingHits := 0
	saveHits := 0
	articleHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			articleHits++
			writeAStockSnapshotOverviewArticleFixture(w, r)
		case "/api/v1/a-stock/recommendations":
			switch r.URL.Query().Get("period") {
			case "morning":
				writeEnvelope(w, http.StatusOK, "ok", morningSnapshot)
			case "afternoon":
				writeEnvelope(w, http.StatusOK, "ok", afternoonSnapshot)
			case "evening":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
			default:
				t.Fatalf("unexpected snapshot period: %s", r.URL.RawQuery)
			}
		case "/api/v1/a-stock/recommendation-selections":
			selectionHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found: true,
				Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600099", Name: "已选股票", Hotspot: "旧", Reason: "selection"}},
			})
		case "/api/v1/a-stock/holdings/summary":
			holdingHits++
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{HolderCount: 1, HolderTypeCount: 1, TotalFloatRatio: 1})
		case "/api/v1/internal/a-stock/recommendations":
			saveHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-24&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"快照上午",
		"快照下午",
		`<span class="astock-muted">财经新闻数</span><strong>2</strong>`,
		`<span class="astock-muted">财经新闻数</span><strong>1</strong>`,
		`<span class="astock-muted">候选热点数</span><strong>1</strong>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected snapshot response to contain %q, got %s", want, body)
		}
	}
	backtestReq := httptest.NewRequest(http.MethodGet, "/a-stock/backtest?date=2026-06-24&period=morning", nil)
	backtestRR := httptest.NewRecorder()
	srv.handleAStockBacktestPage(backtestRR, backtestReq, map[string]any{"id": 1})
	if backtestRR.Code != http.StatusOK {
		t.Fatalf("expected backtest 200, got %d body=%s", backtestRR.Code, backtestRR.Body.String())
	}
	backtestBody := backtestRR.Body.String()
	for _, want := range []string{"快照上午", "快照下午", "10.00", "20.00"} {
		if !strings.Contains(backtestBody, want) {
			t.Fatalf("expected snapshot backtest response to contain %q, got %s", want, backtestBody)
		}
	}
	if marketHits != 0 || selectionHits != 0 || holdingHits != 0 || saveHits != 0 {
		t.Fatalf("expected snapshot fast path to avoid market/selection/holdings/save, got market=%d selection=%d holdings=%d save=%d", marketHits, selectionHits, holdingHits, saveHits)
	}
	if articleHits != 0 {
		t.Fatalf("expected snapshot fast path to use snapshot news summary, got article requests=%d", articleHits)
	}
}

func TestAStockPageCompanionSnapshotMissingDoesNotRecompute(t *testing.T) {
	marketHits := 0
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marketHits++
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": []map[string]any{}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	morningArticles := []model.Item{{ID: 1, SourceType: "jin10_kuaixun", Title: "人工智能产业链活跃", Summary: "AI 算力需求增长", PublishTime: "2026-06-24 09:05:00"}}
	morningSnapshot := model.AStockRecommendationSnapshot{
		Found:                 true,
		StrategyDate:          "2026-06-24",
		Period:                "morning",
		RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "快照热点", Code: "600001", Name: "上午快照", Reason: "snapshot morning"}}),
		BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600001 上午快照", EntryOpen: "10.00", T0Return: "+1.00%", T0Close: "10.10", Status: "已读取快照"}}),
		NewsSummaryJSON:       mustAStockSnapshotNewsSummaryJSON(t, morningArticles, morningArticles, []aStockHotspot{{Name: "人工智能", Keywords: []string{"人工智能"}, Score: 13, Evidence: 1, MatchedItems: morningArticles}}),
		BacktestStatus:        "已读取上午快照",
		GeneratedCount:        1,
		FundFlowFilterEnabled: true,
	}
	articleHits := 0
	selectionHits := 0
	holdingHits := 0
	saveHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			articleHits++
			writeAStockSnapshotOverviewArticleFixture(w, r)
		case "/api/v1/a-stock/recommendations":
			if r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", morningSnapshot)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			selectionHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found: true,
				Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600020", Name: "下午已选", Hotspot: "伴随", MarketScore: 80, Reason: "selection"}},
			})
		case "/api/v1/a-stock/holdings/summary":
			holdingHits++
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations", "/api/v1/internal/a-stock/recommendation-selections":
			saveHits++
			writeEnvelope(w, http.StatusOK, "ok", map[string]any{"updated": 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-24&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "上午快照") {
		t.Fatalf("expected current snapshot to render, got %s", body)
	}
	if strings.Contains(body, "下午已选") {
		t.Fatalf("expected missing companion snapshot not to load selections, got %s", body)
	}
	if articleHits != 0 {
		t.Fatalf("expected current snapshot to use snapshot news summary, got article requests=%d", articleHits)
	}
	if marketHits != 0 || selectionHits != 0 || holdingHits != 0 || saveHits != 0 {
		t.Fatalf("expected missing companion snapshot to avoid recompute, got market=%d selection=%d holdings=%d save=%d", marketHits, selectionHits, holdingHits, saveHits)
	}
}

func TestAStockPageGetDoesNotPersistComputedRecommendations(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(writeAStockTestMarketBars))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	internalPosts := 0
	selectionPeriods := map[string]int{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			period := normalizeAStockPeriod(r.URL.Query().Get("period")).Key
			selectionPeriods[period]++
			code := "600010"
			name := "上午已选"
			if period == "afternoon" {
				code = "600020"
				name = "下午已选"
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-24",
				Period:       period,
				Items:        []model.AStockRecommendationSelection{{Rank: 1, Code: code, Name: name, Hotspot: "只读", MarketScore: 80, Reason: "selection"}},
			})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations", "/api/v1/internal/a-stock/recommendation-selections":
			internalPosts++
			writeEnvelope(w, http.StatusOK, "ok", map[string]any{"updated": 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-24&period=morning&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if internalPosts != 0 {
		t.Fatalf("GET /a-stock must not persist recommendations, got %d internal posts", internalPosts)
	}
	if selectionPeriods["morning"] == 0 || selectionPeriods["afternoon"] == 0 {
		t.Fatalf("expected both displayed periods to load selections without writes, got %+v", selectionPeriods)
	}
}

func TestAStockPageMorningGetDoesNotCreateAfternoonCompanionSnapshot(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(writeAStockTestMarketBars))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	afternoonSnapshotPosts := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendations":
			if r.Method == http.MethodPost {
				var snapshot model.AStockRecommendationSnapshot
				_ = json.NewDecoder(r.Body).Decode(&snapshot)
				if snapshot.Period == "afternoon" {
					afternoonSnapshotPosts++
				}
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
				return
			}
			if r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
					Found:               true,
					StrategyDate:        "2026-06-24",
					Period:              "morning",
					RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "快照", Code: "600001", Name: "上午快照", Reason: "snapshot"}}),
					BacktestsJSON:       mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600001 上午快照", EntryOpen: "10.00", T0Return: "+1.00%", T0Close: "10.10", Status: "已回测T+1"}}),
					BacktestStatus:      "已读取上午快照",
					GeneratedCount:      1,
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-24",
				Period:       "afternoon",
				Items:        []model.AStockRecommendationSelection{{Rank: 1, Code: "600020", Name: "下午已选", Hotspot: "伴随", MarketScore: 80, Reason: "selection"}},
			})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations":
			afternoonSnapshotPosts++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/internal/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-24&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if afternoonSnapshotPosts != 0 {
		t.Fatalf("morning GET must not create afternoon companion snapshot, got %d posts", afternoonSnapshotPosts)
	}
}

func TestAStockContextRefreshAllBacktestsBypassesValidSnapshot(t *testing.T) {
	marketHits := 0
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marketHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "600010", "date": "2026-06-23", "open": 10.00, "close": 11.00, "pct": 10.00, "entry_price": 10.00, "afternoon_entry_price": 10.50},
					{"code": "600010", "date": "2026-06-24", "open": 11.20, "close": 12.00, "pct": 9.09, "entry_price": 11.20, "afternoon_entry_price": 11.50},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	holdingHits := 0
	saveHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-23",
				Period:       "morning",
				Items:        []model.AStockRecommendationSelection{{Rank: 1, Code: "600010", Name: "刷新股票", Hotspot: "刷新", MarketScore: 80, Reason: "selection"}},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:               true,
				StrategyDate:        "2026-06-23",
				Period:              "morning",
				RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Code: "600099", Name: "旧快照", Reason: "snapshot"}}),
				BacktestsJSON:       mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600099 旧快照", EntryOpen: "9.00", T0Return: "+1.00%", T0Close: "9.09"}}),
				BacktestStatus:      "已读取旧快照",
				GeneratedCount:      1,
			})
		case "/api/v1/a-stock/holdings/summary":
			holdingHits++
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{HolderCount: 1, HolderTypeCount: 1, TotalFloatRatio: 2})
		case "/api/v1/internal/a-stock/recommendations":
			saveHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "morning", 1, false, false, true, false, true, newAStockRequestCache())
	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].Code != "600010" {
		t.Fatalf("expected refresh path to use selection, got %+v", ctx.Recommendations)
	}
	if marketHits == 0 || holdingHits == 0 || saveHits == 0 {
		t.Fatalf("expected refresh path to call market/holdings/save, got market=%d holdings=%d save=%d", marketHits, holdingHits, saveHits)
	}
}

func TestAStockContextFallsBackWhenSnapshotMissing(t *testing.T) {
	marketHits := 0
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marketHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "600011", "date": "2026-06-23", "open": 8.00, "close": 8.40, "pct": 5.00, "entry_price": 8.00, "afternoon_entry_price": 8.20},
					{"code": "600011", "date": "2026-06-24", "open": 8.50, "close": 8.80, "pct": 4.76, "entry_price": 8.50, "afternoon_entry_price": 8.60},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	saveHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found: true,
				Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600011", Name: "缺失快照股票", Hotspot: "缺失", MarketScore: 77, Reason: "selection"}},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations":
			saveHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Inserted: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "morning", 1, false, false, true, false, false, newAStockRequestCache())
	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].Code != "600011" {
		t.Fatalf("expected selection fallback recommendation, got %+v", ctx.Recommendations)
	}
	if marketHits == 0 || saveHits == 0 {
		t.Fatalf("expected snapshot miss to recompute and save, got market=%d save=%d", marketHits, saveHits)
	}
}

func TestAStockContextFallsBackWhenSnapshotStale(t *testing.T) {
	marketHits := 0
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marketHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "600012", "date": "2026-06-23", "open": 19.00, "close": 21.00, "pct": 10.53, "entry_price": 19.00, "afternoon_entry_price": 20.00},
					{"code": "600012", "date": "2026-06-24", "open": 21.50, "close": 22.00, "pct": 4.76, "entry_price": 21.50, "afternoon_entry_price": 21.60},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	saveHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("period") != "afternoon" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found: true,
				Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600012", Name: "过期快照股票", Hotspot: "过期", MarketScore: 78, Reason: "selection"}},
			})
		case "/api/v1/a-stock/recommendations":
			if r.URL.Query().Get("period") != "afternoon" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                true,
				StrategyDate:         "2026-06-23",
				Period:               "afternoon",
				RecommendationsJSON:  mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Code: "600012", Name: "过期快照股票", Reason: "snapshot"}}),
				BacktestsJSON:        mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600012 过期快照股票", EntryOpen: "19.00", AfternoonOpen: "--", T0Return: "+10.53%", T0Close: "21.00"}}),
				BacktestStatus:       "等待下午开盘价",
				GeneratedCount:       1,
				LimitUpFilterEnabled: true,
			})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations":
			saveHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "afternoon", 1, false, false, true, false, false, newAStockRequestCache())
	if len(ctx.Backtests) != 1 || ctx.Backtests[0].AfternoonOpen != "20.00" || ctx.Backtests[0].T0Return != "+5.00%" {
		t.Fatalf("expected stale snapshot to recompute afternoon backtest, got %+v", ctx.Backtests)
	}
	if marketHits == 0 || saveHits == 0 {
		t.Fatalf("expected stale snapshot to call market and save, got market=%d save=%d", marketHits, saveHits)
	}
}

func TestAStockContextLoadsPersistedRecommendationSnapshot(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{
				Items: []model.Item{{ID: 1, SourceType: "flash", Title: "科大讯飞午间活跃", Summary: "AI 人工智能算力需求增长", PublishTime: "2026-06-22 12:05:00", TagFlags: "0.002230"}},
				Page:  1, PageSize: 200, Total: 1,
			}})
		case "/api/v1/a-stock/recommendation-selections":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.AStockRecommendationSelectionListResult{Found: false}})
		case "/api/v1/a-stock/recommendation-latest-dates":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}}})
		case "/api/v1/a-stock/recommendations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.AStockRecommendationSnapshot{
					Found:                 true,
					StrategyDate:          "2026-06-22",
					Period:                "afternoon",
					RecommendationsJSON:   `[{"Rank":1,"Hotspot":"人工智能","Code":"002230","Name":"科大讯飞","Reason":"snapshot"}]`,
					BacktestsJSON:         `[]`,
					BacktestStatus:        "已读取推荐快照",
					GeneratedCount:        1,
					LimitUpFilterEnabled:  true,
					FundFlowFilterEnabled: true,
				},
			})
		case "/api/v1/a-stock/auction":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.AStockAuctionListResult{
					Date: r.URL.Query().Get("date"),
					Items: []model.AStockAuctionAmount{{
						TradeDate:     r.URL.Query().Get("date"),
						Code:          "002230",
						Name:          "科大讯飞",
						AuctionVolume: 1000000,
						AuctionAmount: 10000000,
						Status:        "ok",
					}},
				},
			})
		case "/api/v1/internal/a-stock/recommendation-selections":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.AStockRecommendationSelectionUpsertResult{Inserted: 1, Total: 1},
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	scheduler := newAStockTradingDayServer(t, true)
	defer scheduler.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	ctx := srv.loadAStockContext("2026-06-22", "afternoon", 1, false)
	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].Code != "002230" || ctx.BacktestStatus != "已读取推荐快照" {
		t.Fatalf("expected persisted recommendation snapshot, got recs=%+v status=%q", ctx.Recommendations, ctx.BacktestStatus)
	}
	if len(ctx.Hotspots) == 0 || len(ctx.Hotspots[0].TopStocks) == 0 || ctx.Hotspots[0].TopStocks[0].Code != "002230" {
		t.Fatalf("expected hotspot top stocks to be populated before snapshot return, got %+v", ctx.Hotspots)
	}
	if !ctx.LimitUpFilterEnabled {
		t.Fatalf("expected persisted afternoon snapshot to preserve limit-up filter state")
	}
}

func TestAStockCurrentRecommendationSnapshotUsesRealtimeMarketPct(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 21, 11, 30, 0, 0, time.FixedZone("CST", 8*3600)))

	previousQuoteURL := aStockEastmoneyQuoteURL
	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("secid") != "1.601318" {
			t.Fatalf("unexpected realtime quote query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"f43":  5253,
				"f46":  5368,
				"f57":  "601318",
				"f60":  5323,
				"f170": -132,
			},
		})
	}))
	defer quote.Close()
	aStockEastmoneyQuoteURL = quote.URL
	t.Cleanup(func() {
		aStockEastmoneyQuoteURL = previousQuoteURL
	})

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/system/a-stock-recommendation-algorithm" {
			writeEnvelope(w, http.StatusOK, "ok", model.DefaultAStockRecommendationAlgorithmSettings())
			return
		}
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
			Found:               true,
			StrategyDate:        "2026-07-21",
			Period:              "morning",
			RecommendationsJSON: `[{"Rank":1,"Hotspot":"半导体","Code":"601318","Name":"中国平安","CurrentPrice":"53.63","TodayPct":"+75.00%","TodayPctClass":"astock-up","Reason":"snapshot"}]`,
			BacktestsJSON:       `[{"Stock":"601318 中国平安","EntryOpen":"53.68","T0Close":"53.63","T0Return":"-0.09%","T0ReturnClass":"astock-down","CurrentPrice":"53.63","CurrentReturn":"-0.09%","CurrentReturnClass":"astock-down","CurrentMarketPct":"+75.00%","CurrentMarketPctClass":"astock-up","Status":"等待T+1行情"}]`,
			BacktestStatus:      "已读取推荐快照",
			GeneratedCount:      1,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := aStockContext{Date: "2026-07-21", Period: "morning"}
	if !srv.applyAStockBacktestSnapshotOnlyWithCache(&ctx, newAStockRequestCache()) {
		t.Fatal("expected current recommendation snapshot to load")
	}
	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].CurrentPrice != "52.53" || ctx.Recommendations[0].TodayPct != "-1.32%" || ctx.Recommendations[0].TodayPctClass != "astock-down" {
		t.Fatalf("expected recommendation display to use realtime market pct, got %+v", ctx.Recommendations)
	}
	if len(ctx.Backtests) != 1 || ctx.Backtests[0].CurrentPrice != "52.53" || ctx.Backtests[0].CurrentMarketPct != "-1.32%" || ctx.Backtests[0].CurrentMarketPctClass != "astock-down" {
		t.Fatalf("expected backtest display to use realtime market pct, got %+v", ctx.Backtests)
	}
}

func TestAStockContextRefreshKeepsPersistedRecommendationSelections(t *testing.T) {
	t.Setenv("YUQING_ASTOCK_MARKET_URL", "")
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"code": "002008", "date": "2026-06-20", "close": 120.0, "pct": 1.2},
				{"code": "002008", "date": "2026-06-23", "close": 135.54, "pct": 7.57, "afternoon_entry_price": 126.00},
				{"code": "688367", "date": "2026-06-20", "close": 48.10, "pct": 0.8},
				{"code": "688367", "date": "2026-06-23", "close": 50.87, "pct": 4.93, "afternoon_entry_price": 49.10},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	afternoonSnapshotGets := 0
	selectionGets := 0
	selectionPosts := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
		case "/api/v1/a-stock/recommendation-selections":
			selectionGets++
			if r.URL.Query().Get("period") != "afternoon" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-23",
				Period:       "afternoon",
				Items: []model.AStockRecommendationSelection{
					{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 91, Reason: "locked-1"},
					{Rank: 2, Code: "688367", Name: "工大高科", Hotspot: "机器人", MarketScore: 87, Reason: "locked-2"},
				},
				UpdatedAt: time.Date(2026, 6, 23, 5, 5, 0, 0, time.UTC),
			})
		case "/api/v1/a-stock/recommendations":
			if r.URL.Query().Get("period") == "afternoon" {
				afternoonSnapshotGets++
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Inserted: 1})
		case "/api/v1/internal/a-stock/recommendation-selections":
			selectionPosts++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Inserted: 2, Total: 2})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "afternoon", 1, false, false, true, false, true, newAStockRequestCache())
	if len(ctx.Recommendations) != 2 || ctx.Recommendations[0].Code != "002008" || ctx.Recommendations[1].Code != "688367" {
		t.Fatalf("expected locked afternoon selections to stay unchanged, got %+v", ctx.Recommendations)
	}
	if selectionGets == 0 {
		t.Fatal("expected persisted selection lookup")
	}
	if afternoonSnapshotGets != 1 {
		t.Fatalf("expected locked selection path to read afternoon snapshot once for close recovery candidates, got %d", afternoonSnapshotGets)
	}
	if selectionPosts != 0 {
		t.Fatalf("expected locked selection path to avoid rewriting selections, got %d", selectionPosts)
	}
}

func TestAStockRecommendationGeneratePreserveLockedRefreshModeKeepsSelections(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(writeAStockTestMarketBars))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	selectionPosts := 0
	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-23",
				Period:       "morning",
				Items: []model.AStockRecommendationSelection{
					{Rank: 1, Code: "603986", Name: "Locked Corp", Hotspot: "locked", MarketScore: 90, Reason: "locked selection"},
				},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/internal/a-stock/recommendation-shadow-snapshots":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/internal/a-stock/recommendation-selections":
			selectionPosts++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/recommendations/generate?date=2026-06-23&period=morning&phase=final&refresh_mode=preserve_locked&ignore_fund_flow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockRecommendationGenerate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected generate 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if selectionPosts != 0 {
		t.Fatalf("expected preserve_locked mode not to rewrite selections, got %d posts", selectionPosts)
	}
	var response struct {
		Data aStockRecommendationGenerateResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	if response.Data.RecentLookbackDays != 90 {
		t.Fatalf("expected generate response recent_lookback_days=90, got %+v", response.Data)
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(savedSnapshot.RecommendationsJSON), &recommendations); err != nil {
		t.Fatalf("decode saved recommendations: %v", err)
	}
	if len(recommendations) != 1 || recommendations[0].Code != "603986" {
		t.Fatalf("expected locked selection to be preserved in snapshot, got %+v", recommendations)
	}
}

func TestAStockRecommendationGeneratePreserveLockedRecoversOpenedLimitUpAfterClose(t *testing.T) {
	originalNow := aStockNow
	aStockNow = func() time.Time {
		return time.Date(2026, 6, 23, 15, 5, 0, 0, aStockLocation())
	}
	defer func() { aStockNow = originalNow }()

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		date := r.URL.Query().Get("date")
		codes := strings.Split(r.URL.Query().Get("codes"), ",")
		items := make([]map[string]any, 0, len(codes)*2)
		for _, rawCode := range codes {
			code := normalizeAStockCode(rawCode)
			if code == "" {
				continue
			}
			items = append(items, map[string]any{
				"code":  code,
				"date":  "2026-06-20",
				"close": 10.00,
				"pct":   0.00,
			})
			open := 10.00
			low := 10.00
			closePrice := 10.50
			pct := 5.00
			if code == "600162" {
				open = 11.00
				low = 10.50
				closePrice = 11.00
				pct = 10.00
			}
			items = append(items, map[string]any{
				"code":        code,
				"date":        date,
				"open":        open,
				"low":         low,
				"close":       closePrice,
				"pct":         pct,
				"entry_price": open,
			})
		}
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": items})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	lockedSelections := []model.AStockRecommendationSelection{
		{Rank: 1, Code: "600001", Name: "常规一", Hotspot: "热点A", MarketScore: 100, Reason: "locked-1"},
		{Rank: 2, Code: "600002", Name: "常规二", Hotspot: "热点A", MarketScore: 99, Reason: "locked-2"},
		{Rank: 3, Code: "600003", Name: "常规三", Hotspot: "热点B", MarketScore: 98, Reason: "locked-3"},
		{Rank: 4, Code: "600004", Name: "常规四", Hotspot: "热点B", MarketScore: 97, Reason: "locked-4"},
		{Rank: 5, Code: "600005", Name: "常规五", Hotspot: "热点C", MarketScore: 96, Reason: "locked-5"},
	}
	filtered := []aStockFilteredRecommendation{{
		Reason: aStockFilteredReasonTodayHighPct,
		Recommendation: aStockRecommendation{
			Rank:         6,
			Hotspot:      "地产",
			Code:         "600162",
			Name:         "香江控股",
			HotspotScore: 90,
			MarketScore:  95,
			Reason:       "热度分 90",
		},
	}}
	filteredJSON := mustAStockTestJSON(t, filtered)
	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-23",
				Period:       "morning",
				Items:        lockedSelections,
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                       true,
				StrategyDate:                "2026-06-23",
				Period:                      "morning",
				RecommendationsJSON:         mustAStockTestJSON(t, []aStockRecommendation{}),
				FilteredRecommendationsJSON: filteredJSON,
				BacktestsJSON:               `[]`,
			})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/internal/a-stock/recommendation-shadow-snapshots":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/recommendations/generate?date=2026-06-23&period=morning&phase=final&refresh_mode=preserve_locked&ignore_fund_flow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockRecommendationGenerate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected generate 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Data aStockRecommendationGenerateResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	if response.Data.RecoveredCount != 1 || response.Data.RecommendationCount != 6 {
		t.Fatalf("expected one recovered recommendation beyond five regular picks, got %+v", response.Data)
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(savedSnapshot.RecommendationsJSON), &recommendations); err != nil {
		t.Fatalf("decode saved recommendations: %v", err)
	}
	if len(recommendations) != 6 || recommendations[5].Code != "600162" || !recommendations[5].Recovered || !strings.Contains(recommendations[5].RecoveryReason, "不占每日5只限制") {
		t.Fatalf("expected recovered 600162 to be saved as rank 6, got %+v", recommendations)
	}
	if countAStockDailyLimitRecommendations(recommendations) != 5 {
		t.Fatalf("expected recovered recommendation to be excluded from daily limit count, got %+v", recommendations)
	}
	if !strings.Contains(savedSnapshot.BacktestStatus, "收盘开板恢复股票 1") || !strings.Contains(savedSnapshot.FilteredRecommendationsJSON, "600162") {
		t.Fatalf("expected saved snapshot to record recovery status and filtered candidates, got %+v", savedSnapshot)
	}
}

func TestAStockContextRefreshDoesNotRefilterLockedAfternoonSelectionsByMorningQuota(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "002008", "date": "2026-06-20", "close": 120.0, "pct": 1.2},
					{"code": "002008", "date": "2026-06-23", "close": 135.54, "pct": 7.57, "afternoon_entry_price": 126.00},
					{"code": "688367", "date": "2026-06-20", "close": 48.10, "pct": 0.8},
					{"code": "688367", "date": "2026-06-23", "close": 50.87, "pct": 4.93, "afternoon_entry_price": 49.10},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			switch r.URL.Query().Get("period") {
			case "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "morning",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "600001", Name: "上午一", Hotspot: "机器人", MarketScore: 100, Reason: "morning-1"},
						{Rank: 2, Code: "600002", Name: "上午二", Hotspot: "半导体", MarketScore: 99, Reason: "morning-2"},
					},
				})
			case "afternoon":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "afternoon",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 91, Reason: "locked-1"},
						{Rank: 2, Code: "688367", Name: "工大高科", Hotspot: "机器人", MarketScore: 87, Reason: "locked-2"},
					},
				})
			case "evening":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
			default:
				t.Fatalf("unexpected selection period: %s", r.URL.RawQuery)
			}
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "afternoon", 1, false, false, true, false, true, newAStockRequestCache())
	if len(ctx.Recommendations) != 2 || ctx.SameDayMorningFiltered != 0 {
		t.Fatalf("expected locked afternoon selections to avoid morning quota refilter, got filtered=%d recommendations=%+v", ctx.SameDayMorningFiltered, ctx.Recommendations)
	}
	var savedRecommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(savedSnapshot.RecommendationsJSON), &savedRecommendations); err != nil {
		t.Fatalf("decode saved recommendations: %v", err)
	}
	if len(savedRecommendations) != 2 || savedRecommendations[0].Code != "002008" || savedRecommendations[1].Code != "688367" {
		t.Fatalf("expected saved snapshot to keep locked selections, got %+v", savedRecommendations)
	}
}

func TestAStockRebuildRefiltersLockedMorningSelectionsAndClearsPersistedRows(t *testing.T) {
	currentSelectionGets := 0
	var savedSelections model.AStockRecommendationSelectionSet
	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:         900,
					SourceType: "flash",
					Title:      "AI 算力政策加码，科大讯飞活跃",
					Summary:    "人工智能产业链活跃",
					TagFlags:   "0.002230",
					CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date:  r.URL.Query().Get("date"),
				Items: []model.AStockAuctionAmount{{TradeDate: r.URL.Query().Get("date"), Code: "002230", Name: "科大讯飞", AuctionVolume: 1000000, AuctionAmount: 10000000, Status: "ok"}},
			})
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("date") == "2026-06-16" {
				currentSelectionGets++
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "002230", Name: "科大讯飞", Hotspot: "人工智能", Reason: "locked"}},
				})
				return
			}
			if r.URL.Query().Get("date") == "2026-06-15" && r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "002230", Name: "科大讯飞", Hotspot: "人工智能", Reason: "recent"},
						{Rank: 2, Code: "603019", Name: "中科曙光", Hotspot: "人工智能", Reason: "recent"},
						{Rank: 3, Code: "601138", Name: "工业富联", Hotspot: "人工智能", Reason: "recent"},
					},
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			handleEmptyAStockAuctionTestEndpoint(w, r)
		case "/api/v1/internal/a-stock/recommendation-selections":
			if err := json.NewDecoder(r.Body).Decode(&savedSelections); err != nil {
				t.Fatalf("decode saved selections: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Total: len(savedSelections.Items)})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithRecommendationPhasePersistenceMode("2026-06-16", "morning", 1, false, false, true, false, true, aStockRecommendationPhaseFinal, newAStockRequestCache(), false, true, aStockRecommendationRebuild)
	if currentSelectionGets != 0 {
		t.Fatalf("expected rebuild to skip current locked selections, got %d current selection reads", currentSelectionGets)
	}
	if ctx.RecentFiltered == 0 || len(ctx.Recommendations) != 0 {
		t.Fatalf("expected rebuild to filter the regenerated recommendation, got filtered=%d recommendations=%+v", ctx.RecentFiltered, ctx.Recommendations)
	}
	if savedSelections.StrategyDate != "2026-06-16" || savedSelections.Period != "morning" || len(savedSelections.Items) != 0 {
		t.Fatalf("expected rebuild to save empty selections for cleanup, got %+v", savedSelections)
	}
	if savedSnapshot.StrategyDate != "2026-06-16" || savedSnapshot.Period != "morning" || savedSnapshot.RecentFiltered == 0 || strings.TrimSpace(savedSnapshot.RecommendationsJSON) != "[]" {
		t.Fatalf("expected rebuild to save empty filtered snapshot, got %+v", savedSnapshot)
	}
}

func TestAStockMorningRebuildUsesExpandedPoolAfterRecentFilter(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		items := make([]map[string]any, 0)
		for _, code := range []string{"300024", "300857", "688327", "002230", "603019", "601138"} {
			items = append(items,
				map[string]any{"code": code, "date": "2026-06-15", "open": 10.00, "close": 10.00, "pct": 0.10},
				map[string]any{"code": code, "date": "2026-06-16", "open": 10.10, "close": 10.30, "pct": 2.00, "entry_price": 10.10},
			)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": items}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	var savedSelections model.AStockRecommendationSelectionSet
	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          1,
					SourceType:  "flash",
					Title:       "AI 人工智能算力持续升温",
					Summary:     "人工智能产业链盘前活跃",
					PublishTime: "2026-06-16 09:20:00",
					TagFlags:    "0.002230 0.603019 0.601138",
					RawPayload:  `{"stock_list":[{"code":"300024","name":"机器人"},{"code":"300857","name":"协创数据"},{"code":"688327","name":"云从科技"}]}`,
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date: r.URL.Query().Get("date"),
				Items: []model.AStockAuctionAmount{
					{TradeDate: r.URL.Query().Get("date"), Code: "300024", Name: "机器人", AuctionVolume: 1000000, AuctionAmount: 12000000, Status: "ok"},
					{TradeDate: r.URL.Query().Get("date"), Code: "300857", Name: "协创数据", AuctionVolume: 900000, AuctionAmount: 11000000, Status: "ok"},
					{TradeDate: r.URL.Query().Get("date"), Code: "688327", Name: "云从科技", AuctionVolume: 800000, AuctionAmount: 10000000, Status: "ok"},
					{TradeDate: r.URL.Query().Get("date"), Code: "002230", Name: "科大讯飞", AuctionVolume: 700000, AuctionAmount: 9000000, Status: "ok"},
					{TradeDate: r.URL.Query().Get("date"), Code: "603019", Name: "中科曙光", AuctionVolume: 600000, AuctionAmount: 8000000, Status: "ok"},
					{TradeDate: r.URL.Query().Get("date"), Code: "601138", Name: "工业富联", AuctionVolume: 500000, AuctionAmount: 7000000, Status: "ok"},
				},
			})
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("date") == "2026-06-15" && r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "002230", Name: "科大讯飞", Hotspot: "人工智能", Reason: "recent"},
						{Rank: 2, Code: "603019", Name: "中科曙光", Hotspot: "人工智能", Reason: "recent"},
						{Rank: 3, Code: "601138", Name: "工业富联", Hotspot: "人工智能", Reason: "recent"},
					},
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			handleEmptyAStockAuctionTestEndpoint(w, r)
		case "/api/v1/internal/a-stock/recommendation-selections":
			if err := json.NewDecoder(r.Body).Decode(&savedSelections); err != nil {
				t.Fatalf("decode saved selections: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Total: len(savedSelections.Items)})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithRecommendationPhasePersistenceMode("2026-06-16", "morning", 1, false, false, true, false, true, aStockRecommendationPhaseFinal, newAStockRequestCache(), false, true, aStockRecommendationRebuild)
	if ctx.RecentFiltered != 3 || ctx.RecentReplenished != 0 || ctx.RecentReplenishShortfall {
		t.Fatalf("expected recent stocks to be filtered from expanded pool without replenishment bookkeeping, got filtered=%d replenished=%d shortfall=%v recs=%+v", ctx.RecentFiltered, ctx.RecentReplenished, ctx.RecentReplenishShortfall, ctx.Recommendations)
	}
	if len(ctx.Recommendations) != 3 || len(savedSelections.Items) != 3 {
		t.Fatalf("expected three non-recent recommendations from expanded pool to be saved, ctx=%+v saved=%+v", ctx.Recommendations, savedSelections)
	}
	gotCodes := aStockRecommendationCodeSet(ctx.Recommendations)
	for _, code := range []string{"300024", "300857", "688327"} {
		if _, ok := gotCodes[code]; !ok {
			t.Fatalf("expected replacement code %s, got %+v", code, ctx.Recommendations)
		}
	}
	for _, code := range []string{"002230", "603019", "601138"} {
		if _, ok := gotCodes[code]; ok {
			t.Fatalf("expected recent code %s to stay filtered, got %+v", code, ctx.Recommendations)
		}
	}
	if !strings.Contains(ctx.BacktestStatus, "90个交易日内重复过滤 3 只") || strings.Contains(ctx.BacktestStatus, "递补") {
		t.Fatalf("expected backtest status to mention expanded-pool recent filtering without replenishment, got %q", ctx.BacktestStatus)
	}
	if savedSnapshot.RecentFiltered != 3 || strings.Contains(savedSnapshot.BacktestStatus, "递补") {
		t.Fatalf("expected saved snapshot to preserve expanded-pool recent filter status, got %+v", savedSnapshot)
	}
}

func TestAStockContextEmptySnapshotIsAuthoritative(t *testing.T) {
	marketHits := 0
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marketHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "002008", "date": "2026-06-20", "close": 120.0, "pct": 1.2},
					{"code": "002008", "date": "2026-06-23", "close": 135.54, "pct": 7.57, "afternoon_entry_price": 126.00},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	selectionHits := 0
	holdingHits := 0
	saveHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			selectionHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-23",
				Period:       "afternoon",
				Items: []model.AStockRecommendationSelection{
					{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 91, Reason: "locked"},
				},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                 true,
				StrategyDate:          "2026-06-23",
				Period:                "afternoon",
				RecommendationsJSON:   "[]",
				BacktestsJSON:         "[]",
				BacktestStatus:        "无推荐股票",
				EmptyReason:           "空快照",
				LimitUpFilterEnabled:  true,
				FundFlowFilterEnabled: false,
			})
		case "/api/v1/internal/a-stock/recommendations":
			saveHits++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			holdingHits++
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "afternoon", 1, false, false, true, false, false, newAStockRequestCache())
	if len(ctx.Recommendations) != 0 || len(ctx.Backtests) != 0 {
		t.Fatalf("expected empty snapshot to remain empty, got recommendations=%+v backtests=%+v", ctx.Recommendations, ctx.Backtests)
	}
	if ctx.BacktestStatus != "无推荐股票" || ctx.EmptyReason != "空快照" {
		t.Fatalf("expected empty snapshot status/reason, got status=%q reason=%q", ctx.BacktestStatus, ctx.EmptyReason)
	}
	if marketHits != 0 || selectionHits != 0 || holdingHits != 0 || saveHits != 0 {
		t.Fatalf("expected empty snapshot to avoid recompute and writes, got market=%d selections=%d holdings=%d saves=%d", marketHits, selectionHits, holdingHits, saveHits)
	}
}

func TestAStockPageRefreshAllBacktestsPersistsAfternoonBacktestUpdate(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch normalizeAStockCode(r.URL.Query().Get("codes")) {
		case "603083":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "603083", "date": "2026-06-22", "open": 238.00, "close": 238.00, "pct": 0.50},
						{"code": "603083", "date": "2026-06-23", "open": 240.00, "close": 244.08, "pct": 1.70, "entry_price": 240.00},
					},
				},
			})
		case "002008":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "002008", "date": "2026-06-22", "open": 131.93, "close": 131.93, "pct": -2.20},
						{"code": "002008", "date": "2026-06-23", "open": 135.54, "close": 145.11, "pct": 9.99},
					},
				},
			})
		default:
			t.Fatalf("unexpected market query: %s", r.URL.RawQuery)
		}
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		secid := r.URL.Query().Get("secid")
		switch {
		case secid == "0.002008" && r.URL.Query().Get("klt") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-23 09:30,135.54,136.20,0,0,0,0,0,0",
						"2026-06-23 13:01,141.98,142.20,0,0,0,0,0,0",
					},
				},
			})
		case secid == "1.603083" && r.URL.Query().Get("klt") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-23 09:30,239.80,240.00,0,0,0,0,0,0",
					},
				},
			})
		default:
			t.Fatalf("unexpected eastmoney query: %s", r.URL.RawQuery)
		}
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	savedSnapshots := map[string]model.AStockRecommendationSnapshot{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			switch r.URL.Query().Get("period") {
			case "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "morning",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "603083", Name: "剑桥科技", Hotspot: "人工智能", MarketScore: 91, Reason: "morning"},
					},
				})
			case "afternoon":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "afternoon",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 87, Reason: "afternoon"},
					},
				})
			case "evening":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
			default:
				t.Fatalf("unexpected selection period: %s", r.URL.RawQuery)
			}
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			savedSnapshots[snapshot.Period] = snapshot
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := newAStockRefreshBacktestPostRequest("2026-06-23", "morning")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	afternoonSnapshot, ok := savedSnapshots["afternoon"]
	if !ok {
		t.Fatalf("expected afternoon snapshot to be persisted, got %+v", savedSnapshots)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(afternoonSnapshot.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode afternoon backtests: %v", err)
	}
	if len(backtests) != 1 {
		t.Fatalf("expected one afternoon backtest row, got %+v", backtests)
	}
	if backtests[0].AfternoonOpen != "142.20" || backtests[0].T0Return != "+2.05%" || backtests[0].T0Close != "145.11" {
		t.Fatalf("expected refreshed afternoon backtest values, got %+v", backtests[0])
	}
}

func TestAStockPageRefreshAllBacktestsSupplementsPartialCustomMarketHistory(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 24, 14, 0, 0, 0, time.FixedZone("CST", 8*3600)))

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch normalizeAStockCode(r.URL.Query().Get("codes")) {
		case "603083":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "603083", "date": "2026-06-22", "open": 238.00, "close": 238.00, "pct": 0.50},
						{"code": "603083", "date": "2026-06-23", "open": 240.00, "close": 244.08, "pct": 1.70, "entry_price": 240.00},
						{"code": "603083", "date": "2026-06-24", "open": 245.00, "close": 262.20, "pct": 7.42},
					},
				},
			})
		case "002008":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "002008", "date": "2026-06-22", "open": 131.93, "close": 131.93, "pct": -2.20},
						{"code": "002008", "date": "2026-06-24", "open": 146.00, "close": 148.02, "pct": 2.00},
					},
				},
			})
		default:
			t.Fatalf("unexpected market query: %s", r.URL.RawQuery)
		}
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		secid := r.URL.Query().Get("secid")
		switch {
		case secid == "0.002008" && r.URL.Query().Get("klt") == "101":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-22,131.93,131.93,0,0,0,0,0,-2.20",
						"2026-06-23,135.54,145.11,0,0,0,0,0,9.99",
						"2026-06-24,146.00,148.02,0,0,0,0,0,2.00",
					},
				},
			})
		case secid == "0.002008" && r.URL.Query().Get("klt") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-23 09:30,135.54,136.20,0,0,0,0,0,0",
						"2026-06-23 13:01,141.98,142.20,0,0,0,0,0,0",
					},
				},
			})
		case secid == "1.603083" && r.URL.Query().Get("klt") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-23 09:30,239.80,240.00,0,0,0,0,0,0",
					},
				},
			})
		default:
			t.Fatalf("unexpected eastmoney query: %s", r.URL.RawQuery)
		}
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	savedSnapshots := map[string]model.AStockRecommendationSnapshot{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			switch r.URL.Query().Get("period") {
			case "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "morning",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "603083", Name: "剑桥科技", Hotspot: "人工智能", MarketScore: 91, Reason: "morning"},
					},
				})
			case "afternoon":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "afternoon",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 87, Reason: "afternoon"},
					},
				})
			case "evening":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
			default:
				t.Fatalf("unexpected selection period: %s", r.URL.RawQuery)
			}
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			savedSnapshots[snapshot.Period] = snapshot
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := newAStockRefreshBacktestPostRequest("2026-06-23", "morning")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	afternoonSnapshot, ok := savedSnapshots["afternoon"]
	if !ok {
		t.Fatalf("expected afternoon snapshot to be persisted, got %+v", savedSnapshots)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(afternoonSnapshot.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode afternoon backtests: %v", err)
	}
	if len(backtests) != 1 {
		t.Fatalf("expected one afternoon backtest row, got %+v", backtests)
	}
	if backtests[0].AfternoonOpen != "142.20" || backtests[0].T0Return != "+2.05%" || backtests[0].T0Close != "145.11" {
		t.Fatalf("expected supplemented afternoon backtest values, got %+v", backtests[0])
	}
	if backtests[0].Days[0].Close != "148.02" || backtests[0].Days[0].Return != "+4.09%" {
		t.Fatalf("expected supplemented T+1 values, got %+v", backtests[0])
	}
}

func TestAStockPageRefreshAllBacktestsSupplementsCurrentDayAfternoonBacktest(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 24, 14, 2, 0, 0, time.FixedZone("CST", 8*3600)))

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch normalizeAStockCode(r.URL.Query().Get("codes")) {
		case "603936":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "603936", "date": "2026-06-23", "open": 31.05, "close": 31.05, "pct": 0.45},
						{"code": "603936", "date": "2026-06-24", "open": 29.00, "close": 27.09, "pct": -6.59, "entry_price": 29.00},
					},
				},
			})
		case "300024":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "300024", "date": "2026-06-23", "open": 16.79, "close": 16.79, "pct": -2.21},
						{"code": "300024", "date": "2026-06-24", "open": 16.26, "close": 16.42, "pct": 0.98},
					},
				},
			})
		default:
			t.Fatalf("unexpected market query: %s", r.URL.RawQuery)
		}
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		secid := r.URL.Query().Get("secid")
		end := r.URL.Query().Get("end")
		switch {
		case secid == "1.603936" && r.URL.Query().Get("klt") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-24 09:30,28.95,29.00,0,0,0,0,0,0",
					},
				},
			})
		case secid == "0.300024" && r.URL.Query().Get("klt") == "1" && strings.Contains(end, "09:30"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-24 09:30,16.00,16.03,0,0,0,0,0,0",
					},
				},
			})
		case secid == "0.300024" && r.URL.Query().Get("klt") == "1" && strings.Contains(end, "13:01"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-24 13:01,16.05,16.08,0,0,0,0,0,0",
					},
				},
			})
		default:
			t.Fatalf("unexpected eastmoney query: %s", r.URL.RawQuery)
		}
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	savedSnapshots := map[string]model.AStockRecommendationSnapshot{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			switch r.URL.Query().Get("period") {
			case "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-24",
					Period:       "morning",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "603936", Name: "博敏电子", Hotspot: "AI硬件", MarketScore: 88, Reason: "morning"},
					},
				})
			case "afternoon":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-24",
					Period:       "afternoon",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "300024", Name: "机器人", Hotspot: "人工智能", MarketScore: 92, Reason: "afternoon"},
					},
				})
			case "evening":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
			default:
				t.Fatalf("unexpected selection period: %s", r.URL.RawQuery)
			}
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			savedSnapshots[snapshot.Period] = snapshot
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := newAStockRefreshBacktestPostRequest("2026-06-24", "morning")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	afternoonSnapshot, ok := savedSnapshots["afternoon"]
	if !ok {
		t.Fatalf("expected afternoon snapshot to be persisted, got %+v", savedSnapshots)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(afternoonSnapshot.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode afternoon backtests: %v", err)
	}
	if len(backtests) != 1 {
		t.Fatalf("expected one afternoon backtest row, got %+v", backtests)
	}
	if backtests[0].AfternoonOpen != "16.08" || backtests[0].T0Return != "+2.11%" || backtests[0].T0Close != "16.42" {
		t.Fatalf("expected current-day afternoon backtest values to be supplemented, got %+v", backtests[0])
	}
	if backtests[0].Status != "等待T+1行情" {
		t.Fatalf("expected current-day afternoon backtest to wait for T+1, got %+v", backtests[0])
	}
}

func TestAStockPageRefreshCurrentBacktestPersistsT0Return(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 24, 14, 2, 0, 0, time.FixedZone("CST", 8*3600)))
	setAStockEastmoneyQuoteURLForTest(t, "")

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if normalizeAStockCode(r.URL.Query().Get("codes")) != "300024" {
			t.Fatalf("unexpected market query: %s", r.URL.RawQuery)
		}
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{
			"items": []map[string]any{
				{"code": "300024", "date": "2026-06-23", "open": 16.79, "close": 16.79, "pct": -2.21},
				{"code": "300024", "date": "2026-06-24", "open": 16.26, "close": 16.42, "pct": 0.98},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("secid") != "0.300024" || r.URL.Query().Get("klt") != "1" {
			t.Fatalf("unexpected eastmoney query: %s", r.URL.RawQuery)
		}
		price := "16.80"
		minute := "13:01"
		switch {
		case strings.Contains(r.URL.Query().Get("end"), "09:30"):
			price = "16.03"
			minute = "09:30"
		case strings.Contains(r.URL.Query().Get("end"), "10:30"):
			price = "16.08"
			minute = "10:30"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"klines": []string{
					fmt.Sprintf("2026-06-24 %s,16.05,%s,0,0,0,0,0,0", minute, price),
				},
			},
		})
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	var savedSnapshot model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found:        true,
				StrategyDate: "2026-06-24",
				Period:       "afternoon",
				Items: []model.AStockRecommendationSelection{
					{Rank: 1, Code: "300024", Name: "机器人", Hotspot: "人工智能", MarketScore: 92, Reason: "afternoon", EntryTime: "10:30"},
				},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := newAStockRefreshCurrentBacktestPostRequest("2026-06-24", "afternoon")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	if savedSnapshot.Period != "afternoon" {
		t.Fatalf("expected current afternoon snapshot to be saved, got %+v", savedSnapshot)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(savedSnapshot.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode backtests: %v", err)
	}
	if len(backtests) != 1 || backtests[0].AfternoonOpen != "16.08" || backtests[0].T0Close != "16.42" || backtests[0].T0Return != "+2.11%" {
		t.Fatalf("expected current backtest action to persist T+0 return, got %+v", backtests)
	}
}

func TestAStockPageRefreshAllBacktestsPreservesPersistedAfternoonPrices(t *testing.T) {
	setAStockSinaMinuteURLForTest(t, "")

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch normalizeAStockCode(r.URL.Query().Get("codes")) {
		case "603083":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "603083", "date": "2026-06-22", "open": 238.00, "close": 238.00, "pct": 0.50},
						{"code": "603083", "date": "2026-06-23", "open": 240.00, "close": 244.08, "pct": 1.70, "entry_price": 240.00},
					},
				},
			})
		case "002008":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"code": "002008", "date": "2026-06-22", "open": 131.93, "close": 131.93, "pct": -2.20},
						{"code": "002008", "date": "2026-06-23", "open": 135.54, "close": 145.11, "pct": 9.99},
					},
				},
			})
		default:
			t.Fatalf("unexpected market query: %s", r.URL.RawQuery)
		}
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	afternoonMinuteHits := 0
	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		secid := r.URL.Query().Get("secid")
		end := r.URL.Query().Get("end")
		switch {
		case secid == "0.002008" && r.URL.Query().Get("klt") == "1" && strings.Contains(end, "09:30"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-23 09:30,135.54,136.20,0,0,0,0,0,0",
						"2026-06-23 13:01,141.98,142.20,0,0,0,0,0,0",
					},
				},
			})
		case secid == "0.002008" && r.URL.Query().Get("klt") == "1" && strings.Contains(end, "13:01"):
			afternoonMinuteHits++
			lines := []string{}
			if afternoonMinuteHits == 1 {
				lines = []string{
					"2026-06-23 09:30,135.54,136.20,0,0,0,0,0,0",
					"2026-06-23 13:01,141.98,142.20,0,0,0,0,0,0",
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": lines,
				},
			})
		case secid == "1.603083" && r.URL.Query().Get("klt") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-23 09:30,239.80,240.00,0,0,0,0,0,0",
					},
				},
			})
		default:
			t.Fatalf("unexpected eastmoney query: %s", r.URL.RawQuery)
		}
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	savedSnapshots := map[string]model.AStockRecommendationSnapshot{}
	afternoonSnapshotGets := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			switch r.URL.Query().Get("period") {
			case "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "morning",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "603083", Name: "剑桥科技", Hotspot: "人工智能", MarketScore: 91, Reason: "morning"},
					},
				})
			case "afternoon":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-23",
					Period:       "afternoon",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "002008", Name: "大族激光", Hotspot: "机器人", MarketScore: 87, Reason: "afternoon"},
					},
				})
			case "evening":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
			default:
				t.Fatalf("unexpected selection period: %s", r.URL.RawQuery)
			}
		case "/api/v1/a-stock/recommendations":
			period := r.URL.Query().Get("period")
			if period == "afternoon" {
				afternoonSnapshotGets++
			}
			snapshot, ok := savedSnapshots[period]
			if !ok {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
				return
			}
			snapshot.Found = true
			writeEnvelope(w, http.StatusOK, "ok", snapshot)
		case "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			savedSnapshots[snapshot.Period] = snapshot
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	for i := 0; i < 2; i++ {
		req := newAStockRefreshBacktestPostRequest("2026-06-23", "morning")
		rr := httptest.NewRecorder()
		srv.handleAStockPage(rr, req, map[string]any{"id": 1})
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("refresh %d expected 303, got %d body=%s", i+1, rr.Code, rr.Body.String())
		}
		if i == 0 {
			afternoonSnapshot := savedSnapshots["afternoon"]
			var firstRows []aStockBacktestRow
			if err := json.Unmarshal([]byte(afternoonSnapshot.BacktestsJSON), &firstRows); err != nil {
				t.Fatalf("decode first afternoon backtests: %v", err)
			}
			if len(firstRows) != 1 || firstRows[0].AfternoonOpen != "142.20" || firstRows[0].T0Return != "+2.05%" {
				t.Fatalf("expected first refresh to persist afternoon prices, got %+v", firstRows)
			}
		}
	}

	afternoonSnapshot, ok := savedSnapshots["afternoon"]
	if !ok {
		t.Fatalf("expected afternoon snapshot to be persisted, got %+v", savedSnapshots)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(afternoonSnapshot.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode afternoon backtests: %v", err)
	}
	if len(backtests) != 1 {
		t.Fatalf("expected one afternoon backtest row, got %+v", backtests)
	}
	if backtests[0].AfternoonOpen != "142.20" || backtests[0].T0Return != "+2.05%" || backtests[0].T0Close != "145.11" {
		t.Fatalf("expected persisted afternoon backtest values after second refresh, got %+v", backtests[0])
	}
	if !strings.Contains(afternoonSnapshot.BacktestStatus, "已回测") {
		t.Fatalf("expected afternoon status to keep persisted backtest progress, got %q", afternoonSnapshot.BacktestStatus)
	}
	if afternoonMinuteHits != 2 {
		t.Fatalf("expected two afternoon minute lookups, got %d", afternoonMinuteHits)
	}
	if afternoonSnapshotGets == 0 {
		t.Fatal("expected second refresh to read persisted afternoon snapshot")
	}
}

func TestAStockContextRefreshSeedsSelectionsFromExistingSnapshot(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope(w, http.StatusOK, "ok", []map[string]any{
			{"code": "603083", "date": "2026-06-23", "open": 240.00, "close": 244.08, "pct": 1.70, "entry_price": 240.00},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	var savedSelections model.AStockRecommendationSelectionSet
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                 true,
				StrategyDate:          "2026-06-23",
				Period:                "morning",
				RecommendationsJSON:   `[{"Rank":1,"Hotspot":"人工智能","Code":"603083","Name":"剑桥科技","Reason":"snapshot"}]`,
				BacktestsJSON:         `[]`,
				BacktestStatus:        "旧快照",
				GeneratedCount:        1,
				FundFlowFilterEnabled: false,
			})
		case "/api/v1/internal/a-stock/recommendation-selections":
			if err := json.NewDecoder(r.Body).Decode(&savedSelections); err != nil {
				t.Fatalf("decode saved selections: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Inserted: 1, Total: 1})
		case "/api/v1/internal/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextWithCache("2026-06-23", "morning", 1, false, false, true, false, true, newAStockRequestCache())

	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].Code != "603083" {
		t.Fatalf("expected refresh to keep snapshot recommendation code, got %+v", ctx.Recommendations)
	}
	if len(savedSelections.Items) != 1 || savedSelections.Items[0].Code != "603083" {
		t.Fatalf("expected snapshot code to be saved into selections, got %+v", savedSelections)
	}
}

func TestAStockContextRejectsMismatchedLimitUpSnapshot(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.AStockRecommendationSnapshot{
				Found:                true,
				StrategyDate:         "2026-06-22",
				Period:               "afternoon",
				RecommendationsJSON:  `[{"Rank":1,"Code":"002230","Name":"科大讯飞"}]`,
				BacktestsJSON:        `[]`,
				LimitUpFilterEnabled: true,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := aStockContext{Date: "2026-06-22", Period: "afternoon", IgnoreLimitUp: true, LimitUpFilterEnabled: false}
	if srv.applyAStockRecommendationSnapshot(&ctx) {
		t.Fatalf("expected disabled limit-up filter context to reject enabled snapshot, got %+v", ctx)
	}
}

func TestAStockAfternoonBacktestSnapshotRejectsLegacyEntryOpen(t *testing.T) {
	backtests := []aStockBacktestRow{{Stock: "002008 大族激光", EntryOpen: "135.54", AfternoonOpen: "--", T0Return: "+7.57%"}}
	if isFreshAStockBacktestSnapshot("afternoon", backtests) {
		t.Fatalf("expected afternoon snapshot with legacy entry open to be rejected, got %+v", backtests)
	}
	if isFreshAStockBacktestSnapshot("morning", backtests) {
		t.Fatalf("expected morning snapshot with T+0 return but missing close to be rejected, got %+v", backtests)
	}
	if !isFreshAStockBacktestSnapshot("morning", []aStockBacktestRow{{Stock: "603083 剑桥科技", EntryOpen: "240.00", T0Return: "+1.70%", T0Close: "238.37"}}) {
		t.Fatal("expected morning snapshot with T+0 close to stay valid")
	}
	if !isFreshAStockBacktestSnapshot("afternoon", []aStockBacktestRow{{Stock: "002008 大族激光", EntryOpen: "--", AfternoonOpen: "142.00", T0Return: "+2.11%", T0Close: "145.11"}}) {
		t.Fatal("expected afternoon snapshot with dedicated afternoon open to stay valid")
	}
}

func TestAStockRecommendationSnapshotRejectsWaitingMorningBacktest(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			http.NotFound(w, r)
			return
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
			Found:               true,
			StrategyDate:        "2026-06-30",
			Period:              "morning",
			RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{{Code: "603259", Name: "药明康德"}}),
			BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{{
				Stock:     "603259 药明康德",
				EntryOpen: "--",
				Status:    "等待当日开盘价",
			}}),
			BacktestStatus: "已回测 0/1，等待当日开盘价股票 1",
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := aStockContext{Date: "2026-06-30", Period: "morning"}
	if srv.applyAStockRecommendationSnapshot(&ctx) {
		t.Fatalf("expected waiting morning snapshot to be rejected, got %+v", ctx)
	}
}

func TestAStockReadOnlyPageUsesBacktestSnapshotAuthority(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			http.NotFound(w, r)
			return
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
			Found:                 true,
			StrategyDate:          "2026-07-03",
			Period:                "morning",
			RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "人工智能", Code: "601995", Name: "中金公司", Reason: "回测页快照股票"}}),
			BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "601995 中金公司", EntryOpen: "36.23", T0Return: "+0.41%", Status: "已回测T+0"}}),
			NewsSummaryJSON:       mustAStockTestJSON(t, aStockSnapshotNewsSummary{Articles: []model.Item{{ID: 1, Title: "人工智能消息"}}}),
			BacktestStatus:        "回测页快照",
			GeneratedCount:        1,
			FundFlowFilterEnabled: false,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx := srv.loadAStockContextReadOnlyWithCache("2026-07-03", "morning", 1, false, false, false, false, false, newAStockRequestCache())

	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].Code != "601995" {
		t.Fatalf("expected A股 page to use backtest snapshot recommendations, got %+v", ctx.Recommendations)
	}
	if len(ctx.Backtests) != 1 || aStockBacktestRowCode(ctx.Backtests[0]) != "601995" {
		t.Fatalf("expected A股 page backtests to match backtest snapshot, got %+v", ctx.Backtests)
	}
	if ctx.BacktestStatus != "回测页快照" {
		t.Fatalf("expected backtest snapshot status, got %q", ctx.BacktestStatus)
	}
}

func TestAStockMorningBacktestSnapshotRejectsWaitingEntryOpen(t *testing.T) {
	backtests := []aStockBacktestRow{{
		Stock:         "603259 药明康德",
		EntryOpen:     "--",
		T0Return:      "--",
		T0Close:       "--",
		Status:        "等待当日开盘价",
		Days:          []aStockBacktestCell{{Close: "--", Return: "--", ReturnClass: "astock-flat"}},
		BestReturn:    "--",
		AfternoonOpen: "--",
	}}
	if isFreshAStockBacktestSnapshot("morning", backtests) {
		t.Fatalf("expected waiting morning snapshot to be rejected, got %+v", backtests)
	}
}

func TestAStockPopupShowsAndDismissesAfternoonRecommendations(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 23, 12, 57, 0, 0, time.FixedZone("CST", 8*3600)))

	popupStates := map[string]model.PopupState{}
	taskRuns := []model.TaskRun{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:               true,
				StrategyDate:        "2026-06-23",
				Period:              "afternoon",
				RecommendationsJSON: `[{"Rank":1,"Code":"002230","Name":"科大讯飞","Hotspot":"人工智能","Reason":"消息驱动"},{"Rank":2,"Code":"688981","Name":"中芯国际","Hotspot":"半导体","Reason":"景气回升"}]`,
				UpdatedAt:           time.Date(2026, 6, 23, 4, 55, 0, 0, time.UTC),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			key := r.URL.Query().Get("key")
			state, ok := popupStates[popupStateMapKey(userID, key)]
			if !ok {
				state = model.PopupState{UserID: userID, Key: key}
			}
			writeEnvelope(w, http.StatusOK, "ok", state)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/popup":
			var state model.PopupState
			if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
				writeEnvelope(w, http.StatusBadRequest, err.Error(), nil)
				return
			}
			now := time.Now().UTC()
			state.UpdatedAt = now
			if state.Dismissed && state.DismissedAt == nil {
				state.DismissedAt = &now
			}
			popupStates[popupStateMapKey(state.UserID, state.Key)] = state
			writeEnvelope(w, http.StatusOK, "ok", state)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/system/task-runs":
			if r.Header.Get("X-Service-Token") != "test-token" {
				t.Fatalf("expected service token for task run write, got %q", r.Header.Get("X-Service-Token"))
			}
			var run model.TaskRun
			if err := json.NewDecoder(r.Body).Decode(&run); err != nil {
				writeEnvelope(w, http.StatusBadRequest, err.Error(), nil)
				return
			}
			taskRuns = append(taskRuns, run)
			writeEnvelope(w, http.StatusCreated, "ok", map[string]string{"task_name": run.TaskName})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/session" || strings.TrimSpace(r.URL.Query().Get("session_token")) != "session-admin" {
			t.Fatalf("unexpected auth request: %s", r.URL.String())
		}
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{
			"user": map[string]any{"id": int64(1), "username": "admin"},
		})
	}))
	defer auth.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, AuthURL: auth.URL, ServiceToken: "test-token"})
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/a-stock/popup?date=2026-06-23", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected popup 200, got %d", rr.Code)
	}
	var popup aStockPopupPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &popup); err != nil {
		t.Fatalf("unmarshal popup payload: %v", err)
	}
	if !popup.Show || popup.Key != "a-stock-afternoon-preopen-recommendation-1257-2026-06-23" || popup.Period != "afternoon" || len(popup.Recommendations) != 2 {
		t.Fatalf("expected popup to show afternoon recommendations, got %+v", popup)
	}

	dismissReq := httptest.NewRequest(http.MethodPost, "/a-stock/popup/dismiss", strings.NewReader(`{"key":"`+popup.Key+`"}`))
	dismissReq.Header.Set("Content-Type", "application/json")
	dismissReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	dismissRR := httptest.NewRecorder()
	router.ServeHTTP(dismissRR, dismissReq)
	if dismissRR.Code != http.StatusOK {
		t.Fatalf("expected dismiss 200, got %d body=%s", dismissRR.Code, dismissRR.Body.String())
	}
	if len(taskRuns) != 1 || taskRuns[0].TaskName != "a-stock-popup-dismiss:afternoon" || taskRuns[0].Status != "success" || !strings.Contains(taskRuns[0].Message, popup.Key) {
		t.Fatalf("expected popup dismiss task run, got %+v", taskRuns)
	}

	reqAfter := httptest.NewRequest(http.MethodGet, "/a-stock/popup?date=2026-06-23", nil)
	reqAfter.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	rrAfter := httptest.NewRecorder()
	router.ServeHTTP(rrAfter, reqAfter)
	var popupAfter aStockPopupPayload
	if err := json.Unmarshal(rrAfter.Body.Bytes(), &popupAfter); err != nil {
		t.Fatalf("unmarshal popup payload after dismiss: %v", err)
	}
	if popupAfter.Show {
		t.Fatalf("expected popup to stay hidden after dismiss, got %+v", popupAfter)
	}
}

func TestAStockPopupWaitsUntilAfternoonPreopenWindow(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 23, 12, 44, 59, 0, time.FixedZone("CST", 8*3600)))

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		t.Fatalf("unexpected content request before afternoon preopen window: %s", r.URL.String())
	}))
	defer content.Close()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{
			"user": map[string]any{"id": int64(1), "username": "admin"},
		})
	}))
	defer auth.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, AuthURL: auth.URL, ServiceToken: "test-token"})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/popup?date=2026-06-23", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected popup 200 before afternoon preopen window, got %d", rr.Code)
	}
	var popup aStockPopupPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &popup); err != nil {
		t.Fatalf("unmarshal popup payload before afternoon preopen window: %v", err)
	}
	if popup.Show {
		t.Fatalf("expected popup to stay hidden before afternoon preopen window, got %+v", popup)
	}
}

func TestAStockPopupShowsMorningRecommendationsDuringPreopenWindow(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 23, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			writeEnvelope(w, http.StatusOK, "ok", model.PopupState{
				UserID: parseTestInt64(r.URL.Query().Get("user_id")),
				Key:    r.URL.Query().Get("key"),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			if r.URL.Query().Get("period") != "morning" {
				t.Fatalf("expected morning popup recommendations, got query %s", r.URL.RawQuery)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:               true,
				StrategyDate:        "2026-06-23",
				Period:              "morning",
				RecommendationsJSON: `[{"Rank":1,"Code":"002230","Name":"科大讯飞","Hotspot":"人工智能","Reason":"上午盘前推荐"}]`,
				UpdatedAt:           time.Date(2026, 6, 23, 1, 36, 0, 0, time.UTC),
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	popup := srv.buildAStockPreopenPopup(1, "2026-06-23")
	if !popup.Show || popup.Key != "a-stock-morning-preopen-recommendation-0927-2026-06-23" || popup.Period != "morning" || len(popup.Recommendations) != 1 {
		t.Fatalf("expected morning preopen popup to show during configured window, got %+v", popup)
	}
}

func TestAStockPopupShowsMorningRecommendationsAfterSlowPreviewGeneration(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 23, 9, 31, 30, 0, time.FixedZone("CST", 8*3600)))

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			writeEnvelope(w, http.StatusOK, "ok", model.PopupState{
				UserID: parseTestInt64(r.URL.Query().Get("user_id")),
				Key:    r.URL.Query().Get("key"),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("period") != "morning" {
				t.Fatalf("expected morning selection lookup, got query %s", r.URL.RawQuery)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
				Found: true,
				Items: []model.AStockRecommendationSelection{{
					StrategyDate: "2026-06-23",
					Period:       "morning",
					Rank:         1,
					Code:         "002230",
					Name:         "科大讯飞",
					Hotspot:      "人工智能",
					Reason:       "上午预览任务 09:31 才完成",
				}},
				UpdatedAt: time.Date(2026, 6, 23, 1, 31, 24, 0, time.UTC),
			})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	popup := srv.buildAStockPreopenPopup(1, "2026-06-23")
	if !popup.Show || popup.Key != "a-stock-morning-preopen-recommendation-0931-2026-06-23" || popup.Period != "morning" || len(popup.Recommendations) != 1 {
		t.Fatalf("expected delayed morning preopen popup to show, got %+v", popup)
	}
}

func TestAStockPopupDismissesMorningWithoutHidingAfternoon(t *testing.T) {
	current := time.Date(2026, 6, 23, 9, 27, 0, 0, time.FixedZone("CST", 8*3600))
	previousNow := aStockNow
	aStockNow = func() time.Time {
		return current
	}
	t.Cleanup(func() {
		aStockNow = previousNow
	})

	popupStates := map[string]model.PopupState{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			period := r.URL.Query().Get("period")
			recommendationsJSON := `[{"Rank":1,"Code":"002230","Name":"科大讯飞","Hotspot":"人工智能","Reason":"上午消息驱动"}]`
			if period == "afternoon" {
				recommendationsJSON = `[{"Rank":1,"Code":"688981","Name":"中芯国际","Hotspot":"半导体","Reason":"下午消息驱动"}]`
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:               true,
				StrategyDate:        "2026-06-23",
				Period:              period,
				RecommendationsJSON: recommendationsJSON,
				UpdatedAt:           current.Add(-time.Minute).UTC(),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			key := r.URL.Query().Get("key")
			state, ok := popupStates[popupStateMapKey(userID, key)]
			if !ok {
				state = model.PopupState{UserID: userID, Key: key}
			}
			writeEnvelope(w, http.StatusOK, "ok", state)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/popup":
			var state model.PopupState
			if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
				writeEnvelope(w, http.StatusBadRequest, err.Error(), nil)
				return
			}
			now := current.UTC()
			state.UpdatedAt = now
			if state.Dismissed && state.DismissedAt == nil {
				state.DismissedAt = &now
			}
			popupStates[popupStateMapKey(state.UserID, state.Key)] = state
			writeEnvelope(w, http.StatusOK, "ok", state)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/system/task-runs":
			writeEnvelope(w, http.StatusCreated, "ok", map[string]string{"task_name": "a-stock-popup-dismiss:morning"})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{
			"user": map[string]any{"id": int64(1), "username": "admin"},
		})
	}))
	defer auth.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, AuthURL: auth.URL, ServiceToken: "test-token"})
	router := srv.Router()
	getPopup := func() aStockPopupPayload {
		req := httptest.NewRequest(http.MethodGet, "/a-stock/popup?date=2026-06-23", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected popup 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		var popup aStockPopupPayload
		if err := json.Unmarshal(rr.Body.Bytes(), &popup); err != nil {
			t.Fatalf("unmarshal popup payload: %v", err)
		}
		return popup
	}

	morningPopup := getPopup()
	if !morningPopup.Show || morningPopup.Key != "a-stock-morning-preopen-recommendation-0927-2026-06-23" || morningPopup.Period != "morning" {
		t.Fatalf("expected morning preopen popup, got %+v", morningPopup)
	}
	dismissReq := httptest.NewRequest(http.MethodPost, "/a-stock/popup/dismiss", strings.NewReader(`{"key":"`+morningPopup.Key+`"}`))
	dismissReq.Header.Set("Content-Type", "application/json")
	dismissReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	dismissRR := httptest.NewRecorder()
	router.ServeHTTP(dismissRR, dismissReq)
	if dismissRR.Code != http.StatusOK {
		t.Fatalf("expected dismiss 200, got %d body=%s", dismissRR.Code, dismissRR.Body.String())
	}
	if popupAfterDismiss := getPopup(); popupAfterDismiss.Show {
		t.Fatalf("expected morning popup hidden after dismiss, got %+v", popupAfterDismiss)
	}

	current = time.Date(2026, 6, 23, 9, 31, 0, 0, time.FixedZone("CST", 8*3600))
	secondMorningPopup := getPopup()
	if !secondMorningPopup.Show || secondMorningPopup.Key != "a-stock-morning-preopen-recommendation-0931-2026-06-23" || secondMorningPopup.Period != "morning" {
		t.Fatalf("expected second morning preopen popup to show independently, got %+v", secondMorningPopup)
	}

	current = time.Date(2026, 6, 23, 12, 57, 0, 0, time.FixedZone("CST", 8*3600))
	afternoonPopup := getPopup()
	if !afternoonPopup.Show || afternoonPopup.Key != "a-stock-afternoon-preopen-recommendation-1257-2026-06-23" || afternoonPopup.Period != "afternoon" {
		t.Fatalf("expected afternoon preopen popup to show independently, got %+v", afternoonPopup)
	}

	current = time.Date(2026, 6, 23, 13, 1, 0, 0, time.FixedZone("CST", 8*3600))
	secondAfternoonPopup := getPopup()
	if !secondAfternoonPopup.Show || secondAfternoonPopup.Key != "a-stock-afternoon-preopen-recommendation-1301-2026-06-23" || secondAfternoonPopup.Period != "afternoon" {
		t.Fatalf("expected second afternoon preopen popup to show independently, got %+v", secondAfternoonPopup)
	}
}

func TestAStockPageLoadsNewsAndRecommendations(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 16, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("date") != "2026-06-16" {
			t.Fatalf("unexpected market date: %s", r.URL.RawQuery)
		}
		for _, code := range []string{"002230", "688981"} {
			if !strings.Contains(r.URL.Query().Get("codes"), code) {
				t.Fatalf("expected market query to include %s, got %s", code, r.URL.RawQuery)
			}
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
					{"code": "603019", "date": "2026-06-15", "open": 38.00, "close": 38.40, "pct": 1.10},
					{"code": "603019", "date": "2026-06-16", "open": 38.50, "close": 39.20, "pct": 2.08},
					{"code": "601138", "date": "2026-06-15", "open": 24.00, "close": 24.30, "pct": 0.95},
					{"code": "601138", "date": "2026-06-16", "open": 24.40, "close": 24.80, "pct": 2.06},
					{"code": "002371", "date": "2026-06-15", "open": 320.00, "close": 325.00, "pct": 1.34},
					{"code": "002371", "date": "2026-06-16", "open": 326.00, "close": 330.00, "pct": 1.54},
					{"code": "603986", "date": "2026-06-15", "open": 102.00, "close": 103.50, "pct": 1.22},
					{"code": "603986", "date": "2026-06-16", "open": 104.00, "close": 106.00, "pct": 2.42},
				},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/recommendation-latest-dates" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.AStockRecommendationLatestDateListResult{
					StrategyDate: "2026-06-16",
					Period:       "morning",
					Items: []model.AStockRecommendationLatestDate{
						{Code: "002230", LatestDate: "2026-06-12"},
					},
				},
			})
			return
		}
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/stock-fund-flow-trend" {
			if r.URL.Query().Get("end_date") != "2026-06-16" || r.URL.Query().Get("indicator") != "今日" || r.URL.Query().Get("days") != "10" {
				t.Fatalf("unexpected stock fund flow trend query: %s", r.URL.RawQuery)
			}
			code := r.URL.Query().Get("code")
			items := []model.AStockStockFundFlow{}
			switch code {
			case "002230":
				items = []model.AStockStockFundFlow{
					{TradeDate: "2026-06-16", Indicator: "今日", Code: "002230", Name: "科大讯飞", MainNetInflow: 8000000},
					{TradeDate: "2026-06-15", Indicator: "今日", Code: "002230", Name: "科大讯飞", MainNetInflow: 4000000},
				}
			case "688981":
				items = []model.AStockStockFundFlow{
					{TradeDate: "2026-06-16", Indicator: "今日", Code: "688981", Name: "中芯国际", MainNetInflow: 3000000},
					{TradeDate: "2026-06-15", Indicator: "今日", Code: "688981", Name: "中芯国际", MainNetInflow: 2000000},
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.AStockStockFundFlowTrendResult{
					Items:     items,
					Total:     len(items),
					EndDate:   "2026-06-16",
					Indicator: "今日",
					Code:      code,
					Days:      10,
				},
			})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/auction" {
			if r.URL.Query().Get("date") != "2026-06-16" {
				t.Fatalf("unexpected auction date: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.AStockAuctionListResult{
					Date:        "2026-06-16",
					Page:        1,
					PageSize:    200,
					Total:       2,
					TotalAmount: 64170000,
					Items: []model.AStockAuctionAmount{
						{TradeDate: "2026-06-16", Code: "002230", Name: "科大讯飞", AuctionPrice: 10.60, AuctionVolume: 1800000, AuctionAmount: 19080000, Status: "ok"},
						{TradeDate: "2026-06-16", Code: "688981", Name: "中芯国际", AuctionPrice: 50.10, AuctionVolume: 900000, AuctionAmount: 45090000, Status: "ok"},
					},
				},
			})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("time_field") == "captured_at" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 window query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" || (r.URL.Query().Get("end") != "2026-06-16 09:26:59" && r.URL.Query().Get("end") != "2026-06-16 09:30:59") {
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
					{ID: 501, SourceType: "flash", Title: "AI 算力政策加码，科大讯飞盘中活跃", Summary: "人工智能产业链活跃", TagFlags: "0.002230", CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC)},
					{ID: 502, SourceType: "headline", Title: "半导体先进封装景气度提升，中芯国际成交放量", Summary: "芯片设备需求回暖", TagFlags: "1.688981", CapturedAt: time.Date(2026, 6, 16, 1, 12, 0, 0, time.UTC)},
				},
				Page:     1,
				PageSize: 200,
				Total:    2,
			},
		})
	}))
	defer content.Close()

	scheduler := newAStockTradingDayServer(t, true)
	defer scheduler.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-16&refresh_recommendations=1&filter_fund_flow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"金十快讯", "金十资讯", "人工智能", "半导体", "科大讯飞", "中芯国际", "财经新闻数", "集合竞价金额", "6417.00万", "90个交易日内过滤", "涨停过滤", "当日行情", "不过滤", "重新计算", "关闭90个交易日过滤", "关闭涨停过滤", "启用当日行情过滤", "昨日收盘价", "昨日涨跌幅", "30天涨跌幅", "60天涨跌幅", "现价", "今日涨跌幅", "5日资金动向", "+1200.00万", "+500.00万", "上午推荐", "下午推荐", "推荐窗口", "08:00-09:30", "重新生成上午推荐", "重新生成下午推荐", "重新生产晚间推荐", `name="action" value="backfill_window_news"`, `name="action" value="generate_morning_stock"`, `name="action" value="generate_afternoon_stock"`, `name="action" value="generate_evening_stock"`, `name="action" value="refresh_backtest"`, `name="action" value="recalculate"`, "2026-06-12 周五", "2026-06-15 周一", "今日", "astock-recommendation-table", "astock-popup-mask", "/a-stock/popup", "[[9,27],[9,30],[9,31]", "推荐排名前9股票", "002230 科大讯飞", `002230 科大讯飞<span class="astock-hotspot-date">（2026-06-12）</span>`, "688981 中芯国际", "10.50", "+1.25%", "+5.00%", "-12.50%", "10.90", "+3.81%", "50.20", "-0.60%", "50.60", "+0.80%", "002230 科大讯飞", "已回测"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected A股 page to contain %q, got %s", want, body)
		}
	}
	if strings.Index(body, "重新生成下午推荐") < 0 || strings.Index(body, "重新生产晚间推荐") <= strings.Index(body, "重新生成下午推荐") {
		t.Fatalf("expected evening generation action to render after afternoon generation action, got %s", body)
	}
	for _, notWant := range []string{"1. 002230 科大讯飞", "1. 688981 中芯国际"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected A股 hotspot stocks to omit display ranks %q, got %s", notWant, body)
		}
	}
	for _, notWant := range []string{"财经新闻来源统计", "新闻条数", "最近抓取", "08:00-09:30 财经新闻", "09:30-13:00 财经新闻", "08:00-09:30 2 / 09:30-13:00 0", "08:00-09:26:59 2 / 09:30-13:00 0", "astock-overview-meta"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected overview news count details to be absent, got %q in %s", notWant, body)
		}
	}
	for _, notWant := range []string{"AI 算力政策加码", "半导体先进封装景气度提升"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected A股 page not to render individual news title %q, got %s", notWant, body)
		}
	}
	if strings.Index(body, "2026-06-12 周五") < 0 || strings.Index(body, "2026-06-12 周五") > strings.Index(body, "顶部概览") {
		t.Fatalf("expected date tabs to render above overview, got %s", body)
	}
	if strings.Index(body, `<section><h2>推荐股票</h2>`) < strings.Index(body, "顶部概览") || strings.Index(body, `<section><h2>推荐股票</h2>`) > strings.Index(body, "操作区") {
		t.Fatalf("expected recommendation section between overview and actions, got %s", body)
	}
	for _, notWant := range []string{"消息回测", "推荐历史", "T+0 收益", "T+1 收益", "T+1 收盘价", "T+2 收盘价", "T+3 收盘价", "T+4 收盘价", "T+5 收盘价", "补股票名称", "补行情收益", "刷新全部回测", "已回测T+1"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected A股 page not to contain moved backtest content %q, got %s", notWant, body)
		}
	}
	if strings.Contains(body, "astock-history-card") || strings.Contains(body, "astock-history-stocks") {
		t.Fatalf("expected old recommendation history stock card to be removed, got %s", body)
	}
	if !strings.Contains(body, `/a-stock?date=2026-06-15&period=morning`) {
		t.Fatalf("expected top date tab to link previous day, got %s", body)
	}
	if !strings.Contains(body, `class="astock-up"`) || !strings.Contains(body, `class="astock-down"`) {
		t.Fatalf("expected A股 page to color上涨/下跌 percentages, got %s", body)
	}
}

func TestAStockRecommendationFundFlowFilterScoresAndReplenishes(t *testing.T) {
	trends := map[string][]float64{
		"300001": {70000000, 50000000},
		"300002": {40000000},
		"300003": {-7000000, 2000000},
		"300004": {-15000000, -10000000, -8000000},
		"300006": {0},
		"300007": {20000000},
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockAlgorithmSettingsTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/a-stock/stock-fund-flow-trend" {
			if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
				return
			}
			if r.URL.Path == "/api/v1/a-stock/sector-fund-flow-trend" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowTrendResult{})
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("end_date") != "2026-06-16" || r.URL.Query().Get("indicator") != "今日" || r.URL.Query().Get("days") != "10" {
			t.Fatalf("unexpected fund flow query: %s", r.URL.RawQuery)
		}
		code := normalizeAStockCode(r.URL.Query().Get("code"))
		values := trends[code]
		items := make([]model.AStockStockFundFlow, 0, len(values))
		for i, value := range values {
			items = append(items, model.AStockStockFundFlow{
				TradeDate:     fmt.Sprintf("2026-06-%02d", 16-i),
				Indicator:     "今日",
				Code:          code,
				MainNetInflow: value,
			})
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{
			Items:     items,
			Total:     len(items),
			EndDate:   "2026-06-16",
			Indicator: "今日",
			Code:      code,
			Days:      10,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	base := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "300001", Name: "强流入", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 2, Hotspot: "人工智能", Code: "300002", Name: "流入", HotspotScore: 90, MarketScore: 90, Reason: "base"},
		{Rank: 3, Hotspot: "半导体", Code: "300003", Name: "轻流出", HotspotScore: 80, MarketScore: 80, Reason: "base"},
		{Rank: 4, Hotspot: "半导体", Code: "300004", Name: "持续流出", HotspotScore: 70, MarketScore: 70, Reason: "base"},
		{Rank: 5, Hotspot: "新能源", Code: "300005", Name: "缺失", HotspotScore: 60, MarketScore: 60, Reason: "base"},
	}
	replacementPool := []aStockRecommendation{
		{Rank: 1, Hotspot: "半导体", Code: "300006", Name: "递补", HotspotScore: 50, MarketScore: 50, Reason: "replacement"},
		{Rank: 2, Hotspot: "半导体", Code: "300007", Name: "正资金递补", HotspotScore: 49, MarketScore: 49, Reason: "replacement"},
	}

	result := srv.applyAStockRecommendationFundFlowFilterWithCache("2026-06-16", base, replacementPool, nil, len(base), newAStockRequestCache(), false)
	if result.Filtered != 2 || result.Replenished != 2 || result.Missing != 1 || result.Shortfall {
		t.Fatalf("unexpected fund-flow filter result: %+v", result)
	}
	if got := formatAStockFundFlowFilterStatusWithStocks(result.Filtered, result.FilteredStocks, result.Replenished, result.Missing, result.Shortfall); !strings.Contains(got, "资金过滤 2 只（300003 轻流出、300004 持续流出）") {
		t.Fatalf("expected fund-flow status to include filtered stock details, got %q", got)
	}
	if len(result.Recommendations) != len(base) {
		t.Fatalf("expected recommendations to be replenished to %d, got %+v", len(base), result.Recommendations)
	}
	for _, code := range []string{"300003", "300004"} {
		if _, ok := aStockRecommendationCodeSet(result.Recommendations)[code]; ok {
			t.Fatalf("expected negative fund-flow stock %s to be filtered, got %+v", code, result.Recommendations)
		}
	}
	for _, code := range []string{"300006", "300007"} {
		if _, ok := aStockRecommendationCodeSet(result.Recommendations)[code]; !ok {
			t.Fatalf("expected replacement stock %s to be used, got %+v", code, result.Recommendations)
		}
	}

	strong := mustAStockRecommendationForTest(t, result.Recommendations, "300001")
	if strong.MarketScore != 210 || !strings.Contains(strong.Reason, "资金加分 110") {
		t.Fatalf("expected strong inflow bonus, got %+v", strong)
	}
	inflow := mustAStockRecommendationForTest(t, result.Recommendations, "300002")
	if inflow.MarketScore != 140 || !strings.Contains(inflow.Reason, "资金加分 50") {
		t.Fatalf("expected inflow bonus, got %+v", inflow)
	}
	zero := mustAStockRecommendationForTest(t, result.Recommendations, "300006")
	if zero.MarketScore != 50 || strings.Contains(zero.Reason, "资金加分") || strings.Contains(zero.Reason, "资金减分") {
		t.Fatalf("expected zero fund flow to keep stock without score change, got %+v", zero)
	}
	missing := mustAStockRecommendationForTest(t, result.Recommendations, "300005")
	if missing.FundFlow5D != "--" || missing.MarketScore != 60 {
		t.Fatalf("expected missing fund flow data to keep stock without score change, got %+v", missing)
	}
}

func TestAStockRecommendationFundFlowFilterAllowsAfternoonShortfallFallback(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockAlgorithmSettingsTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/a-stock/stock-fund-flow-trend" {
			if r.URL.Path == "/api/v1/a-stock/sector-fund-flows" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
				return
			}
			if r.URL.Path == "/api/v1/a-stock/sector-fund-flow-trend" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowTrendResult{})
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		code := normalizeAStockCode(r.URL.Query().Get("code"))
		writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{
			Items: []model.AStockStockFundFlow{
				{TradeDate: "2026-06-16", Indicator: "今日", Code: code, MainNetInflow: -15000000},
				{TradeDate: "2026-06-15", Indicator: "今日", Code: code, MainNetInflow: -10000000},
			},
			Total:     2,
			EndDate:   "2026-06-16",
			Indicator: "今日",
			Code:      code,
			Days:      10,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	base := []aStockRecommendation{{Rank: 1, Hotspot: "机器人", Code: "300001", Name: "资金流出", HotspotScore: 100, MarketScore: 100}}

	strict := srv.applyAStockRecommendationFundFlowFilterWithCache("2026-06-16", base, nil, nil, 1, newAStockRequestCache(), false)
	if len(strict.Recommendations) != 0 || strict.Filtered != 1 || !strict.Shortfall {
		t.Fatalf("expected strict fund-flow filter to drop hard-filtered candidate, got %+v", strict)
	}
	if got := formatAStockFundFlowFilterStatusWithStocks(strict.Filtered, strict.FilteredStocks, strict.Replenished, strict.Missing, strict.Shortfall); !strings.Contains(got, "资金过滤 1 只（300001 资金流出）") {
		t.Fatalf("expected strict fund-flow status to include filtered stock detail, got %q", got)
	}

	fallback := srv.applyAStockRecommendationFundFlowFilterWithCache("2026-06-16", base, nil, nil, 1, newAStockRequestCache(), true)
	if len(fallback.Recommendations) != 1 || fallback.Recommendations[0].Code != "300001" || fallback.Filtered != 0 || fallback.Shortfall {
		t.Fatalf("expected afternoon fallback to fill target with hard-filtered candidate, got %+v", fallback)
	}
	if fallback.Recommendations[0].FundFlow5D == "" || !strings.Contains(fallback.Recommendations[0].Reason, "资金") {
		t.Fatalf("expected fallback recommendation to keep fund-flow scoring detail, got %+v", fallback.Recommendations[0])
	}
}

func TestAStockRecommendationGenerateIgnoreFundFlowKeepsNegativeFundFlowRecommendation(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{
			"items": []map[string]any{
				{"code": "002230", "date": "2026-06-15", "open": 10.00, "close": 10.00, "pct": 0.0},
				{"code": "002230", "date": "2026-06-16", "open": 10.10, "close": 10.20, "pct": 2.0},
			},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	fundFlowCalls := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			if r.URL.Query().Get("time_field") == "captured_at" {
				writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          9901,
					SourceType:  "flash",
					Title:       "科大讯飞盘前活跃",
					Summary:     "AI 人工智能算力需求增长",
					PublishTime: "2026-06-16 09:26:30",
					TagFlags:    "0.002230",
					CapturedAt:  time.Date(2026, 6, 16, 1, 26, 30, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date:  "2026-06-16",
				Items: []model.AStockAuctionAmount{{TradeDate: "2026-06-16", Code: "002230", Name: "科大讯飞", AuctionAmount: 10000000, AuctionVolume: 1000000}},
			})
		case "/api/v1/a-stock/recommendation-latest-dates":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/a-stock/stock-fund-flow-trend":
			fundFlowCalls++
			writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{
				Items: []model.AStockStockFundFlow{{TradeDate: "2026-06-16", Indicator: "今日", Code: r.URL.Query().Get("code"), MainNetInflow: -1}},
				Total: 1, EndDate: "2026-06-16", Indicator: "今日", Code: r.URL.Query().Get("code"), Days: 5,
			})
		case "/api/v1/internal/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Inserted: 1})
		case "/api/v1/internal/a-stock/recommendation-shadow-snapshots":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Inserted: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/recommendations/generate?date=2026-06-16&period=morning&phase=final&ignore_recent=1&ignore_fund_flow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockRecommendationGenerate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected generate 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data aStockRecommendationGenerateResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	if envelope.Data.FundFlowFilterEnabled || envelope.Data.FundFlowFiltered != 0 || fundFlowCalls != 0 {
		t.Fatalf("expected ignore_fund_flow to skip fund-flow filtering, data=%+v fundFlowCalls=%d", envelope.Data, fundFlowCalls)
	}
	if envelope.Data.RecommendationCount == 0 {
		t.Fatalf("expected recommendation to remain when fund-flow filter is disabled, data=%+v", envelope.Data)
	}
}

func TestAStockTestPageRendersSimulationForm(t *testing.T) {
	srv := NewServer(config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/test", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockTestPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected test page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"模拟生成，不写快照/数据库",
		`name="simulate" value="1"`,
		"模拟结果",
		"/a-stock/test",
		"body[data-page='a-stock-test'] main{max-width:none;width:98vw;box-sizing:border-box",
		"body[data-page='a-stock-test'] section{width:100%;box-sizing:border-box}",
		"body[data-page='a-stock-test'] .astock-recommendation-table th:nth-child(-n+10),body[data-page='a-stock-test'] .astock-recommendation-table td:nth-child(-n+10){white-space:nowrap;word-break:keep-all}",
		"body[data-page='a-stock-test'] .astock-score-table th:nth-child(3),body[data-page='a-stock-test'] .astock-score-table td:nth-child(3){width:33.6%}",
		"body[data-page='a-stock-test'] [data-astock-simulation-form='1'] button[type='submit']:disabled{background:#9aa0a6",
		"模拟生成中...",
		`button.setAttribute("aria-disabled","true")`,
		`window.addEventListener("pageshow"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected test page to contain %q, got body=%s", want, body)
		}
	}
}

func TestAStockTestPageSimulationDoesNotWriteRecommendationData(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(writeAStockTestMarketBars))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	writeCount := 0
	content := newAStockSimulationContentServerForTest(t, &writeCount)
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock/test?date=2026-06-16&period=morning&phase=final&simulate=1&ignore_recent=1&ignore_fund_flow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockTestPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected simulated test page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"模拟生成，不写快照/数据库", "推荐股票", "002230", "科大讯飞"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected simulated test page to contain %q, got body=%s", want, body)
		}
	}
	if writeCount != 0 {
		t.Fatalf("expected simulation page not to write recommendation data, got %d writes", writeCount)
	}
}

func TestAStockRecommendationGenerateDryRunDoesNotWriteRecommendationData(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(writeAStockTestMarketBars))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	writeCount := 0
	content := newAStockSimulationContentServerForTest(t, &writeCount)
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/recommendations/generate?date=2026-06-16&period=morning&phase=final&ignore_recent=1&ignore_fund_flow=1&dry_run=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockRecommendationGenerate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected dry-run generate 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data aStockRecommendationGenerateResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run generate response: %v", err)
	}
	if !envelope.Data.DryRun {
		t.Fatalf("expected dry_run=true in response, got %+v", envelope.Data)
	}
	if envelope.Data.RecommendationCount == 0 {
		t.Fatalf("expected dry-run generate recommendation, got %+v", envelope.Data)
	}
	if writeCount != 0 {
		t.Fatalf("expected dry-run generate not to write recommendation data, got %d writes", writeCount)
	}
}

func TestAStockRecommendationGenerateSkipShadowDoesNotWriteT1Shadow(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(writeAStockTestMarketBars))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	counts := map[string]int{}
	content := newAStockGenerateWriteCountingContentServerForTest(t, counts)
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/recommendations/generate?date=2026-06-16&period=morning&phase=preopen&ignore_recent=1&ignore_fund_flow=1&skip_shadow=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockRecommendationGenerate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected skip-shadow generate 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data aStockRecommendationGenerateResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode skip-shadow generate response: %v", err)
	}
	if !envelope.Data.SkipShadow {
		t.Fatalf("expected skip_shadow=true in response, got %+v", envelope.Data)
	}
	if counts["snapshot"] == 0 {
		t.Fatalf("expected normal recommendation snapshot to be written, counts=%v", counts)
	}
	if counts["shadow"] != 0 {
		t.Fatalf("expected T1 shadow snapshot to be skipped, counts=%v", counts)
	}

	for key := range counts {
		counts[key] = 0
	}
	req = httptest.NewRequest(http.MethodPost, "/internal/a-stock/recommendations/generate?date=2026-06-16&period=morning&phase=preopen&ignore_recent=1&ignore_fund_flow=1", nil)
	rr = httptest.NewRecorder()
	srv.handleAStockRecommendationGenerate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected default generate 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if counts["shadow"] < 2 {
		t.Fatalf("expected default generation to write T1 and auction-strength shadow snapshots, counts=%v", counts)
	}
}

func newAStockGenerateWriteCountingContentServerForTest(t *testing.T, counts map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			if r.URL.Query().Get("time_field") == "captured_at" {
				writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          9901,
					SourceType:  "flash",
					Title:       "科大讯飞盘前活跃",
					Summary:     "AI 人工智能算力需求增长",
					PublishTime: "2026-06-16 09:26:30",
					TagFlags:    "0.002230",
					CapturedAt:  time.Date(2026, 6, 16, 1, 26, 30, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date:  "2026-06-16",
				Items: []model.AStockAuctionAmount{{TradeDate: "2026-06-16", Code: "002230", Name: "科大讯飞", AuctionAmount: 10000000, AuctionVolume: 1000000}},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendation-latest-dates":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations":
			counts["snapshot"]++
			writeEnvelope(w, http.StatusOK, "ok", map[string]any{"updated": 1})
		case "/api/v1/internal/a-stock/recommendation-selections":
			counts["selections"]++
			writeEnvelope(w, http.StatusOK, "ok", map[string]any{"updated": 1})
		case "/api/v1/internal/a-stock/recommendation-shadow-snapshots":
			counts["shadow"]++
			writeEnvelope(w, http.StatusOK, "ok", map[string]any{"updated": 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
}

func newAStockSimulationContentServerForTest(t *testing.T, writeCount *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			if r.URL.Query().Get("time_field") == "captured_at" {
				writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{Items: []model.Item{}, Page: 1, PageSize: 200, Total: 0})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          9901,
					SourceType:  "flash",
					Title:       "科大讯飞盘前活跃",
					Summary:     "AI 人工智能算力需求增长",
					PublishTime: "2026-06-16 09:26:30",
					TagFlags:    "0.002230",
					CapturedAt:  time.Date(2026, 6, 16, 1, 26, 30, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			})
		case "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date:  "2026-06-16",
				Items: []model.AStockAuctionAmount{{TradeDate: "2026-06-16", Code: "002230", Name: "科大讯飞", AuctionAmount: 10000000, AuctionVolume: 1000000}},
			})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendation-latest-dates":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationLatestDateListResult{Items: []model.AStockRecommendationLatestDate{}})
		case "/api/v1/a-stock/holdings/summary":
			writeEnvelope(w, http.StatusOK, "ok", model.StockInstitutionHoldingSummary{})
		case "/api/v1/internal/a-stock/recommendations", "/api/v1/internal/a-stock/recommendation-selections", "/api/v1/internal/a-stock/recommendation-shadow-snapshots":
			*writeCount++
			writeEnvelope(w, http.StatusOK, "ok", map[string]any{"updated": 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
}

func mustAStockRecommendationForTest(t *testing.T, recommendations []aStockRecommendation, code string) aStockRecommendation {
	t.Helper()
	code = normalizeAStockCode(code)
	for _, rec := range recommendations {
		if normalizeAStockCode(rec.Code) == code {
			return rec
		}
	}
	t.Fatalf("expected recommendation code %s in %+v", code, recommendations)
	return aStockRecommendation{}
}

func mustAStockScoreComponentForTest(t *testing.T, rec aStockRecommendation, label string) aStockRecommendationScoreComponent {
	t.Helper()
	for _, component := range aStockRecommendationScoreBreakdown(rec) {
		if component.Label == label {
			return component
		}
	}
	t.Fatalf("expected score component %q in %+v", label, aStockRecommendationScoreBreakdown(rec))
	return aStockRecommendationScoreComponent{}
}

func aStockMomentumBarsForTest(t *testing.T, code string, endDate string, closes []float64, amounts []float64) []aStockMarketBar {
	t.Helper()
	end, err := time.ParseInLocation("2006-01-02", endDate, aStockLocation())
	if err != nil {
		t.Fatalf("parse end date: %v", err)
	}
	start := end.AddDate(0, 0, -len(closes)+1)
	bars := make([]aStockMarketBar, 0, len(closes))
	for i, closeValue := range closes {
		if closeValue <= 0 {
			t.Fatalf("close must be positive at %d", i)
		}
		open := closeValue
		prevClose := closeValue
		if i > 0 {
			prevClose = closes[i-1]
			open = prevClose
		}
		amount := 100000000.0
		if i < len(amounts) && amounts[i] > 0 {
			amount = amounts[i]
		}
		pct := 0.0
		if i > 0 && prevClose > 0 {
			pct = (closeValue/prevClose - 1) * 100
		}
		high := closeValue
		if open > high {
			high = open
		}
		low := closeValue
		if open < low {
			low = open
		}
		bars = append(bars, aStockMarketBar{
			Code:       code,
			Date:       start.AddDate(0, 0, i).Format("2006-01-02"),
			Open:       open,
			High:       high * 1.005,
			Low:        low * 0.995,
			Close:      closeValue,
			Pct:        pct,
			Volume:     amount / closeValue,
			Amount:     amount,
			EntryPrice: open,
		})
	}
	return bars
}

func aStockMomentumAmountsForTest(size int, lastAmount float64) []float64 {
	amounts := make([]float64, size)
	for i := range amounts {
		amounts[i] = 100000000
	}
	if size > 0 {
		amounts[size-1] = lastAmount
	}
	return amounts
}

func aStockMomentumHasSignalForTest(signals []aStockMomentumSignal, name string) bool {
	for _, signal := range signals {
		if signal.Name == name {
			return true
		}
	}
	return false
}

func TestAStockPageBlocksRecommendationsOnNonTradingDay(t *testing.T) {
	scheduler := newAStockTradingDayServer(t, false)
	defer scheduler.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if handleEmptyAStockAuctionTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.ItemListResult{
				Items: []model.Item{{
					ID:         700,
					SourceType: "flash",
					Title:      "AI infrastructure policy update",
					Summary:    "AI industry chain activity",
					TagFlags:   "0.002230",
					CapturedAt: time.Date(2026, 6, 19, 1, 5, 0, 0, time.UTC),
				}},
				Page: 1, PageSize: 200, Total: 1,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	req := httptest.NewRequest(http.MethodGet, "/a-stock?date=2026-06-19&refresh_recommendations=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "该日 A 股休市，不生成股票推荐") {
		t.Fatalf("expected non-trading day message, got %s", body)
	}
	recommendationStart := strings.Index(body, `<section><h2>推荐股票</h2>`)
	actionStart := strings.Index(body, `<section><h2>操作区</h2>`)
	if recommendationStart < 0 || actionStart <= recommendationStart {
		t.Fatalf("expected recommendation section before actions, got %s", body)
	}
	if strings.Contains(body[recommendationStart:actionStart], "002230") {
		t.Fatalf("expected no stock recommendation code on non-trading day, got %s", body[recommendationStart:actionStart])
	}
}

func TestAStockTradingDayFallsBackWhenSchedulerUnavailable(t *testing.T) {
	deadScheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadScheduler.Close()
	srv := NewServer(config.Config{SchedulerURL: deadScheduler.URL})

	tradingStatus, err := srv.loadAStockTradingDayStatus("2026-06-15")
	if err != nil {
		t.Fatalf("expected local trading-day fallback, got error %v", err)
	}
	if !tradingStatus.IsTradingDay || tradingStatus.Source != "portal_local_calendar_fallback" || tradingStatus.Reason != "trading_day" {
		t.Fatalf("expected 2026-06-15 to fall back as trading day, got %+v", tradingStatus)
	}
	blocked, message, reason := srv.aStockRecommendationBlockedStatus("2026-06-15")
	if blocked || message != "" || reason != "trading_day" {
		t.Fatalf("expected fallback trading day not blocked, got blocked=%v message=%q reason=%q", blocked, message, reason)
	}

	holidayStatus, err := srv.loadAStockTradingDayStatus("2026-06-19")
	if err != nil {
		t.Fatalf("expected local holiday fallback, got error %v", err)
	}
	if holidayStatus.IsTradingDay || holidayStatus.Reason != "market_closed" || holidayStatus.LatestTradingDay != "2026-06-18" || holidayStatus.NextTradingDay != "2026-06-22" {
		t.Fatalf("expected 2026-06-19 to fall back as holiday, got %+v", holidayStatus)
	}
}

func TestAStockRecommendationActionBlockedOnNonTradingDay(t *testing.T) {
	for _, action := range []string{"backfill_window_news", "crawl"} {
		t.Run(action, func(t *testing.T) {
			scheduler := newAStockTradingDayServer(t, false)
			defer scheduler.Close()
			crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("crawler should not be called on non-trading day")
			}))
			defer crawler.Close()

			srv := NewServer(config.Config{SchedulerURL: scheduler.URL, CrawlerURL: crawler.URL})
			req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader("date=2026-06-19&period=morning&action="+action))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()
			srv.handleAStockPage(rr, req, map[string]any{"id": 1})
			if rr.Code != http.StatusSeeOther {
				t.Fatalf("expected redirect, got %d", rr.Code)
			}
			loc, _ := url.QueryUnescape(rr.Header().Get("Location"))
			if !strings.Contains(loc, "该日 A 股休市，不生成股票推荐") {
				t.Fatalf("expected non-trading day redirect message, got %q", loc)
			}
		})
	}
}

func TestAStockRecommendationHistoryRendersDateTabs(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var b strings.Builder
	renderAStockRecommendationHistoryTabs(&b, "2026-06-16", "afternoon", false, false, false, false)

	body := b.String()
	for _, want := range []string{
		"推荐历史",
		"2026-06-12 周五",
		"2026-06-15 周一",
		"2026-06-16 周二",
		"2026-06-17 周三",
		"今日",
		`/a-stock?date=2026-06-16&period=afternoon`,
		`/a-stock?date=2026-06-15&period=afternoon`,
		`/a-stock?date=2026-06-17&period=afternoon`,
		`/a-stock?date=2026-06-18&period=afternoon`,
		`data-preserve-scroll="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected history tabs to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "astock-history-card") || strings.Contains(body, "山东黄金") {
		t.Fatalf("expected history stock table/card to be removed, got %s", body)
	}
	if strings.Index(body, "2026-06-12 周五") > strings.Index(body, "今日") {
		t.Fatalf("expected history tabs to render oldest date first, got %s", body)
	}
}

func TestAStockRecommendationHistoryAppendsTodayForPastDate(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 30, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var b strings.Builder
	renderAStockRecommendationHistoryTabs(&b, "2026-06-17", "morning", true, false, false, true)

	body := b.String()
	for _, want := range []string{
		"2026-06-17 周三",
		`/a-stock?date=2026-06-30&period=morning&ignore_recent=1&filter_today_market=1`,
		`>今日</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected past history tabs to contain %q, got %s", want, body)
		}
	}
	if strings.Count(body, `>今日</a>`) != 1 {
		t.Fatalf("expected exactly one today tab, got %s", body)
	}
	if strings.LastIndex(body, `>今日</a>`) < strings.LastIndex(body, `2026-06-17 周三`) {
		t.Fatalf("expected today tab to render to the right of the selected past date, got %s", body)
	}
}

func TestAStockDatePeriodTabsCanRenderWithoutHeading(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var b strings.Builder
	renderAStockDatePeriodTabs(&b, "2026-06-18", "morning", true, false, false, false, false)

	body := b.String()
	for _, want := range []string{
		"2026-06-16 周二",
		"2026-06-17 周三",
		"今日",
		`/a-stock?date=2026-06-18&period=morning&ignore_recent=1`,
		`class="astock-tab active"`,
		`data-preserve-scroll="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected top date tabs to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "推荐历史") {
		t.Fatalf("expected top date tabs to render without history heading, got %s", body)
	}
	if strings.Contains(body, "后一日") || strings.Contains(body, "后两日") {
		t.Fatalf("expected latest trading day tabs to omit future labels, got %s", body)
	}
}

func TestAStockDateTabLabelUsesTodayAndWeekday(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))

	if got := aStockDateTabLabel("2026-06-18"); got != "今日" {
		t.Fatalf("expected today label, got %q", got)
	}
	if got := aStockDateTabLabel("2026-06-16"); got != "2026-06-16 周二" {
		t.Fatalf("expected weekday label for prior trading day, got %q", got)
	}
}

func TestAStockRecommendationHistoryActionsUseSelectedPeriod(t *testing.T) {
	var b strings.Builder
	renderAStockRecommendationHistoryActions(&b, "2026-06-16", "afternoon", false, false, false, false)
	body := b.String()
	for _, want := range []string{
		`name="date" value="2026-06-16"`,
		`name="period" value="morning"`,
		`name="period" value="afternoon"`,
		`name="action" value="backfill_window_news"`,
		`name="action" value="generate_morning_stock"`,
		`name="action" value="generate_afternoon_stock"`,
		`name="action" value="backfill_auction"`,
		`name="action" value="repair_stock_names"`,
		`name="action" value="refresh_current_backtest"`,
		`name="action" value="refresh_backtest"`,
		`data-preserve-scroll="1"`,
		`href="/a-stock?date=2026-06-16&amp;period=afternoon&amp;ignore_recent=1"`,
		"补抓上午新闻",
		"重新生成上午推荐",
		"补抓下午新闻",
		"重新生成下午推荐",
		"关闭90个交易日过滤",
		"补录集合竞价",
		"补股票名称",
		"补行情收益",
		"刷新全部回测",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected history actions to contain %q, got %s", want, body)
		}
	}
	if strings.Index(body, "补行情收益") < 0 || strings.Index(body, "刷新全部回测") < 0 || strings.Index(body, "补行情收益") > strings.Index(body, "刷新全部回测") {
		t.Fatalf("expected 补行情收益 button before 刷新全部回测, got %s", body)
	}
	if strings.Index(body, "补股票名称") < 0 || strings.Index(body, "补行情收益") < 0 || strings.Index(body, "补股票名称") > strings.Index(body, "补行情收益") {
		t.Fatalf("expected 补股票名称 button before 补行情收益, got %s", body)
	}
}

func TestAStockRecommendationHistoryHelpersCanUseBacktestPath(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var tabs strings.Builder
	renderAStockRecommendationHistoryTabsForPath(&tabs, "/a-stock/backtest", "2026-06-16", "afternoon", true, false, false, true)
	tabsBody := tabs.String()
	for _, want := range []string{
		`/a-stock/backtest?date=2026-06-16&period=afternoon&ignore_recent=1&filter_today_market=1`,
		`/a-stock/backtest?date=2026-06-17&period=afternoon&ignore_recent=1&filter_today_market=1`,
	} {
		if !strings.Contains(tabsBody, want) {
			t.Fatalf("expected backtest history tabs to contain %q, got %s", want, tabsBody)
		}
	}
	if strings.Contains(tabsBody, `/a-stock?date=`) {
		t.Fatalf("expected backtest history tabs to avoid default A股 path, got %s", tabsBody)
	}

	var actions strings.Builder
	renderAStockRecommendationHistoryActionsForPath(&actions, "/a-stock/backtest", "2026-06-16", "afternoon", true, false, false, true)
	actionsBody := actions.String()
	for _, want := range []string{
		`action="/a-stock/backtest"`,
		`href="/a-stock/backtest?date=2026-06-16&amp;period=afternoon&amp;filter_today_market=1"`,
		`name="ignore_recent" value="1"`,
		`name="filter_today_market" value="1"`,
	} {
		if !strings.Contains(actionsBody, want) {
			t.Fatalf("expected backtest history actions to contain %q, got %s", want, actionsBody)
		}
	}
}

func TestAStockIgnoreRecentStatePersistsInHistoryNavigation(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var tabs strings.Builder
	renderAStockRecommendationHistoryTabs(&tabs, "2026-06-16", "morning", true, false, false, false)
	tabsBody := tabs.String()
	for _, want := range []string{
		`/a-stock?date=2026-06-16&period=morning&ignore_recent=1`,
		`/a-stock?date=2026-06-17&period=morning&ignore_recent=1`,
	} {
		if !strings.Contains(tabsBody, want) {
			t.Fatalf("expected history tab to preserve ignore_recent %q, got %s", want, tabsBody)
		}
	}

	var actions strings.Builder
	renderAStockRecommendationHistoryActions(&actions, "2026-06-16", "morning", true, false, false, false)
	actionsBody := actions.String()
	if !strings.Contains(actionsBody, `name="ignore_recent" value="1"`) {
		t.Fatalf("expected history actions to preserve ignore_recent, got %s", actionsBody)
	}
	if !strings.Contains(actionsBody, "启用90个交易日过滤") || !strings.Contains(actionsBody, `href="/a-stock?date=2026-06-16&amp;period=morning"`) {
		t.Fatalf("expected history actions to offer enabling 90-trading-day filter, got %s", actionsBody)
	}
}

func TestAStockIgnoreLimitUpStatePersistsInNavigationAndActions(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var tabs strings.Builder
	renderAStockRecommendationHistoryTabs(&tabs, "2026-06-16", "afternoon", false, true, false, false)
	tabsBody := tabs.String()
	for _, want := range []string{
		`/a-stock?date=2026-06-16&period=afternoon&ignore_limit_up=1`,
		`/a-stock?date=2026-06-17&period=afternoon&ignore_limit_up=1`,
	} {
		if !strings.Contains(tabsBody, want) {
			t.Fatalf("expected history tab to preserve ignore_limit_up %q, got %s", want, tabsBody)
		}
	}

	var actions strings.Builder
	renderAStockRecommendationHistoryActions(&actions, "2026-06-16", "afternoon", false, true, false, false)
	actionsBody := actions.String()
	if !strings.Contains(actionsBody, `name="ignore_limit_up" value="1"`) {
		t.Fatalf("expected history actions to preserve ignore_limit_up, got %s", actionsBody)
	}
	if !strings.Contains(actionsBody, `href="/a-stock?date=2026-06-16&amp;period=afternoon&amp;ignore_recent=1&amp;ignore_limit_up=1"`) {
		t.Fatalf("expected 90-trading-day filter toggle to preserve ignore_limit_up, got %s", actionsBody)
	}
}

func TestAStockTodayMarketFilterStatePersistsInNavigationAndActions(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*3600)))
	var tabs strings.Builder
	renderAStockRecommendationHistoryTabs(&tabs, "2026-06-16", "morning", false, false, false, true)
	tabsBody := tabs.String()
	for _, want := range []string{
		`/a-stock?date=2026-06-16&period=morning&filter_today_market=1`,
		`/a-stock?date=2026-06-17&period=morning&filter_today_market=1`,
	} {
		if !strings.Contains(tabsBody, want) {
			t.Fatalf("expected history tab to preserve filter_today_market %q, got %s", want, tabsBody)
		}
	}

	var actions strings.Builder
	renderAStockRecommendationHistoryActions(&actions, "2026-06-16", "morning", false, false, false, true)
	actionsBody := actions.String()
	if !strings.Contains(actionsBody, `name="filter_today_market" value="1"`) {
		t.Fatalf("expected history actions to preserve filter_today_market, got %s", actionsBody)
	}
	if !strings.Contains(actionsBody, `href="/a-stock?date=2026-06-16&amp;period=morning&amp;ignore_recent=1&amp;filter_today_market=1"`) {
		t.Fatalf("expected 90-trading-day filter toggle to preserve filter_today_market, got %s", actionsBody)
	}
}

func TestAStockOverviewBacktestStatusIncludesFilterReasons(t *testing.T) {
	got := aStockOverviewBacktestStatus(aStockContext{
		BacktestStatus:         "已回测 3/3",
		RecentFiltered:         2,
		SameDayMorningFiltered: 1,
		LimitUpFiltered:        3,
	})
	for _, want := range []string{"已回测 3/3", "90个交易日内重复过滤股票 2", "过滤上午同股票/热点/日内名额 1", "涨停过滤股票 3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected overview status to contain %q, got %q", want, got)
		}
	}
}

func TestAStockRecentFilterStatusIncludesFilteredStockDetails(t *testing.T) {
	result := filterRecentAStockRecommendationsWithReplenishment(
		[]aStockRecommendation{
			{Rank: 1, Code: "000001", Name: "重复一号", Hotspot: "人工智能"},
			{Rank: 2, Code: "000002", Name: "保留二号", Hotspot: "人工智能"},
			{Rank: 3, Code: "000003", Name: "重复三号", Hotspot: "半导体"},
		},
		nil,
		map[string]struct{}{"000001": {}, "000003": {}},
		3,
	)

	if result.Filtered != 2 || len(result.Recommendations) != 1 {
		t.Fatalf("expected two recent recommendations to be filtered, got %+v", result)
	}
	got := formatAStockRecentReplenishmentStatusWithStocks(result.Filtered, result.FilteredStocks, result.Replenished, result.Shortfall)
	for _, want := range []string{"90个交易日内重复过滤 2 只", "（000001 重复一号、000003 重复三号）"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected recent filter status to contain %q, got %q", want, got)
		}
	}
}

func TestAStockOverviewLimitUpFilterToggle(t *testing.T) {
	var enabled strings.Builder
	writeAStockOverviewLimitUpFilterCell(&enabled, aStockContext{
		Date:                 "2026-06-22",
		Period:               "afternoon",
		NewsPage:             2,
		IgnoreRecent:         true,
		LimitUpFilterEnabled: true,
		LimitUpFiltered:      9,
	})
	enabledBody := enabled.String()
	for _, want := range []string{"涨停过滤", "已过滤 9", "关闭涨停过滤", `<form class="astock-filter-toggle-form" method="get" action="/a-stock">`, `name="date" value="2026-06-22"`, `name="period" value="afternoon"`, `name="news_page" value="2"`, `name="ignore_recent" value="1"`, `name="ignore_limit_up" value="1"`, `<button class="astock-filter-toggle" type="submit"`} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled limit-up filter cell to contain %q, got %s", want, enabledBody)
		}
	}

	var disabled strings.Builder
	writeAStockOverviewLimitUpFilterCell(&disabled, aStockContext{
		Date:          "2026-06-22",
		Period:        "afternoon",
		IgnoreLimitUp: true,
	})
	disabledBody := disabled.String()
	for _, want := range []string{"涨停过滤", "已关闭", "启用涨停过滤", `<form class="astock-filter-toggle-form" method="get" action="/a-stock">`, `name="date" value="2026-06-22"`, `name="period" value="afternoon"`, `<button class="astock-filter-toggle" type="submit"`} {
		if !strings.Contains(disabledBody, want) {
			t.Fatalf("expected disabled limit-up filter cell to contain %q, got %s", want, disabledBody)
		}
	}
	if strings.Contains(disabledBody, "ignore_limit_up=1") {
		t.Fatalf("expected enabled link to remove ignore_limit_up flag, got %s", disabledBody)
	}

	var morning strings.Builder
	writeAStockOverviewLimitUpFilterCell(&morning, aStockContext{Date: "2026-06-22", Period: "morning"})
	morningBody := morning.String()
	if !strings.Contains(morningBody, "不适用") || strings.Contains(morningBody, "astock-filter-toggle-form") || strings.Contains(morningBody, "<button") {
		t.Fatalf("expected morning limit-up filter cell to be non-interactive, got %s", morningBody)
	}
}

func TestAStockOverviewFundFlowFilterToggleUsesExplicitState(t *testing.T) {
	var enabled strings.Builder
	writeAStockOverviewFundFlowFilterCell(&enabled, aStockContext{
		Date:             "2026-06-22",
		Period:           "morning",
		NewsPage:         2,
		IgnoreRecent:     true,
		IgnoreLimitUp:    true,
		FundFlowFiltered: 3,
	})
	enabledBody := enabled.String()
	for _, want := range []string{"资金过滤", "已过滤 3", "关闭资金过滤", `name="ignore_fund_flow" value="1"`, `name="ignore_recent" value="1"`, `name="ignore_limit_up" value="1"`} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled fund-flow filter cell to contain %q, got %s", want, enabledBody)
		}
	}
	if strings.Contains(enabledBody, `name="filter_fund_flow" value="1"`) {
		t.Fatalf("expected enabled fund-flow toggle to request disabled state only, got %s", enabledBody)
	}

	var disabled strings.Builder
	writeAStockOverviewFundFlowFilterCell(&disabled, aStockContext{
		Date:           "2026-06-22",
		Period:         "morning",
		IgnoreFundFlow: true,
	})
	disabledBody := disabled.String()
	for _, want := range []string{"资金过滤", "已关闭", "启用资金过滤", `name="filter_fund_flow" value="1"`} {
		if !strings.Contains(disabledBody, want) {
			t.Fatalf("expected disabled fund-flow filter cell to contain %q, got %s", want, disabledBody)
		}
	}
	if strings.Contains(disabledBody, `name="ignore_fund_flow" value="1"`) {
		t.Fatalf("expected disabled fund-flow toggle to request enabled state only, got %s", disabledBody)
	}
}

func TestAStockOverviewTodayMarketFilterToggleAndRecalculate(t *testing.T) {
	var disabled strings.Builder
	writeAStockOverviewTodayMarketFilterCell(&disabled, aStockContext{
		Date:               "2026-06-24",
		Period:             "morning",
		NewsPage:           2,
		IgnoreRecent:       true,
		IgnoreLimitUp:      true,
		NoTodayMarketCount: 4,
	})
	disabledBody := disabled.String()
	for _, want := range []string{"当日行情", "允许缺失 4", "启用当日行情过滤", `name="date" value="2026-06-24"`, `name="period" value="morning"`, `name="news_page" value="2"`, `name="ignore_recent" value="1"`, `name="ignore_limit_up" value="1"`, `name="filter_today_market" value="1"`} {
		if !strings.Contains(disabledBody, want) {
			t.Fatalf("expected disabled today-market filter cell to contain %q, got %s", want, disabledBody)
		}
	}

	var enabled strings.Builder
	writeAStockOverviewTodayMarketFilterCell(&enabled, aStockContext{
		Date:                     "2026-06-24",
		Period:                   "morning",
		TodayMarketFilterEnabled: true,
		NoTodayMarketCount:       4,
	})
	enabledBody := enabled.String()
	for _, want := range []string{"当日行情", "已过滤 4", "关闭当日行情过滤"} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled today-market filter cell to contain %q, got %s", want, enabledBody)
		}
	}
	if strings.Contains(enabledBody, `name="filter_today_market" value="1"`) {
		t.Fatalf("expected enabled filter toggle to remove filter_today_market flag, got %s", enabledBody)
	}

	var recalc strings.Builder
	writeAStockOverviewRecalculateCell(&recalc, aStockContext{
		Date:                     "2026-06-24",
		Period:                   "morning",
		IgnoreRecent:             true,
		IgnoreLimitUp:            true,
		TodayMarketFilterEnabled: true,
	})
	recalcBody := recalc.String()
	for _, want := range []string{"重新计算", `method="post"`, `name="action" value="recalculate"`, `name="ignore_recent" value="1"`, `name="ignore_limit_up" value="1"`, `name="filter_today_market" value="1"`} {
		if !strings.Contains(recalcBody, want) {
			t.Fatalf("expected recalculate cell to contain %q, got %s", want, recalcBody)
		}
	}
	if !strings.Contains(recalcBody, `class="astock-overview-recalculate"`) {
		t.Fatalf("expected recalculate cell width class, got %s", recalcBody)
	}
}

func TestFilterRecentAStockRecommendationsDropsPast5DayCodes(t *testing.T) {
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

func TestFilterRecentAStockRecommendationsReplenishesSameHotspotThenGlobal(t *testing.T) {
	base := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞", MarketScore: 210},
		{Rank: 2, Hotspot: "人工智能", Code: "603019", Name: "中科曙光", MarketScore: 200},
		{Rank: 3, Hotspot: "半导体", Code: "688981", Name: "中芯国际", MarketScore: 190},
	}
	replacementPool := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞", MarketScore: 210},
		{Rank: 2, Hotspot: "人工智能", Code: "300024", Name: "机器人", MarketScore: 180},
		{Rank: 3, Hotspot: "半导体", Code: "688981", Name: "中芯国际", MarketScore: 190},
		{Rank: 4, Hotspot: "军工航天", Code: "600760", Name: "中航沈飞", MarketScore: 260},
	}

	result := filterRecentAStockRecommendationsWithReplenishment(base, replacementPool, map[string]struct{}{
		"002230": {},
		"688981": {},
	}, len(base))

	if result.Filtered != 2 || result.Replenished != 2 || result.Shortfall {
		t.Fatalf("unexpected replenishment stats: %+v", result)
	}
	gotCodes := aStockRecommendationCodeSet(result.Recommendations)
	for _, code := range []string{"603019", "300024", "600760"} {
		if _, ok := gotCodes[code]; !ok {
			t.Fatalf("expected replenished recommendations to contain %s, got %+v", code, result.Recommendations)
		}
	}
	for _, code := range []string{"002230", "688981"} {
		if _, ok := gotCodes[code]; ok {
			t.Fatalf("expected recent code %s to stay filtered, got %+v", code, result.Recommendations)
		}
	}
}

func TestFilterRecentAStockRecommendationsShortfallDoesNotReuseRecent(t *testing.T) {
	base := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞", MarketScore: 210},
		{Rank: 2, Hotspot: "人工智能", Code: "603019", Name: "中科曙光", MarketScore: 200},
	}
	replacementPool := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002230", Name: "科大讯飞", MarketScore: 210},
		{Rank: 2, Hotspot: "人工智能", Code: "603019", Name: "中科曙光", MarketScore: 200},
		{Rank: 3, Hotspot: "人工智能", Code: "601138", Name: "工业富联", MarketScore: 190},
	}

	result := filterRecentAStockRecommendationsWithReplenishment(base, replacementPool, map[string]struct{}{
		"002230": {},
		"603019": {},
		"601138": {},
	}, len(base))

	if result.Filtered != 3 || result.Replenished != 0 || !result.Shortfall || len(result.Recommendations) != 0 {
		t.Fatalf("expected recent-only pool to remain empty with shortfall, got %+v", result)
	}
}

func TestAStockRecentRecommendationCodesUseRequestCache(t *testing.T) {
	var mu sync.Mutex
	articleCalls := 0
	auctionCalls := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			mu.Lock()
			articleCalls++
			mu.Unlock()
			t.Fatalf("recent recommendation scan should not rebuild history from articles: %s", r.URL.String())
		case "/api/v1/a-stock/auction":
			mu.Lock()
			auctionCalls++
			mu.Unlock()
			t.Fatalf("recent recommendation scan should not rebuild history from auction candidates: %s", r.URL.String())
		case "/api/v1/a-stock/recommendation-selections":
			result := model.AStockRecommendationSelectionListResult{Found: false}
			if r.URL.Query().Get("date") == "2026-06-15" && r.URL.Query().Get("period") == "morning" {
				result = model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "002230", Name: "科大讯飞"}},
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": result})
		case "/api/v1/a-stock/recommendations":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.AStockRecommendationSnapshot{Found: false}})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	cache := newAStockRequestCache()
	first := srv.loadRecentAStockRecommendationCodesWithCache("2026-06-16", 5, cache)
	if _, ok := first["002230"]; !ok {
		t.Fatalf("expected recent code 002230, got %+v", first)
	}
	mu.Lock()
	firstArticleCalls := articleCalls
	firstAuctionCalls := auctionCalls
	mu.Unlock()
	if firstArticleCalls != 0 || firstAuctionCalls != 0 {
		t.Fatalf("expected recent scan to use persisted recommendations only, got articles=%d auction=%d", firstArticleCalls, firstAuctionCalls)
	}

	second := srv.loadRecentAStockRecommendationCodesWithCache("2026-06-16", 5, cache)
	if _, ok := second["002230"]; !ok {
		t.Fatalf("expected cached recent code 002230, got %+v", second)
	}
	mu.Lock()
	gotArticleCalls := articleCalls
	gotAuctionCalls := auctionCalls
	mu.Unlock()
	if gotArticleCalls != firstArticleCalls || gotAuctionCalls != firstAuctionCalls {
		t.Fatalf("expected cached scan to reuse first result, article calls %d->%d auction calls %d->%d", firstArticleCalls, gotArticleCalls, firstAuctionCalls, gotAuctionCalls)
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
		if r.URL.Path == "/api/v1/a-stock/recommendation-selections" && r.URL.Query().Get("date") == "2026-06-15" && r.URL.Query().Get("period") == "morning" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "601138", Name: "工业富联", Hotspot: "人工智能", Reason: "recent"},
						{Rank: 2, Code: "603019", Name: "中科曙光", Hotspot: "人工智能", Reason: "recent"},
						{Rank: 3, Code: "002230", Name: "科大讯飞", Hotspot: "人工智能", Reason: "recent"},
					},
				},
			})
			return
		}
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/a-stock/holdings/summary" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{}})
			return
		}
		if r.URL.Path == "/api/v1/a-stock/auction" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": model.AStockAuctionListResult{
					Date:  r.URL.Query().Get("date"),
					Items: []model.AStockAuctionAmount{{TradeDate: r.URL.Query().Get("date"), Code: "002230", Name: "科大讯飞", AuctionVolume: 1000000, AuctionAmount: 10000000, Status: "ok"}},
				},
			})
			return
		}
		if r.URL.Path != "/api/v1/articles" {
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.ItemListResult{
				Items: []model.Item{{ID: 900, SourceType: "flash", Title: "AI 算力政策加码，科大讯飞活跃", Summary: "人工智能产业链活跃", TagFlags: "0.002230", CapturedAt: time.Date(2026, 6, 16, 1, 5, 0, 0, time.UTC)}},
				Page:  1, PageSize: 200, Total: 1,
			},
		})
	}))
	defer content.Close()

	scheduler := newAStockTradingDayServer(t, true)
	defer scheduler.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	filtered := srv.loadAStockContext("2026-06-16", "morning", 1, false)
	if filtered.RecentFiltered == 0 || len(filtered.Recommendations) != 0 {
		t.Fatalf("expected recent filter to remove recommendations, got filtered=%d recommendations=%+v", filtered.RecentFiltered, filtered.Recommendations)
	}
	ignored := srv.loadAStockContext("2026-06-16", "morning", 1, true)
	if ignored.RecentFiltered != 0 || len(ignored.Recommendations) == 0 {
		t.Fatalf("expected ignoreRecent to keep recommendations, got filtered=%d recommendations=%+v", ignored.RecentFiltered, ignored.Recommendations)
	}
}

func TestAStockRecentRecommendationFilterUsesTradingDays(t *testing.T) {
	queriedSelectionDates := map[string]struct{}{}
	queriedSnapshotDates := map[string]struct{}{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			date := r.URL.Query().Get("date")
			queriedSelectionDates[date] = struct{}{}
			switch {
			case date == "2026-06-25" && r.URL.Query().Get("period") == "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600030", Name: "中信证券", Hotspot: "金融券商", Reason: "recent"}},
				})
				return
			case date == "2026-06-24" && r.URL.Query().Get("period") == "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600099", Name: "林海股份", Hotspot: "机器人", Reason: "selection recent"}},
				})
				return
			case date == "2026-03-11" && r.URL.Query().Get("period") == "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600031", Name: "三一重工", Hotspot: "工程机械", Reason: "trading-day boundary"}},
				})
				return
			case date == "2026-03-10" && r.URL.Query().Get("period") == "morning":
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "600032", Name: "浙江新能", Hotspot: "新能源", Reason: "outside boundary"}},
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			date := r.URL.Query().Get("date")
			queriedSnapshotDates[date] = struct{}{}
			if date == "2026-06-24" && r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
					Found:        true,
					StrategyDate: date,
					Period:       "morning",
					RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{
						{Rank: 1, Code: "002520", Name: "日发精机", Hotspot: "机器人", Reason: "snapshot recent"},
					}),
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	scheduler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/scheduler/a-stock/trading-day" {
			t.Fatalf("unexpected trading-day path: %s", r.URL.String())
		}
		date := r.URL.Query().Get("date")
		day, err := time.Parse("2006-01-02", date)
		isTradingDay := err == nil && day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope(w, http.StatusOK, "ok", aStockTradingDayStatus{
			Date:         date,
			IsTradingDay: isTradingDay,
			Source:       "test_weekday_calendar",
			Reason:       "test",
			Message:      "test",
		})
	}))
	defer scheduler.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	recentCodes := srv.loadRecentAStockRecommendationCodes("2026-07-15", aStockRecentLookbackDays)
	if _, ok := recentCodes["600030"]; !ok {
		t.Fatalf("expected 2026-06-25 recommendation in 90-trading-day lookback, got %+v", recentCodes)
	}
	if _, ok := recentCodes["600031"]; !ok {
		t.Fatalf("expected 2026-03-11 recommendation at 90-trading-day boundary, got %+v", recentCodes)
	}
	if _, ok := recentCodes["600099"]; !ok {
		t.Fatalf("expected recent selections to be included, got %+v", recentCodes)
	}
	if _, ok := recentCodes["002520"]; !ok {
		t.Fatalf("expected recent snapshot code to be included even when selections exist, got %+v", recentCodes)
	}
	if _, ok := queriedSelectionDates["2026-03-11"]; !ok {
		t.Fatalf("expected 90-trading-day boundary date to be queried, queried dates=%+v", queriedSelectionDates)
	}
	if _, ok := queriedSnapshotDates["2026-06-24"]; !ok {
		t.Fatalf("expected snapshots to be queried alongside selections, queried dates=%+v", queriedSnapshotDates)
	}
	if _, ok := queriedSelectionDates["2026-03-10"]; ok {
		t.Fatalf("expected date outside 90-trading-day lookback to be skipped, queried dates=%+v", queriedSelectionDates)
	}
	if _, ok := queriedSelectionDates["2026-07-12"]; ok {
		t.Fatalf("expected non-trading Sunday to be skipped, queried dates=%+v", queriedSelectionDates)
	}
}

func TestAStockAfternoonRecentCodesIncludeSameDayMorningSelections(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("date") == "2026-07-03" && r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{{Rank: 1, Code: "601138", Name: "工业富联", Hotspot: "算力", Reason: "same-day morning"}},
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	afternoonCodes := srv.loadRecentAStockRecommendationCodesForPeriodWithCache("2026-07-03", "afternoon", aStockRecentLookbackDays, newAStockRequestCache())
	if _, ok := afternoonCodes["601138"]; !ok {
		t.Fatalf("expected afternoon lookback to include same-day morning recommendation, got %+v", afternoonCodes)
	}
	morningCodes := srv.loadRecentAStockRecommendationCodesForPeriodWithCache("2026-07-03", "morning", aStockRecentLookbackDays, newAStockRequestCache())
	if _, ok := morningCodes["601138"]; ok {
		t.Fatalf("expected morning lookback to exclude same-day morning recommendation, got %+v", morningCodes)
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
		{Code: "000002", Date: "2026-06-16", Open: 92, Close: 94, Pct: 2},
		{Code: "000003", Date: "2026-04-17", Close: 100},
		{Code: "000003", Date: "2026-05-16", Close: 100},
		{Code: "000003", Date: "2026-06-15", Close: 98, Pct: 2},
		{Code: "000003", Date: "2026-06-16", Open: 99, Close: 100, Pct: 1},
		{Code: "000004", Date: "2026-04-17", Close: 100},
		{Code: "000004", Date: "2026-05-16", Close: 100},
		{Code: "000004", Date: "2026-06-15", Close: 98, Pct: 2},
	}

	filtered, rows, status, _, _ := applyAStockMarketBars("2026-06-16", "morning", recommendations, bars, false, true, 0)

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
	if filtered[1].Code != "000002" || filtered[1].MarketScore != 40 || !strings.Contains(filtered[1].Reason, "板块回撤减分 40") {
		t.Fatalf("expected remaining AI stock to carry sector penalty, got %+v", filtered[1])
	}
	if filtered[1].CurrentPrice != "94.00" || filtered[1].TodayPct != "+2.00%" || filtered[1].TodayPctClass != "astock-up" {
		t.Fatalf("expected penalized current day market fields to be filled, got %+v", filtered[1])
	}
	if len(rows) != 2 {
		t.Fatalf("expected backtest rows to match filtered recommendations, got %+v", rows)
	}
	if rows[0].T0Return != "+1.01%" || rows[0].T0ReturnClass != "astock-up" {
		t.Fatalf("expected T+0 return to use entry-price based close return, got %+v", rows[0])
	}
	if rows[0].T0Close != "100.00" {
		t.Fatalf("expected T+0 close to carry strategy-day close price, got %+v", rows[0])
	}
	rowStocks := strings.Join([]string{rows[0].Stock, rows[1].Stock}, " ")
	if strings.Contains(rowStocks, "000001") || strings.Contains(rowStocks, "000004") {
		t.Fatalf("expected backtest rows to exclude market-filtered candidates, got %+v", rows)
	}
	if !strings.Contains(status, "过滤回撤股票 1") || !strings.Contains(status, "过滤无当日行情股票 1") {
		t.Fatalf("expected status to mention drawdown and missing price filtering, got %q", status)
	}
	for _, want := range []string{"过滤回撤股票 1（000001 回撤过滤）", "过滤无当日行情股票 1（000004 停牌过滤）"} {
		if !strings.Contains(status, want) {
			t.Fatalf("expected status to include filtered stock detail %q, got %q", want, status)
		}
	}
}

func TestAStockMarketViewStatusIncludesOverheatFilteredStockDetails(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "000001", Name: "60日过热", HotspotScore: 80, MarketScore: 80, Reason: "热度分 80"},
		{Rank: 2, Hotspot: "人工智能", Code: "000002", Name: "正常股票", HotspotScore: 70, MarketScore: 70, Reason: "热度分 70"},
	})
	bars := []aStockMarketBar{
		{Code: "000001", Date: "2026-04-17", Close: 50},
		{Code: "000001", Date: "2026-05-16", Close: 90},
		{Code: "000001", Date: "2026-06-15", Close: 100, Pct: 1},
		{Code: "000001", Date: "2026-06-16", Open: 100, Close: 101, Pct: 1},
		{Code: "000002", Date: "2026-04-17", Close: 100},
		{Code: "000002", Date: "2026-05-16", Close: 100},
		{Code: "000002", Date: "2026-06-15", Close: 100, Pct: 1},
		{Code: "000002", Date: "2026-06-16", Open: 100, Close: 101, Pct: 1},
	}

	filtered, _, status, _, _ := applyAStockMarketBars("2026-06-16", "morning", recommendations, bars, false, false, 0)

	if len(filtered) != 1 || filtered[0].Code != "000002" {
		t.Fatalf("expected 60-day overheated stock to be filtered, got recommendations=%+v status=%q", filtered, status)
	}
	if !strings.Contains(status, "过滤60日过热股票 1（000001 60日过热）") {
		t.Fatalf("expected status to include overheat filtered stock detail, got %q", status)
	}
}

func TestAStockMarketViewKeepsMissingTodayMarketByDefault(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "000004", Name: "无当日行情", HotspotScore: 80, MarketScore: 80, Reason: "热度分 80"},
	})
	bars := []aStockMarketBar{
		{Code: "000004", Date: "2026-04-17", Close: 100},
		{Code: "000004", Date: "2026-05-16", Close: 100},
		{Code: "000004", Date: "2026-06-15", Close: 98, Pct: 2},
	}

	filtered, rows, status, _, noTodayMarket := applyAStockMarketBars("2026-06-16", "morning", recommendations, bars, false, false, 0)

	if noTodayMarket != 1 {
		t.Fatalf("expected missing today-market count 1, got %d status=%q", noTodayMarket, status)
	}
	if len(filtered) != 1 || filtered[0].Code != "000004" {
		t.Fatalf("expected recommendation without today market to remain by default, got %+v", filtered)
	}
	if len(rows) != 1 || rows[0].Status != "等待当日开盘价" {
		t.Fatalf("expected backtest row to wait for today market, got %+v", rows)
	}
	if !strings.Contains(status, "缺少当日行情股票 1") || strings.Contains(status, "过滤无当日行情") || strings.Contains(status, "无当日行情可推荐") {
		t.Fatalf("expected status to mention missing but not filtered today market, got %q", status)
	}
}

func TestAStockMarketBarsPenalizePreviousHighPctRisk(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "机器人", Code: "600001", Name: "昨日涨停", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
		{Rank: 2, Hotspot: "机器人", Code: "600002", Name: "昨日大涨", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
		{Rank: 3, Hotspot: "机器人", Code: "600003", Name: "边界八点", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
	})
	bars := []aStockMarketBar{
		{Code: "600001", Date: "2026-06-15", Close: 11.00, Pct: 9.90},
		{Code: "600001", Date: "2026-06-16", Open: 11.10, Close: 11.20, Pct: 1.82},
		{Code: "600002", Date: "2026-06-15", Close: 10.81, Pct: 8.01},
		{Code: "600002", Date: "2026-06-16", Open: 10.90, Close: 11.00, Pct: 1.76},
		{Code: "600003", Date: "2026-06-15", Close: 10.80, Pct: 8.00},
		{Code: "600003", Date: "2026-06-16", Open: 10.90, Close: 11.00, Pct: 1.85},
	}

	filtered, _, status, _, _ := applyAStockMarketBars("2026-06-16", "morning", recommendations, bars, false, false, 0)

	if len(filtered) != 3 {
		t.Fatalf("expected previous high-pct stocks to remain with penalties, got %+v status=%q", filtered, status)
	}
	limitUp := mustAStockRecommendationForTest(t, filtered, "600001")
	if limitUp.MarketScore != 20 || !strings.Contains(limitUp.Reason, "昨日涨停+9.90%，风险扣分 80") {
		t.Fatalf("expected previous limit-up penalty only, got %+v", limitUp)
	}
	limitUpComponent := mustAStockScoreComponentForTest(t, limitUp, "昨日涨停")
	if limitUpComponent.Score != -80 || !strings.Contains(limitUpComponent.Detail, "昨日涨停+9.90%") {
		t.Fatalf("expected previous limit-up score component, got %+v", limitUpComponent)
	}
	if highComponent := aStockRecommendationScoreBreakdown(limitUp); strings.Contains(fmt.Sprint(highComponent), "昨日涨幅过高") {
		t.Fatalf("expected previous limit-up not to also get high-pct penalty, got %+v", highComponent)
	}
	highPct := mustAStockRecommendationForTest(t, filtered, "600002")
	if highPct.MarketScore != 40 || !strings.Contains(highPct.Reason, "昨日涨幅+8.01%，追高风险扣分 60") {
		t.Fatalf("expected previous >8%% penalty, got %+v", highPct)
	}
	highPctComponent := mustAStockScoreComponentForTest(t, highPct, "昨日涨幅过高")
	if highPctComponent.Score != -60 || !strings.Contains(highPctComponent.Detail, "昨日涨幅+8.01%") {
		t.Fatalf("expected previous high-pct score component, got %+v", highPctComponent)
	}
	boundary := mustAStockRecommendationForTest(t, filtered, "600003")
	if boundary.MarketScore != 100 || strings.Contains(boundary.Reason, "追高风险扣分") {
		t.Fatalf("expected exactly 8.00%% previous pct not to be penalized, got %+v", boundary)
	}
}

func TestAStockMarketBarsFilterTodayHighPctForMorningAndAfternoon(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600001", Name: "今日过高", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
		{Rank: 2, Hotspot: "人工智能", Code: "600002", Name: "今日边界", HotspotScore: 90, MarketScore: 90, Reason: "热度分 90"},
	})
	morningBars := []aStockMarketBar{
		{Code: "600001", Date: "2026-06-15", Close: 10.00, Pct: 1.00},
		{Code: "600001", Date: "2026-06-16", Open: 10.99, Low: 10.99, Close: 11.00, Pct: 10.00},
		{Code: "600002", Date: "2026-06-15", Close: 10.00, Pct: 1.00},
		{Code: "600002", Date: "2026-06-16", Open: 10.79, Close: 10.80, Pct: 8.00},
	}

	filtered, rows, status, limitUpFiltered, _ := applyAStockMarketBars("2026-06-16", "morning", recommendations, morningBars, false, false, 0)
	if limitUpFiltered != 0 || len(filtered) != 1 || filtered[0].Code != "600002" || len(rows) != 1 {
		t.Fatalf("expected morning today >8%% filter to keep only boundary stock, limitUp=%d recommendations=%+v rows=%+v", limitUpFiltered, filtered, rows)
	}
	if !strings.Contains(status, "过滤今日涨幅过高股票 1") {
		t.Fatalf("expected morning status to mention today high-pct filter, got %q", status)
	}
	if !strings.Contains(status, "过滤今日涨幅过高股票 1（600001 今日过高）") {
		t.Fatalf("expected morning status to include today high-pct filtered stock detail, got %q", status)
	}

	afternoonBars := []aStockMarketBar{
		{Code: "600001", Date: "2026-06-15", Close: 10.00, Pct: 1.00},
		{Code: "600001", Date: "2026-06-16", Open: 10.99, Low: 10.99, AfternoonEntryPrice: 10.99, Close: 11.00, Pct: 10.00},
		{Code: "600002", Date: "2026-06-15", Close: 10.00, Pct: 1.00},
		{Code: "600002", Date: "2026-06-16", Open: 10.79, AfternoonEntryPrice: 10.79, Close: 10.80, Pct: 8.00},
	}
	filtered, rows, status, limitUpFiltered, _ = applyAStockMarketBars("2026-06-16", "afternoon", recommendations, afternoonBars, false, false, 0)
	if limitUpFiltered != 0 || len(filtered) != 1 || filtered[0].Code != "600002" || len(rows) != 1 {
		t.Fatalf("expected afternoon today >8%% filter to ignore limit-up toggle and keep boundary stock, limitUp=%d recommendations=%+v rows=%+v", limitUpFiltered, filtered, rows)
	}
	if !strings.Contains(status, "过滤今日涨幅过高股票 1") {
		t.Fatalf("expected afternoon status to mention today high-pct filter, got %q", status)
	}
	if !strings.Contains(status, "过滤今日涨幅过高股票 1（600001 今日过高）") {
		t.Fatalf("expected afternoon status to include today high-pct filtered stock detail, got %q", status)
	}

	filtered, rows, status, _, _ = applyAStockMarketBars("2026-06-16", "morning", recommendations[:1], morningBars[:2], false, false, 0)
	if len(filtered) != 0 || len(rows) != 0 || !strings.Contains(status, "今日涨幅过高过滤后无推荐股票") {
		t.Fatalf("expected empty status when all stocks are filtered by today high-pct, recommendations=%+v rows=%+v status=%q", filtered, rows, status)
	}
}

func TestAStockMarketBarsAllowOpenedLimitUpHighPct(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "地产", Code: "600162", Name: "香江控股", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
		{Rank: 2, Hotspot: "医疗", Code: "002432", Name: "九安医疗", HotspotScore: 90, MarketScore: 90, Reason: "热度分 90"},
	})
	bars := []aStockMarketBar{
		{Code: "600162", Date: "2026-06-15", Close: 10.00, Pct: 0.00},
		{Code: "600162", Date: "2026-06-16", Open: 10.50, Low: 10.50, Close: 11.00, Pct: 10.00, EntryPrice: 10.50, AfternoonEntryPrice: 10.90},
		{Code: "002432", Date: "2026-06-15", Close: 20.00, Pct: 0.00},
		{Code: "002432", Date: "2026-06-16", Open: 22.00, Low: 21.50, Close: 22.00, Pct: 10.00, EntryPrice: 22.00, AfternoonEntryPrice: 22.00},
	}

	result := applyAStockMarketBarsDetailedWithSettings("2026-06-16", "afternoon", recommendations, bars, true, false, 0, defaultAStockAlgorithmSettings())
	if len(result.Recommendations) != 2 || result.LimitUpFiltered != 0 || len(result.FilteredRecommendations) != 0 {
		t.Fatalf("expected open-board high-pct stocks to remain recommendable, result=%+v", result)
	}
	if strings.Contains(result.Status, "过滤今日涨幅过高股票") || strings.Contains(result.Status, "过滤涨停股票") {
		t.Fatalf("expected no high-pct/limit-up status for opened stocks, got %q", result.Status)
	}
}

func TestAStockMarketBarsKeepOldFilterWhenOpenBoardCannotBeConfirmed(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600162", Name: "香江控股", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
	})
	bars := []aStockMarketBar{
		{Code: "600162", Date: "2026-06-16", Open: 10.50, Low: 10.50, Close: 10.81, Pct: 8.10, EntryPrice: 10.50},
	}

	result := applyAStockMarketBarsDetailedWithSettings("2026-06-16", "morning", recommendations, bars, false, false, 0, defaultAStockAlgorithmSettings())
	if len(result.Recommendations) != 0 || len(result.FilteredRecommendations) != 1 || result.FilteredRecommendations[0].Reason != aStockFilteredReasonTodayHighPct {
		t.Fatalf("expected missing previous close to keep old high-pct filter, result=%+v", result)
	}
}

func TestAStockMomentumTrendScoreBoostsMarketRanking(t *testing.T) {
	trendCloses := make([]float64, 45)
	for i := range trendCloses {
		if i < 24 {
			trendCloses[i] = 10 + float64((i%3)-1)*0.02
			continue
		}
		trendCloses[i] = 10 + float64(i-23)*0.045
	}
	flatCloses := make([]float64, 45)
	for i := range flatCloses {
		flatCloses[i] = 10
	}
	bars := append(
		aStockMomentumBarsForTest(t, "600100", "2026-06-16", trendCloses, aStockMomentumAmountsForTest(len(trendCloses), 220000000)),
		aStockMomentumBarsForTest(t, "600101", "2026-06-16", flatCloses, nil)...,
	)
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600101", Name: "震荡股", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
		{Rank: 2, Hotspot: "人工智能", Code: "600100", Name: "趋势股", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
	})

	got, _, status, _, _ := applyAStockMarketBars("2026-06-16", "morning", recommendations, bars, false, false, 0)

	if len(got) != 2 || got[0].Code != "600100" {
		t.Fatalf("expected momentum stock to rank first, got %+v status=%q", got, status)
	}
	component := mustAStockScoreComponentForTest(t, got[0], "动能趋势")
	if component.Score <= 0 || !strings.Contains(component.Detail, "ADX转强") || !strings.Contains(component.Detail, "放量确认") {
		t.Fatalf("expected ADX and volume momentum score component, got %+v", component)
	}
	if got[0].MarketScore != 100+component.Score || !strings.Contains(got[0].Reason, "动能趋势加分") {
		t.Fatalf("expected momentum score to be added to MarketScore and reason, got %+v", got[0])
	}
}

func TestAStockMomentumTrendScoreDetectsBollingerBreakout(t *testing.T) {
	closes := make([]float64, 70)
	for i := range closes {
		switch {
		case i < 30:
			if i%2 == 0 {
				closes[i] = 10.45
			} else {
				closes[i] = 9.55
			}
		case i < len(closes)-1:
			if i%2 == 0 {
				closes[i] = 10.02
			} else {
				closes[i] = 9.98
			}
		default:
			closes[i] = 10.16
		}
	}

	_, signals := aStockMomentumTrendScore(aStockMomentumBarsForTest(t, "600200", "2026-06-16", closes, nil), "2026-06-16")

	if !aStockMomentumHasSignalForTest(signals, "布林突破") {
		t.Fatalf("expected bollinger breakout signal, got %+v", signals)
	}
}

func TestAStockMomentumTrendScoreDetectsMACDStrength(t *testing.T) {
	closes := make([]float64, 35)
	for i := range closes {
		if i < len(closes)-1 {
			closes[i] = 10 - float64(i)*0.04
			continue
		}
		closes[i] = 10
	}

	_, signals := aStockMomentumTrendScore(aStockMomentumBarsForTest(t, "600201", "2026-06-16", closes, nil), "2026-06-16")

	if !aStockMomentumHasSignalForTest(signals, "MACD转强") {
		t.Fatalf("expected MACD strength signal, got %+v", signals)
	}
}

func TestAStockMomentumTrendScoreVolumeOnly(t *testing.T) {
	closes := make([]float64, 35)
	for i := range closes {
		closes[i] = 10
	}

	score, signals := aStockMomentumTrendScore(aStockMomentumBarsForTest(t, "600202", "2026-06-16", closes, aStockMomentumAmountsForTest(len(closes), 200000000)), "2026-06-16")

	if score != aStockMomentumVolumeScore || len(signals) != 1 || signals[0].Name != "放量确认" {
		t.Fatalf("expected volume-only momentum score, score=%d signals=%+v", score, signals)
	}
}

func TestAStockMomentumTrendScoreInsufficientBars(t *testing.T) {
	closes := make([]float64, aStockMomentumMinBars-1)
	for i := range closes {
		closes[i] = 10 + float64(i)*0.1
	}

	score, signals := aStockMomentumTrendScore(aStockMomentumBarsForTest(t, "600203", "2026-06-16", closes, aStockMomentumAmountsForTest(len(closes), 300000000)), "2026-06-16")

	if score != 0 || len(signals) != 0 {
		t.Fatalf("expected insufficient bars to avoid momentum scoring, score=%d signals=%+v", score, signals)
	}
}

func TestAStockMomentumDoesNotBypassTodayHighPctFilter(t *testing.T) {
	closes := make([]float64, 35)
	for i := range closes {
		closes[i] = 10
	}
	closes[len(closes)-1] = 10.81
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600204", Name: "动能过热", HotspotScore: 100, MarketScore: 100, Reason: "热度分 100"},
	})

	bars := aStockMomentumBarsForTest(t, "600204", "2026-06-16", closes, aStockMomentumAmountsForTest(len(closes), 300000000))
	last := len(bars) - 1
	bars[last].Open = 10.99
	bars[last].EntryPrice = 10.99
	bars[last].Low = 10.99
	bars[last].Close = 11.00
	bars[last].Pct = 10.00
	got, _, status, _, _ := applyAStockMarketBars("2026-06-16", "morning", recommendations, bars, false, false, 0)

	if len(got) != 0 || !strings.Contains(status, "过滤今日涨幅过高股票 1") {
		t.Fatalf("expected today high-pct filter to run before momentum scoring, got %+v status=%q", got, status)
	}
}

func TestAStockMarketViewFiltersLimitUpStocksForAfternoon(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "黄金有色", Code: "600172", Name: "ST黄河", HotspotScore: 80, MarketScore: 80, Reason: "热度分 80"},
		{Rank: 2, Hotspot: "新能源", Code: "300179", Name: "ST四方", HotspotScore: 79, MarketScore: 79, Reason: "热度分 79"},
		{Rank: 3, Hotspot: "半导体", Code: "688662", Name: "ST富信", HotspotScore: 78, MarketScore: 78, Reason: "热度分 78"},
		{Rank: 4, Hotspot: "金融券商", Code: "000001", Name: "平安银行", HotspotScore: 77, MarketScore: 77, Reason: "热度分 77"},
	})
	bars := []aStockMarketBar{
		{Code: "600172", Date: "2026-06-22", Open: 15.41, AfternoonEntryPrice: 15.41, Close: 15.41, Pct: 4.90},
		{Code: "300179", Date: "2026-06-22", Open: 46.75, AfternoonEntryPrice: 46.75, Close: 46.75, Pct: 4.90},
		{Code: "688662", Date: "2026-06-22", Open: 158.66, AfternoonEntryPrice: 158.66, Close: 158.66, Pct: 4.90},
		{Code: "000001", Date: "2026-06-22", Open: 12.3, AfternoonEntryPrice: 12.3, Close: 12.5, Pct: 1.63},
	}

	filtered, rows, status, limitUpFiltered, _ := applyAStockMarketBars("2026-06-22", "afternoon", recommendations, bars, true, false, 0)

	if limitUpFiltered != 3 {
		t.Fatalf("expected three limit-up stocks filtered, got %d status=%q recommendations=%+v", limitUpFiltered, status, filtered)
	}
	if len(filtered) != 1 || filtered[0].Code != "000001" || filtered[0].Rank != 1 {
		t.Fatalf("expected only non-limit-up stock to remain and rerank, got %+v", filtered)
	}
	if len(rows) != 1 || strings.Contains(rows[0].Stock, "600172") || strings.Contains(rows[0].Stock, "300179") || strings.Contains(rows[0].Stock, "688662") {
		t.Fatalf("expected backtest rows to exclude limit-up stocks, got %+v", rows)
	}
	if !strings.Contains(status, "过滤涨停股票 3") {
		t.Fatalf("expected status to mention limit-up filter, got %q", status)
	}
}

func TestAStockMarketViewBackfillsLimitUpStocksByHeat(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "黄金有色", Code: "600172", Name: "ST黄河", HotspotScore: 90, MarketScore: 90, Reason: "热度分 90"},
		{Rank: 2, Hotspot: "黄金有色", Code: "000001", Name: "平安银行", HotspotScore: 85, MarketScore: 85, Reason: "热度分 85"},
		{Rank: 3, Hotspot: "黄金有色", Code: "000002", Name: "万科A", HotspotScore: 84, MarketScore: 84, Reason: "热度分 84"},
		{Rank: 4, Hotspot: "黄金有色", Code: "000003", Name: "国华网安", HotspotScore: 83, MarketScore: 83, Reason: "热度分 83"},
	})
	bars := []aStockMarketBar{
		{Code: "600172", Date: "2026-06-22", Open: 15.41, AfternoonEntryPrice: 15.41, Close: 15.41, Pct: 4.90},
		{Code: "000001", Date: "2026-06-22", Open: 12.3, AfternoonEntryPrice: 12.3, Close: 12.5, Pct: 1.63},
		{Code: "000002", Date: "2026-06-22", Open: 8.2, AfternoonEntryPrice: 8.2, Close: 8.4, Pct: 2.44},
		{Code: "000003", Date: "2026-06-22", Open: 9.1, AfternoonEntryPrice: 9.1, Close: 9.2, Pct: 1.1},
	}

	filtered, rows, status, limitUpFiltered, _ := applyAStockMarketBars("2026-06-22", "afternoon", recommendations, bars, true, false, 3)

	if limitUpFiltered != 1 {
		t.Fatalf("expected one limit-up stock filtered, got %d status=%q", limitUpFiltered, status)
	}
	if len(filtered) != 3 || len(rows) != 3 {
		t.Fatalf("expected next hot stock to backfill to target size, got recommendations=%+v rows=%+v", filtered, rows)
	}
	gotCodes := []string{filtered[0].Code, filtered[1].Code, filtered[2].Code}
	if strings.Join(gotCodes, ",") != "000001,000002,000003" {
		t.Fatalf("expected non-limit-up stocks to fill by heat order, got %+v", filtered)
	}
	if !strings.Contains(status, "过滤涨停股票 1") || !strings.Contains(status, "已按热度递补") {
		t.Fatalf("expected status to mention limit-up replacement, got %q", status)
	}
}

func TestAStockMarketViewKeepsAfternoonRecommendationWithout1300Price(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "000001", Name: "平安银行", HotspotScore: 80, MarketScore: 80, Reason: "热度分 80"},
	})
	bars := []aStockMarketBar{
		{Code: "000001", Date: "2026-06-22", Close: 11.2, Pct: 1.1},
		{Code: "000001", Date: "2026-06-23", Open: 11.3, Close: 11.5, Pct: 2.68},
	}

	filtered, rows, status, _, _ := applyAStockMarketBars("2026-06-23", "afternoon", recommendations, bars, false, false, 0)

	if len(filtered) != 1 || filtered[0].Code != "000001" {
		t.Fatalf("expected afternoon recommendation to remain without 13:00 price, got %+v", filtered)
	}
	if filtered[0].CurrentPrice != "11.50" || filtered[0].TodayPct != "+2.68%" {
		t.Fatalf("expected same-day market fields to be retained, got %+v", filtered[0])
	}
	if len(rows) != 1 || rows[0].Status != "等待下午开盘价" || rows[0].AfternoonOpen != "--" {
		t.Fatalf("expected backtest row to wait for afternoon entry price, got %+v", rows)
	}
	if !strings.Contains(status, "等待下午开盘价股票 1") || strings.Contains(status, "无当日行情可推荐") {
		t.Fatalf("expected status to mention waiting afternoon entry price, got %q", status)
	}
}

func TestAStockRecommendationEntryTimeNormalization(t *testing.T) {
	tests := []struct {
		name   string
		period string
		rec    aStockRecommendation
		want   string
	}{
		{name: "morning early maps to open", period: "morning", rec: aStockRecommendation{EntryTime: "09:27"}, want: "09:30"},
		{name: "afternoon pre-open maps to 13:01", period: "afternoon", rec: aStockRecommendation{EntryTime: "12:57"}, want: "13:01"},
		{name: "afternoon 13:00 maps to 13:01", period: "afternoon", rec: aStockRecommendation{EntryTime: "13:00"}, want: "13:01"},
		{name: "afternoon intraday keeps minute", period: "afternoon", rec: aStockRecommendation{EntryTime: "10:30"}, want: "10:30"},
		{name: "afternoon parses generated reason", period: "afternoon", rec: aStockRecommendation{Reason: "按热度生成，生成点 10:30"}, want: "10:30"},
		{name: "afternoon empty defaults", period: "afternoon", rec: aStockRecommendation{}, want: "13:01"},
		{name: "morning empty defaults", period: "morning", rec: aStockRecommendation{}, want: "09:30"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := aStockRecommendationEffectiveEntryTime(tc.rec, tc.period); got != tc.want {
				t.Fatalf("expected entry time %s, got %s", tc.want, got)
			}
		})
	}
}

func TestAStockBacktestUses0930EntryPrice(t *testing.T) {
	recommendations := []aStockRecommendation{{Code: "300285", Name: "国瓷材料"}}
	byCode := groupAStockMarketBars([]aStockMarketBar{
		{Code: "300285", Date: "2026-06-18", Open: 65, EntryPrice: 66, Close: 67, Pct: 3.08},
		{Code: "300285", Date: "2026-06-19", Open: 67.5, Close: 69.3, Pct: 3.43},
	})

	rows := buildAStockBacktestRows("2026-06-18", "morning", recommendations, byCode)

	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].EntryOpen != "66.00" {
		t.Fatalf("expected entry price to use 09:30 price, got %+v", rows[0])
	}
	if rows[0].AfternoonOpen != "--" {
		t.Fatalf("expected morning backtest row to leave afternoon open empty, got %+v", rows[0])
	}
	if rows[0].T0Close != "67.00" {
		t.Fatalf("expected T+0 close to use strategy-day close price, got %+v", rows[0])
	}
	if rows[0].T0Return != "+1.52%" {
		t.Fatalf("expected T+0 return to use 09:30 entry price, got %+v", rows[0])
	}
	if rows[0].Days[0].Return != "+5.00%" {
		t.Fatalf("expected T+1 return to use 09:30 entry price, got %+v", rows[0])
	}
}

func TestAStockBacktestUsesAfternoonEntryPrice(t *testing.T) {
	recommendations := []aStockRecommendation{{Code: "300285", Name: "国瓷材料"}}
	byCode := groupAStockMarketBars([]aStockMarketBar{
		{Code: "300285", Date: "2026-06-18", Open: 65, EntryPrice: 66, AfternoonEntryPrice: 66.8, Close: 67, Pct: 3.08},
		{Code: "300285", Date: "2026-06-19", Open: 67.5, Close: 69.3, Pct: 3.43},
	})

	rows := buildAStockBacktestRows("2026-06-18", "afternoon", recommendations, byCode)

	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].EntryOpen != "--" {
		t.Fatalf("expected afternoon backtest row to leave morning open empty, got %+v", rows[0])
	}
	if rows[0].AfternoonOpen != "66.80" {
		t.Fatalf("expected entry price to use afternoon open price, got %+v", rows[0])
	}
	if rows[0].T0Return != "+0.30%" {
		t.Fatalf("expected T+0 return to use afternoon open price, got %+v", rows[0])
	}
	if rows[0].Days[0].Return != "+3.74%" {
		t.Fatalf("expected T+1 return to use afternoon open price, got %+v", rows[0])
	}
}

func TestAStockBacktestUsesAfternoonRecommendationEntryTimePrice(t *testing.T) {
	recommendations := []aStockRecommendation{{Code: "300024", Name: "机器人", EntryTime: "10:30"}}
	byCode := groupAStockMarketBars([]aStockMarketBar{
		{Code: "300024", Date: "2026-06-24", Open: 16, AfternoonEntryPrice: 18, SessionPrices: map[string]float64{"10:30": 17}, Close: 18, Pct: 4.10},
		{Code: "300024", Date: "2026-06-25", Open: 18.2, Close: 19, Pct: 5.56},
	})

	rows := buildAStockBacktestRows("2026-06-24", "afternoon", recommendations, byCode)

	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].EntryOpen != "--" || rows[0].AfternoonOpen != "17.00" {
		t.Fatalf("expected afternoon entry time price in afternoon column, got %+v", rows[0])
	}
	if rows[0].T0Close != "18.00" || rows[0].T0Return != "+5.88%" {
		t.Fatalf("expected T+0 return to use 10:30 entry price, got %+v", rows[0])
	}
	if rows[0].Days[0].Return != "+11.76%" {
		t.Fatalf("expected T+1 return to use 10:30 entry price, got %+v", rows[0])
	}
}

func TestAStockBacktestDisplayOpenPricesKeepsAfternoonRowsOutOfMorningColumn(t *testing.T) {
	morningOpen, afternoonOpen := aStockBacktestDisplayOpenPrices("afternoon", aStockBacktestRow{
		EntryOpen:     "135.54",
		AfternoonOpen: "--",
	})

	if morningOpen != "--" || afternoonOpen != "135.54" {
		t.Fatalf("expected afternoon display to move legacy entry price into afternoon column, got morning=%q afternoon=%q", morningOpen, afternoonOpen)
	}
}

func TestDecodeEastmoneyAStock0930Price(t *testing.T) {
	body := []byte(`{"data":{"klines":["2026-06-18 09:29,64.00,64.50,0,0,0,0,0,0","2026-06-18 09:30,65.00,66.00,0,0,0,0,0,0","2026-06-18 09:31,66.00,66.50,0,0,0,0,0,0"]}}`)

	price, ok := decodeEastmoneyAStock0930Price(body, "2026-06-18")

	if !ok || price != 66 {
		t.Fatalf("expected 09:30 close price 66, got price=%v ok=%v", price, ok)
	}
}

func TestDecodeEastmoneyAStock1300Price(t *testing.T) {
	body := []byte(`{"data":{"klines":["2026-06-18 12:59,66.00,66.20,0,0,0,0,0,0","2026-06-18 13:00,66.80,67.20,0,0,0,0,0,0","2026-06-18 13:01,67.10,67.30,0,0,0,0,0,0"]}}`)

	price, ok := decodeEastmoneyAStock1300Price(body, "2026-06-18")

	if !ok || price != 67.3 {
		t.Fatalf("expected 13:01 close price 67.3, got price=%v ok=%v", price, ok)
	}
}

func TestDecodeEastmoneyAStockSessionPriceArbitraryMinute(t *testing.T) {
	body := []byte(`{"data":{"klines":["2026-06-24 10:29,16.70,16.80,0,0,0,0,0,0","2026-06-24 10:30,16.80,16.88,0,0,0,0,0,0","2026-06-24 10:31,16.88,16.92,0,0,0,0,0,0"]}}`)

	price, ok := decodeEastmoneyAStockSessionPrice(body, "2026-06-24", "10:30")

	if !ok || price != 16.88 {
		t.Fatalf("expected 10:30 close price 16.88, got price=%v ok=%v", price, ok)
	}
}

func TestDecodeTencentAStockSessionPriceArbitraryMinute(t *testing.T) {
	body := []byte(`{"code":0,"data":{"sz300024":{"data":{"date":"20260624","data":["1029 16.70 1200 200000.00","1030 16.88 1400 236320.00","1031 16.92 1100 186120.00"]}}}}`)

	price, ok := decodeTencentAStockSessionPrice(body, "2026-06-24", "10:30")

	if !ok || price != 16.88 {
		t.Fatalf("expected 10:30 tencent price 16.88, got price=%v ok=%v", price, ok)
	}
}

func TestDecodeSinaAStockSessionPriceArbitraryMinute(t *testing.T) {
	body := []byte(`var data=([{"day":"2026-06-24 10:29:00","open":"16.70","close":"16.80"},{"day":"2026-06-24 10:30:00","open":"16.80","close":"16.88"}]);`)

	price, ok := decodeSinaAStockSessionPrice(body, "2026-06-24", "10:30")

	if !ok || price != 16.88 {
		t.Fatalf("expected 10:30 sina price 16.88, got price=%v ok=%v", price, ok)
	}
}

func TestMapToAStockMarketBarParsesMiddayPrices(t *testing.T) {
	bar, ok := mapToAStockMarketBar(map[string]any{
		"code":       "600011",
		"date":       "2026-07-14",
		"close":      10.50,
		"price1230":  10.10,
		"price_1130": 10.00,
	})
	if !ok {
		t.Fatal("expected market bar to parse")
	}
	if bar.SessionPrices["12:30"] != 10.10 || bar.SessionPrices["11:30"] != 10.00 {
		t.Fatalf("expected 12:30 and 11:30 session prices, got %+v", bar.SessionPrices)
	}

	bar, ok = mapToAStockMarketBar(map[string]any{
		"code":         "600012",
		"date":         "2026-07-14",
		"close":        11.50,
		"minute1230":   11.10,
		"midday_price": 11.00,
	})
	if !ok || bar.SessionPrices["12:30"] != 11.10 {
		t.Fatalf("expected minute1230 to populate 12:30 session price, ok=%v bar=%+v", ok, bar)
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
		if r.URL.Query().Get("klt") == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"klines": []string{
						"2026-06-16 09:30,12.20,12.30,0,0,0,0,0,0",
						"2026-06-16 13:01,12.80,12.90,0,0,0,0,0,0",
					},
				},
			})
			return
		}
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
	if bars[1].EntryPrice != 12.3 || bars[1].AfternoonEntryPrice != 12.9 {
		t.Fatalf("expected fallback bars to include 09:30 and 13:01 entry prices, got %+v", bars[1])
	}
}

func TestAStockMarketBarsCustomEndpointSupplementsMissingSessionPricesFromEastmoney(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "002008", "date": "2026-06-22", "open": 131.93, "close": 131.93, "pct": -2.20},
					{"code": "002008", "date": "2026-06-23", "open": 135.54, "close": 145.11, "pct": 9.99},
				},
			},
		})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.002008" {
			t.Fatalf("unexpected eastmoney secid: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("klt") != "1" {
			t.Fatalf("expected minute eastmoney query, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"klines": []string{
					"2026-06-23 09:30,135.54,136.20,0,0,0,0,0,0",
					"2026-06-23 13:01,141.98,142.20,0,0,0,0,0,0",
				},
			},
		})
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-23", []string{"002008"}, custom.URL)
	if err != nil {
		t.Fatalf("expected custom market bars with eastmoney session enrichment, got error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected two market bars, got %+v", bars)
	}
	if bars[1].EntryPrice != 136.2 || bars[1].AfternoonEntryPrice != 142.20 {
		t.Fatalf("expected custom bars to include supplemented session prices, got %+v", bars[1])
	}

	rows := buildAStockBacktestRows("2026-06-23", "afternoon", []aStockRecommendation{{Code: "002008", Name: "大族激光"}}, groupAStockMarketBars(bars))
	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].AfternoonOpen != "142.20" {
		t.Fatalf("expected afternoon backtest to use supplemented 13:01 price, got %+v", rows[0])
	}
	if rows[0].T0Return != "+2.05%" {
		t.Fatalf("expected T+0 return to use supplemented 13:01 price, got %+v", rows[0])
	}
}

func TestAStockMarketBarsSupplementsRecommendationEntryTimeFromEastmoney(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "300024", "date": "2026-06-23", "open": 16.79, "close": 16.79, "pct": -2.21},
					{"code": "300024", "date": "2026-06-24", "open": 16.66, "close": 16.42, "pct": 0.98},
				},
			},
		})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.300024" {
			t.Fatalf("unexpected eastmoney secid: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("klt") != "1" {
			t.Fatalf("expected minute eastmoney query, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"klines": []string{
					"2026-06-24 09:30,16.62,16.70,0,0,0,0,0,0",
					"2026-06-24 10:30,16.70,16.88,0,0,0,0,0,0",
					"2026-06-24 13:01,16.90,17.02,0,0,0,0,0,0",
				},
			},
		})
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-24", []string{"300024"}, custom.URL)
	if err != nil {
		t.Fatalf("expected custom market bars with eastmoney session enrichment, got error: %v", err)
	}
	srv.enrichAStockRecommendationEntryPrices("2026-06-24", "afternoon", []aStockRecommendation{{Code: "300024", Name: "机器人", EntryTime: "10:30"}}, bars)

	rows := buildAStockBacktestRows("2026-06-24", "afternoon", []aStockRecommendation{{Code: "300024", Name: "机器人", EntryTime: "10:30"}}, groupAStockMarketBars(bars))
	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].AfternoonOpen != "16.88" || rows[0].T0Close != "16.42" || rows[0].T0Return != "-2.73%" {
		t.Fatalf("expected afternoon backtest to use supplemented 10:30 price, got %+v", rows[0])
	}
}

func TestAStockMarketBarsSupplementsSessionPricesFromTencentWhenEastmoneyUnavailable(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "300024", "date": "2026-06-23", "open": 16.79, "close": 16.79, "pct": -2.21},
					{"code": "300024", "date": "2026-06-24", "open": 16.66, "close": 16.23, "pct": -3.34},
				},
			},
		})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "eastmoney unavailable", http.StatusBadGateway)
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	tencent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("code") != "sz300024" {
			t.Fatalf("unexpected tencent query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"sz300024": map[string]any{
					"data": map[string]any{
						"date": "20260624",
						"data": []string{
							"0930 16.66 2686 4474876.00",
							"1301 16.25 390989 639792780.00",
						},
					},
				},
			},
		})
	}))
	defer tencent.Close()
	setAStockTencentMinuteURLForTest(t, tencent.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-24", []string{"300024"}, custom.URL)
	if err != nil {
		t.Fatalf("expected custom market bars with tencent session enrichment, got error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected two market bars, got %+v", bars)
	}
	if bars[1].EntryPrice != 16.66 || bars[1].AfternoonEntryPrice != 16.25 {
		t.Fatalf("expected custom bars to include tencent session prices, got %+v", bars[1])
	}

	rows := buildAStockBacktestRows("2026-06-24", "afternoon", []aStockRecommendation{{Code: "300024", Name: "机器人"}}, groupAStockMarketBars(bars))
	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].AfternoonOpen != "16.25" || rows[0].T0Close != "16.23" || rows[0].T0Return != "-0.12%" {
		t.Fatalf("expected afternoon backtest to use tencent 13:01 price, got %+v", rows[0])
	}
}

func TestAStockMarketBarsSupplementsHistoricalSessionPricesFromSinaWhenRealtimeMinuteUnavailable(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"code": "002008", "date": "2026-06-22", "open": 131.93, "close": 131.93, "pct": -2.20},
					{"code": "002008", "date": "2026-06-23", "open": 135.54, "close": 145.11, "pct": 9.99},
					{"code": "002008", "date": "2026-06-24", "open": 136.73, "close": 152.23, "pct": 4.91},
				},
			},
		})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "eastmoney unavailable", http.StatusBadGateway)
	}))
	defer eastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, eastmoney.URL)

	tencent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"sz002008": map[string]any{
					"data": map[string]any{
						"date": "20260624",
						"data": []string{
							"1301 148.58 819691 11879785821.74",
						},
					},
				},
			},
		})
	}))
	defer tencent.Close()
	setAStockTencentMinuteURLForTest(t, tencent.URL)

	sina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("symbol") != "sz002008" || r.URL.Query().Get("scale") != "1" {
			t.Fatalf("unexpected sina query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`/*<script>location.href='//sina.com';</script>*/
=([{"day":"2026-06-23 09:30:00","open":"135.540","high":"136.200","low":"135.540","close":"136.200","volume":"100","amount":"13620.0000"},{"day":"2026-06-23 13:01:00","open":"141.980","high":"142.600","low":"141.510","close":"142.200","volume":"863300","amount":"122678462.3682"}]);`))
	}))
	defer sina.Close()
	setAStockSinaMinuteURLForTest(t, sina.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-23", []string{"002008"}, custom.URL)
	if err != nil {
		t.Fatalf("expected custom market bars with sina historical session enrichment, got error: %v", err)
	}
	if len(bars) != 3 {
		t.Fatalf("expected three market bars, got %+v", bars)
	}
	if bars[1].EntryPrice != 136.20 || bars[1].AfternoonEntryPrice != 142.20 {
		t.Fatalf("expected custom bars to include sina historical session prices, got %+v", bars[1])
	}

	rows := buildAStockBacktestRows("2026-06-23", "afternoon", []aStockRecommendation{{Code: "002008", Name: "大族激光"}}, groupAStockMarketBars(bars))
	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %+v", rows)
	}
	if rows[0].AfternoonOpen != "142.20" || rows[0].T0Close != "145.11" || rows[0].T0Return != "+2.05%" {
		t.Fatalf("expected historical afternoon backtest to use sina 13:01 price, got %+v", rows[0])
	}
	if rows[0].Days[0].Close != "152.23" || rows[0].Days[0].Return != "+7.05%" {
		t.Fatalf("expected historical afternoon backtest to include T+1 return, got %+v", rows[0])
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

func TestAStockMarketBarsYahooFallbackStillSupplementsSessionPrices(t *testing.T) {
	custom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"items": []any{}}})
	}))
	defer custom.Close()

	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.300024" {
			t.Fatalf("unexpected eastmoney secid: %s", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("klt") {
		case "101":
			http.Error(w, "eastmoney daily unavailable", http.StatusBadGateway)
		case "1":
			w.Header().Set("Content-Type", "application/json")
			end := r.URL.Query().Get("end")
			switch {
			case strings.Contains(end, "09:30"):
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"klines": []string{
							"2026-06-24 09:30,16.00,16.03,0,0,0,0,0,0",
						},
					},
				})
			case strings.Contains(end, "13:01"):
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"klines": []string{
							"2026-06-24 13:01,16.05,16.08,0,0,0,0,0,0",
						},
					},
				})
			default:
				t.Fatalf("unexpected minute eastmoney query: %s", r.URL.RawQuery)
			}
		default:
			t.Fatalf("unexpected eastmoney klt: %s", r.URL.RawQuery)
		}
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
		if r.URL.Path != "/300024.SZ" {
			t.Fatalf("unexpected yahoo path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"chart":{"result":[{"timestamp":[%d,%d],"indicators":{"quote":[{"open":[16.79,16.26],"close":[16.79,16.42]}]}}],"error":null}}`,
			unixDay("2026-06-23"), unixDay("2026-06-24"))
	}))
	defer yahoo.Close()
	setAStockYahooChartURLForTest(t, yahoo.URL)

	srv := NewServer(config.Config{})
	bars, err := srv.loadAStockMarketBars("2026-06-24", []string{"300024"}, custom.URL)
	if err != nil {
		t.Fatalf("expected yahoo fallback bars with session enrichment, got error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected two market bars, got %+v", bars)
	}
	if bars[1].EntryPrice != 16.03 || bars[1].AfternoonEntryPrice != 16.08 {
		t.Fatalf("expected yahoo fallback bars to include session prices, got %+v", bars[1])
	}
	rows := buildAStockBacktestRows("2026-06-24", "afternoon", []aStockRecommendation{{Code: "300024", Name: "机器人"}}, groupAStockMarketBars(bars))
	if len(rows) != 1 || rows[0].AfternoonOpen != "16.08" || rows[0].T0Return != "+2.11%" {
		t.Fatalf("expected afternoon backtest to use session prices after yahoo fallback, got %+v", rows)
	}
}

func TestAStockRecommendationsUseTopThreeHotspotIndustries(t *testing.T) {
	recommendations := buildAStockRecommendations([]aStockHotspot{
		{Name: "黄金有色", Keywords: []string{"黄金"}, Score: 32, Evidence: 2},
		{Name: "消费电子", Keywords: []string{"华为"}, Score: 26, Evidence: 2},
		{Name: "新能源", Keywords: []string{"储能"}, Score: 25, Evidence: 1},
		{Name: "医药生物", Keywords: []string{"医药"}, Score: 19, Evidence: 1},
	}, []aStockMarketCandidate{
		{Code: "600547", Name: "山东黄金", Rank: 1, AuctionAmount: 9000000},
		{Code: "601899", Name: "紫金黄金", Rank: 2, AuctionAmount: 8000000},
		{Code: "600111", Name: "黄金稀土", Rank: 3, AuctionAmount: 7000000},
		{Code: "002475", Name: "华为精密", Rank: 4, AuctionAmount: 6000000},
		{Code: "000725", Name: "华为显示", Rank: 5, AuctionAmount: 5000000},
		{Code: "300433", Name: "华为科技", Rank: 6, AuctionAmount: 4000000},
		{Code: "300750", Name: "储能时代", Rank: 7, AuctionAmount: 3000000},
		{Code: "300274", Name: "储能电源", Rank: 8, AuctionAmount: 2000000},
		{Code: "601012", Name: "储能绿能", Rank: 9, AuctionAmount: 1000000},
		{Code: "600276", Name: "医药一号", Rank: 10, AuctionAmount: 900000},
	})

	if len(recommendations) != aStockDailyRecommendationLimit {
		t.Fatalf("expected %d recommendations by default, got %d", aStockDailyRecommendationLimit, len(recommendations))
	}
	if recommendations[0].Code != "600547" {
		t.Fatalf("expected strongest global score to rank first, got %+v", recommendations)
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
			if len(recommendations) == aStockDailyRecommendationLimit {
				continue
			}
			t.Fatalf("expected recommendations to include hotspot %q, got %+v", want, recommendations)
		}
	}
	wantCodes := map[string]struct{}{
		"600547": {},
		"600111": {},
		"002475": {},
		"000725": {},
		"300274": {},
	}
	for _, rec := range recommendations {
		if _, ok := wantCodes[rec.Code]; !ok {
			t.Fatalf("expected matched market candidate recommendation, got %+v from %+v", rec, recommendations)
		}
	}
}

func TestAStockHotspotsApplyNegativeNewsPenalty(t *testing.T) {
	hotspots := buildAStockHotspots([]model.Item{
		{ID: 1, Title: "半导体板块震荡走弱", Summary: "芯片存储冲高回落，普冉股份跌超10%"},
		{ID: 2, Title: "半导体板块拉升", Summary: "芯片存储需求走强"},
		{ID: 3, Title: "人工智能算力需求增长", Summary: "AI 大模型机器人产业链活跃"},
	})
	semiconductor := aStockHotspot{}
	for _, hotspot := range hotspots {
		if hotspot.Name == "半导体" {
			semiconductor = hotspot
			break
		}
	}
	if semiconductor.Name == "" {
		t.Fatalf("expected semiconductor hotspot, got %+v", hotspots)
	}
	if semiconductor.NegativeNewsCount != 1 || semiconductor.NegativeNewsPenalty != 40 {
		t.Fatalf("expected one negative news penalty, got %+v", semiconductor)
	}
	if semiconductor.Score != 0 {
		t.Fatalf("expected negative penalty to floor hotspot score at 0, got %+v", semiconductor)
	}
	if isAStockNegativeNewsItem(model.Item{Title: "半导体板块拉升", Summary: "芯片存储需求走强"}) {
		t.Fatal("expected positive news text not to trigger negative penalty")
	}

	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{semiconductor}, []aStockMarketCandidate{
		{Code: "688981", Name: "中芯半导体", Rank: 1, AuctionAmount: 9000000},
	}, 3, 3)
	if len(recommendations) == 0 {
		t.Fatalf("expected semiconductor matched recommendation, got %+v", recommendations)
	}
	if !strings.Contains(recommendations[0].Reason, "负面新闻 1 条，情绪扣分 40") {
		t.Fatalf("expected recommendation reason to include negative news penalty, got %+v", recommendations[0])
	}
}

func TestAStockRecommendationsGlobalSortBeforeLimit(t *testing.T) {
	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{
		{Name: "低分热点", Keywords: []string{"低分"}, Score: 40, Evidence: 1},
		{Name: "高分热点", Keywords: []string{"高分"}, Score: 160, Evidence: 8},
	}, []aStockMarketCandidate{
		{Code: "600001", Name: "低分股份", Rank: 1, AuctionAmount: 9000000},
		{Code: "600002", Name: "高分股份", Rank: 80, AuctionAmount: 8000000},
	}, 1, 1)

	if len(recommendations) != 1 || recommendations[0].Code != "600002" {
		t.Fatalf("expected full candidate pool to be sorted before final limit, got %+v", recommendations)
	}
}

func TestAStockRecommendationsPenalizeWeakFinancingEvidence(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "AI测试",
		Keywords: []string{"AI", "人工智能"},
		Score:    100,
		Evidence: 2,
		MatchedItems: []model.Item{
			{Title: "日发精机：融资净买入596.68万元，融资余额1.62亿元", TagFlags: "0.002520"},
			{Title: "瑞芯微上半年净利润同比增长", Summary: "AI芯片需求提升", TagFlags: "1.603893"},
		},
	}

	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{hotspot}, []aStockMarketCandidate{
		{Code: "002520", Name: "日发精机", Rank: 1, AuctionAmount: 9000000},
		{Code: "603893", Name: "瑞芯微", Rank: 20, AuctionAmount: 8000000},
	}, aStockReplacementPoolLimit, aStockReplacementPerHotspot)

	byCode := aStockTestRecommendationsByCode(recommendations)
	if _, ok := byCode["002520"]; !ok {
		t.Fatalf("expected weak evidence stock in recommendations, got %+v", recommendations)
	}
	if _, ok := byCode["603893"]; !ok {
		t.Fatalf("expected strong evidence stock in recommendations, got %+v", recommendations)
	}
	if recommendations[0].Code != "603893" {
		t.Fatalf("expected strong evidence stock to outrank weak financing evidence, got %+v", recommendations)
	}
	weak := byCode["002520"]
	if !strings.Contains(weak.Reason, "融资融券弱新闻 1 条，个股证据减分 25") {
		t.Fatalf("expected weak financing evidence penalty reason, got %+v", weak)
	}
}

func TestAStockRecommendationReasonRendersScoreBreakdownTable(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "人工智能",
		Code:        "002230",
		Name:        "科大讯飞",
		MarketScore: 115,
		Reason:      "命中 AI，证据新闻 3 条，热度分 36；个股证据 1 条，匹配分 79，综合分 115",
		ScoreBreakdown: []aStockRecommendationScoreComponent{
			{Label: "新闻热度", Detail: "证据新闻 3 条", UnitValue: 10, Score: 30},
			{Label: "热点关键词", Detail: "命中关键词 2 个", UnitValue: 3, Score: 6},
			{Label: "个股证据", Detail: "有效证据 1 条 / 总证据 1 条", UnitValue: 25, Score: 25},
			{Label: "行情排名", Detail: "排名 1", UnitValue: 40, Score: 40},
			{Label: "股票名命中", Detail: "命中关键词 1 个", UnitValue: 12, Score: 12},
			{Label: "资金动向", Detail: "资金加分 2", UnitValue: 2, Score: 2},
		},
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	for _, want := range []string{
		`class="astock-score-table"`,
		"<th>类别</th><th>项目</th><th>命中/依据</th><th>分值</th><th>小计</th>",
		"总分",
		"115/1000 分",
		"情绪因子",
		"新闻热度",
		"证据新闻 3 条",
		`class="astock-score-value">每条 10 分</td><td>30 分`,
		"竞价因子",
		"个股证据",
		"情绪因子：73/300 分",
		"竞价因子：40/120 分",
		"个股资金因子：2/220 分",
		"合计：115/1000 分",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected rendered score breakdown to contain %q, got %s", want, body)
		}
	}
}

func assertContainsInOrder(t *testing.T, body string, wants ...string) {
	t.Helper()
	offset := 0
	for _, want := range wants {
		index := strings.Index(body[offset:], want)
		if index < 0 {
			t.Fatalf("expected %q after offset %d, got %s", want, offset, body)
		}
		offset += index + len(want)
	}
}

func TestAStockRecommendationReasonUsesRequestedCategoryOrder(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "人工智能",
		Code:        "688041",
		Name:        "海光信息",
		MarketScore: 100,
		Reason:      "合成总分 100",
		ScoreBreakdown: []aStockRecommendationScoreComponent{
			{Factor: aStockScoreFactorEmotion, Label: "新闻热度", Detail: "证据新闻 2 条", UnitValue: 10, Score: 20},
			{Factor: aStockScoreFactorAuction, Label: "行情排名", Detail: "排名 10", UnitValue: 30, Score: 30},
			{Factor: aStockScoreFactorVolatility, Label: "动能趋势", Detail: "ADX转强", UnitValue: 5, Score: 5},
			{Factor: aStockScoreFactorFund, Label: "资金动向", Detail: "资金加分 10", UnitValue: 10, Score: 10},
			{Factor: aStockScoreFactorSector, Label: "板块资金趋势", Detail: "板块资金趋势加分 40", UnitValue: 40, Score: 40},
			{Factor: aStockScoreFactorHistory, Label: "总分修正", Detail: "历史修正 -5", UnitValue: -5, Score: -5},
		},
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	assertContainsInOrder(t, body,
		`<td>版块资金因子</td><td class="astock-score-category">板块资金趋势`,
		`<td>个股资金因子</td><td class="astock-score-category">资金动向`,
		`<td>情绪因子</td><td class="astock-score-category">新闻热度`,
		`<td>竞价因子</td><td class="astock-score-category">行情排名`,
		`<td>波动因子</td><td class="astock-score-category">动能趋势`,
		`<td>历史修正</td><td class="astock-score-category">总分修正`,
	)
	summaryStart := strings.Index(body, `<div class="astock-score-summary">`)
	if summaryStart < 0 {
		t.Fatalf("expected score summary, got %s", body)
	}
	assertContainsInOrder(t, body[summaryStart:],
		"版块资金因子：40/220 分",
		"个股资金因子：10/220 分",
		"情绪因子：20/300 分",
		"竞价因子：30/120 分",
		"波动因子：5/140 分",
		"历史修正：-5 分",
		"合计：100/1000 分",
	)
}

func TestAStockRecommendationSubsectionMergesFundFlowAndReasonColumns(t *testing.T) {
	ctx := aStockContext{
		Period:      "morning",
		PeriodLabel: "上午推荐",
		Recommendations: []aStockRecommendation{{
			Rank:            1,
			Hotspot:         "人工智能",
			Code:            "688041",
			Name:            "海光信息",
			PrevClose:       "320.40",
			PrevPct:         "-1.14%",
			PrevPctClass:    "astock-down",
			Change30:        "+3.95%",
			Change30Class:   "astock-up",
			Change60:        "+5.52%",
			Change60Class:   "astock-up",
			CurrentPrice:    "310.20",
			TodayPct:        "-3.18%",
			TodayPctClass:   "astock-down",
			FundFlow5D:      "+9.49亿",
			FundFlow5DClass: "astock-up",
			MarketScore:     129,
			ScoreBreakdown: []aStockRecommendationScoreComponent{
				{Factor: aStockScoreFactorEmotion, Label: "新闻热度", Detail: "证据新闻 16 条", UnitValue: 10, Score: 160},
				{Factor: aStockScoreFactorFund, Label: "资金动向", Detail: "5日主力资金净流入 +9.49亿", UnitValue: 10, Score: 10},
			},
		}},
	}
	var b strings.Builder
	renderAStockRecommendationSubsection(&b, ctx)
	body := b.String()
	for _, want := range []string{
		"<th>5日资金动向</th><th>推荐理由</th>",
		`class="astock-recommendation-detail-cell" colspan="2"`,
		`class="astock-recommendation-fundflow"`,
		"+9.49亿",
		`class="astock-score-table"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected merged recommendation detail cell to contain %q, got %s", want, body)
		}
	}
}

func TestAStockRecommendationReasonRendersTotal317Breakdown(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "半导体",
		Code:        "002156",
		Name:        "通富微电",
		MarketScore: 317,
		Reason:      "合成总分 317",
		ScoreBreakdown: []aStockRecommendationScoreComponent{
			{Label: "新闻热度", Detail: "证据新闻 15 条", UnitValue: 10, Score: 150},
			{Label: "热点关键词", Detail: "命中关键词 4 个", UnitValue: 3, Score: 12},
			{Label: "板块资金趋势", Detail: "最近2日连续净流入", UnitValue: 40, Score: 40},
			{Label: "行情排名", Detail: "排名 1", UnitValue: 65, Score: 65},
			{Label: "个股证据", Detail: "有效证据 1 条 / 总证据 1 条", UnitValue: 25, Score: 25},
			{Label: "股票名命中", Detail: "命中关键词 1 个", UnitValue: 12, Score: 12},
			{Label: "资金动向", Detail: "5日主力资金净流入 +41.56亿，资金加分 13", UnitValue: 13, Score: 13},
		},
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	for _, want := range []string{
		"总分",
		"317/1000 分",
		"每条 10 分",
		"每个 3 分",
		"每条 25 分",
		"情绪因子：199/300 分",
		"竞价因子：65/120 分",
		"版块资金因子：40/220 分",
		"个股资金因子：13/220 分",
		"合计：317/1000 分",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected 317 score breakdown to contain %q, got %s", want, body)
		}
	}
}

func TestAStockRecommendationReasonMergesSavedAndParsedBreakdown(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "半导体",
		Code:        "002156",
		Name:        "通富微电",
		MarketScore: 317,
		Reason:      "命中 半导体、存储、晶圆、芯片，证据新闻 17 条，热度分 152；行情排名 1，成交额 8.19亿，个股证据 1 条，行情分 65，综合分 217，负面新闻 1 条，板块减分 30，生成点 09:27，5日主力资金净流入 +41.56亿，板块资金趋势加分 15，板块共振加分 25",
		ScoreBreakdown: []aStockRecommendationScoreComponent{
			{Label: "板块资金趋势", Detail: "芯片概念 5日主力资金净流入 +154.25亿，板块资金趋势加分 15", UnitValue: 15, Score: 15},
			{Label: "板块资金共振", Detail: "通富微电为电子、集成电路封测、科技风格主力净流入代表股，板块共振加分 25", UnitValue: 25, Score: 25},
			{Label: "总分修正", Detail: "保存总分 317", UnitValue: 277, Score: 277},
		},
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	for _, want := range []string{
		"总分",
		"317/1000 分",
		"新闻热度",
		"证据新闻 17 条",
		"热点关键词",
		"命中关键词 4 个",
		"行情排名",
		"排名 1",
		"个股证据",
		"个股证据 1 条",
		"负面新闻",
		"负面新闻 1 条",
		"-30 分",
		"情绪因子",
		"竞价因子",
		"版块资金因子",
		"合计：317/1000 分",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected merged score breakdown to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "保存总分 317</td><td class=\"astock-score-value\">277 分") {
		t.Fatalf("expected stale saved total correction to be recalculated instead of rendered directly, got %s", body)
	}
	if strings.Contains(body, "<td>小计</td>") {
		t.Fatalf("expected subtotal rows to stay out of score table, got %s", body)
	}
	assertContainsInOrder(t, body,
		`<td>版块资金因子</td><td class="astock-score-category">板块资金趋势`,
		`<td>情绪因子</td><td class="astock-score-category">热点关键词`,
		`<td>情绪因子</td><td class="astock-score-category">负面新闻`,
	)
}

func TestAStockRecommendationReasonParsesHighOpenAndMomentumBonuses(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "人工智能",
		Code:        "600001",
		Name:        "高开动能",
		MarketScore: 195,
		Reason:      "09:30 较 昨日收盘 高开 +8.00%，高开加分 65，动能趋势加分 30（布林突破 + 均线趋势）",
		ScoreBreakdown: []aStockRecommendationScoreComponent{
			{Label: "新闻热度", Detail: "证据新闻 10 条", UnitValue: 10, Score: 100},
		},
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	for _, want := range []string{
		"当日高开",
		"09:30 较 昨日收盘 高开 +8.00%",
		"65 分",
		"动能趋势",
		"布林突破 + 均线趋势",
		"30 分",
		"合计：195/1000 分",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected high-open/momentum score breakdown to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "<td>小计</td>") {
		t.Fatalf("expected subtotal rows to stay out of score table, got %s", body)
	}
}

func TestAStockRecommendationReasonParsesUnitValueForScoreBreakdownTable(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "金融券商",
		Code:        "600036",
		Name:        "招商银行",
		MarketScore: 172,
		Reason:      "命中 证券、银行，证据新闻 12 条，热度分 126；行情排名 143，成交额 3671.00万，个股证据 1 条，行情分 36，综合分 162，生成点 09:27，5日主力资金净流入 +10.34亿，资金加分 10",
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	for _, want := range []string{
		"172/1000 分",
		"<th>类别</th><th>项目</th><th>命中/依据</th><th>分值</th><th>小计</th>",
		"证据新闻 12 条",
		`class="astock-score-value">每条 10 分</td><td>120 分`,
		"命中关键词 2 个",
		`class="astock-score-value">每个 3 分</td><td>6 分`,
		"排名 143",
		`class="astock-score-value">64 分</td><td>64 分`,
		"个股证据 1 条",
		`class="astock-score-value">每条 25 分</td><td>25 分`,
		"情绪因子：151/300 分",
		"竞价因子：64/120 分",
		"个股资金因子：10/220 分",
		"历史修正：-53 分",
		"合计：172/1000 分",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected rendered parsed score breakdown to contain %q, got %s", want, body)
		}
	}
}

func TestAStockRecommendationReasonExplainsMatchedScoreDelta(t *testing.T) {
	rec := aStockRecommendation{
		Hotspot:     "金融券商",
		Code:        "000166",
		Name:        "申万宏源",
		MarketScore: 475,
		Reason:      "命中 保险、券商、证券、资本市场、银行，证据新闻 37 条，热度分 295；使用实时新闻明确提及股票，个股证据 3 条，匹配分 170，综合分 465，负面新闻 3 条，板块减分 90，生成点 12:57，银行 5日主力资金净流入 +13.78亿，板块资金趋势加分 5，银行 5日主力资金净流入 +13.62亿，状态震荡偏流入，10日主力资金+43.18亿，近5日净流入3天，当日净流出且板块下跌，资金方向震荡，板块资金趋势加分 5",
	}
	var b strings.Builder
	writeAStockRecommendationReasonCell(&b, rec)
	body := b.String()
	for _, want := range []string{
		"总分",
		"475/1000 分",
		"情绪因子",
		"新闻热度",
		"证据新闻 37 条",
		"负面新闻 3 条，板块减分 90",
		"-90 分",
		"情绪因子：300/300 分",
		"历史修正：175 分",
		"合计：475/1000 分",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected matched score delta explanation to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "展示匹配分 170") {
		t.Fatalf("expected formula explanation instead of opaque matched score detail, got %s", body)
	}
}

func TestAStockRecommendationsFilterExDividendEvents(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/dividend-events" {
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
		if got := r.URL.Query().Get("date"); got != "2026-07-13" {
			t.Fatalf("unexpected date: %s", got)
		}
		if got := r.URL.Query().Get("window_days"); got != "3" {
			t.Fatalf("unexpected window_days: %s", got)
		}
		codes := strings.Split(r.URL.Query().Get("codes"), ",")
		codeSet := map[string]struct{}{}
		for _, code := range codes {
			codeSet[code] = struct{}{}
		}
		for _, want := range []string{"600036", "600879", "002230"} {
			if _, ok := codeSet[want]; !ok {
				t.Fatalf("expected dividend query to include %s, got %s", want, r.URL.RawQuery)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(aStockDividendEventResult{
			Items: []aStockDividendEvent{
				{Code: "600036", Name: "招商银行", ExDate: "2026-07-10", DividendDate: "2026-07-10", Description: "10派10.03元(含税)", Source: "stock_dividend_cninfo"},
			},
			Date:  "2026-07-13",
			Start: "2026-07-10",
			End:   "2026-07-16",
		})
	}))
	defer akshare.Close()

	srv := NewServer(config.Config{AStockAuctionURL: akshare.URL})
	ctx := aStockContext{
		Date: "2026-07-13",
		Recommendations: []aStockRecommendation{
			{Rank: 1, Code: "600036", Name: "招商银行"},
			{Rank: 2, Code: "600879", Name: "XD航天电"},
			{Rank: 3, Code: "002230", Name: "科大讯飞"},
		},
	}

	skipped := srv.applyAStockExDividendFilterWithCache(&ctx, newAStockRequestCache())
	if skipped != 2 || ctx.ExDividendFiltered != 2 {
		t.Fatalf("expected two ex-dividend recommendations filtered, skipped=%d ctx=%+v", skipped, ctx)
	}
	if len(ctx.Recommendations) != 1 || ctx.Recommendations[0].Code != "002230" || ctx.Recommendations[0].Rank != 1 {
		t.Fatalf("expected only reranked 002230 to remain, got %+v", ctx.Recommendations)
	}
}

func TestAStockRecommendationsOutputFiltersApplyDailyLimit(t *testing.T) {
	ctx := aStockContext{
		Date: "2026-07-13",
		Recommendations: []aStockRecommendation{
			{Rank: 1, Code: "600000", Name: "浦发银行"},
			{Rank: 2, Code: "600001", Name: "邯郸钢铁"},
			{Rank: 3, Code: "600002", Name: "齐鲁石化"},
			{Rank: 4, Code: "600003", Name: "东北高速"},
			{Rank: 5, Code: "600004", Name: "白云机场"},
			{Rank: 6, Code: "600005", Name: "武钢股份"},
		},
	}

	srv := NewServer(config.Config{})
	exDividendFiltered, dailyLimitFiltered := srv.applyAStockRecommendationOutputFiltersWithCache(&ctx, newAStockRequestCache())
	if exDividendFiltered != 0 || dailyLimitFiltered != 1 || ctx.SameDayMorningFiltered != 1 {
		t.Fatalf("expected only daily limit to filter one recommendation, ex=%d daily=%d ctx=%+v", exDividendFiltered, dailyLimitFiltered, ctx)
	}
	if len(ctx.Recommendations) != aStockDailyRecommendationLimit {
		t.Fatalf("expected final recommendations capped at %d, got %+v", aStockDailyRecommendationLimit, ctx.Recommendations)
	}
	if ctx.Recommendations[len(ctx.Recommendations)-1].Rank != aStockDailyRecommendationLimit {
		t.Fatalf("expected recommendations to be reranked, got %+v", ctx.Recommendations)
	}
}

func TestAStockRecommendationsDoNotInferFundMonitorTitleAsStockName(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "人工智能",
		Keywords: []string{"AI", "人工智能"},
		Score:    100,
		Evidence: 1,
		MatchedItems: []model.Item{{
			Title:    "主力资金监控：深科技获主力资金净流入",
			Summary:  "人工智能产业链资金异动",
			TagFlags: "0.000021",
		}},
	}

	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{hotspot}, []aStockMarketCandidate{
		{Code: "000021", Name: "深科技", Rank: 1, AuctionAmount: 9000000},
	}, aStockReplacementPoolLimit, aStockReplacementPerHotspot)

	got := aStockTestRecommendationsByCode(recommendations)["000021"]
	if got.Code != "000021" {
		t.Fatalf("expected 000021 recommendation, got %+v", recommendations)
	}
	if got.Name != "深科技" {
		t.Fatalf("expected inferred monitor title to be replaced by market name 深科技, got %+v", got)
	}
	if isAStockRecommendationPlaceholderName(got.Name) || isInvalidAStockRecommendationName(got.Name) {
		t.Fatalf("expected repaired valid stock name, got %+v", got)
	}
}

func TestAStockRecommendationsDoNotUseNewsPhraseAsStockName(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "半导体",
		Keywords: []string{"半导体", "芯片", "存储"},
		Score:    100,
		Evidence: 1,
		MatchedItems: []model.Item{{
			Title:    "存储芯片大幅高开，德明利涨停",
			Summary:  "半导体板块活跃",
			TagFlags: "0.001309",
		}},
	}

	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{hotspot}, []aStockMarketCandidate{
		{Code: "001309", Name: "德明利", Rank: 1, AuctionAmount: 9000000},
	}, aStockReplacementPoolLimit, aStockReplacementPerHotspot)

	got := aStockTestRecommendationsByCode(recommendations)["001309"]
	if got.Code != "001309" {
		t.Fatalf("expected 001309 recommendation, got %+v", recommendations)
	}
	if got.Name != "德明利" {
		t.Fatalf("expected news phrase name to be replaced by market name 德明利, got %+v", got)
	}
	if !isInvalidAStockRecommendationName("存储芯片大幅高开") {
		t.Fatal("expected intraday news phrase to be treated as invalid stock name")
	}
}

func TestAStockRecommendationsExcludeNegativeOnlyStockEvidence(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "半导体测试",
		Keywords: []string{"半导体", "芯片"},
		Score:    120,
		Evidence: 2,
		MatchedItems: []model.Item{
			{Title: "半导体板块冲高回落，芯原股份跟跌", Summary: "晶合集成跌超10%，芯原股份下挫", TagFlags: "0.688521"},
			{Title: "芯片行业午后活跃", Summary: "半导体设备需求改善"},
		},
	}

	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{hotspot}, []aStockMarketCandidate{
		{Code: "688521", Name: "芯原股份", Rank: 1, AuctionAmount: 9000000},
		{Code: "688981", Name: "中芯国际", Rank: 2, AuctionAmount: 8000000},
	}, aStockReplacementPoolLimit, aStockReplacementPerHotspot)

	if _, ok := aStockTestRecommendationsByCode(recommendations)["688521"]; ok {
		t.Fatalf("expected negative-only stock evidence to exclude 688521, got %+v", recommendations)
	}
}

func TestAStockRecommendationsFilterNegativeNewsWithoutStrongEvidence(t *testing.T) {
	recommendations := []aStockRecommendation{
		{
			Rank:         1,
			Code:         "601288",
			Name:         "农业银行",
			Hotspot:      "金融券商",
			HotspotScore: 120,
			MarketScore:  161,
			ScoreBreakdown: []aStockRecommendationScoreComponent{
				{Label: "新闻热度", Detail: "证据新闻 24 条", UnitValue: 10, Score: 240},
				{Label: "负面新闻", Detail: "负面新闻 4 条", UnitValue: -120, Score: -120},
				{Label: "个股证据", Detail: "有效证据 0 条 / 总证据 0 条", UnitValue: 25, Score: 0},
			},
		},
		{
			Rank:         2,
			Code:         "300433",
			Name:         "蓝思科技",
			Hotspot:      "人工智能",
			HotspotScore: 120,
			MarketScore:  203,
			ScoreBreakdown: []aStockRecommendationScoreComponent{
				{Label: "新闻热度", Detail: "证据新闻 12 条", UnitValue: 10, Score: 120},
				{Label: "负面新闻", Detail: "负面新闻 1 条", UnitValue: -30, Score: -30},
				{Label: "个股证据", Detail: "有效证据 1 条 / 总证据 1 条", UnitValue: 25, Score: 25},
			},
		},
	}

	filtered, skipped := filterAStockNegativeNoEvidenceRecommendations(recommendations, nil)
	if skipped != 1 {
		t.Fatalf("expected one negative/no-evidence recommendation to be filtered, skipped=%d filtered=%+v", skipped, filtered)
	}
	if _, ok := aStockTestRecommendationsByCode(filtered)["601288"]; ok {
		t.Fatalf("expected 农业银行 to be filtered, got %+v", filtered)
	}
	if len(filtered) != 1 || filtered[0].Code != "300433" || filtered[0].Rank != 1 {
		t.Fatalf("expected negative stock with strong evidence to remain and rerank, got %+v", filtered)
	}
	if status := formatAStockNegativeNoEvidenceFilterStatus(skipped); !strings.Contains(status, "过滤负面无个股证据股票 1") {
		t.Fatalf("expected negative filter status, got %q", status)
	}
}

func TestAStockMarketBarsScoreMorningLowOpen(t *testing.T) {
	recommendations := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002520", Name: "日发精机", HotspotScore: 100, MarketScore: 100, Reason: "弱盘口"},
		{Rank: 2, Hotspot: "人工智能", Code: "603893", Name: "瑞芯微", HotspotScore: 100, MarketScore: 100, Reason: "正常盘口"},
	}
	bars := []aStockMarketBar{
		{Code: "002520", Date: "2026-07-07", Close: 10},
		{Code: "002520", Date: "2026-07-08", Open: 9.75, EntryPrice: 9.75, Close: 9.90},
		{Code: "603893", Date: "2026-07-07", Close: 10},
		{Code: "603893", Date: "2026-07-08", Open: 10.00, EntryPrice: 10.00, Close: 10.10},
	}

	got, _, status, _, _ := applyAStockMarketBars("2026-07-08", "morning", recommendations, bars, false, false, 0)

	if len(got) != 2 {
		t.Fatalf("expected low-open recommendation to remain with penalty, got %+v", got)
	}
	weak := mustAStockRecommendationForTest(t, got, "002520")
	if weak.MarketScore != 80 {
		t.Fatalf("expected low-open stock to deduct 20 points, got %+v", weak)
	}
	component := mustAStockScoreComponentForTest(t, weak, "当日低开")
	if component.Score != -20 || !strings.Contains(component.Detail, "09:30 较 昨日收盘 低开 -2.50%") {
		t.Fatalf("expected low-open score component, got %+v", component)
	}
	if strings.Contains(status, "过滤低开股票") {
		t.Fatalf("expected low-open hard filter to be disabled, got %q", status)
	}
}

func TestAStockLowOpenPenaltyBoundaries(t *testing.T) {
	tests := []struct {
		openPct float64
		want    int
	}{
		{-0.99, 0},
		{-1.00, 10},
		{-2.00, 20},
		{-3.00, 30},
		{-4.00, 40},
		{-5.00, 50},
		{-5.01, 65},
		{-10.00, 65},
		{-10.01, 65},
	}
	for _, tc := range tests {
		if got := aStockLowOpenPenaltyScore(tc.openPct); got != tc.want {
			t.Fatalf("expected low-open %.2f%% penalty %d, got %d", tc.openPct, tc.want, got)
		}
	}
}

func TestAStockMarketBarsScoreMorningHighOpen(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600001", Name: "边界下方", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 2, Hotspot: "人工智能", Code: "600002", Name: "一档高开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 3, Hotspot: "人工智能", Code: "600003", Name: "二档高开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 4, Hotspot: "人工智能", Code: "600004", Name: "三档高开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 5, Hotspot: "人工智能", Code: "600005", Name: "四档高开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 6, Hotspot: "人工智能", Code: "600006", Name: "五档高开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 7, Hotspot: "人工智能", Code: "600007", Name: "强高开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
	})
	entryPrices := map[string]float64{
		"600001": 100.99,
		"600002": 101.00,
		"600003": 102.00,
		"600004": 103.00,
		"600005": 104.00,
		"600006": 105.00,
		"600007": 105.01,
	}
	bars := make([]aStockMarketBar, 0, len(recommendations)*2)
	for _, rec := range recommendations {
		bars = append(bars,
			aStockMarketBar{Code: rec.Code, Date: "2026-07-13", Close: 100, Pct: 0},
			aStockMarketBar{Code: rec.Code, Date: "2026-07-14", Open: entryPrices[rec.Code], EntryPrice: entryPrices[rec.Code], Close: entryPrices[rec.Code], Pct: 2},
		)
	}

	got, _, _, _, _ := applyAStockMarketBars("2026-07-14", "morning", recommendations, bars, false, false, 0)
	wantScores := map[string]int{
		"600001": 100,
		"600002": 100 + aStockHighOpenScore(1.00),
		"600003": 100 + aStockHighOpenScore(2.00),
		"600004": 100 + aStockHighOpenScore(3.00),
		"600005": 100 + aStockHighOpenScore(4.00),
		"600006": 100 + aStockHighOpenScore(5.00),
		"600007": 100 + aStockHighOpenScore(5.01),
	}
	for code, want := range wantScores {
		rec := mustAStockRecommendationForTest(t, got, code)
		if rec.MarketScore != want {
			t.Fatalf("expected %s score %d, got %+v", code, want, rec)
		}
		if code != "600001" {
			component := mustAStockScoreComponentForTest(t, rec, "当日高开")
			if component.Score != want-100 || !strings.Contains(component.Detail, "09:30 较 昨日收盘 高开") {
				t.Fatalf("expected morning high-open score component for %s, got %+v", code, component)
			}
		}
	}
	if got[0].Code != "600007" {
		t.Fatalf("expected strongest high-open stock to rank first, got %+v", got)
	}
	defaults := defaultAStockAlgorithmSettings()
	if aStockHighOpenScore(5.00) != defaults.Auction.HighOpenScore5 || aStockHighOpenScore(5.01) != defaults.Auction.HighOpenStrongScore {
		t.Fatalf("expected high-open boundary scores to use configured fifth and strong tiers")
	}
}

func TestAStockMarketBarsScoreAfternoonHighOpenFromMiddayBaseline(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600011", Name: "午间基准", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 2, Hotspot: "人工智能", Code: "600012", Name: "午间回退", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 3, Hotspot: "人工智能", Code: "600013", Name: "无午间价", HotspotScore: 100, MarketScore: 100, Reason: "base"},
	})
	bars := []aStockMarketBar{
		{Code: "600011", Date: "2026-07-13", Close: 99, Pct: 0},
		{Code: "600011", Date: "2026-07-14", AfternoonEntryPrice: 102.13, Close: 102.50, Pct: 2.5, SessionPrices: map[string]float64{"12:30": 100}},
		{Code: "600012", Date: "2026-07-13", Close: 99, Pct: 0},
		{Code: "600012", Date: "2026-07-14", AfternoonEntryPrice: 105.01, Close: 105.20, Pct: 3.0, SessionPrices: map[string]float64{"11:30": 100}},
		{Code: "600013", Date: "2026-07-13", Close: 99, Pct: 0},
		{Code: "600013", Date: "2026-07-14", AfternoonEntryPrice: 106.00, Close: 106.10, Pct: 3.5},
	}

	got, _, _, _, _ := applyAStockMarketBars("2026-07-14", "afternoon", recommendations, bars, false, false, 0)
	midday := mustAStockRecommendationForTest(t, got, "600011")
	if midday.MarketScore != 130 {
		t.Fatalf("expected 12:30 baseline high-open score +30, got %+v", midday)
	}
	middayComponent := mustAStockScoreComponentForTest(t, midday, "当日高开")
	if middayComponent.Score != 30 || !strings.Contains(middayComponent.Detail, "13:01 较 12:30 高开 +2.13%") {
		t.Fatalf("expected 12:30 high-open component, got %+v", middayComponent)
	}
	fallback := mustAStockRecommendationForTest(t, got, "600012")
	if fallback.MarketScore != 200 {
		t.Fatalf("expected 11:30 fallback high-open score +100, got %+v", fallback)
	}
	fallbackComponent := mustAStockScoreComponentForTest(t, fallback, "当日高开")
	if fallbackComponent.Score != 100 || !strings.Contains(fallbackComponent.Detail, "13:01 较 11:30 高开") {
		t.Fatalf("expected 11:30 fallback high-open component, got %+v", fallbackComponent)
	}
	noBase := mustAStockRecommendationForTest(t, got, "600013")
	if noBase.MarketScore != 100 {
		t.Fatalf("expected no midday baseline to skip high-open score, got %+v", noBase)
	}
}

func TestAStockMarketBarsScoreAfternoonLowOpenFromMiddayBaseline(t *testing.T) {
	recommendations := initializeAStockRecommendationMarket([]aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "600021", Name: "午间低开", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 2, Hotspot: "人工智能", Code: "600022", Name: "低开回退", HotspotScore: 100, MarketScore: 100, Reason: "base"},
		{Rank: 3, Hotspot: "人工智能", Code: "600023", Name: "无午间价", HotspotScore: 100, MarketScore: 100, Reason: "base"},
	})
	bars := []aStockMarketBar{
		{Code: "600021", Date: "2026-07-13", Close: 99, Pct: 0},
		{Code: "600021", Date: "2026-07-14", AfternoonEntryPrice: 97.87, Close: 98.20, Pct: -1.5, SessionPrices: map[string]float64{"12:30": 100}},
		{Code: "600022", Date: "2026-07-13", Close: 99, Pct: 0},
		{Code: "600022", Date: "2026-07-14", AfternoonEntryPrice: 94.99, Close: 95.20, Pct: -4.0, SessionPrices: map[string]float64{"11:30": 100}},
		{Code: "600023", Date: "2026-07-13", Close: 99, Pct: 0},
		{Code: "600023", Date: "2026-07-14", AfternoonEntryPrice: 94.00, Close: 94.10, Pct: -5.0},
	}

	got, _, _, _, _ := applyAStockMarketBars("2026-07-14", "afternoon", recommendations, bars, false, false, 0)
	midday := mustAStockRecommendationForTest(t, got, "600021")
	if midday.MarketScore != 80 {
		t.Fatalf("expected 12:30 baseline low-open score -20, got %+v", midday)
	}
	middayComponent := mustAStockScoreComponentForTest(t, midday, "当日低开")
	if middayComponent.Score != -20 || !strings.Contains(middayComponent.Detail, "13:01 较 12:30 低开 -2.13%") {
		t.Fatalf("expected 12:30 low-open component, got %+v", middayComponent)
	}
	fallback := mustAStockRecommendationForTest(t, got, "600022")
	if fallback.MarketScore != 35 {
		t.Fatalf("expected 11:30 fallback low-open score -65, got %+v", fallback)
	}
	fallbackComponent := mustAStockScoreComponentForTest(t, fallback, "当日低开")
	if fallbackComponent.Score != -65 || !strings.Contains(fallbackComponent.Detail, "13:01 较 11:30 低开") {
		t.Fatalf("expected 11:30 fallback low-open component, got %+v", fallbackComponent)
	}
	noBase := mustAStockRecommendationForTest(t, got, "600023")
	if noBase.MarketScore != 100 {
		t.Fatalf("expected no midday baseline to skip low-open score, got %+v", noBase)
	}
}

func TestAStockFundFlowMedianPenaltyWithinHotspot(t *testing.T) {
	recommendations := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "002520", Name: "日发精机", HotspotScore: 100, MarketScore: 100, Reason: "资金偏弱"},
		{Rank: 2, Hotspot: "人工智能", Code: "603893", Name: "瑞芯微", HotspotScore: 100, MarketScore: 100, Reason: "资金较强"},
		{Rank: 3, Hotspot: "人工智能", Code: "688385", Name: "复旦微电", HotspotScore: 100, MarketScore: 100, Reason: "资金中位"},
	}
	assessments := map[string]aStockFundFlow5DAssessment{
		"002520": {Total: 80000000},
		"603893": {Total: 1800000000},
		"688385": {Total: 300000000},
	}

	got := applyAStockFundFlowMedianPenaltyToRecommendations(recommendations, assessments)
	byCode := aStockTestRecommendationsByCode(got)

	weak := byCode["002520"]
	if weak.MarketScore != 50 || !strings.Contains(weak.Reason, "5日资金低于同热点中位数 +3.00亿，资金强度减分 50") {
		t.Fatalf("expected below-median fund-flow penalty, got %+v", weak)
	}
	if byCode["603893"].MarketScore != 100 || byCode["688385"].MarketScore != 100 {
		t.Fatalf("expected median and above-median stocks to keep score, got %+v", got)
	}
}

func TestAStockFundFlowScoreStrongInflowCapsAtFifty(t *testing.T) {
	assessment := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:  2100000000,
		Total10D: 2100000000,
	})
	rec := applyAStockFundFlow5DAssessmentToRecommendation(aStockRecommendation{Code: "300001", HotspotScore: 100, MarketScore: 100, Reason: "base"}, assessment, true)

	if assessment.ScoreDelta != 145 || rec.MarketScore != 245 || !strings.Contains(rec.Reason, "资金加分 145") {
		t.Fatalf("expected strong inflow to use new +145 fund score, assessment=%+v rec=%+v", assessment, rec)
	}
}

func TestAStockFundFlowScoreUsesConfiguredScores(t *testing.T) {
	settings := defaultAStockAlgorithmSettings()
	settings.Fund.ExtremeScore = 42
	settings.Fund.ScoreMax = 60
	assessment := scoreAStockFundFlowAssessmentWithSettings(aStockFundFlow5DAssessment{
		Total5D:  2100000000,
		Total10D: 2100000000,
	}, settings)

	if assessment.ScoreDelta != 60 {
		t.Fatalf("expected configured extreme score plus acceleration, got %+v", assessment)
	}
}

func TestAStockFundFlowScoreContinuousOutflowCapsAtMinusFifty(t *testing.T) {
	assessment := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:        -10000000,
		NegativeDays5D: 4,
	})

	if assessment.ScoreDelta != -100 || !strings.Contains(formatAStockFundFlowScoreReason(assessment), "近5日净流出4天") {
		t.Fatalf("expected continuous outflow to use new -100 penalty, got %+v", assessment)
	}
}

func TestAStockSectorTopStockResonanceUsesConfiguredScores(t *testing.T) {
	settings := defaultAStockAlgorithmSettings()
	settings.Sector.TopStockThreeSectorScore = 18
	settings.Sector.TopStockLargeInflowScore = 4
	settings.Sector.TopStockScoreCap = 21
	score := scoreAStockSectorTopStockResonanceWithSettings(aStockSectorTopStockResonance{
		PositiveSectorCount:      3,
		TotalSectorMainNetInflow: 6000000000,
	}, settings)

	if score != 21 {
		t.Fatalf("expected configured resonance score capped at 21, got %d", score)
	}
}

func TestAStockHighOpenScoreUsesConfiguredScores(t *testing.T) {
	settings := defaultAStockAlgorithmSettings()
	settings.Auction.HighOpenThreshold3Pct = 2.5
	settings.Auction.HighOpenScore3 = 33
	if score := aStockHighOpenScoreWithSettings(2.8, settings); score != 33 {
		t.Fatalf("expected configured high-open score 33, got %d", score)
	}
}

func TestAStockLowOpenPenaltyUsesConfiguredScores(t *testing.T) {
	settings := defaultAStockAlgorithmSettings()
	settings.Auction.LowOpenThreshold3Pct = 2.5
	settings.Auction.LowOpenPenalty3 = 33
	if penalty := aStockLowOpenPenaltyScoreWithSettings(-2.8, settings); penalty != 33 {
		t.Fatalf("expected configured low-open penalty 33, got %d", penalty)
	}
}

func TestAStockFundFlowScoreTenDayPositiveFiveDayOutflow(t *testing.T) {
	assessment := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:        -10000000,
		Total10D:       100000000,
		NegativeDays5D: 1,
	})

	if assessment.ScoreDelta != -70 || !strings.Contains(formatAStockFundFlowScoreReason(assessment), "资金退潮减分 70") {
		t.Fatalf("expected 10d inflow but 5d outflow to be penalized, got %+v", assessment)
	}
}

func TestAStockFundFlowScoreTenDayOutflowFiveDayRepairCapsPositive(t *testing.T) {
	assessment := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:  2100000000,
		Total10D: -100000000,
	})

	if assessment.ScoreDelta != 20 || !strings.Contains(formatAStockFundFlowScoreReason(assessment), "资金修复加分上限20") {
		t.Fatalf("expected 10d outflow but 5d repair to cap at +10, got %+v", assessment)
	}
}

func TestAStockFundFlowScoreSectorWeakCapsBonus(t *testing.T) {
	weak := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:    2100000000,
		Total10D:   2100000000,
		SectorWeak: true,
	})
	outflow := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:          2100000000,
		Total10D:         2100000000,
		SectorNetOutflow: true,
	})

	if weak.ScoreDelta != 20 {
		t.Fatalf("expected weak sector to cap fund bonus at +20, got %+v", weak)
	}
	if outflow.ScoreDelta != 0 {
		t.Fatalf("expected sector net outflow to cap fund bonus at 0, got %+v", outflow)
	}
}

func TestAStockSectorFundFlowTrendScoreStrongInflowCapsAtForty(t *testing.T) {
	assessment := scoreAStockSectorFundFlowTrendAssessment(aStockHotspotSectorAlias{SectorType: "概念资金流", SectorName: "人工智能"}, []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-10", Rank: 6, Name: "人工智能", MainNetInflow: 120000000, ChangePct: 1.2},
		{TradeDate: "2026-07-09", Rank: 8, Name: "人工智能", MainNetInflow: 90000000, ChangePct: 0.8},
		{TradeDate: "2026-07-08", Rank: 12, Name: "人工智能", MainNetInflow: 80000000, ChangePct: 0.4},
		{TradeDate: "2026-07-07", Rank: 14, Name: "人工智能", MainNetInflow: -10000000, ChangePct: -0.2},
		{TradeDate: "2026-07-06", Rank: 18, Name: "人工智能", MainNetInflow: 70000000, ChangePct: 0.6},
		{TradeDate: "2026-07-03", Rank: 22, Name: "人工智能", MainNetInflow: 100000000, ChangePct: 0.3},
	})
	rec := applyAStockSectorFundFlowTrendAssessmentToRecommendation(aStockRecommendation{Hotspot: "人工智能", Code: "300001", HotspotScore: 100, MarketScore: 100, Reason: "base"}, assessment)

	if assessment.ScoreDelta != 120 || assessment.Status != "连续流入" || rec.MarketScore != 220 {
		t.Fatalf("expected strong sector trend to cap at +120, assessment=%+v rec=%+v", assessment, rec)
	}
	if !strings.Contains(rec.Reason, "板块资金趋势加分 120") || !strings.Contains(rec.Reason, "最近2日连续净流入") {
		t.Fatalf("expected sector trend reason to be appended, got %+v", rec)
	}
	if len(rec.ScoreBreakdown) == 0 || rec.ScoreBreakdown[len(rec.ScoreBreakdown)-1].Label != "板块资金趋势" || rec.ScoreBreakdown[len(rec.ScoreBreakdown)-1].Score != 120 {
		t.Fatalf("expected sector trend score breakdown, got %+v", rec.ScoreBreakdown)
	}
}

func TestAStockSectorFundFlowTrendScoreContinuousOutflowCapsAtMinusForty(t *testing.T) {
	assessment := scoreAStockSectorFundFlowTrendAssessment(aStockHotspotSectorAlias{SectorType: "行业资金流", SectorName: "半导体"}, []model.AStockSectorFundFlow{
		{TradeDate: "2026-07-10", Rank: 90, Name: "半导体", MainNetInflow: -90000000, ChangePct: -1.2},
		{TradeDate: "2026-07-09", Rank: 80, Name: "半导体", MainNetInflow: -80000000, ChangePct: -0.8},
		{TradeDate: "2026-07-08", Rank: 70, Name: "半导体", MainNetInflow: -70000000, ChangePct: 0.2},
		{TradeDate: "2026-07-07", Rank: 60, Name: "半导体", MainNetInflow: -60000000, ChangePct: -0.3},
		{TradeDate: "2026-07-06", Rank: 50, Name: "半导体", MainNetInflow: -50000000, ChangePct: -0.4},
		{TradeDate: "2026-07-03", Rank: 40, Name: "半导体", MainNetInflow: -40000000, ChangePct: -0.2},
	})

	if assessment.ScoreDelta != -120 || assessment.Status != "连续流出" {
		t.Fatalf("expected continuous sector outflow to cap at -120, got %+v", assessment)
	}
	if reason := formatAStockSectorFundFlowTrendReason(assessment); !strings.Contains(reason, "板块资金趋势减分 120") || !strings.Contains(reason, "10日与5日均净流出") {
		t.Fatalf("expected sector outflow reason, got %q", reason)
	}
}

func TestAStockSectorFundFlowTrendScoreLoadsFromContent(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAStockAlgorithmSettingsTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/a-stock/sector-fund-flow-trend" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		sectorName := r.URL.Query().Get("sector_name")
		if r.URL.Query().Get("end_date") != "2026-07-10" || r.URL.Query().Get("indicator") != "今日" || r.URL.Query().Get("days") != "10" {
			t.Fatalf("unexpected sector trend query: %s", r.URL.RawQuery)
		}
		if sectorName != "人工智能" {
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowTrendResult{})
			return
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowTrendResult{
			Items: []model.AStockSectorFundFlow{
				{TradeDate: "2026-07-10", Rank: 6, Name: "人工智能", MainNetInflow: 120000000, ChangePct: 1.2},
				{TradeDate: "2026-07-09", Rank: 8, Name: "人工智能", MainNetInflow: 90000000, ChangePct: 0.8},
				{TradeDate: "2026-07-08", Rank: 12, Name: "人工智能", MainNetInflow: 80000000, ChangePct: 0.4},
			},
			Total: 3, EndDate: "2026-07-10", SectorType: "概念资金流", SectorName: "人工智能", Indicator: "今日", Days: 10,
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	rec := srv.applyAStockSectorFundFlowTrendScoreWithCache("2026-07-10", aStockRecommendation{Hotspot: "人工智能", Code: "300001", HotspotScore: 100, MarketScore: 100, Reason: "base"}, newAStockRequestCache())
	if rec.MarketScore <= 100 || !strings.Contains(rec.Reason, "人工智能 5日主力资金净流入") || !strings.Contains(rec.Reason, "板块资金趋势加分") {
		t.Fatalf("expected content-loaded sector trend score, got %+v", rec)
	}
}

func TestAStockFundFlowScoreRecentLargeOutflowSample(t *testing.T) {
	assessment := scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{
		Total5D:         -571000000,
		Total10D:        -445629605.79,
		NegativeDays5D:  3,
		LatestNetInflow: -708000000,
		LatestChangePct: -11.44,
	})

	if !assessment.HardFiltered || assessment.ScoreDelta != -130 {
		t.Fatalf("expected 300475-like sample to be hard filtered and heavily penalized, got %+v", assessment)
	}
}

func TestAStockSectorTopStockResonanceScoreLevelsAndCap(t *testing.T) {
	resolver := newAStockSectorTopStockCodeResolver([]aStockRecommendation{
		{Code: "600183", Name: "生益科技"},
		{Code: "600000", Name: "浦发银行"},
		{Code: "000001", Name: "平安银行"},
	})
	flows := []model.AStockSectorFundFlow{
		{SectorType: "行业资金流", Name: "电子器件", ChangePct: 1.2, MainNetInflow: 4531569158.08, TopStock: "生益科技", SourceTypes: "sina"},
		{SectorType: "行业资金流", Name: "元件", ChangePct: 0.8, MainNetInflow: 3862000000, TopStock: "生益科技", SourceTypes: "ths"},
		{SectorType: "概念资金流", Name: "印制电路板", ChangePct: 0.5, MainNetInflow: 3611000000, TopStock: "600183 生益科技", SourceTypes: "eastmoney"},
		{SectorType: "行业资金流", Name: "银行", ChangePct: 0.2, MainNetInflow: 1000000000, TopStock: "浦发银行"},
		{SectorType: "概念资金流", Name: "央企银行", ChangePct: 0.1, MainNetInflow: 1000000000, TopStock: "平安银行"},
		{SectorType: "概念资金流", Name: "金融科技", ChangePct: 0.1, MainNetInflow: 1000000000, TopStock: "平安银行"},
	}

	got := buildAStockSectorTopStockResonanceMap(flows, resolver)

	if got["600000"].ScoreDelta != 25 {
		t.Fatalf("expected one-sector resonance +25, got %+v", got["600000"])
	}
	if got["000001"].ScoreDelta != 45 {
		t.Fatalf("expected two-sector resonance +45, got %+v", got["000001"])
	}
	shengyi := got["600183"]
	if shengyi.ScoreDelta != 90 || shengyi.PositiveSectorCount != 3 || shengyi.TotalSectorMainNetInflow <= aStockSectorTopStockResonanceInflowThreshold {
		t.Fatalf("expected Shengyi 3-sector resonance capped at +90, got %+v", shengyi)
	}
	if strings.Join(shengyi.SectorNames, "、") != "电子器件、元件、印制电路板" {
		t.Fatalf("expected sector names to preserve source order, got %+v", shengyi.SectorNames)
	}
}

func TestAStockSectorTopStockResonanceSkipsNegativeOrDownSectors(t *testing.T) {
	resolver := newAStockSectorTopStockCodeResolver([]aStockRecommendation{{Code: "600183", Name: "生益科技"}})
	flows := []model.AStockSectorFundFlow{
		{SectorType: "行业资金流", Name: "电子器件", ChangePct: -0.1, MainNetInflow: 4531569158.08, TopStock: "生益科技"},
		{SectorType: "行业资金流", Name: "元件", ChangePct: 0.8, MainNetInflow: -3862000000, TopStock: "生益科技"},
		{SectorType: "概念资金流", Name: "未知主题", ChangePct: 1.1, MainNetInflow: 1000000000, TopStock: "不存在"},
	}

	got := buildAStockSectorTopStockResonanceMap(flows, resolver)

	if len(got) != 0 {
		t.Fatalf("expected negative/down/unresolved top stocks to be skipped, got %+v", got)
	}
}

func TestAStockSectorTopStockResonanceOverheat30CapsBonus(t *testing.T) {
	rec := aStockRecommendation{Code: "600183", Name: "生益科技", HotspotScore: 100, MarketScore: 100, Change30: "+44.00%", Reason: "base"}
	resonance := aStockSectorTopStockResonance{
		Code:                     "600183",
		Name:                     "生益科技",
		SectorNames:              []string{"电子器件", "元件", "印制电路板"},
		PositiveSectorCount:      3,
		TotalSectorMainNetInflow: 12000000000,
		ScoreDelta:               90,
	}

	got := applyAStockSectorTopStockResonanceToRecommendation(rec, resonance)

	if got.MarketScore != 120 || positiveAStockSectorTopStockResonanceScore(got) != 20 {
		t.Fatalf("expected 30d overheated resonance bonus capped at +20, got %+v", got)
	}
	if !strings.Contains(got.Reason, "板块资金共振") || !strings.Contains(got.Reason, "板块共振加分 20") {
		t.Fatalf("expected resonance reason with capped score, got %q", got.Reason)
	}
}

func TestAStockSectorTopStockResonanceDoesNotOverrideFundFlowHardFilter(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/stock-fund-flow-trend":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowTrendResult{
				Items: []model.AStockStockFundFlow{
					{TradeDate: "2026-07-14", Code: "600183", Name: "生益科技", MainNetInflow: -100000000},
					{TradeDate: "2026-07-13", Code: "600183", Name: "生益科技", MainNetInflow: -100000000},
					{TradeDate: "2026-07-10", Code: "600183", Name: "生益科技", MainNetInflow: -100000000},
					{TradeDate: "2026-07-09", Code: "600183", Name: "生益科技", MainNetInflow: -100000000},
					{TradeDate: "2026-07-08", Code: "600183", Name: "生益科技", MainNetInflow: -100000000},
				},
			})
		case "/api/v1/a-stock/sector-fund-flows":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{
				Items: []model.AStockSectorFundFlow{
					{TradeDate: "2026-07-14", SectorType: r.URL.Query().Get("sector_type"), Indicator: "今日", Name: "电子器件", ChangePct: 1.2, MainNetInflow: 4531569158.08, TopStock: "生益科技"},
					{TradeDate: "2026-07-14", SectorType: r.URL.Query().Get("sector_type"), Indicator: "今日", Name: "元件", ChangePct: 0.8, MainNetInflow: 3862000000, TopStock: "生益科技"},
					{TradeDate: "2026-07-14", SectorType: r.URL.Query().Get("sector_type"), Indicator: "今日", Name: "印制电路板", ChangePct: 0.5, MainNetInflow: 3611000000, TopStock: "生益科技"},
				},
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
	}))
	defer content.Close()
	srv := NewServer(config.Config{ContentURL: content.URL})
	result := srv.applyAStockRecommendationFundFlowFilterWithCache(
		"2026-07-14",
		[]aStockRecommendation{{Code: "600183", Name: "生益科技", Hotspot: "电子器件", HotspotScore: 100, MarketScore: 100}},
		nil,
		nil,
		1,
		newAStockRequestCache(),
		false,
	)

	if len(result.Recommendations) != 0 || result.Filtered != 1 {
		t.Fatalf("expected hard-filtered stock to stay removed despite sector resonance, got %+v", result)
	}
}

func TestAStockHotspotTopStocksUseRecommendationScoreAndLimit(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "人工智能",
		Keywords: []string{"AI"},
		Score:    80,
		Evidence: 1,
		MatchedItems: []model.Item{
			{Title: "AI六号午后活跃"},
		},
	}
	stocks := buildAStockHotspotTopStocks(hotspot, []aStockMarketCandidate{
		{Code: "000101", Name: "AI一号", Rank: 1, AuctionAmount: 6000000},
		{Code: "000102", Name: "AI二号", Rank: 2, AuctionAmount: 5000000},
		{Code: "000103", Name: "AI三号", Rank: 3, AuctionAmount: 4000000},
		{Code: "000104", Name: "AI四号", Rank: 4, AuctionAmount: 3000000},
		{Code: "000105", Name: "AI五号", Rank: 5, AuctionAmount: 2000000},
		{Code: "000106", Name: "AI六号", Rank: 6, AuctionAmount: 1000000},
	}, 5)

	if len(stocks) != 5 {
		t.Fatalf("expected top 5 hotspot stocks, got %+v", stocks)
	}
	gotCodes := make([]string, 0, len(stocks))
	for _, stock := range stocks {
		gotCodes = append(gotCodes, stock.Code)
	}
	if strings.Join(gotCodes, ",") != "000106,000101,000102,000103,000104" {
		t.Fatalf("expected recommendation-score order with evidence boost and limit, got %+v", stocks)
	}
	for i, stock := range stocks {
		if stock.Rank != i+1 || stock.Name == "" || stock.Score <= 0 {
			t.Fatalf("expected ranked stock display data, got %+v", stocks)
		}
	}
}

func TestAStockHotspotTopStocksDefaultLimitIsNine(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "人工智能",
		Keywords: []string{"AI"},
		Score:    80,
		Evidence: 1,
	}
	candidates := make([]aStockMarketCandidate, 0, 10)
	for i := 1; i <= 10; i++ {
		candidates = append(candidates, aStockMarketCandidate{
			Code:          fmt.Sprintf("000%03d", i+100),
			Name:          fmt.Sprintf("AI%d号", i),
			Rank:          i,
			AuctionAmount: float64(1000000 - i),
		})
	}

	stocks := buildAStockHotspotTopStocks(hotspot, candidates, 0)

	if len(stocks) != 9 {
		t.Fatalf("expected default top 9 hotspot stocks, got %+v", stocks)
	}
	if stocks[0].Code != "000101" || stocks[8].Code != "000109" {
		t.Fatalf("expected first nine ranked candidates, got %+v", stocks)
	}
	for i, stock := range stocks {
		if stock.Rank != i+1 {
			t.Fatalf("expected hotspot display ranks to be sequential, got %+v", stocks)
		}
	}
}

func TestRenderAStockHotspotTopStocksOmitsDisplayRanks(t *testing.T) {
	var b strings.Builder

	renderAStockHotspotTopStocks(&b, []aStockHotspotStock{
		{Rank: 1, Code: "300059", Name: "东方财富"},
		{Rank: 2, Code: "600030", Name: "中信证券"},
	})

	body := b.String()
	for _, want := range []string{"300059 东方财富", "600030 中信证券"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected rendered hotspot stock %q, got %s", want, body)
		}
	}
	for _, notWant := range []string{"1. 300059", "2. 600030"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("expected rendered hotspot stocks to omit display ranks, got %s", body)
		}
	}
}

func TestRenderAStockHotspotTopStocksAppendsRecommendationDate(t *testing.T) {
	var b strings.Builder

	renderAStockHotspotTopStocks(&b, []aStockHotspotStock{
		{Rank: 1, Code: "300024", Name: "机器人", RecommendationDate: "2026-06-28"},
		{Rank: 2, Code: "603019", Name: "中科曙光"},
	})

	body := b.String()
	if !strings.Contains(body, `300024 机器人<span class="astock-hotspot-date">（2026-06-28）</span>`) {
		t.Fatalf("expected rendered recommendation date, got %s", body)
	}
	if strings.Contains(body, "603019 中科曙光（") || strings.Contains(body, "1. 300024") {
		t.Fatalf("expected no empty date or display rank, got %s", body)
	}
}

func TestAStockHotspotRecommendationDatesAreAppliedFromContent(t *testing.T) {
	var requestedCodes string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/a-stock/recommendation-latest-dates" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("date") != "2026-06-30" || r.URL.Query().Get("period") != "afternoon" {
			t.Fatalf("unexpected latest-date query: %s", r.URL.RawQuery)
		}
		requestedCodes = r.URL.Query().Get("codes")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.AStockRecommendationLatestDateListResult{
				StrategyDate: "2026-06-30",
				Period:       "afternoon",
				Items: []model.AStockRecommendationLatestDate{
					{Code: "300024", LatestDate: "2026-06-28"},
				},
			},
		})
	}))
	defer content.Close()
	srv := NewServer(config.Config{ContentURL: content.URL})
	hotspots := []aStockHotspot{{
		Name: "人工智能",
		TopStocks: []aStockHotspotStock{
			{Code: "300024", Name: "机器人"},
			{Code: "603019", Name: "中科曙光"},
		},
	}}

	result := srv.applyAStockHotspotRecommendationDatesWithCache(hotspots, "2026-06-30", "afternoon", newAStockRequestCache())

	if requestedCodes != "300024,603019" {
		t.Fatalf("expected batched hotspot codes, got %q", requestedCodes)
	}
	if result[0].TopStocks[0].RecommendationDate != "2026-06-28" {
		t.Fatalf("expected recommendation date to be applied, got %+v", result)
	}
	if result[0].TopStocks[1].RecommendationDate != "" || hotspots[0].TopStocks[0].RecommendationDate != "" {
		t.Fatalf("expected unmatched and source stocks to stay without dates, got result=%+v source=%+v", result, hotspots)
	}
}

func TestAStockHotspotsWithTopStocksDedupesAcrossHotspots(t *testing.T) {
	hotspots := []aStockHotspot{
		{
			Name:     "人工智能",
			Keywords: []string{"AI"},
			Score:    120,
			Evidence: 1,
			MatchedItems: []model.Item{
				{Title: "AI 主题带动科大讯飞活跃", TagFlags: "0.002230"},
			},
		},
		{
			Name:     "金融券商",
			Keywords: []string{"证券"},
			Score:    100,
			Evidence: 1,
			MatchedItems: []model.Item{
				{Title: "证券板块科大讯飞与中信证券放量", TagFlags: "0.002230 1.600030"},
			},
		},
	}
	marketCandidates := []aStockMarketCandidate{
		{Code: "002230", Name: "科大讯飞", Rank: 1, AuctionAmount: 9000000},
		{Code: "600030", Name: "中信证券", Rank: 2, AuctionAmount: 8000000},
		{Code: "600036", Name: "招商银行", Rank: 3, AuctionAmount: 7000000},
		{Code: "603019", Name: "中科曙光", Rank: 101, AuctionAmount: 900000},
		{Code: "601138", Name: "工业富联", Rank: 102, AuctionAmount: 800000},
	}

	result := buildAStockHotspotsWithTopStocks(hotspots, marketCandidates, 1)

	if len(result) != 2 || len(result[0].TopStocks) != 1 || len(result[1].TopStocks) != 1 {
		t.Fatalf("expected one top stock per hotspot, got %+v", result)
	}
	if result[0].TopStocks[0].Code != "002230" {
		t.Fatalf("expected first hotspot to claim duplicate stock, got %+v", result[0].TopStocks)
	}
	if result[1].TopStocks[0].Code != "600030" || result[1].TopStocks[0].Rank != 1 {
		t.Fatalf("expected second hotspot to skip duplicate and rerank replacement, got %+v", result[1].TopStocks)
	}
}

func TestAStockHotspotsWithTopStocksFillsNinePerHotspot(t *testing.T) {
	hotspots := []aStockHotspot{
		{
			Name:     "人工智能",
			Keywords: []string{"AI"},
			Score:    120,
			Evidence: 1,
			MatchedItems: []model.Item{
				{Title: "AI 主题活跃", TagFlags: "0.300059"},
			},
		},
		{
			Name:     "金融券商",
			Keywords: []string{"证券"},
			Score:    100,
			Evidence: 1,
			MatchedItems: []model.Item{
				{Title: "证券板块活跃", TagFlags: "0.300059 1.600030"},
			},
		},
	}
	marketCandidates := make([]aStockMarketCandidate, 0, 24)
	for i, code := range []string{
		"300059", "600030", "688981", "600036", "000725", "002230", "603019", "601138", "002371", "603986", "601899", "600111",
		"600547", "603259", "300760", "600276", "300750", "300274", "601012", "300433", "002475", "600893", "600760", "002179",
	} {
		marketCandidates = append(marketCandidates, aStockMarketCandidate{
			Code:          code,
			Name:          fmt.Sprintf("候选%s", code),
			Rank:          i + 1,
			AuctionAmount: float64(1000000 - i),
		})
	}

	result := buildAStockHotspotsWithTopStocks(hotspots, marketCandidates, aStockHotspotTopStockLimit)

	if len(result) != 2 {
		t.Fatalf("expected two hotspots, got %+v", result)
	}
	seen := make(map[string]struct{})
	for _, hotspot := range result {
		if len(hotspot.TopStocks) != aStockHotspotTopStockLimit {
			t.Fatalf("expected hotspot %s to fill %d stocks, got %+v", hotspot.Name, aStockHotspotTopStockLimit, hotspot.TopStocks)
		}
		for _, stock := range hotspot.TopStocks {
			if _, exists := seen[stock.Code]; exists {
				t.Fatalf("expected cross-hotspot display stocks to stay unique, duplicate %s in %+v", stock.Code, result)
			}
			seen[stock.Code] = struct{}{}
		}
	}
}

func TestAStockT1ShadowFiltersWeakEvidenceAndLimitsByHotspot(t *testing.T) {
	weak := aStockMarketCandidate{Code: "600001", Name: "弱证据", Evidence: 1, WeakEvidence: 1, Fallback: true}
	if isAStockT1ShadowEligibleCandidate(weak, aStockT1ShadowStrongEvidence(weak)) {
		t.Fatal("expected weak-only evidence candidate to be filtered")
	}
	strong := aStockMarketCandidate{Code: "600002", Name: "强证据", Evidence: 1, Fallback: true}
	if !isAStockT1ShadowEligibleCandidate(strong, aStockT1ShadowStrongEvidence(strong)) {
		t.Fatal("expected strong news evidence candidate to pass")
	}
	marketWithoutKeyword := aStockMarketCandidate{Code: "600003", Name: "市场无关键词", Evidence: 1, Fallback: false}
	if isAStockT1ShadowEligibleCandidate(marketWithoutKeyword, aStockT1ShadowStrongEvidence(marketWithoutKeyword)) {
		t.Fatal("expected market candidate without stock-name keyword to be filtered")
	}
	recommendations := []aStockRecommendation{
		{Hotspot: "人工智能", Code: "600001", MarketScore: 300},
		{Hotspot: "人工智能", Code: "600002", MarketScore: 299},
		{Hotspot: "人工智能", Code: "600003", MarketScore: 298},
		{Hotspot: "机器人", Code: "600004", MarketScore: 297},
		{Hotspot: "机器人", Code: "600005", MarketScore: 296},
		{Hotspot: "机器人", Code: "600006", MarketScore: 295},
		{Hotspot: "低空经济", Code: "600007", MarketScore: 294},
		{Hotspot: "低空经济", Code: "600008", MarketScore: 293},
		{Hotspot: "低空经济", Code: "600009", MarketScore: 292},
	}
	limited := limitAStockRecommendationsByHotspot(recommendations, aStockT1ShadowRecommendationLimit, aStockT1ShadowStocksPerHotspot)
	if len(limited) != aStockT1ShadowRecommendationLimit {
		t.Fatalf("expected shadow recommendations to cap at %d, got %+v", aStockT1ShadowRecommendationLimit, limited)
	}
	counts := aStockRecommendationHotspotCounts(limited)
	for hotspot, count := range counts {
		if count > aStockT1ShadowStocksPerHotspot {
			t.Fatalf("expected hotspot %s to cap at %d, got %d in %+v", hotspot, aStockT1ShadowStocksPerHotspot, count, limited)
		}
	}
	afternoon, skipped := limitAStockRecommendationsByCount(limited, remainingAStockDailyRecommendationLimit(3))
	if skipped != aStockT1ShadowRecommendationLimit-2 || len(afternoon) != 2 {
		t.Fatalf("expected shadow afternoon to leave two slots after three morning recommendations, skipped=%d got %+v", skipped, afternoon)
	}
	afternoon, skipped = limitAStockRecommendationsByCount(limited, remainingAStockDailyRecommendationLimit(4))
	if skipped != aStockT1ShadowRecommendationLimit-1 || len(afternoon) != 1 {
		t.Fatalf("expected shadow afternoon to leave one slot after four morning recommendations, skipped=%d got %+v", skipped, afternoon)
	}
	afternoon, skipped = limitAStockRecommendationsByCount(limited, remainingAStockDailyRecommendationLimit(5))
	if skipped != aStockT1ShadowRecommendationLimit || len(afternoon) != 0 {
		t.Fatalf("expected shadow afternoon to stop after five morning recommendations, skipped=%d got %+v", skipped, afternoon)
	}
}

func TestAStockAuctionStrengthShadowScoresIncreasingAuctionAndFiltersWeakCandidates(t *testing.T) {
	settings := aStockAlgorithmSettingsForRecommendationPeriod("morning", defaultAStockAlgorithmSettings())
	hotspot := aStockHotspot{Name: "人工智能", Keywords: []string{"人工智能"}, Score: 120, Evidence: 2}
	candidates := []aStockMarketCandidate{
		{
			Code:              "300001",
			Name:              "人工智能强承接",
			Rank:              1,
			AuctionAmount:     100000000,
			AuctionVolume:     1000000,
			AuctionAmount0920: 20000000,
			AuctionAmount0925: 60000000,
			AuctionAmount0929: 100000000,
			Sources:           []string{aStockCandidateSourceAuction, aStockCandidateSourceNews},
			PreFundScore:      40,
			PreFundDetail:     "个股资金持续流入",
		},
		{
			Code:              "300002",
			Name:              "人工智能末段回落",
			Rank:              2,
			AuctionAmount:     20000000,
			AuctionAmount0920: 10000000,
			AuctionAmount0925: 100000000,
			AuctionAmount0929: 20000000,
			Sources:           []string{aStockCandidateSourceAuction, aStockCandidateSourceNews},
		},
		{
			Code:              "300003",
			Name:              "纯竞价大额",
			Rank:              3,
			AuctionAmount:     200000000,
			AuctionAmount0929: 200000000,
		},
	}

	recommendations := buildAStockAuctionStrengthRecommendationsWithSectorGateWithSettings([]aStockHotspot{hotspot}, candidates, nil, settings)
	if len(recommendations) != 1 || recommendations[0].Code != "300001" {
		t.Fatalf("expected only the increasing auction candidate, got %+v", recommendations)
	}
	if !strings.Contains(recommendations[0].Reason, "三段递增") || !strings.Contains(recommendations[0].Reason, "集合竞价强承接影子策略") {
		t.Fatalf("expected reason to explain auction strength, got %q", recommendations[0].Reason)
	}
	strengthComponent := mustAStockScoreComponentForTest(t, recommendations[0], "竞价强度")
	if strengthComponent.Score <= 0 {
		t.Fatalf("expected positive auction strength component, got %+v", strengthComponent)
	}
	fundComponent := mustAStockScoreComponentForTest(t, recommendations[0], "个股资金预选")
	if fundComponent.Score != 40 {
		t.Fatalf("expected fund pre-score to be preserved, got %+v", fundComponent)
	}
}

func TestAStockRecommendationsUseNewsMentionedStocksWhenAuctionCandidatesEmpty(t *testing.T) {
	recommendations := buildAStockRecommendations([]aStockHotspot{
		{
			Name:     "黄金有色",
			Keywords: []string{"有色", "黄金"},
			Score:    105,
			Evidence: 2,
			MatchedItems: []model.Item{
				{SourceType: "eastmoney_kuaixun", Title: "中铝国际6月18日快速拉升", Summary: "有色板块活跃", TagFlags: "1.601068"},
				{SourceType: "cls_telegraph", Title: "钢铁板块异动拉升 抚顺特钢涨停", Summary: "财联社6月18日电，钢铁板块盘中异动拉升。", RawPayload: `{"stock_list":[{"StockID":"sh600399","name":"抚顺特钢"}]}`},
			},
		},
		{
			Name:         "人工智能",
			Keywords:     []string{"AI", "人工智能"},
			Score:        82,
			Evidence:     1,
			MatchedItems: []model.Item{{SourceType: "sina_finance_7x24", Title: "人工智能产业链继续活跃"}},
		},
	}, nil)

	if len(recommendations) != 2 {
		t.Fatalf("expected news-mentioned recommendations when auction candidates are empty, got %+v", recommendations)
	}
	got := map[string]string{}
	for _, rec := range recommendations {
		got[rec.Code] = rec.Name
	}
	for code, name := range map[string]string{
		"601068": "中铝国际",
		"600399": "抚顺特钢",
	} {
		if got[code] != name {
			t.Fatalf("expected news-mentioned recommendation %s:%s, got %+v", code, name, recommendations)
		}
	}
	if !strings.Contains(recommendations[0].Reason, "实时新闻明确提及股票") {
		t.Fatalf("expected reason to explain news-mentioned stock, got %q", recommendations[0].Reason)
	}
}

func TestAStockRecommendationCandidatesExcludePureAuctionLargeCap(t *testing.T) {
	hotspots := []aStockHotspot{{
		Name:     "人工智能",
		Keywords: []string{"AI", "机器人"},
		Score:    80,
		Evidence: 1,
	}}
	candidates := aStockRecommendationCandidatesForLimitWithSettings(hotspots, []aStockMarketCandidate{
		{Code: "601988", Name: "中国银行", Rank: 1, AuctionAmount: 900000000},
		{Code: "300024", Name: "机器人", Rank: 998, AuctionAmount: 30000000},
	}, 5, 3, defaultAStockAlgorithmSettings())
	got := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		got[normalizeAStockCode(candidate.Code)] = struct{}{}
	}
	if _, ok := got["601988"]; ok {
		t.Fatalf("expected pure auction large cap 601988 to be excluded, got %+v", candidates)
	}
	if _, ok := got["300024"]; !ok {
		t.Fatalf("expected hotspot-linked auction candidate 300024 to remain, got %+v", candidates)
	}
}

func TestAStockRecommendationsIncludeNewsMentionedLowAuctionRank(t *testing.T) {
	recommendations := buildAStockRecommendations([]aStockHotspot{{
		Name:     "人工智能",
		Keywords: []string{"AI", "算力"},
		Score:    90,
		Evidence: 1,
		MatchedItems: []model.Item{{
			SourceType: "flash",
			Title:      "算力产业链走强，协创数据获资金关注",
			Summary:    "AI应用需求升温",
			TagFlags:   "0.300857",
		}},
	}}, []aStockMarketCandidate{
		{Code: "601988", Name: "中国银行", Rank: 1, AuctionAmount: 900000000},
		{Code: "300857", Name: "协创数据", Rank: 1500, AuctionAmount: 1000000},
	})
	got := aStockTestRecommendationsByCode(recommendations)
	if _, ok := got["300857"]; !ok {
		t.Fatalf("expected news-mentioned low-auction-rank stock 300857 to enter recommendations, got %+v", recommendations)
	}
	if _, ok := got["601988"]; ok {
		t.Fatalf("expected unrelated pure auction stock 601988 to be excluded, got %+v", recommendations)
	}
}

func TestAStockPriorityCandidatePoolBlocksAuctionWhenSectorTrendOutflows(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/system/a-stock-recommendation-algorithm":
			writeEnvelope(w, http.StatusOK, "ok", model.DefaultAStockRecommendationAlgorithmSettings())
		case "/api/v1/a-stock/sector-fund-flow-trend":
			items := make([]model.AStockSectorFundFlow, 0, 10)
			for i := 0; i < 10; i++ {
				items = append(items, model.AStockSectorFundFlow{
					TradeDate:     fmt.Sprintf("2026-07-%02d", 17-i),
					SectorType:    r.URL.Query().Get("sector_type"),
					Name:          r.URL.Query().Get("sector_name"),
					Indicator:     "今日",
					Rank:          80 + i,
					MainNetInflow: -100000000,
				})
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowTrendResult{Items: items})
		case "/api/v1/a-stock/stock-fund-flows":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockStockFundFlowListResult{})
		default:
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, AStockAuctionURL: "enabled"})
	sectorGate := newAStockHotspotSectorGate()
	sectorGate.addCodes("人工智能", map[string]struct{}{"300024": {}})
	pool := srv.buildAStockPriorityCandidatePoolWithCache("2026-07-17", []aStockHotspot{{
		Name:     "人工智能",
		Keywords: []string{"AI", "机器人"},
		Score:    80,
		Evidence: 1,
	}}, []aStockMarketCandidate{
		{Code: "300024", Name: "机器人", Rank: 1, AuctionAmount: 90000000},
	}, sectorGate, newAStockRequestCache())
	for _, candidate := range pool {
		if normalizeAStockCode(candidate.Code) == "300024" {
			t.Fatalf("expected sector 5D/10D outflow to block auction-only sector candidate, got %+v", pool)
		}
	}
}

func TestAStockRecommendationReasonIncludesCandidateSources(t *testing.T) {
	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{{
		Name:     "人工智能",
		Keywords: []string{"机器人"},
		Score:    60,
		Evidence: 1,
	}}, []aStockMarketCandidate{{
		Code:            "300024",
		Name:            "机器人",
		Rank:            1,
		AuctionAmount:   90000000,
		Sources:         []string{aStockCandidateSourceSector, aStockCandidateSourceAuction},
		PreFundScore:    30,
		PreFundDetail:   "机器人 5日主力资金净流入 +2.00亿",
		PreSectorScore:  20,
		PreSectorDetail: "人工智能 5日主力资金净流入 +10.00亿",
	}}, 5, 3)
	if len(recommendations) == 0 {
		t.Fatal("expected source-linked recommendation")
	}
	if !strings.Contains(recommendations[0].Reason, "候选来源 热点板块成分股、集合竞价确认") {
		t.Fatalf("expected reason to include candidate sources, got %q", recommendations[0].Reason)
	}
	for _, component := range recommendations[0].ScoreBreakdown {
		switch component.Label {
		case "候选板块资金", "候选个股资金":
			t.Fatalf("expected candidate pre-scores to stay out of final score breakdown, got %+v", recommendations[0].ScoreBreakdown)
		}
	}
}

func TestAStockRecommendationsKeepNewsDerivedCandidatesThroughSectorGate(t *testing.T) {
	hotspot := aStockHotspot{
		Name:     "黄金有色",
		Keywords: []string{"有色", "稀土", "贵金属", "铜", "黄金"},
		Score:    448,
		Evidence: 43,
		MatchedItems: []model.Item{{
			SourceType: "jin10_kuaixun",
			Title:      "A股盘前市场要闻速递：钨矿、铜、国际金价和黄金板块受关注",
			Summary:    "海南海药公告进展，铜陵有色获市场关注。",
			Content:    "钨矿、铜、黄金、贵金属等有色方向活跃，海南海药和铜陵有色同时出现在综合稿。",
			RawPayload: `{"stock_list":[{"StockID":"000566.SZ","name":"海南海药"},{"StockID":"000630.SZ","name":"铜陵有色"}]}`,
		}},
	}
	candidates := []aStockMarketCandidate{
		{Code: "000566", Name: "海南海药", Rank: 1, AuctionAmount: 9000000},
		{Code: "000630", Name: "铜陵有色", Rank: 2, AuctionAmount: 8000000},
	}

	withoutGate := buildAStockRecommendationsWithLimitAndSectorGate([]aStockHotspot{hotspot}, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot, nil)
	if _, ok := aStockTestRecommendationsByCode(withoutGate)["000566"]; !ok {
		t.Fatalf("expected ungated news-derived recommendation to reproduce 000566, got %+v", withoutGate)
	}

	sectorGate := newAStockHotspotSectorGate()
	sectorGate.addCodes("黄金有色", map[string]struct{}{
		"600547": {},
		"601899": {},
		"600111": {},
		"000630": {},
	})
	withGate := buildAStockRecommendationsWithLimitAndSectorGate([]aStockHotspot{hotspot}, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot, sectorGate)
	got := aStockTestRecommendationsByCode(withGate)
	if rec, ok := got["000566"]; !ok || rec.Name != "海南海药" {
		t.Fatalf("expected news-derived 000566 海南海药 to bypass sector gate, got %+v", withGate)
	}
	if rec, ok := got["000630"]; !ok || rec.Name != "铜陵有色" {
		t.Fatalf("expected matched nonferrous candidate 000630 铜陵有色 to remain, got %+v", withGate)
	}

	emptySectorGate := newAStockHotspotSectorGate()
	emptySectorGate.addHotspot("黄金有色")
	withEmptyGate := buildAStockRecommendationsWithLimitAndSectorGate([]aStockHotspot{hotspot}, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot, emptySectorGate)
	gotEmpty := aStockTestRecommendationsByCode(withEmptyGate)
	if rec, ok := gotEmpty["000566"]; !ok || rec.Name != "海南海药" {
		t.Fatalf("expected empty sector data to keep news-derived 000566, got %+v", withEmptyGate)
	}
	if rec, ok := gotEmpty["000630"]; !ok || rec.Name != "铜陵有色" {
		t.Fatalf("expected empty sector data to keep name-matched nonferrous candidate 000630, got %+v", withEmptyGate)
	}
}

func aStockTestRecommendationsByCode(recommendations []aStockRecommendation) map[string]aStockRecommendation {
	result := make(map[string]aStockRecommendation, len(recommendations))
	for _, rec := range recommendations {
		result[normalizeAStockCode(rec.Code)] = rec
	}
	return result
}

func TestAStockRecommendationsAllowBankStocksButBlockEastmoney(t *testing.T) {
	recommendations := buildAStockRecommendations([]aStockHotspot{
		{
			Name:     "金融券商",
			Keywords: []string{"证券", "银行", "资本市场"},
			Score:    245,
			Evidence: 2,
			MatchedItems: []model.Item{
				{SourceType: "flash", Title: "证券与银行板块活跃", Summary: "资本市场情绪修复"},
			},
		},
	}, []aStockMarketCandidate{
		{Code: "300059", Name: "东方财富", Rank: 1, AuctionAmount: 9000000},
		{Code: "600036", Name: "招商银行", Rank: 2, AuctionAmount: 8000000},
		{Code: "600030", Name: "中信证券", Rank: 3, AuctionAmount: 7000000},
		{Code: "000001", Name: "平安银行", Rank: 4, AuctionAmount: 6000000},
	})

	if len(recommendations) != 3 {
		t.Fatalf("expected bank stocks to be allowed and 东方财富 to be blocked, got %+v", recommendations)
	}
	got := aStockTestRecommendationsByCode(recommendations)
	for _, wantCode := range []string{"600036", "600030", "000001"} {
		if _, ok := got[wantCode]; !ok {
			t.Fatalf("expected allowed finance recommendation %s, got %+v", wantCode, recommendations)
		}
	}
	for _, rec := range recommendations {
		if rec.Code == "300059" {
			t.Fatalf("expected 东方财富 to remain blocked, got %+v", recommendations)
		}
		if rec.Rank < 1 || rec.Rank > len(recommendations) {
			t.Fatalf("expected recommendations to be reranked, got %+v", recommendations)
		}
	}

	newsDerived := buildAStockRecommendationsWithLimit([]aStockHotspot{
		{
			Name:     "金融券商",
			Keywords: []string{"证券", "银行", "资本市场"},
			Score:    245,
			Evidence: 2,
			MatchedItems: []model.Item{
				{SourceType: "flash", Title: "平安银行资本市场业务获关注", Summary: "银行与证券板块活跃", RawPayload: `{"stock_list":[{"StockID":"000001","name":"平安银行"}]}`},
			},
		},
	}, []aStockMarketCandidate{
		{Code: "000001", Name: "平安银行", Rank: 4, AuctionAmount: 6000000},
	}, aStockReplacementPoolLimit, aStockReplacementPerHotspot)
	newsGot := aStockTestRecommendationsByCode(newsDerived)
	if _, ok := newsGot["000001"]; !ok {
		t.Fatalf("expected news-derived bank stock 平安银行 to be allowed, got %+v", newsDerived)
	}
}

func TestAStockRecommendationsFilterInvalidCodesAndUnresolvedNames(t *testing.T) {
	recommendations := buildAStockRecommendationsWithLimit([]aStockHotspot{
		{
			Name:     "AI测试",
			Keywords: []string{"AI"},
			Score:    100,
			Evidence: 1,
			MatchedItems: []model.Item{{
				SourceType: "flash",
				Title:      "AI芯片公司获关注",
				RawPayload: `{"stock_list":[{"StockID":"301696","name":"301696"},{"StockID":"301697","name":"301697"},{"StockID":"012322","name":"012322"}]}`,
			}},
		},
	}, []aStockMarketCandidate{
		{Code: "012322", Name: "012322", Rank: 1, AuctionAmount: 9000000},
		{Code: "011631", Name: "基金测试", Rank: 2, AuctionAmount: 8000000},
		{Code: "301696", Name: "301696", Rank: 3, AuctionAmount: 7000000},
		{Code: "301697", Name: "301697", Rank: 4, AuctionAmount: 6000000},
		{Code: "301696", Name: "AI芯片股份", Rank: 5, AuctionAmount: 5000000},
	}, aStockReplacementPoolLimit, aStockReplacementPerHotspot)

	if len(recommendations) != 1 {
		t.Fatalf("expected only one valid resolved recommendation, got %+v", recommendations)
	}
	if recommendations[0].Code != "301696" || recommendations[0].Name != "AI芯片股份" {
		t.Fatalf("expected 301696 name to be repaired from market dictionary, got %+v", recommendations[0])
	}
	for _, rec := range recommendations {
		if rec.Code == "012322" || rec.Code == "011631" || rec.Code == "301697" || rec.Name == rec.Code {
			t.Fatalf("expected invalid and unresolved recommendations to be filtered, got %+v", recommendations)
		}
	}
}

func TestAStockPersistedRecommendationsAllowBankStocksButBlockEastmoney(t *testing.T) {
	recommendations := aStockRecommendationSelectionsToRecommendations([]model.AStockRecommendationSelection{
		{Rank: 1, Code: "300059", Name: "东方财富", Hotspot: "金融券商"},
		{Rank: 2, Code: "600036", Name: "招商银行", Hotspot: "金融券商"},
		{Rank: 3, Code: "600030", Name: "中信证券", Hotspot: "金融券商"},
		{Rank: 4, Code: "000001", Name: "平安银行", Hotspot: "金融券商"},
	}, "morning")

	if len(recommendations) != 4 || recommendations[2].Code != "600030" || recommendations[2].Rank != 3 {
		t.Fatalf("expected persisted selections to convert before filtering, got %+v", recommendations)
	}
	filtered, skipped := filterBlockedAStockRecommendations(recommendations)
	if skipped != 1 || len(filtered) != 3 || filtered[0].Code != "600036" || filtered[0].Rank != 1 || filtered[2].Code != "000001" || filtered[2].Rank != 3 {
		t.Fatalf("expected filtered persisted recommendation to rerank, skipped=%d filtered=%+v", skipped, filtered)
	}
	backtests := filterBlockedAStockBacktests([]aStockBacktestRow{
		{Stock: "300059 东方财富"},
		{Stock: "600036 招商银行"},
		{Stock: "600030 中信证券"},
	})
	if len(backtests) != 2 || !strings.Contains(backtests[0].Stock, "600036") || !strings.Contains(backtests[1].Stock, "600030") {
		t.Fatalf("expected persisted backtests to drop only blocked stocks, got %+v", backtests)
	}
}

func TestAStockPersistedRecommendationsRepairNamesAndFilterBacktests(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/a-stock/code-names" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{
			Total: 2,
			Items: []model.AStockCodeName{
				{Code: "301696", Name: "测试股份", Source: "akshare_code_name"},
				{Code: "002179", Name: "中航光电", Source: "akshare_code_name"},
				{Code: "603698", Name: "航天工程", Source: "akshare_code_name"},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	recommendations, skipped := srv.repairAStockPersistedRecommendationsWithCache("2026-07-02", []aStockRecommendation{
		{Rank: 1, Code: "012322", Name: "012322", Hotspot: "人工智能"},
		{Rank: 2, Code: "011631", Name: "基金测试", Hotspot: "黄金有色"},
		{Rank: 3, Code: "301696", Name: "301696", Hotspot: "人工智能"},
		{Rank: 4, Code: "002179", Name: "金十数据整理", Hotspot: "人工智能"},
		{Rank: 5, Code: "603698", Name: "????", Hotspot: "军工航天"},
		{Rank: 6, Code: "600030", Name: "中信证券", Hotspot: "金融券商"},
	}, newAStockRequestCache())

	if skipped != 2 || len(recommendations) != 4 {
		t.Fatalf("expected two invalid persisted recommendations to be skipped, skipped=%d recommendations=%+v", skipped, recommendations)
	}
	if recommendations[0].Code != "301696" || recommendations[0].Name != "测试股份" {
		t.Fatalf("expected 301696 name to be repaired from auction dictionary, got %+v", recommendations)
	}
	if recommendations[1].Code != "002179" || recommendations[1].Name != "中航光电" {
		t.Fatalf("expected placeholder name to be repaired from auction dictionary, got %+v", recommendations)
	}
	if recommendations[2].Code != "603698" || recommendations[2].Name != "航天工程" {
		t.Fatalf("expected garbled 603698 name to be repaired from auction dictionary, got %+v", recommendations)
	}
	if recommendations[3].Code != "600030" || recommendations[3].Name != "中信证券" {
		t.Fatalf("expected valid recommendation to remain, got %+v", recommendations)
	}

	backtests := filterAStockBacktestsForRecommendations([]aStockBacktestRow{
		{Stock: "012322 012322"},
		{Stock: "301696 301696", Status: "已回测"},
		{Stock: "002179 金十数据整理", Status: "已回测"},
		{Stock: "603698 ????", Status: "已回测"},
		{Stock: "600030 中信证券", Status: "已回测"},
	}, recommendations)
	if len(backtests) != 4 || backtests[0].Stock != "301696 测试股份" || backtests[1].Stock != "002179 中航光电" || backtests[2].Stock != "603698 航天工程" || backtests[3].Stock != "600030 中信证券" {
		t.Fatalf("expected backtests to follow repaired recommendations, got %+v", backtests)
	}
}

func TestAStockPersistedRecommendationsRepairGarbledHotspotAndReason(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/code-names":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{
				Total: 1,
				Items: []model.AStockCodeName{{Code: "600118", Name: "中国卫星", Source: "auction"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          1,
					SourceType:  "flash",
					Title:       "军工航天卫星产业链走强",
					Summary:     "中国卫星获资金关注",
					PublishTime: "2026-07-13 09:00:00",
					TagFlags:    "600118",
					RawPayload:  `{"stocks":[{"code":"600118","name":"中国卫星"}]}`,
					CapturedAt:  time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC),
				}},
				Page:     1,
				PageSize: 20,
				Total:    1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date: "2026-07-13",
				Items: []model.AStockAuctionAmount{{
					TradeDate:     "2026-07-13",
					Code:          "600118",
					Name:          "中国卫星",
					AuctionAmount: 100000000,
					AuctionVolume: 1000000,
				}},
				Total:       1,
				TotalAmount: 100000000,
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	recommendations, skipped := srv.repairAStockPersistedRecommendationsForPeriodWithCache("2026-07-13", "morning", []aStockRecommendation{{
		Rank:         1,
		Hotspot:      "????",
		Code:         "600118",
		Name:         "中国卫星",
		HotspotScore: 82,
		MarketScore:  217,
		Reason:       "?? ?????????????,???? 7 ?,??? 82;????????????,???? 2 ?,??? 100,??? 182,??? 09:27，5日主力资金净流入 +8.03亿，资金加分 25",
		ScoreBreakdown: []aStockRecommendationScoreComponent{{
			Label:     "资金动向",
			Detail:    "5日主力资金净流入 +8.03亿，资金加分 25",
			UnitValue: 25,
			Score:     25,
		}},
	}}, newAStockRequestCache())

	if skipped != 0 || len(recommendations) != 1 {
		t.Fatalf("expected one repaired recommendation, skipped=%d recommendations=%+v", skipped, recommendations)
	}
	got := recommendations[0]
	if got.Hotspot != "军工航天" || strings.Contains(got.Reason, "??") || strings.Contains(got.Hotspot, "??") {
		t.Fatalf("expected garbled hotspot/reason repaired, got %+v", got)
	}
	if !strings.Contains(got.Reason, "命中") || !strings.Contains(got.Reason, "5日主力资金净流入") {
		t.Fatalf("expected rebuilt base reason plus existing clean adjustment, got %q", got.Reason)
	}
	components := aStockRecommendationScoreBreakdown(got)
	if !aStockScoreBreakdownContainsComponent(components, aStockRecommendationScoreComponent{Label: "资金动向", Detail: "5日主力资金净流入 +8.03亿，资金加分 25", Score: 25}) {
		t.Fatalf("expected existing fund-flow component to be preserved, got %+v", components)
	}
	if !aStockScoreBreakdownContainsComponent(components, aStockRecommendationScoreComponent{Label: "新闻热度", Detail: "证据新闻 1 条", Score: 5}) {
		t.Fatalf("expected rebuilt base score components, got %+v", components)
	}
}

func TestAStockReadOnlySnapshotRepairsGarbledRecommendationText(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                 true,
				StrategyDate:          "2026-07-13",
				Period:                "morning",
				RecommendationsJSON:   mustAStockTestJSON(t, []aStockRecommendation{{Rank: 1, Hotspot: "????", Code: "600118", Name: "中国卫星", HotspotScore: 82, MarketScore: 217, Reason: "?? ?????????????,???? 7 ?,??? 82;??? 09:27"}}),
				BacktestsJSON:         mustAStockTestJSON(t, []aStockBacktestRow{{Stock: "600118 中国卫星", EntryOpen: "93.50", T0Return: "-2.40%", Status: "等待T+1行情"}}),
				BacktestStatus:        "已读取推荐快照",
				GeneratedCount:        1,
				FundFlowFilterEnabled: true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/code-names":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{
				Total: 1,
				Items: []model.AStockCodeName{{Code: "600118", Name: "中国卫星", Source: "auction"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles":
			writeEnvelope(w, http.StatusOK, "ok", model.ItemListResult{
				Items: []model.Item{{
					ID:          1,
					SourceType:  "flash",
					Title:       "军工航天卫星产业链走强",
					Summary:     "中国卫星获资金关注",
					PublishTime: "2026-07-13 09:00:00",
					TagFlags:    "600118",
					RawPayload:  `{"stocks":[{"code":"600118","name":"中国卫星"}]}`,
				}},
				Page:     1,
				PageSize: 20,
				Total:    1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
				Date: "2026-07-13",
				Items: []model.AStockAuctionAmount{{
					TradeDate:     "2026-07-13",
					Code:          "600118",
					Name:          "中国卫星",
					AuctionAmount: 100000000,
					AuctionVolume: 1000000,
				}},
				Total:       1,
				TotalAmount: 100000000,
			})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	ctx, ok := srv.loadAStockReadOnlySnapshotContextWithCache("2026-07-13", "morning", 1, false, false, false, false, newAStockRequestCache())

	if !ok || len(ctx.Recommendations) != 1 {
		t.Fatalf("expected read-only snapshot recommendation, ok=%v ctx=%+v", ok, ctx)
	}
	got := ctx.Recommendations[0]
	if got.Hotspot != "军工航天" || strings.Contains(got.Reason, "??") {
		t.Fatalf("expected read-only snapshot to repair garbled text, got %+v", got)
	}
}

func TestAStockRepairStockNamesActionPersistsNamesFromLocalDictionary(t *testing.T) {
	var savedSnapshot model.AStockRecommendationSnapshot
	var savedSelections model.AStockRecommendationSelectionSet
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:        true,
				StrategyDate: "2026-07-02",
				Period:       "morning",
				RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{
					{Rank: 1, Code: "301696", Name: "301696", Hotspot: "人工智能", HotspotScore: 90, MarketScore: 100, Reason: "placeholder"},
					{Rank: 2, Code: "002179", Name: "金十数据整理", Hotspot: "军工", HotspotScore: 80, MarketScore: 95, Reason: "placeholder"},
					{Rank: 3, Code: "000021", Name: "主力资金监控", Hotspot: "人工智能", HotspotScore: 70, MarketScore: 90, Reason: "bad inferred name"},
					{Rank: 4, Code: "603698", Name: "????", Hotspot: "军工航天", HotspotScore: 60, MarketScore: 85, Reason: "garbled name"},
				}),
				BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{
					{Stock: "301696 301696", EntryOpen: "10.00", T0Return: "+2.00%", T0Close: "10.20", Status: "等待T+1行情"},
					{Stock: "002179 金十数据整理", EntryOpen: "20.00", T0Return: "+1.00%", T0Close: "20.20", Status: "等待T+1行情"},
					{Stock: "000021 主力资金监控", EntryOpen: "30.00", T0Return: "+1.00%", T0Close: "30.30", Status: "等待T+1行情"},
					{Stock: "603698 ????", EntryOpen: "40.00", T0Return: "-1.00%", T0Close: "39.60", Status: "等待T+1行情"},
				}),
				BacktestStatus: "已读取推荐快照",
				GeneratedCount: 4,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/code-names":
			if !strings.Contains(r.URL.Query().Get("codes"), "301696") || !strings.Contains(r.URL.Query().Get("codes"), "002179") || !strings.Contains(r.URL.Query().Get("codes"), "000021") || !strings.Contains(r.URL.Query().Get("codes"), "603698") {
				t.Fatalf("expected code name lookup for recommendation codes, got %s", r.URL.RawQuery)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{
				Total: 4,
				Items: []model.AStockCodeName{
					{Code: "301696", Name: "测试股份", Source: "auction"},
					{Code: "002179", Name: "中航光电", Source: "auction"},
					{Code: "000021", Name: "深科技", Source: "auction"},
					{Code: "603698", Name: "航天工程", Source: "auction"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendation-selections":
			if err := json.NewDecoder(r.Body).Decode(&savedSelections); err != nil {
				t.Fatalf("decode selections: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Updated: len(savedSelections.Items)})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	form := url.Values{}
	form.Set("date", "2026-07-02")
	form.Set("period", "morning")
	form.Set("action", "repair_stock_names")
	req := httptest.NewRequest(http.MethodPost, "/a-stock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	if len(savedSelections.Items) != 4 || savedSelections.Items[0].Name != "测试股份" || savedSelections.Items[1].Name != "中航光电" || savedSelections.Items[2].Name != "深科技" || savedSelections.Items[3].Name != "航天工程" {
		t.Fatalf("expected repaired names to be saved into selections, got %+v", savedSelections)
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(savedSnapshot.RecommendationsJSON), &recommendations); err != nil {
		t.Fatalf("decode recommendations: %v", err)
	}
	if len(recommendations) != 4 || recommendations[0].Name != "测试股份" || recommendations[1].Name != "中航光电" || recommendations[2].Name != "深科技" || recommendations[3].Name != "航天工程" {
		t.Fatalf("expected repaired names to be saved into snapshot, got %+v", recommendations)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(savedSnapshot.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode backtests: %v", err)
	}
	if len(backtests) != 4 || backtests[0].Stock != "301696 测试股份" || backtests[1].Stock != "002179 中航光电" || backtests[2].Stock != "000021 深科技" || backtests[3].Stock != "603698 航天工程" {
		t.Fatalf("expected repaired names to be saved into backtests, got %+v", backtests)
	}
	decodedLocation, _ := url.QueryUnescape(rr.Header().Get("Location"))
	if !strings.Contains(decodedLocation, "上午推荐已修复：名称 4 只，乱码热点/理由 0 只") {
		t.Fatalf("expected repair summary in redirect, got %q", decodedLocation)
	}
}

func TestAStockBacktestRepairStockNamesActionRepairsVisiblePeriods(t *testing.T) {
	snapshots := map[string]model.AStockRecommendationSnapshot{
		"morning": {
			Found:        true,
			StrategyDate: "2026-07-10",
			Period:       "morning",
			RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{
				{Rank: 1, Code: "688249", Name: "晶合集成", Hotspot: "半导体", Reason: "valid"},
			}),
			BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{
				{Stock: "688249 晶合集成", EntryOpen: "65.21", T0Return: "-17.01%", T0Close: "54.12", Status: "等待T+1行情"},
			}),
			BacktestStatus:        "已读取推荐快照",
			GeneratedCount:        1,
			FundFlowFilterEnabled: true,
		},
		"afternoon": {
			Found:        true,
			StrategyDate: "2026-07-10",
			Period:       "afternoon",
			RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{
				{Rank: 1, Code: "000021", Name: "主力资金监控", Hotspot: "人工智能", Reason: "bad inferred name"},
			}),
			BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{
				{Stock: "000021 主力资金监控", AfternoonOpen: "60.30", T0Return: "-3.96%", T0Close: "57.91", Status: "等待T+1行情"},
			}),
			BacktestStatus:           "已读取推荐快照",
			GeneratedCount:           1,
			LimitUpFilterEnabled:     true,
			FundFlowFilterEnabled:    true,
			TodayMarketFilterEnabled: false,
		},
	}
	savedSnapshots := map[string]model.AStockRecommendationSnapshot{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", snapshots[r.URL.Query().Get("period")])
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/code-names":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{
				Total: 1,
				Items: []model.AStockCodeName{{Code: "000021", Name: "深科技", Source: "auction"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Updated: 1})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			savedSnapshots[snapshot.Period] = snapshot
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	form := url.Values{}
	form.Set("date", "2026-07-10")
	form.Set("period", "morning")
	form.Set("filter_fund_flow", "1")
	form.Set("action", "repair_stock_names")
	req := httptest.NewRequest(http.MethodPost, "/a-stock/backtest", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := savedSnapshots["morning"]; !ok {
		t.Fatalf("expected morning snapshot to be saved, got %+v", savedSnapshots)
	}
	afternoon, ok := savedSnapshots["afternoon"]
	if !ok {
		t.Fatalf("expected afternoon snapshot to be saved, got %+v", savedSnapshots)
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(afternoon.RecommendationsJSON), &recommendations); err != nil {
		t.Fatalf("decode recommendations: %v", err)
	}
	if len(recommendations) != 1 || recommendations[0].Code != "000021" || recommendations[0].Name != "深科技" {
		t.Fatalf("expected afternoon recommendation name repaired, got %+v", recommendations)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(afternoon.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode backtests: %v", err)
	}
	if len(backtests) != 1 || backtests[0].Stock != "000021 深科技" {
		t.Fatalf("expected afternoon backtest name repaired, got %+v", backtests)
	}
	decodedLocation, _ := url.QueryUnescape(rr.Header().Get("Location"))
	if !strings.Contains(decodedLocation, "上午推荐已修复") || !strings.Contains(decodedLocation, "下午推荐已修复") {
		t.Fatalf("expected both visible periods in repair summary, got %q", decodedLocation)
	}
}

func TestAStockBacktestRefreshCurrentOnBacktestPageRefreshesVisiblePeriodsWithDetail(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 7, 14, 30, 0, 0, aStockLocation()))
	snapshots := map[string]model.AStockRecommendationSnapshot{
		"morning": {
			Found:        true,
			StrategyDate: "2026-07-07",
			Period:       "morning",
			RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{
				{Rank: 1, Code: "601881", Name: "中国银河", Hotspot: "金融券商", Reason: "生成点 09:27", EntryTime: "09:30"},
				{Rank: 2, Code: "300394", Name: "天孚通信", Hotspot: "通信设备", Reason: "生成点 09:27", EntryTime: "09:30"},
			}),
			BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{
				{Stock: "601881 中国银河", EntryOpen: "13.47", T0Return: "-2.15%", T0Close: "13.18", Status: "等待T+1行情"},
				{Stock: "300394 天孚通信", EntryOpen: "281.00", T0Return: "-0.60%", T0Close: "279.31", T0ReturnClass: "astock-down", Status: "等待T+1行情"},
			}),
			BacktestStatus:           "已回测 0/1",
			GeneratedCount:           1,
			FundFlowFilterEnabled:    true,
			LimitUpFilterEnabled:     false,
			TodayMarketFilterEnabled: false,
		},
		"afternoon": {
			Found:        true,
			StrategyDate: "2026-07-07",
			Period:       "afternoon",
			RecommendationsJSON: mustAStockTestJSON(t, []aStockRecommendation{
				{Rank: 1, Code: "688702", Name: "盛科通信", Hotspot: "半导体", Reason: "生成点 12:57", EntryTime: "13:01"},
				{Rank: 2, Code: "688820", Name: "盛合晶微", Hotspot: "半导体", Reason: "生成点 12:57", EntryTime: "13:01"},
			}),
			BacktestsJSON: mustAStockTestJSON(t, []aStockBacktestRow{
				{Stock: "688702 盛科通信", AfternoonOpen: "389.96", T0Return: "-3.07%", T0Close: "382.00", Days: []aStockBacktestCell{{Close: "--", Return: "--", ReturnClass: "astock-flat"}}, Status: "等待T+1行情"},
				{Stock: "688820 盛合晶微", AfternoonOpen: "197.60", T0Return: "-0.01%", T0Close: "197.59", Days: []aStockBacktestCell{{Close: "188.32", Return: "-4.70%", ReturnClass: "astock-down"}}, BestReturn: "-4.70%", BestReturnClass: "astock-down", Status: "已回测T+1"},
			}),
			BacktestStatus:           "已回测 1/2",
			GeneratedCount:           2,
			FundFlowFilterEnabled:    true,
			LimitUpFilterEnabled:     true,
			TodayMarketFilterEnabled: false,
		},
	}
	saved := map[string]model.AStockRecommendationSnapshot{}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			period := normalizeAStockPeriod(r.URL.Query().Get("period")).Key
			if snapshot, ok := snapshots[period]; ok {
				writeEnvelope(w, http.StatusOK, "ok", snapshot)
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			saved[snapshot.Period] = snapshot
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": []map[string]any{
			{"code": "601881", "date": "2026-07-07", "open": 13.47, "close": 13.18, "pct": -2.51, "entry_price": 13.47},
			{"code": "601881", "date": "2026-07-08", "open": 13.10, "close": 13.03, "pct": -1.14},
			{"code": "601881", "date": "2026-07-09", "open": 13.05, "close": 12.97, "pct": -0.46},
			{"code": "688702", "date": "2026-07-07", "open": 350.00, "close": 382.00, "pct": 7.00, "afternoon_entry_price": 389.96},
			{"code": "688702", "date": "2026-07-08", "open": 389.39, "close": 401.50, "pct": 5.10},
			{"code": "688702", "date": "2026-07-09", "open": 409.13, "close": 402.03, "pct": 0.13},
			{"code": "688820", "date": "2026-07-07", "open": 183.08, "close": 197.59, "pct": 5.18, "afternoon_entry_price": 197.60},
			{"code": "688820", "date": "2026-07-08", "open": 197.99, "close": 188.32, "pct": -4.69},
			{"code": "688820", "date": "2026-07-09", "open": 191.94, "close": 194.77, "pct": 3.43},
		}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)
	emptyEastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"klines": []string{}}})
	}))
	defer emptyEastmoney.Close()
	setAStockEastmoneyKlineURLForTest(t, emptyEastmoney.URL)
	emptyYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer emptyYahoo.Close()
	setAStockYahooChartURLForTest(t, emptyYahoo.URL)
	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(r.URL.Query().Get("secid"), ".")
		code := ""
		if len(parts) == 2 {
			code = normalizeAStockCode(parts[1])
		}
		rawPriceByCode := map[string]int{
			"601881": 1299,
			"300394": 28430,
			"688702": 40600,
			"688820": 19000,
		}
		rawPrice := rawPriceByCode[code]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"f43": rawPrice,
				"f57": code,
				"f58": code,
			},
		})
	}))
	defer quote.Close()
	previousQuoteURL := aStockEastmoneyQuoteURL
	aStockEastmoneyQuoteURL = quote.URL
	t.Cleanup(func() {
		aStockEastmoneyQuoteURL = previousQuoteURL
	})

	srv := NewServer(config.Config{ContentURL: content.URL})
	form := url.Values{}
	form.Set("date", "2026-07-07")
	form.Set("period", "morning")
	form.Set("action", "refresh_current_backtest")
	req := httptest.NewRequest(http.MethodPost, "/a-stock/backtest", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleAStockBacktestPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := saved["morning"]; !ok {
		t.Fatalf("expected morning snapshot to be refreshed, saved=%+v", saved)
	}
	var morningRows []aStockBacktestRow
	if err := json.Unmarshal([]byte(saved["morning"].BacktestsJSON), &morningRows); err != nil {
		t.Fatalf("decode morning backtests: %v", err)
	}
	morningGot := aStockBacktestRowsByCode(morningRows)
	if morningGot["601881"].CurrentPrice != "12.99" || morningGot["601881"].CurrentReturn != "-3.56%" || morningGot["601881"].CurrentReturnClass != "astock-down" {
		t.Fatalf("expected morning realtime price and return to be written, got %+v", morningGot["601881"])
	}
	if morningGot["300394"].EntryOpen != "281.00" || morningGot["300394"].T0Return != "+1.17%" || morningGot["300394"].T0Close != "284.30" || morningGot["300394"].CurrentPrice != "284.30" || morningGot["300394"].CurrentReturn != "+1.17%" || morningGot["300394"].Status != "等待T+1行情" {
		t.Fatalf("expected 300394 old open to be preserved and T+0 to be refreshed from realtime quote, got %+v", morningGot["300394"])
	}
	afternoon, ok := saved["afternoon"]
	if !ok {
		t.Fatalf("expected afternoon snapshot to be refreshed from backtest page, saved=%+v", saved)
	}
	var rows []aStockBacktestRow
	if err := json.Unmarshal([]byte(afternoon.BacktestsJSON), &rows); err != nil {
		t.Fatalf("decode afternoon backtests: %v", err)
	}
	got := aStockBacktestRowsByCode(rows)
	if got["688702"].Status != "已回测T+2" || len(got["688702"].Days) < 2 || got["688702"].Days[1].Return == "--" {
		t.Fatalf("expected 688702 T+2 data to be written, got %+v", got["688702"])
	}
	if got["688820"].Status != "已回测T+2" || len(got["688820"].Days) < 2 || got["688820"].Days[1].Return == "--" {
		t.Fatalf("expected 688820 T+2 data to be written, got %+v", got["688820"])
	}
	if got["688702"].CurrentPrice != "406.00" || got["688702"].CurrentReturn != "+4.11%" || got["688702"].CurrentReturnClass != "astock-up" {
		t.Fatalf("expected 688702 realtime price and return to be written, got %+v", got["688702"])
	}
	location, err := url.QueryUnescape(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("decode redirect location: %v", err)
	}
	for _, want := range []string{"上午推荐行情收益", "下午推荐行情收益", "获取：date=2026-07-07 period=afternoon codes=688702,688820", "现价：通达信 quote", "现价=--->406.00", "当前收益=--->+4.11%", "补齐：688702 盛科通信", "补齐：688820 盛合晶微"} {
		if !strings.Contains(location, want) {
			t.Fatalf("expected redirect detail to contain %q, got %s", want, location)
		}
	}
}

func TestAStockRealtimeQuoteFillsMissingMorningOpenAndT0Return(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 10, 10, 30, 0, 0, aStockLocation()))
	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.300394" {
			t.Fatalf("unexpected quote request: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"f43":1050,"f46":1000,"f57":"300394","f58":"天孚通信","f170":500}}`))
	}))
	defer quote.Close()
	setAStockEastmoneyQuoteURLForTest(t, quote.URL)

	srv := NewServer(config.Config{})
	rows := srv.enrichAStockBacktestsWithRealtimeQuotes("2026-07-10", "morning", []aStockBacktestRow{{
		Stock:              "300394 天孚通信",
		EntryOpen:          "--",
		T0Return:           "--",
		T0Close:            "--",
		T0ReturnClass:      "astock-flat",
		CurrentPrice:       "--",
		CurrentReturn:      "--",
		CurrentReturnClass: "astock-flat",
		Status:             "无行情数据",
	}})

	if len(rows) != 1 || rows[0].EntryOpen != "10.00" || rows[0].CurrentPrice != "10.50" || rows[0].CurrentReturn != "+5.00%" || rows[0].CurrentMarketPct != "+5.00%" || rows[0].T0Return != "+5.00%" || rows[0].T0Close != "10.50" || rows[0].Status != "等待T+1行情" {
		t.Fatalf("expected realtime quote to fill missing morning open/current/T+0 fields, got %+v", rows)
	}
	bars := srv.realtimeAStockMarketBars("2026-07-10", []string{"300394"})
	if len(bars) != 1 || bars[0].Open != 10 || bars[0].EntryPrice != 10 || bars[0].Close != 10.5 || bars[0].Pct != 5 {
		t.Fatalf("expected realtime quote to produce today market bar, got %+v", bars)
	}
}

func TestAStockRealtimeQuoteUpdatesCurrentReturnBeyondT5WithoutChangingBacktestDays(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 23, 11, 30, 0, 0, aStockLocation()))
	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.301017" {
			t.Fatalf("unexpected quote request: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"f43":1247,"f46":1260,"f57":"301017","f58":"漱玉平民","f170":-197}}`))
	}))
	defer quote.Close()
	setAStockEastmoneyQuoteURLForTest(t, quote.URL)

	srv := NewServer(config.Config{})
	rows := srv.enrichAStockBacktestsWithRealtimeQuotes("2026-07-01", "afternoon", []aStockBacktestRow{{
		Stock:            "301017 漱玉平民",
		EntryOpen:        "13.04",
		AfternoonOpen:    "13.70",
		CurrentPrice:     "--",
		CurrentReturn:    "--",
		CurrentMarketPct: "--",
		Days: []aStockBacktestCell{
			{Close: "13.48", Return: "-1.61%"},
			{Close: "13.49", Return: "-1.53%"},
			{Close: "12.25", Return: "-10.58%"},
			{Close: "12.80", Return: "-6.57%"},
			{Close: "12.72", Return: "-7.15%"},
		},
		Status: "已回测T+5",
	}})

	if len(rows) != 1 {
		t.Fatalf("expected one row, got %+v", rows)
	}
	row := rows[0]
	if row.CurrentPrice != "12.47" || row.CurrentReturn != "-8.98%" || row.CurrentReturnClass != "astock-down" || row.CurrentMarketPct != "-1.97%" {
		t.Fatalf("expected realtime current price and entry return beyond T+5, got %+v", row)
	}
	if len(row.Days) != 5 || row.Days[4].Close != "12.72" || row.Days[4].Return != "-7.15%" || row.Status != "已回测T+5" {
		t.Fatalf("expected historical T+5 backtest cells unchanged, got %+v", row)
	}
}

func TestAStockEastmoneyRealtimeQuoteComputesPctFromPrevClose(t *testing.T) {
	quote, ok := decodeEastmoneyAStockRealtimeQuote([]byte(`{"data":{"f43":5521,"f46":5539,"f57":"600276","f58":"恒瑞医药","f60":5539,"f170":-3200}}`), "600276")
	if !ok {
		t.Fatalf("expected quote to decode")
	}
	if quote.Price != 55.21 {
		t.Fatalf("expected price 55.21, got %+v", quote)
	}
	if got := formatAStockPct(quote.Pct); got != "-0.32%" {
		t.Fatalf("expected pct to be computed from price and previous close, got %+v formatted=%s", quote, got)
	}
}

func TestAStockRealtimeQuotesPreferTongdaxinAndCacheForOneMinute(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 30, 0, 0, aStockLocation())
	previousNow := aStockNow
	aStockNow = func() time.Time { return now }
	t.Cleanup(func() { aStockNow = previousNow })
	tdxCalls := 0
	tdx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tdxCalls++
		if r.URL.Path != "/api/a-stock/tdx-quote" || r.URL.Query().Get("codes") != "300946" {
			t.Fatalf("unexpected tdx request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"code":"300946","price":40.00,"open":38.60,"pct":3.63,"source":"tdx"}]}`))
	}))
	defer tdx.Close()
	eastmoneyCalls := 0
	eastmoney := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eastmoneyCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer eastmoney.Close()
	setAStockEastmoneyQuoteURLForTest(t, eastmoney.URL)

	srv := NewServer(config.Config{AStockQuoteURL: tdx.URL})
	first := srv.loadAStockRealtimeQuotes([]string{"300946"})
	if first["300946"].Price != 40 || first["300946"].Pct != 3.63 || first["300946"].Source != "tdx" || first["300946"].Cached {
		t.Fatalf("expected first quote from tdx, got %+v", first["300946"])
	}
	second := srv.loadAStockRealtimeQuotes([]string{"300946"})
	if second["300946"].Price != 40 || !second["300946"].Cached {
		t.Fatalf("expected second quote from one-minute cache, got %+v", second["300946"])
	}
	if tdxCalls != 1 || eastmoneyCalls != 0 {
		t.Fatalf("expected one tdx call and no eastmoney fallback, got tdx=%d eastmoney=%d", tdxCalls, eastmoneyCalls)
	}
	now = now.Add(61 * time.Second)
	third := srv.loadAStockRealtimeQuotes([]string{"300946"})
	if third["300946"].Price != 40 || third["300946"].Cached {
		t.Fatalf("expected refreshed tdx quote after cache expiry, got %+v", third["300946"])
	}
	if tdxCalls != 2 || eastmoneyCalls != 0 {
		t.Fatalf("expected tdx call after cache expiry and no eastmoney fallback, got tdx=%d eastmoney=%d", tdxCalls, eastmoneyCalls)
	}
}

func TestAStockBacktestRefreshPriceOnlyUpdatesRequestedStock(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 10, 10, 30, 0, 0, aStockLocation()))
	recommendations := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "300394", Name: "天孚通信", CurrentPrice: "--", TodayPct: "--", Reason: "目标股票"},
		{Rank: 2, Hotspot: "人工智能", Code: "688249", Name: "晶合集成", CurrentPrice: "58.40", TodayPct: "-10.44%", Reason: "其他股票"},
	}
	backtests := []aStockBacktestRow{
		{
			Stock:              "300394 天孚通信",
			EntryOpen:          "--",
			T0Return:           "--",
			T0Close:            "--",
			T0ReturnClass:      "astock-flat",
			CurrentPrice:       "--",
			CurrentReturn:      "--",
			CurrentReturnClass: "astock-flat",
			Days:               []aStockBacktestCell{{Close: "--", Return: "--", ReturnClass: "astock-flat"}},
			BestReturn:         "--",
			BestReturnClass:    "astock-flat",
			Status:             "无行情数据",
		},
		{
			Stock:              "688249 晶合集成",
			EntryOpen:          "65.21",
			T0Return:           "-10.44%",
			T0Close:            "58.40",
			T0ReturnClass:      "astock-down",
			CurrentPrice:       "58.40",
			CurrentReturn:      "-10.44%",
			CurrentReturnClass: "astock-down",
			Days:               []aStockBacktestCell{{Close: "58.40", Return: "-10.44%", ReturnClass: "astock-down"}},
			BestReturn:         "-10.44%",
			BestReturnClass:    "astock-down",
			Status:             "已回测T+1",
		},
	}
	var saved model.AStockRecommendationSnapshot
	saveCount := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			if r.URL.Query().Get("date") != "2026-07-10" || r.URL.Query().Get("period") != "morning" {
				t.Fatalf("unexpected snapshot query: %s", r.URL.RawQuery)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                    true,
				StrategyDate:             "2026-07-10",
				Period:                   "morning",
				RecommendationsJSON:      mustAStockTestJSON(t, recommendations),
				BacktestsJSON:            mustAStockTestJSON(t, backtests),
				BacktestStatus:           "已锁定推荐股票，已回测 1/2，无行情数据股票 1",
				FundFlowFilterEnabled:    true,
				LimitUpFilterEnabled:     false,
				TodayMarketFilterEnabled: false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			saveCount++
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("codes") != "300394" {
			t.Fatalf("expected only requested stock to hit market endpoint, got %s", r.URL.RawQuery)
		}
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": []map[string]any{
			{"code": "300394", "date": "2026-07-10", "open": 281.00, "close": 279.31, "pct": -0.60, "entry_price": 281.00},
		}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.300394" {
			t.Fatalf("unexpected quote request: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"f43":28430,"f46":28100,"f57":"300394","f58":"天孚通信","f170":117}}`))
	}))
	defer quote.Close()
	setAStockEastmoneyQuoteURLForTest(t, quote.URL)

	srv := NewServer(config.Config{ContentURL: content.URL, ServiceToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/backtests/refresh-price", strings.NewReader(`{"date":"2026-07-10","period":"morning","code":"300394"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", "secret")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected refresh price 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if saveCount != 1 {
		t.Fatalf("expected exactly one snapshot save, got %d", saveCount)
	}
	var envelope struct {
		Data aStockBacktestRefreshPriceResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if !strings.Contains(envelope.Data.Summary, "300394") || envelope.Data.Snapshot.Period != "morning" {
		t.Fatalf("unexpected refresh response: %+v", envelope.Data)
	}
	var savedRows []aStockBacktestRow
	if err := json.Unmarshal([]byte(saved.BacktestsJSON), &savedRows); err != nil {
		t.Fatalf("decode saved backtests: %v", err)
	}
	rowsByCode := aStockBacktestRowsByCode(savedRows)
	target := rowsByCode["300394"]
	if target.EntryOpen != "281.00" || target.T0Close != "284.30" || target.T0Return != "+1.17%" || target.CurrentPrice != "284.30" || target.CurrentReturn != "+1.17%" || target.CurrentMarketPct != "+1.17%" || target.Status != "等待T+1行情" {
		t.Fatalf("expected target stock price fields to refresh, got %+v", target)
	}
	other := rowsByCode["688249"]
	if other.EntryOpen != "65.21" || other.T0Close != "58.40" || other.CurrentPrice != "58.40" || other.CurrentReturn != "-10.44%" || other.Status != "已回测T+1" {
		t.Fatalf("expected other stock to remain unchanged, got %+v", other)
	}
	var savedRecommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(saved.RecommendationsJSON), &savedRecommendations); err != nil {
		t.Fatalf("decode saved recommendations: %v", err)
	}
	targetRecommendation := mustAStockRecommendationForTest(t, savedRecommendations, "300394")
	if targetRecommendation.CurrentPrice != "284.30" || targetRecommendation.TodayPct != "+1.17%" {
		t.Fatalf("expected target recommendation current fields to refresh, got %+v", targetRecommendation)
	}
	otherRecommendation := mustAStockRecommendationForTest(t, savedRecommendations, "688249")
	if otherRecommendation.CurrentPrice != "58.40" || otherRecommendation.TodayPct != "-10.44%" {
		t.Fatalf("expected other recommendation current fields unchanged, got %+v", otherRecommendation)
	}
}

func TestAStockBacktestRefreshPriceUsesRealtimeMarketPctForHistoricalWindow(t *testing.T) {
	setAStockNowForTest(t, time.Date(2026, 7, 10, 11, 30, 0, 0, aStockLocation()))
	recommendations := []aStockRecommendation{
		{Rank: 1, Hotspot: "软件", Code: "000977", Name: "浪潮信息", CurrentPrice: "85.99", TodayPct: "+10.00%", Reason: "目标股票"},
		{Rank: 2, Hotspot: "软件", Code: "300946", Name: "恒而达", CurrentPrice: "44.00", TodayPct: "+1.00%", Reason: "其他股票"},
	}
	backtests := []aStockBacktestRow{
		{
			Stock:              "000977 浪潮信息",
			EntryOpen:          "83.00",
			T0Return:           "+3.60%",
			T0Close:            "85.99",
			T0ReturnClass:      "astock-up",
			CurrentPrice:       "85.99",
			CurrentReturn:      "+3.60%",
			CurrentReturnClass: "astock-up",
			Days:               []aStockBacktestCell{{Close: "--", Return: "--", ReturnClass: "astock-flat"}},
			BestReturn:         "--",
			BestReturnClass:    "astock-flat",
			Status:             "等待T+1行情",
		},
		{
			Stock:              "300946 恒而达",
			EntryOpen:          "44.00",
			T0Return:           "+1.00%",
			T0Close:            "44.44",
			T0ReturnClass:      "astock-up",
			CurrentPrice:       "44.44",
			CurrentReturn:      "+1.00%",
			CurrentReturnClass: "astock-up",
			Days:               []aStockBacktestCell{{Close: "44.44", Return: "+1.00%", ReturnClass: "astock-up"}},
			BestReturn:         "+1.00%",
			BestReturnClass:    "astock-up",
			Status:             "已回测T+1",
		},
	}
	var saved model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			if r.URL.Query().Get("date") != "2026-07-09" || r.URL.Query().Get("period") != "morning" {
				t.Fatalf("unexpected snapshot query: %s", r.URL.RawQuery)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:                    true,
				StrategyDate:             "2026-07-09",
				Period:                   "morning",
				RecommendationsJSON:      mustAStockTestJSON(t, recommendations),
				BacktestsJSON:            mustAStockTestJSON(t, backtests),
				BacktestStatus:           "已锁定推荐股票，已回测 1/2",
				FundFlowFilterEnabled:    true,
				LimitUpFilterEnabled:     false,
				TodayMarketFilterEnabled: false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatalf("decode saved snapshot: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("codes") != "000977" {
			t.Fatalf("expected only requested stock to hit market endpoint, got %s", r.URL.RawQuery)
		}
		writeEnvelope(w, http.StatusOK, "ok", map[string]any{"items": []map[string]any{
			{"code": "000977", "date": "2026-07-09", "open": 83.00, "close": 85.99, "pct": 10.00, "entry_price": 83.00},
			{"code": "000977", "date": "2026-07-10", "open": 92.00, "close": 90.00, "pct": 4.67},
		}})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "0.000977" {
			t.Fatalf("unexpected quote request: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"f43":9367,"f46":9200,"f57":"000977","f58":"浪潮信息","f170":893}}`))
	}))
	defer quote.Close()
	setAStockEastmoneyQuoteURLForTest(t, quote.URL)

	srv := NewServer(config.Config{ContentURL: content.URL, ServiceToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/internal/a-stock/backtests/refresh-price", strings.NewReader(`{"date":"2026-07-09","period":"morning","code":"000977"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", "secret")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected refresh price 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var savedRows []aStockBacktestRow
	if err := json.Unmarshal([]byte(saved.BacktestsJSON), &savedRows); err != nil {
		t.Fatalf("decode saved backtests: %v", err)
	}
	rowsByCode := aStockBacktestRowsByCode(savedRows)
	target := rowsByCode["000977"]
	if target.CurrentPrice != "93.67" || target.CurrentMarketPct != "+8.93%" || target.CurrentReturn != "+12.86%" {
		t.Fatalf("expected historical refresh to use realtime quote price, market pct, and entry return, got %+v", target)
	}
	if len(target.Days) == 0 || target.Days[0].Close != "93.67" || target.Days[0].Return != "+12.86%" || target.Days[0].MarketPct != "+8.93%" {
		t.Fatalf("expected T+1 cell to store close, entry return, and market pct, got %+v", target.Days)
	}
	other := rowsByCode["300946"]
	if other.CurrentPrice != "44.44" || other.CurrentReturn != "+1.00%" || other.Stock != "300946 恒而达" {
		t.Fatalf("expected other stock to remain unchanged, got %+v", other)
	}
	var savedRecommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(saved.RecommendationsJSON), &savedRecommendations); err != nil {
		t.Fatalf("decode saved recommendations: %v", err)
	}
	targetRecommendation := mustAStockRecommendationForTest(t, savedRecommendations, "000977")
	if targetRecommendation.CurrentPrice != "93.67" || targetRecommendation.TodayPct != "+8.93%" || targetRecommendation.TodayPctClass != "astock-up" {
		t.Fatalf("expected target recommendation to store realtime market pct, got %+v", targetRecommendation)
	}
	otherRecommendation := mustAStockRecommendationForTest(t, savedRecommendations, "300946")
	if otherRecommendation.CurrentPrice != "44.00" || otherRecommendation.TodayPct != "+1.00%" {
		t.Fatalf("expected other recommendation current fields unchanged, got %+v", otherRecommendation)
	}
}

func TestAStockBacktestRefreshPriceMissingCodeDoesNotWriteSnapshot(t *testing.T) {
	recommendations := []aStockRecommendation{{Rank: 1, Code: "300394", Name: "天孚通信"}}
	var wrote bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
				Found:               true,
				StrategyDate:        "2026-07-10",
				Period:              "morning",
				RecommendationsJSON: mustAStockTestJSON(t, recommendations),
				BacktestsJSON:       "[]",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			wrote = true
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	_, status, message := srv.refreshAStockBacktestPrice(aStockBacktestRefreshPriceRequest{
		Date:   "2026-07-10",
		Period: "morning",
		Code:   "688249",
	})
	if status != http.StatusNotFound || !strings.Contains(message, "688249") {
		t.Fatalf("expected missing code 404, got status=%d message=%s", status, message)
	}
	if wrote {
		t.Fatal("expected missing code refresh to avoid snapshot write")
	}
}

func TestAStockSnapshotSaveRepairsNamesFromEastmoneyQuote(t *testing.T) {
	var captured model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/code-names":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{Items: nil})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/auction":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{Date: r.URL.Query().Get("date"), Items: nil})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode snapshot payload: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	quote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payloads := map[string]string{
			"0.301696": `{"data":{"f57":"301696","f58":"三瑞智能"}}`,
			"0.000034": `{"data":{"f57":"000034","f58":"神州数码"}}`,
			"1.601995": `{"data":{"f57":"601995","f58":"中金公司"}}`,
		}
		payload, ok := payloads[r.URL.Query().Get("secid")]
		if !ok {
			t.Fatalf("unexpected quote secid: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer quote.Close()
	setAStockEastmoneyQuoteURLForTest(t, quote.URL)

	srv := NewServer(config.Config{ContentURL: content.URL})
	err := srv.saveAStockRecommendationSnapshot(aStockContext{
		Date:   "2026-07-02",
		Period: "afternoon",
		Recommendations: []aStockRecommendation{
			{Rank: 1, Code: "301696", Name: "金十数据整理", Hotspot: "金融券商", HotspotScore: 978, MarketScore: 1111},
			{Rank: 2, Code: "000034", Name: "金十数据整理", Hotspot: "人工智能", HotspotScore: 900, MarketScore: 1000},
			{Rank: 3, Code: "601995", Name: "中金", Hotspot: "金融券商", HotspotScore: 850, MarketScore: 950},
		},
		Backtests: []aStockBacktestRow{
			{Stock: "301696 金十数据整理", Status: "等待T+1行情"},
			{Stock: "000034 金十数据整理", Status: "等待T+1行情"},
			{Stock: "601995 中金", Status: "等待T+1行情"},
		},
	})
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(captured.RecommendationsJSON), &recommendations); err != nil {
		t.Fatalf("decode recommendations: %v", err)
	}
	gotNames := map[string]string{}
	for _, rec := range recommendations {
		gotNames[rec.Code] = rec.Name
	}
	for code, wantName := range map[string]string{"301696": "三瑞智能", "000034": "神州数码", "601995": "中金公司"} {
		if gotNames[code] != wantName {
			t.Fatalf("expected snapshot recommendation %s to repair name to %s, got %+v", code, wantName, recommendations)
		}
	}
	if len(recommendations) != 3 {
		t.Fatalf("expected snapshot recommendations to repair stock name, got %+v", recommendations)
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(captured.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode backtests: %v", err)
	}
	gotStocks := map[string]struct{}{}
	for _, row := range backtests {
		gotStocks[row.Stock] = struct{}{}
	}
	for _, wantStock := range []string{"301696 三瑞智能", "000034 神州数码", "601995 中金公司"} {
		if _, ok := gotStocks[wantStock]; !ok {
			t.Fatalf("expected snapshot backtests to include repaired stock %q, got %+v", wantStock, backtests)
		}
	}
	if len(backtests) != 3 {
		t.Fatalf("expected snapshot backtests to use repaired stock name, got %+v", backtests)
	}
}

func TestAStockSnapshotSaveRepairsNewsPhraseNameFromCodeNames(t *testing.T) {
	var captured model.AStockRecommendationSnapshot
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/code-names":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{Items: []model.AStockCodeName{
				{Code: "001309", Name: "德明利"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode snapshot payload: %v", err)
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	err := srv.saveAStockRecommendationSnapshot(aStockContext{
		Date:   "2026-07-31",
		Period: "morning",
		Recommendations: []aStockRecommendation{
			{Rank: 1, Code: "001309", Name: "存储芯片大幅高开", Hotspot: "半导体", HotspotScore: 21, MarketScore: 281},
		},
		Backtests: []aStockBacktestRow{
			{Stock: "001309 存储芯片大幅高开", Status: "等待T+1行情"},
		},
	})
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(captured.RecommendationsJSON), &recommendations); err != nil {
		t.Fatalf("decode recommendations: %v", err)
	}
	if len(recommendations) != 1 || recommendations[0].Code != "001309" || recommendations[0].Name != "德明利" {
		t.Fatalf("expected snapshot recommendation name to repair to 德明利, got %+v", recommendations)
	}

	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(captured.BacktestsJSON), &backtests); err != nil {
		t.Fatalf("decode backtests: %v", err)
	}
	if len(backtests) != 1 || backtests[0].Stock != "001309 德明利" {
		t.Fatalf("expected snapshot backtest stock to repair to 001309 德明利, got %+v", backtests)
	}
}

func TestAStockHotspotTopStocksAllowBankStocksButBlockEastmoney(t *testing.T) {
	hotspot := aStockHotspot{Name: "金融券商", Keywords: []string{"证券", "银行"}, Score: 100, Evidence: 1}

	stocks := buildAStockHotspotTopStocks(hotspot, []aStockMarketCandidate{
		{Code: "300059", Name: "东方财富", Rank: 1, AuctionAmount: 9000000},
		{Code: "600036", Name: "招商银行", Rank: 2, AuctionAmount: 8000000},
		{Code: "600030", Name: "中信证券", Rank: 3, AuctionAmount: 7000000},
		{Code: "000001", Name: "平安银行", Rank: 4, AuctionAmount: 6000000},
	}, 3)

	if len(stocks) != 3 || stocks[0].Code != "600036" || stocks[1].Code != "600030" || stocks[2].Code != "000001" {
		t.Fatalf("expected hotspot top stocks to allow banks and block 东方财富, got %+v", stocks)
	}
}

func TestAStockSnapshotRecommendationsUseMorningFinalWindow(t *testing.T) {
	items := []model.Item{
		{ID: 1, SourceType: "flash", Title: "科大讯飞盘中活跃", Summary: "AI 人工智能算力需求增长", PublishTime: "2026-06-18 09:20:00", TagFlags: "0.002230"},
		{ID: 2, SourceType: "flash", Title: "科大讯飞继续活跃", Summary: "AI 人工智能应用落地", PublishTime: "2026-06-18 09:28:00", TagFlags: "0.002230"},
		{ID: 3, SourceType: "flash", Title: "中芯国际成交放量", Summary: "半导体芯片国产替代升温", PublishTime: "2026-06-18 09:28:00", TagFlags: "1.688981"},
	}
	candidates := []aStockMarketCandidate{
		{Code: "002230", Name: "科大讯飞", Rank: 1, AuctionAmount: 20000000, AuctionVolume: 1000000},
		{Code: "688981", Name: "中芯国际", Rank: 2, AuctionAmount: 18000000, AuctionVolume: 900000},
	}

	recommendations := buildAStockSnapshotRecommendations("2026-06-18", "morning", items, candidates)

	if len(recommendations) != 1 {
		t.Fatalf("expected morning final to use one 09:27 snapshot, got %+v", recommendations)
	}
	codes := map[string]struct{}{}
	for _, rec := range recommendations {
		if _, exists := codes[rec.Code]; exists {
			t.Fatalf("expected duplicate news-mentioned codes to be deduplicated, got %+v", recommendations)
		}
		codes[rec.Code] = struct{}{}
	}
	if recommendations[0].Code != "002230" {
		t.Fatalf("expected morning final to keep 09:26:59 news and exclude later semiconductor pool, got %+v", recommendations)
	}
	for _, rec := range recommendations {
		if rec.Code == "688981" {
			t.Fatalf("expected morning final to exclude news after 09:26:59, got %+v", recommendations)
		}
		if strings.Contains(rec.Reason, "生成点 09:24") || strings.Contains(rec.Reason, "生成点 09:30") {
			t.Fatalf("expected morning final to remove old snapshot generation labels, got %+v", recommendations)
		}
		if !strings.Contains(rec.Reason, "生成点 09:27") {
			t.Fatalf("expected recommendations to retain 09:27 generation label, got %+v", recommendations)
		}
	}
}

func TestAStockSnapshotRecommendationsUsePreopenWindows(t *testing.T) {
	morningItems := []model.Item{
		{ID: 1, SourceType: "flash", Title: "科大讯飞盘前活跃", Summary: "AI 人工智能算力需求增长", PublishTime: "2026-06-18 09:26:30", TagFlags: "0.002230"},
		{ID: 2, SourceType: "flash", Title: "中芯国际开盘后放量", Summary: "半导体芯片国产替代升温", PublishTime: "2026-06-18 09:27:00", TagFlags: "1.688981"},
	}
	afternoonItems := []model.Item{
		{ID: 3, SourceType: "flash", Title: "机器人午后预期升温", Summary: "人工智能机器人需求增长", PublishTime: "2026-06-18 12:56:30", TagFlags: "0.300024"},
		{ID: 4, SourceType: "flash", Title: "国元证券午后拉升", Summary: "证券资本市场活跃", PublishTime: "2026-06-18 12:57:00", TagFlags: "0.000728"},
	}
	candidates := []aStockMarketCandidate{
		{Code: "002230", Name: "科大讯飞", Rank: 1, AuctionAmount: 20000000, AuctionVolume: 1000000},
		{Code: "688981", Name: "中芯国际", Rank: 2, AuctionAmount: 19000000, AuctionVolume: 900000},
		{Code: "300024", Name: "机器人", Rank: 3, AuctionAmount: 18000000, AuctionVolume: 800000},
		{Code: "000728", Name: "国元证券", Rank: 4, AuctionAmount: 17000000, AuctionVolume: 700000},
	}

	morning := buildAStockSnapshotRecommendationsWithPhase("2026-06-18", "morning", "preopen", morningItems, candidates)
	if len(morning) != 1 || morning[0].Code != "002230" || !strings.Contains(morning[0].Reason, "生成点 09:27") {
		t.Fatalf("expected morning preopen recommendations to stop before 09:27, got %+v", morning)
	}
	for _, rec := range morning {
		if rec.Code == "688981" {
			t.Fatalf("expected morning preopen recommendations to exclude 09:27 item, got %+v", morning)
		}
	}
	afternoon := buildAStockSnapshotRecommendationsWithPhase("2026-06-18", "afternoon", "preopen", afternoonItems, candidates)
	if len(afternoon) != 1 || afternoon[0].Code != "300024" || !strings.Contains(afternoon[0].Reason, "生成点 12:57") {
		t.Fatalf("expected afternoon preopen recommendations to stop before 12:57, got %+v", afternoon)
	}
	for _, rec := range afternoon {
		if rec.Code == "000728" {
			t.Fatalf("expected afternoon preopen recommendations to exclude 12:57 item, got %+v", afternoon)
		}
	}
}

func TestAStockSnapshotRecommendationsRespectPerHotspotLimitAcrossSnapshots(t *testing.T) {
	items := []model.Item{
		{ID: 1, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 12:40:00", TagFlags: "0.300024"},
		{ID: 2, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 12:41:00", TagFlags: "0.300059"},
		{ID: 3, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 12:42:00", TagFlags: "0.300857"},
		{ID: 4, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 12:58:00", TagFlags: "0.301396"},
		{ID: 5, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 12:59:00", TagFlags: "0.603986"},
		{ID: 6, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 13:00:00", TagFlags: "0.603629"},
	}
	candidates := []aStockMarketCandidate{
		{Code: "300024", Name: "机器人", Rank: 1, AuctionAmount: 20000000, AuctionVolume: 1000000},
		{Code: "300059", Name: "东方财富", Rank: 2, AuctionAmount: 19000000, AuctionVolume: 900000},
		{Code: "300857", Name: "协创数据", Rank: 3, AuctionAmount: 18000000, AuctionVolume: 800000},
		{Code: "301396", Name: "宏景科技", Rank: 4, AuctionAmount: 17000000, AuctionVolume: 700000},
		{Code: "603986", Name: "兆易创新", Rank: 5, AuctionAmount: 16000000, AuctionVolume: 600000},
		{Code: "603629", Name: "利通电子", Rank: 6, AuctionAmount: 15000000, AuctionVolume: 500000},
	}

	recommendations := buildAStockSnapshotRecommendations("2026-06-18", "afternoon", items, candidates)

	if len(recommendations) != 3 {
		t.Fatalf("expected afternoon snapshot merge to keep max 3 stocks for one hotspot, got %+v", recommendations)
	}
	for _, rec := range recommendations {
		if rec.Hotspot != "人工智能" {
			t.Fatalf("expected only AI hotspot recommendations, got %+v", recommendations)
		}
	}
}

func TestAStockSnapshotRecommendationsRespectHotspotLimitAcrossSnapshots(t *testing.T) {
	items := []model.Item{
		{ID: 1, SourceType: "flash", Title: "AI 算力继续升温", Summary: "人工智能景气度提升", PublishTime: "2026-06-18 12:40:00", TagFlags: "0.300024"},
		{ID: 2, SourceType: "flash", Title: "券商板块成交活跃", Summary: "证券与资本市场预期修复", PublishTime: "2026-06-18 12:41:00", TagFlags: "0.000728"},
		{ID: 3, SourceType: "flash", Title: "半导体国产替代提速", Summary: "芯片先进封装景气回升", PublishTime: "2026-06-18 12:42:00", TagFlags: "0.688981"},
		{ID: 4, SourceType: "flash", Title: "新能源储能需求回升", Summary: "光伏与储能景气改善", PublishTime: "2026-06-18 12:58:00", TagFlags: "0.300750"},
	}
	candidates := []aStockMarketCandidate{
		{Code: "300024", Name: "机器人", Rank: 1, AuctionAmount: 20000000, AuctionVolume: 1000000},
		{Code: "000728", Name: "国元证券", Rank: 2, AuctionAmount: 19000000, AuctionVolume: 900000},
		{Code: "688981", Name: "中芯国际", Rank: 3, AuctionAmount: 18000000, AuctionVolume: 800000},
		{Code: "300750", Name: "宁德时代", Rank: 4, AuctionAmount: 17000000, AuctionVolume: 700000},
	}

	recommendations := buildAStockSnapshotRecommendations("2026-06-18", "afternoon", items, candidates)

	if len(recommendations) != 3 {
		t.Fatalf("expected afternoon snapshot merge to keep news-mentioned stocks for max 3 hotspots, got %+v", recommendations)
	}
	hotspots := make(map[string]struct{})
	for _, rec := range recommendations {
		hotspots[rec.Hotspot] = struct{}{}
	}
	if len(hotspots) != 3 {
		t.Fatalf("expected only 3 hotspots after merge, got %+v", recommendations)
	}
}

func TestAStockAfternoonRecommendationsFilterMorningCodes(t *testing.T) {
	morning := []aStockRecommendation{{Code: "002230", Name: "科大讯飞"}}
	afternoon := []aStockRecommendation{
		{Rank: 1, Code: "002230", Name: "科大讯飞"},
		{Rank: 2, Code: "688981", Name: "中芯国际"},
	}

	filtered, skipped := filterAStockRecommendationsByCodes(afternoon, aStockRecommendationCodeSet(morning))

	if skipped != 1 || len(filtered) != 1 {
		t.Fatalf("expected one afternoon duplicate to be filtered, got skipped=%d filtered=%+v", skipped, filtered)
	}
	if filtered[0].Rank != 1 || filtered[0].Code != "688981" {
		t.Fatalf("expected remaining afternoon stock to be reranked, got %+v", filtered)
	}
}

func TestAStockAfternoonRecommendationsRespectDailyHotspotQuota(t *testing.T) {
	morningCounts := map[string]int{
		"人工智能": 1,
		"金融券商": 3,
	}
	afternoon := []aStockRecommendation{
		{Rank: 1, Hotspot: "人工智能", Code: "300024", Name: "机器人", MarketScore: 120},
		{Rank: 2, Hotspot: "人工智能", Code: "300059", Name: "东方财富", MarketScore: 110},
		{Rank: 3, Hotspot: "人工智能", Code: "300857", Name: "协创数据", MarketScore: 100},
		{Rank: 4, Hotspot: "金融券商", Code: "601688", Name: "华泰证券", MarketScore: 130},
		{Rank: 5, Hotspot: "金融券商", Code: "600030", Name: "中信证券", MarketScore: 125},
		{Rank: 6, Hotspot: "半导体", Code: "688981", Name: "中芯国际", MarketScore: 90},
	}

	filtered, skipped := filterAStockRecommendationsByMorningHotspotQuota(afternoon, morningCounts, aStockStocksPerHotspot)

	if skipped != 3 {
		t.Fatalf("expected one AI and two finance recommendations to be skipped, got skipped=%d filtered=%+v", skipped, filtered)
	}
	wantCodes := []string{"300024", "300059", "688981"}
	if len(filtered) != len(wantCodes) {
		t.Fatalf("expected daily hotspot quota to keep %d recommendations, got %+v", len(wantCodes), filtered)
	}
	for i, want := range wantCodes {
		if filtered[i].Code != want || filtered[i].Rank != i+1 {
			t.Fatalf("expected filtered recommendation %d to be %s with rerank, got %+v", i+1, want, filtered)
		}
	}
}

func TestAStockAfternoonRecommendationsRespectDailyTotalLimit(t *testing.T) {
	afternoon := []aStockRecommendation{
		{Rank: 1, Hotspot: "AI", Code: "300001", Name: "Stock A", MarketScore: 120},
		{Rank: 2, Hotspot: "AI", Code: "300002", Name: "Stock B", MarketScore: 110},
		{Rank: 3, Hotspot: "Chips", Code: "300003", Name: "Stock C", MarketScore: 100},
	}

	filtered, skipped := limitAStockRecommendationsByCount(afternoon, remainingAStockDailyRecommendationLimit(3))
	if skipped != 1 || len(filtered) != 2 || filtered[0].Code != "300001" || filtered[1].Code != "300002" {
		t.Fatalf("expected morning 3 recommendations to leave two afternoon slots, skipped=%d filtered=%+v", skipped, filtered)
	}
	filtered, skipped = limitAStockRecommendationsByCount(afternoon, remainingAStockDailyRecommendationLimit(4))
	if skipped != 2 || len(filtered) != 1 || filtered[0].Code != "300001" {
		t.Fatalf("expected morning 4 recommendations to leave one afternoon slot, skipped=%d filtered=%+v", skipped, filtered)
	}
	filtered, skipped = limitAStockRecommendationsByCount(afternoon, remainingAStockDailyRecommendationLimit(5))
	if skipped != 3 || len(filtered) != 0 {
		t.Fatalf("expected morning 5 recommendations to block afternoon slots, skipped=%d filtered=%+v", skipped, filtered)
	}
}

func TestAStockAfternoonSameDayCapsUsePersistedMorningDailyTotal(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("date") == "2026-07-03" && r.URL.Query().Get("period") == "morning" {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found: true,
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "600001", Name: "Morning A", Hotspot: "One"},
						{Rank: 2, Code: "600002", Name: "Morning B", Hotspot: "Two"},
						{Rank: 3, Code: "600003", Name: "Morning C", Hotspot: "Three"},
						{Rank: 4, Code: "600004", Name: "Morning D", Hotspot: "Four"},
						{Rank: 5, Code: "600005", Name: "Morning E", Hotspot: "Five"},
					},
				})
				return
			}
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{Found: false})
		case "/api/v1/a-stock/recommendations":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{Found: false})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	ctx := aStockContext{
		Date:   "2026-07-03",
		Period: "afternoon",
		Recommendations: []aStockRecommendation{
			{Rank: 1, Hotspot: "AI", Code: "300001", Name: "Stock A", MarketScore: 120},
			{Rank: 2, Hotspot: "Chips", Code: "300002", Name: "Stock B", MarketScore: 110},
			{Rank: 3, Hotspot: "Robotics", Code: "300003", Name: "Stock C", MarketScore: 100},
		},
	}
	NewServer(config.Config{ContentURL: content.URL}).applyAStockAfternoonSameDayCapsWithCache(&ctx, nil, newAStockRequestCache())
	if len(ctx.Recommendations) != 0 || ctx.SameDayMorningFiltered != 3 {
		t.Fatalf("expected morning five recommendations to fill daily slots, got filtered=%d recommendations=%+v", ctx.SameDayMorningFiltered, ctx.Recommendations)
	}
}

func TestAStockRecommendationsIncludeMatchedFullMarketCandidate(t *testing.T) {
	recommendations := buildAStockRecommendations([]aStockHotspot{
		{
			Name:     "金融券商",
			Keywords: []string{"证券", "资本市场"},
			Score:    245,
			Evidence: 2,
			MatchedItems: []model.Item{
				{SourceType: "flash", Title: "华泰证券：资金面仍具活跃基础", Summary: "资本市场情绪修复"},
			},
		},
	}, []aStockMarketCandidate{
		{Code: "601688", Name: "华泰证券", Rank: 320, AuctionAmount: 1200000, AuctionVolume: 50000},
	})

	if len(recommendations) != 1 {
		t.Fatalf("expected matched full-market candidate recommendation, got %+v", recommendations)
	}
	got := map[string]string{}
	for _, rec := range recommendations {
		got[rec.Code] = rec.Name
	}
	if got["601688"] != "华泰证券" {
		t.Fatalf("expected matched full-market stock 华泰证券, got %+v", recommendations)
	}
	if _, ok := got["300059"]; ok {
		t.Fatalf("expected 东方财富 to remain excluded, got %+v", recommendations)
	}
}

func TestAStockContextFallsBackToLatestAuctionDictionary(t *testing.T) {
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []map[string]any{
				{"code": "600030", "date": "2026-06-16", "open": 27.10, "close": 27.25, "pct": 1.45},
				{"code": "600030", "date": "2026-06-17", "open": 27.37, "close": 27.37, "pct": 0.44},
			}},
		})
	}))
	defer market.Close()
	t.Setenv("YUQING_ASTOCK_MARKET_URL", market.URL)

	auctionQueries := make([]string, 0)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockRecommendationSnapshotTestEndpoint(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/v1/a-stock/holdings/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingSummary{}})
		case "/api/v1/a-stock/auction":
			auctionQueries = append(auctionQueries, r.URL.RawQuery)
			if r.URL.Query().Get("date") == "2026-06-17" {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": model.AStockAuctionListResult{
					Date: "2026-06-17", Page: 1, PageSize: 5000, Total: 0, Items: []model.AStockAuctionAmount{},
				}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.AStockAuctionListResult{
				Date: "2026-06-18", Page: 1, PageSize: 5000, Total: 1,
				Items: []model.AStockAuctionAmount{{TradeDate: "2026-06-18", Code: "600030", Name: "中信证券", AuctionVolume: 100000, AuctionAmount: 1500000, Status: "ok"}},
			}})
		case "/api/v1/articles":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.ItemListResult{
				Items: []model.Item{{ID: 110, SourceType: "flash", Title: "华泰证券：资金面仍具活跃基础", Summary: "证券资本市场活跃", PublishTime: "2026-06-17 08:37:00"}},
				Page:  1, PageSize: 200, Total: 1,
			}})
		case "/api/v1/a-stock/sector-fund-flows":
			writeEnvelope(w, http.StatusOK, "ok", model.AStockSectorFundFlowListResult{})
		default:
			if handleEmptyAStockAuctionTestEndpoint(w, r) {
				return
			}
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
	}))
	defer content.Close()

	scheduler := newAStockTradingDayServer(t, true)
	defer scheduler.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, SchedulerURL: scheduler.URL})
	ctx := srv.loadAStockContext("2026-06-17", "morning", 1, true)

	if len(ctx.Recommendations) != 1 {
		t.Fatalf("expected latest auction dictionary to produce recommendation, got %+v empty=%q", ctx.Recommendations, ctx.EmptyReason)
	}
	got := aStockTestRecommendationsByCode(ctx.Recommendations)
	if got["600030"].Name != "中信证券" {
		t.Fatalf("unexpected latest dictionary recommendations: %+v", ctx.Recommendations)
	}
	if len(auctionQueries) < 2 || !strings.Contains(auctionQueries[0], "date=2026-06-17") || strings.Contains(auctionQueries[1], "date=") {
		t.Fatalf("expected date-specific auction lookup then latest fallback, got %v", auctionQueries)
	}
}

func TestAStockMarketCandidateResultUsesServerCache(t *testing.T) {
	auctionHits := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handleAStockAlgorithmSettingsTestEndpoint(w, r) {
			return
		}
		if r.URL.Path != "/api/v1/a-stock/auction" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		auctionHits++
		writeEnvelope(w, http.StatusOK, "ok", model.AStockAuctionListResult{
			Date:     r.URL.Query().Get("date"),
			Page:     1,
			PageSize: 5000,
			Total:    1,
			Items: []model.AStockAuctionAmount{
				{TradeDate: r.URL.Query().Get("date"), Code: "600030", Name: "中信证券", AuctionAmount: 1500000, AuctionVolume: 100000, Status: "ok"},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	first, ok := srv.loadAStockMarketCandidateResult("2026-07-02")
	if !ok || len(first.Items) != 1 {
		t.Fatalf("expected first auction result, got ok=%v result=%+v", ok, first)
	}
	second, ok := srv.loadAStockMarketCandidateResult("2026-07-02")
	if !ok || len(second.Items) != 1 {
		t.Fatalf("expected cached auction result, got ok=%v result=%+v", ok, second)
	}
	if auctionHits != 1 {
		t.Fatalf("expected second date lookup to use server cache, got %d hits", auctionHits)
	}

	srv.clearAStockAuctionCandidateCache("2026-07-02")
	third, ok := srv.loadAStockMarketCandidateResult("2026-07-02")
	if !ok || len(third.Items) != 1 {
		t.Fatalf("expected auction result after cache clear, got ok=%v result=%+v", ok, third)
	}
	if auctionHits != 2 {
		t.Fatalf("expected cache clear to force reload, got %d hits", auctionHits)
	}
}

func TestAStockRecommendationsApplyHoldingSummaryBonus(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleAStockAlgorithmSettingsTestEndpoint(w, r) {
			return
		}
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
	t.Setenv("YUQING_A_STOCK_NEWS_SOURCE_CONFIG", filepath.Join(t.TempDir(), "sources.json"))
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
	if strings.Join(sources, ",") != "cls_telegraph,eastmoney_full,eastmoney_kuaixun,jin10_full,jin10_kuaixun,jin10_资讯,sina_finance_7x24,wallstreetcn_a_stock" {
		t.Fatalf("expected all A股 public news sources to be crawled, got %v", sources)
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

func TestArticlesPageDefaultsToRealtimeSyncAndNoFavoriteAction(t *testing.T) {
	oldCommit, oldBuildTime, oldBranch := app.GitCommit, app.BuildTime, app.BranchName
	app.GitCommit = "abcdef1"
	app.BuildTime = "2026-06-12T06:17:25Z"
	app.BranchName = "golang-jin10-sqlite"
	t.Cleanup(func() {
		app.GitCommit = oldCommit
		app.BuildTime = oldBuildTime
		app.BranchName = oldBranch
	})

	articleRawQuery := ""
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			articleRawQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.ItemListResult{
					Total:    4,
					PageSize: 20,
					Page:     1,
					Items: []model.Item{
						{
							ID:          1,
							Title:       "上海时间测试文章",
							SourceType:  "flash",
							PublishTime: "2026-06-12T06:17:25Z",
							CapturedAt:  time.Date(2026, 6, 12, 1, 17, 25, 0, time.UTC),
							Read:        false,
							Favorited:   true,
						},
						{
							ID:              2,
							Title:           "PANews 测试文章",
							SourceType:      "panews_newsflash",
							PublishTimeText: "2026/6/12 15:17:25",
							CapturedAt:      time.Date(2026, 6, 12, 2, 17, 25, 0, time.UTC),
							Read:            true,
						},
						{
							ID:         3,
							Title:      "CoinDesk 测试文章",
							SourceType: "coindesk_zh_latest",
							CapturedAt: time.Date(2026, 6, 12, 8, 17, 25, 0, time.UTC),
						},
						{
							ID:              4,
							Title:           "Foresight 测试文章",
							SourceType:      "foresight_newsflash",
							PublishTimeText: "7分钟前",
							CapturedAt:      time.Date(2026, 6, 12, 9, 17, 25, 0, time.UTC),
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
	if !strings.Contains(articleRawQuery, "time_field=captured_at") || !strings.Contains(articleRawQuery, "sort=captured_at_desc") {
		t.Fatalf("expected article list request to default to captured_at realtime sync, got query %q", articleRawQuery)
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
		`<th class="col-title">标题</th><th class="col-source">来源</th><th class="col-time">同步时间</th><th class="col-actions">操作</th>`,
		`name="sort"`,
		`实时同步`,
		`发布时间`,
		`金十`,
		`PANews`,
		`CoinDesk`,
		`Foresight`,
		`<td>2026-06-12 09:17</td>`,
		`<td>2026-06-12 10:17</td>`,
		`<td>2026-06-12 16:17</td>`,
		`<td>2026-06-12 17:17</td>`,
		`隐藏文章`,
		`Code By Yuhao@jiansutech.com - 2026-06-12 14:17:25 UTC+8 - abcdef1 - golang-jin10-sqlite`,
	} {
		if !strings.Contains(renderedText, expected) {
			t.Fatalf("expected article list UI fragment %q in body: %s", expected, body)
		}
	}
	if strings.Contains(renderedText, `删除文章`) {
		t.Fatalf("expected article list to stop showing delete label, got %s", body)
	}
	for _, unexpected := range []string{`7分钟前`, `<td>2026-06-12 01:17</td>`, `<td>2026-06-12 06:17</td>`} {
		if strings.Contains(renderedText, unexpected) {
			t.Fatalf("expected article list to show realtime sync time in Shanghai instead of %q, got %s", unexpected, body)
		}
	}
}

func TestArticlesPagePublishTimeSortPreservesFilters(t *testing.T) {
	articleRawQuery := ""
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/articles":
			articleRawQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.ItemListResult{
					Total:    1,
					PageSize: 20,
					Page:     2,
					Items: []model.Item{
						{
							ID:          11,
							Title:       "发布时间模式文章",
							SourceType:  "flash",
							PublishTime: "2026-06-12T06:17:25Z",
							CapturedAt:  time.Date(2026, 6, 12, 1, 17, 25, 0, time.UTC),
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
	req := httptest.NewRequest(http.MethodGet, "/articles?page=2&sort=publish_time_desc&project_id=12&source_type=flash&read=unread&favorite=favorited&start=2026-06-12&end=2026-06-13", nil)
	rr := httptest.NewRecorder()

	srv.handleArticles(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	values, err := url.ParseQuery(articleRawQuery)
	if err != nil {
		t.Fatalf("parse article query %q: %v", articleRawQuery, err)
	}
	for key, want := range map[string]string{
		"time_field":  "publish_time",
		"sort":        "publish_time_desc",
		"page":        "2",
		"project_id":  "12",
		"source_type": "flash",
		"read":        "unread",
		"favorite":    "favorited",
		"start":       "2026-06-12",
		"end":         "2026-06-13",
	} {
		if got := values.Get(key); got != want {
			t.Fatalf("expected query %s=%q, got %q in %q", key, want, got, articleRawQuery)
		}
	}
	renderedText := strings.ReplaceAll(html.UnescapeString(rr.Body.String()), "&#43;", "+")
	for _, expected := range []string{
		`<th class="col-title">标题</th><th class="col-source">来源</th><th class="col-time">发布时间</th><th class="col-actions">操作</th>`,
		`<td>2026-06-12 14:17</td>`,
		`时间轴<strong>发布时间</strong>`,
	} {
		if !strings.Contains(renderedText, expected) {
			t.Fatalf("expected publish-time article UI fragment %q in body: %s", expected, rr.Body.String())
		}
	}
}

func TestHandleArticlesHideActionUsesSoftDeleteEndpoint(t *testing.T) {
	hideCalls := 0
	var deletePath string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/articles/101":
			hideCalls++
			deletePath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": map[string]bool{"deleted": true}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, HTTPTimeout: time.Second})
	form := url.Values{
		"item_id": {"101"},
		"action":  {"hide"},
	}
	req := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://portal.local/articles?page=2")
	rr := httptest.NewRecorder()

	srv.handleArticles(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after hide action, got %d body=%s", rr.Code, rr.Body.String())
	}
	if hideCalls != 1 || deletePath != "/api/v1/articles/101" {
		t.Fatalf("expected hide action to call soft delete endpoint once, got calls=%d path=%q", hideCalls, deletePath)
	}
	location := rr.Header().Get("Location")
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("expected valid redirect URL, got %q err=%v", location, err)
	}
	query := redirectURL.Query()
	if redirectURL.Path != "/articles" || query.Get("page") != "2" || query.Get("msg") != "文章已隐藏" {
		t.Fatalf("expected redirect back to article list with hidden message, got %q", location)
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
		"hotspots":                hotspotsTemplate,
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

func TestPortalNavPlacesLogoutAfterUpgrade(t *testing.T) {
	if !strings.Contains(portalNavHTML, `<button id="portal-upgrade-button" class="portal-upgrade-button" type="button">升级</button><a class="logout-link" href="/logout">退出</a>`) {
		t.Fatalf("expected logout link to appear immediately after upgrade button")
	}
	if strings.Contains(baseStyles, `.logout-link{position:fixed`) {
		t.Fatalf("expected logout link to inherit nav text-link styles")
	}
	if !strings.Contains(portalNavHTML, `<a href="/system">系统</a><a href="/logs">日志</a><button id="portal-upgrade-button" class="portal-upgrade-button" type="button">升级</button>`) {
		t.Fatalf("expected upgrade button to appear immediately after logs menu")
	}
	for _, expected := range []string{
		`nav .portal-upgrade-button{display:inline-flex`,
		`.portal-upgrade-mask{position:fixed`,
		`.portal-upgrade-console-bar{display:flex`,
		`.portal-upgrade-log{margin:0`,
	} {
		if !strings.Contains(baseStyles, expected) {
			t.Fatalf("expected upgrade style %q", expected)
		}
	}
	for _, expected := range []string{
		`id="portal-upgrade-mask"`,
		`id="portal-upgrade-console-state"`,
		`fetch("/system/upgrade"`,
		`fetch("/system/upgrade/status"`,
		`data.message||data.msg||data.error`,
		`data.__httpStatus=resp.status`,
		`upgradeLog(data)`,
		`data.status==="running"`,
		`data.status==="restarting"`,
		`finalMessage=upgradeMessage(data)`,
		`setTimeout(hide,120000)`,
		`?"重启中":"运行中"`,
		`setLog(data.log||runningMessage,true)`,
		`sessionStorage.setItem(upgradeActiveKey,"1")`,
		`failedPolls>=6&&hasRememberedUpgrade()`,
		`服务重启中，仍在等待恢复连接`,
		`正在重新连接升级状态`,
		`log.scrollTop=log.scrollHeight`,
	} {
		if !strings.Contains(portalUpgradeShellHTML, expected) {
			t.Fatalf("expected upgrade shell to include %q", expected)
		}
	}
	if strings.Contains(portalUpgradeShellHTML, `window.location.reload()`) {
		t.Fatalf("expected upgrade shell not to navigate away during service restart")
	}
	if strings.Contains(portalUpgradeShellHTML, `状态检查`) {
		t.Fatalf("expected upgrade shell to keep polling status out of console log")
	}
}

func TestPortalNavMovesManagementLinksUnderSystemAndAddsHotspots(t *testing.T) {
	if !strings.Contains(portalNavHTML, `<a href="/articles">文章</a><a href="/hotspots">热点</a><a href="/a-stock">A股</a>`) {
		t.Fatalf("expected hotspots nav link immediately after articles, got %s", portalNavHTML)
	}
	for _, unexpected := range []string{
		`<a href="/projects">项目</a>`,
		`<a href="/monitor-rules">规则</a>`,
		`<a href="/reports">报告</a>`,
		`<a href="/crawl-templates">模板中心</a>`,
		`<a href="/crawl-templates/manage">模板管理</a>`,
	} {
		if strings.Contains(portalNavHTML, unexpected) {
			t.Fatalf("expected top nav to omit management link %q", unexpected)
		}
	}
	systemPrefix := `<div class="tabs"><a href="/projects">项目</a><a href="/monitor-rules">规则</a><a href="/reports">报告</a><a href="/crawl-templates">模板中心</a><a href="/crawl-templates/manage">模板管理</a><a class="{{if eq .SectionKey "services"}}active{{end}}" href="/system?section=services">服务状态</a>`
	if !strings.Contains(systemTemplate, systemPrefix) {
		t.Fatalf("expected system tabs to place management links before services")
	}
}

func TestSimplePageIncludesPortalUpgradeShell(t *testing.T) {
	srv := NewServer(config.Config{})
	rr := httptest.NewRecorder()
	if err := srv.writeSimplePage(rr, "test-page", "测试页", "<section>body</section>"); err != nil {
		t.Fatalf("writeSimplePage error: %v", err)
	}
	body := rr.Body.String()
	for _, expected := range []string{
		`id="portal-upgrade-button"`,
		`id="portal-upgrade-mask"`,
		`/system/upgrade`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected simple page to include upgrade shell %q, got %s", expected, body)
		}
	}
}

type fakePortalUpgradeRunner struct {
	called     chan struct{}
	calledOnce sync.Once
	done       chan struct{}
	doneOnce   sync.Once
	release    <-chan struct{}
	result     portalUpgradeResult
}

func (f *fakePortalUpgradeRunner) Run(_ context.Context, _ config.Config, _ portalUpgradeProgress) portalUpgradeResult {
	defer f.doneOnce.Do(func() {
		if f.done != nil {
			close(f.done)
		}
	})
	f.calledOnce.Do(func() {
		if f.called != nil {
			close(f.called)
		}
	})
	if f.release != nil {
		<-f.release
	}
	return f.result
}

func TestSystemUpgradeEndpointStartsBackgroundRunnerAndStatusReturnsLog(t *testing.T) {
	t.Setenv(portalUpgradeStatusFileEnv, filepath.Join(t.TempDir(), "status.json"))
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	release := make(chan struct{})
	fake := &fakePortalUpgradeRunner{called: make(chan struct{}), done: make(chan struct{}), release: release, result: portalUpgradeResult{
		OK:         true,
		Status:     "success",
		Message:    "升级完成",
		Log:        "git pull --ff-only origin golang\nbuild ok",
		StartedAt:  time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 6, 30, 9, 1, 0, 0, time.UTC),
	}}
	srv.upgradeRunner = fake

	req := httptest.NewRequest(http.MethodPost, "/system/upgrade", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected upgrade 202, got %d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-fake.called:
	case <-time.After(time.Second):
		t.Fatal("expected fake upgrade runner to be called")
	}
	var result portalUpgradeResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal upgrade response: %v", err)
	}
	if !result.OK || result.Status != "running" || !result.Running || !strings.Contains(result.Log, "后台执行") {
		t.Fatalf("unexpected initial upgrade response: %+v", result)
	}

	close(release)
	select {
	case <-fake.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fake upgrade runner to finish")
	}
	statusReq := httptest.NewRequest(http.MethodGet, "/system/upgrade/status", nil)
	statusReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-admin"})
	var statusResult portalUpgradeResult
	deadline := time.Now().Add(time.Second)
	for {
		statusRR := httptest.NewRecorder()
		srv.Router().ServeHTTP(statusRR, statusReq)
		if statusRR.Code != http.StatusOK {
			t.Fatalf("expected upgrade status 200, got %d body=%s", statusRR.Code, statusRR.Body.String())
		}
		if err := json.Unmarshal(statusRR.Body.Bytes(), &statusResult); err != nil {
			t.Fatalf("unmarshal upgrade status response: %v", err)
		}
		if statusResult.Status == "success" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for upgrade status: %+v", statusResult)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !statusResult.OK || statusResult.Running || !strings.Contains(statusResult.Log, "build ok") {
		t.Fatalf("unexpected final upgrade status: %+v", statusResult)
	}
}

func TestSystemUpgradeStatusReturnsRunningSnapshotWhenSessionInvalidDuringUpgrade(t *testing.T) {
	t.Setenv(portalUpgradeStatusFileEnv, filepath.Join(t.TempDir(), "status.json"))
	startedAt := time.Now().UTC().Add(-2 * time.Minute)
	persistPortalUpgradeStatus(portalUpgradeResult{
		OK:        true,
		Status:    "running",
		Message:   "阶段: 打包构建",
		Log:       "debug: 命令仍在运行，已耗时 2m0s",
		StartedAt: startedAt,
		Running:   true,
	})
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/system/upgrade/status", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "stale-session"})
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected upgrade status to survive invalid session during running upgrade, got %d body=%s", rr.Code, rr.Body.String())
	}
	var result portalUpgradeResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal upgrade status response: %v", err)
	}
	if !result.OK || result.Status != "running" || !result.Running || !strings.Contains(result.Log, "命令仍在运行") {
		t.Fatalf("expected running upgrade snapshot, got %+v", result)
	}
}

func TestSystemUpgradeEndpointReturnsReadableAuthFailure(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	for _, path := range []string{"/system/upgrade", "/system/upgrade/status"} {
		method := http.MethodPost
		if strings.HasSuffix(path, "/status") {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, path, nil)
		rr := httptest.NewRecorder()
		srv.Router().ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected auth failure 403 for %s, got %d body=%s", path, rr.Code, rr.Body.String())
		}
		var result portalUpgradeResult
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal auth failure response for %s: %v", path, err)
		}
		if result.OK || result.Status != "failed" || result.Message != "未登录" || !strings.Contains(result.Log, "升级请求失败: 未登录") {
			t.Fatalf("expected readable upgrade auth failure for %s, got %+v", path, result)
		}
	}
}

func TestRunPortalUpgradeCommandIgnoresAllowedFailure(t *testing.T) {
	var log bytes.Buffer

	err := runPortalUpgradeCommand(context.Background(), &log, t.TempDir(), true, "go", "tool", "codex-not-a-real-go-tool")

	if err != nil {
		t.Fatalf("expected allowed upgrade command failure to be ignored, got %v", err)
	}
	if !strings.Contains(log.String(), "忽略非关键命令失败") {
		t.Fatalf("expected ignored failure to be logged, got %s", log.String())
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

func TestHotspotsPageRendersSwitchingData(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/sector-fund-flow-intraday" {
			if r.URL.Query().Get("sector_type") != "概念资金流" || r.URL.Query().Get("indicator") != "今日" {
				t.Fatalf("unexpected intraday query %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorFundFlowIntradayResult{
					Date:       "2026-07-14",
					SectorType: "概念资金流",
					Indicator:  "今日",
					LatestTime: "13:11",
					Times:      []string{"09:30", "13:11"},
					Series: []model.AStockSectorFundFlowIntradaySeries{{
						Name:                "创新药",
						LatestRank:          1,
						LatestMainNetInflow: 8005000000,
						Points: []model.AStockSectorFundFlowIntradayPoint{
							{Time: "09:30", MainNetInflow: 100000000, Rank: 1},
							{Time: "13:11", MainNetInflow: 8005000000, Rank: 1},
						},
					}},
					Top: []model.AStockSectorFundFlow{{TradeDate: "2026-07-14", SectorType: "概念资金流", Indicator: "今日", Rank: 1, Name: "创新药", MainNetInflow: 8005000000}},
				},
			})
			return
		}
		if r.URL.Path != "/api/v1/hotspots/switching" || r.URL.Query().Get("days") != "14" {
			t.Fatalf("unexpected content request %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": model.HotspotSwitchingResult{
				Days:          14,
				StartDate:     "2026-07-01",
				EndDate:       "2026-07-14",
				TotalArticles: 12,
				TodayTop: []model.HotspotSwitchingItem{{
					Keyword:    "AI",
					TodayCount: 3,
					Count14D:   8,
					Trend:      []model.HotspotDailyCount{{Date: "2026-07-13", Count: 1}, {Date: "2026-07-14", Count: 3}},
				}},
				Top: []model.HotspotSwitchingItem{{
					Keyword:       "AI",
					Count14D:      8,
					CountRecent7D: 6,
					CountPrev7D:   2,
					ChangeRate:    2,
				}},
				Rising: []model.HotspotSwitchingItem{{
					Keyword:       "机器人",
					CountRecent7D: 4,
					CountPrev7D:   1,
					ChangeRate:    3,
					ActiveDays:    3,
				}},
				DiscoveryTodayTop: []model.HotspotSwitchingItem{{
					Keyword:    "涨幅",
					TodayCount: 2,
					Count14D:   4,
				}},
				DiscoveryRising: []model.HotspotSwitchingItem{{
					Keyword:       "订单",
					CountRecent7D: 3,
					CountPrev7D:   1,
					ChangeRate:    2,
				}},
				Cooling: []model.HotspotSwitchingItem{{
					Keyword:       "黄金",
					CountRecent7D: 1,
					CountPrev7D:   5,
					ChangeRate:    -0.8,
					LastSeenDate:  "2026-07-11",
				}},
				New: []model.HotspotSwitchingItem{{
					Keyword:       "低空经济",
					CountRecent7D: 2,
					FirstSeenDate: "2026-07-13",
					LastSeenDate:  "2026-07-14",
				}},
				ContinuousRising: []model.HotspotSwitchingItem{{
					Keyword:       "算力",
					CountRecent7D: 5,
					Count14D:      6,
					SwitchScore:   18,
				}},
				Switches: []model.HotspotSwitchingPair{{From: "黄金", To: "AI", FromCount: 5, ToCount: 6, SwitchScore: 21}},
				Daily:    []model.HotspotDailyHotspot{{Date: "2026-07-14", Items: []model.HotspotSwitchingItem{{Keyword: "AI"}}}},
			},
		})
	}))
	defer content.Close()
	srv := NewServer(config.Config{ContentURL: content.URL})
	srv.now = func() time.Time {
		return time.Date(2026, 7, 14, 13, 12, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hotspots?days=14&sector_type=%E6%A6%82%E5%BF%B5%E8%B5%84%E9%87%91%E6%B5%81", nil)
	srv.handleHotspots(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected hotspots page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, expected := range []string{
		"热点切换监控",
		"当日板块资金流向",
		"13:11",
		"创新药",
		`html,body{overflow-x:hidden}`,
		`.fundflow-section{width:98vw`,
		`.fundflow-chart{width:100%;height:865px`,
		`viewBox="0 0 1104 865"`,
		`svg.setAttribute("viewBox","0 0 "+width+" "+height)`,
		"function smoothSeriesPoints(points)",
		"function bestFundFlowPoint(points)",
		"function latestFundFlowPoint(points)",
		"idx-4",
		"maxInflow=values.reduce",
		"maxOutflow=values.reduce",
		"yStep=10",
		"yMaxInflow=Math.ceil",
		"yMaxOutflow=Math.ceil",
		"*1.10",
		"function yForValue(value)",
		"function drawYGrid(tick,scale)",
		"for(var upTick=0",
		"for(var downTick=-yStep",
		"morningStart=9*60+30",
		"morningEnd=11*60+30",
		"afternoonStart=13*60",
		"tradingDuration=(morningEnd-morningStart)+(afternoonEnd-afternoonStart)",
		"function xForMinute(minute)",
		`["10:00",10*60,height-18]`,
		`["13:30",13*60+30,height-18]`,
		`["14:30",14*60+30,height-18]`,
		"drawnXTicks",
		"endLabels.sort",
		"labelGap=Math.max",
		"latestPoint=latestFundFlowPoint(s.points)",
		"rawYi(latestPoint.main_net_inflow)!==0",
		"labelX:Math.min(Math.max(x+10,left+8),width-216)",
		"zeroY=top+plotH/2",
		`/api/v1/hotspots/sector-fund-flow-intraday`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected hotspots page to include %q, got %s", expected, body)
		}
	}
	for _, unexpected := range []string{
		`<h2>热点切换</h2>`,
		`<h2>今日热点排行</h2>`,
		`<h2>14日热点排行</h2>`,
		`<h2>升温热点</h2>`,
		`<h2>发现词</h2>`,
		`<h2>降温热点</h2>`,
		`<h2>新增热点</h2>`,
		`<h2>连续升温热点</h2>`,
		`<h2>发现词升温</h2>`,
		`<h2>近 14 日趋势</h2>`,
		`过去 14 天热点`,
		`最近14天`,
		`class="hotspot-table"`,
		`class="hotspot-grid"`,
		`class="hotspot-kpis"`,
		`class="hotspot-kpi"`,
		`统计文章<strong>`,
		`标准热点<strong>`,
		`发现词<strong>`,
		`切换信号<strong>`,
		`.fundflow-section{width:104.5vw`,
		`[-1,-0.5,0,0.5,1].forEach`,
	} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("expected hotspots page to omit %q, got %s", unexpected, body)
		}
	}
}

func TestHotspotsPageRefreshKeepsIndustrySectorType(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/a-stock/sector-fund-flow-intraday" {
			if r.URL.Query().Get("sector_type") != "行业资金流" || r.URL.Query().Get("indicator") != "今日" {
				t.Fatalf("unexpected intraday query %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockSectorFundFlowIntradayResult{
					Date:       "2026-07-14",
					SectorType: "行业资金流",
					Indicator:  "今日",
					LatestTime: "13:11",
					Times:      []string{"13:11"},
					Top:        []model.AStockSectorFundFlow{{TradeDate: "2026-07-14", SectorType: "行业资金流", Indicator: "今日", Rank: 1, Name: "半导体", MainNetInflow: 8005000000}},
				},
			})
			return
		}
		if r.URL.Path != "/api/v1/hotspots/switching" || r.URL.Query().Get("days") != "14" {
			t.Fatalf("unexpected content request %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data":    model.HotspotSwitchingResult{Days: 14},
		})
	}))
	defer content.Close()
	srv := NewServer(config.Config{ContentURL: content.URL})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hotspots?days=14&sector_type=%E8%A1%8C%E4%B8%9A%E8%B5%84%E9%87%91%E6%B5%81", nil)
	srv.handleHotspots(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected hotspots page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, expected := range []string{
		`data-sector-type="行业资金流"`,
		`var currentType = "行业资金流"`,
		`getAttribute("data-sector-type")`,
		`data&&data.sector_type&&data.sector_type!==requestedType`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected hotspots page to include %q, got %s", expected, body)
		}
	}
}

func TestHotspotSectorFundFlowIntradayClearsYesterdayValuesBeforeOpen(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	result := model.AStockSectorFundFlowIntradayResult{
		Date:       "2026-07-16",
		SectorType: "行业资金流",
		Indicator:  "今日",
		LatestTime: "15:00",
		Times:      []string{"09:30", "15:00"},
		Series: []model.AStockSectorFundFlowIntradaySeries{{
			Name:                "机器人",
			LatestRank:          1,
			LatestMainNetInflow: 3273000000,
			Points:              []model.AStockSectorFundFlowIntradayPoint{{Time: "15:00", MainNetInflow: 3273000000, Rank: 1}},
		}},
		Top: []model.AStockSectorFundFlow{{
			TradeDate:              "2026-07-16",
			SectorType:             "行业资金流",
			Indicator:              "今日",
			Rank:                   1,
			Name:                   "机器人",
			MainNetInflow:          3273000000,
			MainNetInflowPct:       9.1,
			SuperLargeNetInflow:    100,
			SuperLargeNetInflowPct: 1,
			LargeNetInflow:         200,
			LargeNetInflowPct:      2,
			MediumNetInflow:        300,
			MediumNetInflowPct:     3,
			SmallNetInflow:         400,
			SmallNetInflowPct:      4,
		}},
	}
	cleared := clearPreOpenHotspotSectorFundFlowIntraday(result, time.Date(2026, 7, 17, 9, 1, 0, 0, loc))
	if cleared.Date != "2026-07-17" || cleared.LatestTime != "" || len(cleared.Times) != 0 || len(cleared.Series) != 0 {
		t.Fatalf("expected stale intraday chart values to be cleared, got %+v", cleared)
	}
	if len(cleared.Top) != 1 || cleared.Top[0].Name != "机器人" || cleared.Top[0].MainNetInflow != 0 || cleared.Top[0].LargeNetInflow != 0 || cleared.Top[0].SmallNetInflowPct != 0 {
		t.Fatalf("expected ranking rows to remain with zeroed values, got %+v", cleared.Top)
	}
}

func TestHotspotSectorFundFlowIntradayKeepsValuesAtOpen(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	result := model.AStockSectorFundFlowIntradayResult{
		Date:       "2026-07-16",
		LatestTime: "15:00",
		Times:      []string{"15:00"},
		Top:        []model.AStockSectorFundFlow{{Name: "机器人", MainNetInflow: 3273000000}},
	}
	kept := clearPreOpenHotspotSectorFundFlowIntraday(result, time.Date(2026, 7, 17, 9, 30, 0, 0, loc))
	if kept.LatestTime != "15:00" || len(kept.Times) != 1 || kept.Top[0].MainNetInflow != 3273000000 {
		t.Fatalf("expected intraday values to be kept at 09:30, got %+v", kept)
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
	serviceTab := `href="/crawl-templates/manage">模板管理</a><a class="{{if eq .SectionKey "services"}}active{{end}}" href="/system?section=services">服务状态</a><a class="{{if eq .SectionKey "legacy"}}active{{end}}" href="/system?section=legacy">Legacy注册表`
	newsStatsTab := `href="/system?section=operations">生产运行</a><a class="{{if eq .SectionKey "newsstats"}}active{{end}}" href="/system?section=newsstats">新闻统计`
	opActionsTab := `href="/system?section=newsstats">新闻统计</a><a class="{{if eq .SectionKey "opactions"}}active{{end}}" href="/system?section=opactions">运营操作`
	contractsTab := `href="/system?section=opactions">运营操作</a><a class="{{if eq .SectionKey "contracts"}}active{{end}}" href="/system?section=contracts">外部契约与审计`
	announcementTab := `href="/system?section=contracts">外部契约与审计</a><a class="{{if eq .SectionKey "announcements"}}active{{end}}" href="/system?section=announcements">公告与任务`
	for _, expected := range []string{
		serviceTab,
		newsStatsTab,
		opActionsTab,
		contractsTab,
		announcementTab,
		`{{if eq .SectionKey "services"}}<section><h2>服务状态</h2>`,
		`{{if eq .SectionKey "legacy"}}<section class="section-block"><h2>Legacy 注册表</h2>`,
		`name="section" value="services"`,
		`{{if eq .SectionKey "operations"}}<section class="section-block"><h2>生产运行</h2>`,
		`{{if eq .SectionKey "newsstats"}}<section class="section-block astock-newsstats-section">`,
		`name="section" value="newsstats"`,
		`{{.AStockNewsStatsHTML}}`,
		`{{if eq .SectionKey "opactions"}}<section class="section-block"><h2>运营操作</h2>`,
		`name="section" value="opactions"`,
		`<h3>历史文章清理</h3>`,
		`{{.ArticleCleanupPreview.Total}}`,
		`name="form_type" value="article_cleanup_preview"`,
		`name="form_type" value="article_cleanup_soft"`,
		`name="form_type" value="article_cleanup_hard"`,
		`{{if eq .SectionKey "contracts"}}<section class="section-block"><h2>外部契约与审计</h2>`,
		`{{if eq .SectionKey "announcements"}}<section class="section-block"><h2>公告与任务</h2>`,
		`<h3>任务记录</h3><table><tr><th>任务</th><th>状态</th><th>说明</th><th>开始时间</th></tr>`,
		`href="/system?section=release">软件发布</a>`,
		`{{if eq .SectionKey "release"}}<section class="section-block"><h2>软件发布</h2>`,
		`name="form_type" value="release"`,
		`.feedback-textarea{min-height:168px;resize:vertical}`,
		`<textarea class="feedback-textarea" name="content" placeholder="问题描述或需求"></textarea>`,
		`href="/system?section=feedbacklist">建议列表</a>`,
		`{{if eq .SectionKey "feedbacklist"}}<section class="section-block"><h2>建议列表</h2>`,
		`{{range .FeedbackItems}}`,
		`class="feedback-delete-form"`,
		`name="form_type" value="delete_feedback"`,
		`class="feedback-delete-button">删除</button>`,
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
	if strings.Contains(systemTemplate, `<div><h3>公告</h3><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}`) {
		t.Fatal("expected announcements section to omit notice table")
	}
}

func TestSystemFeedbackSectionOnlyRendersSubmitForm(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/system?section=feedback", nil)
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected system feedback page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"反馈建议", "建议列表", "name=\"form_type\" value=\"feedback\"", "问题描述或需求"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected feedback page to contain %q, got %s", want, body)
		}
	}
	for _, unwanted := range []string{"右侧显示建议标题", "右侧显示建议内容", "name=\"form_type\" value=\"delete_feedback\""} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("expected feedback page to exclude %q after moving suggestion list, got %s", unwanted, body)
		}
	}
}

func TestSystemFeedbackListSectionRendersSubmittedFeedbackList(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/system?section=feedbacklist", nil)
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected feedback list page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"建议列表", "右侧显示建议标题", "右侧显示建议内容", "用户 1", "删除"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected feedback list page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "name=\"form_type\" value=\"feedback\"") {
		t.Fatalf("expected feedback list page to exclude submit form, got %s", body)
	}
}

func TestSystemFeedbackDeleteRemovesSubmittedSuggestion(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	form := url.Values{
		"form_type":   {"delete_feedback"},
		"section":     {"feedbacklist"},
		"feedback_id": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/system?section=feedbacklist", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected delete redirect 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "msg=") || !strings.Contains(location, "section=feedbacklist") {
		t.Fatalf("expected redirect with feedback list section and message, got %q", location)
	}

	viewReq := httptest.NewRequest(http.MethodGet, "/system?section=feedbacklist", nil)
	viewRR := httptest.NewRecorder()
	srv.handleSystem(viewRR, viewReq, map[string]any{"id": int64(1), "username": "admin"})
	if viewRR.Code != http.StatusOK {
		t.Fatalf("expected feedback list page 200 after delete, got %d body=%s", viewRR.Code, viewRR.Body.String())
	}
	body := viewRR.Body.String()
	if strings.Contains(body, "右侧显示建议标题") {
		t.Fatalf("expected deleted feedback to disappear, got %s", body)
	}
	if !strings.Contains(body, "保留的建议标题") {
		t.Fatalf("expected remaining feedback to stay visible, got %s", body)
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
	if loginBakRR.Code != http.StatusOK || !strings.Contains(loginBakRR.Body.String(), "简苏舆情") {
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
	for _, snippet := range []string{"数据库配置", "数据库连接检测与切换", "已选择驱动", "切换到 PostgreSQL", "切回 SQLite", "database_save_postgres", "database_switch_postgres", "database_switch_sqlite", "YUQING_DB_DRIVER", "postgres_dsn"} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected database section to contain %q, got %s", snippet, body)
		}
	}
}

func TestSystemReleaseSectionRendersPublishSettings(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/system?section=release", nil)
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected system release page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, snippet := range []string{"软件发布", "YUQING_RELEASE_ADDR", "YUQING_RELEASE_DIR", "YUQING_RELEASE_URL", `C:\yuqing\release`, `D:\yuqing\release`, `\\10.15.0.7\yuqing-release`, "robocopy"} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected release section to contain %q, got %s", snippet, body)
		}
	}
}

func TestSystemReleaseSaveRedirectsWithSuccess(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	form := url.Values{}
	form.Set("section", "release")
	form.Set("form_type", "release")
	form.Set("release_addr", ":8100")
	form.Set("release_url", "http://10.15.0.7:8100")
	form.Set("release_dir", `C:\yuqing\release2`)
	form.Set("dev_release_dir", `D:\yuqing\release2`)
	form.Set("server_share_path", `\\10.15.0.7\yuqing-release2`)
	form.Set("server_user", `10.15.0.7\hyuser`)
	req := httptest.NewRequest(http.MethodPost, "/system?section=release", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after release config save, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "section=release") || !strings.Contains(location, url.QueryEscape("软件发布配置已保存")) {
		t.Fatalf("expected release save success redirect, got %q", location)
	}
}

func TestSystemDatabaseSaveConfigRedirectsWithSuccess(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	form := url.Values{}
	form.Set("section", "database")
	form.Set("form_type", "database_save_config")
	form.Set("driver", "postgres")
	form.Set("sqlite_path", "data/yuqing.db")
	form.Set("postgres_host", "127.0.0.1")
	form.Set("postgres_port", "5432")
	form.Set("postgres_database", "yuqing")
	form.Set("postgres_user", "postgres")
	form.Set("postgres_sslmode", "disable")
	req := httptest.NewRequest(http.MethodPost, "/system?section=database", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after database config save, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "section=database") || !strings.Contains(location, url.QueryEscape("数据库连接参数配置已保存")) {
		t.Fatalf("expected database save success redirect, got %q", location)
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

	form.Set("form_type", "database_save_postgres")
	form.Set("driver", "sqlite")
	req = httptest.NewRequest(http.MethodPost, "/system?section=database", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after postgres config save and switch, got %d body=%s", rr.Code, rr.Body.String())
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
	releaseSettings := model.ReleaseSettings{
		ReleaseAddr:     ":8099",
		ReleaseURL:      "http://10.15.0.7:8099",
		ReleaseDir:      `C:\yuqing\release`,
		DevReleaseDir:   `D:\yuqing\release`,
		ServerSharePath: `\\10.15.0.7\yuqing-release`,
		ServerUser:      `10.15.0.7\hyuser`,
		UpdatedAt:       time.Now().UTC(),
	}
	popupStates := map[string]model.PopupState{}
	deletedArticles := map[int64]bool{}
	emotions := map[int64]string{}
	shareChannels := map[int64][]string{}
	opinionConditions := map[int64]model.OpinionCondition{}
	warningSettings := map[int64]model.WarningSetting{}
	createdUsers := map[string]model.User{}
	auditLogs := []model.AuditLog{}
	feedbackItems := []model.Feedback{
		{ID: 1, UserID: 1, Title: "右侧显示建议标题", Content: "右侧显示建议内容", CreatedAt: time.Now().UTC()},
		{ID: 2, UserID: 2, Title: "保留的建议标题", Content: "保留的建议内容", CreatedAt: time.Now().UTC().Add(-time.Hour)},
	}
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/release-settings":
			mu.Lock()
			settings := releaseSettings
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", settings)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/release-settings":
			var settings model.ReleaseSettings
			if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			settings.UpdatedAt = time.Now().UTC()
			mu.Lock()
			releaseSettings = settings
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", settings)
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
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/database-config/save":
			writeEnvelope(http.StatusOK, "database connection config saved", model.DatabaseConfigStatus{
				Driver:           "postgres",
				ConfiguredDriver: "postgres",
				RuntimeDriver:    "sqlite",
				Status:           "ok",
				Message:          "database connection config saved",
				ConfigPath:       "data/database-config.json",
				RestartRequired:  true,
			})
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/feedback":
			mu.Lock()
			items := append([]model.Feedback(nil), feedbackItems...)
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/feedback":
			var item model.Feedback
			_ = json.NewDecoder(r.Body).Decode(&item)
			item.ID = int64(len(feedbackItems) + 1)
			item.CreatedAt = time.Now().UTC()
			mu.Lock()
			feedbackItems = append([]model.Feedback{item}, feedbackItems...)
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/system/feedback/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/feedback/"))
			mu.Lock()
			filtered := make([]model.Feedback, 0, len(feedbackItems))
			for _, item := range feedbackItems {
				if item.ID == id {
					continue
				}
				filtered = append(filtered, item)
			}
			feedbackItems = filtered
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]any{"deleted": true, "id": id})
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
