package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (w *Worker) runHotspotSwitchingSnapshotRefresh(ctx context.Context) error {
	return w.runHotspotSwitchingSnapshotRefreshAt(ctx, time.Now())
}

func (w *Worker) runHotspotSwitchingSnapshotRefreshAt(ctx context.Context, now time.Time) error {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("days", strconv.Itoa(hotspotSwitchingSnapshotRefreshDays(now))).
		Post(baseURL + "/api/v1/internal/hotspots/switching/snapshot")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		message := decodeAStockAuctionEndpointMessage(resp.Body(), resp.String())
		if message == "" {
			message = resp.Status()
		}
		return fmt.Errorf("content hotspot switching snapshot refresh failed: %s", message)
	}
	return nil
}

func hotspotSwitchingSnapshotRefreshDays(now time.Time) int {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.Local
	}
	switch now.In(location).Minute() % 3 {
	case 0:
		return 7
	case 1:
		return 14
	default:
		return 30
	}
}
