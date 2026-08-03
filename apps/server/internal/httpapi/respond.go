package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorBody is the single error shape returned by every endpoint, so the
// frontend only has to understand one contract.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeJSON serialises v as JSON with the given status code.
//
// Encoding happens into a buffer-free stream after the header is written, so a
// late marshalling failure can no longer change the status code. It is logged
// instead - the alternative (a truncated body with a 200) would be worse.
func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("failed to write JSON response", slog.Any("error", err))
	}
}

// writeError returns a structured error payload.
func writeError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string) {
	writeJSON(w, logger, status, ErrorBody{Error: code, Message: message})
}
