package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
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
