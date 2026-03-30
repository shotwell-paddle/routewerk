package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── JSON ────────────────────────────────────────────────────

func TestJSON_WithData(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusOK, map[string]string{"name": "test"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["name"] != "test" {
		t.Errorf("body[name] = %q, want %q", body["name"], "test")
	}
}

func TestJSON_NilData(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body should be empty, got %q", rec.Body.String())
	}
}

func TestJSON_StatusCreated(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusCreated, map[string]int{"id": 42})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// ── Error ───────────────────────────────────────────────────

func TestError_Response(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusBadRequest, "invalid input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "invalid input" {
		t.Errorf("error = %q, want %q", body["error"], "invalid input")
	}
}

func TestError_DifferentCodes(t *testing.T) {
	tests := []struct {
		code int
		msg  string
	}{
		{http.StatusNotFound, "not found"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusInternalServerError, "server error"},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		Error(rec, tc.code, tc.msg)
		if rec.Code != tc.code {
			t.Errorf("Error(%d, %q): status = %d", tc.code, tc.msg, rec.Code)
		}
	}
}

// ── Decode ──────────────────────────────────────────────────

func TestDecode_ValidJSON(t *testing.T) {
	body := `{"name":"test","value":42}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var target struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	if err := Decode(req, &target); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if target.Name != "test" || target.Value != 42 {
		t.Errorf("Decode got %+v", target)
	}
}

func TestDecode_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	if err := Decode(req, &struct{}{}); err == nil {
		t.Error("Decode should fail on empty body")
	}
}

func TestDecode_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{invalid}"))
	if err := Decode(req, &struct{}{}); err == nil {
		t.Error("Decode should fail on invalid JSON")
	}
}

func TestDecode_MultipleJSONValues(t *testing.T) {
	// Two JSON objects back-to-back: should be rejected (request smuggling prevention)
	body := `{"a":1}{"b":2}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var target map[string]int
	if err := Decode(req, &target); err == nil {
		t.Error("Decode should reject bodies with multiple JSON values")
	}
}

func TestDecode_OversizedBody(t *testing.T) {
	// Create a body larger than maxBodySize (1MB)
	big := bytes.Repeat([]byte("x"), maxBodySize+1)
	body := `{"data":"` + string(big) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var target map[string]string
	if err := Decode(req, &target); err == nil {
		t.Error("Decode should reject oversized bodies")
	}
}

// ── slugify ─────────────────────────────────────────────────

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  Leading Trailing  ", "leading-trailing"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special!@#$Chars", "special-chars"},
		{"already-slugged", "already-slugged"},
		{"UPPERCASE", "uppercase"},
		{"mix123numbers", "mix123numbers"},
		{"", ""},
		{"---dashes---", "dashes"},
		{"a", "a"},
	}
	for _, tc := range tests {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── maxBodySize ─────────────────────────────────────────────

func TestMaxBodySize_Value(t *testing.T) {
	if maxBodySize != 1<<20 {
		t.Errorf("maxBodySize = %d, want %d", maxBodySize, 1<<20)
	}
}
