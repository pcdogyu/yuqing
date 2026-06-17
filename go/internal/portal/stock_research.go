package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Server) handleStockResearchPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleStockResearchAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := stockResearchFilterFromRequest(r)
	ctx, err := s.loadStockResearchContext(filter)
	var b strings.Builder
	b.WriteString(`<style>
body[data-page='stock-research'] main,body[data-page='stock-research'] .site-footer{max-width:1534px}
.research-muted{color:#6a6257}.research-message{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}
.research-toolbar{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;align-items:end}
.research-toolbar button{margin:0}.research-actions{display:flex;gap:10px;flex-wrap:wrap}.research-scroll{overflow:auto}
.research-table{min-width:1180px}.research-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
.research-tab{display:inline-flex;align-items:center;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
.research-source{font-size:12px;padding:3px 8px;border-radius:999px;background:#eff6f0;color:#214e34}
</style>`)
	b.WriteString(`<section><h2>研报调研</h2><p class="research-muted">搜索上市公司研报和机构调研记录，数据源包括 AKShare/TuShare、东方财富、新浪财经和搜狐财经。PDF 正文暂不解析，优先展示标题、机构、评级、目标价和原文链接。</p></section>`)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		b.WriteString(`<div class="research-message">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	if err != nil {
		b.WriteString(`<section><p>研报调研数据读取失败：`)
		b.WriteString(html.EscapeString(err.Error()))
		b.WriteString(`</p></section>`)
		_ = s.writeSimplePage(w, "stock-research", "研报调研", b.String())
		return
	}
	renderStockResearchFilters(&b, ctx)
	renderStockResearchTable(&b, ctx)
	_ = s.writeSimplePage(w, "stock-research", "研报调研", b.String())
	_ = user
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
		PageSize:    50,
	}
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
	query.Set("page_size", fmt.Sprintf("%d", max(filter.PageSize, 50)))
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
	b.WriteString(`"></div><div class="research-actions"><button type="submit">查询</button></form><form method="post"><input type="hidden" name="action" value="backfill_year">`)
	for key, value := range map[string]string{"code": ctx.Code, "company": ctx.Company, "institution": ctx.Institution, "kind": ctx.Kind, "source": ctx.Source, "start": ctx.Start, "end": ctx.End} {
		b.WriteString(`<input type="hidden" name="`)
		b.WriteString(key)
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(value))
		b.WriteString(`">`)
	}
	b.WriteString(`<button type="submit">回补近一年</button></form></div></section>`)
}

func renderStockResearchTable(b *strings.Builder, ctx model.StockResearchListResult) {
	b.WriteString(`<section><h2>研报调研列表</h2><div class="research-scroll"><table class="research-table"><tr><th>日期</th><th>股票</th><th>类型</th><th>标题</th><th>机构</th><th>分析师</th><th>评级</th><th>目标价</th><th>来源</th><th>链接</th></tr>`)
	if len(ctx.Items) == 0 {
		b.WriteString(`<tr><td colspan="10">暂无研报调研数据，请点击“回补近一年”或等待定时抓取任务。</td></tr>`)
	} else {
		for _, item := range ctx.Items {
			b.WriteString(`<tr><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.ResearchDate, item.PublishTime, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(strings.TrimSpace(item.Code + " " + item.Name)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(stockResearchKindLabel(item.Kind)))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(item.Title))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Institution, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Analyst, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Rating, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.TargetPrice, "--")))
			b.WriteString(`</td><td><span class="research-source">`)
			b.WriteString(html.EscapeString(stockResearchSourceLabel(item.SourceType)))
			b.WriteString(`</span></td><td>`)
			if strings.TrimSpace(item.SourceURL) != "" {
				b.WriteString(`<a class="inline" href="`)
				b.WriteString(html.EscapeString(item.SourceURL))
				b.WriteString(`" target="_blank" rel="noreferrer">原文</a>`)
			} else {
				b.WriteString(`--`)
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
		filter := model.StockResearchFilter{Code: ctx.Code, Company: ctx.Company, Institution: ctx.Institution, Kind: ctx.Kind, Source: ctx.Source, Start: ctx.Start, End: ctx.End, Page: link.Page, PageSize: ctx.PageSize}
		b.WriteString(`<a class="research-tab" href="/stock-research?`)
		b.WriteString(html.EscapeString(stockResearchQuery(filter).Encode()))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(link.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func (s *Server) handleStockResearchAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := model.StockResearchFilter{
		Code:    strings.TrimSpace(r.FormValue("code")),
		Company: strings.TrimSpace(r.FormValue("company")),
	}
	message := "未知操作"
	if strings.TrimSpace(r.FormValue("action")) == "backfill_year" {
		message = s.triggerStockResearchBackfill(filter)
	}
	query := stockResearchQuery(model.StockResearchFilter{
		Code:        strings.TrimSpace(r.FormValue("code")),
		Company:     strings.TrimSpace(r.FormValue("company")),
		Institution: strings.TrimSpace(r.FormValue("institution")),
		Kind:        strings.TrimSpace(r.FormValue("kind")),
		Source:      strings.TrimSpace(r.FormValue("source")),
		Start:       strings.TrimSpace(r.FormValue("start")),
		End:         strings.TrimSpace(r.FormValue("end")),
		Page:        1,
		PageSize:    50,
	})
	query.Set("msg", message)
	http.Redirect(w, r, "/stock-research?"+query.Encode(), http.StatusSeeOther)
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

func stockResearchSchedulerError(body []byte, fallback string) string {
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && strings.TrimSpace(envelope.Message) != "" {
		return strings.TrimSpace(envelope.Message)
	}
	return strings.TrimSpace(fallback)
}

func stockResearchKindLabel(kind string) string {
	if strings.EqualFold(kind, "survey") {
		return "调研"
	}
	return "研报"
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
	default:
		return nonEmptyText(source, "--")
	}
}
