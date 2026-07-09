package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

const stockResearchDefaultPageSize = 20

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
	SetItemEmotion(rctx context.Context, itemID int64, emotion string) error
	SetItemLegacyStatus(rctx context.Context, itemID int64, status string) error
	MarkItemDeleted(rctx context.Context, itemID int64) error
	MarkItemRead(rctx context.Context, userID, itemID int64) error
	DeleteItemRead(rctx context.Context, userID, itemID int64) error
	ToggleFavorite(rctx context.Context, userID, itemID int64) (bool, error)
	RecordItemShare(rctx context.Context, share model.ShareRecord) error
	ListReports(rctx context.Context, projectID int64) ([]model.Report, error)
	GetReport(rctx context.Context, id int64) (model.Report, error)
	CreateReport(rctx context.Context, report model.Report) (model.Report, error)
	BatchDeleteReports(rctx context.Context, ids []int64) error
	BatchUpdateReportStatus(rctx context.Context, ids []int64, status string) error
	ListNotices(rctx context.Context) ([]model.SystemNotice, error)
	CreateFeedback(rctx context.Context, feedback model.Feedback) (model.Feedback, error)
	ListFeedback(rctx context.Context, limit int) ([]model.Feedback, error)
	DeleteFeedback(rctx context.Context, id int64) error
	ListTaskRuns(rctx context.Context, limit int) ([]model.TaskRun, error)
	CreateAuditLog(rctx context.Context, entry model.AuditLog) (model.AuditLog, error)
	ListAuditLogs(rctx context.Context, limit int, userID int64, action string) ([]model.AuditLog, error)
	GetUserPreference(rctx context.Context, userID int64) (model.UserPreference, error)
	UpsertUserPreference(rctx context.Context, pref model.UserPreference) (model.UserPreference, error)
	GetPopupState(rctx context.Context, userID int64, key string) (model.PopupState, error)
	UpsertPopupState(rctx context.Context, state model.PopupState) (model.PopupState, error)
	GetMailConfig(rctx context.Context) (model.MailConfig, error)
	UpsertMailConfig(rctx context.Context, cfg model.MailConfig) (model.MailConfig, error)
	GetReleaseSettings(rctx context.Context) (model.ReleaseSettings, error)
	UpsertReleaseSettings(rctx context.Context, settings model.ReleaseSettings) (model.ReleaseSettings, error)
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
	ListCrawlRuns(rctx context.Context, limit int, sourceType string) ([]model.CrawlRun, error)
	GetPlatformBinding(rctx context.Context, userID int64, kind string) (model.PlatformBinding, error)
	UpsertPlatformBinding(rctx context.Context, binding model.PlatformBinding) (model.PlatformBinding, error)
	ListPublicOptions(rctx context.Context, userID int64, keyword string) ([]model.PublicOption, error)
	GetPublicOption(rctx context.Context, id int64) (model.PublicOption, error)
	CreatePublicOption(rctx context.Context, option model.PublicOption) (model.PublicOption, error)
	UpdatePublicOption(rctx context.Context, option model.PublicOption) (model.PublicOption, error)
	DeletePublicOption(rctx context.Context, id int64) error
	UpsertAStockCodeNames(rctx context.Context, items []model.AStockCodeName) (model.AStockCodeNameUpsertResult, error)
	ListAStockCodeNames(rctx context.Context, codes []string) (model.AStockCodeNameListResult, error)
	UpsertAStockAuctionAmounts(rctx context.Context, tradeDate string, items []model.AStockAuctionAmount, replace bool) (model.AStockAuctionUpsertResult, error)
	ListAStockAuctionAmounts(rctx context.Context, filter model.AStockAuctionFilter) (model.AStockAuctionListResult, error)
	UpsertAStockRecommendationSnapshot(rctx context.Context, snapshot model.AStockRecommendationSnapshot) (model.AStockRecommendationSnapshotUpsertResult, error)
	GetAStockRecommendationSnapshot(rctx context.Context, strategyDate string, period string, ignoreRecent bool) (model.AStockRecommendationSnapshot, bool, error)
	UpsertAStockRecommendationSelections(rctx context.Context, selectionSet model.AStockRecommendationSelectionSet) (model.AStockRecommendationSelectionUpsertResult, error)
	ListAStockRecommendationSelections(rctx context.Context, strategyDate string, period string) (model.AStockRecommendationSelectionListResult, error)
	ListAStockRecommendationLatestDates(rctx context.Context, strategyDate string, period string, codes []string) (model.AStockRecommendationLatestDateListResult, error)
	UpsertAStockSectorFundFlows(rctx context.Context, tradeDate string, items []model.AStockSectorFundFlow, replace bool) (model.AStockSectorFundFlowUpsertResult, error)
	UpsertAStockSectorFundFlowSourceRows(rctx context.Context, tradeDate string, items []model.AStockSectorFundFlow, replace bool) (model.AStockSectorFundFlowUpsertResult, error)
	ListAStockSectorFundFlows(rctx context.Context, filter model.AStockSectorFundFlowFilter) (model.AStockSectorFundFlowListResult, error)
	UpsertAStockSectorConstituents(rctx context.Context, sectorType string, sectorName string, items []model.AStockSectorConstituent, replace bool) (model.AStockSectorConstituentUpsertResult, error)
	ListAStockSectorConstituents(rctx context.Context, filter model.AStockSectorConstituentFilter) (model.AStockSectorConstituentListResult, error)
	ListAStockSectorFundFlowTrend(rctx context.Context, filter model.AStockFundFlowTrendFilter) (model.AStockSectorFundFlowTrendResult, error)
	UpsertAStockStockFundFlows(rctx context.Context, tradeDate string, items []model.AStockStockFundFlow, replace bool) (model.AStockStockFundFlowUpsertResult, error)
	UpsertAStockStockFundFlowSourceRows(rctx context.Context, tradeDate string, items []model.AStockStockFundFlow, replace bool) (model.AStockStockFundFlowUpsertResult, error)
	ListAStockStockFundFlows(rctx context.Context, filter model.AStockStockFundFlowFilter) (model.AStockStockFundFlowListResult, error)
	ListAStockStockFundFlowTrend(rctx context.Context, filter model.AStockFundFlowTrendFilter) (model.AStockStockFundFlowTrendResult, error)
	UpsertStockResearchSurveys(rctx context.Context, items []model.StockResearchSurvey) (model.StockResearchUpsertResult, error)
	ListStockResearchSurveys(rctx context.Context, filter model.StockResearchFilter) (model.StockResearchListResult, error)
	GetStockResearchSurvey(rctx context.Context, id int64) (model.StockResearchSurvey, error)
	UpdateStockResearchPDF(rctx context.Context, id int64, update model.StockResearchPDFUpdate) (model.StockResearchSurvey, error)
	UpdateStockResearchSource(rctx context.Context, id int64, update model.StockResearchSourceUpdate) (model.StockResearchSurvey, error)
	UpsertStockInstitutionHoldings(rctx context.Context, items []model.StockInstitutionHolding) (model.StockInstitutionHoldingUpsertResult, error)
	ListStockInstitutionHoldings(rctx context.Context, filter model.StockInstitutionHoldingFilter) (model.StockInstitutionHoldingListResult, error)
	GetStockInstitutionHoldingSummary(rctx context.Context, code string, period string) (model.StockInstitutionHoldingSummary, error)
	ListStockInstitutionHoldingSignals(rctx context.Context, filter model.StockInstitutionHoldingSignalFilter) (model.StockInstitutionHoldingSignalListResult, error)
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
	r.Use(s.auditMiddleware)
	s.Routes(r)
	return r
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/healthz", s.handleHealthz)
	r.Get("/healthy", s.handleHealthz)

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
	r.Post("/api/v1/articles/{id}/emotion", s.handleSetArticleEmotion)
	r.Put("/api/v1/articles/{id}/status", s.handleSetArticleStatus)
	r.Delete("/api/v1/articles/{id}", s.handleDeleteArticle)
	r.Post("/api/v1/articles/{id}/read", s.handleMarkArticleRead)
	r.Delete("/api/v1/articles/{id}/read", s.handleUnmarkArticleRead)
	r.Post("/api/v1/articles/{id}/favorite", s.handleToggleFavorite)
	r.Post("/api/v1/articles/{id}/share", s.handleShareArticle)
	r.Get("/api/v1/a-stock/code-names", s.handleListAStockCodeNames)
	r.Get("/api/v1/a-stock/auction", s.handleListAStockAuctionAmounts)
	r.Post("/api/v1/admin/a-stock/auction", s.handleUpsertAStockAuctionAmounts)
	r.Get("/api/v1/a-stock/recommendations", s.handleGetAStockRecommendationSnapshot)
	r.Post("/api/v1/internal/a-stock/recommendations", s.handleUpsertAStockRecommendationSnapshot)
	r.Get("/api/v1/a-stock/recommendation-selections", s.handleListAStockRecommendationSelections)
	r.Post("/api/v1/internal/a-stock/recommendation-selections", s.handleUpsertAStockRecommendationSelections)
	r.Get("/api/v1/a-stock/recommendation-latest-dates", s.handleListAStockRecommendationLatestDates)
	r.Get("/api/v1/a-stock/sector-fund-flows", s.handleListAStockSectorFundFlows)
	r.Post("/api/v1/internal/a-stock/sector-fund-flows", s.handleUpsertAStockSectorFundFlows)
	r.Post("/api/v1/internal/a-stock/sector-fund-flow-sources", s.handleUpsertAStockSectorFundFlowSourceRows)
	r.Get("/api/v1/a-stock/sector-constituents", s.handleListAStockSectorConstituents)
	r.Post("/api/v1/internal/a-stock/sector-constituents", s.handleUpsertAStockSectorConstituents)
	r.Get("/api/v1/a-stock/sector-fund-flow-trend", s.handleListAStockSectorFundFlowTrend)
	r.Get("/api/v1/a-stock/stock-fund-flows", s.handleListAStockStockFundFlows)
	r.Get("/api/v1/a-stock/stock-fund-flow-trend", s.handleListAStockStockFundFlowTrend)
	r.Post("/api/v1/internal/a-stock/stock-fund-flows", s.handleUpsertAStockStockFundFlows)
	r.Post("/api/v1/internal/a-stock/stock-fund-flow-sources", s.handleUpsertAStockStockFundFlowSourceRows)
	r.Get("/api/v1/stock-research", s.handleListStockResearchSurveys)
	r.Get("/api/v1/stock-research/{id}", s.handleGetStockResearchSurvey)
	r.Get("/api/v1/stock-research/{id}/pdf", s.handleGetStockResearchPDF)
	r.Get("/api/v1/stock-research/{id}/pdf/text", s.handleGetStockResearchPDFText)
	r.Post("/api/v1/internal/stock-research/batch", s.handleUpsertStockResearchSurveys)
	r.Post("/api/v1/internal/stock-research/{id}/pdf", s.handleUpdateStockResearchPDF)
	r.Post("/api/v1/internal/stock-research/{id}/source", s.handleUpdateStockResearchSource)
	r.Get("/api/v1/a-stock/holdings", s.handleListStockInstitutionHoldings)
	r.Get("/api/v1/a-stock/holdings/summary", s.handleGetStockInstitutionHoldingSummary)
	r.Get("/api/v1/a-stock/holdings/signals", s.handleListStockInstitutionHoldingSignals)
	r.Post("/api/v1/internal/a-stock/holdings/batch", s.handleUpsertStockInstitutionHoldings)
	r.Get("/api/v1/search/articles", s.handleSearchArticles)
	r.Get("/api/v1/search/full", s.handleSearchFull)
	r.Get("/api/v1/search/timely", s.handleSearchTimely)
	r.Get("/api/v1/search/details/{id}", s.handleSearchDetail)
	r.Get("/api/v1/search/metadata/types", s.handleSearchMetadataTypes)
	r.Get("/api/v1/search/metadata/polymerizations", s.handleSearchMetadataPolymerizations)
	r.Get("/api/v1/search/metadata/breadcrumbs", s.handleSearchMetadataBreadcrumbs)
	r.Get("/api/v1/search/special/{kind}", s.handleSearchSpecialList)
	r.Get("/api/v1/search/special/{kind}/options", s.handleSearchSpecialOptions)
	r.Get("/api/v1/search/special/{kind}/details/{id}", s.handleSearchSpecialDetail)
	r.Get("/api/v1/search/full/facets", s.handleSearchFacets)
	r.Get("/api/v1/search/options", s.handleSearchOptions)
	r.Get("/api/v1/search/options/{kind}", s.handleSearchOptionKind)
	r.Get("/api/v1/search/history", s.handleListSearchHistory)
	r.Get("/api/v1/search/suggestions", s.handleSearchSuggestions)
	r.Get("/api/v1/search/hot-keywords", s.handleHotKeywords)
	r.Post("/api/v1/search/history", s.handleSaveSearchHistory)
	r.Get("/api/v1/crypto/pairs/resolve", s.handleCryptoPairResolve)
	r.Get("/api/v1/crypto/news", s.handleCryptoNews)
	r.Get("/api/v1/crypto/social", s.handleCryptoSocial)
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
	r.Post("/api/v1/reports/batch-delete", s.handleBatchDeleteReports)
	r.Post("/api/v1/reports/batch-status", s.handleBatchUpdateReportStatus)

	r.Get("/api/v1/system/notices", s.handleListNotices)
	r.Get("/api/v1/system/feedback", s.handleListFeedback)
	r.Post("/api/v1/system/feedback", s.handleCreateFeedback)
	r.Delete("/api/v1/system/feedback/{id}", s.handleDeleteFeedback)
	r.Get("/api/v1/system/task-runs", s.handleListTaskRuns)
	r.Get("/api/v1/system/audit-logs", s.handleListAuditLogs)
	r.Post("/api/v1/system/audit-logs", s.handleCreateAuditLog)
	r.Get("/api/v1/system/operations", s.handleOperations)
	r.Get("/api/v1/system/alerts", s.handleAlerts)
	r.Get("/api/v1/system/database-config", s.handleDatabaseConfig)
	r.Post("/api/v1/system/database-config/check", s.handleDatabaseCheck)
	r.Post("/api/v1/system/database-config/save", s.handleDatabaseSave)
	r.Post("/api/v1/system/database-config/switch", s.handleDatabaseSwitch)
	r.Get("/api/v1/system/services/{name}/logs", s.handleServiceLogs)
	r.Post("/api/v1/system/services/{name}/restart", s.handleRestartService)
	r.Get("/api/v1/system/popup", s.handleGetPopupState)
	r.Put("/api/v1/system/popup", s.handleUpdatePopupState)
	r.Get("/api/v1/system/preferences", s.handleGetPreferences)
	r.Put("/api/v1/system/preferences", s.handleUpdatePreferences)
	r.Get("/api/v1/system/mail-config", s.handleGetMailConfig)
	r.Put("/api/v1/system/mail-config", s.handleUpdateMailConfig)
	r.Get("/api/v1/system/release-settings", s.handleGetReleaseSettings)
	r.Put("/api/v1/system/release-settings", s.handleUpdateReleaseSettings)
	r.Get("/api/v1/system/warning-settings/{project_id}", s.handleGetWarningSetting)
	r.Put("/api/v1/system/warning-settings/{project_id}", s.handleUpdateWarningSetting)
	r.Get("/api/v1/system/opinion-conditions/{project_id}", s.handleGetOpinionCondition)
	r.Put("/api/v1/system/opinion-conditions/{project_id}", s.handleUpdateOpinionCondition)

	r.Get("/api/v1/android/bootstrap", s.handleAndroidBootstrap)
	r.Get("/api/v1/android/dashboard", s.handleAndroidDashboard)
	r.Get("/api/v1/android/modules", s.handleAndroidModules)
	r.Post("/api/v1/android/actions/{action}", s.handleAndroidAction)
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteHealth(w, "content-service", "ok", "ok")
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
	tpl.ConfigJSON = syncTemplateWebsiteConfig(tpl.ConfigJSON, tpl.Website)
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
	tpl.ConfigJSON = syncTemplateWebsiteConfig(tpl.ConfigJSON, tpl.Website)
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

