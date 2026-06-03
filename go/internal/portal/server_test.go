package portal

import (
	"encoding/json"
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

	for _, tc := range []struct {
		name string
		req  *http.Request
		fn   func(http.ResponseWriter, *http.Request, any)
	}{
		{name: "update", req: httptest.NewRequest(http.MethodPost, "/datamonitor/updateemtion", strings.NewReader("id=99&flag=1")), fn: srv.handleLegacyUpdateEmotion},
		{name: "delete", req: httptest.NewRequest(http.MethodPost, "/datamonitor/deletedata", strings.NewReader("id=99&flag=1")), fn: srv.handleLegacyDeleteData},
		{name: "send", req: httptest.NewRequest(http.MethodPost, "/datamonitor/sending", strings.NewReader("id=99&projectid=7&groupid=8")), fn: srv.handleLegacySending},
	} {
		tc.req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		tc.fn(rr, tc.req, map[string]any{"id": 1})
		if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("unmarshal %s response: %v", tc.name, err)
		}
		if envelope.Status != http.StatusOK {
			t.Fatalf("unexpected %s response: %+v", tc.name, envelope)
		}
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

func newPortalCompatServer(t *testing.T) (*Server, func()) {
	t.Helper()

	var mu sync.Mutex
	mailCfg := model.MailConfig{}
	popupStates := map[string]model.PopupState{}
	articles := map[int64]model.Item{
		99: {
			ID:         99,
			Title:      "测试标题",
			Content:    "测试内容",
			Summary:    "测试摘要",
			SourceType: "flash",
		},
	}
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/articles/99":
			mu.Lock()
			item := articles[99]
			mu.Unlock()
			userID := parseTestInt64(r.URL.Query().Get("user_id"))
			if userID > 0 {
				item.Favorited = item.Favorited
				item.Read = item.Read
			}
			writeEnvelope(http.StatusOK, "ok", item)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/articles/99/favorite":
			mu.Lock()
			item := articles[99]
			item.Favorited = true
			articles[99] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"favorited": true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/articles/99/read":
			mu.Lock()
			item := articles[99]
			item.Read = true
			articles[99] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"read": true})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/articles/99/read":
			mu.Lock()
			item := articles[99]
			item.Read = false
			articles[99] = item
			mu.Unlock()
			writeEnvelope(http.StatusOK, "ok", map[string]bool{"read": false})
		default:
			writeEnvelope(http.StatusNotFound, "not found", nil)
		}
	}))

	srv := &Server{cfg: config.Config{ContentURL: content.URL}, client: resty.New()}
	return srv, content.Close
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
