package stockresearch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	SourceStatusParsed     = "parsed"
	SourceStatusNoSource   = "no_source"
	SourceStatusFailed     = "failed"
	SourceStatusPendingPDF = "pending_pdf"
)

const defaultSourceMaxBytes = 5 * 1024 * 1024

var blankLinePattern = regexp.MustCompile(`\n{3,}`)

type FetchOptions struct {
	UserAgent string
	Timeout   time.Duration
	MaxBytes  int64
	Now       func() time.Time
}

func FetchSource(ctx context.Context, rawURL string, opts FetchOptions) model.StockResearchSourceUpdate {
	now := sourceNow(opts).UTC().Format(time.RFC3339)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusNoSource, SourceFetchError: "原文链接为空", SourceFetchedAt: now}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: "原文链接无效", SourceFetchedAt: now}
	}
	if LooksLikePDFURL(rawURL) {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusPendingPDF, SourceFetchedAt: now}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: err.Error(), SourceFetchedAt: now}
	}
	if ua := strings.TrimSpace(opts.UserAgent); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	client := http.Client{Timeout: sourceTimeout(opts)}
	resp, err := client.Do(req)
	if err != nil {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: err.Error(), SourceFetchedAt: now}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: "原文请求失败：" + resp.Status, SourceFetchedAt: now}
	}
	maxBytes := sourceMaxBytes(opts)
	if resp.ContentLength > maxBytes {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: fmt.Sprintf("原文过大：%d bytes", resp.ContentLength), SourceFetchedAt: now}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: err.Error(), SourceFetchedAt: now}
	}
	if int64(len(data)) > maxBytes {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: fmt.Sprintf("原文过大：超过 %d bytes", maxBytes), SourceFetchedAt: now}
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(contentType, "pdf") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("%PDF")) {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusPendingPDF, SourceFetchedAt: now}
	}
	reader, err := charset.NewReader(bytes.NewReader(data), contentType)
	if err != nil {
		reader = bytes.NewReader(data)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: err.Error(), SourceFetchedAt: now}
	}
	text := ExtractHTMLSourceText(string(decoded))
	text = CleanSourceTextForURL(rawURL, text)
	if text == "" {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: "未提取到正文", SourceFetchedAt: now}
	}
	return model.StockResearchSourceUpdate{SourceText: text, SourceFetchStatus: SourceStatusParsed, SourceFetchedAt: now}
}

func SourceUpdateFromPDFText(text string, fetchedAt string) model.StockResearchSourceUpdate {
	text = FormatPDFText(text)
	if fetchedAt == "" {
		fetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if text == "" {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: "PDF 文本为空", SourceFetchedAt: fetchedAt}
	}
	return model.StockResearchSourceUpdate{SourceText: text, SourceFetchStatus: SourceStatusParsed, SourceFetchedAt: fetchedAt}
}

func FormatPDFText(raw string) string {
	return NormalizePlainText(raw)
}

func LooksLikePDFURL(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return false
	}
	if idx := strings.Index(raw, "?"); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.HasSuffix(raw, ".pdf")
}

func ExtractHTMLSourceText(rawHTML string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return ""
	}
	insertLineBreakTextNodes(doc.Selection)
	doc.Find("script,style,noscript,svg,canvas,iframe,nav,footer,header,form,button").Remove()
	root := bestSourceRoot(doc)
	blocks := make([]string, 0)
	root.Find("h1,h2,h3,h4,h5,h6,p,li,blockquote,pre,tr").Each(func(_ int, sel *goquery.Selection) {
		if goquery.NodeName(sel) == "tr" {
			cells := make([]string, 0)
			sel.Find("th,td").Each(func(_ int, cell *goquery.Selection) {
				if text := NormalizeInlineText(cell.Text()); text != "" {
					cells = append(cells, text)
				}
			})
			if len(cells) > 0 {
				blocks = append(blocks, strings.Join(cells, "    "))
			}
			return
		}
		text := NormalizePlainText(sel.Text())
		if text != "" {
			blocks = append(blocks, text)
		}
	})
	if len(blocks) == 0 {
		return NormalizePlainText(root.Text())
	}
	return FormatSourceBlocks(blocks)
}

func insertLineBreakTextNodes(root *goquery.Selection) {
	root.Find("br").Each(func(_ int, sel *goquery.Selection) {
		for _, node := range sel.Nodes {
			if node.Parent == nil {
				continue
			}
			node.Parent.InsertBefore(&xhtml.Node{Type: xhtml.TextNode, Data: "\n"}, node)
		}
	})
}

func FormatSourceBlocks(blocks []string) string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if text := NormalizePlainText(block); text != "" {
			out = append(out, text)
		}
	}
	return blankLinePattern.ReplaceAllString(strings.TrimSpace(strings.Join(out, "\n\n")), "\n\n")
}

func CleanSourceTextForURL(rawURL string, text string) string {
	if IsEastMoneyReportURL(rawURL) {
		return CleanEastMoneyReportText(text)
	}
	if IsSinaFinanceReportURL(rawURL) {
		return CleanSinaFinanceReportText(text)
	}
	return NormalizePlainText(text)
}

func IsEastMoneyReportURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	return host == "data.eastmoney.com" && strings.Contains(path, "/report/info/")
}

func IsSinaFinanceReportURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	return host == "stock.finance.sina.com.cn" &&
		strings.Contains(path, "/stock/go.php/vreport_show/")
}

