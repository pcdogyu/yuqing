-- PostgreSQL schema for the Go yuqing data store.
-- This file is intentionally idempotent and does not drop existing data.

BEGIN;

CREATE TABLE IF NOT EXISTS items (
	id BIGSERIAL PRIMARY KEY,
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
	id BIGSERIAL PRIMARY KEY,
	source_type TEXT NOT NULL,
	template_id BIGINT NOT NULL DEFAULT 0,
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
	id BIGSERIAL PRIMARY KEY,
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
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (source_type, source_key)
);
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_url TEXT NOT NULL DEFAULT '';
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_file_path TEXT NOT NULL DEFAULT '';
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_status TEXT NOT NULL DEFAULT '';
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_text TEXT NOT NULL DEFAULT '';
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_error TEXT NOT NULL DEFAULT '';
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_fetched_at TEXT NOT NULL DEFAULT '';
ALTER TABLE stock_research_surveys ADD COLUMN IF NOT EXISTS pdf_parsed_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS stock_institution_holdings (
	id BIGSERIAL PRIMARY KEY,
	stock_code TEXT NOT NULL DEFAULT '',
	stock_name TEXT NOT NULL DEFAULT '',
	report_period TEXT NOT NULL DEFAULT '',
	announce_date TEXT NOT NULL DEFAULT '',
	holder_name TEXT NOT NULL DEFAULT '',
	holder_type TEXT NOT NULL DEFAULT '',
	holder_code TEXT NOT NULL DEFAULT '',
	holder_rank TEXT NOT NULL DEFAULT '',
	shares DOUBLE PRECISION NOT NULL DEFAULT 0,
	shares_change DOUBLE PRECISION NOT NULL DEFAULT 0,
	change_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	float_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
	market_value DOUBLE PRECISION NOT NULL DEFAULT 0,
	source_type TEXT NOT NULL DEFAULT '',
	source_url TEXT NOT NULL DEFAULT '',
	raw_payload TEXT NOT NULL DEFAULT '{}',
	fetched_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (source_type, report_period, stock_code, holder_name, holder_type, holder_code)
);

