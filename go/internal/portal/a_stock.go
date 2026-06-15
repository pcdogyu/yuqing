package portal

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	message := strings.TrimSpace(r.URL.Query().Get("msg"))

	var b strings.Builder
	b.WriteString(`<style>
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
	writeAStockMetric(&b, "策略日期", strategyDate)
	writeAStockMetric(&b, "新闻窗口", "09:00-09:25")
	writeAStockMetric(&b, "候选热点数", "0")
	writeAStockMetric(&b, "推荐股票数", "0")
	writeAStockMetric(&b, "回测状态", "等待回测")
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
		b.WriteString(html.EscapeString(strategyDate))
		b.WriteString(`"><input type="hidden" name="action" value="`)
		b.WriteString(html.EscapeString(action.Name))
		b.WriteString(`"><button type="submit">`)
		b.WriteString(html.EscapeString(action.Label))
		b.WriteString(`</button></form>`)
	}
	b.WriteString(`</div><p class="astock-muted">第一版仅提供页面承载入口，后端策略任务、Tushare 行情和回测表将在后续批次接入。</p></section>`)

	renderAStockNewsSection(&b)
	renderAStockHotspotSection(&b)
	renderAStockRecommendationSection(&b)
	renderAStockBacktestSection(&b)

	_ = s.writeSimplePage(w, "a-stock", "A股策略工作台", b.String())
}

func (s *Server) handleAStockPageAction(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	query := url.Values{}
	if date := normalizeAStockStrategyDate(r.FormValue("date")); date != "" {
		query.Set("date", date)
	}
	query.Set("msg", "A股策略后台接口待接入，当前仅展示页面入口。")
	http.Redirect(w, r, "/a-stock?"+query.Encode(), http.StatusSeeOther)
}

func writeAStockMetric(b *strings.Builder, label string, value string) {
	b.WriteString(`<div class="astock-card"><span class="astock-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong style="display:block;margin-top:8px;font-size:24px">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func renderAStockNewsSection(b *strings.Builder) {
	b.WriteString(`<section><h2>09:00-09:25 财经新闻</h2><div class="astock-empty">暂无数据：等待抓取 A 股财经新闻。后续会展示标题、来源、时间和命中关键词。</div><table><tr><th>标题</th><th>来源</th><th>时间</th><th>命中关键词</th></tr><tr><td colspan="4">暂无 09:00-09:25 新闻</td></tr></table></section>`)
}

func renderAStockHotspotSection(b *strings.Builder) {
	b.WriteString(`<section><h2>热点归纳</h2><p><span class="astock-badge">规则+词典</span><span class="astock-badge">可复现回测</span></p><div class="astock-empty">暂无数据：等待生成热点。后续会展示热点名称、关键词、热度分和证据新闻数。</div><table><tr><th>热点</th><th>关键词</th><th>热度分</th><th>证据新闻数</th></tr><tr><td colspan="4">暂无热点</td></tr></table></section>`)
}

func renderAStockRecommendationSection(b *strings.Builder) {
	b.WriteString(`<section><h2>推荐股票</h2><div class="astock-empty">暂无数据：等待热点映射股票。后续会按热点展示股票代码、名称、推荐理由和排名。</div><table><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>推荐理由</th></tr><tr><td colspan="5">暂无推荐股票</td></tr></table></section>`)
}

func renderAStockBacktestSection(b *strings.Builder) {
	b.WriteString(`<section><h2>消息回测</h2><p class="astock-muted">买入价采用当日开盘价；T+1 到 T+5 按后续交易日收盘价计算收益，并展示五日内最高收益。</p><div class="astock-scroll"><table class="astock-table"><tr><th>股票</th><th>当日开盘价</th><th>T+1 收盘价</th><th>T+1 收益</th><th>T+2 收盘价</th><th>T+2 收益</th><th>T+3 收盘价</th><th>T+3 收益</th><th>T+4 收盘价</th><th>T+4 收益</th><th>T+5 收盘价</th><th>T+5 收益</th><th>五日内最高收益</th><th>命中状态</th></tr><tr><td colspan="14">暂无回测结果，等待 Tushare 行情同步。</td></tr></table></div></section>`)
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
