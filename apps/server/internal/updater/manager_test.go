package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/updater/manifest"
)

// --- test doubles -----------------------------------------------------

type fakeSettingsRepo struct {
	stored *updatersettings.Preferences
}

func (f *fakeSettingsRepo) GetPreferences(ctx context.Context) (updatersettings.Preferences, bool, error) {
	if f.stored == nil {
		return updatersettings.Preferences{}, false, nil
	}
	return *f.stored, true, nil
}

func (f *fakeSettingsRepo) SetPreferences(ctx context.Context, p updatersettings.Preferences, now time.Time) (updatersettings.Preferences, error) {
	p.UpdatedAt = now
	f.stored = &p
	return p, nil
}

func newTestSettings() *updatersettings.Service {
	return updatersettings.NewService(&fakeSettingsRepo{}, func() time.Time { return time.Now().UTC() })
}

type fakeBranches struct {
	snapshots []branch.Snapshot
	err       error
}

func (f *fakeBranches) Snapshot(ctx context.Context) ([]branch.Snapshot, error) {
	return f.snapshots, f.err
}

type fakeHandoff struct {
	available   bool
	blockerCode string
	beginErr    error
	beginCalls  int
}

func (f *fakeHandoff) Available() (bool, string) { return f.available, f.blockerCode }

func (f *fakeHandoff) Begin(ctx context.Context, candidatePath, expectedVersion string) error {
	f.beginCalls++
	return f.beginErr
}

// --- CheckNow -----------------------------------------------------

func TestCheckNowDisabledInDevelopmentBuild(t *testing.T) {
	m := NewManager(Options{
		Client: newClient("http://unused.invalid", "0.1.0"), Settings: newTestSettings(),
		ReleaseBuild: false, CurrentVersion: "0.1.0",
	})

	if err := m.CheckNow(context.Background()); err != ErrDisabled {
		t.Fatalf("CheckNow() error = %v, want ErrDisabled", err)
	}
	if got := m.Status(context.Background()).State; got != StateDisabled {
		t.Fatalf("State = %q, want %q", got, StateDisabled)
	}
}

func TestPlatformUnsupportedNeverStartsOrActs(t *testing.T) {
	// docs/macos-packaging.md §20: a release build whose Handoff reports
	// itself statically unsupported (the non-Windows UnsupportedHandoff's
	// permanent answer) must never begin automatic polling and must
	// refuse every manual action outright - regardless of whether a
	// manifest happens to list an artifact for this platform's identity.
	// The client here points at an unused URL: if CheckNow ever actually
	// contacted it, the test would hang/fail rather than returning
	// ErrPlatformUnsupported immediately.
	m := NewManager(Options{
		Client:            newClient("http://unused.invalid", "0.1.0"),
		Settings:          newTestSettings(),
		Branches:          &fakeBranches{},
		Handoff:           &fakeHandoff{available: false, blockerCode: BlockerPlatformUnsupported},
		ReleaseBuild:      true,
		ProductionVersion: true,
		CurrentVersion:    "0.1.0",
		Identity:          manifest.Identity{OS: manifest.OSDarwin, Arch: manifest.ArchARM64, Kind: manifest.KindDMG},
	})

	if got := m.Status(context.Background()).State; got != StatePlatformUnsupported {
		t.Fatalf("initial State = %q, want %q", got, StatePlatformUnsupported)
	}

	if err := m.CheckNow(context.Background()); err != ErrPlatformUnsupported {
		t.Fatalf("CheckNow() error = %v, want ErrPlatformUnsupported", err)
	}
	if err := m.Download(context.Background()); err != ErrPlatformUnsupported {
		t.Fatalf("Download() error = %v, want ErrPlatformUnsupported", err)
	}
	if err := m.Install(context.Background()); err != ErrPlatformUnsupported {
		t.Fatalf("Install() error = %v, want ErrPlatformUnsupported", err)
	}

	// Enabling AutoCheck must not start the background loop on this
	// platform - if it did, m.stopCh would become non-nil.
	if err := m.SetAutoCheck(context.Background(), true); err != nil {
		t.Fatalf("SetAutoCheck() error = %v", err)
	}
	m.Start(context.Background())
	if m.stopCh != nil {
		t.Fatal("Start() began the automatic check loop on a platform-unsupported build")
	}

	if got := m.Status(context.Background()).State; got != StatePlatformUnsupported {
		t.Fatalf("State after actions = %q, want %q", got, StatePlatformUnsupported)
	}
}

