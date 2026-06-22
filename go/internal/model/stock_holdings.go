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

type StockInstitutionHoldingSignal struct {
	StockCode           string   `json:"stock_code"`
	StockName           string   `json:"stock_name"`
	CurrentPeriod       string   `json:"current_period"`
	PreviousPeriod      string   `json:"previous_period"`
	CurrentHolderCount  int      `json:"current_holder_count"`
	PreviousHolderCount int      `json:"previous_holder_count"`
	HolderCountChange   int      `json:"holder_count_change"`
	CurrentFundCount    int      `json:"current_fund_count"`
	PreviousFundCount   int      `json:"previous_fund_count"`
	FundCountChange     int      `json:"fund_count_change"`
	CurrentShares       float64  `json:"current_shares"`
	PreviousShares      float64  `json:"previous_shares"`
	SharesChange        float64  `json:"shares_change"`
	CurrentFloatRatio   float64  `json:"current_float_ratio"`
	PreviousFloatRatio  float64  `json:"previous_float_ratio"`
	FloatRatioChange    float64  `json:"float_ratio_change"`
	CurrentMarketValue  float64  `json:"current_market_value"`
	PreviousMarketValue float64  `json:"previous_market_value"`
	MarketValueChange   float64  `json:"market_value_change"`
	NewMajorHolders     []string `json:"new_major_holders"`
	Level               string   `json:"level"`
	Reason              string   `json:"reason"`
}

type StockInstitutionHoldingSignalFilter struct {
	Code     string `json:"code"`
	Company  string `json:"company"`
	Period   string `json:"period"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type StockInstitutionHoldingSignalListResult struct {
	Items          []StockInstitutionHoldingSignal `json:"items"`
	Page           int                             `json:"page"`
	PageSize       int                             `json:"page_size"`
	Total          int                             `json:"total"`
	Code           string                          `json:"code"`
	Company        string                          `json:"company"`
	Period         string                          `json:"period"`
	CurrentPeriod  string                          `json:"current_period"`
	PreviousPeriod string                          `json:"previous_period"`
	Thresholds     StockInstitutionSignalThreshold `json:"thresholds"`
}

type StockInstitutionSignalThreshold struct {
	HolderCountChange int     `json:"holder_count_change"`
	FundCountChange   int     `json:"fund_count_change"`
	FloatRatioChange  float64 `json:"float_ratio_change"`
}

type StockInstitutionHoldingUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Total    int `json:"total"`
}
