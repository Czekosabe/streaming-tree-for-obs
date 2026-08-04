package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
)

// FFmpegRuntimeService reports the resolved FFmpeg dependency.
type FFmpegRuntimeService interface {
	FFmpegStatus() ffmpeg.Resolution
}

// BranchRuntimeService is the subset of the branch manager the handlers need.
type BranchRuntimeService interface {
	Snapshot(ctx context.Context) ([]branch.Snapshot, error)
	StartBranch(ctx context.Context, platformID string) (branch.Outcome, error)
	StopBranch(ctx context.Context, platformID string) error
	RestartBranch(ctx context.Context, platformID string) (branch.Outcome, error)
	StartEnabled(ctx context.Context) []branch.StartEnabledResult
	StopAll(ctx context.Context)
	// Forget stops (best-effort) and removes a branch's tracked state. Used
	// only by the platform-deletion cascade in credentials.go, so no branch
	// entry lingers for a platform id that no longer exists.
	Forget(ctx context.Context, platformID string)
}

const branchesSchemaVersion = 1
const ffmpegStatusSchemaVersion = 1

// --- FFmpeg dependency status ------------------------------------------

// ffmpegStatusResponse never carries the resolved executable path - see
// ffmpeg.Resolution.Path's own doc comment for why.
type ffmpegStatusResponse struct {
	Version int              `json:"version"`
	FFmpeg  ffmpegDependency `json:"ffmpeg"`
}

type ffmpegDependency struct {
	State           string               `json:"state"`
	Source          ffmpeg.Source        `json:"source"`
	DetectedVersion string               `json:"detectedVersion,omitempty"`
	MinimumVersion  string               `json:"minimumVersion"`
	Capabilities    ffmpeg.Capabilities  `json:"capabilities"`
	LastError       *ffmpeg.RuntimeError `json:"lastError"`
}

// ffmpegState derives the public state identifier from a Resolution. Kept
// separate from ffmpeg.Resolution itself, which has no notion of "state" -
// only Source, Compatible and Err.
func ffmpegState(resolution ffmpeg.Resolution) string {
	switch {
	case resolution.Source == ffmpeg.SourceMissing:
		return "missing"
	case resolution.Compatible:
		return "ready"
	case resolution.Err != nil &&
		(resolution.Err.Code == ffmpeg.CodeNotExecutable || resolution.Err.Code == ffmpeg.CodeExecutionFailed):
		return "error"
	default:
		return "incompatible"
	}
}

func toFFmpegStatusResponse(resolution ffmpeg.Resolution) ffmpegStatusResponse {
	return ffmpegStatusResponse{
		Version: ffmpegStatusSchemaVersion,
		FFmpeg: ffmpegDependency{
			State:           ffmpegState(resolution),
			Source:          resolution.Source,
			DetectedVersion: resolution.Version,
			MinimumVersion:  ffmpeg.MinimumVersion,
			Capabilities:    resolution.Capabilities,
			LastError:       resolution.Err,
		},
	}
}

func handleGetFFmpegStatus(logger *slog.Logger, service FFmpegRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, toFFmpegStatusResponse(service.FFmpegStatus()))
	}
}

// --- branch list ------------------------------------------------------

// branchesResponse embeds branch.Snapshot directly: every field on it was
// already designed to be API-safe (no secret, no full destination URL, no
// process id) - see branch.Snapshot's own doc comment.
type branchesResponse struct {
	Version  int               `json:"version"`
	Branches []branch.Snapshot `json:"branches"`
}

func handleGetBranches(logger *slog.Logger, service BranchRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshots, err := service.Snapshot(r.Context())
		if err != nil {
			writeBranchError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, branchesResponse{
			Version:  branchesSchemaVersion,
			Branches: snapshots,
		})
	}
}

// --- single-branch commands ---------------------------------------------

type branchCommandResponse struct {
	Status   string   `json:"status"`
	Blockers []string `json:"blockers,omitempty"`
}

