// Package output holds the destination output-configuration domain: the
// non-secret RTMP/RTMPS server address and restart preference for one
// configured platform.
//
// Nothing in this package stores or accepts a stream key. The server address
// alone is not enough to start a broadcast - see internal/domain/credential
// for the destination stream key, held in the OS credential store, and
// internal/runtime/branch for where the two are combined only at process
// start time.
package output

import "time"

// Settings is the output configuration stored for one platform.
type Settings struct {
	ServerURL   string    `json:"serverUrl"`
	AutoRestart bool      `json:"autoRestart"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// UpdateInput is a full replacement of the mutable fields.
type UpdateInput struct {
	ServerURL   string
	AutoRestart bool
}
