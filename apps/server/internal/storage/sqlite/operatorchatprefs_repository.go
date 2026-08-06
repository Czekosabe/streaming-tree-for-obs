package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// OperatorChatPrefsRepository is the SQLite implementation of
// operatorchatprefs.Repository.
type OperatorChatPrefsRepository struct {
	db *sql.DB
}

// NewOperatorChatPrefsRepository builds a repository over an open database.
func NewOperatorChatPrefsRepository(db *sql.DB) *OperatorChatPrefsRepository {
	return &OperatorChatPrefsRepository{db: db}
}

var _ operatorchatprefs.Repository = (*OperatorChatPrefsRepository)(nil)

func operatorChatPrefsStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", operatorchatprefs.ErrStorage, op, err)
}

const preferencesColumns = `show_platform_icon, show_platform_name, show_account_label, show_badges,
	show_timestamps, show_activity_events, show_deleted_messages, hide_command_messages, compact_mode,
	created_at, updated_at`

func scanPreferences(scanner interface{ Scan(...any) error }) (operatorchatprefs.Preferences, error) {
	var (
		p                                                                                                        operatorchatprefs.Preferences
		showIcon, showName, showAccount, showBadges, showTimestamps, showActivity, showDeleted, hideCmd, compact int
		createdAt, updatedAt                                                                                     string
	)
	if err := scanner.Scan(&showIcon, &showName, &showAccount, &showBadges, &showTimestamps,
		&showActivity, &showDeleted, &hideCmd, &compact, &createdAt, &updatedAt); err != nil {
		return operatorchatprefs.Preferences{}, err
	}
	p.ShowPlatformIcon = showIcon != 0
	p.ShowPlatformName = showName != 0
	p.ShowAccountLabel = showAccount != 0
	p.ShowBadges = showBadges != 0
	p.ShowTimestamps = showTimestamps != 0
	p.ShowActivityEvents = showActivity != 0
	p.ShowDeletedMessages = showDeleted != 0
	p.HideCommandMessages = hideCmd != 0
	p.CompactMode = compact != 0
	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return operatorchatprefs.Preferences{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return operatorchatprefs.Preferences{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

// GetPreferences returns the singleton preferences row, if any.
func (r *OperatorChatPrefsRepository) GetPreferences(ctx context.Context) (operatorchatprefs.Preferences, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+preferencesColumns+` FROM operator_chat_preferences WHERE id = 1`)
	p, err := scanPreferences(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return operatorchatprefs.Preferences{}, false, nil
		}
		return operatorchatprefs.Preferences{}, false, operatorChatPrefsStorageErr("get preferences", err)
	}
	return p, true, nil
}

// SetPreferences replaces the singleton preferences row in full.
func (r *OperatorChatPrefsRepository) SetPreferences(ctx context.Context, p operatorchatprefs.Preferences, now time.Time) (operatorchatprefs.Preferences, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO operator_chat_preferences (
            id, show_platform_icon, show_platform_name, show_account_label, show_badges,
            show_timestamps, show_activity_events, show_deleted_messages, hide_command_messages, compact_mode,
            created_at, updated_at
        ) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (id) DO UPDATE SET
            show_platform_icon = excluded.show_platform_icon,
            show_platform_name = excluded.show_platform_name,
            show_account_label = excluded.show_account_label,
            show_badges = excluded.show_badges,
            show_timestamps = excluded.show_timestamps,
            show_activity_events = excluded.show_activity_events,
            show_deleted_messages = excluded.show_deleted_messages,
            hide_command_messages = excluded.hide_command_messages,
            compact_mode = excluded.compact_mode,
            updated_at = excluded.updated_at`,
		boolToInt(p.ShowPlatformIcon), boolToInt(p.ShowPlatformName), boolToInt(p.ShowAccountLabel), boolToInt(p.ShowBadges),
		boolToInt(p.ShowTimestamps), boolToInt(p.ShowActivityEvents), boolToInt(p.ShowDeletedMessages), boolToInt(p.HideCommandMessages), boolToInt(p.CompactMode),
		nowText, nowText,
	); err != nil {
		return operatorchatprefs.Preferences{}, operatorChatPrefsStorageErr("set preferences", err)
	}

	saved, found, err := r.GetPreferences(ctx)
	if err != nil {
		return operatorchatprefs.Preferences{}, err
	}
	if !found {
		return operatorchatprefs.Preferences{}, operatorChatPrefsStorageErr("set preferences", errors.New("preferences missing immediately after write"))
	}
	return saved, nil
}

const accountVisibilityColumns = `account_id, visible, created_at, updated_at`

func scanAccountVisibility(scanner interface{ Scan(...any) error }) (operatorchatprefs.AccountVisibility, error) {
	var (
		v                    operatorchatprefs.AccountVisibility
		visible              int
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&v.AccountID, &visible, &createdAt, &updatedAt); err != nil {
		return operatorchatprefs.AccountVisibility{}, err
	}
	v.Visible = visible != 0
	var err error
	if v.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return operatorchatprefs.AccountVisibility{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if v.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return operatorchatprefs.AccountVisibility{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return v, nil
}

// ListAccountVisibility returns every account with an explicit visibility
// preference, ordered by account_id for a stable, test-friendly order.
func (r *OperatorChatPrefsRepository) ListAccountVisibility(ctx context.Context) ([]operatorchatprefs.AccountVisibility, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountVisibilityColumns+` FROM operator_chat_account_visibility ORDER BY account_id`)
	if err != nil {
		return nil, operatorChatPrefsStorageErr("list account visibility", err)
	}
	defer rows.Close()

	var out []operatorchatprefs.AccountVisibility
	for rows.Next() {
		v, err := scanAccountVisibility(rows)
		if err != nil {
			return nil, operatorChatPrefsStorageErr("list account visibility", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, operatorChatPrefsStorageErr("list account visibility", err)
	}
	return out, nil
}

// SetAccountVisibility creates or replaces one account's visibility
// preference.
func (r *OperatorChatPrefsRepository) SetAccountVisibility(ctx context.Context, accountID string, visible bool, now time.Time) (operatorchatprefs.AccountVisibility, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO operator_chat_account_visibility (account_id, visible, created_at, updated_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT (account_id) DO UPDATE SET
            visible = excluded.visible, updated_at = excluded.updated_at`,
		accountID, boolToInt(visible), nowText, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return operatorchatprefs.AccountVisibility{}, operatorchatprefs.ErrAccountNotFound
		}
		return operatorchatprefs.AccountVisibility{}, operatorChatPrefsStorageErr("set account visibility", err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountVisibilityColumns+` FROM operator_chat_account_visibility WHERE account_id = ?`, accountID)
	v, err := scanAccountVisibility(row)
	if err != nil {
		return operatorchatprefs.AccountVisibility{}, operatorChatPrefsStorageErr("set account visibility", err)
	}
	return v, nil
}

