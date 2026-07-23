package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/pcdogyu/yuqing/go/internal/model"
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

func (s *Store) GetReleaseSettings(ctx context.Context) (model.ReleaseSettings, error) {
	row := s.db.QueryRowContext(ctx, `SELECT release_addr, release_url, release_dir, dev_release_dir, server_share_path, server_user, updated_at FROM release_settings WHERE id = 1`)
	var settings model.ReleaseSettings
	var updatedAt string
	if err := row.Scan(&settings.ReleaseAddr, &settings.ReleaseURL, &settings.ReleaseDir, &settings.DevReleaseDir, &settings.ServerSharePath, &settings.ServerUser, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return defaultReleaseSettings(), nil
		}
		return model.ReleaseSettings{}, err
	}
	settings.UpdatedAt = mustParseRFC3339(updatedAt)
	return normalizeReleaseSettings(settings), nil
}

func (s *Store) UpsertReleaseSettings(ctx context.Context, settings model.ReleaseSettings) (model.ReleaseSettings, error) {
	settings = normalizeReleaseSettings(settings)
	settings.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO release_settings (id, release_addr, release_url, release_dir, dev_release_dir, server_share_path, server_user, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	release_addr = excluded.release_addr,
	release_url = excluded.release_url,
	release_dir = excluded.release_dir,
	dev_release_dir = excluded.dev_release_dir,
	server_share_path = excluded.server_share_path,
	server_user = excluded.server_user,
	updated_at = excluded.updated_at`,
		settings.ReleaseAddr,
		settings.ReleaseURL,
		settings.ReleaseDir,
		settings.DevReleaseDir,
		settings.ServerSharePath,
		settings.ServerUser,
		settings.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.ReleaseSettings{}, err
	}
	return s.GetReleaseSettings(ctx)
}

func defaultReleaseSettings() model.ReleaseSettings {
	return model.ReleaseSettings{
		ReleaseAddr:     ":8099",
		ReleaseURL:      "http://10.15.0.7:8099",
		ReleaseDir:      `C:\yuqing\release`,
		DevReleaseDir:   `D:\yuqing\release`,
		ServerSharePath: `\\10.15.0.7\yuqing-release`,
		ServerUser:      `10.15.0.7\hyuser`,
	}
}

func normalizeReleaseSettings(settings model.ReleaseSettings) model.ReleaseSettings {
	defaults := defaultReleaseSettings()
	settings.ReleaseAddr = strings.TrimSpace(settings.ReleaseAddr)
	if settings.ReleaseAddr == "" {
		settings.ReleaseAddr = defaults.ReleaseAddr
	}
	settings.ReleaseURL = strings.TrimRight(strings.TrimSpace(settings.ReleaseURL), "/")
	if settings.ReleaseURL == "" {
		settings.ReleaseURL = defaults.ReleaseURL
	}
	settings.ReleaseDir = strings.TrimSpace(settings.ReleaseDir)
	if settings.ReleaseDir == "" {
		settings.ReleaseDir = defaults.ReleaseDir
	}
	settings.DevReleaseDir = strings.TrimSpace(settings.DevReleaseDir)
	if settings.DevReleaseDir == "" {
		settings.DevReleaseDir = defaults.DevReleaseDir
	}
	settings.ServerSharePath = strings.TrimRight(strings.TrimSpace(settings.ServerSharePath), `\`)
	if settings.ServerSharePath == "" {
		settings.ServerSharePath = defaults.ServerSharePath
	}
	settings.ServerUser = strings.TrimSpace(settings.ServerUser)
	if settings.ServerUser == "" {
		settings.ServerUser = defaults.ServerUser
	}
	return settings
}

func (s *Store) GetAStockRecommendationAlgorithmSettings(ctx context.Context) (model.AStockRecommendationAlgorithmSettings, error) {
	row := s.db.QueryRowContext(ctx, `SELECT settings_json, updated_at FROM a_stock_recommendation_algorithm_settings WHERE id = 1`)
	settings := model.DefaultAStockRecommendationAlgorithmSettings()
	var raw string
	var updatedAt string
	if err := row.Scan(&raw, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return model.AStockRecommendationAlgorithmSettings{}, err
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.AStockRecommendationAlgorithmSettings{}, err
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	settings.UpdatedAt = mustParseRFC3339(updatedAt)
	return settings, nil
}

func (s *Store) UpsertAStockRecommendationAlgorithmSettings(ctx context.Context, settings model.AStockRecommendationAlgorithmSettings) (model.AStockRecommendationAlgorithmSettings, error) {
	if settings.Version <= 0 {
		settings.Version = model.DefaultAStockRecommendationAlgorithmSettings().Version
	}
	if err := model.ValidateAStockRecommendationAlgorithmSettings(settings); err != nil {
		return model.AStockRecommendationAlgorithmSettings{}, err
	}
	settings.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(settings)
	if err != nil {
		return model.AStockRecommendationAlgorithmSettings{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO a_stock_recommendation_algorithm_settings (id, settings_json, updated_at)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	settings_json = excluded.settings_json,
	updated_at = excluded.updated_at`,
		string(raw),
		settings.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return model.AStockRecommendationAlgorithmSettings{}, err
	}
	return s.GetAStockRecommendationAlgorithmSettings(ctx)
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

const (
	articleCleanupDefaultRetentionDays = 90
	articleCleanupScopeAll             = "all"
	articleCleanupModeSoft             = "soft"
	articleCleanupModeHard             = "hard"
	articleCleanupTombstoneReason      = "article_cleanup"
)

func (s *Store) PreviewArticleCleanup(ctx context.Context, filter model.ArticleCleanupFilter) (model.ArticleCleanupResult, error) {
	filter, cutoff, err := normalizeArticleCleanupFilter(filter)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	result, err := s.articleCleanupPreview(ctx, filter, cutoff)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	before, err := s.visibleArticleCount(ctx)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	result.BeforeTotal = before
	result.AfterTotal = before
	return result, nil
}

func (s *Store) CleanupArticles(ctx context.Context, filter model.ArticleCleanupFilter) (result model.ArticleCleanupResult, err error) {
	filter, cutoff, err := normalizeArticleCleanupFilter(filter)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	before, err := s.visibleArticleCount(ctx)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	result, err = s.articleCleanupPreview(ctx, filter, cutoff)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	result.BeforeTotal = before
	result.Applied = true

	switch filter.Mode {
	case articleCleanupModeSoft:
		result.Affected, err = s.softCleanupArticles(ctx, cutoff)
	case articleCleanupModeHard:
		result.Affected, result.Tombstoned, err = s.hardCleanupArticles(ctx, cutoff)
	default:
		err = fmt.Errorf("unsupported cleanup mode %q", filter.Mode)
	}
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	after, err := s.visibleArticleCount(ctx)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	result.AfterTotal = after
	return result, nil
}

func normalizeArticleCleanupFilter(filter model.ArticleCleanupFilter) (model.ArticleCleanupFilter, time.Time, error) {
	filter.Mode = strings.ToLower(strings.TrimSpace(filter.Mode))
	if filter.Mode == "" {
		filter.Mode = articleCleanupModeSoft
	}
	if filter.Mode != articleCleanupModeSoft && filter.Mode != articleCleanupModeHard {
		return filter, time.Time{}, fmt.Errorf("mode must be soft or hard")
	}
	filter.Scope = strings.ToLower(strings.TrimSpace(filter.Scope))
	if filter.Scope == "" {
		filter.Scope = articleCleanupScopeAll
	}
	if filter.Scope != articleCleanupScopeAll {
		return filter, time.Time{}, fmt.Errorf("scope must be all")
	}
	if filter.RetentionDays <= 0 {
		filter.RetentionDays = articleCleanupDefaultRetentionDays
	}
	if filter.RetentionDays > 3650 {
		return filter, time.Time{}, fmt.Errorf("retention_days must be <= 3650")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -filter.RetentionDays)
	if raw := strings.TrimSpace(filter.Cutoff); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			parsedDate, dateErr := time.Parse("2006-01-02", raw)
			if dateErr != nil {
				return filter, time.Time{}, fmt.Errorf("invalid cutoff")
			}
			parsed = parsedDate
		}
		cutoff = parsed.UTC()
	}
	filter.Cutoff = cutoff.Format(time.RFC3339)
	return filter, cutoff, nil
}

