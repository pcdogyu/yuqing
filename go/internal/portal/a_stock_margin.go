package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const aStockMarginPageSize = 200
const aStockMarginDefaultTrendMetric = "margin_trading_balance"

type aStockMarginPageContext struct {
	List        model.AStockMarginListResult
	Trend       model.AStockMarginTrendResult
	TrendDays   int
	TrendMetric string
	TrendCode   string
	TrendError  string
}

type aStockMarginMetricDef struct {
	Key          string
	Label        string
	Unit         string
	SummaryValue func(model.AStockMarginSummary) *float64
	DetailValue  func(model.AStockMarginDetail) *float64
}

var aStockMarginMetricDefs = []aStockMarginMetricDef{
	{Key: "margin_buy_amount", Label: "融资买入额", Unit: "元", SummaryValue: func(item model.AStockMarginSummary) *float64 { return item.MarginBuyAmount }, DetailValue: func(item model.AStockMarginDetail) *float64 { return item.MarginBuyAmount }},
	{Key: "margin_balance", Label: "融资余额", Unit: "元", SummaryValue: func(item model.AStockMarginSummary) *float64 { return item.MarginBalance }, DetailValue: func(item model.AStockMarginDetail) *float64 { return item.MarginBalance }},
	{Key: "short_sell_volume", Label: "融券卖出量", Unit: "股/份", SummaryValue: func(item model.AStockMarginSummary) *float64 { return item.ShortSellVolume }, DetailValue: func(item model.AStockMarginDetail) *float64 { return item.ShortSellVolume }},
	{Key: "short_balance_volume", Label: "融券余量", Unit: "股/份", SummaryValue: func(item model.AStockMarginSummary) *float64 { return item.ShortBalanceVolume }, DetailValue: func(item model.AStockMarginDetail) *float64 { return item.ShortBalanceVolume }},
	{Key: "short_balance_amount", Label: "融券余额", Unit: "元", SummaryValue: func(item model.AStockMarginSummary) *float64 { return item.ShortBalanceAmount }, DetailValue: func(item model.AStockMarginDetail) *float64 { return item.ShortBalanceAmount }},
	{Key: "margin_trading_balance", Label: "融资融券余额", Unit: "元", SummaryValue: func(item model.AStockMarginSummary) *float64 { return item.MarginTradingBalance }, DetailValue: func(item model.AStockMarginDetail) *float64 { return item.MarginTradingBalance }},
}

