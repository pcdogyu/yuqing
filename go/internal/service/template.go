package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type templateExecContext struct {
	Keyword  string
	Page     int
	Template model.CrawlTemplate
	Config   model.CrawlTemplateConfig
	Now      time.Time
}

func (c *Crawler) RunTemplate(ctx context.Context, tpl model.CrawlTemplate, keyword string) (model.CrawlSummary, error) {
	return c.executeTemplate(ctx, tpl, keyword, false)
}

func (c *Crawler) PreviewTemplate(ctx context.Context, tpl model.CrawlTemplate, keyword string) (model.CrawlSummary, error) {
	return c.executeTemplate(ctx, tpl, keyword, true)
}

func (c *Crawler) executeTemplate(ctx context.Context, tpl model.CrawlTemplate, keyword string, preview bool) (model.CrawlSummary, error) {
	cfg, err := parseTemplateConfig(tpl.ConfigJSON)
	if err != nil {
		return model.CrawlSummary{}, err
	}
	sourceType := nonEmpty(cfg.SourceType, tpl.SourceType, "custom")
	startedAt := time.Now().UTC()
	runID := int64(0)
	if !preview {
		runID, err = c.store.StartCrawlTemplateRun(ctx, sourceType, tpl.ID, tpl.Name, tpl.ConfigJSON, startedAt)
		if err != nil {
			return model.CrawlSummary{}, err
		}
	}

	items, fetchErr := c.fetchTemplateItems(ctx, tpl, cfg, keyword)
	summary := model.CrawlSummary{
		SourceType:   sourceType,
		FetchedCount: len(items),
		RunID:        runID,
	}
	if fetchErr != nil {
		summary.ErrorText = fetchErr.Error()
		if !preview && runID > 0 {
			finishedAt := time.Now().UTC()
			_ = c.store.FinishCrawlRun(ctx, runID, "failed", len(items), 0, 0, fetchErr.Error(), finishedAt)
			_ = c.store.RecordTaskRun(ctx, "crawl-template:"+tpl.Name, "failed", fetchErr.Error(), startedAt, &finishedAt)
		}
		return summary, fetchErr
	}

	if preview {
		summary.Items = trimPreviewItems(items)
		return summary, nil
	}

	now := time.Now().UTC()
	rules, _ := c.store.ListActiveMonitorRules(ctx)
	for idx := range items {
		items[idx].SourceType = sourceType
		items[idx].CapturedAt = now
		if items[idx].CreatedAt.IsZero() {
			items[idx].CreatedAt = now
		}
		items[idx].UpdatedAt = now
		items[idx].SourceKey = buildSourceKey(items[idx])
		items[idx].Title = strings.TrimSpace(items[idx].Title)
		items[idx].Content = strings.TrimSpace(items[idx].Content)
		items[idx].Summary = strings.TrimSpace(items[idx].Summary)
	}
	inserted, updated, err := c.store.UpsertItems(ctx, items)
	if err != nil {
		summary.ErrorText = err.Error()
		if runID > 0 {
			finishedAt := time.Now().UTC()
			_ = c.store.FinishCrawlRun(ctx, runID, "failed", len(items), 0, 0, err.Error(), finishedAt)
			_ = c.store.RecordTaskRun(ctx, "crawl-template:"+tpl.Name, "failed", err.Error(), startedAt, &finishedAt)
		}
		return summary, err
	}

	matches := make(map[int64][]string)
	for index := range items {
		for _, rule := range rules {
			if ruleMatches(rule, sourceType, items[index]) {
				matches[rule.ID] = append(matches[rule.ID], items[index].SourceKey)
			}
		}
	}
	for _, rule := range rules {
		keys := matches[rule.ID]
		if len(keys) == 0 {
			continue
		}
		_ = c.store.LinkItemsToProjects(ctx, keys, []int64{rule.ProjectID}, rule.ID)
	}

	summary.InsertedCount = inserted
	summary.UpdatedCount = updated
	if runID > 0 {
		finishedAt := time.Now().UTC()
		if err := c.store.FinishCrawlRun(ctx, runID, "success", len(items), inserted, updated, "", finishedAt); err != nil {
			return summary, err
		}
		_ = c.store.RecordTaskRun(ctx, "crawl-template:"+tpl.Name, "success", "crawl completed", startedAt, &finishedAt)
	}
	return summary, nil
}

