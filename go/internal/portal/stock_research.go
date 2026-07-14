package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/stockresearch"
)

const stockResearchPageSize = 20
const investorRelationsSourceType = "cninfo_investor_relation"
const investorRelationsScope = "investor_relations"

func (s *Server) handleStockResearchPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if strings.TrimSpace(r.FormValue("scope")) == investorRelationsScope {
			s.handleInvestorRelationsAction(w, r)
		} else {
			s.handleStockResearchAction(w, r)
		}
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := stockResearchFilterFromRequest(r)
	ctx, err := s.loadStockResearchContext(filter)
	syncStockResearchContextFilter(&ctx, filter)
	var b strings.Builder
	renderStockResearchStyle(&b)
	b.WriteString(`<section><h2>研报调研</h2><p class="research-muted">搜索上市公司研报和机构调研记录，支持文本型 PDF 下载与解析；扫描版 PDF 暂标记为无文本。</p></section>`)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<div class="research-message">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	if err != nil {
		b.WriteString(`<section><p>研报调研数据读取失败：`)
		b.WriteString(html.EscapeString(err.Error()))
		b.WriteString(`</p></section>`)
	} else {
		renderStockResearchFilters(&b, ctx)
		renderStockResearchTable(&b, ctx)
	}
	_ = s.writeSimplePage(w, "stock-research", "研报调研", b.String())
	_ = user
}

func renderStockResearchStyle(b *strings.Builder) {
	b.WriteString(`<style>
body[data-page='stock-research'] main,body[data-page='stock-research'] .site-footer{max-width:none;width:100%;box-sizing:border-box}
body[data-page='stock-research'] main{font-size:14px;line-height:1.45}
body[data-page='stock-research'] section{width:100%;box-sizing:border-box}
body[data-page='stock-research'] table{width:100%;min-width:100%}
body[data-page='stock-research'] input,body[data-page='stock-research'] select,body[data-page='stock-research'] textarea,body[data-page='stock-research'] button{font-size:14px}
.research-muted{color:#6a6257}.research-message{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}
.research-toolbar{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;align-items:end}
.research-toolbar button{margin:0}.research-actions{display:flex;gap:10px;flex-wrap:wrap}.research-scroll{width:100%;overflow:auto}
.research-table{width:100%;min-width:0;table-layout:fixed}.research-table th,.research-table td{vertical-align:top}
.research-col-date{width:7.5%}.research-col-stock{width:8.5%}.research-col-title{width:22.8%}.research-col-institution{width:12.8%}.research-col-analyst{width:10.8%}.research-col-source{width:6%}.research-col-link{width:5%}.research-col-pdf{width:8.4%}.research-col-status{width:18.2%}
.research-table th:nth-child(1),.research-table td:nth-child(1),.research-table th:nth-child(2),.research-table td:nth-child(2),.research-table th:nth-child(6),.research-table td:nth-child(6),.research-table th:nth-child(7),.research-table td:nth-child(7),.research-table th:nth-child(8),.research-table td:nth-child(8){white-space:nowrap}
.research-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
.research-tab{display:inline-flex;align-items:center;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
.research-source{font-size:12px;padding:3px 8px;border-radius:999px;background:#eff6f0;color:#214e34}
.research-action-form{display:inline}.research-table form{display:inline}.research-status{font-size:12px;padding:3px 8px;border-radius:999px;background:#f4efe6;color:#5b4a32;white-space:nowrap}
.research-status-actions{display:flex;align-items:center;gap:8px;white-space:nowrap;flex-wrap:wrap}.research-status-actions .research-action-form{flex:1 1 auto;min-width:0}.research-status-actions button{width:90%;height:90%;min-height:32px;margin:0;padding:7px 10px}
.research-status.parsed{background:#e7f4ea;color:#214e34}.research-status.failed{background:#fdecea;color:#8a1f11}.research-status.no_text,.research-status.no_pdf,.research-status.pending_pdf{background:#fff7df;color:#695000}
.research-text-actions{display:flex;gap:12px;flex-wrap:wrap;margin:12px 0}.research-text-meta{color:#6a6257;margin:8px 0 14px}
.research-source-text{white-space:pre-wrap;line-height:1.72;background:#fff;border:1px solid #ece7dc;border-radius:12px;padding:18px;overflow:auto}
</style>`)
}

func (s *Server) handleInvestorRelationsPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleInvestorRelationsAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, investorRelationsRedirectURLFromRequest(r), http.StatusFound)
	_ = user
}

