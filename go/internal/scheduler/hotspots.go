package scheduler

import (
	"context"
	"fmt"
	"strings"
)

func (w *Worker) runHotspotSwitchingSnapshotRefresh(ctx context.Context) error {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.ContentURL), "/")
	if baseURL == "" {
		return fmt.Errorf("YUQING_CONTENT_URL not configured")
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetQueryParam("days", "7,14,30").
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
