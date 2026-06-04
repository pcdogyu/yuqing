package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/cryptoutil"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

const cryptoInsightTTL = 5 * time.Minute

var stableUSDQuotes = map[string]float64{
	"USD":   1,
	"USDT":  1,
	"USDC":  1,
	"BUSD":  1,
	"FDUSD": 1,
}

var coinLoreAssetIDs = map[string]string{
	"BTC":  "90",
	"ETH":  "80",
	"SOL":  "48543",
	"BNB":  "2710",
	"XRP":  "58",
	"DOGE": "2",
	"USDT": "518",
	"USDC": "33285",
}

var coinGeckoAssetIDs = map[string]string{
	"BTC":   "bitcoin",
	"ETH":   "ethereum",
	"SOL":   "solana",
	"BNB":   "binancecoin",
	"XRP":   "ripple",
	"DOGE":  "dogecoin",
	"USDT":  "tether",
	"USDC":  "usd-coin",
	"BUSD":  "binance-usd",
	"FDUSD": "first-digital-usd",
}

type assetPriceQuote struct {
	Symbol      string
	PriceUSD    float64
	Change1H    float64
	Change24H   float64
	Source      string
	UpdatedAt   time.Time
	Has1HChange bool
}

func (s *Service) handleCryptoInsights(w http.ResponseWriter, r *http.Request) {
	response, err := s.buildCryptoInsights(r.Context(), strings.TrimSpace(r.URL.Query().Get("pair")))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, cryptoutil.ErrUnsupportedPair) {
			status = http.StatusBadRequest
		}
		apiutil.WriteJSON(w, status, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", response)
}

func (s *Service) buildCryptoInsights(ctx context.Context, rawPair string) (model.CryptoInsightResponse, error) {
	resolution, err := cryptoutil.ResolvePair(rawPair)
	if err != nil {
		return model.CryptoInsightResponse{}, fmt.Errorf("暂不支持该币对")
	}
	now := time.Now().UTC()
	horizonSet := "4h,24h"

	snapshot, snapshotErr := s.store.GetCryptoInsightSnapshot(ctx, resolution.Pair, horizonSet)
	if snapshotErr == nil && snapshot.ExpiresAt.After(now) {
		var cached model.CryptoInsightResponse
		if err := json.Unmarshal([]byte(snapshot.Payload), &cached); err == nil {
			return cached, nil
		}
	}

	news, newsErr := s.fetchCryptoNews(ctx, resolution.Pair)
	if newsErr != nil {
		news = model.CryptoNewsResult{Resolution: resolution, Page: 1, PageSize: 20}
	}
	social, socialErr := s.fetchCryptoSocial(ctx, resolution.Pair)
	if socialErr != nil {
		social = model.CryptoSocialResult{Resolution: resolution, Page: 1, PageSize: 20}
	}

	price := model.CryptoPriceSnapshot{Symbol: resolution.Pair, UpdatedAt: now}
	if snapshot, priceErr := s.fetchPriceSnapshot(ctx, resolution, now); priceErr == nil {
		price = snapshot
	}
	reasons := aggregateCryptoReasons(news.Items, social.Items)
	socialSentiment := buildSocialSentiment(social.Items)
	signals := buildCryptoSignals(news.Items, social.Items, price)
	explanation := s.buildAIExplanation(ctx, resolution, price, signals, reasons, news.Items, socialSentiment, social.Items)
	response := model.CryptoInsightResponse{
		Pair:             resolution.Pair,
		BaseAsset:        resolution.BaseAsset,
		QuoteAsset:       resolution.QuoteAsset,
		PriceSnapshot:    price,
		Signals:          signals,
		TopReasons:       reasons,
		EvidenceArticles: topEvidence(news.Items, 12),
		SocialSentiment:  socialSentiment,
		SocialPosts:      topSocialEvidence(social.Items, 10),
		AIExplanation:    explanation,
		Disclaimer:       "仅供信息参考，不构成投资建议",
		UpdatedAt:        now,
		CacheTTLSeconds:  int(cryptoInsightTTL / time.Second),
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return model.CryptoInsightResponse{}, err
	}
	_ = s.store.UpsertCryptoInsightSnapshot(ctx, model.CryptoInsightSnapshot{
		Pair:       resolution.Pair,
		HorizonSet: horizonSet,
		Payload:    string(payload),
		ComputedAt: now,
		ExpiresAt:  now.Add(cryptoInsightTTL),
	})
	return response, nil
}

