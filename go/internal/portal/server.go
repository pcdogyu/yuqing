package portal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
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
	Title                    string
	User                     any
	SectionKey               string
	Dashboard                model.DashboardSnapshot
	Groups                   []model.ProjectGroup
	Project                  model.Project
	Projects                 []model.Project
	Rule                     model.MonitorRule
	Rules                    []model.MonitorRule
	Articles                 model.ItemListResult
	Article                  model.Item
	Related                  []model.Item
	CrawlRuns                []model.CrawlRun
	Reports                  []model.Report
	Report                   model.Report
	Notices                  []model.SystemNotice
	TaskRuns                 []model.TaskRun
	Preferences              model.UserPreference
	PopupState               model.PopupState
	MailConfig               model.MailConfig
	WarningSetting           model.WarningSetting
	SearchOptions            model.SearchOptions
	Error                    string
	Message                  string
	ReturnTo                 string
	ReturnURL                string
	FilterKeyword            string
	FilterProject            string
	FilterStatus             string
	FilterRead               string
	FilterFlag               string
	FilterSource             string
	FilterStart              string
	FilterEnd                string
	FilterIndustry           string
	FilterProvince           string
	FilterCity               string
	SearchMode               string
	Section                  string
	FavoriteItems            model.ItemListResult
	FavoritePage             int
	FavoritePagePrev         int
	FavoritePageNext         int
	FavoriteProjectID        string
	FavoriteTotalPages       int
	WarningArticles          []legacyWarningArticleCompat
	WarningArticlePage       int
	WarningArticlePrev       int
	WarningArticleNext       int
	WarningArticleTotalPages int
	WarningArticleProjectID  string
	WarningArticleOpenFlag   int
	WarningArticleKeyword    string
	Services                 []serviceStatus
	ProjectNames             map[int64]string
	GroupNames               map[int64]string
	CountActive              int
	CountPaused              int
	CountRead                int
	CountUnread              int
	CountFlagged             int
	CountDraft               int
	CountGenerated           int
	CountArchived            int
}

type serviceStatus struct {
	Name    string
	URL     string
	Healthy bool
	Message string
}

func NewServer(cfg config.Config) *Server {
	funcMap := template.FuncMap{
		"firstProjectGroupID":   firstProjectGroupIDForTemplate,
		"firstProjectIDForItem": firstProjectIDForItemTemplate,
	}
	tpl := template.Must(template.New("layout").Funcs(funcMap).Parse(layoutTemplate))
	template.Must(tpl.New("login").Parse(loginTemplate))
	template.Must(tpl.New("dashboard").Parse(dashboardTemplate))
	template.Must(tpl.New("projects").Parse(projectsTemplate))
	template.Must(tpl.New("project").Parse(projectTemplate))
	template.Must(tpl.New("rules").Parse(rulesTemplate))
	template.Must(tpl.New("rule").Parse(ruleTemplate))
	template.Must(tpl.New("articles").Parse(articlesTemplate))
	template.Must(tpl.New("article").Parse(articleTemplate))
	template.Must(tpl.New("reports").Parse(reportsTemplate))
	template.Must(tpl.New("report").Parse(reportTemplate))
	template.Must(tpl.New("system").Parse(systemTemplate))

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
	mux.HandleFunc("/login", s.handleLoginPage)
	mux.HandleFunc("/img/code", s.handleCaptchaCode)
	mux.HandleFunc("/displayboard", s.requireSession(s.handleDisplayBoard))
	mux.HandleFunc("/displayboard/", s.requireSession(s.handleDisplayBoard))
	mux.HandleFunc("/displayboard/collection2", s.requireSessionJSON(s.handleDisplayBoardCollection2))
	mux.HandleFunc("/mobile/monitor", s.requireSession(s.handleMobileMonitor))
	mux.HandleFunc("/mobile/monitor/", s.requireSession(s.handleMobileMonitor))
	mux.HandleFunc("/mobile/monitor/detail", s.requireSession(s.handleMobileMonitorDetail))
	mux.HandleFunc("/mobile/warning", s.requireSession(s.handleMobileWarning))
	mux.HandleFunc("/mobile/getGroupAndProject", s.requireSessionJSON(s.handleMobileGetGroupAndProject))
	mux.HandleFunc("/mobile/mobileQRCode", s.requireSession(s.handleMobileQRCode))
	mux.HandleFunc("/mobile/uuid/", s.handleMobileUUID)
	mux.HandleFunc("/volume", s.requireSession(s.handleVolume))
	mux.HandleFunc("/volume/", s.requireSession(s.handleVolume))
	mux.HandleFunc("/volume/getproject", s.requireSessionJSON(s.handleVolumeGetProject))
	mux.HandleFunc("/volume/projectname", s.requireSessionJSON(s.handleVolumeProjectName))
	mux.HandleFunc("/hot/hotpage", s.requireSession(s.handleHotPage))
	mux.HandleFunc("/hot/hotpage/", s.requireSession(s.handleHotPage))
	mux.HandleFunc("/hot/hotlist", s.requireSession(s.handleHotList))
	mux.HandleFunc("/dist/monitor", s.handleDistMonitor)
	mux.HandleFunc("/dist/getdata", s.handleDistGetData)
	mux.HandleFunc("/dist/apply", s.handleDistApply)
	mux.HandleFunc("/dist/yqapply", s.handleDistYqApply)
	mux.HandleFunc("/dist/applydatainfo", s.handleDistApplyDataInfo)
	mux.HandleFunc("/dist/yqmontitor", s.handleDistYqMonitor)
	mux.HandleFunc("/dist/hotdata", s.handleDistHotData)
	mux.HandleFunc("/platform/", s.requireSession(s.handlePlatformCompat))
	mux.HandleFunc("/publicoption", s.requireSession(s.handlePublicOptionEntry))
	mux.HandleFunc("/publicoption/", s.requireSession(s.handlePublicOptionCompat))
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/industry", s.requireSessionJSON(s.handleLegacySearchBuckets("industry")))
	mux.HandleFunc("/industry/", s.requireSessionJSON(s.handleLegacySearchBuckets("industry")))
	mux.HandleFunc("/getevent", s.requireSessionJSON(s.handleLegacySearchBuckets("event")))
	mux.HandleFunc("/getevent/", s.requireSessionJSON(s.handleLegacySearchBuckets("event")))
	mux.HandleFunc("/getProvinceList", s.requireSessionJSON(s.handleLegacySearchBuckets("province")))
	mux.HandleFunc("/getProvinceList/", s.requireSessionJSON(s.handleLegacySearchBuckets("province")))
	mux.HandleFunc("/getArticleCityList", s.requireSessionJSON(s.handleLegacySearchBuckets("city")))
	mux.HandleFunc("/getArticleCityList/", s.requireSessionJSON(s.handleLegacySearchBuckets("city")))
	mux.HandleFunc("/search", s.requireSession(s.handleLegacySearchRedirect("search")))
	mux.HandleFunc("/search/", s.requireSession(s.handleLegacySearchRedirect("search")))
	mux.HandleFunc("/fullsearch", s.requireSession(s.handleFullSearchEntry))
	mux.HandleFunc("/fullsearch/", s.requireSession(s.handleFullSearchCompat))
	mux.HandleFunc("/timelysearch", s.requireSession(s.handleTimelySearchEntry))
	mux.HandleFunc("/timelysearch/", s.requireSession(s.handleTimelySearchCompat))
	mux.HandleFunc("/mail/saveMailConfig", s.requireSessionJSON(s.handleLegacySaveMailConfig))
	mux.HandleFunc("/mail/checkMailConfig", s.requireSessionJSON(s.handleLegacyCheckMailConfig))
	mux.HandleFunc("/mail/getMailConfig", s.requireSessionJSON(s.handleLegacyGetMailConfig))
	mux.HandleFunc("/user/detail", s.requireSessionJSON(s.handleLegacyUserDetail))
	mux.HandleFunc("/user/edit", s.requireSessionJSON(s.handleLegacyUserEdit))
	mux.HandleFunc("/user/getwechatqrcode", s.requireSessionJSON(s.handleLegacyUserWechatQRCode))
	mux.HandleFunc("/user/", s.requireSession(s.handleUserCompat))
	mux.HandleFunc("/system/listSolutionGroupByUserId", s.requireSessionJSON(s.handleLegacyListSolutionGroupByUserID))
	mux.HandleFunc("/system/listProjectByGroupId", s.requireSessionJSON(s.handleLegacyListProjectByGroupID))
	mux.HandleFunc("/system/listProjectByUserId", s.requireSessionJSON(s.handleLegacyListProjectByUserID))
	mux.HandleFunc("/system/listWarning", s.requireSessionJSON(s.handleLegacyListWarning))
	mux.HandleFunc("/system/updateWarningStatusById", s.requireSessionJSON(s.handleLegacyUpdateWarningStatusByID))
	mux.HandleFunc("/system/getFavoriteList", s.requireSessionJSON(s.handleLegacyGetFavoriteList))
	mux.HandleFunc("/system/getWarningArticle", s.requireSessionJSON(s.handleLegacyGetWarningArticle))
	mux.HandleFunc("/system/warningSettingDetail", s.requireSessionJSON(s.handleLegacyWarningSettingDetail))
	mux.HandleFunc("/system/getOpinionConditionByProjectId", s.requireSessionJSON(s.handleLegacyGetOpinionConditionByProjectID))
	mux.HandleFunc("/system/updateOpinionCondition", s.requireSessionJSON(s.handleLegacyUpdateOpinionCondition))
	mux.HandleFunc("/system/getwords", s.requireSessionJSON(s.handleLegacyGetWarningWords))
	mux.HandleFunc("/system/updateWarning", s.requireSessionJSON(s.handleLegacyUpdateWarning))
	mux.HandleFunc("/system/getSystemTitle", s.requireSessionJSON(s.handleLegacyGetSystemTitle))
	mux.HandleFunc("/system/preference", s.requireSession(s.handleSystemSectionRedirect("preferences")))
	mux.HandleFunc("/system/favorite", s.requireSession(s.handleSystemSectionRedirect("favorites")))
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
	mux.HandleFunc("/popUp/needPopUp", s.requireSessionBool(s.handleLegacyNeedPopUp))
	mux.HandleFunc("/popUp/close", s.requireSessionBool(s.handleLegacyClosePopUp))
	mux.HandleFunc("/popUp/needContact", s.handleLegacyNeedContact)
	mux.HandleFunc("/popUp/closeContact", s.handleLegacyCloseContact)
	mux.HandleFunc("/datamonitor/updateemtion", s.requireSessionJSON(s.handleLegacyUpdateEmotion))
	mux.HandleFunc("/datamonitor/addfavoritedata", s.requireSessionJSON(s.handleLegacyAddFavorite))
	mux.HandleFunc("/datamonitor/isread", s.requireSessionJSON(s.handleLegacyReadState))
	mux.HandleFunc("/datamonitor/selectreadsign", s.requireSessionJSON(s.handleLegacySelectReadSign))
	mux.HandleFunc("/datamonitor/deletedata", s.requireSessionJSON(s.handleLegacyDeleteData))
	mux.HandleFunc("/datamonitor/copytext", s.requireSessionJSON(s.handleLegacyCopyText))
	mux.HandleFunc("/datamonitor/sending", s.requireSessionJSON(s.handleLegacySending))
	mux.HandleFunc("/projects/", s.requireSession(s.handleProjectDetail))
	mux.HandleFunc("/projects", s.requireSession(s.handleProjects))
	mux.HandleFunc("/monitor-rules/", s.requireSession(s.handleRuleDetail))
	mux.HandleFunc("/monitor-rules", s.requireSession(s.handleRules))
	mux.HandleFunc("/articles/", s.requireSession(s.handleArticleDetail))
	mux.HandleFunc("/articles", s.requireSession(s.handleArticles))
	mux.HandleFunc("/reports/", s.requireSession(s.handleReportDetail))
	mux.HandleFunc("/reports", s.requireSession(s.handleReports))
	mux.HandleFunc("/system", s.requireSession(s.handleSystem))
	mux.HandleFunc("/", s.requireSession(s.handleDashboard))
	return mux
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = s.render(w, "login", pageData{Title: "登录"})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			_ = s.render(w, "login", pageData{Title: "登录", Error: "表单解析失败"})
			return
		}
		loginResp, loginErr := s.authLogin(r.FormValue("username"), r.FormValue("password"))
		if loginErr != nil {
			_ = s.render(w, "login", pageData{Title: "登录", Error: loginErr.Error()})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    loginResp.SessionToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

func (s *Server) handleLegacySaveMailConfig(w http.ResponseWriter, r *http.Request, user any) {
	req, err := decodeLegacyMailConfigRequest(r)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "invalid body", map[string]any{})
		return
	}
	if strings.TrimSpace(req.Host) == "" || strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "邮箱配置不完整", map[string]any{})
		return
	}
	if req.Port <= 0 {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "SMTP端口无效", map[string]any{})
		return
	}

	stored, err := s.putMailConfig(model.MailConfig{
		Enabled:     true,
		SMTPHost:    req.Host,
		SMTPPort:    req.Port,
		Username:    req.Username,
		Password:    req.Password,
		SenderName:  nonEmpty(req.SenderName, "思通舆情"),
		SenderEmail: nonEmpty(req.To, req.Username),
	})
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), map[string]any{})
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", legacyMailConfigResponseFromStored(stored))
}

