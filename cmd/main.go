package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"server-transfer/internal/config"
	"server-transfer/internal/db/database"
	"server-transfer/internal/db/models"
	"server-transfer/internal/handlers"
	"server-transfer/internal/logger"
	"server-transfer/internal/middleware"
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
		port = "8085"
	}

	logDir := filepath.Dir(config.AppConfig.LogPath)
	h := handlers.NewHandler(handlers.Handler{LogDir: logDir})
	go handlers.GlobalHub.Run()
	go handlers.WatchLogDir(logDir)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"server-transfer","worker":"%s"}`, workerID)
	})
	mux.HandleFunc("/logs", h.HandleLogList)
	mux.HandleFunc("/logs/", h.HandleLogFile)
	mux.HandleFunc("/ui", h.HandleUI)
	mux.HandleFunc("/ws", h.HandleWS)

	go func() {
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Printf("📋 Log viewer skipped (port %s in use)", port)
			return
		}
		server := &http.Server{Handler: middleware.CORS(mux)}
		log.Printf("🌐 Log viewer: http://localhost:%s/ui", port)
		if err := server.Serve(ln); err != http.ErrServerClosed {
			log.Printf("⚠️ HTTP server error: %v", err)
		}
	}()

	go startHeartbeat(workerID)
	startWorkerLoop()
}

func startWorkerLoop() {
	log.Println("⚡ Worker Mode: Polling for transfer jobs...")
	log.Printf("🆔 Worker ID: %s", workerID)
	if config.AppConfig.StorageId != "" {
		log.Printf("📦 Local storage: %s → %s", config.AppConfig.StorageId, config.AppConfig.StoragePath)
	}
	utils.CleanOldLogs()

	ctx := context.Background()
	if isTransferEnabled(ctx) {
		log.Println("✅ transfer_enabled=true")
	} else {
		log.Println("⏸️  transfer_enabled=false — enable in db.settings")
	}

	const pollBusy = 5 * time.Second
	const pollIdle = 30 * time.Second
	idleRounds := 0
	var lastDisabledLog time.Time

	for {
		if !isTransferEnabled(ctx) {
			if time.Since(lastDisabledLog) > 5*time.Minute {
				log.Println("⏸️  transfer_enabled=false — set db.settings.transfer_enabled=true")
				lastDisabledLog = time.Now()
			}
			time.Sleep(pollIdle)
			continue
		}
		if !isWorkerEnabled(ctx) {
			if time.Since(lastDisabledLog) > 5*time.Minute {
				log.Printf("⏸️  worker %s disabled in db.workers", workerID)
				lastDisabledLog = time.Now()
			}
			time.Sleep(pollIdle)
			continue
		}
		if processNextJob(ctx) {
			idleRounds = 0
			time.Sleep(pollBusy)
		} else {
			idleRounds++
			if idleRounds == 1 || idleRounds%10 == 0 {
				logIdleDiagnostics(ctx)
			}
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
