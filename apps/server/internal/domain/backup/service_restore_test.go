package backup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/onboarding"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

type fakeStreamingGuard struct {
	active bool
	err    error
}

func (g fakeStreamingGuard) Active(context.Context) (bool, error) {
	return g.active, g.err
}

type memSafetyStore struct {
	saved []byte
}

func (m *memSafetyStore) Save(data []byte) error {
	m.saved = data
	return nil
}
func (m *memSafetyStore) Load() ([]byte, bool, error) {
	if m.saved == nil {
		return nil, false, nil
	}
	return m.saved, true, nil
}

func newRestoreTestService(t *testing.T, sinks *fakeSinks, guard StreamingGuard, safety SafetySnapshotStore) *Service {
	t.Helper()
	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	return NewService(
		sinks.sources(), sinks.sinks(),
		memBlobSource{}, memBlobSource{},
		fakeBlobWriter{}, fakeBlobWriter{},
		staging, safety, guard,
		"0.1.0-test", "windows",
	)
}

func TestRestoreClearsAndAppliesEndToEnd(t *testing.T) {
	sinks := newFakeSinks()
	svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, &memSafetyStore{})

	pid := "pf_old"
	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms:     []PlatformExport{{Platform: platform.Platform{ID: "pf_old", DisplayName: "Main"}}},
		StreamSetupProfiles: []streamsetup.Profile{
			{
				ID: "setup_old", Name: "Gaming",
				Destinations: []streamsetup.Destination{{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main"}},
			},
		},
	}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}

	result, err := svc.Restore(context.Background(), preview.Token)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if result.Counts.Platforms != 1 {
		t.Errorf("RestoreResult.Counts.Platforms = %d, want 1", result.Counts.Platforms)
	}
	if result.Counts.StreamSetupProfiles != 1 {
		t.Errorf("RestoreResult.Counts.StreamSetupProfiles = %d, want 1", result.Counts.StreamSetupProfiles)
	}
	if !result.RestartRequired {
		t.Error("RestoreResult.RestartRequired = false, want true (chat automation/alerts/engagement connectors only reload at process start)")
	}
	if len(sinks.platforms) != 1 {
		t.Fatalf("got %d platforms after restore, want 1", len(sinks.platforms))
	}
	var newPlatformID string
	for id := range sinks.platforms {
		newPlatformID = id
		if id == "pf_old" {
			t.Error("the restored platform kept its backup-supplied id")
		}
	}

	if len(sinks.streamSetups) != 1 {
		t.Fatalf("got %d stream setup profiles after restore, want 1", len(sinks.streamSetups))
	}
	for _, p := range sinks.streamSetups {
		if p.ID == "setup_old" {
			t.Error("the restored stream setup profile kept its backup-supplied id")
		}
		if len(p.Destinations) != 1 || p.Destinations[0].PlatformID == nil || *p.Destinations[0].PlatformID != newPlatformID {
			t.Errorf("stream setup profile destination was not remapped to the new platform id %q: %+v", newPlatformID, p.Destinations)
		}
	}

	// The staged upload must be gone after a successful commit.
	if _, err := svc.staging.Get(preview.Token); err == nil {
		t.Error("staged bytes are still present after a successful Restore")
	}
}

func TestRestoreRefusesWhileStreamingIsActive(t *testing.T) {
	sinks := newFakeSinks()
	sinks.platforms["pf_existing"] = platform.Platform{ID: "pf_existing", DisplayName: "Untouched"}
	svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: true}, &memSafetyStore{})

	cfg := Config{FormatVersion: FormatVersion, Platforms: []PlatformExport{{Platform: platform.Platform{ID: "pf_new"}}}}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}

	_, err = svc.Restore(context.Background(), preview.Token)
	if !errors.Is(err, ErrStreamingActive) {
		t.Fatalf("Restore() error = %v, want ErrStreamingActive", err)
	}

	// Nothing about the existing configuration may have changed.
	if _, ok := sinks.platforms["pf_existing"]; !ok {
		t.Error("the existing platform was removed despite the streaming-active refusal")
	}
	if len(sinks.platforms) != 1 {
		t.Errorf("got %d platforms, want the original 1 (untouched)", len(sinks.platforms))
	}
}

