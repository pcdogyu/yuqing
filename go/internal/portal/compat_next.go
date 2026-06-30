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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
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
	projects := []model.Project{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	articles := model.ItemListResult{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=12", &articles)
	boardArticles := model.ItemListResult{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=50", &boardArticles)
	hotspots := []model.KeywordHotspot{}
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/keywords", &hotspots)
	returnTo := "/displayboard"
	if groupID != "" || projectID != "" {
		values := url.Values{}
		if groupID != "" {
			values.Set("groupid", groupID)
		}
		if projectID != "" {
			values.Set("projectid", projectID)
		}
		returnTo = "/displayboard?" + values.Encode()
	}
	var b strings.Builder
	b.WriteString(`<section class="hero"><div><h1>综合看板</h1><p>Go 版已接管旧 Java 大屏的核心数据能力，继续保留共享顶部菜单与统一门户体验。</p></div>`)
	b.WriteString(`<div class="hero-meta"><span>更新时间：`)
	if dashboard.UpdatedAt.IsZero() {
		b.WriteString("暂无")
	} else {
		b.WriteString(dashboard.UpdatedAt.Format("2006-01-02 15:04"))
	}
	b.WriteString(`</span><span>总览页面：<a class="inline" href="/analysis">监测分析</a></span></div></section>`)
	b.WriteString("<h1>综合看板</h1>")
	b.WriteString(`<p><a href="/projects">项目中心</a> | <a href="/articles">文章中心</a> | <a href="/reports">报告中心</a> | <a href="/system">系统工作台</a></p>`)
	b.WriteString(`<section><form class="inline" method="get"><select name="groupid"><option value="">全部项目组</option>`)
	for _, project := range projects {
		b.WriteString(`<option value="`)
		b.WriteString(strconv.FormatInt(project.GroupID, 10))
		b.WriteString(`"`)
		if groupID == strconv.FormatInt(project.GroupID, 10) {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(project.GroupName))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select><select name="projectid"><option value="">全部项目</option>`)
	for _, project := range projects {
		b.WriteString(`<option value="`)
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString(`"`)
		if projectID == strconv.FormatInt(project.ID, 10) {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(project.Name))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select><button type="submit">切换范围</button></form>`)
	if projectID != "" {
		b.WriteString(`<p class="muted">当前项目ID：`)
		b.WriteString(html.EscapeString(projectID))
		b.WriteString(` | <a class="inline" href="/projects/`)
		b.WriteString(html.EscapeString(projectID))
		b.WriteString(`">项目详情</a></p>`)
	}
	b.WriteString(`</section>`)
	b.WriteString(`<section><h2>核心指标</h2><div class="metric-row">`)
	b.WriteString(metricCard("文章数", strconv.Itoa(dashboard.Overview.ArticleCount)))
	b.WriteString(metricCard("项目数", strconv.Itoa(dashboard.Overview.ProjectCount)))
	b.WriteString(metricCard("报告数", strconv.Itoa(dashboard.Overview.ReportCount)))
	b.WriteString(metricCard("活跃规则", strconv.Itoa(dashboard.Overview.AlertRuleCount)))
	b.WriteString(`</div></section>`)
	b.WriteString(`<section><div class="dashboard-grid"><div class="dashboard-col">`)
	b.WriteString(`<div class="section-card"><h2>7日趋势</h2>`)
	if len(dashboard.Trends) == 0 {
		b.WriteString(`<p class="muted">暂无趋势数据</p>`)
	} else {
		maxTrend := maxDisplayBoardCount(dashboard.Trends)
		b.WriteString(`<ul class="topic-list">`)
		for _, trend := range dashboard.Trends {
			b.WriteString(`<li><div class="topic-head"><strong>`)
			b.WriteString(html.EscapeString(trend.Label))
			b.WriteString(`</strong><span class="muted">`)
			b.WriteString(strconv.Itoa(trend.Count))
			b.WriteString(`</span></div><div class="progress"><i style="width:`)
			b.WriteString(strconv.Itoa(displayBoardPercent(trend.Count, maxTrend)))
			b.WriteString(`%"></i></div></li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="section-card"><h2>来源分布</h2>`)
	if len(dashboard.Sources) == 0 {
		b.WriteString(`<p class="muted">暂无来源分布数据</p>`)
	} else {
		maxSource := maxDisplayBoardSourceCount(dashboard.Sources)
		b.WriteString(`<ul class="topic-list">`)
		for _, source := range dashboard.Sources {
			b.WriteString(`<li><div class="topic-head"><strong>`)
			b.WriteString(html.EscapeString(nonEmpty(source.SourceType, "未知来源")))
			b.WriteString(`</strong><span class="muted">`)
			b.WriteString(strconv.Itoa(source.Count))
			b.WriteString(`</span></div><div class="progress"><i style="width:`)
			b.WriteString(strconv.Itoa(displayBoardPercent(source.Count, maxSource)))
			b.WriteString(`%"></i></div></li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</div></div><div class="dashboard-col">`)
	synthesizeSections := buildDisplayBoardSynthSections(boardArticles.Items)
	b.WriteString(`<section><h2>旧版主题区</h2><div class="grid">`)
	for _, title := range []string{"政策热点", "财经热点", "36氪", "头条热点"} {
		section := findDisplayBoardSection(synthesizeSections, title)
		if section == nil {
			continue
		}
		b.WriteString(`<div class="section-card"><h3>`)
		b.WriteString(html.EscapeString(section.Title))
		b.WriteString(`</h3><ul class="topic-list">`)
		for _, item := range section.Items {
			link := "/articles/" + strconv.FormatInt(item.ID, 10) + "?return_to=" + url.QueryEscape(returnTo)
			b.WriteString(`<li><a class="inline" href="`)
			b.WriteString(link)
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(item.Title))
			b.WriteString(`</a><div class="muted">`)
			b.WriteString(html.EscapeString(item.SourceName))
			b.WriteString(` · `)
			b.WriteString(item.CapturedAt.Format("2006-01-02 15:04"))
			b.WriteString(`</div></li>`)
		}
		b.WriteString(`</ul></div>`)
	}
	b.WriteString(`</div></section>`)
	b.WriteString(`<section><h2>综合热点</h2><div class="grid">`)
	for _, section := range synthesizeSections {
		b.WriteString(`<div class="section-card"><h3>`)
		b.WriteString(html.EscapeString(section.Title))
		b.WriteString(`</h3><ul class="topic-list">`)
		for _, item := range section.Items {
			link := "/articles/" + strconv.FormatInt(item.ID, 10) + "?return_to=" + url.QueryEscape(returnTo)
			b.WriteString(`<li><a class="inline" href="`)
			b.WriteString(link)
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(item.Title))
			b.WriteString(`</a><div class="muted">`)
			b.WriteString(html.EscapeString(item.SourceName))
			b.WriteString(` · `)
			b.WriteString(item.CapturedAt.Format("2006-01-02 15:04"))
			b.WriteString(`</div></li>`)
		}
		if len(section.Items) == 0 {
			b.WriteString(`<li class="muted">暂无数据</li>`)
		}
		b.WriteString(`</ul></div>`)
	}
	if len(synthesizeSections) == 0 {
		b.WriteString(`<div class="section-card"><p class="muted">暂无综合热点数据</p></div>`)
	}
	b.WriteString(`</div></section>`)
	b.WriteString(`<section><h2>热点关键词</h2><table><tr><th>关键词</th><th>次数</th><th>跳转</th></tr>`)
	for _, item := range hotspots {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(item.Keyword))
		b.WriteString(`</td><td>`)
		b.WriteString(strconv.Itoa(item.Count))
		b.WriteString(`</td><td><a class="inline" href="/fullsearch/result?searchword=`)
		b.WriteString(url.QueryEscape(item.Keyword))
		b.WriteString(`&menuStyle=1&fulltype=8&page=1">全文检索</a></td></tr>`)
	}
	if len(hotspots) == 0 {
		b.WriteString(`<tr><td colspan="3">暂无热点关键词</td></tr>`)
	}
	b.WriteString(`</table><div class="section-card"><h2>迁移说明</h2><div class="summary-strip"><div><strong>Go 门户接管</strong><span class="muted">统一菜单、统一鉴权、统一分析源</span></div><div><strong>旧版结构保留</strong><span class="muted">主题区 / 热点 / 项目入口仍保持熟悉布局</span></div><div><strong>可继续增强</strong><span class="muted">后续可把图表区拆成更强的可视化组件</span></div></div><p class="muted">旧 Java 大屏已逐步迁移到 Go 门户，当前页面继续保留原有信息架构，同时接入统一菜单头、统一鉴权与统一分析数据源。</p></div></div>`)
	b.WriteString(`</div><div class="dashboard-col">`)
	b.WriteString(`<section><h2>右栏摘要</h2><div class="summary-strip">`)
	b.WriteString(`<div><strong>最新文章` + `</strong><span class="muted">`)
	b.WriteString(strconv.Itoa(len(articles.Items)))
	b.WriteString(` 条</span></div>`)
	b.WriteString(`<div><strong>热点关键词</strong><span class="muted">`)
	b.WriteString(strconv.Itoa(len(hotspots)))
	b.WriteString(` 个</span></div>`)
	b.WriteString(`<div><strong>项目入口</strong><span class="muted">`)
	b.WriteString(strconv.Itoa(len(projects)))
	b.WriteString(` 个</span></div>`)
	b.WriteString(`</div></section>`)
	b.WriteString(`<section><h2>最新文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th><th>入口</th></tr>`)
	for _, item := range articles.Items {
		link := "/articles/" + strconv.FormatInt(item.ID, 10) + "?return_to=" + url.QueryEscape(returnTo)
		b.WriteString(`<tr><td><a class="inline" href="`)
		b.WriteString(link)
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(item.Title))
		b.WriteString(`</a></td><td>`)
		b.WriteString(html.EscapeString(nonEmpty(item.FromText, item.SourceType)))
		b.WriteString(`</td><td>`)
		b.WriteString(item.CapturedAt.Format("2006-01-02 15:04"))
		b.WriteString(`</td><td><a class="inline" href="/articles?project_id=`)
		b.WriteString(strconv.FormatInt(displayBoardProjectID(item, projects), 10))
		b.WriteString(`">项目文章</a></td></tr>`)
	}
	if len(articles.Items) == 0 {
		b.WriteString(`<tr><td colspan="4">暂无文章</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>重点摘要</h2><div class="section-card">`)
	if len(articles.Items) > 0 {
		top := articles.Items[0]
		b.WriteString(`<div class="kpi-chip">`)
		b.WriteString(html.EscapeString(nonEmpty(top.FromText, top.SourceType)))
		b.WriteString(`</div><h3>`)
		b.WriteString(html.EscapeString(top.Title))
		b.WriteString(`</h3><p class="muted">`)
		b.WriteString(top.CapturedAt.Format("2006-01-02 15:04"))
		b.WriteString(` · `)
		b.WriteString(html.EscapeString(nonEmpty(top.Summary, "暂无摘要")))
		b.WriteString(`</p>`)
	} else {
		b.WriteString(`<p class="muted">暂无重点摘要</p>`)
	}
	b.WriteString(`</div></section>`)
	b.WriteString(`<section><h2>项目快捷入口</h2><table><tr><th>项目</th><th>项目组</th><th>关键词</th><th>入口</th></tr>`)
	for _, project := range projects {
		b.WriteString(`<tr><td><a class="inline" href="/projects/`)
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(project.Name))
		b.WriteString(`</a></td><td>`)
		b.WriteString(html.EscapeString(project.GroupName))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(project.Keywords))
		b.WriteString(`</td><td><a class="inline" href="/monitor/detail/`)
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString(`?groupid=`)
		b.WriteString(strconv.FormatInt(project.GroupID, 10))
		b.WriteString(`&projectid=`)
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString(`">监测详情</a> <a class="inline" href="/volume?groupid=`)
		b.WriteString(strconv.FormatInt(project.GroupID, 10))
		b.WriteString(`&projectid=`)
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString(`">声量页</a></td></tr>`)
	}
	if len(projects) == 0 {
		b.WriteString(`<tr><td colspan="4">暂无项目</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th><th>健康检查</th></tr>`)
	for _, service := range services {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(service.Name))
		b.WriteString(`</td><td>`)
		b.WriteString(`<span class="`)
		b.WriteString(serviceStatusClass(service))
		b.WriteString(`">`)
		b.WriteString(serviceStatusLabel(service))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(service.Message))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>公告与任务</h2><div class="grid"><div><h3>公告</h3><table><tr><th>标题</th><th>时间</th></tr>`)
	for _, notice := range notices {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(notice.Title))
		b.WriteString(`</td><td>`)
		b.WriteString(notice.CreatedAt.Format("2006-01-02 15:04"))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div><div><h3>任务记录</h3><table><tr><th>任务</th><th>状态</th><th>说明</th></tr>`)
	for _, run := range taskRuns {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(run.TaskName))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(run.Status))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(run.Message))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div><div><h3>抓取记录</h3><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th></tr>`)
	for _, run := range crawlRuns {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(run.SourceType))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(run.Status))
		b.WriteString(`</td><td>`)
		b.WriteString(strconv.Itoa(run.FetchedCount))
		b.WriteString(`</td><td>`)
		b.WriteString(strconv.Itoa(run.InsertedCount))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></div></section></div></section>`)
	_ = s.writeSimplePage(w, "dashboard", "综合看板", b.String())
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

