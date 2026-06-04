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

	result, err := store.SearchItemsFTS(ctx, model.ArticleFilter{Page: 1, PageSize: 10, Keyword: "semiconductor"})
	if err != nil {
		t.Fatalf("SearchItemsFTS error: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected search result: %+v", result)
	}
}

func TestCreateUser(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateUser(ctx, model.User{
		Username:    "charlie",
		DisplayName: "Charlie",
		Email:       "charlie@example.com",
		Role:        "user",
		Status:      1,
	}, "secret")
	if err != nil {
		t.Fatalf("CreateUser error: %v", err)
	}
	if created.Username != "charlie" || created.DisplayName != "Charlie" || created.Email != "charlie@example.com" {
		t.Fatalf("unexpected created user: %+v", created)
	}
	if created.PasswordHash == "" {
		t.Fatal("expected password hash to be stored")
	}
	if _, err := store.AuthenticateUser(ctx, "charlie", "secret"); err != nil {
		t.Fatalf("AuthenticateUser created user error: %v", err)
	}

	if _, err := store.CreateUser(ctx, model.User{Username: "charlie"}, "another"); err == nil {
		t.Fatal("expected duplicate username to fail")
	}
}

func TestFavoriteAndReadState(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.EnsureDefaultAdmin(ctx, "admin", "admin123"); err != nil {
		t.Fatalf("EnsureDefaultAdmin error: %v", err)
	}
	user, err := store.AuthenticateUser(ctx, "admin", "admin123")
	if err != nil {
		t.Fatalf("AuthenticateUser error: %v", err)
	}

	now := time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC)
	_, _, err = store.UpsertItems(ctx, []model.Item{{
		SourceType:  "flash",
		SourceKey:   "state-key-1",
		Title:       "favorite read state",
		Content:     "favorite read content",
		Summary:     "favorite read summary",
		SourceURL:   "https://www.jin10.com/",
		CapturedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishTime: "2026-05-29 11:00:00",
	}})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("ListItems error: %v %+v", err, list)
	}
	itemID := list.Items[0].ID

	if err := store.MarkItemRead(ctx, user.ID, itemID); err != nil {
		t.Fatalf("MarkItemRead error: %v", err)
	}
	favorited, err := store.ToggleFavorite(ctx, user.ID, itemID)
	if err != nil || !favorited {
		t.Fatalf("ToggleFavorite first error: %v favorited=%v", err, favorited)
	}

	list, err = store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10, UserID: user.ID})
	if err != nil {
		t.Fatalf("ListItems with user state error: %v", err)
	}
	if !list.Items[0].Read || !list.Items[0].Favorited {
		t.Fatalf("expected read and favorited state, got %+v", list.Items[0])
	}

	favorited, err = store.ToggleFavorite(ctx, user.ID, itemID)
	if err != nil || favorited {
		t.Fatalf("ToggleFavorite second error: %v favorited=%v", err, favorited)
	}
}

func TestItemEmotionAndDeletion(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC)
	_, _, err := store.UpsertItems(ctx, []model.Item{{
		SourceType:  "headline",
		SourceKey:   "state-key-2",
		Title:       "emotion delete state",
		Content:     "emotion delete content",
		Summary:     "emotion delete summary",
		SourceURL:   "https://www.jin10.com/",
		CapturedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishTime: "2026-05-29 11:00:00",
	}})
	if err != nil {
		t.Fatalf("UpsertItems error: %v", err)
	}

	list, err := store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("ListItems error: %v %+v", err, list)
	}
	itemID := list.Items[0].ID

	if err := store.SetItemEmotion(ctx, itemID, "3"); err != nil {
		t.Fatalf("SetItemEmotion error: %v", err)
	}
	var emotionTag string
	if err := store.db.QueryRowContext(ctx, `SELECT tag FROM item_tags WHERE item_id = ? AND tag LIKE 'emotion:%'`, itemID).Scan(&emotionTag); err != nil {
		t.Fatalf("query emotion tag: %v", err)
	}
	if emotionTag != "emotion:3" {
		t.Fatalf("unexpected emotion tag: %s", emotionTag)
	}

	if err := store.MarkItemDeleted(ctx, itemID); err != nil {
		t.Fatalf("MarkItemDeleted error: %v", err)
	}

	if _, err := store.GetItem(ctx, itemID); err == nil {
		t.Fatal("expected deleted item to be hidden from GetItem")
	}

	list, err = store.ListItems(ctx, model.ArticleFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListItems after deletion error: %v", err)
	}
	if list.Total != 0 || len(list.Items) != 0 {
		t.Fatalf("expected deleted item to disappear from list, got %+v", list)
	}
}
