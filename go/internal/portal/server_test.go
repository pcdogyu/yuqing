package portal

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	"github.com/stonedt-yuqing/go-jin10/internal/nlp"
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

func TestHotPageCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/hot/hotpage?limit=2", nil)
	rr := httptest.NewRecorder()
	srv.handleHotPage(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "AI") {
		t.Fatalf("expected hot page to include analysis keyword, got %s", body)
	}
	if !strings.Contains(body, "/articles?mode=full&keyword=AI") {
		t.Fatalf("expected hot page to link to article search, got %s", body)
	}
	if !strings.Contains(body, "热点数据") {
		t.Fatalf("expected page title, got %s", body)
	}
}

func TestDisplayBoardCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/displayboard?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleDisplayBoard(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "综合看板") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "综合热点") || !strings.Contains(body, "微博热点") || !strings.Contains(body, "政策热点") {
		t.Fatalf("expected synthesize sections on board, got %s", body)
	}
	if !strings.Contains(body, "热点关键词") || !strings.Contains(body, "AI") {
		t.Fatalf("expected hotspot keywords on board, got %s", body)
	}
	if !strings.Contains(body, "项目快捷入口") || !strings.Contains(body, "项目一") {
		t.Fatalf("expected project links on board, got %s", body)
	}
	if !strings.Contains(body, "/fullsearch/result?searchword=AI&menuStyle=1&fulltype=8&page=1") {
		t.Fatalf("expected hot keyword search entry, got %s", body)
	}
}