CREATE TABLE IF NOT EXISTS users (
	id BIGSERIAL PRIMARY KEY,
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
	user_id BIGINT NOT NULL,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS api_tokens (
	token TEXT PRIMARY KEY,
	user_id BIGINT NOT NULL,
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
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL,
	search_word TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS platform_bindings (
	user_id BIGINT NOT NULL,
	kind TEXT NOT NULL,
	secret_id TEXT NOT NULL DEFAULT '',
	secret_key TEXT NOT NULL DEFAULT '',
	bound INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (user_id, kind)
);

CREATE TABLE IF NOT EXISTS project_groups (
	id BIGSERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_members (
	id BIGSERIAL PRIMARY KEY,
	project_id BIGINT NOT NULL,
	user_id BIGINT NOT NULL,
	role TEXT NOT NULL DEFAULT 'editor',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
	id BIGSERIAL PRIMARY KEY,
	group_id BIGINT NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	keywords TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor_rules (
	id BIGSERIAL PRIMARY KEY,
	project_id BIGINT NOT NULL,
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
	item_id BIGINT NOT NULL,
	project_id BIGINT NOT NULL,
	rule_id BIGINT NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	PRIMARY KEY (item_id, project_id, rule_id)
);

CREATE TABLE IF NOT EXISTS item_tags (
	item_id BIGINT NOT NULL,
	tag TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (item_id, tag)
);

CREATE TABLE IF NOT EXISTS favorites (
	user_id BIGINT NOT NULL,
	item_id BIGINT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id)
);

CREATE TABLE IF NOT EXISTS item_reads (
	user_id BIGINT NOT NULL,
	item_id BIGINT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id)
);

CREATE TABLE IF NOT EXISTS reports (
	id BIGSERIAL PRIMARY KEY,
	project_id BIGINT NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'draft',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS report_sections (
	id BIGSERIAL PRIMARY KEY,
	report_id BIGINT NOT NULL,
	heading TEXT NOT NULL,
	content TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS public_options (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL,
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
	id BIGSERIAL PRIMARY KEY,
	scope TEXT NOT NULL,
	scope_id BIGINT NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	payload TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE (scope, scope_id)
);

CREATE TABLE IF NOT EXISTS trend_points (
	id BIGSERIAL PRIMARY KEY,
	scope TEXT NOT NULL DEFAULT 'system',
	scope_id BIGINT NOT NULL DEFAULT 0,
	label TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS keyword_hotspots (
	id BIGSERIAL PRIMARY KEY,
	scope TEXT NOT NULL DEFAULT 'system',
	scope_id BIGINT NOT NULL DEFAULT 0,
	keyword TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS source_breakdowns (
	id BIGSERIAL PRIMARY KEY,
	scope TEXT NOT NULL DEFAULT 'system',
	scope_id BIGINT NOT NULL DEFAULT 0,
	source_type TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS system_notices (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS feedback (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL DEFAULT 0,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_preferences (
	user_id BIGINT PRIMARY KEY,
	language TEXT NOT NULL DEFAULT 'zh-CN',
	theme TEXT NOT NULL DEFAULT 'light',
	default_search_mode TEXT NOT NULL DEFAULT 'default',
	article_page_size INTEGER NOT NULL DEFAULT 20,
	email_notifications INTEGER NOT NULL DEFAULT 1,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS popup_states (
	user_id BIGINT NOT NULL,
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
	project_id BIGINT PRIMARY KEY,
	warning_setting_id BIGINT NOT NULL DEFAULT 0,
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
	project_id BIGINT PRIMARY KEY,
	opinion_condition_id BIGINT NOT NULL DEFAULT 0,
	time INTEGER NOT NULL DEFAULT 4,
	precise INTEGER NOT NULL DEFAULT 0,
	emotion TEXT NOT NULL DEFAULT '[1,2,3]',
	"similar" INTEGER NOT NULL DEFAULT 0,
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
	user_id BIGINT NOT NULL,
	item_id BIGINT NOT NULL,
	channel TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id, channel)
);

CREATE TABLE IF NOT EXISTS crypto_price_candles (
	symbol TEXT NOT NULL,
	interval TEXT NOT NULL,
	open_time TEXT NOT NULL,
	open DOUBLE PRECISION NOT NULL DEFAULT 0,
	high DOUBLE PRECISION NOT NULL DEFAULT 0,
	low DOUBLE PRECISION NOT NULL DEFAULT 0,
	close DOUBLE PRECISION NOT NULL DEFAULT 0,
	volume DOUBLE PRECISION NOT NULL DEFAULT 0,
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
	auction_price DOUBLE PRECISION NOT NULL DEFAULT 0,
	auction_volume DOUBLE PRECISION NOT NULL DEFAULT 0,
	auction_amount DOUBLE PRECISION NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'ok',
	fetched_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (trade_date, code)
);

CREATE TABLE IF NOT EXISTS task_runs (
	id BIGSERIAL PRIMARY KEY,
	task_name TEXT NOT NULL,
	status TEXT NOT NULL,
	message TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL DEFAULT 0,
	username TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL,
	resource TEXT NOT NULL DEFAULT '',
	detail_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS crawl_templates (
	id BIGSERIAL PRIMARY KEY,
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
	user_id BIGINT NOT NULL DEFAULT 0,
	openid TEXT NOT NULL DEFAULT '',
	session_token TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	completed_at TEXT
);

CREATE TABLE IF NOT EXISTS wechat_bindings (
	user_id BIGINT PRIMARY KEY,
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
CREATE INDEX IF NOT EXISTS idx_a_stock_auction_date_amount ON a_stock_auction_amounts(trade_date DESC, auction_amount DESC);
CREATE INDEX IF NOT EXISTS idx_a_stock_auction_code ON a_stock_auction_amounts(code);
CREATE INDEX IF NOT EXISTS idx_wechat_challenges_expires_at ON wechat_challenges(expires_at);
CREATE INDEX IF NOT EXISTS idx_wechat_bindings_openid ON wechat_bindings(openid);
CREATE INDEX IF NOT EXISTS idx_public_options_user_updated ON public_options(user_id, updatetime DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_opinion_conditions_updated ON opinion_conditions(updated_at DESC, project_id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created_at ON audit_logs(action, created_at DESC);

COMMIT;