func (c *Crawler) fetchTemplateItems(ctx context.Context, tpl model.CrawlTemplate, cfg model.CrawlTemplateConfig, keyword string) ([]model.Item, error) {
	pageStart := cfg.Pagination.Start
	if pageStart <= 0 {
		pageStart = 1
	}
	pageEnd := cfg.Pagination.End
	if pageEnd < pageStart {
		pageEnd = pageStart
	}
	step := cfg.Pagination.Step
	if step <= 0 {
		step = 1
	}
	pages := []int{pageStart}
	if cfg.Pagination.Enabled {
		pages = pages[:0]
		for page := pageStart; page <= pageEnd; page += step {
			pages = append(pages, page)
		}
	}

	items := make([]model.Item, 0)
	for _, page := range pages {
		execCtx := templateExecContext{
			Keyword:  keyword,
			Page:     page,
			Template: tpl,
			Config:   cfg,
			Now:      time.Now().UTC(),
		}
		pageItems, err := c.fetchTemplatePage(ctx, execCtx)
		if err != nil {
			return items, err
		}
		items = append(items, pageItems...)
	}
	return dedupeTemplateItems(items), nil
}

func (c *Crawler) fetchTemplatePage(ctx context.Context, execCtx templateExecContext) ([]model.Item, error) {
	pageURL, headers, cookies, body, method, err := renderRequestParts(execCtx)
	if err != nil {
		return nil, err
	}

	req := c.client.R().SetContext(ctx)
	for key, value := range headers {
		req.SetHeader(key, value)
	}
	for key, value := range cookies {
		req.SetCookie(&http.Cookie{Name: key, Value: value})
	}
	if body != "" {
		req.SetBody(body)
	}

	resp, err := req.Execute(method, pageURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("template fetch failed: %s", resp.Status())
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.String()))
	if err != nil {
		return nil, err
	}

	container := doc.Selection
	if strings.TrimSpace(execCtx.Config.ListSelector) != "" {
		container = doc.Find(execCtx.Config.ListSelector)
	}
	if container.Length() == 0 {
		container = doc.Find("body")
	}

	items := make([]model.Item, 0)
	if strings.TrimSpace(execCtx.Config.ListSelector) == "" {
		items = append(items, c.extractTemplateItem(execCtx, doc.Selection, nil)...)
		return items, nil
	}

	container.Each(func(_ int, node *goquery.Selection) {
		items = append(items, c.extractTemplateItem(execCtx, node, doc.Selection)...)
	})
	return items, nil
}

func (c *Crawler) extractTemplateItem(execCtx templateExecContext, node *goquery.Selection, pageDoc *goquery.Selection) []model.Item {
	item := model.Item{
		SourceType: nonEmpty(execCtx.Config.SourceType, execCtx.Template.SourceType, "custom"),
		SourceURL:  "",
		CapturedAt: execCtx.Now,
	}
	item.SourceURL, _ = renderValue(execCtx, firstConfiguredValue(execCtx.Config.Query, "source_url"))
	item.Title = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "title", "list")
	item.Summary = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "summary", "list")
	item.Content = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "content", "detail")
	item.PublishTime = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "publish_time", "list", "detail")
	item.PublishTimeText = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "publish_time_text", "list", "detail")
	item.TagFlags = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "tag_flags", "list")
	item.FromText = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "from_text", "list")
	item.RawPayload = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "raw_payload", "list", "detail")
	item.ExternalSourceHost = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "external_source_host", "detail", "list")
	item.DetailURL = resolveFieldValue(execCtx, node, pageDoc, execCtx.Config.Fields, "detail_url", "list")
	if item.DetailURL == "" && strings.TrimSpace(execCtx.Config.DetailSelector) != "" {
		if href, exists := node.Find(execCtx.Config.DetailSelector).First().Attr("href"); exists {
			item.DetailURL = resolveURL(execCtx, href)
		}
	}
	if item.DetailURL == "" {
		for _, field := range execCtx.Config.Fields {
			if strings.EqualFold(field.Name, "url") && strings.TrimSpace(field.Selector) != "" {
				if value, ok := extractNodeValue(node, field); ok {
					item.DetailURL = resolveURL(execCtx, value)
					break
				}
			}
		}
	}
	if item.DetailURL == "" {
		item.DetailURL = execCtx.Config.BaseURL
	}
	if strings.TrimSpace(item.Content) == "" {
		item.Content = item.Summary
	}

	if strings.TrimSpace(item.DetailURL) != "" && hasDetailFields(execCtx.Config.Fields) {
		if detailDoc, err := c.fetchTemplateDetail(execCtx, item.DetailURL); err == nil {
			applyDetailFields(execCtx, node, detailDoc, &item)
		}
	}

	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Content) == "" && strings.TrimSpace(item.DetailURL) == "" {
		return nil
	}
	return []model.Item{item}
}

