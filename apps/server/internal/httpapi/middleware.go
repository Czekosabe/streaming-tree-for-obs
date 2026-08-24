package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/diagnostics"
)

// middleware is the standard decorator signature used by the chain below.
type middleware func(http.Handler) http.Handler

// chain applies middlewares so that the first one in the list is the outermost.
func chain(h http.Handler, middlewares ...middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// statusRecorder captures the status code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Flush forwards to the underlying ResponseWriter's Flush, when it has one.
// Required so the SSE handler (see engagement.go) can type-assert this
// wrapper to http.Flusher - an embedded http.ResponseWriter interface field
// only promotes ResponseWriter's own three methods, never Flush, so without
// this explicit forwarding method streaming would silently buffer instead
// of pushing each event as it is written.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// withRecovery turns a panic in a handler into a 500 response instead of
// killing the whole process. One failing endpoint must never take the server
// down - the same principle that keeps platform branches independent.
func withRecovery(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered in handler",
						slog.Any("panic", recovered),
						slog.String("path", diagnostics.RedactPath(r.URL.Path)),
					)
					writeError(w, logger, http.StatusInternalServerError,
						"internal_error", "The server encountered an unexpected error.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// withLogging emits one structured line per request.
func withLogging(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, r)

			logger.Info("request",
				slog.String("method", r.Method),
				slog.String("path", diagnostics.RedactPath(r.URL.Path)),
				slog.Int("status", recorder.status),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// withCORS answers pre-flight requests and adds CORS headers for the configured
// local frontend origins.
//
// The allow-list is explicit on purpose: a wildcard would let any page in the
// browser talk to a server that will later control real transmissions.
func withCORS(allowedOrigins []string) middleware {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")

			if origin != "" {
				if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
					w.Header().Set("Access-Control-Max-Age", "600")
				}
				// Responses differ per origin, so caches must key on it.
				w.Header().Add("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
