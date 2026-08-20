package remoteingest

import (
	"context"
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

// stubSupervisor is a controllable Supervisor for Manager tests - the
// real *mediamtx.Supervisor's own process-management behavior is
// covered by its own package tests.
type stubSupervisor struct {
	receiving    bool
	restartCalls int
	restartErr   error
	lastVerifier string
	verifierSet  bool
}

func (s *stubSupervisor) Snapshot() mediamtx.Snapshot {
	state := mediamtx.IngestWaiting
	if s.receiving {
		state = mediamtx.IngestReceiving
	}
	return mediamtx.Snapshot{Ingest: mediamtx.IngestSnapshot{State: state}}
}

func (s *stubSupervisor) UpdateRemoteIngestCredential(verifier string) {
	s.lastVerifier = verifier
	s.verifierSet = true
}

func (s *stubSupervisor) RequestRestart(context.Context) error {
	s.restartCalls++
	return s.restartErr
}

func newTestManager() (*Manager, *stubSupervisor) {
	supervisor := &stubSupervisor{}
	manager := &Manager{
		Store:      secretstest.New(),
		Supervisor: supervisor,
	}
	return manager, supervisor
}

func TestManagerProvisionGeneratesAndAppliesACredential(t *testing.T) {
	manager, supervisor := newTestManager()
	ctx := context.Background()

	secret, err := manager.Provision(ctx)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if secret == "" {
		t.Fatal("Provision() returned an empty secret")
	}
	if !supervisor.verifierSet {
		t.Error("the supervisor's verifier was never updated")
	}
	if supervisor.lastVerifier == secret {
		t.Error("the supervisor received the plaintext secret instead of its verifier")
	}
	if supervisor.restartCalls != 1 {
		t.Errorf("restart called %d times, want 1", supervisor.restartCalls)
	}

	configured, err := manager.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !configured {
		t.Error("Status() = false after Provision, want true")
	}
}

func TestManagerProvisionRefusesWhenAlreadyProvisioned(t *testing.T) {
	manager, _ := newTestManager()
	ctx := context.Background()

	if _, err := manager.Provision(ctx); err != nil {
		t.Fatalf("first Provision() error = %v", err)
	}
	if _, err := manager.Provision(ctx); !errors.Is(err, ErrAlreadyProvisioned) {
		t.Errorf("second Provision() error = %v, want ErrAlreadyProvisioned", err)
	}
}

func TestManagerProvisionRefusesWhileStreamingIsActive(t *testing.T) {
	manager, supervisor := newTestManager()
	supervisor.receiving = true
	ctx := context.Background()

	if _, err := manager.Provision(ctx); !errors.Is(err, ErrStreamingActive) {
		t.Errorf("Provision() error = %v, want ErrStreamingActive", err)
	}
	if supervisor.restartCalls != 0 {
		t.Error("the supervisor was restarted despite the streaming-active refusal")
	}
	configured, _ := manager.Status(ctx)
	if configured {
		t.Error("Status() = true after a refused Provision, want false - nothing should have been stored")
	}
}

func TestManagerRotateReplacesTheCredential(t *testing.T) {
	manager, supervisor := newTestManager()
	ctx := context.Background()

	first, err := manager.Provision(ctx)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	firstVerifier := supervisor.lastVerifier

	second, err := manager.Rotate(ctx)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	if first == second {
		t.Error("Rotate produced the same plaintext secret as Provision")
	}
	if supervisor.lastVerifier == firstVerifier {
		t.Error("Rotate did not change the verifier applied to the supervisor")
	}
	if supervisor.restartCalls != 2 {
		t.Errorf("restart called %d times, want 2 (provision + rotate)", supervisor.restartCalls)
	}
}

func TestManagerRotateRefusesWhileStreamingIsActive(t *testing.T) {
	manager, supervisor := newTestManager()
	ctx := context.Background()

	if _, err := manager.Provision(ctx); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	supervisor.receiving = true

	if _, err := manager.Rotate(ctx); !errors.Is(err, ErrStreamingActive) {
		t.Errorf("Rotate() error = %v, want ErrStreamingActive", err)
	}
	if supervisor.restartCalls != 1 {
		t.Error("Rotate triggered a restart despite the streaming-active refusal")
	}
}

func TestManagerRevokeRemovesTheCredential(t *testing.T) {
	manager, supervisor := newTestManager()
	ctx := context.Background()

	if _, err := manager.Provision(ctx); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if err := manager.Revoke(ctx); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	configured, err := manager.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if configured {
		t.Error("Status() = true after Revoke, want false")
	}
	if supervisor.lastVerifier != "" {
		t.Errorf("supervisor verifier = %q after Revoke, want empty", supervisor.lastVerifier)
	}
	if supervisor.restartCalls != 2 {
		t.Errorf("restart called %d times, want 2 (provision + revoke)", supervisor.restartCalls)
	}
}

func TestManagerRevokeRefusesWhileStreamingIsActive(t *testing.T) {
	manager, supervisor := newTestManager()
	ctx := context.Background()

	if _, err := manager.Provision(ctx); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	supervisor.receiving = true

	if err := manager.Revoke(ctx); !errors.Is(err, ErrStreamingActive) {
		t.Errorf("Revoke() error = %v, want ErrStreamingActive", err)
	}
	configured, _ := manager.Status(ctx)
	if !configured {
		t.Error("Revoke was refused but the credential was removed anyway")
	}
}

func TestManagerRevokeWithNothingProvisionedIsNotAnError(t *testing.T) {
	manager, _ := newTestManager()
	ctx := context.Background()

	if err := manager.Revoke(ctx); err != nil {
		t.Errorf("Revoke() on an empty manager error = %v, want nil", err)
	}
}
