package scheduler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html/charset"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

var (
	eastMoneyInitDataPattern = regexp.MustCompile(`(?s)var\s+initdata\s*=\s*(\{.*?\});`)
	eastMoneyZWInfoPattern   = regexp.MustCompile(`(?s)var\s+zwinfo\s*=\s*(\{.*?\});`)
	stockResearchCodePattern = regexp.MustCompile(`([^\s（(：:，,、]+)[（(](\d{6})[）)]`)
)

var eastMoneyReportAPIURL = "https://reportapi.eastmoney.com/report/list"

func (w *Worker) runStockResearchCrawl(ctx context.Context) error {
	end := stockResearchToday()
	start := end.AddDate(0, 0, -7)
	return w.runStockResearchCrawlForRange(ctx, stockResearchCrawlOptions{
		Start: start.Format("2006-01-02"),
		End:   end.Format("2006-01-02"),
	})
}

func (w *Worker) runStockResearchBackfill(ctx context.Context, opts stockResearchCrawlOptions) error {
	if strings.TrimSpace(opts.End) == "" {
		opts.End = stockResearchToday().Format("2006-01-02")
	}
	if strings.TrimSpace(opts.Start) == "" {
		day, err := time.Parse("2006-01-02", opts.End)
		if err != nil {
			day = stockResearchToday()
		}
		opts.Start = day.AddDate(-1, 0, 0).Format("2006-01-02")
	}
	return w.runStockResearchCrawlForRange(ctx, opts)
}

type stockResearchCrawlOptions struct {
	Code    string
	Company string
	Start   string
	End     string
}

func (w *Worker) runStockResearchCrawlForRange(ctx context.Context, opts stockResearchCrawlOptions) error {
	items := make([]model.StockResearchSurvey, 0)
	var errs []string
	if externalItems, err := w.fetchExternalStockResearch(ctx, opts); err != nil {
		if strings.TrimSpace(w.cfg.StockResearchURL) != "" {
			errs = append(errs, err.Error())
		}
	} else {
		items = append(items, externalItems...)
	}
	if w.cfg.StockResearchPublicEnabled {
		if sinaItems, err := w.fetchSinaFinanceReports(ctx, opts); err != nil {
			log.Warn().Err(err).Str("source", "sina_finance_report").Msg("stock research public source skipped")
		} else {
			items = append(items, sinaItems...)
		}
		if eastMoneyItems, err := w.fetchEastMoneyReports(ctx, opts); err != nil {
			log.Warn().Err(err).Str("source", "eastmoney_report").Msg("stock research public source skipped")
		} else {
			items = append(items, eastMoneyItems...)
		}
		if sohuItems, err := w.fetchSohuFinanceReports(ctx); err != nil {
			log.Warn().Err(err).Str("source", "sohu_finance_report").Msg("stock research public source skipped")
		} else {
			items = append(items, filterStockResearchItems(sohuItems, opts)...)
		}
	}
	items = dedupeStockResearch(items)
	if len(items) == 0 && len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	payload := map[string]any{"items": items}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/internal/stock-research/batch")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content stock research upsert failed: %s", resp.Status())
	}
	log.Info().
		Str("start", opts.Start).
		Str("end", opts.End).
		Str("code", opts.Code).
		Str("company", opts.Company).
		Int("items", len(items)).
		Msg("stock research crawl completed")
	if len(items) > 0 && shouldAutoParseStockResearchPDF(opts) {
		if result, pdfErr := w.runStockResearchPDFParse(ctx, stockResearchPDFParseOptions{
			Code:    opts.Code,
			Company: opts.Company,
			Start:   opts.Start,
			End:     opts.End,
		}); pdfErr != nil {
			log.Warn().Err(pdfErr).Interface("result", result).Msg("stock research pdf parse finished with errors")
		}
	}
	return nil
}

func shouldAutoParseStockResearchPDF(opts stockResearchCrawlOptions) bool {
	return strings.TrimSpace(opts.Code) != "" || strings.TrimSpace(opts.Company) != ""
}

