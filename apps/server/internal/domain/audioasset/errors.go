package audioasset

import "errors"

var (
	// ErrStorage wraps any unexpected persistence/filesystem failure.
	ErrStorage = errors.New("audio asset storage failure")
	// ErrNotFound means no asset exists for the given id.
	ErrNotFound = errors.New("audio asset not found")
	// ErrInvalid means the supplied metadata (display name length) failed
	// a structural bound - never used for a content/signature failure,
	// see ErrUnsupported.
	ErrInvalid = errors.New("audio asset is invalid")
	// ErrUnsupported means the asset's own bytes did not match the one
	// accepted WAV/PCM signature, or its extension/declared media
	// type/detected signature disagreed, or its structure was malformed
	// (docs/alert-audio.md §5.3).
	ErrUnsupported = errors.New("audio asset type is not supported")
	// ErrTooLarge means the asset exceeded MaxSoundBytes or
	// MaxSoundDurationMS (docs/alert-audio.md §5.3).
	ErrTooLarge = errors.New("audio asset is too large")
	// ErrInUse means a delete was rejected because at least one alert
	// rule or template audio preset still references the asset
	// (docs/alert-audio.md §5.6) - stable API error audio_asset_in_use.
	ErrInUse = errors.New("audio asset is in use")
)
