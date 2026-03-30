package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/shotwell-paddle/routewerk/internal/config"
)

// StorageService handles file uploads to an S3-compatible object store.
type StorageService struct {
	client   *s3.Client
	bucket   string
	endpoint string // public-facing endpoint for URL construction
}

// NewStorageService creates a StorageService from the app config.
// Returns nil if storage is not configured (endpoint is empty).
func NewStorageService(cfg *config.Config) *StorageService {
	if cfg.StorageEndpoint == "" {
		slog.Warn("storage not configured — photo uploads disabled")
		return nil
	}

	client := s3.New(s3.Options{
		Region:       "us-east-1", // required but ignored by most S3-compat providers
		BaseEndpoint: aws.String(cfg.StorageEndpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.StorageAccessKey, cfg.StorageSecretKey, ""),
		UsePathStyle: true, // MinIO and most S3-compat providers need path-style
	})

	return &StorageService{
		client:   client,
		bucket:   cfg.StorageBucket,
		endpoint: cfg.StorageEndpoint,
	}
}

// Upload stores a file in the bucket and returns the public URL.
// The key is built from: photos/{routeID}/{timestamp}_{filename}
func (s *StorageService) Upload(ctx context.Context, routeID, filename, contentType string, body io.Reader) (string, error) {
	// Sanitize the filename — keep only the extension
	ext := path.Ext(filename)
	if ext == "" {
		ext = ".jpg" // fallback
	}
	ext = strings.ToLower(ext)

	key := fmt.Sprintf("photos/%s/%d%s", routeID, time.Now().UnixMilli(), ext)

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("upload to s3: %w", err)
	}

	// Construct the public URL
	url := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.endpoint, "/"), s.bucket, key)
	return url, nil
}

// Delete removes a file from the bucket by its full URL or key.
func (s *StorageService) Delete(ctx context.Context, photoURL string) error {
	// Extract the key from the URL
	key := photoURL
	prefix := fmt.Sprintf("%s/%s/", strings.TrimRight(s.endpoint, "/"), s.bucket)
	if strings.HasPrefix(photoURL, prefix) {
		key = strings.TrimPrefix(photoURL, prefix)
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete from s3: %w", err)
	}
	return nil
}

// IsConfigured returns true if the storage backend is ready.
func (s *StorageService) IsConfigured() bool {
	return s != nil && s.client != nil
}
