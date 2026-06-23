package crawlerapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/service"
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
		apiutil.WriteHealth(w, "crawler-service", "ok", "ok")
	})
	r.Get("/healthy", func(w http.ResponseWriter, r *http.Request) {
		apiutil.WriteHealth(w, "crawler-service", "ok", "ok")
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
	templateID, templateIDProvided, templateIDInvalid := int64Query(r, "template_id")
	if templateIDInvalid {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid template_id", nil)
		return
	}
	rawSourceType := strings.TrimSpace(r.URL.Query().Get("source_type"))
	sourceType := validSourceType(rawSourceType)
	if rawSourceType != "" && sourceType == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid source_type", nil)
		return
	}
	if templateIDProvided {
		keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
		summary, err := s.crawler.RunTemplateByID(r.Context(), templateID, keyword)
		if err != nil {
			apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summary)
			return
		}
		apiutil.WriteJSON(w, http.StatusOK, "ok", summary)
		return
	}
	if sourceType == "" {
		summaries, err := s.crawler.RunAll(r.Context())
		if err != nil {
			apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summaries)
			return
		}
		apiutil.WriteJSON(w, http.StatusOK, "ok", summaries)
		return
	}
	summary, err := s.crawler.RunWithOptions(r.Context(), sourceType, crawlOptionsFromRequest(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadGateway, err.Error(), summary)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", summary)
}

func crawlOptionsFromRequest(r *http.Request) model.CrawlOptions {
	return model.CrawlOptions{
		Start:     strings.TrimSpace(r.URL.Query().Get("start")),
		End:       strings.TrimSpace(r.URL.Query().Get("end")),
		TimeField: strings.TrimSpace(r.URL.Query().Get("time_field")),
	}
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
	return provider.ValidSourceType(value)
}

func int64Query(r *http.Request, key string) (int64, bool, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, false, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, true, true
	}
	return value, true, false
}