const userRefColumns = `id, provider_id, connected_account_id, provider_user_id, label, created_at`

func scanUserRef(scanner interface{ Scan(...any) error }) (operatorchatprefs.UserRef, error) {
	var (
		ref        operatorchatprefs.UserRef
		providerID string
		label      sql.NullString
		createdAt  string
	)
	if err := scanner.Scan(&ref.ID, &providerID, &ref.ConnectedAccountID, &ref.ProviderUserID, &label, &createdAt); err != nil {
		return operatorchatprefs.UserRef{}, err
	}
	ref.ProviderID = operatorchatprefs.ProviderID(providerID)
	ref.Label = label.String
	var err error
	if ref.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return operatorchatprefs.UserRef{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	return ref, nil
}

func (r *OperatorChatPrefsRepository) listUserRefs(ctx context.Context, table string) ([]operatorchatprefs.UserRef, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+userRefColumns+` FROM `+table+` ORDER BY created_at, id`)
	if err != nil {
		return nil, operatorChatPrefsStorageErr("list "+table, err)
	}
	defer rows.Close()

	var out []operatorchatprefs.UserRef
	for rows.Next() {
		ref, err := scanUserRef(rows)
		if err != nil {
			return nil, operatorChatPrefsStorageErr("list "+table, err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, operatorChatPrefsStorageErr("list "+table, err)
	}
	return out, nil
}

// addUserRef inserts ref into table, idempotently: a repeated
// provider/connected-account/provider-user-id tuple returns the
// already-listed entry unchanged rather than a duplicate or an error.
func (r *OperatorChatPrefsRepository) addUserRef(ctx context.Context, table string, ref operatorchatprefs.UserRef, now time.Time) (operatorchatprefs.UserRef, error) {
	var label sql.NullString
	if ref.Label != "" {
		label = sql.NullString{String: ref.Label, Valid: true}
	}
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO `+table+` (id, provider_id, connected_account_id, provider_user_id, label, created_at)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT (provider_id, connected_account_id, provider_user_id) DO NOTHING`,
		ref.ID, string(ref.ProviderID), ref.ConnectedAccountID, ref.ProviderUserID, label, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return operatorchatprefs.UserRef{}, operatorchatprefs.ErrAccountNotFound
		}
		return operatorchatprefs.UserRef{}, operatorChatPrefsStorageErr("add to "+table, err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT `+userRefColumns+` FROM `+table+` WHERE provider_id = ? AND connected_account_id = ? AND provider_user_id = ?`,
		string(ref.ProviderID), ref.ConnectedAccountID, ref.ProviderUserID)
	saved, err := scanUserRef(row)
	if err != nil {
		return operatorchatprefs.UserRef{}, operatorChatPrefsStorageErr("add to "+table, err)
	}
	return saved, nil
}

func (r *OperatorChatPrefsRepository) removeUserRef(ctx context.Context, table, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		return operatorChatPrefsStorageErr("remove from "+table, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return operatorChatPrefsStorageErr("remove from "+table, err)
	}
	if affected == 0 {
		return operatorchatprefs.ErrUserNotFound
	}
	return nil
}

// ListHiddenUsers returns every operator-hidden user.
func (r *OperatorChatPrefsRepository) ListHiddenUsers(ctx context.Context) ([]operatorchatprefs.UserRef, error) {
	return r.listUserRefs(ctx, "operator_chat_hidden_users")
}

// AddHiddenUser adds one user to the hidden list, idempotently.
func (r *OperatorChatPrefsRepository) AddHiddenUser(ctx context.Context, ref operatorchatprefs.UserRef, now time.Time) (operatorchatprefs.UserRef, error) {
	return r.addUserRef(ctx, "operator_chat_hidden_users", ref, now)
}

// RemoveHiddenUser removes one hidden-user entry by its own id.
func (r *OperatorChatPrefsRepository) RemoveHiddenUser(ctx context.Context, id string) error {
	return r.removeUserRef(ctx, "operator_chat_hidden_users", id)
}

// ListBotUsers returns every operator-marked bot user.
func (r *OperatorChatPrefsRepository) ListBotUsers(ctx context.Context) ([]operatorchatprefs.UserRef, error) {
	return r.listUserRefs(ctx, "operator_chat_bot_users")
}

// AddBotUser adds one user to the bot list, idempotently.
func (r *OperatorChatPrefsRepository) AddBotUser(ctx context.Context, ref operatorchatprefs.UserRef, now time.Time) (operatorchatprefs.UserRef, error) {
	return r.addUserRef(ctx, "operator_chat_bot_users", ref, now)
}

// RemoveBotUser removes one bot-user entry by its own id.
func (r *OperatorChatPrefsRepository) RemoveBotUser(ctx context.Context, id string) error {
	return r.removeUserRef(ctx, "operator_chat_bot_users", id)
}
