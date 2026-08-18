package updater

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/updater/manifest"
)

// ErrDisabled means the manager was asked to act while not a release
// build (docs/updater.md §35) - every check/download/install operation
// refuses outright rather than silently no-op-ing.
var ErrDisabled = errors.New("updater is disabled in a development build")

// ErrPlatformUnsupported means the manager was asked to act on a
// release build whose platform has no usable update-install path at all
// (docs/macos-packaging.md §20, StatePlatformUnsupported) - every
// check/download/install operation refuses outright, the same way
// ErrDisabled does for a development build.
var ErrPlatformUnsupported = errors.New("updater is not available on this platform")

const (
	// startupCheckDelayBase/Jitter is the short, randomized delay before
	// the very first automatic check after a successful startup
	// (docs/updater.md §10) - never immediately racing startup itself.
	startupCheckDelayBase   = 30 * time.Second
	startupCheckDelayJitter = 30 * time.Second

	// autoCheckInterval/Jitter is the automatic check cadence - roughly
	// hourly with a bounded jitter so many installations do not all
	// poll GitHub on the same wall-clock minute (docs/updater.md §10).
	autoCheckInterval       = 60 * time.Minute
	autoCheckIntervalJitter = 5 * time.Minute

	// maxReleaseNotesRunes bounds the plain-text release notes shown to
	// the operator (docs/updater.md §12).
	maxReleaseNotesRunes = 4000
)

// Options configures a Manager.
type Options struct {
	Client   *Client
	Settings *updatersettings.Service
	Branches BranchSnapshotSource
	Handoff  Handoff

	// DataDir is the per-user application data directory
	// (config.Config.DataDir) - downloads stage under DataDir/updates
	// (docs/updater.md §16).
	DataDir string

	ReleaseBuild   bool
	CurrentVersion string
	Identity       manifest.Identity

	// OnHandoffBegun is called once Install has successfully launched
	// the platform handoff (docs/updater.md §21/§24) - production
	// wiring sets this to the same shutdown trigger
	// POST /api/system/shutdown already reuses, so the updater never
	// invents a second shutdown implementation.
	OnHandoffBegun func()

	// Clock and Rand are injectable for deterministic tests.
	Clock func() time.Time
	Rand  *rand.Rand

	Logger *slog.Logger
}

// Manager is the Stage 20B update-manager state machine
// (docs/updater.md §11).
type Manager struct {
	client   *Client
	settings *updatersettings.Service
	branches BranchSnapshotSource
	handoff  Handoff

	dataDir        string
	releaseBuild   bool
	currentVersion string
	identity       manifest.Identity
	onHandoffBegun func()

	// platformUnsupported is decided once at construction from the
	// Handoff's own static answer and never changes afterward (see
	// StatePlatformUnsupported) - read without m.mu, exactly like
	// releaseBuild/currentVersion/identity above, since it too is
	// immutable after NewManager returns.
	platformUnsupported bool

	clock func() time.Time
	rand  *rand.Rand

	logger *slog.Logger

	mu sync.Mutex

	state      State
	checking   bool
	installing bool
	committing bool // docs/updater.md §18's narrow race gate
	autoCheck  bool
	etag       string

	latestVersion         string
	latestTag             string
	releaseNotes          string
	releaseNotesTruncated bool
	publishedAt           time.Time
	lastSuccessfulCheckAt time.Time
	lastErrorCode         string

	// latestRelease/latestArtifact cache the release Download() acts on,
	// set only when CheckNow finds a genuinely available, installable
	// update for this platform's identity - never used across an
	// application restart (in-memory only, exactly like every other
	// runtime manager in this codebase).
	latestRelease  *Release
	latestArtifact manifest.Artifact

	downloadedBytes int64
	totalBytes      int64

	verifiedCandidatePath    string
	verifiedCandidateVersion string

	postUpdateOutcome     string
	postUpdateFromVersion string
	postUpdateToVersion   string

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewManager builds a Manager. It does not touch the network or the
// filesystem - call Start to begin automatic checking.
func NewManager(opts Options) *Manager {
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	rng := opts.Rand
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// A release build whose Handoff statically reports install as
	// platform-unsupported (never dependent on installed-context state,
	// unlike BlockerNotInstalledCtx) starts, and stays, in
	// StatePlatformUnsupported - never StateIdle, so a fresh manifest
	// listing an artifact for this platform's own identity can never by
	// itself make CheckNow/Start believe an install is actually possible
	// here (docs/macos-packaging.md §20).
	platformUnsupported := false
	state := StateIdle
	if !opts.ReleaseBuild {
		state = StateDisabled
	} else if opts.Handoff != nil {
		if ok, code := opts.Handoff.Available(); !ok && code == BlockerPlatformUnsupported {
			platformUnsupported = true
			state = StatePlatformUnsupported
		}
	}

	return &Manager{
		client:              opts.Client,
		settings:            opts.Settings,
		branches:            opts.Branches,
		handoff:             opts.Handoff,
		dataDir:             opts.DataDir,
		releaseBuild:        opts.ReleaseBuild,
		currentVersion:      opts.CurrentVersion,
		identity:            opts.Identity,
		onHandoffBegun:      opts.OnHandoffBegun,
		platformUnsupported: platformUnsupported,
		clock:               clock,
		rand:                rng,
		logger:              logger,
		state:               state,
		autoCheck:           true,
	}
}

// Start reads the persisted auto-check preference and, in a release
// build with it enabled, begins the automatic startup + hourly
// background check loop (docs/updater.md §10). A no-op in a
// development build regardless of the preference's value.
func (m *Manager) Start(ctx context.Context) {
	m.consumePostUpdateResult()

	prefs, err := m.settings.Preferences(ctx)
	if err != nil {
		m.logger.Warn("could not load updater preferences at startup, using defaults", slog.Any("error", err))
		prefs = updatersettings.Default()
	}

	m.mu.Lock()
	m.autoCheck = prefs.AutoCheck
	m.mu.Unlock()

	if !m.releaseBuild || !prefs.AutoCheck || m.platformUnsupported {
		return
	}

	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	go m.scheduleLoop()
}

// Shutdown stops the automatic check loop, if running.
func (m *Manager) Shutdown(ctx context.Context) {
	if m.stopCh == nil {
		return
	}
	close(m.stopCh)
	select {
	case <-m.doneCh:
	case <-ctx.Done():
	}
}

func (m *Manager) scheduleLoop() {
	defer close(m.doneCh)

	initialDelay := startupCheckDelayBase + jitter(m.rand, startupCheckDelayJitter)
	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-timer.C:
			checkCtx, cancel := context.WithTimeout(context.Background(), metadataRequestTimeout)
			if err := m.CheckNow(checkCtx); err != nil {
				m.logger.Info("automatic update check failed (nonfatal)", slog.Any("error", err))
			}
			cancel()

			next := autoCheckInterval + jitter(m.rand, autoCheckIntervalJitter)
			timer.Reset(next)
		}
	}
}

