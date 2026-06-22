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

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const busyTimeoutMillis = 30000

type Store struct {
	db     *DB
	driver string
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
	if _, err := db.ExecContext(context.Background(), fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMillis)); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &Store{db: newDB(db, "sqlite"), driver: "sqlite"}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func NewPostgres(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: newDB(db, "postgres"), driver: "postgres"}
	if err := store.migratePostgres(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Driver() string {
	if s == nil || s.driver == "" {
		return "sqlite"
	}
	return s.driver
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
	template_id INTEGER NOT NULL DEFAULT 0,
	template_name TEXT NOT NULL DEFAULT '',
	template_snapshot TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT,
	status TEXT NOT NULL,
	fetched_count INTEGER NOT NULL DEFAULT 0,
	inserted_count INTEGER NOT NULL DEFAULT 0,
	updated_count INTEGER NOT NULL DEFAULT 0,
	error_text TEXT
);

CREATE TABLE IF NOT EXISTS crawl_states (
	source_type TEXT NOT NULL,
	cursor_key TEXT NOT NULL,
	cursor_value TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL,
	PRIMARY KEY (source_type, cursor_key)
);

CREATE TABLE IF NOT EXISTS stock_research_surveys (
	id INTEGER PRIMARY KEY,
	code TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'report',
	title TEXT NOT NULL DEFAULT '',
	institution TEXT NOT NULL DEFAULT '',
	analyst TEXT NOT NULL DEFAULT '',
	rating TEXT NOT NULL DEFAULT '',
	target_price TEXT NOT NULL DEFAULT '',
	research_date TEXT NOT NULL DEFAULT '',
	publish_time TEXT NOT NULL DEFAULT '',
	source_url TEXT NOT NULL DEFAULT '',
	source_type TEXT NOT NULL DEFAULT '',
	source_key TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	raw_payload TEXT NOT NULL DEFAULT '{}',
	pdf_url TEXT NOT NULL DEFAULT '',
	pdf_file_path TEXT NOT NULL DEFAULT '',
	pdf_status TEXT NOT NULL DEFAULT '',
	pdf_text TEXT NOT NULL DEFAULT '',
	pdf_error TEXT NOT NULL DEFAULT '',
	pdf_fetched_at TEXT NOT NULL DEFAULT '',
	pdf_parsed_at TEXT NOT NULL DEFAULT '',
	nlp_score REAL NOT NULL DEFAULT 0,
	nlp_rating TEXT NOT NULL DEFAULT '',
	nlp_reason TEXT NOT NULL DEFAULT '',
	nlp_scored_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (source_type, source_key)
);

CREATE TABLE IF NOT EXISTS stock_institution_holdings (
	id INTEGER PRIMARY KEY,
	stock_code TEXT NOT NULL DEFAULT '',
	stock_name TEXT NOT NULL DEFAULT '',
	report_period TEXT NOT NULL DEFAULT '',
	announce_date TEXT NOT NULL DEFAULT '',
	holder_name TEXT NOT NULL DEFAULT '',
	holder_type TEXT NOT NULL DEFAULT '',
	holder_code TEXT NOT NULL DEFAULT '',
	holder_rank TEXT NOT NULL DEFAULT '',
	shares REAL NOT NULL DEFAULT 0,
	shares_change REAL NOT NULL DEFAULT 0,
	change_ratio REAL NOT NULL DEFAULT 0,
	float_ratio REAL NOT NULL DEFAULT 0,
	market_value REAL NOT NULL DEFAULT 0,
	source_type TEXT NOT NULL DEFAULT '',
	source_url TEXT NOT NULL DEFAULT '',
	raw_payload TEXT NOT NULL DEFAULT '{}',
	fetched_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (source_type, report_period, stock_code, holder_name, holder_type, holder_code)
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
	status INTEGER NOT NULL DEFAULT 1,
	term_of_validity TEXT NOT NULL DEFAULT '2099-01-19T00:00:00Z',
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

CREATE TABLE IF NOT EXISTS search_words (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL,
	search_word TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS platform_bindings (
	user_id INTEGER NOT NULL,
	kind TEXT NOT NULL,
	secret_id TEXT NOT NULL DEFAULT '',
	secret_key TEXT NOT NULL DEFAULT '',
	bound INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (user_id, kind)
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

CREATE TABLE IF NOT EXISTS public_options (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL,
	eventname TEXT NOT NULL,
	eventkeywords TEXT NOT NULL DEFAULT '',
	eventstopwords TEXT NOT NULL DEFAULT '',
	eventstarttime TEXT NOT NULL DEFAULT '',
	eventendtime TEXT NOT NULL DEFAULT '',
	createtime TEXT NOT NULL,
	status INTEGER NOT NULL DEFAULT 3,
	updatetime TEXT NOT NULL,
	detail_status INTEGER NOT NULL DEFAULT 3,
	emotional_index TEXT NOT NULL DEFAULT '',
	back_analysis TEXT NOT NULL DEFAULT '{}',
	event_context TEXT NOT NULL DEFAULT '[]',
	event_trace TEXT NOT NULL DEFAULT '{}',
	hot_analysis TEXT NOT NULL DEFAULT '[]',
	netizens_analysis TEXT NOT NULL DEFAULT '{}',
	statistics TEXT NOT NULL DEFAULT '{}',
	propagation_analysis TEXT NOT NULL DEFAULT '{}',
	thematic_analysis TEXT NOT NULL DEFAULT '{}',
	unscramble_content TEXT NOT NULL DEFAULT '{}',
	content_analysis TEXT NOT NULL DEFAULT ''
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
	count INTEGER NOT NULL DEFAULT 0,
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
	warning_setting_id INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	warning_status INTEGER NOT NULL DEFAULT 1,
	warning_name TEXT NOT NULL DEFAULT '预警',
	warning_word TEXT NOT NULL DEFAULT '',
	warning_classify TEXT NOT NULL DEFAULT '',
	warning_content INTEGER NOT NULL DEFAULT 0,
	warning_similar INTEGER NOT NULL DEFAULT 0,
	warning_match INTEGER NOT NULL DEFAULT 1,
	warning_deduplication INTEGER NOT NULL DEFAULT 0,
	warning_source TEXT NOT NULL DEFAULT '',
	warning_receive_time TEXT NOT NULL DEFAULT '',
	weekend_warning INTEGER NOT NULL DEFAULT 0,
	warning_interval TEXT NOT NULL DEFAULT '',
	channels TEXT NOT NULL DEFAULT '',
	threshold INTEGER NOT NULL DEFAULT 80,
	recipients TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS opinion_conditions (
	project_id INTEGER PRIMARY KEY,
	opinion_condition_id INTEGER NOT NULL DEFAULT 0,
	time INTEGER NOT NULL DEFAULT 4,
	precise INTEGER NOT NULL DEFAULT 0,
	emotion TEXT NOT NULL DEFAULT '[1,2,3]',
	similar INTEGER NOT NULL DEFAULT 0,
	sort INTEGER NOT NULL DEFAULT 1,
	matchs INTEGER NOT NULL DEFAULT 1,
	times TEXT NOT NULL DEFAULT '',
	timee TEXT NOT NULL DEFAULT '',
	classify TEXT NOT NULL DEFAULT '',
	websitename TEXT NOT NULL DEFAULT '',
	author TEXT NOT NULL DEFAULT '',
	organization TEXT NOT NULL DEFAULT '',
	categorylable TEXT NOT NULL DEFAULT '',
	enterprisetype TEXT NOT NULL DEFAULT '',
	hightechtype TEXT NOT NULL DEFAULT '',
	policylableflag TEXT NOT NULL DEFAULT '',
	datasource_type TEXT NOT NULL DEFAULT '',
	event_index TEXT NOT NULL DEFAULT '',
	industry_index TEXT NOT NULL DEFAULT '',
	province TEXT NOT NULL DEFAULT '',
	city TEXT NOT NULL DEFAULT '',
	create_time TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS item_shares (
	user_id INTEGER NOT NULL,
	item_id INTEGER NOT NULL,
	channel TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id, channel)
);

CREATE TABLE IF NOT EXISTS crypto_price_candles (
	symbol TEXT NOT NULL,
	interval TEXT NOT NULL,
	open_time TEXT NOT NULL,
	open REAL NOT NULL DEFAULT 0,
	high REAL NOT NULL DEFAULT 0,
	low REAL NOT NULL DEFAULT 0,
	close REAL NOT NULL DEFAULT 0,
	volume REAL NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (symbol, interval, open_time)
);

CREATE TABLE IF NOT EXISTS crypto_insight_snapshots (
	pair TEXT NOT NULL,
	horizon_set TEXT NOT NULL,
	payload TEXT NOT NULL,
	computed_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	PRIMARY KEY (pair, horizon_set)
);

CREATE TABLE IF NOT EXISTS a_stock_auction_amounts (
	trade_date TEXT NOT NULL,
	code TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	auction_price REAL NOT NULL DEFAULT 0,
	auction_volume REAL NOT NULL DEFAULT 0,
	auction_amount REAL NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'ok',
	fetched_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (trade_date, code)
);

CREATE TABLE IF NOT EXISTS a_stock_recommendation_snapshots (
	strategy_date TEXT NOT NULL,
	period TEXT NOT NULL,
	ignore_recent INTEGER NOT NULL DEFAULT 0,
	recommendations_json TEXT NOT NULL DEFAULT '[]',
	backtests_json TEXT NOT NULL DEFAULT '[]',
	backtest_status TEXT NOT NULL DEFAULT '',
	generated_count INTEGER NOT NULL DEFAULT 0,
	recent_filtered INTEGER NOT NULL DEFAULT 0,
	same_day_morning_filtered INTEGER NOT NULL DEFAULT 0,
	market_candidate_status TEXT NOT NULL DEFAULT '',
	market_candidate_count INTEGER NOT NULL DEFAULT 0,
	auction_amount_label TEXT NOT NULL DEFAULT '',
	empty_reason TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (strategy_date, period, ignore_recent)
);

CREATE TABLE IF NOT EXISTS task_runs (
	id INTEGER PRIMARY KEY,
	task_name TEXT NOT NULL,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL DEFAULT 0,
	username TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL,
	resource TEXT NOT NULL DEFAULT '',
	detail_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS crawl_templates (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	website TEXT NOT NULL DEFAULT '',
	source_type TEXT NOT NULL DEFAULT 'custom',
	enabled INTEGER NOT NULL DEFAULT 1,
	config_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wechat_challenges (
	scene_str TEXT PRIMARY KEY,
	purpose TEXT NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	openid TEXT NOT NULL DEFAULT '',
	session_token TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	completed_at TEXT
);

CREATE TABLE IF NOT EXISTS wechat_bindings (
	user_id INTEGER PRIMARY KEY,
	openid TEXT NOT NULL DEFAULT '',
	bound_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_items_source_type_captured_at ON items(source_type, captured_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_items_title ON items(title);
CREATE INDEX IF NOT EXISTS idx_crawl_runs_source_type_started_at ON crawl_runs(source_type, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_crawl_states_updated ON crawl_states(source_type, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_stock_research_code_date ON stock_research_surveys(code, research_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_stock_research_institution_date ON stock_research_surveys(institution, research_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_stock_research_source_date ON stock_research_surveys(source_type, research_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_stock_holdings_code_period ON stock_institution_holdings(stock_code, report_period DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_stock_holdings_holder_period ON stock_institution_holdings(holder_name, report_period DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_stock_holdings_type_period ON stock_institution_holdings(holder_type, report_period DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_crawl_templates_enabled_updated ON crawl_templates(enabled, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_a_stock_auction_date_amount ON a_stock_auction_amounts(trade_date DESC, auction_amount DESC);
CREATE INDEX IF NOT EXISTS idx_a_stock_auction_code ON a_stock_auction_amounts(code);
CREATE INDEX IF NOT EXISTS idx_a_stock_recommendations_updated ON a_stock_recommendation_snapshots(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_projects_group_id ON projects(group_id);
CREATE INDEX IF NOT EXISTS idx_monitor_rules_project_id ON monitor_rules(project_id);
CREATE INDEX IF NOT EXISTS idx_reports_project_id ON reports(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_captchas_expires_at ON captchas(expires_at);
CREATE INDEX IF NOT EXISTS idx_search_words_user_created ON search_words(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_platform_bindings_kind_updated ON platform_bindings(kind, updated_at DESC, user_id DESC);
CREATE INDEX IF NOT EXISTS idx_item_relations_project_id ON item_relations(project_id, item_id DESC);
CREATE INDEX IF NOT EXISTS idx_trend_points_scope ON trend_points(scope, scope_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_keyword_hotspots_scope ON keyword_hotspots(scope, scope_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_breakdowns_scope ON source_breakdowns(scope, scope_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_popup_states_user_id ON popup_states(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_item_shares_item_id ON item_shares(item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_crypto_price_candles_lookup ON crypto_price_candles(symbol, interval, open_time DESC);
CREATE INDEX IF NOT EXISTS idx_crypto_insight_snapshots_expiry ON crypto_insight_snapshots(expires_at, pair);
CREATE INDEX IF NOT EXISTS idx_wechat_challenges_expires_at ON wechat_challenges(expires_at);
CREATE INDEX IF NOT EXISTS idx_wechat_bindings_openid ON wechat_bindings(openid);
CREATE INDEX IF NOT EXISTS idx_public_options_user_updated ON public_options(user_id, updatetime DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_opinion_conditions_updated ON opinion_conditions(updated_at DESC, project_id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created_at ON audit_logs(action, created_at DESC);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN status INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN term_of_validity TEXT NOT NULL DEFAULT '2099-01-19T00:00:00Z'`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE monitor_rules ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE popup_states ADD COLUMN count INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE crawl_runs ADD COLUMN template_id INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE crawl_runs ADD COLUMN template_name TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE crawl_runs ADD COLUMN template_snapshot TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE crawl_templates ADD COLUMN website TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_url TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_file_path TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_status TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_text TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_error TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_fetched_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN pdf_parsed_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN nlp_score REAL NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN nlp_rating TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN nlp_reason TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE stock_research_surveys ADD COLUMN nlp_scored_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE platform_bindings ADD COLUMN bound INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE public_options ADD COLUMN detail_status INTEGER NOT NULL DEFAULT 3`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_setting_id INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_status INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_name TEXT NOT NULL DEFAULT '预警'`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_word TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_classify TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_content INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_similar INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_match INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_deduplication INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_source TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_receive_time TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN weekend_warning INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE warning_settings ADD COLUMN warning_interval TEXT NOT NULL DEFAULT ''`)
	return nil
}

func (s *Store) migratePostgres(ctx context.Context) error {
	schemaPath := filepath.Join("db", "postgres_schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read postgres schema %s: %w", schemaPath, err)
	}
	if _, err := s.db.ExecContext(ctx, string(schema)); err != nil {
		return err
	}
	return nil
}

func (s *Store) StartCrawlRun(ctx context.Context, sourceType string, startedAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO crawl_runs (source_type, template_id, template_name, template_snapshot, started_at, status, fetched_count, inserted_count, updated_count) VALUES (?, 0, '', '', ?, 'running', 0, 0, 0)`,
		sourceType, startedAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) StartCrawlTemplateRun(ctx context.Context, sourceType string, templateID int64, templateName, templateSnapshot string, startedAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO crawl_runs (source_type, template_id, template_name, template_snapshot, started_at, status, fetched_count, inserted_count, updated_count) VALUES (?, ?, ?, ?, ?, 'running', 0, 0, 0)`,
		sourceType, templateID, templateName, templateSnapshot, startedAt.Format(time.RFC3339),
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
	row := s.db.QueryRowContext(ctx, `SELECT id, source_type, source_key, title, content, summary, publish_time, publish_time_text, detail_url, source_url, tag_flags, from_text, external_source_host, is_vip, has_image, raw_payload, captured_at, created_at, updated_at FROM items WHERE id = ? AND NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')`, id)
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
AND NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')
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
	query := `SELECT id, source_type, template_id, template_name, template_snapshot, started_at, finished_at, status, fetched_count, inserted_count, updated_count, error_text FROM crawl_runs ` + where + ` ORDER BY started_at DESC, id DESC LIMIT ?`
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
		var errorText sql.NullString
		if err := rows.Scan(&run.ID, &run.SourceType, &run.TemplateID, &run.TemplateName, &run.TemplateSnapshot, &startedAt, &finishedAt, &run.Status, &run.FetchedCount, &run.InsertedCount, &run.UpdatedCount, &errorText); err != nil {
			return nil, err
		}
		run.StartedAt = mustParseRFC3339(startedAt)
		if finishedAt.Valid {
			value := mustParseRFC3339(finishedAt.String)
			run.FinishedAt = &value
		}
		if errorText.Valid {
			run.ErrorText = errorText.String
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
var ErrConflict = errors.New("conflict")

func (s *Store) sourceKeyExistsTx(ctx context.Context, tx *Tx, sourceKey string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE source_key = ?`, sourceKey).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) itemIDBySourceKeyTx(ctx context.Context, tx *Tx, sourceKey string) (int64, error) {
	var itemID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM items WHERE source_key = ?`, sourceKey).Scan(&itemID); err != nil {
		return 0, err
	}
	return itemID, nil
}

func buildItemFilter(filter model.ArticleFilter) (where string, args []any, joins string) {
	filters := make([]string, 0, 6)
	args = make([]any, 0, 6)
	filters = append(filters, "NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')")
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
	timeColumn := itemFilterTimeColumn(filter.TimeField)
	if filter.Start != "" {
		filters = append(filters, "items."+timeColumn+" >= ?")
		args = append(args, filter.Start)
	}
	if filter.End != "" {
		filters = append(filters, "items."+timeColumn+" <= ?")
		args = append(args, filter.End)
	}
	if len(filters) == 0 {
		return "", args, joins
	}
	return "WHERE " + strings.Join(filters, " AND "), args, joins
}

func itemFilterTimeColumn(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "publish_time", "published_at":
		return "publish_time"
	default:
		return "captured_at"
	}
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
