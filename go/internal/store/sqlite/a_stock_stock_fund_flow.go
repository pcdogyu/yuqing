package sqlite

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockStockFundFlows(ctx context.Context, tradeDate string, items []model.AStockStockFundFlow, replace bool) (model.AStockStockFundFlowUpsertResult, error) {
	tradeDate = strings.TrimSpace(tradeDate)
	result := model.AStockStockFundFlowUpsertResult{Date: tradeDate, Total: len(items)}
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
		replaceKeys := map[[2]string]struct{}{}
		for _, item := range items {
			date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
			indicator := normalizeAStockSectorFundFlowIndicator(item.Indicator)
			if date == "" || indicator == "" {
				continue
			}
			replaceKeys[[2]string{date, indicator}] = struct{}{}
		}
		for key := range replaceKeys {
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_stock_fund_flows WHERE trade_date = ? AND indicator = ?`, key[0], key[1]); err != nil {
				return result, err
			}
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_stock_fund_flows (
	trade_date, indicator, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, indicator, code) DO UPDATE SET
	rank = excluded.rank,
	name = excluded.name,
	price = excluded.price,
	change_pct = excluded.change_pct,
	turnover_pct = excluded.turnover_pct,
	amount = excluded.amount,
	in_amount = excluded.in_amount,
	out_amount = excluded.out_amount,
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
		indicator := normalizeAStockSectorFundFlowIndicator(item.Indicator)
		code := normalizeAStockFundFlowCode(item.Code)
		if date == "" || indicator == "" || code == "" {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_stock_fund_flows WHERE trade_date = ? AND indicator = ? AND code = ?`, date, indicator, code).Scan(new(int)); scanErr == nil {
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
			indicator,
			code,
			item.Rank,
			strings.TrimSpace(item.Name),
			item.Price,
			item.ChangePct,
			item.TurnoverPct,
			item.Amount,
			item.InAmount,
			item.OutAmount,
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

func (s *Store) UpsertAStockStockFundFlowSourceRows(ctx context.Context, tradeDate string, items []model.AStockStockFundFlow, replace bool) (model.AStockStockFundFlowUpsertResult, error) {
	tradeDate = strings.TrimSpace(tradeDate)
	result := model.AStockStockFundFlowUpsertResult{Date: tradeDate, Total: len(items)}
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
		date      string
		indicator string
	}
	recomputeKeys := map[groupKey]struct{}{}
	if replace {
		replaceKeys := map[[3]string]struct{}{}
		for _, item := range items {
			date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
			indicator := normalizeAStockSectorFundFlowIndicator(item.Indicator)
			sourceType := strings.TrimSpace(item.SourceType)
			if date == "" || indicator == "" || sourceType == "" {
				continue
			}
			replaceKeys[[3]string{date, indicator, sourceType}] = struct{}{}
			recomputeKeys[groupKey{date: date, indicator: indicator}] = struct{}{}
		}
		for key := range replaceKeys {
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_stock_fund_flow_source_rows WHERE trade_date = ? AND indicator = ? AND source_type = ?`, key[0], key[1], key[2]); err != nil {
				return result, err
			}
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_stock_fund_flow_source_rows (
	trade_date, indicator, source_type, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, field_counts_json, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, indicator, source_type, code) DO UPDATE SET
	rank = excluded.rank,
	name = excluded.name,
	price = excluded.price,
	change_pct = excluded.change_pct,
	turnover_pct = excluded.turnover_pct,
	amount = excluded.amount,
	in_amount = excluded.in_amount,
	out_amount = excluded.out_amount,
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
		indicator := normalizeAStockSectorFundFlowIndicator(item.Indicator)
		sourceType := strings.TrimSpace(item.SourceType)
		code := normalizeAStockFundFlowCode(item.Code)
		if date == "" || indicator == "" || sourceType == "" || code == "" {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_stock_fund_flow_source_rows WHERE trade_date = ? AND indicator = ? AND source_type = ? AND code = ?`, date, indicator, sourceType, code).Scan(new(int)); scanErr == nil {
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
			indicator,
			sourceType,
			code,
			item.Rank,
			strings.TrimSpace(item.Name),
			item.Price,
			item.ChangePct,
			item.TurnoverPct,
			item.Amount,
			item.InAmount,
			item.OutAmount,
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
			nonEmpty(strings.TrimSpace(item.FieldCountsJSON), "{}"),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
			fetchedAt.UTC().Format(time.RFC3339),
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return result, err
		}
		recomputeKeys[groupKey{date: date, indicator: indicator}] = struct{}{}
		if existed {
			result.Updated++
		} else {
			result.Inserted++
		}
	}
	for key := range recomputeKeys {
		if err = s.recomputeAStockStockFundFlowAverageTx(ctx, tx, key.date, key.indicator); err != nil {
			return result, err
		}
	}
	err = tx.Commit()
	return result, err
}

func (s *Store) ListAStockStockFundFlows(ctx context.Context, filter model.AStockStockFundFlowFilter) (model.AStockStockFundFlowListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 500 {
		filter.PageSize = 500
	}
	result := model.AStockStockFundFlowListResult{
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		Date:       strings.TrimSpace(filter.Date),
		Indicator:  normalizeAStockSectorFundFlowIndicator(filter.Indicator),
		Keyword:    strings.TrimSpace(filter.Keyword),
		SourceType: strings.TrimSpace(filter.SourceType),
	}
	codes := normalizeAStockFundFlowCodes(filter.Codes)
	dates, err := s.listAStockStockFundFlowDistinct(ctx, "trade_date")
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
	indicators, err := s.listAStockStockFundFlowDistinct(ctx, "indicator")
	if err != nil {
		return result, err
	}
	result.Indicators = mergeKnownAStockSectorOptions([]string{"今日", "5日", "10日"}, indicators)
	if result.Date == "" {
		result.Items = []model.AStockStockFundFlow{}
		return result, nil
	}
	if result.SourceType != "" && result.SourceType != "all" && result.SourceType != "average" && result.SourceType != "aggregate" {
		return s.listAStockStockFundFlowSourceRows(ctx, result, filter)
	}

	where := "WHERE trade_date = ? AND indicator = ?"
	args := []any{result.Date, result.Indicator}
	if result.Keyword != "" {
		like := "%" + result.Keyword + "%"
		where += " AND (code LIKE ? OR name LIKE ?)"
		args = append(args, like, like)
	}
	if len(codes) > 0 {
		where += " AND code IN (" + questionPlaceholders(len(codes)) + ")"
		for _, code := range codes {
			args = append(args, code)
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_stock_fund_flows `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM a_stock_stock_fund_flows `+where, args...).Scan(&fetchedAt); err != nil {
		return result, err
	}
	if fetchedAt.Valid && strings.TrimSpace(fetchedAt.String) != "" {
		value := mustParseRFC3339(fetchedAt.String)
		result.FetchedAt = &value
	}
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, indicator, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_stock_fund_flows `+where+`
ORDER BY rank ASC, main_net_inflow DESC, code ASC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockStockFundFlow, 0, filter.PageSize)
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
	return result, nil
}

func (s *Store) listAStockStockFundFlowSourceRows(ctx context.Context, result model.AStockStockFundFlowListResult, filter model.AStockStockFundFlowFilter) (model.AStockStockFundFlowListResult, error) {
	where := "WHERE trade_date = ? AND indicator = ? AND source_type = ?"
	args := []any{result.Date, result.Indicator, result.SourceType}
	codes := normalizeAStockFundFlowCodes(filter.Codes)
	if result.Keyword != "" {
		like := "%" + result.Keyword + "%"
		where += " AND (code LIKE ? OR name LIKE ?)"
		args = append(args, like, like)
	}
	if len(codes) > 0 {
		where += " AND code IN (" + questionPlaceholders(len(codes)) + ")"
		for _, code := range codes {
			args = append(args, code)
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_stock_fund_flow_source_rows `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(fetched_at) FROM a_stock_stock_fund_flow_source_rows `+where, args...).Scan(&fetchedAt); err != nil {
		return result, err
	}
	if fetchedAt.Valid && strings.TrimSpace(fetchedAt.String) != "" {
		value := mustParseRFC3339(fetchedAt.String)
		result.FetchedAt = &value
	}
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, indicator, source_type, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, field_counts_json, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_stock_fund_flow_source_rows `+where+`
ORDER BY rank ASC, main_net_inflow DESC, code ASC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockStockFundFlow, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanAStockStockFundFlowSourceRow(rows)
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

func (s *Store) listAStockStockFundFlowDistinct(ctx context.Context, column string) ([]string, error) {
	switch column {
	case "trade_date", "indicator":
	default:
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT `+column+` FROM a_stock_stock_fund_flows WHERE `+column+` <> '' ORDER BY `+column+` DESC`)
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

func scanAStockStockFundFlow(scanner scanner) (model.AStockStockFundFlow, error) {
	var item model.AStockStockFundFlow
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.Indicator,
		&item.Code,
		&item.Rank,
		&item.Name,
		&item.Price,
		&item.ChangePct,
		&item.TurnoverPct,
		&item.Amount,
		&item.InAmount,
		&item.OutAmount,
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
		&item.SourceCount,
		&item.SourceTypes,
		&item.FieldCountsJSON,
		&item.SourceType,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AStockStockFundFlow{}, err
	}
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

func scanAStockStockFundFlowSourceRow(scanner scanner) (model.AStockStockFundFlow, error) {
	var item model.AStockStockFundFlow
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.Indicator,
		&item.SourceType,
		&item.Code,
		&item.Rank,
		&item.Name,
		&item.Price,
		&item.ChangePct,
		&item.TurnoverPct,
		&item.Amount,
		&item.InAmount,
		&item.OutAmount,
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
		&item.FieldCountsJSON,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AStockStockFundFlow{}, err
	}
	item.SourceCount = 1
	item.SourceTypes = item.SourceType
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

type aStockStockFundFlowAverage struct {
	item      model.AStockStockFundFlow
	fields    map[string]aStockFundFlowAverageValue
	counts    map[string]int
	sourceSet map[string]struct{}
}

func (s *Store) recomputeAStockStockFundFlowAverageTx(ctx context.Context, tx *Tx, tradeDate string, indicator string) error {
	rows, err := tx.QueryContext(ctx, `
SELECT trade_date, indicator, source_type, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, field_counts_json, raw_payload,
	fetched_at, created_at, updated_at
FROM a_stock_stock_fund_flow_source_rows
WHERE trade_date = ? AND indicator = ?`, tradeDate, indicator)
	if err != nil {
		return err
	}
	defer rows.Close()
	averages := map[string]*aStockStockFundFlowAverage{}
	for rows.Next() {
		source, scanErr := scanAStockStockFundFlowSourceRow(rows)
		if scanErr != nil {
			return scanErr
		}
		code := normalizeAStockFundFlowCode(source.Code)
		if code == "" {
			continue
		}
		avg := averages[code]
		if avg == nil {
			avg = &aStockStockFundFlowAverage{
				item: model.AStockStockFundFlow{
					TradeDate:  tradeDate,
					Indicator:  indicator,
					Code:       code,
					Name:       strings.TrimSpace(source.Name),
					SourceType: "average",
					RawPayload: "{}",
					FetchedAt:  source.FetchedAt,
				},
				fields:    map[string]aStockFundFlowAverageValue{},
				counts:    map[string]int{},
				sourceSet: map[string]struct{}{},
			}
			averages[code] = avg
		}
		if avg.item.Name == "" && strings.TrimSpace(source.Name) != "" {
			avg.item.Name = strings.TrimSpace(source.Name)
		}
		if source.FetchedAt.After(avg.item.FetchedAt) {
			avg.item.FetchedAt = source.FetchedAt
		}
		if source.SourceType != "" {
			avg.sourceSet[source.SourceType] = struct{}{}
		}
		counts := parseAStockFundFlowFieldCounts(source.FieldCountsJSON)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "price", source.Price)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "change_pct", source.ChangePct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "turnover_pct", source.TurnoverPct)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "amount", source.Amount)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "in_amount", source.InAmount)
		addAStockFundFlowAverage(avg.fields, avg.counts, counts, "out_amount", source.OutAmount)
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
	}
	if err := rows.Err(); err != nil {
		return err
	}
	items := make([]model.AStockStockFundFlow, 0, len(averages))
	for _, avg := range averages {
		applyAStockStockFundFlowAverages(avg)
		items = append(items, avg.item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].MainNetInflow == items[j].MainNetInflow {
			return items[i].Code < items[j].Code
		}
		return items[i].MainNetInflow > items[j].MainNetInflow
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM a_stock_stock_fund_flows WHERE trade_date = ? AND indicator = ?`, tradeDate, indicator); err != nil {
		return err
	}
	return insertAStockStockFundFlowAggregateRowsTx(ctx, tx, items)
}

func insertAStockStockFundFlowAggregateRowsTx(ctx context.Context, tx *Tx, items []model.AStockStockFundFlow) error {
	if len(items) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_stock_fund_flows (
	trade_date, indicator, code, rank, name, price, change_pct, turnover_pct, amount, in_amount, out_amount,
	main_net_inflow, main_net_inflow_pct, super_large_net_inflow, super_large_net_inflow_pct,
	large_net_inflow, large_net_inflow_pct, medium_net_inflow, medium_net_inflow_pct,
	small_net_inflow, small_net_inflow_pct, source_count, source_types, field_counts_json, source_type, raw_payload,
	fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, indicator, code) DO UPDATE SET
	rank = excluded.rank,
	name = excluded.name,
	price = excluded.price,
	change_pct = excluded.change_pct,
	turnover_pct = excluded.turnover_pct,
	amount = excluded.amount,
	in_amount = excluded.in_amount,
	out_amount = excluded.out_amount,
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
			item.Indicator,
			normalizeAStockFundFlowCode(item.Code),
			item.Rank,
			strings.TrimSpace(item.Name),
			item.Price,
			item.ChangePct,
			item.TurnoverPct,
			item.Amount,
			item.InAmount,
			item.OutAmount,
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

func applyAStockStockFundFlowAverages(avg *aStockStockFundFlowAverage) {
	avg.item.Price = aStockFundFlowAverage(avg.fields["price"])
	avg.item.ChangePct = aStockFundFlowAverage(avg.fields["change_pct"])
	avg.item.TurnoverPct = aStockFundFlowAverage(avg.fields["turnover_pct"])
	avg.item.Amount = aStockFundFlowAverage(avg.fields["amount"])
	avg.item.InAmount = aStockFundFlowAverage(avg.fields["in_amount"])
	avg.item.OutAmount = aStockFundFlowAverage(avg.fields["out_amount"])
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
	if avg.item.RawPayload == "" || avg.item.RawPayload == "{}" {
		avg.item.RawPayload = `{"aggregation":"average"}`
	}
}

func normalizeAStockFundFlowCode(raw string) string {
	code := astockcode.Normalize(raw)
	if len(code) > 6 {
		code = code[:6]
	}
	return strings.TrimSpace(code)
}

func normalizeAStockFundFlowCodes(raw []string) []string {
	seen := map[string]struct{}{}
	codes := make([]string, 0, len(raw))
	for _, value := range raw {
		code := normalizeAStockFundFlowCode(value)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}
