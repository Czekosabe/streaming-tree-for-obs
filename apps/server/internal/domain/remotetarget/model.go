// Package remotetarget holds the remote-broadcast-target association: which
// remote provider resource (a YouTube live broadcast, today) a configured
// destination's metadata should be read from and published to.
//
// This is deliberately a separate fact from a connected account
// (internal/domain/account) and from a destination's stream key or output-
// server settings - see docs/provider-integrations/youtube.md's "Remote
// broadcast target" section. Nothing here ever stores a token, a stream
// key, an ingestion/stream-name value, or a raw provider response.
package remotetarget

import "time"

// ResourceTypeLiveBroadcast is the only resource type this application
// creates today.
const ResourceTypeLiveBroadcast = "live_broadcast"

// Target is one configured destination's remote resource association.
type Target struct {
	PlatformID   string
	ProviderID   string
	ResourceType string
	ResourceID   string
	DisplayName  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
