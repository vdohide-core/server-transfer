package models

import (
	"time"

	"github.com/zergolf1994/goose"
)

type Setting struct {
	ID        string      `bson:"_id" json:"id"`
	Name      string      `bson:"name" json:"name"`
	Value     interface{} `bson:"value" json:"value"`
	CreatedAt time.Time   `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time   `bson:"updatedAt" json:"updatedAt"`
}

var SettingModel = goose.NewModel[Setting]("settings")

func (s *Setting) GetBool(defaultVal bool) bool {
	if v, ok := s.Value.(bool); ok {
		return v
	}
	if v, ok := s.Value.(string); ok {
		return v == "true"
	}
	return defaultVal
}
