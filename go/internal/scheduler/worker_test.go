package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

func TestNewWorkerSetsTokenHeader(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:  3 * time.Second,
		ServiceToken: "secret-token",
	})

	if got := worker.client.Header.Get("X-Service-Token"); got != "secret-token" {
		t.Fatalf("expected service token header, got %q", got)
	}
	if got := worker.crawlClient.Header.Get("X-Service-Token"); got != "secret-token" {
		t.Fatalf("expected crawl service token header, got %q", got)
	}
}

func TestRunCrawlUsesDedicatedTimeout(t *testing.T) {
	var sawToken atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/tasks/crawl" || r.URL.Query().Get("source_type") != provider.SourceTypeHeadline {
			t.Fatalf("unexpected crawl request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("X-Service-Token") == "secret-token" {
			sawToken.Store(true)
		}
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	worker := NewWorker(config.Config{
		CrawlerURL:            server.URL,
		HTTPTimeout:           10 * time.Millisecond,
		SchedulerCrawlTimeout: 250 * time.Millisecond,
		ServiceToken:          "secret-token",
		ExternalRetryWait:     time.Millisecond,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})

	if err := worker.runCrawl(context.Background(), provider.SourceTypeHeadline); err != nil {
		t.Fatalf("expected crawl request to use dedicated timeout, got %v", err)
	}
	if !sawToken.Load() {
		t.Fatal("expected crawl request to include service token")
	}
}

func TestGenerateAStockRecommendationSnapshotUsesDedicatedTimeout(t *testing.T) {
	var sawToken atomic.Bool
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/a-stock/recommendations/generate" {
			t.Fatalf("unexpected gateway request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("date") != "2026-06-16" || r.URL.Query().Get("period") != "morning" || r.URL.Query().Get("phase") != "final" {
			t.Fatalf("unexpected generate query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Service-Token") == "secret-token" {
			sawToken.Store(true)
		}
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           10 * time.Millisecond,
		SchedulerCrawlTimeout: 250 * time.Millisecond,
		ServiceToken:          "secret-token",
		ExternalRetryWait:     time.Millisecond,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})

	if err := worker.generateAStockRecommendationSnapshot(context.Background(), "2026-06-16", "morning", "final"); err != nil {
		t.Fatalf("expected A股 recommendation generation to use dedicated timeout, got %v", err)
	}
	if !sawToken.Load() {
		t.Fatal("expected generate request to include service token")
	}
}

func TestRunCrawlIncludesCrawlerErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/tasks/crawl" || r.URL.Query().Get("source_type") != "foresight_newsflash" {
			t.Fatalf("unexpected crawl request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":502,"message":"foresight_newsflash fetch failed: 567 ","data":null}`))
	}))
	defer server.Close()

	worker := NewWorker(config.Config{
		CrawlerURL:            server.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
		ExternalRetryWait:     time.Millisecond,
	})

	err := worker.runCrawl(context.Background(), "foresight_newsflash")
	if err == nil || !strings.Contains(err.Error(), "foresight_newsflash fetch failed: 567") {
		t.Fatalf("expected crawler response message in error, got %v", err)
	}
}

func TestRunCrawlWithOptionsPassesWindowQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/tasks/crawl" || r.URL.Query().Get("source_type") != provider.SourceTypeFlash {
			t.Fatalf("unexpected crawl request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" ||
			r.URL.Query().Get("end") != "2026-06-16 09:26:59" ||
			r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected crawl options query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	worker := NewWorker(config.Config{
		CrawlerURL:            server.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ExternalRetryWait:     time.Millisecond,
	})

	err := worker.runCrawlWithOptions(context.Background(), provider.SourceTypeFlash, model.CrawlOptions{
		Start:     "2026-06-16 08:00:00",
		End:       "2026-06-16 09:26:59",
		TimeField: "publish_time",
	})
	if err != nil {
		t.Fatalf("runCrawlWithOptions error: %v", err)
	}
}

func TestLoopRunsImmediatelyAndStopsOnCancel(t *testing.T) {
	worker := NewWorker(config.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count atomic.Int32
	done := make(chan struct{})
	go func() {
		worker.loop(ctx, "test-task", 10*time.Millisecond, func() error {
			if count.Add(1) >= 2 {
				cancel()
			}
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("scheduler loop did not stop after context cancellation")
	}

	if count.Load() < 1 {
		t.Fatalf("expected loop to run at least once, got %d", count.Load())
	}
}

func TestWaitForHealthyReturnsWhenServiceIsReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	worker := NewWorker(config.Config{HTTPTimeout: time.Second})
	if err := worker.waitForHealthy(context.Background(), "test-service", server.URL, time.Second); err != nil {
		t.Fatalf("expected readiness success, got %v", err)
	}
}

func TestWaitForHealthyTimesOutForUnavailableService(t *testing.T) {
	worker := NewWorker(config.Config{HTTPTimeout: 50 * time.Millisecond})
	err := worker.waitForHealthy(context.Background(), "missing-service", "http://127.0.0.1:1/healthz", 150*time.Millisecond)
	if err == nil {
		t.Fatal("expected readiness timeout error")
	}
}

func TestSchedulerJobsAPIListsAndRunsJob(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "scheduler-api.db")
	store, err := sqlitestore.New(dbPath)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	defer func() { _ = store.Close() }()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/api/v1/search/hot-keywords":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": []model.SearchWordStat{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		DatabasePath:          dbPath,
		ContentURL:            content.URL,
		HTTPTimeout:           time.Second,
		ServiceToken:          "secret-token",
		FlashInterval:         time.Hour,
		HeadlineInterval:      time.Hour,
		AnalysisInterval:      time.Hour,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})
	defer func() { _ = worker.Close() }()

	router := worker.Router()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/jobs", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), "hot-data-refresh") {
		t.Fatalf("expected scheduler jobs list, got status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var listEnvelope struct {
		Data []Job `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("unmarshal jobs list: %v", err)
	}
	if len(listEnvelope.Data) != 41 {
		t.Fatalf("expected 41 scheduler jobs, got %d", len(listEnvelope.Data))
	}
	var heartbeatJob, hotJob, eastmoneyJob, jin10FullJob, wallStreetCNJob, clsJob, sinaJob, cryptoXJob, cryptoTelegramJob, foresightJob, coindeskJob, panewsJob, theBlockJob, aStockMorningNewsCrawlJob, aStockMorningPreviewJob, aStockMorningJob, aStockAfternoonPreviewJob, aStockMiddayNewsCrawlJob, aStockAfternoonJob, aStockAfternoonOpenRefreshJob, aStockDailyBacktestRefreshJob, aStockAuctionJob, aStockSectorFundFlowJob, aStockHoldingsJob, stockResearchJob, investorRelationsJob Job
	for _, job := range listEnvelope.Data {
		switch job.Name {
		case "crawl-link-heartbeat":
			heartbeatJob = job
		case "hot-data-refresh":
			hotJob = job
		case "eastmoney-kuaixun-crawl":
			eastmoneyJob = job
		case "jin10-full-crawl":
			jin10FullJob = job
		case "wallstreetcn-a-stock-crawl":
			wallStreetCNJob = job
		case "cls-telegraph-crawl":
			clsJob = job
		case "sina-finance-7x24-crawl":
			sinaJob = job
		case "crypto-x-crawl":
			cryptoXJob = job
		case "crypto-telegram-crawl":
			cryptoTelegramJob = job
		case "foresight-newsflash-crawl":
			foresightJob = job
		case "coindesk-zh-latest-crawl":
			coindeskJob = job
		case "panews-newsflash-crawl":
			panewsJob = job
		case "theblock-latest-crawl":
			theBlockJob = job
		case "a-stock-morning-news-crawl":
			aStockMorningNewsCrawlJob = job
		case "a-stock-morning-recommendation":
			aStockMorningJob = job
		case "a-stock-morning-recommendation-preview":
			aStockMorningPreviewJob = job
		case "a-stock-afternoon-recommendation-preview":
			aStockAfternoonPreviewJob = job
		case "a-stock-midday-news-crawl":
			aStockMiddayNewsCrawlJob = job
		case "a-stock-afternoon-recommendation":
			aStockAfternoonJob = job
		case "a-stock-afternoon-open-refresh":
			aStockAfternoonOpenRefreshJob = job
		case "a-stock-daily-backtest-refresh":
			aStockDailyBacktestRefreshJob = job
		case "a-stock-auction-crawl":
			aStockAuctionJob = job
		case "a-stock-sector-fund-flow-crawl":
			aStockSectorFundFlowJob = job
		case "a-stock-holdings-crawl":
			aStockHoldingsJob = job
		case "stock-research-crawl":
			stockResearchJob = job
		case "investor-relations-crawl":
			investorRelationsJob = job
		}
	}
	if hotJob.JavaQuartzName != "HotDataSchedule" || hotJob.Cron == "" || hotJob.NextRunAt == nil {
		t.Fatalf("expected hot job runtime metadata, got %+v", hotJob)
	}
	if heartbeatJob.JavaQuartzName != "CrawlLinkHeartbeat" || heartbeatJob.Cron != "0 0/5 * * * ?" || heartbeatJob.IntervalSec != 300 || heartbeatJob.NextRunAt == nil {
		t.Fatalf("expected crawl link heartbeat metadata, got %+v", heartbeatJob)
	}
	if eastmoneyJob.JavaQuartzName != "EastMoneyKuaixunCrawler" || eastmoneyJob.Cron != "0 0/5 * * * ?" || eastmoneyJob.IntervalSec != 300 || eastmoneyJob.Enabled {
		t.Fatalf("expected eastmoney realtime crawl listed but disabled without URL, got %+v", eastmoneyJob)
	}
	if jin10FullJob.JavaQuartzName != "Jin10FullCrawler" || jin10FullJob.Cron != "0 0/5 * * * ?" || jin10FullJob.IntervalSec != 300 || jin10FullJob.Enabled {
		t.Fatalf("expected jin10 full crawl listed but disabled by default, got %+v", jin10FullJob)
	}
	if wallStreetCNJob.JavaQuartzName != "WallStreetCNAStockCrawler" || wallStreetCNJob.Cron != "0 0/5 * * * ?" || wallStreetCNJob.IntervalSec != 300 || wallStreetCNJob.Enabled {
		t.Fatalf("expected wallstreetcn A股 crawl listed but disabled without URL, got %+v", wallStreetCNJob)
	}
	if clsJob.JavaQuartzName != "CLSTelegraphCrawler" || clsJob.Cron != "0 0/5 * * * ?" || clsJob.IntervalSec != 300 || clsJob.Enabled {
		t.Fatalf("expected cls telegraph crawl listed but disabled without URL, got %+v", clsJob)
	}
	if sinaJob.JavaQuartzName != "SinaFinance7x24Crawler" || sinaJob.Cron != "0 0/5 * * * ?" || sinaJob.IntervalSec != 300 || sinaJob.Enabled {
		t.Fatalf("expected sina finance 7x24 crawl listed but disabled without URL, got %+v", sinaJob)
	}
	if cryptoXJob.Name == "" || cryptoTelegramJob.Name == "" {
		t.Fatalf("expected crypto scheduler jobs, got %+v", listEnvelope.Data)
	}
	if foresightJob.Name == "" || coindeskJob.Name == "" {
		t.Fatalf("expected crypto news scheduler jobs, got %+v", listEnvelope.Data)
	}
	if panewsJob.Name == "" {
		t.Fatalf("expected panews scheduler job, got %+v", listEnvelope.Data)
	}
	if theBlockJob.Name == "" {
		t.Fatalf("expected theblock scheduler job, got %+v", listEnvelope.Data)
	}
	if aStockMorningNewsCrawlJob.Cron != "0 5 9 * * ?" || aStockMorningNewsCrawlJob.NextRunAt == nil {
		t.Fatalf("expected A股 morning news crawl cron metadata, got %+v", aStockMorningNewsCrawlJob)
	}
	if aStockMorningJob.Cron != "0 32 9 * * ?" || aStockMorningJob.NextRunAt == nil {
		t.Fatalf("expected A股 morning recommendation cron metadata, got %+v", aStockMorningJob)
	}
	if aStockMorningPreviewJob.Cron != "0 27 9 * * ?" || aStockMorningPreviewJob.NextRunAt == nil {
		t.Fatalf("expected A股 morning recommendation preview cron metadata, got %+v", aStockMorningPreviewJob)
	}
	if aStockAfternoonPreviewJob.Cron != "0 57 12 * * ?" || aStockAfternoonPreviewJob.NextRunAt == nil {
		t.Fatalf("expected A股 afternoon recommendation preview cron metadata, got %+v", aStockAfternoonPreviewJob)
	}
	if aStockMiddayNewsCrawlJob.Cron != "0 30 12 * * ?" || aStockMiddayNewsCrawlJob.NextRunAt == nil {
		t.Fatalf("expected A股 midday news crawl cron metadata, got %+v", aStockMiddayNewsCrawlJob)
	}
	if aStockAfternoonJob.Cron != "0 2 13 * * ?" || aStockAfternoonJob.NextRunAt == nil {
		t.Fatalf("expected A股 afternoon recommendation cron metadata, got %+v", aStockAfternoonJob)
	}
	if aStockAfternoonOpenRefreshJob.Cron != "0 5 13 * * ?" || aStockAfternoonOpenRefreshJob.NextRunAt == nil {
		t.Fatalf("expected A股 afternoon open refresh cron metadata, got %+v", aStockAfternoonOpenRefreshJob)
	}
	if aStockDailyBacktestRefreshJob.Cron != "0 5 15 * * ?" || aStockDailyBacktestRefreshJob.NextRunAt == nil {
		t.Fatalf("expected A股 daily backtest refresh cron metadata, got %+v", aStockDailyBacktestRefreshJob)
	}
	if aStockAuctionJob.Cron != "0 26 9 * * ?" || aStockAuctionJob.Enabled {
		t.Fatalf("expected A股 auction crawl disabled by default with 09:26 cron, got %+v", aStockAuctionJob)
	}
	if aStockSectorFundFlowJob.Cron != "0 0/5 9-15 * * ?" || aStockSectorFundFlowJob.Enabled {
		t.Fatalf("expected A股 sector fund flow crawl disabled by default with 5 minute trading cron, got %+v", aStockSectorFundFlowJob)
	}
	if aStockHoldingsJob.Cron != "0 35 2 * * ?" || aStockHoldingsJob.Enabled {
		t.Fatalf("expected A股 holdings crawl disabled by default with 02:35 cron, got %+v", aStockHoldingsJob)
	}
	if stockResearchJob.Cron != "0 30 16 * * ?" || stockResearchJob.Enabled {
		t.Fatalf("expected stock research crawl disabled by default in test config, got %+v", stockResearchJob)
	}
	if investorRelationsJob.Cron != "0 45 16 * * ?" || investorRelationsJob.Enabled {
		t.Fatalf("expected investor relations crawl disabled by default in test config, got %+v", investorRelationsJob)
	}
	if cryptoXJob.Enabled || cryptoTelegramJob.Enabled || foresightJob.Enabled || coindeskJob.Enabled || panewsJob.Enabled || theBlockJob.Enabled {
		t.Fatalf("expected crypto jobs disabled without endpoint urls, got x=%+v telegram=%+v foresight=%+v coindesk=%+v panews=%+v theblock=%+v", cryptoXJob, cryptoTelegramJob, foresightJob, coindeskJob, panewsJob, theBlockJob)
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/jobs/hot-data-refresh/run", nil)
	runReq.Header.Set("X-Service-Token", "secret-token")
	runRR := httptest.NewRecorder()
	router.ServeHTTP(runRR, runReq)
	if runRR.Code != http.StatusOK {
		t.Fatalf("expected manual job run 200, got %d body=%s", runRR.Code, runRR.Body.String())
	}

	runs, err := store.ListTaskRuns(ctx, 5)
	if err != nil {
		t.Fatalf("ListTaskRuns error: %v", err)
	}
	if len(runs) == 0 || runs[0].TaskName != "hot-data-refresh" || runs[0].Status != "success" {
		t.Fatalf("expected successful hot-data task run, got %+v", runs)
	}

	listAfterReq := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/jobs", nil)
	listAfterRR := httptest.NewRecorder()
	router.ServeHTTP(listAfterRR, listAfterReq)
	var listAfterEnvelope struct {
		Data []Job `json:"data"`
	}
	if err := json.Unmarshal(listAfterRR.Body.Bytes(), &listAfterEnvelope); err != nil {
		t.Fatalf("unmarshal jobs list after run: %v", err)
	}
	for _, job := range listAfterEnvelope.Data {
		if job.Name == "hot-data-refresh" && (job.LastStatus != "success" || job.LastStartedAt == nil || job.LastFinishedAt == nil) {
			t.Fatalf("expected last run metadata after manual trigger, got %+v", job)
		}
	}
}

