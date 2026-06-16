package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type aStockContext struct {
	Date            string
	Period          string
	PeriodLabel     string
	WindowLabel     string
	NewsPage        int
	NewsPageSize    int
	NewsTotal       int
	NewsTotalPages  int
	WindowStart     time.Time
	WindowEnd       time.Time
	Articles        []model.Item
	PagedArticles   []model.Item
	Hotspots        []aStockHotspot
	Recommendations []aStockRecommendation
	Backtests       []aStockBacktestRow
	LoadMessage     string
	BacktestStatus  string
	EmptyReason     string
}

type aStockHotspot struct {
	Name         string
	Keywords     []string
	Score        int
	Evidence     int
	MatchedItems []model.Item
}

type aStockRecommendation struct {
	Rank          int
	Hotspot       string
	Code          string
	Name          string
	HotspotScore  int
	MarketScore   int
	PrevClose     string
	PrevPct       string
	PrevPctClass  string
	Change30      string
	Change30Class string
	Change60      string
	Change60Class string
	CurrentPrice  string
	TodayPct      string
	TodayPctClass string
	Reason        string
}

type aStockMarketBar struct {
	Code  string
	Date  string
	Open  float64
	Close float64
	Pct   float64
}

type aStockBacktestCell struct {
	Close       string
	Return      string
	ReturnClass string
}

type aStockBacktestRow struct {
	Stock           string
	EntryOpen       string
	Days            []aStockBacktestCell
	BestReturn      string
	BestReturnClass string
	Status          string
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

type aStockPeriod struct {
	Key         string
	Label       string
	WindowLabel string
}

const (
	aStockDrawdownFilterThreshold = -15.0
	aStockSectorDrawdownPenalty   = 15
	aStockNewsPageSize            = 10
)

var (
	aStockNow               = time.Now
	aStockEastmoneyKlineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	aStockYahooChartURL     = "https://query1.finance.yahoo.com/v8/finance/chart/"
)

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
	period := normalizeAStockPeriod(r.URL.Query().Get("period"))
	newsPage := normalizeAStockNewsPage(r.URL.Query().Get("news_page"))
	ctx := s.loadAStockContext(strategyDate, period.Key, newsPage)
	message := strings.TrimSpace(r.URL.Query().Get("msg"))
	if message == "" {
		message = ctx.LoadMessage
	}

	var b strings.Builder
	b.WriteString(`<style>
		body[data-page='a-stock'] main,body[data-page='a-stock'] .site-footer{max-width:1534px}
		.astock-hero{display:grid;grid-template-columns:minmax(0,1.3fr) minmax(280px,.7fr);gap:16px;align-items:stretch}
		.astock-card{padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#fff}
		.astock-soft{background:#faf8f2}
		.astock-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px}
		.astock-actions{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px}
		.astock-actions form{margin:0}
		.astock-actions button{margin:0}
		.astock-form-actions{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-top:12px}
		.astock-button-link{display:inline-flex;align-items:center;justify-content:center;min-height:42px;border-radius:8px;background:#214e34;color:#fff;text-decoration:none;font-weight:700}
		.astock-muted{color:#6a6257}
		.astock-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:12px;background:#fff;color:#6a6257}
		.astock-badge{display:inline-flex;align-items:center;padding:5px 9px;border-radius:999px;background:#eef4ec;color:#214e34;font-size:13px;margin-right:6px}
		.astock-source-list{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
		.astock-table{min-width:960px}
		.astock-scroll{overflow:auto}
		.astock-date-tabs{display:flex;gap:8px;flex-wrap:wrap;margin:14px 0 18px}
		.astock-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
		.astock-tab{display:inline-flex;align-items:center;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
		.astock-tab.active{background:#214e34;color:#fff;border-color:#214e34}
		.astock-tab.disabled{color:#9a9388;border-color:#ece7dc;background:#faf8f2;pointer-events:none}
		.astock-pagination{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-top:14px}
		.astock-up{color:#b3261e;font-weight:700}
		.astock-down{color:#1b7f3a;font-weight:700}
		.astock-flat{color:#6a6257}
		@media (max-width: 760px){.astock-hero{grid-template-columns:1fr}}
	</style>`)
	b.WriteString(`<script>
	(function(){
		var key="astock-scroll-y";
		window.addEventListener("DOMContentLoaded",function(){
			var y=sessionStorage.getItem(key);
			if(y!==null){sessionStorage.removeItem(key);var n=parseInt(y,10);if(!isNaN(n)){window.scrollTo(0,n);}}
			document.querySelectorAll("[data-preserve-scroll='1']").forEach(function(el){
				el.addEventListener("click",function(){sessionStorage.setItem(key,String(window.scrollY||0));});
			});
		});
	})();
	</script>`)
	if message != "" {
		b.WriteString(`<section><p style="color:#214e34">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p></section>`)
	}

	b.WriteString(`<section class="astock-hero"><div class="astock-card astock-soft"><h2>A股策略工作台</h2><p>`)
	b.WriteString(fmt.Sprintf(`欢迎，用户 %d。`, userIDFromMap(user)))
	b.WriteString(`本页用于承载上午、下午财经新闻热点归纳、推荐股票和消息回测结果。</p><p class="astock-muted">每日 09:25 自动抓取 09:00-09:25 新闻生成上午推荐；12:50 自动抓取 09:26-12:50 新闻生成下午推荐。仅供策略研究和回测，不构成投资建议。</p>`)
	renderAStockPeriodTabs(&b, ctx.Date, ctx.Period)
	b.WriteString(`</div><div class="astock-card"><form method="get"><label>策略日期</label><input type="date" name="date" value="`)
	b.WriteString(html.EscapeString(strategyDate))
	b.WriteString(`"><label>推荐窗口</label><select name="period">`)
	for _, option := range aStockPeriods() {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(option.Key))
		b.WriteString(`"`)
		if option.Key == ctx.Period {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(option.Label))
		b.WriteString(`</option>`)
	}
	today := aStockTodayDate()
	b.WriteString(`</select><div class="astock-form-actions"><button type="submit">查看日期</button><a class="astock-button-link" href="/a-stock?date=`)
	b.WriteString(url.QueryEscape(today))
	b.WriteString(`&period=`)
	b.WriteString(url.QueryEscape(ctx.Period))
	b.WriteString(`">回到今天</a></div></form></div></section>`)

	b.WriteString(`<section><h2>顶部概览</h2><div class="astock-grid">`)
	writeAStockMetric(&b, "策略日期", ctx.Date)
	writeAStockMetric(&b, "推荐窗口", ctx.PeriodLabel)
	writeAStockMetric(&b, "新闻窗口", ctx.WindowLabel)
	writeAStockMetric(&b, "财经新闻数", fmt.Sprintf("%d", len(ctx.Articles)))
	writeAStockMetric(&b, "候选热点数", fmt.Sprintf("%d", len(ctx.Hotspots)))
	writeAStockMetric(&b, "推荐股票数", fmt.Sprintf("%d", len(ctx.Recommendations)))
	writeAStockMetric(&b, "回测状态", ctx.BacktestStatus)
	b.WriteString(`</div></section>`)

	b.WriteString(`<section><h2>操作区</h2><div class="astock-actions">`)
	for _, action := range []struct {
		Name   string
		Label  string
		Period string
	}{
		{Name: "crawl", Label: "抓取全部财经信息", Period: ctx.Period},
		{Name: "backfill_window_news", Label: "补抓当前窗口新闻", Period: ctx.Period},
		{Name: "generate_morning_stock", Label: "重新生成上午推荐", Period: "morning"},
		{Name: "generate_afternoon_stock", Label: "重新生成下午推荐", Period: "afternoon"},
		{Name: "generate", Label: "生成今日热点", Period: ctx.Period},
		{Name: "sync_market", Label: "同步行情", Period: ctx.Period},
		{Name: "refresh_backtest", Label: "刷新回测结果", Period: ctx.Period},
	} {
		b.WriteString(`<form method="post"><input type="hidden" name="date" value="`)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(`"><input type="hidden" name="period" value="`)
		b.WriteString(html.EscapeString(action.Period))
		b.WriteString(`"><input type="hidden" name="action" value="`)
		b.WriteString(html.EscapeString(action.Name))
		b.WriteString(`"><button type="submit">`)
		b.WriteString(html.EscapeString(action.Label))
		b.WriteString(`</button></form>`)
	}
	b.WriteString(`</div><p class="astock-muted">已接入已有新闻抓取链路：抓取按钮会触发金十快讯、金十资讯、金十全站信息和东方财富网快讯，页面按策略日期和推荐窗口聚合财经新闻。行情接口读取 `)
	b.WriteString(aStockMarketConfigHint())
	b.WriteString(`，用于展示昨日收盘价、现价、涨跌幅和消息回测。</p><div class="astock-source-list"><span class="astock-badge">flash: https://www.jin10.com/</span><span class="astock-badge">headline: https://xnews.jin10.com/</span><span class="astock-badge">jin10_full: 金十全站</span><span class="astock-badge">eastmoney_kuaixun: 东方财富网</span></div></section>`)

	renderAStockNewsSection(&b, ctx)
	renderAStockHotspotSection(&b, ctx.Hotspots)
	renderAStockRecommendationSection(&b, ctx)
	renderAStockBacktestSection(&b, ctx.Date, ctx.Period, ctx.Recommendations, ctx.Backtests)

	_ = s.writeSimplePage(w, "a-stock", "A股策略工作台", b.String())
}

