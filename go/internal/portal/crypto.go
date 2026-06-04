package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Server) handleCryptoPage(w http.ResponseWriter, r *http.Request, user any) {
	pair := strings.TrimSpace(r.URL.Query().Get("pair"))
	if pair == "" {
		pair = "BTCUSDT"
	}

	var insight model.CryptoInsightResponse
	var message string
	var envelope struct {
		Data model.CryptoInsightResponse `json:"data"`
	}
	resp, err := s.client.R().SetResult(&envelope).Get(s.cfg.AnalysisURL + "/api/v1/crypto/insights?pair=" + url.QueryEscape(pair))
	switch {
	case err != nil:
		message = err.Error()
	case !resp.IsSuccess():
		message = resp.Status()
	default:
		insight = envelope.Data
		if insight.Pair == "" {
			var raw struct {
				Data model.CryptoInsightResponse `json:"data"`
			}
			if json.Unmarshal(resp.Body(), &raw) == nil {
				insight = raw.Data
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<header style="max-width:1100px;margin:0 auto;padding:24px 24px 0"><nav style="display:flex;gap:16px;flex-wrap:wrap"><a href="/">总览</a><a href="/projects">项目</a><a href="/articles">文章</a><a href="/reports">报告</a><a href="/system">系统</a><a href="/logout">退出</a></nav></header>`)
	b.WriteString(`<section><h1>Crypto Insights</h1><p>欢迎，用户 `)
	b.WriteString(fmt.Sprintf("%d", userIDFromMap(user)))
	b.WriteString(`</p><form method="get" style="display:grid;grid-template-columns:2fr 1fr;gap:12px;align-items:end"><div><label>币对</label><input name="pair" value="`)
	b.WriteString(html.EscapeString(pair))
	b.WriteString(`" placeholder="BTCUSDT / BTC-USDT / ETH-USD"></div><div><button type="submit">查询</button></div></form>`)
	if message != "" {
		b.WriteString(`<p style="color:#8f2d2d">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p>`)
	} else {
		b.WriteString(`<p style="color:#6a6257">仅供信息参考，不构成投资建议。</p>`)
	}
	b.WriteString(`</section>`)

	if message == "" {
		b.WriteString(`<section><h2>信号总览</h2><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px">`)
		writeMetricCard(&b, "当前价格", fmt.Sprintf("%.2f", insight.PriceSnapshot.Price))
		writeMetricCard(&b, "1h", fmt.Sprintf("%.2f%%", insight.PriceSnapshot.Change1H))
		writeMetricCard(&b, "4h", fmt.Sprintf("%.2f%%", insight.PriceSnapshot.Change4H))
		writeMetricCard(&b, "24h", fmt.Sprintf("%.2f%%", insight.PriceSnapshot.Change24H))
		writeMetricCard(&b, "社媒热度", fmt.Sprintf("%d 条", insight.SocialSentiment.EvidenceCount))
		b.WriteString(`</div></section>`)

		b.WriteString(`<section><h2>走势预判</h2><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px">`)
		writeSignalCard(&b, insight.Signals.H4)
		writeSignalCard(&b, insight.Signals.H24)
		b.WriteString(`</div></section>`)

		b.WriteString(`<section><h2>社媒情绪</h2><div style="padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#f5f1e8">`)
		b.WriteString(`<p>方向：`)
		b.WriteString(html.EscapeString(insight.SocialSentiment.Direction))
		b.WriteString(` | 置信度：`)
		b.WriteString(fmt.Sprintf("%.2f", insight.SocialSentiment.Confidence))
		b.WriteString(` | 热度：`)
		b.WriteString(fmt.Sprintf("%.2f", insight.SocialSentiment.VolumeScore))
		b.WriteString(`</p><p>bullish `)
		b.WriteString(fmt.Sprintf("%.2f", insight.SocialSentiment.Bullish))
		b.WriteString(` / neutral `)
		b.WriteString(fmt.Sprintf("%.2f", insight.SocialSentiment.Neutral))
		b.WriteString(` / bearish `)
		b.WriteString(fmt.Sprintf("%.2f", insight.SocialSentiment.Bearish))
		b.WriteString(`</p><p>`)
		b.WriteString(html.EscapeString(insight.SocialSentiment.Summary))
		b.WriteString(`</p></div></section>`)

		b.WriteString(`<section><h2>Top Reasons</h2><table><tr><th>分类</th><th>方向</th><th>分数</th><th>证据数</th><th>摘要</th></tr>`)
		for _, reason := range insight.TopReasons {
			b.WriteString("<tr><td>")
			b.WriteString(html.EscapeString(reason.Label))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(reason.Direction))
			b.WriteString("</td><td>")
			b.WriteString(fmt.Sprintf("%.2f", reason.Score))
			b.WriteString("</td><td>")
			b.WriteString(fmt.Sprintf("%d", reason.EvidenceCount))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(reason.Summary))
			b.WriteString("</td></tr>")
		}
		if len(insight.TopReasons) == 0 {
			b.WriteString(`<tr><td colspan="5">暂无原因归纳</td></tr>`)
		}
		b.WriteString(`</table></section>`)

		b.WriteString(`<section><h2>Evidence News</h2><table><tr><th>标题</th><th>方向</th><th>原因</th><th>来源</th><th>时间</th></tr>`)
		for _, item := range insight.EvidenceArticles {
			b.WriteString("<tr><td>")
			target := firstNonEmpty(item.SourceURL, item.DetailURL)
			if target != "" {
				b.WriteString(`<a href="`)
				b.WriteString(html.EscapeString(target))
				b.WriteString(`" target="_blank" rel="noreferrer">`)
			}
			b.WriteString(html.EscapeString(item.Title))
			if target != "" {
				b.WriteString(`</a>`)
			}
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.Direction))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.ReasonLabel))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.SourceType))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.PublishTime))
			b.WriteString("</td></tr>")
		}
		if len(insight.EvidenceArticles) == 0 {
			b.WriteString(`<tr><td colspan="5">暂无相关消息</td></tr>`)
		}
		b.WriteString(`</table></section>`)

		b.WriteString(`<section><h2>Social Evidence</h2><table><tr><th>平台</th><th>内容</th><th>方向</th><th>原因</th><th>热度</th><th>时间</th></tr>`)
		for _, item := range insight.SocialPosts {
			b.WriteString("<tr><td>")
			b.WriteString(html.EscapeString(item.Platform))
			b.WriteString("</td><td>")
			target := firstNonEmpty(item.SourceURL, item.DetailURL)
			if target != "" {
				b.WriteString(`<a href="`)
				b.WriteString(html.EscapeString(target))
				b.WriteString(`" target="_blank" rel="noreferrer">`)
			}
			b.WriteString(html.EscapeString(firstNonEmpty(item.Title, item.Content)))
			if target != "" {
				b.WriteString(`</a>`)
			}
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.Direction))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.ReasonLabel))
			b.WriteString("</td><td>")
			b.WriteString(fmt.Sprintf("%.2f", item.HeatScore))
			b.WriteString("</td><td>")
			b.WriteString(html.EscapeString(item.PublishTime))
			b.WriteString("</td></tr>")
		}
		if len(insight.SocialPosts) == 0 {
			b.WriteString(`<tr><td colspan="6">暂无相关社媒讨论</td></tr>`)
		}
		b.WriteString(`</table></section>`)

		b.WriteString(`<section><h2>AI 解读</h2><p>`)
		b.WriteString(html.EscapeString(insight.AIExplanation))
		b.WriteString(`</p><p style="color:#6a6257">更新时间：`)
		b.WriteString(html.EscapeString(insight.UpdatedAt.Format("2006-01-02 15:04:05")))
		b.WriteString(`，缓存 `)
		b.WriteString(fmt.Sprintf("%d", insight.CacheTTLSeconds))
		b.WriteString(` 秒。</p></section>`)
	}

	_ = s.writeSimplePage(w, "crypto", "Crypto Insights", b.String())
}

