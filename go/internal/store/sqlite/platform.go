package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Store) EnsureDefaultAdmin(ctx context.Context, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	query := `INSERT OR IGNORE INTO users (username, display_name, email, role, status, term_of_validity, password_hash, created_at, updated_at) VALUES (?, ?, '', 'admin', 1, ?, ?, ?, ?)`
	if s.Driver() == "postgres" {
		query = `INSERT INTO users (username, display_name, email, role, status, term_of_validity, password_hash, created_at, updated_at) VALUES (?, ?, '', 'admin', 1, ?, ?, ?, ?) ON CONFLICT(username) DO NOTHING`
	}
	if _, err = s.db.ExecContext(ctx, query,
		username, "管理员", "2099-01-19T00:00:00Z", string(hash), now, now,
	); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE username = ?`, username).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return errors.New("default admin was not created")
	}
	return nil
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
	row := s.db.QueryRowContext(ctx, `SELECT id, username, display_name, email, role, status, term_of_validity, password_hash, created_at, updated_at FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (s *Store) CreateUser(ctx context.Context, user model.User, password string) (model.User, error) {
	username := strings.TrimSpace(user.Username)
	if username == "" {
		return model.User{}, errors.New("username required")
	}
	if strings.TrimSpace(password) == "" {
		return model.User{}, errors.New("password required")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE username = ?`, username).Scan(&exists); err != nil {
		return model.User{}, err
	}
	if exists > 0 {
		return model.User{}, ErrConflict
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, err
	}
	now := time.Now().UTC()
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = username
	}
	role := strings.TrimSpace(user.Role)
	if role == "" {
		role = "user"
	}
	if user.TermOfValidity.IsZero() {
		user.TermOfValidity = time.Date(2099, 1, 19, 0, 0, 0, 0, time.UTC)
	}
	updatedAt := now.Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, display_name, email, role, status, term_of_validity, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		username,
		displayName,
		strings.TrimSpace(user.Email),
		role,
		user.Status,
		user.TermOfValidity.UTC().Format(time.RFC3339),
		string(hash),
		updatedAt,
		updatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return model.User{}, ErrConflict
		}
		return model.User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.User{}, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (model.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, display_name, email, role, status, term_of_validity, password_hash, created_at, updated_at FROM users WHERE id = ?`, id)
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

func (s *Store) ListProjectGroups(ctx context.Context) ([]model.ProjectGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, created_at, updated_at FROM project_groups ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]model.ProjectGroup, 0)
	for rows.Next() {
		group, err := scanProjectGroup(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, group)
	}
	return list, rows.Err()
}

func (s *Store) GetProjectGroup(ctx context.Context, id int64) (model.ProjectGroup, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, created_at, updated_at FROM project_groups WHERE id = ?`, id)
	group, err := scanProjectGroup(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.ProjectGroup{}, ErrNotFound
		}
		return model.ProjectGroup{}, err
	}
	return group, nil
}

func (s *Store) CreateProjectGroup(ctx context.Context, group model.ProjectGroup) (model.ProjectGroup, error) {
	now := time.Now().UTC()
	group.CreatedAt = now
	group.UpdatedAt = now
	res, err := s.db.ExecContext(ctx, `INSERT INTO project_groups (name, description, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		group.Name, group.Description, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return model.ProjectGroup{}, err
	}
	group.ID, _ = res.LastInsertId()
	return group, nil
}

func (s *Store) UpdateProjectGroup(ctx context.Context, group model.ProjectGroup) (model.ProjectGroup, error) {
	group.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE project_groups SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		group.Name, group.Description, group.UpdatedAt.Format(time.RFC3339), group.ID)
	if err != nil {
		return model.ProjectGroup{}, err
	}
	return s.GetProjectGroup(ctx, group.ID)
}

func (s *Store) DeleteProjectGroup(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM project_groups WHERE id = ?`, id)
	return err
}

func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, p.group_id, COALESCE(g.name, ''), p.name, p.keywords, p.description, p.status, p.created_at, p.updated_at
FROM projects p
LEFT JOIN project_groups g ON g.id = p.group_id
ORDER BY p.updated_at DESC, p.id DESC`)
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

