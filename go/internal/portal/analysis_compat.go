package portal

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type legacyAnalysisBundle struct {
	Project     model.Project
	Projects    []model.Project
	TimePeriod  int
	Dashboard   model.DashboardSnapshot
	Notices     []model.SystemNotice
	TaskRuns    []model.TaskRun
	CrawlRuns   []model.CrawlRun
	Services    []serviceStatus
	Articles    model.ItemListResult
	Hotspots    []model.KeywordHotspot
	Emotions    model.EmotionAnalysis
	Events      []model.EventOverview
	Propagation model.PropagationAnalysis
	Themes      []model.ThemeInsight
}

func (s *Server) handleAnalysisEntry(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleAnalysisRefresh(w, r, user)
		return
	}
	bundle, err := s.buildLegacyAnalysisBundle(r)
	if err != nil {
		_ = s.writeSimplePage(w, "analysis", "监测分析", "<p>加载失败："+html.EscapeString(err.Error())+"</p>")
		return
	}
	projectID := bundle.Project.ID
	if projectID == 0 && len(bundle.Projects) > 0 {
		projectID = bundle.Projects[0].ID
	}

	var b strings.Builder
	b.WriteString("<h1>监测分析</h1>")
	b.WriteString(`<p><a href="/projects">项目中心</a> | <a href="/articles">文章中心</a> | <a href="/reports">报告中心</a> | <a href="/system">系统工作台</a></p>`)
	b.WriteString(`<section><form class="inline" method="get"><select name="projectid">`)
	for _, project := range bundle.Projects {
		b.WriteString(`<option value="`)
		b.WriteString(strconv.FormatInt(project.ID, 10))
		b.WriteString(`"`)
		if project.ID == projectID {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(project.Name))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select><button type="submit">切换项目</button></form>`)
	if bundle.Project.ID > 0 {
		b.WriteString(`<p class="muted">当前项目：`)
		b.WriteString(html.EscapeString(bundle.Project.Name))
		b.WriteString(` | 关键词：`)
		b.WriteString(html.EscapeString(bundle.Project.Keywords))
		b.WriteString(` | <a class="inline" href="/projects/`)
		b.WriteString(strconv.FormatInt(bundle.Project.ID, 10))
		b.WriteString(`">项目详情</a>`)
		b.WriteString(` | <form method="post" action="/analysis/updateanalysisdata?projectid=`)
		b.WriteString(strconv.FormatInt(bundle.Project.ID, 10))
		b.WriteString(`" style="display:inline"><button type="submit">刷新分析</button></form></p>`)
	}
	b.WriteString(`</section>`)
	b.WriteString(`<section><h2>数据概览</h2><pre>`)
	b.WriteString(html.EscapeString(mustJSONString(bundle.buildOverviewPayload())))
	b.WriteString(`</pre></section>`)
	b.WriteString(`<section><h2>情感概览</h2><pre>`)
	b.WriteString(html.EscapeString(mustJSONString(bundle.buildEmotionPayload())))
	b.WriteString(`</pre></section>`)
	b.WriteString(`<section><h2>高频词</h2><table><tr><th>关键词</th><th>次数</th><th>趋势</th><th>指数</th></tr>`)
	for _, keyword := range bundle.buildKeywordIndexPayload() {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(keyword["keyword"])))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(keyword["count"])))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(keyword["trend"])))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(keyword["index"])))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>最新资讯</h2><table><tr><th>标题</th><th>来源</th><th>情感</th><th>时间</th></tr>`)
	for _, item := range bundle.buildLatestNewsPayload() {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(item["title"])))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(item["source_name"])))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(item["emotion_name"])))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(legacyStringFromAny(item["publish_time"])))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>服务状态</h2><table><tr><th>服务</th><th>状态</th><th>健康检查</th></tr>`)
	for _, service := range bundle.Services {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(service.Name))
		b.WriteString(`</td><td>`)
		b.WriteString(serviceStatusLabel(service))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(service.Message))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
	b.WriteString(`<section><h2>公告与任务</h2><div class="grid"><div><h3>公告</h3><table><tr><th>标题</th><th>时间</th></tr>`)
	for _, notice := range bundle.Notices {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(notice.Title))
		b.WriteString(`</td><td>`)
		b.WriteString(notice.CreatedAt.Format("2006-01-02 15:04"))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div><div><h3>任务记录</h3><table><tr><th>任务</th><th>状态</th><th>说明</th></tr>`)
	for _, task := range bundle.TaskRuns {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(task.TaskName))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(task.Status))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(task.Message))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div><div><h3>抓取记录</h3><table><tr><th>来源</th><th>状态</th><th>抓取数</th><th>入库数</th></tr>`)
	for _, run := range bundle.CrawlRuns {
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
	b.WriteString(`</table></div></div></section>`)
	_ = s.writeSimplePage(w, "analysis", "监测分析", b.String())
}

