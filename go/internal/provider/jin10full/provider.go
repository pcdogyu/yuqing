package jin10full

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10flash"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10xnews"
)

const (
	defaultFlashAPIURL = "https://flash-api.jin10.com/get_flash_list"
	stateLastRunAt     = "last_run_at"
	stateLastSeen      = "last_seen_publish_time"
)

var (
	reWhitespace = regexp.MustCompile(`\s+`)
	reHTMLTag    = regexp.MustCompile(`<[^>]+>`)
)

type StateStore interface {
	GetCrawlState(ctx context.Context, sourceType, cursorKey string) (string, error)
	UpsertCrawlState(ctx context.Context, sourceType, cursorKey, cursorValue string, updatedAt time.Time) error
}

type Options struct {
	FlashURL       string
	HeadlineURL    string
	BackfillDays   int
	MaxPagesPerRun int
	RateLimit      time.Duration
	IncludeSitemap bool
}

type Provider struct {
	client *resty.Client
	store  StateStore
	opts   Options
}

func NewProvider(client *resty.Client, store StateStore, opts Options) *Provider {
	if opts.FlashURL == "" {
		opts.FlashURL = defaultFlashAPIURL
	}
	if opts.HeadlineURL == "" {
		opts.HeadlineURL = "https://xnews.jin10.com/"
	}
	if opts.BackfillDays <= 0 {
		opts.BackfillDays = 30
	}
	if opts.MaxPagesPerRun <= 0 {
		opts.MaxPagesPerRun = 20
	}
	if opts.RateLimit <= 0 {
		opts.RateLimit = 800 * time.Millisecond
	}
	return &Provider{client: client, store: store, opts: opts}
}

func (p *Provider) SourceType() string {
	return provider.SourceTypeJin10Full
}

func (p *Provider) Fetch(ctx context.Context) ([]model.Item, error) {
	now := time.Now().UTC()
	robots := p.loadRobots(ctx)
	collected := make([]model.Item, 0)
	var errs []string

	if items, err := p.fetchFlash(ctx, now, robots); err != nil {
		errs = append(errs, err.Error())
	} else {
		collected = append(collected, items...)
	}
	p.pause(ctx)

	if items, err := p.fetchHeadlinePages(ctx, now, robots); err != nil {
		errs = append(errs, err.Error())
	} else {
		collected = append(collected, items...)
	}
	p.pause(ctx)

	if p.opts.IncludeSitemap {
		if items, err := p.fetchSitemapItems(ctx, now, robots); err != nil {
			errs = append(errs, err.Error())
		} else {
			collected = append(collected, items...)
		}
	}

	collected = p.filterBackfill(dedupe(collected), now)
	p.updateState(ctx, collected, now)
	if len(collected) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("jin10_full fetch failed: %s", strings.Join(errs, "; "))
	}
	return collected, nil
}

func (p *Provider) FetchWithOptions(ctx context.Context, options model.CrawlOptions) ([]model.Item, error) {
	now := time.Now().UTC()
	robots := robotsRules{}
	collected := make([]model.Item, 0)
	var errs []string

	if items, err := p.fetchFlashWithOptions(ctx, options, now, robots); err != nil {
		errs = append(errs, err.Error())
	} else {
		collected = append(collected, items...)
	}
	p.pause(ctx)

	if items, err := p.fetchHeadlineWindowPages(ctx, now, options); err != nil {
		errs = append(errs, err.Error())
	} else {
		collected = append(collected, items...)
	}

	collected = p.filterBackfill(dedupe(collected), now)
	p.updateState(ctx, collected, now)
	if len(collected) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("jin10_full window fetch failed: %s", strings.Join(errs, "; "))
	}
	return collected, nil
}

func (p *Provider) fetchFlash(ctx context.Context, capturedAt time.Time, robots robotsRules) ([]model.Item, error) {
	return p.fetchFlashWithOptions(ctx, model.CrawlOptions{}, capturedAt, robots)
}

