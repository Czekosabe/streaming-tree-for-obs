package chatoverlay

import (
	"context"
	"testing"
	"time"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// --- deletionRemoveReason mapping --------------------------------------

func TestDeletionRemoveReasonMapsEveryKnownOperatorChatReason(t *testing.T) {
	tests := []struct {
		name   string
		reason operatorchat.DeletionReason
		want   RemoveReason
	}{
		{"moderator delete", operatorchat.DeletionReasonModeratorDeleted, RemoveReasonMessageDeleted},
		{"whole chat clear", operatorchat.DeletionReasonChatCleared, RemoveReasonChatCleared},
		{"per-user clear", operatorchat.DeletionReasonUserMessagesCleared, RemoveReasonUserMessagesCleared},
		{"unrecognized reason falls back to unknown, never guessed", operatorchat.DeletionReason("something_new"), RemoveReasonUnknown},
		{"empty reason falls back to unknown", operatorchat.DeletionReason(""), RemoveReasonUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := deletedMessageItemWithReason("m1", "acct_1", "u1", "viewer", "hi", tt.reason)
			if got := deletionRemoveReason(item); got != tt.want {
				t.Errorf("deletionRemoveReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- RemoveReason.IsCosmetic --------------------------------------------

func TestRemoveReasonIsCosmetic(t *testing.T) {
	cosmetic := []RemoveReason{RemoveReasonExpired, RemoveReasonCapacityEvicted}
	for _, r := range cosmetic {
		if !r.IsCosmetic() {
			t.Errorf("%q should be cosmetic (safe to animate)", r)
		}
	}

	immediate := []RemoveReason{
		RemoveReasonMessageDeleted, RemoveReasonChatCleared, RemoveReasonUserMessagesCleared,
		RemoveReasonUnknown, RemoveReason("configuration_changed"), RemoveReason(""),
	}
	for _, r := range immediate {
		if r.IsCosmetic() {
			t.Errorf("%q must never be cosmetic - only expiry and capacity eviction are safe to animate", r)
		}
	}
}

// --- Projection-level remove-reason wiring -------------------------------

func TestProjectionExpiryRemovalCarriesExpiredReason(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MessageLifetimeSeconds = 1 })
	source := &fakeSource{}
	p := NewProjection(settings.profile.ID, source, nil, nil)
	p.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		p.Shutdown(ctx)
	})
	p.Configure(settings)

	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	item.OccurredAt = time.Now().Add(-900 * time.Millisecond) // ~100ms until expiry
	source.add(item)
	p.HandleUpstreamItem(item)
	waitRevision(time.Second, sub.Revisions()) // upsert

	rev, ok := waitRevision(2*time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove {
		t.Fatalf("expected an expiry remove, got %+v, ok=%v", rev, ok)
	}
	if rev.Reason != RemoveReasonExpired {
		t.Errorf("Reason = %q, want %q", rev.Reason, RemoveReasonExpired)
	}
	if !rev.Reason.IsCosmetic() {
		t.Error("expiry must be a cosmetic reason")
	}
}

func TestProjectionCapacityEvictionCarriesCapacityEvictedReason(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MaxVisibleItems = 2 })
	p, source := newTestProjection(t, settings)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	for _, id := range []string{"m1", "m2", "m3"} {
		item := messageItem(id, "acct_1", "u1", "viewer", "hi")
		source.add(item)
		p.HandleUpstreamItem(item)
	}

	var evictionRev Revision
	found := false
	for i := 0; i < 4; i++ {
		rev, ok := waitRevision(time.Second, sub.Revisions())
		if !ok {
			break
		}
		if rev.Operation == OpRemove && rev.RemovedID == "m1" {
			evictionRev = rev
			found = true
		}
	}
	if !found {
		t.Fatal("expected an eviction remove for m1")
	}
	if evictionRev.Reason != RemoveReasonCapacityEvicted {
		t.Errorf("Reason = %q, want %q", evictionRev.Reason, RemoveReasonCapacityEvicted)
	}
	if !evictionRev.Reason.IsCosmetic() {
		t.Error("capacity eviction must be a cosmetic reason")
	}
}

func TestProjectionMessageDeletionCarriesImmediateMessageDeletedReason(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = false })
	p, source := newTestProjection(t, settings)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)
	waitRevision(time.Second, sub.Revisions()) // upsert

	deleted := deletedMessageItemWithReason("m1", "acct_1", "u1", "viewer", "hi", operatorchat.DeletionReasonModeratorDeleted)
	source.add(deleted)
	p.HandleUpstreamItem(deleted)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove {
		t.Fatalf("expected a moderation remove, got %+v, ok=%v", rev, ok)
	}
	if rev.Reason != RemoveReasonMessageDeleted {
		t.Errorf("Reason = %q, want %q", rev.Reason, RemoveReasonMessageDeleted)
	}
	if rev.Reason.IsCosmetic() {
		t.Error("a moderator deletion must never be cosmetic - it must be immediate")
	}
}

