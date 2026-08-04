package credential

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

func newTestService() (*Service, *secretstest.Store) {
	store := secretstest.New()
	return NewService(store), store
}

// --- status ------------------------------------------------------------

func TestStatusReportsNotConfiguredForAnUnknownPlatform(t *testing.T) {
	svc, _ := newTestService()

	status, store, err := svc.Status(context.Background(), "pf_unknown")
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if status.Configured {
		t.Error("Configured = true, want false")
	}
	if !store.Available {
		t.Error("Store.Available = false, want true")
	}
}

func TestStatusReportsConfiguredAfterSet(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	if err := svc.SetStreamKey(ctx, "pf_1", "sk_live_abc"); err != nil {
		t.Fatalf("SetStreamKey() error = %v", err)
	}

	status, store, err := svc.Status(ctx, "pf_1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Configured {
		t.Error("Configured = false, want true after SetStreamKey")
	}
	if !store.Available {
		t.Error("Store.Available = false, want true")
	}
}

func TestStatusReportsStoreUnavailableWithoutAnError(t *testing.T) {
	svc, fake := newTestService()
	fake.Unavailable = true

	status, store, err := svc.Status(context.Background(), "pf_1")
	if err != nil {
		t.Fatalf("Status() error = %v, want nil - an unavailable store is a stable status, not a failure", err)
	}
	if status.Configured {
		t.Error("Configured = true, want false when the store is unavailable")
	}
	if store.Available {
		t.Error("Store.Available = true, want false")
	}
}

func TestStatusMapsAnUnexpectedStoreFailureToAnError(t *testing.T) {
	svc, fake := newTestService()
	fake.FailNext = errors.New("boom")

	_, _, err := svc.Status(context.Background(), "pf_1")
	if !errors.Is(err, ErrStoreFailure) {
		t.Errorf("Status() error = %v, want ErrStoreFailure", err)
	}
}

// --- set -----------------------------------------------------------------

func TestSetStreamKeyRejectsAnInvalidValueWithoutTouchingTheStore(t *testing.T) {
	svc, fake := newTestService()

	err := svc.SetStreamKey(context.Background(), "pf_1", "")
	if err == nil {
		t.Fatal("SetStreamKey(\"\") returned nil error")
	}
	if fake.Has(streamKeyKey("pf_1")) {
		t.Error("an invalid value was written to the store")
	}
}

func TestSetStreamKeyReplacesAnExistingValue(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	if err := svc.SetStreamKey(ctx, "pf_1", "sk_live_first"); err != nil {
		t.Fatalf("first SetStreamKey() error = %v", err)
	}
	if err := svc.SetStreamKey(ctx, "pf_1", "sk_live_second"); err != nil {
		t.Fatalf("second SetStreamKey() error = %v", err)
	}

	got, err := svc.RetrieveForProcessStart(ctx, "pf_1")
	if err != nil {
		t.Fatalf("RetrieveForProcessStart() error = %v", err)
	}
	if got != "sk_live_second" {
		t.Errorf("stored value = %q, want %q", got, "sk_live_second")
	}
}

func TestSetStreamKeyOnAnUnavailableStoreReturnsErrStoreUnavailable(t *testing.T) {
	svc, fake := newTestService()
	fake.Unavailable = true

	err := svc.SetStreamKey(context.Background(), "pf_1", "sk_live_abc")
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Errorf("SetStreamKey() error = %v, want ErrStoreUnavailable", err)
	}
}

// --- delete ----------------------------------------------------------------

func TestDeleteStreamKeyIsIdempotentForAnAbsentKey(t *testing.T) {
	svc, _ := newTestService()

	if err := svc.DeleteStreamKey(context.Background(), "pf_never_configured"); err != nil {
		t.Errorf("DeleteStreamKey() on an absent key error = %v, want nil", err)
	}
}

func TestDeleteStreamKeyRemovesAnExistingValue(t *testing.T) {
	svc, fake := newTestService()
	ctx := context.Background()

	if err := svc.SetStreamKey(ctx, "pf_1", "sk_live_abc"); err != nil {
		t.Fatalf("SetStreamKey() error = %v", err)
	}
	if err := svc.DeleteStreamKey(ctx, "pf_1"); err != nil {
		t.Fatalf("DeleteStreamKey() error = %v", err)
	}
	if fake.Has(streamKeyKey("pf_1")) {
		t.Error("value still present in the store after DeleteStreamKey")
	}

	status, _, err := svc.Status(ctx, "pf_1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Configured {
		t.Error("Configured = true after DeleteStreamKey")
	}
}

// --- platform-deletion cleanup ---------------------------------------------

func TestDeletePlatformCredentialsRemovesOnlyTheGivenPlatform(t *testing.T) {
	svc, fake := newTestService()
	ctx := context.Background()

	if err := svc.SetStreamKey(ctx, "pf_1", "sk_live_one"); err != nil {
		t.Fatalf("SetStreamKey(pf_1) error = %v", err)
	}
	if err := svc.SetStreamKey(ctx, "pf_2", "sk_live_two"); err != nil {
		t.Fatalf("SetStreamKey(pf_2) error = %v", err)
	}

	if err := svc.DeletePlatformCredentials(ctx, "pf_1"); err != nil {
		t.Fatalf("DeletePlatformCredentials(pf_1) error = %v", err)
	}

	if fake.Has(streamKeyKey("pf_1")) {
		t.Error("pf_1's credential still present after its own cleanup")
	}
	if !fake.Has(streamKeyKey("pf_2")) {
		t.Error("pf_2's credential was removed by pf_1's cleanup")
	}
}