func (s *Store) GetProject(ctx context.Context, id int64) (model.Project, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT p.id, p.group_id, COALESCE(g.name, ''), p.name, p.keywords, p.description, p.status, p.created_at, p.updated_at
FROM projects p
LEFT JOIN project_groups g ON g.id = p.group_id
WHERE p.id = ?`, id)
	project, err := scanProject(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.Project{}, ErrNotFound
		}
		return model.Project{}, err
	}
	return project, nil
}

func (s *Store) CreateProject(ctx context.Context, project model.Project) (model.Project, error) {
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now
	project.Status = nonEmpty(project.Status, "active")
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects (group_id, name, keywords, description, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		project.GroupID, project.Name, project.Keywords, project.Description, project.Status,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return model.Project{}, err
	}
	project.ID, _ = res.LastInsertId()
	return s.GetProject(ctx, project.ID)
}

func (s *Store) UpdateProject(ctx context.Context, project model.Project) (model.Project, error) {
	project.UpdatedAt = time.Now().UTC()
	project.Status = nonEmpty(project.Status, "active")
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET group_id = ?, name = ?, keywords = ?, description = ?, status = ?, updated_at = ? WHERE id = ?`,
		project.GroupID, project.Name, project.Keywords, project.Description, project.Status, project.UpdatedAt.Format(time.RFC3339), project.ID,
	)
	if err != nil {
		return model.Project{}, err
	}
	return s.GetProject(ctx, project.ID)
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	return err
}

func (s *Store) ListMonitorRules(ctx context.Context) ([]model.MonitorRule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.id, r.project_id, COALESCE(p.name, ''), r.name, r.include_keywords, r.exclude_keywords, r.channels, r.severity, r.status, r.created_at, r.updated_at
FROM monitor_rules r
LEFT JOIN projects p ON p.id = r.project_id
ORDER BY r.updated_at DESC, r.id DESC`)
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

func (s *Store) ListActiveMonitorRules(ctx context.Context) ([]model.MonitorRule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.id, r.project_id, COALESCE(p.name, ''), r.name, r.include_keywords, r.exclude_keywords, r.channels, r.severity, r.status, r.created_at, r.updated_at
FROM monitor_rules r
LEFT JOIN projects p ON p.id = r.project_id
WHERE r.status = 'active'
ORDER BY r.updated_at DESC, r.id DESC`)
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

func (s *Store) GetMonitorRule(ctx context.Context, id int64) (model.MonitorRule, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT r.id, r.project_id, COALESCE(p.name, ''), r.name, r.include_keywords, r.exclude_keywords, r.channels, r.severity, r.status, r.created_at, r.updated_at
FROM monitor_rules r
LEFT JOIN projects p ON p.id = r.project_id
WHERE r.id = ?`, id)
	rule, err := scanMonitorRule(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.MonitorRule{}, ErrNotFound
		}
		return model.MonitorRule{}, err
	}
	return rule, nil
}

func (s *Store) CreateMonitorRule(ctx context.Context, rule model.MonitorRule) (model.MonitorRule, error) {
	now := time.Now().UTC()
	rule.Status = nonEmpty(rule.Status, "active")
	res, err := s.db.ExecContext(ctx, `INSERT INTO monitor_rules (project_id, name, include_keywords, exclude_keywords, channels, severity, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ProjectID, rule.Name, rule.IncludeKeywords, rule.ExcludeKeywords, rule.Channels, nonEmpty(rule.Severity, "medium"), rule.Status,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return model.MonitorRule{}, err
	}
	rule.ID, _ = res.LastInsertId()
	return s.GetMonitorRule(ctx, rule.ID)
}

func (s *Store) UpdateMonitorRule(ctx context.Context, rule model.MonitorRule) (model.MonitorRule, error) {
	rule.UpdatedAt = time.Now().UTC()
	rule.Status = nonEmpty(rule.Status, "active")
	_, err := s.db.ExecContext(ctx, `UPDATE monitor_rules SET project_id = ?, name = ?, include_keywords = ?, exclude_keywords = ?, channels = ?, severity = ?, status = ?, updated_at = ? WHERE id = ?`,
		rule.ProjectID, rule.Name, rule.IncludeKeywords, rule.ExcludeKeywords, rule.Channels, nonEmpty(rule.Severity, "medium"), rule.Status, rule.UpdatedAt.Format(time.RFC3339), rule.ID,
	)
	if err != nil {
		return model.MonitorRule{}, err
	}
	return s.GetMonitorRule(ctx, rule.ID)
}

