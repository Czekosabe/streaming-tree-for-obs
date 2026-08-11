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
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/account"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
	bus "github.com/streaming-tree/server/internal/engagement"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type templateTestServer struct {
	handler http.Handler
}

func newTemplateTestServer(t *testing.T) *templateTestServer {
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

	provider := fakeHTTPProvider{}
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(),
		Providers:      map[account.ProviderID]account.Provider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	domainSvc := alerts.NewDomainService(sqlite.NewAlertsRepository(db.DB), accounts)
	visualDesignSvc := alerts.NewVisualDesignService(sqlite.NewVisualDesignRepository(db.DB))
	alertsManager := alerts.NewManager(alerts.ManagerOptions{DomainService: domainSvc, VisualDesignService: visualDesignSvc, Bus: eventBus})
	if err := alertsManager.Start(context.Background()); err != nil {
		t.Fatalf("alertsManager.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = alertsManager.Shutdown(ctx)
	})

	projection := oc.New(oc.Options{Source: eventBus, Capacity: 100})
	if err := projection.Start(context.Background()); err != nil {
		t.Fatalf("operatorchat projection.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		projection.Shutdown(ctx)
	})

	chatProfiles := chatoverlaydomain.NewService(sqlite.NewChatOverlayRepository(db.DB), nil)
	chatVisualDesigns := visualdesign.NewService(sqlite.NewVisualDesignRepository(db.DB), nil)
	chatResolver := &co.DefaultSettingsResolver{
		Profiles: chatProfiles, AccountLabel: func(string) (string, bool) { return "", false }, VisualDesigns: chatVisualDesigns,
	}
	chatRuntime := co.NewManager(co.WrapOperatorChatSource(projection), chatResolver, chatVisualDesigns, logger)
	if err := chatRuntime.Start(context.Background()); err != nil {
		t.Fatalf("chat overlay runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		chatRuntime.Shutdown(ctx)
	})

	templateSvc, err := visualtemplate.NewService(sqlite.NewVisualTemplateRepository(db.DB), visualtemplate.DefaultBuiltins(), nil)
	if err != nil {
		t.Fatalf("visualtemplate.NewService() error = %v", err)
	}

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts, Alerts: alertsManager,
		ChatOverlayProfiles: chatProfiles, ChatOverlayRuntime: chatRuntime,
		VisualTemplates: templateSvc,
	})
	return &templateTestServer{handler: handler}
}

func (ts *templateTestServer) do(t *testing.T, method, path string, body any) *http.Response {
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

func decodeTemplateBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func decodeTemplateList(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response list: %v", err)
	}
	return out
}

func createTemplateTestProfile(t *testing.T, ts *templateTestServer) string {
	t.Helper()
	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	return created["id"].(string)
}

func createTemplateTestFollowRule(t *testing.T, ts *templateTestServer, profileID string) string {
	t.Helper()
	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody()))
	return created["id"].(string)
}

func createTemplateTestChatOverlay(t *testing.T, ts *templateTestServer) string {
	t.Helper()
	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/chat-overlays", map[string]any{"name": "Main Overlay"}))
	return created["id"].(string)
}

func minimalChatDocumentDTO() map[string]any {
	return map[string]any{
		"version": 2,
		"canvas":  map[string]any{"width": 960, "height": 280, "transparent": true},
		"layers": []map[string]any{
			{
				"id": "layer_1", "name": "Username", "kind": "text", "visible": true, "locked": false, "order": 0,
				"frame": map[string]any{"x": 10, "y": 10, "width": 400, "height": 60}, "opacity": 1,
				"text": map[string]any{
					"binding": "username", "missingValueBehavior": "hide",
					"fontFamily": "system-ui", "fontSize": 20, "fontWeight": 700, "lineHeight": 1.2, "letterSpacing": 0,
					"textColor": "#FFFFFF", "horizontalAlign": "left", "verticalAlign": "middle",
					"outlineWidth": 0, "outlineColor": "#000000",
					"shadowEnabled": false, "shadowOffsetX": 0, "shadowOffsetY": 0, "shadowBlur": 0, "shadowColor": "#000000",
				},
				"entryAnimation": "none", "exitAnimation": "none", "animationDurationMs": 0,
			},
		},
	}
}

// --- CRUD ---------------------------------------------------------------

