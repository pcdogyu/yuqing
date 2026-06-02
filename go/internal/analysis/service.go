package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type Store interface {
	Overview(ctx context.Context) (model.Overview, error)
	BuildDashboardSnapshot(ctx context.Context) (model.DashboardSnapshot, error)
	RefreshAnalysis(ctx context.Context) (model.DashboardSnapshot, error)
	GetAnalysisSnapshot(ctx context.Context, scope string, scopeID int64) (model.AnalysisSnapshot, error)
	ListTrendPoints(ctx context.Context) ([]model.TrendPoint, error)
	ListSourceBreakdowns(ctx context.Context) ([]model.SourceBreakdown, error)
	ListKeywordHotspots(ctx context.Context) ([]model.KeywordHotspot, error)
	BuildEmotionAnalysis(ctx context.Context, projectID int64) (model.EmotionAnalysis, error)
	BuildEventOverview(ctx context.Context, projectID int64) ([]model.EventOverview, error)
	BuildPropagationAnalysis(ctx context.Context, projectID int64) (model.PropagationAnalysis, error)
	BuildThemeInsights(ctx context.Context, projectID int64) ([]model.ThemeInsight, error)
	BuildPublicOpinionEvents(ctx context.Context, projectID int64) ([]model.PublicOpinionEvent, error)
	BuildPublicOpinionReports(ctx context.Context, projectID int64) ([]model.PublicOpinionReport, error)
	RecordTaskRun(ctx context.Context, name, status, message string, startedAt time.Time, finishedAt *time.Time) error
}

type Service struct {
	cfg   config.Config
	store Store
}

func NewService(cfg config.Config, store Store) *Service {
	return &Service{cfg: cfg, store: store}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Get("/api/v1/analysis/overview", s.handleOverview)
	r.Get("/api/v1/analysis/trends", s.handleTrends)
	r.Get("/api/v1/analysis/sources", s.handleSources)
	r.Get("/api/v1/analysis/keywords", s.handleKeywords)
	r.Get("/api/v1/analysis/emotions", s.handleEmotions)
	r.Get("/api/v1/analysis/event-overview", s.handleEventOverview)
	r.Get("/api/v1/analysis/propagation", s.handlePropagation)
	r.Get("/api/v1/analysis/themes", s.handleThemes)
	r.Get("/api/v1/public-opinion/events", s.handlePublicOpinionEvents)
	r.Get("/api/v1/public-opinion/reports", s.handlePublicOpinionReports)
	r.Post("/api/v1/admin/tasks/analysis/refresh", s.handleRefresh)
	return r
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
}

func (s *Service) handleOverview(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.store.GetAnalysisSnapshot(r.Context(), "system", 0)
	if err == nil {
		var payload any
		_ = json.Unmarshal([]byte(snapshot.Payload), &payload)
		apiutil.WriteJSON(w, http.StatusOK, "ok", payload)
		return
	}
	live, err := s.store.BuildDashboardSnapshot(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", live)
}

func (s *Service) handleTrends(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListTrendPoints(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleSources(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListSourceBreakdowns(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleKeywords(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListKeywordHotspots(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleEmotions(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildEmotionAnalysis(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleEventOverview(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildEventOverview(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handlePropagation(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildPropagationAnalysis(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleThemes(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildThemeInsights(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handlePublicOpinionEvents(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildPublicOpinionEvents(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handlePublicOpinionReports(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildPublicOpinionReports(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Service-Token") != s.cfg.ServiceToken {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	startedAt := time.Now().UTC()
	data, err := s.store.RefreshAnalysis(r.Context())
	if err != nil {
		finishedAt := time.Now().UTC()
		_ = s.store.RecordTaskRun(r.Context(), "analysis:refresh", "failed", err.Error(), startedAt, &finishedAt)
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	finishedAt := time.Now().UTC()
	_ = s.store.RecordTaskRun(r.Context(), "analysis:refresh", "success", "analysis refreshed", startedAt, &finishedAt)
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func queryProjectID(r *http.Request) int64 {
	value := r.URL.Query().Get("project_id")
	if value == "" {
		return 0
	}
	projectID, _ := strconv.ParseInt(value, 10, 64)
	return projectID
}
