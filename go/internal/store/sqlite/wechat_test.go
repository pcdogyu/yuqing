package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func TestWechatChallengeAndBindingFlow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.EnsureDefaultAdmin(ctx, "admin", "admin123"); err != nil {
		t.Fatalf("EnsureDefaultAdmin error: %v", err)
	}
	user, err := store.AuthenticateUser(ctx, "admin", "admin123")
	if err != nil {
		t.Fatalf("AuthenticateUser error: %v", err)
	}

	challenge, err := store.CreateWechatChallenge(ctx, model.WechatChallenge{
		SceneStr:  "yuqing:abc123",
		Purpose:   "login",
		UserID:    user.ID,
		Status:    "pending",
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateWechatChallenge error: %v", err)
	}
	if challenge.SceneStr != "yuqing:abc123" {
		t.Fatalf("unexpected challenge scene: %+v", challenge)
	}

	updated, err := store.UpdateWechatChallenge(ctx, model.WechatChallenge{
		SceneStr:  challenge.SceneStr,
		Purpose:   challenge.Purpose,
		UserID:    user.ID,
		Status:    "ready",
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("UpdateWechatChallenge error: %v", err)
	}
	if updated.Status != "ready" {
		t.Fatalf("unexpected updated challenge: %+v", updated)
	}

	session, err := store.CreateSession(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if session.Token == "" {
		t.Fatal("expected session token")
	}

	binding, err := store.UpsertWechatBinding(ctx, model.WechatBinding{UserID: user.ID, OpenID: "openid-1"})
	if err != nil {
		t.Fatalf("UpsertWechatBinding error: %v", err)
	}
	if binding.OpenID != "openid-1" || binding.UserID != user.ID {
		t.Fatalf("unexpected binding: %+v", binding)
	}

	gotBinding, err := store.GetWechatBindingByOpenID(ctx, "openid-1")
	if err != nil {
		t.Fatalf("GetWechatBindingByOpenID error: %v", err)
	}
	if gotBinding.UserID != user.ID {
		t.Fatalf("unexpected binding lookup: %+v", gotBinding)
	}

	gotUser, err := store.GetUserByOpenID(ctx, "openid-1")
	if err != nil {
		t.Fatalf("GetUserByOpenID error: %v", err)
	}
	if gotUser.ID != user.ID {
		t.Fatalf("unexpected openid user: %+v", gotUser)
	}

	expired, err := store.CreateWechatChallenge(ctx, model.WechatChallenge{
		SceneStr:  "yuqing:expired",
		Purpose:   "login",
		Status:    "pending",
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateWechatChallenge expired error: %v", err)
	}
	if _, err := store.GetWechatChallenge(ctx, expired.SceneStr); err == nil {
		t.Fatal("expected expired challenge to be removed")
	}
}

func TestDeleteExpiredWechatChallengesAndListBindings(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	firstUser, err := store.CreateUser(ctx, model.User{
		Username:    "wechat-user-1",
		DisplayName: "Wechat User 1",
		Role:        "user",
		Status:      1,
	}, "secret-1")
	if err != nil {
		t.Fatalf("CreateUser first error: %v", err)
	}
	secondUser, err := store.CreateUser(ctx, model.User{
		Username:    "wechat-user-2",
		DisplayName: "Wechat User 2",
		Role:        "user",
		Status:      1,
	}, "secret-2")
	if err != nil {
		t.Fatalf("CreateUser second error: %v", err)
	}

	if _, err := store.UpsertWechatBinding(ctx, model.WechatBinding{UserID: firstUser.ID, OpenID: "openid-a"}); err != nil {
		t.Fatalf("UpsertWechatBinding first error: %v", err)
	}
	if _, err := store.UpsertWechatBinding(ctx, model.WechatBinding{UserID: secondUser.ID, OpenID: "openid-b"}); err != nil {
		t.Fatalf("UpsertWechatBinding second error: %v", err)
	}

	bindings, err := store.ListWechatBindings(ctx, 10)
	if err != nil {
		t.Fatalf("ListWechatBindings error: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected two bindings, got %+v", bindings)
	}

	if _, err := store.CreateWechatChallenge(ctx, model.WechatChallenge{
		SceneStr:  "yuqing:cleanup-expired",
		Purpose:   "login",
		Status:    "pending",
		ExpiresAt: time.Now().UTC().Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateWechatChallenge expired error: %v", err)
	}
	if _, err := store.CreateWechatChallenge(ctx, model.WechatChallenge{
		SceneStr:  "yuqing:cleanup-live",
		Purpose:   "login",
		Status:    "pending",
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateWechatChallenge live error: %v", err)
	}

	deleted, err := store.DeleteExpiredWechatChallenges(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredWechatChallenges error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one expired challenge deleted, got %d", deleted)
	}
	if _, err := store.GetWechatChallenge(ctx, "yuqing:cleanup-expired"); err == nil {
		t.Fatal("expected expired challenge to be deleted")
	}
	if live, err := store.GetWechatChallenge(ctx, "yuqing:cleanup-live"); err != nil || live.SceneStr == "" {
		t.Fatalf("expected live challenge to remain, got challenge=%+v err=%v", live, err)
	}
}
