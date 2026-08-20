package mediamtx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Supervisor owns the MediaMTX lifecycle and the ingest view.
//
// All state it tracks is in memory and resets when the backend restarts. None
// of it is written to SQLite: it describes what is happening now, not what the
// user configured.
type Supervisor struct {
	options   Options
	logger    *slog.Logger
	resolver  *Resolver
	apiClient *APIClient

	// mu guards every field below. State transitions happen only while it is
	// held, so a concurrent Start and Stop cannot interleave halfway.
	mu           sync.Mutex
	state        ProcessState
	installed    string
	source       BinarySource
	binaryPath   string
	current      *process
	startedAt    time.Time
	restartCount int
	lastError    *RuntimeError
	ingest       IngestSnapshot
	// stopRequested suppresses the restart policy for a deliberate stop.
	stopRequested bool
	// installing guards against two concurrent installations.
	installing bool
	// generation increments on every start, so a late exit watcher belonging to
	// an older process cannot trigger a restart for the current one.
	generation uint64

	// backoff is the current restart delay.
	backoff time.Duration
	// restartWindow tracks recent restarts for the retry cap.
	restartTimes []time.Time

	// lifecycle is cancelled when the supervisor shuts down.
	lifecycle context.Context
	cancel    context.CancelFunc
	// workers tracks background goroutines so Shutdown can wait for them.
	workers sync.WaitGroup
}

// Options configures a Supervisor.
type Options struct {
	DataDir     string
	RTMPAddress string
	APIAddress  string
	IngestPath  string
	AutoStart   bool
	AutoRestart bool
	// ExecutablePath is the explicit override, empty when unset.
	ExecutablePath string

	// RemoteIngest is Stage 20D2C's explicit --remote-ingest opt-in
	// (docs/remote-ingest.md §4/§5) - nil for every other deployment
	// mode, threaded straight through to every WriteConfig call this
	// supervisor makes.
	RemoteIngest *RemoteIngestOptions

	Logger *slog.Logger

	// InstallerOptions are forwarded to the installer; tests use them to point
	// at a fixture release server. Not reachable from any HTTP request.
	InstallerOptions []InstallerOption

	// extraEnv is added to the child process environment. It is unexported so
	// only this package's tests can set it: they launch the test binary as a
	// fake MediaMTX and need a flag to reach it. Production always passes none,
	// so this cannot weaken the real environment isolation.
	extraEnv []string
}

const (
	// readinessTimeout bounds how long a spawned process has to answer.
	readinessTimeout = 20 * time.Second
	// readinessPollInterval is how often readiness is probed.
	readinessPollInterval = 250 * time.Millisecond
	// stopGracePeriod is how long a polite stop is given before escalation.
	stopGracePeriod = 5 * time.Second
	// ingestPollInterval is how often the path list is read while ready.
	ingestPollInterval = 1 * time.Second

	// minBackoff and maxBackoff bound the automatic restart delay.
	minBackoff = 1 * time.Second
	maxBackoff = 30 * time.Second
	// stableRunDuration is how long a process must stay ready before the
	// backoff is considered recovered.
	stableRunDuration = 60 * time.Second
	// restartWindow and maxRestartsPerWindow cap a crash loop.
	restartWindow        = 5 * time.Minute
	maxRestartsPerWindow = 5
)

// NewSupervisor builds a supervisor. It performs no I/O.
func NewSupervisor(opts Options) *Supervisor {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	lifecycle, cancel := context.WithCancel(context.Background())

	return &Supervisor{
		options:   opts,
		logger:    logger,
		resolver:  NewResolver(opts.DataDir, opts.ExecutablePath),
		apiClient: NewAPIClient("http://" + opts.APIAddress),
		state:     StateMissing,
		source:    SourceMissing,
		backoff:   minBackoff,
		ingest:    IngestSnapshot{State: IngestUnavailable, Path: opts.IngestPath, Tracks: []string{}},
		lifecycle: lifecycle,
		cancel:    cancel,
	}
}

// Start resolves the binary and, when autostart is on, launches MediaMTX.
//
// It never returns an error for a missing or incompatible binary: those are
// runtime states the interface renders. The Go API must keep serving platform
// configuration regardless of whether MediaMTX can run.
func (s *Supervisor) Start(ctx context.Context) {
	s.refreshResolution(ctx)

	s.mu.Lock()
	autoStart := s.options.AutoStart
	state := s.state
	s.mu.Unlock()

	if !autoStart || state != StateStopped {
		return
	}

	if err := s.RequestStart(ctx); err != nil {
		s.logger.Warn("MediaMTX autostart did not begin", slog.Any("error", err))
	}
}

