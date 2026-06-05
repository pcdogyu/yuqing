package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func TestLegacySearchHistoryAndRedirect(t *testing.T) {
	var posted map[string]string
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/search/history":
			if got := r.URL.Query().Get("user_id"); got != "42" {
				t.Fatalf("expected user_id 42, got %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Fatalf("decode history post: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": map[string]any{"saved": true}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/search/history":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": []model.SearchWordStat{
					{SearchWord: "钢铁", UserID: 42, WordCount: 2},
					{SearchWord: "能源", UserID: 42, WordCount: 1},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": map[string]any{}})
		}
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}

	redirectReq := httptest.NewRequest(http.MethodGet, "/fullsearch/result?searchword=钢铁&page=2&pageSize=20", nil)
	redirectRR := httptest.NewRecorder()
	srv.handleLegacySearchResult(redirectRR, redirectReq, map[string]any{"id": int64(42)}, "full")
	if redirectRR.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", redirectRR.Code)
	}
	target := redirectRR.Header().Get("Location")
	if target == "" {
		t.Fatalf("expected redirect target")
	}
	targetURL, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse redirect target: %v", err)
	}
	if targetURL.Path != "/articles" || targetURL.Query().Get("mode") != "full" {
		t.Fatalf("unexpected redirect target: %s", target)
	}
	if targetURL.Query().Get("keyword") != "钢铁" {
		t.Fatalf("expected keyword preserved in redirect, got %s", target)
	}
	if posted["search_word"] != "钢铁" {
		t.Fatalf("expected search word to be recorded, got %+v", posted)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/fullsearch/search", nil)
	historyRR := httptest.NewRecorder()
	srv.handleLegacySearchHistory(historyRR, historyReq, map[string]any{"id": int64(42)})
	if historyRR.Code != http.StatusOK {
		t.Fatalf("expected history 200, got %d", historyRR.Code)
	}
	var historyEnvelope struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    []model.SearchWordStat `json:"data"`
	}
	if err := json.Unmarshal(historyRR.Body.Bytes(), &historyEnvelope); err != nil {
		t.Fatalf("unmarshal history response: %v", err)
	}
	if historyEnvelope.Code != http.StatusOK || len(historyEnvelope.Data) != 2 {
		t.Fatalf("unexpected history payload: %+v", historyEnvelope)
	}
	if historyEnvelope.Data[0].SearchWord != "钢铁" || historyEnvelope.Data[0].WordCount != 2 {
		t.Fatalf("unexpected history ranking: %+v", historyEnvelope.Data)
	}
}

