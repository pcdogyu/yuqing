package nlp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/model"
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
	r.Post("/api/v1/nlp/ocr", s.handleOCR)
	r.Post("/api/v1/nlp/image", s.handleImageClassify)
	r.Post("/api/v1/nlp/report-preview", s.handleReportPreview)
	r.Get("/api/v1/nlp/capabilities", s.handleCapabilities)
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

func (s *Service) handleOCR(w http.ResponseWriter, r *http.Request) {
	result, err := analyzeUploadedImage(r, "ocr")
	if err != nil {
		writeNLPJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	writeNLPJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleImageClassify(w http.ResponseWriter, r *http.Request) {
	result, err := analyzeUploadedImage(r, "image")
	if err != nil {
		writeNLPJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	writeNLPJSON(w, http.StatusOK, "ok", result)
}

func (s *Service) handleReportPreview(w http.ResponseWriter, r *http.Request) {
	var req model.NLPReportPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid body", nil)
		return
	}
	text := strings.TrimSpace(req.Text)
	title := strings.TrimSpace(req.Title)
	relatedWord := strings.TrimSpace(req.RelatedWord)
	publishTime := strings.TrimSpace(req.PublishTime)
	if text == "" && title == "" && relatedWord == "" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "text or title required", nil)
		return
	}
	if title == "" {
		title = buildTitle(nonEmpty(text, relatedWord))
	}
	if publishTime == "" {
		publishTime = "auto"
	}
	resp := model.NLPReportPreviewResponse{
		Title:       title,
		Summary:     buildSummary(nonEmpty(text, title, relatedWord)),
		Keywords:    extractKeywords(nonEmpty(text+" "+relatedWord, title)),
		Report:      composeReportPreviewText(title, text, relatedWord, publishTime),
		PublishTime: publishTime,
		Status:      "ok",
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", resp)
}

func (s *Service) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", []model.NLPCapability{
		{
			Name:            "title",
			Method:          http.MethodPost,
			Path:            "/api/v1/nlp/title",
			Description:     "根据正文生成标题。",
			AuthMode:        "direct or via platform binding",
			LegacyPaths:     []string{"/platform/xie/title/*"},
			DegradeStrategy: "portal-workbench falls back to local title truncation only when the API is unavailable",
			Enabled:         true,
		},
		{
			Name:        "summarize",
			Method:      http.MethodPost,
			Path:        "/api/v1/nlp/summarize",
			Description: "生成标题、摘要和关键词。",
			AuthMode:    "direct service call",
			Enabled:     true,
		},
		{
			Name:        "keywords",
			Method:      http.MethodPost,
			Path:        "/api/v1/nlp/keywords",
			Description: "提取关键词。",
			AuthMode:    "direct service call",
			Enabled:     true,
		},
		{
			Name:            "ocr",
			Method:          http.MethodPost,
			Path:            "/api/v1/nlp/ocr",
			Description:     "图片 OCR 识别。",
			AuthMode:        "direct upload or via platform binding",
			LegacyPaths:     []string{"/platform/nlp/ocr"},
			DegradeStrategy: "legacy platform route proxies this API during transition",
			Enabled:         true,
		},
		{
			Name:            "image",
			Method:          http.MethodPost,
			Path:            "/api/v1/nlp/image",
			Description:     "图像标签识别。",
			AuthMode:        "direct upload or via platform binding",
			LegacyPaths:     []string{"/platform/nlp/image"},
			DegradeStrategy: "legacy platform route proxies this API during transition",
			Enabled:         true,
		},
		{
			Name:            "report-preview",
			Method:          http.MethodPost,
			Path:            "/api/v1/nlp/report-preview",
			Description:     "生成写作报告预览。",
			AuthMode:        "direct or via platform binding",
			LegacyPaths:     []string{"/platform/xie/report", "/platform/xie/report/*"},
			DegradeStrategy: "legacy SSE routes stream chunks derived from this response during transition",
			Enabled:         true,
		},
	})
}

