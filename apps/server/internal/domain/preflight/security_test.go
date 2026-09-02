package preflight

import (
	"reflect"
	"strings"
	"testing"
)

// secretShapedSubstrings mirrors streamsetup/metadatapreset's own
// denylist (docs/stream-preflight.md §10): a readiness DTO must only
// ever report "configured/available" or "connected/reconnect
// required" - never a stream key, an OAuth token, or a client secret.
var secretShapedSubstrings = []string{
	"key", "token", "secret", "password", "credential", "auth",
	"cookie", "session", "capability", "clientsecret", "refresh",
}

func assertNoSecretShapedFields(t *testing.T, typ reflect.Type, path string) {
	t.Helper()

	switch typ.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array:
		assertNoSecretShapedFields(t, typ.Elem(), path)
		return
	case reflect.Map:
		assertNoSecretShapedFields(t, typ.Elem(), path)
		return
	case reflect.Struct:
		// fall through to field iteration below
	default:
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldPath := path + "." + field.Name
		lower := strings.ToLower(field.Name)
		for _, bad := range secretShapedSubstrings {
			if strings.Contains(lower, bad) {
				t.Errorf("field %s has a secret-shaped name (contains %q) - a preflight readiness report must never carry a credential-shaped field", fieldPath, bad)
			}
		}
		assertNoSecretShapedFields(t, field.Type, fieldPath)
	}
}

func TestReportStructurallyExcludesSecretShapedFields(t *testing.T) {
	assertNoSecretShapedFields(t, reflect.TypeOf(Report{}), "Report")
	assertNoSecretShapedFields(t, reflect.TypeOf(Finding{}), "Finding")
	assertNoSecretShapedFields(t, reflect.TypeOf(Action{}), "Action")
	assertNoSecretShapedFields(t, reflect.TypeOf(DestinationReadiness{}), "DestinationReadiness")
}
