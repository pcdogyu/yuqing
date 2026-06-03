package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
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
	ListCrawlTemplates(rctx context.Context) ([]model.CrawlTemplate, error)
	GetCrawlTemplate(rctx context.Context, id int64) (model.CrawlTemplate, error)
	CreateCrawlTemplate(rctx context.Context, tpl model.CrawlTemplate) (model.CrawlTemplate, error)
	UpdateCrawlTemplate(rctx context.Context, tpl model.CrawlTemplate) (model.CrawlTemplate, error)
	DeleteCrawlTemplate(rctx context.Context, id int64) error
	ListItems(rctx context.Context, filter model.ArticleFilter) (model.ItemListResult, error)
	GetItem(rctx context.Context, id int64) (model.Item, error)
	GetRelatedItems(rctx context.Context, id int64, limit int) ([]model.Item, error)
	PopulateUserItemState(rctx context.Context, userID int64, items []model.Item) error
	SearchItemsFTS(rctx context.Context, filter model.ArticleFilter) (model.SearchResult, error)
	MarkItemRead(rctx context.Context, userID, itemID int64) error
	DeleteItemRead(rctx context.Context, userID, itemID int64) error
	ToggleFavorite(rctx context.Context, userID, itemID int64) (bool, error)
	RecordItemShare(rctx context.Context, share model.ShareRecord) error
	ListReports(rctx context.Context, projectID int64) ([]model.Report, error)
	GetReport(rctx context.Context, id int64) (model.Report, error)
	CreateReport(rctx context.Context, report model.Report) (model.Report, error)
	ListNotices(rctx context.Context) ([]model.SystemNotice, error)
	CreateFeedback(rctx context.Context, feedback model.Feedback) (model.Feedback, error)
	ListTaskRuns(rctx context.Context, limit int) ([]model.TaskRun, error)
	GetUserPreference(rctx context.Context, userID int64) (model.UserPreference, error)
	UpsertUserPreference(rctx context.Context, pref model.UserPreference) (model.UserPreference, error)
	GetPopupState(rctx context.Context, userID int64, key string) (model.PopupState, error)
	UpsertPopupState(rctx context.Context, state model.PopupState) (model.PopupState, error)
	GetMailConfig(rctx context.Context) (model.MailConfig, error)
	UpsertMailConfig(rctx context.Context, cfg model.MailConfig) (model.MailConfig, error)
	GetWarningSetting(rctx context.Context, projectID int64) (model.WarningSetting, error)
	UpsertWarningSetting(rctx context.Context, setting model.WarningSetting) (model.WarningSetting, error)
	GetOpinionCondition(rctx context.Context, projectID int64) (model.OpinionCondition, error)
	UpsertOpinionCondition(rctx context.Context, condition model.OpinionCondition) (model.OpinionCondition, error)
	SearchItemsAdvanced(rctx context.Context, filter model.ArticleFilter) (model.SearchResult, error)
	BuildSearchFacets(rctx context.Context, filter model.ArticleFilter) (model.SearchFacets, error)
	ListSearchOptions(rctx context.Context) (model.SearchOptions, error)
	SaveSearchWord(rctx context.Context, userID int64, searchWord string) error
	ListSearchWords(rctx context.Context, userID int64, limit int) ([]model.SearchWordStat, error)
	ListSearchWordSuggestions(rctx context.Context, userID int64, prefix string, limit int) ([]model.SearchWordStat, error)
	ListHotSearchWords(rctx context.Context, limit int) ([]model.SearchWordStat, error)
	GetPlatformBinding(rctx context.Context, userID int64, kind string) (model.PlatformBinding, error)
	UpsertPlatformBinding(rctx context.Context, binding model.PlatformBinding) (model.PlatformBinding, error)
	ListPublicOptions(rctx context.Context, userID int64, keyword string) ([]model.PublicOption, error)
	GetPublicOption(rctx context.Context, id int64) (model.PublicOption, error)
	CreatePublicOption(rctx context.Context, option model.PublicOption) (model.PublicOption, error)
	UpdatePublicOption(rctx context.Context, option model.PublicOption) (model.PublicOption, error)
	DeletePublicOption(rctx context.Context, id int64) error
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

	r.Get("/api/v1/crawl-templates", s.handleListCrawlTemplates)
	r.Post("/api/v1/crawl-templates", s.handleCreateCrawlTemplate)
	r.Get("/api/v1/crawl-templates/{id}", s.handleGetCrawlTemplate)
	r.Put("/api/v1/crawl-templates/{id}", s.handleUpdateCrawlTemplate)
	r.Delete("/api/v1/crawl-templates/{id}", s.handleDeleteCrawlTemplate)

	r.Get("/api/v1/articles", s.handleListArticles)
	r.Get("/api/v1/articles/{id}", s.handleGetArticle)
	r.Get("/api/v1/articles/{id}/related", s.handleGetRelatedArticles)
	r.Post("/api/v1/articles/{id}/read", s.handleMarkArticleRead)
	r.Delete("/api/v1/articles/{id}/read", s.handleUnmarkArticleRead)
	r.Post("/api/v1/articles/{id}/favorite", s.handleToggleFavorite)
	r.Post("/api/v1/articles/{id}/share", s.handleShareArticle)
	r.Get("/api/v1/search/articles", s.handleSearchArticles)
	r.Get("/api/v1/search/full", s.handleSearchFull)
	r.Get("/api/v1/search/timely", s.handleSearchTimely)
	r.Get("/api/v1/search/full/facets", s.handleSearchFacets)
	r.Get("/api/v1/search/options", s.handleSearchOptions)
	r.Get("/api/v1/search/options/{kind}", s.handleSearchOptionKind)
	r.Get("/api/v1/search/history", s.handleListSearchHistory)
	r.Get("/api/v1/search/suggestions", s.handleSearchSuggestions)
	r.Get("/api/v1/search/hot-keywords", s.handleHotKeywords)
	r.Post("/api/v1/search/history", s.handleSaveSearchHistory)
	r.Get("/api/v1/platform/bindings/{kind}", s.handleGetPlatformBinding)
	r.Post("/api/v1/platform/bindings/{kind}", s.handleUpsertPlatformBinding)
	r.Put("/api/v1/platform/bindings/{kind}", s.handleUpsertPlatformBinding)
	r.Get("/api/v1/public-options", s.handleListPublicOptions)
	r.Post("/api/v1/public-options", s.handleCreatePublicOption)
	r.Get("/api/v1/public-options/{id}", s.handleGetPublicOption)
	r.Put("/api/v1/public-options/{id}", s.handleUpdatePublicOption)
	r.Delete("/api/v1/public-options/{id}", s.handleDeletePublicOption)

	r.Get("/api/v1/reports", s.handleListReports)
	r.Post("/api/v1/reports", s.handleCreateReport)
	r.Get("/api/v1/reports/{id}", s.handleGetReport)
	r.Post("/api/v1/reports/generate", s.handleGenerateReport)

	r.Get("/api/v1/system/notices", s.handleListNotices)
	r.Post("/api/v1/system/feedback", s.handleCreateFeedback)
	r.Get("/api/v1/system/task-runs", s.handleListTaskRuns)
	r.Get("/api/v1/system/popup", s.handleGetPopupState)
	r.Put("/api/v1/system/popup", s.handleUpdatePopupState)
	r.Get("/api/v1/system/preferences", s.handleGetPreferences)
	r.Put("/api/v1/system/preferences", s.handleUpdatePreferences)
	r.Get("/api/v1/system/mail-config", s.handleGetMailConfig)
	r.Put("/api/v1/system/mail-config", s.handleUpdateMailConfig)
	r.Get("/api/v1/system/warning-settings/{project_id}", s.handleGetWarningSetting)
	r.Put("/api/v1/system/warning-settings/{project_id}", s.handleUpdateWarningSetting)
	r.Get("/api/v1/system/opinion-conditions/{project_id}", s.handleGetOpinionCondition)
	r.Put("/api/v1/system/opinion-conditions/{project_id}", s.handleUpdateOpinionCondition)
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
	_, _ = s.store.UpsertPopupState(r.Context(), model.PopupState{
		UserID: 0,
		Key:    "contact-" + strconv.FormatInt(created.ID, 10),
	})
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