func (s *Store) DeleteMonitorRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM monitor_rules WHERE id = ?`, id)
	return err
}

func (s *Store) SearchItemsFTS(ctx context.Context, filter model.ArticleFilter) (model.SearchResult, error) {
	if s.Driver() == "postgres" {
		return s.searchItemsLike(ctx, filter)
	}
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.Keyword == "" {
		list, err := s.ListItems(ctx, filter)
		if err != nil {
			return model.SearchResult{}, err
		}
		return model.SearchResult{Items: list.Items, Keyword: "", Total: list.Total, Page: filter.Page, PageSize: filter.PageSize}, nil
	}

	whereParts := []string{"items_fts MATCH ?", "NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = i.id AND item_tags.tag = 'deleted')"}
	args := []any{filter.Keyword}
	joins := "JOIN items i ON i.id = f.rowid"
	if filter.ProjectID > 0 {
		joins += " JOIN item_relations ir ON ir.item_id = i.id"
		whereParts = append(whereParts, "ir.project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.SourceType != "" {
		whereParts = append(whereParts, "i.source_type = ?")
		args = append(args, filter.SourceType)
	}
	if filter.Start != "" {
		whereParts = append(whereParts, "i.captured_at >= ?")
		args = append(args, filter.Start)
	}
	if filter.End != "" {
		whereParts = append(whereParts, "i.captured_at <= ?")
		args = append(args, filter.End)
	}
	where := " WHERE " + strings.Join(whereParts, " AND ")

	var total int
	countQuery := `SELECT COUNT(DISTINCT i.id) FROM items_fts f ` + joins + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return model.SearchResult{}, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := `
