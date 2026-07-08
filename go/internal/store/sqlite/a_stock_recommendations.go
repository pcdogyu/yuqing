package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockRecommendationSnapshot(ctx context.Context, snapshot model.AStockRecommendationSnapshot) (model.AStockRecommendationSnapshotUpsertResult, error) {
	result := model.AStockRecommendationSnapshotUpsertResult{}
	snapshot.StrategyDate = strings.TrimSpace(snapshot.StrategyDate)
	snapshot.Period = strings.TrimSpace(snapshot.Period)
	if snapshot.StrategyDate == "" || snapshot.Period == "" {
		return result, nil
	}
	ignoreRecent := 0
	if snapshot.IgnoreRecent {
		ignoreRecent = 1
	}
	limitUpFilterEnabled := 0
	if snapshot.LimitUpFilterEnabled {
		limitUpFilterEnabled = 1
	}
	todayMarketFilterEnabled := 0
	if snapshot.TodayMarketFilterEnabled {
		todayMarketFilterEnabled = 1
	}
	fundFlowFilterEnabled := 0
	if snapshot.FundFlowFilterEnabled {
		fundFlowFilterEnabled = 1
	}
	existed := false
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM a_stock_recommendation_snapshots WHERE strategy_date = ? AND period = ? AND ignore_recent = ?`, snapshot.StrategyDate, snapshot.Period, ignoreRecent).Scan(new(int)); err == nil {
		existed = true
	} else if err != sql.ErrNoRows {
		return result, err
	}
	now := time.Now().UTC()
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := snapshot.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	recommendationsJSON := strings.TrimSpace(snapshot.RecommendationsJSON)
	recommendationsJSON = normalizeAStockRecommendationJSONArray(recommendationsJSON)
	backtestsJSON := strings.TrimSpace(snapshot.BacktestsJSON)
	backtestsJSON = normalizeAStockRecommendationJSONArray(backtestsJSON)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO a_stock_recommendation_snapshots (
	strategy_date, period, ignore_recent, recommendations_json, backtests_json, news_summary_json, backtest_status,
	generated_count, recent_filtered, same_day_morning_filtered, limit_up_filter_enabled,
	limit_up_filtered, today_market_filter_enabled, no_today_market_count, fund_flow_filter_enabled,
	fund_flow_filtered, fund_flow_missing_count, market_candidate_status,
	market_candidate_count, auction_amount_label, empty_reason, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(strategy_date, period, ignore_recent) DO UPDATE SET
	recommendations_json = excluded.recommendations_json,
	backtests_json = excluded.backtests_json,
	news_summary_json = excluded.news_summary_json,
	backtest_status = excluded.backtest_status,
	generated_count = excluded.generated_count,
	recent_filtered = excluded.recent_filtered,
	same_day_morning_filtered = excluded.same_day_morning_filtered,
	limit_up_filter_enabled = excluded.limit_up_filter_enabled,
	limit_up_filtered = excluded.limit_up_filtered,
	today_market_filter_enabled = excluded.today_market_filter_enabled,
	no_today_market_count = excluded.no_today_market_count,
	fund_flow_filter_enabled = excluded.fund_flow_filter_enabled,
	fund_flow_filtered = excluded.fund_flow_filtered,
	fund_flow_missing_count = excluded.fund_flow_missing_count,
	market_candidate_status = excluded.market_candidate_status,
	market_candidate_count = excluded.market_candidate_count,
	auction_amount_label = excluded.auction_amount_label,
	empty_reason = excluded.empty_reason,
	updated_at = excluded.updated_at`,
		snapshot.StrategyDate,
		snapshot.Period,
		ignoreRecent,
		recommendationsJSON,
		backtestsJSON,
		strings.TrimSpace(snapshot.NewsSummaryJSON),
		strings.TrimSpace(snapshot.BacktestStatus),
		snapshot.GeneratedCount,
		snapshot.RecentFiltered,
		snapshot.SameDayMorningFiltered,
		limitUpFilterEnabled,
		snapshot.LimitUpFiltered,
		todayMarketFilterEnabled,
		snapshot.NoTodayMarketCount,
		fundFlowFilterEnabled,
		snapshot.FundFlowFiltered,
		snapshot.FundFlowMissingCount,
		strings.TrimSpace(snapshot.MarketCandidateStatus),
		snapshot.MarketCandidateCount,
		strings.TrimSpace(snapshot.AuctionAmountLabel),
		strings.TrimSpace(snapshot.EmptyReason),
		createdAt.UTC().Format(time.RFC3339),
		updatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return result, err
	}
	if existed {
		result.Updated = 1
	} else {
		result.Inserted = 1
	}
	return result, nil
}

