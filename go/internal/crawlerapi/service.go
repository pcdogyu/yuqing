package crawlerapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/service"
)

type Service struct {
	cfg     config.Config
	crawler *service.Crawler
}

func NewService(cfg config.Config, crawler *service.Crawler) *Service {
	return &Service{cfg: cfg, crawler: crawler}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
	})
	r.Get("/api/v1/articles/latest", s.handleLatest)
	r.Post("/api/v1/admin/tasks/crawl", s.handleRunCrawl)
	r.Get("/api/v1/admin/tasks/crawl/runs", s.handleRuns)
	return r
}

func (s *Service) handleLatest(w http.ResponseWriter, r *http.Request) {
	limit := apiutil.IntQuery(r, "limit", 10)
	sourceType := validSourceType(r.URL.Query().Get("source_type"))
	items, err := s.crawler.LatestItems(r.Context(), limit, sourceType)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", items)
}

func (s *Service) handleRunCrawl(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Service-Token") != s.cfg.ServiceToken {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	sourceType := validSourceType(r.URL.Query().Get("source_type"))
	if sourceType == "" {
		summaries, err := s.crawler.RunAll(r.Context())
		if err != nil {
			apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summaries)
			return
		}
		apiutil.WriteJSON(w, http.StatusOK, "ok", summaries)
		return
	}
	summary, err := s.crawler.Run(r.Context(), sourceType)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summary)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", summary)
}

func (s *Service) handleRuns(w http.ResponseWriter, r *http.Request) {
	limit := apiutil.IntQuery(r, "limit", 20)
	sourceType := validSourceType(r.URL.Query().Get("source_type"))
	runs, err := s.crawler.ListCrawlRuns(r.Context(), limit, sourceType)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", runs)
}

func validSourceType(value string) string {
	switch value {
	case provider.SourceTypeFlash, provider.SourceTypeHeadline:
		return value
	default:
		return ""
	}
}
