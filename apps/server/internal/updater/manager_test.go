package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		Client:         newClient(server.URL, currentVersion),
		Settings:       newTestSettings(),
		Branches:       &fakeBranches{},
		Handoff:        &fakeHandoff{available: true},
		DataDir:        dataDir,
		ReleaseBuild:   true,
		CurrentVersion: currentVersion,
		Identity:       manifest.Identity{OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller},
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
