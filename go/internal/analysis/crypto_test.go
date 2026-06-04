package analysis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

func TestHandleCryptoInsightsUsesCachedSnapshot(t *testing.T) {
	payload, err := json.Marshal(model.CryptoInsightResponse{
		Pair:          "BTCUSDT",
		BaseAsset:     "BTC",
		QuoteAsset:    "USDT",
		PriceSnapshot: model.CryptoPriceSnapshot{Symbol: "BTCUSDT", Price: 100000},
		UpdatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	store := &stubStore{
		cryptoSnapshot: model.CryptoInsightSnapshot{
			Pair:       "BTCUSDT",
			HorizonSet: "4h,24h",
			Payload:    string(payload),
			ComputedAt: time.Now().UTC(),
			ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
		},
	}
	svc := NewService(config.Config{}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/insights?pair=BTCUSDT", nil)

	svc.handleCryptoInsights(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "BTCUSDT") {
		t.Fatalf("expected cached payload, got %s", recorder.Body.String())
	}
}

func TestHandleCryptoInsightsBuildsLiveResponse(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/crypto/news":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.CryptoNewsResult{
					Resolution: model.CryptoPairResolution{Pair: "BTCUSDT", BaseAsset: "BTC", QuoteAsset: "USDT", BinanceSymbol: "BTCUSDT", DisplayPair: "BTC/USDT"},
					Items: []model.CryptoEvidenceArticle{
						{ID: 1, Title: "Bitcoin ETF approval lifts BTC", Direction: "bullish", ReasonCategory: "institution_etf", ReasonLabel: "机构与 ETF", RelevanceScore: 8.6, CapturedAt: time.Now().UTC()},
						{ID: 2, Title: "BTC pullback on regulation concerns", Direction: "bearish", ReasonCategory: "regulation", ReasonLabel: "监管政策", RelevanceScore: 5.4, CapturedAt: time.Now().UTC().Add(-time.Hour)},
					},
					Total:    2,
					Page:     1,
					PageSize: 20,
				},
			})
		case "/api/v1/crypto/social":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.CryptoSocialResult{
					Resolution: model.CryptoPairResolution{Pair: "BTCUSDT", BaseAsset: "BTC", QuoteAsset: "USDT", BinanceSymbol: "BTCUSDT", DisplayPair: "BTC/USDT"},
					Items: []model.CryptoSocialPost{
						{ID: 9, Platform: "X", Title: "BTC breakout chatter", Direction: "bullish", ReasonCategory: "onchain_whale", ReasonLabel: "链上鲸鱼资金", RelevanceScore: 7.1, HeatScore: 2.2, CapturedAt: time.Now().UTC()},
					},
					Total:    1,
					Page:     1,
					PageSize: 20,
				},
			})
		default:
			t.Fatalf("unexpected content path: %s", r.URL.Path)
		}
	}))
	defer content.Close()

	now := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	binance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/klines" {
			t.Fatalf("unexpected binance path: %s", r.URL.Path)
		}
		rows := make([][]any, 0, 25)
		price := 100000.0
		for i := 0; i < 25; i++ {
			price += 120
			rows = append(rows, []any{
				float64(now.Add(time.Duration(i-24) * time.Hour).UnixMilli()),
				"100000.0",
				"101000.0",
				"99500.0",
				strconvFormat(price),
				strconvFormat(1000 + float64(i)*10),
			})
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	defer binance.Close()

	store := &stubStore{cryptoSnapErr: errors.New("not found")}
	svc := NewService(config.Config{
		ContentURL:     content.URL,
		BinanceBaseURL: binance.URL,
		HTTPTimeout:    5 * time.Second,
		ServiceToken:   "test-token",
		UserAgent:      "test-agent",
	}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/insights?pair=BTCUSDT", nil)

	svc.handleCryptoInsights(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"signals"`) {
		t.Fatalf("expected signals in response, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"evidence_articles"`) {
		t.Fatalf("expected evidence articles in response, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"social_posts"`) || !strings.Contains(recorder.Body.String(), `"social_sentiment"`) {
		t.Fatalf("expected social insight fields in response, got %s", recorder.Body.String())
	}
}

func TestHandleCryptoInsightsDegradesWhenPriceFetchFails(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/crypto/news":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.CryptoNewsResult{
					Resolution: model.CryptoPairResolution{Pair: "BTCUSDT", BaseAsset: "BTC", QuoteAsset: "USDT", BinanceSymbol: "BTCUSDT", DisplayPair: "BTC/USDT"},
					Items: []model.CryptoEvidenceArticle{
						{ID: 1, Title: "Bitcoin ETF approval lifts BTC", Direction: "bullish", ReasonCategory: "institution_etf", ReasonLabel: "机构与 ETF", RelevanceScore: 8.6, CapturedAt: time.Now().UTC()},
					},
					Total:    1,
					Page:     1,
					PageSize: 20,
				},
			})
		case "/api/v1/crypto/social":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    200,
				"message": "ok",
				"data": model.CryptoSocialResult{
					Resolution: model.CryptoPairResolution{Pair: "BTCUSDT", BaseAsset: "BTC", QuoteAsset: "USDT", BinanceSymbol: "BTCUSDT", DisplayPair: "BTC/USDT"},
					Items:      nil,
					Total:      0,
					Page:       1,
					PageSize:   20,
				},
			})
		default:
			t.Fatalf("unexpected content path: %s", r.URL.Path)
		}
	}))
	defer content.Close()

	binance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "binance unavailable", "data": nil})
	}))
	defer binance.Close()

	store := &stubStore{cryptoSnapErr: errors.New("not found")}
	svc := NewService(config.Config{
		ContentURL:     content.URL,
		BinanceBaseURL: binance.URL,
		HTTPTimeout:    5 * time.Second,
		ServiceToken:   "test-token",
		UserAgent:      "test-agent",
	}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/insights?pair=BTCUSDT", nil)

	svc.handleCryptoInsights(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 degrade response, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "当前实时价格暂不可用") {
		t.Fatalf("expected fallback explanation, got %s", recorder.Body.String())
	}
}

func strconvFormat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
