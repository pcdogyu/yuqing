package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type aStockHoldingsContext struct {
	model.StockInstitutionHoldingListResult
	Summary    model.StockInstitutionHoldingSummary
	Signals    model.StockInstitutionHoldingSignalListResult
	Reports    model.StockHoldingReportDocumentListResult
	SignalType string
}

func (s *Server) handleAStockHoldingsPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleAStockHoldingsAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := aStockHoldingFilterFromRequest(r)
	ctx, err := s.loadAStockHoldingsContext(filter)
	var b strings.Builder
	b.WriteString(`<style>
body[data-page='a-stock-holdings'] main,body[data-page='a-stock-holdings'] .site-footer{max-width:none;width:98%;box-sizing:border-box}
.holding-muted{color:#6a6257}.holding-message{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}
.holding-toolbar{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;align-items:end}
.holding-toolbar button{margin:0}.holding-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:end}.holding-actions form{margin:0}.holding-actions button{margin:0}
.holding-danger{background:#8f6a20}.holding-scroll{overflow:auto}
.holding-table{min-width:1280px}.holding-signal-table{min-width:1180px}.holding-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
.holding-table,.holding-signal-table{font-size:12px;line-height:1.25}.holding-table th,.holding-table td,.holding-signal-table th,.holding-signal-table td{white-space:nowrap;padding:7px 10px}
.holding-tab{display:inline-flex;align-items:center;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
.holding-tab-active{background:#214e34;color:#fff;border-color:#214e34}
.holding-source{font-size:12px;padding:3px 8px;border-radius:999px;background:#eff6f0;color:#214e34}
.holding-change-up{color:#c5221f;font-weight:700}.holding-change-down{color:#087a3d;font-weight:700}
.holding-level{font-size:12px;padding:3px 8px;border-radius:999px;background:#f6e9c6;color:#6a4b00}.holding-level-high{background:#f9d8d2;color:#7a2618}
.holding-summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}
.holding-card{padding:14px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2}.holding-card strong{display:block;font-size:22px;margin-top:6px}
</style>`)
	b.WriteString(`<section><h2>机构持仓</h2><p class="holding-muted">按报告期查看股票被基金、社保、QFII、券商、保险、信托等共同持有的情况。数据来自 AKShare 适配服务返回的东方财富/新浪季度持仓数据，不代表实时持仓。</p></section>`)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<div class="holding-message">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	if err != nil {
		b.WriteString(`<section><p>机构持仓数据读取失败：`)
		b.WriteString(html.EscapeString(err.Error()))
		b.WriteString(`</p></section>`)
		_ = s.writeSimplePage(w, "a-stock-holdings", "机构持仓", b.String())
		return
	}
	renderAStockHoldingSignals(&b, ctx.Signals, ctx.StockInstitutionHoldingListResult, ctx.SignalType)
	renderAStockHoldingSummary(&b, ctx.Summary)
	renderAStockHoldingFilters(&b, ctx.StockInstitutionHoldingListResult)
	renderAStockHoldingTable(&b, ctx.StockInstitutionHoldingListResult)
	renderAStockHoldingReports(&b, ctx.Reports)
	_ = s.writeSimplePage(w, "a-stock-holdings", "机构持仓", b.String())
	_ = user
}

func aStockHoldingFilterFromRequest(r *http.Request) model.StockInstitutionHoldingFilter {
	return model.StockInstitutionHoldingFilter{
		Code:        strings.TrimSpace(r.URL.Query().Get("code")),
		Company:     strings.TrimSpace(r.URL.Query().Get("company")),
		Period:      strings.TrimSpace(r.URL.Query().Get("period")),
		Holder:      strings.TrimSpace(r.URL.Query().Get("holder")),
		HolderType:  strings.TrimSpace(r.URL.Query().Get("holder_type")),
		FundCompany: strings.TrimSpace(r.URL.Query().Get("fund_company")),
		Source:      strings.TrimSpace(r.URL.Query().Get("source")),
		SignalType:  strings.TrimSpace(r.URL.Query().Get("signal_type")),
		Page:        normalizeAStockNewsPage(r.URL.Query().Get("page")),
		PageSize:    50,
	}
}

