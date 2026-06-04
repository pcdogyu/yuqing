package model

import "time"

type CryptoPairResolution struct {
	Input         string   `json:"input"`
	Pair          string   `json:"pair"`
	DisplayPair   string   `json:"display_pair"`
	BaseAsset     string   `json:"base_asset"`
	QuoteAsset    string   `json:"quote_asset"`
	BaseName      string   `json:"base_name"`
	BaseNameCN    string   `json:"base_name_cn"`
	QuoteName     string   `json:"quote_name"`
	BinanceSymbol string   `json:"binance_symbol"`
	SearchTerms   []string `json:"search_terms,omitempty"`
}

type CryptoEvidenceArticle struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	SourceType     string    `json:"source_type"`
	SourceURL      string    `json:"source_url"`
	DetailURL      string    `json:"detail_url"`
	PublishTime    string    `json:"publish_time"`
	CapturedAt     time.Time `json:"captured_at"`
	Direction      string    `json:"direction"`
	ReasonCategory string    `json:"reason_category"`
	ReasonLabel    string    `json:"reason_label"`
	RelevanceScore float64   `json:"relevance_score"`
}

type CryptoNewsResult struct {
	Resolution CryptoPairResolution    `json:"resolution"`
	Items      []CryptoEvidenceArticle `json:"items"`
	Total      int                     `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
}

type CryptoPriceCandle struct {
	Symbol    string    `json:"symbol"`
	Interval  string    `json:"interval"`
	OpenTime  time.Time `json:"open_time"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CryptoPriceSnapshot struct {
	Symbol     string    `json:"symbol"`
	Price      float64   `json:"price"`
	Change1H   float64   `json:"change_1h"`
	Change4H   float64   `json:"change_4h"`
	Change24H  float64   `json:"change_24h"`
	Volatility float64   `json:"volatility"`
	VolumeBias float64   `json:"volume_bias"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CryptoSignal struct {
	Horizon     string  `json:"horizon"`
	Direction   string  `json:"direction"`
	Bullish     float64 `json:"bullish"`
	Neutral     float64 `json:"neutral"`
	Bearish     float64 `json:"bearish"`
	Confidence  float64 `json:"confidence"`
	NewsScore   float64 `json:"news_score"`
	SocialScore float64 `json:"social_score"`
	PriceScore  float64 `json:"price_score"`
	Explanation string  `json:"explanation"`
}

type CryptoSignalSet struct {
	H4  CryptoSignal `json:"4h"`
	H24 CryptoSignal `json:"24h"`
}

type CryptoReason struct {
	Category      string  `json:"category"`
	Label         string  `json:"label"`
	Direction     string  `json:"direction"`
	Score         float64 `json:"score"`
	EvidenceCount int     `json:"evidence_count"`
	Summary       string  `json:"summary"`
}

type CryptoSocialPost struct {
	ID             int64     `json:"id"`
	Platform       string    `json:"platform"`
	Author         string    `json:"author"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	SourceType     string    `json:"source_type"`
	SourceURL      string    `json:"source_url"`
	DetailURL      string    `json:"detail_url"`
	PublishTime    string    `json:"publish_time"`
	CapturedAt     time.Time `json:"captured_at"`
	Direction      string    `json:"direction"`
	ReasonCategory string    `json:"reason_category"`
	ReasonLabel    string    `json:"reason_label"`
	RelevanceScore float64   `json:"relevance_score"`
	HeatScore      float64   `json:"heat_score"`
}

type CryptoSocialResult struct {
	Resolution CryptoPairResolution `json:"resolution"`
	Items      []CryptoSocialPost   `json:"items"`
	Total      int                  `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
}

type CryptoSocialSentiment struct {
	Direction     string  `json:"direction"`
	Bullish       float64 `json:"bullish"`
	Neutral       float64 `json:"neutral"`
	Bearish       float64 `json:"bearish"`
	Confidence    float64 `json:"confidence"`
	SocialScore   float64 `json:"social_score"`
	VolumeScore   float64 `json:"volume_score"`
	EvidenceCount int     `json:"evidence_count"`
	Summary       string  `json:"summary"`
}

type CryptoInsightResponse struct {
	Pair             string                  `json:"pair"`
	BaseAsset        string                  `json:"base_asset"`
	QuoteAsset       string                  `json:"quote_asset"`
	PriceSnapshot    CryptoPriceSnapshot     `json:"price_snapshot"`
	Signals          CryptoSignalSet         `json:"signals"`
	TopReasons       []CryptoReason          `json:"top_reasons"`
	EvidenceArticles []CryptoEvidenceArticle `json:"evidence_articles"`
	SocialSentiment  CryptoSocialSentiment   `json:"social_sentiment"`
	SocialPosts      []CryptoSocialPost      `json:"social_posts"`
	AIExplanation    string                  `json:"ai_explanation"`
	Disclaimer       string                  `json:"disclaimer"`
	UpdatedAt        time.Time               `json:"updated_at"`
	CacheTTLSeconds  int                     `json:"cache_ttl_seconds"`
}

type CryptoInsightSnapshot struct {
	Pair       string    `json:"pair"`
	HorizonSet string    `json:"horizon_set"`
	Payload    string    `json:"payload"`
	ComputedAt time.Time `json:"computed_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}
