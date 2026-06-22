package models

const (
	FileTypeVideo = "video"
)

const (
	FileStatusReady         = "ready"
	FileStatusReadyOriginal = "ready_original"
	FileStatusProcessing    = "processing"
	FileStatusError         = "error"
)

const (
	MediaTypeVideo     = "video"
	MediaTypeThumbnail = "thumbnail"
)

const (
	IngestSourceTypeProcessed = "processed"
)

const (
	StorageTypeS3    = "s3"
	StorageTypeLocal = "local"
)

const (
	StorageStatusOnline = "online"
)

const (
	ResolutionOriginal = "original"
	Resolution1080     = "1080"
	Resolution720      = "720"
	Resolution480      = "480"
	Resolution360      = "360"
)

const (
	FileNameOriginal = "file_original.mp4"
	FileName1080     = "file_1080.mp4"
	FileName720      = "file_720.mp4"
	FileName480      = "file_480.mp4"
	FileName360      = "file_360.mp4"
	SpriteZipName    = "sprite.zip"
	SpriteVTTName    = "sprite.vtt"
)

var ResolutionToFileName = map[string]string{
	ResolutionOriginal: FileNameOriginal,
	Resolution1080:     FileName1080,
	Resolution720:      FileName720,
	Resolution480:      FileName480,
	Resolution360:      FileName360,
}

var ResolutionToShortSide = map[string]int{
	Resolution1080: 1080,
	Resolution720:  720,
	Resolution480:  480,
	Resolution360:  360,
}

const (
	ProcessTypeDownload  = "download"
	ProcessTypeTransfer  = "transfer"
	ProcessTypeTranscode = "transcode"
	ProcessTypeSpritesheet = "spritesheet"
)

const (
	ProcessStatusProcessing = "processing"
	ProcessStatusFailed     = "failed"
	ProcessStatusCancelled  = "cancelled"
)

const (
	StepStatusPending    = "pending"
	StepStatusProcessing = "processing"
	StepStatusCompleted  = "completed"
)

const (
	SettingTransferEnabled = "transfer_enabled"
)
