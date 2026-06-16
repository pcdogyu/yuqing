package portal

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type aStockContext struct {
	Date            string
	WindowStart     time.Time
	WindowEnd       time.Time
	Articles        []model.Item
	Hotspots        []aStockHotspot
	Recommendations []aStockRecommendation
	LoadMessage     string
}

type aStockHotspot struct {
	Name         string
	Keywords     []string
	Score        int
	Evidence     int
	MatchedItems []model.Item
}

type aStockRecommendation struct {
	Rank    int
	Hotspot string
	Code    string
	Name    string
	Reason  string
}

type aStockTopicRule struct {
	Name     string
	Keywords []string
	Stocks   []aStockStockPick
}

type aStockStockPick struct {
	Code string
	Name string
}

func (s *Server) handleAStockPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleAStockPageAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	strategyDate := normalizeAStockStrategyDate(r.URL.Query().Get("date"))
	ctx := s.loadAStockContext(strategyDate)
	message := strings.TrimSpace(r.URL.Query().Get("msg"))
	if message == "" {
		message = ctx.LoadMessage
	}

	var b strings.Builder
	b.WriteString(`<style>
		body[data-page='a-stock'] header,body[data-page='a-stock'] main,body[data-page='a-stock'] .site-footer{max-width:1534px}
		.astock-hero{display:grid;grid-template-columns:minmax(0,1.3fr) minmax(280px,.7fr);gap:16px;align-items:stretch}
		.astock-card{padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#fff}
		.astock-soft{background:#faf8f2}
		.astock-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px}
		.astock-actions{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px}
		.astock-actions form{margin:0}
		.astock-actions button{margin:0}
		.astock-muted{color:#6a6257}
		.astock-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:12px;background:#fff;color:#6a6257}
		.astock-badge{display:inline-flex;align-items:center;padding:5px 9px;border-radius:999px;background:#eef4ec;color:#214e34;font-size:13px;margin-right:6px}
		.astock-source-list{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
		.astock-table{min-width:960px}
		.astock-scroll{overflow:auto}
		@media (max-width: 760px){.astock-hero{grid-template-columns:1fr}}
	</style>`)
	if message != "" {
		b.WriteString(`<section><p style="color:#214e34">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p></section>`)
	}

	b.WriteString(`<section class="astock-hero"><div class="astock-card astock-soft"><h2>A股策略工作台</h2><p>`)
	b.WriteString(fmt.Sprintf(`欢迎，用户 %d。`, userIDFromMap(user)))
	b.WriteString(`本页用于承载 09:00-09:25 财经新闻热点归纳、推荐股票和消息回测结果。</p><p class="astock-muted">仅供策略研究和回测，不构成投资建议。</p></div><div class="astock-card"><form method="get"><label>策略日期</label><input type="date" name="date" value="`)
	b.WriteString(html.EscapeString(strategyDate))
	b.WriteString(`"><button type="submit">查看日期</button></form></div></section>`)

	b.WriteString(`<section><h2>顶部概览</h2><div class="astock-grid">`)
	writeAStockMetric(&b, "策略日期", ctx.Date)
	writeAStockMetric(&b, "新闻窗口", "09:00-09:25")
	writeAStockMetric(&b, "财经新闻数", fmt.Sprintf("%d", len(ctx.Articles)))
	writeAStockMetric(&b, "候选热点数", fmt.Sprintf("%d", len(ctx.Hotspots)))
	writeAStockMetric(&b, "推荐股票数", fmt.Sprintf("%d", len(ctx.Recommendations)))
	writeAStockMetric(&b, "回测状态", "等待 Tushare")
	b.WriteString(`</div></section>`)

	b.WriteString(`<section><h2>操作区</h2><div class="astock-actions">`)
	for _, action := range []struct {
		Name  string
		Label string
	}{
		{Name: "crawl", Label: "抓取 A 股新闻"},
		{Name: "generate", Label: "生成今日热点"},
		{Name: "sync_market", Label: "同步 Tushare 行情"},
		{Name: "refresh_backtest", Label: "刷新回测结果"},
	} {
		b.WriteString(`<form method="post"><input type="hidden" name="date" value="`)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(`"><input type="hidden" name="action" value="`)
		b.WriteString(html.EscapeString(action.Name))
		b.WriteString(`"><button type="submit">`)
		b.WriteString(html.EscapeString(action.Label))
		b.WriteString(`</button></form>`)
	}
	b.WriteString(`</div><p class="astock-muted">已接入已有新闻抓取链路：抓取按钮会触发金十快讯和金十资讯，页面按策略日期 09:00-09:25 聚合财经新闻。Tushare 行情和真实回测仍需配置行情源后接入。</p><div class="astock-source-list"><span class="astock-badge">flash: https://www.jin10.com/</span><span class="astock-badge">headline: https://xnews.jin10.com/</span></div></section>`)

	renderAStockNewsSection(&b, ctx)
	renderAStockHotspotSection(&b, ctx.Hotspots)
	renderAStockRecommendationSection(&b, ctx.Recommendations)
	renderAStockBacktestSection(&b)

	_ = s.writeSimplePage(w, "a-stock", "A股策略工作台", b.String())
}

