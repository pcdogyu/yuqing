package model

import "time"

type StockInstitutionHolding struct {
	ID           int64     `json:"id"`
	StockCode    string    `json:"stock_code"`
	StockName    string    `json:"stock_name"`
	ReportPeriod string    `json:"report_period"`
	AnnounceDate string    `json:"announce_date"`
	HolderName   string    `json:"holder_name"`
	HolderType   string    `json:"holder_type"`
	HolderCode   string    `json:"holder_code"`
	HolderRank   string    `json:"holder_rank"`
	Shares       float64   `json:"shares"`
	SharesChange float64   `json:"shares_change"`
	ChangeRatio  float64   `json:"change_ratio"`
	FloatRatio   float64   `json:"float_ratio"`
	MarketValue  float64   `json:"market_value"`
	SourceType   string    `json:"source_type"`
	SourceURL    string    `json:"source_url"`
	RawPayload   string    `json:"raw_payload"`
	FetchedAt    time.Time `json:"fetched_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type StockInstitutionHoldingFilter struct {
	Code       string `json:"code"`
	Company    string `json:"company"`
	Period     string `json:"period"`
	Holder     string `json:"holder"`
	HolderType string `json:"holder_type"`
	Source     string `json:"source"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

type StockInstitutionHoldingListResult struct {
	Items       []StockInstitutionHolding `json:"items"`
	Page        int                       `json:"page"`
	PageSize    int                       `json:"page_size"`
	Total       int                       `json:"total"`
	Code        string                    `json:"code"`
	Company     string                    `json:"company"`
	Period      string                    `json:"period"`
	Holder      string                    `json:"holder"`
	HolderType  string                    `json:"holder_type"`
	Source      string                    `json:"source"`
	Periods     []string                  `json:"periods"`
	Sources     []string                  `json:"sources"`
	HolderTypes []string                  `json:"holder_types"`
}

type StockInstitutionHoldingSummary struct {
	StockCode        string  `json:"stock_code"`
	StockName        string  `json:"stock_name"`
	ReportPeriod     string  `json:"report_period"`
	HolderCount      int     `json:"holder_count"`
	FundCount        int     `json:"fund_count"`
	HolderTypeCount  int     `json:"holder_type_count"`
	TotalShares      float64 `json:"total_shares"`
	TotalFloatRatio  float64 `json:"total_float_ratio"`
	TotalMarketValue float64 `json:"total_market_value"`
	MaxHolderName    string  `json:"max_holder_name"`
	MaxHolderType    string  `json:"max_holder_type"`
	MaxHolderShares  float64 `json:"max_holder_shares"`
}

type StockInstitutionHoldingUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Total    int `json:"total"`
}
