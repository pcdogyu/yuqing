package model

import "time"

type Item struct {
	ID                 int64     `json:"id"`
	SourceType         string    `json:"source_type"`
	SourceKey          string    `json:"source_key"`
	Title              string    `json:"title"`
	Content            string    `json:"content"`
	Summary            string    `json:"summary"`
	PublishTime        string    `json:"publish_time"`
	PublishTimeText    string    `json:"publish_time_text"`
	DetailURL          string    `json:"detail_url"`
	SourceURL          string    `json:"source_url"`
	TagFlags           string    `json:"tag_flags"`
	FromText           string    `json:"from_text"`
	ExternalSourceHost string    `json:"external_source_host"`
	IsVIP              bool      `json:"is_vip"`
	HasImage           bool      `json:"has_image"`
	RawPayload         string    `json:"raw_payload"`
	ProjectIDs         []int64   `json:"project_ids,omitempty"`
	Favorited          bool      `json:"favorited"`
	Read               bool      `json:"read"`
	CapturedAt         time.Time `json:"captured_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CrawlRun struct {
	ID            int64      `json:"id"`
	SourceType    string     `json:"source_type"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Status        string     `json:"status"`
	FetchedCount  int        `json:"fetched_count"`
	InsertedCount int        `json:"inserted_count"`
	UpdatedCount  int        `json:"updated_count"`
	ErrorText     string     `json:"error_text,omitempty"`
}

type CrawlSummary struct {
	SourceType    string `json:"source_type"`
	FetchedCount  int    `json:"fetched_count"`
	InsertedCount int    `json:"inserted_count"`
	UpdatedCount  int    `json:"updated_count"`
	RunID         int64  `json:"run_id"`
	Items         []Item `json:"items,omitempty"`
	ErrorText     string `json:"error_text,omitempty"`
}

type ItemListResult struct {
	Items    []Item `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int    `json:"total"`
}

type ArticleFilter struct {
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	Keyword    string `json:"keyword"`
	SourceType string `json:"source_type"`
	ProjectID  int64  `json:"project_id"`
	UserID     int64  `json:"user_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Industry   string `json:"industry"`
	Province   string `json:"province"`
	City       string `json:"city"`
	Mode       string `json:"mode"`
	Sort       string `json:"sort"`
	Read       string `json:"read"`
	Favorite   string `json:"favorite"`
	Limit      int    `json:"limit"`
}

type SearchFacetBucket struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type SearchFacets struct {
	Sources    []SearchFacetBucket `json:"sources"`
	Projects   []SearchFacetBucket `json:"projects"`
	Industries []SearchFacetBucket `json:"industries"`
	Provinces  []SearchFacetBucket `json:"provinces"`
	Cities     []SearchFacetBucket `json:"cities"`
}

type SearchOptions struct {
	Industries []string `json:"industries"`
	Provinces  []string `json:"provinces"`
	Cities     []string `json:"cities"`
}
