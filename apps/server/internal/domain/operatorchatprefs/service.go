package operatorchatprefs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// IDGenerator produces identifiers for new hidden/bot user entries.
type IDGenerator func() (string, error)

// NewID returns a random, non-sequential entry identifier, mirroring
// account.NewID's own reasoning.
func NewID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate operator chat user entry id: %w", err)
	}
	return "ocu_" + hex.EncodeToString(buf), nil
}

// Service holds the operator-chat-preferences use cases.
type Service struct {
	repo  Repository
	now   Clock
	newID IDGenerator
}

// NewService builds a Service.
func NewService(repo Repository, now Clock, newID IDGenerator) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = NewID
	}
	return &Service{repo: repo, now: now, newID: newID}
}

// mapRepoErr passes known domain sentinels (ErrAccountNotFound,
// ErrUserNotFound) through unchanged, so callers can still errors.Is match
// them - only a truly unexpected persistence failure is wrapped as
// ErrStorage.
func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrUserNotFound) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

// Preferences returns the current preferences, or the documented defaults
// if none have ever been saved.
func (s *Service) Preferences(ctx context.Context) (Preferences, error) {
	p, found, err := s.repo.GetPreferences(ctx)
	if err != nil {
		return Preferences{}, mapRepoErr(err)
	}
	if !found {
		return Default(), nil
	}
	return p, nil
}

// ReplacePreferences writes a full replacement of the preferences row.
func (s *Service) ReplacePreferences(ctx context.Context, p Preferences) (Preferences, error) {
	saved, err := s.repo.SetPreferences(ctx, p, s.now())
	if err != nil {
		return Preferences{}, mapRepoErr(err)
	}
	return saved, nil
}

// AccountVisibility returns every account with an explicit visibility
// preference recorded. An account absent from this list is visible by
// default - callers with the full connected-account list combine the two.
func (s *Service) AccountVisibility(ctx context.Context) ([]AccountVisibility, error) {
	list, err := s.repo.ListAccountVisibility(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// SetAccountVisibility creates or replaces one account's visibility
// preference. Returns ErrAccountNotFound if accountID does not reference an
// existing connected account.
func (s *Service) SetAccountVisibility(ctx context.Context, accountID string, visible bool) (AccountVisibility, error) {
	saved, err := s.repo.SetAccountVisibility(ctx, accountID, visible, s.now())
	if err != nil {
		return AccountVisibility{}, mapRepoErr(err)
	}
	return saved, nil
}

// HiddenUsers returns every operator-hidden user.
func (s *Service) HiddenUsers(ctx context.Context) ([]UserRef, error) {
	list, err := s.repo.ListHiddenUsers(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// HideUser adds one user to the hidden list, idempotently.
func (s *Service) HideUser(ctx context.Context, providerID ProviderID, connectedAccountID, providerUserID, label string) (UserRef, error) {
	id, err := s.newID()
	if err != nil {
		return UserRef{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	saved, err := s.repo.AddHiddenUser(ctx, UserRef{
		ID: id, ProviderID: providerID, ConnectedAccountID: connectedAccountID,
		ProviderUserID: providerUserID, Label: label,
	}, s.now())
	if err != nil {
		return UserRef{}, mapRepoErr(err)
	}
	return saved, nil
}

// UnhideUser removes one hidden-user entry by its own id.
func (s *Service) UnhideUser(ctx context.Context, id string) error {
	if err := s.repo.RemoveHiddenUser(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// BotUsers returns every operator-marked bot user.
func (s *Service) BotUsers(ctx context.Context) ([]UserRef, error) {
	list, err := s.repo.ListBotUsers(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// MarkBotUser adds one user to the bot list, idempotently.
func (s *Service) MarkBotUser(ctx context.Context, providerID ProviderID, connectedAccountID, providerUserID, label string) (UserRef, error) {
	id, err := s.newID()
	if err != nil {
		return UserRef{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	saved, err := s.repo.AddBotUser(ctx, UserRef{
		ID: id, ProviderID: providerID, ConnectedAccountID: connectedAccountID,
		ProviderUserID: providerUserID, Label: label,
	}, s.now())
	if err != nil {
		return UserRef{}, mapRepoErr(err)
	}
	return saved, nil
}

// UnmarkBotUser removes one bot-user entry by its own id.
func (s *Service) UnmarkBotUser(ctx context.Context, id string) error {
	if err := s.repo.RemoveBotUser(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}