func TestRestoreSavesASafetySnapshotOfThePreRestoreConfiguration(t *testing.T) {
	sinks := newFakeSinks()
	sinks.platforms["pf_before"] = platform.Platform{ID: "pf_before", DisplayName: "Before restore"}
	safety := &memSafetyStore{}
	svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, safety)

	cfg := Config{FormatVersion: FormatVersion, Platforms: []PlatformExport{{Platform: platform.Platform{ID: "pf_after"}}}}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}
	if _, err := svc.Restore(context.Background(), preview.Token); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if safety.saved == nil {
		t.Fatal("no safety snapshot was saved before the restore")
	}
	snapshot, err := ReadArchive(safety.saved)
	if err != nil {
		t.Fatalf("the saved safety snapshot is not a valid package: %v", err)
	}
	if len(snapshot.Config.Platforms) != 1 || snapshot.Config.Platforms[0].Platform.ID != "pf_before" {
		t.Errorf("safety snapshot = %+v, want the PRE-restore configuration (pf_before)", snapshot.Config.Platforms)
	}
}

// docs/backup-restore.md §7 step 5/§19: recovering from a bad restore
// is running the whole restore flow again using the safety snapshot as
// the source package - no separate rollback code path. This proves
// that recovery actually works, not merely that a snapshot gets saved
// (TestRestoreSavesASafetySnapshotOfThePreRestoreConfiguration already
// covers that half).
func TestRestoringTheSafetySnapshotRecoversThePreRestoreConfiguration(t *testing.T) {
	sinks := newFakeSinks()
	sinks.platforms["pf_before"] = platform.Platform{ID: "pf_before", DisplayName: "Before restore"}
	safety := &memSafetyStore{}
	svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, safety)

	cfg := Config{FormatVersion: FormatVersion, Platforms: []PlatformExport{{Platform: platform.Platform{ID: "pf_after"}}}}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}
	if _, err := svc.Restore(context.Background(), preview.Token); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if len(sinks.platforms) != 1 {
		t.Fatalf("got %d platforms after the first restore, want 1", len(sinks.platforms))
	}
	for id := range sinks.platforms {
		if id == "pf_before" || id == "pf_after" {
			t.Fatalf("the first restore did not mint a fresh id: got %q", id)
		}
	}

	// Recover: restore the safety snapshot taken before the first
	// restore, through the exact same preview-then-commit flow.
	rollbackPreview, err := svc.RestorePreview(context.Background(), safety.saved)
	if err != nil {
		t.Fatalf("RestorePreview(safety snapshot) error = %v", err)
	}
	if _, err := svc.Restore(context.Background(), rollbackPreview.Token); err != nil {
		t.Fatalf("Restore(safety snapshot) error = %v", err)
	}

	if len(sinks.platforms) != 1 {
		t.Fatalf("got %d platforms after recovery, want 1", len(sinks.platforms))
	}
	var recoveredName string
	for _, p := range sinks.platforms {
		recoveredName = p.DisplayName
	}
	if recoveredName != "Before restore" {
		t.Errorf("recovered platform DisplayName = %q, want %q (the pre-restore configuration)", recoveredName, "Before restore")
	}
}

