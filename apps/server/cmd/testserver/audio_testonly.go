//go:build integration

// Test-only HTTP control surface for the Stage 17A fake tts.Provider
// (internal/provider/tts/fake.go). Exists ONLY in this
// `-tags integration` binary, invisible to the real cmd/server -
// governing task §66's own "the fake should record only safe request
// metadata needed for assertions" allowance, applied here as a small,
// explicit control API rather than exposing the fake's Go methods
// directly (the 19th integration script only ever talks HTTP to this
// binary, the same way it talks to every other endpoint).
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/provider/tts"
)

type audioTestonlyAvailableRequest struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

type audioTestonlyDelayRequest struct {
	Milliseconds int `json:"milliseconds"`
}

type audioTestonlySynthesizeCallsResponse struct {
	Count int `json:"count"`
}

// wrapWithAudioTestonlyRoutes registers the fake TTS provider's control
// routes on their own small mux, falling through to next for every
// other path - never modifies internal/httpapi's own router.
func wrapWithAudioTestonlyRoutes(next http.Handler, provider *tts.FakeProvider) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/testonly/tts/available", func(w http.ResponseWriter, r *http.Request) {
		var body audioTestonlyAvailableRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		provider.SetAvailable(body.Available, body.Reason)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/testonly/tts/fail-next", func(w http.ResponseWriter, r *http.Request) {
		provider.FailNextSynthesis()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/testonly/tts/oversize-next", func(w http.ResponseWriter, r *http.Request) {
		provider.OversizeNextSynthesis()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/testonly/tts/delay", func(w http.ResponseWriter, r *http.Request) {
		var body audioTestonlyDelayRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		provider.SetSynthesisDelay(time.Duration(body.Milliseconds) * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/testonly/tts/synthesize-calls", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(audioTestonlySynthesizeCallsResponse{Count: provider.SynthesizeCallCount()})
	})

	mux.Handle("/", next)
	return mux
}
