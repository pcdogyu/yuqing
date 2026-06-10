package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Store) CreateWechatChallenge(ctx context.Context, challenge model.WechatChallenge) (model.WechatChallenge, error) {
	now := time.Now().UTC()
	if challenge.SceneStr == "" {
		return model.WechatChallenge{}, errors.New("scene string required")
	}
	if challenge.Purpose == "" {
		challenge.Purpose = "login"
	}
	if challenge.Status == "" {
		challenge.Status = "pending"
	}
	if challenge.CreatedAt.IsZero() {
		challenge.CreatedAt = now
	}
	if challenge.UpdatedAt.IsZero() {
		challenge.UpdatedAt = now
	}
	if challenge.ExpiresAt.IsZero() {
		challenge.ExpiresAt = now.Add(10 * time.Minute)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO wechat_challenges (scene_str, purpose, user_id, openid, session_token, status, expires_at, created_at, updated_at, completed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(scene_str) DO UPDATE SET
	purpose = excluded.purpose,
	user_id = excluded.user_id,
	openid = excluded.openid,
	session_token = excluded.session_token,
	status = excluded.status,
	expires_at = excluded.expires_at,
	updated_at = excluded.updated_at,
	completed_at = excluded.completed_at`,
		challenge.SceneStr,
		challenge.Purpose,
		challenge.UserID,
		strings.TrimSpace(challenge.OpenID),
		strings.TrimSpace(challenge.SessionToken),
		nonEmpty(challenge.Status, "pending"),
		challenge.ExpiresAt.Format(time.RFC3339),
		challenge.CreatedAt.Format(time.RFC3339),
		challenge.UpdatedAt.Format(time.RFC3339),
		nullTimeString(challenge.CompletedAt),
	)
	return challenge, err
}

func (s *Store) GetWechatChallenge(ctx context.Context, sceneStr string) (model.WechatChallenge, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT scene_str, purpose, user_id, openid, session_token, status, expires_at, created_at, updated_at, completed_at
FROM wechat_challenges
WHERE scene_str = ?`, strings.TrimSpace(sceneStr))
	challenge, err := scanWechatChallenge(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.WechatChallenge{}, ErrNotFound
		}
		return model.WechatChallenge{}, err
	}
	if challenge.ExpiresAt.Before(time.Now().UTC()) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM wechat_challenges WHERE scene_str = ?`, strings.TrimSpace(sceneStr))
		return model.WechatChallenge{}, ErrNotFound
	}
	return challenge, nil
}

func (s *Store) UpdateWechatChallenge(ctx context.Context, challenge model.WechatChallenge) (model.WechatChallenge, error) {
	challenge.UpdatedAt = time.Now().UTC()
	if challenge.SceneStr == "" {
		return model.WechatChallenge{}, errors.New("scene string required")
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE wechat_challenges
SET purpose = ?, user_id = ?, openid = ?, session_token = ?, status = ?, expires_at = ?, updated_at = ?, completed_at = ?
WHERE scene_str = ?`,
		challenge.Purpose,
		challenge.UserID,
		strings.TrimSpace(challenge.OpenID),
		strings.TrimSpace(challenge.SessionToken),
		nonEmpty(challenge.Status, "pending"),
		challenge.ExpiresAt.Format(time.RFC3339),
		challenge.UpdatedAt.Format(time.RFC3339),
		nullTimeString(challenge.CompletedAt),
		challenge.SceneStr,
	)
	if err != nil {
		return model.WechatChallenge{}, err
	}
	return s.GetWechatChallenge(ctx, challenge.SceneStr)
}

