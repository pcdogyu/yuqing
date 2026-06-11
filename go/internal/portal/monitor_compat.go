package portal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type legacyMonitorRequest struct {
	legacySearchRequest
	ArticleID      string `json:"articleId"`
	ArticleIDAlt   string `json:"articleid"`
	GroupID        string `json:"groupid"`
	GroupIDAlt     string `json:"group_id"`
	ProjectIDRaw   string `json:"projectid"`
	ProjectIDAlt   string `json:"projectId"`
	ProjectIDAlt2  string `json:"project_id"`
	SearchKeyword  string `json:"searchkeyword"`
	MonitorSearch  string `json:"monitorsearch"`
	Keywords       string `json:"keywords"`
	RelatedWord    string `json:"relatedword"`
	PublishTime    string `json:"publish_time"`
	MatchingMode   string `json:"matchingmode"`
	EmotionalIndex string `json:"emotionalIndex"`
	OpenFlag       string `json:"openFlag"`
	Flag           string `json:"flag"`
	Type           string `json:"type"`
}

func (s *Server) handleLegacyMonitorAction(w http.ResponseWriter, r *http.Request, user any, path string) bool {
	switch {
	case path == "listarticle":
		writeRawJSON(w, http.StatusOK, "")
	case path == "articleDetail":
		s.handleLegacyMonitorArticleDetail(w, r, user)
	case path == "relatedArticles":
		s.handleLegacyMonitorRelatedArticles(w, r, user)
	case path == "getCondition":
		s.handleLegacyMonitorGetCondition(w, r, user)
	case path == "getarticle":
		s.handleLegacyMonitorGetArticle(w, r, user)
	case path == "getSimilarArticle":
		s.handleLegacyMonitorGetSimilarArticle(w, r, user)
	case path == "getanalysisarticle":
		s.handleLegacyMonitorGetAnalysisArticle(w, r, user)
	case path == "getindustry":
		s.handleLegacySearchBuckets("industry")(w, cloneRequestWithJSONBody(r), user)
	case path == "getevent":
		s.handleLegacySearchBuckets("event")(w, cloneRequestWithJSONBody(r), user)
	case path == "getprovince":
		s.handleLegacySearchBuckets("province")(w, cloneRequestWithJSONBody(r), user)
	case path == "getcity":
		s.handleLegacySearchBuckets("city")(w, cloneRequestWithJSONBody(r), user)
	case path == "getapparticle":
		s.handleLegacyMonitorGetAppArticle(w, r, user)
	case path == "getgroupname":
		s.handleLegacyMonitorGetGroupName(w, r, user)
	case path == "warningSetting":
		if r.Method == http.MethodGet {
			s.handleLegacyWarningSettingDetail(w, r, user)
		} else {
			s.handleLegacyUpdateWarning(w, formRequestFromJSON(r), user)
		}
	case strings.HasPrefix(path, "warningSetting/"):
		query := r.URL.Query()
		query.Set("projectId", strings.TrimPrefix(path, "warningSetting/"))
		r2 := cloneRequest(r)
		r2.URL.RawQuery = query.Encode()
		s.handleLegacyWarningSettingDetail(w, r2, user)
	case path == "edit/read":
		s.handleLegacyMonitorEditRead(w, r, user)
	case path == "edit/status":
		s.handleLegacyMonitorEditStatus(w, r, user)
	default:
		return false
	}
	return true
}

func (s *Server) handleLegacyMonitorArticleDetail(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	articleID := nonEmpty(req.ArticleID, req.ArticleIDAlt, r.FormValue("articleId"), r.FormValue("articleid"))
	if articleID == "" {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"code": 500, "msg": "articleId required"})
		return
	}
	item, err := s.fetchLegacyItemByID(articleID, userIDFromMap(user))
	if err != nil {
		writeRawJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "msg": "not found"})
		return
	}
	payload := legacySearchArticleDetailPayload(item)
	if projectID := parseProjectID(nonEmpty(req.ProjectIDAlt2, req.ProjectIDAlt, req.ProjectIDRaw)); projectID > 0 {
		_, _ = s.fetchLegacyWarningSetting(projectID)
	}
	writeRawJSON(w, http.StatusOK, payload)
}

