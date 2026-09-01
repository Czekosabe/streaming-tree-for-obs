package streamsession

import (
	"reflect"
	"regexp"
	"testing"
)

// Stage 24D: the "prove it, do not just state it" content-exclusion
// proof docs/stream-session-history.md §0/§11 24D calls for, mirroring
// exactly the reflection-based structural scan Stage 23's own
// TestConfigStructurallyExcludesSecretShapedFields already established
// for secret-shaped fields - applied here to engagement-content-shaped
// field names instead. A chat message, a chatter's name, a donation
// message/donor name/amount, membership/Super Chat content, an alert
// payload, or TTS text has no field to be assigned to anywhere in this
// package's own data model: not merely undocumented, structurally
// impossible. `DisplayName` (a destination's own operator-configured
// name, e.g. "My Twitch Channel" - an operational identifier this
// feature already documents storing, docs/stream-session-history.md
// §3, never viewer-supplied content) does not match this denylist at
// all, so no exception needs carving out for it.

var engagementContentShapedNamePattern = regexp.MustCompile(
	`(?i)(chat|message|donat|donor|subscri|viewer|superchat|super_chat|membership|tts|alertpayload|alert_payload)`,
)

func assertNoEngagementContentShapedFields(t *testing.T, v reflect.Type, path string, seen map[reflect.Type]bool) {
	t.Helper()
	if seen[v] {
		return
	}
	seen[v] = true

	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldPath := path + "." + field.Name
			if engagementContentShapedNamePattern.MatchString(field.Name) {
				t.Errorf("field %s is engagement-content-shaped (matches the chat/message/donation/subscriber/viewer/TTS/alert-payload denylist) - this package must never carry viewer content", fieldPath)
			}
			assertNoEngagementContentShapedFields(t, field.Type, fieldPath, seen)
		}
	case reflect.Ptr, reflect.Slice, reflect.Array:
		assertNoEngagementContentShapedFields(t, v.Elem(), path, seen)
	case reflect.Map:
		assertNoEngagementContentShapedFields(t, v.Key(), path, seen)
		assertNoEngagementContentShapedFields(t, v.Elem(), path, seen)
	}
}

func TestSessionAndDestinationStructurallyExcludeEngagementContent(t *testing.T) {
	assertNoEngagementContentShapedFields(t, reflect.TypeOf(Session{}), "Session", map[reflect.Type]bool{})
	assertNoEngagementContentShapedFields(t, reflect.TypeOf(Destination{}), "Destination", map[reflect.Type]bool{})
}

// A second, independent proof at the wire level: even if a future
// change added a stray field to Session/Destination, this locks in
// that the CURRENT JSON shape (what the HTTP API and the database
// columns actually carry) has exactly the fields the contract
// documents - no more.
func TestDestinationHasNoFieldBeyondTheDocumentedOperationalSnapshot(t *testing.T) {
	want := map[string]bool{
		"ID": true, "SessionID": true, "PlatformID": true, "ProviderID": true, "DisplayName": true,
		"StartedAt": true, "EndedAt": true, "Outcome": true, "CreatedAt": true, "UpdatedAt": true,
	}
	typ := reflect.TypeOf(Destination{})
	if typ.NumField() != len(want) {
		t.Fatalf("Destination has %d fields, want exactly %d (%v) - a new field means an explicit content-exclusion review is required", typ.NumField(), len(want), want)
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !want[name] {
			t.Errorf("Destination has an undocumented field %q", name)
		}
	}
}
