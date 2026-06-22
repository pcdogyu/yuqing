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
	generated_count, recent_filtered, same_day_morning_filtered, market_candidate_status,
	market_candidate_count, auction_amount_label, empty_reason, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(strategy_date, period, ignore_recent) DO UPDATE SET
	recommendations_json = excluded.recommendations_json,
	backtests_json = excluded.backtests_json,
	backtest_status = excluded.backtest_status,
	generated_count = excluded.generated_count,
	recent_filtered = excluded.recent_filtered,
	same_day_morning_filtered = excluded.same_day_morning_filtered,
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
	generated_count, recent_filtered, same_day_morning_filtered, market_candidate_status,
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
	snapshot.CreatedAt = mustParseRFC3339(createdAt)
	snapshot.UpdatedAt = mustParseRFC3339(updatedAt)
	return snapshot, nil
}