func (s *Server) handleAStockPageAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	query := url.Values{}
	if date := normalizeAStockStrategyDate(r.FormValue("date")); date != "" {
		query.Set("date", date)
	}
	switch strings.TrimSpace(r.FormValue("action")) {
	case "crawl":
		query.Set("msg", s.triggerAStockCrawl())
	case "generate":
		query.Set("msg", "热点已按当前新闻窗口重新计算。")
	case "sync_market":
		query.Set("msg", "Tushare 行情同步接口待配置，当前先展示新闻和热点。")
	case "refresh_backtest":
		query.Set("msg", "回测需要 Tushare 行情数据，当前先展示待接入状态。")
	default:
		query.Set("msg", "未知操作")
	}
	http.Redirect(w, r, "/a-stock?"+query.Encode(), http.StatusSeeOther)
}

func writeAStockMetric(b *strings.Builder, label string, value string) {
	b.WriteString(`<div class="astock-card"><span class="astock-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong style="display:block;margin-top:8px;font-size:24px">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func renderAStockNewsSection(b *strings.Builder, ctx aStockContext) {
	b.WriteString(`<section><h2>09:00-09:25 财经新闻</h2>`)
	if len(ctx.Articles) == 0 {
		b.WriteString(`<div class="astock-empty">暂无数据：请点击“抓取 A 股新闻”，或确认 `)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(` 09:00-09:25 窗口内已有金十新闻入库。</div><table><tr><th>标题</th><th>来源</th><th>时间</th><th>命中关键词</th></tr><tr><td colspan="4">暂无 09:00-09:25 新闻</td></tr></table></section>`)
		return
	}
	b.WriteString(`<table><tr><th>标题</th><th>来源</th><th>时间</th><th>命中关键词</th></tr>`)
	for _, item := range ctx.Articles {
		b.WriteString(`<tr><td><a class="inline" href="/articles/`)
		b.WriteString(fmt.Sprintf("%d", item.ID))
		b.WriteString(`?return_to=%2Fa-stock%3Fdate%3D`)
		b.WriteString(url.QueryEscape(ctx.Date))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(item.Title))
		b.WriteString(`</a></td><td>`)
		b.WriteString(html.EscapeString(articleSourceSiteLabel(item)))
		b.WriteString(` / `)
		b.WriteString(html.EscapeString(item.SourceType))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(formatShanghaiTime(item.CapturedAt)))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(strings.Join(aStockMatchedKeywords(item), "、")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
}

func renderAStockHotspotSection(b *strings.Builder, hotspots []aStockHotspot) {
	b.WriteString(`<section><h2>热点归纳</h2><p><span class="astock-badge">规则+词典</span><span class="astock-badge">可复现回测</span></p>`)
	if len(hotspots) == 0 {
		b.WriteString(`<div class="astock-empty">暂无数据：当前新闻窗口未命中 A 股热点词典。</div><table><tr><th>热点</th><th>关键词</th><th>热度分</th><th>证据新闻数</th></tr><tr><td colspan="4">暂无热点</td></tr></table></section>`)
		return
	}
	b.WriteString(`<table><tr><th>热点</th><th>关键词</th><th>热度分</th><th>证据新闻数</th></tr>`)
	for _, hotspot := range hotspots {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(hotspot.Name))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(strings.Join(hotspot.Keywords, "、")))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", hotspot.Score))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", hotspot.Evidence))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
}

func renderAStockRecommendationSection(b *strings.Builder, recommendations []aStockRecommendation) {
	b.WriteString(`<section><h2>推荐股票</h2>`)
	if len(recommendations) == 0 {
		b.WriteString(`<div class="astock-empty">暂无数据：当前新闻窗口未生成热点映射股票。</div><table><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>推荐理由</th></tr><tr><td colspan="5">暂无推荐股票</td></tr></table></section>`)
		return
	}
	b.WriteString(`<table><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>推荐理由</th></tr>`)
	for _, rec := range recommendations {
		b.WriteString(`<tr><td>`)
		b.WriteString(fmt.Sprintf("%d", rec.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Hotspot))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Code))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Name))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Reason))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
}

