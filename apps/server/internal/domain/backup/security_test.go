package backup

import (
	"reflect"
	"strings"
	"testing"
)

// secretShapedSubstrings mirrors metadatapreset's own denylist exactly
// (internal/domain/metadatapreset/security_test.go) - the same
// concepts are exactly what a backup must never carry either.
var secretShapedSubstrings = []string{
	"key", "token", "secret", "password", "credential", "auth",
	"cookie", "session", "capability", "clientsecret", "refresh",
}

// allowedSecretShapedFields lists the handful of real, audited fields
// that legitimately contain a denylisted substring without being a
// credential - each is individually justified here rather than
// weakening the denylist itself.
var allowedSecretShapedFields = map[string]string{
	// account.Account: Scopes is the *names* of OAuth scopes granted
	// (e.g. "chat:read") - never a token, never a secret value.
	"ConnectedAccountExport.Account.Scopes": "OAuth scope names, not a credential",
	// visualtemplate.Template / visualasset.Asset: Author is a plain
	// attribution string (docs/visual-template-packages.md), matched
	// only because "Author" contains the substring "auth".
	"Config.VisualTemplates.Author": "template attribution text, not authentication",
	"Config.VisualAssets.Author":    "asset attribution text, not authentication",
	// visualasset.Blob / audioasset.Blob: PublicToken is, per its own
	// migration's documented reasoning, "an unguessable locator, not a
	// credential" - the same Category-A classification
	// docs/backup-restore.md §3 gives chat/alert/widget public_slug
	// (matched only because "Token" contains the substring "token").
	"Config.VisualAssets.Blob.PublicToken": "unguessable content locator, not a bearer credential (docs/backup-restore.md §3)",
	"Config.AudioAssets.Blob.PublicToken":  "unguessable content locator, not a bearer credential (docs/backup-restore.md §3)",
	// streamsession: StreamSessionRetentionDays is a plain integer
	// retention-days preference (how long to keep operational history),
	// matched only because "Session" contains the substring "session" -
	// never a session token/cookie/credential of any kind.
	"Config.StreamSessionRetentionDays": "retention-days preference, not a session token or credential",
}

func assertNoSecretShapedFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
	t.Helper()

	switch typ.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array:
		assertNoSecretShapedFields(t, typ.Elem(), path, seen)
		return
	case reflect.Map:
		assertNoSecretShapedFields(t, typ.Elem(), path, seen)
		return
	case reflect.Struct:
		// fall through
	default:
		return
	}

	// Recursion guard: several domain structs are reachable from more
	// than one Config field (e.g. visualdesign.Record's own Document),
	// and time.Time would otherwise be walked repeatedly for no benefit.
	if seen[typ] {
		return
	}
	seen[typ] = true

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldPath := path + "." + field.Name
		lower := strings.ToLower(field.Name)
		for _, bad := range secretShapedSubstrings {
			if strings.Contains(lower, bad) {
				if _, allowed := allowedSecretShapedFields[fieldPath]; allowed {
					continue
				}
				t.Errorf("field %s has a secret-shaped name (contains %q) - a backup must never carry a credential-shaped field", fieldPath, bad)
			}
		}
		assertNoSecretShapedFields(t, field.Type, fieldPath, seen)
	}
}

// TestConfigStructurallyExcludesSecretShapedFields is the structural
// proof docs/backup-restore.md §2 promises: every field reachable from
// Config, across every domain it aggregates, is walked and checked
// against the same denylist Stage 22's own preset domain already
// established - not a claim only tested at the UI layer.
func TestConfigStructurallyExcludesSecretShapedFields(t *testing.T) {
	assertNoSecretShapedFields(t, reflect.TypeOf(Config{}), "Config", map[reflect.Type]bool{})
}
