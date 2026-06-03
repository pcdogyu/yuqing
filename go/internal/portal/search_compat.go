package portal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type legacySearchFullType struct {
	OnlyID     int    `json:"only_id"`
	ID         int    `json:"id"`
	CreateTime string `json:"create_time"`
	Type       int    `json:"type"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	TypeOneID  int    `json:"type_one_id"`
	TypeTwoID  int    `json:"type_two_id"`
	Icon       string `json:"icon"`
	IsShow     int    `json:"is_show"`
	IsDefault  int    `json:"is_default"`
}

type legacySearchPolymerization struct {
	ID         int    `json:"id"`
	CreateTime string `json:"create_time"`
	Type       int    `json:"type"`
	TypeName   string `json:"type_name"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	Icon       string `json:"icon"`
	IsShow     int    `json:"is_show"`
}

type legacySearchArticle struct {
	ArticlePublicID   string   `json:"article_public_id"`
	Classify          int      `json:"classify"`
	Websitelogo       string   `json:"websitelogo"`
	Author            string   `json:"author"`
	KeyWords          string   `json:"key_words"`
	SourceWebsiteName string   `json:"sourcewebsitename"`
	Title             string   `json:"title"`
	Content           string   `json:"content"`
	EmotionalIndex    string   `json:"emotionalIndex"`
	PublishTime       string   `json:"publish_time"`
	ExtendStringOne   string   `json:"extend_string_one"`
	Forwardingvolume  int      `json:"forwardingvolume"`
	Commentsvolume    int      `json:"commentsvolume"`
	Praisevolume      int      `json:"praisevolume"`
	Industrylable     string   `json:"industrylable"`
	Eventlable        string   `json:"eventlable"`
	ArticleCategory   string   `json:"article_category"`
	RelatedWord       []string `json:"relatedWord"`
	Ner               []any    `json:"ner"`
	Num               int      `json:"num"`
	SourceURL         string   `json:"source_url"`
	Source            string   `json:"source"`
	Url               string   `json:"url"`
	Abstract          string   `json:"abstract"`
	PublishTimeText   string   `json:"publish_time_text"`
	VideoJSON         string   `json:"videojson"`
}

type legacySearchArticleListResponse struct {
	Data                []legacySearchArticle `json:"data"`
	TotalPage           int                   `json:"totalPage"`
	TotalCount          int                   `json:"totalCount"`
	CurrentPage         int                   `json:"currentPage"`
	ArticlePublicIDList []string              `json:"article_public_idList,omitempty"`
}

var legacyFullSearchTypes = []legacySearchFullType{
	{OnlyID: 1, ID: 1, Type: 1, Name: "资讯", Icon: "mdi mdi-newspaper", IsShow: 0, IsDefault: 0},
	{OnlyID: 8, ID: 8, Type: 1, Name: "热点", Icon: "mdi mdi-fire", IsShow: 0, IsDefault: 0},
	{OnlyID: 23, ID: 23, Type: 1, Name: "投诉", Icon: "mdi mdi-alert", IsShow: 0, IsDefault: 0},
	{OnlyID: 28, ID: 28, Type: 1, Name: "公告", Icon: "mdi mdi-bullhorn", IsShow: 0, IsDefault: 0},
	{OnlyID: 35, ID: 35, Type: 1, Name: "研报", Icon: "mdi mdi-chart-line", IsShow: 0, IsDefault: 0},
	{OnlyID: 36, ID: 36, Type: 1, Name: "招聘", Icon: "mdi mdi-account-plus", IsShow: 0, IsDefault: 0},
	{OnlyID: 37, ID: 37, Type: 1, Name: "招标", Icon: "mdi mdi-clipboard-text", IsShow: 0, IsDefault: 0},
	{OnlyID: 38, ID: 38, Type: 1, Name: "资讯聚合", Icon: "mdi mdi-view-list", IsShow: 0, IsDefault: 0},
	{OnlyID: 39, ID: 39, Type: 1, Name: "工商", Icon: "mdi mdi-domain", IsShow: 0, IsDefault: 0},
	{OnlyID: 40, ID: 40, Type: 1, Name: "投资融资", Icon: "mdi mdi-cash-multiple", IsShow: 0, IsDefault: 0},
	{OnlyID: 41, ID: 41, Type: 1, Name: "百度知道", Icon: "mdi mdi-help-circle", IsShow: 0, IsDefault: 0},
	{OnlyID: 42, ID: 42, Type: 1, Name: "法律文书", Icon: "mdi mdi-scale-balance", IsShow: 0, IsDefault: 0},
	{OnlyID: 43, ID: 43, Type: 1, Name: "知识产权", Icon: "mdi mdi-lightbulb", IsShow: 0, IsDefault: 0},
	{OnlyID: 45, ID: 45, Type: 1, Name: "学术", Icon: "mdi mdi-school", IsShow: 0, IsDefault: 0},
	{OnlyID: 100, ID: 100, Type: 1, Name: "律师", Icon: "mdi mdi-account-tie", IsShow: 0, IsDefault: 0},
	{OnlyID: 101, ID: 101, Type: 1, Name: "被执行人", Icon: "mdi mdi-account-alert", IsShow: 0, IsDefault: 0},
	{OnlyID: 102, ID: 102, Type: 1, Name: "专家人才", Icon: "mdi mdi-account-star", IsShow: 0, IsDefault: 0},
	{OnlyID: 103, ID: 103, Type: 1, Name: "医生", Icon: "mdi mdi-hospital", IsShow: 0, IsDefault: 0},
}