func TestRunAStockRecommendationCrawlsSourcesAndQueriesWindow(t *testing.T) {
	var sources []string
	var generatedPeriods []string
	var generatedPhases []string
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" || r.URL.Query().Get("date") != "2026-06-16" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":           "2026-06-16",
			"is_trading_day": true,
			"source":         "test",
			"reason":         "trading_day",
			"message":        "open",
		})
	}))
	defer akshare.Close()

	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.URL.Query().Get("start") != "2026-06-16 09:30:00" ||
			r.URL.Query().Get("end") != "2026-06-16 13:00:59" ||
			r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 afternoon crawl window query: %s", r.URL.RawQuery)
		}
		sources = append(sources, r.URL.Query().Get("source_type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16T01:30:00Z" || r.URL.Query().Get("end") != "2026-06-16T05:00:59Z" {
			t.Fatalf("unexpected A股 afternoon window query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/a-stock/recommendations/generate" {
			t.Fatalf("unexpected gateway request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected gateway service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		generatedPeriods = append(generatedPeriods, r.URL.Query().Get("period"))
		generatedPhases = append(generatedPhases, r.URL.Query().Get("phase"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	if err := worker.runAStockRecommendationForDate(context.Background(), "2026-06-16", "afternoon", "final"); err != nil {
		t.Fatalf("runAStockRecommendationForDate error: %v", err)
	}
	sort.Strings(sources)
	if strings.Join(sources, ",") != "cls_telegraph,eastmoney_kuaixun,jin10_kuaixun,jin10_资讯,sina_finance_7x24,wallstreetcn_a_stock" {
		t.Fatalf("expected all A股 sources to be crawled, got %v", sources)
	}
	if len(generatedPeriods) != 1 || generatedPeriods[0] != "afternoon" || len(generatedPhases) != 1 || generatedPhases[0] != "final" {
		t.Fatalf("expected afternoon final recommendation snapshot generation, got periods=%v phases=%v", generatedPeriods, generatedPhases)
	}
}

func TestRunAStockRecommendationGeneratesMorningSnapshot(t *testing.T) {
	var generatedPeriods []string
	var generatedPhases []string
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":           "2026-06-16",
			"is_trading_day": true,
			"source":         "test",
			"reason":         "trading_day",
			"message":        "open",
		})
	}))
	defer akshare.Close()

	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" ||
			r.URL.Query().Get("end") != "2026-06-16 09:26:59" ||
			r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 morning crawl window query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16T00:00:00Z" || r.URL.Query().Get("end") != "2026-06-16T01:26:59Z" {
			t.Fatalf("unexpected A股 morning window query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/a-stock/recommendations/generate" {
			t.Fatalf("unexpected gateway request: %s %s", r.Method, r.URL.String())
		}
		generatedPeriods = append(generatedPeriods, r.URL.Query().Get("period"))
		generatedPhases = append(generatedPhases, r.URL.Query().Get("phase"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
	})

	if err := worker.runAStockRecommendationForDate(context.Background(), "2026-06-16", "morning", "final"); err != nil {
		t.Fatalf("runAStockRecommendationForDate morning error: %v", err)
	}
	if len(generatedPeriods) != 1 || generatedPeriods[0] != "morning" || len(generatedPhases) != 1 || generatedPhases[0] != "final" {
		t.Fatalf("expected morning final recommendation snapshot generation, got periods=%v phases=%v", generatedPeriods, generatedPhases)
	}
}

func TestRunAStockWindowNewsCrawlForDateCrawlsMorningSourcesOnly(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" || r.URL.Query().Get("date") != "2026-06-16" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":           "2026-06-16",
			"is_trading_day": true,
			"source":         "test",
			"reason":         "trading_day",
			"message":        "open",
		})
	}))
	defer akshare.Close()

	var sources []string
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if r.URL.Query().Get("start") != "2026-06-16 08:00:00" ||
			r.URL.Query().Get("end") != "2026-06-16 09:26:59" ||
			r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 morning news crawl window query: %s", r.URL.RawQuery)
		}
		sources = append(sources, r.URL.Query().Get("source_type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	if err := worker.runAStockWindowNewsCrawlForDate(context.Background(), "2026-06-16", "morning", "preopen"); err != nil {
		t.Fatalf("runAStockWindowNewsCrawlForDate error: %v", err)
	}
	sort.Strings(sources)
	if strings.Join(sources, ",") != "cls_telegraph,eastmoney_kuaixun,jin10_kuaixun,jin10_资讯,sina_finance_7x24,wallstreetcn_a_stock" {
		t.Fatalf("expected all enabled A股 news sources to be crawled, got %v", sources)
	}
}

