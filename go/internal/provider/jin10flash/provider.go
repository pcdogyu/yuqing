package jin10flash

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

var (
	reFlashHref   = regexp.MustCompile(`https?://flash\.jin10\.com/detail/[^\s"'<>]+|//flash\.jin10\.com/detail/[^\s"'<>]+`)
	reDateTime    = regexp.MustCompile(`20\d{2}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}`)
	reClock       = regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}\b`)
	reNuxtTitle   = regexp.MustCompile(`title:"((?:\\.|[^"])*)"`)
	reNuxtContent = regexp.MustCompile(`content:"((?:\\.|[^"])*)"`)
)

const (
	defaultPageURL      = "https://www.jin10.com/"
	defaultAPIURL       = "https://flash-api.jin10.com/get_flash_list"
	defaultAPIChannel   = "-8200"
	defaultAPIVIP       = "1"
	maxWindowPages      = 120
	defaultAPIFetchSize = 20
)

type Provider struct {
	client          *resty.Client
	pageURL         string
	apiURL          string
	fallbackHTMLURL string
}

func NewProvider(client *resty.Client, pageURL string) *Provider {
	if client == nil {
		client = resty.New()
	}
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		pageURL = defaultAPIURL
	}
	p := &Provider{client: client}
	if isFlashAPIURL(pageURL) {
		p.apiURL = pageURL
		p.pageURL = defaultPageURL
		p.fallbackHTMLURL = defaultPageURL
		return p
	}
	p.pageURL = pageURL
	p.fallbackHTMLURL = pageURL
	return p
}

func (p *Provider) SourceType() string {
	return provider.SourceTypeFlash
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	capturedAt := time.Now().UTC()
	if p.apiURL != "" {
		items, _, err := p.fetchAPIPage(ctx, "", capturedAt)
		if err == nil && len(items) > 0 {
			return dedupe(items), nil
		}
		htmlItems, htmlErr := p.fetchHTML(ctx, capturedAt)
		if htmlErr == nil && len(htmlItems) > 0 {
			return htmlItems, nil
		}
		if err != nil {
			return nil, err
		}
		return htmlItems, htmlErr
	}
	return p.fetchHTML(ctx, capturedAt)
}

func (p *Provider) FetchWithOptions(ctx context.Context, options model.CrawlOptions) ([]model.Item, error) {
	start, hasStart := parseOptionTime(options.Start)
	end, hasEnd := parseOptionTime(options.End)
	if !hasStart && !hasEnd {
		return p.Fetch(ctx)
	}
	if p.apiURL == "" {
		return p.Fetch(ctx)
	}
	capturedAt := time.Now().UTC()
	maxTime := ""
	if hasEnd {
		maxTime = end.Format("2006-01-02 15:04:05")
	}
	collected := make([]model.Item, 0, maxWindowPages*defaultAPIFetchSize)
	seenMaxTime := make(map[string]struct{})
	var errs []string
	for page := 0; page < maxWindowPages; page++ {
		items, rows, err := p.fetchAPIPage(ctx, maxTime, capturedAt)
		if err != nil {
			errs = append(errs, err.Error())
			break
		}
		if len(rows) == 0 {
			break
		}
		collected = append(collected, items...)
		oldest, ok := oldestFlashAPITime(rows)
		if !ok {
			break
		}
		if hasStart && !oldest.After(start) {
			break
		}
		nextMaxTime := oldest.Add(-time.Second).Format("2006-01-02 15:04:05")
		if _, ok := seenMaxTime[nextMaxTime]; ok {
			break
		}
		seenMaxTime[nextMaxTime] = struct{}{}
		maxTime = nextMaxTime
	}
	collected = dedupe(collected)
	if len(collected) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return collected, nil
}

func (p *Provider) fetchHTML(ctx context.Context, capturedAt time.Time) ([]model.Item, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get(p.fallbackHTMLURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("flash fetch failed: %s", resp.Status())
	}
	items, err := ParseHTML(resp.String(), p.fallbackHTMLURL, capturedAt)
	if err != nil {
		return nil, err
	}
	return p.enrichItems(ctx, items), nil
}