// refreshResolution re-reads the binary situation and updates the state.
func (s *Supervisor) refreshResolution(ctx context.Context) {
	// Resolution runs I/O (stat and `--version`), so it happens outside the
	// lock and only the resulting state transition is serialized.
	resolution := s.resolver.Resolve(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Never disturb a running or installing process with a resolution refresh.
	switch s.state {
	case StateStarting, StateReady, StateStopping, StateInstalling:
		return
	}

	s.applyResolutionLocked(resolution)
}

// applyResolutionLocked records a resolution. The caller must hold s.mu.
//
// The version, source and state are set together, so no snapshot can ever
// observe "stopped" without the version that made it startable.
func (s *Supervisor) applyResolutionLocked(resolution Resolution) {
	s.source = resolution.Source
	s.installed = resolution.Version
	s.binaryPath = resolution.Path

	switch {
	case resolution.Source == SourceMissing:
		s.state = StateMissing
		s.lastError = resolution.Err
	case !resolution.Compatible:
		s.state = StateIncompatible
		s.lastError = resolution.Err
	default:
		s.state = StateStopped
		s.lastError = nil
	}
}

// RequestStart starts MediaMTX if the current state allows it.
func (s *Supervisor) RequestStart(ctx context.Context) error {
	s.mu.Lock()

	switch s.state {
	case StateStarting, StateReady:
		s.mu.Unlock()
		return fmt.Errorf("%w: MediaMTX is already running", ErrInvalidState)
	case StateInstalling:
		s.mu.Unlock()
		return fmt.Errorf("%w: an installation is in progress", ErrInvalidState)
	case StateMissing:
		s.mu.Unlock()
		return fmt.Errorf("%w: MediaMTX is not installed", ErrNotInstalled)
	case StateIncompatible:
		s.mu.Unlock()
		return fmt.Errorf("%w: the installed MediaMTX version is not supported", ErrIncompatibleVersion)
	}

	path := s.binaryPath
	if path == "" {
		s.mu.Unlock()
		return fmt.Errorf("%w: no MediaMTX executable is available", ErrNotInstalled)
	}

	s.state = StateStarting
	s.stopRequested = false
	s.lastError = nil
	s.generation++
	generation := s.generation
	s.mu.Unlock()

	go s.launch(path, generation)
	return nil
}

// launch performs the start sequence off the caller's goroutine.
func (s *Supervisor) launch(path string, generation uint64) {
	// Ports are checked first so "address already in use" becomes an actionable
	// message rather than a readiness timeout ten seconds later. Nothing is
	// ever terminated to free a port: another application may own it
	// legitimately.
	if err := s.checkPorts(); err != nil {
		s.failStart(generation, err)
		return
	}

	configPath, err := WriteConfig(s.options.DataDir, ConfigOptions{
		RTMPAddress:  s.options.RTMPAddress,
		APIAddress:   s.options.APIAddress,
		IngestPath:   s.options.IngestPath,
		RemoteIngest: s.options.RemoteIngest,
	})
	if err != nil {
		s.failStart(generation, NewRuntimeError(CodeStartFailed,
			"The MediaMTX configuration file could not be written."))
		return
	}

	proc, err := startProcess(path, configPath, s.options.extraEnv, s.logger)
	if err != nil {
		s.failStart(generation, NewRuntimeError(CodeStartFailed,
			"MediaMTX could not be started."))
		return
	}

	s.mu.Lock()
	// A stop arrived while the process was spawning.
	if s.generation != generation {
		s.mu.Unlock()
		_ = proc.stop(stopGracePeriod)
		return
	}
	s.current = proc
	s.mu.Unlock()

	if err := s.awaitReady(proc); err != nil {
		_ = proc.stop(stopGracePeriod)
		s.failStart(generation, NewRuntimeError(CodeReadinessTimeout,
			"MediaMTX started but its Control API did not become ready in time."))
		s.scheduleRestart(generation)
		return
	}

	s.mu.Lock()
	if s.generation != generation {
		s.mu.Unlock()
		_ = proc.stop(stopGracePeriod)
		return
	}
	s.state = StateReady
	s.startedAt = time.Now()
	s.lastError = nil
	s.mu.Unlock()

	s.logger.Info("MediaMTX is ready",
		slog.String("rtmp", s.options.RTMPAddress),
		slog.String("path", s.options.IngestPath))

	s.workers.Add(2)
	go func() {
		defer s.workers.Done()
		s.watchExit(proc, generation)
	}()
	go func() {
		defer s.workers.Done()
		s.pollIngest(proc, generation)
	}()
}

// checkPorts confirms both listeners are free before spawning.
func (s *Supervisor) checkPorts() *RuntimeError {
	for label, address := range map[string]string{
		"RTMP":        s.options.RTMPAddress,
		"Control API": s.options.APIAddress,
	} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return NewRuntimeError(CodePortInUse, fmt.Sprintf(
				"The MediaMTX %s address %s is already in use. "+
					"Stop the other application or choose a different port.", label, address))
		}
		// Released immediately; MediaMTX binds it a moment later. A race is
		// possible in principle but harmless: MediaMTX then reports the bind
		// failure itself and readiness fails with a recorded error.
		_ = listener.Close()
	}
	return nil
}