func TestRunAStockWindowNewsCrawlForDateCrawlsMiddaySources(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" || r.URL.Query().Get("date") != "2026-06-16" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":           "2026-06-16",
			"is_trading_day": true,
			"source":         "test",
			"reason":         "trading_day",
			"message":        "open",
		})
	}))
	defer akshare.Close()

	var sources []string
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16 09:30:00" ||
			r.URL.Query().Get("end") != "2026-06-16 13:00:59" ||
			r.URL.Query().Get("time_field") != "publish_time" {
			t.Fatalf("unexpected A股 midday news crawl window query: %s", r.URL.RawQuery)
		}
		sources = append(sources, r.URL.Query().Get("source_type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		Jin10FullEnabled:      true,
	})

	if err := worker.runAStockWindowNewsCrawlForDate(context.Background(), "2026-06-16", "afternoon", "final"); err != nil {
		t.Fatalf("runAStockWindowNewsCrawlForDate midday error: %v", err)
	}
	sort.Strings(sources)
	if strings.Join(sources, ",") != "cls_telegraph,eastmoney_kuaixun,jin10_full,jin10_kuaixun,jin10_资讯,sina_finance_7x24,wallstreetcn_a_stock" {
		t.Fatalf("expected midday crawl to cover all A股 news sources including 金十全站, got %v", sources)
	}
}

func TestAStockRecommendationWindowUsesPhase(t *testing.T) {
	cases := []struct {
		name      string
		period    string
		phase     string
		wantStart string
		wantEnd   string
		wantLabel string
	}{
		{name: "morning preopen", period: "morning", phase: "preopen", wantStart: "08:00:00", wantEnd: "09:26:59", wantLabel: "08:00-09:26:59"},
		{name: "morning final", period: "morning", phase: "final", wantStart: "08:00:00", wantEnd: "09:26:59", wantLabel: "08:00-09:26:59"},
		{name: "afternoon preopen", period: "afternoon", phase: "preopen", wantStart: "09:30:00", wantEnd: "12:56:59", wantLabel: "09:30-12:56:59"},
		{name: "afternoon final", period: "afternoon", phase: "final", wantStart: "09:30:00", wantEnd: "13:00:59", wantLabel: "09:30-13:00:59"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, label, err := aStockRecommendationWindow("2026-06-16", tc.period, tc.phase)
			if err != nil {
				t.Fatalf("aStockRecommendationWindow error: %v", err)
			}
			if start.Format("15:04:05") != tc.wantStart || end.Format("15:04:05") != tc.wantEnd || label != tc.wantLabel {
				t.Fatalf("unexpected window start=%s end=%s label=%s", start.Format("15:04:05"), end.Format("15:04:05"), label)
			}
		})
	}
}

func TestRunAStockRecommendationContinuesWhenOneSourceFails(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":           "2026-06-16",
			"is_trading_day": true,
			"source":         "test",
			"reason":         "trading_day",
			"message":        "open",
		})
	}))
	defer akshare.Close()

	var sources []string
	var generatedPeriods []string
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceType := r.URL.Query().Get("source_type")
		sources = append(sources, sourceType)
		if sourceType == provider.SourceTypeHeadline {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":502,"message":"headline timeout","data":null}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	var contentQueried bool
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentQueried = true
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/a-stock/recommendations/generate" {
			t.Fatalf("unexpected gateway request: %s %s", r.Method, r.URL.String())
		}
		generatedPeriods = append(generatedPeriods, r.URL.Query().Get("period"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
	})

	if err := worker.runAStockRecommendationForDate(context.Background(), "2026-06-16", "afternoon", "final"); err != nil {
		t.Fatalf("runAStockRecommendationForDate should continue after one source failure, got %v", err)
	}
	if !contentQueried {
		t.Fatal("expected recommendation window to be queried after partial crawl failure")
	}
	if !slices.Contains(sources, "eastmoney_kuaixun") || !slices.Contains(sources, "sina_finance_7x24") {
		t.Fatalf("expected later A股 sources to run after headline failure, got %v", sources)
	}
	if len(generatedPeriods) != 1 || generatedPeriods[0] != "afternoon" {
		t.Fatalf("expected afternoon recommendation snapshot generation after partial crawl failure, got %v", generatedPeriods)
	}
}

func TestRunAStockAfternoonOpenRefreshForDateRefreshesExistingAfternoonSelection(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" || r.URL.Query().Get("date") != "2026-06-24" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":                 "2026-06-24",
			"is_trading_day":       true,
			"previous_trading_day": "2026-06-23",
			"source":               "test",
			"reason":               "trading_day",
			"message":              "open",
		})
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			if r.URL.Query().Get("date") != "2026-06-24" || r.URL.Query().Get("period") != "afternoon" {
				t.Fatalf("unexpected selection request: %s", r.URL.String())
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: "2026-06-24",
					Period:       "afternoon",
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "300024", Name: "机器人"},
					},
				},
			})
		case "/api/v1/a-stock/recommendations":
			t.Fatalf("snapshot probe should not run when selection exists: %s", r.URL.String())
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	var gatewayCalls []string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/a-stock/recommendations/generate" {
			t.Fatalf("unexpected gateway request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("date") != "2026-06-24" || r.URL.Query().Get("period") != "afternoon" || r.URL.Query().Get("phase") != "final" {
			t.Fatalf("unexpected gateway query: %s", r.URL.RawQuery)
		}
		gatewayCalls = append(gatewayCalls, r.URL.Query().Get("date")+"/"+r.URL.Query().Get("period"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	if err := worker.runAStockAfternoonOpenRefreshForDate(context.Background(), "2026-06-24"); err != nil {
		t.Fatalf("runAStockAfternoonOpenRefreshForDate error: %v", err)
	}
	if len(gatewayCalls) != 1 || gatewayCalls[0] != "2026-06-24/afternoon" {
		t.Fatalf("expected one afternoon refresh, got %v", gatewayCalls)
	}
}

func TestRunAStockAfternoonOpenRefreshForDateSkipsWhenNoExistingRecommendation(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "scheduler-astock-afternoon-open-skip.db")
	store, err := sqlitestore.New(dbPath)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	defer func() { _ = store.Close() }()

	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":                 "2026-06-24",
			"is_trading_day":       true,
			"previous_trading_day": "2026-06-23",
			"source":               "test",
			"reason":               "trading_day",
			"message":              "open",
		})
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSelectionListResult{
					Found:        false,
					StrategyDate: r.URL.Query().Get("date"),
					Period:       r.URL.Query().Get("period"),
				},
			})
		case "/api/v1/a-stock/recommendations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSnapshot{
					Found:        false,
					StrategyDate: r.URL.Query().Get("date"),
					Period:       r.URL.Query().Get("period"),
				},
			})
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	var gatewayCalls int
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		DatabasePath:          dbPath,
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})
	defer func() { _ = worker.Close() }()

	job := jobDefinition{
		Name:    "a-stock-afternoon-open-refresh",
		Enabled: true,
		Run: func(ctx context.Context) error {
			return worker.runAStockAfternoonOpenRefreshForDate(ctx, "2026-06-24")
		},
	}
	if err := worker.runJob(ctx, job); err != nil {
		t.Fatalf("expected skipped afternoon open refresh not to return error, got %v", err)
	}
	if gatewayCalls != 0 {
		t.Fatalf("expected no gateway refresh when recommendation is missing, got %d", gatewayCalls)
	}
	runs, err := store.ListTaskRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListTaskRuns error: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "skipped" || !strings.Contains(runs[0].Message, "no existing recommendation") {
		t.Fatalf("expected skipped task run for missing recommendation, got %+v", runs)
	}
}