func (s *Server) handleStockResearchAsset(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, action, ok := parseStockResearchAssetPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "detail":
		s.renderStockResearchDetail(w, r, id)
	case "pdf":
		s.proxyStockResearchPDF(w, r, id)
	case "text":
		s.renderStockResearchPDFText(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func parseStockResearchAssetPath(path string) (int64, string, bool) {
	path = strings.TrimSpace(path)
	for _, prefix := range []string{"/stock-research/", "/api/v1/stock-research/"} {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(path, prefix), "/"), "/")
		if len(parts) == 1 {
			id, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil || id <= 0 {
				return 0, "", false
			}
			return id, "detail", true
		}
		if len(parts) < 2 || len(parts) > 3 || parts[1] != "pdf" {
			return 0, "", false
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 {
			return 0, "", false
		}
		if len(parts) == 2 {
			return id, "pdf", true
		}
		if parts[2] == "text" {
			return id, "text", true
		}
		return 0, "", false
	}
	return 0, "", false
}

func (s *Server) proxyStockResearchPDF(w http.ResponseWriter, r *http.Request, id int64) {
	contentURL := strings.TrimRight(strings.TrimSpace(s.cfg.ContentURL), "/")
	if contentURL == "" {
		http.Error(w, "content service is not configured", http.StatusServiceUnavailable)
		return
	}
	resp, err := s.client.R().
		SetContext(r.Context()).
		Get(fmt.Sprintf("%s/api/v1/stock-research/%d/pdf", contentURL, id))
	if err != nil {
		http.Error(w, "PDF 下载失败："+err.Error(), http.StatusBadGateway)
		return
	}
	if !resp.IsSuccess() {
		http.Error(w, "PDF 下载失败："+stockResearchSchedulerError(resp.Body(), resp.String()), resp.StatusCode())
		return
	}
	contentType := strings.TrimSpace(resp.Header().Get("Content-Type"))
	if contentType == "" {
		contentType = "application/pdf"
	}
	w.Header().Set("Content-Type", contentType)
	if disposition := strings.TrimSpace(resp.Header().Get("Content-Disposition")); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.WriteHeader(resp.StatusCode())
	_, _ = w.Write(resp.Body())
}

func (s *Server) renderStockResearchDetail(w http.ResponseWriter, r *http.Request, id int64) {
	contentURL := strings.TrimRight(strings.TrimSpace(s.cfg.ContentURL), "/")
	if contentURL == "" {
		http.Error(w, "content service is not configured", http.StatusServiceUnavailable)
		return
	}
	var item model.StockResearchSurvey
	err := s.getJSONWithContext(r.Context(), fmt.Sprintf("%s/api/v1/stock-research/%d", contentURL, id), &item)
	if err != nil {
		http.Error(w, "研报详情读取失败："+err.Error(), http.StatusBadGateway)
		return
	}
	returnTo := safeStockResearchReturnURL(r.URL.Query().Get("return_to"))
	if returnTo == "" {
		returnTo = "/stock-research"
	}
	var b strings.Builder
	renderStockResearchStyle(&b)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<div class="research-message">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	b.WriteString(`<section><div class="research-text-actions"><a class="inline" href="`)
	b.WriteString(html.EscapeString(returnTo))
	b.WriteString(`">返回研报列表</a>`)
	if strings.TrimSpace(item.SourceURL) != "" {
		b.WriteString(`<a class="inline" href="`)
		b.WriteString(html.EscapeString(item.SourceURL))
		b.WriteString(`" target="_blank" rel="noreferrer">外部原文</a>`)
	}
	b.WriteString(`<form method="post" action="/stock-research" class="research-action-form"><input type="hidden" name="action" value="fetch_source_one"><input type="hidden" name="force" value="1"><input type="hidden" name="id" value="`)
	b.WriteString(fmt.Sprintf("%d", item.ID))
	b.WriteString(`"><input type="hidden" name="redirect_to" value="`)
	b.WriteString(html.EscapeString(stockResearchDetailURL(item.ID, returnTo)))
	b.WriteString(`"><button type="submit">重新抓取原文</button></form>`)
	b.WriteString(`<form method="post" action="/stock-research" class="research-action-form"><input type="hidden" name="action" value="parse_nlp_one"><input type="hidden" name="id" value="`)
	b.WriteString(fmt.Sprintf("%d", item.ID))
	b.WriteString(`"><input type="hidden" name="redirect_to" value="`)
	b.WriteString(html.EscapeString(stockResearchDetailURL(item.ID, returnTo)))
	b.WriteString(`"><button type="submit">用 NLP 解析文档</button></form></div>`)
	b.WriteString(`<h2>`)
	b.WriteString(html.EscapeString(cleanStockResearchTitle(item)))
	b.WriteString(`</h2><p class="research-text-meta">`)
	b.WriteString(html.EscapeString(stockResearchDetailMeta(item)))
	b.WriteString(`</p></section>`)
	if strings.TrimSpace(item.NLPScoredAt) != "" {
		b.WriteString(`<section><h2>NLP 解析</h2><p class="research-muted">`)
		b.WriteString(html.EscapeString(stockResearchNLPStatusText(item)))
		b.WriteString(`</p></section>`)
	}
	text := strings.TrimSpace(item.SourceText)
	heading := "原文正文"
	if text == "" {
		text = strings.TrimSpace(item.PDFText)
		heading = "PDF 文本"
	}
	if text == "" {
		text = strings.TrimSpace(item.Summary)
		heading = "摘要"
	}
	b.WriteString(`<section><h2>`)
	b.WriteString(html.EscapeString(heading))
	b.WriteString(`</h2>`)
	if text != "" {
		b.WriteString(`<pre class="research-source-text">`)
		b.WriteString(html.EscapeString(text))
		b.WriteString(`</pre>`)
	} else {
		b.WriteString(`<p class="research-muted">暂无入库正文，请点击“重新抓取原文”或查看外部原文。</p>`)
	}
	if strings.TrimSpace(item.SourceFetchStatus) != "" || strings.TrimSpace(item.SourceFetchError) != "" {
		b.WriteString(`<p class="research-muted">原文状态：`)
		b.WriteString(html.EscapeString(stockResearchSourceStatusLabel(item)))
		if strings.TrimSpace(item.SourceFetchedAt) != "" {
			b.WriteString(` ｜ 时间：`)
			b.WriteString(html.EscapeString(item.SourceFetchedAt))
		}
		if strings.TrimSpace(item.SourceFetchError) != "" {
			b.WriteString(` ｜ `)
			b.WriteString(html.EscapeString(item.SourceFetchError))
		}
		b.WriteString(`</p>`)
	}
	b.WriteString(`</section>`)
	_ = s.writeSimplePage(w, "stock-research", "研报详情", b.String())
}

func (s *Server) renderStockResearchPDFText(w http.ResponseWriter, r *http.Request, id int64) {
	contentURL := strings.TrimRight(strings.TrimSpace(s.cfg.ContentURL), "/")
	if contentURL == "" {
		http.Error(w, "content service is not configured", http.StatusServiceUnavailable)
		return
	}
	var item model.StockResearchSurvey
	err := s.getJSONWithContext(r.Context(), fmt.Sprintf("%s/api/v1/stock-research/%d/pdf/text", contentURL, id), &item)
	if err != nil {
		http.Error(w, "PDF 文本读取失败："+err.Error(), http.StatusBadGateway)
		return
	}
	text := strings.TrimSpace(item.PDFText)
	if text == "" {
		http.Error(w, "PDF 文本未解析，请返回列表点击重新解析。", http.StatusNotFound)
		return
	}
	var b strings.Builder
	b.WriteString(`<style>
body[data-page='stock-research-pdf-text'] main,body[data-page='stock-research-pdf-text'] .site-footer{max-width:1180px}
.research-text-meta{color:#6a6257;margin:8px 0 14px}
.research-text-actions{display:flex;gap:12px;flex-wrap:wrap;margin:12px 0}
.research-pdf-text{white-space:pre-wrap;line-height:1.72;background:#fff;border:1px solid #ece7dc;border-radius:12px;padding:18px;overflow:auto}
</style>`)
	b.WriteString(`<section><h2>研报 PDF 文本</h2>`)
	if strings.TrimSpace(item.Title) != "" {
		b.WriteString(`<h3>`)
		b.WriteString(html.EscapeString(item.Title))
		b.WriteString(`</h3>`)
	}
	b.WriteString(`<p class="research-text-meta">`)
	b.WriteString(html.EscapeString(strings.TrimSpace(item.Code + " " + item.Name)))
	if strings.TrimSpace(item.PDFParsedAt) != "" {
		b.WriteString(` ｜ 解析时间：`)
		b.WriteString(html.EscapeString(item.PDFParsedAt))
	}
	b.WriteString(`</p><div class="research-text-actions"><a class="inline" href="/stock-research">返回研报列表</a>`)
	if strings.TrimSpace(item.PDFURL) != "" {
		b.WriteString(`<a class="inline" href="`)
		b.WriteString(html.EscapeString(item.PDFURL))
		b.WriteString(`" target="_blank" rel="noreferrer">原始 PDF 链接</a>`)
	}
	b.WriteString(`</div></section><section><pre class="research-pdf-text">`)
	b.WriteString(html.EscapeString(text))
	b.WriteString(`</pre></section>`)
	_ = s.writeSimplePage(w, "stock-research-pdf-text", "研报 PDF 文本", b.String())
}

func stockResearchFilterFromRequest(r *http.Request) model.StockResearchFilter {
	return model.StockResearchFilter{
		Code:        strings.TrimSpace(r.URL.Query().Get("code")),
		Company:     strings.TrimSpace(r.URL.Query().Get("company")),
		Institution: strings.TrimSpace(r.URL.Query().Get("institution")),
		Kind:        strings.TrimSpace(r.URL.Query().Get("kind")),
		Source:      strings.TrimSpace(r.URL.Query().Get("source")),
		Start:       strings.TrimSpace(r.URL.Query().Get("start")),
		End:         strings.TrimSpace(r.URL.Query().Get("end")),
		Page:        normalizeAStockNewsPage(r.URL.Query().Get("page")),
		PageSize:    stockResearchPageSize,
	}
}

func investorRelationsFilterFromForm(r *http.Request) model.StockResearchFilter {
	return model.StockResearchFilter{
		Code:     formValueAny(r, "ir_code", "code"),
		Company:  formValueAny(r, "ir_company", "company"),
		Kind:     "survey",
		Source:   investorRelationsSourceType,
		Start:    formValueAny(r, "ir_start", "start"),
		End:      formValueAny(r, "ir_end", "end"),
		PageSize: stockResearchPageSize,
	}
}

func formValueAny(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.FormValue(name)); value != "" {
			return value
		}
	}
	return ""
}