func (s *Server) handleLegacyCheckMailConfig(w http.ResponseWriter, r *http.Request, _ any) {
	cfg, err := s.getMailConfig()
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), map[string]any{})
		return
	}
	if !mailConfigConfigured(cfg) {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, "未配置邮件", map[string]any{})
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
}

func (s *Server) handleLegacyGetMailConfig(w http.ResponseWriter, r *http.Request, _ any) {
	cfg, err := s.getMailConfig()
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), map[string]any{})
		return
	}
	if !mailConfigConfigured(cfg) {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, "未配置邮件", map[string]any{})
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", legacyMailConfigResponseFromStored(cfg))
}

func (s *Server) handleLegacyNeedPopUp(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	key := legacyMobilePopupKey
	state, ok, err := s.getPopupState(userID, key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		if _, err := s.putPopupState(model.PopupState{UserID: userID, Key: key, Count: 0}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSONBool(w, true)
		return
	}
	if state.Dismissed && state.DismissedAt != nil && time.Since(*state.DismissedAt) < 24*time.Hour {
		writeJSONBool(w, false)
		return
	}
	writeJSONBool(w, state.Count < 5)
}

func (s *Server) handleLegacyClosePopUp(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	key := legacyMobilePopupKey
	state, ok, err := s.getPopupState(userID, key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		state = model.PopupState{UserID: userID, Key: key}
	}
	state.Count++
	state.Dismissed = true
	now := time.Now().UTC()
	state.DismissedAt = &now
	if _, err := s.putPopupState(state); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
}

func (s *Server) handleLegacyNeedContact(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("projectId")), 10, 64)
	if err != nil || projectID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	total, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("total")))
	if err != nil {
		total = 0
	}
	if total > 50 {
		now := time.Now().UTC()
		_, _ = s.putPopupState(model.PopupState{UserID: 0, Key: legacyContactPopupKey(projectID), Dismissed: true, DismissedAt: &now, Count: 0})
		writeJSONBool(w, false)
		return
	}
	state, ok, err := s.getPopupState(0, legacyContactPopupKey(projectID))
	if err != nil || !ok {
		writeJSONBool(w, false)
		return
	}
	writeJSONBool(w, !state.Dismissed)
}

func (s *Server) handleLegacyCloseContact(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("projectId")), 10, 64)
	if err != nil || projectID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	state, ok, err := s.getPopupState(0, legacyContactPopupKey(projectID))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		state = model.PopupState{UserID: 0, Key: legacyContactPopupKey(projectID)}
	}
	state.Dismissed = true
	now := time.Now().UTC()
	state.DismissedAt = &now
	if _, err := s.putPopupState(state); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
}

func (s *Server) handleLegacyUpdateEmotion(w http.ResponseWriter, r *http.Request, user any) {
	if _, err := legacyArticleIDFromRequest(r); err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "fail")
		return
	}
	writeDatamonitorJSON(w, http.StatusOK, "success")
}

func (s *Server) handleLegacyAddFavorite(w http.ResponseWriter, r *http.Request, user any) {
	itemID, err := legacyArticleIDFromRequest(r)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "fail")
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeDatamonitorJSON(w, http.StatusForbidden, "fail")
		return
	}
	item, err := s.fetchLegacyArticle(itemID, userID)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusInternalServerError, "fail")
		return
	}
	if !item.Favorited {
		if err := s.ensureLegacyFavorite(itemID, userID); err != nil {
			writeDatamonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
	}
	writeDatamonitorJSON(w, http.StatusOK, "success")
}

func (s *Server) handleLegacyReadState(w http.ResponseWriter, r *http.Request, user any) {
	itemID, err := legacyArticleIDFromRequest(r)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "fail")
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeDatamonitorJSON(w, http.StatusForbidden, "fail")
		return
	}
	flag, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("flag")))
	item, err := s.fetchLegacyArticle(itemID, userID)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusInternalServerError, "fail")
		return
	}
	switch flag {
	case 1:
		if item.Read {
			writeDatamonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
		if err := s.markLegacyRead(itemID, userID); err != nil {
			writeDatamonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
		writeDatamonitorJSON(w, http.StatusOK, "success")
	case 2:
		if err := s.unmarkLegacyRead(itemID, userID); err != nil {
			writeDatamonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
		writeDatamonitorJSON(w, http.StatusOK, "success")
	default:
		writeDatamonitorJSON(w, http.StatusBadRequest, "fail")
	}
}

func (s *Server) handleLegacySelectReadSign(w http.ResponseWriter, r *http.Request, user any) {
	itemID, err := legacyArticleIDFromRequest(r)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "err")
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeDatamonitorJSON(w, http.StatusForbidden, "err")
		return
	}
	item, err := s.fetchLegacyArticle(itemID, userID)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusInternalServerError, "err")
		return
	}
	if item.Read {
		writeDatamonitorJSON(w, http.StatusOK, "success")
		return
	}
	writeDatamonitorJSON(w, http.StatusInternalServerError, "err")
}

func (s *Server) handleLegacyDeleteData(w http.ResponseWriter, r *http.Request, user any) {
	if _, err := legacyArticleIDFromRequest(r); err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "fail")
		return
	}
	writeDatamonitorJSON(w, http.StatusOK, "success")
}

func (s *Server) handleLegacyCopyText(w http.ResponseWriter, r *http.Request, user any) {
	itemID, err := legacyArticleIDFromRequest(r)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "")
		return
	}
	userID := userIDFromMap(user)
	item, err := s.fetchLegacyArticle(itemID, userID)
	if err != nil {
		writeDatamonitorJSON(w, http.StatusInternalServerError, "")
		return
	}
	writeDatamonitorJSON(w, http.StatusOK, "标题："+item.Title+" 内容："+nonEmpty(item.Content, item.Summary))
}

