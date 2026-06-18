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
		.auction-chart{width:100%;height:auto;min-height:260px}
		.auction-chart-line{fill:none;stroke:#214e34;stroke-width:3}
		.auction-chart-area{fill:rgba(33,78,52,.08)}
		.auction-chart-axis{stroke:#d6ccbb;stroke-width:1}
		.auction-chart-label{font-size:12px;fill:#6a6257}
		.auction-chart-dot{fill:#214e34}
		.auction-trend-table{margin-top:12px}
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
	renderAStockAuctionTrend(&b, ctx)
	renderAStockAuctionFilters(&b, ctx)
	renderAStockAuctionTable(&b, ctx)
	_ = s.writeSimplePage(w, "a-stock-auction", "A股集合竞价", b.String())
}

func (s *Server) handleAStockAuctionAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	today := aStockNow().In(aStockLocation()).Format("2006-01-02")
	message := "未知操作"
	action := strings.TrimSpace(r.FormValue("action"))
	switch action {
	case "fetch_today_auction":
		message = s.triggerAStockAuctionCrawl()
	case "backfill_30d_auction":
		message = s.triggerAStockAuctionBackfill(30)
	}
	query := url.Values{}
	if action != "fetch_today_auction" {
		query.Set("date", today)
	}
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
	b.WriteString(`<section><h2>操作区</h2><div class="auction-actions"><form method="post"><input type="hidden" name="action" value="fetch_today_auction"><button type="submit">获取最新交易日集合竞价金额</button></form><form method="post"><input type="hidden" name="action" value="backfill_30d_auction"><button type="submit">回溯近30天集合竞价</button></form></div><p class="auction-muted">立即触发 scheduler 的 A股集合竞价抓取任务，从 AKShare 业务服务读取最新交易日并写入当前业务库。若 09:30 定时任务漏抓，仍可点击“获取最新交易日集合竞价金额”补抓；更早历史交易日只能读取已有缓存或业务库记录。</p></section>`)
}

func renderAStockAuctionTrend(b *strings.Builder, ctx model.AStockAuctionListResult) {
	b.WriteString(`<section><h2>近30日资金趋势</h2>`)
	if len(ctx.Trend) == 0 {
		b.WriteString(`<div class="auction-empty">暂无趋势数据。请先点击“回溯近30天集合竞价”，或检查 YUQING_ASTOCK_AUCTION_URL 指向的 AKShare 业务服务。</div></section>`)
		return
	}
	b.WriteString(`<p class="auction-muted">折线按每日集合竞价总成交额绘制，明细表同步展示总成交量。</p>`)
	b.WriteString(aStockAuctionTrendSVG(ctx.Trend))
	b.WriteString(`<div class="auction-scroll"><table class="auction-table auction-trend-table"><tr><th>日期</th><th>股票数</th><th>集合竞价总金额</th><th>成交量</th><th>最大金额股票</th></tr>`)
	start := max(len(ctx.Trend)-8, 0)
	for _, point := range ctx.Trend[start:] {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(point.Date))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", point.StockCount))
		b.WriteString(`</td><td>`)
		b.WriteString(formatAStockAuctionMoney(point.TotalAmount))
		b.WriteString(`</td><td>`)
		b.WriteString(formatAStockAuctionVolume(point.TotalVolume))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(nonEmptyText(strings.TrimSpace(point.MaxStockCode+" "+point.MaxStockName), "--")))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></section>`)
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

func aStockAuctionTrendSVG(points []model.AStockAuctionTrend) string {
	const (
		width  = 1120.0
		height = 280.0
		left   = 56.0
		right  = 24.0
		top    = 24.0
		bottom = 44.0
	)
	maxAmount := 0.0
	for _, point := range points {
		if point.TotalAmount > maxAmount {
			maxAmount = point.TotalAmount
		}
	}
	if maxAmount <= 0 {
		return `<div class="auction-empty">近30日趋势金额均为空，请确认 AKShare 返回了成交额字段。</div>`
	}
	plotWidth := width - left - right
	plotHeight := height - top - bottom
	coords := make([]string, 0, len(points))
	area := make([]string, 0, len(points)+2)
	for i, point := range points {
		x := left
		if len(points) > 1 {
			x += float64(i) * plotWidth / float64(len(points)-1)
		}
		y := top + (1-point.TotalAmount/maxAmount)*plotHeight
		coords = append(coords, fmt.Sprintf("%.1f,%.1f", x, y))
		area = append(area, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	area = append([]string{fmt.Sprintf("%.1f,%.1f", left, top+plotHeight)}, area...)
	lastX := left
	if len(points) > 1 {
		lastX += plotWidth
	}
	area = append(area, fmt.Sprintf("%.1f,%.1f", lastX, top+plotHeight))
	var b strings.Builder
	b.WriteString(`<svg class="auction-chart" viewBox="0 0 1120 280" role="img" aria-label="近30日集合竞价资金趋势">`)
	for i := 0; i <= 4; i++ {
		y := top + float64(i)*plotHeight/4
		b.WriteString(`<line class="auction-chart-axis" x1="`)
		b.WriteString(fmt.Sprintf("%.1f", left))
		b.WriteString(`" y1="`)
		b.WriteString(fmt.Sprintf("%.1f", y))
		b.WriteString(`" x2="`)
		b.WriteString(fmt.Sprintf("%.1f", left+plotWidth))
		b.WriteString(`" y2="`)
		b.WriteString(fmt.Sprintf("%.1f", y))
		b.WriteString(`"></line>`)
	}
	b.WriteString(`<polygon class="auction-chart-area" points="`)
	b.WriteString(strings.Join(area, " "))
	b.WriteString(`"></polygon><polyline class="auction-chart-line" points="`)
	b.WriteString(strings.Join(coords, " "))
	b.WriteString(`"></polyline>`)
	for i, point := range points {
		if i != 0 && i != len(points)-1 && i%5 != 0 {
			continue
		}
		x := left
		if len(points) > 1 {
			x += float64(i) * plotWidth / float64(len(points)-1)
		}
		y := top + (1-point.TotalAmount/maxAmount)*plotHeight
		b.WriteString(`<circle class="auction-chart-dot" cx="`)
		b.WriteString(fmt.Sprintf("%.1f", x))
		b.WriteString(`" cy="`)
		b.WriteString(fmt.Sprintf("%.1f", y))
		b.WriteString(`" r="4"><title>`)
		b.WriteString(html.EscapeString(point.Date + " " + formatAStockAuctionMoney(point.TotalAmount)))
		b.WriteString(`</title></circle>`)
	}
	if len(points) > 0 {
		b.WriteString(`<text class="auction-chart-label" x="`)
		b.WriteString(fmt.Sprintf("%.1f", left))
		b.WriteString(`" y="268">`)
		b.WriteString(html.EscapeString(points[0].Date))
		b.WriteString(`</text><text class="auction-chart-label" text-anchor="end" x="`)
		b.WriteString(fmt.Sprintf("%.1f", left+plotWidth))
		b.WriteString(`" y="268">`)
		b.WriteString(html.EscapeString(points[len(points)-1].Date))
		b.WriteString(`</text><text class="auction-chart-label" x="`)
		b.WriteString(fmt.Sprintf("%.1f", left))
		b.WriteString(`" y="18">最高 `)
		b.WriteString(html.EscapeString(formatAStockAuctionMoney(maxAmount)))
		b.WriteString(`</text>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
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
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/auction/latest")
	if err != nil {
		return "最新交易日集合竞价获取失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		detail := schedulerAuctionErrorMessage(resp.Body(), resp.String())
		if detail == "" {
			detail = resp.Status()
		}
		return "最新交易日集合竞价获取失败：" + detail
	}
	result := decodeSchedulerAuctionLatestResult(resp.Body())
	if result.Date != "" {
		return fmt.Sprintf("最新交易日集合竞价已写入：%s，明细 %d 条，有效 %d 条。", result.Date, result.Total, result.OK)
	}
	return "最新交易日集合竞价获取任务已触发，请稍后刷新查看当日汇总和明细。"
}

func (s *Server) triggerAStockAuctionBackfill(days int) string {
	if days <= 0 {
		days = 30
	}
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/auction/backfill?days=" + fmt.Sprintf("%d", days))
	if err != nil {
		return "集合竞价回溯失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		detail := schedulerAuctionErrorMessage(resp.Body(), resp.String())
		if detail == "" {
			detail = resp.Status()
		}
		return "集合竞价回溯失败：" + detail
	}
	return fmt.Sprintf("近%d天集合竞价回溯任务已触发，请稍后刷新查看资金趋势。", days)
}

func (s *Server) triggerAStockAuctionBackfillDate(date string) string {
	date = normalizeAStockStrategyDate(date)
	if date == "" {
		return "集合竞价补录失败：策略日期无效。"
	}
	query := url.Values{}
	query.Set("days", "1")
	query.Set("start", date)
	query.Set("end", date)
	resp, err := s.client.R().
		SetHeader("X-Service-Token", s.cfg.ServiceToken).
		Post(s.cfg.SchedulerURL + "/api/v1/scheduler/a-stock/auction/backfill?" + query.Encode())
	if err != nil {
		return "集合竞价补录失败：" + err.Error()
	}
	if !resp.IsSuccess() {
		detail := schedulerAuctionErrorMessage(resp.Body(), resp.String())
		if detail == "" {
			detail = resp.Status()
		}
		if isAStockAuctionHistoryUnavailableMessage(detail) {
			return fmt.Sprintf("%s 集合竞价无可补录数据：AKShare 适配服务没有该交易日缓存，历史集合竞价金额保持 --；推荐股票仍按已入库新闻和行情生成。需要显示真实金额时，请导入该日 09:30 抓取缓存或业务库记录。", date)
		}
		return "集合竞价补录失败：" + detail
	}
	result := decodeSchedulerAuctionBackfillResult(resp.Body())
	if result.Succeeded > 0 {
		return fmt.Sprintf("已补录 %s 集合竞价：成功 %d 天，跳过 %d 天，失败 %d 天。请重新生成推荐。", date, result.Succeeded, result.Skipped, result.Failed)
	}
	if result.Skipped > 0 || result.Failed > 0 {
		return fmt.Sprintf("%s 集合竞价未写入：跳过 %d 天，失败 %d 天。%s", date, result.Skipped, result.Failed, strings.Join(result.Errors, "；"))
	}
	return fmt.Sprintf("%s 集合竞价补录任务已触发，请稍后刷新后重新生成推荐。", date)
}

func isAStockAuctionHistoryUnavailableMessage(message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	return (strings.Contains(message, "skipped all dates without usable data") ||
		strings.Contains(message, "only serves the latest trading day") ||
		strings.Contains(message, "only serves the current trading day")) &&
		strings.Contains(message, "usable local cache")
}

func decodeSchedulerAuctionBackfillResult(body []byte) struct {
	Succeeded int
	Skipped   int
	Failed    int
	Errors    []string
} {
	var envelope struct {
		Data struct {
			Result struct {
				Succeeded int      `json:"succeeded"`
				Skipped   int      `json:"skipped"`
				Failed    int      `json:"failed"`
				Errors    []string `json:"errors"`
			} `json:"result"`
		} `json:"data"`
	}
	var result struct {
		Succeeded int
		Skipped   int
		Failed    int
		Errors    []string
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		result.Succeeded = envelope.Data.Result.Succeeded
		result.Skipped = envelope.Data.Result.Skipped
		result.Failed = envelope.Data.Result.Failed
		result.Errors = envelope.Data.Result.Errors
	}
	return result
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
		if strings.Contains(message, "YUQING_ASTOCK_AUCTION_URL not configured") {
			return "集合竞价抓取服务未配置：请配置 YUQING_ASTOCK_AUCTION_URL 为 AKShare HTTP 服务地址，并重启 scheduler-service 后再点击获取。"
		}
		if strings.Contains(message, "skipped all dates without usable data") {
			return "集合竞价回溯未写入数据：最新交易日可点击“获取最新交易日集合竞价金额”补抓；更早历史交易日需要依赖过去每日抓取形成的缓存/业务库记录。原始信息：" + message
		}
		if strings.Contains(message, "only serves the current trading day") {
			return "集合竞价历史日期无法实时回抓：当前 AKShare 接口只支持最新交易日，旧交易日只能读取已有缓存；如是 09:30 漏抓，请点击“获取最新交易日集合竞价金额”。"
		}
		if strings.Contains(message, "no usable auction amounts") || strings.Contains(message, "有效成交额/成交量为 0") {
			return "集合竞价抓取未写入：AKShare 返回了明细但成交额/成交量均为 0，请稍后重试或检查 AKShare 数据源。"
		}
		return message
	}
	return strings.TrimSpace(fallback)
}

type schedulerAuctionLatestResult struct {
	Date  string `json:"date"`
	Total int    `json:"total"`
	OK    int    `json:"ok"`
}

func decodeSchedulerAuctionLatestResult(body []byte) schedulerAuctionLatestResult {
	var envelope struct {
		Data struct {
			Result schedulerAuctionLatestResult `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return schedulerAuctionLatestResult{}
	}
	return envelope.Data.Result
}
