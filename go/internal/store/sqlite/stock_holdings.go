package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	defaultStockSignalHolderCountChange = 5
	defaultStockSignalFundCountChange   = 3
	defaultStockSignalFundCompanyChange = 2
	defaultStockSignalFloatRatioChange  = 3.0
	defaultStockSignalValueChangeRatio  = 0.10
)

type stockHoldingAggregate struct {
	StockCode        string
	StockName        string
	HolderCount      int
	FundCount        int
	FundCompanyCount int
	Shares           float64
	FloatRatio       float64
	MarketValue      float64
	ReportPeriod     string
	Holders          map[string]stockHoldingHolderSnapshot
	FundCompanies    map[string]struct{}
	SourceTypes      map[string]struct{}
	DisclosureScopes map[string]struct{}
}

type stockHoldingHolderSnapshot struct {
	Name        string
	Type        string
	Code        string
	FundCompany string
	FundCode    string
	Shares      float64
	FloatRatio  float64
	MarketValue float64
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
INSERT INTO stock_institution_holdings (stock_code, stock_name, report_period, announce_date, holder_name, holder_type, holder_code, holder_rank, fund_company, fund_code, report_doc_id, disclosure_scope, shares, shares_change, change_ratio, float_ratio, market_value, source_type, source_url, raw_payload, fetched_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_type, report_period, stock_code, holder_name, holder_type, holder_code) DO UPDATE SET
	stock_name = excluded.stock_name,
	announce_date = excluded.announce_date,
	holder_rank = excluded.holder_rank,
	fund_company = CASE WHEN excluded.fund_company <> '' THEN excluded.fund_company ELSE stock_institution_holdings.fund_company END,
	fund_code = CASE WHEN excluded.fund_code <> '' THEN excluded.fund_code ELSE stock_institution_holdings.fund_code END,
	report_doc_id = CASE WHEN excluded.report_doc_id <> 0 THEN excluded.report_doc_id ELSE stock_institution_holdings.report_doc_id END,
	disclosure_scope = CASE WHEN excluded.disclosure_scope <> '' THEN excluded.disclosure_scope ELSE stock_institution_holdings.disclosure_scope END,
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
		item.FundCompany = strings.TrimSpace(item.FundCompany)
		item.FundCode = strings.TrimSpace(item.FundCode)
		item.DisclosureScope = strings.TrimSpace(item.DisclosureScope)
		if item.HolderCode == "" && item.FundCode != "" {
			item.HolderCode = item.FundCode
		}
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
			item.FundCompany,
			item.FundCode,
			item.ReportDocID,
			item.DisclosureScope,
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

func (s *Store) UpsertStockHoldingReportDocuments(ctx context.Context, items []model.StockHoldingReportDocument) (model.StockHoldingReportDocumentUpsertResult, error) {
	result := model.StockHoldingReportDocumentUpsertResult{Total: len(items)}
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
INSERT INTO stock_holding_report_documents (source_type, source_key, report_period, fund_code, fund_name, fund_company, announcement_title, announcement_date, source_url, pdf_url, parse_status, raw_payload, fetched_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_type, source_key) DO UPDATE SET
	report_period = excluded.report_period,
	fund_code = excluded.fund_code,
	fund_name = excluded.fund_name,
	fund_company = excluded.fund_company,
	announcement_title = excluded.announcement_title,
	announcement_date = excluded.announcement_date,
	source_url = excluded.source_url,
	pdf_url = CASE WHEN excluded.pdf_url <> '' THEN excluded.pdf_url ELSE stock_holding_report_documents.pdf_url END,
	parse_status = CASE WHEN excluded.parse_status <> '' THEN excluded.parse_status ELSE stock_holding_report_documents.parse_status END,
	raw_payload = excluded.raw_payload,
	fetched_at = excluded.fetched_at,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, item := range items {
		item.SourceType = strings.TrimSpace(item.SourceType)
		item.SourceKey = strings.TrimSpace(item.SourceKey)
		item.ReportPeriod = strings.TrimSpace(item.ReportPeriod)
		item.FundCode = strings.TrimSpace(item.FundCode)
		item.FundName = strings.TrimSpace(item.FundName)
		item.FundCompany = strings.TrimSpace(item.FundCompany)
		item.AnnouncementTitle = strings.TrimSpace(item.AnnouncementTitle)
		item.AnnouncementDate = strings.TrimSpace(item.AnnouncementDate)
		item.SourceURL = strings.TrimSpace(item.SourceURL)
		item.PDFURL = strings.TrimSpace(item.PDFURL)
		item.ParseStatus = strings.TrimSpace(item.ParseStatus)
		item.RawPayload = nonEmpty(strings.TrimSpace(item.RawPayload), "{}")
		if item.SourceType == "" || item.SourceKey == "" || item.ReportPeriod == "" || item.AnnouncementTitle == "" {
			continue
		}
		existed := false
		if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM stock_holding_report_documents WHERE source_type = ? AND source_key = ?`,
			item.SourceType, item.SourceKey).Scan(new(int)); scanErr == nil {
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
			item.SourceType,
			item.SourceKey,
			item.ReportPeriod,
			item.FundCode,
			item.FundName,
			item.FundCompany,
			item.AnnouncementTitle,
			item.AnnouncementDate,
			item.SourceURL,
			item.PDFURL,
			item.ParseStatus,
			item.RawPayload,
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

func (s *Store) ListStockHoldingReportDocuments(ctx context.Context, filter model.StockHoldingReportDocumentFilter) (model.StockHoldingReportDocumentListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	result := model.StockHoldingReportDocumentListResult{
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Period:      strings.TrimSpace(filter.Period),
		FundCompany: strings.TrimSpace(filter.FundCompany),
		Fund:        strings.TrimSpace(filter.Fund),
		Source:      strings.TrimSpace(filter.Source),
	}
	where, args := stockHoldingReportWhere(result)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_holding_report_documents `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	periods, err := s.listStockHoldingReportDistinct(ctx, "report_period")
	if err != nil {
		return result, err
	}
	result.Periods = periods
	sources, err := s.listStockHoldingReportDistinct(ctx, "source_type")
	if err != nil {
		return result, err
	}
	result.Sources = sources

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source_type, source_key, report_period, fund_code, fund_name, fund_company, announcement_title, announcement_date, source_url, pdf_url, parse_status, raw_payload, fetched_at, created_at, updated_at
FROM stock_holding_report_documents `+where+`
ORDER BY report_period DESC, announcement_date DESC, id DESC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.StockHoldingReportDocument, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanStockHoldingReportDocument(rows)
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

func (s *Store) ListStockInstitutionHoldings(ctx context.Context, filter model.StockInstitutionHoldingFilter) (model.StockInstitutionHoldingListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	result := model.StockInstitutionHoldingListResult{
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Code:        strings.TrimSpace(filter.Code),
		Company:     strings.TrimSpace(filter.Company),
		Period:      strings.TrimSpace(filter.Period),
		Holder:      strings.TrimSpace(filter.Holder),
		HolderType:  strings.TrimSpace(filter.HolderType),
		FundCompany: strings.TrimSpace(filter.FundCompany),
		Source:      strings.TrimSpace(filter.Source),
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
SELECT id, stock_code, stock_name, report_period, announce_date, holder_name, holder_type, holder_code, holder_rank, fund_company, fund_code, report_doc_id, disclosure_scope, shares, shares_change, change_ratio, float_ratio, market_value, source_type, source_url, raw_payload, fetched_at, created_at, updated_at
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
	COUNT(DISTINCT CASE WHEN holder_type = 'fund' AND fund_company <> '' THEN fund_company END),
	COUNT(DISTINCT holder_type), COALESCE(SUM(shares), 0), COALESCE(SUM(float_ratio), 0), COALESCE(SUM(market_value), 0)
FROM stock_institution_holdings `+where, args...)
	if err := row.Scan(&summary.StockName, &summary.HolderCount, &summary.FundCount, &summary.FundCompanyCount, &summary.HolderTypeCount, &summary.TotalShares, &summary.TotalFloatRatio, &summary.TotalMarketValue); err != nil {
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
	filter.FundCompany = strings.TrimSpace(filter.FundCompany)
	filter.SignalType = normalizeStockHoldingSignalType(filter.SignalType)

	result := model.StockInstitutionHoldingSignalListResult{
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Code:        filter.Code,
		Company:     filter.Company,
		Period:      filter.Period,
		FundCompany: filter.FundCompany,
		SignalType:  filter.SignalType,
		Thresholds: model.StockInstitutionSignalThreshold{
			HolderCountChange:      defaultStockSignalHolderCountChange,
			FundCountChange:        defaultStockSignalFundCountChange,
			FundCompanyCountChange: defaultStockSignalFundCompanyChange,
			ExitHolderCountChange:  defaultStockSignalHolderCountChange,
			ExitFundCountChange:    defaultStockSignalFundCountChange,
			FloatRatioChange:       defaultStockSignalFloatRatioChange,
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
	codes := make(map[string]struct{}, len(current)+len(previous))
	for code := range current {
		codes[code] = struct{}{}
	}
	for code := range previous {
		codes[code] = struct{}{}
	}
	for code := range codes {
		cur := current[code]
		prev := previous[code]
		newHolders, exitedHolders := diffStockHoldingHolders(cur.Holders, prev.Holders)
		newFundCount, newFundCompanies := stockHoldingFundDiffStats(newHolders)
		exitedFundCount, exitedFundCompanies := stockHoldingFundDiffStats(exitedHolders)
		signal := model.StockInstitutionHoldingSignal{
			StockCode:                nonEmpty(cur.StockCode, prev.StockCode),
			StockName:                nonEmpty(cur.StockName, prev.StockName),
			CurrentPeriod:            currentPeriod,
			PreviousPeriod:           previousPeriod,
			CurrentHolderCount:       cur.HolderCount,
			PreviousHolderCount:      prev.HolderCount,
			HolderCountChange:        cur.HolderCount - prev.HolderCount,
			NewHolderCount:           len(newHolders),
			ExitedHolderCount:        len(exitedHolders),
			CurrentFundCount:         cur.FundCount,
			PreviousFundCount:        prev.FundCount,
			FundCountChange:          cur.FundCount - prev.FundCount,
			NewFundCount:             newFundCount,
			ExitedFundCount:          exitedFundCount,
			CurrentFundCompanyCount:  cur.FundCompanyCount,
			PreviousFundCompanyCount: prev.FundCompanyCount,
			FundCompanyCountChange:   cur.FundCompanyCount - prev.FundCompanyCount,
			NewFundCompanyCount:      len(newFundCompanies),
			ExitedFundCompanyCount:   len(exitedFundCompanies),
			CurrentShares:            cur.Shares,
			PreviousShares:           prev.Shares,
			SharesChange:             cur.Shares - prev.Shares,
			CurrentFloatRatio:        cur.FloatRatio,
			PreviousFloatRatio:       prev.FloatRatio,
			FloatRatioChange:         cur.FloatRatio - prev.FloatRatio,
			CurrentMarketValue:       cur.MarketValue,
			PreviousMarketValue:      prev.MarketValue,
			MarketValueChange:        cur.MarketValue - prev.MarketValue,
			NewMajorHolders:          stockHoldingTopHolderNames(newHolders, 3),
			ExitedMajorHolders:       stockHoldingTopHolderNames(exitedHolders, 3),
			SourceTypes:              stockHoldingSortedSetUnion(cur.SourceTypes, prev.SourceTypes),
			DisclosureScope:          stockHoldingDisclosureScope(cur.DisclosureScopes, prev.DisclosureScopes),
		}
		signalTypes := stockHoldingSignalTypes(signal, result.Thresholds)
		if len(signalTypes) == 0 {
			continue
		}
		if filter.SignalType != "" {
			if !stockHoldingSignalTypeIn(signalTypes, filter.SignalType) {
				continue
			}
			signal.SignalType = filter.SignalType
		} else {
			signal.SignalType = signalTypes[0]
		}
		signal.Level = stockHoldingSignalLevel(signal)
		signal.Reason = stockHoldingSignalReason(signal)
		signals = append(signals, signal)
	}

	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Level != signals[j].Level {
			return signals[i].Level == "high"
		}
		if severity := stockHoldingSignalSeverity(signals[i]) - stockHoldingSignalSeverity(signals[j]); severity != 0 {
			return severity > 0
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
	if filter.FundCompany != "" {
		like := "%" + filter.FundCompany + "%"
		where += " AND fund_company LIKE ?"
		args = append(args, like)
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
	if filter.FundCompany != "" {
		like := "%" + filter.FundCompany + "%"
		where += " AND fund_company LIKE ?"
		args = append(args, like)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT stock_code, stock_name, holder_name, holder_type, holder_code, fund_company, fund_code, shares, float_ratio, market_value, source_type, disclosure_scope
FROM stock_institution_holdings `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]stockHoldingAggregate)
	for rows.Next() {
		var stockCode, stockName, holderName, holderType, holderCode, fundCompany, fundCode, sourceType, disclosureScope string
		var shares, floatRatio, marketValue float64
		if err := rows.Scan(&stockCode, &stockName, &holderName, &holderType, &holderCode, &fundCompany, &fundCode, &shares, &floatRatio, &marketValue, &sourceType, &disclosureScope); err != nil {
			return nil, err
		}
		stockCode = strings.TrimSpace(stockCode)
		holderName = strings.TrimSpace(holderName)
		if stockCode == "" || holderName == "" {
			continue
		}
		item := out[stockCode]
		if item.StockCode == "" {
			item = stockHoldingAggregate{
				StockCode:        stockCode,
				ReportPeriod:     period,
				Holders:          make(map[string]stockHoldingHolderSnapshot),
				FundCompanies:    make(map[string]struct{}),
				SourceTypes:      make(map[string]struct{}),
				DisclosureScopes: make(map[string]struct{}),
			}
		}
		if item.StockName == "" {
			item.StockName = strings.TrimSpace(stockName)
		}
		holderType = strings.TrimSpace(holderType)
		holderCode = strings.TrimSpace(holderCode)
		fundCompany = strings.TrimSpace(fundCompany)
		fundCode = strings.TrimSpace(fundCode)
		if holderCode == "" && fundCode != "" {
			holderCode = fundCode
		}
		holder := stockHoldingHolderSnapshot{
			Name:        holderName,
			Type:        holderType,
			Code:        holderCode,
			FundCompany: fundCompany,
			FundCode:    fundCode,
			Shares:      shares,
			FloatRatio:  floatRatio,
			MarketValue: marketValue,
		}
		key := stockHoldingHolderKey(holder)
		if key != "" {
			if existing, ok := item.Holders[key]; ok {
				if stockHoldingHolderBetter(holder, existing) {
					item.Shares += holder.Shares - existing.Shares
					item.FloatRatio += holder.FloatRatio - existing.FloatRatio
					item.MarketValue += holder.MarketValue - existing.MarketValue
					item.Holders[key] = holder
				}
			} else {
				item.Holders[key] = holder
				item.HolderCount++
				if holder.Type == "fund" {
					item.FundCount++
				}
				item.Shares += holder.Shares
				item.FloatRatio += holder.FloatRatio
				item.MarketValue += holder.MarketValue
			}
		}
		if holder.Type == "fund" && fundCompany != "" {
			item.FundCompanies[fundCompany] = struct{}{}
		}
		if sourceType = strings.TrimSpace(sourceType); sourceType != "" {
			item.SourceTypes[sourceType] = struct{}{}
		}
		if disclosureScope = strings.TrimSpace(disclosureScope); disclosureScope != "" {
			item.DisclosureScopes[disclosureScope] = struct{}{}
		}
		item.FundCompanyCount = len(item.FundCompanies)
		out[stockCode] = item
	}
	return out, rows.Err()
}

func stockHoldingSignalLevel(signal model.StockInstitutionHoldingSignal) string {
	if signal.NewHolderCount >= 10 || signal.ExitedHolderCount >= 10 ||
		signal.NewFundCount >= 5 || signal.ExitedFundCount >= 5 ||
		math.Abs(signal.FloatRatioChange) >= 5 {
		return "high"
	}
	return "medium"
}

func stockHoldingSignalReason(signal model.StockInstitutionHoldingSignal) string {
	parts := make([]string, 0, 5)
	switch signal.SignalType {
	case "new_entry":
		if signal.NewHolderCount > 0 {
			parts = append(parts, fmt.Sprintf("新进披露机构 %d 家", signal.NewHolderCount))
		}
		if signal.NewFundCount > 0 {
			parts = append(parts, fmt.Sprintf("新进基金 %d 只", signal.NewFundCount))
		}
		if signal.NewFundCompanyCount > 0 {
			parts = append(parts, fmt.Sprintf("新进基金公司 %d 家", signal.NewFundCompanyCount))
		}
	case "exit_disclosure":
		if signal.ExitedHolderCount > 0 {
			parts = append(parts, fmt.Sprintf("退出披露名单机构 %d 家", signal.ExitedHolderCount))
		}
		if signal.ExitedFundCount > 0 {
			parts = append(parts, fmt.Sprintf("退出披露名单基金 %d 只", signal.ExitedFundCount))
		}
		if signal.ExitedFundCompanyCount > 0 {
			parts = append(parts, fmt.Sprintf("退出披露名单基金公司 %d 家", signal.ExitedFundCompanyCount))
		}
	case "increase":
		if signal.HolderCountChange > 0 {
			parts = append(parts, fmt.Sprintf("机构数 +%d", signal.HolderCountChange))
		}
		if signal.FundCountChange > 0 {
			parts = append(parts, fmt.Sprintf("基金数 +%d", signal.FundCountChange))
		}
		if signal.SharesChange > 0 {
			parts = append(parts, "持股数 "+stockHoldingReasonSignedNumber(signal.SharesChange))
		}
		if signal.MarketValueChange > 0 {
			parts = append(parts, "市值 "+stockHoldingReasonSignedMoney(signal.MarketValueChange))
		}
	case "decrease":
		if signal.HolderCountChange < 0 {
			parts = append(parts, fmt.Sprintf("机构数 %d", signal.HolderCountChange))
		}
		if signal.FundCountChange < 0 {
			parts = append(parts, fmt.Sprintf("基金数 %d", signal.FundCountChange))
		}
		if signal.SharesChange < 0 {
			parts = append(parts, "持股数 "+stockHoldingReasonSignedNumber(signal.SharesChange))
		}
		if signal.MarketValueChange < 0 {
			parts = append(parts, "市值 "+stockHoldingReasonSignedMoney(signal.MarketValueChange))
		}
	}
	if signal.FloatRatioChange > 0 {
		parts = append(parts, fmt.Sprintf("流通占比 +%.2f%%", signal.FloatRatioChange))
	} else if signal.FloatRatioChange < 0 {
		parts = append(parts, fmt.Sprintf("流通占比 %.2f%%", signal.FloatRatioChange))
	}
	if len(parts) == 0 {
		return "季度披露持仓变化达到监控阈值"
	}
	return strings.Join(parts, "，")
}

func stockHoldingReasonSignedNumber(value float64) string {
	if value > 0 {
		return fmt.Sprintf("+%.0f", value)
	}
	return fmt.Sprintf("%.0f", value)
}

func stockHoldingReasonSignedMoney(value float64) string {
	sign := ""
	if value > 0 {
		sign = "+"
	} else if value < 0 {
		sign = "-"
	}
	absValue := math.Abs(value)
	if absValue >= 100000000 {
		return fmt.Sprintf("%s%.2f亿", sign, absValue/100000000)
	}
	if absValue >= 10000 {
		return fmt.Sprintf("%s%.2f万", sign, absValue/10000)
	}
	return fmt.Sprintf("%s%.0f", sign, absValue)
}

func stockHoldingSignalTypes(signal model.StockInstitutionHoldingSignal, thresholds model.StockInstitutionSignalThreshold) []string {
	types := make([]string, 0, 2)
	primary := stockHoldingPrimarySignalType(signal, thresholds)
	if primary != "" {
		types = append(types, primary)
	}
	switch direction := stockHoldingValueChangeDirection(signal, thresholds); direction {
	case 1:
		if !stockHoldingSignalTypeIn(types, "increase") {
			types = append(types, "increase")
		}
	case -1:
		if !stockHoldingSignalTypeIn(types, "decrease") {
			types = append(types, "decrease")
		}
	}
	return types
}

func stockHoldingPrimarySignalType(signal model.StockInstitutionHoldingSignal, thresholds model.StockInstitutionSignalThreshold) string {
	newTriggered := signal.NewHolderCount >= thresholds.HolderCountChange ||
		signal.NewFundCount >= thresholds.FundCountChange ||
		signal.NewFundCompanyCount >= thresholds.FundCompanyCountChange ||
		signal.HolderCountChange >= thresholds.HolderCountChange ||
		signal.FundCountChange >= thresholds.FundCountChange
	exitTriggered := signal.ExitedHolderCount >= thresholds.ExitHolderCountChange ||
		signal.ExitedFundCount >= thresholds.ExitFundCountChange ||
		signal.HolderCountChange <= -thresholds.ExitHolderCountChange ||
		signal.FundCountChange <= -thresholds.ExitFundCountChange
	switch {
	case newTriggered && exitTriggered:
		if signal.ExitedHolderCount > signal.NewHolderCount || signal.HolderCountChange < 0 {
			return "exit_disclosure"
		}
		return "new_entry"
	case newTriggered:
		return "new_entry"
	case exitTriggered:
		return "exit_disclosure"
	}
	switch direction := stockHoldingValueChangeDirection(signal, thresholds); direction {
	case 1:
		return "increase"
	case -1:
		return "decrease"
	default:
		return ""
	}
}

func stockHoldingValueChangeDirection(signal model.StockInstitutionHoldingSignal, thresholds model.StockInstitutionSignalThreshold) int {
	if signal.FloatRatioChange >= thresholds.FloatRatioChange {
		return 1
	}
	if signal.FloatRatioChange <= -thresholds.FloatRatioChange {
		return -1
	}
	if stockHoldingRelativeChangeTriggered(signal.SharesChange, signal.PreviousShares) {
		if signal.SharesChange > 0 {
			return 1
		}
		return -1
	}
	if stockHoldingRelativeChangeTriggered(signal.MarketValueChange, signal.PreviousMarketValue) {
		if signal.MarketValueChange > 0 {
			return 1
		}
		return -1
	}
	return 0
}

func stockHoldingRelativeChangeTriggered(change float64, previous float64) bool {
	if change == 0 {
		return false
	}
	base := math.Abs(previous)
	if base == 0 {
		return true
	}
	return math.Abs(change)/base >= defaultStockSignalValueChangeRatio
}

func stockHoldingSignalTypeIn(types []string, signalType string) bool {
	for _, value := range types {
		if value == signalType {
			return true
		}
	}
	return false
}

func normalizeStockHoldingSignalType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "new", "new_entry", "in":
		return "new_entry"
	case "exit", "exit_disclosure", "out":
		return "exit_disclosure"
	case "increase", "add":
		return "increase"
	case "decrease", "reduce":
		return "decrease"
	default:
		return ""
	}
}

