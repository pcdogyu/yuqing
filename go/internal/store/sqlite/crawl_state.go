package sqlite

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) GetCrawlState(ctx context.Context, sourceType, cursorKey string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT cursor_value FROM crawl_states WHERE source_type = ? AND cursor_key = ?`, sourceType, cursorKey).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) UpsertCrawlState(ctx context.Context, sourceType, cursorKey, cursorValue string, updatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO crawl_states (source_type, cursor_key, cursor_value, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(source_type, cursor_key) DO UPDATE SET
	cursor_value = excluded.cursor_value,
	updated_at = excluded.updated_at
`, sourceType, cursorKey, cursorValue, updatedAt.Format(time.RFC3339))
	return err
}
