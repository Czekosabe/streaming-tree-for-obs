package visualdesign

// MigrateToCurrentVersion upgrades doc to CurrentVersion if it was
// stored at an older version, applying every intermediate migration in
// order (Stage 13B, docs/visual-designs.md §19/§11). Called once, on
// read, by the SQLite repository - before doc ever reaches Validate or
// a renderer.
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
func MigrateToCurrentVersion(doc Document) Document {
	if doc.Version == Version1 {
		doc.Version = Version2
	}
	return doc
}