SELECT DISTINCT i.id, i.source_type, i.source_key, i.title, i.content, i.summary, i.publish_time, i.publish_time_text, i.detail_url, i.source_url, i.tag_flags, i.from_text, i.external_source_host, i.is_vip, i.has_image, i.raw_payload, i.captured_at, i.created_at, i.updated_at
FROM items_fts f
` + joins + where + `
ORDER BY bm25(items_fts), i.captured_at DESC
LIMIT ? OFFSET ?`
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return model.SearchResult{}, err
	}
	defer rows.Close()
	items, err := scanItems(rows)
	if err != nil {
		return model.SearchResult{}, err
	}
	if err := s.attachProjectIDs(ctx, items); err != nil {
		return model.SearchResult{}, err
	}
	return model.SearchResult{Items: items, Keyword: filter.Keyword, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Store) searchItemsLike(ctx context.Context, filter model.ArticleFilter) (model.SearchResult, error) {
	filter.Page = max(filter.Page, 1)
	filter.PageSize = max(filter.PageSize, 1)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.Keyword == "" {
		list, err := s.ListItems(ctx, filter)
		if err != nil {
			return model.SearchResult{}, err
		}
		return model.SearchResult{Items: list.Items, Keyword: "", Total: list.Total, Page: filter.Page, PageSize: filter.PageSize}, nil
	}

	whereParts := []string{"NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')"}
	args := []any{}
	joins := ""
	if filter.ProjectID > 0 {
		joins += " JOIN item_relations ir ON ir.item_id = items.id"
		whereParts = append(whereParts, "ir.project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.SourceType != "" {
		whereParts = append(whereParts, "items.source_type = ?")
		args = append(args, filter.SourceType)
	}
	like := "%" + escapeLike(filter.Keyword) + "%"
	whereParts = append(whereParts, "(items.title LIKE ? ESCAPE '\\' OR items.content LIKE ? ESCAPE '\\' OR items.summary LIKE ? ESCAPE '\\')")
	args = append(args, like, like, like)
	if filter.Start != "" {
		whereParts = append(whereParts, "items.captured_at >= ?")
		args = append(args, filter.Start)
	}
	if filter.End != "" {
		whereParts = append(whereParts, "items.captured_at <= ?")
		args = append(args, filter.End)
	}
	where := " WHERE " + strings.Join(whereParts, " AND ")

	var total int
	countQuery := "SELECT COUNT(DISTINCT items.id) FROM items " + joins + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return model.SearchResult{}, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := "SELECT DISTINCT items.id, items.source_type, items.source_key, items.title, items.content, items.summary, items.publish_time, items.publish_time_text, items.detail_url, items.source_url, items.tag_flags, items.from_text, items.external_source_host, items.is_vip, items.has_image, items.raw_payload, items.captured_at, items.created_at, items.updated_at FROM items " + joins + where + " ORDER BY items.captured_at DESC, items.id DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, filter.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return model.SearchResult{}, err
	}
	defer rows.Close()
	items, err := scanItems(rows)
	if err != nil {
		return model.SearchResult{}, err
	}
	if err := s.attachProjectIDs(ctx, items); err != nil {
		return model.SearchResult{}, err
	}
	return model.SearchResult{Items: items, Keyword: filter.Keyword, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Store) Overview(ctx context.Context) (model.Overview, error) {
	var overview model.Overview
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')`).Scan(&overview.ArticleCount); err != nil {
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
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM monitor_rules WHERE status = 'active'`).Scan(&overview.AlertRuleCount); err != nil {
		return overview, err
	}
	return overview, nil
}

func (s *Store) BuildDashboardSnapshot(ctx context.Context) (model.DashboardSnapshot, error) {
	overview, err := s.Overview(ctx)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	trends, err := s.BuildTrendPoints(ctx)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	sources, err := s.BuildSourceBreakdowns(ctx)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	keywords, err := s.BuildKeywordHotspots(ctx)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	return model.DashboardSnapshot{
		Overview:  overview,
		Trends:    trends,
		Sources:   sources,
		Keywords:  keywords,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (s *Store) BuildTrendPoints(ctx context.Context) ([]model.TrendPoint, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT substr(captured_at, 1, 10) AS day, COUNT(1)
FROM items
GROUP BY substr(captured_at, 1, 10)
ORDER BY day DESC
LIMIT 7`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := make([]model.TrendPoint, 0)
	for rows.Next() {
		var point model.TrendPoint
		if err := rows.Scan(&point.Label, &point.Count); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Label < points[j].Label })
	return points, rows.Err()
}

