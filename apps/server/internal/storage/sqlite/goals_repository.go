package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// GoalsRepository is the SQLite implementation of goals.Repository.
type GoalsRepository struct {
	db *sql.DB
}

// NewGoalsRepository builds a repository over an open database.
func NewGoalsRepository(db *sql.DB) *GoalsRepository {
	return &GoalsRepository{db: db}
}

var _ goals.Repository = (*GoalsRepository)(nil)

func goalsStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", goals.ErrStorage, op, err)
}

const goalColumns = `id, name, kind, enabled, target, current_value, baseline, currency,
	created_at, updated_at, started_at, config_revision`

func scanGoal(scanner interface{ Scan(...any) error }) (goals.Goal, error) {
	var (
		g                               goals.Goal
		kind                            string
		enabled                         int
		currency                        sql.NullString
		createdAt, updatedAt, startedAt string
	)
	if err := scanner.Scan(
		&g.ID, &g.Name, &kind, &enabled, &g.Target, &g.Current, &g.Baseline, &currency,
		&createdAt, &updatedAt, &startedAt, &g.ConfigRevision,
	); err != nil {
		return goals.Goal{}, err
	}
	g.Kind = goals.Kind(kind)
	g.Enabled = enabled != 0
	g.Currency = currency.String
	var err error
	if g.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return goals.Goal{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if g.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return goals.Goal{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	if g.StartedAt, err = platform.ParseTimestamp(startedAt); err != nil {
		return goals.Goal{}, fmt.Errorf("parse started_at %q: %w", startedAt, err)
	}
	return g, nil
}

func (r *GoalsRepository) loadFilters(ctx context.Context, tx *sql.Tx, goalID string) (providers []goals.ProviderID, accounts []string, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT provider_id FROM goal_providers WHERE goal_id = ? ORDER BY provider_id`, goalID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, nil, err
		}
		providers = append(providers, goals.ProviderID(p))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	rows2, err := tx.QueryContext(ctx, `SELECT account_id FROM goal_accounts WHERE goal_id = ? ORDER BY account_id`, goalID)
	if err != nil {
		return nil, nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var a string
		if err := rows2.Scan(&a); err != nil {
			return nil, nil, err
		}
		accounts = append(accounts, a)
	}
	if err := rows2.Err(); err != nil {
		return nil, nil, err
	}
	return providers, accounts, nil
}

func writeGoalFilters(ctx context.Context, tx *sql.Tx, g goals.Goal) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM goal_providers WHERE goal_id = ?`, g.ID); err != nil {
		return err
	}
	for _, p := range g.Providers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO goal_providers (goal_id, provider_id) VALUES (?, ?)`, g.ID, string(p)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM goal_accounts WHERE goal_id = ?`, g.ID); err != nil {
		return err
	}
	for _, a := range g.Accounts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO goal_accounts (goal_id, account_id) VALUES (?, ?)`, g.ID, a); err != nil {
			return err
		}
	}
	return nil
}

func nullableCurrency(currency string) sql.NullString {
	if currency == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: currency, Valid: true}
}

// --- goals -----------------------------------------------------------------

func (r *GoalsRepository) CreateGoal(ctx context.Context, g goals.Goal) (goals.Goal, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return goals.Goal{}, goalsStorageErr("begin create goal", err)
	}
	defer tx.Rollback()

	nowText := platform.FormatTimestamp(g.CreatedAt)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO goals (id, name, kind, enabled, target, current_value, baseline, currency,
			created_at, updated_at, started_at, config_revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.Name, string(g.Kind), boolToInt(g.Enabled), g.Target, g.Current, g.Baseline, nullableCurrency(g.Currency),
		nowText, nowText, nowText, g.ConfigRevision,
	); err != nil {
		return goals.Goal{}, goalsStorageErr("create goal", err)
	}
	if err := writeGoalFilters(ctx, tx, g); err != nil {
		return goals.Goal{}, goalsStorageErr("create goal filters", err)
	}
	if err := tx.Commit(); err != nil {
		return goals.Goal{}, goalsStorageErr("commit create goal", err)
	}

	created, ok, err := r.GetGoal(ctx, g.ID)
	if err != nil {
		return goals.Goal{}, err
	}
	if !ok {
		return goals.Goal{}, goalsStorageErr("create goal", errors.New("goal missing immediately after write"))
	}
	return created, nil
}

func (r *GoalsRepository) GetGoal(ctx context.Context, id string) (goals.Goal, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return goals.Goal{}, false, goalsStorageErr("begin get goal", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT `+goalColumns+` FROM goals WHERE id = ?`, id)
	g, err := scanGoal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return goals.Goal{}, false, nil
	}
	if err != nil {
		return goals.Goal{}, false, goalsStorageErr("get goal", err)
	}
	providers, accounts, err := r.loadFilters(ctx, tx, id)
	if err != nil {
		return goals.Goal{}, false, goalsStorageErr("get goal filters", err)
	}
	g.Providers, g.Accounts = providers, accounts
	return g, true, nil
}

