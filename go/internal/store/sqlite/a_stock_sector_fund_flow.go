package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
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
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	source_count = excluded.source_count,
	source_types = excluded.source_types,
	field_counts_json = excluded.field_counts_json,
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
			defaultPositiveInt(item.SourceCount, 1),
			nonEmpty(strings.TrimSpace(item.SourceTypes), nonEmpty(strings.TrimSpace(item.SourceType), "akshare_sector_fund_flow")),
			nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}"),
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
		SourceType: strings.TrimSpace(filter.SourceType),
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
	if result.SourceType != "" && result.SourceType != "all" && result.SourceType != "average" && result.SourceType != "aggregate" {
		return s.listAStockSectorFundFlowSourceRows(ctx, result, filter)
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
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
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

func (s *Store) SnapshotAStockSectorFundFlowIntraday(ctx context.Context, tradeDate string, captureTime string, sectorType string, indicator string) (model.AStockSectorFundFlowUpsertResult, error) {
	tradeDate = strings.TrimSpace(tradeDate)
	sectorType = normalizeAStockSectorFundFlowSectorType(sectorType)
	indicator = normalizeAStockSectorFundFlowIndicator(indicator)
	captureTime = normalizeAStockSectorFundFlowCaptureTime(captureTime)
	result := model.AStockSectorFundFlowUpsertResult{Date: tradeDate}
	if tradeDate == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(trade_date), '') FROM a_stock_sector_fund_flows WHERE sector_type = ? AND indicator = ?`, sectorType, indicator).Scan(&tradeDate); err != nil {
			return result, err
		}
		result.Date = tradeDate
	}
	if tradeDate == "" {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_sector_fund_flows
WHERE trade_date = ? AND sector_type = ? AND indicator = ?
ORDER BY rank ASC, main_net_inflow DESC, name ASC`, tradeDate, sectorType, indicator)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockSectorFundFlow, 0, 128)
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
	result.Total = len(items)
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
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_sector_fund_flow_intraday_snapshots (
	trade_date, capture_time, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, capture_time, sector_type, indicator, name) DO UPDATE SET
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
	source_count = excluded.source_count,
	source_types = excluded.source_types,
	field_counts_json = excluded.field_counts_json,
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
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		existed := false
		if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_sector_fund_flow_intraday_snapshots WHERE trade_date = ? AND capture_time = ? AND sector_type = ? AND indicator = ? AND name = ?`, tradeDate, captureTime, sectorType, indicator, name).Scan(new(int)); scanErr == nil {
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
			tradeDate,
			captureTime,
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
			defaultPositiveInt(item.SourceCount, 1),
			nonEmpty(strings.TrimSpace(item.SourceTypes), nonEmpty(strings.TrimSpace(item.SourceType), "average")),
			nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}"),
			nonEmpty(strings.TrimSpace(item.SourceType), "average"),
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

func (s *Store) ListAStockSectorFundFlowIntraday(ctx context.Context, filter model.AStockSectorFundFlowIntradayFilter) (model.AStockSectorFundFlowIntradayResult, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	result := model.AStockSectorFundFlowIntradayResult{
		Date:       strings.TrimSpace(filter.Date),
		SectorType: normalizeAStockSectorFundFlowSectorType(filter.SectorType),
		Indicator:  normalizeAStockSectorFundFlowIndicator(filter.Indicator),
		Times:      []string{},
		Series:     []model.AStockSectorFundFlowIntradaySeries{},
		Top:        []model.AStockSectorFundFlow{},
	}
	if result.Date == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(trade_date), '') FROM a_stock_sector_fund_flow_intraday_snapshots WHERE sector_type = ? AND indicator = ?`, result.SectorType, result.Indicator).Scan(&result.Date); err != nil {
			return result, err
		}
	}
	if result.Date == "" {
		return result, nil
	}
	timeRows, err := s.db.QueryContext(ctx, `SELECT DISTINCT capture_time FROM a_stock_sector_fund_flow_intraday_snapshots WHERE trade_date = ? AND sector_type = ? AND indicator = ? ORDER BY capture_time ASC`, result.Date, result.SectorType, result.Indicator)
	if err != nil {
		return result, err
	}
	for timeRows.Next() {
		var captureTime string
		if scanErr := timeRows.Scan(&captureTime); scanErr != nil {
			_ = timeRows.Close()
			return result, scanErr
		}
		result.Times = append(result.Times, captureTime)
	}
	if err := timeRows.Close(); err != nil {
		return result, err
	}
	if len(result.Times) == 0 {
		return result, nil
	}
	result.LatestTime = result.Times[len(result.Times)-1]

	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_sector_fund_flow_intraday_snapshots
