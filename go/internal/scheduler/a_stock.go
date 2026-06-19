package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

var aStockRecommendationSources = []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun", "wallstreetcn_a_stock", "cls_telegraph", "sina_finance_7x24"}

func (w *Worker) runAStockRecommendation(ctx context.Context, period string) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockRecommendationForDate(ctx, time.Now().In(location).Format("2006-01-02"), period)
}

func (w *Worker) runAStockAuctionCrawl(ctx context.Context) error {
	result, err := w.runAStockAuctionLatest(ctx)
	if err != nil {
		return err
	}
	if result.Skipped {
		return fmt.Errorf("a-stock auction crawl skipped for %s: %s", result.Date, result.Message)
	}
	log.Info().
		Str("trade_date", result.Date).
		Int("total", result.Total).
		Int("ok", result.OK).
		Msg("a-stock auction amounts crawled")
	return nil
}

func (w *Worker) runAStockAuctionLatest(ctx context.Context) (aStockAuctionCrawlResult, error) {
	return w.runAStockAuctionCrawlForDateResult(ctx, "")
}

func (w *Worker) runAStockAuctionCrawlForDate(ctx context.Context, tradeDate string) error {
	result, err := w.runAStockAuctionCrawlForDateResult(ctx, tradeDate)
	if err != nil {
		return err
	}
	if result.Skipped {
		return fmt.Errorf("a-stock auction crawl skipped for %s: %s", result.Date, result.Message)
	}
	log.Info().
		Str("trade_date", result.Date).
		Int("total", result.Total).
		Int("ok", result.OK).
		Msg("a-stock auction amounts crawled")
	return nil
}

type aStockAuctionCrawlResult struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	OK      int    `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
	Message string `json:"message,omitempty"`
}

type aStockTradingDayStatus struct {
	Date               string `json:"date"`
	IsTradingDay       bool   `json:"is_trading_day"`
	LatestTradingDay   string `json:"latest_trading_day"`
	PreviousTradingDay string `json:"previous_trading_day"`
	NextTradingDay     string `json:"next_trading_day"`
	Source             string `json:"source"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
}

type jobSkippedError struct {
	message string
}

func (e jobSkippedError) Error() string {
	return e.message
}

type aStockAuctionBackfillResult struct {
	Days      int                        `json:"days"`
	Succeeded int                        `json:"succeeded"`
	Skipped   int                        `json:"skipped"`
	Failed    int                        `json:"failed"`
	Results   []aStockAuctionCrawlResult `json:"results"`
	Errors    []string                   `json:"errors"`
}

func (w *Worker) runAStockAuctionCrawlForDateResult(ctx context.Context, tradeDate string) (aStockAuctionCrawlResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return aStockAuctionCrawlResult{}, fmt.Errorf("YUQING_ASTOCK_AUCTION_URL not configured")
	}
	req := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("limit", "0")
	if strings.TrimSpace(tradeDate) == "" {
		req.SetQueryParam("force", "1")
	}
	if strings.TrimSpace(tradeDate) != "" {
		req.SetQueryParam("date", tradeDate)
	}
	httpResp, err := req.Get(baseURL + "/api/a-stock/auction")
	if err != nil {
		return aStockAuctionCrawlResult{}, err
	}
	if !httpResp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(httpResp.Body(), httpResp.String())
		if message == "" {
			message = httpResp.Status()
		}
		if httpResp.StatusCode() == http.StatusUnprocessableEntity {
			return aStockAuctionCrawlResult{Date: tradeDate, Skipped: true, Message: message}, nil
		}
		return aStockAuctionCrawlResult{}, fmt.Errorf("akshare auction endpoint failed: %s", message)
	}
	payload, err := decodeAStockAuctionPayload(httpResp.Body())
	if err != nil {
		return aStockAuctionCrawlResult{}, err
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
	okCount := 0
	for _, item := range payload.Items {
		if isUsableAStockAuctionItem(item) {
			okCount++
		}
	}
	if len(payload.Items) == 0 || okCount == 0 {
		message := nonEmpty(strings.TrimSpace(payload.Message), strings.TrimSpace(payload.Warning))
		if message == "" {
			if len(payload.Items) == 0 {
				message = "AKShare 集合竞价接口未返回明细，未写入业务库"
			} else {
				message = fmt.Sprintf("AKShare 集合竞价接口返回 %d 条明细，但有效成交额/成交量为 0，未写入业务库", len(payload.Items))
			}
		}
		return aStockAuctionCrawlResult{Date: payload.Date, Total: len(payload.Items), OK: okCount, Skipped: true, Message: message}, nil
	}
	writeResp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/admin/a-stock/auction")
	if err != nil {
		return aStockAuctionCrawlResult{}, err
	}
	if !writeResp.IsSuccess() {
		return aStockAuctionCrawlResult{}, fmt.Errorf("content auction upsert failed: %s", writeResp.Status())
	}
	return aStockAuctionCrawlResult{Date: payload.Date, Total: len(payload.Items), OK: okCount}, nil
}

