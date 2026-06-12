package analysis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
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

	coinLore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "90" {
			t.Fatalf("unexpected coinlore id: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id":                 "90",
			"symbol":             "BTC",
			"price_usd":          "62751.68",
			"percent_change_1h":  "-0.11",
			"percent_change_24h": "-6.76",
		}})
	}))
	defer coinLore.Close()

	store := &stubStore{cryptoSnapErr: errors.New("not found")}
	svc := NewService(config.Config{
		ContentURL:   content.URL,
		CoinLoreURL:  coinLore.URL,
		HTTPTimeout:  5 * time.Second,
		ServiceToken: "test-token",
		UserAgent:    "test-agent",
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
	if !strings.Contains(recorder.Body.String(), `"price":62751.68`) {
		t.Fatalf("expected live price in response, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"evidence_articles"`) {
		t.Fatalf("expected evidence articles in response, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"social_posts"`) || !strings.Contains(recorder.Body.String(), `"social_sentiment"`) {
		t.Fatalf("expected social insight fields in response, got %s", recorder.Body.String())
	}
}

func TestHandleCryptoInsightsFallsBackToCoinGecko(t *testing.T) {
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

	coinLore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "coinlore unavailable", "data": nil})
	}))
	defer coinLore.Close()

	coinGecko := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/simple/price" {
			t.Fatalf("unexpected coingecko path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"bitcoin": map[string]any{
				"usd":             62553,
				"usd_24h_change":  -6.901906697026034,
				"last_updated_at": 1780569821,
			},
		})
	}))
	defer coinGecko.Close()

	store := &stubStore{cryptoSnapErr: errors.New("not found")}
	svc := NewService(config.Config{
		ContentURL:   content.URL,
		CoinLoreURL:  coinLore.URL,
		CoinGeckoURL: coinGecko.URL,
		HTTPTimeout:  5 * time.Second,
		ServiceToken: "test-token",
		UserAgent:    "test-agent",
	}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/insights?pair=BTCUSDT", nil)

	svc.handleCryptoInsights(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 coingecko fallback response, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "当前实时价格暂不可用") {
		t.Fatalf("expected coingecko fallback to keep price available, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"price":62553`) {
		t.Fatalf("expected coingecko price in response, got %s", recorder.Body.String())
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

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	store := &stubStore{cryptoSnapErr: errors.New("not found")}
	svc := NewService(config.Config{
		ContentURL:   content.URL,
		CoinLoreURL:  failServer.URL,
		CoinGeckoURL: failServer.URL,
		HTTPTimeout:  5 * time.Second,
		ServiceToken: "test-token",
		UserAgent:    "test-agent",
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

func TestHandleCryptoInsightsSupportsCrossPairQuote(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/crypto/news":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.CryptoNewsResult{Resolution: model.CryptoPairResolution{Pair: "ETHBTC", BaseAsset: "ETH", QuoteAsset: "BTC"}, Page: 1, PageSize: 20}})
		case "/api/v1/crypto/social":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": model.CryptoSocialResult{Resolution: model.CryptoPairResolution{Pair: "ETHBTC", BaseAsset: "ETH", QuoteAsset: "BTC"}, Page: 1, PageSize: 20}})
		default:
			t.Fatalf("unexpected content path: %s", r.URL.Path)
		}
	}))
	defer content.Close()

	coinLore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("id") {
		case "80":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id":                 "80",
				"symbol":             "ETH",
				"price_usd":          "1760.42",
				"percent_change_1h":  "0.63",
				"percent_change_24h": "-6.62",
			}})
		case "90":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id":                 "90",
				"symbol":             "BTC",
				"price_usd":          "62751.68",
				"percent_change_1h":  "-0.11",
				"percent_change_24h": "-6.76",
			}})
		default:
			t.Fatalf("unexpected coinlore id: %s", r.URL.RawQuery)
		}
	}))
	defer coinLore.Close()

	store := &stubStore{cryptoSnapErr: errors.New("not found")}
	svc := NewService(config.Config{
		ContentURL:   content.URL,
		CoinLoreURL:  coinLore.URL,
		HTTPTimeout:  5 * time.Second,
		ServiceToken: "test-token",
		UserAgent:    "test-agent",
	}, store)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/crypto/insights?pair=ETHBTC", nil)

	svc.handleCryptoInsights(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 cross-pair response, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pair":"ETHBTC"`) {
		t.Fatalf("expected ETHBTC payload, got %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"price":0.03`) {
		t.Fatalf("expected cross pair price near ETH/BTC ratio, got %s", recorder.Body.String())
	}
}
