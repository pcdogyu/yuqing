package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

type Store interface {
	EnsureDefaultAdmin(context.Context, string, string) error
	EnsureSeedData(context.Context) error
	AuthenticateUser(context.Context, string, string) (model.User, error)
	CreateSession(context.Context, int64, time.Duration) (model.Session, error)
	GetSession(context.Context, string) (model.Session, error)
	DeleteSession(context.Context, string) error
	GetUserByID(context.Context, int64) (model.User, error)
	UpdateUserProfile(context.Context, int64, model.UserProfileUpdate) (model.User, error)
	CreateAPIToken(context.Context, int64, string) (model.APIToken, error)
	ResolveAPIToken(context.Context, string) (model.User, error)
	CreateCaptcha(context.Context, time.Duration) (model.Captcha, error)
	VerifyCaptcha(context.Context, string, string) error
	CreateWechatChallenge(context.Context, model.WechatChallenge) (model.WechatChallenge, error)
	GetWechatChallenge(context.Context, string) (model.WechatChallenge, error)
	UpdateWechatChallenge(context.Context, model.WechatChallenge) (model.WechatChallenge, error)
	DeleteWechatChallenge(context.Context, string) error
	UpsertWechatBinding(context.Context, model.WechatBinding) (model.WechatBinding, error)
	GetWechatBindingByUserID(context.Context, int64) (model.WechatBinding, error)
	GetWechatBindingByOpenID(context.Context, string) (model.WechatBinding, error)
	GetUserByOpenID(context.Context, string) (model.User, error)
}

type Service struct {
	cfg   config.Config
	store Store
}

func NewService(cfg config.Config, store Store) *Service {
	return &Service{cfg: cfg, store: store}
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if err := s.store.EnsureDefaultAdmin(ctx, s.cfg.DefaultAdminUser, s.cfg.DefaultAdminPass); err != nil {
		return err
	}
	return s.store.EnsureSeedData(ctx)
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	s.Routes(r)
	return r
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
	})
	r.Post("/api/v1/auth/login", s.handleLogin)
	r.Post("/api/v1/auth/logout", s.handleLogout)
	r.Get("/api/v1/auth/captcha", s.handleCaptcha)
	r.Post("/api/v1/auth/captcha/verify", s.handleVerifyCaptcha)
	r.Get("/api/v1/auth/me", s.handleMe)
	r.Post("/api/v1/auth/tokens", s.handleCreateToken)
	r.Get("/api/v1/auth/session", s.handleSession)
	r.Get("/api/v1/users/me", s.handleGetCurrentUser)
	r.Get("/api/v1/users/{id}", s.handleGetUser)
	r.Put("/api/v1/users/{id}", s.handleUpdateUser)
	r.Get("/api/v1/wechat/getQrCode", s.handleWechatGetQRCode)
	r.Get("/api/v1/wechat/getBindQrCode", s.handleWechatGetBindQRCode)
	r.Get("/api/v1/wechat/checkBind", s.handleWechatCheckBind)
	r.Get("/api/v1/wechat/wasBind", s.handleWechatWasBind)
	r.Get("/api/v1/wechat/checkLogin", s.handleWechatCheckLogin)
	r.Get("/api/v1/wechat/token", s.handleWechatToken)
	r.Post("/api/v1/wechat/handleSubscribe", s.handleWechatHandleSubscribe)
	r.Get("/api/v1/wechat/handleUnsubscribe", s.handleWechatHandleUnsubscribe)
	r.Post("/api/v1/wechat/handleAuthorize", s.handleWechatHandleAuthorize)
	r.Get("/api/v1/wechat/mock/scan", s.handleWechatMockScan)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := jsonNewDecoder(r).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	user, err := s.store.AuthenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "invalid credentials", nil)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, s.cfg.SessionTTL)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{
		"user":          user,
		"session_token": session.Token,
		"expires_at":    session.ExpiresAt,
	})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("session_token"))
	if token == "" {
		token = apiutil.BearerToken(r)
	}
	if token == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "missing session token", nil)
		return
	}
	if err := s.store.DeleteSession(r.Context(), token); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"logged_out": true})
}

func (s *Service) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	captcha, err := s.store.CreateCaptcha(r.Context(), 10*time.Minute)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	captcha.ImageSVG = renderCaptchaSVG(captcha.Code)
	captcha.Code = ""
	apiutil.WriteJSON(w, http.StatusOK, "ok", captcha)
}

func (s *Service) handleVerifyCaptcha(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	if err := jsonNewDecoder(r).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	if err := s.store.VerifyCaptcha(r.Context(), req.ID, req.Code); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid captcha", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]bool{"valid": true})
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", user)
}

func (s *Service) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	s.handleMe(w, r)
}

func (s *Service) handleGetUser(w http.ResponseWriter, r *http.Request) {
	current, err := s.userFromRequest(r)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	targetID, ok := parseID(r)
	if !ok {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid id", nil)
		return
	}
	if current.Role != "admin" && current.ID != targetID {
		apiutil.WriteJSON(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), targetID)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", user)
}

func (s *Service) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	current, err := s.userFromRequest(r)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	targetID, ok := parseID(r)
	if !ok {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid id", nil)
		return
	}
	if current.Role != "admin" && current.ID != targetID {
		apiutil.WriteJSON(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	var req model.UserProfileUpdate
	if err := jsonNewDecoder(r).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	updated, err := s.store.UpdateUserProfile(r.Context(), targetID, req)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", updated)
}

func (s *Service) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = jsonNewDecoder(r).Decode(&req)
	token, err := s.store.CreateAPIToken(r.Context(), user.ID, req.Name)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", token)
}

func (s *Service) handleSession(w http.ResponseWriter, r *http.Request) {
	sessionToken := strings.TrimSpace(r.URL.Query().Get("session_token"))
	if sessionToken == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "missing session token", nil)
		return
	}
	session, err := s.store.GetSession(r.Context(), sessionToken)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "invalid session", nil)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), session.UserID)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "invalid session", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]any{"user": user, "session": session})
}

func (s *Service) userFromRequest(r *http.Request) (model.User, error) {
	if token := strings.TrimSpace(r.URL.Query().Get("session_token")); token != "" {
		session, err := s.store.GetSession(r.Context(), token)
		if err == nil {
			return s.store.GetUserByID(r.Context(), session.UserID)
		}
	}
	if token := apiutil.BearerToken(r); token != "" {
		return s.store.ResolveAPIToken(r.Context(), token)
	}
	return model.User{}, errors.New("unauthorized")
}

func IsNotFound(err error) bool {
	return errors.Is(err, sqlitestore.ErrNotFound)
}

func jsonNewDecoder(r *http.Request) *json.Decoder {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder
}

func parseID(r *http.Request) (int64, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func renderCaptchaSVG(code string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="160" height="48" viewBox="0 0 160 48"><rect width="160" height="48" rx="8" fill="#f4efe4"/><text x="80" y="31" text-anchor="middle" font-family="monospace" font-size="24" fill="#2f4858">` + code + `</text></svg>`
}
