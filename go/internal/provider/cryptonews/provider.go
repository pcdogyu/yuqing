package cryptonews

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

const coindeskRSSURL = "https://www.coindesk.com/arc/outboundfeeds/rss/?outputType=xml"

var browserHeaders = map[string]string{
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
	"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	"Cache-Control":   "no-cache",
	"Pragma":          "no-cache",
}

type Provider struct {
	client     *resty.Client
	sourceType string
	pageURL    string
	fromText   string
}

func NewForesightNewsflashProvider(client *resty.Client, pageURL string) *Provider {
	return &Provider{
		client:     client,
		sourceType: provider.SourceTypeForesightNewsflash,
		pageURL:    strings.TrimSpace(pageURL),
		fromText:   "Foresight News",
	}
}

func NewCoinDeskZHLatestProvider(client *resty.Client, pageURL string) *Provider {
	return &Provider{
		client:     client,
		sourceType: provider.SourceTypeCoinDeskZHLatest,
		pageURL:    strings.TrimSpace(pageURL),
		fromText:   "CoinDesk 中文",
	}
}

func NewPANewsNewsflashProvider(client *resty.Client, feedURL string) *Provider {
	return &Provider{
		client:     client,
		sourceType: provider.SourceTypePANewsNewsflash,
		pageURL:    strings.TrimSpace(feedURL),
		fromText:   "PANews",
	}
}

func (p *Provider) SourceType() string {
	return p.sourceType
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	if strings.TrimSpace(p.pageURL) == "" {
		return nil, fmt.Errorf("%s endpoint is empty", p.sourceType)
	}
	resp, err := p.fetchPage(ctx, p.pageURL)
	if p.sourceType == provider.SourceTypeForesightNewsflash && shouldTryForesightFallback(resp, err) {
		if fallbackURL := foresightFallbackURL(p.pageURL); fallbackURL != "" {
			resp, err = p.fetchPage(ctx, fallbackURL)
		}
	}
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s fetch failed: %s", p.sourceType, resp.Status())
	}
	capturedAt := time.Now().UTC()
	switch p.sourceType {
	case provider.SourceTypeForesightNewsflash:
		return ParseForesightHTML(resp.String(), p.pageURL, capturedAt)
	case provider.SourceTypeCoinDeskZHLatest:
		items, err := ParseCoinDeskHTML(resp.String(), p.pageURL, capturedAt)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			return items, nil
		}
		return p.fetchCoinDeskRSSFallback(ctx, capturedAt)
	case provider.SourceTypePANewsNewsflash:
		return ParsePANewsRSS(resp.Body(), p.pageURL, capturedAt)
	default:
		return nil, fmt.Errorf("unsupported crypto news source_type %s", p.sourceType)
	}
}

func (p *Provider) fetchPage(ctx context.Context, targetURL string) (*resty.Response, error) {
	req := p.client.R().SetContext(ctx)
	for key, value := range browserHeaders {
		req.SetHeader(key, value)
	}
	if p.sourceType == provider.SourceTypeForesightNewsflash {
		req.SetHeader("Referer", "https://foresightnews.pro/")
	}
	return req.Get(targetURL)
}

func shouldTryForesightFallback(resp *resty.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	return resp.StatusCode() == 502 || resp.StatusCode() == 503 || resp.StatusCode() == 504
}

func foresightFallbackURL(pageURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = "/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (p *Provider) fetchCoinDeskRSSFallback(ctx context.Context, capturedAt time.Time) ([]model.Item, error) {
	resp, err := p.client.R().SetContext(ctx).Get(coindeskRSSURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("coindesk rss fetch failed: %s", resp.Status())
	}
	return ParseCoinDeskRSS(resp.Body(), coindeskRSSURL, capturedAt)
}

func ParseForesightHTML(rawHTML, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	items := parseForesightNuxt(rawHTML, pageURL, capturedAt)
	if len(items) == 0 {
		items = parseForesightSSR(rawHTML, pageURL, capturedAt)
	}
	return dedupe(items), nil
}

