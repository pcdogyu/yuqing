package jin10xnews

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

type Provider struct {
	client  *resty.Client
	pageURL string
}

func NewProvider(client *resty.Client, pageURL string) *Provider {
	return &Provider{client: client, pageURL: pageURL}
}

func (p *Provider) SourceType() string {
	return provider.SourceTypeHeadline
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get(p.pageURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("headline fetch failed: %s", resp.Status())
	}
	items, err := ParseHTML(resp.String(), p.pageURL, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return p.enrichItems(ctx, items), nil
}

func ParseHTML(html, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	items := make([]model.Item, 0)
	doc.Find(".jin10-news-list-item.news").Each(func(_ int, s *goquery.Selection) {
		item := model.Item{
			SourceType: provider.SourceTypeHeadline,
			SourceURL:  pageURL,
			CapturedAt: capturedAt,
		}
		item.Title = cleanText(s.Find(".jin10-news-list-item-title").First().Text())
		item.Summary = cleanText(s.Find(".jin10-news-list-item-introduction").First().Text())
		item.PublishTimeText = cleanText(s.Find(".jin10-news-list-item-display_datetime").First().Text())
		item.TagFlags = collectTags(s)
		item.FromText = inferFromText(s)
		item.HasImage = s.Find(".jin10-news-list-item-thumb").Length() > 0
		item.DetailURL = findDetailURL(s)
		item.ExternalSourceHost = hostOf(item.DetailURL)
		item.RawPayload = outerHTML(s)

		if item.Title == "" && item.DetailURL == "" {
			return
		}
		items = append(items, item)
	})

	return dedupe(items), nil
}

func collectTags(s *goquery.Selection) string {
	parts := make([]string, 0, 4)
	s.Find(".jin10-news-list-item-footer .status-mini, .jin10-news-list-item-footer .jin10-news-img-new, .jin10-news-list-item-footer .u-list li span").Each(func(_ int, child *goquery.Selection) {
		value := cleanText(child.Text())
		switch value {
		case "", "来自", "刚刚":
			return
		}
		if strings.Contains(value, "来自：") || strings.Contains(value, "来自:") {
			return
		}
		if value == "分钟前" || value == "小时前" {
			return
		}
		if strings.Contains(value, "前") || strings.Contains(value, "年") || strings.Contains(value, ":") {
			return
		}
		if !contains(parts, value) {
			parts = append(parts, value)
		}
	})
	return strings.Join(parts, "/")
}

func inferFromText(s *goquery.Selection) string {
	text := cleanText(s.Find(".jin10-news-list-item-footer").Text())
	if idx := strings.Index(text, "来自"); idx >= 0 {
		return strings.TrimSpace(text[idx:])
	}
	anchors := s.Find("a")
	if anchors.Length() > 1 {
		if href, ok := anchors.Eq(1).Attr("href"); ok && strings.Contains(href, "mp.weixin.qq.com") {
			return "来自：微信"
		}
	}
	return ""
}

func findDetailURL(s *goquery.Selection) string {
	href := ""
	s.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		value, ok := a.Attr("href")
		if !ok || strings.TrimSpace(value) == "" {
			return true
		}
		href = strings.TrimSpace(value)
		return false
	})
	return normalizeURL(href)
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

func hostOf(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func dedupe(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	result := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := item.DetailURL + "|" + item.Title + "|" + item.PublishTimeText
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func outerHTML(s *goquery.Selection) string {
	html, err := goquery.OuterHtml(s)
	if err != nil {
		return ""
	}
	data, err := json.Marshal(html)
	if err != nil {
		return html
	}
	return string(data)
}

func cleanText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func (p *Provider) enrichItems(ctx context.Context, items []model.Item) []model.Item {
	for idx := range items {
		if !shouldFetchDetail(items[idx]) {
			continue
		}
		resp, err := p.client.R().
			SetContext(ctx).
			Get(items[idx].DetailURL)
		if err != nil || resp.IsError() {
			continue
		}
		content, summary, shareURL := parseDetailHTML(resp.String())
		if strings.TrimSpace(content) != "" {
			items[idx].Content = content
		}
		if strings.TrimSpace(items[idx].Summary) == "" && strings.TrimSpace(summary) != "" {
			items[idx].Summary = summary
		}
		if strings.TrimSpace(shareURL) != "" {
			items[idx].SourceURL = shareURL
		}
	}
	return items
}

func shouldFetchDetail(item model.Item) bool {
	if strings.TrimSpace(item.DetailURL) == "" {
		return false
	}
	if !strings.Contains(item.DetailURL, "xnews.jin10.com/details/") {
		return false
	}
	return strings.TrimSpace(item.Content) == ""
}

func parseDetailHTML(html string) (content string, summary string, shareURL string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", "", ""
	}

	summary = cleanText(doc.Find(".jin10-news-cdetails-introduction").First().Text())

	parts := make([]string, 0, 8)
	doc.Find(".jin10-news-cdetails-content p, .jin10-news-cdetails-content li, .jin10-news-cdetails-content blockquote").Each(func(_ int, s *goquery.Selection) {
		text := cleanText(s.Text())
		if text != "" {
			parts = append(parts, text)
		}
	})
	if len(parts) == 0 {
		text := cleanText(doc.Find(".jin10-news-cdetails-content").First().Text())
		if text != "" {
			parts = append(parts, text)
		}
	}
	content = strings.Join(parts, "\n")

	if value, ok := doc.Find(".social-share").First().Attr("data-url"); ok {
		shareURL = normalizeURL(value)
	}
	return content, summary, shareURL
}