// TestRestoreRecomputesOnboardingStatusFromRestoredConfigNotFromPriorState
// proves the onboarding-auto-show decision restore leaves behind always
// reflects what actually just landed in the database, never whatever the
// PRE-restore installation happened to have - a backup carries no
// onboarding field at all (Sinks.Onboarding's own doc comment), so a
// value literally left over from before Restore ran would be exactly the
// internal-inconsistency bug this recompute exists to prevent.
func TestRestoreRecomputesOnboardingStatusFromRestoredConfigNotFromPriorState(t *testing.T) {
	t.Run("restoring a real configured destination dismisses onboarding, even onto a pending install", func(t *testing.T) {
		sinks := newFakeSinks()
		sinks.onboardingStatus = onboarding.StatusPending
		svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, &memSafetyStore{})

		cfg := Config{
			FormatVersion: FormatVersion,
			Platforms:     []PlatformExport{{Platform: platform.Platform{ID: "pf_1", Enabled: true}}},
		}
		data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
		if err != nil {
			t.Fatalf("WriteArchive() error = %v", err)
		}
		preview, err := svc.RestorePreview(context.Background(), data)
		if err != nil {
			t.Fatalf("RestorePreview() error = %v", err)
		}
		if _, err := svc.Restore(context.Background(), preview.Token); err != nil {
			t.Fatalf("Restore() error = %v", err)
		}
		if sinks.onboardingStatus != onboarding.StatusDismissed {
			t.Errorf("onboardingStatus = %q, want %q", sinks.onboardingStatus, onboarding.StatusDismissed)
		}
	})

	t.Run("restoring an empty config resets onboarding to pending, even onto a dismissed install", func(t *testing.T) {
		sinks := newFakeSinks()
		sinks.onboardingStatus = onboarding.StatusDismissed
		svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, &memSafetyStore{})

		data, err := WriteArchive(Config{FormatVersion: FormatVersion}, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
		if err != nil {
			t.Fatalf("WriteArchive() error = %v", err)
		}
		preview, err := svc.RestorePreview(context.Background(), data)
		if err != nil {
			t.Fatalf("RestorePreview() error = %v", err)
		}
		if _, err := svc.Restore(context.Background(), preview.Token); err != nil {
			t.Fatalf("Restore() error = %v", err)
		}
		if sinks.onboardingStatus != onboarding.StatusPending {
			t.Errorf("onboardingStatus = %q, want %q", sinks.onboardingStatus, onboarding.StatusPending)
		}
	})
}

// failingChatOverlaySink wraps a real Sinks.ChatOverlays implementation
// and deterministically fails its CreateProfile call on the Nth
// invocation - a test-controlled abstraction (never a real filesystem/
// database fault), used below to prove docs/backup-restore.md §7 step
// 7's own documented claim: a restore that fails PARTWAY through commit
// is recoverable by restoring the safety snapshot step 5 already saved,
// which "self-heals a partial state regardless of where the previous
// attempt stopped".
type failingChatOverlaySink struct {
	inner interface {
		CreateProfile(ctx context.Context, p chatoverlay.Profile) (chatoverlay.Profile, error)
		DeleteProfile(ctx context.Context, id string) error
		SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error
		AddHiddenUser(ctx context.Context, ref chatoverlay.HiddenUser, now time.Time) (chatoverlay.HiddenUser, error)
		AddBlockedTerm(ctx context.Context, term chatoverlay.BlockedTerm, now time.Time) (chatoverlay.BlockedTerm, error)
		SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error
	}
	failOnCall int
	calls      *int
}

func (f failingChatOverlaySink) CreateProfile(ctx context.Context, p chatoverlay.Profile) (chatoverlay.Profile, error) {
	*f.calls++
	if *f.calls == f.failOnCall {
		return chatoverlay.Profile{}, errors.New("injected failure: simulated write fault")
	}
	return f.inner.CreateProfile(ctx, p)
}
func (f failingChatOverlaySink) DeleteProfile(ctx context.Context, id string) error {
	return f.inner.DeleteProfile(ctx, id)
}
func (f failingChatOverlaySink) SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error {
	return f.inner.SetAccounts(ctx, overlayID, accountIDs)
}
func (f failingChatOverlaySink) AddHiddenUser(ctx context.Context, ref chatoverlay.HiddenUser, now time.Time) (chatoverlay.HiddenUser, error) {
	return f.inner.AddHiddenUser(ctx, ref, now)
}
func (f failingChatOverlaySink) AddBlockedTerm(ctx context.Context, term chatoverlay.BlockedTerm, now time.Time) (chatoverlay.BlockedTerm, error) {
	return f.inner.AddBlockedTerm(ctx, term, now)
}
func (f failingChatOverlaySink) SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error {
	return f.inner.SetActivityTypes(ctx, overlayID, activityTypes)
}

