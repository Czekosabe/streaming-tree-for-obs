package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// ChatOverlayRepository is the SQLite implementation of
// chatoverlay.Repository.
type ChatOverlayRepository struct {
	db *sql.DB
}

// NewChatOverlayRepository builds a repository over an open database.
func NewChatOverlayRepository(db *sql.DB) *ChatOverlayRepository {
	return &ChatOverlayRepository{db: db}
}

var _ chatoverlay.Repository = (*ChatOverlayRepository)(nil)

func chatOverlayStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", chatoverlay.ErrStorage, op, err)
}

const profileColumns = `id, public_slug, name, enabled,
	layout_mode, stack_direction, horizontal_alignment,
	show_platform_icon, show_platform_name, show_account_label, show_avatar, show_badges, show_timestamp,
	show_activity_events, show_deleted_placeholder, hide_commands, hide_bots,
	max_visible_items, message_lifetime_seconds,
	font_family, font_size, font_weight, line_height, text_color, username_color_mode,
	bubble_color, bubble_opacity, border_radius, item_spacing, text_outline, text_shadow,
	entry_animation, exit_animation, animation_duration_ms,
	highlight_broadcaster, highlight_moderators, highlight_subscribers, highlight_vips,
	language, created_at, updated_at`

func scanProfile(scanner interface{ Scan(...any) error }) (chatoverlay.Profile, error) {
	var (
		p                                                                               chatoverlay.Profile
		enabled, showIcon, showName, showAccount, showAvatar, showBadges, showTimestamp int
		showActivity, showDeleted, hideCommands, hideBots, textOutline, textShadow      int
		highlightBroadcaster, highlightMods, highlightSubs, highlightVIPs               int
		layoutMode, stackDirection, alignment, fontFamily, usernameColorMode            string
		entryAnimation, exitAnimation, language                                         string
		createdAt, updatedAt                                                            string
	)
	if err := scanner.Scan(
		&p.ID, &p.PublicSlug, &p.Name, &enabled,
		&layoutMode, &stackDirection, &alignment,
		&showIcon, &showName, &showAccount, &showAvatar, &showBadges, &showTimestamp,
		&showActivity, &showDeleted, &hideCommands, &hideBots,
		&p.MaxVisibleItems, &p.MessageLifetimeSeconds,
		&fontFamily, &p.FontSize, &p.FontWeight, &p.LineHeight, &p.TextColor, &usernameColorMode,
		&p.BubbleColor, &p.BubbleOpacity, &p.BorderRadius, &p.ItemSpacing, &textOutline, &textShadow,
		&entryAnimation, &exitAnimation, &p.AnimationDurationMS,
		&highlightBroadcaster, &highlightMods, &highlightSubs, &highlightVIPs,
		&language, &createdAt, &updatedAt,
	); err != nil {
		return chatoverlay.Profile{}, err
	}

	p.Enabled = enabled != 0
	p.LayoutMode = chatoverlay.LayoutMode(layoutMode)
	p.StackDirection = chatoverlay.StackDirection(stackDirection)
	p.HorizontalAlignment = chatoverlay.HorizontalAlignment(alignment)
	p.ShowPlatformIcon = showIcon != 0
	p.ShowPlatformName = showName != 0
	p.ShowAccountLabel = showAccount != 0
	p.ShowAvatar = showAvatar != 0
	p.ShowBadges = showBadges != 0
	p.ShowTimestamp = showTimestamp != 0
	p.ShowActivityEvents = showActivity != 0
	p.ShowDeletedPlaceholder = showDeleted != 0
	p.HideCommands = hideCommands != 0
	p.HideBots = hideBots != 0
	p.FontFamily = chatoverlay.FontFamily(fontFamily)
	p.UsernameColorMode = chatoverlay.UsernameColorMode(usernameColorMode)
	p.TextOutline = textOutline != 0
	p.TextShadow = textShadow != 0
	p.EntryAnimation = chatoverlay.Animation(entryAnimation)
	p.ExitAnimation = chatoverlay.Animation(exitAnimation)
	p.HighlightBroadcaster = highlightBroadcaster != 0
	p.HighlightModerators = highlightMods != 0
	p.HighlightSubscribers = highlightSubs != 0
	p.HighlightVIPs = highlightVIPs != 0
	p.Language = chatoverlay.Language(language)

	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return chatoverlay.Profile{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return chatoverlay.Profile{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

func profileWriteArgs(p chatoverlay.Profile) []any {
	return []any{
		boolToInt(p.Enabled),
		string(p.LayoutMode), string(p.StackDirection), string(p.HorizontalAlignment),
		boolToInt(p.ShowPlatformIcon), boolToInt(p.ShowPlatformName), boolToInt(p.ShowAccountLabel),
		boolToInt(p.ShowAvatar), boolToInt(p.ShowBadges), boolToInt(p.ShowTimestamp),
		boolToInt(p.ShowActivityEvents), boolToInt(p.ShowDeletedPlaceholder), boolToInt(p.HideCommands), boolToInt(p.HideBots),
		p.MaxVisibleItems, p.MessageLifetimeSeconds,
		string(p.FontFamily), p.FontSize, p.FontWeight, p.LineHeight, p.TextColor, string(p.UsernameColorMode),
		p.BubbleColor, p.BubbleOpacity, p.BorderRadius, p.ItemSpacing, boolToInt(p.TextOutline), boolToInt(p.TextShadow),
		string(p.EntryAnimation), string(p.ExitAnimation), p.AnimationDurationMS,
		boolToInt(p.HighlightBroadcaster), boolToInt(p.HighlightModerators), boolToInt(p.HighlightSubscribers), boolToInt(p.HighlightVIPs),
		string(p.Language),
	}
}

// CreateProfile inserts a new overlay profile.
func (r *ChatOverlayRepository) CreateProfile(ctx context.Context, p chatoverlay.Profile) (chatoverlay.Profile, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())
	args := append([]any{p.ID, p.PublicSlug, p.Name}, profileWriteArgs(p)...)
	args = append(args, nowText, nowText)

	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_overlays (
			id, public_slug, name,
			enabled, layout_mode, stack_direction, horizontal_alignment,
			show_platform_icon, show_platform_name, show_account_label, show_avatar, show_badges, show_timestamp,
			show_activity_events, show_deleted_placeholder, hide_commands, hide_bots,
			max_visible_items, message_lifetime_seconds,
			font_family, font_size, font_weight, line_height, text_color, username_color_mode,
			bubble_color, bubble_opacity, border_radius, item_spacing, text_outline, text_shadow,
			entry_animation, exit_animation, animation_duration_ms,
			highlight_broadcaster, highlight_moderators, highlight_subscribers, highlight_vips,
			language, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		args...,
	); err != nil {
		if isUniqueViolation(err) {
			return chatoverlay.Profile{}, chatOverlayStorageErr("create chat overlay", errors.New("public slug already in use"))
		}
		return chatoverlay.Profile{}, chatOverlayStorageErr("create chat overlay", err)
	}

	saved, found, err := r.GetProfile(ctx, p.ID)
	if err != nil {
		return chatoverlay.Profile{}, err
	}
	if !found {
		return chatoverlay.Profile{}, chatOverlayStorageErr("create chat overlay", errors.New("profile missing immediately after write"))
	}
	return saved, nil
}

// GetProfile returns one profile by its management id.
func (r *ChatOverlayRepository) GetProfile(ctx context.Context, id string) (chatoverlay.Profile, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM chat_overlays WHERE id = ?`, id)
	p, err := scanProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return chatoverlay.Profile{}, false, nil
		}
		return chatoverlay.Profile{}, false, chatOverlayStorageErr("get chat overlay", err)
	}
	return p, true, nil
}

// GetProfileByPublicSlug returns one profile by its current public slug.
func (r *ChatOverlayRepository) GetProfileByPublicSlug(ctx context.Context, slug string) (chatoverlay.Profile, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM chat_overlays WHERE public_slug = ?`, slug)
	p, err := scanProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return chatoverlay.Profile{}, false, nil
		}
		return chatoverlay.Profile{}, false, chatOverlayStorageErr("get chat overlay by public slug", err)
	}
	return p, true, nil
}

