package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const stockResearchSelectColumns = "id, code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, source_text, source_fetch_status, source_fetch_error, source_fetched_at, pdf_url, pdf_file_path, pdf_status, pdf_text, pdf_error, pdf_fetched_at, pdf_parsed_at, nlp_score, nlp_rating, nlp_reason, nlp_scored_at, created_at, updated_at"

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
INSERT INTO stock_research_surveys (code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, source_text, source_fetch_status, source_fetch_error, source_fetched_at, pdf_url, pdf_file_path, pdf_status, pdf_text, pdf_error, pdf_fetched_at, pdf_parsed_at, nlp_score, nlp_rating, nlp_reason, nlp_scored_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	source_text = CASE WHEN stock_research_surveys.source_text = '' AND excluded.source_text <> '' THEN excluded.source_text ELSE stock_research_surveys.source_text END,
	source_fetch_status = CASE WHEN stock_research_surveys.source_text = '' AND excluded.source_fetch_status <> '' THEN excluded.source_fetch_status ELSE stock_research_surveys.source_fetch_status END,
	source_fetch_error = CASE WHEN stock_research_surveys.source_text = '' AND excluded.source_fetch_error <> '' THEN excluded.source_fetch_error ELSE stock_research_surveys.source_fetch_error END,
	source_fetched_at = CASE WHEN stock_research_surveys.source_text = '' AND excluded.source_fetched_at <> '' THEN excluded.source_fetched_at ELSE stock_research_surveys.source_fetched_at END,
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
		item.Code = strings.TrimSpace(item.Code)
		item.Name = strings.TrimSpace(item.Name)
		item.Kind = stockResearchKind(item.Kind)
		item.Title = strings.TrimSpace(item.Title)
		item.Institution = strings.TrimSpace(item.Institution)
		item.Analyst = strings.TrimSpace(item.Analyst)
		item.Rating = strings.TrimSpace(item.Rating)
		item.TargetPrice = strings.TrimSpace(item.TargetPrice)
		item.ResearchDate = strings.TrimSpace(item.ResearchDate)
		item.PublishTime = strings.TrimSpace(item.PublishTime)
		item.SourceURL = strings.TrimSpace(item.SourceURL)
		item.SourceType = strings.TrimSpace(item.SourceType)
		item.SourceKey = strings.TrimSpace(item.SourceKey)
		item.Summary = strings.TrimSpace(item.Summary)
		item.RawPayload = nonEmpty(strings.TrimSpace(item.RawPayload), "{}")
		item.SourceText = strings.TrimSpace(item.SourceText)
		item.SourceFetchStatus = stockResearchSourceStatus(item.SourceFetchStatus)
		item.SourceFetchError = strings.TrimSpace(item.SourceFetchError)
		item.SourceFetchedAt = strings.TrimSpace(item.SourceFetchedAt)
		item.PDFURL = strings.TrimSpace(item.PDFURL)
		item.PDFFilePath = strings.TrimSpace(item.PDFFilePath)
		item.PDFStatus = stockResearchPDFStatus(item.PDFStatus)
		item.PDFText = strings.TrimSpace(item.PDFText)
		item.PDFError = strings.TrimSpace(item.PDFError)
		item.PDFFetchedAt = strings.TrimSpace(item.PDFFetchedAt)
		item.PDFParsedAt = strings.TrimSpace(item.PDFParsedAt)
		item.NLPRating = strings.TrimSpace(item.NLPRating)
		item.NLPReason = strings.TrimSpace(item.NLPReason)
		item.NLPScoredAt = strings.TrimSpace(item.NLPScoredAt)
		sourceType := item.SourceType
		sourceKey := item.SourceKey
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
		item.CreatedAt = createdAt
		item.UpdatedAt = updatedAt
		if !existed {
			equivalent, equivalentOK, equivalentErr := s.findEquivalentStockResearchSurveyTx(ctx, tx, item)
			if equivalentErr != nil {
				err = equivalentErr
				return result, err
			}
			if equivalentOK {
				if err = s.updateEquivalentStockResearchSurveyTx(ctx, tx, equivalent, item, updatedAt); err != nil {
					return result, err
				}
				if err = s.collapseEquivalentStockResearchSurveysTx(ctx, tx, item); err != nil {
					return result, err
				}
				result.Updated++
				continue
			}
		}
		if _, err = stmt.ExecContext(ctx,
			item.Code,
			item.Name,
			item.Kind,
			item.Title,
			item.Institution,
			item.Analyst,
			item.Rating,
			item.TargetPrice,
			item.ResearchDate,
			item.PublishTime,
			item.SourceURL,
			sourceType,
			sourceKey,
			item.Summary,
			item.RawPayload,
			item.SourceText,
			item.SourceFetchStatus,
			item.SourceFetchError,
			item.SourceFetchedAt,
			item.PDFURL,
			item.PDFFilePath,
			item.PDFStatus,
			item.PDFText,
			item.PDFError,
			item.PDFFetchedAt,
			item.PDFParsedAt,
			item.NLPScore,
			item.NLPRating,
			item.NLPReason,
			item.NLPScoredAt,
			createdAt.UTC().Format(time.RFC3339),
			updatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return result, err
		}
		if err = s.collapseEquivalentStockResearchSurveysTx(ctx, tx, item); err != nil {
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

func (s *Store) findEquivalentStockResearchSurveyTx(ctx context.Context, tx *Tx, item model.StockResearchSurvey) (model.StockResearchSurvey, bool, error) {
	key := stockResearchEquivalentKey(item)
	if key == "" {
		return model.StockResearchSurvey{}, false, nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT `+stockResearchSelectColumns+`
FROM stock_research_surveys
WHERE kind = ? AND code = ? AND COALESCE(NULLIF(research_date, ''), publish_time) >= ? AND COALESCE(NULLIF(research_date, ''), publish_time) <= ?`,
		item.Kind, item.Code, stockResearchEquivalentDate(item), stockResearchEquivalentDate(item)+" 23:59:59")
	if err != nil {
		return model.StockResearchSurvey{}, false, err
	}
	defer rows.Close()
	var found model.StockResearchSurvey
	for rows.Next() {
		candidate, scanErr := scanStockResearchSurvey(rows)
		if scanErr != nil {
			return model.StockResearchSurvey{}, false, scanErr
		}
		if stockResearchEquivalentKey(candidate) != key {
			continue
		}
		if found.ID == 0 || stockResearchSourceRank(candidate) > stockResearchSourceRank(found) {
			found = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return model.StockResearchSurvey{}, false, err
	}
	return found, found.ID > 0, nil
}

func (s *Store) updateEquivalentStockResearchSurveyTx(ctx context.Context, tx *Tx, existing, incoming model.StockResearchSurvey, updatedAt time.Time) error {
	preferred := mergePreferredStockResearch(existing, incoming)
	if preferred.CreatedAt.IsZero() {
		preferred.CreatedAt = existing.CreatedAt
	}
	if preferred.UpdatedAt.IsZero() || updatedAt.After(preferred.UpdatedAt) {
		preferred.UpdatedAt = updatedAt
	}
	_, err := tx.ExecContext(ctx, `
UPDATE stock_research_surveys
SET code = ?, name = ?, kind = ?, title = ?, institution = ?, analyst = ?, rating = ?, target_price = ?, research_date = ?, publish_time = ?, source_url = ?, source_type = ?, source_key = ?, summary = ?, raw_payload = ?,
	source_text = CASE WHEN source_text = '' AND ? <> '' THEN ? ELSE source_text END,
	source_fetch_status = CASE WHEN source_text = '' AND ? <> '' THEN ? ELSE source_fetch_status END,
	source_fetch_error = CASE WHEN source_text = '' AND ? <> '' THEN ? ELSE source_fetch_error END,
	source_fetched_at = CASE WHEN source_text = '' AND ? <> '' THEN ? ELSE source_fetched_at END,
	pdf_url = CASE WHEN ? <> '' THEN ? ELSE pdf_url END,
	pdf_file_path = CASE WHEN ? <> '' THEN ? ELSE pdf_file_path END,
	pdf_status = CASE WHEN ? <> '' THEN ? ELSE pdf_status END,
	pdf_text = CASE WHEN ? <> '' THEN ? ELSE pdf_text END,
	pdf_error = CASE WHEN ? <> '' THEN ? ELSE pdf_error END,
	pdf_fetched_at = CASE WHEN ? <> '' THEN ? ELSE pdf_fetched_at END,
	pdf_parsed_at = CASE WHEN ? <> '' THEN ? ELSE pdf_parsed_at END,
	nlp_score = CASE WHEN ? <> '' THEN ? ELSE nlp_score END,
	nlp_rating = CASE WHEN ? <> '' THEN ? ELSE nlp_rating END,
	nlp_reason = CASE WHEN ? <> '' THEN ? ELSE nlp_reason END,
	nlp_scored_at = CASE WHEN ? <> '' THEN ? ELSE nlp_scored_at END,
	updated_at = ?
WHERE id = ?`,
		preferred.Code,
		preferred.Name,
		preferred.Kind,
		preferred.Title,
		preferred.Institution,
		preferred.Analyst,
		preferred.Rating,
		preferred.TargetPrice,
		preferred.ResearchDate,
		preferred.PublishTime,
		preferred.SourceURL,
		preferred.SourceType,
		preferred.SourceKey,
		preferred.Summary,
		preferred.RawPayload,
		incoming.SourceText, incoming.SourceText,
		incoming.SourceFetchStatus, incoming.SourceFetchStatus,
		incoming.SourceFetchError, incoming.SourceFetchError,
		incoming.SourceFetchedAt, incoming.SourceFetchedAt,
		incoming.PDFURL, incoming.PDFURL,
		incoming.PDFFilePath, incoming.PDFFilePath,
		incoming.PDFStatus, incoming.PDFStatus,
		incoming.PDFText, incoming.PDFText,
		incoming.PDFError, incoming.PDFError,
		incoming.PDFFetchedAt, incoming.PDFFetchedAt,
		incoming.PDFParsedAt, incoming.PDFParsedAt,
		incoming.NLPScoredAt, incoming.NLPScore,
		incoming.NLPRating, incoming.NLPRating,
		incoming.NLPReason, incoming.NLPReason,
		incoming.NLPScoredAt, incoming.NLPScoredAt,
		preferred.UpdatedAt.UTC().Format(time.RFC3339),
		existing.ID,
	)
	return err
}

func (s *Store) collapseEquivalentStockResearchSurveysTx(ctx context.Context, tx *Tx, item model.StockResearchSurvey) error {
	key := stockResearchEquivalentKey(item)
	if key == "" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT `+stockResearchSelectColumns+`
FROM stock_research_surveys
WHERE kind = ? AND code = ? AND COALESCE(NULLIF(research_date, ''), publish_time) >= ? AND COALESCE(NULLIF(research_date, ''), publish_time) <= ?`,
		item.Kind, item.Code, stockResearchEquivalentDate(item), stockResearchEquivalentDate(item)+" 23:59:59")
	if err != nil {
		return err
	}
	candidates := make([]model.StockResearchSurvey, 0)
	for rows.Next() {
		candidate, scanErr := scanStockResearchSurvey(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		if stockResearchEquivalentKey(candidate) == key {
			candidates = append(candidates, candidate)
		}
	}
	if rowErr := rows.Err(); rowErr != nil {
		rows.Close()
		return rowErr
	}
	rows.Close()
	if len(candidates) < 2 {
		return nil
	}
	winner := candidates[0]
	for _, candidate := range candidates[1:] {
		if stockResearchSourceRank(candidate) > stockResearchSourceRank(winner) {
			winner = candidate
		}
	}
	for _, candidate := range candidates {
		if candidate.ID == winner.ID {
			continue
		}
		if err := mergeStockResearchRowFieldsTx(ctx, tx, winner.ID, candidate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM stock_research_surveys WHERE id = ?`, candidate.ID); err != nil {
			return err
		}
	}
	return nil
}

func mergeStockResearchRowFieldsTx(ctx context.Context, tx *Tx, winnerID int64, loser model.StockResearchSurvey) error {
	_, err := tx.ExecContext(ctx, `
UPDATE stock_research_surveys
SET source_text = CASE WHEN source_text = '' THEN ? ELSE source_text END,
	source_fetch_status = CASE WHEN source_fetch_status = '' THEN ? ELSE source_fetch_status END,
	source_fetch_error = CASE WHEN source_fetch_error = '' THEN ? ELSE source_fetch_error END,
	source_fetched_at = CASE WHEN source_fetched_at = '' THEN ? ELSE source_fetched_at END,
	pdf_url = CASE WHEN pdf_url = '' THEN ? ELSE pdf_url END,
	pdf_file_path = CASE WHEN pdf_file_path = '' THEN ? ELSE pdf_file_path END,
	pdf_status = CASE WHEN pdf_status = '' THEN ? ELSE pdf_status END,
	pdf_text = CASE WHEN pdf_text = '' THEN ? ELSE pdf_text END,
	pdf_error = CASE WHEN pdf_error = '' THEN ? ELSE pdf_error END,
	pdf_fetched_at = CASE WHEN pdf_fetched_at = '' THEN ? ELSE pdf_fetched_at END,
	pdf_parsed_at = CASE WHEN pdf_parsed_at = '' THEN ? ELSE pdf_parsed_at END,
	nlp_score = CASE WHEN nlp_scored_at = '' THEN ? ELSE nlp_score END,
	nlp_rating = CASE WHEN nlp_rating = '' THEN ? ELSE nlp_rating END,
	nlp_reason = CASE WHEN nlp_reason = '' THEN ? ELSE nlp_reason END,
	nlp_scored_at = CASE WHEN nlp_scored_at = '' THEN ? ELSE nlp_scored_at END,
	updated_at = ?
WHERE id = ?`,
		loser.SourceText,
		loser.SourceFetchStatus,
		loser.SourceFetchError,
		loser.SourceFetchedAt,
		loser.PDFURL,
		loser.PDFFilePath,
		loser.PDFStatus,
		loser.PDFText,
		loser.PDFError,
		loser.PDFFetchedAt,
		loser.PDFParsedAt,
		loser.NLPScore,
		loser.NLPRating,
		loser.NLPReason,
		loser.NLPScoredAt,
		time.Now().UTC().Format(time.RFC3339),
		winnerID,
	)
	return err
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
SELECT `+stockResearchSelectColumns+`
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
SELECT `+stockResearchSelectColumns+`
FROM stock_research_surveys
WHERE id = ?`, id)
	return scanStockResearchSurvey(row)
}

func (s *Store) UpdateStockResearchPDF(ctx context.Context, id int64, update model.StockResearchPDFUpdate) (model.StockResearchSurvey, error) {
	force := update.Force
	sourceText := strings.TrimSpace(update.SourceText)
	sourceStatus := stockResearchSourceStatus(update.SourceFetchStatus)
	sourceError := strings.TrimSpace(update.SourceFetchError)
	sourceFetchedAt := strings.TrimSpace(update.SourceFetchedAt)
	if sourceText == "" && stockResearchPDFStatus(update.PDFStatus) == "parsed" && strings.TrimSpace(update.PDFText) != "" {
		sourceText = strings.TrimSpace(update.PDFText)
		sourceStatus = "parsed"
		sourceError = ""
		sourceFetchedAt = nonEmpty(sourceFetchedAt, strings.TrimSpace(update.PDFParsedAt), strings.TrimSpace(update.PDFFetchedAt))
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE stock_research_surveys
SET pdf_url = ?, pdf_file_path = ?, pdf_status = ?, pdf_text = ?, pdf_error = ?, pdf_fetched_at = ?, pdf_parsed_at = ?, nlp_score = ?, nlp_rating = ?, nlp_reason = ?, nlp_scored_at = ?,
	source_text = CASE WHEN ? <> '' AND (? OR source_text = '' OR source_fetch_status = 'pending_pdf') THEN ? ELSE source_text END,
	source_fetch_status = CASE WHEN ? <> '' AND (? OR source_text = '' OR source_fetch_status = 'pending_pdf') THEN ? ELSE source_fetch_status END,
	source_fetch_error = CASE WHEN ? <> '' AND (? OR source_text = '' OR source_fetch_status = 'pending_pdf') THEN ? ELSE source_fetch_error END,
	source_fetched_at = CASE WHEN ? <> '' AND (? OR source_text = '' OR source_fetch_status = 'pending_pdf') THEN ? ELSE source_fetched_at END,
	updated_at = ?
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
		sourceText, force, sourceText,
		sourceStatus, force, sourceStatus,
		sourceError, force, sourceError,
		sourceFetchedAt, force, sourceFetchedAt,
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

func (s *Store) UpdateStockResearchSource(ctx context.Context, id int64, update model.StockResearchSourceUpdate) (model.StockResearchSurvey, error) {
	existing, err := s.GetStockResearchSurvey(ctx, id)
	if err != nil {
		return model.StockResearchSurvey{}, err
	}
	if !update.Force && strings.TrimSpace(existing.SourceText) != "" {
		return existing, nil
	}
	sourceText := strings.TrimSpace(update.SourceText)
	if update.Force && sourceText == "" && strings.TrimSpace(existing.SourceText) != "" {
		sourceText = strings.TrimSpace(existing.SourceText)
	}
	status := stockResearchSourceStatus(update.SourceFetchStatus)
	if status == "" {
		if sourceText != "" {
			status = "parsed"
		} else {
			status = "failed"
		}
	}
	fetchedAt := strings.TrimSpace(update.SourceFetchedAt)
	if fetchedAt == "" {
		fetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE stock_research_surveys
SET source_text = ?, source_fetch_status = ?, source_fetch_error = ?, source_fetched_at = ?, updated_at = ?
WHERE id = ?`,
		sourceText,
		status,
		strings.TrimSpace(update.SourceFetchError),
		fetchedAt,
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
		&item.SourceText,
		&item.SourceFetchStatus,
		&item.SourceFetchError,
		&item.SourceFetchedAt,
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

func stockResearchSourceStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "parsed", "no_source", "failed", "pending_pdf":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.TrimSpace(value)
	}
}

func mergePreferredStockResearch(current, incoming model.StockResearchSurvey) model.StockResearchSurvey {
	preferred, fallback := current, incoming
	if stockResearchSourceRank(incoming) > stockResearchSourceRank(current) {
		preferred, fallback = incoming, current
		preferred.ID = current.ID
	}
	if strings.TrimSpace(preferred.Code) == "" {
		preferred.Code = fallback.Code
	}
	if strings.TrimSpace(preferred.Name) == "" {
		preferred.Name = fallback.Name
	}
	if strings.TrimSpace(preferred.Kind) == "" {
		preferred.Kind = fallback.Kind
	}
	if strings.TrimSpace(preferred.Title) == "" {
		preferred.Title = fallback.Title
	}
	if strings.TrimSpace(preferred.Institution) == "" {
		preferred.Institution = fallback.Institution
	}
	if strings.TrimSpace(preferred.Analyst) == "" {
		preferred.Analyst = fallback.Analyst
	}
	if strings.TrimSpace(preferred.Rating) == "" {
		preferred.Rating = fallback.Rating
	}
	if strings.TrimSpace(preferred.TargetPrice) == "" {
		preferred.TargetPrice = fallback.TargetPrice
	}
	if strings.TrimSpace(preferred.ResearchDate) == "" {
		preferred.ResearchDate = fallback.ResearchDate
	}
	if strings.TrimSpace(preferred.PublishTime) == "" {
		preferred.PublishTime = fallback.PublishTime
	}
	if strings.TrimSpace(preferred.SourceURL) == "" {
		preferred.SourceURL = fallback.SourceURL
	}
	if strings.TrimSpace(preferred.Summary) == "" {
		preferred.Summary = fallback.Summary
	}
	if strings.TrimSpace(preferred.RawPayload) == "" || strings.TrimSpace(preferred.RawPayload) == "{}" {
		preferred.RawPayload = fallback.RawPayload
	}
	if strings.TrimSpace(preferred.SourceText) == "" {
		preferred.SourceText = fallback.SourceText
	}
	if strings.TrimSpace(preferred.SourceFetchStatus) == "" {
		preferred.SourceFetchStatus = fallback.SourceFetchStatus
	}
	if strings.TrimSpace(preferred.SourceFetchError) == "" {
		preferred.SourceFetchError = fallback.SourceFetchError
	}
	if strings.TrimSpace(preferred.SourceFetchedAt) == "" {
		preferred.SourceFetchedAt = fallback.SourceFetchedAt
	}
	if strings.TrimSpace(preferred.PDFURL) == "" {
		preferred.PDFURL = fallback.PDFURL
	}
	if strings.TrimSpace(preferred.PDFFilePath) == "" {
		preferred.PDFFilePath = fallback.PDFFilePath
	}
	if strings.TrimSpace(preferred.PDFStatus) == "" {
		preferred.PDFStatus = fallback.PDFStatus
	}
	if strings.TrimSpace(preferred.PDFText) == "" {
		preferred.PDFText = fallback.PDFText
	}
	if strings.TrimSpace(preferred.PDFError) == "" {
		preferred.PDFError = fallback.PDFError
	}
	if strings.TrimSpace(preferred.PDFFetchedAt) == "" {
		preferred.PDFFetchedAt = fallback.PDFFetchedAt
	}
	if strings.TrimSpace(preferred.PDFParsedAt) == "" {
		preferred.PDFParsedAt = fallback.PDFParsedAt
	}
	if preferred.NLPScore == 0 {
		preferred.NLPScore = fallback.NLPScore
	}
	if strings.TrimSpace(preferred.NLPRating) == "" {
		preferred.NLPRating = fallback.NLPRating
	}
	if strings.TrimSpace(preferred.NLPReason) == "" {
		preferred.NLPReason = fallback.NLPReason
	}
	if strings.TrimSpace(preferred.NLPScoredAt) == "" {
		preferred.NLPScoredAt = fallback.NLPScoredAt
	}
	return preferred
}

func stockResearchSourceRank(item model.StockResearchSurvey) int {
	switch strings.TrimSpace(item.SourceType) {
	case "eastmoney_report":
		if strings.HasPrefix(strings.TrimSpace(item.SourceKey), "AP") || strings.TrimSpace(item.PDFURL) != "" {
			return 40
		}
		return 35
	case "akshare_stock_research":
		return 30
	case "sina_finance_report":
		return 20
	case "sohu_finance_report":
		return 10
	default:
		return 0
	}
}

func stockResearchEquivalentKey(item model.StockResearchSurvey) string {
	if stockResearchKind(item.Kind) != "report" {
		return ""
	}
	code := strings.TrimSpace(item.Code)
	date := stockResearchEquivalentDate(item)
	institution := stockResearchComparableText(item.Institution)
	title := stockResearchComparableTitle(item)
	if code == "" || date == "" || institution == "" || title == "" {
		return ""
	}
	return strings.Join([]string{code, date, institution, title}, "|")
}

func stockResearchEquivalentDate(item model.StockResearchSurvey) string {
	date := strings.TrimSpace(item.ResearchDate)
	if date == "" {
		date = strings.TrimSpace(item.PublishTime)
	}
	if idx := strings.Index(date, " "); idx > 0 && strings.Contains(date[:idx], "-") {
		date = date[:idx]
	}
	return date
}

func stockResearchComparableTitle(item model.StockResearchSurvey) string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return ""
	}
	colonIdx, colonSize := stockResearchComparableTitleColonIndex(title)
	if colonIdx >= 0 {
		prefix := title[:colonIdx]
		if strings.Contains(prefix, strings.TrimSpace(item.Code)) || strings.Contains(prefix, strings.TrimSpace(item.Name)) || stockResearchTitleHasStockCode(prefix) {
			title = title[colonIdx+colonSize:]
		}
	}
	return stockResearchComparableText(title)
}

func stockResearchComparableTitleColonIndex(title string) (int, int) {
	half := strings.Index(title, ":")
	full := strings.Index(title, "：")
	switch {
	case half < 0:
		if full < 0 {
			return -1, 0
		}
		return full, len("：")
	case full < 0 || half < full:
		return half, len(":")
	default:
		return full, len("：")
	}
}

func stockResearchTitleHasStockCode(value string) bool {
	for i := 0; i+6 <= len(value); i++ {
		part := value[i : i+6]
		allDigits := true
		for _, r := range part {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return true
		}
	}
	return false
}

func stockResearchComparableText(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
