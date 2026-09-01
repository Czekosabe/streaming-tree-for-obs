package metadatapreset

import (
	"context"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// PlatformMetadataStore is the narrow port this domain needs from
// internal/domain/platform, mirroring this codebase's own established
// per-consumer-interface convention (e.g. httpapi.PlatformService).
// Satisfied by *platform.Service - this domain never talks to SQLite
// directly for destination data, and never duplicates platform
// business logic (docs/metadata-presets.md §5).
type PlatformMetadataStore interface {
	// GetMany returns every named platform, or platform.ErrNotFound if
	// any id does not exist.
	GetMany(ctx context.Context, ids []string) (map[string]platform.Platform, error)
	// SaveMetadataBatch validates and replaces the metadata of every
	// named platform atomically - either all succeed or none are
	// persisted. Provider publishing is never part of this
	// transaction: Apply only ever writes local metadata.
	SaveMetadataBatch(ctx context.Context, updates map[string]platform.Metadata) (map[string]platform.Metadata, error)
}

// FieldStatus classifies one metadata field's outcome for one
// destination in an apply preview.
type FieldStatus string

const (
	// FieldWillChange means applying the preset would change this
	// field's stored value on the destination.
	FieldWillChange FieldStatus = "will_change"
	// FieldUnchanged means the destination already holds this value.
	FieldUnchanged FieldStatus = "unchanged"
	// FieldNotSupported means the destination's provider has no
	// capability for this field at all - the preset's own value for it,
	// if any, is never sent.
	FieldNotSupported FieldStatus = "not_supported"
)

// FieldPreview is one field's classification for one destination.
type FieldPreview struct {
	Field  string
	Status FieldStatus
}

// DestinationPreview is one destination's compatibility preview for
// applying a preset (docs/metadata-presets.md §6).
type DestinationPreview struct {
	PlatformID string
	ProviderID platform.ProviderID
	// Valid is false when the preset's projected values fail this
	// destination's own validation (e.g. a title too long for that
	// specific provider's limit, even though it fit the preset's own
	// generic bound) - Apply refuses to write anything for any
	// destination when even one is invalid.
	Valid  bool
	Fields []FieldPreview
	Errors []platform.FieldViolation
}

// previewFieldNames lists every field classify walks, in the same
// order MetadataForm.tsx's own CAPABILITY_FIELDS does, so the preview
// list always has a stable, predictable order.
var previewFieldNames = []string{
	"title", "description", "category", "tags",
	"language", "visibility", "matureContent", "dvr", "latencyMode",
}

// buildCandidate projects a preset's stored values onto one provider's
// real capability table: a field is copied only when the provider
// supports it, otherwise it is left at zero. This is the exact
// projection platform.ValidateMetadata itself requires - it is a hard
// error to hand it a non-empty value for an unsupported field.
func buildCandidate(preset Preset, def platform.ProviderDefinition) platform.Metadata {
	caps := def.Capabilities
	c := platform.Metadata{Tags: []string{}}

	if caps.Title {
		c.Title = preset.Common.Title
	}
	if caps.Description {
		c.Description = preset.Common.Description
	}
	if caps.Tags && len(preset.Common.Tags) > 0 {
		c.Tags = append([]string{}, preset.Common.Tags...)
	}
	if caps.Language {
		c.Language = preset.Common.Language
	}
	if caps.Visibility {
		c.Visibility = preset.Common.Visibility
	}
	if caps.MatureContent {
		c.MatureContent = preset.Common.MatureContent
	}
	if caps.DVR {
		c.DVR = preset.Common.DVR
	}
	if caps.LatencyMode {
		c.LatencyMode = preset.Common.LatencyMode
	}
	if caps.Category {
		if pm, ok := preset.Providers[def.ID]; ok {
			c.Category = pm.Category
			c.CategoryID = pm.CategoryID
		}
	}

	return c
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if b[i] != v {
			return false
		}
	}
	return true
}

