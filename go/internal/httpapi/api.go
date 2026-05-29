package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/service"
)

type Server struct {
	crawler *service.Crawler
}

func NewServer(cfg config.Config, crawler *service.Crawler, _ any) *Server {
	_ = cfg
	return &Server{crawler: crawler}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Get("/api/v1/items", s.handleListItems)
	r.Get("/api/v1/items/latest", s.handleLatestItems)
	r.Get("/api/v1/items/{id}", s.handleGetItem)
	r.Post("/api/v1/crawl/run", s.handleRunCrawl)
	r.Get("/api/v1/crawl/runs", s.handleListRuns)
	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, "ok", map[string]any{"status": "ok"})
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	filter := model.ArticleFilter{
		Page:       intQuery(r, "page", 1),
		PageSize:   intQuery(r, "page_size", 20),
		Keyword:    r.URL.Query().Get("keyword"),
		SourceType: validSourceType(r.URL.Query().Get("source_type")),
	}
	result, err := s.crawler.ListItems(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, "ok", result)
}

func (s *Server) handleLatestItems(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 10)
	sourceType := validSourceType(r.URL.Query().Get("source_type"))
	items, err := s.crawler.LatestItems(r.Context(), limit, sourceType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, "ok", items)
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, "invalid id", nil)
		return
	}
	item, err := s.crawler.GetItem(r.Context(), id)
	if err != nil {
		if service.IsNotFound(err) {
			writeJSON(w, http.StatusNotFound, "not found", nil)
			return
		}
		writeJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, "ok", item)
}

func (s *Server) handleRunCrawl(w http.ResponseWriter, r *http.Request) {
	sourceType := validSourceType(r.URL.Query().Get("source_type"))
	if sourceType == "" {
		summaries, err := s.crawler.RunAll(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, err.Error(), summaries)
			return
		}
		writeJSON(w, http.StatusOK, "ok", summaries)
		return
	}
	summary, err := s.crawler.Run(r.Context(), sourceType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, err.Error(), summary)
		return
	}
	writeJSON(w, http.StatusOK, "ok", summary)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	sourceType := validSourceType(r.URL.Query().Get("source_type"))
	limit := intQuery(r, "limit", 20)
	runs, err := s.crawler.ListCrawlRuns(r.Context(), limit, sourceType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, "ok", runs)
}

func writeJSON(w http.ResponseWriter, status int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    status,
		"message": message,
		"data":    data,
	})
}

func intQuery(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func validSourceType(value string) string {
	switch value {
	case provider.SourceTypeFlash, provider.SourceTypeHeadline:
		return value
	default:
		return ""
	}
}
