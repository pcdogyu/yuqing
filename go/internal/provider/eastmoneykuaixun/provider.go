package eastmoneykuaixun

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

const (
	defaultPageURL      = "https://kuaixun.eastmoney.com/"
	defaultAPIURL       = "https://np-weblist.eastmoney.com/comm/web/getFastNewsList"
	defaultSearchAPIURL = "https://search-api-web.eastmoney.com/search/jsonp"
	fromText            = "东方财富网"
)

type Provider struct {
	client       *resty.Client
	pageURL      string
	apiURL       string
	searchAPIURL string
}

func NewProvider(client *resty.Client, pageURL string) *Provider {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		pageURL = defaultPageURL
	}
	return &Provider{
		client:       client,
		pageURL:      pageURL,
		apiURL:       listAPIURL(pageURL),
		searchAPIURL: searchAPIURL(pageURL),
	}
}

func (p *Provider) SourceType() string {
	return provider.SourceTypeEastMoneyKuaixun
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	collected := make([]model.Item, 0, 80)
	var errs []string

	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json,text/javascript,*/*;q=0.8").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		SetHeader("Referer", defaultPageURL).
		SetQueryParam("client", "web").
		SetQueryParam("biz", "web_724").
		SetQueryParam("fastColumn", "").
		SetQueryParam("sortEnd", "").
		SetQueryParam("pageSize", "50").
		SetQueryParam("req_trace", fmt.Sprintf("%d", time.Now().UnixMilli())).
		Get(p.apiURL)
	if err != nil {
		errs = append(errs, err.Error())
	} else if resp.IsError() {
		errs = append(errs, fmt.Sprintf("eastmoney kuaixun fetch failed: %s", resp.Status()))
	} else if items, parseErr := ParseResponse(resp.Body(), p.pageURL, time.Now().UTC()); parseErr != nil {
		errs = append(errs, parseErr.Error())
	} else {
		collected = append(collected, items...)
	}

	searchItems, searchErr := p.fetchSearchItems(ctx, time.Now().UTC())
	if searchErr != nil {
		errs = append(errs, searchErr.Error())
	} else {
		collected = append(collected, searchItems...)
	}

	collected = dedupe(collected)
	if len(collected) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return collected, nil
}

func (p *Provider) fetchSearchItems(ctx context.Context, capturedAt time.Time) ([]model.Item, error) {
	keywords := []string{"A股", "人工智能", "半导体", "券商", "银行", "黄金", "稀土", "机器人", "低空经济", "算力"}
	items := make([]model.Item, 0, len(keywords)*10)
	var errs []string
	for _, keyword := range keywords {
		param := map[string]any{
			"uid":           "",
			"keyword":       keyword,
			"type":          []string{"cmsArticleWebOld"},
			"client":        "web",
			"clientType":    "web",
			"clientVersion": "curr",
			"param": map[string]any{
				"cmsArticleWebOld": map[string]any{
					"searchScope": "default",
					"sort":        "time",
					"pageIndex":   1,
					"pageSize":    50,
					"preTag":      "",
					"postTag":     "",
				},
			},
		}
		rawParam, _ := json.Marshal(param)
		resp, err := p.client.R().
			SetContext(ctx).
			SetHeader("Accept", "application/json,text/javascript,*/*;q=0.8").
			SetHeader("Referer", "https://so.eastmoney.com/").
			SetQueryParam("cb", "yuqing").
			SetQueryParam("param", string(rawParam)).
			Get(p.searchAPIURL)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if resp.IsError() {
			errs = append(errs, fmt.Sprintf("eastmoney search %s failed: %s", keyword, resp.Status()))
			continue
		}
		parsed, err := ParseSearchResponse(resp.Body(), p.pageURL, capturedAt)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		items = append(items, parsed...)
	}
	if len(items) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return dedupe(items), nil
}

func ParseResponse(body []byte, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	payload := stripJSONP(strings.TrimSpace(string(body)))
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FastNewsList []newsRow `json:"fastNewsList"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, err
	}
	if envelope.Code != "" && envelope.Code != "1" {
		return nil, fmt.Errorf("eastmoney kuaixun response code %s: %s", envelope.Code, envelope.Message)
	}
	items := make([]model.Item, 0, len(envelope.Data.FastNewsList))
	for _, row := range envelope.Data.FastNewsList {
		item := rowToItem(row, pageURL, capturedAt)
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Summary) == "" {
			continue
		}
		items = append(items, item)
	}
	return dedupe(items), nil
}

func ParseSearchResponse(body []byte, pageURL string, capturedAt time.Time) ([]model.Item, error) {
	payload := stripJSONP(strings.TrimSpace(string(body)))
	var envelope struct {
		Result map[string][]searchRow `json:"result"`
		Data   []searchRow            `json:"data"`
		Items  []searchRow            `json:"items"`
		List   []searchRow            `json:"list"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, err
	}
	rows := make([]searchRow, 0)
	rows = append(rows, envelope.Data...)
	rows = append(rows, envelope.Items...)
	rows = append(rows, envelope.List...)
	for _, group := range envelope.Result {
		rows = append(rows, group...)
	}
	items := make([]model.Item, 0, len(rows))
	for _, row := range rows {
		item := searchRowToItem(row, pageURL, capturedAt)
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Summary) == "" {
			continue
		}
		items = append(items, item)
	}
	return dedupe(items), nil
}