func (p *Provider) fetchFlashWithOptions(ctx context.Context, options model.CrawlOptions, capturedAt time.Time, robots robotsRules) ([]model.Item, error) {
	if !robots.Allowed(p.opts.FlashURL) {
		return nil, nil
	}
	flashProvider := jin10flash.NewProvider(p.client, p.opts.FlashURL)
	var (
		items []model.Item
		err   error
	)
	if strings.TrimSpace(options.Start) != "" || strings.TrimSpace(options.End) != "" {
		items, err = flashProvider.FetchWithOptions(ctx, options)
	} else {
		items, err = flashProvider.Fetch(ctx)
	}
	if err != nil {
		return nil, err
	}
	for idx := range items {
		items[idx].SourceType = provider.SourceTypeJin10Full
		if items[idx].CapturedAt.IsZero() {
			items[idx].CapturedAt = capturedAt
		}
	}
	return items, nil
}

func (p *Provider) fetchHeadlineWindowPages(ctx context.Context, capturedAt time.Time, options model.CrawlOptions) ([]model.Item, error) {
	items := make([]model.Item, 0)
	maxPages := p.opts.MaxPagesPerRun
	if maxPages <= 0 || maxPages > 3 {
		maxPages = 3
	}
	start, hasStart := parseWindowOptionTime(options.Start)
	for page := 1; page <= maxPages; page++ {
		pageURL := withPage(p.opts.HeadlineURL, page)
		resp, err := p.client.R().SetContext(ctx).Get(pageURL)
		if err != nil {
			if page == 1 {
				return items, err
			}
			break
		}
		if resp.IsError() {
			if page == 1 {
				return items, fmt.Errorf("jin10_full headline fetch failed: %s", resp.Status())
			}
			break
		}
		pageItems, err := jin10xnews.ParseHTML(resp.String(), pageURL, capturedAt)
		if err != nil {
			return items, err
		}
		if len(pageItems) == 0 {
			break
		}
		pageHasWindowCandidate := false
		pageOldest := time.Time{}
		for idx := range pageItems {
			pageItems[idx].SourceType = provider.SourceTypeJin10Full
			if parsed, ok := parseItemTime(pageItems[idx]); ok {
				if pageOldest.IsZero() || parsed.Before(pageOldest) {
					pageOldest = parsed
				}
				if !hasStart || !parsed.Before(start) {
					pageHasWindowCandidate = true
				}
			} else {
				pageHasWindowCandidate = true
			}
		}
		items = append(items, pageItems...)
		if hasStart && !pageOldest.IsZero() && pageOldest.Before(start) && !pageHasWindowCandidate {
			break
		}
		p.pause(ctx)
	}
	return items, nil
}

func (p *Provider) fetchHeadlinePages(ctx context.Context, capturedAt time.Time, robots robotsRules) ([]model.Item, error) {
	items := make([]model.Item, 0)
	for page := 1; page <= p.opts.MaxPagesPerRun; page++ {
		pageURL := withPage(p.opts.HeadlineURL, page)
		if !robots.Allowed(pageURL) {
			continue
		}
		resp, err := p.client.R().SetContext(ctx).Get(pageURL)
		if err != nil {
			if page == 1 {
				return items, err
			}
			break
		}
		if resp.IsError() {
			if page == 1 {
				return items, fmt.Errorf("jin10_full headline fetch failed: %s", resp.Status())
			}
			break
		}
		pageItems, err := jin10xnews.ParseHTML(resp.String(), pageURL, capturedAt)
		if err != nil {
			return items, err
		}
		if len(pageItems) == 0 {
			break
		}
		items = append(items, p.enrichItems(ctx, pageItems, robots)...)
		p.pause(ctx)
	}
	return items, nil
}

func parseWindowOptionTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, raw, location); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func (p *Provider) fetchSitemapItems(ctx context.Context, capturedAt time.Time, robots robotsRules) ([]model.Item, error) {
	sitemapURL := sitemapURLFor(p.opts.FlashURL)
	if !robots.Allowed(sitemapURL) {
		return nil, nil
	}
	resp, err := p.client.R().SetContext(ctx).Get(sitemapURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("jin10_full sitemap fetch failed: %s", resp.Status())
	}
	urls := parseSitemapURLs(resp.Body())
	items := make([]model.Item, 0)
	limit := p.opts.MaxPagesPerRun
	for _, entry := range urls {
		if len(items) >= limit {
			break
		}
		if !isArticleURL(entry.Location) || !robots.Allowed(entry.Location) {
			continue
		}
		item, ok := p.fetchDetailItem(ctx, entry.Location, capturedAt)
		if !ok {
			continue
		}
		if item.PublishTime == "" {
			item.PublishTime = entry.LastMod
		}
		items = append(items, item)
		p.pause(ctx)
	}
	return items, nil
}

