package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/provider"
)

func TestWriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeJSON(recorder, http.StatusCreated, "created", map[string]string{"id": "42"})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
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
		t.Fatalf("failed to decode response body: %v", err)
	}
	if payload.Code != http.StatusCreated || payload.Message != "created" || payload.Data["id"] != "42" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestIntQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=3&size=bad&zero=0", nil)

	if got := intQuery(req, "page", 1); got != 3 {
		t.Fatalf("expected parsed page, got %d", got)
	}
	if got := intQuery(req, "size", 10); got != 10 {
		t.Fatalf("expected fallback for invalid integer, got %d", got)
	}
	if got := intQuery(req, "zero", 10); got != 10 {
		t.Fatalf("expected fallback for non-positive value, got %d", got)
	}
}

func TestValidSourceType(t *testing.T) {
	if got := validSourceType(provider.SourceTypeFlash); got != provider.SourceTypeFlash {
		t.Fatalf("expected flash source type, got %q", got)
	}
	if got := validSourceType(provider.SourceTypeHeadline); got != provider.SourceTypeHeadline {
		t.Fatalf("expected headline source type, got %q", got)
	}
	if got := validSourceType("other"); got != "" {
		t.Fatalf("expected empty string for invalid source type, got %q", got)
	}
}
