package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type cryptoDefaultCard struct {
	Pair      string
	Title     string
	Insight   model.CryptoInsightResponse
	Message   string
	UpdatedAt string
}

func (s *Server) handleCryptoPage(w http.ResponseWriter, r *http.Request, user any) {
	pair := strings.TrimSpace(r.URL.Query().Get("pair"))

	var b strings.Builder
	b.WriteString(`<section><p>欢迎，用户 `)
	b.WriteString(fmt.Sprintf("%d", userIDFromMap(user)))
	b.WriteString(`</p><form method="get" style="display:grid;grid-template-columns:2fr 1fr;gap:12px;align-items:end"><div><label>币对</label><input name="pair" value="`)
	b.WriteString(html.EscapeString(pair))
	b.WriteString(`" placeholder="BTCUSDT / BTC-USDT / ETH-USD"></div><div><button type="submit">查询</button></div></form><p style="color:#6a6257">默认展示 BTC 和 ETH 价格信息；输入其他币对后点击查询。</p></section>`)

	if pair == "" {
		renderDefaultCryptoCards(&b, s.loadDefaultCryptoCards())
		_ = s.writeSimplePage(w, "crypto", "Crypto Insights", b.String())
		return
	}

	insight, message := s.fetchCryptoInsight(pair)
	if message != "" {
		b.WriteString(`<section><p style="color:#8f2d2d">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p></section>`)
		_ = s.writeSimplePage(w, "crypto", "Crypto Insights", b.String())
		return
	}

	b.WriteString(`<section><p style="color:#6a6257">仅供信息参考，不构成投资建议。</p></section>`)

	hasPrice := insight.PriceSnapshot.Price > 0
	b.WriteString(`<section><h2>信号总览</h2><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px">`)
	writeMetricCard(&b, "当前价格", cryptoMetricValue(insight.PriceSnapshot.Price, hasPrice, ""))
	writeMetricCard(&b, "1h", cryptoMetricValue(insight.PriceSnapshot.Change1H, hasPrice, "%"))
	writeMetricCard(&b, "4h", cryptoMetricValue(insight.PriceSnapshot.Change4H, hasPrice, "%"))
	writeMetricCard(&b, "24h", cryptoMetricValue(insight.PriceSnapshot.Change24H, hasPrice, "%"))
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

	_ = s.writeSimplePage(w, "crypto", "Crypto Insights", b.String())
}

func (s *Server) fetchCryptoInsight(pair string) (model.CryptoInsightResponse, string) {
	var insight model.CryptoInsightResponse
	var envelope struct {
		Data model.CryptoInsightResponse `json:"data"`
	}
	resp, err := s.client.R().SetResult(&envelope).Get(s.cfg.AnalysisURL + "/api/v1/crypto/insights?pair=" + url.QueryEscape(pair))
	switch {
	case err != nil:
		return model.CryptoInsightResponse{}, "行情服务暂时不可用，请稍后重试"
	case !resp.IsSuccess():
		message := "行情服务暂时不可用，请稍后重试"
		var errEnvelope struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(resp.Body(), &errEnvelope) == nil && strings.TrimSpace(errEnvelope.Message) != "" {
			message = errEnvelope.Message
		}
		return model.CryptoInsightResponse{}, message
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
		return insight, ""
	}
}

func (s *Server) loadDefaultCryptoCards() []cryptoDefaultCard {
	pairs := []string{"BTCUSDT", "ETHUSDT"}
	cards := make([]cryptoDefaultCard, 0, len(pairs))
	for _, pair := range pairs {
		insight, message := s.fetchCryptoInsight(pair)
		title := pair
		updatedAt := ""
		if insight.Pair != "" {
			title = insight.Pair
			if insight.PriceSnapshot.UpdatedAt.IsZero() {
				updatedAt = insight.UpdatedAt.Format("2006-01-02 15:04:05")
			} else {
				updatedAt = insight.PriceSnapshot.UpdatedAt.Format("2006-01-02 15:04:05")
			}
		}
		cards = append(cards, cryptoDefaultCard{
			Pair:      pair,
			Title:     title,
			Insight:   insight,
			Message:   message,
			UpdatedAt: updatedAt,
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Pair < cards[j].Pair })
	return cards
}

func renderDefaultCryptoCards(b *strings.Builder, cards []cryptoDefaultCard) {
	b.WriteString(`<section><h2>默认价格看板</h2><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px">`)
	for _, card := range cards {
		b.WriteString(`<div style="padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#faf8f2">`)
		b.WriteString(`<h3 style="margin-top:0">`)
		b.WriteString(html.EscapeString(card.Title))
		b.WriteString(`</h3>`)
		if card.Message != "" {
			b.WriteString(`<p style="color:#8f2d2d">`)
			b.WriteString(html.EscapeString(card.Message))
			b.WriteString(`</p></div>`)
			continue
		}
		hasPrice := card.Insight.PriceSnapshot.Price > 0
		b.WriteString(`<div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px">`)
		writeMetricCard(b, "当前价格", cryptoMetricValue(card.Insight.PriceSnapshot.Price, hasPrice, ""))
		writeMetricCard(b, "1h", cryptoMetricValue(card.Insight.PriceSnapshot.Change1H, hasPrice, "%"))
		writeMetricCard(b, "4h", cryptoMetricValue(card.Insight.PriceSnapshot.Change4H, hasPrice, "%"))
		writeMetricCard(b, "24h", cryptoMetricValue(card.Insight.PriceSnapshot.Change24H, hasPrice, "%"))
		b.WriteString(`</div>`)
		if card.UpdatedAt != "" {
			b.WriteString(`<p style="color:#6a6257;margin-bottom:0">更新时间：`)
			b.WriteString(html.EscapeString(card.UpdatedAt))
			b.WriteString(`</p>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></section>`)
}

func writeMetricCard(b *strings.Builder, label, value string) {
	b.WriteString(`<div style="padding:16px;border:1px solid #ece7dc;border-radius:12px;background:#fff"><div style="color:#6a6257">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</div><strong style="display:block;font-size:26px;margin-top:6px">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func cryptoMetricValue(value float64, available bool, suffix string) string {
	if !available {
		return "N/A"
	}
	if suffix == "" {
		return fmt.Sprintf("%.4f", value)
	}
	return fmt.Sprintf("%.2f%s", value, suffix)
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
