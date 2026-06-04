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
	"strconv"
	"strings"
	"time"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/cryptoutil"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

const cryptoInsightTTL = 5 * time.Minute

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

	news, err := s.fetchCryptoNews(ctx, resolution.Pair)
	if err != nil && snapshotErr == nil {
		var cached model.CryptoInsightResponse
		if json.Unmarshal([]byte(snapshot.Payload), &cached) == nil {
			return cached, nil
		}
	}
	if err != nil {
		return model.CryptoInsightResponse{}, err
	}
	social, _ := s.fetchCryptoSocial(ctx, resolution.Pair)

	candles, err := s.fetchCandles(ctx, resolution.BinanceSymbol)
	if err != nil && snapshotErr == nil {
		var cached model.CryptoInsightResponse
		if json.Unmarshal([]byte(snapshot.Payload), &cached) == nil {
			return cached, nil
		}
	}
	if err != nil {
		return model.CryptoInsightResponse{}, err
	}

	price := buildPriceSnapshot(resolution.BinanceSymbol, candles, now)
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

func (s *Service) fetchCandles(ctx context.Context, symbol string) ([]model.CryptoPriceCandle, error) {
	resp, err := s.client.R().
		SetContext(ctx).
		SetQueryParam("symbol", symbol).
		SetQueryParam("interval", "1h").
		SetQueryParam("limit", "25").
		Get(strings.TrimRight(s.cfg.BinanceBaseURL, "/") + "/api/v3/klines")
	if err == nil && resp.IsSuccess() {
		var rows [][]any
		if unmarshalErr := json.Unmarshal(resp.Body(), &rows); unmarshalErr == nil {
			candles := make([]model.CryptoPriceCandle, 0, len(rows))
			now := time.Now().UTC()
			for _, row := range rows {
				if len(row) < 6 {
					continue
				}
				candles = append(candles, model.CryptoPriceCandle{
					Symbol:    symbol,
					Interval:  "1h",
					OpenTime:  parseBinanceMillis(row[0]),
					Open:      parseBinanceFloat(row[1]),
					High:      parseBinanceFloat(row[2]),
					Low:       parseBinanceFloat(row[3]),
					Close:     parseBinanceFloat(row[4]),
					Volume:    parseBinanceFloat(row[5]),
					CreatedAt: now,
					UpdatedAt: now,
				})
			}
			if len(candles) >= 24 {
				_ = s.store.UpsertCryptoCandles(ctx, symbol, "1h", candles)
				return candles, nil
			}
		}
	}

	stored, storedErr := s.store.ListCryptoCandles(ctx, symbol, "1h", 25)
	if storedErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, storedErr
	}
	if len(stored) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("no candle data")
	}
	sort.Slice(stored, func(i, j int) bool { return stored[i].OpenTime.Before(stored[j].OpenTime) })
	return stored, nil
}

func buildPriceSnapshot(symbol string, candles []model.CryptoPriceCandle, now time.Time) model.CryptoPriceSnapshot {
	if len(candles) == 0 {
		return model.CryptoPriceSnapshot{Symbol: symbol, UpdatedAt: now}
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].OpenTime.Before(candles[j].OpenTime) })
	latest := candles[len(candles)-1]
	return model.CryptoPriceSnapshot{
		Symbol:     symbol,
		Price:      cryptoutil.Round2(latest.Close),
		Change1H:   cryptoutil.Round2(priceChange(candles, 1)),
		Change4H:   cryptoutil.Round2(priceChange(candles, 4)),
		Change24H:  cryptoutil.Round2(priceChange(candles, 24)),
		Volatility: cryptoutil.Round2(candleVolatility(candles)),
		VolumeBias: cryptoutil.Round2(volumeBias(candles)),
		UpdatedAt:  now,
	}
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
	buf.WriteString(fmt.Sprintf("%s 当前价格 %.2f，1h %.2f%%，4h %.2f%%，24h %.2f%%。", resolution.DisplayPair, price.Price, price.Change1H, price.Change4H, price.Change24H))
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

func parseBinanceMillis(value any) time.Time {
	switch typed := value.(type) {
	case float64:
		return time.UnixMilli(int64(typed)).UTC()
	case int64:
		return time.UnixMilli(typed).UTC()
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return time.UnixMilli(parsed).UTC()
		}
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return time.UnixMilli(parsed).UTC()
		}
	}
	return time.Time{}
}

func parseBinanceFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func priceChange(candles []model.CryptoPriceCandle, hours int) float64 {
	if len(candles) <= hours {
		return 0
	}
	latest := candles[len(candles)-1].Close
	prev := candles[len(candles)-1-hours].Close
	if prev == 0 {
		return 0
	}
	return (latest - prev) / prev * 100
}

func candleVolatility(candles []model.CryptoPriceCandle) float64 {
	if len(candles) < 2 {
		return 0
	}
	returns := make([]float64, 0, len(candles)-1)
	for i := 1; i < len(candles); i++ {
		prev := candles[i-1].Close
		if prev == 0 {
			continue
		}
		returns = append(returns, (candles[i].Close-prev)/prev*100)
	}
	if len(returns) == 0 {
		return 0
	}
	mean := 0.0
	for _, ret := range returns {
		mean += ret
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, ret := range returns {
		variance += math.Pow(ret-mean, 2)
	}
	return math.Sqrt(variance / float64(len(returns)))
}

func volumeBias(candles []model.CryptoPriceCandle) float64 {
	if len(candles) < 7 {
		return 0
	}
	latest := candles[len(candles)-1].Volume
	base := 0.0
	count := 0.0
	for _, candle := range candles[len(candles)-7 : len(candles)-1] {
		base += candle.Volume
		count++
	}
	if count == 0 {
		return 0
	}
	avg := base / count
	if avg == 0 {
		return 0
	}
	return latest/avg - 1
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
