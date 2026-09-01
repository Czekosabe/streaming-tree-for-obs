package metadatapreset

import (
	"reflect"
	"strings"
	"testing"
)

// secretShapedSubstrings names every substring this project's own real
// secret concepts are known by (internal/secrets' own key namespace,
// docs/metadata-presets.md §10/§40's own explicit denylist): a field
// name containing any of these, anywhere in the preset domain's own
// exported types, would be a structural red flag worth failing the
// build over - not something a UI omission alone could ever prove
// absent.
var secretShapedSubstrings = []string{
	"key", "token", "secret", "password", "credential", "auth",
	"cookie", "session", "capability", "clientsecret", "refresh",
}

// assertNoSecretShapedFields walks every exported field of typ
// (recursively into struct/map/slice element types actually reachable
// from it) and fails the test if any field name contains a secret-
// shaped substring. Case-insensitive, since Go export rules only
// constrain the first letter.
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
				t.Errorf("field %s has a secret-shaped name (contains %q) - metadata presets must never carry a credential-shaped field", fieldPath, bad)
			}
		}
		assertNoSecretShapedFields(t, field.Type, fieldPath)
	}
}

func TestPresetStructurallyExcludesSecretShapedFields(t *testing.T) {
	assertNoSecretShapedFields(t, reflect.TypeOf(Preset{}), "Preset")
	assertNoSecretShapedFields(t, reflect.TypeOf(CommonMetadata{}), "CommonMetadata")
	assertNoSecretShapedFields(t, reflect.TypeOf(ProviderMetadata{}), "ProviderMetadata")
	assertNoSecretShapedFields(t, reflect.TypeOf(CreateInput{}), "CreateInput")
	assertNoSecretShapedFields(t, reflect.TypeOf(UpdateInput{}), "UpdateInput")
}

// TestPresetFieldCountIsExactlyWhatDocsRecord is a deliberate tripwire:
// if a future change adds a new field to CommonMetadata or
// ProviderMetadata, this test fails and forces a conscious decision
// (and a docs/metadata-presets.md update) rather than a new field
// silently slipping in unreviewed - including, potentially, a
// credential-shaped one.
func TestPresetFieldCountIsExactlyWhatDocsRecord(t *testing.T) {
	if got := reflect.TypeOf(CommonMetadata{}).NumField(); got != 8 {
		t.Fatalf("CommonMetadata has %d fields, want exactly 8 (Title, Description, Tags, Language, Visibility, MatureContent, DVR, LatencyMode) - update docs/metadata-presets.md if this is a deliberate change", got)
	}
	if got := reflect.TypeOf(ProviderMetadata{}).NumField(); got != 2 {
		t.Fatalf("ProviderMetadata has %d fields, want exactly 2 (Category, CategoryID) - update docs/metadata-presets.md if this is a deliberate change", got)
	}
}
