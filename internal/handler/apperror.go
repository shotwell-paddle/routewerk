package handler

import (
	"errors"
	"fmt"
	"net/http"
)

// AppError is a structured application error that carries an HTTP status code,
// a user-facing message, and an optional wrapped cause for logging.
// Handlers can return AppErrors; the recovery middleware will catch them and
// render the appropriate response.
type AppError struct {
	Code    int    // HTTP status code
	Message string // user-facing message (safe to expose)
	Err     error  // underlying error (logged, never exposed to user)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Common constructors — keep handler code clean.

func ErrBadRequest(msg string, cause error) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: msg, Err: cause}
}

func ErrUnauthorized(msg string) *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: msg}
}

func ErrForbidden(msg string) *AppError {
	return &AppError{Code: http.StatusForbidden, Message: msg}
}

func ErrNotFound(msg string) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: msg}
}

func ErrConflict(msg string) *AppError {
	return &AppError{Code: http.StatusConflict, Message: msg}
}

func ErrInternal(msg string, cause error) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: msg, Err: cause}
}

// IsAppError checks whether an error is an AppError and returns it.
func IsAppError(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