func (s *Server) handleAnalysisCompatJSON(w http.ResponseWriter, r *http.Request, _ any) {
	path := strings.TrimPrefix(r.URL.Path, "/analysis/")
	switch {
	case path == "getAanlysisByProjectidAndTimeperiod", path == "opinionScreen/getAanlysisByProjectidAndTimeperiod", path == "getAnalysisMonitorProjectid":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildLegacyAnalysisSnapshot())
	case path == "latestnews":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildLatestNewsPayload())
	case path == "emotionalproportion":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildEmotionPayload())
	case path == "planwordhit":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildPlanWordHitPayload())
	case path == "popularinformation":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildPopularInformationPayload())
	case path == "emotioncategory":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildEmotionCategoryPayload())
	case path == "keywordindex":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildKeywordIndexPayload())
	case path == "popularkeyword":
		bundle, err := s.buildLegacyAnalysisBundle(r)
		if err != nil {
			writeLegacyJSON(w, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		writeLegacyJSON(w, http.StatusOK, "ok", bundle.buildPopularKeywordPayload())
	case path == "updateanalysisdata":
		s.handleAnalysisRefresh(w, r, nil)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAnalysisRefresh(w http.ResponseWriter, r *http.Request, _ any) {
	resp, err := s.client.R().Post(s.cfg.AnalysisURL + "/api/v1/admin/tasks/analysis/refresh")
	if err != nil || resp == nil || !resp.IsSuccess() {
		writeJSONText(w, map[string]any{
			"status":  500,
			"result":  "fail",
			"message": "分析刷新失败",
		})
		return
	}
	writeJSONText(w, map[string]any{
		"status":  200,
		"result":  "success",
		"message": "分析刷新已提交",
	})
}

func (s *Server) buildLegacyAnalysisBundle(r *http.Request) (legacyAnalysisBundle, error) {
	projects := []model.Project{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/projects", &projects)
	projectID := analysisProjectIDFromRequest(r)
	timePeriod := analysisTimePeriodFromRequest(r)
	if projectID <= 0 && len(projects) > 0 {
		projectID = projects[0].ID
	}
	selected := model.Project{}
	for _, project := range projects {
		if project.ID == projectID {
			selected = project
			break
		}
	}
	dashboard, notices, taskRuns, crawlRuns, services := s.loadDashboardContext(r)
	var articles model.ItemListResult
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/articles?project_id="+strconv.FormatInt(projectID, 10)+"&page=1&page_size=20", &articles)
	var hotspots []model.KeywordHotspot
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/keywords", &hotspots)
	var emotions model.EmotionAnalysis
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/emotions?project_id="+strconv.FormatInt(projectID, 10), &emotions)
	var events []model.EventOverview
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/event-overview?project_id="+strconv.FormatInt(projectID, 10), &events)
	var propagation model.PropagationAnalysis
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/propagation?project_id="+strconv.FormatInt(projectID, 10), &propagation)
	var themes []model.ThemeInsight
	_ = s.getJSON(s.cfg.AnalysisURL+"/api/v1/analysis/themes?project_id="+strconv.FormatInt(projectID, 10), &themes)
	return legacyAnalysisBundle{
		Project:     selected,
		Projects:    projects,
		TimePeriod:  timePeriod,
		Dashboard:   dashboard,
		Notices:     notices,
		TaskRuns:    taskRuns,
		CrawlRuns:   crawlRuns,
		Services:    services,
		Articles:    articles,
		Hotspots:    hotspots,
		Emotions:    emotions,
		Events:      events,
		Propagation: propagation,
		Themes:      themes,
	}, nil
}

func analysisProjectIDFromRequest(r *http.Request) int64 {
	for _, key := range []string{"projectid", "projectId", "project_id"} {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				return parsed
			}
		}
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func analysisTimePeriodFromRequest(r *http.Request) int {
	for _, key := range []string{"timePeriod", "timeperiod", "time_period"} {
		if value := strings.TrimSpace(r.FormValue(key)); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed
			}
		}
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func (b legacyAnalysisBundle) buildOverviewPayload() map[string]any {
	total := maxInt(b.Dashboard.Overview.ArticleCount, len(b.Articles.Items))
	negative := emotionCount(b.Emotions, "negative")
	neutral := emotionCount(b.Emotions, "neutral")
	positive := emotionCount(b.Emotions, "positive")
	all := total
	if all == 0 {
		all = positive + neutral + negative
	}
	if all == 0 {
		all = len(b.Articles.Items)
	}
	if all == 0 {
		all = 1
	}
	sensitive := negative
	if sensitive == 0 && positive+neutral == 0 {
		sensitive = all / 3
	}
	noSensitive := all - sensitive
	if noSensitive < 0 {
		noSensitive = 0
	}
	return map[string]any{
		"all": map[string]any{
			"count": all,
			"rate":  100,
		},
		"sensitive": map[string]any{
			"count": sensitive,
			"rate":  percentage(sensitive, all),
		},
		"noSensitive": map[string]any{
			"count": noSensitive,
			"rate":  percentage(noSensitive, all),
		},
		"earlyWarning": map[string]any{
			"dataSwitch": "open",
			"count":      b.Dashboard.Overview.AlertRuleCount,
			"rate":       percentage(b.Dashboard.Overview.AlertRuleCount, all),
		},
	}
}

func (b legacyAnalysisBundle) buildEmotionPayload() map[string]any {
	negative := emotionCount(b.Emotions, "negative")
	neutral := emotionCount(b.Emotions, "neutral")
	positive := emotionCount(b.Emotions, "positive")
	return map[string]any{
		"rate": map[string]int{
			"positive": positive,
			"neutral":  neutral,
			"negative": negative,
		},
		"chart": [][]any{
			{"正面", positive},
			{"中性", neutral},
			{"负面", negative},
		},
	}
}

func (b legacyAnalysisBundle) buildLatestNewsPayload() []map[string]any {
	items := make([]model.Item, len(b.Articles.Items))
	copy(items, b.Articles.Items)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CapturedAt.After(items[j].CapturedAt)
	})
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"article_public_id": item.ID,
			"title":             item.Title,
			"publish_time":      nonEmpty(item.PublishTimeText, item.PublishTime, item.CapturedAt.Format("2006-01-02 15:04:05")),
			"source_name":       nonEmpty(item.FromText, item.SourceType),
			"emotion":           articleEmotionIndex(item),
			"emotion_name":      emotionName(articleEmotionIndex(item)),
		})
	}
	return result
}

