package portal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

const legacyAPIPageSize = 25

type legacyExportRequest struct {
	legacySearchRequest
	ExportType any `json:"exportType"`
	ExportList any `json:"exportlist"`
}

func (s *Server) handleLegacyAPIToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"code": 500, "msg": "invalid body"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || req.Password == "" {
		writeRawJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "用户名或密码不能为空"})
		return
	}

	loginResp, err := s.authLogin(req.Username, req.Password)
	if err != nil || strings.TrimSpace(loginResp.SessionToken) == "" {
		writeRawJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "登录失败"})
		return
	}

	var tokenResp struct {
		Data model.APIToken `json:"data"`
	}
	httpResp, createErr := s.client.R().
		SetQueryParam("session_token", loginResp.SessionToken).
		SetBody(map[string]string{"name": "legacy-api"}).
		SetResult(&tokenResp).
		Post(s.cfg.AuthURL + "/api/v1/auth/tokens")
	if createErr != nil || !httpResp.IsSuccess() || strings.TrimSpace(tokenResp.Data.Token) == "" {
		writeRawJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "登录失败"})
		return
	}

	s.recordAuditLog(map[string]any{
		"username": req.Username,
		"id":       tokenResp.Data.UserID,
	}, "legacy_api.getToken", "/api/getToken", map[string]any{
		"username": req.Username,
	})

	writeRawJSON(w, http.StatusOK, map[string]any{
		"code": 200,
		"msg":  "success",
		"data": tokenResp.Data.Token,
	})
}

func (s *Server) handleLegacyAPIArticleList(w http.ResponseWriter, r *http.Request) {
	s.handleLegacyAPIArticleListCompat(w, r, false)
}

func (s *Server) handleLegacyAPIMergeArticleList(w http.ResponseWriter, r *http.Request) {
	s.handleLegacyAPIArticleListCompat(w, r, true)
}

func (s *Server) handleLegacyAPIArticleListCompat(w http.ResponseWriter, r *http.Request, merged bool) {
	if r.Method != http.MethodPost {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	user, authLabel, err := s.legacyAPIUserFromRequest(r)
	if err != nil {
		writeRawJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "unauthorized"})
		return
	}

	var req legacySearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"code": 500, "msg": "invalid body"})
		return
	}
	filter := legacyAPIFilterFromRequest(req, userIDFromMap(user))
	result, err := s.fetchLegacyAPIItems(filter)
	if err != nil {
		writeRawJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "msg": err.Error()})
		return
	}
	entries := make([]legacySearchArticle, 0, len(result.Items))
	projectID := filter.ProjectID
	keyword := strings.TrimSpace(filter.Keyword)
	for _, item := range result.Items {
		entries = append(entries, legacySearchArticleFromItem(item, keyword, legacyAPIArticleDetailURL(item.ID, projectID)))
	}
	totalCount := result.Total
	if merged {
		entries = mergeLegacyAPIArticles(entries)
		totalCount = len(entries)
	}
	pageSize := maxInt(result.PageSize, filter.PageSize, legacyAPIPageSize)
	totalPage := 1
	if totalCount > 0 {
		totalPage = (totalCount + pageSize - 1) / pageSize
	}

	s.recordAuditLog(user, chooseLegacyAPIAction(merged), "/api/getArticle", map[string]any{
		"auth":        authLabel,
		"keyword":     keyword,
		"page":        filter.Page,
		"page_size":   pageSize,
		"project_id":  projectID,
		"merged":      merged,
		"result_size": len(entries),
		"total_count": totalCount,
	})

	writeRawJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg":  "success",
		"data": map[string]any{
			"data":        entries,
			"totalPage":   totalPage,
			"totalCount":  totalCount,
			"currentPage": maxInt(result.Page, filter.Page, 1),
		},
	})
}

func (s *Server) handleLegacyAPIArticleDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	user, authLabel, err := s.legacyAPIUserFromRequest(r)
	if err != nil {
		writeRawJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "unauthorized"})
		return
	}
	articleID := strings.TrimSpace(r.URL.Query().Get("articleId"))
	if articleID == "" {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"code": 500, "msg": "articleId required"})
		return
	}
	item, err := s.fetchLegacyItemByID(articleID, userIDFromMap(user))
	if err != nil {
		writeRawJSON(w, http.StatusNotFound, map[string]any{"code": 500, "msg": "not found"})
		return
	}
	payload := legacySearchArticleDetailPayload(item)
	s.recordAuditLog(user, "legacy_api.detail", "/api/detail", map[string]any{
		"auth":       authLabel,
		"article_id": articleID,
		"project_id": strings.TrimSpace(r.URL.Query().Get("projectId")),
	})
	writeRawJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg":  "success",
		"data": payload,
	})
}