func TestLegacySearchInformationList(t *testing.T) {
	var seenQuery url.Values
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search/full" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": map[string]any{}})
			return
		}
		seenQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.SearchResult{
				Total:    1,
				Page:     2,
				PageSize: 3,
				Items: []model.Item{{
					ID:              55,
					Title:           "钢铁行业回暖",
					Content:         "钢铁行业回暖",
					Summary:         "钢铁行业回暖",
					SourceType:      "headline",
					SourceURL:       "https://example.com/55",
					PublishTime:     "2026-06-03 11:00:00",
					PublishTimeText: "5分钟前",
					FromText:        "新华网",
					TagFlags:        "钢铁,能源",
				}},
			},
		})
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	req := httptest.NewRequest(http.MethodPost, "/fullsearch/informationListpost?searchword=钢铁&page=2&pageSize=3&project_id=7&source_type=headline&industry=能源&province=上海&city=浦东", nil)
	rr := httptest.NewRecorder()

	srv.handleLegacySearchInformationList(rr, req, map[string]any{"id": int64(42)}, "full")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if seenQuery.Get("q") != "钢铁" || seenQuery.Get("page") != "2" || seenQuery.Get("page_size") != "3" {
		t.Fatalf("unexpected forwarded search query: %+v", seenQuery)
	}
	if seenQuery.Get("project_id") != "7" || seenQuery.Get("source_type") != "headline" || seenQuery.Get("province") != "上海" || seenQuery.Get("city") != "浦东" {
		t.Fatalf("unexpected forwarded filters: %+v", seenQuery)
	}

	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Data                []legacySearchArticle `json:"data"`
			TotalPage           int                   `json:"totalPage"`
			TotalCount          int                   `json:"totalCount"`
			CurrentPage         int                   `json:"currentPage"`
			ArticlePublicIDList []string              `json:"article_public_idList"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if envelope.Code != http.StatusOK || envelope.Data.TotalCount != 1 || envelope.Data.CurrentPage != 2 || envelope.Data.TotalPage != 1 {
		t.Fatalf("unexpected response metadata: %+v", envelope)
	}
	if len(envelope.Data.Data) != 1 || envelope.Data.Data[0].ArticlePublicID != "55" {
		t.Fatalf("unexpected article list: %+v", envelope.Data.Data)
	}
	if len(envelope.Data.ArticlePublicIDList) != 1 || envelope.Data.ArticlePublicIDList[0] != "55" {
		t.Fatalf("unexpected article id list: %+v", envelope.Data.ArticlePublicIDList)
	}
	if !strings.Contains(envelope.Data.Data[0].KeyWords, "钢铁") {
		t.Fatalf("expected keyword in article keywords, got %+v", envelope.Data.Data[0])
	}
}

func TestLegacyHotList(t *testing.T) {
	var seenQuery url.Values
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search/full" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": map[string]any{}})
			return
		}
		seenQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.SearchResult{
				Total:    1,
				Page:     3,
				PageSize: 25,
				Items: []model.Item{{
					ID:              88,
					Title:           "热点文章上涨",
					Content:         "热点文章上涨",
					Summary:         "热点文章上涨",
					SourceType:      "headline",
					SourceURL:       "https://example.com/88",
					PublishTime:     "2026-06-03 12:00:00",
					PublishTimeText: "2分钟前",
					FromText:        "新华网",
				}},
			},
		})
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	req := httptest.NewRequest(http.MethodGet, "/fullsearch/hotList?pageNum=3&pageSize=25&searchWord=%E7%83%AD%E7%82%B9", nil)
	rr := httptest.NewRecorder()

	srv.handleLegacyHotList(rr, req, map[string]any{"id": int64(42)})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if seenQuery.Get("q") != "热点" || seenQuery.Get("page") != "3" || seenQuery.Get("page_size") != "25" {
		t.Fatalf("unexpected forwarded hot list query: %+v", seenQuery)
	}

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Code      int `json:"code"`
			PageCount int `json:"page_count"`
			Count     int `json:"count"`
			Page      int `json:"page"`
			Size      int `json:"size"`
			Data      []struct {
				Source map[string]any `json:"_source"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal hot list response: %v", err)
	}
	if envelope.Code != http.StatusOK || envelope.Data.Code != http.StatusOK || envelope.Data.PageCount != 1 || envelope.Data.Count != 1 || envelope.Data.Page != 3 || envelope.Data.Size != 25 {
		t.Fatalf("unexpected hot list metadata: %+v", envelope)
	}
	if len(envelope.Data.Data) != 1 || envelope.Data.Data[0].Source["topic"] != "热点文章上涨" || envelope.Data.Data[0].Source["source_name"] != "新华网" {
		t.Fatalf("unexpected hot list payload: %+v", envelope.Data.Data)
	}
}

