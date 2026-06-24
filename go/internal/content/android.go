package content

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

const androidPackageName = "com.jiansutech.yuqing"

type androidModule struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Category    string `json:"category"`
	RequiresOp  bool   `json:"requires_operation"`
}

type androidBootstrap struct {
	PackageName string          `json:"package_name"`
	AppName     string          `json:"app_name"`
	APIVersion  string          `json:"api_version"`
	GeneratedAt time.Time       `json:"generated_at"`
	User        androidUser     `json:"user"`
	Modules     []androidModule `json:"modules"`
	Actions     []string        `json:"actions"`
}

type androidUser struct {
	ID       int64  `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

type androidDashboard struct {
	GeneratedAt   time.Time                               `json:"generated_at"`
	Overview      model.Overview                          `json:"overview"`
	Projects      []model.Project                         `json:"projects"`
	Rules         []model.MonitorRule                     `json:"rules"`
	Articles      model.ItemListResult                    `json:"articles"`
	Reports       []model.Report                          `json:"reports"`
	Notices       []model.SystemNotice                    `json:"notices"`
	TaskRuns      []model.TaskRun                         `json:"task_runs"`
	CrawlRuns     []model.CrawlRun                        `json:"crawl_runs"`
	Operations    model.OperationsSummary                 `json:"operations"`
	AStock        androidAStockDashboard                  `json:"a_stock"`
	StockResearch model.StockResearchListResult           `json:"stock_research"`
	Holdings      model.StockInstitutionHoldingListResult `json:"holdings"`
	PartialErrors map[string]string                       `json:"partial_errors,omitempty"`
}

type androidAStockDashboard struct {
	Auction model.AStockAuctionListResult `json:"auction"`
}

type androidActionRequest struct {
	Params map[string]string `json:"params"`
}

type androidActionResponse struct {
	Action    string `json:"action"`
	Status    string `json:"status"`
	TargetURL string `json:"target_url,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (s *Service) handleAndroidBootstrap(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", androidBootstrap{
		PackageName: androidPackageName,
		AppName:     "简苏舆情",
		APIVersion:  "v1",
		GeneratedAt: time.Now().UTC(),
		User: androidUser{
			ID:       auditUserID(r),
			Username: strings.TrimSpace(r.Header.Get("X-User-Name")),
			Role:     strings.TrimSpace(r.Header.Get("X-User-Role")),
		},
		Modules: androidModules(),
		Actions: androidSupportedActions(),
	})
}

func (s *Service) handleAndroidModules(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", androidModules())
}

func (s *Service) handleAndroidDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	errs := map[string]string{}
	projects := []model.Project{}
	if data, err := s.store.ListProjects(ctx); err != nil {
		errs["projects"] = err.Error()
	} else {
		projects = data
	}
	rules := []model.MonitorRule{}
	if data, err := s.store.ListMonitorRules(ctx); err != nil {
		errs["rules"] = err.Error()
	} else {
		rules = data
	}
	articles := model.ItemListResult{Page: 1, PageSize: 10}
	if data, err := s.store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Sort: "publish_time_desc", UserID: auditUserID(r)}); err != nil {
		errs["articles"] = err.Error()
	} else {
		articles = data
	}
	reports := []model.Report{}
	if data, err := s.store.ListReports(ctx, 0); err != nil {
		errs["reports"] = err.Error()
	} else {
		reports = limitReports(data, 10)
	}
	notices := []model.SystemNotice{}
	if data, err := s.store.ListNotices(ctx); err != nil {
		errs["notices"] = err.Error()
	} else {
		notices = data
	}
	taskRuns := []model.TaskRun{}
	if data, err := s.store.ListTaskRuns(ctx, 10); err != nil {
		errs["task_runs"] = err.Error()
	} else {
		taskRuns = data
	}
	crawlRuns := []model.CrawlRun{}
	if data, err := s.store.ListCrawlRuns(ctx, 10, ""); err != nil {
		errs["crawl_runs"] = err.Error()
	} else {
		crawlRuns = data
	}
	auction := model.AStockAuctionListResult{Page: 1, PageSize: 10}
	if data, err := s.store.ListAStockAuctionAmounts(ctx, model.AStockAuctionFilter{Page: 1, PageSize: 10}); err != nil {
		errs["a_stock_auction"] = err.Error()
	} else {
		auction = data
	}
	stockResearch := model.StockResearchListResult{Page: 1, PageSize: stockResearchDefaultPageSize}
	if data, err := s.store.ListStockResearchSurveys(ctx, model.StockResearchFilter{Page: 1, PageSize: stockResearchDefaultPageSize}); err != nil {
		errs["stock_research"] = err.Error()
	} else {
		stockResearch = data
	}
	holdings := model.StockInstitutionHoldingListResult{Page: 1, PageSize: 10}
	if data, err := s.store.ListStockInstitutionHoldings(ctx, model.StockInstitutionHoldingFilter{Page: 1, PageSize: 10}); err != nil {
		errs["holdings"] = err.Error()
	} else {
		holdings = data
	}
	payload := androidDashboard{
		GeneratedAt:   time.Now().UTC(),
		Overview:      androidOverview(articles, projects, reports, rules, crawlRuns),
		Projects:      projects,
		Rules:         rules,
		Articles:      articles,
		Reports:       reports,
		Notices:       notices,
		TaskRuns:      taskRuns,
		CrawlRuns:     crawlRuns,
		Operations:    s.operationsSummary(ctx),
		AStock:        androidAStockDashboard{Auction: auction},
		StockResearch: stockResearch,
		Holdings:      holdings,
		PartialErrors: errs,
	}
	if len(payload.PartialErrors) == 0 {
		payload.PartialErrors = nil
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", payload)
}