func (s *Server) handleLegacyMonitorRelatedArticles(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	if articleID := nonEmpty(req.ArticleID, req.ArticleIDAlt, r.FormValue("articleId"), r.FormValue("articleid")); articleID != "" {
		var related []model.Item
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles/"+articleID+"/related?limit=10", &related); err == nil {
			data := make([]map[string]any, 0, len(related))
			for _, item := range related {
				data = append(data, map[string]any{
					"article_public_id": legacyArticlePublicID(item),
					"title":             item.Title,
					"source_name":       nonEmpty(item.FromText, item.SourceType),
					"publish_time":      legacyPublishTime(item),
					"source_url":        nonEmpty(item.SourceURL, item.DetailURL),
				})
			}
			writeRawJSON(w, http.StatusOK, data)
			return
		}
	}
	items, err := s.fetchLegacySearchItems(cloneRequestWithJSONBody(r), req.legacySearchRequest)
	if err != nil {
		writeRawJSON(w, http.StatusInternalServerError, []map[string]any{})
		return
	}
	data := make([]map[string]any, 0, min(len(items), 10))
	for _, item := range items {
		data = append(data, map[string]any{
			"article_public_id": legacyArticlePublicID(item),
			"title":             item.Title,
			"source_name":       nonEmpty(item.FromText, item.SourceType),
			"publish_time":      legacyPublishTime(item),
			"source_url":        nonEmpty(item.SourceURL, item.DetailURL),
		})
		if len(data) >= 10 {
			break
		}
	}
	writeRawJSON(w, http.StatusOK, data)
}

func (s *Server) handleLegacyMonitorGetCondition(w http.ResponseWriter, r *http.Request, _ any) {
	req, _ := decodeLegacyMonitorRequest(r)
	projectID := parseProjectID(nonEmpty(req.ProjectIDAlt2, req.ProjectIDAlt, req.ProjectIDRaw, req.ProjectID3, req.ProjectID2, req.ProjectID))
	options, _ := s.fetchSearchOptions()
	condition := map[string]any{
		"projectid":  projectID,
		"searchType": 1,
		"timeType":   max(req.TimeType, 4),
		"matchingmode": nonEmpty(
			req.MatchingMode,
			"1",
		),
		"precise":     strconv.Itoa(req.Precise),
		"emotion":     nonEmpty(req.EmotionalIndex, "0"),
		"sort":        "desc",
		"merge":       "0",
		"industry":    options.Industries,
		"province":    options.Provinces,
		"city":        options.Cities,
		"groupList":   s.fetchLegacyProjectGroups(),
		"projectList": s.fetchLegacyProjects(),
	}
	writeRawJSON(w, http.StatusOK, condition)
}

func (s *Server) handleLegacyMonitorGetArticle(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	filter := legacyMonitorFilterFromRequest(req, userIDFromMap(user))
	result, err := s.fetchLegacyMonitorSearchResult(filter)
	if err != nil {
		writeResultUtilJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	payload := legacyMonitorArticlePage(result, filter, s.fetchLegacyProjectMap())
	writeResultUtilJSON(w, http.StatusOK, "success", payload)
}

func (s *Server) handleLegacyMonitorGetSimilarArticle(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	if articleID := parseProjectID(nonEmpty(req.ArticleID, req.ArticleIDAlt)); articleID > 0 {
		var related []model.Item
		if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles/"+strconv.FormatInt(articleID, 10)+"/related?limit=20", &related); err == nil {
			result := model.SearchResult{Items: related, Total: len(related), Page: 1, PageSize: len(related)}
			writeResultUtilJSON(w, http.StatusOK, "success", legacyMonitorArticlePage(result, legacyMonitorFilterFromRequest(req, userIDFromMap(user)), s.fetchLegacyProjectMap()))
			return
		}
	}
	s.handleLegacyMonitorGetArticle(w, r, user)
}