func (s *Service) handleListCrawlTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := s.store.ListCrawlTemplates(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", templates)
}

func (s *Service) handleGetCrawlTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	tpl, err := s.store.GetCrawlTemplate(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", tpl)
}

func (s *Service) handleCreateCrawlTemplate(w http.ResponseWriter, r *http.Request) {
	var tpl model.CrawlTemplate
	if !decodeJSON(w, r, &tpl) {
		return
	}
	if strings.TrimSpace(tpl.Name) == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "template name required", nil)
		return
	}
	if strings.TrimSpace(tpl.ConfigJSON) == "" {
		tpl.ConfigJSON = "{}"
	}
	created, err := s.store.CreateCrawlTemplate(r.Context(), tpl)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleUpdateCrawlTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var tpl model.CrawlTemplate
	if !decodeJSON(w, r, &tpl) {
		return
	}
	tpl.ID = id
	if strings.TrimSpace(tpl.ConfigJSON) == "" {
		tpl.ConfigJSON = "{}"
	}
	updated, err := s.store.UpdateCrawlTemplate(r.Context(), tpl)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleDeleteCrawlTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteCrawlTemplate(r.Context(), id); err != nil {
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

func (s *Service) handleUnmarkArticleRead(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	if err := s.store.DeleteItemRead(r.Context(), userID, id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"read": false})
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

func (s *Service) handleShareArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	var req struct {
		Channel string `json:"channel"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.RecordItemShare(r.Context(), model.ShareRecord{
		UserID:  userID,
		ItemID:  id,
		Channel: nonEmpty(req.Channel, "link"),
	}); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{"shared": true, "channel": nonEmpty(req.Channel, "link")})
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

func (s *Service) handleSearchFull(w http.ResponseWriter, r *http.Request) {
	filter := articleFilterFromRequest(r)
	filter.Keyword = nonEmpty(strings.TrimSpace(r.URL.Query().Get("q")), filter.Keyword)
	filter.Mode = "full"
	filter.Sort = nonEmpty(strings.TrimSpace(r.URL.Query().Get("sort")), "captured_at_desc")
	result, err := s.store.SearchItemsAdvanced(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleSearchTimely(w http.ResponseWriter, r *http.Request) {
	filter := articleFilterFromRequest(r)
	filter.Keyword = nonEmpty(strings.TrimSpace(r.URL.Query().Get("q")), filter.Keyword)
	filter.Mode = "timely"
	filter.Sort = nonEmpty(strings.TrimSpace(r.URL.Query().Get("sort")), "captured_at_desc")
	if filter.Start == "" {
		filter.Start = time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02")
	}
	result, err := s.store.SearchItemsAdvanced(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleSearchFacets(w http.ResponseWriter, r *http.Request) {
	filter := articleFilterFromRequest(r)
	filter.Keyword = nonEmpty(strings.TrimSpace(r.URL.Query().Get("q")), filter.Keyword)
	facets, err := s.store.BuildSearchFacets(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", facets)
}

func (s *Service) handleSearchOptions(w http.ResponseWriter, r *http.Request) {
	options, err := s.store.ListSearchOptions(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", options)
}

func (s *Service) handleSearchOptionKind(w http.ResponseWriter, r *http.Request) {
	options, err := s.store.ListSearchOptions(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	switch strings.TrimSpace(chi.URLParam(r, "kind")) {
	case "industry":
		apiutil.WriteJSON(w, http.StatusOK, "ok", options.Industries)
	case "province":
		apiutil.WriteJSON(w, http.StatusOK, "ok", options.Provinces)
	case "city":
		apiutil.WriteJSON(w, http.StatusOK, "ok", options.Cities)
	default:
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported option kind", nil)
	}
}

func (s *Service) handleListSearchHistory(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	limit := apiutil.IntQuery(r, "limit", 6)
	words, err := s.store.ListSearchWords(r.Context(), userID, limit)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", words)
}

func (s *Service) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	limit := apiutil.IntQuery(r, "limit", 10)
	prefix := strings.TrimSpace(r.URL.Query().Get("q"))
	var (
		words []model.SearchWordStat
		err   error
	)
	if prefix == "" {
		words, err = s.store.ListSearchWords(r.Context(), userID, limit)
	} else {
		words, err = s.store.ListSearchWordSuggestions(r.Context(), userID, prefix, limit)
	}
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", words)
}

func (s *Service) handleHotKeywords(w http.ResponseWriter, r *http.Request) {
	limit := apiutil.IntQuery(r, "limit", 10)
	words, err := s.store.ListHotSearchWords(r.Context(), limit)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", words)
}

func (s *Service) handleSaveSearchHistory(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	var req struct {
		SearchWord string `json:"search_word"`
		Searchword string `json:"searchword"`
		Keyword    string `json:"keyword"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
		return
	}
	searchWord := nonEmpty(req.SearchWord, req.Searchword, req.Keyword, r.URL.Query().Get("search_word"), r.URL.Query().Get("searchword"), r.URL.Query().Get("keyword"))
	if err := s.store.SaveSearchWord(r.Context(), userID, searchWord); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"saved": true})
}

