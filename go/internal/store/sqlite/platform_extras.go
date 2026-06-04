package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Store) UpdateUserProfile(ctx context.Context, userID int64, update model.UserProfileUpdate) (model.User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `UPDATE users SET display_name = ?, email = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(update.DisplayName), strings.TrimSpace(update.Email), now, userID,
	)
	if err != nil {
		return model.User{}, err
	}
	return s.GetUserByID(ctx, userID)
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		string(hash), time.Now().UTC().Format(time.RFC3339), userID,
	)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateCaptcha(ctx context.Context, ttl time.Duration) (model.Captcha, error) {
	now := time.Now().UTC()
	code := strconv.Itoa(int(now.UnixNano()%9000) + 1000)
	captcha := model.Captcha{
		ID:        uuid.NewString(),
		Code:      code,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO captchas (id, code, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		captcha.ID, captcha.Code, captcha.ExpiresAt.Format(time.RFC3339), captcha.CreatedAt.Format(time.RFC3339),
	)
	return captcha, err
}

func (s *Store) VerifyCaptcha(ctx context.Context, id, code string) error {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM captchas WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	row := s.db.QueryRowContext(ctx, `SELECT code, expires_at, created_at FROM captchas WHERE id = ?`, strings.TrimSpace(id))
	var expected, expiresAt, createdAt string
	if err := row.Scan(&expected, &expiresAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if mustParseRFC3339(expiresAt).Before(time.Now().UTC()) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM captchas WHERE id = ?`, id)
		return ErrNotFound
	}
	if strings.TrimSpace(code) != expected {
		return ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM captchas WHERE id = ?`, id)
	return err
}

func (s *Store) GetUserPreference(ctx context.Context, userID int64) (model.UserPreference, error) {
	row := s.db.QueryRowContext(ctx, `SELECT user_id, language, theme, default_search_mode, article_page_size, email_notifications, updated_at FROM user_preferences WHERE user_id = ?`, userID)
	return scanUserPreference(row)
}

func (s *Store) UpsertUserPreference(ctx context.Context, pref model.UserPreference) (model.UserPreference, error) {
	now := time.Now().UTC()
	pref.Language = nonEmpty(pref.Language, "zh-CN")
	pref.Theme = nonEmpty(pref.Theme, "light")
	pref.DefaultSearchMode = nonEmpty(pref.DefaultSearchMode, "default")
	if pref.ArticlePageSize <= 0 {
		pref.ArticlePageSize = 20
	}
	pref.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_preferences (user_id, language, theme, default_search_mode, article_page_size, email_notifications, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
	language = excluded.language,
	theme = excluded.theme,
	default_search_mode = excluded.default_search_mode,
	article_page_size = excluded.article_page_size,
	email_notifications = excluded.email_notifications,
	updated_at = excluded.updated_at`,
		pref.UserID, pref.Language, pref.Theme, pref.DefaultSearchMode, pref.ArticlePageSize, boolToInt(pref.EmailNotifications), pref.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.UserPreference{}, err
	}
	return s.GetUserPreference(ctx, pref.UserID)
}

func (s *Store) GetPopupState(ctx context.Context, userID int64, key string) (model.PopupState, error) {
	row := s.db.QueryRowContext(ctx, `SELECT user_id, popup_key, dismissed, count, dismissed_at, updated_at FROM popup_states WHERE user_id = ? AND popup_key = ?`, userID, strings.TrimSpace(key))
	return scanPopupState(row)
}

