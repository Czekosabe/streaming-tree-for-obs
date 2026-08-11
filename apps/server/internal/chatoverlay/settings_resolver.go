package chatoverlay

import (
	"context"
	"fmt"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchatprefs "github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// SettingsResolver builds one overlay's resolvedSettings from durable
// storage - the profile itself, its selected accounts/hidden users/
// blocked terms/activity types, and Stage 9's own shared bot-user list.
// The only implementation lives in this package (defaultSettingsResolver
// below) because resolvedSettings is unexported - main.go wires a
// *DefaultSettingsResolver in, it never builds one itself.
type SettingsResolver interface {
	Resolve(ctx context.Context, overlayID string) (resolvedSettings, error)
}

// DefaultSettingsResolver is the production SettingsResolver: reads
// through internal/domain/chatoverlay's own Service for everything
// overlay-specific, and through internal/domain/operatorchatprefs's
// Service for the one thing deliberately shared with Stage 9's operator
// chat - the explicit, operator-maintained bot-user classification (see
// filtering.go's own doc comment on why that list, and only that list,
// is shared rather than duplicated per overlay).
type DefaultSettingsResolver struct {
	Profiles      *chatoverlaydomain.Service
	OperatorPrefs *operatorchatprefs.Service
	AccountLabel  AccountLabelLookup
	// VisualDesigns is Stage 13B's own shared visual-design service -
	// optional (nil degrades every overlay to legacy presentation,
	// mirroring internal/alerts.ManagerOptions's own nil-safe pattern),
	// used only to derive this overlay's own designDataNeeds
	// (docs/visual-designs.md §22) - never to decide filtering.
	VisualDesigns *visualdesign.Service
}

func (r *DefaultSettingsResolver) Resolve(ctx context.Context, overlayID string) (resolvedSettings, error) {
	profile, err := r.Profiles.GetProfile(ctx, overlayID)
	if err != nil {
		return resolvedSettings{}, fmt.Errorf("resolve profile: %w", err)
	}

	accountIDs, err := r.Profiles.Accounts(ctx, overlayID)
	if err != nil {
		return resolvedSettings{}, fmt.Errorf("resolve accounts: %w", err)
	}

	hidden, err := r.Profiles.HiddenUsers(ctx, overlayID)
	if err != nil {
		return resolvedSettings{}, fmt.Errorf("resolve hidden users: %w", err)
	}
	hiddenSet := make(map[string]struct{}, len(hidden))
	for _, u := range hidden {
		hiddenSet[userKey(string(u.ProviderID), u.ConnectedAccountID, u.ProviderUserID)] = struct{}{}
	}

	var botSet map[string]struct{}
	if r.OperatorPrefs != nil {
		bots, err := r.OperatorPrefs.BotUsers(ctx)
		if err != nil {
			return resolvedSettings{}, fmt.Errorf("resolve bot users: %w", err)
		}
		botSet = make(map[string]struct{}, len(bots))
		for _, u := range bots {
			botSet[userKey(string(u.ProviderID), u.ConnectedAccountID, u.ProviderUserID)] = struct{}{}
		}
	}

	terms, err := r.Profiles.BlockedTerms(ctx, overlayID)
	if err != nil {
		return resolvedSettings{}, fmt.Errorf("resolve blocked terms: %w", err)
	}

	activityTypes, err := r.Profiles.ActivityTypes(ctx, overlayID)
	if err != nil {
		return resolvedSettings{}, fmt.Errorf("resolve activity types: %w", err)
	}

	var designDataNeeds *chatoverlaydomain.ChatDataNeeds
	if r.VisualDesigns != nil {
		rec, found, err := r.VisualDesigns.Get(ctx, visualdesign.OwnerKindChatOverlay, overlayID)
		if err != nil {
			return resolvedSettings{}, fmt.Errorf("resolve visual design: %w", err)
		}
		if found {
			needs := chatoverlaydomain.DeriveDataNeeds(rec.Document)
			designDataNeeds = &needs
		}
	}

	return resolvedSettings{
		profile:         profile,
		accountIDs:      toSet(accountIDs),
		hiddenUsers:     hiddenSet,
		botUsers:        botSet,
		activityTypes:   toSet(activityTypes),
		blockedTerms:    terms,
		accountLabel:    r.AccountLabel,
		designDataNeeds: designDataNeeds,
	}, nil
}