var legacySearchPolymerizations = []legacySearchPolymerization{
	{ID: 1, Type: 0, TypeName: "竞争对手", Name: "竞争对手", Value: "1,100,101", Icon: "mdi mdi-account-group", IsShow: 0},
	{ID: 2, Type: 0, TypeName: "领域范围", Name: "领域范围", Value: "8,102", Icon: "mdi mdi-sitemap", IsShow: 0},
	{ID: 3, Type: 0, TypeName: "政策法规", Name: "政策法规", Value: "23,28", Icon: "mdi mdi-file-document", IsShow: 0},
	{ID: 4, Type: 0, TypeName: "产业市场", Name: "产业市场", Value: "35,39", Icon: "mdi mdi-office-building", IsShow: 0},
	{ID: 5, Type: 0, TypeName: "产品品牌", Name: "产品品牌", Value: "36,37,40,45", Icon: "mdi mdi-tag-multiple", IsShow: 0},
	{ID: 6, Type: 0, TypeName: "技术人才", Name: "技术人才", Value: "41,42,43", Icon: "mdi mdi-flask", IsShow: 0},
}

func (s *Server) handleFullSearchEntry(w http.ResponseWriter, r *http.Request, _ any) {
	http.Redirect(w, r, s.legacySearchTarget("full", r), http.StatusSeeOther)
}

func (s *Server) handleTimelySearchEntry(w http.ResponseWriter, r *http.Request, _ any) {
	http.Redirect(w, r, s.legacySearchTarget("timely", r), http.StatusSeeOther)
}

func (s *Server) handleFullSearchCompat(w http.ResponseWriter, r *http.Request, user any) {
	s.handleSearchCompat(w, r, user, "full")
}

func (s *Server) handleTimelySearchCompat(w http.ResponseWriter, r *http.Request, user any) {
	s.handleSearchCompat(w, r, user, "timely")
}