func (s *Server) handleLegacySending(w http.ResponseWriter, r *http.Request, user any) {
	if _, err := legacyArticleIDFromRequest(r); err != nil {
		writeDatamonitorJSON(w, http.StatusBadRequest, "1")
		return
	}
	writeDatamonitorJSON(w, http.StatusOK, "1")
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

func (s *Server) handleLegacyUserDetail(w http.ResponseWriter, r *http.Request, user any) {
	mapped := legacyUserDetailPayload(user)
	writeLegacyJSON(w, http.StatusOK, "OK", mapped)
}

func (s *Server) handleLegacyUserEdit(w http.ResponseWriter, r *http.Request, user any) {
	if err := r.ParseForm(); err != nil {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "请求参数错误", nil)
		return
	}
	oldPassword := strings.TrimSpace(r.FormValue("oldPassword"))
	newPassword := strings.TrimSpace(r.FormValue("newPassword"))
	if oldPassword == "" || newPassword == "" {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "密码不能为空", nil)
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	token, ok := s.sessionTokenFromRequest(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	resp, err := s.client.R().
		SetQueryParam("session_token", token).
		SetBody(map[string]string{
			"old_password": oldPassword,
			"new_password": newPassword,
		}).
		Put(s.cfg.AuthURL + "/api/v1/users/" + strconv.FormatInt(userID, 10) + "/password")
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if resp.IsSuccess() {
		writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{})
		return
	}
	if resp.StatusCode() == http.StatusBadRequest && strings.Contains(strings.ToLower(resp.String()), "old password") {
		writeLegacyStatusJSON(w, 203, "旧密码输入错误！", map[string]any{})
		return
	}
	writeLegacyStatusJSON(w, 201, "密码修改失败！", map[string]any{})
}

