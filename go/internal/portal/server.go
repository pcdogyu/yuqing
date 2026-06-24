package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/cryptoutil"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

const sessionCookieName = "stonedt_portal_session"
const legacySearchPageSize = 1000
const legacyMobilePopupKey = "mobile-popup"
const legacyContactPopupKeyPrefix = "contact-"

type legacyFacetBucket struct {
	Key      string `json:"key"`
	DocCount int    `json:"doc_count"`
}

type legacySearchRequest struct {
	Searchword         string           `json:"searchword"`
	SearchWord         string           `json:"searchWord"`
	Keyword            string           `json:"keyword"`
	SearchKeyword      string           `json:"searchkeyword"`
	Page               int              `json:"page"`
	PageNum            int              `json:"pageNum"`
	PageSize           int              `json:"pageSize"`
	Similar            int              `json:"similar"`
	MatchingMode       int              `json:"matchingmode"`
	SearchType         int              `json:"searchType"`
	TimeType           int              `json:"timeType"`
	Times              string           `json:"times"`
	Timee              string           `json:"timee"`
	Precise            int              `json:"precise"`
	ProjectID          string           `json:"projectid"`
	ProjectID2         string           `json:"projectId"`
	ProjectID3         string           `json:"project_id"`
	SourceType         string           `json:"source_type"`
	IndustryIndex      legacyStringList `json:"industryIndex"`
	EventIndex         legacyStringList `json:"eventIndex"`
	Province           legacyStringList `json:"province"`
	City               legacyStringList `json:"city"`
	OrganizationType   legacyStringList `json:"organizationtype"`
	CategoryLableData  legacyStringList `json:"categorylabledata"`
	EnterpriseTypeList legacyStringList `json:"enterprisetypelist"`
	HighTechTypeList   legacyStringList `json:"hightechtypelist"`
	PolicyLableFlag    legacyStringList `json:"policylableflag"`
	Classify           legacyStringList `json:"classify"`
	Read               string           `json:"read"`
	Favorite           string           `json:"favorite"`
	Start              string           `json:"start"`
	End                string           `json:"end"`
}

type legacyStringList []string

func (l *legacyStringList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*l = nil
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			*l = nil
			return nil
		}
		*l = []string{value}
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*l = values
		return nil
	}
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	values = values[:0]
	for _, item := range raw {
		switch typed := item.(type) {
		case string:
			typed = strings.TrimSpace(typed)
			if typed != "" {
				values = append(values, typed)
			}
		case float64:
			values = append(values, strconv.FormatFloat(typed, 'f', -1, 64))
		case int:
			values = append(values, strconv.Itoa(typed))
		}
	}
	*l = values
	return nil
}

type Server struct {
	cfg       config.Config
	client    *resty.Client
	templates *template.Template
	mu        sync.Mutex
	captchas  map[string]string
	mobileQRs map[string]mobileQRCodeState
}

type mobileQRCodeState struct {
	Token     string
	ExpiresAt time.Time
}

type pageData struct {
	Title                      string
	User                       any
	SectionKey                 string
	AStockRepair               aStockRepairView
	Dashboard                  model.DashboardSnapshot
	Groups                     []model.ProjectGroup
	Project                    model.Project
	Projects                   []model.Project
	Rule                       model.MonitorRule
	Rules                      []model.MonitorRule
	CrawlTemplates             []model.CrawlTemplate
	Articles                   model.ItemListResult
	Article                    model.Item
	Related                    []model.Item
	CrawlRuns                  []model.CrawlRun
	Reports                    []model.Report
	Report                     model.Report
	Notices                    []model.SystemNotice
	FeedbackItems              []model.Feedback
	AuditLogs                  []model.AuditLog
	TaskRuns                   []model.TaskRun
	SchedulerJobs              []model.OperationSchedulerJob
	LegacyRouteSummary         []legacyRouteSummary
	LegacyLiveRoutes           []legacyRouteSpec
	Operations                 model.OperationsSummary
	DatabaseConfig             model.DatabaseConfigStatus
	PublicOptions              []model.PublicOption
	PublicOption               model.PublicOption
	Preferences                model.UserPreference
	PopupState                 model.PopupState
	MailConfig                 model.MailConfig
	WarningSetting             model.WarningSetting
	SearchOptions              model.SearchOptions
	Error                      string
	Message                    string
	ServiceLogName             string
	ServiceLogText             string
	ServiceLogEntries          []serviceLogEntry
	ServiceLogSummaries        []serviceLogSummary
	ServiceLogPage             int
	ServiceLogPrev             int
	ServiceLogNext             int
	ServiceLogTotalPages       int
	Reference                  string
	ReturnTo                   string
	ReturnURL                  string
	FilterKeyword              string
	FilterProject              string
	FilterStatus               string
	FilterRead                 string
	FilterFlag                 string
	FilterSource               string
	FilterStart                string
	FilterEnd                  string
	FilterIndustry             string
	FilterProvince             string
	FilterCity                 string
	SearchMode                 string
	Section                    string
	FavoriteItems              model.ItemListResult
	FavoritePage               int
	FavoritePagePrev           int
	FavoritePageNext           int
	FavoriteProjectID          string
	FavoriteTotalPages         int
	ArticlePage                int
	ArticlePagePrev            int
	ArticlePageNext            int
	ArticleTotalPages          int
	ArticlePrevURL             string
	ArticleNextURL             string
	ArticleSort                string
	ArticleTimeLabel           string
	WarningArticles            []legacyWarningArticleCompat
	WarningArticlePage         int
	WarningArticlePrev         int
	WarningArticleNext         int
	WarningArticleTotalPages   int
	WarningArticleProjectID    string
	WarningArticleOpenFlag     int
	WarningArticleKeyword      string
	PlatformNLPBinding         model.PlatformBinding
	PlatformXieBinding         model.PlatformBinding
	PlatformImageURL           string
	PlatformOCRText            string
	PlatformImageKeywords      string
	PlatformXieArticleID       string
	PlatformXieDraftText       string
	PlatformXieGeneratedTitle  string
	PlatformXieGeneratedReport string
	Services                   []serviceStatus
	ProjectNames               map[int64]string
	GroupNames                 map[int64]string
	CountActive                int
	CountPaused                int
	CountRead                  int
	CountUnread                int
	CountFlagged               int
	CountDraft                 int
	CountGenerated             int
	CountArchived              int
	FooterCommit               string
	FooterBuildTime            string
	FooterBranch               string
}

type serviceStatus struct {
	Name    string
	URL     string
	Status  string
	Healthy bool
	Message string
}

func serviceStatusKey(service serviceStatus) string {
	if strings.TrimSpace(service.Status) == "" {
		if service.Healthy {
			return "ok"
		}
		return "failed"
	}
	normalized := apiutil.NormalizeHealthStatus(service.Status)
	if normalized == "ok" && !service.Healthy {
		return "failed"
	}
	return normalized
}

func serviceStatusLabel(service serviceStatus) string {
	switch serviceStatusKey(service) {
	case "working":
		return "工作中"
	case "ok":
		return "正常"
	default:
		return "异常"
	}
}

func serviceStatusClass(service serviceStatus) string {
	switch serviceStatusKey(service) {
	case "working":
		return "busy"
	case "ok":
		return "ok"
	default:
		return "bad"
	}
}

type serviceLogSummary struct {
	Service string
	Entries []serviceLogEntry
	Message string
}

type legacyRouteSummary struct {
	Strategy string
	Count    int
}

func NewServer(cfg config.Config) *Server {
	funcMap := template.FuncMap{
		"firstProjectGroupID":      firstProjectGroupIDForTemplate,
		"firstProjectIDForItem":    firstProjectIDForItemTemplate,
		"publicOptionAnalysisText": publicOptionAnalysisText,
		"serviceStatusClass":       serviceStatusClass,
		"serviceStatusLabel":       serviceStatusLabel,
		"subInt":                   func(a, b int) int { return a - b },
		"formatShanghaiTime":       formatShanghaiTime,
		"formatArticleCaptureTime": formatArticleCaptureTime,
		"formatArticlePublishTime": formatArticlePublishTime,
		"formatArticleListTime":    formatArticleListTime,
		"articleBodyText":          articleBodyText,
	}
	tpl := template.Must(template.New("layout").Funcs(funcMap).Parse(layoutTemplate))
	template.Must(tpl.New("login").Parse(loginTemplate))
	template.Must(tpl.New("dashboard").Parse(dashboardTemplate))
	template.Must(tpl.New("projects").Parse(projectsTemplate))
	template.Must(tpl.New("project").Parse(projectTemplate))
	template.Must(tpl.New("rules").Parse(rulesTemplate))
	template.Must(tpl.New("rule").Parse(ruleTemplate))
	template.Must(tpl.New("crawl_templates").Parse(crawlTemplatesTemplate))
	template.Must(tpl.New("articles").Parse(articlesRealtimeTemplate))
	template.Must(tpl.New("article").Parse(articleTemplate))
	template.Must(tpl.New("reports").Parse(reportsTemplate))
	template.Must(tpl.New("report").Parse(reportTemplate))
	template.Must(tpl.New("system").Parse(systemTemplate))
	template.Must(tpl.New("system_logs").Parse(systemLogsTemplate))
	template.Must(tpl.New("logs").Parse(logsTemplate))
	template.Must(tpl.New("platform_bindings").Parse(platformBindingsTemplate))
	template.Must(tpl.New("platform_bindings_v2").Parse(platformWorkbenchTemplateV3))
	template.Must(tpl.New("public_option_workbench").Parse(publicOptionWorkbenchTemplate))

	return &Server{
		cfg: cfg,
		client: resty.New().
			SetTimeout(cfg.HTTPTimeout).
			SetHeader("X-Service-Token", cfg.ServiceToken),
		templates: tpl,
		captchas:  map[string]string{},
		mobileQRs: map[string]mobileQRCodeState{},
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/healthy", s.handleHealthz)
	mux.HandleFunc("/login", s.handleLoginPage)
	mux.HandleFunc("/loginbak", s.handleLoginBakPage)
	mux.HandleFunc("/forgotpwd", s.handleForgotPasswordPage)
	mux.HandleFunc("/img/code", s.unlessRemoved(s.handleCaptchaCode))
	mux.HandleFunc("/displayboard", s.requireSessionUnlessRemoved(s.handleDisplayBoard))
	mux.HandleFunc("/displayboard/", s.requireSessionUnlessRemoved(s.handleDisplayBoard))
	mux.HandleFunc("/displayboard/collection2", s.requireSessionJSONUnlessRemoved(s.handleDisplayBoardCollection2))
	mux.HandleFunc("/analysis", s.requireSession(s.handleAnalysisEntry))
	mux.HandleFunc("/analysis/", s.requireSessionJSON(s.handleAnalysisCompatJSON))
	mux.HandleFunc("/mobile/monitor", s.requireSessionUnlessRemoved(s.handleMobileMonitor))
	mux.HandleFunc("/mobile/monitor/", s.requireSessionUnlessRemoved(s.handleMobileMonitor))
	mux.HandleFunc("/mobile/monitor/detail", s.requireSessionUnlessRemoved(s.handleMobileMonitorDetail))
	mux.HandleFunc("/mobile/warning", s.requireSessionUnlessRemoved(s.handleMobileWarning))
	mux.HandleFunc("/mobile/getGroupAndProject", s.requireSessionJSONUnlessRemoved(s.handleMobileGetGroupAndProject))
	mux.HandleFunc("/mobile/mobileQRCode", s.requireSessionUnlessRemoved(s.handleMobileQRCode))
	mux.HandleFunc("/mobile/uuid/", s.unlessRemoved(s.handleMobileUUID))
	mux.HandleFunc("/monitor", s.requireSession(s.handleMonitorEntry))
	mux.HandleFunc("/monitor/", s.requireSession(s.handleMonitorCompat))
	mux.HandleFunc("/monitor/wxGroup", s.requireSession(s.handleMonitorWxGroup))
	mux.HandleFunc("/volume", s.requireSessionUnlessRemoved(s.handleVolume))
	mux.HandleFunc("/volume/", s.requireSessionUnlessRemoved(s.handleVolume))
	mux.HandleFunc("/internal/a-stock/recommendations/generate", s.requireServiceToken(s.handleAStockRecommendationGenerate))
	mux.HandleFunc("/a-stock/popup", s.requireSessionJSON(s.handleAStockPopup))
	mux.HandleFunc("/a-stock/popup/dismiss", s.requireSessionJSON(s.handleAStockPopupDismiss))
	mux.HandleFunc("/a-stock/auction", s.requireSession(s.handleAStockAuctionPage))
	mux.HandleFunc("/a-stock/holdings", s.requireSession(s.handleAStockHoldingsPage))
	mux.HandleFunc("/a-stock", s.requireSession(s.handleAStockPage))
	mux.HandleFunc("/a-stock/", s.requireSession(s.handleAStockPage))
	mux.HandleFunc("/stock-research", s.requireSession(s.handleStockResearchPage))
	mux.HandleFunc("/stock-research/", s.requireSession(s.handleStockResearchAsset))
	mux.HandleFunc("/api/v1/stock-research/", s.requireSession(s.handleStockResearchAsset))
	mux.HandleFunc("/investor-relations", s.requireSession(s.handleInvestorRelationsPage))
	mux.HandleFunc("/crypto", s.requireSession(s.handleCryptoPage))
	mux.HandleFunc("/crypto/", s.requireSession(s.handleCryptoPage))
	mux.HandleFunc("/volume/getproject", s.requireSessionJSONUnlessRemoved(s.handleVolumeGetProject))
	mux.HandleFunc("/volume/projectname", s.requireSessionJSONUnlessRemoved(s.handleVolumeProjectName))
	mux.HandleFunc("/hot/hotpage", s.requireSessionUnlessRemoved(s.handleHotPage))
	mux.HandleFunc("/hot/hotpage/", s.requireSessionUnlessRemoved(s.handleHotPage))
	mux.HandleFunc("/hot/hotlist", s.requireSessionUnlessRemoved(s.handleHotList))
	mux.HandleFunc("/dist/monitor", s.unlessRemoved(s.handleDistMonitor))
	mux.HandleFunc("/dist/getdata", s.unlessRemoved(s.handleDistGetData))
	mux.HandleFunc("/dist/apply", s.unlessRemoved(s.handleDistApply))
	mux.HandleFunc("/dist/yqapply", s.unlessRemoved(s.handleDistYqApply))
	mux.HandleFunc("/dist/applydatainfo", s.unlessRemoved(s.handleDistApplyDataInfo))
	mux.HandleFunc("/dist/yqmontitor", s.unlessRemoved(s.handleDistYqMonitor))
	mux.HandleFunc("/dist/hotdata", s.unlessRemoved(s.handleDistHotData))
	mux.HandleFunc("/platform/", s.requireSessionUnlessRemoved(s.handlePlatformCompat))
	mux.HandleFunc("/fullsearch", s.requireSessionUnlessRemoved(s.handleFullSearchEntry))
	mux.HandleFunc("/fullsearch/", s.requireSessionUnlessRemoved(s.handleFullSearchCompat))
	mux.HandleFunc("/timelysearch", s.requireSessionUnlessRemoved(s.handleTimelySearchEntry))
	mux.HandleFunc("/timelysearch/", s.requireSessionUnlessRemoved(s.handleTimelySearchCompat))
	mux.HandleFunc("/publicoption", s.requireSession(s.handlePublicOptionEntry))
	mux.HandleFunc("/publicoption/", s.requireSession(s.handlePublicOptionCompat))
	mux.HandleFunc("/logs", s.requireSession(s.handleLogsPage))
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/system/productmanual/online", s.requireSession(s.handleSystemProductManualOnline))
	mux.HandleFunc("/system/uploadProductManual", s.requireSession(s.handleSystemUploadProductManual))
	mux.HandleFunc("/system/preference", s.requireSession(s.handleSystemSectionRedirect("preferences")))
	mux.HandleFunc("/system/favorite", s.requireSession(s.handleSystemSectionRedirect("favorites")))
	mux.HandleFunc("/system/warning", s.requireSession(s.handleSystemWarningEdit))
	mux.HandleFunc("/system/warningmsg", s.requireSession(s.handleSystemWarningMessage))
	mux.HandleFunc("/system/feedback", s.requireSession(s.handleSystemSectionRedirect("feedback")))
	mux.HandleFunc("/system/warningedit", s.requireSession(s.handleSystemWarningEdit))
	mux.HandleFunc("/wechat/getQrCode", s.handleWechatGetQrCode)
	mux.HandleFunc("/wechat/getBindQrCode", s.handleWechatGetBindQRCode)
	mux.HandleFunc("/wechat/checkBind", s.handleWechatCheckBind)
	mux.HandleFunc("/wechat/wasBind", s.handleWechatWasBind)
	mux.HandleFunc("/wechat/checkLogin", s.handleWechatCheckLogin)
	mux.HandleFunc("/wechat/token", s.handleWechatToken)
	mux.HandleFunc("/wechat/handleSubscribe", s.handleWechatHandleSubscribe)
	mux.HandleFunc("/wechat/handleUnsubscribe", s.handleWechatHandleUnsubscribe)
	mux.HandleFunc("/wechat/handleAuthorize", s.handleWechatHandleAuthorize)
	mux.HandleFunc("/projects/", s.requireSession(s.handleProjectDetail))
	mux.HandleFunc("/projects", s.requireSession(s.handleProjects))
	mux.HandleFunc("/crawl-templates/manage/", s.requireSession(s.handleCrawlTemplatesPage))
	mux.HandleFunc("/crawl-templates/manage", s.requireSession(s.handleCrawlTemplatesPage))
	mux.HandleFunc("/crawl-templates/", s.requireSession(s.handleCrawlTemplates))
	mux.HandleFunc("/crawl-templates", s.requireSession(s.handleCrawlTemplates))
	mux.HandleFunc("/monitor-rules/", s.requireSession(s.handleRuleDetail))
	mux.HandleFunc("/monitor-rules", s.requireSession(s.handleRules))
	mux.HandleFunc("/articles/", s.requireSession(s.handleArticleDetail))
	mux.HandleFunc("/articles", s.requireSession(s.handleArticles))
	mux.HandleFunc("/reports/", s.requireSession(s.handleReportDetail))
	mux.HandleFunc("/reports", s.requireSession(s.handleReports))
	mux.HandleFunc("/system/logs", s.requireSession(s.handleSystemLogs))
	mux.HandleFunc("/system", s.requireSession(s.handleSystem))
	mux.HandleFunc("/", s.handleRoot)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteHealth(w, "gateway-web", "ok", "ok")
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if status, ok := removedLegacyPortalStatus(r.URL.Path); ok {
		writeRemovedLegacyPortalResponse(w, r, status)
		return
	}
	s.requireSession(s.handleDashboard)(w, r)
}

func writeRemovedLegacyPortalResponse(w http.ResponseWriter, r *http.Request, status int) {
	if status == http.StatusNotFound {
		http.NotFound(w, r)
		return
	}
	http.Error(w, http.StatusText(status), status)
}

func (s *Server) requireSessionUnlessRemoved(next func(http.ResponseWriter, *http.Request, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if status, ok := removedLegacyPortalStatus(r.URL.Path); ok {
			writeRemovedLegacyPortalResponse(w, r, status)
			return
		}
		s.requireSession(next)(w, r)
	}
}

func (s *Server) requireSessionJSONUnlessRemoved(next func(http.ResponseWriter, *http.Request, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if status, ok := removedLegacyPortalStatus(r.URL.Path); ok {
			writeRemovedLegacyPortalResponse(w, r, status)
			return
		}
		s.requireSessionJSON(next)(w, r)
	}
}

func (s *Server) unlessRemoved(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if status, ok := removedLegacyPortalStatus(r.URL.Path); ok {
			writeRemovedLegacyPortalResponse(w, r, status)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireServiceToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(s.cfg.ServiceToken)
		if expected == "" || r.Header.Get("X-Service-Token") != expected {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = s.render(w, "login", pageData{Title: "登录", Reference: localRedirectTarget(r.URL.Query().Get("reference"))})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			_ = s.render(w, "login", pageData{Title: "登录", Error: "表单解析失败"})
			return
		}
		username := nonEmpty(strings.TrimSpace(r.FormValue("username")), strings.TrimSpace(r.FormValue("telephone")))
		reference := localRedirectTarget(r.FormValue("reference"))
		loginResp, loginErr := s.authLogin(username, r.FormValue("password"))
		if loginErr != nil {
			_ = s.render(w, "login", pageData{Title: "登录", Error: loginErr.Error(), Reference: reference})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    loginResp.SessionToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, nonEmpty(reference, "/"), http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLoginBakPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = s.render(w, "login", pageData{Title: "登录", Reference: localRedirectTarget(r.URL.Query().Get("reference"))})
}

func (s *Server) handleForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body := `<section><h1>忘记密码</h1><p>Go 兼容版暂未开放在线重置密码，请联系管理员处理。</p><p><a class="inline" href="/login">返回登录</a></p></section>`
	_ = s.writeSimplePage(w, "forgotpwd", "忘记密码", body)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_, _ = s.client.R().
			SetQueryParam("session_token", cookie.Value).
			Post(s.cfg.AuthURL + "/api/v1/auth/logout")
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) requireSessionJSON(next func(http.ResponseWriter, *http.Request, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeLegacyJSON(w, http.StatusForbidden, "未登录", map[string]any{})
			return
		}
		user, err := s.getSessionUser(cookie.Value)
		if err != nil {
			writeLegacyJSON(w, http.StatusForbidden, "会话无效", map[string]any{})
			return
		}
		next(w, r, user)
	}
}

func (s *Server) requireSessionBool(next func(http.ResponseWriter, *http.Request, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		user, err := s.getSessionUser(cookie.Value)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r, user)
	}
}

func (s *Server) handleLegacySearchRedirect(mode string) func(http.ResponseWriter, *http.Request, any) {
	return func(w http.ResponseWriter, r *http.Request, _ any) {
		http.Redirect(w, r, s.legacySearchTarget(mode, r), http.StatusSeeOther)
	}
}

func (s *Server) legacySearchTarget(mode string, r *http.Request) string {
	if target, ok := s.legacyCryptoSearchTarget(r); ok {
		return target
	}

	values := url.Values{}
	values.Set("mode", mode)
	if keyword := nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("searchword"), r.URL.Query().Get("searchWord")); keyword != "" {
		values.Set("keyword", keyword)
	}
	for _, key := range []string{"project_id", "source_type", "read", "favorite", "start", "end", "industry", "province", "city", "fulltype", "full_poly", "menuStyle", "page", "pageSize", "onlyid", "sourcename", "stype", "website_id", "pageNoData"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			values.Set(key, value)
		}
	}
	return "/articles?" + values.Encode()
}

func (s *Server) legacyCryptoSearchTarget(r *http.Request) (string, bool) {
	keyword := nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("searchword"), r.URL.Query().Get("searchWord"))
	if keyword == "" {
		return "", false
	}

	resolution, err := cryptoutil.ResolvePair(keyword)
	if err != nil {
		return "", false
	}
	return "/crypto?pair=" + url.QueryEscape(resolution.Pair), true
}

func (s *Server) handleLegacySearchBuckets(kind string) func(http.ResponseWriter, *http.Request, any) {
	return func(w http.ResponseWriter, r *http.Request, _ any) {
		req, err := decodeLegacySearchRequest(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusBadRequest, "invalid body", map[string]any{})
			return
		}

		items, err := s.fetchLegacySearchItems(r, req)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), map[string]any{})
			return
		}

		buckets := bucketLegacySearchItems(items, kind)
		buckets = append(buckets, legacyFacetBucket{Key: "total", DocCount: len(items)})

		writeLegacyJSON(w, http.StatusOK, legacySearchSuccessMessage(kind), map[string]any{"data": buckets})
	}
}

func (s *Server) handleUserCompat(w http.ResponseWriter, r *http.Request, user any) {
	target := url.Values{}
	target.Set("section", "account")
	http.Redirect(w, r, "/system?"+target.Encode(), http.StatusSeeOther)
}

func (s *Server) handleSystemSectionRedirect(section string) func(http.ResponseWriter, *http.Request, any) {
	return func(w http.ResponseWriter, r *http.Request, _ any) {
		target := url.Values{}
		target.Set("section", normalizeSystemSection(section))
		if projectID := nonEmpty(r.URL.Query().Get("project_id"), r.URL.Query().Get("projectid")); projectID != "" {
			target.Set("project_id", projectID)
		}
		if page := nonEmpty(r.URL.Query().Get("page"), r.URL.Query().Get("pageNum")); page != "" {
			target.Set("page", page)
		}
		http.Redirect(w, r, "/system?"+target.Encode(), http.StatusSeeOther)
	}
}

func (s *Server) handleSystemWarningEdit(w http.ResponseWriter, r *http.Request, _ any) {
	target := url.Values{}
	target.Set("section", "warning")
	if projectID := nonEmpty(r.URL.Query().Get("project_id"), r.URL.Query().Get("projectid")); projectID != "" {
		target.Set("project_id", projectID)
	}
	if page := nonEmpty(r.URL.Query().Get("page"), r.URL.Query().Get("pageNum")); page != "" {
		target.Set("page", page)
	}
	http.Redirect(w, r, "/system?"+target.Encode(), http.StatusSeeOther)
}

func (s *Server) handleSystemWarningMessage(w http.ResponseWriter, r *http.Request, _ any) {
	target := url.Values{}
	target.Set("section", "warningmsg")
	if projectID := nonEmpty(r.URL.Query().Get("project_id"), r.URL.Query().Get("projectid")); projectID != "" {
		target.Set("project_id", projectID)
	}
	if page := nonEmpty(r.URL.Query().Get("page"), r.URL.Query().Get("pageNum")); page != "" {
		target.Set("page", page)
	}
	if openFlag := nonEmpty(r.URL.Query().Get("openFlag"), r.URL.Query().Get("open_flag")); openFlag != "" {
		target.Set("openFlag", openFlag)
	}
	if keyword := strings.TrimSpace(r.URL.Query().Get("keyword")); keyword != "" {
		target.Set("keyword", keyword)
	}
	http.Redirect(w, r, "/system?"+target.Encode(), http.StatusSeeOther)
}

func (s *Server) handleLegacyListSolutionGroupByUserID(w http.ResponseWriter, r *http.Request, _ any) {
	groups := s.fetchLegacyProjectGroups()
	writeRawJSON(w, http.StatusOK, groups)
}

func (s *Server) handleLegacyListProjectByGroupID(w http.ResponseWriter, r *http.Request, _ any) {
	if err := r.ParseForm(); err != nil {
		writeRawJSON(w, http.StatusOK, []map[string]any{})
		return
	}
	groupID := parseFormProjectGroupID(r)
	projects := s.fetchLegacyProjects()
	if groupID > 0 {
		projects = filterLegacyProjectsByGroupID(projects, groupID)
	}
	writeRawJSON(w, http.StatusOK, projects)
}

func (s *Server) handleLegacyListProjectByUserID(w http.ResponseWriter, r *http.Request, _ any) {
	_ = r.ParseForm()
	writeRawJSON(w, http.StatusOK, s.fetchLegacyProjects())
}

func (s *Server) handleLegacyListWarning(w http.ResponseWriter, r *http.Request, _ any) {
	if err := r.ParseForm(); err != nil {
		writeLegacyJSON(w, http.StatusBadRequest, "invalid body", map[string]any{})
		return
	}
	pageNum := parsePositiveInt(nonEmpty(r.FormValue("page"), r.FormValue("pageNum")), 1)
	groupID := parseFormProjectGroupID(r)
	projects := s.fetchLegacyProjects()
	if groupID > 0 {
		projects = filterLegacyProjectsByGroupID(projects, groupID)
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].GroupID == projects[j].GroupID {
			return projects[i].ProjectID < projects[j].ProjectID
		}
		return projects[i].GroupID < projects[j].GroupID
	})
	entries := make([]map[string]any, 0, len(projects))
	for _, project := range projects {
		setting, _ := s.fetchLegacyWarningSetting(project.ProjectID)
		entries = append(entries, legacyWarningListEntry(project, setting))
	}
	pageSize := 10
	total := len(entries)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (maxInt(pageNum, 1) - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	payload := map[string]any{
		"list":      entries[start:end],
		"pageCount": totalPages,
		"dataCount": total,
	}
	writeLegacyJSON(w, http.StatusOK, "", legacyJSONString(payload))
}

func (s *Server) handleLegacyUpdateWarningStatusByID(w http.ResponseWriter, r *http.Request, _ any) {
	if err := r.ParseForm(); err != nil {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	projectID := parseFormProjectID(r)
	if projectID <= 0 {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "project_id required", nil)
		return
	}
	warningStatus := parsePositiveInt(nonEmpty(r.FormValue("warning_status"), r.FormValue("warningStatus")), 0)
	setting, _ := s.fetchLegacyWarningSetting(projectID)
	if warningStatus == 1 && strings.TrimSpace(setting.WarningWord) == "" {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "预警词为空不能打开预警开关！", nil)
		return
	}
	setting.WarningStatus = warningStatus
	setting.Enabled = warningStatus == 1
	resp, err := s.client.R().
		SetBody(setting).
		Put(s.cfg.ContentURL + "/api/v1/system/warning-settings/" + strconv.FormatInt(projectID, 10))
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if !resp.IsSuccess() {
		writeLegacyStatusJSON(w, resp.StatusCode(), resp.Status(), nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
}

func (s *Server) handleLegacyGetFavoriteList(w http.ResponseWriter, r *http.Request, user any) {
	if err := r.ParseForm(); err != nil {
		writeLegacyJSON(w, http.StatusBadRequest, "invalid body", map[string]any{})
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyJSON(w, http.StatusForbidden, "未登录", map[string]any{})
		return
	}
	pageNum := parsePositiveInt(nonEmpty(r.FormValue("pageNum"), r.FormValue("page")), 1)
	projectID := strings.TrimSpace(nonEmpty(r.FormValue("project_id"), r.FormValue("projectId"), r.FormValue("projectid")))
	var result model.ItemListResult
	favoriteURL := s.cfg.ContentURL + "/api/v1/articles?favorite=favorited&page=" + strconv.Itoa(pageNum) + "&page_size=10&user_id=" + strconv.FormatInt(userID, 10)
	if projectID != "" {
		favoriteURL += "&project_id=" + url.QueryEscape(projectID)
	}
	if err := s.getJSON(favoriteURL, &result); err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), map[string]any{})
		return
	}
	items := make([]map[string]any, 0, len(result.Items))
	projectMap := s.fetchLegacyProjectMap()
	for _, item := range result.Items {
		projectIDValue, groupIDValue := firstLegacyProjectAndGroup(item, projectMap)
		items = append(items, map[string]any{
			"article_public_id": legacyArticlePublicID(item),
			"groupid":           groupIDValue,
			"projectid":         projectIDValue,
			"title":             item.Title,
			"source_name":       nonEmpty(item.FromText, item.SourceType),
			"emotionalIndex":    legacyEmotionalIndex(item),
			"publish_time":      legacyPublishTime(item),
		})
	}
	totalPages := 1
	if result.PageSize > 0 && result.Total > 0 {
		totalPages = (result.Total + result.PageSize - 1) / result.PageSize
	}
	writeLegacyJSON(w, http.StatusOK, "OK", map[string]any{
		"favoriteList": items,
		"pageInfo": map[string]any{
			"pageNum":  pageNum,
			"pages":    totalPages,
			"total":    result.Total,
			"pageSize": result.PageSize,
		},
	})
}

func (s *Server) handleLegacyWarningSettingDetail(w http.ResponseWriter, r *http.Request, _ any) {
	projectID := parseFormProjectID(r)
	if projectID <= 0 {
		projectID = parseProjectID(r.URL.Query().Get("projectId"))
	}
	if projectID <= 0 {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "projectId required", nil)
		return
	}
	setting, projectName := s.fetchLegacyWarningSetting(projectID)
	writeLegacyStatusJSON(w, http.StatusOK, "OK", legacyWarningSettingPayload(setting, projectID, projectName))
}

func (s *Server) handleLegacyGetWarningArticle(w http.ResponseWriter, r *http.Request, user any) {
	if err := r.ParseForm(); err != nil {
		writeLegacyJSON(w, http.StatusBadRequest, "invalid body", map[string]any{})
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyJSON(w, http.StatusForbidden, "未登录", map[string]any{})
		return
	}
	pageNum := parsePositiveInt(nonEmpty(r.FormValue("pageNum"), r.FormValue("page")), 1)
	openFlag, _ := strconv.Atoi(strings.TrimSpace(nonEmpty(r.FormValue("openFlag"), r.FormValue("open_flag"))))
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	projectID := parseFormProjectID(r)
	payload, err := s.buildLegacyWarningArticlePayload(userID, projectID, openFlag, keyword, pageNum)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), map[string]any{})
		return
	}
	writeLegacyJSON(w, http.StatusOK, "", payload)
}

