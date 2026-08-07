package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// ChatAutomationRepository is the SQLite implementation of
// chatautomation.Repository.
type ChatAutomationRepository struct {
	db *sql.DB
}

// NewChatAutomationRepository builds a repository over an open database.
func NewChatAutomationRepository(db *sql.DB) *ChatAutomationRepository {
	return &ChatAutomationRepository{db: db}
}

var _ chatautomation.Repository = (*ChatAutomationRepository)(nil)

func chatAutomationStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", chatautomation.ErrStorage, op, err)
}

const scheduleColumns = `id, name, enabled, interval_seconds, first_delay_seconds, jitter_seconds,
	only_while_ingest_receiving, minimum_chat_messages, maximum_sends_per_hour, created_at, updated_at`

func scanSchedule(scanner interface{ Scan(...any) error }) (chatautomation.Schedule, error) {
	var (
		s                      chatautomation.Schedule
		enabled, onlyReceiving int
		createdAt, updatedAt   string
	)
	if err := scanner.Scan(
		&s.ID, &s.Name, &enabled, &s.IntervalSeconds, &s.FirstDelaySeconds, &s.JitterSeconds,
		&onlyReceiving, &s.MinimumChatMessages, &s.MaximumSendsPerHour, &createdAt, &updatedAt,
	); err != nil {
		return chatautomation.Schedule{}, err
	}
	s.Enabled = enabled != 0
	s.OnlyWhileIngestReceiving = onlyReceiving != 0
	var err error
	if s.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return chatautomation.Schedule{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if s.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return chatautomation.Schedule{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return s, nil
}

func (r *ChatAutomationRepository) loadScheduleTargets(ctx context.Context, id string) ([]chatautomation.Target, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT account_id, COALESCE(platform_id, '') FROM chat_schedule_targets WHERE schedule_id = ? ORDER BY account_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatautomation.Target
	for rows.Next() {
		var t chatautomation.Target
		if err := rows.Scan(&t.AccountID, &t.PlatformID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *ChatAutomationRepository) loadScheduleMessages(ctx context.Context, id string) ([]chatautomation.ScheduleMessage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, message_template, position, created_at, updated_at FROM chat_schedule_messages WHERE schedule_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatautomation.ScheduleMessage
	for rows.Next() {
		var m chatautomation.ScheduleMessage
		var createdAt, updatedAt string
		if err := rows.Scan(&m.ID, &m.MessageTemplate, &m.Position, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if m.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
			return nil, err
		}
		if m.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ChatAutomationRepository) writeScheduleChildren(ctx context.Context, tx *sql.Tx, s chatautomation.Schedule, nowText string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_schedule_targets WHERE schedule_id = ?`, s.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_schedule_messages WHERE schedule_id = ?`, s.ID); err != nil {
		return err
	}
	for _, t := range s.Targets {
		var platformID any
		if t.PlatformID != "" {
			platformID = t.PlatformID
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_schedule_targets (schedule_id, account_id, platform_id) VALUES (?, ?, ?)`,
			s.ID, t.AccountID, platformID,
		); err != nil {
			return err
		}
	}
	for _, m := range s.Messages {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_schedule_messages (id, schedule_id, message_template, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			m.ID, s.ID, m.MessageTemplate, m.Position, nowText, nowText,
		); err != nil {
			return err
		}
	}
	return nil
}

// CreateSchedule inserts a new schedule, its targets, and its messages
// in one transaction.
func (r *ChatAutomationRepository) CreateSchedule(ctx context.Context, s chatautomation.Schedule) (chatautomation.Schedule, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("create chat schedule", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO chat_schedules (
			id, name, enabled, interval_seconds, first_delay_seconds, jitter_seconds,
			only_while_ingest_receiving, minimum_chat_messages, maximum_sends_per_hour, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, boolToInt(s.Enabled), s.IntervalSeconds, s.FirstDelaySeconds, s.JitterSeconds,
		boolToInt(s.OnlyWhileIngestReceiving), s.MinimumChatMessages, s.MaximumSendsPerHour, nowText, nowText,
	); err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("create chat schedule", err)
	}
	if err := r.writeScheduleChildren(ctx, tx, s, nowText); err != nil {
		if isForeignKeyViolation(err) {
			return chatautomation.Schedule{}, chatautomation.ErrAccountNotFound
		}
		return chatautomation.Schedule{}, chatAutomationStorageErr("create chat schedule", err)
	}
	if err := tx.Commit(); err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("create chat schedule", err)
	}

	saved, found, err := r.GetSchedule(ctx, s.ID)
	if err != nil {
		return chatautomation.Schedule{}, err
	}
	if !found {
		return chatautomation.Schedule{}, chatAutomationStorageErr("create chat schedule", errors.New("schedule missing immediately after write"))
	}
	return saved, nil
}

// GetSchedule returns one schedule, its targets, and its messages.
func (r *ChatAutomationRepository) GetSchedule(ctx context.Context, id string) (chatautomation.Schedule, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM chat_schedules WHERE id = ?`, id)
	s, err := scanSchedule(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return chatautomation.Schedule{}, false, nil
		}
		return chatautomation.Schedule{}, false, chatAutomationStorageErr("get chat schedule", err)
	}
	if s.Targets, err = r.loadScheduleTargets(ctx, id); err != nil {
		return chatautomation.Schedule{}, false, chatAutomationStorageErr("get chat schedule targets", err)
	}
	if s.Messages, err = r.loadScheduleMessages(ctx, id); err != nil {
		return chatautomation.Schedule{}, false, chatAutomationStorageErr("get chat schedule messages", err)
	}
	return s, true, nil
}

// ListSchedules returns every schedule, ordered by creation time.
func (r *ChatAutomationRepository) ListSchedules(ctx context.Context) ([]chatautomation.Schedule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+scheduleColumns+` FROM chat_schedules ORDER BY created_at, id`)
	if err != nil {
		return nil, chatAutomationStorageErr("list chat schedules", err)
	}
	defer rows.Close()

	var ids []string
	var out []chatautomation.Schedule
	for rows.Next() {
		s, err := scanSchedule(rows)
		if err != nil {
			return nil, chatAutomationStorageErr("list chat schedules", err)
		}
		out = append(out, s)
		ids = append(ids, s.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, chatAutomationStorageErr("list chat schedules", err)
	}
	for i, id := range ids {
		if out[i].Targets, err = r.loadScheduleTargets(ctx, id); err != nil {
			return nil, chatAutomationStorageErr("list chat schedule targets", err)
		}
		if out[i].Messages, err = r.loadScheduleMessages(ctx, id); err != nil {
			return nil, chatAutomationStorageErr("list chat schedule messages", err)
		}
	}
	return out, nil
}

// UpdateSchedule replaces every editable field, target, and message of
// one schedule in one transaction.
func (r *ChatAutomationRepository) UpdateSchedule(ctx context.Context, s chatautomation.Schedule) (chatautomation.Schedule, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("update chat schedule", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE chat_schedules SET
			name = ?, enabled = ?, interval_seconds = ?, first_delay_seconds = ?, jitter_seconds = ?,
			only_while_ingest_receiving = ?, minimum_chat_messages = ?, maximum_sends_per_hour = ?, updated_at = ?
		WHERE id = ?`,
		s.Name, boolToInt(s.Enabled), s.IntervalSeconds, s.FirstDelaySeconds, s.JitterSeconds,
		boolToInt(s.OnlyWhileIngestReceiving), s.MinimumChatMessages, s.MaximumSendsPerHour, nowText, s.ID,
	)
	if err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("update chat schedule", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("update chat schedule", err)
	}
	if affected == 0 {
		return chatautomation.Schedule{}, chatautomation.ErrScheduleNotFound
	}
	if err := r.writeScheduleChildren(ctx, tx, s, nowText); err != nil {
		if isForeignKeyViolation(err) {
			return chatautomation.Schedule{}, chatautomation.ErrAccountNotFound
		}
		return chatautomation.Schedule{}, chatAutomationStorageErr("update chat schedule", err)
	}
	if err := tx.Commit(); err != nil {
		return chatautomation.Schedule{}, chatAutomationStorageErr("update chat schedule", err)
	}

	saved, found, err := r.GetSchedule(ctx, s.ID)
	if err != nil {
		return chatautomation.Schedule{}, err
	}
	if !found {
		return chatautomation.Schedule{}, chatAutomationStorageErr("update chat schedule", errors.New("schedule missing immediately after write"))
	}
	return saved, nil
}

// DeleteSchedule removes a schedule; its targets and messages cascade.
func (r *ChatAutomationRepository) DeleteSchedule(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM chat_schedules WHERE id = ?`, id); err != nil {
		return chatAutomationStorageErr("delete chat schedule", err)
	}
	return nil
}

// --- commands -------------------------------------------------------------

const commandColumns = `id, name, enabled, response_template, required_role,
	global_cooldown_seconds, user_cooldown_seconds, created_at, updated_at`

func scanCommand(scanner interface{ Scan(...any) error }) (chatautomation.Command, error) {
	var (
		c                    chatautomation.Command
		enabled              int
		requiredRole         string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&c.ID, &c.Name, &enabled, &c.ResponseTemplate, &requiredRole,
		&c.GlobalCooldownSeconds, &c.UserCooldownSeconds, &createdAt, &updatedAt,
	); err != nil {
		return chatautomation.Command{}, err
	}
	c.Enabled = enabled != 0
	c.RequiredRole = chatautomation.Role(requiredRole)
	var err error
	if c.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return chatautomation.Command{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if c.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return chatautomation.Command{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return c, nil
}

func (r *ChatAutomationRepository) loadCommandAliases(ctx context.Context, id string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT alias FROM chat_command_aliases WHERE command_id = ? ORDER BY alias`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		out = append(out, alias)
	}
	return out, rows.Err()
}

func (r *ChatAutomationRepository) loadCommandTargets(ctx context.Context, id string) ([]chatautomation.Target, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT account_id, COALESCE(platform_id, '') FROM chat_command_targets WHERE command_id = ? ORDER BY account_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatautomation.Target
	for rows.Next() {
		var t chatautomation.Target
		if err := rows.Scan(&t.AccountID, &t.PlatformID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *ChatAutomationRepository) writeCommandChildren(ctx context.Context, tx *sql.Tx, c chatautomation.Command) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_command_aliases WHERE command_id = ?`, c.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_command_targets WHERE command_id = ?`, c.ID); err != nil {
		return err
	}
	for _, alias := range c.Aliases {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_command_aliases (command_id, alias) VALUES (?, ?)`, c.ID, alias,
		); err != nil {
			return err
		}
	}
	for _, t := range c.Targets {
		var platformID any
		if t.PlatformID != "" {
			platformID = t.PlatformID
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_command_targets (command_id, account_id, platform_id) VALUES (?, ?, ?)`,
			c.ID, t.AccountID, platformID,
		); err != nil {
			return err
		}
	}
	return nil
}

// CreateCommand inserts a new command, its aliases, and its targets in
// one transaction.
func (r *ChatAutomationRepository) CreateCommand(ctx context.Context, c chatautomation.Command) (chatautomation.Command, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return chatautomation.Command{}, chatAutomationStorageErr("create chat command", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO chat_commands (
			id, name, enabled, response_template, required_role,
			global_cooldown_seconds, user_cooldown_seconds, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, boolToInt(c.Enabled), c.ResponseTemplate, string(c.RequiredRole),
		c.GlobalCooldownSeconds, c.UserCooldownSeconds, nowText, nowText,
	); err != nil {
		if isUniqueViolation(err) {
			return chatautomation.Command{}, chatautomation.ErrCommandNameConflict
		}
		return chatautomation.Command{}, chatAutomationStorageErr("create chat command", err)
	}
	if err := r.writeCommandChildren(ctx, tx, c); err != nil {
		if isUniqueViolation(err) {
			return chatautomation.Command{}, chatautomation.ErrCommandNameConflict
		}
		if isForeignKeyViolation(err) {
			return chatautomation.Command{}, chatautomation.ErrAccountNotFound
		}
		return chatautomation.Command{}, chatAutomationStorageErr("create chat command", err)
	}
	if err := tx.Commit(); err != nil {
		return chatautomation.Command{}, chatAutomationStorageErr("create chat command", err)
	}

	saved, found, err := r.GetCommand(ctx, c.ID)
	if err != nil {
		return chatautomation.Command{}, err
	}
	if !found {
		return chatautomation.Command{}, chatAutomationStorageErr("create chat command", errors.New("command missing immediately after write"))
	}
	return saved, nil
}

// GetCommand returns one command, its aliases, and its targets.
func (r *ChatAutomationRepository) GetCommand(ctx context.Context, id string) (chatautomation.Command, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+commandColumns+` FROM chat_commands WHERE id = ?`, id)
	c, err := scanCommand(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return chatautomation.Command{}, false, nil
		}
		return chatautomation.Command{}, false, chatAutomationStorageErr("get chat command", err)
	}
	if c.Aliases, err = r.loadCommandAliases(ctx, id); err != nil {
		return chatautomation.Command{}, false, chatAutomationStorageErr("get chat command aliases", err)
	}
	if c.Targets, err = r.loadCommandTargets(ctx, id); err != nil {
		return chatautomation.Command{}, false, chatAutomationStorageErr("get chat command targets", err)
	}
	return c, true, nil
}

// ListCommands returns every command, ordered by creation time.
func (r *ChatAutomationRepository) ListCommands(ctx context.Context) ([]chatautomation.Command, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+commandColumns+` FROM chat_commands ORDER BY created_at, id`)
	if err != nil {
		return nil, chatAutomationStorageErr("list chat commands", err)
	}
	defer rows.Close()

	var ids []string
	var out []chatautomation.Command
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, chatAutomationStorageErr("list chat commands", err)
		}
		out = append(out, c)
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, chatAutomationStorageErr("list chat commands", err)
	}
	for i, id := range ids {
		if out[i].Aliases, err = r.loadCommandAliases(ctx, id); err != nil {
			return nil, chatAutomationStorageErr("list chat command aliases", err)
		}
		if out[i].Targets, err = r.loadCommandTargets(ctx, id); err != nil {
			return nil, chatAutomationStorageErr("list chat command targets", err)
		}
	}
	return out, nil
}

// UpdateCommand replaces every editable field, alias, and target of one
// command in one transaction.
func (r *ChatAutomationRepository) UpdateCommand(ctx context.Context, c chatautomation.Command) (chatautomation.Command, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return chatautomation.Command{}, chatAutomationStorageErr("update chat command", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE chat_commands SET
			name = ?, enabled = ?, response_template = ?, required_role = ?,
			global_cooldown_seconds = ?, user_cooldown_seconds = ?, updated_at = ?
		WHERE id = ?`,
		c.Name, boolToInt(c.Enabled), c.ResponseTemplate, string(c.RequiredRole),
		c.GlobalCooldownSeconds, c.UserCooldownSeconds, nowText, c.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return chatautomation.Command{}, chatautomation.ErrCommandNameConflict
		}
		return chatautomation.Command{}, chatAutomationStorageErr("update chat command", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return chatautomation.Command{}, chatAutomationStorageErr("update chat command", err)
	}
	if affected == 0 {
		return chatautomation.Command{}, chatautomation.ErrCommandNotFound
	}
	if err := r.writeCommandChildren(ctx, tx, c); err != nil {
		if isUniqueViolation(err) {
			return chatautomation.Command{}, chatautomation.ErrCommandNameConflict
		}
		if isForeignKeyViolation(err) {
			return chatautomation.Command{}, chatautomation.ErrAccountNotFound
		}
		return chatautomation.Command{}, chatAutomationStorageErr("update chat command", err)
	}
	if err := tx.Commit(); err != nil {
		return chatautomation.Command{}, chatAutomationStorageErr("update chat command", err)
	}

	saved, found, err := r.GetCommand(ctx, c.ID)
	if err != nil {
		return chatautomation.Command{}, err
	}
	if !found {
		return chatautomation.Command{}, chatAutomationStorageErr("update chat command", errors.New("command missing immediately after write"))
	}
	return saved, nil
}

// DeleteCommand removes a command; its aliases and targets cascade.
func (r *ChatAutomationRepository) DeleteCommand(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM chat_commands WHERE id = ?`, id); err != nil {
		return chatAutomationStorageErr("delete chat command", err)
	}
	return nil
}

// NameOrAliasInUse reports whether name already names a different
// command's canonical name or any command's alias.
func (r *ChatAutomationRepository) NameOrAliasInUse(ctx context.Context, name, excludeCommandID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chat_commands WHERE name = ? AND id != ?
			UNION ALL
			SELECT 1 FROM chat_command_aliases WHERE alias = ? AND command_id != ?
		)`,
		name, excludeCommandID, name, excludeCommandID,
	).Scan(&exists)
	if err != nil {
		return false, chatAutomationStorageErr("check chat command name/alias uniqueness", err)
	}
	return exists != 0, nil
}
