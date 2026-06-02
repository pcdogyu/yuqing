package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	schema := `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS items (
	id INTEGER PRIMARY KEY,
	source_type TEXT NOT NULL,
	source_key TEXT NOT NULL UNIQUE,
	title TEXT NOT NULL,
	content TEXT,
	summary TEXT,
	publish_time TEXT,
	publish_time_text TEXT,
	detail_url TEXT,
	source_url TEXT NOT NULL,
	tag_flags TEXT,
	from_text TEXT,
	external_source_host TEXT,
	is_vip INTEGER NOT NULL DEFAULT 0,
	has_image INTEGER NOT NULL DEFAULT 0,
	raw_payload TEXT,
	captured_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS crawl_runs (
	id INTEGER PRIMARY KEY,
	source_type TEXT NOT NULL,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	status TEXT NOT NULL,
	fetched_count INTEGER NOT NULL DEFAULT 0,
	inserted_count INTEGER NOT NULL DEFAULT 0,
	updated_count INTEGER NOT NULL DEFAULT 0,
	error_text TEXT
);

CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(
	title,
	content,
	summary,
	content='items',
	content_rowid='id',
	tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS items_ai AFTER INSERT ON items BEGIN
  INSERT INTO items_fts(rowid, title, content, summary) VALUES (new.id, new.title, new.content, new.summary);
END;

CREATE TRIGGER IF NOT EXISTS items_ad AFTER DELETE ON items BEGIN
  INSERT INTO items_fts(items_fts, rowid, title, content, summary) VALUES('delete', old.id, old.title, old.content, old.summary);
END;

CREATE TRIGGER IF NOT EXISTS items_au AFTER UPDATE ON items BEGIN
  INSERT INTO items_fts(items_fts, rowid, title, content, summary) VALUES('delete', old.id, old.title, old.content, old.summary);
  INSERT INTO items_fts(rowid, title, content, summary) VALUES (new.id, new.title, new.content, new.summary);
END;

CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT 'admin',
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS api_tokens (
	token TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL,
	last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS captchas (
	id TEXT PRIMARY KEY,
	code TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_groups (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_members (
	id INTEGER PRIMARY KEY,
	project_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	role TEXT NOT NULL DEFAULT 'editor',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
	id INTEGER PRIMARY KEY,
	group_id INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	keywords TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor_rules (
	id INTEGER PRIMARY KEY,
	project_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	include_keywords TEXT NOT NULL DEFAULT '',
	exclude_keywords TEXT NOT NULL DEFAULT '',
	channels TEXT NOT NULL DEFAULT '',
	severity TEXT NOT NULL DEFAULT 'medium',
	status TEXT NOT NULL DEFAULT 'active',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS item_relations (
	item_id INTEGER NOT NULL,
	project_id INTEGER NOT NULL,
	rule_id INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	PRIMARY KEY (item_id, project_id, rule_id)
);

CREATE TABLE IF NOT EXISTS item_tags (
	item_id INTEGER NOT NULL,
	tag TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (item_id, tag)
);

CREATE TABLE IF NOT EXISTS favorites (
	user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id)
);

CREATE TABLE IF NOT EXISTS item_reads (
	user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id)
);

CREATE TABLE IF NOT EXISTS reports (
	id INTEGER PRIMARY KEY,
	project_id INTEGER NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'draft',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS report_sections (
	id INTEGER PRIMARY KEY,
	report_id INTEGER NOT NULL,
	heading TEXT NOT NULL,
	content TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS analysis_snapshots (
	id INTEGER PRIMARY KEY,
	scope TEXT NOT NULL,
	scope_id INTEGER NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	payload TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE(scope, scope_id)
);

CREATE TABLE IF NOT EXISTS trend_points (
	id INTEGER PRIMARY KEY,
	scope TEXT NOT NULL DEFAULT 'system',
	scope_id INTEGER NOT NULL DEFAULT 0,
	label TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS keyword_hotspots (
	id INTEGER PRIMARY KEY,
	scope TEXT NOT NULL DEFAULT 'system',
	scope_id INTEGER NOT NULL DEFAULT 0,
	keyword TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS source_breakdowns (
	id INTEGER PRIMARY KEY,
	scope TEXT NOT NULL DEFAULT 'system',
	scope_id INTEGER NOT NULL DEFAULT 0,
	source_type TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS system_notices (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS feedback (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_preferences (
	user_id INTEGER PRIMARY KEY,
	language TEXT NOT NULL DEFAULT 'zh-CN',
	theme TEXT NOT NULL DEFAULT 'light',
	default_search_mode TEXT NOT NULL DEFAULT 'default',
	article_page_size INTEGER NOT NULL DEFAULT 20,
	email_notifications INTEGER NOT NULL DEFAULT 1,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS popup_states (
	user_id INTEGER NOT NULL,
	popup_key TEXT NOT NULL,
	dismissed INTEGER NOT NULL DEFAULT 0,
	dismissed_at TEXT,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (user_id, popup_key)
);

CREATE TABLE IF NOT EXISTS mail_configs (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	enabled INTEGER NOT NULL DEFAULT 0,
	smtp_host TEXT NOT NULL DEFAULT '',
	smtp_port INTEGER NOT NULL DEFAULT 25,
	username TEXT NOT NULL DEFAULT '',
	password TEXT NOT NULL DEFAULT '',
	sender_name TEXT NOT NULL DEFAULT '',
	sender_email TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS warning_settings (
	project_id INTEGER PRIMARY KEY,
	enabled INTEGER NOT NULL DEFAULT 1,
	channels TEXT NOT NULL DEFAULT '',
	threshold INTEGER NOT NULL DEFAULT 80,
	recipients TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS item_shares (
	user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	channel TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id, channel)
);

CREATE TABLE IF NOT EXISTS task_runs (
	id INTEGER PRIMARY KEY,
	task_name TEXT NOT NULL,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_items_source_type_captured_at ON items(source_type, captured_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_items_title ON items(title);
CREATE INDEX IF NOT EXISTS idx_crawl_runs_source_type_started_at ON crawl_runs(source_type, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_projects_group_id ON projects(group_id);
CREATE INDEX IF NOT EXISTS idx_monitor_rules_project_id ON monitor_rules(project_id);
CREATE INDEX IF NOT EXISTS idx_reports_project_id ON reports(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_captchas_expires_at ON captchas(expires_at);
CREATE INDEX IF NOT EXISTS idx_item_relations_project_id ON item_relations(project_id, item_id DESC);
CREATE INDEX IF NOT EXISTS idx_trend_points_scope ON trend_points(scope, scope_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_hotspots_scope ON keyword_hotspots(scope, scope_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_breakdowns_scope ON source_breakdowns(scope, scope_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_popup_states_user_id ON popup_states(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_item_shares_item_id ON item_shares(item_id, created_at DESC);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE monitor_rules ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`)
	return nil
}