func (s *Service) fetchCryptoNews(ctx context.Context, pair string) (model.CryptoNewsResult, error) {
	var envelope struct {
		Data model.CryptoNewsResult `json:"data"`
	}
	resp, err := s.client.R().
		SetContext(ctx).
		SetResult(&envelope).
		SetQueryParam("pair", pair).
		SetQueryParam("page", "1").
		SetQueryParam("page_size", "20").
		Get(s.cfg.ContentURL + "/api/v1/crypto/news")
	if err != nil {
		return model.CryptoNewsResult{}, err
	}
	if !resp.IsSuccess() {
		return model.CryptoNewsResult{}, errors.New(resp.Status())
	}
	return envelope.Data, nil
}

func (s *Service) fetchCryptoSocial(ctx context.Context, pair string) (model.CryptoSocialResult, error) {
	var envelope struct {
		Data model.CryptoSocialResult `json:"data"`
	}
	resp, err := s.client.R().
		SetContext(ctx).
		SetResult(&envelope).
		SetQueryParam("pair", pair).
		SetQueryParam("page", "1").
		SetQueryParam("page_size", "20").
		Get(s.cfg.ContentURL + "/api/v1/crypto/social")
	if err != nil {
		return model.CryptoSocialResult{}, err
	}
	if !resp.IsSuccess() {
		return model.CryptoSocialResult{}, errors.New(resp.Status())
	}
	return envelope.Data, nil
}

func (s *Service) fetchPriceSnapshot(ctx context.Context, resolution model.CryptoPairResolution, now time.Time) (model.CryptoPriceSnapshot, error) {
	base, err := s.fetchAssetPriceQuote(ctx, resolution.BaseAsset, now)
	if err != nil {
		return model.CryptoPriceSnapshot{}, err
	}

	quotePrice := 1.0
	quoteChange1H := 0.0
	quoteChange24H := 0.0
	updatedAt := base.UpdatedAt
	if stablePrice, ok := stableUSDQuotes[resolution.QuoteAsset]; ok {
		quotePrice = stablePrice
	} else {
		quote, quoteErr := s.fetchAssetPriceQuote(ctx, resolution.QuoteAsset, now)
		if quoteErr != nil {
			return model.CryptoPriceSnapshot{}, quoteErr
		}
		quotePrice = quote.PriceUSD
		quoteChange1H = normalized1HChange(quote)
		quoteChange24H = quote.Change24H
		if quote.UpdatedAt.After(updatedAt) {
			updatedAt = quote.UpdatedAt
		}
	}

	if base.PriceUSD <= 0 || quotePrice <= 0 {
		return model.CryptoPriceSnapshot{}, errors.New("price source unavailable")
	}

	currentPrice := base.PriceUSD / quotePrice
	change1H := derivePairChange(currentPrice, base.PriceUSD, normalized1HChange(base), quotePrice, quoteChange1H)
	change24H := derivePairChange(currentPrice, base.PriceUSD, base.Change24H, quotePrice, quoteChange24H)
	change4H := estimate4HChange(currentPrice, change1H, change24H)

	return model.CryptoPriceSnapshot{
		Symbol:     resolution.Pair,
		Price:      cryptoutil.Round2(currentPrice),
		Change1H:   cryptoutil.Round2(change1H),
		Change4H:   cryptoutil.Round2(change4H),
		Change24H:  cryptoutil.Round2(change24H),
		Volatility: cryptoutil.Round2(estimateVolatility(change1H, change4H, change24H)),
		VolumeBias: 0,
		UpdatedAt:  updatedAt,
	}, nil
}

func (s *Service) fetchAssetPriceQuote(ctx context.Context, symbol string, now time.Time) (assetPriceQuote, error) {
	if quote, err := s.fetchCoinLorePrice(ctx, symbol, now); err == nil {
		return quote, nil
	}
	return s.fetchCoinGeckoPrice(ctx, symbol, now)
}

