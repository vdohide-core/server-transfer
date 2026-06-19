package main

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"server-transfer/internal/db/models"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func startHeartbeat(wID string) {
	log.Printf("💓 Starting heartbeat (workerId=%s)", wID)
	hostname, pid := parseWorkerID(wID)
	ip := getOutboundIP()

	doHeartbeat := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		activeJobs, _ := models.VideoProcessModel.Col().CountDocuments(ctx, bson.M{
			"workerId": wID, "status": models.ProcessStatusProcessing, "processType": models.ProcessTypeTransfer,
		})
		status := "idle"
		if activeJobs > 0 {
			status = "busy"
		}
		sys := gatherSystemInfo()
		now := time.Now()
		filter := bson.M{"workerId": wID}
		update := bson.M{
			"$set": bson.M{
				"hostname": hostname, "ip": ip, "pid": pid, "type": "transfer",
				"status": status, "activeJobs": activeJobs, "maxJobs": 1,
				"system": sys, "heartbeatAt": now, "updatedAt": now,
			},
			"$setOnInsert": bson.M{"_id": uuid.New().String(), "enable": true, "createdAt": now},
		}
		if _, err := models.WorkerModel.Col().UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
			log.Printf("⚠️ Heartbeat failed: %v", err)
		}
	}

	doHeartbeat()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		doHeartbeat()
	}
}

func parseWorkerID(wID string) (string, int) {
	parts := strings.SplitN(wID, "@", 2)
	return parts[0], os.Getpid()
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "unknown"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

type systemInfo struct {
	DiskTotal  int64   `bson:"diskTotal,omitempty"`
	DiskUsed   int64   `bson:"diskUsed,omitempty"`
	DiskFree   int64   `bson:"diskFree,omitempty"`
	MemTotal   int64   `bson:"memTotal,omitempty"`
	MemUsed    int64   `bson:"memUsed,omitempty"`
	CPUPercent float64 `bson:"cpuPercent,omitempty"`
}

func gatherSystemInfo() *systemInfo {
	info := &systemInfo{}
	info.DiskTotal, info.DiskUsed, info.DiskFree = getDiskUsage("/")
	info.MemTotal, info.MemUsed = getMemoryUsage()
	info.CPUPercent = getCPUPercent()
	return info
}
