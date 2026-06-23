package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresGatewayWeb(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("gateway-web"`,
		`portal.NewServer(cfg).Router()`,
	)
}

func TestBuildGatewayListenersSupportsHTTPRedirectAndTLS(t *testing.T) {
	router := http.NewServeMux()
	cfg := config.Config{
		GatewayWebHTTPAddrs:    []string{":8079"},
		GatewayWebRedirectAddr: ":80",
		GatewayWebTLSAddr:      ":443",
		GatewayWebTLSCertFile:  "cert.pem",
		GatewayWebTLSKeyFile:   "key.pem",
	}

	listeners, err := buildGatewayListeners(cfg, router)
	if err != nil {
		t.Fatalf("build listeners: %v", err)
	}
	if len(listeners) != 3 {
		t.Fatalf("expected 3 listeners, got %d", len(listeners))
	}
	want := map[string]string{
		"http":     ":8079",
		"https":    ":443",
		"redirect": ":80",
	}
	for _, listener := range listeners {
		if want[listener.kind] != listener.server.Addr {
			t.Fatalf("unexpected listener %s addr %q", listener.kind, listener.server.Addr)
		}
	}
}

func TestBuildGatewayListenersSkipsTLSWithoutCertificatePair(t *testing.T) {
	router := http.NewServeMux()
	cfg := config.Config{
		GatewayWebHTTPAddrs:    []string{":8079"},
		GatewayWebRedirectAddr: ":80",
		GatewayWebTLSAddr:      ":443",
	}

	listeners, err := buildGatewayListeners(cfg, router)
	if err != nil {
		t.Fatalf("build listeners: %v", err)
	}
	if len(listeners) != 1 || listeners[0].kind != "http" || listeners[0].server.Addr != ":8079" {
		t.Fatalf("expected only :8079 http listener, got %#v", listeners)
	}
}

func TestBuildGatewayListenersRequiresCompleteCertificatePair(t *testing.T) {
	router := http.NewServeMux()
	cfg := config.Config{
		GatewayWebHTTPAddrs:  []string{":8079"},
		GatewayWebTLSAddr:    ":443",
		GatewayWebTLSKeyFile: "key.pem",
	}

	if _, err := buildGatewayListeners(cfg, router); err == nil {
		t.Fatalf("expected missing certificate error")
	}
}

func TestHTTPSRedirectHandlerPreservesPathAndQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com:80/search?q=AI", nil)
	rec := httptest.NewRecorder()

	httpsRedirectHandler(":9443").ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected permanent redirect, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://example.com:9443/search?q=AI" {
		t.Fatalf("unexpected redirect location %q", got)
	}
}
