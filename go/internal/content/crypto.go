package content

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/cryptoutil"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func (s *Service) handleCryptoPairResolve(w http.ResponseWriter, r *http.Request) {
	resolution, err := cryptoutil.ResolvePair(strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "暂不支持该币对", nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", resolution)
}

func (s *Service) handleCryptoNews(w http.ResponseWriter, r *http.Request) {
	pair := nonEmpty(r.URL.Query().Get("pair"), r.URL.Query().Get("q"))
	result, err := s.buildCryptoNews(r, pair)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleCryptoSocial(w http.ResponseWriter, r *http.Request) {
	pair := nonEmpty(r.URL.Query().Get("pair"), r.URL.Query().Get("q"))
	result, err := s.buildCryptoSocial(r, pair)
	if err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) buildCryptoNews(r *http.Request, pair string) (model.CryptoNewsResult, error) {
	resolution, err := cryptoutil.ResolvePair(pair)
	if err != nil {
		return model.CryptoNewsResult{}, errors.New("暂不支持该币对")
	}

	page := apiutil.IntQuery(r, "page", 1)
	pageSize := apiutil.IntQuery(r, "page_size", 20)
	if pageSize > 50 {
		pageSize = 50
	}
	now := time.Now().UTC()
	start := now.Add(-48 * time.Hour).Format(time.RFC3339)

	scored := map[int64]model.CryptoEvidenceArticle{}
	for _, term := range resolution.SearchTerms {
		search, searchErr := s.store.SearchItemsFTS(r.Context(), model.ArticleFilter{
			Page:     1,
			PageSize: 60,
			Keyword:  term,
			Start:    start,
			End:      now.Format(time.RFC3339),
			UserID:   filterUserID(r),
		})
		if searchErr != nil {
			continue
		}
		for _, item := range search.Items {
			score := cryptoArticleScore(item, term, resolution, now)
			if score <= 0 {
				continue
			}
			text := strings.TrimSpace(item.Title + " " + item.Summary + " " + item.Content)
			category, label := cryptoutil.DetectReason(text)
			direction := cryptoutil.DirectionLabel(cryptoutil.ScoreDirection(text))
			article := model.CryptoEvidenceArticle{
				ID:             item.ID,
				Title:          item.Title,
				Summary:        cryptoSummary(item),
				SourceType:     item.SourceType,
				SourceURL:      item.SourceURL,
				DetailURL:      item.DetailURL,
				PublishTime:    nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04")),
				CapturedAt:     item.CapturedAt,
				Direction:      direction,
				ReasonCategory: category,
				ReasonLabel:    label,
				RelevanceScore: cryptoutil.Round2(score),
			}
			existing, ok := scored[item.ID]
			if !ok || article.RelevanceScore > existing.RelevanceScore {
				scored[item.ID] = article
			}
		}
	}

	list := make([]model.CryptoEvidenceArticle, 0, len(scored))
	for _, item := range scored {
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].RelevanceScore == list[j].RelevanceScore {
			return list[i].CapturedAt.After(list[j].CapturedAt)
		}
		return list[i].RelevanceScore > list[j].RelevanceScore
	})

	total := len(list)
	startIdx := (page - 1) * pageSize
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}
	return model.CryptoNewsResult{
		Resolution: resolution,
		Items:      list[startIdx:endIdx],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func (s *Service) buildCryptoSocial(r *http.Request, pair string) (model.CryptoSocialResult, error) {
	resolution, err := cryptoutil.ResolvePair(pair)
	if err != nil {
		return model.CryptoSocialResult{}, errors.New("暂不支持该币对")
	}

	page := apiutil.IntQuery(r, "page", 1)
	pageSize := apiutil.IntQuery(r, "page_size", 20)
	if pageSize > 50 {
		pageSize = 50
	}
	now := time.Now().UTC()
	start := now.Add(-48 * time.Hour).Format(time.RFC3339)

	scored := map[int64]model.CryptoSocialPost{}
	for _, term := range resolution.SearchTerms {
		search, searchErr := s.store.SearchItemsFTS(r.Context(), model.ArticleFilter{
			Page:     1,
			PageSize: 80,
			Keyword:  term,
			Start:    start,
			End:      now.Format(time.RFC3339),
			UserID:   filterUserID(r),
		})
		if searchErr != nil {
			continue
		}
		for _, item := range search.Items {
			if !isCryptoSocialCandidate(item) {
				continue
			}
			score := cryptoSocialScore(item, term, resolution, now)
			if score <= 0 {
				continue
			}
			text := strings.TrimSpace(strings.Join([]string{
				item.Title,
				item.Summary,
				item.Content,
				item.FromText,
				item.ExternalSourceHost,
			}, " "))
			category, label := cryptoutil.DetectReason(text)
			post := model.CryptoSocialPost{
				ID:             item.ID,
				Platform:       detectCryptoSocialPlatform(item),
				Author:         nonEmpty(item.FromText, item.ExternalSourceHost, item.SourceType),
				Title:          summarizeText(nonEmpty(item.Title, item.Summary, item.Content)),
				Content:        summarizeText(nonEmpty(item.Content, item.Summary, item.Title)),
				SourceType:     item.SourceType,
				SourceURL:      item.SourceURL,
				DetailURL:      item.DetailURL,
				PublishTime:    nonEmpty(item.PublishTime, item.PublishTimeText, item.CapturedAt.Format("2006-01-02 15:04")),
				CapturedAt:     item.CapturedAt,
				Direction:      cryptoutil.DirectionLabel(cryptoutil.ScoreDirection(text)),
				ReasonCategory: category,
				ReasonLabel:    label,
				RelevanceScore: cryptoutil.Round2(score),
				HeatScore:      cryptoutil.Round2(cryptoSocialHeatScore(item, now)),
			}
			existing, ok := scored[item.ID]
			if !ok || post.RelevanceScore > existing.RelevanceScore {
				scored[item.ID] = post
			}
		}
	}

	list := make([]model.CryptoSocialPost, 0, len(scored))
	for _, item := range scored {
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].RelevanceScore == list[j].RelevanceScore {
			if list[i].HeatScore == list[j].HeatScore {
				return list[i].CapturedAt.After(list[j].CapturedAt)
			}
			return list[i].HeatScore > list[j].HeatScore
		}
		return list[i].RelevanceScore > list[j].RelevanceScore
	})

	total := len(list)
	startIdx := (page - 1) * pageSize
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}
	return model.CryptoSocialResult{
		Resolution: resolution,
		Items:      list[startIdx:endIdx],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

func cryptoSummary(item model.Item) string {
	return summarizeText(nonEmpty(item.Summary, item.Content, item.Title))
}

func cryptoArticleScore(item model.Item, term string, resolution model.CryptoPairResolution, now time.Time) float64 {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Summary, item.Content}, " "))
	termScore := 0.0
	targets := []struct {
		term   string
		weight float64
	}{
		{resolution.Pair, 5},
		{strings.ReplaceAll(resolution.DisplayPair, "/", ""), 4.5},
		{resolution.BaseAsset, 3},
		{resolution.BaseName, 3.2},
		{resolution.BaseNameCN, 3.4},
		{term, 2.2},
	}
	for _, target := range targets {
		normalized := strings.ToLower(strings.TrimSpace(target.term))
		if normalized != "" && strings.Contains(text, normalized) {
			termScore += target.weight
		}
	}
	recencyHours := now.Sub(item.CapturedAt.UTC()).Hours()
	recencyScore := 0.0
	switch {
	case recencyHours <= 2:
		recencyScore = 3.5
	case recencyHours <= 12:
		recencyScore = 2.2
	case recencyHours <= 24:
		recencyScore = 1.4
	case recencyHours <= 48:
		recencyScore = 0.8
	}
	sourceScore := 1.0
	switch item.SourceType {
	case "headline":
		sourceScore = 1.5
	case "flash":
		sourceScore = 1.2
	}
	return termScore + recencyScore + sourceScore
}