func syncStockResearchContextFilter(ctx *model.StockResearchListResult, filter model.StockResearchFilter) {
	ctx.Code = filter.Code
	ctx.Company = filter.Company
	ctx.Institution = filter.Institution
	ctx.Kind = filter.Kind
	ctx.Source = filter.Source
	ctx.Start = filter.Start
	ctx.End = filter.End
	ctx.Page = max(filter.Page, 1)
	ctx.PageSize = stockResearchPageSize
}

func (s *Server) loadStockResearchContext(filter model.StockResearchFilter) (model.StockResearchListResult, error) {
	query := stockResearchQuery(filter)
	var result model.StockResearchListResult
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/stock-research?"+query.Encode(), &result)
	return result, err
}

func stockResearchQuery(filter model.StockResearchFilter) url.Values {
	query := url.Values{}
	query.Set("page", fmt.Sprintf("%d", max(filter.Page, 1)))
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > stockResearchPageSize {
		pageSize = stockResearchPageSize
	}
	query.Set("page_size", fmt.Sprintf("%d", pageSize))
	if filter.Code != "" {
		query.Set("code", filter.Code)
	}
	if filter.Company != "" {
		query.Set("company", filter.Company)
	}
	if filter.Institution != "" {
		query.Set("institution", filter.Institution)
	}
	if filter.Kind != "" {
		query.Set("kind", filter.Kind)
	}
	if filter.Source != "" {
		query.Set("source", filter.Source)
	}
	if filter.Start != "" {
		query.Set("start", filter.Start)
	}
	if filter.End != "" {
		query.Set("end", filter.End)
	}
	return query
}

func investorRelationsStockResearchURL(filter model.StockResearchFilter, message string) string {
	filter.Kind = "survey"
	filter.Source = investorRelationsSourceType
	if filter.Page <= 0 {
		filter.Page = 1
	}
	filter.PageSize = stockResearchPageSize
	query := stockResearchQuery(filter)
	if strings.TrimSpace(message) != "" {
		query.Set("msg", strings.TrimSpace(message))
	}
	if encoded := query.Encode(); encoded != "" {
		return "/stock-research?" + encoded
	}
	return "/stock-research"
}

func investorRelationsRedirectURLFromRequest(r *http.Request) string {
	query := r.URL.Query()
	filter := model.StockResearchFilter{
		Code:     firstNonEmptyQuery(query, "code", "ir_code"),
		Company:  firstNonEmptyQuery(query, "company", "ir_company"),
		Kind:     "survey",
		Source:   investorRelationsSourceType,
		Start:    firstNonEmptyQuery(query, "start", "ir_start"),
		End:      firstNonEmptyQuery(query, "end", "ir_end"),
		Page:     normalizeAStockNewsPage(query.Get("page")),
		PageSize: stockResearchPageSize,
	}
	if filter.Page <= 1 && strings.TrimSpace(query.Get("ir_page")) != "" {
		filter.Page = normalizeAStockNewsPage(query.Get("ir_page"))
	}
	return investorRelationsStockResearchURL(filter, query.Get("msg"))
}