func (p *Provider) enrichItems(ctx context.Context, items []model.Item, robots robotsRules) []model.Item {
	for idx := range items {
		items[idx].SourceType = provider.SourceTypeJin10Full
		if strings.TrimSpace(items[idx].DetailURL) == "" || !robots.Allowed(items[idx].DetailURL) {
			continue
		}
		detail, ok := p.fetchDetailItem(ctx, items[idx].DetailURL, items[idx].CapturedAt)
		if !ok {
			continue
		}
		mergeItem(&items[idx], detail)
		p.pause(ctx)
	}
	return items
}

func (p *Provider) fetchDetailItem(ctx context.Context, detailURL string, capturedAt time.Time) (model.Item, bool) {
	if !isAllowedHost(detailURL, allowedHosts(p.opts)) {
		return model.Item{}, false
	}
	resp, err := p.client.R().SetContext(ctx).Get(detailURL)
	if err != nil || resp.IsError() {
		return model.Item{}, false
	}
	item := parseGenericDetail(resp.String(), detailURL, capturedAt)
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Content) == "" {
		return model.Item{}, false
	}
	return item, true
}

func parseGenericDetail(html, detailURL string, capturedAt time.Time) model.Item {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return model.Item{}
	}
	item := model.Item{
		SourceType:         provider.SourceTypeJin10Full,
		SourceURL:          detailURL,
		DetailURL:          detailURL,
		CapturedAt:         capturedAt,
		ExternalSourceHost: hostOf(detailURL),
	}
	item.Title = cleanText(doc.Find("h1,.content-title .flash-title,.jin10-news-cdetails-title,title").First().Text())
	item.Title = strings.TrimSuffix(item.Title, " - 金十数据")
	item.Summary = cleanText(doc.Find("meta[name=description]").AttrOr("content", ""))
	if item.Summary == "" {
		item.Summary = cleanText(doc.Find(".jin10-news-cdetails-introduction,.summary,.description").First().Text())
	}
	parts := make([]string, 0, 12)
	doc.Find(".jin10-news-cdetails-content p,.jin10-news-cdetails-content li,.article-content p,.content p,.content-title div").Each(func(_ int, s *goquery.Selection) {
		text := cleanText(s.Text())
		if text != "" && text != item.Title {
			parts = append(parts, text)
		}
	})
	if len(parts) == 0 {
		parts = append(parts, cleanText(doc.Find("article,main,body").First().Text()))
	}
	item.Content = strings.TrimSpace(strings.Join(parts, "\n"))
	if item.Summary == "" {
		item.Summary = firstRunes(item.Content, 180)
	}
	item.PublishTime = cleanText(doc.Find("time,.content-time,.publish-time,.jin10-news-cdetails-time").First().Text())
	item.RawPayload = firstRunes(cleanText(html), 4000)
	return item
}

func mergeItem(target *model.Item, detail model.Item) {
	if strings.TrimSpace(detail.Title) != "" {
		target.Title = detail.Title
	}
	if strings.TrimSpace(detail.Content) != "" {
		target.Content = detail.Content
	}
	if strings.TrimSpace(target.Summary) == "" && strings.TrimSpace(detail.Summary) != "" {
		target.Summary = detail.Summary
	}
	if strings.TrimSpace(target.PublishTime) == "" && strings.TrimSpace(detail.PublishTime) != "" {
		target.PublishTime = detail.PublishTime
	}
	if strings.TrimSpace(target.ExternalSourceHost) == "" {
		target.ExternalSourceHost = detail.ExternalSourceHost
	}
}

