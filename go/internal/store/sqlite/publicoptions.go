package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func (s *Store) ListPublicOptions(ctx context.Context, userID int64, keyword string) ([]model.PublicOption, error) {
	query := `
SELECT id, user_id, eventname, eventkeywords, eventstopwords, eventstarttime, eventendtime, createtime, status, updatetime, detail_status, emotional_index, back_analysis, event_context, event_trace, hot_analysis, netizens_analysis, statistics, propagation_analysis, thematic_analysis, unscramble_content, content_analysis
FROM public_options`
	args := make([]any, 0, 2)
	clauses := make([]string, 0, 2)
	if userID > 0 {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		clauses = append(clauses, "(eventname LIKE ? OR eventkeywords LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY updatetime DESC, id DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.PublicOption, 0)
	for rows.Next() {
		option, scanErr := scanPublicOption(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, option)
	}
	return result, rows.Err()
}

func (s *Store) GetPublicOption(ctx context.Context, id int64) (model.PublicOption, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, eventname, eventkeywords, eventstopwords, eventstarttime, eventendtime, createtime, status, updatetime, detail_status, emotional_index, back_analysis, event_context, event_trace, hot_analysis, netizens_analysis, statistics, propagation_analysis, thematic_analysis, unscramble_content, content_analysis
FROM public_options
WHERE id = ?`, id)
	option, err := scanPublicOption(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.PublicOption{}, ErrNotFound
		}
		return model.PublicOption{}, err
	}
	return option, nil
}

func (s *Store) CreatePublicOption(ctx context.Context, option model.PublicOption) (model.PublicOption, error) {
	now := time.Now().UTC()
	if option.UserID <= 0 {
		option.UserID = 0
	}
	if strings.TrimSpace(option.EventName) == "" {
		option.EventName = "未命名事件"
	}
	if option.Status <= 0 {
		option.Status = 3
	}
	if option.DetailStatus <= 0 {
		option.DetailStatus = option.Status
	}
	option.CreateTime = now
	option.Updatetime = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO public_options (
	user_id, eventname, eventkeywords, eventstopwords, eventstarttime, eventendtime,
	createtime, status, updatetime, detail_status, emotional_index,
	back_analysis, event_context, event_trace, hot_analysis, netizens_analysis,
	statistics, propagation_analysis, thematic_analysis, unscramble_content, content_analysis
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		option.UserID, option.EventName, option.EventKeywords, option.EventStopWords, option.EventStartTime, option.EventEndTime,
		option.CreateTime.Format(time.RFC3339), option.Status, option.Updatetime.Format(time.RFC3339), option.DetailStatus, option.EmotionalIndex,
		nonEmpty(option.BackAnalysis, "{}"), nonEmpty(option.EventContext, "[]"), nonEmpty(option.EventTrace, "{}"), nonEmpty(option.HotAnalysis, "[]"), nonEmpty(option.NetizensAnalysis, "{}"),
		nonEmpty(option.Statistics, "{}"), nonEmpty(option.PropagationAnalysis, "{}"), nonEmpty(option.ThematicAnalysis, "{}"), nonEmpty(option.UnscrambleContent, "{}"), option.ContentAnalysis,
	)
	if err != nil {
		return model.PublicOption{}, err
	}
	return s.lastPublicOption(ctx)
}

func (s *Store) UpdatePublicOption(ctx context.Context, option model.PublicOption) (model.PublicOption, error) {
	if option.ID <= 0 {
		return model.PublicOption{}, ErrNotFound
	}
	current, err := s.GetPublicOption(ctx, option.ID)
	if err != nil {
		return model.PublicOption{}, err
	}
	now := time.Now().UTC()
	if option.UserID <= 0 {
		option.UserID = current.UserID
	}
	if strings.TrimSpace(option.EventName) == "" {
		option.EventName = current.EventName
	}
	if option.Status <= 0 {
		option.Status = current.Status
	}
	if option.DetailStatus <= 0 {
		option.DetailStatus = current.DetailStatus
	}
	if option.CreateTime.IsZero() {
		option.CreateTime = current.CreateTime
	}
	option.Updatetime = now
	_, err = s.db.ExecContext(ctx, `
UPDATE public_options SET
	user_id = ?,
	eventname = ?,
	eventkeywords = ?,
	eventstopwords = ?,
	eventstarttime = ?,
	eventendtime = ?,
	status = ?,
	updatetime = ?,
	detail_status = ?,
	emotional_index = ?,
	back_analysis = ?,
	event_context = ?,
	event_trace = ?,
	hot_analysis = ?,
	netizens_analysis = ?,
	statistics = ?,
	propagation_analysis = ?,
	thematic_analysis = ?,
	unscramble_content = ?,
	content_analysis = ?
WHERE id = ?`,
		option.UserID, option.EventName, option.EventKeywords, option.EventStopWords, option.EventStartTime, option.EventEndTime,
		option.Status, option.Updatetime.Format(time.RFC3339), option.DetailStatus, option.EmotionalIndex,
		nonEmpty(option.BackAnalysis, current.BackAnalysis, "{}"),
		nonEmpty(option.EventContext, current.EventContext, "[]"),
		nonEmpty(option.EventTrace, current.EventTrace, "{}"),
		nonEmpty(option.HotAnalysis, current.HotAnalysis, "[]"),
		nonEmpty(option.NetizensAnalysis, current.NetizensAnalysis, "{}"),
		nonEmpty(option.Statistics, current.Statistics, "{}"),
		nonEmpty(option.PropagationAnalysis, current.PropagationAnalysis, "{}"),
		nonEmpty(option.ThematicAnalysis, current.ThematicAnalysis, "{}"),
		nonEmpty(option.UnscrambleContent, current.UnscrambleContent, "{}"),
		nonEmpty(option.ContentAnalysis, current.ContentAnalysis),
		option.ID,
	)
	if err != nil {
		return model.PublicOption{}, err
	}
	return s.GetPublicOption(ctx, option.ID)
}

func (s *Store) DeletePublicOption(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM public_options WHERE id = ?`, id)
	return err
}

func scanPublicOption(scanner scanner) (model.PublicOption, error) {
	var option model.PublicOption
	var createTime, updateTime string
	if err := scanner.Scan(
		&option.ID,
		&option.UserID,
		&option.EventName,
		&option.EventKeywords,
		&option.EventStopWords,
		&option.EventStartTime,
		&option.EventEndTime,
		&createTime,
		&option.Status,
		&updateTime,
		&option.DetailStatus,
		&option.EmotionalIndex,
		&option.BackAnalysis,
		&option.EventContext,
		&option.EventTrace,
		&option.HotAnalysis,
		&option.NetizensAnalysis,
		&option.Statistics,
		&option.PropagationAnalysis,
		&option.ThematicAnalysis,
		&option.UnscrambleContent,
		&option.ContentAnalysis,
	); err != nil {
		return model.PublicOption{}, err
	}
	option.CreateTime = mustParseRFC3339(createTime)
	option.Updatetime = mustParseRFC3339(updateTime)
	return option, nil
}

func (s *Store) lastPublicOption(ctx context.Context) (model.PublicOption, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, eventname, eventkeywords, eventstopwords, eventstarttime, eventendtime, createtime, status, updatetime, detail_status, emotional_index, back_analysis, event_context, event_trace, hot_analysis, netizens_analysis, statistics, propagation_analysis, thematic_analysis, unscramble_content, content_analysis
FROM public_options
ORDER BY id DESC
LIMIT 1`)
	option, err := scanPublicOption(row)
	if err != nil {
		return model.PublicOption{}, err
	}
	return option, nil
}

func publicOptionIDs(raw string) []int64 {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	ids := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func formatPlatformKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "default"
	}
	return kind
}
