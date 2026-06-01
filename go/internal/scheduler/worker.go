package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
)

type Worker struct {
	cfg    config.Config
	client *resty.Client
}

func NewWorker(cfg config.Config) *Worker {
	return &Worker{
		cfg: cfg,
		client: resty.New().
			SetTimeout(cfg.HTTPTimeout).
			SetHeader("X-Service-Token", cfg.ServiceToken),
	}
}

func (w *Worker) Run(ctx context.Context) {
	w.waitForDependencies(ctx)

	go w.loop(ctx, "flash-crawl", w.cfg.FlashInterval, func() error {
		_, err := w.client.R().
			SetQueryParam("source_type", "flash").
			Post(w.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		return err
	})
	go w.loop(ctx, "headline-crawl", w.cfg.HeadlineInterval, func() error {
		_, err := w.client.R().
			SetQueryParam("source_type", "headline").
			Post(w.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		return err
	})
	go w.loop(ctx, "analysis-refresh", w.cfg.AnalysisInterval, func() error {
		_, err := w.client.R().
			Post(w.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
		return err
	})
	<-ctx.Done()
}

func (w *Worker) waitForDependencies(ctx context.Context) {
	dependencies := []struct {
		name string
		url  string
	}{
		{name: "crawler-service", url: w.cfg.CrawlerURL + "/healthz"},
		{name: "analysis-service", url: w.cfg.AnalysisURL + "/healthz"},
	}
	for _, dependency := range dependencies {
		if err := w.waitForHealthy(ctx, dependency.name, dependency.url, 15*time.Second); err != nil {
			log.Warn().Err(err).Str("dependency", dependency.name).Msg("dependency was not ready before scheduler started")
		}
	}
}

func (w *Worker) waitForHealthy(ctx context.Context, name, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		resp, err := w.client.R().Get(url)
		if err == nil && resp.IsSuccess() {
			log.Info().Str("dependency", name).Str("url", url).Msg("dependency is ready")
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("wait for %s readiness: %w", name, err)
			}
			return fmt.Errorf("wait for %s readiness: unexpected status %s", name, resp.Status())
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (w *Worker) loop(ctx context.Context, name string, interval time.Duration, fn func() error) {
	run := func() {
		if err := fn(); err != nil {
			log.Error().Err(err).Str("task", name).Msg("scheduler task failed")
			return
		}
		log.Info().Str("task", name).Msg("scheduler task completed")
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
