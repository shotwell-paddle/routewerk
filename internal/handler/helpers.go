package handler

import (
	"encoding/json"
	"io"
	"net/http"
)

// maxBodySize limits request body to 1MB to prevent memory exhaustion.
const maxBodySize = 1 << 20 // 1 MB

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// Error writes a JSON error response.
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// Decode reads a JSON request body into the target with a size limit.
func Decode(r *http.Request, target interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodySize)
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(target)

	// Reject bodies with multiple JSON values (prevents request smuggling)
	if err == nil {
		if dec.More() {
			return io.ErrUnexpectedEOF
		}
	}

	return err
}