func TestManualBuildNeverStartsOrActs(t *testing.T) {
	// A packaged build whose injected version is not a strict
	// major.minor.patch production version (ProductionVersion: false -
	// e.g. a manual/test build such as "0.1.0-manualtest+abc") must
	// never begin automatic polling and must refuse every manual action
	// outright, exactly like a platform-unsupported build does - the
	// release pipeline itself refuses to generate real release-manifest
	// metadata for a version shaped like this, so there is nothing such
	// a build could ever successfully check against. The client here
	// points at an unused URL: if CheckNow ever actually contacted it,
	// the test would hang/fail rather than returning ErrManualBuild
	// immediately.
	m := NewManager(Options{
		Client:            newClient("http://unused.invalid", "0.1.0-manualtest+abc"),
		Settings:          newTestSettings(),
		Branches:          &fakeBranches{},
		Handoff:           &fakeHandoff{available: true},
		ReleaseBuild:      true,
		ProductionVersion: false,
		CurrentVersion:    "0.1.0-manualtest+abc",
		Identity:          manifest.Identity{OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller},
	})

	status := m.Status(context.Background())
	if status.State != StateManualBuild {
		t.Fatalf("initial State = %q, want %q", status.State, StateManualBuild)
	}
	if !status.InstallBlocked || status.BlockerCode != BlockerManualBuild {
		t.Fatalf("InstallBlocked/BlockerCode = %v/%q, want true/%q", status.InstallBlocked, status.BlockerCode, BlockerManualBuild)
	}
	if status.CurrentVersion != "0.1.0-manualtest+abc" {
		t.Fatalf("CurrentVersion = %q, want it still reported honestly", status.CurrentVersion)
	}

	if err := m.CheckNow(context.Background()); err != ErrManualBuild {
		t.Fatalf("CheckNow() error = %v, want ErrManualBuild", err)
	}
	if err := m.Download(context.Background()); err != ErrManualBuild {
		t.Fatalf("Download() error = %v, want ErrManualBuild", err)
	}
	if err := m.Install(context.Background()); err != ErrManualBuild {
		t.Fatalf("Install() error = %v, want ErrManualBuild", err)
	}

	// Enabling AutoCheck must not start the background loop on a
	// manual/test build - if it did, m.stopCh would become non-nil.
	if err := m.SetAutoCheck(context.Background(), true); err != nil {
		t.Fatalf("SetAutoCheck() error = %v", err)
	}
	m.Start(context.Background())
	if m.stopCh != nil {
		t.Fatal("Start() began the automatic check loop on a manual/test build")
	}

	if got := m.Status(context.Background()).State; got != StateManualBuild {
		t.Fatalf("State after actions = %q, want %q", got, StateManualBuild)
	}
}

func TestCheckNowNoStableReleasePublished(t *testing.T) {
	// GitHub's documented 404 for /releases/latest - this repository has
	// no published Stable release yet. Must be treated as a successful
	// check in a distinct, non-alarming state, never as StateError -
	// see ErrNoStableRelease/StateNoReleaseYet.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	m := newTestManager(t, server, "0.1.0")

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v, want nil (no Stable release yet is not a failure)", err)
	}

	status := m.Status(context.Background())
	if status.State != StateNoReleaseYet {
		t.Fatalf("State = %q, want %q", status.State, StateNoReleaseYet)
	}
	if status.LastErrorCode != "" {
		t.Fatalf("LastErrorCode = %q, want empty - this is not an error state", status.LastErrorCode)
	}
	if status.LastSuccessfulCheckAt == "" {
		t.Fatal("LastSuccessfulCheckAt not set - a 404 'no release yet' is still a successful check")
	}
}

func TestCheckNowUpToDate(t *testing.T) {
	server := simpleReleaseServer(t, "0.1.0")
	m := newTestManager(t, server, "0.1.0")

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	status := m.Status(context.Background())
	if status.State != StateUpToDate {
		t.Fatalf("State = %q, want %q", status.State, StateUpToDate)
	}
	if status.UpdateAvailable {
		t.Fatal("UpdateAvailable = true, want false")
	}
}

func TestCheckNowNeverOffersDowngrade(t *testing.T) {
	server := simpleReleaseServer(t, "0.1.0")
	// Installed version is newer than the "latest" release.
	m := newTestManager(t, server, "0.2.0")

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	status := m.Status(context.Background())
	if status.State != StateUpToDate || status.UpdateAvailable {
		t.Fatalf("State = %q, UpdateAvailable = %v, want up_to_date/false (no downgrade offered)", status.State, status.UpdateAvailable)
	}
}

func TestCheckNowAvailable(t *testing.T) {
	server := simpleReleaseServer(t, "0.2.0")
	m := newTestManager(t, server, "0.1.0")

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	status := m.Status(context.Background())
	if status.State != StateAvailable || !status.UpdateAvailable {
		t.Fatalf("State = %q, UpdateAvailable = %v, want available/true", status.State, status.UpdateAvailable)
	}
	if status.LatestVersion != "0.2.0" {
		t.Fatalf("LatestVersion = %q, want 0.2.0", status.LatestVersion)
	}
}

func TestCheckNowRejectsDraftRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Czekosabe/streaming-tree-for-obs/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "tag_name": "v0.2.0", "draft": true, "prerelease": false,
			"published_at": time.Now().UTC().Format(time.RFC3339), "assets": []map[string]any{},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	m := newTestManager(t, server, "0.1.0")
	if err := m.CheckNow(context.Background()); err == nil {
		t.Fatal("CheckNow() accepted a draft release, want rejection")
	}
	if got := m.Status(context.Background()).State; got != StateError {
		t.Fatalf("State = %q, want %q", got, StateError)
	}
}

// --- streaming guard integration --------------------------------------

func TestStatusInstallBlockedWhenStreaming(t *testing.T) {
	server := simpleReleaseServer(t, "0.2.0")
	m := newTestManager(t, server, "0.1.0")
	m.branches = &fakeBranches{snapshots: []branch.Snapshot{{State: branch.StateLive, DesiredRunning: true}}}
	m.handoff = &fakeHandoff{available: true}

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	if err := m.Download(context.Background()); err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	status := m.Status(context.Background())
	if !status.InstallBlocked || status.BlockerCode != BlockerStreamingActive {
		t.Fatalf("InstallBlocked = %v, BlockerCode = %q, want true/%q", status.InstallBlocked, status.BlockerCode, BlockerStreamingActive)
	}
}

func TestInstallRefusesWhenStreamingActiveAtFinalCheck(t *testing.T) {
	server := simpleReleaseServer(t, "0.2.0")
	m := newTestManager(t, server, "0.1.0")
	branches := &fakeBranches{} // idle at Download time
	m.branches = branches
	handoff := &fakeHandoff{available: true}
	m.handoff = handoff

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	if err := m.Download(context.Background()); err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Stream starts in the gap between "button enabled" and "Install
	// clicked" - docs/updater.md §18's race.
	branches.snapshots = []branch.Snapshot{{State: branch.StateLive, DesiredRunning: true}}

	if err := m.Install(context.Background()); err == nil {
		t.Fatal("Install() succeeded while streaming was active at the final check, want refusal")
	}
	if handoff.beginCalls != 0 {
		t.Fatalf("handoff.Begin was called %d times, want 0 - nothing should be shut down", handoff.beginCalls)
	}
}

func TestInstallSucceedsAndTriggersShutdown(t *testing.T) {
	server := simpleReleaseServer(t, "0.2.0")
	m := newTestManager(t, server, "0.1.0")
	m.branches = &fakeBranches{}
	handoff := &fakeHandoff{available: true}
	m.handoff = handoff

	var shutdownCalled bool
	m.onHandoffBegun = func() { shutdownCalled = true }

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	if err := m.Download(context.Background()); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if handoff.beginCalls != 1 {
		t.Fatalf("handoff.Begin called %d times, want 1", handoff.beginCalls)
	}
	if !shutdownCalled {
		t.Fatal("onHandoffBegun was not called after a successful handoff")
	}
	if !m.UpdateCommitInProgress() {
		t.Fatal("UpdateCommitInProgress() = false, want true after a successful handoff begins")
	}
}

func TestInstallBlockedWhenHandoffUnavailable(t *testing.T) {
	server := simpleReleaseServer(t, "0.2.0")
	m := newTestManager(t, server, "0.1.0")
	m.branches = &fakeBranches{}
	m.handoff = &fakeHandoff{available: false, blockerCode: BlockerNotInstalledCtx}

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	if err := m.Download(context.Background()); err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	status := m.Status(context.Background())
	if !status.InstallBlocked || status.BlockerCode != BlockerNotInstalledCtx {
		t.Fatalf("InstallBlocked = %v, BlockerCode = %q, want true/%q", status.InstallBlocked, status.BlockerCode, BlockerNotInstalledCtx)
	}

	if err := m.Install(context.Background()); err == nil {
		t.Fatal("Install() succeeded with an unavailable handoff, want refusal")
	}
}

// --- helpers -----------------------------------------------------

func newTestManager(t *testing.T, server *httptest.Server, currentVersion string) *Manager {
	t.Helper()
	dataDir := t.TempDir()
	return NewManager(Options{
		Client:            newClient(server.URL, currentVersion),
		Settings:          newTestSettings(),
		Branches:          &fakeBranches{},
		Handoff:           &fakeHandoff{available: true},
		DataDir:           dataDir,
		ReleaseBuild:      true,
		ProductionVersion: true,
		CurrentVersion:    currentVersion,
		Identity:          manifest.Identity{OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller},
	})
}

