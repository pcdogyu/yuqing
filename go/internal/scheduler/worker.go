package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/external"
	"github.com/pcdogyu/yuqing/go/internal/model"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

type Worker struct {
	cfg         config.Config
	client      *resty.Client
	crawlClient *resty.Client
	store       *sqlitestore.Store
	mu          sync.Mutex
}

func NewWorker(cfg config.Config) *Worker {
	crawlTimeout := cfg.SchedulerCrawlTimeout
	if crawlTimeout <= 0 {
		crawlTimeout = maxDuration(cfg.HTTPTimeout*6, cfg.HTTPTimeout)
	}
	return &Worker{
		cfg: cfg,
		client: resty.New().
			SetTimeout(cfg.HTTPTimeout).
			SetRetryCount(cfg.ExternalRetryCount).
			SetRetryWaitTime(cfg.ExternalRetryWait).
			SetRetryMaxWaitTime(maxDuration(cfg.ExternalRetryWait*6, cfg.ExternalRetryWait)).
			AddRetryCondition(external.ShouldRetryResponse).
			SetHeader("X-Service-Token", cfg.ServiceToken),
		crawlClient: resty.New().
			SetTimeout(crawlTimeout).
			SetHeader("X-Service-Token", cfg.ServiceToken),
	}
}

func maxDuration(value, fallback time.Duration) time.Duration {
	if value > fallback {
		return value
	}
	return fallback
}

func (w *Worker) Run(ctx context.Context) {
	w.waitForDependencies(ctx)

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Warn().Err(err).Msg("load scheduler timezone failed, falling back to local timezone")
		location = time.Local
	}
	runner := cron.New(
		cron.WithLocation(location),
		cron.WithParser(cronParser()),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger), cron.Recover(cron.DefaultLogger)),
	)
	for _, job := range w.jobDefinitions() {
		if !job.Enabled {
			log.Info().Str("service", "scheduler-service").Str("task", job.Name).Msg("scheduler task disabled")
			continue
		}
		current := job
		if _, err := runner.AddFunc(quartzCronSpec(current.Cron), func() {
			if err := w.runJob(ctx, current); err != nil {
				log.Error().Err(err).Str("task", current.Name).Msg("scheduler cron task failed")
			}
		}); err != nil {
			log.Error().Err(err).Str("service", "scheduler-service").Str("task", current.Name).Str("cron", current.Cron).Msg("scheduler cron registration failed")
			continue
		}
		log.Info().Str("service", "scheduler-service").Str("task", current.Name).Str("cron", current.Cron).Msg("scheduler cron task registered")
	}
	runner.Start()
	log.Info().Str("service", "scheduler-service").Msg("scheduler service ready")
	<-ctx.Done()
	stopCtx := runner.Stop()
	select {
	case <-stopCtx.Done():
	case <-time.After(5 * time.Second):
		log.Warn().Msg("scheduler cron shutdown timed out")
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

func (w *Worker) waitForDependencies(ctx context.Context) {
	dependencies := []struct {
		name string
		url  string
	}{
		{name: "crawler-service", url: w.cfg.CrawlerURL + "/healthz"},
		{name: "analysis-service", url: w.cfg.AnalysisURL + "/healthz"},
		{name: "content-service", url: w.cfg.ContentURL + "/healthz"},
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

func (w *Worker) request(method, url string, body any) error {
	req := w.client.R()
	if body != nil {
		req.SetBody(body)
	}
	var (
		resp *resty.Response
		err  error
	)
	switch method {
	case http.MethodGet:
		resp, err = req.Get(url)
	case http.MethodPost:
		resp, err = req.Post(url)
	default:
		return fmt.Errorf("unsupported scheduler request method %s", method)
	}
	if err != nil {
		if code := external.Classify(err); code != "" {
			return external.New(code, err.Error())
		}
		return err
	}
	if !resp.IsSuccess() {
		return external.HTTPStatus(fmt.Sprintf("%s %s failed: %s", method, url, resp.Status()), resp.StatusCode())
	}
	return nil
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
	if w.cfg.DatabaseDriver != "postgres" && strings.TrimSpace(w.cfg.DatabasePath) == "" {
		return nil, errStoreDisabled
	}
	store, err := app.NewStore(w.cfg)
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