func parseForesightNuxt(rawHTML, pageURL string, capturedAt time.Time) []model.Item {
	start := strings.Index(rawHTML, "window.__NUXT__=")
	if start < 0 {
		return nil
	}
	payload := rawHTML[start:]
	objects := scanJSObjects(payload)
	items := make([]model.Item, 0, len(objects))
	for _, object := range objects {
		if !strings.Contains(object, "title:") || !strings.Contains(object, "published_at:") {
			continue
		}
		id := firstJSNumber(object, "id")
		title := cleanText(firstJSString(object, "title"))
		summary := cleanText(firstJSString(object, "brief"))
		content := cleanHTML(firstJSString(object, "content"))
		if title == "" && summary == "" && content == "" {
			continue
		}
		publishedAt := firstJSNumber(object, "published_at")
		published := unixSeconds(publishedAt)
		detailURL := absoluteURL(pageURL, fmt.Sprintf("/news/detail/%s", id))
		sourceLink := normalizeURL(firstJSString(object, "source_link"))
		if sourceLink == "" {
			sourceLink = detailURL
		}
		items = append(items, model.Item{
			SourceType:         provider.SourceTypeForesightNewsflash,
			Title:              title,
			Summary:            summary,
			Content:            content,
			PublishTime:        formatTime(published),
			PublishTimeText:    formatTime(published),
			DetailURL:          detailURL,
			SourceURL:          sourceLink,
			TagFlags:           strings.Join(firstJSTagNames(object), "/"),
			FromText:           "Foresight News",
			ExternalSourceHost: hostOf(pageURL),
			RawPayload:         marshalRaw(map[string]string{"nuxt_item": object}),
			CapturedAt:         capturedAt,
		})
	}
	return items
}

func parseForesightSSR(rawHTML, pageURL string, capturedAt time.Time) []model.Item {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	items := make([]model.Item, 0)
	doc.Find("a[href*='/news/detail/']").Each(func(_ int, a *goquery.Selection) {
		title := cleanText(a.Text())
		if title == "" {
			return
		}
		href, _ := a.Attr("href")
		detailURL := absoluteURL(pageURL, href)
		container := a.Parent()
		for i := 0; i < 4 && container != nil && container.Length() > 0; i++ {
			text := cleanText(container.Text())
			if strings.Contains(text, title) && len([]rune(text)) > len([]rune(title))+20 {
				items = append(items, model.Item{
					SourceType:         provider.SourceTypeForesightNewsflash,
					Title:              title,
					Summary:            summarizeText(strings.Replace(text, title, "", 1), 180),
					Content:            text,
					DetailURL:          detailURL,
					SourceURL:          detailURL,
					FromText:           "Foresight News",
					ExternalSourceHost: hostOf(pageURL),
					RawPayload:         text,
					CapturedAt:         capturedAt,
				})
				return
			}
			container = container.Parent()
		}
	})
	return items
}