func (r *GoalsRepository) ListGoals(ctx context.Context) ([]goals.Goal, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, goalsStorageErr("begin list goals", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT `+goalColumns+` FROM goals ORDER BY created_at, id`)
	if err != nil {
		return nil, goalsStorageErr("list goals", err)
	}
	var list []goals.Goal
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			rows.Close()
			return nil, goalsStorageErr("list goals", err)
		}
		list = append(list, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, goalsStorageErr("list goals", err)
	}
	rows.Close()

	for i := range list {
		providers, accounts, err := r.loadFilters(ctx, tx, list[i].ID)
		if err != nil {
			return nil, goalsStorageErr("list goal filters", err)
		}
		list[i].Providers, list[i].Accounts = providers, accounts
	}
	return list, nil
}

func (r *GoalsRepository) UpdateGoal(ctx context.Context, g goals.Goal) (goals.Goal, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return goals.Goal{}, goalsStorageErr("begin update goal", err)
	}
	defer tx.Rollback()

	var currentRevision int64
	err = tx.QueryRowContext(ctx, `SELECT config_revision FROM goals WHERE id = ?`, g.ID).Scan(&currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return goals.Goal{}, goals.ErrGoalNotFound
	}
	if err != nil {
		return goals.Goal{}, goalsStorageErr("update goal", err)
	}
	if currentRevision != g.ConfigRevision {
		return goals.Goal{}, goals.ErrConfigConflict
	}

	nowText := platform.FormatTimestamp(g.UpdatedAt)
	startedAtText := platform.FormatTimestamp(g.StartedAt)
	if _, err := tx.ExecContext(ctx, `
		UPDATE goals SET name = ?, kind = ?, enabled = ?, target = ?, current_value = ?, baseline = ?,
			currency = ?, updated_at = ?, started_at = ?, config_revision = config_revision + 1
		WHERE id = ?`,
		g.Name, string(g.Kind), boolToInt(g.Enabled), g.Target, g.Current, g.Baseline, nullableCurrency(g.Currency),
		nowText, startedAtText, g.ID,
	); err != nil {
		return goals.Goal{}, goalsStorageErr("update goal", err)
	}
	if err := writeGoalFilters(ctx, tx, g); err != nil {
		return goals.Goal{}, goalsStorageErr("update goal filters", err)
	}
	if err := tx.Commit(); err != nil {
		return goals.Goal{}, goalsStorageErr("commit update goal", err)
	}

	updated, ok, err := r.GetGoal(ctx, g.ID)
	if err != nil {
		return goals.Goal{}, err
	}
	if !ok {
		return goals.Goal{}, goalsStorageErr("update goal", errors.New("goal missing immediately after write"))
	}
	return updated, nil
}

func (r *GoalsRepository) DeleteGoal(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return goalsStorageErr("begin delete goal", err)
	}
	defer tx.Rollback()

	var refCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM widget_profiles WHERE goal_id = ?`, id).Scan(&refCount); err != nil {
		return goalsStorageErr("check goal references", err)
	}
	if refCount > 0 {
		return goals.ErrGoalInUse
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM goals WHERE id = ?`, id)
	if err != nil {
		return goalsStorageErr("delete goal", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return goalsStorageErr("delete goal", err)
	}
	if n == 0 {
		return goals.ErrGoalNotFound
	}
	if err := tx.Commit(); err != nil {
		return goalsStorageErr("commit delete goal", err)
	}
	return nil
}

func (r *GoalsRepository) SetCurrent(ctx context.Context, id string, current int64) (goals.Goal, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE goals SET current_value = ?, updated_at = ? WHERE id = ?`,
		current, platform.FormatTimestamp(time.Now().UTC()), id)
	if err != nil {
		return goals.Goal{}, goalsStorageErr("set current", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return goals.Goal{}, goalsStorageErr("set current", err)
	}
	if n == 0 {
		return goals.Goal{}, goals.ErrGoalNotFound
	}
	updated, ok, err := r.GetGoal(ctx, id)
	if err != nil {
		return goals.Goal{}, err
	}
	if !ok {
		return goals.Goal{}, goals.ErrGoalNotFound
	}
	return updated, nil
}

func (r *GoalsRepository) ResetProgress(ctx context.Context, id string) (goals.Goal, error) {
	now := platform.FormatTimestamp(time.Now().UTC())
	res, err := r.db.ExecContext(ctx, `
		UPDATE goals SET current_value = baseline, updated_at = ?, started_at = ? WHERE id = ?`,
		now, now, id)
	if err != nil {
		return goals.Goal{}, goalsStorageErr("reset progress", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return goals.Goal{}, goalsStorageErr("reset progress", err)
	}
	if n == 0 {
		return goals.Goal{}, goals.ErrGoalNotFound
	}
	updated, ok, err := r.GetGoal(ctx, id)
	if err != nil {
		return goals.Goal{}, err
	}
	if !ok {
		return goals.Goal{}, goals.ErrGoalNotFound
	}
	return updated, nil
}

// ApplyContribution implements the atomic, per-goal dedupe-then-increment
// transaction from docs/goals-widgets.md §12.
func (r *GoalsRepository) ApplyContribution(ctx context.Context, goalID string, key goals.AppliedEventKey, amount int64) (bool, goals.Goal, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, goals.Goal{}, goalsStorageErr("begin apply contribution", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO goal_applied_events (goal_id, provider_id, account_id, provider_event_key, applied_at)
		VALUES (?, ?, ?, ?, ?)`,
		goalID, string(key.ProviderID), key.AccountID, key.ProviderEventKey, platform.FormatTimestamp(time.Now().UTC()),
	)
	if isUniqueViolation(err) {
		// Already applied for this goal - not an error, the normal
		// outcome of a duplicate delivery (docs/goals-widgets.md §12).
		return false, goals.Goal{}, nil
	}
	if err != nil {
		return false, goals.Goal{}, goalsStorageErr("record applied event", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE goals SET current_value = current_value + ?, updated_at = ? WHERE id = ?`,
		amount, platform.FormatTimestamp(time.Now().UTC()), goalID)
	if err != nil {
		return false, goals.Goal{}, goalsStorageErr("apply contribution", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, goals.Goal{}, goalsStorageErr("apply contribution", err)
	}
	if n == 0 {
		return false, goals.Goal{}, goals.ErrGoalNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, goals.Goal{}, goalsStorageErr("commit apply contribution", err)
	}

	updated, ok, err := r.GetGoal(ctx, goalID)
	if err != nil {
		return false, goals.Goal{}, err
	}
	if !ok {
		return false, goals.Goal{}, goals.ErrGoalNotFound
	}
	return true, updated, nil
}

func (r *GoalsRepository) PruneAppliedEvents(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM goal_applied_events WHERE applied_at < ?`, platform.FormatTimestamp(olderThan))
	if err != nil {
		return 0, goalsStorageErr("prune applied events", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, goalsStorageErr("prune applied events", err)
	}
	return n, nil
}

// --- widget profiles ---------------------------------------------------

const widgetProfileColumns = `id, kind, goal_id, name, enabled, public_slug, title_override,
	show_current, show_target, show_percent, show_provider, show_time, show_message, max_items,
	currency, metric, columns, orientation, text_align, font_family,
	background_color, foreground_color, fill_color, border_color, border_radius_px, opacity,
	created_at, updated_at`

func scanWidgetProfile(scanner interface{ Scan(...any) error }) (goals.WidgetProfile, error) {
	var (
		p                                             goals.WidgetProfile
		kind                                          string
		goalID                                        sql.NullString
		enabled, showCurrent, showTarget, showPercent int
		showProvider, showTime, showMessage           int
		currency, metric                              sql.NullString
		orientation, textAlign, font                  string
		createdAt, updatedAt                          string
	)
	if err := scanner.Scan(
		&p.ID, &kind, &goalID, &p.Name, &enabled, &p.PublicSlug, &p.TitleOverride,
		&showCurrent, &showTarget, &showPercent, &showProvider, &showTime, &showMessage, &p.MaxItems,
		&currency, &metric, &p.Columns, &orientation, &textAlign, &font,
		&p.BackgroundColor, &p.ForegroundColor, &p.FillColor, &p.BorderColor, &p.BorderRadiusPx, &p.Opacity,
		&createdAt, &updatedAt,
	); err != nil {
		return goals.WidgetProfile{}, err
	}
	p.Kind = goals.WidgetProfileKind(kind)
	p.GoalID = goalID.String
	p.Enabled = enabled != 0
	p.ShowCurrent = showCurrent != 0
	p.ShowTarget = showTarget != 0
	p.ShowPercent = showPercent != 0
	p.ShowProvider = showProvider != 0
	p.ShowTime = showTime != 0
	p.ShowMessage = showMessage != 0
	p.Currency = currency.String
	p.Metric = goals.SessionMetric(metric.String)
	p.Orientation = goals.Orientation(orientation)
	p.TextAlign = goals.TextAlign(textAlign)
	p.FontFamily = goals.FontFamily(font)
	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return goals.WidgetProfile{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return goals.WidgetProfile{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// loadWidgetProfileExtras fills p.Providers/Accounts/EventTypes/Children
// from their own child tables (docs/supporter-widgets.md §18) - called
// after every scanWidgetProfile so every returned WidgetProfile is fully
// populated regardless of its Kind.
func (r *GoalsRepository) loadWidgetProfileExtras(ctx context.Context, tx queryer, p *goals.WidgetProfile) error {
	rows, err := tx.QueryContext(ctx, `SELECT provider_id FROM widget_profile_providers WHERE widget_profile_id = ? ORDER BY provider_id`, p.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		p.Providers = append(p.Providers, goals.ProviderID(v))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows2, err := tx.QueryContext(ctx, `SELECT account_id FROM widget_profile_accounts WHERE widget_profile_id = ? ORDER BY account_id`, p.ID)
	if err != nil {
		return err
	}
	for rows2.Next() {
		var v string
		if err := rows2.Scan(&v); err != nil {
			rows2.Close()
			return err
		}
		p.Accounts = append(p.Accounts, v)
	}
	if err := rows2.Err(); err != nil {
		rows2.Close()
		return err
	}
	rows2.Close()

	rows3, err := tx.QueryContext(ctx, `SELECT event_type FROM widget_profile_event_types WHERE widget_profile_id = ? ORDER BY event_type`, p.ID)
	if err != nil {
		return err
	}
	for rows3.Next() {
		var v string
		if err := rows3.Scan(&v); err != nil {
			rows3.Close()
			return err
		}
		p.EventTypes = append(p.EventTypes, goals.SupporterEventType(v))
	}
	if err := rows3.Err(); err != nil {
		rows3.Close()
		return err
	}
	rows3.Close()

	rows4, err := tx.QueryContext(ctx, `
		SELECT child_id, column_start, column_span, row_start, row_span
		FROM widget_profile_dashboard_children WHERE dashboard_id = ? ORDER BY position`, p.ID)
	if err != nil {
		return err
	}
	defer rows4.Close()
	for rows4.Next() {
		var c goals.DashboardChild
		if err := rows4.Scan(&c.WidgetProfileID, &c.Column, &c.ColumnSpan, &c.Row, &c.RowSpan); err != nil {
			return err
		}
		p.Children = append(p.Children, c)
	}
	return rows4.Err()
}

func writeWidgetProfileExtras(ctx context.Context, tx *sql.Tx, p goals.WidgetProfile) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM widget_profile_providers WHERE widget_profile_id = ?`, p.ID); err != nil {
		return err
	}
	for _, v := range p.Providers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO widget_profile_providers (widget_profile_id, provider_id) VALUES (?, ?)`, p.ID, string(v)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM widget_profile_accounts WHERE widget_profile_id = ?`, p.ID); err != nil {
		return err
	}
	for _, v := range p.Accounts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO widget_profile_accounts (widget_profile_id, account_id) VALUES (?, ?)`, p.ID, v); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM widget_profile_event_types WHERE widget_profile_id = ?`, p.ID); err != nil {
		return err
	}
	for _, v := range p.EventTypes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO widget_profile_event_types (widget_profile_id, event_type) VALUES (?, ?)`, p.ID, string(v)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM widget_profile_dashboard_children WHERE dashboard_id = ?`, p.ID); err != nil {
		return err
	}
	for i, c := range p.Children {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO widget_profile_dashboard_children (dashboard_id, child_id, position, column_start, column_span, row_start, row_span)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.ID, c.WidgetProfileID, i, c.Column, c.ColumnSpan, c.Row, c.RowSpan,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *GoalsRepository) CreateWidgetProfile(ctx context.Context, p goals.WidgetProfile) (goals.WidgetProfile, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("begin create widget profile", err)
	}
	defer tx.Rollback()

	nowText := platform.FormatTimestamp(p.CreatedAt)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO widget_profiles (id, kind, goal_id, name, enabled, public_slug, title_override,
			show_current, show_target, show_percent, show_provider, show_time, show_message, max_items,
			currency, metric, columns, orientation, text_align, font_family,
			background_color, foreground_color, fill_color, border_color, border_radius_px, opacity,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, string(p.Kind), nullableString(p.GoalID), p.Name, boolToInt(p.Enabled), p.PublicSlug, p.TitleOverride,
		boolToInt(p.ShowCurrent), boolToInt(p.ShowTarget), boolToInt(p.ShowPercent),
		boolToInt(p.ShowProvider), boolToInt(p.ShowTime), boolToInt(p.ShowMessage), p.MaxItems,
		nullableString(p.Currency), nullableString(string(p.Metric)), p.Columns,
		string(p.Orientation), string(p.TextAlign), string(p.FontFamily),
		p.BackgroundColor, p.ForegroundColor, p.FillColor, p.BorderColor, p.BorderRadiusPx, p.Opacity,
		nowText, nowText,
	); err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("create widget profile", err)
	}
	if err := writeWidgetProfileExtras(ctx, tx, p); err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("create widget profile extras", err)
	}
	if err := tx.Commit(); err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("commit create widget profile", err)
	}

	created, ok, err := r.GetWidgetProfile(ctx, p.ID)
	if err != nil {
		return goals.WidgetProfile{}, err
	}
	if !ok {
		return goals.WidgetProfile{}, goalsStorageErr("create widget profile", errors.New("widget profile missing immediately after write"))
	}
	return created, nil
}

