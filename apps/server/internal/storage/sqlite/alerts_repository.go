package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// AlertsRepository is the SQLite implementation of alerts.Repository.
type AlertsRepository struct {
	db *sql.DB
}

// NewAlertsRepository builds a repository over an open database.
func NewAlertsRepository(db *sql.DB) *AlertsRepository {
	return &AlertsRepository{db: db}
}

var _ alerts.Repository = (*AlertsRepository)(nil)

func alertsStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", alerts.ErrStorage, op, err)
}

// --- profiles -------------------------------------------------------------

const alertProfileColumns = `id, public_slug, name, enabled, language, theme, position, text_align,
	max_queue_items, maximum_queue_age_seconds, created_at, updated_at`

func scanAlertProfile(scanner interface{ Scan(...any) error }) (alerts.Profile, error) {
	var (
		p                    alerts.Profile
		enabled              int
		language             string
		theme, position, ta  string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&p.ID, &p.PublicSlug, &p.Name, &enabled, &language, &theme, &position, &ta,
		&p.MaxQueueItems, &p.MaximumQueueAgeSeconds, &createdAt, &updatedAt,
	); err != nil {
		return alerts.Profile{}, err
	}
	p.Enabled = enabled != 0
	p.Language = alerts.Language(language)
	p.Theme = alerts.Theme(theme)
	p.Position = alerts.Position(position)
	p.TextAlign = alerts.TextAlign(ta)
	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return alerts.Profile{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return alerts.Profile{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

// CreateProfile inserts a new alert profile.
func (r *AlertsRepository) CreateProfile(ctx context.Context, p alerts.Profile) (alerts.Profile, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO alert_profiles (
			id, public_slug, name, enabled, language, theme, position, text_align,
			max_queue_items, maximum_queue_age_seconds, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.PublicSlug, p.Name, boolToInt(p.Enabled), string(p.Language), string(p.Theme), string(p.Position), string(p.TextAlign),
		p.MaxQueueItems, p.MaximumQueueAgeSeconds, nowText, nowText,
	); err != nil {
		return alerts.Profile{}, alertsStorageErr("create alert profile", err)
	}
	saved, found, err := r.GetProfile(ctx, p.ID)
	if err != nil {
		return alerts.Profile{}, err
	}
	if !found {
		return alerts.Profile{}, alertsStorageErr("create alert profile", errors.New("profile missing immediately after write"))
	}
	return saved, nil
}

// GetProfile returns one profile by its management id.
func (r *AlertsRepository) GetProfile(ctx context.Context, id string) (alerts.Profile, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+alertProfileColumns+` FROM alert_profiles WHERE id = ?`, id)
	p, err := scanAlertProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return alerts.Profile{}, false, nil
		}
		return alerts.Profile{}, false, alertsStorageErr("get alert profile", err)
	}
	return p, true, nil
}

// GetProfileByPublicSlug returns one profile by its current public slug.
func (r *AlertsRepository) GetProfileByPublicSlug(ctx context.Context, slug string) (alerts.Profile, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+alertProfileColumns+` FROM alert_profiles WHERE public_slug = ?`, slug)
	p, err := scanAlertProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return alerts.Profile{}, false, nil
		}
		return alerts.Profile{}, false, alertsStorageErr("get alert profile by slug", err)
	}
	return p, true, nil
}

