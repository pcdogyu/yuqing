package model

import "time"

type PlatformBinding struct {
	UserID    int64     `json:"user_id"`
	Kind      string    `json:"kind"`
	SecretID  string    `json:"secret_id"`
	SecretKey string    `json:"secret_key"`
	Bound     bool      `json:"bound"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PublicOption struct {
	ID                  int64     `json:"id"`
	UserID              int64     `json:"user_id"`
	EventName           string    `json:"eventname"`
	EventKeywords       string    `json:"eventkeywords"`
	EventStopWords      string    `json:"eventstopwords"`
	EventStartTime      string    `json:"eventstarttime"`
	EventEndTime        string    `json:"eventendtime"`
	CreateTime          time.Time `json:"createtime"`
	Status              int       `json:"status"`
	Updatetime          time.Time `json:"updatetime"`
	DetailStatus        int       `json:"detail_status"`
	EmotionalIndex      string    `json:"emotionalIndex"`
	BackAnalysis        string    `json:"back_analysis,omitempty"`
	EventContext        string    `json:"event_context,omitempty"`
	EventTrace          string    `json:"event_trace,omitempty"`
	HotAnalysis         string    `json:"hot_analysis,omitempty"`
	NetizensAnalysis    string    `json:"netizens_analysis,omitempty"`
	Statistics          string    `json:"statistics,omitempty"`
	PropagationAnalysis string    `json:"propagation_analysis,omitempty"`
	ThematicAnalysis    string    `json:"thematic_analysis,omitempty"`
	UnscrambleContent   string    `json:"unscramble_content,omitempty"`
	ContentAnalysis     string    `json:"content_analysis,omitempty"`
	Page                int       `json:"page,omitempty"`
}
