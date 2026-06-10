package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

type Worker struct {
	cfg    config.Config
	client *resty.Client
	store  *sqlitestore.Store
	mu     sync.Mutex
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

	log.Info().Str("service", "scheduler-service").Str("task", "flash-crawl").Dur("interval", w.cfg.FlashInterval).Msg("scheduler task registered")
	go w.loop(ctx, "flash-crawl", w.cfg.FlashInterval, func() error {
		_, err := w.client.R().
			SetQueryParam("source_type", "flash").
			Post(w.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		return err
	})
	log.Info().Str("service", "scheduler-service").Str("task", "headline-crawl").Dur("interval", w.cfg.HeadlineInterval).Msg("scheduler task registered")
	go w.loop(ctx, "headline-crawl", w.cfg.HeadlineInterval, func() error {
		_, err := w.client.R().
			SetQueryParam("source_type", "headline").
			Post(w.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		return err
	})
	log.Info().Str("service", "scheduler-service").Str("task", "analysis-refresh").Dur("interval", w.cfg.AnalysisInterval).Msg("scheduler task registered")
	go w.loop(ctx, "analysis-refresh", w.cfg.AnalysisInterval, func() error {
		_, err := w.client.R().
			Post(w.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
		return err
	})
	log.Info().Str("service", "scheduler-service").Str("task", "wechat-challenge-cleanup").Dur("interval", w.cfg.WechatCleanupInterval).Msg("scheduler task registered")
	go w.loop(ctx, "wechat-challenge-cleanup", w.cfg.WechatCleanupInterval, func() error {
		return w.cleanupExpiredWechatChallenges(ctx)
	})
	if w.cfg.WechatPushEnabled {
		log.Info().Str("service", "scheduler-service").Str("task", "wechat-daily-push").Dur("interval", w.cfg.WechatPushInterval).Msg("scheduler task registered")
		go w.loop(ctx, "wechat-daily-push", w.cfg.WechatPushInterval, func() error {
			return w.pushWechatDailySummary(ctx)
		})
	}
	log.Info().Str("service", "scheduler-service").Msg("scheduler service ready")
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
	if w.cfg.WechatPushEnabled {
		dependencies = append(dependencies, struct {
			name string
			url  string
		}{name: "content-service", url: w.cfg.ContentURL + "/healthz"})
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

func (w *Worker) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.store == nil {
		return nil
	}
	err := w.store.Close()
	w.store = nil
	return err
}

func (w *Worker) cleanupExpiredWechatChallenges(ctx context.Context) error {
	store, err := w.ensureStore()
	if err != nil {
		if errors.Is(err, errStoreDisabled) {
			return nil
		}
		return err
	}
	deleted, err := store.DeleteExpiredWechatChallenges(ctx)
	if err != nil {
		return err
	}
	log.Info().Int64("deleted", deleted).Msg("wechat challenge cleanup completed")
	return nil
}

func (w *Worker) pushWechatDailySummary(ctx context.Context) error {
	store, err := w.ensureStore()
	if err != nil {
		if errors.Is(err, errStoreDisabled) {
			return nil
		}
		return err
	}
	keywords, err := w.fetchHotKeywords()
	if err != nil {
		return err
	}
	if len(keywords) == 0 {
		log.Info().Msg("wechat daily push skipped because no hot keywords were returned")
		return nil
	}
	bindings, err := store.ListWechatBindings(ctx, 500)
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		log.Info().Msg("wechat daily push skipped because no bindings exist")
		return nil
	}

	summaryLines := make([]string, 0, len(keywords))
	for idx, keyword := range keywords {
		summaryLines = append(summaryLines, fmt.Sprintf("%d. %s (%d)", idx+1, keyword.SearchWord, keyword.WordCount))
	}
	summary := strings.Join(summaryLines, "\n")
	for _, binding := range bindings {
		user, _ := store.GetUserByID(ctx, binding.UserID)
		detail := map[string]any{
			"openid":        binding.OpenID,
			"keyword_count": len(keywords),
			"keywords":      keywords,
			"summary":       summary,
			"delivered_at":  time.Now().UTC().Format(time.RFC3339),
		}
		if strings.TrimSpace(w.cfg.WechatPushWebhookURL) != "" {
			resp, postErr := w.client.R().
				SetBody(map[string]any{
					"openid":   binding.OpenID,
					"user_id":  binding.UserID,
					"username": user.Username,
					"summary":  summary,
					"keywords": keywords,
				}).
				Post(w.cfg.WechatPushWebhookURL)
			if postErr != nil {
				return postErr
			}
			detail["webhook_status"] = resp.StatusCode()
		}
		detailJSON, marshalErr := json.Marshal(detail)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err := store.CreateAuditLog(ctx, model.AuditLog{
			UserID:     binding.UserID,
			Username:   user.Username,
			Action:     "wechat.daily_push",
			Resource:   "/scheduler/wechat/daily-push",
			DetailJSON: string(detailJSON),
		}); err != nil {
			return err
		}
	}
	log.Info().Int("bindings", len(bindings)).Int("keywords", len(keywords)).Msg("wechat daily push completed")
	return nil
}

var errStoreDisabled = errors.New("scheduler store disabled")

func (w *Worker) ensureStore() (*sqlitestore.Store, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.store != nil {
		return w.store, nil
	}
	if strings.TrimSpace(w.cfg.DatabasePath) == "" {
		return nil, errStoreDisabled
	}
	store, err := sqlitestore.New(w.cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	w.store = store
	return w.store, nil
}

func (w *Worker) fetchHotKeywords() ([]model.SearchWordStat, error) {
	var envelope struct {
		Data []model.SearchWordStat `json:"data"`
	}
	resp, err := w.client.R().
		SetResult(&envelope).
		Get(w.cfg.ContentURL + "/api/v1/search/hot-keywords?limit=10")
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("hot keyword request failed: %s", resp.Status())
	}
	return envelope.Data, nil
}
