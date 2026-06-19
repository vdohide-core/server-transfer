package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"server-transfer/internal/logger"
)

func GenerateWorkerID() string {
	if envWorkerID := os.Getenv("WORKER_ID"); envWorkerID != "" {
		return envWorkerID
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s@1", hostname)
}

func RandomString(n int, special bool) string {
	if special {
		return RandomStringSpecial(n)
	}
	return RandomAlphaNum(n)
}

type ProcessLogger struct {
	file *os.File
}

func NewProcessLogger(slug string) *ProcessLogger {
	logDir := filepath.Join("logs", "process")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("⚠️ Failed to create log dir: %v", err)
		return &ProcessLogger{}
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", slug))
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("⚠️ Failed to open process log: %v", err)
		return &ProcessLogger{}
	}
	log.SetOutput(f)
	return &ProcessLogger{file: f}
}

func (pl *ProcessLogger) Close() {
	log.SetOutput(logger.GlobalWriter)
	if pl.file != nil {
		pl.file.Close()
	}
}

func LogMain(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Printf("%s", msg)
	fmt.Fprintf(logger.GlobalWriter, "%s %s\n", time.Now().Format("2006/01/02 15:04:05"), msg)
}

func CleanOldLogs() {
	logDir := filepath.Join("logs", "process")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(logDir, entry.Name()))
			removed++
		}
	}
	if removed > 0 {
		log.Printf("🧹 Removed %d old log files", removed)
	}
}
