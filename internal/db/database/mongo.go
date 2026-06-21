package database

import (
	"context"
	"log"
	"time"

	"server-transfer/internal/config"
	"server-transfer/internal/db/models"

	"github.com/zergolf1994/goose"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func Connect() error {
	if err := goose.Connect(config.AppConfig.MongoURI); err != nil {
		return err
	}
	ensureIndexes()
	return nil
}

func Disconnect() {
	if goose.Client() != nil {
		_ = goose.Close()
	}
}

func ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vpCol := models.VideoProcessModel.Col()

	// Drop legacy single-field index — conflicts with transcode/spritesheet processes
	vpCol.Indexes().DropOne(ctx, "postId_1")
	vpCol.Indexes().DropOne(ctx, "fileId_1")

	// One process per file per processType (transfer, transcode, spritesheet, …)
	_, err := vpCol.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "fileId", Value: 1}, {Key: "processType", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true),
	})
	if err != nil {
		log.Printf("⚠️  video_process index: %v", err)
	} else {
		log.Printf("✅ Unique index on video_process.{fileId,processType} ensured")
	}
}
