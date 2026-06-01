package portal

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

const sessionCookieName = "stonedt_portal_session"

type Server struct {
	cfg       config.Config
	client    *resty.Client
	templates *template.Template
}

type pageData struct {
	Title          string
	User           any
	Dashboard      model.DashboardSnapshot
	Groups         []model.ProjectGroup
	Project        model.Project
	Projects       []model.Project
	Rule           model.MonitorRule
	Rules          []model.MonitorRule
	Articles       model.ItemListResult
	Article        model.Item
	Related        []model.Item
	CrawlRuns      []model.CrawlRun
	Reports        []model.Report
	Report         model.Report
	Notices        []model.SystemNotice
	TaskRuns       []model.TaskRun
	Error          string
	Message        string
	ReturnTo       string
	ReturnURL      string
	FilterKeyword  string
	FilterProject  string
	FilterStatus   string
	FilterRead     string
	FilterFlag     string
	FilterSource   string
	FilterStart    string
	FilterEnd      string
	SearchMode     string
	Services       []serviceStatus
	ProjectNames   map[int64]string
	CountActive    int
	CountPaused    int
	CountRead      int
	CountUnread    int
	CountFlagged   int
	CountDraft     int
	CountGenerated int
	CountArchived  int
}

type serviceStatus struct {
	Name    string
	URL     string
	Healthy bool
	Message string
}

