package nlp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestBuildTitleAndSummary(t *testing.T) {
	if got := buildTitle(""); got != "自动生成标题" {
		t.Fatalf("expected default title, got %q", got)
	}
	if got := buildSummary("  short text  "); got != "short text" {
		t.Fatalf("expected trimmed summary, got %q", got)
	}

	longTitle := strings.Repeat("a", 30)
	if got := buildTitle(longTitle); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated title, got %q", got)
	}

	longSummary := strings.Repeat("中", 130)
	if got := buildSummary(longSummary); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated summary, got %q", got)
	}
}

func TestExtractKeywords(t *testing.T) {
	keywords := extractKeywords("btc btc btc eth eth 宏观 宏观 新闻 market market")
	if len(keywords) == 0 {
		t.Fatal("expected extracted keywords")
	}
	if keywords[0] != "btc" {
		t.Fatalf("expected btc to rank first, got %+v", keywords)
	}
}

func TestHandleSummarize(t *testing.T) {
	svc := NewService()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nlp/summarize", strings.NewReader(`{"text":"btc breaks higher and macro sentiment improves"}`))

	svc.handleSummarize(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload struct {
		Data struct {
			Title    string   `json:"title"`
			Summary  string   `json:"summary"`
			Keywords []string `json:"keywords"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload.Data.Title == "" || payload.Data.Summary == "" || len(payload.Data.Keywords) == 0 {
		t.Fatalf("expected populated response, got %+v", payload.Data)
	}
}

func TestHandleOCRAndImageClassify(t *testing.T) {
	imgData := testPNG(t)

	ocrReq := multipartRequest(t, "/api/v1/nlp/ocr", "screenshot.png", imgData)
	ocrRR := httptest.NewRecorder()
	NewService().handleOCR(ocrRR, ocrReq)
	if ocrRR.Code != http.StatusOK {
		t.Fatalf("expected OCR 200, got %d", ocrRR.Code)
	}
	var ocrEnvelope struct {
		Code    int `json:"code"`
		Results []struct {
			Data []struct {
				Text string `json:"text"`
			} `json:"data"`
		} `json:"results"`
	}
	if err := json.Unmarshal(ocrRR.Body.Bytes(), &ocrEnvelope); err != nil {
		t.Fatalf("decode OCR response: %v", err)
	}
	if ocrEnvelope.Code != http.StatusOK || len(ocrEnvelope.Results) != 1 || len(ocrEnvelope.Results[0].Data) != 1 {
		t.Fatalf("unexpected OCR response: %+v", ocrEnvelope)
	}
	if got := ocrEnvelope.Results[0].Data[0].Text; !strings.Contains(got, "screenshot") {
		t.Fatalf("expected OCR text to mention source, got %q", got)
	}

	imgReq := multipartRequest(t, "/api/v1/nlp/image", "screenshot.png", imgData)
	imgRR := httptest.NewRecorder()
	NewService().handleImageClassify(imgRR, imgReq)
	if imgRR.Code != http.StatusOK {
		t.Fatalf("expected image 200, got %d", imgRR.Code)
	}
	var imgEnvelope struct {
		Code    int `json:"code"`
		Results struct {
			Result []struct {
				Keyword string `json:"keyword"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal(imgRR.Body.Bytes(), &imgEnvelope); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if imgEnvelope.Code != http.StatusOK || len(imgEnvelope.Results.Result) == 0 {
		t.Fatalf("unexpected image response: %+v", imgEnvelope)
	}
}

func TestHandleReportPreviewAndCapabilities(t *testing.T) {
	svc := NewService()

	reportRR := httptest.NewRecorder()
	reportReq := httptest.NewRequest(http.MethodPost, "/api/v1/nlp/report-preview", strings.NewReader(`{"text":"平台工作台报告预览正文。","title":"测试标题","relatedword":"AI","publish_time":"2026-06-10 12:00:00"}`))
	svc.handleReportPreview(reportRR, reportReq)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("expected report-preview 200, got %d", reportRR.Code)
	}
	var reportEnvelope struct {
		Data model.NLPReportPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal(reportRR.Body.Bytes(), &reportEnvelope); err != nil {
		t.Fatalf("decode report-preview response: %v", err)
	}
	if reportEnvelope.Data.Title != "测试标题" || !strings.Contains(reportEnvelope.Data.Report, "平台工作台报告预览正文") {
		t.Fatalf("unexpected report-preview response: %+v", reportEnvelope.Data)
	}

	capsRR := httptest.NewRecorder()
	capsReq := httptest.NewRequest(http.MethodGet, "/api/v1/nlp/capabilities", nil)
	svc.handleCapabilities(capsRR, capsReq)
	if capsRR.Code != http.StatusOK {
		t.Fatalf("expected capabilities 200, got %d", capsRR.Code)
	}
	var capsEnvelope struct {
		Data []model.NLPCapability `json:"data"`
	}
	if err := json.Unmarshal(capsRR.Body.Bytes(), &capsEnvelope); err != nil {
		t.Fatalf("decode capabilities response: %v", err)
	}
	if len(capsEnvelope.Data) == 0 {
		t.Fatalf("expected capabilities, got %+v", capsEnvelope)
	}
	foundReport := false
	for _, capability := range capsEnvelope.Data {
		if capability.Path == "/api/v1/nlp/report-preview" && capability.Enabled {
			foundReport = true
			break
		}
	}
	if !foundReport {
		t.Fatalf("expected report-preview capability, got %+v", capsEnvelope.Data)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func multipartRequest(t *testing.T, path, filename string, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("images", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
