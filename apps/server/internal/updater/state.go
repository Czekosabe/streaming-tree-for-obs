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
	// StatePlatformUnsupported means this is a release build, but the
	// platform Handoff reports install as structurally impossible here
	// (docs/macos-packaging.md §20) - distinct from StateDisabled, which
	// specifically means "not a release build". Set once at
	// construction and permanent for the life of the process: automatic
	// polling never begins in this state, and CheckNow/Download both
	// refuse immediately, regardless of the persisted AutoCheck
	// preference or of whether the release manifest happens to list an
	// artifact for this platform's identity.
	StatePlatformUnsupported State = "platform_unsupported"
	// StateManualBuild means this is a release build (produced by the
	// release build script), but its injected version is not a strict
	// major.minor.patch production version (buildinfo.IsStrictProductionVersion()
	// == false) - e.g. a manual/test build such as
	// "0.1.0-manualtest+<commit>". Distinct from StateDisabled (which
	// means "not a release build at all") and from StateError (this is
	// not a failure - the build is simply not the kind of build the
	// production updater ever applies to, since the release pipeline
	// itself refuses to generate real release-manifest metadata for a
	// version shaped like this). Set once at construction and permanent
	// for the life of the process, the same way StatePlatformUnsupported
	// is: automatic polling never begins in this state, and
	// CheckNow/Download/Install all refuse immediately, regardless of
	// the persisted AutoCheck preference.
	StateManualBuild State = "manual_build"
	// StateNoReleaseYet means the most recent check reached GitHub
	// successfully and GitHub reported (via a 404 from /releases/latest)
	// that this repository has no published Stable release yet. This is
	// a normal, well-understood outcome, not a failure: lastSuccessfulCheckAt
	// still advances and no error code is set, exactly like
	// StateUpToDate. Distinct from StateError, which is reserved for an
	// actual network/API failure or a genuinely malformed response -
	// the two must never be presented identically (see
	// ErrNoStableRelease).
	StateNoReleaseYet State = "no_release_published"
)

// Blocker codes - stable, machine-readable, surfaced to the frontend
// (docs/updater.md §17/§33).
const (
	BlockerStreamingActive     = "install_blocked_streaming_active"
	BlockerNotInstalledCtx     = "not_installed_context"
	BlockerNoCandidate         = "no_verified_candidate"
	BlockerPlatformUnsupported = "platform_unsupported"
	// BlockerManualBuild mirrors StateManualBuild's own string value,
	// the same convention BlockerPlatformUnsupported/StatePlatformUnsupported
	// already follow.
	BlockerManualBuild = "manual_build"
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
