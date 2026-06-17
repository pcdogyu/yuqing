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

func (s *Server) handleAStockAuctionPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleAStockAuctionAction(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	page := normalizeAStockNewsPage(r.URL.Query().Get("page"))
	ctx, err := s.loadAStockAuctionContext(date, keyword, page)

	var b strings.Builder
	b.WriteString(`<style>
		body[data-page='a-stock-auction'] main,body[data-page='a-stock-auction'] .site-footer{max-width:1534px}
		.auction-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px}
		.auction-card{padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#fff}
		.auction-muted{color:#6a6257}
		.auction-tabs{display:flex;gap:8px;flex-wrap:wrap;margin:14px 0}
		.auction-tab{display:inline-flex;align-items:center;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
		.auction-tab.active{background:#214e34;color:#fff;border-color:#214e34}
		.auction-toolbar{display:grid;grid-template-columns:minmax(180px,.4fr) minmax(220px,.6fr) 120px;gap:10px;align-items:end}
		.auction-scroll{overflow:auto}
		.auction-table{min-width:980px}
		.auction-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:12px;background:#fff;color:#6a6257}
		.auction-message{padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34;margin:12px 0}
		.auction-actions{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:10px}
		.auction-actions form{margin:0}
		.auction-actions button{margin:0}
		@media (max-width:760px){.auction-toolbar{grid-template-columns:1fr}}
	</style>`)
	b.WriteString(`<section><h2>集合竞价</h2><p class="auction-muted">每日 09:30 抓取全市场 A 股 09:25 开盘集合竞价成交金额，历史数据来自 PostgreSQL 配置下的业务库。</p></section>`)
	if message := strings.TrimSpace(r.URL.Query().Get("msg")); message != "" {
		b.WriteString(`<div class="auction-message">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</div>`)
	}
	if err != nil {
		b.WriteString(`<section><div class="auction-empty">集合竞价数据读取失败：`)
		b.WriteString(html.EscapeString(err.Error()))
		b.WriteString(`</div></section>`)
		_ = s.writeSimplePage(w, "a-stock-auction", "A股集合竞价", b.String())
		return
	}
	renderAStockAuctionActions(&b)
	renderAStockAuctionSummary(&b, ctx)
	renderAStockAuctionFilters(&b, ctx)
	renderAStockAuctionTable(&b, ctx)
	_ = s.writeSimplePage(w, "a-stock-auction", "A股集合竞价", b.String())
}

func (s *Server) handleAStockAuctionAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	today := aStockNow().In(aStockLocation()).Format("2006-01-02")
	message := "未知操作"
	if strings.TrimSpace(r.FormValue("action")) == "fetch_today_auction" {
		message = s.triggerAStockAuctionCrawl()
	}
	query := url.Values{}
	query.Set("date", today)
	query.Set("msg", message)
	http.Redirect(w, r, "/a-stock/auction?"+query.Encode(), http.StatusSeeOther)
}

func (s *Server) loadAStockAuctionContext(date string, keyword string, page int) (model.AStockAuctionListResult, error) {
	query := url.Values{}
	query.Set("page", fmt.Sprintf("%d", page))
	query.Set("page_size", "50")
	if date != "" {
		query.Set("date", date)
	}
	if keyword != "" {
		query.Set("keyword", keyword)
	}
	var result model.AStockAuctionListResult
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/auction?"+query.Encode(), &result)
	return result, err
}

func renderAStockAuctionSummary(b *strings.Builder, ctx model.AStockAuctionListResult) {
	maxStock := "--"
	if ctx.MaxItem != nil {
		maxStock = strings.TrimSpace(ctx.MaxItem.Code + " " + ctx.MaxItem.Name)
		if maxStock == "" {
			maxStock = "--"
		}
	}
	fetchedAt := "--"
	if ctx.FetchedAt != nil && !ctx.FetchedAt.IsZero() {
		fetchedAt = ctx.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")
	}
	b.WriteString(`<section><h2>当日汇总</h2><div class="auction-grid">`)
	writeAStockAuctionMetric(b, "交易日", nonEmptyText(ctx.Date, "--"))
	writeAStockAuctionMetric(b, "股票数", fmt.Sprintf("%d", ctx.SummaryCount))
	writeAStockAuctionMetric(b, "集合竞价总金额", formatAStockAuctionMoney(ctx.TotalAmount))
	writeAStockAuctionMetric(b, "最大金额股票", maxStock)
	writeAStockAuctionMetric(b, "最近抓取时间", fetchedAt)
	b.WriteString(`</div></section>`)
}

func renderAStockAuctionActions(b *strings.Builder) {
	b.WriteString(`<section><h2>操作区</h2><div class="auction-actions"><form method="post"><input type="hidden" name="action" value="fetch_today_auction"><button type="submit">获取今日集合竞价金额</button></form></div><p class="auction-muted">立即触发 scheduler 的 A股集合竞价抓取任务，按服务器 Asia/Shanghai 今日日期从 AKShare 业务服务读取并写入当前业务库。</p></section>`)
}

