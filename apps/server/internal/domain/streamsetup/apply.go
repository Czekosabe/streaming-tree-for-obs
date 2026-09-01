package streamsetup

import (
	"context"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
)

// ErrActiveStreamBlocksApply means one or more destinations this apply
// would affect currently have an active branch - docs/stream-setup-
// profiles.md §6. Never force-applied; the operator must stop
// streaming on the affected destination(s) first.
var ErrActiveStreamBlocksApply = errors.New("streaming is active on one or more affected destinations")

// DestinationChange classifies what applying a profile would do to one
// destination's own Enabled state.
type DestinationChange string

const (
	ChangeWillEnable  DestinationChange = "will_enable"
	ChangeWillDisable DestinationChange = "will_disable"
	ChangeUnchanged   DestinationChange = "unchanged"
	// ChangeMissing means this profile once referenced a destination
	// that has since been deleted - never recreated automatically,
	// never rebound to a different destination merely because the
	// provider matches (docs/stream-setup-profiles.md §4).
	ChangeMissing DestinationChange = "missing"
)

// DestinationPreviewItem is one destination's own preview row.
type DestinationPreviewItem struct {
	PlatformID       string
	ProviderID       string
	DisplayName      string
	CurrentlyEnabled bool
	Change           DestinationChange
	// Active is true when this destination currently has a branch in
	// one of the states that blocks apply (§6) - shown even for a
	// destination whose own Change is Unchanged, since it may still be
	// part of the metadata-preset target set.
	Active bool
}

// Preview is the full, bounded compatibility preview for applying one
// profile - nothing is written by computing it.
type Preview struct {
	Profile                     Profile
	Destinations                []DestinationPreviewItem
	MetadataPresetReferenced    bool
	MetadataPresetMissing       bool
	MetadataPresetName          string
	MetadataDestinationPreviews []metadatapreset.DestinationPreview
	Blocked                     bool
	BlockedDestinationIDs       []string
}

// ApplyResult summarizes what a completed Apply actually did.
type ApplyResult struct {
	DestinationsChanged int
	MetadataApplied     bool
	// MetadataSkippedReason is "" when MetadataApplied is true or no
	// preset was referenced at all; otherwise "preset_missing" or the
	// underlying apply error's own message - the metadata step's own
	// outcome is always reported honestly, never silently merged into
	// overall success (docs/stream-setup-profiles.md §3).
	MetadataSkippedReason string
}

func (s *Service) Preview(ctx context.Context, profileID string) (Preview, error) {
	p, err := s.repo.Get(ctx, profileID)
	if err != nil {
		return Preview{}, err
	}

	allPlatforms, err := s.platforms.List(ctx)
	if err != nil {
		return Preview{}, fmt.Errorf("list platforms: %w", err)
	}
	enabledNow := make(map[string]bool, len(allPlatforms))
	for _, pf := range allPlatforms {
		enabledNow[pf.ID] = pf.Enabled
	}

	activeNow, err := s.activeBranchPlatformIDs(ctx)
	if err != nil {
		return Preview{}, err
	}

	profileIDs := make(map[string]bool, len(p.Destinations))
	items := make([]DestinationPreviewItem, 0, len(p.Destinations))
	for _, d := range p.Destinations {
		if d.PlatformID == nil {
			items = append(items, DestinationPreviewItem{
				ProviderID: d.ProviderID, DisplayName: d.DisplayName, Change: ChangeMissing,
			})
			continue
		}
		id := *d.PlatformID
		profileIDs[id] = true
		was := enabledNow[id]
		change := ChangeUnchanged
		if !was {
			change = ChangeWillEnable
		}
		items = append(items, DestinationPreviewItem{
			PlatformID: id, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
			CurrentlyEnabled: was, Change: change, Active: activeNow[id],
		})
	}
	// Every currently-enabled destination NOT in the profile will be
	// disabled.
	for _, pf := range allPlatforms {
		if !pf.Enabled || profileIDs[pf.ID] {
			continue
		}
		items = append(items, DestinationPreviewItem{
			PlatformID: pf.ID, ProviderID: string(pf.ProviderID), DisplayName: pf.DisplayName,
			CurrentlyEnabled: true, Change: ChangeWillDisable, Active: activeNow[pf.ID],
		})
	}

	preview := Preview{Profile: p, Destinations: items}
	if p.MetadataPresetID != nil {
		preview.MetadataPresetReferenced = true
		preview.MetadataPresetName = p.MetadataPresetName
		destPreviews, err := s.presets.ApplyPreview(ctx, *p.MetadataPresetID, p.PlatformIDs())
		if err != nil {
			return Preview{}, fmt.Errorf("preview metadata preset apply: %w", err)
		}
		preview.MetadataDestinationPreviews = destPreviews
	} else if p.MetadataPresetMissing() {
		preview.MetadataPresetReferenced = true
		preview.MetadataPresetMissing = true
		preview.MetadataPresetName = p.MetadataPresetName
	}

	var blocked []string
	for _, item := range items {
		if item.Active && item.Change != ChangeMissing {
			blocked = append(blocked, item.PlatformID)
		}
	}
	preview.Blocked = len(blocked) > 0
	preview.BlockedDestinationIDs = blocked
	return preview, nil
}

func (s *Service) activeBranchPlatformIDs(ctx context.Context) (map[string]bool, error) {
	snapshots, err := s.branches.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot branches: %w", err)
	}
	active := make(map[string]bool, len(snapshots))
	for _, snap := range snapshots {
		if activeBranchStates[snap.State] {
			active[snap.PlatformID] = true
		}
	}
	return active, nil
}

// Apply commits a profile: destination membership is applied in one
// atomic batch (docs/stream-setup-profiles.md §5), then the referenced
// metadata preset (if any, and if it still exists) is applied through
// Stage 22's own unchanged Apply. These are two separately-atomic
// steps, not one cross-domain transaction - if the destination batch
// fails, nothing changes; if it succeeds but the metadata step fails,
// the destination membership is left correctly applied and the
// metadata outcome is reported honestly in the result, never silently
// folded into overall failure or overall success.
func (s *Service) Apply(ctx context.Context, profileID string) (ApplyResult, error) {
	preview, err := s.Preview(ctx, profileID)
	if err != nil {
		return ApplyResult{}, err
	}
	if preview.Blocked {
		return ApplyResult{}, fmt.Errorf("%w: %v", ErrActiveStreamBlocksApply, preview.BlockedDestinationIDs)
	}

	updates := make(map[string]bool)
	for _, item := range preview.Destinations {
		switch item.Change {
		case ChangeWillEnable:
			updates[item.PlatformID] = true
		case ChangeWillDisable:
			updates[item.PlatformID] = false
		}
	}
	if len(updates) > 0 {
		if err := s.platforms.SetEnabledBatch(ctx, updates); err != nil {
			return ApplyResult{}, fmt.Errorf("apply destination membership: %w", err)
		}
	}

	result := ApplyResult{DestinationsChanged: len(updates)}
	p := preview.Profile
	switch {
	case p.MetadataPresetID == nil && !p.MetadataPresetMissing():
		// No preset referenced at all - nothing to report.
	case p.MetadataPresetMissing():
		result.MetadataSkippedReason = "preset_missing"
	default:
		if _, err := s.presets.Apply(ctx, *p.MetadataPresetID, p.PlatformIDs()); err != nil {
			result.MetadataSkippedReason = err.Error()
		} else {
			result.MetadataApplied = true
		}
	}
	return result, nil
}