func renderAStockBacktestSection(b *strings.Builder) {
	b.WriteString(`<section><h2>消息回测</h2><p class="astock-muted">买入价采用当日开盘价；T+1 到 T+5 按后续交易日收盘价计算收益，并展示五日内最高收益。</p><div class="astock-scroll"><table class="astock-table"><tr><th>股票</th><th>当日开盘价</th><th>T+1 收盘价</th><th>T+1 收益</th><th>T+2 收盘价</th><th>T+2 收益</th><th>T+3 收盘价</th><th>T+3 收益</th><th>T+4 收盘价</th><th>T+4 收益</th><th>T+5 收盘价</th><th>T+5 收益</th><th>五日内最高收益</th><th>命中状态</th></tr><tr><td colspan="14">暂无回测结果，等待 Tushare 行情同步。</td></tr></table></div></section>`)
}

func (s *Server) loadAStockContext(strategyDate string) aStockContext {
	start, end := aStockWindow(strategyDate)
	ctx := aStockContext{
		Date:        strategyDate,
		WindowStart: start,
		WindowEnd:   end,
	}
	result := model.ItemListResult{}
	query := "/api/v1/articles?page=1&page_size=200&start=" + url.QueryEscape(start.UTC().Format(time.RFC3339)) + "&end=" + url.QueryEscape(end.UTC().Format(time.RFC3339))
	if err := s.getJSON(s.cfg.ContentURL+query, &result); err != nil {
		ctx.LoadMessage = "A股新闻读取失败：" + err.Error()
		return ctx
	}
	ctx.Articles = filterAStockNews(result.Items)
	ctx.Hotspots = buildAStockHotspots(ctx.Articles)
	ctx.Recommendations = buildAStockRecommendations(ctx.Hotspots)
	return ctx
}

func (s *Server) triggerAStockCrawl() string {
	sources := []string{"flash", "headline"}
	ok := 0
	failures := make([]string, 0)
	for _, sourceType := range sources {
		resp, err := s.client.R().
			SetQueryParam("source_type", sourceType).
			Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		if err != nil || !resp.IsSuccess() {
			failures = append(failures, sourceType)
			continue
		}
		ok++
	}
	if len(failures) > 0 {
		return fmt.Sprintf("A股新闻抓取部分触发：成功 %d 个，失败 %s", ok, strings.Join(failures, "、"))
	}
	return "A股新闻抓取已触发：金十快讯、金十资讯"
}

func aStockWindow(strategyDate string) (time.Time, time.Time) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	day, err := time.ParseInLocation("2006-01-02", strategyDate, location)
	if err != nil {
		day = time.Now().In(location)
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, location)
	end := time.Date(day.Year(), day.Month(), day.Day(), 9, 25, 59, 0, location)
	return start, end
}

func filterAStockNews(items []model.Item) []model.Item {
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		sourceType := strings.TrimSpace(item.SourceType)
		if sourceType != "flash" && sourceType != "headline" && sourceType != "jin10_full" {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CapturedAt.Before(filtered[j].CapturedAt)
	})
	return filtered
}

func buildAStockHotspots(items []model.Item) []aStockHotspot {
	rules := aStockTopicRules()
	hotspots := make([]aStockHotspot, 0, len(rules))
	for _, rule := range rules {
		keywordSet := make(map[string]struct{})
		matches := make([]model.Item, 0)
		for _, item := range items {
			text := strings.ToLower(item.Title + " " + item.Summary + " " + item.Content)
			itemMatched := false
			for _, keyword := range rule.Keywords {
				if strings.Contains(text, strings.ToLower(keyword)) {
					keywordSet[keyword] = struct{}{}
					itemMatched = true
				}
			}
			if itemMatched {
				matches = append(matches, item)
			}
		}
		if len(matches) == 0 {
			continue
		}
		keywords := make([]string, 0, len(keywordSet))
		for keyword := range keywordSet {
			keywords = append(keywords, keyword)
		}
		sort.Strings(keywords)
		hotspots = append(hotspots, aStockHotspot{
			Name:         rule.Name,
			Keywords:     keywords,
			Score:        len(matches)*10 + len(keywords)*3,
			Evidence:     len(matches),
			MatchedItems: matches,
		})
	}
	sort.SliceStable(hotspots, func(i, j int) bool {
		if hotspots[i].Score == hotspots[j].Score {
			return hotspots[i].Name < hotspots[j].Name
		}
		return hotspots[i].Score > hotspots[j].Score
	})
	if len(hotspots) > 8 {
		return hotspots[:8]
	}
	return hotspots
}

