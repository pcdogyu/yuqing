package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func TestAuthAndFTSFlow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.EnsureDefaultAdmin(ctx, "admin", "admin123"); err != nil {
		t.Fatalf("EnsureDefaultAdmin error: %v", err)
	}
	user, err := store.AuthenticateUser(ctx, "admin", "admin123")
	if err != nil {
		t.Fatalf("AuthenticateUser error: %v", err)
	}
	session, err := store.CreateSession(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if _, err := store.GetSession(ctx, session.Token); err != nil {
		t.Fatalf("GetSession error: %v", err)
	}

	now := time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC)
	_, _, err = store.UpsertItems(ctx, []model.Item{{
		SourceType:  "headline",
		SourceKey:   "fts-key-1",
		Title:       "Jin10 market flash",
		Content:     "semiconductor sector enters bubble mode",
		Summary:     "market watches semiconductor bubble",
		SourceURL:   "https://xnews.jin10.com/",
		CapturedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishTime: "2026-05-29 11:00:00",
	}})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	result, err := store.SearchItemsFTS(ctx, "semiconductor", 1, 10)
	if err != nil {
		t.Fatalf("SearchItemsFTS error: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected search result: %+v", result)
	}
}