func (s *Server) handleLegacyGetOpinionConditionByProjectID(w http.ResponseWriter, r *http.Request, _ any) {
	projectID := parseProjectID(nonEmpty(r.FormValue("projectId"), r.FormValue("project_id"), r.URL.Query().Get("projectId"), r.URL.Query().Get("project_id")))
	if projectID <= 0 {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{})
		return
	}
	var condition model.OpinionCondition
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/system/opinion-conditions/"+strconv.FormatInt(projectID, 10), &condition); err != nil {
		condition = model.OpinionCondition{ProjectID: projectID, Time: 4, Emotion: "[1,2,3]", Sort: 1, Matchs: 1}
	}
	writeRawJSON(w, http.StatusOK, condition)
}

func (s *Server) handleLegacyUpdateOpinionCondition(w http.ResponseWriter, r *http.Request, _ any) {
	condition, err := decodeLegacyOpinionCondition(r)
	if err != nil {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"status": false, "message": "偏好设置修改失败"})
		return
	}
	if condition.ProjectID <= 0 {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"status": false, "message": "project_id required"})
		return
	}
	resp, err := s.client.R().
		SetBody(condition).
		Put(s.cfg.ContentURL + "/api/v1/system/opinion-conditions/" + strconv.FormatInt(condition.ProjectID, 10))
	if err != nil {
		writeRawJSON(w, http.StatusInternalServerError, map[string]any{"status": false, "message": err.Error()})
		return
	}
	if !resp.IsSuccess() {
		writeRawJSON(w, resp.StatusCode(), map[string]any{"status": false, "message": resp.Status()})
		return
	}
	writeRawJSON(w, http.StatusOK, map[string]any{"status": true, "message": "偏好设置修改成功"})
}

type legacyWarningArticlePage struct {
	Articles   []legacyWarningArticleCompat
	PageNum    int
	Total      int
	TotalPages int
}

func (s *Server) collectLegacyWarningArticles(userID int64, projectID int64, openFlag int, keyword string, pageNum int) (legacyWarningArticlePage, error) {
	var result model.ItemListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=1000&user_id="+strconv.FormatInt(userID, 10)+"&read=unread", &result); err != nil {
		return legacyWarningArticlePage{}, err
	}
	projectMap := s.fetchLegacyProjectMap()
	groupNames := s.fetchLegacyGroupNameMap()
	articles := make([]legacyWarningArticleCompat, 0, len(result.Items))
	for _, item := range result.Items {
		if keyword != "" && !strings.Contains(strings.ToLower(item.Title), strings.ToLower(keyword)) {
			continue
		}
		if len(item.ProjectIDs) == 0 {
			continue
		}
		for _, itemProjectID := range item.ProjectIDs {
			if projectID > 0 && itemProjectID != projectID {
				continue
			}
			project, ok := projectMap[itemProjectID]
			if !ok {
				continue
			}
			setting, _ := s.fetchLegacyWarningSetting(itemProjectID)
			if openFlag == 1 && !setting.Enabled {
				continue
			}
			articles = append(articles, legacyWarningArticleCompat{
				ArticleID:     legacyArticlePublicID(item),
				ArticleTitle:  item.Title,
				ArticleTime:   legacyPublishTime(item),
				ArticleDetail: legacyJSONString(map[string]any{"sourcewebsitename": nonEmpty(item.FromText, item.ExternalSourceHost, item.SourceType)}),
				GroupID:       strconv.FormatInt(project.GroupID, 10),
				ProjectID:     strconv.FormatInt(project.ID, 10),
				GroupName:     groupNames[project.GroupID],
				ProjectName:   project.Name,
			})
		}
	}
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].ArticleTime > articles[j].ArticleTime
	})
	pageSize := 10
	total := len(articles)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start := (maxInt(pageNum, 1) - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return legacyWarningArticlePage{
		Articles:   articles[start:end],
		PageNum:    pageNum,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *Server) buildLegacyWarningArticlePayload(userID int64, projectID int64, openFlag int, keyword string, pageNum int) (map[string]any, error) {
	data, err := s.collectLegacyWarningArticles(userID, projectID, openFlag, keyword, pageNum)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"warningArticle": data.Articles,
		"pageInfo": map[string]any{
			"pageNum":  data.PageNum,
			"pages":    data.TotalPages,
			"total":    data.Total,
			"pageSize": 10,
		},
	}, nil
}

func (s *Server) handleLegacyGetWarningWords(w http.ResponseWriter, r *http.Request, _ any) {
	projectID := parseFormProjectID(r)
	if projectID <= 0 {
		projectID = parseProjectID(r.URL.Query().Get("projectId"))
	}
	if projectID <= 0 {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "project_id required", nil)
		return
	}
	setting, _ := s.fetchLegacyWarningSetting(projectID)
	if strings.TrimSpace(setting.WarningWord) == "" {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, "预警词为空不能打开预警开关！", nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
}