WHERE trade_date = ? AND capture_time = ? AND sector_type = ? AND indicator = ?
ORDER BY main_net_inflow DESC, rank ASC, name ASC`, result.Date, result.LatestTime, result.SectorType, result.Indicator)
	if err != nil {
		return result, err
	}
	latestRows := make([]model.AStockSectorFundFlow, 0, 128)
	for rows.Next() {
		item, scanErr := scanAStockSectorFundFlow(rows)
		if scanErr != nil {
			_ = rows.Close()
			return result, scanErr
		}
		latestRows = append(latestRows, item)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	result.Total = len(latestRows)
	topLimit := min(limit, len(latestRows))
	if topLimit > 0 {
		result.Top = append(result.Top, latestRows[:topLimit]...)
	}

	selectedNames := selectAStockSectorFundFlowIntradaySeriesNames(latestRows, limit)
	if len(selectedNames) == 0 {
		return result, nil
	}
	placeholders := make([]string, 0, len(selectedNames))
	args := []any{result.Date, result.SectorType, result.Indicator}
	for _, name := range selectedNames {
		placeholders = append(placeholders, "?")
		args = append(args, name)
	}
	query := `
SELECT capture_time, name, rank, main_net_inflow
FROM a_stock_sector_fund_flow_intraday_snapshots
WHERE trade_date = ? AND sector_type = ? AND indicator = ? AND name IN (` + strings.Join(placeholders, ",") + `)
ORDER BY capture_time ASC, name ASC`
	pointRows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	pointsByName := map[string][]model.AStockSectorFundFlowIntradayPoint{}
	for pointRows.Next() {
		var point model.AStockSectorFundFlowIntradayPoint
		var name string
		if scanErr := pointRows.Scan(&point.Time, &name, &point.Rank, &point.MainNetInflow); scanErr != nil {
			_ = pointRows.Close()
			return result, scanErr
		}
		pointsByName[name] = append(pointsByName[name], point)
	}
	if err := pointRows.Close(); err != nil {
		return result, err
	}
	latestByName := map[string]model.AStockSectorFundFlow{}
	for _, item := range latestRows {
		latestByName[item.Name] = item
	}
	for _, name := range selectedNames {
		item := latestByName[name]
		result.Series = append(result.Series, model.AStockSectorFundFlowIntradaySeries{
			Name:                name,
			LatestRank:          item.Rank,
			LatestMainNetInflow: item.MainNetInflow,
			Points:              pointsByName[name],
		})
	}
	return result, nil
}

func normalizeAStockSectorFundFlowCaptureTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().In(aStockSectorFundFlowStoreLocation()).Format("15:04")
	}
	if len(value) == 4 && !strings.Contains(value, ":") {
		return value[:2] + ":" + value[2:]
	}
	return value
}

func aStockSectorFundFlowStoreLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*3600)
	}
	return loc
}

func selectAStockSectorFundFlowIntradaySeriesNames(items []model.AStockSectorFundFlow, limit int) []string {
	if limit <= 0 {
		limit = 20
	}
	topCount := limit / 2
	bottomCount := limit - topCount
	names := make([]string, 0, limit)
	seen := map[string]struct{}{}
	addName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for i := 0; i < len(items) && i < topCount; i++ {
		addName(items[i].Name)
	}
	bottom := append([]model.AStockSectorFundFlow(nil), items...)
	sort.SliceStable(bottom, func(i, j int) bool {
		if bottom[i].MainNetInflow != bottom[j].MainNetInflow {
			return bottom[i].MainNetInflow < bottom[j].MainNetInflow
		}
		if bottom[i].Rank != bottom[j].Rank {
			return bottom[i].Rank > bottom[j].Rank
		}
		return bottom[i].Name < bottom[j].Name
	})
	for i := 0; i < len(bottom) && i < bottomCount; i++ {
		addName(bottom[i].Name)
	}
	return names
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
		&item.SourceCount,
		&item.SourceTypes,
		&item.FieldCountsJSON,
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

func (s *Store) UpsertAStockSectorFundFlowSourceRows(ctx context.Context, tradeDate string, items []model.AStockSectorFundFlow, replace bool) (model.AStockSectorFundFlowUpsertResult, error) {
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

	type groupKey struct {
		date       string
		sectorType string
		indicator  string
	}
	recomputeKeys := map[groupKey]struct{}{}
	if replace {
		replaceKeys := map[[4]string]struct{}{}
		for _, item := range items {
			date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
			sectorType := strings.TrimSpace(item.SectorType)
			indicator := strings.TrimSpace(item.Indicator)
			sourceType := strings.TrimSpace(item.SourceType)
			if date == "" || sectorType == "" || indicator == "" || sourceType == "" {
				continue
			}
			replaceKeys[[4]string{date, sectorType, indicator, sourceType}] = struct{}{}
			recomputeKeys[groupKey{date: date, sectorType: sectorType, indicator: indicator}] = struct{}{}
		}
		for key := range replaceKeys {
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_sector_fund_flow_source_rows WHERE trade_date = ? AND sector_type = ? AND indicator = ? AND source_type = ?`, key[0], key[1], key[2], key[3]); err != nil {
				return result, err
			}
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_sector_fund_flow_source_rows (
	trade_date, sector_type, indicator, source_type, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, field_counts_json, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, sector_type, indicator, source_type, name) DO UPDATE SET
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
	field_counts_json = excluded.field_counts_json,
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
		sourceType := strings.TrimSpace(item.SourceType)
		name := strings.TrimSpace(item.Name)
		if date == "" || sectorType == "" || indicator == "" || sourceType == "" || name == "" {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_sector_fund_flow_source_rows WHERE trade_date = ? AND sector_type = ? AND indicator = ? AND source_type = ? AND name = ?`, date, sectorType, indicator, sourceType, name).Scan(new(int)); scanErr == nil {
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
			sourceType,
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
			nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}"),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
			fetchedAt.UTC().Format(time.RFC3339),
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return result, err
		}
		recomputeKeys[groupKey{date: date, sectorType: sectorType, indicator: indicator}] = struct{}{}
		if existed {
			result.Updated++
		} else {
			result.Inserted++
		}
	}
	for key := range recomputeKeys {
		if err = s.recomputeAStockSectorFundFlowAverageTx(ctx, tx, key.date, key.sectorType, key.indicator); err != nil {
			return result, err
		}
	}
	err = tx.Commit()
	return result, err
}

func (s *Store) listAStockSectorFundFlowSourceRows(ctx context.Context, result model.AStockSectorFundFlowListResult, filter model.AStockSectorFundFlowFilter) (model.AStockSectorFundFlowListResult, error) {
	where := "WHERE trade_date = ? AND sector_type = ? AND indicator = ? AND source_type = ?"
	args := []any{result.Date, result.SectorType, result.Indicator, result.SourceType}
	if result.Keyword != "" {
		like := "%" + result.Keyword + "%"
		where += " AND (name LIKE ? OR top_stock LIKE ?)"
		args = append(args, like, like)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_sector_fund_flow_source_rows `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM a_stock_sector_fund_flow_source_rows `+where, args...).Scan(&fetchedAt); err != nil {
		return result, err
	}
	if fetchedAt.Valid && strings.TrimSpace(fetchedAt.String) != "" {
		value := mustParseRFC3339(fetchedAt.String)
		result.FetchedAt = &value
	}
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, sector_type, indicator, source_type, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, field_counts_json, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_sector_fund_flow_source_rows `+where+`
ORDER BY rank ASC, main_net_inflow DESC, name ASC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockSectorFundFlow, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanAStockSectorFundFlowSourceRow(rows)
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

func scanAStockSectorFundFlowSourceRow(scanner scanner) (model.AStockSectorFundFlow, error) {
	var item model.AStockSectorFundFlow
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.SectorType,
		&item.Indicator,
		&item.SourceType,
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
		&item.FieldCountsJSON,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AStockSectorFundFlow{}, err
	}
	item.SourceCount = 1
	item.SourceTypes = item.SourceType
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

type aStockFundFlowAverageValue struct {
	sum   float64
	count int
}

type aStockSectorFundFlowAverage struct {
	item      model.AStockSectorFundFlow
	fields    map[string]aStockFundFlowAverageValue
	counts    map[string]int
	sourceSet map[string]struct{}
	topStocks map[string]aStockSectorFundFlowTopStock
}

type aStockSectorFundFlowTopStock struct {
	name string
	net  float64
}

func (s *Store) recomputeAStockSectorFundFlowAverageTx(ctx context.Context, tx *Tx, tradeDate string, sectorType string, indicator string) error {
	rows, err := tx.QueryContext(ctx, `
SELECT trade_date, sector_type, indicator, source_type, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, field_counts_json, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_sector_fund_flow_source_rows
WHERE trade_date = ? AND sector_type = ? AND indicator = ?`, tradeDate, sectorType, indicator)
	if err != nil {
		return err
	}
	defer rows.Close()
	averages := map[string]*aStockSectorFundFlowAverage{}
	for rows.Next() {
		source, scanErr := scanAStockSectorFundFlowSourceRow(rows)
		if scanErr != nil {
			return scanErr
		}
		name := strings.TrimSpace(source.Name)
		if name == "" {
			continue
		}
		avg := averages[name]
		if avg == nil {
			avg = &aStockSectorFundFlowAverage{
				item: model.AStockSectorFundFlow{
					TradeDate:  tradeDate,
					SectorType: sectorType,
					Indicator:  indicator,
					Name:       name,
					SourceType: "average",
					RawPayload: "{}",
					FetchedAt:  source.FetchedAt,
				},
				fields:    map[string]aStockFundFlowAverageValue{},
				counts:    map[string]int{},
				sourceSet: map[string]struct{}{},
				topStocks: map[string]aStockSectorFundFlowTopStock{},
			}
			averages[name] = avg
		}
		if source.FetchedAt.After(avg.item.FetchedAt) {
			avg.item.FetchedAt = source.FetchedAt
		}
		if source.SourceType != "" {
			avg.sourceSet[source.SourceType] = struct{}{}
		}
		counts := parseAStockFundFlowFieldCounts(source.FieldCountsJSON)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "change_pct", source.ChangePct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "main_net_inflow", source.MainNetInflow)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "main_net_inflow_pct", source.MainNetInflowPct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "super_large_net_inflow", source.SuperLargeNetInflow)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "super_large_net_inflow_pct", source.SuperLargeNetInflowPct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "large_net_inflow", source.LargeNetInflow)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "large_net_inflow_pct", source.LargeNetInflowPct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "medium_net_inflow", source.MediumNetInflow)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "medium_net_inflow_pct", source.MediumNetInflowPct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "small_net_inflow", source.SmallNetInflow)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "small_net_inflow_pct", source.SmallNetInflowPct)
		addAStockSectorFundFlowTopStocks(avg, source.TopStock, source.MainNetInflow)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	items := make([]model.AStockSectorFundFlow, 0, len(averages))
	for _, avg := range averages {
		applyAStockSectorFundFlowAverages(avg)
		items = append(items, avg.item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].MainNetInflow == items[j].MainNetInflow {
			return items[i].Name < items[j].Name
		}
		return items[i].MainNetInflow > items[j].MainNetInflow
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM a_stock_sector_fund_flows WHERE trade_date = ? AND sector_type = ? AND indicator = ?`, tradeDate, sectorType, indicator); err != nil {
		return err
	}
	return insertAStockSectorFundFlowAggregateRowsTx(ctx, tx, items)
}

