package visualdesign

// MigrateToCurrentVersion upgrades doc to CurrentVersion if it was
// stored at an older version, applying every intermediate migration in
// order (Stage 13B, docs/visual-designs.md §19/§11; Stage 14B,
// docs/visual-template-packages.md §12). Called once, on read, by the
// SQLite repository - before doc ever reaches Validate or a renderer.
//
// The Version1 -> Version2 step is intentionally trivial and lossless:
// a Version1 document's own wire shape is already byte-for-byte a valid
// Version2 one (every Version1 field still exists, unchanged, in
// Version2 - only the set of values LayerKind/TextBinding accept grew
// wider). A Version1 document, by construction, could never have used a
// Version2-only value (they did not exist yet), so this migration is
// exactly "relabel Version = 2" - no field is renamed, reinterpreted,
// or dropped, and every existing Stage 13A alert design therefore loads
// and renders identically after migration.
//
// The Version2 -> Version3 step is the same shape of trivial, lossless
// relabel: Version3 only adds two new closed layer kinds
// (image/video, entirely new Layer.Image/Layer.Video fields that stay
// nil on every existing layer) and one new optional field on
// TextProps/MessageFragmentsProps (FontAssetID, empty string by
// default/zero value). No Version2 document could ever have populated
// any of these - they did not exist yet - so this step never invents an
// asset reference, and every existing Stage 13A/13B design keeps
// rendering identically after migration, chained through both steps.
func MigrateToCurrentVersion(doc Document) Document {
	if doc.Version == Version1 {
		doc.Version = Version2
	}
	if doc.Version == Version2 {
		doc.Version = Version3
	}
	return doc
}
