package sqlite

import (
	"context"
	"database/sql"
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
	if recommendationsJSON == "" {
		recommendationsJSON = "[]"
	}
	backtestsJSON := strings.TrimSpace(snapshot.BacktestsJSON)
	if backtestsJSON == "" {
		backtestsJSON = "[]"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO a_stock_recommendation_snapshots (
	strategy_date, period, ignore_recent, recommendations_json, backtests_json, backtest_status,
	generated_count, recent_filtered, same_day_morning_filtered, limit_up_filter_enabled,
	limit_up_filtered, today_market_filter_enabled, no_today_market_count, market_candidate_status,
	market_candidate_count, auction_amount_label, empty_reason, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(strategy_date, period, ignore_recent) DO UPDATE SET
	recommendations_json = excluded.recommendations_json,
	backtests_json = excluded.backtests_json,
	backtest_status = excluded.backtest_status,
	generated_count = excluded.generated_count,
	recent_filtered = excluded.recent_filtered,
	same_day_morning_filtered = excluded.same_day_morning_filtered,
	limit_up_filter_enabled = excluded.limit_up_filter_enabled,
	limit_up_filtered = excluded.limit_up_filtered,
	today_market_filter_enabled = excluded.today_market_filter_enabled,
	no_today_market_count = excluded.no_today_market_count,
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
		strings.TrimSpace(snapshot.BacktestStatus),
		snapshot.GeneratedCount,
		snapshot.RecentFiltered,
		snapshot.SameDayMorningFiltered,
		limitUpFilterEnabled,
		snapshot.LimitUpFiltered,
		todayMarketFilterEnabled,
		snapshot.NoTodayMarketCount,
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
	generated_count, recent_filtered, same_day_morning_filtered, limit_up_filter_enabled,
	limit_up_filtered, today_market_filter_enabled, no_today_market_count, market_candidate_status,
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
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&snapshot.StrategyDate,
		&snapshot.Period,
		&ignoreRecent,
		&snapshot.RecommendationsJSON,
		&snapshot.BacktestsJSON,
		&snapshot.BacktestStatus,
		&snapshot.GeneratedCount,
		&snapshot.RecentFiltered,
		&snapshot.SameDayMorningFiltered,
		&limitUpFilterEnabled,
		&snapshot.LimitUpFiltered,
		&todayMarketFilterEnabled,
		&snapshot.NoTodayMarketCount,
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
	snapshot.LimitUpFilterEnabled = limitUpFilterEnabled != 0
	snapshot.TodayMarketFilterEnabled = todayMarketFilterEnabled != 0
	snapshot.CreatedAt = mustParseRFC3339(createdAt)
	snapshot.UpdatedAt = mustParseRFC3339(updatedAt)
	return snapshot, nil
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
	strategy_date, period, code, rank, hotspot, name, hotspot_score, market_score, reason, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(strategy_date, period, code) DO UPDATE SET
	rank = excluded.rank,
	hotspot = excluded.hotspot,
	name = excluded.name,
	hotspot_score = excluded.hotspot_score,
	market_score = excluded.market_score,
	reason = excluded.reason,
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
SELECT strategy_date, period, rank, hotspot, code, name, hotspot_score, market_score, reason, created_at, updated_at
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
