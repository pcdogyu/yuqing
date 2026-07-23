package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/astockcalendar"
	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/astocknews"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

var aStockRecommendationSources = astocknews.SourceTypes()

const (
	aStockIncrementalNewsLookback  = 30 * time.Minute
	aStockIncrementalNewsLookahead = 2 * time.Minute
)

type aStockRecommendationCrawlSummary struct {
	TotalSources  int
	SuccessCount  int
	FailedSources []string
}

type aStockRecommendationGenerateResult struct {
	StrategyDate             string `json:"strategy_date"`
	Period                   string `json:"period"`
	Phase                    string `json:"phase"`
	RecommendationCount      int    `json:"recommendation_count"`
	GeneratedCount           int    `json:"generated_count"`
	BacktestStatus           string `json:"backtest_status"`
	RecentFiltered           int    `json:"recent_filtered"`
	SameDayMorningFiltered   int    `json:"same_day_morning_filtered"`
	LimitUpFiltered          int    `json:"limit_up_filtered"`
	TodayMarketFilterEnabled bool   `json:"today_market_filter_enabled"`
	NoTodayMarketCount       int    `json:"no_today_market_count"`
	FundFlowFilterEnabled    bool   `json:"fund_flow_filter_enabled"`
	FundFlowFiltered         int    `json:"fund_flow_filtered"`
	FundFlowMissingCount     int    `json:"fund_flow_missing_count"`
	LoadMessage              string `json:"load_message"`
}

func (w *Worker) runAStockRecommendation(ctx context.Context, period string, phase string) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockRecommendationForDate(ctx, time.Now().In(location).Format("2006-01-02"), period, phase)
}

func (w *Worker) runAStockWindowNewsCrawl(ctx context.Context, period string, phase string) error {
	return w.runAStockWindowNewsCrawlForDate(ctx, time.Now().In(aStockLocation()).Format("2006-01-02"), period, phase)
}

func (w *Worker) runAStockIncrementalNewsCrawl(ctx context.Context, sourceType string) error {
	settings, _ := astocknews.LoadSettings("")
	if !astocknews.CrawlEnabled(settings, sourceType) {
		return jobSkippedError{message: fmt.Sprintf("a-stock news crawl skipped because %s is disabled", astocknews.Label(sourceType))}
	}
	return w.runCrawlWithOptions(ctx, sourceType, aStockIncrementalNewsCrawlOptions(time.Now()))
}

func aStockIncrementalNewsCrawlOptions(now time.Time) model.CrawlOptions {
	if now.IsZero() {
		now = time.Now()
	}
	current := now.In(aStockLocation())
	return model.CrawlOptions{
		Start:     formatAStockRecommendationCrawlTime(current.Add(-aStockIncrementalNewsLookback)),
		End:       formatAStockRecommendationCrawlTime(current.Add(aStockIncrementalNewsLookahead)),
		TimeField: "publish_time",
	}
}

type aStockBacktestRefreshTarget struct {
	StrategyDate string
	Period       string
}

type aStockBacktestRefreshSummary struct {
	Refreshed int
	Missing   int
	Failed    int
	Failures  []string
}

const aStockExactSnapshotBackfillMaxPerRun = 12

type aStockExactSnapshotBackfillTarget struct {
	StrategyDate   string
	Period         string
	Phase          string
	IgnoreFundFlow bool
}

type aStockExactSnapshotBackfillSummary struct {
	Generated int
	Existing  int
	Skipped   int
	Failed    int
	Failures  []string
}

func (w *Worker) runAStockAfternoonOpenRefresh(ctx context.Context) error {
	return w.runAStockAfternoonOpenRefreshForDate(ctx, time.Now().In(aStockLocation()).Format("2006-01-02"))
}

func (w *Worker) runAStockAfternoonOpenRefreshForDate(ctx context.Context, strategyDate string) error {
	return w.runAStockBacktestRefreshForDate(ctx, strategyDate, 0, []string{"afternoon"}, "a-stock afternoon open refresh")
}

func (w *Worker) runAStockDailyBacktestRefresh(ctx context.Context) error {
	return w.runAStockDailyBacktestRefreshForDate(ctx, time.Now().In(aStockLocation()).Format("2006-01-02"), 5)
}

func (w *Worker) runAStockDailyBacktestRefreshForDate(ctx context.Context, strategyDate string, previousTradingDays int) error {
	return w.runAStockBacktestRefreshForDate(ctx, strategyDate, previousTradingDays, []string{"morning", "afternoon", "evening"}, "a-stock daily backtest refresh")
}

func (w *Worker) runAStockBacktestRefreshForDate(ctx context.Context, strategyDate string, previousTradingDays int, periods []string, label string) error {
	unlock, lockErr := w.tryLockAStockRecommendationGeneration(label)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	strategyDate = normalizeAStockRecommendationDate(strategyDate)
	if strategyDate == "" {
		strategyDate = time.Now().In(aStockLocation()).Format("2006-01-02")
	}
	tradingDay, err := w.loadAStockTradingDayStatus(ctx, strategyDate)
	if err != nil {
		return fmt.Errorf("%s trading calendar unavailable for %s: %w", label, strategyDate, err)
	}
	if !tradingDay.IsTradingDay {
		message := strings.TrimSpace(tradingDay.Message)
		if message == "" {
			message = "A-share market is closed; stock recommendations are disabled."
		}
		return jobSkippedError{message: fmt.Sprintf("%s skipped for %s: %s", label, nonEmpty(tradingDay.Date, strategyDate), message)}
	}
	dates, err := w.aStockBacktestRefreshDates(ctx, tradingDay, previousTradingDays)
	if err != nil {
		return fmt.Errorf("%s target date build failed for %s: %w", label, nonEmpty(tradingDay.Date, strategyDate), err)
	}
	summary := aStockBacktestRefreshSummary{Failures: make([]string, 0)}
	for _, target := range aStockBacktestRefreshTargets(dates, periods) {
		exists, err := w.hasExistingAStockRecommendation(ctx, target.StrategyDate, target.Period)
		if err != nil {
			summary.Failed++
			summary.Failures = append(summary.Failures, fmt.Sprintf("%s: %v", aStockBacktestRefreshTargetLabel(target), err))
			continue
		}
		if !exists {
			summary.Missing++
			continue
		}
		if err := w.refreshAStockRecommendationBacktestSnapshot(ctx, target.StrategyDate, target.Period); err != nil {
			summary.Failed++
			summary.Failures = append(summary.Failures, fmt.Sprintf("%s: %v", aStockBacktestRefreshTargetLabel(target), err))
			continue
		}
		summary.Refreshed++
	}
	if summary.Refreshed == 0 && summary.Failed == 0 {
		return jobSkippedError{message: fmt.Sprintf("%s skipped for %s: no existing recommendation selections or snapshots", label, nonEmpty(tradingDay.Date, strategyDate))}
	}
	log.Info().
		Str("strategy_date", nonEmpty(tradingDay.Date, strategyDate)).
		Str("label", label).
		Int("refreshed", summary.Refreshed).
		Int("missing", summary.Missing).
		Int("failed", summary.Failed).
		Msg("a-stock backtest refresh finished")
	if summary.Failed > 0 {
		return fmt.Errorf("%s failed for %s: refreshed=%d missing=%d failed=%d: %s", label, nonEmpty(tradingDay.Date, strategyDate), summary.Refreshed, summary.Missing, summary.Failed, strings.Join(summary.Failures, "; "))
	}
	return nil
}

func (w *Worker) runAStockExactSnapshotBackfill(ctx context.Context) error {
	return w.runAStockExactSnapshotBackfillAt(ctx, time.Now().In(aStockLocation()), aStockExactSnapshotBackfillMaxPerRun)
}