func (s *Service) handleAndroidAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(chi.URLParam(r, "action"))
	if action == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "action required", nil)
		return
	}
	var payload androidActionRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
			return
		}
	}
	if payload.Params == nil {
		payload.Params = map[string]string{}
	}
	if action == "service_restart" {
		s.handleAndroidServiceRestart(w, r, action, payload.Params)
		return
	}
	method, endpoint, ok := s.androidActionTarget(action, payload.Params)
	if !ok {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported android action", nil)
		return
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Execute(method, endpoint)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), androidActionResponse{Action: action, Status: "failed", TargetURL: endpoint})
		return
	}
	if !resp.IsSuccess() {
		apiutil.WriteJSON(w, resp.StatusCode(), resp.Status(), androidActionResponse{Action: action, Status: "failed", TargetURL: endpoint, Message: string(resp.Body())})
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", androidActionResponse{Action: action, Status: "triggered", TargetURL: endpoint})
}

func (s *Service) handleAndroidServiceRestart(w http.ResponseWriter, r *http.Request, action string, params map[string]string) {
	name := strings.TrimSpace(params["service"])
	if name == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "service required", nil)
		return
	}
	spec, ok := s.serviceRestartSpec(name)
	if !ok {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported service", nil)
		return
	}
	if err := startServiceRestart(spec); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusAccepted, "restart submitted", androidActionResponse{Action: action, Status: "triggered", Message: spec.Name})
}

func (s *Service) androidActionTarget(action string, params map[string]string) (string, string, bool) {
	query := actionQuery(params)
	switch action {
	case "refresh_analysis":
		return http.MethodPost, strings.TrimRight(s.cfg.AnalysisURL, "/") + "/api/v1/admin/tasks/analysis/refresh", true
	case "run_scheduler_job":
		name := strings.TrimSpace(params["name"])
		if name == "" {
			return "", "", false
		}
		return http.MethodPost, strings.TrimRight(s.cfg.SchedulerURL, "/") + "/api/v1/scheduler/jobs/" + url.PathEscape(name) + "/run", true
	case "run_crawl":
		return http.MethodPost, withQuery(strings.TrimRight(s.cfg.CrawlerURL, "/")+"/api/v1/admin/tasks/crawl", query), true
	case "stock_research_backfill":
		return http.MethodPost, withQuery(strings.TrimRight(s.cfg.SchedulerURL, "/")+"/api/v1/scheduler/stock-research/backfill", query), true
	case "stock_research_pdf_parse":
		return http.MethodPost, withQuery(strings.TrimRight(s.cfg.SchedulerURL, "/")+"/api/v1/scheduler/stock-research/pdf/parse", query), true
	case "investor_relations_backfill":
		return http.MethodPost, withQuery(strings.TrimRight(s.cfg.SchedulerURL, "/")+"/api/v1/scheduler/investor-relations/backfill", query), true
	case "a_stock_auction_latest":
		return http.MethodPost, strings.TrimRight(s.cfg.SchedulerURL, "/") + "/api/v1/scheduler/a-stock/auction/latest", true
	case "a_stock_auction_backfill":
		return http.MethodPost, withQuery(strings.TrimRight(s.cfg.SchedulerURL, "/")+"/api/v1/scheduler/a-stock/auction/backfill", query), true
	case "a_stock_holdings_backfill":
		return http.MethodPost, withQuery(strings.TrimRight(s.cfg.SchedulerURL, "/")+"/api/v1/scheduler/a-stock/holdings/backfill", query), true
	default:
		return "", "", false
	}
}

