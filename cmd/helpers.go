package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"server-transfer/internal/db/models"
	"server-transfer/internal/utils"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func newUUID() string { return uuid.New().String() }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func isCancelled(ctx context.Context, processID string) bool {
	p, err := models.VideoProcessModel.FindByID(ctx, processID)
	if err != nil {
		return false
	}
	return derefStr(p.Status) == models.ProcessStatusCancelled
}

func failProcess(ctx context.Context, processID, slug, errMsg string) {
	utils.LogMain("❌ [%s] ERROR: %s", slug, errMsg)
	retryNum := 1
	current, _ := models.VideoProcessModel.FindByID(ctx, processID)
	if current != nil && current.RetryCount != nil {
		retryNum = *current.RetryCount + 1
	}
	_, _ = models.VideoProcessModel.Col().UpdateOne(ctx,
		bson.M{"_id": processID},
		bson.M{"$set": bson.M{
			"status":     models.ProcessStatusFailed,
			"error":      errMsg,
			"retryCount": retryNum,
			"updatedAt":  time.Now(),
		}},
	)
	log.Printf("❌ [%s] Failed (retry %d/3): %s", slug, retryNum, errMsg)
}

func updateTimelineStep(ctx context.Context, processID, step, status string, percent float64) {
	models.VideoProcessModel.UpdateByID(ctx, processID, bson.M{"$set": bson.M{
		fmt.Sprintf("timeline.%s.status", step):  status,
		fmt.Sprintf("timeline.%s.percent", step): percent,
		"updatedAt": time.Now(),
	}})
}

func startStep(ctx context.Context, processID, step string) {
	now := time.Now()
	models.VideoProcessModel.UpdateByID(ctx, processID, bson.M{"$set": bson.M{
		fmt.Sprintf("timeline.%s.status", step):    models.StepStatusProcessing,
		fmt.Sprintf("timeline.%s.percent", step):   0,
		fmt.Sprintf("timeline.%s.startedAt", step): now,
		"updatedAt": now,
	}})
}

func completeStep(ctx context.Context, processID, step string) {
	now := time.Now()
	models.VideoProcessModel.UpdateByID(ctx, processID, bson.M{"$set": bson.M{
		fmt.Sprintf("timeline.%s.status", step):  models.StepStatusCompleted,
		fmt.Sprintf("timeline.%s.percent", step): 100,
		fmt.Sprintf("timeline.%s.endedAt", step): now,
		"updatedAt": now,
	}})
}

func updateOverallPercent(ctx context.Context, processID string, percent float64) {
	models.VideoProcessModel.UpdateByID(ctx, processID, bson.M{"$set": bson.M{
		"overallPercent": percent,
		"updatedAt":      time.Now(),
	}})
}

func DetermineHighestResolution(height int) int {
	threshold := func(t int) int { return t * 95 / 100 }
	if height >= threshold(1080) {
		return 1080
	}
	if height >= threshold(720) {
		return 720
	}
	if height >= threshold(480) {
		return 480
	}
	return 360
}