// classify compares a validated candidate against a destination's
// current stored metadata, field by field.
func classify(caps platform.Capabilities, current, candidate platform.Metadata) []FieldPreview {
	changed := map[string]bool{
		"title":         current.Title != candidate.Title,
		"description":   current.Description != candidate.Description,
		"category":      current.Category != candidate.Category || current.CategoryID != candidate.CategoryID,
		"tags":          !stringSliceEqual(current.Tags, candidate.Tags),
		"language":      current.Language != candidate.Language,
		"visibility":    current.Visibility != candidate.Visibility,
		"matureContent": current.MatureContent != candidate.MatureContent,
		"dvr":           current.DVR != candidate.DVR,
		"latencyMode":   current.LatencyMode != candidate.LatencyMode,
	}
	supported := map[string]bool{
		"title": caps.Title, "description": caps.Description, "category": caps.Category,
		"tags": caps.Tags, "language": caps.Language, "visibility": caps.Visibility,
		"matureContent": caps.MatureContent, "dvr": caps.DVR, "latencyMode": caps.LatencyMode,
	}

	fields := make([]FieldPreview, 0, len(previewFieldNames))
	for _, name := range previewFieldNames {
		status := FieldNotSupported
		if supported[name] {
			if changed[name] {
				status = FieldWillChange
			} else {
				status = FieldUnchanged
			}
		}
		fields = append(fields, FieldPreview{Field: name, Status: status})
	}
	return fields
}

// resolveApply loads the preset and its target destinations, then
// builds and validates (but does not persist) a candidate per
// destination - the shared computation apply-preview and apply both
// need, so they can never disagree about what would happen.
func (s *Service) resolveApply(
	ctx context.Context, presetID string, platformIDs []string,
) (Preset, map[string]platform.Platform, map[string]platform.Metadata, map[string][]platform.FieldViolation, error) {
	if presetID == "" {
		return Preset{}, nil, nil, nil, ErrNotFound
	}
	if len(platformIDs) == 0 {
		verr := &platform.ValidationError{}
		verr.Add("platformIds", platform.RuleRequired, "Select at least one destination.", nil)
		return Preset{}, nil, nil, nil, verr
	}

	preset, err := s.repo.Get(ctx, presetID)
	if err != nil {
		return Preset{}, nil, nil, nil, err
	}

	platforms, err := s.store.GetMany(ctx, platformIDs)
	if err != nil {
		return Preset{}, nil, nil, nil, err
	}

	candidates := make(map[string]platform.Metadata, len(platformIDs))
	violations := make(map[string][]platform.FieldViolation)

	for _, id := range platformIDs {
		p := platforms[id]
		def, ok := platform.Definition(p.ProviderID)
		if !ok {
			return Preset{}, nil, nil, nil,
				fmt.Errorf("%w: platform %s references unknown provider %q", platform.ErrStorage, id, p.ProviderID)
		}

		candidate := buildCandidate(preset, def)
		validated, err := platform.ValidateMetadata(def, candidate)
		if verr, isValidation := platform.AsValidationError(err); isValidation {
			violations[id] = verr.Violations
		} else if err != nil {
			return Preset{}, nil, nil, nil, err
		}
		validated.UpdatedAt = s.now().UTC()
		candidates[id] = validated
	}

	return preset, platforms, candidates, violations, nil
}

// ApplyPreview computes, without writing anything, what applying a
// preset to each named destination would change (docs/metadata-
// presets.md §6). Matches the existing publish-preview/publish
// "preview first" pattern.
func (s *Service) ApplyPreview(ctx context.Context, presetID string, platformIDs []string) ([]DestinationPreview, error) {
	_, platforms, candidates, violations, err := s.resolveApply(ctx, presetID, platformIDs)
	if err != nil {
		return nil, err
	}

	previews := make([]DestinationPreview, 0, len(platformIDs))
	for _, id := range platformIDs {
		p := platforms[id]
		def, _ := platform.Definition(p.ProviderID)

		preview := DestinationPreview{PlatformID: id, ProviderID: p.ProviderID}
		if errs, invalid := violations[id]; invalid {
			preview.Valid = false
			preview.Errors = errs
		} else {
			preview.Valid = true
		}
		preview.Fields = classify(def.Capabilities, p.Metadata, candidates[id])
		previews = append(previews, preview)
	}

	return previews, nil
}

// Apply re-validates independently (never trusts a frontend preview as
// authority) and, only if every selected destination's candidate
// validates successfully, writes all of them in one atomic
// transaction. If any destination fails validation, nothing is written
// for any of them (docs/metadata-presets.md §6/§15/§23).
//
// After Apply, the preset and each destination's metadata are
// independent persisted objects: later editing or deleting the preset
// never touches already-applied metadata (§16).
func (s *Service) Apply(ctx context.Context, presetID string, platformIDs []string) (map[string]platform.Metadata, error) {
	_, _, candidates, violations, err := s.resolveApply(ctx, presetID, platformIDs)
	if err != nil {
		return nil, err
	}

	if len(violations) > 0 {
		return nil, &ApplyValidationError{Destinations: violations}
	}

	return s.store.SaveMetadataBatch(ctx, candidates)
}