func (s *Store) StartCrawlRun(ctx context.Context, sourceType string, startedAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO crawl_runs (source_type, started_at, status, fetched_count, inserted_count, updated_count) VALUES (?, ?, 'running', 0, 0, 0)`,
		sourceType, startedAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishCrawlRun(ctx context.Context, runID int64, status string, fetched, inserted, updated int, errText string, finishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawl_runs SET finished_at = ?, status = ?, fetched_count = ?, inserted_count = ?, updated_count = ?, error_text = ? WHERE id = ?`,
		finishedAt.Format(time.RFC3339), status, fetched, inserted, updated, errText, runID,
	)
	return err
}

func (s *Store) UpsertItems(ctx context.Context, items []model.Item) (inserted, updated int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	query := `
INSERT INTO items (
	source_type, source_key, title, content, summary, publish_time, publish_time_text, detail_url,
	source_url, tag_flags, from_text, external_source_host, is_vip, has_image, raw_payload,
	captured_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_key) DO UPDATE SET
	title = excluded.title,
	content = excluded.content,
	summary = excluded.summary,
	publish_time = excluded.publish_time,
	publish_time_text = excluded.publish_time_text,
	detail_url = excluded.detail_url,
	source_url = excluded.source_url,
	tag_flags = excluded.tag_flags,
	from_text = excluded.from_text,
	external_source_host = excluded.external_source_host,
	is_vip = excluded.is_vip,
	has_image = excluded.has_image,
	raw_payload = excluded.raw_payload,
	captured_at = excluded.captured_at,
	updated_at = excluded.updated_at
`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()

	for _, item := range items {
		now := item.UpdatedAt.Format(time.RFC3339)
		if item.CreatedAt.IsZero() {
			item.CreatedAt = item.UpdatedAt
		}

		existed, errCheck := s.sourceKeyExistsTx(ctx, tx, item.SourceKey)
		if errCheck != nil {
			err = errCheck
			return 0, 0, err
		}

		_, execErr := stmt.ExecContext(ctx,
			item.SourceType,
			item.SourceKey,
			nonEmpty(item.Title, item.Content, item.Summary, "(untitled)"),
			item.Content,
			item.Summary,
			item.PublishTime,
			item.PublishTimeText,
			item.DetailURL,
			item.SourceURL,
			item.TagFlags,
			item.FromText,
			item.ExternalSourceHost,
			boolToInt(item.IsVIP),
			boolToInt(item.HasImage),
			item.RawPayload,
			item.CapturedAt.Format(time.RFC3339),
			item.CreatedAt.Format(time.RFC3339),
			now,
		)
		if execErr != nil {
			err = execErr
			return 0, 0, err
		}

		if existed {
			updated++
		} else {
			inserted++
		}
	}

	err = tx.Commit()
	return inserted, updated, err
}

