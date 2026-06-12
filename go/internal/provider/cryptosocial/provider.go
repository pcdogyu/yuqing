package cryptosocial

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/external"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

type Provider struct {
	client     *resty.Client
	sourceType string
	endpoint   string
	authToken  string
	platform   string
	rateLimit  time.Duration
	mu         sync.Mutex
	lastFetch  time.Time
}

type Options struct {
	RateLimit time.Duration
}

func NewXProvider(client *resty.Client, endpoint, authToken string) *Provider {
	return NewXProviderWithOptions(client, endpoint, authToken, Options{})
}

func NewXProviderWithOptions(client *resty.Client, endpoint, authToken string, opts Options) *Provider {
	return &Provider{
		client:     client,
		sourceType: provider.SourceTypeCryptoX,
		endpoint:   strings.TrimSpace(endpoint),
		authToken:  strings.TrimSpace(authToken),
		platform:   "x",
		rateLimit:  opts.RateLimit,
	}
}

func NewTelegramProvider(client *resty.Client, endpoint, authToken string) *Provider {
	return NewTelegramProviderWithOptions(client, endpoint, authToken, Options{})
}

func NewTelegramProviderWithOptions(client *resty.Client, endpoint, authToken string, opts Options) *Provider {
	return &Provider{
		client:     client,
		sourceType: provider.SourceTypeCryptoTelegram,
		endpoint:   strings.TrimSpace(endpoint),
		authToken:  strings.TrimSpace(authToken),
		platform:   "telegram",
		rateLimit:  opts.RateLimit,
	}
}

func (p *Provider) SourceType() string {
	return p.sourceType
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	if strings.TrimSpace(p.endpoint) == "" {
		return nil, external.New(external.ErrDisabled, p.sourceType+" endpoint is empty")
	}
	if err := p.waitRateLimit(ctx); err != nil {
		return nil, err
	}
	req := p.client.R().SetContext(ctx)
	if p.authToken != "" {
		req.SetHeader("Authorization", "Bearer "+p.authToken)
	}
	resp, err := req.Get(p.endpoint)
	if err != nil {
		if code := external.Classify(err); code != "" {
			classified := external.New(code, err.Error())
			p.logFetchError(classified)
			return nil, classified
		}
		p.logFetchError(err)
		return nil, err
	}
	if resp.IsError() {
		err := external.HTTPStatus(p.sourceType+" fetch failed: "+resp.Status(), resp.StatusCode())
		p.logFetchError(err)
		return nil, err
	}
	items, err := ParseResponse(resp.Body(), p.sourceType, p.platform, p.endpoint, time.Now().UTC())
	if err != nil {
		if code := external.Classify(err); code != "" {
			classified := external.New(code, err.Error())
			p.logFetchError(classified)
			return nil, classified
		}
		p.logFetchError(err)
		return nil, err
	}
	if len(items) == 0 {
		err := external.New(external.ErrEmptyData, p.sourceType+" response contained no usable rows")
		p.logFetchError(err)
		return nil, err
	}
	return items, nil
}

func (p *Provider) waitRateLimit(ctx context.Context) error {
	if p.rateLimit <= 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if !p.lastFetch.IsZero() {
		wait := p.rateLimit - now.Sub(p.lastFetch)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return external.New(external.ErrTimeout, ctx.Err().Error())
			case <-timer.C:
			}
		}
	}
	p.lastFetch = time.Now()
	return nil
}

func (p *Provider) logFetchError(err error) {
	code := external.Classify(err)
	if code == "" {
		code = "external_error"
	}
	log.Warn().Err(err).Str("source_type", p.sourceType).Str("endpoint", p.endpoint).Str("error_code", code).Msg("crypto social fetch failed")
}

func ParseResponse(body []byte, sourceType, platform, endpoint string, capturedAt time.Time) ([]model.Item, error) {
	rows, err := decodePayload(body)
	if err != nil {
		return nil, err
	}
	items := make([]model.Item, 0, len(rows))
	for _, row := range rows {
		item := buildItem(row, sourceType, platform, endpoint, capturedAt)
		if item.SourceURL == "" && item.DetailURL == "" && item.Title == "" && item.Content == "" {
			continue
		}
		items = append(items, item)
	}
	deduped := dedupe(items)
	if len(deduped) < len(items) {
		log.Warn().Str("source_type", sourceType).Int("input", len(items)).Int("deduped", len(deduped)).Str("error_code", external.ErrDuplicateData).Msg("crypto social duplicate rows removed")
	}
	return deduped, nil
}

