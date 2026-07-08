package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
)

func (w *Worker) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(wr http.ResponseWriter, r *http.Request) {
		apiutil.WriteHealth(wr, "scheduler-service", "ok", "ok")
	})
	r.Get("/healthy", func(wr http.ResponseWriter, r *http.Request) {
		apiutil.WriteHealth(wr, "scheduler-service", "ok", "ok")
	})
	r.Get("/api/v1/scheduler/jobs", w.handleListJobs)
	r.Post("/api/v1/scheduler/jobs/{name}/run", w.handleRunJob)
	r.Post("/api/v1/scheduler/stock-research/backfill", w.handleRunStockResearchBackfill)
	r.Post("/api/v1/scheduler/stock-research/pdf/parse", w.handleRunStockResearchPDFParse)
	r.Post("/api/v1/scheduler/investor-relations/backfill", w.handleRunInvestorRelationsBackfill)
	r.Post("/api/v1/scheduler/a-stock/auction/latest", w.handleRunAStockAuctionLatest)
	r.Post("/api/v1/scheduler/a-stock/auction/backfill", w.handleRunAStockAuctionBackfill)
	r.Get("/api/v1/scheduler/a-stock/trading-day", w.handleGetAStockTradingDay)
	r.Post("/api/v1/scheduler/a-stock/holdings/backfill", w.handleRunAStockHoldingsBackfill)
	r.Post("/api/v1/scheduler/a-stock/sector-fund-flow/latest", w.handleRunAStockSectorFundFlowLatest)
	return r
}

func (w *Worker) handleRunInvestorRelationsBackfill(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	opts := stockResearchCrawlOptions{
		Code:    strings.TrimSpace(r.URL.Query().Get("code")),
		Company: strings.TrimSpace(r.URL.Query().Get("company")),
		Start:   strings.TrimSpace(r.URL.Query().Get("start")),
		End:     strings.TrimSpace(r.URL.Query().Get("end")),
	}
	startedAt := time.Now().UTC()
	go w.runInvestorRelationsBackfillTask(context.Background(), opts, startedAt)
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]string{"status": "triggered"})
}

func (w *Worker) runInvestorRelationsBackfillTask(ctx context.Context, opts stockResearchCrawlOptions, startedAt time.Time) {
	err := w.runInvestorRelationsBackfill(ctx, opts)
	finishedAt := time.Now().UTC()
	status := "success"
	message := "investor relations backfill completed"
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	if recordErr := w.recordTaskRun(ctx, "investor-relations-backfill", status, message, startedAt, &finishedAt); recordErr != nil {
		log.Warn().Err(recordErr).Str("task", "investor-relations-backfill").Msg("record scheduler task run failed")
	}
	if err != nil {
		log.Error().Err(err).Str("task", "investor-relations-backfill").Msg("scheduler task failed")
		return
	}
	log.Info().Str("task", "investor-relations-backfill").Msg("scheduler task completed")
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

func (w *Worker) handleRunStockResearchBackfill(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	opts := stockResearchCrawlOptions{
		Code:    strings.TrimSpace(r.URL.Query().Get("code")),
		Company: strings.TrimSpace(r.URL.Query().Get("company")),
		Start:   strings.TrimSpace(r.URL.Query().Get("start")),
		End:     strings.TrimSpace(r.URL.Query().Get("end")),
	}
	startedAt := time.Now().UTC()
	err := w.runStockResearchBackfill(r.Context(), opts)
	finishedAt := time.Now().UTC()
	status := "success"
	message := "stock research backfill completed"
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	_ = w.recordTaskRun(r.Context(), "stock-research-backfill", status, message, startedAt, &finishedAt)
	if err != nil {
		apiutil.WriteJSON(wr, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]string{"status": "triggered"})
}

func (w *Worker) handleRunStockResearchPDFParse(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	var id int64
	if rawID := strings.TrimSpace(r.URL.Query().Get("id")); rawID != "" {
		parsedID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || parsedID <= 0 {
			apiutil.WriteJSON(wr, http.StatusBadRequest, "invalid id", nil)
			return
		}
		id = parsedID
	}
	opts := stockResearchPDFParseOptions{
		ID:      id,
		Code:    strings.TrimSpace(r.URL.Query().Get("code")),
		Company: strings.TrimSpace(r.URL.Query().Get("company")),
		Kind:    strings.TrimSpace(r.URL.Query().Get("kind")),
		Source:  strings.TrimSpace(r.URL.Query().Get("source")),
		Start:   strings.TrimSpace(r.URL.Query().Get("start")),
		End:     strings.TrimSpace(r.URL.Query().Get("end")),
		DryRun:  parseBoolQuery(r, "dry_run"),
		Force:   parseBoolQuery(r, "force"),
	}
	startedAt := time.Now().UTC()
	result, err := w.runStockResearchPDFParse(r.Context(), opts)
	finishedAt := time.Now().UTC()
	status := "success"
	message := fmt.Sprintf("stock research pdf parse completed: total=%d existing_pdf=%d need_download=%d parsed=%d no_pdf=%d no_text=%d failed=%d dry_run=%t", result.Total, result.ExistingPDF, result.NeedDownload, result.Parsed, result.NoPDF, result.NoText, result.Failed, result.DryRun)
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	_ = w.recordTaskRun(r.Context(), "stock-research-pdf-parse", status, message, startedAt, &finishedAt)
	if err != nil {
		apiutil.WriteJSON(wr, http.StatusInternalServerError, err.Error(), result)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]any{"status": "triggered", "result": result})
}