func (w *Worker) runAStockExactSnapshotBackfillAt(ctx context.Context, now time.Time, maxGenerate int) error {
	unlock, lockErr := w.tryLockAStockRecommendationGeneration("a-stock exact snapshot backfill")
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	targets, err := w.aStockExactSnapshotBackfillTargets(ctx, now)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return jobSkippedError{message: "a-stock exact snapshot backfill skipped: no targets"}
	}
	if maxGenerate <= 0 {
		maxGenerate = aStockExactSnapshotBackfillMaxPerRun
	}
	summary := aStockExactSnapshotBackfillSummary{Failures: make([]string, 0)}
	for _, target := range targets {
		if summary.Generated >= maxGenerate {
			summary.Skipped++
			continue
		}
		exists, err := w.hasAStockExactRecommendationSnapshot(ctx, target)
		if err != nil {
			summary.Failed++
			summary.Failures = append(summary.Failures, fmt.Sprintf("%s/%s ignore_fund_flow=%t: %v", target.StrategyDate, target.Period, target.IgnoreFundFlow, err))
			continue
		}
		if exists {
			summary.Existing++
			continue
		}
		if err := w.generateAStockRecommendationSnapshotWithModeAndFundFlow(ctx, target.StrategyDate, target.Period, target.Phase, "", target.IgnoreFundFlow); err != nil {
			summary.Failed++
			summary.Failures = append(summary.Failures, fmt.Sprintf("%s/%s ignore_fund_flow=%t: %v", target.StrategyDate, target.Period, target.IgnoreFundFlow, err))
			continue
		}
		summary.Generated++
	}
	log.Info().
		Int("generated", summary.Generated).
		Int("existing", summary.Existing).
		Int("skipped", summary.Skipped).
		Int("failed", summary.Failed).
		Msg("a-stock exact snapshot backfill finished")
	if summary.Generated == 0 && summary.Existing == 0 && summary.Failed == 0 {
		return jobSkippedError{message: fmt.Sprintf("a-stock exact snapshot backfill skipped: generated=0 existing=0 skipped=%d", summary.Skipped)}
	}
	if summary.Failed > 0 {
		return fmt.Errorf("a-stock exact snapshot backfill failed: generated=%d existing=%d skipped=%d failed=%d: %s", summary.Generated, summary.Existing, summary.Skipped, summary.Failed, strings.Join(summary.Failures, "; "))
	}
	return nil
}

func (w *Worker) aStockExactSnapshotBackfillTargets(ctx context.Context, now time.Time) ([]aStockExactSnapshotBackfillTarget, error) {
	if now.IsZero() {
		now = time.Now()
	}
	current := now.In(aStockLocation())
	periods := []string{"morning", "afternoon"}
	phase := "final"
	startDate := current.Format("2006-01-02")
	previousTradingDays := 0
	switch current.Hour() {
	case 2:
		status, err := w.loadAStockTradingDayStatus(ctx, startDate)
		if err != nil {
			return nil, err
		}
		startDate = strings.TrimSpace(status.PreviousTradingDay)
		if startDate == "" && !status.IsTradingDay {
			startDate = strings.TrimSpace(status.LatestTradingDay)
		}
		if startDate == "" {
			return nil, jobSkippedError{message: "a-stock exact snapshot backfill skipped: previous trading day unavailable"}
		}
		previousTradingDays = 6
	case 9:
		periods = []string{"morning"}
		phase = "preopen"
	case 12:
		periods = []string{"afternoon"}
		phase = "preopen"
	case 15:
		previousTradingDays = 6
	default:
		previousTradingDays = 0
	}
	status, err := w.loadAStockTradingDayStatus(ctx, startDate)
	if err != nil {
		return nil, err
	}
	if !status.IsTradingDay {
		message := strings.TrimSpace(status.Message)
		if message == "" {
			message = "A-share market is closed; stock recommendations are disabled."
		}
		return nil, jobSkippedError{message: fmt.Sprintf("a-stock exact snapshot backfill skipped for %s: %s", nonEmpty(status.Date, startDate), message)}
	}
	dates, err := w.aStockBacktestRefreshDates(ctx, status, previousTradingDays)
	if err != nil {
		return nil, err
	}
	targets := make([]aStockExactSnapshotBackfillTarget, 0, len(dates)*len(periods)*2)
	for _, date := range dates {
		for _, period := range periods {
			for _, ignoreFundFlow := range []bool{false, true} {
				targets = append(targets, aStockExactSnapshotBackfillTarget{
					StrategyDate:   date,
					Period:         period,
					Phase:          phase,
					IgnoreFundFlow: ignoreFundFlow,
				})
			}
		}
	}
	return targets, nil
}

func (w *Worker) aStockBacktestRefreshDates(ctx context.Context, tradingDay aStockTradingDayStatus, previousTradingDays int) ([]string, error) {
	current := strings.TrimSpace(tradingDay.Date)
	if current == "" {
		return nil, fmt.Errorf("trading day date is empty")
	}
	dates := []string{current}
	if previousTradingDays <= 0 {
		return dates, nil
	}
	visited := map[string]struct{}{current: {}}
	previous := strings.TrimSpace(tradingDay.PreviousTradingDay)
	for len(dates) < previousTradingDays+1 && previous != "" {
		if _, ok := visited[previous]; ok {
			break
		}
		dates = append(dates, previous)
		visited[previous] = struct{}{}
		if len(dates) >= previousTradingDays+1 {
			break
		}
		status, err := w.loadAStockTradingDayStatus(ctx, previous)
		if err != nil {
			return nil, fmt.Errorf("load previous trading day %s failed: %w", previous, err)
		}
		previous = strings.TrimSpace(status.PreviousTradingDay)
	}
	return dates, nil
}

func aStockBacktestRefreshTargets(dates []string, periods []string) []aStockBacktestRefreshTarget {
	targets := make([]aStockBacktestRefreshTarget, 0, len(dates)*len(periods))
	for _, strategyDate := range dates {
		for _, period := range periods {
			if strings.TrimSpace(strategyDate) == "" || strings.TrimSpace(period) == "" {
				continue
			}
			targets = append(targets, aStockBacktestRefreshTarget{
				StrategyDate: strategyDate,
				Period:       normalizeAStockRecommendationPeriod(period),
			})
		}
	}
	return targets
}

func aStockBacktestRefreshTargetLabel(target aStockBacktestRefreshTarget) string {
	return target.StrategyDate + "/" + target.Period
}

func (w *Worker) hasExistingAStockRecommendation(ctx context.Context, strategyDate string, period string) (bool, error) {
	selections, err := w.loadAStockRecommendationSelections(ctx, strategyDate, period)
	if err != nil {
		return false, err
	}
	if selections.Found && len(selections.Items) > 0 {
		return true, nil
	}
	snapshot, err := w.loadAStockRecommendationSnapshot(ctx, strategyDate, period)
	if err != nil {
		return false, err
	}
	return snapshot.Found && aStockSnapshotHasRecommendations(snapshot), nil
}