func (s *Server) handleLegacyUpdateWarning(w http.ResponseWriter, r *http.Request, _ any) {
	if err := r.ParseForm(); err != nil {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	projectID := parseFormProjectID(r)
	if projectID <= 0 {
		projectID = parseProjectID(r.FormValue("projectid"))
	}
	if projectID <= 0 {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "project_id required", nil)
		return
	}
	setting := legacyWarningSettingFromForm(r)
	resp, err := s.client.R().
		SetBody(setting).
		Put(s.cfg.ContentURL + "/api/v1/system/warning-settings/" + strconv.FormatInt(projectID, 10))
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if !resp.IsSuccess() {
		writeLegacyStatusJSON(w, resp.StatusCode(), resp.Status(), nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
}

func decodeLegacySearchRequest(r *http.Request) (legacySearchRequest, error) {
	var req legacySearchRequest
	if r.Body == nil {
		return req, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
		}
		return legacySearchRequest{}, err
	}
	return req, nil
}

type legacyMailConfigRequest struct {
	Host       string
	Username   string
	Password   string
	Port       int
	To         string
	SenderName string
}

type legacyMailConfigResponse struct {
	Host     string   `json:"host"`
	Port     string   `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	To       string   `json:"to"`
	Cc       []string `json:"cc"`
	ToList   []string `json:"toList"`
}

type legacyReportListEnvelope struct {
	List      []map[string]any `json:"list"`
	PageCount int              `json:"pageCount"`
	DataCount int              `json:"dataCount"`
}

func (s *Server) handleLegacyReportPage(w http.ResponseWriter, r *http.Request, user any) {
	reportsReq := cloneRequestWithURL(r, legacyReportListPath(r))
	s.handleReports(w, reportsReq, user)
}

func (s *Server) handleLegacyReportCompat(w http.ResponseWriter, r *http.Request, user any) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/report/"), "/")
	if path == "" {
		s.handleLegacyReportPage(w, r, user)
		return
	}
	if _, err := strconv.ParseInt(path, 10, 64); err == nil {
		reportReq := cloneRequestWithURL(r, legacyReportDetailPath(r, path))
		s.handleReportDetail(w, reportReq, user)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleLegacyReportCustomList(w http.ResponseWriter, r *http.Request, _ any) {
	if err := r.ParseForm(); err != nil {
		writeRawJSON(w, http.StatusOK, legacyReportListEnvelope{})
		return
	}
	pageNum := parsePositiveInt(nonEmpty(r.FormValue("pageNum"), r.FormValue("page")), 1)
	projectID := strings.TrimSpace(nonEmpty(r.FormValue("projectId"), r.FormValue("project_id")))
	keyword := strings.TrimSpace(r.FormValue("nameSearch"))
	status := legacyReportTypeToStatus(r.FormValue("reportType"))

	reports, err := s.fetchFilteredReports(projectID, keyword, status)
	if err != nil {
		writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), legacyJSONString(legacyReportListEnvelope{}))
		return
	}
	pageSize := 10
	total := len(reports)
	pageCount := 1
	if total > 0 {
		pageCount = (total + pageSize - 1) / pageSize
	}
	start := (maxInt(pageNum, 1) - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	projectNames := map[int64]string{}
	for _, project := range s.fetchLegacyProjects() {
		projectNames[project.ProjectID] = project.ProjectName
	}
	items := make([]map[string]any, 0, end-start)
	for _, report := range reports[start:end] {
		items = append(items, map[string]any{
			"id":           report.ID,
			"report_id":    report.ID,
			"reportId":     report.ID,
			"project_id":   report.ProjectID,
			"projectId":    report.ProjectID,
			"project_name": projectNames[report.ProjectID],
			"title":        report.Title,
			"summary":      report.Summary,
			"status":       report.Status,
			"report_type":  legacyReportStatusToType(report.Status),
			"create_time":  report.CreatedAt.Format("2006-01-02 15:04:05"),
			"update_time":  report.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	payload := legacyReportListEnvelope{
		List:      items,
		PageCount: pageCount,
		DataCount: total,
	}
	writeLegacyJSON(w, http.StatusOK, "", legacyJSONString(payload))
}

func (s *Server) handleLegacyReportCustomDetailJSON(w http.ResponseWriter, r *http.Request, _ any) {
	if err := r.ParseForm(); err != nil {
		writeRawJSON(w, http.StatusOK, map[string]any{})
		return
	}
	reportID := parsePositiveInt(nonEmpty(r.FormValue("reportId"), r.FormValue("report_id")), 0)
	if reportID <= 0 {
		writeRawJSON(w, http.StatusOK, map[string]any{})
		return
	}
	var report model.Report
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/reports/"+strconv.Itoa(reportID), &report); err != nil {
		writeRawJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeRawJSON(w, http.StatusOK, map[string]any{
		"id":         report.ID,
		"report_id":  report.ID,
		"reportId":   report.ID,
		"project_id": report.ProjectID,
		"title":      report.Title,
		"summary":    report.Summary,
		"content":    report.Content,
		"status":     report.Status,
		"reportType": legacyReportStatusToType(report.Status),
		"createdAt":  report.CreatedAt.Format("2006-01-02 15:04:05"),
		"updatedAt":  report.UpdatedAt.Format("2006-01-02 15:04:05"),
	})
}

func (s *Server) handleLegacyBatchUpdateReportCustom(w http.ResponseWriter, r *http.Request, _ any) {
	ids, err := decodeLegacyReportIDs(r)
	if err != nil || len(ids) == 0 {
		writeRawJSON(w, http.StatusOK, map[string]any{"status": false, "message": "reportIds required"})
		return
	}
	resp, err := s.client.R().
		SetBody(map[string]any{"report_ids": ids}).
		Post(s.cfg.ContentURL + "/api/v1/reports/batch-delete")
	if err != nil {
		writeRawJSON(w, http.StatusOK, map[string]any{"status": false, "message": err.Error()})
		return
	}
	if !resp.IsSuccess() {
		writeRawJSON(w, http.StatusOK, map[string]any{"status": false, "message": resp.Status()})
		return
	}
	writeRawJSON(w, http.StatusOK, map[string]any{"status": true, "message": "操作成功"})
}

func (s *Server) handleLegacyBatchUpdateReportCustomStatus(w http.ResponseWriter, r *http.Request, _ any) {
	ids, err := decodeLegacyReportIDs(r)
	if err != nil || len(ids) == 0 {
		writeRawJSON(w, http.StatusOK, map[string]any{"status": false, "message": "reportIds required"})
		return
	}
	status := legacyReportTypeToStatus(nonEmpty(r.FormValue("status"), r.FormValue("reportType")))
	if status == "" {
		status = "generated"
	}
	resp, err := s.client.R().
		SetBody(map[string]any{"report_ids": ids, "status": status}).
		Post(s.cfg.ContentURL + "/api/v1/reports/batch-status")
	if err != nil {
		writeRawJSON(w, http.StatusOK, map[string]any{"status": false, "message": err.Error()})
		return
	}
	if !resp.IsSuccess() {
		writeRawJSON(w, http.StatusOK, map[string]any{"status": false, "message": resp.Status()})
		return
	}
	writeRawJSON(w, http.StatusOK, map[string]any{"status": true, "message": "操作成功"})
}

func (s *Server) fetchFilteredReports(projectID, keyword, status string) ([]model.Report, error) {
	reportURL := s.cfg.ContentURL + "/api/v1/reports"
	if strings.TrimSpace(projectID) != "" {
		reportURL += "?project_id=" + url.QueryEscape(projectID)
	}
	reports := []model.Report{}
	if err := s.getJSON(reportURL, &reports); err != nil {
		return nil, err
	}
	filtered := make([]model.Report, 0, len(reports))
	for _, report := range reports {
		if keyword != "" && !strings.Contains(strings.ToLower(report.Title), strings.ToLower(keyword)) {
			continue
		}
		if status != "" && report.Status != status {
			continue
		}
		filtered = append(filtered, report)
	}
	return filtered, nil
}

func legacyReportTypeToStatus(raw string) string {
	switch strings.TrimSpace(raw) {
	case "1":
		return "generated"
	case "2":
		return "draft"
	case "3":
		return "archived"
	default:
		return ""
	}
}

func legacyReportStatusToType(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "generated":
		return 1
	case "draft":
		return 2
	case "archived":
		return 3
	default:
		return 0
	}
}

func decodeLegacyReportIDs(r *http.Request) ([]int64, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(nonEmpty(r.FormValue("reportIds"), r.FormValue("report_ids")))
	if raw == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(ch rune) bool {
		return ch == ',' || ch == '，'
	})
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || value <= 0 {
			return nil, errors.New("invalid report id")
		}
		ids = append(ids, value)
	}
	return ids, nil
}

func cloneRequestWithPath(r *http.Request, path string) *http.Request {
	return cloneRequestWithURL(r, path)
}

func cloneRequestWithURL(r *http.Request, rawURL string) *http.Request {
	cloned := r.Clone(r.Context())
	nextURL, err := url.Parse(rawURL)
	if err == nil {
		cloned.URL = nextURL
		cloned.RequestURI = rawURL
		return cloned
	}
	if cloned.URL != nil {
		fallback := *cloned.URL
		fallback.Path = rawURL
		fallback.RawQuery = ""
		cloned.URL = &fallback
	}
	cloned.RequestURI = rawURL
	return cloned
}

func legacyReportListPath(r *http.Request) string {
	values := url.Values{}
	projectID := strings.TrimSpace(nonEmpty(r.URL.Query().Get("project_id"), r.URL.Query().Get("projectid")))
	if projectID == "" {
		projectID = strings.TrimSpace(r.URL.Query().Get("projectId"))
	}
	if projectID != "" {
		values.Set("project_id", projectID)
	}
	keyword := strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("search")))
	if keyword != "" {
		values.Set("keyword", keyword)
	}
	status := legacyReportTypeToStatus(r.URL.Query().Get("type"))
	if status != "" {
		values.Set("status", status)
	}
	page := strings.TrimSpace(r.URL.Query().Get("page"))
	if page != "" {
		values.Set("page", page)
	}
	if len(values) == 0 {
		return "/reports"
	}
	return "/reports?" + values.Encode()
}

func legacyReportDetailPath(r *http.Request, id string) string {
	returnURL := legacyReportListPath(r)
	return "/reports/" + id + "?return_to=" + url.QueryEscape(returnURL)
}

func decodeLegacyMailConfigRequest(r *http.Request) (legacyMailConfigRequest, error) {
	var raw map[string]any
	if r.Body == nil {
		return legacyMailConfigRequest{}, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return legacyMailConfigRequest{}, nil
		}
		return legacyMailConfigRequest{}, err
	}
	req := legacyMailConfigRequest{
		Host:     legacyStringFromAny(raw["host"]),
		Username: legacyStringFromAny(raw["username"]),
		Password: legacyStringFromAny(raw["password"]),
		To:       legacyStringFromAny(raw["to"]),
	}
	if port, ok := legacyIntFromAny(raw["port"]); ok {
		req.Port = port
	}
	req.SenderName = legacyStringFromAny(raw["sender_name"])
	return req, nil
}

func (s *Server) fetchLegacySearchItems(r *http.Request, req legacySearchRequest) ([]model.Item, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("page_size", strconv.Itoa(legacySearchPageSize))
	if keyword := legacySearchKeyword(req); keyword != "" {
		query.Set("q", keyword)
	}
	if projectID := legacySearchProjectID(req); projectID != "" {
		query.Set("project_id", projectID)
	}
	if sourceType := strings.TrimSpace(req.SourceType); sourceType != "" {
		query.Set("source_type", sourceType)
	}
	if start, end := legacySearchTimeRange(req); start != "" {
		query.Set("start", start)
		if end != "" {
			query.Set("end", end)
		}
	}

	items := make([]model.Item, 0, legacySearchPageSize)
	total := 0
	for page := 1; page <= 20; page++ {
		query.Set("page", strconv.Itoa(page))
		var result model.SearchResult
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
			return nil, err
		}
		if page == 1 {
			total = result.Total
		}
		items = append(items, result.Items...)
		if len(result.Items) == 0 || len(items) >= total {
			break
		}
	}
	return items, nil
}

func legacySearchKeyword(req legacySearchRequest) string {
	keyword := nonEmpty(req.Keyword, req.Searchword, req.SearchWord)
	keyword = strings.TrimSpace(keyword)
	keyword = strings.ReplaceAll(keyword, "+", " AND ")
	keyword = strings.ReplaceAll(keyword, " ", " OR ")
	return keyword
}

func legacySearchProjectID(req legacySearchRequest) string {
	return nonEmpty(req.ProjectID3, req.ProjectID2, req.ProjectID)
}

func legacySearchTimeRange(req legacySearchRequest) (string, string) {
	if strings.TrimSpace(req.Start) != "" || strings.TrimSpace(req.End) != "" {
		return strings.TrimSpace(req.Start), strings.TrimSpace(req.End)
	}
	if req.TimeType == 0 {
		return "", ""
	}
	now := time.Now()
	switch req.TimeType {
	case 1:
		end := now
		start := now.Add(-24 * time.Hour)
		return start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05")
	case 2:
		start := now.AddDate(-1, 0, 0)
		return start.Format("2006-01-02") + " 00:00:00", now.Format("2006-01-02") + " 23:59:59"
	case 3:
		start := now.AddDate(0, 0, -1)
		return start.Format("2006-01-02") + " 00:00:00", start.Format("2006-01-02") + " 23:59:59"
	case 4:
		start := now.AddDate(0, 0, -3)
		return start.Format("2006-01-02") + " 00:00:00", now.Format("2006-01-02") + " 23:59:59"
	case 5:
		start := now.AddDate(0, 0, -7)
		return start.Format("2006-01-02") + " 00:00:00", now.Format("2006-01-02") + " 23:59:59"
	case 6:
		start := now.AddDate(0, 0, -15)
		return start.Format("2006-01-02") + " 00:00:00", now.Format("2006-01-02") + " 23:59:59"
	case 7:
		start := now.AddDate(0, 0, -30)
		return start.Format("2006-01-02") + " 00:00:00", now.Format("2006-01-02") + " 23:59:59"
	case 8:
		return strings.TrimSpace(req.Times), strings.TrimSpace(req.Timee)
	default:
		return "", ""
	}
}

func bucketLegacySearchItems(items []model.Item, kind string) []legacyFacetBucket {
	counts := map[string]int{}
	for _, item := range items {
		meta := legacySearchMetadata(item)
		for _, value := range meta[kind] {
			counts[value]++
		}
	}
	buckets := make([]legacyFacetBucket, 0, len(counts))
	for value, count := range counts {
		buckets = append(buckets, legacyFacetBucket{Key: value, DocCount: count})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].DocCount == buckets[j].DocCount {
			return buckets[i].Key < buckets[j].Key
		}
		return buckets[i].DocCount > buckets[j].DocCount
	})
	return buckets
}

func legacySearchMetadata(item model.Item) map[string][]string {
	meta := map[string][]string{
		"industry": {},
		"province": {},
		"city":     {},
		"event":    {},
	}
	if strings.TrimSpace(item.RawPayload) != "" {
		var payload any
		if err := json.Unmarshal([]byte(item.RawPayload), &payload); err == nil {
			collectLegacyMetadata(payload, meta)
		}
	}
	if len(meta["event"]) == 0 && strings.TrimSpace(item.TagFlags) != "" {
		meta["event"] = append(meta["event"], splitLegacyLabels(item.TagFlags)...)
	}
	for key, values := range meta {
		meta[key] = uniqueLegacyStrings(values)
	}
	return meta
}

func collectLegacyMetadata(value any, meta map[string][]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			switch normalized {
			case "industry", "industries", "province", "provinces", "city", "cities":
				metaKey := normalized
				switch normalized {
				case "industries":
					metaKey = "industry"
				case "provinces":
					metaKey = "province"
				case "cities":
					metaKey = "city"
				}
				meta[metaKey] = append(meta[metaKey], legacyValueStrings(child)...)
			case "event", "events", "eventlable", "eventlabel", "eventindex", "tag_flags", "tagflags":
				meta["event"] = append(meta["event"], legacyValueStrings(child)...)
			}
			collectLegacyMetadata(child, meta)
		}
	case []any:
		for _, child := range typed {
			collectLegacyMetadata(child, meta)
		}
	}
}

func legacyValueStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return splitLegacyLabels(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, legacyValueStrings(item)...)
		}
		return out
	case map[string]any:
		out := make([]string, 0)
		for _, child := range typed {
			out = append(out, legacyValueStrings(child)...)
		}
		return out
	case float64:
		return []string{strconv.FormatFloat(typed, 'f', -1, 64)}
	case int:
		return []string{strconv.Itoa(typed)}
	default:
		return nil
	}
}

func splitLegacyLabels(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '|', '/', '\n', '\t':
			return true
		default:
			return false
		}
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		return []string{value}
	}
	return result
}

func uniqueLegacyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func legacySearchSuccessMessage(kind string) string {
	switch kind {
	case "industry":
		return "行业标签列表成功"
	case "event":
		return "事件标签列表成功"
	case "province":
		return "省份列表成功"
	case "city":
		return "城市列表成功"
	default:
		return "查询成功"
	}
}

func writeLegacyJSON(w http.ResponseWriter, status int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": status,
		"msg":  message,
		"data": data,
	})
}

func writeLegacyStatusJSON(w http.ResponseWriter, status int, args ...any) {
	message := "OK"
	data := any(nil)
	responseStatus := status
	switch len(args) {
	case 0:
	case 1:
		message = fmt.Sprint(args[0])
	case 2:
		message = fmt.Sprint(args[0])
		data = args[1]
	case 3:
		if parsed, ok := args[0].(int); ok {
			responseStatus = parsed
		}
		message = fmt.Sprint(args[1])
		data = args[2]
	default:
		if parsed, ok := args[0].(int); ok {
			responseStatus = parsed
		}
		if len(args) > 1 {
			message = fmt.Sprint(args[1])
		}
		if len(args) > 2 {
			data = args[2]
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": responseStatus,
		"msg":    message,
		"data":   data,
	})
}

func writeJSONBool(w http.ResponseWriter, value bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(value)
}

func writeRawJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeResultUtilJSON(w http.ResponseWriter, status int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"msg":    msg,
		"data":   data,
	})
}

func legacyContactPopupKey(projectID int64) string {
	return legacyContactPopupKeyPrefix + strconv.FormatInt(projectID, 10)
}

func legacyStringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func legacyIntFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	default:
		return 0, false
	}
}

func (s *Server) getPopupState(userID int64, key string) (model.PopupState, bool, error) {
	var state model.PopupState
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/system/popup?user_id="+strconv.FormatInt(userID, 10)+"&key="+url.QueryEscape(key), &state)
	if err != nil {
		return model.PopupState{}, false, err
	}
	if state.UpdatedAt.IsZero() {
		return state, false, nil
	}
	return state, true, nil
}

func (s *Server) putPopupState(state model.PopupState) (model.PopupState, error) {
	var envelope struct {
		Data model.PopupState `json:"data"`
	}
	resp, err := s.client.R().
		SetBody(state).
		SetResult(&envelope).
		Put(s.cfg.ContentURL + "/api/v1/system/popup")
	if err != nil {
		return model.PopupState{}, err
	}
	if !resp.IsSuccess() {
		return model.PopupState{}, errors.New(resp.Status())
	}
	return envelope.Data, nil
}

func (s *Server) getMailConfig() (model.MailConfig, error) {
	var cfg model.MailConfig
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/system/mail-config", &cfg); err != nil {
		return model.MailConfig{}, err
	}
	return cfg, nil
}

func (s *Server) putMailConfig(cfg model.MailConfig) (model.MailConfig, error) {
	var envelope struct {
		Data model.MailConfig `json:"data"`
	}
	resp, err := s.client.R().
		SetBody(cfg).
		SetResult(&envelope).
		Put(s.cfg.ContentURL + "/api/v1/system/mail-config")
	if err != nil {
		return model.MailConfig{}, err
	}
	if !resp.IsSuccess() {
		return model.MailConfig{}, errors.New(resp.Status())
	}
	return envelope.Data, nil
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		resp, err := s.client.R().Post(s.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
		message := "分析刷新已提交"
		if err != nil || !resp.IsSuccess() {
			message = "分析刷新失败"
		}
		http.Redirect(w, r, "/?msg="+message, http.StatusSeeOther)
		return
	}
	dashboard := model.DashboardSnapshot{}
	notices := []model.SystemNotice{}
	taskRuns := []model.TaskRun{}
	crawlRuns := []model.CrawlRun{}
	projects := []model.Project{}
	rules := []model.MonitorRule{}
	reports := []model.Report{}
	articles := model.ItemListResult{}
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/overview", &dashboard)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=1", &articles); err == nil {
		dashboard.Overview.ArticleCount = articles.Total
	}
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects); err == nil {
		dashboard.Overview.ProjectCount = len(projects)
	}
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/reports", &reports); err == nil {
		dashboard.Overview.ReportCount = len(reports)
	}
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/monitor-rules", &rules); err == nil {
		activeRules := 0
		for _, rule := range rules {
			if strings.EqualFold(strings.TrimSpace(rule.Status), "active") {
				activeRules++
			}
		}
		dashboard.Overview.AlertRuleCount = activeRules
	}
	_ = s.render(w, "dashboard", pageData{
		Title:     "总览",
		User:      user,
		Dashboard: dashboard,
		Notices:   notices,
		TaskRuns:  taskRuns,
		CrawlRuns: crawlRuns,
		Message:   r.URL.Query().Get("msg"),
	})
}

func isRemovedLegacyPortalPath(path string) bool {
	_, ok := removedLegacyPortalStatus(path)
	return ok
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		message := "操作已完成"
		switch r.FormValue("form_type") {
		case "group":
			action := r.FormValue("action")
			groupID := r.FormValue("group_id")
			body := map[string]string{
				"name":        r.FormValue("name"),
				"description": r.FormValue("description"),
			}
			switch action {
			case "update":
				resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/project-groups/" + groupID)
				if err != nil || !resp.IsSuccess() {
					message = "项目组更新失败"
				} else {
					message = "项目组更新成功"
				}
			case "delete":
				resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/project-groups/" + groupID)
				if err != nil || !resp.IsSuccess() {
					message = "项目组删除失败"
				} else {
					message = "项目组已删除"
				}
			default:
				resp, err := s.client.R().SetBody(body).Post(s.cfg.ContentURL + "/api/v1/project-groups")
				if err != nil || !resp.IsSuccess() {
					message = "项目组创建失败"
				} else {
					message = "项目组创建成功"
				}
			}
		case "project":
			action := r.FormValue("action")
			groupID, _ := strconv.ParseInt(r.FormValue("group_id"), 10, 64)
			body := map[string]any{
				"group_id":    groupID,
				"name":        r.FormValue("name"),
				"keywords":    r.FormValue("keywords"),
				"description": r.FormValue("description"),
				"status":      nonEmpty(r.FormValue("status"), "active"),
			}
			projectID := r.FormValue("project_id")
			switch action {
			case "update":
				resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/projects/" + projectID)
				if err != nil || !resp.IsSuccess() {
					message = "项目更新失败"
				} else {
					message = "项目更新成功"
				}
			case "delete":
				resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/projects/" + projectID)
				if err != nil || !resp.IsSuccess() {
					message = "项目删除失败"
				} else {
					message = "项目已删除"
				}
			default:
				resp, err := s.client.R().SetBody(body).Post(s.cfg.ContentURL + "/api/v1/projects")
				if err != nil || !resp.IsSuccess() {
					message = "项目创建失败"
				} else {
					message = "项目创建成功"
				}
			}
		}
		redirectURL := localRedirectTarget(r.Referer())
		if strings.TrimSpace(redirectURL) == "" {
			redirectURL = "/projects"
		}
		http.Redirect(w, r, appendMessage(redirectURL, message), http.StatusSeeOther)
		return
	}
	groups := []model.ProjectGroup{}
	projects := []model.Project{}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/project-groups", &groups)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	if keyword != "" || status != "" {
		groupFiltered := make([]model.ProjectGroup, 0, len(groups))
		projectFiltered := make([]model.Project, 0, len(projects))
		lowerKeyword := strings.ToLower(keyword)
		for _, group := range groups {
			if keyword != "" && (strings.Contains(strings.ToLower(group.Name), lowerKeyword) || strings.Contains(strings.ToLower(group.Description), lowerKeyword)) {
				groupFiltered = append(groupFiltered, group)
			}
		}
		for _, project := range projects {
			if status != "" && project.Status != status {
				continue
			}
			if keyword != "" {
				if strings.Contains(strings.ToLower(project.Name), lowerKeyword) ||
					strings.Contains(strings.ToLower(project.Keywords), lowerKeyword) ||
					strings.Contains(strings.ToLower(project.Description), lowerKeyword) ||
					strings.Contains(strings.ToLower(project.GroupName), lowerKeyword) {
					projectFiltered = append(projectFiltered, project)
				}
				continue
			}
			projectFiltered = append(projectFiltered, project)
		}
		if keyword != "" {
			groups = groupFiltered
		}
		projects = projectFiltered
	}
	activeCount := 0
	pausedCount := 0
	for _, project := range projects {
		switch project.Status {
		case "active":
			activeCount++
		case "paused":
			pausedCount++
		}
	}
	_ = s.render(w, "projects", pageData{Title: "项目中心", User: user, Groups: groups, Projects: projects, FilterKeyword: keyword, FilterStatus: status, CountActive: activeCount, CountPaused: pausedCount, Message: r.URL.Query().Get("msg")})
}

func (s *Server) handleCrawlTemplates(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/crawl-templates/") {
		templateID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/crawl-templates/"), "/")
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		if action == "" {
			action = strings.TrimSpace(r.FormValue("action"))
		}
		if templateID == "" {
			http.Redirect(w, r, "/crawl-templates?msg="+url.QueryEscape("模板编号缺失"), http.StatusSeeOther)
			return
		}
		tpl, err := s.fetchCrawlTemplate(r.Context(), templateID)
		if err != nil {
			http.Redirect(w, r, "/crawl-templates?msg="+url.QueryEscape("模板获取失败"), http.StatusSeeOther)
			return
		}
		payload, _ := json.Marshal(tpl)
		target := s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl/templates/"
		switch action {
		case "preview":
			target += "preview"
		default:
			target += "run"
		}
		resp, err := s.client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(payload).
			SetQueryParam("keyword", strings.TrimSpace(r.URL.Query().Get("keyword"))).
			Post(target)
		msg := "模板执行失败"
		if err == nil && resp != nil && resp.IsSuccess() {
			msg = "模板执行已触发"
		}
		http.Redirect(w, r, "/crawl-templates?msg="+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		message := "模板操作已提交"
		formType := r.FormValue("form_type")
		if formType == "" || formType == "template" {
			action := strings.TrimSpace(r.FormValue("action"))
			templateID := strings.TrimSpace(nonEmpty(r.FormValue("template_id"), strings.TrimPrefix(r.URL.Path, "/crawl-templates/")))
			body := map[string]any{
				"name":        strings.TrimSpace(r.FormValue("name")),
				"website":     strings.TrimSpace(r.FormValue("website")),
				"source_type": strings.TrimSpace(r.FormValue("source_type")),
				"enabled":     r.FormValue("enabled") == "on" || strings.EqualFold(r.FormValue("enabled"), "true"),
				"config_json": normalizeTemplateConfigJSON(r.FormValue("config_json"), r.FormValue("website"), r.FormValue("config_method"), r.FormValue("config_base_url"), r.FormValue("config_list_selector"), r.FormValue("config_detail_url_field")),
			}
			switch action {
			case "delete":
				resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/crawl-templates/" + templateID)
				if err != nil || !resp.IsSuccess() {
					message = "模板删除失败"
				} else {
					message = "模板已删除"
				}
			case "update":
				resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/crawl-templates/" + templateID)
				if err != nil || !resp.IsSuccess() {
					message = "模板更新失败"
				} else {
					message = "模板更新成功"
				}
			default:
				resp, err := s.client.R().SetBody(body).Post(s.cfg.ContentURL + "/api/v1/crawl-templates")
				if err != nil || !resp.IsSuccess() {
					message = "模板创建失败"
				} else {
					message = "模板创建成功"
				}
			}
		}
		http.Redirect(w, r, "/crawl-templates?msg="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	var templates []model.CrawlTemplate
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/crawl-templates", &templates)
	var runs []model.CrawlRun
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &runs)
	enabledCount := 0
	for _, tpl := range templates {
		if tpl.Enabled {
			enabledCount++
		}
	}
	_ = s.render(w, "crawl_templates", pageData{
		Title:          "模板中心",
		User:           user,
		CrawlTemplates: templates,
		CrawlRuns:      runs,
		CountActive:    enabledCount,
		Message:        r.URL.Query().Get("msg"),
	})
}

func (s *Server) fetchCrawlTemplate(ctx context.Context, id string) (model.CrawlTemplate, error) {
	var tpl model.CrawlTemplate
	if err := s.getJSONWithContext(ctx, s.cfg.ContentURL+"/api/v1/crawl-templates/"+url.PathEscape(id), &tpl); err != nil {
		return model.CrawlTemplate{}, err
	}
	return tpl, nil
}

func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request, user any) {
	id := strings.TrimPrefix(r.URL.Path, "/projects/")
	if id == "" {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		message := "操作已提交"
		projectID, _ := strconv.ParseInt(id, 10, 64)
		switch r.FormValue("form_type") {
		case "project":
			action := r.FormValue("action")
			groupID, _ := strconv.ParseInt(r.FormValue("group_id"), 10, 64)
			body := map[string]any{
				"group_id":    groupID,
				"name":        r.FormValue("name"),
				"keywords":    r.FormValue("keywords"),
				"description": r.FormValue("description"),
				"status":      nonEmpty(r.FormValue("status"), "active"),
			}
			if action == "delete" {
				resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/projects/" + id)
				if err != nil || !resp.IsSuccess() {
					message = "项目删除失败"
					http.Redirect(w, r, "/projects/"+id+"?msg="+message, http.StatusSeeOther)
					return
				}
				http.Redirect(w, r, "/projects?msg=项目已删除", http.StatusSeeOther)
				return
			}
			resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/projects/" + id)
			if err != nil || !resp.IsSuccess() {
				message = "项目更新失败"
			} else {
				message = "项目更新成功"
			}
		case "rule":
			resp, err := s.client.R().SetBody(map[string]any{
				"project_id":       projectID,
				"name":             r.FormValue("name"),
				"include_keywords": r.FormValue("include_keywords"),
				"exclude_keywords": r.FormValue("exclude_keywords"),
				"channels":         r.FormValue("channels"),
				"severity":         nonEmpty(r.FormValue("severity"), "medium"),
				"status":           "active",
			}).Post(s.cfg.ContentURL + "/api/v1/monitor-rules")
			if err != nil || !resp.IsSuccess() {
				message = "创建规则失败"
			} else {
				message = "规则创建成功"
			}
		case "report":
			resp, err := s.client.R().SetBody(map[string]any{
				"project_id": projectID,
				"title":      r.FormValue("title"),
				"text":       r.FormValue("content"),
			}).Post(s.cfg.ContentURL + "/api/v1/reports/generate")
			if err != nil || !resp.IsSuccess() {
				message = "生成报告失败"
			} else {
				message = "报告生成成功"
			}
		case "crawl":
			req := s.client.R()
			if templateID := strings.TrimSpace(r.FormValue("template_id")); templateID != "" {
				if ok := applyManualCrawlTemplate(req, templateID); !ok {
					message = "不支持的抓取模板"
					break
				}
				if keyword := strings.TrimSpace(r.FormValue("keyword")); keyword != "" {
					req.SetQueryParam("keyword", keyword)
				}
				resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
				if err != nil || !resp.IsSuccess() {
					message = "项目模板抓取触发失败"
				} else {
					message = "项目模板抓取已触发"
				}
				break
			}
			if ok := applyManualCrawlSourceType(req, r.FormValue("source_type")); !ok {
				message = "不支持的抓取来源"
				break
			}
			resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
			if err != nil || !resp.IsSuccess() {
				message = "项目抓取触发失败"
			} else {
				message = "项目抓取已触发"
			}
		case "analysis":
			resp, err := s.client.R().Post(s.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
			if err != nil || !resp.IsSuccess() {
				message = "项目分析刷新失败"
			} else {
				message = "项目分析已刷新"
			}
		}
		http.Redirect(w, r, "/projects/"+id+"?msg="+message, http.StatusSeeOther)
		return
	}
	project := model.Project{}
	articles := model.ItemListResult{}
	crawlRuns := []model.CrawlRun{}
	reports := []model.Report{}
	allRules := []model.MonitorRule{}
	taskRuns := []model.TaskRun{}
	groups := []model.ProjectGroup{}
	crawlTemplates := s.loadCrawlTemplates()
	projectID, _ := strconv.ParseInt(id, 10, 64)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+id, &project)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=10&project_id="+id, &articles)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+id, &reports)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/monitor-rules", &allRules)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/project-groups", &groups)
	rules := make([]model.MonitorRule, 0, len(allRules))
	activeRuleCount := 0
	pausedRuleCount := 0
	for _, rule := range allRules {
		if rule.ProjectID == projectID {
			rules = append(rules, rule)
			if rule.Status == "active" {
				activeRuleCount++
			} else if rule.Status == "paused" {
				pausedRuleCount++
			}
		}
	}
	draftCount := 0
	generatedCount := 0
	archivedCount := 0
	for _, report := range reports {
		switch report.Status {
		case "draft":
			draftCount++
		case "generated":
			generatedCount++
		case "archived":
			archivedCount++
		}
	}
	_ = s.render(w, "project", pageData{
		Title:          "项目详情",
		User:           user,
		Project:        project,
		Groups:         groups,
		Rules:          rules,
		Articles:       articles,
		CrawlRuns:      crawlRuns,
		Reports:        reports,
		TaskRuns:       taskRuns,
		CrawlTemplates: crawlTemplates,
		Message:        r.URL.Query().Get("msg"),
		CountActive:    activeRuleCount,
		CountPaused:    pausedRuleCount,
		CountDraft:     draftCount,
		CountGenerated: generatedCount,
		CountArchived:  archivedCount,
	})
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		action := r.FormValue("action")
		message := "规则创建成功"
		projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
		ruleName := strings.TrimSpace(r.FormValue("name"))
		includeKeywords := strings.TrimSpace(r.FormValue("include_keywords"))
		excludeKeywords := strings.TrimSpace(r.FormValue("exclude_keywords"))
		channels := strings.TrimSpace(r.FormValue("channels"))
		severity := nonEmpty(r.FormValue("severity"), "medium")
		status := nonEmpty(r.FormValue("status"), "active")
		redirectURL := localRedirectTarget(r.Referer())
		if strings.TrimSpace(redirectURL) == "" {
			redirectURL = "/monitor-rules"
		}
		if action == "" {
			if ruleName == "" {
				ruleName = defaultRuleName(includeKeywords, channels)
			}
			if ruleName == "" {
				http.Redirect(w, r, appendMessage(redirectURL, "请填写规则名称、包含关键词或来源"), http.StatusSeeOther)
				return
			}
			if projectID <= 0 {
				projectName := defaultRuleProjectName(strings.TrimSpace(r.FormValue("project_name")), ruleName, includeKeywords)
				createdProject, err := s.createProjectForRule(projectName, includeKeywords)
				if err != nil {
					http.Redirect(w, r, appendMessage(redirectURL, "规则创建失败：项目创建失败："+err.Error()), http.StatusSeeOther)
					return
				}
				projectID = createdProject.ID
			}
		}
		body := map[string]any{
			"project_id":       projectID,
			"name":             ruleName,
			"include_keywords": includeKeywords,
			"exclude_keywords": excludeKeywords,
			"channels":         channels,
			"severity":         severity,
			"status":           status,
		}
		ruleID := r.FormValue("rule_id")
		switch action {
		case "update":
			resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/monitor-rules/" + ruleID)
			if err != nil || !resp.IsSuccess() {
				message = "规则更新失败：" + responseErrorMessage(resp, err)
			} else {
				message = "规则更新成功"
			}
		case "toggle":
			status := "active"
			if r.FormValue("status") == "active" {
				status = "paused"
				message = "规则已停用"
			} else {
				message = "规则已启用"
			}
			body["status"] = status
			resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/monitor-rules/" + ruleID)
			if err != nil || !resp.IsSuccess() {
				message = "规则状态更新失败：" + responseErrorMessage(resp, err)
			}
		case "delete":
			resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/monitor-rules/" + ruleID)
			if err != nil || !resp.IsSuccess() {
				message = "规则删除失败：" + responseErrorMessage(resp, err)
			} else {
				message = "规则已删除"
			}
		default:
			resp, err := s.client.R().SetBody(body).Post(s.cfg.ContentURL + "/api/v1/monitor-rules")
			if err != nil || !resp.IsSuccess() {
				message = "规则创建失败：" + responseErrorMessage(resp, err)
			}
		}
		http.Redirect(w, r, appendMessage(redirectURL, message), http.StatusSeeOther)
		return
	}
	rules := []model.MonitorRule{}
	projects := []model.Project{}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/monitor-rules", &rules)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	if keyword != "" || status != "" {
		filtered := make([]model.MonitorRule, 0, len(rules))
		lowerKeyword := strings.ToLower(keyword)
		for _, rule := range rules {
			if status != "" && rule.Status != status {
				continue
			}
			if keyword != "" {
				if strings.Contains(strings.ToLower(rule.Name), lowerKeyword) ||
					strings.Contains(strings.ToLower(rule.ProjectName), lowerKeyword) ||
					strings.Contains(strings.ToLower(rule.IncludeKeywords), lowerKeyword) ||
					strings.Contains(strings.ToLower(rule.ExcludeKeywords), lowerKeyword) ||
					strings.Contains(strings.ToLower(rule.Channels), lowerKeyword) {
					filtered = append(filtered, rule)
				}
				continue
			}
			filtered = append(filtered, rule)
		}
		rules = filtered
	}
	activeCount := 0
	pausedCount := 0
	for _, rule := range rules {
		switch rule.Status {
		case "active":
			activeCount++
		case "paused":
			pausedCount++
		}
	}
	_ = s.render(w, "rules", pageData{Title: "监测规则", User: user, Rules: rules, Projects: projects, FilterKeyword: keyword, FilterStatus: status, CountActive: activeCount, CountPaused: pausedCount, Message: r.URL.Query().Get("msg")})
}

func (s *Server) handleRuleDetail(w http.ResponseWriter, r *http.Request, user any) {
	id := strings.TrimPrefix(r.URL.Path, "/monitor-rules/")
	if id == "" {
		http.Redirect(w, r, "/monitor-rules", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		formType := r.FormValue("form_type")
		if formType == "crawl" || formType == "analysis" {
			projectID := strings.TrimSpace(r.FormValue("project_id"))
			message := "任务提交失败"
			switch formType {
			case "crawl":
				req := s.client.R()
				if templateID := strings.TrimSpace(r.FormValue("template_id")); templateID != "" {
					if ok := applyManualCrawlTemplate(req, templateID); !ok {
						message = "不支持的抓取模板"
						break
					}
					if keyword := strings.TrimSpace(r.FormValue("keyword")); keyword != "" {
						req.SetQueryParam("keyword", keyword)
					}
					resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
					if err == nil && resp.IsSuccess() {
						message = "规则关联项目模板抓取已触发"
					}
					break
				}
				if ok := applyManualCrawlSourceType(req, r.FormValue("source_type")); !ok {
					message = "不支持的抓取来源"
					break
				}
				resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
				if err == nil && resp.IsSuccess() {
					message = "规则关联项目抓取已触发"
				}
			case "analysis":
				resp, err := s.client.R().Post(s.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
				if err == nil && resp.IsSuccess() {
					message = "规则关联项目分析已刷新"
				}
			}
			target := "/monitor-rules/" + id
			if projectID != "" {
				target += "?project_id=" + projectID
			}
			http.Redirect(w, r, appendMessage(target, message), http.StatusSeeOther)
			return
		}
		action := r.FormValue("action")
		projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
		body := map[string]any{
			"project_id":       projectID,
			"name":             r.FormValue("name"),
			"include_keywords": r.FormValue("include_keywords"),
			"exclude_keywords": r.FormValue("exclude_keywords"),
			"channels":         r.FormValue("channels"),
			"severity":         nonEmpty(r.FormValue("severity"), "medium"),
			"status":           nonEmpty(r.FormValue("status"), "active"),
		}
		message := "规则更新成功"
		switch action {
		case "toggle":
			if body["status"] == "active" {
				body["status"] = "paused"
				message = "规则已停用"
			} else {
				body["status"] = "active"
				message = "规则已启用"
			}
			resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/monitor-rules/" + id)
			if err != nil || !resp.IsSuccess() {
				message = "规则状态更新失败"
			}
		case "delete":
			resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/monitor-rules/" + id)
			if err != nil || !resp.IsSuccess() {
				http.Redirect(w, r, "/monitor-rules/"+id+"?msg=规则删除失败", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/monitor-rules?msg=规则已删除", http.StatusSeeOther)
			return
		default:
			resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/monitor-rules/" + id)
			if err != nil || !resp.IsSuccess() {
				message = "规则更新失败"
			}
		}
		http.Redirect(w, r, "/monitor-rules/"+id+"?msg="+message, http.StatusSeeOther)
		return
	}
	rule := model.MonitorRule{}
	project := model.Project{}
	articles := model.ItemListResult{}
	reports := []model.Report{}
	crawlRuns := []model.CrawlRun{}
	taskRuns := []model.TaskRun{}
	crawlTemplates := s.loadCrawlTemplates()
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/monitor-rules/"+id, &rule)
	if rule.ProjectID > 0 {
		projectID := strconv.FormatInt(rule.ProjectID, 10)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+projectID, &project)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=10&project_id="+projectID, &articles)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+projectID, &reports)
		_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	}
	_ = s.render(w, "rule", pageData{Title: "规则详情", User: user, Rule: rule, Project: project, Articles: articles, Reports: reports, CrawlRuns: crawlRuns, TaskRuns: taskRuns, CrawlTemplates: crawlTemplates, Message: r.URL.Query().Get("msg")})
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		itemID := r.FormValue("item_id")
		action := r.FormValue("action")
		message := s.performPortalArticleAction(itemID, action, userID, portalArticleActionOptions{
			Emotion: r.FormValue("emotion"),
			Channel: "portal-list",
		})
		redirectURL := localRedirectTarget(r.Referer())
		if strings.TrimSpace(redirectURL) == "" {
			redirectURL = "/articles"
		}
		http.Redirect(w, r, appendMessage(redirectURL, message), http.StatusSeeOther)
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	pageNum := maxInt(parseIntDefault(strings.TrimSpace(r.URL.Query().Get("page")), 1), 1)
	pageSize := 20
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	sourceType := strings.TrimSpace(r.URL.Query().Get("source_type"))
	readFilter := strings.TrimSpace(r.URL.Query().Get("read"))
	flagFilter := strings.TrimSpace(r.URL.Query().Get("favorite"))
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	industry := strings.TrimSpace(r.URL.Query().Get("industry"))
	province := strings.TrimSpace(r.URL.Query().Get("province"))
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	articleSort := normalizeArticleListSort(r.URL.Query().Get("sort"))
	articleTimeField := articleListTimeField(articleSort)
	query := "/api/v1/articles?page=" + strconv.Itoa(pageNum) + "&page_size=" + strconv.Itoa(pageSize) + "&time_field=" + url.QueryEscape(articleTimeField) + "&sort=" + url.QueryEscape(articleSort)
	if mode == "search" {
		query = "/api/v1/search/articles?page=" + strconv.Itoa(pageNum) + "&page_size=" + strconv.Itoa(pageSize) + "&time_field=" + url.QueryEscape(articleTimeField) + "&sort=" + url.QueryEscape(articleSort)
		if keyword != "" {
			query += "&q=" + url.QueryEscape(keyword)
		}
	} else if mode == "full" {
		query = "/api/v1/search/full?page=" + strconv.Itoa(pageNum) + "&page_size=" + strconv.Itoa(pageSize) + "&time_field=" + url.QueryEscape(articleTimeField) + "&sort=" + url.QueryEscape(articleSort)
		if keyword != "" {
			query += "&q=" + url.QueryEscape(keyword)
		}
	} else if mode == "timely" {
		query = "/api/v1/search/timely?page=" + strconv.Itoa(pageNum) + "&page_size=" + strconv.Itoa(pageSize) + "&time_field=" + url.QueryEscape(articleTimeField) + "&sort=" + url.QueryEscape(articleSort)
		if keyword != "" {
			query += "&q=" + url.QueryEscape(keyword)
		}
	} else if keyword != "" {
		query += "&keyword=" + url.QueryEscape(keyword)
	}
	if projectID != "" {
		query += "&project_id=" + projectID
	}
	if sourceType != "" {
		query += "&source_type=" + sourceType
	}
	if start != "" {
		query += "&start=" + start
	}
	if end != "" {
		query += "&end=" + end
	}
	if industry != "" {
		query += "&industry=" + url.QueryEscape(industry)
	}
	if province != "" {
		query += "&province=" + url.QueryEscape(province)
	}
	if city != "" {
		query += "&city=" + url.QueryEscape(city)
	}
	if readFilter != "" {
		query += "&read=" + url.QueryEscape(readFilter)
	}
	if flagFilter != "" {
		query += "&favorite=" + url.QueryEscape(flagFilter)
	}
	if userID > 0 {
		query += "&user_id=" + strconv.FormatInt(userID, 10)
	}
	articles := model.ItemListResult{}
	projects := []model.Project{}
	options := model.SearchOptions{}
	_ = s.getJSON(s.cfg.ContentURL+query, &articles)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/search/options", &options)
	readCount := 0
	unreadCount := 0
	flaggedCount := 0
	for index, item := range articles.Items {
		articles.Items[index].SourceType = articleSourceSiteLabel(item)
		if item.Read {
			readCount++
		} else {
			unreadCount++
		}
		if item.Favorited {
			flaggedCount++
		}
	}
	totalPages := maxInt((articles.Total+pageSize-1)/pageSize, 1)
	prevURL := buildPageURL(r, maxInt(pageNum-1, 1))
	nextURL := buildPageURL(r, minInt(pageNum+1, totalPages))
	returnTo := url.QueryEscape(r.URL.RequestURI())
	_ = s.render(w, "articles", pageData{
		Title:             "文章中心",
		User:              user,
		Articles:          articles,
		Projects:          projects,
		ReturnTo:          returnTo,
		FilterKeyword:     keyword,
		FilterProject:     projectID,
		FilterRead:        readFilter,
		FilterFlag:        flagFilter,
		FilterSource:      sourceType,
		FilterStart:       start,
		FilterEnd:         end,
		FilterIndustry:    industry,
		FilterProvince:    province,
		FilterCity:        city,
		SearchMode:        mode,
		SearchOptions:     options,
		CountRead:         readCount,
		CountUnread:       unreadCount,
		CountFlagged:      flaggedCount,
		ArticlePage:       pageNum,
		ArticlePagePrev:   maxInt(pageNum-1, 1),
		ArticlePageNext:   minInt(pageNum+1, totalPages),
		ArticleTotalPages: totalPages,
		ArticlePrevURL:    prevURL,
		ArticleNextURL:    nextURL,
		ArticleSort:       articleSort,
		ArticleTimeLabel:  articleListTimeLabel(articleSort),
		Message:           r.URL.Query().Get("msg"),
	})
}

func (s *Server) handleArticleDetail(w http.ResponseWriter, r *http.Request, user any) {
	id := strings.TrimPrefix(r.URL.Path, "/articles/")
	if id == "" {
		http.Redirect(w, r, "/articles", http.StatusSeeOther)
		return
	}
	returnURL := "/articles"
	if encoded := strings.TrimSpace(r.URL.Query().Get("return_to")); encoded != "" {
		if decoded, err := url.QueryUnescape(encoded); err == nil && strings.HasPrefix(decoded, "/articles") {
			returnURL = decoded
		}
	}
	userID := userIDFromMap(user)
	if r.Method == http.MethodPost && userID > 0 {
		_ = r.ParseForm()
		action := r.FormValue("action")
		message := s.performPortalArticleAction(id, action, userID, portalArticleActionOptions{
			Emotion: r.FormValue("emotion"),
			Channel: "portal-detail",
		})
		if (action == "hide" || action == "delete") && message == "文章已隐藏" {
			http.Redirect(w, r, appendMessage(returnURL, message), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, appendMessage("/articles/"+id+"?return_to="+url.QueryEscape(returnURL), message), http.StatusSeeOther)
		return
	}
	article := model.Item{}
	related := []model.Item{}
	projects := make([]model.Project, 0)
	reports := make([]model.Report, 0)
	querySuffix := ""
	if userID > 0 {
		querySuffix = "?user_id=" + strconv.FormatInt(userID, 10)
	}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles/"+id+querySuffix, &article)
	relatedURL := s.cfg.ContentURL + "/api/v1/articles/" + id + "/related"
	if userID > 0 {
		relatedURL += "?user_id=" + strconv.FormatInt(userID, 10)
	}
	_ = s.getJSON(relatedURL, &related)
	reportSeen := make(map[int64]struct{})
	readCount := 0
	unreadCount := 0
	flaggedCount := 0
	for _, item := range related {
		if item.Read {
			readCount++
		} else {
			unreadCount++
		}
		if item.Favorited {
			flaggedCount++
		}
	}
	for _, projectID := range article.ProjectIDs {
		project := model.Project{}
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+strconv.FormatInt(projectID, 10), &project); err == nil && project.ID > 0 {
			projects = append(projects, project)
		}
		projectReports := []model.Report{}
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+strconv.FormatInt(projectID, 10), &projectReports); err == nil {
			for _, report := range projectReports {
				if _, ok := reportSeen[report.ID]; ok {
					continue
				}
				reportSeen[report.ID] = struct{}{}
				reports = append(reports, report)
			}
		}
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].UpdatedAt.After(reports[j].UpdatedAt)
	})
	draftCount := 0
	generatedCount := 0
	archivedCount := 0
	for _, report := range reports {
		switch report.Status {
		case "draft":
			draftCount++
		case "generated":
			generatedCount++
		case "archived":
			archivedCount++
		}
	}
	if len(reports) > 8 {
		reports = reports[:8]
	}
	projectNames := make(map[int64]string, len(projects))
	for _, project := range projects {
		projectNames[project.ID] = project.Name
	}
	_ = s.render(w, "article", pageData{
		Title:          "文章详情",
		User:           user,
		Article:        article,
		Related:        related,
		Projects:       projects,
		Reports:        reports,
		ProjectNames:   projectNames,
		Message:        r.URL.Query().Get("msg"),
		ReturnURL:      returnURL,
		ReturnTo:       url.QueryEscape(returnURL),
		CountRead:      readCount,
		CountUnread:    unreadCount,
		CountFlagged:   flaggedCount,
		CountDraft:     draftCount,
		CountGenerated: generatedCount,
		CountArchived:  archivedCount,
	})
}

type portalArticleActionOptions struct {
	Emotion string
	Channel string
}

func (s *Server) performPortalArticleAction(itemID string, action string, userID int64, opts portalArticleActionOptions) string {
	if strings.TrimSpace(itemID) == "" || userID <= 0 {
		return "文章操作失败"
	}
	switch action {
	case "favorite":
		resp, err := s.client.R().
			SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
			Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/favorite")
		if err == nil && resp.IsSuccess() {
			return "收藏状态已更新"
		}
	case "read":
		resp, err := s.client.R().
			SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
			Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/read")
		if err == nil && resp.IsSuccess() {
			return "文章已标记为已读"
		}
	case "share":
		resp, err := s.client.R().
			SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
			SetBody(map[string]string{"channel": nonEmpty(opts.Channel, "portal")}).
			Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/share")
		if err == nil && resp.IsSuccess() {
			return "文章已登记分享"
		}
	case "emotion":
		emotion := strings.TrimSpace(nonEmpty(opts.Emotion, "2"))
		resp, err := s.client.R().
			SetQueryParam("emotion", emotion).
			SetQueryParam("flag", emotion).
			SetFormData(map[string]string{"emotion": emotion}).
			Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/emotion")
		if err == nil && resp.IsSuccess() {
			return "情感标记已更新"
		}
	case "hide", "delete":
		resp, err := s.client.R().
			Delete(s.cfg.ContentURL + "/api/v1/articles/" + itemID)
		if err == nil && resp.IsSuccess() {
			return "文章已隐藏"
		}
	}
	return "文章操作失败"
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
		message := "报告生成成功"
		resp, err := s.client.R().SetBody(map[string]any{
			"project_id": projectID,
			"title":      r.FormValue("title"),
			"text":       r.FormValue("content"),
		}).Post(s.cfg.ContentURL + "/api/v1/reports/generate")
		if err != nil || !resp.IsSuccess() {
			message = "报告生成失败"
		}
		redirectURL := localRedirectTarget(r.Referer())
		if strings.TrimSpace(redirectURL) == "" {
			redirectURL = "/reports"
		}
		http.Redirect(w, r, appendMessage(redirectURL, message), http.StatusSeeOther)
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	order := strings.TrimSpace(r.URL.Query().Get("order"))
	returnTo := url.QueryEscape(r.URL.RequestURI())
	reports := []model.Report{}
	projects := []model.Project{}
	projectNames := map[int64]string{}
	reportURL := s.cfg.ContentURL + "/api/v1/reports"
	if projectID != "" {
		reportURL += "?project_id=" + projectID
	}
	_ = s.getJSON(reportURL, &reports)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	for _, project := range projects {
		projectNames[project.ID] = project.Name
	}
	filtered := make([]model.Report, 0, len(reports))
	draftCount := 0
	generatedCount := 0
	archivedCount := 0
	for _, report := range reports {
		if keyword != "" && !strings.Contains(strings.ToLower(report.Title), strings.ToLower(keyword)) {
			continue
		}
		if status != "" && report.Status != status {
			continue
		}
		switch report.Status {
		case "draft":
			draftCount++
		case "generated":
			generatedCount++
		case "archived":
			archivedCount++
		}
		filtered = append(filtered, report)
	}
	if order == "asc" {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].UpdatedAt.Before(filtered[j].UpdatedAt)
		})
	} else {
		order = "desc"
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
		})
	}
	_ = s.render(w, "reports", pageData{Title: "报告中心", User: user, Reports: filtered, Projects: projects, ProjectNames: projectNames, ReturnTo: returnTo, FilterKeyword: keyword, FilterProject: projectID, FilterStatus: status, FilterSource: order, CountDraft: draftCount, CountGenerated: generatedCount, CountArchived: archivedCount, Message: r.URL.Query().Get("msg")})
}

func (s *Server) handleReportDetail(w http.ResponseWriter, r *http.Request, user any) {
	id := strings.TrimPrefix(r.URL.Path, "/reports/")
	if id == "" {
		http.Redirect(w, r, "/reports", http.StatusSeeOther)
		return
	}
	returnURL := "/reports"
	if encoded := strings.TrimSpace(r.URL.Query().Get("return_to")); encoded != "" {
		if decoded, err := url.QueryUnescape(encoded); err == nil && strings.HasPrefix(decoded, "/reports") {
			returnURL = decoded
		}
	}
	report := model.Report{}
	project := model.Project{}
	articles := model.ItemListResult{}
	reports := []model.Report{}
	crawlRuns := []model.CrawlRun{}
	taskRuns := []model.TaskRun{}
	crawlTemplates := s.loadCrawlTemplates()
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports/"+id, &report)
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if formType := r.FormValue("form_type"); formType == "crawl" || formType == "analysis" {
			message := "报告关联任务提交失败"
			switch formType {
			case "crawl":
				req := s.client.R()
				if templateID := strings.TrimSpace(r.FormValue("template_id")); templateID != "" {
					if ok := applyManualCrawlTemplate(req, templateID); !ok {
						message = "不支持的抓取模板"
						break
					}
					if keyword := strings.TrimSpace(r.FormValue("keyword")); keyword != "" {
						req.SetQueryParam("keyword", keyword)
					}
					resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
					if err == nil && resp.IsSuccess() {
						message = "报告关联项目模板抓取已触发"
					}
					break
				}
				if ok := applyManualCrawlSourceType(req, r.FormValue("source_type")); !ok {
					message = "不支持的抓取来源"
					break
				}
				resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
				if err == nil && resp.IsSuccess() {
					message = "报告关联项目抓取已触发"
				}
			case "analysis":
				resp, err := s.client.R().Post(s.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
				if err == nil && resp.IsSuccess() {
					message = "报告关联项目分析已刷新"
				}
			}
			http.Redirect(w, r, appendMessage("/reports/"+id+"?return_to="+url.QueryEscape(returnURL), message), http.StatusSeeOther)
			return
		}
		message := "报告操作失败"
		var generated model.Report
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		resp, err := s.client.R().
			SetBody(map[string]any{
				"project_id": report.ProjectID,
				"title":      nonEmpty(r.FormValue("title"), report.Title+" 重生成"),
				"text":       nonEmpty(r.FormValue("content"), report.Content),
			}).
			SetResult(&envelope).
			Post(s.cfg.ContentURL + "/api/v1/reports/generate")
		if err == nil && resp.IsSuccess() {
			if json.Unmarshal(envelope.Data, &generated) == nil && generated.ID > 0 {
				http.Redirect(w, r, "/reports/"+strconv.FormatInt(generated.ID, 10)+"?msg=报告已重新生成&return_to="+url.QueryEscape(returnURL), http.StatusSeeOther)
				return
			}
			message = "报告已提交重新生成"
		}
		http.Redirect(w, r, "/reports/"+id+"?msg="+message+"&return_to="+url.QueryEscape(returnURL), http.StatusSeeOther)
		return
	}
	if report.ProjectID > 0 {
		projectID := strconv.FormatInt(report.ProjectID, 10)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+projectID, &project)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+projectID, &reports)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=5&project_id="+projectID, &articles)
		_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	}
	relatedReports := make([]model.Report, 0, len(reports))
	draftCount := 0
	generatedCount := 0
	archivedCount := 0
	for _, item := range reports {
		switch item.Status {
		case "draft":
			draftCount++
		case "generated":
			generatedCount++
		case "archived":
			archivedCount++
		}
		if item.ID == report.ID {
			continue
		}
		relatedReports = append(relatedReports, item)
	}
	if len(relatedReports) > 5 {
		relatedReports = relatedReports[:5]
	}
	_ = s.render(w, "report", pageData{
		Title:          "报告详情",
		User:           user,
		Report:         report,
		Project:        project,
		Articles:       articles,
		Reports:        relatedReports,
		CrawlRuns:      crawlRuns,
		TaskRuns:       taskRuns,
		CrawlTemplates: crawlTemplates,
		Message:        r.URL.Query().Get("msg"),
		ReturnURL:      returnURL,
		ReturnTo:       url.QueryEscape(returnURL),
		CountDraft:     draftCount,
		CountGenerated: generatedCount,
		CountArchived:  archivedCount,
	})
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request, user any) {
	sectionKey := normalizeSystemSection(strings.TrimSpace(r.URL.Query().Get("section")))
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	favoritePage := parsePositiveInt(r.URL.Query().Get("page"), 1)
	logService := strings.TrimSpace(r.URL.Query().Get("log_service"))
	aStockRepairDate := normalizeAStockStrategyDate(r.URL.Query().Get("strategy_date"))
	aStockRepairPeriod := normalizeAStockPeriod(r.URL.Query().Get("period")).Key
	aStockRepairIgnoreRecent := normalizeAStockBool(r.URL.Query().Get("ignore_recent"))
	crawlTemplates := s.loadCrawlTemplates()

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		sectionKey = normalizeSystemSection(nonEmpty(r.FormValue("section"), sectionKey))
		projectID = nonEmpty(r.FormValue("project_id"), projectID)
		favoritePage = parsePositiveInt(r.FormValue("page"), favoritePage)
		aStockRepairDate = normalizeAStockStrategyDate(nonEmpty(r.FormValue("strategy_date"), aStockRepairDate))
		aStockRepairPeriod = normalizeAStockPeriod(nonEmpty(r.FormValue("period"), aStockRepairPeriod)).Key
		aStockRepairIgnoreRecent = normalizeAStockBool(nonEmpty(r.FormValue("ignore_recent"), r.FormValue("snapshot_ignore_recent")))
		message := "操作已提交"
		switch strings.TrimSpace(r.FormValue("form_type")) {
		case "profile":
			userID := userIDFromMap(user)
			if userID > 0 {
				if token, ok := s.sessionTokenFromRequest(r); ok {
					resp, err := s.client.R().
						SetQueryParam("session_token", token).
						SetBody(map[string]string{
							"display_name": r.FormValue("display_name"),
							"email":        r.FormValue("email"),
						}).
						Put(s.cfg.AuthURL + "/api/v1/users/" + strconv.FormatInt(userID, 10))
					if err != nil || !resp.IsSuccess() {
						message = "个人资料更新失败"
					} else {
						message = "个人资料已更新"
					}
				}
			}
		case "password":
			userID := userIDFromMap(user)
			if userID > 0 {
				if token, ok := s.sessionTokenFromRequest(r); ok {
					resp, err := s.client.R().
						SetQueryParam("session_token", token).
						SetBody(map[string]string{
							"old_password": r.FormValue("old_password"),
							"new_password": r.FormValue("new_password"),
						}).
						Put(s.cfg.AuthURL + "/api/v1/users/" + strconv.FormatInt(userID, 10) + "/password")
					if err != nil || !resp.IsSuccess() {
						message = "密码修改失败"
					} else {
						message = "密码已修改"
					}
				}
			}
		case "preferences":
			userID := userIDFromMap(user)
			pageSize, _ := strconv.Atoi(r.FormValue("article_page_size"))
			resp, err := s.client.R().SetBody(map[string]any{
				"user_id":             userID,
				"language":            r.FormValue("language"),
				"theme":               r.FormValue("theme"),
				"default_search_mode": r.FormValue("default_search_mode"),
				"article_page_size":   pageSize,
				"email_notifications": r.FormValue("email_notifications") == "on",
			}).Put(s.cfg.ContentURL + "/api/v1/system/preferences")
			if err != nil || !resp.IsSuccess() {
				message = "偏好设置保存失败"
			} else {
				message = "偏好设置已保存"
			}
		case "popup":
			userID := userIDFromMap(user)
			resp, err := s.client.R().SetBody(map[string]any{
				"user_id":   userID,
				"key":       nonEmpty(r.FormValue("key"), "system-announcement"),
				"dismissed": r.FormValue("dismissed") == "on",
			}).Put(s.cfg.ContentURL + "/api/v1/system/popup")
			if err != nil || !resp.IsSuccess() {
				message = "弹窗状态保存失败"
			} else {
				message = "弹窗状态已保存"
			}
		case "mail":
			port, _ := strconv.Atoi(r.FormValue("smtp_port"))
			resp, err := s.client.R().SetBody(map[string]any{
				"enabled":      r.FormValue("enabled") == "on",
				"smtp_host":    r.FormValue("smtp_host"),
				"smtp_port":    port,
				"username":     r.FormValue("username"),
				"password":     r.FormValue("password"),
				"sender_name":  r.FormValue("sender_name"),
				"sender_email": r.FormValue("sender_email"),
			}).Put(s.cfg.ContentURL + "/api/v1/system/mail-config")
			if err != nil || !resp.IsSuccess() {
				message = "邮件配置保存失败"
			} else {
				message = "邮件配置已保存"
			}
		case "database_check":
			resp, err := s.client.R().SetBody(map[string]any{
				"driver":            r.FormValue("driver"),
				"sqlite_path":       r.FormValue("sqlite_path"),
				"postgres_dsn":      r.FormValue("postgres_dsn"),
				"postgres_host":     r.FormValue("postgres_host"),
				"postgres_port":     r.FormValue("postgres_port"),
				"postgres_database": r.FormValue("postgres_database"),
				"postgres_user":     r.FormValue("postgres_user"),
				"postgres_password": r.FormValue("postgres_password"),
				"postgres_sslmode":  r.FormValue("postgres_sslmode"),
			}).Post(s.cfg.ContentURL + "/api/v1/system/database-config/check")
			if err != nil {
				message = "数据库连接检测失败"
			} else if resp.IsSuccess() {
				message = "数据库连接检测通过"
			} else {
				message = "数据库连接检测失败：" + responseErrorMessage(resp, nil)
			}
		case "database_save_config":
			resp, err := s.client.R().SetBody(map[string]any{
				"driver":            r.FormValue("driver"),
				"sqlite_path":       r.FormValue("sqlite_path"),
				"postgres_dsn":      r.FormValue("postgres_dsn"),
				"postgres_host":     r.FormValue("postgres_host"),
				"postgres_port":     r.FormValue("postgres_port"),
				"postgres_database": r.FormValue("postgres_database"),
				"postgres_user":     r.FormValue("postgres_user"),
				"postgres_password": r.FormValue("postgres_password"),
				"postgres_sslmode":  r.FormValue("postgres_sslmode"),
			}).Post(s.cfg.ContentURL + "/api/v1/system/database-config/save")
			if err != nil {
				message = "数据库连接参数保存失败"
			} else if resp.IsSuccess() {
				message = "数据库连接参数配置已保存"
			} else {
				message = "数据库连接参数保存失败：" + responseErrorMessage(resp, nil)
			}
		case "database_switch", "database_switch_postgres", "database_save_postgres", "database_switch_sqlite":
			driver := r.FormValue("driver")
			switch strings.TrimSpace(r.FormValue("form_type")) {
			case "database_switch_postgres", "database_save_postgres":
				driver = "postgres"
			case "database_switch_sqlite":
				driver = "sqlite"
			}
			resp, err := s.client.R().SetBody(map[string]any{
				"driver":            driver,
				"sqlite_path":       r.FormValue("sqlite_path"),
				"postgres_dsn":      r.FormValue("postgres_dsn"),
				"postgres_host":     r.FormValue("postgres_host"),
				"postgres_port":     r.FormValue("postgres_port"),
				"postgres_database": r.FormValue("postgres_database"),
				"postgres_user":     r.FormValue("postgres_user"),
				"postgres_password": r.FormValue("postgres_password"),
				"postgres_sslmode":  r.FormValue("postgres_sslmode"),
			}).Post(s.cfg.ContentURL + "/api/v1/system/database-config/switch")
			if err != nil {
				message = "数据库切换保存失败"
			} else if resp.IsSuccess() {
				message = "数据库切换配置已保存，正在自动重启全部服务"
			} else {
				message = "数据库切换保存失败：" + responseErrorMessage(resp, nil)
			}
		case "warning":
			selectedProjectID, _ := strconv.ParseInt(nonEmpty(r.FormValue("project_id"), projectID), 10, 64)
			if selectedProjectID <= 0 {
				selectedProjectID = firstProjectID(s.cfg.ContentURL, s.client)
			}
			if selectedProjectID > 0 {
				threshold, _ := strconv.Atoi(r.FormValue("threshold"))
				resp, err := s.client.R().SetBody(map[string]any{
					"enabled":     r.FormValue("enabled") == "on",
					"channels":    r.FormValue("channels"),
					"threshold":   threshold,
					"recipients":  r.FormValue("recipients"),
					"description": r.FormValue("description"),
				}).Put(s.cfg.ContentURL + "/api/v1/system/warning-settings/" + strconv.FormatInt(selectedProjectID, 10))
				if err != nil || !resp.IsSuccess() {
					message = "预警设置保存失败"
				} else {
					message = "预警设置已保存"
					projectID = strconv.FormatInt(selectedProjectID, 10)
				}
			} else {
				message = "预警设置保存失败"
			}
		case "feedback":
			resp, err := s.client.R().SetBody(map[string]any{
				"user_id": userIDFromMap(user),
				"title":   r.FormValue("title"),
				"content": r.FormValue("content"),
			}).Post(s.cfg.ContentURL + "/api/v1/system/feedback")
			if err != nil || !resp.IsSuccess() {
				message = "反馈提交失败"
			} else {
				message = "反馈已提交"
			}
		case "delete_feedback":
			feedbackID := strings.TrimSpace(r.FormValue("feedback_id"))
			if feedbackID == "" {
				message = "反馈删除失败"
				break
			}
			resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/system/feedback/" + url.PathEscape(feedbackID))
			if err != nil || !resp.IsSuccess() {
				message = "反馈删除失败"
			} else {
				message = "反馈已删除"
			}
		case "crawl":
			req := s.client.R()
			if templateID := strings.TrimSpace(r.FormValue("template_id")); templateID != "" {
				if ok := applyManualCrawlTemplate(req, templateID); !ok {
					message = "不支持的抓取模板"
					break
				}
				if keyword := strings.TrimSpace(r.FormValue("keyword")); keyword != "" {
					req.SetQueryParam("keyword", keyword)
				}
				resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
				if err != nil || !resp.IsSuccess() {
					message = "模板抓取任务提交失败"
				} else {
					message = "模板抓取任务已提交"
				}
				break
			}
			if ok := applyManualCrawlSourceType(req, r.FormValue("source_type")); !ok {
				message = "不支持的抓取来源"
				break
			}
			resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
			if err != nil || !resp.IsSuccess() {
				message = "抓取任务提交失败"
			} else {
				message = "抓取任务已提交"
			}
		case "analysis":
			resp, err := s.client.R().Post(s.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
			if err != nil || !resp.IsSuccess() {
				message = "分析刷新失败"
			} else {
				message = "分析刷新已提交"
			}
		case "restart_service":
			serviceName := strings.TrimSpace(r.FormValue("service_name"))
			resp, err := s.client.R().
				SetHeader("X-Service-Token", s.cfg.ServiceToken).
				Post(s.cfg.ContentURL + "/api/v1/system/services/" + url.PathEscape(serviceName) + "/restart")
			if err != nil || !resp.IsSuccess() {
				message = "服务重启提交失败"
			} else {
				message = serviceName + " 重启已提交"
			}
		case "astock_repair_selections_save":
			if err := s.saveAStockRepairSelections(aStockRepairDate, aStockRepairPeriod, r.FormValue("selection_items_json")); err != nil {
				message = "推荐股票已选层保存失败：" + err.Error()
			} else {
				message = "推荐股票已选层已保存"
			}
		case "astock_repair_snapshot_save":
			err := s.saveAStockRepairSnapshot(aStockRepairSnapshotInput{
				StrategyDate:             aStockRepairDate,
				Period:                   aStockRepairPeriod,
				IgnoreRecent:             aStockRepairIgnoreRecent,
				RecommendationsJSON:      r.FormValue("snapshot_recommendations_json"),
				BacktestsJSON:            r.FormValue("snapshot_backtests_json"),
				BacktestStatus:           r.FormValue("snapshot_backtest_status"),
				GeneratedCount:           parseIntDefault(r.FormValue("snapshot_generated_count"), 0),
				RecentFiltered:           parseIntDefault(r.FormValue("snapshot_recent_filtered"), 0),
				SameDayMorningFiltered:   parseIntDefault(r.FormValue("snapshot_same_day_morning_filtered"), 0),
				LimitUpFilterEnabled:     r.FormValue("snapshot_limit_up_filter_enabled") == "on",
				LimitUpFiltered:          parseIntDefault(r.FormValue("snapshot_limit_up_filtered"), 0),
				TodayMarketFilterEnabled: r.FormValue("snapshot_today_market_filter_enabled") == "on",
				NoTodayMarketCount:       parseIntDefault(r.FormValue("snapshot_no_today_market_count"), 0),
				MarketCandidateStatus:    r.FormValue("snapshot_market_candidate_status"),
				MarketCandidateCount:     parseIntDefault(r.FormValue("snapshot_market_candidate_count"), 0),
				AuctionAmountLabel:       r.FormValue("snapshot_auction_amount_label"),
				EmptyReason:              r.FormValue("snapshot_empty_reason"),
			})
			if err != nil {
				message = "推荐股票快照层保存失败：" + err.Error()
			} else {
				message = "推荐股票快照层已保存"
			}
		}
		target := url.Values{}
		target.Set("section", sectionKey)
		if projectID != "" {
			target.Set("project_id", projectID)
		}
		if favoritePage > 1 {
			target.Set("page", strconv.Itoa(favoritePage))
		}
		if sectionKey == "stockrepair" {
			target.Set("strategy_date", aStockRepairDate)
			target.Set("period", aStockRepairPeriod)
			if aStockRepairIgnoreRecent {
				target.Set("ignore_recent", "1")
			}
		}
		target.Set("msg", message)
		http.Redirect(w, r, "/system?"+target.Encode(), http.StatusSeeOther)
		return
	}

	notices := []model.SystemNotice{}
	feedbackItems := []model.Feedback{}
	taskRuns := []model.TaskRun{}
	auditLogs := []model.AuditLog{}
	schedulerJobs := []model.OperationSchedulerJob{}
	crawlRuns := []model.CrawlRun{}
	preferences := model.UserPreference{}
	popupState := model.PopupState{}
	mailConfig := model.MailConfig{}
	databaseConfig := model.DatabaseConfigStatus{}
	warningSetting := model.WarningSetting{}
	projects := []model.Project{}
	groups := []model.ProjectGroup{}
	favoriteArticles := model.ItemListResult{}
	warningArticles := []legacyWarningArticleCompat{}
	warningArticlePage := maxInt(favoritePage, 1)
	warningArticlePrev := 1
	warningArticleNext := 1
	warningArticleTotalPages := 1
	warningArticleProjectID := projectID
	warningArticleOpenFlag := parsePositiveInt(r.URL.Query().Get("openFlag"), 0)
	warningArticleKeyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices)
	if sectionKey == "feedbacklist" {
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/feedback?limit=20", &feedbackItems)
	}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=20", &taskRuns)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/audit-logs?limit=20", &auditLogs)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=20", &crawlRuns)
	schedulerJobs = s.collectSchedulerJobs()
	operations := model.OperationsSummary{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/operations", &operations)
	if !operations.GeneratedAt.IsZero() {
		taskRuns = operations.RecentTaskRuns
		auditLogs = operations.RecentAuditLogs
		schedulerJobs = operations.SchedulerJobs
		databaseConfig = operations.Database
	}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/project-groups", &groups)
	projectNames := make(map[int64]string, len(projects))
	for _, project := range projects {
		projectNames[project.ID] = project.Name
	}
	groupNames := make(map[int64]string, len(groups))
	for _, group := range groups {
		groupNames[group.ID] = group.Name
	}
	userID := userIDFromMap(user)
	if userID > 0 {
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/preferences?user_id="+strconv.FormatInt(userID, 10), &preferences)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/popup?user_id="+strconv.FormatInt(userID, 10)+"&key=system-announcement", &popupState)
		favoriteProjectID := strings.TrimSpace(projectID)
		if favoriteProjectID != "" || sectionKey == "favorites" {
			favoriteURL := s.cfg.ContentURL + "/api/v1/articles?favorite=favorited&page=" + strconv.Itoa(maxInt(favoritePage, 1)) + "&page_size=10&user_id=" + strconv.FormatInt(userID, 10)
			if favoriteProjectID != "" {
				favoriteURL += "&project_id=" + url.QueryEscape(favoriteProjectID)
			}
			_ = s.getJSON(favoriteURL, &favoriteArticles)
		}
		if sectionKey == "warningmsg" {
			selectedProjectID := int64(0)
			if parsedProjectID, err := strconv.ParseInt(projectID, 10, 64); err == nil {
				selectedProjectID = parsedProjectID
			}
			if data, err := s.collectLegacyWarningArticles(userID, selectedProjectID, warningArticleOpenFlag, warningArticleKeyword, warningArticlePage); err == nil {
				warningArticles = data.Articles
				warningArticlePage = data.PageNum
				warningArticleTotalPages = data.TotalPages
				warningArticlePrev = maxInt(warningArticlePage-1, 1)
				warningArticleNext = minInt(warningArticlePage+1, warningArticleTotalPages)
				warningArticleProjectID = projectID
			}
		}
	}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/mail-config", &mailConfig)
	if databaseConfig.Driver == "" {
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/database-config", &databaseConfig)
	}
	selectedProjectID := int64(0)
	if parsedProjectID, err := strconv.ParseInt(projectID, 10, 64); err == nil {
		selectedProjectID = parsedProjectID
	}
	if selectedProjectID <= 0 && len(projects) > 0 {
		selectedProjectID = projects[0].ID
	}
	if selectedProjectID > 0 {
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/warning-settings/"+strconv.FormatInt(selectedProjectID, 10), &warningSetting)
	}
	serviceLogName := ""
	serviceLogText := ""
	if logService != "" {
		serviceLogName, serviceLogText = s.loadServiceLog(logService)
		if serviceLogName == "" {
			serviceLogName = logService
			serviceLogText = "日志读取失败"
		}
	}
	if warningSetting.ProjectID == 0 {
		warningSetting.ProjectID = selectedProjectID
	}
	if favoriteArticles.Page <= 0 {
		favoriteArticles.Page = maxInt(favoritePage, 1)
	}
	if favoriteArticles.PageSize <= 0 {
		favoriteArticles.PageSize = 10
	}
	if favoriteArticles.Total < 0 {
		favoriteArticles.Total = 0
	}
	totalPages := 1
	if favoriteArticles.PageSize > 0 && favoriteArticles.Total > 0 {
		totalPages = (favoriteArticles.Total + favoriteArticles.PageSize - 1) / favoriteArticles.PageSize
	}
	if totalPages < 1 {
		totalPages = 1
	}
	currentPage := maxInt(favoriteArticles.Page, 1)
	if currentPage > totalPages {
		currentPage = totalPages
	}
	sectionLabel := map[string]string{
		"services":      "服务状态",
		"legacy":        "Legacy注册表",
		"account":       "账号安全",
		"preferences":   "偏好设置",
		"stockrepair":   "推荐股票修复",
		"favorites":     "收藏夹",
		"warningmsg":    "预警消息",
		"warning":       "预警设置",
		"feedback":      "反馈建议",
		"feedbacklist":  "建议列表",
		"operations":    "生产运行",
		"opactions":     "运营操作",
		"contracts":     "外部契约与审计",
		"announcements": "公告与任务",
		"database":      "数据库配置",
	}[sectionKey]
	aStockRepair := aStockRepairView{}
	if sectionKey == "stockrepair" {
		aStockRepair = s.loadAStockRepairView(
			r.URL.Query().Get("strategy_date"),
			r.URL.Query().Get("period"),
			normalizeAStockBool(r.URL.Query().Get("ignore_recent")),
		)
	}
	_ = s.render(w, "system", pageData{
		Title:                    "系统设置",
		User:                     user,
		SectionKey:               sectionKey,
		AStockRepair:             aStockRepair,
		Notices:                  notices,
		FeedbackItems:            feedbackItems,
		AuditLogs:                auditLogs,
		TaskRuns:                 taskRuns,
		SchedulerJobs:            schedulerJobs,
		LegacyRouteSummary:       chooseLegacyRouteSummary(operations.LegacyRegistry),
		LegacyLiveRoutes:         collectLegacyLiveRoutes(),
		CrawlRuns:                crawlRuns,
		Projects:                 projects,
		ProjectNames:             projectNames,
		GroupNames:               groupNames,
		Services:                 chooseServiceStatuses(operations.Services, s.collectServiceStatuses()),
		Operations:               operations,
		DatabaseConfig:           databaseConfig,
		Preferences:              preferences,
		PopupState:               popupState,
		MailConfig:               mailConfig,
		WarningSetting:           warningSetting,
		ServiceLogName:           serviceLogName,
		ServiceLogText:           serviceLogText,
		FavoriteItems:            favoriteArticles,
		FavoritePage:             currentPage,
		FavoritePagePrev:         maxInt(currentPage-1, 1),
		FavoritePageNext:         minInt(currentPage+1, totalPages),
		FavoriteProjectID:        projectID,
		FavoriteTotalPages:       totalPages,
		WarningArticles:          warningArticles,
		WarningArticlePage:       warningArticlePage,
		WarningArticlePrev:       warningArticlePrev,
		WarningArticleNext:       warningArticleNext,
		WarningArticleTotalPages: warningArticleTotalPages,
		WarningArticleProjectID:  warningArticleProjectID,
		WarningArticleOpenFlag:   warningArticleOpenFlag,
		WarningArticleKeyword:    warningArticleKeyword,
		CrawlTemplates:           crawlTemplates,
		Section:                  nonEmpty(sectionLabel, "系统工作台"),
		Message:                  r.URL.Query().Get("msg"),
	})
}

func (s *Server) handleSystemLogs(w http.ResponseWriter, r *http.Request, _ any) {
	serviceName := strings.TrimSpace(r.URL.Query().Get("service"))
	if serviceName == "" {
		http.Redirect(w, r, "/system", http.StatusSeeOther)
		return
	}
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	name, logText := s.loadServiceLog(serviceName)
	if name == "" {
		name = serviceName
		logText = "日志读取失败"
	}
	entries := parseServiceLogEntries(logText)
	entries = reverseLogEntries(entries)
	totalPages := 1
	if len(entries) > 0 {
		totalPages = (len(entries) + serviceLogPageSize - 1) / serviceLogPageSize
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * serviceLogPageSize
	end := minInt(start+serviceLogPageSize, len(entries))
	if start > len(entries) {
		start = len(entries)
		end = len(entries)
	}
	data := pageData{
		Title:                name + " 日志",
		ServiceLogName:       name,
		ServiceLogEntries:    entries[start:end],
		ServiceLogPage:       page,
		ServiceLogPrev:       maxInt(page-1, 1),
		ServiceLogNext:       minInt(page+1, totalPages),
		ServiceLogTotalPages: totalPages,
	}
	_ = s.render(w, "system_logs", data)
}

func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request, _ any) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data := pageData{
		Title:               "日志中心",
		ServiceLogSummaries: s.collectServiceLogSummaries(),
	}
	_ = s.render(w, "logs", data)
}

func (s *Server) requireSession(next func(http.ResponseWriter, *http.Request, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		user, err := s.getSessionUser(cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, user)
	}
}

func (s *Server) sessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return cookie.Value, true
}

func (s *Server) getSessionUser(token string) (map[string]any, error) {
	var authResp struct {
		Code int `json:"code"`
		Data struct {
			User map[string]any `json:"user"`
		} `json:"data"`
	}
	resp2, err := s.client.R().
		SetQueryParam("session_token", token).
		SetResult(&authResp).
		Get(s.cfg.AuthURL + "/api/v1/auth/session")
	if err != nil {
		return nil, err
	}
	if !resp2.IsSuccess() {
		return nil, errors.New(resp2.Status())
	}
	return authResp.Data.User, nil
}

func (s *Server) authLogin(username, password string) (struct {
	SessionToken string `json:"session_token"`
}, error) {
	var resp struct {
		Code int `json:"code"`
		Data struct {
			SessionToken string `json:"session_token"`
		} `json:"data"`
	}
	httpResp, err := s.client.R().
		SetBody(map[string]string{"username": username, "password": password}).
		SetResult(&resp).
		Post(s.cfg.AuthURL + "/api/v1/auth/login")
	if err != nil {
		return struct {
			SessionToken string `json:"session_token"`
		}{}, err
	}
	if !httpResp.IsSuccess() {
		return struct {
			SessionToken string `json:"session_token"`
		}{}, errors.New("用户名或密码错误")
	}
	return resp.Data, nil
}

func (s *Server) getJSON(url string, target any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	resp, err := s.client.R().SetResult(&envelope).Get(url)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	if len(envelope.Data) == 0 {
		return nil
	}
	return json.NewDecoder(bytes.NewReader(envelope.Data)).Decode(target)
}

func (s *Server) getJSONWithContext(ctx context.Context, url string, target any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	resp, err := s.client.R().SetContext(ctx).SetResult(&envelope).Get(url)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	if len(envelope.Data) == 0 {
		return nil
	}
	return json.NewDecoder(bytes.NewReader(envelope.Data)).Decode(target)
}

func (s *Server) loadServiceLog(serviceName string) (string, string) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		SetResult(&envelope).
		Get(s.cfg.ContentURL + "/api/v1/system/services/" + url.PathEscape(serviceName) + "/logs?lines=1000")
	if err != nil || !resp.IsSuccess() || len(envelope.Data) == 0 {
		return "", ""
	}
	var payload struct {
		Service string `json:"service"`
		Log     string `json:"log"`
	}
	if err := json.NewDecoder(bytes.NewReader(envelope.Data)).Decode(&payload); err != nil {
		return "", ""
	}
	return payload.Service, payload.Log
}

func (s *Server) collectServiceLogSummaries() []serviceLogSummary {
	services := []string{
		"gateway-web",
		"auth-service",
		"wechat-service",
		"content-service",
		"crawler-service",
		"analysis-service",
		"nlp-service",
		"scheduler-service",
	}
	summaries := make([]serviceLogSummary, 0, len(services))
	for _, service := range services {
		name, logText := s.loadServiceLog(service)
		if name == "" {
			name = service
		}
		entries := reverseLogEntries(parseServiceLogEntries(logText))
		message := ""
		if len(entries) == 0 {
			message = "暂无日志或读取失败"
		}
		if len(entries) > 8 {
			entries = entries[:8]
		}
		summaries = append(summaries, serviceLogSummary{
			Service: name,
			Entries: entries,
			Message: message,
		})
	}
	return summaries
}

const serviceLogPageSize = 50

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

type serviceLogEntry struct {
	Time    string
	Level   string
	Message string
}

func parseServiceLogEntries(text string) []serviceLogEntry {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	entries := make([]serviceLogEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entry, ok := parseServiceLogLine(line)
		if ok {
			entries = append(entries, entry)
			continue
		}
		entries = append(entries, serviceLogEntry{Message: stripANSI(line)})
	}
	return entries
}

func parseServiceLogLine(line string) (serviceLogEntry, bool) {
	line = stripANSI(line)
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return serviceLogEntry{Message: line}, false
	}
	if isLogLevel(fields[1]) && strings.Contains(fields[0], "T") {
		return serviceLogEntry{
			Time:    fields[0],
			Level:   fields[1],
			Message: strings.TrimSpace(strings.TrimPrefix(line, fields[0]+" "+fields[1])),
		}, true
	}
	if len(fields) >= 3 && isLogLevel(fields[2]) && strings.Contains(fields[0], "/") {
		prefix := fields[0] + " " + fields[1] + " " + fields[2]
		return serviceLogEntry{
			Time:    fields[0] + " " + fields[1],
			Level:   fields[2],
			Message: strings.TrimSpace(strings.TrimPrefix(line, prefix)),
		}, true
	}
	return serviceLogEntry{Message: line}, false
}

func stripANSI(value string) string {
	return strings.TrimSpace(ansiEscapePattern.ReplaceAllString(value, ""))
}

func isLogLevel(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DBG", "DEBUG", "INF", "INFO", "WRN", "WARN", "ERR", "ERROR", "FATAL", "PANIC":
		return true
	default:
		return false
	}
}

func reverseLogEntries(entries []serviceLogEntry) []serviceLogEntry {
	result := append([]serviceLogEntry(nil), entries...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func (s *Server) render(w http.ResponseWriter, name string, data pageData) error {
	data = withPortalFooterData(data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return s.templates.ExecuteTemplate(w, name, data)
}

func withPortalFooterData(data pageData) pageData {
	if strings.TrimSpace(data.FooterCommit) == "" {
		data.FooterCommit = app.GitCommit
	}
	if strings.TrimSpace(data.FooterBuildTime) == "" {
		data.FooterBuildTime = app.BuildTime
	}
	data.FooterBuildTime = formatFooterBuildTime(data.FooterBuildTime)
	if strings.TrimSpace(data.FooterBranch) == "" {
		data.FooterBranch = app.BranchName
	}
	return data
}

func renderPortalFooter(data pageData) string {
	data = withPortalFooterData(data)
	var buf bytes.Buffer
	if err := template.Must(template.New("portal_footer").Parse(portalFooterHTML)).Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}

func formatFooterBuildTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "unknown") {
		return raw
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, parseErr := time.Parse(layout, raw); parseErr == nil {
			return ts.In(loc).Format("2006-01-02 15:04:05 UTC+8")
		}
	}
	return raw
}

func formatShanghaiTime(ts time.Time) string {
	if ts.IsZero() {
		return "--"
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	return ts.In(loc).Format("2006-01-02 15:04")
}

func formatArticleCaptureTime(item model.Item) string {
	if item.CapturedAt.IsZero() {
		return "--"
	}
	return formatShanghaiTime(item.CapturedAt)
}

func normalizeArticleListSort(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "publish_time_desc", "published_at_desc":
		return "publish_time_desc"
	default:
		return "captured_at_desc"
	}
}

func articleListTimeField(sort string) string {
	if normalizeArticleListSort(sort) == "publish_time_desc" {
		return "publish_time"
	}
	return "captured_at"
}

func articleListTimeLabel(sort string) string {
	if normalizeArticleListSort(sort) == "publish_time_desc" {
		return "发布时间"
	}
	return "同步时间"
}

func formatArticleListTime(item model.Item, sort string) string {
	if normalizeArticleListSort(sort) == "publish_time_desc" {
		return formatArticlePublishTime(item)
	}
	return formatArticleCaptureTime(item)
}

func formatArticlePublishTime(item model.Item) string {
	for _, value := range []string{item.PublishTime, item.PublishTimeText} {
		if formatted, ok := formatArticleTimeText(value, item.CapturedAt); ok {
			return formatted
		}
	}
	return formatArticleCaptureTime(item)
}

func formatArticleTimeText(raw string, capturedAt time.Time) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.In(loc).Format("2006-01-02 15:04"), true
		}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/1/2 15:04:05",
		"2006/1/2 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	} {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			return parsed.In(loc).Format("2006-01-02 15:04"), true
		}
	}
	if formatted, ok := formatRelativeArticleTime(value, capturedAt, loc); ok {
		return formatted, true
	}
	return value, true
}

func formatRelativeArticleTime(value string, capturedAt time.Time, loc *time.Location) (string, bool) {
	if capturedAt.IsZero() {
		return "", false
	}
	base := capturedAt.In(loc)
	switch value {
	case "刚刚", "刚才", "现在":
		return base.Format("2006-01-02 15:04"), true
	}
	if minutes, ok := parseChineseRelativeNumber(value, "分钟前"); ok {
		return base.Add(-time.Duration(minutes) * time.Minute).Format("2006-01-02 15:04"), true
	}
	if hours, ok := parseChineseRelativeNumber(value, "小时前"); ok {
		return base.Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04"), true
	}
	return "", false
}

func parseChineseRelativeNumber(value, suffix string) (int, bool) {
	if !strings.HasSuffix(value, suffix) {
		return 0, false
	}
	raw := strings.TrimSpace(strings.TrimSuffix(value, suffix))
	if raw == "" {
		return 0, false
	}
	number, err := strconv.Atoi(raw)
	if err != nil || number < 0 {
		return 0, false
	}
	return number, true
}

func userIDFromMap(user any) int64 {
	mapped, ok := user.(map[string]any)
	if !ok {
		return 0
	}
	switch value := mapped["id"].(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	default:
		return 0
	}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func articleSourceSiteLabel(item model.Item) string {
	switch strings.ToLower(strings.TrimSpace(item.SourceType)) {
	case "flash", "headline", "jin10_full":
		return "金十"
	case "eastmoney_kuaixun":
		return "东方财富网"
	case "wallstreetcn_a_stock":
		return "华尔街见闻"
	case "cls_telegraph":
		return "财联社"
	case "sina_finance_7x24":
		return "新浪财经"
	case "panews_newsflash":
		return "PANews"
	case "coindesk_zh_latest":
		return "CoinDesk"
	case "foresight_newsflash":
		return "Foresight"
	case "theblock_latest":
		return "The Block"
	}
	sourceBlob := strings.ToLower(strings.Join([]string{item.FromText, item.ExternalSourceHost, item.SourceURL, item.DetailURL}, " "))
	switch {
	case strings.Contains(sourceBlob, "panews") || strings.Contains(sourceBlob, "panewslab"):
		return "PANews"
	case strings.Contains(sourceBlob, "coindesk"):
		return "CoinDesk"
	case strings.Contains(sourceBlob, "theblock") || strings.Contains(sourceBlob, "the block"):
		return "The Block"
	case strings.Contains(sourceBlob, "foresight"):
		return "Foresight"
	case strings.Contains(sourceBlob, "jin10") || strings.Contains(sourceBlob, "金十"):
		return "金十"
	case strings.Contains(sourceBlob, "eastmoney") || strings.Contains(sourceBlob, "东方财富"):
		return "东方财富网"
	default:
		return nonEmpty(item.FromText, item.ExternalSourceHost, item.SourceType, "未知来源")
	}
}

func articleBodyText(item model.Item) string {
	title := strings.TrimSpace(item.Title)
	for _, value := range []string{item.Content, item.Summary} {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if title != "" && strings.EqualFold(text, title) {
			continue
		}
		return text
	}
	return ""
}

func (s *Server) loadCrawlTemplates() []model.CrawlTemplate {
	var templates []model.CrawlTemplate
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/crawl-templates", &templates); err != nil {
		return nil
	}
	filtered := make([]model.CrawlTemplate, 0, len(templates))
	for _, tpl := range templates {
		if tpl.ID <= 0 || !tpl.Enabled {
			continue
		}
		filtered = append(filtered, tpl)
	}
	return filtered
}

func validManualSourceType(value string) string {
	return provider.ValidSourceType(value)
}

func applyManualCrawlSourceType(req *resty.Request, value string) bool {
	rawValue := strings.TrimSpace(value)
	sourceType := validManualSourceType(rawValue)
	if rawValue != "" && sourceType == "" {
		return false
	}
	if sourceType != "" {
		req.SetQueryParam("source_type", sourceType)
	}
	return true
}

func applyManualCrawlTemplate(req *resty.Request, value string) bool {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return true
	}
	templateID, err := strconv.ParseInt(rawValue, 10, 64)
	if err != nil || templateID <= 0 {
		return false
	}
	req.SetQueryParam("template_id", strconv.FormatInt(templateID, 10))
	return true
}

func normalizeSystemSection(section string) string {
	switch strings.TrimSpace(section) {
	case "services", "service", "status":
		return "services"
	case "legacy", "legacyregistry", "legacy_registry", "legacy-routes":
		return "legacy"
	case "preferences", "preference":
		return "preferences"
	case "stockrepair", "stock_repair", "stock-repair", "astockrepair", "a-stock-repair":
		return "stockrepair"
	case "favorites", "favorite":
		return "favorites"
	case "warningmsg", "warningmessage":
		return "warningmsg"
	case "warning", "warningedit":
		return "warning"
	case "feedback":
		return "feedback"
	case "feedbacklist", "feedback_list", "feedback-list", "suggestions":
		return "feedbacklist"
	case "operations", "production":
		return "operations"
	case "opactions", "operation-actions", "operation_actions", "actions", "crawl-actions":
		return "opactions"
	case "contracts", "contract", "audit", "external", "external-contracts":
		return "contracts"
	case "announcements", "announcement", "notice", "notices", "tasks":
		return "announcements"
	case "database", "postgres", "postgresql":
		return "database"
	default:
		return "account"
	}
}

func parsePositiveInt(raw string, fallback int) int {
	if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

func parseIntDefault(raw string, fallback int) int {
	if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return parsed
	}
	return fallback
}

func maxInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	return max
}

func minInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}

func buildPageURL(r *http.Request, page int) string {
	query := r.URL.Query()
	query.Set("page", strconv.Itoa(maxInt(page, 1)))
	return r.URL.Path + "?" + query.Encode()
}

func parseProjectID(raw string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return id
}

func parseFormProjectID(r *http.Request) int64 {
	return parseProjectID(nonEmpty(r.FormValue("project_id"), r.FormValue("projectId"), r.FormValue("projectid")))
}

func parseFormProjectGroupID(r *http.Request) int64 {
	return parseProjectID(nonEmpty(r.FormValue("group_id"), r.FormValue("groupId"), r.FormValue("groupid")))
}

func publicOptionIDs(raw string) []int64 {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	ids := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

type legacyGroupCompat struct {
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
}

type legacyProjectCompat struct {
	GroupID     int64  `json:"groupId"`
	GroupName   string `json:"groupName"`
	ProjectID   int64  `json:"projectId"`
	ProjectName string `json:"projectName"`
}

type legacyWarningArticleCompat struct {
	ArticleID     string `json:"article_id"`
	ArticleTitle  string `json:"article_title"`
	ArticleTime   string `json:"article_time"`
	ArticleDetail string `json:"article_detail"`
	GroupID       string `json:"group_id"`
	ProjectID     string `json:"project_id"`
	GroupName     string `json:"groupName"`
	ProjectName   string `json:"project_name"`
}

func (s *Server) fetchLegacyProjectGroups() []legacyGroupCompat {
	var groups []model.ProjectGroup
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/project-groups", &groups); err != nil {
		return []legacyGroupCompat{}
	}
	result := make([]legacyGroupCompat, 0, len(groups))
	for _, group := range groups {
		result = append(result, legacyGroupCompat{
			GroupID:   group.ID,
			GroupName: group.Name,
		})
	}
	return result
}

func (s *Server) fetchLegacyProjects() []legacyProjectCompat {
	var projects []model.Project
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects); err != nil {
		return []legacyProjectCompat{}
	}
	result := make([]legacyProjectCompat, 0, len(projects))
	for _, project := range projects {
		result = append(result, legacyProjectCompat{
			GroupID:     project.GroupID,
			GroupName:   project.GroupName,
			ProjectID:   project.ID,
			ProjectName: project.Name,
		})
	}
	return result
}

func (s *Server) fetchLegacyProjectMap() map[int64]model.Project {
	var projects []model.Project
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects); err != nil {
		return map[int64]model.Project{}
	}
	result := make(map[int64]model.Project, len(projects))
	for _, project := range projects {
		result[project.ID] = project
	}
	return result
}

func (s *Server) fetchLegacyGroupNameMap() map[int64]string {
	var groups []model.ProjectGroup
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/project-groups", &groups); err != nil {
		return map[int64]string{}
	}
	result := make(map[int64]string, len(groups))
	for _, group := range groups {
		result[group.ID] = group.Name
	}
	return result
}

func filterLegacyProjectsByGroupID(projects []legacyProjectCompat, groupID int64) []legacyProjectCompat {
	if groupID <= 0 {
		return projects
	}
	filtered := make([]legacyProjectCompat, 0, len(projects))
	for _, project := range projects {
		if project.GroupID == groupID {
			filtered = append(filtered, project)
		}
	}
	return filtered
}

func legacyWarningListEntry(project legacyProjectCompat, setting model.WarningSetting) map[string]any {
	source := setting.WarningSource
	if strings.TrimSpace(source) == "" {
		source = legacyJSONString(map[string]any{"type": 1, "email": setting.Recipients})
	}
	interval := setting.WarningInterval
	if strings.TrimSpace(interval) == "" {
		interval = legacyJSONString(map[string]any{"type": 1, "time": strconv.Itoa(max(setting.Threshold, 1))})
	}
	warningStatus := setting.WarningStatus
	if warningStatus == 0 && setting.Enabled {
		warningStatus = 1
	}
	warningMatch := setting.WarningMatch
	if warningMatch == 0 {
		warningMatch = 1
	}
	return map[string]any{
		"project_id":            project.ProjectID,
		"project_name":          project.ProjectName,
		"group_name":            project.GroupName,
		"warning_setting_id":    setting.WarningSettingID,
		"warning_status":        warningStatus,
		"warning_name":          nonEmpty(setting.WarningName, setting.Description, project.ProjectName, "预警"),
		"warning_word":          setting.WarningWord,
		"warning_classify":      nonEmpty(setting.WarningClassify, setting.Channels),
		"warning_content":       setting.WarningContent,
		"warning_similar":       setting.WarningSimilar,
		"warning_match":         warningMatch,
		"warning_deduplication": setting.WarningDeduplication,
		"warning_source":        source,
		"warning_receive_time":  nonEmpty(setting.WarningReceiveTime, legacyJSONString(map[string]any{"start": "", "end": ""})),
		"weekend_warning":       setting.WeekendWarning,
		"warning_interval":      interval,
	}
}

func legacyProjectPublicID(item model.Item) string {
	if strings.TrimSpace(item.SourceKey) != "" {
		return item.SourceKey
	}
	return strconv.FormatInt(item.ID, 10)
}

func legacyArticlePublicID(item model.Item) string {
	return legacyProjectPublicID(item)
}

func legacyEmotionalIndex(item model.Item) int {
	raw := strings.TrimSpace(item.TagFlags)
	switch {
	case strings.Contains(raw, "负") || strings.Contains(raw, "3"):
		return 3
	case strings.Contains(raw, "正") || strings.Contains(raw, "1"):
		return 1
	default:
		return 2
	}
}

func legacyPublishTime(item model.Item) string {
	if strings.TrimSpace(item.PublishTime) != "" {
		return strings.TrimSpace(item.PublishTime)
	}
	if !item.CapturedAt.IsZero() {
		return item.CapturedAt.UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func apiutilIntQuery(r *http.Request, key string, fallback int) int {
	if r == nil {
		return fallback
	}
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	if parsed, err := strconv.Atoi(raw); err == nil {
		return parsed
	}
	return fallback
}

func containsAny(text string, words []string) bool {
	text = strings.ToLower(text)
	for _, word := range words {
		if word = strings.TrimSpace(strings.ToLower(word)); word != "" && strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func summarizeText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= 140 {
		return text
	}
	return string(runes[:140]) + "..."
}

func firstLegacyProjectAndGroup(item model.Item, projectMap map[int64]model.Project) (int64, int64) {
	for _, projectID := range item.ProjectIDs {
		project, ok := projectMap[projectID]
		if !ok {
			continue
		}
		return project.ID, project.GroupID
	}
	return 0, 0
}

func firstProjectIDForItemTemplate(item model.Item, projects []model.Project) int64 {
	projectMap := make(map[int64]model.Project, len(projects))
	for _, project := range projects {
		projectMap[project.ID] = project
	}
	projectID, _ := firstLegacyProjectAndGroup(item, projectMap)
	return projectID
}

func firstProjectGroupIDForTemplate(item model.Item, projects []model.Project) int64 {
	projectMap := make(map[int64]model.Project, len(projects))
	for _, project := range projects {
		projectMap[project.ID] = project
	}
	_, groupID := firstLegacyProjectAndGroup(item, projectMap)
	return groupID
}

func legacyIntFromAnyValue(value any) int {
	v, ok := legacyIntFromAny(value)
	if !ok {
		return 0
	}
	return v
}

func legacyWarningSettingPayload(setting model.WarningSetting, projectID int64, projectName string) map[string]any {
	source := setting.WarningSource
	if strings.TrimSpace(source) == "" {
		source = legacyJSONString(map[string]any{"type": 1, "email": setting.Recipients})
	}
	interval := setting.WarningInterval
	if strings.TrimSpace(interval) == "" {
		interval = legacyJSONString(map[string]any{"type": 1, "time": strconv.Itoa(max(setting.Threshold, 1))})
	}
	receiveTime := setting.WarningReceiveTime
	if strings.TrimSpace(receiveTime) == "" {
		receiveTime = legacyJSONString(map[string]any{"start": "", "end": ""})
	}
	warningStatus := setting.WarningStatus
	if warningStatus == 0 && setting.Enabled {
		warningStatus = 1
	}
	warningMatch := setting.WarningMatch
	if warningMatch == 0 {
		warningMatch = 1
	}
	return map[string]any{
		"project_id":            projectID,
		"project_name":          projectName,
		"warning_setting_id":    setting.WarningSettingID,
		"warning_status":        warningStatus,
		"warning_name":          nonEmpty(setting.WarningName, setting.Description, projectName),
		"warning_word":          setting.WarningWord,
		"warning_classify":      nonEmpty(setting.WarningClassify, setting.Channels),
		"warning_content":       setting.WarningContent,
		"warning_similar":       setting.WarningSimilar,
		"warning_match":         warningMatch,
		"warning_deduplication": setting.WarningDeduplication,
		"warning_source":        source,
		"warning_receive_time":  receiveTime,
		"weekend_warning":       setting.WeekendWarning,
		"warning_interval":      interval,
	}
}

func legacyWarningSettingFromForm(r *http.Request) model.WarningSetting {
	warningSource := nonEmpty(r.FormValue("warning_source"))
	warningInterval := nonEmpty(r.FormValue("warning_interval"))
	if warningSource == "" {
		warningSource = legacyJSONString(map[string]any{"type": 1, "email": r.FormValue("recipients")})
	}
	if warningInterval == "" {
		warningInterval = legacyJSONString(map[string]any{"type": 1, "time": nonEmpty(r.FormValue("threshold"), "1")})
	}
	channels := nonEmpty(r.FormValue("channels"), r.FormValue("warning_classify"))
	warningWord := nonEmpty(r.FormValue("warning_word"))
	warningName := nonEmpty(r.FormValue("warning_name"), warningWord, "预警")
	receiveTime := nonEmpty(r.FormValue("warning_receive_time"))
	if receiveTime == "" {
		receiveTime = legacyJSONString(map[string]any{"start": "", "end": ""})
	}
	threshold := parseWarningThreshold(r.FormValue("threshold"), warningInterval)
	if threshold <= 0 {
		threshold = 1
	}
	return model.WarningSetting{
		WarningStatus:        parseLegacyWarningStatus(r.FormValue("warning_status"), r.FormValue("enabled")),
		WarningName:          warningName,
		WarningWord:          warningWord,
		WarningClassify:      channels,
		WarningContent:       parseLegacyInt(r.FormValue("warning_content"), 0),
		WarningSimilar:       parseLegacyInt(r.FormValue("warning_similar"), 0),
		WarningMatch:         parseLegacyInt(r.FormValue("warning_match"), 1),
		WarningDeduplication: parseLegacyInt(r.FormValue("warning_deduplication"), 0),
		WarningSource:        warningSource,
		WarningReceiveTime:   receiveTime,
		WeekendWarning:       parseLegacyInt(r.FormValue("weekend_warning"), 0),
		WarningInterval:      warningInterval,
		Enabled:              parseBoolLike(r.FormValue("enabled"), r.FormValue("warning_status")),
		Channels:             channels,
		Threshold:            threshold,
		Recipients:           extractWarningEmail(warningSource),
		Description:          warningName,
	}
}

func (s *Server) fetchLegacyWarningSetting(projectID int64) (model.WarningSetting, string) {
	var setting model.WarningSetting
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/system/warning-settings/"+strconv.FormatInt(projectID, 10), &setting); err != nil {
		setting.ProjectID = projectID
	}
	var project model.Project
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+strconv.FormatInt(projectID, 10), &project)
	return setting, nonEmpty(project.Name, setting.WarningName, setting.Description)
}

func legacyJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func boolToLegacyInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseBoolLike(values ...string) bool {
	for _, raw := range values {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "on", "enabled", "open":
			return true
		case "0", "false", "no", "off", "disabled", "close":
			return false
		}
	}
	return false
}

func parseLegacyWarningStatus(values ...string) int {
	for _, raw := range values {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "on", "enabled", "open":
			return 1
		case "0", "false", "no", "off", "disabled", "close":
			return 0
		}
	}
	return 0
}

func parseLegacyInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if parsed, err := strconv.Atoi(raw); err == nil {
		return parsed
	}
	return fallback
}

func parseWarningThreshold(rawValues ...string) int {
	for _, raw := range rawValues {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if parsed, err := strconv.Atoi(raw); err == nil {
			return parsed
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			if value := strings.TrimSpace(legacyStringFromAny(payload["time"])); value != "" {
				if parsed, err := strconv.Atoi(value); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func extractWarningEmail(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	return legacyStringFromAny(payload["email"])
}

func decodeLegacyOpinionCondition(r *http.Request) (model.OpinionCondition, error) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return model.OpinionCondition{}, err
	}
	emotion := legacyJSONString(raw["emotion"])
	if emotion == "null" || emotion == "\"\"" {
		emotion = "[1,2,3]"
	}
	condition := model.OpinionCondition{
		Time:            parseLegacyInt(legacyStringFromAny(raw["time"]), 4),
		Precise:         parseLegacyInt(legacyStringFromAny(raw["precise"]), 0),
		Emotion:         emotion,
		Similar:         parseLegacyInt(legacyStringFromAny(raw["similar"]), 0),
		Sort:            parseLegacyInt(legacyStringFromAny(raw["sort"]), 1),
		Matchs:          parseLegacyInt(legacyStringFromAny(raw["matchs"]), 1),
		Times:           legacyStringFromAny(raw["times"]),
		Timee:           legacyStringFromAny(raw["timee"]),
		Classify:        legacyStringFromAny(raw["classify"]),
		Websitename:     legacyStringFromAny(raw["websitename"]),
		Author:          legacyStringFromAny(raw["author"]),
		Organization:    legacyStringFromAny(raw["organization"]),
		Categorylable:   legacyStringFromAny(raw["categorylable"]),
		Enterprisetype:  legacyStringFromAny(raw["enterprisetype"]),
		Hightechtype:    legacyStringFromAny(raw["hightechtype"]),
		Policylableflag: legacyStringFromAny(raw["policylableflag"]),
		DatasourceType:  legacyStringFromAny(raw["datasource_type"]),
		EventIndex:      legacyStringFromAny(raw["eventIndex"]),
		IndustryIndex:   legacyStringFromAny(raw["industryIndex"]),
		Province:        legacyStringFromAny(raw["province"]),
		City:            legacyStringFromAny(raw["city"]),
	}
	if id, ok := legacyIntFromAny(raw["project_id"]); ok {
		condition.ProjectID = int64(id)
	}
	if id, ok := legacyIntFromAny(raw["opinion_condition_id"]); ok {
		condition.OpinionConditionID = int64(id)
	}
	if createTime := legacyStringFromAny(raw["create_time"]); createTime != "" {
		condition.CreateTime = createTime
	}
	return condition, nil
}
func firstProjectID(contentURL string, client *resty.Client) int64 {
	var projects []model.Project
	resp, err := client.R().SetResult(&struct {
		Data json.RawMessage `json:"data"`
	}{}).Get(contentURL + "/api/v1/projects")
	if err != nil || resp == nil || !resp.IsSuccess() {
		return 0
	}
	var envelope struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil || len(envelope.Data) == 0 {
		return 0
	}
	if err := json.Unmarshal(envelope.Data, &projects); err != nil || len(projects) == 0 {
		return 0
	}
	return projects[0].ID
}

func appendMessage(rawURL, message string) string {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(message) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set("msg", message)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func defaultRuleName(includeKeywords, channels string) string {
	if includeKeywords = strings.TrimSpace(includeKeywords); includeKeywords != "" {
		return "关键词监测：" + includeKeywords
	}
	if channels = strings.TrimSpace(channels); channels != "" {
		return "来源监测：" + channels
	}
	return ""
}

func defaultRuleProjectName(projectName, ruleName, includeKeywords string) string {
	if projectName = strings.TrimSpace(projectName); projectName != "" {
		return projectName
	}
	if includeKeywords = strings.TrimSpace(includeKeywords); includeKeywords != "" {
		return "监测项目：" + includeKeywords
	}
	if ruleName = strings.TrimSpace(ruleName); ruleName != "" {
		return "监测项目：" + ruleName
	}
	return "默认监测项目"
}

func (s *Server) createProjectForRule(name, keywords string) (model.Project, error) {
	resp, err := s.client.R().SetBody(map[string]any{
		"name":        strings.TrimSpace(name),
		"keywords":    strings.TrimSpace(keywords),
		"description": "由监测规则页面自动创建",
		"status":      "active",
	}).Post(s.cfg.ContentURL + "/api/v1/projects")
	if err != nil || !resp.IsSuccess() {
		return model.Project{}, errors.New(responseErrorMessage(resp, err))
	}
	var envelope struct {
		Data model.Project `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return model.Project{}, err
	}
	if envelope.Data.ID <= 0 {
		return model.Project{}, errors.New("content service returned empty project id")
	}
	return envelope.Data, nil
}

