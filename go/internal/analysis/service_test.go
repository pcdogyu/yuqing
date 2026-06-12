package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type stubStore struct {
	snapshot       model.AnalysisSnapshot
	snapshotErr    error
	cryptoSnapshot model.CryptoInsightSnapshot
	cryptoSnapErr  error
	dashboard      model.DashboardSnapshot
	dashboardErr   error
	trends         []model.TrendPoint
	trendsErr      error
	sources        []model.SourceBreakdown
	sourcesErr     error
	keywords       []model.KeywordHotspot
	keywordsErr    error
	emotions       model.EmotionAnalysis
	emotionsErr    error
	events         []model.EventOverview
	eventsErr      error
	propagation    model.PropagationAnalysis
	propagationErr error
	themes         []model.ThemeInsight
	themesErr      error
	opinionEvents  []model.PublicOpinionEvent
	opinionEvtErr  error
	opinionReports []model.PublicOpinionReport
	opinionRptErr  error
	refresh        model.DashboardSnapshot
	refreshErr     error
	recordTaskRuns []string
}

func (s *stubStore) Overview(context.Context) (model.Overview, error) {
	return model.Overview{}, nil
}

func (s *stubStore) BuildDashboardSnapshot(context.Context) (model.DashboardSnapshot, error) {
	return s.dashboard, s.dashboardErr
}

func (s *stubStore) RefreshAnalysis(context.Context) (model.DashboardSnapshot, error) {
	return s.refresh, s.refreshErr
}

func (s *stubStore) GetAnalysisSnapshot(context.Context, string, int64) (model.AnalysisSnapshot, error) {
	return s.snapshot, s.snapshotErr
}

func (s *stubStore) ListTrendPoints(context.Context) ([]model.TrendPoint, error) {
	return s.trends, s.trendsErr
}

func (s *stubStore) ListSourceBreakdowns(context.Context) ([]model.SourceBreakdown, error) {
	return s.sources, s.sourcesErr
}

func (s *stubStore) ListKeywordHotspots(context.Context) ([]model.KeywordHotspot, error) {
	return s.keywords, s.keywordsErr
}

func (s *stubStore) BuildEmotionAnalysis(context.Context, int64) (model.EmotionAnalysis, error) {
	return s.emotions, s.emotionsErr
}

func (s *stubStore) BuildEventOverview(context.Context, int64) ([]model.EventOverview, error) {
	return s.events, s.eventsErr
}

func (s *stubStore) BuildPropagationAnalysis(context.Context, int64) (model.PropagationAnalysis, error) {
	return s.propagation, s.propagationErr
}

func (s *stubStore) BuildThemeInsights(context.Context, int64) ([]model.ThemeInsight, error) {
	return s.themes, s.themesErr
}

func (s *stubStore) BuildPublicOpinionEvents(context.Context, int64) ([]model.PublicOpinionEvent, error) {
	return s.opinionEvents, s.opinionEvtErr
}

func (s *stubStore) BuildPublicOpinionReports(context.Context, int64) ([]model.PublicOpinionReport, error) {
	return s.opinionReports, s.opinionRptErr
}

func (s *stubStore) UpsertCryptoCandles(context.Context, string, string, []model.CryptoPriceCandle) error {
	return nil
}

func (s *stubStore) ListCryptoCandles(context.Context, string, string, int) ([]model.CryptoPriceCandle, error) {
	return nil, nil
}

func (s *stubStore) GetCryptoInsightSnapshot(context.Context, string, string) (model.CryptoInsightSnapshot, error) {
	return s.cryptoSnapshot, s.cryptoSnapErr
}

func (s *stubStore) UpsertCryptoInsightSnapshot(context.Context, model.CryptoInsightSnapshot) error {
	return nil
}

func (s *stubStore) RecordTaskRun(_ context.Context, name, status, _ string, _ time.Time, _ *time.Time) error {
	s.recordTaskRuns = append(s.recordTaskRuns, name+":"+status)
	return nil
}

func TestHandleOverviewUsesSnapshotWhenAvailable(t *testing.T) {
	store := &stubStore{
		snapshot: model.AnalysisSnapshot{Payload: `{"overview":{"article_count":3}}`},
	}
	svc := NewService(config.Config{}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/overview", nil)

	svc.handleOverview(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := payload.Data["overview"]; !ok {
		t.Fatalf("expected snapshot payload, got %+v", payload.Data)
	}
}

func TestHandleOverviewFallsBackToLiveSnapshot(t *testing.T) {
	store := &stubStore{
		snapshotErr: errors.New("not found"),
		dashboard: model.DashboardSnapshot{
			Overview: model.Overview{ArticleCount: 9},
		},
	}
	svc := NewService(config.Config{}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/overview", nil)

	svc.handleOverview(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload struct {
		Data model.DashboardSnapshot `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload.Data.Overview.ArticleCount != 9 {
		t.Fatalf("expected live dashboard response, got %+v", payload.Data)
	}
}

func TestHandleRefreshRequiresServiceToken(t *testing.T) {
	svc := NewService(config.Config{ServiceToken: "secret"}, &stubStore{})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tasks/analysis/refresh", nil)

	svc.handleRefresh(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
}

func TestHandleRefreshRecordsSuccess(t *testing.T) {
	store := &stubStore{
		refresh: model.DashboardSnapshot{Overview: model.Overview{ArticleCount: 5}},
	}
	svc := NewService(config.Config{ServiceToken: "secret"}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tasks/analysis/refresh", nil)
	req.Header.Set("X-Service-Token", "secret")

	svc.handleRefresh(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if len(store.recordTaskRuns) != 1 || store.recordTaskRuns[0] != "analysis:refresh:success" {
		t.Fatalf("expected success task run recorded, got %+v", store.recordTaskRuns)
	}
}

func TestHandlePublicOpinionEnrich(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/full" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := strings.TrimSpace(r.URL.Query().Get("q")); got != "AI,大模型" {
			t.Fatalf("expected keywords query, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": http.StatusOK,
			"data": model.SearchResult{
				Items: []model.Item{
					{ID: 1, Title: "AI 热点一", Content: "市场情绪升温", Summary: "摘要一", SourceType: "headline", FromText: "新闻", PublishTimeText: "2026-06-01 10:00:00"},
					{ID: 2, Title: "无关词 命中", Content: "这条应该被过滤", Summary: "摘要二", SourceType: "weibo", FromText: "微博", PublishTimeText: "2026-06-01 11:00:00"},
				},
			},
		})
	}))
	defer content.Close()

	svc := NewService(config.Config{ContentURL: content.URL}, &stubStore{})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public-opinion/enrich?"+url.Values{
		"eventname":      {"AI 舆情"},
		"eventkeywords":  {"AI,大模型"},
		"eventstopwords": {"无关词"},
		"eventstarttime": {"2026-06-01"},
		"eventendtime":   {"2026-06-04"},
	}.Encode(), nil)

	svc.handlePublicOpinionEnrich(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload struct {
		Data model.PublicOpinionAnalysisBundle `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.ArticleCount != 1 {
		t.Fatalf("expected filtered article count 1, got %+v", payload.Data)
	}
	if !strings.Contains(payload.Data.BackAnalysis, "AI 热点一") {
		t.Fatalf("expected back analysis to include kept article, got %s", payload.Data.BackAnalysis)
	}
	if payload.Data.EventStartTime != "2026-06-01 00:00:00" || payload.Data.EventEndTime != "2026-06-04 23:59:59" {
		t.Fatalf("expected normalized time range, got %+v", payload.Data)
	}
}
