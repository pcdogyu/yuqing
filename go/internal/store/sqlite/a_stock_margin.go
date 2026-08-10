package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockMargins(ctx context.Context, tradeDate string, summaries []model.AStockMarginSummary, details []model.AStockMarginDetail, replace bool) (model.AStockMarginUpsertResult, error) {
	tradeDate = strings.TrimSpace(tradeDate)
	result := model.AStockMarginUpsertResult{Date: tradeDate, Summaries: len(summaries), Details: len(details), Total: len(summaries) + len(details)}
	if tradeDate == "" {
		for _, item := range summaries {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				tradeDate = date
				result.Date = date
				break
			}
		}
	}
	if tradeDate == "" {
		for _, item := range details {
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
		for _, item := range summaries {
			date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
			market := normalizeAStockMarginMarket(item.Market)
			if date != "" && isAStockMarginMarket(market) {
				replaceKeys[[2]string{date, market}] = struct{}{}
			}
		}
		for _, item := range details {
			date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
			market := normalizeAStockMarginMarket(item.Market)
			if date != "" && isAStockMarginMarket(market) {
				replaceKeys[[2]string{date, market}] = struct{}{}
			}
		}
		for key := range replaceKeys {
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_margin_summaries WHERE trade_date = ? AND market = ?`, key[0], key[1]); err != nil {
				return result, err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_margin_details WHERE trade_date = ? AND market = ?`, key[0], key[1]); err != nil {
				return result, err
			}
		}
	}

	now := time.Now().UTC()
	summaryStmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_margin_summaries (
	trade_date, market, margin_buy_amount, margin_balance, short_sell_volume, short_balance_volume,
	short_balance_amount, margin_trading_balance, source_type, raw_payload, fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, market) DO UPDATE SET
	margin_buy_amount = excluded.margin_buy_amount,
	margin_balance = excluded.margin_balance,
	short_sell_volume = excluded.short_sell_volume,
	short_balance_volume = excluded.short_balance_volume,
	short_balance_amount = excluded.short_balance_amount,
	margin_trading_balance = excluded.margin_trading_balance,
	source_type = excluded.source_type,
	raw_payload = excluded.raw_payload,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer summaryStmt.Close()
	for _, item := range summaries {
		date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
		market := normalizeAStockMarginMarket(item.Market)
		if date == "" || !isAStockMarginMarket(market) {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_margin_summaries WHERE trade_date = ? AND market = ?`, date, market).Scan(new(int)); scanErr == nil {
				existed = true
			} else if scanErr != sql.ErrNoRows {
				err = scanErr
				return result, err
			}
		}
		fetchedAt, createdAt, updatedAt := aStockMarginTimestamps(item.FetchedAt, item.CreatedAt, item.UpdatedAt, now)
		if _, err = summaryStmt.ExecContext(ctx,
			date,
			market,
			floatArg(item.MarginBuyAmount),
			floatArg(item.MarginBalance),
			floatArg(item.ShortSellVolume),
			floatArg(item.ShortBalanceVolume),
			floatArg(item.ShortBalanceAmount),
			floatArg(item.MarginTradingBalance),
			nonEmpty(strings.TrimSpace(item.SourceType), "akshare_stock_margin_"+market),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
			fetchedAt.Format(time.RFC3339),
			createdAt.Format(time.RFC3339),
			updatedAt.Format(time.RFC3339),
		); err != nil {
			return result, err
		}
		if existed {
			result.Updated++
		} else {
			result.Inserted++
		}
	}

	detailStmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_margin_details (
	trade_date, market, code, rank, name, margin_buy_amount, margin_balance, margin_repay_amount,
	short_sell_volume, short_balance_volume, short_repay_volume, short_balance_amount, margin_trading_balance,
	source_type, raw_payload, fetched_at, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, market, code) DO UPDATE SET
	rank = excluded.rank,
	name = excluded.name,
	margin_buy_amount = excluded.margin_buy_amount,
	margin_balance = excluded.margin_balance,
	margin_repay_amount = excluded.margin_repay_amount,
	short_sell_volume = excluded.short_sell_volume,
	short_balance_volume = excluded.short_balance_volume,
	short_repay_volume = excluded.short_repay_volume,
	short_balance_amount = excluded.short_balance_amount,
	margin_trading_balance = excluded.margin_trading_balance,
	source_type = excluded.source_type,
	raw_payload = excluded.raw_payload,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer detailStmt.Close()
	for _, item := range details {
		date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
		market := normalizeAStockMarginMarket(item.Market)
		code := normalizeAStockMarginCode(item.Code)
		if date == "" || !isAStockMarginMarket(market) || code == "" {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_margin_details WHERE trade_date = ? AND market = ? AND code = ?`, date, market, code).Scan(new(int)); scanErr == nil {
				existed = true
			} else if scanErr != sql.ErrNoRows {
				err = scanErr
				return result, err
			}
		}
		fetchedAt, createdAt, updatedAt := aStockMarginTimestamps(item.FetchedAt, item.CreatedAt, item.UpdatedAt, now)
		if _, err = detailStmt.ExecContext(ctx,
			date,
			market,
			code,
			item.Rank,
			strings.TrimSpace(item.Name),
			floatArg(item.MarginBuyAmount),
			floatArg(item.MarginBalance),
			floatArg(item.MarginRepayAmount),
			floatArg(item.ShortSellVolume),
			floatArg(item.ShortBalanceVolume),
			floatArg(item.ShortRepayVolume),
			floatArg(item.ShortBalanceAmount),
			floatArg(item.MarginTradingBalance),
			nonEmpty(strings.TrimSpace(item.SourceType), "akshare_stock_margin_detail_"+market),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
			fetchedAt.Format(time.RFC3339),
			createdAt.Format(time.RFC3339),
			updatedAt.Format(time.RFC3339),
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

func (s *Store) ListAStockMargins(ctx context.Context, filter model.AStockMarginFilter) (model.AStockMarginListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 500 {
		filter.PageSize = 500
	}
	result := model.AStockMarginListResult{
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Date:     strings.TrimSpace(filter.Date),
		Market:   normalizeAStockMarginMarket(filter.Market),
		Keyword:  strings.TrimSpace(filter.Keyword),
		Markets:  []string{"sse", "szse"},
	}
	dates, err := s.listAStockMarginDistinctDates(ctx)
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
	if result.Date == "" {
		result.Summaries = []model.AStockMarginSummary{}
		result.Details = []model.AStockMarginDetail{}
		return result, nil
	}
	summaries, err := s.listAStockMarginSummaries(ctx, result.Date, result.Market)
	if err != nil {
		return result, err
	}
	if result.Market == "all" && len(summaries) > 0 {
		summaries = append(summaries, aggregateAStockMarginSummary(result.Date, summaries))
	}
	result.Summaries = summaries

	where := "WHERE trade_date = ?"
	args := []any{result.Date}
	if isAStockMarginMarket(result.Market) {
		where += " AND market = ?"
		args = append(args, result.Market)
	}
	if result.Keyword != "" {
		like := "%" + result.Keyword + "%"
		code := normalizeAStockMarginCode(result.Keyword)
		if code != "" {
			where += " AND (code = ? OR code LIKE ? OR name LIKE ?)"
			args = append(args, code, like, like)
		} else {
			where += " AND (code LIKE ? OR name LIKE ?)"
			args = append(args, like, like)
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_margin_details `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	fetchedAt, err := s.maxAStockMarginFetchedAt(ctx, result.Date, result.Market)
	if err != nil {
		return result, err
	}
	if !fetchedAt.IsZero() {
		result.FetchedAt = &fetchedAt
	}
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, market, code, rank, name, margin_buy_amount, margin_balance, margin_repay_amount,
	short_sell_volume, short_balance_volume, short_repay_volume, short_balance_amount, margin_trading_balance,
	source_type, raw_payload, fetched_at, created_at, updated_at
FROM a_stock_margin_details `+where+`
ORDER BY CASE market WHEN 'sse' THEN 1 WHEN 'szse' THEN 2 ELSE 3 END, rank ASC, code ASC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	details := make([]model.AStockMarginDetail, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanAStockMarginDetail(rows)
		if scanErr != nil {
			return result, scanErr
		}
		details = append(details, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Details = details
	return result, nil
}

func (s *Store) listAStockMarginDistinctDates(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date FROM (
	SELECT trade_date FROM a_stock_margin_summaries
	UNION
	SELECT trade_date FROM a_stock_margin_details
) dates
GROUP BY trade_date
ORDER BY trade_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}
	return dates, rows.Err()
}

func (s *Store) listAStockMarginSummaries(ctx context.Context, date string, market string) ([]model.AStockMarginSummary, error) {
	where := "WHERE trade_date = ?"
	args := []any{date}
	if isAStockMarginMarket(market) {
		where += " AND market = ?"
		args = append(args, market)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, market, margin_buy_amount, margin_balance, short_sell_volume, short_balance_volume,
	short_balance_amount, margin_trading_balance, source_type, raw_payload, fetched_at, created_at, updated_at
FROM a_stock_margin_summaries `+where+`
ORDER BY CASE market WHEN 'sse' THEN 1 WHEN 'szse' THEN 2 ELSE 3 END`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []model.AStockMarginSummary
	for rows.Next() {
		item, scanErr := scanAStockMarginSummary(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		summaries = append(summaries, item)
	}
	return summaries, rows.Err()
}

func (s *Store) maxAStockMarginFetchedAt(ctx context.Context, date string, market string) (time.Time, error) {
	summaryWhere := "WHERE trade_date = ?"
	detailWhere := "WHERE trade_date = ?"
	args := []any{date}
	detailArgs := []any{date}
	if isAStockMarginMarket(market) {
		summaryWhere += " AND market = ?"
		detailWhere += " AND market = ?"
		args = append(args, market)
		detailArgs = append(detailArgs, market)
	}
	var raw sql.NullString
	query := `SELECT MAX(fetched_at) FROM (
	SELECT fetched_at FROM a_stock_margin_summaries ` + summaryWhere + `
	UNION ALL
	SELECT fetched_at FROM a_stock_margin_details ` + detailWhere + `
)`
	allArgs := append(args, detailArgs...)
	if err := s.db.QueryRowContext(ctx, query, allArgs...).Scan(&raw); err != nil {
		return time.Time{}, err
	}
	if raw.Valid && strings.TrimSpace(raw.String) != "" {
		return mustParseRFC3339(raw.String), nil
	}
	return time.Time{}, nil
}

type marginSummaryScanner interface {
	Scan(dest ...any) error
}

func scanAStockMarginSummary(scanner marginSummaryScanner) (model.AStockMarginSummary, error) {
	var item model.AStockMarginSummary
	var marginBuy, marginBalance, shortSell, shortBalanceVol, shortBalanceAmount, marginTradingBalance sql.NullFloat64
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.Market,
		&marginBuy,
		&marginBalance,
		&shortSell,
		&shortBalanceVol,
		&shortBalanceAmount,
		&marginTradingBalance,
		&item.SourceType,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return item, err
	}
	item.Market = normalizeAStockMarginMarket(item.Market)
	item.MarketLabel = aStockMarginMarketLabel(item.Market)
	item.MarginBuyAmount = floatPtr(marginBuy)
	item.MarginBalance = floatPtr(marginBalance)
	item.ShortSellVolume = floatPtr(shortSell)
	item.ShortBalanceVolume = floatPtr(shortBalanceVol)
	item.ShortBalanceAmount = floatPtr(shortBalanceAmount)
	item.MarginTradingBalance = floatPtr(marginTradingBalance)
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

type marginDetailScanner interface {
	Scan(dest ...any) error
}

func scanAStockMarginDetail(scanner marginDetailScanner) (model.AStockMarginDetail, error) {
	var item model.AStockMarginDetail
	var marginBuy, marginBalance, marginRepay, shortSell, shortBalanceVol, shortRepay, shortBalanceAmount, marginTradingBalance sql.NullFloat64
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.Market,
		&item.Code,
		&item.Rank,
		&item.Name,
		&marginBuy,
		&marginBalance,
		&marginRepay,
		&shortSell,
		&shortBalanceVol,
		&shortRepay,
		&shortBalanceAmount,
		&marginTradingBalance,
		&item.SourceType,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return item, err
	}
	item.Market = normalizeAStockMarginMarket(item.Market)
	item.MarketLabel = aStockMarginMarketLabel(item.Market)
	item.MarginBuyAmount = floatPtr(marginBuy)
	item.MarginBalance = floatPtr(marginBalance)
	item.MarginRepayAmount = floatPtr(marginRepay)
	item.ShortSellVolume = floatPtr(shortSell)
	item.ShortBalanceVolume = floatPtr(shortBalanceVol)
	item.ShortRepayVolume = floatPtr(shortRepay)
	item.ShortBalanceAmount = floatPtr(shortBalanceAmount)
	item.MarginTradingBalance = floatPtr(marginTradingBalance)
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

func aggregateAStockMarginSummary(date string, summaries []model.AStockMarginSummary) model.AStockMarginSummary {
	item := model.AStockMarginSummary{
		TradeDate:   date,
		Market:      "all",
		MarketLabel: "合计",
		SourceType:  "aggregate",
		RawPayload:  `{"aggregation":"sum"}`,
	}
	for _, summary := range summaries {
		item.MarginBuyAmount = sumFloatPtr(item.MarginBuyAmount, summary.MarginBuyAmount)
		item.MarginBalance = sumFloatPtr(item.MarginBalance, summary.MarginBalance)
		item.ShortSellVolume = sumFloatPtr(item.ShortSellVolume, summary.ShortSellVolume)
		item.ShortBalanceVolume = sumFloatPtr(item.ShortBalanceVolume, summary.ShortBalanceVolume)
		item.ShortBalanceAmount = sumFloatPtr(item.ShortBalanceAmount, summary.ShortBalanceAmount)
		item.MarginTradingBalance = sumFloatPtr(item.MarginTradingBalance, summary.MarginTradingBalance)
		if summary.FetchedAt.After(item.FetchedAt) {
			item.FetchedAt = summary.FetchedAt
		}
		if summary.CreatedAt.After(item.CreatedAt) {
			item.CreatedAt = summary.CreatedAt
		}
		if summary.UpdatedAt.After(item.UpdatedAt) {
			item.UpdatedAt = summary.UpdatedAt
		}
	}
	return item
}

func normalizeAStockMarginMarket(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sse", "sh", "shanghai", "沪", "沪市", "上海":
		return "sse"
	case "szse", "sz", "shenzhen", "深", "深市", "深圳":
		return "szse"
	default:
		return "all"
	}
}

func isAStockMarginMarket(value string) bool {
	return value == "sse" || value == "szse"
}

func aStockMarginMarketLabel(market string) string {
	switch normalizeAStockMarginMarket(market) {
	case "sse":
		return "沪市"
	case "szse":
		return "深市"
	default:
		return "合计"
	}
}

func normalizeAStockMarginCode(raw string) string {
	text := strings.TrimSpace(strings.ToLower(raw))
	if text == "" {
		return ""
	}
	var digits strings.Builder
	for _, r := range text {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	value := digits.String()
	if len(value) >= 6 {
		return value[len(value)-6:]
	}
	if value == "" {
		return ""
	}
	for len(value) < 6 {
		value = "0" + value
	}
	return value
}

func aStockMarginTimestamps(fetchedAt time.Time, createdAt time.Time, updatedAt time.Time, now time.Time) (time.Time, time.Time, time.Time) {
	if fetchedAt.IsZero() {
		fetchedAt = now
	}
	if createdAt.IsZero() {
		createdAt = now
	}
	if updatedAt.IsZero() {
		updatedAt = now
	}
	return fetchedAt.UTC(), createdAt.UTC(), updatedAt.UTC()
}

func floatArg(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
	return &out
}

func sumFloatPtr(left *float64, right *float64) *float64 {
	if left == nil && right == nil {
		return nil
	}
	sum := 0.0
	if left != nil {
		sum += *left
	}
	if right != nil {
		sum += *right
	}
	return &sum
}
