package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
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

// Upload stores a file in the bucket and returns both the stable storage key
// and the public URL. Callers should persist BOTH: the URL for rendering and
// the key for future deletion. Deleting by URL was brittle because any change
// to StorageEndpoint (CDN swap, bucket move) would desync parsing and leak
// orphaned objects silently (S3 DeleteObject returns 204 even for missing
// keys).
//
// The key is built from: photos/{routeID}/{timestamp}{ext}
func (s *StorageService) Upload(ctx context.Context, routeID, filename, contentType string, body io.Reader) (key, publicURL string, err error) {
	// Sanitize the filename — keep only the extension
	ext := path.Ext(filename)
	if ext == "" {
		ext = ".jpg" // fallback
	}
	ext = strings.ToLower(ext)

	key = fmt.Sprintf("photos/%s/%d%s", routeID, time.Now().UnixMilli(), ext)

	// ACL=public-read so the rendered URL works for anonymous <img> requests
	// without presigning. Bucket default ACL varies between prod and dev
	// (Tigris buckets are private by default unless a bucket policy is set),
	// so setting it per-object makes the upload path self-sufficient.
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
		ACL:         s3types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return "", "", fmt.Errorf("upload to s3: %w", err)
	}

	publicURL, err = s.publicURL(key)
	if err != nil {
		return "", "", err
	}
	return key, publicURL, nil
}

// publicURL builds the virtual-hosted-style public URL for a key:
//
//	https://<bucket>.<endpoint-host>/<key>
//
// We deliberately do NOT reuse the configured endpoint's host verbatim in
// path-style (https://<endpoint-host>/<bucket>/<key>) because Tigris only
// serves anonymous <img> GETs on virtual-hosted hostnames — path-style
// requests to the same object return 403 AccessDenied even for buckets
// configured as public. Virtual-hosted style works on both Tigris and
// AWS S3, so this is the portable choice.
//
// If the configured endpoint is unparseable (e.g. an IP with no host, or
// a bare string), we return an error rather than fabricating a URL — the
// caller's PutObject has already succeeded by this point, so surfacing
// the config problem loudly is better than persisting a broken URL.
func (s *StorageService) publicURL(key string) (string, error) {
	ep, err := url.Parse(strings.TrimRight(s.endpoint, "/"))
	if err != nil || ep.Scheme == "" || ep.Host == "" {
		return "", fmt.Errorf("storage endpoint %q is not a usable URL", s.endpoint)
	}
	return fmt.Sprintf("%s://%s.%s/%s", ep.Scheme, s.bucket, ep.Host, key), nil
}

// Delete removes an object from the bucket by its storage key.
//
// Callers that only have a URL on hand (legacy rows where storage_key is NULL)
// should derive the key via KeyFromURL first. New code paths must store the
// key returned by Upload and pass it here directly.
func (s *StorageService) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("delete from s3: empty key")
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

// KeyFromURL derives a storage key from a public URL produced by Upload.
// Returns the empty string if the URL doesn't look like one of ours — callers
// should treat that as "don't attempt delete" rather than guessing.
//
// This is a transitional helper for legacy route_photos rows inserted before
// migration 28 (which added the storage_key column). Once those rows have
// aged out / been backfilled, this helper and its callers can go.
//
// Recognizes both URL shapes:
//   - virtual-hosted (current):  https://<bucket>.<host>/<key>
//   - path-style    (pre-31):    https://<host>/<bucket>/<key>
//
// Migration 31 rewrote all persisted path-style URLs to virtual-hosted, but
// we keep the path-style branch so a rollback (or a straggler written before
// the migration ran) still deletes correctly.
func (s *StorageService) KeyFromURL(photoURL string) string {
	// Path-style: "<endpoint>/<bucket>/<key>"
	pathPrefix := fmt.Sprintf("%s/%s/", strings.TrimRight(s.endpoint, "/"), s.bucket)
	if strings.HasPrefix(photoURL, pathPrefix) {
		return strings.TrimPrefix(photoURL, pathPrefix)
	}

	// Virtual-hosted: "<scheme>://<bucket>.<host>/<key>"
	ep, err := url.Parse(strings.TrimRight(s.endpoint, "/"))
	if err == nil && ep.Scheme != "" && ep.Host != "" {
		vhPrefix := fmt.Sprintf("%s://%s.%s/", ep.Scheme, s.bucket, ep.Host)
		if strings.HasPrefix(photoURL, vhPrefix) {
			return strings.TrimPrefix(photoURL, vhPrefix)
		}
	}
	return ""
}

// IsConfigured returns true if the storage backend is ready.
func (s *StorageService) IsConfigured() bool {
	return s != nil && s.client != nil
}

// Healthy checks whether the S3 bucket is reachable.
func (s *StorageService) Healthy(ctx context.Context) bool {
	if !s.IsConfigured() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	return err == nil
}