func (s *Service) fetchCoinLorePrice(ctx context.Context, symbol string, now time.Time) (assetPriceQuote, error) {
	id, ok := coinLoreAssetIDs[symbol]
	if !ok {
		return assetPriceQuote{}, errors.New("coinlore asset id not found")
	}
	var payload []struct {
		Symbol           string  `json:"symbol"`
		PriceUSD         string  `json:"price_usd"`
		PercentChange1H  string  `json:"percent_change_1h"`
		PercentChange24H string  `json:"percent_change_24h"`
		LastUpdated      int64   `json:"last_updated"`
		Volume24         float64 `json:"volume24"`
		MarketCapUSD     string  `json:"market_cap_usd"`
	}
	resp, err := s.client.R().
		SetContext(ctx).
		SetResult(&payload).
		Get(strings.TrimRight(s.cfg.CoinLoreURL, "/") + "/api/ticker/?id=" + id)
	if err != nil {
		return assetPriceQuote{}, err
	}
	if !resp.IsSuccess() || len(payload) == 0 {
		return assetPriceQuote{}, errors.New("coinlore price unavailable")
	}

	updatedAt := now
	if payload[0].LastUpdated > 0 {
		updatedAt = time.Unix(payload[0].LastUpdated, 0).UTC()
	}
	resolvedSymbol := strings.TrimSpace(payload[0].Symbol)
	if resolvedSymbol == "" {
		resolvedSymbol = symbol
	}
	return assetPriceQuote{
		Symbol:      resolvedSymbol,
		PriceUSD:    parseLooseFloat(payload[0].PriceUSD),
		Change1H:    parseLooseFloat(payload[0].PercentChange1H),
		Change24H:   parseLooseFloat(payload[0].PercentChange24H),
		Source:      "coinlore",
		UpdatedAt:   updatedAt,
		Has1HChange: true,
	}, nil
}

func (s *Service) fetchCoinGeckoPrice(ctx context.Context, symbol string, now time.Time) (assetPriceQuote, error) {
	id, ok := coinGeckoAssetIDs[symbol]
	if !ok {
		return assetPriceQuote{}, errors.New("coingecko asset id not found")
	}
	var payload map[string]struct {
		USD           float64 `json:"usd"`
		USD24HChange  float64 `json:"usd_24h_change"`
		LastUpdatedAt int64   `json:"last_updated_at"`
	}
	resp, err := s.client.R().
		SetContext(ctx).
		SetResult(&payload).
		SetQueryParam("ids", id).
		SetQueryParam("vs_currencies", "usd").
		SetQueryParam("include_24hr_change", "true").
		SetQueryParam("include_last_updated_at", "true").
		Get(strings.TrimRight(s.cfg.CoinGeckoURL, "/") + "/simple/price")
	if err != nil {
		return assetPriceQuote{}, err
	}
	if !resp.IsSuccess() {
		return assetPriceQuote{}, errors.New("coingecko price unavailable")
	}
	entry, ok := payload[id]
	if !ok || entry.USD <= 0 {
		return assetPriceQuote{}, errors.New("coingecko payload missing asset")
	}
	updatedAt := now
	if entry.LastUpdatedAt > 0 {
		updatedAt = time.Unix(entry.LastUpdatedAt, 0).UTC()
	}
	return assetPriceQuote{
		Symbol:      symbol,
		PriceUSD:    entry.USD,
		Change1H:    0,
		Change24H:   entry.USD24HChange,
		Source:      "coingecko",
		UpdatedAt:   updatedAt,
		Has1HChange: false,
	}, nil
}

func normalized1HChange(quote assetPriceQuote) float64 {
	if quote.Has1HChange {
		return quote.Change1H
	}
	return estimate1HFrom24H(quote.Change24H)
}

func estimate1HFrom24H(change24H float64) float64 {
	base := 1 + change24H/100
	if base <= 0 {
		return 0
	}
	return (math.Pow(base, 1.0/24.0) - 1) * 100
}