func ParseCoinDeskHTML(rawHTML, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil, err
	}
	items := make([]model.Item, 0)
	doc.Find("a.content-card-title").Each(func(_ int, a *goquery.Selection) {
		title := cleanText(a.Find("h1,h2,h3").First().Text())
		if title == "" {
			title = cleanText(a.Text())
		}
		href, _ := a.Attr("href")
		detailURL := absoluteURL(pageURL, href)
		if title == "" || detailURL == "" {
			return
		}
		container := a.Parent()
		summary := cleanText(container.Find("p.font-body").First().Text())
		category := cleanText(container.Find("a.font-title").First().Text())
		publishText := cleanText(container.Find("span.font-metadata").First().Text())
		if summary == "" {
			summary = summarizeText(cleanText(container.Text()), 180)
		}
		raw, _ := goquery.OuterHtml(container)
		items = append(items, model.Item{
			SourceType:         provider.SourceTypeCoinDeskZHLatest,
			Title:              title,
			Summary:            summary,
			Content:            summary,
			PublishTimeText:    publishText,
			DetailURL:          detailURL,
			SourceURL:          detailURL,
			TagFlags:           category,
			FromText:           "CoinDesk 中文",
			ExternalSourceHost: hostOf(pageURL),
			RawPayload:         marshalRaw(raw),
			CapturedAt:         capturedAt,
		})
	})
	return dedupe(items), nil
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Content     string   `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Categories  []string `xml:"category"`
}

func ParseCoinDeskRSS(body []byte, feedURL string, capturedAt time.Time) ([]model.Item, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	items := make([]model.Item, 0, len(feed.Channel.Items))
	for _, row := range feed.Channel.Items {
		title := cleanText(row.Title)
		link := normalizeURL(row.Link)
		if title == "" || link == "" {
			continue
		}
		published, publishText := parseRSSDate(row.PubDate)
		items = append(items, model.Item{
			SourceType:         provider.SourceTypeCoinDeskZHLatest,
			Title:              title,
			Summary:            cleanHTML(row.Description),
			Content:            cleanHTML(row.Description),
			PublishTime:        formatTime(published),
			PublishTimeText:    nonEmpty(publishText, row.PubDate),
			DetailURL:          link,
			SourceURL:          link,
			TagFlags:           strings.Join(row.Categories, "/"),
			FromText:           "CoinDesk",
			ExternalSourceHost: hostOf(feedURL),
			RawPayload:         marshalRaw(row),
			CapturedAt:         capturedAt,
		})
	}
	return dedupe(items), nil
}

func ParsePANewsRSS(body []byte, feedURL string, capturedAt time.Time) ([]model.Item, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	items := make([]model.Item, 0, len(feed.Channel.Items))
	for _, row := range feed.Channel.Items {
		title := cleanText(row.Title)
		link := normalizeURL(row.Link)
		if title == "" || link == "" {
			continue
		}
		published, publishText := parseRSSDate(row.PubDate)
		content := cleanHTML(nonEmpty(row.Content, row.Description))
		summary := cleanHTML(row.Description)
		items = append(items, model.Item{
			SourceType:         provider.SourceTypePANewsNewsflash,
			Title:              title,
			Summary:            summary,
			Content:            content,
			PublishTime:        formatTime(published),
			PublishTimeText:    nonEmpty(publishText, row.PubDate),
			DetailURL:          link,
			SourceURL:          link,
			TagFlags:           strings.Join(row.Categories, "/"),
			FromText:           "PANews",
			ExternalSourceHost: hostOf(feedURL),
			HasImage:           strings.Contains(row.Content, "<img") || strings.Contains(row.Description, "<img"),
			RawPayload:         marshalRaw(row),
			CapturedAt:         capturedAt,
		})
	}
	return dedupe(items), nil
}

func scanJSObjects(input string) []string {
	objects := make([]string, 0)
	for offset := 0; ; {
		rel := strings.Index(input[offset:], "{id:")
		if rel < 0 {
			break
		}
		start := offset + rel
		depth := 0
		inString := false
		escaped := false
		for i := start; i < len(input); i++ {
			ch := input[i]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					object := input[start : i+1]
					if strings.Contains(object, "published_at:") {
						objects = append(objects, object)
					}
					offset = i + 1
					goto next
				}
			}
		}
		break
	next:
	}
	return objects
}

func firstJSString(object, field string) string {
	idx := strings.Index(object, field+":")
	if idx < 0 {
		return ""
	}
	rest := object[idx+len(field)+1:]
	rest = strings.TrimLeft(rest, " \t\r\n")
	if !strings.HasPrefix(rest, "\"") {
		return ""
	}
	end := 1
	escaped := false
	for ; end < len(rest); end++ {
		ch := rest[end]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			break
		}
	}
	if end >= len(rest) {
		return ""
	}
	value, err := strconv.Unquote(rest[:end+1])
	if err != nil {
		return ""
	}
	return html.UnescapeString(value)
}

func firstJSNumber(object, field string) string {
	re := regexp.MustCompile(field + `:([0-9]+)`)
	match := re.FindStringSubmatch(object)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func firstJSTagNames(object string) []string {
	tagBlock := regexp.MustCompile(`tags:\[(.*?)\]`).FindStringSubmatch(object)
	if len(tagBlock) < 2 {
		return nil
	}
	matches := regexp.MustCompile(`name:"((?:\\"|[^"])*)"`).FindAllStringSubmatch(tagBlock[1], -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		value, err := strconv.Unquote(`"` + match[1] + `"`)
		if err == nil && strings.TrimSpace(value) != "" {
			out = append(out, cleanText(value))
		}
	}
	return out
}

func unixSeconds(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}

func parseRSSDate(raw string) (time.Time, string) {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339} {
		parsed, err := time.Parse(layout, strings.TrimSpace(raw))
		if err == nil {
			return parsed.UTC(), parsed.UTC().Format(time.RFC3339)
		}
	}
	return time.Time{}, ""
}

func cleanHTML(raw string) string {
	raw = html.UnescapeString(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return cleanText(raw)
	}
	return cleanText(doc.Text())
}

func cleanText(raw string) string {
	return strings.Join(strings.Fields(html.UnescapeString(strings.TrimSpace(raw))), " ")
}

func summarizeText(raw string, limit int) string {
	text := cleanText(raw)
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func absoluteURL(base, href string) string {
	href = normalizeURL(href)
	if href == "" {
		return ""
	}
	parsed, err := url.Parse(href)
	if err == nil && parsed.IsAbs() {
		return href
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return href
	}
	rel, err := url.Parse(href)
	if err != nil {
		return href
	}
	return baseURL.ResolveReference(rel).String()
}

func hostOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Host
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func dedupe(items []model.Item) []model.Item {
	seen := map[string]struct{}{}
	out := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := nonEmpty(item.DetailURL, item.SourceURL) + "|" + item.Title + "|" + item.PublishTime + "|" + item.PublishTimeText
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func marshalRaw(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
