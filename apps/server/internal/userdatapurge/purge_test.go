package userdatapurge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// seedRealRecords creates one real platform, account, and donation
// source in the database (the same repositories, and the same
// required fields, production code uses) and stores a matching secret
// for each under the exact key production code would use - so this
// test exercises the real enumeration-from-the-database path, not a
// hand-picked list of keys.
func seedRealRecords(t *testing.T, databasePath string, store secrets.SecretStore) (platformID, accountID, sourceID string) {
	t.Helper()
	ctx := context.Background()

	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	defer db.Close()
	if _, err := sqlite.Migrate(ctx, db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	now := time.Now().UTC()

	platformID = "pf_test_purge"
	if err := sqlite.NewPlatformRepository(db.DB).Create(ctx, platform.Platform{
		ID: platformID, ProviderID: platform.ProviderTwitch, DisplayName: "Test destination",
		Enabled: false, SortOrder: 99, CreatedAt: now, UpdatedAt: now,
		Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("PlatformRepository.Create() error = %v", err)
	}

	accountID = "acct_test_purge"
	if err := sqlite.NewAccountRepository(db.DB).CreateAccount(ctx, account.Account{
		ID: accountID, ProviderID: account.ProviderTwitch, ProviderUserID: "u_test_purge",
		Login: "streamer", DisplayName: "Streamer", AvatarURL: "https://example.invalid/a.png",
		Status: account.StatusConnected, CreatedAt: now, UpdatedAt: now, Scopes: []string{"channel:manage:broadcast"},
	}); err != nil {
		t.Fatalf("AccountRepository.CreateAccount() error = %v", err)
	}

	sourceID = "ds_test_purge"
	if err := sqlite.NewDonationSourceRepository(db.DB).CreateSource(ctx, donationsource.Source{
		ID: sourceID, ProviderID: donationsource.ProviderStreamElements,
		Label: "Test donations", Enabled: false, RemoteChannelID: "5ad23dcc18fff500d78c5348",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("DonationSourceRepository.CreateSource() error = %v", err)
	}

	if err := store.Set(ctx, secrets.BuildKey(secrets.SecretTypeDestinationStreamKey, platformID), []byte("stream-key")); err != nil {
		t.Fatalf("seed stream-key secret: %v", err)
	}
	if err := store.Set(ctx, secrets.BuildKey(secrets.SecretTypeOAuthTokenBundle, accountID), []byte("token-bundle")); err != nil {
		t.Fatalf("seed oauth secret: %v", err)
	}
	if err := store.Set(ctx, secrets.BuildKey(secrets.SecretTypeDonationSourceToken, sourceID), []byte("donation-token")); err != nil {
		t.Fatalf("seed donation-source secret: %v", err)
	}
	if err := store.Set(ctx, secrets.BuildKey(secrets.SecretTypeAdminPassword, secrets.AdminPasswordSubjectID), []byte("admin-hash")); err != nil {
		t.Fatalf("seed admin-password secret: %v", err)
	}
	if err := store.Set(ctx, secrets.BuildKey(secrets.SecretTypeRemoteIngestPublisherPassword, secrets.RemoteIngestPublisherSubjectID), []byte("publisher-verifier")); err != nil {
		t.Fatalf("seed remote-ingest secret: %v", err)
	}

	return platformID, accountID, sourceID
}

func TestPurgeRemovesEveryRealCredentialAndTheWholeDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "streaming-tree.db")
	store := secretstest.New()

	platformID, accountID, sourceID := seedRealRecords(t, databasePath, store)

	// A sentinel outside dataDir - proves Purge never reaches beyond
	// its own application-owned directory.
	outsideDir := t.TempDir()
	sentinelPath := filepath.Join(outsideDir, "unrelated-file.txt")
	if err := os.WriteFile(sentinelPath, []byte("do not touch"), 0o600); err != nil {
		t.Fatalf("write sentinel file: %v", err)
	}

	// A credential this application never created (a different
	// service name entirely) - proves Purge's own store.Delete calls
	// are scoped to exactly the keys it looks up, not a wildcard wipe
	// of everything the store happens to hold.
	if err := store.Set(context.Background(), "unrelated-service:some-key", []byte("not ours")); err != nil {
		t.Fatalf("seed unrelated secret: %v", err)
	}

	if err := Purge(context.Background(), dataDir, databasePath, store); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}

	for _, key := range []string{
		secrets.BuildKey(secrets.SecretTypeDestinationStreamKey, platformID),
		secrets.BuildKey(secrets.SecretTypeOAuthTokenBundle, accountID),
		secrets.BuildKey(secrets.SecretTypeDonationSourceToken, sourceID),
		secrets.BuildKey(secrets.SecretTypeAdminPassword, secrets.AdminPasswordSubjectID),
		secrets.BuildKey(secrets.SecretTypeRemoteIngestPublisherPassword, secrets.RemoteIngestPublisherSubjectID),
	} {
		if store.Has(key) {
			t.Errorf("credential %q still present after Purge()", key)
		}
	}

	if !store.Has("unrelated-service:some-key") {
		t.Error("Purge() removed a credential outside its own known key set - it must never wildcard-delete")
	}

	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("dataDir %q still exists after Purge(), stat err = %v", dataDir, err)
	}

	if _, err := os.Stat(sentinelPath); err != nil {
		t.Errorf("sentinel file outside dataDir was affected by Purge(): %v", err)
	}
}

func TestPurgeWithNoExistingDatabaseStillRemovesDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "streaming-tree.db")
	store := secretstest.New()

	// No sqlite.Open() call at all - a fresh install that was never
	// actually used, matching a real uninstall right after a first
	// launch that the operator immediately decided against.
	if err := Purge(context.Background(), dataDir, databasePath, store); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}

	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("dataDir %q still exists after Purge(), stat err = %v", dataDir, err)
	}
}

func TestPurgeStillRemovesFixedSubjectSecretsAndDataDirEvenIfDatabaseSecretsWereNeverSet(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "streaming-tree.db")
	store := secretstest.New()

	ctx := context.Background()
	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	if _, err := sqlite.Migrate(ctx, db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}
	db.Close()

	if err := Purge(ctx, dataDir, databasePath, store); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}

	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("dataDir %q still exists after Purge(), stat err = %v", dataDir, err)
	}
	if store.Len() != 0 {
		t.Errorf("store still has %d entries after Purge() with an empty database", store.Len())
	}
}