func (s *Service) handleGetPlatformBinding(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	kind := strings.TrimSpace(chi.URLParam(r, "kind"))
	if kind == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "kind required", nil)
		return
	}
	binding, err := s.store.GetPlatformBinding(r.Context(), userID, kind)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", binding)
}

func (s *Service) handleUpsertPlatformBinding(w http.ResponseWriter, r *http.Request) {
	var binding model.PlatformBinding
	if !decodeJSON(w, r, &binding) {
		return
	}
	if binding.UserID <= 0 {
		binding.UserID = filterUserID(r)
	}
	if binding.UserID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	binding.Kind = nonEmpty(binding.Kind, strings.TrimSpace(chi.URLParam(r, "kind")))
	if binding.Kind == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "kind required", nil)
		return
	}
	updated, err := s.store.UpsertPlatformBinding(r.Context(), binding)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleListPublicOptions(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	options, err := s.store.ListPublicOptions(r.Context(), userID, keyword)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", options)
}

func (s *Service) handleGetPublicOption(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	option, err := s.store.GetPublicOption(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", option)
}

func (s *Service) handleCreatePublicOption(w http.ResponseWriter, r *http.Request) {
	var option model.PublicOption
	if !decodeJSON(w, r, &option) {
		return
	}
	if option.UserID <= 0 {
		option.UserID = filterUserID(r)
	}
	if option.UserID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	created, err := s.store.CreatePublicOption(r.Context(), option)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", created)
}

func (s *Service) handleUpdatePublicOption(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var option model.PublicOption
	if !decodeJSON(w, r, &option) {
		return
	}
	option.ID = id
	if option.UserID <= 0 {
		option.UserID = filterUserID(r)
	}
	updated, err := s.store.UpdatePublicOption(r.Context(), option)
	if err != nil {
		if errors.Is(err, sqlitestore.ErrNotFound) {
			apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
			return
		}
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleDeletePublicOption(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeletePublicOption(r.Context(), id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"deleted": true})
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

func (s *Service) handleGetPopupState(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	key := nonEmpty(strings.TrimSpace(r.URL.Query().Get("key")), "default")
	state, err := s.store.GetPopupState(r.Context(), userID, key)
	if err != nil {
		state = model.PopupState{UserID: userID, Key: key}
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", state)
}

func (s *Service) handleUpdatePopupState(w http.ResponseWriter, r *http.Request) {
	var state model.PopupState
	if !decodeJSON(w, r, &state) {
		return
	}
	if state.UserID <= 0 {
		state.UserID = filterUserID(r)
	}
	if state.UserID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	updated, err := s.store.UpsertPopupState(r.Context(), state)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := filterUserID(r)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	pref, err := s.store.GetUserPreference(r.Context(), userID)
	if err != nil {
		pref = model.UserPreference{UserID: userID, Language: "zh-CN", Theme: "light", DefaultSearchMode: "default", ArticlePageSize: 20, EmailNotifications: true}
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", pref)
}

func (s *Service) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var pref model.UserPreference
	if !decodeJSON(w, r, &pref) {
		return
	}
	if pref.UserID <= 0 {
		pref.UserID = filterUserID(r)
	}
	if pref.UserID <= 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "user_id required", nil)
		return
	}
	updated, err := s.store.UpsertUserPreference(r.Context(), pref)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleGetMailConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.GetMailConfig(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", cfg)
}

func (s *Service) handleUpdateMailConfig(w http.ResponseWriter, r *http.Request) {
	var cfg model.MailConfig
	if !decodeJSON(w, r, &cfg) {
		return
	}
	updated, err := s.store.UpsertMailConfig(r.Context(), cfg)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleGetWarningSetting(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseID(w, r, "project_id")
	if !ok {
		return
	}
	setting, err := s.store.GetWarningSetting(r.Context(), projectID)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", setting)
}

func (s *Service) handleUpdateWarningSetting(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseID(w, r, "project_id")
	if !ok {
		return
	}
	var setting model.WarningSetting
	if !decodeJSON(w, r, &setting) {
		return
	}
	setting.ProjectID = projectID
	updated, err := s.store.UpsertWarningSetting(r.Context(), setting)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleGetOpinionCondition(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseID(w, r, "project_id")
	if !ok {
		return
	}
	condition, err := s.store.GetOpinionCondition(r.Context(), projectID)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", condition)
}

func (s *Service) handleUpdateOpinionCondition(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseID(w, r, "project_id")
	if !ok {
		return
	}
	condition, err := decodeOpinionConditionRequest(r)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	condition.ProjectID = projectID
	updated, err := s.store.UpsertOpinionCondition(r.Context(), condition)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
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
		Industry:   strings.TrimSpace(r.URL.Query().Get("industry")),
		Province:   strings.TrimSpace(r.URL.Query().Get("province")),
		City:       strings.TrimSpace(r.URL.Query().Get("city")),
		Sort:       strings.TrimSpace(r.URL.Query().Get("sort")),
		Read:       strings.TrimSpace(r.URL.Query().Get("read")),
		Favorite:   strings.TrimSpace(r.URL.Query().Get("favorite")),
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

func decodeOpinionConditionRequest(r *http.Request) (model.OpinionCondition, error) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return model.OpinionCondition{}, err
	}
	condition := model.OpinionCondition{
		Time:            intFromAny(raw["time"], 4),
		Precise:         intFromAny(raw["precise"], 0),
		Emotion:         stringOrJSON(raw["emotion"], "[1,2,3]"),
		Similar:         intFromAny(raw["similar"], 0),
		Sort:            intFromAny(raw["sort"], 1),
		Matchs:          intFromAny(raw["matchs"], 1),
		Times:           stringFromAny(raw["times"]),
		Timee:           stringFromAny(raw["timee"]),
		Classify:        stringFromAny(raw["classify"]),
		Websitename:     stringFromAny(raw["websitename"]),
		Author:          stringFromAny(raw["author"]),
		Organization:    stringFromAny(raw["organization"]),
		Categorylable:   stringFromAny(raw["categorylable"]),
		Enterprisetype:  stringFromAny(raw["enterprisetype"]),
		Hightechtype:    stringFromAny(raw["hightechtype"]),
		Policylableflag: stringFromAny(raw["policylableflag"]),
		DatasourceType:  stringFromAny(raw["datasource_type"]),
		EventIndex:      stringFromAny(raw["eventIndex"]),
		IndustryIndex:   stringFromAny(raw["industryIndex"]),
		Province:        stringFromAny(raw["province"]),
		City:            stringFromAny(raw["city"]),
	}
	if id, ok := int64FromAny(raw["opinion_condition_id"]); ok {
		condition.OpinionConditionID = id
	}
	if createTime := strings.TrimSpace(stringFromAny(raw["create_time"])); createTime != "" {
		condition.CreateTime = createTime
	}
	return condition, nil
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func stringOrJSON(value any, fallback string) string {
	switch typed := value.(type) {
	case nil:
		return fallback
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return fallback
		}
		if json.Valid([]byte(trimmed)) {
			return trimmed
		}
		raw, err := json.Marshal(trimmed)
		if err != nil {
			return fallback
		}
		return string(raw)
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fallback
		}
		return string(raw)
	}
}

func intFromAny(value any, fallback int) int {
	switch typed := value.(type) {
	case nil:
		return fallback
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func int64FromAny(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
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