func TestRunAStockDailyBacktestRefreshForDateRefreshesCurrentAndPreviousTradingDays(t *testing.T) {
	tradingDays := map[string]aStockTradingDayStatus{
		"2026-06-22": {Date: "2026-06-22", IsTradingDay: true, PreviousTradingDay: "2026-06-19", Message: "open"},
		"2026-06-19": {Date: "2026-06-19", IsTradingDay: true, PreviousTradingDay: "2026-06-18", Message: "open"},
		"2026-06-18": {Date: "2026-06-18", IsTradingDay: true, PreviousTradingDay: "2026-06-17", Message: "open"},
		"2026-06-17": {Date: "2026-06-17", IsTradingDay: true, PreviousTradingDay: "2026-06-16", Message: "open"},
		"2026-06-16": {Date: "2026-06-16", IsTradingDay: true, PreviousTradingDay: "2026-06-15", Message: "open"},
	}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, ok := tradingDays[r.URL.Query().Get("date")]
		if !ok {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: r.URL.Query().Get("date"),
					Period:       r.URL.Query().Get("period"),
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "603936", Name: "博敏电子"},
					},
				},
			})
		case "/api/v1/a-stock/recommendations":
			t.Fatalf("snapshot probe should not run when selection exists: %s", r.URL.String())
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	gatewayCalls := make([]string, 0)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalls = append(gatewayCalls, r.URL.Query().Get("date")+"/"+r.URL.Query().Get("period"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	if err := worker.runAStockDailyBacktestRefreshForDate(context.Background(), "2026-06-22", 5); err != nil {
		t.Fatalf("runAStockDailyBacktestRefreshForDate error: %v", err)
	}
	expected := []string{
		"2026-06-22/morning", "2026-06-22/afternoon",
		"2026-06-19/morning", "2026-06-19/afternoon",
		"2026-06-18/morning", "2026-06-18/afternoon",
		"2026-06-17/morning", "2026-06-17/afternoon",
		"2026-06-16/morning", "2026-06-16/afternoon",
		"2026-06-15/morning", "2026-06-15/afternoon",
	}
	slices.Sort(gatewayCalls)
	slices.Sort(expected)
	if !slices.Equal(gatewayCalls, expected) {
		t.Fatalf("expected refreshed target set %v, got %v", expected, gatewayCalls)
	}
}

func TestRunAStockDailyBacktestRefreshForDateSkipsMissingTargetsButContinues(t *testing.T) {
	tradingDays := map[string]aStockTradingDayStatus{
		"2026-06-22": {Date: "2026-06-22", IsTradingDay: true, PreviousTradingDay: "2026-06-19", Message: "open"},
		"2026-06-19": {Date: "2026-06-19", IsTradingDay: true, PreviousTradingDay: "2026-06-18", Message: "open"},
	}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, ok := tradingDays[r.URL.Query().Get("date")]
		if !ok {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer akshare.Close()

	existing := map[string]bool{
		"2026-06-22/morning":   true,
		"2026-06-19/afternoon": true,
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("date") + "/" + r.URL.Query().Get("period")
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			found := existing[target]
			items := []model.AStockRecommendationSelection{}
			if found {
				items = append(items, model.AStockRecommendationSelection{Rank: 1, Code: "603936", Name: "博敏电子"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSelectionListResult{
					Found:        found,
					StrategyDate: r.URL.Query().Get("date"),
					Period:       r.URL.Query().Get("period"),
					Items:        items,
				},
			})
		case "/api/v1/a-stock/recommendations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSnapshot{
					Found:        false,
					StrategyDate: r.URL.Query().Get("date"),
					Period:       r.URL.Query().Get("period"),
				},
			})
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	gatewayCalls := make([]string, 0)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalls = append(gatewayCalls, r.URL.Query().Get("date")+"/"+r.URL.Query().Get("period"))
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	if err := worker.runAStockDailyBacktestRefreshForDate(context.Background(), "2026-06-22", 1); err != nil {
		t.Fatalf("runAStockDailyBacktestRefreshForDate error: %v", err)
	}
	expected := []string{"2026-06-19/afternoon", "2026-06-22/morning"}
	slices.Sort(gatewayCalls)
	slices.Sort(expected)
	if !slices.Equal(gatewayCalls, expected) {
		t.Fatalf("expected only existing targets to refresh, got %v", gatewayCalls)
	}
}

func TestRunAStockDailyBacktestRefreshForDateContinuesAfterGatewayFailure(t *testing.T) {
	tradingDays := map[string]aStockTradingDayStatus{
		"2026-06-22": {Date: "2026-06-22", IsTradingDay: true, PreviousTradingDay: "2026-06-19", Message: "open"},
		"2026-06-19": {Date: "2026-06-19", IsTradingDay: true, PreviousTradingDay: "2026-06-18", Message: "open"},
	}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, ok := tradingDays[r.URL.Query().Get("date")]
		if !ok {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/a-stock/recommendation-selections":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": model.AStockRecommendationSelectionListResult{
					Found:        true,
					StrategyDate: r.URL.Query().Get("date"),
					Period:       r.URL.Query().Get("period"),
					Items: []model.AStockRecommendationSelection{
						{Rank: 1, Code: "603936", Name: "博敏电子"},
					},
				},
			})
		case "/api/v1/a-stock/recommendations":
			t.Fatalf("snapshot probe should not run when selection exists: %s", r.URL.String())
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	gatewayCalls := make([]string, 0)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("date") + "/" + r.URL.Query().Get("period")
		gatewayCalls = append(gatewayCalls, target)
		if target == "2026-06-19/afternoon" {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	err := worker.runAStockDailyBacktestRefreshForDate(context.Background(), "2026-06-22", 1)
	if err == nil || !strings.Contains(err.Error(), "2026-06-19/afternoon") {
		t.Fatalf("expected aggregated failure mentioning 2026-06-19/afternoon, got %v", err)
	}
	expected := []string{"2026-06-22/morning", "2026-06-22/afternoon", "2026-06-19/morning", "2026-06-19/afternoon"}
	slices.Sort(gatewayCalls)
	slices.Sort(expected)
	if !slices.Equal(gatewayCalls, expected) {
		t.Fatalf("expected refresh to continue after one target failure, got %v", gatewayCalls)
	}
}

func TestRunAStockBacktestRefreshJobsSkipNonTradingDay(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":               "2026-06-19",
			"is_trading_day":     false,
			"latest_trading_day": "2026-06-18",
			"next_trading_day":   "2026-06-22",
			"source":             "test",
			"reason":             "market_closed",
			"message":            "closed",
		})
	}))
	defer akshare.Close()

	contentCalls := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentCalls++
		t.Fatalf("content should not be called on non-trading day: %s", r.URL.String())
	}))
	defer content.Close()

	gatewayCalls := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalls++
		t.Fatalf("gateway should not be called on non-trading day: %s", r.URL.String())
	}))
	defer gateway.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		GatewayWebURL:         gateway.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "afternoon open refresh",
			run: func() error {
				return worker.runAStockAfternoonOpenRefreshForDate(context.Background(), "2026-06-19")
			},
		},
		{
			name: "daily backtest refresh",
			run: func() error {
				return worker.runAStockDailyBacktestRefreshForDate(context.Background(), "2026-06-19", 5)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			var skipped jobSkippedError
			if !errors.As(err, &skipped) {
				t.Fatalf("expected skipped error, got %v", err)
			}
			if !strings.Contains(skipped.Error(), "2026-06-19") {
				t.Fatalf("expected skipped message to mention date, got %q", skipped.Error())
			}
		})
	}
	if contentCalls != 0 || gatewayCalls != 0 {
		t.Fatalf("expected no content/gateway calls on non-trading day, got content=%d gateway=%d", contentCalls, gatewayCalls)
	}
}

func TestRunAStockRecommendationSkipsNonTradingDay(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "scheduler-skip.db")
	store, err := sqlitestore.New(dbPath)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	defer func() { _ = store.Close() }()

	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" || r.URL.Query().Get("date") != "2026-06-19" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":               "2026-06-19",
			"is_trading_day":     false,
			"latest_trading_day": "2026-06-18",
			"next_trading_day":   "2026-06-22",
			"source":             "test",
			"reason":             "market_closed",
			"message":            "A-share market is closed; stock recommendations are disabled.",
		})
	}))
	defer akshare.Close()

	var crawlerCalls int
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crawlerCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("content should not be called on non-trading day: %s", r.URL.String())
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		DatabasePath:          dbPath,
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		ContentURL:            content.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})
	defer func() { _ = worker.Close() }()

	job := jobDefinition{
		Name:    "a-stock-morning-recommendation",
		Enabled: true,
		Run: func(ctx context.Context) error {
			return worker.runAStockRecommendationForDate(ctx, "2026-06-19", "morning", "final")
		},
	}
	if err := worker.runJob(context.Background(), job); err != nil {
		t.Fatalf("expected skipped job not to return error, got %v", err)
	}
	if crawlerCalls != 0 {
		t.Fatalf("expected no crawler calls on non-trading day, got %d", crawlerCalls)
	}
	runs, err := store.ListTaskRuns(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListTaskRuns error: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "skipped" || !strings.Contains(runs[0].Message, "2026-06-19") {
		t.Fatalf("expected skipped task run, got %+v", runs)
	}
}

func TestRunAStockRecommendationFailsClosedWhenTradingCalendarUnavailable(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "calendar unavailable"})
	}))
	defer akshare.Close()

	var crawlerCalls int
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crawlerCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		CrawlerURL:            crawler.URL,
		ContentURL:            "http://127.0.0.1:1",
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})
	err := worker.runAStockRecommendationForDate(context.Background(), "2026-06-19", "morning", "final")
	if err == nil || !strings.Contains(err.Error(), "trading calendar unavailable") {
		t.Fatalf("expected fail-closed calendar error, got %v", err)
	}
	if crawlerCalls != 0 {
		t.Fatalf("expected no crawler calls when calendar is unavailable, got %d", crawlerCalls)
	}
}

func TestSchedulerAStockTradingDayProxy(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" || r.URL.Query().Get("date") != "2026-06-19" {
			t.Fatalf("unexpected trading-day request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date":               "2026-06-19",
			"is_trading_day":     false,
			"latest_trading_day": "2026-06-18",
			"next_trading_day":   "2026-06-22",
			"source":             "test",
			"reason":             "market_closed",
			"message":            "closed",
		})
	}))
	defer akshare.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/a-stock/trading-day?date=2026-06-19", nil)
	rr := httptest.NewRecorder()
	worker.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected trading-day proxy 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data aStockTradingDayStatus `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal trading-day proxy: %v", err)
	}
	if envelope.Data.Date != "2026-06-19" || envelope.Data.IsTradingDay {
		t.Fatalf("unexpected trading-day proxy payload: %+v", envelope.Data)
	}
}

func TestRunAStockAuctionCrawlFetchesAkshareAndWritesContent(t *testing.T) {
	var contentPayload struct {
		Date  string                      `json:"date"`
		Items []model.AStockAuctionAmount `json:"items"`
	}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/auction" || r.URL.Query().Get("date") != "2026-06-16" || r.URL.Query().Get("limit") != "0" {
			t.Fatalf("unexpected akshare request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"date":"2026-06-16","items":[{"code":"002230","name":"科大讯飞","auction_price":41.2,"auction_volume":123400,"auction_amount":5084080,"source":"akshare_pre_min","status":"ok"},{"code":"012322","name":"012322","auction_price":1.1,"auction_volume":120000,"auction_amount":132000,"source":"akshare_pre_min","status":"ok"},{"code":"011631","name":"基金测试","auction_price":1.2,"auction_volume":110000,"auction_amount":132000,"source":"akshare_pre_min","status":"ok"},{"code":"301696","name":"301696","auction_price":118.22,"auction_volume":100000,"auction_amount":11822000,"source":"akshare_pre_min","status":"ok"},{"code":"000001","name":"平安银行","auction_price":12,"auction_volume":0,"auction_amount":0,"source":"akshare_pre_min","status":"no_auction_data"}]}}`))
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/a-stock/auction" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if err := json.NewDecoder(r.Body).Decode(&contentPayload); err != nil {
			t.Fatalf("decode content payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL: akshare.URL,
		ContentURL:       content.URL,
		HTTPTimeout:      time.Second,
		ServiceToken:     "secret-token",
	})
	if err := worker.runAStockAuctionCrawlForDate(context.Background(), "2026-06-16"); err != nil {
		t.Fatalf("runAStockAuctionCrawlForDate error: %v", err)
	}
	if contentPayload.Date != "2026-06-16" || len(contentPayload.Items) != 2 || contentPayload.Items[0].Code != "002230" || contentPayload.Items[1].Status != "no_auction_data" {
		t.Fatalf("unexpected content payload: %+v", contentPayload)
	}
}