func firstNonEmptyQuery(query url.Values, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(query.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func renderStockResearchFilters(b *strings.Builder, ctx model.StockResearchListResult) {
	b.WriteString(`<section><h2>筛选</h2><form method="get" class="research-toolbar"><div><label>股票代码</label><input name="code" placeholder="002230" value="`)
	b.WriteString(html.EscapeString(ctx.Code))
	b.WriteString(`"></div><div><label>公司名称</label><input name="company" placeholder="科大讯飞" value="`)
	b.WriteString(html.EscapeString(ctx.Company))
	b.WriteString(`"></div><div><label>机构名称</label><input name="institution" placeholder="中金公司" value="`)
	b.WriteString(html.EscapeString(ctx.Institution))
	b.WriteString(`"></div><div><label>类型</label><select name="kind">`)
	for _, opt := range []struct{ Value, Label string }{{"", "全部"}, {"report", "研报"}, {"survey", "调研"}} {
		b.WriteString(`<option value="`)
		b.WriteString(opt.Value)
		b.WriteString(`"`)
		if opt.Value == ctx.Kind {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(opt.Label)
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></div><div><label>来源</label><select name="source"><option value="">全部</option>`)
	for _, source := range ctx.Sources {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(source))
		b.WriteString(`"`)
		if source == ctx.Source {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(stockResearchSourceLabel(source)))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></div><div><label>开始日期</label><input type="date" name="start" value="`)
	b.WriteString(html.EscapeString(ctx.Start))
	b.WriteString(`"></div><div><label>结束日期</label><input type="date" name="end" value="`)
	b.WriteString(html.EscapeString(ctx.End))
	b.WriteString(`"></div><div><button type="submit">查询</button></div></form><div class="research-actions">`)
	b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="action" value="backfill_year">`)
	stockResearchHiddenFields(b, ctx)
	b.WriteString(`<button type="submit">回补近一年</button></form>`)
	b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="action" value="parse_pdf">`)
	stockResearchHiddenFields(b, ctx)
	b.WriteString(`<button type="submit">解析当前筛选研报PDF</button></form>`)
	b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="action" value="parse_nlp">`)
	stockResearchHiddenFields(b, ctx)
	b.WriteString(`<button type="submit">NLP解析当前筛选文档</button></form>`)
	b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="scope" value="investor_relations"><input type="hidden" name="action" value="backfill_year">`)
	stockResearchHiddenFields(b, ctx)
	b.WriteString(`<button type="submit">抓取投资者关系近一年并解析PDF</button></form>`)
	b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="action" value="fetch_source_page">`)
	stockResearchHiddenFields(b, ctx)
	b.WriteString(`<input type="hidden" name="page" value="`)
	b.WriteString(fmt.Sprintf("%d", max(ctx.Page, 1)))
	b.WriteString(`"><button type="submit">补抓当前页原文入库</button></form>`)
	b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="action" value="repair_sina_source_7d">`)
	stockResearchHiddenFields(b, ctx)
	b.WriteString(`<button type="submit">修复近7日新浪原文</button></form></div></section>`)
}

type stockResearchTableOptions struct {
	title        string
	emptyMessage string
}

func renderStockResearchTable(b *strings.Builder, ctx model.StockResearchListResult) {
	renderStockResearchTableWithOptions(b, ctx, stockResearchTableOptions{
		title:        "研报调研列表",
		emptyMessage: "暂无研报调研数据，请点击“回补近一年”或等待定时抓取任务。",
	})
}

func renderStockResearchTableWithOptions(b *strings.Builder, ctx model.StockResearchListResult, opts stockResearchTableOptions) {
	if opts.title == "" {
		opts.title = "研报调研列表"
	}
	if opts.emptyMessage == "" {
		opts.emptyMessage = "暂无研报调研数据，请点击“回补近一年”或等待定时抓取任务。"
	}
	b.WriteString(`<section><h2>`)
	b.WriteString(html.EscapeString(opts.title))
	b.WriteString(`</h2><div class="research-scroll"><table class="research-table"><colgroup><col class="research-col-date"><col class="research-col-stock"><col class="research-col-title"><col class="research-col-institution"><col class="research-col-analyst"><col class="research-col-source"><col class="research-col-link"><col class="research-col-pdf"><col class="research-col-status"></colgroup><tr><th>日期</th><th>股票</th><th>标题</th><th>机构</th><th>分析师</th><th>来源</th><th>链接</th><th>PDF</th><th>解析状态</th></tr>`)
	if len(ctx.Items) == 0 {
		b.WriteString(`<tr><td colspan="9">`)
		b.WriteString(html.EscapeString(opts.emptyMessage))
		b.WriteString(`</td></tr>`)
	} else {
		for _, item := range ctx.Items {
			b.WriteString(`<tr><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.ResearchDate, item.PublishTime, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(item.Code + " " + item.Name)))
			b.WriteString(`</td><td>`)
			b.WriteString(`<a class="inline" href="`)
			b.WriteString(html.EscapeString(stockResearchDetailURL(item.ID, stockResearchListReturnURL(ctx))))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(cleanStockResearchTitle(item)))
			b.WriteString(`</a>`)
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Institution, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Analyst, "--")))
			b.WriteString(`</td><td>`)
			label := stockResearchSourceLabel(item.SourceType)
			if label != "" {
				b.WriteString(`<span class="research-source">`)
				b.WriteString(html.EscapeString(label))
				b.WriteString(`</span>`)
			} else {
				b.WriteString(`--`)
			}
			b.WriteString(`</td><td>`)
			if strings.TrimSpace(item.SourceURL) != "" {
				b.WriteString(`<a class="inline" href="`)
				b.WriteString(html.EscapeString(item.SourceURL))
				b.WriteString(`" target="_blank" rel="noreferrer">原文</a>`)
			} else {
				b.WriteString(`--`)
			}
			b.WriteString(`</td><td>`)
			if strings.TrimSpace(item.PDFFilePath) != "" {
				b.WriteString(`<a class="inline" href="/stock-research/`)
				b.WriteString(fmt.Sprintf("%d", item.ID))
				b.WriteString(`/pdf" target="_blank" rel="noreferrer">下载PDF</a>`)
			} else if strings.TrimSpace(item.PDFURL) != "" {
				b.WriteString(`<a class="inline" href="`)
				b.WriteString(html.EscapeString(item.PDFURL))
				b.WriteString(`" target="_blank" rel="noreferrer">PDF链接</a>`)
			} else {
				b.WriteString(`--`)
			}
			if strings.EqualFold(item.PDFStatus, "parsed") && strings.TrimSpace(item.PDFText) != "" {
				b.WriteString(` <a class="inline" href="/stock-research/`)
				b.WriteString(fmt.Sprintf("%d", item.ID))
				b.WriteString(`/pdf/text" target="_blank" rel="noreferrer">查看文本</a>`)
			}
			displayStatus := stockResearchDisplayPDFStatus(item)
			b.WriteString(`</td><td><div class="research-status-actions"><span class="research-status `)
			b.WriteString(html.EscapeString(stockResearchPDFStatusClass(displayStatus)))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(stockResearchPDFStatusLabel(displayStatus)))
			parseAction := "parse_pdf_one"
			parseLabel := "重新解析"
			if stockResearchHasNLPText(item) {
				parseAction = "parse_nlp_one"
				parseLabel = "NLP解析"
			}
			b.WriteString(`</span><form method="post" class="research-action-form"><input type="hidden" name="action" value="`)
			b.WriteString(html.EscapeString(parseAction))
			b.WriteString(`"><input type="hidden" name="id" value="`)
			b.WriteString(fmt.Sprintf("%d", item.ID))
			b.WriteString(`">`)
			stockResearchHiddenFields(b, ctx)
			b.WriteString(`<button type="submit">`)
			b.WriteString(html.EscapeString(parseLabel))
			b.WriteString(`</button></form>`)
			b.WriteString(`<form method="post" class="research-action-form"><input type="hidden" name="action" value="fetch_source_one"><input type="hidden" name="id" value="`)
			b.WriteString(fmt.Sprintf("%d", item.ID))
			b.WriteString(`">`)
			if strings.TrimSpace(item.SourceText) != "" {
				b.WriteString(`<input type="hidden" name="force" value="1">`)
			}
			stockResearchHiddenFields(b, ctx)
			b.WriteString(`<input type="hidden" name="page" value="`)
			b.WriteString(fmt.Sprintf("%d", max(ctx.Page, 1)))
			b.WriteString(`"><button type="submit">`)
			if strings.TrimSpace(item.SourceText) != "" {
				b.WriteString(`重新抓取`)
			} else {
				b.WriteString(`抓取原文`)
			}
			b.WriteString(`</button></form></div>`)
			if strings.TrimSpace(item.SourceText) != "" || strings.TrimSpace(item.SourceFetchStatus) != "" {
				b.WriteString(`<div class="research-muted">`)
				b.WriteString(html.EscapeString(stockResearchSourceStatusLabel(item)))
				b.WriteString(`</div>`)
			}
			if strings.TrimSpace(item.NLPScoredAt) != "" {
				b.WriteString(`<div class="research-muted">NLP `)
				b.WriteString(html.EscapeString(formatStockResearchTargetPrice(fmt.Sprintf("%.2f", item.NLPScore))))
				if strings.TrimSpace(item.NLPRating) != "" {
					b.WriteString(` `)
					b.WriteString(html.EscapeString(item.NLPRating))
				}
				if strings.TrimSpace(item.NLPReason) != "" {
					b.WriteString(`：`)
					b.WriteString(html.EscapeString(item.NLPReason))
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</td></tr>`)
		}
	}
	b.WriteString(`</table></div>`)
	renderStockResearchPagination(b, ctx)
	b.WriteString(`</section>`)
}

func renderStockResearchPagination(b *strings.Builder, ctx model.StockResearchListResult) {
	if ctx.PageSize <= 0 || ctx.Total <= ctx.PageSize {
		return
	}
	totalPages := (ctx.Total + ctx.PageSize - 1) / ctx.PageSize
	b.WriteString(`<div class="research-tabs"><span class="research-muted">第 `)
	b.WriteString(fmt.Sprintf("%d/%d 页，共 %d 条", ctx.Page, totalPages, ctx.Total))
	b.WriteString(`</span>`)
	for _, link := range []struct {
		Page  int
		Label string
	}{{ctx.Page - 1, "上一页"}, {ctx.Page + 1, "下一页"}} {
		if link.Page < 1 || link.Page > totalPages {
			continue
		}
		filter := model.StockResearchFilter{Code: ctx.Code, Company: ctx.Company, Institution: ctx.Institution, Kind: ctx.Kind, Source: ctx.Source, Start: ctx.Start, End: ctx.End, Page: link.Page, PageSize: stockResearchPageSize}
		query := stockResearchQuery(filter)
		href := "/stock-research"
		if encoded := query.Encode(); encoded != "" {
			href += "?" + encoded
		}
		b.WriteString(`<a class="research-tab" href="`)
		b.WriteString(html.EscapeString(href))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(link.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func (s *Server) handleStockResearchAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := model.StockResearchFilter{
		Code:        strings.TrimSpace(r.FormValue("code")),
		Company:     strings.TrimSpace(r.FormValue("company")),
		Institution: strings.TrimSpace(r.FormValue("institution")),
		Kind:        strings.TrimSpace(r.FormValue("kind")),
		Source:      strings.TrimSpace(r.FormValue("source")),
		Start:       strings.TrimSpace(r.FormValue("start")),
		End:         strings.TrimSpace(r.FormValue("end")),
		Page:        normalizeAStockNewsPage(r.FormValue("page")),
		PageSize:    stockResearchPageSize,
	}
	message := "未知操作"
	preservePage := false
	switch strings.TrimSpace(r.FormValue("action")) {
	case "backfill_year":
		message = s.triggerStockResearchBackfill(filter)
	case "parse_pdf":
		message = s.triggerStockResearchPDFParse(filter, 0)
	case "parse_pdf_one":
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if err != nil || id <= 0 {
			message = "研报 PDF 解析失败：无效记录 ID"
		} else {
			message = s.triggerStockResearchPDFParse(filter, id)
		}
	case "parse_nlp":
		message = s.triggerStockResearchNLPParse(filter, 0)
	case "parse_nlp_one":
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if err != nil || id <= 0 {
			message = "研报 NLP 解析失败：无效记录 ID"
		} else {
			message = s.triggerStockResearchNLPParse(filter, id)
		}
	case "fetch_source_page":
		preservePage = true
		message = s.fetchStockResearchSourcePage(r.Context(), filter)
	case "fetch_source_one":
		preservePage = true
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if err != nil || id <= 0 {
			message = "原文抓取失败：无效记录 ID"
		} else {
			force := strings.TrimSpace(r.FormValue("force")) == "1"
			message = s.fetchStockResearchSourceOne(r.Context(), id, force)
		}
	case "repair_sina_source_7d":
		message = s.triggerSinaStockResearchSourceRepair()
	}
	if !preservePage {
		filter.Page = 1
	}
	filter.PageSize = stockResearchPageSize
	if redirectTo := safeStockResearchReturnURL(r.FormValue("redirect_to")); redirectTo != "" {
		http.Redirect(w, r, stockResearchURLWithMessage(redirectTo, message), http.StatusSeeOther)
		return
	}
	query := stockResearchQuery(filter)
	query.Set("msg", message)
	http.Redirect(w, r, "/stock-research?"+query.Encode(), http.StatusSeeOther)
}

func (s *Server) handleInvestorRelationsAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := investorRelationsFilterFromForm(r)
	message := "未知操作"
	switch strings.TrimSpace(r.FormValue("action")) {
	case "backfill_year":
		message = s.triggerInvestorRelationsBackfill(filter)
	case "parse_pdf":
		message = s.triggerStockResearchPDFParse(filter, 0)
	case "parse_pdf_one":
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if err != nil || id <= 0 {
			message = "投资者关系 PDF 解析失败：无效记录 ID"
		} else {
			message = s.triggerStockResearchPDFParse(filter, id)
		}
	case "parse_nlp":
		message = s.triggerStockResearchNLPParse(filter, 0)
	case "parse_nlp_one":
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if err != nil || id <= 0 {
			message = "投资者关系 NLP 解析失败：无效记录 ID"
		} else {
			message = s.triggerStockResearchNLPParse(filter, id)
		}
	}
	filter.Page = 1
	http.Redirect(w, r, investorRelationsStockResearchURL(filter, message), http.StatusSeeOther)
}

func (s *Server) fetchStockResearchSourcePage(ctx context.Context, filter model.StockResearchFilter) string {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = stockResearchPageSize
	list, err := s.loadStockResearchContext(filter)
	if err != nil {
		return "当前页原文补抓失败：" + err.Error()
	}
	total, fetched, skipped, failed := len(list.Items), 0, 0, 0
	for _, item := range list.Items {
		if strings.TrimSpace(item.SourceText) != "" {
			skipped++
			continue
		}
		if err := s.fetchAndWriteStockResearchSource(ctx, item, false); err != nil {
			failed++
		} else {
			fetched++
		}
	}
	return fmt.Sprintf("当前页原文补抓完成：共 %d 条，抓取 %d 条，跳过已有正文 %d 条，失败 %d 条。", total, fetched, skipped, failed)
}

func (s *Server) fetchStockResearchSourceOne(ctx context.Context, id int64, force bool) string {
	item, err := s.loadStockResearchItem(ctx, id)
	if err != nil {
		return "原文抓取失败：" + err.Error()
	}
	if strings.TrimSpace(item.SourceText) != "" && !force {
		return "原文已入库，已跳过。"
	}
	if err := s.fetchAndWriteStockResearchSource(ctx, item, force); err != nil {
		return "原文抓取失败：" + err.Error()
	}
	if force {
		return "原文已重新抓取并入库。"
	}
	return "原文已抓取并入库。"
}

func (s *Server) fetchAndWriteStockResearchSource(ctx context.Context, item model.StockResearchSurvey, force bool) error {
	update := stockResearchSourceUpdateFromItem(ctx, item, s.cfg.UserAgent, stockResearchHTTPTimeout(s.cfg.HTTPTimeout))
	update.Force = force
	contentURL := strings.TrimRight(strings.TrimSpace(s.cfg.ContentURL), "/")
	if contentURL == "" {
		return fmt.Errorf("content service is not configured")
	}
	resp, err := s.client.R().
		SetContext(ctx).
		SetBody(update).
		Post(fmt.Sprintf("%s/api/v1/internal/stock-research/%d/source", contentURL, item.ID))
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content source update failed: %s", resp.Status())
	}
	if update.SourceFetchStatus == stockresearch.SourceStatusFailed {
		return fmt.Errorf("%s", update.SourceFetchError)
	}
	return nil
}