func TestProjectionChatClearCarriesImmediateChatClearedReason(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = false })
	p, source := newTestProjection(t, settings)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)
	waitRevision(time.Second, sub.Revisions()) // upsert

	cleared := deletedMessageItemWithReason("m1", "acct_1", "u1", "viewer", "hi", operatorchat.DeletionReasonChatCleared)
	source.add(cleared)
	p.HandleUpstreamItem(cleared)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove {
		t.Fatalf("expected a chat-cleared remove, got %+v, ok=%v", rev, ok)
	}
	if rev.Reason != RemoveReasonChatCleared {
		t.Errorf("Reason = %q, want %q", rev.Reason, RemoveReasonChatCleared)
	}
	if rev.Reason.IsCosmetic() {
		t.Error("a whole-chat clear must never be cosmetic - it must be immediate")
	}
}

func TestProjectionUserMessagesClearCarriesImmediateUserMessagesClearedReason(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = false })
	p, source := newTestProjection(t, settings)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)
	waitRevision(time.Second, sub.Revisions()) // upsert

	cleared := deletedMessageItemWithReason("m1", "acct_1", "u1", "viewer", "hi", operatorchat.DeletionReasonUserMessagesCleared)
	source.add(cleared)
	p.HandleUpstreamItem(cleared)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove {
		t.Fatalf("expected a user-messages-cleared remove, got %+v, ok=%v", rev, ok)
	}
	if rev.Reason != RemoveReasonUserMessagesCleared {
		t.Errorf("Reason = %q, want %q", rev.Reason, RemoveReasonUserMessagesCleared)
	}
	if rev.Reason.IsCosmetic() {
		t.Error("a per-user clear must never be cosmetic - it must be immediate")
	}
}

// --- Filter/settings changes never produce an OpRemove at all -----------

func TestProjectionSettingsChangeNeverProducesAnOpRemove(t *testing.T) {
	// Hiding a previously-visible user must remove it immediately, but
	// in this design that always travels as a full OpReset (Configure's
	// own rebuild), never as an individual OpRemove with a filter-shaped
	// reason - see RemoveReason's own doc comment for why.
	p, source := newTestProjection(t, testSettings(nil))
	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)

	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	hidden := testSettings(nil)
	hidden.hiddenUsers = toSet([]string{userKey("twitch", "acct_1", "u1")})
	p.Configure(hidden)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok {
		t.Fatal("expected a revision after the settings change")
	}
	if rev.Operation != OpReset {
		t.Errorf("Operation = %q, want %q - a settings change must never emit an individual OpRemove", rev.Operation, OpReset)
	}
	if len(rev.ResetItems) != 0 {
		t.Errorf("expected the newly-hidden user's item to be absent from the reset, got %+v", rev.ResetItems)
	}
}

// --- Replay preserves the reason ----------------------------------------

func TestSubscribeReplayPreservesRemoveReason(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MaxVisibleItems = 1 })
	p, source := newTestProjection(t, settings)

	item1 := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item1)
	p.HandleUpstreamItem(item1)
	item2 := messageItem("m2", "acct_1", "u1", "viewer", "hi again")
	source.add(item2)
	p.HandleUpstreamItem(item2) // evicts m1 with RemoveReasonCapacityEvicted

	// A subscriber connecting AFTER the fact still replays the retained
	// remove revision, reason intact.
	sub, _, err := p.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Cancel()

	var sawEvictionWithReason bool
	for i := 0; i < 4; i++ {
		rev, ok := waitRevision(time.Second, sub.Revisions())
		if !ok {
			break
		}
		if rev.Operation == OpRemove && rev.RemovedID == "m1" {
			if rev.Reason != RemoveReasonCapacityEvicted {
				t.Errorf("replayed Reason = %q, want %q", rev.Reason, RemoveReasonCapacityEvicted)
			}
			sawEvictionWithReason = true
		}
	}
	if !sawEvictionWithReason {
		t.Fatal("expected the replayed stream to include the eviction remove")
	}
}

// --- Duplicate replay produces the same final state ----------------------

func TestReplayedCosmeticRemoveIsHarmlessOnDuplicateApplication(t *testing.T) {
	// A client that already applied a cosmetic remove (and has already
	// finished its own local exit animation) must end up in the same
	// final state if the same remove is somehow delivered again - the
	// backend is the source of truth for whether an item exists; the
	// animation is only frontend presentation and must never desync
	// from it. This is exercised here at the ring/replay level: the
	// same Revision value structurally carries the same id and reason
	// every time it is replayed, so folding it twice on the frontend
	// (upsert-by-id / delete-by-id) is idempotent by construction.
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MaxVisibleItems = 1 })
	p, source := newTestProjection(t, settings)
	item1 := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item1)
	p.HandleUpstreamItem(item1)
	item2 := messageItem("m2", "acct_1", "u1", "viewer", "hi again")
	source.add(item2)
	p.HandleUpstreamItem(item2)

	first, _, _ := p.Subscribe(0)
	defer first.Cancel()
	second, _, _ := p.Subscribe(0)
	defer second.Cancel()

	var firstRemove, secondRemove Revision
	for i := 0; i < 4; i++ {
		rev, ok := waitRevision(time.Second, first.Revisions())
		if ok && rev.Operation == OpRemove {
			firstRemove = rev
		}
	}
	for i := 0; i < 4; i++ {
		rev, ok := waitRevision(time.Second, second.Revisions())
		if ok && rev.Operation == OpRemove {
			secondRemove = rev
		}
	}
	if firstRemove.RemovedID != secondRemove.RemovedID || firstRemove.Reason != secondRemove.Reason {
		t.Errorf("two independent replays disagree: %+v vs %+v", firstRemove, secondRemove)
	}
}
