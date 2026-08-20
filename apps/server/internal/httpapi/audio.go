package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	audiort "github.com/streaming-tree/server/internal/audio"
	domain "github.com/streaming-tree/server/internal/domain/audio"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/remoteoverlay"
	"github.com/streaming-tree/server/internal/provider/tts"
)

// maxAudioBodyBytes/maxAudioTestSpeakBodyBytes/maxAudioAckBodyBytes cap
// the small JSON bodies this surface accepts - settings are the
// largest (a handful of short lists), Test Speak carries only bounded
// text, and an ACK carries only an id/token/kind triple.
const (
	maxAudioBodyBytes          = 16 * 1024
	maxAudioTestSpeakBodyBytes = 8 * 1024
	maxAudioAckBodyBytes       = 2 * 1024

	maxAudioSSEClients = 4
	audioSSEKeepalive  = 15 * time.Second
)

// AudioService is the subset of internal/audio.Manager the HTTP layer
// needs - settings CRUD, provider introspection, runtime status/queue
// commands, Test Speak, and the renderer-lease/acknowledgement surface
// the public routes drive.
type AudioService interface {
	GetSettings(ctx context.Context) (domain.Settings, error)
	UpdateSettings(ctx context.Context, input domain.Settings) (domain.Settings, error)
	RotatePublicSlug(ctx context.Context) (domain.Settings, error)
	CurrentPublicSlug() string

	ProviderCapabilities() tts.Capabilities
	ListVoices(ctx context.Context) ([]tts.Voice, error)

	Status() audiort.Status
	PendingList() []audiort.Item

	SkipCurrent() bool
	ClearQueue() int
	Approve(id string) bool
	Reject(id string) bool
	TestSpeak(text string) (audiort.Item, error)

	ConnectRenderer() (string, error)
	DisconnectRenderer(token string)
	Ack(token, itemID string, kind audiort.AckKind) error
	PublicCurrentSnapshot() audiort.PublicCurrent
	CurrentAudioBytes(itemID, bytesToken string) (audioBytes []byte, contentType string, ok bool)
	SubscribeCurrentChanges() (id uint64, ch <-chan struct{})
	UnsubscribeCurrentChanges(id uint64)
}

