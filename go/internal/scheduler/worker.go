package scheduler

import (
	"context"
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
	go w.loop(ctx, "analysis-refresh", 2*time.Minute, func() error {
		_, err := w.client.R().
			Post(w.cfg.ContentURL + "/api/v1/admin/tasks/analysis/refresh")
		return err
	})
	<-ctx.Done()
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