func (w *Worker) fetchExternalStockResearch(ctx context.Context, opts stockResearchCrawlOptions) ([]model.StockResearchSurvey, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(w.cfg.StockResearchURL), "/")
	if baseURL == "" {
		return nil, nil
	}
	query := url.Values{}
	query.Set("start", opts.Start)
	query.Set("end", opts.End)
	if opts.Code != "" {
		query.Set("code", opts.Code)
	}
	if opts.Company != "" {
		query.Set("company", opts.Company)
	}
	if token := strings.TrimSpace(w.cfg.TuShareToken); token != "" {
		query.Set("tushare_token", token)
	}
	resp, err := w.client.R().
		SetContext(ctx).
		Get(baseURL + "/api/stock-research?" + query.Encode())
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("stock research endpoint failed: %s", resp.Status())
	}
	var envelope struct {
		Items []model.StockResearchSurvey `json:"items"`
		Data  struct {
			Items []model.StockResearchSurvey `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Items) == 0 && len(envelope.Data.Items) > 0 {
		envelope.Items = envelope.Data.Items
	}
	return normalizeStockResearchSourceItems(envelope.Items, "akshare_stock_research"), nil
}

func (w *Worker) fetchSinaFinanceReports(ctx context.Context, opts stockResearchCrawlOptions) ([]model.StockResearchSurvey, error) {
	if strings.TrimSpace(w.cfg.SinaFinanceReportURL) == "" {
		return nil, nil
	}
	listURL := w.cfg.SinaFinanceReportURL
	if code := strings.TrimSpace(opts.Code); code != "" {
		listURL = sinaFinanceSearchURL(listURL, code)
	}
	doc, _, err := w.fetchStockResearchDocument(ctx, listURL, "sina_finance_report")
	if err != nil {
		return nil, err
	}
	items := parseSinaFinanceReportDocument(doc, listURL, time.Now().UTC())
	items = filterStockResearchItems(items, opts)
	return normalizeStockResearchSourceItems(items, "sina_finance_report"), nil
}

func (w *Worker) fetchEastMoneyReports(ctx context.Context, opts stockResearchCrawlOptions) ([]model.StockResearchSurvey, error) {
	if strings.TrimSpace(w.cfg.EastMoneyReportURL) == "" {
		return nil, nil
	}
	items, err := w.fetchEastMoneyReportsByAPI(ctx, opts)
	if err != nil {
		if strings.TrimSpace(opts.Code) != "" {
			return nil, err
		}
		_, body, err := w.fetchStockResearchDocument(ctx, w.cfg.EastMoneyReportURL, "eastmoney_report")
		if err != nil {
			return nil, err
		}
		items, err = parseEastMoneyReportDocument(body, w.cfg.EastMoneyReportURL, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		items = filterStockResearchItems(items, opts)
	}
	if shouldAutoParseStockResearchPDF(opts) {
		items = w.enrichEastMoneyReportDetails(ctx, items)
	}
	return normalizeStockResearchSourceItems(items, "eastmoney_report"), nil
}

func (w *Worker) fetchEastMoneyReportsByAPI(ctx context.Context, opts stockResearchCrawlOptions) ([]model.StockResearchSurvey, error) {
	items := make([]model.StockResearchSurvey, 0)
	pageSize := 50
	for page := 1; ; page++ {
		query := url.Values{}
		query.Set("industryCode", "*")
		query.Set("pageSize", fmt.Sprintf("%d", pageSize))
		query.Set("industry", "*")
		query.Set("rating", "*")
		query.Set("ratingChange", "*")
		query.Set("beginTime", normalizeStockResearchDate(opts.Start))
		query.Set("endTime", normalizeStockResearchDate(opts.End))
		query.Set("pageNo", fmt.Sprintf("%d", page))
		query.Set("fields", "")
		query.Set("qType", "0")
		query.Set("orgCode", "")
		query.Set("code", strings.TrimSpace(opts.Code))
		query.Set("rcode", "")
		body, err := w.fetchStockResearchText(ctx, eastMoneyReportAPIURL+"?"+query.Encode(), "eastmoney_report_api")
		if err != nil {
			return nil, err
		}
		pageItems, total, err := parseEastMoneyReportAPI(body, w.cfg.EastMoneyReportURL, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		rawCount := len(pageItems)
		pageItems = filterStockResearchItems(pageItems, opts)
		items = append(items, pageItems...)
		if total <= page*pageSize || rawCount == 0 {
			break
		}
	}
	return items, nil
}

func (w *Worker) fetchSohuFinanceReports(ctx context.Context) ([]model.StockResearchSurvey, error) {
	return w.fetchFinanceReportHTML(ctx, w.cfg.SohuFinanceReportURL, "sohu_finance_report")
}

func (w *Worker) fetchFinanceReportHTML(ctx context.Context, rawURL string, sourceType string) ([]model.StockResearchSurvey, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}
	doc, _, err := w.fetchStockResearchDocument(ctx, rawURL, sourceType)
	if err != nil {
		return nil, err
	}
	items := parseFinanceReportDocument(doc, rawURL, sourceType, time.Now().UTC())
	return normalizeStockResearchSourceItems(items, sourceType), nil
}

func (w *Worker) fetchStockResearchDocument(ctx context.Context, rawURL string, sourceType string) (*goquery.Document, string, error) {
	body, err := w.fetchStockResearchText(ctx, rawURL, sourceType)
	if err != nil {
		return nil, "", err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	return doc, body, nil
}

func (w *Worker) fetchStockResearchText(ctx context.Context, rawURL string, sourceType string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	resp, err := w.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", w.cfg.UserAgent).
		SetHeader("Referer", "https://data.eastmoney.com/report/stock.jshtml").
		Get(rawURL)
	if err != nil {
		return "", err
	}
	if !resp.IsSuccess() {
		return "", fmt.Errorf("%s failed: %s", sourceType, resp.Status())
	}
	reader, err := charset.NewReader(bytes.NewReader(resp.Body()), resp.Header().Get("Content-Type"))
	if err != nil {
		reader = bytes.NewReader(resp.Body())
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sinaFinanceSearchURL(rawURL string, code string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}
	parsed.Path = "/stock/go.php/vReport_List/kind/search/index.phtml"
	query := parsed.Query()
	query.Set("t1", "2")
	query.Set("symbol", strings.TrimSpace(code))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func parseSinaFinanceReportDocument(doc *goquery.Document, pageURL string, now time.Time) []model.StockResearchSurvey {
	items := make([]model.StockResearchSurvey, 0)
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := make([]string, 0)
		row.Find("td").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, cleanStockResearchText(cell.Text()))
		})
		if len(cells) < 6 || strings.EqualFold(cells[0], "序号") {
			return
		}
		link := row.Find("a").First()
		title := cleanStockResearchText(link.Text())
		if attrTitle, ok := link.Attr("title"); ok {
			title = nonEmptyText(cleanStockResearchText(attrTitle), title)
		}
		if title == "" || !looksLikeResearchTitle(title) {
			return
		}
		href, _ := link.Attr("href")
		code, name := stockResearchNameCodeFromTitle(title)
		item := model.StockResearchSurvey{
			Code:         code,
			Name:         name,
			Kind:         "report",
			Title:        title,
			ResearchDate: normalizeStockResearchDate(stockResearchCell(cells, 3)),
			PublishTime:  normalizeStockResearchDate(stockResearchCell(cells, 3)),
			Institution:  stockResearchCell(cells, 4),
			Analyst:      stockResearchCell(cells, 5),
			SourceURL:    resolveStockResearchURL(pageURL, href),
			SourceType:   "sina_finance_report",
			RawPayload:   stockResearchJSONPayload(map[string]any{"cells": cells}),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if item.SourceURL == "" {
			item.SourceURL = pageURL
		}
		item.SourceKey = stockResearchSourceKey(item)
		items = append(items, item)
	})
	return items
}

func (w *Worker) enrichSinaFinanceReportDetails(ctx context.Context, items []model.StockResearchSurvey) []model.StockResearchSurvey {
	for i := range items {
		sourceURL := strings.TrimSpace(items[i].SourceURL)
		if sourceURL == "" {
			continue
		}
		doc, _, err := w.fetchStockResearchDocument(ctx, sourceURL, "sina_finance_report_detail")
		if err != nil {
			log.Warn().Err(err).Str("url", sourceURL).Msg("sina stock research detail skipped")
			continue
		}
		enrichSinaFinanceReportDetail(doc, &items[i])
	}
	return items
}

func enrichSinaFinanceReportDetail(doc *goquery.Document, item *model.StockResearchSurvey) {
	if title := cleanStockResearchText(doc.Find(".content h1").First().Text()); title != "" {
		item.Title = title
		if code, name := stockResearchNameCodeFromTitle(title); code != "" {
			item.Code = code
			item.Name = nonEmptyText(item.Name, name)
		}
	}
	doc.Find(".content .creab span").Each(func(_ int, span *goquery.Selection) {
		text := cleanStockResearchText(span.Text())
		switch {
		case strings.HasPrefix(text, "类别："):
			item.Kind = "report"
		case strings.HasPrefix(text, "机构："):
			item.Institution = strings.TrimSpace(strings.TrimPrefix(text, "机构："))
		case strings.HasPrefix(text, "研究员："):
			item.Analyst = strings.TrimSpace(strings.TrimPrefix(text, "研究员："))
		case strings.HasPrefix(text, "日期："):
			if date := normalizeStockResearchDate(strings.TrimPrefix(text, "日期：")); date != "" {
				item.ResearchDate = date
				item.PublishTime = date
			}
		}
	})
	if body := cleanStockResearchText(doc.Find(".content .blk_container").First().Text()); body != "" {
		item.Summary = truncateStockResearchText(body, 6000)
	}
	code, name := stockResearchNameCodeFromTitle(item.Title)
	if item.Code == "" {
		item.Code = code
	}
	if item.Name == "" {
		item.Name = name
	}
	item.RawPayload = stockResearchJSONPayload(map[string]any{
		"title":        item.Title,
		"institution":  item.Institution,
		"analyst":      item.Analyst,
		"researchDate": item.ResearchDate,
		"summary":      item.Summary,
	})
	item.SourceKey = stockResearchSourceKey(*item)
}

type eastMoneyInitData struct {
	Hits int                  `json:"hits"`
	Size int                  `json:"size"`
	Data []eastMoneyReportRow `json:"data"`
}

type eastMoneyReportRow struct {
	Title                 string   `json:"title"`
	StockName             string   `json:"stockName"`
	StockCode             string   `json:"stockCode"`
	OrgName               string   `json:"orgName"`
	OrgSName              string   `json:"orgSName"`
	PublishDate           string   `json:"publishDate"`
	InfoCode              string   `json:"infoCode"`
	EmRatingName          string   `json:"emRatingName"`
	SRatingName           string   `json:"sRatingName"`
	Researcher            string   `json:"researcher"`
	IndvAimPriceT         string   `json:"indvAimPriceT"`
	IndvAimPriceL         string   `json:"indvAimPriceL"`
	PredictThisYearEPS    string   `json:"predictThisYearEps"`
	PredictThisYearPE     string   `json:"predictThisYearPe"`
	PredictNextYearEPS    string   `json:"predictNextYearEps"`
	PredictNextYearPE     string   `json:"predictNextYearPe"`
	PredictNextTwoYearEPS string   `json:"predictNextTwoYearEps"`
	PredictNextTwoYearPE  string   `json:"predictNextTwoYearPe"`
	IndustryName          string   `json:"industryName"`
	IndvInduName          string   `json:"indvInduName"`
	Author                []string `json:"author"`
}

type eastMoneyReportDetail struct {
	AttachURL     string `json:"attach_url"`
	InfoCode      string `json:"info_code"`
	NoticeTitle   string `json:"notice_title"`
	NoticeContent string `json:"notice_content"`
	NoticeDate    string `json:"notice_date"`
	Rating        string `json:"rating"`
	Researcher    string `json:"researcher"`
	ShortName     string `json:"short_name"`
	SourceName    string `json:"source_sample_name"`
}

func parseEastMoneyReportDocument(body string, pageURL string, now time.Time) ([]model.StockResearchSurvey, error) {
	match := eastMoneyInitDataPattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return nil, errors.New("eastmoney initdata not found")
	}
	var initData eastMoneyInitData
	if err := json.Unmarshal([]byte(match[1]), &initData); err != nil {
		return nil, err
	}
	return buildEastMoneyReportItems(initData.Data, pageURL, now), nil
}

func parseEastMoneyReportAPI(body string, pageURL string, now time.Time) ([]model.StockResearchSurvey, int, error) {
	var data eastMoneyInitData
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil, 0, err
	}
	return buildEastMoneyReportItems(data.Data, pageURL, now), data.Hits, nil
}

func buildEastMoneyReportItems(rows []eastMoneyReportRow, pageURL string, now time.Time) []model.StockResearchSurvey {
	items := make([]model.StockResearchSurvey, 0, len(rows))
	for _, row := range rows {
		title := cleanStockResearchText(row.Title)
		infoCode := strings.TrimSpace(row.InfoCode)
		if title == "" || infoCode == "" {
			continue
		}
		sourceURL := resolveStockResearchURL(pageURL, "/report/info/"+infoCode+".html")
		rowPayload, _ := json.Marshal(row)
		item := model.StockResearchSurvey{
			Code:         strings.TrimSpace(row.StockCode),
			Name:         cleanStockResearchText(row.StockName),
			Kind:         "report",
			Title:        title,
			Institution:  nonEmptyText(cleanStockResearchText(row.OrgName), cleanStockResearchText(row.OrgSName)),
			Analyst:      cleanStockResearchText(nonEmptyText(row.Researcher, strings.Join(row.Author, "/"))),
			Rating:       cleanStockResearchText(nonEmptyText(row.EmRatingName, row.SRatingName)),
			TargetPrice:  eastMoneyTargetPrice(row),
			ResearchDate: normalizeStockResearchDate(row.PublishDate),
			PublishTime:  normalizeStockResearchDate(row.PublishDate),
			SourceURL:    sourceURL,
			SourceType:   "eastmoney_report",
			SourceKey:    infoCode,
			Summary:      eastMoneyReportSummary(row),
			RawPayload:   string(rowPayload),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		items = append(items, item)
	}
	return items
}

func (w *Worker) enrichEastMoneyReportDetails(ctx context.Context, items []model.StockResearchSurvey) []model.StockResearchSurvey {
	for i := range items {
		sourceURL := strings.TrimSpace(items[i].SourceURL)
		if sourceURL == "" {
			continue
		}
		_, body, err := w.fetchStockResearchDocument(ctx, sourceURL, "eastmoney_report_detail")
		if err != nil {
			log.Warn().Err(err).Str("url", sourceURL).Msg("eastmoney stock research detail skipped")
			continue
		}
		if err := enrichEastMoneyReportDetail(body, &items[i]); err != nil {
			log.Warn().Err(err).Str("url", sourceURL).Msg("eastmoney stock research detail parse skipped")
		}
	}
	return items
}

func enrichEastMoneyReportDetail(body string, item *model.StockResearchSurvey) error {
	match := eastMoneyZWInfoPattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return errors.New("eastmoney zwinfo not found")
	}
	var detail eastMoneyReportDetail
	if err := json.Unmarshal([]byte(match[1]), &detail); err != nil {
		return err
	}
	if title := cleanStockResearchText(detail.NoticeTitle); title != "" {
		item.Title = title
	}
	if researcher := cleanStockResearchText(detail.Researcher); researcher != "" {
		item.Analyst = researcher
	}
	if rating := cleanStockResearchText(detail.Rating); rating != "" {
		item.Rating = rating
	}
	if shortName := cleanStockResearchText(detail.ShortName); shortName != "" && item.Name == "" {
		item.Name = shortName
	}
	if date := normalizeStockResearchDate(detail.NoticeDate); date != "" && strings.TrimSpace(item.ResearchDate) == "" {
		item.ResearchDate = date
		item.PublishTime = date
	}
	if pdfURL := strings.TrimSpace(detail.AttachURL); pdfURL != "" {
		item.PDFURL = pdfURL
		item.PDFStatus = "pending"
	}
	if content := cleanStockResearchText(detail.NoticeContent); content != "" {
		item.Summary = truncateStockResearchText(content, 6000)
		if item.Code == "" || item.Name == "" {
			code, name := stockResearchNameCodeFromTitle(content)
			if item.Code == "" {
				item.Code = code
			}
			if item.Name == "" {
				item.Name = name
			}
		}
	}
	item.RawPayload = stockResearchJSONPayload(map[string]any{
		"infoCode":      nonEmptyText(detail.InfoCode, item.SourceKey),
		"noticeTitle":   detail.NoticeTitle,
		"noticeDate":    detail.NoticeDate,
		"researcher":    detail.Researcher,
		"rating":        detail.Rating,
		"sourceName":    detail.SourceName,
		"attachURL":     detail.AttachURL,
		"noticeContent": item.Summary,
	})
	return nil
}

func parseFinanceReportDocument(doc *goquery.Document, pageURL string, sourceType string, now time.Time) []model.StockResearchSurvey {
	items := make([]model.StockResearchSurvey, 0)
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := make([]string, 0)
		row.Find("td").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, cleanStockResearchText(cell.Text()))
		})
		if len(cells) < 3 {
			return
		}
		link := row.Find("a").First()
		title := cleanStockResearchText(link.Text())
		if title == "" {
			title = firstMeaningfulStockResearchCell(cells)
		}
		if title == "" || !looksLikeResearchTitle(title) {
			return
		}
		href, _ := link.Attr("href")
		item := model.StockResearchSurvey{
			Kind:         "report",
			Title:        title,
			ResearchDate: firstStockResearchDate(cells),
			PublishTime:  firstStockResearchDate(cells),
			Institution:  stockResearchCell(cells, len(cells)-2),
			Analyst:      stockResearchCell(cells, len(cells)-1),
			SourceURL:    resolveStockResearchURL(pageURL, href),
			SourceType:   sourceType,
			RawPayload:   rawStockResearchPayload(cells),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if item.SourceURL == "" {
			item.SourceURL = pageURL
		}
		item.SourceKey = stockResearchSourceKey(item)
		items = append(items, item)
	})
	if len(items) > 0 {
		return items
	}
	doc.Find("a").Each(func(_ int, link *goquery.Selection) {
		title := cleanStockResearchText(link.Text())
		if !looksLikeResearchTitle(title) {
			return
		}
		href, _ := link.Attr("href")
		item := model.StockResearchSurvey{
			Kind:       "report",
			Title:      title,
			SourceURL:  resolveStockResearchURL(pageURL, href),
			SourceType: sourceType,
			RawPayload: "{}",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		item.SourceKey = stockResearchSourceKey(item)
		items = append(items, item)
	})
	return items
}

func normalizeStockResearchSourceItems(items []model.StockResearchSurvey, fallbackSource string) []model.StockResearchSurvey {
	now := time.Now().UTC()
	for i := range items {
		items[i].Code = strings.TrimSpace(items[i].Code)
		items[i].Name = strings.TrimSpace(items[i].Name)
		items[i].Kind = stockResearchKindText(items[i].Kind)
		items[i].Title = cleanStockResearchText(items[i].Title)
		items[i].Institution = cleanStockResearchText(items[i].Institution)
		items[i].Analyst = cleanStockResearchText(items[i].Analyst)
		items[i].Rating = cleanStockResearchText(items[i].Rating)
		items[i].TargetPrice = cleanStockResearchText(items[i].TargetPrice)
		items[i].ResearchDate = cleanStockResearchText(items[i].ResearchDate)
		items[i].PublishTime = cleanStockResearchText(items[i].PublishTime)
		items[i].SourceURL = strings.TrimSpace(items[i].SourceURL)
		items[i].SourceType = strings.TrimSpace(items[i].SourceType)
		items[i].PDFURL = strings.TrimSpace(items[i].PDFURL)
		items[i].PDFFilePath = strings.TrimSpace(items[i].PDFFilePath)
		items[i].PDFStatus = strings.TrimSpace(items[i].PDFStatus)
		items[i].PDFError = cleanStockResearchText(items[i].PDFError)
		items[i].PDFFetchedAt = cleanStockResearchText(items[i].PDFFetchedAt)
		items[i].PDFParsedAt = cleanStockResearchText(items[i].PDFParsedAt)
		if items[i].SourceType == "" {
			items[i].SourceType = fallbackSource
		}
		items[i].Summary = cleanStockResearchText(items[i].Summary)
		if strings.TrimSpace(items[i].RawPayload) == "" {
			items[i].RawPayload = "{}"
		}
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = now
		}
		if items[i].UpdatedAt.IsZero() {
			items[i].UpdatedAt = now
		}
		if strings.TrimSpace(items[i].SourceKey) == "" {
			items[i].SourceKey = stockResearchSourceKey(items[i])
		}
	}
	return items
}

func filterStockResearchItems(items []model.StockResearchSurvey, opts stockResearchCrawlOptions) []model.StockResearchSurvey {
	code := strings.TrimSpace(opts.Code)
	company := strings.TrimSpace(opts.Company)
	start := normalizeStockResearchDate(opts.Start)
	end := normalizeStockResearchDate(opts.End)
	if code == "" && company == "" && start == "" && end == "" {
		return items
	}
	out := make([]model.StockResearchSurvey, 0, len(items))
	for _, item := range items {
		itemDate := normalizeStockResearchDate(nonEmptyText(item.ResearchDate, item.PublishTime))
		if !stockResearchDateInRange(itemDate, start, end) {
			continue
		}
		blob := strings.ToLower(strings.Join([]string{item.Code, item.Name, item.Title, item.Summary, item.RawPayload}, " "))
		if code == "" && company == "" {
			out = append(out, item)
			continue
		}
		if code != "" && strings.Contains(blob, strings.ToLower(code)) {
			out = append(out, item)
			continue
		}
		if company != "" && strings.Contains(blob, strings.ToLower(company)) {
			out = append(out, item)
		}
	}
	return out
}

func stockResearchDateInRange(value, start, end string) bool {
	if value == "" {
		return true
	}
	if start != "" && value < start {
		return false
	}
	if end != "" && value > end {
		return false
	}
	return true
}

func dedupeStockResearch(items []model.StockResearchSurvey) []model.StockResearchSurvey {
	seenSource := map[string]int{}
	seenEquivalent := map[string]int{}
	out := make([]model.StockResearchSurvey, 0, len(items))
	for _, item := range items {
		key := item.SourceType + "|" + item.SourceKey
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.SourceKey) == "" {
			continue
		}
		if idx, ok := seenSource[key]; ok {
			out[idx] = mergePreferredStockResearch(out[idx], item)
			continue
		}
		equivalentKey := stockResearchEquivalentKey(item)
		if equivalentKey != "" {
			if idx, ok := seenEquivalent[equivalentKey]; ok {
				out[idx] = mergePreferredStockResearch(out[idx], item)
				seenSource[out[idx].SourceType+"|"+out[idx].SourceKey] = idx
				continue
			}
			seenEquivalent[equivalentKey] = len(out)
		}
		seenSource[key] = len(out)
		out = append(out, item)
	}
	return out
}

func mergePreferredStockResearch(current, incoming model.StockResearchSurvey) model.StockResearchSurvey {
	preferred, fallback := current, incoming
	if stockResearchSourceRank(incoming) > stockResearchSourceRank(current) {
		preferred, fallback = incoming, current
	}
	if strings.TrimSpace(preferred.Code) == "" {
		preferred.Code = fallback.Code
	}
	if strings.TrimSpace(preferred.Name) == "" {
		preferred.Name = fallback.Name
	}
	if strings.TrimSpace(preferred.Kind) == "" {
		preferred.Kind = fallback.Kind
	}
	if strings.TrimSpace(preferred.Title) == "" {
		preferred.Title = fallback.Title
	}
	if strings.TrimSpace(preferred.Institution) == "" {
		preferred.Institution = fallback.Institution
	}
	if strings.TrimSpace(preferred.Analyst) == "" {
		preferred.Analyst = fallback.Analyst
	}
	if strings.TrimSpace(preferred.Rating) == "" {
		preferred.Rating = fallback.Rating
	}
	if strings.TrimSpace(preferred.TargetPrice) == "" {
		preferred.TargetPrice = fallback.TargetPrice
	}
	if strings.TrimSpace(preferred.ResearchDate) == "" {
		preferred.ResearchDate = fallback.ResearchDate
	}
	if strings.TrimSpace(preferred.PublishTime) == "" {
		preferred.PublishTime = fallback.PublishTime
	}
	if strings.TrimSpace(preferred.SourceURL) == "" {
		preferred.SourceURL = fallback.SourceURL
	}
	if strings.TrimSpace(preferred.Summary) == "" {
		preferred.Summary = fallback.Summary
	}
	if strings.TrimSpace(preferred.RawPayload) == "" || strings.TrimSpace(preferred.RawPayload) == "{}" {
		preferred.RawPayload = fallback.RawPayload
	}
	if strings.TrimSpace(preferred.PDFURL) == "" {
		preferred.PDFURL = fallback.PDFURL
	}
	if strings.TrimSpace(preferred.PDFFilePath) == "" {
		preferred.PDFFilePath = fallback.PDFFilePath
	}
	if strings.TrimSpace(preferred.PDFStatus) == "" {
		preferred.PDFStatus = fallback.PDFStatus
	}
	if strings.TrimSpace(preferred.PDFText) == "" {
		preferred.PDFText = fallback.PDFText
	}
	if strings.TrimSpace(preferred.PDFError) == "" {
		preferred.PDFError = fallback.PDFError
	}
	if strings.TrimSpace(preferred.PDFFetchedAt) == "" {
		preferred.PDFFetchedAt = fallback.PDFFetchedAt
	}
	if strings.TrimSpace(preferred.PDFParsedAt) == "" {
		preferred.PDFParsedAt = fallback.PDFParsedAt
	}
	if preferred.NLPScore == 0 {
		preferred.NLPScore = fallback.NLPScore
	}
	if strings.TrimSpace(preferred.NLPRating) == "" {
		preferred.NLPRating = fallback.NLPRating
	}
	if strings.TrimSpace(preferred.NLPReason) == "" {
		preferred.NLPReason = fallback.NLPReason
	}
	if strings.TrimSpace(preferred.NLPScoredAt) == "" {
		preferred.NLPScoredAt = fallback.NLPScoredAt
	}
	if preferred.CreatedAt.IsZero() || (!fallback.CreatedAt.IsZero() && fallback.CreatedAt.Before(preferred.CreatedAt)) {
		preferred.CreatedAt = fallback.CreatedAt
	}
	if fallback.UpdatedAt.After(preferred.UpdatedAt) {
		preferred.UpdatedAt = fallback.UpdatedAt
	}
	return preferred
}

func stockResearchSourceRank(item model.StockResearchSurvey) int {
	switch strings.TrimSpace(item.SourceType) {
	case "eastmoney_report":
		if strings.HasPrefix(strings.TrimSpace(item.SourceKey), "AP") || strings.TrimSpace(item.PDFURL) != "" {
			return 40
		}
		return 35
	case "akshare_stock_research":
		return 30
	case "sina_finance_report":
		return 20
	case "sohu_finance_report":
		return 10
	default:
		return 0
	}
}

func stockResearchEquivalentKey(item model.StockResearchSurvey) string {
	if stockResearchKindText(item.Kind) != "report" {
		return ""
	}
	code := strings.TrimSpace(item.Code)
	date := normalizeStockResearchDate(nonEmptyText(item.ResearchDate, item.PublishTime))
	institution := stockResearchComparableText(item.Institution)
	title := stockResearchComparableTitle(item)
	if code == "" || date == "" || institution == "" || title == "" {
		return ""
	}
	return strings.Join([]string{code, date, institution, title}, "|")
}

func stockResearchComparableTitle(item model.StockResearchSurvey) string {
	title := cleanStockResearchText(item.Title)
	if title == "" {
		return ""
	}
	colonIdx, colonSize := stockResearchComparableTitleColonIndex(title)
	if colonIdx >= 0 {
		prefix := title[:colonIdx]
		if strings.Contains(prefix, strings.TrimSpace(item.Code)) || strings.Contains(prefix, strings.TrimSpace(item.Name)) || stockResearchCodePattern.MatchString(prefix) {
			title = title[colonIdx+colonSize:]
		}
	}
	return stockResearchComparableText(title)
}

func stockResearchComparableTitleColonIndex(title string) (int, int) {
	half := strings.Index(title, ":")
	full := strings.Index(title, "：")
	switch {
	case half < 0:
		if full < 0 {
			return -1, 0
		}
		return full, len("：")
	case full < 0 || half < full:
		return half, len(":")
	default:
		return full, len("：")
	}
}

func stockResearchComparableText(value string) string {
	value = strings.ToLower(cleanStockResearchText(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func stockResearchToday() time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	return time.Now().In(location)
}

func stockResearchKindText(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "survey", "调研":
		return "survey"
	default:
		return "report"
	}
}

func stockResearchSourceKey(item model.StockResearchSurvey) string {
	raw := strings.Join([]string{item.SourceType, item.Code, item.Name, item.Title, item.ResearchDate, item.PublishTime, item.SourceURL}, "|")
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func stockResearchJSONPayload(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func nonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeStockResearchDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "T"); idx > 0 {
		value = value[:idx]
	}
	if idx := strings.Index(value, " "); idx > 0 && strings.Contains(value[:idx], "-") {
		value = value[:idx]
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006.01.02", "20060102", "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}

func stockResearchNameCodeFromTitle(title string) (string, string) {
	matches := stockResearchCodePattern.FindAllStringSubmatch(title, -1)
	if len(matches) == 0 {
		return "", ""
	}
	match := matches[len(matches)-1]
	name := strings.TrimSpace(match[1])
	for _, sep := range []string{"*", "：", ":", " ", "　"} {
		if idx := strings.LastIndex(name, sep); idx >= 0 {
			name = strings.TrimSpace(name[idx+len(sep):])
		}
	}
	return match[2], strings.Trim(name, " -_")
}

func eastMoneyTargetPrice(row eastMoneyReportRow) string {
	high := strings.TrimSpace(row.IndvAimPriceT)
	low := strings.TrimSpace(row.IndvAimPriceL)
	switch {
	case high != "" && low != "" && high != low:
		return low + "-" + high
	case high != "":
		return high
	case low != "":
		return low
	default:
		return ""
	}
}

func eastMoneyReportSummary(row eastMoneyReportRow) string {
	parts := make([]string, 0)
	if industry := nonEmptyText(row.IndustryName, row.IndvInduName); industry != "" {
		parts = append(parts, "行业："+industry)
	}
	for _, item := range []struct {
		Year string
		EPS  string
		PE   string
	}{
		{"当年", row.PredictThisYearEPS, row.PredictThisYearPE},
		{"次年", row.PredictNextYearEPS, row.PredictNextYearPE},
		{"后年", row.PredictNextTwoYearEPS, row.PredictNextTwoYearPE},
	} {
		eps := strings.TrimSpace(item.EPS)
		pe := strings.TrimSpace(item.PE)
		if eps == "" && pe == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s EPS %s / PE %s", item.Year, nonEmptyText(eps, "--"), nonEmptyText(pe, "--")))
	}
	return strings.Join(parts, "；")
}

func truncateStockResearchText(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func cleanStockResearchText(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func firstMeaningfulStockResearchCell(cells []string) string {
	for _, cell := range cells {
		if cell != "" && !isStockResearchDate(cell) {
			return cell
		}
	}
	return ""
}

func firstStockResearchDate(cells []string) string {
	for _, cell := range cells {
		if isStockResearchDate(cell) {
			return normalizeStockResearchDate(cell)
		}
	}
	return ""
}

func isStockResearchDate(value string) bool {
	value = normalizeStockResearchDate(value)
	if len(value) < 8 {
		return false
	}
	for _, layout := range []string{"2006-01-02"} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func looksLikeResearchTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if stockResearchCodePattern.MatchString(title) {
		return true
	}
	for _, keyword := range []string{"研报", "研究", "评级", "调研", "点评", "深度", "策略"} {
		if strings.Contains(title, keyword) {
			return true
		}
	}
	return false
}

func stockResearchCell(cells []string, index int) string {
	if index < 0 || index >= len(cells) {
		return ""
	}
	return cells[index]
}

func rawStockResearchPayload(cells []string) string {
	payload, err := json.Marshal(map[string]any{"cells": cells})
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func resolveStockResearchURL(pageURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	if parsed, err := url.Parse(href); err == nil && parsed.Scheme != "" {
		return parsed.String()
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return href
	}
	resolved, err := base.Parse(href)
	if err != nil {
		return href
	}
	return resolved.String()
}