func (s *Server) handleLegacyMonitorGetAnalysisArticle(w http.ResponseWriter, r *http.Request, user any) {
	s.handleLegacyMonitorGetArticle(w, r, user)
}

func (s *Server) handleLegacyMonitorGetAppArticle(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	filter := legacyMonitorFilterFromRequest(req, userIDFromMap(user))
	result, err := s.fetchLegacyMonitorSearchResult(filter)
	if err != nil {
		writeRawJSON(w, http.StatusInternalServerError, map[string]any{"status": 500, "msg": err.Error()})
		return
	}
	projectMap := s.fetchLegacyProjectMap()
	groupNames := s.fetchLegacyGroupNameMap()
	items := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		projectID, groupID := firstLegacyProjectAndGroup(item, projectMap)
		items = append(items, map[string]any{
			"article_public_id": legacyArticlePublicID(item),
			"title":             item.Title,
			"source_name":       nonEmpty(item.FromText, item.SourceType),
			"publish_time":      legacyPublishTime(item),
			"emotionalIndex":    legacyEmotionalIndex(item),
			"groupName":         groupNames[groupID],
			"groupid":           groupID,
			"projectid":         projectID,
			"source_url":        nonEmpty(item.SourceURL, item.DetailURL),
		})
	}
	payload := map[string]any{
		"data": map[string]any{
			"list":      items,
			"total":     result.Total,
			"pageNum":   max(result.Page, 1),
			"pageSize":  max(result.PageSize, 1),
			"pageCount": calcLegacyPages(result.Total, result.PageSize),
			"groupName": firstLegacyGroupNameFromItems(result.Items, projectMap, groupNames),
		},
	}
	writeRawJSON(w, http.StatusOK, payload)
}

func (s *Server) handleLegacyMonitorGetGroupName(w http.ResponseWriter, r *http.Request, _ any) {
	req, _ := decodeLegacyMonitorRequest(r)
	projectID := parseProjectID(nonEmpty(req.ProjectIDAlt2, req.ProjectIDAlt, req.ProjectIDRaw, req.ProjectID3, req.ProjectID2, req.ProjectID))
	projectMap := s.fetchLegacyProjectMap()
	project, ok := projectMap[projectID]
	if !ok {
		writeRawJSON(w, http.StatusOK, map[string]any{"group_name": "", "groupid": 0, "projectid": projectID})
		return
	}
	groupNames := s.fetchLegacyGroupNameMap()
	writeRawJSON(w, http.StatusOK, map[string]any{
		"group_name": groupNames[project.GroupID],
		"groupid":    project.GroupID,
		"projectid":  project.ID,
		"name":       groupNames[project.GroupID],
	})
}

