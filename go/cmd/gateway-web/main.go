package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/logging"
	"github.com/pcdogyu/yuqing/go/internal/portal"
)

type gatewayListener struct {
	kind        string
	server      *http.Server
	tlsCertFile string
	tlsKeyFile  string
}

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "gateway-web")

	router := portal.NewServer(cfg).Router()
	listeners, err := buildGatewayListeners(cfg, router)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid gateway-web listener configuration")
	}

	addrSummary := gatewayAddrSummary(listeners)
	app.LogStartup("gateway-web", addrSummary, cfg)
	app.LogServiceReady("gateway-web", addrSummary)

	errCh := make(chan error, len(listeners))
	for _, listener := range listeners {
		listener := listener
		go func() {
			log.Info().Str("service", "gateway-web").Str("kind", listener.kind).Str("addr", listener.server.Addr).Msg("listening")
			var err error
			if listener.kind == "https" {
				err = listener.server.ListenAndServeTLS(listener.tlsCertFile, listener.tlsKeyFile)
			} else {
				err = listener.server.ListenAndServe()
			}
			if err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("%s listener %s stopped: %w", listener.kind, listener.server.Addr, err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	var listenerErr error
	select {
	case <-sigCh:
	case listenerErr = <-errCh:
		log.Error().Err(listenerErr).Msg("gateway-web listener failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, listener := range listeners {
		_ = listener.server.Shutdown(ctx)
	}
	if listenerErr != nil {
		log.Fatal().Err(listenerErr).Msg("gateway-web stopped")
	}
}

func buildGatewayListeners(cfg config.Config, router http.Handler) ([]gatewayListener, error) {
	listeners := make([]gatewayListener, 0, len(cfg.GatewayWebHTTPAddrs)+2)
	for _, addr := range cfg.GatewayWebHTTPAddrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		listeners = append(listeners, gatewayListener{
			kind:   "http",
			server: newGatewayServer(addr, router),
		})
	}

	tlsEnabled, err := gatewayTLSEnabled(cfg)
	if err != nil {
		return nil, err
	}
	if tlsEnabled {
		tlsAddr := strings.TrimSpace(cfg.GatewayWebTLSAddr)
		listeners = append(listeners, gatewayListener{
			kind:        "https",
			server:      newGatewayServer(tlsAddr, router),
			tlsCertFile: cfg.GatewayWebTLSCertFile,
			tlsKeyFile:  cfg.GatewayWebTLSKeyFile,
		})
		if redirectAddr := strings.TrimSpace(cfg.GatewayWebRedirectAddr); redirectAddr != "" {
			listeners = append(listeners, gatewayListener{
				kind:   "redirect",
				server: newGatewayServer(redirectAddr, httpsRedirectHandler(tlsAddr)),
			})
		}
	}

	if len(listeners) == 0 {
		return nil, fmt.Errorf("at least one gateway-web listener must be configured")
	}
	return listeners, nil
}

func gatewayTLSEnabled(cfg config.Config) (bool, error) {
	tlsAddr := strings.TrimSpace(cfg.GatewayWebTLSAddr)
	certFile := strings.TrimSpace(cfg.GatewayWebTLSCertFile)
	keyFile := strings.TrimSpace(cfg.GatewayWebTLSKeyFile)
	if tlsAddr == "" || certFile == "" && keyFile == "" {
		return false, nil
	}
	if certFile == "" || keyFile == "" {
		return false, fmt.Errorf("both YUQING_GATEWAY_TLS_CERT_FILE and YUQING_GATEWAY_TLS_KEY_FILE are required for %s", tlsAddr)
	}
	return true, nil
}

func newGatewayServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func httpsRedirectHandler(tlsAddr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHost := redirectHost(r.Host, tlsAddr)
		http.Redirect(w, r, "https://"+targetHost+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}

func redirectHost(host string, tlsAddr string) string {
	host = strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	_, tlsPort, err := net.SplitHostPort(normalizeListenAddr(tlsAddr))
	if err != nil || tlsPort == "" || tlsPort == "443" {
		return host
	}
	return net.JoinHostPort(host, tlsPort)
}

func normalizeListenAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

func gatewayAddrSummary(listeners []gatewayListener) string {
	parts := make([]string, 0, len(listeners))
	for _, listener := range listeners {
		parts = append(parts, listener.kind+"="+listener.server.Addr)
	}
	return strings.Join(parts, ",")
}
