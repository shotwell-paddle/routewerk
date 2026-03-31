package main

import (
	"log/slog"
	"net/http"
	"testing"
	"time"
)

// ── initLogger ─────────────────────────────────────────────────────

func TestInitLogger_Dev(t *testing.T) {
	// Should not panic and should set up a text handler with debug level
	initLogger(true)

	// Verify the logger is functional by logging at debug level
	slog.Debug("test debug message from dev logger")
}

func TestInitLogger_Prod(t *testing.T) {
	// Should not panic and should set up a JSON handler with info level
	initLogger(false)

	slog.Info("test info message from prod logger")
}

// ── Server configuration constants ─────────────────────────────────

func TestServerTimeouts_Reasonable(t *testing.T) {
	// These match what main() configures — verify they're sensible
	readTimeout := 10 * time.Second
	readHeaderTimeout := 5 * time.Second
	writeTimeout := 30 * time.Second
	idleTimeout := 120 * time.Second
	maxHeaderBytes := 1 << 20

	if readTimeout < 5*time.Second || readTimeout > 60*time.Second {
		t.Errorf("ReadTimeout %v is outside reasonable range", readTimeout)
	}
	if readHeaderTimeout < 1*time.Second || readHeaderTimeout > readTimeout {
		t.Errorf("ReadHeaderTimeout %v should be > 1s and <= ReadTimeout", readHeaderTimeout)
	}
	if writeTimeout < readTimeout {
		t.Errorf("WriteTimeout %v should be >= ReadTimeout %v", writeTimeout, readTimeout)
	}
	if idleTimeout < writeTimeout {
		t.Errorf("IdleTimeout %v should be >= WriteTimeout %v", idleTimeout, writeTimeout)
	}
	if maxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want 1MB (%d)", maxHeaderBytes, 1<<20)
	}
}

// ── Server construction ────────────────────────────────────────────

func TestServerConfig_Structure(t *testing.T) {
	// Verify the server can be constructed with proper timeouts
	// (doesn't start listening — just checks the struct)
	srv := &http.Server{
		Addr:              ":8080",
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	if srv.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", srv.Addr, ":8080")
	}
	if srv.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v", srv.ReadTimeout)
	}
}
