package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"server-transfer/internal/config"
	"server-transfer/internal/db/database"
	"server-transfer/internal/db/models"
	"server-transfer/internal/logger"
	"server-transfer/internal/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var workerID string

func main() {
	config.Load()
	workerID = utils.GenerateWorkerID()
	log.Printf("Starting Server Transfer [Worker: %s]", workerID)

	logCloser, err := logger.Init(config.AppConfig.LogPath)
	if err != nil {
		log.Printf("⚠️ File logging disabled: %v", err)
	} else {
		defer logCloser.Close()
	}

	if err := database.Connect(); err != nil {
		log.Printf("ERROR: MongoDB: %v", err)
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}
	defer database.Disconnect()
	log.Println("✅ MongoDB connected")

	if config.AppConfig.StorageId == "" || config.AppConfig.StoragePath == "" {
		log.Println("⚠️  STORAGE_ID and STORAGE_PATH must be set (same machine as server-storage)")
	}

	port := config.AppConfig.Port
	if port == "" {
		port = "8084"
	}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","service":"server-transfer","worker":"%s"}`, workerID)
		})
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Printf("Health check skipped (port %s in use by another worker)", port)
			return
		}
		log.Printf("Health: http://localhost:%s/health", port)
		if err := http.Serve(ln, mux); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	go startHeartbeat(workerID)
	startWorkerLoop()
}

func startWorkerLoop() {
	log.Println("⚡ Worker Mode: Polling for transfer jobs...")
	utils.CleanOldLogs()

	ctx := context.Background()
	const pollBusy = 5 * time.Second
	const pollIdle = 30 * time.Second

	for {
		if !isTransferEnabled(ctx) || !isWorkerEnabled(ctx) {
			time.Sleep(pollIdle)
			continue
		}
		if processNextJob(ctx) {
			time.Sleep(pollBusy)
		} else {
			time.Sleep(pollIdle)
		}
	}
}

func isTransferEnabled(ctx context.Context) bool {
	setting, err := models.SettingModel.FindOne(ctx, bson.M{"name": models.SettingTransferEnabled})
	if err != nil {
		if err == mongo.ErrNoDocuments {
			now := time.Now()
			newSetting := models.SettingModel.New()
			newSetting.ID = newUUID()
			newSetting.Name = models.SettingTransferEnabled
			newSetting.Value = false
			newSetting.CreatedAt = now
			newSetting.UpdatedAt = now
			models.SettingModel.Create(ctx, newSetting)
			log.Println("⚙️  Created 'transfer_enabled' = false")
		}
		return false
	}
	return setting.GetBool(false)
}

func isWorkerEnabled(ctx context.Context) bool {
	worker, err := models.WorkerModel.FindOne(ctx, bson.M{"workerId": workerID})
	if err != nil {
		return true
	}
	return worker.Enable
}