func (s *Store) GetAStockRecommendationSnapshot(ctx context.Context, strategyDate string, period string, ignoreRecent bool) (model.AStockRecommendationSnapshot, bool, error) {
	ignoreRecentInt := 0
	if ignoreRecent {
		ignoreRecentInt = 1
	}
	row := s.db.QueryRowContext(ctx, `
SELECT strategy_date, period, ignore_recent, recommendations_json, backtests_json, backtest_status,
	news_summary_json, generated_count, recent_filtered, same_day_morning_filtered, limit_up_filter_enabled,
	limit_up_filtered, today_market_filter_enabled, no_today_market_count, fund_flow_filter_enabled,
	fund_flow_filtered, fund_flow_missing_count, market_candidate_status,
	market_candidate_count, auction_amount_label, empty_reason, created_at, updated_at
FROM a_stock_recommendation_snapshots
WHERE strategy_date = ? AND period = ? AND ignore_recent = ?`,
		strings.TrimSpace(strategyDate),
		strings.TrimSpace(period),
		ignoreRecentInt,
	)
	snapshot, err := scanAStockRecommendationSnapshot(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return model.AStockRecommendationSnapshot{}, false, nil
		}
		return model.AStockRecommendationSnapshot{}, false, err
	}
	snapshot.Found = true
	return snapshot, true, nil
}

func scanAStockRecommendationSnapshot(scanner scanner) (model.AStockRecommendationSnapshot, error) {
	var snapshot model.AStockRecommendationSnapshot
	var ignoreRecent int
	var limitUpFilterEnabled int
	var todayMarketFilterEnabled int
	var fundFlowFilterEnabled int
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&snapshot.StrategyDate,
		&snapshot.Period,
		&ignoreRecent,
		&snapshot.RecommendationsJSON,
		&snapshot.BacktestsJSON,
		&snapshot.BacktestStatus,
		&snapshot.NewsSummaryJSON,
		&snapshot.GeneratedCount,
		&snapshot.RecentFiltered,
		&snapshot.SameDayMorningFiltered,
		&limitUpFilterEnabled,
		&snapshot.LimitUpFiltered,
		&todayMarketFilterEnabled,
		&snapshot.NoTodayMarketCount,
		&fundFlowFilterEnabled,
		&snapshot.FundFlowFiltered,
		&snapshot.FundFlowMissingCount,
		&snapshot.MarketCandidateStatus,
		&snapshot.MarketCandidateCount,
		&snapshot.AuctionAmountLabel,
		&snapshot.EmptyReason,
		&createdAt,
		&updatedAt,
	); err != nil {
		return snapshot, err
	}
	snapshot.IgnoreRecent = ignoreRecent != 0
	snapshot.RecommendationsJSON = normalizeAStockRecommendationJSONArray(snapshot.RecommendationsJSON)
	snapshot.BacktestsJSON = normalizeAStockRecommendationJSONArray(snapshot.BacktestsJSON)
	snapshot.LimitUpFilterEnabled = limitUpFilterEnabled != 0
	snapshot.TodayMarketFilterEnabled = todayMarketFilterEnabled != 0
	snapshot.FundFlowFilterEnabled = fundFlowFilterEnabled != 0
	snapshot.CreatedAt = mustParseRFC3339(createdAt)
	snapshot.UpdatedAt = mustParseRFC3339(updatedAt)
	return snapshot, nil
}

func normalizeAStockRecommendationJSONArray(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return "[]"
	}
	return raw
}