func TestRunAStockAuctionLatestUsesAdapterDate(t *testing.T) {
	var contentPayload struct {
		Date  string                      `json:"date"`
		Items []model.AStockAuctionAmount `json:"items"`
	}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/auction" || r.URL.Query().Get("date") != "" || r.URL.Query().Get("limit") != "0" || r.URL.Query().Get("force") != "1" {
			t.Fatalf("unexpected akshare latest request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-06-15",
			"items": []model.AStockAuctionAmount{{
				TradeDate:     "2026-06-15",
				Code:          "002230",
				Name:          "科大讯飞",
				AuctionVolume: 123400,
				AuctionAmount: 5084080,
				Source:        "akshare_spot_em",
				Status:        "ok",
			}},
		})
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/a-stock/auction" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&contentPayload); err != nil {
			t.Fatalf("decode content payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL: akshare.URL,
		ContentURL:       content.URL,
		HTTPTimeout:      time.Second,
		ServiceToken:     "secret-token",
	})
	result, err := worker.runAStockAuctionLatest(context.Background())
	if err != nil {
		t.Fatalf("runAStockAuctionLatest error: %v", err)
	}
	if result.Date != "2026-06-15" || contentPayload.Date != "2026-06-15" || len(contentPayload.Items) != 1 {
		t.Fatalf("expected latest adapter date to be written, result=%+v payload=%+v", result, contentPayload)
	}
}

func TestRunAStockSectorFundFlowLatestFetchesAllGroupsAndWritesContent(t *testing.T) {
	requested := map[string]bool{}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/a-stock/trading-day":
			_ = json.NewEncoder(w).Encode(aStockTradingDayStatus{Date: "2026-07-01", IsTradingDay: true, Message: "open"})
		case "/api/a-stock/sector-fund-flow":
			sectorType := r.URL.Query().Get("sector_type")
			indicator := r.URL.Query().Get("indicator")
			requested[sectorType+"/"+indicator] = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"date":        "2026-07-01",
				"sector_type": sectorType,
				"indicator":   indicator,
				"items": []model.AStockSectorFundFlow{{
					TradeDate:     "2026-07-01",
					SectorType:    sectorType,
					Indicator:     indicator,
					Rank:          1,
					Name:          sectorType + indicator,
					MainNetInflow: 100000000,
				}},
			})
		default:
			t.Fatalf("unexpected akshare request: %s", r.URL.Path)
		}
	}))
	defer akshare.Close()

	var writes int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/internal/a-stock/sector-fund-flows" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			Date       string                       `json:"date"`
			SectorType string                       `json:"sector_type"`
			Indicator  string                       `json:"indicator"`
			Items      []model.AStockSectorFundFlow `json:"items"`
			Replace    bool                         `json:"replace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode content sector payload: %v", err)
		}
		if payload.Date != "2026-07-01" || !payload.Replace || len(payload.Items) != 1 || payload.Items[0].Name == "" {
			t.Fatalf("unexpected content sector payload: %+v", payload)
		}
		writes++
		_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusOK, "message": "ok", "data": map[string]any{"inserted": len(payload.Items), "total": len(payload.Items)}})
	}))
	defer content.Close()

	worker := NewWorker(config.Config{AStockAuctionURL: akshare.URL, ContentURL: content.URL, HTTPTimeout: 2 * time.Second})
	result, err := worker.runAStockSectorFundFlowLatest(context.Background(), false)
	if err != nil {
		t.Fatalf("runAStockSectorFundFlowLatest error: %v", err)
	}
	if result.Date != "2026-07-01" || result.Groups != 6 || result.Items != 6 || writes != 6 {
		t.Fatalf("unexpected sector fund flow result=%+v writes=%d", result, writes)
	}
	for _, key := range []string{"行业资金流/今日", "行业资金流/5日", "行业资金流/10日", "概念资金流/今日", "概念资金流/5日", "概念资金流/10日"} {
		if !requested[key] {
			t.Fatalf("expected akshare request for %s, got %+v", key, requested)
		}
	}
}

func TestRunAStockSectorFundFlowCrawlSkipsNonTradingDay(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/trading-day" {
			t.Fatalf("unexpected akshare request on non-trading day: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(aStockTradingDayStatus{Date: "2026-06-19", IsTradingDay: false, Message: "holiday"})
	}))
	defer akshare.Close()
	worker := NewWorker(config.Config{AStockAuctionURL: akshare.URL, HTTPTimeout: 2 * time.Second})
	result, err := worker.runAStockSectorFundFlowLatest(context.Background(), false)
	if err != nil {
		t.Fatalf("runAStockSectorFundFlowLatest non-trading error: %v", err)
	}
	if !result.Skipped || !strings.Contains(result.Message, "holiday") {
		t.Fatalf("expected sector fund flow non-trading skip, got %+v", result)
	}
}

func TestRunAStockAuctionCrawlForcesLatestRefresh(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/auction" || r.URL.Query().Get("date") != "" || r.URL.Query().Get("limit") != "0" || r.URL.Query().Get("force") != "1" {
			t.Fatalf("unexpected scheduled auction request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-06-18",
			"items": []model.AStockAuctionAmount{{
				TradeDate:     "2026-06-18",
				Code:          "002230",
				Name:          "科大讯飞",
				AuctionVolume: 123400,
				AuctionAmount: 5084080,
				Source:        "eastmoney_clist",
				Status:        "ok",
			}},
		})
	}))
	defer akshare.Close()

	var writeCount int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/a-stock/auction" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		writeCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL: akshare.URL,
		ContentURL:       content.URL,
		HTTPTimeout:      time.Second,
		ServiceToken:     "secret-token",
	})
	if err := worker.runAStockAuctionCrawl(context.Background()); err != nil {
		t.Fatalf("runAStockAuctionCrawl error: %v", err)
	}
	if writeCount != 1 {
		t.Fatalf("expected scheduled latest crawl to write once, got %d", writeCount)
	}
}

func TestHandleRunAStockAuctionLatestTriggersAsync(t *testing.T) {
	releaseAdapter := make(chan struct{})
	contentWritten := make(chan struct{}, 1)
	adapterReleased := false
	defer func() {
		if !adapterReleased {
			close(releaseAdapter)
		}
	}()

	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/auction" || r.URL.Query().Get("date") != "" || r.URL.Query().Get("limit") != "0" || r.URL.Query().Get("force") != "1" {
			t.Fatalf("unexpected akshare latest request: %s", r.URL.String())
		}
		<-releaseAdapter
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-06-15",
			"items": []model.AStockAuctionAmount{{
				TradeDate:     "2026-06-15",
				Code:          "002230",
				Name:          "科大讯飞",
				AuctionVolume: 123400,
				AuctionAmount: 5084080,
				Source:        "akshare_spot_em",
				Status:        "ok",
			}},
		})
	}))
	defer akshare.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/a-stock/auction" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		select {
		case contentWritten <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL:      akshare.URL,
		ContentURL:            content.URL,
		HTTPTimeout:           500 * time.Millisecond,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/a-stock/auction/latest", nil)
	req.Header.Set("X-Service-Token", "secret-token")
	rr := httptest.NewRecorder()
	started := time.Now()
	worker.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected async latest trigger 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("expected latest trigger to return before crawl finishes, took %s", elapsed)
	}
	if !strings.Contains(rr.Body.String(), `"status":"triggered"`) {
		t.Fatalf("expected triggered response, got %s", rr.Body.String())
	}
	select {
	case <-contentWritten:
		t.Fatal("expected content write to wait for background crawl")
	default:
	}

	close(releaseAdapter)
	adapterReleased = true
	select {
	case <-contentWritten:
	case <-time.After(time.Second):
		t.Fatal("expected background latest crawl to write content")
	}
}

func TestRunAStockAuctionBackfillFetchesDateRange(t *testing.T) {
	var requested []string
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if r.URL.Query().Get("limit") != "0" {
			t.Fatalf("expected full-market auction limit override, got %s", r.URL.String())
		}
		requested = append(requested, date)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]model.AStockAuctionAmount{{
			TradeDate:     date,
			Code:          "002230",
			Name:          "科大讯飞",
			AuctionPrice:  41.2,
			AuctionVolume: 123400,
			AuctionAmount: 5084080,
			Source:        "akshare_pre_min",
			Status:        "ok",
		}})
	}))
	defer akshare.Close()

	var writeCount int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/a-stock/auction" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		writeCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL: akshare.URL,
		ContentURL:       content.URL,
		HTTPTimeout:      time.Second,
		ServiceToken:     "secret-token",
	})
	result, err := worker.runAStockAuctionBackfill(context.Background(), 0, "2026-06-14", "2026-06-16")
	if err != nil {
		t.Fatalf("runAStockAuctionBackfill error: %v", err)
	}
	if strings.Join(requested, ",") != "2026-06-14,2026-06-15,2026-06-16" || writeCount != 3 || result.Succeeded != 3 {
		t.Fatalf("unexpected backfill result requested=%v writes=%d result=%+v", requested, writeCount, result)
	}
}

func TestRunAStockAuctionBackfillSkipsUnsupportedAndZeroAmountDates(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if r.URL.Query().Get("limit") != "0" {
			t.Fatalf("expected full-market auction limit override, got %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if date == "2026-06-14" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"date":    date,
				"message": "AKShare auction adapter only serves the current trading day without a usable local cache for the requested date.",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": date,
			"items": []model.AStockAuctionAmount{{
				TradeDate: date,
				Code:      "002230",
				Name:      "科大讯飞",
				Source:    "akshare_pre_min",
				Status:    "no_auction_amount",
			}},
		})
	}))
	defer akshare.Close()

	var writeCount int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL: akshare.URL,
		ContentURL:       content.URL,
		HTTPTimeout:      time.Second,
		ServiceToken:     "secret-token",
	})
	result, err := worker.runAStockAuctionBackfill(context.Background(), 0, "2026-06-14", "2026-06-15")
	if err == nil || !strings.Contains(err.Error(), "skipped all dates without usable data") {
		t.Fatalf("expected all-skipped backfill error, got err=%v result=%+v", err, result)
	}
	if result.Succeeded != 0 || result.Skipped != 2 || result.Failed != 0 || writeCount != 0 {
		t.Fatalf("expected skipped without writes, got writes=%d result=%+v", writeCount, result)
	}
}

func TestRunAStockAuctionCrawlRejectsZeroAmountPayload(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "0" {
			t.Fatalf("expected full-market auction limit override, got %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-06-16",
			"items": []model.AStockAuctionAmount{{
				TradeDate: "2026-06-16",
				Code:      "002230",
				Name:      "科大讯飞",
				Source:    "akshare_pre_min",
				Status:    "no_auction_amount",
			}},
		})
	}))
	defer akshare.Close()

	var writeCount int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockAuctionURL: akshare.URL,
		ContentURL:       content.URL,
		HTTPTimeout:      time.Second,
		ServiceToken:     "secret-token",
	})
	err := worker.runAStockAuctionCrawlForDate(context.Background(), "2026-06-16")
	if err == nil || !strings.Contains(err.Error(), "有效成交额/成交量为 0") {
		t.Fatalf("expected zero amount crawl error, got %v", err)
	}
	if writeCount != 0 {
		t.Fatalf("expected zero amount payload not to be written, got writes=%d", writeCount)
	}
}

func TestRunAStockHoldingsBackfillFetchesExternalAndWritesContent(t *testing.T) {
	var contentPayload struct {
		Items []model.StockInstitutionHolding `json:"items"`
	}
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/holdings" || r.URL.Query().Get("period") != "20260331" || r.URL.Query().Get("code") != "002230" || r.URL.Query().Get("tushare_token") != "token" {
			t.Fatalf("unexpected external holdings request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []model.StockInstitutionHolding{{
			StockCode:    "002230",
			StockName:    "科大讯飞",
			ReportPeriod: "20260331",
			HolderName:   "易方达基金",
			HolderType:   "fund",
			SourceType:   "stock_institute_hold_detail",
		}}})
	}))
	defer external.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/internal/a-stock/holdings/batch" {
			t.Fatalf("unexpected content holdings request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		if err := json.NewDecoder(r.Body).Decode(&contentPayload); err != nil {
			t.Fatalf("decode holdings content payload: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockInstitutionHoldingUpsertResult{Inserted: len(contentPayload.Items), Total: len(contentPayload.Items)}})
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		AStockHoldingURL:      external.URL,
		ContentURL:            content.URL,
		TuShareToken:          "token",
		HTTPTimeout:           time.Second,
		ServiceToken:          "secret-token",
		ExternalRetryCount:    0,
		ExternalRetryWait:     time.Millisecond,
		SchedulerCrawlTimeout: time.Second,
	})
	if err := worker.runAStockHoldingsBackfill(context.Background(), aStockHoldingCrawlOptions{Code: "002230", Period: "20260331"}); err != nil {
		t.Fatalf("runAStockHoldingsBackfill error: %v", err)
	}
	if len(contentPayload.Items) != 1 || contentPayload.Items[0].StockCode != "002230" || contentPayload.Items[0].ReportPeriod != "20260331" {
		t.Fatalf("unexpected holdings content payload: %+v", contentPayload)
	}
}

func TestParseFinanceReportDocumentExtractsRows(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><table><tr><td>1</td><td><a href="/report/1.html">科大讯飞深度研究：AI 应用点评</a></td><td>公司研究</td><td>2026-06-16</td><td>中金公司</td><td>张三</td></tr></table></body></html>`))
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	items := parseFinanceReportDocument(doc, "https://stock.finance.sina.com.cn/list.html", "sina_finance_report", time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC))
	if len(items) != 1 {
		t.Fatalf("expected one parsed report, got %+v", items)
	}
	if items[0].Title != "科大讯飞深度研究：AI 应用点评" || items[0].ResearchDate != "2026-06-16" || items[0].Institution != "中金公司" || items[0].Analyst != "张三" {
		t.Fatalf("unexpected parsed item: %+v", items[0])
	}
	if items[0].SourceURL != "https://stock.finance.sina.com.cn/report/1.html" || items[0].SourceKey == "" {
		t.Fatalf("expected resolved source url and key, got %+v", items[0])
	}
}