// TestRestoreFailingPartwayThroughCommitRecoversViaTheSafetySnapshot is
// the deterministic failure-injection proof docs/backup-restore.md §7
// step 7 promises: restore is not one database transaction, so a fault
// partway through commit leaves a genuinely partial state - but the
// pre-restore safety snapshot (step 5, saved before any destructive
// write) restores it exactly, via the same ordinary restore flow, never
// a second special-case recovery code path.
func TestRestoreFailingPartwayThroughCommitRecoversViaTheSafetySnapshot(t *testing.T) {
	sinks := newFakeSinks()
	sinks.platforms["pf_before"] = platform.Platform{ID: "pf_before", DisplayName: "Before restore"}
	safety := &memSafetyStore{}

	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	realSinks := sinks.sinks()
	calls := 0
	realSinks.ChatOverlays = failingChatOverlaySink{inner: realSinks.ChatOverlays, failOnCall: 2, calls: &calls}
	svc := NewService(
		sinks.sources(), realSinks,
		memBlobSource{}, memBlobSource{},
		fakeBlobWriter{}, fakeBlobWriter{},
		staging, safety, fakeStreamingGuard{active: false},
		"0.1.0-test", "windows",
	)

	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms:     []PlatformExport{{Platform: platform.Platform{ID: "pf_after", DisplayName: "After restore"}}},
		ChatOverlays: []ChatOverlayExport{
			{Profile: chatoverlay.Profile{ID: "ov_1", Name: "First overlay"}},
			{Profile: chatoverlay.Profile{ID: "ov_2", Name: "Second overlay - never committed"}},
		},
	}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}

	if _, err := svc.Restore(context.Background(), preview.Token); err == nil {
		t.Fatal("Restore() error = nil, want the injected failure to surface")
	}

	// Step 5 already ran: the pre-restore safety snapshot exists.
	if safety.saved == nil {
		t.Fatal("no safety snapshot was saved before the failed restore")
	}
	snapshot, err := ReadArchive(safety.saved)
	if err != nil {
		t.Fatalf("the saved safety snapshot is not a valid package: %v", err)
	}
	if len(snapshot.Config.Platforms) != 1 || snapshot.Config.Platforms[0].Platform.DisplayName != "Before restore" {
		t.Fatalf("safety snapshot = %+v, want the PRE-restore configuration", snapshot.Config.Platforms)
	}

	// The documented honest consequence: a genuinely partial state.
	// clearExisting already ran (pf_before is gone) and platforms +
	// the first chat overlay already committed before the injected
	// failure on the second - never claiming a false all-or-nothing
	// rollback within this one attempt.
	if _, stillHasBefore := sinks.platforms["pf_before"]; stillHasBefore {
		t.Error("pf_before is still present - clearExisting should have already run before the injected failure")
	}
	if len(sinks.overlays) != 1 {
		t.Fatalf("got %d chat overlays after the failed restore, want exactly 1 (the second CreateProfile call was the injected failure)", len(sinks.overlays))
	}

	// Recovery: restore the safety snapshot through the exact same
	// flow, no special-case code path.
	rollbackPreview, err := svc.RestorePreview(context.Background(), safety.saved)
	if err != nil {
		t.Fatalf("RestorePreview(safety snapshot) error = %v", err)
	}
	if _, err := svc.Restore(context.Background(), rollbackPreview.Token); err != nil {
		t.Fatalf("Restore(safety snapshot) error = %v", err)
	}

	if len(sinks.platforms) != 1 {
		t.Fatalf("got %d platforms after recovery, want 1", len(sinks.platforms))
	}
	var recoveredName string
	for _, p := range sinks.platforms {
		recoveredName = p.DisplayName
	}
	if recoveredName != "Before restore" {
		t.Errorf("recovered platform DisplayName = %q, want %q - the safety snapshot did not fully self-heal the partial failure", recoveredName, "Before restore")
	}
	if len(sinks.overlays) != 0 {
		t.Errorf("got %d chat overlays after recovery, want 0 (the pre-restore snapshot had none)", len(sinks.overlays))
	}
}

func TestRestoreUnknownTokenReturnsNotFound(t *testing.T) {
	sinks := newFakeSinks()
	svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, &memSafetyStore{})

	_, err := svc.Restore(context.Background(), "rst_does_not_exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Restore() error = %v, want ErrNotFound", err)
	}
}
