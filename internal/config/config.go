package config

import (
	"os"

	"github.com/joho/godotenv"
)

var AppConfig Config

type Config struct {
	Port        string
	MongoURI    string
	StorageId   string
	StoragePath string
	LogPath     string
}

func Load() {
	_ = godotenv.Load()

	mongoURI := getEnv("MONGODB_URI", "")
	if mongoURI == "" {
		mongoURI = getEnv("MONGO_URI", "")
	}
	if mongoURI == "" {
		mongoURI = getEnv("DATABASE_URL", "mongodb://localhost:27017")
	}

	AppConfig = Config{
		Port:        getEnv("PORT", "8085"),
		MongoURI:    mongoURI,
		StorageId:   getEnv("STORAGE_ID", ""),
		StoragePath: getEnv("STORAGE_PATH", "/home/files"),
		LogPath:     getEnv("LOG_PATH", "logs/server-transfer.log"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