func TestParseSinaFinanceReportDocumentAndDetail(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><table>
<tr><th>序号</th><th>标题</th><th>报告类型</th><th>发布日期</th><th>机构</th><th>研究员</th></tr>
<tr><td>1</td><td><a href="//stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/company/rptid/835086605455/index.phtml">深度*公司*瑞联新材(688550)：显示材料持续发力</a></td><td>公司</td><td>2026-06-18</td><td>中银国际证券股份有限公司</td><td>余嫄嫄/范琦岩</td></tr>
</table></body></html>`))
	if err != nil {
		t.Fatalf("new sina list document: %v", err)
	}
	items := parseSinaFinanceReportDocument(doc, "https://stock.finance.sina.com.cn/stock/go.php/vReport_List/kind/company/index.phtml", time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC))
	if len(items) != 1 {
		t.Fatalf("expected one sina item, got %+v", items)
	}
	if items[0].Code != "688550" || items[0].Name != "瑞联新材" || items[0].Institution != "中银国际证券股份有限公司" || items[0].Analyst != "余嫄嫄/范琦岩" {
		t.Fatalf("unexpected sina list item: %+v", items[0])
	}
	detail, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><div class="content">
<h1>深度*公司*瑞联新材(688550)：显示材料持续发力 医药CDMO开拓新增量</h1>
<div class="creab"><span>类别：公司</span><span>机构：<a>中银国际证券股份有限公司</a></span><span>研究员：<a>余嫄嫄/范琦岩</a></span><span>日期：2026-06-18</span></div>
<div class="blk_container"><p>公司在显示材料、医药板块以及电子材料多点布局，给予买入评级。</p></div>
</div></body></html>`))
	if err != nil {
		t.Fatalf("new sina detail document: %v", err)
	}
	enrichSinaFinanceReportDetail(detail, &items[0])
	if !strings.Contains(items[0].Summary, "显示材料") || items[0].ResearchDate != "2026-06-18" || !strings.Contains(items[0].RawPayload, "显示材料") {
		t.Fatalf("unexpected sina detail item: %+v", items[0])
	}
}

func TestParseEastMoneyReportDocumentAndDetail(t *testing.T) {
	body := `<script>var initdata = {"hits":1,"size":50,"data":[{"title":"CMD放量业绩高速增长，稳定分红回报股东","stockName":"泛亚微透","stockCode":"688386","orgName":"山西证券股份有限公司","orgSName":"山西证券","publishDate":"2026-06-18 00:00:00.000","infoCode":"AP202606181823661327","emRatingName":"买入","researcher":"冀泳洁","predictThisYearEps":"1.79","predictThisYearPe":"53.2","predictNextYearEps":"3.08","predictNextYearPe":"30.9","indvInduName":"塑料","indvAimPriceT":"120","indvAimPriceL":"100","author":["11000390531.冀泳洁"]}]};</script>`
	items, err := parseEastMoneyReportDocument(body, "https://data.eastmoney.com/report/stock.jshtml", time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse eastmoney report: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one eastmoney item, got %+v", items)
	}
	if items[0].Code != "688386" || items[0].SourceType != "eastmoney_report" || items[0].SourceKey != "AP202606181823661327" || items[0].TargetPrice != "100-120" {
		t.Fatalf("unexpected eastmoney list item: %+v", items[0])
	}
	detail := `<script>var zwinfo= {"attach_url":"https://pdf.dfcfw.com/pdf/H3_AP202606181823661327_1.pdf?1781791903000.pdf","info_code":"AP202606181823661327","notice_content":"　　泛亚微透(688386)\n　　事件描述\n　　营收稳健增长。","notice_date":"2026-06-18 14:11:43","notice_title":"CMD放量业绩高速增长，稳定分红回报股东","rating":"买入","researcher":"冀泳洁","short_name":"泛亚微透","source_sample_name":"东方财富"};</script>`
	if err := enrichEastMoneyReportDetail(detail, &items[0]); err != nil {
		t.Fatalf("enrich eastmoney detail: %v", err)
	}
	if items[0].PDFURL == "" || !strings.Contains(items[0].Summary, "营收稳健增长") || items[0].ResearchDate != "2026-06-18" {
		t.Fatalf("unexpected eastmoney detail item: %+v", items[0])
	}
}

func TestDedupeStockResearchPrefersEastMoneyPDFAcrossSources(t *testing.T) {
	items := dedupeStockResearch([]model.StockResearchSurvey{
		{
			Code:         "603100",
			Name:         "川仪股份",
			Kind:         "report",
			Title:        "川仪股份(603100)：工业自动化仪表龙头，国产替代持续推进",
			Institution:  "国投证券股份有限公司",
			ResearchDate: "2026-06-21",
			SourceType:   "sina_finance_report",
			SourceKey:    "sina-603100",
			SourceURL:    "https://stock.finance.sina.com.cn/report/603100.html",
		},
		{
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
		},
	})
	if len(items) != 1 {
		t.Fatalf("expected one deduped item, got %+v", items)
	}
	if items[0].SourceType != "eastmoney_report" || items[0].SourceKey != "AP202606211823706310" || !strings.Contains(items[0].PDFURL, "AP202606211823706310") {
		t.Fatalf("expected eastmoney report with PDF to win, got %+v", items[0])
	}
}

