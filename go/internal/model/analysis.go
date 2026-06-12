package model

type EmotionBucket struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Ratio float64 `json:"ratio"`
}

type EmotionAnalysis struct {
	ProjectID int64           `json:"project_id"`
	Total     int             `json:"total"`
	Buckets   []EmotionBucket `json:"buckets"`
}

type EventOverview struct {
	ProjectID    int64    `json:"project_id"`
	ProjectName  string   `json:"project_name"`
	Keyword      string   `json:"keyword"`
	Count        int      `json:"count"`
	LatestTitles []string `json:"latest_titles"`
}

type PropagationNode struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type PropagationAnalysis struct {
	ProjectID  int64             `json:"project_id"`
	SourceFlow []PropagationNode `json:"source_flow"`
	Trend      []TrendPoint      `json:"trend"`
}

type ThemeInsight struct {
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Samples []string `json:"samples"`
}

type PublicOpinionEvent struct {
	Title       string `json:"title"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`
	Keyword     string `json:"keyword"`
	Count       int    `json:"count"`
	Summary     string `json:"summary"`
}

type PublicOpinionReport struct {
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
}

type PublicOpinionAnalysisBundle struct {
	EventName           string `json:"eventname"`
	EventKeywords       string `json:"eventkeywords"`
	EventStopWords      string `json:"eventstopwords"`
	EventStartTime      string `json:"eventstarttime"`
	EventEndTime        string `json:"eventendtime"`
	EmotionalIndex      string `json:"emotional_index"`
	ArticleCount        int    `json:"article_count"`
	BackAnalysis        string `json:"back_analysis"`
	EventContext        string `json:"event_context"`
	EventTrace          string `json:"event_trace"`
	HotAnalysis         string `json:"hot_analysis"`
	NetizensAnalysis    string `json:"netizens_analysis"`
	Statistics          string `json:"statistics"`
	PropagationAnalysis string `json:"propagation_analysis"`
	ThematicAnalysis    string `json:"thematic_analysis"`
	UnscrambleContent   string `json:"unscramble_content"`
	ContentAnalysis     string `json:"content_analysis"`
}

type PublicOpinionAnalysisView struct {
	Status        string                      `json:"status"`
	Message       string                      `json:"message"`
	Bundle        PublicOpinionAnalysisBundle `json:"bundle"`
	EventOverview []EventOverview             `json:"event_overview"`
	Emotions      EmotionAnalysis             `json:"emotions"`
	Propagation   PropagationAnalysis         `json:"propagation"`
	Themes        []ThemeInsight              `json:"themes"`
	Events        []PublicOpinionEvent        `json:"events"`
	Reports       []PublicOpinionReport       `json:"reports"`
}
