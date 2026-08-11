package visualtemplate

import (
	"context"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// AssetRefTracker is the narrow subset of visualasset.Service a
// template that references a Stage 14B managed asset needs - identical
// in shape to internal/alerts.AssetRefTracker/internal/chatoverlay.
// AssetRefTracker. Optional: nil degrades to "no template created or
// imported through this Service may reference a managed asset".
type AssetRefTracker interface {
	Get(ctx context.Context, id string) (visualasset.Asset, error)
	SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error
	ClearTemplateRefs(ctx context.Context, templateID string) error
}

// MaxImportBytes bounds a raw imported/previewed template file (Stage
// 14A task Part 20) - generous relative to visualdesign.MaxDocumentBytes
// (64 KiB) so a well-formed, in-bounds document is never rejected
// purely by the small amount of portable metadata wrapped around it.
const MaxImportBytes = 128 * 1024

// Clock returns the current time - injected so tests are deterministic,
// exactly like internal/domain/visualdesign's own Service.
type Clock func() time.Time

// Service is the validated façade over Repository plus the built-in
// registry - never bypassed by an HTTP handler (Stage 14A task Part
// 26/27).
type Service struct {
	repo     Repository
	builtins []Template
	now      Clock
	newID    func() (string, error)
	// assetSvc is Stage 14B's own managed-asset service - optional, see
	// AssetRefTracker's own doc comment. Set after construction via
	// SetAssetService so NewService's own signature/call sites never
	// need to change.
	assetSvc AssetRefTracker
}

// SetAssetService wires Stage 14B's managed-asset service in after
// construction - call once, before serving any request.
func (s *Service) SetAssetService(svc AssetRefTracker) {
	s.assetSvc = svc
}

func (s *Service) resolveAssetKind(ctx context.Context) visualdesign.AssetResolverFunc {
	if s.assetSvc == nil {
		return nil
	}
	return func(assetID string) (string, bool) {
		asset, err := s.assetSvc.Get(ctx, assetID)
		if err != nil {
			return "", false
		}
		return string(asset.Kind), true
	}
}

// NewService builds a Service. now defaults to time.Now().UTC when nil.
// builtins is validated once here (ValidateBuiltins) so a malformed
// built-in fails loudly at construction, never silently (Stage 14A task
// Part 25).
func NewService(repo Repository, builtins []Template, now Clock) (*Service, error) {
	if err := ValidateBuiltins(builtins); err != nil {
		return nil, err
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, builtins: builtins, now: now, newID: NewTemplateID}, nil
}

// Builtins returns the application-owned, immutable template set this
// Service was constructed with (a defensive copy).
func (s *Service) Builtins() []Template {
	out := make([]Template, len(s.builtins))
	copy(out, s.builtins)
	return out
}

func (s *Service) findBuiltin(id string) (Template, bool) {
	for _, b := range s.builtins {
		if b.ID == id {
			return b, true
		}
	}
	return Template{}, false
}

// List returns every built-in followed by every user template.
func (s *Service) List(ctx context.Context) ([]Template, error) {
	users, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(s.builtins)+len(users))
	out = append(out, s.builtins...)
	out = append(out, users...)
	return out, nil
}

// Get returns id, checking built-ins first (a built-in id can never
// collide with a "tpl_"-prefixed user id, but built-ins are checked
// first regardless, since they are the cheaper, in-memory lookup).
func (s *Service) Get(ctx context.Context, id string) (Template, error) {
	if b, ok := s.findBuiltin(id); ok {
		return b, nil
	}
	return s.repo.Get(ctx, id)
}

