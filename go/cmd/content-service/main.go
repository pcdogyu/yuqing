package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/app"
	"github.com/stonedt-yuqing/go-jin10/internal/auth"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/content"
	"github.com/stonedt-yuqing/go-jin10/internal/logging"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel)

	store, err := app.NewStore(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("open store")
	}
	defer store.Close()

	authSvc := auth.NewService(cfg, store)
	contentSvc := content.NewService(cfg, store)

	router := chi.NewRouter()
	authSvc.Routes(router)
	contentSvc.Routes(router)

	server := &http.Server{
		Addr:              cfg.ContentAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	run(server, "content-service", cfg.ContentAddr)
}

func run(server *http.Server, name, addr string) {
	go func() {
		log.Info().Str("service", name).Str("addr", addr).Msg("listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Str("service", name).Msg("server stopped")
		}
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