// ListProfiles returns every overlay profile, ordered by creation time for
// a stable, test-friendly order.
func (r *ChatOverlayRepository) ListProfiles(ctx context.Context) ([]chatoverlay.Profile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+profileColumns+` FROM chat_overlays ORDER BY created_at, id`)
	if err != nil {
		return nil, chatOverlayStorageErr("list chat overlays", err)
	}
	defer rows.Close()

	var out []chatoverlay.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, chatOverlayStorageErr("list chat overlays", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, chatOverlayStorageErr("list chat overlays", err)
	}
	return out, nil
}

// UpdateProfile replaces every editable field of one profile in full.
func (r *ChatOverlayRepository) UpdateProfile(ctx context.Context, p chatoverlay.Profile) (chatoverlay.Profile, error) {
	nowText := platform.FormatTimestamp(time.Now().UTC())
	args := append([]any{p.Name}, profileWriteArgs(p)...)
	args = append(args, nowText, p.ID)

	result, err := r.db.ExecContext(ctx, `
		UPDATE chat_overlays SET
			name = ?,
			enabled = ?, layout_mode = ?, stack_direction = ?, horizontal_alignment = ?,
			show_platform_icon = ?, show_platform_name = ?, show_account_label = ?, show_avatar = ?, show_badges = ?, show_timestamp = ?,
			show_activity_events = ?, show_deleted_placeholder = ?, hide_commands = ?, hide_bots = ?,
			max_visible_items = ?, message_lifetime_seconds = ?,
			font_family = ?, font_size = ?, font_weight = ?, line_height = ?, text_color = ?, username_color_mode = ?,
			bubble_color = ?, bubble_opacity = ?, border_radius = ?, item_spacing = ?, text_outline = ?, text_shadow = ?,
			entry_animation = ?, exit_animation = ?, animation_duration_ms = ?,
			highlight_broadcaster = ?, highlight_moderators = ?, highlight_subscribers = ?, highlight_vips = ?,
			language = ?, updated_at = ?
		WHERE id = ?`,
		args...,
	)
	if err != nil {
		return chatoverlay.Profile{}, chatOverlayStorageErr("update chat overlay", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return chatoverlay.Profile{}, chatOverlayStorageErr("update chat overlay", err)
	}
	if affected == 0 {
		return chatoverlay.Profile{}, chatoverlay.ErrNotFound
	}

	saved, found, err := r.GetProfile(ctx, p.ID)
	if err != nil {
		return chatoverlay.Profile{}, err
	}
	if !found {
		return chatoverlay.Profile{}, chatOverlayStorageErr("update chat overlay", errors.New("profile missing immediately after write"))
	}
	return saved, nil
}

// DeleteProfile removes a profile; every related row cascades.
func (r *ChatOverlayRepository) DeleteProfile(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM chat_overlays WHERE id = ?`, id); err != nil {
		return chatOverlayStorageErr("delete chat overlay", err)
	}
	return nil
}

// RotatePublicSlug replaces one profile's public slug.
func (r *ChatOverlayRepository) RotatePublicSlug(ctx context.Context, id, newSlug string, now time.Time) (chatoverlay.Profile, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE chat_overlays SET public_slug = ?, updated_at = ? WHERE id = ?`,
		newSlug, platform.FormatTimestamp(now), id,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return chatoverlay.Profile{}, chatOverlayStorageErr("rotate public slug", errors.New("generated slug collided, retry"))
		}
		return chatoverlay.Profile{}, chatOverlayStorageErr("rotate public slug", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return chatoverlay.Profile{}, chatOverlayStorageErr("rotate public slug", err)
	}
	if affected == 0 {
		return chatoverlay.Profile{}, chatoverlay.ErrNotFound
	}

	saved, found, err := r.GetProfile(ctx, id)
	if err != nil {
		return chatoverlay.Profile{}, err
	}
	if !found {
		return chatoverlay.Profile{}, chatOverlayStorageErr("rotate public slug", errors.New("profile missing immediately after write"))
	}
	return saved, nil
}