// ListProfiles returns every alert profile, ordered by creation time.
func (r *AlertsRepository) ListProfiles(ctx context.Context) ([]alerts.Profile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+alertProfileColumns+` FROM alert_profiles ORDER BY created_at, id`)
	if err != nil {
		return nil, alertsStorageErr("list alert profiles", err)
	}
	defer rows.Close()
	var out []alerts.Profile
	for rows.Next() {
		p, err := scanAlertProfile(rows)
		if err != nil {
			return nil, alertsStorageErr("list alert profiles", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProfile replaces every editable field. public_slug and
// created_at are unchanged.
func (r *AlertsRepository) UpdateProfile(ctx context.Context, p alerts.Profile) (alerts.Profile, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())
	result, err := r.db.ExecContext(ctx, `
		UPDATE alert_profiles SET
			name = ?, enabled = ?, language = ?, theme = ?, position = ?, text_align = ?,
			max_queue_items = ?, maximum_queue_age_seconds = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, boolToInt(p.Enabled), string(p.Language), string(p.Theme), string(p.Position), string(p.TextAlign),
		p.MaxQueueItems, p.MaximumQueueAgeSeconds, nowText, p.ID,
	)
	if err != nil {
		return alerts.Profile{}, alertsStorageErr("update alert profile", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return alerts.Profile{}, alertsStorageErr("update alert profile", err)
	}
	if affected == 0 {
		return alerts.Profile{}, alerts.ErrProfileNotFound
	}
	saved, found, err := r.GetProfile(ctx, p.ID)
	if err != nil {
		return alerts.Profile{}, err
	}
	if !found {
		return alerts.Profile{}, alertsStorageErr("update alert profile", errors.New("profile missing immediately after write"))
	}
	return saved, nil
}

// RotatePublicSlug atomically replaces a profile's public slug.
func (r *AlertsRepository) RotatePublicSlug(ctx context.Context, id, newSlug string) (alerts.Profile, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())
	result, err := r.db.ExecContext(ctx,
		`UPDATE alert_profiles SET public_slug = ?, updated_at = ? WHERE id = ?`, newSlug, nowText, id)
	if err != nil {
		return alerts.Profile{}, alertsStorageErr("rotate alert profile public slug", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return alerts.Profile{}, alertsStorageErr("rotate alert profile public slug", err)
	}
	if affected == 0 {
		return alerts.Profile{}, alerts.ErrProfileNotFound
	}
	saved, found, err := r.GetProfile(ctx, id)
	if err != nil {
		return alerts.Profile{}, err
	}
	if !found {
		return alerts.Profile{}, alertsStorageErr("rotate alert profile public slug", errors.New("profile missing immediately after write"))
	}
	return saved, nil
}

// DeleteProfile removes a profile; its rules (and their filters) cascade.
func (r *AlertsRepository) DeleteProfile(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM alert_profiles WHERE id = ?`, id); err != nil {
		return alertsStorageErr("delete alert profile", err)
	}
	return nil
}

// --- rules ------------------------------------------------------------------

const ruleColumns = `id, profile_id, name, enabled, event_type, priority, duration_ms,
	minimum_quantity, maximum_quantity, required_role,
	show_platform, show_username, show_message, show_quantity,
	text_template, entry_animation, exit_animation, animation_duration_ms, created_at, updated_at`

func scanRule(scanner interface{ Scan(...any) error }) (alerts.Rule, error) {
	var (
		ru                                                    alerts.Rule
		enabled, showPlatform, showUsername, showMsg, showQty int
		eventType, requiredRole, entryAnim, exitAnim          string
		minQty, maxQty                                        sql.NullInt64
		createdAt, updatedAt                                  string
	)
	if err := scanner.Scan(
		&ru.ID, &ru.ProfileID, &ru.Name, &enabled, &eventType, &ru.Priority, &ru.DurationMS,
		&minQty, &maxQty, &requiredRole,
		&showPlatform, &showUsername, &showMsg, &showQty,
		&ru.TextTemplate, &entryAnim, &exitAnim, &ru.AnimationDurationMS, &createdAt, &updatedAt,
	); err != nil {
		return alerts.Rule{}, err
	}
	ru.Enabled = enabled != 0
	ru.EventType = alerts.EventType(eventType)
	ru.RequiredRole = alerts.Role(requiredRole)
	ru.ShowPlatform = showPlatform != 0
	ru.ShowUsername = showUsername != 0
	ru.ShowMessage = showMsg != 0
	ru.ShowQuantity = showQty != 0
	ru.EntryAnimation = alerts.Animation(entryAnim)
	ru.ExitAnimation = alerts.Animation(exitAnim)
	if minQty.Valid {
		v := minQty.Int64
		ru.MinimumQuantity = &v
	}
	if maxQty.Valid {
		v := maxQty.Int64
		ru.MaximumQuantity = &v
	}
	var err error
	if ru.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return alerts.Rule{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if ru.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return alerts.Rule{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return ru, nil
}

func (r *AlertsRepository) loadRuleProviders(ctx context.Context, id string) ([]alerts.ProviderID, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT provider_id FROM alert_rule_providers WHERE rule_id = ? ORDER BY provider_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []alerts.ProviderID
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, alerts.ProviderID(p))
	}
	return out, rows.Err()
}

func (r *AlertsRepository) loadRuleAccounts(ctx context.Context, id string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT connected_account_id FROM alert_rule_accounts WHERE rule_id = ? ORDER BY connected_account_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AlertsRepository) writeRuleChildren(ctx context.Context, tx *sql.Tx, ru alerts.Rule) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_rule_providers WHERE rule_id = ?`, ru.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_rule_accounts WHERE rule_id = ?`, ru.ID); err != nil {
		return err
	}
	for _, p := range ru.Providers {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO alert_rule_providers (rule_id, provider_id) VALUES (?, ?)`, ru.ID, string(p),
		); err != nil {
			return err
		}
	}
	for _, a := range ru.Accounts {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO alert_rule_accounts (rule_id, connected_account_id) VALUES (?, ?)`, ru.ID, a,
		); err != nil {
			return err
		}
	}
	return nil
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// CreateRule inserts a new rule, its provider filters, and its account
// filters in one transaction.
func (r *AlertsRepository) CreateRule(ctx context.Context, ru alerts.Rule) (alerts.Rule, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return alerts.Rule{}, alertsStorageErr("create alert rule", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO alert_rules (
			id, profile_id, name, enabled, event_type, priority, duration_ms,
			minimum_quantity, maximum_quantity, required_role,
			show_platform, show_username, show_message, show_quantity,
			text_template, entry_animation, exit_animation, animation_duration_ms, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ru.ID, ru.ProfileID, ru.Name, boolToInt(ru.Enabled), string(ru.EventType), ru.Priority, ru.DurationMS,
		nullableInt64(ru.MinimumQuantity), nullableInt64(ru.MaximumQuantity), string(ru.RequiredRole),
		boolToInt(ru.ShowPlatform), boolToInt(ru.ShowUsername), boolToInt(ru.ShowMessage), boolToInt(ru.ShowQuantity),
		ru.TextTemplate, string(ru.EntryAnimation), string(ru.ExitAnimation), ru.AnimationDurationMS, nowText, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return alerts.Rule{}, alerts.ErrProfileNotFound
		}
		return alerts.Rule{}, alertsStorageErr("create alert rule", err)
	}
	if err := r.writeRuleChildren(ctx, tx, ru); err != nil {
		if isForeignKeyViolation(err) {
			return alerts.Rule{}, alerts.ErrAccountNotFound
		}
		return alerts.Rule{}, alertsStorageErr("create alert rule", err)
	}
	if err := tx.Commit(); err != nil {
		return alerts.Rule{}, alertsStorageErr("create alert rule", err)
	}

	saved, found, err := r.GetRule(ctx, ru.ID)
	if err != nil {
		return alerts.Rule{}, err
	}
	if !found {
		return alerts.Rule{}, alertsStorageErr("create alert rule", errors.New("rule missing immediately after write"))
	}
	return saved, nil
}