func (s *Server) handleSearchCompat(w http.ResponseWriter, r *http.Request, user any, mode string) {
	path := strings.TrimPrefix(r.URL.Path, "/"+mode+"search/")
	switch path {
	case "", "result":
		s.handleLegacySearchResult(w, r, user, mode)
	case "search":
		s.handleLegacySearchHistory(w, r, user)
	case "listFullTypeByFirst":
		apiutil.WriteJSON(w, http.StatusOK, "ok", legacySearchTypesForMode(mode))
	case "listFullTypeBySecond":
		typeOneID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("type_one_id")))
		apiutil.WriteJSON(w, http.StatusOK, "ok", legacySearchTypesBySecond(typeOneID))
	case "listFullTypeByThird":
		typeTwoID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("type_two_id")))
		apiutil.WriteJSON(w, http.StatusOK, "ok", legacySearchTypesByThird(typeTwoID))
	case "listFullTypeOneByIdList":
		apiutil.WriteJSON(w, http.StatusOK, "ok", legacySearchTypesByIDs(strings.TrimSpace(r.URL.Query().Get("id"))))
	case "listFullPolymerization":
		apiutil.WriteJSON(w, http.StatusOK, "ok", legacySearchPolymerizations)
	case "getBreadCrumbs":
		apiutil.WriteJSON(w, http.StatusOK, "ok", legacySearchBreadcrumbs(r))
	case "hotList":
		if mode == "full" {
			s.handleLegacyHotList(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "informationList", "informationListpost":
		s.handleLegacySearchInformationList(w, r, user, mode)
	case "data":
		if mode == "timely" {
			s.handleTimelySearchData(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "templete":
		if mode == "timely" {
			s.handleTimelySearchTemplate(w, r, user)
			return
		}
		http.NotFound(w, r)
	default:
		if strings.Contains(path, "Detail/") || strings.Contains(path, "detail/") {
			s.handleLegacySearchArticleRedirect(w, r, mode)
			return
		}
		http.Redirect(w, r, s.legacySearchTarget(mode, r), http.StatusSeeOther)
	}
}

func (s *Server) handleLegacySearchResult(w http.ResponseWriter, r *http.Request, user any, mode string) {
	if userID := userIDFromMap(user); userID > 0 {
		_ = s.recordLegacySearchWord(r, userID)
	}
	http.Redirect(w, r, s.legacySearchTarget(mode, r), http.StatusSeeOther)
}

func (s *Server) handleLegacySearchHistory(w http.ResponseWriter, r *http.Request, user any) {
	userID := userIDFromMap(user)
	if userID <= 0 {
		apiutil.WriteJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	var words []model.SearchWordStat
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/history?user_id="+strconv.FormatInt(userID, 10)+"&limit=6", &words); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", words)
}

func (s *Server) handleLegacySearchInformationList(w http.ResponseWriter, r *http.Request, user any, mode string) {
	filter, pageSize := legacySearchFilterFromRequest(r, mode)
	var result model.SearchResult
	searchURL := s.cfg.ContentURL + "/api/v1/search/full"
	if mode == "timely" {
		searchURL = s.cfg.ContentURL + "/api/v1/search/timely"
	}
	query := url.Values{}
	query.Set("page", strconv.Itoa(max(filter.Page, 1)))
	query.Set("page_size", strconv.Itoa(max(filter.PageSize, pageSize)))
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
	if filter.Industry != "" {
		query.Set("industry", filter.Industry)
	}
	if filter.Province != "" {
		query.Set("province", filter.Province)
	}
	if filter.City != "" {
		query.Set("city", filter.City)
	}
	if filter.Read != "" {
		query.Set("read", filter.Read)
	}
	if filter.Favorite != "" {
		query.Set("favorite", filter.Favorite)
	}
	if err := s.getJSON(searchURL+"?"+query.Encode(), &result); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	articles := make([]legacySearchArticle, 0, len(result.Items))
	articleIDs := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		articles = append(articles, legacySearchArticleFromItem(item, filter.Keyword))
		articleIDs = append(articleIDs, strconv.FormatInt(item.ID, 10))
	}
	totalPage := 1
	if result.PageSize > 0 {
		totalPage = (result.Total + result.PageSize - 1) / result.PageSize
		if totalPage <= 0 {
			totalPage = 1
		}
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"data":                  articles,
		"totalPage":             totalPage,
		"totalCount":            result.Total,
		"currentPage":           max(result.Page, 1),
		"article_public_idList": articleIDs,
	})
}

func (s *Server) handleLegacyHotList(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	page := max(apiutil.IntQuery(r, "pageNum", 1), 1)
	pageSize := apiutil.IntQuery(r, "pageSize", 25)
	if pageSize <= 0 {
		pageSize = 25
	}
	keyword := nonEmpty(
		strings.TrimSpace(r.URL.Query().Get("searchWord")),
		strings.TrimSpace(r.URL.Query().Get("searchword")),
		strings.TrimSpace(r.URL.Query().Get("keyword")),
	)
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	if keyword != "" {
		query.Set("q", keyword)
	}
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, map[string]any{
			"_source": legacyHotItemSource(item),
		})
	}
	totalPages := 1
	if result.PageSize > 0 && result.Total > 0 {
		totalPages = (result.Total + result.PageSize - 1) / result.PageSize
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"code":       http.StatusOK,
		"data":       items,
		"page_count": totalPages,
		"count":      result.Total,
		"page":       max(result.Page, page),
		"size":       max(result.PageSize, pageSize),
	})
}