func (b legacyAnalysisBundle) buildPlanWordHitPayload() []map[string]any {
	keywords := keywordHotspotsFromProject(b.Project, b.Hotspots)
	total := 0
	for _, item := range keywords {
		total += item.Count
	}
	if total == 0 {
		total = len(keywords)
	}
	result := make([]map[string]any, 0, len(keywords))
	for i, item := range keywords {
		result = append(result, map[string]any{
			"keyword": item.Keyword,
			"count":   item.Count,
			"rate":    fmt.Sprintf("%d%%", percentage(item.Count, maxInt(total, 1))),
			"trend":   trendFromIndex(i),
		})
	}
	return result
}

func (b legacyAnalysisBundle) buildPopularInformationPayload() []map[string]any {
	latest := b.buildLatestNewsPayload()
	result := make([]map[string]any, 0, len(latest))
	total := maxInt(len(latest), 1)
	for i, item := range latest {
		result = append(result, map[string]any{
			"title":             item["title"],
			"article_public_id": item["article_public_id"],
			"count":             total - i,
			"rate":              fmt.Sprintf("%d%%", percentage(total-i, total)),
			"publish_time":      item["publish_time"],
			"source_name":       item["source_name"],
		})
	}
	return result
}

func (b legacyAnalysisBundle) buildEmotionCategoryPayload() map[string]any {
	trend := make([][]any, 0, minInt(len(b.buildLatestNewsPayload()), 7))
	for i, item := range b.buildLatestNewsPayload() {
		if i >= 7 {
			break
		}
		trend = append(trend, []any{
			item["publish_time"],
			articleEmotionIndexFromValue(item["emotion"]),
			maxInt(1, i+1),
		})
	}
	if len(trend) == 0 {
		trend = append(trend, []any{"暂无数据", 0, 0})
	}
	hotEvents := make([]map[string]any, 0, len(b.Events))
	for _, event := range b.Events {
		hotEvents = append(hotEvents, map[string]any{
			"keyword":   event.Keyword,
			"count":     event.Count,
			"key_words": mustJSONString(map[string]int{event.Keyword: event.Count}),
		})
	}
	if len(hotEvents) == 0 {
		hotEvents = append(hotEvents, map[string]any{
			"keyword":   "默认",
			"count":     0,
			"key_words": mustJSONString(map[string]int{"默认": 0}),
		})
	}
	return map[string]any{
		"china": fmt.Sprintf("当前项目共有 %d 条分析资讯，关键词聚合已完成。", len(b.Articles.Items)),
		"data": map[string]any{
			"keyword_emotion_trend": mustJSONString(map[string]any{
				"text":  "关键词情感走势",
				"chart": trend,
			}),
			"hot_event_ranking": mustJSONString(map[string]any{
				"all":      hotEvents,
				"positive": hotEvents,
				"negative": hotEvents,
			}),
			"highword_cloud": mustJSONString(keywordCloudPayload(b.Hotspots, b.Project)),
		},
	}
}

