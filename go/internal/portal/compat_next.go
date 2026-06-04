package portal

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Server) handleCaptchaCode(w http.ResponseWriter, r *http.Request) {
	code := randomDigits(4)
	http.SetCookie(w, &http.Cookie{
		Name:     "stonedt_captcha",
		Value:    code,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="132" height="44" viewBox="0 0 132 44"><rect width="132" height="44" rx="6" fill="#f6f3ea"/><text x="50%%" y="60%%" dominant-baseline="middle" text-anchor="middle" font-family="Segoe UI,Arial" font-size="24" fill="#214e34">%s</text></svg>`, html.EscapeString(code))
}

func (s *Server) handleDisplayBoard(w http.ResponseWriter, r *http.Request, user any) {
	dashboard, notices, taskRuns, crawlRuns, services := s.loadDashboardContext(r)
	groupID := strings.TrimSpace(r.URL.Query().Get("groupid"))
	projectID := strings.TrimSpace(r.URL.Query().Get("projectid"))
	_ = s.render(w, "dashboard", pageData{
		Title:     "综合看板",
		User:      user,
		Dashboard: dashboard,
		Notices:   notices,
		TaskRuns:  taskRuns,
		CrawlRuns: crawlRuns,
		Services:  services,
		Message:   "groupid=" + groupID + " projectid=" + projectID,
	})
}

func (s *Server) handleDisplayBoardCollection2(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	var items model.ItemListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles?favorite=favorited&page=1&page_size=20&user_id="+strconv.FormatInt(userID, 10), &items); err != nil {
		writeLegacyStatusJSON(w, http.StatusOK, "ok", map[string]any{"data": []any{}})
		return
	}
	writeLegacyStatusJSON(w, http.StatusOK, "ok", map[string]any{"data": items.Items})
}

func (s *Server) handleMobileMonitor(w http.ResponseWriter, r *http.Request, user any) {
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	s.writeSimplePage(w, "mobile/monitor", "移动端监测", s.mobileShell(user, "monitor", r.URL.Query()))
}

func (s *Server) handleMobileMonitorDetail(w http.ResponseWriter, r *http.Request, user any) {
	s.writeSimplePage(w, "mobile/detail", "移动端详情", s.mobileShell(user, "detail", r.URL.Query()))
}

func (s *Server) handleMobileWarning(w http.ResponseWriter, r *http.Request, user any) {
	s.writeSimplePage(w, "mobile/warning", "移动端预警", s.mobileShell(user, "warning", r.URL.Query()))
}

func (s *Server) handleMobileGetGroupAndProject(w http.ResponseWriter, r *http.Request, user any) {
	groups := s.groupProjectsForMobile()
	writeLegacyStatusJSON(w, http.StatusOK, "用户方案和方案组返回成功", map[string]any{"data": groups})
}

func (s *Server) handleMobileQRCode(w http.ResponseWriter, r *http.Request, user any) {
	token, ok := s.sessionTokenFromRequest(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", map[string]any{})
		return
	}
	uuid := fmt.Sprintf("%d-%d", userIDFromMap(user), time.Now().UnixMilli())
	key := sha1Hex(s.cfg.ServiceToken + uuid)
	s.mu.Lock()
	s.mobileQRs[uuid] = mobileQRCodeState{Token: token, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	s.mu.Unlock()
	qrURL := strings.TrimRight(s.cfg.GatewayWebURL, "/") + "/mobile/uuid/" + url.PathEscape(uuid) + "/" + url.PathEscape(key)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="250" height="250" viewBox="0 0 250 250"><rect width="250" height="250" rx="16" fill="#f6f3ea"/><rect x="16" y="16" width="218" height="218" rx="12" fill="#fff" stroke="#214e34" stroke-width="2"/><text x="125" y="114" text-anchor="middle" font-family="Segoe UI,Arial" font-size="18" fill="#214e34">Mobile QR</text><text x="125" y="144" text-anchor="middle" font-family="Segoe UI,Arial" font-size="12" fill="#6a6257">%s</text></svg>`, html.EscapeString(qrURL))
}

func (s *Server) handleMobileUUID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/mobile/uuid/"), "/"), "/")
	if len(parts) < 2 {
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	uuid, key := parts[0], parts[1]
	if sha1Hex(s.cfg.ServiceToken+uuid) != key {
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	s.mu.Lock()
	state, ok := s.mobileQRs[uuid]
	if ok && time.Now().UTC().After(state.ExpiresAt) {
		delete(s.mobileQRs, uuid)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: state.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/mobile/monitor?token="+url.QueryEscape(state.Token), http.StatusSeeOther)
}

func (s *Server) handleVolume(w http.ResponseWriter, r *http.Request, user any) {
	groupID := strings.TrimSpace(r.URL.Query().Get("groupid"))
	projectID := strings.TrimSpace(r.URL.Query().Get("projectid"))
	_ = s.writeSimplePage(w, "volume", "声量监测", fmt.Sprintf(`<h1>声量监测</h1><p>groupid=%s projectid=%s</p><p><a href="/system">系统页</a></p>`, html.EscapeString(groupID), html.EscapeString(projectID)))
}

func (s *Server) handleVolumeGetProject(w http.ResponseWriter, r *http.Request, user any) {
	_ = r.ParseForm()
	projectID := firstNonEmpty(r.FormValue("projectid"), r.URL.Query().Get("projectid"))
	timePeriod := firstNonEmpty(r.FormValue("time_period"), r.URL.Query().Get("time_period"))
	var projects []model.Project
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	var projectName, groupName string
	for _, project := range projects {
		if strconv.FormatInt(project.ID, 10) == projectID {
			projectName = project.Name
			groupName = project.GroupName
			break
		}
	}
	emotions := model.EmotionAnalysis{}
	events := []model.EventOverview{}
	propagation := model.PropagationAnalysis{}
	themes := []model.ThemeInsight{}
	hotspots := []model.KeywordHotspot{}
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/emotions?project_id="+url.QueryEscape(projectID), &emotions)
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/event-overview?project_id="+url.QueryEscape(projectID), &events)
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/propagation?project_id="+url.QueryEscape(projectID), &propagation)
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/themes?project_id="+url.QueryEscape(projectID), &themes)
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/keywords", &hotspots)
	payload := map[string]any{
		"code": 200,
		"msg":  "ok",
		"data": map[string]any{
			"project_name":                projectName,
			"group_name":                  groupName,
			"time_period":                 timePeriod,
			"keywordsMood":                map[string]any{"china": summarizeAnalysisEmotions(emotions), "list": emotions.Buckets},
			"biaoge":                      map[string]any{"china": "Go 迁移版已接管", "list": themes},
			"keywordsLine":                propagation.Trend,
			"keyword_news_rank":           events,
			"highword_cloud":              hotspots,
			"keyword_exposure_rank":       hotspots,
			"media_user_volume_rank":      events,
			"keyword_emotion_stat":        emotions,
			"keyword_emotion_trend":       propagation.Trend,
			"keyword_source_distribution": hotspots,
		},
	}
	writeJSONText(w, payload)
}

func (s *Server) handleVolumeProjectName(w http.ResponseWriter, r *http.Request, user any) {
	_ = r.ParseForm()
	projectID := firstNonEmpty(r.FormValue("projectid"), r.URL.Query().Get("projectid"))
	groupID := firstNonEmpty(r.FormValue("groupId"), r.URL.Query().Get("groupId"))
	var projects []model.Project
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	for _, project := range projects {
		if strconv.FormatInt(project.ID, 10) == projectID {
			writeJSONText(w, map[string]any{"project_name": project.Name, "group_name": project.GroupName, "project_id": projectID, "group_id": groupID})
			return
		}
	}
	writeJSONText(w, map[string]any{"project_name": "", "group_name": "", "project_id": projectID, "group_id": groupID})
}

func (s *Server) handleHotPage(w http.ResponseWriter, r *http.Request, user any) {
	limit := parsePositiveInt(firstNonEmpty(r.URL.Query().Get("limit"), "20"), 20)
	if limit > 50 {
		limit = 50
	}
	var hotspots []model.KeywordHotspot
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/keywords", &hotspots)
	if len(hotspots) == 0 {
		hotspots = []model.KeywordHotspot{{Keyword: "暂无数据", Count: 0}}
	}
	if len(hotspots) > limit {
		hotspots = hotspots[:limit]
	}
	var b strings.Builder
	b.WriteString("<h1>热点数据</h1>")
	b.WriteString("<p>来源：analysis-service 的关键词聚合结果。</p>")
	b.WriteString(`<form class="inline" method="get"><input name="limit" type="number" min="1" max="50" value="`)
	b.WriteString(strconv.Itoa(limit))
	b.WriteString(`"><button type="submit">刷新</button></form>`)
	b.WriteString(`<table><tr><th>关键词</th><th>热度</th><th>跳转</th></tr>`)
	for _, item := range hotspots {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(item.Keyword))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(item.Count))
		b.WriteString("</td><td><a href=\"/articles?mode=full&keyword=")
		b.WriteString(url.QueryEscape(item.Keyword))
		b.WriteString("\">查看相关文章</a></td></tr>")
	}
	b.WriteString("</table>")
	b.WriteString(`<p><a href="/hot/hotlist">JSON 接口</a> | <a href="/articles?mode=full">全文检索</a></p>`)
	_ = s.writeSimplePage(w, "hot", "热点数据", b.String())
}

func (s *Server) handleHotList(w http.ResponseWriter, r *http.Request, user any) {
	var hotspots []model.KeywordHotspot
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/keywords", &hotspots)
	if len(hotspots) == 0 {
		hotspots = append(hotspots, model.KeywordHotspot{Keyword: "暂无数据", Count: 0})
	}
	writeJSONText(w, map[string]any{"code": 200, "msg": "ok", "data": hotspots})
}

func (s *Server) handleDistMonitor(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
}

func (s *Server) handleDistGetData(w http.ResponseWriter, r *http.Request) {
	openid := strings.TrimSpace(r.URL.Query().Get("openid"))
	if openid == "" {
		s.writeSimplePage(w, "userapply", "申请试用", `<h1>申请试用</h1><p>请通过表单提交申请。</p>`)
		return
	}
	if strings.EqualFold(r.URL.Query().Get("approved"), "true") {
		http.Redirect(w, r, "/dist/yqmontitor", http.StatusSeeOther)
		return
	}
	_ = s.writeSimplePage(w, "userapply", "申请试用", `<h1>申请试用</h1><p>openid=`+html.EscapeString(openid)+`</p><p>申请流程已迁移为 Go 兼容页。</p>`)
}

func (s *Server) handleDistApply(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dist/yqapply", http.StatusSeeOther)
}

func (s *Server) handleDistYqApply(w http.ResponseWriter, r *http.Request) {
	_ = s.writeSimplePage(w, "userapply", "申请试用", `<h1>申请试用</h1><p>Go 兼容版申请页。</p>`)
}

func (s *Server) handleDistApplyDataInfo(w http.ResponseWriter, r *http.Request) {
	writeJSONText(w, map[string]any{"code": 200})
}

func (s *Server) handleDistYqMonitor(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
}

func (s *Server) handleDistHotData(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/hot/hotpage", http.StatusSeeOther)
}

func (s *Server) loadDashboardContext(r *http.Request) (model.DashboardSnapshot, []model.SystemNotice, []model.TaskRun, []model.CrawlRun, []serviceStatus) {
	dashboard := model.DashboardSnapshot{}
	notices := []model.SystemNotice{}
	taskRuns := []model.TaskRun{}
	crawlRuns := []model.CrawlRun{}
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/overview", &dashboard)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/notices", &notices)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/system/task-runs?limit=10", &taskRuns)
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &crawlRuns)
	return dashboard, notices, taskRuns, crawlRuns, s.collectServiceStatuses()
}

