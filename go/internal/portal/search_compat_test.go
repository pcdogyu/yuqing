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
	"golang.org/x/net/websocket"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
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

func TestLegacySearchResultRedirectsCryptoTermsAndKeepsHistory(t *testing.T) {
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
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": map[string]any{}})
		}
	}))
	defer content.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	tests := []struct {
		name string
		path string
		want string
		word string
	}{
		{name: "btc", path: "/fullsearch/result?searchword=btc", want: "/crypto?pair=BTCUSDT", word: "btc"},
		{name: "bitcoin cn", path: "/fullsearch/result?searchword=比特币", want: "/crypto?pair=BTCUSDT", word: "比特币"},
		{name: "btc usdt", path: "/fullsearch/result?searchword=btc/usdt", want: "/crypto?pair=BTCUSDT", word: "btc/usdt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			posted = nil
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			srv.handleLegacySearchResult(rr, req, map[string]any{"id": int64(42)}, "full")
			if rr.Code != http.StatusSeeOther {
				t.Fatalf("expected redirect, got %d", rr.Code)
			}
			if got := rr.Header().Get("Location"); got != tc.want {
				t.Fatalf("expected redirect %q, got %q", tc.want, got)
			}
			if posted["search_word"] != tc.word {
				t.Fatalf("expected search word %q to be recorded, got %+v", tc.word, posted)
			}
		})
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
	if !strings.Contains(envelope.Data.Data[0].Url, "/articles/55?return_to=%2Farticles%3F") || !strings.Contains(envelope.Data.Data[0].Url, "mode%3Dfull") {
		t.Fatalf("expected canonical article link in fullsearch list, got %+v", envelope.Data.Data[0])
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

	srv.handleLegacyHotList(rr, req, map[string]any{"id": int64(42)}, "full")
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
	srv.handleLegacyComplaintList(complaintRR, complaintReq, user, "full")
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
	srv.handleLegacyAnnouncementList(announcementRR, announcementReq, user, "full")
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
	srv.handleLegacyReportList(reportRR, reportReq, user, "full")
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
							RawPayload:      `{"name":"星云科技有限公司","industry_involved":"人工智能","legal_person":"李四","registered_capital_str":"500万","status":"存续","location":"上海市浦东新区","business_scope":"人工智能软件开发","uniformSocialCreditCode":"91310000X","insured_num":"30","key_person":"[{\"id\":1,\"name\":\"李四\",\"position\":\"董事长\"}]","shareholder":"[{\"id\":1,\"name\":\"星云控股\",\"capital_contribution\":\"300万\",\"actual_contribution\":\"300万\"}]","change_record":"[{\"id\":1,\"alterDate\":\"2026-01-01\",\"alterItem\":\"法定代表人\",\"alterBefore\":\"王五\",\"alterAfter\":\"李四\"}]","phone_number":"025-12345678"}`,
						},
						{
							ID:              303,
							Title:           "Alpha AI 完成 A 轮融资",
							Content:         "项目简介与融资历史",
							Summary:         "聚焦智能风控",
							SourceType:      "investment",
							FromText:        "投融资库",
							SourceURL:       "https://example.com/investment/303",
							PublishTime:     "2026-06-05 12:00:00",
							PublishTimeText: "今天",
							RawPayload:      `{"title":"Alpha AI 完成 A 轮融资","companyName":"Alpha AI","rounds":"A轮","money":"数千万人民币","industry":"人工智能","infoIntro":"智能风控平台","companyLogo":"https://example.com/logo.png","deatilUrl":"https://example.com/investment/303","push_time":"2026-06-05","spider_time":"2026-06-05 12:00:00","investorArray":"[{\"investorName\":\"启明创投\",\"investorType\":\"VC\"}]","historyArray":"[{\"history_rounds\":\"天使轮\",\"history_investors\":\"个人投资者\",\"history_time\":\"2025-01-01\",\"history_money\":\"数百万人民币\"}]"} `,
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
					RawPayload:      `{"name":"星云科技有限公司","industry_involved":"人工智能","legal_person":"李四","registered_capital_str":"500万","status":"存续","location":"上海市浦东新区","business_scope":"人工智能软件开发","uniformSocialCreditCode":"91310000X","insured_num":"30","key_person":"[{\"id\":1,\"name\":\"李四\",\"position\":\"董事长\"}]","shareholder":"[{\"id\":1,\"name\":\"星云控股\",\"capital_contribution\":\"300万\",\"actual_contribution\":\"300万\"}]","change_record":"[{\"id\":1,\"alterDate\":\"2026-01-01\",\"alterItem\":\"法定代表人\",\"alterBefore\":\"王五\",\"alterAfter\":\"李四\"}]","phone_number":"025-12345678"}`,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/303":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.Item{
					ID:              303,
					Title:           "Alpha AI 完成 A 轮融资",
					Content:         "项目简介与融资历史",
					Summary:         "聚焦智能风控",
					SourceType:      "investment",
					FromText:        "投融资库",
					SourceURL:       "https://example.com/investment/303",
					PublishTime:     "2026-06-05 12:00:00",
					PublishTimeText: "今天",
					RawPayload:      `{"title":"Alpha AI 完成 A 轮融资","companyName":"Alpha AI","rounds":"A轮","money":"数千万人民币","industry":"人工智能","infoIntro":"智能风控平台","companyLogo":"https://example.com/logo.png","deatilUrl":"https://example.com/investment/303","push_time":"2026-06-05","spider_time":"2026-06-05 12:00:00","investorArray":"[{\"investorName\":\"启明创投\",\"investorType\":\"VC\"}]","historyArray":"[{\"history_rounds\":\"天使轮\",\"history_investors\":\"个人投资者\",\"history_time\":\"2025-01-01\",\"history_money\":\"数百万人民币\"}]"} `,
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

	for _, legacyPath := range []string{
		"/fullsearch/result?searchword=AI",
		"/fullsearch/index",
		"/fullsearch/lawyerDetail/101",
		"/fullsearch/companyDetail/202",
		"/fullsearch/investmentDetail/303",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, legacyPath, nil)
		srv.handleSearchCompat(rr, req, user, "full")
		if rr.Code != http.StatusGone {
			t.Fatalf("expected %s 410 Gone, got %d", legacyPath, rr.Code)
		}
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

	timelyCompanyReq := httptest.NewRequest(http.MethodGet, "/timelysearch/companyDetails?article_public_id=202", nil)
	timelyCompanyRR := httptest.NewRecorder()
	srv.handleSearchCompat(timelyCompanyRR, timelyCompanyReq, user, "timely")
	if timelyCompanyRR.Code != http.StatusOK {
		t.Fatalf("expected timely companyDetails 200, got %d", timelyCompanyRR.Code)
	}
	var timelyCompanyDetail map[string]interface{}
	if err := json.Unmarshal(timelyCompanyRR.Body.Bytes(), &timelyCompanyDetail); err != nil {
		t.Fatalf("unmarshal timely company detail: %v", err)
	}
	if timelyCompanyDetail["name"] != "星云科技有限公司" {
		t.Fatalf("unexpected timely company detail payload: %+v", timelyCompanyDetail)
	}

	timelyCompanyPageReq := httptest.NewRequest(http.MethodGet, "/timelysearch/companyDetail/202?return_to=%2Ftimelysearch%2Fresult%3Fkeyword%3DAI", nil)
	timelyCompanyPageRR := httptest.NewRecorder()
	srv.handleSearchCompat(timelyCompanyPageRR, timelyCompanyPageReq, user, "timely")
	if timelyCompanyPageRR.Code != http.StatusOK {
		t.Fatalf("expected timely company detail page 200, got %d", timelyCompanyPageRR.Code)
	}
	if !strings.Contains(timelyCompanyPageRR.Body.String(), "data-page='timelysearch/company'") || !strings.Contains(timelyCompanyPageRR.Body.String(), "/timelysearch/result?keyword=AI") {
		t.Fatalf("unexpected timely company detail page: %s", timelyCompanyPageRR.Body.String())
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
	var wsRequest map[string]any
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
			if r.URL.Query().Get("q") == "张三" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    200,
					"message": "ok",
					"data": model.SearchResult{
						Total:    1,
						Page:     1,
						PageSize: 10,
						Items: []model.Item{{
							ID:         101,
							Title:      "张三律师",
							Summary:    "擅长知识产权与资本市场业务",
							Content:    "张三律师详细介绍",
							SourceType: "lawyer",
							FromText:   "律师库",
							SourceURL:  "https://example.com/lawyer/101",
							RawPayload: `{"name":"张三","lawfirm":"金陵律师事务所","goods":"知识产权,资本市场","telephone":"13800000000","detailurl":"https://example.com/lawyer/101"}`,
							TagFlags:   "律师,知识产权",
						}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.SearchResult{
					Total:    1,
					Page:     2,
					PageSize: 20,
					Items: []model.Item{{
						ID:              301,
						Title:           "AI 行业快报",
						Summary:         "AI 行业快报摘要",
						SourceType:      "report",
						FromText:        "Flash Source",
						PublishTimeText: "刚刚",
						SourceURL:       "https://example.com/report/301",
						RawPayload:      `{"reportDate":"2026-06-05","url":"https://example.com/report/301"}`,
					}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/301":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.Item{
					ID:              301,
					Title:           "AI 行业快报",
					Summary:         "AI 行业快报摘要",
					Content:         "AI 行业快报正文",
					SourceType:      "report",
					FromText:        "Flash Source",
					PublishTime:     "2026-06-05 12:45:00",
					PublishTimeText: "刚刚",
					SourceURL:       "https://example.com/report/301",
					TagFlags:        "AI,产业",
					RawPayload:      `{"reportDate":"2026-06-05","url":"https://example.com/report/301","sourcewebsitename":"Flash Source","ner":{"org":{"OpenAI":1}}}`,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/101":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.Item{
					ID:         101,
					Title:      "张三律师",
					Summary:    "擅长知识产权与资本市场业务",
					Content:    "张三律师详细介绍",
					SourceType: "lawyer",
					FromText:   "律师库",
					SourceURL:  "https://example.com/lawyer/101",
					RawPayload: `{"name":"张三","lawfirm":"金陵律师事务所","goods":"知识产权,资本市场","telephone":"13800000000","detailurl":"https://example.com/lawyer/101"}`,
					TagFlags:   "律师,知识产权",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/301/related":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": []model.Item{{
					ID:              302,
					Title:           "AI 关联快讯",
					Summary:         "AI 关联快讯摘要",
					Content:         "AI 关联快讯正文",
					SourceType:      "headline",
					FromText:        "Flash Source",
					PublishTime:     "2026-06-05 13:00:00",
					PublishTimeText: "1分钟前",
				}},
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

	ws := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		var request map[string]any
		if err := websocket.JSON.Receive(conn, &request); err != nil {
			return
		}
		wsRequest = request
		_ = websocket.JSON.Send(conn, map[string]any{
			"eventType": "output",
			"message": mustJSONString(map[string]any{
				"outputNames": []string{"title", "abstract", "url", "publish_time", "source", "videojson", "author", "detailUrl"},
				"values":      []any{"AI 行业快报", "AI 行业快报摘要", "https://example.com/report/301", "刚刚", "Flash Source", "", "Flash Source", "/timelysearch/reportdetail/301"},
			}),
		})
		_ = websocket.JSON.Send(conn, map[string]any{
			"eventType": "finish",
			"message":   "finish",
		})
	}))
	defer ws.Close()

	srv := &Server{cfg: config.Config{ContentURL: content.URL, CrawlerURL: crawler.URL, TimelyWebSocketURL: strings.Replace(ws.URL, "http://", "ws://", 1)}, client: resty.New()}

	pageReq := httptest.NewRequest(http.MethodGet, "/timelysearch/result?keyword=AI&website_id=1&stype=flash&pageNoData=2", nil)
	pageRR := httptest.NewRecorder()
	srv.handleSearchCompat(pageRR, pageReq, nil, "timely")
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch result page 200, got %d", pageRR.Code)
	}
	pageBody := pageRR.Body.String()
	if !strings.Contains(pageBody, "即时搜索") || !strings.Contains(pageBody, "Flash Template") || !strings.Contains(pageBody, "AI 行业快报") || !strings.Contains(pageBody, "立即抓取") {
		t.Fatalf("unexpected timelysearch result page: %s", pageBody)
	}
	if !strings.Contains(pageBody, `/timelysearch/reportdetail/301`) {
		t.Fatalf("expected timelysearch result page to point to timely detail route, got %s", pageBody)
	}
	if searchQuery.Get("q") != "AI" || searchQuery.Get("page") != "2" || searchQuery.Get("source_type") != "flash" {
		t.Fatalf("unexpected timely search forwarding: %+v", searchQuery)
	}

	stypeOnlyPageReq := httptest.NewRequest(http.MethodGet, "/timelysearch/result?keyword=AI&stype=flash&pageNoData=2", nil)
	stypeOnlyPageRR := httptest.NewRecorder()
	srv.handleSearchCompat(stypeOnlyPageRR, stypeOnlyPageReq, nil, "timely")
	if stypeOnlyPageRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch stype-only result page 200, got %d", stypeOnlyPageRR.Code)
	}
	if searchQuery.Get("source_type") != "flash" {
		t.Fatalf("expected stype-only timelysearch page to forward source_type=flash, got %+v", searchQuery)
	}

	dataReq := httptest.NewRequest(http.MethodGet, "/timelysearch/data?keyword=AI&website_id=1&pageNoData=2", nil)
	dataRR := httptest.NewRecorder()
	srv.handleSearchCompat(dataRR, dataReq, nil, "timely")
	if dataRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch data 200, got %d", dataRR.Code)
	}
	var dataEnvelope struct {
		Time int64  `json:"time"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(dataRR.Body.Bytes(), &dataEnvelope); err != nil {
		t.Fatalf("unmarshal timelysearch data: %v", err)
	}
	if dataEnvelope.Time < 0 || !strings.Contains(dataEnvelope.Data, `AI 行业快报`) || !strings.Contains(dataEnvelope.Data, `Flash Source`) {
		t.Fatalf("unexpected timelysearch data body: %s", dataRR.Body.String())
	}
	if wsRequest == nil {
		t.Fatal("expected websocket request to be sent")
	}
	requestMessage, _ := wsRequest["message"].(string)
	var requestPayload map[string]any
	if err := json.Unmarshal([]byte(requestMessage), &requestPayload); err != nil {
		t.Fatalf("unmarshal websocket request payload: %v", err)
	}
	if requestPayload["keyword"] != "AI" || int(requestPayload["pageNoData"].(float64)) != 2 || requestPayload["website_id"] != "1" {
		t.Fatalf("unexpected websocket request payload: %+v", requestPayload)
	}

	stypeOnlyDataReq := httptest.NewRequest(http.MethodGet, "/timelysearch/data?keyword=AI&stype=flash&pageNoData=2", nil)
	stypeOnlyDataRR := httptest.NewRecorder()
	srv.handleSearchCompat(stypeOnlyDataRR, stypeOnlyDataReq, nil, "timely")
	if stypeOnlyDataRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch stype-only data 200, got %d", stypeOnlyDataRR.Code)
	}
	if !strings.Contains(stypeOnlyDataRR.Body.String(), `AI 行业快报`) {
		t.Fatalf("expected ws data response to contain article content, got %s", stypeOnlyDataRR.Body.String())
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/timelysearch/stream?keyword=AI&website_id=1&pageNoData=2", nil)
	streamRR := httptest.NewRecorder()
	srv.handleSearchCompat(streamRR, streamReq, nil, "timely")
	if streamRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch stream 200, got %d", streamRR.Code)
	}
	if contentType := streamRR.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", contentType)
	}
	streamBody := streamRR.Body.String()
	for _, want := range []string{"event: start", "event: item", "event: end", "AI 行业快报"} {
		if !strings.Contains(streamBody, want) {
			t.Fatalf("expected timelysearch stream to contain %q, got %s", want, streamBody)
		}
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

	detailReq := httptest.NewRequest(http.MethodGet, "/timelysearch/reportdetail/301?return_to=%2Ftimelysearch%2Fresult%3Fkeyword%3DAI", nil)
	detailRR := httptest.NewRecorder()
	srv.handleSearchCompat(detailRR, detailReq, nil, "timely")
	if detailRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch detail page 200, got %d", detailRR.Code)
	}
	detailBody := detailRR.Body.String()
	if !strings.Contains(detailBody, "公告研报详情") || !strings.Contains(detailBody, "返回搜索结果") || !strings.Contains(detailBody, "/timelysearch/result?keyword=AI") {
		t.Fatalf("unexpected timely detail page: %s", detailBody)
	}

	reportDataReq := httptest.NewRequest(http.MethodGet, "/timelysearch/getresearch-report-detail?article_public_id=301&type=report", nil)
	reportDataRR := httptest.NewRecorder()
	srv.handleSearchCompat(reportDataRR, reportDataReq, nil, "timely")
	if reportDataRR.Code != http.StatusOK {
		t.Fatalf("expected timely report detail data 200, got %d", reportDataRR.Code)
	}
	var timelyReportDetail map[string]interface{}
	if err := json.Unmarshal(reportDataRR.Body.Bytes(), &timelyReportDetail); err != nil {
		t.Fatalf("unmarshal timely report detail: %v", err)
	}
	if timelyReportDetail["title"] != "AI 行业快报" {
		t.Fatalf("unexpected timely report detail payload: %+v", timelyReportDetail)
	}

	timelyLawyerListReq := httptest.NewRequest(http.MethodGet, "/timelysearch/lawyerList?searchWord=张三&pageNum=1&pageSize=10", nil)
	timelyLawyerListRR := httptest.NewRecorder()
	srv.handleSearchCompat(timelyLawyerListRR, timelyLawyerListReq, nil, "timely")
	if timelyLawyerListRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch lawyerList 200, got %d", timelyLawyerListRR.Code)
	}
	var timelyLawyerList struct {
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(timelyLawyerListRR.Body.Bytes(), &timelyLawyerList); err != nil {
		t.Fatalf("unmarshal timely lawyerList: %v", err)
	}
	if len(timelyLawyerList.List) != 1 || timelyLawyerList.List[0]["lawfirm"] != "金陵律师事务所" {
		t.Fatalf("unexpected timely lawyerList payload: %+v", timelyLawyerList)
	}

	timelyReportTypeReq := httptest.NewRequest(http.MethodGet, "/timelysearch/reportIndustry?searchWord=AI", nil)
	timelyReportTypeRR := httptest.NewRecorder()
	srv.handleSearchCompat(timelyReportTypeRR, timelyReportTypeReq, nil, "timely")
	if timelyReportTypeRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch reportIndustry 200, got %d", timelyReportTypeRR.Code)
	}
	var timelyReportTypes []map[string]any
	if err := json.Unmarshal(timelyReportTypeRR.Body.Bytes(), &timelyReportTypes); err != nil {
		t.Fatalf("unmarshal timely reportIndustry: %v", err)
	}
	if len(timelyReportTypes) == 0 {
		t.Fatalf("expected timely reportIndustry options, got %+v", timelyReportTypes)
	}

	lawyerDetailReq := httptest.NewRequest(http.MethodGet, "/timelysearch/lawyerDetailData?article_public_id=101", nil)
	lawyerDetailRR := httptest.NewRecorder()
	srv.handleSearchCompat(lawyerDetailRR, lawyerDetailReq, nil, "timely")
	if lawyerDetailRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch lawyerDetailData 200, got %d", lawyerDetailRR.Code)
	}
	var timelyLawyerDetail struct {
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(lawyerDetailRR.Body.Bytes(), &timelyLawyerDetail); err != nil {
		t.Fatalf("unmarshal timely lawyerDetailData: %v", err)
	}
	if len(timelyLawyerDetail.List) != 1 || timelyLawyerDetail.List[0]["lawfirm"] != "金陵律师事务所" {
		t.Fatalf("unexpected timely lawyerDetailData payload: %+v", timelyLawyerDetail)
	}

	articleDetailReq := httptest.NewRequest(http.MethodPost, "/timelysearch/articleDetail", strings.NewReader(url.Values{"articleId": {"301"}}.Encode()))
	articleDetailReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	articleDetailRR := httptest.NewRecorder()
	srv.handleSearchCompat(articleDetailRR, articleDetailReq, nil, "timely")
	if articleDetailRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch articleDetail 200, got %d", articleDetailRR.Code)
	}
	var timelyDetail map[string]any
	if err := json.Unmarshal(articleDetailRR.Body.Bytes(), &timelyDetail); err != nil {
		t.Fatalf("unmarshal timely articleDetail: %v", err)
	}
	if timelyDetail["title"] != "AI 行业快报" || !strings.Contains(timelyDetail["text"].(string), "正文") {
		t.Fatalf("unexpected timely articleDetail payload: %+v", timelyDetail)
	}
	detailMap, ok := timelyDetail["detail"].(map[string]any)
	if !ok || detailMap["sourcewebsitename"] != "Flash Source" {
		t.Fatalf("unexpected timely articleDetail detail map: %+v", timelyDetail["detail"])
	}

	relatedReq := httptest.NewRequest(http.MethodPost, "/timelysearch/relatedArticles", strings.NewReader(url.Values{"articleId": {"301"}, "keywords": {"AI"}}.Encode()))
	relatedReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	relatedRR := httptest.NewRecorder()
	srv.handleSearchCompat(relatedRR, relatedReq, nil, "timely")
	if relatedRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch relatedArticles 200, got %d", relatedRR.Code)
	}
	var timelyRelated []map[string]any
	if err := json.Unmarshal(relatedRR.Body.Bytes(), &timelyRelated); err != nil {
		t.Fatalf("unmarshal timely relatedArticles: %v", err)
	}
	if len(timelyRelated) != 1 || timelyRelated[0]["article_public_id"] != "302" {
		t.Fatalf("unexpected timely relatedArticles payload: %+v", timelyRelated)
	}
	if legacyAnyString(timelyRelated[0]["detailUrl"]) == "" || legacyAnyString(timelyRelated[0]["url"]) == "" {
		t.Fatalf("expected timely relatedArticles to expose detailUrl and url, got %+v", timelyRelated[0])
	}

	infoReq := httptest.NewRequest(http.MethodGet, "/timelysearch/informationList?keyword=AI&page=2&pageSize=20&website_id=1", nil)
	infoRR := httptest.NewRecorder()
	srv.handleSearchCompat(infoRR, infoReq, nil, "timely")
	if infoRR.Code != http.StatusOK {
		t.Fatalf("expected timelysearch informationList 200, got %d", infoRR.Code)
	}
	var infoEnvelope struct {
		Data struct {
			Data []legacySearchArticle `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(infoRR.Body.Bytes(), &infoEnvelope); err != nil {
		t.Fatalf("unmarshal timely informationList: %v", err)
	}
	if len(infoEnvelope.Data.Data) != 1 || !strings.Contains(infoEnvelope.Data.Data[0].Url, "/timelysearch/reportdetail/301") {
		t.Fatalf("unexpected timely informationList payload: %+v", infoEnvelope)
	}
}
