package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type Store interface {
	Overview(ctx context.Context) (model.Overview, error)
	BuildDashboardSnapshot(ctx context.Context) (model.DashboardSnapshot, error)
	RefreshAnalysis(ctx context.Context) (model.DashboardSnapshot, error)
	GetAnalysisSnapshot(ctx context.Context, scope string, scopeID int64) (model.AnalysisSnapshot, error)
	ListTrendPoints(ctx context.Context) ([]model.TrendPoint, error)
	ListSourceBreakdowns(ctx context.Context) ([]model.SourceBreakdown, error)
	ListKeywordHotspots(ctx context.Context) ([]model.KeywordHotspot, error)
	BuildEmotionAnalysis(ctx context.Context, projectID int64) (model.EmotionAnalysis, error)
	BuildEventOverview(ctx context.Context, projectID int64) ([]model.EventOverview, error)
	BuildPropagationAnalysis(ctx context.Context, projectID int64) (model.PropagationAnalysis, error)
	BuildThemeInsights(ctx context.Context, projectID int64) ([]model.ThemeInsight, error)
	BuildPublicOpinionEvents(ctx context.Context, projectID int64) ([]model.PublicOpinionEvent, error)
	BuildPublicOpinionReports(ctx context.Context, projectID int64) ([]model.PublicOpinionReport, error)
	UpsertCryptoCandles(ctx context.Context, symbol, interval string, candles []model.CryptoPriceCandle) error
	ListCryptoCandles(ctx context.Context, symbol, interval string, limit int) ([]model.CryptoPriceCandle, error)
	GetCryptoInsightSnapshot(ctx context.Context, pair, horizonSet string) (model.CryptoInsightSnapshot, error)
	UpsertCryptoInsightSnapshot(ctx context.Context, snapshot model.CryptoInsightSnapshot) error
	RecordTaskRun(ctx context.Context, name, status, message string, startedAt time.Time, finishedAt *time.Time) error
}

type Service struct {
	cfg    config.Config
	store  Store
	client *resty.Client
}

func NewService(cfg config.Config, store Store) *Service {
	return &Service{
		cfg:   cfg,
		store: store,
		client: resty.New().
			SetTimeout(cfg.HTTPTimeout).
			SetHeader("X-Service-Token", cfg.ServiceToken).
			SetHeader("User-Agent", cfg.UserAgent),
	}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Get("/api/v1/analysis/overview", s.handleOverview)
	r.Get("/api/v1/analysis/trends", s.handleTrends)
	r.Get("/api/v1/analysis/sources", s.handleSources)
	r.Get("/api/v1/analysis/keywords", s.handleKeywords)
	r.Get("/api/v1/analysis/emotions", s.handleEmotions)
	r.Get("/api/v1/analysis/event-overview", s.handleEventOverview)
	r.Get("/api/v1/analysis/propagation", s.handlePropagation)
	r.Get("/api/v1/analysis/themes", s.handleThemes)
	r.Get("/api/v1/public-opinion/enrich", s.handlePublicOpinionEnrich)
	r.Get("/api/v1/public-opinion/events", s.handlePublicOpinionEvents)
	r.Get("/api/v1/public-opinion/reports", s.handlePublicOpinionReports)
	r.Get("/api/v1/crypto/insights", s.handleCryptoInsights)
	r.Post("/api/v1/admin/tasks/analysis/refresh", s.handleRefresh)
	return r
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
}