func (s *Server) groupProjectsForMobile() []map[string][]map[string]any {
	var projects []model.Project
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	groupMap := map[string][]map[string]any{}
	for _, project := range projects {
		key := fmt.Sprintf("%d-%s", project.GroupID, project.GroupName)
		groupMap[key] = append(groupMap[key], map[string]any{
			"group_id":     strconv.FormatInt(project.GroupID, 10),
			"group_name":   project.GroupName,
			"project_id":   strconv.FormatInt(project.ID, 10),
			"project_name": project.Name,
			"keywords":     project.Keywords,
			"status":       project.Status,
		})
	}
	result := make([]map[string][]map[string]any, 0, len(groupMap))
	for key, list := range groupMap {
		result = append(result, map[string][]map[string]any{key: list})
	}
	return result
}

func (s *Server) mobileShell(user any, page string, query url.Values) string {
	return fmt.Sprintf(`<h1>%s</h1><p>欢迎，%v</p><p><a href="/mobile/getGroupAndProject">分组项目</a></p><p><a href="/mobile/mobileQRCode">二维码</a></p>`, html.EscapeString(page), userIDFromMap(user))
}

func (s *Server) writeSimplePage(w http.ResponseWriter, _ string, title string, body string) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := fmt.Fprintf(w, "<!doctype html><html><head><meta charset='utf-8'><title>%s</title><style>body{font-family:Segoe UI,system-ui;background:#f7f3eb;margin:0;color:#222}main{max-width:1100px;margin:0 auto;padding:24px}section{background:#fff;border-radius:16px;padding:20px;margin-top:20px;box-shadow:0 8px 24px rgba(0,0,0,.06)}a{color:#214e34;text-decoration:none}</style></head><body><main>%s</main></body></html>", html.EscapeString(title), body)
	return err
}

func writeJSONText(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func summarizeAnalysisEmotions(emotions model.EmotionAnalysis) string {
	if emotions.Total <= 0 {
		return "暂无数据"
	}
	return fmt.Sprintf("项目共有 %d 条情感样本", emotions.Total)
}

func randomDigits(length int) string {
	if length <= 0 {
		length = 4
	}
	b := make([]byte, length)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = byte('0' + (b[i] % 10))
	}
	return string(b)
}

func sha1Hex(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}
