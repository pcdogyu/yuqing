package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

var aStockAuctionSHSZFilterSQL = astockcode.SQLWhere()

func (s *Store) UpsertAStockAuctionAmounts(ctx context.Context, tradeDate string, items []model.AStockAuctionAmount, replace bool) (model.AStockAuctionUpsertResult, error) {
	tradeDate = strings.TrimSpace(tradeDate)
	result := model.AStockAuctionUpsertResult{Date: tradeDate, Total: len(items)}
	if tradeDate == "" {
		for _, item := range items {
			if strings.TrimSpace(item.TradeDate) != "" {
				tradeDate = strings.TrimSpace(item.TradeDate)
				result.Date = tradeDate
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
		replaceDates := map[string]struct{}{}
		if tradeDate != "" {
			replaceDates[tradeDate] = struct{}{}
		}
		for _, item := range items {
			if date := strings.TrimSpace(item.TradeDate); date != "" {
				replaceDates[date] = struct{}{}
			}
		}
		for date := range replaceDates {
			if _, err = tx.ExecContext(ctx, `DELETE FROM a_stock_auction_amounts WHERE trade_date = ?`, date); err != nil {
				return result, err
			}
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO a_stock_auction_amounts (trade_date, code, name, auction_price, auction_volume, auction_amount, source, status, fetched_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trade_date, code) DO UPDATE SET
	name = excluded.name,
	auction_price = excluded.auction_price,
	auction_volume = excluded.auction_volume,
	auction_amount = excluded.auction_amount,
	source = excluded.source,
	status = excluded.status,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, item := range items {
		date := nonEmpty(strings.TrimSpace(item.TradeDate), tradeDate)
		code := astockcode.Normalize(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if date == "" || !astockcode.IsShanghaiShenzhen(code) || !astockcode.HasResolvedName(code, name) {
			continue
		}
		existed := false
		if !replace {
			if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_auction_amounts WHERE trade_date = ? AND code = ?`, date, code).Scan(new(int)); scanErr == nil {
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
		if _, err = stmt.ExecContext(
			ctx,
			date,
			code,
			name,
			item.AuctionPrice,
			item.AuctionVolume,
			item.AuctionAmount,
			strings.TrimSpace(item.Source),
			nonEmpty(strings.TrimSpace(item.Status), "ok"),
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

func (s *Store) ListAStockAuctionAmounts(ctx context.Context, filter model.AStockAuctionFilter) (model.AStockAuctionListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 6000 {
		filter.PageSize = 6000
	}
	filter.TrendDays = normalizeAStockAuctionTrendDays(filter.TrendDays)
	result := model.AStockAuctionListResult{Page: filter.Page, PageSize: filter.PageSize, Keyword: strings.TrimSpace(filter.Keyword)}
	dates, err := s.latestAStockAuctionDates(ctx, 7)
	if err != nil {
		return result, err
	}
	result.Dates = dates
	trend, err := s.listAStockAuctionTrend(ctx, filter.TrendDays)
	if err != nil {
		return result, err
	}
	result.Trend = trend
	result.LatestDate = ""
	if len(dates) > 0 {
		result.LatestDate = dates[0]
	}
	result.Date = strings.TrimSpace(filter.Date)
	if result.Date == "" {
		result.Date = result.LatestDate
	}
	if result.Date == "" {
		result.Items = []model.AStockAuctionAmount{}
		return result, nil
	}

	where := withAStockAuctionSHSZWhere(`WHERE trade_date = ?`)
	args := []any{result.Date}
	keyword := result.Keyword
	if keyword != "" {
		like := "%" + keyword + "%"
		where += ` AND (code LIKE ? OR name LIKE ?)`
		args = append(args, like, like)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM a_stock_auction_amounts `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}

	if err := s.loadAStockAuctionSummary(ctx, &result); err != nil {
		return result, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT trade_date, code, name, auction_price, auction_volume, auction_amount, source, status, fetched_at, created_at, updated_at
FROM a_stock_auction_amounts `+where+`
ORDER BY auction_amount DESC, code ASC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.AStockAuctionAmount, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanAStockAuctionAmount(rows)
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

func normalizeAStockAuctionTrendDays(days int) int {
	switch days {
	case 14:
		return 14
	case 30:
		return 30
	default:
		return 7
	}
}

func (s *Store) latestAStockAuctionDates(ctx context.Context, limit int) ([]string, error) {
	limit = max(limit, 1)
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT trade_date FROM a_stock_auction_amounts WHERE `+aStockAuctionSHSZFilterSQL+` ORDER BY trade_date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dates := make([]string, 0, limit)
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}
	return dates, rows.Err()
}

func (s *Store) listAStockAuctionTrend(ctx context.Context, limit int) ([]model.AStockAuctionTrend, error) {
	limit = max(limit, 1)
	rows, err := s.db.QueryContext(ctx, `
WITH recent_dates AS (
	SELECT DISTINCT trade_date
	FROM a_stock_auction_amounts
	WHERE `+aStockAuctionSHSZFilterSQL+`
	ORDER BY trade_date DESC
	LIMIT ?
),
daily AS (
	SELECT trade_date, COUNT(*) AS stock_count, COALESCE(SUM(auction_volume), 0) AS total_volume, COALESCE(SUM(auction_amount), 0) AS total_amount
	FROM a_stock_auction_amounts
	WHERE trade_date IN (SELECT trade_date FROM recent_dates)
	  AND `+aStockAuctionSHSZFilterSQL+`
	GROUP BY trade_date
),
max_rows AS (
	SELECT a.trade_date, a.code, a.name
	FROM a_stock_auction_amounts a
	WHERE a.trade_date IN (SELECT trade_date FROM recent_dates)
	  AND `+aStockAuctionSHSZFilterSQL+`
	  AND NOT EXISTS (
		SELECT 1
		FROM a_stock_auction_amounts b
		WHERE b.trade_date = a.trade_date
		  AND `+aStockAuctionSHSZFilterSQL+`
		  AND (b.auction_amount > a.auction_amount OR (b.auction_amount = a.auction_amount AND b.code < a.code))
	  )
)
SELECT daily.trade_date, daily.stock_count, daily.total_volume, daily.total_amount, COALESCE(max_rows.code, ''), COALESCE(max_rows.name, '')
FROM daily
LEFT JOIN max_rows ON max_rows.trade_date = daily.trade_date
ORDER BY daily.trade_date ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.AStockAuctionTrend, 0, limit)
	for rows.Next() {
		var point model.AStockAuctionTrend
		if err := rows.Scan(&point.Date, &point.StockCount, &point.TotalVolume, &point.TotalAmount, &point.MaxStockCode, &point.MaxStockName); err != nil {
			return nil, err
		}
		out = append(out, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	marketTop, err := s.listAStockAuctionTrendMarketTop(ctx, limit)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].MarketTop = marketTop[out[i].Date]
	}
	return out, nil
}

func (s *Store) listAStockAuctionTrendMarketTop(ctx context.Context, limit int) (map[string][]model.AStockAuctionMarketTop, error) {
	limit = max(limit, 1)
	rows, err := s.db.QueryContext(ctx, `
WITH recent_dates AS (
	SELECT DISTINCT trade_date
	FROM a_stock_auction_amounts
	WHERE `+aStockAuctionSHSZFilterSQL+`
	ORDER BY trade_date DESC
	LIMIT ?
),
classified AS (
	SELECT
		trade_date,
		CASE
			WHEN code LIKE '6%' THEN '沪市'
			WHEN code LIKE '0%' OR code LIKE '3%' THEN '深市'
			ELSE ''
		END AS market,
		code, name, auction_price, auction_volume, auction_amount, source, status, fetched_at, created_at, updated_at
	FROM a_stock_auction_amounts
	WHERE trade_date IN (SELECT trade_date FROM recent_dates)
	  AND `+aStockAuctionSHSZFilterSQL+`
	  AND auction_amount > 0
),
ranked AS (
	SELECT
		*,
		ROW_NUMBER() OVER (
			PARTITION BY trade_date, market
			ORDER BY auction_amount DESC, code ASC
		) AS rn
	FROM classified
)
SELECT trade_date, market, code, name, auction_price, auction_volume, auction_amount, source, status, fetched_at, created_at, updated_at
FROM ranked
WHERE rn <= 3
  AND market <> ''
ORDER BY trade_date ASC,
	CASE market
		WHEN '沪市' THEN 1
		WHEN '深市' THEN 2
		ELSE 3
	END,
	rn ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDateMarket := map[string]map[string][]model.AStockAuctionAmount{}
	for rows.Next() {
		var item model.AStockAuctionAmount
		var market string
		var fetchedAt, createdAt, updatedAt string
		if err := rows.Scan(
			&item.TradeDate,
			&market,
			&item.Code,
			&item.Name,
			&item.AuctionPrice,
			&item.AuctionVolume,
			&item.AuctionAmount,
			&item.Source,
			&item.Status,
			&fetchedAt,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		item.FetchedAt = mustParseRFC3339(fetchedAt)
		item.CreatedAt = mustParseRFC3339(createdAt)
		item.UpdatedAt = mustParseRFC3339(updatedAt)
		if byDateMarket[item.TradeDate] == nil {
			byDateMarket[item.TradeDate] = map[string][]model.AStockAuctionAmount{}
		}
		byDateMarket[item.TradeDate][market] = append(byDateMarket[item.TradeDate][market], item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string][]model.AStockAuctionMarketTop, len(byDateMarket))
	marketOrder := []string{"沪市", "深市"}
	for date, markets := range byDateMarket {
		groups := make([]model.AStockAuctionMarketTop, 0, len(markets))
		for _, market := range marketOrder {
			items := markets[market]
			if len(items) == 0 {
				continue
			}
			groups = append(groups, model.AStockAuctionMarketTop{Market: market, Items: items})
		}
		out[date] = groups
	}
	return out, nil
}

func (s *Store) loadAStockAuctionSummary(ctx context.Context, result *model.AStockAuctionListResult) error {
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*), COALESCE(SUM(auction_amount), 0), MAX(fetched_at)
FROM a_stock_auction_amounts
WHERE trade_date = ?
  AND `+aStockAuctionSHSZFilterSQL, result.Date).Scan(&result.SummaryCount, &result.TotalAmount, &fetchedAt); err != nil {
		return err
	}
	if fetchedAt.Valid {
		value := mustParseRFC3339(fetchedAt.String)
		result.FetchedAt = &value
	}
	row := s.db.QueryRowContext(ctx, `
SELECT trade_date, code, name, auction_price, auction_volume, auction_amount, source, status, fetched_at, created_at, updated_at
FROM a_stock_auction_amounts
WHERE trade_date = ?
  AND `+aStockAuctionSHSZFilterSQL+`
ORDER BY auction_amount DESC, code ASC
LIMIT 1`, result.Date)
	maxItem, err := scanAStockAuctionAmount(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	result.MaxItem = &maxItem
	return nil
}

func scanAStockAuctionAmount(scanner scanner) (model.AStockAuctionAmount, error) {
	var item model.AStockAuctionAmount
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.TradeDate,
		&item.Code,
		&item.Name,
		&item.AuctionPrice,
		&item.AuctionVolume,
		&item.AuctionAmount,
		&item.Source,
		&item.Status,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AStockAuctionAmount{}, err
	}
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func withAStockAuctionSHSZWhere(where string) string {
	where = strings.TrimSpace(where)
	if where == "" {
		return `WHERE ` + aStockAuctionSHSZFilterSQL
	}
	return where + ` AND ` + aStockAuctionSHSZFilterSQL
}
