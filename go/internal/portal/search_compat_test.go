package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
