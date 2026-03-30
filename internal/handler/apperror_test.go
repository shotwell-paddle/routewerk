package handler

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	err := ErrNotFound("route not found")
	if err.Error() != "route not found" {
		t.Errorf("Error() = %q", err.Error())
	}

	errWithCause := ErrInternal("db failed", fmt.Errorf("connection refused"))
	if got := errWithCause.Error(); got != "db failed: connection refused" {
		t.Errorf("Error() with cause = %q", got)
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("original error")
	err := ErrBadRequest("invalid input", cause)
	if !errors.Is(err, cause) {
		t.Error("Unwrap should expose the cause")
	}
}

func TestAppError_Constructors(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		code int
	}{
		{"bad request", ErrBadRequest("bad", nil), http.StatusBadRequest},
		{"unauthorized", ErrUnauthorized("no auth"), http.StatusUnauthorized},
		{"forbidden", ErrForbidden("no access"), http.StatusForbidden},
		{"not found", ErrNotFound("gone"), http.StatusNotFound},
		{"conflict", ErrConflict("duplicate"), http.StatusConflict},
		{"internal", ErrInternal("boom", nil), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Code != tc.code {
				t.Errorf("Code = %d, want %d", tc.err.Code, tc.code)
			}
		})
	}
}

func TestIsAppError(t *testing.T) {
	ae := ErrNotFound("not found")
	got, ok := IsAppError(ae)
	if !ok || got.Code != http.StatusNotFound {
		t.Error("IsAppError should return the AppError")
	}

	_, ok = IsAppError(fmt.Errorf("plain error"))
	if ok {
		t.Error("IsAppError should return false for plain errors")
	}

	// Wrapped
	wrapped := fmt.Errorf("wrapping: %w", ae)
	got, ok = IsAppError(wrapped)
	if !ok || got.Code != http.StatusNotFound {
		t.Error("IsAppError should unwrap to find AppError")
	}
}