func metricCard(label, value string) string {
	var b strings.Builder
	b.WriteString(`<div style="padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2"><span class="muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong style="display:block;font-size:28px;margin-top:6px">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
	return b.String()
}

func maxDisplayBoardCount(points []model.TrendPoint) int {
	maxValue := 0
	for _, point := range points {
		if point.Count > maxValue {
			maxValue = point.Count
		}
	}
	if maxValue <= 0 {
		return 1
	}
	return maxValue
}

func maxDisplayBoardSourceCount(points []model.SourceBreakdown) int {
	maxValue := 0
	for _, point := range points {
		if point.Count > maxValue {
			maxValue = point.Count
		}
	}
	if maxValue <= 0 {
		return 1
	}
	return maxValue
}

func displayBoardPercent(value, maxValue int) int {
	if maxValue <= 0 {
		return 0
	}
	if value < 0 {
		value = 0
	}
	percent := int(float64(value) / float64(maxValue) * 100)
	if percent < 1 && value > 0 {
		percent = 1
	}
	if percent > 100 {
		percent = 100
	}
	return percent
}

func displayBoardProjectID(item model.Item, projects []model.Project) int64 {
	if len(projects) == 0 {
		return 0
	}
	if len(item.ProjectIDs) > 0 {
		return item.ProjectIDs[0]
	}
	return projects[0].ID
}

type displayBoardTopicItem struct {
	ID           int64
	Title        string
	SourceName   string
	CapturedAt   time.Time
	Category     string
	SourceType   string
	FromText     string
	ExternalHost string
}

type displayBoardTopicSection struct {
	Title string
	Items []displayBoardTopicItem
}

func buildDisplayBoardSynthSections(items []model.Item) []displayBoardTopicSection {
	if len(items) == 0 {
		return nil
	}
	sorted := append([]model.Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CapturedAt.Equal(sorted[j].CapturedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CapturedAt.After(sorted[j].CapturedAt)
	})

	categories := []string{"头条热点", "微博热点", "微信热点", "抖音热点", "B站热点", "腾讯热门", "36氪", "财经热点", "政策热点"}
	buckets := make(map[string][]displayBoardTopicItem, len(categories))
	for _, item := range sorted {
		category := displayBoardItemCategory(item)
		if category == "" {
			continue
		}
		buckets[category] = append(buckets[category], displayBoardTopicItem{
			ID:           item.ID,
			Title:        nonEmpty(item.Title, "未命名文章"),
			SourceName:   displayBoardItemSourceName(item),
			CapturedAt:   item.CapturedAt,
			Category:     category,
			SourceType:   item.SourceType,
			FromText:     item.FromText,
			ExternalHost: item.ExternalSourceHost,
		})
	}

	sections := make([]displayBoardTopicSection, 0, len(categories))
	for _, category := range categories {
		items := buckets[category]
		if len(items) > 3 {
			items = items[:3]
		}
		if len(items) == 0 && category == "头条热点" {
			for idx, item := range sorted {
				if idx >= 3 {
					break
				}
				items = append(items, displayBoardTopicItem{
					ID:         item.ID,
					Title:      nonEmpty(item.Title, "未命名文章"),
					SourceName: displayBoardItemSourceName(item),
					CapturedAt: item.CapturedAt,
					Category:   category,
				})
			}
		}
		if len(items) == 0 {
			continue
		}
		sections = append(sections, displayBoardTopicSection{Title: category, Items: items})
	}
	return sections
}

func displayBoardItemCategory(item model.Item) string {
	kind := strings.ToLower(strings.TrimSpace(strings.Join([]string{item.SourceType, item.FromText, item.ExternalSourceHost}, " ")))
	switch {
	case strings.Contains(kind, "weibo") || strings.Contains(item.FromText, "微博"):
		return "微博热点"
	case strings.Contains(kind, "wechat") || strings.Contains(item.FromText, "微信"):
		return "微信热点"
	case strings.Contains(kind, "douyin") || strings.Contains(item.FromText, "抖音"):
		return "抖音热点"
	case strings.Contains(kind, "bilibili") || strings.Contains(item.FromText, "B站") || strings.Contains(item.FromText, "哔哩哔哩"):
		return "B站热点"
	case strings.Contains(kind, "tencent") || strings.Contains(item.FromText, "腾讯"):
		return "腾讯热门"
	case strings.Contains(kind, "36kr"):
		return "36氪"
	case strings.Contains(kind, "eastmoney") || strings.Contains(kind, "finance") || strings.Contains(item.FromText, "东方财富"):
		return "财经热点"
	case strings.Contains(kind, "gov") || strings.Contains(item.ExternalSourceHost, "gov.cn") || strings.Contains(item.Title, "国务院"):
		return "政策热点"
	default:
		return "头条热点"
	}
}

func displayBoardItemSourceName(item model.Item) string {
	if name := strings.TrimSpace(item.FromText); name != "" {
		return name
	}
	if name := strings.TrimSpace(item.SourceType); name != "" {
		return name
	}
	if host := strings.TrimSpace(item.ExternalSourceHost); host != "" {
		return host
	}
	return "综合"
}

func findDisplayBoardSection(sections []displayBoardTopicSection, title string) *displayBoardTopicSection {
	for i := range sections {
		if sections[i].Title == title {
			return &sections[i]
		}
	}
	return nil
}

func (s *Server) handleSystemProductManualOnline(w http.ResponseWriter, r *http.Request, _ any) {
	manualURL := "/system/uploadProductManual"
	body := `<h1>产品手册</h1><section><p>在线阅读入口直接复用仓库内的产品手册 PDF。</p><p><a class="inline" href="` + manualURL + `" target="_blank" rel="noreferrer">下载 / 打开产品手册</a></p><iframe src="` + manualURL + `" style="width:100%;height:80vh;border:1px solid #ece7dc;border-radius:12px"></iframe></section>`
	_ = s.writeSimplePage(w, "productmanual", "产品手册", body)
}

func (s *Server) handleSystemUploadProductManual(w http.ResponseWriter, r *http.Request, _ any) {
	manualPath, err := locateProductManualPath()
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusNotFound, "产品手册文件未找到", nil)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="product-manual.pdf"`)
	http.ServeFile(w, r, manualPath)
}

