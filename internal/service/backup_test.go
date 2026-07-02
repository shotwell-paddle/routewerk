package service

import (
	"testing"
	"time"
)

func TestBackupKey(t *testing.T) {
	ts := time.Date(2026, 7, 1, 23, 59, 0, 0, time.UTC)
	if got := backupKey("backups/", ts); got != "backups/routewerk-2026-07-01.dump" {
		t.Errorf("backupKey = %q", got)
	}
	// Local-time input must still name by UTC date.
	loc := time.FixedZone("CST", -6*3600)
	tsLocal := time.Date(2026, 7, 1, 20, 0, 0, 0, loc) // 2026-07-02 02:00 UTC
	if got := backupKey("backups/", tsLocal); got != "backups/routewerk-2026-07-02.dump" {
		t.Errorf("backupKey local = %q", got)
	}
}

func TestKeysToPrune(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	keys := []string{
		"backups/routewerk-2026-07-01.dump", // today — keep
		"backups/routewerk-2026-05-27.dump", // exactly 35d — keep (cutoff is strictly older)
		"backups/routewerk-2026-05-26.dump", // 36d — prune
		"backups/routewerk-2026-01-01.dump", // old — prune
		"backups/routewerk-garbage.dump",    // unparseable date — never touch
		"backups/other-file.txt",            // not ours — never touch
		"photos/route1/x.webp",              // different prefix survives TrimPrefix mismatch
	}
	got := keysToPrune(keys, "backups/", now, 35)
	want := []string{
		"backups/routewerk-2026-01-01.dump",
		"backups/routewerk-2026-05-26.dump",
	}
	if len(got) != len(want) {
		t.Fatalf("prune = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prune[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNextRunAt(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		hour int
		want time.Time
	}{
		{
			"before the hour → same day",
			time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC), 9,
			time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
		},
		{
			"exactly the hour → next day (strictly after)",
			time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), 9,
			time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
		},
		{
			"after the hour → next day",
			time.Date(2026, 7, 1, 22, 30, 0, 0, time.UTC), 9,
			time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
		},
		{
			"month boundary",
			time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), 9,
			time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRunAt(tt.now, tt.hour); !got.Equal(tt.want) {
				t.Errorf("nextRunAt = %v, want %v", got, tt.want)
			}
		})
	}
}