func (s *Server) loadAStockHoldingsContext(filter model.StockInstitutionHoldingFilter) (aStockHoldingsContext, error) {
	query := aStockHoldingQuery(filter)
	var list model.StockInstitutionHoldingListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/holdings?"+query.Encode(), &list); err != nil {
		return aStockHoldingsContext{}, err
	}
	signalType := normalizeAStockHoldingSignalType(filter.SignalType)
	var signals model.StockInstitutionHoldingSignalListResult
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/holdings/signals?"+aStockHoldingSignalQuery(filter, signalType).Encode(), &signals)
	summaryQuery := url.Values{}
	if filter.Code != "" {
		summaryQuery.Set("code", filter.Code)
	}
	if filter.Period != "" {
		summaryQuery.Set("period", filter.Period)
	}
	var summary model.StockInstitutionHoldingSummary
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/holdings/summary?"+summaryQuery.Encode(), &summary)
	var reports model.StockHoldingReportDocumentListResult
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/holding-reports?"+aStockHoldingReportQuery(filter).Encode(), &reports)
	return aStockHoldingsContext{StockInstitutionHoldingListResult: list, Summary: summary, Signals: signals, Reports: reports, SignalType: signalType}, nil
}

func aStockHoldingQuery(filter model.StockInstitutionHoldingFilter) url.Values {
	query := url.Values{}
	query.Set("page", fmt.Sprintf("%d", max(filter.Page, 1)))
	query.Set("page_size", fmt.Sprintf("%d", max(filter.PageSize, 50)))
	if filter.Code != "" {
		query.Set("code", filter.Code)
	}
	if filter.Company != "" {
		query.Set("company", filter.Company)
	}
	if filter.Period != "" {
		query.Set("period", filter.Period)
	}
	if filter.Holder != "" {
		query.Set("holder", filter.Holder)
	}
	if filter.HolderType != "" {
		query.Set("holder_type", filter.HolderType)
	}
	if filter.FundCompany != "" {
		query.Set("fund_company", filter.FundCompany)
	}
	if filter.Source != "" {
		query.Set("source", filter.Source)
	}
	if filter.SignalType != "" {
		query.Set("signal_type", filter.SignalType)
	}
	return query
}

func aStockHoldingSignalQuery(filter model.StockInstitutionHoldingFilter, signalType string) url.Values {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("page_size", "20")
	if filter.Code != "" {
		query.Set("code", filter.Code)
	}
	if filter.Company != "" {
		query.Set("company", filter.Company)
	}
	if filter.Period != "" {
		query.Set("period", filter.Period)
	}
	if filter.FundCompany != "" {
		query.Set("fund_company", filter.FundCompany)
	}
	if signalType != "" {
		query.Set("signal_type", signalType)
	}
	return query
}

func aStockHoldingReportQuery(filter model.StockInstitutionHoldingFilter) url.Values {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("page_size", "10")
	if filter.Period != "" {
		query.Set("period", filter.Period)
	}
	if filter.FundCompany != "" {
		query.Set("fund_company", filter.FundCompany)
	}
	return query
}

