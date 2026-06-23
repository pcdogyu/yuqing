package publicfinance

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
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
	collected := make([]model.Item, 0, 80)
	errs := make([]string, 0)

	if p.sourceType == provider.SourceTypeWallStreetCNAStock && p.pageURL == defaultWallStreetCNAStockURL {
		items, err := p.fetchWallStreetCNLives(ctx, time.Now().UTC())
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			collected = append(collected, items...)
		}
	}
	if p.sourceType == provider.SourceTypeCLSTelegraph && p.pageURL == defaultCLSTelegraphURL {
		items, err := p.fetchCLSMobileTelegraph(ctx, time.Now().UTC())
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			collected = append(collected, items...)
		}
	}

	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36").
		Get(p.pageURL)
	if err != nil {
		errs = append(errs, err.Error())
	} else if resp.IsError() {
		errs = append(errs, fmt.Sprintf("%s fetch failed: %s", p.sourceType, resp.Status()))
	} else {
		collected = append(collected, ParsePage(resp.Body(), p.pageURL, p.sourceType, p.fromText, time.Now().UTC())...)
	}
	collected = dedupe(collected)
	if len(collected) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return collected, nil
}

func (p *Provider) fetchWallStreetCNLives(ctx context.Context, capturedAt time.Time) ([]model.Item, error) {
	const apiURL = "https://api-one-wscn.awtmt.com/apiv1/content/lives"
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json,text/plain,*/*").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		SetHeader("Referer", defaultWallStreetCNAStockURL).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36").
		SetQueryParam("channel", "global-channel").
		SetQueryParam("limit", "50").
		Get(apiURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s api fetch failed: %s", p.sourceType, resp.Status())
	}
	return parseWallStreetCNLives(resp.Body(), p.pageURL, capturedAt)
}

func (p *Provider) fetchCLSMobileTelegraph(ctx context.Context, capturedAt time.Time) ([]model.Item, error) {
	const mobileURL = "https://m.cls.cn/telegraph"
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		SetHeader("Referer", defaultCLSTelegraphURL).
		SetHeader("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148").
		Get(mobileURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s mobile fetch failed: %s", p.sourceType, resp.Status())
	}
	return parseCLSNextData(resp.Body(), p.pageURL, capturedAt), nil
}

func ParsePage(body []byte, pageURL string, sourceType string, fromText string, capturedAt time.Time) []model.Item {
	raw := string(body)
	items := make([]model.Item, 0, 64)
	items = append(items, parseSinaHiddenList(raw, pageURL, sourceType, fromText, capturedAt)...)
	items = append(items, parseAnchors(raw, pageURL, sourceType, fromText, capturedAt)...)
	items = append(items, parseJSONText(raw, pageURL, sourceType, fromText, capturedAt)...)
	return dedupe(items)
}

var (
	anchorPattern    = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	sinaBlockPattern = regexp.MustCompile(`(?is)<div\s+newsdata-id=["']?([^"'\s>]+)["']?\s+newsdata-time=["']?([^"'\s>]+)["']?[^>]*>(.*?)</div>\s*</div>`)
	tagPattern       = regexp.MustCompile(`(?is)<[^>]+>`)
	spacePattern     = regexp.MustCompile(`\s+`)
	jsonTextPattern  = regexp.MustCompile(`(?is)"(?:title|content|summary|brief|desc|abstract)"\s*:\s*"([^"]{6,240})"`)
	timePattern      = regexp.MustCompile(`\b20\d{2}[-/]\d{1,2}[-/]\d{1,2}(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?|\b\d{1,2}:\d{2}(?::\d{2})?\b`)
)

func parseSinaHiddenList(raw string, pageURL string, sourceType string, fromText string, capturedAt time.Time) []model.Item {
	if sourceType != provider.SourceTypeSinaFinance7x24 {
		return nil
	}
	blocks := sinaBlockPattern.FindAllStringSubmatch(raw, -1)
	items := make([]model.Item, 0, len(blocks))
	for _, block := range blocks {
		anchor := anchorPattern.FindStringSubmatch(block[3])
		if len(anchor) < 3 {
			continue
		}
		title := cleanHTML(anchor[2])
		if !looksLikeNewsTitle(title) {
			continue
		}
		item := newItem(sourceType, fromText, title, "", anchor[1], pageURL, capturedAt)
		if publishTime := sinaBlockPublishTime(block[2], block[3]); publishTime != "" {
			item.PublishTime = publishTime
			item.PublishTimeText = publishTime
		}
		item.SourceKey = strings.TrimSpace(block[1])
		items = append(items, item)
	}
	return items
}

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

func parseWallStreetCNLives(body []byte, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items []struct {
				ID             int64    `json:"id"`
				Content        string   `json:"content"`
				ContentText    string   `json:"content_text"`
				Title          string   `json:"title"`
				URI            string   `json:"uri"`
				GlobalMoreURI  string   `json:"global_more_uri"`
				DisplayTime    int64    `json:"display_time"`
				Channels       []string `json:"channels"`
				GlobalChannel  string   `json:"global_channel_name"`
				ReferenceTitle string   `json:"reference_title"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 0 && envelope.Code != 20000 {
		return nil, fmt.Errorf("wallstreetcn response code %d: %s", envelope.Code, envelope.Message)
	}
	items := make([]model.Item, 0, len(envelope.Data.Items))
	for _, row := range envelope.Data.Items {
		title := cleanHTML(nonEmpty(row.ContentText, row.Title, row.ReferenceTitle, row.Content))
		if !looksLikeAStockNewsTitle(title) {
			continue
		}
		item := newItem(provider.SourceTypeWallStreetCNAStock, "华尔街见闻", title, "", nonEmpty(row.URI, row.GlobalMoreURI), pageURL, capturedAt)
		if row.DisplayTime > 0 {
			item.PublishTime = time.Unix(row.DisplayTime, 0).In(financeLocation()).Format("2006-01-02 15:04:05")
			item.PublishTimeText = item.PublishTime
		}
		item.SourceKey = strconv.FormatInt(row.ID, 10)
		item.RawPayload = marshalRaw(row)
		items = append(items, item)
	}
	return dedupe(items), nil
}

func parseCLSNextData(body []byte, pageURL string, capturedAt time.Time) []model.Item {
	raw := string(body)
	raw = extractAssignedJSONObject(raw, "__NEXT_DATA__")
	if raw == "" {
		return nil
	}
	var envelope struct {
		Props struct {
			InitialState struct {
				RollData []struct {
					ID        int64  `json:"id"`
					Title     string `json:"title"`
					Content   string `json:"content"`
					Brief     string `json:"brief"`
					ShareURL  string `json:"shareurl"`
					CTime     int64  `json:"ctime"`
					Modified  int64  `json:"modified_time"`
					StockList []struct {
						StockID string `json:"StockID"`
						Name    string `json:"name"`
					} `json:"stock_list"`
				} `json:"roll_data"`
			} `json:"initialState"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}
	items := make([]model.Item, 0, len(envelope.Props.InitialState.RollData))
	for _, row := range envelope.Props.InitialState.RollData {
		title := cleanHTML(nonEmpty(row.Title, row.Brief, row.Content))
		if !looksLikeNewsTitle(title) {
			continue
		}
		detailURL := nonEmpty(row.ShareURL, fmt.Sprintf("https://www.cls.cn/detail/%d", row.ID))
		item := newItem(provider.SourceTypeCLSTelegraph, "财联社", title, cleanHTML(row.Brief), detailURL, pageURL, capturedAt)
		publishUnix := row.CTime
		if publishUnix == 0 {
			publishUnix = row.Modified
		}
		if publishUnix > 0 {
			item.PublishTime = time.Unix(publishUnix, 0).In(financeLocation()).Format("2006-01-02 15:04:05")
			item.PublishTimeText = item.PublishTime
		}
		item.SourceKey = strconv.FormatInt(row.ID, 10)
		item.TagFlags = clsStockFlags(row.StockList)
		item.RawPayload = marshalRaw(row)
		items = append(items, item)
	}
	return dedupe(items)
}

func extractAssignedJSONObject(raw string, marker string) string {
	start := strings.Index(raw, marker)
	if start < 0 {
		return ""
	}
	brace := strings.Index(raw[start:], "{")
	if brace < 0 {
		return ""
	}
	brace += start
	depth := 0
	inString := false
	escaped := false
	for i := brace; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
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
				return raw[brace : i+1]
			}
		}
	}
	return ""
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
	if len([]rune(title)) < 6 || len([]rune(title)) > 320 {
		return false
	}
	lower := strings.ToLower(title)
	blocked := []string{"javascript", "更多", "登录", "注册", "首页", "下载", "广告", "关于我们", "用户协议"}
	for _, word := range blocked {
		if lower == strings.ToLower(word) || strings.Contains(lower, strings.ToLower(word)+" ") {
			return false
		}
	}
	if strings.Contains(strings.ToUpper(title), "A股") {
		return true
	}
	keywords := []string{"股票", "股市", "个股", "板块", "涨停", "跌停", "涨幅", "跌幅", "证券", "券商", "银行", "指数", "资金", "公司", "公告", "财经", "交易", "概念", "期货", "债券", "基金", "港股", "美股"}
	for _, keyword := range keywords {
		if strings.Contains(title, keyword) {
			return true
		}
	}
	return false
}

func looksLikeAStockNewsTitle(title string) bool {
	if !looksLikeNewsTitle(title) {
		return false
	}
	upper := strings.ToUpper(title)
	if strings.Contains(upper, "A股") {
		return true
	}
	return strings.ContainsAny(title, "股板块证券券商银行涨停跌停概念公司")
}

func sinaBlockPublishTime(dateText string, rawBlock string) string {
	dateText = strings.TrimSpace(dateText)
	if len(dateText) != 8 {
		return ""
	}
	timeText := ""
	if match := regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}\b`).FindString(rawBlock); match != "" {
		timeText = match
	}
	if timeText == "" {
		return ""
	}
	return fmt.Sprintf("%s-%s-%s %s", dateText[:4], dateText[4:6], dateText[6:], timeText)
}

func clsStockFlags(stocks []struct {
	StockID string `json:"StockID"`
	Name    string `json:"name"`
}) string {
	flags := make([]string, 0, len(stocks))
	for _, stock := range stocks {
		code := strings.TrimSpace(stock.StockID)
		if code != "" {
			flags = append(flags, code)
		}
	}
	return strings.Join(flags, "/")
}

func marshalRaw(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func financeLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return location
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