func derivePairChange(currentPairPrice, basePrice, baseChange, quotePrice, quoteChange float64) float64 {
	if currentPairPrice <= 0 {
		return 0
	}
	pastBase := historicalPrice(basePrice, baseChange)
	pastQuote := historicalPrice(quotePrice, quoteChange)
	if pastBase <= 0 || pastQuote <= 0 {
		return 0
	}
	pastPairPrice := pastBase / pastQuote
	if pastPairPrice <= 0 {
		return 0
	}
	return (currentPairPrice/pastPairPrice - 1) * 100
}

func historicalPrice(currentPrice, changePct float64) float64 {
	denominator := 1 + changePct/100
	if currentPrice <= 0 || denominator <= 0 {
		return 0
	}
	return currentPrice / denominator
}

func estimate4HChange(currentPrice, change1H, change24H float64) float64 {
	price1H := historicalPrice(currentPrice, change1H)
	price24H := historicalPrice(currentPrice, change24H)
	if price1H <= 0 || price24H <= 0 {
		return 0
	}
	ratio := price1H / price24H
	if ratio <= 0 {
		return 0
	}
	hourlyRate := math.Pow(ratio, 1.0/23.0) - 1
	if 1+hourlyRate <= 0 {
		return 0
	}
	price4H := price1H / math.Pow(1+hourlyRate, 3)
	if price4H <= 0 {
		return 0
	}
	return (currentPrice/price4H - 1) * 100
}

func estimateVolatility(change1H, change4H, change24H float64) float64 {
	hourly := []float64{change1H, change4H / 4, change24H / 24}
	mean := 0.0
	for _, value := range hourly {
		mean += value
	}
	mean /= float64(len(hourly))
	variance := 0.0
	for _, value := range hourly {
		variance += math.Pow(value-mean, 2)
	}
	return math.Sqrt(variance / float64(len(hourly)))
}

func parseLooseFloat(raw string) float64 {
	value, err := json.Number(strings.TrimSpace(raw)).Float64()
	if err != nil {
		return 0
	}
	return value
}

func aggregateCryptoReasons(items []model.CryptoEvidenceArticle, posts []model.CryptoSocialPost) []model.CryptoReason {
	type bucket struct {
		category    string
		label       string
		score       float64
		count       int
		newsCount   int
		socialCount int
	}
	buckets := map[string]*bucket{}
	for _, item := range items {
		sign := directionalWeight(item.Direction)
		key := item.ReasonCategory + "|" + item.Direction
		if _, ok := buckets[key]; !ok {
			buckets[key] = &bucket{category: item.ReasonCategory, label: item.ReasonLabel}
		}
		buckets[key].score += item.RelevanceScore * sign
		buckets[key].count++
		buckets[key].newsCount++
	}
	for _, post := range posts {
		sign := directionalWeight(post.Direction)
		key := post.ReasonCategory + "|" + post.Direction
		if _, ok := buckets[key]; !ok {
			buckets[key] = &bucket{category: post.ReasonCategory, label: post.ReasonLabel}
		}
		buckets[key].score += (post.RelevanceScore*0.75 + post.HeatScore*0.25) * sign
		buckets[key].count++
		buckets[key].socialCount++
	}
	reasons := make([]model.CryptoReason, 0, len(buckets))
	for _, bucket := range buckets {
		direction := "neutral"
		if bucket.score > 0.15 {
			direction = "bullish"
		} else if bucket.score < -0.15 {
			direction = "bearish"
		}
		reasons = append(reasons, model.CryptoReason{
			Category:      bucket.category,
			Label:         bucket.label,
			Direction:     direction,
			Score:         cryptoutil.Round2(bucket.score),
			EvidenceCount: bucket.count,
			Summary:       reasonSummary(bucket.label, bucket.newsCount, bucket.socialCount),
		})
	}
	sort.Slice(reasons, func(i, j int) bool {
		if math.Abs(reasons[i].Score) == math.Abs(reasons[j].Score) {
			return reasons[i].EvidenceCount > reasons[j].EvidenceCount
		}
		return math.Abs(reasons[i].Score) > math.Abs(reasons[j].Score)
	})
	if len(reasons) > 6 {
		reasons = reasons[:6]
	}
	return reasons
}

