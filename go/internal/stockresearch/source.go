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
	if text == "" {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: "未提取到正文", SourceFetchedAt: now}
	}
	return model.StockResearchSourceUpdate{SourceText: text, SourceFetchStatus: SourceStatusParsed, SourceFetchedAt: now}
}

func SourceUpdateFromPDFText(text string, fetchedAt string) model.StockResearchSourceUpdate {
	text = NormalizePlainText(text)
	if fetchedAt == "" {
		fetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if text == "" {
		return model.StockResearchSourceUpdate{SourceFetchStatus: SourceStatusFailed, SourceFetchError: "PDF 文本为空", SourceFetchedAt: fetchedAt}
	}
	return model.StockResearchSourceUpdate{SourceText: text, SourceFetchStatus: SourceStatusParsed, SourceFetchedAt: fetchedAt}
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
