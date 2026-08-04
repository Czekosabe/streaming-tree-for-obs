package mediamtx

import "time"

// ProcessState is the explicit MediaMTX lifecycle state.
//
// It is a single value rather than a set of booleans: "installing and starting"
// or "ready and missing" must be unrepresentable, and the interface renders one
// state at a time.
type ProcessState string

const (
	// StateMissing means no usable binary was found.
	StateMissing ProcessState = "missing"
	// StateInstalling means a managed installation is running.
	StateInstalling ProcessState = "installing"
	// StateIncompatible means a binary exists but reports the wrong version.
	StateIncompatible ProcessState = "incompatible"
	// StateStopped means a usable binary exists and is not running.
	StateStopped ProcessState = "stopped"
	// StateStarting means the process was spawned and readiness is pending.
	StateStarting ProcessState = "starting"
	// StateReady means the Control API answered correctly.
	StateReady ProcessState = "ready"
	// StateStopping means a controlled stop is in progress.
	StateStopping ProcessState = "stopping"
	// StateError means the last start or run failed.
	StateError ProcessState = "error"
)

// IngestState is the state of the configured RTMP path.
type IngestState string

const (
	// IngestUnavailable means MediaMTX is not ready, so nothing can publish.
	IngestUnavailable IngestState = "unavailable"
	// IngestWaiting means MediaMTX is ready and no publisher is connected.
	IngestWaiting IngestState = "waiting"
	// IngestReceiving means a publisher is connected and the path is ready.
	IngestReceiving IngestState = "receiving"
	// IngestError means MediaMTX is ready but its path status cannot be read.
	IngestError IngestState = "error"
)

// MediaMTXSnapshot is the runtime view of the MediaMTX component.
//
// It deliberately excludes the executable path, the process environment, the
// command line and the process id: none is useful to the interface, and each
// would leak machine detail into the browser.
type MediaMTXSnapshot struct {
	SupportedVersion string       `json:"supportedVersion"`
	InstalledVersion string       `json:"installedVersion,omitempty"`
	Source           BinarySource `json:"source"`
	State            ProcessState `json:"state"`
	AutoStart        bool         `json:"autoStart"`
	AutoRestart      bool         `json:"autoRestart"`
	// StartedAt is when the current ready process started, RFC 3339.
	StartedAt string `json:"startedAt,omitempty"`
	// RestartCount counts automatic restarts since the backend started.
	RestartCount int           `json:"restartCount"`
	LastError    *RuntimeError `json:"lastError"`
}

// IngestSnapshot is the runtime view of the configured ingest path.
//
// Every optional field is present only when MediaMTX actually reported it.
// Nothing here is estimated: there is no bitrate, resolution or frame rate,
// because the Control API does not provide them and inventing them would make
// the panel untrustworthy.
type IngestSnapshot struct {
	State IngestState `json:"state"`
	Path  string      `json:"path"`
	// SourceType is the MediaMTX source kind, e.g. "rtmpConn". It identifies
	// the protocol, not the application: RTMP does not prove the publisher is
	// OBS, so the interface says "OBS or another RTMP publisher".
	SourceType string `json:"sourceType,omitempty"`
	// ConnectedAt is when the path became ready, as reported by MediaMTX.
	ConnectedAt string `json:"connectedAt,omitempty"`
	// TrackCount is nil when no publisher is connected.
	TrackCount *int `json:"trackCount"`
	// Tracks are codec identifiers reported by MediaMTX, e.g. ["H264"].
	Tracks []string `json:"tracks"`
}

// ConnectionSnapshot is what a publisher needs in order to connect.
//
// The stream key is the MediaMTX path: a route identifier, not a secret, and
// not a destination-platform stream key.
type ConnectionSnapshot struct {
	ServerURL  string `json:"serverUrl"`
	StreamKey  string `json:"streamKey"`
	PublishURL string `json:"publishUrl"`
}

// Snapshot is one coherent runtime picture.
type Snapshot struct {
	MediaMTX   MediaMTXSnapshot   `json:"mediaMtx"`
	Ingest     IngestSnapshot     `json:"ingest"`
	Connection ConnectionSnapshot `json:"connection"`
}

// formatTime renders a timestamp for the API, or "" when unset.
func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
