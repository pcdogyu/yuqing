package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
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
	s.handleTimelySearchPage(w, r)
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
	case "":
		if mode == "timely" {
			s.handleTimelySearchPage(w, r)
			return
		}
		s.handleLegacySearchResult(w, r, user, mode)
	case "result":
		if mode == "timely" {
			s.handleTimelySearchPage(w, r)
			return
		}
		s.handleLegacySearchResult(w, r, user, mode)
	case "index":
		if mode == "timely" {
			s.handleTimelySearchPage(w, r)
			return
		}
		http.NotFound(w, r)
	case "search":
		s.handleLegacySearchHistory(w, r, user)
	case "execute":
		if mode == "timely" {
			s.handleTimelySearchExecute(w, r)
			return
		}
		http.NotFound(w, r)
	case "listFullTypeByFirst":
		writeJSONText(w, legacySearchTypesForMode(mode))
	case "listFullTypeBySecond":
		typeOneID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("type_one_id")))
		writeJSONText(w, legacySearchTypesBySecond(typeOneID))
	case "listFullTypeByThird":
		typeTwoID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("type_two_id")))
		writeJSONText(w, legacySearchTypesByThird(typeTwoID))
	case "listFullTypeOneByIdList":
		writeJSONText(w, legacySearchTypesByIDs(strings.TrimSpace(r.URL.Query().Get("id"))))
	case "listFullPolymerization":
		writeJSONText(w, legacySearchPolymerizations)
	case "getBreadCrumbs":
		writeJSONText(w, legacySearchBreadcrumbs(r))
	case "hotList":
		if mode == "full" {
			s.handleLegacyHotList(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "complaintList":
		if mode == "full" {
			s.handleLegacyComplaintList(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "announcementList":
		if mode == "full" {
			s.handleLegacyAnnouncementList(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "reportList":
		if mode == "full" {
			s.handleLegacyReportList(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "announcementrtype":
		if mode == "full" {
			writeJSONText(w, legacySearchCategoryOptions(r, "announcement"))
			return
		}
		http.NotFound(w, r)
	case "reportIndustry":
		if mode == "full" {
			writeJSONText(w, legacySearchCategoryOptions(r, "report"))
			return
		}
		http.NotFound(w, r)
	case "lawyerList", "executionPersonList", "professorList", "doctorList", "biddingList", "inviteList", "companyList", "judgmentList", "knowLedgeList", "investmentList", "baiduKnowsList", "thesisnList":
		if mode == "full" {
			s.handleLegacySpecialList(w, r, user, path)
			return
		}
		http.NotFound(w, r)
	case "lawyerDetailData", "executionPersonDetailData", "professorDetailData", "doctorDetailData":
		if mode == "full" {
			s.handleLegacySpecialDetailData(w, r, path)
			return
		}
		http.NotFound(w, r)
	case "companyIndustry":
		if mode == "full" {
			s.handleLegacySpecialCategoryOptions(w, r, "company")
			return
		}
		http.NotFound(w, r)
	case "judgmentCaseType":
		if mode == "full" {
			s.handleLegacySpecialCategoryOptions(w, r, "judgment")
			return
		}
		http.NotFound(w, r)
	case "knowLedgeCaseType":
		if mode == "full" {
			s.handleLegacySpecialCategoryOptions(w, r, "knowledge")
			return
		}
		http.NotFound(w, r)
	case "investmentType":
		if mode == "full" {
			s.handleLegacySpecialCategoryOptions(w, r, "investment")
			return
		}
		http.NotFound(w, r)
	case "companyDetails":
		if mode == "full" || mode == "timely" {
			s.handleLegacyCompanyDetailData(w, r)
			return
		}
		http.NotFound(w, r)
	case "getresearch-report-detail":
		if mode == "full" || mode == "timely" {
			s.handleLegacyReportDetailData(w, r)
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
	case "articleDetail":
		if mode == "timely" {
			s.handleLegacySearchArticleDetailData(w, r, user)
			return
		}
		http.NotFound(w, r)
	case "relatedArticles":
		if mode == "timely" {
			s.handleLegacySearchRelatedArticles(w, r, user)
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
		switch {
		case strings.HasPrefix(path, "lawyerDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "lawyer", strings.TrimPrefix(path, "lawyerDetail/"))
			return
		case strings.HasPrefix(path, "executionPersonDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "executionPerson", strings.TrimPrefix(path, "executionPersonDetail/"))
			return
		case strings.HasPrefix(path, "professorDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "professor", strings.TrimPrefix(path, "professorDetail/"))
			return
		case strings.HasPrefix(path, "doctorDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "doctor", strings.TrimPrefix(path, "doctorDetail/"))
			return
		case strings.HasPrefix(path, "biddingdetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "bidding", strings.TrimPrefix(path, "biddingdetail/"))
			return
		case strings.HasPrefix(path, "inviteDetails/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "invite", strings.TrimPrefix(path, "inviteDetails/"))
			return
		case strings.HasPrefix(path, "companyDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "company", strings.TrimPrefix(path, "companyDetail/"))
			return
		case strings.HasPrefix(path, "judgmentDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "judgment", strings.TrimPrefix(path, "judgmentDetail/"))
			return
		case strings.HasPrefix(path, "knowLedgeDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "knowledge", strings.TrimPrefix(path, "knowLedgeDetail/"))
			return
		case strings.HasPrefix(path, "investmentDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "investment", strings.TrimPrefix(path, "investmentDetail/"))
			return
		case strings.HasPrefix(path, "thesisnDetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "thesisn", strings.TrimPrefix(path, "thesisnDetail/"))
			return
		case strings.HasPrefix(path, "reportdetail/"):
			s.handleLegacySpecialDetailPage(w, r, mode, "report", strings.TrimPrefix(path, "reportdetail/"))
			return
		}
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
	returnPath := legacySearchResultPath(mode, r)
	for _, item := range result.Items {
		articles = append(articles, legacySearchArticleFromItem(item, filter.Keyword, s.legacySpecialDetailTarget(mode, item, returnPath)))
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

func (s *Server) handleLegacyComplaintList(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	filter, pageSize := legacySearchFilterFromRequest(r, "full")
	query := url.Values{}
	query.Set("page", strconv.Itoa(max(filter.Page, 1)))
	query.Set("page_size", strconv.Itoa(max(filter.PageSize, pageSize)))
	if filter.Keyword != "" {
		query.Set("q", filter.Keyword)
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
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	entries := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		entries = append(entries, map[string]any{
			"_source": legacyComplaintSource(item),
		})
	}
	totalPages := 1
	if result.PageSize > 0 && result.Total > 0 {
		totalPages = (result.Total + result.PageSize - 1) / result.PageSize
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"code":       http.StatusOK,
		"news":       entries,
		"count":      result.Total,
		"page_count": totalPages,
		"page":       max(result.Page, filter.Page),
		"size":       max(result.PageSize, max(filter.PageSize, pageSize)),
		"classify":   nonEmpty(r.URL.Query().Get("classify"), "x"),
	})
}

func (s *Server) handleLegacyAnnouncementList(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	filter, pageSize := legacySearchFilterFromRequest(r, "full")
	entries, total, page, size := s.legacyPublicationEntries(r, filter, pageSize)
	totalPages := 1
	if size > 0 && total > 0 {
		totalPages = (total + size - 1) / size
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"code":      http.StatusOK,
		"list":      entries,
		"totalPage": totalPages,
		"totalData": total,
		"page":      page,
		"size":      size,
	})
}

func (s *Server) handleLegacyReportList(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	filter, pageSize := legacySearchFilterFromRequest(r, "full")
	entries, total, page, size := s.legacyReportEntries(r, filter, pageSize)
	totalPages := 1
	if size > 0 && total > 0 {
		totalPages = (total + size - 1) / size
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"code":      http.StatusOK,
		"list":      entries,
		"totalPage": totalPages,
		"totalData": total,
		"page":      page,
		"size":      size,
	})
}

func (s *Server) handleLegacySearchArticleRedirect(w http.ResponseWriter, r *http.Request, mode string) {
	articleID := legacySearchArticleIDFromPath(strings.TrimPrefix(r.URL.Path, "/"+mode+"search/"))
	if articleID == "" {
		http.Redirect(w, r, s.legacySearchTarget(mode, r), http.StatusSeeOther)
		return
	}
	target := s.legacyArticleDetailTarget(r.Context(), mode, articleID, legacySearchResultPath(mode, r))
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleTimelySearchData(w http.ResponseWriter, r *http.Request, user any) {
	started := time.Now()
	filter, _ := legacySearchFilterFromRequest(r, "timely")
	filter.Page = max(apiutil.IntQuery(r, "pageNoData", 1), 1)
	filter.PageSize = 30
	if tpl, ok := s.legacyTimelyTemplate(r.Context(), r); ok && strings.TrimSpace(filter.SourceType) == "" {
		filter.SourceType = tpl.SourceType
	}
	query := url.Values{}
	query.Set("page", strconv.Itoa(filter.Page))
	query.Set("page_size", strconv.Itoa(filter.PageSize))
	if filter.Keyword != "" {
		query.Set("q", filter.Keyword)
	}
	if filter.SourceType != "" {
		query.Set("source_type", filter.SourceType)
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
	returnPath := legacySearchResultPath("timely", r)
	for _, item := range result.Items {
		detailTarget := s.legacySpecialDetailTarget("timely", item, returnPath)
		items = append(items, map[string]any{
			"title":        item.Title,
			"abstract":     nonEmpty(item.Summary, item.Content),
			"url":          detailTarget,
			"publish_time": nonEmpty(item.PublishTimeText, item.PublishTime),
			"source":       nonEmpty(item.FromText, item.SourceType),
			"videojson":    "",
			"author":       item.FromText,
			"detailUrl":    detailTarget,
		})
	}
	body, _ := json.Marshal(items)
	writePlainJSONText(w, map[string]any{
		"time": time.Since(started).Milliseconds(),
		"data": string(body),
	})
}

func (s *Server) handleTimelySearchPage(w http.ResponseWriter, r *http.Request) {
	templates := []model.CrawlTemplate{}
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/crawl-templates", &templates)
	selectedID := legacyTimelyTemplateID(r)
	selectedTemplate, hasSelectedTemplate := s.legacyTimelyTemplate(r.Context(), r)
	keyword := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("keyword"), r.URL.Query().Get("searchword"), r.URL.Query().Get("searchWord")))
	stype := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("stype"), selectedTemplate.SourceType))
	pageNo := max(apiutil.IntQuery(r, "pageNoData", 1), 1)

	query := url.Values{}
	query.Set("page", strconv.Itoa(pageNo))
	query.Set("page_size", "20")
	if keyword != "" {
		query.Set("q", keyword)
	}
	if hasSelectedTemplate && strings.TrimSpace(selectedTemplate.SourceType) != "" {
		query.Set("source_type", selectedTemplate.SourceType)
	} else if sourceType := strings.TrimSpace(r.URL.Query().Get("source_type")); sourceType != "" {
		query.Set("source_type", sourceType)
	}
	var result model.SearchResult
	_ = s.getJSON(s.cfg.ContentURL+"/api/v1/search/timely?"+query.Encode(), &result)

	runs := []model.CrawlRun{}
	_ = s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=10", &runs)
	visibleRuns := make([]model.CrawlRun, 0, len(runs))
	for _, run := range runs {
		if hasSelectedTemplate && selectedTemplate.ID > 0 {
			if run.TemplateID == selectedTemplate.ID {
				visibleRuns = append(visibleRuns, run)
			}
			continue
		}
		if query.Get("source_type") != "" && run.SourceType == query.Get("source_type") {
			visibleRuns = append(visibleRuns, run)
			continue
		}
		if query.Get("source_type") == "" {
			visibleRuns = append(visibleRuns, run)
		}
	}
	if len(visibleRuns) > 5 {
		visibleRuns = visibleRuns[:5]
	}

	var body strings.Builder
	body.WriteString(`<section><div class="summary-grid">`)
	body.WriteString(`<div class="summary-card"><div class="template-meta">当前关键词</div><strong>`)
	body.WriteString(html.EscapeString(nonEmpty(keyword, "未设置")))
	body.WriteString(`</strong></div>`)
	body.WriteString(`<div class="summary-card"><div class="template-meta">模板来源</div><strong>`)
	if hasSelectedTemplate {
		body.WriteString(html.EscapeString(selectedTemplate.Name))
		body.WriteString(`</strong><div class="template-meta">`)
		body.WriteString(html.EscapeString(selectedTemplate.SourceType))
		body.WriteString(`</div></div>`)
	} else {
		body.WriteString(`全部模板</strong></div>`)
	}
	body.WriteString(`<div class="summary-card"><div class="template-meta">结果页码</div><strong>`)
	body.WriteString(strconv.Itoa(pageNo))
	body.WriteString(`</strong></div>`)
	body.WriteString(`<div class="summary-card"><div class="template-meta">当前结果数</div><strong>`)
	body.WriteString(strconv.Itoa(len(result.Items)))
	body.WriteString(`</strong></div></div></section>`)

	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		body.WriteString(`<section><p style="padding:12px;border-radius:10px;background:#e7f4ea;color:#214e34">`)
		body.WriteString(html.EscapeString(msg))
		body.WriteString(`</p></section>`)
	}

	body.WriteString(`<section><h2>即时搜索</h2><form method="get" action="/timelysearch/result">`)
	body.WriteString(`<label>关键词</label><input name="keyword" value="`)
	body.WriteString(html.EscapeString(keyword))
	body.WriteString(`" placeholder="输入检索关键词">`)
	body.WriteString(`<label>来源类型</label><input name="stype" value="`)
	body.WriteString(html.EscapeString(stype))
	body.WriteString(`" placeholder="如 flash、headline、crypto">`)
	body.WriteString(`<label>模板</label><select name="website_id"><option value="">全部模板</option>`)
	for _, tpl := range templates {
		if !tpl.Enabled {
			continue
		}
		if stype != "" && stype != "0" && !legacyTemplateMatchesType(stype, tpl.SourceType, tpl.Name, tpl.ConfigJSON) {
			continue
		}
		body.WriteString(`<option value="`)
		body.WriteString(strconv.FormatInt(tpl.ID, 10))
		body.WriteString(`"`)
		if selectedID != "" && selectedID == strconv.FormatInt(tpl.ID, 10) {
			body.WriteString(` selected`)
		}
		body.WriteString(`>`)
		body.WriteString(html.EscapeString(tpl.Name + " [" + tpl.SourceType + "]"))
		body.WriteString(`</option>`)
	}
	body.WriteString(`</select><label>数据页码</label><input name="pageNoData" value="`)
	body.WriteString(strconv.Itoa(pageNo))
	body.WriteString(`" placeholder="1">`)
	body.WriteString(`<button type="submit">查看结果</button></form></section>`)

	body.WriteString(`<section><h2>模板执行</h2><form method="post" action="/timelysearch/execute">`)
	body.WriteString(`<input type="hidden" name="keyword" value="`)
	body.WriteString(html.EscapeString(keyword))
	body.WriteString(`"><input type="hidden" name="stype" value="`)
	body.WriteString(html.EscapeString(stype))
	body.WriteString(`"><input type="hidden" name="pageNoData" value="`)
	body.WriteString(strconv.Itoa(pageNo))
	body.WriteString(`"><label>执行模板</label><select name="website_id"><option value="">请选择模板</option>`)
	for _, tpl := range templates {
		if !tpl.Enabled {
			continue
		}
		if stype != "" && stype != "0" && !legacyTemplateMatchesType(stype, tpl.SourceType, tpl.Name, tpl.ConfigJSON) {
			continue
		}
		body.WriteString(`<option value="`)
		body.WriteString(strconv.FormatInt(tpl.ID, 10))
		body.WriteString(`"`)
		if selectedID != "" && selectedID == strconv.FormatInt(tpl.ID, 10) {
			body.WriteString(` selected`)
		}
		body.WriteString(`>`)
		body.WriteString(html.EscapeString(tpl.Name + " [" + tpl.SourceType + "]"))
		body.WriteString(`</option>`)
	}
	body.WriteString(`</select><button type="submit">立即抓取</button><p class="template-meta">执行后会回到当前结果页，并展示最近抓取记录。</p></form><p><a class="inline" href="/crawl-templates/manage">打开模板管理</a></p></section>`)

	body.WriteString(`<section><h2>最近抓取状态</h2><table><tr><th>模板</th><th>来源</th><th>状态</th><th>抓取</th><th>入库</th><th>开始时间</th></tr>`)
	if len(visibleRuns) == 0 {
		body.WriteString(`<tr><td colspan="6">暂无抓取记录</td></tr>`)
	} else {
		for _, run := range visibleRuns {
			body.WriteString(`<tr><td>`)
			body.WriteString(html.EscapeString(nonEmpty(run.TemplateName, strconv.FormatInt(run.TemplateID, 10), "手动抓取")))
			body.WriteString(`</td><td>`)
			body.WriteString(html.EscapeString(run.SourceType))
			body.WriteString(`</td><td>`)
			body.WriteString(html.EscapeString(run.Status))
			body.WriteString(`</td><td>`)
			body.WriteString(strconv.Itoa(run.FetchedCount))
			body.WriteString(`</td><td>`)
			body.WriteString(strconv.Itoa(run.InsertedCount))
			body.WriteString(`</td><td>`)
			body.WriteString(html.EscapeString(run.StartedAt.Format("2006-01-02 15:04")))
			body.WriteString(`</td></tr>`)
		}
	}
	body.WriteString(`</table></section>`)

	body.WriteString(`<section><h2>搜索结果</h2><table><tr><th>标题</th><th>来源</th><th>时间</th><th>跳转</th></tr>`)
	if len(result.Items) == 0 {
		body.WriteString(`<tr><td colspan="4">暂无匹配结果</td></tr>`)
	} else {
		for _, item := range result.Items {
			detailTarget := s.legacySpecialDetailTarget("timely", item, legacySearchResultPath("timely", r))
			body.WriteString(`<tr><td>`)
			body.WriteString(html.EscapeString(item.Title))
			body.WriteString(`</td><td>`)
			body.WriteString(html.EscapeString(nonEmpty(item.FromText, item.SourceType)))
			body.WriteString(`</td><td>`)
			body.WriteString(html.EscapeString(nonEmpty(item.PublishTimeText, item.PublishTime)))
			body.WriteString(`</td><td>`)
			body.WriteString(`<a class="inline" href="`)
			body.WriteString(html.EscapeString(detailTarget))
			body.WriteString(`">查看详情</a></td></tr>`)
		}
	}
	body.WriteString(`</table></section>`)

	body.WriteString(`<section><h2>兼容接口</h2><p><a class="inline" href="/timelysearch/data?`)
	body.WriteString(legacySearchResultPath("timely", r)[len("/timelysearch/result?"):])
	body.WriteString(`">查看 data 接口结果</a></p></section>`)

	_ = s.writeSimplePage(w, "timelysearch/result", "即时搜索", body.String())
}

func (s *Server) handleLegacySearchArticleDetailData(w http.ResponseWriter, r *http.Request, user any) {
	articleID := strings.TrimSpace(firstNonEmpty(r.FormValue("articleId"), r.FormValue("articleid"), r.URL.Query().Get("articleId"), r.URL.Query().Get("articleid")))
	if articleID == "" {
		writeJSONText(w, map[string]any{"detail": "", "title": "", "text": ""})
		return
	}
	item, err := s.fetchLegacyItemByID(articleID, userIDFromMap(user))
	if err != nil {
		writeJSONText(w, map[string]any{"detail": "", "title": "", "text": ""})
		return
	}
	writeJSONText(w, legacySearchArticleDetailPayload(item))
}

func (s *Server) handleLegacySearchRelatedArticles(w http.ResponseWriter, r *http.Request, user any) {
	articleID := strings.TrimSpace(firstNonEmpty(r.FormValue("articleId"), r.FormValue("articleid"), r.URL.Query().Get("articleId"), r.URL.Query().Get("articleid")))
	userID := userIDFromMap(user)
	var related []model.Item
	if articleID != "" {
		target := s.cfg.ContentURL + "/api/v1/articles/" + url.PathEscape(articleID) + "/related"
		if userID > 0 {
			target += "?user_id=" + strconv.FormatInt(userID, 10)
		}
		_ = s.getJSON(target, &related)
	}
	if len(related) == 0 {
		keyword := strings.TrimSpace(firstNonEmpty(r.FormValue("keywords"), r.FormValue("keyword"), r.URL.Query().Get("keywords"), r.URL.Query().Get("keyword")))
		if keyword != "" {
			var result model.SearchResult
			query := url.Values{}
			query.Set("page", "1")
			query.Set("page_size", "6")
			query.Set("q", keyword)
			if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/timely?"+query.Encode(), &result); err == nil {
				related = result.Items
			}
		}
	}
	payload := make([]map[string]any, 0, len(related))
	for _, item := range related {
		payload = append(payload, map[string]any{
			"article_public_id": strconv.FormatInt(item.ID, 10),
			"title":             item.Title,
			"content":           nonEmpty(item.Summary, item.Content),
			"sourceName":        nonEmpty(item.FromText, item.SourceType, item.ExternalSourceHost),
			"publishTime":       nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
		})
	}
	writeJSONText(w, payload)
}

func (s *Server) handleTimelySearchExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/timelysearch/result", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	templateID := strings.TrimSpace(firstNonEmpty(r.FormValue("website_id"), r.FormValue("template_id")))
	keyword := strings.TrimSpace(firstNonEmpty(r.FormValue("keyword"), r.URL.Query().Get("keyword")))
	stype := strings.TrimSpace(firstNonEmpty(r.FormValue("stype"), r.URL.Query().Get("stype")))
	pageNoData := strings.TrimSpace(firstNonEmpty(r.FormValue("pageNoData"), r.URL.Query().Get("pageNoData"), "1"))
	targetQuery := url.Values{}
	if keyword != "" {
		targetQuery.Set("keyword", keyword)
	}
	if stype != "" {
		targetQuery.Set("stype", stype)
	}
	if pageNoData != "" {
		targetQuery.Set("pageNoData", pageNoData)
	}
	if templateID != "" {
		targetQuery.Set("website_id", templateID)
	}
	message := "模板抓取已触发"
	req := s.client.R()
	if !applyManualCrawlTemplate(req, templateID) {
		targetQuery.Set("msg", "模板编号无效")
		http.Redirect(w, r, "/timelysearch/result?"+targetQuery.Encode(), http.StatusSeeOther)
		return
	}
	if keyword != "" {
		req.SetQueryParam("keyword", keyword)
	}
	resp, err := req.Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
	if err != nil || !resp.IsSuccess() {
		message = "模板抓取触发失败"
	}
	targetQuery.Set("msg", message)
	http.Redirect(w, r, "/timelysearch/result?"+targetQuery.Encode(), http.StatusSeeOther)
}

func (s *Server) legacyTimelyTemplate(ctx context.Context, r *http.Request) (model.CrawlTemplate, bool) {
	templateID := legacyTimelyTemplateID(r)
	if templateID == "" {
		return model.CrawlTemplate{}, false
	}
	tpl, err := s.fetchCrawlTemplate(ctx, templateID)
	if err != nil {
		return model.CrawlTemplate{}, false
	}
	return tpl, true
}

func legacyTimelyTemplateID(r *http.Request) string {
	return strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("website_id"), r.URL.Query().Get("template_id"), r.FormValue("website_id"), r.FormValue("template_id")))
}

func (s *Server) handleTimelySearchTemplate(w http.ResponseWriter, r *http.Request, user any) {
	_ = user
	templates := []model.CrawlTemplate{}
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/crawl-templates", &templates); err != nil {
		writeJSONText(w, []map[string]any{{"id": 0, "engine": "全部"}})
		return
	}
	stype := strings.TrimSpace(r.URL.Query().Get("stype"))
	result := []map[string]any{{"id": 0, "engine": "全部", "source_type": stype}}
	for _, tpl := range templates {
		if !tpl.Enabled {
			continue
		}
		if stype != "" && stype != "0" && !legacyTemplateMatchesType(stype, tpl.SourceType, tpl.Name, tpl.ConfigJSON) {
			continue
		}
		result = append(result, map[string]any{
			"id":          tpl.ID,
			"engine":      tpl.Name,
			"source_type": tpl.SourceType,
		})
	}
	writeJSONText(w, result)
}

func (s *Server) handleLegacySpecialList(w http.ResponseWriter, r *http.Request, _ any, kind string) {
	filter, pageSize := legacySearchFilterFromRequest(r, "full")
	if page := apiutil.IntQuery(r, "pageNum", 0); page > 0 {
		filter.Page = page
	}
	if size := apiutil.IntQuery(r, "pageSize", 0); size > 0 {
		filter.PageSize = size
		pageSize = size
	}
	filter.Keyword = nonEmpty(
		strings.TrimSpace(r.URL.Query().Get("searchWord")),
		strings.TrimSpace(r.URL.Query().Get("searchword")),
		strings.TrimSpace(r.URL.Query().Get("keyword")),
		filter.Keyword,
	)
	items, err := s.fetchLegacyCompatSearchItems(filter, "full", 200)
	if err != nil {
		writeJSONText(w, map[string]any{"code": "500", "msg": err.Error(), "list": []map[string]any{}})
		return
	}
	criteria := legacySpecialCriteriaFromRequest(r)
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		if legacySpecialMatchesItem(kind, item, criteria) {
			filtered = append(filtered, item)
		}
	}
	page := max(filter.Page, 1)
	size := max(filter.PageSize, pageSize)
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	list := make([]map[string]any, 0, end-start)
	for _, item := range filtered[start:end] {
		list = append(list, legacySpecialListEntry(kind, item))
	}
	totalPages := 1
	if size > 0 && len(filtered) > 0 {
		totalPages = (len(filtered) + size - 1) / size
	}
	writeJSONText(w, map[string]any{
		"code":        "200",
		"msg":         "success",
		"list":        list,
		"totalData":   len(filtered),
		"totalPage":   totalPages,
		"currentPage": page,
	})
}

func (s *Server) handleLegacySpecialDetailData(w http.ResponseWriter, r *http.Request, path string) {
	kind := map[string]string{
		"lawyerDetailData":          "lawyer",
		"executionPersonDetailData": "executionPerson",
		"professorDetailData":       "professor",
		"doctorDetailData":          "doctor",
	}[path]
	itemID := strings.TrimSpace(firstNonEmpty(r.FormValue("article_public_id"), r.URL.Query().Get("article_public_id"), r.FormValue("articleid"), r.URL.Query().Get("articleid")))
	if itemID == "" {
		writeJSONText(w, map[string]any{"list": []map[string]any{}})
		return
	}
	item, err := s.fetchLegacyItemByID(itemID, 0)
	if err != nil {
		writeJSONText(w, map[string]any{"list": []map[string]any{}})
		return
	}
	writeJSONText(w, map[string]any{"list": []map[string]any{legacySpecialDetailEntry(kind, item)}})
}

func (s *Server) handleLegacyCompanyDetailData(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimSpace(firstNonEmpty(r.FormValue("article_public_id"), r.URL.Query().Get("article_public_id"), r.FormValue("articleid"), r.URL.Query().Get("articleid")))
	if itemID == "" {
		writeJSONText(w, map[string]any{})
		return
	}
	item, err := s.fetchLegacyItemByID(itemID, 0)
	if err != nil {
		writeJSONText(w, map[string]any{})
		return
	}
	writeJSONText(w, legacySpecialDetailEntry("company", item))
}

func (s *Server) handleLegacyReportDetailData(w http.ResponseWriter, r *http.Request) {
	itemID := strings.TrimSpace(firstNonEmpty(r.FormValue("article_public_id"), r.URL.Query().Get("article_public_id"), r.FormValue("articleid"), r.URL.Query().Get("articleid")))
	if itemID == "" {
		writeJSONText(w, map[string]any{})
		return
	}
	item, err := s.fetchLegacyItemByID(itemID, 0)
	if err != nil {
		writeJSONText(w, map[string]any{})
		return
	}
	writeJSONText(w, legacySpecialDetailEntry("report", item))
}

func (s *Server) handleLegacySpecialCategoryOptions(w http.ResponseWriter, r *http.Request, kind string) {
	filter, _ := legacySearchFilterFromRequest(r, "full")
	filter.Keyword = nonEmpty(
		strings.TrimSpace(r.URL.Query().Get("searchWord")),
		strings.TrimSpace(r.URL.Query().Get("searchword")),
		strings.TrimSpace(r.URL.Query().Get("keyword")),
		filter.Keyword,
	)
	items, err := s.fetchLegacyCompatSearchItems(filter, "full", 200)
	if err != nil {
		writeJSONText(w, legacySearchCategoryOptions(r, kind))
		return
	}
	options := legacyDynamicCategoryOptions(kind, items)
	if len(options) == 0 {
		options = legacySearchCategoryOptions(r, kind)
	}
	writeJSONText(w, options)
}

func (s *Server) handleLegacySpecialDetailPage(w http.ResponseWriter, r *http.Request, mode string, kind string, rawID string) {
	itemID := legacySpecialDetailID(rawID)
	returnPath := strings.TrimSpace(r.URL.Query().Get("return_to"))
	if returnPath == "" {
		returnPath = legacySearchResultPath(mode, r)
	}
	if itemID == "" {
		http.Redirect(w, r, s.legacySearchTarget(mode, r), http.StatusSeeOther)
		return
	}
	item, err := s.fetchLegacyItemByID(itemID, 0)
	if err != nil {
		http.Redirect(w, r, "/articles/"+url.PathEscape(itemID)+"?return_to="+url.QueryEscape(returnPath), http.StatusSeeOther)
		return
	}
	detail := legacySpecialDetailEntry(kind, item)
	body := legacySpecialDetailPageBody(kind, detail, mode, returnPath)
	_ = s.writeSimplePage(w, "fullsearch/"+kind, legacySpecialTitle(kind), body)
}

func (s *Server) fetchLegacyCompatSearchItems(filter model.ArticleFilter, mode string, limit int) ([]model.Item, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("page_size", strconv.Itoa(max(limit, 50)))
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
	path := "/api/v1/search/full"
	if mode == "timely" {
		path = "/api/v1/search/timely"
	}
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+path+"?"+query.Encode(), &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (s *Server) fetchLegacyItemByID(itemID string, userID int64) (model.Item, error) {
	target := s.cfg.ContentURL + "/api/v1/articles/" + url.PathEscape(strings.TrimSpace(itemID))
	if userID > 0 {
		target += "?user_id=" + strconv.FormatInt(userID, 10)
	}
	var item model.Item
	if err := s.getJSON(target, &item); err != nil {
		return model.Item{}, err
	}
	return item, nil
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

func legacyComplaintSource(item model.Item) map[string]any {
	return map[string]any{
		"letter_content": nonEmpty(item.Content, item.Summary, item.Title),
		"reply_content":  nonEmpty(item.Summary, item.Content),
		"writer":         nonEmpty(item.FromText, "匿名"),
		"release_time":   nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
		"detailUrl":      nonEmpty(item.SourceURL, item.DetailURL, "/articles/"+strconv.FormatInt(item.ID, 10)),
		"reply_source":   nonEmpty(item.SourceType, "Go 兼容版"),
		"reply_time":     nonEmpty(item.PublishTime, item.PublishTimeText),
		"sourceName":     nonEmpty(item.FromText, item.SourceType, "来源"),
		"content":        nonEmpty(item.Content, item.Summary, item.Title),
		"process":        "[]",
		"problem":        item.Title,
		"detail":         nonEmpty(item.Content, item.Summary),
		"object":         nonEmpty(item.FromText, item.SourceType),
		"money":          "",
		"appeal":         "",
		"progress":       "已迁移",
		"sourceUrl":      nonEmpty(item.SourceURL, item.DetailURL),
	}
}

func (s *Server) legacyPublicationEntries(r *http.Request, filter model.ArticleFilter, pageSize int) ([]map[string]any, int, int, int) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(max(filter.Page, 1)))
	query.Set("page_size", strconv.Itoa(max(filter.PageSize, pageSize)))
	if filter.Keyword != "" {
		query.Set("q", filter.Keyword)
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
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
		return []map[string]any{}, 0, filter.Page, max(filter.PageSize, pageSize)
	}
	entries := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		entries = append(entries, legacyAnnouncementSource(item))
	}
	page := max(result.Page, filter.Page)
	size := max(result.PageSize, max(filter.PageSize, pageSize))
	return entries, result.Total, page, size
}

func (s *Server) legacyReportEntries(r *http.Request, filter model.ArticleFilter, pageSize int) ([]map[string]any, int, int, int) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(max(filter.Page, 1)))
	query.Set("page_size", strconv.Itoa(max(filter.PageSize, pageSize)))
	if filter.Keyword != "" {
		query.Set("q", filter.Keyword)
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
	var result model.SearchResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/search/full?"+query.Encode(), &result); err != nil {
		return []map[string]any{}, 0, filter.Page, max(filter.PageSize, pageSize)
	}
	entries := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		entries = append(entries, legacyReportSource(item))
	}
	page := max(result.Page, filter.Page)
	size := max(result.PageSize, max(filter.PageSize, pageSize))
	return entries, result.Total, page, size
}

func legacyAnnouncementSource(item model.Item) map[string]any {
	return map[string]any{
		"article_public_id": strconv.FormatInt(item.ID, 10),
		"codename":          nonEmpty(item.FromText, item.SourceType, "来源"),
		"title":             item.Title,
		"rtype":             nonEmpty(item.SourceType, "公告"),
		"reportDate":        nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
	}
}

func legacyReportSource(item model.Item) map[string]any {
	code := strconv.FormatInt(item.ID, 10)
	authors := []map[string]any{}
	return map[string]any{
		"article_public_id": strconv.FormatInt(item.ID, 10),
		"codename":          nonEmpty(item.FromText, item.SourceType, "机构"),
		"title":             item.Title,
		"code":              code,
		"authorList":        legacyJSONString(authors),
		"reportDate":        nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
	}
}

func legacySearchCategoryOptions(r *http.Request, kind string) []map[string]any {
	_ = r
	switch kind {
	case "announcement":
		return []map[string]any{
			{"value": "", "name": "全部"},
			{"value": "公告", "name": "公告"},
			{"value": "新闻", "name": "新闻"},
		}
	case "report":
		return []map[string]any{
			{"value": "", "name": "全部"},
			{"value": "研报", "name": "研报"},
			{"value": "公告", "name": "公告"},
		}
	default:
		return []map[string]any{{"value": "", "name": "全部"}}
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
	switch typeOneID {
	case 28:
		return legacyTypeChildren(typeOneID, "公告类型", []string{"公告", "新闻"})
	case 35:
		return legacyTypeChildren(typeOneID, "研报行业", []string{"研报", "公告"})
	case 39:
		return legacyTypeChildren(typeOneID, "工商分类", []string{"企业信息", "股东信息", "变更记录"})
	case 40:
		return legacyTypeChildren(typeOneID, "投融资分类", []string{"投融资", "融资轮次", "机构动态"})
	case 42:
		return legacyTypeChildren(typeOneID, "法律文书分类", []string{"裁判文书", "执行公告", "开庭公告"})
	case 43:
		return legacyTypeChildren(typeOneID, "知识产权分类", []string{"专利", "商标", "著作权"})
	case 45:
		return legacyTypeChildren(typeOneID, "学术分类", []string{"学术论文", "学位论文", "研究成果"})
	case 100:
		return legacyTypeChildren(typeOneID, "律师筛选", []string{"姓名", "律所名称", "擅长领域", "城市"})
	case 101:
		return legacyTypeChildren(typeOneID, "被执行人筛选", []string{"地区", "名称", "企业", "个人"})
	case 102:
		return legacyTypeChildren(typeOneID, "专家人才筛选", []string{"姓名", "研究领域", "机构"})
	case 103:
		return legacyTypeChildren(typeOneID, "医生筛选", []string{"姓名", "医院", "擅长领域", "所属科室"})
	default:
		return []legacySearchFullType{}
	}
}

func legacySearchTypesByThird(typeTwoID int) []legacySearchFullType {
	if typeTwoID <= 0 {
		return []legacySearchFullType{}
	}
	switch typeTwoID / 100 {
	case 28:
		return legacyTypeChildren(typeTwoID, "公告来源", []string{"全部", "公告", "新闻"})
	case 35:
		return legacyTypeChildren(typeTwoID, "研报来源", []string{"全部", "研报", "公告"})
	case 39:
		return legacyTypeChildren(typeTwoID, "工商来源", []string{"全部", "天眼查", "企查查", "企业公示"})
	case 40:
		return legacyTypeChildren(typeTwoID, "投融资来源", []string{"全部", "机构", "企业", "项目"})
	case 42:
		return legacyTypeChildren(typeTwoID, "法律文书来源", []string{"全部", "法院", "执行", "公告"})
	case 43:
		return legacyTypeChildren(typeTwoID, "知识产权来源", []string{"全部", "专利", "商标", "著作权"})
	case 45:
		return legacyTypeChildren(typeTwoID, "学术来源", []string{"全部", "期刊", "学位", "论文"})
	case 100:
		return legacyTypeChildren(typeTwoID, "律师来源", []string{"全部", "专职律师", "合伙人", "顾问"})
	case 101:
		return legacyTypeChildren(typeTwoID, "被执行人来源", []string{"全部", "企业", "个人"})
	case 102:
		return legacyTypeChildren(typeTwoID, "专家人才来源", []string{"全部", "高校", "研究院", "企业"})
	case 103:
		return legacyTypeChildren(typeTwoID, "医生来源", []string{"全部", "三甲医院", "专科医院", "社区医院"})
	default:
		return []legacySearchFullType{}
	}
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

func legacySearchArticleFromItem(item model.Item, keyword string, detailTarget string) legacySearchArticle {
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
		Url:               nonEmpty(detailTarget, item.SourceURL, item.DetailURL, "/articles/"+strconv.FormatInt(item.ID, 10)),
		Abstract:          nonEmpty(item.Summary, item.Content),
		PublishTimeText:   nonEmpty(item.PublishTimeText, item.PublishTime),
		VideoJSON:         "",
	}
}

func legacySearchArticleDetailPayload(item model.Item) map[string]any {
	payload := legacyPayloadMap(item)
	keywords := legacyJSONTextPayload(payload, "key_words")
	if keywords == "" {
		labels := splitLegacyLabels(strings.TrimSpace(item.TagFlags))
		if len(labels) > 0 {
			keywordMap := make(map[string]int, len(labels))
			for _, label := range labels {
				keywordMap[label] = 1
			}
			keywords = legacyJSONString(keywordMap)
		}
	}
	ner := legacyJSONTextPayload(payload, "ner")
	if ner == "" {
		ner = "{}"
	}
	policy := legacyJSONTextPayload(payload, "policylable")
	detail := map[string]any{
		"article_public_id": strconv.FormatInt(item.ID, 10),
		"author":            nonEmpty(legacyPayloadString(payload, "author"), item.FromText, item.ExternalSourceHost),
		"sourcewebsitename": nonEmpty(legacyPayloadString(payload, "sourcewebsitename"), legacyPayloadString(payload, "source_name"), item.FromText, item.SourceType),
		"publish_time":      nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
		"industrylable":     nonEmpty(legacyPayloadString(payload, "industrylable"), legacyPayloadString(payload, "industry")),
		"eventlable":        nonEmpty(legacyPayloadString(payload, "eventlable"), strings.TrimSpace(item.TagFlags)),
		"source_url":        nonEmpty(item.SourceURL, item.DetailURL),
		"extend_string_one": legacyJSONTextPayload(payload, "extend_string_one"),
		"key_words":         keywords,
		"ner":               ner,
		"policylable":       policy,
	}
	return map[string]any{
		"title":       item.Title,
		"text":        nonEmpty(item.Content, item.Summary),
		"emotionText": legacySearchEmotionText(item),
		"emotionChart": [][]any{
			{"正面", 0},
			{"中性", 1},
			{"负面", 0},
		},
		"detail": detail,
	}
}

func legacySearchEmotionText(item model.Item) string {
	text := strings.ToLower(strings.TrimSpace(item.Title + " " + item.Summary + " " + item.Content))
	switch legacyHotSentiment(text) {
	case "positive":
		return "正面"
	case "negative":
		return "负面"
	default:
		return "中性"
	}
}

type legacySpecialCriteria struct {
	Keyword      string
	MatchingMode string
	KindFilter   string
	SourceName   string
	RType        string
}

func legacySpecialCriteriaFromRequest(r *http.Request) legacySpecialCriteria {
	return legacySpecialCriteria{
		Keyword:      nonEmpty(strings.TrimSpace(r.URL.Query().Get("searchWord")), strings.TrimSpace(r.URL.Query().Get("searchword")), strings.TrimSpace(r.URL.Query().Get("keyword"))),
		MatchingMode: strings.TrimSpace(r.URL.Query().Get("matchingmode")),
		KindFilter:   strings.TrimSpace(r.URL.Query().Get("kinds")),
		SourceName:   strings.TrimSpace(r.URL.Query().Get("source_name")),
		RType:        strings.TrimSpace(r.URL.Query().Get("rtype")),
	}
}

func legacySpecialMatchesItem(kind string, item model.Item, criteria legacySpecialCriteria) bool {
	payload := legacyPayloadMap(item)
	if criteria.KindFilter != "" && !legacyPayloadContains(payload, criteria.KindFilter) && !legacyItemBlobContains(item, criteria.KindFilter) {
		return false
	}
	if criteria.SourceName != "" && criteria.SourceName != "全部" && criteria.SourceName != item.SourceType && criteria.SourceName != item.FromText {
		if !legacyPayloadContains(payload, criteria.SourceName) && !legacyItemBlobContains(item, criteria.SourceName) {
			return false
		}
	}
	if criteria.RType != "" && criteria.RType != "全部" && !legacyPayloadContains(payload, criteria.RType) && !legacyItemBlobContains(item, criteria.RType) {
		return false
	}
	if criteria.Keyword == "" {
		return true
	}
	return legacySpecialKeywordMatch(kind, item, payload, criteria.Keyword, criteria.MatchingMode)
}

func legacySpecialKeywordMatch(kind string, item model.Item, payload map[string]any, keyword string, matchingMode string) bool {
	fields := legacySpecialSearchFields(kind, matchingMode)
	if len(fields) == 0 {
		return legacyItemBlobContains(item, keyword) || legacyPayloadContains(payload, keyword)
	}
	for _, field := range fields {
		if legacyPayloadFieldContains(payload, field, keyword) {
			return true
		}
	}
	return false
}

func legacySpecialSearchFields(kind string, matchingMode string) []string {
	switch kind {
	case "lawyerList":
		switch matchingMode {
		case "lawpace":
			return []string{"lawfirm"}
		case "lawyerAdept":
			return []string{"goods", "adept"}
		case "lawyerCity":
			return []string{"city"}
		default:
			return []string{"name", "title"}
		}
	case "executionPersonList":
		switch matchingMode {
		case "executionPersonArea":
			return []string{"areaNameNew", "province", "city"}
		default:
			return []string{"iname", "name", "title"}
		}
	case "professorList":
		switch matchingMode {
		case "professorAdept":
			return []string{"field", "research_field"}
		case "organization":
			return []string{"institution", "source_name"}
		default:
			return []string{"title", "name"}
		}
	case "doctorList":
		switch matchingMode {
		case "hospital":
			return []string{"hospital"}
		case "doctorAdept":
			return []string{"adept"}
		case "doctorDept":
			return []string{"department"}
		default:
			return []string{"name", "title"}
		}
	case "judgmentList":
		switch matchingMode {
		case "parties":
			return []string{"parties"}
		case "court":
			return []string{"court"}
		case "text":
			return []string{"content", "summary"}
		case "area":
			return []string{"province", "city", "area"}
		default:
			return []string{"title", "name"}
		}
	default:
		return nil
	}
}

func legacySpecialListEntry(kind string, item model.Item) map[string]any {
	payload := legacyPayloadMap(item)
	base := map[string]any{
		"article_public_id": strconv.FormatInt(item.ID, 10),
		"title":             nonEmpty(legacyPayloadString(payload, "title"), item.Title),
		"content":           nonEmpty(legacyPayloadString(payload, "content"), item.Content, item.Summary),
		"source_name":       nonEmpty(legacyPayloadString(payload, "source_name"), item.FromText, item.SourceType),
		"publish_time":      nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05")),
		"detailUrl":         nonEmpty(item.SourceURL, item.DetailURL),
		"url":               nonEmpty(item.SourceURL, item.DetailURL),
	}
	switch kind {
	case "lawyerList":
		base["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		base["telephone"] = nonEmpty(legacyPayloadString(payload, "telephone"), legacyPayloadString(payload, "phone_number"))
		base["kinds"] = legacyPayloadString(payload, "kinds")
		base["goods"] = nonEmpty(legacyPayloadString(payload, "goods"), legacyPayloadString(payload, "adept"))
		base["educationbackground"] = legacyPayloadString(payload, "educationbackground")
		base["email"] = legacyPayloadString(payload, "email")
		base["certID"] = legacyPayloadString(payload, "certID")
		base["qualifitime"] = legacyPayloadString(payload, "qualifitime")
		base["lawfirm"] = legacyPayloadString(payload, "lawfirm")
		base["address"] = legacyPayloadString(payload, "address")
		base["city"] = legacyPayloadString(payload, "city")
	case "executionPersonList":
		base["iname"] = nonEmpty(legacyPayloadString(payload, "iname"), legacyPayloadString(payload, "name"), item.Title)
		base["gistUnit"] = legacyPayloadString(payload, "gistUnit")
		base["cardNum"] = legacyPayloadString(payload, "cardNum")
		base["type"] = legacyPayloadString(payload, "type")
		base["caseCode"] = legacyPayloadString(payload, "caseCode")
		base["gistId"] = legacyPayloadString(payload, "gistId")
		base["areaNameNew"] = legacyPayloadString(payload, "areaNameNew")
		base["courtName"] = legacyPayloadString(payload, "courtName")
		base["duty"] = legacyPayloadString(payload, "duty")
		base["performance"] = legacyPayloadString(payload, "performance")
		base["disruptTypeName"] = legacyPayloadString(payload, "disruptTypeName")
	case "professorList":
		base["title"] = nonEmpty(legacyPayloadString(payload, "title"), legacyPayloadString(payload, "name"), item.Title)
		base["avatar"] = nonEmpty(legacyPayloadString(payload, "avatar"), legacyPayloadString(payload, "profile"))
		base["institution"] = legacyPayloadString(payload, "institution")
		base["field"] = legacyPayloadJSONArrayString(payload, "field")
		base["works"] = legacyPayloadString(payload, "works")
		base["times_cited"] = legacyPayloadString(payload, "times_cited")
	case "doctorList":
		base["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		base["profile"] = nonEmpty(legacyPayloadString(payload, "profile"), legacyPayloadString(payload, "avatar"))
		base["hospital"] = legacyPayloadString(payload, "hospital")
		base["department"] = legacyPayloadString(payload, "department")
		base["province"] = legacyPayloadString(payload, "province")
		base["city"] = legacyPayloadString(payload, "city")
		base["area"] = legacyPayloadString(payload, "area")
		base["degree"] = legacyPayloadString(payload, "degree")
		base["adept"] = legacyPayloadString(payload, "adept")
	case "companyList":
		base["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		base["legal_person"] = nonEmpty(legacyPayloadString(payload, "legal_person"), legacyPayloadString(payload, "legal_representative"))
		base["status"] = legacyPayloadString(payload, "status")
		base["registered_capital_str"] = legacyPayloadString(payload, "registered_capital_str")
		base["industry_involved"] = nonEmpty(legacyPayloadString(payload, "industry_involved"), legacyPayloadString(payload, "industry"))
		base["location"] = nonEmpty(legacyPayloadString(payload, "location"), legacyPayloadString(payload, "address"))
	case "judgmentList":
		base["caseTitle"] = nonEmpty(legacyPayloadString(payload, "caseTitle"), item.Title)
		base["court"] = legacyPayloadString(payload, "court")
		base["caseType"] = legacyPayloadString(payload, "caseType")
		base["parties"] = legacyPayloadString(payload, "parties")
	case "knowLedgeList":
		base["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		base["caseType"] = nonEmpty(legacyPayloadString(payload, "caseType"), legacyPayloadString(payload, "ip_type"))
		base["owner"] = legacyPayloadString(payload, "owner")
	case "investmentList":
		base["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		base["round"] = nonEmpty(legacyPayloadString(payload, "round"), legacyPayloadString(payload, "investment_type"))
		base["company"] = legacyPayloadString(payload, "company")
	case "baiduKnowsList", "thesisnList", "biddingList", "inviteList":
		base["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
	}
	for key, value := range payload {
		if _, exists := base[key]; !exists {
			base[key] = value
		}
	}
	return base
}

func legacySpecialDetailEntry(kind string, item model.Item) map[string]any {
	entry := legacySpecialListEntry(kind, item)
	payload := legacyPayloadMap(item)
	entry["summary"] = nonEmpty(item.Summary, item.Content)
	entry["source_url"] = nonEmpty(item.SourceURL, item.DetailURL)
	entry["publish_time"] = nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05"))
	entry["detailUrl"] = nonEmpty(legacyAnyString(entry["detailUrl"]), item.SourceURL, item.DetailURL)
	entry["detail_url"] = nonEmpty(legacyPayloadString(payload, "detail_url"), legacyPayloadString(payload, "detailUrl"), legacyPayloadString(payload, "detailurl"), item.SourceURL, item.DetailURL)
	entry["detailurl"] = nonEmpty(legacyPayloadString(payload, "detailurl"), legacyAnyString(entry["detail_url"]), legacyAnyString(entry["detailUrl"]))
	entry["source_name"] = nonEmpty(legacyAnyString(entry["source_name"]), item.FromText, item.SourceType)
	entry["source"] = nonEmpty(legacyPayloadString(payload, "source"), item.SourceType)
	switch kind {
	case "company":
		entry["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		entry["phone_number"] = nonEmpty(legacyPayloadString(payload, "phone_number"), legacyPayloadString(payload, "phone"))
		entry["phone"] = nonEmpty(legacyPayloadString(payload, "phone"), legacyAnyString(entry["phone_number"]))
		entry["address"] = nonEmpty(legacyPayloadString(payload, "address"), legacyPayloadString(payload, "location"))
		entry["location"] = nonEmpty(legacyPayloadString(payload, "location"), legacyAnyString(entry["address"]))
		entry["legal_representative"] = nonEmpty(legacyPayloadString(payload, "legal_representative"), legacyPayloadString(payload, "legal_person"))
		entry["legal_person"] = nonEmpty(legacyPayloadString(payload, "legal_person"), legacyAnyString(entry["legal_representative"]))
		entry["uniformSocialCreditCode"] = nonEmpty(legacyPayloadString(payload, "uniformSocialCreditCode"), legacyPayloadString(payload, "taxpayer_identification"))
		entry["taxpayer_identification"] = nonEmpty(legacyPayloadString(payload, "taxpayer_identification"), legacyAnyString(entry["uniformSocialCreditCode"]))
		entry["insured_num"] = nonEmpty(legacyPayloadString(payload, "insured_num"), legacyPayloadString(payload, "insureds"))
		entry["insureds"] = nonEmpty(legacyPayloadString(payload, "insureds"), legacyAnyString(entry["insured_num"]))
		entry["registration"] = legacyPayloadString(payload, "registration")
		entry["enterprise_type"] = legacyPayloadString(payload, "enterprise_type")
		entry["registered_capital_str"] = legacyPayloadString(payload, "registered_capital_str")
		entry["industry_involved"] = nonEmpty(legacyPayloadString(payload, "industry_involved"), legacyPayloadString(payload, "industry"))
		entry["business_scope"] = legacyPayloadString(payload, "business_scope")
		entry["establish_time"] = nonEmpty(legacyPayloadString(payload, "establish_time"), item.PublishTime)
		entry["key_person"] = legacyJSONTextPayload(payload, "key_person")
		entry["shareholder"] = legacyJSONTextPayload(payload, "shareholder")
		entry["change_record"] = legacyJSONTextPayload(payload, "change_record")
	case "report":
		entry["title"] = item.Title
		entry["reportDate"] = nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04:05"))
		entry["url"] = nonEmpty(item.SourceURL, item.DetailURL)
	case "lawyer":
		entry["img"] = nonEmpty(legacyPayloadString(payload, "img"), legacyPayloadString(payload, "profile"), legacyPayloadString(payload, "avatar"))
		entry["telephone"] = nonEmpty(legacyPayloadString(payload, "telephone"), legacyPayloadString(payload, "phone_number"))
		entry["detailurl"] = nonEmpty(legacyPayloadString(payload, "detailurl"), legacyAnyString(entry["detail_url"]), legacyAnyString(entry["detailUrl"]))
		entry["name"] = nonEmpty(legacyPayloadString(payload, "name"), item.Title)
		entry["goods"] = nonEmpty(legacyPayloadString(payload, "goods"), legacyPayloadString(payload, "adept"))
		entry["WeChat"] = legacyPayloadString(payload, "WeChat")
		entry["microblog"] = legacyPayloadString(payload, "microblog")
		entry["tecent"] = legacyPayloadString(payload, "tecent")
		entry["status"] = legacyPayloadString(payload, "status")
		entry["language"] = legacyPayloadString(payload, "language")
		entry["sex"] = legacyPayloadString(payload, "sex")
		entry["achievements"] = legacyPayloadString(payload, "achievements")
	case "executionPerson":
		entry["photo"] = nonEmpty(legacyPayloadString(payload, "photo"), legacyPayloadString(payload, "avatar"), legacyPayloadString(payload, "img"))
		entry["detailurl"] = nonEmpty(legacyPayloadString(payload, "detailurl"), legacyAnyString(entry["detail_url"]), legacyAnyString(entry["detailUrl"]))
		entry["address"] = legacyPayloadString(payload, "address")
		entry["iname"] = nonEmpty(legacyPayloadString(payload, "iname"), legacyPayloadString(payload, "name"), item.Title)
	case "professor":
		entry["avatar"] = nonEmpty(legacyPayloadString(payload, "avatar"), legacyPayloadString(payload, "profile"), legacyPayloadString(payload, "img"))
		entry["detail_url"] = nonEmpty(legacyPayloadString(payload, "detail_url"), legacyAnyString(entry["detailUrl"]), legacyAnyString(entry["detailurl"]))
		entry["views"] = legacyPayloadString(payload, "views")
		entry["field"] = legacyPayloadJSONArrayString(payload, "field")
		entry["H_index"] = nonEmpty(legacyPayloadString(payload, "H_index"), legacyPayloadString(payload, "h_index"))
		entry["G_index"] = nonEmpty(legacyPayloadString(payload, "G_index"), legacyPayloadString(payload, "g_index"))
		entry["cooperation_agency"] = legacyJSONTextPayload(payload, "cooperation_agency")
		entry["periodical"] = legacyJSONTextPayload(payload, "periodical")
	case "doctor":
		entry["hospital_url"] = legacyPayloadString(payload, "hospital_url")
		entry["phone_number"] = nonEmpty(legacyPayloadString(payload, "phone_number"), legacyPayloadString(payload, "telephone"))
		entry["location"] = nonEmpty(legacyPayloadString(payload, "location"), strings.TrimSpace(strings.Join([]string{legacyPayloadString(payload, "province"), legacyPayloadString(payload, "city"), legacyPayloadString(payload, "area")}, " ")))
		entry["honor"] = legacyPayloadString(payload, "honor")
		entry["paper"] = legacyPayloadString(payload, "paper")
		entry["detailUrl"] = nonEmpty(legacyPayloadString(payload, "detailUrl"), legacyAnyString(entry["detail_url"]), legacyAnyString(entry["detailurl"]))
		entry["email"] = legacyPayloadString(payload, "email")
		entry["postcode"] = legacyPayloadString(payload, "postcode")
		entry["administrative_function"] = legacyPayloadString(payload, "administrative_function")
	}
	for key, value := range payload {
		if _, exists := entry[key]; !exists {
			entry[key] = value
		}
	}
	return entry
}

func legacyDynamicCategoryOptions(kind string, items []model.Item) []map[string]any {
	fieldSets := map[string][][]string{
		"company":    {{"industry_involved", "industry", "industrylable"}},
		"judgment":   {{"caseType", "case_type", "category"}},
		"knowledge":  {{"caseType", "ip_type", "type"}},
		"investment": {{"round", "investment_type", "type"}},
	}
	fields := fieldSets[kind]
	if len(fields) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := []map[string]any{{"id": 0, "value": "", "name": "全部"}}
	nextID := 1
	for _, item := range items {
		payload := legacyPayloadMap(item)
		for _, group := range fields {
			value := ""
			for _, field := range group {
				value = legacyPayloadString(payload, field)
				if value != "" {
					break
				}
			}
			for _, part := range splitLegacyLabels(value) {
				if _, ok := seen[part]; ok || part == "" {
					continue
				}
				seen[part] = struct{}{}
				result = append(result, map[string]any{"id": nextID, "value": part, "name": part})
				nextID++
			}
		}
	}
	return result
}

func legacyPayloadMap(item model.Item) map[string]any {
	payload := map[string]any{}
	if strings.TrimSpace(item.RawPayload) != "" {
		_ = json.Unmarshal([]byte(item.RawPayload), &payload)
	}
	if _, ok := payload["title"]; !ok {
		payload["title"] = item.Title
	}
	if _, ok := payload["content"]; !ok {
		payload["content"] = item.Content
	}
	if _, ok := payload["summary"]; !ok {
		payload["summary"] = item.Summary
	}
	if _, ok := payload["source_name"]; !ok {
		payload["source_name"] = nonEmpty(item.FromText, item.SourceType)
	}
	return payload
}

func legacyPayloadContains(payload map[string]any, needle string) bool {
	for _, value := range payload {
		if strings.Contains(strings.ToLower(legacyAnyString(value)), strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func legacyPayloadFieldContains(payload map[string]any, field string, needle string) bool {
	return strings.Contains(strings.ToLower(legacyAnyString(payload[field])), strings.ToLower(strings.TrimSpace(needle)))
}

func legacyItemBlobContains(item model.Item, needle string) bool {
	blob := strings.ToLower(strings.TrimSpace(item.Title + " " + item.Content + " " + item.Summary + " " + item.SourceType + " " + item.FromText + " " + item.TagFlags))
	return strings.Contains(blob, strings.ToLower(strings.TrimSpace(needle)))
}

func legacyPayloadString(payload map[string]any, key string) string {
	return strings.TrimSpace(legacyAnyString(payload[key]))
}

func legacyPayloadJSONArrayString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return "[]"
	}
	switch typed := value.(type) {
	case []any:
		raw, _ := json.Marshal(typed)
		return string(raw)
	case []string:
		raw, _ := json.Marshal(typed)
		return string(raw)
	default:
		if str := strings.TrimSpace(legacyAnyString(value)); str != "" {
			if strings.HasPrefix(str, "[") {
				return str
			}
			return legacyJSONString([]string{str})
		}
	}
	return "[]"
}

func legacyJSONTextPayload(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		raw, _ := json.Marshal(typed)
		return string(raw)
	}
}

func legacyAnyString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			part := strings.TrimSpace(legacyAnyString(item))
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, ",")
	case map[string]any:
		raw, _ := json.Marshal(typed)
		return string(raw)
	default:
		return fmt.Sprint(typed)
	}
}

func legacyTypeChildren(parentID int, prefix string, names []string) []legacySearchFullType {
	result := make([]legacySearchFullType, 0, len(names))
	for idx, name := range names {
		result = append(result, legacySearchFullType{
			OnlyID:    parentID*100 + idx + 1,
			ID:        parentID*100 + idx + 1,
			Type:      2,
			Name:      name,
			Value:     name,
			TypeOneID: parentID,
			Icon:      "mdi mdi-chevron-right",
			IsShow:    0,
			IsDefault: 0,
		})
	}
	if len(result) > 0 {
		result[0].Name = prefix + " / " + result[0].Name
	}
	return result
}

func legacySpecialDetailID(raw string) string {
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func legacySpecialTitle(kind string) string {
	switch kind {
	case "lawyer":
		return "律师详情"
	case "executionPerson":
		return "被执行人详情"
	case "professor":
		return "专家人才详情"
	case "doctor":
		return "医生详情"
	case "bidding":
		return "招标详情"
	case "invite":
		return "招聘详情"
	case "company":
		return "工商详情"
	case "judgment":
		return "法律文书详情"
	case "knowledge":
		return "知识产权详情"
	case "investment":
		return "投融资详情"
	case "thesisn":
		return "学术详情"
	case "report":
		return "公告研报详情"
	default:
		return "全文搜索详情"
	}
}

func legacySpecialDetailPageBody(kind string, detail map[string]any, mode string, returnPath string) string {
	title := nonEmpty(legacyDetailValue(detail, "name", "title", "caseTitle", "companyName"), "未命名")
	source := nonEmpty(legacyDetailValue(detail, "source_name", "source"), "未知来源")
	publishTime := legacyDetailValue(detail, "publish_time", "reportDate", "spider_time", "push_time")
	link := nonEmpty(legacyDetailValue(detail, "detailUrl", "detail_url", "detailurl", "url", "source_url"), "")
	summary := nonEmpty(legacyDetailValue(detail, "summary", "content"), "")

	var body strings.Builder
	body.WriteString(`<section><style>
	.detail-hero{padding:20px;border:1px solid #ece7dc;border-radius:16px;background:linear-gradient(135deg,#faf8f2,#f3efe4);margin-bottom:18px}
	.detail-kicker{color:#6a6257;font-size:13px;text-transform:uppercase;letter-spacing:.08em}
	.detail-meta{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin-top:16px}
	.detail-card{padding:14px;border:1px solid #e5dece;border-radius:12px;background:#fffdf8}
	.detail-card strong{display:block;font-size:20px;margin-top:6px;color:#214e34}
	.detail-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:18px;margin-bottom:18px}
	.detail-panel{padding:16px;border:1px solid #ece7dc;border-radius:14px;background:#fff}
	.detail-panel h3{margin-top:0}
	.detail-prose{white-space:pre-wrap;line-height:1.75}
	.detail-content-html{line-height:1.75}
	.detail-actions{display:flex;gap:12px;flex-wrap:wrap;margin-top:12px}
	.detail-figure{max-width:160px;max-height:160px;border-radius:14px;display:block;background:#f5f2ea;object-fit:cover}
	@media (max-width: 760px){.detail-grid{grid-template-columns:1fr}}
	</style>`)
	body.WriteString(`<div class="detail-hero"><div class="detail-kicker">`)
	body.WriteString(html.EscapeString(legacySpecialTitle(kind)))
	body.WriteString(`</div><h2>`)
	body.WriteString(html.EscapeString(title))
	body.WriteString(`</h2><div class="detail-actions">`)
	if link != "" {
		body.WriteString(`<a class="inline" href="`)
		body.WriteString(html.EscapeString(link))
		body.WriteString(`" target="_blank" rel="noreferrer">查看原文</a>`)
	}
	body.WriteString(`<a class="inline" href="`)
	body.WriteString(html.EscapeString(nonEmpty(returnPath, "/"+mode+"search/result")))
	body.WriteString(`">返回搜索结果</a></div><div class="detail-meta">`)
	body.WriteString(legacyDetailCard("来源", source))
	body.WriteString(legacyDetailCard("时间", publishTime))
	body.WriteString(legacyDetailCard("类型", legacySpecialTitle(kind)))
	body.WriteString(legacyDetailCard("编号", legacyDetailValue(detail, "article_public_id", "numberid", "caseCode")))
	body.WriteString(`</div></div></section>`)

	if image := nonEmpty(legacyDetailValue(detail, "img", "photo", "profile", "avatar", "companyLogo"), ""); image != "" {
		body.WriteString(`<section><div class="detail-panel"><h3>图片</h3><img class="detail-figure" src="`)
		body.WriteString(html.EscapeString(image))
		body.WriteString(`" alt="detail image"></div></section>`)
	}

	overviewFields := legacyDetailOverviewFields(kind)
	if len(overviewFields) > 0 {
		body.WriteString(`<section><div class="detail-panel"><h3>基础信息</h3><table><tbody>`)
		for _, field := range overviewFields {
			value := nonEmpty(legacyDetailValue(detail, field.Keys...), "")
			if value == "" {
				continue
			}
			body.WriteString(`<tr><th>`)
			body.WriteString(html.EscapeString(field.Label))
			body.WriteString(`</th><td>`)
			body.WriteString(html.EscapeString(value))
			body.WriteString(`</td></tr>`)
		}
		body.WriteString(`</tbody></table></div></section>`)
	}

	switch kind {
	case "company":
		if scope := legacyDetailValue(detail, "business_scope"); scope != "" {
			body.WriteString(legacyDetailTextPanel("经营范围", scope))
		}
		body.WriteString(legacyDetailJSONArraySection("主要人员", legacyDetailJSONRows(detail, "key_person"), []legacyDetailColumn{
			{Header: "序号", Keys: []string{"id"}},
			{Header: "姓名", Keys: []string{"name"}},
			{Header: "职务", Keys: []string{"position"}},
		}))
		body.WriteString(legacyDetailJSONArraySection("股东信息", legacyDetailJSONRows(detail, "shareholder"), []legacyDetailColumn{
			{Header: "序号", Keys: []string{"id"}},
			{Header: "股东", Keys: []string{"name"}},
			{Header: "认缴出资额", Keys: []string{"capital_contribution"}},
			{Header: "实际出资额", Keys: []string{"actual_contribution"}},
		}))
		body.WriteString(legacyDetailJSONArraySection("变更记录", legacyDetailJSONRows(detail, "change_record"), []legacyDetailColumn{
			{Header: "序号", Keys: []string{"id"}},
			{Header: "变更日期", Keys: []string{"alterDate"}},
			{Header: "变更项目", Keys: []string{"alterItem"}},
			{Header: "变更前", Keys: []string{"alterBefore"}},
			{Header: "变更后", Keys: []string{"alterAfter"}},
		}))
	case "investment":
		if intro := legacyDetailValue(detail, "infoIntro", "summary", "content"); intro != "" {
			body.WriteString(legacyDetailTextPanel("项目简介", intro))
		}
		body.WriteString(legacyDetailJSONArraySection("投资方", legacyDetailJSONRows(detail, "investorArray"), []legacyDetailColumn{
			{Header: "投资方", Keys: []string{"investorName", "name"}},
			{Header: "机构类型", Keys: []string{"investorType", "type"}},
		}))
		body.WriteString(legacyDetailJSONArraySection("融资历史", legacyDetailJSONRows(detail, "historyArray"), []legacyDetailColumn{
			{Header: "轮次", Keys: []string{"history_rounds"}},
			{Header: "投资方", Keys: []string{"history_investors"}},
			{Header: "时间", Keys: []string{"history_time"}},
			{Header: "金额", Keys: []string{"history_money"}},
		}))
	case "thesisn":
		body.WriteString(legacyDetailJSONArraySection("作者信息", legacyDetailJSONRows(detail, "co_author"), []legacyDetailColumn{
			{Header: "作者", Keys: []string{"name"}},
			{Header: "机构", Keys: []string{"institution", "org"}},
		}))
		if keywords := legacyDetailJSONArrayStrings(detail, "key_words"); len(keywords) > 0 {
			body.WriteString(`<section><div class="detail-panel"><h3>关键词</h3><p>`)
			body.WriteString(html.EscapeString(strings.Join(keywords, "、")))
			body.WriteString(`</p></div></section>`)
		}
		if readNum := legacyDetailValue(detail, "read_num"); readNum != "" {
			body.WriteString(legacyDetailTextPanel("阅读统计", "阅读量："+readNum))
		}
	case "bidding", "knowledge", "invite":
		if contentHTML := legacyDetailValue(detail, "content_html"); contentHTML != "" {
			body.WriteString(`<section><div class="detail-panel"><h3>正文内容</h3><div class="detail-content-html">`)
			body.WriteString(contentHTML)
			body.WriteString(`</div></div></section>`)
		}
		if intro := legacyDetailValue(detail, "company_intro"); intro != "" {
			body.WriteString(legacyDetailTextPanel("企业介绍", intro))
		}
	case "report":
		if reportURL := legacyDetailValue(detail, "url", "detailUrl", "source_url"); reportURL != "" {
			body.WriteString(`<section><div class="detail-panel"><h3>报告预览</h3><iframe src="`)
			body.WriteString(html.EscapeString(reportURL))
			body.WriteString(`" style="width:100%;min-height:720px;border:1px solid #ece7dc;border-radius:12px"></iframe></div></section>`)
		}
	}

	if kind == "judgment" || kind == "lawyer" || kind == "executionPerson" || kind == "professor" || kind == "doctor" || summary != "" {
		body.WriteString(legacyDetailTextPanel("摘要", summary))
	}
	if content := legacyDetailValue(detail, "content"); content != "" && content != summary && kind != "bidding" && kind != "knowledge" && kind != "invite" {
		body.WriteString(legacyDetailTextPanel("正文", content))
	}

	return body.String()
}

func (s *Server) legacyArticleDetailTarget(ctx context.Context, mode string, articleID string, returnPath string) string {
	item, err := s.fetchLegacyItemByID(articleID, 0)
	if err == nil {
		return s.legacySpecialDetailTarget(mode, item, returnPath)
	}
	return "/articles/" + url.PathEscape(articleID) + "?return_to=" + url.QueryEscape(returnPath)
}

func (s *Server) legacySpecialDetailTarget(mode string, item model.Item, returnPath string) string {
	kind := legacySpecialKind(item)
	if kind == "" {
		return "/articles/" + url.PathEscape(strconv.FormatInt(item.ID, 10)) + "?return_to=" + url.QueryEscape(returnPath)
	}
	path := legacySpecialPathPrefix(kind)
	if path == "" {
		return "/articles/" + url.PathEscape(strconv.FormatInt(item.ID, 10)) + "?return_to=" + url.QueryEscape(returnPath)
	}
	target := "/" + mode + "search/" + path + "/" + url.PathEscape(strconv.FormatInt(item.ID, 10))
	if returnPath != "" {
		target += "?return_to=" + url.QueryEscape(returnPath)
	}
	return target
}

func legacySpecialKind(item model.Item) string {
	source := strings.ToLower(strings.TrimSpace(item.SourceType))
	switch {
	case strings.Contains(source, "lawyer"):
		return "lawyer"
	case strings.Contains(source, "execution"):
		return "executionPerson"
	case strings.Contains(source, "professor"):
		return "professor"
	case strings.Contains(source, "doctor"):
		return "doctor"
	case strings.Contains(source, "bidding"):
		return "bidding"
	case strings.Contains(source, "invite"):
		return "invite"
	case strings.Contains(source, "company"):
		return "company"
	case strings.Contains(source, "judgment"):
		return "judgment"
	case strings.Contains(source, "knowledge"):
		return "knowledge"
	case strings.Contains(source, "investment"):
		return "investment"
	case strings.Contains(source, "thesis"):
		return "thesisn"
	case strings.Contains(source, "report"), strings.Contains(source, "announcement"):
		return "report"
	default:
		payload := legacyPayloadMap(item)
		for _, candidate := range []struct {
			keys []string
			kind string
		}{
			{[]string{"lawfirm", "goods", "telephone"}, "lawyer"},
			{[]string{"gistUnit", "caseCode", "disruptTypeName"}, "executionPerson"},
			{[]string{"institution", "times_cited", "H_index"}, "professor"},
			{[]string{"hospital", "department", "adept"}, "doctor"},
			{[]string{"numberid", "content_html"}, "bidding"},
			{[]string{"company_intro"}, "invite"},
			{[]string{"business_scope", "registered_capital_str"}, "company"},
			{[]string{"court", "caseType", "parties"}, "judgment"},
			{[]string{"ip_type", "owner", "content_html"}, "knowledge"},
			{[]string{"investorArray", "historyArray", "rounds"}, "investment"},
			{[]string{"co_author", "read_num"}, "thesisn"},
			{[]string{"reportDate", "authorList"}, "report"},
		} {
			for _, key := range candidate.keys {
				if strings.TrimSpace(legacyPayloadString(payload, key)) != "" {
					return candidate.kind
				}
			}
		}
	}
	return ""
}

func legacySpecialPathPrefix(kind string) string {
	switch kind {
	case "lawyer":
		return "lawyerDetail"
	case "executionPerson":
		return "executionPersonDetail"
	case "professor":
		return "professorDetail"
	case "doctor":
		return "doctorDetail"
	case "bidding":
		return "biddingdetail"
	case "invite":
		return "inviteDetails"
	case "company":
		return "companyDetail"
	case "judgment":
		return "judgmentDetail"
	case "knowledge":
		return "knowLedgeDetail"
	case "investment":
		return "investmentDetail"
	case "thesisn":
		return "thesisnDetail"
	case "report":
		return "reportdetail"
	default:
		return ""
	}
}

type legacyDetailField struct {
	Label string
	Keys  []string
}

type legacyDetailColumn struct {
	Header string
	Keys   []string
}

func legacyDetailOverviewFields(kind string) []legacyDetailField {
	switch kind {
	case "lawyer":
		return []legacyDetailField{
			{Label: "所属机构", Keys: []string{"lawfirm"}},
			{Label: "电话", Keys: []string{"telephone", "phone_number"}},
			{Label: "城市", Keys: []string{"city"}},
			{Label: "擅长领域", Keys: []string{"goods", "adept"}},
			{Label: "类型", Keys: []string{"kinds"}},
			{Label: "学历", Keys: []string{"educationbackground"}},
			{Label: "邮箱", Keys: []string{"email"}},
			{Label: "地址", Keys: []string{"address"}},
		}
	case "executionPerson":
		return []legacyDetailField{
			{Label: "执行单位", Keys: []string{"gistUnit"}},
			{Label: "履行情况", Keys: []string{"performance"}},
			{Label: "法院名称", Keys: []string{"courtName"}},
			{Label: "案件编号", Keys: []string{"caseCode"}},
			{Label: "行为", Keys: []string{"disruptTypeName"}},
			{Label: "职责", Keys: []string{"duty"}},
			{Label: "地址", Keys: []string{"address"}},
		}
	case "professor":
		return []legacyDetailField{
			{Label: "机构", Keys: []string{"institution"}},
			{Label: "引用量", Keys: []string{"times_cited"}},
			{Label: "作品数", Keys: []string{"works"}},
			{Label: "H 指数", Keys: []string{"H_index"}},
			{Label: "G 指数", Keys: []string{"G_index"}},
		}
	case "doctor":
		return []legacyDetailField{
			{Label: "医院", Keys: []string{"hospital"}},
			{Label: "科室", Keys: []string{"department"}},
			{Label: "电话", Keys: []string{"phone_number"}},
			{Label: "擅长", Keys: []string{"adept"}},
			{Label: "职称", Keys: []string{"degree"}},
			{Label: "地区", Keys: []string{"location", "province"}},
			{Label: "邮箱", Keys: []string{"email"}},
		}
	case "company":
		return []legacyDetailField{
			{Label: "企业名称", Keys: []string{"name"}},
			{Label: "法定代表人", Keys: []string{"legal_representative", "legal_person"}},
			{Label: "统一社会信用代码", Keys: []string{"uniformSocialCreditCode", "taxpayer_identification"}},
			{Label: "登记状态", Keys: []string{"status"}},
			{Label: "注册资本", Keys: []string{"registered_capital_str"}},
			{Label: "所属行业", Keys: []string{"industry_involved"}},
			{Label: "地址", Keys: []string{"address", "location"}},
			{Label: "参保人数", Keys: []string{"insured_num", "insureds"}},
		}
	case "judgment":
		return []legacyDetailField{
			{Label: "案由标题", Keys: []string{"caseTitle", "title"}},
			{Label: "法院", Keys: []string{"court", "courtName"}},
			{Label: "案件类型", Keys: []string{"caseType"}},
			{Label: "当事人", Keys: []string{"parties"}},
		}
	case "knowledge":
		return []legacyDetailField{
			{Label: "名称", Keys: []string{"name", "title"}},
			{Label: "类型", Keys: []string{"caseType", "ip_type"}},
			{Label: "权利人", Keys: []string{"owner"}},
		}
	case "investment":
		return []legacyDetailField{
			{Label: "公司", Keys: []string{"companyName", "company", "name"}},
			{Label: "轮次", Keys: []string{"rounds", "round", "investment_type"}},
			{Label: "融资金额", Keys: []string{"money"}},
			{Label: "行业", Keys: []string{"industry"}},
			{Label: "采集时间", Keys: []string{"spider_time", "publish_time"}},
		}
	case "thesisn":
		return []legacyDetailField{
			{Label: "来源", Keys: []string{"source_name"}},
			{Label: "阅读量", Keys: []string{"read_num"}},
			{Label: "时间", Keys: []string{"spider_time", "publish_time"}},
		}
	case "bidding":
		return []legacyDetailField{
			{Label: "项目编号", Keys: []string{"numberid"}},
			{Label: "来源", Keys: []string{"source_name"}},
			{Label: "时间", Keys: []string{"publish_time", "spider_time"}},
		}
	case "invite":
		return []legacyDetailField{
			{Label: "企业", Keys: []string{"company_name", "name"}},
			{Label: "地区", Keys: []string{"address", "city"}},
			{Label: "时间", Keys: []string{"publish_time"}},
		}
	case "report":
		return []legacyDetailField{
			{Label: "标题", Keys: []string{"title"}},
			{Label: "日期", Keys: []string{"reportDate"}},
			{Label: "链接", Keys: []string{"url"}},
		}
	default:
		return nil
	}
}

func legacyDetailValue(detail map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(legacyAnyString(detail[key])); value != "" {
			return value
		}
	}
	return ""
}

func legacyDetailCard(label string, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "暂无"
	}
	return `<div class="detail-card"><div class="template-meta">` + html.EscapeString(label) + `</div><strong>` + html.EscapeString(value) + `</strong></div>`
}

func legacyDetailTextPanel(title string, content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return `<section><div class="detail-panel"><h3>` + html.EscapeString(title) + `</h3><div class="detail-prose">` + html.EscapeString(content) + `</div></div></section>`
}

func legacyDetailJSONRows(detail map[string]any, key string) []map[string]any {
	raw := strings.TrimSpace(legacyAnyString(detail[key]))
	if raw == "" {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err == nil {
		return rows
	}
	return nil
}

func legacyDetailJSONArrayStrings(detail map[string]any, key string) []string {
	raw := strings.TrimSpace(legacyAnyString(detail[key]))
	if raw == "" {
		return nil
	}
	var stringsOut []string
	if err := json.Unmarshal([]byte(raw), &stringsOut); err == nil {
		return stringsOut
	}
	var mixed []any
	if err := json.Unmarshal([]byte(raw), &mixed); err == nil {
		out := make([]string, 0, len(mixed))
		for _, item := range mixed {
			if value := strings.TrimSpace(legacyAnyString(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	}
	return nil
}

func legacyDetailJSONArraySection(title string, rows []map[string]any, columns []legacyDetailColumn) string {
	if len(rows) == 0 || len(columns) == 0 {
		return ""
	}
	var body strings.Builder
	body.WriteString(`<section><div class="detail-panel"><h3>`)
	body.WriteString(html.EscapeString(title))
	body.WriteString(`</h3><table><thead><tr>`)
	for _, column := range columns {
		body.WriteString(`<th>`)
		body.WriteString(html.EscapeString(column.Header))
		body.WriteString(`</th>`)
	}
	body.WriteString(`</tr></thead><tbody>`)
	for _, row := range rows {
		body.WriteString(`<tr>`)
		for _, column := range columns {
			value := ""
			for _, key := range column.Keys {
				if text := strings.TrimSpace(legacyAnyString(row[key])); text != "" {
					value = text
					break
				}
			}
			body.WriteString(`<td>`)
			body.WriteString(html.EscapeString(value))
			body.WriteString(`</td>`)
		}
		body.WriteString(`</tr>`)
	}
	body.WriteString(`</tbody></table></div></section>`)
	return body.String()
}

func legacyTemplateMatchesType(stype string, sourceType string, name string, configJSON string) bool {
	target := strings.ToLower(strings.TrimSpace(stype))
	blob := strings.ToLower(strings.TrimSpace(sourceType + " " + name + " " + configJSON))
	return strings.Contains(blob, target)
}

func writePlainJSONText(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(payload)
	_, _ = w.Write(bytes.TrimSpace(buf.Bytes()))
}