func (s *Server) handleAStockPageAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	query := url.Values{}
	if date := normalizeAStockStrategyDate(r.FormValue("date")); date != "" {
		query.Set("date", date)
	}
	period := normalizeAStockPeriod(r.FormValue("period"))
	query.Set("period", period.Key)
	switch strings.TrimSpace(r.FormValue("action")) {
	case "crawl":
		query.Set("msg", s.triggerAStockCrawl())
	case "backfill_window_news":
		date := normalizeAStockStrategyDate(r.FormValue("date"))
		query.Set("msg", s.triggerAStockWindowCrawl(date, period.Key))
	case "generate_morning_stock":
		period = normalizeAStockPeriod("morning")
		query.Set("period", period.Key)
		query.Set("msg", "已切换到上午窗口，按 09:00-09:25 历史新闻重新计算推荐。")
	case "generate_afternoon_stock":
		period = normalizeAStockPeriod("afternoon")
		query.Set("period", period.Key)
		query.Set("msg", "已切换到下午窗口，按 09:26-12:50 历史新闻重新计算推荐。")
	case "generate":
		query.Set("msg", period.Label+"热点已按当前新闻窗口重新计算。")
	case "sync_market":
		query.Set("msg", "行情已按当前策略日期刷新，页面已重新计算昨日收盘价、现价、涨跌幅和回测。")
	case "refresh_backtest":
		query.Set("msg", "消息回测已按当前推荐股票和行情数据刷新。")
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
	b.WriteString(`<section><h2>`)
	b.WriteString(html.EscapeString(ctx.WindowLabel))
	b.WriteString(` 财经新闻</h2>`)
	if len(ctx.Articles) == 0 {
		b.WriteString(`<div class="astock-empty">暂无数据：请点击“抓取 A 股新闻”，或确认 `)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(` `)
		b.WriteString(html.EscapeString(ctx.WindowLabel))
		b.WriteString(` 窗口内已有财经新闻源入库。</div><table><tr><th>标题</th><th>来源</th><th>时间</th><th>命中关键词</th></tr><tr><td colspan="4">暂无 `)
		b.WriteString(html.EscapeString(ctx.WindowLabel))
		b.WriteString(` 新闻</td></tr></table></section>`)
		return
	}
	b.WriteString(`<table><tr><th>标题</th><th>来源</th><th>时间</th><th>命中关键词</th></tr>`)
	for _, item := range ctx.PagedArticles {
		b.WriteString(`<tr><td><a class="inline" href="/articles/`)
		b.WriteString(fmt.Sprintf("%d", item.ID))
		b.WriteString(`?return_to=`)
		b.WriteString(url.QueryEscape(aStockPageHref(ctx.Date, ctx.Period, ctx.NewsPage)))
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
	b.WriteString(`</table>`)
	renderAStockNewsPagination(b, ctx)
	b.WriteString(`</section>`)
}

func renderAStockNewsPagination(b *strings.Builder, ctx aStockContext) {
	if ctx.NewsTotalPages <= 1 {
		return
	}
	b.WriteString(`<div class="astock-pagination"><span class="astock-muted">新闻分页：`)
	b.WriteString(fmt.Sprintf("%d/%d，共%d条", ctx.NewsPage, ctx.NewsTotalPages, ctx.NewsTotal))
	b.WriteString(`</span>`)
	renderAStockNewsPageLink(b, ctx, ctx.NewsPage-1, "上一页", ctx.NewsPage <= 1)
	for page := 1; page <= ctx.NewsTotalPages; page++ {
		renderAStockNewsPageLink(b, ctx, page, fmt.Sprintf("%d", page), false)
	}
	renderAStockNewsPageLink(b, ctx, ctx.NewsPage+1, "下一页", ctx.NewsPage >= ctx.NewsTotalPages)
	b.WriteString(`</div>`)
}

func renderAStockNewsPageLink(b *strings.Builder, ctx aStockContext, page int, label string, disabled bool) {
	b.WriteString(`<a class="astock-tab`)
	if page == ctx.NewsPage && !disabled {
		b.WriteString(` active`)
	}
	if disabled {
		b.WriteString(` disabled`)
	}
	b.WriteString(`" href="`)
	if disabled {
		b.WriteString(`#`)
	} else {
		b.WriteString(aStockPageHref(ctx.Date, ctx.Period, page))
	}
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</a>`)
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

func renderAStockRecommendationSection(b *strings.Builder, ctx aStockContext) {
	b.WriteString(`<section><h2>推荐股票</h2>`)
	recommendations := ctx.Recommendations
	if len(recommendations) == 0 {
		reason := strings.TrimSpace(ctx.EmptyReason)
		if reason == "" {
			reason = "暂无数据：当前新闻窗口未生成热点映射股票，或候选股票被行情、当日开盘价、30/60天跌幅过滤。"
		}
		b.WriteString(`<div class="astock-empty">`)
		b.WriteString(html.EscapeString(reason))
		b.WriteString(`</div><table><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>昨日收盘价</th><th>昨日涨跌幅</th><th>30天涨跌幅</th><th>60天涨跌幅</th><th>现价</th><th>今日跌幅</th><th>推荐理由</th></tr><tr><td colspan="11">暂无推荐股票</td></tr></table></section>`)
		return
	}
	b.WriteString(`<table><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>昨日收盘价</th><th>昨日涨跌幅</th><th>30天涨跌幅</th><th>60天涨跌幅</th><th>现价</th><th>今日跌幅</th><th>推荐理由</th></tr>`)
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
		b.WriteString(html.EscapeString(rec.PrevClose))
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.PrevPctClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.PrevPct))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.Change30Class))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.Change30))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.Change60Class))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.Change60))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.CurrentPrice))
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.TodayPctClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.TodayPct))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Reason))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
}

func renderAStockBacktestSection(b *strings.Builder, strategyDate string, period string, recommendations []aStockRecommendation, rows []aStockBacktestRow) {
	b.WriteString(`<section><h2>消息回测</h2><p class="astock-muted">买入价采用当日开盘价；T+1 到 T+5 按后续交易日收盘价计算收益，并展示五日内最高收益。</p>`)
	renderAStockRecommendationHistoryTabs(b, strategyDate, period)
	b.WriteString(`<div class="astock-scroll"><table class="astock-table"><tr><th>股票</th><th>当日开盘价</th><th>T+1 收盘价</th><th>T+1 收益</th><th>T+2 收益</th><th>T+3 收益</th><th>T+4 收益</th><th>T+5 收益</th><th>五日内最高收益</th><th>命中状态</th></tr>`)
	if len(rows) == 0 {
		b.WriteString(`<tr><td colspan="10">暂无回测结果，等待行情同步。</td></tr>`)
		b.WriteString(`</table></div></section>`)
		return
	}
	for _, row := range rows {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(row.Stock))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(row.EntryOpen))
		b.WriteString(`</td>`)
		for i := 0; i < 5; i++ {
			cell := aStockBacktestCell{Close: "--", Return: "--", ReturnClass: "astock-flat"}
			if i < len(row.Days) {
				cell = row.Days[i]
			}
			if i == 0 {
				b.WriteString(`<td>`)
				b.WriteString(html.EscapeString(cell.Close))
				b.WriteString(`</td>`)
			}
			b.WriteString(`<td><span class="`)
			b.WriteString(html.EscapeString(cell.ReturnClass))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(cell.Return))
			b.WriteString(`</span></td>`)
		}
		b.WriteString(`<td><span class="`)
		b.WriteString(html.EscapeString(row.BestReturnClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(row.BestReturn))
		b.WriteString(`</span></td><td>`)
		b.WriteString(html.EscapeString(row.Status))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></section>`)
}

func renderAStockRecommendationHistoryTabs(b *strings.Builder, strategyDate string, period string) {
	b.WriteString(`<h3>推荐历史</h3><div class="astock-date-tabs">`)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	today := aStockTodayDate()
	day, err := time.Parse("2006-01-02", today)
	if err != nil {
		b.WriteString(`<span class="astock-muted">暂无推荐历史日期</span></div>`)
		return
	}
	for offset := 0; offset <= 5; offset++ {
		date := day.AddDate(0, 0, -offset).Format("2006-01-02")
		label := "今日"
		if offset > 0 {
			label = fmt.Sprintf("前%d日", offset)
		}
		for _, option := range aStockPeriods() {
			periodLabel := strings.TrimSuffix(option.Label, "推荐")
			active := strategyDate == date && normalizedPeriod == option.Key
			writeAStockDateTab(b, label+" "+date+" "+periodLabel, date, option.Key, active)
		}
	}
	b.WriteString(`</div>`)
}

func writeAStockDateTab(b *strings.Builder, label string, date string, period string, active bool) {
	b.WriteString(`<a class="astock-tab`)
	if active {
		b.WriteString(` active`)
	}
	b.WriteString(`" data-preserve-scroll="1" href="/a-stock?date=`)
	b.WriteString(url.QueryEscape(date))
	b.WriteString(`&period=`)
	b.WriteString(url.QueryEscape(period))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</a>`)
}

func (s *Server) loadAStockContext(strategyDate string, periodKey string, newsPage int) aStockContext {
	period := normalizeAStockPeriod(periodKey)
	start, end := aStockWindow(strategyDate, period.Key)
	ctx := aStockContext{
		Date:           strategyDate,
		Period:         period.Key,
		PeriodLabel:    period.Label,
		WindowLabel:    period.WindowLabel,
		NewsPage:       newsPage,
		NewsPageSize:   aStockNewsPageSize,
		WindowStart:    start,
		WindowEnd:      end,
		BacktestStatus: "等待行情接口",
	}
	result := model.ItemListResult{}
	query := "/api/v1/articles?page=1&page_size=200&time_field=publish_time&start=" + url.QueryEscape(formatAStockPublishTime(start)) + "&end=" + url.QueryEscape(formatAStockPublishTime(end))
	if err := s.getJSON(s.cfg.ContentURL+query, &result); err != nil {
		ctx.LoadMessage = "A股新闻读取失败：" + err.Error()
		return ctx
	}
	ctx.Articles = filterAStockNews(result.Items)
	ctx.NewsTotal = len(ctx.Articles)
	ctx.PagedArticles, ctx.NewsPage, ctx.NewsTotalPages = paginateAStockNews(ctx.Articles, newsPage, aStockNewsPageSize)
	ctx.Hotspots = buildAStockHotspots(ctx.Articles)
	ctx.Recommendations = buildAStockRecommendations(ctx.Hotspots)
	ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus = s.loadAStockMarketView(strategyDate, ctx.Recommendations)
	ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
	return ctx
}

func formatAStockPublishTime(value time.Time) string {
	return value.In(aStockLocation()).Format("2006-01-02 15:04:05")
}

func aStockRecommendationEmptyReason(ctx aStockContext) string {
	if len(ctx.Recommendations) > 0 {
		return ""
	}
	if ctx.NewsTotal == 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 没有历史新闻，请先抓取或补抓财经信息。", ctx.PeriodLabel, ctx.WindowLabel)
	}
	if len(ctx.Hotspots) == 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 有 %d 条新闻，但未命中 A股热点关键词。", ctx.PeriodLabel, ctx.WindowLabel, ctx.NewsTotal)
	}
	return fmt.Sprintf("暂无推荐股票：%s %s 已命中 %d 个热点，但候选股票可能被行情、当日开盘价、30/60天跌幅过滤。", ctx.PeriodLabel, ctx.WindowLabel, len(ctx.Hotspots))
}

func paginateAStockNews(items []model.Item, page int, pageSize int) ([]model.Item, int, int) {
	if pageSize <= 0 {
		pageSize = aStockNewsPageSize
	}
	if page < 1 {
		page = 1
	}
	total := len(items)
	if total == 0 {
		return nil, 1, 0
	}
	totalPages := (total + pageSize - 1) / pageSize
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end], page, totalPages
}

func (s *Server) loadAStockMarketView(strategyDate string, recommendations []aStockRecommendation) ([]aStockRecommendation, []aStockBacktestRow, string) {
	recommendations = initializeAStockRecommendationMarket(recommendations)
	if len(recommendations) == 0 {
		return recommendations, nil, "无推荐股票"
	}
	endpoint := aStockMarketEndpoint()
	codes := make([]string, 0, len(recommendations))
	for _, rec := range recommendations {
		codes = append(codes, rec.Code)
	}
	bars, err := s.loadAStockMarketBars(strategyDate, codes, endpoint)
	if err != nil {
		return recommendations, buildAStockBacktestRows(strategyDate, recommendations, nil), "行情读取失败"
	}
	return applyAStockMarketBars(strategyDate, recommendations, bars)
}

func (s *Server) loadAStockMarketBars(strategyDate string, codes []string, endpoint string) ([]aStockMarketBar, error) {
	if endpoint != "" {
		resp, err := s.client.R().
			SetQueryParam("date", strategyDate).
			SetQueryParam("codes", strings.Join(codes, ",")).
			Get(endpoint)
		if err == nil && resp.IsSuccess() {
			bars, decodeErr := decodeAStockMarketBars(resp.Body())
			if decodeErr == nil && len(bars) > 0 {
				return bars, nil
			}
		}
		if bars, fallbackErr := s.loadDefaultAStockBars(strategyDate, codes); fallbackErr == nil && len(bars) > 0 {
			return bars, nil
		}
		if err != nil {
			return nil, err
		}
		if !resp.IsSuccess() {
			return nil, fmt.Errorf("market endpoint status %d", resp.StatusCode())
		}
		return nil, fmt.Errorf("market endpoint returned no bars")
	}
	return s.loadDefaultAStockBars(strategyDate, codes)
}

func (s *Server) loadDefaultAStockBars(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	if bars, err := s.loadEastmoneyAStockBars(strategyDate, codes); err == nil && len(bars) > 0 {
		return bars, nil
	}
	if bars, err := s.loadYahooAStockBars(strategyDate, codes); err == nil && len(bars) > 0 {
		return bars, nil
	}
	return nil, fmt.Errorf("no market bars")
}

func (s *Server) loadEastmoneyAStockBars(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	endDate := aStockMarketEndDate(strategyDate)
	var wg sync.WaitGroup
	barCh := make(chan []aStockMarketBar, len(codes))
	for _, code := range codes {
		code := code
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetQueryParam("secid", eastmoneyAStockSecID(code)).
				SetQueryParam("klt", "101").
				SetQueryParam("fqt", "1").
				SetQueryParam("end", endDate).
				SetQueryParam("lmt", "100").
				SetQueryParam("fields1", "f1,f2,f3,f4,f5,f6").
				SetQueryParam("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61").
				Get(aStockEastmoneyKlineURL)
			if err != nil || !resp.IsSuccess() {
				return
			}
			bars, err := decodeAStockMarketBars(resp.Body())
			if err != nil {
				return
			}
			for i := range bars {
				if bars[i].Code == "" {
					bars[i].Code = code
				}
			}
			barCh <- bars
		}()
	}
	wg.Wait()
	close(barCh)
	bars := make([]aStockMarketBar, 0)
	for chunk := range barCh {
		bars = append(bars, chunk...)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no eastmoney market bars")
	}
	return bars, nil
}

func (s *Server) loadYahooAStockBars(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	day, err := time.ParseInLocation("2006-01-02", strategyDate, aStockLocation())
	if err != nil {
		return nil, err
	}
	period1 := day.AddDate(0, 0, -90).Unix()
	period2 := day.AddDate(0, 0, 15).Unix()
	baseURL := strings.TrimRight(aStockYahooChartURL, "/")
	var wg sync.WaitGroup
	barCh := make(chan []aStockMarketBar, len(codes))
	for _, code := range codes {
		code := normalizeAStockCode(code)
		if code == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetHeader("User-Agent", "Mozilla/5.0").
				SetQueryParam("period1", fmt.Sprint(period1)).
				SetQueryParam("period2", fmt.Sprint(period2)).
				SetQueryParam("interval", "1d").
				SetQueryParam("events", "history").
				SetQueryParam("includeAdjustedClose", "true").
				Get(baseURL + "/" + url.PathEscape(yahooAStockSymbol(code)))
			if err != nil || !resp.IsSuccess() {
				return
			}
			bars, err := decodeYahooAStockBars(resp.Body(), code)
			if err != nil || len(bars) == 0 {
				return
			}
			barCh <- bars
		}()
	}
	wg.Wait()
	close(barCh)
	bars := make([]aStockMarketBar, 0)
	for chunk := range barCh {
		bars = append(bars, chunk...)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no yahoo market bars")
	}
	return bars, nil
}

func initializeAStockRecommendationMarket(recommendations []aStockRecommendation) []aStockRecommendation {
	for i := range recommendations {
		recommendations[i].PrevClose = "--"
		recommendations[i].PrevPct = "--"
		recommendations[i].PrevPctClass = "astock-flat"
		recommendations[i].Change30 = "--"
		recommendations[i].Change30Class = "astock-flat"
		recommendations[i].Change60 = "--"
		recommendations[i].Change60Class = "astock-flat"
		recommendations[i].CurrentPrice = "--"
		recommendations[i].TodayPct = "--"
		recommendations[i].TodayPctClass = "astock-flat"
	}
	return recommendations
}

func applyAStockMarketBars(strategyDate string, recommendations []aStockRecommendation, bars []aStockMarketBar) ([]aStockRecommendation, []aStockBacktestRow, string) {
	byCode := groupAStockMarketBars(bars)
	withPrev := 0
	sectorPenalties := make(map[string]int)
	filteredCount := 0
	noEntryPriceCount := 0
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	for i := range recommendations {
		blockedByDrawdown := false
		entry, ok := aStockEntryBar(byCode[recommendations[i].Code], strategyDate)
		if !ok {
			noEntryPriceCount++
			continue
		}
		if entry.Close > 0 {
			recommendations[i].CurrentPrice = formatAStockPrice(entry.Close)
			recommendations[i].TodayPct = formatAStockPct(entry.Pct)
			recommendations[i].TodayPctClass = aStockPctClass(entry.Pct)
		}
		if prev, ok := previousAStockBar(byCode[recommendations[i].Code], strategyDate); ok {
			recommendations[i].PrevClose = formatAStockPrice(prev.Close)
			recommendations[i].PrevPct = formatAStockPct(prev.Pct)
			recommendations[i].PrevPctClass = aStockPctClass(prev.Pct)
			if change, ok := aStockLookbackChange(byCode[recommendations[i].Code], strategyDate, 30, prev.Close); ok {
				recommendations[i].Change30 = formatAStockPct(change)
				recommendations[i].Change30Class = aStockPctClass(change)
				if change <= aStockDrawdownFilterThreshold {
					blockedByDrawdown = true
				}
			}
			if change, ok := aStockLookbackChange(byCode[recommendations[i].Code], strategyDate, 60, prev.Close); ok {
				recommendations[i].Change60 = formatAStockPct(change)
				recommendations[i].Change60Class = aStockPctClass(change)
				if change <= aStockDrawdownFilterThreshold {
					blockedByDrawdown = true
				}
			}
			withPrev++
		}
		if blockedByDrawdown {
			sectorPenalties[recommendations[i].Hotspot] += aStockSectorDrawdownPenalty
			filteredCount++
			continue
		}
		filtered = append(filtered, recommendations[i])
	}
	recommendations = filtered
	for i := range recommendations {
		penalty := sectorPenalties[recommendations[i].Hotspot]
		baseScore := recommendations[i].HotspotScore
		if baseScore == 0 {
			baseScore = recommendations[i].MarketScore
		}
		recommendations[i].MarketScore = baseScore - penalty
		if penalty > 0 {
			recommendations[i].Reason = fmt.Sprintf("%s，板块回撤减分 %d，调整分 %d", recommendations[i].Reason, penalty, recommendations[i].MarketScore)
		}
	}
	sort.SliceStable(recommendations, func(i, j int) bool {
		if recommendations[i].MarketScore == recommendations[j].MarketScore {
			if recommendations[i].Hotspot == recommendations[j].Hotspot {
				return recommendations[i].Code < recommendations[j].Code
			}
			return recommendations[i].Hotspot < recommendations[j].Hotspot
		}
		return recommendations[i].MarketScore > recommendations[j].MarketScore
	})
	for i := range recommendations {
		recommendations[i].Rank = i + 1
	}
	rows := buildAStockBacktestRows(strategyDate, recommendations, byCode)
	completed := 0
	for _, row := range rows {
		if row.Status == "已回测" || strings.HasPrefix(row.Status, "已回测") {
			completed++
		}
	}
	status := fmt.Sprintf("已回测 %d/%d", completed, len(recommendations))
	if filteredCount > 0 {
		status = fmt.Sprintf("%s，过滤回撤股票 %d", status, filteredCount)
	}
	if noEntryPriceCount > 0 {
		status = fmt.Sprintf("%s，过滤无当日行情股票 %d", status, noEntryPriceCount)
	}
	if len(recommendations) == 0 && filteredCount > 0 {
		status = fmt.Sprintf("回撤过滤后无推荐股票，过滤回撤股票 %d", filteredCount)
	}
	if len(recommendations) == 0 && noEntryPriceCount > 0 {
		status = fmt.Sprintf("无当日行情可推荐，过滤无当日行情股票 %d", noEntryPriceCount)
		if filteredCount > 0 {
			status = fmt.Sprintf("%s，过滤回撤股票 %d", status, filteredCount)
		}
	}
	if withPrev == 0 && completed == 0 && filteredCount == 0 && noEntryPriceCount == 0 {
		status = "无匹配行情"
	}
	return recommendations, rows, status
}

func groupAStockMarketBars(bars []aStockMarketBar) map[string][]aStockMarketBar {
	grouped := make(map[string][]aStockMarketBar)
	for _, bar := range bars {
		bar.Code = normalizeAStockCode(bar.Code)
		bar.Date = normalizeAStockMarketDate(bar.Date)
		if bar.Code == "" || bar.Date == "" {
			continue
		}
		grouped[bar.Code] = append(grouped[bar.Code], bar)
	}
	for code := range grouped {
		sort.SliceStable(grouped[code], func(i, j int) bool {
			return grouped[code][i].Date < grouped[code][j].Date
		})
	}
	return grouped
}

func previousAStockBar(bars []aStockMarketBar, strategyDate string) (aStockMarketBar, bool) {
	var found aStockMarketBar
	ok := false
	for _, bar := range bars {
		if bar.Date >= strategyDate {
			break
		}
		found = bar
		ok = true
	}
	return found, ok
}

func aStockLookbackChange(bars []aStockMarketBar, strategyDate string, days int, prevClose float64) (float64, bool) {
	if prevClose <= 0 {
		return 0, false
	}
	targetDate, err := aStockDateOffset(strategyDate, -days)
	if err != nil {
		return 0, false
	}
	bar, ok := latestAStockBarOnOrBefore(bars, targetDate)
	if !ok || bar.Close <= 0 {
		return 0, false
	}
	return (prevClose/bar.Close - 1) * 100, true
}

func latestAStockBarOnOrBefore(bars []aStockMarketBar, targetDate string) (aStockMarketBar, bool) {
	var found aStockMarketBar
	ok := false
	for _, bar := range bars {
		if bar.Date > targetDate {
			break
		}
		found = bar
		ok = true
	}
	return found, ok
}

func aStockEntryBar(bars []aStockMarketBar, strategyDate string) (aStockMarketBar, bool) {
	for _, bar := range bars {
		if bar.Date == strategyDate && bar.Open > 0 {
			return bar, true
		}
	}
	return aStockMarketBar{}, false
}

func buildAStockBacktestRows(strategyDate string, recommendations []aStockRecommendation, byCode map[string][]aStockMarketBar) []aStockBacktestRow {
	rows := make([]aStockBacktestRow, 0, len(recommendations))
	for _, rec := range recommendations {
		row := aStockBacktestRow{
			Stock:           rec.Code + " " + rec.Name,
			EntryOpen:       "--",
			Days:            make([]aStockBacktestCell, 5),
			BestReturn:      "--",
			BestReturnClass: "astock-flat",
			Status:          "等待行情接口配置",
		}
		for i := range row.Days {
			row.Days[i] = aStockBacktestCell{Close: "--", Return: "--", ReturnClass: "astock-flat"}
		}
		bars := byCode[rec.Code]
		if len(bars) == 0 {
			if byCode != nil {
				row.Status = "无行情数据"
			}
			rows = append(rows, row)
			continue
		}
		entryIdx := aStockEntryBarIndex(bars, strategyDate)
		if entryIdx < 0 {
			row.Status = "等待当日开盘价"
			rows = append(rows, row)
			continue
		}
		entry := bars[entryIdx]
		row.EntryOpen = formatAStockPrice(entry.Open)
		bestSet := false
		bestReturn := 0.0
		filled := 0
		for day := 1; day <= 5; day++ {
			barIdx := entryIdx + day
			if barIdx >= len(bars) {
				break
			}
			bar := bars[barIdx]
			ret := (bar.Close/entry.Open - 1) * 100
			row.Days[day-1] = aStockBacktestCell{
				Close:       formatAStockPrice(bar.Close),
				Return:      formatAStockPct(ret),
				ReturnClass: aStockPctClass(ret),
			}
			if !bestSet || ret > bestReturn {
				bestSet = true
				bestReturn = ret
			}
			filled++
		}
		switch {
		case filled == 0:
			row.Status = "等待T+1行情"
		case filled < 5:
			row.Status = fmt.Sprintf("已回测T+%d", filled)
		default:
			row.Status = "已回测"
		}
		if bestSet {
			row.BestReturn = formatAStockPct(bestReturn)
			row.BestReturnClass = aStockPctClass(bestReturn)
		}
		rows = append(rows, row)
	}
	return rows
}

func aStockEntryBarIndex(bars []aStockMarketBar, strategyDate string) int {
	for i, bar := range bars {
		if bar.Date == strategyDate && bar.Open > 0 {
			return i
		}
	}
	return -1
}

func decodeYahooAStockBars(body []byte, code string) ([]aStockMarketBar, error) {
	var payload struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open  []*float64 `json:"open"`
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("empty yahoo chart")
	}
	result := payload.Chart.Result[0]
	quote := result.Indicators.Quote[0]
	bars := make([]aStockMarketBar, 0, len(result.Timestamp))
	location := aStockLocation()
	code = normalizeAStockCode(code)
	for i, ts := range result.Timestamp {
		if i >= len(quote.Open) || i >= len(quote.Close) || quote.Open[i] == nil || quote.Close[i] == nil {
			continue
		}
		open := *quote.Open[i]
		closeValue := *quote.Close[i]
		if open <= 0 || closeValue <= 0 {
			continue
		}
		bars = append(bars, aStockMarketBar{
			Code:  code,
			Date:  time.Unix(ts, 0).In(location).Format("2006-01-02"),
			Open:  open,
			Close: closeValue,
		})
	}
	sort.SliceStable(bars, func(i, j int) bool {
		return bars[i].Date < bars[j].Date
	})
	for i := 1; i < len(bars); i++ {
		if bars[i-1].Close > 0 {
			bars[i].Pct = (bars[i].Close/bars[i-1].Close - 1) * 100
		}
	}
	return bars, nil
}

func decodeAStockMarketBars(body []byte) ([]aStockMarketBar, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return collectAStockMarketBars(payload, nil), nil
}

func collectAStockMarketBars(value any, fields []string) []aStockMarketBar {
	switch typed := value.(type) {
	case map[string]any:
		if data, ok := typed["data"]; ok {
			return collectAStockMarketBars(data, fields)
		}
		if rawFields, ok := typed["fields"].([]any); ok {
			fields = make([]string, 0, len(rawFields))
			for _, field := range rawFields {
				fields = append(fields, strings.ToLower(strings.TrimSpace(fmt.Sprint(field))))
			}
		}
		if items, ok := typed["items"]; ok {
			return withAStockParentCode(collectAStockMarketBars(items, fields), typed)
		}
		if klines, ok := typed["klines"]; ok {
			return withAStockParentCode(collectAStockMarketBars(klines, fields), typed)
		}
		if bar, ok := mapToAStockMarketBar(typed); ok {
			return []aStockMarketBar{bar}
		}
	case []any:
		bars := make([]aStockMarketBar, 0, len(typed))
		for _, item := range typed {
			switch row := item.(type) {
			case map[string]any:
				if bar, ok := mapToAStockMarketBar(row); ok {
					bars = append(bars, bar)
				}
			case []any:
				if bar, ok := arrayToAStockMarketBar(row, fields); ok {
					bars = append(bars, bar)
				}
			case string:
				if bar, ok := eastmoneyKlineToAStockMarketBar(row); ok {
					bars = append(bars, bar)
				}
			}
		}
		return bars
	}
	return nil
}

func withAStockParentCode(bars []aStockMarketBar, parent map[string]any) []aStockMarketBar {
	parentCode := normalizeAStockCode(firstString(parent, "code", "stock_code", "symbol", "ts_code"))
	if parentCode == "" {
		return bars
	}
	for i := range bars {
		if bars[i].Code == "" {
			bars[i].Code = parentCode
		}
	}
	return bars
}

func mapToAStockMarketBar(row map[string]any) (aStockMarketBar, bool) {
	code := normalizeAStockCode(firstString(row, "code", "stock_code", "symbol", "ts_code"))
	date := normalizeAStockMarketDate(firstString(row, "date", "trade_date", "day"))
	open, _ := firstFloat(row, "open", "open_price")
	closeValue, ok := firstFloat(row, "close", "close_price", "pre_close")
	pct, _ := firstFloat(row, "pct", "pct_chg", "change_pct")
	if code == "" || date == "" || !ok {
		return aStockMarketBar{}, false
	}
	return aStockMarketBar{Code: code, Date: date, Open: open, Close: closeValue, Pct: pct}, true
}

func arrayToAStockMarketBar(row []any, fields []string) (aStockMarketBar, bool) {
	if len(fields) == 0 {
		return aStockMarketBar{}, false
	}
	values := make(map[string]any, len(fields))
	for i, field := range fields {
		if i < len(row) {
			values[field] = row[i]
		}
	}
	return mapToAStockMarketBar(values)
}

func eastmoneyKlineToAStockMarketBar(raw string) (aStockMarketBar, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) < 9 {
		return aStockMarketBar{}, false
	}
	open := parseAStockFloat(parts[1])
	closeValue := parseAStockFloat(parts[2])
	pct := parseAStockFloat(parts[8])
	return aStockMarketBar{Date: normalizeAStockMarketDate(parts[0]), Open: open, Close: closeValue, Pct: pct}, true
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func firstFloat(row map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if number, ok := aStockFloat(value); ok {
				return number, true
			}
		}
	}
	return 0, false
}

func aStockFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		text := strings.TrimSpace(strings.TrimSuffix(typed, "%"))
		if text == "" || text == "--" {
			return 0, false
		}
		var number float64
		if _, err := fmt.Sscanf(text, "%f", &number); err == nil {
			return number, true
		}
	}
	return 0, false
}

func parseAStockFloat(raw string) float64 {
	number, _ := aStockFloat(strings.TrimSpace(raw))
	return number
}

func normalizeAStockCode(raw string) string {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "SH"), ".SH")
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "SZ"), ".SZ")
	raw = strings.TrimPrefix(raw, "1.")
	raw = strings.TrimPrefix(raw, "0.")
	if len(raw) >= 6 {
		return raw[:6]
	}
	return raw
}

func normalizeAStockMarketDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return raw[:10]
	}
	if len(raw) == 8 {
		return raw[:4] + "-" + raw[4:6] + "-" + raw[6:8]
	}
	return raw
}

func formatAStockPrice(value float64) string {
	if value <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.2f", value)
}

func formatAStockPct(value float64) string {
	return fmt.Sprintf("%+.2f%%", value)
}

func aStockPctClass(value float64) string {
	switch {
	case value > 0:
		return "astock-up"
	case value < 0:
		return "astock-down"
	default:
		return "astock-flat"
	}
}

func aStockMarketEndpoint() string {
	return strings.TrimSpace(os.Getenv("YUQING_ASTOCK_MARKET_URL"))
}

func aStockMarketConfigHint() string {
	if aStockMarketEndpoint() == "" {
		return "默认东方财富日 K，失败后使用 Yahoo Finance 日线；也可用 YUQING_ASTOCK_MARKET_URL 覆盖"
	}
	return "YUQING_ASTOCK_MARKET_URL，失败后回退东方财富日 K / Yahoo Finance 日线"
}

func aStockMarketEndDate(strategyDate string) string {
	day, err := time.Parse("2006-01-02", strategyDate)
	if err != nil {
		return time.Now().Format("20060102")
	}
	return day.AddDate(0, 0, 14).Format("20060102")
}

func aStockDateOffset(strategyDate string, days int) (string, error) {
	day, err := time.Parse("2006-01-02", strategyDate)
	if err != nil {
		return "", err
	}
	return day.AddDate(0, 0, days).Format("2006-01-02"), nil
}

func eastmoneyAStockSecID(code string) string {
	code = normalizeAStockCode(code)
	market := "0"
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") || strings.HasPrefix(code, "5") {
		market = "1"
	}
	return market + "." + code
}

func yahooAStockSymbol(code string) string {
	code = normalizeAStockCode(code)
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") || strings.HasPrefix(code, "5") {
		return code + ".SS"
	}
	return code + ".SZ"
}

func aStockLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return location
}

func (s *Server) triggerAStockCrawl() string {
	sources := []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun"}
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
	return "A股新闻抓取已触发：金十快讯、金十资讯、金十全站信息、东方财富网"
}

func (s *Server) triggerAStockWindowCrawl(strategyDate string, periodKey string) string {
	period := normalizeAStockPeriod(periodKey)
	start, end := aStockWindow(strategyDate, period.Key)
	sources := []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun"}
	ok := 0
	failures := make([]string, 0)
	for _, sourceType := range sources {
		resp, err := s.client.R().
			SetQueryParam("source_type", sourceType).
			SetQueryParam("start", formatAStockPublishTime(start)).
			SetQueryParam("end", formatAStockPublishTime(end)).
			SetQueryParam("time_field", "publish_time").
			Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		if err != nil || !resp.IsSuccess() {
			failures = append(failures, sourceType)
			continue
		}
		ok++
	}

	countText := ""
	if count, err := s.countAStockWindowNews(start, end); err == nil {
		countText = fmt.Sprintf("当前窗口已有 %d 条财经新闻。", count)
	}
	windowText := fmt.Sprintf("%s %s %s", normalizeAStockStrategyDate(strategyDate), strings.TrimSuffix(period.Label, "推荐"), period.WindowLabel)
	if len(failures) > 0 {
		return fmt.Sprintf("已补抓 %s：成功 %d 个来源，失败 %s。%s", windowText, ok, strings.Join(failures, "、"), countText)
	}
	return fmt.Sprintf("已补抓 %s：金十快讯、金十资讯、金十全站信息、东方财富网。%s", windowText, countText)
}

func (s *Server) countAStockWindowNews(start time.Time, end time.Time) (int, error) {
	result := model.ItemListResult{}
	query := "/api/v1/articles?page=1&page_size=200&time_field=publish_time&start=" + url.QueryEscape(formatAStockPublishTime(start)) + "&end=" + url.QueryEscape(formatAStockPublishTime(end))
	if err := s.getJSON(s.cfg.ContentURL+query, &result); err != nil {
		return 0, err
	}
	return len(filterAStockNews(result.Items)), nil
}

func aStockWindow(strategyDate string, periodKey string) (time.Time, time.Time) {
	location := aStockLocation()
	day, err := time.ParseInLocation("2006-01-02", strategyDate, location)
	if err != nil {
		day = time.Now().In(location)
	}
	period := normalizeAStockPeriod(periodKey)
	startHour, startMinute, endHour, endMinute := 9, 0, 9, 25
	if period.Key == "afternoon" {
		startHour, startMinute, endHour, endMinute = 9, 26, 12, 50
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), startHour, startMinute, 0, 0, location)
	end := time.Date(day.Year(), day.Month(), day.Day(), endHour, endMinute, 59, 0, location)
	return start, end
}

func filterAStockNews(items []model.Item) []model.Item {
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		sourceType := strings.TrimSpace(item.SourceType)
		if sourceType != "flash" && sourceType != "headline" && sourceType != "jin10_full" && sourceType != "eastmoney_kuaixun" {
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
	if len(hotspots) > 3 {
		hotspots = hotspots[:3]
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
				Rank:         len(recommendations) + 1,
				Hotspot:      hotspot.Name,
				Code:         stock.Code,
				Name:         stock.Name,
				HotspotScore: hotspot.Score,
				MarketScore:  hotspot.Score,
				Reason:       fmt.Sprintf("命中 %s，证据新闻 %d 条，热度分 %d", strings.Join(hotspot.Keywords, "、"), hotspot.Evidence, hotspot.Score),
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

func aStockTodayDate() string {
	return aStockNow().In(aStockLocation()).Format("2006-01-02")
}

func normalizeAStockStrategyDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			return parsed.Format("2006-01-02")
		}
		return raw
	}
	return aStockTodayDate()
}

func normalizeAStockNewsPage(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 1
	}
	var page int
	if _, err := fmt.Sscanf(raw, "%d", &page); err != nil || page < 1 {
		return 1
	}
	return page
}

func aStockPageHref(strategyDate string, period string, newsPage int) string {
	href := "/a-stock?date=" + url.QueryEscape(normalizeAStockStrategyDate(strategyDate)) + "&period=" + url.QueryEscape(normalizeAStockPeriod(period).Key)
	if newsPage > 1 {
		href += "&news_page=" + url.QueryEscape(fmt.Sprintf("%d", newsPage))
	}
	return href
}

func aStockPeriods() []aStockPeriod {
	return []aStockPeriod{
		{Key: "morning", Label: "上午推荐", WindowLabel: "09:00-09:25"},
		{Key: "afternoon", Label: "下午推荐", WindowLabel: "09:26-12:50"},
	}
}

func normalizeAStockPeriod(raw string) aStockPeriod {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "after" || raw == "pm" {
		raw = "afternoon"
	}
	for _, period := range aStockPeriods() {
		if period.Key == raw {
			return period
		}
	}
	return aStockPeriods()[0]
}

func renderAStockPeriodTabs(b *strings.Builder, strategyDate string, selected string) {
	b.WriteString(`<div class="astock-tabs">`)
	for _, period := range aStockPeriods() {
		b.WriteString(`<a class="astock-tab`)
		if period.Key == selected {
			b.WriteString(` active`)
		}
		b.WriteString(`" href="/a-stock?date=`)
		b.WriteString(url.QueryEscape(strategyDate))
		b.WriteString(`&period=`)
		b.WriteString(url.QueryEscape(period.Key))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(period.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}
