package model

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type APIToken struct {
	Token      string     `json:"token"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type SolutionGroup struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Project struct {
	ID          int64     `json:"id"`
	GroupID     int64     `json:"group_id"`
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
	Name            string    `json:"name"`
	IncludeKeywords string    `json:"include_keywords"`
	ExcludeKeywords string    `json:"exclude_keywords"`
	Channels        string    `json:"channels"`
	Severity        string    `json:"severity"`
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

type NLPRequest struct {
	Text string `json:"text"`
}

type NLPResponse struct {
	Title    string   `json:"title,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}
