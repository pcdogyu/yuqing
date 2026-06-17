package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

var aStockRecommendationSources = []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun"}

func (w *Worker) runAStockRecommendation(ctx context.Context, period string) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockRecommendationForDate(ctx, time.Now().In(location).Format("2006-01-02"), period)
}

func (w *Worker) runAStockAuctionCrawl(ctx context.Context) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockAuctionCrawlForDate(ctx, time.Now().In(location).Format("2006-01-02"))
}

func (w *Worker) runAStockAuctionCrawlForDate(ctx context.Context, tradeDate string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return fmt.Errorf("YUQING_ASTOCK_AUCTION_URL not configured")
	}
	var payload struct {
		Date  string                      `json:"date"`
		Items []model.AStockAuctionAmount `json:"items"`
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("date", tradeDate).
		Get(baseURL + "/api/a-stock/auction")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("akshare auction endpoint failed: %s", resp.Status())
	}
	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return err
	}
	if payload.Date == "" {
		payload.Date = tradeDate
	}
	for i := range payload.Items {
		if payload.Items[i].TradeDate == "" {
			payload.Items[i].TradeDate = payload.Date
		}
		if payload.Items[i].FetchedAt.IsZero() {
			payload.Items[i].FetchedAt = time.Now().UTC()
		}
	}
	writeResp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/admin/a-stock/auction")
	if err != nil {
		return err
	}
	if !writeResp.IsSuccess() {
		return fmt.Errorf("content auction upsert failed: %s", writeResp.Status())
	}
	okCount := 0
	for _, item := range payload.Items {
		if strings.TrimSpace(item.Status) == "" || strings.EqualFold(item.Status, "ok") {
			okCount++
		}
	}
	log.Info().
		Str("trade_date", payload.Date).
		Int("total", len(payload.Items)).
		Int("ok", okCount).
		Msg("a-stock auction amounts crawled")
	return nil
}

type aStockHoldingCrawlOptions struct {
	Code        string
	Period      string
	StartPeriod string
	EndPeriod   string
}

func (w *Worker) runAStockHoldingsCrawl(ctx context.Context) error {
	return w.runAStockHoldingsBackfill(ctx, aStockHoldingCrawlOptions{})
}

func (w *Worker) runAStockHoldingsBackfill(ctx context.Context, opts aStockHoldingCrawlOptions) error {
	periods := aStockHoldingPeriods(opts)
	if len(periods) == 0 {
		periods = latestAStockHoldingPeriods(4, time.Now().In(aStockLocation()))
	}
	items := make([]model.StockInstitutionHolding, 0)
	for _, period := range periods {
		fetched, err := w.fetchExternalAStockHoldings(ctx, period, opts.Code)
		if err != nil {
			return err
		}
		items = append(items, fetched...)
	}
	payload := map[string]any{"items": items}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/internal/a-stock/holdings/batch")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock holdings upsert failed: %s", resp.Status())
	}
	log.Info().
		Str("code", opts.Code).
		Str("period", opts.Period).
		Str("start_period", opts.StartPeriod).
		Str("end_period", opts.EndPeriod).
		Int("items", len(items)).
		Msg("a-stock institution holdings crawled")
	return nil
}

