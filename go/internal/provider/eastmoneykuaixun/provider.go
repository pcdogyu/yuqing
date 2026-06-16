package eastmoneykuaixun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

const (
	defaultPageURL = "https://kuaixun.eastmoney.com/"
	defaultAPIURL  = "https://np-weblist.eastmoney.com/comm/web/getFastNewsList"
	fromText       = "东方财富网"
)

type Provider struct {
	client  *resty.Client
	pageURL string
	apiURL  string
}

func NewProvider(client *resty.Client, pageURL string) *Provider {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		pageURL = defaultPageURL
	}
	return &Provider{
		client:  client,
		pageURL: pageURL,
		apiURL:  listAPIURL(pageURL),
	}
}

func (p *Provider) SourceType() string {
	return provider.SourceTypeEastMoneyKuaixun
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
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
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("eastmoney kuaixun fetch failed: %s", resp.Status())
	}
	return ParseResponse(resp.Body(), p.pageURL, time.Now().UTC())
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

func cleanText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
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

func marshalRaw(row newsRow) string {
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
