package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	defaultStockSignalHolderCountChange = 5
	defaultStockSignalFundCountChange   = 3
	defaultStockSignalFloatRatioChange  = 3.0
)

type stockHoldingAggregate struct {
	StockCode    string
	StockName    string
	HolderCount  int
	FundCount    int
	Shares       float64
	FloatRatio   float64
	MarketValue  float64
	ReportPeriod string
}

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

func (s *Store) ListStockInstitutionHoldingSignals(ctx context.Context, filter model.StockInstitutionHoldingSignalFilter) (model.StockInstitutionHoldingSignalListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Code = strings.TrimSpace(filter.Code)
	filter.Company = strings.TrimSpace(filter.Company)
	filter.Period = strings.TrimSpace(filter.Period)

	result := model.StockInstitutionHoldingSignalListResult{
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Code:     filter.Code,
		Company:  filter.Company,
		Period:   filter.Period,
		Thresholds: model.StockInstitutionSignalThreshold{
			HolderCountChange: defaultStockSignalHolderCountChange,
			FundCountChange:   defaultStockSignalFundCountChange,
			FloatRatioChange:  defaultStockSignalFloatRatioChange,
		},
	}

	currentPeriod := filter.Period
	var err error
	if currentPeriod == "" {
		currentPeriod, err = s.stockHoldingSignalPeriod(ctx, filter, "")
		if err != nil {
			return result, err
		}
	}
	if currentPeriod == "" {
		return result, nil
	}
	previousPeriod, err := s.stockHoldingSignalPeriod(ctx, filter, currentPeriod)
	if err != nil {
		return result, err
	}
	result.CurrentPeriod = currentPeriod
	result.PreviousPeriod = previousPeriod
	if previousPeriod == "" {
		return result, nil
	}

	current, err := s.stockHoldingAggregates(ctx, currentPeriod, filter)
	if err != nil {
		return result, err
	}
	previous, err := s.stockHoldingAggregates(ctx, previousPeriod, filter)
	if err != nil {
		return result, err
	}

	signals := make([]model.StockInstitutionHoldingSignal, 0)
	for code, cur := range current {
		prev, ok := previous[code]
		if !ok {
			continue
		}
		signal := model.StockInstitutionHoldingSignal{
			StockCode:           cur.StockCode,
			StockName:           cur.StockName,
			CurrentPeriod:       currentPeriod,
			PreviousPeriod:      previousPeriod,
			CurrentHolderCount:  cur.HolderCount,
			PreviousHolderCount: prev.HolderCount,
			HolderCountChange:   cur.HolderCount - prev.HolderCount,
			CurrentFundCount:    cur.FundCount,
			PreviousFundCount:   prev.FundCount,
			FundCountChange:     cur.FundCount - prev.FundCount,
			CurrentShares:       cur.Shares,
			PreviousShares:      prev.Shares,
			SharesChange:        cur.Shares - prev.Shares,
			CurrentFloatRatio:   cur.FloatRatio,
			PreviousFloatRatio:  prev.FloatRatio,
			FloatRatioChange:    cur.FloatRatio - prev.FloatRatio,
			CurrentMarketValue:  cur.MarketValue,
			PreviousMarketValue: prev.MarketValue,
			MarketValueChange:   cur.MarketValue - prev.MarketValue,
		}
		if !stockHoldingSignalTriggered(signal, result.Thresholds) {
			continue
		}
		signal.Level = stockHoldingSignalLevel(signal)
		signal.Reason = stockHoldingSignalReason(signal)
		signal.NewMajorHolders, err = s.listNewMajorStockHolders(ctx, code, currentPeriod, previousPeriod, 3)
		if err != nil {
			return result, err
		}
		signals = append(signals, signal)
	}

	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Level != signals[j].Level {
			return signals[i].Level == "high"
		}
		if signals[i].FloatRatioChange != signals[j].FloatRatioChange {
			return signals[i].FloatRatioChange > signals[j].FloatRatioChange
		}
		if signals[i].HolderCountChange != signals[j].HolderCountChange {
			return signals[i].HolderCountChange > signals[j].HolderCountChange
		}
		if signals[i].FundCountChange != signals[j].FundCountChange {
			return signals[i].FundCountChange > signals[j].FundCountChange
		}
		return signals[i].MarketValueChange > signals[j].MarketValueChange
	})

	result.Total = len(signals)
	start := (filter.Page - 1) * filter.PageSize
	if start >= len(signals) {
		result.Items = []model.StockInstitutionHoldingSignal{}
		return result, nil
	}
	end := start + filter.PageSize
	if end > len(signals) {
		end = len(signals)
	}
	result.Items = signals[start:end]
	return result, nil
}