func (w *Worker) loadAStockRecommendationSelections(ctx context.Context, strategyDate string, period string) (model.AStockRecommendationSelectionListResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return model.AStockRecommendationSelectionListResult{}, fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("date", normalizeAStockRecommendationDate(strategyDate)).
		SetQueryParam("period", normalizeAStockRecommendationPeriod(period)).
		Get(baseURL + "/api/v1/a-stock/recommendation-selections")
	if err != nil {
		return model.AStockRecommendationSelectionListResult{}, err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return model.AStockRecommendationSelectionListResult{}, fmt.Errorf("content recommendation selection request failed: %s", message)
	}
	var envelope struct {
		Data model.AStockRecommendationSelectionListResult `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return model.AStockRecommendationSelectionListResult{}, err
	}
	return envelope.Data, nil
}

func (w *Worker) loadAStockRecommendationSnapshot(ctx context.Context, strategyDate string, period string) (model.AStockRecommendationSnapshot, error) {
	return w.loadAStockRecommendationSnapshotWithExactFilters(ctx, strategyDate, period, false, false, false, false, false)
}

func (w *Worker) hasAStockExactRecommendationSnapshot(ctx context.Context, target aStockExactSnapshotBackfillTarget) (bool, error) {
	period := normalizeAStockRecommendationPeriod(target.Period)
	snapshot, err := w.loadAStockRecommendationSnapshotWithExactFilters(ctx, target.StrategyDate, period, true, false, period == "afternoon", false, !target.IgnoreFundFlow)
	if err != nil {
		return false, err
	}
	return snapshot.Found, nil
}

func (w *Worker) loadAStockRecommendationSnapshotWithExactFilters(ctx context.Context, strategyDate string, period string, exact bool, ignoreRecent bool, limitUpFilterEnabled bool, todayMarketFilterEnabled bool, fundFlowFilterEnabled bool) (model.AStockRecommendationSnapshot, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return model.AStockRecommendationSnapshot{}, fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	req := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("date", normalizeAStockRecommendationDate(strategyDate)).
		SetQueryParam("period", normalizeAStockRecommendationPeriod(period))
	if ignoreRecent {
		req.SetQueryParam("ignore_recent", "1")
	}
	if exact {
		req.SetQueryParam("limit_up_filter_enabled", boolQuery(limitUpFilterEnabled))
		req.SetQueryParam("today_market_filter_enabled", boolQuery(todayMarketFilterEnabled))
		req.SetQueryParam("fund_flow_filter_enabled", boolQuery(fundFlowFilterEnabled))
	}
	resp, err := req.Get(baseURL + "/api/v1/a-stock/recommendations")
	if err != nil {
		return model.AStockRecommendationSnapshot{}, err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return model.AStockRecommendationSnapshot{}, fmt.Errorf("content recommendation snapshot request failed: %s", message)
	}
	var envelope struct {
		Data model.AStockRecommendationSnapshot `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return model.AStockRecommendationSnapshot{}, err
	}
	return envelope.Data, nil
}

func boolQuery(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func aStockSnapshotHasRecommendations(snapshot model.AStockRecommendationSnapshot) bool {
	raw := strings.TrimSpace(snapshot.RecommendationsJSON)
	if raw == "" {
		return false
	}
	var payload []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	return len(payload) > 0
}

func (w *Worker) runAStockAuctionCrawl(ctx context.Context) error {
	return w.runAStockAuctionCrawlAt(ctx, time.Now().In(aStockLocation()))
}

func (w *Worker) runAStockAuctionCrawlAt(ctx context.Context, now time.Time) error {
	captureSlot := aStockAuctionCaptureSlotForTime(now)
	status, err := w.loadAStockTradingDayStatus(ctx, "")
	if err != nil {
		return fmt.Errorf("a-stock auction crawl trading-day check failed: %w", err)
	}
	if !status.IsTradingDay {
		message := strings.TrimSpace(status.Message)
		if message == "" {
			message = "A-share market is closed; auction crawl is disabled."
		}
		return jobSkippedError{message: fmt.Sprintf("a-stock auction crawl skipped for %s: %s", nonEmpty(status.Date, time.Now().In(aStockLocation()).Format("2006-01-02")), message)}
	}
	result, err := w.runAStockAuctionCrawlForDateResult(ctx, "", captureSlot)
	if err != nil {
		return err
	}
	if result.Skipped {
		return fmt.Errorf("a-stock auction crawl skipped for %s: %s", result.Date, result.Message)
	}
	log.Info().
		Str("trade_date", result.Date).
		Str("capture_slot", result.CaptureSlot).
		Int("total", result.Total).
		Int("ok", result.OK).
		Msg("a-stock auction amounts crawled")
	return nil
}

func (w *Worker) runAStockAuctionLatest(ctx context.Context) (aStockAuctionCrawlResult, error) {
	return w.runAStockAuctionCrawlForDateResult(ctx, "", "0929")
}

func (w *Worker) runAStockAuctionCrawlForDate(ctx context.Context, tradeDate string) error {
	result, err := w.runAStockAuctionCrawlForDateResult(ctx, tradeDate, "0929")
	if err != nil {
		return err
	}
	if result.Skipped {
		return fmt.Errorf("a-stock auction crawl skipped for %s: %s", result.Date, result.Message)
	}
	log.Info().
		Str("trade_date", result.Date).
		Str("capture_slot", result.CaptureSlot).
		Int("total", result.Total).
		Int("ok", result.OK).
		Msg("a-stock auction amounts crawled")
	return nil
}

type aStockAuctionCrawlResult struct {
	Date        string `json:"date"`
	CaptureSlot string `json:"capture_slot"`
	Total       int    `json:"total"`
	OK          int    `json:"ok"`
	Skipped     bool   `json:"skipped,omitempty"`
	Message     string `json:"message,omitempty"`
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

func (w *Worker) tryLockAStockRecommendationGeneration(label string) (func(), error) {
	if !w.aStockMu.TryLock() {
		return nil, jobSkippedError{message: fmt.Sprintf("%s skipped: another a-stock recommendation task is running", strings.TrimSpace(label))}
	}
	return w.aStockMu.Unlock, nil
}

type aStockAuctionBackfillResult struct {
	Days      int                        `json:"days"`
	Succeeded int                        `json:"succeeded"`
	Skipped   int                        `json:"skipped"`
	Failed    int                        `json:"failed"`
	Results   []aStockAuctionCrawlResult `json:"results"`
	Errors    []string                   `json:"errors"`
}

type aStockSectorFundFlowCrawlResult struct {
	Date           string   `json:"date"`
	Groups         int      `json:"groups"`
	Items          int      `json:"items"`
	SectorGroups   int      `json:"sector_groups"`
	SectorItems    int      `json:"sector_items"`
	StockGroups    int      `json:"stock_groups"`
	StockItems     int      `json:"stock_items"`
	SnapshotGroups int      `json:"snapshot_groups"`
	SnapshotItems  int      `json:"snapshot_items"`
	SourceErrors   []string `json:"source_errors,omitempty"`
	Skipped        bool     `json:"skipped,omitempty"`
	Message        string   `json:"message,omitempty"`
	SectorType     string   `json:"sector_type,omitempty"`
	Indicator      string   `json:"indicator,omitempty"`
}

func aStockAuctionCaptureSlotForTime(value time.Time) string {
	local := value.In(aStockLocation())
	if local.Hour() == 9 && local.Minute() == 20 {
		return "0920"
	}
	if local.Hour() == 9 && local.Minute() == 25 {
		return "0925"
	}
	if local.Hour() == 9 && local.Minute() == 29 {
		return "0929"
	}
	return "0929"
}

func normalizeAStockAuctionCaptureSlot(value string) string {
	switch strings.TrimSpace(value) {
	case "0920":
		return "0920"
	case "0925":
		return "0925"
	case "0929":
		return "0929"
	case "0930":
		return "0930"
	default:
		return "0929"
	}
}

func (w *Worker) runAStockAuctionCrawlForDateResult(ctx context.Context, tradeDate string, captureSlot string) (aStockAuctionCrawlResult, error) {
	captureSlot = normalizeAStockAuctionCaptureSlot(captureSlot)
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
			return aStockAuctionCrawlResult{Date: tradeDate, CaptureSlot: captureSlot, Skipped: true, Message: message}, nil
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
		if payload.Items[i].CaptureSlot == "" {
			payload.Items[i].CaptureSlot = captureSlot
		}
		if payload.Items[i].FetchedAt.IsZero() {
			payload.Items[i].FetchedAt = time.Now().UTC()
		}
	}
	repairAStockAuctionPayloadItemNames(payload.Items, payload.CodeNames)
	payload.Items = filterAStockAuctionPayloadItems(payload.Items)
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
		return aStockAuctionCrawlResult{Date: payload.Date, CaptureSlot: captureSlot, Total: len(payload.Items), OK: okCount, Skipped: true, Message: message}, nil
	}
	writePayload := map[string]any{
		"date":         payload.Date,
		"capture_slot": captureSlot,
		"items":        payload.Items,
		"code_names":   payload.CodeNames,
		"replace":      true,
	}
	writeResp, err := w.client.R().
		SetContext(ctx).
		SetBody(writePayload).
		Post(w.cfg.ContentURL + "/api/v1/admin/a-stock/auction")
	if err != nil {
		return aStockAuctionCrawlResult{}, err
	}
	if !writeResp.IsSuccess() {
		return aStockAuctionCrawlResult{}, fmt.Errorf("content auction upsert failed: %s", writeResp.Status())
	}
	return aStockAuctionCrawlResult{Date: payload.Date, CaptureSlot: captureSlot, Total: len(payload.Items), OK: okCount}, nil
}

