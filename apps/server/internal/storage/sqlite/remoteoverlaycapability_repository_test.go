package sqlite

import (
	"context"
	"testing"

	"github.com/streaming-tree/server/internal/domain/remoteoverlay"
)

func TestRemoteOverlayCapabilityIssueThenResolveRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	cap, err := repo.Issue(ctx, remoteoverlay.DomainChatOverlay, "local-slug-1")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if cap.Token == "" {
		t.Fatal("Issue() returned an empty token")
	}
	if cap.Domain != remoteoverlay.DomainChatOverlay || cap.LocalSlug != "local-slug-1" {
		t.Errorf("capability = %+v", cap)
	}

	resolved, ok, err := repo.Resolve(ctx, remoteoverlay.DomainChatOverlay, cap.Token)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !ok {
		t.Fatal("Resolve() ok = false for a just-issued token, want true")
	}
	if resolved != "local-slug-1" {
		t.Errorf("resolved slug = %q, want %q", resolved, "local-slug-1")
	}
}

func TestRemoteOverlayCapabilityResolveFailsForUnknownToken(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	_, ok, err := repo.Resolve(ctx, remoteoverlay.DomainChatOverlay, "never-issued-token")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ok {
		t.Error("Resolve() ok = true for a token that was never issued, want false")
	}
}

func TestRemoteOverlayCapabilityResolveIsScopedToDomain(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	cap, err := repo.Issue(ctx, remoteoverlay.DomainChatOverlay, "local-slug-1")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	// The same token value, presented against a different domain, must
	// not resolve - a token issued for one domain grants access only
	// within that domain.
	_, ok, err := repo.Resolve(ctx, remoteoverlay.DomainAudio, cap.Token)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ok {
		t.Error("Resolve() ok = true for the wrong domain, want false")
	}
}

func TestRemoteOverlayCapabilityIssueReplacesThePreviousToken(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	first, err := repo.Issue(ctx, remoteoverlay.DomainAudio, "audio-profile-1")
	if err != nil {
		t.Fatalf("first Issue() error = %v", err)
	}
	second, err := repo.Issue(ctx, remoteoverlay.DomainAudio, "audio-profile-1")
	if err != nil {
		t.Fatalf("second Issue() error = %v", err)
	}

	if first.Token == second.Token {
		t.Error("two Issue() calls for the same profile produced the same token")
	}

	// The old token must stop resolving immediately.
	_, ok, err := repo.Resolve(ctx, remoteoverlay.DomainAudio, first.Token)
	if err != nil {
		t.Fatalf("Resolve(first) error = %v", err)
	}
	if ok {
		t.Error("Resolve() ok = true for the rotated-away token, want false")
	}

	resolved, ok, err := repo.Resolve(ctx, remoteoverlay.DomainAudio, second.Token)
	if err != nil {
		t.Fatalf("Resolve(second) error = %v", err)
	}
	if !ok || resolved != "audio-profile-1" {
		t.Errorf("Resolve(second) = %q, %v, want %q, true", resolved, ok, "audio-profile-1")
	}
}

func TestRemoteOverlayCapabilityRevokeRemovesTheCapability(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	cap, err := repo.Issue(ctx, remoteoverlay.DomainWidget, "widget-1")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := repo.Revoke(ctx, remoteoverlay.DomainWidget, "widget-1"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	_, ok, err := repo.Resolve(ctx, remoteoverlay.DomainWidget, cap.Token)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ok {
		t.Error("Resolve() ok = true after Revoke, want false")
	}

	_, found, err := repo.Get(ctx, remoteoverlay.DomainWidget, "widget-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() found = true after Revoke, want false")
	}
}

func TestRemoteOverlayCapabilityRevokeWithNoneIssuedIsNotAnError(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)

	if err := repo.Revoke(context.Background(), remoteoverlay.DomainAlertProfile, "never-issued"); err != nil {
		t.Errorf("Revoke() on an unissued profile error = %v, want nil", err)
	}
}

func TestRemoteOverlayCapabilityGetReflectsAbsence(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)

	_, found, err := repo.Get(context.Background(), remoteoverlay.DomainAlertProfile, "never-issued")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() found = true for a profile with no capability ever issued, want false")
	}
}

func TestRemoteOverlayCapabilityDomainsAreIndependent(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	// The same local_slug value under two different domains must not
	// collide - the UNIQUE constraint is on (domain, local_slug), not
	// local_slug alone.
	if _, err := repo.Issue(ctx, remoteoverlay.DomainChatOverlay, "shared-slug"); err != nil {
		t.Fatalf("Issue(chat-overlay) error = %v", err)
	}
	if _, err := repo.Issue(ctx, remoteoverlay.DomainAudio, "shared-slug"); err != nil {
		t.Fatalf("Issue(audio) error = %v", err)
	}

	_, chatFound, _ := repo.Get(ctx, remoteoverlay.DomainChatOverlay, "shared-slug")
	_, audioFound, _ := repo.Get(ctx, remoteoverlay.DomainAudio, "shared-slug")
	if !chatFound || !audioFound {
		t.Error("issuing a capability for one domain removed or hid the other domain's capability for the same slug")
	}
}

func TestRemoteOverlayCapabilityRejectsAnInvalidDomain(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteOverlayCapabilityRepository(db.DB)
	ctx := context.Background()

	invalid := remoteoverlay.Domain("not-a-real-domain")
	if _, err := repo.Issue(ctx, invalid, "slug"); err == nil {
		t.Error("Issue() with an invalid domain succeeded, want an error")
	}
	if err := repo.Revoke(ctx, invalid, "slug"); err == nil {
		t.Error("Revoke() with an invalid domain succeeded, want an error")
	}
	if _, _, err := repo.Get(ctx, invalid, "slug"); err == nil {
		t.Error("Get() with an invalid domain succeeded, want an error")
	}
	if _, _, err := repo.Resolve(ctx, invalid, "token"); err == nil {
		t.Error("Resolve() with an invalid domain succeeded, want an error")
	}
}