func (s *Server) handleAStockMarginPage(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	if r.Method == http.MethodPost {
		s.handleAStockMarginAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := aStockMarginFilterFromRequest(r)
	ctx, err := s.loadAStockMarginPageContext(filter, r)
	var b strings.Builder
	b.WriteString(`<style>
body[data-page='a-stock-margin'] main,body[data-page='a-stock-margin'] .site-footer{max-width:none;width:100%;box-sizing:border-box}
body[data-page='a-stock-margin'] main{padding-left:18px;padding-right:18px;font-size:14px;line-height:1.45}
body[data-page='a-stock-margin'] section{width:100%;box-sizing:border-box}
.margin-muted{color:#6a6257}.margin-head{display:flex;align-items:flex-start;justify-content:space-between;gap:14px;flex-wrap:wrap}.margin-head h2{margin-bottom:6px}
.margin-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:center;justify-content:flex-end}.margin-actions form{margin:0}.margin-actions button{margin:0;white-space:nowrap}.margin-backfill-range{display:grid;grid-template-columns:140px 140px 110px;gap:8px;align-items:end}
.margin-toolbar{display:grid;grid-template-columns:150px 140px minmax(220px,.35fr) 110px;gap:10px;align-items:end;margin-top:12px}.margin-toolbar button{margin:0}
.margin-message{padding:12px;border-radius:8px;background:#e7f4ea;color:#214e34;margin:12px 0}.margin-error{padding:14px;border:1px dashed #d0c8b8;border-radius:8px;background:#fff;color:#8a4b00}
.margin-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;margin:14px 0}.margin-card{padding:14px;border:1px solid #ece7dc;border-radius:8px;background:#fff}.margin-card h3{margin:0 0 8px;font-size:15px}.margin-card dl{display:grid;grid-template-columns:1fr auto;gap:6px 10px;margin:0}.margin-card dt{color:#6a6257}.margin-card dd{margin:0;font-weight:700;white-space:nowrap}
.margin-trend-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap}.margin-trend-toolbar{display:grid;grid-template-columns:170px 150px 110px;gap:8px;align-items:end}.margin-trend-toolbar button{margin:0}.margin-trend-grid{display:grid;grid-template-columns:1fr;gap:12px;margin-top:12px}.margin-chart{border:1px solid #ece7dc;border-radius:8px;background:#fff;overflow:auto}.margin-chart h4{margin:12px 14px 0;font-size:15px}.margin-chart svg{display:block;width:100%;min-width:760px;height:auto}.margin-chart-axis{fill:#6a6257;font-size:12px}.margin-chart-grid{stroke:#ece7dc;stroke-width:1}.margin-chart-line{fill:none;stroke-width:2.5}.margin-chart-point{stroke:#fff;stroke-width:1}.margin-chart-legend{font-size:12px;font-weight:700}.margin-trend-values{display:flex;gap:10px;flex-wrap:wrap;padding:0 14px 12px;color:#6a6257;font-size:12px}.margin-trend-values b{color:#214e34}.margin-trend-links{display:flex;gap:8px;flex-wrap:wrap}.margin-trend-link{display:inline-flex;align-items:center;justify-content:center;border:1px solid #d0c8b8;border-radius:8px;padding:5px 8px;text-decoration:none;color:#214e34;background:#fff}.margin-trend-link.active{background:#214e34;color:#fff;border-color:#214e34}
.margin-scroll{overflow:auto}.margin-table{min-width:1460px;width:100%;table-layout:fixed}.margin-table th,.margin-table td{vertical-align:middle;white-space:nowrap}.margin-table th{text-align:left}.margin-table th:nth-child(1),.margin-table td:nth-child(1){width:58px;text-align:center}.margin-table th:nth-child(2),.margin-table td:nth-child(2){width:70px;text-align:center}.margin-table th:nth-child(3),.margin-table td:nth-child(3){width:90px}.margin-table th:nth-child(4),.margin-table td:nth-child(4){width:140px}.margin-num{text-align:right}.margin-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:8px;background:#fff;color:#6a6257}
@media (max-width:760px){.margin-toolbar,.margin-trend-toolbar,.margin-backfill-range{grid-template-columns:1fr}.margin-head{display:block}.margin-actions{justify-content:flex-start}}
</style>`)
	b.WriteString(`<section><div class="margin-head"><div><h2>融资融券</h2><p class="margin-muted">展示沪市、深市融资融券汇总和标的明细，金额按元归一化展示。</p></div>`)
	renderAStockMarginActions(&b, ctx)
	b.WriteString(`</div></section>`)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<div class="margin-message">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	renderAStockMarginFilters(&b, ctx)
	if err != nil {
		b.WriteString(`<section><div class="margin-error">融资融券数据读取失败：`)
		b.WriteString(html.EscapeString(err.Error()))
		b.WriteString(`</div></section>`)
		_ = s.writeSimplePage(w, "a-stock-margin", "融资融券", b.String())
		return
	}
	renderAStockMarginSummary(&b, ctx.List)
	renderAStockMarginTrend(&b, ctx)
	renderAStockMarginTable(&b, ctx)
	_ = s.writeSimplePage(w, "a-stock-margin", "融资融券", b.String())
}

func (s *Server) handleAStockMarginAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := model.AStockMarginFilter{
		Date:    strings.TrimSpace(r.FormValue("date")),
		Market:  strings.TrimSpace(r.FormValue("market")),
		Keyword: strings.TrimSpace(r.FormValue("keyword")),
	}
	trendDays := normalizeAStockMarginTrendDaysText(r.FormValue("trend_days"))
	trendMetric := normalizeAStockMarginTrendMetric(r.FormValue("trend_metric"))
	trendCode := normalizeAStockMarginCode(r.FormValue("trend_code"))
	message := "未知操作"
	switch strings.TrimSpace(r.FormValue("action")) {
	case "refresh_margin_trading":
		message = s.triggerAStockMarginRefresh(filter.Date)
	case "backfill_margin_trading":
		message = s.triggerAStockMarginBackfill(normalizeAStockMarginBackfillDays(r.FormValue("days")), "", "")
	case "backfill_range_margin_trading":
		message = s.triggerAStockMarginBackfill(0, strings.TrimSpace(r.FormValue("start")), strings.TrimSpace(r.FormValue("end")))
	}
	query := aStockMarginQuery(filter)
	setAStockMarginTrendQuery(query, trendDays, trendMetric, trendCode)
	query.Set("msg", message)
	http.Redirect(w, r, "/a-stock/margin?"+query.Encode(), http.StatusSeeOther)
}

func aStockMarginFilterFromRequest(r *http.Request) model.AStockMarginFilter {
	return model.AStockMarginFilter{
		Date:     strings.TrimSpace(r.URL.Query().Get("date")),
		Market:   strings.TrimSpace(r.URL.Query().Get("market")),
		Keyword:  strings.TrimSpace(nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("q"))),
		Page:     normalizeAStockNewsPage(r.URL.Query().Get("page")),
		PageSize: aStockMarginPageSize,
	}
}