func TestDeletePlatformCredentialsSucceedsForAPlatformWithNoCredential(t *testing.T) {
	svc, _ := newTestService()

	if err := svc.DeletePlatformCredentials(context.Background(), "pf_never_configured"); err != nil {
		t.Errorf("DeletePlatformCredentials() error = %v, want nil", err)
	}
}

func TestDeletePlatformCredentialsProceedsWhenTheStoreIsUnavailable(t *testing.T) {
	// An unreachable store must not block ordinary platform deletion: see
	// the ErrUnavailable case in DeletePlatformCredentials for the reasoning.
	svc, fake := newTestService()
	fake.Unavailable = true

	if err := svc.DeletePlatformCredentials(context.Background(), "pf_1"); err != nil {
		t.Errorf("DeletePlatformCredentials() error = %v, want nil when the store is merely unavailable", err)
	}
}

func TestDeletePlatformCredentialsFailsWhenTheStoreReportsAnUnexpectedFailure(t *testing.T) {
	// Unlike a simply-unavailable store, an unexpected failure while the
	// store IS reachable must abort so the caller does not delete the
	// platform row while its credential's fate is unknown.
	svc, fake := newTestService()
	fake.FailNext = errors.New("boom")

	err := svc.DeletePlatformCredentials(context.Background(), "pf_1")
	if !errors.Is(err, ErrStoreFailure) {
		t.Errorf("DeletePlatformCredentials() error = %v, want ErrStoreFailure", err)
	}
}

// --- namespace stability (via the service's key derivation) ----------------

func TestTwoDestinationsForTheSameProviderGetIndependentCredentials(t *testing.T) {
	// The credential domain has no notion of "provider" at all - only a
	// platform ID - which is exactly what guarantees this.
	svc, _ := newTestService()
	ctx := context.Background()

	if err := svc.SetStreamKey(ctx, "pf_twitch_main", "sk_live_main"); err != nil {
		t.Fatalf("SetStreamKey(main) error = %v", err)
	}
	if err := svc.SetStreamKey(ctx, "pf_twitch_backup", "sk_live_backup"); err != nil {
		t.Fatalf("SetStreamKey(backup) error = %v", err)
	}

	main, err := svc.RetrieveForProcessStart(ctx, "pf_twitch_main")
	if err != nil {
		t.Fatalf("RetrieveForProcessStart(main) error = %v", err)
	}
	backup, err := svc.RetrieveForProcessStart(ctx, "pf_twitch_backup")
	if err != nil {
		t.Fatalf("RetrieveForProcessStart(backup) error = %v", err)
	}

	if main != "sk_live_main" || backup != "sk_live_backup" {
		t.Errorf("got main=%q backup=%q, want independent values", main, backup)
	}
}

func TestKeyDerivationIsUnaffectedByAnythingResemblingADisplayName(t *testing.T) {
	// The credential domain never sees a display name - only the platform's
	// generated ID - so a rename at the platform layer cannot orphan a
	// credential; this is true by construction, not by a lookup this
	// package performs.
	if streamKeyKey("pf_stable_id") != secrets.BuildKey(secrets.SecretTypeDestinationStreamKey, "pf_stable_id") {
		t.Fatal("streamKeyKey does not use secrets.BuildKey as its single source of truth")
	}
}

// --- error safety ------------------------------------------------------

func TestNoErrorFromTheServiceEverContainsTheStreamKeyValue(t *testing.T) {
	svc, fake := newTestService()
	ctx := context.Background()

	const secretLookingValue = "sk_live_should_never_appear_in_any_error_or_log"

	if err := svc.SetStreamKey(ctx, "pf_1", secretLookingValue); err != nil {
		t.Fatalf("SetStreamKey() error = %v", err)
	}

	fake.FailNext = errors.New("boom")
	_, _, statusErr := svc.Status(ctx, "pf_1")
	if statusErr != nil && strings.Contains(statusErr.Error(), secretLookingValue) {
		t.Fatalf("Status() error leaked the stored value: %v", statusErr)
	}

	fake.FailNext = errors.New("boom")
	setErr := svc.SetStreamKey(ctx, "pf_1", "sk_live_new_value_also_secret")
	if setErr != nil && strings.Contains(setErr.Error(), "sk_live_new_value_also_secret") {
		t.Fatalf("SetStreamKey() error leaked the value: %v", setErr)
	}

	fake.FailNext = errors.New("boom")
	deleteErr := svc.DeleteStreamKey(ctx, "pf_1")
	if deleteErr != nil && strings.Contains(deleteErr.Error(), secretLookingValue) {
		t.Fatalf("DeleteStreamKey() error leaked the value: %v", deleteErr)
	}
}
