// Package engagementsettings holds the one persistent fact Stage 8A saves
// about a connected account's engagement connector: whether it is enabled.
//
// Deliberately minimal and deliberately separate from
// internal/domain/account: everything about a live EventSub connection -
// its WebSocket session, its subscriptions, its reconnect count, its last
// error - is runtime state, kept in memory only (see
// internal/runtime/twitchengagement), never here. Restarting the backend
// resets every one of those; only the enabled/disabled preference survives,
// exactly like a destination's output settings survive while its branch
// runtime state does not.
package engagementsettings

import "time"

// Settings is one connected account's persisted engagement-connector
// preference.
type Settings struct {
	AccountID string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
