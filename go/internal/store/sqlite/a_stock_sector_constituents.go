package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockSectorConstituents(ctx context.Context, sectorType string, sectorName string, items []model.AStockSectorConstituent, replace bool) (model.AStockSectorConstituentUpsertResult, error) {
	sectorType = normalizeAStockSectorFundFlowSectorType(sectorType)
	sectorName = strings.TrimSpace(sectorName)
	result := model.AStockSectorConstituentUpsertResult{SectorType: sectorType, SectorName: sectorName, Total: len(items)}
	if sectorName == "" {
		for _, item := range items {
			if value := strings.TrimSpace(item.SectorName); value != "" {
				sectorName = value
				result.SectorName = value
				break
			}
		}
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
	if replace && sectorType != "" && sectorName != "" {
		if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_sector_constituents WHERE sector_type = ? AND sector_name = ?`, sectorType, sectorName); err != nil {
			return result, err
		}
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_sector_constituents (sector_type, sector_name, code, name, source, fetched_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(sector_type, sector_name, code) DO UPDATE SET
	name = excluded.name,
	source = excluded.source,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()
	now := time.Now().UTC()
	for _, item := range items {
		itemSectorType := normalizeAStockSectorFundFlowSectorType(nonEmpty(strings.TrimSpace(item.SectorType), sectorType))
		itemSectorName := strings.TrimSpace(nonEmpty(item.SectorName, sectorName))
		code := normalizeAStockFundFlowCode(item.Code)
		name := astockcode.DisplayName(code, strings.TrimSpace(item.Name))
		if itemSectorType == "" || itemSectorName == "" || !astockcode.IsShanghaiShenzhen(code) || !astockcode.HasResolvedName(code, name) {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_sector_constituents WHERE sector_type = ? AND sector_name = ? AND code = ?`, itemSectorType, itemSectorName, code).Scan(new(int)); scanErr == nil {
				existed = true
			} else if scanErr != sql.ErrNoRows {
				err = scanErr
				return result, err
			}
		}
		fetchedAt := item.FetchedAt
		if fetchedAt.IsZero() {
			fetchedAt = now
		}
		createdAt := item.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := item.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		if _, err = stmt.ExecContext(ctx,
			itemSectorType,
			itemSectorName,
			code,
			name,
			strings.TrimSpace(item.Source),
			fetchedAt.UTC().Format(time.RFC3339),
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return result, err
		}
		if existed {
			result.Updated++
		} else {
			result.Inserted++
		}
	}
	err = tx.Commit()
	return result, err
}

func (s *Store) ListAStockSectorConstituents(ctx context.Context, filter model.AStockSectorConstituentFilter) (model.AStockSectorConstituentListResult, error) {
	result := model.AStockSectorConstituentListResult{
		SectorType: normalizeAStockSectorFundFlowSectorType(filter.SectorType),
		SectorName: strings.TrimSpace(filter.SectorName),
		Keyword:    strings.TrimSpace(filter.Keyword),
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	where := "WHERE sector_type = ? AND sector_name = ?"
	args := []any{result.SectorType, result.SectorName}
	if result.Keyword != "" {
		like := "%" + result.Keyword + "%"
		where += " AND (code LIKE ? OR name LIKE ?)"
		args = append(args, like, like)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_sector_constituents `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM a_stock_sector_constituents `+where, args...).Scan(&fetchedAt); err != nil {
		return result, err
	}
	if fetchedAt.Valid && strings.TrimSpace(fetchedAt.String) != "" {
		value := mustParseRFC3339(fetchedAt.String)
		result.FetchedAt = &value
	}
	queryArgs := append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT sector_type, sector_name, code, name, source, fetched_at, created_at, updated_at
FROM a_stock_sector_constituents `+where+`
ORDER BY code ASC
LIMIT ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockSectorConstituent, 0, limit)
	for rows.Next() {
		var item model.AStockSectorConstituent
		var fetchedAtRaw, createdAt, updatedAt string
		if err := rows.Scan(&item.SectorType, &item.SectorName, &item.Code, &item.Name, &item.Source, &fetchedAtRaw, &createdAt, &updatedAt); err != nil {
			return result, err
		}
		item.FetchedAt = mustParseRFC3339(fetchedAtRaw)
		item.CreatedAt = mustParseRFC3339(createdAt)
		item.UpdatedAt = mustParseRFC3339(updatedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Items = items
	return result, nil
}