func androidModules() []androidModule {
	return []androidModule{
		{Key: "dashboard", Title: "总览", Description: "核心指标、最新内容、任务状态", Path: "dashboard", Category: "workbench"},
		{Key: "projects", Title: "项目", Description: "项目与监测规则", Path: "projects", Category: "public_opinion", RequiresOp: true},
		{Key: "articles", Title: "文章", Description: "文章流、已读、收藏、详情", Path: "articles", Category: "public_opinion"},
		{Key: "search", Title: "搜索", Description: "全文搜索、及时搜索、专项搜索", Path: "search", Category: "public_opinion"},
		{Key: "analysis", Title: "分析", Description: "舆情分析、情绪、热点、传播", Path: "analysis", Category: "public_opinion"},
		{Key: "reports", Title: "报告", Description: "报告列表、详情、生成", Path: "reports", Category: "public_opinion", RequiresOp: true},
		{Key: "a_stock", Title: "A股", Description: "推荐、交易日、集合竞价", Path: "a-stock", Category: "finance", RequiresOp: true},
		{Key: "stock_research", Title: "研报调研", Description: "研报、调研、PDF 解析", Path: "stock-research", Category: "finance", RequiresOp: true},
		{Key: "holdings", Title: "机构持仓", Description: "机构持仓与汇总", Path: "holdings", Category: "finance"},
		{Key: "investor_relations", Title: "投资者关系", Description: "投资者关系记录补抓", Path: "investor-relations", Category: "finance", RequiresOp: true},
		{Key: "crypto", Title: "加密资讯", Description: "加密新闻、社交源与洞察", Path: "crypto", Category: "finance"},
		{Key: "system", Title: "系统", Description: "任务、日志、服务、配置", Path: "system", Category: "admin", RequiresOp: true},
	}
}

func androidSupportedActions() []string {
	return []string{
		"refresh_analysis",
		"run_scheduler_job",
		"run_crawl",
		"stock_research_backfill",
		"stock_research_pdf_parse",
		"investor_relations_backfill",
		"a_stock_auction_latest",
		"a_stock_auction_backfill",
		"a_stock_holdings_backfill",
		"service_restart",
	}
}

func androidOverview(articles model.ItemListResult, projects []model.Project, reports []model.Report, rules []model.MonitorRule, crawlRuns []model.CrawlRun) model.Overview {
	return model.Overview{
		ArticleCount:   articles.Total,
		ProjectCount:   len(projects),
		ReportCount:    len(reports),
		CrawlRunCount:  len(crawlRuns),
		AlertRuleCount: len(rules),
	}
}

func limitReports(reports []model.Report, limit int) []model.Report {
	if limit <= 0 || len(reports) <= limit {
		return reports
	}
	return reports[:limit]
}

func actionQuery(params map[string]string) url.Values {
	query := url.Values{}
	for key, value := range params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || key == "service" || key == "name" {
			continue
		}
		query.Set(key, value)
	}
	return query
}

func withQuery(base string, query url.Values) string {
	if len(query) == 0 {
		return base
	}
	return fmt.Sprintf("%s?%s", base, query.Encode())
}