func stockHoldingHolderKey(holder stockHoldingHolderSnapshot) string {
	name := strings.TrimSpace(holder.Name)
	holderType := strings.TrimSpace(holder.Type)
	code := strings.TrimSpace(nonEmpty(holder.Code, holder.FundCode))
	if name == "" || holderType == "" {
		return ""
	}
	return strings.Join([]string{name, holderType, code}, "|")
}

func stockHoldingHolderBetter(next, current stockHoldingHolderSnapshot) bool {
	if next.MarketValue != current.MarketValue {
		return next.MarketValue > current.MarketValue
	}
	if next.Shares != current.Shares {
		return next.Shares > current.Shares
	}
	return math.Abs(next.FloatRatio) > math.Abs(current.FloatRatio)
}

func diffStockHoldingHolders(current, previous map[string]stockHoldingHolderSnapshot) ([]stockHoldingHolderSnapshot, []stockHoldingHolderSnapshot) {
	newHolders := make([]stockHoldingHolderSnapshot, 0)
	for key, holder := range current {
		if _, ok := previous[key]; !ok {
			newHolders = append(newHolders, holder)
		}
	}
	exitedHolders := make([]stockHoldingHolderSnapshot, 0)
	for key, holder := range previous {
		if _, ok := current[key]; !ok {
			exitedHolders = append(exitedHolders, holder)
		}
	}
	return newHolders, exitedHolders
}