// awaitReady polls the Control API until it answers or the budget runs out.
//
// Process creation is explicitly not treated as readiness: MediaMTX exits a few
// milliseconds later if its configuration is rejected or a port is taken.
func (s *Supervisor) awaitReady(proc *process) error {
	deadline := time.Now().Add(readinessTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-proc.Exited():
			return errors.New("the process exited during startup")
		case <-s.lifecycle.Done():
			return errors.New("the supervisor is shutting down")
		case <-time.After(readinessPollInterval):
		}

		ctx, cancel := context.WithTimeout(s.lifecycle, apiTimeout)
		err := s.apiClient.Ping(ctx)
		cancel()

		if err == nil {
			return nil
		}
	}

	return errors.New("the Control API did not become ready in time")
}

// failStart records a failed start.
func (s *Supervisor) failStart(generation uint64, runtimeErr *RuntimeError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.generation != generation {
		return
	}

	s.state = StateError
	s.lastError = runtimeErr
	s.current = nil
	s.startedAt = time.Time{}
	s.ingest = IngestSnapshot{State: IngestUnavailable, Path: s.options.IngestPath, Tracks: []string{}}

	s.logger.Warn("MediaMTX start failed",
		slog.String("code", runtimeErr.Code), slog.String("detail", runtimeErr.Message))
}

// watchExit reacts to a process ending on its own.
func (s *Supervisor) watchExit(proc *process, generation uint64) {
	select {
	case <-proc.Exited():
	case <-s.lifecycle.Done():
		return
	}

	s.mu.Lock()
	if s.generation != generation {
		// A newer process owns the state now.
		s.mu.Unlock()
		return
	}

	deliberate := s.stopRequested
	s.current = nil
	s.startedAt = time.Time{}
	s.ingest = IngestSnapshot{State: IngestUnavailable, Path: s.options.IngestPath, Tracks: []string{}}

	if deliberate {
		s.state = StateStopped
		s.mu.Unlock()
		return
	}

	s.state = StateError
	s.lastError = NewRuntimeError(CodeExitedUnexpectedly,
		"MediaMTX stopped unexpectedly.")
	s.mu.Unlock()

	s.logger.Warn("MediaMTX exited unexpectedly")
	s.scheduleRestart(generation)
}

// scheduleRestart applies the restart policy after an unexpected failure.
//
// Bounded exponential backoff, a cap on restarts per window, and a stable-run
// reset. An explicit stop never reaches here, so a deliberate Stop is never
// undone by the policy.
func (s *Supervisor) scheduleRestart(generation uint64) {
	s.mu.Lock()

	if !s.options.AutoRestart || s.stopRequested || s.generation != generation {
		s.mu.Unlock()
		return
	}

	now := time.Now()
	kept := s.restartTimes[:0]
	for _, at := range s.restartTimes {
		if now.Sub(at) < restartWindow {
			kept = append(kept, at)
		}
	}
	s.restartTimes = kept

	if len(s.restartTimes) >= maxRestartsPerWindow {
		s.state = StateError
		s.lastError = NewRuntimeError(CodeRestartLimit, fmt.Sprintf(
			"MediaMTX failed %d times in %s and will not be restarted automatically. "+
				"Fix the underlying problem and start it manually.",
			maxRestartsPerWindow, restartWindow))
		s.mu.Unlock()
		s.logger.Error("MediaMTX restart limit reached")
		return
	}

	s.restartTimes = append(s.restartTimes, now)
	s.restartCount++
	delay := s.backoff
	if s.backoff < maxBackoff {
		s.backoff *= 2
		if s.backoff > maxBackoff {
			s.backoff = maxBackoff
		}
	}
	s.mu.Unlock()

	s.logger.Info("restarting MediaMTX", slog.Duration("delay", delay))

	s.workers.Add(1)
	go func() {
		defer s.workers.Done()

		select {
		case <-time.After(delay):
		case <-s.lifecycle.Done():
			return
		}

		s.refreshResolution(s.lifecycle)
		if err := s.RequestStart(s.lifecycle); err != nil {
			s.logger.Warn("automatic restart did not begin", slog.Any("error", err))
		}
	}()
}

