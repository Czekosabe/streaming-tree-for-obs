package backup_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/backup"
	"github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/onboarding"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// Stage 23F: real end-to-end security/integration proof, against a
// genuinely migrated SQLite database and the real repositories/
// FileStores production code uses - not the package-internal fakes
// export_test.go/archive_test.go/restore_commit_test.go already
// exercise the logic with. Mirrors internal/userdatapurge's own
// seedRealRecords precedent: real rows through the real repositories,
// real secrets under the exact production key format
// (secrets.BuildKey), so this test exercises the real "does a backup
// ever leak what's actually sitting in the database next to it"
// question, not a hand-picked list of fields.

// installation is one independently-wired backup.Service, backed by
// its own temp SQLite database and its own in-memory SecretStore -
// two of these in one test are two genuinely independent
// installations, never sharing any state.
type installation struct {
	svc          *backup.Service
	db           *sqlite.DB
	store        *secretstest.Store
	outputSvc    *output.Service
	platformRepo *sqlite.PlatformRepository
	visualStore  *visualasset.FileStore
	audioStore   *visualasset.FileStore
}

func newInstallation(t *testing.T) *installation {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()

	db, err := sqlite.Open(ctx, filepath.Join(dir, "streaming-tree.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := sqlite.Migrate(ctx, db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	platformRepo := sqlite.NewPlatformRepository(db.DB)
	outputSvc := output.NewService(sqlite.NewOutputRepository(db.DB))
	remoteTargetRepo := sqlite.NewRemoteTargetRepository(db.DB)
	accountRepo := sqlite.NewAccountRepository(db.DB)
	youtubeRegionRepo := sqlite.NewYouTubeRegionRepository(db.DB)
	engagementSettingsRepo := sqlite.NewEngagementSettingsRepository(db.DB)
	operatorChatPrefsRepo := sqlite.NewOperatorChatPrefsRepository(db.DB)
	chatOverlayRepo := sqlite.NewChatOverlayRepository(db.DB)
	chatAutomationRepo := sqlite.NewChatAutomationRepository(db.DB)
	alertsRepo := sqlite.NewAlertsRepository(db.DB)
	visualDesignRepo := sqlite.NewVisualDesignRepository(db.DB)
	visualTemplateRepo := sqlite.NewVisualTemplateRepository(db.DB)
	visualAssetRepo := sqlite.NewVisualAssetRepository(db.DB)
	audioAssetRepo := sqlite.NewAudioAssetRepository(db.DB)
	audioSettingsRepo := sqlite.NewAudioSettingsRepository(db.DB)
	goalsRepo := sqlite.NewGoalsRepository(db.DB)
	metadataPresetRepo := sqlite.NewMetadataPresetRepository(db.DB)
	streamSetupProfileRepo := sqlite.NewStreamSetupProfileRepository(db.DB)
	donationSourceRepo := sqlite.NewDonationSourceRepository(db.DB)
	updatePreferencesRepo := sqlite.NewUpdateSettingsRepository(db.DB)
	onboardingRepo := sqlite.NewOnboardingRepository(db.DB)
	streamSessionRepo := sqlite.NewStreamSessionRepository(db.DB)

	sources := backup.Sources{
		Platforms: platformRepo, Output: outputSvc, RemoteTarget: remoteTargetRepo, Accounts: accountRepo,
		YouTubeRegion: youtubeRegionRepo, EngagementSettings: engagementSettingsRepo,
		OperatorChatPrefs: operatorChatPrefsRepo, ChatOverlays: chatOverlayRepo,
		ChatAutomation: chatAutomationRepo, Alerts: alertsRepo,
		VisualDesigns: visualDesignRepo, VisualTemplates: visualTemplateRepo,
		VisualAssets: visualAssetRepo, AudioAssets: audioAssetRepo,
		AudioSettings: audioSettingsRepo, Goals: goalsRepo,
		MetadataPresets: metadataPresetRepo, StreamSetupProfiles: streamSetupProfileRepo,
		DonationSources:       donationSourceRepo,
		UpdatePreferences:     updatePreferencesRepo,
		StreamSessionSettings: streamSessionRepo,
	}
	sinks := backup.Sinks{
		Platforms: platformRepo, Output: outputSvc, RemoteTarget: remoteTargetRepo, Accounts: accountRepo,
		YouTubeRegion: youtubeRegionRepo, EngagementSettings: engagementSettingsRepo,
		OperatorChatPrefs: operatorChatPrefsRepo, ChatOverlays: chatOverlayRepo,
		ChatAutomation: chatAutomationRepo, Alerts: alertsRepo,
		VisualDesigns: visualDesignRepo, VisualTemplates: visualTemplateRepo,
		VisualAssets: visualAssetRepo, AudioAssets: audioAssetRepo,
		AudioSettings: audioSettingsRepo, Goals: goalsRepo,
		MetadataPresets: metadataPresetRepo, StreamSetupProfiles: streamSetupProfileRepo,
		DonationSources:       donationSourceRepo,
		UpdatePreferences:     updatePreferencesRepo,
		Onboarding:            onboardingRepo,
		StreamSessionSettings: streamSessionRepo,
	}

	visualStore := visualasset.NewFileStore(filepath.Join(dir, "assets", "visual"))
	audioStore := visualasset.NewFileStore(filepath.Join(dir, "assets", "audio"))
	if err := visualStore.EnsureDirs(); err != nil {
		t.Fatalf("visual FileStore.EnsureDirs() error = %v", err)
	}
	if err := audioStore.EnsureDirs(); err != nil {
		t.Fatalf("audio FileStore.EnsureDirs() error = %v", err)
	}
	staging, err := backup.NewFileStaging(filepath.Join(dir, "backup-staging"), backup.PreviewTTL)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}

	// sqlite.Migrate seeds a handful of demo destinations on a fresh
	// database (onboarding content) - removed here so this test's own
	// platform counts mean exactly what they say, not "N plus whatever
	// the migration happened to seed".
	seeded, err := platformRepo.List(ctx)
	if err != nil {
		t.Fatalf("list seeded platforms: %v", err)
	}
	for _, p := range seeded {
		if err := platformRepo.Delete(ctx, p.ID); err != nil {
			t.Fatalf("remove seeded demo platform %q: %v", p.ID, err)
		}
	}

	svc := backup.NewService(
		sources, sinks,
		visualStore, audioStore,
		visualStore, audioStore,
		staging, nil, nil,
		"0.1.0-test", "windows",
	)

	return &installation{
		svc: svc, db: db, store: secretstest.New(), outputSvc: outputSvc, platformRepo: platformRepo,
		visualStore: visualStore, audioStore: audioStore,
	}
}

// TestExportedBackupNeverContainsAnyRealSecretValue is the governing
// task's hermetic fixture scan (§29): a real installation with real
// non-secret configuration seeded next to real secrets in its own
// SecretStore (destination stream key, OAuth token bundle, donation-
// source token, plus the two singleton secrets a backup never even
// reads - admin password and the remote-ingest publisher password),
// each set to a unique sentinel value. A real Export/WriteArchive is
// then scanned byte-for-byte for every one of those sentinels - not a
// structural type check (security_test.go already covers that), a
// proof about actual bytes leaving the application.
func TestExportedBackupNeverContainsAnyRealSecretValue(t *testing.T) {
	inst := newInstallation(t)
	ctx := context.Background()
	now := time.Now().UTC()

	platformID := "pf_sentinel_test"
	if err := inst.platformRepo.Create(ctx, platform.Platform{
		ID: platformID, ProviderID: platform.ProviderTwitch, DisplayName: "Sentinel destination",
		Enabled: true, SortOrder: 1, CreatedAt: now, UpdatedAt: now,
		Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("PlatformRepository.Create() error = %v", err)
	}

	accountID := "acct_sentinel_test"
	accountRepo := sqlite.NewAccountRepository(inst.db.DB)
	if err := accountRepo.CreateAccount(ctx, account.Account{
		ID: accountID, ProviderID: account.ProviderTwitch, ProviderUserID: "u_sentinel",
		Login: "sentinelstreamer", DisplayName: "Sentinel Streamer", AvatarURL: "https://example.invalid/a.png",
		Status: account.StatusConnected, CreatedAt: now, UpdatedAt: now, Scopes: []string{"channel:manage:broadcast"},
	}); err != nil {
		t.Fatalf("AccountRepository.CreateAccount() error = %v", err)
	}

	sourceID := "ds_sentinel_test"
	if err := sqlite.NewDonationSourceRepository(inst.db.DB).CreateSource(ctx, donationsource.Source{
		ID: sourceID, ProviderID: donationsource.ProviderStreamElements,
		Label: "Sentinel donations", Enabled: false, RemoteChannelID: "5ad23dcc18fff500d78c5348",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("DonationSourceRepository.CreateSource() error = %v", err)
	}

	sentinels := map[string]struct {
		secretType secrets.SecretType
		subjectID  string
	}{
		"SENTINEL-STREAMKEY-6b2f9c1a4d":       {secrets.SecretTypeDestinationStreamKey, platformID},
		"SENTINEL-OAUTHBUNDLE-8e13a90fce":     {secrets.SecretTypeOAuthTokenBundle, accountID},
		"SENTINEL-DONATIONTOKEN-c4571bde33":   {secrets.SecretTypeDonationSourceToken, sourceID},
		"SENTINEL-ADMINPASSWORD-0a9d8e7c6b":   {secrets.SecretTypeAdminPassword, secrets.AdminPasswordSubjectID},
		"SENTINEL-REMOTEINGESTPUB-f1e2d3c4b5": {secrets.SecretTypeRemoteIngestPublisherPassword, secrets.RemoteIngestPublisherSubjectID},
	}
	for value, k := range sentinels {
		if err := inst.store.Set(ctx, secrets.BuildKey(k.secretType, k.subjectID), []byte(value)); err != nil {
			t.Fatalf("seed secret %s: %v", value, err)
		}
	}

	data, err := inst.svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	for value := range sentinels {
		if bytesContainString(data, value) {
			t.Errorf("exported backup archive contains the secret sentinel %q - a real credential value leaked into the backup", value)
		}
	}

	// Sanity check: the fixture actually exercised something real, not
	// an accidentally-empty export.
	validated, err := backup.ReadArchive(data)
	if err != nil {
		t.Fatalf("ReadArchive() on the real export error = %v", err)
	}
	if len(validated.Config.Platforms) != 1 || len(validated.Config.ConnectedAccounts) != 1 || len(validated.Config.DonationSources) != 1 {
		t.Fatalf("fixture did not round-trip as expected: %+v", validated.Config)
	}
}

func bytesContainString(data []byte, needle string) bool {
	return indexOf(data, []byte(needle)) >= 0
}

func indexOf(haystack, needle []byte) int {
	n, m := len(haystack), len(needle)
	if m == 0 || m > n {
		return -1
	}
	for i := 0; i+m <= n; i++ {
		if string(haystack[i:i+m]) == string(needle) {
			return i
		}
	}
	return -1
}

// TestRestoreIntoAnIndependentInstallationNeverAdoptsItsPreExisting
// Secret is the governing task's §31 secret-collision attack test -
// explicitly release-blocking - and doubles as its §30 portable-
// restore proof (two genuinely independent installations, each with
// its own database and its own SecretStore).
//
// Installation B already has a real platform with a real stream-key
// secret under secrets.BuildKey(SecretTypeDestinationStreamKey,
// "pf_victim"). A crafted backup claims that exact same platform id.
// If restore ever reused a backup-supplied id as a literal local
// primary key, the newly restored platform would resolve to B's real
// pre-existing secret through nothing but a matching id string. This
// proves it cannot: every restored object always gets a freshly
// minted local id (docs/backup-restore.md §4), so the crafted id is
// never adopted, B's original secret is left completely untouched,
// and the new platform's own credential status genuinely reads as
// "not configured" - it can never silently inherit the victim's key.
func TestRestoreIntoAnIndependentInstallationNeverAdoptsItsPreExistingSecret(t *testing.T) {
	b := newInstallation(t)
	ctx := context.Background()
	now := time.Now().UTC()

	victimID := "pf_victim"
	if err := b.platformRepo.Create(ctx, platform.Platform{
		ID: victimID, ProviderID: platform.ProviderTwitch, DisplayName: "B's real destination",
		Enabled: true, SortOrder: 1, CreatedAt: now, UpdatedAt: now,
		Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("seed installation B's real platform: %v", err)
	}
	victimSecretKey := secrets.BuildKey(secrets.SecretTypeDestinationStreamKey, victimID)
	const victimSecret = "VICTIM-REAL-STREAM-KEY-do-not-leak"
	if err := b.store.Set(ctx, victimSecretKey, []byte(victimSecret)); err != nil {
		t.Fatalf("seed installation B's real secret: %v", err)
	}

	// The malicious/coincidental backup: built independently (never
	// derived from B in any way), claiming the exact same platform id
	// B already uses.
	malicious := backup.Config{
		FormatVersion: backup.FormatVersion,
		Platforms: []backup.PlatformExport{
			{Platform: platform.Platform{
				ID: victimID, ProviderID: platform.ProviderTwitch, DisplayName: "Attacker-crafted",
				Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
			}},
		},
	}
	archive, err := backup.WriteArchive(malicious, "0.1.0-test", "windows", now, noAssets{}, noAssets{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	preview, err := b.svc.RestorePreview(ctx, archive)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}
	result, err := b.svc.Restore(ctx, preview.Token)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if result.Counts.Platforms != 1 {
		t.Fatalf("RestoreResult.Counts.Platforms = %d, want 1", result.Counts.Platforms)
	}

	platforms, err := b.platformRepo.List(ctx)
	if err != nil {
		t.Fatalf("PlatformRepository.List() error = %v", err)
	}
	if len(platforms) != 1 {
		t.Fatalf("got %d platforms after restore, want exactly 1", len(platforms))
	}
	newID := platforms[0].ID
	if newID == victimID {
		t.Fatalf("the restored platform kept the crafted backup's id %q instead of a fresh one", victimID)
	}

	// B's ORIGINAL secret must be completely untouched - restore never
	// reads, writes, or deletes anything in the SecretStore.
	stillThere, err := b.store.Get(ctx, victimSecretKey)
	if err != nil {
		t.Fatalf("the pre-existing secret under %q is gone after restore: %v", victimSecretKey, err)
	}
	if string(stillThere) != victimSecret {
		t.Fatalf("the pre-existing secret under %q was modified by restore: got %q", victimSecretKey, stillThere)
	}

	// The critical assertion: the newly restored platform - under its
	// own real, freshly minted id - must never resolve to the victim's
	// secret. Checked through the real credential.Service, the actual
	// production code path that answers "does this destination have a
	// stream key", not by re-deriving the key ourselves.
	credSvc := credential.NewService(b.store)
	status, storeStatus, err := credSvc.Status(ctx, newID)
	if err != nil {
		t.Fatalf("credential.Service.Status() error = %v", err)
	}
	if !storeStatus.Available {
		t.Fatal("credential store reported unavailable in-memory - fixture is broken")
	}
	if status.Configured {
		t.Fatalf("the restored platform (new id %q) reports a configured stream key - it must not, since no backup ever carries one", newID)
	}
}

// noAssets is a trivial AssetBlobSource that is never expected to be
// called - the crafted fixture above declares zero visual/audio
// assets, so WriteArchive's own blob-collection pass finds nothing to
// open in the first place.
type noAssets struct{}

func (noAssets) Open(sha256Hex string) (*os.File, error) {
	return nil, errors.New("no assets are expected to be opened by this fixture")
}

func TestManagedVisualAssetRoundTripsThroughBackupAndRestoreByContentHash(t *testing.T) {
	a := newInstallation(t)
	b := newInstallation(t)
	ctx := context.Background()
	now := time.Now().UTC()

	imageBytes := []byte("a real (fake) PNG payload for the round-trip test")
	sum := sha256.Sum256(imageBytes)
	shaHex := hex.EncodeToString(sum[:])

	visualARepo := sqlite.NewVisualAssetRepository(a.db.DB)
	if err := visualARepo.CreateBlob(ctx, visualasset.Blob{
		SHA256: shaHex, MediaType: visualasset.MediaPNG, ByteSize: int64(len(imageBytes)),
		StorageName: shaHex, PublicToken: "tok_original", CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed installation A's blob row: %v", err)
	}
	assetID := "asset_sentinel_test"
	if err := visualARepo.CreateAsset(ctx, visualasset.Asset{
		ID: assetID, BlobSHA256: shaHex, Kind: visualasset.KindImage,
		DisplayName: "sentinel-logo.png", Source: "upload", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed installation A's asset row: %v", err)
	}

	// Write the real blob bytes into A's real FileStore (the same
	// WriteBlob production upload handling uses), rather than only
	// inserting the database rows - a genuine content round trip needs
	// a real file on disk to actually read back from.
	if _, _, err := a.visualStore.WriteBlob(bytes.NewReader(imageBytes), int64(len(imageBytes))); err != nil {
		t.Fatalf("write installation A's real blob file: %v", err)
	}

	data, err := a.svc.Export(ctx)
	if err != nil {
		t.Fatalf("installation A Export() error = %v", err)
	}

	preview, err := b.svc.RestorePreview(ctx, data)
	if err != nil {
		t.Fatalf("installation B RestorePreview() error = %v", err)
	}
	if preview.AssetCount != 1 {
		t.Fatalf("PreviewSession.AssetCount = %d, want 1", preview.AssetCount)
	}
	result, err := b.svc.Restore(ctx, preview.Token)
	if err != nil {
		t.Fatalf("installation B Restore() error = %v", err)
	}
	if result.Counts.VisualAssets != 1 {
		t.Fatalf("RestoreResult.Counts.VisualAssets = %d, want 1", result.Counts.VisualAssets)
	}

	visualBRepo := sqlite.NewVisualAssetRepository(b.db.DB)
	assets, err := visualBRepo.ListAssets(ctx)
	if err != nil {
		t.Fatalf("installation B ListAssets() error = %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("got %d assets in installation B, want 1", len(assets))
	}
	if assets[0].DisplayName != "sentinel-logo.png" {
		t.Errorf("restored asset DisplayName = %q, want %q", assets[0].DisplayName, "sentinel-logo.png")
	}
	if assets[0].ID == assetID {
		t.Error("the restored asset kept the source backup's own id instead of a fresh local one")
	}
	if assets[0].BlobSHA256 != shaHex {
		t.Errorf("restored asset BlobSHA256 = %q, want %q (the original content hash)", assets[0].BlobSHA256, shaHex)
	}

	// The real proof: read the blob back from B's OWN FileStore by
	// content hash and confirm it is byte-identical to the original.
	f, err := b.visualStore.Open(shaHex)
	if err != nil {
		t.Fatalf("installation B's FileStore does not have the restored blob: %v", err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read restored blob: %v", err)
	}
	if string(got) != string(imageBytes) {
		t.Errorf("restored blob content = %q, want the original %q", got, imageBytes)
	}
}

// TestRestoreRecomputesOnboardingStateAgainstRealDatabase is the real-
// database counterpart to service_restore_test.go's fake-sink onboarding
// coverage: proves the recompute (restore_commit.go's
// recomputeOnboardingStatus) actually reaches the real onboarding_state
// row through sqlite.OnboardingRepository, end to end, across two
// genuinely different restores into the SAME installation - not merely
// set once and left alone.
func TestRestoreRecomputesOnboardingStateAgainstRealDatabase(t *testing.T) {
	inst := newInstallation(t)
	ctx := context.Background()
	onboardingRepo := sqlite.NewOnboardingRepository(inst.db.DB)

	st, found, err := onboardingRepo.GetState(ctx)
	if err != nil || !found {
		t.Fatalf("GetState() = %+v, %v, %v", st, found, err)
	}
	if st.Status != onboarding.StatusPending {
		t.Fatalf("initial onboarding status = %q, want %q (fresh installation with its seed platforms removed, docs/onboarding.md §4.3)", st.Status, onboarding.StatusPending)
	}

	// A backup taken of this genuinely untouched installation.
	emptyBackup, err := inst.svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export() (empty) error = %v", err)
	}

	// Now configure and enable a real destination, and back THAT up too.
	now := time.Now().UTC()
	if err := inst.platformRepo.Create(ctx, platform.Platform{
		ID: "pf_configured", ProviderID: platform.ProviderTwitch, DisplayName: "Real destination",
		Enabled: true, SortOrder: 1, CreatedAt: now, UpdatedAt: now,
		Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("PlatformRepository.Create() error = %v", err)
	}
	configuredBackup, err := inst.svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export() (configured) error = %v", err)
	}

	restore := func(t *testing.T, data []byte) {
		t.Helper()
		preview, err := inst.svc.RestorePreview(ctx, data)
		if err != nil {
			t.Fatalf("RestorePreview() error = %v", err)
		}
		if _, err := inst.svc.Restore(ctx, preview.Token); err != nil {
			t.Fatalf("Restore() error = %v", err)
		}
	}

	t.Run("restoring the configured backup dismisses onboarding", func(t *testing.T) {
		restore(t, configuredBackup)
		st, _, err := onboardingRepo.GetState(ctx)
		if err != nil {
			t.Fatalf("GetState() error = %v", err)
		}
		if st.Status != onboarding.StatusDismissed {
			t.Errorf("onboarding status = %q, want %q", st.Status, onboarding.StatusDismissed)
		}
	})

	t.Run("restoring the empty backup afterward resets onboarding to pending", func(t *testing.T) {
		restore(t, emptyBackup)
		st, _, err := onboardingRepo.GetState(ctx)
		if err != nil {
			t.Fatalf("GetState() error = %v", err)
		}
		if st.Status != onboarding.StatusPending {
			t.Errorf("onboarding status = %q, want %q (the second restore's own config has no real prior use, regardless of what the first restore left behind)", st.Status, onboarding.StatusPending)
		}
	})
}

// TestRichStateRoundTripsAcrossTwoIndependentRealInstallations is the
// governing task's §6 rich-state round trip: one deterministic fixture
// spanning every MUST-BACK-UP domain that has a simple, flat shape
// (platforms+output+remote target, provider integration settings,
// connected accounts+links+region+engagement, chat overlays+hidden
// users+blocked terms+activity types, chat automation schedules+
// commands, alert profiles+rules, goals+widget profiles, metadata
// presets, stream setup profiles, donation sources, operator chat
// preferences, update preferences, the stream-session retention
// preference) - exercised through two genuinely independent real
// SQLite installations and the real Service (never a mock), proving
// portability end to end:
//
//	rich fixture -> restore into A (seeds A for real) -> Export A
//	(STATE A) -> restore STATE A into a DIFFERENT installation B ->
//	Export B (STATE B) -> STATE B must semantically equal STATE A.
//
// Visual designs/templates/managed assets are deliberately left out of
// THIS fixture - TestExportComposesChatOverlayWithChildRowsAndVisual
// Design, TestExportComposesAlertProfileWithRulesAndVisualDesign, and
// TestManagedVisualAssetRoundTripsThroughBackupAndRestoreByContentHash
// already give that domain dedicated, real-content-hash coverage
// stronger than folding it into this already-large fixture would add.
func TestRichStateRoundTripsAcrossTwoIndependentRealInstallations(t *testing.T) {
	a := newInstallation(t)
	b := newInstallation(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pid := "pf_1"
	fixture := backup.Config{
		FormatVersion: backup.FormatVersion,
		Platforms: []backup.PlatformExport{
			{
				Platform: platform.Platform{
					ID: "pf_1", ProviderID: platform.ProviderYouTube, DisplayName: "Rich YouTube",
					Enabled: true, SortOrder: 0, CreatedAt: now, UpdatedAt: now,
					Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
				},
				Output: output.Settings{ServerURL: "rtmp://example.invalid/live", AutoRestart: true},
				RemoteTarget: &remotetarget.Target{
					PlatformID: "pf_1", ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast,
					ResourceID: "yt-external-broadcast-id-999", DisplayName: "My Live Broadcast",
					CreatedAt: now, UpdatedAt: now,
				},
			},
		},
		ProviderIntegrationSettings: []account.IntegrationSettings{
			{ProviderID: account.ProviderTwitch, ClientID: "rich-twitch-client-id", UpdatedAt: now},
		},
		ConnectedAccounts: []backup.ConnectedAccountExport{
			{
				Account: account.Account{
					ID: "acct_1", ProviderID: account.ProviderTwitch, ProviderUserID: "pu_rich",
					Login: "richuser", DisplayName: "Rich User", AvatarURL: "https://example.invalid/a.png",
					Status: account.StatusConnected, CreatedAt: now, UpdatedAt: now, Scopes: []string{"chat:read"},
				},
				PlatformLinks: []string{"pf_1"}, YouTubeRegion: "US", EngagementEnabled: boolPtr(true),
			},
		},
		ChatOverlays: []backup.ChatOverlayExport{
			{
				Profile: func() chatoverlay.Profile {
					p := chatoverlay.Default("Main overlay")
					p.ID, p.PublicSlug = "ov_1", "slug-rich-1"
					return p
				}(),
				AccountIDs: []string{"acct_1"},
				HiddenUsers: []chatoverlay.HiddenUser{
					{OverlayID: "ov_1", ProviderID: chatoverlay.ProviderTwitch, ConnectedAccountID: "acct_1", ProviderUserID: "hidden_pu", Label: "Hidden", CreatedAt: now},
				},
				BlockedTerms:  []chatoverlay.BlockedTerm{{ID: "bt_1", OverlayID: "ov_1", Value: "badword", MatchMode: chatoverlay.MatchContains, CreatedAt: now, UpdatedAt: now}},
				ActivityTypes: []string{"follow", "subscription"},
			},
		},
		ChatSchedules: []chatautomation.Schedule{
			{ID: "sch_1", Name: "Reminder", Enabled: true, IntervalSeconds: 600, Targets: []chatautomation.Target{{AccountID: "acct_1", PlatformID: "pf_1"}}, CreatedAt: now, UpdatedAt: now},
		},
		ChatCommands: []chatautomation.Command{
			{
				ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "join us!",
				RequiredRole: chatautomation.RoleEveryone,
				Targets:      []chatautomation.Target{{AccountID: "acct_1", PlatformID: "pf_1"}},
				CreatedAt:    now, UpdatedAt: now,
			},
		},
		AlertProfiles: []backup.AlertProfileExport{
			{
				Profile: func() alerts.Profile {
					p := alerts.DefaultProfile("Main alerts")
					p.ID, p.PublicSlug = "alp_1", "alert-slug-1"
					return p
				}(),
				Rules: []alerts.Rule{
					{
						ID: "alr_1", ProfileID: "alp_1", Name: "Follow", Enabled: true,
						EventType: alerts.EventFollow, Priority: 50, DurationMS: 5000,
						RequiredRole: alerts.RoleEveryone, ShowPlatform: true, ShowUsername: true,
						EntryAnimation: alerts.AnimationNone, ExitAnimation: alerts.AnimationNone, AnimationDurationMS: 400,
						GroupWindowMS: 5000, InterruptMode: alerts.InterruptNever,
						Audio:    alerts.RuleAudio{SoundVolume: 1.0, TTSVolume: 1.0},
						Accounts: []string{"acct_1"}, CreatedAt: now, UpdatedAt: now,
					},
				},
			},
		},
		Goals: []goals.Goal{
			func() goals.Goal {
				g := goals.DefaultGoal("Follower goal", goals.KindFollowers, 1000)
				g.ID, g.Current, g.CreatedAt, g.UpdatedAt, g.StartedAt = "goal_1", 250, now, now, now
				return g
			}(),
		},
		WidgetProfiles: []goals.WidgetProfile{
			func() goals.WidgetProfile {
				w := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindGoal, "goal_1", "Goal widget")
				w.ID, w.PublicSlug, w.CreatedAt, w.UpdatedAt = "wp_1", "widget-slug-1", now, now
				return w
			}(),
		},
		MetadataPresets: []metadatapreset.Preset{
			{ID: "mp_1", Name: "Coding preset", Note: "for dev streams", CreatedAt: now, UpdatedAt: now},
		},
		StreamSetupProfiles: []streamsetup.Profile{
			{
				ID: "setup_1", Name: "Gaming", MetadataPresetID: strPtr("mp_1"), MetadataPresetName: "Coding preset",
				Destinations: []streamsetup.Destination{{PlatformID: &pid, ProviderID: "youtube", DisplayName: "Rich YouTube"}},
				CreatedAt:    now, UpdatedAt: now,
			},
		},
		DonationSources: []donationsource.Source{
			{ID: "donsrc_1", ProviderID: donationsource.ProviderStreamElements, Label: "Main donations", Enabled: true, RemoteChannelID: "5ad23dcc18fff500d78c5348", CreatedAt: now, UpdatedAt: now},
		},
		OperatorChatPreferences: &backup.OperatorChatPreferencesExport{
			Preferences:       operatorchatprefs.Preferences{ShowBadges: true, CompactMode: true, CreatedAt: now, UpdatedAt: now},
			AccountVisibility: []operatorchatprefs.AccountVisibility{{AccountID: "acct_1", Visible: true, CreatedAt: now, UpdatedAt: now}},
			HiddenUsers:       []operatorchatprefs.UserRef{{ID: "ur_1", ProviderID: operatorchatprefs.ProviderTwitch, ConnectedAccountID: "acct_1", ProviderUserID: "hidden_op", Label: "Hidden op", CreatedAt: now}},
			BotUsers:          []operatorchatprefs.UserRef{{ID: "ur_2", ProviderID: operatorchatprefs.ProviderTwitch, ConnectedAccountID: "acct_1", ProviderUserID: "bot_op", Label: "Bot op", CreatedAt: now}},
		},
		UpdatePreferences:          &updatersettings.Preferences{AutoCheck: false, CreatedAt: now, UpdatedAt: now},
		StreamSessionRetentionDays: intPtr(30),
	}

	archive, err := backup.WriteArchive(fixture, "0.1.0-test", "windows", now, noAssets{}, noAssets{})
	if err != nil {
		t.Fatalf("WriteArchive(fixture) error = %v", err)
	}

	// Seed installation A for real, through the real production Restore
	// path - never a hand-written repository seed.
	previewA, err := a.svc.RestorePreview(ctx, archive)
	if err != nil {
		t.Fatalf("installation A RestorePreview() error = %v", err)
	}
	if _, err := a.svc.Restore(ctx, previewA.Token); err != nil {
		t.Fatalf("installation A Restore() error = %v", err)
	}

	archiveA, err := a.svc.Export(ctx)
	if err != nil {
		t.Fatalf("installation A Export() error = %v", err)
	}
	validatedA, err := backup.ReadArchive(archiveA)
	if err != nil {
		t.Fatalf("ReadArchive(installation A's own export) error = %v", err)
	}
	stateA := validatedA.Config
	assertRichStateShape(t, "A", stateA)

	// Restore installation A's own real export directly into a
	// DIFFERENT real installation - the actual portability path an
	// operator moving to a new machine exercises, never a re-serialized
	// copy of the same bytes.
	previewB, err := b.svc.RestorePreview(ctx, archiveA)
	if err != nil {
		t.Fatalf("installation B RestorePreview() error = %v", err)
	}
	if _, err := b.svc.Restore(ctx, previewB.Token); err != nil {
		t.Fatalf("installation B Restore() error = %v", err)
	}

	archiveB, err := b.svc.Export(ctx)
	if err != nil {
		t.Fatalf("installation B Export() error = %v", err)
	}
	validatedB, err := backup.ReadArchive(archiveB)
	if err != nil {
		t.Fatalf("ReadArchive(installation B's own export) error = %v", err)
	}
	stateB := validatedB.Config
	assertRichStateShape(t, "B", stateB)

	// The real proof: B is a semantic copy of A, on a completely
	// independent database - never merely "some data exists".
	if stateA.Platforms[0].Platform.DisplayName != stateB.Platforms[0].Platform.DisplayName {
		t.Errorf("platform DisplayName drifted: A=%q B=%q", stateA.Platforms[0].Platform.DisplayName, stateB.Platforms[0].Platform.DisplayName)
	}
	if stateA.Platforms[0].Platform.ID == stateB.Platforms[0].Platform.ID {
		t.Error("platform id was not re-minted on B's own restore")
	}
	if stateA.Platforms[0].Output.ServerURL != stateB.Platforms[0].Output.ServerURL {
		t.Errorf("output ServerURL drifted: A=%q B=%q", stateA.Platforms[0].Output.ServerURL, stateB.Platforms[0].Output.ServerURL)
	}
	if stateA.Platforms[0].RemoteTarget == nil || stateB.Platforms[0].RemoteTarget == nil {
		t.Fatal("RemoteTarget missing on A or B")
	}
	if stateA.Platforms[0].RemoteTarget.ResourceID != stateB.Platforms[0].RemoteTarget.ResourceID {
		t.Errorf("RemoteTarget.ResourceID (an EXTERNAL provider id, never remapped) drifted: A=%q B=%q", stateA.Platforms[0].RemoteTarget.ResourceID, stateB.Platforms[0].RemoteTarget.ResourceID)
	}
	if stateA.Platforms[0].RemoteTarget.PlatformID == stateB.Platforms[0].RemoteTarget.PlatformID {
		t.Error("RemoteTarget.PlatformID (a LOCAL backup-local reference) was not re-minted on B - it must track the new platform id")
	}
	if stateB.Platforms[0].RemoteTarget.PlatformID != stateB.Platforms[0].Platform.ID {
		t.Error("RemoteTarget.PlatformID does not point at B's own restored platform - cross-reference broken")
	}

	if len(stateB.ConnectedAccounts) != 1 || stateB.ConnectedAccounts[0].Account.Status != account.StatusReconnectRequired {
		t.Errorf("ConnectedAccounts[0] = %+v, want exactly one, status reconnect_required", stateB.ConnectedAccounts)
	}
	if len(stateB.ConnectedAccounts[0].PlatformLinks) != 1 || stateB.ConnectedAccounts[0].PlatformLinks[0] != stateB.Platforms[0].Platform.ID {
		t.Errorf("ConnectedAccounts[0].PlatformLinks = %v, want [%s] (B's own platform id)", stateB.ConnectedAccounts[0].PlatformLinks, stateB.Platforms[0].Platform.ID)
	}
	if stateB.ConnectedAccounts[0].EngagementEnabled == nil || !*stateB.ConnectedAccounts[0].EngagementEnabled {
		t.Error("ConnectedAccounts[0].EngagementEnabled did not survive the round trip")
	}

	newAccountID := stateB.ConnectedAccounts[0].Account.ID
	if len(stateB.ChatOverlays) != 1 || len(stateB.ChatOverlays[0].AccountIDs) != 1 || stateB.ChatOverlays[0].AccountIDs[0] != newAccountID {
		t.Errorf("ChatOverlays[0].AccountIDs = %+v, want [%s]", stateB.ChatOverlays, newAccountID)
	}
	if len(stateB.AlertProfiles) != 1 || len(stateB.AlertProfiles[0].Rules) != 1 || len(stateB.AlertProfiles[0].Rules[0].Accounts) != 1 || stateB.AlertProfiles[0].Rules[0].Accounts[0] != newAccountID {
		t.Errorf("AlertProfiles[0].Rules[0].Accounts = %+v, want [%s]", stateB.AlertProfiles, newAccountID)
	}
	if stateB.AlertProfiles[0].Rules[0].ProfileID != stateB.AlertProfiles[0].Profile.ID {
		t.Error("alert rule ProfileID does not point at B's own restored profile - cross-reference broken")
	}

	if len(stateB.Goals) != 1 || len(stateB.WidgetProfiles) != 1 {
		t.Fatalf("got %d goals, %d widget profiles, want 1, 1", len(stateB.Goals), len(stateB.WidgetProfiles))
	}
	if stateB.WidgetProfiles[0].GoalID != stateB.Goals[0].ID {
		t.Error("widget profile GoalID does not point at B's own restored goal - cross-reference broken")
	}
	if stateB.Goals[0].Target != stateA.Goals[0].Target || stateB.Goals[0].Current != stateA.Goals[0].Current {
		t.Errorf("goal Target/Current drifted: A={%d,%d} B={%d,%d}", stateA.Goals[0].Target, stateA.Goals[0].Current, stateB.Goals[0].Target, stateB.Goals[0].Current)
	}

	if len(stateB.StreamSetupProfiles) != 1 {
		t.Fatalf("got %d stream setup profiles, want 1", len(stateB.StreamSetupProfiles))
	}
	setup := stateB.StreamSetupProfiles[0]
	if setup.MetadataPresetID == nil || *setup.MetadataPresetID != stateB.MetadataPresets[0].ID {
		t.Errorf("StreamSetupProfiles[0].MetadataPresetID = %v, want B's own restored preset id %q", setup.MetadataPresetID, stateB.MetadataPresets[0].ID)
	}
	if len(setup.Destinations) != 1 || setup.Destinations[0].PlatformID == nil || *setup.Destinations[0].PlatformID != stateB.Platforms[0].Platform.ID {
		t.Errorf("StreamSetupProfiles[0].Destinations[0].PlatformID = %v, want B's own restored platform id %q", setup.Destinations, stateB.Platforms[0].Platform.ID)
	}

	if len(stateB.DonationSources) != 1 || stateB.DonationSources[0].Enabled {
		t.Errorf("DonationSources[0] = %+v, want exactly one, Enabled=false (never auto-enabled by restore)", stateB.DonationSources)
	}
	if stateB.DonationSources[0].Label != stateA.DonationSources[0].Label {
		t.Errorf("donation source Label drifted: A=%q B=%q", stateA.DonationSources[0].Label, stateB.DonationSources[0].Label)
	}

	if stateB.OperatorChatPreferences == nil || !stateB.OperatorChatPreferences.Preferences.ShowBadges {
		t.Error("OperatorChatPreferences did not survive the round trip")
	}
	if stateB.UpdatePreferences == nil || stateB.UpdatePreferences.AutoCheck != stateA.UpdatePreferences.AutoCheck {
		t.Errorf("UpdatePreferences drifted: A=%+v B=%+v", stateA.UpdatePreferences, stateB.UpdatePreferences)
	}
	if stateB.StreamSessionRetentionDays == nil || *stateB.StreamSessionRetentionDays != 30 {
		t.Errorf("StreamSessionRetentionDays = %v, want 30", stateB.StreamSessionRetentionDays)
	}
	if len(stateB.ChatSchedules) != 1 || stateB.ChatSchedules[0].Targets[0].AccountID != newAccountID {
		t.Errorf("ChatSchedules[0].Targets = %+v, want AccountID %s", stateB.ChatSchedules, newAccountID)
	}
	if len(stateB.ChatCommands) != 1 || stateB.ChatCommands[0].Targets[0].PlatformID != stateB.Platforms[0].Platform.ID {
		t.Errorf("ChatCommands[0].Targets = %+v, want PlatformID %s", stateB.ChatCommands, stateB.Platforms[0].Platform.ID)
	}
}

// assertRichStateShape is the common "every domain this fixture touches
// actually round-tripped at all" sanity check, shared between STATE A
// (seeded from the hand-authored fixture) and STATE B (seeded from
// STATE A) - fails fast and clearly if either export came back
// unexpectedly empty, before the more detailed field-level assertions
// run.
func assertRichStateShape(t *testing.T, label string, cfg backup.Config) {
	t.Helper()
	counts := map[string]int{
		"Platforms": len(cfg.Platforms), "ProviderIntegrationSettings": len(cfg.ProviderIntegrationSettings),
		"ConnectedAccounts": len(cfg.ConnectedAccounts), "ChatOverlays": len(cfg.ChatOverlays),
		"ChatSchedules": len(cfg.ChatSchedules), "ChatCommands": len(cfg.ChatCommands),
		"AlertProfiles": len(cfg.AlertProfiles), "Goals": len(cfg.Goals), "WidgetProfiles": len(cfg.WidgetProfiles),
		"MetadataPresets": len(cfg.MetadataPresets), "StreamSetupProfiles": len(cfg.StreamSetupProfiles),
		"DonationSources": len(cfg.DonationSources),
	}
	for name, n := range counts {
		if n != 1 {
			t.Errorf("state %s: %s has %d entries, want 1", label, name, n)
		}
	}
	if cfg.OperatorChatPreferences == nil {
		t.Errorf("state %s: OperatorChatPreferences is nil, want present", label)
	}
	if cfg.UpdatePreferences == nil {
		t.Errorf("state %s: UpdatePreferences is nil, want present", label)
	}
	if cfg.StreamSessionRetentionDays == nil {
		t.Errorf("state %s: StreamSessionRetentionDays is nil, want present", label)
	}
}

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
