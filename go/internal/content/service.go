package content

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type Store interface {
	ListProjects(context.Context) ([]model.Project, error)
	CreateProject(context.Context, model.Project) (model.Project, error)
	ListMonitorRules(context.Context) ([]model.MonitorRule, error)
	CreateMonitorRule(context.Context, model.MonitorRule) (model.MonitorRule, error)
	ListItems(context.Context, int, int, string, string) (model.ItemListResult, error)
	GetItem(context.Context, int64) (model.Item, error)
	SearchItemsFTS(context.Context, string, int, int) (model.SearchResult, error)
	Overview(context.Context) (model.Overview, error)
	ListReports(context.Context) ([]model.Report, error)
	CreateReport(context.Context, model.Report) (model.Report, error)
	ListNotices(context.Context) ([]model.SystemNotice, error)
	BuildOverviewSnapshot(context.Context) (model.AnalysisSnapshot, error)
	UpsertAnalysisSnapshot(context.Context, model.AnalysisSnapshot) error
	GetAnalysisSnapshot(context.Context, string, int64) (model.AnalysisSnapshot, error)
}

type Service struct {
	cfg    config.Config
	store  Store
	client *resty.Client
}

func NewService(cfg config.Config, store Store) *Service {
	return &Service{
		cfg:   cfg,
		store: store,
		client: resty.New().
			SetTimeout(cfg.HTTPTimeout).
			SetHeader("X-Service-Token", cfg.ServiceToken),
	}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	s.Routes(r)
	return r
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/api/v1/projects", s.handleListProjects)
	r.Post("/api/v1/projects", s.handleCreateProject)
	r.Get("/api/v1/monitors", s.handleListRules)
	r.Post("/api/v1/monitors", s.handleCreateRule)
	r.Get("/api/v1/articles", s.handleListArticles)
	r.Get("/api/v1/articles/{id}", s.handleGetArticle)
	r.Get("/api/v1/search/articles", s.handleSearchArticles)
	r.Get("/api/v1/analysis/overview", s.handleOverview)
	r.Post("/api/v1/admin/tasks/analysis/refresh", s.handleRefreshAnalysis)
	r.Get("/api/v1/reports", s.handleListReports)
	r.Post("/api/v1/reports/generate", s.handleGenerateReport)
	r.Get("/api/v1/system/notices", s.handleListNotices)
	r.Get("/api/v1/public-opinion/overview", s.handlePublicOpinion)
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
}

func (s *Service) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", projects)
}

func (s *Service) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var project model.Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	if strings.TrimSpace(project.Name) == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "project name required", nil)
		return
	}
	created, err := s.store.CreateProject(r.Context(), project)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListMonitorRules(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", rules)
}

func (s *Service) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var rule model.MonitorRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	created, err := s.store.CreateMonitorRule(r.Context(), rule)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleListArticles(w http.ResponseWriter, r *http.Request) {
	page := apiutil.IntQuery(r, "page", 1)
	pageSize := apiutil.IntQuery(r, "page_size", 20)
	sourceType := r.URL.Query().Get("source_type")
	keyword := r.URL.Query().Get("keyword")
	result, err := s.store.ListItems(r.Context(), page, pageSize, keyword, sourceType)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleGetArticle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid id", nil)
		return
	}
	item, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", item)
}

func (s *Service) handleSearchArticles(w http.ResponseWriter, r *http.Request) {
	page := apiutil.IntQuery(r, "page", 1)
	pageSize := apiutil.IntQuery(r, "page_size", 20)
	keyword := r.URL.Query().Get("q")
	result, err := s.store.SearchItemsFTS(r.Context(), keyword, page, pageSize)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleOverview(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.store.GetAnalysisSnapshot(r.Context(), "system", 0)
	if err == nil {
		var payload any
		_ = json.Unmarshal([]byte(snapshot.Payload), &payload)
		apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
			"snapshot":   payload,
			"created_at": snapshot.CreatedAt,
		})
		return
	}
	overview, err := s.store.Overview(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", overview)
}

func (s *Service) handleRefreshAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Service-Token") != s.cfg.ServiceToken {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	snapshot, err := s.store.BuildOverviewSnapshot(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if err := s.store.UpsertAnalysisSnapshot(r.Context(), snapshot); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", snapshot)
}

func (s *Service) handleListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := s.store.ListReports(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", reports)
}

func (s *Service) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int64  `json:"project_id"`
		Title     string `json:"title"`
		Text      string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	report := model.Report{
		ProjectID: req.ProjectID,
		Title:     nonEmpty(req.Title, "自动生成报告"),
		Summary:   summarizeText(req.Text),
		Content:   req.Text,
		Status:    "generated",
	}
	if strings.TrimSpace(req.Text) != "" {
		nlpResp, err := s.client.R().
			SetBody(map[string]string{"text": req.Text}).
			SetResult(&struct {
				Code int               `json:"code"`
				Data model.NLPResponse `json:"data"`
			}{}).
			Post(s.cfg.NLPURL + "/api/v1/nlp/summarize")
		if err == nil && nlpResp.IsSuccess() {
			var parsed struct {
				Code int               `json:"code"`
				Data model.NLPResponse `json:"data"`
			}
			_ = json.Unmarshal(nlpResp.Body(), &parsed)
			report.Summary = nonEmpty(parsed.Data.Summary, report.Summary)
			report.Title = nonEmpty(parsed.Data.Title, report.Title)
		}
	}
	created, err := s.store.CreateReport(r.Context(), report)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleListNotices(w http.ResponseWriter, r *http.Request) {
	notices, err := s.store.ListNotices(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", notices)
}

func (s *Service) handlePublicOpinion(w http.ResponseWriter, r *http.Request) {
	overview, err := s.store.Overview(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"headline":      "Go 重构版舆情总览",
		"article_count": overview.ArticleCount,
		"report_count":  overview.ReportCount,
		"updated_at":    time.Now().UTC(),
	})
}

func summarizeText(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= 140 {
		return text
	}
	return string([]rune(text)[:140]) + "..."
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
