package models

import (
	"github.com/zergolf1994/goose"
)

type StorageS3Config struct {
	Endpoint        *string `bson:"endpoint,omitempty" json:"endpoint,omitempty"`
	Region          string  `bson:"region" json:"region"`
	Bucket          string  `bson:"bucket" json:"bucket"`
	Prefix          string  `bson:"prefix" json:"prefix"`
	AccessKeyID     string  `bson:"accessKeyId" json:"-"`
	SecretAccessKey string  `bson:"secretAccessKey" json:"-"`
	ForcePathStyle  bool    `bson:"forcePathStyle" json:"forcePathStyle"`
}

type StorageCapacity struct {
	Total      interface{} `bson:"total" json:"total"`
	Used       interface{} `bson:"used" json:"used"`
	Free       interface{} `bson:"free" json:"free"`
	Percentage float64     `bson:"percentage" json:"percentage"`
}

type Storage struct {
	ID       string           `bson:"_id" json:"id"`
	Name     string           `bson:"name" json:"name"`
	Enable   bool             `bson:"enable" json:"enable"`
	Type     string           `bson:"type" json:"type"`
	Status   string           `bson:"status" json:"status"`
	S3       *StorageS3Config `bson:"s3,omitempty" json:"s3,omitempty"`
	Capacity *StorageCapacity `bson:"capacity,omitempty" json:"capacity,omitempty"`
}

var StorageModel = goose.NewModel[Storage]("storages")

func (s *Storage) IsOnline() bool {
	return s.Enable && s.Status == StorageStatusOnline
}