// writeOutcome renders a Start/Restart result.
//
// "Blocked" is answered 200, not an error status: it is a normal, structured
// outcome the caller asked for and the response body describes fully (which
// blockers), not a malformed-request or server failure. A genuine conflict
// (already running) is a real HTTP error, since that is a state problem, not
// an eligibility answer - see writeBranchError's ErrConflict case for the
// path used when the manager itself returns that error rather than an
// Outcome with Conflict set.
func writeOutcome(w http.ResponseWriter, logger *slog.Logger, outcome branch.Outcome, acceptedStatus string) {
	switch {
	case outcome.Conflict:
		writeError(w, logger, http.StatusConflict, "branch_conflict",
			"This destination already has a process starting, live or restarting.")
	case len(outcome.Blockers) > 0:
		writeJSON(w, logger, http.StatusOK, branchCommandResponse{
			Status:   "blocked",
			Blockers: outcome.Blockers,
		})
	default:
		writeJSON(w, logger, http.StatusAccepted, branchCommandResponse{Status: acceptedStatus})
	}
}

func handleStartBranch(logger *slog.Logger, service BranchRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		outcome, err := service.StartBranch(r.Context(), r.PathValue("id"))
		if err != nil {
			writeBranchError(w, logger, r, err)
			return
		}
		writeOutcome(w, logger, outcome, "starting")
	}
}

func handleStopBranch(logger *slog.Logger, service BranchRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := service.StopBranch(r.Context(), r.PathValue("id")); err != nil {
			writeBranchError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, branchCommandResponse{Status: "stopping"})
	}
}

func handleRestartBranch(logger *slog.Logger, service BranchRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		outcome, err := service.RestartBranch(r.Context(), r.PathValue("id"))
		if err != nil {
			writeBranchError(w, logger, r, err)
			return
		}
		writeOutcome(w, logger, outcome, "starting")
	}
}

// --- bulk commands ------------------------------------------------------

type startEnabledResultResponse struct {
	PlatformID string   `json:"platformId"`
	Accepted   bool     `json:"accepted"`
	Blockers   []string `json:"blockers,omitempty"`
	Conflict   bool     `json:"conflict,omitempty"`
}

type startEnabledResponse struct {
	Results []startEnabledResultResponse `json:"results"`
}

func handleStartEnabledBranches(logger *slog.Logger, service BranchRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		results := service.StartEnabled(r.Context())

		response := startEnabledResponse{Results: make([]startEnabledResultResponse, 0, len(results))}
		for _, result := range results {
			response.Results = append(response.Results, startEnabledResultResponse{
				PlatformID: result.PlatformID,
				Accepted:   result.Outcome.Accepted,
				Blockers:   result.Outcome.Blockers,
				Conflict:   result.Outcome.Conflict,
			})
		}
		writeJSON(w, logger, http.StatusOK, response)
	}
}

func handleStopAllBranches(logger *slog.Logger, service BranchRuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		service.StopAll(r.Context())
		writeJSON(w, logger, http.StatusOK, branchCommandResponse{Status: "stopping"})
	}
}

// --- error mapping ------------------------------------------------------

func writeBranchError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, branch.ErrNotFound):
		writeError(w, logger, http.StatusNotFound,
			"platform_not_found", "The requested platform does not exist.")

	case errors.Is(err, branch.ErrNotRunning):
		writeError(w, logger, http.StatusConflict,
			"branch_not_running", "This destination is not currently running.")

	case errors.Is(err, branch.ErrConflict):
		writeError(w, logger, http.StatusConflict,
			"branch_conflict", "This destination already has a process starting, live or restarting.")

	default:
		logger.Error("unhandled branch error",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Any("error", err),
		)
		writeError(w, logger, http.StatusInternalServerError,
			"internal_error", "The server encountered an unexpected error.")
	}
}