func syncTemplateWebsiteConfig(rawConfig, website string) string {
	rawConfig = strings.TrimSpace(rawConfig)
	if rawConfig == "" {
		rawConfig = "{}"
	}
	website = strings.TrimSpace(website)
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
		return rawConfig
	}
	if website == "" {
		delete(payload, "website")
	} else {
		payload["website"] = website
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return rawConfig
	}
	return string(normalized)
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

func (s *Service) handleListAStockAuctionAmounts(w http.ResponseWriter, r *http.Request) {
	filter := model.AStockAuctionFilter{
		Date:      strings.TrimSpace(r.URL.Query().Get("date")),
		Keyword:   strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("code"))),
		Page:      apiutil.IntQuery(r, "page", 1),
		PageSize:  apiutil.IntQuery(r, "page_size", 50),
		TrendDays: normalizeAStockAuctionTrendDays(apiutil.IntQuery(r, "trend_days", 7)),
	}
	result, err := s.store.ListAStockAuctionAmounts(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListAStockCodeNames(w http.ResponseWriter, r *http.Request) {
	codes := splitAStockRecommendationCodes(r.URL.Query().Get("codes"))
	if code := strings.TrimSpace(r.URL.Query().Get("code")); code != "" {
		codes = append(codes, code)
	}
	result, err := s.store.ListAStockCodeNames(r.Context(), codes)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func normalizeAStockAuctionTrendDays(days int) int {
	switch days {
	case 14:
		return 14
	case 30:
		return 30
	default:
		return 7
	}
}

func (s *Service) handleUpsertAStockAuctionAmounts(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Date      string                      `json:"date"`
		Items     []model.AStockAuctionAmount `json:"items"`
		CodeNames []model.AStockCodeName      `json:"code_names"`
		Replace   bool                        `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	payload.Date = strings.TrimSpace(payload.Date)
	if payload.Date == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date required", nil)
		return
	}
	for i := range payload.Items {
		if strings.TrimSpace(payload.Items[i].TradeDate) == "" {
			payload.Items[i].TradeDate = payload.Date
		}
	}
	if len(payload.CodeNames) > 0 {
		if _, err := s.store.UpsertAStockCodeNames(r.Context(), payload.CodeNames); err != nil {
			apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
	}
	result, err := s.store.UpsertAStockAuctionAmounts(r.Context(), payload.Date, payload.Items, payload.Replace)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleGetAStockRecommendationSnapshot(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if date == "" || period == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date and period required", nil)
		return
	}
	ignoreRecent := normalizeBoolQuery(r.URL.Query().Get("ignore_recent"))
	snapshot, found, err := s.store.GetAStockRecommendationSnapshot(r.Context(), date, period, ignoreRecent)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	snapshot.Found = found
	normalizeAStockRecommendationSnapshotJSON(&snapshot)
	apiutil.WriteJSON(w, http.StatusOK, "ok", snapshot)
}

func (s *Service) handleUpsertAStockRecommendationSnapshot(w http.ResponseWriter, r *http.Request) {
	var snapshot model.AStockRecommendationSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	snapshot.StrategyDate = strings.TrimSpace(snapshot.StrategyDate)
	snapshot.Period = strings.TrimSpace(snapshot.Period)
	if snapshot.StrategyDate == "" || snapshot.Period == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "strategy_date and period required", nil)
		return
	}
	normalizeAStockRecommendationSnapshotJSON(&snapshot)
	result, err := s.store.UpsertAStockRecommendationSnapshot(r.Context(), snapshot)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func normalizeAStockRecommendationSnapshotJSON(snapshot *model.AStockRecommendationSnapshot) {
	if snapshot == nil {
		return
	}
	snapshot.RecommendationsJSON = normalizeAStockRecommendationJSONArray(snapshot.RecommendationsJSON)
	snapshot.BacktestsJSON = normalizeAStockRecommendationJSONArray(snapshot.BacktestsJSON)
}

func normalizeAStockRecommendationJSONArray(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return "[]"
	}
	return raw
}

func (s *Service) handleListAStockRecommendationSelections(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if date == "" || period == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date and period required", nil)
		return
	}
	result, err := s.store.ListAStockRecommendationSelections(r.Context(), date, period)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	result.Found = len(result.Items) > 0
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListAStockRecommendationLatestDates(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	codes := splitAStockRecommendationCodes(r.URL.Query().Get("codes"))
	if date == "" || period == "" || len(codes) == 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date, period and codes required", nil)
		return
	}
	result, err := s.store.ListAStockRecommendationLatestDates(r.Context(), date, period, codes)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func splitAStockRecommendationCodes(raw string) []string {
	parts := strings.Split(raw, ",")
	codes := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		code := strings.TrimSpace(part)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

func (s *Service) handleUpsertAStockRecommendationSelections(w http.ResponseWriter, r *http.Request) {
	var selectionSet model.AStockRecommendationSelectionSet
	if err := json.NewDecoder(r.Body).Decode(&selectionSet); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	selectionSet.StrategyDate = strings.TrimSpace(selectionSet.StrategyDate)
	selectionSet.Period = strings.TrimSpace(selectionSet.Period)
	if selectionSet.StrategyDate == "" || selectionSet.Period == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "strategy_date and period required", nil)
		return
	}
	result, err := s.store.UpsertAStockRecommendationSelections(r.Context(), selectionSet)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func normalizeBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Service) handleListAStockSectorFundFlows(w http.ResponseWriter, r *http.Request) {
	filter := model.AStockSectorFundFlowFilter{
		Date:       strings.TrimSpace(r.URL.Query().Get("date")),
		SectorType: strings.TrimSpace(r.URL.Query().Get("sector_type")),
		Indicator:  strings.TrimSpace(r.URL.Query().Get("indicator")),
		Keyword:    strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("q"))),
		SourceType: strings.TrimSpace(nonEmpty(r.URL.Query().Get("source_type"), r.URL.Query().Get("source"))),
		Page:       apiutil.IntQuery(r, "page", 1),
		PageSize:   apiutil.IntQuery(r, "page_size", 100),
	}
	result, err := s.store.ListAStockSectorFundFlows(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleUpsertAStockSectorFundFlowSourceRows(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Date       string                       `json:"date"`
		SectorType string                       `json:"sector_type"`
		Indicator  string                       `json:"indicator"`
		SourceType string                       `json:"source_type"`
		Items      []model.AStockSectorFundFlow `json:"items"`
		Replace    bool                         `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	payload.Date = strings.TrimSpace(payload.Date)
	payload.SectorType = normalizeAStockSectorFundFlowSectorType(payload.SectorType)
	payload.Indicator = normalizeAStockSectorFundFlowIndicator(payload.Indicator)
	payload.SourceType = normalizeAStockFundFlowSourceType(payload.SourceType)
	if payload.Date == "" {
		for _, item := range payload.Items {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				payload.Date = date
				break
			}
		}
	}
	if payload.SourceType == "" {
		for _, item := range payload.Items {
			if sourceType := normalizeAStockFundFlowSourceType(item.SourceType); sourceType != "" {
				payload.SourceType = sourceType
				break
			}
		}
	}
	if payload.Date == "" || payload.SourceType == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date and source_type required", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i] = normalizeAStockSectorFundFlow(payload.Items[i], payload.Date, payload.SectorType, payload.Indicator, payload.SourceType, now)
	}
	result, err := s.store.UpsertAStockSectorFundFlowSourceRows(r.Context(), payload.Date, payload.Items, payload.Replace)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleUpsertAStockSectorFundFlows(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Date       string                       `json:"date"`
		SectorType string                       `json:"sector_type"`
		Indicator  string                       `json:"indicator"`
		Items      []model.AStockSectorFundFlow `json:"items"`
		Replace    bool                         `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	payload.Date = strings.TrimSpace(payload.Date)
	payload.SectorType = normalizeAStockSectorFundFlowSectorType(payload.SectorType)
	payload.Indicator = normalizeAStockSectorFundFlowIndicator(payload.Indicator)
	if payload.Date == "" {
		for _, item := range payload.Items {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				payload.Date = date
				break
			}
		}
	}
	if payload.Date == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date required", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i] = normalizeAStockSectorFundFlow(payload.Items[i], payload.Date, payload.SectorType, payload.Indicator, "", now)
	}
	result, err := s.store.UpsertAStockSectorFundFlows(r.Context(), payload.Date, payload.Items, payload.Replace)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListAStockSectorConstituents(w http.ResponseWriter, r *http.Request) {
	filter := model.AStockSectorConstituentFilter{
		SectorType: strings.TrimSpace(r.URL.Query().Get("sector_type")),
		SectorName: strings.TrimSpace(nonEmpty(r.URL.Query().Get("sector_name"), r.URL.Query().Get("name"), r.URL.Query().Get("symbol"))),
		Keyword:    strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("q"))),
		Limit:      apiutil.IntQuery(r, "limit", 500),
	}
	result, err := s.store.ListAStockSectorConstituents(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleUpsertAStockSectorConstituents(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SectorType string                          `json:"sector_type"`
		SectorName string                          `json:"sector_name"`
		Items      []model.AStockSectorConstituent `json:"items"`
		Replace    bool                            `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	payload.SectorType = normalizeAStockSectorFundFlowSectorType(payload.SectorType)
	payload.SectorName = strings.TrimSpace(payload.SectorName)
	if payload.SectorName == "" {
		for _, item := range payload.Items {
			if sectorName := strings.TrimSpace(item.SectorName); sectorName != "" {
				payload.SectorName = sectorName
				break
			}
		}
	}
	if payload.SectorName == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "sector_name required", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i].SectorType = normalizeAStockSectorFundFlowSectorType(nonEmpty(payload.Items[i].SectorType, payload.SectorType))
		payload.Items[i].SectorName = strings.TrimSpace(nonEmpty(payload.Items[i].SectorName, payload.SectorName))
		payload.Items[i].Code = astockcode.Normalize(payload.Items[i].Code)
		payload.Items[i].Name = strings.TrimSpace(payload.Items[i].Name)
		payload.Items[i].Source = strings.TrimSpace(payload.Items[i].Source)
		if payload.Items[i].FetchedAt.IsZero() {
			payload.Items[i].FetchedAt = now
		}
		if payload.Items[i].CreatedAt.IsZero() {
			payload.Items[i].CreatedAt = now
		}
		if payload.Items[i].UpdatedAt.IsZero() {
			payload.Items[i].UpdatedAt = now
		}
	}
	result, err := s.store.UpsertAStockSectorConstituents(r.Context(), payload.SectorType, payload.SectorName, payload.Items, payload.Replace)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListAStockSectorFundFlowTrend(w http.ResponseWriter, r *http.Request) {
	filter := model.AStockFundFlowTrendFilter{
		EndDate:    strings.TrimSpace(nonEmpty(r.URL.Query().Get("end_date"), r.URL.Query().Get("date"))),
		SectorType: strings.TrimSpace(r.URL.Query().Get("sector_type")),
		SectorName: strings.TrimSpace(nonEmpty(r.URL.Query().Get("sector_name"), r.URL.Query().Get("name"), r.URL.Query().Get("symbol"))),
		Indicator:  strings.TrimSpace(r.URL.Query().Get("indicator")),
		Days:       apiutil.IntQuery(r, "days", 5),
	}
	result, err := s.store.ListAStockSectorFundFlowTrend(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func normalizeAStockSectorFundFlow(item model.AStockSectorFundFlow, date string, sectorType string, indicator string, sourceType string, now time.Time) model.AStockSectorFundFlow {
	item.TradeDate = nonEmpty(strings.TrimSpace(item.TradeDate), strings.TrimSpace(date))
	item.SectorType = normalizeAStockSectorFundFlowSectorType(nonEmpty(strings.TrimSpace(item.SectorType), sectorType))
	item.Indicator = normalizeAStockSectorFundFlowIndicator(nonEmpty(strings.TrimSpace(item.Indicator), indicator))
	item.Name = strings.TrimSpace(item.Name)
	item.TopStock = strings.TrimSpace(item.TopStock)
	item.SourceType = nonEmpty(normalizeAStockFundFlowSourceType(item.SourceType), sourceType, "akshare_sector_fund_flow")
	item.SourceTypes = nonEmpty(strings.TrimSpace(item.SourceTypes), item.SourceType)
	item.FieldCountsJSON = nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}")
	item.RawPayload = nonEmpty(strings.TrimSpace(item.RawPayload), "{}")
	if item.FetchedAt.IsZero() {
		item.FetchedAt = now
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	return item
}

func (s *Service) handleListAStockStockFundFlows(w http.ResponseWriter, r *http.Request) {
	filter := model.AStockStockFundFlowFilter{
		Date:       strings.TrimSpace(r.URL.Query().Get("date")),
		Indicator:  strings.TrimSpace(r.URL.Query().Get("indicator")),
		Keyword:    strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("q"))),
		SourceType: strings.TrimSpace(nonEmpty(r.URL.Query().Get("source_type"), r.URL.Query().Get("source"))),
		Codes:      parseAStockCodeQuery(r.URL.Query()),
		Page:       apiutil.IntQuery(r, "page", 1),
		PageSize:   apiutil.IntQuery(r, "page_size", 100),
	}
	result, err := s.store.ListAStockStockFundFlows(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListAStockStockFundFlowTrend(w http.ResponseWriter, r *http.Request) {
	filter := model.AStockFundFlowTrendFilter{
		EndDate:   strings.TrimSpace(nonEmpty(r.URL.Query().Get("end_date"), r.URL.Query().Get("date"))),
		Indicator: strings.TrimSpace(r.URL.Query().Get("indicator")),
		Code:      strings.TrimSpace(nonEmpty(r.URL.Query().Get("code"), r.URL.Query().Get("codes"))),
		Keyword:   strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("q"))),
		Days:      apiutil.IntQuery(r, "days", 5),
	}
	result, err := s.store.ListAStockStockFundFlowTrend(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func parseAStockCodeQuery(query url.Values) []string {
	values := make([]string, 0, len(query["code"])+len(query["codes"]))
	values = append(values, query["code"]...)
	values = append(values, query["codes"]...)
	seen := map[string]struct{}{}
	codes := make([]string, 0)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			code := astockcode.Normalize(strings.TrimSpace(part))
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
	}
	return codes
}

func (s *Service) handleUpsertAStockStockFundFlows(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Date      string                      `json:"date"`
		Indicator string                      `json:"indicator"`
		Items     []model.AStockStockFundFlow `json:"items"`
		Replace   bool                        `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	payload.Date = strings.TrimSpace(payload.Date)
	payload.Indicator = normalizeAStockSectorFundFlowIndicator(payload.Indicator)
	if payload.Date == "" {
		for _, item := range payload.Items {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				payload.Date = date
				break
			}
		}
	}
	if payload.Date == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date required", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i] = normalizeAStockStockFundFlow(payload.Items[i], payload.Date, payload.Indicator, "", now)
	}
	result, err := s.store.UpsertAStockStockFundFlows(r.Context(), payload.Date, payload.Items, payload.Replace)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleUpsertAStockStockFundFlowSourceRows(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Date       string                      `json:"date"`
		Indicator  string                      `json:"indicator"`
		SourceType string                      `json:"source_type"`
		Items      []model.AStockStockFundFlow `json:"items"`
		Replace    bool                        `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	payload.Date = strings.TrimSpace(payload.Date)
	payload.Indicator = normalizeAStockSectorFundFlowIndicator(payload.Indicator)
	payload.SourceType = normalizeAStockFundFlowSourceType(payload.SourceType)
	if payload.Date == "" {
		for _, item := range payload.Items {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				payload.Date = date
				break
			}
		}
	}
	if payload.SourceType == "" {
		for _, item := range payload.Items {
			if sourceType := normalizeAStockFundFlowSourceType(item.SourceType); sourceType != "" {
				payload.SourceType = sourceType
				break
			}
		}
	}
	if payload.Date == "" || payload.SourceType == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "date and source_type required", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i] = normalizeAStockStockFundFlow(payload.Items[i], payload.Date, payload.Indicator, payload.SourceType, now)
	}
	result, err := s.store.UpsertAStockStockFundFlowSourceRows(r.Context(), payload.Date, payload.Items, payload.Replace)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func normalizeAStockStockFundFlow(item model.AStockStockFundFlow, date string, indicator string, sourceType string, now time.Time) model.AStockStockFundFlow {
	item.TradeDate = nonEmpty(strings.TrimSpace(item.TradeDate), strings.TrimSpace(date))
	item.Indicator = normalizeAStockSectorFundFlowIndicator(nonEmpty(strings.TrimSpace(item.Indicator), indicator))
	item.Code = astockcode.Normalize(item.Code)
	item.Name = strings.TrimSpace(item.Name)
	item.SourceType = nonEmpty(normalizeAStockFundFlowSourceType(item.SourceType), sourceType, "average")
	item.SourceTypes = nonEmpty(strings.TrimSpace(item.SourceTypes), item.SourceType)
	item.FieldCountsJSON = nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}")
	item.RawPayload = nonEmpty(strings.TrimSpace(item.RawPayload), "{}")
	if item.FetchedAt.IsZero() {
		item.FetchedAt = now
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	return item
}

func normalizeAStockFundFlowSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "eastmoney", "em", "东方财富", "akshare_sector_fund_flow":
		return "eastmoney"
	case "ths", "同花顺", "10jqka":
		return "ths"
	case "sina", "新浪", "sinafinance", "sina_finance":
		return "sina"
	case "all", "average", "aggregate":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.TrimSpace(value)
	}
}

func normalizeAStockSectorFundFlowSectorType(value string) string {
	switch strings.TrimSpace(value) {
	case "概念", "概念资金", "概念资金流":
		return "概念资金流"
	default:
		return "行业资金流"
	}
}

func normalizeAStockSectorFundFlowIndicator(value string) string {
	switch strings.TrimSpace(value) {
	case "5", "5日":
		return "5日"
	case "10", "10日":
		return "10日"
	default:
		return "今日"
	}
}

func (s *Service) handleListStockResearchSurveys(w http.ResponseWriter, r *http.Request) {
	filter := model.StockResearchFilter{
		Code:        strings.TrimSpace(r.URL.Query().Get("code")),
		Company:     strings.TrimSpace(nonEmpty(r.URL.Query().Get("company"), r.URL.Query().Get("q"))),
		Institution: strings.TrimSpace(r.URL.Query().Get("institution")),
		Kind:        strings.TrimSpace(r.URL.Query().Get("kind")),
		Source:      strings.TrimSpace(r.URL.Query().Get("source")),
		Start:       strings.TrimSpace(r.URL.Query().Get("start")),
		End:         strings.TrimSpace(r.URL.Query().Get("end")),
		Page:        apiutil.IntQuery(r, "page", 1),
		PageSize:    apiutil.IntQuery(r, "page_size", stockResearchDefaultPageSize),
	}
	result, err := s.store.ListStockResearchSurveys(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleUpsertStockResearchSurveys(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Items []model.StockResearchSurvey `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i] = normalizeStockResearchSurvey(payload.Items[i], now)
	}
	result, err := s.store.UpsertStockResearchSurveys(r.Context(), payload.Items)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleGetStockResearchSurvey(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.store.GetStockResearchSurvey(r.Context(), int64(id))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "stock research not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", item)
}

func (s *Service) handleGetStockResearchPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.store.GetStockResearchSurvey(r.Context(), int64(id))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "stock research not found", nil)
		return
	}
	path := strings.TrimSpace(item.PDFFilePath)
	if path == "" {
		apiutil.WriteJSON(w, http.StatusNotFound, "pdf not downloaded", nil)
		return
	}
	cleanPath, err := s.safeStockResearchPDFPath(path)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if _, err := os.Stat(cleanPath); err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "pdf file not found", nil)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="stock-research.pdf"`)
	http.ServeFile(w, r, cleanPath)
}

func (s *Service) handleGetStockResearchPDFText(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.store.GetStockResearchSurvey(r.Context(), int64(id))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "stock research not found", nil)
		return
	}
	if strings.TrimSpace(item.PDFText) == "" {
		apiutil.WriteJSON(w, http.StatusNotFound, "pdf text not parsed", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"id":            item.ID,
		"title":         item.Title,
		"pdf_url":       item.PDFURL,
		"pdf_status":    item.PDFStatus,
		"pdf_text":      item.PDFText,
		"pdf_parsed_at": item.PDFParsedAt,
		"nlp_score":     item.NLPScore,
		"nlp_rating":    item.NLPRating,
		"nlp_reason":    item.NLPReason,
		"nlp_scored_at": item.NLPScoredAt,
	})
}

func (s *Service) handleUpdateStockResearchPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var payload model.StockResearchPDFUpdate
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	item, err := s.store.UpdateStockResearchPDF(r.Context(), int64(id), normalizeStockResearchPDFUpdate(payload))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", item)
}

func (s *Service) handleUpdateStockResearchSource(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var payload model.StockResearchSourceUpdate
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	item, err := s.store.UpdateStockResearchSource(r.Context(), int64(id), normalizeStockResearchSourceUpdate(payload))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", item)
}

func normalizeStockResearchSurvey(item model.StockResearchSurvey, now time.Time) model.StockResearchSurvey {
	item.Code = strings.TrimSpace(item.Code)
	item.Name = strings.TrimSpace(item.Name)
	item.Kind = normalizeStockResearchKind(item.Kind)
	item.Title = strings.TrimSpace(item.Title)
	item.Institution = strings.TrimSpace(item.Institution)
	item.Analyst = strings.TrimSpace(item.Analyst)
	item.Rating = strings.TrimSpace(item.Rating)
	item.TargetPrice = strings.TrimSpace(item.TargetPrice)
	item.ResearchDate = strings.TrimSpace(item.ResearchDate)
	item.PublishTime = strings.TrimSpace(item.PublishTime)
	item.SourceURL = strings.TrimSpace(item.SourceURL)
	item.SourceType = strings.TrimSpace(item.SourceType)
	item.SourceKey = strings.TrimSpace(item.SourceKey)
	item.Summary = strings.TrimSpace(item.Summary)
	item.RawPayload = strings.TrimSpace(item.RawPayload)
	item.SourceText = strings.TrimSpace(item.SourceText)
	item.SourceFetchStatus = normalizeStockResearchSourceStatus(item.SourceFetchStatus)
	item.SourceFetchError = strings.TrimSpace(item.SourceFetchError)
	item.SourceFetchedAt = strings.TrimSpace(item.SourceFetchedAt)
	item.PDFURL = strings.TrimSpace(item.PDFURL)
	item.PDFFilePath = strings.TrimSpace(item.PDFFilePath)
	item.PDFStatus = normalizeStockResearchPDFStatus(item.PDFStatus)
	item.PDFText = strings.TrimSpace(item.PDFText)
	item.PDFError = strings.TrimSpace(item.PDFError)
	item.PDFFetchedAt = strings.TrimSpace(item.PDFFetchedAt)
	item.PDFParsedAt = strings.TrimSpace(item.PDFParsedAt)
	item.NLPRating = strings.TrimSpace(item.NLPRating)
	item.NLPReason = strings.TrimSpace(item.NLPReason)
	item.NLPScoredAt = strings.TrimSpace(item.NLPScoredAt)
	if item.SourceType == "" {
		item.SourceType = "stock_research"
	}
	if item.SourceKey == "" {
		item.SourceKey = strings.Join([]string{item.SourceType, item.Code, item.Title, item.ResearchDate, item.SourceURL}, "|")
	}
	if item.RawPayload == "" {
		item.RawPayload = "{}"
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	return item
}

func normalizeStockResearchPDFUpdate(update model.StockResearchPDFUpdate) model.StockResearchPDFUpdate {
	update.PDFURL = strings.TrimSpace(update.PDFURL)
	update.PDFFilePath = strings.TrimSpace(update.PDFFilePath)
	update.PDFStatus = normalizeStockResearchPDFStatus(update.PDFStatus)
	update.PDFText = strings.TrimSpace(update.PDFText)
	update.PDFError = strings.TrimSpace(update.PDFError)
	update.PDFFetchedAt = strings.TrimSpace(update.PDFFetchedAt)
	update.PDFParsedAt = strings.TrimSpace(update.PDFParsedAt)
	update.NLPRating = strings.TrimSpace(update.NLPRating)
	update.NLPReason = strings.TrimSpace(update.NLPReason)
	update.NLPScoredAt = strings.TrimSpace(update.NLPScoredAt)
	update.SourceText = strings.TrimSpace(update.SourceText)
	update.SourceFetchStatus = normalizeStockResearchSourceStatus(update.SourceFetchStatus)
	update.SourceFetchError = strings.TrimSpace(update.SourceFetchError)
	update.SourceFetchedAt = strings.TrimSpace(update.SourceFetchedAt)
	if update.SourceText == "" && update.PDFStatus == "parsed" && update.PDFText != "" {
		update.SourceText = update.PDFText
		update.SourceFetchStatus = "parsed"
		update.SourceFetchError = ""
		update.SourceFetchedAt = nonEmpty(update.SourceFetchedAt, update.PDFParsedAt, update.PDFFetchedAt)
	}
	return update
}

func normalizeStockResearchSourceUpdate(update model.StockResearchSourceUpdate) model.StockResearchSourceUpdate {
	update.SourceText = strings.TrimSpace(update.SourceText)
	update.SourceFetchStatus = normalizeStockResearchSourceStatus(update.SourceFetchStatus)
	update.SourceFetchError = strings.TrimSpace(update.SourceFetchError)
	update.SourceFetchedAt = strings.TrimSpace(update.SourceFetchedAt)
	return update
}

func normalizeStockResearchPDFStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "downloaded", "parsed", "no_pdf", "no_text", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.TrimSpace(value)
	}
}

func normalizeStockResearchSourceStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "parsed", "no_source", "failed", "pending_pdf":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.TrimSpace(value)
	}
}

func (s *Service) safeStockResearchPDFPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("pdf file path required")
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		root := filepath.Clean(strings.TrimSpace(s.cfg.StockResearchPDFDir))
		if root == "" || !filepath.IsAbs(root) {
			return "", errors.New("absolute pdf file path is not allowed")
		}
		rel, err := filepath.Rel(root, clean)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return "", errors.New("pdf file path is outside configured directory")
		}
		return clean, nil
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", errors.New("invalid pdf file path")
	}
	return clean, nil
}

func normalizeStockResearchKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "survey", "调研":
		return "survey"
	default:
		return "report"
	}
}

func (s *Service) handleListStockInstitutionHoldings(w http.ResponseWriter, r *http.Request) {
	filter := model.StockInstitutionHoldingFilter{
		Code:       normalizeAStockContentCode(r.URL.Query().Get("code")),
		Company:    strings.TrimSpace(nonEmpty(r.URL.Query().Get("company"), r.URL.Query().Get("q"))),
		Period:     normalizeStockHoldingPeriod(r.URL.Query().Get("period")),
		Holder:     strings.TrimSpace(r.URL.Query().Get("holder")),
		HolderType: strings.TrimSpace(r.URL.Query().Get("holder_type")),
		Source:     strings.TrimSpace(r.URL.Query().Get("source")),
		Page:       apiutil.IntQuery(r, "page", 1),
		PageSize:   apiutil.IntQuery(r, "page_size", 50),
	}
	result, err := s.store.ListStockInstitutionHoldings(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleGetStockInstitutionHoldingSummary(w http.ResponseWriter, r *http.Request) {
	code := normalizeAStockContentCode(r.URL.Query().Get("code"))
	period := normalizeStockHoldingPeriod(r.URL.Query().Get("period"))
	result, err := s.store.GetStockInstitutionHoldingSummary(r.Context(), code, period)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleListStockInstitutionHoldingSignals(w http.ResponseWriter, r *http.Request) {
	filter := model.StockInstitutionHoldingSignalFilter{
		Code:     normalizeAStockContentCode(r.URL.Query().Get("code")),
		Company:  strings.TrimSpace(nonEmpty(r.URL.Query().Get("company"), r.URL.Query().Get("q"))),
		Period:   normalizeStockHoldingPeriod(r.URL.Query().Get("period")),
		Page:     apiutil.IntQuery(r, "page", 1),
		PageSize: apiutil.IntQuery(r, "page_size", 20),
	}
	result, err := s.store.ListStockInstitutionHoldingSignals(r.Context(), filter)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleUpsertStockInstitutionHoldings(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Items []model.StockInstitutionHolding `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid json", nil)
		return
	}
	now := time.Now().UTC()
	for i := range payload.Items {
		payload.Items[i] = normalizeStockInstitutionHolding(payload.Items[i], now)
	}
	result, err := s.store.UpsertStockInstitutionHoldings(r.Context(), payload.Items)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func normalizeStockInstitutionHolding(item model.StockInstitutionHolding, now time.Time) model.StockInstitutionHolding {
	item.StockCode = normalizeAStockContentCode(item.StockCode)
	item.StockName = strings.TrimSpace(item.StockName)
	item.ReportPeriod = normalizeStockHoldingPeriod(item.ReportPeriod)
	item.AnnounceDate = strings.TrimSpace(item.AnnounceDate)
	item.HolderName = strings.TrimSpace(item.HolderName)
	item.HolderType = normalizeStockInstitutionHolderType(item.HolderType, item.HolderName)
	item.HolderCode = strings.TrimSpace(item.HolderCode)
	item.HolderRank = strings.TrimSpace(item.HolderRank)
	item.SourceType = strings.TrimSpace(item.SourceType)
	item.SourceURL = strings.TrimSpace(item.SourceURL)
	item.RawPayload = strings.TrimSpace(item.RawPayload)
	if item.SourceType == "" {
		item.SourceType = "akshare_stock_holding"
	}
	if item.RawPayload == "" {
		item.RawPayload = "{}"
	}
	if item.FetchedAt.IsZero() {
		item.FetchedAt = now
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	return item
}

func normalizeStockHoldingPeriod(value string) string {
	value = strings.TrimSpace(value)
	upper := strings.ToUpper(value)
	upper = strings.ReplaceAll(upper, "-", "")
	upper = strings.ReplaceAll(upper, "/", "")
	upper = strings.ReplaceAll(upper, ".", "")
	if len(upper) == 6 && upper[4] == 'Q' {
		switch upper[5:] {
		case "1":
			return upper[:4] + "0331"
		case "2":
			return upper[:4] + "0630"
		case "3":
			return upper[:4] + "0930"
		case "4":
			return upper[:4] + "1231"
		}
	}
	if len(upper) == 8 {
		return upper
	}
	return strings.TrimSpace(value)
}

func normalizeAStockContentCode(value string) string {
	value = strings.TrimSpace(value)
	upper := strings.ToUpper(value)
	for _, suffix := range []string{".SH", ".SZ", ".BJ", ".OF"} {
		if strings.HasSuffix(upper, suffix) {
			return strings.TrimSpace(value[:len(value)-len(suffix)])
		}
	}
	for _, prefix := range []string{"SH.", "SZ.", "BJ.", "OF."} {
		if strings.HasPrefix(upper, prefix) {
			return strings.TrimSpace(value[len(prefix):])
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) > 1 {
		value = parts[0]
	}
	return value
}

func normalizeStockInstitutionHolderType(value string, holderName string) string {
	text := strings.ToLower(strings.TrimSpace(value + " " + holderName))
	switch {
	case strings.Contains(text, "社保"):
		return "social_security"
	case strings.Contains(text, "qfii") || strings.Contains(text, "rqfii") || strings.Contains(text, "境外"):
		return "qfii"
	case strings.Contains(text, "fund") || strings.Contains(text, "基金"):
		return "fund"
	case strings.Contains(text, "证券") || strings.Contains(text, "券商"):
		return "broker"
	case strings.Contains(text, "保险"):
		return "insurance"
	case strings.Contains(text, "信托"):
		return "trust"
	case strings.Contains(text, "银行") || strings.Contains(text, "理财"):
		return "bank_wealth"
	case strings.Contains(text, "自然人") || strings.Contains(text, "个人"):
		return "natural_person"
	case strings.Contains(text, "机构") || strings.Contains(text, "公司") || strings.Contains(text, "资产") || strings.Contains(text, "资管"):
		return "institution"
	default:
		return "other"
	}
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

func (s *Service) handleSetArticleEmotion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	emotion := nonEmpty(r.FormValue("emotion"), r.FormValue("flag"))
	if err := s.store.SetItemEmotion(r.Context(), id, emotion); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{"emotion": emotion})
}

func (s *Service) handleSetArticleStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid id", nil)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	status := strings.ToLower(strings.TrimSpace(nonEmpty(req.Status, r.URL.Query().Get("status"), r.FormValue("status"))))
	if status == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "status required", nil)
		return
	}
	switch status {
	case "active", "valid", "normal":
		status = "active"
	case "invalid", "inactive", "deleted":
		status = "invalid"
	default:
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported status", nil)
		return
	}
	if err := s.store.SetItemLegacyStatus(r.Context(), id, status); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": status})
}

func (s *Service) handleDeleteArticle(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.MarkItemDeleted(r.Context(), id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"deleted": true})
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

func (s *Service) handleSearchDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", buildSearchDetail(item))
}

func (s *Service) handleSearchMetadataTypes(w http.ResponseWriter, r *http.Request) {
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	parentID := apiutil.IntQuery(r, "parent_id", 0)
	ids := strings.TrimSpace(r.URL.Query().Get("ids"))
	var result []searchMetadataType
	switch level {
	case "", "1":
		result = searchTypesForMode(strings.TrimSpace(r.URL.Query().Get("mode")))
	case "2":
		result = searchTypesBySecond(parentID)
	case "3":
		result = searchTypesByThird(parentID)
	case "ids":
		result = searchTypesByIDs(ids)
	default:
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported metadata level", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleSearchMetadataPolymerizations(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", searchPolymerizations)
}

func (s *Service) handleSearchMetadataBreadcrumbs(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", searchBreadcrumbs(r))
}

func (s *Service) handleSearchSpecialList(w http.ResponseWriter, r *http.Request) {
	kind := normalizeSearchSpecialKind(searchKindParam(r))
	if kind == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported special search kind", nil)
		return
	}
	filter, pageSize := searchSpecialFilterFromRequest(r)
	items, err := s.searchCompatItems(r.Context(), filter, searchSpecialMode(r), 200)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	criteria := searchSpecialCriteriaFromRequest(r)
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		if searchSpecialMatchesItem(kind, item, criteria) {
			filtered = append(filtered, item)
		}
	}
	page := max(filter.Page, 1)
	size := max(filter.PageSize, pageSize)
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	list := make([]map[string]any, 0, end-start)
	for _, item := range filtered[start:end] {
		list = append(list, searchSpecialListEntry(kind, item))
	}
	totalPages := 1
	if size > 0 && len(filtered) > 0 {
		totalPages = (len(filtered) + size - 1) / size
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"code":        "200",
		"msg":         "success",
		"list":        list,
		"totalData":   len(filtered),
		"totalPage":   totalPages,
		"currentPage": page,
	})
}

func (s *Service) handleSearchSpecialOptions(w http.ResponseWriter, r *http.Request) {
	kind := normalizeSearchSpecialKind(searchKindParam(r))
	if kind == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported special search kind", nil)
		return
	}
	filter, _ := searchSpecialFilterFromRequest(r)
	items, err := s.searchCompatItems(r.Context(), filter, searchSpecialMode(r), 200)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusOK, "ok", searchSpecialCategoryOptions(kind))
		return
	}
	options := searchDynamicCategoryOptions(kind, items)
	if len(options) == 0 {
		options = searchSpecialCategoryOptions(kind)
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", options)
}

func (s *Service) handleSearchSpecialDetail(w http.ResponseWriter, r *http.Request) {
	kind := normalizeSearchSpecialKind(searchKindParam(r))
	if kind == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported special search kind", nil)
		return
	}
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", searchSpecialDetailEntry(kind, item))
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

func buildSearchDetail(item model.Item) model.SearchDetail {
	payload := map[string]any{}
	if strings.TrimSpace(item.RawPayload) != "" {
		_ = json.Unmarshal([]byte(item.RawPayload), &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	mergeSearchDetailDefaults(payload, item)
	return model.SearchDetail{
		ID:              item.ID,
		SourceType:      item.SourceType,
		Title:           nonEmpty(stringValue(payload["title"]), item.Title),
		Content:         nonEmpty(stringValue(payload["content"]), item.Content),
		Summary:         nonEmpty(stringValue(payload["summary"]), item.Summary),
		PublishTime:     nonEmpty(stringValue(payload["publish_time"]), item.PublishTime),
		PublishTimeText: nonEmpty(stringValue(payload["publish_time_text"]), item.PublishTimeText),
		DetailURL:       nonEmpty(stringValue(payload["detail_url"]), stringValue(payload["detailUrl"]), item.DetailURL, item.SourceURL),
		SourceURL:       nonEmpty(stringValue(payload["source_url"]), item.SourceURL, item.DetailURL),
		SourceName:      nonEmpty(stringValue(payload["source_name"]), item.FromText, item.SourceType),
		URL:             nonEmpty(stringValue(payload["url"]), stringValue(payload["source_url"]), item.SourceURL, stringValue(payload["detail_url"]), stringValue(payload["detailUrl"]), item.DetailURL),
		Payload:         payload,
	}
}

type searchMetadataType struct {
	OnlyID     int    `json:"only_id"`
	ID         int    `json:"id"`
	CreateTime string `json:"create_time"`
	Type       int    `json:"type"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	TypeOneID  int    `json:"type_one_id"`
	TypeTwoID  int    `json:"type_two_id"`
	Icon       string `json:"icon"`
	IsShow     int    `json:"is_show"`
	IsDefault  int    `json:"is_default"`
}

type searchMetadataPolymerization struct {
	ID         int    `json:"id"`
	CreateTime string `json:"create_time"`
	Type       int    `json:"type"`
	TypeName   string `json:"type_name"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	Icon       string `json:"icon"`
	IsShow     int    `json:"is_show"`
}

var searchTypes = []searchMetadataType{
	{OnlyID: 1, ID: 1, Type: 1, Name: "资讯", Icon: "mdi mdi-newspaper", IsShow: 0, IsDefault: 0},
	{OnlyID: 8, ID: 8, Type: 1, Name: "热点", Icon: "mdi mdi-fire", IsShow: 0, IsDefault: 0},
	{OnlyID: 23, ID: 23, Type: 1, Name: "投诉", Icon: "mdi mdi-alert", IsShow: 0, IsDefault: 0},
	{OnlyID: 28, ID: 28, Type: 1, Name: "公告", Icon: "mdi mdi-bullhorn", IsShow: 0, IsDefault: 0},
	{OnlyID: 35, ID: 35, Type: 1, Name: "研报", Icon: "mdi mdi-chart-line", IsShow: 0, IsDefault: 0},
	{OnlyID: 36, ID: 36, Type: 1, Name: "招聘", Icon: "mdi mdi-account-plus", IsShow: 0, IsDefault: 0},
	{OnlyID: 37, ID: 37, Type: 1, Name: "招标", Icon: "mdi mdi-clipboard-text", IsShow: 0, IsDefault: 0},
	{OnlyID: 38, ID: 38, Type: 1, Name: "资讯聚合", Icon: "mdi mdi-view-list", IsShow: 0, IsDefault: 0},
	{OnlyID: 39, ID: 39, Type: 1, Name: "工商", Icon: "mdi mdi-domain", IsShow: 0, IsDefault: 0},
	{OnlyID: 40, ID: 40, Type: 1, Name: "投资融资", Icon: "mdi mdi-cash-multiple", IsShow: 0, IsDefault: 0},
	{OnlyID: 41, ID: 41, Type: 1, Name: "百度知道", Icon: "mdi mdi-help-circle", IsShow: 0, IsDefault: 0},
	{OnlyID: 42, ID: 42, Type: 1, Name: "法律文书", Icon: "mdi mdi-scale-balance", IsShow: 0, IsDefault: 0},
	{OnlyID: 43, ID: 43, Type: 1, Name: "知识产权", Icon: "mdi mdi-lightbulb", IsShow: 0, IsDefault: 0},
	{OnlyID: 45, ID: 45, Type: 1, Name: "学术", Icon: "mdi mdi-school", IsShow: 0, IsDefault: 0},
	{OnlyID: 100, ID: 100, Type: 1, Name: "律师", Icon: "mdi mdi-account-tie", IsShow: 0, IsDefault: 0},
	{OnlyID: 101, ID: 101, Type: 1, Name: "被执行人", Icon: "mdi mdi-account-alert", IsShow: 0, IsDefault: 0},
	{OnlyID: 102, ID: 102, Type: 1, Name: "专家人才", Icon: "mdi mdi-account-star", IsShow: 0, IsDefault: 0},
	{OnlyID: 103, ID: 103, Type: 1, Name: "医生", Icon: "mdi mdi-hospital", IsShow: 0, IsDefault: 0},
}

var searchPolymerizations = []searchMetadataPolymerization{
	{ID: 1, Type: 0, TypeName: "竞争对手", Name: "竞争对手", Value: "1,100,101", Icon: "mdi mdi-account-group", IsShow: 0},
	{ID: 2, Type: 0, TypeName: "领域范围", Name: "领域范围", Value: "8,102", Icon: "mdi mdi-sitemap", IsShow: 0},
	{ID: 3, Type: 0, TypeName: "政策法规", Name: "政策法规", Value: "23,28", Icon: "mdi mdi-file-document", IsShow: 0},
	{ID: 4, Type: 0, TypeName: "产业市场", Name: "产业市场", Value: "35,39", Icon: "mdi mdi-office-building", IsShow: 0},
	{ID: 5, Type: 0, TypeName: "产品品牌", Name: "产品品牌", Value: "36,37,40,45", Icon: "mdi mdi-tag-multiple", IsShow: 0},
	{ID: 6, Type: 0, TypeName: "技术人才", Name: "技术人才", Value: "41,42,43", Icon: "mdi mdi-flask", IsShow: 0},
}

type searchSpecialCriteria struct {
	Keyword      string
	MatchingMode string
	KindFilter   string
	SourceName   string
	RType        string
}

func searchTypesForMode(_ string) []searchMetadataType {
	return append([]searchMetadataType(nil), searchTypes...)
}

func searchTypesBySecond(typeOneID int) []searchMetadataType {
	result := make([]searchMetadataType, 0)
	for _, item := range searchTypes {
		if item.TypeOneID == typeOneID {
			result = append(result, item)
		}
	}
	return result
}

func searchTypesByThird(typeTwoID int) []searchMetadataType {
	result := make([]searchMetadataType, 0)
	for _, item := range searchTypes {
		if item.TypeTwoID == typeTwoID {
			result = append(result, item)
		}
	}
	return result
}

func searchTypesByIDs(raw string) []searchMetadataType {
	idSet := map[int]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && id > 0 {
			idSet[id] = struct{}{}
		}
	}
	result := make([]searchMetadataType, 0, len(idSet))
	for _, item := range searchTypes {
		if _, ok := idSet[item.OnlyID]; ok {
			result = append(result, item)
		}
	}
	return result
}

func searchBreadcrumbs(r *http.Request) []map[string]any {
	values := []string{}
	if poly := strings.TrimSpace(r.URL.Query().Get("full_poly")); poly != "" {
		values = append(values, "聚合:"+poly)
	}
	if fullType := strings.TrimSpace(r.URL.Query().Get("fulltype")); fullType != "" {
		values = append(values, "类型:"+fullType)
	}
	if onlyID := strings.TrimSpace(r.URL.Query().Get("onlyid")); onlyID != "" {
		values = append(values, "分类:"+onlyID)
	}
	if len(values) == 0 {
		values = append(values, "全文搜索")
	}
	result := make([]map[string]any, 0, len(values))
	for idx, item := range values {
		result = append(result, map[string]any{"id": idx + 1, "name": item})
	}
	return result
}

func searchSpecialMode(r *http.Request) string {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode != "timely" {
		return "full"
	}
	return mode
}

func searchKindParam(r *http.Request) string {
	if kind := strings.TrimSpace(chi.URLParam(r, "kind")); kind != "" {
		return kind
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for idx := range parts {
		if parts[idx] == "special" && idx+1 < len(parts) {
			return parts[idx+1]
		}
	}
	return ""
}

func searchSpecialFilterFromRequest(r *http.Request) (model.ArticleFilter, int) {
	page := apiutil.IntQuery(r, "page", apiutil.IntQuery(r, "pageNum", 1))
	if page <= 0 {
		page = 1
	}
	pageSize := apiutil.IntQuery(r, "page_size", apiutil.IntQuery(r, "pageSize", 25))
	if pageSize <= 0 {
		pageSize = 25
	}
	filter := model.ArticleFilter{
		Page:       page,
		PageSize:   pageSize,
		Keyword:    nonEmpty(strings.TrimSpace(r.URL.Query().Get("q")), strings.TrimSpace(r.URL.Query().Get("searchWord")), strings.TrimSpace(r.URL.Query().Get("searchword")), strings.TrimSpace(r.URL.Query().Get("keyword"))),
		SourceType: strings.TrimSpace(r.URL.Query().Get("source_type")),
		Start:      strings.TrimSpace(r.URL.Query().Get("start")),
		End:        strings.TrimSpace(r.URL.Query().Get("end")),
		Industry:   strings.TrimSpace(r.URL.Query().Get("industry")),
		Province:   strings.TrimSpace(r.URL.Query().Get("province")),
		City:       strings.TrimSpace(r.URL.Query().Get("city")),
		Read:       strings.TrimSpace(r.URL.Query().Get("read")),
		Favorite:   strings.TrimSpace(r.URL.Query().Get("favorite")),
	}
	if projectID, err := strconv.ParseInt(strings.TrimSpace(nonEmpty(r.URL.Query().Get("project_id"), r.URL.Query().Get("projectid"))), 10, 64); err == nil {
		filter.ProjectID = projectID
	}
	return filter, pageSize
}

func (s *Service) searchCompatItems(ctx context.Context, filter model.ArticleFilter, mode string, limit int) ([]model.Item, error) {
	filter.Page = 1
	filter.PageSize = max(limit, 50)
	filter.Mode = mode
	if mode == "timely" && filter.Start == "" {
		filter.Start = time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02")
	}
	result, err := s.store.SearchItemsAdvanced(ctx, filter)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func normalizeSearchSpecialKind(raw string) string {
	switch strings.TrimSpace(raw) {
	case "lawyer", "lawyerList":
		return "lawyer"
	case "executionPerson", "executionPersonList":
		return "executionPerson"
	case "professor", "professorList":
		return "professor"
	case "doctor", "doctorList":
		return "doctor"
	case "bidding", "biddingList":
		return "bidding"
	case "invite", "inviteList":
		return "invite"
	case "company", "companyList":
		return "company"
	case "judgment", "judgmentList":
		return "judgment"
	case "knowledge", "knowLedgeList":
		return "knowledge"
	case "investment", "investmentList":
		return "investment"
	case "baiduKnows", "baiduKnowsList":
		return "baiduKnows"
	case "thesisn", "thesisnList":
		return "thesisn"
	case "report":
		return "report"
	case "announcement":
		return "announcement"
	default:
		return ""
	}
}

func searchSpecialCriteriaFromRequest(r *http.Request) searchSpecialCriteria {
	return searchSpecialCriteria{
		Keyword:      nonEmpty(strings.TrimSpace(r.URL.Query().Get("searchWord")), strings.TrimSpace(r.URL.Query().Get("searchword")), strings.TrimSpace(r.URL.Query().Get("keyword")), strings.TrimSpace(r.URL.Query().Get("q"))),
		MatchingMode: strings.TrimSpace(r.URL.Query().Get("matchingmode")),
		KindFilter:   strings.TrimSpace(r.URL.Query().Get("kinds")),
		SourceName:   strings.TrimSpace(r.URL.Query().Get("source_name")),
		RType:        strings.TrimSpace(r.URL.Query().Get("rtype")),
	}
}

func searchSpecialMatchesItem(kind string, item model.Item, criteria searchSpecialCriteria) bool {
	payload := searchPayloadMap(item)
	if criteria.KindFilter != "" && !searchPayloadContains(payload, criteria.KindFilter) && !searchItemBlobContains(item, criteria.KindFilter) {
		return false
	}
	if criteria.SourceName != "" && criteria.SourceName != "全部" && criteria.SourceName != item.SourceType && criteria.SourceName != item.FromText {
		if !searchPayloadContains(payload, criteria.SourceName) && !searchItemBlobContains(item, criteria.SourceName) {
			return false
		}
	}
	if criteria.RType != "" && criteria.RType != "全部" && !searchPayloadContains(payload, criteria.RType) && !searchItemBlobContains(item, criteria.RType) {
		return false
	}
	if criteria.Keyword == "" {
		return true
	}
	return searchSpecialKeywordMatch(kind, item, payload, criteria.Keyword, criteria.MatchingMode)
}

func searchSpecialKeywordMatch(kind string, item model.Item, payload map[string]any, keyword string, matchingMode string) bool {
	fields := searchSpecialSearchFields(kind, matchingMode)
	if len(fields) == 0 {
		return searchItemBlobContains(item, keyword) || searchPayloadContains(payload, keyword)
	}
	for _, field := range fields {
		if searchPayloadFieldContains(payload, field, keyword) {
			return true
		}
	}
	return false
}

func searchSpecialSearchFields(kind string, matchingMode string) []string {
	switch kind {
	case "lawyer":
		switch matchingMode {
		case "lawpace":
			return []string{"lawfirm"}
		case "lawyerAdept":
			return []string{"goods", "adept"}
		case "lawyerCity":
			return []string{"city"}
		default:
			return []string{"name", "title"}
		}
	case "executionPerson":
		switch matchingMode {
		case "executionPersonArea":
			return []string{"areaNameNew", "province", "city"}
		default:
			return []string{"iname", "name", "title"}
		}
	case "professor":
		switch matchingMode {
		case "professorAdept":
			return []string{"field", "research_field"}
		case "organization":
			return []string{"institution", "source_name"}
		default:
			return []string{"title", "name"}
		}
	case "doctor":
		switch matchingMode {
		case "hospital":
			return []string{"hospital"}
		case "doctorAdept":
			return []string{"adept"}
		case "doctorDept":
			return []string{"department"}
		default:
			return []string{"name", "title"}
		}
	case "judgment":
		switch matchingMode {
		case "parties":
			return []string{"parties"}
		case "court":
			return []string{"court"}
		case "text":
			return []string{"content", "summary"}
		case "area":
			return []string{"province", "city", "area"}
		default:
			return []string{"title", "name"}
		}
	default:
		return nil
	}
}

func searchSpecialListEntry(kind string, item model.Item) map[string]any {
	payload := searchPayloadMap(item)
	base := map[string]any{
		"article_public_id": strconv.FormatInt(item.ID, 10),
		"title":             nonEmpty(searchPayloadString(payload, "title"), item.Title),
		"content":           nonEmpty(searchPayloadString(payload, "content"), item.Content, item.Summary),
		"source_name":       nonEmpty(searchPayloadString(payload, "source_name"), item.FromText, item.SourceType),
		"publish_time":      nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
		"detailUrl":         nonEmpty(searchPayloadString(payload, "detailUrl"), searchPayloadString(payload, "detail_url"), searchPayloadString(payload, "detailurl"), item.SourceURL, item.DetailURL),
		"url":               nonEmpty(searchPayloadString(payload, "url"), searchPayloadString(payload, "source_url"), item.SourceURL, item.DetailURL),
	}
	switch kind {
	case "lawyer":
		base["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		base["telephone"] = nonEmpty(searchPayloadString(payload, "telephone"), searchPayloadString(payload, "phone_number"))
		base["kinds"] = searchPayloadString(payload, "kinds")
		base["goods"] = nonEmpty(searchPayloadString(payload, "goods"), searchPayloadString(payload, "adept"))
		base["educationbackground"] = searchPayloadString(payload, "educationbackground")
		base["email"] = searchPayloadString(payload, "email")
		base["certID"] = searchPayloadString(payload, "certID")
		base["qualifitime"] = searchPayloadString(payload, "qualifitime")
		base["lawfirm"] = searchPayloadString(payload, "lawfirm")
		base["address"] = searchPayloadString(payload, "address")
		base["city"] = searchPayloadString(payload, "city")
	case "executionPerson":
		base["iname"] = nonEmpty(searchPayloadString(payload, "iname"), searchPayloadString(payload, "name"), item.Title)
		base["gistUnit"] = searchPayloadString(payload, "gistUnit")
		base["cardNum"] = searchPayloadString(payload, "cardNum")
		base["type"] = searchPayloadString(payload, "type")
		base["caseCode"] = searchPayloadString(payload, "caseCode")
		base["gistId"] = searchPayloadString(payload, "gistId")
		base["areaNameNew"] = searchPayloadString(payload, "areaNameNew")
		base["courtName"] = searchPayloadString(payload, "courtName")
		base["duty"] = searchPayloadString(payload, "duty")
		base["performance"] = searchPayloadString(payload, "performance")
		base["disruptTypeName"] = searchPayloadString(payload, "disruptTypeName")
	case "professor":
		base["title"] = nonEmpty(searchPayloadString(payload, "title"), searchPayloadString(payload, "name"), item.Title)
		base["avatar"] = nonEmpty(searchPayloadString(payload, "avatar"), searchPayloadString(payload, "profile"))
		base["institution"] = searchPayloadString(payload, "institution")
		base["field"] = searchPayloadJSONArrayString(payload, "field")
		base["works"] = searchPayloadString(payload, "works")
		base["times_cited"] = searchPayloadString(payload, "times_cited")
	case "doctor":
		base["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		base["profile"] = nonEmpty(searchPayloadString(payload, "profile"), searchPayloadString(payload, "avatar"))
		base["hospital"] = searchPayloadString(payload, "hospital")
		base["department"] = searchPayloadString(payload, "department")
		base["province"] = searchPayloadString(payload, "province")
		base["city"] = searchPayloadString(payload, "city")
		base["area"] = searchPayloadString(payload, "area")
		base["degree"] = searchPayloadString(payload, "degree")
		base["adept"] = searchPayloadString(payload, "adept")
	case "company":
		base["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		base["legal_person"] = nonEmpty(searchPayloadString(payload, "legal_person"), searchPayloadString(payload, "legal_representative"))
		base["status"] = searchPayloadString(payload, "status")
		base["registered_capital_str"] = searchPayloadString(payload, "registered_capital_str")
		base["industry_involved"] = nonEmpty(searchPayloadString(payload, "industry_involved"), searchPayloadString(payload, "industry"))
		base["location"] = nonEmpty(searchPayloadString(payload, "location"), searchPayloadString(payload, "address"))
	case "judgment":
		base["caseTitle"] = nonEmpty(searchPayloadString(payload, "caseTitle"), item.Title)
		base["court"] = searchPayloadString(payload, "court")
		base["caseType"] = searchPayloadString(payload, "caseType")
		base["parties"] = searchPayloadString(payload, "parties")
	case "knowledge":
		base["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		base["caseType"] = nonEmpty(searchPayloadString(payload, "caseType"), searchPayloadString(payload, "ip_type"))
		base["owner"] = searchPayloadString(payload, "owner")
	case "investment":
		base["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		base["round"] = nonEmpty(searchPayloadString(payload, "round"), searchPayloadString(payload, "investment_type"))
		base["company"] = searchPayloadString(payload, "company")
	case "baiduKnows", "thesisn", "bidding", "invite":
		base["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
	}
	for key, value := range payload {
		if _, exists := base[key]; !exists {
			base[key] = value
		}
	}
	return base
}

func searchSpecialDetailEntry(kind string, item model.Item) map[string]any {
	entry := searchSpecialListEntry(kind, item)
	payload := searchPayloadMap(item)
	entry["summary"] = nonEmpty(item.Summary, item.Content)
	entry["source_url"] = nonEmpty(item.SourceURL, item.DetailURL)
	entry["publish_time"] = nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05"))
	entry["detailUrl"] = nonEmpty(searchString(entry["detailUrl"]), item.SourceURL, item.DetailURL)
	entry["detail_url"] = nonEmpty(searchPayloadString(payload, "detail_url"), searchPayloadString(payload, "detailUrl"), searchPayloadString(payload, "detailurl"), item.SourceURL, item.DetailURL)
	entry["detailurl"] = nonEmpty(searchPayloadString(payload, "detailurl"), searchString(entry["detail_url"]), searchString(entry["detailUrl"]))
	entry["source_name"] = nonEmpty(searchString(entry["source_name"]), item.FromText, item.SourceType)
	entry["source"] = nonEmpty(searchPayloadString(payload, "source"), item.SourceType)
	switch kind {
	case "company":
		entry["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		entry["phone_number"] = nonEmpty(searchPayloadString(payload, "phone_number"), searchPayloadString(payload, "phone"))
		entry["phone"] = nonEmpty(searchPayloadString(payload, "phone"), searchString(entry["phone_number"]))
		entry["address"] = nonEmpty(searchPayloadString(payload, "address"), searchPayloadString(payload, "location"))
		entry["location"] = nonEmpty(searchPayloadString(payload, "location"), searchString(entry["address"]))
		entry["legal_representative"] = nonEmpty(searchPayloadString(payload, "legal_representative"), searchPayloadString(payload, "legal_person"))
		entry["legal_person"] = nonEmpty(searchPayloadString(payload, "legal_person"), searchString(entry["legal_representative"]))
		entry["uniformSocialCreditCode"] = nonEmpty(searchPayloadString(payload, "uniformSocialCreditCode"), searchPayloadString(payload, "taxpayer_identification"))
		entry["taxpayer_identification"] = nonEmpty(searchPayloadString(payload, "taxpayer_identification"), searchString(entry["uniformSocialCreditCode"]))
		entry["insured_num"] = nonEmpty(searchPayloadString(payload, "insured_num"), searchPayloadString(payload, "insureds"))
		entry["insureds"] = nonEmpty(searchPayloadString(payload, "insureds"), searchString(entry["insured_num"]))
		entry["registration"] = searchPayloadString(payload, "registration")
		entry["enterprise_type"] = searchPayloadString(payload, "enterprise_type")
		entry["registered_capital_str"] = searchPayloadString(payload, "registered_capital_str")
		entry["industry_involved"] = nonEmpty(searchPayloadString(payload, "industry_involved"), searchPayloadString(payload, "industry"))
		entry["business_scope"] = searchPayloadString(payload, "business_scope")
		entry["establish_time"] = nonEmpty(searchPayloadString(payload, "establish_time"), item.PublishTime)
		entry["key_person"] = searchJSONTextPayload(payload, "key_person")
		entry["shareholder"] = searchJSONTextPayload(payload, "shareholder")
		entry["change_record"] = searchJSONTextPayload(payload, "change_record")
	case "report":
		entry["title"] = item.Title
		entry["reportDate"] = nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05"))
		entry["url"] = nonEmpty(item.SourceURL, item.DetailURL)
	case "lawyer":
		entry["img"] = nonEmpty(searchPayloadString(payload, "img"), searchPayloadString(payload, "profile"), searchPayloadString(payload, "avatar"))
		entry["telephone"] = nonEmpty(searchPayloadString(payload, "telephone"), searchPayloadString(payload, "phone_number"))
		entry["detailurl"] = nonEmpty(searchPayloadString(payload, "detailurl"), searchString(entry["detail_url"]), searchString(entry["detailUrl"]))
		entry["name"] = nonEmpty(searchPayloadString(payload, "name"), item.Title)
		entry["goods"] = nonEmpty(searchPayloadString(payload, "goods"), searchPayloadString(payload, "adept"))
		entry["WeChat"] = searchPayloadString(payload, "WeChat")
		entry["microblog"] = searchPayloadString(payload, "microblog")
		entry["tecent"] = searchPayloadString(payload, "tecent")
		entry["status"] = searchPayloadString(payload, "status")
		entry["language"] = searchPayloadString(payload, "language")
		entry["sex"] = searchPayloadString(payload, "sex")
		entry["achievements"] = searchPayloadString(payload, "achievements")
	case "executionPerson":
		entry["photo"] = nonEmpty(searchPayloadString(payload, "photo"), searchPayloadString(payload, "avatar"), searchPayloadString(payload, "img"))
		entry["detailurl"] = nonEmpty(searchPayloadString(payload, "detailurl"), searchString(entry["detail_url"]), searchString(entry["detailUrl"]))
		entry["address"] = searchPayloadString(payload, "address")
		entry["iname"] = nonEmpty(searchPayloadString(payload, "iname"), searchPayloadString(payload, "name"), item.Title)
	case "professor":
		entry["avatar"] = nonEmpty(searchPayloadString(payload, "avatar"), searchPayloadString(payload, "profile"), searchPayloadString(payload, "img"))
		entry["detail_url"] = nonEmpty(searchPayloadString(payload, "detail_url"), searchString(entry["detailUrl"]), searchString(entry["detailurl"]))
		entry["views"] = searchPayloadString(payload, "views")
		entry["field"] = searchPayloadJSONArrayString(payload, "field")
		entry["H_index"] = nonEmpty(searchPayloadString(payload, "H_index"), searchPayloadString(payload, "h_index"))
		entry["G_index"] = nonEmpty(searchPayloadString(payload, "G_index"), searchPayloadString(payload, "g_index"))
		entry["cooperation_agency"] = searchJSONTextPayload(payload, "cooperation_agency")
		entry["periodical"] = searchJSONTextPayload(payload, "periodical")
	case "doctor":
		entry["hospital_url"] = searchPayloadString(payload, "hospital_url")
		entry["phone_number"] = nonEmpty(searchPayloadString(payload, "phone_number"), searchPayloadString(payload, "telephone"))
		entry["location"] = nonEmpty(searchPayloadString(payload, "location"), strings.TrimSpace(strings.Join([]string{searchPayloadString(payload, "province"), searchPayloadString(payload, "city"), searchPayloadString(payload, "area")}, " ")))
		entry["honor"] = searchPayloadString(payload, "honor")
		entry["paper"] = searchPayloadString(payload, "paper")
		entry["detailUrl"] = nonEmpty(searchPayloadString(payload, "detailUrl"), searchString(entry["detail_url"]), searchString(entry["detailurl"]))
		entry["email"] = searchPayloadString(payload, "email")
		entry["postcode"] = searchPayloadString(payload, "postcode")
		entry["administrative_function"] = searchPayloadString(payload, "administrative_function")
	}
	for key, value := range payload {
		if _, exists := entry[key]; !exists {
			entry[key] = value
		}
	}
	return entry
}

func searchDynamicCategoryOptions(kind string, items []model.Item) []map[string]any {
	fieldSets := map[string][][]string{
		"company":    {{"industry_involved", "industry", "industrylable"}},
		"judgment":   {{"caseType", "case_type", "category"}},
		"knowledge":  {{"caseType", "ip_type", "type"}},
		"investment": {{"round", "investment_type", "type"}},
	}
	fields := fieldSets[kind]
	if len(fields) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := []map[string]any{{"id": 0, "value": "", "name": "全部"}}
	nextID := 1
	for _, item := range items {
		payload := searchPayloadMap(item)
		for _, group := range fields {
			value := ""
			for _, field := range group {
				value = searchPayloadString(payload, field)
				if value != "" {
					break
				}
			}
			for _, part := range splitSearchLabels(value) {
				if _, ok := seen[part]; ok || part == "" {
					continue
				}
				seen[part] = struct{}{}
				result = append(result, map[string]any{"id": nextID, "value": part, "name": part})
				nextID++
			}
		}
	}
	return result
}

func searchSpecialCategoryOptions(kind string) []map[string]any {
	switch kind {
	case "announcement":
		return []map[string]any{
			{"value": "", "name": "全部"},
			{"value": "公告", "name": "公告"},
			{"value": "新闻", "name": "新闻"},
		}
	case "report":
		return []map[string]any{
			{"value": "", "name": "全部"},
			{"value": "研报", "name": "研报"},
			{"value": "公告", "name": "公告"},
		}
	default:
		return []map[string]any{{"value": "", "name": "全部"}}
	}
}

func searchPayloadMap(item model.Item) map[string]any {
	payload := map[string]any{}
	if strings.TrimSpace(item.RawPayload) != "" {
		_ = json.Unmarshal([]byte(item.RawPayload), &payload)
	}
	if _, ok := payload["title"]; !ok {
		payload["title"] = item.Title
	}
	if _, ok := payload["content"]; !ok {
		payload["content"] = item.Content
	}
	if _, ok := payload["summary"]; !ok {
		payload["summary"] = item.Summary
	}
	if _, ok := payload["source_name"]; !ok {
		payload["source_name"] = nonEmpty(item.FromText, item.SourceType)
	}
	if _, ok := payload["source_url"]; !ok {
		payload["source_url"] = nonEmpty(item.SourceURL, item.DetailURL)
	}
	if _, ok := payload["detailUrl"]; !ok {
		payload["detailUrl"] = nonEmpty(item.DetailURL, item.SourceURL)
	}
	if _, ok := payload["publish_time"]; !ok {
		payload["publish_time"] = nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05"))
	}
	return payload
}

func searchPayloadContains(payload map[string]any, needle string) bool {
	for _, value := range payload {
		if strings.Contains(strings.ToLower(searchString(value)), strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func searchPayloadFieldContains(payload map[string]any, field string, needle string) bool {
	return strings.Contains(strings.ToLower(searchString(payload[field])), strings.ToLower(strings.TrimSpace(needle)))
}

func searchItemBlobContains(item model.Item, needle string) bool {
	blob := strings.ToLower(strings.Join([]string{item.Title, item.Content, item.Summary, item.RawPayload, item.FromText, item.SourceType}, " "))
	return strings.Contains(blob, strings.ToLower(strings.TrimSpace(needle)))
}

func searchPayloadString(payload map[string]any, key string) string {
	return strings.TrimSpace(searchString(payload[key]))
}

func searchPayloadJSONArrayString(payload map[string]any, key string) string {
	raw := strings.TrimSpace(searchString(payload[key]))
	if raw == "" {
		return ""
	}
	return raw
}

func searchJSONTextPayload(payload map[string]any, key string) string {
	raw := strings.TrimSpace(searchString(payload[key]))
	return raw
}

func searchString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		raw, _ := json.Marshal(typed)
		return string(raw)
	}
}

func splitSearchLabels(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '|' || r == '/' || r == '\n' || r == '\t'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func mergeSearchDetailDefaults(payload map[string]any, item model.Item) {
	if _, ok := payload["id"]; !ok {
		payload["id"] = item.ID
	}
	if _, ok := payload["source_type"]; !ok {
		payload["source_type"] = item.SourceType
	}
	if _, ok := payload["title"]; !ok {
		payload["title"] = item.Title
	}
	if _, ok := payload["content"]; !ok {
		payload["content"] = item.Content
	}
	if _, ok := payload["summary"]; !ok {
		payload["summary"] = item.Summary
	}
	if _, ok := payload["publish_time"]; !ok {
		payload["publish_time"] = nonEmpty(item.PublishTime, item.PublishTimeText)
	}
	if _, ok := payload["publish_time_text"]; !ok {
		payload["publish_time_text"] = nonEmpty(item.PublishTimeText, item.PublishTime)
	}
	if _, ok := payload["detail_url"]; !ok && stringValue(payload["detailUrl"]) == "" {
		payload["detail_url"] = nonEmpty(item.DetailURL, item.SourceURL)
	}
	if _, ok := payload["detailUrl"]; !ok {
		payload["detailUrl"] = nonEmpty(item.DetailURL, item.SourceURL)
	}
	if _, ok := payload["source_url"]; !ok {
		payload["source_url"] = nonEmpty(item.SourceURL, item.DetailURL)
	}
	if _, ok := payload["source_name"]; !ok {
		payload["source_name"] = nonEmpty(item.FromText, item.SourceType)
	}
	if _, ok := payload["url"]; !ok {
		payload["url"] = nonEmpty(item.SourceURL, item.DetailURL)
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
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

func (s *Service) handleBatchDeleteReports(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReportIDs []int64 `json:"report_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.ReportIDs) == 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "report_ids required", nil)
		return
	}
	if err := s.store.BatchDeleteReports(r.Context(), req.ReportIDs); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"updated": len(req.ReportIDs),
		"status":  "archived",
	})
}

func (s *Service) handleBatchUpdateReportStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReportIDs []int64 `json:"report_ids"`
		Status    string  `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.ReportIDs) == 0 {
		apiutil.WriteJSON(w, http.StatusBadRequest, "report_ids required", nil)
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	switch status {
	case "draft", "generated", "archived":
	default:
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported status", nil)
		return
	}
	if err := s.store.BatchUpdateReportStatus(r.Context(), req.ReportIDs, status); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"updated": len(req.ReportIDs),
		"status":  status,
	})
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

func (s *Service) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListFeedback(r.Context(), apiutil.IntQuery(r, "limit", 20))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", items)
}

func (s *Service) handleDeleteFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteFeedback(r.Context(), id); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{"deleted": true, "id": id})
}

func (s *Service) handleListTaskRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListTaskRuns(r.Context(), apiutil.IntQuery(r, "limit", 20))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", runs)
}

func (s *Service) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.store.ListAuditLogs(r.Context(), apiutil.IntQuery(r, "limit", 20), filterUserID(r), strings.TrimSpace(r.URL.Query().Get("action")))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", logs)
}

func (s *Service) handleOperations(w http.ResponseWriter, r *http.Request) {
	ops := s.operationsSummary(r.Context())
	apiutil.WriteJSON(w, http.StatusOK, "ok", ops)
}

func (s *Service) handleAlerts(w http.ResponseWriter, r *http.Request) {
	ops := s.operationsSummary(r.Context())
	alerts := buildOperationsAlerts(ops)
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"ready":        ops.Ready,
		"generated_at": time.Now().UTC(),
		"alerts":       alerts,
	})
}

func (s *Service) handleRestartService(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(s.cfg.ServiceToken) {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	spec, ok := s.serviceRestartSpec(name)
	if !ok {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported service", nil)
		return
	}
	if err := startServiceRestart(spec); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusAccepted, "restart submitted", map[string]string{"service": spec.Name})
}

func (s *Service) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(s.cfg.ServiceToken) {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	spec, ok := s.serviceRestartSpec(name)
	if !ok {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported service", nil)
		return
	}
	lines := apiutil.IntQuery(r, "lines", 200)
	if lines <= 0 {
		lines = 200
	}
	if lines > 1000 {
		lines = 1000
	}
	logText, err := readServiceLogTail(spec, lines)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{
		"service": spec.Name,
		"log":     logText,
	})
}

func (s *Service) operationsSummary(ctx context.Context) model.OperationsSummary {
	taskRuns, _ := s.store.ListTaskRuns(ctx, 50)
	auditLogs, _ := s.store.ListAuditLogs(ctx, 20, 0, "")
	failedRuns := make([]model.TaskRun, 0)
	for _, run := range taskRuns {
		if run.Status == "failed" {
			failedRuns = append(failedRuns, run)
		}
	}
	ops := model.OperationsSummary{
		GeneratedAt:          time.Now().UTC(),
		Services:             s.operationServiceStatuses(ctx),
		SchedulerJobs:        s.schedulerJobStatuses(ctx),
		TaskSummary:          taskSummary(taskRuns, failedRuns),
		RecentTaskRuns:       taskRuns,
		FailedTaskRuns:       failedRuns,
		AuditSummary:         auditSummary(auditLogs),
		RecentAuditLogs:      auditLogs,
		LegacyRegistry:       legacyRegistryCounts(),
		LegacyRouteProbes:    s.legacyRouteProbeStatuses(ctx),
		ExternalIntegrations: s.externalIntegrationStatuses(ctx),
		Backup:               s.backupStatus(),
		Database:             s.databaseConfigStatus(ctx, databaseConfigRequest{}),
	}
	ops.Ready = len(buildOperationsAlerts(ops)) == 0
	return ops
}

type serviceRestartSpec struct {
	Name       string
	Path       string
	BinaryPath string
	Port       int
	Ports      []int
	Root       string
}

func (s *Service) serviceRestartSpec(name string) (serviceRestartSpec, bool) {
	services := map[string]struct {
		path string
		addr string
	}{
		"auth-service":      {path: ".\\cmd\\auth-service", addr: s.cfg.AuthAddr},
		"wechat-service":    {path: ".\\cmd\\wechat-service", addr: s.cfg.WechatAddr},
		"content-service":   {path: ".\\cmd\\content-service", addr: s.cfg.ContentAddr},
		"crawler-service":   {path: ".\\cmd\\crawler-service", addr: s.cfg.CrawlerAddr},
		"analysis-service":  {path: ".\\cmd\\analysis-service", addr: s.cfg.AnalysisAddr},
		"nlp-service":       {path: ".\\cmd\\nlp-service", addr: s.cfg.NLPAddr},
		"scheduler-service": {path: ".\\cmd\\scheduler-service", addr: s.cfg.SchedulerAddr},
		"gateway-web":       {path: ".\\cmd\\gateway-web", addr: s.cfg.GatewayWebAddr},
	}
	def, ok := services[name]
	if !ok {
		return serviceRestartSpec{}, false
	}
	port := portFromAddr(def.addr)
	if port <= 0 {
		return serviceRestartSpec{}, false
	}
	ports := []int{port}
	if name == "gateway-web" {
		for _, addr := range s.gatewayWebListenAddrs() {
			if extraPort := portFromAddr(addr); extraPort > 0 {
				ports = append(ports, extraPort)
			}
		}
	}
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	return serviceRestartSpec{
		Name:       name,
		Path:       def.path,
		BinaryPath: filepath.Join(root, "bin", name+".exe"),
		Port:       port,
		Ports:      uniqueServiceRestartPorts(ports),
		Root:       root,
	}, true
}

func (s *Service) gatewayWebListenAddrs() []string {
	addrs := append([]string(nil), s.cfg.GatewayWebHTTPAddrs...)
	addrs = append(addrs, s.cfg.GatewayWebRedirectAddr, s.cfg.GatewayWebTLSAddr)
	return addrs
}

func uniqueServiceRestartPorts(ports []int) []int {
	seen := make(map[int]struct{}, len(ports))
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	return out
}

func portFromAddr(addr string) int {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0
	}
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		addr = addr[idx+1:]
	}
	port, err := strconv.Atoi(addr)
	if err != nil || port <= 0 {
		return 0
	}
	return port
}

var startServiceRestart = func(spec serviceRestartSpec) error {
	ports := spec.Ports
	if len(ports) == 0 && spec.Port > 0 {
		ports = []int{spec.Port}
	}
	script := fmt.Sprintf(`
$ErrorActionPreference = "SilentlyContinue"
Start-Sleep -Seconds 1
$root = %q
$name = %q
$servicePath = %q
$binaryPath = %q
$ports = @(%s)
$pidDir = Join-Path $root "runtime-pids"
New-Item -ItemType Directory -Force -Path $pidDir | Out-Null
$pidFile = Join-Path $pidDir ($name + ".pid")
function Stop-ProcessTree($targetPid) {
    if (-not $targetPid) { return }
    Get-CimInstance Win32_Process -Filter ("ParentProcessId=" + $targetPid) -ErrorAction SilentlyContinue | ForEach-Object {
        Stop-ProcessTree $_.ProcessId
    }
    $taskkill = Join-Path $env:SystemRoot "System32\taskkill.exe"
    if (-not (Test-Path $taskkill)) { $taskkill = "taskkill.exe" }
    & $taskkill /F /T /PID $targetPid *> $null
    if (Get-Process -Id $targetPid -ErrorAction SilentlyContinue) {
        Stop-Process -Id $targetPid -Force -ErrorAction SilentlyContinue
    }
}
if (Test-Path $pidFile) {
    $oldPid = Get-Content $pidFile -ErrorAction SilentlyContinue
    if ($oldPid) {
        Stop-ProcessTree $oldPid
    }
    Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
}
$ports | ForEach-Object {
    $port = $_
    Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique | ForEach-Object {
        Stop-ProcessTree $_
    }
}
Start-Sleep -Milliseconds 500
if (Test-Path $binaryPath) {
    $process = Start-Process -FilePath $binaryPath -WorkingDirectory $root -PassThru -WindowStyle Hidden
} else {
    $process = Start-Process -FilePath "go" -ArgumentList @("run", $servicePath) -WorkingDirectory $root -PassThru -WindowStyle Hidden
}
Set-Content -Path $pidFile -Value $process.Id
`, spec.Root, spec.Name, spec.Path, spec.BinaryPath, serviceRestartPortsPowerShellLiteral(ports))
	return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Start()
}

func serviceRestartPortsPowerShellLiteral(ports []int) string {
	if len(ports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		if port > 0 {
			parts = append(parts, strconv.Itoa(port))
		}
	}
	return strings.Join(parts, ",")
}

var startAllServicesRestart = func() error {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	script := fmt.Sprintf(`
$ErrorActionPreference = "SilentlyContinue"
Start-Sleep -Seconds 1
$root = %q
$stopScript = Join-Path $root "scripts\stop-all.ps1"
$startScript = Join-Path $root "scripts\start-all.ps1"
if (Test-Path $stopScript) {
    & powershell -NoProfile -ExecutionPolicy Bypass -File $stopScript -Root $root
}
Start-Sleep -Seconds 1
if (Test-Path $startScript) {
    & powershell -NoProfile -ExecutionPolicy Bypass -File $startScript -Root $root
}
`, root)
	return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script).Start()
}

func readServiceLogTail(spec serviceRestartSpec, lines int) (string, error) {
	logPath := filepath.Join(spec.Root, "runtime-logs", spec.Name+".out.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return "", nil
	}
	parts := strings.Split(text, "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n"), nil
}

func (s *Service) operationServiceStatuses(ctx context.Context) []model.OperationServiceStatus {
	targets := []struct {
		name string
		url  string
	}{
		{name: "gateway-web", url: s.cfg.GatewayWebURL},
		{name: "auth-service", url: s.cfg.AuthURL},
		{name: "wechat-service", url: s.cfg.WechatURL},
		{name: "content-service", url: s.cfg.ContentURL},
		{name: "crawler-service", url: s.cfg.CrawlerURL},
		{name: "analysis-service", url: s.cfg.AnalysisURL},
		{name: "nlp-service", url: s.cfg.NLPURL},
		{name: "scheduler-service", url: s.cfg.SchedulerURL},
		{name: "release-service", url: s.cfg.ReleaseURL},
	}
	result := make([]model.OperationServiceStatus, 0, len(targets))
	for _, target := range targets {
		if strings.TrimSpace(target.url) == "" {
			continue
		}
		result = append(result, s.probeOperationServiceStatus(ctx, target.name, target.url))
	}
	return result
}

func (s *Service) probeOperationServiceStatus(ctx context.Context, name string, baseURL string) model.OperationServiceStatus {
	status := model.OperationServiceStatus{
		Name:   name,
		URL:    strings.TrimRight(baseURL, "/") + "/healthy",
		Status: "failed",
	}
	resp, err := s.client.R().SetContext(ctx).Get(status.URL)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	fallbackMessage := resp.Status()
	if resp.IsSuccess() {
		fallbackMessage = "ok"
	}
	health := apiutil.CoerceHealthPayload(resp.IsSuccess(), name, resp.Body(), fallbackMessage)
	status.Status = health.Status
	status.Healthy = health.Healthy
	status.Message = health.Message
	return status
}

func (s *Service) externalIntegrationStatuses(ctx context.Context) []model.OperationExternalStatus {
	result := []model.OperationExternalStatus{
		{Name: "jin10_full", Status: boolStatus(s.cfg.Jin10FullEnabled), Message: "controlled by YUQING_JIN10_FULL_ENABLED"},
		{Name: "eastmoney_kuaixun", Status: boolStatus(strings.TrimSpace(s.cfg.EastMoneyKuaixunURL) != ""), Message: s.cfg.EastMoneyKuaixunURL},
		{Name: "eastmoney_full", Status: boolStatus(strings.TrimSpace(s.cfg.EastMoneyKuaixunURL) != ""), Message: s.cfg.EastMoneyKuaixunURL},
		{Name: "wallstreetcn_a_stock", Status: boolStatus(strings.TrimSpace(s.cfg.WallStreetCNAStockURL) != ""), Message: s.cfg.WallStreetCNAStockURL},
		{Name: "cls_telegraph", Status: boolStatus(strings.TrimSpace(s.cfg.CLSTelegraphURL) != ""), Message: s.cfg.CLSTelegraphURL},
		{Name: "sina_finance_7x24", Status: boolStatus(strings.TrimSpace(s.cfg.SinaFinance7x24URL) != ""), Message: s.cfg.SinaFinance7x24URL},
		s.cryptoSocialStatus(ctx, "crypto_x", s.cfg.CryptoXURL),
		s.cryptoSocialStatus(ctx, "crypto_telegram", s.cfg.CryptoTelegramURL),
		{Name: "binance", Status: "configured", Message: s.cfg.BinanceBaseURL},
		{Name: "coinlore", Status: "configured", Message: s.cfg.CoinLoreURL},
		{Name: "coingecko", Status: "configured", Message: s.cfg.CoinGeckoURL},
	}
	if strings.TrimSpace(s.cfg.NLPURL) != "" {
		status := model.OperationExternalStatus{Name: "nlp-service", Status: "failed", Message: s.cfg.NLPURL}
		resp, err := s.client.R().SetContext(ctx).Get(strings.TrimRight(s.cfg.NLPURL, "/") + "/api/v1/nlp/capabilities")
		if err != nil {
			status.Message = err.Error()
		} else if resp.IsSuccess() {
			status.Status = "ok"
			status.Message = "capabilities ok"
		} else {
			status.Message = resp.Status()
		}
		result = append(result, status)
	}
	return result
}

func (s *Service) schedulerJobStatuses(ctx context.Context) []model.OperationSchedulerJob {
	if strings.TrimSpace(s.cfg.SchedulerURL) == "" {
		return nil
	}
	var envelope struct {
		Data []model.OperationSchedulerJob `json:"data"`
	}
	resp, err := s.client.R().
		SetContext(ctx).
		SetResult(&envelope).
		Get(strings.TrimRight(s.cfg.SchedulerURL, "/") + "/api/v1/scheduler/jobs")
	if err != nil || resp == nil || !resp.IsSuccess() {
		return nil
	}
	return envelope.Data
}

func (s *Service) cryptoSocialStatus(ctx context.Context, sourceType, endpoint string) model.OperationExternalStatus {
	if strings.TrimSpace(endpoint) == "" {
		return model.OperationExternalStatus{Name: sourceType, Status: "disabled", Message: "external_disabled"}
	}
	runs, err := s.store.ListCrawlRuns(ctx, 1, sourceType)
	if err != nil {
		return model.OperationExternalStatus{Name: sourceType, Status: "failed", Message: err.Error()}
	}
	if len(runs) == 0 {
		return model.OperationExternalStatus{Name: sourceType, Status: "warning", Message: "no crawl run recorded"}
	}
	run := runs[0]
	status := run.Status
	if status == "" {
		status = "unknown"
	}
	message := fmt.Sprintf("last=%s fetched=%d inserted=%d", run.StartedAt.Format(time.RFC3339), run.FetchedCount, run.InsertedCount)
	if run.ErrorText != "" {
		message += " error=" + run.ErrorText
	}
	duplicateCount := run.FetchedCount - run.InsertedCount - run.UpdatedCount
	if duplicateCount < 0 {
		duplicateCount = 0
	}
	return model.OperationExternalStatus{
		Name:           sourceType,
		Status:         status,
		Message:        message,
		LastFetchAt:    &run.StartedAt,
		FetchedCount:   run.FetchedCount,
		InsertedCount:  run.InsertedCount,
		UpdatedCount:   run.UpdatedCount,
		DuplicateCount: duplicateCount,
	}
}

func boolStatus(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func externalURLMessage(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "external_disabled"
	}
	return raw
}

func legacyRegistryCounts() []model.LegacyStrategyCount {
	return []model.LegacyStrategyCount{
		{Strategy: "delete", Count: 40},
		{Strategy: "gone", Count: 75},
		{Strategy: "preserve", Count: 0},
		{Strategy: "proxy", Count: 0},
	}
}

func (s *Service) backupStatus() model.OperationBackupStatus {
	dbPath := strings.TrimSpace(s.cfg.DatabasePath)
	if dbPath == "" {
		dbPath = filepath.Join("data", "yuqing.db")
	}
	backupDir := filepath.Join(filepath.Dir(dbPath), "..", "backups")
	if !filepath.IsAbs(backupDir) {
		backupDir = filepath.Clean(backupDir)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return model.OperationBackupStatus{Name: "sqlite_backup", Status: "warning", Message: "backup directory not found"}
	}
	var latest os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latest == nil || info.ModTime().After(latest.ModTime()) {
			latest = info
		}
	}
	if latest == nil {
		return model.OperationBackupStatus{Name: "sqlite_backup", Status: "warning", Message: "no backup db found"}
	}
	latestAt := latest.ModTime().UTC()
	status := "ok"
	message := latest.Name()
	if time.Since(latestAt) > 7*24*time.Hour {
		status = "warning"
		message = "latest backup is older than 7 days"
	}
	return model.OperationBackupStatus{
		Name:         "sqlite_backup",
		Status:       status,
		Message:      message,
		Path:         filepath.Join(backupDir, latest.Name()),
		SizeBytes:    latest.Size(),
		LastBackupAt: &latestAt,
		AgeHours:     time.Since(latestAt).Hours(),
	}
}

func buildOperationsAlerts(ops model.OperationsSummary) []model.OperationAlert {
	now := time.Now().UTC()
	alerts := make([]model.OperationAlert, 0)
	for _, service := range ops.Services {
		if !service.Healthy {
			alerts = append(alerts, model.OperationAlert{Name: "service_unhealthy", Severity: "critical", Status: "firing", Message: service.Name + ": " + service.Message, CreatedAt: now})
		}
	}
	if len(ops.FailedTaskRuns) > 0 {
		alerts = append(alerts, model.OperationAlert{Name: "failed_task_runs", Severity: "critical", Status: "firing", Message: fmt.Sprintf("%d failed task runs", len(ops.FailedTaskRuns)), CreatedAt: now})
	}
	if ops.TaskSummary.ConsecutiveFailures >= 2 {
		alerts = append(alerts, model.OperationAlert{Name: "consecutive_task_failures", Severity: "critical", Status: "firing", Message: fmt.Sprintf("%d consecutive task failures: %s", ops.TaskSummary.ConsecutiveFailures, ops.TaskSummary.ConsecutiveFailureTask), CreatedAt: now})
	}
	if ops.AuditSummary.RecentCount == 0 {
		alerts = append(alerts, model.OperationAlert{Name: "recent_audit_missing", Severity: "warning", Status: "firing", Message: "no recent audit logs returned", CreatedAt: now})
	} else if ops.AuditSummary.LastAgeSec > int64((24 * time.Hour / time.Second)) {
		alerts = append(alerts, model.OperationAlert{Name: "recent_audit_stale", Severity: "warning", Status: "firing", Message: "latest audit log is older than 24 hours", CreatedAt: now})
	}
	if ops.Backup.Status == "warning" || ops.Backup.Status == "failed" {
		alerts = append(alerts, model.OperationAlert{Name: "backup_not_ready", Severity: "warning", Status: "firing", Message: ops.Backup.Message, CreatedAt: now})
	}
	for _, count := range ops.LegacyRegistry {
		if (count.Strategy == "proxy" || count.Strategy == "preserve") && count.Count > 0 {
			alerts = append(alerts, model.OperationAlert{Name: "legacy_live_routes", Severity: "critical", Status: "firing", Message: fmt.Sprintf("%s=%d", count.Strategy, count.Count), CreatedAt: now})
		}
	}
	for _, probe := range ops.LegacyRouteProbes {
		if probe.Status != "gone" {
			alerts = append(alerts, model.OperationAlert{Name: "legacy_non_410", Severity: "critical", Status: "firing", Message: fmt.Sprintf("%s returned %d", probe.Path, probe.Actual), CreatedAt: now})
		}
	}
	for _, external := range ops.ExternalIntegrations {
		switch external.Status {
		case "failed":
			alerts = append(alerts, model.OperationAlert{Name: "external_integration_failed", Severity: "warning", Status: "firing", Message: external.Name + ": " + external.Message, CreatedAt: now})
		case "success", "ok":
			if strings.HasPrefix(external.Name, "crypto_") && external.LastFetchAt != nil && external.InsertedCount == 0 && now.Sub(*external.LastFetchAt) > 6*time.Hour {
				alerts = append(alerts, model.OperationAlert{Name: "crypto_social_no_recent_insert", Severity: "warning", Status: "firing", Message: external.Name + " has no inserted rows in latest run", CreatedAt: now})
			}
		}
	}
	return alerts
}

func taskSummary(runs []model.TaskRun, failedRuns []model.TaskRun) model.OperationTaskSummary {
	summary := model.OperationTaskSummary{
		RecentCount: len(runs),
		FailedCount: len(failedRuns),
	}
	if len(runs) == 0 {
		return summary
	}
	latest := runs[0]
	summary.LastTaskName = latest.TaskName
	summary.LastStatus = latest.Status
	summary.LastStartedAt = &latest.StartedAt
	summary.LastFinishedAt = latest.FinishedAt
	for _, run := range runs {
		if run.Status != "failed" {
			break
		}
		summary.ConsecutiveFailures++
		if summary.ConsecutiveFailureTask == "" {
			summary.ConsecutiveFailureTask = run.TaskName
			summary.ConsecutiveFailureReason = run.Message
		}
	}
	return summary
}

func auditSummary(logs []model.AuditLog) model.OperationAuditSummary {
	summary := model.OperationAuditSummary{RecentCount: len(logs)}
	if len(logs) == 0 {
		return summary
	}
	latest := logs[0]
	summary.LastAction = latest.Action
	summary.LastAt = &latest.CreatedAt
	if !latest.CreatedAt.IsZero() {
		summary.LastAgeSec = int64(time.Since(latest.CreatedAt).Seconds())
	}
	return summary
}

func (s *Service) legacyRouteProbeStatuses(ctx context.Context) []model.OperationLegacyRouteStatus {
	if strings.TrimSpace(s.cfg.GatewayWebURL) == "" {
		return nil
	}
	probes := []struct {
		path   string
		formal string
	}{
		{path: "/timelysearch", formal: "/articles?mode=timely"},
		{path: "/platform/nlp/capabilities", formal: "/api/v1/nlp/capabilities"},
		{path: "/platform/xie/report", formal: "/api/v1/nlp/report-preview"},
		{path: "/mobile/monitor", formal: "/articles"},
		{path: "/displayboard", formal: "/system?section=operations"},
		{path: "/volume", formal: "/api/v1/analysis/sources"},
		{path: "/hot/hotpage", formal: "/api/v1/search/hot-keywords"},
		{path: "/dist/monitor", formal: "/login"},
		{path: "/img/code", formal: "/login"},
	}
	result := make([]model.OperationLegacyRouteStatus, 0, len(probes))
	base := strings.TrimRight(s.cfg.GatewayWebURL, "/")
	for _, probe := range probes {
		status := model.OperationLegacyRouteStatus{
			Path:       probe.path,
			Expected:   http.StatusGone,
			Status:     "unknown",
			FormalPath: probe.formal,
		}
		resp, err := s.client.R().SetContext(ctx).Get(base + probe.path)
		if err != nil {
			status.Status = "failed"
			status.Message = err.Error()
		} else {
			status.Actual = resp.StatusCode()
			status.Message = resp.Status()
			if resp.StatusCode() == http.StatusGone {
				status.Status = "gone"
			} else {
				status.Status = "unexpected"
			}
		}
		result = append(result, status)
	}
	return result
}

func (s *Service) handleCreateAuditLog(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(s.cfg.ServiceToken) {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	var entry model.AuditLog
	if !decodeJSON(w, r, &entry) {
		return
	}
	created, err := s.store.CreateAuditLog(r.Context(), entry)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusCreated, "ok", created)
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

func (s *Service) handleGetReleaseSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetReleaseSettings(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", settings)
}

func (s *Service) handleUpdateReleaseSettings(w http.ResponseWriter, r *http.Request) {
	var settings model.ReleaseSettings
	if !decodeJSON(w, r, &settings) {
		return
	}
	updated, err := s.store.UpsertReleaseSettings(r.Context(), settings)
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
		TimeField:  strings.TrimSpace(r.URL.Query().Get("time_field")),
		Industry:   strings.TrimSpace(r.URL.Query().Get("industry")),
		Province:   strings.TrimSpace(r.URL.Query().Get("province")),
		City:       strings.TrimSpace(r.URL.Query().Get("city")),
		Sort:       strings.TrimSpace(r.URL.Query().Get("sort")),
		Read:       strings.TrimSpace(r.URL.Query().Get("read")),
		Favorite:   strings.TrimSpace(r.URL.Query().Get("favorite")),
		Lite:       normalizeBoolQuery(r.URL.Query().Get("lite")),
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