// Create persists a brand-new user template from operator-provided
// fields (Stage 14A task Part 17/28 - "Save as template"/"create
// directly"). The server always generates ID/CreatedAt/UpdatedAt; the
// caller-provided target/name/description/author/license/document are
// fully validated (including migrating the embedded document to the
// current visual-design version) before anything is persisted.
func (s *Service) Create(ctx context.Context, target Target, name, description, author, license string, doc visualdesign.Document) (Template, error) {
	normalized, err := NormalizeAndValidateDocument(doc)
	if err != nil {
		return Template{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Template{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	now := s.now()
	t := Template{
		ID: id, Target: target, Source: SourceUser,
		Name: name, Description: description, Author: author, License: license,
		TemplateSchemaVersion: CurrentTemplateSchemaVersion,
		Document:              normalized,
		CreatedAt:             now, UpdatedAt: now,
	}
	if err := Validate(t); err != nil {
		return Template{}, err
	}
	if err := visualdesign.ValidateAssetReferences(t.Document, s.resolveAssetKind(ctx)); err != nil {
		return Template{}, err
	}
	created, err := s.repo.Create(ctx, t)
	if err != nil {
		return Template{}, err
	}
	if s.assetSvc != nil {
		if err := s.assetSvc.SetTemplateAssetRefs(ctx, created.ID, created.Document.AssetReferences()); err != nil {
			return Template{}, err
		}
	}
	return created, nil
}

// UpdateMetadata edits name/description/author/license for a USER
// template only - a built-in returns ErrImmutable (Stage 14A task Part
// 18/29).
func (s *Service) UpdateMetadata(ctx context.Context, id, name, description, author, license string) (Template, error) {
	if _, ok := s.findBuiltin(id); ok {
		return Template{}, ErrImmutable
	}
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Template{}, err
	}
	candidate := existing
	candidate.Name, candidate.Description, candidate.Author, candidate.License = name, description, author, license
	if err := Validate(candidate); err != nil {
		return Template{}, err
	}
	return s.repo.UpdateMetadata(ctx, id, name, description, author, license)
}

// Delete removes a USER template only - a built-in returns
// ErrImmutable. Idempotent for a genuinely-missing user template id
// (mirrors visualdesign.Service.Delete).
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, ok := s.findBuiltin(id); ok {
		return ErrImmutable
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.assetSvc != nil {
		// docs/visual-template-packages.md §46: deleting a template
		// never cascade-deletes the assets it references - only its own
		// reference rows are cleared.
		if err := s.assetSvc.ClearTemplateRefs(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ImportPreview validates raw (a Stage 14A portable template file,
// already parsed into a Template by the caller's own wire-DTO layer)
// without persisting anything (Stage 14A task Part 19) - the backend
// stays authoritative even for a preview, but a preview can never
// itself be the proof a later Import call reuses; Import always
// re-validates the actual bytes it receives.
func (s *Service) ImportPreview(candidate Template) (Template, error) {
	normalized, err := NormalizeAndValidateDocument(candidate.Document)
	if err != nil {
		return Template{}, err
	}
	candidate.Document = normalized
	candidate.Source = SourceUser
	// candidate.TemplateSchemaVersion is left exactly as the caller
	// supplied it (the raw imported file's own top-level schemaVersion)
	// so Validate below can actually reject an unsupported one - it
	// must never be silently forced to CurrentTemplateSchemaVersion
	// before that check runs.
	if err := Validate(candidate); err != nil {
		return Template{}, err
	}
	return candidate, nil
}

// Import re-validates candidate (see ImportPreview's own doc comment -
// never trusts a prior preview call) and persists it as a brand-new
// user template with a freshly generated local id - an imported file's
// own metadata can never choose the local database id (Stage 14A task
// Part 10).
func (s *Service) Import(ctx context.Context, candidate Template) (Template, error) {
	normalized, err := s.ImportPreview(candidate)
	if err != nil {
		return Template{}, err
	}
	return s.Create(ctx, normalized.Target, normalized.Name, normalized.Description, normalized.Author, normalized.License, normalized.Document)
}

// Export returns id's own current, normalized representation, suitable
// for direct portable-file serialization by the caller (Stage 14A task
// Part 22/23) - built-ins may be exported too.
func (s *Service) Export(ctx context.Context, id string) (Template, error) {
	return s.Get(ctx, id)
}
