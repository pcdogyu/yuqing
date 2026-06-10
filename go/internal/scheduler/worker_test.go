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

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
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
