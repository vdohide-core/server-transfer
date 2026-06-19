package downloader

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"server-transfer/internal/db/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func newS3Client(storage *models.Storage) (*s3.Client, string, error) {
	if storage.S3 == nil {
		return nil, "", fmt.Errorf("storage has no S3 config")
	}
	s3Cfg := storage.S3
	endpoint := strings.TrimRight(derefStr(s3Cfg.Endpoint), "/")
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint
	}
	if strings.HasSuffix(endpoint, "/"+s3Cfg.Bucket) {
		endpoint = endpoint[:len(endpoint)-len(s3Cfg.Bucket)-1]
	}
	region := s3Cfg.Region
	if region == "" {
		region = "auto"
	}
	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: &endpoint,
		Credentials: credentials.NewStaticCredentialsProvider(
			s3Cfg.AccessKeyID,
			s3Cfg.SecretAccessKey,
			"",
		),
		UsePathStyle: s3Cfg.ForcePathStyle,
	})
	return client, s3Cfg.Bucket, nil
}

func objectKey(storage *models.Storage, key string) string {
	if storage.S3.Prefix != "" && !strings.HasPrefix(key, storage.S3.Prefix) {
		return strings.TrimRight(storage.S3.Prefix, "/") + "/" + key
	}
	return key
}

func ObjectExists(storage *models.Storage, key string) (bool, error) {
	client, bucket, err := newS3Client(storage)
	if err != nil {
		return false, err
	}
	fullKey := objectKey(storage, key)
	_, err = client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		var nfe *types.NotFound
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		_ = nfe
		return false, err
	}
	return true, nil
}

func DownloadFromS3(storage *models.Storage, key, outputPath string, onProgress func(downloaded, total int64)) error {
	client, bucket, err := newS3Client(storage)
	if err != nil {
		return err
	}
	fullKey := objectKey(storage, key)
	log.Printf("📥 S3: bucket=%s key=%s", bucket, fullKey)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	result, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("GetObject: %w", err)
	}
	defer result.Body.Close()

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	var total int64
	if result.ContentLength != nil {
		total = *result.ContentLength
	}

	buf := make([]byte, 256*1024)
	var written int64
	for {
		n, readErr := result.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return wErr
			}
			written += int64(n)
			if onProgress != nil && total > 0 {
				onProgress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	log.Printf("✅ Downloaded %.2f MB from S3", float64(written)/1024/1024)
	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
