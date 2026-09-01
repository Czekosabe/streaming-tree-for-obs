package streamsetup

import (
	"reflect"
	"strings"
	"testing"
)

// secretShapedSubstrings mirrors internal/domain/metadatapreset's own
// denylist (docs/stream-setup-profiles.md §12): a field name containing
// any of these, anywhere in the setup-profile domain's own exported
// types, would be a structural red flag worth failing the build over -
// a setup profile must never carry a stream key, an OAuth token, a
// client secret, or any other credential-shaped field. This proves the
// model has no secret-semantic fields; it does not and cannot prove an
// arbitrary text field (like Note) was never manually pasted full of a
// secret by the user themselves.
var secretShapedSubstrings = []string{
	"key", "token", "secret", "password", "credential", "auth",
	"cookie", "session", "capability", "clientsecret", "refresh",
}

// assertNoSecretShapedFields walks every exported field of typ
// (recursively into pointer/slice/array/map element types actually
// reachable from it) and fails the test if any field name contains a
// secret-shaped substring. Case-insensitive, since Go export rules
// only constrain the first letter.
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
				t.Errorf("field %s has a secret-shaped name (contains %q) - stream setup profiles must never carry a credential-shaped field", fieldPath, bad)
			}
		}
		assertNoSecretShapedFields(t, field.Type, fieldPath)
	}
}

func TestProfileStructurallyExcludesSecretShapedFields(t *testing.T) {
	assertNoSecretShapedFields(t, reflect.TypeOf(Profile{}), "Profile")
	assertNoSecretShapedFields(t, reflect.TypeOf(Destination{}), "Destination")
	assertNoSecretShapedFields(t, reflect.TypeOf(CreateInput{}), "CreateInput")
	assertNoSecretShapedFields(t, reflect.TypeOf(UpdateInput{}), "UpdateInput")
}

// TestProfileFieldCountIsExactlyWhatDocsRecord is a deliberate tripwire:
// if a future change adds a new field to Profile or Destination, this
// test fails and forces a conscious decision (and a
// docs/stream-setup-profiles.md update) rather than a new field
// silently slipping in unreviewed - including, potentially, a
// credential-shaped one.
func TestProfileFieldCountIsExactlyWhatDocsRecord(t *testing.T) {
	if got := reflect.TypeOf(Profile{}).NumField(); got != 8 {
		t.Fatalf("Profile has %d fields, want exactly 8 (ID, Name, Note, Destinations, MetadataPresetID, MetadataPresetName, CreatedAt, UpdatedAt) - update docs/stream-setup-profiles.md if this is a deliberate change", got)
	}
	if got := reflect.TypeOf(Destination{}).NumField(); got != 3 {
		t.Fatalf("Destination has %d fields, want exactly 3 (PlatformID, ProviderID, DisplayName) - update docs/stream-setup-profiles.md if this is a deliberate change", got)
	}
}