func (s *Store) ListItems(ctx context.Context, filter model.ArticleFilter) (model.ItemListResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	offset := (filter.Page - 1) * filter.PageSize

	where, args, joins := buildItemFilter(filter)
	countQuery := "SELECT COUNT(DISTINCT items.id) FROM items " + joins + " " + where

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return model.ItemListResult{}, err
	}

	query := "SELECT DISTINCT items.id, items.source_type, items.source_key, items.title, items.content, items.summary, items.publish_time, items.publish_time_text, items.detail_url, items.source_url, items.tag_flags, items.from_text, items.external_source_host, items.is_vip, items.has_image, items.raw_payload, items.captured_at, items.created_at, items.updated_at FROM items " + joins + " " + where + " ORDER BY items.captured_at DESC, items.id DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return model.ItemListResult{}, err
	}
	defer rows.Close()

	items, err := scanItems(rows)
	if err != nil {
		return model.ItemListResult{}, err
	}
	if err := s.attachProjectIDs(ctx, items); err != nil {
		return model.ItemListResult{}, err
	}
	if filter.UserID > 0 {
		if err := s.attachUserState(ctx, filter.UserID, items); err != nil {
			return model.ItemListResult{}, err
		}
	}

	return model.ItemListResult{
		Items:    items,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Total:    total,
	}, nil
}