// registerAudioRoutes wires the Stage 17A TTS/audio management API
// (/api/audio/...) and the public Browser Source audio output API
// (/api/public/audio/{slug}/...).
func registerAudioRoutes(mux *http.ServeMux, logger *slog.Logger, svc AudioService, remoteOverlayResolver RemoteOverlayResolver) {
	mux.HandleFunc("GET /api/audio/settings", handleGetAudioSettings(logger, svc))
	mux.HandleFunc("PUT /api/audio/settings", handlePutAudioSettings(logger, svc))
	mux.HandleFunc("/api/audio/settings", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("GET /api/audio/capabilities", handleGetAudioCapabilities(logger, svc))
	mux.HandleFunc("/api/audio/capabilities", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/audio/voices", handleGetAudioVoices(logger, svc))
	mux.HandleFunc("/api/audio/voices", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/audio/status", handleGetAudioStatus(logger, svc))
	mux.HandleFunc("/api/audio/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/audio/pending", handleGetAudioPending(logger, svc))
	mux.HandleFunc("/api/audio/pending", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/audio/queue/skip-current", handlePostAudioQueueSkipCurrent(logger, svc))
	mux.HandleFunc("/api/audio/queue/skip-current", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/audio/queue/clear", handlePostAudioQueueClear(logger, svc))
	mux.HandleFunc("/api/audio/queue/clear", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/audio/pending/{id}/approve", handlePostAudioPendingApprove(logger, svc))
	mux.HandleFunc("/api/audio/pending/{id}/approve", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/audio/pending/{id}/reject", handlePostAudioPendingReject(logger, svc))
	mux.HandleFunc("/api/audio/pending/{id}/reject", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/audio/rotate-slug", handlePostAudioRotateSlug(logger, svc))
	mux.HandleFunc("/api/audio/rotate-slug", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/audio/test-speak", handlePostAudioTestSpeak(logger, svc))
	mux.HandleFunc("/api/audio/test-speak", methodNotAllowed(logger, http.MethodPost))

	limiter := newAudioStreamLimiter()
	mux.HandleFunc("GET /api/public/audio/{slug}/stream", handlePublicAudioStream(logger, svc, limiter, remoteOverlayResolver))
	mux.HandleFunc("/api/public/audio/{slug}/stream", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/public/audio/{slug}/bytes/{token}", handlePublicAudioBytes(logger, svc, remoteOverlayResolver))
	mux.HandleFunc("/api/public/audio/{slug}/bytes/{token}", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/public/audio/{slug}/ack", handlePublicAudioAck(logger, svc, remoteOverlayResolver))
	mux.HandleFunc("/api/public/audio/{slug}/ack", methodNotAllowed(logger, http.MethodPost))
}

// --- settings DTOs -------------------------------------------------------

// audioSettingsRequest deliberately omits PublicSlug/CreatedAt/UpdatedAt
// - none of them is ever client-settable (PublicSlug changes only via
// POST .../rotate-slug; the timestamps are server-assigned), so strict
// unknown-field rejection catches a client that tries anyway.
type audioSettingsRequest struct {
	Enabled                      bool     `json:"enabled"`
	ProviderMode                 string   `json:"providerMode"`
	EnabledEventTypes            []string `json:"enabledEventTypes"`
	EnabledProviderIDs           []string `json:"enabledProviderIds"`
	EnabledSourceIDs             []string `json:"enabledSourceIds"`
	SupporterOnlyMode            bool     `json:"supporterOnlyMode"`
	ThresholdCurrency            string   `json:"thresholdCurrency"`
	ThresholdMinimumAmountMicros *int64   `json:"thresholdMinimumAmountMicros,omitempty"`
	MinimumBits                  *int64   `json:"minimumBits,omitempty"`
	MaxTextLengthCodePoints      int      `json:"maxTextLengthCodePoints"`
	PerUserCooldownSeconds       int      `json:"perUserCooldownSeconds"`
	GlobalCooldownSeconds        int      `json:"globalCooldownSeconds"`
	BlockedWords                 []string `json:"blockedWords"`
	RemoveURLs                   bool     `json:"removeUrls"`
	NormalizeRepeatedChars       bool     `json:"normalizeRepeatedChars"`
	SuppressCommands             bool     `json:"suppressCommands"`
	QueueCapacity                int      `json:"queueCapacity"`
	ManualApproval               bool     `json:"manualApproval"`
	VoiceID                      string   `json:"voiceId"`
	Language                     string   `json:"language"`
	Speed                        float64  `json:"speed"`
	Volume                       float64  `json:"volume"`
}

func (req audioSettingsRequest) toDomain() domain.Settings {
	eventTypes := make([]engagement.Type, len(req.EnabledEventTypes))
	for i, t := range req.EnabledEventTypes {
		eventTypes[i] = engagement.Type(t)
	}
	providerIDs := make([]engagement.ProviderID, len(req.EnabledProviderIDs))
	for i, p := range req.EnabledProviderIDs {
		providerIDs[i] = engagement.ProviderID(p)
	}
	return domain.Settings{
		Enabled:                      req.Enabled,
		ProviderMode:                 domain.ProviderMode(req.ProviderMode),
		EnabledEventTypes:            eventTypes,
		EnabledProviderIDs:           providerIDs,
		EnabledSourceIDs:             req.EnabledSourceIDs,
		SupporterOnlyMode:            req.SupporterOnlyMode,
		ThresholdCurrency:            domain.NormalizeCurrency(req.ThresholdCurrency),
		ThresholdMinimumAmountMicros: req.ThresholdMinimumAmountMicros,
		MinimumBits:                  req.MinimumBits,
		MaxTextLengthCodePoints:      req.MaxTextLengthCodePoints,
		PerUserCooldownSeconds:       req.PerUserCooldownSeconds,
		GlobalCooldownSeconds:        req.GlobalCooldownSeconds,
		BlockedWords:                 req.BlockedWords,
		RemoveURLs:                   req.RemoveURLs,
		NormalizeRepeatedChars:       req.NormalizeRepeatedChars,
		SuppressCommands:             req.SuppressCommands,
		QueueCapacity:                req.QueueCapacity,
		ManualApproval:               req.ManualApproval,
		VoiceID:                      req.VoiceID,
		Language:                     req.Language,
		Speed:                        req.Speed,
		Volume:                       req.Volume,
	}
}

type audioSettingsResponse struct {
	Enabled                      bool     `json:"enabled"`
	ProviderMode                 string   `json:"providerMode"`
	EnabledEventTypes            []string `json:"enabledEventTypes"`
	EnabledProviderIDs           []string `json:"enabledProviderIds"`
	EnabledSourceIDs             []string `json:"enabledSourceIds"`
	SupporterOnlyMode            bool     `json:"supporterOnlyMode"`
	ThresholdCurrency            string   `json:"thresholdCurrency,omitempty"`
	ThresholdMinimumAmountMicros *int64   `json:"thresholdMinimumAmountMicros,omitempty"`
	MinimumBits                  *int64   `json:"minimumBits,omitempty"`
	MaxTextLengthCodePoints      int      `json:"maxTextLengthCodePoints"`
	PerUserCooldownSeconds       int      `json:"perUserCooldownSeconds"`
	GlobalCooldownSeconds        int      `json:"globalCooldownSeconds"`
	BlockedWords                 []string `json:"blockedWords"`
	RemoveURLs                   bool     `json:"removeUrls"`
	NormalizeRepeatedChars       bool     `json:"normalizeRepeatedChars"`
	SuppressCommands             bool     `json:"suppressCommands"`
	QueueCapacity                int      `json:"queueCapacity"`
	ManualApproval               bool     `json:"manualApproval"`
	VoiceID                      string   `json:"voiceId"`
	Language                     string   `json:"language"`
	Speed                        float64  `json:"speed"`
	Volume                       float64  `json:"volume"`
	PublicSlug                   string   `json:"publicSlug"`
	CreatedAt                    string   `json:"createdAt"`
	UpdatedAt                    string   `json:"updatedAt"`
}

func toAudioSettingsResponse(s domain.Settings) audioSettingsResponse {
	eventTypes := make([]string, len(s.EnabledEventTypes))
	for i, t := range s.EnabledEventTypes {
		eventTypes[i] = string(t)
	}
	providerIDs := make([]string, len(s.EnabledProviderIDs))
	for i, p := range s.EnabledProviderIDs {
		providerIDs[i] = string(p)
	}
	sourceIDs := s.EnabledSourceIDs
	if sourceIDs == nil {
		sourceIDs = []string{}
	}
	blockedWords := s.BlockedWords
	if blockedWords == nil {
		blockedWords = []string{}
	}
	return audioSettingsResponse{
		Enabled: s.Enabled, ProviderMode: string(s.ProviderMode),
		EnabledEventTypes: eventTypes, EnabledProviderIDs: providerIDs, EnabledSourceIDs: sourceIDs,
		SupporterOnlyMode:            s.SupporterOnlyMode,
		ThresholdCurrency:            s.ThresholdCurrency,
		ThresholdMinimumAmountMicros: s.ThresholdMinimumAmountMicros,
		MinimumBits:                  s.MinimumBits,
		MaxTextLengthCodePoints:      s.MaxTextLengthCodePoints,
		PerUserCooldownSeconds:       s.PerUserCooldownSeconds,
		GlobalCooldownSeconds:        s.GlobalCooldownSeconds,
		BlockedWords:                 blockedWords,
		RemoveURLs:                   s.RemoveURLs,
		NormalizeRepeatedChars:       s.NormalizeRepeatedChars,
		SuppressCommands:             s.SuppressCommands,
		QueueCapacity:                s.QueueCapacity,
		ManualApproval:               s.ManualApproval,
		VoiceID:                      s.VoiceID,
		Language:                     s.Language,
		Speed:                        s.Speed,
		Volume:                       s.Volume,
		PublicSlug:                   s.PublicSlug,
		CreatedAt:                    s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:                    s.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleGetAudioSettings(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := svc.GetSettings(r.Context())
		if err != nil {
			writeAudioError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAudioSettingsResponse(s))
	}
}

func handlePutAudioSettings(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body audioSettingsRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAudioBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		saved, err := svc.UpdateSettings(r.Context(), body.toDomain())
		if err != nil {
			writeAudioError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAudioSettingsResponse(saved))
	}
}

func handlePostAudioRotateSlug(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		saved, err := svc.RotatePublicSlug(r.Context())
		if err != nil {
			writeAudioError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAudioSettingsResponse(saved))
	}
}

// --- capabilities / voices -----------------------------------------------

type audioCapabilitiesResponse struct {
	KnownProviderModes       []string `json:"knownProviderModes"`
	ImplementedProviderModes []string `json:"implementedProviderModes"`
	SystemProviderAvailable  bool     `json:"systemProviderAvailable"`
	SystemProviderReason     string   `json:"systemProviderReason,omitempty"`
}

func handleGetAudioCapabilities(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caps := svc.ProviderCapabilities()
		writeJSON(w, logger, http.StatusOK, audioCapabilitiesResponse{
			KnownProviderModes:       providerModeStrings(domain.KnownProviderModes),
			ImplementedProviderModes: providerModeStrings(domain.ImplementedProviderModes),
			SystemProviderAvailable:  caps.Available,
			SystemProviderReason:     caps.Reason,
		})
	}
}

func providerModeStrings(modes []domain.ProviderMode) []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = string(m)
	}
	return out
}

type audioVoiceResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Language  string `json:"language,omitempty"`
	Gender    string `json:"gender,omitempty"`
	IsDefault bool   `json:"isDefault"`
}

func handleGetAudioVoices(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		voices, err := svc.ListVoices(r.Context())
		if err != nil {
			writeAudioError(w, logger, err)
			return
		}
		out := make([]audioVoiceResponse, 0, len(voices))
		for _, v := range voices {
			out = append(out, audioVoiceResponse{ID: v.ID, Name: v.Name, Language: v.Language, Gender: v.Gender, IsDefault: v.IsDefault})
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

// --- status / pending / queue commands ------------------------------------

type audioStatusResponse struct {
	Enabled              bool   `json:"enabled"`
	ProviderMode         string `json:"providerMode"`
	ProviderAvailable    bool   `json:"providerAvailable"`
	RendererConnected    bool   `json:"rendererConnected"`
	HasCurrentItem       bool   `json:"hasCurrentItem"`
	CurrentSynthetic     bool   `json:"currentSynthetic"`
	PendingApprovalCount int    `json:"pendingApprovalCount"`
	ReadyQueueCount      int    `json:"readyQueueCount"`
	Capacity             int    `json:"capacity"`
	TotalEnqueued        int    `json:"totalEnqueued"`
	TotalCapacityDropped int    `json:"totalCapacityDropped"`
	TotalExpired         int    `json:"totalExpired"`
	TotalRejected        int    `json:"totalRejected"`
	TotalManuallySkipped int    `json:"totalManuallySkipped"`
	TotalSynthetic       int    `json:"totalSynthetic"`
	TotalPlayed          int    `json:"totalPlayed"`
	TotalPlaybackFailed  int    `json:"totalPlaybackFailed"`
	TotalSynthesisFailed int    `json:"totalSynthesisFailed"`
	TotalInterrupted     int    `json:"totalInterrupted"`
	// TotalInterruptedByAlert counts a global-TTS item cancelled because
	// a rule-owned alert item preempted it (docs/alert-audio.md §8.3) -
	// kept distinct from TotalInterrupted (renderer-disconnect) above so
	// the two causes are never conflated.
	TotalInterruptedByAlert int  `json:"totalInterruptedByAlert"`
	InputGap                bool `json:"inputGap"`
	Subscribed              bool `json:"subscribed"`
}

func toAudioStatusResponse(st audiort.Status) audioStatusResponse {
	return audioStatusResponse{
		Enabled: st.Enabled, ProviderMode: string(st.ProviderMode), ProviderAvailable: st.ProviderAvailable,
		RendererConnected: st.RendererConnected, HasCurrentItem: st.HasCurrentItem, CurrentSynthetic: st.CurrentSynthetic,
		PendingApprovalCount: st.PendingApprovalCount, ReadyQueueCount: st.ReadyQueueCount, Capacity: st.Capacity,
		TotalEnqueued: st.Counters.TotalEnqueued, TotalCapacityDropped: st.Counters.TotalCapacityDropped, TotalExpired: st.Counters.TotalExpired,
		TotalRejected: st.Counters.TotalRejected, TotalManuallySkipped: st.Counters.TotalManuallySkipped, TotalSynthetic: st.Counters.TotalSynthetic,
		TotalPlayed: st.TotalPlayed, TotalPlaybackFailed: st.TotalPlaybackFailed, TotalSynthesisFailed: st.TotalSynthesisFailed, TotalInterrupted: st.TotalInterrupted,
		TotalInterruptedByAlert: st.TotalInterruptedByAlert,
		InputGap:                st.InputGap, Subscribed: st.Subscribed,
	}
}

func handleGetAudioStatus(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, toAudioStatusResponse(svc.Status()))
	}
}

type audioPendingItemResponse struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	EnqueuedAt string `json:"enqueuedAt"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
}

func toAudioPendingItemResponse(it audiort.Item) audioPendingItemResponse {
	resp := audioPendingItemResponse{ID: it.ID, Text: it.Text, EnqueuedAt: it.EnqueuedAt.UTC().Format(time.RFC3339Nano)}
	if !it.ExpiresAt.IsZero() {
		resp.ExpiresAt = it.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return resp
}

func handleGetAudioPending(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := svc.PendingList()
		out := make([]audioPendingItemResponse, 0, len(items))
		for _, it := range items {
			out = append(out, toAudioPendingItemResponse(it))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handlePostAudioQueueSkipCurrent(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		svc.SkipCurrent()
		writeJSON(w, logger, http.StatusOK, toAudioStatusResponse(svc.Status()))
	}
}

func handlePostAudioQueueClear(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		svc.ClearQueue()
		writeJSON(w, logger, http.StatusOK, toAudioStatusResponse(svc.Status()))
	}
}

func handlePostAudioPendingApprove(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if !svc.Approve(r.PathValue("id")) {
			writeError(w, logger, http.StatusNotFound, "audio_item_not_found", "The requested pending item does not exist.")
			return
		}
		writeJSON(w, logger, http.StatusOK, toAudioStatusResponse(svc.Status()))
	}
}

func handlePostAudioPendingReject(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if !svc.Reject(r.PathValue("id")) {
			writeError(w, logger, http.StatusNotFound, "audio_item_not_found", "The requested pending item does not exist.")
			return
		}
		writeJSON(w, logger, http.StatusOK, toAudioStatusResponse(svc.Status()))
	}
}

// --- Test Speak ------------------------------------------------------------

type audioTestSpeakRequest struct {
	Text string `json:"text"`
}

func handlePostAudioTestSpeak(logger *slog.Logger, svc AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body audioTestSpeakRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAudioTestSpeakBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		item, err := svc.TestSpeak(body.Text)
		if err != nil {
			writeAudioError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAudioPendingItemResponse(item))
	}
}

// --- error funnel ----------------------------------------------------------

func writeAudioError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, audiort.ErrDisabled):
		writeError(w, logger, http.StatusConflict, "audio_disabled", "Audio/TTS is currently disabled.")
	case errors.Is(err, audiort.ErrEmptyText):
		writeError(w, logger, http.StatusUnprocessableEntity, "audio_text_empty", "The text is empty after preprocessing.")
	case errors.Is(err, audiort.ErrQueueFull):
		writeError(w, logger, http.StatusTooManyRequests, "audio_queue_full", "The audio queue is currently full.")
	case errors.Is(err, tts.ErrVoiceNotFound):
		writeError(w, logger, http.StatusUnprocessableEntity, "audio_voice_not_found", "The selected voice is not available.")
	case errors.Is(err, tts.ErrUnavailable):
		writeError(w, logger, http.StatusServiceUnavailable, "audio_provider_unavailable", "The text-to-speech provider is not available.")
	case errors.Is(err, domain.ErrValidation):
		writeError(w, logger, http.StatusUnprocessableEntity, "audio_settings_invalid", "The request failed validation.")
	case errors.Is(err, domain.ErrStorage):
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

// --- public routes ---------------------------------------------------------

// audioStreamLimiter mirrors internal/httpapi/alerts.go's own
// alertStreamLimiter exactly, reimplemented here (package-private
// there) - a bounded per-slug SSE connection count.
type audioStreamLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newAudioStreamLimiter() *audioStreamLimiter {
	return &audioStreamLimiter{counts: make(map[string]int)}
}

func (l *audioStreamLimiter) acquire(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[key] >= maxAudioSSEClients {
		return false
	}
	l.counts[key]++
	return true
}

func (l *audioStreamLimiter) release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[key]--
	if l.counts[key] <= 0 {
		delete(l.counts, key)
	}
}

// handlePublicAudioStream serves the public SSE stream
// (docs/audio-tts.md §14): current item only, never the future queue.
// Connecting establishes a new renderer lease (§15) - disconnecting
// (including a plain client navigation away) releases it via the
// deferred DisconnectRenderer call, so a stale browser tab can never
// keep blocking a fresh one.
func handlePublicAudioStream(logger *slog.Logger, svc AudioService, limiter *audioStreamLimiter, remoteOverlayResolver RemoteOverlayResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "Streaming is not supported by this response writer.")
			return
		}

		// presentedSlug is exactly what the client used to reach this
		// endpoint - a remote capability token for a forwarded request,
		// or the real local publicSlug for a direct one. It is echoed
		// back into bytesUrl below unchanged, so the client's follow-up
		// request to that URL resolves the same way this one did
		// (docs/remote-ingest.md §11/§12) - resolvedSlug (the real
		// local slug) is used only for the internal CurrentPublicSlug
		// comparison, never exposed to the client.
		presentedSlug := r.PathValue("slug")
		resolvedSlug, resolvedOK, resolveErr := resolvePublicSlug(r.Context(), remoteOverlayResolver, remoteoverlay.DomainAudio, presentedSlug)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		keepalive := time.NewTicker(audioSSEKeepalive)
		defer keepalive.Stop()

		if resolveErr != nil || !resolvedOK || svc.CurrentPublicSlug() != resolvedSlug {
			_ = writeSSEEvent(w, "audio.gap", 0, map[string]string{"reason": "unknown_slug"})
			flusher.Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case <-keepalive.C:
					writeSSEComment(w, "keepalive")
					flusher.Flush()
				}
			}
		}

		if !limiter.acquire(resolvedSlug) {
			_ = writeSSEEvent(w, "audio.gap", 0, map[string]string{"reason": "stream_limit_reached"})
			flusher.Flush()
			return
		}
		defer limiter.release(resolvedSlug)

		token, err := svc.ConnectRenderer()
		if err != nil {
			_ = writeSSEEvent(w, "audio.gap", 0, map[string]string{"reason": "renderer_unavailable"})
			flusher.Flush()
			return
		}
		defer svc.DisconnectRenderer(token)

		subID, changed := svc.SubscribeCurrentChanges()
		defer svc.UnsubscribeCurrentChanges(subID)

		emit := func() {
			snap := svc.PublicCurrentSnapshot()
			if snap.Idle || !snap.HasItem {
				_ = writeSSEEvent(w, "audio.idle", 0, map[string]any{})
				flusher.Flush()
				return
			}
			_ = writeSSEEvent(w, "audio.current", snap.Sequence, map[string]any{
				"itemId":      snap.ItemID,
				"bytesUrl":    "/api/public/audio/" + presentedSlug + "/bytes/" + snap.BytesToken,
				"contentType": snap.ContentType,
				"volume":      snap.Volume,
			})
			flusher.Flush()
		}

		_ = writeSSEEvent(w, "audio.reset", 0, map[string]any{"rendererToken": token})
		flusher.Flush()
		emit()

		for {
			select {
			case <-r.Context().Done():
				return
			case _, open := <-changed:
				if !open {
					return
				}
				emit()
			case <-keepalive.C:
				writeSSEComment(w, "keepalive")
				flusher.Flush()
			}
		}
	}
}

// handlePublicAudioBytes serves the current item's already-generated
// audio bytes. The token alone (§14's own single-segment design)
// authorizes the fetch: it is a fresh, high-entropy value per
// synthesized item, so it is combined here with a freshly-read current
// item id purely to reuse CurrentAudioBytes's existing double-check,
// never as an additional secret the client must supply.
func handlePublicAudioBytes(logger *slog.Logger, svc AudioService, remoteOverlayResolver RemoteOverlayResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolvedSlug, ok, err := resolvePublicSlug(r.Context(), remoteOverlayResolver, remoteoverlay.DomainAudio, r.PathValue("slug"))
		if err != nil || !ok || svc.CurrentPublicSlug() != resolvedSlug {
			writeError(w, logger, http.StatusNotFound, "audio_not_available", "No audio is currently available.")
			return
		}
		snap := svc.PublicCurrentSnapshot()
		if snap.Idle || !snap.HasItem {
			writeError(w, logger, http.StatusNotFound, "audio_not_available", "No audio is currently available.")
			return
		}
		audioBytes, contentType, ok := svc.CurrentAudioBytes(snap.ItemID, r.PathValue("token"))
		if !ok {
			writeError(w, logger, http.StatusNotFound, "audio_not_available", "No audio is currently available.")
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(audioBytes))
	}
}

type audioAckRequest struct {
	Token  string `json:"token"`
	ItemID string `json:"itemId"`
	Kind   string `json:"kind"`
}

// handlePublicAudioAck applies one playback acknowledgement -
// validated end to end by Manager.Ack itself (session/item/state);
// this handler only decodes and translates the outcome.
func handlePublicAudioAck(logger *slog.Logger, svc AudioService, remoteOverlayResolver RemoteOverlayResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolvedSlug, ok, err := resolvePublicSlug(r.Context(), remoteOverlayResolver, remoteoverlay.DomainAudio, r.PathValue("slug"))
		if err != nil || !ok || svc.CurrentPublicSlug() != resolvedSlug {
			writeError(w, logger, http.StatusNotFound, "audio_not_available", "No audio is currently available.")
			return
		}
		var body audioAckRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAudioAckBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		kind, ok := parseAudioAckKind(body.Kind)
		if !ok {
			writeError(w, logger, http.StatusUnprocessableEntity, "audio_ack_kind_invalid", "The acknowledgement kind is not recognized.")
			return
		}
		if err := svc.Ack(body.Token, body.ItemID, kind); err != nil {
			writeError(w, logger, http.StatusConflict, "audio_ack_rejected", "This acknowledgement was rejected.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseAudioAckKind(s string) (audiort.AckKind, bool) {
	switch audiort.AckKind(s) {
	case audiort.AckStarted, audiort.AckEnded, audiort.AckFailed:
		return audiort.AckKind(s), true
	default:
		return "", false
	}
}