func (s *Server) loadStockResearchItem(ctx context.Context, id int64) (model.StockResearchSurvey, error) {
	contentURL := strings.TrimRight(strings.TrimSpace(s.cfg.ContentURL), "/")
	if contentURL == "" {
		return model.StockResearchSurvey{}, fmt.Errorf("content service is not configured")
	}
	var item model.StockResearchSurvey
	err := s.getJSONWithContext(ctx, fmt.Sprintf("%s/api/v1/stock-research/%d", contentURL, id), &item)
	return item, err
}

func stockResearchSourceUpdateFromItem(ctx context.Context, item model.StockResearchSurvey, userAgent string, timeout time.Duration) model.StockResearchSourceUpdate {
	if strings.TrimSpace(item.PDFText) != "" {
		return stockresearch.SourceUpdateFromPDFText(item.PDFText, time.Now().UTC().Format(time.RFC3339))
	}
	if stockresearch.LooksLikePDFURL(nonEmptyText(item.PDFURL, item.SourceURL)) {
		return model.StockResearchSourceUpdate{
			SourceFetchStatus: stockresearch.SourceStatusPendingPDF,
			SourceFetchedAt:   time.Now().UTC().Format(time.RFC3339),
		}
	}
	return stockresearch.FetchSource(ctx, item.SourceURL, stockresearch.FetchOptions{
		UserAgent: nonEmptyText(userAgent, "Mozilla/5.0"),
		Timeout:   timeout,
	})
}

