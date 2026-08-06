package operatorchat

import (
	"context"
	"testing"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
)

func newTestProjection(t *testing.T, capacity int) (*Projection, *bus.Bus) {
	t.Helper()
	b := bus.New(bus.Options{Capacity: 1000})
	t.Cleanup(b.Shutdown)

	p := New(Options{Source: b, Capacity: capacity})
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		p.Shutdown(ctx)
	})
	return p, b
}

var testTime = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

func chatMessageEvent(accountID, providerEventID, dedupeKey string, userID, login string, text string) engagement.Event {
	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: text}})
	return engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeChatMessage, ProviderEventType: "channel.chat.message",
		ProviderEventID: providerEventID, PlatformTimestamp: testTime, DedupeKey: dedupeKey,
		User:    &engagement.User{ProviderUserID: userID, Login: login, DisplayName: login},
		Message: &msg,
	}
}

func waitForItem(t *testing.T, sub *Subscription, timeout time.Duration) Item {
	t.Helper()
	select {
	case item := <-sub.Items():
		return item
	case <-time.After(timeout):
		t.Fatal("timed out waiting for an item")
		return Item{}
	}
}

func TestChatMessageProjectsToMessageItem(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, err := p.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Cancel()

	if _, _, err := b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "hello")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	item := waitForItem(t, sub, 2*time.Second)
	if item.Kind != KindMessage {
		t.Errorf("Kind = %q, want message", item.Kind)
	}
	if item.Message == nil || item.Message.PlainText != "hello" {
		t.Errorf("Message = %+v, want plain text 'hello'", item.Message)
	}
	if item.User == nil || item.User.Login != "viewer" {
		t.Errorf("User = %+v, want login 'viewer'", item.User)
	}
	if item.ProviderID != "twitch" || item.ConnectedAccountID != "acct_1" {
		t.Errorf("source context = %q/%q, want twitch/acct_1", item.ProviderID, item.ConnectedAccountID)
	}
	if item.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", item.Sequence)
	}
}

func TestMultipleMessagesGetDistinctStableIDs(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer1", "a"))
	b.Publish(chatMessageEvent("acct_1", "msg_2", "dedupe_2", "u2", "viewer2", "b"))

	first := waitForItem(t, sub, time.Second)
	second := waitForItem(t, sub, time.Second)
	if first.ID == second.ID {
		t.Errorf("expected distinct item IDs, got the same: %q", first.ID)
	}
	if first.Sequence >= second.Sequence {
		t.Errorf("expected monotonically increasing sequence, got %d then %d", first.Sequence, second.Sequence)
	}
}

func TestMultipleAccountsMergeInReceiveOrder(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer1", "from account 1"))
	b.Publish(chatMessageEvent("acct_2", "msg_2", "dedupe_2", "u2", "viewer2", "from account 2"))
	b.Publish(chatMessageEvent("acct_1", "msg_3", "dedupe_3", "u1", "viewer1", "from account 1 again"))

	var accounts []string
	for i := 0; i < 3; i++ {
		item := waitForItem(t, sub, time.Second)
		accounts = append(accounts, item.ConnectedAccountID)
	}
	want := []string{"acct_1", "acct_2", "acct_1"}
	for i := range want {
		if accounts[i] != want[i] {
			t.Errorf("accounts[%d] = %q, want %q (receive order across accounts)", i, accounts[i], want[i])
		}
	}
}

func TestExactMessageDeletionUpdatesTheOriginalItem(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "hello"))
	original := waitForItem(t, sub, time.Second)

	deleteEvt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeChatMessageDeleted, ProviderEventType: "channel.chat.message_delete",
		PlatformTimestamp: testTime, DedupeKey: "dedupe_del_1", ModerationRef: "msg_1",
		User: &engagement.User{ProviderUserID: "u1"},
	}
	b.Publish(deleteEvt)
	updated := waitForItem(t, sub, time.Second)

	if updated.ID != original.ID {
		t.Errorf("deletion produced a different item ID (%q) than the original message (%q) - want the same row updated", updated.ID, original.ID)
	}
	if !updated.Lifecycle.Deleted {
		t.Error("expected Lifecycle.Deleted = true")
	}
	if updated.Message == nil || updated.Message.PlainText != "hello" {
		t.Errorf("expected the original message content preserved after deletion, got %+v", updated.Message)
	}
	if updated.Sequence <= original.Sequence {
		t.Errorf("expected the deletion revision to have a higher sequence than the original (%d), got %d", original.Sequence, updated.Sequence)
	}
}

