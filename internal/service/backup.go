package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/shotwell-paddle/routewerk/internal/config"
)

// BackupService runs nightly logical Postgres backups from INSIDE the
// app: pg_dump against DATABASE_URL (the DB is on Fly's private network —
// no proxy), uploaded to the S3-compatible storage the app already has
// credentials for (Tigris, shared with photos under a separate prefix by
// default; point BACKUP_BUCKET elsewhere for a dedicated bucket).
//
// This replaces the pull-from-a-local-machine mode as the default: zero
// setup, zero new secrets, survives the local machine being off, and the
// last-success timestamp is surfaced on /health so silent failure is
// visible. Trade-off: backups share the app's storage credential, so this
// does not protect against that credential being compromised — the threat
// model is bad migrations, fat-fingered deletes, and Fly volume loss.
//
// Objects are uploaded PRIVATE (no public-read ACL — unlike photos).
type BackupService struct {
	client        *s3.Client
	bucket        string
	prefix        string
	dbURL         string
	retentionDays int
	hourUTC       int

	// lastSuccess is the unix time of the last successful run (0 = never).
	lastSuccess atomic.Int64
}

// NewBackupService returns nil when object storage isn't configured —
// callers treat nil as "backups disabled".
func NewBackupService(cfg *config.Config) *BackupService {
	if cfg.StorageEndpoint == "" || cfg.StorageAccessKey == "" {
		return nil
	}
	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(cfg.StorageEndpoint),
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.StorageAccessKey, cfg.StorageSecretKey, "",
		),
	})
	bucket := cfg.BackupBucket
	if bucket == "" {
		bucket = cfg.StorageBucket
	}
	return &BackupService{
		client:        client,
		bucket:        bucket,
		prefix:        cfg.BackupPrefix,
		dbURL:         cfg.DatabaseURL,
		retentionDays: cfg.BackupRetentionDays,
		hourUTC:       cfg.BackupHourUTC,
	}
}

// LastSuccess reports when the most recent backup succeeded (ok=false if
// none exists). Seeded from the bucket at scheduler start so the signal
// survives restarts; live runs then keep it current. Surfaced on /health.
func (b *BackupService) LastSuccess() (time.Time, bool) {
	ts := b.lastSuccess.Load()
	if ts == 0 {
		return time.Time{}, false
	}
	return time.Unix(ts, 0).UTC(), true
}

// backupKey names a dump object for a given day. Deterministic per day:
// a re-run the same day overwrites rather than duplicates.
func backupKey(prefix string, t time.Time) string {
	return prefix + "routewerk-" + t.UTC().Format("2006-01-02") + ".dump"
}

// parseBackupDate extracts the YYYY-MM-DD date from a key written by
// backupKey. ok=false for anything that doesn't match the naming scheme —
// we never touch objects we didn't write.
func parseBackupDate(key, prefix string) (string, bool) {
	name := strings.TrimPrefix(key, prefix)
	if !strings.HasPrefix(name, "routewerk-") || !strings.HasSuffix(name, ".dump") {
		return "", false
	}
	date := strings.TrimSuffix(strings.TrimPrefix(name, "routewerk-"), ".dump")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", false
	}
	return date, true
}

// keysToPrune returns the object keys whose embedded date is older than
// retentionDays before now.
func keysToPrune(keys []string, prefix string, now time.Time, retentionDays int) []string {
	cutoff := now.UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	var prune []string
	for _, k := range keys {
		date, ok := parseBackupDate(k, prefix)
		if !ok {
			continue
		}
		if date < cutoff {
			prune = append(prune, k)
		}
	}
	sort.Strings(prune)
	return prune
}

// nextRunAt returns the next occurrence of hourUTC:00 strictly after now.
func nextRunAt(now time.Time, hourUTC int) time.Time {
	next := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), hourUTC, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// BackupObject describes one stored dump.