func (b legacyAnalysisBundle) buildKeywordIndexPayload() []map[string]any {
	keywords := keywordHotspotsFromProject(b.Project, b.Hotspots)
	total := 0
	for _, item := range keywords {
		total += item.Count
	}
	if total == 0 {
		total = len(keywords)
	}
	result := make([]map[string]any, 0, len(keywords))
	for i, item := range keywords {
		result = append(result, map[string]any{
			"keyword": item.Keyword,
			"count":   item.Count,
			"index":   fmt.Sprintf("%d", maxInt(1, total-i)),
			"trend":   trendFromIndex(i),
		})
	}
	return result
}

func (b legacyAnalysisBundle) buildPopularKeywordPayload() map[string]any {
	latest := b.buildLatestNewsPayload()
	keywords := keywordHotspotsFromProject(b.Project, b.Hotspots)
	if len(keywords) == 0 {
		keywords = []model.KeywordHotspot{{Keyword: b.Project.Keywords, Count: len(b.Articles.Items)}}
	}
	hotCompany := make([]map[string]any, 0, len(keywords))
	hotPeople := make([]map[string]any, 0, len(keywords))
	hotSpot := make([]map[string]any, 0, len(keywords))
	for i, item := range keywords {
		source := map[string]any{
			"keyword":           item.Keyword,
			"count":             item.Count,
			"index":             fmt.Sprintf("%d", maxInt(1, item.Count)),
			"trend":             trendFromIndex(i),
			"title":             latestTitleFromIndex(latest, i),
			"article_public_id": latestIDFromIndex(latest, i),
			"emotionalIndex":    articleEmotionIndexFromValue(latestEmotionFromIndex(latest, i)),
		}
		hotCompany = append(hotCompany, source)
		hotPeople = append(hotPeople, source)
		hotSpot = append(hotSpot, source)
	}
	return map[string]any{
		"updateTime": time.Now().Format("2006-01-02 15:04:05"),
		"hotCompany": hotCompany,
		"hotPeople":  hotPeople,
		"hotSpot":    hotSpot,
	}
}