func (s *Service) handleOverview(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.store.GetAnalysisSnapshot(r.Context(), "system", 0)
	if err == nil {
		var payload any
		_ = json.Unmarshal([]byte(snapshot.Payload), &payload)
		apiutil.WriteJSON(w, http.StatusOK, "ok", payload)
		return
	}
	live, err := s.store.BuildDashboardSnapshot(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", live)
}

func (s *Service) handleTrends(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListTrendPoints(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleSources(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListSourceBreakdowns(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleKeywords(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListKeywordHotspots(r.Context())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleEmotions(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildEmotionAnalysis(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleEventOverview(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildEventOverview(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handlePropagation(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildPropagationAnalysis(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleThemes(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildThemeInsights(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handlePublicOpinionEnrich(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.buildPublicOpinionBundle(r.Context(), r.URL.Query())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", bundle)
}

func (s *Service) handlePublicOpinionEvents(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildPublicOpinionEvents(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handlePublicOpinionReports(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.BuildPublicOpinionReports(r.Context(), queryProjectID(r))
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func (s *Service) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Service-Token") != s.cfg.ServiceToken {
		apiutil.WriteJSON(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	startedAt := time.Now().UTC()
	data, err := s.store.RefreshAnalysis(r.Context())
	if err != nil {
		finishedAt := time.Now().UTC()
		_ = s.store.RecordTaskRun(r.Context(), "analysis:refresh", "failed", err.Error(), startedAt, &finishedAt)
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	finishedAt := time.Now().UTC()
	_ = s.store.RecordTaskRun(r.Context(), "analysis:refresh", "success", "analysis refreshed", startedAt, &finishedAt)
	apiutil.WriteJSON(w, http.StatusOK, "ok", data)
}

func queryProjectID(r *http.Request) int64 {
	value := r.URL.Query().Get("project_id")
	if value == "" {
		return 0
	}
	projectID, _ := strconv.ParseInt(value, 10, 64)
	return projectID
}

func (s *Service) buildPublicOpinionBundle(ctx context.Context, query url.Values) (model.PublicOpinionAnalysisBundle, error) {
	criteria := model.PublicOpinionAnalysisBundle{
		EventName:      strings.TrimSpace(query.Get("eventname")),
		EventKeywords:  strings.TrimSpace(query.Get("eventkeywords")),
		EventStopWords: strings.TrimSpace(query.Get("eventstopwords")),
		EventStartTime: normalizeLegacyStartTime(query.Get("eventstarttime")),
		EventEndTime:   normalizeLegacyEndTime(query.Get("eventendtime")),
	}
	page := intQueryValues(query, "page", 1)
	pageSize := intQueryValues(query, "page_size", 50)
	items, err := s.fetchPublicOpinionArticles(ctx, criteria, page, pageSize)
	if err != nil {
		return model.PublicOpinionAnalysisBundle{}, err
	}
	back := map[string]any{
		"summary":   summarizeText(strings.Join(articleTitles(items), "，")),
		"keywords":  criteria.EventKeywords,
		"stopwords": criteria.EventStopWords,
	}
	events := make([]map[string]any, 0, len(items))
	hot := make([]map[string]any, 0, len(items))
	figures := make([]map[string]any, 0, len(items))
	objects := make([]map[string]any, 0, len(items))
	media := make([]map[string]any, 0, len(items))
	for i, item := range items {
		events = append(events, map[string]any{
			"title":        item.Title,
			"publish_time": firstNonEmpty(item.PublishTimeText, item.PublishTime),
			"source_name":  firstNonEmpty(item.FromText, item.SourceType),
			"source_url":   firstNonEmpty(item.SourceURL, item.DetailURL),
		})
		if i < 8 {
			hot = append(hot, map[string]any{
				"topic":           item.Title,
				"publish_time":    firstNonEmpty(item.PublishTimeText, item.PublishTime),
				"source_name":     firstNonEmpty(item.FromText, item.SourceType),
				"source_url":      firstNonEmpty(item.SourceURL, item.DetailURL),
				"original_weight": len(item.Title) + len(item.Content),
			})
		}
		figures = append(figures, map[string]any{
			"name":       firstNonEmpty(item.FromText, item.SourceType),
			"author_url": firstNonEmpty(item.SourceURL, item.DetailURL),
			"content":    truncateRunes(item.Title+" "+item.Summary, 80),
			"value":      len(item.Title),
		})
		objects = append(objects, map[string]any{
			"title":        item.Title,
			"author":       firstNonEmpty(item.FromText, item.SourceType),
			"publish_time": firstNonEmpty(item.PublishTimeText, item.PublishTime),
			"hot":          len(item.Title) + len(item.Content),
		})
		media = append(media, map[string]any{
			"name":        firstNonEmpty(item.FromText, item.SourceType),
			"logo":        "",
			"abstract":    truncateRunes(item.Summary, 90),
			"source_name": firstNonEmpty(item.SourceType, item.FromText),
			"fans":        len(item.Title) * 10,
			"publishs":    len(item.Content),
		})
	}
	stats := map[string]any{
		"website":      bucketToNamedValues(bucketSourceCounts(items, []string{"flash", "headline"})),
		"weibo":        bucketToNamedValues(bucketSourceCounts(items, []string{"weibo"})),
		"social_media": bucketToNamedValues(bucketSourceCounts(items, []string{"wechat", "social"})),
		"wemedia":      bucketToNamedValues(bucketSourceCounts(items, nil)),
	}
	propagation := map[string]any{
		"media": media,
		"source": map[string]any{
			"all":     sourceAnalysisBuckets(items),
			"clinet":  sourceAnalysisBuckets(items),
			"website": sourceAnalysisBuckets(items),
			"BBS":     sourceAnalysisBuckets(items),
			"wechat":  sourceAnalysisBuckets(items),
			"weibo":   sourceAnalysisBuckets(items),
		},
	}
	netizens := map[string]any{
		"relation": map[string]any{
			"data":  []any{},
			"links": []any{},
		},
		"figure": figures,
		"object": objects,
	}
	trace := map[string]any{
		"backAnalysis": back,
		"eventContext": events,
	}
	thematic := map[string]any{
		"view":    hot[:minInt(len(hot), 5)],
		"media":   media[:minInt(len(media), 5)],
		"netizen": figures[:minInt(len(figures), 5)],
	}
	return model.PublicOpinionAnalysisBundle{
		EventName:           criteria.EventName,
		EventKeywords:       criteria.EventKeywords,
		EventStopWords:      criteria.EventStopWords,
		EventStartTime:      criteria.EventStartTime,
		EventEndTime:        criteria.EventEndTime,
		EmotionalIndex:      "3",
		ArticleCount:        len(items),
		BackAnalysis:        mustJSONString(back),
		EventContext:        mustJSONString(events),
		EventTrace:          mustJSONString(trace),
		HotAnalysis:         mustJSONString(hot),
		NetizensAnalysis:    mustJSONString(netizens),
		Statistics:          mustJSONString(stats),
		PropagationAnalysis: mustJSONString(propagation),
		ThematicAnalysis:    mustJSONString(thematic),
		UnscrambleContent:   mustJSONString(map[string]any{"content": summarizeText(criteria.EventName + " " + criteria.EventKeywords)}),
		ContentAnalysis:     summarizeText(strings.Join(articleTitles(items), "；")),
	}, nil
}

func (s *Service) fetchPublicOpinionArticles(ctx context.Context, criteria model.PublicOpinionAnalysisBundle, page int, pageSize int) ([]model.Item, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	query.Set("q", criteria.EventKeywords)
	query.Set("start", criteria.EventStartTime)
	query.Set("end", criteria.EventEndTime)
	var envelope struct {
		Code int                `json:"code"`
		Data model.SearchResult `json:"data"`
	}
	resp, err := s.client.R().Get(s.cfg.ContentURL + "/api/v1/search/full?" + query.Encode())
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf(resp.Status())
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return nil, err
	}
	stopWords := splitLegacyLabels(criteria.EventStopWords)
	filtered := make([]model.Item, 0, len(envelope.Data.Items))
	for _, item := range envelope.Data.Items {
		if containsAny(item.Title+" "+item.Content, stopWords) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func normalizeLegacyStartTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "00:00:00") {
		return value
	}
	return value + " 00:00:00"
}

func normalizeLegacyEndTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "23:59:59") {
		return value
	}
	return value + " 23:59:59"
}

func splitLegacyLabels(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '|' || r == '/' || r == '\n' || r == '\t'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func containsAny(text string, words []string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, word := range words {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(word))) {
			return true
		}
	}
	return false
}

func summarizeText(text string) string {
	return truncateRunes(strings.TrimSpace(text), 140)
}

func truncateRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if limit <= 0 || len(runes) <= limit {
		return strings.TrimSpace(text)
	}
	return string(runes[:limit]) + "..."
}

func articleTitles(items []model.Item) []string {
	titles := make([]string, 0, len(items))
	for _, item := range items {
		if title := strings.TrimSpace(item.Title); title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}

func bucketSourceCounts(items []model.Item, kinds []string) map[string]int {
	allowed := map[string]struct{}{}
	for _, kind := range kinds {
		allowed[strings.ToLower(strings.TrimSpace(kind))] = struct{}{}
	}
	counts := map[string]int{}
	for _, item := range items {
		name := strings.TrimSpace(firstNonEmpty(item.FromText, item.SourceType))
		if name == "" {
			name = "未知来源"
		}
		if len(allowed) > 0 {
			if _, ok := allowed[strings.ToLower(strings.TrimSpace(item.SourceType))]; !ok {
				continue
			}
		}
		counts[name]++
	}
	return counts
}

func bucketToNamedValues(counts map[string]int) []map[string]any {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	result := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		result = append(result, map[string]any{"name": key, "value": counts[key]})
	}
	return result
}

func sourceAnalysisBuckets(items []model.Item) []map[string]any {
	counts := bucketSourceCounts(items, nil)
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	result := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		result = append(result, map[string]any{"name": key, "value": counts[key]})
	}
	return result
}

func mustJSONString(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intQueryValues(values url.Values, key string, fallback int) int {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
