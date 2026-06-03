package auth

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

func TestWechatLegacyFlow(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	if err := store.EnsureDefaultAdmin(ctx, "admin", "admin123"); err != nil {
		t.Fatalf("EnsureDefaultAdmin error: %v", err)
	}
	user, err := store.AuthenticateUser(ctx, "admin", "admin123")
	if err != nil {
		t.Fatalf("AuthenticateUser error: %v", err)
	}
	session, err := store.CreateSession(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	svc := NewService(config.Config{
		SessionTTL:        time.Hour,
		WechatPrivateKey:  "wechat-private-key",
		WechatAccountName: "Go 舆情系统",
		AuthURL:           "http://127.0.0.1:8081",
		GatewayWebURL:     "http://127.0.0.1",
	}, store)
	router := svc.Router()

	getBindReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/getBindQrCode?session_token="+session.Token, nil)
	getBindRR := httptest.NewRecorder()
	router.ServeHTTP(getBindRR, getBindReq)
	assertWechatJSON(t, getBindRR, http.StatusOK, http.StatusOK)
	var bindResp struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			Name      string `json:"name"`
			SceneStr  string `json:"sceneStr"`
			QRCodeURL string `json:"qrcodeUrl"`
		} `json:"data"`
	}
	decodeBody(t, getBindRR.Body.Bytes(), &bindResp)
	if bindResp.Data.SceneStr == "" || bindResp.Data.QRCodeURL == "" || bindResp.Data.Name == "" {
		t.Fatalf("unexpected bind qr response: %+v", bindResp)
	}

	checkBindReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/checkBind?session_token="+session.Token, nil)
	checkBindRR := httptest.NewRecorder()
	router.ServeHTTP(checkBindRR, checkBindReq)
	assertWechatJSON(t, checkBindRR, http.StatusOK, http.StatusInternalServerError)

	authBody := `{"sceneStr":"` + bindResp.Data.SceneStr + `","openid":"openid-bind","user_id":` + strconv.FormatInt(user.ID, 10) + `}`
	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/wechat/handleAuthorize", strings.NewReader(authBody))
	authReq.Header.Set("Content-Type", "application/json")
	authRR := httptest.NewRecorder()
	router.ServeHTTP(authRR, authReq)
	assertWechatJSON(t, authRR, http.StatusOK, http.StatusOK)

	wasBindReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/wasBind?sceneStr="+bindResp.Data.SceneStr, nil)
	wasBindRR := httptest.NewRecorder()
	router.ServeHTTP(wasBindRR, wasBindReq)
	assertWechatJSON(t, wasBindRR, http.StatusOK, http.StatusOK)

	checkBindRR = httptest.NewRecorder()
	router.ServeHTTP(checkBindRR, checkBindReq)
	assertWechatJSON(t, checkBindRR, http.StatusOK, http.StatusOK)

	tokenTime := time.Now().Unix()
	sum := sha1.Sum([]byte("openid-bind" + strconv.FormatInt(tokenTime, 10) + "wechat-private-key"))
	tokenReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/token?openid=openid-bind&time="+strconv.FormatInt(tokenTime, 10)+"&ciphering="+hex.EncodeToString(sum[:]), nil)
	tokenRR := httptest.NewRecorder()
	router.ServeHTTP(tokenRR, tokenReq)
	if tokenRR.Code != http.StatusOK || strings.TrimSpace(tokenRR.Body.String()) == "" {
		t.Fatalf("unexpected token response: code=%d body=%q", tokenRR.Code, tokenRR.Body.String())
	}

	loginChallenge, err := store.CreateWechatChallenge(ctx, model.WechatChallenge{
		SceneStr:  "yuqing:login-test",
		Purpose:   "login",
		UserID:    user.ID,
		Status:    "ready",
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateWechatChallenge error: %v", err)
	}

	loginReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/checkLogin?sceneStr="+loginChallenge.SceneStr, nil)
	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, loginReq)
	assertWechatJSON(t, loginRR, http.StatusOK, http.StatusOK)
	var loginResp struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			SessionToken string `json:"session_token"`
		} `json:"data"`
	}
	decodeBody(t, loginRR.Body.Bytes(), &loginResp)
	if loginResp.Data.SessionToken == "" {
		t.Fatalf("expected session token in login response: %+v", loginResp)
	}
}

func TestWechatPendingLoginAndBind(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	if err := store.EnsureDefaultAdmin(ctx, "admin", "admin123"); err != nil {
		t.Fatalf("EnsureDefaultAdmin error: %v", err)
	}
	user, err := store.AuthenticateUser(ctx, "admin", "admin123")
	if err != nil {
		t.Fatalf("AuthenticateUser error: %v", err)
	}
	session, err := store.CreateSession(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	svc := NewService(config.Config{
		SessionTTL:        time.Hour,
		WechatPrivateKey:  "wechat-private-key",
		WechatAccountName: "Go 舆情系统",
		AuthURL:           "http://127.0.0.1:8081",
		GatewayWebURL:     "http://127.0.0.1",
	}, store)
	router := svc.Router()

	loginReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/checkLogin?sceneStr=missing", nil)
	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, loginReq)
	assertWechatJSON(t, loginRR, http.StatusOK, http.StatusNoContent)

	bindReq := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/checkBind?session_token="+session.Token, nil)
	bindRR := httptest.NewRecorder()
	router.ServeHTTP(bindRR, bindReq)
	assertWechatJSON(t, bindRR, http.StatusOK, http.StatusInternalServerError)
}

func assertWechatJSON(t *testing.T, rr *httptest.ResponseRecorder, httpStatus int, bodyStatus int) {
	t.Helper()
	if rr.Code != httpStatus {
		t.Fatalf("unexpected http status: got %d want %d body=%s", rr.Code, httpStatus, rr.Body.String())
	}
	var envelope struct {
		Status int             `json:"status"`
		Msg    string          `json:"msg"`
		Data   json.RawMessage `json:"data"`
	}
	decodeBody(t, rr.Body.Bytes(), &envelope)
	if envelope.Status != bodyStatus {
		t.Fatalf("unexpected body status: got %d want %d body=%s", envelope.Status, bodyStatus, rr.Body.String())
	}
}

func decodeBody(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode body error: %v body=%s", err, string(body))
	}
}

func newAuthTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.db")
	store, err := sqlitestore.New(path)
	if err != nil {
		t.Fatalf("New store error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
