package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

const sectorFundFlowPageSize = 200
const sectorFundFlowStockPageSize = 500
const sectorFundFlowConstituentTimeout = 1500 * time.Millisecond

type sectorFundFlowStockContext struct {
	SectorName      string
	Constituents    []model.AStockSectorConstituent
	ConstituentWarn string
	Stocks          model.AStockStockFundFlowListResult
	Error           string
}

type sectorFundFlowConstituentPayload struct {
	Items      []model.AStockCodeName `json:"items"`
	Count      int                    `json:"count"`
	Warning    string                 `json:"warning"`
	SourceErrs []string               `json:"source_errors"`
}

type sectorFundFlowTrendContext struct {
	Mode        string
	Title       string
	Days        int
	SectorName  string
	StockCode   string
	StockName   string
	SectorTrend model.AStockSectorFundFlowTrendResult
	StockTrend  model.AStockStockFundFlowTrendResult
	Error       string
}

type sectorFundFlowStockSearchContext struct {
	Keyword string
	Stocks  model.AStockStockFundFlowListResult
	Trend   model.AStockStockFundFlowTrendResult
	Error   string
}

func (s *Server) handleSectorFundFlowPage(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	if r.Method == http.MethodPost {
		s.handleSectorFundFlowAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := sectorFundFlowFilterFromRequest(r)
	ctx, err := s.loadSectorFundFlowContext(filter)
	selectedSector := strings.TrimSpace(nonEmpty(r.URL.Query().Get("sector_name"), r.URL.Query().Get("sector")))
	var stockCtx sectorFundFlowStockContext
	if err == nil && selectedSector != "" {
		stockCtx = s.loadSectorFundFlowStocks(ctx, selectedSector)
	}
	var trendCtx sectorFundFlowTrendContext
	if err == nil {
		trendCtx = s.loadSectorFundFlowTrendContext(r, ctx)
	}
	var stockSearchCtx sectorFundFlowStockSearchContext
	if err == nil && strings.TrimSpace(ctx.Keyword) != "" {
		stockSearchCtx = s.loadSectorFundFlowStockSearchContext(ctx)
	}

	var b strings.Builder
	b.WriteString(`<style>
body[data-page='sector-fund-flow'] main,body[data-page='sector-fund-flow'] .site-footer{max-width:none;width:100%;box-sizing:border-box}
body[data-page='sector-fund-flow'] main{padding-left:18px;padding-right:18px;font-size:14px;line-height:1.45}
body[data-page='sector-fund-flow'] section{width:100%;box-sizing:border-box}
.sector-muted{color:#6a6257}.sector-message{padding:12px;border-radius:8px;background:#e7f4ea;color:#214e34;margin:12px 0}
.sector-head{display:flex;align-items:flex-start;justify-content:space-between;gap:14px;flex-wrap:wrap}.sector-head h2{margin-bottom:6px}
.sector-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:center}.sector-actions form{margin:0}.sector-actions button{margin:0}
.sector-tabs{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}.sector-tab{display:inline-flex;align-items:center;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}.sector-tab.active{background:#214e34;color:#fff;border-color:#214e34}
.sector-toolbar{display:grid;grid-template-columns:minmax(160px,.25fr) minmax(180px,.35fr) 110px;gap:10px;align-items:end;margin-top:10px}.sector-toolbar button{margin:0}
.sector-date-list{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}.sector-date{display:inline-flex;align-items:center;padding:7px 11px;border:1px solid #d6ccbb;border-radius:8px;background:#fff;color:#214e34;text-decoration:none}.sector-date.active{background:#214e34;color:#fff;border-color:#214e34}
.sector-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px}.sector-card{padding:16px;border:1px solid #ece7dc;border-radius:8px;background:#fff}.sector-card strong{display:block;font-size:20px;margin-top:6px}
.sector-scroll{overflow:auto}.sector-table{min-width:1440px;width:100%;table-layout:fixed}.sector-table th,.sector-table td{vertical-align:middle;white-space:nowrap}.sector-table th{font-weight:700;text-align:left}.sector-table th:nth-child(1),.sector-table td:nth-child(1){width:54px;text-align:center}.sector-table th:nth-child(2),.sector-table td:nth-child(2){width:140px;text-align:left}.sector-table th:nth-child(3),.sector-table td:nth-child(3){width:70px;text-align:center}.sector-table th:nth-child(4),.sector-table th:nth-child(5),.sector-table th:nth-child(6),.sector-table th:nth-child(7),.sector-table th:nth-child(8),.sector-table th:nth-child(9),.sector-table th:nth-child(10),.sector-table th:nth-child(11),.sector-table th:nth-child(12){text-align:right}.sector-table th:nth-child(13),.sector-table td:nth-child(13){width:150px;text-align:left}.sector-table th:nth-child(14),.sector-table td:nth-child(14){width:150px;text-align:left}.sector-num{text-align:right;white-space:nowrap}.sector-positive{color:#d93025;font-weight:700}.sector-negative{color:#087333;font-weight:700}.sector-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:8px;background:#fff;color:#6a6257}
.sector-name-link,.sector-trend-link{color:#214e34;font-weight:700;text-decoration:none}.sector-name-link:hover,.sector-trend-link:hover{text-decoration:underline}.sector-name-link.active{color:#0b5cab}.sector-detail-head{display:flex;align-items:flex-end;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:10px}.sector-detail-head h3{margin:0}.sector-stock-table{min-width:1460px}.sector-stock-table th:nth-child(2),.sector-stock-table td:nth-child(2){width:90px}.sector-stock-table th:nth-child(3),.sector-stock-table td:nth-child(3){width:120px;text-align:left}.sector-stock-table td.sector-num{text-align:right}.sector-stock-note{margin-top:6px}.sector-trend-tabs{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin:10px 0}
@media (max-width:760px){.sector-toolbar{grid-template-columns:1fr}.sector-head{display:block}}
</style>`)
	b.WriteString(`<section><div class="sector-head"><div><h2>版块资金</h2><p class="sector-muted">展示 AKShare 行业/概念版块资金流入流出，支持今日、5日、10日切换。</p></div>`)
	renderSectorFundFlowActions(&b, ctx)
	b.WriteString(`</div></section>`)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<div class="sector-message">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	if err != nil {
		b.WriteString(`<section><div class="sector-empty">版块资金数据读取失败：`)
		b.WriteString(html.EscapeString(err.Error()))
		b.WriteString(`</div></section>`)
		_ = s.writeSimplePage(w, "sector-fund-flow", "版块资金", b.String())
		return
	}
	renderSectorFundFlowSummary(&b, ctx)
	renderSectorFundFlowFilters(&b, ctx)
	renderSectorFundFlowTable(&b, ctx, selectedSector)
	renderSectorFundFlowTrend(&b, ctx, trendCtx)
	renderSectorFundFlowStockTable(&b, ctx, stockCtx)
	renderSectorFundFlowStockSearch(&b, ctx, stockSearchCtx)
	_ = s.writeSimplePage(w, "sector-fund-flow", "版块资金", b.String())
}

func (s *Server) handleSectorFundFlowAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := model.AStockSectorFundFlowFilter{
		Date:       strings.TrimSpace(r.FormValue("date")),
		SectorType: strings.TrimSpace(r.FormValue("sector_type")),
		Indicator:  strings.TrimSpace(r.FormValue("indicator")),
		Keyword:    strings.TrimSpace(r.FormValue("keyword")),
	}
	message := "未知操作"
	if strings.TrimSpace(r.FormValue("action")) == "refresh_sector_fund_flow" {
		message = s.triggerSectorFundFlowRefresh()
	}
	query := sectorFundFlowQuery(filter)
	query.Set("msg", message)
	http.Redirect(w, r, "/sector-fund-flow?"+query.Encode(), http.StatusSeeOther)
}

