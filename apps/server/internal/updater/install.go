package updater

import (
	"context"
	"errors"
	"fmt"
)

// ErrInstallBlocked is returned by Install when the streaming-active
// guard (or the installed-context/candidate checks) refuses the
// request - callers map this to the specific blocker code from
// Status()/installBlocker rather than a generic failure.
var ErrInstallBlocked = errors.New("update install is currently blocked")

// UpdateCommitInProgress reports whether Install has begun its final
// commit critical section (docs/updater.md §18) - branch-start
// endpoints consult this to reject a stream-start request arriving in
// the brief window between the final guard re-check and the actual
// shutdown signal, closing the narrow race a stream starting in that
// gap would otherwise create.
func (m *Manager) UpdateCommitInProgress() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.committing
}

// Install begins the update-installation handoff (docs/updater.md
// §18/§21/§24): re-checks every guard against the real, current
// runtime state immediately before committing to shutdown, sets the
// short-lived "committing" gate for the duration of that check, hands
// off to the platform Handoff implementation, and - only once the
// handoff has been successfully launched - invokes the shutdown
// callback to begin the application's existing graceful-shutdown
// sequence. If any guard fails, nothing is shut down and the
// application keeps running normally.
func (m *Manager) Install(ctx context.Context) error {
	if !m.releaseBuild {
		return ErrDisabled
	}
	if m.platformUnsupported {
		return ErrPlatformUnsupported
	}

	m.mu.Lock()
	if m.installing {
		m.mu.Unlock()
		return nil
	}
	if m.state != StateReadyToInstall || m.verifiedCandidatePath == "" {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrInstallBlocked, BlockerNoCandidate)
	}
	candidatePath := m.verifiedCandidatePath
	candidateVersion := m.verifiedCandidateVersion
	m.installing = true
	m.committing = true
	m.mu.Unlock()

	// Re-check every guard against the real, current state - this is
	// the narrow race-closing re-check from docs/updater.md §18, not a
	// repeat of the check already done to enable the button.
	if ok, code := m.handoff.Available(); !ok {
		m.abortInstall()
		return fmt.Errorf("%w: %s", ErrInstallBlocked, code)
	}
	if m.streamingActive(ctx) {
		m.abortInstall()
		return fmt.Errorf("%w: %s", ErrInstallBlocked, BlockerStreamingActive)
	}

	m.mu.Lock()
	m.state = StateInstalling
	m.mu.Unlock()

	if err := m.handoff.Begin(ctx, candidatePath, candidateVersion); err != nil {
		m.mu.Lock()
		m.state = StateError
		m.lastErrorCode = ErrorCodeInstallFailed
		m.installing = false
		m.committing = false
		m.mu.Unlock()
		return err
	}

	// Handoff launched successfully - the application is now expected
	// to shut down immediately. m.committing is deliberately left true:
	// there is no meaningful "after" state to return to once the
	// helper has taken over, and the process is about to exit via the
	// existing graceful-shutdown path.
	if m.onHandoffBegun != nil {
		m.onHandoffBegun()
	}
	return nil
}

func (m *Manager) abortInstall() {
	m.mu.Lock()
	m.installing = false
	m.committing = false
	m.mu.Unlock()
}
