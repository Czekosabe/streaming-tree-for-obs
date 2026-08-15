package visualtemplate

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("visual template storage failure")
	// ErrNotFound means no template exists for the given id (built-in
	// or user).
	ErrNotFound = errors.New("visual template not found")
	// ErrValidation wraps any semantic template validation failure -
	// see validation.go for the exact rules.
	ErrValidation = errors.New("visual template validation failed")
	// ErrImmutable means a caller tried to rename, delete, or replace a
	// built-in template - built-ins are application-owned and never
	// mutate (Stage 14A task Part 18/29).
	ErrImmutable = errors.New("visual template is immutable")
	// ErrTargetMismatch means a template's own Target does not match
	// the target it was asked to be used/compatible with.
	ErrTargetMismatch = errors.New("visual template target mismatch")
	// ErrUnsupportedTemplateVersion means a template file's own
	// top-level schemaVersion is not CurrentTemplateSchemaVersion.
	ErrUnsupportedTemplateVersion = errors.New("visual template schema version is not supported")
	// ErrUnsupportedDesignVersion means the embedded visual-design
	// document's own version could not be migrated to
	// visualdesign.CurrentVersion (unknown/future/malformed version).
	ErrUnsupportedDesignVersion = errors.New("embedded visual design version is not supported")
	// ErrTooLarge means a raw imported template file exceeded
	// MaxImportBytes.
	ErrTooLarge = errors.New("visual template import is too large")
	// ErrRequiresPackageExport means a caller asked for the asset-free
	// Stage 14A JSON export of a template whose document references at
	// least one managed asset (docs/visual-template-packages.md §21,
	// stable error visual_template_requires_package_export) - it must
	// be exported as a package instead.
	ErrRequiresPackageExport = errors.New("this template references managed assets and must be exported as a package")
	// ErrAssetsMissing means a standalone Stage 14A JSON template file
	// (import or create) embeds a document that references a managed
	// asset - a JSON file has no channel to carry the asset bytes a
	// reference depends on (docs/visual-template-packages.md §21,
	// stable error visual_template_assets_missing).
	ErrAssetsMissing = errors.New("this template document references managed assets, which a standalone JSON file cannot carry")
	// ErrAudioAssetNotFound means a package v2 import's own alertAudio
	// preset names a sound asset id that does not resolve to a real
	// managed audio asset (docs/alert-audio.md §10.2) - checked only
	// when SoundEnabled, via the injected AudioAssetRefTracker.
	ErrAudioAssetNotFound = errors.New("visual template alert-audio preset references an audio asset that was not found")
	// ErrAudioNotAllowedForTarget means a TargetChat template carries a
	// non-nil AlertAudio - alert-owned audio is only ever legal for a
	// TargetAlert template (docs/alert-audio.md §10.2's own explicit
	// "chat-target package containing this object is rejected").
	ErrAudioNotAllowedForTarget = errors.New("alert-audio presets are only valid for an alert-target template")
)
