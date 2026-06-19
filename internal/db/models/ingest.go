package models

import (
	"time"

	"github.com/zergolf1994/goose"
)

type Ingest struct {
	ID         string    `bson:"_id" json:"id"`
	FileID     *string   `bson:"fileId,omitempty" json:"fileId,omitempty"`
	StorageID  *string   `bson:"storageId,omitempty" json:"storageId,omitempty"`
	FileName   string    `bson:"fileName" json:"fileName"`
	Status     string    `bson:"status" json:"status"`
	Size       int64     `bson:"size" json:"size"`
	Path       *string   `bson:"path,omitempty" json:"path,omitempty"`
	SourceType string     `bson:"sourceType" json:"sourceType"`
	DeletedAt  *time.Time `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
	CreatedAt  time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time `bson:"updatedAt" json:"updatedAt"`
}

var IngestModel = goose.NewModel[Ingest]("ingests")
