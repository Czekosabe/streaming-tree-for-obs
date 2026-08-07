package operatorchat

import engagement "github.com/streaming-tree/server/internal/domain/engagement"

// messageItemID deterministically derives a message item's stable ID from
// the provider event that created it, so a later chat.message_deleted
// event (which references the same provider event id via ModerationRef)
// can look the item up directly without a separate index structure.
func messageItemID(providerID engagement.ProviderID, connectedAccountID, providerEventID string) string {
	return "chat_" + string(providerID) + "_" + connectedAccountID + "_" + providerEventID
}

// buildMessageItem converts a normalized chat.message event into a message
// item. Does not mutate evt.
func (p *Projection) buildMessageItem(evt engagement.Event) Item {
	item := Item{
		ID:                 messageItemID(evt.ProviderID, evt.ConnectedAccountID, evt.ProviderEventID),
		SourceEventID:      evt.ID,
		ProviderMessageID:  evt.ProviderEventID,
		ProviderID:         string(evt.ProviderID),
		ConnectedAccountID: evt.ConnectedAccountID,
		DestinationID:      p.resolveDestination(evt.ConnectedAccountID),
		Kind:               KindMessage,
		OccurredAt:         evt.PlatformTimestamp,
		ReceivedAt:         evt.ReceivedAt,
		Synthetic:          evt.Synthetic,
		Lifecycle:          Lifecycle{},
	}
	if evt.User != nil {
		item.User = toItemUser(evt.User)
	}
	if evt.Message != nil {
		item.Message = toItemMessage(evt.Message)
	}
	return item
}

func toItemUser(u *engagement.User) *User {
	badges := make([]Badge, 0, len(u.Badges))
	for _, b := range u.Badges {
		badges = append(badges, Badge{SetID: b.SetID, ID: b.ID, Info: b.Info})
	}
	return &User{
		ProviderUserID: u.ProviderUserID,
		Login:          u.Login,
		DisplayName:    u.DisplayName,
		AvatarURL:      u.AvatarURL,
		Color:          u.Color,
		Badges:         badges,
		Anonymous:      u.Anonymous,
	}
}

func toItemMessage(m *engagement.Message) *Message {
	fragments := make([]Fragment, 0, len(m.Fragments))
	for _, f := range m.Fragments {
		fragments = append(fragments, Fragment{
			Type: FragmentType(f.Type), Text: f.Text, EmoteID: f.EmoteID,
			CheermotePrefix: f.CheermotePrefix, CheermoteBits: f.CheermoteBits,
			MentionUserID: f.MentionUserID, MentionLogin: f.MentionLogin, MentionDisplayName: f.MentionDisplayName,
		})
	}
	return &Message{PlainText: m.Text, Fragments: fragments}
}
