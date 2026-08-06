package operatorchat

import engagement "github.com/streaming-tree/server/internal/domain/engagement"

// lookupMessage returns the current retained state of a message item, if
// any is still retained under that ID.
func (p *Projection) lookupMessage(id string) (Item, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.latestByID[id]
	return item, ok
}

// messagesMatching returns every currently-retained, not-yet-deleted
// message item matching pred - used by the whole-chat and per-user clear
// handlers below.
func (p *Projection) messagesMatching(pred func(Item) bool) []Item {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Item
	for _, item := range p.latestByID {
		if !item.Lifecycle.Deleted && pred(item) {
			out = append(out, item)
		}
	}
	return out
}

func markDeleted(item Item, at engagement.Event, reason DeletionReason, sourceEventID string) Item {
	item.SourceEventID = sourceEventID
	deletedAt := at.ReceivedAt
	item.Lifecycle = Lifecycle{Deleted: true, DeletedAt: &deletedAt, DeletionReason: reason}
	return item
}

// applyMessageDeleted handles channel.chat.message_delete (normalized as
// TypeChatMessageDeleted, ModerationRef = the deleted message's provider
// event id).
//
// If the referenced message is still retained, the SAME item ID is
// updated in place (a new revision, never a second row for the same
// message) - the moderator sees exactly what was said, marked deleted.
// If it is no longer retained (evicted, or arrived before this process
// started), a small moderation item says so without inventing content.
func (p *Projection) applyMessageDeleted(evt engagement.Event) []Item {
	id := messageItemID(evt.ProviderID, evt.ConnectedAccountID, evt.ModerationRef)
	if existing, ok := p.lookupMessage(id); ok {
		updated := markDeleted(existing, evt, DeletionReasonModeratorDeleted, evt.ID)
		return []Item{updated}
	}

	return []Item{{
		ID:                 newItemID("mod"),
		SourceEventID:      evt.ID,
		ProviderID:         string(evt.ProviderID),
		ConnectedAccountID: evt.ConnectedAccountID,
		DestinationID:      p.resolveDestination(evt.ConnectedAccountID),
		Kind:               KindModeration,
		OccurredAt:         evt.PlatformTimestamp,
		ReceivedAt:         evt.ReceivedAt,
		Synthetic:          evt.Synthetic,
		Moderation: &ModerationInfo{
			Action:           "message_deleted_not_retained",
			TargetMessageRef: evt.ModerationRef,
		},
	}}
}

// applyChatCleared handles channel.chat.clear (TypeChatCleared): marks
// every currently-retained message from the same provider+account context
// deleted, and adds one system divider item. Unrelated accounts are
// untouched - the filter is scoped to evt.ConnectedAccountID.
func (p *Projection) applyChatCleared(evt engagement.Event) []Item {
	affected := p.messagesMatching(func(item Item) bool {
		return item.ProviderID == string(evt.ProviderID) && item.ConnectedAccountID == evt.ConnectedAccountID
	})

	items := make([]Item, 0, len(affected)+1)
	for _, item := range affected {
		items = append(items, markDeleted(item, evt, DeletionReasonChatCleared, evt.ID))
	}
	items = append(items, Item{
		ID:                 newItemID("sys"),
		SourceEventID:      evt.ID,
		ProviderID:         string(evt.ProviderID),
		ConnectedAccountID: evt.ConnectedAccountID,
		DestinationID:      p.resolveDestination(evt.ConnectedAccountID),
		Kind:               KindSystem,
		OccurredAt:         evt.PlatformTimestamp,
		ReceivedAt:         evt.ReceivedAt,
		Synthetic:          evt.Synthetic,
		Moderation:         &ModerationInfo{Action: "chat_cleared"},
	})
	return items
}

// applyModeration handles the normalized "moderation" type - in Stage 9
// this is exactly channel.chat.clear_user_messages
// (evt.ModerationAction == "clear_user_messages"), which targets a user,
// not a specific prior message - it deliberately does not require
// ModerationRef, preserving the stage 8A validation fix that made this
// event valid with only a ModerationAction. Other users, and other
// accounts, are left untouched.
func (p *Projection) applyModeration(evt engagement.Event) []Item {
	if evt.ModerationAction != "clear_user_messages" {
		// A future moderation action this projection does not yet know
		// how to present - ignored safely rather than guessed at.
		return nil
	}
	targetUserID := ""
	if evt.User != nil {
		targetUserID = evt.User.ProviderUserID
	}

	affected := p.messagesMatching(func(item Item) bool {
		return item.ProviderID == string(evt.ProviderID) &&
			item.ConnectedAccountID == evt.ConnectedAccountID &&
			item.User != nil && item.User.ProviderUserID == targetUserID
	})

	items := make([]Item, 0, len(affected)+1)
	for _, item := range affected {
		items = append(items, markDeleted(item, evt, DeletionReasonUserMessagesCleared, evt.ID))
	}
	items = append(items, Item{
		ID:                 newItemID("sys"),
		SourceEventID:      evt.ID,
		ProviderID:         string(evt.ProviderID),
		ConnectedAccountID: evt.ConnectedAccountID,
		DestinationID:      p.resolveDestination(evt.ConnectedAccountID),
		Kind:               KindModeration,
		OccurredAt:         evt.PlatformTimestamp,
		ReceivedAt:         evt.ReceivedAt,
		Synthetic:          evt.Synthetic,
		Moderation:         &ModerationInfo{Action: "user_messages_cleared", TargetUserID: targetUserID},
	})
	return items
}