func sectorFundFlowFilterFromRequest(r *http.Request) model.AStockSectorFundFlowFilter {
	return model.AStockSectorFundFlowFilter{
		Date:       strings.TrimSpace(r.URL.Query().Get("date")),
		SectorType: strings.TrimSpace(r.URL.Query().Get("sector_type")),
		Indicator:  strings.TrimSpace(r.URL.Query().Get("indicator")),
		Keyword:    strings.TrimSpace(r.URL.Query().Get("keyword")),
		Page:       normalizeAStockNewsPage(r.URL.Query().Get("page")),
		PageSize:   sectorFundFlowPageSize,
	}
}

func (s *Server) loadSectorFundFlowContext(filter model.AStockSectorFundFlowFilter) (model.AStockSectorFundFlowListResult, error) {
	query := sectorFundFlowQuery(filter)
	query.Set("page", fmt.Sprintf("%d", max(filter.Page, 1)))
	query.Set("page_size", fmt.Sprintf("%d", sectorFundFlowPageSize))
	var result model.AStockSectorFundFlowListResult
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-fund-flows?"+query.Encode(), &result)
	return result, err
}

func (s *Server) loadSectorFundFlowStocks(ctx model.AStockSectorFundFlowListResult, sectorName string) sectorFundFlowStockContext {
	stockCtx := sectorFundFlowStockContext{SectorName: strings.TrimSpace(sectorName)}
	if stockCtx.SectorName == "" {
		return stockCtx
	}
	constituents, warn, err := s.loadSectorFundFlowCachedConstituents(ctx, stockCtx.SectorName)
	if err != nil {
		stockCtx.Error = "板块成分股缓存读取失败：" + err.Error()
		return stockCtx
	}
	if len(constituents) == 0 {
		constituents, warn, err = s.refreshSectorFundFlowConstituents(ctx, stockCtx.SectorName)
		if err != nil {
			stockCtx.Error = "暂无成分股缓存，实时刷新失败。请稍后重试或先刷新版块资金。"
			return stockCtx
		}
	}
	stockCtx.Constituents = constituents
	stockCtx.ConstituentWarn = strings.TrimSpace(warn)
	codes := make([]string, 0, len(constituents))
	for _, item := range constituents {
		code := strings.TrimSpace(item.Code)
		if code != "" {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		stockCtx.Error = "板块成分股为空，无法匹配个股资金流。"
		if stockCtx.ConstituentWarn != "" {
			stockCtx.Error += " " + stockCtx.ConstituentWarn
		}
		return stockCtx
	}
	stockQuery := url.Values{}
	if strings.TrimSpace(ctx.Date) != "" {
		stockQuery.Set("date", strings.TrimSpace(ctx.Date))
	}
	stockQuery.Set("indicator", normalizeSectorFundFlowIndicator(ctx.Indicator))
	stockQuery.Set("codes", strings.Join(codes, ","))
	stockQuery.Set("page", "1")
	stockQuery.Set("page_size", fmt.Sprintf("%d", sectorFundFlowStockPageSize))
	var stocks model.AStockStockFundFlowListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/stock-fund-flows?"+stockQuery.Encode(), &stocks); err != nil {
		stockCtx.Error = "个股资金流读取失败：" + err.Error()
		return stockCtx
	}
	stockCtx.Stocks = stocks
	return stockCtx
}