func cloneMediaToClonedFiles(ctx context.Context, sourceFileID string, media models.Media, slug string) {
	cursor, err := models.FileModel.FindRaw(ctx, bson.M{
		"clonedFrom":         sourceFileID,
		"type":               models.FileTypeVideo,
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var clonedFile models.File
		if err := cursor.Decode(&clonedFile); err != nil {
			continue
		}
		filter := bson.M{"fileId": clonedFile.ID, "type": media.Type}
		if media.Resolution != nil {
			filter["resolution"] = *media.Resolution
		}
		existCount, _ := models.MediaModel.CountDocuments(ctx, filter)
		if existCount > 0 {
			continue
		}
		now := time.Now()
		clonedMedia := models.Media{
			ID:         uuid.New().String(),
			Type:       media.Type,
			FileName:   media.FileName,
			MimeType:   media.MimeType,
			Resolution: media.Resolution,
			StorageID:  media.StorageID,
			Slug:       utils.RandomString(11, true),
			FileID:     &clonedFile.ID,
			Metadata:   media.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		clonedFrom := sourceFileID
		clonedMedia.ClonedFrom = &clonedFrom
		if _, err := models.MediaModel.Create(ctx, &clonedMedia); err != nil {
			log.Printf("⚠️  [%s] Failed to clone media to %s: %v", slug, clonedFile.ID, err)
		}
	}
}

func updateClonedFilesReady(ctx context.Context, sourceFileID string, highest int, slug string) {
	now := time.Now()
	update := bson.M{"status": models.FileStatusReady, "updatedAt": now}
	if highest > 0 {
		update["metadata.highest"] = highest
	}
	result, _ := models.FileModel.UpdateMany(ctx, bson.M{
		"clonedFrom":         sourceFileID,
		"type":               models.FileTypeVideo,
		"status":             models.FileStatusReadyOriginal,
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	}, bson.M{"$set": update})
	if result != nil && result.ModifiedCount > 0 {
		log.Printf("📋 [%s] Updated %d cloned files → ready", slug, result.ModifiedCount)
	}
}

func s3ObjectKey(fileID, fileName string) string {
	return fmt.Sprintf("%s/%s", fileID, fileName)
}

var transcodedResolutions = []string{
	models.Resolution1080,
	models.Resolution720,
	models.Resolution480,
	models.Resolution360,
}

// needsPendingAssetInstall returns true when pending ingests exist for assets
// that do not yet have a global media record (one resolution = one media).
func needsPendingAssetInstall(ctx context.Context, fileID string) bool {
	if hasPendingSpriteIngest(ctx, fileID) && !hasThumbnailMedia(ctx, fileID) {
		return true
	}
	for _, res := range transcodedResolutions {
		if hasVideoMedia(ctx, fileID, res) {
			continue
		}
		if hasPendingIngestForFileName(ctx, fileID, models.ResolutionToFileName[res]) {
			return true
		}
	}
	return false
}

func hasPendingIngestForFileName(ctx context.Context, fileID, fileName string) bool {
	count, _ := models.IngestModel.CountDocuments(ctx, bson.M{
		"fileId":     fileID,
		"fileName":   fileName,
		"sourceType": models.IngestSourceTypeProcessed,
		"deletedAt":  bson.M{"$exists": false},
	})
	return count > 0
}

// hasVideoMedia checks medias collection globally (any storage) for this resolution.
func hasVideoMedia(ctx context.Context, fileID, resolution string) bool {
	count, _ := models.MediaModel.CountDocuments(ctx, bson.M{
		"fileId":     fileID,
		"type":       models.MediaTypeVideo,
		"resolution": resolution,
		"deletedAt":  bson.M{"$exists": false},
	})
	return count > 0
}

// hasThumbnailMedia checks medias collection globally (any storage).
func hasThumbnailMedia(ctx context.Context, fileID string) bool {
	count, _ := models.MediaModel.CountDocuments(ctx, bson.M{
		"fileId":    fileID,
		"type":      models.MediaTypeThumbnail,
		"deletedAt": bson.M{"$exists": false},
	})
	return count > 0
}

func hasPendingSpriteIngest(ctx context.Context, fileID string) bool {
	count, _ := models.IngestModel.CountDocuments(ctx, bson.M{
		"fileId":     fileID,
		"fileName":   models.SpriteZipName,
		"sourceType": models.IngestSourceTypeProcessed,
		"deletedAt":  bson.M{"$exists": false},
	})
	return count > 0
}

// needsTransfer returns true while there are pending S3 ingests to process.
func needsTransfer(ctx context.Context, fileID string) bool {
	count, _ := models.IngestModel.CountDocuments(ctx, bson.M{
		"fileId":     fileID,
		"sourceType": models.IngestSourceTypeProcessed,
		"deletedAt":  bson.M{"$exists": false},
	})
	return count > 0
}

func softDeleteIngests(ctx context.Context, fileID string, fileNames []string, slug string) {
	if len(fileNames) == 0 {
		return
	}
	now := time.Now()
	for _, fileName := range fileNames {
		path := s3ObjectKey(fileID, fileName)
		result, err := models.IngestModel.Col().UpdateMany(ctx, bson.M{
			"fileId":     fileID,
			"sourceType": models.IngestSourceTypeProcessed,
			"deletedAt":  bson.M{"$exists": false},
			"$or": []bson.M{
				{"fileName": fileName},
				{"path": path},
			},
		}, bson.M{"$set": bson.M{"deletedAt": now, "updatedAt": now}})
		if err != nil {
			log.Printf("⚠️  [%s] soft-delete ingest %s: %v", slug, fileName, err)
			continue
		}
		if result.ModifiedCount > 0 {
			log.Printf("🗑️  [%s] Soft-deleted ingest: %s (%d)", slug, fileName, result.ModifiedCount)
		}
	}
}

func categorizeError(errMsg string) string {
	e := strings.ToLower(errMsg)
	switch {
	case strings.Contains(e, "s3") || strings.Contains(e, "download"):
		return "network"
	case strings.Contains(e, "ffmpeg") || strings.Contains(e, "encode"):
		return "ffmpeg"
	case strings.Contains(e, "sprite") || strings.Contains(e, "thumbnail"):
		return "thumbnail"
	default:
		return "unknown"
	}
}

func isPurgeResolution(res string) bool {
	switch res {
	case models.Resolution360, models.Resolution480, models.Resolution720, models.Resolution1080:
		return true
	default:
		return false
	}
}

// purgePlaylistCache purges playlist.m3u8 from Cloudflare for the file and its clones.
func purgePlaylistCache(ctx context.Context, slug, fileID string) {
	domainSetting, err := models.SettingModel.FindOne(ctx, bson.M{"name": models.SettingDomainPlaylist})
	if err != nil {
		return
	}
	domain := domainSetting.GetString("")
	if domain == "" {
		return
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	domain = strings.TrimRight(domain, "/")

	zoneSetting, err := models.SettingModel.FindOne(ctx, bson.M{"name": models.SettingCfZoneID})
	if err != nil {
		return
	}
	tokenSetting, err := models.SettingModel.FindOne(ctx, bson.M{"name": models.SettingCfApiToken})
	if err != nil {
		return
	}

	cfConfig := utils.CloudflareConfig{
		ZoneID:   zoneSetting.GetString(""),
		APIToken: tokenSetting.GetString(""),
	}
	if cfConfig.ZoneID == "" || cfConfig.APIToken == "" {
		return
	}

	purgeURLs := []string{fmt.Sprintf("%s/%s/playlist.m3u8", domain, slug)}

	cursor, err := models.FileModel.FindRaw(ctx, bson.M{
		"clonedFrom":         fileID,
		"type":               models.FileTypeVideo,
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	}, options.Find().SetProjection(bson.M{"slug": 1}))
	if err == nil {
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var clonedFile models.File
			if err := cursor.Decode(&clonedFile); err != nil {
				continue
			}
			if clonedFile.Slug != "" {
				purgeURLs = append(purgeURLs, fmt.Sprintf("%s/%s/playlist.m3u8", domain, clonedFile.Slug))
			}
		}
	}

	log.Printf("☁️  [%s] Purging %d playlist URL(s) from Cloudflare cache...", slug, len(purgeURLs))
	for _, u := range purgeURLs {
		log.Printf("   → %s", u)
	}

	if err := utils.PurgeCloudflareCache(cfConfig, purgeURLs); err != nil {
		log.Printf("⚠️  [%s] Cloudflare purge failed: %v", slug, err)
	} else {
		log.Printf("✅ [%s] Cloudflare cache purged", slug)
	}
}
