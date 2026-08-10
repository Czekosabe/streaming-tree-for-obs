package visualdesign

import (
	"context"
	"time"
)

// Record is one persisted visual design row, including the management-
// only metadata (id, owner, revision, timestamps) PublicDocument
// deliberately excludes.
type Record struct {
	ID        string
	OwnerKind OwnerKind
	OwnerID   string
	Document  Document
	// Revision starts at 1 on first save and increments by exactly 1 on
	// every successful replacement (Stage 13A task Part 7) - used for
	// optimistic concurrency by Save's own expectedRevision parameter.
	Revision  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository persists visual designs, one per (ownerKind, ownerID) pair
// (Stage 13A task Part 18: one design belongs to exactly one owner).
// Implementations must parse every stored document into typed Go
// structs before returning it (Stage 13A task Part 6: "malformed JSON
// cannot reach a renderer") and must never return a document that
// fails Validate.
type Repository interface {
	// Get returns the design currently saved for (ownerKind, ownerID),
	// or found=false if none has ever been saved (a normal state, never
	// an error).
	Get(ctx context.Context, ownerKind OwnerKind, ownerID string) (Record, bool, error)

	// Save performs an atomic, optimistic-concurrency full replacement:
	// if no record exists yet, expectedRevision must be 0 and a new
	// record is created at revision 1; if one exists, expectedRevision
	// must equal its current revision or ErrRevisionConflict is
	// returned and nothing is written (Stage 13A task Part 7/41). doc
	// is assumed already Validate-d by the caller. newID mints a fresh
	// Record.ID, used only when creating.
	Save(ctx context.Context, ownerKind OwnerKind, ownerID string, doc Document, expectedRevision int, newID func() (string, error)) (Record, error)

	// Delete removes the design saved for (ownerKind, ownerID), if any.
	// Idempotent: deleting an owner with no saved design is not an
	// error (Stage 13A task Part 42's own DELETE idempotency
	// requirement). internal/alerts.Manager.DeleteRule calls this
	// explicitly as part of deleting a rule (Stage 13A task Part 52's
	// "rule cascade") - a real SQL foreign key cannot express this
	// cascade here, since owner_id is deliberately polymorphic
	// (owner_kind-discriminated, not a single-table foreign key) to let
	// a future owner kind (Stage 13B's chat overlays) share this same
	// table without a schema change.
	Delete(ctx context.Context, ownerKind OwnerKind, ownerID string) error
}
