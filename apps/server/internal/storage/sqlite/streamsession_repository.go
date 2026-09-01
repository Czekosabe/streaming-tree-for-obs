package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsession"
)

// StreamSessionRepository is the SQLite implementation of
// streamsession.Repository.
type StreamSessionRepository struct {
	db *sql.DB
}

// NewStreamSessionRepository builds a repository over an open database.
func NewStreamSessionRepository(db *sql.DB) *StreamSessionRepository {
	return &StreamSessionRepository{db: db}
}

var _ streamsession.Repository = (*StreamSessionRepository)(nil)

func streamSessionStorageErr(op string, err error) error {
	return fmt.Errorf("stream session storage failure: %s: %w", op, err)
}

const streamSessionColumns = `id, started_at, ended_at, last_seen_at, end_reason, created_at, updated_at`

func scanSession(scanner interface{ Scan(...any) error }) (streamsession.Session, error) {
	var (
		s                     streamsession.Session
		startedAt, lastSeenAt string
		endedAt               sql.NullString
		createdAt, updatedAt  string
		endReason             string
	)
	if err := scanner.Scan(&s.ID, &startedAt, &endedAt, &lastSeenAt, &endReason, &createdAt, &updatedAt); err != nil {
		return streamsession.Session{}, err
	}

	var err error
	if s.StartedAt, err = platform.ParseTimestamp(startedAt); err != nil {
		return streamsession.Session{}, fmt.Errorf("parse started_at %q: %w", startedAt, err)
	}
	if s.LastSeenAt, err = platform.ParseTimestamp(lastSeenAt); err != nil {
		return streamsession.Session{}, fmt.Errorf("parse last_seen_at %q: %w", lastSeenAt, err)
	}
	if s.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return streamsession.Session{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if s.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return streamsession.Session{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	if endedAt.Valid {
		t, err := platform.ParseTimestamp(endedAt.String)
		if err != nil {
			return streamsession.Session{}, fmt.Errorf("parse ended_at %q: %w", endedAt.String, err)
		}
		s.EndedAt = &t
	}
	s.EndReason = streamsession.EndReason(endReason)
	return s, nil
}

func (r *StreamSessionRepository) CreateSession(ctx context.Context, s streamsession.Session) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO stream_sessions (`+streamSessionColumns+`)
		VALUES (?, ?, NULL, ?, ?, ?, ?)`,
		s.ID, platform.FormatTimestamp(s.StartedAt), platform.FormatTimestamp(s.LastSeenAt),
		string(s.EndReason), platform.FormatTimestamp(s.CreatedAt), platform.FormatTimestamp(s.UpdatedAt),
	); err != nil {
		return streamSessionStorageErr("create session", err)
	}
	return nil
}

func (r *StreamSessionRepository) UpdateSession(ctx context.Context, s streamsession.Session) error {
	var endedAt sql.NullString
	if s.EndedAt != nil {
		endedAt = sql.NullString{String: platform.FormatTimestamp(*s.EndedAt), Valid: true}
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE stream_sessions SET ended_at = ?, last_seen_at = ?, end_reason = ?, updated_at = ?
		WHERE id = ?`,
		endedAt, platform.FormatTimestamp(s.LastSeenAt), string(s.EndReason), platform.FormatTimestamp(s.UpdatedAt), s.ID,
	)
	if err != nil {
		return streamSessionStorageErr("update session", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return streamSessionStorageErr("update session rows affected", err)
	}
	if affected == 0 {
		return streamsession.ErrNotFound
	}
	return nil
}

func (r *StreamSessionRepository) OpenSession(ctx context.Context) (streamsession.Session, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+streamSessionColumns+` FROM stream_sessions WHERE ended_at IS NULL LIMIT 1`)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return streamsession.Session{}, false, nil
	}
	if err != nil {
		return streamsession.Session{}, false, streamSessionStorageErr("open session", err)
	}
	return s, true, nil
}

func (r *StreamSessionRepository) GetSession(ctx context.Context, id string) (streamsession.Session, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+streamSessionColumns+` FROM stream_sessions WHERE id = ?`, id)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return streamsession.Session{}, streamsession.ErrNotFound
	}
	if err != nil {
		return streamsession.Session{}, streamSessionStorageErr("get session", err)
	}
	dests, err := r.listDestinations(ctx, id)
	if err != nil {
		return streamsession.Session{}, err
	}
	s.Destinations = dests
	return s, nil
}