// GetRule returns one rule, its provider filters, and its account
// filters.
func (r *AlertsRepository) GetRule(ctx context.Context, id string) (alerts.Rule, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE id = ?`, id)
	ru, err := scanRule(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return alerts.Rule{}, false, nil
		}
		return alerts.Rule{}, false, alertsStorageErr("get alert rule", err)
	}
	if ru.Providers, err = r.loadRuleProviders(ctx, id); err != nil {
		return alerts.Rule{}, false, alertsStorageErr("get alert rule providers", err)
	}
	if ru.Accounts, err = r.loadRuleAccounts(ctx, id); err != nil {
		return alerts.Rule{}, false, alertsStorageErr("get alert rule accounts", err)
	}
	return ru, true, nil
}

// ListRules returns every rule belonging to profileID, ordered by
// creation time - never SQLite row order, so a caller must never rely
// on this order for match-evaluation semantics (see the Stage 12A
// task's own Part 8).
func (r *AlertsRepository) ListRules(ctx context.Context, profileID string) ([]alerts.Rule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE profile_id = ? ORDER BY created_at, id`, profileID)
	if err != nil {
		return nil, alertsStorageErr("list alert rules", err)
	}
	defer rows.Close()

	var ids []string
	var out []alerts.Rule
	for rows.Next() {
		ru, err := scanRule(rows)
		if err != nil {
			return nil, alertsStorageErr("list alert rules", err)
		}
		out = append(out, ru)
		ids = append(ids, ru.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, alertsStorageErr("list alert rules", err)
	}
	for i, id := range ids {
		if out[i].Providers, err = r.loadRuleProviders(ctx, id); err != nil {
			return nil, alertsStorageErr("list alert rule providers", err)
		}
		if out[i].Accounts, err = r.loadRuleAccounts(ctx, id); err != nil {
			return nil, alertsStorageErr("list alert rule accounts", err)
		}
	}
	return out, nil
}