func (s *Server) loadAStockMarginContext(filter model.AStockMarginFilter) (model.AStockMarginListResult, error) {
	query := aStockMarginQuery(filter)
	query.Set("page", fmt.Sprintf("%d", max(filter.Page, 1)))
	query.Set("page_size", fmt.Sprintf("%d", aStockMarginPageSize))
	var result model.AStockMarginListResult
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/margin-trading?"+query.Encode(), &result)
	if result.Market == "" {
		result.Market = normalizeAStockMarginMarket(filter.Market)
	}
	if result.Page <= 0 {
		result.Page = max(filter.Page, 1)
	}
	if result.PageSize <= 0 {
		result.PageSize = aStockMarginPageSize
	}
	return result, err
}

func (s *Server) loadAStockMarginPageContext(filter model.AStockMarginFilter, r *http.Request) (aStockMarginPageContext, error) {
	trendDays := normalizeAStockMarginTrendDaysText(r.URL.Query().Get("trend_days"))
	trendMetric := normalizeAStockMarginTrendMetric(nonEmpty(r.URL.Query().Get("trend_metric"), r.URL.Query().Get("metric")))
	trendCode := normalizeAStockMarginCode(r.URL.Query().Get("trend_code"))
	list, err := s.loadAStockMarginContext(filter)
	ctx := aStockMarginPageContext{List: list, TrendDays: trendDays, TrendMetric: trendMetric, TrendCode: trendCode}
	if err != nil {
		return ctx, err
	}
	if ctx.TrendCode == "" && len(list.Details) == 1 {
		ctx.TrendCode = normalizeAStockMarginCode(list.Details[0].Code)
	}
	trend, trendErr := s.loadAStockMarginTrendContext(list, trendDays, ctx.TrendCode)
	ctx.Trend = trend
	if trendErr != nil {
		ctx.TrendError = trendErr.Error()
	}
	return ctx, nil
}

func (s *Server) loadAStockMarginTrendContext(list model.AStockMarginListResult, days int, code string) (model.AStockMarginTrendResult, error) {
	query := url.Values{}
	query.Set("days", fmt.Sprintf("%d", normalizeAStockMarginTrendDays(days)))
	if strings.TrimSpace(list.Date) != "" {
		query.Set("end_date", strings.TrimSpace(list.Date))
	}
	if market := normalizeAStockMarginMarket(list.Market); market != "" {
		query.Set("market", market)
	}
	if code = normalizeAStockMarginCode(code); code != "" {
		query.Set("code", code)
	}
	var result model.AStockMarginTrendResult
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/margin-trading/trend?"+query.Encode(), &result)
	return result, err
}

func renderAStockMarginActions(b *strings.Builder, ctx aStockMarginPageContext) {
	b.WriteString(`<div class="margin-actions"><form method="post" action="/a-stock/margin">`)
	b.WriteString(`<input type="hidden" name="action" value="refresh_margin_trading">`)
	renderAStockMarginHiddenState(b, ctx)
	b.WriteString(`<button type="submit">刷新融资融券</button></form>`)
	for _, days := range []int{5, 10, 30} {
		b.WriteString(`<form method="post" action="/a-stock/margin"><input type="hidden" name="action" value="backfill_margin_trading">`)
		renderAStockMarginHiddenState(b, ctx)
		b.WriteString(`<input type="hidden" name="days" value="`)
		b.WriteString(fmt.Sprintf("%d", days))
		b.WriteString(`"><button type="submit">回填近`)
		b.WriteString(fmt.Sprintf("%d", days))
		b.WriteString(`日</button></form>`)
	}
	b.WriteString(`<form class="margin-backfill-range" method="post" action="/a-stock/margin"><input type="hidden" name="action" value="backfill_range_margin_trading">`)
	renderAStockMarginHiddenState(b, ctx)
	b.WriteString(`<label>开始<input type="date" name="start"></label><label>结束<input type="date" name="end"></label><button type="submit">范围回填</button></form></div>`)
}

