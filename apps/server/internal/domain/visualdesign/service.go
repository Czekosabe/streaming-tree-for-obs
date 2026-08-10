package visualdesign

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Clock returns the current time - injected everywhere in this project
// so tests are deterministic (see internal/domain/alerts's own Clock).
type Clock func() time.Time

// Service is the validated façade over Repository - never bypassed by
// an HTTP handler, exactly like every other domain package's own
// Service (internal/domain/alerts.Service, internal/domain/
// chatoverlay.Service).
type Service struct {
	repo  Repository
	now   Clock
	newID func() (string, error)
}

// NewService builds a Service. now/newID default to time.Now().UTC and
// NewDesignID when nil, exactly like the sibling domain packages' own
// constructors.
func NewService(repo Repository, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, now: now, newID: NewDesignID}
}

func (s *Service) validateOwner(ownerKind OwnerKind, ownerID string) error {
	if !ownerKind.valid() {
		return fmt.Errorf("%w: owner kind %q is not accepted", ErrValidation, string(ownerKind))
	}
	if ownerID == "" {
		return fmt.Errorf("%w: owner id must not be empty", ErrValidation)
	}
	return nil
}

// Get returns the design currently saved for (ownerKind, ownerID), or
// found=false if none exists yet.
func (s *Service) Get(ctx context.Context, ownerKind OwnerKind, ownerID string) (Record, bool, error) {
	if err := s.validateOwner(ownerKind, ownerID); err != nil {
		return Record{}, false, err
	}
	return s.repo.Get(ctx, ownerKind, ownerID)
}

// normalizeLayerOrder rewrites doc's own layers' Order fields to a
// dense 0..N-1 sequence, stable-sorted by their current Order - so a
// caller (or a persisted document written by an earlier, slightly
// different client) never needs to reason about gaps, and Save always
// persists a canonical, gap-free order (Stage 13A task Part 32: "no
// reliance on DOM insertion accidents", extended here to "no reliance
// on whatever raw Order integers a client happened to send").
func normalizeLayerOrder(doc Document) Document {
	layers := make([]Layer, len(doc.Layers))
	copy(layers, doc.Layers)
	sort.SliceStable(layers, func(i, j int) bool { return layers[i].Order < layers[j].Order })
	for i := range layers {
		layers[i].Order = i
	}
	doc.Layers = layers
	return doc
}

// Save validates doc (after normalizing layer order) and persists it as
// a full replacement for (ownerKind, ownerID), enforcing
// expectedRevision (Stage 13A task Part 41: "PUT must require the
// expected revision... 409 on conflict, never silently overwrite").
// Callers needing alert-specific binding-availability validation (a
// text layer's binding must make sense for the owning rule's own event
// type) must run that check themselves before calling Save - this
// method only performs the generic, owner-agnostic checks in
// validation.go.
func (s *Service) Save(ctx context.Context, ownerKind OwnerKind, ownerID string, doc Document, expectedRevision int) (Record, error) {
	if err := s.validateOwner(ownerKind, ownerID); err != nil {
		return Record{}, err
	}
	doc = normalizeLayerOrder(doc)
	if err := Validate(doc); err != nil {
		return Record{}, err
	}
	return s.repo.Save(ctx, ownerKind, ownerID, doc, expectedRevision, s.newID)
}

// Delete removes the design saved for (ownerKind, ownerID), if any -
// idempotent (Stage 13A task Part 42).
func (s *Service) Delete(ctx context.Context, ownerKind OwnerKind, ownerID string) error {
	if err := s.validateOwner(ownerKind, ownerID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, ownerKind, ownerID)
}
