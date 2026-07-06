package sqlite

import (
	"context"
	"strings"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) ListAStockSectorFundFlowTrend(ctx context.Context, filter model.AStockFundFlowTrendFilter) (model.AStockSectorFundFlowTrendResult, error) {
	days := normalizeAStockFundFlowTrendDays(filter.Days)
	result := model.AStockSectorFundFlowTrendResult{
		EndDate:    strings.TrimSpace(filter.EndDate),
		SectorType: normalizeAStockSectorFundFlowSectorType(filter.SectorType),
		SectorName: strings.TrimSpace(filter.SectorName),
		Indicator:  normalizeAStockSectorFundFlowIndicator(filter.Indicator),
		Days:       days,
	}
	if result.SectorName == "" {
		result.Items = []model.AStockSectorFundFlow{}
		return result, nil
	}
	where := "WHERE sector_type = ? AND name = ? AND indicator = ?"
	args := []any{result.SectorType, result.SectorName, result.Indicator}
	if result.EndDate != "" {
		where += " AND trade_date <= ?"
		args = append(args, result.EndDate)
	}
	queryArgs := append(args, days)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_sector_fund_flows `+where+`
ORDER BY trade_date DESC
LIMIT ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockSectorFundFlow, 0, days)
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
	result.Total = len(items)
	if result.EndDate == "" && len(items) > 0 {
		result.EndDate = items[0].TradeDate
	}
	return result, nil
}

func (s *Store) ListAStockStockFundFlowTrend(ctx context.Context, filter model.AStockFundFlowTrendFilter) (model.AStockStockFundFlowTrendResult, error) {
	days := normalizeAStockFundFlowTrendDays(filter.Days)
	code := normalizeAStockFundFlowCode(filter.Code)
	keyword := strings.TrimSpace(filter.Keyword)
	result := model.AStockStockFundFlowTrendResult{
		EndDate:   strings.TrimSpace(filter.EndDate),
		Indicator: normalizeAStockSectorFundFlowIndicator(filter.Indicator),
		Code:      code,
		Keyword:   keyword,
		Days:      days,
	}
	where := "WHERE indicator = ?"
	args := []any{result.Indicator}
	if result.EndDate != "" {
		where += " AND trade_date <= ?"
		args = append(args, result.EndDate)
	}
	if code != "" {
		where += " AND code = ?"
		args = append(args, code)
	} else if keyword != "" {
		normalizedKeyword := astockcode.Normalize(keyword)
		if normalizedKeyword != "" {
			where += " AND code = ?"
			args = append(args, normalizedKeyword)
			result.Code = normalizedKeyword
		} else {
			like := "%" + keyword + "%"
			where += " AND (code LIKE ? OR name LIKE ?)"
			args = append(args, like, like)
		}
	} else {
		result.Items = []model.AStockStockFundFlow{}
		return result, nil
	}
	queryArgs := append(args, days)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, indicator, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_stock_fund_flows `+where+`
ORDER BY trade_date DESC, rank ASC
LIMIT ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockStockFundFlow, 0, days)
	for rows.Next() {
		item, scanErr := scanAStockStockFundFlow(rows)
		if scanErr != nil {
			return result, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Items = items
	result.Total = len(items)
	if result.EndDate == "" && len(items) > 0 {
		result.EndDate = items[0].TradeDate
	}
	if result.Code == "" && len(items) > 0 {
		result.Code = items[0].Code
	}
	return result, nil
}

func normalizeAStockFundFlowTrendDays(days int) int {
	switch days {
	case 10, 30:
		return days
	default:
		return 5
	}
}
