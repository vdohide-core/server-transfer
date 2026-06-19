package models

import (
	"time"

	"github.com/zergolf1994/goose"
)

type VideoProcess struct {
	ID             string      `bson:"_id" json:"id"`
	FileID         *string     `bson:"fileId,omitempty" json:"fileId,omitempty"`
	SpaceID        *string     `bson:"spaceId,omitempty" json:"spaceId,omitempty"`
	Slug           *string     `bson:"slug,omitempty" json:"slug,omitempty"`
	WorkerID       *string     `bson:"workerId,omitempty" json:"workerId,omitempty"`
	Status         *string     `bson:"status,omitempty" json:"status,omitempty"`
	OverallPercent *float64    `bson:"overallPercent,omitempty" json:"overallPercent,omitempty"`
	Timeline       interface{} `bson:"timeline,omitempty" json:"timeline,omitempty"`
	ProcessType    string      `bson:"processType" json:"processType"`
	Error          *string     `bson:"error,omitempty" json:"error,omitempty"`
	RetryCount     *int        `bson:"retryCount,omitempty" json:"retryCount,omitempty"`
	CreatedAt      time.Time   `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time   `bson:"updatedAt" json:"updatedAt"`
}

var VideoProcessModel = goose.NewModel[VideoProcess]("video_process")