func (s *Store) LatestItems(ctx context.Context, limit int, sourceType string) ([]model.Item, error) {
	limit = max(limit, 1)
	filter := model.ArticleFilter{Page: 1, PageSize: limit, SourceType: sourceType}
	list, err := s.ListItems(ctx, filter)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *Store) GetItem(ctx context.Context, id int64) (model.Item, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, source_type, source_key, title, content, summary, publish_time, publish_time_text, detail_url, source_url, tag_flags, from_text, external_source_host, is_vip, has_image, raw_payload, captured_at, created_at, updated_at FROM items WHERE id = ?`, id)
	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Item{}, ErrNotFound
		}
		return model.Item{}, err
	}
	if err := s.attachProjectIDs(ctx, []model.Item{item}); err == nil {
		list := []model.Item{item}
		_ = s.attachProjectIDs(ctx, list)
		item = list[0]
	}
	return item, nil
}

func (s *Store) GetRelatedItems(ctx context.Context, id int64, limit int) ([]model.Item, error) {
	item, err := s.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	limit = max(limit, 1)
	needle := firstKeyword(item.Title, item.Content)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source_type, source_key, title, content, summary, publish_time, publish_time_text, detail_url, source_url, tag_flags, from_text, external_source_host, is_vip, has_image, raw_payload, captured_at, created_at, updated_at
FROM items
WHERE id <> ? AND source_type = ? AND (title LIKE ? OR content LIKE ?)
ORDER BY captured_at DESC, id DESC
LIMIT ?`, id, item.SourceType, "%"+needle+"%", "%"+needle+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanItems(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachProjectIDs(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) ListCrawlRuns(ctx context.Context, limit int, sourceType string) ([]model.CrawlRun, error) {
	limit = max(limit, 1)
	where := ""
	args := make([]any, 0, 2)
	if sourceType != "" {
		where = "WHERE source_type = ?"
		args = append(args, sourceType)
	}
	query := `SELECT id, source_type, started_at, finished_at, status, fetched_count, inserted_count, updated_count, error_text FROM crawl_runs ` + where + ` ORDER BY started_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]model.CrawlRun, 0)
	for rows.Next() {
		var run model.CrawlRun
		var startedAt string
		var finishedAt sql.NullString
		if err := rows.Scan(&run.ID, &run.SourceType, &startedAt, &finishedAt, &run.Status, &run.FetchedCount, &run.InsertedCount, &run.UpdatedCount, &run.ErrorText); err != nil {
			return nil, err
		}
		run.StartedAt = mustParseRFC3339(startedAt)
		if finishedAt.Valid {
			value := mustParseRFC3339(finishedAt.String)
			run.FinishedAt = &value
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) LinkItemsToProjects(ctx context.Context, sourceKeys []string, projectIDs []int64, ruleID int64) error {
	if len(sourceKeys) == 0 || len(projectIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, sourceKey := range sourceKeys {
		itemID, lookupErr := s.itemIDBySourceKeyTx(ctx, tx, sourceKey)
		if lookupErr != nil {
			err = lookupErr
			return err
		}
		for _, projectID := range projectIDs {
			if _, execErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_relations (item_id, project_id, rule_id, created_at) VALUES (?, ?, ?, ?)`, itemID, projectID, ruleID, now); execErr != nil {
				err = execErr
				return err
			}
		}
	}
	err = tx.Commit()
	return err
}

var ErrNotFound = errors.New("not found")

func (s *Store) sourceKeyExistsTx(ctx context.Context, tx *sql.Tx, sourceKey string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE source_key = ?`, sourceKey).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) itemIDBySourceKeyTx(ctx context.Context, tx *sql.Tx, sourceKey string) (int64, error) {
	var itemID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM items WHERE source_key = ?`, sourceKey).Scan(&itemID); err != nil {
		return 0, err
	}
	return itemID, nil
}

func buildItemFilter(filter model.ArticleFilter) (where string, args []any, joins string) {
	filters := make([]string, 0, 6)
	args = make([]any, 0, 6)
	if filter.ProjectID > 0 {
		joins = "JOIN item_relations ir ON ir.item_id = items.id"
		filters = append(filters, "ir.project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.SourceType != "" {
		filters = append(filters, "items.source_type = ?")
		args = append(args, filter.SourceType)
	}
	if filter.Keyword != "" {
		filters = append(filters, "(items.title LIKE ? OR items.content LIKE ? OR items.summary LIKE ?)")
		like := "%" + filter.Keyword + "%"
		args = append(args, like, like, like)
	}
	if filter.Start != "" {
		filters = append(filters, "items.captured_at >= ?")
		args = append(args, filter.Start)
	}
	if filter.End != "" {
		filters = append(filters, "items.captured_at <= ?")
		args = append(args, filter.End)
	}
	if len(filters) == 0 {
		return "", args, joins
	}
	return "WHERE " + strings.Join(filters, " AND "), args, joins
}

func (s *Store) attachProjectIDs(ctx context.Context, items []model.Item) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	lookup := make(map[int64]*model.Item, len(items))
	for index := range items {
		ids = append(ids, "?")
		args = append(args, items[index].ID)
		lookup[items[index].ID] = &items[index]
	}
	query := `SELECT item_id, project_id FROM item_relations WHERE item_id IN (` + strings.Join(ids, ",") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, projectID int64
		if err := rows.Scan(&itemID, &projectID); err != nil {
			return err
		}
		if item, ok := lookup[itemID]; ok {
			item.ProjectIDs = append(item.ProjectIDs, projectID)
		}
	}
	return rows.Err()
}

