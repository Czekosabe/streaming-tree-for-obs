package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/account"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualpackage"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
	bus "github.com/streaming-tree/server/internal/engagement"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// assetTestServer is a Stage 14B-focused twin of templateTestServer
// (visualtemplate_test.go) - it additionally wires VisualAssets/
// VisualPackages, which the template-only harness never needed.
type assetTestServer struct {
	handler   http.Handler
	assetSvc  *visualasset.Service
	pkgSvc    *visualpackage.Service
	tmplSvc   *visualtemplate.Service
	alertsMgr *alerts.Manager
}

func newAssetTestServer(t *testing.T) *assetTestServer {
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

	provider := fakeHTTPProvider{}
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(),
		Providers:      map[account.ProviderID]account.Provider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})

	store := visualasset.NewFileStore(filepath.Join(t.TempDir(), "assets"))
	if err := store.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}
	assetSvc := visualasset.NewService(sqlite.NewVisualAssetRepository(db.DB), store, nil)

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	domainSvc := alerts.NewDomainService(sqlite.NewAlertsRepository(db.DB), accounts, nil, nil)
	visualDesignSvc := alerts.NewVisualDesignService(sqlite.NewVisualDesignRepository(db.DB))
	alertsManager := alerts.NewManager(alerts.ManagerOptions{
		DomainService: domainSvc, VisualDesignService: visualDesignSvc, AssetService: assetSvc, Bus: eventBus,
	})
	if err := alertsManager.Start(ctx); err != nil {
		t.Fatalf("alertsManager.Start() error = %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = alertsManager.Shutdown(c)
	})

	projection := oc.New(oc.Options{Source: eventBus, Capacity: 100})
	if err := projection.Start(ctx); err != nil {
		t.Fatalf("projection.Start() error = %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		projection.Shutdown(c)
	})

	chatProfiles := chatoverlaydomain.NewService(sqlite.NewChatOverlayRepository(db.DB), nil)
	chatVisualDesigns := visualdesign.NewService(sqlite.NewVisualDesignRepository(db.DB), nil)
	chatResolver := &co.DefaultSettingsResolver{
		Profiles: chatProfiles, AccountLabel: func(string) (string, bool) { return "", false }, VisualDesigns: chatVisualDesigns,
	}
	chatRuntime := co.NewManager(co.WrapOperatorChatSource(projection), chatResolver, chatVisualDesigns, logger)
	chatRuntime.SetAssetService(assetSvc)
	if err := chatRuntime.Start(ctx); err != nil {
		t.Fatalf("chatRuntime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		chatRuntime.Shutdown(c)
	})

	templateSvc, err := visualtemplate.NewService(sqlite.NewVisualTemplateRepository(db.DB), visualtemplate.DefaultBuiltins(), nil)
	if err != nil {
		t.Fatalf("visualtemplate.NewService() error = %v", err)
	}
	templateSvc.SetAssetService(assetSvc)

	pkgSvc := visualpackage.NewService(assetSvc, templateSvc, nil)

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts, Alerts: alertsManager,
		ChatOverlayProfiles: chatProfiles, ChatOverlayRuntime: chatRuntime,
		VisualTemplates: templateSvc, VisualAssets: assetSvc, VisualPackages: pkgSvc,
	})
	return &assetTestServer{handler: handler, assetSvc: assetSvc, pkgSvc: pkgSvc, tmplSvc: templateSvc, alertsMgr: alertsManager}
}

func (ts *assetTestServer) do(t *testing.T, method, path string, body any) *http.Response {
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

func (ts *assetTestServer) doRaw(t *testing.T, method, path, contentType string, body []byte) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeAssetBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func decodeAssetList(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response list: %v", err)
	}
	return out
}

func testPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

// buildUploadBody builds a multipart/form-data body with a single "file"
// part plus optional metadata fields, returning the body and its own
// Content-Type header (including the boundary).
func buildUploadBody(t *testing.T, filename string, data []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%q) error = %v", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

func uploadTestPNG(t *testing.T, ts *assetTestServer) map[string]any {
	t.Helper()
	body, contentType := buildUploadBody(t, "badge.png", testPNGBytes(t), map[string]string{"displayName": "Badge"})
	resp := ts.doRaw(t, http.MethodPost, "/api/visual-assets", contentType, body)
	if resp.StatusCode != http.StatusCreated {
		b := decodeAssetBody(t, resp)
		t.Fatalf("upload status = %d, want 201, body = %+v", resp.StatusCode, b)
	}
	return decodeAssetBody(t, resp)
}

// --- tests ----------------------------------------------------------------

func TestUploadVisualAsset(t *testing.T) {
	ts := newAssetTestServer(t)
	asset := uploadTestPNG(t, ts)
	id, _ := asset["id"].(string)
	if id == "" || len(id) < 6 || id[:6] != "asset_" {
		t.Fatalf("expected an asset_-prefixed id, got %v", asset["id"])
	}
	if asset["kind"] != "image" {
		t.Errorf("kind = %v, want image", asset["kind"])
	}
	url, _ := asset["url"].(string)
	if url == "" {
		t.Fatal("expected a non-empty public url")
	}
}

func TestUploadVisualAsset_RejectsWrongContentType(t *testing.T) {
	ts := newAssetTestServer(t)
	resp := ts.doRaw(t, http.MethodPost, "/api/visual-assets", "application/json", []byte("{}"))
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestUploadVisualAsset_RejectsSVGMasqueradingAsPNG(t *testing.T) {
	ts := newAssetTestServer(t)
	body, contentType := buildUploadBody(t, "evil.png", []byte("<svg onload=alert(1)></svg>"), nil)
	resp := ts.doRaw(t, http.MethodPost, "/api/visual-assets", contentType, body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b := decodeAssetBody(t, resp)
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, b)
	}
	if decodeAssetBody(t, resp)["error"] != "visual_asset_unsupported" {
		t.Errorf("unexpected error code: %+v", decodeAssetBody(t, resp))
	}
}