func (s *Store) UpsertPopupState(ctx context.Context, state model.PopupState) (model.PopupState, error) {
	now := time.Now().UTC()
	state.Key = nonEmpty(state.Key, "default")
	state.UpdatedAt = now
	var dismissedAt any
	if state.Dismissed {
		value := now
		state.DismissedAt = &value
		dismissedAt = value.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO popup_states (user_id, popup_key, dismissed, count, dismissed_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, popup_key) DO UPDATE SET
	dismissed = excluded.dismissed,
	count = excluded.count,
	dismissed_at = excluded.dismissed_at,
	updated_at = excluded.updated_at`,
		state.UserID, state.Key, boolToInt(state.Dismissed), max(state.Count, 0), dismissedAt, state.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.PopupState{}, err
	}
	return s.GetPopupState(ctx, state.UserID, state.Key)
}

func (s *Store) GetMailConfig(ctx context.Context) (model.MailConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT enabled, smtp_host, smtp_port, username, password, sender_name, sender_email, updated_at FROM mail_configs WHERE id = 1`)
	var cfg model.MailConfig
	var enabled int
	var updatedAt string
	if err := row.Scan(&enabled, &cfg.SMTPHost, &cfg.SMTPPort, &cfg.Username, &cfg.Password, &cfg.SenderName, &cfg.SenderEmail, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return model.MailConfig{
				SMTPPort:  25,
				UpdatedAt: time.Time{},
			}, nil
		}
		return model.MailConfig{}, err
	}
	cfg.Enabled = enabled == 1
	cfg.UpdatedAt = mustParseRFC3339(updatedAt)
	return cfg, nil
}