func (b legacyAnalysisBundle) buildLegacyAnalysisSnapshot() map[string]any {
	overview := b.buildOverviewPayload()
	emotion := b.buildEmotionPayload()
	planWordHit := b.buildPlanWordHitPayload()
	latest := b.buildLatestNewsPayload()
	return map[string]any{
		"create_time":                 time.Now().Format("2006-01-02 15:04:05"),
		"project_id":                  b.Project.ID,
		"time_period":                 b.TimePeriod,
		"data_overview":               mustJSONString(overview),
		"relative_news":               mustJSONString(latest),
		"emotional_proportion":        mustJSONString(emotion),
		"plan_word_hit":               mustJSONString(planWordHit),
		"keyword_emotion_trend":       mustJSONString(map[string]any{"text": "关键词情感走势", "chart": emotion["chart"]}),
		"hot_event_ranking":           mustJSONString(map[string]any{"all": planWordHit, "positive": planWordHit, "negative": planWordHit}),
		"highword_cloud":              mustJSONString(keywordCloudPayload(b.Hotspots, b.Project)),
		"keyword_index":               mustJSONString(b.buildKeywordIndexPayload()),
		"media_activity_analysis":     mustJSONString(mediaActivityPayload(b)),
		"hot_spot_ranking":            mustJSONString(hotSpotRankingPayload(b)),
		"data_source_distribution":    mustJSONString(dataSourceDistributionPayload(b)),
		"data_source_analysis":        mustJSONString(dataSourceAnalysisPayload(b)),
		"keyword_exposure_rank":       mustJSONString(keywordExposurePayload(b)),
		"selfmedia_volume_rank":       mustJSONString(selfMediaPayload(b)),
		"category_rank":               mustJSONString(b.buildPlanWordHitPayload()),
		"industrial_distribution":     mustJSONString(dataSourceDistributionPayload(b)),
		"event_statistics":            mustJSONString(eventStatisticsPayload(b)),
		"hotCompany":                  mustJSONString(b.buildPopularKeywordPayload()["hotCompany"]),
		"hotPeople":                   mustJSONString(b.buildPopularKeywordPayload()["hotPeople"]),
		"hotSpot":                     mustJSONString(b.buildPopularKeywordPayload()["hotSpot"]),
		"keyword_emotion_statistical": mustJSONString(keywordEmotionStatisticalPayload(b)),
		"isNeedRefresh":               b.Dashboard.Overview.AlertRuleCount > 0,
	}
}

func keywordHotspotsFromProject(project model.Project, hotspots []model.KeywordHotspot) []model.KeywordHotspot {
	if len(hotspots) > 0 {
		return hotspots
	}
	parts := strings.Split(project.Keywords, ",")
	result := make([]model.KeywordHotspot, 0, len(parts))
	for _, part := range parts {
		keyword := strings.TrimSpace(part)
		if keyword == "" {
			continue
		}
		result = append(result, model.KeywordHotspot{Keyword: keyword, Count: 1})
	}
	return result
}

func keywordCloudPayload(hotspots []model.KeywordHotspot, project model.Project) []map[string]any {
	keywords := keywordHotspotsFromProject(project, hotspots)
	result := make([]map[string]any, 0, len(keywords))
	for _, item := range keywords {
		result = append(result, map[string]any{"name": item.Keyword, "value": item.Count})
	}
	return result
}

func dataSourceDistributionPayload(b legacyAnalysisBundle) []map[string]any {
	counts := sourceCounts(b.Articles.Items)
	order := []string{"wechat", "weibo", "gov", "bbs", "news", "newspaper", "app", "web", "foreign_media", "video", "blog"}
	result := make([]map[string]any, 0, len(counts))
	for _, key := range order {
		if count := counts[key]; count > 0 {
			result = append(result, map[string]any{"name": sourceLabel(key), "value": count})
		}
	}
	if len(result) == 0 {
		result = append(result, map[string]any{"name": "网站", "value": maxInt(len(b.Articles.Items), 1)})
	}
	return result
}