func TestListGetUpdateDeleteVisualAsset(t *testing.T) {
	ts := newAssetTestServer(t)
	asset := uploadTestPNG(t, ts)
	id := asset["id"].(string)

	list := decodeAssetList(t, ts.do(t, http.MethodGet, "/api/visual-assets", nil))
	if len(list) != 1 {
		t.Fatalf("expected 1 listed asset, got %d", len(list))
	}

	got := decodeAssetBody(t, ts.do(t, http.MethodGet, "/api/visual-assets/"+id, nil))
	if got["id"] != id {
		t.Fatalf("Get id mismatch: %+v", got)
	}

	updated := decodeAssetBody(t, ts.do(t, http.MethodPut, "/api/visual-assets/"+id, map[string]any{
		"displayName": "New name", "author": "Me", "license": "CC0", "notice": "",
	}))
	if updated["displayName"] != "New name" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	del := ts.do(t, http.MethodDelete, "/api/visual-assets/"+id, nil)
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", del.StatusCode)
	}

	notFound := ts.do(t, http.MethodGet, "/api/visual-assets/"+id, nil)
	if notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("get-after-delete status = %d, want 404", notFound.StatusCode)
	}
}

func TestVisualAssetRoutes_WrongMethod(t *testing.T) {
	ts := newAssetTestServer(t)
	resp := ts.do(t, http.MethodPatch, "/api/visual-assets", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow == "" {
		t.Error("expected an Allow header")
	}
}

func TestPublicVisualAsset_ServesContentWithRangeSupport(t *testing.T) {
	ts := newAssetTestServer(t)
	asset := uploadTestPNG(t, ts)
	url, _ := asset["url"].(string)

	full := ts.do(t, http.MethodGet, url, nil)
	if full.StatusCode != http.StatusOK {
		t.Fatalf("full request status = %d, want 200", full.StatusCode)
	}
	fullBody, _ := io.ReadAll(full.Body)
	full.Body.Close()
	if ct := full.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if full.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if full.Header.Get("Cache-Control") == "" {
		t.Errorf("missing Cache-Control header")
	}

	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Range", "bytes=0-3")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	rangeResp := rec.Result()
	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range request status = %d, want 206", rangeResp.StatusCode)
	}
	rangeBody, _ := io.ReadAll(rangeResp.Body)
	if len(rangeBody) != 4 {
		t.Errorf("range body length = %d, want 4", len(rangeBody))
	}
	if !bytes.Equal(rangeBody, fullBody[:4]) {
		t.Errorf("range body does not match the start of the full body")
	}

	unknown := ts.do(t, http.MethodGet, "/api/public/visual-assets/does-not-exist", nil)
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want 404", unknown.StatusCode)
	}

	wrongMethod := ts.do(t, http.MethodPost, url, nil)
	if wrongMethod.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d, want 405", wrongMethod.StatusCode)
	}
}

func TestDeleteVisualAsset_RejectsWhenReferencedByADesign(t *testing.T) {
	ts := newAssetTestServer(t)
	asset := uploadTestPNG(t, ts)
	assetID := asset["id"].(string)

	profileID := createTemplateTestProfileFor(t, ts)
	ruleID := createTemplateTestFollowRuleFor(t, ts, profileID)

	doc := map[string]any{
		"version": 3,
		"canvas":  map[string]any{"width": 1920, "height": 1080, "transparent": true},
		"layers": []map[string]any{
			{
				"id": "layer_1", "name": "Badge", "kind": "image", "visible": true, "locked": false, "order": 0,
				"frame": map[string]any{"x": 0, "y": 0, "width": 100, "height": 100}, "opacity": 1,
				"image":               map[string]any{"assetId": assetID, "fit": "contain", "alt": ""},
				"entryAnimation":      "none",
				"exitAnimation":       "none",
				"animationDurationMs": 0,
			},
		},
	}
	saveResp := ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{
		"expectedRevision": 0, "document": doc,
	})
	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("save visual design status = %d, want 200, body = %+v", saveResp.StatusCode, decodeAssetBody(t, saveResp))
	}

	del := ts.do(t, http.MethodDelete, "/api/visual-assets/"+assetID, nil)
	if del.StatusCode != http.StatusConflict {
		t.Fatalf("delete-while-in-use status = %d, want 409", del.StatusCode)
	}
	delBody := decodeAssetBody(t, del)
	if delBody["error"] != "visual_asset_in_use" {
		t.Errorf("unexpected error code: %+v", delBody)
	}

	// The public alert profile config's own current-state stream is not
	// exercised here; instead confirm the public asset URL itself never
	// exposes the local asset id (docs/visual-template-packages.md §18).
	assetURL, _ := asset["url"].(string)
	if bytesContains(assetURL, assetID) {
		t.Errorf("public asset URL leaks the local asset id: %q", assetURL)
	}
}

func bytesContains(haystack, needle string) bool {
	return len(needle) > 0 && bytes.Contains([]byte(haystack), []byte(needle))
}

func createTemplateTestProfileFor(t *testing.T, ts *assetTestServer) string {
	t.Helper()
	created := decodeAssetBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	return created["id"].(string)
}

func createTemplateTestFollowRuleFor(t *testing.T, ts *assetTestServer, profileID string) string {
	t.Helper()
	created := decodeAssetBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody()))
	return created["id"].(string)
}
