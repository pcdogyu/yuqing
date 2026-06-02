package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type stubStore struct {
	snapshot       model.AnalysisSnapshot
	snapshotErr    error
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
