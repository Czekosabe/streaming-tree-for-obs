package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/onboarding"
)

// OnboardingService is the subset of onboarding.Service the HTTP layer
// needs.
type OnboardingService interface {
	State(ctx context.Context) (onboarding.State, error)
	SetStatus(ctx context.Context, status onboarding.Status) (onboarding.State, error)
}

// onboardingSchemaResponseVersion is the current shape of GET/PUT
// /api/onboarding - the same versioning convention GET /api/runtime
// already uses, so the frontend can reject a payload it does not
// understand instead of rendering half a screen.
const onboardingSchemaResponseVersion = 1

// onboardingStateResponse is the public onboarding-state contract.
type onboardingStateResponse struct {
	Version       int    `json:"version"`
	Status        string `json:"status"`
	SchemaVersion int    `json:"schemaVersion"`
}

func toOnboardingStateResponse(st onboarding.State) onboardingStateResponse {
	return onboardingStateResponse{
		Version:       onboardingSchemaResponseVersion,
		Status:        string(st.Status),
		SchemaVersion: st.SchemaVersion,
	}
}

// putOnboardingStatusRequest is the one accepted body shape for PUT
// /api/onboarding.
type putOnboardingStatusRequest struct {
	Status string `json:"status"`
}

// registerOnboardingRoutes wires the Stage 21 first-run onboarding-state
// API (docs/onboarding.md §4.4).
func registerOnboardingRoutes(mux *http.ServeMux, logger *slog.Logger, service OnboardingService) {
	mux.HandleFunc("GET /api/onboarding", handleGetOnboardingState(logger, service))
	mux.HandleFunc("PUT /api/onboarding", handlePutOnboardingStatus(logger, service))
	mux.HandleFunc("/api/onboarding", methodNotAllowed(logger, http.MethodGet, http.MethodPut))
}

func handleGetOnboardingState(logger *slog.Logger, service OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := service.State(r.Context())
		if err != nil {
			writeOnboardingError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOnboardingStateResponse(st))
	}
}

func handlePutOnboardingStatus(logger *slog.Logger, service OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body putOnboardingStatusRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		st, err := service.SetStatus(r.Context(), onboarding.Status(body.Status))
		if err != nil {
			writeOnboardingError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOnboardingStateResponse(st))
	}
}

// writeOnboardingError maps a service failure onto the HTTP contract.
func writeOnboardingError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if errors.Is(err, onboarding.ErrInvalidStatus) {
		writeError(w, logger, http.StatusBadRequest, "invalid_status",
			"status must be one of: pending, completed, dismissed.")
		return
	}

	logger.Error("unhandled onboarding error",
		slog.String("path", r.URL.Path), slog.Any("error", err))
	writeError(w, logger, http.StatusInternalServerError, "internal_error",
		"The server encountered an unexpected error.")
}
