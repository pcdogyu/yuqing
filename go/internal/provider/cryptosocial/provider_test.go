package cryptosocial

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/external"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

func TestParseResponseSupportsEnvelope(t *testing.T) {
	body := []byte(`{"items":[{"url":"https://x.com/a/status/1","content":"BTC whale inflow sparks bullish chatter","author":"@alpha","created_at":"2026-06-04T10:00:00Z","tags":["BTC","USDT"]}]}`)
	items, err := ParseResponse(body, provider.SourceTypeCryptoX, "x", "https://example.com/feed", time.Date(2026, 6, 4, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title == "" || items[0].SourceType != provider.SourceTypeCryptoX {
		t.Fatalf("unexpected item: %+v", items[0])
	}
	if items[0].FromText != "@alpha" || items[0].TagFlags != "BTC,USDT" {
		t.Fatalf("unexpected author/tags: %+v", items[0])
	}
}

func TestProviderFetchAppliesBearerToken(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`[{"url":"https://t.me/channel/1","message":"ETH breakout","channel":"tg-alpha","created_at":"2026-06-04 10:15:00"}]`))
	}))
	defer server.Close()

	prov := NewTelegramProvider(resty.New(), server.URL, "secret-token")
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if seenAuth != "Bearer secret-token" {
		t.Fatalf("expected bearer token, got %q", seenAuth)
	}
	if len(items) != 1 || !strings.Contains(items[0].SourceURL, "t.me") {
		t.Fatalf("unexpected fetched items: %+v", items)
	}
}

func TestProviderFetchClassifiesExternalErrors(t *testing.T) {
	t.Run("disabled endpoint", func(t *testing.T) {
		_, err := NewXProvider(resty.New(), "", "").Fetch(context.Background())
		if got := external.Classify(err); got != external.ErrDisabled {
			t.Fatalf("expected %s, got %q err=%v", external.ErrDisabled, got, err)
		}
	})

	t.Run("non 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewXProvider(resty.New(), server.URL, "").Fetch(context.Background())
		if got := external.Classify(err); got != external.ErrNon200 {
			t.Fatalf("expected %s, got %q err=%v", external.ErrNon200, got, err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer server.Close()

		_, err := NewXProvider(resty.New(), server.URL, "").Fetch(context.Background())
		if got := external.Classify(err); got != external.ErrInvalidJSON {
			t.Fatalf("expected %s, got %q err=%v", external.ErrInvalidJSON, got, err)
		}
	})

	t.Run("empty data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"items":[]}`))
		}))
		defer server.Close()

		_, err := NewXProvider(resty.New(), server.URL, "").Fetch(context.Background())
		if got := external.Classify(err); got != external.ErrEmptyData {
			t.Fatalf("expected %s, got %q err=%v", external.ErrEmptyData, got, err)
		}
	})
}

func TestProviderFetchAppliesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"url":"https://x.com/a/status/1","content":"BTC update"}]`))
	}))
	defer server.Close()

	prov := NewXProviderWithOptions(resty.New(), server.URL, "", Options{RateLimit: 40 * time.Millisecond})
	if _, err := prov.Fetch(context.Background()); err != nil {
		t.Fatalf("first fetch error: %v", err)
	}
	started := time.Now()
	if _, err := prov.Fetch(context.Background()); err != nil {
		t.Fatalf("second fetch error: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 30*time.Millisecond {
		t.Fatalf("expected second fetch to wait for rate limit, elapsed=%s", elapsed)
	}
}
