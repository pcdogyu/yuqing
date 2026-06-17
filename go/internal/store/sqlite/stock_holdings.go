package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertStockInstitutionHoldings(ctx context.Context, items []model.StockInstitutionHolding) (model.StockInstitutionHoldingUpsertResult, error) {
	result := model.StockInstitutionHoldingUpsertResult{Total: len(items)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO stock_institution_holdings (stock_code, stock_name, report_period, announce_date, holder_name, holder_type, holder_code, holder_rank, shares, shares_change, change_ratio, float_ratio, market_value, source_type, source_url, raw_payload, fetched_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_type, report_period, stock_code, holder_name, holder_type, holder_code) DO UPDATE SET
	stock_name = excluded.stock_name,
	announce_date = excluded.announce_date,
	holder_rank = excluded.holder_rank,
	shares = excluded.shares,
	shares_change = excluded.shares_change,
	change_ratio = excluded.change_ratio,
	float_ratio = excluded.float_ratio,
	market_value = excluded.market_value,
	source_url = excluded.source_url,
	raw_payload = excluded.raw_payload,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, item := range items {
		item.StockCode = strings.TrimSpace(item.StockCode)
		item.ReportPeriod = strings.TrimSpace(item.ReportPeriod)
		item.HolderName = strings.TrimSpace(item.HolderName)
		item.HolderType = strings.TrimSpace(item.HolderType)
		item.HolderCode = strings.TrimSpace(item.HolderCode)
		item.SourceType = strings.TrimSpace(item.SourceType)
		if item.StockCode == "" || item.ReportPeriod == "" || item.HolderName == "" || item.HolderType == "" || item.SourceType == "" {
			continue
		}
		existed := false
		if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM stock_institution_holdings WHERE source_type = ? AND report_period = ? AND stock_code = ? AND holder_name = ? AND holder_type = ? AND holder_code = ?`,
			item.SourceType, item.ReportPeriod, item.StockCode, item.HolderName, item.HolderType, item.HolderCode).Scan(new(int)); scanErr == nil {
			existed = true
		} else if scanErr != sql.ErrNoRows {
			err = scanErr
			return result, err
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
			item.StockCode,
			strings.TrimSpace(item.StockName),
			item.ReportPeriod,
			strings.TrimSpace(item.AnnounceDate),
			item.HolderName,
			item.HolderType,
			item.HolderCode,
			strings.TrimSpace(item.HolderRank),
			item.Shares,
			item.SharesChange,
			item.ChangeRatio,
			item.FloatRatio,
			item.MarketValue,
			item.SourceType,
			strings.TrimSpace(item.SourceURL),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
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

func (s *Store) ListStockInstitutionHoldings(ctx context.Context, filter model.StockInstitutionHoldingFilter) (model.StockInstitutionHoldingListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	result := model.StockInstitutionHoldingListResult{
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		Code:       strings.TrimSpace(filter.Code),
		Company:    strings.TrimSpace(filter.Company),
		Period:     strings.TrimSpace(filter.Period),
		Holder:     strings.TrimSpace(filter.Holder),
		HolderType: strings.TrimSpace(filter.HolderType),
		Source:     strings.TrimSpace(filter.Source),
	}
	where, args := stockHoldingWhere(result)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_institution_holdings `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	periods, err := s.listStockHoldingDistinct(ctx, "report_period")
	if err != nil {
		return result, err
	}
	result.Periods = periods
	sources, err := s.listStockHoldingDistinct(ctx, "source_type")
	if err != nil {
		return result, err
	}
	result.Sources = sources
	holderTypes, err := s.listStockHoldingDistinct(ctx, "holder_type")
	if err != nil {
		return result, err
	}
	result.HolderTypes = holderTypes

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, stock_code, stock_name, report_period, announce_date, holder_name, holder_type, holder_code, holder_rank, shares, shares_change, change_ratio, float_ratio, market_value, source_type, source_url, raw_payload, fetched_at, created_at, updated_at
FROM stock_institution_holdings `+where+`
ORDER BY report_period DESC, market_value DESC, shares DESC, id DESC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.StockInstitutionHolding, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanStockInstitutionHolding(rows)
		if scanErr != nil {
			return result, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Items = items
	return result, nil
}

func (s *Store) GetStockInstitutionHoldingSummary(ctx context.Context, code string, period string) (model.StockInstitutionHoldingSummary, error) {
	code = strings.TrimSpace(code)
	period = strings.TrimSpace(period)
	summary := model.StockInstitutionHoldingSummary{StockCode: code, ReportPeriod: period}
	where := "WHERE 1=1"
	args := []any{}
	if code != "" {
		where += " AND stock_code = ?"
		args = append(args, code)
	}
	if period != "" {
		where += " AND report_period = ?"
		args = append(args, period)
	} else if code != "" {
		var latest string
		if err := s.db.QueryRowContext(ctx, `SELECT report_period FROM stock_institution_holdings WHERE stock_code = ? AND report_period <> '' ORDER BY report_period DESC LIMIT 1`, code).Scan(&latest); err == nil {
			where += " AND report_period = ?"
			args = append(args, latest)
			summary.ReportPeriod = latest
		} else if err != sql.ErrNoRows {
			return summary, err
		}
	}
	row := s.db.QueryRowContext(ctx, `
SELECT COALESCE(MAX(stock_name), ''), COUNT(DISTINCT holder_name || '|' || holder_type || '|' || holder_code),
	COUNT(DISTINCT CASE WHEN holder_type = 'fund' THEN holder_name || '|' || holder_code END),
	COUNT(DISTINCT holder_type), COALESCE(SUM(shares), 0), COALESCE(SUM(float_ratio), 0), COALESCE(SUM(market_value), 0)
FROM stock_institution_holdings `+where, args...)
	if err := row.Scan(&summary.StockName, &summary.HolderCount, &summary.FundCount, &summary.HolderTypeCount, &summary.TotalShares, &summary.TotalFloatRatio, &summary.TotalMarketValue); err != nil {
		return summary, err
	}
	maxRow := s.db.QueryRowContext(ctx, `
SELECT holder_name, holder_type, shares
FROM stock_institution_holdings `+where+`
ORDER BY shares DESC, market_value DESC, id DESC
LIMIT 1`, args...)
	if err := maxRow.Scan(&summary.MaxHolderName, &summary.MaxHolderType, &summary.MaxHolderShares); err != nil && err != sql.ErrNoRows {
		return summary, err
	}
	return summary, nil
}

func stockHoldingWhere(filter model.StockInstitutionHoldingListResult) (string, []any) {
	where := "WHERE 1=1"
	args := []any{}
	if filter.Code != "" {
		where += " AND stock_code = ?"
		args = append(args, filter.Code)
	}
	if filter.Company != "" {
		like := "%" + filter.Company + "%"
		where += " AND (stock_name LIKE ? OR stock_code LIKE ?)"
		args = append(args, like, like)
	}
	if filter.Period != "" {
		where += " AND report_period = ?"
		args = append(args, filter.Period)
	}
	if filter.Holder != "" {
		like := "%" + filter.Holder + "%"
		where += " AND holder_name LIKE ?"
		args = append(args, like)
	}
	if filter.HolderType != "" && filter.HolderType != "all" {
		where += " AND holder_type = ?"
		args = append(args, filter.HolderType)
	}
	if filter.Source != "" && filter.Source != "all" {
		where += " AND source_type = ?"
		args = append(args, filter.Source)
	}
	return where, args
}

func (s *Store) listStockHoldingDistinct(ctx context.Context, column string) ([]string, error) {
	switch column {
	case "report_period", "source_type", "holder_type":
	default:
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT `+column+` FROM stock_institution_holdings WHERE `+column+` <> '' ORDER BY `+column+` DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func scanStockInstitutionHolding(scanner scanner) (model.StockInstitutionHolding, error) {
	var item model.StockInstitutionHolding
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.ID,
		&item.StockCode,
		&item.StockName,
		&item.ReportPeriod,
		&item.AnnounceDate,
		&item.HolderName,
		&item.HolderType,
		&item.HolderCode,
		&item.HolderRank,
		&item.Shares,
		&item.SharesChange,
		&item.ChangeRatio,
		&item.FloatRatio,
		&item.MarketValue,
		&item.SourceType,
		&item.SourceURL,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.StockInstitutionHolding{}, err
	}
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}
