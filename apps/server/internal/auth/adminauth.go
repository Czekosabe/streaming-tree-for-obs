package auth

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/secrets"
)

// adminPasswordKey is the fixed SecretStore key the single
// administrator verifier is stored under (docs/remote-management.md
// §9.1/§16) - reuses the existing SecretStore abstraction, no new
// persistence mechanism.
func adminPasswordKey() string {
	return secrets.BuildKey(secrets.SecretTypeAdminPassword, secrets.AdminPasswordSubjectID)
}

// AdminAuthenticator is the concrete implementation of
// httpapi.AdminAuthService: it verifies the single administrator's
// password against the verifier stored in store. Never returns the
// verifier itself to any caller.
type AdminAuthenticator struct {
	Store secrets.SecretStore
}

// VerifyPassword reports whether password matches the stored
// verifier. A missing verifier (never provisioned) is treated as "no
// match" rather than an error - run()'s own fail-closed startup check
// (AdminPasswordProvisioned) is what prevents remote management from
// ever starting in that state at all, so this path exists mainly for
// defense in depth and for tests that construct the type directly.
func (a AdminAuthenticator) VerifyPassword(ctx context.Context, password string) (bool, error) {
	raw, err := a.Store.Get(ctx, adminPasswordKey())
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	ok, err := VerifyPassword(password, string(raw))
	if err != nil {
		// A malformed stored verifier is a corrupted-state condition,
		// never surfaced as "wrong password" to an HTTP caller (that
		// would be misleading) - the caller logs this and answers a
		// generic 500, per docs/remote-management.md §9.
		return false, err
	}
	return ok, nil
}

// SetAdminPassword hashes password and stores its verifier, replacing
// any previous one - used only by the local
// --provision-admin-password CLI mode (docs/remote-management.md
// §9.2), never by any HTTP route.
func SetAdminPassword(ctx context.Context, store secrets.SecretStore, password string) error {
	verifier, err := HashPassword(password)
	if err != nil {
		return err
	}
	return store.Set(ctx, adminPasswordKey(), []byte(verifier))
}

// AdminPasswordProvisioned reports whether an administrator verifier
// already exists - used by run()'s fail-closed startup check
// (docs/remote-management.md §5): remote management never starts with
// no way to authenticate.
func AdminPasswordProvisioned(ctx context.Context, store secrets.SecretStore) (bool, error) {
	return store.Exists(ctx, adminPasswordKey())
}