func stockHoldingFundDiffStats(holders []stockHoldingHolderSnapshot) (int, map[string]struct{}) {
	companies := make(map[string]struct{})
	count := 0
	for _, holder := range holders {
		if holder.Type != "fund" {
			continue
		}
		count++
		if company := strings.TrimSpace(holder.FundCompany); company != "" {
			companies[company] = struct{}{}
		}
	}
	return count, companies
}

func stockHoldingTopHolderNames(holders []stockHoldingHolderSnapshot, limit int) []string {
	if limit <= 0 || len(holders) == 0 {
		return nil
	}
	sort.Slice(holders, func(i, j int) bool {
		if holders[i].MarketValue != holders[j].MarketValue {
			return holders[i].MarketValue > holders[j].MarketValue
		}
		if holders[i].Shares != holders[j].Shares {
			return holders[i].Shares > holders[j].Shares
		}
		return holders[i].Name < holders[j].Name
	})
	if len(holders) > limit {
		holders = holders[:limit]
	}
	out := make([]string, 0, len(holders))
	for _, holder := range holders {
		name := strings.TrimSpace(holder.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func stockHoldingSortedSetUnion(sets ...map[string]struct{}) []string {
	seen := make(map[string]struct{})
	for _, set := range sets {
		for value := range set {
			value = strings.TrimSpace(value)
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stockHoldingDisclosureScope(sets ...map[string]struct{}) string {
	values := stockHoldingSortedSetUnion(sets...)
	if len(values) == 0 {
		return "quarter_disclosure"
	}
	return strings.Join(values, ",")
}

func stockHoldingSignalSeverity(signal model.StockInstitutionHoldingSignal) int {
	return absInt(signal.NewHolderCount)*12 +
		absInt(signal.ExitedHolderCount)*12 +
		absInt(signal.NewFundCount)*10 +
		absInt(signal.ExitedFundCount)*10 +
		absInt(signal.NewFundCompanyCount)*8 +
		absInt(signal.ExitedFundCompanyCount)*8 +
		int(math.Abs(signal.FloatRatioChange)*10) +
		absInt(signal.HolderCountChange)*4 +
		absInt(signal.FundCountChange)*4
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
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
	if filter.FundCompany != "" {
		like := "%" + filter.FundCompany + "%"
		where += " AND fund_company LIKE ?"
		args = append(args, like)
	}
	if filter.Source != "" && filter.Source != "all" {
		where += " AND source_type = ?"
		args = append(args, filter.Source)
	}
	return where, args
}

func stockHoldingReportWhere(filter model.StockHoldingReportDocumentListResult) (string, []any) {
	where := "WHERE 1=1"
	args := []any{}
	if filter.Period != "" {
		where += " AND report_period = ?"
		args = append(args, filter.Period)
	}
	if filter.FundCompany != "" {
		like := "%" + filter.FundCompany + "%"
		where += " AND fund_company LIKE ?"
		args = append(args, like)
	}
	if filter.Fund != "" {
		like := "%" + filter.Fund + "%"
		where += " AND (fund_name LIKE ? OR fund_code LIKE ? OR announcement_title LIKE ?)"
		args = append(args, like, like, like)
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

func (s *Store) listStockHoldingReportDistinct(ctx context.Context, column string) ([]string, error) {
	switch column {
	case "report_period", "source_type":
	default:
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT `+column+` FROM stock_holding_report_documents WHERE `+column+` <> '' ORDER BY `+column+` DESC`)
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
		&item.FundCompany,
		&item.FundCode,
		&item.ReportDocID,
		&item.DisclosureScope,
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

func scanStockHoldingReportDocument(scanner scanner) (model.StockHoldingReportDocument, error) {
	var item model.StockHoldingReportDocument
	var fetchedAt, createdAt, updatedAt string
	if err := scanner.Scan(
		&item.ID,
		&item.SourceType,
		&item.SourceKey,
		&item.ReportPeriod,
		&item.FundCode,
		&item.FundName,
		&item.FundCompany,
		&item.AnnouncementTitle,
		&item.AnnouncementDate,
		&item.SourceURL,
		&item.PDFURL,
		&item.ParseStatus,
		&item.RawPayload,
		&fetchedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.StockHoldingReportDocument{}, err
	}
	item.FetchedAt = mustParseRFC3339(fetchedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}