func responseErrorMessage(resp *resty.Response, err error) string {
	if err != nil {
		return err.Error()
	}
	if resp == nil {
		return "请求未返回响应"
	}
	var envelope struct {
		Message string `json:"message"`
	}
	if len(resp.Body()) > 0 && json.Unmarshal(resp.Body(), &envelope) == nil && strings.TrimSpace(envelope.Message) != "" {
		return strings.TrimSpace(envelope.Message)
	}
	return resp.Status()
}

func localRedirectTarget(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "/") {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.HasPrefix(parsed.Path, "/") {
		return ""
	}
	target := parsed.Path
	if parsed.RawQuery != "" {
		target += "?" + parsed.RawQuery
	}
	return target
}

func (s *Server) collectServiceStatuses() []serviceStatus {
	services := []serviceStatus{
		{Name: "gateway-web", URL: strings.TrimRight(s.cfg.GatewayWebURL, "/") + "/healthy"},
		{Name: "auth-service", URL: strings.TrimRight(s.cfg.AuthURL, "/") + "/healthy"},
		{Name: "content-service", URL: strings.TrimRight(s.cfg.ContentURL, "/") + "/healthy"},
		{Name: "crawler-service", URL: strings.TrimRight(s.cfg.CrawlerURL, "/") + "/healthy"},
		{Name: "analysis-service", URL: strings.TrimRight(s.cfg.AnalysisURL, "/") + "/healthy"},
		{Name: "nlp-service", URL: strings.TrimRight(s.cfg.NLPURL, "/") + "/healthy"},
		{Name: "scheduler-service", URL: strings.TrimRight(s.cfg.SchedulerURL, "/") + "/healthy"},
	}
	for idx := range services {
		if strings.TrimSpace(services[idx].URL) == "/healthy" {
			continue
		}
		resp, err := s.client.R().Get(services[idx].URL)
		if err != nil {
			services[idx].Status = "failed"
			services[idx].Message = err.Error()
			continue
		}
		fallbackMessage := resp.Status()
		if resp.IsSuccess() {
			fallbackMessage = "ok"
		}
		health := apiutil.CoerceHealthPayload(resp.IsSuccess(), services[idx].Name, resp.Body(), fallbackMessage)
		services[idx].Status = health.Status
		services[idx].Healthy = health.Healthy
		services[idx].Message = health.Message
	}
	return services
}