func (c *Crawler) fetchTemplateDetail(execCtx templateExecContext, detailURL string) (*goquery.Selection, error) {
	resp, err := c.client.R().SetContext(context.Background()).Get(detailURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("detail fetch failed: %s", resp.Status())
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.String()))
	if err != nil {
		return nil, err
	}
	return doc.Selection, nil
}

func renderRequestParts(execCtx templateExecContext) (string, map[string]string, map[string]string, string, string, error) {
	method := strings.ToUpper(strings.TrimSpace(execCtx.Config.Method))
	if method == "" {
		method = "GET"
	}
	pageURL, err := renderValue(execCtx, execCtx.Config.BaseURL)
	if err != nil {
		return "", nil, nil, "", "", err
	}
	headers := renderKVMap(execCtx, execCtx.Config.Headers)
	cookies := renderKVMap(execCtx, execCtx.Config.Cookies)
	query := renderKVMap(execCtx, execCtx.Config.Query)
	if len(query) > 0 {
		parsed, err := url.Parse(pageURL)
		if err != nil {
			return "", nil, nil, "", "", err
		}
		values := parsed.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		parsed.RawQuery = values.Encode()
		pageURL = parsed.String()
	}
	body, err := renderValue(execCtx, execCtx.Config.Body)
	if err != nil {
		return "", nil, nil, "", "", err
	}
	return pageURL, headers, cookies, body, method, nil
}

func renderKVMap(execCtx templateExecContext, source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		rendered, err := renderValue(execCtx, value)
		if err != nil {
			continue
		}
		out[key] = rendered
	}
	return out
}

func renderValue(execCtx templateExecContext, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	tpl, err := template.New("value").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	data := map[string]any{
		"Keyword":  execCtx.Keyword,
		"Page":     execCtx.Page,
		"Template": execCtx.Template,
		"Config":   execCtx.Config,
		"Now":      execCtx.Now,
	}
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func resolveFieldValue(execCtx templateExecContext, node, pageDoc *goquery.Selection, fields []model.CrawlTemplateField, name string, scopes ...string) string {
	for _, field := range fields {
		if !strings.EqualFold(field.Name, name) {
			continue
		}
		if len(scopes) > 0 && !matchesScope(field.Scope, scopes...) {
			continue
		}
		if value, ok := extractFieldValue(execCtx, node, pageDoc, field); ok {
			return value
		}
	}
	return ""
}

func extractFieldValue(execCtx templateExecContext, node, pageDoc *goquery.Selection, field model.CrawlTemplateField) (string, bool) {
	scope := strings.ToLower(strings.TrimSpace(field.Scope))
	target := node
	if scope == "page" && pageDoc != nil {
		target = pageDoc
	}
	raw := strings.TrimSpace(field.Value)
	switch strings.ToLower(strings.TrimSpace(field.From)) {
	case "", "text":
		if strings.TrimSpace(field.Selector) != "" && target != nil {
			raw = cleanText(target.Find(field.Selector).First().Text())
		}
	case "html":
		if strings.TrimSpace(field.Selector) != "" && target != nil {
			html, err := target.Find(field.Selector).First().Html()
			if err == nil {
				raw = html
			}
		}
	case "attr":
		if strings.TrimSpace(field.Selector) != "" && target != nil {
			if value, ok := target.Find(field.Selector).First().Attr(field.Attr); ok {
				raw = value
			}
		}
	case "const":
		raw = field.Value
	}
	if strings.TrimSpace(raw) == "" {
		return "", !field.Required
	}
	rendered, err := renderValue(execCtx, raw)
	if err != nil {
		if field.Required {
			return "", false
		}
		return "", true
	}
	if field.Trim {
		rendered = strings.TrimSpace(rendered)
	}
	if field.Prefix != "" {
		rendered = field.Prefix + rendered
	}
	if field.Suffix != "" {
		rendered += field.Suffix
	}
	return strings.TrimSpace(rendered), true
}

func extractNodeValue(node *goquery.Selection, field model.CrawlTemplateField) (string, bool) {
	if node == nil || strings.TrimSpace(field.Selector) == "" {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(field.From)) {
	case "attr":
		if value, ok := node.Find(field.Selector).First().Attr(field.Attr); ok {
			return value, true
		}
	default:
		return cleanText(node.Find(field.Selector).First().Text()), true
	}
	return "", false
}

func matchesScope(scope string, scopes ...string) bool {
	if scope == "" {
		scope = "list"
	}
	for _, candidate := range scopes {
		if strings.EqualFold(scope, candidate) {
			return true
		}
	}
	return false
}

func hasDetailFields(fields []model.CrawlTemplateField) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.Scope), "detail") {
			return true
		}
	}
	return false
}

