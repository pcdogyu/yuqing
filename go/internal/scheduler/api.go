package scheduler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
)

func (w *Worker) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(wr http.ResponseWriter, r *http.Request) {
		apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]string{"status": "ok"})
	})
	r.Get("/api/v1/scheduler/jobs", w.handleListJobs)
	r.Post("/api/v1/scheduler/jobs/{name}/run", w.handleRunJob)
	return r
}

func (w *Worker) handleListJobs(wr http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(wr, http.StatusOK, "ok", w.Jobs())
}

func (w *Worker) handleRunJob(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if name == "" {
		apiutil.WriteJSON(wr, http.StatusBadRequest, "job name required", nil)
		return
	}
	if err := w.RunJobByName(r.Context(), name); err != nil {
		apiutil.WriteJSON(wr, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]string{"name": name, "status": "triggered"})
}
