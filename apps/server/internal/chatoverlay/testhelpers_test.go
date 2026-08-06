package chatoverlay

import (
	"time"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

var testTime = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

func testProfile(id string, mutate func(*chatoverlaydomain.Profile)) chatoverlaydomain.Profile {
	p := chatoverlaydomain.Default("Test Overlay")
	p.ID = id
	p.PublicSlug = id + "_slug"
	if mutate != nil {
		mutate(&p)
	}
	return p
}

func testSettings(mutate func(*chatoverlaydomain.Profile)) resolvedSettings {
	return resolvedSettings{profile: testProfile("ov_test", mutate)}
}

func messageItem(id, accountID, providerUserID, login, text string) operatorchat.Item {
	return operatorchat.Item{
		Version: 1, ID: id, ProviderID: "twitch", ConnectedAccountID: accountID,
		Kind: operatorchat.KindMessage, OccurredAt: testTime, ReceivedAt: testTime,
		User:    &operatorchat.User{ProviderUserID: providerUserID, Login: login, DisplayName: login, Color: "#112233"},
		Message: &operatorchat.Message{PlainText: text, Fragments: []operatorchat.Fragment{{Type: operatorchat.FragmentText, Text: text}}},
	}
}

func messageItemWithBadges(id, accountID, providerUserID, login, text string, badges []operatorchat.Badge) operatorchat.Item {
	item := messageItem(id, accountID, providerUserID, login, text)
	item.User.Badges = badges
	return item
}

func anonymousActivityItem(id, accountID, activityType string) operatorchat.Item {
	return operatorchat.Item{
		ID: id, ProviderID: "twitch", ConnectedAccountID: accountID, Kind: operatorchat.KindActivity,
		OccurredAt: testTime, ReceivedAt: testTime,
		Activity: &operatorchat.Activity{ActivityType: activityType},
	}
}

func moderationItem(id, accountID string) operatorchat.Item {
	return operatorchat.Item{
		ID: id, ProviderID: "twitch", ConnectedAccountID: accountID, Kind: operatorchat.KindModeration,
		OccurredAt: testTime, ReceivedAt: testTime,
		Moderation: &operatorchat.ModerationInfo{Action: "message_deleted"},
	}
}

func systemItem(id, accountID string) operatorchat.Item {
	return operatorchat.Item{
		ID: id, ProviderID: "twitch", ConnectedAccountID: accountID, Kind: operatorchat.KindSystem,
		OccurredAt: testTime, ReceivedAt: testTime,
		Moderation: &operatorchat.ModerationInfo{Action: "chat_cleared"},
	}
}

func deletedMessageItem(id, accountID, providerUserID, login, text string) operatorchat.Item {
	return deletedMessageItemWithReason(id, accountID, providerUserID, login, text, operatorchat.DeletionReasonModeratorDeleted)
}

func deletedMessageItemWithReason(id, accountID, providerUserID, login, text string, reason operatorchat.DeletionReason) operatorchat.Item {
	item := messageItem(id, accountID, providerUserID, login, text)
	item.Lifecycle = operatorchat.Lifecycle{Deleted: true, DeletionReason: reason}
	return item
}

// fakeSource is an in-memory, concurrency-safe OperatorChatSource - the
// same role a real internal/operatorchat.Projection plays for
// Projection.Configure's own rebuild replay.
type fakeSource struct {
	items []operatorchat.Item
	gap   bool
}

func (f *fakeSource) ItemsAfter(after uint64, limit int) ([]operatorchat.Item, bool) {
	out := make([]operatorchat.Item, len(f.items))
	copy(out, f.items)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, f.gap
}

func (f *fakeSource) add(item operatorchat.Item) {
	f.items = append(f.items, item)
}

func waitRevision(deadline time.Duration, ch <-chan Revision) (Revision, bool) {
	select {
	case rev, ok := <-ch:
		return rev, ok
	case <-time.After(deadline):
		return Revision{}, false
	}
}