func buildCryptoSignals(items []model.CryptoEvidenceArticle, posts []model.CryptoSocialPost, price model.CryptoPriceSnapshot) model.CryptoSignalSet {
	newsScore := aggregateNewsScore(items)
	socialScore := aggregateSocialScore(posts)
	priceScore4h := aggregatePriceScore(price.Change4H, price.Change1H, price.VolumeBias, price.Volatility)
	priceScore24h := aggregatePriceScore(price.Change24H, price.Change4H, price.VolumeBias, price.Volatility)
	return model.CryptoSignalSet{
		H4:  makeSignal("4h", newsScore, socialScore, priceScore4h, len(items), len(posts)),
		H24: makeSignal("24h", newsScore, socialScore, priceScore24h, len(items), len(posts)),
	}
}

func aggregateNewsScore(items []model.CryptoEvidenceArticle) float64 {
	if len(items) == 0 {
		return 0
	}
	total := 0.0
	weight := 0.0
	for _, item := range items {
		score := 0.0
		switch item.Direction {
		case "bullish":
			score = 1
		case "bearish":
			score = -1
		default:
			score = 0
		}
		total += score * maxFloat(item.RelevanceScore, 1)
		weight += maxFloat(item.RelevanceScore, 1)
	}
	if weight == 0 {
		return 0
	}
	return math.Tanh(total / weight)
}

func aggregateSocialScore(posts []model.CryptoSocialPost) float64 {
	if len(posts) == 0 {
		return 0
	}
	total := 0.0
	weight := 0.0
	for _, post := range posts {
		score := directionalWeight(post.Direction)
		postWeight := maxFloat(post.RelevanceScore*0.7+post.HeatScore*0.3, 1)
		total += score * postWeight
		weight += postWeight
	}
	if weight == 0 {
		return 0
	}
	return math.Tanh(total / weight)
}

func aggregatePriceScore(changePrimary, changeSecondary, volumeBias, volatility float64) float64 {
	score := changePrimary*0.08 + changeSecondary*0.04 + volumeBias*0.6 - volatility*0.15
	return math.Tanh(score / 3)
}

func makeSignal(horizon string, newsScore, socialScore, priceScore float64, newsCount, socialCount int) model.CryptoSignal {
	combined := combineSignalScores(horizon, newsScore, socialScore, priceScore, socialCount > 0)
	evidenceFactor := cryptoutil.Clamp01(float64(newsCount+socialCount) / 14)
	consensusScore := consensusAverage(newsScore, socialScore, priceScore, socialCount > 0)
	agreement := 1 - consensusSpread(newsScore, socialScore, priceScore, socialCount > 0)/2
	confidence := cryptoutil.Clamp01((math.Abs(combined)*0.45 + evidenceFactor*0.25 + agreement*0.3))

	bullish := 0.33 + combined*0.3
	bearish := 0.33 - combined*0.3
	neutral := 1 - bullish - bearish + (1-math.Abs(consensusScore))*0.04
	if neutral < 0.05 {
		neutral = 0.05
	}
	sum := bullish + neutral + bearish
	bullish /= sum
	neutral /= sum
	bearish /= sum

	direction := "neutral"
	maxScore := neutral
	if bullish > maxScore {
		direction = "bullish"
		maxScore = bullish
	}
	if bearish > maxScore {
		direction = "bearish"
	}
	return model.CryptoSignal{
		Horizon:     horizon,
		Direction:   direction,
		Bullish:     cryptoutil.Round2(bullish),
		Neutral:     cryptoutil.Round2(neutral),
		Bearish:     cryptoutil.Round2(bearish),
		Confidence:  cryptoutil.Round2(confidence),
		NewsScore:   cryptoutil.Round2(newsScore),
		SocialScore: cryptoutil.Round2(socialScore),
		PriceScore:  cryptoutil.Round2(priceScore),
		Explanation: signalExplanation(horizon, direction, confidence, newsScore, socialScore, priceScore),
	}
}

