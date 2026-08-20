package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

func TestRemoteIngestPublisherProvisionedReflectsState(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	provisioned, err := RemoteIngestPublisherProvisioned(ctx, store)
	if err != nil {
		t.Fatalf("RemoteIngestPublisherProvisioned() error = %v", err)
	}
	if provisioned {
		t.Error("RemoteIngestPublisherProvisioned() = true before any credential was provisioned, want false")
	}

	if _, err := ProvisionRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}

	provisioned, err = RemoteIngestPublisherProvisioned(ctx, store)
	if err != nil {
		t.Fatalf("RemoteIngestPublisherProvisioned() error = %v", err)
	}
	if !provisioned {
		t.Error("RemoteIngestPublisherProvisioned() = false after provisioning, want true")
	}
}

func TestProvisionRemoteIngestPublisherCredentialReturnsTheSecretOnce(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	secret, err := ProvisionRemoteIngestPublisherCredential(ctx, store)
	if err != nil {
		t.Fatalf("ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}
	if secret == "" {
		t.Fatal("ProvisionRemoteIngestPublisherCredential() returned an empty secret")
	}

	verifier, ok, err := RemoteIngestPublisherVerifier(ctx, store)
	if err != nil {
		t.Fatalf("RemoteIngestPublisherVerifier() error = %v", err)
	}
	if !ok {
		t.Fatal("RemoteIngestPublisherVerifier() reports no verifier after provisioning")
	}
	if verifier == secret {
		t.Fatal("the stored verifier equals the plaintext secret")
	}
	if strings.Contains(verifier, secret) {
		t.Fatal("the stored verifier contains the plaintext secret as a substring")
	}
	if !strings.HasPrefix(verifier, "sha256:") {
		t.Errorf("verifier = %q, want the sha256: prefix", verifier)
	}
}

func TestRemoteIngestPublisherVerifierReflectsNoneProvisioned(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	verifier, ok, err := RemoteIngestPublisherVerifier(ctx, store)
	if err != nil {
		t.Fatalf("RemoteIngestPublisherVerifier() error = %v, want nil (treated as absent, not an error)", err)
	}
	if ok {
		t.Error("RemoteIngestPublisherVerifier() ok = true with nothing ever provisioned, want false")
	}
	if verifier != "" {
		t.Errorf("verifier = %q, want empty", verifier)
	}
}

func TestProvisionRemoteIngestPublisherCredentialOverwritesPrevious(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	first, err := ProvisionRemoteIngestPublisherCredential(ctx, store)
	if err != nil {
		t.Fatalf("first ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}
	firstVerifier, _, _ := RemoteIngestPublisherVerifier(ctx, store)

	second, err := ProvisionRemoteIngestPublisherCredential(ctx, store)
	if err != nil {
		t.Fatalf("second ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}
	secondVerifier, _, _ := RemoteIngestPublisherVerifier(ctx, store)

	if first == second {
		t.Error("rotation produced the same plaintext secret twice")
	}
	if firstVerifier == secondVerifier {
		t.Error("rotation produced the same stored verifier twice")
	}
}

func TestRevokeRemoteIngestPublisherCredentialRemovesTheVerifier(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	if _, err := ProvisionRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}
	if err := RevokeRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("RevokeRemoteIngestPublisherCredential() error = %v", err)
	}

	provisioned, err := RemoteIngestPublisherProvisioned(ctx, store)
	if err != nil {
		t.Fatalf("RemoteIngestPublisherProvisioned() error = %v", err)
	}
	if provisioned {
		t.Error("RemoteIngestPublisherProvisioned() = true after revoke, want false")
	}
	_, ok, err := RemoteIngestPublisherVerifier(ctx, store)
	if err != nil {
		t.Fatalf("RemoteIngestPublisherVerifier() error = %v", err)
	}
	if ok {
		t.Error("RemoteIngestPublisherVerifier() ok = true after revoke, want false")
	}
}

func TestRevokeRemoteIngestPublisherCredentialIsIdempotent(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	// Revoking with nothing ever provisioned must not be an error - the
	// end state (no credential) is exactly what the caller wanted.
	if err := RevokeRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("RevokeRemoteIngestPublisherCredential() on an empty store error = %v, want nil", err)
	}

	if _, err := ProvisionRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}
	if err := RevokeRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("first RevokeRemoteIngestPublisherCredential() error = %v", err)
	}
	if err := RevokeRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("second RevokeRemoteIngestPublisherCredential() (already revoked) error = %v, want nil", err)
	}
}

func TestRemoteIngestCredentialDoesNotShareStorageWithTheAdminPassword(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	if err := SetAdminPassword(ctx, store, "an-admin-password"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}
	if _, err := ProvisionRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("ProvisionRemoteIngestPublisherCredential() error = %v", err)
	}

	adminProvisioned, err := AdminPasswordProvisioned(ctx, store)
	if err != nil || !adminProvisioned {
		t.Errorf("AdminPasswordProvisioned() = %v, %v, want true, nil", adminProvisioned, err)
	}
	ingestProvisioned, err := RemoteIngestPublisherProvisioned(ctx, store)
	if err != nil || !ingestProvisioned {
		t.Errorf("RemoteIngestPublisherProvisioned() = %v, %v, want true, nil", ingestProvisioned, err)
	}

	if err := RevokeRemoteIngestPublisherCredential(ctx, store); err != nil {
		t.Fatalf("RevokeRemoteIngestPublisherCredential() error = %v", err)
	}
	adminStillProvisioned, err := AdminPasswordProvisioned(ctx, store)
	if err != nil || !adminStillProvisioned {
		t.Error("revoking the remote-ingest credential also removed the admin password - they must be independent keys")
	}
}