func (p *Provider) filterBackfill(items []model.Item, now time.Time) []model.Item {
	cutoff := now.AddDate(0, 0, -p.opts.BackfillDays)
	out := make([]model.Item, 0, len(items))
	for _, item := range items {
		if parsed, ok := parseItemTime(item); ok && parsed.Before(cutoff) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (p *Provider) updateState(ctx context.Context, items []model.Item, now time.Time) {
	if p.store == nil {
		return
	}
	_ = p.store.UpsertCrawlState(ctx, provider.SourceTypeJin10Full, stateLastRunAt, now.Format(time.RFC3339), now)
	latest := latestPublishTime(items)
	if !latest.IsZero() {
		_ = p.store.UpsertCrawlState(ctx, provider.SourceTypeJin10Full, stateLastSeen, latest.Format(time.RFC3339), now)
	}
}

func latestPublishTime(items []model.Item) time.Time {
	var latest time.Time
	for _, item := range items {
		parsed, ok := parseItemTime(item)
		if !ok || parsed.Before(latest) {
			continue
		}
		latest = parsed
	}
	return latest
}

func parseItemTime(item model.Item) (time.Time, bool) {
	for _, value := range []string{item.PublishTime, item.PublishTimeText} {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, "前") || strings.Contains(value, "刚刚") {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
			if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
				return parsed.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

type sitemapEntry struct {
	Location string `xml:"loc"`
	LastMod  string `xml:"lastmod"`
}

type sitemapURLSet struct {
	URLs []sitemapEntry `xml:"url"`
}

func parseSitemapURLs(data []byte) []sitemapEntry {
	var set sitemapURLSet
	if err := xml.Unmarshal(data, &set); err != nil {
		return nil
	}
	out := make([]sitemapEntry, 0, len(set.URLs))
	for _, entry := range set.URLs {
		entry.Location = strings.TrimSpace(entry.Location)
		if entry.Location != "" {
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastMod > out[j].LastMod
	})
	return out
}

func withPage(raw string, page int) string {
	if page <= 1 {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	query.Set("page", fmt.Sprintf("%d", page))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sitemapURLFor(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "https://www.jin10.com/sitemap.xml"
	}
	if isFlashAPIURL(raw) {
		return "https://www.jin10.com/sitemap.xml"
	}
	parsed.Path = "/sitemap.xml"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func isFlashAPIURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(parsed.Host), "flash-api.jin10.com") || strings.Contains(strings.ToLower(parsed.Path), "get_flash_list")
}

func isArticleURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := parsed.Host
	path := strings.ToLower(parsed.Path)
	if strings.Contains(path, "login") || strings.Contains(path, "vip") || strings.Contains(path, "pay") || strings.Contains(path, "open") {
		return false
	}
	if host == "xnews.jin10.com" {
		return strings.Contains(path, "/details/")
	}
	if host == "flash.jin10.com" {
		return strings.Contains(path, "/detail/")
	}
	return strings.Contains(path, "detail") || strings.Contains(path, "news") || strings.Contains(path, "article")
}

func dedupe(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	out := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.DetailURL)
		if key == "" {
			key = item.Title + "|" + item.PublishTime + "|" + item.Content
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (p *Provider) pause(ctx context.Context) {
	if p.opts.RateLimit <= 0 {
		return
	}
	timer := time.NewTimer(p.opts.RateLimit)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func cleanText(raw string) string {
	raw = strings.ReplaceAll(raw, "&nbsp;", " ")
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	raw = reHTMLTag.ReplaceAllString(raw, " ")
	raw = reWhitespace.ReplaceAllString(raw, " ")
	return strings.TrimSpace(raw)
}

func firstRunes(raw string, limit int) string {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func hostOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func allowedHosts(opts Options) map[string]struct{} {
	hosts := map[string]struct{}{
		"www.jin10.com":   {},
		"flash.jin10.com": {},
		"xnews.jin10.com": {},
	}
	for _, raw := range []string{opts.FlashURL, opts.HeadlineURL} {
		if host := hostOf(raw); host != "" {
			hosts[host] = struct{}{}
		}
	}
	return hosts
}

func isAllowedHost(raw string, hosts map[string]struct{}) bool {
	host := hostOf(raw)
	_, ok := hosts[host]
	return ok
}
