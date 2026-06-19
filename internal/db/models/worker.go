package models

import (
	"time"

	"github.com/zergolf1994/goose"
)

type Worker struct {
	ID          string    `bson:"_id" json:"id"`
	WorkerID    string    `bson:"workerId" json:"workerId"`
	Enable      bool      `bson:"enable" json:"enable"`
	HeartbeatAt time.Time `bson:"heartbeatAt" json:"heartbeatAt"`
}

var WorkerModel = goose.NewModel[Worker]("workers")
