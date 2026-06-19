package models

import (
	"time"

	"github.com/zergolf1994/goose"
)

type FileMetadata struct {
	Duration  *float64    `bson:"duration,omitempty" json:"duration,omitempty"`
	Highest   *int        `bson:"highest,omitempty" json:"highest,omitempty"`
	Size      interface{} `bson:"size,omitempty" json:"size,omitempty"`
	TrashedAt *time.Time  `bson:"trashedAt,omitempty" json:"trashedAt,omitempty"`
	DeletedAt *time.Time  `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
}

type File struct {
	ID         string        `bson:"_id" json:"id"`
	Status     string        `bson:"status" json:"status"`
	Type       string        `bson:"type" json:"type"`
	Name       string        `bson:"name" json:"name"`
	SpaceID    *string       `bson:"spaceId,omitempty" json:"spaceId,omitempty"`
	Slug       string        `bson:"slug" json:"slug"`
	ClonedFrom *string       `bson:"clonedFrom,omitempty" json:"clonedFrom,omitempty"`
	Metadata   *FileMetadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	CreatedAt  time.Time     `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time     `bson:"updatedAt" json:"updatedAt"`
}

var FileModel = goose.NewModel[File]("files")
