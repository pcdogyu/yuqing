package model

import (
	"encoding/json"
	"time"
)

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
	ID               int64      `json:"id"`
	SourceType       string     `json:"source_type"`
	TemplateID       int64      `json:"template_id,omitempty"`
	TemplateName     string     `json:"template_name,omitempty"`
	TemplateSnapshot string     `json:"template_snapshot,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	Status           string     `json:"status"`
	FetchedCount     int        `json:"fetched_count"`
	InsertedCount    int        `json:"inserted_count"`
	UpdatedCount     int        `json:"updated_count"`
	ErrorText        string     `json:"error_text,omitempty"`
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

type CrawlTemplate struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Website    string    `json:"website"`
	SourceType string    `json:"source_type"`
	Enabled    bool      `json:"enabled"`
	ConfigJSON string    `json:"config_json"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CrawlTemplateConfig struct {
	SourceType     string                  `json:"source_type"`
	BaseURL        string                  `json:"base_url"`
	Method         string                  `json:"method"`
	Headers        map[string]string       `json:"headers,omitempty"`
	Cookies        map[string]string       `json:"cookies,omitempty"`
	Query          map[string]string       `json:"query,omitempty"`
	Body           string                  `json:"body,omitempty"`
	Pagination     CrawlTemplatePagination `json:"pagination,omitempty"`
	ListSelector   string                  `json:"list_selector,omitempty"`
	DetailSelector string                  `json:"detail_selector,omitempty"`
	DetailURLField string                  `json:"detail_url_field,omitempty"`
	Fields         []CrawlTemplateField    `json:"fields,omitempty"`
}

type CrawlTemplatePagination struct {
	Enabled bool   `json:"enabled"`
	Param   string `json:"param,omitempty"`
	Start   int    `json:"start,omitempty"`
	End     int    `json:"end,omitempty"`
	Step    int    `json:"step,omitempty"`
}

type CrawlTemplateField struct {
	Scope    string `json:"scope,omitempty"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
	Attr     string `json:"attr,omitempty"`
	From     string `json:"from,omitempty"`
	Value    string `json:"value,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Suffix   string `json:"suffix,omitempty"`
	Join     string `json:"join,omitempty"`
	Trim     bool   `json:"trim,omitempty"`
	Required bool   `json:"required,omitempty"`
}

func (c CrawlTemplateConfig) MarshalJSON() ([]byte, error) {
	type alias CrawlTemplateConfig
	return json.Marshal(alias(c))
}

func (c *CrawlTemplateConfig) UnmarshalJSON(data []byte) error {
	type alias CrawlTemplateConfig
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*c = CrawlTemplateConfig(v)
	return nil
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
	TimeField  string `json:"time_field"`
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

type SearchWordStat struct {
	SearchWord string `json:"search_word"`
	UserID     int64  `json:"user_id"`
	WordCount  int    `json:"wordCount"`
}

type SearchDetail struct {
	ID              int64          `json:"id"`
	SourceType      string         `json:"source_type"`
	Title           string         `json:"title"`
	Content         string         `json:"content"`
	Summary         string         `json:"summary"`
	PublishTime     string         `json:"publish_time"`
	PublishTimeText string         `json:"publish_time_text"`
	DetailURL       string         `json:"detail_url"`
	SourceURL       string         `json:"source_url"`
	SourceName      string         `json:"source_name"`
	URL             string         `json:"url"`
	Payload         map[string]any `json:"payload"`
}
