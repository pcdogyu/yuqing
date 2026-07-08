package model

import "time"

type StockResearchSurvey struct {
	ID                int64     `json:"id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Kind              string    `json:"kind"`
	Title             string    `json:"title"`
	Institution       string    `json:"institution"`
	Analyst           string    `json:"analyst"`
	Rating            string    `json:"rating"`
	TargetPrice       string    `json:"target_price"`
	ResearchDate      string    `json:"research_date"`
	PublishTime       string    `json:"publish_time"`
	SourceURL         string    `json:"source_url"`
	SourceType        string    `json:"source_type"`
	SourceKey         string    `json:"source_key"`
	Summary           string    `json:"summary"`
	RawPayload        string    `json:"raw_payload"`
	SourceText        string    `json:"source_text"`
	SourceFetchStatus string    `json:"source_fetch_status"`
	SourceFetchError  string    `json:"source_fetch_error"`
	SourceFetchedAt   string    `json:"source_fetched_at"`
	PDFURL            string    `json:"pdf_url"`
	PDFFilePath       string    `json:"pdf_file_path"`
	PDFStatus         string    `json:"pdf_status"`
	PDFText           string    `json:"pdf_text"`
	PDFError          string    `json:"pdf_error"`
	PDFFetchedAt      string    `json:"pdf_fetched_at"`
	PDFParsedAt       string    `json:"pdf_parsed_at"`
	NLPScore          float64   `json:"nlp_score"`
	NLPRating         string    `json:"nlp_rating"`
	NLPReason         string    `json:"nlp_reason"`
	NLPScoredAt       string    `json:"nlp_scored_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type StockResearchFilter struct {
	Code        string `json:"code"`
	Company     string `json:"company"`
	Institution string `json:"institution"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
}

type StockResearchListResult struct {
	Items       []StockResearchSurvey `json:"items"`
	Page        int                   `json:"page"`
	PageSize    int                   `json:"page_size"`
	Total       int                   `json:"total"`
	Code        string                `json:"code"`
	Company     string                `json:"company"`
	Institution string                `json:"institution"`
	Kind        string                `json:"kind"`
	Source      string                `json:"source"`
	Start       string                `json:"start"`
	End         string                `json:"end"`
	Sources     []string              `json:"sources"`
}

type StockResearchUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Total    int `json:"total"`
}

type StockResearchPDFUpdate struct {
	PDFURL            string  `json:"pdf_url"`
	PDFFilePath       string  `json:"pdf_file_path"`
	PDFStatus         string  `json:"pdf_status"`
	PDFText           string  `json:"pdf_text"`
	PDFError          string  `json:"pdf_error"`
	PDFFetchedAt      string  `json:"pdf_fetched_at"`
	PDFParsedAt       string  `json:"pdf_parsed_at"`
	NLPScore          float64 `json:"nlp_score"`
	NLPRating         string  `json:"nlp_rating"`
	NLPReason         string  `json:"nlp_reason"`
	NLPScoredAt       string  `json:"nlp_scored_at"`
	SourceText        string  `json:"source_text"`
	SourceFetchStatus string  `json:"source_fetch_status"`
	SourceFetchError  string  `json:"source_fetch_error"`
	SourceFetchedAt   string  `json:"source_fetched_at"`
}

type StockResearchSourceUpdate struct {
	SourceText        string `json:"source_text"`
	SourceFetchStatus string `json:"source_fetch_status"`
	SourceFetchError  string `json:"source_fetch_error"`
	SourceFetchedAt   string `json:"source_fetched_at"`
	Force             bool   `json:"force"`
}