func locateProductManualPath() (string, error) {
	candidates := []string{
		filepath.Clean(filepath.Join("assets", "product-manual.pdf")),
		filepath.Clean(filepath.Join("go", "assets", "product-manual.pdf")),
		filepath.Clean(filepath.Join("..", "assets", "product-manual.pdf")),
		filepath.Clean(filepath.Join("..", "..", "assets", "product-manual.pdf")),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

func (s *Server) handleMonitorWxGroup(w http.ResponseWriter, r *http.Request, _ any) {
	body := `<section style="display:grid;gap:20px"><div><h2>联系我们</h2><p class="muted">系统使用中有任何问题，可以通过以下方式联系支持团队。</p></div><div class="grid" style="grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:20px"><div style="text-align:center;padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#faf8f2"><h3>微信公众号</h3><img src="/assets/images/users/wxOfficialAccount.jpg" alt="微信公众号" style="max-width:180px;width:100%;border-radius:10px"><p class="muted">关注公众号获取产品动态</p></div><div style="text-align:center;padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#faf8f2"><h3>微信交流群</h3><img src="/assets/images/users/wxGroup.jpg" alt="微信交流群" style="max-width:180px;width:100%;border-radius:10px"><p class="muted">扫码加入交流群</p></div><div style="text-align:center;padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#faf8f2"><h3>产品经理微信</h3><img src="/assets/images/expireCode.jpg" alt="产品经理微信" style="max-width:180px;width:100%;border-radius:10px"><p class="muted">添加产品经理微信</p></div><div style="text-align:center;padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#faf8f2"><h3>官方网站</h3><p><a class="inline" href="https://www.stonedt.com/" target="_blank" rel="noreferrer">www.stonedt.com</a></p><img src="/assets/images/bt.jpg" alt="合作伙伴" style="max-width:180px;width:100%;border-radius:10px"><p class="muted">合作伙伴与更多信息</p></div></div></section>`
	_ = s.writeSimplePage(w, "monitor/wxGroup", "联系我们", body)
}

func (s *Server) handleMobileMonitor(w http.ResponseWriter, r *http.Request, user any) {
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	dashboard, notices, taskRuns, crawlRuns, services := s.loadDashboardContext(r)
	groups := s.groupProjectsForMobile()
	var b strings.Builder
	b.WriteString("<h1>移动端监测</h1>")
	b.WriteString(`<p><a href="/system">系统页</a> | <a href="/mobile/mobileQRCode">二维码</a> | <a href="/hot/hotpage">热点页</a></p>`)
	b.WriteString(`<section><h2>核心指标</h2><ul>`)
	b.WriteString("<li>文章数：")
	b.WriteString(strconv.Itoa(dashboard.Overview.ArticleCount))
	b.WriteString("</li><li>项目数：")
	b.WriteString(strconv.Itoa(dashboard.Overview.ProjectCount))
	b.WriteString("</li><li>报告数：")
	b.WriteString(strconv.Itoa(dashboard.Overview.ReportCount))
	b.WriteString("</li><li>活跃规则：")
	b.WriteString(strconv.Itoa(dashboard.Overview.AlertRuleCount))
	b.WriteString("</li></ul></section>")
	b.WriteString(`<section><h2>项目分组</h2>`)
	for _, group := range groups {
		for key, list := range group {
			parts := strings.SplitN(key, "-", 2)
			groupID := ""
			groupName := key
			if len(parts) == 2 {
				groupID = parts[0]
				groupName = parts[1]
			}
			b.WriteString("<section><h3>")
			b.WriteString(html.EscapeString(groupName))
			b.WriteString("</h3><p>groupid=")
			b.WriteString(html.EscapeString(groupID))
			b.WriteString("</p><table><tr><th>项目</th><th>关键词</th><th>状态</th><th>操作</th></tr>")
			for _, item := range list {
				groupIDValue := legacyStringFromAny(item["group_id"])
				projectIDValue := legacyStringFromAny(item["project_id"])
				b.WriteString("<tr><td>")
				b.WriteString(html.EscapeString(legacyStringFromAny(item["project_name"])))
				b.WriteString("</td><td>")
				b.WriteString(html.EscapeString(legacyStringFromAny(item["keywords"])))
				b.WriteString("</td><td>")
				b.WriteString(html.EscapeString(legacyStringFromAny(item["status"])))
				b.WriteString("</td><td><a href=\"/mobile/monitor/detail?groupid=")
				b.WriteString(url.QueryEscape(groupIDValue))
				b.WriteString("&projectid=")
				b.WriteString(url.QueryEscape(projectIDValue))
				b.WriteString("\">查看详情</a></td></tr>")
			}
			b.WriteString("</table></section>")
		}
	}
	b.WriteString(`<section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th></tr>`)
	for _, service := range services {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(service.Name))
		b.WriteString("</td><td>")
		b.WriteString(serviceStatusLabel(service))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>最近公告</h2><table><tr><th>标题</th><th>时间</th></tr>`)
	for _, notice := range notices {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(notice.Title))
		b.WriteString("</td><td>")
		b.WriteString(notice.CreatedAt.Format("2006-01-02 15:04"))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>最近任务</h2><table><tr><th>任务</th><th>状态</th><th>说明</th></tr>`)
	for _, run := range taskRuns {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(run.TaskName))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(run.Status))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(run.Message))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>最近抓取</h2><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th></tr>`)
	for _, run := range crawlRuns {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(run.SourceType))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(run.Status))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(run.FetchedCount))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(run.InsertedCount))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	_ = s.writeSimplePage(w, "mobile/monitor", "移动端监测", b.String())
}

func (s *Server) handleMobileMonitorDetail(w http.ResponseWriter, r *http.Request, user any) {
	groupID := strings.TrimSpace(r.URL.Query().Get("groupid"))
	projectID := strings.TrimSpace(r.URL.Query().Get("projectid"))
	if projectID == "" {
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	var projects []model.Project
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	var project model.Project
	for _, item := range projects {
		if strconv.FormatInt(item.ID, 10) == projectID {
			project = item
			break
		}
	}
	articles := model.ItemListResult{}
	reports := []model.Report{}
	rules := []model.MonitorRule{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?page=1&page_size=10&project_id="+url.QueryEscape(projectID), &articles)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+url.QueryEscape(projectID), &reports)
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/monitor-rules", &rules)
	filteredRules := make([]model.MonitorRule, 0, len(rules))
	for _, rule := range rules {
		if strconv.FormatInt(rule.ProjectID, 10) == projectID {
			filteredRules = append(filteredRules, rule)
		}
	}
	warningArticles := []legacyWarningArticleCompat{}
	if userID := userIDFromMap(user); userID > 0 {
		if page, err := s.collectLegacyWarningArticles(userID, parseProjectID(projectID), 0, "", 1); err == nil {
			warningArticles = page.Articles
		}
	}
	var b strings.Builder
	b.WriteString("<h1>移动端详情</h1>")
	b.WriteString(`<p><a href="/mobile/monitor">返回监测页</a> | <a href="/volume?groupid=`)
	b.WriteString(url.QueryEscape(groupID))
	b.WriteString(`&projectid=`)
	b.WriteString(url.QueryEscape(projectID))
	b.WriteString(`">声量页</a> | <a href="/articles?project_id=`)
	b.WriteString(url.QueryEscape(projectID))
	b.WriteString(`">文章中心</a></p>`)
	b.WriteString("<section><h2>")
	b.WriteString(html.EscapeString(project.Name))
	b.WriteString("</h2><p>项目组：")
	b.WriteString(html.EscapeString(project.GroupName))
	b.WriteString(" | 状态：")
	b.WriteString(html.EscapeString(project.Status))
	b.WriteString("</p><p>关键词：")
	b.WriteString(html.EscapeString(project.Keywords))
	b.WriteString("</p><pre>")
	b.WriteString(html.EscapeString(project.Description))
	b.WriteString("</pre></section>")
	b.WriteString(`<section><h2>最近文章</h2><table><tr><th>标题</th><th>来源</th><th>时间</th></tr>`)
	for _, item := range articles.Items {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(item.Title))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(item.SourceType))
		b.WriteString("</td><td>")
		b.WriteString(item.CapturedAt.Format("2006-01-02 15:04"))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>关联规则</h2><table><tr><th>名称</th><th>等级</th><th>状态</th></tr>`)
	for _, rule := range filteredRules {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(rule.Name))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(rule.Severity))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(rule.Status))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>预警消息</h2><p><a href="/system?section=warningmsg&project_id=`)
	b.WriteString(url.QueryEscape(projectID))
	b.WriteString(`">查看系统预警列表</a></p><table><tr><th>标题</th><th>项目</th><th>时间</th></tr>`)
	for _, item := range warningArticles {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(item.ArticleTitle))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(item.ProjectName))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(item.ArticleTime))
		b.WriteString("</td></tr>")
	}
	if len(warningArticles) == 0 {
		b.WriteString(`<tr><td colspan="3">暂无预警消息</td></tr>`)
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>关联报告</h2><table><tr><th>标题</th><th>状态</th><th>更新时间</th></tr>`)
	for _, report := range reports {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(report.Title))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(report.Status))
		b.WriteString("</td><td>")
		b.WriteString(report.UpdatedAt.Format("2006-01-02 15:04"))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	_ = s.writeSimplePage(w, "mobile/detail", "移动端详情", b.String())
}

func (s *Server) handleMobileWarning(w http.ResponseWriter, r *http.Request, user any) {
	projectID := strings.TrimSpace(r.URL.Query().Get("projectid"))
	if projectID == "" {
		projectID = strings.TrimSpace(r.URL.Query().Get("project_id"))
	}
	articles := []legacyWarningArticleCompat{}
	if userID := userIDFromMap(user); userID > 0 {
		page, err := s.collectLegacyWarningArticles(userID, parseProjectID(projectID), 1, "", 1)
		if err == nil {
			articles = page.Articles
		}
	}
	var b strings.Builder
	b.WriteString("<h1>移动端预警</h1>")
	b.WriteString(`<p><a href="/system?section=warningmsg`)
	if projectID != "" {
		b.WriteString(`&project_id=`)
		b.WriteString(url.QueryEscape(projectID))
	}
	b.WriteString(`">系统预警消息</a></p>`)
	b.WriteString(`<section><h2>预警文章</h2><table><tr><th>标题</th><th>项目</th><th>时间</th></tr>`)
	for _, item := range articles {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(item.ArticleTitle))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(item.ProjectName))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(item.ArticleTime))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table></section>")
	_ = s.writeSimplePage(w, "mobile/warning", "移动端预警", b.String())
}