func (s *Server) loadSectorFundFlowCachedConstituents(ctx model.AStockSectorFundFlowListResult, sectorName string) ([]model.AStockSectorConstituent, string, error) {
	query := url.Values{}
	query.Set("sector_type", normalizeSectorFundFlowSectorType(ctx.SectorType))
	query.Set("sector_name", strings.TrimSpace(sectorName))
	query.Set("limit", fmt.Sprintf("%d", sectorFundFlowStockPageSize))
	var result model.AStockSectorConstituentListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-constituents?"+query.Encode(), &result); err != nil {
		return nil, "", err
	}
	return result.Items, "", nil
}

func (s *Server) refreshSectorFundFlowConstituents(ctx model.AStockSectorFundFlowListResult, sectorName string) ([]model.AStockSectorConstituent, string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return nil, "", fmt.Errorf("akshare constituent service not configured")
	}
	query := url.Values{}
	query.Set("sector_type", normalizeSectorFundFlowSectorType(ctx.SectorType))
	query.Set("sector_name", strings.TrimSpace(sectorName))
	query.Set("indicator", normalizeSectorFundFlowIndicator(ctx.Indicator))
	query.Set("limit", fmt.Sprintf("%d", sectorFundFlowStockPageSize))
	var payload sectorFundFlowConstituentPayload
	reqCtx, cancel := context.WithTimeout(context.Background(), sectorFundFlowConstituentTimeout)
	defer cancel()
	resp, err := s.client.R().SetContext(reqCtx).SetResult(&payload).Get(baseURL + "/api/a-stock/sector-constituents?" + query.Encode())
	if err != nil {
		return nil, "", err
	}
	if !resp.IsSuccess() {
		return nil, "", fmt.Errorf("akshare constituent status %s", resp.Status())
	}
	constituents := make([]model.AStockSectorConstituent, 0, len(payload.Items))
	for _, item := range payload.Items {
		code := astockcode.Normalize(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !astockcode.HasResolvedName(code, name) {
			continue
		}
		constituents = append(constituents, model.AStockSectorConstituent{
			SectorType: normalizeSectorFundFlowSectorType(ctx.SectorType),
			SectorName: strings.TrimSpace(sectorName),
			Code:       code,
			Name:       name,
			Source:     strings.TrimSpace(item.Source),
			FetchedAt:  time.Now().UTC(),
		})
	}
	if len(constituents) == 0 {
		return nil, strings.TrimSpace(payload.Warning), fmt.Errorf("akshare constituent rows empty")
	}
	s.saveSectorFundFlowConstituents(ctx, sectorName, constituents)
	return constituents, strings.TrimSpace(payload.Warning), nil
}

func (s *Server) saveSectorFundFlowConstituents(ctx model.AStockSectorFundFlowListResult, sectorName string, items []model.AStockSectorConstituent) {
	if len(items) == 0 {
		return
	}
	body := map[string]any{
		"sector_type": normalizeSectorFundFlowSectorType(ctx.SectorType),
		"sector_name": strings.TrimSpace(sectorName),
		"items":       items,
		"replace":     true,
	}
	_, _ = s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		SetBody(body).
		Post(s.cfg.ContentURL + "/api/v1/internal/a-stock/sector-constituents")
}

func sectorFundFlowQuery(filter model.AStockSectorFundFlowFilter) url.Values {
	query := url.Values{}
	if strings.TrimSpace(filter.Date) != "" {
		query.Set("date", strings.TrimSpace(filter.Date))
	}
	query.Set("sector_type", normalizeSectorFundFlowSectorType(filter.SectorType))
	query.Set("indicator", normalizeSectorFundFlowIndicator(filter.Indicator))
	if strings.TrimSpace(filter.Keyword) != "" {
		query.Set("keyword", strings.TrimSpace(filter.Keyword))
	}
	return query
}

func renderSectorFundFlowActions(b *strings.Builder, ctx model.AStockSectorFundFlowListResult) {
	b.WriteString(`<div class="sector-actions"><form method="post">`)
	b.WriteString(`<input type="hidden" name="action" value="refresh_sector_fund_flow">`)
	b.WriteString(`<input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"><input type="hidden" name="sector_type" value="`)
	b.WriteString(html.EscapeString(ctx.SectorType))
	b.WriteString(`"><input type="hidden" name="indicator" value="`)
	b.WriteString(html.EscapeString(ctx.Indicator))
	b.WriteString(`"><input type="hidden" name="keyword" value="`)
	b.WriteString(html.EscapeString(ctx.Keyword))
	b.WriteString(`"><button type="submit">刷新版块资金</button></form></div>`)
}

