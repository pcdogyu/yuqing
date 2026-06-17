package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertAStockAuctionAmounts(ctx context.Context, tradeDate string, items []model.AStockAuctionAmount) (model.AStockAuctionUpsertResult, error) {
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
		code := strings.TrimSpace(item.Code)
		if date == "" || code == "" {
			continue
		}
		existed := false
		if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM a_stock_auction_amounts WHERE trade_date = ? AND code = ?`, date, code).Scan(new(int)); scanErr == nil {
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
		if _, err = stmt.ExecContext(
			ctx,
			date,
			code,
			strings.TrimSpace(item.Name),
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
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	result := model.AStockAuctionListResult{Page: filter.Page, PageSize: filter.PageSize, Keyword: strings.TrimSpace(filter.Keyword)}
	dates, err := s.latestAStockAuctionDates(ctx, 7)
	if err != nil {
		return result, err
	}
	result.Dates = dates
	trend, err := s.listAStockAuctionTrend(ctx, 30)
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

	where := `WHERE trade_date = ?`
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

func (s *Store) latestAStockAuctionDates(ctx context.Context, limit int) ([]string, error) {
	limit = max(limit, 1)
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT trade_date FROM a_stock_auction_amounts ORDER BY trade_date DESC LIMIT ?`, limit)
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
	ORDER BY trade_date DESC
	LIMIT ?
),
daily AS (
	SELECT trade_date, COUNT(*) AS stock_count, COALESCE(SUM(auction_volume), 0) AS total_volume, COALESCE(SUM(auction_amount), 0) AS total_amount
	FROM a_stock_auction_amounts
	WHERE trade_date IN (SELECT trade_date FROM recent_dates)
	GROUP BY trade_date
),
max_rows AS (
	SELECT a.trade_date, a.code, a.name
	FROM a_stock_auction_amounts a
	WHERE a.trade_date IN (SELECT trade_date FROM recent_dates)
	  AND NOT EXISTS (
		SELECT 1
		FROM a_stock_auction_amounts b
		WHERE b.trade_date = a.trade_date
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
	return out, rows.Err()
}

func (s *Store) loadAStockAuctionSummary(ctx context.Context, result *model.AStockAuctionListResult) error {
	var fetchedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*), COALESCE(SUM(auction_amount), 0), MAX(fetched_at)
FROM a_stock_auction_amounts
WHERE trade_date = ?`, result.Date).Scan(&result.SummaryCount, &result.TotalAmount, &fetchedAt); err != nil {
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