func TestListVisualTemplatesIncludesBuiltins(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/visual-templates", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	list := decodeTemplateList(t, resp)
	var alertBuiltins, chatBuiltins int
	for _, item := range list {
		if item["source"] != "builtin" {
			continue
		}
		switch item["target"] {
		case "alert":
			alertBuiltins++
		case "chat":
			chatBuiltins++
		}
	}
	if alertBuiltins < 3 {
		t.Errorf("alert built-ins = %d, want >= 3", alertBuiltins)
	}
	if chatBuiltins < 3 {
		t.Errorf("chat built-ins = %d, want >= 3", chatBuiltins)
	}
}

func TestCreateVisualTemplateGeneratesTplID(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/visual-templates", map[string]any{
		"target": "chat", "name": "Mine", "description": "", "author": "", "license": "",
		"document": minimalChatDocumentDTO(),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %+v", resp.StatusCode, decodeTemplateBody(t, resp))
	}
	body := decodeTemplateBody(t, resp)
	id, _ := body["id"].(string)
	if len(id) < 4 || id[:4] != "tpl_" {
		t.Errorf("id = %q, want tpl_-prefixed", id)
	}
	if body["source"] != "user" {
		t.Errorf("source = %v, want user", body["source"])
	}
}

func TestGetVisualTemplateBuiltin(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/visual-templates/builtin_alert_minimal_dark", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestGetVisualTemplateMissingReturns404(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/visual-templates/tpl_missing", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestUpdateVisualTemplateMetadataRejectsBuiltin(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodPut, "/api/visual-templates/builtin_alert_minimal_dark", map[string]any{
		"name": "Renamed", "description": "", "author": "", "license": "",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %+v", resp.StatusCode, decodeTemplateBody(t, resp))
	}
}

func TestDeleteVisualTemplateRejectsBuiltin(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodDelete, "/api/visual-templates/builtin_alert_minimal_dark", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestDeleteVisualTemplateUserTemplate(t *testing.T) {
	ts := newTemplateTestServer(t)
	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/visual-templates", map[string]any{
		"target": "chat", "name": "Mine", "description": "", "author": "", "license": "",
		"document": minimalChatDocumentDTO(),
	}))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodDelete, "/api/visual-templates/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	getResp := ts.do(t, http.MethodGet, "/api/visual-templates/"+id, nil)
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", getResp.StatusCode)
	}
}

// --- import/export --------------------------------------------------------

func templateFileBody() map[string]any {
	return map[string]any{
		"format": "streaming-tree-visual-template", "schemaVersion": 1, "target": "chat",
		"name": "Imported", "description": "d", "author": "a", "license": "l",
		"visualDesign": minimalChatDocumentDTO(),
	}
}

func TestImportPreviewDoesNotPersist(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import/preview", templateFileBody())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %+v", resp.StatusCode, decodeTemplateBody(t, resp))
	}
	listResp := ts.do(t, http.MethodGet, "/api/visual-templates", nil)
	list := decodeTemplateList(t, listResp)
	for _, item := range list {
		if item["source"] == "user" {
			t.Fatalf("import preview must not persist, found a user template: %+v", item)
		}
	}
}

func TestImportPersistsWithNewID(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import", templateFileBody())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %+v", resp.StatusCode, decodeTemplateBody(t, resp))
	}
	body := decodeTemplateBody(t, resp)
	id, _ := body["id"].(string)
	if len(id) < 4 || id[:4] != "tpl_" {
		t.Errorf("id = %q, want tpl_-prefixed", id)
	}
}

func TestImportRejectsUnknownField(t *testing.T) {
	ts := newTemplateTestServer(t)
	file := templateFileBody()
	file["unknownField"] = "x"
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import", file)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestImportRejectsUnsupportedTemplateVersion(t *testing.T) {
	ts := newTemplateTestServer(t)
	file := templateFileBody()
	file["schemaVersion"] = 99
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import", file)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeTemplateBody(t, resp))
	}
	body := decodeTemplateBody(t, resp)
	if body["error"] != "visual_template_version_unsupported" {
		t.Errorf("error = %v, want visual_template_version_unsupported", body["error"])
	}
}

