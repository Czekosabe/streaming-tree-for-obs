package backup

import (
	"context"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/audio"
	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// Sources is the narrow read-only port Export needs from every domain
// this backup includes - mirroring this codebase's own established
// per-consumer-interface convention (e.g. Stage 22's
// metadatapreset.PlatformMetadataStore). Every method here is already
// exercised by that domain's own real repository, exactly as
// internal/userdatapurge already reads several of these same
// repositories directly for its own cross-domain sweep. Export never
// depends on internal/secrets, internal/auth, or any credential
// accessor - see model.go's own package doc comment.
type Sources struct {
	Platforms interface {
		List(ctx context.Context) ([]platform.Platform, error)
	}
	Output interface {
		Get(ctx context.Context, platformID string) (output.Settings, error)
	}
	RemoteTarget interface {
		Get(ctx context.Context, platformID string) (remotetarget.Target, bool, error)
	}
	Accounts interface {
		ListAccounts(ctx context.Context) ([]account.Account, error)
		ListLinksByAccount(ctx context.Context, accountID string) ([]account.Link, error)
		GetIntegrationSettings(ctx context.Context, providerID account.ProviderID) (account.IntegrationSettings, bool, error)
	}
	YouTubeRegion interface {
		GetRegion(ctx context.Context, accountID string) (string, bool, error)
	}
	EngagementSettings interface {
		Get(ctx context.Context, accountID string) (engagementsettings.Settings, bool, error)
	}
	OperatorChatPrefs interface {
		GetPreferences(ctx context.Context) (operatorchatprefs.Preferences, bool, error)
		ListAccountVisibility(ctx context.Context) ([]operatorchatprefs.AccountVisibility, error)
		ListHiddenUsers(ctx context.Context) ([]operatorchatprefs.UserRef, error)
		ListBotUsers(ctx context.Context) ([]operatorchatprefs.UserRef, error)
	}
	ChatOverlays interface {
		ListProfiles(ctx context.Context) ([]chatoverlay.Profile, error)
		ListAccounts(ctx context.Context, overlayID string) ([]string, error)
		ListHiddenUsers(ctx context.Context, overlayID string) ([]chatoverlay.HiddenUser, error)
		ListBlockedTerms(ctx context.Context, overlayID string) ([]chatoverlay.BlockedTerm, error)
		ListActivityTypes(ctx context.Context, overlayID string) ([]string, error)
	}
	ChatAutomation interface {
		ListSchedules(ctx context.Context) ([]chatautomation.Schedule, error)
		ListCommands(ctx context.Context) ([]chatautomation.Command, error)
	}
	Alerts interface {
		ListProfiles(ctx context.Context) ([]alerts.Profile, error)
		ListRules(ctx context.Context, profileID string) ([]alerts.Rule, error)
	}
	VisualDesigns interface {
		Get(ctx context.Context, ownerKind visualdesign.OwnerKind, ownerID string) (visualdesign.Record, bool, error)
	}
	VisualTemplates interface {
		List(ctx context.Context) ([]visualtemplate.Template, error)
	}
	VisualAssets interface {
		ListAssets(ctx context.Context) ([]visualasset.Asset, error)
		// GetBlob resolves one asset's own Blob (ListAssets never
		// populates it - it is a read-time join, exactly like
		// visualasset.Service.resolveBlob already does for every other
		// visual-asset read path). Export calls this once per distinct
		// asset so WriteArchive's own blob-collection pass
		// (collectBlobRefs, which only ever looks at Asset.Blob) has
		// something to find - without this, a backup would silently
		// include an asset's METADATA row but never its actual image/
		// sound file.
		GetBlob(ctx context.Context, sha256Hex string) (visualasset.Blob, bool, error)
	}
	AudioAssets interface {
		ListAssets(ctx context.Context) ([]audioasset.Asset, error)
		// GetBlob: see VisualAssets.GetBlob's own doc comment - the same
		// reasoning, mirrored for audio assets.
		GetBlob(ctx context.Context, sha256Hex string) (audioasset.Blob, bool, error)
	}
	AudioSettings interface {
		GetSettings(ctx context.Context) (audio.Settings, bool, error)
	}
	Goals interface {
		ListGoals(ctx context.Context) ([]goals.Goal, error)
		ListWidgetProfiles(ctx context.Context, goalID string) ([]goals.WidgetProfile, error)
	}
	MetadataPresets interface {
		List(ctx context.Context) ([]metadatapreset.Preset, error)
	}
	StreamSetupProfiles interface {
		List(ctx context.Context) ([]streamsetup.Profile, error)
	}
	DonationSources interface {
		ListSources(ctx context.Context) ([]donationsource.Source, error)
	}
	UpdatePreferences interface {
		GetPreferences(ctx context.Context) (updatersettings.Preferences, bool, error)
	}
	// StreamSessionSettings is the one PORTABLE preference in the
	// operational-history domain - how many days to keep session
	// history, not the history itself (streamsession's Session/
	// Destination rows are deliberately excluded - docs/backup-
	// restore.md's history/observability classification). Mirrors
	// UpdatePreferences/AudioSettings' own singleton-preference shape
	// exactly.
	StreamSessionSettings interface {
		GetRetentionDays(ctx context.Context) (days int, found bool, err error)
	}
}

// knownAccountProviders lists every account.ProviderID this
// application's type system defines, so Export can read each
// provider's own integration settings without needing a "list every
// provider that has a row" query - the provider set is small, fixed,
// and defined in Go, exactly like platform.Definitions().
var knownAccountProviders = []account.ProviderID{account.ProviderTwitch, account.ProviderYouTube}

// Export reads a complete, coherent Config from every included domain.
// Callers are responsible for the coherent-snapshot boundary
// (docs/backup-restore.md §6) - Export itself only issues reads, in a
// deterministic order, against whatever Sources resolves to.
func Export(ctx context.Context, src Sources) (Config, error) {
	cfg := Config{FormatVersion: FormatVersion}

	platforms, err := src.Platforms.List(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("list platforms: %w", err)
	}
	for _, p := range platforms {
		out, err := src.Output.Get(ctx, p.ID)
		if err != nil {
			return Config{}, fmt.Errorf("get output settings for platform %s: %w", p.ID, err)
		}
		pe := PlatformExport{Platform: p, Output: out}
		if rt, ok, err := src.RemoteTarget.Get(ctx, p.ID); err != nil {
			return Config{}, fmt.Errorf("get remote target for platform %s: %w", p.ID, err)
		} else if ok {
			pe.RemoteTarget = &rt
		}
		cfg.Platforms = append(cfg.Platforms, pe)
	}

	for _, providerID := range knownAccountProviders {
		settings, ok, err := src.Accounts.GetIntegrationSettings(ctx, providerID)
		if err != nil {
			return Config{}, fmt.Errorf("get integration settings for provider %s: %w", providerID, err)
		}
		if ok {
			cfg.ProviderIntegrationSettings = append(cfg.ProviderIntegrationSettings, settings)
		}
	}

	accounts, err := src.Accounts.ListAccounts(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("list accounts: %w", err)
	}
	for _, a := range accounts {
		links, err := src.Accounts.ListLinksByAccount(ctx, a.ID)
		if err != nil {
			return Config{}, fmt.Errorf("list links for account %s: %w", a.ID, err)
		}
		platformIDs := make([]string, 0, len(links))
		for _, l := range links {
			platformIDs = append(platformIDs, l.PlatformID)
		}

		region, hasRegion, err := src.YouTubeRegion.GetRegion(ctx, a.ID)
		if err != nil {
			return Config{}, fmt.Errorf("get youtube region for account %s: %w", a.ID, err)
		}
		if !hasRegion {
			region = ""
		}

		var engagementEnabled *bool
		if es, ok, err := src.EngagementSettings.Get(ctx, a.ID); err != nil {
			return Config{}, fmt.Errorf("get engagement settings for account %s: %w", a.ID, err)
		} else if ok {
			engagementEnabled = &es.Enabled
		}

		cfg.ConnectedAccounts = append(cfg.ConnectedAccounts, ConnectedAccountExport{
			Account:           a,
			PlatformLinks:     platformIDs,
			YouTubeRegion:     region,
			EngagementEnabled: engagementEnabled,
		})
	}

	if prefs, ok, err := src.OperatorChatPrefs.GetPreferences(ctx); err != nil {
		return Config{}, fmt.Errorf("get operator chat preferences: %w", err)
	} else if ok {
		visibility, err := src.OperatorChatPrefs.ListAccountVisibility(ctx)
		if err != nil {
			return Config{}, fmt.Errorf("list operator chat account visibility: %w", err)
		}
		hidden, err := src.OperatorChatPrefs.ListHiddenUsers(ctx)
		if err != nil {
			return Config{}, fmt.Errorf("list operator chat hidden users: %w", err)
		}
		bots, err := src.OperatorChatPrefs.ListBotUsers(ctx)
		if err != nil {
			return Config{}, fmt.Errorf("list operator chat bot users: %w", err)
		}
		cfg.OperatorChatPreferences = &OperatorChatPreferencesExport{
			Preferences: prefs, AccountVisibility: visibility, HiddenUsers: hidden, BotUsers: bots,
		}
	}

	overlays, err := src.ChatOverlays.ListProfiles(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("list chat overlays: %w", err)
	}
	for _, o := range overlays {
		accountIDs, err := src.ChatOverlays.ListAccounts(ctx, o.ID)
		if err != nil {
			return Config{}, fmt.Errorf("list chat overlay accounts for %s: %w", o.ID, err)
		}
		hiddenUsers, err := src.ChatOverlays.ListHiddenUsers(ctx, o.ID)
		if err != nil {
			return Config{}, fmt.Errorf("list chat overlay hidden users for %s: %w", o.ID, err)
		}
		blockedTerms, err := src.ChatOverlays.ListBlockedTerms(ctx, o.ID)
		if err != nil {
			return Config{}, fmt.Errorf("list chat overlay blocked terms for %s: %w", o.ID, err)
		}
		activityTypes, err := src.ChatOverlays.ListActivityTypes(ctx, o.ID)
		if err != nil {
			return Config{}, fmt.Errorf("list chat overlay activity types for %s: %w", o.ID, err)
		}

		design, hasDesign, err := src.VisualDesigns.Get(ctx, visualdesign.OwnerKindChatOverlay, o.ID)
		if err != nil {
			return Config{}, fmt.Errorf("get visual design for chat overlay %s: %w", o.ID, err)
		}
		if hasDesign {
			cfg.VisualDesigns = append(cfg.VisualDesigns, design)
		}

		cfg.ChatOverlays = append(cfg.ChatOverlays, ChatOverlayExport{
			Profile: o, AccountIDs: accountIDs, HiddenUsers: hiddenUsers,
			BlockedTerms: blockedTerms, ActivityTypes: activityTypes,
		})
	}

	if cfg.ChatSchedules, err = src.ChatAutomation.ListSchedules(ctx); err != nil {
		return Config{}, fmt.Errorf("list chat schedules: %w", err)
	}
	if cfg.ChatCommands, err = src.ChatAutomation.ListCommands(ctx); err != nil {
		return Config{}, fmt.Errorf("list chat commands: %w", err)
	}

	profiles, err := src.Alerts.ListProfiles(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("list alert profiles: %w", err)
	}
	for _, p := range profiles {
		rules, err := src.Alerts.ListRules(ctx, p.ID)
		if err != nil {
			return Config{}, fmt.Errorf("list alert rules for profile %s: %w", p.ID, err)
		}
		cfg.AlertProfiles = append(cfg.AlertProfiles, AlertProfileExport{Profile: p, Rules: rules})

		for _, ru := range rules {
			design, hasDesign, err := src.VisualDesigns.Get(ctx, visualdesign.OwnerKindAlertRule, ru.ID)
			if err != nil {
				return Config{}, fmt.Errorf("get visual design for alert rule %s: %w", ru.ID, err)
			}
			if hasDesign {
				cfg.VisualDesigns = append(cfg.VisualDesigns, design)
			}
		}
	}

	if cfg.VisualTemplates, err = src.VisualTemplates.List(ctx); err != nil {
		return Config{}, fmt.Errorf("list visual templates: %w", err)
	}
	if cfg.VisualAssets, err = src.VisualAssets.ListAssets(ctx); err != nil {
		return Config{}, fmt.Errorf("list visual assets: %w", err)
	}
	for i, a := range cfg.VisualAssets {
		blob, found, err := src.VisualAssets.GetBlob(ctx, a.BlobSHA256)
		if err != nil {
			return Config{}, fmt.Errorf("resolve visual asset blob %q: %w", a.BlobSHA256, err)
		}
		if found {
			cfg.VisualAssets[i].Blob = &blob
		}
	}
	if cfg.AudioAssets, err = src.AudioAssets.ListAssets(ctx); err != nil {
		return Config{}, fmt.Errorf("list audio assets: %w", err)
	}
	for i, a := range cfg.AudioAssets {
		blob, found, err := src.AudioAssets.GetBlob(ctx, a.BlobSHA256)
		if err != nil {
			return Config{}, fmt.Errorf("resolve audio asset blob %q: %w", a.BlobSHA256, err)
		}
		if found {
			cfg.AudioAssets[i].Blob = &blob
		}
	}

	if settings, ok, err := src.AudioSettings.GetSettings(ctx); err != nil {
		return Config{}, fmt.Errorf("get audio settings: %w", err)
	} else if ok {
		cfg.AudioSettings = &settings
	}

	goalRows, err := src.Goals.ListGoals(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("list goals: %w", err)
	}
	cfg.Goals = goalRows
	if cfg.WidgetProfiles, err = src.Goals.ListWidgetProfiles(ctx, ""); err != nil {
		return Config{}, fmt.Errorf("list widget profiles: %w", err)
	}

	if cfg.MetadataPresets, err = src.MetadataPresets.List(ctx); err != nil {
		return Config{}, fmt.Errorf("list metadata presets: %w", err)
	}
	if cfg.StreamSetupProfiles, err = src.StreamSetupProfiles.List(ctx); err != nil {
		return Config{}, fmt.Errorf("list stream setup profiles: %w", err)
	}
	if cfg.DonationSources, err = src.DonationSources.ListSources(ctx); err != nil {
		return Config{}, fmt.Errorf("list donation sources: %w", err)
	}

	if prefs, ok, err := src.UpdatePreferences.GetPreferences(ctx); err != nil {
		return Config{}, fmt.Errorf("get update preferences: %w", err)
	} else if ok {
		cfg.UpdatePreferences = &prefs
	}

	if days, ok, err := src.StreamSessionSettings.GetRetentionDays(ctx); err != nil {
		return Config{}, fmt.Errorf("get stream session retention days: %w", err)
	} else if ok {
		cfg.StreamSessionRetentionDays = &days
	}

	return cfg, nil
}
