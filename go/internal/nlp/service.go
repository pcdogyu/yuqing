package nlp

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/apiutil"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		apiutil.WriteJSON(w, http.StatusOK, "ok", map[string]string{"status": "ok"})
	})
	r.Post("/api/v1/nlp/summarize", s.handleSummarize)
	r.Post("/api/v1/nlp/title", s.handleTitle)
	r.Post("/api/v1/nlp/keywords", s.handleKeywords)
	return r
}

func (s *Service) handleSummarize(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", model.NLPResponse{
		Title:    buildTitle(req.Text),
		Summary:  buildSummary(req.Text),
		Keywords: extractKeywords(req.Text),
	})
}

func (s *Service) handleTitle(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", model.NLPResponse{Title: buildTitle(req.Text)})
}

func (s *Service) handleKeywords(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", model.NLPResponse{Keywords: extractKeywords(req.Text)})
}

func decodeRequest(w http.ResponseWriter, r *http.Request) (model.NLPRequest, bool) {
	var req model.NLPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return model.NLPRequest{}, false
	}
	return req, true
}

func buildTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "自动生成标题"
	}
	runes := []rune(text)
	if len(runes) > 24 {
		return string(runes[:24]) + "..."
	}
	return text
}

func buildSummary(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) > 120 {
		return string(runes[:120]) + "..."
	}
	return text
}

func extractKeywords(text string) []string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) {
			return r
		}
		return ' '
	}, text)
	words := strings.Fields(normalized)
	counts := make(map[string]int)
	for _, word := range words {
		if len([]rune(word)) < 2 {
			continue
		}
		counts[word]++
	}
	type pair struct {
		Word  string
		Count int
	}
	pairs := make([]pair, 0, len(counts))
	for word, count := range counts {
		pairs = append(pairs, pair{Word: word, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Word < pairs[j].Word
		}
		return pairs[i].Count > pairs[j].Count
	})
	limit := 5
	if len(pairs) < limit {
		limit = len(pairs)
	}
	result := make([]string, 0, limit)
	for idx := 0; idx < limit; idx++ {
		result = append(result, pairs[idx].Word)
	}
	return result
}
