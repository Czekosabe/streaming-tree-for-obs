package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type audioAssetTestServer struct {
	handler http.Handler
	svc     *audioasset.Service
	db      *sqlite.DB
}

func newAudioAssetTestServer(t *testing.T) *audioAssetTestServer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(ctx, db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	store := visualasset.NewFileStore(filepath.Join(t.TempDir(), "assets"))
	if err := store.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}
	svc := audioasset.NewService(sqlite.NewAudioAssetRepository(db.DB), store, nil)

	handler := NewRouter(Options{Logger: logger, StartedAt: time.Now(), AudioAssets: svc})
	return &audioAssetTestServer{handler: handler, svc: svc, db: db}
}

// seedAlertRule inserts a minimal, real alert_profiles + alert_rules row
// pair directly - required because this project's SQLite connection
// enforces foreign keys (PRAGMA foreign_keys=1), so a reference-tracking
// insert against a rule id that doesn't exist as a real row would fail.
func (ts *audioAssetTestServer) seedAlertRule(t *testing.T, ruleID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := ts.db.DB.ExecContext(context.Background(), `
		INSERT INTO alert_profiles (id, public_slug, name, created_at, updated_at)
		VALUES (?, ?, 'Profile', ?, ?)`, "profile_for_"+ruleID, "slug_for_"+ruleID, now, now); err != nil {
		t.Fatalf("seed alert_profiles: %v", err)
	}
	if _, err := ts.db.DB.ExecContext(context.Background(), `
		INSERT INTO alert_rules (id, profile_id, name, event_type, text_template, created_at, updated_at)
		VALUES (?, ?, 'Rule', 'follow', '{username} followed!', ?, ?)`, ruleID, "profile_for_"+ruleID, now, now); err != nil {
		t.Fatalf("seed alert_rules: %v", err)
	}
}

func (ts *audioAssetTestServer) do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeAudioAssetBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

// testWAVBytes builds a minimal well-formed 16-bit PCM mono WAV file.
func testWAVBytes(numSamples int) []byte {
	const sampleRate, channels, bits = 44100, 1, 16
	blockAlign := channels * bits / 8
	dataSize := numSamples * blockAlign
	byteRate := sampleRate * blockAlign

	buf := make([]byte, 44+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], bits)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))
	return buf
}

func multipartUploadRequest(t *testing.T, path string, fileBytes []byte, filename, displayName string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if fileBytes != nil {
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("CreateFormFile() error = %v", err)
		}
		if _, err := part.Write(fileBytes); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}
	if displayName != "" {
		if err := w.WriteField("displayName", displayName); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestUploadAudioAsset_ValidWAVSucceeds(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	req := multipartUploadRequest(t, "/api/audio-assets", testWAVBytes(4410), "chime.wav", "Coin chime")
	resp := ts.do(t, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	body := decodeAudioAssetBody(t, resp)
	if body["id"] == "" || body["id"] == nil {
		t.Error("expected a non-empty server-generated id")
	}
	if body["mediaType"] != "audio/wav" {
		t.Errorf("mediaType = %v, want audio/wav", body["mediaType"])
	}
	if body["displayName"] != "Coin chime" {
		t.Errorf("displayName = %v, want %q", body["displayName"], "Coin chime")
	}
	if _, hasPath := body["storageName"]; hasPath {
		t.Error("response must never expose a local storage path")
	}
}

func TestUploadAudioAsset_RejectsUnsupportedFormat(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	mp3 := []byte{0xFF, 0xFB, 0x90, 0x00, 0, 0, 0, 0}
	req := multipartUploadRequest(t, "/api/audio-assets", mp3, "song.mp3", "")
	resp := ts.do(t, req)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	body := decodeAudioAssetBody(t, resp)
	if body["error"] != "audio_asset_unsupported" {
		t.Errorf("error = %v, want audio_asset_unsupported", body["error"])
	}
}

func TestUploadAudioAsset_RejectsMissingFile(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	req := multipartUploadRequest(t, "/api/audio-assets", nil, "", "no file here")
	resp := ts.do(t, req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUploadAudioAsset_RejectsUnrecognizedFormField(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("unexpectedField", "value"); err != nil {
		t.Fatalf("WriteField() error = %v", err)
	}
	part, _ := w.CreateFormFile("file", "chime.wav")
	_, _ = part.Write(testWAVBytes(100))
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/audio-assets", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp := ts.do(t, req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestListAndGetAudioAsset(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	uploadReq := multipartUploadRequest(t, "/api/audio-assets", testWAVBytes(100), "a.wav", "A")
	uploaded := decodeAudioAssetBody(t, ts.do(t, uploadReq))
	id, _ := uploaded["id"].(string)

	listResp := ts.do(t, httptest.NewRequest(http.MethodGet, "/api/audio-assets", nil))
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listResp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("list length = %d, want 1", len(items))
	}

	getResp := ts.do(t, httptest.NewRequest(http.MethodGet, "/api/audio-assets/"+id, nil))
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}

	notFoundResp := ts.do(t, httptest.NewRequest(http.MethodGet, "/api/audio-assets/audioasset_doesnotexist", nil))
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get unknown id status = %d, want 404", notFoundResp.StatusCode)
	}
}

func TestDeleteAudioAsset_SucceedsWhenUnreferenced(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	uploadReq := multipartUploadRequest(t, "/api/audio-assets", testWAVBytes(100), "a.wav", "")
	uploaded := decodeAudioAssetBody(t, ts.do(t, uploadReq))
	id, _ := uploaded["id"].(string)

	delResp := ts.do(t, httptest.NewRequest(http.MethodDelete, "/api/audio-assets/"+id, nil))
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", delResp.StatusCode)
	}

	getResp := ts.do(t, httptest.NewRequest(http.MethodGet, "/api/audio-assets/"+id, nil))
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", getResp.StatusCode)
	}
}

func TestDeleteAudioAsset_RejectsWhenReferenced(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	uploadReq := multipartUploadRequest(t, "/api/audio-assets", testWAVBytes(100), "a.wav", "")
	uploaded := decodeAudioAssetBody(t, ts.do(t, uploadReq))
	id, _ := uploaded["id"].(string)

	ts.seedAlertRule(t, "rule_1")
	if err := ts.svc.SetRuleAssetRefs(context.Background(), "rule_1", []string{id}); err != nil {
		t.Fatalf("SetRuleAssetRefs() error = %v", err)
	}

	delResp := ts.do(t, httptest.NewRequest(http.MethodDelete, "/api/audio-assets/"+id, nil))
	if delResp.StatusCode != http.StatusConflict {
		t.Fatalf("delete status = %d, want 409", delResp.StatusCode)
	}
	body := decodeAudioAssetBody(t, delResp)
	if body["error"] != "audio_asset_in_use" {
		t.Errorf("error = %v, want audio_asset_in_use", body["error"])
	}
}

func TestAudioAssetRoutes_MethodNotAllowed(t *testing.T) {
	ts := newAudioAssetTestServer(t)
	resp := ts.do(t, httptest.NewRequest(http.MethodPatch, "/api/audio-assets", nil))
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") == "" {
		t.Error("expected an Allow header on 405")
	}
}
