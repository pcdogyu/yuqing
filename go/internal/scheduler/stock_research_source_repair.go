package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/stockresearch"
)

type stockResearchSourceRepairOptions struct {
	Source string
	Start  string
	End    string
	DryRun bool
	Force  bool
}

type stockResearchSourceRepairResult struct {
	Total    int  `json:"total"`
	Repaired int  `json:"repaired"`
	Skipped  int  `json:"skipped"`
	Failed   int  `json:"failed"`
	DryRun   bool `json:"dry_run"`
}

func (w *Worker) runStockResearchSourceRepair(ctx context.Context, opts stockResearchSourceRepairOptions) (stockResearchSourceRepairResult, error) {
	opts = normalizeStockResearchSourceRepairOptions(opts, stockResearchToday())
	if !isSupportedStockResearchSourceRepair(opts.Source) {
		return stockResearchSourceRepairResult{DryRun: opts.DryRun}, fmt.Errorf("unsupported source repair: %s", opts.Source)
	}
	items, err := w.loadStockResearchSourceRepairCandidates(ctx, opts)
	if err != nil {
		return stockResearchSourceRepairResult{}, err
	}
	result := stockResearchSourceRepairResult{Total: len(items), DryRun: opts.DryRun}
	var errs []string
	for _, item := range items {
		if strings.TrimSpace(item.SourceURL) == "" {
			result.Skipped++
			continue
		}
		if !opts.Force && strings.TrimSpace(item.SourceText) != "" {
			result.Skipped++
			continue
		}
		if opts.DryRun {
			result.Repaired++
			continue
		}
		update := w.buildStockResearchSourceRepairUpdate(ctx, item, opts)
		if strings.TrimSpace(update.SourceText) == "" || update.SourceFetchStatus != stockresearch.SourceStatusParsed {
			result.Failed++
			errs = append(errs, fmt.Sprintf("%d: %s", item.ID, nonEmptyText(update.SourceFetchError, update.SourceFetchStatus)))
			continue
		}
		if err := w.writeStockResearchSourceUpdate(ctx, item.ID, update); err != nil {
			result.Failed++
			errs = append(errs, fmt.Sprintf("%d: %v", item.ID, err))
			continue
		}
		result.Repaired++
	}
	if len(errs) > 0 {
		return result, errors.New(strings.Join(errs, "; "))
	}
	return result, nil
}

func normalizeStockResearchSourceRepairOptions(opts stockResearchSourceRepairOptions, now time.Time) stockResearchSourceRepairOptions {
	opts.Source = strings.TrimSpace(opts.Source)
	if opts.Source == "" {
		opts.Source = "sina_finance_report"
	}
	opts.Start = normalizeStockResearchDate(opts.Start)
	opts.End = normalizeStockResearchDate(opts.End)
	if opts.Start == "" || opts.End == "" {
		start, end := stockResearchSourceRepairDefaultRange(now)
		if opts.Start == "" {
			opts.Start = start
		}
		if opts.End == "" {
			opts.End = end
		}
	}
	return opts
}

func isSupportedStockResearchSourceRepair(source string) bool {
	switch strings.TrimSpace(source) {
	case "sina_finance_report", "eastmoney_report":
		return true
	default:
		return false
	}
}

func stockResearchSourceRepairDefaultRange(now time.Time) (string, string) {
	local := now.In(aStockLocation())
	end := local.Format("2006-01-02")
	start := local.AddDate(0, 0, -6).Format("2006-01-02")
	return start, end
}

func (w *Worker) loadStockResearchSourceRepairCandidates(ctx context.Context, opts stockResearchSourceRepairOptions) ([]model.StockResearchSurvey, error) {
	query := url.Values{}
	query.Set("kind", "report")
	query.Set("source", opts.Source)
	query.Set("start", opts.Start)
	query.Set("end", opts.End)
	query.Set("page_size", "200")
	items := make([]model.StockResearchSurvey, 0)
	for page := 1; ; page++ {
		query.Set("page", fmt.Sprintf("%d", page))
		var list model.StockResearchListResult
		if err := w.getContentJSON(ctx, "/api/v1/stock-research?"+query.Encode(), &list); err != nil {
			return nil, err
		}
		items = append(items, list.Items...)
		if list.PageSize <= 0 || list.Total <= page*list.PageSize || len(list.Items) == 0 {
			break
		}
	}
	return items, nil
}

func (w *Worker) buildStockResearchSourceRepairUpdate(ctx context.Context, item model.StockResearchSurvey, opts stockResearchSourceRepairOptions) model.StockResearchSourceUpdate {
	update := stockresearch.FetchSource(ctx, item.SourceURL, stockresearch.FetchOptions{
		UserAgent: nonEmptyText(w.cfg.UserAgent, "Mozilla/5.0"),
		Timeout:   maxDuration(w.cfg.HTTPTimeout, 15*time.Second),
	})
	update.Force = opts.Force
	return update
}

func (w *Worker) writeStockResearchSourceUpdate(ctx context.Context, id int64, update model.StockResearchSourceUpdate) error {
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(update).
		Post(fmt.Sprintf("%s/api/v1/internal/stock-research/%d/source", strings.TrimRight(w.cfg.ContentURL, "/"), id))
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock research source update failed: %s", resp.Status())
	}
	return nil
}