func stockResearchHTTPTimeout(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return 15 * time.Second
}

func (s *Server) triggerStockResearchBackfill(filter model.StockResearchFilter) string {
	end := time.Now().In(aStockLocation())
	start := end.AddDate(-1, 0, 0)
	query := url.Values{}
	query.Set("start", start.Format("2006-01-02"))
	query.Set("end", end.Format("2006-01-02"))
	if filter.Code != "" {
		query.Set("code", filter.Code)
	}
	if filter.Company != "" {
		query.Set("company", filter.Company)
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/stock-research/backfill?" + query.Encode())
	if err != nil {
		return "研报调研回补失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "研报调研回补失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	return "研报调研近一年回补任务已触发，请稍后刷新查看。"
}

func (s *Server) triggerSinaStockResearchSourceRepair() string {
	query := url.Values{}
	query.Set("source", "sina_finance_report")
	query.Set("force", "true")
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/stock-research/source/repair?" + query.Encode())
	if err != nil {
		return "新浪研报原文修复失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "新浪研报原文修复失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	var envelope struct {
		Data struct {
			Result struct {
				Total    int  `json:"total"`
				Repaired int  `json:"repaired"`
				Skipped  int  `json:"skipped"`
				Failed   int  `json:"failed"`
				DryRun   bool `json:"dry_run"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return "新浪近7日研报原文修复任务已触发，请稍后刷新查看。"
	}
	result := envelope.Data.Result
	return fmt.Sprintf("新浪近7日研报原文修复完成：共 %d 条，修复 %d 条，跳过 %d 条，失败 %d 条。", result.Total, result.Repaired, result.Skipped, result.Failed)
}

func (s *Server) triggerInvestorRelationsBackfill(filter model.StockResearchFilter) string {
	end := time.Now().In(aStockLocation())
	start := end.AddDate(-1, 0, 0)
	query := url.Values{}
	query.Set("start", start.Format("2006-01-02"))
	query.Set("end", end.Format("2006-01-02"))
	if filter.Code != "" {
		query.Set("code", filter.Code)
	}
	if filter.Company != "" {
		query.Set("company", filter.Company)
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/investor-relations/backfill?" + query.Encode())
	if err != nil {
		return "投资者关系抓取失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "投资者关系抓取失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	return "投资者关系近一年抓取和 PDF 解析任务已触发，请稍后刷新查看。"
}

func (s *Server) triggerStockResearchPDFParse(filter model.StockResearchFilter, id int64) string {
	query := url.Values{}
	if id > 0 {
		query.Set("id", fmt.Sprintf("%d", id))
	} else {
		if filter.Code != "" {
			query.Set("code", filter.Code)
		}
		if filter.Company != "" {
			query.Set("company", filter.Company)
		}
		if filter.Source != "" {
			query.Set("source", filter.Source)
		}
		if filter.Start != "" {
			query.Set("start", filter.Start)
		}
		if filter.End != "" {
			query.Set("end", filter.End)
		}
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/stock-research/pdf/parse?" + query.Encode())
	if err != nil {
		return "研报 PDF 解析失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "研报 PDF 解析失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	return "研报 PDF 解析任务已触发，请稍后刷新查看。"
}

func (s *Server) triggerStockResearchNLPParse(filter model.StockResearchFilter, id int64) string {
	query := url.Values{}
	if id > 0 {
		query.Set("id", fmt.Sprintf("%d", id))
	} else {
		if filter.Code != "" {
			query.Set("code", filter.Code)
		}
		if filter.Company != "" {
			query.Set("company", filter.Company)
		}
		if filter.Kind != "" {
			query.Set("kind", filter.Kind)
		}
		if filter.Source != "" {
			query.Set("source", filter.Source)
		}
		if filter.Start != "" {
			query.Set("start", filter.Start)
		}
		if filter.End != "" {
			query.Set("end", filter.End)
		}
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/stock-research/nlp/parse?" + query.Encode())
	if err != nil {
		return "研报 NLP 解析失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		return "研报 NLP 解析失败：" + stockResearchSchedulerError(resp.Body(), resp.String())
	}
	var envelope struct {
		Data struct {
			Result struct {
				Total  int `json:"total"`
				Scored int `json:"scored"`
				NoText int `json:"no_text"`
				Failed int `json:"failed"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err == nil && envelope.Data.Result.Total > 0 {
		result := envelope.Data.Result
		return fmt.Sprintf("研报 NLP 解析完成：共 %d 条，评分 %d 条，无文本 %d 条，失败 %d 条。", result.Total, result.Scored, result.NoText, result.Failed)
	}
	return "研报 NLP 解析任务已触发，请稍后刷新查看。"
}

func stockResearchSchedulerError(body []byte, fallback string) string {
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && strings.TrimSpace(envelope.Message) != "" {
		return strings.TrimSpace(envelope.Message)
	}
	return strings.TrimSpace(fallback)
}

func stockResearchHiddenFields(b *strings.Builder, ctx model.StockResearchListResult) {
	for _, field := range []struct {
		Name  string
		Value string
	}{
		{"code", ctx.Code},
		{"company", ctx.Company},
		{"institution", ctx.Institution},
		{"kind", ctx.Kind},
		{"source", ctx.Source},
		{"start", ctx.Start},
		{"end", ctx.End},
	} {
		b.WriteString(`<input type="hidden" name="`)
		b.WriteString(field.Name)
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(field.Value))
		b.WriteString(`">`)
	}
}

func stockResearchListReturnURL(ctx model.StockResearchListResult) string {
	filter := model.StockResearchFilter{
		Code:        ctx.Code,
		Company:     ctx.Company,
		Institution: ctx.Institution,
		Kind:        ctx.Kind,
		Source:      ctx.Source,
		Start:       ctx.Start,
		End:         ctx.End,
		Page:        max(ctx.Page, 1),
		PageSize:    stockResearchPageSize,
	}
	query := stockResearchQuery(filter)
	if encoded := query.Encode(); encoded != "" {
		return "/stock-research?" + encoded
	}
	return "/stock-research"
}

func stockResearchDetailURL(id int64, returnTo string) string {
	query := url.Values{}
	if safe := safeStockResearchReturnURL(returnTo); safe != "" {
		query.Set("return_to", safe)
	}
	path := fmt.Sprintf("/stock-research/%d", id)
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}

func stockResearchURLWithMessage(rawURL string, message string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "/stock-research?msg=" + url.QueryEscape(message)
	}
	query := parsed.Query()
	query.Set("msg", message)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func safeStockResearchReturnURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/stock-research") && !strings.HasPrefix(value, "//") {
		return value
	}
	return ""
}

func stockResearchSourceStatusLabel(item model.StockResearchSurvey) string {
	switch strings.TrimSpace(item.SourceFetchStatus) {
	case stockresearch.SourceStatusParsed:
		return "原文已入库"
	case stockresearch.SourceStatusPendingPDF:
		return "原文等待PDF解析"
	case stockresearch.SourceStatusNoSource:
		return "无原文链接"
	case stockresearch.SourceStatusFailed:
		return "原文抓取失败"
	default:
		if strings.TrimSpace(item.SourceText) != "" {
			return "原文已入库"
		}
		return "原文未入库"
	}
}

func stockResearchHasNLPText(item model.StockResearchSurvey) bool {
	return strings.TrimSpace(item.SourceText) != "" || strings.TrimSpace(item.PDFText) != "" || strings.TrimSpace(item.Summary) != ""
}

func stockResearchNLPStatusText(item model.StockResearchSurvey) string {
	parts := []string{fmt.Sprintf("NLP %.2f", item.NLPScore)}
	if rating := strings.TrimSpace(item.NLPRating); rating != "" {
		parts = append(parts, rating)
	}
	if reason := strings.TrimSpace(item.NLPReason); reason != "" {
		parts = append(parts, reason)
	}
	if scoredAt := strings.TrimSpace(item.NLPScoredAt); scoredAt != "" {
		parts = append(parts, "时间："+scoredAt)
	}
	return strings.Join(parts, " ｜ ")
}

func stockResearchDetailMeta(item model.StockResearchSurvey) string {
	parts := []string{}
	if stock := strings.TrimSpace(item.Code + " " + item.Name); stock != "" {
		parts = append(parts, stock)
	}
	if date := strings.TrimSpace(nonEmptyText(item.ResearchDate, item.PublishTime)); date != "" {
		parts = append(parts, "日期："+date)
	}
	if institution := strings.TrimSpace(item.Institution); institution != "" {
		parts = append(parts, "机构："+institution)
	}
	if analyst := strings.TrimSpace(item.Analyst); analyst != "" {
		parts = append(parts, "分析师："+analyst)
	}
	if source := stockResearchSourceLabel(item.SourceType); source != "" && source != "--" {
		parts = append(parts, "来源："+source)
	}
	return strings.Join(parts, " ｜ ")
}

func stockResearchKindLabel(kind string) string {
	if strings.EqualFold(kind, "survey") {
		return "调研"
	}
	return "研报"
}

func cleanStockResearchTitle(item model.StockResearchSurvey) string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return "--"
	}
	for _, token := range []string{strings.TrimSpace(item.Code), strings.TrimSpace(item.Name)} {
		if token == "" {
			continue
		}
		title = strings.ReplaceAll(title, token, "")
	}
	title = strings.TrimSpace(title)
	title = cleanStockResearchTitlePrefix(title)
	title = strings.Trim(title, " \t\r\n:：-—_｜|·,，、;；")
	if title == "" {
		return "--"
	}
	return strings.Join(strings.Fields(title), " ")
}

func cleanStockResearchTitlePrefix(title string) string {
	for {
		before := title
		title = strings.TrimSpace(title)
		title = trimStockResearchLeadingParenthesized(title)
		title = trimStockResearchLeadingZeroMarker(title)
		title = trimStockResearchLeadingColonPrefix(title)
		title = strings.TrimLeft(title, " \t\r\n:：-—_｜|·,，、;；")
		if title == before {
			return title
		}
	}
}

func trimStockResearchLeadingParenthesized(title string) string {
	if title == "" {
		return title
	}
	pairs := []struct {
		Open  string
		Close string
	}{{"(", ")"}, {"（", "）"}}
	for _, pair := range pairs {
		if !strings.HasPrefix(title, pair.Open) {
			continue
		}
		closeIdx := strings.Index(title, pair.Close)
		if closeIdx < 0 {
			return title
		}
		prefix := title[:closeIdx+len(pair.Close)]
		if len([]rune(prefix)) > 24 {
			return title
		}
		return title[closeIdx+len(pair.Close):]
	}
	return title
}

func trimStockResearchLeadingZeroMarker(title string) string {
	if title == "" {
		return title
	}
	trimmed := strings.TrimLeft(title, "0")
	if trimmed == "" || trimmed == title {
		return title
	}
	if strings.HasPrefix(trimmed, ":") || strings.HasPrefix(trimmed, "：") || stockResearchTitlePrefixHasColon(trimmed) {
		return trimmed
	}
	return title
}

func trimStockResearchLeadingColonPrefix(title string) string {
	colonIdx, colonSize := stockResearchTitleColonIndex(title)
	if colonIdx < 0 {
		return title
	}
	prefix := strings.TrimSpace(title[:colonIdx])
	if prefix == "" || len([]rune(prefix)) > 24 {
		return title
	}
	if stockResearchTitlePrefixIsNoise(prefix) {
		return title[colonIdx+colonSize:]
	}
	return title
}

func stockResearchTitlePrefixHasColon(title string) bool {
	colonIdx, _ := stockResearchTitleColonIndex(title)
	return colonIdx >= 0 && len([]rune(strings.TrimSpace(title[:colonIdx]))) <= 24
}

func stockResearchTitleColonIndex(title string) (int, int) {
	half := strings.Index(title, ":")
	full := strings.Index(title, "：")
	switch {
	case half < 0:
		if full < 0 {
			return -1, 0
		}
		return full, len("：")
	case full < 0 || half < full:
		return half, len(":")
	default:
		return full, len("：")
	}
}

func stockResearchTitlePrefixIsNoise(prefix string) bool {
	prefix = strings.Trim(prefix, " \t\r\n()（）:：-—_｜|·,，、;；")
	if prefix == "" || prefix == "0" {
		return true
	}
	if strings.Contains(prefix, "点评") || strings.Contains(prefix, "报告") || strings.Contains(prefix, "深度") || strings.Contains(prefix, "跟踪") {
		return true
	}
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatStockResearchTargetPrice(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "--"
	}
	normalized := strings.ReplaceAll(raw, ",", "")
	if parsed, err := strconv.ParseFloat(normalized, 64); err == nil {
		return fmt.Sprintf("%.2f", parsed)
	}
	return raw
}

func stockResearchSourceLabel(source string) string {
	switch source {
	case "akshare_stock_research":
		return "AKShare"
	case "eastmoney_report", "eastmoney_survey":
		return "东方财富"
	case "tushare_survey":
		return "TuShare"
	case "sina_finance_report":
		return "新浪财经"
	case "sohu_finance_report":
		return "搜狐财经"
	case investorRelationsSourceType:
		return "互动易投资者关系"
	default:
		return nonEmptyText(source, "--")
	}
}

func stockResearchPDFStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "待解析"
	case "downloaded":
		return "已下载"
	case "parsed":
		return "已解析"
	case "no_pdf":
		return "无PDF"
	case "no_text":
		return "无文本"
	case "failed":
		return "失败"
	default:
		return "未处理"
	}
}

func stockResearchDisplayPDFStatus(item model.StockResearchSurvey) string {
	if strings.TrimSpace(item.PDFFilePath) == "" && strings.TrimSpace(item.PDFURL) == "" {
		return "no_pdf"
	}
	return item.PDFStatus
}

func stockResearchPDFStatusClass(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "parsed":
		return "parsed"
	case "failed":
		return "failed"
	case "no_pdf":
		return "no_pdf"
	case "no_text":
		return "no_text"
	default:
		return "pending"
	}
}