func decodePayload(body []byte) ([]map[string]any, error) {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return nil, nil
	}

	var direct []map[string]any
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, nil
	}

	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	for _, key := range []string{"items", "data", "results", "posts", "messages"} {
		rows := mapsFromAny(envelope[key])
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return nil, nil
}

func buildItem(row map[string]any, sourceType, platform, endpoint string, capturedAt time.Time) model.Item {
	content := cleanText(firstString(row, "content", "text", "body", "message", "full_text", "description"))
	title := cleanText(firstString(row, "title", "headline", "subject"))
	if title == "" {
		title = summarizeTitle(content)
	}
	link := normalizeURL(firstString(row, "url", "link", "source_url", "detail_url", "permalink", "tweet_url", "message_url"))
	publishTime := strings.TrimSpace(firstString(row, "publish_time", "posted_at", "created_at", "timestamp", "date"))
	if publishTime == "" {
		if value := parseTimestamp(row["created_at"]); !value.IsZero() {
			publishTime = value.Format(time.RFC3339)
		}
	}

	author := firstString(row, "author", "username", "screen_name", "channel", "from", "account")
	host := hostOf(link)
	if host == "" {
		host = hostOf(endpoint)
	}
	if author == "" {
		author = inferAuthor(platform, host)
	}

	return model.Item{
		SourceType:         sourceType,
		Title:              nonEmpty(title, summarizeTitle(cleanText(firstString(row, "summary", "excerpt")))),
		Content:            content,
		Summary:            cleanText(firstString(row, "summary", "excerpt", "snippet")),
		PublishTime:        publishTime,
		PublishTimeText:    publishTime,
		DetailURL:          link,
		SourceURL:          nonEmpty(link, endpoint),
		TagFlags:           strings.TrimSpace(firstString(row, "tags", "symbols", "tickers")),
		FromText:           author,
		ExternalSourceHost: host,
		HasImage:           firstBool(row, "has_image", "image", "media", "photo"),
		RawPayload:         marshalRaw(row),
		CapturedAt:         capturedAt,
	}
}

func mapsFromAny(value any) []map[string]any {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		row, ok := item.(map[string]any)
		if ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := row[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case json.Number:
			return typed.String()
		case []any:
			parts := make([]string, 0, len(typed))
			for _, item := range typed {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ",")
			}
		}
	}
	return ""
}

func firstBool(row map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := row[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case float64:
			return typed != 0
		case string:
			lower := strings.ToLower(strings.TrimSpace(typed))
			return lower == "1" || lower == "true" || lower == "yes"
		case []any:
			return len(typed) > 0
		}
	}
	return false
}

func parseTimestamp(value any) time.Time {
	switch typed := value.(type) {
	case float64:
		return time.Unix(int64(typed), 0).UTC()
	case json.Number:
		if raw, err := typed.Int64(); err == nil {
			return time.Unix(raw, 0).UTC()
		}
	case string:
		raw := strings.TrimSpace(typed)
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02 15:04",
		} {
			if parsed, err := time.Parse(layout, raw); err == nil {
				return parsed.UTC()
			}
		}
		if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if len(raw) >= 13 {
				return time.UnixMilli(unix).UTC()
			}
			return time.Unix(unix, 0).UTC()
		}
	}
	return time.Time{}
}

func dedupe(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	out := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := nonEmpty(item.DetailURL, item.SourceURL) + "|" + item.PublishTime + "|" + item.Title + "|" + item.Content
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
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
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Host
}

func summarizeTitle(content string) string {
	content = cleanText(content)
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= 64 {
		return content
	}
	return string(runes[:64]) + "..."
}

func cleanText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func marshalRaw(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func inferAuthor(platform, host string) string {
	switch {
	case strings.Contains(platform, "x"):
		return nonEmpty(host, "X")
	case strings.Contains(platform, "telegram"):
		return nonEmpty(host, "Telegram")
	default:
		return host
	}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
