package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// AccountRepository is the SQLite implementation of account.Repository.
//
// Nothing in this file ever touches a network or a provider API - see
// internal/provider/twitch for that boundary - and no column here is a
// token, a device code, or a client secret; the complete OAuth token bundle
// lives only in the OS credential store, addressed by account ID.
type AccountRepository struct {
	db *sql.DB
}

// NewAccountRepository builds a repository over an open database.
func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

var _ account.Repository = (*AccountRepository)(nil)

func accountStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", account.ErrStorage, op, err)
}

const accountColumns = `id, provider_id, provider_user_id, login, display_name, avatar_url, status, last_validated_at, created_at, updated_at`

func scanAccount(scanner interface{ Scan(...any) error }) (account.Account, error) {
	var (
		a               account.Account
		providerID      string
		status          string
		avatarURL       sql.NullString
		lastValidatedAt sql.NullString
		createdAt       string
		updatedAt       string
	)

	if err := scanner.Scan(
		&a.ID, &providerID, &a.ProviderUserID, &a.Login, &a.DisplayName, &avatarURL,
		&status, &lastValidatedAt, &createdAt, &updatedAt,
	); err != nil {
		return account.Account{}, err
	}

	a.ProviderID = account.ProviderID(providerID)
	a.Status = account.Status(status)
	a.AvatarURL = avatarURL.String

	var err error
	if a.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return account.Account{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if a.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return account.Account{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	if lastValidatedAt.Valid {
		parsed, err := platform.ParseTimestamp(lastValidatedAt.String)
		if err != nil {
			return account.Account{}, fmt.Errorf("parse last_validated_at %q: %w", lastValidatedAt.String, err)
		}
		a.LastValidatedAt = &parsed
	}

	return a, nil
}

func (r *AccountRepository) loadScopes(ctx context.Context, ex execer, accountID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT scope FROM connected_account_scopes WHERE account_id = ? ORDER BY scope`, accountID)
	if err != nil {
		return nil, accountStorageErr("load scopes", err)
	}
	defer rows.Close()

	scopes := []string{}
	for rows.Next() {
		var scope string
		if err := rows.Scan(&scope); err != nil {
			return nil, accountStorageErr("scan scope", err)
		}
		scopes = append(scopes, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, accountStorageErr("iterate scopes", err)
	}
	return scopes, nil
}

// GetAccount returns one account with its scopes.
func (r *AccountRepository) GetAccount(ctx context.Context, id string) (account.Account, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM connected_accounts WHERE id = ?`, id)
	acc, err := scanAccount(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.Account{}, account.ErrNotFound
		}
		return account.Account{}, accountStorageErr("get account", err)
	}
	scopes, err := r.loadScopes(ctx, r.db, id)
	if err != nil {
		return account.Account{}, err
	}
	acc.Scopes = scopes
	return acc, nil
}

// FindByProviderIdentity looks up an account by its real provider identity.
func (r *AccountRepository) FindByProviderIdentity(ctx context.Context, providerID account.ProviderID, providerUserID string) (account.Account, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM connected_accounts WHERE provider_id = ? AND provider_user_id = ?`,
		string(providerID), providerUserID)
	acc, err := scanAccount(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.Account{}, false, nil
		}
		return account.Account{}, false, accountStorageErr("find account by identity", err)
	}
	scopes, err := r.loadScopes(ctx, r.db, acc.ID)
	if err != nil {
		return account.Account{}, false, err
	}
	acc.Scopes = scopes
	return acc, true, nil
}

func lastValidatedAtValue(a account.Account) any {
	if a.LastValidatedAt == nil {
		return nil
	}
	return platform.FormatTimestamp(*a.LastValidatedAt)
}

func avatarURLValue(a account.Account) any {
	if a.AvatarURL == "" {
		return nil
	}
	return a.AvatarURL
}

func (r *AccountRepository) replaceScopes(ctx context.Context, tx *sql.Tx, accountID string, scopes []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM connected_account_scopes WHERE account_id = ?`, accountID); err != nil {
		return accountStorageErr("clear scopes", err)
	}
	for _, scope := range scopes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO connected_account_scopes (account_id, scope) VALUES (?, ?)`, accountID, scope,
		); err != nil {
			return accountStorageErr("insert scope", err)
		}
	}
	return nil
}

// CreateAccount inserts a new account and its scopes in one transaction.
func (r *AccountRepository) CreateAccount(ctx context.Context, acc account.Account) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return accountStorageErr("begin create account", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO connected_accounts
            (id, provider_id, provider_user_id, login, display_name, avatar_url, status, last_validated_at, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		acc.ID, string(acc.ProviderID), acc.ProviderUserID, acc.Login, acc.DisplayName, avatarURLValue(acc),
		string(acc.Status), lastValidatedAtValue(acc), platform.FormatTimestamp(acc.CreatedAt), platform.FormatTimestamp(acc.UpdatedAt),
	); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: an account for this provider identity already exists", account.ErrConflict)
		}
		return accountStorageErr("insert account", err)
	}

	if err := r.replaceScopes(ctx, tx, acc.ID, acc.Scopes); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return accountStorageErr("commit create account", err)
	}
	return nil
}

// UpdateAccount replaces an account's mutable fields and scopes.
func (r *AccountRepository) UpdateAccount(ctx context.Context, acc account.Account) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return accountStorageErr("begin update account", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
        UPDATE connected_accounts
        SET login = ?, display_name = ?, avatar_url = ?, status = ?, last_validated_at = ?, updated_at = ?
        WHERE id = ?`,
		acc.Login, acc.DisplayName, avatarURLValue(acc), string(acc.Status), lastValidatedAtValue(acc),
		platform.FormatTimestamp(acc.UpdatedAt), acc.ID,
	)
	if err != nil {
		return accountStorageErr("update account", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return accountStorageErr("update account rows", err)
	}
	if affected == 0 {
		return account.ErrNotFound
	}

	if err := r.replaceScopes(ctx, tx, acc.ID, acc.Scopes); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return accountStorageErr("commit update account", err)
	}
	return nil
}