func renderAStockHoldingSignals(b *strings.Builder, signals model.StockInstitutionHoldingSignalListResult, filter model.StockInstitutionHoldingListResult, signalType string) {
	b.WriteString(`<section><h2>机构持仓异动</h2><p class="holding-muted">按当前报告期与上一可用报告期对比，识别新进、退出披露名单、增持和减持。退出披露名单表示上期披露、本期未披露，不等同于确认清仓。</p>`)
	if signals.CurrentPeriod != "" || signals.PreviousPeriod != "" {
		b.WriteString(`<p class="holding-muted">对比区间：`)
		b.WriteString(html.EscapeString(nonEmptyText(formatAStockHoldingPeriod(signals.PreviousPeriod), "--")))
		b.WriteString(` -> `)
		b.WriteString(html.EscapeString(nonEmptyText(formatAStockHoldingPeriod(signals.CurrentPeriod), "--")))
		b.WriteString(`；阈值：机构数 +`)
		b.WriteString(fmt.Sprintf("%d", signals.Thresholds.HolderCountChange))
		b.WriteString(` / 基金数 +`)
		b.WriteString(fmt.Sprintf("%d", signals.Thresholds.FundCountChange))
		b.WriteString(` / 流通占比 +`)
		b.WriteString(html.EscapeString(formatAStockHoldingPct(signals.Thresholds.FloatRatioChange)))
		b.WriteString(`；增减按持股数、市值或流通占比方向筛选`)
		b.WriteString(`</p>`)
	}
	renderAStockHoldingSignalTabs(b, filter, signalType)
	b.WriteString(`<div class="holding-scroll"><table class="holding-signal-table"><tr><th>类型</th><th>等级</th><th>股票</th><th>报告期对比</th><th>机构新进/退出</th><th>基金新进/退出</th><th>基金公司变化</th><th>流通占比变化</th><th>持股数变化</th><th>市值变化</th><th>代表新进</th><th>代表退出披露</th><th>原因</th></tr>`)
	if len(signals.Items) == 0 {
		b.WriteString(`<tr><td colspan="13">暂无机构持仓异动结果：需要至少两个报告期的数据，且变化达到监控阈值。</td></tr>`)
	} else {
		for _, item := range signals.Items {
			levelClass := "holding-level"
			if item.Level == "high" {
				levelClass += " holding-level-high"
			}
			b.WriteString(`<tr><td>`)
			b.WriteString(html.EscapeString(stockHoldingSignalTypeLabel(item.SignalType)))
			b.WriteString(`</td><td><span class="`)
			b.WriteString(levelClass)
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(stockHoldingSignalLevelLabel(item.Level)))
			b.WriteString(`</span></td><td>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(item.StockCode + " " + item.StockName)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingPeriod(item.PreviousPeriod)))
			b.WriteString(` -> `)
			b.WriteString(html.EscapeString(formatAStockHoldingPeriod(item.CurrentPeriod)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(fmt.Sprintf("+%d / -%d", item.NewHolderCount, item.ExitedHolderCount)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(fmt.Sprintf("+%d / -%d", item.NewFundCount, item.ExitedFundCount)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(fmt.Sprintf("%s（+%d / -%d）", formatAStockHoldingSignedInt(item.FundCompanyCountChange), item.NewFundCompanyCount, item.ExitedFundCompanyCount)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingSignedPct(item.FloatRatioChange)))
			b.WriteString(`</td><td>`)
			writeAStockHoldingSignedValue(b, item.SharesChange, formatAStockHoldingSignedNumber(item.SharesChange))
			b.WriteString(`</td><td>`)
			writeAStockHoldingSignedValue(b, item.MarketValueChange, formatAStockHoldingSignedMoney(item.MarketValueChange))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(strings.Join(item.NewMajorHolders, "、"), "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(strings.Join(item.ExitedMajorHolders, "、"), "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Reason, "--")))
			b.WriteString(`</td></tr>`)
		}
	}
	b.WriteString(`</table></div></section>`)
}

func renderAStockHoldingSignalTabs(b *strings.Builder, filter model.StockInstitutionHoldingListResult, current string) {
	b.WriteString(`<div class="holding-tabs">`)
	for _, tab := range []struct {
		Type  string
		Label string
	}{{"", "全部"}, {"new_entry", "新进"}, {"exit_disclosure", "退出披露"}, {"increase", "增持"}, {"decrease", "减持"}} {
		queryFilter := model.StockInstitutionHoldingFilter{Code: filter.Code, Company: filter.Company, Period: filter.Period, Holder: filter.Holder, HolderType: filter.HolderType, FundCompany: filter.FundCompany, Source: filter.Source, SignalType: tab.Type, Page: 1, PageSize: filter.PageSize}
		className := "holding-tab"
		if current == tab.Type {
			className += " holding-tab-active"
		}
		b.WriteString(`<a class="`)
		b.WriteString(className)
		b.WriteString(`" href="/a-stock/holdings?`)
		b.WriteString(html.EscapeString(aStockHoldingQuery(queryFilter).Encode()))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(tab.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func renderAStockHoldingSummary(b *strings.Builder, summary model.StockInstitutionHoldingSummary) {
	b.WriteString(`<section><h2>共持摘要</h2><div class="holding-summary">`)
	writeAStockHoldingMetric(b, "报告期", nonEmptyText(formatAStockHoldingPeriod(summary.ReportPeriod), "--"))
	writeAStockHoldingMetric(b, "持有人总数", fmt.Sprintf("%d", summary.HolderCount))
	writeAStockHoldingMetric(b, "基金数", fmt.Sprintf("%d", summary.FundCount))
	writeAStockHoldingMetric(b, "基金公司数", fmt.Sprintf("%d", summary.FundCompanyCount))
	writeAStockHoldingMetric(b, "机构类型数", fmt.Sprintf("%d", summary.HolderTypeCount))
	writeAStockHoldingMetric(b, "合计持股数", formatAStockHoldingNumber(summary.TotalShares))
	writeAStockHoldingMetric(b, "合计流通占比", formatAStockHoldingPct(summary.TotalFloatRatio))
	writeAStockHoldingMetric(b, "最大持有人", nonEmptyText(summary.MaxHolderName, "--"))
	b.WriteString(`</div></section>`)
}

func writeAStockHoldingMetric(b *strings.Builder, label string, value string) {
	b.WriteString(`<div class="holding-card">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`<strong>`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func renderAStockHoldingFilters(b *strings.Builder, ctx model.StockInstitutionHoldingListResult) {
	b.WriteString(`<section><h2>筛选</h2><form method="get" class="holding-toolbar"><div><label>股票代码</label><input name="code" placeholder="002230" value="`)
	b.WriteString(html.EscapeString(ctx.Code))
	b.WriteString(`"></div><div><label>股票名称</label><input name="company" placeholder="科大讯飞" value="`)
	b.WriteString(html.EscapeString(ctx.Company))
	b.WriteString(`"></div><div><label>报告期</label><select name="period"><option value="">全部/最新</option>`)
	for _, period := range ctx.Periods {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(period))
		b.WriteString(`"`)
		if period == ctx.Period {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(formatAStockHoldingPeriod(period)))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></div><div><label>持有人类型</label><select name="holder_type"><option value="">全部</option>`)
	for _, holderType := range ctx.HolderTypes {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(holderType))
		b.WriteString(`"`)
		if holderType == ctx.HolderType {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(stockHoldingHolderTypeLabel(holderType)))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></div><div><label>基金公司</label><input name="fund_company" placeholder="易方达 / 华夏" value="`)
	b.WriteString(html.EscapeString(ctx.FundCompany))
	b.WriteString(`"></div><div><label>持有人</label><input name="holder" placeholder="社保 / 基金 / QFII" value="`)
	b.WriteString(html.EscapeString(ctx.Holder))
	b.WriteString(`"></div><div><label>来源</label><select name="source"><option value="">全部</option>`)
	for _, source := range ctx.Sources {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(source))
		b.WriteString(`"`)
		if source == ctx.Source {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(stockHoldingSourceLabel(source)))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></div><div class="holding-actions"><button type="submit">查询</button></div></form><div class="holding-actions"><form method="post"><input type="hidden" name="action" value="backfill">`)
	for key, value := range map[string]string{"code": ctx.Code, "company": ctx.Company, "period": ctx.Period, "holder": ctx.Holder, "holder_type": ctx.HolderType, "fund_company": ctx.FundCompany, "source": ctx.Source} {
		b.WriteString(`<input type="hidden" name="`)
		b.WriteString(key)
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(value))
		b.WriteString(`">`)
	}
	b.WriteString(`<button type="submit">回补当前筛选持仓</button></form><form method="post"><input type="hidden" name="action" value="backfill_all"><button class="holding-danger" type="submit">回补近一年全市场</button></form></div><p class="holding-muted">近一年全市场回补会调用 scheduler 的机构持仓回补接口，不带股票代码时默认抓取最近 4 个可用报告期，用于跨季度对比。</p></section>`)
}

func renderAStockHoldingTable(b *strings.Builder, ctx model.StockInstitutionHoldingListResult) {
	b.WriteString(`<section><h2>持仓明细</h2><div class="holding-scroll"><table class="holding-table"><tr><th>报告期</th><th>股票</th><th>持有人</th><th>类型</th><th>基金公司</th><th>披露口径</th><th>排名</th><th>持股数</th><th>变化</th><th>变化比例</th><th>流通占比</th><th>持股市值</th><th>公告日</th></tr>`)
	if len(ctx.Items) == 0 {
		b.WriteString(`<tr><td colspan="13">暂无机构持仓数据，请点击“回补近一年全市场”或配置定时抓取任务。</td></tr>`)
	} else {
		for _, item := range ctx.Items {
			b.WriteString(`<tr><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingPeriod(item.ReportPeriod)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(item.StockCode + " " + item.StockName)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(item.HolderName))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(stockHoldingHolderTypeLabel(item.HolderType)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.FundCompany, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(stockHoldingDisclosureScopeLabel(item.DisclosureScope)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.HolderRank, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingNumber(item.Shares)))
			b.WriteString(`</td><td>`)
			writeAStockHoldingSignedValue(b, item.SharesChange, formatAStockHoldingSignedNumber(item.SharesChange))
			b.WriteString(`</td><td>`)
			writeAStockHoldingSignedValue(b, item.ChangeRatio, formatAStockHoldingSignedPct(item.ChangeRatio))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingPct(item.FloatRatio)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingMoney(item.MarketValue)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.AnnounceDate, "--")))
			b.WriteString(`</td></tr>`)
		}
	}
	b.WriteString(`</table></div>`)
	renderAStockHoldingPagination(b, ctx)
	b.WriteString(`</section>`)
}

func renderAStockHoldingPagination(b *strings.Builder, ctx model.StockInstitutionHoldingListResult) {
	if ctx.PageSize <= 0 || ctx.Total <= ctx.PageSize {
		return
	}
	totalPages := (ctx.Total + ctx.PageSize - 1) / ctx.PageSize
	b.WriteString(`<div class="holding-tabs"><span class="holding-muted">第 `)
	b.WriteString(fmt.Sprintf("%d/%d 页，共 %d 条", ctx.Page, totalPages, ctx.Total))
	b.WriteString(`</span>`)
	for _, link := range []struct {
		Page  int
		Label string
	}{{ctx.Page - 1, "上一页"}, {ctx.Page + 1, "下一页"}} {
		if link.Page < 1 || link.Page > totalPages {
			continue
		}
		filter := model.StockInstitutionHoldingFilter{Code: ctx.Code, Company: ctx.Company, Period: ctx.Period, Holder: ctx.Holder, HolderType: ctx.HolderType, FundCompany: ctx.FundCompany, Source: ctx.Source, Page: link.Page, PageSize: ctx.PageSize}
		b.WriteString(`<a class="holding-tab" href="/a-stock/holdings?`)
		b.WriteString(html.EscapeString(aStockHoldingQuery(filter).Encode()))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(link.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func renderAStockHoldingReports(b *strings.Builder, reports model.StockHoldingReportDocumentListResult) {
	b.WriteString(`<section><h2>基金季报索引</h2><p class="holding-muted">保存公开定期报告的公告元数据和来源链接，用于回溯结构化持仓结果；v1 不强制解析 PDF 全文。</p><div class="holding-scroll"><table class="holding-table"><tr><th>报告期</th><th>基金</th><th>基金公司</th><th>公告标题</th><th>公告日</th><th>解析状态</th></tr>`)
	if len(reports.Items) == 0 {
		b.WriteString(`<tr><td colspan="6">暂无基金季报索引；持仓回补时若适配服务返回报告元数据会自动写入。</td></tr>`)
	} else {
		for _, item := range reports.Items {
			b.WriteString(`<tr><td>`)
			b.WriteString(html.EscapeString(formatAStockHoldingPeriod(item.ReportPeriod)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(nonEmptyText(item.FundCode, "") + " " + nonEmptyText(item.FundName, ""))))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.FundCompany, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.AnnouncementTitle, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.AnnouncementDate, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(stockHoldingReportParseStatusLabel(item.ParseStatus)))
			b.WriteString(`</td></tr>`)
		}
	}
	b.WriteString(`</table></div></section>`)
}

func (s *Server) handleAStockHoldingsAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := model.StockInstitutionHoldingFilter{
		Code:        strings.TrimSpace(r.FormValue("code")),
		Company:     strings.TrimSpace(r.FormValue("company")),
		Period:      strings.TrimSpace(r.FormValue("period")),
		Holder:      strings.TrimSpace(r.FormValue("holder")),
		HolderType:  strings.TrimSpace(r.FormValue("holder_type")),
		FundCompany: strings.TrimSpace(r.FormValue("fund_company")),
		Source:      strings.TrimSpace(r.FormValue("source")),
		Page:        1,
		PageSize:    50,
	}
	action := strings.TrimSpace(r.FormValue("action"))
	message := "未知操作"
	if action == "backfill" {
		message = s.triggerAStockHoldingsBackfill(filter)
	} else if action == "backfill_all" {
		filter = model.StockInstitutionHoldingFilter{Page: 1, PageSize: 50}
		message = s.triggerAStockHoldingsBackfill(filter)
	}
	query := aStockHoldingQuery(filter)
	query.Set("msg", message)
	http.Redirect(w, r, "/a-stock/holdings?"+query.Encode(), http.StatusSeeOther)
}

func (s *Server) triggerAStockHoldingsBackfill(filter model.StockInstitutionHoldingFilter) string {
	query := url.Values{}
	if filter.Code != "" {
		query.Set("code", filter.Code)
	}
	if filter.Period != "" {
		query.Set("period", filter.Period)
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/holdings/backfill?" + query.Encode())
	if err != nil {
		return "机构持仓回补失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "机构持仓回补失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	if filter.Code == "" && filter.Period == "" {
		return "近一年全市场机构持仓回补任务已触发，请稍后刷新查看。"
	}
	return "机构持仓回补任务已触发，请稍后刷新查看。"
}

func stockHoldingHolderTypeLabel(value string) string {
	switch value {
	case "fund":
		return "基金"
	case "social_security":
		return "社保"
	case "qfii":
		return "QFII"
	case "broker":
		return "券商"
	case "insurance":
		return "保险"
	case "trust":
		return "信托"
	case "bank_wealth":
		return "银行理财"
	case "natural_person":
		return "自然人"
	case "institution":
		return "机构"
	case "other":
		return "其他"
	default:
		return nonEmptyText(value, "--")
	}
}

func stockHoldingSourceLabel(source string) string {
	switch source {
	case "akshare_stock_holding":
		return "AKShare 持仓"
	case "stock_gdfx_free_holding_detail_em":
		return "东财十大流通股东"
	case "stock_gdfx_holding_detail_em":
		return "东财十大股东"
	case "stock_institute_hold_detail":
		return "新浪机构持股"
	case "stock_fund_stock_holder":
		return "新浪基金持股"
	case "tushare_fund_portfolio":
		return "TuShare 基金持仓"
	case "tiantian_fund_regular_report":
		return "天天基金定期报告"
	case "fund_quarterly_report":
		return "基金季报"
	default:
		return nonEmptyText(source, "--")
	}
}

func stockHoldingSourceLabels(sources []string) string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		label := stockHoldingSourceLabel(source)
		if label != "--" {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, "、")
}

func writeAStockHoldingSourceCell(b *strings.Builder, sourceType string, sourceURL string) {
	label := stockHoldingSourceLabel(sourceType)
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		b.WriteString(`<span class="holding-source">`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</span>`)
		return
	}
	b.WriteString(`<a class="holding-source" target="_blank" rel="noopener noreferrer" href="`)
	b.WriteString(html.EscapeString(sourceURL))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</a>`)
}

func writeAStockHoldingSignedValue(b *strings.Builder, value float64, text string) {
	className := ""
	switch {
	case value > 0:
		className = "holding-change-up"
	case value < 0:
		className = "holding-change-down"
	}
	if className == "" {
		b.WriteString(html.EscapeString(text))
		return
	}
	b.WriteString(`<span class="`)
	b.WriteString(className)
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(text))
	b.WriteString(`</span>`)
}

func stockHoldingSignalLevelLabel(value string) string {
	switch value {
	case "high":
		return "强异动"
	case "medium":
		return "异动"
	default:
		return nonEmptyText(value, "--")
	}
}

func stockHoldingSignalTypeLabel(value string) string {
	switch value {
	case "new_entry":
		return "新进"
	case "exit_disclosure":
		return "退出披露"
	case "increase":
		return "增持"
	case "decrease":
		return "减持"
	default:
		return nonEmptyText(value, "--")
	}
}

func normalizeAStockHoldingSignalType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "new_entry", "new", "in":
		return "new_entry"
	case "exit_disclosure", "exit", "out":
		return "exit_disclosure"
	case "increase", "add":
		return "increase"
	case "decrease", "reduce":
		return "decrease"
	default:
		return ""
	}
}

func stockHoldingDisclosureScopeLabel(value string) string {
	switch value {
	case "top10":
		return "十大股东"
	case "free_float_top10":
		return "十大流通股东"
	case "fund_quarterly":
		return "基金季报"
	case "full":
		return "全量"
	case "quarter_disclosure":
		return "季度披露"
	default:
		return nonEmptyText(value, "--")
	}
}

func stockHoldingReportParseStatusLabel(value string) string {
	switch value {
	case "indexed":
		return "已索引"
	case "pending":
		return "待解析"
	case "parsed":
		return "已解析"
	case "failed":
		return "失败"
	case "no_pdf":
		return "无PDF"
	default:
		return nonEmptyText(value, "--")
	}
}

func formatAStockHoldingPeriod(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return value
	}
	year := value[:4]
	switch value[4:] {
	case "0331":
		return year + "-Q1"
	case "0630":
		return year + "-Q2"
	case "0930":
		return year + "-Q3"
	case "1231":
		return year + "-Q4"
	default:
		return value
	}
}

func formatAStockHoldingNumber(value float64) string {
	if value == 0 {
		return "--"
	}
	return fmt.Sprintf("%.0f", value)
}

func formatAStockHoldingSignedInt(value int) string {
	if value > 0 {
		return fmt.Sprintf("+%d", value)
	}
	return fmt.Sprintf("%d", value)
}

func formatAStockHoldingSignedNumber(value float64) string {
	if value == 0 {
		return "--"
	}
	if value > 0 {
		return "+" + formatAStockHoldingNumber(value)
	}
	return "-" + formatAStockHoldingNumber(-value)
}

func formatAStockHoldingMoney(value float64) string {
	if value == 0 {
		return "--"
	}
	if value >= 100000000 {
		return fmt.Sprintf("%.2f亿", value/100000000)
	}
	if value >= 10000 {
		return fmt.Sprintf("%.2f万", value/10000)
	}
	return fmt.Sprintf("%.0f", value)
}

func formatAStockHoldingSignedMoney(value float64) string {
	if value == 0 {
		return "--"
	}
	if value > 0 {
		return "+" + formatAStockHoldingMoney(value)
	}
	return "-" + formatAStockHoldingMoney(-value)
}

func formatAStockHoldingPct(value float64) string {
	if value == 0 {
		return "--"
	}
	return fmt.Sprintf("%.2f%%", value)
}

func formatAStockHoldingSignedPct(value float64) string {
	if value == 0 {
		return "--"
	}
	if value > 0 {
		return "+" + formatAStockHoldingPct(value)
	}
	return "-" + formatAStockHoldingPct(-value)
}

func decodeAStockHoldingSummary(body []byte) (model.StockInstitutionHoldingSummary, error) {
	var envelope struct {
		Data model.StockInstitutionHoldingSummary `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return model.StockInstitutionHoldingSummary{}, err
	}
	return envelope.Data, nil
}