func (s *Store) articleCleanupPreview(ctx context.Context, filter model.ArticleCleanupFilter, cutoff time.Time) (model.ArticleCleanupResult, error) {
	where := articleCleanupCandidateWhere(filter.Mode)
	args := []any{cutoff.Format(time.RFC3339)}
	result := model.ArticleCleanupResult{
		Mode:          filter.Mode,
		RetentionDays: filter.RetentionDays,
		Scope:         filter.Scope,
		Cutoff:        cutoff.Format(time.RFC3339),
		Sources:       []model.ArticleCleanupSourceCount{},
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE `+where, args...).Scan(&result.Total); err != nil {
		return model.ArticleCleanupResult{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT source_type, COUNT(1)
FROM items
WHERE `+where+`
GROUP BY source_type
ORDER BY COUNT(1) DESC, source_type ASC`, args...)
	if err != nil {
		return model.ArticleCleanupResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item model.ArticleCleanupSourceCount
		if err := rows.Scan(&item.SourceType, &item.Count); err != nil {
			return model.ArticleCleanupResult{}, err
		}
		result.Sources = append(result.Sources, item)
	}
	if err := rows.Err(); err != nil {
		return model.ArticleCleanupResult{}, err
	}
	return result, nil
}

func (s *Store) softCleanupArticles(ctx context.Context, cutoff time.Time) (int, error) {
	insert := `
INSERT OR IGNORE INTO item_tags (item_id, tag, created_at)
SELECT id, 'deleted', ? FROM items WHERE ` + articleCleanupCandidateWhere(articleCleanupModeSoft)
	if s.Driver() == "postgres" {
		insert = `
INSERT INTO item_tags (item_id, tag, created_at)
SELECT id, 'deleted', ? FROM items WHERE ` + articleCleanupCandidateWhere(articleCleanupModeSoft) + `
ON CONFLICT DO NOTHING`
	}
	res, err := s.db.ExecContext(ctx, insert, time.Now().UTC().Format(time.RFC3339), cutoff.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (s *Store) hardCleanupArticles(ctx context.Context, cutoff time.Time) (affected int, tombstoned int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = s.createArticleCleanupCandidateTable(ctx, tx); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM article_cleanup_candidates`); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO article_cleanup_candidates (id, source_key) SELECT id, source_key FROM items WHERE `+articleCleanupCandidateWhere(articleCleanupModeHard),
		cutoff.Format(time.RFC3339),
	); err != nil {
		return 0, 0, err
	}
	var candidateCount int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM article_cleanup_candidates`).Scan(&candidateCount); err != nil {
		return 0, 0, err
	}
	if candidateCount == 0 {
		err = tx.Commit()
		return 0, 0, err
	}
	tombstoneQuery := `
INSERT OR IGNORE INTO item_deletion_tombstones (source_key, deleted_at, reason)
SELECT source_key, ?, ? FROM article_cleanup_candidates WHERE source_key <> ''`
	if s.Driver() == "postgres" {
		tombstoneQuery = `
INSERT INTO item_deletion_tombstones (source_key, deleted_at, reason)
SELECT source_key, ?, ? FROM article_cleanup_candidates WHERE source_key <> ''
ON CONFLICT DO NOTHING`
	}
	res, err := tx.ExecContext(ctx, tombstoneQuery, time.Now().UTC().Format(time.RFC3339), articleCleanupTombstoneReason)
	if err != nil {
		return 0, 0, err
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr == nil {
		tombstoned = int(rows)
	}
	for _, table := range []string{"item_relations", "item_tags", "favorites", "item_reads", "item_shares"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE item_id IN (SELECT id FROM article_cleanup_candidates)`); err != nil {
			return 0, 0, err
		}
	}
	res, err = tx.ExecContext(ctx, `DELETE FROM items WHERE id IN (SELECT id FROM article_cleanup_candidates)`)
	if err != nil {
		return 0, 0, err
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr == nil {
		affected = int(rows)
	}
	err = tx.Commit()
	return affected, tombstoned, err
}