func applyDetailFields(execCtx templateExecContext, listNode, detailDoc *goquery.Selection, item *model.Item) {
	for _, field := range execCtx.Config.Fields {
		if !strings.EqualFold(strings.TrimSpace(field.Scope), "detail") {
			continue
		}
		if value, ok := extractFieldValue(execCtx, detailDoc, detailDoc, field); ok {
			switch strings.ToLower(strings.TrimSpace(field.Name)) {
			case "title":
				item.Title = value
			case "content":
				item.Content = value
			case "summary":
				item.Summary = value
			case "publish_time":
				item.PublishTime = value
			case "publish_time_text":
				item.PublishTimeText = value
			case "detail_url":
				item.DetailURL = resolveURL(execCtx, value)
			case "source_url":
				item.SourceURL = resolveURL(execCtx, value)
			case "from_text":
				item.FromText = value
			case "tag_flags":
				item.TagFlags = value
			case "external_source_host":
				item.ExternalSourceHost = value
			case "raw_payload":
				item.RawPayload = value
			}
		}
	}
	if strings.TrimSpace(item.Content) == "" {
		item.Content = item.Summary
	}
	if strings.TrimSpace(item.Summary) == "" {
		item.Summary = item.Content
	}
	_ = listNode
}

func dedupeTemplateItems(items []model.Item) []model.Item {
	seen := make(map[string]struct{}, len(items))
	out := make([]model.Item, 0, len(items))
	for _, item := range items {
		key := item.DetailURL + "|" + item.Title + "|" + item.PublishTime + "|" + item.Content
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func trimPreviewItems(items []model.Item) []model.Item {
	if len(items) <= 10 {
		return items
	}
	return items[:10]
}

func resolveURL(execCtx templateExecContext, raw string) string {
	rendered, err := renderValue(execCtx, raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return ""
	}
	if strings.HasPrefix(rendered, "//") {
		return "https:" + rendered
	}
	if parsed, err := url.Parse(rendered); err == nil && parsed.Scheme != "" {
		return parsed.String()
	}
	if base, err := url.Parse(execCtx.Config.BaseURL); err == nil {
		if resolved, err := base.Parse(rendered); err == nil {
			return resolved.String()
		}
	}
	return rendered
}

func parseTemplateConfig(raw string) (model.CrawlTemplateConfig, error) {
	cfg := model.CrawlTemplateConfig{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg, errors.New("template config required")
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return model.CrawlTemplateConfig{}, err
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}
	if cfg.Pagination.Step <= 0 {
		cfg.Pagination.Step = 1
	}
	return cfg, nil
}

func firstConfiguredValue(values map[string]string, key string) string {
	if len(values) == 0 {
		return ""
	}
	return values[key]
}
