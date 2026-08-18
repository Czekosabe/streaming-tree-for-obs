package updater

// State is the update-manager's own explicit lifecycle state
// (docs/updater.md §11) - a single value, not a set of booleans,
// mirroring internal/runtime/branch.State's own reasoning.
type State string

const (
	// StateDisabled means this is not a release build - see
	// docs/updater.md §35. No automatic or manual check is ever
	// performed while in this state.
	StateDisabled       State = "disabled"
	StateIdle           State = "idle"
	StateChecking       State = "checking"
	StateUpToDate       State = "up_to_date"
	StateAvailable      State = "available"
	StateDownloading    State = "downloading"
	StateReadyToInstall State = "ready_to_install"
	StateInstalling     State = "installing"
	// StateError is recoverable - the next successful check returns to
	// StateIdle/StateUpToDate/StateAvailable, never a terminal dead end.
	StateError State = "error"
)

// Blocker codes - stable, machine-readable, surfaced to the frontend
// (docs/updater.md §17/§33).
const (
	BlockerStreamingActive     = "install_blocked_streaming_active"
	BlockerNotInstalledCtx     = "not_installed_context"
	BlockerNoCandidate         = "no_verified_candidate"
	BlockerPlatformUnsupported = "platform_unsupported"
)

// Error codes - stable, machine-readable, never a raw error string
// (docs/updater.md §11/§30).
const (
	ErrorCodeCheckFailed     = "check_failed"
	ErrorCodeRateLimited     = "rate_limited"
	ErrorCodeInvalidManifest = "invalid_manifest"
	ErrorCodeDownloadFailed  = "download_failed"
	ErrorCodeHashMismatch    = "hash_mismatch"
	ErrorCodeSizeExceeded    = "size_exceeded"
	ErrorCodeInstallFailed   = "install_failed"
)

// Status is the safe snapshot the HTTP API exposes (docs/updater.md
// §11/§30) - deliberately excludes any local filesystem path, GitHub
// asset id, download URL, SHA-256 value, or machine identity.
type Status struct {
	Enabled        bool   `json:"enabled"`
	ReleaseBuild   bool   `json:"releaseBuild"`
	CurrentVersion string `json:"currentVersion"`

	AutoCheck bool  `json:"autoCheck"`
	State     State `json:"state"`

	LatestVersion         string `json:"latestVersion,omitempty"`
	UpdateAvailable       bool   `json:"updateAvailable"`
	ReleaseNotes          string `json:"releaseNotes,omitempty"`
	ReleaseNotesTruncated bool   `json:"releaseNotesTruncated,omitempty"`
	PublishedAt           string `json:"publishedAt,omitempty"`

	LastSuccessfulCheckAt string `json:"lastSuccessfulCheckAt,omitempty"`

	DownloadedBytes int64 `json:"downloadedBytes,omitempty"`
	TotalBytes      int64 `json:"totalBytes,omitempty"`

	InstallBlocked bool   `json:"installBlocked"`
	BlockerCode    string `json:"blockerCode,omitempty"`

	LastErrorCode string `json:"lastErrorCode,omitempty"`

	// PostUpdateOutcome/FromVersion/ToVersion surface the one-shot
	// result of the previous update attempt, if any (docs/updater.md
	// §26) - present only once, immediately after a restart triggered
	// by a successful or failed install; consumed and cleared by
	// Manager.Start, never re-read from disk after that.
	PostUpdateOutcome     string `json:"postUpdateOutcome,omitempty"`
	PostUpdateFromVersion string `json:"postUpdateFromVersion,omitempty"`
	PostUpdateToVersion   string `json:"postUpdateToVersion,omitempty"`
}
