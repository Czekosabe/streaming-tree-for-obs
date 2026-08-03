package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// writeValidationError renders a 422 with per-field details.
//
// Only the first violation per field is reported: forms show one message per
// input, and sending more would be noise the client discards anyway.
func writeValidationError(w http.ResponseWriter, logger *slog.Logger, verr *platform.ValidationError) {
	fields := make(map[string]string, len(verr.Violations))
	details := make(map[string]Detail, len(verr.Violations))

	for _, violation := range verr.Violations {
		if _, seen := fields[violation.Field]; seen {
			continue
		}
		fields[violation.Field] = violation.Message
		details[violation.Field] = Detail{Rule: violation.Rule, Params: violation.Params}
	}

	writeJSON(w, logger, http.StatusUnprocessableEntity, ErrorBody{
		Error:   "validation_failed",
		Message: "Validation failed",
		Fields:  fields,
		Details: details,
	})
}

// writeDomainError maps a domain error onto the HTTP contract.
//
// Storage failures are logged with their cause and answered with a generic
// message, so no SQLite text ever reaches a client.
func writeDomainError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if verr, ok := platform.AsValidationError(err); ok {
		writeValidationError(w, logger, verr)
		return
	}

	switch {
	case errors.Is(err, platform.ErrNotFound):
		writeError(w, logger, http.StatusNotFound,
			"not_found", "The requested resource does not exist.")

	case errors.Is(err, platform.ErrUnknownProvider):
		writeError(w, logger, http.StatusUnprocessableEntity,
			"unknown_provider", "The requested provider is not supported.")

	case errors.Is(err, platform.ErrConflict):
		writeError(w, logger, http.StatusConflict,
			"conflict", "The request conflicts with the current state of the resource.")

	default:
		logger.Error("unhandled domain error",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Any("error", err),
		)
		writeError(w, logger, http.StatusInternalServerError,
			"internal_error", "The server encountered an unexpected error.")
	}
}

// writeDecodeError renders a request-body failure.
func writeDecodeError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var derr *decodeError
	if errors.As(err, &derr) {
		writeError(w, logger, derr.status, derr.code, derr.message)
		return
	}

	writeError(w, logger, http.StatusBadRequest,
		"malformed_json", "Request body could not be read as JSON.")
}