func insertAStockSectorFundFlowAggregateRowsTx(ctx context.Context, tx *Tx, items []model.AStockSectorFundFlow) error {
	if len(items) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_sector_fund_flows (
	trade_date, sector_type, indicator, rank, name, change_pct,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, top_stock, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	source_count = excluded.source_count,
	source_types = excluded.source_types,
	field_counts_json = excluded.field_counts_json,
	source_type = excluded.source_type,
	raw_payload = excluded.raw_payload,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC()
	for _, item := range items {
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
		if _, err := stmt.ExecContext(ctx,
			item.TradeDate,
			item.SectorType,
			item.Indicator,
			item.Rank,
			item.Name,
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
			defaultPositiveInt(item.SourceCount, 1),
			strings.TrimSpace(item.SourceTypes),
			nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}"),
			nonEmpty(strings.TrimSpace(item.SourceType), "average"),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
			fetchedAt.UTC().Format(time.RFC3339),
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return err
		}
	}
	return nil
}

func addAStockFundFlowAverage(values map[string]aStockFundFlowAverageValue, counts map[string]int, sourceCounts map[string]int, field string, value float64) {
	if !aStockFundFlowFieldAvailable(sourceCounts, field) {
		return
	}
	current := values[field]
	current.sum += value
	current.count++
	values[field] = current
	counts[field]++
}

