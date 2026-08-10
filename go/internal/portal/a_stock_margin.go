package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const aStockMarginPageSize = 200

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
	ctx, err := s.loadAStockMarginContext(filter)
	var b strings.Builder
	b.WriteString(`<style>
body[data-page='a-stock-margin'] main,body[data-page='a-stock-margin'] .site-footer{max-width:none;width:100%;box-sizing:border-box}
body[data-page='a-stock-margin'] main{padding-left:18px;padding-right:18px;font-size:14px;line-height:1.45}
body[data-page='a-stock-margin'] section{width:100%;box-sizing:border-box}
.margin-muted{color:#6a6257}.margin-head{display:flex;align-items:flex-start;justify-content:space-between;gap:14px;flex-wrap:wrap}.margin-head h2{margin-bottom:6px}
.margin-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:center}.margin-actions form{margin:0}.margin-actions button{margin:0}
.margin-toolbar{display:grid;grid-template-columns:150px 140px minmax(220px,.35fr) 110px;gap:10px;align-items:end;margin-top:12px}.margin-toolbar button{margin:0}
.margin-message{padding:12px;border-radius:8px;background:#e7f4ea;color:#214e34;margin:12px 0}.margin-error{padding:14px;border:1px dashed #d0c8b8;border-radius:8px;background:#fff;color:#8a4b00}
.margin-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;margin:14px 0}.margin-card{padding:14px;border:1px solid #ece7dc;border-radius:8px;background:#fff}.margin-card h3{margin:0 0 8px;font-size:15px}.margin-card dl{display:grid;grid-template-columns:1fr auto;gap:6px 10px;margin:0}.margin-card dt{color:#6a6257}.margin-card dd{margin:0;font-weight:700;white-space:nowrap}
.margin-scroll{overflow:auto}.margin-table{min-width:1380px;width:100%;table-layout:fixed}.margin-table th,.margin-table td{vertical-align:middle;white-space:nowrap}.margin-table th{text-align:left}.margin-table th:nth-child(1),.margin-table td:nth-child(1){width:58px;text-align:center}.margin-table th:nth-child(2),.margin-table td:nth-child(2){width:70px;text-align:center}.margin-table th:nth-child(3),.margin-table td:nth-child(3){width:90px}.margin-table th:nth-child(4),.margin-table td:nth-child(4){width:140px}.margin-num{text-align:right}.margin-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:8px;background:#fff;color:#6a6257}
@media (max-width:760px){.margin-toolbar{grid-template-columns:1fr}.margin-head{display:block}}
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
	renderAStockMarginSummary(&b, ctx)
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
	message := "未知操作"
	if strings.TrimSpace(r.FormValue("action")) == "refresh_margin_trading" {
		message = s.triggerAStockMarginRefresh(filter.Date)
	}
	query := aStockMarginQuery(filter)
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

func renderAStockMarginActions(b *strings.Builder, ctx model.AStockMarginListResult) {
	b.WriteString(`<div class="margin-actions"><form method="post" action="/a-stock/margin">`)
	b.WriteString(`<input type="hidden" name="action" value="refresh_margin_trading">`)
	b.WriteString(`<input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"><input type="hidden" name="market" value="`)
	b.WriteString(html.EscapeString(ctx.Market))
	b.WriteString(`"><input type="hidden" name="keyword" value="`)
	b.WriteString(html.EscapeString(ctx.Keyword))
	b.WriteString(`"><button type="submit">刷新融资融券</button></form></div>`)
}

func renderAStockMarginFilters(b *strings.Builder, ctx model.AStockMarginListResult) {
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
	b.WriteString(`" placeholder="000001 或 平安银行"></label><button type="submit">查询</button></form>`)
	if len(ctx.Dates) > 0 {
		b.WriteString(`<p class="margin-muted">已有日期：`)
		limit := min(len(ctx.Dates), 8)
		for i := 0; i < limit; i++ {
			if i > 0 {
				b.WriteString(` · `)
			}
			query := aStockMarginQuery(model.AStockMarginFilter{Date: ctx.Dates[i], Market: ctx.Market, Keyword: ctx.Keyword})
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

func renderAStockMarginTable(b *strings.Builder, ctx model.AStockMarginListResult) {
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
	for _, th := range []string{"排名", "交易所", "代码", "简称", "融资买入额", "融资余额", "融资偿还额", "融券卖出量", "融券余量", "融券偿还量", "融券余额", "融资融券余额"} {
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