func (s *Store) UpsertAStockRecommendationSelections(ctx context.Context, selectionSet model.AStockRecommendationSelectionSet) (model.AStockRecommendationSelectionUpsertResult, error) {
	result := model.AStockRecommendationSelectionUpsertResult{}
	strategyDate := strings.TrimSpace(selectionSet.StrategyDate)
	period := strings.TrimSpace(selectionSet.Period)
	if strategyDate == "" || period == "" {
		return result, nil
	}
	items := normalizeAStockRecommendationSelections(strategyDate, period, selectionSet.Items)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	existingCreatedAt := make(map[string]time.Time)
	rows, err := tx.QueryContext(ctx, `
SELECT code, created_at
FROM a_stock_recommendation_selections
WHERE strategy_date = ? AND period = ?`,
		strategyDate,
		period,
	)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var createdAt string
		if err := rows.Scan(&code, &createdAt); err != nil {
			return result, err
		}
		existingCreatedAt[strings.TrimSpace(code)] = mustParseRFC3339(createdAt)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	if len(items) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM a_stock_recommendation_selections WHERE strategy_date = ? AND period = ?`, strategyDate, period); err != nil {
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	}

	deleteArgs := make([]any, 0, len(items)+2)
	deleteArgs = append(deleteArgs, strategyDate, period)
	placeholders := make([]string, 0, len(items))
	for _, item := range items {
		placeholders = append(placeholders, "?")
		deleteArgs = append(deleteArgs, item.Code)
	}
	deleteQuery := `
DELETE FROM a_stock_recommendation_selections
WHERE strategy_date = ? AND period = ? AND code NOT IN (` + strings.Join(placeholders, ",") + `)`
	if _, err := tx.ExecContext(ctx, deleteQuery, deleteArgs...); err != nil {
		return result, err
	}

	now := time.Now().UTC()
	for _, item := range items {
		createdAt, existed := existingCreatedAt[item.Code]
		if !existed {
			createdAt = item.CreatedAt
			if createdAt.IsZero() {
				createdAt = now
			}
			result.Inserted++
		} else {
			result.Updated++
		}
		updatedAt := item.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO a_stock_recommendation_selections (
	strategy_date, period, code, rank, hotspot, name, hotspot_score, market_score, reason, entry_time, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(strategy_date, period, code) DO UPDATE SET
	rank = excluded.rank,
	hotspot = excluded.hotspot,
	name = excluded.name,
	hotspot_score = excluded.hotspot_score,
	market_score = excluded.market_score,
	reason = excluded.reason,
	entry_time = excluded.entry_time,
	updated_at = excluded.updated_at`,
			strategyDate,
			period,
			item.Code,
			item.Rank,
			item.Hotspot,
			item.Name,
			item.HotspotScore,
			item.MarketScore,
			item.Reason,
			item.EntryTime,
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.Total = len(items)
	return result, nil
}