func (w *Worker) runAStockAuctionBackfill(ctx context.Context, days int, start string, end string) (aStockAuctionBackfillResult, error) {
	dates := aStockAuctionBackfillDates(days, start, end, time.Now().In(aStockLocation()))
	result := aStockAuctionBackfillResult{Days: len(dates), Results: make([]aStockAuctionCrawlResult, 0, len(dates))}
	for _, date := range dates {
		item, err := w.runAStockAuctionCrawlForDateResult(ctx, date, "0929")
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

func (w *Worker) runAStockSectorFundFlowCrawl(ctx context.Context) error {
	result, err := w.runAStockSectorFundFlowLatest(ctx, true)
	if err != nil {
		return err
	}
	if result.Skipped {
		return jobSkippedError{message: result.Message}
	}
	log.Info().
		Str("trade_date", result.Date).
		Int("groups", result.Groups).
		Int("items", result.Items).
		Msg("a-stock sector fund flow crawled")
	return nil
}

func (w *Worker) runAStockSectorFundFlowIntradayCrawl(ctx context.Context) error {
	result, err := w.runAStockSectorFundFlowIntradayLatest(ctx, true)
	if err != nil {
		return err
	}
	if result.Skipped {
		return jobSkippedError{message: result.Message}
	}
	log.Info().
		Str("trade_date", result.Date).
		Int("groups", result.Groups).
		Int("items", result.Items).
		Int("snapshots", result.SnapshotGroups).
		Int("snapshot_items", result.SnapshotItems).
		Msg("a-stock sector fund flow intraday crawled")
	return nil
}

func (w *Worker) runAStockSectorFundFlowLatest(ctx context.Context, requireTradingSession bool) (aStockSectorFundFlowCrawlResult, error) {
	now := time.Now().In(aStockLocation())
	strategyDate := now.Format("2006-01-02")
	tradingDay, err := w.loadAStockTradingDayStatus(ctx, strategyDate)
	if err != nil {
		return aStockSectorFundFlowCrawlResult{Date: strategyDate}, fmt.Errorf("a-stock sector fund flow trading calendar unavailable for %s: %w", strategyDate, err)
	}
	tradeDate := nonEmpty(strings.TrimSpace(tradingDay.Date), strategyDate)
	if !tradingDay.IsTradingDay {
		message := strings.TrimSpace(tradingDay.Message)
		if message == "" {
			message = "A-share market is closed; sector fund flow crawl is disabled."
		}
		return aStockSectorFundFlowCrawlResult{Date: tradeDate, Skipped: true, Message: fmt.Sprintf("a-stock sector fund flow skipped for %s: %s", tradeDate, message)}, nil
	}
	if requireTradingSession && !isAStockSectorFundFlowTradingSession(now) {
		return aStockSectorFundFlowCrawlResult{Date: tradeDate, Skipped: true, Message: fmt.Sprintf("a-stock sector fund flow skipped for %s: outside trading session", tradeDate)}, nil
	}
	sectorTypes := []string{"行业资金流", "概念资金流"}
	indicators := []string{"今日", "5日", "10日"}
	sectorSources := []string{"eastmoney", "ths", "sina"}
	stockSources := []string{"eastmoney", "ths", "sina"}
	result := aStockSectorFundFlowCrawlResult{Date: tradeDate}
	for _, sectorType := range sectorTypes {
		for _, indicator := range indicators {
			for _, source := range sectorSources {
				items, err := w.fetchExternalAStockSectorFundFlowSource(ctx, tradeDate, sectorType, indicator, source)
				if err != nil {
					result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("sector %s/%s/%s: %v", sectorType, indicator, source, err))
					continue
				}
				if err := w.writeAStockSectorFundFlowSource(ctx, tradeDate, sectorType, indicator, source, items); err != nil {
					result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("sector %s/%s/%s write: %v", sectorType, indicator, source, err))
					continue
				}
				result.Groups++
				result.Items += len(items)
				result.SectorGroups++
				result.SectorItems += len(items)
			}
		}
	}
	for _, indicator := range indicators {
		for _, source := range stockSources {
			if indicator != "今日" && source == "sina" {
				continue
			}
			items, err := w.fetchExternalAStockStockFundFlowSource(ctx, tradeDate, indicator, source)
			if err != nil {
				result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("stock %s/%s: %v", indicator, source, err))
				continue
			}
			if err := w.writeAStockStockFundFlowSource(ctx, tradeDate, indicator, source, items); err != nil {
				result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("stock %s/%s write: %v", indicator, source, err))
				continue
			}
			result.Groups++
			result.Items += len(items)
			result.StockGroups++
			result.StockItems += len(items)
		}
	}
	if result.Groups == 0 && len(result.SourceErrors) > 0 {
		return result, fmt.Errorf("a-stock fund flow crawl failed for all sources: %s", strings.Join(result.SourceErrors, "; "))
	}
	return result, nil
}

func (w *Worker) runAStockSectorFundFlowIntradayLatest(ctx context.Context, requireTradingSession bool) (aStockSectorFundFlowCrawlResult, error) {
	now := time.Now().In(aStockLocation())
	strategyDate := now.Format("2006-01-02")
	tradingDay, err := w.loadAStockTradingDayStatus(ctx, strategyDate)
	if err != nil {
		return aStockSectorFundFlowCrawlResult{Date: strategyDate}, fmt.Errorf("a-stock sector fund flow intraday trading calendar unavailable for %s: %w", strategyDate, err)
	}
	tradeDate := nonEmpty(strings.TrimSpace(tradingDay.Date), strategyDate)
	if !tradingDay.IsTradingDay {
		message := strings.TrimSpace(tradingDay.Message)
		if message == "" {
			message = "A-share market is closed; sector fund flow intraday crawl is disabled."
		}
		return aStockSectorFundFlowCrawlResult{Date: tradeDate, Skipped: true, Message: fmt.Sprintf("a-stock sector fund flow intraday skipped for %s: %s", tradeDate, message)}, nil
	}
	if requireTradingSession && !isAStockSectorFundFlowTradingSession(now) {
		return aStockSectorFundFlowCrawlResult{Date: tradeDate, Skipped: true, Message: fmt.Sprintf("a-stock sector fund flow intraday skipped for %s: outside trading session", tradeDate)}, nil
	}

	sectorTypes := []string{"行业资金流", "概念资金流"}
	sectorSources := []string{"eastmoney", "ths", "sina"}
	const indicator = "今日"
	captureTime := now.Format("15:04")
	result := aStockSectorFundFlowCrawlResult{Date: tradeDate, Indicator: indicator}
	for _, sectorType := range sectorTypes {
		for _, source := range sectorSources {
			items, err := w.fetchExternalAStockSectorFundFlowSource(ctx, tradeDate, sectorType, indicator, source)
			if err != nil {
				result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("sector %s/%s/%s: %v", sectorType, indicator, source, err))
				continue
			}
			if err := w.writeAStockSectorFundFlowSource(ctx, tradeDate, sectorType, indicator, source, items); err != nil {
				result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("sector %s/%s/%s write: %v", sectorType, indicator, source, err))
				continue
			}
			result.Groups++
			result.Items += len(items)
			result.SectorGroups++
			result.SectorItems += len(items)
		}
		snapshot, err := w.writeAStockSectorFundFlowIntradaySnapshot(ctx, tradeDate, captureTime, sectorType, indicator)
		if err != nil {
			result.SourceErrors = append(result.SourceErrors, fmt.Sprintf("sector %s/%s snapshot: %v", sectorType, indicator, err))
			continue
		}
		if snapshot.Total > 0 {
			result.SnapshotGroups++
			result.SnapshotItems += snapshot.Total
		}
	}
	if result.Groups == 0 && len(result.SourceErrors) > 0 {
		return result, fmt.Errorf("a-stock sector fund flow intraday crawl failed for all sources: %s", strings.Join(result.SourceErrors, "; "))
	}
	return result, nil
}