func TestDisplayBoardCollection2Compat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/displayboard/collection2", nil)
	rr := httptest.NewRecorder()
	srv.handleDisplayBoardCollection2(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Data struct {
			Data []model.Item `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(envelope.Data.Data) == 0 {
		t.Fatalf("expected collection data, got %+v", envelope)
	}
}

func TestMonitorDetailCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/monitor/detail/100?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleMonitorCompat(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "监测详情") || !strings.Contains(body, "AI 观察日报") {
		t.Fatalf("expected monitor detail content, got %s", body)
	}
	if !strings.Contains(body, "/mobile/monitor?groupid=1&amp;projectid=1") {
		t.Fatalf("expected return link to mobile monitor, got %s", body)
	}
}

func TestProductManualCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	pageReq := httptest.NewRequest(http.MethodGet, "/system/productmanual/online", nil)
	pageRR := httptest.NewRecorder()
	srv.handleSystemProductManualOnline(pageRR, pageReq, map[string]any{"id": 1})
	if pageRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", pageRR.Code)
	}
	body := pageRR.Body.String()
	if !strings.Contains(body, "产品手册") || !strings.Contains(body, "/system/uploadProductManual") {
		t.Fatalf("expected product manual page content, got %s", body)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/system/uploadProductManual", nil)
	downloadRR := httptest.NewRecorder()
	srv.handleSystemUploadProductManual(downloadRR, downloadReq, map[string]any{"id": 1})
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", downloadRR.Code)
	}
	if ct := downloadRR.Header().Get("Content-Type"); !strings.Contains(ct, "application/pdf") {
		t.Fatalf("expected pdf content type, got %q", ct)
	}
	if downloadRR.Body.Len() == 0 {
		t.Fatalf("expected pdf body, got empty response")
	}
}

func TestMonitorWxGroupCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/monitor/wxGroup", nil)
	rr := httptest.NewRecorder()
	srv.handleMonitorWxGroup(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"联系我们", "/assets/images/users/wxOfficialAccount.jpg", "/assets/images/users/wxGroup.jpg", "/assets/images/expireCode.jpg", "www.stonedt.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %s", want, body)
		}
	}
}

func TestVolumePageCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/volume?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleVolume(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "声量监测") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "AI") {
		t.Fatalf("expected volume page to include hot keyword, got %s", body)
	}
	if !strings.Contains(body, "/volume/getproject?groupid=1&projectid=1") {
		t.Fatalf("expected project link, got %s", body)
	}
}

func TestMobileMonitorCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/mobile/monitor", nil)
	rr := httptest.NewRecorder()
	srv.handleMobileMonitor(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "移动端监测") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "项目分组") || !strings.Contains(body, "项目一") {
		t.Fatalf("expected project group content, got %s", body)
	}
	if !strings.Contains(body, "/mobile/monitor/detail?groupid=1&projectid=1") {
		t.Fatalf("expected detail link, got %s", body)
	}
}

func TestMobileMonitorDetailCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/mobile/monitor/detail?groupid=1&projectid=1", nil)
	rr := httptest.NewRecorder()
	srv.handleMobileMonitorDetail(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "移动端详情") {
		t.Fatalf("expected page title, got %s", body)
	}
	if !strings.Contains(body, "项目一") || !strings.Contains(body, "AI") {
		t.Fatalf("expected project details and articles, got %s", body)
	}
	if !strings.Contains(body, "预警消息") || !strings.Contains(body, "新能源 研判") {
		t.Fatalf("expected warning summary on mobile detail, got %s", body)
	}
}

func TestAnalysisCompatPage(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/analysis?projectid=1&timePeriod=7", nil)
	rr := httptest.NewRecorder()
	srv.handleAnalysisEntry(rr, req, map[string]any{"id": 1})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "监测分析") {
		t.Fatalf("expected analysis page title, got %s", body)
	}
	if !strings.Contains(body, "AI 观察日报") || !strings.Contains(body, "新能源 研判") {
		t.Fatalf("expected analysis page to include latest news, got %s", body)
	}
	if !strings.Contains(body, "AI") {
		t.Fatalf("expected analysis page to include analysis keyword, got %s", body)
	}
}

func TestAnalysisCompatJSON(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	latestReq := httptest.NewRequest(http.MethodPost, "/analysis/latestnews", strings.NewReader("projectid=1&timePeriod=7"))
	latestReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	latestRR := httptest.NewRecorder()
	srv.handleAnalysisCompatJSON(latestRR, latestReq, map[string]any{"id": 1})
	if latestRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", latestRR.Code)
	}
	var latestEnvelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(latestRR.Body.Bytes(), &latestEnvelope); err != nil {
		t.Fatalf("unmarshal latestnews response: %v", err)
	}
	if len(latestEnvelope.Data) == 0 || latestEnvelope.Data[0]["title"] == "" {
		t.Fatalf("unexpected latestnews response: %+v", latestEnvelope)
	}

	refreshReq := httptest.NewRequest(http.MethodGet, "/analysis/updateanalysisdata?projectid=1", nil)
	refreshRR := httptest.NewRecorder()
	srv.handleAnalysisCompatJSON(refreshRR, refreshReq, map[string]any{"id": 1})
	if refreshRR.Code != http.StatusOK {
		t.Fatalf("expected 200 refresh, got %d", refreshRR.Code)
	}
	if !strings.Contains(refreshRR.Body.String(), "success") {
		t.Fatalf("unexpected refresh response: %s", refreshRR.Body.String())
	}
}

func TestLegacyMailCompatibility(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	blankReq := httptest.NewRequest(http.MethodPost, "/mail/checkMailConfig", strings.NewReader(`{}`))
	blankRR := httptest.NewRecorder()
	srv.handleLegacyCheckMailConfig(blankRR, blankReq, map[string]any{"id": 1})
	if blankRR.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 before save, got %d", blankRR.Code)
	}

	saveReq := httptest.NewRequest(http.MethodPost, "/mail/saveMailConfig", strings.NewReader(`{"host":"smtp.example.com","username":"robot@example.com","password":"secret","port":"465"}`))
	saveRR := httptest.NewRecorder()
	srv.handleLegacySaveMailConfig(saveRR, saveReq, map[string]any{"id": 1})
	if saveRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on save, got %d", saveRR.Code)
	}
	var saveEnvelope struct {
		Status int                      `json:"status"`
		Msg    string                   `json:"msg"`
		Data   legacyMailConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(saveRR.Body.Bytes(), &saveEnvelope); err != nil {
		t.Fatalf("unmarshal save response: %v", err)
	}
	if saveEnvelope.Status != http.StatusOK || saveEnvelope.Data.Host != "smtp.example.com" || saveEnvelope.Data.Port != "465" {
		t.Fatalf("unexpected save response: %+v", saveEnvelope)
	}

	checkReq := httptest.NewRequest(http.MethodPost, "/mail/checkMailConfig", strings.NewReader(`{}`))
	checkRR := httptest.NewRecorder()
	srv.handleLegacyCheckMailConfig(checkRR, checkReq, map[string]any{"id": 1})
	if checkRR.Code != http.StatusOK {
		t.Fatalf("expected 200 after save, got %d", checkRR.Code)
	}
	var checkEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(checkRR.Body.Bytes(), &checkEnvelope); err != nil {
		t.Fatalf("unmarshal check response: %v", err)
	}
	if checkEnvelope.Status != http.StatusOK {
		t.Fatalf("unexpected check response: %+v", checkEnvelope)
	}

	getReq := httptest.NewRequest(http.MethodPost, "/mail/getMailConfig", strings.NewReader(`{}`))
	getRR := httptest.NewRecorder()
	srv.handleLegacyGetMailConfig(getRR, getReq, map[string]any{"id": 1})
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d", getRR.Code)
	}
	var getEnvelope struct {
		Status int                      `json:"status"`
		Data   legacyMailConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &getEnvelope); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if getEnvelope.Data.Host != "smtp.example.com" || getEnvelope.Data.Username != "robot@example.com" || getEnvelope.Data.Password != "secret" {
		t.Fatalf("unexpected get response: %+v", getEnvelope)
	}
}

func TestLegacyPopupCompatibility(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	seedPopupState(t, srv, model.PopupState{
		UserID:      1,
		Key:         legacyMobilePopupKey,
		Count:       4,
		Dismissed:   true,
		DismissedAt: timePtr(time.Now().Add(-25 * time.Hour)),
	})
	needReq := httptest.NewRequest(http.MethodGet, "/popUp/needPopUp", nil)
	needRR := httptest.NewRecorder()
	srv.handleLegacyNeedPopUp(needRR, needReq, map[string]any{"id": 1})
	if needRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on needPopUp, got %d", needRR.Code)
	}
	if got := strings.TrimSpace(needRR.Body.String()); got != "true" {
		t.Fatalf("expected true on expired popup state, got %q", got)
	}

	closeReq := httptest.NewRequest(http.MethodPost, "/popUp/close", nil)
	closeRR := httptest.NewRecorder()
	srv.handleLegacyClosePopUp(closeRR, closeReq, map[string]any{"id": 1})
	if closeRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on close, got %d", closeRR.Code)
	}
	state := getStoredPopupState(t, srv, 1, legacyMobilePopupKey)
	if !state.Dismissed || state.Count != 5 {
		t.Fatalf("expected popup to be dismissed with count 5, got %+v", state)
	}

	needRR2 := httptest.NewRecorder()
	srv.handleLegacyNeedPopUp(needRR2, needReq, map[string]any{"id": 1})
	if got := strings.TrimSpace(needRR2.Body.String()); got != "false" {
		t.Fatalf("expected false after 5th close, got %q", got)
	}

	seedPopupState(t, srv, model.PopupState{
		UserID:      0,
		Key:         legacyContactPopupKey(7),
		Dismissed:   false,
		DismissedAt: nil,
	})
	contactReq := httptest.NewRequest(http.MethodGet, "/popUp/needContact?projectId=7&total=20", nil)
	contactRR := httptest.NewRecorder()
	srv.handleLegacyNeedContact(contactRR, contactReq)
	if got := strings.TrimSpace(contactRR.Body.String()); got != "true" {
		t.Fatalf("expected true for seeded contact popup, got %q", got)
	}
	closeContactReq := httptest.NewRequest(http.MethodPost, "/popUp/closeContact?projectId=7", nil)
	closeContactRR := httptest.NewRecorder()
	srv.handleLegacyCloseContact(closeContactRR, closeContactReq)
	if closeContactRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on closeContact, got %d", closeContactRR.Code)
	}
	contactState := getStoredPopupState(t, srv, 0, legacyContactPopupKey(7))
	if !contactState.Dismissed {
		t.Fatalf("expected contact popup dismissed, got %+v", contactState)
	}

	contactRR2 := httptest.NewRecorder()
	srv.handleLegacyNeedContact(contactRR2, httptest.NewRequest(http.MethodGet, "/popUp/needContact?projectId=7&total=51", nil))
	if got := strings.TrimSpace(contactRR2.Body.String()); got != "false" {
		t.Fatalf("expected false for large result set, got %q", got)
	}
}

func TestLegacyDataMonitorCompatibility(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	var envelope struct {
		Status int    `json:"status"`
		Result string `json:"result"`
	}

	addReq := httptest.NewRequest(http.MethodPost, "/datamonitor/addfavoritedata", strings.NewReader("id=99&projectid=7&groupid=8"))
	addReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addRR := httptest.NewRecorder()
	srv.handleLegacyAddFavorite(addRR, addReq, map[string]any{"id": 1})
	if err := json.Unmarshal(addRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal add favorite response: %v", err)
	}
	if envelope.Status != http.StatusOK || envelope.Result != "success" {
		t.Fatalf("unexpected add favorite response: %+v", envelope)
	}

	addRR2 := httptest.NewRecorder()
	srv.handleLegacyAddFavorite(addRR2, addReq, map[string]any{"id": 1})
	if err := json.Unmarshal(addRR2.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal second add favorite response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("unexpected second add favorite response: %+v", envelope)
	}

	readReq := httptest.NewRequest(http.MethodPost, "/datamonitor/isread", strings.NewReader("id=99&flag=1"))
	readReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	readRR := httptest.NewRecorder()
	srv.handleLegacyReadState(readRR, readReq, map[string]any{"id": 1})
	if err := json.Unmarshal(readRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal read response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("expected 200 on mark read, got %+v", envelope)
	}

	selectReq := httptest.NewRequest(http.MethodPost, "/datamonitor/selectreadsign", strings.NewReader("id=99"))
	selectReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	selectRR := httptest.NewRecorder()
	srv.handleLegacySelectReadSign(selectRR, selectReq, map[string]any{"id": 1})
	if err := json.Unmarshal(selectRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal select read response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("expected read sign to exist, got %+v", envelope)
	}

	unreadReq := httptest.NewRequest(http.MethodPost, "/datamonitor/isread", strings.NewReader("id=99&flag=2"))
	unreadReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unreadRR := httptest.NewRecorder()
	srv.handleLegacyReadState(unreadRR, unreadReq, map[string]any{"id": 1})
	if err := json.Unmarshal(unreadRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal unread response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("expected 200 on unmark read, got %+v", envelope)
	}

	selectRR2 := httptest.NewRecorder()
	srv.handleLegacySelectReadSign(selectRR2, selectReq, map[string]any{"id": 1})
	if err := json.Unmarshal(selectRR2.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal second select read response: %v", err)
	}
	if envelope.Status != http.StatusInternalServerError {
		t.Fatalf("expected missing read sign after unmark, got %+v", envelope)
	}

	copyReq := httptest.NewRequest(http.MethodPost, "/datamonitor/copytext", strings.NewReader("id=99"))
	copyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	copyRR := httptest.NewRecorder()
	srv.handleLegacyCopyText(copyRR, copyReq, map[string]any{"id": 1})
	if err := json.Unmarshal(copyRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal copy response: %v", err)
	}
	if envelope.Status != http.StatusOK || !strings.Contains(envelope.Result, "测试标题") {
		t.Fatalf("unexpected copy response: %+v", envelope)
	}

	emotionReq := httptest.NewRequest(http.MethodPost, "/datamonitor/updateemtion", strings.NewReader("id=99&flag=3"))
	emotionReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	emotionRR := httptest.NewRecorder()
	srv.handleLegacyUpdateEmotion(emotionRR, emotionReq, map[string]any{"id": 1})
	if err := json.Unmarshal(emotionRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal emotion response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("unexpected emotion response: %+v", envelope)
	}

	if item, err := srv.fetchLegacyArticle(99, 1); err != nil || item.TagFlags != "3" {
		t.Fatalf("expected emotion to be reflected in article payload, got item=%+v err=%v", item, err)
	}

	sendReq := httptest.NewRequest(http.MethodPost, "/datamonitor/sending", strings.NewReader("id=99&projectid=7&groupid=8"))
	sendReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	sendRR := httptest.NewRecorder()
	srv.handleLegacySending(sendRR, sendReq, map[string]any{"id": 1})
	if err := json.Unmarshal(sendRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal send response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("unexpected send response: %+v", envelope)
	}

	if item, err := srv.fetchLegacyArticle(99, 1); err != nil || item.FromText != "project:7" {
		t.Fatalf("expected share channel to be reflected in article payload, got item=%+v err=%v", item, err)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/datamonitor/deletedata", strings.NewReader("id=99&flag=1"))
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteRR := httptest.NewRecorder()
	srv.handleLegacyDeleteData(deleteRR, deleteReq, map[string]any{"id": 1})
	if err := json.Unmarshal(deleteRR.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal delete response: %v", err)
	}
	if envelope.Status != http.StatusOK {
		t.Fatalf("unexpected delete response: %+v", envelope)
	}
	if _, err := srv.fetchLegacyArticle(99, 1); err == nil {
		t.Fatal("expected deleted article to become inaccessible")
	}
}

func TestLegacySystemAndUserCompat(t *testing.T) {
	srv := &Server{}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/preference?projectid=12&page=3", nil)
	srv.handleSystemSectionRedirect("preferences")(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "section=preferences") || !strings.Contains(loc, "project_id=12") || !strings.Contains(loc, "page=3") {
		t.Fatalf("unexpected redirect location: %s", loc)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/warningmsg?project_id=7&page=2&openFlag=1&keyword=%E9%92%A2%E9%93%81", nil)
	srv.handleSystemWarningMessage(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected warningmsg redirect, got %d", rr.Code)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse warningmsg redirect: %v", err)
	}
	if loc.Path != "/system" {
		t.Fatalf("unexpected redirect path: %s", loc.Path)
	}
	query := loc.Query()
	if query.Get("section") != "warningmsg" || query.Get("project_id") != "7" || query.Get("page") != "2" || query.Get("openFlag") != "1" || query.Get("keyword") != "钢铁" {
		t.Fatalf("unexpected warningmsg redirect query: %s", loc.RawQuery)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/warning?projectid=7&page=2", nil)
	srv.handleSystemWarningEdit(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected warning redirect, got %d", rr.Code)
	}
	loc, err = url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse warning redirect: %v", err)
	}
	if loc.Path != "/system" {
		t.Fatalf("unexpected warning redirect path: %s", loc.Path)
	}
	query = loc.Query()
	if query.Get("section") != "warning" || query.Get("project_id") != "7" || query.Get("page") != "2" {
		t.Fatalf("unexpected warning redirect query: %s", loc.RawQuery)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/user/123", nil)
	srv.handleUserCompat(rr, req, nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected user redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "section=account") {
		t.Fatalf("unexpected user redirect location: %s", loc)
	}
}

func TestLegacyUserJSONCompat(t *testing.T) {
	srv := &Server{}
	user := map[string]any{
		"username":     "alice",
		"display_name": "Alice",
		"email":        "alice@example.com",
		"status":       1,
		"updated_at":   "2026-06-03T10:00:00Z",
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/user/detail", nil)
	srv.handleLegacyUserDetail(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var detail struct {
		Code int `json:"code"`
		Data struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Status   int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Code != http.StatusOK || detail.Data.Username != "alice" || detail.Data.Email != "alice@example.com" || detail.Data.Status != 1 {
		t.Fatalf("unexpected detail response: %+v", detail)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/getSystemTitle", nil)
	srv.handleLegacyGetSystemTitle(rr, req, user)
	var title struct {
		Code int `json:"code"`
		Data struct {
			SystemTitle string `json:"system_title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &title); err != nil {
		t.Fatalf("decode title: %v", err)
	}
	if title.Data.SystemTitle == "" {
		t.Fatal("expected system title")
	}
}

func TestLegacyUserSaveCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()

	body := strings.NewReader("telephone=13800000000&password=secret&display_name=运营账号&email=ops@example.com&status=1")
	req := httptest.NewRequest(http.MethodPost, "/user/save", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		State   bool   `json:"state"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if !envelope.State || envelope.Message != "" {
		t.Fatalf("unexpected save response: %+v", envelope)
	}
}

func TestLegacyProjectCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/project/names?groupId=1", nil)
	srv.handleLegacyProjectNames(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected names 200, got %d", rr.Code)
	}
	var names struct {
		GroupName   string `json:"groupName"`
		ProjectName string `json:"projectName"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &names); err != nil {
		t.Fatalf("decode names: %v", err)
	}
	if names.GroupName != "组一" {
		t.Fatalf("unexpected group name: %+v", names)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/groupandproject", nil)
	srv.handleLegacyProjectGroupAndProject(rr, req, user)
	var groupsEnvelope struct {
		Code int `json:"code"`
		Data []struct {
			GroupID   string `json:"group_id"`
			GroupName string `json:"group_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &groupsEnvelope); err != nil {
		t.Fatalf("decode group list: %v", err)
	}
	if groupsEnvelope.Code != 200 || len(groupsEnvelope.Data) == 0 || groupsEnvelope.Data[0].GroupName != "组一" {
		t.Fatalf("unexpected group list: %+v", groupsEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/mkdirgroup", strings.NewReader("group_name=组二"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleLegacyProjectMkdirGroup(rr, req, user)
	if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != "success" {
		t.Fatalf("unexpected mkdirgroup response: code=%d body=%q", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/getProjectCountByGroupId?groupId=2", nil)
	srv.handleLegacyProjectGetProjectCountByGroupID(rr, req, user)
	var countEnvelope struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &countEnvelope); err != nil {
		t.Fatalf("decode count response: %v", err)
	}
	if countEnvelope.Count != 0 {
		t.Fatalf("unexpected new group count: %+v", countEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/editgroup", strings.NewReader("group_id=2&group_name=组二改"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleLegacyProjectEditGroup(rr, req, user)
	var editGroupEnvelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &editGroupEnvelope); err != nil {
		t.Fatalf("decode editgroup response: %v", err)
	}
	if editGroupEnvelope.Code != 200 {
		t.Fatalf("unexpected editgroup response: %+v", editGroupEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/listproject", strings.NewReader("groupid=1&page=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleLegacyProjectListProject(rr, req, user)
	var listEnvelope struct {
		Code      int              `json:"code"`
		TotalPage int              `json:"totalPage"`
		TotalData int              `json:"totalData"`
		Page      int              `json:"page"`
		Data      []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode project list: %v", err)
	}
	if listEnvelope.Code != 200 || listEnvelope.TotalData != 1 || len(listEnvelope.Data) != 1 {
		t.Fatalf("unexpected project list: %+v", listEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/getGroupAndProject", nil)
	srv.handleLegacyProjectGetGroupAndProject(rr, req, user)
	var treeEnvelope struct {
		Code int                           `json:"code"`
		Flag bool                          `json:"flag"`
		Data []map[string][]map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &treeEnvelope); err != nil {
		t.Fatalf("decode group tree: %v", err)
	}
	if treeEnvelope.Code != 200 || len(treeEnvelope.Data) == 0 {
		t.Fatalf("unexpected group tree: %+v", treeEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/verifygroup", nil)
	srv.handleLegacyProjectVerifyGroup(rr, req, user)
	var verifyEnvelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &verifyEnvelope); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if verifyEnvelope.Code != 200 {
		t.Fatalf("unexpected verify response: %+v", verifyEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/project/getedit?projectid=1", nil)
	srv.handleLegacyProjectGetEdit(rr, req, user)
	var editEnvelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &editEnvelope); err != nil {
		t.Fatalf("decode getedit response: %v", err)
	}
	if editEnvelope.Code != 200 || editEnvelope.Data["project_name"] != "项目一" {
		t.Fatalf("unexpected getedit response: %+v", editEnvelope)
	}

	createBody := strings.NewReader(`{"project_name":"新方案","group_id":1,"project_type":2,"subject_word":"AI,金融","stop_word":"stop","regional_word":"北京","character_word":"张三","event_word":"事件","project_description":"新建方案"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/commitproject", createBody)
	req.Header.Set("Content-Type", "application/json")
	srv.handleLegacyProjectCommitProject(rr, req, user)
	var createEnvelope struct {
		Code int `json:"code"`
		Data struct {
			GroupID   string `json:"group_id"`
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode commitproject response: %v", err)
	}
	if createEnvelope.Code != 200 || createEnvelope.Data.ProjectID != "2" {
		t.Fatalf("unexpected commitproject response: %+v", createEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/detail", strings.NewReader("projectid=2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleLegacyProjectDetail(rr, req, user)
	var detailEnvelope struct {
		ProjectName   string `json:"project_name"`
		ProjectType   int    `json:"project_type"`
		SubjectWord   string `json:"subject_word"`
		StopWord      string `json:"stop_word"`
		RegionalWord  string `json:"regional_word"`
		CharacterWord string `json:"character_word"`
		EventWord     string `json:"event_word"`
		Precise       string `json:"precise"`
		IsOpenWarning bool   `json:"isOpenWarning"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detailEnvelope); err != nil {
		t.Fatalf("decode project detail: %v", err)
	}
	if detailEnvelope.ProjectName != "新方案" || detailEnvelope.SubjectWord != "AI,金融" || detailEnvelope.ProjectType != 2 {
		t.Fatalf("unexpected detail response: %+v", detailEnvelope)
	}
	if detailEnvelope.Precise != "1" {
		t.Fatalf("expected precise 1 after create, got %+v", detailEnvelope)
	}

	editBody := strings.NewReader(`{"project_id":2,"group_id":1,"project_name":"新方案改","project_type":1,"subject_word":"AI,金融,科技","stop_word":"","project_description":"更新方案"}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/commiteditproject", editBody)
	req.Header.Set("Content-Type", "application/json")
	srv.handleLegacyProjectCommitEditProject(rr, req, user)
	var updateEnvelope struct {
		Code int `json:"code"`
		Data struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &updateEnvelope); err != nil {
		t.Fatalf("decode commitedit response: %v", err)
	}
	if updateEnvelope.Code != 200 || updateEnvelope.Data.ProjectID != "2" {
		t.Fatalf("unexpected commitedit response: %+v", updateEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/project/getedit?projectid=2", nil)
	srv.handleLegacyProjectGetEdit(rr, req, user)
	if err := json.Unmarshal(rr.Body.Bytes(), &editEnvelope); err != nil {
		t.Fatalf("decode getedit after update: %v", err)
	}
	if editEnvelope.Code != 200 || editEnvelope.Data["project_name"] != "新方案改" {
		t.Fatalf("unexpected updated getedit response: %+v", editEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/detail", strings.NewReader("projectid=2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleLegacyProjectDetail(rr, req, user)
	if err := json.Unmarshal(rr.Body.Bytes(), &detailEnvelope); err != nil {
		t.Fatalf("decode project detail after update: %v", err)
	}
	if detailEnvelope.ProjectName != "新方案改" || detailEnvelope.Precise != "0" {
		t.Fatalf("unexpected updated detail response: %+v", detailEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/delProject?groupid=1&projectid=1,2", nil)
	srv.handleLegacyProjectDelProject(rr, req, user)
	var deleteEnvelope struct {
		DelStatus int    `json:"delstatus"`
		Msg       string `json:"msg"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &deleteEnvelope); err != nil {
		t.Fatalf("decode delProject response: %v", err)
	}
	if deleteEnvelope.DelStatus != 200 {
		t.Fatalf("unexpected delProject response: %+v", deleteEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/getProjectCountByGroupId?groupId=1", nil)
	srv.handleLegacyProjectGetProjectCountByGroupID(rr, req, user)
	if err := json.Unmarshal(rr.Body.Bytes(), &countEnvelope); err != nil {
		t.Fatalf("decode count response after delete: %v", err)
	}
	if countEnvelope.Count != 0 {
		t.Fatalf("unexpected deleted group count: %+v", countEnvelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/updateSolutionGroupStatus?groupId=2", nil)
	srv.handleLegacyProjectUpdateSolutionGroupStatus(rr, req, user)
	var deleteGroupEnvelope struct {
		State   bool   `json:"state"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &deleteGroupEnvelope); err != nil {
		t.Fatalf("decode updateSolutionGroupStatus response: %v", err)
	}
	if !deleteGroupEnvelope.State {
		t.Fatalf("unexpected group delete response: %+v", deleteGroupEnvelope)
	}
}

func TestLegacyProjectBatchUpdateCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	createBody := strings.NewReader(`{"project_name":"批量删除方案","group_id":1,"project_type":1,"subject_word":"港股,AI","stop_word":"","project_description":"批量删除测试"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/project/commitproject", createBody)
	req.Header.Set("Content-Type", "application/json")
	srv.handleLegacyProjectCommitProject(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d", rr.Code)
	}

	form := url.Values{}
	form.Set("projectIds", "[1,2]")
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/batchUpdateProject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleLegacyProjectBatchUpdateProject(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected batch delete 200, got %d", rr.Code)
	}
	var envelope struct {
		State   bool   `json:"state"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode batch delete response: %v", err)
	}
	if !envelope.State || envelope.Message != "删除方案成功！" {
		t.Fatalf("unexpected batch delete response: %+v", envelope)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/getProjectCountByGroupId?groupId=1", nil)
	srv.handleLegacyProjectGetProjectCountByGroupID(rr, req, user)
	var countEnvelope struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &countEnvelope); err != nil {
		t.Fatalf("decode count response: %v", err)
	}
	if countEnvelope.Count != 0 {
		t.Fatalf("expected empty group after batch delete, got %+v", countEnvelope)
	}
}

func TestLegacyProjectKeywordsCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	user := map[string]any{"id": 1}

	createBody := strings.NewReader(`{"project_name":"关键词方案","group_id":1,"project_type":1,"subject_word":"港股,AI","stop_word":"","project_description":"关键词测试"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/project/commitproject", createBody)
	req.Header.Set("Content-Type", "application/json")
	srv.handleLegacyProjectCommitProject(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/project/keywords?page=1&size=20", nil)
	srv.handleLegacyProjectKeywords(rr, req, user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected keywords 200, got %d", rr.Code)
	}
	var envelope struct {
		Code int    `json:"code"`
		Data string `json:"data"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode keywords response: %v", err)
	}
	if envelope.Code != 200 || envelope.Msg != "获取关键词成功！" {
		t.Fatalf("unexpected keywords response: %+v", envelope)
	}
	if envelope.Data != "AI,新能源,港股,AI" {
		t.Fatalf("unexpected keywords data: %+v", envelope)
	}
}

func TestPlatformNLPCompat(t *testing.T) {
	srv, cleanup := newPortalCompatServer(t)
	defer cleanup()
	imageBytes := buildTestPNGBytes(t)

	ocrReq := httptest.NewRequest(http.MethodPost, "/platform/nlp/ocr", strings.NewReader("imageUrl="+url.QueryEscape(srv.cfg.GatewayWebURL+"/image/screenshot.png")))
	ocrReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ocrRR := httptest.NewRecorder()
	srv.handlePlatformCompat(ocrRR, ocrReq, map[string]any{"id": 1})
	if ocrRR.Code != http.StatusOK {
		t.Fatalf("expected OCR 200, got %d", ocrRR.Code)
	}
	var ocrEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   []struct {
			Data []struct {
				Text string `json:"text"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ocrRR.Body.Bytes(), &ocrEnvelope); err != nil {
		t.Fatalf("decode OCR response: %v", err)
	}
	if ocrEnvelope.Status != http.StatusOK || len(ocrEnvelope.Data) != 1 || len(ocrEnvelope.Data[0].Data) != 1 {
		t.Fatalf("unexpected OCR payload: %+v", ocrEnvelope)
	}
	if got := ocrEnvelope.Data[0].Data[0].Text; !strings.Contains(got, "screenshot") {
		t.Fatalf("expected OCR text to mention screenshot, got %q", got)
	}

	imageReq := httptest.NewRequest(http.MethodPost, "/platform/nlp/image", strings.NewReader("imageUrl="+url.QueryEscape(srv.cfg.GatewayWebURL+"/image/screenshot.png")))
	imageReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	imageRR := httptest.NewRecorder()
	srv.handlePlatformCompat(imageRR, imageReq, map[string]any{"id": 1})
	if imageRR.Code != http.StatusOK {
		t.Fatalf("expected image 200, got %d", imageRR.Code)
	}
	var imageEnvelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			Result []struct {
				Keyword string `json:"keyword"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(imageRR.Body.Bytes(), &imageEnvelope); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if imageEnvelope.Status != http.StatusOK || len(imageEnvelope.Data.Result) == 0 {
		t.Fatalf("unexpected image payload: %+v", imageEnvelope)
	}

	multipartOCRReq := newMultipartNLPRequest(t, "/platform/nlp/ocr", "images", "screenshot.png", imageBytes)
	multipartOCRRR := httptest.NewRecorder()
	srv.handlePlatformCompat(multipartOCRRR, multipartOCRReq, map[string]any{"id": 1})
	if multipartOCRRR.Code != http.StatusOK {
		t.Fatalf("expected multipart OCR 200, got %d", multipartOCRRR.Code)
	}
	var multipartOCREnvelope struct {
		Data []struct {
			Data []struct {
				Text string `json:"text"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(multipartOCRRR.Body.Bytes(), &multipartOCREnvelope); err != nil {
		t.Fatalf("decode multipart OCR response: %v", err)
	}
	if len(multipartOCREnvelope.Data) != 1 || len(multipartOCREnvelope.Data[0].Data) != 1 {
		t.Fatalf("unexpected multipart OCR payload: %+v", multipartOCREnvelope)
	}
	if got := multipartOCREnvelope.Data[0].Data[0].Text; !strings.Contains(got, "screenshot") {
		t.Fatalf("expected multipart OCR text to mention screenshot, got %q", got)
	}

	multipartImageReq := newMultipartNLPRequest(t, "/platform/nlp/image", "file", "chart.png", imageBytes)
	multipartImageRR := httptest.NewRecorder()
	srv.handlePlatformCompat(multipartImageRR, multipartImageReq, map[string]any{"id": 1})
	if multipartImageRR.Code != http.StatusOK {
		t.Fatalf("expected multipart image 200, got %d", multipartImageRR.Code)
	}
	var multipartImageEnvelope struct {
		Data struct {
			Result []struct {
				Keyword string `json:"keyword"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(multipartImageRR.Body.Bytes(), &multipartImageEnvelope); err != nil {
		t.Fatalf("decode multipart image response: %v", err)
	}
	if len(multipartImageEnvelope.Data.Result) == 0 {
		t.Fatalf("unexpected multipart image payload: %+v", multipartImageEnvelope)
	}
}

func newPortalCompatServer(t *testing.T) (*Server, func()) {
	t.Helper()
	legacyProjectMetaMu.Lock()
	legacyProjectMetaByID = map[int64]legacyProjectMeta{}
	legacyProjectMetaMu.Unlock()

	var mu sync.Mutex
	var projectMu sync.Mutex
	var authMu sync.Mutex
	var bindingMu sync.Mutex
	mailCfg := model.MailConfig{}
	popupStates := map[string]model.PopupState{}
	deletedArticles := map[int64]bool{}
	emotions := map[int64]string{}
	shareChannels := map[int64][]string{}
	opinionConditions := map[int64]model.OpinionCondition{}
	warningSettings := map[int64]model.WarningSetting{}
	createdUsers := map[string]model.User{}
	projectGroups := []model.ProjectGroup{
		{ID: 1, Name: "组一", Description: "测试项目组", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}
	projects := []model.Project{
		{ID: 1, GroupID: 1, GroupName: "组一", Name: "项目一", Keywords: "AI,新能源", Description: "测试项目", Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}
	nextGroupID := int64(2)
	nextProjectID := int64(2)
	platformBindings := map[string]model.PlatformBinding{
		"nlp:1": {
			UserID:    1,
			Kind:      "nlp",
			SecretID:  "secret-id",
			SecretKey: "secret-key",
			Bound:     true,
		},
	}
	nextUserID := int64(3)
	articles := map[int64]model.Item{
		99: {
			ID:         99,
			Title:      "测试标题",
			Content:    "测试内容",
			Summary:    "测试摘要",
			SourceType: "flash",
		},
		100: {
			ID:                 100,
			Title:              "AI 观察日报",
			Content:            "AI 相关内容",
			Summary:            "AI 摘要",
			SourceType:         "headline",
			FromText:           "新闻",
			PublishTimeText:    "2026-06-04 10:00:00",
			CapturedAt:         time.Now().UTC().Add(-2 * time.Hour),
			ProjectIDs:         []int64{1},
			ExternalSourceHost: "news.example.com",
		},
		102: {
			ID:              102,
			Title:           "微博热点追踪",
			Content:         "微博 热点 内容",
			Summary:         "微博 摘要",
			SourceType:      "weibo",
			FromText:        "微博",
			PublishTimeText: "2026-06-04 08:30:00",
			CapturedAt:      time.Now().UTC().Add(-90 * time.Minute),
		},
		103: {
			ID:              103,
			Title:           "抖音热评速览",
			Content:         "抖音 热点 内容",
			Summary:         "抖音 摘要",
			SourceType:      "douyin",
			FromText:        "抖音",
			PublishTimeText: "2026-06-04 08:20:00",
			CapturedAt:      time.Now().UTC().Add(-80 * time.Minute),
		},
		104: {
			ID:              104,
			Title:           "B站财经解读",
			Content:         "B站 热点 内容",
			Summary:         "B站 摘要",
			SourceType:      "bilibili",
			FromText:        "B站",
			PublishTimeText: "2026-06-04 08:10:00",
			CapturedAt:      time.Now().UTC().Add(-70 * time.Minute),
		},
		105: {
			ID:                 105,
			Title:              "36氪创业观察",
			Content:            "36kr 热点 内容",
			Summary:            "36kr 摘要",
			SourceType:         "36kr",
			FromText:           "36氪",
			PublishTimeText:    "2026-06-04 08:00:00",
			CapturedAt:         time.Now().UTC().Add(-60 * time.Minute),
			ExternalSourceHost: "36kr.com",
		},
		106: {
			ID:                 106,
			Title:              "国务院政策解读",
			Content:            "政策 热点 内容",
			Summary:            "政策 摘要",
			SourceType:         "gov",
			FromText:           "政策",
			PublishTimeText:    "2026-06-04 07:50:00",
			CapturedAt:         time.Now().UTC().Add(-50 * time.Minute),
			ExternalSourceHost: "gov.cn",
		},
		101: {
			ID:              101,
			Title:           "新能源 研判",
			Content:         "新能源 观察",
			Summary:         "新能源 摘要",
			SourceType:      "wechat",
			FromText:        "微信",
			PublishTimeText: "2026-06-04 09:30:00",
			CapturedAt:      time.Now().UTC().Add(-time.Hour),
			ProjectIDs:      []int64{1},
		},
	}
	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope := func(code int, message string, data any) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    code,
				"message": message,
				"data":    data,
			})
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/keywords":
			writeEnvelope(http.StatusOK, "ok", []model.KeywordHotspot{
				{Keyword: "AI", Count: 12},
				{Keyword: "新能源", Count: 8},
				{Keyword: "港股", Count: 5},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/overview":
			writeEnvelope(http.StatusOK, "ok", model.DashboardSnapshot{
				Overview: model.Overview{ArticleCount: 3, ProjectCount: 1, ReportCount: 1, AlertRuleCount: 1},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/emotions":
			writeEnvelope(http.StatusOK, "ok", model.EmotionAnalysis{ProjectID: 1, Total: 3, Buckets: []model.EmotionBucket{{Name: "positive", Count: 2, Ratio: 0.66}, {Name: "neutral", Count: 1, Ratio: 0.33}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/event-overview":
			writeEnvelope(http.StatusOK, "ok", []model.EventOverview{{Keyword: "AI", Count: 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/propagation":
			writeEnvelope(http.StatusOK, "ok", model.PropagationAnalysis{ProjectID: 1, SourceFlow: []model.PropagationNode{{Label: "flash", Count: 2}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/themes":
			writeEnvelope(http.StatusOK, "ok", []model.ThemeInsight{{Name: "AI", Count: 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-opinion/events":
			writeEnvelope(http.StatusOK, "ok", []model.PublicOpinionEvent{{Title: "AI 舆情", Keyword: "AI", Count: 2}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/public-opinion/reports":
			writeEnvelope(http.StatusOK, "ok", []model.PublicOpinionReport{{Title: "AI 报告"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/tasks/analysis/refresh":
			writeEnvelope(http.StatusOK, "ok", model.DashboardSnapshot{})
		default:
			writeEnvelope(http.StatusNotFound, "not found", nil)
		}
	}))
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeEnvelope := func(code int, message string, data any) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    code,
				"message": message,
				"data":    data,
			})
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/project-groups":
			writeEnvelope(http.StatusOK, "ok", projectGroups)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/project-groups":
			var group model.ProjectGroup
			if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			group.ID = nextGroupID
			nextGroupID++
			group.CreatedAt = time.Now().UTC()
			group.UpdatedAt = group.CreatedAt
			projectGroups = append(projectGroups, group)
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", group)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/project-groups/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/project-groups/"))
			var group model.ProjectGroup
			if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			updated := false
			for idx, candidate := range projectGroups {
				if candidate.ID == id {
					group.ID = id
					group.CreatedAt = candidate.CreatedAt
					group.UpdatedAt = time.Now().UTC()
					projectGroups[idx] = group
					updated = true
					break
				}
			}
			projectMu.Unlock()
			if !updated {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", group)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/project-groups/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/project-groups/"))
			projectMu.Lock()
			kept := projectGroups[:0]
			removed := false
			for _, candidate := range projectGroups {
				if candidate.ID == id {
					removed = true
					continue
				}
				kept = append(kept, candidate)
			}
			projectGroups = kept
			projectMu.Unlock()
			if !removed {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			projectMu.Lock()
			items := append([]model.Project(nil), projects...)
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			var project model.Project
			if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			project.ID = nextProjectID
			nextProjectID++
			project.CreatedAt = time.Now().UTC()
			project.UpdatedAt = project.CreatedAt
			if project.Status == "" {
				project.Status = "active"
			}
			for _, group := range projectGroups {
				if group.ID == project.GroupID {
					project.GroupName = group.Name
					break
				}
			}
			projects = append(projects, project)
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", project)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			projectMu.Lock()
			var item model.Project
			for _, candidate := range projects {
				if candidate.ID == id {
					item = candidate
					break
				}
			}
			projectMu.Unlock()
			if item.ID == 0 {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			var project model.Project
			if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			projectMu.Lock()
			updated := false
			for idx, candidate := range projects {
				if candidate.ID == id {
					project.ID = id
					project.CreatedAt = candidate.CreatedAt
					project.UpdatedAt = time.Now().UTC()
					if project.Status == "" {
						project.Status = candidate.Status
					}
					for _, group := range projectGroups {
						if group.ID == project.GroupID {
							project.GroupName = group.Name
							break
						}
					}
					projects[idx] = project
					updated = true
					break
				}
			}
			projectMu.Unlock()
			if !updated {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", project)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			projectMu.Lock()
			kept := projects[:0]
			removed := false
			for _, candidate := range projects {
				if candidate.ID == id {
					removed = true
					continue
				}
				kept = append(kept, candidate)
			}
			projects = kept
			projectMu.Unlock()
			if !removed {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"))
			projectMu.Lock()
			item, ok := opinionConditions[id]
			projectMu.Unlock()
			if !ok {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/opinion-conditions/"))
			var condition model.OpinionCondition
			if err := json.NewDecoder(r.Body).Decode(&condition); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			condition.ProjectID = id
			if condition.Time == 0 {
				condition.Time = 4
			}
			if condition.Emotion == "" {
				condition.Emotion = "[1,2,3]"
			}
			if condition.Sort == 0 {
				condition.Sort = 1
			}
			if condition.Matchs == 0 {
				condition.Matchs = 1
			}
			projectMu.Lock()
			opinionConditions[id] = condition
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", condition)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/system/warning-settings/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/warning-settings/"))
			projectMu.Lock()
			item, ok := warningSettings[id]
			projectMu.Unlock()
			if !ok {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/system/warning-settings/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/system/warning-settings/"))
			var setting model.WarningSetting
			if err := json.NewDecoder(r.Body).Decode(&setting); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			setting.ProjectID = id
			setting.Enabled = setting.Enabled || setting.WarningStatus == 1
			projectMu.Lock()
			warningSettings[id] = setting
			projectMu.Unlock()
			writeEnvelope(http.StatusOK, "ok", setting)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles":
			projectID := parseTestInt64(r.URL.Query().Get("project_id"))
			items := make([]model.Item, 0, len(articles))
			for _, item := range articles {
				if projectID > 0 && len(item.ProjectIDs) > 0 {
					matched := false
					for _, pid := range item.ProjectIDs {
						if pid == projectID {
							matched = true
							break
						}
					}
					if !matched {
						continue
					}
				}
				items = append(items, item)
			}
			writeEnvelope(http.StatusOK, "ok", model.ItemListResult{Items: items, Page: 1, PageSize: 20, Total: len(items)})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/mail-config":
			mu.Lock()
			cfg := mailCfg
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", cfg)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/mail-config":
			var cfg model.MailConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			cfg.UpdatedAt = time.Now().UTC()
			mu.Lock()
			mailCfg = cfg
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", cfg)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			key := r.URL.Query().Get("key")
			mu.Lock()
			state, ok := popupStates[popupStateMapKey(userID, key)]
			mu.Unlock()
			if !ok {
				state = model.PopupState{UserID: userID, Key: key}
			}
			writeEnvelope(http.StatusOK, "ok", state)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/system/popup":
			var state model.PopupState
			if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
				writeEnvelope(http.StatusBadRequest, err.Error(), nil)
				return
			}
			state.UpdatedAt = time.Now().UTC()
			if state.Dismissed && state.DismissedAt == nil {
				now := state.UpdatedAt
				state.DismissedAt = &now
			}
			mu.Lock()
			popupStates[popupStateMapKey(state.UserID, state.Key)] = state
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", state)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"))
			mu.Lock()
			item, ok := articles[id]
			deleted := deletedArticles[id]
			emotion := emotions[id]
			shares := append([]string(nil), shareChannels[id]...)
			mu.Unlock()
			if !ok || deleted {
				writeEnvelope(http.StatusNotFound, "not found", nil)
				return
			}
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			if userID > 0 {
				item.Favorited = item.Favorited
				item.Read = item.Read
			}
			if emotion != "" {
				item.TagFlags = emotion
			}
			if len(shares) > 0 {
				item.FromText = strings.Join(shares, ",")
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/related"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/related"))
			mu.Lock()
			items := make([]model.Item, 0, len(articles))
			for itemID, item := range articles {
				if itemID == id || deletedArticles[itemID] {
					continue
				}
				items = append(items, item)
			}
			mu.Unlock()
			if len(items) > 3 {
				items = items[:3]
			}
			writeEnvelope(http.StatusOK, "ok", items)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/emotion"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/emotion"))
			emotion := nonEmpty(r.URL.Query().Get("emotion"), r.URL.Query().Get("flag"))
			mu.Lock()
			emotions[id] = emotion
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]any{"emotion": emotion})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/"):
			id := parseTestInt64(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"))
			mu.Lock()
			deletedArticles[id] = true
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"deleted": true})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/favorite"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/favorite"))
			mu.Lock()
			item := articles[id]
			item.Favorited = true
			articles[id] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"favorited": true})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/read"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/read"))
			mu.Lock()
			item := articles[id]
			item.Read = true
			articles[id] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"read": true})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/read"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/read"))
			mu.Lock()
			item := articles[id]
			item.Read = false
			articles[id] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"read": false})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/articles/") && strings.HasSuffix(r.URL.Path, "/share"):
			id := parseTestInt64(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/articles/"), "/share"))
			var payload struct {
				Channel string `json:"channel"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			mu.Lock()
			shareChannels[id] = append(shareChannels[id], payload.Channel)
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]any{"shared": true, "channel": payload.Channel})
		case strings.HasPrefix(r.URL.Path, "/api/v1/platform/bindings/"):
			kind := strings.TrimPrefix(r.URL.Path, "/api/v1/platform/bindings/")
			key := kind + ":" + strconv.FormatInt(parseTestInt64(r.URL.Query().Get("user_id")), 10)
			if r.Method == http.MethodGet {
				bindingMu.Lock()
				binding, ok := platformBindings[key]
				bindingMu.Unlock()
				if !ok {
					writeEnvelope(http.StatusNotFound, "not found", nil)
					return
				}
				writeEnvelope(http.StatusOK, "ok", binding)
				return
			}
			if r.Method == http.MethodPost {
				var binding model.PlatformBinding
				if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
					writeEnvelope(http.StatusBadRequest, err.Error(), nil)
					return
				}
				binding.Kind = kind
				binding.Bound = true
				bindingMu.Lock()
				platformBindings[key] = binding
				bindingMu.Unlock()
				writeEnvelope(http.StatusOK, "ok", binding)
				return
			}
			writeEnvelope(http.StatusMethodNotAllowed, "method not allowed", nil)
		default:
			writeEnvelope(http.StatusNotFound, "not found", nil)
		}
	}))

	ocrImage := createTestImageServer(t)
	nlpServer := httptest.NewServer(nlp.NewService().Router())

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Service-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "unauthorized", "data": nil})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/users" {
			var req struct {
				Username       string `json:"username"`
				Telephone      string `json:"telephone"`
				Password       string `json:"password"`
				DisplayName    string `json:"display_name"`
				Email          string `json:"email"`
				Role           string `json:"role"`
				Status         *int   `json:"status"`
				TermOfValidity string `json:"term_of_validity"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": err.Error(), "data": nil})
				return
			}
			username := nonEmpty(req.Username, req.Telephone)
			if username == "" || strings.TrimSpace(req.Password) == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusBadRequest, "message": "invalid body", "data": nil})
				return
			}
			authMu.Lock()
			if _, ok := createdUsers[username]; ok {
				authMu.Unlock()
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusConflict, "message": "user already exists", "data": nil})
				return
			}
			id := nextUserID
			nextUserID++
			user := model.User{
				ID:          id,
				Username:    username,
				DisplayName: nonEmpty(req.DisplayName, username),
				Email:       req.Email,
				Role:        nonEmpty(req.Role, "user"),
				Status:      1,
			}
			if req.Status != nil {
				user.Status = *req.Status
			}
			if req.TermOfValidity != "" {
				if parsed, err := time.Parse(time.RFC3339, req.TermOfValidity); err == nil {
					user.TermOfValidity = parsed
				}
			}
			createdUsers[username] = user
			authMu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusCreated,
				"message": "ok",
				"data":    map[string]any{"user": user},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/wechat/getQrCode" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    http.StatusOK,
				"message": "ok",
				"data": map[string]any{
					"qrcodeUrl": "https://example.com/qr",
					"sceneStr":  "scene-1",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusNotFound, "message": "not found", "data": nil})
	}))

	srv := &Server{cfg: config.Config{ContentURL: content.URL, AuthURL: auth.URL, NLPURL: nlpServer.URL, GatewayWebURL: ocrImage.URL, AnalysisURL: analysis.URL, ServiceToken: "test-token"}, client: resty.New().SetHeader("X-Service-Token", "test-token")}
	return srv, func() {
		ocrImage.Close()
		nlpServer.Close()
		analysis.Close()
		auth.Close()
		content.Close()
	}
}

func createTestImageServer(t *testing.T) *httptest.Server {
	t.Helper()
	body := buildTestPNGBytes(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func buildTestPNGBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func newMultipartNLPRequest(t *testing.T, path, field, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create multipart form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func seedPopupState(t *testing.T, srv *Server, state model.PopupState) {
	t.Helper()
	if _, err := srv.putPopupState(state); err != nil {
		t.Fatalf("seed popup state: %v", err)
	}
}

func getStoredPopupState(t *testing.T, srv *Server, userID int64, key string) model.PopupState {
	t.Helper()
	state, ok, err := srv.getPopupState(userID, key)
	if err != nil {
		t.Fatalf("get popup state: %v", err)
	}
	if !ok {
		t.Fatalf("expected popup state to exist for user %d key %q", userID, key)
	}
	return state
}

func popupStateMapKey(userID int64, key string) string {
	return strconv.FormatInt(userID, 10) + ":" + key
}

func parseTestInt64(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func timePtr(t time.Time) *time.Time {
	return &t
}