func mediaActivityPayload(b legacyAnalysisBundle) map[string]any {
	counts := sourceCounts(b.Articles.Items)
	siteList := make([]map[string]any, 0, len(counts))
	total := 0
	for source, count := range counts {
		total += count
		siteList = append(siteList, map[string]any{
			"name":  sourceLabel(source),
			"logo":  "",
			"rate":  fmt.Sprintf("%d%%", percentage(count, maxInt(len(b.Articles.Items), 1))),
			"count": count,
		})
	}
	if total == 0 {
		total = maxInt(len(b.Articles.Items), 1)
	}
	return map[string]any{
		"total_site": maxInt(len(siteList), 1),
		"total":      total,
		"sites":      siteList,
	}
}

func dataSourceAnalysisPayload(b legacyAnalysisBundle) map[string]any {
	counts := sourceCounts(b.Articles.Items)
	makeList := func(keys []string) []map[string]any {
		result := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			count := counts[key]
			if count == 0 {
				continue
			}
			result = append(result, map[string]any{
				"name":           sourceLabel(key),
				"type":           sourceLabel(key),
				"allCount":       count,
				"sensitiveCount": maxInt(count/3, 0),
			})
		}
		if len(result) == 0 {
			result = append(result, map[string]any{
				"name":           "网站",
				"type":           "网站",
				"allCount":       maxInt(len(b.Articles.Items), 1),
				"sensitiveCount": 0,
			})
		}
		return result
	}
	return map[string]any{
		"all":           makeList([]string{"wechat", "weibo", "gov", "bbs", "news", "newspaper", "app", "web", "foreign_media", "video", "blog"}),
		"wechat":        makeList([]string{"wechat"}),
		"weibo":         makeList([]string{"weibo"}),
		"gov":           makeList([]string{"gov"}),
		"bbs":           makeList([]string{"bbs"}),
		"news":          makeList([]string{"news"}),
		"newspaper":     makeList([]string{"newspaper"}),
		"app":           makeList([]string{"app"}),
		"web":           makeList([]string{"web"}),
		"foreign_media": makeList([]string{"foreign_media"}),
		"video":         makeList([]string{"video"}),
		"blog":          makeList([]string{"blog"}),
	}
}

func keywordExposurePayload(b legacyAnalysisBundle) []map[string]any {
	keywords := keywordHotspotsFromProject(b.Project, b.Hotspots)
	result := make([]map[string]any, 0, len(keywords))
	for i, item := range keywords {
		result = append(result, map[string]any{
			"keyword":       item.Keyword,
			"total":         item.Count,
			"positive_rate": 50 + i*5,
			"negative_rate": 20 + i*3,
			"chain_growth":  strconv.Itoa(item.Count * 10),
			"rank":          i + 1,
		})
	}
	return result
}

func selfMediaPayload(b legacyAnalysisBundle) []map[string]any {
	counts := sourceCounts(b.Articles.Items)
	result := make([]map[string]any, 0, len(counts))
	for source, count := range counts {
		result = append(result, map[string]any{
			"name":           sourceLabel(source),
			"logo":           "",
			"platform_name":  sourceLabel(source),
			"platform_names": sourceLabel(source),
			"release_count":  count,
			"fans_count":     count * 10,
			"volume":         count,
			"email":          "",
		})
	}
	if len(result) == 0 {
		result = append(result, map[string]any{
			"name":           "网站",
			"logo":           "",
			"platform_name":  "网站",
			"platform_names": "网站",
			"release_count":  1,
			"fans_count":     10,
			"volume":         1,
			"email":          "",
		})
	}
	return result
}

func eventStatisticsPayload(b legacyAnalysisBundle) []map[string]any {
	result := make([]map[string]any, 0, len(b.Events))
	for _, event := range b.Events {
		result = append(result, map[string]any{
			"keyword": event.Keyword,
			"count":   event.Count,
		})
	}
	if len(result) == 0 {
		result = append(result, map[string]any{"keyword": "默认", "count": 1})
	}
	return result
}

