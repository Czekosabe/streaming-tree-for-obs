// Package userdatapurge implements the explicit, operator-opt-in
// "remove all Streaming Tree settings, local data, and saved
// credentials" uninstall path (docs/windows-packaging.md §26).
//
// This is deliberately its own small, narrowly-scoped package rather
// than logic embedded directly in cmd/server: it must be reachable
// from a thin CLI entry point (`-purge-user-data`, invoked only by the
// Windows uninstaller's own [UninstallRun] entry) while staying unit-
// testable the normal way, matching this codebase's existing pattern
// of keeping cmd/server's own flag handlers thin wrappers around a
// real internal package (see internal/updater's
// ValidateHelperArgs/RunHelper and cmd/server's own runUpdateHelper).
package userdatapurge

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// Purge permanently deletes every piece of data this application
// owns: every OS-credential-store entry a real destination stream
// key, connected-account OAuth token bundle, or donation-source token
// could have created (enumerated from the real database, using the
// exact same secrets.BuildKey namespacing every other part of this
// application already reads and writes those same entries with - not
// a second, parallel key-naming scheme), the two fixed-subject
// secrets (the administrator password, the remote-ingest publisher
// password), and finally the whole dataDir itself - which is where
// the database, managed visual/audio assets, the managed MediaMTX
// runtime, and updater staging data all live (see internal/config's
// own resolveDataDir and every filepath.Join(cfg.DataDir, ...) call
// site across this codebase - there is nothing this application ever
// writes outside of it).
//
// Callers must already have confirmed no other instance of the
// application is running (internal/runtime/singleinstance owns that
// check, platform-specific, deliberately not duplicated here) -
// deleting a database or credential-store entry a live process might
// still have open is never attempted by this package itself.
//
// A missing database file (a fresh install that was never actually
// used) is not an error - there is simply nothing to enumerate.
// Individual credential-store deletions are best-effort: a subject
// that never created a given secret type answering "not found" is the
// normal case, and even a genuine per-key deletion failure does not
// abort the overall purge, since the dataDir removal below is the
// operationally important, fully testable outcome. Only a failure to
// open an existing database, or to remove dataDir itself, is returned
// as an error.
func Purge(ctx context.Context, dataDir, databasePath string, store secrets.SecretStore) error {
	if _, statErr := os.Stat(databasePath); statErr == nil {
		db, dbErr := sqlite.Open(ctx, databasePath)
		if dbErr != nil {
			return fmt.Errorf("open database: %w", dbErr)
		}
		deleteKnownSecrets(ctx, db.DB, store)
		_ = db.Close()
	}

	deleteSecretBestEffort(ctx, store, secrets.BuildKey(secrets.SecretTypeAdminPassword, secrets.AdminPasswordSubjectID))
	deleteSecretBestEffort(ctx, store,
		secrets.BuildKey(secrets.SecretTypeRemoteIngestPublisherPassword, secrets.RemoteIngestPublisherSubjectID))

	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("remove data directory: %w", err)
	}
	return nil
}

// deleteKnownSecrets enumerates every real subject a stored credential
// could exist for, from the real database, and deletes each one's
// entry - never a wildcard/prefix scan of the OS credential store
// itself, per the operator's own explicit "audit actual key prefixes/
// targets rather than wildcard-deleting Credential Manager"
// requirement.
func deleteKnownSecrets(ctx context.Context, db *sql.DB, store secrets.SecretStore) {
	if platforms, err := sqlite.NewPlatformRepository(db).List(ctx); err == nil {
		for _, p := range platforms {
			deleteSecretBestEffort(ctx, store, secrets.BuildKey(secrets.SecretTypeDestinationStreamKey, p.ID))
		}
	}
	if accounts, err := sqlite.NewAccountRepository(db).ListAccounts(ctx); err == nil {
		for _, a := range accounts {
			deleteSecretBestEffort(ctx, store, secrets.BuildKey(secrets.SecretTypeOAuthTokenBundle, a.ID))
		}
	}
	if sources, err := sqlite.NewDonationSourceRepository(db).ListSources(ctx); err == nil {
		for _, s := range sources {
			deleteSecretBestEffort(ctx, store, secrets.BuildKey(secrets.SecretTypeDonationSourceToken, s.ID))
		}
	}
}

func deleteSecretBestEffort(ctx context.Context, store secrets.SecretStore, key string) {
	_ = store.Delete(ctx, key)
}