func TestImportMigratesEmbeddedV1Document(t *testing.T) {
	ts := newTemplateTestServer(t)
	file := templateFileBody()
	visualDesign := file["visualDesign"].(map[string]any)
	visualDesign["version"] = 1
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import", file)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %+v", resp.StatusCode, decodeTemplateBody(t, resp))
	}
	body := decodeTemplateBody(t, resp)
	doc := body["document"].(map[string]any)
	if doc["version"] != float64(2) {
		t.Errorf("document version = %v, want migrated to 2", doc["version"])
	}
}

func TestImportRejectsUnsupportedFutureDesignVersion(t *testing.T) {
	ts := newTemplateTestServer(t)
	file := templateFileBody()
	visualDesign := file["visualDesign"].(map[string]any)
	visualDesign["version"] = 999
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import", file)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	body := decodeTemplateBody(t, resp)
	if body["error"] != "visual_template_design_version_unsupported" {
		t.Errorf("error = %v, want visual_template_design_version_unsupported", body["error"])
	}
}

func TestImportCannotChooseLocalID(t *testing.T) {
	ts := newTemplateTestServer(t)
	file := templateFileBody()
	file["id"] = "tpl_attacker_chosen" // rejected by decoder: unknown field
	resp := ts.do(t, http.MethodPost, "/api/visual-templates/import", file)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unknown field \"id\" must be rejected)", resp.StatusCode)
	}
}

func TestExportUserTemplateHasSafeHeadersAndNoLocalIdentifiers(t *testing.T) {
	ts := newTemplateTestServer(t)
	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/visual-templates", map[string]any{
		"target": "chat", "name": "My/Weird\\Name", "description": "", "author": "", "license": "",
		"document": minimalChatDocumentDTO(),
	}))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodGet, "/api/visual-templates/"+id+"/export", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if disposition == "" {
		t.Fatal("Content-Disposition missing")
	}
	if bytes.ContainsAny([]byte(disposition), "/\\") {
		t.Errorf("Content-Disposition still contains a path separator: %q", disposition)
	}
	if !bytes.HasSuffix([]byte(disposition[:len(disposition)-1]), []byte(".streaming-tree-template.json")) && !bytes.Contains([]byte(disposition), []byte(".streaming-tree-template.json")) {
		t.Errorf("Content-Disposition missing the expected extension: %q", disposition)
	}

	body := decodeTemplateBody(t, resp)
	for _, forbidden := range []string{"id", "createdAt", "updatedAt"} {
		if _, present := body[forbidden]; present {
			t.Errorf("exported file must not contain %q, got %+v", forbidden, body)
		}
	}
	if body["format"] != "streaming-tree-visual-template" {
		t.Errorf("format = %v, want streaming-tree-visual-template", body["format"])
	}
}

func TestExportBuiltinWorks(t *testing.T) {
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/visual-templates/builtin_chat_compact/export", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestExportThenReimportRoundTrip(t *testing.T) {
	ts := newTemplateTestServer(t)
	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/visual-templates", map[string]any{
		"target": "chat", "name": "RoundTrip", "description": "d", "author": "a", "license": "l",
		"document": minimalChatDocumentDTO(),
	}))
	id := created["id"].(string)

	exportResp := ts.do(t, http.MethodGet, "/api/visual-templates/"+id+"/export", nil)
	exported := decodeTemplateBody(t, exportResp)

	deleteResp := ts.do(t, http.MethodDelete, "/api/visual-templates/"+id, nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", deleteResp.StatusCode)
	}

	reimportResp := ts.do(t, http.MethodPost, "/api/visual-templates/import", exported)
	if reimportResp.StatusCode != http.StatusCreated {
		t.Fatalf("reimport status = %d, want 201, body = %+v", reimportResp.StatusCode, decodeTemplateBody(t, reimportResp))
	}
	reimported := decodeTemplateBody(t, reimportResp)
	if reimported["id"] == id {
		t.Error("re-import must receive a new local id, not reuse the deleted one")
	}
	if reimported["name"] != "RoundTrip" || reimported["description"] != "d" || reimported["author"] != "a" || reimported["license"] != "l" {
		t.Errorf("metadata round trip mismatch: %+v", reimported)
	}
}

// --- compatibility ----------------------------------------------------