func buildAStockRecommendations(hotspots []aStockHotspot) []aStockRecommendation {
	ruleByName := make(map[string]aStockTopicRule)
	for _, rule := range aStockTopicRules() {
		ruleByName[rule.Name] = rule
	}
	recommendations := make([]aStockRecommendation, 0)
	seen := make(map[string]struct{})
	for _, hotspot := range hotspots {
		rule, ok := ruleByName[hotspot.Name]
		if !ok {
			continue
		}
		for _, stock := range rule.Stocks {
			if _, exists := seen[stock.Code]; exists {
				continue
			}
			seen[stock.Code] = struct{}{}
			recommendations = append(recommendations, aStockRecommendation{
				Rank:    len(recommendations) + 1,
				Hotspot: hotspot.Name,
				Code:    stock.Code,
				Name:    stock.Name,
				Reason:  fmt.Sprintf("命中 %s，证据新闻 %d 条，热度分 %d", strings.Join(hotspot.Keywords, "、"), hotspot.Evidence, hotspot.Score),
			})
			if len(recommendations) >= 12 {
				return recommendations
			}
		}
	}
	return recommendations
}

func aStockMatchedKeywords(item model.Item) []string {
	keywordSet := make(map[string]struct{})
	text := strings.ToLower(item.Title + " " + item.Summary + " " + item.Content)
	for _, rule := range aStockTopicRules() {
		for _, keyword := range rule.Keywords {
			if strings.Contains(text, strings.ToLower(keyword)) {
				keywordSet[keyword] = struct{}{}
			}
		}
	}
	keywords := make([]string, 0, len(keywordSet))
	for keyword := range keywordSet {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	return keywords
}

func aStockTopicRules() []aStockTopicRule {
	return []aStockTopicRule{
		{Name: "人工智能", Keywords: []string{"AI", "人工智能", "大模型", "算力", "AIGC", "机器人"}, Stocks: []aStockStockPick{{Code: "002230", Name: "科大讯飞"}, {Code: "603019", Name: "中科曙光"}, {Code: "601138", Name: "工业富联"}}},
		{Name: "半导体", Keywords: []string{"半导体", "芯片", "光刻", "晶圆", "存储", "先进封装"}, Stocks: []aStockStockPick{{Code: "688981", Name: "中芯国际"}, {Code: "002371", Name: "北方华创"}, {Code: "603986", Name: "兆易创新"}}},
		{Name: "新能源", Keywords: []string{"新能源", "锂电", "储能", "光伏", "风电", "充电桩"}, Stocks: []aStockStockPick{{Code: "300750", Name: "宁德时代"}, {Code: "300274", Name: "阳光电源"}, {Code: "601012", Name: "隆基绿能"}}},
		{Name: "低空经济", Keywords: []string{"低空经济", "eVTOL", "无人机", "通航", "飞行汽车"}, Stocks: []aStockStockPick{{Code: "000099", Name: "中信海直"}, {Code: "002085", Name: "万丰奥威"}, {Code: "300124", Name: "汇川技术"}}},
		{Name: "金融券商", Keywords: []string{"券商", "证券", "银行", "保险", "降准", "降息", "资本市场"}, Stocks: []aStockStockPick{{Code: "300059", Name: "东方财富"}, {Code: "600030", Name: "中信证券"}, {Code: "600036", Name: "招商银行"}}},
		{Name: "黄金有色", Keywords: []string{"黄金", "有色", "铜", "铝", "稀土", "贵金属"}, Stocks: []aStockStockPick{{Code: "600547", Name: "山东黄金"}, {Code: "601899", Name: "紫金矿业"}, {Code: "600111", Name: "北方稀土"}}},
		{Name: "医药生物", Keywords: []string{"医药", "创新药", "疫苗", "医疗器械", "CXO"}, Stocks: []aStockStockPick{{Code: "600276", Name: "恒瑞医药"}, {Code: "300760", Name: "迈瑞医疗"}, {Code: "603259", Name: "药明康德"}}},
		{Name: "消费电子", Keywords: []string{"消费电子", "苹果", "华为", "手机", "MR", "AR", "VR"}, Stocks: []aStockStockPick{{Code: "002475", Name: "立讯精密"}, {Code: "000725", Name: "京东方A"}, {Code: "300433", Name: "蓝思科技"}}},
		{Name: "房地产", Keywords: []string{"房地产", "地产", "房贷", "楼市", "保障房"}, Stocks: []aStockStockPick{{Code: "000002", Name: "万科A"}, {Code: "001979", Name: "招商蛇口"}, {Code: "600048", Name: "保利发展"}}},
		{Name: "军工航天", Keywords: []string{"军工", "航天", "卫星", "商业航天", "航空发动机"}, Stocks: []aStockStockPick{{Code: "600760", Name: "中航沈飞"}, {Code: "600893", Name: "航发动力"}, {Code: "002179", Name: "中航光电"}}},
	}
}

func normalizeAStockStrategyDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			return parsed.Format("2006-01-02")
		}
		return raw
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.Local
	}
	return time.Now().In(location).Format("2006-01-02")
}
