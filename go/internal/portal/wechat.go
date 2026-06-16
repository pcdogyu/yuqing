package portal

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
)

type legacyWechatEnvelope struct {
	Status int             `json:"status"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

func (s *Server) handleWechatGetQrCode(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatJSON(w, r, "/getQrCode", "", false)
}

func (s *Server) handleWechatGetBindQRCode(w http.ResponseWriter, r *http.Request) {
	token, ok := s.sessionTokenFromRequest(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	s.proxyWechatJSON(w, r, "/getBindQrCode", token, false)
}

func (s *Server) handleWechatCheckBind(w http.ResponseWriter, r *http.Request) {
	token, ok := s.sessionTokenFromRequest(r)
	if !ok {
		writeLegacyStatusJSON(w, http.StatusForbidden, "未登录", nil)
		return
	}
	s.proxyWechatJSON(w, r, "/checkBind", token, false)
}

func (s *Server) handleWechatWasBind(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatJSON(w, r, "/wasBind", "", false)
}

func (s *Server) handleWechatCheckLogin(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatJSON(w, r, "/checkLogin", "", true)
}

func (s *Server) handleWechatToken(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatPlain(w, r, "/token", "")
}

func (s *Server) handleWechatHandleSubscribe(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatPlain(w, r, "/handleSubscribe", "")
}

func (s *Server) handleWechatHandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatPlain(w, r, "/handleUnsubscribe", "")
}

func (s *Server) handleWechatHandleAuthorize(w http.ResponseWriter, r *http.Request) {
	s.proxyWechatPlain(w, r, "/handleAuthorize", "")
}

func (s *Server) proxyWechatJSON(w http.ResponseWriter, r *http.Request, path string, sessionToken string, setPortalSession bool) {
	targetURL, err := s.wechatAuthURL(r, path, sessionToken)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "读取请求失败", nil)
		return
	}
	req := s.client.R()
	if len(bodyBytes) > 0 {
		req.SetBody(bodyBytes)
	}
	if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" {
		req.SetHeader("Content-Type", ct)
	}
	resp, err := req.Execute(r.Method, targetURL)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusBadGateway, err.Error(), nil)
		return
	}
	if setPortalSession && resp.IsSuccess() {
		var envelope legacyWechatEnvelope
		if err := json.Unmarshal(resp.Body(), &envelope); err == nil && envelope.Status == http.StatusOK {
			var data struct {
				SessionToken string `json:"session_token"`
			}
			if len(envelope.Data) > 0 && json.Unmarshal(envelope.Data, &data) == nil && strings.TrimSpace(data.SessionToken) != "" {
				s.setPortalSessionCookie(w, data.SessionToken)
			}
		}
	}
	copyResponse(w, resp)
}

func (s *Server) proxyWechatPlain(w http.ResponseWriter, r *http.Request, path string, sessionToken string) {
	targetURL, err := s.wechatAuthURL(r, path, sessionToken)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusBadRequest, "读取请求失败", nil)
		return
	}
	req := s.client.R()
	if len(bodyBytes) > 0 {
		req.SetBody(bodyBytes)
	}
	if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" {
		req.SetHeader("Content-Type", ct)
	}
	resp, err := req.Execute(r.Method, targetURL)
	if err != nil {
		writeLegacyStatusJSON(w, http.StatusBadGateway, err.Error(), nil)
		return
	}
	copyResponse(w, resp)
}

func (s *Server) wechatAuthURL(r *http.Request, path string, sessionToken string) (string, error) {
	baseURL := strings.TrimSpace(s.cfg.WechatURL)
	if baseURL == "" {
		baseURL = s.cfg.AuthURL
	}
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v1/wechat" + path)
	if err != nil {
		return "", err
	}
	query := base.Query()
	if raw := strings.TrimSpace(r.URL.RawQuery); raw != "" {
		existing, err := url.ParseQuery(raw)
		if err != nil {
			return "", err
		}
		for key, values := range existing {
			for _, value := range values {
				query.Add(key, value)
			}
		}
	}
	if sessionToken != "" {
		query.Set("session_token", sessionToken)
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func copyResponse(w http.ResponseWriter, resp *resty.Response) {
	for key, values := range resp.Header() {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if resp.RawResponse != nil {
		w.WriteHeader(resp.StatusCode())
		_, _ = w.Write(resp.Body())
		return
	}
	w.WriteHeader(http.StatusBadGateway)
}

func (s *Server) wechatSceneURL(sceneStr string) string {
	value := strings.TrimSpace(sceneStr)
	if value == "" {
		return ""
	}
	target := strings.TrimRight(s.cfg.GatewayWebURL, "/") + "/wechat/mock/scan?sceneStr=" + url.QueryEscape(value)
	return target
}

func (s *Server) setPortalSessionCookie(w http.ResponseWriter, sessionToken string) {
	if strings.TrimSpace(sessionToken) == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func wechatSessionTokenFromEnvelope(body []byte) (string, error) {
	var envelope legacyWechatEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", err
	}
	if envelope.Status != http.StatusOK {
		return "", errors.New("not ok")
	}
	var data struct {
		SessionToken string `json:"session_token"`
	}
	if len(envelope.Data) == 0 {
		return "", nil
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return "", err
	}
	return strings.TrimSpace(data.SessionToken), nil
}

func newWechatScene(prefix string) string {
	return strings.TrimSpace(prefix) + uuid.NewString()
}

func wechatBindNameFromData(body []byte) string {
	var envelope legacyWechatEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	var data struct {
		Name string `json:"name"`
	}
	if len(envelope.Data) == 0 {
		return ""
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Name)
}
