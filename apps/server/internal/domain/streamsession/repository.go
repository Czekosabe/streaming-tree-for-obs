package streamsession

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound means no session exists with the given id.
var ErrNotFound = errors.New("stream session not found")

// Repository is the persistence port this domain depends on. Every
// method operates on the real SQLite-backed store in production
// (internal/storage/sqlite) and an in-memory fake in tests.
type Repository interface {
	// CreateSession inserts a new, open session row.
	CreateSession(ctx context.Context, s Session) error
	// UpdateSession replaces a session's own mutable fields
	// (LastSeenAt, EndedAt, EndReason, UpdatedAt) - never its id or
	// StartedAt.
	UpdateSession(ctx context.Context, s Session) error
	// OpenSession returns the session with EndedAt still nil, if any.
	// By construction there is never more than one at a time.
	OpenSession(ctx context.Context) (Session, bool, error)
	// GetSession returns one session with its destination rows.
	GetSession(ctx context.Context, id string) (Session, error)
	// ListSessions returns sessions newest-first, bounded by limit.
	ListSessions(ctx context.Context, limit int) ([]Session, error)

	// CreateDestination inserts a new, open destination-participation
	// row.
	CreateDestination(ctx context.Context, d Destination) error
	// UpdateDestination replaces a destination row's own mutable
	// fields (EndedAt, Outcome, UpdatedAt).
	UpdateDestination(ctx context.Context, d Destination) error
	// OpenDestinations returns every open destination row for one
	// session.
	OpenDestinations(ctx context.Context, sessionID string) ([]Destination, error)

	// PruneSessionsBefore deletes every session whose EndedAt is
	// before cutoff (ON DELETE CASCADE removes their destination
	// rows) - an open session (EndedAt nil) is never pruned regardless
	// of age. Returns the number of sessions deleted.
	PruneSessionsBefore(ctx context.Context, cutoff time.Time) (int, error)
	// DeleteAllSessions deletes every session (and, via cascade, every
	// destination row) - the explicit "Clear history" action.
	DeleteAllSessions(ctx context.Context) error
}
