package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) ListCrawlTemplates(ctx context.Context) ([]model.CrawlTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, website, source_type, enabled, config_json, created_at, updated_at FROM crawl_templates ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.CrawlTemplate, 0)
	for rows.Next() {
		tpl, err := scanCrawlTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tpl)
	}
	return out, rows.Err()
}

func (s *Store) GetCrawlTemplate(ctx context.Context, id int64) (model.CrawlTemplate, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, website, source_type, enabled, config_json, created_at, updated_at FROM crawl_templates WHERE id = ?`, id)
	tpl, err := scanCrawlTemplate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.CrawlTemplate{}, ErrNotFound
		}
		return model.CrawlTemplate{}, err
	}
	return tpl, nil
}

func (s *Store) CreateCrawlTemplate(ctx context.Context, tpl model.CrawlTemplate) (model.CrawlTemplate, error) {
	now := time.Now().UTC()
	tpl.Name = strings.TrimSpace(tpl.Name)
	tpl.Website = strings.TrimSpace(tpl.Website)
	tpl.SourceType = nonEmpty(tpl.SourceType, "custom")
	if tpl.ConfigJSON == "" {
		tpl.ConfigJSON = "{}"
	}
	tpl.Enabled = true
	res, err := s.db.ExecContext(ctx, `INSERT INTO crawl_templates (name, website, source_type, enabled, config_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tpl.Name, tpl.Website, tpl.SourceType, boolToInt(tpl.Enabled), tpl.ConfigJSON, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return model.CrawlTemplate{}, err
	}
	tpl.ID, _ = res.LastInsertId()
	return s.GetCrawlTemplate(ctx, tpl.ID)
}

func (s *Store) UpdateCrawlTemplate(ctx context.Context, tpl model.CrawlTemplate) (model.CrawlTemplate, error) {
	tpl.Name = strings.TrimSpace(tpl.Name)
	tpl.Website = strings.TrimSpace(tpl.Website)
	tpl.SourceType = nonEmpty(tpl.SourceType, "custom")
	if tpl.ConfigJSON == "" {
		tpl.ConfigJSON = "{}"
	}
	tpl.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE crawl_templates SET name = ?, website = ?, source_type = ?, enabled = ?, config_json = ?, updated_at = ? WHERE id = ?`,
		tpl.Name, tpl.Website, tpl.SourceType, boolToInt(tpl.Enabled), tpl.ConfigJSON, tpl.UpdatedAt.Format(time.RFC3339), tpl.ID)
	if err != nil {
		return model.CrawlTemplate{}, err
	}
	return s.GetCrawlTemplate(ctx, tpl.ID)
}

func (s *Store) DeleteCrawlTemplate(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM crawl_templates WHERE id = ?`, id)
	return err
}

func scanCrawlTemplate(scanner scanner) (model.CrawlTemplate, error) {
	var tpl model.CrawlTemplate
	var enabled int
	var createdAt, updatedAt string
	if err := scanner.Scan(&tpl.ID, &tpl.Name, &tpl.Website, &tpl.SourceType, &enabled, &tpl.ConfigJSON, &createdAt, &updatedAt); err != nil {
		return model.CrawlTemplate{}, err
	}
	tpl.Enabled = enabled != 0
	tpl.CreatedAt = mustParseRFC3339(createdAt)
	tpl.UpdatedAt = mustParseRFC3339(updatedAt)
	return tpl, nil
}
