package model

import "time"

type User struct {
	ID             int64     `json:"id"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	Status         int       `json:"status"`
	TermOfValidity time.Time `json:"term_of_validity"`
	PasswordHash   string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UserProfileUpdate struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type Session struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type WechatQRCode struct {
	QRCodeURL string `json:"qrcodeUrl"`
	SceneStr  string `json:"sceneStr"`
}

type WechatBindQRCode struct {
	QRCodeURL string `json:"qrcodeUrl"`
	SceneStr  string `json:"sceneStr"`
	Name      string `json:"name"`
}

type WechatEvent struct {
	SceneStr string `json:"sceneStr"`
	OpenID   string `json:"openid"`
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname,omitempty"`
}

type WechatChallenge struct {
	SceneStr     string     `json:"scene_str"`
	Purpose      string     `json:"purpose"`
	UserID       int64      `json:"user_id"`
	OpenID       string     `json:"openid"`
	SessionToken string     `json:"session_token"`
	Status       string     `json:"status"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type WechatBinding struct {
	UserID    int64     `json:"user_id"`
	OpenID    string    `json:"openid"`
	BoundAt   time.Time `json:"bound_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type APIToken struct {
	Token      string     `json:"token"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type ProjectGroup struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Project struct {
	ID          int64     `json:"id"`
	GroupID     int64     `json:"group_id"`
	GroupName   string    `json:"group_name,omitempty"`
	Name        string    `json:"name"`
	Keywords    string    `json:"keywords"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MonitorRule struct {
	ID              int64     `json:"id"`
	ProjectID       int64     `json:"project_id"`
	ProjectName     string    `json:"project_name,omitempty"`
	Name            string    `json:"name"`
	IncludeKeywords string    `json:"include_keywords"`
	ExcludeKeywords string    `json:"exclude_keywords"`
	Channels        string    `json:"channels"`
	Severity        string    `json:"severity"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Favorite struct {
	UserID    int64     `json:"user_id"`
	ItemID    int64     `json:"item_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Report struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReportSection struct {
	ID        int64     `json:"id"`
	ReportID  int64     `json:"report_id"`
	Heading   string    `json:"heading"`
	Content   string    `json:"content"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type AnalysisSnapshot struct {
	ID        int64     `json:"id"`
	Scope     string    `json:"scope"`
	ScopeID   int64     `json:"scope_id"`
	Title     string    `json:"title"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

type SystemNotice struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Feedback struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Captcha struct {
	ID        string    `json:"id"`
	Code      string    `json:"code,omitempty"`
	ImageSVG  string    `json:"image_svg,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type UserPreference struct {
	UserID             int64     `json:"user_id"`
	Language           string    `json:"language"`
	Theme              string    `json:"theme"`
	DefaultSearchMode  string    `json:"default_search_mode"`
	ArticlePageSize    int       `json:"article_page_size"`
	EmailNotifications bool      `json:"email_notifications"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PopupState struct {
	UserID      int64      `json:"user_id"`
	Key         string     `json:"key"`
	Dismissed   bool       `json:"dismissed"`
	Count       int        `json:"count"`
	DismissedAt *time.Time `json:"dismissed_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type MailConfig struct {
	Enabled     bool      `json:"enabled"`
	SMTPHost    string    `json:"smtp_host"`
	SMTPPort    int       `json:"smtp_port"`
	Username    string    `json:"username"`
	Password    string    `json:"password,omitempty"`
	SenderName  string    `json:"sender_name"`
	SenderEmail string    `json:"sender_email"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WarningSetting struct {
	WarningSettingID     int64     `json:"warning_setting_id,omitempty"`
	ProjectID            int64     `json:"project_id"`
	WarningStatus        int       `json:"warning_status,omitempty"`
	WarningName          string    `json:"warning_name,omitempty"`
	WarningWord          string    `json:"warning_word,omitempty"`
	WarningClassify      string    `json:"warning_classify,omitempty"`
	WarningContent       int       `json:"warning_content,omitempty"`
	WarningSimilar       int       `json:"warning_similar,omitempty"`
	WarningMatch         int       `json:"warning_match,omitempty"`
	WarningDeduplication int       `json:"warning_deduplication,omitempty"`
	WarningSource        string    `json:"warning_source,omitempty"`
	WarningReceiveTime   string    `json:"warning_receive_time,omitempty"`
	WeekendWarning       int       `json:"weekend_warning,omitempty"`
	WarningInterval      string    `json:"warning_interval,omitempty"`
	UserID               int64     `json:"user_id,omitempty"`
	Enabled              bool      `json:"enabled"`
	Channels             string    `json:"channels"`
	Threshold            int       `json:"threshold"`
	Recipients           string    `json:"recipients"`
	Description          string    `json:"description"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ShareRecord struct {
	UserID    int64     `json:"user_id"`
	ItemID    int64     `json:"item_id"`
	Channel   string    `json:"channel"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskRun struct {
	ID         int64      `json:"id"`
	TaskName   string     `json:"task_name"`
	Status     string     `json:"status"`
	Message    string     `json:"message"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type AuditLog struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Username   string    `json:"username"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	DetailJSON string    `json:"detail_json"`
	CreatedAt  time.Time `json:"created_at"`
}

type SearchResult struct {
	Items    []Item `json:"items"`
	Keyword  string `json:"keyword"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type Overview struct {
	ArticleCount   int `json:"article_count"`
	ProjectCount   int `json:"project_count"`
	ReportCount    int `json:"report_count"`
	CrawlRunCount  int `json:"crawl_run_count"`
	AlertRuleCount int `json:"alert_rule_count"`
}

type TrendPoint struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type SourceBreakdown struct {
	SourceType string `json:"source_type"`
	Count      int    `json:"count"`
}

type KeywordHotspot struct {
	Keyword string `json:"keyword"`
	Count   int    `json:"count"`
}

type DashboardSnapshot struct {
	Overview  Overview          `json:"overview"`
	Trends    []TrendPoint      `json:"trends"`
	Sources   []SourceBreakdown `json:"sources"`
	Keywords  []KeywordHotspot  `json:"keywords"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type NLPRequest struct {
	Text string `json:"text"`
}

type NLPResponse struct {
	Title    string   `json:"title,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

type NLPStockScoreRequest struct {
	Code  string `json:"code,omitempty"`
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text"`
}

type NLPStockScoreResponse struct {
	Code     string   `json:"code,omitempty"`
	Name     string   `json:"name,omitempty"`
	Score    float64  `json:"score"`
	Rating   string   `json:"rating"`
	Reason   string   `json:"reason"`
	Keywords []string `json:"keywords,omitempty"`
	Status   string   `json:"status,omitempty"`
}

type NLPReportPreviewRequest struct {
	ArticleID   string `json:"article_id,omitempty"`
	Text        string `json:"text,omitempty"`
	Title       string `json:"title,omitempty"`
	RelatedWord string `json:"relatedword,omitempty"`
	PublishTime string `json:"publish_time,omitempty"`
}

type NLPReportPreviewResponse struct {
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Keywords    []string `json:"keywords,omitempty"`
	Report      string   `json:"report"`
	PublishTime string   `json:"publish_time,omitempty"`
	Status      string   `json:"status,omitempty"`
}

type NLPCapability struct {
	Name            string   `json:"name"`
	Method          string   `json:"method"`
	Path            string   `json:"path"`
	Description     string   `json:"description,omitempty"`
	AuthMode        string   `json:"auth_mode,omitempty"`
	LegacyPaths     []string `json:"legacy_paths,omitempty"`
	DegradeStrategy string   `json:"degrade_strategy,omitempty"`
	Enabled         bool     `json:"enabled"`
}

type OperationServiceStatus struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

type OperationExternalStatus struct {
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	Message        string     `json:"message,omitempty"`
	LastFetchAt    *time.Time `json:"last_fetch_at,omitempty"`
	FetchedCount   int        `json:"fetched_count,omitempty"`
	InsertedCount  int        `json:"inserted_count,omitempty"`
	UpdatedCount   int        `json:"updated_count,omitempty"`
	DuplicateCount int        `json:"duplicate_count,omitempty"`
}

type LegacyStrategyCount struct {
	Strategy string `json:"strategy"`
	Count    int    `json:"count"`
}

type OperationSchedulerJob struct {
	Name           string     `json:"name"`
	Group          string     `json:"group"`
	Description    string     `json:"description"`
	JavaQuartzName string     `json:"java_quartz_name"`
	Cron           string     `json:"cron"`
	IntervalSec    int64      `json:"interval_sec"`
	Enabled        bool       `json:"enabled"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
	LastStatus     string     `json:"last_status,omitempty"`
	LastMessage    string     `json:"last_message,omitempty"`
	LastStartedAt  *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
}

type OperationTaskSummary struct {
	RecentCount              int        `json:"recent_count"`
	FailedCount              int        `json:"failed_count"`
	ConsecutiveFailures      int        `json:"consecutive_failures"`
	LastTaskName             string     `json:"last_task_name,omitempty"`
	LastStatus               string     `json:"last_status,omitempty"`
	LastStartedAt            *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt           *time.Time `json:"last_finished_at,omitempty"`
	ConsecutiveFailureTask   string     `json:"consecutive_failure_task,omitempty"`
	ConsecutiveFailureReason string     `json:"consecutive_failure_reason,omitempty"`
}

type OperationAuditSummary struct {
	RecentCount int        `json:"recent_count"`
	LastAction  string     `json:"last_action,omitempty"`
	LastAt      *time.Time `json:"last_at,omitempty"`
	LastAgeSec  int64      `json:"last_age_sec,omitempty"`
}

type OperationBackupStatus struct {
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	Message      string     `json:"message,omitempty"`
	Path         string     `json:"path,omitempty"`
	SizeBytes    int64      `json:"size_bytes,omitempty"`
	LastBackupAt *time.Time `json:"last_backup_at,omitempty"`
	AgeHours     float64    `json:"age_hours,omitempty"`
}

type DatabaseConfigStatus struct {
	Driver             string `json:"driver"`
	ConfiguredDriver   string `json:"configured_driver,omitempty"`
	RuntimeDriver      string `json:"runtime_driver"`
	Status             string `json:"status"`
	Message            string `json:"message,omitempty"`
	ConfigPath         string `json:"config_path,omitempty"`
	RestartRequired    bool   `json:"restart_required"`
	SQLitePath         string `json:"sqlite_path,omitempty"`
	PostgresHost       string `json:"postgres_host,omitempty"`
	PostgresPort       string `json:"postgres_port,omitempty"`
	PostgresDatabase   string `json:"postgres_database,omitempty"`
	PostgresUser       string `json:"postgres_user,omitempty"`
	PostgresSSLMode    string `json:"postgres_sslmode,omitempty"`
	PostgresConfigured bool   `json:"postgres_configured"`
	PostgresDSN        string `json:"postgres_dsn,omitempty"`
}

type OperationLegacyRouteStatus struct {
	Path       string `json:"path"`
	Expected   int    `json:"expected"`
	Actual     int    `json:"actual"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	FormalPath string `json:"formal_path,omitempty"`
}

type OperationsSummary struct {
	GeneratedAt          time.Time                    `json:"generated_at"`
	Services             []OperationServiceStatus     `json:"services"`
	SchedulerJobs        []OperationSchedulerJob      `json:"scheduler_jobs"`
	TaskSummary          OperationTaskSummary         `json:"task_summary"`
	RecentTaskRuns       []TaskRun                    `json:"recent_task_runs"`
	FailedTaskRuns       []TaskRun                    `json:"failed_task_runs"`
	AuditSummary         OperationAuditSummary        `json:"audit_summary"`
	RecentAuditLogs      []AuditLog                   `json:"recent_audit_logs"`
	LegacyRegistry       []LegacyStrategyCount        `json:"legacy_registry"`
	LegacyRouteProbes    []OperationLegacyRouteStatus `json:"legacy_route_probes"`
	ExternalIntegrations []OperationExternalStatus    `json:"external_integrations"`
	Backup               OperationBackupStatus        `json:"backup"`
	Database             DatabaseConfigStatus         `json:"database"`
	Ready                bool                         `json:"ready"`
}

type OperationAlert struct {
	Name      string    `json:"name"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
