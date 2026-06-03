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
	ProjectID   int64     `json:"project_id"`
	Enabled     bool      `json:"enabled"`
	Channels    string    `json:"channels"`
	Threshold   int       `json:"threshold"`
	Recipients  string    `json:"recipients"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
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