type newsRow struct {
	Summary    string   `json:"summary"`
	Code       string   `json:"code"`
	TitleColor int      `json:"titleColor"`
	RealSort   string   `json:"realSort"`
	ShowTime   string   `json:"showTime"`
	Title      string   `json:"title"`
	StockList  []string `json:"stockList"`
	Image      []string `json:"image"`
}

type searchRow struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Summary   string `json:"summary"`
	Date      string `json:"date"`
	ShowTime  string `json:"showTime"`
	URL       string `json:"url"`
	Code      string `json:"code"`
	MediaName string `json:"mediaName"`
	Source    string `json:"source"`
}

func rowToItem(row newsRow, pageURL string, capturedAt time.Time) model.Item {
	title := cleanText(row.Title)
	summary := cleanText(row.Summary)
	publishTime := cleanText(row.ShowTime)
	detailURL := eastmoneyDetailURL(row.Code)
	return model.Item{
		SourceType:         provider.SourceTypeEastMoneyKuaixun,
		Title:              title,
		Summary:            summary,
		Content:            summary,
		PublishTime:        publishTime,
		PublishTimeText:    publishTime,
		DetailURL:          detailURL,
		SourceURL:          nonEmpty(detailURL, pageURL),
		TagFlags:           strings.Join(row.StockList, "/"),
		FromText:           fromText,
		ExternalSourceHost: hostOf(nonEmpty(pageURL, detailURL)),
		HasImage:           len(row.Image) > 0,
		RawPayload:         marshalRaw(row),
		CapturedAt:         capturedAt,
	}
}

func searchRowToItem(row searchRow, pageURL string, capturedAt time.Time) model.Item {
	title := cleanHTMLText(row.Title)
	summary := cleanHTMLText(nonEmpty(row.Summary, row.Content))
	publishTime := cleanText(nonEmpty(row.Date, row.ShowTime))
	detailURL := normalizeURL(nonEmpty(row.URL, eastmoneyDetailURL(row.Code)))
	source := cleanText(nonEmpty(row.MediaName, row.Source, fromText))
	return model.Item{
		SourceType:         provider.SourceTypeEastMoneyKuaixun,
		Title:              title,
		Summary:            summary,
		Content:            summary,
		PublishTime:        publishTime,
		PublishTimeText:    publishTime,
		DetailURL:          detailURL,
		SourceURL:          nonEmpty(detailURL, pageURL),
		FromText:           source,
		ExternalSourceHost: hostOf(nonEmpty(detailURL, pageURL)),
		RawPayload:         marshalRaw(row),
		CapturedAt:         capturedAt,
	}
}

func listAPIURL(pageURL string) string {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		return defaultAPIURL
	}
	if strings.Contains(pageURL, "getFastNewsList") {
		return pageURL
	}
	return defaultAPIURL
}

func searchAPIURL(pageURL string) string {
	pageURL = strings.TrimSpace(pageURL)
	if strings.Contains(pageURL, "search/jsonp") || strings.Contains(pageURL, "search-api-web.eastmoney.com") {
		return pageURL
	}
	return defaultSearchAPIURL
}

func eastmoneyDetailURL(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	return "https://finance.eastmoney.com/a/" + code + ".html"
}

func stripJSONP(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		return raw
	}
	open := strings.Index(raw, "(")
	close := strings.LastIndex(raw, ")")
	if open >= 0 && close > open {
		return strings.TrimSpace(raw[open+1 : close])
	}
	return raw
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

func cleanText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func cleanHTMLText(raw string) string {
	raw = strings.ReplaceAll(raw, "<em>", "")
	raw = strings.ReplaceAll(raw, "</em>", "")
	return cleanText(html.UnescapeString(raw))
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

func marshalRaw(row any) string {
	payload, err := json.Marshal(row)
	if err != nil {
		return ""
	}
	return string(payload)
}

func dedupe(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	result := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := nonEmpty(item.DetailURL, item.SourceURL) + "|" + item.PublishTime + "|" + item.Title + "|" + item.Content
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}
