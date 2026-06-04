package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Store) UpsertCryptoCandles(ctx context.Context, symbol, interval string, candles []model.CryptoPriceCandle) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO crypto_price_candles (symbol, interval, open_time, open, high, low, close, volume, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(symbol, interval, open_time) DO UPDATE SET
	open = excluded.open,
	high = excluded.high,
	low = excluded.low,
	close = excluded.close,
	volume = excluded.volume,
	updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, candle := range candles {
		createdAt := candle.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := candle.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		if _, err = stmt.ExecContext(
			ctx,
			nonEmpty(candle.Symbol, symbol),
			nonEmpty(candle.Interval, interval),
			candle.OpenTime.UTC().Format(time.RFC3339),
			candle.Open,
			candle.High,
			candle.Low,
			candle.Close,
			candle.Volume,
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *Store) ListCryptoCandles(ctx context.Context, symbol, interval string, limit int) ([]model.CryptoPriceCandle, error) {
	if limit <= 0 {
		limit = 24
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT symbol, interval, open_time, open, high, low, close, volume, created_at, updated_at
FROM crypto_price_candles
WHERE symbol = ? AND interval = ?
ORDER BY open_time DESC
LIMIT ?`, symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.CryptoPriceCandle, 0, limit)
	for rows.Next() {
		candle, scanErr := scanCryptoPriceCandle(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, candle)
	}
	return result, rows.Err()
}

func (s *Store) GetCryptoInsightSnapshot(ctx context.Context, pair, horizonSet string) (model.CryptoInsightSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT pair, horizon_set, payload, computed_at, expires_at
FROM crypto_insight_snapshots
WHERE pair = ? AND horizon_set = ?`, pair, horizonSet)
	return scanCryptoInsightSnapshot(row)
}

func (s *Store) UpsertCryptoInsightSnapshot(ctx context.Context, snapshot model.CryptoInsightSnapshot) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO crypto_insight_snapshots (pair, horizon_set, payload, computed_at, expires_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(pair, horizon_set) DO UPDATE SET
	payload = excluded.payload,
	computed_at = excluded.computed_at,
	expires_at = excluded.expires_at`,
		snapshot.Pair,
		snapshot.HorizonSet,
		snapshot.Payload,
		snapshot.ComputedAt.UTC().Format(time.RFC3339),
		snapshot.ExpiresAt.UTC().Format(time.RFC3339),
	)
	return err
}

func scanCryptoPriceCandle(scanner scanner) (model.CryptoPriceCandle, error) {
	var candle model.CryptoPriceCandle
	var openTime, createdAt, updatedAt string
	if err := scanner.Scan(
		&candle.Symbol,
		&candle.Interval,
		&openTime,
		&candle.Open,
		&candle.High,
		&candle.Low,
		&candle.Close,
		&candle.Volume,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.CryptoPriceCandle{}, err
	}
	candle.OpenTime = mustParseRFC3339(openTime)
	candle.CreatedAt = mustParseRFC3339(createdAt)
	candle.UpdatedAt = mustParseRFC3339(updatedAt)
	return candle, nil
}

func scanCryptoInsightSnapshot(scanner scanner) (model.CryptoInsightSnapshot, error) {
	var snapshot model.CryptoInsightSnapshot
	var computedAt, expiresAt string
	if err := scanner.Scan(&snapshot.Pair, &snapshot.HorizonSet, &snapshot.Payload, &computedAt, &expiresAt); err != nil {
		return model.CryptoInsightSnapshot{}, err
	}
	snapshot.ComputedAt = mustParseRFC3339(computedAt)
	snapshot.ExpiresAt = mustParseRFC3339(expiresAt)
	return snapshot, nil
}

func (s *Store) DeleteExpiredCryptoInsightSnapshots(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM crypto_insight_snapshots WHERE expires_at < ?`, now.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetLatestCryptoInsightSnapshot(ctx context.Context, pair, horizonSet string) (model.CryptoInsightSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT pair, horizon_set, payload, computed_at, expires_at
FROM crypto_insight_snapshots
WHERE pair = ? AND horizon_set = ?
ORDER BY computed_at DESC
LIMIT 1`, pair, horizonSet)
	snapshot, err := scanCryptoInsightSnapshot(row)
	if err != nil && err == sql.ErrNoRows {
		return model.CryptoInsightSnapshot{}, ErrNotFound
	}
	return snapshot, err
}