// pollIngest reads the configured path while the process is ready.
func (s *Supervisor) pollIngest(proc *process, generation uint64) {
	ticker := time.NewTicker(ingestPollInterval)
	defer ticker.Stop()

	// A stable run clears the accumulated backoff, so an occasional crash days
	// apart does not inherit a long delay from an earlier bad patch.
	stable := time.NewTimer(stableRunDuration)
	defer stable.Stop()

	for {
		select {
		case <-proc.Exited():
			return
		case <-s.lifecycle.Done():
			return
		case <-stable.C:
			s.mu.Lock()
			if s.generation == generation {
				s.backoff = minBackoff
				s.restartTimes = nil
			}
			s.mu.Unlock()
		case <-ticker.C:
			s.updateIngest(generation)
		}
	}
}

// updateIngest performs one path-status read.
func (s *Supervisor) updateIngest(generation uint64) {
	ctx, cancel := context.WithTimeout(s.lifecycle, apiTimeout)
	status, err := s.apiClient.PathStatusFor(ctx, s.options.IngestPath)
	cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.generation != generation || s.state != StateReady {
		return
	}

	if err != nil {
		// The process is running but its API cannot be read. That is its own
		// state: reporting "waiting" would claim knowledge we do not have.
		s.ingest = IngestSnapshot{
			State:  IngestError,
			Path:   s.options.IngestPath,
			Tracks: []string{},
		}
		s.lastError = NewRuntimeError(CodeAPIUnreachable,
			"MediaMTX is running but its status could not be read.")
		return
	}

	if s.lastError != nil && s.lastError.Code == CodeAPIUnreachable {
		s.lastError = nil
	}

	if !status.Found || !status.Ready {
		s.ingest = IngestSnapshot{
			State:  IngestWaiting,
			Path:   s.options.IngestPath,
			Tracks: []string{},
		}
		return
	}

	trackCount := len(status.Tracks)
	s.ingest = IngestSnapshot{
		State:       IngestReceiving,
		Path:        s.options.IngestPath,
		SourceType:  status.SourceType,
		ConnectedAt: status.ReadyTime,
		TrackCount:  &trackCount,
		Tracks:      status.Tracks,
	}
}

// RequestStop performs a controlled stop and suppresses the restart policy.
func (s *Supervisor) RequestStop(ctx context.Context) error {
	s.mu.Lock()

	if s.state != StateReady && s.state != StateStarting {
		s.mu.Unlock()
		return fmt.Errorf("%w: MediaMTX is not running", ErrInvalidState)
	}

	proc := s.current
	s.state = StateStopping
	s.stopRequested = true
	// Bump the generation so any in-flight launch abandons its work.
	s.generation++
	s.mu.Unlock()

	if proc != nil {
		if err := proc.stop(stopGracePeriod); err != nil {
			s.logger.Warn("stopping MediaMTX was not clean", slog.Any("error", err))
		}
	}

	s.mu.Lock()
	s.state = StateStopped
	s.current = nil
	s.startedAt = time.Time{}
	s.lastError = nil
	s.ingest = IngestSnapshot{State: IngestUnavailable, Path: s.options.IngestPath, Tracks: []string{}}
	// A manual stop is a clean slate for the restart policy.
	s.backoff = minBackoff
	s.restartTimes = nil
	s.mu.Unlock()

	return nil
}

// UpdateRemoteIngestCredential replaces the running supervisor's own
// RemoteIngestOptions.PublisherPassVerifier (docs/remote-ingest.md
// §6/§9) - takes effect on the next WriteConfig call, which
// RequestRestart triggers; this method itself never touches the
// MediaMTX process. A no-op if this supervisor was never constructed
// with RemoteIngest options (desktop/D2A/D2B-only deployments) - it
// mutates existing options, it never enables remote ingest for a
// supervisor that was not given it at construction.
func (s *Supervisor) UpdateRemoteIngestCredential(verifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.options.RemoteIngest == nil {
		return
	}
	// Copy-on-write: RemoteIngestOptions is shared by pointer with
	// whatever WriteConfig call may already be reading through the old
	// value, so a fresh struct replaces it rather than mutating the
	// existing one in place.
	updated := *s.options.RemoteIngest
	updated.PublisherPassVerifier = verifier
	s.options.RemoteIngest = &updated
}

