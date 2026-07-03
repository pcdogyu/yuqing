package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockCodeNames(ctx context.Context, items []model.AStockCodeName) (model.AStockCodeNameUpsertResult, error) {
	result := model.AStockCodeNameUpsertResult{Total: len(items)}
	if len(items) == 0 {
		return result, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if result, err = upsertAStockCodeNamesTx(ctx, tx, items); err != nil {
		return result, err
	}
	err = tx.Commit()
	return result, err
}

func (s *Store) ListAStockCodeNames(ctx context.Context, codes []string) (model.AStockCodeNameListResult, error) {
	result := model.AStockCodeNameListResult{Items: make([]model.AStockCodeName, 0)}
	normalized := normalizeAStockCodeNameCodes(codes)
	if len(normalized) == 0 {
		return result, nil
	}
	args := make([]any, 0, len(normalized))
	for _, code := range normalized {
		args = append(args, code)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT code, name, source, updated_at
FROM a_stock_code_names
WHERE code IN (`+questionPlaceholders(len(normalized))+`)
ORDER BY code ASC`, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item model.AStockCodeName
		var updatedAt string
		if err := rows.Scan(&item.Code, &item.Name, &item.Source, &updatedAt); err != nil {
			return result, err
		}
		item.UpdatedAt = mustParseRFC3339(updatedAt)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Total = len(result.Items)
	return result, nil
}

func upsertAStockCodeNamesTx(ctx context.Context, tx *Tx, items []model.AStockCodeName) (model.AStockCodeNameUpsertResult, error) {
	result := model.AStockCodeNameUpsertResult{Total: len(items)}
	if len(items) == 0 {
		return result, nil
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_code_names (code, name, source, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(code) DO UPDATE SET
	name = excluded.name,
	source = excluded.source,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		code := astockcode.Normalize(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !astockcode.HasResolvedName(code, name) {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		updatedAt := item.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		existed, err := aStockCodeNameExistsTx(ctx, tx, code)
		if err != nil {
			return result, err
		}
		if _, err := stmt.ExecContext(ctx, code, name, strings.TrimSpace(item.Source), updatedAt.UTC().Format(time.RFC3339)); err != nil {
			return result, err
		}
		if existed {
			result.Updated++
		} else {
			result.Inserted++
		}
	}
	return result, nil
}

func aStockCodeNameExistsTx(ctx context.Context, tx *Tx, code string) (bool, error) {
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_code_names WHERE code = ?`, code).Scan(&one); err == nil {
		return true, nil
	} else if err != sql.ErrNoRows {
		return false, err
	}
	return false, nil
}

func loadAStockCodeNameMapTx(ctx context.Context, tx *Tx, codes []string) (map[string]string, error) {
	normalized := normalizeAStockCodeNameCodes(codes)
	names := make(map[string]string, len(normalized))
	if len(normalized) == 0 {
		return names, nil
	}
	args := make([]any, 0, len(normalized))
	for _, code := range normalized {
		args = append(args, code)
	}
	rows, err := tx.QueryContext(ctx, `SELECT code, name FROM a_stock_code_names WHERE code IN (`+questionPlaceholders(len(normalized))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var name string
		if err := rows.Scan(&code, &name); err != nil {
			return nil, err
		}
		code = astockcode.Normalize(code)
		name = astockcode.DisplayName(code, name)
		if astockcode.IsShanghaiShenzhen(code) && astockcode.HasResolvedName(code, name) {
			names[code] = name
		}
	}
	return names, rows.Err()
}

func normalizeAStockCodeNameCodes(codes []string) []string {
	normalized := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, raw := range codes {
		code := astockcode.Normalize(raw)
		if !astockcode.IsShanghaiShenzhen(code) {
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
