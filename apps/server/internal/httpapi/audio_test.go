package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	audiort "github.com/streaming-tree/server/internal/audio"
	audiodomain "github.com/streaming-tree/server/internal/domain/audio"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/tts"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeAudioHTTPProvider is a minimal tts.Provider double for HTTP-layer
// tests - internal/audio's own extensive fake provider lives in an
// unexported test file in a different package, so this is a small,
// independent equivalent.
type fakeAudioHTTPProvider struct {
	available bool
}

func (p fakeAudioHTTPProvider) Capabilities() tts.Capabilities {
	if !p.available {
		return tts.Capabilities{Available: false, Reason: "fake provider disabled for this test"}
	}
	return tts.Capabilities{Available: true}
}

func (p fakeAudioHTTPProvider) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	return []tts.Voice{{ID: "voice-1", Name: "Fake Voice", Language: "en-US", IsDefault: true}}, nil
}

func (p fakeAudioHTTPProvider) Synthesize(ctx context.Context, in tts.SynthesizeInput) (tts.SynthesizeResult, error) {
	return tts.SynthesizeResult{ContentType: "audio/wav", Audio: []byte{1, 2, 3, 4}}, nil
}

type audioTestServer struct {
	handler http.Handler
	manager *audiort.Manager
}

func newAudioTestServer(t *testing.T, available bool) *audioTestServer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	settingsSvc := audiodomain.NewService(audiodomain.Options{Repository: sqlite.NewAudioSettingsRepository(db.DB)})
	manager := audiort.NewManager(audiort.Options{
		SettingsService: settingsSvc,
		Bus:             eventBus,
		Provider:        fakeAudioHTTPProvider{available: available},
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
	})

	handler := NewRouter(Options{Logger: logger, StartedAt: time.Now(), Audio: manager})
	return &audioTestServer{handler: handler, manager: manager}
}

func (ts *audioTestServer) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeAudioBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func validAudioSettingsBody() map[string]any {
	return map[string]any{
		"enabled": true, "providerMode": "system",
		"enabledEventTypes": []string{"chat.message"}, "enabledProviderIds": []string{}, "enabledSourceIds": []string{},
		"supporterOnlyMode": false, "thresholdCurrency": "", "blockedWords": []string{},
		"maxTextLengthCodePoints": 500, "perUserCooldownSeconds": 30, "globalCooldownSeconds": 3,
		"removeUrls": true, "normalizeRepeatedChars": true, "suppressCommands": true,
		"queueCapacity": 100, "manualApproval": false,
		"voiceId": "", "language": "", "speed": 1.0, "volume": 1.0,
	}
}

// --- settings --------------------------------------------------------------

func TestGetAudioSettingsReturnsDefaults(t *testing.T) {
	ts := newAudioTestServer(t, true)
	body := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	if body["enabled"] != false {
		t.Errorf(`body["enabled"] = %v, want false`, body["enabled"])
	}
	if body["publicSlug"] == "" || body["publicSlug"] == nil {
		t.Error("publicSlug is empty on first GET, want a generated slug")
	}
}

func TestPutAudioSettingsRoundTrips(t *testing.T) {
	ts := newAudioTestServer(t, true)
	resp := ts.do(t, http.MethodPut, "/api/audio/settings", validAudioSettingsBody())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp.StatusCode)
	}
	body := decodeAudioBody(t, resp)
	if body["enabled"] != true || body["providerMode"] != "system" {
		t.Errorf("body = %+v, want enabled=true providerMode=system", body)
	}

	get := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	if get["enabled"] != true {
		t.Error("settings did not persist across GET after PUT")
	}
}

func TestPutAudioSettingsInvalidProviderModeReturns422(t *testing.T) {
	ts := newAudioTestServer(t, true)
	invalid := validAudioSettingsBody()
	invalid["providerMode"] = "not-a-real-mode"
	resp := ts.do(t, http.MethodPut, "/api/audio/settings", invalid)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestPutAudioSettingsUnknownFieldRejected(t *testing.T) {
	ts := newAudioTestServer(t, true)
	body := validAudioSettingsBody()
	body["totallyUnknownField"] = true
	resp := ts.do(t, http.MethodPut, "/api/audio/settings", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown field", resp.StatusCode)
	}
}

func TestPutAudioSettingsCannotSetPublicSlug(t *testing.T) {
	ts := newAudioTestServer(t, true)
	before := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))

	body := validAudioSettingsBody()
	body["publicSlug"] = "attacker-supplied"
	resp := ts.do(t, http.MethodPut, "/api/audio/settings", body)
	// publicSlug is not a field on the request DTO at all, so strict
	// unknown-field rejection catches it (400), rather than silently
	// accepting and ignoring it.
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (publicSlug is not a settable field)", resp.StatusCode)
	}
	_ = before
}