func TestDeletionOfAnEvictedMessageAddsASystemItemWithoutInventingContent(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	deleteEvt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeChatMessageDeleted, ProviderEventType: "channel.chat.message_delete",
		PlatformTimestamp: testTime, DedupeKey: "dedupe_del_1", ModerationRef: "msg_never_seen",
		User: &engagement.User{ProviderUserID: "u1"},
	}
	b.Publish(deleteEvt)
	item := waitForItem(t, sub, time.Second)

	if item.Kind != KindModeration {
		t.Errorf("Kind = %q, want moderation", item.Kind)
	}
	if item.Message != nil {
		t.Error("expected no fabricated message content for an evicted-message deletion")
	}
	if item.Moderation == nil || item.Moderation.Action != "message_deleted_not_retained" {
		t.Errorf("Moderation = %+v, want action message_deleted_not_retained", item.Moderation)
	}
}

func TestChatClearMarksOnlyThatAccountsMessagesDeleted(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer1", "a"))
	b.Publish(chatMessageEvent("acct_2", "msg_2", "dedupe_2", "u2", "viewer2", "b"))
	waitForItem(t, sub, time.Second)
	waitForItem(t, sub, time.Second)

	clearEvt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeChatCleared, ProviderEventType: "channel.chat.clear",
		PlatformTimestamp: testTime, DedupeKey: "dedupe_clear_1",
	}
	b.Publish(clearEvt)

	deletedForAcct1 := waitForItem(t, sub, time.Second)
	systemItem := waitForItem(t, sub, time.Second)

	if deletedForAcct1.ConnectedAccountID != "acct_1" || !deletedForAcct1.Lifecycle.Deleted {
		t.Errorf("expected acct_1's message marked deleted, got %+v", deletedForAcct1)
	}
	if systemItem.Kind != KindSystem {
		t.Errorf("expected a trailing system item, got kind %q", systemItem.Kind)
	}

	acct2Msg, ok := p.lookupMessage(messageItemID(engagement.ProviderTwitch, "acct_2", "msg_2"))
	if !ok || acct2Msg.Lifecycle.Deleted {
		t.Errorf("expected acct_2's message to remain untouched, got found=%v deleted=%v", ok, acct2Msg.Lifecycle.Deleted)
	}
}

func TestClearUserMessagesOnlyAffectsThatUser(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "target", "spam"))
	b.Publish(chatMessageEvent("acct_1", "msg_2", "dedupe_2", "u2", "other", "hi"))
	waitForItem(t, sub, time.Second)
	waitForItem(t, sub, time.Second)

	modEvt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeModeration, ProviderEventType: "channel.chat.clear_user_messages",
		PlatformTimestamp: testTime, DedupeKey: "dedupe_mod_1", ModerationAction: "clear_user_messages",
		User: &engagement.User{ProviderUserID: "u1"},
	}
	// Deliberately no ModerationRef - this must still validate and process,
	// preserving the stage 8A validation fix for clear_user_messages.
	if err := modEvt.Validate(); err != nil {
		t.Fatalf("clear_user_messages event unexpectedly failed validation without a moderationRef: %v", err)
	}
	b.Publish(modEvt)

	deletedItem := waitForItem(t, sub, time.Second)
	systemItem := waitForItem(t, sub, time.Second)

	if deletedItem.User == nil || deletedItem.User.ProviderUserID != "u1" || !deletedItem.Lifecycle.Deleted {
		t.Errorf("expected target user's message deleted, got %+v", deletedItem)
	}
	if systemItem.Kind != KindModeration || systemItem.Moderation.Action != "user_messages_cleared" {
		t.Errorf("expected a moderation system item, got %+v", systemItem)
	}

	other, ok := p.lookupMessage(messageItemID(engagement.ProviderTwitch, "acct_1", "msg_2"))
	if !ok || other.Lifecycle.Deleted {
		t.Errorf("expected the other user's message untouched, got found=%v deleted=%v", ok, other.Lifecycle.Deleted)
	}
}

func testActivityEvent(typ engagement.Type, accountID string) engagement.Event {
	return engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: typ, ProviderEventType: string(typ),
		PlatformTimestamp: testTime, DedupeKey: "dedupe_" + string(typ) + "_" + accountID,
		User: &engagement.User{ProviderUserID: "u1", Login: "viewer"},
	}
}