func TestRunStockResearchBackfillFetchesExternalAndWritesContent(t *testing.T) {
	var captured []model.StockResearchSurvey
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{Page: 1, PageSize: 200, Total: 0}})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/internal/stock-research/batch" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		var payload struct {
			Items []model.StockResearchSurvey `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode content payload: %v", err)
		}
		captured = payload.Items
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchUpsertResult{Inserted: len(payload.Items), Total: len(payload.Items)}})
	}))
	defer content.Close()

	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stock-research" || r.URL.Query().Get("code") != "002230" || r.URL.Query().Get("tushare_token") != "token" {
			t.Fatalf("unexpected external request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []model.StockResearchSurvey{{
			Code:         "002230",
			Name:         "科大讯飞",
			Kind:         "report",
			Title:        "科大讯飞深度研究",
			Institution:  "中金公司",
			ResearchDate: "2026-06-16",
			SourceType:   "akshare_stock_research",
			SourceKey:    "ak-1",
		}}})
	}))
	defer external.Close()

	worker := NewWorker(config.Config{
		ContentURL:                 content.URL,
		StockResearchURL:           external.URL,
		TuShareToken:               "token",
		StockResearchPublicEnabled: false,
		HTTPTimeout:                time.Second,
		ServiceToken:               "secret-token",
		ExternalRetryCount:         0,
		ExternalRetryWait:          time.Millisecond,
		SchedulerCrawlTimeout:      time.Second,
		SinaFinanceReportURL:       "",
		SohuFinanceReportURL:       "",
	})
	if err := worker.runStockResearchBackfill(context.Background(), stockResearchCrawlOptions{Code: "002230", Start: "2026-06-01", End: "2026-06-17"}); err != nil {
		t.Fatalf("runStockResearchBackfill error: %v", err)
	}
	if len(captured) != 1 || captured[0].Code != "002230" || captured[0].SourceType != "akshare_stock_research" {
		t.Fatalf("unexpected captured stock research payload: %+v", captured)
	}
}

func TestRunStockResearchBackfillSkipsUnavailablePublicSources(t *testing.T) {
	var captured []model.StockResearchSurvey
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/internal/stock-research/batch" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		var payload struct {
			Items []model.StockResearchSurvey `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode content payload: %v", err)
		}
		captured = payload.Items
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchUpsertResult{Total: len(payload.Items)}})
	}))
	defer content.Close()

	sina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table></table></body></html>`))
	}))
	defer sina.Close()

	sohu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer sohu.Close()

	worker := NewWorker(config.Config{
		ContentURL:                 content.URL,
		StockResearchPublicEnabled: true,
		SinaFinanceReportURL:       sina.URL,
		SohuFinanceReportURL:       sohu.URL,
		HTTPTimeout:                time.Second,
		ServiceToken:               "secret-token",
		ExternalRetryCount:         0,
		ExternalRetryWait:          time.Millisecond,
		SchedulerCrawlTimeout:      time.Second,
	})
	if err := worker.runStockResearchBackfill(context.Background(), stockResearchCrawlOptions{Start: "2026-06-01", End: "2026-06-17"}); err != nil {
		t.Fatalf("expected unavailable public sources to be skipped, got %v", err)
	}
	if len(captured) != 0 {
		t.Fatalf("expected empty stock research payload, got %+v", captured)
	}
}

func TestFetchCNInfoInvestorRelationsPaginatesAndBuildsPDFItems(t *testing.T) {
	var pages []string
	cninfo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected cninfo POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse cninfo form: %v", err)
		}
		if r.Form.Get("searchTypes") != "4" || r.Form.Get("stockCode") != "300250" || r.Form.Get("beginDate") != "2026-06-17 00:00:00" || r.Form.Get("endDate") != "2026-06-18 23:59:59" {
			t.Fatalf("unexpected cninfo form: %s", r.Form.Encode())
		}
		pages = append(pages, r.Form.Get("pageNo"))
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("pageNo") {
		case "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pageNo": 1, "pageSize": 1, "totalRecord": 2, "totalPage": 2,
				"results": []map[string]any{{
					"indexId": "ir-1", "mainContent": "初灵信息投资者关系管理信息20260617", "attachmentUrl": "finalpage/2026-06-18/1225377390.PDF", "stockCode": "300250", "companyShortName": "初灵信息", "pubDate": "1781768047000", "filetype": "PDF",
				}},
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pageNo": 2, "pageSize": 1, "totalRecord": 2, "totalPage": 2,
				"results": []map[string]any{{
					"indexId": "ir-2", "mainContent": "初灵信息投资者关系活动记录表", "attachmentUrl": "http://static.cninfo.com.cn/finalpage/2026-06-17/1225000000.PDF", "stockCode": "300250", "companyShortName": "初灵信息", "pubDate": "1781681647000", "filetype": "PDF",
				}},
			})
		default:
			t.Fatalf("unexpected page: %s", r.Form.Get("pageNo"))
		}
	}))
	defer cninfo.Close()

	worker := NewWorker(config.Config{
		InvestorRelationsURL: cninfo.URL + "/newircs/index/search",
		HTTPTimeout:          time.Second,
		ExternalRetryWait:    time.Millisecond,
		ExternalRetryCount:   0,
	})
	items, err := worker.fetchCNInfoInvestorRelations(context.Background(), stockResearchCrawlOptions{Code: "300250", Start: "2026-06-17", End: "2026-06-18"})
	if err != nil {
		t.Fatalf("fetchCNInfoInvestorRelations error: %v", err)
	}
	if strings.Join(pages, ",") != "1,2" {
		t.Fatalf("expected two cninfo pages, got %+v", pages)
	}
	if len(items) != 2 || items[0].SourceType != investorRelationsSourceType || items[0].Kind != "survey" || !strings.HasPrefix(items[0].PDFURL, cninfoStaticBaseURL) || items[1].PDFURL == "" {
		t.Fatalf("unexpected investor relation items: %+v", items)
	}
}

func TestInvestorRelationsBackfillHandlerReturnsBeforeCrawlCompletes(t *testing.T) {
	cninfoRequested := make(chan struct{}, 1)
	unblockCNInfo := make(chan struct{})
	cninfo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cninfoRequested <- struct{}{}
		<-unblockCNInfo
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pageNo": 1, "pageSize": 50, "totalRecord": 0, "totalPage": 0, "results": []map[string]any{},
		})
	}))
	defer cninfo.Close()

	contentCalled := make(chan struct{}, 1)
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentCalled <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "ok"}})
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:            content.URL,
		InvestorRelationsURL:  cninfo.URL + "/newircs/index/search",
		ServiceToken:          "secret-token",
		HTTPTimeout:           5 * time.Second,
		ExternalRetryCount:    0,
		ExternalRetryWait:     time.Millisecond,
		SchedulerCrawlTimeout: 5 * time.Second,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/investor-relations/backfill?start=2025-06-18&end=2026-06-18", nil)
	req.Header.Set("X-Service-Token", "secret-token")
	rr := httptest.NewRecorder()

	started := time.Now()
	worker.Router().ServeHTTP(rr, req)
	elapsed := time.Since(started)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected async backfill trigger 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("expected handler to return before crawl completes, took %s", elapsed)
	}

	select {
	case <-cninfoRequested:
	case <-time.After(time.Second):
		t.Fatal("expected background investor relations crawl to start")
	}
	close(unblockCNInfo)
	select {
	case <-contentCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected background investor relations crawl to finish")
	}
}

func TestRunCrawlLinkHeartbeatRecordsFailures(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "scheduler-heartbeat.db")
	store, err := sqlitestore.New(dbPath)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	defer func() { _ = store.Close() }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/redirect":
			w.WriteHeader(http.StatusFound)
		case "/bad":
			http.Error(w, "bad upstream", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := NewWorker(config.Config{
		DatabasePath:      dbPath,
		HTTPTimeout:       time.Second,
		ExternalRetryWait: time.Millisecond,
	})
	defer func() { _ = worker.Close() }()

	err = worker.runCrawlLinkHeartbeatForSites(ctx, []crawlLinkHeartbeatSite{
		{SourceType: "ok_site", Name: "OK site", URL: server.URL + "/ok"},
		{SourceType: "redirect_site", Name: "Redirect site", URL: server.URL + "/redirect"},
		{SourceType: "bad_site", Name: "Bad site", URL: server.URL + "/bad"},
	})
	if err != nil {
		t.Fatalf("runCrawlLinkHeartbeatForSites error: %v", err)
	}

	runs, err := store.ListTaskRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListTaskRuns error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one heartbeat task run, got %+v", runs)
	}
	if runs[0].TaskName != "crawl-link-heartbeat" || runs[0].Status != "failed" {
		t.Fatalf("expected failed heartbeat task run, got %+v", runs[0])
	}
	if !strings.Contains(runs[0].Message, "ok=2 failed=1") || !strings.Contains(runs[0].Message, "bad_site") {
		t.Fatalf("expected heartbeat failure summary, got %q", runs[0].Message)
	}
}

func TestTheBlockHeartbeatUsesRSSForLatestPage(t *testing.T) {
	if got := theBlockHeartbeatURL("https://www.theblock.co/latest-crypto-news"); got != "https://www.theblock.co/rss.xml" {
		t.Fatalf("expected The Block latest page heartbeat to use RSS, got %q", got)
	}
	if got := theBlockHeartbeatURL("https://example.com/custom.xml"); got != "https://example.com/custom.xml" {
		t.Fatalf("expected custom The Block heartbeat URL to pass through, got %q", got)
	}
}

func TestSchedulerCryptoJobsEnabledWhenEndpointsConfigured(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:                time.Second,
		CryptoXURL:                 "https://crypto.example.com/x",
		CryptoTelegramURL:          "https://crypto.example.com/telegram",
		ForesightNewsflashURL:      "https://foresight.example.com/news",
		CoinDeskZHLatestURL:        "https://coindesk.example.com/zh/latest",
		PANewsNewsflashURL:         "https://panews.example.com/rss.xml",
		TheBlockLatestURL:          "https://www.theblock.co/latest-crypto-news",
		CryptoXInterval:            2 * time.Minute,
		CryptoTelegramInterval:     3 * time.Minute,
		ForesightNewsflashInterval: 4 * time.Minute,
		CoinDeskZHLatestInterval:   5 * time.Minute,
		PANewsNewsflashInterval:    6 * time.Minute,
		TheBlockLatestInterval:     7 * time.Minute,
		WechatCleanupInterval:      time.Hour,
		WechatPushInterval:         time.Hour,
		FlashInterval:              time.Hour,
		HeadlineInterval:           time.Hour,
		AnalysisInterval:           time.Hour,
	})

	var cryptoXJob, cryptoTelegramJob, foresightJob, coindeskJob, panewsJob, theBlockJob Job
	for _, job := range worker.Jobs() {
		switch job.Name {
		case "crypto-x-crawl":
			cryptoXJob = job
		case "crypto-telegram-crawl":
			cryptoTelegramJob = job
		case "foresight-newsflash-crawl":
			foresightJob = job
		case "coindesk-zh-latest-crawl":
			coindeskJob = job
		case "panews-newsflash-crawl":
			panewsJob = job
		case "theblock-latest-crawl":
			theBlockJob = job
		}
	}

	if !cryptoXJob.Enabled || cryptoXJob.IntervalSec != 120 || cryptoXJob.NextRunAt == nil {
		t.Fatalf("expected enabled crypto x job with runtime metadata, got %+v", cryptoXJob)
	}
	if !cryptoTelegramJob.Enabled || cryptoTelegramJob.IntervalSec != 180 || cryptoTelegramJob.NextRunAt == nil {
		t.Fatalf("expected enabled crypto telegram job with runtime metadata, got %+v", cryptoTelegramJob)
	}
	if !foresightJob.Enabled || foresightJob.IntervalSec != 240 || foresightJob.NextRunAt == nil {
		t.Fatalf("expected enabled foresight job with runtime metadata, got %+v", foresightJob)
	}
	if !coindeskJob.Enabled || coindeskJob.IntervalSec != 300 || coindeskJob.NextRunAt == nil {
		t.Fatalf("expected enabled coindesk job with runtime metadata, got %+v", coindeskJob)
	}
	if !panewsJob.Enabled || panewsJob.IntervalSec != 360 || panewsJob.NextRunAt == nil {
		t.Fatalf("expected enabled panews job with runtime metadata, got %+v", panewsJob)
	}
	if !theBlockJob.Enabled || theBlockJob.IntervalSec != 420 || theBlockJob.NextRunAt == nil {
		t.Fatalf("expected enabled theblock job with runtime metadata, got %+v", theBlockJob)
	}
}

func TestSchedulerFinanceNewsCrawlJobsEnabledWhenConfigured(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		EastMoneyKuaixunURL:   "https://kuaixun.eastmoney.com/",
		Jin10FullEnabled:      true,
		WallStreetCNAStockURL: "https://wallstreetcn.com/live/a-stock",
		CLSTelegraphURL:       "https://www.cls.cn/telegraph",
		SinaFinance7x24URL:    "https://finance.sina.com.cn/7x24/?tag=10",
		FlashInterval:         time.Hour,
		HeadlineInterval:      time.Hour,
		AnalysisInterval:      time.Hour,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})
	jobs := map[string]Job{}
	for _, job := range worker.Jobs() {
		jobs[job.Name] = job
	}

	for _, name := range []string{
		"eastmoney-kuaixun-crawl",
		"jin10-full-crawl",
		"wallstreetcn-a-stock-crawl",
		"cls-telegraph-crawl",
		"sina-finance-7x24-crawl",
	} {
		job := jobs[name]
		if !job.Enabled || job.Cron != "0 0/5 * * * ?" || job.IntervalSec != 300 || job.NextRunAt == nil {
			t.Fatalf("expected enabled 5-minute finance news crawl job %s, got %+v", name, job)
		}
	}
}

func TestRunAStockIncrementalCrawlJobsUseRecentPublishWindow(t *testing.T) {
	type crawlRequest struct {
		sourceType string
		start      string
		end        string
		timeField  string
	}
	var got []crawlRequest
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		got = append(got, crawlRequest{
			sourceType: r.URL.Query().Get("source_type"),
			start:      r.URL.Query().Get("start"),
			end:        r.URL.Query().Get("end"),
			timeField:  r.URL.Query().Get("time_field"),
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		CrawlerURL:            crawler.URL,
		EastMoneyKuaixunURL:   "https://kuaixun.eastmoney.com/",
		Jin10FullEnabled:      true,
		WallStreetCNAStockURL: "https://wallstreetcn.com/live/a-stock",
		CLSTelegraphURL:       "https://www.cls.cn/telegraph",
		SinaFinance7x24URL:    "https://finance.sina.com.cn/7x24/?tag=10",
	})

	jobs := []struct {
		name       string
		sourceType string
	}{
		{name: "eastmoney-kuaixun-crawl", sourceType: provider.SourceTypeEastMoneyKuaixun},
		{name: "jin10-full-crawl", sourceType: provider.SourceTypeJin10Full},
		{name: "wallstreetcn-a-stock-crawl", sourceType: provider.SourceTypeWallStreetCNAStock},
		{name: "cls-telegraph-crawl", sourceType: provider.SourceTypeCLSTelegraph},
		{name: "sina-finance-7x24-crawl", sourceType: provider.SourceTypeSinaFinance7x24},
	}
	for _, job := range jobs {
		if err := worker.RunJobByName(context.Background(), job.name); err != nil {
			t.Fatalf("RunJobByName %s error: %v", job.name, err)
		}
	}
	if len(got) != len(jobs) {
		t.Fatalf("expected %d crawl requests, got %d: %+v", len(jobs), len(got), got)
	}
	for idx, request := range got {
		if request.sourceType != jobs[idx].sourceType {
			t.Fatalf("request %d expected source type %q, got %q", idx, jobs[idx].sourceType, request.sourceType)
		}
		if request.timeField != "publish_time" {
			t.Fatalf("request %d expected publish_time filter, got %q", idx, request.timeField)
		}
		start, err := time.ParseInLocation("2006-01-02 15:04:05", request.start, aStockLocation())
		if err != nil {
			t.Fatalf("request %d invalid start time %q: %v", idx, request.start, err)
		}
		end, err := time.ParseInLocation("2006-01-02 15:04:05", request.end, aStockLocation())
		if err != nil {
			t.Fatalf("request %d invalid end time %q: %v", idx, request.end, err)
		}
		if end.Sub(start) != 32*time.Minute {
			t.Fatalf("request %d expected 32-minute recent window, got start=%s end=%s", idx, request.start, request.end)
		}
	}
}

func TestSchedulerAStockAuctionJobEnabledWhenEndpointConfigured(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		AStockAuctionURL:      "http://127.0.0.1:8087",
		FlashInterval:         time.Hour,
		HeadlineInterval:      time.Hour,
		AnalysisInterval:      time.Hour,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})
	var auctionJob Job
	for _, job := range worker.Jobs() {
		if job.Name == "a-stock-auction-crawl" {
			auctionJob = job
			break
		}
	}
	if !auctionJob.Enabled || auctionJob.Cron != "0 26 9 * * ?" || auctionJob.NextRunAt == nil {
		t.Fatalf("expected enabled A股 auction crawl with 09:26 cron, got %+v", auctionJob)
	}
}

func TestSchedulerAStockHoldingsJobEnabledWhenEndpointConfigured(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		AStockHoldingURL:      "http://127.0.0.1:19092",
		FlashInterval:         time.Hour,
		HeadlineInterval:      time.Hour,
		AnalysisInterval:      time.Hour,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})
	var holdingsJob Job
	for _, job := range worker.Jobs() {
		if job.Name == "a-stock-holdings-crawl" {
			holdingsJob = job
			break
		}
	}
	if !holdingsJob.Enabled || holdingsJob.Cron != "0 35 2 * * ?" || holdingsJob.NextRunAt == nil {
		t.Fatalf("expected enabled A股 holdings crawl with 02:35 cron, got %+v", holdingsJob)
	}
}

func TestSchedulerCronNextRunAndEnvOverrides(t *testing.T) {
	next, err := nextCronRun("0 0/10 * * * ?", time.Date(2026, 6, 12, 8, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("nextCronRun error: %v", err)
	}
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	if got := next.In(shanghai).Format("15:04:05"); got != "16:10:00" {
		t.Fatalf("expected next run at 16:10:00 Asia/Shanghai, got %s", got)
	}

	t.Setenv("YUQING_SCHEDULER_HOT_DATA_REFRESH_ENABLED", "false")
	t.Setenv("YUQING_SCHEDULER_ANALYSIS_REFRESH_CRON", "0 30 9 * * ?")
	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		FlashInterval:         time.Hour,
		HeadlineInterval:      time.Hour,
		AnalysisInterval:      time.Hour,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})
	jobs := worker.Jobs()
	var hotData, analysis Job
	for _, job := range jobs {
		switch job.Name {
		case "hot-data-refresh":
			hotData = job
		case "analysis-refresh":
			analysis = job
		}
	}
	if hotData.Enabled {
		t.Fatalf("expected hot-data-refresh to be disabled by env, got %+v", hotData)
	}
	if analysis.Cron != "0 30 9 * * ?" || analysis.NextRunAt == nil {
		t.Fatalf("expected analysis cron override and next run, got %+v", analysis)
	}
}

func TestSchedulerCronAcceptsFiveFieldAndIgnoresInvalidOverrides(t *testing.T) {
	if _, err := nextCronRun("*/10 * * * *", time.Date(2026, 6, 12, 8, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("expected five-field cron to parse, got %v", err)
	}

	t.Setenv("YUQING_SCHEDULER_FLASH_CRAWL_CRON", "interval from YUQING_FLASH_INTERVAL_SEC, default 15s")
	t.Setenv("YUQING_SCHEDULER_HEADLINE_CRAWL_CRON", "*/5 * * * *")
	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		FlashInterval:         time.Hour,
		HeadlineInterval:      time.Hour,
		AnalysisInterval:      time.Hour,
		WechatCleanupInterval: time.Hour,
		WechatPushInterval:    time.Hour,
	})
	jobs := worker.Jobs()
	var flash, headline Job
	for _, job := range jobs {
		switch job.Name {
		case "flash-crawl":
			flash = job
		case "headline-crawl":
			headline = job
		}
	}
	if flash.Cron != "0/15 * * * * ?" || flash.NextRunAt == nil || flash.LastStatus == "invalid_cron" {
		t.Fatalf("expected invalid flash cron override to fall back to default, got %+v", flash)
	}
	if headline.Cron != "*/5 * * * *" || headline.NextRunAt == nil {
		t.Fatalf("expected five-field headline cron override to be accepted, got %+v", headline)
	}
}

func TestSchedulerRunJobRejectsUnknownJob(t *testing.T) {
	worker := NewWorker(config.Config{})
	if err := worker.RunJobByName(context.Background(), "missing-job"); err == nil {
		t.Fatal("expected missing job error")
	}
}

func TestPushWechatDailySummaryWritesAuditAndWebhook(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "scheduler.db")
	store, err := sqlitestore.New(dbPath)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	defer func() { _ = store.Close() }()

	user, err := store.CreateUser(ctx, model.User{
		Username:    "wx-user",
		DisplayName: "Wechat User",
		Role:        "user",
		Status:      1,
	}, "secret")
	if err != nil {
		t.Fatalf("CreateUser error: %v", err)
	}
	if _, err := store.UpsertWechatBinding(ctx, model.WechatBinding{UserID: user.ID, OpenID: "openid-1"}); err != nil {
		t.Fatalf("UpsertWechatBinding error: %v", err)
	}

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": []model.SearchWordStat{
				{SearchWord: "AI", WordCount: 3},
				{SearchWord: "新能源", WordCount: 2},
			},
		})
	}))
	defer content.Close()

	var webhookCalls atomic.Int32
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhook.Close()

	worker := NewWorker(config.Config{
		DatabasePath:         dbPath,
		ContentURL:           content.URL,
		HTTPTimeout:          time.Second,
		ServiceToken:         "secret-token",
		WechatPushEnabled:    true,
		WechatPushWebhookURL: webhook.URL,
	})
	defer func() { _ = worker.Close() }()

	if err := worker.pushWechatDailySummary(ctx); err != nil {
		t.Fatalf("pushWechatDailySummary error: %v", err)
	}
	if webhookCalls.Load() != 1 {
		t.Fatalf("expected one webhook call, got %d", webhookCalls.Load())
	}

	logs, err := store.ListAuditLogs(ctx, 10, user.ID, "wechat.daily_push")
	if err != nil {
		t.Fatalf("ListAuditLogs error: %v", err)
	}
	if len(logs) != 1 || !strings.Contains(logs[0].DetailJSON, "AI") {
		t.Fatalf("unexpected audit logs: %+v", logs)
	}
}
