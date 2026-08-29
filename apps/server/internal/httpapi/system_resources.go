package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/sysresources"
)

// ResourcesService serves the local host-resource snapshot behind
// GET /api/system/resources.
type ResourcesService interface {
	Snapshot() sysresources.Snapshot
}

// handleGetSystemResources reports the collector's most recent cached
// sample - never sampling directly inside the request, so a slow syscall
// can never make this endpoint slow to respond.
func handleGetSystemResources(logger *slog.Logger, svc ResourcesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, svc.Snapshot())
	}
}