func (s *Server) handleLegacyMonitorEditRead(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	formReq := formRequestFromJSON(r)
	values := formReq.Form
	if values == nil {
		values = url.Values{}
	}
	if values.Get("id") == "" {
		values.Set("id", nonEmpty(req.ArticleID, req.ArticleIDAlt))
	}
	if values.Get("flag") == "" {
		values.Set("flag", nonEmpty(req.Type, req.Flag, "1"))
	}
	formReq.Form = values
	formReq.PostForm = values
	itemID := parseProjectID(nonEmpty(values.Get("id"), values.Get("articleid"), values.Get("articleId"), r.URL.Query().Get("id")))
	if itemID <= 0 {
		writeLegacyMonitorJSON(w, http.StatusBadRequest, "fail")
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeLegacyMonitorJSON(w, http.StatusForbidden, "fail")
		return
	}
	flag, _ := strconv.Atoi(strings.TrimSpace(formReq.FormValue("flag")))
	var item model.Item
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/articles/"+strconv.FormatInt(itemID, 10)+"?user_id="+strconv.FormatInt(userID, 10), &item); err != nil {
		writeLegacyMonitorJSON(w, http.StatusInternalServerError, "fail")
		return
	}
	switch flag {
	case 1:
		if item.Read {
			writeLegacyMonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
		resp, err := s.client.R().
			SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
			Post(s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10) + "/read")
		if err != nil || !resp.IsSuccess() {
			writeLegacyMonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
		writeLegacyMonitorJSON(w, http.StatusOK, "success")
	case 2:
		resp, err := s.client.R().
			SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
			Delete(s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10) + "/read")
		if err != nil || !resp.IsSuccess() {
			writeLegacyMonitorJSON(w, http.StatusInternalServerError, "fail")
			return
		}
		writeLegacyMonitorJSON(w, http.StatusOK, "success")
	default:
		writeLegacyMonitorJSON(w, http.StatusBadRequest, "fail")
	}
}

func writeLegacyMonitorJSON(w http.ResponseWriter, status int, result any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"result": result,
	})
}

func (s *Server) handleLegacyMonitorEditStatus(w http.ResponseWriter, r *http.Request, user any) {
	req, _ := decodeLegacyMonitorRequest(r)
	itemID := parseProjectID(nonEmpty(req.ArticleID, req.ArticleIDAlt, r.FormValue("id"), r.URL.Query().Get("id")))
	if itemID <= 0 {
		writeResultUtilJSON(w, http.StatusBadRequest, "id required", nil)
		return
	}
	status := "invalid"
	if nonEmpty(req.Type, req.Flag, r.FormValue("type"), r.FormValue("flag")) == "2" {
		status = "active"
	}
	resp, err := s.client.R().
		SetBody(map[string]string{"status": status}).
		Put(s.cfg.ContentURL + "/api/v1/articles/" + strconv.FormatInt(itemID, 10) + "/status")
	if err != nil || !resp.IsSuccess() {
		writeResultUtilJSON(w, http.StatusInternalServerError, "fail", nil)
		return
	}
	writeResultUtilJSON(w, http.StatusOK, "success", map[string]any{"status": status, "user_id": userIDFromMap(user)})
}

func decodeLegacyMonitorRequest(r *http.Request) (legacyMonitorRequest, error) {
	var req legacyMonitorRequest
	if r == nil {
		return req, nil
	}
	body, err := readRequestBody(r)
	if err != nil || len(body) == 0 {
		_ = r.ParseForm()
		req.ArticleID = nonEmpty(r.FormValue("articleId"), r.FormValue("articleid"))
		req.ArticleIDAlt = req.ArticleID
		req.ProjectIDRaw = nonEmpty(r.FormValue("projectid"), r.FormValue("projectId"), r.FormValue("project_id"))
		req.ProjectIDAlt = req.ProjectIDRaw
		req.ProjectIDAlt2 = req.ProjectIDRaw
		req.GroupID = nonEmpty(r.FormValue("groupid"), r.FormValue("group_id"))
		req.SearchKeyword = nonEmpty(r.FormValue("searchkeyword"), r.FormValue("searchWord"), r.FormValue("searchword"), r.FormValue("keyword"))
		req.MonitorSearch = nonEmpty(r.FormValue("monitorsearch"), req.SearchKeyword)
		req.Keywords = r.FormValue("keywords")
		req.RelatedWord = r.FormValue("relatedword")
		req.PublishTime = r.FormValue("publish_time")
		req.MatchingMode = r.FormValue("matchingmode")
		req.EmotionalIndex = r.FormValue("emotionalIndex")
		req.OpenFlag = nonEmpty(r.FormValue("openFlag"), r.FormValue("open_flag"))
		req.Flag = r.FormValue("flag")
		req.Type = r.FormValue("type")
		req.SourceType = r.FormValue("source_type")
		req.Times = r.FormValue("times")
		req.Timee = r.FormValue("timee")
		req.SearchType = parsePositiveInt(r.FormValue("searchType"), 0)
		req.TimeType = parsePositiveInt(r.FormValue("timeType"), 0)
		req.Page = parsePositiveInt(nonEmpty(r.FormValue("page"), r.FormValue("pageNum")), 0)
		req.PageNum = req.Page
		return req, nil
	}
	err = json.Unmarshal(body, &req)
	return req, err
}