func (s *Server) handleLegacyMonitorExport(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method != http.MethodPost {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	request, rawDetail, err := decodeLegacyExportRequest(r)
	if err != nil {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"code": 500, "msg": "invalid export request"})
		return
	}

	items, err := s.collectLegacyExportItems(request, userIDFromMap(user))
	if err != nil {
		writeRawJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "msg": err.Error()})
		return
	}

	filename := "monitor_export_" + time.Now().Format("20060102_150405") + ".xls"
	workbook := buildLegacyExcelWorkbook(items)
	w.Header().Set("Content-Type", "application/vnd.ms-excel; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(workbook))

	s.recordAuditLog(user, "monitor.exportarticle", "/monitor/exportarticle", map[string]any{
		"count":       len(items),
		"export_type": legacyExportTypeString(request.ExportType),
		"request":     rawDetail,
	})
}

func decodeLegacyExportRequest(r *http.Request) (legacyExportRequest, map[string]any, error) {
	var request legacyExportRequest
	raw := map[string]any{}
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	switch {
	case strings.Contains(contentType, "application/json"):
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			return legacyExportRequest{}, nil, err
		}
	case strings.Contains(contentType, "application/x-www-form-urlencoded"), strings.Contains(contentType, "multipart/form-data"), contentType == "":
		if err := r.ParseForm(); err != nil {
			return legacyExportRequest{}, nil, err
		}
		payload := strings.TrimSpace(r.FormValue("data"))
		if payload != "" {
			if err := json.Unmarshal([]byte(payload), &raw); err != nil {
				return legacyExportRequest{}, nil, err
			}
		} else {
			raw = map[string]any{}
			for key, values := range r.Form {
				if len(values) == 1 {
					raw[key] = values[0]
					continue
				}
				copied := make([]string, 0, len(values))
				copied = append(copied, values...)
				raw[key] = copied
			}
		}
	default:
		return legacyExportRequest{}, nil, errors.New("unsupported content type")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return legacyExportRequest{}, nil, err
	}
	if err := json.NewDecoder(bytes.NewReader(encoded)).Decode(&request); err != nil {
		return legacyExportRequest{}, nil, err
	}
	return request, raw, nil
}

func (s *Server) collectLegacyExportItems(request legacyExportRequest, userID int64) ([]model.Item, error) {
	ids := legacyExportItemIDs(request.ExportList)
	if len(ids) > 0 && legacyExportTypeString(request.ExportType) != "1" {
		items := make([]model.Item, 0, len(ids))
		for _, id := range ids {
			item, err := s.fetchLegacyItemByID(strconv.FormatInt(id, 10), userID)
			if err != nil {
				continue
			}
			items = append(items, item)
		}
		return items, nil
	}

	filter := legacyAPIFilterFromRequest(request.legacySearchRequest, userID)
	filter.Page = 1
	filter.PageSize = 200
	collected := make([]model.Item, 0, 200)
	for {
		result, err := s.fetchLegacyAPIItems(filter)
		if err != nil {
			return nil, err
		}
		collected = append(collected, result.Items...)
		if len(collected) >= result.Total || len(result.Items) == 0 {
			break
		}
		filter.Page++
	}
	return collected, nil
}

func buildLegacyExcelWorkbook(items []model.Item) string {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	body.WriteString(`<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel" xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">`)
	body.WriteString(`<Worksheet ss:Name="舆情导出"><Table>`)
	body.WriteString(`<Row>`)
	for _, header := range []string{"标题", "来源", "发布时间", "情感", "行业", "事件", "标签", "摘要", "链接"} {
		body.WriteString(legacyExcelCell(header))
	}
	body.WriteString(`</Row>`)
	for _, item := range items {
		payload := legacyPayloadMap(item)
		body.WriteString(`<Row>`)
		body.WriteString(legacyExcelCell(item.Title))
		body.WriteString(legacyExcelCell(nonEmpty(legacyPayloadString(payload, "sourcewebsitename"), legacyPayloadString(payload, "source_name"), item.FromText, item.SourceType)))
		body.WriteString(legacyExcelCell(nonEmpty(item.PublishTimeText, item.PublishTime, item.CapturedAt.Format("2006-01-02 15:04:05"))))
		body.WriteString(legacyExcelCell(legacySearchEmotionText(item)))
		body.WriteString(legacyExcelCell(nonEmpty(legacyPayloadString(payload, "industrylable"), legacyPayloadString(payload, "industry"))))
		body.WriteString(legacyExcelCell(nonEmpty(legacyPayloadString(payload, "eventlable"), strings.TrimSpace(item.TagFlags))))
		body.WriteString(legacyExcelCell(strings.TrimSpace(item.TagFlags)))
		body.WriteString(legacyExcelCell(nonEmpty(item.Summary, item.Content)))
		body.WriteString(legacyExcelCell(nonEmpty(item.SourceURL, item.DetailURL)))
		body.WriteString(`</Row>`)
	}
	body.WriteString(`</Table></Worksheet></Workbook>`)
	return body.String()
}

