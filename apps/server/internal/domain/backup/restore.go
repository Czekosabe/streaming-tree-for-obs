package backup

import (
	"context"
	"io"
	"time"

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
	"github.com/streaming-tree/server/internal/domain/onboarding"
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

// Sinks is the narrow write-only port Restore needs from every domain
// this backup includes - the write-side mirror of Sources. Every
// method here operates at the REPOSITORY layer, deliberately never
// through another domain's own Service: a restored row was already
// validated once (when it was originally created) and structurally
// re-validated by ReadArchive (docs/backup-restore.md §7 step "validate
// all logical objects"), so restore's own job is to persist it with a
// freshly generated LOCAL identity (§4/§5) - never to re-run OAuth,
// business-rule side effects, or anything that would touch a
// credential.
type Sinks struct {
	Platforms interface {
		Create(ctx context.Context, p platform.Platform) error
		Delete(ctx context.Context, id string) error
	}
	Output interface {
		Update(ctx context.Context, platformID string, input output.UpdateInput) (output.Settings, error)
	}
	RemoteTarget interface {
		// Set is enough on its own for restore: no explicit clear step is
		// needed (clearExisting's own doc comment) because
		// platform_remote_targets cascades on its platform's own
		// deletion (migration 0007), exactly like output settings.
		Set(ctx context.Context, t remotetarget.Target, now time.Time) (remotetarget.Target, error)
	}
	Accounts interface {
		CreateAccount(ctx context.Context, acc account.Account) error
		DeleteAccount(ctx context.Context, id string) error
		SetLink(ctx context.Context, platformID, accountID string, now time.Time) (account.Link, error)
		SetIntegrationSettings(ctx context.Context, providerID account.ProviderID, clientID string, now time.Time) (account.IntegrationSettings, error)
	}
	YouTubeRegion interface {
		SetRegion(ctx context.Context, accountID, region string, now time.Time) error
	}
	EngagementSettings interface {
		Set(ctx context.Context, s engagementsettings.Settings, now time.Time) (engagementsettings.Settings, error)
	}
	OperatorChatPrefs interface {
		SetPreferences(ctx context.Context, p operatorchatprefs.Preferences, now time.Time) (operatorchatprefs.Preferences, error)
		SetAccountVisibility(ctx context.Context, accountID string, visible bool, now time.Time) (operatorchatprefs.AccountVisibility, error)
		AddHiddenUser(ctx context.Context, ref operatorchatprefs.UserRef, now time.Time) (operatorchatprefs.UserRef, error)
		AddBotUser(ctx context.Context, ref operatorchatprefs.UserRef, now time.Time) (operatorchatprefs.UserRef, error)
	}
	ChatOverlays interface {
		CreateProfile(ctx context.Context, p chatoverlay.Profile) (chatoverlay.Profile, error)
		DeleteProfile(ctx context.Context, id string) error
		SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error
		AddHiddenUser(ctx context.Context, ref chatoverlay.HiddenUser, now time.Time) (chatoverlay.HiddenUser, error)
		AddBlockedTerm(ctx context.Context, term chatoverlay.BlockedTerm, now time.Time) (chatoverlay.BlockedTerm, error)
		SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error
	}
	ChatAutomation interface {
		CreateSchedule(ctx context.Context, s chatautomation.Schedule) (chatautomation.Schedule, error)
		CreateCommand(ctx context.Context, c chatautomation.Command) (chatautomation.Command, error)
		DeleteSchedule(ctx context.Context, id string) error
		DeleteCommand(ctx context.Context, id string) error
	}
	Alerts interface {
		CreateProfile(ctx context.Context, p alerts.Profile) (alerts.Profile, error)
		CreateRule(ctx context.Context, r alerts.Rule) (alerts.Rule, error)
		DeleteProfile(ctx context.Context, id string) error
	}
	VisualDesigns interface {
		Save(ctx context.Context, ownerKind visualdesign.OwnerKind, ownerID string, doc visualdesign.Document, expectedRevision int, newID func() (string, error)) (visualdesign.Record, error)
		Delete(ctx context.Context, ownerKind visualdesign.OwnerKind, ownerID string) error
	}
	VisualTemplates interface {
		Create(ctx context.Context, t visualtemplate.Template) (visualtemplate.Template, error)
		Delete(ctx context.Context, id string) error
	}
	VisualAssets interface {
		CreateBlob(ctx context.Context, b visualasset.Blob) error
		CreateAsset(ctx context.Context, a visualasset.Asset) error
		DeleteAsset(ctx context.Context, id string) error
	}
	AudioAssets interface {
		CreateBlob(ctx context.Context, b audioasset.Blob) error
		CreateAsset(ctx context.Context, a audioasset.Asset) error
		DeleteAsset(ctx context.Context, id string) error
	}
	AudioSettings interface {
		SetSettings(ctx context.Context, s audio.Settings, now time.Time) (audio.Settings, error)
	}
	Goals interface {
		CreateGoal(ctx context.Context, g goals.Goal) (goals.Goal, error)
		CreateWidgetProfile(ctx context.Context, p goals.WidgetProfile) (goals.WidgetProfile, error)
		// UpdateWidgetProfile is used only for a dashboard's own
		// Children, in a second pass once every sibling widget profile
		// this restore creates has a real new id (docs/backup-
		// restore.md - a dashboard's children must already exist).
		UpdateWidgetProfile(ctx context.Context, p goals.WidgetProfile) (goals.WidgetProfile, error)
		DeleteGoal(ctx context.Context, id string) error
		DeleteWidgetProfile(ctx context.Context, id string) error
	}
	MetadataPresets interface {
		Create(ctx context.Context, p metadatapreset.Preset) error
		Delete(ctx context.Context, id string) error
	}
	StreamSetupProfiles interface {
		Create(ctx context.Context, p streamsetup.Profile) error
		Delete(ctx context.Context, id string) error
	}
	DonationSources interface {
		CreateSource(ctx context.Context, s donationsource.Source) error
		DeleteSource(ctx context.Context, id string) error
	}
	UpdatePreferences interface {
		SetPreferences(ctx context.Context, p updatersettings.Preferences, now time.Time) (updatersettings.Preferences, error)
	}
	// StreamSessionSettings is the write side of Sources'
	// StreamSessionSettings - see its own doc comment. Never touches
	// stream_sessions/stream_session_destinations themselves.
	StreamSessionSettings interface {
		SetRetentionDays(ctx context.Context, days int, now time.Time) error
	}
	// Onboarding is never populated FROM the backup - Config carries no
	// onboarding field at all (docs/backup-restore.md §1). Restore calls
	// SetStatus itself, once, after every other domain above has been
	// written, with a status RECOMPUTED from what actually just landed
	// in the database - never the backup-time value verbatim - so a
	// restored install's onboarding-auto-show behavior always stays
	// consistent with its own just-restored configuration (see
	// recomputeOnboardingState in restore_commit.go for why).
	Onboarding interface {
		SetStatus(ctx context.Context, status onboarding.Status, schemaVersion int, now time.Time) (onboarding.State, error)
	}
}

// BlobWriter writes one already-validated asset's bytes into the real
// local blob store, returning the sha256 the store confirms -
// identical to the asset's own already-checked manifest hash (§5),
// since FileStore.WriteBlob recomputes and returns the hash of
// exactly what it wrote. Signature matches
// *visualasset.FileStore.WriteBlob exactly (both visual and audio
// assets share that one implementation) so no adapter is needed.
type BlobWriter interface {
	WriteBlob(r io.Reader, maxBytes int64) (sha256Hex string, size int64, err error)
}

// idMap tracks every backup-supplied id this restore has minted a
// fresh local id for, across every domain - flat, not per-domain,
// since every domain's own id prefix (pf_, acct_, ov_, ...) already
// makes cross-domain collision structurally impossible (docs/backup-
// restore.md §4).
type idMap map[string]string

func (m idMap) remap(old string) string {
	if old == "" {
		return ""
	}
	if newID, ok := m[old]; ok {
		return newID
	}
	// A reference to an id this restore never created a mapping for
	// (e.g. a cross-reference to an object type not included in v1, or
	// a genuinely broken backup) - left as-is rather than dropped
	// silently, so a downstream FK-style consistency issue is visible
	// rather than quietly losing data. Real domains validate/ignore an
	// unresolvable reference through their own existing rules (e.g.
	// alerts.Rule.Accounts already tolerates a stale id).
	return old
}

func (m idMap) remapAll(olds []string) []string {
	out := make([]string, len(olds))
	for i, o := range olds {
		out[i] = m.remap(o)
	}
	return out
}

// RestoreResult summarizes what a completed Restore actually wrote -
// built from real counts, never assumed equal to the preview (docs/
// backup-restore.md §26: "use actual restored state").
// RestartRequired is always true on a successful Restore: several
// runtime managers (chat automation, alerts, Twitch/YouTube/
// StreamElements engagement connectors) load their working state once
// at process start and are only ever refreshed through their own
// Service-layer mutating methods, which restore's direct-repository
// writes deliberately bypass (docs/backup-restore.md §7 step 7's own
// rationale for not routing bulk restore through per-domain business
// rules). Without a restart, a restored chat-automation schedule/
// command or alert profile/rule would sit correctly in the database
// but never actually run. RestoreResult always reports this honestly
// rather than claiming a live, seamless refresh that does not exist.
type RestoreResult struct {
	Counts                            ObjectCounts `json:"counts"`
	ConnectedAccountsRequireReconnect int          `json:"connectedAccountsRequireReconnect"`
	DestinationsNeedStreamKey         int          `json:"destinationsNeedStreamKey"`
	DonationSourcesNeedCredential     int          `json:"donationSourcesNeedCredential"`
	RestartRequired                   bool         `json:"restartRequired"`
}