func aStockFundFlowFieldAvailable(counts map[string]int, field string) bool {
	if len(counts) == 0 {
		return true
	}
	return counts[field] > 0
}

func addAStockSectorFundFlowTopStocks(avg *aStockSectorFundFlowAverage, value string, net float64) {
	if avg == nil {
		return
	}
	if avg.topStocks == nil {
		avg.topStocks = map[string]aStockSectorFundFlowTopStock{}
	}
	for _, name := range splitAStockSectorFundFlowTopStocks(value) {
		current, ok := avg.topStocks[name]
		if !ok || net > current.net {
			avg.topStocks[name] = aStockSectorFundFlowTopStock{name: name, net: net}
		}
	}
}

func splitAStockSectorFundFlowTopStocks(value string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, 3)
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '、', ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func formatAStockSectorFundFlowTopStocks(stocks map[string]aStockSectorFundFlowTopStock, limit int) string {
	if len(stocks) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = 3
	}
	items := make([]aStockSectorFundFlowTopStock, 0, len(stocks))
	for _, stock := range stocks {
		items = append(items, stock)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].net == items[j].net {
			return items[i].name < items[j].name
		}
		return items[i].net > items[j].net
	})
	if len(items) > limit {
		items = items[:limit]
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.name)
	}
	return strings.Join(names, "、")
}