func TestLegacySearchCategoryLists(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search/full" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": model.SearchResult{
				Total:    1,
				Page:     2,
				PageSize: 25,
				Items: []model.Item{{
					ID:              88,
					Title:           "公告/研报标题",
					Content:         "公告/研报正文",
					Summary:         "摘要",
					SourceType:      "headline",
					SourceURL:       "https://example.com/88",
					PublishTime:     "2026-06-03 12:00:00",
					PublishTimeText: "2分钟前",
					FromText:        "新华网",
				}},
			},
		})
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	user := map[string]any{"id": int64(42)}

	complaintReq := httptest.NewRequest(http.MethodGet, "/fullsearch/complaintList?page=2&pageSize=25&searchword=%E7%83%AD%E7%82%B9", nil)
	complaintRR := httptest.NewRecorder()
	srv.handleLegacyComplaintList(complaintRR, complaintReq, user)
	if complaintRR.Code != http.StatusOK {
		t.Fatalf("expected complaint 200, got %d", complaintRR.Code)
	}
	var complaintEnvelope struct {
		Code int `json:"code"`
		Data struct {
			News []struct {
				Source map[string]any `json:"_source"`
			} `json:"news"`
			Count     int    `json:"count"`
			PageCount int    `json:"page_count"`
			Page      int    `json:"page"`
			Size      int    `json:"size"`
			Classify  string `json:"classify"`
		} `json:"data"`
	}
	if err := json.Unmarshal(complaintRR.Body.Bytes(), &complaintEnvelope); err != nil {
		t.Fatalf("unmarshal complaint response: %v", err)
	}
	if complaintEnvelope.Code != http.StatusOK || complaintEnvelope.Data.Count != 1 || complaintEnvelope.Data.PageCount != 1 || complaintEnvelope.Data.Page != 2 || complaintEnvelope.Data.Size != 25 {
		t.Fatalf("unexpected complaint metadata: %+v", complaintEnvelope)
	}
	if len(complaintEnvelope.Data.News) != 1 || complaintEnvelope.Data.News[0].Source["problem"] != "公告/研报标题" {
		t.Fatalf("unexpected complaint payload: %+v", complaintEnvelope.Data.News)
	}

	announcementReq := httptest.NewRequest(http.MethodGet, "/fullsearch/announcementList?page=2&pageSize=25&searchword=%E7%83%AD%E7%82%B9", nil)
	announcementRR := httptest.NewRecorder()
	srv.handleLegacyAnnouncementList(announcementRR, announcementReq, user)
	if announcementRR.Code != http.StatusOK {
		t.Fatalf("expected announcement 200, got %d", announcementRR.Code)
	}
	var announcementEnvelope struct {
		Code int `json:"code"`
		Data struct {
			List      []map[string]any `json:"list"`
			TotalPage int              `json:"totalPage"`
			TotalData int              `json:"totalData"`
			Page      int              `json:"page"`
			Size      int              `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(announcementRR.Body.Bytes(), &announcementEnvelope); err != nil {
		t.Fatalf("unmarshal announcement response: %v", err)
	}
	if announcementEnvelope.Code != http.StatusOK || announcementEnvelope.Data.TotalData != 1 || announcementEnvelope.Data.TotalPage != 1 || announcementEnvelope.Data.Page != 2 || announcementEnvelope.Data.Size != 25 {
		t.Fatalf("unexpected announcement metadata: %+v", announcementEnvelope)
	}
	if len(announcementEnvelope.Data.List) != 1 || announcementEnvelope.Data.List[0]["title"] != "公告/研报标题" {
		t.Fatalf("unexpected announcement payload: %+v", announcementEnvelope.Data.List)
	}

	reportReq := httptest.NewRequest(http.MethodGet, "/fullsearch/reportList?page=2&pageSize=25&searchword=%E7%83%AD%E7%82%B9", nil)
	reportRR := httptest.NewRecorder()
	srv.handleLegacyReportList(reportRR, reportReq, user)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("expected report 200, got %d", reportRR.Code)
	}
	var reportEnvelope struct {
		Code int `json:"code"`
		Data struct {
			List      []map[string]any `json:"list"`
			TotalPage int              `json:"totalPage"`
			TotalData int              `json:"totalData"`
			Page      int              `json:"page"`
			Size      int              `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reportRR.Body.Bytes(), &reportEnvelope); err != nil {
		t.Fatalf("unmarshal report response: %v", err)
	}
	if reportEnvelope.Code != http.StatusOK || reportEnvelope.Data.TotalData != 1 || reportEnvelope.Data.TotalPage != 1 || reportEnvelope.Data.Page != 2 || reportEnvelope.Data.Size != 25 {
		t.Fatalf("unexpected report metadata: %+v", reportEnvelope)
	}
	if len(reportEnvelope.Data.List) != 1 || reportEnvelope.Data.List[0]["title"] != "公告/研报标题" {
		t.Fatalf("unexpected report payload: %+v", reportEnvelope.Data.List)
	}

	announceTypeReq := httptest.NewRequest(http.MethodGet, "/fullsearch/announcementrtype", nil)
	announceTypeRR := httptest.NewRecorder()
	srv.handleSearchCompat(announceTypeRR, announceTypeReq, user, "full")
	if announceTypeRR.Code != http.StatusOK {
		t.Fatalf("expected announcementrtype 200, got %d", announceTypeRR.Code)
	}
	reportTypeReq := httptest.NewRequest(http.MethodGet, "/fullsearch/reportIndustry", nil)
	reportTypeRR := httptest.NewRecorder()
	srv.handleSearchCompat(reportTypeRR, reportTypeReq, user, "full")
	if reportTypeRR.Code != http.StatusOK {
		t.Fatalf("expected reportIndustry 200, got %d", reportTypeRR.Code)
	}
}

