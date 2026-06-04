package crawlerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/service"
)

func TestValidSourceType(t *testing.T) {
	if got := validSourceType("flash"); got != "flash" {
		t.Fatalf("expected flash, got %q", got)
	}
	if got := validSourceType("headline"); got != "headline" {
		t.Fatalf("expected headline, got %q", got)
	}
	if got := validSourceType("other"); got != "" {
		t.Fatalf("expected empty source type, got %q", got)
	}
}

func TestRouterHealthz(t *testing.T) {
	svc := &Service{}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	svc.Router().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestTemplateEndpoints(t *testing.T) {
	listing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><article class="item"><a class="title" href="/detail/1">模板文章</a><p class="summary">模板摘要</p></article></body></html>`))
	}))
	defer listing.Close()

	st := &templateCrawlerStore{
		activeRules: []model.MonitorRule{},
	}
	crawler := service.NewCrawler(st, provider.Registry{}, resty.New())
	svc := NewService(config.Config{ServiceToken: "token"}, crawler)

	templateJSON := map[string]any{
		"source_type":      "flash",
		"base_url":         listing.URL,
		"method":           "GET",
		"list_selector":    ".item",
		"detail_url_field": "href",
		"fields": []map[string]any{
			{"name": "title", "selector": ".title", "scope": "list", "required": true},
			{"name": "summary", "selector": ".summary", "scope": "list"},
		},
	}
	body, _ := json.Marshal(map[string]any{
		"id":          1,
		"name":        "模板一",
		"source_type": "flash",
		"enabled":     true,
		"config_json": string(mustJSON(templateJSON)),
	})

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tasks/crawl/templates/run", bytes.NewReader(body))
	runReq.Header.Set("X-Service-Token", "token")
	runReq.Header.Set("Content-Type", "application/json")
	runRR := httptest.NewRecorder()
	svc.handleRunTemplate(runRR, runReq)
	if runRR.Code != http.StatusOK {
		t.Fatalf("expected run template success, got %d: %s", runRR.Code, runRR.Body.String())
	}

	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tasks/crawl/templates/preview", bytes.NewReader(body))
	previewReq.Header.Set("X-Service-Token", "token")
	previewReq.Header.Set("Content-Type", "application/json")
	previewRR := httptest.NewRecorder()
	svc.handlePreviewTemplate(previewRR, previewReq)
	if previewRR.Code != http.StatusOK {
		t.Fatalf("expected preview success, got %d: %s", previewRR.Code, previewRR.Body.String())
	}
}

type templateCrawlerStore struct {
	activeRules []model.MonitorRule
	items       []model.Item
}

func (s *templateCrawlerStore) StartCrawlRun(context.Context, string, time.Time) (int64, error) {
	return 1, nil
}

func (s *templateCrawlerStore) StartCrawlTemplateRun(context.Context, string, int64, string, string, time.Time) (int64, error) {
	return 1, nil
}

func (s *templateCrawlerStore) FinishCrawlRun(context.Context, int64, string, int, int, int, string, time.Time) error {
	return nil
}

func (s *templateCrawlerStore) UpsertItems(context.Context, []model.Item) (int, int, error) {
	return 1, 0, nil
}

func (s *templateCrawlerStore) ListItems(context.Context, model.ArticleFilter) (model.ItemListResult, error) {
	return model.ItemListResult{}, nil
}

func (s *templateCrawlerStore) LatestItems(context.Context, int, string) ([]model.Item, error) {
	return nil, nil
}

func (s *templateCrawlerStore) GetItem(context.Context, int64) (model.Item, error) {
	return model.Item{}, nil
}

func (s *templateCrawlerStore) GetRelatedItems(context.Context, int64, int) ([]model.Item, error) {
	return nil, nil
}

func (s *templateCrawlerStore) ListCrawlRuns(context.Context, int, string) ([]model.CrawlRun, error) {
	return nil, nil
}

func (s *templateCrawlerStore) ListActiveMonitorRules(context.Context) ([]model.MonitorRule, error) {
	return s.activeRules, nil
}

func (s *templateCrawlerStore) GetCrawlTemplate(context.Context, int64) (model.CrawlTemplate, error) {
	return model.CrawlTemplate{}, nil
}

func (s *templateCrawlerStore) ListCrawlTemplates(context.Context) ([]model.CrawlTemplate, error) {
	return nil, nil
}

func (s *templateCrawlerStore) LinkItemsToProjects(context.Context, []string, []int64, int64) error {
	return nil
}

func (s *templateCrawlerStore) RecordTaskRun(context.Context, string, string, string, time.Time, *time.Time) error {
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
