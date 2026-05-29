package content

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type Store interface {
	ListProjectGroups(rctx context.Context) ([]model.ProjectGroup, error)
	GetProjectGroup(rctx context.Context, id int64) (model.ProjectGroup, error)
	CreateProjectGroup(rctx context.Context, group model.ProjectGroup) (model.ProjectGroup, error)
	UpdateProjectGroup(rctx context.Context, group model.ProjectGroup) (model.ProjectGroup, error)
	DeleteProjectGroup(rctx context.Context, id int64) error
	ListProjects(rctx context.Context) ([]model.Project, error)
	GetProject(rctx context.Context, id int64) (model.Project, error)
	CreateProject(rctx context.Context, project model.Project) (model.Project, error)
	UpdateProject(rctx context.Context, project model.Project) (model.Project, error)
	DeleteProject(rctx context.Context, id int64) error
	ListMonitorRules(rctx context.Context) ([]model.MonitorRule, error)
	GetMonitorRule(rctx context.Context, id int64) (model.MonitorRule, error)
	CreateMonitorRule(rctx context.Context, rule model.MonitorRule) (model.MonitorRule, error)
	UpdateMonitorRule(rctx context.Context, rule model.MonitorRule) (model.MonitorRule, error)
	DeleteMonitorRule(rctx context.Context, id int64) error
	ListItems(rctx context.Context, filter model.ArticleFilter) (model.ItemListResult, error)
	GetItem(rctx context.Context, id int64) (model.Item, error)
	GetRelatedItems(rctx context.Context, id int64, limit int) ([]model.Item, error)
	PopulateUserItemState(rctx context.Context, userID int64, items []model.Item) error
	SearchItemsFTS(rctx context.Context, filter model.ArticleFilter) (model.SearchResult, error)
	MarkItemRead(rctx context.Context, userID, itemID int64) error
	ToggleFavorite(rctx context.Context, userID, itemID int64) (bool, error)
	ListReports(rctx context.Context, projectID int64) ([]model.Report, error)
	GetReport(rctx context.Context, id int64) (model.Report, error)
	CreateReport(rctx context.Context, report model.Report) (model.Report, error)
	ListNotices(rctx context.Context) ([]model.SystemNotice, error)
	CreateFeedback(rctx context.Context, feedback model.Feedback) (model.Feedback, error)
	ListTaskRuns(rctx context.Context, limit int) ([]model.TaskRun, error)
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
	r.Get("/healthz", s.handleHealthz)

	r.Get("/api/v1/project-groups", s.handleListProjectGroups)
	r.Post("/api/v1/project-groups", s.handleCreateProjectGroup)
	r.Get("/api/v1/project-groups/{id}", s.handleGetProjectGroup)
	r.Put("/api/v1/project-groups/{id}", s.handleUpdateProjectGroup)
	r.Delete("/api/v1/project-groups/{id}", s.handleDeleteProjectGroup)

	r.Get("/api/v1/projects", s.handleListProjects)
	r.Post("/api/v1/projects", s.handleCreateProject)
	r.Get("/api/v1/projects/{id}", s.handleGetProject)
	r.Put("/api/v1/projects/{id}", s.handleUpdateProject)
	r.Delete("/api/v1/projects/{id}", s.handleDeleteProject)

	r.Get("/api/v1/monitor-rules", s.handleListRules)
	r.Post("/api/v1/monitor-rules", s.handleCreateRule)
	r.Get("/api/v1/monitor-rules/{id}", s.handleGetRule)
	r.Put("/api/v1/monitor-rules/{id}", s.handleUpdateRule)
	r.Delete("/api/v1/monitor-rules/{id}", s.handleDeleteRule)

	r.Get("/api/v1/articles", s.handleListArticles)
	r.Get("/api/v1/articles/{id}", s.handleGetArticle)
	r.Get("/api/v1/articles/{id}/related", s.handleGetRelatedArticles)
	r.Post("/api/v1/articles/{id}/read", s.handleMarkArticleRead)
	r.Post("/api/v1/articles/{id}/favorite", s.handleToggleFavorite)
	r.Get("/api/v1/search/articles", s.handleSearchArticles)

	r.Get("/api/v1/reports", s.handleListReports)
	r.Post("/api/v1/reports", s.handleCreateReport)
	r.Get("/api/v1/reports/{id}", s.handleGetReport)
	r.Post("/api/v1/reports/generate", s.handleGenerateReport)

	r.Get("/api/v1/system/notices", s.handleListNotices)
	r.Post("/api/v1/system/feedback", s.handleCreateFeedback)
	r.Get("/api/v1/system/task-runs", s.handleListTaskRuns)
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
}

func (s *Service) handleListProjectGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListProjectGroups(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", groups)
}

func (s *Service) handleGetProjectGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	group, err := s.store.GetProjectGroup(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", group)
}

