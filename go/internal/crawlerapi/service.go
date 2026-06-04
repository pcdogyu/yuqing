package crawlerapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
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
	r.Post("/api/v1/admin/tasks/crawl/templates/run", s.handleRunTemplate)
	r.Post("/api/v1/admin/tasks/crawl/templates/preview", s.handlePreviewTemplate)
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
	templateID := apiutil.IntQuery(r, "template_id", 0)
	runs, err := s.crawler.ListCrawlRuns(r.Context(), limit, sourceType)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	if templateID > 0 {
		filtered := runs[:0]
		for _, run := range runs {
			if run.TemplateID == int64(templateID) {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", runs)
}

func (s *Service) handleRunTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Service-Token") != s.cfg.ServiceToken {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	var tpl model.CrawlTemplate
	if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	summary, err := s.crawler.RunTemplate(r.Context(), tpl, r.URL.Query().Get("keyword"))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summary)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", summary)
}

func (s *Service) handlePreviewTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Service-Token") != s.cfg.ServiceToken {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	var tpl model.CrawlTemplate
	if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	summary, err := s.crawler.PreviewTemplate(r.Context(), tpl, r.URL.Query().Get("keyword"))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summary)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", summary)
}

func validSourceType(value string) string {
	switch value {
	case provider.SourceTypeFlash, provider.SourceTypeHeadline:
		return value
	default:
		return ""
	}
}
