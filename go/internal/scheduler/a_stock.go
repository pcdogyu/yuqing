package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

var aStockRecommendationSources = []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun"}

func (w *Worker) runAStockRecommendation(ctx context.Context, period string) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockRecommendationForDate(ctx, time.Now().In(location).Format("2006-01-02"), period)
}

func (w *Worker) runAStockRecommendationForDate(ctx context.Context, strategyDate string, period string) error {
	for _, sourceType := range aStockRecommendationSources {
		if err := w.runCrawl(ctx, sourceType); err != nil {
			return err
		}
	}
	start, end, label, err := aStockRecommendationWindow(strategyDate, period)
	if err != nil {
		return err
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("page", "1").
		SetQueryParam("page_size", "200").
		SetQueryParam("start", start.UTC().Format(time.RFC3339)).
		SetQueryParam("end", end.UTC().Format(time.RFC3339)).
		Get(w.cfg.ContentURL + "/api/v1/articles")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("a-stock %s recommendation content warmup failed: %s", label, resp.Status())
	}
	log.Info().
		Str("strategy_date", strategyDate).
		Str("period", period).
		Str("window", label).
		Msg("a-stock recommendation window generated")
	return nil
}

func aStockRecommendationWindow(strategyDate string, period string) (time.Time, time.Time, string, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	day, err := time.ParseInLocation("2006-01-02", strategyDate, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	switch period {
	case "afternoon":
		return time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 12, 50, 59, 0, location),
			"09:26-12:50", nil
	default:
		return time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 9, 25, 59, 0, location),
			"09:00-09:25", nil
	}
}
