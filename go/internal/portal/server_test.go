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

func TestUserIDFromMap(t *testing.T) {
	tests := []struct {
		name string
		user any
		want int64
	}{
		{name: "float64", user: map[string]any{"id": float64(7)}, want: 7},
		{name: "int64", user: map[string]any{"id": int64(9)}, want: 9},
		{name: "json number", user: map[string]any{"id": json.Number("11")}, want: 11},
		{name: "invalid type", user: map[string]any{"id": "abc"}, want: 0},
		{name: "not map", user: "abc", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := userIDFromMap(tc.user); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestNonEmpty(t *testing.T) {
	if got := nonEmpty("", "   ", " alpha ", "beta"); got != "alpha" {
		t.Fatalf("expected first non-empty trimmed value, got %q", got)
	}
	if got := nonEmpty("", "   "); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestLegacySearchTarget(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/fullsearch/result?searchword=钢铁&project_id=7&source_type=headline&industry=能源&province=上海&city=浦东&read=read&favorite=favorited&start=2026-01-01&end=2026-01-31", nil)

	got := s.legacySearchTarget("full", req)
	gotURL, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if gotURL.Path != "/articles" {
		t.Fatalf("expected /articles, got %q", gotURL.Path)
	}

	query := gotURL.Query()
	if query.Get("mode") != "full" {
		t.Fatalf("expected mode full, got %q", query.Get("mode"))
	}
	if query.Get("keyword") != "钢铁" {
		t.Fatalf("expected keyword 钢铁, got %q", query.Get("keyword"))
	}
	if query.Get("project_id") != "7" || query.Get("source_type") != "headline" {
		t.Fatalf("expected search filters preserved, got %q", got)
	}
	if query.Get("industry") != "能源" || query.Get("province") != "上海" || query.Get("city") != "浦东" {
		t.Fatalf("expected location filters preserved, got %q", got)
	}
	if query.Get("read") != "read" || query.Get("favorite") != "favorited" {
		t.Fatalf("expected state filters preserved, got %q", got)
	}
}

func TestLegacySearchBucketsIndustry(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/full" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		result := model.SearchResult{Total: 3, PageSize: 1000}
		switch page {
		case "1":
			result.Page = 1
			result.Items = []model.Item{
				{ID: 1, RawPayload: `{"industry":"能源","province":"上海","city":"浦东","eventlable":"风电"}`},
				{ID: 2, RawPayload: `{"industry":["能源","科技"],"province":"上海","city":"徐汇","eventIndex":["风电","新能源"]}`},
			}
		case "2":
			result.Page = 2
			result.Items = []model.Item{
				{ID: 3, RawPayload: `{"industry":"金融","province":"北京","city":"朝阳","eventlable":"资本市场"}`},
			}
		default:
			result.Page = 3
			result.Items = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data":    result,
		})
	}))
	defer content.Close()

	s := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	body := `{"searchword":"能源","industryIndex":["能源"],"province":["上海"],"city":["浦东","徐汇"],"eventIndex":["风电"],"timeType":8,"times":"2026-06-01 00:00:00","timee":"2026-06-03 23:59:59"}`
	req := httptest.NewRequest(http.MethodPost, "/industry", strings.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleLegacySearchBuckets("industry")(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Data []legacyFacetBucket `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if envelope.Msg != "行业标签列表成功" {
		t.Fatalf("unexpected msg %q", envelope.Msg)
	}
	if len(envelope.Data.Data) != 4 {
		t.Fatalf("expected 4 buckets including total, got %d", len(envelope.Data.Data))
	}
	buckets := map[string]int{}
	for _, bucket := range envelope.Data.Data {
		buckets[bucket.Key] = bucket.DocCount
	}
	if buckets["能源"] != 2 || buckets["科技"] != 1 || buckets["金融"] != 1 {
		t.Fatalf("unexpected industry buckets: %+v", buckets)
	}
	if buckets["total"] != 3 {
		t.Fatalf("expected total 3, got %+v", buckets)
	}
}

func TestLegacySearchBucketsEvent(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		result := model.SearchResult{Total: 2, PageSize: 1000, Page: 1}
		result.Items = []model.Item{
			{ID: 1, RawPayload: `{"industry":"能源","province":"上海","city":"浦东","eventlable":"风电"}`, TagFlags: "风电"},
			{ID: 2, RawPayload: `{"industry":"能源","province":"上海","city":"徐汇","eventIndex":["风电","新能源"]}`, TagFlags: "新能源"},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "ok",
			"data":    result,
		})
	}))
	defer content.Close()

	s := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	body := `{"eventIndex":["风电"],"timeType":8,"times":"2026-06-01 00:00:00","timee":"2026-06-03 23:59:59"}`
	req := httptest.NewRequest(http.MethodPost, "/getevent", strings.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleLegacySearchBuckets("event")(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Data struct {
			Data []legacyFacetBucket `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(envelope.Data.Data) != 3 {
		t.Fatalf("expected 3 buckets including total, got %d", len(envelope.Data.Data))
	}
	buckets := map[string]int{}
	for _, bucket := range envelope.Data.Data {
		buckets[bucket.Key] = bucket.DocCount
	}
	if buckets["风电"] != 2 || buckets["新能源"] != 1 {
		t.Fatalf("unexpected event buckets: %+v", buckets)
	}
	if buckets["total"] != 2 {
		t.Fatalf("expected total 2, got %+v", buckets)
	}
}
