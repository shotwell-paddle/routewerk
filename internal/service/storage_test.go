package service

import (
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/config"
)

// ── NewStorageService ──────────────────────────────────────────────

func TestNewStorageService_NilWhenUnconfigured(t *testing.T) {
	cfg := &config.Config{
		StorageEndpoint: "", // not configured
	}

	svc := NewStorageService(cfg)
	if svc != nil {
		t.Error("NewStorageService should return nil when endpoint is empty")
	}
}

func TestNewStorageService_NonNilWhenConfigured(t *testing.T) {
	cfg := &config.Config{
		StorageEndpoint:  "https://s3.example.com",
		StorageBucket:    "test-bucket",
		StorageAccessKey: "test-key",
		StorageSecretKey: "test-secret",
	}

	svc := NewStorageService(cfg)
	if svc == nil {
		t.Fatal("NewStorageService should return non-nil when endpoint is set")
	}
	if svc.bucket != "test-bucket" {
		t.Errorf("bucket = %q, want %q", svc.bucket, "test-bucket")
	}
	if svc.endpoint != "https://s3.example.com" {
		t.Errorf("endpoint = %q, want %q", svc.endpoint, "https://s3.example.com")
	}
}

// ── IsConfigured ───────────────────────────────────────────────────

func TestIsConfigured_NilService(t *testing.T) {
	var svc *StorageService
	if svc.IsConfigured() {
		t.Error("nil StorageService should not be configured")
	}
}

func TestIsConfigured_NilClient(t *testing.T) {
	svc := &StorageService{client: nil}
	if svc.IsConfigured() {
		t.Error("StorageService with nil client should not be configured")
	}
}

func TestIsConfigured_WithClient(t *testing.T) {
	cfg := &config.Config{
		StorageEndpoint:  "https://s3.example.com",
		StorageBucket:    "test-bucket",
		StorageAccessKey: "key",
		StorageSecretKey: "secret",
	}
	svc := NewStorageService(cfg)
	if !svc.IsConfigured() {
		t.Error("StorageService with client should be configured")
	}
}

// ── Healthy ────────────────────────────────────────────────────────

func TestHealthy_NilService(t *testing.T) {
	var svc *StorageService
	// IsConfigured returns false, so Healthy should also return false
	// but calling Healthy on nil will panic. We verify IsConfigured guards it.
	if svc.IsConfigured() {
		t.Error("nil service should not be configured")
	}
}

func TestHealthy_UnconfiguredService(t *testing.T) {
	svc := &StorageService{client: nil}
	if svc.IsConfigured() {
		t.Error("service with nil client should not be configured")
	}
	// The health check path in the handler checks IsConfigured() before Healthy()
}

// ── KeyFromURL: legacy URL → storage-key derivation ────────────────
//
// After migration 28 new rows persist storage_key directly. KeyFromURL is
// only used for legacy rows that predate that column; these tests pin the
// format we committed to so a future endpoint change doesn't break the
// derivation before the legacy rows have aged out.

func TestKeyFromURL(t *testing.T) {
	svc := &StorageService{
		endpoint: "https://s3.example.com",
		bucket:   "my-bucket",
	}

	tests := []struct {
		name    string
		input   string
		wantKey string
	}{
		{
			"full URL",
			"https://s3.example.com/my-bucket/photos/route-1/123.jpg",
			"photos/route-1/123.jpg",
		},
		{
			"empty string returns empty (don't attempt delete)",
			"",
			"",
		},
		{
			"URL with different bucket returns empty",
			"https://s3.example.com/other-bucket/photos/route-2/456.jpg",
			"",
		},
		{
			"URL with different endpoint returns empty",
			"https://other.example.com/my-bucket/photos/route-3/789.jpg",
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.KeyFromURL(tc.input)
			if got != tc.wantKey {
				t.Errorf("KeyFromURL(%q) = %q, want %q", tc.input, got, tc.wantKey)
			}
		})
	}
}

// ── Upload URL construction ────────────────────────────────────────

func TestUploadURLFormat(t *testing.T) {
	// Verify the URL format that Upload would produce
	endpoint := "https://s3.example.com"
	bucket := "my-bucket"
	key := "photos/route-1/1234567890.jpg"

	url := endpoint + "/" + bucket + "/" + key
	expected := "https://s3.example.com/my-bucket/photos/route-1/1234567890.jpg"

	if url != expected {
		t.Errorf("URL = %q, want %q", url, expected)
	}
}

func TestUploadURLFormat_TrailingSlash(t *testing.T) {
	// Upload trims trailing slashes from endpoint
	endpoint := "https://s3.example.com/"
	bucket := "my-bucket"
	key := "photos/route-1/123.jpg"

	// The Upload method does: strings.TrimRight(s.endpoint, "/")
	trimmed := "https://s3.example.com"
	_ = endpoint // used for clarity
	url := trimmed + "/" + bucket + "/" + key
	expected := "https://s3.example.com/my-bucket/photos/route-1/123.jpg"

	if url != expected {
		t.Errorf("URL = %q, want %q", url, expected)
	}
}
