package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
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
		if r.URL.Path != "/api/v1/admin/tasks/crawl" || r.URL.Query().Get("source_type") != "headline" {
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

	if err := worker.runCrawl(context.Background(), "headline"); err != nil {
		t.Fatalf("expected crawl request to use dedicated timeout, got %v", err)
	}
	if !sawToken.Load() {
		t.Fatal("expected crawl request to include service token")
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
	if len(listEnvelope.Data) != 25 {
		t.Fatalf("expected 25 scheduler jobs, got %d", len(listEnvelope.Data))
	}
	var heartbeatJob, hotJob, cryptoXJob, cryptoTelegramJob, foresightJob, coindeskJob, panewsJob, aStockMorningJob, aStockAfternoonJob, aStockAuctionJob Job
	for _, job := range listEnvelope.Data {
		switch job.Name {
		case "crawl-link-heartbeat":
			heartbeatJob = job
		case "hot-data-refresh":
			hotJob = job
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
		case "a-stock-morning-recommendation":
			aStockMorningJob = job
		case "a-stock-afternoon-recommendation":
			aStockAfternoonJob = job
		case "a-stock-auction-crawl":
			aStockAuctionJob = job
		}
	}
	if hotJob.JavaQuartzName != "HotDataSchedule" || hotJob.Cron == "" || hotJob.NextRunAt == nil {
		t.Fatalf("expected hot job runtime metadata, got %+v", hotJob)
	}
	if heartbeatJob.JavaQuartzName != "CrawlLinkHeartbeat" || heartbeatJob.Cron != "0 0/5 * * * ?" || heartbeatJob.IntervalSec != 300 || heartbeatJob.NextRunAt == nil {
		t.Fatalf("expected crawl link heartbeat metadata, got %+v", heartbeatJob)
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
	if aStockMorningJob.Cron != "0 25 9 * * ?" || aStockMorningJob.NextRunAt == nil {
		t.Fatalf("expected A股 morning recommendation cron metadata, got %+v", aStockMorningJob)
	}
	if aStockAfternoonJob.Cron != "0 50 12 * * ?" || aStockAfternoonJob.NextRunAt == nil {
		t.Fatalf("expected A股 afternoon recommendation cron metadata, got %+v", aStockAfternoonJob)
	}
	if aStockAuctionJob.Cron != "0 30 9 * * ?" || aStockAuctionJob.Enabled {
		t.Fatalf("expected A股 auction crawl disabled by default with 09:30 cron, got %+v", aStockAuctionJob)
	}
	if cryptoXJob.Enabled || cryptoTelegramJob.Enabled || foresightJob.Enabled || coindeskJob.Enabled || panewsJob.Enabled {
		t.Fatalf("expected crypto jobs disabled without endpoint urls, got x=%+v telegram=%+v foresight=%+v coindesk=%+v panews=%+v", cryptoXJob, cryptoTelegramJob, foresightJob, coindeskJob, panewsJob)
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
	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/tasks/crawl" {
			t.Fatalf("unexpected crawler request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("X-Service-Token") != "secret-token" {
			t.Fatalf("expected service token header, got %q", r.Header.Get("X-Service-Token"))
		}
		sources = append(sources, r.URL.Query().Get("source_type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer crawler.Close()

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/articles" {
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
		if r.URL.Query().Get("start") != "2026-06-16T01:26:00Z" || r.URL.Query().Get("end") != "2026-06-16T04:50:59Z" {
			t.Fatalf("unexpected A股 afternoon window query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		CrawlerURL:            crawler.URL,
		ContentURL:            content.URL,
		HTTPTimeout:           time.Second,
		SchedulerCrawlTimeout: time.Second,
		ServiceToken:          "secret-token",
	})

	if err := worker.runAStockRecommendationForDate(context.Background(), "2026-06-16", "afternoon"); err != nil {
		t.Fatalf("runAStockRecommendationForDate error: %v", err)
	}
	sort.Strings(sources)
	if strings.Join(sources, ",") != "eastmoney_kuaixun,flash,headline,jin10_full" {
		t.Fatalf("expected all A股 sources to be crawled, got %v", sources)
	}
}

func TestRunAStockAuctionCrawlFetchesAkshareAndWritesContent(t *testing.T) {
	var contentPayload struct {
		Date  string                      `json:"date"`
		Items []model.AStockAuctionAmount `json:"items"`
	}
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/auction" || r.URL.Query().Get("date") != "2026-06-16" {
			t.Fatalf("unexpected akshare request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"date":"2026-06-16","items":[{"code":"002230","name":"科大讯飞","auction_price":41.2,"auction_volume":123400,"auction_amount":5084080,"source":"akshare_pre_min","status":"ok"},{"code":"000001","name":"平安银行","auction_price":12,"auction_volume":0,"auction_amount":0,"source":"akshare_pre_min","status":"no_auction_data"}]}`))
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

func TestSchedulerCryptoJobsEnabledWhenEndpointsConfigured(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:                time.Second,
		CryptoXURL:                 "https://crypto.example.com/x",
		CryptoTelegramURL:          "https://crypto.example.com/telegram",
		ForesightNewsflashURL:      "https://foresight.example.com/news",
		CoinDeskZHLatestURL:        "https://coindesk.example.com/zh/latest",
		PANewsNewsflashURL:         "https://panews.example.com/rss.xml",
		CryptoXInterval:            2 * time.Minute,
		CryptoTelegramInterval:     3 * time.Minute,
		ForesightNewsflashInterval: 4 * time.Minute,
		CoinDeskZHLatestInterval:   5 * time.Minute,
		PANewsNewsflashInterval:    6 * time.Minute,
		WechatCleanupInterval:      time.Hour,
		WechatPushInterval:         time.Hour,
		FlashInterval:              time.Hour,
		HeadlineInterval:           time.Hour,
		AnalysisInterval:           time.Hour,
	})

	var cryptoXJob, cryptoTelegramJob, foresightJob, coindeskJob, panewsJob Job
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
}

func TestSchedulerAStockAuctionJobEnabledWhenEndpointConfigured(t *testing.T) {
	worker := NewWorker(config.Config{
		HTTPTimeout:           time.Second,
		AStockAuctionURL:      "http://127.0.0.1:19091",
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
	if !auctionJob.Enabled || auctionJob.Cron != "0 30 9 * * ?" || auctionJob.NextRunAt == nil {
		t.Fatalf("expected enabled A股 auction crawl with 09:30 cron, got %+v", auctionJob)
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
