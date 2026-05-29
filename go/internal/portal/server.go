package portal

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
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
	Title    string
	User     any
	Overview any
	Projects any
	Articles any
	Reports  any
	Notices  any
	Error    string
}

func NewServer(cfg config.Config) *Server {
	tpl := template.Must(template.New("layout").Parse(layoutTemplate))
	template.Must(tpl.New("login").Parse(loginTemplate))
	template.Must(tpl.New("dashboard").Parse(dashboardTemplate))
	template.Must(tpl.New("projects").Parse(projectsTemplate))
	template.Must(tpl.New("articles").Parse(articlesTemplate))
	template.Must(tpl.New("reports").Parse(reportsTemplate))

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
	mux.HandleFunc("/projects", s.requireSession(s.handleProjects))
	mux.HandleFunc("/articles", s.requireSession(s.handleArticles))
	mux.HandleFunc("/reports", s.requireSession(s.handleReports))
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
			Post(s.cfg.ContentURL + "/api/v1/auth/logout")
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, user any) {
	overview := map[string]any{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/analysis/overview", &overview)
	notices := []model.SystemNotice{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices)
	_ = s.render(w, "dashboard", pageData{Title: "总览", User: user, Overview: overview, Notices: notices})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			_, _ = s.client.R().
				SetBody(map[string]string{
					"name":        r.FormValue("name"),
					"keywords":    r.FormValue("keywords"),
					"description": r.FormValue("description"),
				}).
				Post(s.cfg.ContentURL + "/api/v1/projects")
		}
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}
	projects := []model.Project{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	_ = s.render(w, "projects", pageData{Title: "项目中心", User: user, Projects: projects})
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request, user any) {
	articles := map[string]any{}
	query := "/api/v1/articles?page=1&page_size=20"
	if keyword := strings.TrimSpace(r.URL.Query().Get("keyword")); keyword != "" {
		query += "&keyword=" + keyword
	}
	_ = s.getJSON(s.cfg.ContentURL+query, &articles)
	_ = s.render(w, "articles", pageData{Title: "文章中心", User: user, Articles: articles})
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			_, _ = s.client.R().
				SetBody(map[string]any{
					"project_id": 0,
					"title":      r.FormValue("title"),
					"text":       r.FormValue("content"),
				}).
				Post(s.cfg.ContentURL + "/api/v1/reports/generate")
		}
		http.Redirect(w, r, "/reports", http.StatusSeeOther)
		return
	}
	reports := []model.Report{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports", &reports)
	_ = s.render(w, "reports", pageData{Title: "报告中心", User: user, Reports: reports})
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
		Get(s.cfg.ContentURL + "/api/v1/auth/session")
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
		Post(s.cfg.ContentURL + "/api/v1/auth/login")
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

const layoutTemplate = `
{{define "nav"}}<nav><a href="/">总览</a><a href="/projects">项目中心</a><a href="/articles">文章中心</a><a href="/reports">报告中心</a><a href="/logout">退出</a></nav>{{end}}
`

const loginTemplate = `
{{define "login"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>body{font-family:Segoe UI,system-ui;background:#f4f0e8;margin:0;padding:40px;color:#1b1b1b}main{max-width:420px;margin:8vh auto;background:#fff;padding:32px;border-radius:16px;box-shadow:0 10px 30px rgba(0,0,0,.08)}input,button,textarea{width:100%;padding:12px;margin:8px 0;border-radius:10px;border:1px solid #d0c8b8}button{background:#214e34;color:#fff;border:none}small{color:#666}</style></head><body><main><h1>Go 舆情门户</h1>{{if .Error}}<p style="color:#9b1c1c">{{.Error}}</p>{{end}}<form method="post"><input name="username" placeholder="用户名" value="admin"><input name="password" type="password" placeholder="密码" value="admin123"><button type="submit">登录</button></form><small>默认账号由系统启动时自动写入 SQLite。</small></main></body></html>{{end}}
`

const dashboardTemplate = `
{{define "dashboard"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222}header,main{max-width:1080px;margin:0 auto;padding:24px}nav a{margin-right:16px;color:#214e34;text-decoration:none}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}pre{white-space:pre-wrap}</style></head><body><header><h1>Go 舆情门户</h1>{{template "nav" .}}</header><main><section><h2>系统总览</h2><pre>{{printf "%+v" .Overview}}</pre></section><section><h2>系统公告</h2><pre>{{printf "%+v" .Notices}}</pre></section></main></body></html>{{end}}
`

const projectsTemplate = `
{{define "projects"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222}header,main{max-width:1080px;margin:0 auto;padding:24px}nav a{margin-right:16px;color:#214e34;text-decoration:none}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}input,textarea,button{width:100%;padding:12px;margin:8px 0;border-radius:10px;border:1px solid #d0c8b8}button{background:#214e34;color:#fff;border:none}</style></head><body><header><h1>项目中心</h1>{{template "nav" .}}</header><main><section><h2>新建项目</h2><form method="post"><input name="name" placeholder="项目名称"><input name="keywords" placeholder="关键词，逗号分隔"><textarea name="description" placeholder="项目描述"></textarea><button type="submit">创建</button></form></section><section><h2>项目列表</h2><pre>{{printf "%+v" .Projects}}</pre></section></main></body></html>{{end}}
`

const articlesTemplate = `
{{define "articles"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222}header,main{max-width:1080px;margin:0 auto;padding:24px}nav a{margin-right:16px;color:#214e34;text-decoration:none}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}input,button{padding:12px;border-radius:10px;border:1px solid #d0c8b8}button{background:#214e34;color:#fff;border:none}</style></head><body><header><h1>文章中心</h1>{{template "nav" .}}</header><main><section><form method="get"><input name="keyword" placeholder="搜索标题或内容"><button type="submit">搜索</button></form></section><section><pre>{{printf "%+v" .Articles}}</pre></section></main></body></html>{{end}}
`

const reportsTemplate = `
{{define "reports"}}<!doctype html><html><head><meta charset="utf-8"><title>{{.Title}}</title><style>body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222}header,main{max-width:1080px;margin:0 auto;padding:24px}nav a{margin-right:16px;color:#214e34;text-decoration:none}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}input,textarea,button{width:100%;padding:12px;margin:8px 0;border-radius:10px;border:1px solid #d0c8b8}button{background:#214e34;color:#fff;border:none}</style></head><body><header><h1>报告中心</h1>{{template "nav" .}}</header><main><section><h2>生成报告</h2><form method="post"><input name="title" placeholder="报告标题"><textarea name="content" placeholder="输入待生成的报告正文"></textarea><button type="submit">生成</button></form></section><section><h2>报告列表</h2><pre>{{printf "%+v" .Reports}}</pre></section></main></body></html>{{end}}
`
