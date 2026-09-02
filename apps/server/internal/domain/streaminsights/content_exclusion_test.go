package streaminsights

import (
	"reflect"
	"regexp"
	"testing"
)

// Mirrors streamsession's own TestSessionAndDestinationStructurallyExcludeEngagementContent
// exactly - applied here to this domain's own aggregate types.
// DisplayName (a destination's own operator-configured name, snapshot
// inherited unchanged from streamsession.Destination) does not match
// this denylist, so no exception needs carving out for it.

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

func TestInsightsStructurallyExcludeEngagementContent(t *testing.T) {
	assertNoEngagementContentShapedFields(t, reflect.TypeOf(Insights{}), "Insights", map[reflect.Type]bool{})
	assertNoEngagementContentShapedFields(t, reflect.TypeOf(DestinationInsights{}), "DestinationInsights", map[reflect.Type]bool{})
	assertNoEngagementContentShapedFields(t, reflect.TypeOf(SessionSummary{}), "SessionSummary", map[reflect.Type]bool{})
}
