package crawlerapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidSourceType(t *testing.T) {
	if got := validSourceType("flash"); got != "flash" {
		t.Fatalf("expected flash, got %q", got)
	}
	if got := validSourceType("headline"); got != "headline" {
		t.Fatalf("expected headline, got %q", got)
	}
	if got := validSourceType("other"); got != "" {
		t.Fatalf("expected empty source type, got %q", got)
	}
}

func TestRouterHealthz(t *testing.T) {
	svc := &Service{}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	svc.Router().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}
