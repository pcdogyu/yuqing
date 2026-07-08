package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	investorRelationsSourceType = "cninfo_investor_relation"
	cninfoSurveyPageURL         = "https://irm.cninfo.com.cn/views/comprehensiveInfo/survey"
	cninfoStaticBaseURL         = "http://static.cninfo.com.cn/"
)

func (w *Worker) runInvestorRelationsCrawl(ctx context.Context) error {
	end := stockResearchToday()
	start := end.AddDate(0, 0, -7)
	return w.runInvestorRelationsBackfill(ctx, stockResearchCrawlOptions{
		Start: start.Format("2006-01-02"),
		End:   end.Format("2006-01-02"),
	})
}

func (w *Worker) runInvestorRelationsBackfill(ctx context.Context, opts stockResearchCrawlOptions) error {
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
	items, err := w.fetchCNInfoInvestorRelations(ctx, opts)
	if err != nil {
		return err
	}
	items = w.enrichStockResearchSourceTexts(ctx, items, false)
	payload := map[string]any{"items": items}
	resp, err := w.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post(w.cfg.ContentURL + "/api/v1/internal/stock-research/batch")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("content investor relations upsert failed: %s", resp.Status())
	}
	log.Info().
		Str("start", opts.Start).
		Str("end", opts.End).
		Str("code", opts.Code).
		Str("company", opts.Company).
		Int("items", len(items)).
		Msg("investor relations crawl completed")
	if len(items) > 0 {
		if result, pdfErr := w.runStockResearchPDFParse(ctx, stockResearchPDFParseOptions{
			Code:    opts.Code,
			Company: opts.Company,
			Source:  investorRelationsSourceType,
			Start:   opts.Start,
			End:     opts.End,
		}); pdfErr != nil {
			log.Warn().Err(pdfErr).Interface("result", result).Msg("investor relations pdf parse finished with errors")
		}
	}
	return nil
}

func (w *Worker) fetchCNInfoInvestorRelations(ctx context.Context, opts stockResearchCrawlOptions) ([]model.StockResearchSurvey, error) {
	endpoint := strings.TrimSpace(w.cfg.InvestorRelationsURL)
	if endpoint == "" {
		return nil, nil
	}
	items := make([]model.StockResearchSurvey, 0)
	pageSize := 50
	now := time.Now().UTC()
	for page := 1; page <= 200; page++ {
		form := cninfoInvestorRelationsForm(opts, page, pageSize)
		var envelope cninfoInvestorRelationsResponse
		resp, err := w.client.R().
			SetContext(ctx).
			SetHeader("Accept", "application/json, text/plain, */*").
			SetHeader("Origin", "https://irm.cninfo.com.cn").
			SetHeader("Referer", cninfoSurveyPageURL).
			SetHeader("User-Agent", nonEmptyText(w.cfg.UserAgent, "Mozilla/5.0")).
			SetFormData(form).
			SetResult(&envelope).
			Post(endpoint)
		if err != nil {
			return nil, err
		}
		if !resp.IsSuccess() {
			return nil, fmt.Errorf("cninfo investor relations failed: %s", resp.Status())
		}
		pageItems := buildCNInfoInvestorRelationsItems(envelope.Results, now)
		pageItems = filterStockResearchItems(pageItems, opts)
		items = append(items, pageItems...)
		rawCount := len(envelope.Results)
		effectivePageSize := pageSize
		if envelope.PageSize > 0 {
			effectivePageSize = envelope.PageSize
		}
		totalPage := envelope.TotalPage
		if totalPage <= 0 && envelope.TotalRecord > 0 {
			totalPage = (envelope.TotalRecord + effectivePageSize - 1) / effectivePageSize
		}
		if rawCount == 0 || (totalPage > 0 && page >= totalPage) || (envelope.TotalRecord > 0 && page*effectivePageSize >= envelope.TotalRecord) {
			break
		}
	}
	return dedupeStockResearch(normalizeStockResearchSourceItems(items, investorRelationsSourceType)), nil
}

func cninfoInvestorRelationsForm(opts stockResearchCrawlOptions, page, pageSize int) map[string]string {
	form := map[string]string{
		"pageNo":      strconv.Itoa(page),
		"pageSize":    strconv.Itoa(pageSize),
		"searchTypes": "4",
		"keyWord":     strings.TrimSpace(opts.Company),
		"stockCode":   strings.TrimSpace(opts.Code),
		"highLight":   "true",
	}
	if start := normalizeStockResearchDate(opts.Start); start != "" {
		form["beginDate"] = start + " 00:00:00"
	}
	if end := normalizeStockResearchDate(opts.End); end != "" {
		form["endDate"] = end + " 23:59:59"
	}
	return form
}

type cninfoInvestorRelationsResponse struct {
	PageNo      int                           `json:"pageNo"`
	PageSize    int                           `json:"pageSize"`
	TotalRecord int                           `json:"totalRecord"`
	TotalPage   int                           `json:"totalPage"`
	Results     []cninfoInvestorRelationsItem `json:"results"`
}

type cninfoInvestorRelationsItem struct {
	IndexID          string `json:"indexId"`
	MainContent      string `json:"mainContent"`
	AttachmentURL    string `json:"attachmentUrl"`
	StockCode        string `json:"stockCode"`
	CompanyShortName string `json:"companyShortName"`
	PubDate          string `json:"pubDate"`
	FileType         string `json:"filetype"`
	PackageDate      string `json:"packageDate"`
}

func buildCNInfoInvestorRelationsItems(rows []cninfoInvestorRelationsItem, now time.Time) []model.StockResearchSurvey {
	items := make([]model.StockResearchSurvey, 0, len(rows))
	for _, row := range rows {
		title := cleanStockResearchText(row.MainContent)
		indexID := strings.TrimSpace(row.IndexID)
		pdfURL := cninfoPDFURL(row.AttachmentURL)
		if title == "" || indexID == "" || pdfURL == "" {
			continue
		}
		publishTime, researchDate := cninfoPublishTimes(row)
		rowPayload, _ := json.Marshal(row)
		items = append(items, model.StockResearchSurvey{
			Code:         strings.TrimSpace(row.StockCode),
			Name:         cleanStockResearchText(row.CompanyShortName),
			Kind:         "survey",
			Title:        title,
			ResearchDate: researchDate,
			PublishTime:  publishTime,
			SourceURL:    pdfURL,
			SourceType:   investorRelationsSourceType,
			SourceKey:    indexID,
			Summary:      cleanStockResearchText(nonEmptyText(row.PackageDate, row.FileType)),
			RawPayload:   string(rowPayload),
			PDFURL:       pdfURL,
			PDFStatus:    "pending",
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	return items
}

func cninfoPDFURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return resolveStockResearchURL(cninfoStaticBaseURL, raw)
}

func cninfoPublishTimes(row cninfoInvestorRelationsItem) (string, string) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("UTC+8", 8*60*60)
	}
	if millis, err := strconv.ParseInt(strings.TrimSpace(row.PubDate), 10, 64); err == nil && millis > 0 {
		if millis < 1_000_000_000_000 {
			millis *= 1000
		}
		parsed := time.UnixMilli(millis).In(location)
		return parsed.Format("2006-01-02 15:04:05"), parsed.Format("2006-01-02")
	}
	date := normalizeStockResearchDate(row.PackageDate)
	if date != "" {
		return date, date
	}
	return "", ""
}
