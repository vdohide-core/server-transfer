package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"server-transfer/internal/archive"
	"server-transfer/internal/config"
	"server-transfer/internal/db/models"
	"server-transfer/internal/downloader"
	"server-transfer/internal/install"
	"server-transfer/internal/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var allResolutions = []string{
	models.ResolutionOriginal,
	models.Resolution1080,
	models.Resolution720,
	models.Resolution480,
	models.Resolution360,
}

func findAndClaimFile(ctx context.Context) (*models.VideoProcess, *models.File, error) {
	if reason := localStorageBlockReason(ctx); reason != "" {
		return nil, nil, fmt.Errorf("%s", reason)
	}

	filter := bson.M{
		"status": bson.M{"$in": []string{
			models.FileStatusReadyOriginal,
			models.FileStatusReady,
		}},
		"type":               models.FileTypeVideo,
		"clonedFrom":         bson.M{"$exists": false},
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	}

	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(200)
	cursor, err := models.FileModel.FindRaw(ctx, filter, opts)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var file models.File
		if err := cursor.Decode(&file); err != nil {
			continue
		}

		reason := transferBlockReason(ctx, &file)
		if reason != "" {
			continue
		}

		process, err := claimFile(ctx, &file)
		if err != nil {
			log.Printf("⚠️  [%s] Claim failed: %v", file.Slug, err)
			continue
		}
		if file.Status == models.FileStatusReady {
			log.Printf("🖼️  [%s] Claimed for pending asset install (skip resolutions that already have media)", file.Slug)
		}
		return process, &file, nil
	}
	return nil, nil, nil
}

// transferBlockReason returns why a file cannot be claimed (empty = ok).
func transferBlockReason(ctx context.Context, file *models.File) string {
	if !needsTransfer(ctx, file.ID) {
		return "no_pending_ingest"
	}

	if file.Status == models.FileStatusReady {
		if !needsPendingAssetInstall(ctx, file.ID) {
			return "ready_all_assets_have_media"
		}
	}

	ingest, err := models.IngestModel.FindOne(ctx, bson.M{
		"fileId":     file.ID,
		"sourceType": models.IngestSourceTypeProcessed,
		"deletedAt":  bson.M{"$exists": false},
	})
	if err != nil {
		return "ingest_not_found"
	}
	if ingest.StorageID == nil || *ingest.StorageID == "" {
		return "ingest_no_storage"
	}
	s3Storage, err := models.StorageModel.FindByID(ctx, *ingest.StorageID)
	if err != nil || s3Storage.Type != models.StorageTypeS3 || !s3Storage.IsOnline() {
		return "s3_offline"
	}

	activeCount, _ := models.VideoProcessModel.CountDocuments(ctx, bson.M{
		"fileId":      file.ID,
		"processType": models.ProcessTypeTransfer,
		"status": bson.M{"$in": []string{
			models.ProcessStatusProcessing,
			models.ProcessStatusFailed,
		}},
	})
	if activeCount > 0 {
		return "active_transfer_process"
	}

	otherActive, _ := models.VideoProcessModel.CountDocuments(ctx, bson.M{
		"fileId":      file.ID,
		"processType": models.ProcessTypeDownload,
		"status":      models.ProcessStatusProcessing,
	})
	if otherActive > 0 {
		return "download_processing"
	}
	return ""
}

// logIdleDiagnostics explains why no job was claimed (logged periodically).
func logIdleDiagnostics(ctx context.Context) {
	if reason := localStorageBlockReason(ctx); reason != "" {
		log.Printf("💤 Idle — local storage blocked: %s (storageId=%s)", reason, config.AppConfig.StorageId)
		return
	}

	readyOrig, _ := models.FileModel.CountDocuments(ctx, bson.M{
		"status": models.FileStatusReadyOriginal, "type": models.FileTypeVideo,
		"clonedFrom": bson.M{"$exists": false},
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	})
	readySprite, _ := models.FileModel.CountDocuments(ctx, bson.M{
		"status": models.FileStatusReady, "type": models.FileTypeVideo,
		"clonedFrom": bson.M{"$exists": false},
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	})

	var eligible, blocked int
	cursor, err := models.FileModel.FindRaw(ctx, bson.M{
		"status": bson.M{"$in": []string{models.FileStatusReadyOriginal, models.FileStatusReady}},
		"type": models.FileTypeVideo,
		"clonedFrom": bson.M{"$exists": false},
		"metadata.trashedAt": bson.M{"$exists": false},
		"metadata.deletedAt": bson.M{"$exists": false},
	}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(200))
	if err == nil {
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var file models.File
			if cursor.Decode(&file) != nil {
				continue
			}
			reason := transferBlockReason(ctx, &file)
			if reason == "" {
				eligible++
				log.Printf("✅ [%s] eligible for transfer (%s)", file.Slug, file.Status)
			} else if reason != "no_pending_ingest" && reason != "ready_all_assets_have_media" {
				blocked++
				log.Printf("🔒 [%s] blocked: %s", file.Slug, reason)
			}
		}
	}

	log.Printf("💤 Idle — ready_original: %d | ready (sprite): %d | eligible: %d | blocked: %d",
		readyOrig, readySprite, eligible, blocked)
}