func (w *Worker) fetchExternalAStockHoldings(ctx context.Context, period string, code string) ([]model.StockInstitutionHolding, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockHoldingURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("YUQING_ASTOCK_HOLDING_URL not configured")
	}
	req := w.client.R().
		SetContext(ctx).
		SetQueryParam("period", period)
	if strings.TrimSpace(code) != "" {
		req.SetQueryParam("code", strings.TrimSpace(code))
	}
	if token := strings.TrimSpace(w.cfg.TuShareToken); token != "" {
		req.SetQueryParam("tushare_token", token)
	}
	resp, err := req.Get(baseURL + "/api/a-stock/holdings")
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("a-stock holdings endpoint failed: %s", resp.Status())
	}
	var envelope struct {
		Items []model.StockInstitutionHolding `json:"items"`
		Data  struct {
			Items []model.StockInstitutionHolding `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Items) == 0 && len(envelope.Data.Items) > 0 {
		envelope.Items = envelope.Data.Items
	}
	now := time.Now().UTC()
	for i := range envelope.Items {
		if envelope.Items[i].ReportPeriod == "" {
			envelope.Items[i].ReportPeriod = period
		}
		if envelope.Items[i].SourceType == "" {
			envelope.Items[i].SourceType = "akshare_stock_holding"
		}
		if envelope.Items[i].FetchedAt.IsZero() {
			envelope.Items[i].FetchedAt = now
		}
	}
	return envelope.Items, nil
}

func (w *Worker) runAStockRecommendationForDate(ctx context.Context, strategyDate string, period string) error {
	for _, sourceType := range aStockRecommendationSources {
		if err := w.runCrawl(ctx, sourceType); err != nil {
			return err
		}
	}
	start, end, label, err := aStockRecommendationWindow(strategyDate, period)
	if err != nil {
		return err
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("page", "1").
		SetQueryParam("page_size", "200").
		SetQueryParam("start", start.UTC().Format(time.RFC3339)).
		SetQueryParam("end", end.UTC().Format(time.RFC3339)).
		Get(w.cfg.ContentURL + "/api/v1/articles")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("a-stock %s recommendation content warmup failed: %s", label, resp.Status())
	}
	log.Info().
		Str("strategy_date", strategyDate).
		Str("period", period).
		Str("window", label).
		Msg("a-stock recommendation window generated")
	return nil
}

func aStockHoldingPeriods(opts aStockHoldingCrawlOptions) []string {
	if period := normalizeAStockHoldingPeriod(opts.Period); period != "" {
		return []string{period}
	}
	start := normalizeAStockHoldingPeriod(opts.StartPeriod)
	end := normalizeAStockHoldingPeriod(opts.EndPeriod)
	if start == "" && end == "" {
		return nil
	}
	if start == "" {
		start = end
	}
	if end == "" {
		end = start
	}
	startTime, err := time.Parse("20060102", start)
	if err != nil {
		return nil
	}
	endTime, err := time.Parse("20060102", end)
	if err != nil {
		return nil
	}
	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime
	}
	out := make([]string, 0)
	for cursor := startTime; !cursor.After(endTime); cursor = cursor.AddDate(0, 3, 0) {
		period := quarterEndDate(cursor)
		if !period.Before(startTime) && !period.After(endTime) {
			out = append(out, period.Format("20060102"))
		}
	}
	return out
}

func latestAStockHoldingPeriods(limit int, now time.Time) []string {
	if limit <= 0 {
		return nil
	}
	periods := make([]string, 0, limit)
	cursor := previousCompletedQuarterEnd(now)
	for len(periods) < limit {
		periods = append(periods, cursor.Format("20060102"))
		cursor = previousQuarterEnd(cursor)
	}
	return periods
}

func previousCompletedQuarterEnd(now time.Time) time.Time {
	current := quarterEndDate(now)
	if !current.After(now) {
		return current
	}
	return previousQuarterEnd(current)
}

func previousQuarterEnd(value time.Time) time.Time {
	startOfQuarter := time.Date(value.Year(), time.Month(((int(value.Month())-1)/3)*3+1), 1, 0, 0, 0, 0, value.Location())
	return quarterEndDate(startOfQuarter.AddDate(0, -1, 0))
}

func quarterEndDate(value time.Time) time.Time {
	quarter := (int(value.Month())-1)/3 + 1
	endMonth := time.Month(quarter * 3)
	firstNextMonth := time.Date(value.Year(), endMonth, 1, 0, 0, 0, 0, value.Location()).AddDate(0, 1, 0)
	return firstNextMonth.AddDate(0, 0, -1)
}

func normalizeAStockHoldingPeriod(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "/", "")
	value = strings.ReplaceAll(value, ".", "")
	if len(value) == 8 {
		return value
	}
	return ""
}

func aStockLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return location
}

func aStockRecommendationWindow(strategyDate string, period string) (time.Time, time.Time, string, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	day, err := time.ParseInLocation("2006-01-02", strategyDate, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	switch period {
	case "afternoon":
		return time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 12, 50, 59, 0, location),
			"09:26-12:50", nil
	default:
		return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 59, 0, location),
			"08:00-09:30", nil
	}
}
