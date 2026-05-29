package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