func jitter(rng *rand.Rand, max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rng.Int63n(int64(max)))
}

// SetAutoCheck persists the automatic-check preference and starts/stops
// the background loop accordingly (release builds only - a development
// build persists the preference but never starts a loop, per
// docs/updater.md §35).
func (m *Manager) SetAutoCheck(ctx context.Context, enabled bool) error {
	if _, err := m.settings.ReplacePreferences(ctx, updatersettings.Preferences{AutoCheck: enabled}); err != nil {
		return err
	}

	m.mu.Lock()
	wasRunning := m.stopCh != nil
	m.autoCheck = enabled
	m.mu.Unlock()

	if !m.releaseBuild || m.platformUnsupported {
		return nil
	}

	if enabled && !wasRunning {
		m.stopCh = make(chan struct{})
		m.doneCh = make(chan struct{})
		go m.scheduleLoop()
	} else if !enabled && wasRunning {
		m.Shutdown(ctx)
		m.mu.Lock()
		m.stopCh = nil
		m.doneCh = nil
		m.mu.Unlock()
	}
	return nil
}

// Status returns the current safe status snapshot (docs/updater.md
// §11/§30), including a live install-block check.
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	s := Status{
		Enabled:               m.releaseBuild,
		ReleaseBuild:          m.releaseBuild,
		CurrentVersion:        m.currentVersion,
		AutoCheck:             m.autoCheck,
		State:                 m.state,
		LatestVersion:         m.latestVersion,
		UpdateAvailable:       m.state == StateAvailable || m.state == StateDownloading || m.state == StateReadyToInstall,
		ReleaseNotes:          m.releaseNotes,
		ReleaseNotesTruncated: m.releaseNotesTruncated,
		DownloadedBytes:       m.downloadedBytes,
		TotalBytes:            m.totalBytes,
		LastErrorCode:         m.lastErrorCode,
		PostUpdateOutcome:     m.postUpdateOutcome,
		PostUpdateFromVersion: m.postUpdateFromVersion,
		PostUpdateToVersion:   m.postUpdateToVersion,
	}
	// One-shot: cleared from memory the first time it is read back out,
	// exactly like the on-disk record was already consumed once at
	// Start (docs/updater.md §26).
	m.postUpdateOutcome = ""
	m.postUpdateFromVersion = ""
	m.postUpdateToVersion = ""
	if !m.publishedAt.IsZero() {
		s.PublishedAt = m.publishedAt.UTC().Format(time.RFC3339)
	}
	if !m.lastSuccessfulCheckAt.IsZero() {
		s.LastSuccessfulCheckAt = m.lastSuccessfulCheckAt.UTC().Format(time.RFC3339)
	}
	hasCandidate := m.verifiedCandidatePath != ""
	m.mu.Unlock()

	if !m.releaseBuild {
		return s
	}

	blocked, code := m.installBlocker(ctx, hasCandidate)
	s.InstallBlocked = blocked
	s.BlockerCode = code
	return s
}

// installBlocker reports whether Install would currently be refused,
// and why - shared by Status (informational) and Install (enforced).
func (m *Manager) installBlocker(ctx context.Context, hasCandidate bool) (bool, string) {
	if ok, code := m.handoff.Available(); !ok {
		return true, code
	}
	if !hasCandidate {
		return true, BlockerNoCandidate
	}
	if m.streamingActive(ctx) {
		return true, BlockerStreamingActive
	}
	return false, ""
}

func (m *Manager) streamingActive(ctx context.Context) bool {
	if m.branches == nil {
		return false
	}
	snapshots, err := m.branches.Snapshot(ctx)
	if err != nil {
		// Unable to determine real state - fail safe by treating this as
		// active rather than silently allowing an install to proceed
		// against unknown runtime state.
		m.logger.Warn("could not read branch runtime state for the update guard; blocking install", slog.Any("error", err))
		return true
	}
	return StreamingActive(snapshots)
}