func renderSectorFundFlowSummary(b *strings.Builder, ctx model.AStockSectorFundFlowListResult) {
	fetchedAt := "--"
	if ctx.FetchedAt != nil && !ctx.FetchedAt.IsZero() {
		fetchedAt = ctx.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")
	}
	topName := "--"
	topValue := "--"
	if len(ctx.Items) > 0 {
		topName = ctx.Items[0].Name
		topValue = formatSectorFundFlowMoney(ctx.Items[0].MainNetInflow)
	}
	b.WriteString(`<section><div class="sector-grid">`)
	writeSectorFundFlowMetric(b, "交易日", nonEmptyText(ctx.Date, "--"))
	writeSectorFundFlowMetric(b, "当前口径", strings.TrimSpace(ctx.SectorType+" / "+ctx.Indicator))
	writeSectorFundFlowMetric(b, "版块数量", fmt.Sprintf("%d", ctx.Total))
	writeSectorFundFlowMetric(b, "主力净流入第一", topName+" "+topValue)
	writeSectorFundFlowMetric(b, "更新时间", fetchedAt)
	b.WriteString(`</div></section>`)
}

func writeSectorFundFlowMetric(b *strings.Builder, label string, value string) {
	b.WriteString(`<div class="sector-card"><span class="sector-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong>`)
	b.WriteString(html.EscapeString(nonEmptyText(value, "--")))
	b.WriteString(`</strong></div>`)
}

func renderSectorFundFlowFilters(b *strings.Builder, ctx model.AStockSectorFundFlowListResult) {
	b.WriteString(`<section>`)
	renderSectorFundFlowTabs(b, ctx, "sector_type", []string{"行业资金流", "概念资金流"})
	renderSectorFundFlowTabs(b, ctx, "indicator", []string{"今日", "5日", "10日"})
	if len(ctx.Dates) > 0 {
		b.WriteString(`<div class="sector-date-list">`)
		for _, date := range ctx.Dates {
			className := "sector-date"
			if date == ctx.Date {
				className += " active"
			}
			linkFilter := model.AStockSectorFundFlowFilter{Date: date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword}
			b.WriteString(`<a class="`)
			b.WriteString(className)
			b.WriteString(`" href="/sector-fund-flow?`)
			b.WriteString(html.EscapeString(sectorFundFlowQuery(linkFilter).Encode()))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(date))
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<form class="sector-toolbar" method="get" action="/sector-fund-flow"><label>交易日<input name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`" placeholder="YYYY-MM-DD"></label><label>搜索<input name="keyword" value="`)
	b.WriteString(html.EscapeString(ctx.Keyword))
	b.WriteString(`" placeholder="版块或最大股"></label><input type="hidden" name="sector_type" value="`)
	b.WriteString(html.EscapeString(ctx.SectorType))
	b.WriteString(`"><input type="hidden" name="indicator" value="`)
	b.WriteString(html.EscapeString(ctx.Indicator))
	b.WriteString(`"><button type="submit">筛选</button></form></section>`)
}

