package model

import "time"

type OpinionCondition struct {
	OpinionConditionID int64     `json:"opinion_condition_id"`
	ProjectID          int64     `json:"project_id"`
	Time               int       `json:"time"`
	Precise            int       `json:"precise"`
	Emotion            string    `json:"emotion"`
	Similar            int       `json:"similar"`
	Sort               int       `json:"sort"`
	Matchs             int       `json:"matchs"`
	Times              string    `json:"times"`
	Timee              string    `json:"timee"`
	Classify           string    `json:"classify"`
	Websitename        string    `json:"websitename"`
	Author             string    `json:"author"`
	Organization       string    `json:"organization"`
	Categorylable      string    `json:"categorylable"`
	Enterprisetype     string    `json:"enterprisetype"`
	Hightechtype       string    `json:"hightechtype"`
	Policylableflag    string    `json:"policylableflag"`
	DatasourceType     string    `json:"datasource_type"`
	EventIndex         string    `json:"eventIndex"`
	IndustryIndex      string    `json:"industryIndex"`
	Province           string    `json:"province"`
	City               string    `json:"city"`
	CreateTime         string    `json:"create_time,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}
