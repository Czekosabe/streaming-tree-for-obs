package httpapi

import (
	"net/http"
	"testing"
)

func createTestVisualDesignRule(t *testing.T, ts *alertsTestServer, profileID string) string {
	t.Helper()
	body := validFollowRuleBody()
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body))
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("createTestVisualDesignRule: created rule missing id: %+v", created)
	}
	return id
}

func validDesignDocumentDTO() map[string]any {
	return map[string]any{
		"version": 1,
		"canvas":  map[string]any{"width": 1920, "height": 1080, "transparent": true},
		"layers": []map[string]any{
			{
				"id": "layer_1", "name": "Text", "kind": "text", "visible": true, "locked": false, "order": 0,
				"frame": map[string]any{"x": 10, "y": 10, "width": 400, "height": 100}, "opacity": 1,
				"text": map[string]any{
					"binding": "alert_rendered_text", "missingValueBehavior": "hide",
					"fontFamily": "system-ui", "fontSize": 32, "fontWeight": 700, "lineHeight": 1.2, "letterSpacing": 0,
					"textColor": "#FFFFFF", "horizontalAlign": "center", "verticalAlign": "middle",
					"outlineWidth": 0, "outlineColor": "#000000",
					"shadowEnabled": false, "shadowOffsetX": 0, "shadowOffsetY": 0, "shadowBlur": 0, "shadowColor": "#000000",
				},
				"entryAnimation": "fade", "exitAnimation": "fade", "animationDurationMs": 300,
			},
		},
	}
}

func TestGetVisualDesignReturnsAnUnpersistedDraftWhenNoneSaved(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	resp := ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	body := decodeAlertsBody(t, resp)
	if body["persisted"] != false {
		t.Errorf("persisted = %v, want false", body["persisted"])
	}
	if body["revision"] != float64(0) {
		t.Errorf("revision = %v, want 0", body["revision"])
	}
	doc, _ := body["document"].(map[string]any)
	if doc == nil {
		t.Fatal("document missing from draft response")
	}
	layers, _ := doc["layers"].([]any)
	if len(layers) == 0 {
		t.Error("draft document has no layers, want at least one representing the legacy presentation")
	}
}

func TestGetVisualDesignDraftIsDeterministic(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	first := decodeAlertsBody(t, ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil))
	second := decodeAlertsBody(t, ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil))
	firstDoc, _ := first["document"].(map[string]any)
	secondDoc, _ := second["document"].(map[string]any)
	firstLayers, _ := firstDoc["layers"].([]any)
	secondLayers, _ := secondDoc["layers"].([]any)
	if len(firstLayers) != len(secondLayers) {
		t.Fatalf("draft layer counts differ across repeated GETs: %d vs %d", len(firstLayers), len(secondLayers))
	}
	firstLayer, _ := firstLayers[0].(map[string]any)
	secondLayer, _ := secondLayers[0].(map[string]any)
	if firstLayer["id"] != secondLayer["id"] {
		t.Errorf("draft layer id differs across repeated GETs: %v vs %v", firstLayer["id"], secondLayer["id"])
	}
}

func TestGetVisualDesignDraftIsNeverPersisted(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	// Two GETs in a row, neither should have persisted anything.
	ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil)
	resp := decodeAlertsBody(t, ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil))
	if resp["persisted"] != false {
		t.Error("persisted = true after only GET calls, want false - opening the designer must never save anything")
	}
}

func TestSaveVisualDesignCreatesAtRevisionOne(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	body := map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()}
	resp := ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	saved := decodeAlertsBody(t, resp)
	if saved["persisted"] != true {
		t.Errorf("persisted = %v, want true", saved["persisted"])
	}
	if saved["revision"] != float64(1) {
		t.Errorf("revision = %v, want 1", saved["revision"])
	}
}

func TestSaveVisualDesignThenGetRoundTrips(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	body := map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()}
	ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", body)

	got := decodeAlertsBody(t, ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil))
	if got["persisted"] != true {
		t.Fatalf("persisted = %v, want true after saving", got["persisted"])
	}
	doc, _ := got["document"].(map[string]any)
	layers, _ := doc["layers"].([]any)
	if len(layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(layers))
	}
	layer, _ := layers[0].(map[string]any)
	if layer["id"] != "layer_1" {
		t.Errorf("layer id = %v, want layer_1", layer["id"])
	}
}