func NewServer(cfg config.Config) *Server {
	tpl := template.Must(template.New("layout").Parse(layoutTemplate))
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
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", s.handleLoginPage)
	mux.HandleFunc("/logout", s.handleLogout)
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
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/overview", &dashboard)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
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
	query := "/api/v1/articles?page=1&page_size=20"
	if mode == "search" {
		query = "/api/v1/search/articles?page=1&page_size=20"
		if keyword != "" {
			query += "&q=" + keyword
		}
	} else if keyword != "" {
		query += "&keyword=" + keyword
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
	if userID > 0 {
		query += "&user_id=" + strconv.FormatInt(userID, 10)
	}
	articles := model.ItemListResult{}
	projects := []model.Project{}
	_ = s.getJSON(s.cfg.ContentURL+query, &articles)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
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
	articles.Total = len(filteredItems)
	returnTo := url.QueryEscape(r.URL.RequestURI())
	_ = s.render(w, "articles", pageData{
		Title:         "文章中心",
		User:          user,
		Articles:      articles,
		Projects:      projects,
		ReturnTo:      returnTo,
		FilterKeyword: keyword,
		FilterProject: projectID,
		FilterRead:    readFilter,
		FilterFlag:    flagFilter,
		FilterSource:  sourceType,
		FilterStart:   start,
		FilterEnd:     end,
		SearchMode:    mode,
		CountRead:     readCount,
		CountUnread:   unreadCount,
		CountFlagged:  flaggedCount,
		Message:       r.URL.Query().Get("msg"),
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
	if len(reports) > 8 {
		reports = reports[:8]
	}
	projectNames := make(map[int64]string, len(projects))
	for _, project := range projects {
		projectNames[project.ID] = project.Name
	}
	_ = s.render(w, "article", pageData{Title: "文章详情", User: user, Article: article, Related: related, Projects: projects, Reports: reports, ProjectNames: projectNames, Message: r.URL.Query().Get("msg"), ReturnURL: returnURL, ReturnTo: url.QueryEscape(returnURL)})
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
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		message := "操作已提交"
		switch r.FormValue("form_type") {
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
			params := map[string]string{}
			if sourceType := strings.TrimSpace(r.FormValue("source_type")); sourceType != "" {
				params["source_type"] = sourceType
			}
			req := s.client.R()
			for key, value := range params {
				req.SetQueryParam(key, value)
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
		http.Redirect(w, r, "/system?msg="+message, http.StatusSeeOther)
		return
	}
	notices := []model.SystemNotice{}
	taskRuns := []model.TaskRun{}
	crawlRuns := []model.CrawlRun{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=20", &taskRuns)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=20", &crawlRuns)
	_ = s.render(w, "system", pageData{
		Title:     "系统设置",
		User:      user,
		Notices:   notices,
		TaskRuns:  taskRuns,
		CrawlRuns: crawlRuns,
		Services:  s.collectServiceStatuses(),
		Message:   r.URL.Query().Get("msg"),
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
{{define "articles"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>文章中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><form class="inline" method="get"><select name="mode"><option value="" {{if eq .SearchMode ""}}selected{{end}}>普通筛选</option><option value="search" {{if eq .SearchMode "search"}}selected{{end}}>全文搜索</option></select><input name="keyword" placeholder="关键词" value="{{.FilterKeyword}}"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><select name="source_type"><option value="">全部来源</option><option value="flash" {{if eq .FilterSource "flash"}}selected{{end}}>flash</option><option value="headline" {{if eq .FilterSource "headline"}}selected{{end}}>headline</option></select><select name="read"><option value="">全部阅读状态</option><option value="read" {{if eq .FilterRead "read"}}selected{{end}}>已读</option><option value="unread" {{if eq .FilterRead "unread"}}selected{{end}}>未读</option></select><select name="favorite"><option value="">全部收藏状态</option><option value="favorited" {{if eq .FilterFlag "favorited"}}selected{{end}}>已收藏</option><option value="unfavorited" {{if eq .FilterFlag "unfavorited"}}selected{{end}}>未收藏</option></select><input type="date" name="start" value="{{.FilterStart}}"><input type="date" name="end" value="{{.FilterEnd}}"><button type="submit">筛选</button></form>{{if or .FilterKeyword .FilterProject .FilterSource .FilterRead .FilterFlag .FilterStart .FilterEnd .SearchMode}}<p class="subtle">当前筛选已生效 <a class="inline" href="/articles">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">文章<strong>{{.Articles.Total}}</strong></div><div class="summary-card">已读<strong>{{.CountRead}}</strong></div><div class="summary-card">未读<strong>{{.CountUnread}}</strong></div><div class="summary-card">已收藏<strong>{{.CountFlagged}}</strong></div><div class="summary-card">模式<strong>{{if eq .SearchMode "search"}}全文{{else}}筛选{{end}}</strong></div></div></section><section><h2>{{if eq .SearchMode "search"}}全文搜索结果{{else}}列表{{end}}</h2><table><tr><th>标题</th><th>来源</th><th>状态</th><th>时间</th><th>操作</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{if .Read}}<span class="pill">已读</span>{{else}}<span class="pill">未读</span>{{end}} {{if .Favorited}}<span class="pill">已收藏</span>{{end}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td><td><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post"><input type="hidden" name="item_id" value="{{.ID}}"><input type="hidden" name="action" value="favorite"><button type="submit">{{if .Favorited}}取消收藏{{else}}收藏{{end}}</button></form></td></tr>{{else}}<tr><td colspan="5">没有符合条件的文章</td></tr>{{end}}</table><p>共 {{.Articles.Total}} 条</p></section></main></body></html>{{end}}
`

const articleTemplate = `
{{define "article"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.pill{display:inline-block;padding:4px 10px;border-radius:999px;background:#ece7dc;margin-right:8px}.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}` + `</style></head><body><header><h1>文章详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="{{.ReturnURL}}">返回筛选结果</a><a class="inline" href="/articles">返回文章中心</a></div><h2>{{.Article.Title}}</h2><p>来源：{{.Article.SourceType}} | 抓取时间：{{.Article.CapturedAt.Format "2006-01-02 15:04"}}</p><p>{{if .Article.Read}}<span class="pill">已读</span>{{else}}<span class="pill">未读</span>{{end}}{{if .Article.Favorited}}<span class="pill">已收藏</span>{{end}}</p><form method="post"><input type="hidden" name="action" value="read"><button type="submit">标记已读</button></form><form method="post"><input type="hidden" name="action" value="favorite"><button type="submit">{{if .Article.Favorited}}取消收藏{{else}}收藏{{end}}</button></form><pre>{{.Article.Content}}</pre></section>{{if .Projects}}<section><h2>关联项目</h2><table><tr><th>项目</th><th>项目组</th><th>状态</th><th>关键词</th></tr>{{range .Projects}}<tr><td><a class="inline" href="/projects/{{.ID}}">{{.Name}}</a></td><td>{{.GroupName}}</td><td>{{.Status}}</td><td>{{.Keywords}}</td></tr>{{end}}</table></section>{{end}}{{if .Reports}}<section><h2>关联项目最近报告</h2><table><tr><th>ID</th><th>项目</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td>{{index $.ProjectNames .ProjectID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{.ProjectID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{end}}</table></section>{{end}}<section><h2>相关文章</h2><table><tr><th>标题</th><th>来源</th><th>状态</th></tr>{{range .Related}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{if .Read}}已读{{else}}未读{{end}}{{if .Favorited}} / 已收藏{{end}}</td></tr>{{else}}<tr><td colspan="3">暂无相关文章</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const reportsTemplate = `
{{define "reports"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.subtle{color:#6a6257}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>报告中心</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>生成报告</h2><form method="post"><select name="project_id">{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><input name="title" placeholder="报告标题"><textarea name="content" placeholder="输入文章摘要、正文或人工内容"></textarea><button type="submit">生成报告</button></form></section><section><h2>报告筛选</h2><form class="inline" method="get"><input name="keyword" placeholder="标题关键词" value="{{.FilterKeyword}}"><select name="project_id"><option value="">全部项目</option>{{range .Projects}}<option value="{{.ID}}" {{if eq (printf "%d" .ID) $.FilterProject}}selected{{end}}>{{.Name}}</option>{{end}}</select><select name="status"><option value="">全部状态</option><option value="generated" {{if eq .FilterStatus "generated"}}selected{{end}}>generated</option><option value="draft" {{if eq .FilterStatus "draft"}}selected{{end}}>draft</option><option value="archived" {{if eq .FilterStatus "archived"}}selected{{end}}>archived</option></select><select name="order"><option value="desc" {{if eq .FilterSource "desc"}}selected{{end}}>更新时间倒序</option><option value="asc" {{if eq .FilterSource "asc"}}selected{{end}}>更新时间正序</option></select><button type="submit">筛选</button></form>{{if or .FilterKeyword .FilterProject .FilterStatus .FilterSource}}<p class="subtle">当前筛选已生效{{if .FilterKeyword}}，关键词：{{.FilterKeyword}}{{end}}{{if .FilterProject}}，项目ID：{{.FilterProject}}{{end}}{{if .FilterStatus}}，状态：{{.FilterStatus}}{{end}}{{if .FilterSource}}，排序：{{.FilterSource}}{{end}} <a class="inline" href="/reports">清空筛选</a></p>{{end}}</section><section><h2>当前结果</h2><div class="summary-grid"><div class="summary-card">报告<strong>{{len .Reports}}</strong></div><div class="summary-card">draft<strong>{{.CountDraft}}</strong></div><div class="summary-card">generated<strong>{{.CountGenerated}}</strong></div><div class="summary-card">archived<strong>{{.CountArchived}}</strong></div><div class="summary-card">项目<strong>{{len .Projects}}</strong></div></div></section><section><h2>报告列表</h2><table><tr><th>ID</th><th>项目</th><th>标题</th><th>状态</th><th>更新时间</th><th>跳转</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td>{{index $.ProjectNames .ProjectID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to={{$.ReturnTo}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td><td><a class="inline" href="/projects/{{.ProjectID}}">项目</a><a class="inline" href="/articles?project_id={{.ProjectID}}">文章</a></td></tr>{{else}}<tr><td colspan="6">没有符合条件的报告</td></tr>{{end}}</table></section></main></body></html>{{end}}
`

const reportTemplate = `
{{define "report"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.toolbar{display:flex;gap:12px;flex-wrap:wrap}.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.summary-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.summary-card strong{display:block;font-size:24px;margin-top:6px}` + `</style></head><body><header><h1>报告详情</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><div class="toolbar"><a class="inline" href="{{.ReturnURL}}">返回筛选结果</a><a class="inline" href="/reports">返回报告中心</a>{{if .Project.ID}}<a class="inline" href="/projects/{{.Project.ID}}">返回所属项目</a><a class="inline" href="/reports?project_id={{.Project.ID}}">查看项目全部报告</a><a class="inline" href="/articles?project_id={{.Project.ID}}">查看项目文章</a>{{end}}</div><h2>{{.Report.Title}}</h2><p>状态：{{.Report.Status}} | 更新时间：{{.Report.UpdatedAt.Format "2006-01-02 15:04"}}</p>{{if .Project.ID}}<p>所属项目：<a class="inline" href="/projects/{{.Project.ID}}">{{.Project.Name}}</a></p><div class="summary-grid"><div class="summary-card">其他报告<strong>{{len .Reports}}</strong></div><div class="summary-card">draft<strong>{{.CountDraft}}</strong></div><div class="summary-card">generated<strong>{{.CountGenerated}}</strong></div><div class="summary-card">archived<strong>{{.CountArchived}}</strong></div><div class="summary-card">最近文章<strong>{{len .Articles.Items}}</strong></div></div>{{end}}<h3>摘要</h3><pre>{{.Report.Summary}}</pre><h3>正文</h3><pre>{{.Report.Content}}</pre></section>{{if .Project.ID}}<section><h2>同项目最近报告</h2><table><tr><th>ID</th><th>标题</th><th>状态</th><th>更新时间</th></tr>{{range .Reports}}<tr><td>{{.ID}}</td><td><a class="inline" href="/reports/{{.ID}}?return_to=%2Freports%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.UpdatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="4">除当前报告外暂无同项目报告</td></tr>{{end}}</table></section><section><h2>报告任务</h2><form class="inline" method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type"><option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option></select><button type="submit">立即抓取</button></form><form method="post"><input type="hidden" name="form_type" value="analysis"><button type="submit">刷新分析</button></form></section><section><h2>最近任务记录</h2><table><tr><th>任务</th><th>状态</th><th>时间</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="4">暂无任务记录</td></tr>{{end}}</table></section><section><h2>最近抓取状态</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>所属项目最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>{{range .Articles.Items}}<tr><td><a class="inline" href="/articles/{{.ID}}?return_to=%2Farticles%3Fproject_id%3D{{$.Project.ID}}">{{.Title}}</a></td><td>{{.SourceType}}</td><td>{{.CapturedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="3">暂无关联文章</td></tr>{{end}}</table></section>{{end}}<section><h2>重新生成报告</h2><form method="post"><input name="title" value="{{.Report.Title}} 重生成" placeholder="新报告标题"><textarea name="content" placeholder="可修改正文后重新生成">{{.Report.Content}}</textarea><button type="submit">重新生成</button></form></section></main></body></html>{{end}}
`

const systemTemplate = `
{{define "system"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>` + baseStyles + `.msg{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}.ok{color:#214e34;font-weight:700}.bad{color:#8f2d2d;font-weight:700}` + `</style></head><body><header><h1>系统页</h1>{{template "nav" .}}</header><main>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th><th>健康检查</th></tr>{{range .Services}}<tr><td>{{.Name}}</td><td>{{if .Healthy}}<span class="ok">正常</span>{{else}}<span class="bad">异常</span>{{end}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无服务状态</td></tr>{{end}}</table></section><section><h2>手动任务</h2><form method="post"><input type="hidden" name="form_type" value="crawl"><select name="source_type"><option value="">全部来源</option><option value="flash">flash</option><option value="headline">headline</option></select><button type="submit">立即抓取</button></form><form method="post"><input type="hidden" name="form_type" value="analysis"><button type="submit">刷新分析快照</button></form></section><section><h2>提交反馈</h2><form method="post"><input type="hidden" name="form_type" value="feedback"><input name="title" placeholder="标题"><textarea name="content" placeholder="问题描述或需求"></textarea><button type="submit">提交</button></form></section><section><h2>最新抓取记录</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th><th>开始时间</th></tr>{{range .CrawlRuns}}<tr><td>{{.SourceType}}</td><td>{{.Status}}</td><td>{{.FetchedCount}}</td><td>{{.InsertedCount}}</td><td>{{.StartedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="5">暂无抓取记录</td></tr>{{end}}</table></section><section><h2>公告</h2><table><tr><th>标题</th><th>时间</th></tr>{{range .Notices}}<tr><td>{{.Title}}</td><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>{{else}}<tr><td colspan="2">暂无公告</td></tr>{{end}}</table></section><section><h2>任务记录</h2><table><tr><th>任务</th><th>状态</th><th>说明</th></tr>{{range .TaskRuns}}<tr><td>{{.TaskName}}</td><td>{{.Status}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3">暂无任务记录</td></tr>{{end}}</table></section></main></body></html>{{end}}
`
