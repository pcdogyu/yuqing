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
INSERT INTO stock_research_surveys (code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
SELECT id, code, name, kind, title, institution, analyst, rating, target_price, research_date, publish_time, source_url, source_type, source_key, summary, raw_payload, created_at, updated_at
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
