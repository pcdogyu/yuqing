package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestNextCompatRoutes(t *testing.T) {
	srv, cleanup := newNextCompatTestServer(t)
	defer cleanup()

	captchaReq := httptest.NewRequest(http.MethodGet, "/img/code", nil)
	captchaRR := httptest.NewRecorder()
	srv.handleCaptchaCode(captchaRR, captchaReq)
	if captchaRR.Code != http.StatusOK {
		t.Fatalf("expected captcha 200, got %d", captchaRR.Code)
	}
	if ct := captchaRR.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("unexpected captcha content type: %s", ct)
	}

	cookie := &http.Cookie{Name: sessionCookieName, Value: "session-token", Path: "/"}
	mobileReq := httptest.NewRequest(http.MethodGet, "/mobile/getGroupAndProject", nil)
	mobileReq.AddCookie(cookie)
	mobileRR := httptest.NewRecorder()
	srv.handleMobileGetGroupAndProject(mobileRR, mobileReq, map[string]any{"id": 1})
	if mobileRR.Code != http.StatusOK {
		t.Fatalf("expected mobile group 200, got %d", mobileRR.Code)
	}
	var mobileEnvelope struct {
		Status int `json:"status"`
		Data   struct {
			Data []map[string][]map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(mobileRR.Body.Bytes(), &mobileEnvelope); err != nil {
		t.Fatalf("unmarshal mobile response: %v", err)
	}
	if mobileEnvelope.Status != http.StatusOK || len(mobileEnvelope.Data.Data) == 0 {
		t.Fatalf("unexpected mobile response: %+v", mobileEnvelope)
	}

	qrReq := httptest.NewRequest(http.MethodGet, "/mobile/mobileQRCode", nil)
	qrReq.AddCookie(cookie)
	qrRR := httptest.NewRecorder()
	srv.handleMobileQRCode(qrRR, qrReq, map[string]any{"id": 1})
	if qrRR.Code != http.StatusOK {
		t.Fatalf("expected mobile qr 200, got %d", qrRR.Code)
	}
	if ct := qrRR.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("unexpected qr content type: %s", ct)
	}

	mobileUUIDReq := httptest.NewRequest(http.MethodGet, "/mobile/uuid/1-1000/"+sha1Hex(srv.cfg.ServiceToken+"1-1000"), nil)
	mobileUUIDRR := httptest.NewRecorder()
	srv.mu.Lock()
	srv.mobileQRs["1-1000"] = mobileQRCodeState{Token: "session-token", ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	srv.mu.Unlock()
	srv.handleMobileUUID(mobileUUIDRR, mobileUUIDReq)
	if mobileUUIDRR.Code != http.StatusSeeOther {
		t.Fatalf("expected mobile redirect 303, got %d", mobileUUIDRR.Code)
	}

	boardReq := httptest.NewRequest(http.MethodGet, "/displayboard", nil)
	boardReq.AddCookie(cookie)
	boardRR := httptest.NewRecorder()
	srv.handleDisplayBoard(boardRR, boardReq, map[string]any{"id": 1})
	if boardRR.Code != http.StatusOK {
		t.Fatalf("expected displayboard 200, got %d", boardRR.Code)
	}
	if !strings.Contains(boardRR.Body.String(), "综合看板") {
		t.Fatalf("expected displayboard html, got %s", boardRR.Body.String())
	}

	hotReq := httptest.NewRequest(http.MethodGet, "/hot/hotlist", nil)
	hotReq.AddCookie(cookie)
	hotRR := httptest.NewRecorder()
	srv.handleHotList(hotRR, hotReq, map[string]any{"id": 1})
	if hotRR.Code != http.StatusOK {
		t.Fatalf("expected hotlist 200, got %d", hotRR.Code)
	}
	var hotEnvelope struct {
		Code int                    `json:"code"`
		Data []model.KeywordHotspot `json:"data"`
	}
	if err := json.Unmarshal(hotRR.Body.Bytes(), &hotEnvelope); err != nil {
		t.Fatalf("unmarshal hotlist: %v", err)
	}
	if hotEnvelope.Code != 200 || len(hotEnvelope.Data) == 0 {
		t.Fatalf("unexpected hotlist payload: %+v", hotEnvelope)
	}

	volumeReq := httptest.NewRequest(http.MethodPost, "/volume/getproject", strings.NewReader("projectid=7&time_period=3"))
	volumeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	volumeReq.AddCookie(cookie)
	volumeRR := httptest.NewRecorder()
	srv.handleVolumeGetProject(volumeRR, volumeReq, map[string]any{"id": 1})
	if volumeRR.Code != http.StatusOK {
		t.Fatalf("expected volume 200, got %d", volumeRR.Code)
	}
	var volumeEnvelope struct {
		Code int `json:"code"`
		Data struct {
			ProjectName  string `json:"project_name"`
			KeywordsMood any    `json:"keywordsMood"`
		} `json:"data"`
	}
	if err := json.Unmarshal(volumeRR.Body.Bytes(), &volumeEnvelope); err != nil {
		t.Fatalf("unmarshal volume: %v", err)
	}
	if volumeEnvelope.Code != 200 || volumeEnvelope.Data.ProjectName == "" {
		t.Fatalf("unexpected volume payload: %+v", volumeEnvelope)
	}

	distReq := httptest.NewRequest(http.MethodGet, "/dist/monitor", nil)
	distRR := httptest.NewRecorder()
	srv.handleDistMonitor(distRR, distReq)
	if distRR.Code != http.StatusSeeOther {
		t.Fatalf("expected dist monitor redirect, got %d", distRR.Code)
	}

	applyPageReq := httptest.NewRequest(http.MethodGet, "/dist/yqapply?openid=o1", nil)
	applyPageRR := httptest.NewRecorder()
	srv.handleDistYqApply(applyPageRR, applyPageReq)
	if applyPageRR.Code != http.StatusOK {
		t.Fatalf("expected yqapply 200, got %d", applyPageRR.Code)
	}
	if !strings.Contains(applyPageRR.Body.String(), `name="openid"`) || !strings.Contains(applyPageRR.Body.String(), `action="/dist/applydatainfo"`) {
		t.Fatalf("expected yqapply form, got %s", applyPageRR.Body.String())
	}

	applyReq := httptest.NewRequest(http.MethodPost, "/dist/applydatainfo", strings.NewReader("openid=o1&name=n1"))
	applyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyRR := httptest.NewRecorder()
	srv.handleDistApplyDataInfo(applyRR, applyReq)
	if applyRR.Code != http.StatusOK {
		t.Fatalf("expected applydatainfo 200, got %d", applyRR.Code)
	}
	var applyEnvelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID  string `json:"openid"`
			Name    string `json:"name"`
			NextURL string `json:"next_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(applyRR.Body.Bytes(), &applyEnvelope); err != nil {
		t.Fatalf("unmarshal applydatainfo: %v", err)
	}
	if applyEnvelope.Code != 200 || applyEnvelope.Data.OpenID != "o1" || applyEnvelope.Data.Name != "n1" || !strings.Contains(applyEnvelope.Data.NextURL, "/dist/getdata?openid=o1") {
		t.Fatalf("unexpected applydatainfo payload: %+v", applyEnvelope)
	}

	approvedReq := httptest.NewRequest(http.MethodGet, "/dist/getdata?openid=o1&approved=true", nil)
	approvedRR := httptest.NewRecorder()
	srv.handleDistGetData(approvedRR, approvedReq)
	if approvedRR.Code != http.StatusSeeOther {
		t.Fatalf("expected approved getdata redirect, got %d", approvedRR.Code)
	}
}

func newNextCompatTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/session":
			if r.URL.Query().Get("session_token") != "session-token" {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 403, "msg": "forbidden", "data": map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": map[string]any{"user": map[string]any{"id": 1, "username": "alice"}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": map[string]any{}})
		}
	}))

	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{"code": 200, "msg": "ok", "data": map[string]any{}}
		switch {
		case r.URL.Path == "/api/v1/projects":
			payload["data"] = []model.Project{{ID: 7, GroupID: 3, GroupName: "A组", Name: "项目A", Keywords: "alpha,beta", Status: "active"}}
		case r.URL.Path == "/api/v1/project-groups":
			payload["data"] = []model.ProjectGroup{{ID: 3, Name: "A组"}}
		case r.URL.Path == "/api/v1/articles":
			payload["data"] = model.ItemListResult{Items: []model.Item{{ID: 1, Title: "文章1", SourceType: "flash", Favorited: true}}, Total: 1}
		case r.URL.Path == "/api/v1/system/notices":
			payload["data"] = []model.SystemNotice{{Title: "公告1", CreatedAt: time.Now().UTC()}}
		case r.URL.Path == "/api/v1/system/task-runs":
			payload["data"] = []model.TaskRun{{TaskName: "analysis:refresh", Status: "success", Message: "ok", StartedAt: time.Now().UTC()}}
		default:
			payload["data"] = map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))

	analysis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{"code": 200, "msg": "ok", "data": map[string]any{}}
		switch {
		case r.URL.Path == "/api/v1/analysis/overview":
			payload["data"] = model.DashboardSnapshot{Overview: model.Overview{ArticleCount: 1, ProjectCount: 1, ReportCount: 1, AlertRuleCount: 1}, Keywords: []model.KeywordHotspot{{Keyword: "热词", Count: 8}}}
		case r.URL.Path == "/api/v1/analysis/emotions":
			payload["data"] = model.EmotionAnalysis{ProjectID: 7, Total: 3, Buckets: []model.EmotionBucket{{Name: "正面", Count: 2, Ratio: 0.66}}}
		case r.URL.Path == "/api/v1/analysis/event-overview":
			payload["data"] = []model.EventOverview{{ProjectID: 7, ProjectName: "项目A", Keyword: "事件", Count: 2}}
		case r.URL.Path == "/api/v1/analysis/propagation":
			payload["data"] = model.PropagationAnalysis{ProjectID: 7, Trend: []model.TrendPoint{{Label: "d1", Count: 1}}}
		case r.URL.Path == "/api/v1/analysis/themes":
			payload["data"] = []model.ThemeInsight{{Name: "主题1", Count: 1}}
		case r.URL.Path == "/api/v1/analysis/keywords":
			payload["data"] = []model.KeywordHotspot{{Keyword: "热词", Count: 8}}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))

	srv := NewServer(config.Config{
		GatewayWebURL: auth.URL,
		AuthURL:       auth.URL,
		ContentURL:    content.URL,
		CrawlerURL:    content.URL,
		AnalysisURL:   analysis.URL,
		ServiceToken:  "test-token",
	})
	return srv, func() {
		auth.Close()
		content.Close()
		analysis.Close()
	}
}
