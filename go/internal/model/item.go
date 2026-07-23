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

type CrawlOptions struct {
	Start     string `json:"start,omitempty"`
	End       string `json:"end,omitempty"`
	TimeField string `json:"time_field,omitempty"`
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
	Lite       bool   `json:"lite,omitempty"`
}

type ArticleCleanupFilter struct {
	Mode          string `json:"mode,omitempty"`
	RetentionDays int    `json:"retention_days,omitempty"`
	Scope         string `json:"scope,omitempty"`
	Confirm       bool   `json:"confirm,omitempty"`
	Cutoff        string `json:"cutoff,omitempty"`
}

type ArticleCleanupSourceCount struct {
	SourceType string `json:"source_type"`
	Count      int    `json:"count"`
}

type ArticleCleanupResult struct {
	Mode          string                      `json:"mode,omitempty"`
	RetentionDays int                         `json:"retention_days"`
	Scope         string                      `json:"scope"`
	Cutoff        string                      `json:"cutoff"`
	Total         int                         `json:"total"`
	Sources       []ArticleCleanupSourceCount `json:"sources"`
	Applied       bool                        `json:"applied"`
	Affected      int                         `json:"affected"`
	Tombstoned    int                         `json:"tombstoned"`
	BeforeTotal   int                         `json:"before_total"`
	AfterTotal    int                         `json:"after_total"`
}
type HotspotSwitchingResult struct {
	Days              int                    `json:"days"`
	StartDate         string                 `json:"start_date"`
	EndDate           string                 `json:"end_date"`
	RecentDays        int                    `json:"recent_days"`
	PreviousDays      int                    `json:"previous_days"`
	TotalArticles     int                    `json:"total_articles"`
	TodayTop          []HotspotSwitchingItem `json:"today_top"`
	Top               []HotspotSwitchingItem `json:"top"`
	Rising            []HotspotSwitchingItem `json:"rising"`
	DiscoveryTodayTop []HotspotSwitchingItem `json:"discovery_today_top"`
	DiscoveryTop      []HotspotSwitchingItem `json:"discovery_top"`
	DiscoveryRising   []HotspotSwitchingItem `json:"discovery_rising"`
	Cooling           []HotspotSwitchingItem `json:"cooling"`
	New               []HotspotSwitchingItem `json:"new"`
	ContinuousRising  []HotspotSwitchingItem `json:"continuous_rising"`
	Switches          []HotspotSwitchingPair `json:"switches"`
	Daily             []HotspotDailyHotspot  `json:"daily"`
}

type HotspotSwitchingItem struct {
	Keyword       string              `json:"keyword"`
	Count14D      int                 `json:"count_14d"`
	CountRecent7D int                 `json:"count_recent_7d"`
	CountPrev7D   int                 `json:"count_prev_7d"`
	ChangeRate    float64             `json:"change_rate"`
	SwitchScore   float64             `json:"switch_score"`
	FirstSeenDate string              `json:"first_seen_date"`
	LastSeenDate  string              `json:"last_seen_date"`
	ActiveDays    int                 `json:"active_days"`
	TodayCount    int                 `json:"today_count"`
	Trend         []HotspotDailyCount `json:"trend,omitempty"`
}

type HotspotDailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type HotspotDailyHotspot struct {
	Date  string                 `json:"date"`
	Items []HotspotSwitchingItem `json:"items"`
}

type HotspotSwitchingPair struct {
	From        string  `json:"from"`
	To          string  `json:"to"`
	FromCount   int     `json:"from_count"`
	ToCount     int     `json:"to_count"`
	SwitchScore float64 `json:"switch_score"`
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
