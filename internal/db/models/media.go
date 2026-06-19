package models

import (
	"time"

	"github.com/zergolf1994/goose"
)

type MediaMetadata struct {
	Size     interface{} `bson:"size,omitempty" json:"size,omitempty"`
	Width    int         `bson:"width" json:"width"`
	Height   int         `bson:"height" json:"height"`
	Duration float64     `bson:"duration" json:"duration"`
}

type Media struct {
	ID         string         `bson:"_id" json:"id"`
	Type       string         `bson:"type" json:"type"`
	FileName   *string        `bson:"fileName,omitempty" json:"fileName,omitempty"`
	MimeType   *string        `bson:"mimeType,omitempty" json:"mimeType,omitempty"`
	Resolution *string        `bson:"resolution,omitempty" json:"resolution,omitempty"`
	StorageID  *string        `bson:"storageId,omitempty" json:"storageId,omitempty"`
	Slug       string         `bson:"slug" json:"slug"`
	FileID     *string        `bson:"fileId,omitempty" json:"fileId,omitempty"`
	ClonedFrom *string        `bson:"clonedFrom,omitempty" json:"clonedFrom,omitempty"`
	Metadata   *MediaMetadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	DeletedAt  *time.Time     `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
	CreatedAt  time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time      `bson:"updatedAt" json:"updatedAt"`
}

var MediaModel = goose.NewModel[Media]("medias")