func (w *Worker) runAStockAuctionBackfill(ctx context.Context, days int, start string, end string) (aStockAuctionBackfillResult, error) {
	dates := aStockAuctionBackfillDates(days, start, end, time.Now().In(aStockLocation()))
	result := aStockAuctionBackfillResult{Days: len(dates), Results: make([]aStockAuctionCrawlResult, 0, len(dates))}
	for _, date := range dates {
		item, err := w.runAStockAuctionCrawlForDateResult(ctx, date)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", date, err))
			continue
		}
		if item.Skipped {
			result.Skipped++
			result.Results = append(result.Results, item)
			if strings.TrimSpace(item.Message) != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", date, item.Message))
			}
			continue
		}
		result.Succeeded++
		result.Results = append(result.Results, item)
	}
	if result.Succeeded == 0 && result.Failed > 0 {
		return result, fmt.Errorf("a-stock auction backfill failed: %s", strings.Join(result.Errors, "; "))
	}
	if result.Succeeded == 0 && result.Skipped > 0 {
		return result, fmt.Errorf("a-stock auction backfill skipped all dates without usable data: %s", strings.Join(result.Errors, "; "))
	}
	return result, nil
}

type aStockAuctionPayload struct {
	Date    string                      `json:"date"`
	Items   []model.AStockAuctionAmount `json:"items"`
	Message string                      `json:"message"`
	Warning string                      `json:"warning"`
}

func decodeAStockAuctionPayload(body []byte) (aStockAuctionPayload, error) {
	var payload aStockAuctionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		var items []model.AStockAuctionAmount
		if arrayErr := json.Unmarshal(body, &items); arrayErr == nil {
			payload.Items = items
			return payload, nil
		}
		return payload, err
	}
	if len(payload.Items) > 0 || payload.Date != "" {
		return payload, nil
	}
	var envelope struct {
		Data struct {
			Date  string                      `json:"date"`
			Items []model.AStockAuctionAmount `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Data.Date != "" || len(envelope.Data.Items) > 0) {
		payload.Date = envelope.Data.Date
		payload.Items = envelope.Data.Items
	}
	return payload, nil
}

func decodeAStockAuctionEndpointMessage(body []byte, fallback string) string {
	var envelope struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Data    struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		return nonEmpty(
			strings.TrimSpace(envelope.Message),
			strings.TrimSpace(envelope.Error),
			strings.TrimSpace(envelope.Data.Message),
			strings.TrimSpace(envelope.Data.Error),
		)
	}
	return strings.TrimSpace(fallback)
}

func isUsableAStockAuctionItem(item model.AStockAuctionAmount) bool {
	status := strings.TrimSpace(item.Status)
	if status != "" && !strings.EqualFold(status, "ok") {
		return false
	}
	return item.AuctionAmount > 0 || item.AuctionVolume > 0
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func aStockAuctionBackfillDates(days int, start string, end string, now time.Time) []string {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	location := now.Location()
	if days <= 0 {
		days = 30
	}
	if days > 120 {
		days = 120
	}
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if parsed, err := time.ParseInLocation("2006-01-02", end, location); err == nil {
		endDate = parsed
	}
	startDate := endDate.AddDate(0, 0, -days+1)
	if parsed, err := time.ParseInLocation("2006-01-02", start, location); err == nil {
		startDate = parsed
	}
	if startDate.After(endDate) {
		startDate, endDate = endDate, startDate
	}
	out := make([]string, 0)
	for cursor := startDate; !cursor.After(endDate); cursor = cursor.AddDate(0, 0, 1) {
		out = append(out, cursor.Format("2006-01-02"))
	}
	return out
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
	tradingDay, err := w.loadAStockTradingDayStatus(ctx, strategyDate)
	if err != nil {
		return fmt.Errorf("a-stock trading calendar unavailable for %s: %w", strategyDate, err)
	}
	if !tradingDay.IsTradingDay {
		message := strings.TrimSpace(tradingDay.Message)
		if message == "" {
			message = "A-share market is closed; stock recommendations are disabled."
		}
		return jobSkippedError{message: fmt.Sprintf("a-stock recommendation skipped for %s: %s", tradingDay.Date, message)}
	}
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

func (w *Worker) loadAStockTradingDayStatus(ctx context.Context, strategyDate string) (aStockTradingDayStatus, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return aStockTradingDayStatus{}, fmt.Errorf("YUQING_ASTOCK_AUCTION_URL not configured")
	}
	date := strings.TrimSpace(strategyDate)
	if date == "" {
		date = time.Now().In(aStockLocation()).Format("2006-01-02")
	}
	resp, err := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("date", date).
		Get(baseURL + "/api/a-stock/trading-day")
	if err != nil {
		return aStockTradingDayStatus{}, err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return aStockTradingDayStatus{}, fmt.Errorf("akshare trading-day endpoint failed: %s", message)
	}
	status, err := decodeAStockTradingDayStatus(resp.Body())
	if err != nil {
		return aStockTradingDayStatus{}, err
	}
	if strings.TrimSpace(status.Date) == "" {
		status.Date = date
	}
	return status, nil
}

func decodeAStockTradingDayStatus(body []byte) (aStockTradingDayStatus, error) {
	var status aStockTradingDayStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return status, err
	}
	if status.Date != "" || status.Source != "" || status.Reason != "" {
		return status, nil
	}
	var envelope struct {
		Data aStockTradingDayStatus `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return status, err
	}
	return envelope.Data, nil
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
		return time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 13, 0, 59, 0, location),
			"09:30-13:00", nil
	default:
		return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 59, 0, location),
			"08:00-09:30", nil
	}
}
