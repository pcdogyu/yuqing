package scheduler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	pdf "github.com/dslipak/pdf"
	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/stockresearch"
)

const stockResearchPDFMaxBytes = 30 * 1024 * 1024

var stockResearchPDFParseItemMinTimeout = 2 * time.Minute
var stockResearchPDFContentMinTimeout = time.Second

type stockResearchPDFParseOptions struct {
	ID      int64
	Code    string
	Company string
	Kind    string
	Source  string
	Start   string
	End     string
	DryRun  bool
	Force   bool
}

type stockResearchPDFParseResult struct {
	Total        int  `json:"total"`
	ExistingPDF  int  `json:"existing_pdf"`
	NeedDownload int  `json:"need_download"`
	Parsed       int  `json:"parsed"`
	NoPDF        int  `json:"no_pdf"`
	NoText       int  `json:"no_text"`
	Failed       int  `json:"failed"`
	DryRun       bool `json:"dry_run"`
}

var stockResearchPDFTextExtractor = extractStockResearchPDFText

func (w *Worker) runStockResearchPDFParse(ctx context.Context, opts stockResearchPDFParseOptions) (stockResearchPDFParseResult, error) {
	items, err := w.loadStockResearchPDFCandidates(ctx, opts)
	if err != nil {
		return stockResearchPDFParseResult{}, err
	}
	result := stockResearchPDFParseResult{Total: len(items), DryRun: opts.DryRun}
	var errs []string
	for _, item := range items {
		if existingStockResearchPDFPath(item) != "" {
			result.ExistingPDF++
		} else if stockResearchHasPDFSource(item) {
			result.NeedDownload++
		}
		if opts.DryRun {
			if !stockResearchHasPDFSource(item) {
				result.NoPDF++
			}
			continue
		}
		update := w.buildStockResearchPDFUpdateWithTimeout(ctx, item, opts)
		switch update.PDFStatus {
		case "parsed":
			result.Parsed++
		case "no_pdf":
			result.NoPDF++
		case "no_text":
			result.NoText++
		default:
			result.Failed++
		}
		if err := w.writeStockResearchPDFUpdate(ctx, item.ID, update); err != nil {
			errs = append(errs, fmt.Sprintf("%d: %v", item.ID, err))
		}
	}
	if len(errs) > 0 {
		return result, errors.New(strings.Join(errs, "; "))
	}
	return result, nil
}

func (w *Worker) loadStockResearchPDFCandidates(ctx context.Context, opts stockResearchPDFParseOptions) ([]model.StockResearchSurvey, error) {
	if opts.ID > 0 {
		var item model.StockResearchSurvey
		if err := w.getContentJSON(ctx, fmt.Sprintf("/api/v1/stock-research/%d", opts.ID), &item); err != nil {
			return nil, err
		}
		return []model.StockResearchSurvey{item}, nil
	}
	query := url.Values{}
	query.Set("page_size", "200")
	switch {
	case strings.TrimSpace(opts.Kind) != "":
		query.Set("kind", strings.TrimSpace(opts.Kind))
	case strings.TrimSpace(opts.Source) == "cninfo_investor_relation":
		query.Set("kind", "survey")
	default:
		query.Set("kind", "report")
	}
	if opts.Code != "" {
		query.Set("code", opts.Code)
	}
	if opts.Company != "" {
		query.Set("company", opts.Company)
	}
	if opts.Source != "" {
		query.Set("source", opts.Source)
	}
	if opts.Start != "" {
		query.Set("start", opts.Start)
	}
	if opts.End != "" {
		query.Set("end", opts.End)
	}
	items := make([]model.StockResearchSurvey, 0)
	for page := 1; ; page++ {
		query.Set("page", fmt.Sprintf("%d", page))
		var list model.StockResearchListResult
		if err := w.getContentJSON(ctx, "/api/v1/stock-research?"+query.Encode(), &list); err != nil {
			return nil, err
		}
		items = append(items, list.Items...)
		if list.PageSize <= 0 || list.Total <= page*list.PageSize || len(list.Items) == 0 {
			break
		}
	}
	return items, nil
}

func (w *Worker) buildStockResearchPDFUpdateWithTimeout(ctx context.Context, item model.StockResearchSurvey, opts stockResearchPDFParseOptions) model.StockResearchPDFUpdate {
	now := time.Now().UTC().Format(time.RFC3339)
	timeout := maxDuration(w.cfg.HTTPTimeout*6, stockResearchPDFParseItemMinTimeout)
	itemCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	updates := make(chan model.StockResearchPDFUpdate, 1)
	go func() {
		updates <- w.buildStockResearchPDFUpdate(itemCtx, item, opts)
	}()

	select {
	case update := <-updates:
		return update
	case <-itemCtx.Done():
		return model.StockResearchPDFUpdate{
			PDFURL:       strings.TrimSpace(nonEmptyText(item.PDFURL, item.SourceURL)),
			PDFFilePath:  existingStockResearchPDFPath(item),
			PDFStatus:    "failed",
			PDFError:     fmt.Sprintf("PDF 解析超时: %s", timeout),
			PDFParsedAt:  now,
			PDFFetchedAt: now,
			Force:        opts.Force,
		}
	}
}

