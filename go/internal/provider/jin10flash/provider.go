package jin10flash

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
)

var (
	reFlashHref = regexp.MustCompile(`https?://flash\.jin10\.com/detail/[^\s"'<>]+|//flash\.jin10\.com/detail/[^\s"'<>]+`)
	reDateTime  = regexp.MustCompile(`20\d{2}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}`)
	reClock     = regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}\b`)
)

type Provider struct {
	client  *resty.Client
	pageURL string
}

func NewProvider(client *resty.Client, pageURL string) *Provider {
	return &Provider{client: client, pageURL: pageURL}
}

func (p *Provider) SourceType() string {
	return provider.SourceTypeFlash
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get(p.pageURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("flash fetch failed: %s", resp.Status())
	}
	return ParseHTML(resp.String(), p.pageURL, time.Now().UTC())
}

func ParseHTML(html, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	items := parseDataListHTML(html, pageURL, capturedAt)
	if len(items) > 0 {
		return dedupe(items), nil
	}
	return dedupe(parseFallbackHTML(html, pageURL, capturedAt)), nil
}

func parseDataListHTML(html, pageURL string, capturedAt time.Time) []model.Item {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	raw, exists := doc.Find("#jin_slide_news").Attr("data-list")
	if !exists || strings.TrimSpace(raw) == "" {
		return nil
	}

	var payload []map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}

	items := make([]model.Item, 0, len(payload))
	for _, node := range payload {
		item := model.Item{
			SourceType: provider.SourceTypeFlash,
			SourceURL:  pageURL,
			CapturedAt: capturedAt,
		}

		item.Title = firstString(node, "title", "important", "country", "name")
		item.Content = firstString(node, "content", "remark", "text", "data", "desc")
		item.PublishTime = firstString(node, "time", "created_at", "datetime", "date")
		item.DetailURL = normalizeDetailURL(firstString(node, "url", "news_url", "link", "detail_url"))
		item.RawPayload = marshalRaw(node)
		item.IsVIP = firstBool(node, "is_vip", "vip")
		item.HasImage = firstBool(node, "has_pic", "has_image", "showPic")

		if item.DetailURL == "" && item.Title == "" && item.Content == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func parseFallbackHTML(html, pageURL string, capturedAt time.Time) []model.Item {
	matches := reFlashHref.FindAllStringIndex(html, -1)
	if len(matches) == 0 {
		return nil
	}

	items := make([]model.Item, 0, len(matches))
	for _, match := range matches {
		href := html[match[0]:match[1]]
		start := match[0] - 200
		if start < 0 {
			start = 0
		}
		end := match[1] + 400
		if end > len(html) {
			end = len(html)
		}

		segment := cleanText(html[start:end])
		publishTime := firstMatch(segment, reDateTime)
		if publishTime == "" {
			publishTime = firstMatch(segment, reClock)
		}
		content := segment
		content = strings.ReplaceAll(content, href, "")
		content = strings.TrimSpace(content)
		title := content
		if len([]rune(title)) > 80 {
			title = string([]rune(title)[:80])
		}

		items = append(items, model.Item{
			SourceType:  provider.SourceTypeFlash,
			Title:       title,
			Content:     content,
			PublishTime: publishTime,
			DetailURL:   normalizeDetailURL(href),
			SourceURL:   pageURL,
			CapturedAt:  capturedAt,
			RawPayload:  segment,
		})
	}
	return items
}

func dedupe(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	result := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := item.DetailURL + "|" + item.PublishTime + "|" + item.Title + "|" + item.Content
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func firstString(node map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := node[key]
		if !ok || value == nil {
			continue
		}
		switch actual := value.(type) {
		case string:
			if strings.TrimSpace(actual) != "" {
				return strings.TrimSpace(actual)
			}
		case float64:
			return strings.TrimSpace(fmt.Sprintf("%.0f", actual))
		}
	}
	return ""
}

func firstBool(node map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := node[key]
		if !ok || value == nil {
			continue
		}
		switch actual := value.(type) {
		case bool:
			return actual
		case float64:
			return actual != 0
		case string:
			normalized := strings.ToLower(strings.TrimSpace(actual))
			return normalized == "1" || normalized == "true" || normalized == "yes"
		}
	}
	return false
}

func normalizeDetailURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return raw
	}
	return u.String()
}

func marshalRaw(node any) string {
	data, err := json.Marshal(node)
	if err != nil {
		return ""
	}
	return string(data)
}

func cleanText(raw string) string {
	replacer := strings.NewReplacer(
		"\n", " ",
		"\r", " ",
		"\t", " ",
		"&nbsp;", " ",
		"&amp;", "&",
		"&quot;", "\"",
	)
	cleaned := replacer.Replace(raw)
	cleaned = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(cleaned, " ")
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}

func firstMatch(text string, re *regexp.Regexp) string {
	match := re.FindString(text)
	return strings.TrimSpace(match)
}