// --- accounts ---------------------------------------------------------

// ListAccounts returns the connected-account ids explicitly selected for
// one overlay, ordered for a stable, test-friendly result.
func (r *ChatOverlayRepository) ListAccounts(ctx context.Context, overlayID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT account_id FROM chat_overlay_accounts WHERE overlay_id = ? ORDER BY account_id`, overlayID)
	if err != nil {
		return nil, chatOverlayStorageErr("list chat overlay accounts", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			return nil, chatOverlayStorageErr("list chat overlay accounts", err)
		}
		out = append(out, accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, chatOverlayStorageErr("list chat overlay accounts", err)
	}
	return out, nil
}

// SetAccounts replaces the full selected-account set for one overlay
// inside one transaction, so a reader never observes a partially-replaced
// set.
func (r *ChatOverlayRepository) SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return chatOverlayStorageErr("set chat overlay accounts", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_overlay_accounts WHERE overlay_id = ?`, overlayID); err != nil {
		return chatOverlayStorageErr("set chat overlay accounts", err)
	}
	for _, accountID := range accountIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_overlay_accounts (overlay_id, account_id) VALUES (?, ?)`, overlayID, accountID,
		); err != nil {
			if isForeignKeyViolation(err) {
				return chatoverlay.ErrAccountNotFound
			}
			return chatOverlayStorageErr("set chat overlay accounts", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return chatOverlayStorageErr("set chat overlay accounts", err)
	}
	return nil
}

// --- hidden users -------------------------------------------------------

const hiddenUserColumns = `overlay_id, provider_id, connected_account_id, provider_user_id, label, created_at`

func scanHiddenUser(scanner interface{ Scan(...any) error }) (chatoverlay.HiddenUser, error) {
	var (
		ref        chatoverlay.HiddenUser
		providerID string
		label      sql.NullString
		createdAt  string
	)
	if err := scanner.Scan(&ref.OverlayID, &providerID, &ref.ConnectedAccountID, &ref.ProviderUserID, &label, &createdAt); err != nil {
		return chatoverlay.HiddenUser{}, err
	}
	ref.ProviderID = chatoverlay.ProviderID(providerID)
	ref.Label = label.String
	var err error
	if ref.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return chatoverlay.HiddenUser{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	return ref, nil
}

// ListHiddenUsers returns every user hidden from one overlay's public
// output.
func (r *ChatOverlayRepository) ListHiddenUsers(ctx context.Context, overlayID string) ([]chatoverlay.HiddenUser, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+hiddenUserColumns+` FROM chat_overlay_hidden_users WHERE overlay_id = ? ORDER BY created_at, provider_user_id`, overlayID)
	if err != nil {
		return nil, chatOverlayStorageErr("list chat overlay hidden users", err)
	}
	defer rows.Close()

	var out []chatoverlay.HiddenUser
	for rows.Next() {
		ref, err := scanHiddenUser(rows)
		if err != nil {
			return nil, chatOverlayStorageErr("list chat overlay hidden users", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, chatOverlayStorageErr("list chat overlay hidden users", err)
	}
	return out, nil
}

// AddHiddenUser adds one user to one overlay's hidden list, idempotently.
func (r *ChatOverlayRepository) AddHiddenUser(ctx context.Context, ref chatoverlay.HiddenUser, now time.Time) (chatoverlay.HiddenUser, error) {
	var label sql.NullString
	if ref.Label != "" {
		label = sql.NullString{String: ref.Label, Valid: true}
	}
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_overlay_hidden_users (overlay_id, provider_id, connected_account_id, provider_user_id, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (overlay_id, provider_id, connected_account_id, provider_user_id) DO NOTHING`,
		ref.OverlayID, string(ref.ProviderID), ref.ConnectedAccountID, ref.ProviderUserID, label, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return chatoverlay.HiddenUser{}, chatoverlay.ErrAccountNotFound
		}
		return chatoverlay.HiddenUser{}, chatOverlayStorageErr("add chat overlay hidden user", err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT `+hiddenUserColumns+` FROM chat_overlay_hidden_users
		 WHERE overlay_id = ? AND provider_id = ? AND connected_account_id = ? AND provider_user_id = ?`,
		ref.OverlayID, string(ref.ProviderID), ref.ConnectedAccountID, ref.ProviderUserID)
	saved, err := scanHiddenUser(row)
	if err != nil {
		return chatoverlay.HiddenUser{}, chatOverlayStorageErr("add chat overlay hidden user", err)
	}
	return saved, nil
}

// RemoveHiddenUser removes one hidden-user entry by its identity tuple.
func (r *ChatOverlayRepository) RemoveHiddenUser(ctx context.Context, overlayID string, providerID chatoverlay.ProviderID, connectedAccountID, providerUserID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM chat_overlay_hidden_users
		 WHERE overlay_id = ? AND provider_id = ? AND connected_account_id = ? AND provider_user_id = ?`,
		overlayID, string(providerID), connectedAccountID, providerUserID)
	if err != nil {
		return chatOverlayStorageErr("remove chat overlay hidden user", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return chatOverlayStorageErr("remove chat overlay hidden user", err)
	}
	if affected == 0 {
		return chatoverlay.ErrUserNotFound
	}
	return nil
}