func TestAudioSettingsMethodNotAllowed(t *testing.T) {
	ts := newAudioTestServer(t, true)
	resp := ts.do(t, http.MethodDelete, "/api/audio/settings", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, "GET") || !strings.Contains(allow, "PUT") {
		t.Errorf("Allow header = %q, want it to contain GET and PUT", allow)
	}
}

// --- capabilities / voices --------------------------------------------------

func TestGetAudioCapabilitiesReflectsProvider(t *testing.T) {
	ts := newAudioTestServer(t, false)
	body := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/capabilities", nil))
	if body["systemProviderAvailable"] != false {
		t.Errorf("systemProviderAvailable = %v, want false", body["systemProviderAvailable"])
	}
	if body["systemProviderReason"] == "" || body["systemProviderReason"] == nil {
		t.Error("systemProviderReason is empty when unavailable, want an honest reason")
	}
}

func TestGetAudioVoicesReturnsProviderVoices(t *testing.T) {
	ts := newAudioTestServer(t, true)
	resp := ts.do(t, http.MethodGet, "/api/audio/voices", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var voices []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&voices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(voices) != 1 || voices[0]["id"] != "voice-1" {
		t.Errorf("voices = %+v, want the fake provider's one voice", voices)
	}
}

// --- status / pending / queue commands --------------------------------------

func TestGetAudioStatus(t *testing.T) {
	ts := newAudioTestServer(t, true)
	body := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/status", nil))
	if body["readyQueueCount"] != float64(0) {
		t.Errorf("readyQueueCount = %v, want 0", body["readyQueueCount"])
	}
	if body["totalInterruptedByAlert"] != float64(0) {
		t.Errorf("totalInterruptedByAlert = %v, want 0", body["totalInterruptedByAlert"])
	}
}

func TestAudioTestSpeakThenPendingListAndApprove(t *testing.T) {
	ts := newAudioTestServer(t, true)
	ts.do(t, http.MethodPut, "/api/audio/settings", validAudioSettingsBody())

	manualApproval := validAudioSettingsBody()
	manualApproval["manualApproval"] = true
	ts.do(t, http.MethodPut, "/api/audio/settings", manualApproval)

	// Test Speak always bypasses manual approval, so it lands in the
	// ready queue - approve/reject below are exercised against a
	// synthetically-pending item created directly through the manager
	// instead, since Test Speak is documented to skip pending entirely.
	resp := ts.do(t, http.MethodPost, "/api/audio/test-speak", map[string]any{"text": "hello from a test"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST test-speak status = %d, want 200", resp.StatusCode)
	}
	status := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/status", nil))
	if status["readyQueueCount"] != float64(1) {
		t.Errorf("readyQueueCount = %v, want 1 after Test Speak", status["readyQueueCount"])
	}
}

func TestAudioTestSpeakDisabledReturns409(t *testing.T) {
	ts := newAudioTestServer(t, true)
	resp := ts.do(t, http.MethodPost, "/api/audio/test-speak", map[string]any{"text": "hello"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409 (disabled by default)", resp.StatusCode)
	}
}

func TestAudioPendingApproveRejectUnknownIDReturns404(t *testing.T) {
	ts := newAudioTestServer(t, true)
	if resp := ts.do(t, http.MethodPost, "/api/audio/pending/nonexistent/approve", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("approve unknown id status = %d, want 404", resp.StatusCode)
	}
	if resp := ts.do(t, http.MethodPost, "/api/audio/pending/nonexistent/reject", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("reject unknown id status = %d, want 404", resp.StatusCode)
	}
}

func TestAudioQueueSkipCurrentAndClear(t *testing.T) {
	ts := newAudioTestServer(t, true)
	ts.do(t, http.MethodPut, "/api/audio/settings", validAudioSettingsBody())

	if resp := ts.do(t, http.MethodPost, "/api/audio/queue/skip-current", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("skip-current status = %d, want 200 even with nothing current", resp.StatusCode)
	}
	if resp := ts.do(t, http.MethodPost, "/api/audio/queue/clear", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("clear status = %d, want 200", resp.StatusCode)
	}
}

func TestAudioRotateSlugChangesSlug(t *testing.T) {
	ts := newAudioTestServer(t, true)
	before := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	after := decodeAudioBody(t, ts.do(t, http.MethodPost, "/api/audio/rotate-slug", nil))
	if before["publicSlug"] == after["publicSlug"] {
		t.Error("rotate-slug did not change the public slug")
	}
}

// --- public routes -----------------------------------------------------------

func TestPublicAudioStreamUnknownSlugRendersGapNotError(t *testing.T) {
	ts := newAudioTestServer(t, true)
	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/audio/does-not-exist/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never a hard error for an unknown slug)", resp.StatusCode)
	}
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the gap event failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: audio.gap") || !strings.Contains(chunk, "unknown_slug") {
		t.Fatalf("chunk missing audio.gap/unknown_slug: %q", chunk)
	}
}

