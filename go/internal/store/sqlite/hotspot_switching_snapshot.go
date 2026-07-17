package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) GetHotspotSwitchingSnapshot(ctx context.Context, days int) (model.HotspotSwitchingResult, bool, error) {
	if days <= 0 {
		days = 14
	}
	var payload string
	row := s.db.QueryRowContext(ctx, `
SELECT payload_json
FROM hotspot_switching_snapshots
WHERE days = ?`, days)
	if err := row.Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return model.HotspotSwitchingResult{Days: days}, false, nil
		}
		return model.HotspotSwitchingResult{}, false, err
	}
	var result model.HotspotSwitchingResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return model.HotspotSwitchingResult{}, false, err
	}
	if result.Days == 0 {
		result.Days = days
	}
	return result, true, nil
}

func (s *Store) UpsertHotspotSwitchingSnapshot(ctx context.Context, days int, result model.HotspotSwitchingResult, capturedAt time.Time) error {
	if days <= 0 {
		days = 14
	}
	if result.Days == 0 {
		result.Days = days
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO hotspot_switching_snapshots (days, payload_json, captured_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(days) DO UPDATE SET
	payload_json = excluded.payload_json,
	captured_at = excluded.captured_at,
	updated_at = excluded.updated_at`,
		days,
		string(payload),
		capturedAt.UTC().Format(time.RFC3339),
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)
	return err
}
