package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/onboarding"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// OnboardingRepository is the SQLite implementation of
// onboarding.Repository.
type OnboardingRepository struct {
	db *sql.DB
}

// NewOnboardingRepository builds a repository over an open database.
func NewOnboardingRepository(db *sql.DB) *OnboardingRepository {
	return &OnboardingRepository{db: db}
}

var _ onboarding.Repository = (*OnboardingRepository)(nil)

func onboardingStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", onboarding.ErrStorage, op, err)
}

func scanOnboardingState(scanner interface{ Scan(...any) error }) (onboarding.State, error) {
	var (
		st                   onboarding.State
		status               string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&status, &st.SchemaVersion, &createdAt, &updatedAt); err != nil {
		return onboarding.State{}, err
	}
	st.Status = onboarding.Status(status)
	var err error
	if st.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return onboarding.State{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if st.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return onboarding.State{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return st, nil
}

// GetState returns the singleton onboarding state row, if any.
func (r *OnboardingRepository) GetState(ctx context.Context) (onboarding.State, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT status, schema_version, created_at, updated_at FROM onboarding_state WHERE id = 1`)
	st, err := scanOnboardingState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return onboarding.State{}, false, nil
		}
		return onboarding.State{}, false, onboardingStorageErr("get state", err)
	}
	return st, true, nil
}

// SetStatus replaces the singleton row's status and schema version.
func (r *OnboardingRepository) SetStatus(ctx context.Context, status onboarding.Status, schemaVersion int, now time.Time) (onboarding.State, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO onboarding_state (id, status, schema_version, created_at, updated_at)
        VALUES (1, ?, ?, ?, ?)
        ON CONFLICT (id) DO UPDATE SET
            status = excluded.status,
            schema_version = excluded.schema_version,
            updated_at = excluded.updated_at`,
		string(status), schemaVersion, nowText, nowText,
	); err != nil {
		return onboarding.State{}, onboardingStorageErr("set status", err)
	}

	saved, found, err := r.GetState(ctx)
	if err != nil {
		return onboarding.State{}, err
	}
	if !found {
		return onboarding.State{}, onboardingStorageErr("set status", errors.New("state missing immediately after write"))
	}
	return saved, nil
}