func (s *Server) handleMonitorEntry(w http.ResponseWriter, r *http.Request, _ any) {
	target := "/mobile/monitor"
	if groupID := strings.TrimSpace(r.URL.Query().Get("groupid")); groupID != "" {
		target += "?groupid=" + url.QueryEscape(groupID)
		if projectID := strings.TrimSpace(r.URL.Query().Get("projectid")); projectID != "" {
			target += "&projectid=" + url.QueryEscape(projectID)
		}
	} else if projectID := strings.TrimSpace(r.URL.Query().Get("projectid")); projectID != "" {
		target += "?projectid=" + url.QueryEscape(projectID)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleMonitorCompat(w http.ResponseWriter, r *http.Request, user any) {
	path := strings.TrimPrefix(r.URL.Path, "/monitor/")
	switch {
	case path == "":
		s.handleMonitorEntry(w, r, user)
	case path == "detail" || strings.HasPrefix(path, "detail/"):
		s.handleMonitorDetail(w, r, user)
	case path == "wxGroup":
		s.handleMonitorWxGroup(w, r, user)
	case s.handleLegacyMonitorAction(w, r, user, path):
		return
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleMonitorDetail(w http.ResponseWriter, r *http.Request, user any) {
	articleID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/monitor/detail/"))
	if articleID == "" {
		articleID = strings.TrimSpace(r.URL.Query().Get("articleid"))
	}
	if articleID == "" {
		http.Redirect(w, r, "/mobile/monitor", http.StatusSeeOther)
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("groupid"))
	projectID := strings.TrimSpace(r.URL.Query().Get("projectid"))
	returnURL := "/mobile/monitor"
	query := url.Values{}
	if groupID != "" {
		query.Set("groupid", groupID)
	}
	if projectID != "" {
		query.Set("projectid", projectID)
	}
	if len(query) > 0 {
		returnURL += "?" + query.Encode()
	}
	returnTo := url.QueryEscape(returnURL)
	userID := userIDFromMap(user)
	if r.Method == http.MethodPost && userID > 0 {
		_ = r.ParseForm()
		action := r.FormValue("action")
		message := "文章操作失败"
		switch action {
		case "favorite":
			resp, err := s.client.R().SetQueryParam("user_id", strconv.FormatInt(userID, 10)).Post(s.cfg.ContentURL + "/api/v1/articles/" + articleID + "/favorite")
			if err == nil && resp.IsSuccess() {
				message = "收藏状态已更新"
			}
		case "read":
			resp, err := s.client.R().SetQueryParam("user_id", strconv.FormatInt(userID, 10)).Post(s.cfg.ContentURL + "/api/v1/articles/" + articleID + "/read")
			if err == nil && resp.IsSuccess() {
				message = "文章已标记为已读"
			}
		case "share":
			resp, err := s.client.R().
				SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
				SetBody(map[string]string{"channel": "portal-detail"}).
				Post(s.cfg.ContentURL + "/api/v1/articles/" + articleID + "/share")
			if err == nil && resp.IsSuccess() {
				message = "文章已登记分享"
			}
		}
		monitorURL := "/monitor/detail/" + articleID
		if len(query) > 0 {
			monitorURL += "?" + query.Encode()
		}
		http.Redirect(w, r, appendMessage(monitorURL, message), http.StatusSeeOther)
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
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles/"+articleID+querySuffix, &article)
	relatedURL := s.cfg.ContentURL + "/api/v1/articles/" + articleID + "/related"
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
	for _, projectIDValue := range article.ProjectIDs {
		project := model.Project{}
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/projects/"+strconv.FormatInt(projectIDValue, 10), &project); err == nil && project.ID > 0 {
			projects = append(projects, project)
		}
		projectReports := []model.Report{}
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/reports?project_id="+strconv.FormatInt(projectIDValue, 10), &projectReports); err == nil {
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
	warningArticles := []legacyWarningArticleCompat{}
	if userID > 0 {
		if page, err := s.collectLegacyWarningArticles(userID, parseProjectID(projectID), 0, "", 1); err == nil {
			warningArticles = page.Articles
		}
	}
	var b strings.Builder
	b.WriteString(`<h1>监测详情</h1>`)
	b.WriteString(`<p><a href="`)
	b.WriteString(html.EscapeString(returnURL))
	b.WriteString(`">返回监测页</a> | <a href="/articles/`)
	b.WriteString(strconv.FormatInt(article.ID, 10))
	b.WriteString(`?return_to=`)
	b.WriteString(returnTo)
	b.WriteString(`">文章详情页</a></p>`)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<section><div class="msg">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div></section>`)
	}
	b.WriteString(`<section><h2>`)
	if article.ID > 0 {
		b.WriteString(html.EscapeString(article.Title))
	} else {
		b.WriteString("未找到文章")
	}
	b.WriteString(`</h2><p>来源：`)
	b.WriteString(html.EscapeString(nonEmpty(article.FromText, article.SourceType, article.ExternalSourceHost)))
	b.WriteString(` | 时间：`)
	b.WriteString(article.CapturedAt.Format("2006-01-02 15:04"))
	b.WriteString(`</p><p>项目：`)
	if len(projects) == 0 {
		b.WriteString("暂无项目")
	} else {
		for idx, project := range projects {
			if idx > 0 {
				b.WriteString("，")
			}
			b.WriteString(html.EscapeString(project.Name))
		}
	}
	b.WriteString(`</p><pre>`)
	b.WriteString(html.EscapeString(nonEmpty(article.Summary, article.Content)))
	b.WriteString(`</pre><div class="grid"><div class="section-card">已读`)
	b.WriteString(strconv.Itoa(readCount))
	b.WriteString(`</div><div class="section-card">未读`)
	b.WriteString(strconv.Itoa(unreadCount))
	b.WriteString(`</div><div class="section-card">收藏`)
	b.WriteString(strconv.Itoa(flaggedCount))
	b.WriteString(`</div></div></section>`)
	b.WriteString(`<section><h2>操作</h2><form class="inline" method="post"><button name="action" value="read" type="submit">标记已读</button><button name="action" value="favorite" type="submit">收藏</button><button name="action" value="share" type="submit">分享</button></form></section>`)
	b.WriteString(`<section><h2>相关文章</h2><table><tr><th>标题</th><th>来源</th><th>状态</th></tr>`)
	for _, item := range related {
		b.WriteString(`<tr><td><a class="inline" href="/monitor/detail/`)
		b.WriteString(strconv.FormatInt(item.ID, 10))
		b.WriteString(`?groupid=`)
		b.WriteString(url.QueryEscape(groupID))
		b.WriteString(`&projectid=`)
		b.WriteString(url.QueryEscape(projectID))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(item.Title))
		b.WriteString(`</a></td><td>`)
		b.WriteString(html.EscapeString(nonEmpty(item.FromText, item.SourceType)))
		b.WriteString(`</td><td>`)
		if item.Read {
			b.WriteString("已读")
		} else {
			b.WriteString("未读")
		}
		if item.Favorited {
			b.WriteString(" / 已收藏")
		}
		b.WriteString(`</td></tr>`)
	}
	if len(related) == 0 {
		b.WriteString(`<tr><td colspan="3">暂无相关文章</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>预警消息</h2><p><a href="/system?section=warningmsg`)
	if projectID != "" {
		b.WriteString(`&project_id=`)
		b.WriteString(url.QueryEscape(projectID))
	}
	b.WriteString(`">查看系统预警列表</a></p><table><tr><th>标题</th><th>项目</th><th>时间</th></tr>`)
	for _, item := range warningArticles {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(item.ArticleTitle))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.ProjectName))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.ArticleTime))
		b.WriteString(`</td></tr>`)
	}
	if len(warningArticles) == 0 {
		b.WriteString(`<tr><td colspan="3">暂无预警消息</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>关联报告</h2><table><tr><th>标题</th><th>状态</th><th>更新时间</th></tr>`)
	for _, report := range reports {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(report.Title))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(report.Status))
		b.WriteString(`</td><td>`)
		b.WriteString(report.UpdatedAt.Format("2006-01-02 15:04"))
		b.WriteString(`</td></tr>`)
	}
	if len(reports) == 0 {
		b.WriteString(`<tr><td colspan="3">暂无报告</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	_ = s.writeSimplePage(w, "monitor/detail", "监测详情", b.String())
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
	var projects []model.Project
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].GroupName == projects[j].GroupName {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].GroupName < projects[j].GroupName
	})
	var hotspots []model.KeywordHotspot
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/keywords", &hotspots)
	if len(hotspots) == 0 {
		hotspots = []model.KeywordHotspot{{Keyword: "暂无数据", Count: 0}}
	}
	var b strings.Builder
	b.WriteString("<h1>声量监测</h1>")
	b.WriteString("<p>groupid=")
	b.WriteString(html.EscapeString(groupID))
	b.WriteString(" projectid=")
	b.WriteString(html.EscapeString(projectID))
	b.WriteString(`</p><p><a href="/system">系统页</a> | <a href="/hot/hotpage">热点页</a></p>`)
	b.WriteString(`<section><h2>项目入口</h2><table><tr><th>项目组</th><th>项目</th><th>关键词</th><th>操作</th></tr>`)
	for _, project := range projects {
		if groupID != "" && strconv.FormatInt(project.GroupID, 10) != groupID {
			continue
		}
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(project.GroupName))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(project.Name))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(project.Keywords))
		b.WriteString("</td><td><a href=\"/volume/getproject?groupid=")
		b.WriteString(strconv.FormatInt(project.GroupID, 10))
		b.WriteString("&projectid=")
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString("\">查看声量</a> <a href=\"/volume/projectname?groupId=")
		b.WriteString(strconv.FormatInt(project.GroupID, 10))
		b.WriteString("&projectid=")
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString("\">项目名</a></td></tr>")
	}
	b.WriteString("</table></section>")
	b.WriteString(`<section><h2>热点关键词</h2><table><tr><th>关键词</th><th>热度</th><th>跳转</th></tr>`)
	for _, item := range hotspots {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(item.Keyword))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(item.Count))
		b.WriteString("</td><td><a href=\"/articles?mode=full&keyword=")
		b.WriteString(url.QueryEscape(item.Keyword))
		b.WriteString("\">全文检索</a></td></tr>")
	}
	b.WriteString("</table></section>")
	_ = s.writeSimplePage(w, "volume", "声量监测", b.String())
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
		_ = s.writeSimplePage(w, "userapply", "申请试用", `<section><h1>申请试用</h1><p>先填写申请信息，再等待审核开通。</p><p><a href="/dist/yqapply">去填写申请表单</a></p></section>`)
		return
	}
	if strings.EqualFold(r.URL.Query().Get("approved"), "true") {
		http.Redirect(w, r, "/dist/yqmontitor", http.StatusSeeOther)
		return
	}
	body := `<section><h1>申请试用</h1><p>openid=` + html.EscapeString(openid) + `</p><p>申请信息已收到，等待审核后可跳转监测页。</p><p><a href="/dist/yqapply?openid=` + url.QueryEscape(openid) + `">继续完善申请信息</a></p></section>`
	_ = s.writeSimplePage(w, "userapply", "申请试用", body)
}

func (s *Server) handleDistApply(w http.ResponseWriter, r *http.Request) {
	target := "/dist/yqapply"
	if raw := strings.TrimSpace(r.URL.RawQuery); raw != "" {
		target += "?" + raw
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleDistYqApply(w http.ResponseWriter, r *http.Request) {
	openid := html.EscapeString(strings.TrimSpace(r.URL.Query().Get("openid")))
	body := `<section><h1>申请试用</h1><p>Go 兼容版申请页，提交后仍返回 JSON 结果，便于旧前端继续使用。</p><form method="post" action="/dist/applydatainfo"><input name="openid" placeholder="openid" value="` + openid + `"><input name="name" placeholder="姓名"><input name="telephone" placeholder="电话"><input name="industry" placeholder="行业"><input name="company" placeholder="公司"><button type="submit">提交申请</button></form></section>`
	_ = s.writeSimplePage(w, "userapply", "申请试用", body)
}

func (s *Server) handleDistApplyDataInfo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSONText(w, map[string]any{"code": 400, "msg": "参数解析失败"})
		return
	}
	openid := strings.TrimSpace(firstNonEmpty(r.FormValue("openid"), r.URL.Query().Get("openid")))
	name := strings.TrimSpace(r.FormValue("name"))
	if openid == "" || name == "" {
		writeJSONText(w, map[string]any{"code": 400, "msg": "openid和姓名不能为空"})
		return
	}
	writeJSONText(w, map[string]any{
		"code": 200,
		"msg":  "ok",
		"data": map[string]any{
			"openid":   openid,
			"name":     name,
			"next_url": "/dist/getdata?openid=" + url.QueryEscape(openid) + "&approved=true",
		},
	})
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

func (s *Server) writeSimplePage(w http.ResponseWriter, page string, title string, body string) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	bodyAttr := ""
	if strings.TrimSpace(page) != "" {
		bodyAttr = " data-page='" + html.EscapeString(strings.TrimSpace(page)) + "'"
	}
	footer := renderPortalFooter(pageData{})
	_, err := fmt.Fprintf(w, "<!doctype html><html><head><meta charset='utf-8'><title>%s</title><style>%s a{color:#214e34;text-decoration:none}</style></head><body%s><header><h1>%s</h1>%s%s</header><main>%s</main>%s</body></html>", html.EscapeString(title), baseStyles, bodyAttr, html.EscapeString(title), portalNavHTML, portalUpgradeShellHTML, body, footer)
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