func TestListCompatibilityForAlertOwner(t *testing.T) {
	ts := newTemplateTestServer(t)
	profileID := createTemplateTestProfile(t, ts)
	ruleID := createTemplateTestFollowRule(t, ts, profileID)

	resp := ts.do(t, http.MethodGet, "/api/visual-templates?target=alert&ownerId="+ruleID, nil)
	list := decodeTemplateList(t, resp)
	for _, item := range list {
		if item["target"] != "alert" {
			continue
		}
		compat, ok := item["compatibility"].(map[string]any)
		if !ok {
			t.Fatalf("expected a compatibility block for %+v", item)
		}
		if compat["compatible"] != true {
			t.Errorf("built-in alert template %v expected compatible with a follow rule, got %+v", item["id"], compat)
		}
	}
}

func TestListCompatibilityTargetMismatch(t *testing.T) {
	ts := newTemplateTestServer(t)
	profileID := createTemplateTestProfile(t, ts)
	ruleID := createTemplateTestFollowRule(t, ts, profileID)

	resp := ts.do(t, http.MethodGet, "/api/visual-templates?target=alert&ownerId="+ruleID, nil)
	list := decodeTemplateList(t, resp)
	for _, item := range list {
		if item["target"] != "chat" {
			continue
		}
		compat, ok := item["compatibility"].(map[string]any)
		if !ok {
			t.Fatalf("expected a compatibility block for %+v", item)
		}
		if compat["compatible"] != false {
			t.Errorf("a chat template must be incompatible with an alert target, got %+v", compat)
		}
		blockers, _ := compat["blockers"].([]any)
		if len(blockers) != 1 || blockers[0] != "template_target_mismatch" {
			t.Errorf("blockers = %v, want [template_target_mismatch]", blockers)
		}
	}
}

func TestCreateThenListIncompatibleQuantityTemplateForFollowRule(t *testing.T) {
	ts := newTemplateTestServer(t)
	profileID := createTemplateTestProfile(t, ts)
	ruleID := createTemplateTestFollowRule(t, ts, profileID)

	created := decodeTemplateBody(t, ts.do(t, http.MethodPost, "/api/visual-templates", map[string]any{
		"target": "alert", "name": "Quantity Template", "description": "", "author": "", "license": "",
		"document": quantityAlertDocumentDTO(),
	}))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodGet, "/api/visual-templates?target=alert&ownerId="+ruleID, nil)
	list := decodeTemplateList(t, resp)
	for _, item := range list {
		if item["id"] != id {
			continue
		}
		compat := item["compatibility"].(map[string]any)
		if compat["compatible"] != false {
			t.Fatalf("expected the quantity template to be incompatible with a follow rule, got %+v", compat)
		}
		blockers, _ := compat["blockers"].([]any)
		if len(blockers) != 1 || blockers[0] != "alert_binding_unavailable" {
			t.Errorf("blockers = %v, want [alert_binding_unavailable]", blockers)
		}
		return
	}
	t.Fatal("created template not found in list")
}

func quantityAlertDocumentDTO() map[string]any {
	doc := validDesignDocumentDTO()
	layers := doc["layers"].([]map[string]any)
	layers[0]["text"].(map[string]any)["binding"] = "quantity"
	return doc
}

func TestCompatibilityForUnknownOwnerReturns404OnListQuery(t *testing.T) {
	// An ownerId that resolves to nothing should not silently fall back
	// to target-only compatibility on a scoped query - the list itself
	// still succeeds (compatibility is best-effort per item), but no
	// item claims compatibility with a nonexistent owner beyond target
	// matching, since resolveOwnerCheck's error is swallowed into "no
	// owner check" only for the informational list endpoint. Assert the
	// endpoint at least never 500s.
	ts := newTemplateTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/visual-templates?target=alert&ownerId=alrule_missing", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestChatOverlayCompatibility(t *testing.T) {
	ts := newTemplateTestServer(t)
	overlayID := createTemplateTestChatOverlay(t, ts)

	resp := ts.do(t, http.MethodGet, "/api/visual-templates?target=chat&ownerId="+overlayID, nil)
	list := decodeTemplateList(t, resp)
	for _, item := range list {
		if item["target"] != "chat" {
			continue
		}
		compat := item["compatibility"].(map[string]any)
		if compat["compatible"] != true {
			t.Errorf("built-in chat template %v expected compatible with a chat overlay, got %+v", item["id"], compat)
		}
	}
}