func legacyMonitorFilterFromRequest(req legacyMonitorRequest, userID int64) model.ArticleFilter {
	page := req.Page
	if page <= 0 {
		page = req.PageNum
	}
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	filter := model.ArticleFilter{
		Page:       page,
		PageSize:   pageSize,
		Keyword:    nonEmpty(req.SearchKeyword, req.MonitorSearch, req.Keyword, req.Searchword, req.SearchWord, req.Keywords),
		SourceType: strings.TrimSpace(req.SourceType),
		ProjectID:  parseProjectID(nonEmpty(req.ProjectIDAlt2, req.ProjectIDAlt, req.ProjectIDRaw, req.ProjectID3, req.ProjectID2, req.ProjectID)),
		UserID:     userID,
		Start:      nonEmpty(req.Start, req.Times),
		End:        nonEmpty(req.End, req.Timee),
		Sort:       "captured_at_desc",
		Read:       legacyReadFilter(req.Flag, req.Read),
		Favorite:   strings.TrimSpace(req.Favorite),
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 10
	}
	return filter
}

func (s *Server) fetchLegacyMonitorSearchResult(filter model.ArticleFilter) (model.SearchResult, error) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(max(filter.Page, 1)))
	query.Set("page_size", strconv.Itoa(max(filter.PageSize, 1)))
	if filter.Keyword != "" {
		query.Set("q", filter.Keyword)
	}
	if filter.ProjectID > 0 {
		query.Set("project_id", strconv.FormatInt(filter.ProjectID, 10))
	}
	if filter.SourceType != "" {
		query.Set("source_type", filter.SourceType)
	}
	if filter.Start != "" {
		query.Set("start", filter.Start)
	}
	if filter.End != "" {
		query.Set("end", filter.End)
	}
	if filter.UserID > 0 {
		query.Set("user_id", strconv.FormatInt(filter.UserID, 10))
	}
	if filter.Read != "" {
		query.Set("read", filter.Read)
	}
	if filter.Favorite != "" {
		query.Set("favorite", filter.Favorite)
	}
	var result model.SearchResult
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result)
	return result, err
}

func legacyMonitorArticlePage(result model.SearchResult, filter model.ArticleFilter, projectMap map[int64]model.Project) map[string]any {
	groupNames := map[int64]string{}
	for _, project := range projectMap {
		if strings.TrimSpace(project.GroupName) != "" {
			groupNames[project.GroupID] = project.GroupName
		}
	}
	list := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		projectID, groupID := firstLegacyProjectAndGroup(item, projectMap)
		entry := map[string]any{
			"article_public_id": legacyArticlePublicID(item),
			"title":             item.Title,
			"content":           nonEmpty(item.Summary, item.Content),
			"source_name":       nonEmpty(item.FromText, item.SourceType),
			"publish_time":      legacyPublishTime(item),
			"emotionalIndex":    legacyEmotionalIndex(item),
			"groupid":           groupID,
			"projectid":         projectID,
			"group_name":        groupNames[groupID],
			"url":               legacyMonitorDetailURL(item, projectID, groupID),
			"read":              item.Read,
			"favorited":         item.Favorited,
			"source_url":        nonEmpty(item.SourceURL, item.DetailURL),
		}
		list = append(list, entry)
	}
	return map[string]any{
		"list": list,
		"pageInfo": map[string]any{
			"pageNum":  max(result.Page, 1),
			"pages":    calcLegacyPages(result.Total, result.PageSize),
			"total":    result.Total,
			"pageSize": max(result.PageSize, 1),
		},
	}
}