func (s *Store) ListAStockRecommendationSelections(ctx context.Context, strategyDate string, period string) (model.AStockRecommendationSelectionListResult, error) {
	result := model.AStockRecommendationSelectionListResult{
		StrategyDate: strings.TrimSpace(strategyDate),
		Period:       strings.TrimSpace(period),
		Items:        make([]model.AStockRecommendationSelection, 0),
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT strategy_date, period, rank, hotspot, code, name, hotspot_score, market_score, reason, entry_time, created_at, updated_at
FROM a_stock_recommendation_selections
WHERE strategy_date = ? AND period = ?
ORDER BY rank ASC, code ASC`,
		result.StrategyDate,
		result.Period,
	)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanAStockRecommendationSelection(rows)
		if err != nil {
			return result, err
		}
		if result.CreatedAt.IsZero() || item.CreatedAt.Before(result.CreatedAt) {
			result.CreatedAt = item.CreatedAt
		}
		if result.UpdatedAt.IsZero() || item.UpdatedAt.After(result.UpdatedAt) {
			result.UpdatedAt = item.UpdatedAt
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Found = len(result.Items) > 0
	return result, nil
}

func (s *Store) ListAStockRecommendationLatestDates(ctx context.Context, strategyDate string, period string, codes []string) (model.AStockRecommendationLatestDateListResult, error) {
	result := model.AStockRecommendationLatestDateListResult{
		StrategyDate: strings.TrimSpace(strategyDate),
		Period:       strings.TrimSpace(period),
		Items:        make([]model.AStockRecommendationLatestDate, 0),
	}
	normalizedCodes := normalizeAStockRecommendationDateCodes(codes)
	if result.StrategyDate == "" || result.Period == "" || len(normalizedCodes) == 0 {
		return result, nil
	}
	wanted := make(map[string]struct{}, len(normalizedCodes))
	for _, code := range normalizedCodes {
		wanted[code] = struct{}{}
	}
	latestByCode := make(map[string]string, len(normalizedCodes))
	if err := s.collectAStockRecommendationLatestDatesFromSelections(ctx, result.StrategyDate, result.Period, normalizedCodes, latestByCode); err != nil {
		return result, err
	}
	if err := s.collectAStockRecommendationLatestDatesFromSnapshots(ctx, result.StrategyDate, result.Period, wanted, latestByCode); err != nil {
		return result, err
	}
	for _, code := range normalizedCodes {
		if latestDate := latestByCode[code]; latestDate != "" {
			result.Items = append(result.Items, model.AStockRecommendationLatestDate{
				Code:       code,
				LatestDate: latestDate,
			})
		}
	}
	return result, nil
}

func (s *Store) collectAStockRecommendationLatestDatesFromSelections(ctx context.Context, strategyDate string, period string, codes []string, latestByCode map[string]string) error {
	where, whereArgs := aStockRecommendationHistoryWhere(strategyDate, period)
	args := make([]any, 0, len(codes)+len(whereArgs))
	for _, code := range codes {
		args = append(args, code)
	}
	args = append(args, whereArgs...)
	rows, err := s.db.QueryContext(ctx, `
SELECT code, strategy_date
FROM a_stock_recommendation_selections
WHERE code IN (`+questionPlaceholders(len(codes))+`) AND `+where,
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var latestDate string
		if err := rows.Scan(&code, &latestDate); err != nil {
			return err
		}
		updateAStockRecommendationLatestDate(latestByCode, code, latestDate)
	}
	return rows.Err()
}

func (s *Store) collectAStockRecommendationLatestDatesFromSnapshots(ctx context.Context, strategyDate string, period string, wanted map[string]struct{}, latestByCode map[string]string) error {
	where, whereArgs := aStockRecommendationHistoryWhere(strategyDate, period)
	rows, err := s.db.QueryContext(ctx, `
SELECT strategy_date, recommendations_json
FROM a_stock_recommendation_snapshots
WHERE ignore_recent = 0 AND `+where,
		whereArgs...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var latestDate string
		var recommendationsJSON string
		if err := rows.Scan(&latestDate, &recommendationsJSON); err != nil {
			return err
		}
		for _, code := range aStockRecommendationSnapshotCodes(recommendationsJSON) {
			if _, ok := wanted[code]; !ok {
				continue
			}
			updateAStockRecommendationLatestDate(latestByCode, code, latestDate)
		}
	}
	return rows.Err()
}

func aStockRecommendationHistoryWhere(strategyDate string, period string) (string, []any) {
	strategyDate = strings.TrimSpace(strategyDate)
	period = strings.ToLower(strings.TrimSpace(period))
	if period == "afternoon" {
		return `(strategy_date < ? OR (strategy_date = ? AND period = ?))`, []any{strategyDate, strategyDate, "morning"}
	}
	return `strategy_date < ?`, []any{strategyDate}
}

func normalizeAStockRecommendationDateCodes(codes []string) []string {
	normalized := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, raw := range codes {
		code := normalizeAStockRecommendationDateCode(raw)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	return normalized
}

func normalizeAStockRecommendationDateCode(raw string) string {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "SH"), ".SH")
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "SZ"), ".SZ")
	raw = strings.TrimPrefix(raw, "1.")
	raw = strings.TrimPrefix(raw, "0.")
	if len(raw) >= 6 {
		return raw[:6]
	}
	return raw
}

func questionPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func aStockRecommendationSnapshotCodes(recommendationsJSON string) []string {
	recommendationsJSON = strings.TrimSpace(recommendationsJSON)
	if recommendationsJSON == "" {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(recommendationsJSON), &rows); err != nil {
		return nil
	}
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		code := ""
		for _, key := range []string{"Code", "code", "stock_code"} {
			if raw, ok := row[key].(string); ok {
				code = normalizeAStockRecommendationDateCode(raw)
				break
			}
		}
		if code != "" {
			codes = append(codes, code)
		}
	}
	return codes
}

func updateAStockRecommendationLatestDate(latestByCode map[string]string, code string, latestDate string) {
	code = normalizeAStockRecommendationDateCode(code)
	latestDate = strings.TrimSpace(latestDate)
	if code == "" || latestDate == "" {
		return
	}
	if current := latestByCode[code]; current == "" || latestDate > current {
		latestByCode[code] = latestDate
	}
}

func scanAStockRecommendationSelection(scanner scanner) (model.AStockRecommendationSelection, error) {
	var item model.AStockRecommendationSelection
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&item.StrategyDate,
		&item.Period,
		&item.Rank,
		&item.Hotspot,
		&item.Code,
		&item.Name,
		&item.HotspotScore,
		&item.MarketScore,
		&item.Reason,
		&item.EntryTime,
		&createdAt,
		&updatedAt,
	); err != nil {
		return item, err
	}
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

func normalizeAStockRecommendationSelections(strategyDate string, period string, items []model.AStockRecommendationSelection) []model.AStockRecommendationSelection {
	normalized := make([]model.AStockRecommendationSelection, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		item.StrategyDate = strategyDate
		item.Period = period
		item.Code = strings.TrimSpace(item.Code)
		item.Name = strings.TrimSpace(item.Name)
		item.Hotspot = strings.TrimSpace(item.Hotspot)
		item.Reason = strings.TrimSpace(item.Reason)
		item.EntryTime = strings.TrimSpace(item.EntryTime)
		if item.Code == "" {
			continue
		}
		if _, exists := seen[item.Code]; exists {
			continue
		}
		seen[item.Code] = struct{}{}
		if item.Rank <= 0 {
			item.Rank = len(normalized) + 1
		}
		normalized = append(normalized, item)
	}
	return normalized
}