func hotSpotRankingPayload(b legacyAnalysisBundle) []map[string]any {
	counts := sourceCounts(b.Articles.Items)
	result := make([]map[string]any, 0, len(counts))
	for source, count := range counts {
		result = append(result, map[string]any{
			"name":  sourceLabel(source),
			"count": count,
			"rate":  percentage(count, maxInt(len(b.Articles.Items), 1)),
		})
	}
	if len(result) == 0 {
		result = append(result, map[string]any{"name": "网站", "count": 1, "rate": 100})
	}
	return result
}

func keywordEmotionStatisticalPayload(b legacyAnalysisBundle) map[string]any {
	keywords := keywordHotspotsFromProject(b.Project, b.Hotspots)
	return map[string]any{
		"keyword_count": maxInt(len(keywords), 1),
		"positive": []map[string]any{
			{"keyword": keywordFromIndex(keywords, 0), "rate": 60},
		},
		"negative": []map[string]any{
			{"keyword": keywordFromIndex(keywords, 0), "rate": 20},
		},
	}
}

func emotionCount(emotions model.EmotionAnalysis, name string) int {
	for _, bucket := range emotions.Buckets {
		if strings.EqualFold(bucket.Name, name) {
			return bucket.Count
		}
	}
	return 0
}

func emotionName(emotion int) string {
	switch emotion {
	case 1:
		return "正面"
	case 3:
		return "负面"
	default:
		return "中性"
	}
}

func articleEmotionIndexFromValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(v))
		return parsed
	default:
		return 2
	}
}

func trendFromIndex(index int) int {
	switch {
	case index == 0:
		return 1
	case index == 1:
		return 2
	default:
		return 3
	}
}

func percentage(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(part) * 100 / float64(total))
}

func sourceCounts(items []model.Item) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		key := normalizeSourceKey(item.SourceType)
		counts[key]++
	}
	if len(counts) == 0 {
		counts["web"] = 1
	}
	return counts
}

func normalizeSourceKey(sourceType string) string {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "wechat", "weixin":
		return "wechat"
	case "weibo":
		return "weibo"
	case "gov", "government":
		return "gov"
	case "bbs", "forum":
		return "bbs"
	case "headline", "news":
		return "news"
	case "newspaper", "paper":
		return "newspaper"
	case "app":
		return "app"
	case "foreign", "foreign_media":
		return "foreign_media"
	case "video":
		return "video"
	case "blog":
		return "blog"
	default:
		return "web"
	}
}

func sourceLabel(sourceKey string) string {
	switch sourceKey {
	case "wechat":
		return "微信"
	case "weibo":
		return "微博"
	case "gov":
		return "政务"
	case "bbs":
		return "论坛"
	case "news":
		return "新闻"
	case "newspaper":
		return "报刊"
	case "app":
		return "客户端"
	case "web":
		return "网站"
	case "foreign_media":
		return "外媒"
	case "video":
		return "视频"
	case "blog":
		return "博客"
	default:
		return sourceKey
	}
}

func latestTitleFromIndex(items []map[string]any, index int) string {
	if index < 0 || index >= len(items) {
		return ""
	}
	return legacyStringFromAny(items[index]["title"])
}

func latestIDFromIndex(items []map[string]any, index int) int64 {
	if index < 0 || index >= len(items) {
		return 0
	}
	switch value := items[index]["article_public_id"].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		parsed, _ := strconv.ParseInt(legacyStringFromAny(value), 10, 64)
		return parsed
	}
}

func latestEmotionFromIndex(items []map[string]any, index int) int {
	if index < 0 || index >= len(items) {
		return 2
	}
	return articleEmotionIndexFromValue(items[index]["emotion"])
}

func keywordFromIndex(items []model.KeywordHotspot, index int) string {
	if index < 0 || index >= len(items) {
		return "默认"
	}
	return items[index].Keyword
}
