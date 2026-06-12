package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if len(listEnvelope.Data) != 16 {
		t.Fatalf("expected 16 scheduler jobs, got %d", len(listEnvelope.Data))
	}
	var hotJob Job
	for _, job := range listEnvelope.Data {
		if job.Name == "hot-data-refresh" {
			hotJob = job
			break
		}
	}
	if hotJob.JavaQuartzName != "HotDataSchedule" || hotJob.Cron == "" || hotJob.NextRunAt == nil {
		t.Fatalf("expected hot job runtime metadata, got %+v", hotJob)
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