func (r *GoalsRepository) GetWidgetProfile(ctx context.Context, id string) (goals.WidgetProfile, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return goals.WidgetProfile{}, false, goalsStorageErr("begin get widget profile", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT `+widgetProfileColumns+` FROM widget_profiles WHERE id = ?`, id)
	p, err := scanWidgetProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return goals.WidgetProfile{}, false, nil
	}
	if err != nil {
		return goals.WidgetProfile{}, false, goalsStorageErr("get widget profile", err)
	}
	if err := r.loadWidgetProfileExtras(ctx, tx, &p); err != nil {
		return goals.WidgetProfile{}, false, goalsStorageErr("get widget profile extras", err)
	}
	return p, true, nil
}

func (r *GoalsRepository) GetWidgetProfileByPublicSlug(ctx context.Context, slug string) (goals.WidgetProfile, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return goals.WidgetProfile{}, false, goalsStorageErr("begin get widget profile by slug", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT `+widgetProfileColumns+` FROM widget_profiles WHERE public_slug = ?`, slug)
	p, err := scanWidgetProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return goals.WidgetProfile{}, false, nil
	}
	if err != nil {
		return goals.WidgetProfile{}, false, goalsStorageErr("get widget profile by slug", err)
	}
	if err := r.loadWidgetProfileExtras(ctx, tx, &p); err != nil {
		return goals.WidgetProfile{}, false, goalsStorageErr("get widget profile by slug extras", err)
	}
	return p, true, nil
}