// simpleReleaseServer serves a release/manifest describing exactly one
// windows/amd64/installer artifact at version, with a small fake
// installer payload, wired so DownloadAssetTo can fetch it too.
func simpleReleaseServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	payload := []byte("fake installer bytes for " + version)

	installerSHA := shaSum(payload)
	m := manifest.Manifest{
		Format: manifest.Format, SchemaVersion: manifest.SchemaVersion,
		Version: version, Channel: manifest.ChannelStable,
		Artifacts: []manifest.Artifact{
			{
				OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller,
				Name:      "StreamingTreeForOBS-Setup-" + version + ".exe",
				SizeBytes: int64(len(payload)), SHA256: installerSHA,
			},
		},
	}
	manifestBytes := manifest.MustMarshal(m)

	mux := http.NewServeMux()
	var installerURL, manifestURL string
	mux.HandleFunc("/repos/Czekosabe/streaming-tree-for-obs/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "tag_name": "v" + version, "name": version,
			"draft": false, "prerelease": false, "body": "notes",
			"published_at": time.Now().UTC().Format(time.RFC3339),
			"assets": []map[string]any{
				{"id": 1, "name": manifestAssetName, "size": len(manifestBytes), "url": manifestURL},
				{"id": 2, "name": m.Artifacts[0].Name, "size": len(payload), "url": installerURL},
			},
		})
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifestBytes)
	})
	mux.HandleFunc("/installer.exe", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	installerURL = server.URL + "/installer.exe"
	manifestURL = server.URL + "/manifest.json"
	return server
}

func shaSum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// tamperedReleaseServer is simpleReleaseServer, except the installer
// bytes actually served differ from what the manifest declares -
// simulating corruption or tampering between manifest publication and
// download (docs/updater.md §14).
func tamperedReleaseServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	declaredPayload := []byte("fake installer bytes for " + version)
	// Same length as declaredPayload, different content - isolates the
	// SHA-256 check specifically. A differently-sized tampered payload
	// would instead be caught by the separate size-bound check
	// (docs/updater.md §13), which is real, correct, and intentional,
	// but is not what this test exercises.
	servedPayload := []byte(strings.Repeat("X", len(declaredPayload)))

	installerSHA := shaSum(declaredPayload)
	m := manifest.Manifest{
		Format: manifest.Format, SchemaVersion: manifest.SchemaVersion,
		Version: version, Channel: manifest.ChannelStable,
		Artifacts: []manifest.Artifact{
			{
				OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller,
				Name:      "StreamingTreeForOBS-Setup-" + version + ".exe",
				SizeBytes: int64(len(declaredPayload)), SHA256: installerSHA,
			},
		},
	}
	manifestBytes := manifest.MustMarshal(m)

	mux := http.NewServeMux()
	var installerURL, manifestURL string
	mux.HandleFunc("/repos/Czekosabe/streaming-tree-for-obs/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "tag_name": "v" + version, "name": version,
			"draft": false, "prerelease": false, "body": "notes",
			"published_at": time.Now().UTC().Format(time.RFC3339),
			"assets": []map[string]any{
				{"id": 1, "name": manifestAssetName, "size": len(manifestBytes), "url": manifestURL},
				// size declared honestly matches the manifest, so the
				// size-bound check alone would not catch this - only the
				// SHA-256 check does.
				{"id": 2, "name": m.Artifacts[0].Name, "size": len(declaredPayload), "url": installerURL},
			},
		})
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifestBytes)
	})
	mux.HandleFunc("/installer.exe", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(servedPayload)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	installerURL = server.URL + "/installer.exe"
	manifestURL = server.URL + "/manifest.json"
	return server
}

func TestDownloadDetectsHashMismatch(t *testing.T) {
	server := tamperedReleaseServer(t, "0.2.0")
	m := newTestManager(t, server, "0.1.0")

	if err := m.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow() error = %v", err)
	}
	if got := m.Status(context.Background()).State; got != StateAvailable {
		t.Fatalf("State after CheckNow = %q, want %q", got, StateAvailable)
	}

	err := m.Download(context.Background())
	if err == nil {
		t.Fatal("Download() succeeded against tampered content, want a hash-mismatch failure")
	}

	status := m.Status(context.Background())
	if status.State != StateError {
		t.Fatalf("State after failed Download = %q, want %q", status.State, StateError)
	}
	if status.LastErrorCode != ErrorCodeHashMismatch {
		t.Fatalf("LastErrorCode = %q, want %q", status.LastErrorCode, ErrorCodeHashMismatch)
	}
	if !status.InstallBlocked {
		// No verified candidate exists - Install must remain blocked.
		t.Fatal("InstallBlocked = false after a failed download, want true (no verified candidate)")
	}
}
