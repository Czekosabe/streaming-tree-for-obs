// Package streamsetup is Stage 25's stream setup profile domain
// (docs/stream-setup-profiles.md): a reusable LOCAL preparation of the
// app for a particular kind of show - which destinations are intended,
// and optionally which Stage 22 metadata preset to apply. It never
// holds a stream key, an OAuth token, a client secret, or any other
// credential - there is no field shaped to carry one. Applying a
// profile never starts a stream, never publishes provider metadata,
// and never touches a credential.
package streamsetup

import "time"

// Destination is one destination this profile intends to have enabled.
// PlatformID is nil once the referenced platform has been deleted -
// ProviderID/DisplayName are a snapshot taken when this row was
// written, so the profile can still say what it used to point at.
type Destination struct {
	PlatformID  *string
	ProviderID  string
	DisplayName string
}

// Profile is the persisted, named, reusable setup.
type Profile struct {
	ID   string
	Name string
	Note string
	// Destinations is the COMPLETE intended-enabled set for this show,
	// in a deterministic order - applying the profile enables exactly
	// these destinations and disables every other configured one
	// (docs/stream-setup-profiles.md §2/§3).
	Destinations []Destination
	// MetadataPresetID is nil if no preset is referenced, or if the
	// referenced preset has since been deleted (docs/stream-setup-
	// profiles.md §4 - the reference is reported as missing, never
	// silently dropped or substituted).
	MetadataPresetID *string
	// MetadataPresetName is a snapshot of the referenced preset's own
	// name, taken when the reference was set - "" only if a preset was
	// never referenced at all. Once non-empty it survives even after
	// MetadataPresetID goes nil (the preset was deleted), so "never
	// referenced" and "referenced one that is now missing" stay
	// distinguishable.
	MetadataPresetName string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// MetadataPresetMissing reports whether this profile once referenced a
// metadata preset that has since been deleted.
func (p Profile) MetadataPresetMissing() bool {
	return p.MetadataPresetID == nil && p.MetadataPresetName != ""
}

// PlatformIDs returns every non-nil destination platform id, in order.
func (p Profile) PlatformIDs() []string {
	ids := make([]string, 0, len(p.Destinations))
	for _, d := range p.Destinations {
		if d.PlatformID != nil {
			ids = append(ids, *d.PlatformID)
		}
	}
	return ids
}

// CreateInput is the accepted payload for creating a new profile.
// Destinations here are always real, currently-existing platform ids -
// the snapshot provider/display name is resolved and stored by the
// Service at write time, never trusted from the caller.
type CreateInput struct {
	Name             string
	Note             string
	DestinationIDs   []string
	MetadataPresetID *string
}

// UpdateInput is a full replacement of a profile's mutable fields.
type UpdateInput struct {
	Name             string
	Note             string
	DestinationIDs   []string
	MetadataPresetID *string
}