func (s *Service) handleCreateProjectGroup(w http.ResponseWriter, r *http.Request) {
	var group model.ProjectGroup
	if !decodeJSON(w, r, &group) {
		return
	}
	if strings.TrimSpace(group.Name) == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "group name required", nil)
		return
	}
	created, err := s.store.CreateProjectGroup(r.Context(), group)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleUpdateProjectGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var group model.ProjectGroup
	if !decodeJSON(w, r, &group) {
		return
	}
	group.ID = id
	updated, err := s.store.UpdateProjectGroup(r.Context(), group)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleDeleteProjectGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteProjectGroup(r.Context(), id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"deleted": true})
}

func (s *Service) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", projects)
}

func (s *Service) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	project, err := s.store.GetProject(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", project)
}

func (s *Service) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var project model.Project
	if !decodeJSON(w, r, &project) {
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

func (s *Service) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var project model.Project
	if !decodeJSON(w, r, &project) {
		return
	}
	project.ID = id
	updated, err := s.store.UpdateProject(r.Context(), project)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteProject(r.Context(), id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"deleted": true})
}

func (s *Service) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListMonitorRules(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", rules)
}

func (s *Service) handleGetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	rule, err := s.store.GetMonitorRule(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", rule)
}

func (s *Service) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var rule model.MonitorRule
	if !decodeJSON(w, r, &rule) {
		return
	}
	if rule.ProjectID <= 0 || strings.TrimSpace(rule.Name) == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "project_id and name required", nil)
		return
	}
	created, err := s.store.CreateMonitorRule(r.Context(), rule)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var rule model.MonitorRule
	if !decodeJSON(w, r, &rule) {
		return
	}
	rule.ID = id
	updated, err := s.store.UpdateMonitorRule(r.Context(), rule)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteMonitorRule(r.Context(), id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"deleted": true})
}

func (s *Service) handleListArticles(w http.ResponseWriter, r *http.Request) {
	filter := articleFilterFromRequest(r)
	result, err := s.store.ListItems(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleGetArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	if userID := filterUserID(r); userID > 0 {
		items := []model.Item{item}
		_ = s.store.PopulateUserItemState(r.Context(), userID, items)
		item = items[0]
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", item)
}

func (s *Service) handleGetRelatedArticles(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.store.GetRelatedItems(r.Context(), id, apiutil.IntQuery(r, "limit", 5))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if userID := filterUserID(r); userID > 0 {
		_ = s.store.PopulateUserItemState(r.Context(), userID, items)
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", items)
}

func (s *Service) handleMarkArticleRead(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	if err := s.store.MarkItemRead(r.Context(), userID, id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"read": true})
}

func (s *Service) handleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	favorited, err := s.store.ToggleFavorite(r.Context(), userID, id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"favorited": favorited})
}

func (s *Service) handleSearchArticles(w http.ResponseWriter, r *http.Request) {
	filter := articleFilterFromRequest(r)
	filter.Keyword = r.URL.Query().Get("q")
	result, err := s.store.SearchItemsFTS(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListReports(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("project_id")), 10, 64)
	reports, err := s.store.ListReports(r.Context(), projectID)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", reports)
}

func (s *Service) handleGetReport(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	report, err := s.store.GetReport(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", report)
}

func (s *Service) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	var report model.Report
	if !decodeJSON(w, r, &report) {
		return
	}
	if strings.TrimSpace(report.Title) == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "report title required", nil)
		return
	}
	created, err := s.store.CreateReport(r.Context(), report)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int64  `json:"project_id"`
		Title     string `json:"title"`
		Text      string `json:"text"`
	}
	if !decodeJSON(w, r, &req) {
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

func (s *Service) handleCreateFeedback(w http.ResponseWriter, r *http.Request) {
	var feedback model.Feedback
	if !decodeJSON(w, r, &feedback) {
		return
	}
	if strings.TrimSpace(feedback.Title) == "" || strings.TrimSpace(feedback.Content) == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "title and content required", nil)
		return
	}
	created, err := s.store.CreateFeedback(r.Context(), feedback)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleListTaskRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListTaskRuns(r.Context(), apiutil.IntQuery(r, "limit", 20))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", runs)
}

func articleFilterFromRequest(r *http.Request) model.ArticleFilter {
	projectID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("project_id")), 10, 64)
	return model.ArticleFilter{
		Page:       apiutil.IntQuery(r, "page", 1),
		PageSize:   apiutil.IntQuery(r, "page_size", 20),
		Keyword:    strings.TrimSpace(r.URL.Query().Get("keyword")),
		SourceType: strings.TrimSpace(r.URL.Query().Get("source_type")),
		ProjectID:  projectID,
		UserID:     filterUserID(r),
		Start:      strings.TrimSpace(r.URL.Query().Get("start")),
		End:        strings.TrimSpace(r.URL.Query().Get("end")),
	}
}

func filterUserID(r *http.Request) int64 {
	userID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("user_id")), 10, 64)
	return userID
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return false
	}
	return true
}

func parseID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil || id <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid id", nil)
		return 0, false
	}
	return id, true
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