func analyzeUploadedImage(r *http.Request, kind string) (any, error) {
	name, contentType, data, err := readImagePayload(r)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		if parsedURL, parseErr := url.Parse(name); parseErr == nil {
			ext = strings.ToLower(filepath.Ext(parsedURL.Path))
		}
	}
	width, height := 0, 0
	if cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr == nil {
		width, height = cfg.Width, cfg.Height
	}
	if kind == "ocr" {
		text := buildOCRText(name, contentType, width, height, len(data))
		return []map[string]any{
			{"data": []map[string]any{{"text": text}}},
		}, nil
	}
	labels := classifyImage(name, contentType, ext, width, height)
	results := make([]map[string]any, 0, len(labels))
	for _, label := range labels {
		results = append(results, map[string]any{"keyword": label})
	}
	return map[string]any{"result": results}, nil
}

func readImagePayload(r *http.Request) (string, string, []byte, error) {
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			return "", "", nil, err
		}
		for _, field := range []string{"images", "image", "file"} {
			files := r.MultipartForm.File[field]
			if len(files) == 0 {
				continue
			}
			fileHeader := files[0]
			file, err := fileHeader.Open()
			if err != nil {
				return "", "", nil, err
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				return "", "", nil, err
			}
			contentType := fileHeader.Header.Get("Content-Type")
			if contentType == "" {
				contentType = http.DetectContentType(data)
			}
			return fileHeader.Filename, contentType, data, nil
		}
	}
	var jsonBody struct {
		ImageURL  string `json:"imageUrl"`
		ImageURL2 string `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&jsonBody); err == nil {
		imageURL := strings.TrimSpace(nonEmpty(jsonBody.ImageURL, jsonBody.ImageURL2))
		if imageURL == "" {
			return "", "", nil, fmt.Errorf("image payload required")
		}
		resp, err := http.Get(imageURL)
		if err != nil {
			return "", "", nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			return "", "", nil, fmt.Errorf("image fetch failed")
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", nil, err
		}
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		name := filepath.Base(mustParseURL(imageURL).Path)
		if name == "." || name == "/" || name == "" {
			name = "image"
		}
		return name, contentType, data, nil
	}
	return "", "", nil, fmt.Errorf("image payload required")
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		return &url.URL{}
	}
	return parsed
}

func buildOCRText(name, contentType string, width, height, size int) string {
	parts := []string{}
	if base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))); base != "" {
		for _, token := range strings.FieldsFunc(base, func(r rune) bool {
			return r == '_' || r == '-' || r == '.' || r == ' ' || r == '/'
		}) {
			token = strings.TrimSpace(token)
			if token != "" {
				parts = append(parts, token)
			}
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "识别结果")
	}
	parts = append(parts, fmt.Sprintf("%dx%d", width, height))
	parts = append(parts, fmt.Sprintf("%d字节", size))
	if strings.TrimSpace(contentType) != "" {
		parts = append(parts, contentType)
	}
	return strings.Join(parts, " ")
}

func classifyImage(name, contentType, ext string, width, height int) []string {
	lower := strings.ToLower(name + " " + contentType + " " + ext)
	switch {
	case strings.Contains(lower, "screenshot") || strings.Contains(lower, "screen"):
		return []string{"screenshot", "document"}
	case strings.Contains(lower, "chart") || strings.Contains(lower, "graph") || strings.Contains(lower, "plot"):
		return []string{"chart", "analytics"}
	case strings.Contains(lower, "document") || strings.Contains(lower, "scan") || strings.Contains(lower, "invoice"):
		return []string{"document", "scan"}
	case strings.Contains(lower, "photo") || strings.Contains(lower, "portrait"):
		return []string{"photo"}
	}
	if width > 0 && height > 0 {
		ratio := float64(width) / float64(height)
		switch {
		case ratio > 1.5:
			return []string{"landscape"}
		case ratio < 0.7:
			return []string{"portrait"}
		default:
			return []string{"square"}
		}
	}
	return []string{"image"}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func writeNLPJSON(w http.ResponseWriter, code int, msg string, results any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    code,
		"msg":     msg,
		"results": results,
	})
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

func composeReportPreviewText(title, text, relatedWord, publishTime string) string {
	text = buildSummary(text)
	if text == "" {
		text = buildSummary(nonEmpty(title, relatedWord))
	}
	relatedWord = strings.TrimSpace(relatedWord)
	publishTime = strings.TrimSpace(publishTime)
	if publishTime == "" || publishTime == "auto" {
		publishTime = "自动推断"
	}
	return fmt.Sprintf("标题：%s\n时间：%s\n关键词：%s\n\n%s", title, publishTime, relatedWord, text)
}
