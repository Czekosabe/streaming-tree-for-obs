package backup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
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

	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms:     []PlatformExport{{Platform: platform.Platform{ID: "pf_old", DisplayName: "Main"}}},
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
	if !result.RestartRequired {
		t.Error("RestoreResult.RestartRequired = false, want true (chat automation/alerts/engagement connectors only reload at process start)")
	}
	if len(sinks.platforms) != 1 {
		t.Fatalf("got %d platforms after restore, want 1", len(sinks.platforms))
	}
	for id := range sinks.platforms {
		if id == "pf_old" {
			t.Error("the restored platform kept its backup-supplied id")
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

func TestRestoreUnknownTokenReturnsNotFound(t *testing.T) {
	sinks := newFakeSinks()
	svc := newRestoreTestService(t, sinks, fakeStreamingGuard{active: false}, &memSafetyStore{})

	_, err := svc.Restore(context.Background(), "rst_does_not_exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Restore() error = %v, want ErrNotFound", err)
	}
}
