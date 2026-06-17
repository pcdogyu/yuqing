package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

var aStockRecommendationSources = []string{"flash", "headline", "jin10_full", "eastmoney_kuaixun"}

func (w *Worker) runAStockRecommendation(ctx context.Context, period string) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockRecommendationForDate(ctx, time.Now().In(location).Format("2006-01-02"), period)
}

func (w *Worker) runAStockAuctionCrawl(ctx context.Context) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return w.runAStockAuctionCrawlForDate(ctx, time.Now().In(location).Format("2006-01-02"))
}

func (w *Worker) runAStockAuctionCrawlForDate(ctx context.Context, tradeDate string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return fmt.Errorf("YUQING_ASTOCK_AUCTION_URL not configured")
	}
	var payload struct {
		Date  string                      `json:"date"`
		Items []model.AStockAuctionAmount `json:"items"`
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("date", tradeDate).
		Get(baseURL + "/api/a-stock/auction")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("akshare auction endpoint failed: %s", resp.Status())
	}
	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return err
	}
	if payload.Date == "" {
		payload.Date = tradeDate
	}
	for i := range payload.Items {
		if payload.Items[i].TradeDate == "" {
			payload.Items[i].TradeDate = payload.Date
		}
		if payload.Items[i].FetchedAt.IsZero() {
			payload.Items[i].FetchedAt = time.Now().UTC()
		}
	}
	writeResp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/admin/a-stock/auction")
	if err != nil {
		return err
	}
	if !writeResp.IsSuccess() {
		return fmt.Errorf("content auction upsert failed: %s", writeResp.Status())
	}
	okCount := 0
	for _, item := range payload.Items {
		if strings.TrimSpace(item.Status) == "" || strings.EqualFold(item.Status, "ok") {
			okCount++
		}
	}
	log.Info().
		Str("trade_date", payload.Date).
		Int("total", len(payload.Items)).
		Int("ok", okCount).
		Msg("a-stock auction amounts crawled")
	return nil
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
		return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 59, 0, location),
			"08:00-09:30", nil
	}
}