func (r *GoalsRepository) ListWidgetProfiles(ctx context.Context, goalID string) ([]goals.WidgetProfile, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, goalsStorageErr("begin list widget profiles", err)
	}
	defer tx.Rollback()

	query := `SELECT ` + widgetProfileColumns + ` FROM widget_profiles`
	args := []any{}
	if goalID != "" {
		query += ` WHERE goal_id = ?`
		args = append(args, goalID)
	}
	query += ` ORDER BY created_at, id`

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, goalsStorageErr("list widget profiles", err)
	}
	var list []goals.WidgetProfile
	for rows.Next() {
		p, err := scanWidgetProfile(rows)
		if err != nil {
			rows.Close()
			return nil, goalsStorageErr("list widget profiles", err)
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, goalsStorageErr("list widget profiles", err)
	}
	rows.Close()

	for i := range list {
		if err := r.loadWidgetProfileExtras(ctx, tx, &list[i]); err != nil {
			return nil, goalsStorageErr("list widget profiles extras", err)
		}
	}
	return list, nil
}

func (r *GoalsRepository) UpdateWidgetProfile(ctx context.Context, p goals.WidgetProfile) (goals.WidgetProfile, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("begin update widget profile", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE widget_profiles SET name = ?, enabled = ?, title_override = ?,
			show_current = ?, show_target = ?, show_percent = ?, show_provider = ?, show_time = ?, show_message = ?,
			max_items = ?, currency = ?, metric = ?, columns = ?,
			orientation = ?, text_align = ?, font_family = ?,
			background_color = ?, foreground_color = ?, fill_color = ?, border_color = ?, border_radius_px = ?, opacity = ?,
			updated_at = ?
		WHERE id = ?`,
		p.Name, boolToInt(p.Enabled), p.TitleOverride,
		boolToInt(p.ShowCurrent), boolToInt(p.ShowTarget), boolToInt(p.ShowPercent),
		boolToInt(p.ShowProvider), boolToInt(p.ShowTime), boolToInt(p.ShowMessage),
		p.MaxItems, nullableString(p.Currency), nullableString(string(p.Metric)), p.Columns,
		string(p.Orientation), string(p.TextAlign), string(p.FontFamily),
		p.BackgroundColor, p.ForegroundColor, p.FillColor, p.BorderColor, p.BorderRadiusPx, p.Opacity,
		platform.FormatTimestamp(p.UpdatedAt), p.ID,
	)
	if err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("update widget profile", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("update widget profile", err)
	}
	if n == 0 {
		return goals.WidgetProfile{}, goals.ErrWidgetProfileNotFound
	}
	if err := writeWidgetProfileExtras(ctx, tx, p); err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("update widget profile extras", err)
	}
	if err := tx.Commit(); err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("commit update widget profile", err)
	}

	updated, ok, err := r.GetWidgetProfile(ctx, p.ID)
	if err != nil {
		return goals.WidgetProfile{}, err
	}
	if !ok {
		return goals.WidgetProfile{}, goals.ErrWidgetProfileNotFound
	}
	return updated, nil
}

func (r *GoalsRepository) RotatePublicSlug(ctx context.Context, id, newSlug string) (goals.WidgetProfile, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE widget_profiles SET public_slug = ?, updated_at = ? WHERE id = ?`,
		newSlug, platform.FormatTimestamp(time.Now().UTC()), id)
	if err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("rotate widget public slug", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return goals.WidgetProfile{}, goalsStorageErr("rotate widget public slug", err)
	}
	if n == 0 {
		return goals.WidgetProfile{}, goals.ErrWidgetProfileNotFound
	}
	updated, ok, err := r.GetWidgetProfile(ctx, id)
	if err != nil {
		return goals.WidgetProfile{}, err
	}
	if !ok {
		return goals.WidgetProfile{}, goals.ErrWidgetProfileNotFound
	}
	return updated, nil
}

func (r *GoalsRepository) DeleteWidgetProfile(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return goalsStorageErr("begin delete widget profile", err)
	}
	defer tx.Rollback()

	var refCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM widget_profile_dashboard_children WHERE child_id = ?`, id).Scan(&refCount); err != nil {
		return goalsStorageErr("check widget profile references", err)
	}
	if refCount > 0 {
		return goals.ErrWidgetProfileInUse
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM widget_profiles WHERE id = ?`, id)
	if err != nil {
		return goalsStorageErr("delete widget profile", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return goalsStorageErr("delete widget profile", err)
	}
	if n == 0 {
		return goals.ErrWidgetProfileNotFound
	}
	if err := tx.Commit(); err != nil {
		return goalsStorageErr("commit delete widget profile", err)
	}
	return nil
}