func renderSectorFundFlowTabs(b *strings.Builder, ctx model.AStockSectorFundFlowListResult, field string, options []string) {
	b.WriteString(`<div class="sector-tabs">`)
	for _, option := range options {
		linkFilter := model.AStockSectorFundFlowFilter{Date: ctx.Date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword}
		if field == "sector_type" {
			linkFilter.SectorType = option
		} else {
			linkFilter.Indicator = option
		}
		active := option == ctx.SectorType || option == ctx.Indicator
		className := "sector-tab"
		if active {
			className += " active"
		}
		b.WriteString(`<a class="`)
		b.WriteString(className)
		b.WriteString(`" href="/sector-fund-flow?`)
		b.WriteString(html.EscapeString(sectorFundFlowQuery(linkFilter).Encode()))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(option))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func renderSectorFundFlowTable(b *strings.Builder, ctx model.AStockSectorFundFlowListResult, selectedSector string) {
	b.WriteString(`<section><div class="sector-scroll">`)
	if len(ctx.Items) == 0 {
		b.WriteString(`<div class="sector-empty">暂无版块资金数据，请点击“刷新版块资金”，或等待交易时段自动抓取。</div></div></section>`)
		return
	}
	b.WriteString(`<table class="sector-table"><tr><th>排名</th><th>版块名称</th><th>趋势</th><th>涨跌幅</th><th>主力净流入</th><th>主力净占比</th><th>超大单</th><th>大单</th><th>中单</th><th>小单</th><th>超大单占比</th><th>大单占比</th><th>主力净流入最大股</th><th>更新时间</th></tr>`)
	for _, item := range ctx.Items {
		b.WriteString(`<tr><td>`)
		b.WriteString(fmt.Sprintf("%d", item.Rank))
		b.WriteString(`</td><td>`)
		linkFilter := model.AStockSectorFundFlowFilter{Date: ctx.Date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword}
		linkQuery := sectorFundFlowQuery(linkFilter)
		linkQuery.Set("sector_name", item.Name)
		className := "sector-name-link"
		if item.Name == selectedSector {
			className += " active"
		}
		b.WriteString(`<a class="`)
		b.WriteString(className)
		b.WriteString(`" href="/sector-fund-flow?`)
		b.WriteString(html.EscapeString(linkQuery.Encode()))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(item.Name))
		b.WriteString(`</a>`)
		trendQuery := sectorFundFlowQuery(linkFilter)
		trendQuery.Set("trend", "sector")
		trendQuery.Set("sector_name", item.Name)
		trendQuery.Set("trend_days", "5")
		b.WriteString(`</td><td><a class="sector-trend-link" href="/sector-fund-flow?`)
		b.WriteString(html.EscapeString(trendQuery.Encode()))
		b.WriteString(`">趋势</a>`)
		b.WriteString(`</td><td class="sector-num `)
		b.WriteString(sectorFundFlowValueClass(item.ChangePct))
		b.WriteString(`">`)
		b.WriteString(formatSectorFundFlowPct(item.ChangePct))
		b.WriteString(`</td>`)
		writeSectorFundFlowMoneyCell(b, item.MainNetInflow)
		writeSectorFundFlowPctCell(b, item.MainNetInflowPct)
		writeSectorFundFlowMoneyCell(b, item.SuperLargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.LargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.MediumNetInflow)
		writeSectorFundFlowMoneyCell(b, item.SmallNetInflow)
		writeSectorFundFlowPctCell(b, item.SuperLargeNetInflowPct)
		writeSectorFundFlowPctCell(b, item.LargeNetInflowPct)
		b.WriteString(`<td>`)
		b.WriteString(html.EscapeString(nonEmptyText(item.TopStock, "--")))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></section>`)
}

func (s *Server) loadSectorFundFlowTrendContext(r *http.Request, ctx model.AStockSectorFundFlowListResult) sectorFundFlowTrendContext {
	mode := strings.TrimSpace(r.URL.Query().Get("trend"))
	days := normalizeSectorFundFlowTrendDays(r.URL.Query().Get("trend_days"))
	trendCtx := sectorFundFlowTrendContext{Mode: mode, Days: days}
	switch mode {
	case "sector":
		sectorName := strings.TrimSpace(nonEmpty(r.URL.Query().Get("sector_name"), r.URL.Query().Get("sector")))
		if sectorName == "" {
			return trendCtx
		}
		query := url.Values{}
		query.Set("end_date", ctx.Date)
		query.Set("sector_type", normalizeSectorFundFlowSectorType(ctx.SectorType))
		query.Set("sector_name", sectorName)
		query.Set("indicator", "今日")
		query.Set("days", fmt.Sprintf("%d", days))
		var result model.AStockSectorFundFlowTrendResult
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-fund-flow-trend?"+query.Encode(), &result); err != nil {
			trendCtx.Error = "版块趋势读取失败：" + err.Error()
			return trendCtx
		}
		trendCtx.SectorName = sectorName
		trendCtx.Title = sectorName + " 版块主力资金趋势"
		trendCtx.SectorTrend = result
	case "stock":
		code := astockcode.Normalize(r.URL.Query().Get("code"))
		keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
		if code == "" && keyword == "" {
			return trendCtx
		}
		query := url.Values{}
		query.Set("end_date", ctx.Date)
		query.Set("indicator", "今日")
		query.Set("days", fmt.Sprintf("%d", days))
		if code != "" {
			query.Set("code", code)
		} else {
			query.Set("keyword", keyword)
		}
		var result model.AStockStockFundFlowTrendResult
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/stock-fund-flow-trend?"+query.Encode(), &result); err != nil {
			trendCtx.Error = "个股趋势读取失败：" + err.Error()
			return trendCtx
		}
		trendCtx.StockCode = result.Code
		trendCtx.StockName = firstStockFundFlowName(result.Items)
		trendCtx.Title = strings.TrimSpace(result.Code + " " + firstStockFundFlowName(result.Items) + " 个股主力资金趋势")
		if trendCtx.Title == "" {
			trendCtx.Title = nonEmpty(keyword, code) + " 个股主力资金趋势"
		}
		trendCtx.StockTrend = result
	}
	return trendCtx
}

func (s *Server) loadSectorFundFlowStockSearchContext(ctx model.AStockSectorFundFlowListResult) sectorFundFlowStockSearchContext {
	stockCtx := sectorFundFlowStockSearchContext{Keyword: strings.TrimSpace(ctx.Keyword)}
	if stockCtx.Keyword == "" {
		return stockCtx
	}
	query := url.Values{}
	query.Set("date", ctx.Date)
	query.Set("indicator", normalizeSectorFundFlowIndicator(ctx.Indicator))
	query.Set("keyword", stockCtx.Keyword)
	query.Set("page", "1")
	query.Set("page_size", "50")
	var stocks model.AStockStockFundFlowListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/stock-fund-flows?"+query.Encode(), &stocks); err != nil {
		stockCtx.Error = "个股资金流读取失败：" + err.Error()
		return stockCtx
	}
	stockCtx.Stocks = stocks
	if len(stocks.Items) > 0 {
		trendQuery := url.Values{}
		trendQuery.Set("end_date", ctx.Date)
		trendQuery.Set("indicator", "今日")
		trendQuery.Set("code", stocks.Items[0].Code)
		trendQuery.Set("days", "5")
		var trend model.AStockStockFundFlowTrendResult
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/stock-fund-flow-trend?"+trendQuery.Encode(), &trend); err == nil {
			stockCtx.Trend = trend
		}
	}
	return stockCtx
}

func renderSectorFundFlowTrend(b *strings.Builder, ctx model.AStockSectorFundFlowListResult, trendCtx sectorFundFlowTrendContext) {
	if trendCtx.Mode == "" {
		return
	}
	title := nonEmptyText(trendCtx.Title, "主力资金趋势")
	b.WriteString(`<section><div class="sector-detail-head"><div><h3>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h3><p class="sector-muted sector-stock-note">按最近已落库交易日的“今日”资金流展示。</p></div>`)
	backQuery := sectorFundFlowQuery(model.AStockSectorFundFlowFilter{Date: ctx.Date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword})
	b.WriteString(`<a class="sector-tab" href="/sector-fund-flow?`)
	b.WriteString(html.EscapeString(backQuery.Encode()))
	b.WriteString(`">关闭趋势</a></div>`)
	renderSectorFundFlowTrendTabs(b, ctx, trendCtx)
	if trendCtx.Error != "" {
		b.WriteString(`<div class="sector-empty">`)
		b.WriteString(html.EscapeString(trendCtx.Error))
		b.WriteString(`</div></section>`)
		return
	}
	if trendCtx.Mode == "sector" {
		renderSectorFundFlowSectorTrendTable(b, trendCtx)
	} else {
		renderSectorFundFlowStockTrendTable(b, trendCtx)
	}
	b.WriteString(`</section>`)
}

func renderSectorFundFlowTrendTabs(b *strings.Builder, ctx model.AStockSectorFundFlowListResult, trendCtx sectorFundFlowTrendContext) {
	b.WriteString(`<div class="sector-trend-tabs">`)
	for _, days := range []int{5, 10, 30} {
		query := sectorFundFlowQuery(model.AStockSectorFundFlowFilter{Date: ctx.Date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword})
		query.Set("trend", trendCtx.Mode)
		query.Set("trend_days", fmt.Sprintf("%d", days))
		if trendCtx.Mode == "sector" {
			query.Set("sector_name", trendCtx.SectorName)
		} else if trendCtx.StockCode != "" {
			query.Set("code", trendCtx.StockCode)
		} else {
			query.Set("keyword", ctx.Keyword)
		}
		className := "sector-tab"
		if days == trendCtx.Days {
			className += " active"
		}
		b.WriteString(`<a class="`)
		b.WriteString(className)
		b.WriteString(`" href="/sector-fund-flow?`)
		b.WriteString(html.EscapeString(query.Encode()))
		b.WriteString(`">`)
		b.WriteString(fmt.Sprintf("%d日", days))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func renderSectorFundFlowSectorTrendTable(b *strings.Builder, trendCtx sectorFundFlowTrendContext) {
	items := trendCtx.SectorTrend.Items
	if len(items) == 0 {
		b.WriteString(`<div class="sector-empty">暂无版块趋势数据。</div>`)
		return
	}
	if len(items) < trendCtx.Days {
		b.WriteString(`<div class="sector-empty">当前仅有 `)
		b.WriteString(fmt.Sprintf("%d", len(items)))
		b.WriteString(` 个交易日数据。</div>`)
	}
	b.WriteString(`<div class="sector-scroll"><table class="sector-table"><tr><th>日期</th><th>排名</th><th>版块名称</th><th>涨跌幅</th><th>主力净流入</th><th>主力净占比</th><th>超大单</th><th>大单</th><th>中单</th><th>小单</th><th>更新时间</th></tr>`)
	for _, item := range items {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(item.TradeDate))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", item.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Name))
		b.WriteString(`</td>`)
		writeSectorFundFlowPctCell(b, item.ChangePct)
		writeSectorFundFlowMoneyCell(b, item.MainNetInflow)
		writeSectorFundFlowPctCell(b, item.MainNetInflowPct)
		writeSectorFundFlowMoneyCell(b, item.SuperLargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.LargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.MediumNetInflow)
		writeSectorFundFlowMoneyCell(b, item.SmallNetInflow)
		b.WriteString(`<td>`)
		b.WriteString(html.EscapeString(item.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div>`)
}

func renderSectorFundFlowStockTrendTable(b *strings.Builder, trendCtx sectorFundFlowTrendContext) {
	items := trendCtx.StockTrend.Items
	if len(items) == 0 {
		b.WriteString(`<div class="sector-empty">暂无个股趋势数据。</div>`)
		return
	}
	if len(items) < trendCtx.Days {
		b.WriteString(`<div class="sector-empty">当前仅有 `)
		b.WriteString(fmt.Sprintf("%d", len(items)))
		b.WriteString(` 个交易日数据。</div>`)
	}
	b.WriteString(`<div class="sector-scroll"><table class="sector-table sector-stock-table"><tr><th>日期</th><th>排名</th><th>代码</th><th>名称</th><th>最新价</th><th>涨跌幅</th><th>主力净流入</th><th>主力净占比</th><th>超大单</th><th>大单</th><th>中单</th><th>小单</th><th>更新时间</th></tr>`)
	for _, item := range items {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(item.TradeDate))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", item.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Code))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Name))
		b.WriteString(`</td>`)
		writeSectorFundFlowNumberCell(b, item.Price, "%.2f")
		writeSectorFundFlowPctCell(b, item.ChangePct)
		writeSectorFundFlowMoneyCell(b, item.MainNetInflow)
		writeSectorFundFlowPctCell(b, item.MainNetInflowPct)
		writeSectorFundFlowMoneyCell(b, item.SuperLargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.LargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.MediumNetInflow)
		writeSectorFundFlowMoneyCell(b, item.SmallNetInflow)
		b.WriteString(`<td>`)
		b.WriteString(html.EscapeString(item.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div>`)
}

func renderSectorFundFlowStockTable(b *strings.Builder, ctx model.AStockSectorFundFlowListResult, stockCtx sectorFundFlowStockContext) {
	if strings.TrimSpace(stockCtx.SectorName) == "" {
		return
	}
	b.WriteString(`<section><div class="sector-detail-head"><div><h3>`)
	b.WriteString(html.EscapeString(stockCtx.SectorName))
	b.WriteString(` 个股资金流</h3><p class="sector-muted sector-stock-note">`)
	b.WriteString(html.EscapeString(fmt.Sprintf("成分股 %d 只，本地资金流命中 %d 只。", len(stockCtx.Constituents), stockCtx.Stocks.Total)))
	if stockCtx.ConstituentWarn != "" {
		b.WriteString(` `)
		b.WriteString(html.EscapeString(stockCtx.ConstituentWarn))
	}
	b.WriteString(`</p></div>`)
	backQuery := sectorFundFlowQuery(model.AStockSectorFundFlowFilter{Date: ctx.Date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword})
	b.WriteString(`<a class="sector-tab" href="/sector-fund-flow?`)
	b.WriteString(html.EscapeString(backQuery.Encode()))
	b.WriteString(`">关闭个股明细</a></div>`)
	if stockCtx.Error != "" {
		b.WriteString(`<div class="sector-empty">`)
		b.WriteString(html.EscapeString(stockCtx.Error))
		b.WriteString(`</div></section>`)
		return
	}
	b.WriteString(`<div class="sector-scroll">`)
	if len(stockCtx.Stocks.Items) == 0 {
		b.WriteString(`<div class="sector-empty">当前本地缓存没有匹配到该板块成分股资金流。请先刷新版块资金，或等待交易时段自动抓取个股资金流。</div></div></section>`)
		return
	}
	b.WriteString(`<table class="sector-table sector-stock-table"><tr><th>排名</th><th>代码</th><th>名称</th><th>最新价</th><th>涨跌幅</th><th>主力净流入</th><th>主力净占比</th><th>超大单</th><th>大单</th><th>中单</th><th>小单</th><th>超大单占比</th><th>大单占比</th><th>更新时间</th></tr>`)
	for _, item := range stockCtx.Stocks.Items {
		b.WriteString(`<tr><td>`)
		b.WriteString(fmt.Sprintf("%d", item.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Code))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(nonEmptyText(item.Name, "--")))
		b.WriteString(`</td>`)
		writeSectorFundFlowNumberCell(b, item.Price, "%.2f")
		writeSectorFundFlowPctCell(b, item.ChangePct)
		writeSectorFundFlowMoneyCell(b, item.MainNetInflow)
		writeSectorFundFlowPctCell(b, item.MainNetInflowPct)
		writeSectorFundFlowMoneyCell(b, item.SuperLargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.LargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.MediumNetInflow)
		writeSectorFundFlowMoneyCell(b, item.SmallNetInflow)
		writeSectorFundFlowPctCell(b, item.SuperLargeNetInflowPct)
		writeSectorFundFlowPctCell(b, item.LargeNetInflowPct)
		b.WriteString(`<td>`)
		b.WriteString(html.EscapeString(item.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></section>`)
}

func renderSectorFundFlowStockSearch(b *strings.Builder, ctx model.AStockSectorFundFlowListResult, stockCtx sectorFundFlowStockSearchContext) {
	if strings.TrimSpace(stockCtx.Keyword) == "" {
		return
	}
	if stockCtx.Error != "" {
		b.WriteString(`<section><h3>个股资金流搜索</h3><div class="sector-empty">`)
		b.WriteString(html.EscapeString(stockCtx.Error))
		b.WriteString(`</div></section>`)
		return
	}
	if len(stockCtx.Stocks.Items) == 0 {
		return
	}
	b.WriteString(`<section><div class="sector-detail-head"><div><h3>个股资金流搜索</h3><p class="sector-muted sector-stock-note">`)
	b.WriteString(html.EscapeString(fmt.Sprintf("关键词 %s，命中 %d 只。", stockCtx.Keyword, stockCtx.Stocks.Total)))
	b.WriteString(`</p></div></div><div class="sector-scroll">`)
	b.WriteString(`<table class="sector-table sector-stock-table"><tr><th>排名</th><th>代码</th><th>名称</th><th>趋势</th><th>最新价</th><th>涨跌幅</th><th>主力净流入</th><th>主力净占比</th><th>超大单</th><th>大单</th><th>中单</th><th>小单</th><th>更新时间</th></tr>`)
	for _, item := range stockCtx.Stocks.Items {
		trendQuery := sectorFundFlowQuery(model.AStockSectorFundFlowFilter{Date: ctx.Date, SectorType: ctx.SectorType, Indicator: ctx.Indicator, Keyword: ctx.Keyword})
		trendQuery.Set("trend", "stock")
		trendQuery.Set("code", item.Code)
		trendQuery.Set("trend_days", "5")
		b.WriteString(`<tr><td>`)
		b.WriteString(fmt.Sprintf("%d", item.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(item.Code))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(nonEmptyText(item.Name, "--")))
		b.WriteString(`</td><td><a class="sector-trend-link" href="/sector-fund-flow?`)
		b.WriteString(html.EscapeString(trendQuery.Encode()))
		b.WriteString(`">趋势</a></td>`)
		writeSectorFundFlowNumberCell(b, item.Price, "%.2f")
		writeSectorFundFlowPctCell(b, item.ChangePct)
		writeSectorFundFlowMoneyCell(b, item.MainNetInflow)
		writeSectorFundFlowPctCell(b, item.MainNetInflowPct)
		writeSectorFundFlowMoneyCell(b, item.SuperLargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.LargeNetInflow)
		writeSectorFundFlowMoneyCell(b, item.MediumNetInflow)
		writeSectorFundFlowMoneyCell(b, item.SmallNetInflow)
		b.WriteString(`<td>`)
		b.WriteString(html.EscapeString(item.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div>`)
	if len(stockCtx.Trend.Items) > 0 {
		nestedTrend := sectorFundFlowTrendContext{
			Mode:       "stock",
			Title:      firstStockFundFlowTitle(stockCtx.Trend.Items),
			Days:       5,
			StockCode:  stockCtx.Trend.Code,
			StockName:  firstStockFundFlowName(stockCtx.Trend.Items),
			StockTrend: stockCtx.Trend,
		}
		b.WriteString(`<h3>`)
		b.WriteString(html.EscapeString(firstStockFundFlowTitle(stockCtx.Trend.Items)))
		b.WriteString(`</h3>`)
		renderSectorFundFlowStockTrendTable(b, nestedTrend)
	}
	b.WriteString(`</section>`)
}

func firstStockFundFlowName(items []model.AStockStockFundFlow) string {
	for _, item := range items {
		if strings.TrimSpace(item.Name) != "" {
			return strings.TrimSpace(item.Name)
		}
	}
	return ""
}

func firstStockFundFlowTitle(items []model.AStockStockFundFlow) string {
	if len(items) == 0 {
		return "个股主力资金趋势"
	}
	title := strings.TrimSpace(items[0].Code + " " + items[0].Name)
	return nonEmptyText(title, "个股主力资金趋势")
}

func normalizeSectorFundFlowTrendDays(value string) int {
	switch strings.TrimSpace(value) {
	case "10":
		return 10
	case "30":
		return 30
	default:
		return 5
	}
}

func writeSectorFundFlowMoneyCell(b *strings.Builder, value float64) {
	b.WriteString(`<td class="sector-num `)
	b.WriteString(sectorFundFlowValueClass(value))
	b.WriteString(`">`)
	b.WriteString(formatSectorFundFlowMoney(value))
	b.WriteString(`</td>`)
}

func writeSectorFundFlowPctCell(b *strings.Builder, value float64) {
	b.WriteString(`<td class="sector-num `)
	b.WriteString(sectorFundFlowValueClass(value))
	b.WriteString(`">`)
	b.WriteString(formatSectorFundFlowPct(value))
	b.WriteString(`</td>`)
}

func writeSectorFundFlowNumberCell(b *strings.Builder, value float64, format string) {
	b.WriteString(`<td class="sector-num">`)
	if math.IsNaN(value) || math.IsInf(value, 0) || value == 0 {
		b.WriteString(`--`)
	} else {
		b.WriteString(html.EscapeString(fmt.Sprintf(format, value)))
	}
	b.WriteString(`</td>`)
}

func formatSectorFundFlowMoney(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "--"
	}
	abs := math.Abs(value)
	switch {
	case abs >= 100000000:
		return fmt.Sprintf("%+.2f亿", value/100000000)
	case abs >= 10000:
		return fmt.Sprintf("%+.2f万", value/10000)
	default:
		return fmt.Sprintf("%+.0f", value)
	}
}

func formatSectorFundFlowPct(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "--"
	}
	return fmt.Sprintf("%+.2f%%", value)
}

func sectorFundFlowValueClass(value float64) string {
	if value > 0 {
		return "sector-positive"
	}
	if value < 0 {
		return "sector-negative"
	}
	return ""
}

func normalizeSectorFundFlowSectorType(value string) string {
	switch strings.TrimSpace(value) {
	case "概念", "概念资金", "概念资金流":
		return "概念资金流"
	default:
		return "行业资金流"
	}
}

func normalizeSectorFundFlowIndicator(value string) string {
	switch strings.TrimSpace(value) {
	case "5", "5日":
		return "5日"
	case "10", "10日":
		return "10日"
	default:
		return "今日"
	}
}

func (s *Server) triggerSectorFundFlowRefresh() string {
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/sector-fund-flow/latest")
	if err != nil {
		return "版块资金刷新失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "版块资金刷新失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	var envelope struct {
		Data struct {
			Result struct {
				Date   string `json:"date"`
				Groups int    `json:"groups"`
				Items  int    `json:"items"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err == nil && envelope.Data.Result.Date != "" {
		return fmt.Sprintf("版块资金已刷新：%s，%d 组，%d 条。", envelope.Data.Result.Date, envelope.Data.Result.Groups, envelope.Data.Result.Items)
	}
	return "版块资金刷新已完成，请查看当前列表。"
}
