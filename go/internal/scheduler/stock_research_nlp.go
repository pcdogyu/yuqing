package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type stockResearchNLPParseOptions struct {
	ID      int64
	Code    string
	Company string
	Kind    string
	Source  string
	Start   string
	End     string
	DryRun  bool
}

type stockResearchNLPParseResult struct {
	Total  int  `json:"total"`
	Scored int  `json:"scored"`
	NoText int  `json:"no_text"`
	Failed int  `json:"failed"`
	DryRun bool `json:"dry_run"`
}

func (w *Worker) runStockResearchNLPParse(ctx context.Context, opts stockResearchNLPParseOptions) (stockResearchNLPParseResult, error) {
	items, err := w.loadStockResearchPDFCandidates(ctx, stockResearchPDFParseOptions{
		ID:      opts.ID,
		Code:    opts.Code,
		Company: opts.Company,
		Kind:    opts.Kind,
		Source:  opts.Source,
		Start:   opts.Start,
		End:     opts.End,
		DryRun:  opts.DryRun,
	})
	if err != nil {
		return stockResearchNLPParseResult{}, err
	}
	result := stockResearchNLPParseResult{Total: len(items), DryRun: opts.DryRun}
	if !opts.DryRun && len(items) > 0 && strings.TrimSpace(w.cfg.NLPURL) == "" {
		return result, errors.New("nlp service is not configured")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var errs []string
	for _, item := range items {
		text, _ := stockResearchNLPText(item)
		if strings.TrimSpace(text) == "" {
			result.NoText++
			continue
		}
		if opts.DryRun {
			continue
		}
		score, ok := w.scoreStockResearchText(ctx, item, text)
		if !ok {
			result.Failed++
			errs = append(errs, fmt.Sprintf("%d: NLP score failed", item.ID))
			continue
		}
		update := model.StockResearchNLPUpdate{
			NLPScore:    score.Score,
			NLPRating:   score.Rating,
			NLPReason:   score.Reason,
			NLPScoredAt: now,
		}
		if err := w.writeStockResearchNLPUpdate(ctx, item.ID, update); err != nil {
			result.Failed++
			errs = append(errs, fmt.Sprintf("%d: %v", item.ID, err))
			continue
		}
		result.Scored++
	}
	if len(errs) > 0 {
		return result, errors.New(strings.Join(errs, "; "))
	}
	return result, nil
}

func stockResearchNLPText(item model.StockResearchSurvey) (string, string) {
	for _, candidate := range []struct {
		label string
		text  string
	}{
		{label: "source_text", text: item.SourceText},
		{label: "pdf_text", text: item.PDFText},
		{label: "summary", text: item.Summary},
	} {
		if text := strings.TrimSpace(candidate.text); text != "" {
			return text, candidate.label
		}
	}
	return "", ""
}

func stockResearchNLPParseQuery(opts stockResearchNLPParseOptions) url.Values {
	query := url.Values{}
	if opts.ID > 0 {
		query.Set("id", fmt.Sprintf("%d", opts.ID))
		return query
	}
	if opts.Code != "" {
		query.Set("code", opts.Code)
	}
	if opts.Company != "" {
		query.Set("company", opts.Company)
	}
	if opts.Kind != "" {
		query.Set("kind", opts.Kind)
	}
	if opts.Source != "" {
		query.Set("source", opts.Source)
	}
	if opts.Start != "" {
		query.Set("start", opts.Start)
	}
	if opts.End != "" {
		query.Set("end", opts.End)
	}
	return query
}