func renderAStockAuctionFilters(b *strings.Builder, ctx model.AStockAuctionListResult) {
	b.WriteString(`<section><h2>历史查询</h2><div class="auction-tabs">`)
	for _, date := range ctx.Dates {
		b.WriteString(`<a class="auction-tab`)
		if date == ctx.Date {
			b.WriteString(` active`)
		}
		b.WriteString(`" href="/a-stock/auction?date=`)
		b.WriteString(url.QueryEscape(date))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(date))
		b.WriteString(`</a>`)
	}
	if len(ctx.Dates) == 0 {
		b.WriteString(`<span class="auction-muted">暂无历史日期</span>`)
	}
	b.WriteString(`</div><form method="get" class="auction-toolbar"><div><label>交易日</label><input type="date" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"></div><div><label>股票代码/名称</label><input name="keyword" placeholder="如 002230 或 科大讯飞" value="`)
	b.WriteString(html.EscapeString(ctx.Keyword))
	b.WriteString(`"></div><button type="submit">查询</button></form></section>`)
}

func renderAStockAuctionTable(b *strings.Builder, ctx model.AStockAuctionListResult) {
	b.WriteString(`<section><h2>集合竞价明细</h2><div class="auction-scroll"><table class="auction-table"><tr><th>日期</th><th>股票代码</th><th>股票名称</th><th>集合竞价价</th><th>成交量</th><th>成交额</th><th>来源</th><th>状态</th><th>抓取时间</th></tr>`)
	if len(ctx.Items) == 0 {
		b.WriteString(`<tr><td colspan="9">暂无集合竞价数据，请确认 09:30 抓取任务或 AKShare 服务。</td></tr>`)
	} else {
		for _, item := range ctx.Items {
			b.WriteString(`<tr><td>`)
			b.WriteString(html.EscapeString(item.TradeDate))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(item.Code))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(item.Name))
			b.WriteString(`</td><td>`)
			b.WriteString(formatAStockAuctionPrice(item.AuctionPrice))
			b.WriteString(`</td><td>`)
			b.WriteString(formatAStockAuctionVolume(item.AuctionVolume))
			b.WriteString(`</td><td>`)
			b.WriteString(formatAStockAuctionMoney(item.AuctionAmount))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(item.Source))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(nonEmptyText(item.Status, "--")))
			b.WriteString(`</td><td>`)
			b.WriteString(html.EscapeString(item.FetchedAt.In(aStockLocation()).Format("2006-01-02 15:04:05")))
			b.WriteString(`</td></tr>`)
		}
	}
	b.WriteString(`</table></div>`)
	renderAStockAuctionPagination(b, ctx)
	b.WriteString(`</section>`)
}

func renderAStockAuctionPagination(b *strings.Builder, ctx model.AStockAuctionListResult) {
	if ctx.PageSize <= 0 || ctx.Total <= ctx.PageSize {
		return
	}
	totalPages := (ctx.Total + ctx.PageSize - 1) / ctx.PageSize
	b.WriteString(`<div class="auction-tabs"><span class="auction-muted">第 `)
	b.WriteString(fmt.Sprintf("%d/%d 页，共 %d 条", ctx.Page, totalPages, ctx.Total))
	b.WriteString(`</span>`)
	for _, link := range []struct {
		Page  int
		Label string
	}{
		{Page: ctx.Page - 1, Label: "上一页"},
		{Page: ctx.Page + 1, Label: "下一页"},
	} {
		if link.Page < 1 || link.Page > totalPages {
			continue
		}
		b.WriteString(`<a class="auction-tab" href="/a-stock/auction?date=`)
		b.WriteString(url.QueryEscape(ctx.Date))
		if ctx.Keyword != "" {
			b.WriteString(`&keyword=`)
			b.WriteString(url.QueryEscape(ctx.Keyword))
		}
		b.WriteString(`&page=`)
		b.WriteString(fmt.Sprintf("%d", link.Page))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(link.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func writeAStockAuctionMetric(b *strings.Builder, label string, value string) {
	b.WriteString(`<div class="auction-card"><span class="auction-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong style="display:block;margin-top:8px;font-size:22px">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func formatAStockAuctionPrice(value float64) string {
	if value <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.2f", value)
}

func formatAStockAuctionVolume(value float64) string {
	if value <= 0 {
		return "--"
	}
	if math.Abs(value) >= 10000 {
		return fmt.Sprintf("%.2f万", value/10000)
	}
	return fmt.Sprintf("%.0f", value)
}

func formatAStockAuctionMoney(value float64) string {
	if value <= 0 {
		return "--"
	}
	if math.Abs(value) >= 100000000 {
		return fmt.Sprintf("%.2f亿", value/100000000)
	}
	return fmt.Sprintf("%.2f万", value/10000)
}

func nonEmptyText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) triggerAStockAuctionCrawl() string {
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/jobs/a-stock-auction-crawl/run")
	if err != nil {
		return "今日集合竞价获取失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		detail := schedulerAuctionErrorMessage(resp.Body(), resp.String())
		if detail == "" {
			detail = resp.Status()
		}
		return "今日集合竞价获取失败：" + detail
	}
	return "今日集合竞价获取任务已触发，请稍后刷新查看当日汇总和明细。"
}

func schedulerAuctionErrorMessage(body []byte, fallback string) string {
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && strings.TrimSpace(envelope.Message) != "" {
		message := strings.TrimSpace(envelope.Message)
		if strings.Contains(message, "a-stock-auction-crawl") && strings.Contains(message, "disabled") {
			return "集合竞价抓取任务未启用：请配置 YUQING_ASTOCK_AUCTION_URL 为 AKShare HTTP 服务地址，并重启 scheduler-service 后再点击获取。"
		}
		return message
	}
	return strings.TrimSpace(fallback)
}
