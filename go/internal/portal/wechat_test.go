package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/config"
)

func TestWechatProxyRoutes(t *testing.T) {
	wechat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/wechat/checkLogin":
			if r.URL.Query().Get("sceneStr") != "scene-1" {
				t.Fatalf("unexpected sceneStr: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"msg":    "OK",
				"data": map[string]any{
					"session_token": "session-abc",
				},
			})
		case "/api/v1/wechat/getBindQrCode":
			if r.URL.Query().Get("session_token") != "session-abc" {
				t.Fatalf("expected session token to be forwarded, got %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"msg":    "OK",
				"data": map[string]any{
					"name":      "Go 舆情系统",
					"sceneStr":  "yuqing:bind",
					"qrcodeUrl": "data:image/svg+xml;base64,ZmFrZQ==",
				},
			})
		case "/api/v1/wechat/checkBind":
			if r.URL.Query().Get("session_token") != "session-abc" {
				t.Fatalf("expected session token to be forwarded, got %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"msg":    "OK",
				"data":   nil,
			})
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
	}))
	defer wechat.Close()

	svc := &Server{
		cfg: config.Config{
			WechatURL: wechat.URL,
		},
		client: resty.New(),
	}

	loginReq := httptest.NewRequest(http.MethodGet, "/wechat/checkLogin?sceneStr=scene-1", nil)
	loginRR := httptest.NewRecorder()
	svc.handleWechatCheckLogin(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("unexpected login status: %d body=%s", loginRR.Code, loginRR.Body.String())
	}
	if cookie := loginRR.Result().Cookies(); len(cookie) == 0 || cookie[0].Name != sessionCookieName || cookie[0].Value != "session-abc" {
		t.Fatalf("expected session cookie, got %+v", cookie)
	}

	bindReq := httptest.NewRequest(http.MethodGet, "/wechat/getBindQrCode", nil)
	bindReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-abc"})
	bindRR := httptest.NewRecorder()
	svc.handleWechatGetBindQRCode(bindRR, bindReq)
	if bindRR.Code != http.StatusOK {
		t.Fatalf("unexpected bind status: %d body=%s", bindRR.Code, bindRR.Body.String())
	}
	if !strings.Contains(bindRR.Body.String(), `"sceneStr":"yuqing:bind"`) {
		t.Fatalf("unexpected bind response: %s", bindRR.Body.String())
	}

	checkBindReq := httptest.NewRequest(http.MethodGet, "/wechat/checkBind", nil)
	checkBindReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-abc"})
	checkBindRR := httptest.NewRecorder()
	svc.handleWechatCheckBind(checkBindRR, checkBindReq)
	if checkBindRR.Code != http.StatusOK {
		t.Fatalf("unexpected checkBind status: %d body=%s", checkBindRR.Code, checkBindRR.Body.String())
	}
	if !strings.Contains(checkBindRR.Body.String(), `"status":200`) {
		t.Fatalf("unexpected checkBind response: %s", checkBindRR.Body.String())
	}
}

func TestWechatAuthURLPreservesQuery(t *testing.T) {
	svc := &Server{cfg: config.Config{WechatURL: "http://127.0.0.1:8088"}}
	req := httptest.NewRequest(http.MethodGet, "/wechat/checkLogin?sceneStr=scene-1&foo=bar", nil)
	target, err := svc.wechatAuthURL(req, "/checkLogin", "session-abc")
	if err != nil {
		t.Fatalf("wechatAuthURL error: %v", err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse target error: %v", err)
	}
	if parsed.Query().Get("sceneStr") != "scene-1" || parsed.Query().Get("foo") != "bar" || parsed.Query().Get("session_token") != "session-abc" {
		t.Fatalf("unexpected target url: %s", target)
	}
	if parsed.Host != "127.0.0.1:8088" {
		t.Fatalf("expected wechat-service host, got %s", parsed.Host)
	}
}
