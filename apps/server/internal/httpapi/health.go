package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/buildinfo"
)

// HealthResponse is the payload of GET /api/health.
//
// The frontend validates this shape with Zod, so field names and JSON tags are
// part of the API contract and must not change without updating
// apps/web/src/models/health.ts.
type HealthResponse struct {
	Status        string  `json:"status"`
	Service       string  `json:"service"`
	Version       string  `json:"version"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
	Time          string  `json:"time"`
}

// healthHandler reports that the process is up. It intentionally checks nothing
// else yet: there is no database, no MediaMTX and no FFmpeg to probe. Once those
// exist this endpoint reports their state too.
func healthHandler(logger *slog.Logger, startedAt time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, HealthResponse{
			Status:        "ok",
			Service:       buildinfo.ServiceName,
			Version:       buildinfo.EffectiveVersion(),
			UptimeSeconds: time.Since(startedAt).Seconds(),
			Time:          time.Now().UTC().Format(time.RFC3339),
		})
	}
}