func parseBoolQuery(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (w *Worker) handleRunAStockAuctionBackfill(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	days := apiutil.IntQuery(r, "days", 30)
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	startedAt := time.Now().UTC()
	result, err := w.runAStockAuctionBackfill(r.Context(), days, start, end)
	finishedAt := time.Now().UTC()
	status := "success"
	message := fmt.Sprintf("a-stock auction backfill completed: days=%d succeeded=%d skipped=%d failed=%d", result.Days, result.Succeeded, result.Skipped, result.Failed)
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	_ = w.recordTaskRun(r.Context(), "a-stock-auction-backfill", status, message, startedAt, &finishedAt)
	if err != nil {
		apiutil.WriteJSON(wr, http.StatusInternalServerError, err.Error(), result)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]any{"status": "triggered", "result": result})
}

func (w *Worker) handleRunAStockAuctionLatest(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	startedAt := time.Now().UTC()
	go w.runAStockAuctionLatestTask(context.Background(), startedAt)
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]any{"status": "triggered"})
}

func (w *Worker) handleGetAStockTradingDay(wr http.ResponseWriter, r *http.Request) {
	status, err := w.loadAStockTradingDayStatus(r.Context(), strings.TrimSpace(r.URL.Query().Get("date")))
	if err != nil {
		apiutil.WriteJSON(wr, http.StatusServiceUnavailable, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", status)
}

func (w *Worker) runAStockAuctionLatestTask(ctx context.Context, startedAt time.Time) {
	result, err := w.runAStockAuctionLatest(ctx)
	finishedAt := time.Now().UTC()
	status := "success"
	message := fmt.Sprintf("a-stock auction latest completed: date=%s total=%d ok=%d", result.Date, result.Total, result.OK)
	if err != nil {
		status = "failed"
		message = err.Error()
	} else if result.Skipped {
		status = "failed"
		message = fmt.Sprintf("a-stock auction latest skipped for %s: %s", result.Date, result.Message)
		err = fmt.Errorf("%s", message)
	}
	if recordErr := w.recordTaskRun(ctx, "a-stock-auction-latest", status, message, startedAt, &finishedAt); recordErr != nil {
		log.Warn().Err(recordErr).Str("task", "a-stock-auction-latest").Msg("record scheduler task run failed")
	}
	if err != nil {
		log.Error().Err(err).Str("task", "a-stock-auction-latest").Msg("scheduler task failed")
		return
	}
	log.Info().Str("task", "a-stock-auction-latest").Str("trade_date", result.Date).Int("total", result.Total).Int("ok", result.OK).Msg("scheduler task completed")
}

func (w *Worker) handleRunAStockHoldingsBackfill(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	opts := aStockHoldingCrawlOptions{
		Code:        strings.TrimSpace(r.URL.Query().Get("code")),
		Period:      strings.TrimSpace(r.URL.Query().Get("period")),
		StartPeriod: strings.TrimSpace(r.URL.Query().Get("start_period")),
		EndPeriod:   strings.TrimSpace(r.URL.Query().Get("end_period")),
	}
	startedAt := time.Now().UTC()
	err := w.runAStockHoldingsBackfill(r.Context(), opts)
	finishedAt := time.Now().UTC()
	status := "success"
	message := "a-stock holdings backfill completed"
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	_ = w.recordTaskRun(r.Context(), "a-stock-holdings-backfill", status, message, startedAt, &finishedAt)
	if err != nil {
		apiutil.WriteJSON(wr, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]string{"status": "triggered"})
}

func (w *Worker) handleRunAStockSectorFundFlowLatest(wr http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Service-Token")) != strings.TrimSpace(w.cfg.ServiceToken) {
		apiutil.WriteJSON(wr, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	startedAt := time.Now().UTC()
	result, err := w.runAStockSectorFundFlowLatest(r.Context(), false)
	finishedAt := time.Now().UTC()
	status := "success"
	message := fmt.Sprintf("a-stock fund flow completed: date=%s groups=%d items=%d sector_groups=%d stock_groups=%d", result.Date, result.Groups, result.Items, result.SectorGroups, result.StockGroups)
	if result.Skipped {
		status = "skipped"
		message = result.Message
	}
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	_ = w.recordTaskRun(r.Context(), "a-stock-sector-fund-flow-latest", status, message, startedAt, &finishedAt)
	if err != nil {
		apiutil.WriteJSON(wr, http.StatusInternalServerError, err.Error(), result)
		return
	}
	if result.Skipped {
		apiutil.WriteJSON(wr, http.StatusUnprocessableEntity, message, result)
		return
	}
	apiutil.WriteJSON(wr, http.StatusOK, "ok", map[string]any{"status": "completed", "result": result})
}