func TestActivityEventsMapToActivityItemsWithoutRelabeling(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	cases := []engagement.Type{
		engagement.TypeFollow, engagement.TypeSubscription, engagement.TypeResubscription,
		engagement.TypeGiftedSubscription, engagement.TypeBits, engagement.TypeRaid,
		engagement.TypeChannelPointRedemption, engagement.TypeStreamOnline, engagement.TypeStreamOffline,
	}
	for i, typ := range cases {
		evt := testActivityEvent(typ, "acct_1")
		evt.DedupeKey = evt.DedupeKey + string(rune('a'+i))
		if typ == engagement.TypeBits {
			qty := int64(100)
			evt.Quantity = &qty
		}
		b.Publish(evt)
	}

	for _, typ := range cases {
		item := waitForItem(t, sub, time.Second)
		if item.Kind != KindActivity {
			t.Fatalf("Kind = %q, want activity for %s", item.Kind, typ)
		}
		if item.Activity == nil || item.Activity.ActivityType != string(typ) {
			t.Errorf("Activity.ActivityType = %+v, want %q (never relabeled)", item.Activity, typ)
		}
	}
}

func TestGiftBatchAndRecipientGiftStayAsSeparateItems(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	batch := testActivityEvent(engagement.TypeSubscriptionGiftBatch, "acct_1")
	qty := int64(5)
	batch.Quantity = &qty
	recipient := testActivityEvent(engagement.TypeGiftedSubscription, "acct_1")
	recipient.DedupeKey = "dedupe_recipient"

	b.Publish(batch)
	b.Publish(recipient)

	first := waitForItem(t, sub, time.Second)
	second := waitForItem(t, sub, time.Second)

	if first.Activity.ActivityType != string(engagement.TypeSubscriptionGiftBatch) {
		t.Errorf("first.Activity.ActivityType = %q, want subscription_gift_batch", first.Activity.ActivityType)
	}
	if second.Activity.ActivityType != string(engagement.TypeGiftedSubscription) {
		t.Errorf("second.Activity.ActivityType = %q, want gifted_subscription", second.Activity.ActivityType)
	}
	if first.ID == second.ID {
		t.Error("expected the gift batch and the recipient gift to be distinct items")
	}
}

func TestUnknownNormalizedTypeIsIgnoredSafely(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	// Publish a recognized event first, then a synthetic "unknown type"
	// bypassing engagement.Event.Validate (which would itself reject an
	// unknown type) to directly exercise the projection's own defensive
	// default branch, then a second recognized event - the projection must
	// keep working, never crash or stall, and must not emit anything for
	// the unknown one.
	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "first"))
	waitForItem(t, sub, time.Second)

	p.handleEvent(engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: "some.future.type", PlatformTimestamp: testTime,
	})

	b.Publish(chatMessageEvent("acct_1", "msg_2", "dedupe_2", "u1", "viewer", "second"))
	item := waitForItem(t, sub, time.Second)
	if item.Message.PlainText != "second" {
		t.Errorf("expected the projection to keep working after an unknown type, got %+v", item)
	}
}

func TestSyntheticMarkerIsPreserved(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	evt := chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "test")
	evt.Synthetic = true
	b.Publish(evt)

	item := waitForItem(t, sub, time.Second)
	if !item.Synthetic {
		t.Error("expected Synthetic = true to be preserved")
	}
}

func TestAnonymousActorHasNoFabricatedIdentity(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	evt := testActivityEvent(engagement.TypeBits, "acct_1")
	evt.User = &engagement.User{Anonymous: true}
	qty := int64(50)
	evt.Quantity = &qty
	b.Publish(evt)

	item := waitForItem(t, sub, time.Second)
	if item.User == nil || !item.User.Anonymous {
		t.Fatalf("expected Anonymous = true, got %+v", item.User)
	}
	if item.User.ProviderUserID != "" || item.User.Login != "" {
		t.Errorf("expected no fabricated identity for an anonymous actor, got %+v", item.User)
	}
}

func TestLongUsernameAndLongMessageAreNotTruncated(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	longName := ""
	for i := 0; i < 200; i++ {
		longName += "a"
	}
	longMessage := ""
	for i := 0; i < 3000; i++ {
		longMessage += "b"
	}
	evt := chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", longName, longMessage)
	b.Publish(evt)

	item := waitForItem(t, sub, time.Second)
	if item.User.Login != longName {
		t.Error("expected the long username preserved verbatim")
	}
	if item.Message.PlainText != longMessage {
		t.Error("expected the long message preserved verbatim")
	}
}

func TestMissingOptionalFieldsDoNotPanic(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	// A follow event with no color/badges/avatar set - only the required
	// fields.
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, ProviderEventType: "channel.follow",
		PlatformTimestamp: testTime, DedupeKey: "dedupe_follow_1",
		User: &engagement.User{ProviderUserID: "u1"},
	}
	b.Publish(evt)
	item := waitForItem(t, sub, time.Second)
	if item.User.Color != "" || len(item.User.Badges) != 0 {
		t.Errorf("expected no invented color/badges, got %+v", item.User)
	}
}