func (s *Store) UpsertMailConfig(ctx context.Context, cfg model.MailConfig) (model.MailConfig, error) {
	now := time.Now().UTC()
	if cfg.SMTPPort <= 0 {
		cfg.SMTPPort = 25
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mail_configs (id, enabled, smtp_host, smtp_port, username, password, sender_name, sender_email, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	enabled = excluded.enabled,
	smtp_host = excluded.smtp_host,
	smtp_port = excluded.smtp_port,
	username = excluded.username,
	password = excluded.password,
	sender_name = excluded.sender_name,
	sender_email = excluded.sender_email,
	updated_at = excluded.updated_at`,
		boolToInt(cfg.Enabled), strings.TrimSpace(cfg.SMTPHost), cfg.SMTPPort, strings.TrimSpace(cfg.Username), cfg.Password, strings.TrimSpace(cfg.SenderName), strings.TrimSpace(cfg.SenderEmail), cfg.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.MailConfig{}, err
	}
	return s.GetMailConfig(ctx)
}

func (s *Store) GetWarningSetting(ctx context.Context, projectID int64) (model.WarningSetting, error) {
	row := s.db.QueryRowContext(ctx, `SELECT project_id, warning_setting_id, enabled, warning_status, warning_name, warning_word, warning_classify, warning_content, warning_similar, warning_match, warning_deduplication, warning_source, warning_receive_time, weekend_warning, warning_interval, channels, threshold, recipients, description, updated_at FROM warning_settings WHERE project_id = ?`, projectID)
	var setting model.WarningSetting
	var enabled int
	var updatedAt string
	if err := row.Scan(
		&setting.ProjectID,
		&setting.WarningSettingID,
		&enabled,
		&setting.WarningStatus,
		&setting.WarningName,
		&setting.WarningWord,
		&setting.WarningClassify,
		&setting.WarningContent,
		&setting.WarningSimilar,
		&setting.WarningMatch,
		&setting.WarningDeduplication,
		&setting.WarningSource,
		&setting.WarningReceiveTime,
		&setting.WeekendWarning,
		&setting.WarningInterval,
		&setting.Channels,
		&setting.Threshold,
		&setting.Recipients,
		&setting.Description,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.WarningSetting{
				ProjectID:            projectID,
				WarningStatus:        0,
				WarningName:          "预警",
				WarningClassify:      "1,2,3,4,5,6,7,8,9,10,11",
				WarningContent:       0,
				WarningSimilar:       0,
				WarningMatch:         2,
				WarningDeduplication: 0,
				WarningSource:        `{"type":"1","email":""}`,
				WarningReceiveTime:   `{"start":"00:00","end":"23:00"}`,
				WeekendWarning:       1,
				WarningInterval:      `{"type":"1","time":"1"}`,
				Enabled:              false,
				Channels:             "1,2,3,4,5,6,7,8,9,10,11",
				Threshold:            1,
				Recipients:           "",
				Description:          "预警",
			}, nil
		}
		return model.WarningSetting{}, err
	}
	if setting.WarningStatus == 0 && enabled == 1 {
		setting.WarningStatus = 1
	}
	setting.Enabled = enabled == 1
	setting.UpdatedAt = mustParseRFC3339(updatedAt)
	return setting, nil
}

func (s *Store) UpsertWarningSetting(ctx context.Context, setting model.WarningSetting) (model.WarningSetting, error) {
	now := time.Now().UTC()
	if setting.Threshold <= 0 {
		setting.Threshold = 80
	}
	if setting.WarningStatus == 0 && setting.Enabled {
		setting.WarningStatus = 1
	}
	if setting.WarningName == "" {
		setting.WarningName = nonEmpty(setting.Description, "预警")
	}
	if setting.WarningWord == "" && setting.Description != "" {
		setting.WarningWord = setting.Description
	}
	if setting.WarningClassify == "" {
		setting.WarningClassify = setting.Channels
	}
	if setting.WarningSource == "" {
		setting.WarningSource = `{"type":"1","email":"` + strings.TrimSpace(setting.Recipients) + `"}`
	}
	if setting.WarningReceiveTime == "" {
		setting.WarningReceiveTime = `{"start":"","end":""}`
	}
	if setting.WarningInterval == "" {
		setting.WarningInterval = `{"type":"1","time":"` + strconv.Itoa(max(setting.Threshold, 1)) + `"}`
	}
	setting.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO warning_settings (project_id, warning_setting_id, enabled, warning_status, warning_name, warning_word, warning_classify, warning_content, warning_similar, warning_match, warning_deduplication, warning_source, warning_receive_time, weekend_warning, warning_interval, channels, threshold, recipients, description, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
	warning_setting_id = excluded.warning_setting_id,
	enabled = excluded.enabled,
	warning_status = excluded.warning_status,
	warning_name = excluded.warning_name,
	warning_word = excluded.warning_word,
	warning_classify = excluded.warning_classify,
	warning_content = excluded.warning_content,
	warning_similar = excluded.warning_similar,
	warning_match = excluded.warning_match,
	warning_deduplication = excluded.warning_deduplication,
	warning_source = excluded.warning_source,
	warning_receive_time = excluded.warning_receive_time,
	weekend_warning = excluded.weekend_warning,
	warning_interval = excluded.warning_interval,
	channels = excluded.channels,
	threshold = excluded.threshold,
	recipients = excluded.recipients,
	description = excluded.description,
	updated_at = excluded.updated_at`,
		setting.ProjectID, setting.WarningSettingID, boolToInt(setting.Enabled), setting.WarningStatus, strings.TrimSpace(setting.WarningName), strings.TrimSpace(setting.WarningWord), strings.TrimSpace(setting.WarningClassify), setting.WarningContent, setting.WarningSimilar, setting.WarningMatch, setting.WarningDeduplication, strings.TrimSpace(setting.WarningSource), strings.TrimSpace(setting.WarningReceiveTime), setting.WeekendWarning, strings.TrimSpace(setting.WarningInterval), strings.TrimSpace(setting.Channels), setting.Threshold, strings.TrimSpace(setting.Recipients), strings.TrimSpace(setting.Description), setting.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.WarningSetting{}, err
	}
	return s.GetWarningSetting(ctx, setting.ProjectID)
}

func (s *Store) GetOpinionCondition(ctx context.Context, projectID int64) (model.OpinionCondition, error) {
	row := s.db.QueryRowContext(ctx, `SELECT project_id, opinion_condition_id, time, precise, emotion, similar, sort, matchs, times, timee, classify, websitename, author, organization, categorylable, enterprisetype, hightechtype, policylableflag, datasource_type, event_index, industry_index, province, city, create_time, updated_at FROM opinion_conditions WHERE project_id = ?`, projectID)
	return scanOpinionCondition(row)
}

func (s *Store) UpsertOpinionCondition(ctx context.Context, condition model.OpinionCondition) (model.OpinionCondition, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if condition.Time == 0 {
		condition.Time = 4
	}
	if strings.TrimSpace(condition.Emotion) == "" {
		condition.Emotion = "[1,2,3]"
	}
	if condition.Sort == 0 {
		condition.Sort = 1
	}
	if condition.Matchs == 0 {
		condition.Matchs = 1
	}
	if strings.TrimSpace(condition.CreateTime) == "" {
		condition.CreateTime = time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	condition.UpdatedAt = mustParseRFC3339(now)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO opinion_conditions (
	project_id, opinion_condition_id, time, precise, emotion, similar, sort, matchs, times, timee, classify, websitename, author, organization, categorylable, enterprisetype, hightechtype, policylableflag, datasource_type, event_index, industry_index, province, city, create_time, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
	opinion_condition_id = excluded.opinion_condition_id,
	time = excluded.time,
	precise = excluded.precise,
	emotion = excluded.emotion,
	similar = excluded.similar,
	sort = excluded.sort,
	matchs = excluded.matchs,
	times = excluded.times,
	timee = excluded.timee,
	classify = excluded.classify,
	websitename = excluded.websitename,
	author = excluded.author,
	organization = excluded.organization,
	categorylable = excluded.categorylable,
	enterprisetype = excluded.enterprisetype,
	hightechtype = excluded.hightechtype,
	policylableflag = excluded.policylableflag,
	datasource_type = excluded.datasource_type,
	event_index = excluded.event_index,
	industry_index = excluded.industry_index,
	province = excluded.province,
	city = excluded.city,
	create_time = excluded.create_time,
	updated_at = excluded.updated_at`,
		condition.ProjectID, condition.OpinionConditionID, condition.Time, condition.Precise, condition.Emotion, condition.Similar, condition.Sort, condition.Matchs, condition.Times, condition.Timee, condition.Classify, condition.Websitename, condition.Author, condition.Organization, condition.Categorylable, condition.Enterprisetype, condition.Hightechtype, condition.Policylableflag, condition.DatasourceType, condition.EventIndex, condition.IndustryIndex, condition.Province, condition.City, condition.CreateTime, condition.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.OpinionCondition{}, err
	}
	return s.GetOpinionCondition(ctx, condition.ProjectID)
}

func scanOpinionCondition(row scanner) (model.OpinionCondition, error) {
	var condition model.OpinionCondition
	var updatedAt string
	err := row.Scan(
		&condition.ProjectID,
		&condition.OpinionConditionID,
		&condition.Time,
		&condition.Precise,
		&condition.Emotion,
		&condition.Similar,
		&condition.Sort,
		&condition.Matchs,
		&condition.Times,
		&condition.Timee,
		&condition.Classify,
		&condition.Websitename,
		&condition.Author,
		&condition.Organization,
		&condition.Categorylable,
		&condition.Enterprisetype,
		&condition.Hightechtype,
		&condition.Policylableflag,
		&condition.DatasourceType,
		&condition.EventIndex,
		&condition.IndustryIndex,
		&condition.Province,
		&condition.City,
		&condition.CreateTime,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.OpinionCondition{Time: 4, Emotion: "[1,2,3]", Sort: 1, Matchs: 1}, nil
		}
		return model.OpinionCondition{}, err
	}
	condition.UpdatedAt = mustParseRFC3339(updatedAt)
	if condition.Emotion == "" {
		condition.Emotion = "[1,2,3]"
	}
	if condition.Time == 0 {
		condition.Time = 4
	}
	if condition.Sort == 0 {
		condition.Sort = 1
	}
	if condition.Matchs == 0 {
		condition.Matchs = 1
	}
	return condition, nil
}

func (s *Store) RecordItemShare(ctx context.Context, share model.ShareRecord) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO item_shares (user_id, item_id, channel, created_at) VALUES (?, ?, ?, ?)`,
		share.UserID, share.ItemID, nonEmpty(share.Channel, "link"), now.Format(time.RFC3339),
	)
	return err
}

func (s *Store) SetItemEmotion(ctx context.Context, itemID int64, emotion string) error {
	tag := normalizeLegacyEmotionTag(emotion)
	if tag == "" {
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM item_tags WHERE item_id = ? AND tag LIKE 'emotion:%'`, itemID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_tags (item_id, tag, created_at) VALUES (?, ?, ?)`, itemID, tag, now); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *Store) MarkItemDeleted(ctx context.Context, itemID int64) error {
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
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_tags (item_id, tag, created_at) VALUES (?, 'deleted', ?)`, itemID, now); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *Store) SearchItemsAdvanced(ctx context.Context, filter model.ArticleFilter) (model.SearchResult, error) {
	items, err := s.listItemsForAdvancedFilter(ctx, filter)
	if err != nil {
		return model.SearchResult{}, err
	}
	total := len(items)
	page := max(filter.Page, 1)
	pageSize := max(filter.PageSize, 1)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return model.SearchResult{
		Items:    items[start:end],
		Keyword:  filter.Keyword,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func normalizeLegacyEmotionTag(emotion string) string {
	switch strings.TrimSpace(emotion) {
	case "1", "positive", "pos", "positive-emotion":
		return "emotion:1"
	case "2", "neutral", "mid", "neutral-emotion":
		return "emotion:2"
	case "3", "negative", "neg", "negative-emotion":
		return "emotion:3"
	case "":
		return ""
	default:
		if strings.HasPrefix(emotion, "emotion:") {
			return emotion
		}
		return "emotion:" + strings.TrimSpace(emotion)
	}
}

func (s *Store) BuildSearchFacets(ctx context.Context, filter model.ArticleFilter) (model.SearchFacets, error) {
	filter.Page = 1
	filter.PageSize = max(filter.Limit, 500)
	items, err := s.listItemsForAdvancedFilter(ctx, filter)
	if err != nil {
		return model.SearchFacets{}, err
	}
	projectNames := make(map[int64]string)
	projects, err := s.ListProjects(ctx)
	if err == nil {
		for _, project := range projects {
			projectNames[project.ID] = project.Name
		}
	}
	facets := model.SearchFacets{
		Sources: bucketizeStrings(func() []string {
			out := make([]string, 0, len(items))
			for _, item := range items {
				out = append(out, item.SourceType)
			}
			return out
		}()),
		Industries: bucketizeStrings(collectMetadataValues(items, "industry")),
		Provinces:  bucketizeStrings(collectMetadataValues(items, "province")),
		Cities:     bucketizeStrings(collectMetadataValues(items, "city")),
	}
	projectCounter := map[string]int{}
	for _, item := range items {
		for _, projectID := range item.ProjectIDs {
			name := projectNames[projectID]
			if name == "" {
				name = strconv.FormatInt(projectID, 10)
			}
			projectCounter[name]++
		}
	}
	facets.Projects = mapToBuckets(projectCounter)
	return facets, nil
}

func (s *Store) ListSearchOptions(ctx context.Context) (model.SearchOptions, error) {
	items, err := s.listItemsForAdvancedFilter(ctx, model.ArticleFilter{Page: 1, PageSize: 500, Limit: 500})
	if err != nil {
		return model.SearchOptions{}, err
	}
	return model.SearchOptions{
		Industries: uniqueSortedStrings(collectMetadataValues(items, "industry")),
		Provinces:  uniqueSortedStrings(collectMetadataValues(items, "province")),
		Cities:     uniqueSortedStrings(collectMetadataValues(items, "city")),
	}, nil
}

func (s *Store) SaveSearchWord(ctx context.Context, userID int64, searchWord string) error {
	searchWord = strings.TrimSpace(searchWord)
	if userID <= 0 || searchWord == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO search_words (user_id, search_word, created_at) VALUES (?, ?, ?)`,
		userID, searchWord, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListSearchWords(ctx context.Context, userID int64, limit int) ([]model.SearchWordStat, error) {
	if userID <= 0 {
		return []model.SearchWordStat{}, nil
	}
	if limit <= 0 {
		limit = 6
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT search_word, user_id, COUNT(*) AS word_count
FROM search_words
WHERE user_id = ?
GROUP BY user_id, search_word
ORDER BY word_count DESC, MAX(created_at) DESC, search_word ASC
LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SearchWordStat, 0, limit)
	for rows.Next() {
		stat, scanErr := scanSearchWordStat(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

func (s *Store) ListSearchWordSuggestions(ctx context.Context, userID int64, prefix string, limit int) ([]model.SearchWordStat, error) {
	if userID <= 0 {
		return []model.SearchWordStat{}, nil
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return s.ListSearchWords(ctx, userID, limit)
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT search_word, user_id, COUNT(*) AS word_count
FROM search_words
WHERE user_id = ? AND search_word LIKE ? ESCAPE '\'
GROUP BY user_id, search_word
ORDER BY word_count DESC, MAX(created_at) DESC, search_word ASC
LIMIT ?`, userID, escapeLike(prefix)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SearchWordStat, 0, limit)
	for rows.Next() {
		stat, scanErr := scanSearchWordStat(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

func (s *Store) ListHotSearchWords(ctx context.Context, limit int) ([]model.SearchWordStat, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT search_word, 0 AS user_id, COUNT(*) AS word_count
FROM search_words
GROUP BY search_word
ORDER BY word_count DESC, MAX(created_at) DESC, search_word ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SearchWordStat, 0, limit)
	for rows.Next() {
		stat, scanErr := scanSearchWordStat(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

func (s *Store) BuildEmotionAnalysis(ctx context.Context, projectID int64) (model.EmotionAnalysis, error) {
	items, err := s.listItemsForAdvancedFilter(ctx, model.ArticleFilter{ProjectID: projectID, Page: 1, PageSize: 500, Limit: 500})
	if err != nil {
		return model.EmotionAnalysis{}, err
	}
	counts := map[string]int{"positive": 0, "neutral": 0, "negative": 0}
	for _, item := range items {
		counts[classifyEmotion(item.Title+" "+item.Summary+" "+item.Content)]++
	}
	total := len(items)
	buckets := make([]model.EmotionBucket, 0, 3)
	for _, name := range []string{"positive", "neutral", "negative"} {
		ratio := 0.0
		if total > 0 {
			ratio = float64(counts[name]) / float64(total)
		}
		buckets = append(buckets, model.EmotionBucket{Name: name, Count: counts[name], Ratio: ratio})
	}
	return model.EmotionAnalysis{ProjectID: projectID, Total: total, Buckets: buckets}, nil
}

func (s *Store) BuildEventOverview(ctx context.Context, projectID int64) ([]model.EventOverview, error) {
	items, err := s.listItemsForAdvancedFilter(ctx, model.ArticleFilter{ProjectID: projectID, Page: 1, PageSize: 200, Limit: 200})
	if err != nil {
		return nil, err
	}
	projects, _ := s.ListProjects(ctx)
	projectNames := map[int64]string{}
	for _, project := range projects {
		projectNames[project.ID] = project.Name
	}
	keywordTitles := make(map[string][]string)
	keywordCount := make(map[string]int)
	for _, item := range items {
		keywords := extractKeywords(item.Title + " " + item.Summary)
		if len(keywords) == 0 {
			continue
		}
		keyword := keywords[0]
		keywordCount[keyword]++
		if len(keywordTitles[keyword]) < 3 {
			keywordTitles[keyword] = append(keywordTitles[keyword], item.Title)
		}
	}
	result := make([]model.EventOverview, 0, len(keywordCount))
	for keyword, count := range keywordCount {
		result = append(result, model.EventOverview{
			ProjectID:    projectID,
			ProjectName:  projectNames[projectID],
			Keyword:      keyword,
			Count:        count,
			LatestTitles: keywordTitles[keyword],
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Keyword < result[j].Keyword
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > 8 {
		result = result[:8]
	}
	return result, nil
}

func (s *Store) BuildPropagationAnalysis(ctx context.Context, projectID int64) (model.PropagationAnalysis, error) {
	items, err := s.listItemsForAdvancedFilter(ctx, model.ArticleFilter{ProjectID: projectID, Page: 1, PageSize: 300, Limit: 300})
	if err != nil {
		return model.PropagationAnalysis{}, err
	}
	sourceCounts := map[string]int{}
	trendCounts := map[string]int{}
	for _, item := range items {
		sourceCounts[item.SourceType]++
		day := item.CapturedAt.UTC().Format("2006-01-02")
		trendCounts[day]++
	}
	flow := mapToPropagationNodes(sourceCounts)
	trend := make([]model.TrendPoint, 0, len(trendCounts))
	for label, count := range trendCounts {
		trend = append(trend, model.TrendPoint{Label: label, Count: count})
	}
	sort.Slice(trend, func(i, j int) bool { return trend[i].Label < trend[j].Label })
	return model.PropagationAnalysis{ProjectID: projectID, SourceFlow: flow, Trend: trend}, nil
}

func (s *Store) BuildThemeInsights(ctx context.Context, projectID int64) ([]model.ThemeInsight, error) {
	items, err := s.listItemsForAdvancedFilter(ctx, model.ArticleFilter{ProjectID: projectID, Page: 1, PageSize: 200, Limit: 200})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	samples := map[string][]string{}
	for _, item := range items {
		for _, keyword := range extractKeywords(item.Title + " " + item.Summary) {
			counts[keyword]++
			if len(samples[keyword]) < 2 {
				samples[keyword] = append(samples[keyword], item.Title)
			}
		}
	}
	result := make([]model.ThemeInsight, 0, len(counts))
	for keyword, count := range counts {
		result = append(result, model.ThemeInsight{Name: keyword, Count: count, Samples: samples[keyword]})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Name < result[j].Name
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result, nil
}

func (s *Store) BuildPublicOpinionEvents(ctx context.Context, projectID int64) ([]model.PublicOpinionEvent, error) {
	events, err := s.BuildEventOverview(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result := make([]model.PublicOpinionEvent, 0, len(events))
	for _, event := range events {
		summary := ""
		if len(event.LatestTitles) > 0 {
			summary = summarizeText(event.LatestTitles[0])
		}
		result = append(result, model.PublicOpinionEvent{
			Title:       nonEmpty(event.ProjectName, "全局") + " - " + event.Keyword,
			ProjectID:   event.ProjectID,
			ProjectName: event.ProjectName,
			Keyword:     event.Keyword,
			Count:       event.Count,
			Summary:     summary,
		})
	}
	return result, nil
}

func (s *Store) BuildPublicOpinionReports(ctx context.Context, projectID int64) ([]model.PublicOpinionReport, error) {
	themes, err := s.BuildThemeInsights(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result := make([]model.PublicOpinionReport, 0, len(themes))
	for _, theme := range themes {
		result = append(result, model.PublicOpinionReport{
			Title:    "专题报告 - " + theme.Name,
			Summary:  summarizeText(strings.Join(theme.Samples, "；")),
			Keywords: append([]string{theme.Name}, theme.Samples...),
		})
		if len(result) >= 5 {
			break
		}
	}
	return result, nil
}

func (s *Store) listItemsForAdvancedFilter(ctx context.Context, filter model.ArticleFilter) ([]model.Item, error) {
	baseFilter := filter
	baseFilter.Page = 1
	if baseFilter.PageSize <= 0 {
		baseFilter.PageSize = max(filter.Limit, 500)
	}
	if baseFilter.PageSize <= 0 {
		baseFilter.PageSize = 500
	}
	var items []model.Item
	var err error
	if strings.TrimSpace(filter.Keyword) != "" {
		result, searchErr := s.SearchItemsFTS(ctx, baseFilter)
		err = searchErr
		items = result.Items
	} else {
		result, listErr := s.ListItems(ctx, baseFilter)
		err = listErr
		items = result.Items
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		meta := extractSearchMetadata(item)
		if filter.Industry != "" && !containsCI(meta["industry"], filter.Industry) {
			continue
		}
		if filter.Province != "" && !containsCI(meta["province"], filter.Province) {
			continue
		}
		if filter.City != "" && !containsCI(meta["city"], filter.City) {
			continue
		}
		if filter.Read == "read" && !item.Read {
			continue
		}
		if filter.Read == "unread" && item.Read {
			continue
		}
		if filter.Favorite == "favorited" && !item.Favorited {
			continue
		}
		if filter.Favorite == "unfavorited" && item.Favorited {
			continue
		}
		filtered = append(filtered, item)
	}
	switch strings.TrimSpace(filter.Sort) {
	case "captured_at_asc":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].CapturedAt.Before(filtered[j].CapturedAt) })
	case "title_asc":
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].Title < filtered[j].Title })
	default:
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].CapturedAt.After(filtered[j].CapturedAt) })
	}
	return filtered, nil
}

func scanUserPreference(scanner scanner) (model.UserPreference, error) {
	var pref model.UserPreference
	var emailNotifications int
	var updatedAt string
	if err := scanner.Scan(&pref.UserID, &pref.Language, &pref.Theme, &pref.DefaultSearchMode, &pref.ArticlePageSize, &emailNotifications, &updatedAt); err != nil {
		return model.UserPreference{}, err
	}
	pref.EmailNotifications = emailNotifications == 1
	pref.UpdatedAt = mustParseRFC3339(updatedAt)
	return pref, nil
}

func scanPopupState(scanner scanner) (model.PopupState, error) {
	var state model.PopupState
	var dismissed int
	var count int
	var dismissedAt sql.NullString
	var updatedAt string
	if err := scanner.Scan(&state.UserID, &state.Key, &dismissed, &count, &dismissedAt, &updatedAt); err != nil {
		return model.PopupState{}, err
	}
	state.Dismissed = dismissed == 1
	state.Count = count
	if dismissedAt.Valid {
		value := mustParseRFC3339(dismissedAt.String)
		state.DismissedAt = &value
	}
	state.UpdatedAt = mustParseRFC3339(updatedAt)
	return state, nil
}

func scanSearchWordStat(scanner scanner) (model.SearchWordStat, error) {
	var stat model.SearchWordStat
	if err := scanner.Scan(&stat.SearchWord, &stat.UserID, &stat.WordCount); err != nil {
		return model.SearchWordStat{}, err
	}
	return stat, nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func collectMetadataValues(items []model.Item, key string) []string {
	values := make([]string, 0)
	for _, item := range items {
		values = append(values, extractSearchMetadata(item)[key]...)
	}
	return values
}

func extractSearchMetadata(item model.Item) map[string][]string {
	meta := map[string][]string{
		"industry": {},
		"province": {},
		"city":     {},
	}
	if strings.TrimSpace(item.RawPayload) == "" {
		return meta
	}
	var payload any
	if err := json.Unmarshal([]byte(item.RawPayload), &payload); err != nil {
		return meta
	}
	walkMetadata(payload, meta)
	return meta
}

func walkMetadata(value any, meta map[string][]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			switch normalized {
			case "industry", "industries", "province", "provinces", "city", "cities":
				metaKey := normalized
				switch normalized {
				case "industries":
					metaKey = "industry"
				case "provinces":
					metaKey = "province"
				case "cities":
					metaKey = "city"
				default:
					metaKey = strings.TrimSuffix(normalized, "s")
				}
				switch value := child.(type) {
				case string:
					meta[metaKey] = append(meta[metaKey], strings.TrimSpace(value))
				case []any:
					for _, item := range value {
						if text, ok := item.(string); ok {
							meta[metaKey] = append(meta[metaKey], strings.TrimSpace(text))
						}
					}
				}
			}
			walkMetadata(child, meta)
		}
	case []any:
		for _, child := range typed {
			walkMetadata(child, meta)
		}
	}
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func bucketizeStrings(values []string) []model.SearchFacetBucket {
	counter := map[string]int{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		counter[value]++
	}
	return mapToBuckets(counter)
}

func mapToBuckets(counter map[string]int) []model.SearchFacetBucket {
	result := make([]model.SearchFacetBucket, 0, len(counter))
	for value, count := range counter {
		result = append(result, model.SearchFacetBucket{Value: value, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Value < result[j].Value
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func mapToPropagationNodes(counter map[string]int) []model.PropagationNode {
	result := make([]model.PropagationNode, 0, len(counter))
	for label, count := range counter {
		result = append(result, model.PropagationNode{Label: label, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Label < result[j].Label
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func containsCI(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func classifyEmotion(text string) string {
	normalized := strings.ToLower(text)
	positiveWords := []string{"上涨", "利好", "增长", "突破", "新高", "improve", "beat", "surge", "gain"}
	negativeWords := []string{"下跌", "利空", "风险", "暴跌", "回落", "loss", "drop", "fall", "miss"}
	positive := 0
	negative := 0
	for _, word := range positiveWords {
		if strings.Contains(normalized, strings.ToLower(word)) {
			positive++
		}
	}
	for _, word := range negativeWords {
		if strings.Contains(normalized, strings.ToLower(word)) {
			negative++
		}
	}
	switch {
	case positive > negative:
		return "positive"
	case negative > positive:
		return "negative"
	default:
		return "neutral"
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func summarizeText(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= 120 {
		return text
	}
	return string([]rune(text)[:120]) + "..."
}