func claimFile(ctx context.Context, file *models.File) (*models.VideoProcess, error) {
	now := time.Now()
	processing := models.ProcessStatusProcessing
	pending := models.StepStatusPending

	process := &models.VideoProcess{
		ID:          newUUID(),
		FileID:      &file.ID,
		SpaceID:     file.SpaceID,
		Slug:        &file.Slug,
		WorkerID:    &workerID,
		Status:      &processing,
		ProcessType: models.ProcessTypeTransfer,
		Timeline: bson.M{
			"download": bson.M{"status": pending},
			"extract":  bson.M{"status": pending},
			"install":  bson.M{"status": pending},
			"media":    bson.M{"status": pending},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := models.VideoProcessModel.Create(ctx, process); err != nil {
		return nil, err
	}
	log.Printf("🆕 [%s] Claimed for transfer (fileId=%s)", file.Slug, file.ID)
	return process, nil
}

func runTransfer(ctx context.Context, process *models.VideoProcess) error {
	fileID := derefStr(process.FileID)
	slug := derefStr(process.Slug)
	storagePath := config.AppConfig.StoragePath
	storageID := config.AppConfig.StorageId

	if storagePath == "" || storageID == "" {
		failProcess(ctx, process.ID, slug, "STORAGE_ID or STORAGE_PATH not configured")
		return fmt.Errorf("storage not configured")
	}
	if reason := localStorageBlockReason(ctx); reason != "" {
		failProcess(ctx, process.ID, slug, reason)
		return fmt.Errorf("%s", reason)
	}

	procLogger := utils.NewProcessLogger(slug)
	defer procLogger.Close()

	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)
	if strings.Contains(exePath, "go-build") {
		baseDir, _ = os.Getwd()
	}
	workDir := filepath.Join(baseDir, "transfer", slug)
	os.MkdirAll(workDir, 0755)

	var success bool
	defer func() {
		if success {
			os.RemoveAll(workDir)
			log.Printf("🧹 [%s] Cleaned up temp dir", slug)
		} else {
			log.Printf("⚠️  [%s] Keeping temp dir for retry: %s", slug, workDir)
		}
	}()

	utils.LogMain("📦 [%s] START TRANSFER (S3 → local storage)", slug)

	file, err := models.FileModel.FindByID(ctx, fileID)
	if err != nil {
		models.VideoProcessModel.DeleteByID(ctx, process.ID)
		return fmt.Errorf("file not found: %w", err)
	}

	ingest, err := models.IngestModel.FindOne(ctx, bson.M{
		"fileId":     fileID,
		"sourceType": models.IngestSourceTypeProcessed,
		"deletedAt":  bson.M{"$exists": false},
	})
	if err != nil {
		failProcess(ctx, process.ID, slug, "ingest not found")
		return err
	}

	s3Storage, err := models.StorageModel.FindByID(ctx, derefStr(ingest.StorageID))
	if err != nil {
		failProcess(ctx, process.ID, slug, "S3 storage not found")
		return err
	}

	duration := fileDuration(file)

	// ─── STEP 1: DOWNLOAD from S3 ingests ───────────────────
	startStep(ctx, process.ID, "download")
	totalAssets := float64(len(allResolutions) + 1)
	downloadedRes := make([]string, 0, len(allResolutions))
	transferredAssets := make([]string, 0, len(allResolutions)+1)

	for i, res := range allResolutions {
		if isCancelled(ctx, process.ID) {
			return nil
		}
		fileName := models.ResolutionToFileName[res]

		if hasVideoMedia(ctx, fileID, res) {
			log.Printf("⏭️  [%s] %s media already exists — skip download", slug, res)
			if hasPendingIngestForFileName(ctx, fileID, fileName) {
				transferredAssets = append(transferredAssets, fileName)
			}
			continue
		}

		key := s3ObjectKey(fileID, fileName)
		exists, err := downloader.ObjectExists(s3Storage, key)
		if err != nil {
			log.Printf("⚠️  [%s] HeadObject %s: %v", slug, key, err)
			continue
		}
		if !exists {
			continue
		}
		dest := filepath.Join(workDir, fileName)
		log.Printf("📥 [%s] Downloading %s...", slug, fileName)
		if err := downloader.DownloadFromS3(s3Storage, key, dest, func(done, total int64) {
			if total > 0 {
				assetPct := float64(done) / float64(total)
				updateOverallPercent(ctx, process.ID, (float64(i)+assetPct)/totalAssets*50)
			}
		}); err != nil {
			failProcess(ctx, process.ID, slug, fmt.Sprintf("download %s: %v", fileName, err))
			return err
		}
		downloadedRes = append(downloadedRes, res)
		transferredAssets = append(transferredAssets, fileName)
	}

	spriteZipPath := filepath.Join(workDir, models.SpriteZipName)
	hasSpriteZip := false
	if hasThumbnailMedia(ctx, fileID) {
		log.Printf("⏭️  [%s] thumbnail media already exists — skip sprite.zip", slug)
		if hasPendingSpriteIngest(ctx, fileID) {
			transferredAssets = append(transferredAssets, models.SpriteZipName)
		}
	} else {
		spriteKey := s3ObjectKey(fileID, models.SpriteZipName)
		if exists, _ := downloader.ObjectExists(s3Storage, spriteKey); exists {
			log.Printf("📥 [%s] Downloading %s...", slug, models.SpriteZipName)
			if err := downloader.DownloadFromS3(s3Storage, spriteKey, spriteZipPath, nil); err != nil {
				failProcess(ctx, process.ID, slug, fmt.Sprintf("download sprite.zip: %v", err))
				return err
			}
			hasSpriteZip = true
			transferredAssets = append(transferredAssets, models.SpriteZipName)
		}
	}

	if len(downloadedRes) == 0 && !hasSpriteZip {
		for _, res := range allResolutions {
			fileName := models.ResolutionToFileName[res]
			if hasVideoMedia(ctx, fileID, res) && hasPendingIngestForFileName(ctx, fileID, fileName) {
				transferredAssets = append(transferredAssets, fileName)
			}
		}
		if hasThumbnailMedia(ctx, fileID) && hasPendingSpriteIngest(ctx, fileID) {
			transferredAssets = append(transferredAssets, models.SpriteZipName)
		}
		if len(transferredAssets) == 0 {
			failProcess(ctx, process.ID, slug, "nothing to transfer on S3")
			return fmt.Errorf("nothing to transfer")
		}
		log.Printf("⏭️  [%s] All assets already have media — cleaning up stale ingests", slug)
	}

	if !hasVideoMedia(ctx, fileID, models.ResolutionOriginal) {
		originalPath := filepath.Join(workDir, models.FileNameOriginal)
		_, originalOnDisk := os.Stat(originalPath)
		hasNonOriginalDownload := false
		for _, res := range downloadedRes {
			if res != models.ResolutionOriginal {
				hasNonOriginalDownload = true
				break
			}
		}
		if originalOnDisk != nil {
			switch {
			case hasSpriteZip && file.Status == models.FileStatusReady:
				log.Printf("🖼️  [%s] Sprite-only transfer — skipping original video re-download", slug)
			case hasNonOriginalDownload:
				log.Printf("⏭️  [%s] Partial transfer — original not ready yet, installing %v", slug, downloadedRes)
			case len(downloadedRes) == 0 && len(transferredAssets) > 0:
				log.Printf("⏭️  [%s] Ingest cleanup only — skipping original requirement", slug)
			default:
				failProcess(ctx, process.ID, slug, "file_original.mp4 not found on S3 ingest")
				return fmt.Errorf("original missing on S3")
			}
		}
	}

	softDeleteIngests(ctx, fileID, transferredAssets, slug)

	completeStep(ctx, process.ID, "download")
	updateOverallPercent(ctx, process.ID, 50)
	log.Printf("✅ [%s] Downloaded %d video(s) from S3", slug, len(downloadedRes))

	// ─── STEP 2: EXTRACT sprite.zip ──────────────────────────
	startStep(ctx, process.ID, "extract")
	spriteDir := filepath.Join(workDir, "sprite")
	hasSprite := false
	if hasSpriteZip {
		log.Printf("📦 [%s] Extracting sprite.zip...", slug)
		if err := archive.Unzip(spriteZipPath, spriteDir); err != nil {
			failProcess(ctx, process.ID, slug, fmt.Sprintf("extract sprite.zip: %v", err))
			return err
		}
		hasSprite = true
	} else if !hasSprite {
		log.Printf("⚠️  [%s] No sprite.zip on S3 — skipping sprite", slug)
	}
	completeStep(ctx, process.ID, "extract")
	updateOverallPercent(ctx, process.ID, 60)

	// ─── STEP 3: INSTALL to local storage path ────────────────
	startStep(ctx, process.ID, "install")
	highestResolution := 0
	installedRes := make([]string, 0, len(downloadedRes))

	for _, res := range downloadedRes {
		fileName := models.ResolutionToFileName[res]
		src := filepath.Join(workDir, fileName)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := install.File(storagePath, fileID, fileName, src); err != nil {
			failProcess(ctx, process.ID, slug, fmt.Sprintf("install %s: %v", fileName, err))
			return err
		}
		installedRes = append(installedRes, res)
		if res != models.ResolutionOriginal {
			if resInt, err := strconv.Atoi(res); err == nil && resInt > highestResolution {
				highestResolution = resInt
			}
		}
		log.Printf("📂 [%s] Installed %s → %s/%s/", slug, fileName, storagePath, fileID)
	}

	if hasSpriteZip {
		hasSprite = true
		if err := install.Dir(storagePath, fileID, "sprite", spriteDir); err != nil {
			failProcess(ctx, process.ID, slug, fmt.Sprintf("install sprite: %v", err))
			return err
		}
		log.Printf("📂 [%s] Installed sprite/ → %s/%s/sprite/", slug, storagePath, fileID)
	}

	completeStep(ctx, process.ID, "install")
	updateOverallPercent(ctx, process.ID, 80)

	// ─── STEP 4: CREATE MEDIA RECORDS ─────────────────────────
	startStep(ctx, process.ID, "media")
	now := time.Now()
	mimeType := "video/mp4"
	storageIDPtr := storageID

	for _, res := range installedRes {
		if hasVideoMedia(ctx, fileID, res) {
			continue
		}
		fileName := models.ResolutionToFileName[res]
		destPath := filepath.Join(storagePath, fileID, fileName)
		fn := fileName
		resPtr := res
		fileSize := fileSize(destPath)
		media := models.Media{
			ID: newUUID(), Type: models.MediaTypeVideo, FileName: &fn, MimeType: &mimeType,
			Resolution: &resPtr, StorageID: &storageIDPtr, Slug: utils.RandomString(11, false),
			FileID: &fileID,
			Metadata: &models.MediaMetadata{
				Size: fileSize, Duration: duration,
			},
			CreatedAt: now, UpdatedAt: now,
		}
		models.MediaModel.Create(ctx, &media)
		cloneMediaToClonedFiles(ctx, fileID, media, slug)
		log.Printf("✅ [%s] Media record: %s", slug, res)
		if isPurgeResolution(res) {
			purgePlaylistCache(ctx, slug, fileID)
		}
	}

	if hasSprite {
		if !hasThumbnailMedia(ctx, fileID) {
			var totalSpriteSize int64
			spriteDest := filepath.Join(storagePath, fileID, "sprite")
			if entries, err := os.ReadDir(spriteDest); err == nil {
				for _, e := range entries {
					if !e.IsDir() {
						if info, err := e.Info(); err == nil {
							totalSpriteSize += info.Size()
						}
					}
				}
			}
			thumbFn := models.SpriteVTTName
			thumbMedia := models.Media{
				ID: newUUID(), Type: models.MediaTypeThumbnail, FileName: &thumbFn,
				StorageID: &storageIDPtr, Slug: utils.RandomString(11, false), FileID: &fileID,
				Metadata:  &models.MediaMetadata{Size: totalSpriteSize, Duration: duration},
				CreatedAt: now, UpdatedAt: now,
			}
			models.MediaModel.Create(ctx, &thumbMedia)
			cloneMediaToClonedFiles(ctx, fileID, thumbMedia, slug)
			log.Printf("✅ [%s] Media record: thumbnail", slug)
		}
	}

	completeStep(ctx, process.ID, "media")

	if highestResolution == 0 && file.Metadata != nil && file.Metadata.Highest != nil {
		highestResolution = *file.Metadata.Highest
	}
	if highestResolution == 0 {
		highestResolution = highestResolutionFromMedias(ctx, fileID)
	}

	// Mark ready when original exists on any storage (this server or another)
	if hasVideoMedia(ctx, fileID, models.ResolutionOriginal) {
		updateFields := bson.M{"status": models.FileStatusReady, "updatedAt": now}
		if highestResolution > 0 {
			updateFields["metadata.highest"] = highestResolution
		}
		if duration > 0 {
			updateFields["metadata.duration"] = int64(duration)
		}
		models.FileModel.UpdateByID(ctx, fileID, bson.M{"$set": updateFields})
		updateClonedFilesReady(ctx, fileID, highestResolution, slug)
	}

	updateOverallPercent(ctx, process.ID, 100)
	success = true
	models.VideoProcessModel.DeleteByID(ctx, process.ID)

	if hasSpriteZip && file.Status == models.FileStatusReady {
		utils.LogMain("✅ [%s] SPRITE HANDOFF COMPLETE", slug)
	} else {
		utils.LogMain("✅ [%s] TRANSFER COMPLETE → ready", slug)
	}
	return nil
}

func fileDuration(file *models.File) float64 {
	if file.Metadata != nil && file.Metadata.Duration != nil {
		return *file.Metadata.Duration
	}
	return 0
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func highestResolutionFromMedias(ctx context.Context, fileID string) int {
	highest := 0
	for _, res := range []string{models.Resolution1080, models.Resolution720, models.Resolution480, models.Resolution360} {
		if hasVideoMedia(ctx, fileID, res) {
			if n, err := strconv.Atoi(res); err == nil && n > highest {
				highest = n
			}
		}
	}
	return highest
}

func resumeOwnProcess(ctx context.Context) *models.VideoProcess {
	process, err := models.VideoProcessModel.FindOne(ctx, bson.M{
		"workerId": workerID, "status": models.ProcessStatusProcessing, "processType": models.ProcessTypeTransfer,
	})
	if err == nil {
		log.Printf("🔄 [%s] Resuming interrupted transfer", derefStr(process.Slug))
		return process
	}

	failed, err := models.VideoProcessModel.FindOne(ctx, bson.M{
		"workerId": workerID, "status": models.ProcessStatusFailed,
		"processType": models.ProcessTypeTransfer, "retryCount": bson.M{"$lt": 3},
	})
	if err == nil {
		slug := derefStr(failed.Slug)
		retryNum := 0
		if failed.RetryCount != nil {
			retryNum = *failed.RetryCount
		}
		waitSec := 30
		if retryNum >= 2 {
			waitSec = 60
		}
		log.Printf("🔁 [%s] Retrying transfer (attempt %d/3) — waiting %ds...", slug, retryNum+1, waitSec)
		time.Sleep(time.Duration(waitSec) * time.Second)
		models.VideoProcessModel.Col().UpdateOne(ctx, bson.M{"_id": failed.ID}, bson.M{"$set": bson.M{
			"status": models.ProcessStatusProcessing, "error": "", "updatedAt": time.Now(),
		}})
		status := models.ProcessStatusProcessing
		failed.Status = &status
		return failed
	}
	return nil
}

func processNextJob(ctx context.Context) bool {
	if process := resumeOwnProcess(ctx); process != nil {
		if err := runTransfer(ctx, process); err != nil {
			log.Printf("❌ Resume failed: %v", err)
		}
		return true
	}

	process, file, err := findAndClaimFile(ctx)
	if err == nil && process != nil {
		log.Printf("📦 New transfer job: [%s] %s", file.Slug, file.Name)
		if err := runTransfer(ctx, process); err != nil {
			log.Printf("❌ Failed: %s - %v", file.Slug, err)
		}
		return true
	}
	return false
}