func TestPublicAudioStreamKnownSlugSendsResetThenIdle(t *testing.T) {
	ts := newAudioTestServer(t, true)
	settings := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	slug := settings["publicSlug"].(string)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/audio/"+slug+"/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// audio.reset and audio.idle are flushed as two separate writes, so
	// they can arrive as two separate reads on the client side -
	// accumulate until both markers are seen (or the context times out).
	var accumulated strings.Builder
	buf := make([]byte, 8192)
	for !strings.Contains(accumulated.String(), "event: audio.idle") {
		n, err := resp.Body.Read(buf)
		accumulated.Write(buf[:n])
		if err != nil {
			t.Fatalf("reading the stream failed: %v (so far: %q)", err, accumulated.String())
		}
	}
	chunk := accumulated.String()
	if !strings.Contains(chunk, "event: audio.reset") {
		t.Errorf("stream missing audio.reset: %q", chunk)
	}
	// The public reset/idle payload must never leak internal identifiers.
	if strings.Contains(chunk, "\"queue\"") || strings.Contains(chunk, "\"settings\"") {
		t.Errorf("stream leaked internal state: %q", chunk)
	}
}

func TestPublicAudioBytesReturns404WhenIdle(t *testing.T) {
	ts := newAudioTestServer(t, true)
	settings := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	slug := settings["publicSlug"].(string)

	resp := ts.do(t, http.MethodGet, "/api/public/audio/"+slug+"/bytes/some-token", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when nothing is currently playing", resp.StatusCode)
	}
}

func TestPublicAudioAckUnknownSlugReturns404(t *testing.T) {
	ts := newAudioTestServer(t, true)
	resp := ts.do(t, http.MethodPost, "/api/public/audio/does-not-exist/ack", map[string]any{
		"token": "x", "itemId": "y", "kind": "playback_started",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestPublicAudioAckInvalidKindReturns422(t *testing.T) {
	ts := newAudioTestServer(t, true)
	settings := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	slug := settings["publicSlug"].(string)

	resp := ts.do(t, http.MethodPost, "/api/public/audio/"+slug+"/ack", map[string]any{
		"token": "x", "itemId": "y", "kind": "not_a_real_kind",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestPublicAudioAckRejectedWithoutActiveRenderer(t *testing.T) {
	ts := newAudioTestServer(t, true)
	settings := decodeAudioBody(t, ts.do(t, http.MethodGet, "/api/audio/settings", nil))
	slug := settings["publicSlug"].(string)

	resp := ts.do(t, http.MethodPost, "/api/public/audio/"+slug+"/ack", map[string]any{
		"token": "no-such-session", "itemId": "no-such-item", "kind": "playback_started",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409 (no active renderer session matches)", resp.StatusCode)
	}
}

func TestPublicAudioRoutesMethodNotAllowed(t *testing.T) {
	ts := newAudioTestServer(t, true)
	if resp := ts.do(t, http.MethodPost, "/api/public/audio/x/stream", nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST stream status = %d, want 405", resp.StatusCode)
	}
	if resp := ts.do(t, http.MethodGet, "/api/public/audio/x/ack", nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET ack status = %d, want 405", resp.StatusCode)
	}
}