func signalExplanation(horizon, direction string, confidence, newsScore, socialScore, priceScore float64) string {
	return fmt.Sprintf("%s 周期偏%s，置信度 %.2f，消息面 %.2f，社媒面 %.2f，价格面 %.2f", horizon, direction, confidence, newsScore, socialScore, priceScore)
}

func (s *Service) buildAIExplanation(ctx context.Context, resolution model.CryptoPairResolution, price model.CryptoPriceSnapshot, signals model.CryptoSignalSet, reasons []model.CryptoReason, items []model.CryptoEvidenceArticle, socialSentiment model.CryptoSocialSentiment, posts []model.CryptoSocialPost) string {
	fallback := fallbackExplanation(resolution, price, signals, reasons, items, socialSentiment, posts)
	if strings.TrimSpace(s.cfg.LLMBaseURL) == "" || strings.TrimSpace(s.cfg.LLMModel) == "" {
		return fallback
	}
	prompt := fallback + "\n请将上述结论整理为 120 字以内中文说明，保留非投资建议语气。"
	requestBody := map[string]any{
		"model": s.cfg.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是谨慎的加密货币市场分析助手，只输出简洁中文说明，不承诺收益。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	req := s.client.R().
		SetContext(ctx).
		SetBody(requestBody).
		SetResult(&envelope)
	if key := strings.TrimSpace(s.cfg.LLMAPIKey); key != "" {
		req.SetHeader("Authorization", "Bearer "+key)
	}
	resp, err := req.Post(strings.TrimRight(s.cfg.LLMBaseURL, "/") + "/chat/completions")
	if err != nil || !resp.IsSuccess() || len(envelope.Choices) == 0 {
		return fallback
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	if content == "" {
		return fallback
	}
	return content
}

func fallbackExplanation(resolution model.CryptoPairResolution, price model.CryptoPriceSnapshot, signals model.CryptoSignalSet, reasons []model.CryptoReason, items []model.CryptoEvidenceArticle, socialSentiment model.CryptoSocialSentiment, posts []model.CryptoSocialPost) string {
	var buf bytes.Buffer
	if price.Price > 0 {
		buf.WriteString(fmt.Sprintf("%s 当前价格 %.2f，1h %.2f%%，4h %.2f%%，24h %.2f%%。", resolution.DisplayPair, price.Price, price.Change1H, price.Change4H, price.Change24H))
	} else {
		buf.WriteString(fmt.Sprintf("%s 当前实时价格暂不可用。", resolution.DisplayPair))
	}
	buf.WriteString(fmt.Sprintf(" 4h 信号偏%s，24h 信号偏%s。", signals.H4.Direction, signals.H24.Direction))
	if len(reasons) > 0 {
		buf.WriteString(" 主要驱动：")
		for idx, reason := range reasons {
			if idx >= 3 {
				break
			}
			if idx > 0 {
				buf.WriteString("；")
			}
			buf.WriteString(fmt.Sprintf("%s(%s)", reason.Label, reason.Direction))
		}
	} else {
		buf.WriteString(" 资讯证据不足，当前判断更依赖价格面。")
	}
	if len(posts) > 0 {
		buf.WriteString(fmt.Sprintf(" 社媒情绪偏%s，热度证据 %d 条。", socialSentiment.Direction, socialSentiment.EvidenceCount))
	} else {
		buf.WriteString(" 社媒证据仍偏少。")
	}
	if len(items) == 0 {
		buf.WriteString(" 近 48 小时未发现足够相关新闻。")
	}
	if len(items) == 0 && len(posts) == 0 && price.Price <= 0 {
		buf.WriteString(" 当前数据源暂时都不完整，已返回降级结果。")
	}
	buf.WriteString(" 仅供信息参考，不构成投资建议。")
	return buf.String()
}

func topEvidence(items []model.CryptoEvidenceArticle, limit int) []model.CryptoEvidenceArticle {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func topSocialEvidence(items []model.CryptoSocialPost, limit int) []model.CryptoSocialPost {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func buildSocialSentiment(posts []model.CryptoSocialPost) model.CryptoSocialSentiment {
	if len(posts) == 0 {
		return model.CryptoSocialSentiment{
			Direction:   "neutral",
			Neutral:     1,
			VolumeScore: 0,
			Summary:     "近 48 小时暂无足够社媒证据，当前仍以新闻与价格面为主。",
		}
	}
	bullishWeight := 0.0
	neutralWeight := 0.0
	bearishWeight := 0.0
	totalWeight := 0.0
	for _, post := range posts {
		weight := maxFloat(post.RelevanceScore*0.7+post.HeatScore*0.3, 1)
		switch post.Direction {
		case "bullish":
			bullishWeight += weight
		case "bearish":
			bearishWeight += weight
		default:
			neutralWeight += weight
		}
		totalWeight += weight
	}
	if totalWeight == 0 {
		totalWeight = 1
	}
	bullish := bullishWeight / totalWeight
	neutral := neutralWeight / totalWeight
	bearish := bearishWeight / totalWeight
	direction := "neutral"
	maxWeight := neutral
	if bullish > maxWeight {
		direction = "bullish"
		maxWeight = bullish
	}
	if bearish > maxWeight {
		direction = "bearish"
	}
	score := aggregateSocialScore(posts)
	volumeScore := cryptoutil.Clamp01(float64(len(posts)) / 12)
	confidence := cryptoutil.Clamp01(volumeScore*0.45 + math.Abs(score)*0.35 + (1-math.Abs(bullish-bearish))*0.2)
	return model.CryptoSocialSentiment{
		Direction:     direction,
		Bullish:       cryptoutil.Round2(bullish),
		Neutral:       cryptoutil.Round2(neutral),
		Bearish:       cryptoutil.Round2(bearish),
		Confidence:    cryptoutil.Round2(confidence),
		SocialScore:   cryptoutil.Round2(score),
		VolumeScore:   cryptoutil.Round2(volumeScore),
		EvidenceCount: len(posts),
		Summary:       socialSummary(direction, len(posts), volumeScore),
	}
}

func combineSignalScores(horizon string, newsScore, socialScore, priceScore float64, hasSocial bool) float64 {
	priceWeight := 0.55
	newsWeight := 0.45
	socialWeight := 0.0
	if hasSocial {
		priceWeight = 0.48
		newsWeight = 0.32
		socialWeight = 0.20
	}
	if horizon == "24h" {
		priceWeight -= 0.03
		newsWeight += 0.02
		socialWeight += 0.01
	}
	return priceScore*priceWeight + newsScore*newsWeight + socialScore*socialWeight
}

func consensusAverage(newsScore, socialScore, priceScore float64, hasSocial bool) float64 {
	total := newsScore + priceScore
	count := 2.0
	if hasSocial {
		total += socialScore
		count++
	}
	return total / count
}

func consensusSpread(newsScore, socialScore, priceScore float64, hasSocial bool) float64 {
	minValue := math.Min(newsScore, priceScore)
	maxValue := math.Max(newsScore, priceScore)
	if hasSocial {
		minValue = math.Min(minValue, socialScore)
		maxValue = math.Max(maxValue, socialScore)
	}
	return maxValue - minValue
}

func directionalWeight(direction string) float64 {
	switch direction {
	case "bullish":
		return 1
	case "bearish":
		return -1
	default:
		return 0.2
	}
}

func reasonSummary(label string, newsCount, socialCount int) string {
	switch {
	case newsCount > 0 && socialCount > 0:
		return fmt.Sprintf("%s：新闻 %d 条，社媒 %d 条", label, newsCount, socialCount)
	case socialCount > 0:
		return fmt.Sprintf("%s：社媒证据 %d 条", label, socialCount)
	default:
		return fmt.Sprintf("%s：新闻证据 %d 条", label, newsCount)
	}
}

func socialSummary(direction string, count int, volumeScore float64) string {
	heat := "一般"
	switch {
	case volumeScore >= 0.75:
		heat = "很高"
	case volumeScore >= 0.4:
		heat = "中等"
	}
	return fmt.Sprintf("近 48 小时社媒证据 %d 条，情绪偏%s，热度%s。", count, direction, heat)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