func (s *Store) DeleteWechatChallenge(ctx context.Context, sceneStr string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM wechat_challenges WHERE scene_str = ?`, strings.TrimSpace(sceneStr))
	return err
}

func (s *Store) DeleteExpiredWechatChallenges(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM wechat_challenges WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) UpsertWechatBinding(ctx context.Context, binding model.WechatBinding) (model.WechatBinding, error) {
	now := time.Now().UTC()
	if binding.UserID <= 0 {
		return model.WechatBinding{}, errors.New("user id required")
	}
	openID := strings.TrimSpace(binding.OpenID)
	if openID == "" {
		return model.WechatBinding{}, errors.New("openid required")
	}
	binding.BoundAt = now
	binding.UpdatedAt = now
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.WechatBinding{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM wechat_bindings WHERE openid = ? AND user_id <> ?`, openID, binding.UserID); err != nil {
		return model.WechatBinding{}, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO wechat_bindings (user_id, openid, bound_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
	openid = excluded.openid,
	bound_at = excluded.bound_at,
	updated_at = excluded.updated_at`,
		binding.UserID,
		openID,
		binding.BoundAt.Format(time.RFC3339),
		binding.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.WechatBinding{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.WechatBinding{}, err
	}
	return s.GetWechatBindingByUserID(ctx, binding.UserID)
}

func (s *Store) GetWechatBindingByUserID(ctx context.Context, userID int64) (model.WechatBinding, error) {
	row := s.db.QueryRowContext(ctx, `SELECT user_id, openid, bound_at, updated_at FROM wechat_bindings WHERE user_id = ?`, userID)
	binding, err := scanWechatBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.WechatBinding{}, ErrNotFound
		}
		return model.WechatBinding{}, err
	}
	return binding, nil
}

func (s *Store) GetWechatBindingByOpenID(ctx context.Context, openID string) (model.WechatBinding, error) {
	row := s.db.QueryRowContext(ctx, `SELECT user_id, openid, bound_at, updated_at FROM wechat_bindings WHERE openid = ?`, strings.TrimSpace(openID))
	binding, err := scanWechatBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.WechatBinding{}, ErrNotFound
		}
		return model.WechatBinding{}, err
	}
	return binding, nil
}

func (s *Store) ListWechatBindings(ctx context.Context, limit int) ([]model.WechatBinding, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, openid, bound_at, updated_at FROM wechat_bindings ORDER BY updated_at DESC, user_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bindings := make([]model.WechatBinding, 0, limit)
	for rows.Next() {
		binding, scanErr := scanWechatBinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (s *Store) GetUserByOpenID(ctx context.Context, openID string) (model.User, error) {
	binding, err := s.GetWechatBindingByOpenID(ctx, openID)
	if err != nil {
		return model.User{}, err
	}
	return s.GetUserByID(ctx, binding.UserID)
}

func scanWechatChallenge(scanner scanner) (model.WechatChallenge, error) {
	var challenge model.WechatChallenge
	var expiresAt, createdAt, updatedAt string
	var completedAt sql.NullString
	if err := scanner.Scan(
		&challenge.SceneStr,
		&challenge.Purpose,
		&challenge.UserID,
		&challenge.OpenID,
		&challenge.SessionToken,
		&challenge.Status,
		&expiresAt,
		&createdAt,
		&updatedAt,
		&completedAt,
	); err != nil {
		return model.WechatChallenge{}, err
	}
	challenge.ExpiresAt = mustParseRFC3339(expiresAt)
	challenge.CreatedAt = mustParseRFC3339(createdAt)
	challenge.UpdatedAt = mustParseRFC3339(updatedAt)
	if completedAt.Valid {
		value := mustParseRFC3339(completedAt.String)
		challenge.CompletedAt = &value
	}
	return challenge, nil
}

func scanWechatBinding(scanner scanner) (model.WechatBinding, error) {
	var binding model.WechatBinding
	var boundAt, updatedAt string
	if err := scanner.Scan(&binding.UserID, &binding.OpenID, &boundAt, &updatedAt); err != nil {
		return model.WechatBinding{}, err
	}
	binding.BoundAt = mustParseRFC3339(boundAt)
	binding.UpdatedAt = mustParseRFC3339(updatedAt)
	return binding, nil
}

func nullTimeString(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}
