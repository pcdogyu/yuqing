package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockSectorFundFlows(ctx context.Context, tradeDate string, items []model.AStockSectorFundFlow, replace bool) (model.AStockSectorFundFlowUpsertResult, error) {
	tradeDate = strings.TrimSpace(tradeDate)
	result := model.AStockSectorFundFlowUpsertResult{Date: tradeDate, Total: len(items)}
	if tradeDate == "" {
		for _, item := range items {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				tradeDate = date
				result.Date = date
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

	if replace {
		replaceKeys := map[[3]string]struct{}{}
		for _, item := range items {
			date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
			sectorType := strings.TrimSpace(item.SectorType)
			indicator := strings.TrimSpace(item.Indicator)
			if date == "" || sectorType == "" || indicator == "" {
				continue
			}
			replaceKeys[[3]string{date, sectorType, indicator}] = struct{}{}
		}
		for key := range replaceKeys {
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_sector_fund_flows WHERE trade_date = ? AND sector_type = ? AND indicator = ?`, key[0], key[1], key[2]); err != nil {
				return result, err
			}
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_sector_fund_flows (
	trade_date, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_type, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, sector_type, indicator, name) DO UPDATE SET
	rank = excluded.rank,
	change_pct = excluded.change_pct,
	main_net_inflow = excluded.main_net_inflow,
	main_net_inflow_pct = excluded.main_net_inflow_pct,
	super_large_net_inflow = excluded.super_large_net_inflow,
	super_large_net_inflow_pct = excluded.super_large_net_inflow_pct,
	large_net_inflow = excluded.large_net_inflow,
	large_net_inflow_pct = excluded.large_net_inflow_pct,
	medium_net_inflow = excluded.medium_net_inflow,
	medium_net_inflow_pct = excluded.medium_net_inflow_pct,
	small_net_inflow = excluded.small_net_inflow,
	small_net_inflow_pct = excluded.small_net_inflow_pct,
	top_stock = excluded.top_stock,
	source_type = excluded.source_type,
	raw_payload = excluded.raw_payload,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, item := range items {
		date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
		sectorType := strings.TrimSpace(item.SectorType)
		indicator := strings.TrimSpace(item.Indicator)
		name := strings.TrimSpace(item.Name)
		if date == "" || sectorType == "" || indicator == "" || name == "" {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_sector_fund_flows WHERE trade_date = ? AND sector_type = ? AND indicator = ? AND name = ?`, date, sectorType, indicator, name).Scan(new(int)); scanErr == nil {
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
			date,
			sectorType,
			indicator,
			item.Rank,
			name,
			item.ChangePct,
			item.MainNetInflow,
			item.MainNetInflowPct,
			item.SuperLargeNetInflow,
			item.SuperLargeNetInflowPct,
			item.LargeNetInflow,
			item.LargeNetInflowPct,
			item.MediumNetInflow,
			item.MediumNetInflowPct,
			item.SmallNetInflow,
			item.SmallNetInflowPct,
			strings.TrimSpace(item.TopStock),
			nonEmpty(strings.TrimSpace(item.SourceType), "akshare_sector_fund_flow"),
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

func (s *Store) ListAStockSectorFundFlows(ctx context.Context, filter model.AStockSectorFundFlowFilter) (model.AStockSectorFundFlowListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 500 {
		filter.PageSize = 500
	}
	result := model.AStockSectorFundFlowListResult{
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		Date:       strings.TrimSpace(filter.Date),
		SectorType: normalizeAStockSectorFundFlowSectorType(filter.SectorType),
		Indicator:  normalizeAStockSectorFundFlowIndicator(filter.Indicator),
		Keyword:    strings.TrimSpace(filter.Keyword),
	}
	dates, err := s.listAStockSectorFundFlowDistinct(ctx, "trade_date")
	if err != nil {
		return result, err
	}
	result.Dates = dates
	if len(dates) > 0 {
		result.LatestDate = dates[0]
	}
	if result.Date == "" {
		result.Date = result.LatestDate
	}
	sectorTypes, err := s.listAStockSectorFundFlowDistinct(ctx, "sector_type")
	if err != nil {
		return result, err
	}
	result.SectorTypes = mergeKnownAStockSectorOptions([]string{"行业资金流", "概念资金流"}, sectorTypes)
	indicators, err := s.listAStockSectorFundFlowDistinct(ctx, "indicator")
	if err != nil {
		return result, err
	}
	result.Indicators = mergeKnownAStockSectorOptions([]string{"今日", "5日", "10日"}, indicators)
	if result.Date == "" {
		result.Items = []model.AStockSectorFundFlow{}
		return result, nil
	}

	where := "WHERE trade_date = ? AND sector_type = ? AND indicator = ?"
	args := []any{result.Date, result.SectorType, result.Indicator}
	if result.Keyword != "" {
		like := "%" + result.Keyword + "%"
		where += " AND (name LIKE ? OR top_stock LIKE ?)"
		args = append(args, like, like)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_sector_fund_flows `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM a_stock_sector_fund_flows `+where, args...).Scan(&fetchedAt); err != nil {
		return result, err
	}
	if fetchedAt.Valid && strings.TrimSpace(fetchedAt.String) != "" {
		value := mustParseRFC3339(fetchedAt.String)
		result.FetchedAt = &value
	}

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_type, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_sector_fund_flows `+where+`
ORDER BY rank ASC, main_net_inflow DESC, name ASC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockSectorFundFlow, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanAStockSectorFundFlow(rows)
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

func normalizeAStockSectorFundFlowSectorType(value string) string {
	switch strings.TrimSpace(value) {
	case "概念", "概念资金", "概念资金流":
		return "概念资金流"
	default:
		return "行业资金流"
	}
}

func normalizeAStockSectorFundFlowIndicator(value string) string {
	switch strings.TrimSpace(value) {
	case "5", "5日":
		return "5日"
	case "10", "10日":
		return "10日"
	default:
		return "今日"
	}
}

func mergeKnownAStockSectorOptions(known []string, stored []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(known)+len(stored))
	for _, value := range append(known, stored...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *Store) listAStockSectorFundFlowDistinct(ctx context.Context, column string) ([]string, error) {
	switch column {
	case "trade_date", "sector_type", "indicator":
	default:
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT `+column+` FROM a_stock_sector_fund_flows WHERE `+column+` <> '' ORDER BY `+column+` DESC`)
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

func scanAStockSectorFundFlow(scanner scanner) (model.AStockSectorFundFlow, error) {
	var item model.AStockSectorFundFlow
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.SectorType,
		&item.Indicator,
		&item.Rank,
		&item.Name,
		&item.ChangePct,
		&item.MainNetInflow,
		&item.MainNetInflowPct,
		&item.SuperLargeNetInflow,
		&item.SuperLargeNetInflowPct,
		&item.LargeNetInflow,
		&item.LargeNetInflowPct,
		&item.MediumNetInflow,
		&item.MediumNetInflowPct,
		&item.SmallNetInflow,
		&item.SmallNetInflowPct,
		&item.TopStock,
		&item.SourceType,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AStockSectorFundFlow{}, err
	}
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}