func renderAStockMarginHiddenState(b *strings.Builder, ctx aStockMarginPageContext) {
	b.WriteString(`<input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.List.Date))
	b.WriteString(`"><input type="hidden" name="market" value="`)
	b.WriteString(html.EscapeString(ctx.List.Market))
	b.WriteString(`"><input type="hidden" name="keyword" value="`)
	b.WriteString(html.EscapeString(ctx.List.Keyword))
	b.WriteString(`"><input type="hidden" name="trend_days" value="`)
	b.WriteString(fmt.Sprintf("%d", normalizeAStockMarginTrendDays(ctx.TrendDays)))
	b.WriteString(`"><input type="hidden" name="trend_metric" value="`)
	b.WriteString(html.EscapeString(ctx.TrendMetric))
	b.WriteString(`"><input type="hidden" name="trend_code" value="`)
	b.WriteString(html.EscapeString(ctx.TrendCode))
	b.WriteString(`">`)
}

func renderAStockMarginFilters(b *strings.Builder, page aStockMarginPageContext) {
	ctx := page.List
	b.WriteString(`<section><form class="margin-toolbar" method="get" action="/a-stock/margin">`)
	b.WriteString(`<label>日期<input type="date" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"></label>`)
	b.WriteString(`<label>交易所<select name="market">`)
	for _, option := range []struct {
		Value string
		Label string
	}{{"all", "沪深合计"}, {"sse", "沪市"}, {"szse", "深市"}} {
		b.WriteString(`<option value="`)
		b.WriteString(option.Value)
		b.WriteString(`"`)
		if normalizeAStockMarginMarket(ctx.Market) == option.Value {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(option.Label)
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></label>`)
	b.WriteString(`<label>代码/简称<input type="search" name="keyword" value="`)
	b.WriteString(html.EscapeString(ctx.Keyword))
	b.WriteString(`" placeholder="000001 或 平安银行"></label>`)
	b.WriteString(`<input type="hidden" name="trend_days" value="`)
	b.WriteString(fmt.Sprintf("%d", normalizeAStockMarginTrendDays(page.TrendDays)))
	b.WriteString(`"><input type="hidden" name="trend_metric" value="`)
	b.WriteString(html.EscapeString(page.TrendMetric))
	b.WriteString(`"><input type="hidden" name="trend_code" value="`)
	b.WriteString(html.EscapeString(page.TrendCode))
	b.WriteString(`"><button type="submit">查询</button></form>`)
	if len(ctx.Dates) > 0 {
		b.WriteString(`<p class="margin-muted">已有日期：`)
		limit := min(len(ctx.Dates), 8)
		for i := 0; i < limit; i++ {
			if i > 0 {
				b.WriteString(` · `)
			}
			query := aStockMarginQuery(model.AStockMarginFilter{Date: ctx.Dates[i], Market: ctx.Market, Keyword: ctx.Keyword})
			setAStockMarginTrendQuery(query, page.TrendDays, page.TrendMetric, page.TrendCode)
			b.WriteString(`<a href="/a-stock/margin?`)
			b.WriteString(html.EscapeString(query.Encode()))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(ctx.Dates[i]))
			b.WriteString(`</a>`)
		}
		b.WriteString(`</p>`)
	}
	b.WriteString(`</section>`)
}

func renderAStockMarginSummary(b *strings.Builder, ctx model.AStockMarginListResult) {
	if len(ctx.Summaries) == 0 {
		b.WriteString(`<section><div class="margin-empty">暂无融资融券汇总数据，可点击刷新融资融券。</div></section>`)
		return
	}
	b.WriteString(`<section><div class="margin-grid">`)
	for _, item := range ctx.Summaries {
		b.WriteString(`<div class="margin-card"><h3>`)
		b.WriteString(html.EscapeString(nonEmpty(item.MarketLabel, aStockMarginMarketLabel(item.Market))))
		b.WriteString(`</h3><dl>`)
		renderAStockMarginSummaryField(b, "融资买入额", formatAStockMarginMoney(item.MarginBuyAmount))
		renderAStockMarginSummaryField(b, "融资余额", formatAStockMarginMoney(item.MarginBalance))
		renderAStockMarginSummaryField(b, "融券卖出量", formatAStockMarginQuantity(item.ShortSellVolume))
		renderAStockMarginSummaryField(b, "融券余量", formatAStockMarginQuantity(item.ShortBalanceVolume))
		renderAStockMarginSummaryField(b, "融券余额", formatAStockMarginMoney(item.ShortBalanceAmount))
		renderAStockMarginSummaryField(b, "融资融券余额", formatAStockMarginMoney(item.MarginTradingBalance))
		b.WriteString(`</dl></div>`)
	}
	b.WriteString(`</div></section>`)
}

func renderAStockMarginSummaryField(b *strings.Builder, label string, value string) {
	b.WriteString(`<dt>`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</dt><dd>`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</dd>`)
}

type aStockMarginChartSeries struct {
	Label  string
	Color  string
	Values []*float64
}

func renderAStockMarginTrend(b *strings.Builder, ctx aStockMarginPageContext) {
	metric := aStockMarginMetricDefFor(ctx.TrendMetric)
	days := normalizeAStockMarginTrendDays(ctx.TrendDays)
	b.WriteString(`<section><div class="margin-trend-head"><div><h3>趋势</h3><p class="margin-muted">`)
	b.WriteString(html.EscapeString(fmt.Sprintf("最近 %d 个已落库交易日，当前指标：%s。", days, metric.Label)))
	b.WriteString(`</p></div>`)
	renderAStockMarginTrendControls(b, ctx, metric, days)
	b.WriteString(`</div>`)
	if strings.TrimSpace(ctx.TrendError) != "" {
		b.WriteString(`<div class="margin-error">趋势数据读取失败：`)
		b.WriteString(html.EscapeString(ctx.TrendError))
		b.WriteString(`</div>`)
		b.WriteString(`</section>`)
		return
	}
	dates := aStockMarginChronologicalDates(ctx.Trend.Dates)
	if len(dates) == 0 {
		b.WriteString(`<div class="margin-empty">暂无可展示的融资融券趋势。</div></section>`)
		return
	}
	b.WriteString(`<div class="margin-trend-grid">`)
	renderAStockMarginTrendChart(b, "市场汇总趋势", metric.Unit, dates, buildAStockMarginSummaryChartSeries(ctx.Trend.SummarySeries, dates, metric))
	if strings.TrimSpace(ctx.TrendCode) != "" {
		detailTitle := "标的趋势：" + ctx.TrendCode
		if len(ctx.Trend.DetailSeries) > 0 {
			first := ctx.Trend.DetailSeries[0]
			detailTitle = "标的趋势：" + first.Code
			if strings.TrimSpace(first.Name) != "" {
				detailTitle += " " + strings.TrimSpace(first.Name)
			}
		}
		renderAStockMarginTrendChart(b, detailTitle, metric.Unit, dates, buildAStockMarginDetailChartSeries(ctx.Trend.DetailSeries, dates, metric))
	}
	b.WriteString(`</div></section>`)
}

func renderAStockMarginTrendControls(b *strings.Builder, ctx aStockMarginPageContext, metric aStockMarginMetricDef, days int) {
	b.WriteString(`<div><div class="margin-trend-links">`)
	for _, option := range []int{5, 10, 30} {
		query := aStockMarginQuery(model.AStockMarginFilter{Date: ctx.List.Date, Market: ctx.List.Market, Keyword: ctx.List.Keyword})
		setAStockMarginTrendQuery(query, option, metric.Key, ctx.TrendCode)
		b.WriteString(`<a class="margin-trend-link`)
		if option == days {
			b.WriteString(` active`)
		}
		b.WriteString(`" href="/a-stock/margin?`)
		b.WriteString(html.EscapeString(query.Encode()))
		b.WriteString(`">`)
		b.WriteString(fmt.Sprintf("%d日", option))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div><form class="margin-trend-toolbar" method="get" action="/a-stock/margin">`)
	b.WriteString(`<input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.List.Date))
	b.WriteString(`"><input type="hidden" name="market" value="`)
	b.WriteString(html.EscapeString(ctx.List.Market))
	b.WriteString(`"><input type="hidden" name="keyword" value="`)
	b.WriteString(html.EscapeString(ctx.List.Keyword))
	b.WriteString(`"><input type="hidden" name="trend_days" value="`)
	b.WriteString(fmt.Sprintf("%d", days))
	b.WriteString(`"><label>趋势指标<select name="trend_metric">`)
	for _, option := range aStockMarginMetricDefs {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(option.Key))
		b.WriteString(`"`)
		if option.Key == metric.Key {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(option.Label))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></label><label>标的代码<input name="trend_code" value="`)
	b.WriteString(html.EscapeString(ctx.TrendCode))
	b.WriteString(`" placeholder="000001"></label><button type="submit">应用</button></form></div>`)
}

func buildAStockMarginSummaryChartSeries(items []model.AStockMarginSummaryTrendSeries, dates []string, metric aStockMarginMetricDef) []aStockMarginChartSeries {
	series := make([]aStockMarginChartSeries, 0, len(items))
	for idx, item := range items {
		byDate := map[string]*float64{}
		for _, point := range item.Items {
			byDate[point.TradeDate] = metric.SummaryValue(point)
		}
		values := make([]*float64, 0, len(dates))
		for _, date := range dates {
			values = append(values, byDate[date])
		}
		series = append(series, aStockMarginChartSeries{
			Label:  nonEmpty(item.MarketLabel, aStockMarginMarketLabel(item.Market)),
			Color:  aStockMarginSeriesColor(idx, item.Market),
			Values: values,
		})
	}
	return series
}

func buildAStockMarginDetailChartSeries(items []model.AStockMarginDetailTrendSeries, dates []string, metric aStockMarginMetricDef) []aStockMarginChartSeries {
	series := make([]aStockMarginChartSeries, 0, len(items))
	for idx, item := range items {
		byDate := map[string]*float64{}
		for _, point := range item.Items {
			byDate[point.TradeDate] = metric.DetailValue(point)
		}
		values := make([]*float64, 0, len(dates))
		for _, date := range dates {
			values = append(values, byDate[date])
		}
		label := strings.TrimSpace(item.Code)
		if strings.TrimSpace(item.Name) != "" {
			label += " " + strings.TrimSpace(item.Name)
		}
		if market := nonEmpty(item.MarketLabel, aStockMarginMarketLabel(item.Market)); market != "" {
			label = market + " " + label
		}
		series = append(series, aStockMarginChartSeries{
			Label:  label,
			Color:  aStockMarginSeriesColor(idx, item.Market),
			Values: values,
		})
	}
	return series
}

func renderAStockMarginTrendChart(b *strings.Builder, title string, unit string, dates []string, series []aStockMarginChartSeries) {
	b.WriteString(`<div class="margin-chart"><h4>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h4>`)
	if len(series) == 0 || !aStockMarginChartHasValues(series) {
		b.WriteString(`<div class="margin-empty">暂无该指标趋势数据。</div></div>`)
		return
	}
	minValue, maxValue := aStockMarginChartRange(series)
	if minValue == maxValue {
		minValue -= 1
		maxValue += 1
	}
	width, height := 920.0, 300.0
	left, top, right, bottom := 92.0, 42.0, 24.0, 54.0
	plotWidth, plotHeight := width-left-right, height-top-bottom
	b.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`, width, height, html.EscapeString(title)))
	for i := 0; i <= 4; i++ {
		y := top + float64(i)*plotHeight/4
		value := maxValue - float64(i)*(maxValue-minValue)/4
		b.WriteString(fmt.Sprintf(`<line class="margin-chart-grid" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"></line>`, left, y, width-right, y))
		b.WriteString(fmt.Sprintf(`<text class="margin-chart-axis" x="%.1f" y="%.1f" text-anchor="end">%s</text>`, left-8, y+4, html.EscapeString(formatAStockMarginNumber(&value, unit))))
	}
	labelStep := max(1, (len(dates)+5)/6)
	for i, date := range dates {
		x := aStockMarginChartX(i, len(dates), left, plotWidth)
		if i%labelStep == 0 || i == len(dates)-1 {
			b.WriteString(fmt.Sprintf(`<text class="margin-chart-axis" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`, x, height-20, html.EscapeString(aStockMarginShortDate(date))))
		}
	}
	legendX := left
	for _, item := range series {
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="18" r="4" fill="%s"></circle>`, legendX, item.Color))
		b.WriteString(fmt.Sprintf(`<text class="margin-chart-legend" x="%.1f" y="22" fill="%s">%s</text>`, legendX+8, item.Color, html.EscapeString(item.Label)))
		legendX += 120
	}
	for _, item := range series {
		path := strings.Builder{}
		open := false
		for i, value := range item.Values {
			if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
				open = false
				continue
			}
			x := aStockMarginChartX(i, len(dates), left, plotWidth)
			y := top + (maxValue-*value)/(maxValue-minValue)*plotHeight
			if !open {
				path.WriteString(fmt.Sprintf("M %.1f %.1f", x, y))
				open = true
			} else {
				path.WriteString(fmt.Sprintf(" L %.1f %.1f", x, y))
			}
		}
		if path.Len() > 0 {
			b.WriteString(fmt.Sprintf(`<path class="margin-chart-line" d="%s" stroke="%s"></path>`, path.String(), item.Color))
		}
		for i, value := range item.Values {
			if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
				continue
			}
			x := aStockMarginChartX(i, len(dates), left, plotWidth)
			y := top + (maxValue-*value)/(maxValue-minValue)*plotHeight
			b.WriteString(fmt.Sprintf(`<circle class="margin-chart-point" cx="%.1f" cy="%.1f" r="3.5" fill="%s"><title>%s %s：%s</title></circle>`, x, y, item.Color, html.EscapeString(dates[i]), html.EscapeString(item.Label), html.EscapeString(formatAStockMarginNumber(value, unit))))
		}
	}
	b.WriteString(`</svg><div class="margin-trend-values">`)
	for _, item := range series {
		latest := (*float64)(nil)
		if len(item.Values) > 0 {
			latest = item.Values[len(item.Values)-1]
		}
		b.WriteString(`<span>`)
		b.WriteString(html.EscapeString(item.Label))
		b.WriteString(`：<b>`)
		b.WriteString(html.EscapeString(formatAStockMarginNumber(latest, unit)))
		b.WriteString(`</b></span>`)
	}
	b.WriteString(`</div></div>`)
}

func aStockMarginChartHasValues(series []aStockMarginChartSeries) bool {
	for _, item := range series {
		for _, value := range item.Values {
			if value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
				return true
			}
		}
	}
	return false
}

func aStockMarginChartRange(series []aStockMarginChartSeries) (float64, float64) {
	minValue := math.Inf(1)
	maxValue := math.Inf(-1)
	for _, item := range series {
		for _, value := range item.Values {
			if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
				continue
			}
			minValue = math.Min(minValue, *value)
			maxValue = math.Max(maxValue, *value)
		}
	}
	if math.IsInf(minValue, 0) || math.IsInf(maxValue, 0) {
		return 0, 1
	}
	padding := (maxValue - minValue) * 0.08
	if padding == 0 {
		padding = math.Max(1, math.Abs(maxValue)*0.05)
	}
	return minValue - padding, maxValue + padding
}

func aStockMarginChartX(index int, count int, left float64, plotWidth float64) float64 {
	if count <= 1 {
		return left + plotWidth/2
	}
	return left + float64(index)*plotWidth/float64(count-1)
}

func aStockMarginChronologicalDates(dates []string) []string {
	out := append([]string(nil), dates...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func aStockMarginShortDate(date string) string {
	if len(date) >= 10 {
		return date[5:10]
	}
	return date
}

func aStockMarginSeriesColor(index int, market string) string {
	switch normalizeAStockMarginMarket(market) {
	case "sse":
		return "#2563eb"
	case "szse":
		return "#16a34a"
	case "all":
		return "#f97316"
	default:
		palette := []string{"#2563eb", "#16a34a", "#f97316", "#7c3aed", "#dc2626"}
		return palette[index%len(palette)]
	}
}

func renderAStockMarginTable(b *strings.Builder, page aStockMarginPageContext) {
	ctx := page.List
	b.WriteString(`<section><h3>标的明细</h3>`)
	if ctx.Total == 0 || len(ctx.Details) == 0 {
		b.WriteString(`<div class="margin-empty">暂无匹配的融资融券明细。</div></section>`)
		return
	}
	b.WriteString(`<p class="margin-muted">共 `)
	b.WriteString(fmt.Sprintf("%d", ctx.Total))
	b.WriteString(` 条，当前显示 `)
	b.WriteString(fmt.Sprintf("%d", len(ctx.Details)))
	b.WriteString(` 条。</p><div class="margin-scroll"><table class="margin-table"><thead><tr>`)
	for _, th := range []string{"排名", "交易所", "代码", "简称", "融资买入额", "融资余额", "融资偿还额", "融券卖出量", "融券余量", "融券偿还量", "融券余额", "融资融券余额", "趋势"} {
		b.WriteString(`<th>`)
		b.WriteString(th)
		b.WriteString(`</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, item := range ctx.Details {
		b.WriteString(`<tr><td>`)
		b.WriteString(fmt.Sprintf("%d", item.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(nonEmpty(item.MarketLabel, aStockMarginMarketLabel(item.Market))))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Code))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Name))
		b.WriteString(`</td>`)
		renderAStockMarginNumericCell(b, formatAStockMarginMoney(item.MarginBuyAmount))
		renderAStockMarginNumericCell(b, formatAStockMarginMoney(item.MarginBalance))
		renderAStockMarginNumericCell(b, formatAStockMarginMoney(item.MarginRepayAmount))
		renderAStockMarginNumericCell(b, formatAStockMarginQuantity(item.ShortSellVolume))
		renderAStockMarginNumericCell(b, formatAStockMarginQuantity(item.ShortBalanceVolume))
		renderAStockMarginNumericCell(b, formatAStockMarginQuantity(item.ShortRepayVolume))
		renderAStockMarginNumericCell(b, formatAStockMarginMoney(item.ShortBalanceAmount))
		renderAStockMarginNumericCell(b, formatAStockMarginMoney(item.MarginTradingBalance))
		query := aStockMarginQuery(model.AStockMarginFilter{Date: ctx.Date, Market: ctx.Market, Keyword: ctx.Keyword})
		setAStockMarginTrendQuery(query, page.TrendDays, page.TrendMetric, item.Code)
		b.WriteString(`<td><a class="margin-trend-link`)
		if normalizeAStockMarginCode(page.TrendCode) == normalizeAStockMarginCode(item.Code) {
			b.WriteString(` active`)
		}
		b.WriteString(`" href="/a-stock/margin?`)
		b.WriteString(html.EscapeString(query.Encode()))
		b.WriteString(`">趋势</a></td>`)
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table></div></section>`)
}

func renderAStockMarginNumericCell(b *strings.Builder, value string) {
	b.WriteString(`<td class="margin-num">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</td>`)
}

func aStockMarginQuery(filter model.AStockMarginFilter) url.Values {
	query := url.Values{}
	if strings.TrimSpace(filter.Date) != "" {
		query.Set("date", strings.TrimSpace(filter.Date))
	}
	if market := normalizeAStockMarginMarket(filter.Market); market != "" && market != "all" {
		query.Set("market", market)
	}
	if strings.TrimSpace(filter.Keyword) != "" {
		query.Set("keyword", strings.TrimSpace(filter.Keyword))
	}
	return query
}

func normalizeAStockMarginMarket(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sse", "sh", "shanghai", "沪", "沪市", "上海":
		return "sse"
	case "szse", "sz", "shenzhen", "深", "深市", "深圳":
		return "szse"
	default:
		return "all"
	}
}

func aStockMarginMarketLabel(market string) string {
	switch normalizeAStockMarginMarket(market) {
	case "sse":
		return "沪市"
	case "szse":
		return "深市"
	default:
		return "合计"
	}
}

func aStockMarginMetricDefFor(key string) aStockMarginMetricDef {
	normalized := normalizeAStockMarginTrendMetric(key)
	for _, item := range aStockMarginMetricDefs {
		if item.Key == normalized {
			return item
		}
	}
	return aStockMarginMetricDefs[len(aStockMarginMetricDefs)-1]
}

func normalizeAStockMarginTrendDaysText(value string) int {
	days, _ := strconv.Atoi(strings.TrimSpace(value))
	return normalizeAStockMarginTrendDays(days)
}

func normalizeAStockMarginTrendDays(days int) int {
	switch days {
	case 10, 30:
		return days
	default:
		return 5
	}
}

func normalizeAStockMarginTrendMetric(value string) string {
	raw := strings.TrimSpace(value)
	for _, item := range aStockMarginMetricDefs {
		if item.Key == raw {
			return item.Key
		}
	}
	return aStockMarginDefaultTrendMetric
}

func normalizeAStockMarginBackfillDays(value string) int {
	days, _ := strconv.Atoi(strings.TrimSpace(value))
	switch days {
	case 10, 30:
		return days
	default:
		return 5
	}
}

func normalizeAStockMarginCode(raw string) string {
	text := strings.TrimSpace(strings.ToLower(raw))
	if text == "" {
		return ""
	}
	var digits strings.Builder
	for _, r := range text {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	value := digits.String()
	if len(value) >= 6 {
		return value[len(value)-6:]
	}
	if value == "" {
		return ""
	}
	for len(value) < 6 {
		value = "0" + value
	}
	return value
}

func setAStockMarginTrendQuery(query url.Values, days int, metric string, code string) {
	query.Set("trend_days", fmt.Sprintf("%d", normalizeAStockMarginTrendDays(days)))
	query.Set("trend_metric", normalizeAStockMarginTrendMetric(metric))
	if code = normalizeAStockMarginCode(code); code != "" {
		query.Set("trend_code", code)
	} else {
		query.Del("trend_code")
	}
}

func formatAStockMarginMoney(value *float64) string {
	return formatAStockMarginNumber(value, "元")
}

func formatAStockMarginQuantity(value *float64) string {
	return formatAStockMarginNumber(value, "股/份")
}

func formatAStockMarginNumber(value *float64, unit string) string {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "--"
	}
	abs := math.Abs(*value)
	switch {
	case abs >= 100000000:
		return fmt.Sprintf("%.2f亿%s", *value/100000000, unit)
	case abs >= 10000:
		return fmt.Sprintf("%.2f万%s", *value/10000, unit)
	default:
		return fmt.Sprintf("%.0f%s", *value, unit)
	}
}

func (s *Server) triggerAStockMarginRefresh(date string) string {
	req := s.client.R().SetHeader("X-Service-Token", s.cfg.ServiceToken)
	if strings.TrimSpace(date) != "" {
		req.SetQueryParam("date", strings.TrimSpace(date))
	}
	resp, err := req.Post(s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/margin-trading/latest")
	if err != nil {
		return "融资融券刷新失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "融资融券刷新失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	var envelope struct {
		Data struct {
			Result struct {
				Date      string   `json:"date"`
				Summaries int      `json:"summaries"`
				Details   int      `json:"details"`
				Errors    []string `json:"source_errors"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err == nil && envelope.Data.Result.Date != "" {
		message := fmt.Sprintf("融资融券已刷新：%s，汇总 %d 条，明细 %d 条。", envelope.Data.Result.Date, envelope.Data.Result.Summaries, envelope.Data.Result.Details)
		if len(envelope.Data.Result.Errors) > 0 {
			message += " 部分来源失败：" + strings.Join(envelope.Data.Result.Errors, "；")
		}
		return message
	}
	return "融资融券刷新已完成，请查看当前列表。"
}