// --- blocked terms -------------------------------------------------------

const blockedTermColumns = `id, overlay_id, value, match_mode, created_at, updated_at`

func scanBlockedTerm(scanner interface{ Scan(...any) error }) (chatoverlay.BlockedTerm, error) {
	var (
		t                    chatoverlay.BlockedTerm
		matchMode            string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&t.ID, &t.OverlayID, &t.Value, &matchMode, &createdAt, &updatedAt); err != nil {
		return chatoverlay.BlockedTerm{}, err
	}
	t.MatchMode = chatoverlay.MatchMode(matchMode)
	var err error
	if t.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return chatoverlay.BlockedTerm{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if t.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return chatoverlay.BlockedTerm{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return t, nil
}

// ListBlockedTerms returns every blocked term for one overlay.
func (r *ChatOverlayRepository) ListBlockedTerms(ctx context.Context, overlayID string) ([]chatoverlay.BlockedTerm, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+blockedTermColumns+` FROM chat_overlay_blocked_terms WHERE overlay_id = ? ORDER BY created_at, id`, overlayID)
	if err != nil {
		return nil, chatOverlayStorageErr("list chat overlay blocked terms", err)
	}
	defer rows.Close()

	var out []chatoverlay.BlockedTerm
	for rows.Next() {
		t, err := scanBlockedTerm(rows)
		if err != nil {
			return nil, chatOverlayStorageErr("list chat overlay blocked terms", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, chatOverlayStorageErr("list chat overlay blocked terms", err)
	}
	return out, nil
}

// AddBlockedTerm adds one term to one overlay, idempotently by its
// normalized value.
func (r *ChatOverlayRepository) AddBlockedTerm(ctx context.Context, term chatoverlay.BlockedTerm, now time.Time) (chatoverlay.BlockedTerm, error) {
	normalized := chatoverlay.NormalizeTerm(term.Value)
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_overlay_blocked_terms (id, overlay_id, value, normalized_value, match_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (overlay_id, normalized_value) DO NOTHING`,
		term.ID, term.OverlayID, term.Value, normalized, string(term.MatchMode), nowText, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return chatoverlay.BlockedTerm{}, chatoverlay.ErrNotFound
		}
		return chatoverlay.BlockedTerm{}, chatOverlayStorageErr("add chat overlay blocked term", err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT `+blockedTermColumns+` FROM chat_overlay_blocked_terms WHERE overlay_id = ? AND normalized_value = ?`,
		term.OverlayID, normalized)
	saved, err := scanBlockedTerm(row)
	if err != nil {
		return chatoverlay.BlockedTerm{}, chatOverlayStorageErr("add chat overlay blocked term", err)
	}
	return saved, nil
}

// RemoveBlockedTerm removes one blocked-term entry by its own id.
func (r *ChatOverlayRepository) RemoveBlockedTerm(ctx context.Context, overlayID, id string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM chat_overlay_blocked_terms WHERE overlay_id = ? AND id = ?`, overlayID, id)
	if err != nil {
		return chatOverlayStorageErr("remove chat overlay blocked term", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return chatOverlayStorageErr("remove chat overlay blocked term", err)
	}
	if affected == 0 {
		return chatoverlay.ErrTermNotFound
	}
	return nil
}

// --- activity types -------------------------------------------------------

// ListActivityTypes returns the activity types explicitly selected for
// one overlay.
func (r *ChatOverlayRepository) ListActivityTypes(ctx context.Context, overlayID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT activity_type FROM chat_overlay_activity_types WHERE overlay_id = ? ORDER BY activity_type`, overlayID)
	if err != nil {
		return nil, chatOverlayStorageErr("list chat overlay activity types", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var activityType string
		if err := rows.Scan(&activityType); err != nil {
			return nil, chatOverlayStorageErr("list chat overlay activity types", err)
		}
		out = append(out, activityType)
	}
	if err := rows.Err(); err != nil {
		return nil, chatOverlayStorageErr("list chat overlay activity types", err)
	}
	return out, nil
}

// SetActivityTypes replaces the full activity-type selection for one
// overlay inside one transaction.
func (r *ChatOverlayRepository) SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return chatOverlayStorageErr("set chat overlay activity types", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_overlay_activity_types WHERE overlay_id = ?`, overlayID); err != nil {
		return chatOverlayStorageErr("set chat overlay activity types", err)
	}
	for _, activityType := range activityTypes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_overlay_activity_types (overlay_id, activity_type) VALUES (?, ?)`, overlayID, activityType,
		); err != nil {
			if isForeignKeyViolation(err) {
				return chatoverlay.ErrNotFound
			}
			return chatOverlayStorageErr("set chat overlay activity types", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return chatOverlayStorageErr("set chat overlay activity types", err)
	}
	return nil
}