func (s *Store) BuildSourceBreakdowns(ctx context.Context) ([]model.SourceBreakdown, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_type, COUNT(1) FROM items GROUP BY source_type ORDER BY COUNT(1) DESC, source_type ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SourceBreakdown, 0)
	for rows.Next() {
		var row model.SourceBreakdown
		if err := rows.Scan(&row.SourceType, &row.Count); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) BuildKeywordHotspots(ctx context.Context) ([]model.KeywordHotspot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT title, summary, content FROM items ORDER BY captured_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var title, summary, content string
		if err := rows.Scan(&title, &summary, &content); err != nil {
			return nil, err
		}
		for _, keyword := range extractKeywords(title + " " + summary + " " + content) {
			counts[keyword]++
		}
	}

	result := make([]model.KeywordHotspot, 0, len(counts))
	for keyword, count := range counts {
		result = append(result, model.KeywordHotspot{Keyword: keyword, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Keyword < result[j].Keyword
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result, nil
}

func (s *Store) RefreshAnalysis(ctx context.Context) (model.DashboardSnapshot, error) {
	snapshot, err := s.BuildDashboardSnapshot(ctx)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	now := time.Now().UTC()

	if err := s.UpsertAnalysisSnapshot(ctx, model.AnalysisSnapshot{
		Scope:     "system",
		ScopeID:   0,
		Title:     "Dashboard Snapshot",
		Payload:   string(payload),
		CreatedAt: now,
	}); err != nil {
		return model.DashboardSnapshot{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM trend_points WHERE scope = 'system' AND scope_id = 0`); err != nil {
		return model.DashboardSnapshot{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM source_breakdowns WHERE scope = 'system' AND scope_id = 0`); err != nil {
		return model.DashboardSnapshot{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM keyword_hotspots WHERE scope = 'system' AND scope_id = 0`); err != nil {
		return model.DashboardSnapshot{}, err
	}
	for _, point := range snapshot.Trends {
		if _, err = tx.ExecContext(ctx, `INSERT INTO trend_points (scope, scope_id, label, count, created_at) VALUES ('system', 0, ?, ?, ?)`,
			point.Label, point.Count, now.Format(time.RFC3339)); err != nil {
			return model.DashboardSnapshot{}, err
		}
	}
	for _, source := range snapshot.Sources {
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_breakdowns (scope, scope_id, source_type, count, created_at) VALUES ('system', 0, ?, ?, ?)`,
			source.SourceType, source.Count, now.Format(time.RFC3339)); err != nil {
			return model.DashboardSnapshot{}, err
		}
	}
	for _, keyword := range snapshot.Keywords {
		if _, err = tx.ExecContext(ctx, `INSERT INTO keyword_hotspots (scope, scope_id, keyword, count, created_at) VALUES ('system', 0, ?, ?, ?)`,
			keyword.Keyword, keyword.Count, now.Format(time.RFC3339)); err != nil {
			return model.DashboardSnapshot{}, err
		}
	}
	err = tx.Commit()
	return snapshot, err
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

func (s *Store) ListTrendPoints(ctx context.Context) ([]model.TrendPoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT label, count FROM trend_points WHERE scope = 'system' AND scope_id = 0 ORDER BY label ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.TrendPoint, 0)
	for rows.Next() {
		var point model.TrendPoint
		if err := rows.Scan(&point.Label, &point.Count); err != nil {
			return nil, err
		}
		result = append(result, point)
	}
	return result, rows.Err()
}

func (s *Store) ListSourceBreakdowns(ctx context.Context) ([]model.SourceBreakdown, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_type, count FROM source_breakdowns WHERE scope = 'system' AND scope_id = 0 ORDER BY count DESC, source_type ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SourceBreakdown, 0)
	for rows.Next() {
		var row model.SourceBreakdown
		if err := rows.Scan(&row.SourceType, &row.Count); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) ListKeywordHotspots(ctx context.Context) ([]model.KeywordHotspot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT keyword, count FROM keyword_hotspots WHERE scope = 'system' AND scope_id = 0 ORDER BY count DESC, keyword ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.KeywordHotspot, 0)
	for rows.Next() {
		var row model.KeywordHotspot
		if err := rows.Scan(&row.Keyword, &row.Count); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) ListReports(ctx context.Context, projectID int64) ([]model.Report, error) {
	args := []any{}
	query := `SELECT id, project_id, title, summary, content, status, created_at, updated_at FROM reports`
	if projectID > 0 {
		query += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY updated_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *Store) GetReport(ctx context.Context, id int64) (model.Report, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, project_id, title, summary, content, status, created_at, updated_at FROM reports WHERE id = ?`, id)
	report, err := scanReport(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.Report{}, ErrNotFound
		}
		return model.Report{}, err
	}
	return report, nil
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
	if err := s.seedReportSections(ctx, report); err != nil {
		return model.Report{}, err
	}
	return report, nil
}

func (s *Store) BatchDeleteReports(ctx context.Context, ids []int64) error {
	return s.BatchUpdateReportStatus(ctx, ids, "archived")
}

func (s *Store) BatchUpdateReportStatus(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return errors.New("report_ids required")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "draft", "generated", "archived":
	default:
		return errors.New("unsupported report status")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)
	args = append(args, status, now)
	for _, id := range ids {
		if id <= 0 {
			return errors.New("invalid report id")
		}
		args = append(args, id)
	}
	query := `UPDATE reports SET status = ?, updated_at = ? WHERE id IN (` + placeholders + `)`
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) seedReportSections(ctx context.Context, report model.Report) error {
	parts := strings.Split(report.Content, "\n")
	now := time.Now().UTC().Format(time.RFC3339)
	order := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		order++
		if _, err := s.db.ExecContext(ctx, `INSERT INTO report_sections (report_id, heading, content, sort_order, created_at) VALUES (?, ?, ?, ?, ?)`,
			report.ID, nonEmpty(report.Title, "报告"), part, order, now); err != nil {
			return err
		}
	}
	return nil
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

func (s *Store) CreateFeedback(ctx context.Context, feedback model.Feedback) (model.Feedback, error) {
	feedback.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO feedback (user_id, title, content, created_at) VALUES (?, ?, ?, ?)`,
		feedback.UserID, feedback.Title, feedback.Content, feedback.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return model.Feedback{}, err
	}
	feedback.ID, _ = res.LastInsertId()
	return feedback, nil
}

func (s *Store) ListFeedback(ctx context.Context, limit int) ([]model.Feedback, error) {
	limit = max(limit, 1)
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, title, content, created_at FROM feedback ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Feedback, 0, limit)
	for rows.Next() {
		var item model.Feedback
		var createdAt string
		if err := rows.Scan(&item.ID, &item.UserID, &item.Title, &item.Content, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt = mustParseRFC3339(createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeleteFeedback(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM feedback WHERE id = ?`, id)
	return err
}

func (s *Store) MarkItemRead(ctx context.Context, userID, itemID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO item_reads (user_id, item_id, created_at) VALUES (?, ?, ?)`, userID, itemID, now)
	return err
}

func (s *Store) DeleteItemRead(ctx context.Context, userID, itemID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM item_reads WHERE user_id = ? AND item_id = ?`, userID, itemID)
	return err
}

func (s *Store) ToggleFavorite(ctx context.Context, userID, itemID int64) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM favorites WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND item_id = ?`, userID, itemID)
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO favorites (user_id, item_id, created_at) VALUES (?, ?, ?)`, userID, itemID, now)
	return true, err
}

func (s *Store) ListTaskRuns(ctx context.Context, limit int) ([]model.TaskRun, error) {
	limit = max(limit, 1)
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_name, status, message, started_at, finished_at FROM task_runs ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]model.TaskRun, 0)
	for rows.Next() {
		var run model.TaskRun
		var startedAt string
		var finishedAt sql.NullString
		if err := rows.Scan(&run.ID, &run.TaskName, &run.Status, &run.Message, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		run.StartedAt = mustParseRFC3339(startedAt)
		if finishedAt.Valid {
			value := mustParseRFC3339(finishedAt.String)
			run.FinishedAt = &value
		}
		list = append(list, run)
	}
	return list, rows.Err()
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

func (s *Store) CreateAuditLog(ctx context.Context, entry model.AuditLog) (model.AuditLog, error) {
	entry.Username = strings.TrimSpace(entry.Username)
	entry.Action = strings.TrimSpace(entry.Action)
	entry.Resource = strings.TrimSpace(entry.Resource)
	entry.DetailJSON = strings.TrimSpace(entry.DetailJSON)
	if entry.Action == "" {
		return model.AuditLog{}, errors.New("action required")
	}
	if entry.DetailJSON == "" {
		entry.DetailJSON = "{}"
	}
	entry.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs (user_id, username, action, resource, detail_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.UserID,
		entry.Username,
		entry.Action,
		entry.Resource,
		entry.DetailJSON,
		entry.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.AuditLog{}, err
	}
	entry.ID, _ = res.LastInsertId()
	return entry, nil
}

func (s *Store) ListAuditLogs(ctx context.Context, limit int, userID int64, action string) ([]model.AuditLog, error) {
	limit = max(limit, 1)
	args := []any{}
	var query strings.Builder
	query.WriteString(`SELECT id, user_id, username, action, resource, detail_json, created_at FROM audit_logs WHERE 1=1`)
	if userID > 0 {
		query.WriteString(` AND user_id = ?`)
		args = append(args, userID)
	}
	if trimmed := strings.TrimSpace(action); trimmed != "" {
		query.WriteString(` AND action = ?`)
		args = append(args, trimmed)
	}
	query.WriteString(` ORDER BY created_at DESC, id DESC LIMIT ?`)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := make([]model.AuditLog, 0, limit)
	for rows.Next() {
		var entry model.AuditLog
		var createdAt string
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.Username, &entry.Action, &entry.Resource, &entry.DetailJSON, &createdAt); err != nil {
			return nil, err
		}
		entry.CreatedAt = mustParseRFC3339(createdAt)
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

func (s *Store) EnsureSeedData(ctx context.Context) error {
	var count int
	now := time.Now().UTC().Format(time.RFC3339)

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM project_groups`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO project_groups (name, description, created_at, updated_at) VALUES ('默认项目组', 'Go 重构后的默认项目组', ?, ?)`, now, now); err != nil {
			return err
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM system_notices`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO system_notices (title, content, created_at) VALUES ('Go 系统已启用', '当前门户已切换到 Go 多服务骨架。', ?)`, now); err != nil {
			return err
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(scanner scanner) (model.User, error) {
	var user model.User
	var createdAt, updatedAt, termOfValidity string
	if err := scanner.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.Role, &user.Status, &termOfValidity, &user.PasswordHash, &createdAt, &updatedAt); err != nil {
		return model.User{}, err
	}
	user.TermOfValidity = mustParseRFC3339(termOfValidity)
	if user.TermOfValidity.IsZero() {
		user.TermOfValidity = time.Date(2099, 1, 19, 0, 0, 0, 0, time.UTC)
	}
	user.CreatedAt = mustParseRFC3339(createdAt)
	user.UpdatedAt = mustParseRFC3339(updatedAt)
	return user, nil
}

func scanProjectGroup(scanner scanner) (model.ProjectGroup, error) {
	var group model.ProjectGroup
	var createdAt, updatedAt string
	if err := scanner.Scan(&group.ID, &group.Name, &group.Description, &createdAt, &updatedAt); err != nil {
		return model.ProjectGroup{}, err
	}
	group.CreatedAt = mustParseRFC3339(createdAt)
	group.UpdatedAt = mustParseRFC3339(updatedAt)
	return group, nil
}

func scanProject(scanner scanner) (model.Project, error) {
	var project model.Project
	var createdAt, updatedAt string
	if err := scanner.Scan(&project.ID, &project.GroupID, &project.GroupName, &project.Name, &project.Keywords, &project.Description, &project.Status, &createdAt, &updatedAt); err != nil {
		return model.Project{}, err
	}
	project.CreatedAt = mustParseRFC3339(createdAt)
	project.UpdatedAt = mustParseRFC3339(updatedAt)
	return project, nil
}

func scanMonitorRule(scanner scanner) (model.MonitorRule, error) {
	var rule model.MonitorRule
	var createdAt, updatedAt string
	if err := scanner.Scan(&rule.ID, &rule.ProjectID, &rule.ProjectName, &rule.Name, &rule.IncludeKeywords, &rule.ExcludeKeywords, &rule.Channels, &rule.Severity, &rule.Status, &createdAt, &updatedAt); err != nil {
		return model.MonitorRule{}, err
	}
	rule.CreatedAt = mustParseRFC3339(createdAt)
	rule.UpdatedAt = mustParseRFC3339(updatedAt)
	return rule, nil
}

func scanReport(scanner scanner) (model.Report, error) {
	var report model.Report
	var createdAt, updatedAt string
	if err := scanner.Scan(&report.ID, &report.ProjectID, &report.Title, &report.Summary, &report.Content, &report.Status, &createdAt, &updatedAt); err != nil {
		return model.Report{}, err
	}
	report.CreatedAt = mustParseRFC3339(createdAt)
	report.UpdatedAt = mustParseRFC3339(updatedAt)
	return report, nil
}

func extractKeywords(text string) []string {
	normalized := strings.NewReplacer("，", " ", "。", " ", ",", " ", ".", " ", "\n", " ", "\r", " ", "\t", " ").Replace(text)
	fields := strings.Fields(normalized)
	counts := make(map[string]int)
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len([]rune(field)) < 2 {
			continue
		}
		counts[field]++
	}
	result := make([]string, 0, len(counts))
	for field, count := range counts {
		if count >= 2 {
			result = append(result, field)
		}
	}
	if len(result) == 0 {
		for field := range counts {
			result = append(result, field)
		}
	}
	sort.Strings(result)
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}
