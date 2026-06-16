package model

import "time"

type AStockAuctionAmount struct {
	TradeDate     string    `json:"trade_date"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	AuctionPrice  float64   `json:"auction_price"`
	AuctionVolume float64   `json:"auction_volume"`
	AuctionAmount float64   `json:"auction_amount"`
	Source        string    `json:"source"`
	Status        string    `json:"status"`
	FetchedAt     time.Time `json:"fetched_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AStockAuctionFilter struct {
	Date     string `json:"date"`
	Keyword  string `json:"keyword"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type AStockAuctionListResult struct {
	Items        []AStockAuctionAmount `json:"items"`
	Page         int                   `json:"page"`
	PageSize     int                   `json:"page_size"`
	Total        int                   `json:"total"`
	Date         string                `json:"date"`
	Keyword      string                `json:"keyword"`
	LatestDate   string                `json:"latest_date"`
	Dates        []string              `json:"dates"`
	SummaryCount int                   `json:"summary_count"`
	TotalAmount  float64               `json:"total_amount"`
	MaxItem      *AStockAuctionAmount  `json:"max_item,omitempty"`
	FetchedAt    *time.Time            `json:"fetched_at,omitempty"`
}

type AStockAuctionUpsertResult struct {
	Date     string `json:"date"`
	Inserted int    `json:"inserted"`
	Updated  int    `json:"updated"`
	Total    int    `json:"total"`
}
