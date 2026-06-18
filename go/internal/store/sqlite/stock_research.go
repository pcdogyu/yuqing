package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) UpsertStockResearchSurveys(ctx context.Context, items []model.StockResearchSurvey) (model.StockResearchUpsertResult, error) {
	result := model.StockResearchUpsertResult{Total: len(items)}
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
INSERT INTO stock_research_surveys (code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, pdf_url, pdf_file_path, pdf_status, pdf_text, pdf_error, pdf_fetched_at, pdf_parsed_at, nlp_score, nlp_rating, nlp_reason, nlp_scored_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_type, source_key) DO UPDATE SET
	code = excluded.code,
	name = excluded.name,
	kind = excluded.kind,
	title = excluded.title,
	institution = excluded.institution,
	analyst = excluded.analyst,
	rating = excluded.rating,
	target_price = excluded.target_price,
	research_date = excluded.research_date,
	publish_time = excluded.publish_time,
	source_url = excluded.source_url,
	summary = excluded.summary,
	raw_payload = excluded.raw_payload,
	pdf_url = CASE WHEN excluded.pdf_url <> '' THEN excluded.pdf_url ELSE stock_research_surveys.pdf_url END,
	pdf_file_path = CASE WHEN excluded.pdf_file_path <> '' THEN excluded.pdf_file_path ELSE stock_research_surveys.pdf_file_path END,
	pdf_status = CASE WHEN excluded.pdf_status <> '' THEN excluded.pdf_status ELSE stock_research_surveys.pdf_status END,
	pdf_text = CASE WHEN excluded.pdf_text <> '' THEN excluded.pdf_text ELSE stock_research_surveys.pdf_text END,
	pdf_error = CASE WHEN excluded.pdf_error <> '' THEN excluded.pdf_error ELSE stock_research_surveys.pdf_error END,
	pdf_fetched_at = CASE WHEN excluded.pdf_fetched_at <> '' THEN excluded.pdf_fetched_at ELSE stock_research_surveys.pdf_fetched_at END,
	pdf_parsed_at = CASE WHEN excluded.pdf_parsed_at <> '' THEN excluded.pdf_parsed_at ELSE stock_research_surveys.pdf_parsed_at END,
	nlp_score = CASE WHEN excluded.nlp_scored_at <> '' THEN excluded.nlp_score ELSE stock_research_surveys.nlp_score END,
	nlp_rating = CASE WHEN excluded.nlp_rating <> '' THEN excluded.nlp_rating ELSE stock_research_surveys.nlp_rating END,
	nlp_reason = CASE WHEN excluded.nlp_reason <> '' THEN excluded.nlp_reason ELSE stock_research_surveys.nlp_reason END,
	nlp_scored_at = CASE WHEN excluded.nlp_scored_at <> '' THEN excluded.nlp_scored_at ELSE stock_research_surveys.nlp_scored_at END,
	updated_at = excluded.updated_at`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, item := range items {
		sourceType := strings.TrimSpace(item.SourceType)
		sourceKey := strings.TrimSpace(item.SourceKey)
		if sourceType == "" || sourceKey == "" {
			continue
		}
		existed := false
		if scanErr := tx.QueryRowContext(ctx, `SELECT 1 FROM stock_research_surveys WHERE source_type = ? AND source_key = ?`, sourceType, sourceKey).Scan(new(int)); scanErr == nil {
			existed = true
		} else if scanErr != sql.ErrNoRows {
			err = scanErr
			return result, err
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
			strings.TrimSpace(item.Code),
			strings.TrimSpace(item.Name),
			stockResearchKind(item.Kind),
			strings.TrimSpace(item.Title),
			strings.TrimSpace(item.Institution),
			strings.TrimSpace(item.Analyst),
			strings.TrimSpace(item.Rating),
			strings.TrimSpace(item.TargetPrice),
			strings.TrimSpace(item.ResearchDate),
			strings.TrimSpace(item.PublishTime),
			strings.TrimSpace(item.SourceURL),
			sourceType,
			sourceKey,
			strings.TrimSpace(item.Summary),
			nonEmpty(strings.TrimSpace(item.RawPayload), "{}"),
			strings.TrimSpace(item.PDFURL),
			strings.TrimSpace(item.PDFFilePath),
			stockResearchPDFStatus(item.PDFStatus),
			strings.TrimSpace(item.PDFText),
			strings.TrimSpace(item.PDFError),
			strings.TrimSpace(item.PDFFetchedAt),
			strings.TrimSpace(item.PDFParsedAt),
			item.NLPScore,
			strings.TrimSpace(item.NLPRating),
			strings.TrimSpace(item.NLPReason),
			strings.TrimSpace(item.NLPScoredAt),
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

func (s *Store) ListStockResearchSurveys(ctx context.Context, filter model.StockResearchFilter) (model.StockResearchListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	result := model.StockResearchListResult{
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Code:        strings.TrimSpace(filter.Code),
		Company:     strings.TrimSpace(filter.Company),
		Institution: strings.TrimSpace(filter.Institution),
		Kind:        strings.TrimSpace(filter.Kind),
		Source:      strings.TrimSpace(filter.Source),
		Start:       strings.TrimSpace(filter.Start),
		End:         strings.TrimSpace(filter.End),
	}
	where, args := stockResearchWhere(result)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_research_surveys `+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	sources, err := s.listStockResearchSources(ctx)
	if err != nil {
		return result, err
	}
	result.Sources = sources
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, pdf_url, pdf_file_path, pdf_status, pdf_text, pdf_error, pdf_fetched_at, pdf_parsed_at, nlp_score, nlp_rating, nlp_reason, nlp_scored_at, created_at, updated_at
FROM stock_research_surveys `+where+`
ORDER BY COALESCE(NULLIF(research_date, ''), publish_time) DESC, id DESC
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	items := make([]model.StockResearchSurvey, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanStockResearchSurvey(rows)
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

func (s *Store) GetStockResearchSurvey(ctx context.Context, id int64) (model.StockResearchSurvey, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, pdf_url, pdf_file_path, pdf_status, pdf_text, pdf_error, pdf_fetched_at, pdf_parsed_at, nlp_score, nlp_rating, nlp_reason, nlp_scored_at, created_at, updated_at
FROM stock_research_surveys
WHERE id = ?`, id)
	return scanStockResearchSurvey(row)
}

func (s *Store) UpdateStockResearchPDF(ctx context.Context, id int64, update model.StockResearchPDFUpdate) (model.StockResearchSurvey, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE stock_research_surveys
SET pdf_url = ?, pdf_file_path = ?, pdf_status = ?, pdf_text = ?, pdf_error = ?, pdf_fetched_at = ?, pdf_parsed_at = ?, nlp_score = ?, nlp_rating = ?, nlp_reason = ?, nlp_scored_at = ?, updated_at = ?
WHERE id = ?`,
		strings.TrimSpace(update.PDFURL),
		strings.TrimSpace(update.PDFFilePath),
		stockResearchPDFStatus(update.PDFStatus),
		strings.TrimSpace(update.PDFText),
		strings.TrimSpace(update.PDFError),
		strings.TrimSpace(update.PDFFetchedAt),
		strings.TrimSpace(update.PDFParsedAt),
		update.NLPScore,
		strings.TrimSpace(update.NLPRating),
		strings.TrimSpace(update.NLPReason),
		strings.TrimSpace(update.NLPScoredAt),
		time.Now().UTC().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return model.StockResearchSurvey{}, err
	}
	if rows, rowErr := res.RowsAffected(); rowErr == nil && rows == 0 {
		return model.StockResearchSurvey{}, sql.ErrNoRows
	}
	return s.GetStockResearchSurvey(ctx, id)
}

func (s *Store) listStockResearchSources(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT source_type FROM stock_research_surveys WHERE source_type <> '' ORDER BY source_type ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func stockResearchWhere(filter model.StockResearchListResult) (string, []any) {
	where := "WHERE 1=1"
	args := []any{}
	if filter.Code != "" {
		where += " AND code = ?"
		args = append(args, filter.Code)
	}
	if filter.Company != "" {
		like := "%" + filter.Company + "%"
		where += " AND (name LIKE ? OR title LIKE ?)"
		args = append(args, like, like)
	}
	if filter.Institution != "" {
		like := "%" + filter.Institution + "%"
		where += " AND institution LIKE ?"
		args = append(args, like)
	}
	if filter.Kind != "" && filter.Kind != "all" {
		where += " AND kind = ?"
		args = append(args, stockResearchKind(filter.Kind))
	}
	if filter.Source != "" && filter.Source != "all" {
		where += " AND source_type = ?"
		args = append(args, filter.Source)
	}
	if filter.Start != "" {
		where += " AND COALESCE(NULLIF(research_date, ''), publish_time) >= ?"
		args = append(args, filter.Start)
	}
	if filter.End != "" {
		where += " AND COALESCE(NULLIF(research_date, ''), publish_time) <= ?"
		args = append(args, filter.End+" 23:59:59")
	}
	return where, args
}

func scanStockResearchSurvey(scanner scanner) (model.StockResearchSurvey, error) {
	var item model.StockResearchSurvey
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&item.ID,
		&item.Code,
		&item.Name,
		&item.Kind,
		&item.Title,
		&item.Institution,
		&item.Analyst,
		&item.Rating,
		&item.TargetPrice,
		&item.ResearchDate,
		&item.PublishTime,
		&item.SourceURL,
		&item.SourceType,
		&item.SourceKey,
		&item.Summary,
		&item.RawPayload,
		&item.PDFURL,
		&item.PDFFilePath,
		&item.PDFStatus,
		&item.PDFText,
		&item.PDFError,
		&item.PDFFetchedAt,
		&item.PDFParsedAt,
		&item.NLPScore,
		&item.NLPRating,
		&item.NLPReason,
		&item.NLPScoredAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.StockResearchSurvey{}, err
	}
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

func stockResearchKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "survey", "调研":
		return "survey"
	default:
		return "report"
	}
}

func stockResearchPDFStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "downloaded", "parsed", "no_pdf", "no_text", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.TrimSpace(value)
	}
}
