package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Store) EnsureDefaultAdmin(ctx context.Context, username, password string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE username = ?`, username).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `INSERT INTO users (username, display_name, email, role, password_hash, created_at, updated_at) VALUES (?, ?, '', 'admin', ?, ?, ?)`,
		username, "管理员", string(hash), now, now,
	)
	return err
}

func (s *Store) AuthenticateUser(ctx context.Context, username, password string) (model.User, error) {
	user, err := s.GetUserByUsername(ctx, username)
	if err != nil {
		return model.User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return model.User{}, ErrNotFound
	}
	return user, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (model.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, display_name, email, role, password_hash, created_at, updated_at FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (model.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, display_name, email, role, password_hash, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (model.Session, error) {
	now := time.Now().UTC()
	session := model.Session{
		Token:     uuid.NewString(),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		session.Token, session.UserID, session.ExpiresAt.Format(time.RFC3339), session.CreatedAt.Format(time.RFC3339),
	)
	return session, err
}

func (s *Store) GetSession(ctx context.Context, token string) (model.Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT token, user_id, expires_at, created_at FROM sessions WHERE token = ?`, token)
	var session model.Session
	var expiresAt, createdAt string
	if err := row.Scan(&session.Token, &session.UserID, &expiresAt, &createdAt); err != nil {
		return model.Session{}, err
	}
	session.ExpiresAt = mustParseRFC3339(expiresAt)
	session.CreatedAt = mustParseRFC3339(createdAt)
	if session.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.DeleteSession(ctx, token)
		return model.Session{}, ErrNotFound
	}
	return session, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *Store) CreateAPIToken(ctx context.Context, userID int64, name string) (model.APIToken, error) {
	now := time.Now().UTC()
	token := model.APIToken{
		Token:     uuid.NewString(),
		UserID:    userID,
		Name:      nonEmpty(name, "default"),
		CreatedAt: now,
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (token, user_id, name, created_at) VALUES (?, ?, ?, ?)`,
		token.Token, token.UserID, token.Name, token.CreatedAt.Format(time.RFC3339),
	)
	return token, err
}

func (s *Store) ResolveAPIToken(ctx context.Context, token string) (model.User, error) {
	var userID int64
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM api_tokens WHERE token = ?`, token).Scan(&userID); err != nil {
		return model.User{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE token = ?`, now, token)
	return s.GetUserByID(ctx, userID)
}

func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, group_id, name, keywords, description, status, created_at, updated_at FROM projects ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]model.Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, project)
	}
	return list, rows.Err()
}

func (s *Store) CreateProject(ctx context.Context, project model.Project) (model.Project, error) {
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects (group_id, name, keywords, description, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		project.GroupID, project.Name, project.Keywords, project.Description, nonEmpty(project.Status, "active"),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return model.Project{}, err
	}
	project.ID, _ = res.LastInsertId()
	project.Status = nonEmpty(project.Status, "active")
	return project, nil
}

func (s *Store) ListMonitorRules(ctx context.Context) ([]model.MonitorRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, name, include_keywords, exclude_keywords, channels, severity, created_at, updated_at FROM monitor_rules ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.MonitorRule, 0)
	for rows.Next() {
		rule, err := scanMonitorRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func (s *Store) CreateMonitorRule(ctx context.Context, rule model.MonitorRule) (model.MonitorRule, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO monitor_rules (project_id, name, include_keywords, exclude_keywords, channels, severity, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ProjectID, rule.Name, rule.IncludeKeywords, rule.ExcludeKeywords, rule.Channels, nonEmpty(rule.Severity, "medium"),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return model.MonitorRule{}, err
	}
	rule.ID, _ = res.LastInsertId()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	rule.Severity = nonEmpty(rule.Severity, "medium")
	return rule, nil
}

func (s *Store) SearchItemsFTS(ctx context.Context, keyword string, page, pageSize int) (model.SearchResult, error) {
	page = max(page, 1)
	pageSize = max(pageSize, 1)
	offset := (page - 1) * pageSize
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		list, err := s.ListItems(ctx, page, pageSize, "", "")
		if err != nil {
			return model.SearchResult{}, err
		}
		return model.SearchResult{Items: list.Items, Keyword: "", Total: list.Total, Page: page, PageSize: pageSize}, nil
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM items_fts WHERE items_fts MATCH ?`, keyword).Scan(&total); err != nil {
		return model.SearchResult{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT i.id, i.source_type, i.source_key, i.title, i.content, i.summary, i.publish_time, i.publish_time_text, i.detail_url, i.source_url, i.tag_flags, i.from_text, i.external_source_host, i.is_vip, i.has_image, i.raw_payload, i.captured_at, i.created_at, i.updated_at
FROM items_fts f
JOIN items i ON i.id = f.rowid
WHERE items_fts MATCH ?
ORDER BY bm25(items_fts), i.captured_at DESC
LIMIT ? OFFSET ?`, keyword, pageSize, offset)
	if err != nil {
		return model.SearchResult{}, err
	}
	defer rows.Close()
	items, err := scanItems(rows)
	if err != nil {
		return model.SearchResult{}, err
	}
	return model.SearchResult{Items: items, Keyword: keyword, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Store) Overview(ctx context.Context) (model.Overview, error) {
	var overview model.Overview
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM items`).Scan(&overview.ArticleCount); err != nil {
		return overview, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM projects`).Scan(&overview.ProjectCount); err != nil {
		return overview, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM reports`).Scan(&overview.ReportCount); err != nil {
		return overview, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM crawl_runs`).Scan(&overview.CrawlRunCount); err != nil {
		return overview, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM monitor_rules`).Scan(&overview.AlertRuleCount); err != nil {
		return overview, err
	}
	return overview, nil
}

func (s *Store) ListReports(ctx context.Context) ([]model.Report, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, title, summary, content, status, created_at, updated_at FROM reports ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := make([]model.Report, 0)
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (s *Store) CreateReport(ctx context.Context, report model.Report) (model.Report, error) {
	now := time.Now().UTC()
	report.Status = nonEmpty(report.Status, "draft")
	report.CreatedAt = now
	report.UpdatedAt = now
	res, err := s.db.ExecContext(ctx, `INSERT INTO reports (project_id, title, summary, content, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		report.ProjectID, report.Title, report.Summary, report.Content, report.Status, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return model.Report{}, err
	}
	report.ID, _ = res.LastInsertId()
	return report, nil
}

func (s *Store) UpsertAnalysisSnapshot(ctx context.Context, snapshot model.AnalysisSnapshot) error {
	now := time.Now().UTC()
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO analysis_snapshots (scope, scope_id, title, payload, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(scope, scope_id) DO UPDATE SET
	title = excluded.title,
	payload = excluded.payload,
	created_at = excluded.created_at`,
		snapshot.Scope, snapshot.ScopeID, snapshot.Title, snapshot.Payload, snapshot.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetAnalysisSnapshot(ctx context.Context, scope string, scopeID int64) (model.AnalysisSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, scope, scope_id, title, payload, created_at FROM analysis_snapshots WHERE scope = ? AND scope_id = ?`, scope, scopeID)
	var snapshot model.AnalysisSnapshot
	var createdAt string
	if err := row.Scan(&snapshot.ID, &snapshot.Scope, &snapshot.ScopeID, &snapshot.Title, &snapshot.Payload, &createdAt); err != nil {
		return model.AnalysisSnapshot{}, err
	}
	snapshot.CreatedAt = mustParseRFC3339(createdAt)
	return snapshot, nil
}

func (s *Store) ListNotices(ctx context.Context) ([]model.SystemNotice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, content, created_at FROM system_notices ORDER BY created_at DESC, id DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notices := make([]model.SystemNotice, 0)
	for rows.Next() {
		var notice model.SystemNotice
		var createdAt string
		if err := rows.Scan(&notice.ID, &notice.Title, &notice.Content, &createdAt); err != nil {
			return nil, err
		}
		notice.CreatedAt = mustParseRFC3339(createdAt)
		notices = append(notices, notice)
	}
	return notices, rows.Err()
}

func (s *Store) EnsureSeedData(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM solution_groups`).Scan(&count); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if count == 0 {
		_, err := s.db.ExecContext(ctx, `INSERT INTO solution_groups (name, description, created_at, updated_at) VALUES ('默认方案组', 'Go 重构后的默认方案组', ?, ?)`, now, now)
		if err != nil {
			return err
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM system_notices`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := s.db.ExecContext(ctx, `INSERT INTO system_notices (title, content, created_at) VALUES ('Go 系统已启用', '当前门户已切换到 Go 多服务骨架。', ?)`, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecordTaskRun(ctx context.Context, name, status, message string, startedAt time.Time, finishedAt *time.Time) error {
	finished := sql.NullString{}
	if finishedAt != nil {
		finished.Valid = true
		finished.String = finishedAt.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO task_runs (task_name, status, message, started_at, finished_at) VALUES (?, ?, ?, ?, ?)`,
		name, status, message, startedAt.Format(time.RFC3339), finished,
	)
	return err
}

func (s *Store) BuildOverviewSnapshot(ctx context.Context) (model.AnalysisSnapshot, error) {
	overview, err := s.Overview(ctx)
	if err != nil {
		return model.AnalysisSnapshot{}, err
	}
	payload, err := json.Marshal(overview)
	if err != nil {
		return model.AnalysisSnapshot{}, err
	}
	return model.AnalysisSnapshot{
		Scope:     "system",
		ScopeID:   0,
		Title:     "System Overview",
		Payload:   string(payload),
		CreatedAt: time.Now().UTC(),
	}, nil
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(scanner userScanner) (model.User, error) {
	var user model.User
	var createdAt, updatedAt string
	if err := scanner.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.Role, &user.PasswordHash, &createdAt, &updatedAt); err != nil {
		return model.User{}, err
	}
	user.CreatedAt = mustParseRFC3339(createdAt)
	user.UpdatedAt = mustParseRFC3339(updatedAt)
	return user, nil
}

func scanProject(scanner userScanner) (model.Project, error) {
	var project model.Project
	var createdAt, updatedAt string
	if err := scanner.Scan(&project.ID, &project.GroupID, &project.Name, &project.Keywords, &project.Description, &project.Status, &createdAt, &updatedAt); err != nil {
		return model.Project{}, err
	}
	project.CreatedAt = mustParseRFC3339(createdAt)
	project.UpdatedAt = mustParseRFC3339(updatedAt)
	return project, nil
}

func scanMonitorRule(scanner userScanner) (model.MonitorRule, error) {
	var rule model.MonitorRule
	var createdAt, updatedAt string
	if err := scanner.Scan(&rule.ID, &rule.ProjectID, &rule.Name, &rule.IncludeKeywords, &rule.ExcludeKeywords, &rule.Channels, &rule.Severity, &createdAt, &updatedAt); err != nil {
		return model.MonitorRule{}, err
	}
	rule.CreatedAt = mustParseRFC3339(createdAt)
	rule.UpdatedAt = mustParseRFC3339(updatedAt)
	return rule, nil
}

func scanReport(scanner userScanner) (model.Report, error) {
	var report model.Report
	var createdAt, updatedAt string
	if err := scanner.Scan(&report.ID, &report.ProjectID, &report.Title, &report.Summary, &report.Content, &report.Status, &createdAt, &updatedAt); err != nil {
		return model.Report{}, err
	}
	report.CreatedAt = mustParseRFC3339(createdAt)
	report.UpdatedAt = mustParseRFC3339(updatedAt)
	return report, nil
}