func (s *Server) triggerAStockMarginBackfill(days int, start string, end string) string {
	query := url.Values{}
	if days > 0 {
		query.Set("days", fmt.Sprintf("%d", normalizeAStockMarginTrendDays(days)))
	}
	if strings.TrimSpace(start) != "" {
		query.Set("start", strings.TrimSpace(start))
	}
	if strings.TrimSpace(end) != "" {
		query.Set("end", strings.TrimSpace(end))
	}
	endpoint := s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/margin-trading/backfill"
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(endpoint)
	if err != nil {
		return "融资融券回填失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "融资融券回填失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	var envelope struct {
		Data struct {
			Result struct {
				Requested int      `json:"requested"`
				Succeeded int      `json:"succeeded"`
				Skipped   int      `json:"skipped"`
				Failed    int      `json:"failed"`
				Summaries int      `json:"summaries"`
				Details   int      `json:"details"`
				Errors    []string `json:"errors"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err == nil && envelope.Data.Result.Requested > 0 {
		result := envelope.Data.Result
		message := fmt.Sprintf("融资融券回填完成：请求 %d 个交易日，成功 %d，跳过 %d，失败 %d，汇总 %d 条，明细 %d 条。", result.Requested, result.Succeeded, result.Skipped, result.Failed, result.Summaries, result.Details)
		if len(result.Errors) > 0 {
			errors := result.Errors
			if len(errors) > 3 {
				errors = errors[:3]
			}
			message += " 部分异常：" + strings.Join(errors, "；")
		}
		return message
	}
	return "融资融券回填已完成，请查看当前列表。"
}