func cryptoSocialScore(item model.Item, term string, resolution model.CryptoPairResolution, now time.Time) float64 {
	score := cryptoArticleScore(item, term, resolution, now)
	platform := strings.ToLower(detectCryptoSocialPlatform(item))
	switch platform {
	case "x", "telegram", "reddit":
		score += 1.4
	case "wechat", "weibo":
		score += 1.2
	default:
		score += 0.8
	}
	return score + cryptoSocialHeatScore(item, now)
}

func cryptoSocialHeatScore(item model.Item, now time.Time) float64 {
	ageHours := now.Sub(item.CapturedAt.UTC()).Hours()
	heat := 0.0
	switch {
	case ageHours <= 1:
		heat = 2.8
	case ageHours <= 6:
		heat = 2.0
	case ageHours <= 24:
		heat = 1.2
	default:
		heat = 0.5
	}
	text := strings.ToLower(strings.Join([]string{item.Title, item.Summary, item.Content}, " "))
	if strings.Contains(text, "breaking") || strings.Contains(text, "突发") {
		heat += 0.8
	}
	if strings.Contains(text, "whale") || strings.Contains(text, "巨鲸") || strings.Contains(text, "etf") {
		heat += 0.4
	}
	return heat
}

func isCryptoSocialCandidate(item model.Item) bool {
	candidate := strings.ToLower(strings.Join([]string{
		item.SourceType,
		item.SourceURL,
		item.DetailURL,
		item.ExternalSourceHost,
		item.FromText,
	}, " "))
	for _, marker := range []string{
		"social", "wechat", "crypto_x", "x.com", "twitter", "crypto_telegram", "telegram", "t.me", "reddit",
		"discord", "weibo", "xiaohongshu", "zhihu", "forum", "community",
	} {
		if strings.Contains(candidate, marker) {
			return true
		}
	}
	return false
}

func detectCryptoSocialPlatform(item model.Item) string {
	candidate := strings.ToLower(strings.Join([]string{
		item.SourceType,
		item.SourceURL,
		item.DetailURL,
		item.ExternalSourceHost,
		item.FromText,
	}, " "))
	switch {
	case strings.Contains(candidate, "crypto_x"), strings.Contains(candidate, "x.com"), strings.Contains(candidate, "twitter"):
		return "X"
	case strings.Contains(candidate, "crypto_telegram"), strings.Contains(candidate, "telegram"), strings.Contains(candidate, "t.me"):
		return "Telegram"
	case strings.Contains(candidate, "reddit"):
		return "Reddit"
	case strings.Contains(candidate, "wechat"):
		return "WeChat"
	case strings.Contains(candidate, "weibo"):
		return "Weibo"
	case strings.Contains(candidate, "xiaohongshu"):
		return "Xiaohongshu"
	case strings.Contains(candidate, "zhihu"):
		return "Zhihu"
	default:
		return "Community"
	}
}
