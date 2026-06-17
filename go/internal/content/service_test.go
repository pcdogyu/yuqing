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
	if envelope.Data.TotalAmount != 5876080 || envelope.Data.MaxItem == nil || envelope.Data.MaxItem.Code != "002230" {
		t.Fatalf("expected date summary independent of keyword filter, got %+v", envelope.Data)
	}
	if len(envelope.Data.Trend) != 1 || envelope.Data.Trend[0].TotalAmount != 5876080 || envelope.Data.Trend[0].TotalVolume != 213400 {
		t.Fatalf("expected auction trend totals, got %+v", envelope.Data.Trend)
	}
}

func TestStockResearchAPIUpsertsAndLists(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	payload := `{"items":[{"code":"002230","name":"科大讯飞","kind":"report","title":"科大讯飞深度研究","institution":"中金公司","analyst":"张三","rating":"买入","target_price":"50.00","research_date":"2026-06-16","source_type":"sina_finance_report","source_key":"sina-1","source_url":"https://sina.example.com/1"},{"code":"300059","name":"东方财富","kind":"survey","title":"东方财富机构调研","institution":"华泰证券","research_date":"2026-06-15","source_type":"sohu_finance_report","source_key":"sohu-1"}]}`
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

	textReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stock-research/%d/pdf/text", itemID), nil)
	textRR := httptest.NewRecorder()
	router.ServeHTTP(textRR, textReq)
	if textRR.Code != http.StatusOK || !strings.Contains(textRR.Body.String(), "科大讯飞研报正文") {
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
		case "/healthz", "/api/v1/nlp/capabilities":
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
	if len(opsEnvelope.Data.Services) != 8 || len(opsEnvelope.Data.FailedTaskRuns) != 2 {
		t.Fatalf("unexpected operations summary: %+v", opsEnvelope.Data)
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