func writeMetricCard(b *strings.Builder, label, value string) {
	b.WriteString(`<div style="padding:16px;border:1px solid #ece7dc;border-radius:12px;background:#faf8f2"><div style="color:#6a6257">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</div><strong style="display:block;font-size:26px;margin-top:6px">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func writeSignalCard(b *strings.Builder, signal model.CryptoSignal) {
	b.WriteString(`<div style="padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#fff7ea"><h3 style="margin-top:0">`)
	b.WriteString(html.EscapeString(signal.Horizon))
	b.WriteString(` 预测</h3><p>方向：`)
	b.WriteString(html.EscapeString(signal.Direction))
	b.WriteString(` | 置信度：`)
	b.WriteString(fmt.Sprintf("%.2f", signal.Confidence))
	b.WriteString(`</p><p>bullish `)
	b.WriteString(fmt.Sprintf("%.2f", signal.Bullish))
	b.WriteString(` / neutral `)
	b.WriteString(fmt.Sprintf("%.2f", signal.Neutral))
	b.WriteString(` / bearish `)
	b.WriteString(fmt.Sprintf("%.2f", signal.Bearish))
	b.WriteString(`</p><p>消息 `)
	b.WriteString(fmt.Sprintf("%.2f", signal.NewsScore))
	b.WriteString(` / 社媒 `)
	b.WriteString(fmt.Sprintf("%.2f", signal.SocialScore))
	b.WriteString(` / 价格 `)
	b.WriteString(fmt.Sprintf("%.2f", signal.PriceScore))
	b.WriteString(`</p><p>`)
	b.WriteString(html.EscapeString(signal.Explanation))
	b.WriteString(`</p></div>`)
}
