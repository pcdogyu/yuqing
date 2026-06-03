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
