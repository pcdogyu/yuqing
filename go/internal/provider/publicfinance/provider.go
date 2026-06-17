package publicfinance

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

const (
	defaultWallStreetCNAStockURL = "https://wallstreetcn.com/live/a-stock"
	defaultCLSTelegraphURL       = "https://www.cls.cn/telegraph"
	defaultSinaFinance7x24URL    = "https://finance.sina.com.cn/7x24/?tag=10"
)

type Provider struct {
	client     *resty.Client
	pageURL    string
	sourceType string
	fromText   string
}

func NewWallStreetCNAStockProvider(client *resty.Client, pageURL string) *Provider {
	return newProvider(client, pageURL, defaultWallStreetCNAStockURL, provider.SourceTypeWallStreetCNAStock, "华尔街见闻")
}

func NewCLSTelegraphProvider(client *resty.Client, pageURL string) *Provider {
	return newProvider(client, pageURL, defaultCLSTelegraphURL, provider.SourceTypeCLSTelegraph, "财联社")
}

func NewSinaFinance7x24Provider(client *resty.Client, pageURL string) *Provider {
	return newProvider(client, pageURL, defaultSinaFinance7x24URL, provider.SourceTypeSinaFinance7x24, "新浪财经")
}

func newProvider(client *resty.Client, pageURL string, defaultURL string, sourceType string, fromText string) *Provider {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		pageURL = defaultURL
	}
	return &Provider{client: client, pageURL: pageURL, sourceType: sourceType, fromText: fromText}
}

func (p *Provider) SourceType() string {
	return p.sourceType
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		Get(p.pageURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s fetch failed: %s", p.sourceType, resp.Status())
	}
	return ParsePage(resp.Body(), p.pageURL, p.sourceType, p.fromText, time.Now().UTC()), nil
}

func ParsePage(body []byte, pageURL string, sourceType string, fromText string, capturedAt time.Time) []model.Item {
	raw := string(body)
	items := make([]model.Item, 0, 64)
	items = append(items, parseAnchors(raw, pageURL, sourceType, fromText, capturedAt)...)
	items = append(items, parseJSONText(raw, pageURL, sourceType, fromText, capturedAt)...)
	return dedupe(items)
}

var (
	anchorPattern   = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	tagPattern      = regexp.MustCompile(`(?is)<[^>]+>`)
	spacePattern    = regexp.MustCompile(`\s+`)
	jsonTextPattern = regexp.MustCompile(`(?is)"(?:title|content|summary|brief|desc|abstract)"\s*:\s*"([^"]{6,240})"`)
	timePattern     = regexp.MustCompile(`\b20\d{2}[-/]\d{1,2}[-/]\d{1,2}(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?|\b\d{1,2}:\d{2}(?::\d{2})?\b`)
)

func parseAnchors(raw string, pageURL string, sourceType string, fromText string, capturedAt time.Time) []model.Item {
	matches := anchorPattern.FindAllStringSubmatch(raw, -1)
	items := make([]model.Item, 0, len(matches))
	for _, match := range matches {
		title := cleanHTML(match[2])
		if !looksLikeNewsTitle(title) {
			continue
		}
		detailURL := absoluteURL(match[1], pageURL)
		items = append(items, newItem(sourceType, fromText, title, "", detailURL, pageURL, capturedAt))
	}
	return items
}

func parseJSONText(raw string, pageURL string, sourceType string, fromText string, capturedAt time.Time) []model.Item {
	matches := jsonTextPattern.FindAllStringSubmatch(raw, -1)
	items := make([]model.Item, 0, len(matches))
	for _, match := range matches {
		title := cleanJSONString(match[1])
		if !looksLikeNewsTitle(title) {
			continue
		}
		publishTime := ""
		if timeMatch := timePattern.FindString(raw[max(0, strings.Index(raw, match[0])-120):min(len(raw), strings.Index(raw, match[0])+len(match[0])+180)]); timeMatch != "" {
			publishTime = cleanText(timeMatch)
		}
		item := newItem(sourceType, fromText, title, "", pageURL, pageURL, capturedAt)
		item.PublishTime = publishTime
		item.PublishTimeText = publishTime
		items = append(items, item)
	}
	return items
}

func newItem(sourceType string, fromText string, title string, summary string, detailURL string, pageURL string, capturedAt time.Time) model.Item {
	detailURL = absoluteURL(detailURL, pageURL)
	return model.Item{
		SourceType:         sourceType,
		Title:              title,
		Summary:            summary,
		Content:            nonEmpty(summary, title),
		DetailURL:          detailURL,
		SourceURL:          nonEmpty(detailURL, pageURL),
		FromText:           fromText,
		ExternalSourceHost: hostOf(nonEmpty(detailURL, pageURL)),
		CapturedAt:         capturedAt,
	}
}

func looksLikeNewsTitle(title string) bool {
	title = strings.TrimSpace(title)
	if len([]rune(title)) < 6 || len([]rune(title)) > 160 {
		return false
	}
	lower := strings.ToLower(title)
	blocked := []string{"javascript", "更多", "登录", "注册", "首页", "下载", "广告", "关于我们", "用户协议"}
	for _, word := range blocked {
		if lower == strings.ToLower(word) || strings.Contains(lower, strings.ToLower(word)+" ") {
			return false
		}
	}
	return strings.ContainsAny(title, "股市证券公司指数资金银行券商板块涨跌公告财经交易") || strings.Contains(strings.ToUpper(title), "A股")
}

func cleanHTML(raw string) string {
	return cleanText(html.UnescapeString(tagPattern.ReplaceAllString(raw, " ")))
}

func cleanJSONString(raw string) string {
	raw = strings.ReplaceAll(raw, `\"`, `"`)
	raw = strings.ReplaceAll(raw, `\n`, " ")
	raw = strings.ReplaceAll(raw, `\u003c`, "<")
	raw = strings.ReplaceAll(raw, `\u003e`, ">")
	return cleanHTML(raw)
}

func cleanText(raw string) string {
	return spacePattern.ReplaceAllString(strings.TrimSpace(raw), " ")
}

func absoluteURL(raw string, baseURL string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func hostOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Host
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dedupe(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	result := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := nonEmpty(item.DetailURL, item.SourceURL) + "|" + item.Title
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