func TestLegacySpecializedFullSearchEndpoints(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/search/full":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.SearchResult{
					Total:    2,
					Page:     1,
					PageSize: 200,
					Items: []model.Item{
						{
							ID:              101,
							Title:           "张三律师",
							Content:         "擅长公司法与投融资",
							Summary:         "南京律师",
							SourceType:      "lawyer",
							FromText:        "律所库",
							SourceURL:       "https://example.com/lawyer/101",
							PublishTime:     "2026-06-05 10:00:00",
							PublishTimeText: "今天",
							RawPayload:      `{"name":"张三","telephone":"13800000000","kinds":"专职律师","goods":"公司法","educationbackground":"硕士","email":"zhang@example.com","certID":"A1001","qualifitime":"2020-01-01","lawfirm":"金陵律师事务所","address":"南京市鼓楼区","city":"南京"}`,
						},
						{
							ID:              202,
							Title:           "星云科技有限公司",
							Content:         "企业信息与股东结构",
							Summary:         "高新技术企业",
							SourceType:      "company",
							FromText:        "企查查",
							SourceURL:       "https://example.com/company/202",
							PublishTime:     "2026-06-05 11:00:00",
							PublishTimeText: "今天",
							RawPayload:      `{"name":"星云科技有限公司","industry_involved":"人工智能","legal_person":"李四","registered_capital_str":"500万","status":"存续","location":"上海市浦东新区","business_scope":"人工智能软件开发","uniformSocialCreditCode":"91310000X","insured_num":"30"}`,
						},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/101":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.Item{
					ID:              101,
					Title:           "张三律师",
					Content:         "擅长公司法与投融资",
					Summary:         "南京律师",
					SourceType:      "lawyer",
					FromText:        "律所库",
					SourceURL:       "https://example.com/lawyer/101",
					PublishTime:     "2026-06-05 10:00:00",
					PublishTimeText: "今天",
					RawPayload:      `{"name":"张三","telephone":"13800000000","kinds":"专职律师","goods":"公司法","educationbackground":"硕士","email":"zhang@example.com","certID":"A1001","qualifitime":"2020-01-01","lawfirm":"金陵律师事务所","address":"南京市鼓楼区","city":"南京"}`,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/202":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.Item{
					ID:              202,
					Title:           "星云科技有限公司",
					Content:         "企业信息与股东结构",
					Summary:         "高新技术企业",
					SourceType:      "company",
					FromText:        "企查查",
					SourceURL:       "https://example.com/company/202",
					PublishTime:     "2026-06-05 11:00:00",
					PublishTimeText: "今天",
					RawPayload:      `{"name":"星云科技有限公司","industry_involved":"人工智能","legal_person":"李四","registered_capital_str":"500万","status":"存续","location":"上海市浦东新区","business_scope":"人工智能软件开发","uniformSocialCreditCode":"91310000X","insured_num":"30"}`,
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": nil})
		}
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	user := map[string]any{"id": int64(42)}

	listReq := httptest.NewRequest(http.MethodGet, "/fullsearch/lawyerList?searchWord=张&pageNum=1&pageSize=10", nil)
	listRR := httptest.NewRecorder()
	srv.handleSearchCompat(listRR, listReq, user, "full")
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected lawyerList 200, got %d", listRR.Code)
	}
	var lawyerList struct {
		Code      string                   `json:"code"`
		TotalData int                      `json:"totalData"`
		List      []map[string]interface{} `json:"list"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &lawyerList); err != nil {
		t.Fatalf("unmarshal lawyerList: %v", err)
	}
	if lawyerList.Code != "200" || lawyerList.TotalData != 1 || lawyerList.List[0]["name"] != "张三" {
		t.Fatalf("unexpected lawyer list payload: %+v", lawyerList)
	}

	detailDataReq := httptest.NewRequest(http.MethodPost, "/fullsearch/lawyerDetailData?article_public_id=101", nil)
	detailDataRR := httptest.NewRecorder()
	srv.handleSearchCompat(detailDataRR, detailDataReq, user, "full")
	if detailDataRR.Code != http.StatusOK {
		t.Fatalf("expected lawyerDetailData 200, got %d", detailDataRR.Code)
	}
	var lawyerDetail struct {
		List []map[string]interface{} `json:"list"`
	}
	if err := json.Unmarshal(detailDataRR.Body.Bytes(), &lawyerDetail); err != nil {
		t.Fatalf("unmarshal lawyerDetailData: %v", err)
	}
	if len(lawyerDetail.List) != 1 || lawyerDetail.List[0]["lawfirm"] != "金陵律师事务所" {
		t.Fatalf("unexpected lawyer detail payload: %+v", lawyerDetail)
	}
	if lawyerDetail.List[0]["telephone"] != "13800000000" || lawyerDetail.List[0]["detailurl"] != "https://example.com/lawyer/101" {
		t.Fatalf("expected lawyer compatibility fields, got %+v", lawyerDetail.List[0])
	}

	companyCategoryReq := httptest.NewRequest(http.MethodGet, "/fullsearch/companyIndustry", nil)
	companyCategoryRR := httptest.NewRecorder()
	srv.handleSearchCompat(companyCategoryRR, companyCategoryReq, user, "full")
	if companyCategoryRR.Code != http.StatusOK {
		t.Fatalf("expected companyIndustry 200, got %d", companyCategoryRR.Code)
	}
	var companyCategories []map[string]interface{}
	if err := json.Unmarshal(companyCategoryRR.Body.Bytes(), &companyCategories); err != nil {
		t.Fatalf("unmarshal companyIndustry: %v", err)
	}
	if len(companyCategories) < 2 || companyCategories[1]["name"] != "人工智能" {
		t.Fatalf("unexpected company categories: %+v", companyCategories)
	}

	typeReq := httptest.NewRequest(http.MethodGet, "/fullsearch/listFullTypeBySecond?type_one_id=39", nil)
	typeRR := httptest.NewRecorder()
	srv.handleSearchCompat(typeRR, typeReq, user, "full")
	if typeRR.Code != http.StatusOK {
		t.Fatalf("expected listFullTypeBySecond 200, got %d", typeRR.Code)
	}
	var secondTypes []legacySearchFullType
	if err := json.Unmarshal(typeRR.Body.Bytes(), &secondTypes); err != nil {
		t.Fatalf("unmarshal second types: %v", err)
	}
	if len(secondTypes) == 0 || secondTypes[0].TypeOneID != 39 {
		t.Fatalf("unexpected second type payload: %+v", secondTypes)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/fullsearch/lawyerDetail/101", nil)
	pageRR := httptest.NewRecorder()
	srv.handleSearchCompat(pageRR, pageReq, user, "full")
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected lawyerDetail page 200, got %d", pageRR.Code)
	}
	if !strings.Contains(pageRR.Body.String(), "律师详情") || !strings.Contains(pageRR.Body.String(), "张三") {
		t.Fatalf("unexpected lawyer detail page: %s", pageRR.Body.String())
	}

	companyDetailReq := httptest.NewRequest(http.MethodGet, "/fullsearch/companyDetails?article_public_id=202", nil)
	companyDetailRR := httptest.NewRecorder()
	srv.handleSearchCompat(companyDetailRR, companyDetailReq, user, "full")
	if companyDetailRR.Code != http.StatusOK {
		t.Fatalf("expected companyDetails 200, got %d", companyDetailRR.Code)
	}
	var companyDetail map[string]interface{}
	if err := json.Unmarshal(companyDetailRR.Body.Bytes(), &companyDetail); err != nil {
		t.Fatalf("unmarshal company detail: %v", err)
	}
	if companyDetail["name"] != "星云科技有限公司" || companyDetail["industry_involved"] != "人工智能" {
		t.Fatalf("unexpected company detail payload: %+v", companyDetail)
	}
	if companyDetail["taxpayer_identification"] != "91310000X" || companyDetail["insureds"] != "30" || companyDetail["legal_person"] != "李四" {
		t.Fatalf("expected company alias fields, got %+v", companyDetail)
	}
}

func TestTimelySearchTemplateUsesCrawlTemplates(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/crawl-templates" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": nil})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data": []model.CrawlTemplate{
				{ID: 1, Name: "Flash Template", SourceType: "flash", Enabled: true, ConfigJSON: `{"source_type":"flash"}`},
				{ID: 2, Name: "Disabled Template", SourceType: "flash", Enabled: false, ConfigJSON: `{"source_type":"flash"}`},
				{ID: 3, Name: "Headline Template", SourceType: "headline", Enabled: true, ConfigJSON: `{"source_type":"headline"}`},
			},
		})
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	req := httptest.NewRequest(http.MethodGet, "/timelysearch/templete?stype=flash", nil)
	rr := httptest.NewRecorder()
	srv.handleTimelySearchTemplate(rr, req, map[string]any{"id": int64(42)})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected timelysearch template 200, got %d", rr.Code)
	}
	var templates []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &templates); err != nil {
		t.Fatalf("unmarshal timely template response: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected filtered templates plus all option, got %+v", templates)
	}
	if templates[0]["engine"] != "全部" || templates[1]["engine"] != "Flash Template" {
		t.Fatalf("unexpected timely template payload: %+v", templates)
	}
}

func TestTimelySearchPageExecuteAndDataFlow(t *testing.T) {
	var searchQuery url.Values
	var crawlQuery url.Values
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": []model.CrawlTemplate{
					{ID: 1, Name: "Flash Template", SourceType: "flash", Enabled: true, ConfigJSON: `{"source_type":"flash"}`},
					{ID: 2, Name: "Headline Template", SourceType: "headline", Enabled: true, ConfigJSON: `{"source_type":"headline"}`},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data":    model.CrawlTemplate{ID: 1, Name: "Flash Template", SourceType: "flash", Enabled: true, ConfigJSON: `{"source_type":"flash"}`},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/search/timely":
			searchQuery = r.URL.Query()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.SearchResult{
					Total:    1,
					Page:     2,
					PageSize: 20,
					Items: []model.Item{{
						ID:              301,
						Title:           "Flash item",
						Summary:         "Flash summary",
						SourceType:      "flash",
						FromText:        "Flash Source",
						PublishTimeText: "刚刚",
						SourceURL:       "https://example.com/flash/301",
					}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": nil})
		}
	}))
	defer content.Close()

	crawler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": []model.CrawlRun{{
					ID:            99,
					SourceType:    "flash",
					TemplateID:    1,
					TemplateName:  "Flash Template",
					Status:        "success",
					FetchedCount:  12,
					InsertedCount: 8,
					StartedAt:     time.Date(2026, 6, 5, 12, 30, 0, 0, time.UTC),
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/tasks/crawl":
			crawlQuery = r.URL.Query()
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.CrawlSummary{SourceType: "flash", RunID: 99}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found", "data": nil})
		}
	}))
	defer crawler.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL, CrawlerURL: crawler.URL}, client: resty.New()}

	pageReq := httptest.NewRequest(http.MethodGet, "/timelysearch/result?keyword=AI&website_id=1&stype=flash&pageNoData=2", nil)
	pageRR := httptest.NewRecorder()
	srv.handleSearchCompat(pageRR, pageReq, nil, "timely")
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch result page 200, got %d", pageRR.Code)
	}
	pageBody := pageRR.Body.String()
	if !strings.Contains(pageBody, "即时搜索") || !strings.Contains(pageBody, "Flash Template") || !strings.Contains(pageBody, "Flash item") || !strings.Contains(pageBody, "立即抓取") {
		t.Fatalf("unexpected timelysearch result page: %s", pageBody)
	}
	if searchQuery.Get("q") != "AI" || searchQuery.Get("page") != "2" || searchQuery.Get("source_type") != "flash" {
		t.Fatalf("unexpected timely search forwarding: %+v", searchQuery)
	}

	dataReq := httptest.NewRequest(http.MethodGet, "/timelysearch/data?keyword=AI&website_id=1&pageNoData=2", nil)
	dataRR := httptest.NewRecorder()
	srv.handleSearchCompat(dataRR, dataReq, nil, "timely")
	if dataRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch data 200, got %d", dataRR.Code)
	}
	if !strings.Contains(dataRR.Body.String(), `Flash item`) || !strings.Contains(dataRR.Body.String(), `Flash Source`) {
		t.Fatalf("unexpected timelysearch data body: %s", dataRR.Body.String())
	}

	form := url.Values{}
	form.Set("website_id", "1")
	form.Set("keyword", "AI")
	form.Set("stype", "flash")
	form.Set("pageNoData", "2")
	executeReq := httptest.NewRequest(http.MethodPost, "/timelysearch/execute", strings.NewReader(form.Encode()))
	executeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	executeRR := httptest.NewRecorder()
	srv.handleSearchCompat(executeRR, executeReq, nil, "timely")
	if executeRR.Code != http.StatusSeeOther {
		t.Fatalf("expected timelysearch execute redirect, got %d", executeRR.Code)
	}
	loc := executeRR.Header().Get("Location")
	if !strings.Contains(loc, "/timelysearch/result?") || !strings.Contains(loc, "website_id=1") || !strings.Contains(loc, "msg=") {
		t.Fatalf("unexpected execute redirect: %s", loc)
	}
	if crawlQuery.Get("template_id") != "1" || crawlQuery.Get("keyword") != "AI" {
		t.Fatalf("unexpected crawl execute query: %+v", crawlQuery)
	}
}