func isAStockSectorFundFlowTradingSession(now time.Time) bool {
	local := now.In(aStockLocation())
	weekday := local.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	minutes := local.Hour()*60 + local.Minute()
	return (minutes >= 9*60+30 && minutes <= 11*60+30) || (minutes >= 13*60 && minutes <= 15*60)
}

func (w *Worker) fetchExternalAStockSectorFundFlowSource(ctx context.Context, tradeDate string, sectorType string, indicator string, source string) ([]model.AStockSectorFundFlow, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("YUQING_ASTOCK_AUCTION_URL not configured")
	}
	resp, err := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("date", tradeDate).
		SetQueryParam("sector_type", sectorType).
		SetQueryParam("indicator", indicator).
		SetQueryParam("source", source).
		Get(baseURL + "/api/a-stock/sector-fund-flow")
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return nil, fmt.Errorf("akshare sector fund flow endpoint failed for %s/%s/%s: %s", sectorType, indicator, source, message)
	}
	payload, err := decodeAStockSectorFundFlowPayload(resp.Body())
	if err != nil {
		return nil, err
	}
	for i := range payload.Items {
		if payload.Items[i].TradeDate == "" {
			payload.Items[i].TradeDate = tradeDate
		}
		if payload.Items[i].SectorType == "" {
			payload.Items[i].SectorType = sectorType
		}
		if payload.Items[i].Indicator == "" {
			payload.Items[i].Indicator = indicator
		}
		if payload.Items[i].SourceType == "" {
			payload.Items[i].SourceType = source
		}
		if payload.Items[i].FetchedAt.IsZero() {
			payload.Items[i].FetchedAt = time.Now().UTC()
		}
	}
	return payload.Items, nil
}

func (w *Worker) writeAStockSectorFundFlowSource(ctx context.Context, tradeDate string, sectorType string, indicator string, source string, items []model.AStockSectorFundFlow) error {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	payload := map[string]any{
		"date":        tradeDate,
		"sector_type": sectorType,
		"indicator":   indicator,
		"source_type": source,
		"items":       items,
		"replace":     true,
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(baseURL + "/api/v1/internal/a-stock/sector-fund-flow-sources")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return fmt.Errorf("content sector fund flow source upsert failed for %s/%s/%s: %s", sectorType, indicator, source, message)
	}
	return nil
}