// RequestRestart performs one controlled stop followed by a start.
func (s *Supervisor) RequestRestart(ctx context.Context) error {
	s.mu.Lock()
	running := s.state == StateReady || s.state == StateStarting
	s.mu.Unlock()

	if running {
		if err := s.RequestStop(ctx); err != nil {
			return err
		}
	}

	s.refreshResolution(ctx)
	return s.RequestStart(ctx)
}

// RequestInstall runs a managed installation.
//
// Only one may run at a time: two concurrent downloads would race over the same
// target directory.
func (s *Supervisor) RequestInstall(ctx context.Context) error {
	s.mu.Lock()

	if s.installing {
		s.mu.Unlock()
		return ErrInstallInProgress
	}
	if s.state == StateStarting || s.state == StateReady || s.state == StateStopping {
		s.mu.Unlock()
		return fmt.Errorf("%w: stop MediaMTX before reinstalling it", ErrInvalidState)
	}

	s.installing = true
	s.state = StateInstalling
	s.lastError = nil
	s.mu.Unlock()

	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		s.runInstall()
	}()

	return nil
}

func (s *Supervisor) runInstall() {
	installer := NewInstaller(s.options.DataDir, s.options.InstallerOptions...)
	_, err := installer.Install(s.lifecycle)

	if err != nil {
		s.mu.Lock()
		s.installing = false
		s.state = StateError
		s.lastError = ClassifyInstallError(err)
		s.mu.Unlock()
		s.logger.Warn("MediaMTX installation failed", slog.Any("error", err))
		return
	}

	s.logger.Info("MediaMTX installed", slog.String("version", SupportedVersion))

	// The freshly installed binary is resolved BEFORE the installing state is
	// cleared, so the transition out of "installing" already carries the
	// version and source. Clearing the flag first would leave a window where a
	// snapshot reported "stopped" with no installed version.
	resolution := s.resolver.Resolve(s.lifecycle)

	s.mu.Lock()
	s.installing = false
	s.applyResolutionLocked(resolution)
	state := s.state
	autoStart := s.options.AutoStart
	s.mu.Unlock()

	if autoStart && state == StateStopped {
		if err := s.RequestStart(s.lifecycle); err != nil {
			s.logger.Warn("MediaMTX did not start after installation", slog.Any("error", err))
		}
	}
}

// Snapshot returns the current runtime picture.
func (s *Supervisor) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	ingest := s.ingest
	if ingest.Tracks == nil {
		ingest.Tracks = []string{}
	}
	if ingest.Path == "" {
		ingest.Path = s.options.IngestPath
	}

	return Snapshot{
		MediaMTX: MediaMTXSnapshot{
			SupportedVersion: SupportedVersion,
			InstalledVersion: s.installed,
			Source:           s.source,
			State:            s.state,
			AutoStart:        s.options.AutoStart,
			AutoRestart:      s.options.AutoRestart,
			StartedAt:        formatTime(s.startedAt),
			RestartCount:     s.restartCount,
			LastError:        s.lastError,
		},
		Ingest: ingest,
		Connection: ConnectionSnapshot{
			ServerURL:  "rtmp://" + s.options.RTMPAddress,
			StreamKey:  s.options.IngestPath,
			PublishURL: "rtmp://" + s.options.RTMPAddress + "/" + s.options.IngestPath,
		},
	}
}

// Shutdown stops polling, terminates MediaMTX and reaps it.
//
// It is safe to call when nothing is running.
func (s *Supervisor) Shutdown(ctx context.Context) {
	s.mu.Lock()
	proc := s.current
	s.stopRequested = true
	s.generation++
	s.current = nil
	s.state = StateStopped
	s.mu.Unlock()

	// Stops every background worker: pollers, exit watchers, pending restarts.
	s.cancel()

	if proc != nil {
		if err := proc.stop(stopGracePeriod); err != nil {
			s.logger.Warn("MediaMTX did not stop cleanly", slog.Any("error", err))
		}
	}

	// Wait for the workers so no goroutine outlives the process.
	done := make(chan struct{})
	go func() {
		s.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		s.logger.Warn("timed out waiting for MediaMTX workers to finish")
	}
}

// RecentLogLines returns the bounded diagnostic ring of the running process.
func (s *Supervisor) RecentLogLines() []string {
	s.mu.Lock()
	proc := s.current
	s.mu.Unlock()

	if proc == nil {
		return []string{}
	}
	return proc.recentLines()
}