func (s *Server) handleLegacySearchArticleRedirect(w http.ResponseWriter, r *http.Request, mode string) {
	articleID := legacySearchArticleIDFromPath(strings.TrimPrefix(r.URL.Path, "/"+mode+"search/"))
	if articleID == "" {
		http.Redirect(w, r, s.legacySearchTarget(mode, r), http.StatusSeeOther)
		return
	}
	returnTo := legacySearchResultPath(mode, r)
	target := "/articles/" + url.PathEscape(articleID) + "?return_to=" + url.QueryEscape(returnTo)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleTimelySearchData(w http.ResponseWriter, r *http.Request, user any) {
	started := time.Now()
	filter, _ := legacySearchFilterFromRequest(r, "timely")
	filter.Page = max(apiutil.IntQuery(r, "pageNoData", 1), 1)
	filter.PageSize = 30
	query := url.Values{}
	query.Set("page", strconv.Itoa(filter.Page))
	query.Set("page_size", strconv.Itoa(filter.PageSize))
	if filter.Keyword != "" {
		query.Set("q", filter.Keyword)
	}
	if filter.Industry != "" {
		query.Set("industry", filter.Industry)
	}
	if filter.Province != "" {
		query.Set("province", filter.Province)
	}
	if filter.City != "" {
		query.Set("city", filter.City)
	}
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/timely?"+query.Encode(), &result); err != nil {
		writeJSONText(w, map[string]any{"time": 0, "data": "[]"})
		return
	}
	items := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, map[string]any{
			"title":        item.Title,
			"abstract":     nonEmpty(item.Summary, item.Content),
			"url":          nonEmpty(item.SourceURL, item.DetailURL, "/articles/"+strconv.FormatInt(item.ID, 10)),
			"publish_time": nonEmpty(item.PublishTimeText, item.PublishTime),
			"source":       nonEmpty(item.FromText, item.SourceType),
			"videojson":    "",
			"author":       item.FromText,
		})
	}
	body, _ := json.Marshal(items)
	writePlainJSONText(w, map[string]any{
		"time": time.Since(started).Milliseconds(),
		"data": string(body),
	})
}

func (s *Server) handleTimelySearchTemplate(w http.ResponseWriter, r *http.Request, user any) {
	_ = r
	writeJSONText(w, []map[string]any{
		{"id": 1, "engine": "全部"},
		{"id": 2, "engine": "默认"},
	})
}

func (s *Server) recordLegacySearchWord(r *http.Request, userID int64) error {
	searchWord := nonEmpty(
		strings.TrimSpace(r.URL.Query().Get("searchword")),
		strings.TrimSpace(r.URL.Query().Get("searchWord")),
		strings.TrimSpace(r.URL.Query().Get("keyword")),
	)
	if searchWord == "" || userID <= 0 {
		return nil
	}
	resp, err := s.client.R().
		SetQueryParam("user_id", strconv.FormatInt(userID, 10)).
		SetBody(map[string]string{"search_word": searchWord}).
		Post(s.cfg.ContentURL + "/api/v1/search/history")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func legacySearchFilterFromRequest(r *http.Request, mode string) (model.ArticleFilter, int) {
	page := max(apiutil.IntQuery(r, "page", 1), 1)
	pageSize := apiutil.IntQuery(r, "pageSize", 50)
	if pageSize <= 0 {
		pageSize = 50
	}
	keyword := nonEmpty(r.URL.Query().Get("searchword"), r.URL.Query().Get("searchWord"), r.URL.Query().Get("keyword"))
	filter := model.ArticleFilter{
		Page:       page,
		PageSize:   pageSize,
		Keyword:    keyword,
		SourceType: strings.TrimSpace(r.URL.Query().Get("source_type")),
		Start:      strings.TrimSpace(r.URL.Query().Get("start")),
		End:        strings.TrimSpace(r.URL.Query().Get("end")),
		Industry:   strings.TrimSpace(r.URL.Query().Get("industry")),
		Province:   strings.TrimSpace(r.URL.Query().Get("province")),
		City:       strings.TrimSpace(r.URL.Query().Get("city")),
		Read:       strings.TrimSpace(r.URL.Query().Get("read")),
		Favorite:   strings.TrimSpace(r.URL.Query().Get("favorite")),
		Mode:       mode,
	}
	if projectID, err := strconv.ParseInt(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("project_id"), r.URL.Query().Get("projectid"))), 10, 64); err == nil {
		filter.ProjectID = projectID
	}
	return filter, pageSize
}

func legacySearchResultPath(mode string, r *http.Request) string {
	values := url.Values{}
	for _, key := range []string{"searchword", "searchWord", "keyword", "fulltype", "full_poly", "menuStyle", "page", "pageSize", "project_id", "projectid", "source_type", "read", "favorite", "start", "end", "industry", "province", "city", "onlyid", "sourcename", "stype", "website_id", "pageNoData"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			values.Set(key, value)
		}
	}
	if values.Get("searchword") == "" {
		if keyword := nonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("searchWord")); keyword != "" {
			values.Set("searchword", keyword)
		}
	}
	if values.Get("page") == "" {
		values.Set("page", "1")
	}
	return "/" + mode + "search/result?" + values.Encode()
}

