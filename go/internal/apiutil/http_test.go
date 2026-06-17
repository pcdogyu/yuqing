package apiutil

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteJSON(recorder, http.StatusAccepted, "queued", map[string]string{"task": "crawl"})

	if got := recorder.Code; got != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected json content type, got %q", got)
	}

	var payload struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}
	if payload.Code != http.StatusAccepted || payload.Message != "queued" || payload.Data["task"] != "crawl" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestIntQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=3&size=bad&zero=0", nil)

	if got := IntQuery(req, "page", 1); got != 3 {
		t.Fatalf("expected parsed page, got %d", got)
	}
	if got := IntQuery(req, "size", 20); got != 20 {
		t.Fatalf("expected fallback for invalid integer, got %d", got)
	}
	if got := IntQuery(req, "zero", 20); got != 20 {
		t.Fatalf("expected fallback for non-positive value, got %d", got)
	}
	if got := IntQuery(req, "missing", 7); got != 7 {
		t.Fatalf("expected fallback for missing value, got %d", got)
	}
}

func TestBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "  Bearer   token-123  ")
	if got := BearerToken(req); got != "token-123" {
		t.Fatalf("expected token, got %q", got)
	}

	req.Header.Set("Authorization", "Basic abc")
	if got := BearerToken(req); got != "" {
		t.Fatalf("expected empty token for non-bearer auth, got %q", got)
	}
}

func TestWithUserAndUserIDFromContext(t *testing.T) {
	ctx := WithUser(context.Background(), 42)

	if got := UserIDFromContext(ctx); got != 42 {
		t.Fatalf("expected user id 42, got %d", got)
	}
	if got := UserIDFromContext(context.Background()); got != 0 {
		t.Fatalf("expected zero user id for missing context value, got %d", got)
	}
}
