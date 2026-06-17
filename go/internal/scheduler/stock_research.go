package scheduler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (w *Worker) runStockResearchCrawl(ctx context.Context) error {
	end := stockResearchToday()
	start := end.AddDate(0, 0, -7)
	return w.runStockResearchCrawlForRange(ctx, stockResearchCrawlOptions{
		Start: start.Format("2006-01-02"),
		End:   end.Format("2006-01-02"),
	})
}

func (w *Worker) runStockResearchBackfill(ctx context.Context, opts stockResearchCrawlOptions) error {
	if strings.TrimSpace(opts.End) == "" {
		opts.End = stockResearchToday().Format("2006-01-02")
	}
	if strings.TrimSpace(opts.Start) == "" {
		day, err := time.Parse("2006-01-02", opts.End)
		if err != nil {
			day = stockResearchToday()
		}
		opts.Start = day.AddDate(-1, 0, 0).Format("2006-01-02")
	}
	return w.runStockResearchCrawlForRange(ctx, opts)
}

type stockResearchCrawlOptions struct {
	Code    string
	Company string
	Start   string
	End     string
}

func (w *Worker) runStockResearchCrawlForRange(ctx context.Context, opts stockResearchCrawlOptions) error {
	items := make([]model.StockResearchSurvey, 0)
	var errs []string
	if externalItems, err := w.fetchExternalStockResearch(ctx, opts); err != nil {
		if strings.TrimSpace(w.cfg.StockResearchURL) != "" {
			errs = append(errs, err.Error())
		}
	} else {
		items = append(items, externalItems...)
	}
	if w.cfg.StockResearchPublicEnabled {
		if sinaItems, err := w.fetchSinaFinanceReports(ctx); err != nil {
			errs = append(errs, "sina: "+err.Error())
		} else {
			items = append(items, filterStockResearchItems(sinaItems, opts)...)
		}
		if sohuItems, err := w.fetchSohuFinanceReports(ctx); err != nil {
			errs = append(errs, "sohu: "+err.Error())
		} else {
			items = append(items, filterStockResearchItems(sohuItems, opts)...)
		}
	}
	items = dedupeStockResearch(items)
	if len(items) == 0 && len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	payload := map[string]any{"items": items}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/internal/stock-research/batch")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock research upsert failed: %s", resp.Status())
	}
	log.Info().
		Str("start", opts.Start).
		Str("end", opts.End).
		Str("code", opts.Code).
		Str("company", opts.Company).
		Int("items", len(items)).
		Msg("stock research crawl completed")
	return nil
}