func TestSaveVisualDesignIncrementsRevisionOnReplace(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	first := decodeAlertsBody(t, ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design",
		map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()}))
	second := decodeAlertsBody(t, ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design",
		map[string]any{"expectedRevision": first["revision"], "document": validDesignDocumentDTO()}))
	if second["revision"] != float64(2) {
		t.Errorf("second revision = %v, want 2", second["revision"])
	}
}

func TestSaveVisualDesignReturnsConflictOnStaleRevision(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()})
	resp := ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["error"] != "visual_design_revision_conflict" {
		t.Errorf("error = %v, want visual_design_revision_conflict", body["error"])
	}
}

func TestSaveVisualDesignRejectsAnOffCanvasFrame(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	doc := validDesignDocumentDTO()
	layers := doc["layers"].([]map[string]any)
	layers[0]["frame"] = map[string]any{"x": 1900, "y": 0, "width": 400, "height": 100}
	resp := ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": doc})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestSaveVisualDesignRejectsAnUnavailableBindingForTheRulesEventType(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	// validFollowRuleBody() uses eventType "follow", which has no quantity.
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	doc := validDesignDocumentDTO()
	layers := doc["layers"].([]map[string]any)
	text := layers[0]["text"].(map[string]any)
	text["binding"] = "quantity"
	resp := ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": doc})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a quantity binding on a follow rule, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestSaveVisualDesignRejectsAnOversizedDocument(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	doc := validDesignDocumentDTO()
	layers := doc["layers"].([]map[string]any)
	huge := ""
	for i := 0; i < 501; i++ {
		huge += "a"
	}
	text := layers[0]["text"].(map[string]any)
	text["binding"] = "static"
	text["staticText"] = huge
	resp := ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": doc})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for overlong static text, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestDeleteVisualDesignReturnsRuleToLegacyMode(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()})
	deleteResp := ts.do(t, http.MethodDelete, "/api/alert-rules/"+ruleID+"/visual-design", nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", deleteResp.StatusCode)
	}

	got := decodeAlertsBody(t, ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil))
	if got["persisted"] != false {
		t.Errorf("persisted = %v after delete, want false (back to a generated legacy draft)", got["persisted"])
	}
}

func TestDeleteVisualDesignIsIdempotent(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)

	first := ts.do(t, http.MethodDelete, "/api/alert-rules/"+ruleID+"/visual-design", nil)
	if first.StatusCode != http.StatusNoContent {
		t.Fatalf("first delete status = %d, want 204", first.StatusCode)
	}
	second := ts.do(t, http.MethodDelete, "/api/alert-rules/"+ruleID+"/visual-design", nil)
	if second.StatusCode != http.StatusNoContent {
		t.Fatalf("second delete status = %d, want 204 (idempotent)", second.StatusCode)
	}
}

func TestVisualDesignRuleCascadeOnRuleDelete(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)
	ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()})

	deleteResp := ts.do(t, http.MethodDelete, "/api/alert-rules/"+ruleID, nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete rule status = %d, want 204", deleteResp.StatusCode)
	}

	getResp := ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil)
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET visual-design after rule delete status = %d, want 404 (the rule itself is gone)", getResp.StatusCode)
	}
}

func TestGetVisualDesignForUnknownRuleReturns404(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/alert-rules/alrule_missing/visual-design", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestVisualDesignNeverExposesRawEventSubOrTokenFields(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	ruleID := createTestVisualDesignRule(t, ts, profileID)
	ts.do(t, http.MethodPut, "/api/alert-rules/"+ruleID+"/visual-design", map[string]any{"expectedRevision": 0, "document": validDesignDocumentDTO()})

	got := decodeAlertsBody(t, ts.do(t, http.MethodGet, "/api/alert-rules/"+ruleID+"/visual-design", nil))
	for _, forbidden := range []string{"access_token", "refresh_token", "eventsub", "ownerKind", "ownerId"} {
		if _, ok := got[forbidden]; ok {
			t.Errorf("response contains forbidden top-level field %q", forbidden)
		}
	}
}