func applyAStockSectorFundFlowAverages(avg *aStockSectorFundFlowAverage) {
	avg.item.ChangePct = aStockFundFlowAverage(avg.fields["change_pct"])
	avg.item.MainNetInflow = aStockFundFlowAverage(avg.fields["main_net_inflow"])
	avg.item.MainNetInflowPct = aStockFundFlowAverage(avg.fields["main_net_inflow_pct"])
	avg.item.SuperLargeNetInflow = aStockFundFlowAverage(avg.fields["super_large_net_inflow"])
	avg.item.SuperLargeNetInflowPct = aStockFundFlowAverage(avg.fields["super_large_net_inflow_pct"])
	avg.item.LargeNetInflow = aStockFundFlowAverage(avg.fields["large_net_inflow"])
	avg.item.LargeNetInflowPct = aStockFundFlowAverage(avg.fields["large_net_inflow_pct"])
	avg.item.MediumNetInflow = aStockFundFlowAverage(avg.fields["medium_net_inflow"])
	avg.item.MediumNetInflowPct = aStockFundFlowAverage(avg.fields["medium_net_inflow_pct"])
	avg.item.SmallNetInflow = aStockFundFlowAverage(avg.fields["small_net_inflow"])
	avg.item.SmallNetInflowPct = aStockFundFlowAverage(avg.fields["small_net_inflow_pct"])
	avg.item.SourceCount = avg.counts["main_net_inflow"]
	avg.item.SourceTypes = joinAStockFundFlowSourceTypes(avg.sourceSet)
	avg.item.FieldCountsJSON = marshalAStockFundFlowFieldCounts(avg.counts)
	avg.item.TopStock = formatAStockSectorFundFlowTopStocks(avg.topStocks, 3)
	if avg.item.RawPayload == "" || avg.item.RawPayload == "{}" {
		avg.item.RawPayload = `{"aggregation":"average"}`
	}
}

func aStockFundFlowAverage(value aStockFundFlowAverageValue) float64 {
	if value.count <= 0 {
		return 0
	}
	return value.sum / float64(value.count)
}

func parseAStockFundFlowFieldCounts(raw string) map[string]int {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return map[string]int{}
	}
	out := map[string]int{}
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	floatCounts := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &floatCounts); err != nil {
		return map[string]int{}
	}
	for key, value := range floatCounts {
		if value > 0 {
			out[key] = int(value)
		}
	}
	return out
}

func marshalAStockFundFlowFieldCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "{}"
	}
	body, err := json.Marshal(counts)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func joinAStockFundFlowSourceTypes(sourceSet map[string]struct{}) string {
	if len(sourceSet) == 0 {
		return ""
	}
	values := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		source = strings.TrimSpace(source)
		if source != "" {
			values = append(values, source)
		}
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func defaultPositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
