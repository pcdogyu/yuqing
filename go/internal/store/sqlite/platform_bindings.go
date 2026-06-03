package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Store) GetPlatformBinding(ctx context.Context, userID int64, kind string) (model.PlatformBinding, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT user_id, kind, secret_id, secret_key, bound, created_at, updated_at
FROM platform_bindings
WHERE user_id = ? AND kind = ?`, userID, strings.TrimSpace(kind))
	binding, err := scanPlatformBinding(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.PlatformBinding{}, ErrNotFound
		}
		return model.PlatformBinding{}, err
	}
	return binding, nil
}

func (s *Store) UpsertPlatformBinding(ctx context.Context, binding model.PlatformBinding) (model.PlatformBinding, error) {
	now := time.Now().UTC()
	binding.Kind = strings.TrimSpace(binding.Kind)
	binding.SecretID = strings.TrimSpace(binding.SecretID)
	binding.SecretKey = strings.TrimSpace(binding.SecretKey)
	if binding.Kind == "" {
		binding.Kind = "default"
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO platform_bindings (user_id, kind, secret_id, secret_key, bound, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, kind) DO UPDATE SET
	secret_id = excluded.secret_id,
	secret_key = excluded.secret_key,
	bound = excluded.bound,
	updated_at = excluded.updated_at`,
		binding.UserID, binding.Kind, binding.SecretID, binding.SecretKey, boolToInt(binding.Bound), binding.CreatedAt.Format(time.RFC3339), binding.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.PlatformBinding{}, err
	}
	return s.GetPlatformBinding(ctx, binding.UserID, binding.Kind)
}

func scanPlatformBinding(scanner scanner) (model.PlatformBinding, error) {
	var binding model.PlatformBinding
	var bound int
	var createdAt, updatedAt string
	if err := scanner.Scan(&binding.UserID, &binding.Kind, &binding.SecretID, &binding.SecretKey, &bound, &createdAt, &updatedAt); err != nil {
		return model.PlatformBinding{}, err
	}
	binding.Bound = bound == 1
	binding.CreatedAt = mustParseRFC3339(createdAt)
	binding.UpdatedAt = mustParseRFC3339(updatedAt)
	return binding, nil
}