type BackupObject struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// List returns the stored dumps under the backup prefix, newest first
// (the key embeds the date, so reverse-lexicographic order is
// chronological). Non-backup objects under the prefix are ignored.
func (b *BackupService) List(ctx context.Context) ([]BackupObject, error) {
	out, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(b.prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var objs []BackupObject
	for _, obj := range out.Contents {
		if obj.Key == nil {
			continue
		}
		if _, ok := parseBackupDate(*obj.Key, b.prefix); !ok {
			continue
		}
		o := BackupObject{Key: *obj.Key}
		if obj.Size != nil {
			o.Size = *obj.Size
		}
		if obj.LastModified != nil {
			o.LastModified = obj.LastModified.UTC()
		}
		objs = append(objs, o)
	}
	sort.Slice(objs, func(i, j int) bool { return objs[i].Key > objs[j].Key })
	return objs, nil
}

// seedLastSuccess raises lastSuccess to t if t is newer — used to restore
// the freshness signal from the bucket after a restart, without ever
// rolling back a success recorded by a live run.
func (b *BackupService) seedLastSuccess(t time.Time) {
	ts := t.Unix()
	for {
		cur := b.lastSuccess.Load()
		if cur >= ts {
			return
		}
		if b.lastSuccess.CompareAndSwap(cur, ts) {
			return
		}
	}
}

// RunOnce takes one backup: pg_dump -Fc → verify archive → upload
// (private) → prune beyond retention. Returns the object key and size.
func (b *BackupService) RunOnce(ctx context.Context) (string, int64, error) {
	tmpDir, err := os.MkdirTemp("", "rw-backup-*")
	if err != nil {
		return "", 0, fmt.Errorf("backup tmpdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	tmp := filepath.Join(tmpDir, "db.dump")

	dump := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "-Fc", "-d", b.dbURL, "-f", tmp)
	if out, err := dump.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// A truncated or corrupt archive fails to list.
	verify := exec.CommandContext(ctx, "pg_restore", "--list", tmp)
	verify.Stdout = nil
	if out, err := verify.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("pg_restore verify: %w: %s", err, firstLine(string(out)))
	}

	f, err := os.Open(tmp)
	if err != nil {
		return "", 0, fmt.Errorf("open dump: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat dump: %w", err)
	}

	key := backupKey(b.prefix, time.Now())
	// No ACL: private object, unlike the public-read photo uploads.
	if _, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentType:   aws.String("application/octet-stream"),
		ContentLength: aws.Int64(info.Size()),
	}); err != nil {
		return "", 0, fmt.Errorf("upload backup: %w", err)
	}

	b.lastSuccess.Store(time.Now().Unix())
	b.prune(ctx)
	return key, info.Size(), nil
}

// prune deletes dumps older than the retention window. Best-effort: a
// prune failure must never fail the backup that just succeeded.
func (b *BackupService) prune(ctx context.Context) {
	out, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(b.prefix),
	})
	if err != nil {
		slog.Warn("backup prune: list failed", "error", err)
		return
	}
	var keys []string
	for _, obj := range out.Contents {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}
	for _, k := range keysToPrune(keys, b.prefix, time.Now(), b.retentionDays) {
		if _, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(k),
		}); err != nil {
			slog.Warn("backup prune: delete failed", "key", k, "error", err)
			continue
		}
		slog.Info("backup pruned", "key", k)
	}
}

// StartScheduler runs RunOnce daily at the configured UTC hour until ctx
// is cancelled. runOnBoot additionally fires one immediately — set on
// staging so every deploy exercises a real end-to-end backup.
func (b *BackupService) StartScheduler(ctx context.Context, runOnBoot bool) {
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		start := time.Now()
		key, size, err := b.RunOnce(runCtx)
		if err != nil {
			slog.Error("database backup failed", "error", err)
			return
		}
		slog.Info("database backup complete", "key", key, "bytes", size, "took", time.Since(start).Round(time.Millisecond).String())
	}

	go func() {
		// Seed the freshness signal from the bucket: /health's last_backup
		// should answer "when did any backup last succeed", not "has this
		// process backed up yet" — otherwise every deploy resets it to
		// none until the next nightly. Best-effort; a failed list just
		// leaves the signal unseeded.
		if objs, err := b.List(ctx); err != nil {
			slog.Warn("backup: seeding last-success from bucket failed", "error", err)
		} else if len(objs) > 0 {
			b.seedLastSuccess(objs[0].LastModified)
		}
		if runOnBoot {
			run()
		}
		for {
			next := nextRunAt(time.Now(), b.hourUTC)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				run()
			}
		}
	}()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