func legacySearchArticleIDFromPath(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func legacyHotItemSource(item model.Item) map[string]any {
	sentiment := 2
	switch legacyHotSentiment(strings.TrimSpace(item.Title + " " + item.Summary + " " + item.Content)) {
	case "positive":
		sentiment = 1
	case "negative":
		sentiment = 3
	}
	classify := 1
	if strings.Contains(strings.ToLower(item.SourceType), "video") {
		classify = 2
	}
	return map[string]any{
		"source_name":     nonEmpty(item.FromText, item.SourceType, "热点"),
		"topic":           item.Title,
		"spider_time":     nonEmpty(item.PublishTimeText, item.PublishTime, item.CapturedAt.Format("2006-01-02 15:04:05")),
		"sentiment":       sentiment,
		"classify":        classify,
		"sales_volume":    0,
		"original_weight": 0,
		"source_url":      nonEmpty(item.SourceURL, item.DetailURL),
		"article_id":      item.ID,
	}
}

func legacyHotSentiment(text string) string {
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

func legacySearchTypesForMode(mode string) []legacySearchFullType {
	_ = mode
	return legacyFullSearchTypes
}

func legacySearchTypesBySecond(typeOneID int) []legacySearchFullType {
	if typeOneID <= 0 {
		return []legacySearchFullType{}
	}
	return []legacySearchFullType{}
}

func legacySearchTypesByThird(typeTwoID int) []legacySearchFullType {
	if typeTwoID <= 0 {
		return []legacySearchFullType{}
	}
	return []legacySearchFullType{}
}

func legacySearchTypesByIDs(raw string) []legacySearchFullType {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	ids := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && id > 0 {
			ids[id] = struct{}{}
		}
	}
	result := make([]legacySearchFullType, 0, len(ids))
	for _, item := range legacyFullSearchTypes {
		if _, ok := ids[item.OnlyID]; ok {
			result = append(result, item)
		}
	}
	return result
}

func legacySearchBreadcrumbs(r *http.Request) map[string]string {
	menuStyle, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("menuStyle")))
	fulltype, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("fulltype")))
	onlyid, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("onlyid")))
	polyid, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("polyid")))
	result := map[string]string{}
	if menuStyle == 0 {
		if name := legacySearchPolyName(polyid); name != "" {
			result["polyIdName"] = name
		}
		if name := legacySearchFullTypeName(onlyid); name != "" {
			result["onlyIdName"] = name
		}
		return result
	}
	if name := legacySearchFullTypeName(fulltype); name != "" {
		result["fullTypeName"] = name
	}
	return result
}

func legacySearchFullTypeName(id int) string {
	for _, item := range legacyFullSearchTypes {
		if item.ID == id || item.OnlyID == id {
			return item.Name
		}
	}
	return ""
}

func legacySearchPolyName(id int) string {
	for _, item := range legacySearchPolymerizations {
		if item.ID == id {
			return item.TypeName
		}
	}
	return ""
}

func legacySearchArticleFromItem(item model.Item, keyword string) legacySearchArticle {
	sourceType := strings.TrimSpace(item.SourceType)
	classify := 1
	if strings.Contains(strings.ToLower(sourceType), "weibo") || strings.Contains(strings.ToLower(sourceType), "video") {
		classify = 2
	}
	relatedWord := splitLegacyLabels(nonEmpty(item.TagFlags, keyword))
	if len(relatedWord) == 0 && keyword != "" {
		relatedWord = []string{keyword}
	}
	return legacySearchArticle{
		ArticlePublicID:   strconv.FormatInt(item.ID, 10),
		Classify:          classify,
		Websitelogo:       "",
		Author:            nonEmpty(item.FromText, item.ExternalSourceHost),
		KeyWords:          nonEmpty(keyword, strings.Join(relatedWord, ",")),
		SourceWebsiteName: sourceType,
		Title:             item.Title,
		Content:           item.Content,
		EmotionalIndex:    "",
		PublishTime:       nonEmpty(item.PublishTimeText, item.PublishTime),
		ExtendStringOne:   "",
		Forwardingvolume:  0,
		Commentsvolume:    0,
		Praisevolume:      0,
		Industrylable:     "",
		Eventlable:        strings.TrimSpace(item.TagFlags),
		ArticleCategory:   sourceType,
		RelatedWord:       relatedWord,
		Ner:               []any{},
		Num:               0,
		SourceURL:         item.SourceURL,
		Source:            sourceType,
		Url:               nonEmpty(item.SourceURL, item.DetailURL, "/articles/"+strconv.FormatInt(item.ID, 10)),
		Abstract:          nonEmpty(item.Summary, item.Content),
		PublishTimeText:   nonEmpty(item.PublishTimeText, item.PublishTime),
		VideoJSON:         "",
	}
}

func writePlainJSONText(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(payload)
	_, _ = w.Write(bytes.TrimSpace(buf.Bytes()))
}