func (s *Store) attachUserState(ctx context.Context, userID int64, items []model.Item) error {
	if userID <= 0 || len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items)+1)
	lookup := make(map[int64]*model.Item, len(items))
	for index := range items {
		ids = append(ids, "?")
		args = append(args, items[index].ID)
		lookup[items[index].ID] = &items[index]
	}

	favoriteArgs := append([]any{userID}, args...)
	favoriteQuery := `SELECT item_id FROM favorites WHERE user_id = ? AND item_id IN (` + strings.Join(ids, ",") + `)`
	rows, err := s.db.QueryContext(ctx, favoriteQuery, favoriteArgs...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var itemID int64
		if err := rows.Scan(&itemID); err != nil {
			rows.Close()
			return err
		}
		if item, ok := lookup[itemID]; ok {
			item.Favorited = true
		}
	}
	rows.Close()

	readArgs := append([]any{userID}, args...)
	readQuery := `SELECT item_id FROM item_reads WHERE user_id = ? AND item_id IN (` + strings.Join(ids, ",") + `)`
	readRows, err := s.db.QueryContext(ctx, readQuery, readArgs...)
	if err != nil {
		return err
	}
	defer readRows.Close()
	for readRows.Next() {
		var itemID int64
		if err := readRows.Scan(&itemID); err != nil {
			return err
		}
		if item, ok := lookup[itemID]; ok {
			item.Read = true
		}
	}
	return readRows.Err()
}

func (s *Store) PopulateUserItemState(ctx context.Context, userID int64, items []model.Item) error {
	return s.attachUserState(ctx, userID, items)
}

type itemScanner interface {
	Scan(dest ...any) error
}

func scanItem(scanner itemScanner) (model.Item, error) {
	var item model.Item
	var capturedAt, createdAt, updatedAt string
	var isVIP, hasImage int
	err := scanner.Scan(
		&item.ID,
		&item.SourceType,
		&item.SourceKey,
		&item.Title,
		&item.Content,
		&item.Summary,
		&item.PublishTime,
		&item.PublishTimeText,
		&item.DetailURL,
		&item.SourceURL,
		&item.TagFlags,
		&item.FromText,
		&item.ExternalSourceHost,
		&isVIP,
		&hasImage,
		&item.RawPayload,
		&capturedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Item{}, err
	}
	item.IsVIP = isVIP != 0
	item.HasImage = hasImage != 0
	item.CapturedAt = mustParseRFC3339(capturedAt)
	item.CreatedAt = mustParseRFC3339(createdAt)
	item.UpdatedAt = mustParseRFC3339(updatedAt)
	return item, nil
}

func scanItems(rows *sql.Rows) ([]model.Item, error) {
	items := make([]model.Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func mustParseRFC3339(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstKeyword(values ...string) string {
	for _, value := range values {
		for _, field := range strings.Fields(strings.TrimSpace(value)) {
			if len([]rune(field)) >= 2 {
				return field
			}
		}
	}
	return "快讯"
}

func max(value, fallback int) int {
	if value < fallback {
		return fallback
	}
	return value
}

func (s *Store) String() string {
	return fmt.Sprintf("sqlite store")
}