func (s *Store) createArticleCleanupCandidateTable(ctx context.Context, tx *Tx) error {
	if s.Driver() == "postgres" {
		_, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS article_cleanup_candidates (id BIGINT PRIMARY KEY, source_key TEXT NOT NULL) ON COMMIT DELETE ROWS`)
		return err
	}
	_, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS article_cleanup_candidates (id INTEGER PRIMARY KEY, source_key TEXT NOT NULL)`)
	return err
}

func articleCleanupCandidateWhere(mode string) string {
	base := `items.captured_at < ?`
	switch mode {
	case articleCleanupModeHard:
		return base + ` AND EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')`
	default:
		return base + ` AND NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')`
	}
}

func (s *Store) visibleArticleCount(ctx context.Context) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.item_id = items.id AND item_tags.tag = 'deleted')`).Scan(&total)
	return total, err
}

func (s *Store) filterItemDeletionTombstones(ctx context.Context, items []model.Item) ([]model.Item, error) {
	if len(items) == 0 {
		return items, nil
	}
	keys := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.SourceKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return items, nil
	}
	tombstoned := make(map[string]struct{}, len(keys))
	const chunkSize = 500
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[start:end]
		args := make([]any, 0, len(chunk))
		for _, key := range chunk {
			args = append(args, key)
		}
		rows, err := s.db.QueryContext(ctx, `SELECT source_key FROM item_deletion_tombstones WHERE source_key IN (`+placeholders(len(chunk))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				_ = rows.Close()
				return nil, err
			}
			tombstoned[key] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if len(tombstoned) == 0 {
		return items, nil
	}
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		if _, ok := tombstoned[strings.TrimSpace(item.SourceKey)]; ok {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}
func (s *Store) SetItemLegacyStatus(ctx context.Context, itemID int64, status string) error {
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM item_tags WHERE item_id = ? AND tag LIKE 'legacy_status:%'`, itemID); err != nil {
		return err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" || status == "active" || status == "valid" || status == "normal" {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_tags (item_id, tag, created_at) VALUES (?, ?, ?)`, itemID, "legacy_status:"+status, now); err != nil {
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
