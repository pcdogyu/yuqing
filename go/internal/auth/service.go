package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	CreateAPIToken(context.Context, int64, string) (model.APIToken, error)
	ResolveAPIToken(context.Context, string) (model.User, error)
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
	r.Get("/api/v1/auth/me", s.handleMe)
	r.Post("/api/v1/auth/tokens", s.handleCreateToken)
	r.Get("/api/v1/auth/session", s.handleSession)
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

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", user)
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