func (w *Worker) writeAStockSectorFundFlowIntradaySnapshot(ctx context.Context, tradeDate string, captureTime string, sectorType string, indicator string) (model.AStockSectorFundFlowUpsertResult, error) {
	result := model.AStockSectorFundFlowUpsertResult{Date: tradeDate}
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return result, fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	payload := map[string]any{
		"date":         tradeDate,
		"capture_time": captureTime,
		"sector_type":  sectorType,
		"indicator":    indicator,
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		SetResult(&struct {
			Data *model.AStockSectorFundFlowUpsertResult `json:"data"`
		}{}).
		Post(baseURL + "/api/v1/internal/a-stock/sector-fund-flow-intraday/snapshot")
	if err != nil {
		return result, err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return result, fmt.Errorf("content sector fund flow intraday snapshot failed for %s/%s: %s", sectorType, indicator, message)
	}
	var envelope struct {
		Data model.AStockSectorFundFlowUpsertResult `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return result, err
	}
	if envelope.Data.Date == "" {
		envelope.Data.Date = tradeDate
	}
	return envelope.Data, nil
}

func (w *Worker) fetchExternalAStockStockFundFlowSource(ctx context.Context, tradeDate string, indicator string, source string) ([]model.AStockStockFundFlow, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("YUQING_ASTOCK_AUCTION_URL not configured")
	}
	resp, err := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("date", tradeDate).
		SetQueryParam("indicator", indicator).
		SetQueryParam("source", source).
		Get(baseURL + "/api/a-stock/stock-fund-flow")
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return nil, fmt.Errorf("akshare stock fund flow endpoint failed for %s/%s: %s", indicator, source, message)
	}
	payload, err := decodeAStockStockFundFlowPayload(resp.Body())
	if err != nil {
		return nil, err
	}
	for i := range payload.Items {
		if payload.Items[i].TradeDate == "" {
			payload.Items[i].TradeDate = tradeDate
		}
		if payload.Items[i].Indicator == "" {
			payload.Items[i].Indicator = indicator
		}
		if payload.Items[i].SourceType == "" {
			payload.Items[i].SourceType = source
		}
		if payload.Items[i].FetchedAt.IsZero() {
			payload.Items[i].FetchedAt = time.Now().UTC()
		}
	}
	return payload.Items, nil
}

func (w *Worker) writeAStockStockFundFlowSource(ctx context.Context, tradeDate string, indicator string, source string, items []model.AStockStockFundFlow) error {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	payload := map[string]any{
		"date":        tradeDate,
		"indicator":   indicator,
		"source_type": source,
		"items":       items,
		"replace":     true,
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(baseURL + "/api/v1/internal/a-stock/stock-fund-flow-sources")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return fmt.Errorf("content stock fund flow source upsert failed for %s/%s: %s", indicator, source, message)
	}
	return nil
}

type aStockSectorFundFlowPayload struct {
	Items []model.AStockSectorFundFlow `json:"items"`
}

type aStockStockFundFlowPayload struct {
	Items []model.AStockStockFundFlow `json:"items"`
}

func decodeAStockSectorFundFlowPayload(body []byte) (aStockSectorFundFlowPayload, error) {
	var payload aStockSectorFundFlowPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		var items []model.AStockSectorFundFlow
		if arrayErr := json.Unmarshal(body, &items); arrayErr == nil {
			payload.Items = items
			return payload, nil
		}
		return payload, err
	}
	if len(payload.Items) > 0 {
		return payload, nil
	}
	var envelope struct {
		Data struct {
			Items []model.AStockSectorFundFlow `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Data.Items) > 0 {
		payload.Items = envelope.Data.Items
	}
	return payload, nil
}

func decodeAStockStockFundFlowPayload(body []byte) (aStockStockFundFlowPayload, error) {
	var payload aStockStockFundFlowPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		var items []model.AStockStockFundFlow
		if arrayErr := json.Unmarshal(body, &items); arrayErr == nil {
			payload.Items = items
			return payload, nil
		}
		return payload, err
	}
	if len(payload.Items) > 0 {
		return payload, nil
	}
	var envelope struct {
		Data struct {
			Items []model.AStockStockFundFlow `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Data.Items) > 0 {
		payload.Items = envelope.Data.Items
	}
	return payload, nil
}

type aStockAuctionPayload struct {
	Date      string                      `json:"date"`
	Items     []model.AStockAuctionAmount `json:"items"`
	CodeNames []model.AStockCodeName      `json:"code_names"`
	Message   string                      `json:"message"`
	Warning   string                      `json:"warning"`
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
			Date      string                      `json:"date"`
			Items     []model.AStockAuctionAmount `json:"items"`
			CodeNames []model.AStockCodeName      `json:"code_names"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Data.Date != "" || len(envelope.Data.Items) > 0) {
		payload.Date = envelope.Data.Date
		payload.Items = envelope.Data.Items
		payload.CodeNames = envelope.Data.CodeNames
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
	if !astockcode.IsShanghaiShenzhen(item.Code) || !astockcode.HasResolvedName(item.Code, item.Name) {
		return false
	}
	status := strings.TrimSpace(item.Status)
	if status != "" && !strings.EqualFold(status, "ok") {
		return false
	}
	return item.AuctionAmount > 0 || item.AuctionVolume > 0
}

func repairAStockAuctionPayloadItemNames(items []model.AStockAuctionAmount, codeNames []model.AStockCodeName) {
	if len(items) == 0 || len(codeNames) == 0 {
		return
	}
	names := make(map[string]string, len(codeNames))
	for _, item := range codeNames {
		code := astockcode.Normalize(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if astockcode.IsShanghaiShenzhen(code) && astockcode.HasResolvedName(code, name) {
			names[code] = name
		}
	}
	for i := range items {
		code := astockcode.Normalize(items[i].Code)
		if name := names[code]; name != "" {
			items[i].Name = name
		}
	}
}

func filterAStockAuctionPayloadItems(items []model.AStockAuctionAmount) []model.AStockAuctionAmount {
	if len(items) == 0 {
		return items
	}
	filtered := make([]model.AStockAuctionAmount, 0, len(items))
	for _, item := range items {
		code := astockcode.Normalize(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !astockcode.HasResolvedName(code, name) {
			continue
		}
		item.Code = code
		item.Name = name
		filtered = append(filtered, item)
	}
	return filtered
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

type aStockHoldingFetchResult struct {
	Items   []model.StockInstitutionHolding
	Reports []model.StockHoldingReportDocument
}

func (w *Worker) runAStockHoldingsCrawl(ctx context.Context) error {
	return w.runAStockHoldingsBackfill(ctx, aStockHoldingCrawlOptions{})
}

func (w *Worker) runAStockHoldingsBackfill(ctx context.Context, opts aStockHoldingCrawlOptions) error {
	periods := aStockHoldingPeriods(opts)
	if len(periods) == 0 {
		periods = latestAStockHoldingPeriods(4, time.Now().In(aStockLocation()))
	}
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type periodResult struct {
		Period  string
		Items   int
		Reports int
		Err     error
	}
	jobs := make(chan string)
	results := make(chan periodResult, len(periods))
	workerCount := aStockHoldingsPeriodConcurrency(len(periods))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-taskCtx.Done():
					return
				case period, ok := <-jobs:
					if !ok {
						return
					}
					fetched, err := w.fetchExternalAStockHoldings(taskCtx, period, opts.Code)
					if err == nil {
						err = w.upsertAStockHoldings(taskCtx, fetched.Items, fetched.Reports)
					}
					results <- periodResult{Period: period, Items: len(fetched.Items), Reports: len(fetched.Reports), Err: err}
					if err != nil {
						cancel()
						return
					}
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, period := range periods {
			select {
			case <-taskCtx.Done():
				return
			case jobs <- period:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	totalItems := 0
	totalReports := 0
	var firstErr error
	for result := range results {
		if result.Err != nil && firstErr == nil {
			firstErr = result.Err
		}
		if result.Err != nil {
			continue
		}
		totalItems += result.Items
		totalReports += result.Reports
		log.Info().
			Str("code", opts.Code).
			Str("period", result.Period).
			Int("items", result.Items).
			Int("reports", result.Reports).
			Msg("a-stock institution holdings period crawled")
	}
	if firstErr != nil {
		return firstErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Info().
		Str("code", opts.Code).
		Str("period", opts.Period).
		Str("start_period", opts.StartPeriod).
		Str("end_period", opts.EndPeriod).
		Int("items", totalItems).
		Int("reports", totalReports).
		Int("periods", len(periods)).
		Msg("a-stock institution holdings crawled")
	return nil
}

func (w *Worker) upsertAStockHoldings(ctx context.Context, items []model.StockInstitutionHolding, reports []model.StockHoldingReportDocument) error {
	payload := map[string]any{"items": items}
	if len(reports) > 0 {
		payload["reports"] = reports
	}
	resp, err := w.holdingClient.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/internal/a-stock/holdings/batch")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock holdings upsert failed: %s", resp.Status())
	}
	return nil
}

func (w *Worker) fetchExternalAStockHoldings(ctx context.Context, period string, code string) (aStockHoldingFetchResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockHoldingURL), "/")
	if baseURL == "" {
		return aStockHoldingFetchResult{}, fmt.Errorf("YUQING_ASTOCK_HOLDING_URL not configured")
	}
	req := w.holdingClient.R().
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
		return aStockHoldingFetchResult{}, err
	}
	if !resp.IsSuccess() {
		return aStockHoldingFetchResult{}, fmt.Errorf("a-stock holdings endpoint failed: %s", resp.Status())
	}
	var envelope struct {
		Items   []model.StockInstitutionHolding    `json:"items"`
		Reports []model.StockHoldingReportDocument `json:"reports"`
		Data    struct {
			Items   []model.StockInstitutionHolding    `json:"items"`
			Reports []model.StockHoldingReportDocument `json:"reports"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return aStockHoldingFetchResult{}, err
	}
	if len(envelope.Items) == 0 && len(envelope.Data.Items) > 0 {
		envelope.Items = envelope.Data.Items
	}
	if len(envelope.Reports) == 0 && len(envelope.Data.Reports) > 0 {
		envelope.Reports = envelope.Data.Reports
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
	for i := range envelope.Reports {
		if envelope.Reports[i].ReportPeriod == "" {
			envelope.Reports[i].ReportPeriod = period
		}
		if envelope.Reports[i].SourceType == "" {
			envelope.Reports[i].SourceType = "fund_quarterly_report"
		}
		if envelope.Reports[i].FetchedAt.IsZero() {
			envelope.Reports[i].FetchedAt = now
		}
	}
	return aStockHoldingFetchResult{Items: envelope.Items, Reports: envelope.Reports}, nil
}

func aStockHoldingsBackfillTimeout(base time.Duration, opts aStockHoldingCrawlOptions) time.Duration {
	periods := aStockHoldingPeriods(opts)
	periodCount := len(periods)
	if periodCount == 0 {
		periodCount = 4
	}
	return maxDuration(aStockHoldingsHTTPTimeout(base)*time.Duration(periodCount+1), 2*time.Hour)
}

func aStockHoldingsPeriodConcurrency(periodCount int) int {
	if periodCount <= 1 {
		return 1
	}
	return 2
}

func aStockHoldingsHTTPTimeout(base time.Duration) time.Duration {
	if base <= 0 {
		return 30 * time.Minute
	}
	return maxDuration(base*6, 30*time.Minute)
}

func (w *Worker) runAStockRecommendationForDate(ctx context.Context, strategyDate string, period string, phase string) error {
	unlock, lockErr := w.tryLockAStockRecommendationGeneration("a-stock recommendation")
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	normalizedPhase := normalizeAStockRecommendationPhase(phase)
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
	start, end, label, err := aStockRecommendationWindow(strategyDate, period, normalizedPhase)
	if err != nil {
		return err
	}
	crawlSummary := aStockRecommendationCrawlSummary{}
	if normalizeAStockRecommendationPeriod(period) != "evening" {
		crawlSummary, err = w.crawlAStockRecommendationSources(ctx, strategyDate, period, normalizedPhase, start, end, "a-stock recommendation")
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
	}
	refreshMode := ""
	if normalizeAStockRecommendationPeriod(period) == "morning" && normalizedPhase == "final" {
		refreshMode = "preserve_locked"
	}
	generationStarted := time.Now().UTC()
	generateResult, err := w.generateAStockRecommendationSnapshotWithModeResult(ctx, strategyDate, period, normalizedPhase, refreshMode)
	generationFinished := time.Now().UTC()
	if err != nil {
		return err
	}
	w.recordAStockRecommendationGenerateResult(ctx, strategyDate, period, normalizedPhase, label, crawlSummary, generateResult, generationStarted, generationFinished)
	if normalizedPhase == "preopen" {
		w.recordAStockPopupReadyResult(ctx, strategyDate, period, normalizedPhase, generateResult, generationFinished)
	}
	log.Info().
		Str("strategy_date", strategyDate).
		Str("period", period).
		Str("phase", normalizedPhase).
		Str("window", label).
		Msg("a-stock recommendation window generated")
	return nil
}

func (w *Worker) runAStockWindowNewsCrawlForDate(ctx context.Context, strategyDate string, period string, phase string) error {
	normalizedPhase := normalizeAStockRecommendationPhase(phase)
	tradingDay, err := w.loadAStockTradingDayStatus(ctx, strategyDate)
	if err != nil {
		return fmt.Errorf("a-stock trading calendar unavailable for %s: %w", strategyDate, err)
	}
	if !tradingDay.IsTradingDay {
		message := strings.TrimSpace(tradingDay.Message)
		if message == "" {
			message = "A-share market is closed; stock news crawl is disabled."
		}
		return jobSkippedError{message: fmt.Sprintf("a-stock window news crawl skipped for %s: %s", tradingDay.Date, message)}
	}
	start, end, label, err := aStockRecommendationWindow(strategyDate, period, normalizedPhase)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	crawlSummary, err := w.crawlAStockRecommendationSources(ctx, strategyDate, period, normalizedPhase, start, end, "a-stock window news crawl")
	finishedAt := time.Now().UTC()
	w.recordAStockWindowNewsResult(ctx, strategyDate, period, normalizedPhase, label, crawlSummary, err, startedAt, finishedAt)
	if err != nil {
		return err
	}
	log.Info().
		Str("strategy_date", strategyDate).
		Str("period", period).
		Str("phase", normalizedPhase).
		Str("window", label).
		Msg("a-stock window news crawled")
	return nil
}

func (w *Worker) crawlAStockRecommendationSources(ctx context.Context, strategyDate string, period string, phase string, start time.Time, end time.Time, label string) (aStockRecommendationCrawlSummary, error) {
	crawlOptions := model.CrawlOptions{
		Start:     formatAStockRecommendationCrawlTime(start),
		End:       formatAStockRecommendationCrawlTime(end),
		TimeField: "publish_time",
	}
	sources := w.aStockRecommendationCrawlSources()
	summary := aStockRecommendationCrawlSummary{
		TotalSources:  len(sources),
		FailedSources: make([]string, 0),
	}
	for _, sourceType := range sources {
		if err := w.runCrawlWithOptions(ctx, sourceType, crawlOptions); err != nil {
			summary.FailedSources = append(summary.FailedSources, sourceType+": "+err.Error())
			log.Warn().
				Err(err).
				Str("source_type", sourceType).
				Str("strategy_date", strategyDate).
				Str("period", period).
				Str("phase", phase).
				Str("job", label).
				Msg("a-stock crawl source failed")
			continue
		}
		summary.SuccessCount++
	}
	if summary.SuccessCount == 0 && len(summary.FailedSources) > 0 {
		return summary, fmt.Errorf("%s failed for all sources: %s", label, strings.Join(summary.FailedSources, "; "))
	}
	return summary, nil
}

func formatAStockRecommendationCrawlTime(value time.Time) string {
	return value.In(aStockLocation()).Format("2006-01-02 15:04:05")
}

func (w *Worker) recordAStockWindowNewsResult(ctx context.Context, strategyDate string, period string, phase string, windowLabel string, summary aStockRecommendationCrawlSummary, runErr error, startedAt time.Time, finishedAt time.Time) {
	status := "success"
	if runErr != nil {
		status = "failed"
	}
	message := fmt.Sprintf("date=%s window=%s sources=%d success=%d failed=%d",
		normalizeAStockRecommendationDate(strategyDate),
		windowLabel,
		summary.TotalSources,
		summary.SuccessCount,
		len(summary.FailedSources),
	)
	if len(summary.FailedSources) > 0 {
		message += " failures=" + strings.Join(summary.FailedSources, "; ")
	}
	if runErr != nil && len(summary.FailedSources) == 0 {
		message += " error=" + runErr.Error()
	}
	finished := finishedAt
	taskName := fmt.Sprintf("a-stock-window-news:%s:%s", normalizeAStockRecommendationPeriod(period), normalizeAStockRecommendationPhase(phase))
	if err := w.recordTaskRun(ctx, taskName, status, message, startedAt, &finished); err != nil {
		log.Warn().Err(err).Str("task", taskName).Msg("record a-stock window news task run failed")
	}
}

func (w *Worker) recordAStockRecommendationGenerateResult(ctx context.Context, strategyDate string, period string, phase string, windowLabel string, crawlSummary aStockRecommendationCrawlSummary, result aStockRecommendationGenerateResult, startedAt time.Time, finishedAt time.Time) {
	message := fmt.Sprintf("date=%s window=%s recommendations=%d generated=%d recent_filtered=%d same_day_morning_filtered=%d limit_up_filtered=%d no_today_market=%d fund_flow_filtered=%d fund_flow_missing=%d backtest=%s news_sources=%d/%d",
		nonEmpty(result.StrategyDate, normalizeAStockRecommendationDate(strategyDate)),
		windowLabel,
		result.RecommendationCount,
		result.GeneratedCount,
		result.RecentFiltered,
		result.SameDayMorningFiltered,
		result.LimitUpFiltered,
		result.NoTodayMarketCount,
		result.FundFlowFiltered,
		result.FundFlowMissingCount,
		nonEmpty(result.BacktestStatus, "unknown"),
		crawlSummary.SuccessCount,
		crawlSummary.TotalSources,
	)
	if loadMessage := strings.TrimSpace(result.LoadMessage); loadMessage != "" {
		message += " load_message=" + loadMessage
	}
	finished := finishedAt
	taskName := fmt.Sprintf("a-stock-recommendation-generate:%s:%s", normalizeAStockRecommendationPeriod(period), normalizeAStockRecommendationPhase(phase))
	if err := w.recordTaskRun(ctx, taskName, "success", message, startedAt, &finished); err != nil {
		log.Warn().Err(err).Str("task", taskName).Msg("record a-stock recommendation generate task run failed")
	}
}

func (w *Worker) recordAStockPopupReadyResult(ctx context.Context, strategyDate string, period string, phase string, result aStockRecommendationGenerateResult, eventTime time.Time) {
	status := "success"
	ready := true
	if result.RecommendationCount <= 0 {
		status = "skipped"
		ready = false
	}
	message := fmt.Sprintf("date=%s period=%s phase=%s popup_ready=%t recommendations=%d",
		nonEmpty(result.StrategyDate, normalizeAStockRecommendationDate(strategyDate)),
		normalizeAStockRecommendationPeriod(period),
		normalizeAStockRecommendationPhase(phase),
		ready,
		result.RecommendationCount,
	)
	finished := eventTime
	taskName := fmt.Sprintf("a-stock-popup-ready:%s", normalizeAStockRecommendationPeriod(period))
	if err := w.recordTaskRun(ctx, taskName, status, message, eventTime, &finished); err != nil {
		log.Warn().Err(err).Str("task", taskName).Msg("record a-stock popup ready task run failed")
	}
}

func (w *Worker) generateAStockRecommendationSnapshot(ctx context.Context, strategyDate string, period string, phase string) error {
	return w.generateAStockRecommendationSnapshotWithMode(ctx, strategyDate, period, phase, "")
}

func (w *Worker) refreshAStockRecommendationBacktestSnapshot(ctx context.Context, strategyDate string, period string) error {
	return w.generateAStockRecommendationSnapshotWithMode(ctx, strategyDate, period, "final", "preserve_locked")
}

func (w *Worker) generateAStockRecommendationSnapshotWithMode(ctx context.Context, strategyDate string, period string, phase string, refreshMode string) error {
	_, err := w.generateAStockRecommendationSnapshotWithModeResult(ctx, strategyDate, period, phase, refreshMode)
	return err
}

func (w *Worker) generateAStockRecommendationSnapshotWithModeAndFundFlow(ctx context.Context, strategyDate string, period string, phase string, refreshMode string, ignoreFundFlow bool) error {
	_, err := w.generateAStockRecommendationSnapshotWithModeAndFundFlowResult(ctx, strategyDate, period, phase, refreshMode, ignoreFundFlow)
	return err
}

func (w *Worker) generateAStockRecommendationSnapshotWithModeResult(ctx context.Context, strategyDate string, period string, phase string, refreshMode string) (aStockRecommendationGenerateResult, error) {
	return w.generateAStockRecommendationSnapshotWithModeAndFundFlowResult(ctx, strategyDate, period, phase, refreshMode, false)
}

func (w *Worker) generateAStockRecommendationSnapshotWithModeAndFundFlowResult(ctx context.Context, strategyDate string, period string, phase string, refreshMode string, ignoreFundFlow bool) (aStockRecommendationGenerateResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.GatewayWebURL), "/")
	if baseURL == "" {
		return aStockRecommendationGenerateResult{}, fmt.Errorf("YUQING_GATEWAY_URL not configured")
	}
	var envelope struct {
		Data aStockRecommendationGenerateResult `json:"data"`
	}
	req := w.crawlClient.R().
		SetContext(ctx).
		SetResult(&envelope).
		SetQueryParam("date", normalizeAStockRecommendationDate(strategyDate)).
		SetQueryParam("period", normalizeAStockRecommendationPeriod(period)).
		SetQueryParam("phase", normalizeAStockRecommendationPhase(phase))
	if strings.TrimSpace(refreshMode) != "" {
		req.SetQueryParam("refresh_mode", strings.TrimSpace(refreshMode))
	}
	if ignoreFundFlow {
		req.SetQueryParam("ignore_fund_flow", "1")
	}
	resp, err := req.Post(baseURL + "/internal/a-stock/recommendations/generate")
	if err != nil {
		return aStockRecommendationGenerateResult{}, err
	}
	if !resp.IsSuccess() {
		return aStockRecommendationGenerateResult{}, fmt.Errorf("a-stock recommendation generate failed: %s", resp.Status())
	}
	result := envelope.Data
	if result.StrategyDate == "" {
		result.StrategyDate = normalizeAStockRecommendationDate(strategyDate)
	}
	if result.Period == "" {
		result.Period = normalizeAStockRecommendationPeriod(period)
	}
	if result.Phase == "" {
		result.Phase = normalizeAStockRecommendationPhase(phase)
	}
	return result, nil
}

func normalizeAStockRecommendationDate(strategyDate string) string {
	return strings.TrimSpace(strategyDate)
}

func normalizeAStockRecommendationPeriod(period string) string {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "after", "pm":
		return "afternoon"
	case "evening", "night", "pm2":
		return "evening"
	case "afternoon":
		return "afternoon"
	default:
		return "morning"
	}
}

func normalizeAStockRecommendationPhase(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "preopen":
		return "preopen"
	default:
		return "final"
	}
}

func (w *Worker) aStockRecommendationCrawlSources() []string {
	sources := make([]string, 0, len(aStockRecommendationSources))
	settings, _ := astocknews.LoadSettings("")
	for _, sourceType := range aStockRecommendationSources {
		if sourceType == "jin10_full" && !w.cfg.Jin10FullEnabled {
			continue
		}
		if !astocknews.CrawlEnabled(settings, sourceType) {
			continue
		}
		sources = append(sources, sourceType)
	}
	return sources
}

func (w *Worker) loadAStockTradingDayStatus(ctx context.Context, strategyDate string) (aStockTradingDayStatus, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	date := strings.TrimSpace(strategyDate)
	if date == "" {
		date = time.Now().In(aStockLocation()).Format("2006-01-02")
	}
	if baseURL == "" {
		status := localAStockTradingDayStatus(date, "scheduler_local_calendar_fallback")
		log.Warn().
			Str("date", status.Date).
			Bool("is_trading_day", status.IsTradingDay).
			Msg("YUQING_ASTOCK_AUCTION_URL not configured; using local A-stock trading calendar fallback")
		return status, nil
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

func localAStockTradingDayStatus(date string, source string) aStockTradingDayStatus {
	date = strings.TrimSpace(date)
	if date == "" {
		date = time.Now().In(aStockLocation()).Format("2006-01-02")
	}
	isTradingDay := astockcalendar.IsTradingDay(date)
	reason := "trading_day"
	message := "A-share market is open."
	if !isTradingDay {
		reason = "market_closed"
		message = "该日 A 股休市，不生成股票推荐。"
	}
	return aStockTradingDayStatus{
		Date:               date,
		IsTradingDay:       isTradingDay,
		LatestTradingDay:   astockcalendar.AdjacentTradingDay(date, 0),
		PreviousTradingDay: astockcalendar.AdjacentTradingDay(date, -1),
		NextTradingDay:     astockcalendar.AdjacentTradingDay(date, 1),
		Source:             source,
		Reason:             reason,
		Message:            message,
	}
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
	for cursor := startTime; !cursor.After(endTime); {
		period := quarterEndDate(cursor)
		if !period.Before(startTime) && !period.After(endTime) {
			out = append(out, period.Format("20060102"))
		}
		cursor = period.AddDate(0, 0, 1)
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

func aStockRecommendationWindow(strategyDate string, period string, phase string) (time.Time, time.Time, string, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	day, err := time.ParseInLocation("2006-01-02", strategyDate, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	normalizedPhase := normalizeAStockRecommendationPhase(phase)
	switch normalizeAStockRecommendationPeriod(period) {
	case "afternoon":
		if normalizedPhase == "preopen" {
			return time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, location),
				time.Date(day.Year(), day.Month(), day.Day(), 12, 56, 59, 0, location),
				"09:30-12:56:59", nil
		}
		return time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 13, 0, 59, 0, location),
			"09:30-13:00:59", nil
	case "evening":
		return time.Date(day.Year(), day.Month(), day.Day(), 15, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 18, 30, 59, 0, location),
			"15:00-18:30:59", nil
	default:
		if normalizedPhase == "preopen" {
			return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
				time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 59, 0, location),
				"08:00-09:26:59", nil
		}
		return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 59, 0, location),
			"08:00-09:26:59", nil
	}
}
