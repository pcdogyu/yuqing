package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockRecommendationCandidateAuditRun(ctx context.Context, run model.AStockRecommendationCandidateAuditRun) error {
	run.RunID = strings.TrimSpace(run.RunID)
	run.StrategyDate = strings.TrimSpace(run.StrategyDate)
	run.Period = strings.TrimSpace(run.Period)
	run.Phase = strings.TrimSpace(run.Phase)
	if run.RunID == "" || run.StrategyDate == "" || run.Period == "" || run.Phase == "" {
		return nil
	}
	if run.ExitCounts == nil {
		run.ExitCounts = map[string]int{}
	}
	itemsJSON, err := json.Marshal(run.Items)
	if err != nil {
		return err
	}
	exitCountsJSON, err := json.Marshal(run.ExitCounts)
	if err != nil {
		return err
	}
	createdAt := run.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO a_stock_recommendation_candidate_audit_runs (
 run_id, strategy_date, period, phase, raw_candidate_count, valid_candidate_count,
 hotspot_linked_count, scored_count, selected_count, exit_counts_json, items_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
 raw_candidate_count=excluded.raw_candidate_count, valid_candidate_count=excluded.valid_candidate_count,
 hotspot_linked_count=excluded.hotspot_linked_count, scored_count=excluded.scored_count,
 selected_count=excluded.selected_count, exit_counts_json=excluded.exit_counts_json,
 items_json=excluded.items_json, created_at=excluded.created_at`,
		run.RunID, run.StrategyDate, run.Period, run.Phase, run.RawCandidateCount, run.ValidCandidateCount,
		run.HotspotLinkedCount, run.ScoredCount, run.SelectedCount, string(exitCountsJSON), string(itemsJSON), createdAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetAStockRecommendationCandidateAuditRun(ctx context.Context, filter model.AStockRecommendationCandidateAuditFilter) (model.AStockRecommendationCandidateAuditRun, bool, error) {
	args := []any{strings.TrimSpace(filter.StrategyDate), strings.TrimSpace(filter.Period)}
	query := `SELECT run_id, strategy_date, period, phase, raw_candidate_count, valid_candidate_count,
hotspot_linked_count, scored_count, selected_count, exit_counts_json, items_json, created_at
FROM a_stock_recommendation_candidate_audit_runs WHERE strategy_date = ? AND period = ?`
	if runID := strings.TrimSpace(filter.RunID); runID != "" {
		query += " AND run_id = ?"
		args = append(args, runID)
	} else if phase := strings.TrimSpace(filter.Phase); phase != "" {
		query += " AND phase = ?"
		args = append(args, phase)
	}
	query += " ORDER BY created_at DESC LIMIT 1"
	var run model.AStockRecommendationCandidateAuditRun
	var exitsJSON, itemsJSON, createdAt string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&run.RunID, &run.StrategyDate, &run.Period, &run.Phase,
		&run.RawCandidateCount, &run.ValidCandidateCount, &run.HotspotLinkedCount, &run.ScoredCount,
		&run.SelectedCount, &exitsJSON, &itemsJSON, &createdAt)
	if err == sql.ErrNoRows {
		return model.AStockRecommendationCandidateAuditRun{}, false, nil
	}
	if err != nil {
		return model.AStockRecommendationCandidateAuditRun{}, false, err
	}
	_ = json.Unmarshal([]byte(exitsJSON), &run.ExitCounts)
	_ = json.Unmarshal([]byte(itemsJSON), &run.Items)
	run.CreatedAt = mustParseRFC3339(createdAt)
	return run, true, nil
}
