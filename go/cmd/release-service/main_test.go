package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	releasesvc "github.com/pcdogyu/yuqing/go/internal/release"
)

func TestReleaseServiceServerWiring(t *testing.T) {
	cfg := config.Config{
		ReleaseAddr: ":0",
		ReleaseDir:  t.TempDir(),
		ReleaseURL:  "http://release.example.com",
	}
	server := &http.Server{
		Addr:              cfg.ReleaseAddr,
		Handler:           releasesvc.NewService(cfg).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if server.Addr != ":0" {
		t.Fatalf("server addr = %q", server.Addr)
	}
	if server.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("read header timeout = %v", server.ReadHeaderTimeout)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	server.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status = %d body=%s", rr.Code, rr.Body.String())
	}
}