func (s *Server) collectSchedulerJobs() []model.OperationSchedulerJob {
	jobs := []model.OperationSchedulerJob{}
	_ = s.getJSON(s.cfg.SchedulerURL+"/api/v1/scheduler/jobs", &jobs)
	return jobs
}

func collectLegacyRouteSummary() []legacyRouteSummary {
	counts := map[string]int{}
	for _, spec := range portalLegacyRoutes {
		counts[string(spec.Strategy)]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]legacyRouteSummary, 0, len(keys))
	for _, key := range keys {
		result = append(result, legacyRouteSummary{Strategy: key, Count: counts[key]})
	}
	return result
}

func chooseLegacyRouteSummary(registry []model.LegacyStrategyCount) []legacyRouteSummary {
	if len(registry) == 0 {
		return collectLegacyRouteSummary()
	}
	result := make([]legacyRouteSummary, 0, len(registry))
	for _, item := range registry {
		result = append(result, legacyRouteSummary{Strategy: item.Strategy, Count: item.Count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Strategy < result[j].Strategy })
	return result
}

func chooseServiceStatuses(ops []model.OperationServiceStatus, fallback []serviceStatus) []serviceStatus {
	if len(ops) == 0 {
		return fallback
	}
	result := make([]serviceStatus, 0, len(ops))
	for _, item := range ops {
		result = append(result, serviceStatus{
			Name:    item.Name,
			URL:     item.URL,
			Status:  item.Status,
			Healthy: item.Healthy,
			Message: item.Message,
		})
	}
	return result
}

func collectLegacyLiveRoutes() []legacyRouteSpec {
	result := make([]legacyRouteSpec, 0)
	for _, spec := range portalLegacyRoutes {
		if spec.Strategy == legacyStrategyProxy || spec.Strategy == legacyStrategyPreserve {
			result = append(result, spec)
		}
	}
	return result
}

const portalNavHTML = `<nav><a href="/">总览</a><a href="/projects">项目</a><a href="/monitor-rules">规则</a><a href="/articles">文章</a><a href="/reports">报告</a><a href="/crawl-templates">模板中心</a><a href="/crawl-templates/manage">模板管理</a><a href="/a-stock">A股</a><a href="/stock-research">研报调研</a><a href="/investor-relations">投资者关系</a><a href="/a-stock/holdings">机构持仓</a><a href="/a-stock/auction">集合竞价</a><a href="/crypto">Crypto</a><a href="/system">系统</a><a href="/logs">日志</a><a class="logout-link" href="/logout">退出</a></nav>`
const portalFooterHTML = `<footer class="site-footer"><div>Code By Yuhao@jiansutech.com - {{.FooterBuildTime}} - {{.FooterCommit}} - {{.FooterBranch}} - <a class="footer-feedback-link" href="/system?section=feedback">反馈建议</a></div></footer>` + portalRulesFormEnhancementScript + portalSystemServiceLogsScript

const portalRulesFormEnhancementScript = `<script>(function(){if(location.pathname!=="/monitor-rules"){return}var headings=[].slice.call(document.querySelectorAll("h2"));var heading=headings.find(function(node){return node.textContent.trim()==="新建规则"});if(!heading){return}var section=heading.closest("section");var form=section&&section.querySelector("form");if(!form){return}var project=form.querySelector('select[name="project_id"]');if(project&&project.options.length===0){var option=document.createElement("option");option.value="";option.textContent="无可选项目，提交时自动创建项目";project.appendChild(option)}if(project&&!form.querySelector('input[name="project_name"]')){var input=document.createElement("input");input.name="project_name";input.placeholder="新项目名称（可选，未选择项目时使用）";project.insertAdjacentElement("afterend",input)}var channels=form.querySelector('input[name="channels"]');if(channels){channels.setAttribute("list","monitor-rule-channel-options");if(!document.getElementById("monitor-rule-channel-options")){var list=document.createElement("datalist");list.id="monitor-rule-channel-options";["flash","headline","crypto_x","crypto_telegram","flash,headline","all"].forEach(function(value){var option=document.createElement("option");option.value=value;list.appendChild(option)});document.body.appendChild(list)}}var name=form.querySelector('input[name="name"]');var include=form.querySelector('input[name="include_keywords"]');form.addEventListener("submit",function(){if(name&&include&&!name.value.trim()&&include.value.trim()){name.value="关键词监测："+include.value.trim()}})})();</script>`

const portalSystemServiceLogsScript = ``

const layoutTemplate = `
{{define "nav"}}` + portalNavHTML + `{{end}}
{{define "footer"}}` + portalFooterHTML + `{{end}}
`

const baseStyles = `body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222;padding-top:52px}header,main,.site-footer{max-width:1180px;margin:0 auto;padding:24px}header{padding-bottom:0}header h1{margin:0 0 18px;font-size:30px;line-height:1.25}nav{display:flex;gap:16px;flex-wrap:wrap;align-items:center}nav a{margin-right:0;color:#214e34;text-decoration:none;font-weight:600;display:inline-flex;align-items:center;min-height:24px;line-height:1.2}.logout-link{position:fixed;top:14px;right:18px;z-index:1000;padding:8px 14px;border-radius:8px;background:#214e34;color:#fff!important;box-shadow:0 8px 20px rgba(0,0,0,.12)}.site-footer{color:#6a6257;font-size:13px;padding-top:12px;padding-bottom:28px;text-align:center}.site-footer a{color:#214e34;font-weight:600;text-decoration:none}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}input,select,textarea,button{width:100%;padding:12px;margin:8px 0;border-radius:10px;border:1px solid #d0c8b8;box-sizing:border-box}button{background:#214e34;color:#fff;border:none;cursor:pointer}table{width:100%;border-collapse:collapse}th,td{padding:10px;border-bottom:1px solid #ece7dc;text-align:left}pre{white-space:pre-wrap;line-height:1.6}a.inline{margin-right:0;color:#214e34}.muted{color:#6a6257}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:16px}.dashboard-grid{display:grid;grid-template-columns:1.05fr 1.65fr 1.1fr;gap:18px;align-items:start}.dashboard-col{display:grid;gap:16px}.section-card{border:1px solid #ece7dc;border-radius:14px;background:#faf8f2;padding:16px}.topic-list{list-style:none;padding:0;margin:0}.topic-list li{padding:12px 0;border-bottom:1px solid #ece7dc}.topic-list li:last-child{border-bottom:none}.topic-head{display:flex;justify-content:space-between;gap:12px;align-items:baseline}.metric-row{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px}.summary-strip{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;margin-bottom:12px}.summary-strip>div{padding:12px 14px;border-radius:12px;border:1px solid #ece7dc;background:#fff}.summary-strip strong{display:block;margin-bottom:4px;font-size:16px;color:#214e34}.summary-strip .muted{font-size:13px}.kpi-chip{display:inline-flex;align-items:center;gap:8px;padding:6px 10px;border-radius:999px;background:rgba(33,78,52,.08);color:#214e34;font-size:13px}.hero{display:flex;justify-content:space-between;gap:16px;align-items:flex-start;background:linear-gradient(135deg,#214e34 0%,#315d42 100%);color:#fff}.hero h1,.hero p{margin:0}.hero p{opacity:.9}.hero-meta{display:flex;flex-direction:column;gap:10px;align-items:flex-end;font-size:14px}.hero-meta a{color:#fff;text-decoration:underline}.progress{height:10px;background:rgba(33,78,52,.1);border-radius:999px;overflow:hidden;margin-top:8px}.progress i{display:block;height:100%;background:linear-gradient(90deg,#214e34,#5b8b69)}.section-card h3{margin-top:0}@media (max-width: 1100px){.dashboard-grid{grid-template-columns:1fr}.hero{flex-direction:column}.hero-meta{align-items:flex-start}.site-footer{padding-top:8px}}`

const portalSourceOptions = `<option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option><option value="jin10_full">jin10_full</option><option value="eastmoney_kuaixun">eastmoney_kuaixun</option><option value="wallstreetcn_a_stock">wallstreetcn_a_stock</option><option value="cls_telegraph">cls_telegraph</option><option value="sina_finance_7x24">sina_finance_7x24</option><option value="crypto_x">crypto_x</option><option value="crypto_telegram">crypto_telegram</option><option value="foresight_newsflash">foresight_newsflash</option><option value="coindesk_zh_latest">coindesk_zh_latest</option><option value="panews_newsflash">panews_newsflash</option><option value="theblock_latest">theblock_latest</option>`
const portalSourceFilterOptions = `<option value="">全部来源</option><option value="flash" {{if eq .FilterSource "flash"}}selected{{end}}>flash</option><option value="headline" {{if eq .FilterSource "headline"}}selected{{end}}>headline</option><option value="jin10_full" {{if eq .FilterSource "jin10_full"}}selected{{end}}>jin10_full</option><option value="eastmoney_kuaixun" {{if eq .FilterSource "eastmoney_kuaixun"}}selected{{end}}>eastmoney_kuaixun</option><option value="wallstreetcn_a_stock" {{if eq .FilterSource "wallstreetcn_a_stock"}}selected{{end}}>wallstreetcn_a_stock</option><option value="cls_telegraph" {{if eq .FilterSource "cls_telegraph"}}selected{{end}}>cls_telegraph</option><option value="sina_finance_7x24" {{if eq .FilterSource "sina_finance_7x24"}}selected{{end}}>sina_finance_7x24</option><option value="crypto_x" {{if eq .FilterSource "crypto_x"}}selected{{end}}>crypto_x</option><option value="crypto_telegram" {{if eq .FilterSource "crypto_telegram"}}selected{{end}}>crypto_telegram</option><option value="foresight_newsflash" {{if eq .FilterSource "foresight_newsflash"}}selected{{end}}>foresight_newsflash</option><option value="coindesk_zh_latest" {{if eq .FilterSource "coindesk_zh_latest"}}selected{{end}}>coindesk_zh_latest</option><option value="panews_newsflash" {{if eq .FilterSource "panews_newsflash"}}selected{{end}}>panews_newsflash</option><option value="theblock_latest" {{if eq .FilterSource "theblock_latest"}}selected{{end}}>theblock_latest</option>`
const portalChannelPlaceholder = `来源，如 flash,headline,jin10_full,eastmoney_kuaixun,wallstreetcn_a_stock,cls_telegraph,sina_finance_7x24,crypto_x,crypto_telegram,foresight_newsflash,coindesk_zh_latest,panews_newsflash,theblock_latest`

const loginTemplate = `
{{define "login"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `main{max-width:420px}</style></head><body><main><section><h1>简苏舆情</h1>{{if .Error}}<p style="color:#9b1c1c">{{.Error}}</p>{{end}}<form method="post"><input type="hidden" name="reference" value="{{.Reference}}"><input name="username" placeholder="用户名" value="admin"><input name="password" type="password" placeholder="密码" value="admin123"><button type="submit">登录</button></form></section></main>{{template "footer" .}}</body></html>{{end}}
`

const dashboardTemplate = `
{{define "dashboard"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.metric-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.metric{padding:16px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.metric strong{display:block;font-size:28px;margin-top:6px}.toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.ok{color:#214e34;font-weight:700}.bad{color:#8f2d2d;font-weight:700}` + `</style></head><body><header><h1>总览</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><h2>核心指标</h2><form method="post"><button type="submit">手动刷新分析</button></form></div><div class="metric-grid"><div class="metric">文章数<strong>{{.Dashboard.Overview.ArticleCount}}</strong></div><div class="metric">项目数<strong>{{.Dashboard.Overview.ProjectCount}}</strong></div><div class="metric">报告数<strong>{{.Dashboard.Overview.ReportCount}}</strong></div><div class="metric">活跃规则<strong>{{.Dashboard.Overview.AlertRuleCount}}</strong></div></div></section><section><h2>近 7 日趋势</h2><table><tr><th>日期</th><th>文章数</th></tr>{{range .Dashboard.Trends}}<tr><td>{{.Label}}</td><td>{{.Count}}</td></tr>{{end}}</table></section><section><h2>来源分布</h2><table><tr><th>来源</th><th>数量</th></tr>{{range .Dashboard.Sources}}<tr><td>{{.SourceType}}</td><td>{{.Count}}</td></tr>{{end}}</table></section><section><h2>关键词热点</h2><table><tr><th>关键词</th><th>次数</th></tr>{{range .Dashboard.Keywords}}<tr><td>{{.Keyword}}</td><td>{{.Count}}</td></tr>{{end}}</table></section><section><h2>最近抓取</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>系统公告</h2><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></section><section><h2>最近任务</h2><table><tr><th>任务</th><th>状态</th><th>开始时间</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无任务记录</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const crawlTemplatesTemplate = `
{{define "crawl_templates"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.compact td form{margin:0}.compact textarea,.compact input,.compact select,.compact button{margin:4px 0;padding:8px}.inline-form{display:inline-block;width:auto;margin-right:6px}` + `</style></head><body><header><h1>模板中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="summary-grid"><div class="summary-card">模板总数<strong>{{len .CrawlTemplates}}</strong></div><div class="summary-card">已启用<strong>{{.CountActive}}</strong></div><div class="summary-card">已停用<strong>{{subInt (len .CrawlTemplates) .CountActive}}</strong></div></div></section><section><h2>新建模板</h2><form method="post"><input type="hidden" name="form_type" value="template"><input name="name" placeholder="模板名称"><input class="js-website-input" name="website" list="crawl-template-website-options" placeholder="网站参数，如 x.com / t.me / example.com"><select class="js-source-type-input" name="source_type"><option value="">选择来源类型</option><option value="flash">flash</option><option value="headline">headline</option><option value="jin10_full">jin10_full</option><option value="crypto_x">crypto_x</option><option value="crypto_telegram">crypto_telegram</option><option value="custom">custom</option></select><label>请求方式</label><select class="js-method-input" name="config_method"><option value="GET">GET</option><option value="POST">POST</option></select><label>页面地址</label><input class="js-base-url-input" name="config_base_url" placeholder="https://example.com"><label>列表选择器</label><input class="js-list-selector-input" name="config_list_selector" placeholder=".list-item"><label>详情链接字段</label><input class="js-detail-url-field-input" name="config_detail_url_field" placeholder="href"><label><input type="checkbox" name="enabled" checked> 启用</label><textarea class="js-config-json-input" name="config_json" placeholder="模板配置 JSON">{"source_type":"flash","method":"GET","base_url":"https://example.com","list_selector":".list-item","detail_url_field":"href","fields":[{"name":"title","selector":"a","scope":"list","required":true}]}</textarea><button type="submit">创建模板</button></form></section><section><h2>模板列表</h2><table class="compact"><tr><th>ID</th><th>名称</th><th>网站参数</th><th>来源</th><th>请求方式</th><th>页面地址</th><th>列表选择器</th><th>详情链接字段</th><th>启用</th><th>配置 JSON</th><th>操作</th></tr>{{range .CrawlTemplates}}<tr><td>{{.ID}}</td><td><form method="post"><input type="hidden" name="form_type" value="template"><input type="hidden" name="template_id" value="{{.ID}}"><input type="hidden" name="action" value="update"><input name="name" value="{{.Name}}"></td><td><input class="js-website-input" name="website" list="crawl-template-website-options" value="{{.Website}}" placeholder="网站参数"></td><td><select class="js-source-type-input" name="source_type"><option value="flash" {{if eq .SourceType "flash"}}selected{{end}}>flash</option><option value="headline" {{if eq .SourceType "headline"}}selected{{end}}>headline</option><option value="jin10_full" {{if eq .SourceType "jin10_full"}}selected{{end}}>jin10_full</option><option value="crypto_x" {{if eq .SourceType "crypto_x"}}selected{{end}}>crypto_x</option><option value="crypto_telegram" {{if eq .SourceType "crypto_telegram"}}selected{{end}}>crypto_telegram</option><option value="custom" {{if eq .SourceType "custom"}}selected{{end}}>custom</option></select></td><td><select class="js-method-input" name="config_method"><option value="GET">GET</option><option value="POST">POST</option></select></td><td><input class="js-base-url-input" name="config_base_url" placeholder="https://example.com"></td><td><input class="js-list-selector-input" name="config_list_selector" placeholder=".list-item"></td><td><input class="js-detail-url-field-input" name="config_detail_url_field" placeholder="href"></td><td><label><input type="checkbox" name="enabled" {{if .Enabled}}checked{{end}}> 启用</label></td><td><textarea class="js-config-json-input" name="config_json">{{.ConfigJSON}}</textarea></td><td><button type="submit">保存</button></form><form class="inline-form" method="post" action="/crawl-templates/{{.ID}}?action=preview"><input type="hidden" name="form_type" value="template"><button type="submit">预览执行</button></form><form class="inline-form" method="post" action="/crawl-templates/{{.ID}}?action=run"><input type="hidden" name="form_type" value="template"><button type="submit">立即执行</button></form><form class="inline-form" method="post"><input type="hidden" name="form_type" value="template"><input type="hidden" name="template_id" value="{{.ID}}"><input type="hidden" name="action" value="delete"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="11">暂无模板</td></tr>{{end}}</table></section><section><h2>最近执行记录</h2><table><tr><th>ID</th><th>模板</th><th>来源</th><th>状态</th><th>抓取</th><th>入库</th><th>更新时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.ID}}</td><td>{{if .TemplateName}}{{.TemplateName}}{{else}}模板 #{{.TemplateID}}{{end}}</td><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="7">暂无执行记录</td></tr>{{end}}</table></section><datalist id="crawl-template-website-options"><option value="x.com"></option><option value="twitter.com"></option><option value="t.me"></option><option value="telegram.org"></option><option value="jin10.com"></option><option value="xnews.jin10.com"></option><option value="flash.jin10.com"></option><option value="example.com"></option></datalist><script>(function(){var defaults={flash:"flash.jin10.com",headline:"xnews.jin10.com",jin10_full:"jin10.com",crypto_x:"x.com",crypto_telegram:"t.me"};var parseConfig=function(text){try{return JSON.parse(text||"{}")}catch(e){return {}}};document.querySelectorAll("form").forEach(function(form){var source=form.querySelector(".js-source-type-input");var website=form.querySelector(".js-website-input");var method=form.querySelector(".js-method-input");var baseUrl=form.querySelector(".js-base-url-input");var listSelector=form.querySelector(".js-list-selector-input");var detailUrlField=form.querySelector(".js-detail-url-field-input");var config=form.querySelector(".js-config-json-input");if(!source||!website||!method||!baseUrl||!listSelector||!detailUrlField||!config){return}var hydrate=function(){var parsed=parseConfig(config.value);if(!method.value&&parsed.method){method.value=String(parsed.method).toUpperCase()}if(method.value===""){method.value="GET"}if(!baseUrl.value&&parsed.base_url){baseUrl.value=parsed.base_url}if(!listSelector.value&&parsed.list_selector){listSelector.value=parsed.list_selector}if(!detailUrlField.value&&parsed.detail_url_field){detailUrlField.value=parsed.detail_url_field}if(!website.value&&parsed.website){website.value=parsed.website}};var applyDefaultWebsite=function(){if(website.value.trim()!==""||!defaults[source.value]){return}website.value=defaults[source.value]};source.addEventListener("change",applyDefaultWebsite);hydrate();applyDefaultWebsite()})})();</script></main>{{template "footer" .}}</body></html>{{end}}
`

const projectsTemplate = `
{{define "projects"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.compact td form{margin:0}.compact input,.compact textarea,.compact select,.compact button{margin:4px 0;padding:8px}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>项目中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>列表搜索</h2><form class="inline" method="get"><input name="keyword" placeholder="搜索项目组、项目名、关键词、描述" value="{{.FilterKeyword}}"><select name="status"><option value="">全部状态</option><option value="active" {{if eq .FilterStatus "active"}}selected{{end}}>active</option><option value="paused" {{if eq .FilterStatus "paused"}}selected{{end}}>paused</option></select><button type="submit">搜索</button></form>{{if or .FilterKeyword .FilterStatus}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}} <a class="inline" href="/projects">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">项目组<strong>{{len .Groups}}</strong></div><div class="summary-card">项目<strong>{{len .Projects}}</strong></div><div class="summary-card">active<strong>{{.CountActive}}</strong></div><div class="summary-card">paused<strong>{{.CountPaused}}</strong></div></div></section><section><h2>新建项目组</h2><form method="post"><input type="hidden" name="form_type" value="group"><input name="name" placeholder="项目组名称"><textarea name="description" placeholder="项目组描述"></textarea><button type="submit">创建项目组</button></form></section><section><h2>项目组列表</h2><table class="compact"><tr><th>ID</th><th>名称</th><th>描述</th><th>操作</th></tr>{{range .Groups}}<tr><td>{{.ID}}</td><td><form method="post"><input type="hidden" name="form_type" value="group"><input type="hidden" name="action" value="update"><input type="hidden" name="group_id" value="{{.ID}}"><input name="name" value="{{.Name}}"></td><td><textarea name="description">{{.Description}}</textarea></td><td><button type="submit">保存</button></form><form method="post"><input type="hidden" name="form_type" value="group"><input type="hidden" name="action" value="delete"><input type="hidden" name="group_id" value="{{.ID}}"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="4">没有符合条件的项目组</td></tr>{{end}}</table></section><section><h2>新建项目</h2><form method="post"><input type="hidden" name="form_type" value="project"><select name="group_id">{{range .Groups}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><input name="name" placeholder="项目名称"><input name="keywords" placeholder="关键词，逗号分隔"><textarea name="description" placeholder="项目描述"></textarea><button type="submit">创建项目</button></form></section><section><h2>项目列表</h2><table class="compact"><tr><th>ID</th><th>项目组ID</th><th>项目组</th><th>名称</th><th>关键词</th><th>描述</th><th>状态</th><th>操作</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.ID}}</a></td><td><form method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="update"><input type="hidden" name="project_id" value="{{.ID}}"><input name="group_id" value="{{.GroupID}}"></td><td>{{.GroupName}}</td><td><input name="name" value="{{.Name}}"></td><td><input name="keywords" value="{{.Keywords}}"></td><td><textarea name="description">{{.Description}}</textarea></td><td><select name="status"><option value="active" {{if eq .Status "active"}}selected{{end}}>active</option><option value="paused" {{if eq .Status "paused"}}selected{{end}}>paused</option></select></td><td><button type="submit">保存</button></form><a class="inline" href="/projects/{{.ID}}">详情</a><a class="inline" href="/articles?project_id={{.ID}}">文章</a><a class="inline" href="/reports?project_id={{.ID}}">报告</a><form method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="delete"><input type="hidden" name="project_id" value="{{.ID}}"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="8">没有符合条件的项目</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const projectTemplate = `
{{define "project"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.danger button{background:#8f2d2d}.crawl-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.crawl-card{border:1px solid #ece7dc;border-radius:14px;padding:16px;background:#faf8f2}` + `</style></head><body><header><h1>项目详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="/projects">返回项目中心</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看全部文章</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看全部报告</a></div><h2>{{.Project.Name}}</h2><p>项目组：{{.Project.GroupName}} | 状态：{{.Project.Status}}</p><p>关键词：{{.Project.Keywords}}</p><pre>{{.Project.Description}}</pre><div class="summary-grid"><div class="summary-card">关联规则<strong>{{len .Rules}}</strong></div><div class="summary-card">active 规则<strong>{{.CountActive}}</strong></div><div class="summary-card">paused 规则<strong>{{.CountPaused}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div><div class="summary-card">关联报告<strong>{{len .Reports}}</strong></div><div class="summary-card">generated 报告<strong>{{.CountGenerated}}</strong></div><div class="summary-card">draft 报告<strong>{{.CountDraft}}</strong></div><div class="summary-card">archived 报告<strong>{{.CountArchived}}</strong></div></div></section><section><h2>编辑项目</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="update"><select name="group_id">{{range .Groups}}<option value="{{.ID}}" {{if eq .ID $.Project.GroupID}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="name" value="{{.Project.Name}}" placeholder="项目名称"><input name="keywords" value="{{.Project.Keywords}}" placeholder="关键词"><select name="status"><option value="active" {{if eq .Project.Status "active"}}selected{{end}}>active</option><option value="paused" {{if eq .Project.Status "paused"}}selected{{end}}>paused</option></select><textarea name="description" placeholder="项目描述">{{.Project.Description}}</textarea><button type="submit">保存项目</button></form><form class="danger" method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="delete"><button type="submit">删除项目</button></form></section><section><h2>抓取任务</h2><div class="crawl-grid"><div class="crawl-card"><h3>按来源抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type">` + portalSourceOptions + `</select><button type="submit">立即抓取</button></form></div><div class="crawl-card"><h3>按模板抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="template_id"><option value="">选择模板</option>{{range .CrawlTemplates}}<option value="{{.ID}}">{{.Name}} [{{.SourceType}}]</option>{{else}}<option value="">暂无可用模板</option>{{end}}</select><input name="keyword" value="{{.Project.Keywords}}" placeholder="模板关键词（可选）"><button type="submit">模板抓取</button></form></div></div></section><section><h2>快速创建规则</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="rule"><input name="name" placeholder="规则名称"><input name="include_keywords" value="{{.Project.Keywords}}" placeholder="包含关键词"><input name="exclude_keywords" placeholder="排除关键词"><input name="channels" placeholder="` + portalChannelPlaceholder + `"><select name="severity"><option value="medium">medium</option><option value="high">high</option><option value="low">low</option></select><button type="submit">创建规则</button></form></section><section><h2>快速生成报告</h2><form method="post"><input type="hidden" name="form_type" value="report"><input name="title" value="{{.Project.Name}} 每日简报" placeholder="报告标题"><textarea name="content" placeholder="输入摘要、正文或人工分析内容"></textarea><button type="submit">生成报告</button></form></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>关联规则</h2><table><tr><th>名称</th><th>等级</th><th>状态</th><th>来源</th></tr>{{range .Rules}}<tr><td><a class="inline" href="/monitor-rules/{{.ID}}">{{.Name}}</a></td><td>{{.Severity}}</td><td>{{.Status}}</td><td>{{.Channels}}</td></tr>{{else}}<tr><td colspan="4">暂无关联规则</td></tr>{{end}}</table></section><section><h2>最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无文章</td></tr>{{end}}</table></section><section><h2>关联报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">暂无报告</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const rulesTemplate = `
{{define "rules"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.compact td form{margin:0}.compact input,.compact textarea,.compact select,.compact button{margin:4px 0;padding:8px}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.crawl-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.crawl-card{border:1px solid #ece7dc;border-radius:14px;padding:16px;background:#faf8f2}` + `</style></head><body><header><h1>监测规则</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>列表搜索</h2><form class="inline" method="get"><input name="keyword" placeholder="搜索规则名、项目名、关键词、来源" value="{{.FilterKeyword}}"><select name="status"><option value="">全部状态</option><option value="active" {{if eq .FilterStatus "active"}}selected{{end}}>active</option><option value="paused" {{if eq .FilterStatus "paused"}}selected{{end}}>paused</option></select><button type="submit">搜索</button></form>{{if or .FilterKeyword .FilterStatus}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}} <a class="inline" href="/monitor-rules">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">规则<strong>{{len .Rules}}</strong></div><div class="summary-card">可选项目<strong>{{len .Projects}}</strong></div><div class="summary-card">active<strong>{{.CountActive}}</strong></div><div class="summary-card">paused<strong>{{.CountPaused}}</strong></div></div></section><section><h2>新建规则</h2><form method="post"><select name="project_id">{{range .Projects}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><input name="name" placeholder="规则名称"><input name="include_keywords" placeholder="包含关键词"><input name="exclude_keywords" placeholder="排除关键词"><input name="channels" placeholder="` + portalChannelPlaceholder + `"><select name="severity"><option value="low">low</option><option value="medium">medium</option><option value="high">high</option></select><button type="submit">创建规则</button></form></section><section><h2>规则任务</h2><div class="crawl-grid"><div class="crawl-card"><h3>按来源抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type">` + portalSourceOptions + `</select><button type="submit">立即抓取</button></form></div><div class="crawl-card"><h3>按模板抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="template_id"><option value="">选择模板</option>{{range .CrawlTemplates}}<option value="{{.ID}}">{{.Name}} [{{.SourceType}}]</option>{{else}}<option value="">暂无可用模板</option>{{end}}</select><input name="keyword" placeholder="模板关键词（可选）"><button type="submit">模板抓取</button></form></div></div></section><section><h2>规则列表</h2><table class="compact"><tr><th>ID</th><th>项目ID</th><th>项目</th><th>名称</th><th>包含</th><th>排除</th><th>来源</th><th>等级</th><th>状态</th><th>操作</th></tr>{{range .Rules}}<tr><td><a class="inline" href="/monitor-rules/{{.ID}}">{{.ID}}</a></td><td><form method="post"><input type="hidden" name="action" value="update"><input type="hidden" name="rule_id" value="{{.ID}}"><input name="project_id" value="{{.ProjectID}}"></td><td>{{.ProjectName}}</td><td><input name="name" value="{{.Name}}"></td><td><input name="include_keywords" value="{{.IncludeKeywords}}"></td><td><input name="exclude_keywords" value="{{.ExcludeKeywords}}"></td><td><input name="channels" value="{{.Channels}}"></td><td><select name="severity"><option value="low" {{if eq .Severity "low"}}selected{{end}}>low</option><option value="medium" {{if eq .Severity "medium"}}selected{{end}}>medium</option><option value="high" {{if eq .Severity "high"}}selected{{end}}>high</option></select></td><td><select name="status"><option value="active" {{if eq .Status "active"}}selected{{end}}>active</option><option value="paused" {{if eq .Status "paused"}}selected{{end}}>paused</option></select></td><td><button type="submit">保存</button></form><a class="inline" href="/monitor-rules/{{.ID}}">详情</a><a class="inline" href="/articles?project_id={{.ProjectID}}">文章</a><a class="inline" href="/reports?project_id={{.ProjectID}}">报告</a><form method="post"><input type="hidden" name="action" value="toggle"><input type="hidden" name="rule_id" value="{{.ID}}"><input type="hidden" name="project_id" value="{{.ProjectID}}"><input type="hidden" name="name" value="{{.Name}}"><input type="hidden" name="include_keywords" value="{{.IncludeKeywords}}"><input type="hidden" name="exclude_keywords" value="{{.ExcludeKeywords}}"><input type="hidden" name="channels" value="{{.Channels}}"><input type="hidden" name="severity" value="{{.Severity}}"><input type="hidden" name="status" value="{{.Status}}"><button type="submit">{{if eq .Status "active"}}停用{{else}}启用{{end}}</button></form><form method="post"><input type="hidden" name="action" value="delete"><input type="hidden" name="rule_id" value="{{.ID}}"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="10">没有符合条件的规则</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const ruleTemplate = `
{{define "rule"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.danger button{background:#8f2d2d}.crawl-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.crawl-card{border:1px solid #ece7dc;border-radius:14px;padding:16px;background:#faf8f2}` + `</style></head><body><header><h1>规则详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="/monitor-rules">返回规则中心</a>{{if .Project.ID}}<a class="inline" href="/projects/{{.Project.ID}}">所属项目详情</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看项目文章</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看项目报告</a>{{end}}</div><h2>{{.Rule.Name}}</h2><p>项目：{{.Rule.ProjectName}} | 状态：{{.Rule.Status}} | 等级：{{.Rule.Severity}}</p><p>包含关键词：{{.Rule.IncludeKeywords}}</p><p>排除关键词：{{.Rule.ExcludeKeywords}}</p><p>来源：{{.Rule.Channels}}</p><div class="summary-grid"><div class="summary-card">所属项目<strong>{{if .Project.Name}}{{.Project.Name}}{{else}}未关联{{end}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div><div class="summary-card">最近报告<strong>{{len .Reports}}</strong></div></div></section><section><h2>编辑规则</h2><form class="inline" method="post"><input type="hidden" name="action" value="update"><input type="hidden" name="project_id" value="{{.Rule.ProjectID}}"><input type="hidden" name="status" value="{{.Rule.Status}}"><input name="name" value="{{.Rule.Name}}" placeholder="规则名称"><input name="include_keywords" value="{{.Rule.IncludeKeywords}}" placeholder="包含关键词"><input name="exclude_keywords" value="{{.Rule.ExcludeKeywords}}" placeholder="排除关键词"><input name="channels" value="{{.Rule.Channels}}" placeholder="` + portalChannelPlaceholder + `"><select name="severity"><option value="low" {{if eq .Rule.Severity "low"}}selected{{end}}>low</option><option value="medium" {{if eq .Rule.Severity "medium"}}selected{{end}}>medium</option><option value="high" {{if eq .Rule.Severity "high"}}selected{{end}}>high</option></select><button type="submit">保存规则</button></form><form class="inline" method="post"><input type="hidden" name="action" value="toggle"><input type="hidden" name="project_id" value="{{.Rule.ProjectID}}"><input type="hidden" name="name" value="{{.Rule.Name}}"><input type="hidden" name="include_keywords" value="{{.Rule.IncludeKeywords}}"><input type="hidden" name="exclude_keywords" value="{{.Rule.ExcludeKeywords}}"><input type="hidden" name="channels" value="{{.Rule.Channels}}"><input type="hidden" name="severity" value="{{.Rule.Severity}}"><input type="hidden" name="status" value="{{.Rule.Status}}"><button type="submit">{{if eq .Rule.Status "active"}}停用规则{{else}}启用规则{{end}}</button></form><form class="danger" method="post"><input type="hidden" name="action" value="delete"><button type="submit">删除规则</button></form></section>{{if .Project.ID}}<section><h2>规则任务</h2><div class="crawl-grid"><div class="crawl-card"><h3>按来源抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><input type="hidden" name="project_id" value="{{.Project.ID}}"><select name="source_type">` + portalSourceOptions + `</select><button type="submit">立即抓取</button></form></div><div class="crawl-card"><h3>按模板抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><input type="hidden" name="project_id" value="{{.Project.ID}}"><select name="template_id"><option value="">选择模板</option>{{range .CrawlTemplates}}<option value="{{.ID}}">{{.Name}} [{{.SourceType}}]</option>{{else}}<option value="">暂无可用模板</option>{{end}}</select><input name="keyword" placeholder="模板关键词（可选）"><button type="submit">模板抓取</button></form></div><div class="crawl-card"><h3>分析刷新</h3><form method="post"><input type="hidden" name="form_type" value="analysis"><input type="hidden" name="project_id" value="{{.Project.ID}}"><button type="submit">刷新分析</button></form></div></div></section><section><h2>所属项目</h2><table><tr><th>项目</th><th>项目组</th><th>状态</th><th>关键词</th></tr><tr><td><a class="inline" href="/projects/{{.Project.ID}}">{{.Project.Name}}</a></td><td>{{.Project.GroupName}}</td><td>{{.Project.Status}}</td><td>{{.Project.Keywords}}</td></tr></table></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section>{{end}}<section><h2>关联项目最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Rule.ProjectID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无文章</td></tr>{{end}}</table></section><section><h2>关联项目最近报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Rule.ProjectID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">暂无报告</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const articlesTemplate = `
{{define "articles"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `main{max-width:1700px;font-size:14px}.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.articles-table th,.articles-table td{font-size:14px;padding:8px}.articles-table .col-title{width:62%}.articles-table .col-source{width:8%}.articles-table .col-time{width:12%}.articles-table .col-actions{width:18%}.articles-table .ops{white-space:nowrap}.articles-table .ops form{display:inline-block;width:auto;margin:0 6px 6px 0;vertical-align:middle}.articles-table .ops form:last-child{margin-right:0}.articles-table .ops button{width:auto;margin:0;padding:9px 12px;font-size:13px;white-space:nowrap}.articles-table .ops select{width:auto;min-width:88px;margin:0 6px 0 0;padding:8px}.page-title{display:flex;justify-content:space-between;gap:16px;align-items:flex-end;flex-wrap:wrap}.page-title p{margin:0;color:#6a6257}.filter-grid{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:12px;align-items:end}.filter-grid .field{margin:0}.filter-grid .field input,.filter-grid .field select{margin:0}.filter-grid .filter-submit button{width:100%;margin:0}.filter-actions{display:flex;justify-content:flex-end;gap:10px;margin-top:12px;flex-wrap:wrap}.filter-actions a{width:auto;min-width:140px}.page-nav{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;margin-top:16px}.page-nav .pager-links{display:flex;gap:10px;flex-wrap:wrap}.page-nav a{padding:8px 12px;border:1px solid #d0c8b8;border-radius:10px;text-decoration:none;color:#214e34;background:#fff}@media (max-width: 1400px){.filter-grid{grid-template-columns:repeat(4,minmax(0,1fr))}}@media (max-width: 900px){.filter-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.page-nav{align-items:flex-start}}` + `</style></head><body><header><div class="page-title"><div><h1>文章中心</h1></div></div>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><form method="get"><div class="filter-grid"><div class="field"><select name="mode"><option value="" {{if eq .SearchMode ""}}selected{{end}}>普通筛选</option><option value="search" {{if eq .SearchMode "search"}}selected{{end}}>基础全文</option><option value="full" {{if eq .SearchMode "full"}}selected{{end}}>高级检索</option><option value="timely" {{if eq .SearchMode "timely"}}selected{{end}}>实时搜索</option></select></div><div class="field"><input name="keyword" placeholder="关键词" value="{{.FilterKeyword}}"></div><div class="field"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select></div><div class="field"><select name="source_type">` + portalSourceFilterOptions + `</select></div><div class="field"><select name="industry"><option value="">全部行业</option>{{range .SearchOptions.Industries}}<option value="{{.}}" {{if eq . $.FilterIndustry}}selected{{end}}>{{.}}</option>{{end}}</select></div><div class="field"><select name="province"><option value="">全部省份</option>{{range .SearchOptions.Provinces}}<option value="{{.}}" {{if eq . $.FilterProvince}}selected{{end}}>{{.}}</option>{{end}}</select></div><div class="field"><select name="city"><option value="">全部城市</option>{{range .SearchOptions.Cities}}<option value="{{.}}" {{if eq . $.FilterCity}}selected{{end}}>{{.}}</option>{{end}}</select></div><div class="field"><select name="read"><option value="">全部阅读状态</option><option value="read" {{if eq .FilterRead "read"}}selected{{end}}>已读</option><option value="unread" {{if eq .FilterRead "unread"}}selected{{end}}>未读</option></select></div><div class="field"><select name="favorite"><option value="">全部收藏状态</option><option value="favorited" {{if eq .FilterFlag "favorited"}}selected{{end}}>已收藏</option><option value="unfavorited" {{if eq .FilterFlag "unfavorited"}}selected{{end}}>未收藏</option></select></div><div class="field"><input type="date" name="start" value="{{.FilterStart}}"></div><div class="field"><input type="date" name="end" value="{{.FilterEnd}}"></div><div class="field filter-submit"><button type="submit">筛选</button></div></div>{{if or .FilterKeyword .FilterProject .FilterSource .FilterRead .FilterFlag .FilterStart .FilterEnd .SearchMode .FilterIndustry .FilterProvince .FilterCity}}<div class="filter-actions"><a class="inline" href="/articles">清空筛选</a></div>{{end}}</form></section><section><h2>{{if eq .SearchMode "search"}}全文搜索结果{{else if eq .SearchMode "full"}}高级检索结果{{else if eq .SearchMode "timely"}}实时搜索结果{{else}}列表{{end}}</h2><table class="articles-table"><tr><th class="col-title">标题</th><th class="col-source">来源</th><th class="col-time">发布时间</th><th class="col-actions">操作</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{formatArticlePublishTime .}}</td><td class="ops"><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="share"><button type="submit">登记分享</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="emotion"><select name="emotion"><option value="1">正面</option><option value="2" selected>中性</option><option value="3">负面</option></select><button type="submit">更新情感</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="hide"><button type="submit">隐藏文章</button></form></td></tr>{{else}}<tr><td colspan="4">没有符合条件的文章</td></tr>{{end}}</table><div class="page-nav"><div>共 {{.Articles.Total}} 条，每页 20 条</div><div class="pager-links"><a class="{{if le .ArticlePage 1}}disabled{{end}}" href="{{.ArticlePrevURL}}">上一页</a><span>第 {{.ArticlePage}} / {{.ArticleTotalPages}} 页</span><a class="{{if ge .ArticlePage .ArticleTotalPages}}disabled{{end}}" href="{{.ArticleNextURL}}">下一页</a></div></div></section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">文章<strong>{{.Articles.Total}}</strong></div><div class="summary-card">当前页已读<strong>{{.CountRead}}</strong></div><div class="summary-card">当前页未读<strong>{{.CountUnread}}</strong></div><div class="summary-card">当前页已收藏<strong>{{.CountFlagged}}</strong></div><div class="summary-card">模式<strong>{{if eq .SearchMode "search"}}基础全文{{else if eq .SearchMode "full"}}高级检索{{else if eq .SearchMode "timely"}}实时搜索{{else}}筛选{{end}}</strong></div><div class="summary-card">分页<strong>{{.ArticlePage}} / {{.ArticleTotalPages}}</strong></div></div></section></main>{{template "footer" .}}</body></html>{{end}}
`

const articlesRealtimeTemplate = `
{{define "articles"}}<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<style>` + baseStyles + `main{max-width:1700px;font-size:14px}.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.articles-table th,.articles-table td{font-size:14px;padding:8px}.articles-table .col-title{width:62%}.articles-table .col-source{width:8%}.articles-table .col-time{width:12%}.articles-table .col-actions{width:18%}.articles-table .ops{white-space:nowrap}.articles-table .ops form{display:inline-block;width:auto;margin:0 6px 6px 0;vertical-align:middle}.articles-table .ops form:last-child{margin-right:0}.articles-table .ops button{width:auto;margin:0;padding:9px 12px;font-size:13px;white-space:nowrap}.articles-table .ops select{width:auto;min-width:88px;margin:0 6px 0 0;padding:8px}.page-title{display:flex;justify-content:space-between;gap:16px;align-items:flex-end;flex-wrap:wrap}.page-title p{margin:0;color:#6a6257}.filter-grid{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:12px;align-items:end}.filter-grid .field{margin:0}.filter-grid .field input,.filter-grid .field select{margin:0}.filter-grid .filter-submit button{width:100%;margin:0}.filter-actions{display:flex;justify-content:flex-end;gap:10px;margin-top:12px;flex-wrap:wrap}.filter-actions a{width:auto;min-width:140px}.page-nav{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;margin-top:16px}.page-nav .pager-links{display:flex;gap:10px;flex-wrap:wrap}.page-nav a{padding:8px 12px;border:1px solid #d0c8b8;border-radius:10px;text-decoration:none;color:#214e34;background:#fff}@media (max-width: 1400px){.filter-grid{grid-template-columns:repeat(4,minmax(0,1fr))}}@media (max-width: 900px){.filter-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.page-nav{align-items:flex-start}}` + `</style>
</head>
<body>
<header><div class="page-title"><div><h1>文章中心</h1></div></div>{{template "nav" .}}</header>
<main>
{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}
<section>
<form method="get">
<div class="filter-grid">
<div class="field"><select name="mode"><option value="" {{if eq .SearchMode ""}}selected{{end}}>普通筛选</option><option value="search" {{if eq .SearchMode "search"}}selected{{end}}>基础全文</option><option value="full" {{if eq .SearchMode "full"}}selected{{end}}>高级检索</option><option value="timely" {{if eq .SearchMode "timely"}}selected{{end}}>实时搜索</option></select></div>
<div class="field"><select name="sort"><option value="captured_at_desc" {{if eq .ArticleSort "captured_at_desc"}}selected{{end}}>实时同步</option><option value="publish_time_desc" {{if eq .ArticleSort "publish_time_desc"}}selected{{end}}>发布时间</option></select></div>
<div class="field"><input name="keyword" placeholder="关键词" value="{{.FilterKeyword}}"></div>
<div class="field"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select></div>
<div class="field"><select name="source_type">` + portalSourceFilterOptions + `</select></div>
<div class="field"><select name="industry"><option value="">全部行业</option>{{range .SearchOptions.Industries}}<option value="{{.}}" {{if eq . $.FilterIndustry}}selected{{end}}>{{.}}</option>{{end}}</select></div>
<div class="field"><select name="province"><option value="">全部省份</option>{{range .SearchOptions.Provinces}}<option value="{{.}}" {{if eq . $.FilterProvince}}selected{{end}}>{{.}}</option>{{end}}</select></div>
<div class="field"><select name="city"><option value="">全部城市</option>{{range .SearchOptions.Cities}}<option value="{{.}}" {{if eq . $.FilterCity}}selected{{end}}>{{.}}</option>{{end}}</select></div>
<div class="field"><select name="read"><option value="">全部阅读状态</option><option value="read" {{if eq .FilterRead "read"}}selected{{end}}>已读</option><option value="unread" {{if eq .FilterRead "unread"}}selected{{end}}>未读</option></select></div>
<div class="field"><select name="favorite"><option value="">全部收藏状态</option><option value="favorited" {{if eq .FilterFlag "favorited"}}selected{{end}}>已收藏</option><option value="unfavorited" {{if eq .FilterFlag "unfavorited"}}selected{{end}}>未收藏</option></select></div>
<div class="field"><input type="date" name="start" value="{{.FilterStart}}"></div>
<div class="field"><input type="date" name="end" value="{{.FilterEnd}}"></div>
<div class="field filter-submit"><button type="submit">筛选</button></div>
</div>
{{if or .FilterKeyword .FilterProject .FilterSource .FilterRead .FilterFlag .FilterStart .FilterEnd .SearchMode .FilterIndustry .FilterProvince .FilterCity (ne .ArticleSort "captured_at_desc")}}<div class="filter-actions"><a class="inline" href="/articles">清空筛选</a></div>{{end}}
</form>
</section>
<section>
<h2>{{if eq .SearchMode "search"}}全文搜索结果{{else if eq .SearchMode "full"}}高级检索结果{{else if eq .SearchMode "timely"}}实时搜索结果{{else}}列表{{end}}</h2>
<table class="articles-table">
<tr><th class="col-title">标题</th><th class="col-source">来源</th><th class="col-time">{{.ArticleTimeLabel}}</th><th class="col-actions">操作</th></tr>
{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{formatArticleListTime . $.ArticleSort}}</td><td class="ops"><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="share"><button type="submit">登记分享</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="emotion"><select name="emotion"><option value="1">正面</option><option value="2" selected>中性</option><option value="3">负面</option></select><button type="submit">更新情感</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="hide"><button type="submit">隐藏文章</button></form></td></tr>{{else}}<tr><td colspan="4">没有符合条件的文章</td></tr>{{end}}
</table>
<div class="page-nav"><div>共 {{.Articles.Total}} 条，每页 20 条</div><div class="pager-links"><a class="{{if le .ArticlePage 1}}disabled{{end}}" href="{{.ArticlePrevURL}}">上一页</a><span>第 {{.ArticlePage}} / {{.ArticleTotalPages}} 页</span><a class="{{if ge .ArticlePage .ArticleTotalPages}}disabled{{end}}" href="{{.ArticleNextURL}}">下一页</a></div></div>
</section>
<section>
<h2>当前结果</h2>
<div class="summary-grid"><div class="summary-card">文章<strong>{{.Articles.Total}}</strong></div><div class="summary-card">当前页已读<strong>{{.CountRead}}</strong></div><div class="summary-card">当前页未读<strong>{{.CountUnread}}</strong></div><div class="summary-card">当前页已收藏<strong>{{.CountFlagged}}</strong></div><div class="summary-card">模式<strong>{{if eq .SearchMode "search"}}基础全文{{else if eq .SearchMode "full"}}高级检索{{else if eq .SearchMode "timely"}}实时搜索{{else}}筛选{{end}}</strong></div><div class="summary-card">时间轴<strong>{{if eq .ArticleSort "publish_time_desc"}}发布时间{{else}}实时同步{{end}}</strong></div><div class="summary-card">分页<strong>{{.ArticlePage}} / {{.ArticleTotalPages}}</strong></div></div>
</section>
</main>
{{template "footer" .}}
</body>
</html>{{end}}
`

const articleTemplate = `
{{define "article"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc;margin-right:8px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}form.inline{display:inline-flex;gap:8px;align-items:center;margin-right:8px}form.inline select{width:auto;min-width:88px;margin:0;padding:8px}.empty-body{color:#6a6257;margin-top:18px}` + `</style></head><body><header><h1>文章详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="{{.ReturnURL}}">返回筛选结果</a><a class="inline" href="/articles">返回文章中心</a>{{if .Article.SourceURL}}<a class="inline" href="{{.Article.SourceURL}}" target="_blank" rel="noreferrer">原文链接</a>{{end}}{{if and .Article.DetailURL (ne .Article.DetailURL .Article.SourceURL)}}<a class="inline" href="{{.Article.DetailURL}}" target="_blank" rel="noreferrer">金十详情</a>{{end}}</div><h2>{{.Article.Title}}</h2><p>来源：{{.Article.SourceType}}{{if .Article.FromText}} | {{.Article.FromText}}{{end}} | 发布时间：{{formatArticlePublishTime .Article}}</p>{{if .Article.Summary}}<p>摘要：{{.Article.Summary}}</p>{{end}}<p>{{if .Article.Read}}<span class="pill">已读</span>{{else}}<span class="pill">未读</span>{{end}}{{if .Article.Favorited}}<span class="pill">已收藏</span>{{end}}{{if .Article.TagFlags}}<span class="pill">情感 {{.Article.TagFlags}}</span>{{end}}</p><div class="summary-grid"><div class="summary-card">关联项目<strong>{{len .Projects}}</strong></div><div class="summary-card">关联报告<strong>{{len .Reports}}</strong></div><div class="summary-card">generated 报告<strong>{{.CountGenerated}}</strong></div><div class="summary-card">draft 报告<strong>{{.CountDraft}}</strong></div><div class="summary-card">archived 报告<strong>{{.CountArchived}}</strong></div><div class="summary-card">相关文章<strong>{{len .Related}}</strong></div><div class="summary-card">相关文章已读<strong>{{.CountRead}}</strong></div><div class="summary-card">相关文章未读<strong>{{.CountUnread}}</strong></div><div class="summary-card">相关文章已收藏<strong>{{.CountFlagged}}</strong></div></div><form method="post" class="inline"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post" class="inline"><input type="hidden" name="action" value="favorite"><button type="submit">{{if .Article.Favorited}}取消收藏{{else}}收藏{{end}}</button></form><form method="post" class="inline"><input type="hidden" name="action" value="share"><button type="submit">登记分享</button></form><form method="post" class="inline"><input type="hidden" name="action" value="emotion"><select name="emotion"><option value="1">正面</option><option value="2" selected>中性</option><option value="3">负面</option></select><button type="submit">更新情感</button></form><form method="post" class="inline"><input type="hidden" name="action" value="hide"><button type="submit">隐藏文章</button></form>{{if articleBodyText .Article}}<pre>{{articleBodyText .Article}}</pre>{{else}}<p class="empty-body">暂无正文内容，请点击上方原文链接或金十详情查看。</p>{{end}}</section>{{if .Projects}}<section><h2>项目联查</h2><table><tr><th>项目</th><th>状态</th><th>快捷入口</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.Name}}</a></td><td>{{.Status}}</td><td><a class="inline" href="/articles?project_id={{.ID}}">项目文章</a><a class="inline" href="/reports?project_id={{.ID}}">项目报告</a></td></tr>{{end}}</table></section><section><h2>关联项目</h2><table><tr><th>项目</th><th>项目组</th><th>状态</th><th>关键词</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.Name}}</a></td><td>{{.GroupName}}</td><td>{{.Status}}</td><td>{{.Keywords}}</td></tr>{{end}}</table></section>{{end}}{{if .Reports}}<section><h2>关联项目最近报告</h2><table><tr><th>ID</th><th>项目</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td>{{index $.ProjectNames .ProjectID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{.ProjectID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{end}}</table></section>{{end}}<section><h2>相关文章</h2><table><tr><th>标题</th><th>来源</th><th>状态</th></tr>{{range .Related}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{if .Read}}已读{{else}}未读{{end}}{{if .Favorited}} / 已收藏{{end}}</td></tr>{{else}}<tr><td colspan="3">暂无相关文章</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const reportsTemplate = `
{{define "reports"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>报告中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>生成报告</h2><form method="post"><select name="project_id">{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="title" placeholder="报告标题"><textarea name="content" placeholder="输入文章摘要、正文或人工内容"></textarea><button type="submit">生成报告</button></form></section><section><h2>报告筛选</h2><form class="inline" method="get"><input name="keyword" placeholder="标题关键词" value="{{.FilterKeyword}}"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><select name="status"><option value="">全部状态</option><option value="generated" {{if eq .FilterStatus "generated"}}selected{{end}}>generated</option><option value="draft" {{if eq .FilterStatus "draft"}}selected{{end}}>draft</option><option value="archived" {{if eq .FilterStatus "archived"}}selected{{end}}>archived</option></select><select name="order"><option value="desc" {{if eq .FilterSource "desc"}}selected{{end}}>更新时间倒序</option><option value="asc" {{if eq .FilterSource "asc"}}selected{{end}}>更新时间正序</option></select><button type="submit">筛选</button></form>{{if or .FilterKeyword .FilterProject .FilterStatus .FilterSource}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterProject}}，项目ID：{{.FilterProject}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}}{{if .FilterSource}}，排序：{{.FilterSource}}{{end}} <a class="inline" href="/reports">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">报告<strong>{{len .Reports}}</strong></div><div class="summary-card">draft<strong>{{.CountDraft}}</strong></div><div class="summary-card">generated<strong>{{.CountGenerated}}</strong></div><div class="summary-card">archived<strong>{{.CountArchived}}</strong></div><div class="summary-card">项目<strong>{{len .Projects}}</strong></div></div></section><section><h2>报告列表</h2><table><tr><th>ID</th><th>项目</th><th>标题</th><th>状态</th><th>更新时间</th><th>跳转</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td>{{index $.ProjectNames .ProjectID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td><td><a class="inline" href="/projects/{{.ProjectID}}">项目</a><a class="inline" href="/articles?project_id={{.ProjectID}}">文章</a></td></tr>{{else}}<tr><td colspan="6">没有符合条件的报告</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const reportTemplate = `
{{define "report"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.crawl-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.crawl-card{border:1px solid #ece7dc;border-radius:14px;padding:16px;background:#faf8f2}` + `</style></head><body><header><h1>报告详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="{{.ReturnURL}}">返回筛选结果</a><a class="inline" href="/reports">返回报告中心</a>{{if .Project.ID}}<a class="inline" href="/projects/{{.Project.ID}}">返回所属项目</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看项目全部报告</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看项目文章</a>{{end}}</div><h2>{{.Report.Title}}</h2><p>状态：{{.Report.Status}} | 更新时间：{{.Report.UpdatedAt.Format "2006-01-02 15:04"}}</p>{{if .Project.ID}}<p>所属项目：<a class="inline" href="/projects/{{.Project.ID}}">{{.Project.Name}}</a></p><div class="summary-grid"><div class="summary-card">其他报告<strong>{{len .Reports}}</strong></div><div class="summary-card">draft<strong>{{.CountDraft}}</strong></div><div class="summary-card">generated<strong>{{.CountGenerated}}</strong></div><div class="summary-card">archived<strong>{{.CountArchived}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div></div>{{end}}<h3>摘要</h3><pre>{{.Report.Summary}}</pre><h3>正文</h3><pre>{{.Report.Content}}</pre></section>{{if .Project.ID}}<section><h2>同项目最近报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">除当前报告外暂无同项目报告</td></tr>{{end}}</table></section><section><h2>报告任务</h2><div class="crawl-grid"><div class="crawl-card"><h3>按来源抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type">` + portalSourceOptions + `</select><button type="submit">立即抓取</button></form></div><div class="crawl-card"><h3>按模板抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="template_id"><option value="">选择模板</option>{{range .CrawlTemplates}}<option value="{{.ID}}">{{.Name}} [{{.SourceType}}]</option>{{else}}<option value="">暂无可用模板</option>{{end}}</select><input name="keyword" placeholder="模板关键词（可选）"><button type="submit">模板抓取</button></form></div></div></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>所属项目最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无关联文章</td></tr>{{end}}</table></section>{{end}}<section><h2>重新生成报告</h2><form method="post"><input name="title" value="{{.Report.Title}} 重生成" placeholder="新报告标题"><textarea name="content" placeholder="可修改正文后重新生成">{{.Report.Content}}</textarea><button type="submit">重新生成</button></form></section></main>{{template "footer" .}}</body></html>{{end}}
`

const systemLogsTemplate = `
{{define "system_logs"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.log-table{table-layout:fixed}.log-time{width:230px}.log-level{width:90px}.log-message{white-space:pre-wrap;word-break:break-word}.page-nav{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin:12px 0}.page-nav a{padding:6px 10px;border:1px solid #d0c8b8;border-radius:8px;text-decoration:none;color:#214e34}` + `</style></head><body><header><h1>{{.ServiceLogName}} 日志</h1>{{template "nav" .}}</header><main><section><p><a class="inline" href="/system">返回系统工作台</a></p><div class="page-nav">{{if gt .ServiceLogTotalPages 1}}<a href="/system/logs?service={{urlquery .ServiceLogName}}&page={{.ServiceLogPrev}}">上一页</a><span>第 {{.ServiceLogPage}} / {{.ServiceLogTotalPages}} 页</span><a href="/system/logs?service={{urlquery .ServiceLogName}}&page={{.ServiceLogNext}}">下一页</a>{{else}}<span>共 {{len .ServiceLogEntries}} 条日志</span>{{end}}</div><table class="log-table"><tr><th class="log-time">时间</th><th class="log-level">Level</th><th>具体内容</th></tr>{{range .ServiceLogEntries}}<tr><td class="log-time">{{.Time}}</td><td class="log-level">{{.Level}}</td><td class="log-message">{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无日志</td></tr>{{end}}</table></section></main>{{template "footer" .}}</body></html>{{end}}
`

const logsTemplate = `
{{define "logs"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.logs-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(520px,1fr));gap:16px}.log-card{background:#fff;border-radius:16px;padding:18px;box-shadow:0 8px 24px rgba(0,0,0,.06)}.log-card h2{display:flex;justify-content:space-between;gap:12px;align-items:center;margin-top:0}.log-card h2 a{font-size:14px;color:#214e34;text-decoration:none}.log-table{table-layout:fixed}.log-time{width:210px}.log-level{width:80px}.log-message{white-space:pre-wrap;word-break:break-word}.muted{color:#6a6257}@media (max-width:760px){.logs-grid{grid-template-columns:1fr}.log-time{width:auto}.log-level{width:auto}}` + `</style></head><body><header><h1>日志中心</h1>{{template "nav" .}}</header><main><section><p class="muted">显示各服务最近日志，详情页保留 50 条分页查看。</p></section><div class="logs-grid">{{range .ServiceLogSummaries}}<section class="log-card"><h2><span>{{.Service}}</span><a href="/system/logs?service={{urlquery .Service}}">查看详情</a></h2>{{if .Message}}<p class="muted">{{.Message}}</p>{{end}}<table class="log-table"><tr><th class="log-time">时间</th><th class="log-level">Level</th><th>具体内容</th></tr>{{range .Entries}}<tr><td class="log-time">{{.Time}}</td><td class="log-level">{{.Level}}</td><td class="log-message">{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无日志</td></tr>{{end}}</table></section>{{else}}<section><p>暂无服务日志</p></section>{{end}}</div></main>{{template "footer" .}}</body></html>{{end}}
`

const systemTemplateRaw = `
{{define "system"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.ok{color:#214e34;font-weight:700}.busy{color:#1f5fbf;font-weight:700}.bad{color:#8f2d2d;font-weight:700}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px}.tabs{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:16px}.tabs a{padding:8px 12px;border-radius:999px;background:#efe9dc;color:#214e34;text-decoration:none}.tabs a.active{background:#214e34;color:#fff}.muted{color:#6a6257}.page-nav{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.page-nav a{padding:6px 10px;border:1px solid #d0c8b8;border-radius:8px;text-decoration:none;color:#214e34}.favorite-card,.warning-card{display:grid;grid-template-columns:2.2fr 1fr 1fr 1fr 1fr;gap:10px;align-items:center;padding:10px 0;border-bottom:1px solid #ece7dc}.warning-card{grid-template-columns:2.2fr 1fr 1fr 1fr 1fr 1fr}.section-block{margin-top:20px}.crawl-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.crawl-card{border:1px solid #ece7dc;border-radius:14px;padding:16px;background:#faf8f2}.config-line{font-family:Consolas,monospace;font-size:13px;background:#faf8f2;border:1px solid #ece7dc;border-radius:8px;padding:10px;overflow:auto}.database-grid{grid-template-columns:minmax(336px,1.2fr) minmax(320px,1fr) minmax(280px,1fr)}.status-table{width:100%}.status-table td:first-child{width:216px;color:#6a6257}.button-row{display:flex;gap:10px;align-items:center}.button-row button{margin:8px 0}.database-switch-row{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.database-switch-row button{margin:8px 0}.service-actions{display:flex;gap:8px;align-items:center;flex-wrap:nowrap}.service-actions .service-action-form{display:inline-flex;width:auto;margin:0}.service-actions .service-button{display:inline-flex;align-items:center;justify-content:center;width:42px;min-width:42px;max-width:42px;margin:0;padding:8px 0;white-space:nowrap}.feedback-textarea{min-height:168px;resize:vertical}` + `</style></head><body><header><h1>系统工作台</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="tabs"><a class="{{if eq .SectionKey "account"}}active{{end}}" href="/system?section=account">账号安全</a><a class="{{if eq .SectionKey "preferences"}}active{{end}}" href="/system?section=preferences">偏好设置</a><a class="{{if eq .SectionKey "database"}}active{{end}}" href="/system?section=database">数据库配置</a><a class="{{if eq .SectionKey "favorites"}}active{{end}}" href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}">收藏夹</a><a class="{{if eq .SectionKey "warningmsg"}}active{{end}}" href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}">预警消息</a><a class="{{if eq .SectionKey "warning"}}active{{end}}" href="/system?section=warning{{if .WarningSetting.ProjectID}}&project_id={{.WarningSetting.ProjectID}}{{end}}">预警设置</a><a class="{{if eq .SectionKey "feedback"}}active{{end}}" href="/system?section=feedback">反馈建议</a><a class="{{if eq .SectionKey "operations"}}active{{end}}" href="/system?section=operations">生产运行</a></div><div class="muted">当前视图：{{.Section}}</div></section><section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th><th>健康检查</th><th>操作</th></tr>{{range .Services}}<tr><td>{{.Name}}</td><td><span class="{{serviceStatusClass .}}">{{serviceStatusLabel .}}</span></td><td>{{.Message}}</td><td><div class="service-actions"><form method="post" class="service-action-form" onsubmit="return confirm('确认重启 {{.Name}}？')"><input type="hidden" name="form_type" value="restart_service"><input type="hidden" name="section" value="{{$.SectionKey}}"><input type="hidden" name="service_name" value="{{.Name}}"><button class="service-button" type="submit">重启</button></form><form method="get" action="/system/logs" class="service-action-form"><input type="hidden" name="service" value="{{.Name}}"><button class="service-button" type="submit">日志</button></form></div></td></tr>{{else}}<tr><td colspan="4">暂无服务状态</td></tr>{{end}}</table></section>{{if eq .SectionKey "database"}}<section class="section-block"><h2>数据库配置</h2><div class="grid database-grid"><div><h3>当前状态</h3><table class="status-table"><tr><td>当前驱动</td><td>{{.DatabaseConfig.Driver}}</td></tr><tr><td>已选择驱动</td><td>{{if .DatabaseConfig.ConfiguredDriver}}{{.DatabaseConfig.ConfiguredDriver}}{{else}}{{.DatabaseConfig.Driver}}{{end}}</td></tr><tr><td>运行 Store</td><td>{{.DatabaseConfig.RuntimeDriver}}</td></tr><tr><td>切换状态</td><td>{{if .DatabaseConfig.RestartRequired}}<span class="bad">待重启生效</span>{{else}}<span class="ok">已生效</span>{{end}}</td></tr><tr><td>状态</td><td>{{if eq .DatabaseConfig.Status "ok"}}<span class="ok">{{.DatabaseConfig.Status}}</span>{{else}}<span class="bad">{{.DatabaseConfig.Status}}</span>{{end}}</td></tr><tr><td>说明</td><td>{{.DatabaseConfig.Message}}</td></tr><tr><td>SQLite 路径</td><td>{{.DatabaseConfig.SQLitePath}}</td></tr><tr><td>PostgreSQL DSN</td><td>{{if .DatabaseConfig.PostgresDSN}}{{.DatabaseConfig.PostgresDSN}}{{else}}未配置{{end}}</td></tr><tr><td>配置文件</td><td>{{.DatabaseConfig.ConfigPath}}</td></tr></table></div><div><h3>数据库连接检测与切换</h3><form method="post"><input type="hidden" name="section" value="database"><select name="driver"><option value="sqlite" {{if eq .DatabaseConfig.ConfiguredDriver "sqlite"}}selected{{end}}>本地 SQLite</option><option value="postgres" {{if eq .DatabaseConfig.ConfiguredDriver "postgres"}}selected{{end}}>PostgreSQL</option></select><input name="sqlite_path" placeholder="SQLite 路径" value="{{.DatabaseConfig.SQLitePath}}"><input name="postgres_dsn" placeholder="postgres://user:password@127.0.0.1:5432/yuqing?sslmode=disable"><input name="postgres_host" placeholder="Host" value="{{.DatabaseConfig.PostgresHost}}"><input name="postgres_port" placeholder="Port" value="{{if .DatabaseConfig.PostgresPort}}{{.DatabaseConfig.PostgresPort}}{{else}}5432{{end}}"><input name="postgres_database" placeholder="Database" value="{{.DatabaseConfig.PostgresDatabase}}"><input name="postgres_user" placeholder="User" value="{{.DatabaseConfig.PostgresUser}}"><input type="password" name="postgres_password" placeholder="Password"><select name="postgres_sslmode"><option value="disable" {{if eq .DatabaseConfig.PostgresSSLMode "disable"}}selected{{end}}>disable</option><option value="require" {{if eq .DatabaseConfig.PostgresSSLMode "require"}}selected{{end}}>require</option><option value="verify-ca" {{if eq .DatabaseConfig.PostgresSSLMode "verify-ca"}}selected{{end}}>verify-ca</option><option value="verify-full" {{if eq .DatabaseConfig.PostgresSSLMode "verify-full"}}selected{{end}}>verify-full</option></select><div class="button-row"><button type="submit" name="form_type" value="database_check">测试连接</button></div><div class="database-switch-row"><button type="submit" name="form_type" value="database_switch_postgres">切换到 PostgreSQL</button><button type="submit" name="form_type" value="database_switch_sqlite">切回 SQLite</button></div></form></div><div><h3>PowerShell 配置</h3><div class="config-line">$env:YUQING_DB_DRIVER='postgres'<br>$env:YUQING_POSTGRES_HOST='127.0.0.1'<br>$env:YUQING_POSTGRES_PORT='5432'<br>$env:YUQING_POSTGRES_DB='yuqing'<br>$env:YUQING_POSTGRES_USER='postgres'<br>$env:YUQING_POSTGRES_PASSWORD='&lt;password&gt;'<br>$env:YUQING_POSTGRES_SSLMODE='disable'</div><p class="muted">切换按钮会保存到本地数据库运行配置文件，并自动重启全部服务读取新配置。当前版本已支持 PostgreSQL schema、数据迁移、配置保存、连接检测和 PostgreSQL Store 接入；业务 Store 运行状态以左侧“运行 Store”为准。</p></div></div></section>{{end}}{{if eq .SectionKey "operations"}}<section class="section-block"><h2>生产运行</h2><div class="grid"><div><h3>Scheduler Jobs</h3><table><tr><th>任务</th><th>Java Quartz</th><th>状态</th><th>下次执行</th></tr>{{range .SchedulerJobs}}<tr><td>{{.Name}}</td><td>{{.JavaQuartzName}}</td><td>{{if .Enabled}}{{if eq .LastStatus "failed"}}<span class="bad">{{.LastStatus}}</span>{{else}}{{.LastStatus}}{{end}}{{else}}disabled{{end}}</td><td>{{if .NextRunAt}}{{.NextRunAt.Format "2006-01-02 15:04"}}{{else}}--{{end}}</td></tr>{{else}}<tr><td colspan="4">暂无 scheduler 数据</td></tr>{{end}}</table></div><div><h3>失败任务</h3><table><tr><th>任务</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}{{if eq .Status "failed"}}<tr><td>{{.TaskName}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{end}}{{else}}<tr><td colspan="3">暂无任务记录</td></tr>{{end}}</table></div><div><h3>Legacy 注册表</h3><table><tr><th>策略</th><th>数量</th></tr>{{range .LegacyRouteSummary}}<tr><td>{{.Strategy}}</td><td>{{.Count}}</td></tr>{{else}}<tr><td colspan="2">暂无注册表数据</td></tr>{{end}}</table></div></div></section><section class="section-block"><h2>外部契约与审计</h2><div class="grid"><div><h3>存活 legacy 路由</h3><table><tr><th>入口</th><th>策略</th><th>下线门槛</th></tr>{{range .LegacyLiveRoutes}}<tr><td>{{.LegacyPath}}</td><td>{{.Strategy}}</td><td>{{.RemovalGate}}</td></tr>{{else}}<tr><td colspan="3">无存活 legacy 路由</td></tr>{{end}}</table></div><div><h3>最近审计</h3><table><tr><th>动作</th><th>资源</th><th>时间</th></tr>{{range .AuditLogs}}<tr><td>{{.Action}}</td><td>{{.Resource}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无审计日志</td></tr>{{end}}</table></div><div><h3>抓取健康</h3><table><tr><th>来源</th><th>状态</th><th>抓取</th><th>入库</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{if eq .Status "failed"}}<span class="bad">{{.Status}}</span>{{else}}{{.Status}}{{end}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td></tr>{{else}}<tr><td colspan="4">暂无抓取记录</td></tr>{{end}}</table></div></div></section>{{end}}{{if eq .SectionKey "account"}}<section class="section-block"><h2>账号安全</h2><div class="grid"><div><h3>个人资料</h3><form method="post"><input type="hidden" name="form_type" value="profile"><input type="hidden" name="section" value="account"><input name="display_name" placeholder="显示名" value="{{index .User "display_name"}}"><input name="email" placeholder="邮箱" value="{{index .User "email"}}"><button type="submit">保存资料</button></form></div><div><h3>修改密码</h3><form method="post"><input type="hidden" name="form_type" value="password"><input type="hidden" name="section" value="account"><input type="password" name="old_password" placeholder="旧密码"><input type="password" name="new_password" placeholder="新密码"><button type="submit">修改密码</button></form></div></div></section>{{end}}{{if eq .SectionKey "preferences"}}<section class="section-block"><h2>偏好设置</h2><div class="grid"><div><h3>用户偏好</h3><form method="post"><input type="hidden" name="form_type" value="preferences"><input type="hidden" name="section" value="preferences"><input name="language" placeholder="语言" value="{{.Preferences.Language}}"><input name="theme" placeholder="主题" value="{{.Preferences.Theme}}"><input name="default_search_mode" placeholder="默认搜索模式" value="{{.Preferences.DefaultSearchMode}}"><input name="article_page_size" placeholder="文章分页大小" value="{{.Preferences.ArticlePageSize}}"><label><input type="checkbox" name="email_notifications" {{if .Preferences.EmailNotifications}}checked{{end}}> 邮件通知</label><button type="submit">保存偏好</button></form></div><div><h3>弹窗状态</h3><form method="post"><input type="hidden" name="form_type" value="popup"><input type="hidden" name="section" value="preferences"><input name="key" value="{{.PopupState.Key}}"><label><input type="checkbox" name="dismissed" {{if .PopupState.Dismissed}}checked{{end}}> 已关闭</label><button type="submit">保存弹窗状态</button></form></div><div><h3>邮件配置</h3><form method="post"><input type="hidden" name="form_type" value="mail"><input type="hidden" name="section" value="preferences"><label><input type="checkbox" name="enabled" {{if .MailConfig.Enabled}}checked{{end}}> 启用</label><input name="smtp_host" placeholder="SMTP Host" value="{{.MailConfig.SMTPHost}}"><input name="smtp_port" placeholder="SMTP Port" value="{{.MailConfig.SMTPPort}}"><input name="username" placeholder="用户名" value="{{.MailConfig.Username}}"><input name="password" placeholder="密码" value="{{.MailConfig.Password}}"><input name="sender_name" placeholder="发件人名称" value="{{.MailConfig.SenderName}}"><input name="sender_email" placeholder="发件人邮箱" value="{{.MailConfig.SenderEmail}}"><button type="submit">保存邮件配置</button></form></div></div></section>{{end}}{{if eq .SectionKey "favorites"}}<section class="section-block"><h2>收藏夹</h2><form class="inline" method="get"><input type="hidden" name="section" value="favorites"><select name="project_id">{{if not .FavoriteProjectID}}<option value="">全部项目</option>{{end}}{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FavoriteProjectID}}selected{{end}}>{{.Name}}</option>{{end}}</select><button type="submit">筛选</button></form><div class="page-nav">{{if gt .FavoriteTotalPages 1}}<a href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}&page={{.FavoritePagePrev}}">上一页</a><span>第 {{.FavoritePage}} / {{.FavoriteTotalPages}} 页</span><a href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}&page={{.FavoritePageNext}}">下一页</a>{{else}}<span>共 {{len .FavoriteItems.Items}} 条收藏</span>{{end}}</div><div>{{range .FavoriteItems.Items}}<div class="favorite-card"><div><a class="inline" href="/monitor/detail/{{if .SourceKey}}{{.SourceKey}}{{else}}{{.ID}}{{end}}?groupid={{index $.GroupNames (firstProjectGroupID . $.Projects)}}&projectid={{firstProjectIDForItem . $.Projects}}">{{.Title}}</a></div><div>{{or .FromText .SourceType}}</div><div>{{index $.GroupNames (firstProjectGroupID . $.Projects)}}</div><div>{{index $.ProjectNames (firstProjectIDForItem . $.Projects)}}</div><div>{{.CapturedAt.Format "2006-01-02 15:04"}}</div></div>{{else}}<p class="muted">暂无收藏文章</p>{{end}}</div></section>{{end}}{{if eq .SectionKey "warningmsg"}}<section class="section-block"><h2>预警消息</h2><form class="inline" method="get"><input type="hidden" name="section" value="warningmsg"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.WarningArticleProjectID}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="keyword" placeholder="关键词" value="{{.WarningArticleKeyword}}"><select name="openFlag"><option value="0" {{if eq .WarningArticleOpenFlag 0}}selected{{end}}>全部</option><option value="1" {{if eq .WarningArticleOpenFlag 1}}selected{{end}}>仅开启预警</option></select><button type="submit">筛选</button></form><div class="page-nav">{{if gt .WarningArticleTotalPages 1}}<a href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}{{if ne .WarningArticleOpenFlag 0}}&openFlag={{.WarningArticleOpenFlag}}{{end}}&page={{.WarningArticlePrev}}">上一页</a><span>第 {{.WarningArticlePage}} / {{.WarningArticleTotalPages}} 页</span><a href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}{{if ne .WarningArticleOpenFlag 0}}&openFlag={{.WarningArticleOpenFlag}}{{end}}&page={{.WarningArticleNext}}">下一页</a>{{else}}<span>共 {{len .WarningArticles}} 条消息</span>{{end}}</div><div>{{range .WarningArticles}}<div class="warning-card"><div><a class="inline" href="/monitor/detail/{{.ArticleID}}?groupid={{.GroupID}}&projectid={{.ProjectID}}" target="_blank">{{.ArticleTitle}}</a></div><div>{{.GroupName}}</div><div>{{.ProjectName}}</div><div>{{.ArticleTime}}</div><div>{{.GroupID}}</div><div>{{.ProjectID}}</div></div>{{else}}<p class="muted">暂无预警消息</p>{{end}}</div></section>{{end}}{{if eq .SectionKey "warning"}}<section class="section-block"><h2>预警设置</h2><form method="post"><input type="hidden" name="form_type" value="warning"><input type="hidden" name="section" value="warning"><select name="project_id">{{range .Projects}}<option value="{{.ID}}" {{if eq .ID $.WarningSetting.ProjectID}}selected{{end}}>{{.Name}}</option>{{end}}</select><label><input type="checkbox" name="enabled" {{if .WarningSetting.Enabled}}checked{{end}}> 启用</label><input name="channels" placeholder="` + portalChannelPlaceholder + `" value="{{.WarningSetting.Channels}}"><input name="threshold" placeholder="阈值" value="{{.WarningSetting.Threshold}}"><input name="recipients" placeholder="接收人" value="{{.WarningSetting.Recipients}}"><textarea name="description" placeholder="说明">{{.WarningSetting.Description}}</textarea><button type="submit">保存预警设置</button></form></section>{{end}}{{if eq .SectionKey "feedback"}}<section class="section-block"><h2>反馈建议</h2><form method="post"><input type="hidden" name="form_type" value="feedback"><input type="hidden" name="section" value="feedback"><input name="title" placeholder="标题"><textarea class="feedback-textarea" name="content" placeholder="问题描述或需求"></textarea><button type="submit">提交</button></form></section>{{end}}<section class="section-block"><h2>运营操作</h2><div class="crawl-grid"><div class="crawl-card"><h3>按来源抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><input type="hidden" name="section" value="{{.SectionKey}}"><select name="source_type">` + portalSourceOptions + `</select><button type="submit">立即抓取</button></form></div><div class="crawl-card"><h3>按模板抓取</h3><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><input type="hidden" name="section" value="{{.SectionKey}}"><select name="template_id"><option value="">选择模板</option>{{range .CrawlTemplates}}<option value="{{.ID}}">{{.Name}} [{{.SourceType}}]</option>{{else}}<option value="">暂无可用模板</option>{{end}}</select><input name="keyword" placeholder="模板关键词（可选）"><button type="submit">模板抓取</button></form></div><div class="crawl-card"><h3>分析刷新</h3><form method="post"><input type="hidden" name="form_type" value="analysis"><input type="hidden" name="section" value="{{.SectionKey}}"><button type="submit">刷新分析快照</button></form></div></div></section><section class="section-block"><h2>公告与任务</h2><div class="grid"><div><h3>公告</h3><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></div><div><h3>任务记录</h3><table><tr><th>任务</th><th>状态</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无任务记录</td></tr>{{end}}</table></div><div><h3>抓取记录</h3><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></div></div></section></main>{{template "footer" .}}</body></html>{{end}}
`

var systemTemplate = buildSystemTemplate()

const systemFullWidthStyles = `body>main,body>.site-footer{max-width:none;width:100%;box-sizing:border-box}`

func buildSystemTemplate() string {
	template := strings.NewReplacer(
		`.feedback-textarea{min-height:168px;resize:vertical}`,
		`.feedback-textarea{min-height:168px;resize:vertical}.feedback-layout{display:grid;grid-template-columns:minmax(0,1fr) minmax(360px,.55fr);gap:16px;align-items:start}.feedback-list{display:grid;gap:10px}.feedback-item{border:1px solid #ece7dc;border-radius:8px;padding:12px;background:#faf8f2}.feedback-item-head{display:flex;gap:12px;align-items:flex-start;justify-content:space-between;margin-bottom:6px}.feedback-item h3{margin:0;font-size:16px}.feedback-delete-form{margin:0}.feedback-delete-button{margin:0;padding:6px 12px;background:#fff;border:1px solid #d7cdbb;color:#8f2d2d}.feedback-delete-button:hover{background:#f8efe9}.feedback-meta{font-size:12px;color:#6a6257;margin-bottom:8px}.feedback-content{white-space:pre-wrap;word-break:break-word}.repair-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(420px,1fr));gap:16px;align-items:start}.repair-card{border:1px solid #ece7dc;border-radius:14px;background:#faf8f2;padding:16px}.repair-meta{font-size:12px;color:#6a6257;margin:8px 0 12px}.repair-field-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.repair-textarea{min-height:220px;font-family:Consolas,Monaco,monospace;font-size:13px;white-space:pre}.repair-textarea.compact{min-height:140px}.repair-tip{font-size:12px;color:#6a6257;margin-top:8px;white-space:pre-wrap}.repair-status{display:flex;gap:12px;flex-wrap:wrap;margin:8px 0 16px}.repair-status strong{display:block;font-size:24px;margin-top:4px}.repair-status-item{min-width:140px;padding:12px;border:1px solid #ece7dc;border-radius:10px;background:#fff}@media (max-width:920px){.feedback-layout{grid-template-columns:1fr}.repair-grid{grid-template-columns:1fr}}`,
		`{{if eq .SectionKey "feedback"}}<section class="section-block"><h2>反馈建议</h2><form method="post"><input type="hidden" name="form_type" value="feedback"><input type="hidden" name="section" value="feedback"><input name="title" placeholder="标题"><textarea class="feedback-textarea" name="content" placeholder="问题描述或需求"></textarea><button type="submit">提交</button></form></section>{{end}}`,
		`{{if eq .SectionKey "feedback"}}<section class="section-block"><h2>反馈建议</h2><form method="post"><input type="hidden" name="form_type" value="feedback"><input type="hidden" name="section" value="feedback"><input name="title" placeholder="标题"><textarea class="feedback-textarea" name="content" placeholder="问题描述或需求"></textarea><button type="submit">提交</button></form></section>{{end}}{{if eq .SectionKey "feedbacklist"}}<section class="section-block"><h2>建议列表</h2><div class="feedback-list">{{range .FeedbackItems}}<article class="feedback-item"><div class="feedback-item-head"><h3>{{.Title}}</h3><form method="post" class="feedback-delete-form" onsubmit="return confirm('确认删除这条建议？')"><input type="hidden" name="form_type" value="delete_feedback"><input type="hidden" name="section" value="feedbacklist"><input type="hidden" name="feedback_id" value="{{.ID}}"><button type="submit" class="feedback-delete-button">删除</button></form></div><div class="feedback-meta">用户 {{.UserID}} · {{.CreatedAt.Format "2006-01-02 15:04"}}</div><div class="feedback-content">{{.Content}}</div></article>{{else}}<p class="muted">暂无反馈建议</p>{{end}}</div></section>{{end}}`,
		`<div class="tabs"><a class="{{if eq .SectionKey "account"}}active{{end}}" href="/system?section=account">账号安全</a><a class="{{if eq .SectionKey "preferences"}}active{{end}}" href="/system?section=preferences">偏好设置</a><a class="{{if eq .SectionKey "database"}}active{{end}}" href="/system?section=database">数据库配置</a><a class="{{if eq .SectionKey "favorites"}}active{{end}}" href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}">收藏夹</a><a class="{{if eq .SectionKey "warningmsg"}}active{{end}}" href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}">预警消息</a><a class="{{if eq .SectionKey "warning"}}active{{end}}" href="/system?section=warning{{if .WarningSetting.ProjectID}}&project_id={{.WarningSetting.ProjectID}}{{end}}">预警设置</a><a class="{{if eq .SectionKey "feedback"}}active{{end}}" href="/system?section=feedback">反馈建议</a><a class="{{if eq .SectionKey "operations"}}active{{end}}" href="/system?section=operations">生产运行</a></div>`,
		`<div class="tabs"><a class="{{if eq .SectionKey "services"}}active{{end}}" href="/system?section=services">服务状态</a><a class="{{if eq .SectionKey "legacy"}}active{{end}}" href="/system?section=legacy">Legacy注册表</a><a class="{{if eq .SectionKey "account"}}active{{end}}" href="/system?section=account">账号安全</a><a class="{{if eq .SectionKey "preferences"}}active{{end}}" href="/system?section=preferences">偏好设置</a><a class="{{if eq .SectionKey "database"}}active{{end}}" href="/system?section=database">数据库配置</a><a class="{{if eq .SectionKey "stockrepair"}}active{{end}}" href="/system?section=stockrepair">推荐股票修复</a><a class="{{if eq .SectionKey "favorites"}}active{{end}}" href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}">收藏夹</a><a class="{{if eq .SectionKey "warningmsg"}}active{{end}}" href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}">预警消息</a><a class="{{if eq .SectionKey "warning"}}active{{end}}" href="/system?section=warning{{if .WarningSetting.ProjectID}}&project_id={{.WarningSetting.ProjectID}}{{end}}">预警设置</a><a class="{{if eq .SectionKey "feedback"}}active{{end}}" href="/system?section=feedback">反馈建议</a><a class="{{if eq .SectionKey "feedbacklist"}}active{{end}}" href="/system?section=feedbacklist">建议列表</a><a class="{{if eq .SectionKey "operations"}}active{{end}}" href="/system?section=operations">生产运行</a><a class="{{if eq .SectionKey "opactions"}}active{{end}}" href="/system?section=opactions">运营操作</a><a class="{{if eq .SectionKey "contracts"}}active{{end}}" href="/system?section=contracts">外部契约与审计</a><a class="{{if eq .SectionKey "announcements"}}active{{end}}" href="/system?section=announcements">公告与任务</a></div>`,
		`</section><section><h2>服务状态</h2>`,
		`</section>{{if eq .SectionKey "services"}}<section><h2>服务状态</h2>`,
		`name="section" value="{{$.SectionKey}}"`,
		`name="section" value="services"`,
		`</table></section>{{if eq .SectionKey "database"}}`,
		`</table></section>{{end}}{{if eq .SectionKey "legacy"}}<section class="section-block"><h2>Legacy 注册表</h2><table><tr><th>策略</th><th>数量</th></tr>{{range .LegacyRouteSummary}}<tr><td>{{.Strategy}}</td><td>{{.Count}}</td></tr>{{else}}<tr><td colspan="2">暂无注册表数据</td></tr>{{end}}</table></section>{{end}}{{if eq .SectionKey "database"}}`,
		`<div><h3>Legacy 注册表</h3><table><tr><th>策略</th><th>数量</th></tr>{{range .LegacyRouteSummary}}<tr><td>{{.Strategy}}</td><td>{{.Count}}</td></tr>{{else}}<tr><td colspan="2">暂无注册表数据</td></tr>{{end}}</table></div>`,
		``,
		`</div></section><section class="section-block"><h2>外部契约与审计</h2>`,
		`</div></section>{{end}}{{if eq .SectionKey "contracts"}}<section class="section-block"><h2>外部契约与审计</h2>`,
		`<section class="section-block"><h2>运营操作</h2>`,
		`{{if eq .SectionKey "opactions"}}<section class="section-block"><h2>运营操作</h2>`,
		`name="section" value="{{.SectionKey}}"`,
		`name="section" value="opactions"`,
		`</div></section><section class="section-block"><h2>公告与任务</h2>`,
		`</div></section>{{end}}{{if eq .SectionKey "announcements"}}<section class="section-block"><h2>公告与任务</h2>`,
		`<div><h3>PowerShell 閰嶇疆</h3>`,
		`<div><h3>数据库连接参数保存配置</h3><form method="post"><input type="hidden" name="section" value="database"><input type="hidden" name="form_type" value="database_save_config"><select name="driver"><option value="sqlite" {{if eq .DatabaseConfig.ConfiguredDriver "sqlite"}}selected{{end}}>本地 SQLite</option><option value="postgres" {{if eq .DatabaseConfig.ConfiguredDriver "postgres"}}selected{{end}}>PostgreSQL</option></select><input name="sqlite_path" placeholder="SQLite 路径" value="{{.DatabaseConfig.SQLitePath}}"><input name="postgres_dsn" placeholder="postgres://user:password@127.0.0.1:5432/yuqing?sslmode=disable"><input name="postgres_host" placeholder="Host" value="{{.DatabaseConfig.PostgresHost}}"><input name="postgres_port" placeholder="Port" value="{{if .DatabaseConfig.PostgresPort}}{{.DatabaseConfig.PostgresPort}}{{else}}5432{{end}}"><input name="postgres_database" placeholder="Database" value="{{.DatabaseConfig.PostgresDatabase}}"><input name="postgres_user" placeholder="User" value="{{.DatabaseConfig.PostgresUser}}"><input type="password" name="postgres_password" placeholder="Password"><select name="postgres_sslmode"><option value="disable" {{if eq .DatabaseConfig.PostgresSSLMode "disable"}}selected{{end}}>disable</option><option value="require" {{if eq .DatabaseConfig.PostgresSSLMode "require"}}selected{{end}}>require</option><option value="verify-ca" {{if eq .DatabaseConfig.PostgresSSLMode "verify-ca"}}selected{{end}}>verify-ca</option><option value="verify-full" {{if eq .DatabaseConfig.PostgresSSLMode "verify-full"}}selected{{end}}>verify-full</option></select><button type="submit">保存连接参数配置</button></form><p class="muted">仅保存连接参数到本地配置文件，不执行连接检测，不自动重启服务。</p><h3>PowerShell 配置</h3>`,
		`<div class="button-row"><button type="submit" name="form_type" value="database_check">测试连接</button></div>`,
		`<div class="button-row"><button type="submit" name="form_type" value="database_check">测试连接</button><button type="submit" name="form_type" value="database_save_postgres">保存配置</button></div>`,
		`</div></section></main>{{template "footer" .}}</body></html>{{end}}`,
		`</div></section>{{end}}{{if eq .SectionKey "stockrepair"}}<section class="section-block"><h2>推荐股票修复</h2><form class="inline" method="get" action="/system"><input type="hidden" name="section" value="stockrepair"><input type="date" name="strategy_date" value="{{.AStockRepair.StrategyDate}}"><select name="period"><option value="morning" {{if eq .AStockRepair.Period "morning"}}selected{{end}}>上午推荐</option><option value="afternoon" {{if eq .AStockRepair.Period "afternoon"}}selected{{end}}>下午推荐</option></select><label><input type="checkbox" name="ignore_recent" value="1" {{if .AStockRepair.IgnoreRecent}}checked{{end}}> 快照 ignore_recent</label><button type="submit">加载</button></form><p class="muted">按层修复推荐股票数据。已选股票层覆盖正式推荐结果；推荐快照层覆盖页面展示和回测快照。</p>{{if .AStockRepair.Error}}<p class="bad">{{.AStockRepair.Error}}</p>{{end}}<div class="repair-status"><div class="repair-status-item">已选股票层<strong>{{len .AStockRepair.Selections.Items}}</strong><div class="muted">{{if .AStockRepair.Selections.Found}}已入库{{else}}未找到{{end}}</div></div><div class="repair-status-item">推荐快照层<strong>{{if .AStockRepair.Snapshot.Found}}已入库{{else}}未找到{{end}}</strong><div class="muted">ignore_recent={{if .AStockRepair.IgnoreRecent}}1{{else}}0{{end}}</div></div><div class="repair-status-item">策略日期<strong>{{.AStockRepair.StrategyDate}}</strong><div class="muted">{{if eq .AStockRepair.Period "afternoon"}}下午推荐{{else}}上午推荐{{end}}</div></div></div><div class="repair-grid"><article class="repair-card"><h3>已选股票层</h3><div class="repair-meta">created_at={{.AStockRepair.SelectionsCreatedAtText}} | updated_at={{.AStockRepair.SelectionsUpdatedAtText}} | 此层不区分 ignore_recent</div><form method="post"><input type="hidden" name="form_type" value="astock_repair_selections_save"><input type="hidden" name="section" value="stockrepair"><input type="hidden" name="strategy_date" value="{{.AStockRepair.StrategyDate}}"><input type="hidden" name="period" value="{{.AStockRepair.Period}}"><input type="hidden" name="ignore_recent" value="{{if .AStockRepair.IgnoreRecent}}1{{end}}"><textarea class="repair-textarea" name="selection_items_json">{{.AStockRepair.SelectionsJSON}}</textarea><div class="repair-tip">按 AStockRecommendationSelection[] JSON 整体覆盖当前日期和窗口的正式推荐股票。保存空数组 [] 会清空这一层。</div><button type="submit">保存已选股票层</button></form></article><article class="repair-card"><h3>推荐快照层</h3><div class="repair-meta">created_at={{.AStockRepair.SnapshotCreatedAtText}} | updated_at={{.AStockRepair.SnapshotUpdatedAtText}} | ignore_recent={{if .AStockRepair.IgnoreRecent}}1{{else}}0{{end}}</div><form method="post"><input type="hidden" name="form_type" value="astock_repair_snapshot_save"><input type="hidden" name="section" value="stockrepair"><input type="hidden" name="strategy_date" value="{{.AStockRepair.StrategyDate}}"><input type="hidden" name="period" value="{{.AStockRepair.Period}}"><input type="hidden" name="snapshot_ignore_recent" value="{{if .AStockRepair.IgnoreRecent}}1{{end}}"><div class="repair-field-grid"><input type="number" name="snapshot_generated_count" placeholder="generated_count" value="{{.AStockRepair.Snapshot.GeneratedCount}}"><input type="number" name="snapshot_recent_filtered" placeholder="recent_filtered" value="{{.AStockRepair.Snapshot.RecentFiltered}}"><input type="number" name="snapshot_same_day_morning_filtered" placeholder="same_day_morning_filtered" value="{{.AStockRepair.Snapshot.SameDayMorningFiltered}}"><input type="number" name="snapshot_limit_up_filtered" placeholder="limit_up_filtered" value="{{.AStockRepair.Snapshot.LimitUpFiltered}}"><input type="number" name="snapshot_no_today_market_count" placeholder="no_today_market_count" value="{{.AStockRepair.Snapshot.NoTodayMarketCount}}"><input type="number" name="snapshot_market_candidate_count" placeholder="market_candidate_count" value="{{.AStockRepair.Snapshot.MarketCandidateCount}}"><label><input type="checkbox" name="snapshot_limit_up_filter_enabled" {{if .AStockRepair.Snapshot.LimitUpFilterEnabled}}checked{{end}}> 启用涨停过滤</label><label><input type="checkbox" name="snapshot_today_market_filter_enabled" {{if .AStockRepair.Snapshot.TodayMarketFilterEnabled}}checked{{end}}> 启用当日行情过滤</label></div><input name="snapshot_backtest_status" placeholder="backtest_status" value="{{.AStockRepair.Snapshot.BacktestStatus}}"><input name="snapshot_market_candidate_status" placeholder="market_candidate_status" value="{{.AStockRepair.Snapshot.MarketCandidateStatus}}"><input name="snapshot_auction_amount_label" placeholder="auction_amount_label" value="{{.AStockRepair.Snapshot.AuctionAmountLabel}}"><textarea class="repair-textarea compact" name="snapshot_empty_reason" placeholder="empty_reason">{{.AStockRepair.Snapshot.EmptyReason}}</textarea><textarea class="repair-textarea" name="snapshot_recommendations_json">{{.AStockRepair.SnapshotRecommendationsJSON}}</textarea><textarea class="repair-textarea" name="snapshot_backtests_json">{{.AStockRepair.SnapshotBacktestsJSON}}</textarea><div class="repair-tip">上面两个大文本框分别是 recommendations_json 和 backtests_json，保存前会校验 JSON 结构。</div><button type="submit">保存推荐快照层</button></form></article></div></section>{{end}}</main>{{template "footer" .}}</body></html>{{end}}`,
	).Replace(systemTemplateRaw)
	return strings.Replace(template, "<style>"+baseStyles, "<style>"+baseStyles+systemFullWidthStyles, 1)
}

const platformBindingsWorkbenchTemplate = `
{{define "platform_bindings_v2"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.grid2{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.stack{display:grid;gap:16px}.section-card{border:1px solid #ece7dc;border-radius:14px;background:#faf8f2;padding:16px}.toolbar{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap}.muted{color:#6a6257}` + `</style></head><body><header><h1>平台工作台</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="summary-grid"><div class="summary-card">NLP 绑定<strong>{{if .PlatformNLPBinding.Bound}}已绑定{{else}}未绑定{{end}}</strong></div><div class="summary-card">写作绑定<strong>{{if .PlatformXieBinding.Bound}}已绑定{{else}}未绑定{{end}}</strong></div><div class="summary-card">平台公告<strong>{{len .Notices}}</strong></div><div class="summary-card">最近操作<strong>{{len .AuditLogs}}</strong></div></div></section><section class="grid2"><div class="stack"><div class="section-card"><h2>NLP 绑定</h2><form method="post"><input type="hidden" name="kind" value="nlp"><input name="secret_id" placeholder="secretId" value="{{.PlatformNLPBinding.SecretID}}"><input name="secret_key" placeholder="secretKey" value="{{.PlatformNLPBinding.SecretKey}}"><label><input type="checkbox" name="bound" {{if .PlatformNLPBinding.Bound}}checked{{end}}> 已绑定</label><button type="submit">保存 NLP 绑定</button></form></div><div class="section-card"><h2>写作绑定</h2><form method="post"><input type="hidden" name="kind" value="xie"><input name="secret_id" placeholder="secretId" value="{{.PlatformXieBinding.SecretID}}"><input name="secret_key" placeholder="secretKey" value="{{.PlatformXieBinding.SecretKey}}"><label><input type="checkbox" name="bound" {{if .PlatformXieBinding.Bound}}checked{{end}}> 已绑定</label><button type="submit">保存写作绑定</button></form></div></div><div class="stack"><div class="section-card"><div class="toolbar"><h2>平台公告</h2><a class="inline" href="/platform/notice" target="_blank" rel="noreferrer">旧 notice 接口</a></div><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></div><div class="section-card"><h2>最近平台操作</h2><table><tr><th>动作</th><th>资源</th><th>用户</th><th>时间</th></tr>{{range .AuditLogs}}<tr><td>{{.Action}}</td><td>{{.Resource}}</td><td>{{if .Username}}{{.Username}}{{else}}{{printf "%d" .UserID}}{{end}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">暂无平台操作记录</td></tr>{{end}}</table><p class="muted">当前仅展示 platform.* 操作审计。</p></div></div></section></main>{{template "footer" .}}</body></html>{{end}}
`

const platformWorkbenchTemplateV3 = `
{{define "platform_bindings_v2"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.grid3{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}.stack{display:grid;gap:16px}.section-card{border:1px solid #ece7dc;border-radius:14px;background:#faf8f2;padding:16px}.toolbar{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap}.muted{color:#6a6257}.result{white-space:pre-wrap;line-height:1.6;background:#fff;border:1px solid #ece7dc;border-radius:12px;padding:14px;min-height:72px}.legacy-copy{display:none}` + `</style></head><body><header><h1>平台工作台</h1>{{template "nav" .}}</header><main><div class="legacy-copy">骞冲彴缁戝畾 骞冲彴宸ヤ綔鍙? NLP 缁戝畾 鍐欎綔缁戝畾 宸茬粦瀹? 骞冲彴鍏憡 鏈€杩戝钩鍙版搷浣?</div>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="summary-grid"><div class="summary-card">NLP 绑定<strong>{{if .PlatformNLPBinding.Bound}}已绑定{{else}}未绑定{{end}}</strong></div><div class="summary-card">写作绑定<strong>{{if .PlatformXieBinding.Bound}}已绑定{{else}}未绑定{{end}}</strong></div><div class="summary-card">平台公告<strong>{{len .Notices}}</strong></div><div class="summary-card">最近操作<strong>{{len .AuditLogs}}</strong></div></div></section><section class="grid3"><div class="stack"><div class="section-card"><h2>NLP 绑定</h2><form method="post"><input type="hidden" name="kind" value="nlp"><input name="secret_id" placeholder="secretId" value="{{.PlatformNLPBinding.SecretID}}"><input name="secret_key" placeholder="secretKey" value="{{.PlatformNLPBinding.SecretKey}}"><label><input type="checkbox" name="bound" {{if .PlatformNLPBinding.Bound}}checked{{end}}> 已绑定</label><button type="submit">保存 NLP 绑定</button></form></div><div class="section-card"><h2>写作绑定</h2><form method="post"><input type="hidden" name="kind" value="xie"><input name="secret_id" placeholder="secretId" value="{{.PlatformXieBinding.SecretID}}"><input name="secret_key" placeholder="secretKey" value="{{.PlatformXieBinding.SecretKey}}"><label><input type="checkbox" name="bound" {{if .PlatformXieBinding.Bound}}checked{{end}}> 已绑定</label><button type="submit">保存写作绑定</button></form></div></div><div class="stack"><div class="section-card"><h2>OCR 识别</h2><form method="post" enctype="multipart/form-data"><input type="hidden" name="form_type" value="nlp_ocr"><input name="imageUrl" placeholder="图片 URL" value="{{.PlatformImageURL}}"><input type="file" name="images"><button type="submit">执行 OCR</button></form><div class="result">{{if .PlatformOCRText}}{{.PlatformOCRText}}{{else}}等待识别结果{{end}}</div></div><div class="section-card"><h2>图像识别</h2><form method="post" enctype="multipart/form-data"><input type="hidden" name="form_type" value="nlp_image"><input name="imageUrl" placeholder="图片 URL" value="{{.PlatformImageURL}}"><input type="file" name="file"><button type="submit">提取标签</button></form><div class="result">{{if .PlatformImageKeywords}}{{.PlatformImageKeywords}}{{else}}等待图像标签{{end}}</div></div></div><div class="stack"><div class="section-card"><h2>写作标题生成</h2><form method="post"><input type="hidden" name="form_type" value="xie_title"><input name="article_id" placeholder="文章 ID（可选）" value="{{.PlatformXieArticleID}}"><textarea name="text" placeholder="输入正文、摘要或人工整理内容">{{.PlatformXieDraftText}}</textarea><button type="submit">生成标题</button></form><div class="result">{{if .PlatformXieGeneratedTitle}}{{.PlatformXieGeneratedTitle}}{{else}}等待标题结果{{end}}</div></div><div class="section-card"><h2>写作报告预览</h2><form method="post"><input type="hidden" name="form_type" value="xie_report"><input name="article_id" placeholder="文章 ID（可选）" value="{{.PlatformXieArticleID}}"><input name="title" placeholder="指定标题（可选）" value="{{.PlatformXieGeneratedTitle}}"><input name="relatedword" placeholder="关键词（可选）"><input name="publishTime" placeholder="发布时间（可选）"><textarea name="text" placeholder="输入正文、摘要或人工整理内容">{{.PlatformXieDraftText}}</textarea><button type="submit">生成报告预览</button></form><div class="result">{{if .PlatformXieGeneratedReport}}{{.PlatformXieGeneratedReport}}{{else}}等待报告预览{{end}}</div></div><div class="section-card"><div class="toolbar"><h2>平台公告</h2><a class="inline" href="/platform/notice" target="_blank" rel="noreferrer">旧 notice 接口</a></div><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></div><div class="section-card"><h2>最近平台操作</h2><table><tr><th>动作</th><th>资源</th><th>用户</th><th>时间</th></tr>{{range .AuditLogs}}<tr><td>{{.Action}}</td><td>{{.Resource}}</td><td>{{if .Username}}{{.Username}}{{else}}{{printf "%d" .UserID}}{{end}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">暂无平台操作记录</td></tr>{{end}}</table><p class="muted">当前仅展示 platform.* 操作审计。</p></div></div></section></main>{{template "footer" .}}</body></html>{{end}}
`

const publicOptionWorkbenchTemplate = `
{{define "public_option_workbench"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.grid2{display:grid;grid-template-columns:minmax(320px,1.2fr) minmax(360px,1fr);gap:16px}.stack{display:grid;gap:16px}.section-card{border:1px solid #ece7dc;border-radius:14px;background:#faf8f2;padding:16px}.compact td form{margin:0}.compact input,.compact textarea,.compact select,.compact button{margin:4px 0;padding:8px}.toolbar{display:flex;gap:10px;flex-wrap:wrap;align-items:center}.toolbar a{padding:8px 12px;border-radius:999px;background:#efe9dc;color:#214e34;text-decoration:none}.toolbar a.active{background:#214e34;color:#fff}.muted{color:#6a6257}.analysis{white-space:pre-wrap;line-height:1.6;background:#fff;border:1px solid #ece7dc;border-radius:12px;padding:14px}.danger button{background:#8f2d2d}` + `</style></head><body><header><h1>事件分析工作台</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><form class="inline" method="get" action="/publicoption"><input name="keyword" placeholder="搜索任务名称或关键词" value="{{.FilterKeyword}}"><button type="submit">搜索</button>{{if .FilterKeyword}}<a class="inline" href="/publicoption">清空</a>{{end}}</form><div class="summary-grid"><div class="summary-card">分析任务<strong>{{len .PublicOptions}}</strong></div><div class="summary-card">当前任务<strong>{{if .PublicOption.ID}}{{.PublicOption.EventName}}{{else}}未选择{{end}}</strong></div><div class="summary-card">事件关键词<strong>{{if .PublicOption.EventKeywords}}{{.PublicOption.EventKeywords}}{{else}}未填写{{end}}</strong></div><div class="summary-card">情感指数<strong>{{if .PublicOption.EmotionalIndex}}{{.PublicOption.EmotionalIndex}}{{else}}--{{end}}</strong></div></div></section><section class="grid2"><div class="stack"><div class="section-card"><h2>新建分析任务</h2><form method="post" action="/publicoption"><input type="hidden" name="action" value="create"><input name="eventname" placeholder="事件名称"><input name="eventkeywords" placeholder="事件关键词，逗号分隔"><input name="eventstarttime" placeholder="开始时间，例如 2026-06-01 00:00:00"><input name="eventendtime" placeholder="结束时间，例如 2026-06-04 23:59:59"><input name="eventstopwords" placeholder="停用词"><button type="submit">创建任务</button></form></div><div class="section-card"><h2>任务列表</h2><table class="compact"><tr><th>ID</th><th>事件名称</th><th>关键词</th><th>更新时间</th><th>操作</th></tr>{{range .PublicOptions}}<tr><td>{{.ID}}</td><td>{{.EventName}}</td><td>{{.EventKeywords}}</td><td>{{if .Updatetime.IsZero}}--{{else}}{{.Updatetime.Format "2006-01-02 15:04"}}{{end}}</td><td><a class="inline" href="/publicoption?id={{.ID}}">详情</a></td></tr>{{else}}<tr><td colspan="5">暂无分析任务</td></tr>{{end}}</table></div></div><div class="stack"><div class="section-card"><h2>{{if .PublicOption.ID}}编辑任务{{else}}任务详情{{end}}</h2>{{if .PublicOption.ID}}<form method="post" action="/publicoption"><input type="hidden" name="action" value="update"><input type="hidden" name="id" value="{{.PublicOption.ID}}"><input name="eventname" value="{{.PublicOption.EventName}}" placeholder="事件名称"><input name="eventkeywords" value="{{.PublicOption.EventKeywords}}" placeholder="事件关键词"><input name="eventstarttime" value="{{.PublicOption.EventStartTime}}" placeholder="开始时间"><input name="eventendtime" value="{{.PublicOption.EventEndTime}}" placeholder="结束时间"><input name="eventstopwords" value="{{.PublicOption.EventStopWords}}" placeholder="停用词"><button type="submit">保存任务</button></form><form class="danger" method="post" action="/publicoption"><input type="hidden" name="action" value="delete"><input type="hidden" name="id" value="{{.PublicOption.ID}}"><button type="submit">删除任务</button></form>{{else}}<p class="muted">先创建任务，或从左侧任务列表选择一个任务。</p>{{end}}</div>{{if .PublicOption.ID}}<div class="section-card"><div class="toolbar"><a class="{{if eq .Section ""}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}">总览</a><a class="{{if eq .Section "backanalysis"}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}&section=backanalysis">回溯分析</a><a class="{{if eq .Section "eventContext"}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}&section=eventContext">事件脉络</a><a class="{{if eq .Section "eventTrace"}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}&section=eventTrace">事件追踪</a><a class="{{if eq .Section "statistics"}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}&section=statistics">统计分析</a><a class="{{if eq .Section "propagationAnalysis"}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}&section=propagationAnalysis">传播分析</a><a class="{{if eq .Section "thematicAnalysis"}}active{{end}}" href="/publicoption?id={{.PublicOption.ID}}&section=thematicAnalysis">专题分析</a></div>{{if .Section}}<h2>分析结果</h2><div class="analysis">{{publicOptionAnalysisText .PublicOption .Section}}</div>{{else}}<h2>任务总览</h2><table><tr><th>字段</th><th>内容</th></tr><tr><td>事件名称</td><td>{{.PublicOption.EventName}}</td></tr><tr><td>关键词</td><td>{{.PublicOption.EventKeywords}}</td></tr><tr><td>停用词</td><td>{{.PublicOption.EventStopWords}}</td></tr><tr><td>时间范围</td><td>{{.PublicOption.EventStartTime}} 至 {{.PublicOption.EventEndTime}}</td></tr><tr><td>回溯分析</td><td>{{.PublicOption.BackAnalysis}}</td></tr><tr><td>事件脉络</td><td>{{.PublicOption.EventContext}}</td></tr><tr><td>事件追踪</td><td>{{.PublicOption.EventTrace}}</td></tr><tr><td>统计分析</td><td>{{.PublicOption.Statistics}}</td></tr><tr><td>传播分析</td><td>{{.PublicOption.PropagationAnalysis}}</td></tr><tr><td>专题分析</td><td>{{.PublicOption.ThematicAnalysis}}</td></tr></table>{{end}}</div>{{end}}</div></section></main>{{template "footer" .}}</body></html>{{end}}
`

const platformBindingsTemplate = `
{{define "platform_bindings"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.grid2{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:16px}` + `</style></head><body><header><h1>平台绑定</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="summary-grid"><div class="summary-card">NLP 绑定<strong>{{if .PlatformNLPBinding.Bound}}已绑定{{else}}未绑定{{end}}</strong></div><div class="summary-card">写作绑定<strong>{{if .PlatformXieBinding.Bound}}已绑定{{else}}未绑定{{end}}</strong></div></div></section><section><div class="grid2"><div><h2>NLP 绑定</h2><form method="post"><input type="hidden" name="kind" value="nlp"><input name="secret_id" placeholder="secretId" value="{{.PlatformNLPBinding.SecretID}}"><input name="secret_key" placeholder="secretKey" value="{{.PlatformNLPBinding.SecretKey}}"><label><input type="checkbox" name="bound" {{if .PlatformNLPBinding.Bound}}checked{{end}}> 已绑定</label><button type="submit">保存 NLP 绑定</button></form></div><div><h2>写作绑定</h2><form method="post"><input type="hidden" name="kind" value="xie"><input name="secret_id" placeholder="secretId" value="{{.PlatformXieBinding.SecretID}}"><input name="secret_key" placeholder="secretKey" value="{{.PlatformXieBinding.SecretKey}}"><label><input type="checkbox" name="bound" {{if .PlatformXieBinding.Bound}}checked{{end}}> 已绑定</label><button type="submit">保存写作绑定</button></form></div></div></section></main>{{template "footer" .}}</body></html>{{end}}
`