// DeleteAccount removes the account row; platform_account_links cascade.
func (r *AccountRepository) DeleteAccount(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM connected_accounts WHERE id = ?`, id)
	if err != nil {
		return accountStorageErr("delete account", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return accountStorageErr("delete account rows", err)
	}
	if affected == 0 {
		return account.ErrNotFound
	}
	return nil
}

// ListAccounts returns every connected account, ordered by creation time.
func (r *AccountRepository) ListAccounts(ctx context.Context) ([]account.Account, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+accountColumns+` FROM connected_accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, accountStorageErr("list accounts", err)
	}
	defer rows.Close()

	accounts := []account.Account{}
	for rows.Next() {
		acc, err := scanAccount(rows)
		if err != nil {
			return nil, accountStorageErr("scan account", err)
		}
		accounts = append(accounts, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, accountStorageErr("iterate accounts", err)
	}

	for i := range accounts {
		scopes, err := r.loadScopes(ctx, r.db, accounts[i].ID)
		if err != nil {
			return nil, err
		}
		accounts[i].Scopes = scopes
	}
	return accounts, nil
}

// CountAccounts reports how many connected accounts exist for a provider.
func (r *AccountRepository) CountAccounts(ctx context.Context, providerID account.ProviderID) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM connected_accounts WHERE provider_id = ?`, string(providerID),
	).Scan(&count); err != nil {
		return 0, accountStorageErr("count accounts", err)
	}
	return count, nil
}

// GetLink returns the account linked to a platform, if any.
func (r *AccountRepository) GetLink(ctx context.Context, platformID string) (account.Link, bool, error) {
	var (
		link      account.Link
		createdAt string
		updatedAt string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT platform_id, account_id, created_at, updated_at FROM platform_account_links WHERE platform_id = ?`,
		platformID,
	).Scan(&link.PlatformID, &link.AccountID, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.Link{}, false, nil
		}
		return account.Link{}, false, accountStorageErr("get link", err)
	}

	var parseErr error
	if link.CreatedAt, parseErr = platform.ParseTimestamp(createdAt); parseErr != nil {
		return account.Link{}, false, fmt.Errorf("parse created_at %q: %w", createdAt, parseErr)
	}
	if link.UpdatedAt, parseErr = platform.ParseTimestamp(updatedAt); parseErr != nil {
		return account.Link{}, false, fmt.Errorf("parse updated_at %q: %w", updatedAt, parseErr)
	}
	return link, true, nil
}

// SetLink creates or replaces a platform's link. "INSERT ... ON CONFLICT" is
// used rather than a delete-then-insert, so a replace is one statement, not
// two, and there is no window where the platform briefly has no link.
func (r *AccountRepository) SetLink(ctx context.Context, platformID, accountID string, now time.Time) (account.Link, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO platform_account_links (platform_id, account_id, created_at, updated_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT (platform_id) DO UPDATE SET account_id = excluded.account_id, updated_at = excluded.updated_at`,
		platformID, accountID, nowText, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return account.Link{}, fmt.Errorf("%w: platform or account does not exist", account.ErrConflict)
		}
		return account.Link{}, accountStorageErr("set link", err)
	}

	link, found, err := r.GetLink(ctx, platformID)
	if err != nil {
		return account.Link{}, err
	}
	if !found {
		return account.Link{}, accountStorageErr("set link", errors.New("link missing immediately after write"))
	}
	return link, nil
}

// DeleteLink removes a platform's link without touching the platform or the
// account. Deleting an absent link is not an error.
func (r *AccountRepository) DeleteLink(ctx context.Context, platformID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM platform_account_links WHERE platform_id = ?`, platformID); err != nil {
		return accountStorageErr("delete link", err)
	}
	return nil
}

// GetIntegrationSettings returns the database-managed Client ID for a
// provider, if one was ever saved.
func (r *AccountRepository) GetIntegrationSettings(ctx context.Context, providerID account.ProviderID) (account.IntegrationSettings, bool, error) {
	var (
		settings  account.IntegrationSettings
		updatedAt string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT client_id, updated_at FROM provider_integration_settings WHERE provider_id = ?`, string(providerID),
	).Scan(&settings.ClientID, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.IntegrationSettings{}, false, nil
		}
		return account.IntegrationSettings{}, false, accountStorageErr("get integration settings", err)
	}

	settings.ProviderID = providerID
	parsed, parseErr := platform.ParseTimestamp(updatedAt)
	if parseErr != nil {
		return account.IntegrationSettings{}, false, fmt.Errorf("parse updated_at %q: %w", updatedAt, parseErr)
	}
	settings.UpdatedAt = parsed
	return settings, true, nil
}

// SetIntegrationSettings creates or replaces the database-managed Client ID
// for a provider.
func (r *AccountRepository) SetIntegrationSettings(ctx context.Context, providerID account.ProviderID, clientID string, now time.Time) (account.IntegrationSettings, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO provider_integration_settings (provider_id, client_id, created_at, updated_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT (provider_id) DO UPDATE SET client_id = excluded.client_id, updated_at = excluded.updated_at`,
		string(providerID), clientID, nowText, nowText,
	); err != nil {
		return account.IntegrationSettings{}, accountStorageErr("set integration settings", err)
	}
	return account.IntegrationSettings{ProviderID: providerID, ClientID: clientID, UpdatedAt: now}, nil
}