func (s *Store) stockHoldingSignalPeriod(ctx context.Context, filter model.StockInstitutionHoldingSignalFilter, before string) (string, error) {
	where := "WHERE report_period <> ''"
	args := []any{}
	if before != "" {
		where += " AND report_period < ?"
		args = append(args, before)
	}
	if filter.Code != "" {
		where += " AND stock_code = ?"
		args = append(args, filter.Code)
	}
	if filter.Company != "" {
		like := "%" + filter.Company + "%"
		where += " AND (stock_name LIKE ? OR stock_code LIKE ?)"
		args = append(args, like, like)
	}
	var period string
	err := s.db.QueryRowContext(ctx, `SELECT report_period FROM stock_institution_holdings `+where+` ORDER BY report_period DESC LIMIT 1`, args...).Scan(&period)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return period, err
}

func (s *Store) stockHoldingAggregates(ctx context.Context, period string, filter model.StockInstitutionHoldingSignalFilter) (map[string]stockHoldingAggregate, error) {
	where := "WHERE report_period = ?"
	args := []any{period}
	if filter.Code != "" {
		where += " AND stock_code = ?"
		args = append(args, filter.Code)
	}
	if filter.Company != "" {
		like := "%" + filter.Company + "%"
		where += " AND (stock_name LIKE ? OR stock_code LIKE ?)"
		args = append(args, like, like)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT stock_code, COALESCE(MAX(stock_name), ''),
	COUNT(DISTINCT holder_name || '|' || holder_type || '|' || holder_code),
	COUNT(DISTINCT CASE WHEN holder_type = 'fund' THEN holder_name || '|' || holder_code END),
	COALESCE(SUM(shares), 0), COALESCE(SUM(float_ratio), 0), COALESCE(SUM(market_value), 0)
FROM stock_institution_holdings `+where+`
GROUP BY stock_code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]stockHoldingAggregate)
	for rows.Next() {
		var item stockHoldingAggregate
		item.ReportPeriod = period
		if err := rows.Scan(&item.StockCode, &item.StockName, &item.HolderCount, &item.FundCount, &item.Shares, &item.FloatRatio, &item.MarketValue); err != nil {
			return nil, err
		}
		out[item.StockCode] = item
	}
	return out, rows.Err()
}

func stockHoldingSignalTriggered(signal model.StockInstitutionHoldingSignal, thresholds model.StockInstitutionSignalThreshold) bool {
	return signal.HolderCountChange >= thresholds.HolderCountChange ||
		signal.FundCountChange >= thresholds.FundCountChange ||
		signal.FloatRatioChange >= thresholds.FloatRatioChange
}

func stockHoldingSignalLevel(signal model.StockInstitutionHoldingSignal) string {
	if signal.HolderCountChange >= 10 || signal.FundCountChange >= 5 || signal.FloatRatioChange >= 5 {
		return "high"
	}
	return "medium"
}

func stockHoldingSignalReason(signal model.StockInstitutionHoldingSignal) string {
	parts := make([]string, 0, 3)
	if signal.HolderCountChange > 0 {
		parts = append(parts, fmt.Sprintf("机构数 +%d", signal.HolderCountChange))
	}
	if signal.FundCountChange > 0 {
		parts = append(parts, fmt.Sprintf("基金数 +%d", signal.FundCountChange))
	}
	if signal.FloatRatioChange > 0 {
		parts = append(parts, fmt.Sprintf("流通占比 +%.2f%%", signal.FloatRatioChange))
	}
	if len(parts) == 0 {
		return "机构持仓变化达到监控阈值"
	}
	return strings.Join(parts, "，")
}

func (s *Store) listNewMajorStockHolders(ctx context.Context, code string, currentPeriod string, previousPeriod string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT c.holder_name, COALESCE(MAX(c.market_value), 0), COALESCE(MAX(c.shares), 0)
FROM stock_institution_holdings c
WHERE c.stock_code = ? AND c.report_period = ? AND c.holder_name <> ''
	AND NOT EXISTS (
		SELECT 1 FROM stock_institution_holdings p
		WHERE p.stock_code = c.stock_code
			AND p.report_period = ?
			AND p.holder_name = c.holder_name
			AND p.holder_type = c.holder_type
			AND p.holder_code = c.holder_code
	)
GROUP BY c.holder_name
ORDER BY COALESCE(MAX(c.market_value), 0) DESC, COALESCE(MAX(c.shares), 0) DESC
LIMIT ?`, code, currentPeriod, previousPeriod, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var name string
		var marketValue, shares float64
		if err := rows.Scan(&name, &marketValue, &shares); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
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