func (r *StreamSessionRepository) ListSessions(ctx context.Context, limit int) ([]streamsession.Session, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+streamSessionColumns+` FROM stream_sessions ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, streamSessionStorageErr("list sessions", err)
	}
	defer rows.Close()

	sessions := make([]streamsession.Session, 0)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, streamSessionStorageErr("scan session", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, streamSessionStorageErr("iterate sessions", err)
	}

	for i := range sessions {
		dests, err := r.listDestinations(ctx, sessions[i].ID)
		if err != nil {
			return nil, err
		}
		sessions[i].Destinations = dests
	}
	return sessions, nil
}

const streamSessionDestinationColumns = `id, session_id, platform_id, provider_id, display_name, started_at, ended_at, outcome, created_at, updated_at`

func scanDestination(scanner interface{ Scan(...any) error }) (streamsession.Destination, error) {
	var (
		d                    streamsession.Destination
		platformID           sql.NullString
		startedAt, endedAt   sql.NullString
		createdAt, updatedAt string
		outcome              string
	)
	if err := scanner.Scan(
		&d.ID, &d.SessionID, &platformID, &d.ProviderID, &d.DisplayName,
		&startedAt, &endedAt, &outcome, &createdAt, &updatedAt,
	); err != nil {
		return streamsession.Destination{}, err
	}
	if platformID.Valid {
		v := platformID.String
		d.PlatformID = &v
	}
	d.Outcome = streamsession.Outcome(outcome)

	var err error
	if d.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return streamsession.Destination{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if d.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return streamsession.Destination{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	if startedAt.Valid {
		t, err := platform.ParseTimestamp(startedAt.String)
		if err != nil {
			return streamsession.Destination{}, fmt.Errorf("parse started_at %q: %w", startedAt.String, err)
		}
		d.StartedAt = t
	}
	if endedAt.Valid {
		t, err := platform.ParseTimestamp(endedAt.String)
		if err != nil {
			return streamsession.Destination{}, fmt.Errorf("parse ended_at %q: %w", endedAt.String, err)
		}
		d.EndedAt = &t
	}
	return d, nil
}

func (r *StreamSessionRepository) listDestinations(ctx context.Context, sessionID string) ([]streamsession.Destination, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+streamSessionDestinationColumns+` FROM stream_session_destinations WHERE session_id = ? ORDER BY started_at, id`, sessionID)
	if err != nil {
		return nil, streamSessionStorageErr("list destinations", err)
	}
	defer rows.Close()

	dests := make([]streamsession.Destination, 0)
	for rows.Next() {
		d, err := scanDestination(rows)
		if err != nil {
			return nil, streamSessionStorageErr("scan destination", err)
		}
		dests = append(dests, d)
	}
	if err := rows.Err(); err != nil {
		return nil, streamSessionStorageErr("iterate destinations", err)
	}
	return dests, nil
}

func (r *StreamSessionRepository) CreateDestination(ctx context.Context, d streamsession.Destination) error {
	var platformID sql.NullString
	if d.PlatformID != nil {
		platformID = sql.NullString{String: *d.PlatformID, Valid: true}
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO stream_session_destinations (`+streamSessionDestinationColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
		d.ID, d.SessionID, platformID, d.ProviderID, d.DisplayName,
		platform.FormatTimestamp(d.StartedAt), string(d.Outcome),
		platform.FormatTimestamp(d.CreatedAt), platform.FormatTimestamp(d.UpdatedAt),
	); err != nil {
		return streamSessionStorageErr("create destination", err)
	}
	return nil
}

func (r *StreamSessionRepository) UpdateDestination(ctx context.Context, d streamsession.Destination) error {
	var endedAt sql.NullString
	if d.EndedAt != nil {
		endedAt = sql.NullString{String: platform.FormatTimestamp(*d.EndedAt), Valid: true}
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE stream_session_destinations SET ended_at = ?, outcome = ?, updated_at = ?
		WHERE id = ?`,
		endedAt, string(d.Outcome), platform.FormatTimestamp(d.UpdatedAt), d.ID,
	)
	if err != nil {
		return streamSessionStorageErr("update destination", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return streamSessionStorageErr("update destination rows affected", err)
	}
	if affected == 0 {
		return streamsession.ErrNotFound
	}
	return nil
}

func (r *StreamSessionRepository) OpenDestinations(ctx context.Context, sessionID string) ([]streamsession.Destination, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+streamSessionDestinationColumns+` FROM stream_session_destinations WHERE session_id = ? AND ended_at IS NULL`, sessionID)
	if err != nil {
		return nil, streamSessionStorageErr("open destinations", err)
	}
	defer rows.Close()

	dests := make([]streamsession.Destination, 0)
	for rows.Next() {
		d, err := scanDestination(rows)
		if err != nil {
			return nil, streamSessionStorageErr("scan open destination", err)
		}
		dests = append(dests, d)
	}
	return dests, rows.Err()
}

func (r *StreamSessionRepository) PruneSessionsBefore(ctx context.Context, cutoff time.Time) (int, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM stream_sessions WHERE ended_at IS NOT NULL AND ended_at < ?`, platform.FormatTimestamp(cutoff))
	if err != nil {
		return 0, streamSessionStorageErr("prune sessions", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, streamSessionStorageErr("prune sessions rows affected", err)
	}
	return int(affected), nil
}

func (r *StreamSessionRepository) DeleteAllSessions(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM stream_sessions`); err != nil {
		return streamSessionStorageErr("delete all sessions", err)
	}
	return nil
}

func (r *StreamSessionRepository) GetRetentionDays(ctx context.Context) (int, bool, error) {
	var days int
	err := r.db.QueryRowContext(ctx, `SELECT retention_days FROM stream_session_settings WHERE id = 1`).Scan(&days)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, streamSessionStorageErr("get retention days", err)
	}
	return days, true, nil
}

func (r *StreamSessionRepository) SetRetentionDays(ctx context.Context, days int, now time.Time) error {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO stream_session_settings (id, retention_days, created_at, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			retention_days = excluded.retention_days,
			updated_at = excluded.updated_at`,
		days, nowText, nowText,
	); err != nil {
		return streamSessionStorageErr("set retention days", err)
	}
	return nil
}
