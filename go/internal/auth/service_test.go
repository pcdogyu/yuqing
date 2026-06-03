package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func TestJSONNewDecoderDisallowsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"alice","extra":"nope"}`))

	var payload struct {
		Username string `json:"username"`
	}
	err := jsonNewDecoder(req).Decode(&payload)
	if err == nil {
		t.Fatal("expected decode error for unknown field")
	}
}

func TestJSONNewDecoderAcceptsKnownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"alice"}`))

	var payload struct {
		Username string `json:"username"`
	}
	if err := jsonNewDecoder(req).Decode(&payload); err != nil {
		t.Fatalf("expected decode success, got %v", err)
	}
	if payload.Username != "alice" {
		t.Fatalf("expected username alice, got %q", payload.Username)
	}
}

func TestHandleUpdateUserPassword(t *testing.T) {
	store := newFakeAuthStore()
	svc := NewService(config.Config{}, store)
	router := svc.Router()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/1/password?session_token=session-1", strings.NewReader(`{"old_password":"old-secret","new_password":"new-secret"}`))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var envelope struct {
		Code int `json:"code"`
		Data map[string]bool
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode success response: %v", err)
	}
	if envelope.Code != http.StatusOK || !envelope.Data["updated"] {
		t.Fatalf("unexpected success response: %+v", envelope)
	}
	if got := store.passwords["alice"]; got != "new-secret" {
		t.Fatalf("expected updated password, got %q", got)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/1/password?session_token=session-1", strings.NewReader(`{"old_password":"wrong","new_password":"new-secret-2"}`))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong old password, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/2/password?session_token=session-1", strings.NewReader(`{"old_password":"old-secret","new_password":"new-secret-3"}`))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin changing another user, got %d", rr.Code)
	}
}

type fakeAuthStore struct {
	users      map[int64]model.User
	sessions   map[string]model.Session
	passwords  map[string]string
	updateErr  error
	lastUpdate int64
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		users: map[int64]model.User{
			1: {ID: 1, Username: "alice", Role: "user", Status: 1},
			2: {ID: 2, Username: "bob", Role: "admin", Status: 1},
		},
		sessions: map[string]model.Session{
			"session-1": {Token: "session-1", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)},
			"session-2": {Token: "session-2", UserID: 2, ExpiresAt: time.Now().Add(time.Hour)},
		},
		passwords: map[string]string{
			"alice": "old-secret",
			"bob":   "admin-secret",
		},
	}
}

func (f *fakeAuthStore) EnsureDefaultAdmin(context.Context, string, string) error { return nil }
func (f *fakeAuthStore) EnsureSeedData(context.Context) error { return nil }
func (f *fakeAuthStore) AuthenticateUser(_ context.Context, username, password string) (model.User, error) {
	if got := f.passwords[username]; got != password {
		return model.User{}, errors.New("invalid credentials")
	}
	for _, user := range f.users {
		if user.Username == username {
			return user, nil
		}
	}
	return model.User{}, errors.New("not found")
}
func (f *fakeAuthStore) CreateSession(context.Context, int64, time.Duration) (model.Session, error) {
	return model.Session{}, errors.New("unused")
}
func (f *fakeAuthStore) GetSession(_ context.Context, token string) (model.Session, error) {
	session, ok := f.sessions[token]
	if !ok {
		return model.Session{}, errors.New("not found")
	}
	return session, nil
}
func (f *fakeAuthStore) DeleteSession(context.Context, string) error { return nil }
func (f *fakeAuthStore) GetUserByID(_ context.Context, id int64) (model.User, error) {
	user, ok := f.users[id]
	if !ok {
		return model.User{}, errors.New("not found")
	}
	return user, nil
}
func (f *fakeAuthStore) UpdateUserProfile(context.Context, int64, model.UserProfileUpdate) (model.User, error) {
	return model.User{}, errors.New("unused")
}
func (f *fakeAuthStore) UpdateUserPassword(_ context.Context, userID int64, password string) error {
	user, ok := f.users[userID]
	if !ok {
		return errors.New("not found")
	}
	f.passwords[user.Username] = password
	f.lastUpdate = userID
	return f.updateErr
}
func (f *fakeAuthStore) CreateAPIToken(context.Context, int64, string) (model.APIToken, error) {
	return model.APIToken{}, errors.New("unused")
}
func (f *fakeAuthStore) ResolveAPIToken(context.Context, string) (model.User, error) {
	return model.User{}, errors.New("unused")
}
func (f *fakeAuthStore) CreateCaptcha(context.Context, time.Duration) (model.Captcha, error) {
	return model.Captcha{}, errors.New("unused")
}
func (f *fakeAuthStore) VerifyCaptcha(context.Context, string, string) error { return errors.New("unused") }
func (f *fakeAuthStore) CreateWechatChallenge(context.Context, model.WechatChallenge) (model.WechatChallenge, error) {
	return model.WechatChallenge{}, errors.New("unused")
}
func (f *fakeAuthStore) GetWechatChallenge(context.Context, string) (model.WechatChallenge, error) {
	return model.WechatChallenge{}, errors.New("unused")
}
func (f *fakeAuthStore) UpdateWechatChallenge(context.Context, model.WechatChallenge) (model.WechatChallenge, error) {
	return model.WechatChallenge{}, errors.New("unused")
}
func (f *fakeAuthStore) DeleteWechatChallenge(context.Context, string) error { return errors.New("unused") }
func (f *fakeAuthStore) UpsertWechatBinding(context.Context, model.WechatBinding) (model.WechatBinding, error) {
	return model.WechatBinding{}, errors.New("unused")
}
func (f *fakeAuthStore) GetWechatBindingByUserID(context.Context, int64) (model.WechatBinding, error) {
	return model.WechatBinding{}, errors.New("unused")
}
func (f *fakeAuthStore) GetWechatBindingByOpenID(context.Context, string) (model.WechatBinding, error) {
	return model.WechatBinding{}, errors.New("unused")
}
func (f *fakeAuthStore) GetUserByOpenID(context.Context, string) (model.User, error) {
	return model.User{}, errors.New("unused")
}
