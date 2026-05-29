package apiutil

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type contextKey string

const UserContextKey contextKey = "user"

func WriteJSON(w http.ResponseWriter, status int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    status,
		"message": message,
		"data":    data,
	})
}

func IntQuery(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func BearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[7:])
}

func WithUser(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, UserContextKey, userID)
}

func UserIDFromContext(ctx context.Context) int64 {
	value := ctx.Value(UserContextKey)
	userID, _ := value.(int64)
	return userID
}