func (s *Server) handleLegacyUserWechatQRCode(w http.ResponseWriter, r *http.Request, _ any) {
	resp, err := s.client.R().Get(s.cfg.AuthURL + "/api/v1/wechat/getQrCode")
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusBadGateway, err.Error(), nil)
		return
	}
	if !resp.IsSuccess() {
		writeLegacyStatusJSON(w, resp.StatusCode(), resp.Status(), nil)
		return
	}
	var envelope struct {
		Msg  string `json:"msg"`
		Data struct {
			QRCodeURL string `json:"qrcodeUrl"`
			SceneStr  string `json:"sceneStr"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "OK", map[string]any{
		"ticket":   envelope.Data.QRCodeURL,
		"sceneStr": envelope.Data.SceneStr,
	})
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
	projectID := strings.TrimSpace(r.FormValue("project_id"))
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

func (s *Server) handleLegacyGetSystemTitle(w http.ResponseWriter, r *http.Request, user any) {
	payload := map[string]any{
		"system_title": "网络情报分析系统",
	}
	if mapped, ok := user.(map[string]any); ok {
		if displayName := nonEmpty(legacyStringFromAny(mapped["display_name"]), legacyStringFromAny(mapped["username"])); displayName != "" {
			payload["user_name"] = displayName
		}
	}
	writeLegacyJSON(w, http.StatusOK, "OK", payload)
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

func writeDatamonitorJSON(w http.ResponseWriter, status int, result any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"result": result,
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

func legacyArticleIDFromRequest(r *http.Request) (int64, error) {
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		id = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if id == "" {
		return 0, errors.New("id required")
	}
	return strconv.ParseInt(id, 10, 64)
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

func mailConfigConfigured(cfg model.MailConfig) bool {
	return cfg.Enabled || strings.TrimSpace(cfg.SMTPHost) != "" || strings.TrimSpace(cfg.Username) != "" || strings.TrimSpace(cfg.Password) != "" || strings.TrimSpace(cfg.SenderEmail) != "" || strings.TrimSpace(cfg.SenderName) != ""
}

func legacyMailConfigResponseFromStored(cfg model.MailConfig) legacyMailConfigResponse {
	return legacyMailConfigResponse{
		Host:     cfg.SMTPHost,
		Port:     strconv.Itoa(cfg.SMTPPort),
		Username: cfg.Username,
		Password: cfg.Password,
		To:       nonEmpty(cfg.SenderEmail, cfg.Username),
		Cc:       nil,
		ToList:   nil,
	}
}

func (s *Server) fetchLegacyArticle(itemID, userID int64) (model.Item, error) {
	var item model.Item
	target := s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10)
	if userID > 0 {
		target += "?user_id=" + strconv.FormatInt(userID, 10)
	}
	if err := s.getJSON(target, &item); err != nil {
		return model.Item{}, err
	}
	return item, nil
}

func (s *Server) ensureLegacyFavorite(itemID, userID int64) error {
	item, err := s.fetchLegacyArticle(itemID, userID)
	if err != nil {
		return err
	}
	if item.Favorited {
		return nil
	}
	resp, err := s.client.R().
		SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
		Post(s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10) + "/favorite")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	return nil
}

func (s *Server) markLegacyRead(itemID, userID int64) error {
	resp, err := s.client.R().
		SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
		Post(s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10) + "/read")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	return nil
}

func (s *Server) unmarkLegacyRead(itemID, userID int64) error {
	resp, err := s.client.R().
		SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
		Delete(s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10) + "/read")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	return nil
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
		Services:  s.collectServiceStatuses(),
		Message:   r.URL.Query().Get("msg"),
	})
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
			if sourceType := strings.TrimSpace(r.FormValue("source_type")); sourceType != "" {
				req.SetQueryParam("source_type", sourceType)
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
		body := map[string]any{
			"project_id":       projectID,
			"name":             r.FormValue("name"),
			"include_keywords": r.FormValue("include_keywords"),
			"exclude_keywords": r.FormValue("exclude_keywords"),
			"channels":         r.FormValue("channels"),
			"severity":         r.FormValue("severity"),
			"status":           nonEmpty(r.FormValue("status"), "active"),
		}
		ruleID := r.FormValue("rule_id")
		switch action {
		case "update":
			resp, err := s.client.R().SetBody(body).Put(s.cfg.ContentURL + "/api/v1/monitor-rules/" + ruleID)
			if err != nil || !resp.IsSuccess() {
				message = "规则更新失败"
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
				message = "规则状态更新失败"
			}
		case "delete":
			resp, err := s.client.R().Delete(s.cfg.ContentURL + "/api/v1/monitor-rules/" + ruleID)
			if err != nil || !resp.IsSuccess() {
				message = "规则删除失败"
			} else {
				message = "规则已删除"
			}
		default:
			resp, err := s.client.R().SetBody(body).Post(s.cfg.ContentURL + "/api/v1/monitor-rules")
			if err != nil || !resp.IsSuccess() {
				message = "规则创建失败"
			}
		}
		redirectURL := localRedirectTarget(r.Referer())
		if strings.TrimSpace(redirectURL) == "" {
			redirectURL = "/monitor-rules"
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
				if sourceType := strings.TrimSpace(r.FormValue("source_type")); sourceType != "" {
					req.SetQueryParam("source_type", sourceType)
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
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/monitor-rules/"+id, &rule)
	if rule.ProjectID > 0 {
		projectID := strconv.FormatInt(rule.ProjectID, 10)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+projectID, &project)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=10&project_id="+projectID, &articles)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+projectID, &reports)
		_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
		_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	}
	_ = s.render(w, "rule", pageData{Title: "规则详情", User: user, Rule: rule, Project: project, Articles: articles, Reports: reports, CrawlRuns: crawlRuns, TaskRuns: taskRuns, Message: r.URL.Query().Get("msg")})
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		itemID := r.FormValue("item_id")
		action := r.FormValue("action")
		message := "文章操作失败"
		if itemID != "" && userID > 0 {
			switch action {
			case "favorite":
				resp, err := s.client.R().SetQueryParam("user_id", strconv.FormatInt(userID, 10)).Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/favorite")
				if err == nil && resp.IsSuccess() {
					message = "收藏状态已更新"
				}
			case "read":
				resp, err := s.client.R().SetQueryParam("user_id", strconv.FormatInt(userID, 10)).Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/read")
				if err == nil && resp.IsSuccess() {
					message = "文章已标记为已读"
				}
			case "share":
				resp, err := s.client.R().
					SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
					SetBody(map[string]string{"channel": "portal"}).
					Post(s.cfg.ContentURL + "/api/v1/articles/" + itemID + "/share")
				if err == nil && resp.IsSuccess() {
					message = "文章已登记分享"
				}
			}
		}
		redirectURL := localRedirectTarget(r.Referer())
		if strings.TrimSpace(redirectURL) == "" {
			redirectURL = "/articles"
		}
		http.Redirect(w, r, appendMessage(redirectURL, message), http.StatusSeeOther)
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
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
	query := "/api/v1/articles?page=1&page_size=200"
	if mode == "search" {
		query = "/api/v1/search/articles?page=1&page_size=200"
		if keyword != "" {
			query += "&q=" + keyword
		}
	} else if mode == "full" {
		query = "/api/v1/search/full?page=1&page_size=200"
		if keyword != "" {
			query += "&q=" + url.QueryEscape(keyword)
		}
	} else if mode == "timely" {
		query = "/api/v1/search/timely?page=1&page_size=200"
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
	if userID > 0 {
		query += "&user_id=" + strconv.FormatInt(userID, 10)
	}
	articles := model.ItemListResult{}
	projects := []model.Project{}
	options := model.SearchOptions{}
	_ = s.getJSON(s.cfg.ContentURL+query, &articles)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/search/options", &options)
	filteredItems := make([]model.Item, 0, len(articles.Items))
	readCount := 0
	unreadCount := 0
	flaggedCount := 0
	for _, item := range articles.Items {
		if readFilter == "read" && !item.Read {
			continue
		}
		if readFilter == "unread" && item.Read {
			continue
		}
		if flagFilter == "favorited" && !item.Favorited {
			continue
		}
		if flagFilter == "unfavorited" && item.Favorited {
			continue
		}
		if item.Read {
			readCount++
		} else {
			unreadCount++
		}
		if item.Favorited {
			flaggedCount++
		}
		filteredItems = append(filteredItems, item)
	}
	articles.Items = filteredItems
	if readFilter != "" || flagFilter != "" {
		articles.Total = len(filteredItems)
	}
	returnTo := url.QueryEscape(r.URL.RequestURI())
	_ = s.render(w, "articles", pageData{
		Title:          "文章中心",
		User:           user,
		Articles:       articles,
		Projects:       projects,
		ReturnTo:       returnTo,
		FilterKeyword:  keyword,
		FilterProject:  projectID,
		FilterRead:     readFilter,
		FilterFlag:     flagFilter,
		FilterSource:   sourceType,
		FilterStart:    start,
		FilterEnd:      end,
		FilterIndustry: industry,
		FilterProvince: province,
		FilterCity:     city,
		SearchMode:     mode,
		SearchOptions:  options,
		CountRead:      readCount,
		CountUnread:    unreadCount,
		CountFlagged:   flaggedCount,
		Message:        r.URL.Query().Get("msg"),
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
		message := "文章操作失败"
		switch action {
		case "favorite":
			resp, err := s.client.R().SetQueryParam("user_id", strconv.FormatInt(userID, 10)).Post(s.cfg.ContentURL + "/api/v1/articles/" + id + "/favorite")
			if err == nil && resp.IsSuccess() {
				message = "收藏状态已更新"
			}
		case "read":
			resp, err := s.client.R().SetQueryParam("user_id", strconv.FormatInt(userID, 10)).Post(s.cfg.ContentURL + "/api/v1/articles/" + id + "/read")
			if err == nil && resp.IsSuccess() {
				message = "文章已标记为已读"
			}
		case "share":
			resp, err := s.client.R().
				SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
				SetBody(map[string]string{"channel": "portal-detail"}).
				Post(s.cfg.ContentURL + "/api/v1/articles/" + id + "/share")
			if err == nil && resp.IsSuccess() {
				message = "文章已登记分享"
			}
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
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports/"+id, &report)
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if formType := r.FormValue("form_type"); formType == "crawl" || formType == "analysis" {
			message := "报告关联任务提交失败"
			switch formType {
			case "crawl":
				req := s.client.R()
				if sourceType := strings.TrimSpace(r.FormValue("source_type")); sourceType != "" {
					req.SetQueryParam("source_type", sourceType)
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

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		sectionKey = normalizeSystemSection(nonEmpty(r.FormValue("section"), sectionKey))
		projectID = nonEmpty(r.FormValue("project_id"), projectID)
		favoritePage = parsePositiveInt(r.FormValue("page"), favoritePage)
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
				"title":   r.FormValue("title"),
				"content": r.FormValue("content"),
			}).Post(s.cfg.ContentURL + "/api/v1/system/feedback")
			if err != nil || !resp.IsSuccess() {
				message = "反馈提交失败"
			} else {
				message = "反馈已提交"
			}
		case "crawl":
			req := s.client.R()
			if sourceType := strings.TrimSpace(r.FormValue("source_type")); sourceType != "" {
				req.SetQueryParam("source_type", sourceType)
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
		}
		target := url.Values{}
		target.Set("section", sectionKey)
		if projectID != "" {
			target.Set("project_id", projectID)
		}
		if favoritePage > 1 {
			target.Set("page", strconv.Itoa(favoritePage))
		}
		target.Set("msg", message)
		http.Redirect(w, r, "/system?"+target.Encode(), http.StatusSeeOther)
		return
	}

	notices := []model.SystemNotice{}
	taskRuns := []model.TaskRun{}
	crawlRuns := []model.CrawlRun{}
	preferences := model.UserPreference{}
	popupState := model.PopupState{}
	mailConfig := model.MailConfig{}
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
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=20", &taskRuns)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=20", &crawlRuns)
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
		"account":     "账号安全",
		"preferences": "偏好设置",
		"favorites":   "收藏夹",
		"warningmsg":  "预警消息",
		"warning":     "预警设置",
		"feedback":    "反馈建议",
	}[sectionKey]
	_ = s.render(w, "system", pageData{
		Title:                    "系统设置",
		User:                     user,
		SectionKey:               sectionKey,
		Notices:                  notices,
		TaskRuns:                 taskRuns,
		CrawlRuns:                crawlRuns,
		Projects:                 projects,
		ProjectNames:             projectNames,
		GroupNames:               groupNames,
		Services:                 s.collectServiceStatuses(),
		Preferences:              preferences,
		PopupState:               popupState,
		MailConfig:               mailConfig,
		WarningSetting:           warningSetting,
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
		Section:                  nonEmpty(sectionLabel, "系统工作台"),
		Message:                  r.URL.Query().Get("msg"),
	})
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

func (s *Server) render(w http.ResponseWriter, name string, data pageData) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return s.templates.ExecuteTemplate(w, name, data)
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

func normalizeSystemSection(section string) string {
	switch strings.TrimSpace(section) {
	case "preferences", "preference":
		return "preferences"
	case "favorites", "favorite":
		return "favorites"
	case "warningmsg", "warningmessage":
		return "warningmsg"
	case "warning", "warningedit":
		return "warning"
	case "feedback":
		return "feedback"
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

func legacyUserDetailPayload(user any) map[string]any {
	mapped, _ := user.(map[string]any)
	displayName := legacyStringFromAny(mapped["display_name"])
	username := legacyStringFromAny(mapped["username"])
	email := legacyStringFromAny(mapped["email"])
	status := legacyIntFromAnyValue(mapped["status"])
	if status == 0 {
		status = 1
	}
	updatedAt := legacyStringFromAny(mapped["updated_at"])
	if updatedAt == "" {
		updatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"username":          username,
		"display_name":      displayName,
		"telephone":         nonEmpty(legacyStringFromAny(mapped["telephone"]), username),
		"organization_name": legacyStringFromAny(mapped["organization_name"]),
		"email":             email,
		"status":            status,
		"login_count":       legacyIntFromAnyValue(mapped["login_count"]),
		"end_login_time":    nonEmpty(legacyStringFromAny(mapped["end_login_time"]), updatedAt),
		"role":              legacyStringFromAny(mapped["role"]),
		"system_title":      "网络情报分析系统",
		"updated_at":        updatedAt,
	}
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
		{Name: "auth-service", URL: s.cfg.AuthURL + "/healthz"},
		{Name: "content-service", URL: s.cfg.ContentURL + "/healthz"},
		{Name: "crawler-service", URL: s.cfg.CrawlerURL + "/healthz"},
		{Name: "analysis-service", URL: s.cfg.AnalysisURL + "/healthz"},
		{Name: "nlp-service", URL: s.cfg.NLPURL + "/healthz"},
	}
	for idx := range services {
		resp, err := s.client.R().Get(services[idx].URL)
		if err != nil {
			services[idx].Message = err.Error()
			continue
		}
		services[idx].Healthy = resp.IsSuccess()
		if resp.IsSuccess() {
			services[idx].Message = "ok"
		} else {
			services[idx].Message = resp.Status()
		}
	}
	return services
}

const layoutTemplate = `
{{define "nav"}}<nav><a href="/">总览</a><a href="/projects">项目</a><a href="/monitor-rules">规则</a><a href="/articles">文章</a><a href="/reports">报告</a><a href="/system">系统</a><a href="/logout">退出</a></nav>{{end}}
`

const baseStyles = `body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222}header,main{max-width:1180px;margin:0 auto;padding:24px}nav a{margin-right:16px;color:#214e34;text-decoration:none;font-weight:600}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}input,select,textarea,button{width:100%;padding:12px;margin:8px 0;border-radius:10px;border:1px solid #d0c8b8;box-sizing:border-box}button{background:#214e34;color:#fff;border:none;cursor:pointer}table{width:100%;border-collapse:collapse}th,td{padding:10px;border-bottom:1px solid #ece7dc;text-align:left}pre{white-space:pre-wrap;line-height:1.6}a.inline{margin-right:0;color:#214e34}form.inline{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;align-items:end}`

const loginTemplate = `
{{define "login"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `main{max-width:420px}</style></head><body><main><section><h1>Go 舆情系统</h1>{{if .Error}}<p style="color:#9b1c1c">{{.Error}}</p>{{end}}<form method="post"><input name="username" placeholder="用户名" value="admin"><input name="password" type="password" placeholder="密码" value="admin123"><button type="submit">登录</button></form></section></main></body></html>{{end}}
`

const dashboardTemplate = `
{{define "dashboard"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.metric-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.metric{padding:16px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.metric strong{display:block;font-size:28px;margin-top:6px}.toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.ok{color:#214e34;font-weight:700}.bad{color:#8f2d2d;font-weight:700}` + `</style></head><body><header><h1>总览</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><h2>核心指标</h2><form method="post"><button type="submit">手动刷新分析</button></form></div><div class="metric-grid"><div class="metric">文章数<strong>{{.Dashboard.Overview.ArticleCount}}</strong></div><div class="metric">项目数<strong>{{.Dashboard.Overview.ProjectCount}}</strong></div><div class="metric">报告数<strong>{{.Dashboard.Overview.ReportCount}}</strong></div><div class="metric">活跃规则<strong>{{.Dashboard.Overview.AlertRuleCount}}</strong></div></div></section><section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th><th>健康检查</th></tr>{{range .Services}}<tr><td>{{.Name}}</td><td>{{if .Healthy}}<span class="ok">正常</span>{{else}}<span class="bad">异常</span>{{end}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无服务状态</td></tr>{{end}}</table></section><section><h2>近 7 日趋势</h2><table><tr><th>日期</th><th>文章数</th></tr>{{range .Dashboard.Trends}}<tr><td>{{.Label}}</td><td>{{.Count}}</td></tr>{{end}}</table></section><section><h2>来源分布</h2><table><tr><th>来源</th><th>数量</th></tr>{{range .Dashboard.Sources}}<tr><td>{{.SourceType}}</td><td>{{.Count}}</td></tr>{{end}}</table></section><section><h2>关键词热点</h2><table><tr><th>关键词</th><th>次数</th></tr>{{range .Dashboard.Keywords}}<tr><td>{{.Keyword}}</td><td>{{.Count}}</td></tr>{{end}}</table></section><section><h2>最近抓取</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>系统公告</h2><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></section><section><h2>最近任务</h2><table><tr><th>任务</th><th>状态</th><th>开始时间</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无任务记录</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const projectsTemplate = `
{{define "projects"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.compact td form{margin:0}.compact input,.compact textarea,.compact select,.compact button{margin:4px 0;padding:8px}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>项目中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>列表搜索</h2><form class="inline" method="get"><input name="keyword" placeholder="搜索项目组、项目名、关键词、描述" value="{{.FilterKeyword}}"><select name="status"><option value="">全部状态</option><option value="active" {{if eq .FilterStatus "active"}}selected{{end}}>active</option><option value="paused" {{if eq .FilterStatus "paused"}}selected{{end}}>paused</option></select><button type="submit">搜索</button></form>{{if or .FilterKeyword .FilterStatus}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}} <a class="inline" href="/projects">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">项目组<strong>{{len .Groups}}</strong></div><div class="summary-card">项目<strong>{{len .Projects}}</strong></div><div class="summary-card">active<strong>{{.CountActive}}</strong></div><div class="summary-card">paused<strong>{{.CountPaused}}</strong></div></div></section><section><h2>新建项目组</h2><form method="post"><input type="hidden" name="form_type" value="group"><input name="name" placeholder="项目组名称"><textarea name="description" placeholder="项目组描述"></textarea><button type="submit">创建项目组</button></form></section><section><h2>项目组列表</h2><table class="compact"><tr><th>ID</th><th>名称</th><th>描述</th><th>操作</th></tr>{{range .Groups}}<tr><td>{{.ID}}</td><td><form method="post"><input type="hidden" name="form_type" value="group"><input type="hidden" name="action" value="update"><input type="hidden" name="group_id" value="{{.ID}}"><input name="name" value="{{.Name}}"></td><td><textarea name="description">{{.Description}}</textarea></td><td><button type="submit">保存</button></form><form method="post"><input type="hidden" name="form_type" value="group"><input type="hidden" name="action" value="delete"><input type="hidden" name="group_id" value="{{.ID}}"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="4">没有符合条件的项目组</td></tr>{{end}}</table></section><section><h2>新建项目</h2><form method="post"><input type="hidden" name="form_type" value="project"><select name="group_id">{{range .Groups}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><input name="name" placeholder="项目名称"><input name="keywords" placeholder="关键词，逗号分隔"><textarea name="description" placeholder="项目描述"></textarea><button type="submit">创建项目</button></form></section><section><h2>项目列表</h2><table class="compact"><tr><th>ID</th><th>项目组ID</th><th>项目组</th><th>名称</th><th>关键词</th><th>描述</th><th>状态</th><th>操作</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.ID}}</a></td><td><form method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="update"><input type="hidden" name="project_id" value="{{.ID}}"><input name="group_id" value="{{.GroupID}}"></td><td>{{.GroupName}}</td><td><input name="name" value="{{.Name}}"></td><td><input name="keywords" value="{{.Keywords}}"></td><td><textarea name="description">{{.Description}}</textarea></td><td><select name="status"><option value="active" {{if eq .Status "active"}}selected{{end}}>active</option><option value="paused" {{if eq .Status "paused"}}selected{{end}}>paused</option></select></td><td><button type="submit">保存</button></form><a class="inline" href="/projects/{{.ID}}">详情</a><a class="inline" href="/articles?project_id={{.ID}}">文章</a><a class="inline" href="/reports?project_id={{.ID}}">报告</a><form method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="delete"><input type="hidden" name="project_id" value="{{.ID}}"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="8">没有符合条件的项目</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const projectTemplate = `
{{define "project"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.danger button{background:#8f2d2d}` + `</style></head><body><header><h1>项目详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="/projects">返回项目中心</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看全部文章</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看全部报告</a></div><h2>{{.Project.Name}}</h2><p>项目组：{{.Project.GroupName}} | 状态：{{.Project.Status}}</p><p>关键词：{{.Project.Keywords}}</p><pre>{{.Project.Description}}</pre><div class="summary-grid"><div class="summary-card">关联规则<strong>{{len .Rules}}</strong></div><div class="summary-card">active 规则<strong>{{.CountActive}}</strong></div><div class="summary-card">paused 规则<strong>{{.CountPaused}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div><div class="summary-card">关联报告<strong>{{len .Reports}}</strong></div><div class="summary-card">generated 报告<strong>{{.CountGenerated}}</strong></div><div class="summary-card">draft 报告<strong>{{.CountDraft}}</strong></div><div class="summary-card">archived 报告<strong>{{.CountArchived}}</strong></div></div></section><section><h2>编辑项目</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="update"><select name="group_id">{{range .Groups}}<option value="{{.ID}}" {{if eq .ID $.Project.GroupID}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="name" value="{{.Project.Name}}" placeholder="项目名称"><input name="keywords" value="{{.Project.Keywords}}" placeholder="关键词"><select name="status"><option value="active" {{if eq .Project.Status "active"}}selected{{end}}>active</option><option value="paused" {{if eq .Project.Status "paused"}}selected{{end}}>paused</option></select><textarea name="description" placeholder="项目描述">{{.Project.Description}}</textarea><button type="submit">保存项目</button></form><form class="danger" method="post"><input type="hidden" name="form_type" value="project"><input type="hidden" name="action" value="delete"><button type="submit">删除项目</button></form></section><section><h2>快速任务</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type"><option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option></select><button type="submit">立即抓取</button></form><form method="post"><input type="hidden" name="form_type" value="analysis"><button type="submit">刷新分析</button></form></section><section><h2>快速创建规则</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="rule"><input name="name" placeholder="规则名称"><input name="include_keywords" value="{{.Project.Keywords}}" placeholder="包含关键词"><input name="exclude_keywords" placeholder="排除关键词"><input name="channels" placeholder="来源，如 flash,headline"><select name="severity"><option value="medium">medium</option><option value="high">high</option><option value="low">low</option></select><button type="submit">创建规则</button></form></section><section><h2>快速生成报告</h2><form method="post"><input type="hidden" name="form_type" value="report"><input name="title" value="{{.Project.Name}} 每日简报" placeholder="报告标题"><textarea name="content" placeholder="输入摘要、正文或人工分析内容"></textarea><button type="submit">生成报告</button></form></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>关联规则</h2><table><tr><th>名称</th><th>等级</th><th>状态</th><th>来源</th></tr>{{range .Rules}}<tr><td><a class="inline" href="/monitor-rules/{{.ID}}">{{.Name}}</a></td><td>{{.Severity}}</td><td>{{.Status}}</td><td>{{.Channels}}</td></tr>{{else}}<tr><td colspan="4">暂无关联规则</td></tr>{{end}}</table></section><section><h2>最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无文章</td></tr>{{end}}</table></section><section><h2>关联报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">暂无报告</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const rulesTemplate = `
{{define "rules"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.compact td form{margin:0}.compact input,.compact textarea,.compact select,.compact button{margin:4px 0;padding:8px}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>监测规则</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>列表搜索</h2><form class="inline" method="get"><input name="keyword" placeholder="搜索规则名、项目名、关键词、来源" value="{{.FilterKeyword}}"><select name="status"><option value="">全部状态</option><option value="active" {{if eq .FilterStatus "active"}}selected{{end}}>active</option><option value="paused" {{if eq .FilterStatus "paused"}}selected{{end}}>paused</option></select><button type="submit">搜索</button></form>{{if or .FilterKeyword .FilterStatus}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}} <a class="inline" href="/monitor-rules">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">规则<strong>{{len .Rules}}</strong></div><div class="summary-card">可选项目<strong>{{len .Projects}}</strong></div><div class="summary-card">active<strong>{{.CountActive}}</strong></div><div class="summary-card">paused<strong>{{.CountPaused}}</strong></div></div></section><section><h2>新建规则</h2><form method="post"><select name="project_id">{{range .Projects}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><input name="name" placeholder="规则名称"><input name="include_keywords" placeholder="包含关键词"><input name="exclude_keywords" placeholder="排除关键词"><input name="channels" placeholder="来源，如 flash,headline"><select name="severity"><option value="low">low</option><option value="medium">medium</option><option value="high">high</option></select><button type="submit">创建规则</button></form></section><section><h2>规则列表</h2><table class="compact"><tr><th>ID</th><th>项目ID</th><th>项目</th><th>名称</th><th>包含</th><th>排除</th><th>来源</th><th>等级</th><th>状态</th><th>操作</th></tr>{{range .Rules}}<tr><td><a class="inline" href="/monitor-rules/{{.ID}}">{{.ID}}</a></td><td><form method="post"><input type="hidden" name="action" value="update"><input type="hidden" name="rule_id" value="{{.ID}}"><input name="project_id" value="{{.ProjectID}}"></td><td>{{.ProjectName}}</td><td><input name="name" value="{{.Name}}"></td><td><input name="include_keywords" value="{{.IncludeKeywords}}"></td><td><input name="exclude_keywords" value="{{.ExcludeKeywords}}"></td><td><input name="channels" value="{{.Channels}}"></td><td><select name="severity"><option value="low" {{if eq .Severity "low"}}selected{{end}}>low</option><option value="medium" {{if eq .Severity "medium"}}selected{{end}}>medium</option><option value="high" {{if eq .Severity "high"}}selected{{end}}>high</option></select></td><td><select name="status"><option value="active" {{if eq .Status "active"}}selected{{end}}>active</option><option value="paused" {{if eq .Status "paused"}}selected{{end}}>paused</option></select></td><td><button type="submit">保存</button></form><a class="inline" href="/monitor-rules/{{.ID}}">详情</a><a class="inline" href="/articles?project_id={{.ProjectID}}">文章</a><a class="inline" href="/reports?project_id={{.ProjectID}}">报告</a><form method="post"><input type="hidden" name="action" value="toggle"><input type="hidden" name="rule_id" value="{{.ID}}"><input type="hidden" name="project_id" value="{{.ProjectID}}"><input type="hidden" name="name" value="{{.Name}}"><input type="hidden" name="include_keywords" value="{{.IncludeKeywords}}"><input type="hidden" name="exclude_keywords" value="{{.ExcludeKeywords}}"><input type="hidden" name="channels" value="{{.Channels}}"><input type="hidden" name="severity" value="{{.Severity}}"><input type="hidden" name="status" value="{{.Status}}"><button type="submit">{{if eq .Status "active"}}停用{{else}}启用{{end}}</button></form><form method="post"><input type="hidden" name="action" value="delete"><input type="hidden" name="rule_id" value="{{.ID}}"><button type="submit">删除</button></form></td></tr>{{else}}<tr><td colspan="10">没有符合条件的规则</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const ruleTemplate = `
{{define "rule"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.danger button{background:#8f2d2d}` + `</style></head><body><header><h1>规则详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="/monitor-rules">返回规则中心</a>{{if .Project.ID}}<a class="inline" href="/projects/{{.Project.ID}}">所属项目详情</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看项目文章</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看项目报告</a>{{end}}</div><h2>{{.Rule.Name}}</h2><p>项目：{{.Rule.ProjectName}} | 状态：{{.Rule.Status}} | 等级：{{.Rule.Severity}}</p><p>包含关键词：{{.Rule.IncludeKeywords}}</p><p>排除关键词：{{.Rule.ExcludeKeywords}}</p><p>来源：{{.Rule.Channels}}</p><div class="summary-grid"><div class="summary-card">所属项目<strong>{{if .Project.Name}}{{.Project.Name}}{{else}}未关联{{end}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div><div class="summary-card">最近报告<strong>{{len .Reports}}</strong></div></div></section><section><h2>编辑规则</h2><form class="inline" method="post"><input type="hidden" name="action" value="update"><input type="hidden" name="project_id" value="{{.Rule.ProjectID}}"><input type="hidden" name="status" value="{{.Rule.Status}}"><input name="name" value="{{.Rule.Name}}" placeholder="规则名称"><input name="include_keywords" value="{{.Rule.IncludeKeywords}}" placeholder="包含关键词"><input name="exclude_keywords" value="{{.Rule.ExcludeKeywords}}" placeholder="排除关键词"><input name="channels" value="{{.Rule.Channels}}" placeholder="来源"><select name="severity"><option value="low" {{if eq .Rule.Severity "low"}}selected{{end}}>low</option><option value="medium" {{if eq .Rule.Severity "medium"}}selected{{end}}>medium</option><option value="high" {{if eq .Rule.Severity "high"}}selected{{end}}>high</option></select><button type="submit">保存规则</button></form><form class="inline" method="post"><input type="hidden" name="action" value="toggle"><input type="hidden" name="project_id" value="{{.Rule.ProjectID}}"><input type="hidden" name="name" value="{{.Rule.Name}}"><input type="hidden" name="include_keywords" value="{{.Rule.IncludeKeywords}}"><input type="hidden" name="exclude_keywords" value="{{.Rule.ExcludeKeywords}}"><input type="hidden" name="channels" value="{{.Rule.Channels}}"><input type="hidden" name="severity" value="{{.Rule.Severity}}"><input type="hidden" name="status" value="{{.Rule.Status}}"><button type="submit">{{if eq .Rule.Status "active"}}停用规则{{else}}启用规则{{end}}</button></form><form class="danger" method="post"><input type="hidden" name="action" value="delete"><button type="submit">删除规则</button></form></section>{{if .Project.ID}}<section><h2>规则任务</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><input type="hidden" name="project_id" value="{{.Project.ID}}"><select name="source_type"><option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option></select><button type="submit">立即抓取</button></form><form method="post"><input type="hidden" name="form_type" value="analysis"><input type="hidden" name="project_id" value="{{.Project.ID}}"><button type="submit">刷新分析</button></form></section><section><h2>所属项目</h2><table><tr><th>项目</th><th>项目组</th><th>状态</th><th>关键词</th></tr><tr><td><a class="inline" href="/projects/{{.Project.ID}}">{{.Project.Name}}</a></td><td>{{.Project.GroupName}}</td><td>{{.Project.Status}}</td><td>{{.Project.Keywords}}</td></tr></table></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section>{{end}}<section><h2>关联项目最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Rule.ProjectID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无文章</td></tr>{{end}}</table></section><section><h2>关联项目最近报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Rule.ProjectID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">暂无报告</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const articlesTemplate = `
{{define "articles"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `main{max-width:1700px;font-size:14px}.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}.articles-table th,.articles-table td{font-size:14px;padding:8px}.articles-table .col-title{width:58%}.articles-table .col-source{width:7%}.articles-table .col-status{width:9%}.articles-table .col-time{width:12%}.articles-table .col-actions{width:14%}.articles-table .ops{white-space:nowrap}.articles-table .ops form{display:inline-block;width:auto;margin:0 6px 0 0;vertical-align:middle}.articles-table .ops form:last-child{margin-right:0}.articles-table .ops button{width:auto;margin:0;padding:9px 12px;font-size:13px;white-space:nowrap}` + `</style></head><body><header><h1>文章中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><form class="inline" method="get"><select name="mode"><option value="" {{if eq .SearchMode ""}}selected{{end}}>普通筛选</option><option value="search" {{if eq .SearchMode "search"}}selected{{end}}>基础全文</option><option value="full" {{if eq .SearchMode "full"}}selected{{end}}>高级检索</option><option value="timely" {{if eq .SearchMode "timely"}}selected{{end}}>实时搜索</option></select><input name="keyword" placeholder="关键词" value="{{.FilterKeyword}}"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><select name="source_type"><option value="">全部来源</option><option value="flash" {{if eq .FilterSource "flash"}}selected{{end}}>flash</option><option value="headline" {{if eq .FilterSource "headline"}}selected{{end}}>headline</option></select><select name="industry"><option value="">全部行业</option>{{range .SearchOptions.Industries}}<option value="{{.}}" {{if eq . $.FilterIndustry}}selected{{end}}>{{.}}</option>{{end}}</select><select name="province"><option value="">全部省份</option>{{range .SearchOptions.Provinces}}<option value="{{.}}" {{if eq . $.FilterProvince}}selected{{end}}>{{.}}</option>{{end}}</select><select name="city"><option value="">全部城市</option>{{range .SearchOptions.Cities}}<option value="{{.}}" {{if eq . $.FilterCity}}selected{{end}}>{{.}}</option>{{end}}</select><select name="read"><option value="">全部阅读状态</option><option value="read" {{if eq .FilterRead "read"}}selected{{end}}>已读</option><option value="unread" {{if eq .FilterRead "unread"}}selected{{end}}>未读</option></select><select name="favorite"><option value="">全部收藏状态</option><option value="favorited" {{if eq .FilterFlag "favorited"}}selected{{end}}>已收藏</option><option value="unfavorited" {{if eq .FilterFlag "unfavorited"}}selected{{end}}>未收藏</option></select><input type="date" name="start" value="{{.FilterStart}}"><input type="date" name="end" value="{{.FilterEnd}}"><button type="submit">筛选</button></form>{{if or .FilterKeyword .FilterProject .FilterSource .FilterRead .FilterFlag .FilterStart .FilterEnd .SearchMode .FilterIndustry .FilterProvince .FilterCity}}<p class="subtle">当前筛选已生效 <a class="inline" href="/articles">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">文章<strong>{{.Articles.Total}}</strong></div><div class="summary-card">已读<strong>{{.CountRead}}</strong></div><div class="summary-card">未读<strong>{{.CountUnread}}</strong></div><div class="summary-card">已收藏<strong>{{.CountFlagged}}</strong></div><div class="summary-card">模式<strong>{{if eq .SearchMode "search"}}基础全文{{else if eq .SearchMode "full"}}高级检索{{else if eq .SearchMode "timely"}}实时搜索{{else}}筛选{{end}}</strong></div></div></section><section><h2>{{if eq .SearchMode "search"}}全文搜索结果{{else if eq .SearchMode "full"}}高级检索结果{{else if eq .SearchMode "timely"}}实时搜索结果{{else}}列表{{end}}</h2><table class="articles-table"><tr><th class="col-title">标题</th><th class="col-source">来源</th><th class="col-status">状态</th><th class="col-time">时间</th><th class="col-actions">操作</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{if .Read}}<span class="pill">已读</span>{{else}}<span class="pill">未读</span>{{end}} {{if .Favorited}}<span class="pill">已收藏</span>{{end}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td><td class="ops"><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="favorite"><button type="submit">{{if .Favorited}}取消收藏{{else}}收藏{{end}}</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="share"><button type="submit">登记分享</button></form></td></tr>{{else}}<tr><td colspan="5">没有符合条件的文章</td></tr>{{end}}</table><p>共 {{.Articles.Total}} 条</p></section></main></body></html>{{end}}
`

const articleTemplate = `
{{define "article"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc;margin-right:8px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>文章详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="{{.ReturnURL}}">返回筛选结果</a><a class="inline" href="/articles">返回文章中心</a>{{if .Article.SourceURL}}<a class="inline" href="{{.Article.SourceURL}}" target="_blank" rel="noreferrer">原文链接</a>{{end}}{{if and .Article.DetailURL (ne .Article.DetailURL .Article.SourceURL)}}<a class="inline" href="{{.Article.DetailURL}}" target="_blank" rel="noreferrer">金十详情</a>{{end}}</div><h2>{{.Article.Title}}</h2><p>来源：{{.Article.SourceType}}{{if .Article.FromText}} | {{.Article.FromText}}{{end}} | 抓取时间：{{.Article.CapturedAt.Format "2006-01-02 15:04"}}</p>{{if .Article.Summary}}<p>摘要：{{.Article.Summary}}</p>{{end}}<p>{{if .Article.Read}}<span class="pill">已读</span>{{else}}<span class="pill">未读</span>{{end}}{{if .Article.Favorited}}<span class="pill">已收藏</span>{{end}}</p><div class="summary-grid"><div class="summary-card">关联项目<strong>{{len .Projects}}</strong></div><div class="summary-card">关联报告<strong>{{len .Reports}}</strong></div><div class="summary-card">generated 报告<strong>{{.CountGenerated}}</strong></div><div class="summary-card">draft 报告<strong>{{.CountDraft}}</strong></div><div class="summary-card">archived 报告<strong>{{.CountArchived}}</strong></div><div class="summary-card">相关文章<strong>{{len .Related}}</strong></div><div class="summary-card">相关文章已读<strong>{{.CountRead}}</strong></div><div class="summary-card">相关文章未读<strong>{{.CountUnread}}</strong></div><div class="summary-card">相关文章已收藏<strong>{{.CountFlagged}}</strong></div></div><form method="post"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post"><input type="hidden" name="action" value="favorite"><button type="submit">{{if .Article.Favorited}}取消收藏{{else}}收藏{{end}}</button></form><form method="post"><input type="hidden" name="action" value="share"><button type="submit">登记分享</button></form><pre>{{if .Article.Content}}{{.Article.Content}}{{else}}{{.Article.Summary}}{{end}}</pre></section>{{if .Projects}}<section><h2>项目联查</h2><table><tr><th>项目</th><th>状态</th><th>快捷入口</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.Name}}</a></td><td>{{.Status}}</td><td><a class="inline" href="/articles?project_id={{.ID}}">项目文章</a><a class="inline" href="/reports?project_id={{.ID}}">项目报告</a></td></tr>{{end}}</table></section><section><h2>关联项目</h2><table><tr><th>项目</th><th>项目组</th><th>状态</th><th>关键词</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.Name}}</a></td><td>{{.GroupName}}</td><td>{{.Status}}</td><td>{{.Keywords}}</td></tr>{{end}}</table></section>{{end}}{{if .Reports}}<section><h2>关联项目最近报告</h2><table><tr><th>ID</th><th>项目</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td>{{index $.ProjectNames .ProjectID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{.ProjectID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{end}}</table></section>{{end}}<section><h2>相关文章</h2><table><tr><th>标题</th><th>来源</th><th>状态</th></tr>{{range .Related}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{if .Read}}已读{{else}}未读{{end}}{{if .Favorited}} / 已收藏{{end}}</td></tr>{{else}}<tr><td colspan="3">暂无相关文章</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const reportsTemplate = `
{{define "reports"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>报告中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>生成报告</h2><form method="post"><select name="project_id">{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="title" placeholder="报告标题"><textarea name="content" placeholder="输入文章摘要、正文或人工内容"></textarea><button type="submit">生成报告</button></form></section><section><h2>报告筛选</h2><form class="inline" method="get"><input name="keyword" placeholder="标题关键词" value="{{.FilterKeyword}}"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><select name="status"><option value="">全部状态</option><option value="generated" {{if eq .FilterStatus "generated"}}selected{{end}}>generated</option><option value="draft" {{if eq .FilterStatus "draft"}}selected{{end}}>draft</option><option value="archived" {{if eq .FilterStatus "archived"}}selected{{end}}>archived</option></select><select name="order"><option value="desc" {{if eq .FilterSource "desc"}}selected{{end}}>更新时间倒序</option><option value="asc" {{if eq .FilterSource "asc"}}selected{{end}}>更新时间正序</option></select><button type="submit">筛选</button></form>{{if or .FilterKeyword .FilterProject .FilterStatus .FilterSource}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterProject}}，项目ID：{{.FilterProject}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}}{{if .FilterSource}}，排序：{{.FilterSource}}{{end}} <a class="inline" href="/reports">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">报告<strong>{{len .Reports}}</strong></div><div class="summary-card">draft<strong>{{.CountDraft}}</strong></div><div class="summary-card">generated<strong>{{.CountGenerated}}</strong></div><div class="summary-card">archived<strong>{{.CountArchived}}</strong></div><div class="summary-card">项目<strong>{{len .Projects}}</strong></div></div></section><section><h2>报告列表</h2><table><tr><th>ID</th><th>项目</th><th>标题</th><th>状态</th><th>更新时间</th><th>跳转</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td>{{index $.ProjectNames .ProjectID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td><td><a class="inline" href="/projects/{{.ProjectID}}">项目</a><a class="inline" href="/articles?project_id={{.ProjectID}}">文章</a></td></tr>{{else}}<tr><td colspan="6">没有符合条件的报告</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const reportTemplate = `
{{define "report"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>报告详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="{{.ReturnURL}}">返回筛选结果</a><a class="inline" href="/reports">返回报告中心</a>{{if .Project.ID}}<a class="inline" href="/projects/{{.Project.ID}}">返回所属项目</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看项目全部报告</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看项目文章</a>{{end}}</div><h2>{{.Report.Title}}</h2><p>状态：{{.Report.Status}} | 更新时间：{{.Report.UpdatedAt.Format "2006-01-02 15:04"}}</p>{{if .Project.ID}}<p>所属项目：<a class="inline" href="/projects/{{.Project.ID}}">{{.Project.Name}}</a></p><div class="summary-grid"><div class="summary-card">其他报告<strong>{{len .Reports}}</strong></div><div class="summary-card">draft<strong>{{.CountDraft}}</strong></div><div class="summary-card">generated<strong>{{.CountGenerated}}</strong></div><div class="summary-card">archived<strong>{{.CountArchived}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div></div>{{end}}<h3>摘要</h3><pre>{{.Report.Summary}}</pre><h3>正文</h3><pre>{{.Report.Content}}</pre></section>{{if .Project.ID}}<section><h2>同项目最近报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">除当前报告外暂无同项目报告</td></tr>{{end}}</table></section><section><h2>报告任务</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type"><option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option></select><button type="submit">立即抓取</button></form><form method="post"><input type="hidden" name="form_type" value="analysis"><button type="submit">刷新分析</button></form></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>所属项目最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无关联文章</td></tr>{{end}}</table></section>{{end}}<section><h2>重新生成报告</h2><form method="post"><input name="title" value="{{.Report.Title}} 重生成" placeholder="新报告标题"><textarea name="content" placeholder="可修改正文后重新生成">{{.Report.Content}}</textarea><button type="submit">重新生成</button></form></section></main></body></html>{{end}}
`

const systemTemplate = `
{{define "system"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.ok{color:#214e34;font-weight:700}.bad{color:#8f2d2d;font-weight:700}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px}.tabs{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:16px}.tabs a{padding:8px 12px;border-radius:999px;background:#efe9dc;color:#214e34;text-decoration:none}.tabs a.active{background:#214e34;color:#fff}.muted{color:#6a6257}.page-nav{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.page-nav a{padding:6px 10px;border:1px solid #d0c8b8;border-radius:8px;text-decoration:none;color:#214e34}.favorite-card,.warning-card{display:grid;grid-template-columns:2.2fr 1fr 1fr 1fr 1fr;gap:10px;align-items:center;padding:10px 0;border-bottom:1px solid #ece7dc}.warning-card{grid-template-columns:2.2fr 1fr 1fr 1fr 1fr 1fr}.section-block{margin-top:20px}` + `</style></head><body><header><h1>系统工作台</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="tabs"><a class="{{if eq .SectionKey "account"}}active{{end}}" href="/system?section=account">账号安全</a><a class="{{if eq .SectionKey "preferences"}}active{{end}}" href="/system?section=preferences">偏好设置</a><a class="{{if eq .SectionKey "favorites"}}active{{end}}" href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}">收藏夹</a><a class="{{if eq .SectionKey "warningmsg"}}active{{end}}" href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}">预警消息</a><a class="{{if eq .SectionKey "warning"}}active{{end}}" href="/system?section=warning{{if .WarningSetting.ProjectID}}&project_id={{.WarningSetting.ProjectID}}{{end}}">预警设置</a><a class="{{if eq .SectionKey "feedback"}}active{{end}}" href="/system?section=feedback">反馈建议</a></div><div class="muted">当前视图：{{.Section}}</div></section><section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th><th>健康检查</th></tr>{{range .Services}}<tr><td>{{.Name}}</td><td>{{if .Healthy}}<span class="ok">正常</span>{{else}}<span class="bad">异常</span>{{end}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无服务状态</td></tr>{{end}}</table></section>{{if eq .SectionKey "account"}}<section class="section-block"><h2>账号安全</h2><div class="grid"><div><h3>个人资料</h3><form method="post"><input type="hidden" name="form_type" value="profile"><input type="hidden" name="section" value="account"><input name="display_name" placeholder="显示名" value="{{index .User "display_name"}}"><input name="email" placeholder="邮箱" value="{{index .User "email"}}"><button type="submit">保存资料</button></form></div><div><h3>修改密码</h3><form method="post"><input type="hidden" name="form_type" value="password"><input type="hidden" name="section" value="account"><input type="password" name="old_password" placeholder="旧密码"><input type="password" name="new_password" placeholder="新密码"><button type="submit">修改密码</button></form></div></div></section>{{end}}{{if eq .SectionKey "preferences"}}<section class="section-block"><h2>偏好设置</h2><div class="grid"><div><h3>用户偏好</h3><form method="post"><input type="hidden" name="form_type" value="preferences"><input type="hidden" name="section" value="preferences"><input name="language" placeholder="语言" value="{{.Preferences.Language}}"><input name="theme" placeholder="主题" value="{{.Preferences.Theme}}"><input name="default_search_mode" placeholder="默认搜索模式" value="{{.Preferences.DefaultSearchMode}}"><input name="article_page_size" placeholder="文章分页大小" value="{{.Preferences.ArticlePageSize}}"><label><input type="checkbox" name="email_notifications" {{if .Preferences.EmailNotifications}}checked{{end}}> 邮件通知</label><button type="submit">保存偏好</button></form></div><div><h3>弹窗状态</h3><form method="post"><input type="hidden" name="form_type" value="popup"><input type="hidden" name="section" value="preferences"><input name="key" value="{{.PopupState.Key}}"><label><input type="checkbox" name="dismissed" {{if .PopupState.Dismissed}}checked{{end}}> 已关闭</label><button type="submit">保存弹窗状态</button></form></div><div><h3>邮件配置</h3><form method="post"><input type="hidden" name="form_type" value="mail"><input type="hidden" name="section" value="preferences"><label><input type="checkbox" name="enabled" {{if .MailConfig.Enabled}}checked{{end}}> 启用</label><input name="smtp_host" placeholder="SMTP Host" value="{{.MailConfig.SMTPHost}}"><input name="smtp_port" placeholder="SMTP Port" value="{{.MailConfig.SMTPPort}}"><input name="username" placeholder="用户名" value="{{.MailConfig.Username}}"><input name="password" placeholder="密码" value="{{.MailConfig.Password}}"><input name="sender_name" placeholder="发件人名称" value="{{.MailConfig.SenderName}}"><input name="sender_email" placeholder="发件人邮箱" value="{{.MailConfig.SenderEmail}}"><button type="submit">保存邮件配置</button></form></div></div></section>{{end}}{{if eq .SectionKey "favorites"}}<section class="section-block"><h2>收藏夹</h2><form class="inline" method="get"><input type="hidden" name="section" value="favorites"><select name="project_id">{{if not .FavoriteProjectID}}<option value="">全部项目</option>{{end}}{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FavoriteProjectID}}selected{{end}}>{{.Name}}</option>{{end}}</select><button type="submit">筛选</button></form><div class="page-nav">{{if gt .FavoriteTotalPages 1}}<a href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}&page={{.FavoritePagePrev}}">上一页</a><span>第 {{.FavoritePage}} / {{.FavoriteTotalPages}} 页</span><a href="/system?section=favorites{{if .FavoriteProjectID}}&project_id={{.FavoriteProjectID}}{{end}}&page={{.FavoritePageNext}}">下一页</a>{{else}}<span>共 {{len .FavoriteItems.Items}} 条收藏</span>{{end}}</div><div>{{range .FavoriteItems.Items}}<div class="favorite-card"><div><a class="inline" href="/monitor/detail/{{if .SourceKey}}{{.SourceKey}}{{else}}{{.ID}}{{end}}?groupid={{index $.GroupNames (firstProjectGroupID . $.Projects)}}&projectid={{firstProjectIDForItem . $.Projects}}">{{.Title}}</a></div><div>{{or .FromText .SourceType}}</div><div>{{index $.GroupNames (firstProjectGroupID . $.Projects)}}</div><div>{{index $.ProjectNames (firstProjectIDForItem . $.Projects)}}</div><div>{{.CapturedAt.Format "2006-01-02 15:04"}}</div></div>{{else}}<p class="muted">暂无收藏文章</p>{{end}}</div></section>{{end}}{{if eq .SectionKey "warningmsg"}}<section class="section-block"><h2>预警消息</h2><form class="inline" method="get"><input type="hidden" name="section" value="warningmsg"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.WarningArticleProjectID}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="keyword" placeholder="关键词" value="{{.WarningArticleKeyword}}"><select name="openFlag"><option value="0" {{if eq .WarningArticleOpenFlag 0}}selected{{end}}>全部</option><option value="1" {{if eq .WarningArticleOpenFlag 1}}selected{{end}}>仅开启预警</option></select><button type="submit">筛选</button></form><div class="page-nav">{{if gt .WarningArticleTotalPages 1}}<a href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}{{if ne .WarningArticleOpenFlag 0}}&openFlag={{.WarningArticleOpenFlag}}{{end}}&page={{.WarningArticlePrev}}">上一页</a><span>第 {{.WarningArticlePage}} / {{.WarningArticleTotalPages}} 页</span><a href="/system?section=warningmsg{{if .WarningArticleProjectID}}&project_id={{.WarningArticleProjectID}}{{end}}{{if .WarningArticleKeyword}}&keyword={{.WarningArticleKeyword}}{{end}}{{if ne .WarningArticleOpenFlag 0}}&openFlag={{.WarningArticleOpenFlag}}{{end}}&page={{.WarningArticleNext}}">下一页</a>{{else}}<span>共 {{len .WarningArticles}} 条消息</span>{{end}}</div><div>{{range .WarningArticles}}<div class="warning-card"><div><a class="inline" href="/monitor/detail/{{.ArticleID}}?groupid={{.GroupID}}&projectid={{.ProjectID}}" target="_blank">{{.ArticleTitle}}</a></div><div>{{.GroupName}}</div><div>{{.ProjectName}}</div><div>{{.ArticleTime}}</div><div>{{.GroupID}}</div><div>{{.ProjectID}}</div></div>{{else}}<p class="muted">暂无预警消息</p>{{end}}</div></section>{{end}}{{if eq .SectionKey "warning"}}<section class="section-block"><h2>预警设置</h2><form method="post"><input type="hidden" name="form_type" value="warning"><input type="hidden" name="section" value="warning"><select name="project_id">{{range .Projects}}<option value="{{.ID}}" {{if eq .ID $.WarningSetting.ProjectID}}selected{{end}}>{{.Name}}</option>{{end}}</select><label><input type="checkbox" name="enabled" {{if .WarningSetting.Enabled}}checked{{end}}> 启用</label><input name="channels" placeholder="渠道，逗号分隔" value="{{.WarningSetting.Channels}}"><input name="threshold" placeholder="阈值" value="{{.WarningSetting.Threshold}}"><input name="recipients" placeholder="接收人" value="{{.WarningSetting.Recipients}}"><textarea name="description" placeholder="说明">{{.WarningSetting.Description}}</textarea><button type="submit">保存预警设置</button></form></section>{{end}}{{if eq .SectionKey "feedback"}}<section class="section-block"><h2>反馈建议</h2><form method="post"><input type="hidden" name="form_type" value="feedback"><input type="hidden" name="section" value="feedback"><input name="title" placeholder="标题"><textarea name="content" placeholder="问题描述或需求"></textarea><button type="submit">提交</button></form></section>{{end}}<section class="section-block"><h2>运营操作</h2><div class="grid"><div><h3>抓取任务</h3><form method="post"><input type="hidden" name="form_type" value="crawl"><input type="hidden" name="section" value="{{.SectionKey}}"><select name="source_type"><option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option></select><button type="submit">立即抓取</button></form></div><div><h3>分析刷新</h3><form method="post"><input type="hidden" name="form_type" value="analysis"><input type="hidden" name="section" value="{{.SectionKey}}"><button type="submit">刷新分析快照</button></form></div></div></section><section class="section-block"><h2>公告与任务</h2><div class="grid"><div><h3>公告</h3><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></div><div><h3>任务记录</h3><table><tr><th>任务</th><th>状态</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无任务记录</td></tr>{{end}}</table></div><div><h3>抓取记录</h3><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></div></div></section></main></body></html>{{end}}
`