func (w *Worker) buildStockResearchPDFUpdate(ctx context.Context, item model.StockResearchSurvey, opts stockResearchPDFParseOptions) model.StockResearchPDFUpdate {
	now := time.Now().UTC().Format(time.RFC3339)
	filePath := existingStockResearchPDFPath(item)
	pdfURL := strings.TrimSpace(item.PDFURL)
	if filePath == "" {
		resolvedURL, err := w.resolveStockResearchPDFURL(ctx, item)
		if err != nil {
			return model.StockResearchPDFUpdate{PDFURL: item.PDFURL, PDFStatus: "failed", PDFError: err.Error(), PDFParsedAt: now}
		}
		pdfURL = resolvedURL
		if pdfURL == "" {
			return model.StockResearchPDFUpdate{PDFURL: item.PDFURL, PDFStatus: "no_pdf", PDFError: "未找到 PDF 链接", PDFParsedAt: now}
		}
		filePath, err = w.downloadStockResearchPDF(ctx, item, pdfURL)
		if err != nil {
			return model.StockResearchPDFUpdate{PDFURL: pdfURL, PDFStatus: "failed", PDFError: err.Error(), PDFParsedAt: now}
		}
	} else if pdfURL == "" {
		pdfURL = strings.TrimSpace(item.SourceURL)
	}
	rawText, err := stockResearchPDFTextExtractor(filePath)
	if err != nil {
		return model.StockResearchPDFUpdate{PDFURL: pdfURL, PDFFilePath: filePath, PDFStatus: "failed", PDFError: err.Error(), PDFFetchedAt: now, PDFParsedAt: now}
	}
	text := stockresearch.FormatPDFText(rawText)
	if text == "" {
		return model.StockResearchPDFUpdate{PDFURL: pdfURL, PDFFilePath: filePath, PDFStatus: "no_text", PDFError: "PDF 可能为扫描版或图片型研报", PDFFetchedAt: now, PDFParsedAt: now}
	}
	if item.SourceType == "cninfo_investor_relation" {
		text = stockResearchPDFMarkdown(item, text)
	}
	sourceUpdate := stockresearch.SourceUpdateFromPDFText(text, now)
	update := model.StockResearchPDFUpdate{
		PDFURL:            pdfURL,
		PDFFilePath:       filePath,
		PDFStatus:         "parsed",
		PDFText:           text,
		PDFFetchedAt:      now,
		PDFParsedAt:       now,
		SourceText:        sourceUpdate.SourceText,
		SourceFetchStatus: sourceUpdate.SourceFetchStatus,
		SourceFetchError:  sourceUpdate.SourceFetchError,
		SourceFetchedAt:   sourceUpdate.SourceFetchedAt,
		Force:             opts.Force,
	}
	if score, ok := w.scoreStockResearchText(ctx, item, text); ok {
		update.NLPScore = score.Score
		update.NLPRating = score.Rating
		update.NLPReason = score.Reason
		update.NLPScoredAt = now
	}
	return update
}

func existingStockResearchPDFPath(item model.StockResearchSurvey) string {
	path := filepath.Clean(strings.TrimSpace(item.PDFFilePath))
	if path == "." || path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Size() > 0 {
		return path
	}
	return ""
}

func stockResearchHasPDFSource(item model.StockResearchSurvey) bool {
	return existingStockResearchPDFPath(item) != "" ||
		strings.TrimSpace(item.PDFURL) != "" ||
		strings.TrimSpace(item.SourceURL) != ""
}

func (w *Worker) scoreStockResearchText(ctx context.Context, item model.StockResearchSurvey, text string) (model.NLPStockScoreResponse, bool) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.NLPURL), "/")
	if baseURL == "" || strings.TrimSpace(text) == "" {
		return model.NLPStockScoreResponse{}, false
	}
	var envelope struct {
		Data model.NLPStockScoreResponse `json:"data"`
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetResult(&envelope).
		SetBody(model.NLPStockScoreRequest{
			Code:  item.Code,
			Name:  item.Name,
			Title: item.Title,
			Text:  text,
		}).
		Post(baseURL + "/api/v1/nlp/stock-score")
	if err != nil || !resp.IsSuccess() {
		return model.NLPStockScoreResponse{}, false
	}
	if envelope.Data.Status == "" && len(resp.Body()) > 0 {
		_ = json.Unmarshal(resp.Body(), &envelope)
	}
	return envelope.Data, true
}