func (p *Provider) fetchAPIPage(ctx context.Context, maxTime string, capturedAt time.Time) ([]model.Item, []flashAPIItem, error) {
	req := p.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json,text/plain,*/*").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		SetHeader("Referer", defaultPageURL).
		SetHeader("Origin", defaultPageURL).
		SetHeader("x-app-id", "bVBF4FyRTn5NJF5n").
		SetHeader("x-version", "1.0.0").
		SetQueryParam("channel", defaultAPIChannel).
		SetQueryParam("vip", defaultAPIVIP)
	if strings.TrimSpace(maxTime) != "" {
		req.SetQueryParam("max_time", strings.TrimSpace(maxTime))
	}
	resp, err := req.Get(p.apiURL)
	if err != nil {
		return nil, nil, err
	}
	if resp.IsError() {
		return nil, nil, fmt.Errorf("flash api fetch failed: %s", resp.Status())
	}
	items, rows, err := ParseAPI(resp.Body(), p.pageURL, capturedAt)
	if err != nil {
		return nil, nil, err
	}
	return items, rows, nil
}

type flashAPIItem struct {
	ID        string         `json:"id"`
	Time      string         `json:"time"`
	Type      int            `json:"type"`
	Data      map[string]any `json:"data"`
	Important int            `json:"important"`
	Tags      []any          `json:"tags"`
	Remark    []any          `json:"remark"`
	Extras    map[string]any `json:"extras"`
}

func ParseAPI(body []byte, pageURL string, capturedAt time.Time) ([]model.Item, []flashAPIItem, error) {
	var envelope struct {
		Status  int            `json:"status"`
		Message string         `json:"message"`
		Data    []flashAPIItem `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	if envelope.Status != 0 && envelope.Status != httpStatusOK {
		return nil, nil, fmt.Errorf("flash api response status %d: %s", envelope.Status, envelope.Message)
	}
	items := flashAPIRowsToItems(envelope.Data, pageURL, capturedAt)
	return dedupe(items), envelope.Data, nil
}

const httpStatusOK = 200

func flashAPIRowsToItems(rows []flashAPIItem, pageURL string, capturedAt time.Time) []model.Item {
	items := make([]model.Item, 0, len(rows))
	for _, row := range rows {
		content := cleanText(firstString(row.Data, "content", "title", "name"))
		title := cleanText(firstString(row.Data, "title"))
		if title == "" {
			title = firstRunes(content, 80)
		}
		if title == "" && content == "" {
			continue
		}
		source := cleanText(firstString(row.Data, "source"))
		sourceLink := normalizeDetailURL(firstString(row.Data, "source_link", "url", "link"))
		detailURL := flashDetailURL(row.ID)
		rawPayload := marshalRaw(row)
		item := model.Item{
			SourceType:         provider.SourceTypeFlash,
			Title:              title,
			Content:            content,
			Summary:            content,
			PublishTime:        strings.TrimSpace(row.Time),
			PublishTimeText:    strings.TrimSpace(row.Time),
			DetailURL:          detailURL,
			SourceURL:          nonEmpty(sourceLink, pageURL, defaultPageURL),
			TagFlags:           collectAPITags(row.Tags, row.Remark),
			FromText:           formatFromText(source),
			ExternalSourceHost: hostOf(sourceLink),
			IsVIP:              firstBool(row.Extras, "vip", "is_vip"),
			HasImage:           strings.TrimSpace(firstString(row.Data, "pic", "image")) != "",
			RawPayload:         rawPayload,
			CapturedAt:         capturedAt,
		}
		items = append(items, item)
	}
	return items
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
		title, content, publishTime, sourceURL, fromText := parseDetailHTML(resp.String())
		if strings.TrimSpace(title) != "" {
			items[idx].Title = title
		}
		if strings.TrimSpace(content) != "" {
			items[idx].Content = content
			if strings.TrimSpace(items[idx].Summary) == "" {
				items[idx].Summary = content
			}
		}
		if strings.TrimSpace(publishTime) != "" {
			items[idx].PublishTime = publishTime
		}
		if strings.TrimSpace(sourceURL) != "" {
			items[idx].SourceURL = sourceURL
			items[idx].ExternalSourceHost = hostOf(sourceURL)
		}
		if strings.TrimSpace(fromText) != "" {
			items[idx].FromText = fromText
		}
	}
	return items
}

func shouldFetchDetail(item model.Item) bool {
	if strings.TrimSpace(item.DetailURL) == "" {
		return false
	}
	return strings.Contains(item.DetailURL, "flash.jin10.com/detail/")
}

func parseDetailHTML(html string) (title string, content string, publishTime string, sourceURL string, fromText string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", "", "", "", ""
	}

	title = cleanText(doc.Find(".content-title .flash-title").First().Text())
	if title == "" {
		title = cleanText(doc.Find(".content-title div").First().Text())
	}
	if title == "" {
		title = cleanText(doc.Find("title").First().Text())
		title = strings.TrimSuffix(title, " - 金十数据")
	}

	body := doc.Find(".content-title div").Eq(1)
	content = cleanText(body.Text())

	nuxtTitle, nuxtContent := parseNuxtFlashContent(html)
	switch {
	case strings.TrimSpace(content) == "":
		content = nuxtContent
	case strings.TrimSpace(content) == strings.TrimSpace(title) && strings.TrimSpace(nuxtContent) != "":
		content = nuxtContent
	case len([]rune(nuxtContent)) > len([]rune(content)):
		content = nuxtContent
	}
	if strings.TrimSpace(title) == "" && strings.TrimSpace(nuxtTitle) != "" {
		title = nuxtTitle
	}

	if href, ok := body.Find("a[href]").Last().Attr("href"); ok {
		sourceURL = normalizeDetailURL(href)
	}
	if sourceURL != "" {
		linkText := cleanText(body.Find("a[href]").Last().Text())
		linkText = strings.Trim(linkText, "()（）")
		switch {
		case linkText != "":
			fromText = "来自：" + linkText
		case hostOf(sourceURL) != "":
			fromText = "来自：" + hostOf(sourceURL)
		}
	}

	if node := doc.Find(".content-time").First(); node.Length() > 0 {
		parts := make([]string, 0, 3)
		node.Find("span").Each(func(_ int, s *goquery.Selection) {
			text := cleanText(s.Text())
			if text != "" && text != "周一" && text != "周二" && text != "周三" && text != "周四" && text != "周五" && text != "周六" && text != "周日" {
				parts = append(parts, text)
			}
		})
		publishTime = strings.Join(parts, " ")
	}

	return strings.TrimSpace(title), strings.TrimSpace(content), strings.TrimSpace(publishTime), strings.TrimSpace(sourceURL), strings.TrimSpace(fromText)
}

func parseNuxtFlashContent(html string) (title string, content string) {
	start := strings.Index(html, "flash:{")
	if start < 0 {
		return "", ""
	}
	segment := html[start:]
	titleMatch := reNuxtTitle.FindStringSubmatch(segment)
	contentMatch := reNuxtContent.FindStringSubmatch(segment)
	if len(titleMatch) >= 2 {
		title = decodeNuxtString(titleMatch[1])
	}
	if len(contentMatch) >= 2 {
		content = cleanText(decodeNuxtString(contentMatch[1]))
	}
	return strings.TrimSpace(title), strings.TrimSpace(content)
}

func decodeNuxtString(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var decoded string
	if err := json.Unmarshal([]byte(`"`+raw+`"`), &decoded); err == nil {
		return decoded
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

func isFlashAPIURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	return strings.Contains(host, "flash-api.jin10.com") || strings.Contains(strings.ToLower(parsed.Path), "get_flash_list")
}

func flashDetailURL(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "https://flash.jin10.com/detail/" + id
}

func formatFromText(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	return "来自：" + source
}

func collectAPITags(tags []any, remarks []any) string {
	parts := make([]string, 0, len(tags)+len(remarks))
	appendPart := func(value string) {
		value = cleanText(value)
		if value == "" || containsString(parts, value) {
			return
		}
		parts = append(parts, value)
	}
	for _, tag := range tags {
		switch actual := tag.(type) {
		case string:
			appendPart(actual)
		case map[string]any:
			appendPart(firstString(actual, "name", "title", "label"))
		}
	}
	for _, remark := range remarks {
		if actual, ok := remark.(map[string]any); ok {
			title := firstString(actual, "title", "name")
			symbol := firstString(actual, "symbol")
			appendPart(strings.TrimSpace(strings.TrimSpace(title) + " " + strings.TrimSpace(symbol)))
		}
	}
	return strings.Join(parts, "/")
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func oldestFlashAPITime(rows []flashAPIItem) (time.Time, bool) {
	var oldest time.Time
	for _, row := range rows {
		parsed, ok := parseOptionTime(row.Time)
		if !ok {
			continue
		}
		if oldest.IsZero() || parsed.Before(oldest) {
			oldest = parsed
		}
	}
	if oldest.IsZero() {
		return time.Time{}, false
	}
	return oldest, true
}

func parseOptionTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	location := jin10FlashLocation()
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, raw, location); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func jin10FlashLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}

func firstRunes(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if limit <= 0 {
		return ""
	}
	runes := []rune(raw)
	if len(runes) <= limit {
		return raw
	}
	return string(runes[:limit])
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	cleaned = html.UnescapeString(cleaned)
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}

func firstMatch(text string, re *regexp.Regexp) string {
	match := re.FindString(text)
	return strings.TrimSpace(match)
}