func legacyExcelCell(value string) string {
	return `<Cell><Data ss:Type="String">` + html.EscapeString(strings.TrimSpace(value)) + `</Data></Cell>`
}

func legacyAPIFilterFromRequest(req legacySearchRequest, userID int64) model.ArticleFilter {
	page := maxInt(req.Page, req.PageNum, 1)
	pageSize := maxInt(req.PageSize, legacyAPIPageSize)
	projectID := parseProjectID(nonEmpty(req.ProjectID3, req.ProjectID2, req.ProjectID))
	start, end := legacyAPIDateRange(req.TimeType, req.Start, req.End)
	if req.TimeType == 8 {
		start = nonEmpty(req.Times, start)
		end = nonEmpty(req.Timee, end)
	}
	sort := ""
	switch req.SearchType {
	case 0:
		sort = "captured_at_asc"
	default:
		sort = "captured_at_desc"
	}
	return model.ArticleFilter{
		Page:       page,
		PageSize:   pageSize,
		Keyword:    nonEmpty(req.SearchKeyword, req.Searchword, req.SearchWord, req.Keyword),
		SourceType: legacyClassifySourceType(req.Classify, req.SourceType),
		ProjectID:  projectID,
		UserID:     userID,
		Start:      nonEmpty(req.Times, start),
		End:        nonEmpty(req.Timee, end),
		Industry:   firstString(req.IndustryIndex),
		Province:   firstString(req.Province),
		City:       firstString(req.City),
		Sort:       sort,
	}
}

func (s *Server) fetchLegacyAPIItems(filter model.ArticleFilter) (model.SearchResult, error) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(maxInt(filter.Page, 1)))
	query.Set("page_size", strconv.Itoa(maxInt(filter.PageSize, legacyAPIPageSize)))
	if strings.TrimSpace(filter.Keyword) != "" {
		query.Set("q", strings.TrimSpace(filter.Keyword))
	}
	if strings.TrimSpace(filter.SourceType) != "" {
		query.Set("source_type", strings.TrimSpace(filter.SourceType))
	}
	if filter.ProjectID > 0 {
		query.Set("project_id", strconv.FormatInt(filter.ProjectID, 10))
	}
	if filter.UserID > 0 {
		query.Set("user_id", strconv.FormatInt(filter.UserID, 10))
	}
	if strings.TrimSpace(filter.Start) != "" {
		query.Set("start", strings.TrimSpace(filter.Start))
	}
	if strings.TrimSpace(filter.End) != "" {
		query.Set("end", strings.TrimSpace(filter.End))
	}
	if strings.TrimSpace(filter.Industry) != "" {
		query.Set("industry", strings.TrimSpace(filter.Industry))
	}
	if strings.TrimSpace(filter.Province) != "" {
		query.Set("province", strings.TrimSpace(filter.Province))
	}
	if strings.TrimSpace(filter.City) != "" {
		query.Set("city", strings.TrimSpace(filter.City))
	}
	if strings.TrimSpace(filter.Sort) != "" {
		query.Set("sort", strings.TrimSpace(filter.Sort))
	}
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
		return model.SearchResult{}, err
	}
	return result, nil
}

func (s *Server) legacyAPIUserFromRequest(r *http.Request) (map[string]any, string, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		if token := strings.TrimSpace(firstNonEmpty(r.Header.Get("token"), r.URL.Query().Get("token"))); token != "" {
			authHeader = "Bearer " + token
		}
	}
	if authHeader != "" {
		var authResp struct {
			Data map[string]any `json:"data"`
		}
		resp, err := s.client.R().
			SetHeader("Authorization", authHeader).
			SetResult(&authResp).
			Get(s.cfg.AuthURL + "/api/v1/auth/me")
		if err == nil && resp.IsSuccess() && authResp.Data != nil {
			return authResp.Data, "bearer", nil
		}
	}
	if sessionToken, ok := s.sessionTokenFromRequest(r); ok {
		user, err := s.getSessionUser(sessionToken)
		if err == nil {
			return user, "session", nil
		}
	}
	return nil, "", errors.New("unauthorized")
}

func legacyAPIArticleDetailURL(itemID int64, projectID int64) string {
	values := url.Values{}
	values.Set("articleId", strconv.FormatInt(itemID, 10))
	if projectID > 0 {
		values.Set("projectId", strconv.FormatInt(projectID, 10))
	}
	return "/api/detail?" + values.Encode()
}