func (w *Worker) fetchExternalStockResearch(ctx context.Context, opts stockResearchCrawlOptions) ([]model.StockResearchSurvey, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.StockResearchURL), "/")
	if baseURL == "" {
		return nil, nil
	}
	query := url.Values{}
	query.Set("start", opts.Start)
	query.Set("end", opts.End)
	if opts.Code != "" {
		query.Set("code", opts.Code)
	}
	if opts.Company != "" {
		query.Set("company", opts.Company)
	}
	if token := strings.TrimSpace(w.cfg.TuShareToken); token != "" {
		query.Set("tushare_token", token)
	}
	resp, err := w.client.R().
		SetContext(ctx).
		Get(baseURL + "/api/stock-research?" + query.Encode())
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("stock research endpoint failed: %s", resp.Status())
	}
	var envelope struct {
		Items []model.StockResearchSurvey `json:"items"`
		Data  struct {
			Items []model.StockResearchSurvey `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Items) == 0 && len(envelope.Data.Items) > 0 {
		envelope.Items = envelope.Data.Items
	}
	return normalizeStockResearchSourceItems(envelope.Items, "akshare_stock_research"), nil
}

func (w *Worker) fetchSinaFinanceReports(ctx context.Context) ([]model.StockResearchSurvey, error) {
	return w.fetchFinanceReportHTML(ctx, w.cfg.SinaFinanceReportURL, "sina_finance_report")
}

func (w *Worker) fetchSohuFinanceReports(ctx context.Context) ([]model.StockResearchSurvey, error) {
	return w.fetchFinanceReportHTML(ctx, w.cfg.SohuFinanceReportURL, "sohu_finance_report")
}

func (w *Worker) fetchFinanceReportHTML(ctx context.Context, rawURL string, sourceType string) ([]model.StockResearchSurvey, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", w.cfg.UserAgent).
		Get(rawURL)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("%s failed: %s", sourceType, resp.Status())
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.String()))
	if err != nil {
		return nil, err
	}
	items := parseFinanceReportDocument(doc, rawURL, sourceType, time.Now().UTC())
	return normalizeStockResearchSourceItems(items, sourceType), nil
}

func parseFinanceReportDocument(doc *goquery.Document, pageURL string, sourceType string, now time.Time) []model.StockResearchSurvey {
	items := make([]model.StockResearchSurvey, 0)
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := make([]string, 0)
		row.Find("td").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, cleanStockResearchText(cell.Text()))
		})
		if len(cells) < 3 {
			return
		}
		link := row.Find("a").First()
		title := cleanStockResearchText(link.Text())
		if title == "" {
			title = firstMeaningfulStockResearchCell(cells)
		}
		if title == "" || !looksLikeResearchTitle(title) {
			return
		}
		href, _ := link.Attr("href")
		item := model.StockResearchSurvey{
			Kind:         "report",
			Title:        title,
			ResearchDate: firstStockResearchDate(cells),
			PublishTime:  firstStockResearchDate(cells),
			Institution:  stockResearchCell(cells, len(cells)-2),
			Analyst:      stockResearchCell(cells, len(cells)-1),
			SourceURL:    resolveStockResearchURL(pageURL, href),
			SourceType:   sourceType,
			RawPayload:   rawStockResearchPayload(cells),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if item.SourceURL == "" {
			item.SourceURL = pageURL
		}
		item.SourceKey = stockResearchSourceKey(item)
		items = append(items, item)
	})
	if len(items) > 0 {
		return items
	}
	doc.Find("a").Each(func(_ int, link *goquery.Selection) {
		title := cleanStockResearchText(link.Text())
		if !looksLikeResearchTitle(title) {
			return
		}
		href, _ := link.Attr("href")
		item := model.StockResearchSurvey{
			Kind:       "report",
			Title:      title,
			SourceURL:  resolveStockResearchURL(pageURL, href),
			SourceType: sourceType,
			RawPayload: "{}",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		item.SourceKey = stockResearchSourceKey(item)
		items = append(items, item)
	})
	return items
}

func normalizeStockResearchSourceItems(items []model.StockResearchSurvey, fallbackSource string) []model.StockResearchSurvey {
	now := time.Now().UTC()
	for i := range items {
		items[i].Code = strings.TrimSpace(items[i].Code)
		items[i].Name = strings.TrimSpace(items[i].Name)
		items[i].Kind = stockResearchKindText(items[i].Kind)
		items[i].Title = cleanStockResearchText(items[i].Title)
		items[i].Institution = cleanStockResearchText(items[i].Institution)
		items[i].Analyst = cleanStockResearchText(items[i].Analyst)
		items[i].Rating = cleanStockResearchText(items[i].Rating)
		items[i].TargetPrice = cleanStockResearchText(items[i].TargetPrice)
		items[i].ResearchDate = cleanStockResearchText(items[i].ResearchDate)
		items[i].PublishTime = cleanStockResearchText(items[i].PublishTime)
		items[i].SourceURL = strings.TrimSpace(items[i].SourceURL)
		items[i].SourceType = strings.TrimSpace(items[i].SourceType)
		if items[i].SourceType == "" {
			items[i].SourceType = fallbackSource
		}
		items[i].Summary = cleanStockResearchText(items[i].Summary)
		if strings.TrimSpace(items[i].RawPayload) == "" {
			items[i].RawPayload = "{}"
		}
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = now
		}
		if items[i].UpdatedAt.IsZero() {
			items[i].UpdatedAt = now
		}
		if strings.TrimSpace(items[i].SourceKey) == "" {
			items[i].SourceKey = stockResearchSourceKey(items[i])
		}
	}
	return items
}

func filterStockResearchItems(items []model.StockResearchSurvey, opts stockResearchCrawlOptions) []model.StockResearchSurvey {
	code := strings.TrimSpace(opts.Code)
	company := strings.TrimSpace(opts.Company)
	if code == "" && company == "" {
		return items
	}
	out := make([]model.StockResearchSurvey, 0, len(items))
	for _, item := range items {
		blob := strings.ToLower(strings.Join([]string{item.Code, item.Name, item.Title, item.Summary, item.RawPayload}, " "))
		if code != "" && strings.Contains(blob, strings.ToLower(code)) {
			out = append(out, item)
			continue
		}
		if company != "" && strings.Contains(blob, strings.ToLower(company)) {
			out = append(out, item)
		}
	}
	return out
}

func dedupeStockResearch(items []model.StockResearchSurvey) []model.StockResearchSurvey {
	seen := map[string]struct{}{}
	out := make([]model.StockResearchSurvey, 0, len(items))
	for _, item := range items {
		key := item.SourceType + "|" + item.SourceKey
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.SourceKey) == "" {
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

func stockResearchToday() time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return time.Now().In(location)
}

func stockResearchKindText(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "survey", "调研":
		return "survey"
	default:
		return "report"
	}
}

func stockResearchSourceKey(item model.StockResearchSurvey) string {
	raw := strings.Join([]string{item.SourceType, item.Code, item.Name, item.Title, item.ResearchDate, item.PublishTime, item.SourceURL}, "|")
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func cleanStockResearchText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func firstMeaningfulStockResearchCell(cells []string) string {
	for _, cell := range cells {
		if cell != "" && !isStockResearchDate(cell) {
			return cell
		}
	}
	return ""
}

func firstStockResearchDate(cells []string) string {
	for _, cell := range cells {
		if isStockResearchDate(cell) {
			return cell
		}
	}
	return ""
}

func isStockResearchDate(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 8 {
		return false
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006.01.02"} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func looksLikeResearchTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	for _, keyword := range []string{"研报", "研究", "评级", "调研", "点评", "深度", "策略"} {
		if strings.Contains(title, keyword) {
			return true
		}
	}
	return false
}

func stockResearchCell(cells []string, index int) string {
	if index < 0 || index >= len(cells) {
		return ""
	}
	return cells[index]
}

func rawStockResearchPayload(cells []string) string {
	payload, err := json.Marshal(map[string]any{"cells": cells})
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func resolveStockResearchURL(pageURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	if parsed, err := url.Parse(href); err == nil && parsed.Scheme != "" {
		return parsed.String()
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return href
	}
	resolved, err := base.Parse(href)
	if err != nil {
		return href
	}
	return resolved.String()
}