// UpdateRule replaces every editable field and the full filter sets of
// one rule in one transaction.
func (r *AlertsRepository) UpdateRule(ctx context.Context, ru alerts.Rule) (alerts.Rule, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return alerts.Rule{}, alertsStorageErr("update alert rule", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE alert_rules SET
			name = ?, enabled = ?, event_type = ?, priority = ?, duration_ms = ?,
			minimum_quantity = ?, maximum_quantity = ?, required_role = ?,
			show_platform = ?, show_username = ?, show_message = ?, show_quantity = ?,
			text_template = ?, entry_animation = ?, exit_animation = ?, animation_duration_ms = ?, updated_at = ?
		WHERE id = ?`,
		ru.Name, boolToInt(ru.Enabled), string(ru.EventType), ru.Priority, ru.DurationMS,
		nullableInt64(ru.MinimumQuantity), nullableInt64(ru.MaximumQuantity), string(ru.RequiredRole),
		boolToInt(ru.ShowPlatform), boolToInt(ru.ShowUsername), boolToInt(ru.ShowMessage), boolToInt(ru.ShowQuantity),
		ru.TextTemplate, string(ru.EntryAnimation), string(ru.ExitAnimation), ru.AnimationDurationMS, nowText, ru.ID,
	)
	if err != nil {
		return alerts.Rule{}, alertsStorageErr("update alert rule", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return alerts.Rule{}, alertsStorageErr("update alert rule", err)
	}
	if affected == 0 {
		return alerts.Rule{}, alerts.ErrRuleNotFound
	}
	if err := r.writeRuleChildren(ctx, tx, ru); err != nil {
		if isForeignKeyViolation(err) {
			return alerts.Rule{}, alerts.ErrAccountNotFound
		}
		return alerts.Rule{}, alertsStorageErr("update alert rule", err)
	}
	if err := tx.Commit(); err != nil {
		return alerts.Rule{}, alertsStorageErr("update alert rule", err)
	}

	saved, found, err := r.GetRule(ctx, ru.ID)
	if err != nil {
		return alerts.Rule{}, err
	}
	if !found {
		return alerts.Rule{}, alertsStorageErr("update alert rule", errors.New("rule missing immediately after write"))
	}
	return saved, nil
}

// DeleteRule removes a rule; its filters cascade.
func (r *AlertsRepository) DeleteRule(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id); err != nil {
		return alertsStorageErr("delete alert rule", err)
	}
	return nil
}