func stockResearchPDFMarkdown(item model.StockResearchSurvey, text string) string {
	var b strings.Builder
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = strings.TrimSpace(item.Code + " " + item.Name + " 投资者关系活动记录")
	}
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	if stock := strings.TrimSpace(item.Code + " " + item.Name); stock != "" {
		b.WriteString("- 股票: ")
		b.WriteString(stock)
		b.WriteString("\n")
	}
	if date := strings.TrimSpace(nonEmptyText(item.ResearchDate, item.PublishTime)); date != "" {
		b.WriteString("- 日期: ")
		b.WriteString(date)
		b.WriteString("\n")
	}
	if pdfURL := strings.TrimSpace(nonEmptyText(item.PDFURL, item.SourceURL)); pdfURL != "" {
		b.WriteString("- PDF: ")
		b.WriteString(pdfURL)
		b.WriteString("\n")
	}
	b.WriteString("\n## PDF 原文\n\n")
	b.WriteString(strings.TrimSpace(text))
	return b.String()
}

func (w *Worker) resolveStockResearchPDFURL(ctx context.Context, item model.StockResearchSurvey) (string, error) {
	if looksLikePDFURL(item.PDFURL) {
		return strings.TrimSpace(item.PDFURL), nil
	}
	if looksLikePDFURL(item.SourceURL) {
		return strings.TrimSpace(item.SourceURL), nil
	}
	sourceURL := strings.TrimSpace(item.SourceURL)
	if sourceURL == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", w.cfg.UserAgent)
	client := http.Client{Timeout: maxDuration(w.cfg.HTTPTimeout, 20*time.Second)}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("source page failed: %s", resp.Status)
	}
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", err
	}
	var found string
	doc.Find("a").EachWithBreak(func(_ int, link *goquery.Selection) bool {
		href, _ := link.Attr("href")
		resolved := resolveStockResearchURL(sourceURL, href)
		if looksLikePDFURL(resolved) {
			found = resolved
			return false
		}
		return true
	})
	return found, nil
}

func (w *Worker) downloadStockResearchPDF(ctx context.Context, item model.StockResearchSurvey, pdfURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pdfURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", w.cfg.UserAgent)
	client := http.Client{Timeout: maxDuration(w.cfg.HTTPTimeout, 20*time.Second)}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pdf download failed: %s", resp.Status)
	}
	if resp.ContentLength > stockResearchPDFMaxBytes {
		return "", fmt.Errorf("pdf too large: %d bytes", resp.ContentLength)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "pdf") && !looksLikePDFURL(pdfURL) {
		return "", fmt.Errorf("not a pdf response: %s", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, stockResearchPDFMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > stockResearchPDFMaxBytes {
		return "", fmt.Errorf("pdf too large: over %d bytes", stockResearchPDFMaxBytes)
	}
	dir := filepath.Join(stockResearchPDFRoot(w.cfg.StockResearchPDFDir), safeStockResearchPathSegment(item.SourceType))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filePath := filepath.Join(dir, stockResearchPDFFileName(item, pdfURL))
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", err
	}
	return filepath.Clean(filePath), nil
}

func (w *Worker) writeStockResearchPDFUpdate(ctx context.Context, id int64, update model.StockResearchPDFUpdate) error {
	resp, err := w.stockResearchPDFContentRequest().
		SetContext(ctx).
		SetBody(update).
		Post(fmt.Sprintf("%s/api/v1/internal/stock-research/%d/pdf", strings.TrimRight(w.cfg.ContentURL, "/"), id))
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock research pdf update failed: %s", resp.Status())
	}
	return nil
}

func (w *Worker) writeStockResearchNLPUpdate(ctx context.Context, id int64, update model.StockResearchNLPUpdate) error {
	resp, err := w.stockResearchPDFContentRequest().
		SetContext(ctx).
		SetBody(update).
		Post(fmt.Sprintf("%s/api/v1/internal/stock-research/%d/nlp", strings.TrimRight(w.cfg.ContentURL, "/"), id))
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock research nlp update failed: %s", resp.Status())
	}
	return nil
}

func (w *Worker) getContentJSON(ctx context.Context, path string, target any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	resp, err := w.stockResearchPDFContentRequest().
		SetContext(ctx).
		SetResult(&envelope).
		Get(strings.TrimRight(w.cfg.ContentURL, "/") + path)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content request failed: %s", resp.Status())
	}
	if len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, target)
}

func (w *Worker) stockResearchPDFContentRequest() *resty.Request {
	client := w.stockResearchPDFClient
	if client == nil {
		client = w.client
	}
	return client.R()
}

func extractStockResearchPDFText(filePath string) (string, error) {
	reader, err := pdf.Open(filePath)
	if err != nil {
		return "", err
	}
	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(textReader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func looksLikePDFURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" {
		return false
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".pdf")
}

func stockResearchPDFRoot(configured string) string {
	if strings.TrimSpace(configured) == "" {
		return filepath.Join("data", "stock-research-pdfs")
	}
	return filepath.Clean(strings.TrimSpace(configured))
}

func stockResearchPDFFileName(item model.StockResearchSurvey, pdfURL string) string {
	key := strings.TrimSpace(item.SourceKey)
	if key == "" {
		sum := sha1.Sum([]byte(strings.Join([]string{item.SourceType, item.Title, pdfURL}, "|")))
		key = hex.EncodeToString(sum[:])
	}
	return safeStockResearchPathSegment(key) + ".pdf"
}

func safeStockResearchPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