func CleanSinaFinanceReportText(text string) string {
	blocks := sourceTextBlocks(text)
	for len(blocks) > 0 && sinaFinanceReportLeadingNoise[blocks[0]] {
		blocks = blocks[1:]
	}
	for i, block := range blocks {
		if block == "数据推荐" {
			blocks = blocks[:i]
			break
		}
	}
	return FormatSourceBlocks(blocks)
}

func CleanEastMoneyReportText(text string) string {
	normalized := NormalizePlainText(text)
	if normalized == "" {
		return ""
	}
	loc := eastMoneyReportInvestmentPointsPattern.FindStringIndex(normalized)
	if loc == nil {
		cleaned := normalized
		if cut := eastMoneyReportTailIndex(cleaned); cut >= 0 {
			cleaned = cleaned[:cut]
		}
		return FormatSourceBlocks(sourceTextBlocks(cleaned))
	}
	cleaned := normalized[loc[0]:]
	cleaned = eastMoneyReportInvestmentPointsAtStartPattern.ReplaceAllString(cleaned, `${1} 投资要点`)
	if cut := eastMoneyReportTailIndex(cleaned); cut >= 0 {
		cleaned = cleaned[:cut]
	}
	return FormatSourceBlocks(sourceTextBlocks(cleaned))
}

func eastMoneyReportTailIndex(text string) int {
	if loc := eastMoneyReportTailPattern.FindStringIndex(text); loc != nil {
		return loc[0]
	}
	cut := -1
	for _, marker := range eastMoneyReportTailMarkers {
		idx := strings.Index(text, marker)
		if idx >= 0 && (cut < 0 || idx < cut) {
			cut = idx
		}
	}
	return cut
}

func sourceTextBlocks(text string) []string {
	normalized := NormalizePlainText(text)
	if normalized == "" {
		return nil
	}
	rawBlocks := strings.Split(normalized, "\n\n")
	blocks := make([]string, 0, len(rawBlocks))
	for _, block := range rawBlocks {
		if cleaned := NormalizePlainText(block); cleaned != "" {
			blocks = append(blocks, cleaned)
		}
	}
	return blocks
}

var sinaFinanceReportLeadingNoise = map[string]bool{
	"研究报告":  true,
	"最新滚动":  true,
	"主力动向":  true,
	"个股评级":  true,
	"公司研究":  true,
	"行业研究":  true,
	"投资策略":  true,
	"宏观研究":  true,
	"金麒麟研报": true,
	"更多":    true,
	"晨报":    true,
	"创业板":   true,
	"基金":    true,
	"债券":    true,
	"金融工程":  true,
	"个股点评":  true,
}

var eastMoneyReportInvestmentPointsPattern = regexp.MustCompile(`[*\p{Han}A-Za-z0-9Ａ-Ｚａ-ｚ·-]+[（(]\d{6}[）)]\s*投资要点`)

var eastMoneyReportInvestmentPointsAtStartPattern = regexp.MustCompile(`^([*\p{Han}A-Za-z0-9Ａ-Ｚａ-ｚ·-]+[（(]\d{6}[）)])\s*投资要点`)

var eastMoneyReportTailPattern = regexp.MustCompile(`调\s*高\s*投\s*资\s*评\s*级|调\s*低\s*投\s*资\s*评\s*级|首\s*次\s*评\s*级\s*股\s*票|盈\s*利\s*预\s*测\s*排\s*行|最\s*新\s*研\s*究\s*报\s*告|买\s*入\s*评\s*级\s*个\s*股|数\s*据\s*来\s*源\s*：\s*东\s*方\s*财\s*富\s*Choice\s*数\s*据|郑\s*重\s*声\s*明\s*：\s*东\s*方\s*财\s*富\s*网\s*发\s*布\s*此\s*信\s*息|东\s*方\s*财\s*富\s*免\s*费\s*版`)

var eastMoneyReportTailMarkers = []string{
	"调高投资评级",
	"调低投资评级",
	"首次评级股票",
	"盈利预测排行",
	"最新研究报告",
	"买入评级个股",
	"数据来源：东方财富Choice数据",
	"郑重声明：东方财富网发布此信息",
	"东方财富免费版",
}

func NormalizePlainText(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			if blank <= 1 {
				out = append(out, "")
			}
			continue
		}
		blank = 0
		out = append(out, NormalizeInlineText(line))
	}
	return blankLinePattern.ReplaceAllString(strings.TrimSpace(strings.Join(out, "\n")), "\n\n")
}

func NormalizeInlineText(raw string) string {
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	raw = strings.ReplaceAll(raw, "\t", "    ")
	raw = strings.ReplaceAll(raw, "\f", " ")
	raw = strings.ReplaceAll(raw, "\v", " ")
	return strings.TrimSpace(raw)
}

func bestSourceRoot(doc *goquery.Document) *goquery.Selection {
	candidates := []string{"article", ".article", ".article-content", ".content", ".main-content", ".main", "#content", "body"}
	var best *goquery.Selection
	bestLen := -1
	for _, selector := range candidates {
		doc.Find(selector).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
			length := len([]rune(strings.TrimSpace(sel.Text())))
			if length > bestLen {
				best = sel
				bestLen = length
			}
			return true
		})
	}
	if best != nil {
		return best
	}
	return doc.Selection
}

func sourceTimeout(opts FetchOptions) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return 15 * time.Second
}

func sourceMaxBytes(opts FetchOptions) int64 {
	if opts.MaxBytes > 0 {
		return opts.MaxBytes
	}
	return defaultSourceMaxBytes
}

func sourceNow(opts FetchOptions) time.Time {
	if opts.Now != nil {
		return opts.Now()
	}
	return time.Now()
}