func mergeLegacyAPIArticles(entries []legacySearchArticle) []legacySearchArticle {
	type grouped struct {
		entry legacySearchArticle
		count int
	}
	orderedKeys := make([]string, 0, len(entries))
	groups := make(map[string]*grouped, len(entries))
	for _, entry := range entries {
		key := strings.ToLower(strings.TrimSpace(entry.Title + "|" + entry.SourceWebsiteName + "|" + entry.PublishTime))
		if group, ok := groups[key]; ok {
			group.count++
			group.entry.Num = group.count
			continue
		}
		copyEntry := entry
		copyEntry.Num = 1
		groups[key] = &grouped{entry: copyEntry, count: 1}
		orderedKeys = append(orderedKeys, key)
	}
	merged := make([]legacySearchArticle, 0, len(groups))
	for _, key := range orderedKeys {
		merged = append(merged, groups[key].entry)
	}
	return merged
}

func chooseLegacyAPIAction(merged bool) string {
	if merged {
		return "legacy_api.getMergeArticle"
	}
	return "legacy_api.getArticle"
}

func legacyAPIDateRange(timeType int, start, end string) (string, string) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start != "" || end != "" {
		return start, end
	}
	now := time.Now().In(time.Local)
	switch timeType {
	case 1:
		return now.Add(-24 * time.Hour).Format("2006-01-02"), now.Format("2006-01-02")
	case 2:
		today := now.Format("2006-01-02")
		return today, today
	case 3:
		day := now.Add(-24 * time.Hour).Format("2006-01-02")
		return day, day
	case 4:
		return now.AddDate(0, 0, -3).Format("2006-01-02"), now.Format("2006-01-02")
	case 5:
		return now.AddDate(0, 0, -7).Format("2006-01-02"), now.Format("2006-01-02")
	case 6:
		return now.AddDate(0, 0, -15).Format("2006-01-02"), now.Format("2006-01-02")
	case 7:
		return now.AddDate(0, -1, 0).Format("2006-01-02"), now.Format("2006-01-02")
	default:
		return "", ""
	}
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func legacyClassifySourceType(classify []string, direct string) string {
	if strings.TrimSpace(direct) != "" {
		return strings.TrimSpace(direct)
	}
	if len(classify) == 0 {
		return ""
	}
	switch strings.TrimSpace(classify[0]) {
	case "", "0":
		return ""
	case "1":
		return "wechat"
	case "2":
		return "weibo"
	case "3":
		return "gov"
	case "4":
		return "bbs"
	case "5":
		return "news"
	case "6":
		return "newspaper"
	case "7":
		return "app"
	case "8":
		return "web"
	case "9":
		return "foreign_media"
	case "10":
		return "video"
	case "11":
		return "blog"
	default:
		return ""
	}
}

func legacyExportItemIDs(value any) []int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if strings.HasPrefix(trimmed, "[") {
			var raw []any
			if err := json.Unmarshal([]byte(trimmed), &raw); err == nil {
				return legacyExportItemIDs(raw)
			}
		}
		parts := strings.Split(trimmed, ",")
		ids := make([]int64, 0, len(parts))
		for _, part := range parts {
			if id := parseProjectID(part); id > 0 {
				ids = append(ids, id)
			}
		}
		return ids
	case []string:
		ids := make([]int64, 0, len(typed))
		for _, item := range typed {
			if id := parseProjectID(item); id > 0 {
				ids = append(ids, id)
			}
		}
		return ids
	case []any:
		ids := make([]int64, 0, len(typed))
		for _, item := range typed {
			if id, ok := legacyAPIInt64(item); ok && id > 0 {
				ids = append(ids, id)
			}
		}
		return ids
	default:
		if id, ok := legacyAPIInt64(value); ok && id > 0 {
			return []int64{id}
		}
		return nil
	}
}

func legacyExportTypeString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.Itoa(int(typed))
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func (s *Server) recordAuditLog(user any, action, resource string, detail any) {
	userID := userIDFromMap(user)
	username := usernameFromAny(user)
	payload := "{}"
	if detail != nil {
		if raw, err := json.Marshal(detail); err == nil {
			payload = string(raw)
		}
	}
	_, _ = s.client.R().
		SetBody(model.AuditLog{
			UserID:     userID,
			Username:   username,
			Action:     action,
			Resource:   resource,
			DetailJSON: payload,
		}).
		Post(s.cfg.ContentURL + "/api/v1/system/audit-logs")
}

func usernameFromAny(user any) string {
	mapped, ok := user.(map[string]any)
	if !ok {
		return ""
	}
	return nonEmpty(legacyStringFromAny(mapped["username"]), legacyStringFromAny(mapped["display_name"]), legacyStringFromAny(mapped["name"]))
}

func legacyAPIInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