func cloneRequestWithJSONBody(r *http.Request) *http.Request {
	req := cloneRequest(r)
	req.Body = nil
	req.ContentLength = 0
	return req
}

func cloneRequest(r *http.Request) *http.Request {
	req := r.Clone(r.Context())
	req.Form = nil
	req.PostForm = nil
	return req
}

func formRequestFromJSON(r *http.Request) *http.Request {
	req := cloneRequest(r)
	values := url.Values{}
	if parsed, err := decodeLegacyMonitorRequest(r); err == nil {
		if parsed.ArticleID != "" {
			values.Set("id", parsed.ArticleID)
		} else if parsed.ArticleIDAlt != "" {
			values.Set("id", parsed.ArticleIDAlt)
		}
		for key, value := range map[string]string{
			"flag":           parsed.Flag,
			"type":           parsed.Type,
			"projectid":      parsed.ProjectIDRaw,
			"projectId":      parsed.ProjectIDAlt,
			"project_id":     parsed.ProjectIDAlt2,
			"groupid":        parsed.GroupID,
			"group_id":       parsed.GroupIDAlt,
			"warning_status": parsed.Flag,
			"keyword":        parsed.Keyword,
			"openFlag":       parsed.OpenFlag,
			"matchingmode":   parsed.MatchingMode,
			"searchkeyword":  parsed.SearchKeyword,
			"monitorsearch":  parsed.MonitorSearch,
			"publish_time":   parsed.PublishTime,
			"emotionalIndex": parsed.EmotionalIndex,
		} {
			if strings.TrimSpace(value) != "" {
				values.Set(key, value)
			}
		}
	}
	if err := req.ParseForm(); err == nil {
		for key, list := range req.Form {
			for _, value := range list {
				if _, ok := values[key]; !ok {
					values.Add(key, value)
				}
			}
		}
	}
	req.Form = values
	req.PostForm = values
	req.Body = http.NoBody
	req.ContentLength = 0
	return req
}

func readRequestBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return nil, err
	}
	body := buf.Bytes()
	r.Body = ioNopCloser(bytes.NewReader(body))
	return body, nil
}

func ioNopCloser(reader *bytes.Reader) *readCloser {
	return &readCloser{Reader: reader}
}

type readCloser struct {
	*bytes.Reader
}

func (r *readCloser) Close() error { return nil }

func legacyReadFilter(flag string, current string) string {
	switch strings.TrimSpace(flag) {
	case "1":
		return "unread"
	case "2":
		return "read"
	}
	return strings.TrimSpace(current)
}

func legacyMonitorDetailURL(item model.Item, projectID, groupID int64) string {
	values := url.Values{}
	if groupID > 0 {
		values.Set("groupid", strconv.FormatInt(groupID, 10))
	}
	if projectID > 0 {
		values.Set("projectid", strconv.FormatInt(projectID, 10))
	}
	target := "/monitor/detail/" + strconv.FormatInt(item.ID, 10)
	if len(values) > 0 {
		target += "?" + values.Encode()
	}
	return target
}

func calcLegacyPages(total, pageSize int) int {
	if pageSize <= 0 {
		pageSize = 1
	}
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

func firstLegacyGroupNameFromItems(items []model.Item, projectMap map[int64]model.Project, groupNames map[int64]string) string {
	for _, item := range items {
		_, groupID := firstLegacyProjectAndGroup(item, projectMap)
		if groupID > 0 {
			return groupNames[groupID]
		}
	}
	return ""
}

func (s *Server) fetchSearchOptions() (model.SearchOptions, error) {
	var options model.SearchOptions
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/options", &options); err != nil {
		return model.SearchOptions{}, err
	}
	return options, nil
}
