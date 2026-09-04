package backup

import (
	"context"
	"testing"

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

type fakePlatforms struct{ rows []platform.Platform }

func (f fakePlatforms) List(context.Context) ([]platform.Platform, error) { return f.rows, nil }

type fakeOutput struct{ byPlatform map[string]output.Settings }

func (f fakeOutput) Get(_ context.Context, platformID string) (output.Settings, error) {
	return f.byPlatform[platformID], nil
}

type fakeRemoteTarget struct {
	byPlatform map[string]remotetarget.Target
}

func (f fakeRemoteTarget) Get(_ context.Context, platformID string) (remotetarget.Target, bool, error) {
	t, ok := f.byPlatform[platformID]
	return t, ok, nil
}

type fakeAccounts struct {
	rows        []account.Account
	links       map[string][]account.Link
	integration map[account.ProviderID]account.IntegrationSettings
}

func (f fakeAccounts) ListAccounts(context.Context) ([]account.Account, error) { return f.rows, nil }
func (f fakeAccounts) ListLinksByAccount(_ context.Context, accountID string) ([]account.Link, error) {
	return f.links[accountID], nil
}
func (f fakeAccounts) GetIntegrationSettings(_ context.Context, providerID account.ProviderID) (account.IntegrationSettings, bool, error) {
	s, ok := f.integration[providerID]
	return s, ok, nil
}

type fakeYouTubeRegion struct{ byAccount map[string]string }

func (f fakeYouTubeRegion) GetRegion(_ context.Context, accountID string) (string, bool, error) {
	r, ok := f.byAccount[accountID]
	return r, ok, nil
}

type fakeEngagementSettings struct {
	byAccount map[string]engagementsettings.Settings
}

func (f fakeEngagementSettings) Get(_ context.Context, accountID string) (engagementsettings.Settings, bool, error) {
	s, ok := f.byAccount[accountID]
	return s, ok, nil
}

type fakeOperatorChatPrefs struct {
	prefs      operatorchatprefs.Preferences
	hasPrefs   bool
	visibility []operatorchatprefs.AccountVisibility
	hidden     []operatorchatprefs.UserRef
	bots       []operatorchatprefs.UserRef
}

func (f fakeOperatorChatPrefs) GetPreferences(context.Context) (operatorchatprefs.Preferences, bool, error) {
	return f.prefs, f.hasPrefs, nil
}
func (f fakeOperatorChatPrefs) ListAccountVisibility(context.Context) ([]operatorchatprefs.AccountVisibility, error) {
	return f.visibility, nil
}
func (f fakeOperatorChatPrefs) ListHiddenUsers(context.Context) ([]operatorchatprefs.UserRef, error) {
	return f.hidden, nil
}
func (f fakeOperatorChatPrefs) ListBotUsers(context.Context) ([]operatorchatprefs.UserRef, error) {
	return f.bots, nil
}

type fakeChatOverlays struct {
	profiles      []chatoverlay.Profile
	accounts      map[string][]string
	hidden        map[string][]chatoverlay.HiddenUser
	blocked       map[string][]chatoverlay.BlockedTerm
	activityTypes map[string][]string
}

func (f fakeChatOverlays) ListProfiles(context.Context) ([]chatoverlay.Profile, error) {
	return f.profiles, nil
}
func (f fakeChatOverlays) ListAccounts(_ context.Context, overlayID string) ([]string, error) {
	return f.accounts[overlayID], nil
}
func (f fakeChatOverlays) ListHiddenUsers(_ context.Context, overlayID string) ([]chatoverlay.HiddenUser, error) {
	return f.hidden[overlayID], nil
}
func (f fakeChatOverlays) ListBlockedTerms(_ context.Context, overlayID string) ([]chatoverlay.BlockedTerm, error) {
	return f.blocked[overlayID], nil
}
func (f fakeChatOverlays) ListActivityTypes(_ context.Context, overlayID string) ([]string, error) {
	return f.activityTypes[overlayID], nil
}

type fakeChatAutomation struct {
	schedules []chatautomation.Schedule
	commands  []chatautomation.Command
}

func (f fakeChatAutomation) ListSchedules(context.Context) ([]chatautomation.Schedule, error) {
	return f.schedules, nil
}
func (f fakeChatAutomation) ListCommands(context.Context) ([]chatautomation.Command, error) {
	return f.commands, nil
}

type fakeAlerts struct {
	profiles []alerts.Profile
	rules    map[string][]alerts.Rule
}

func (f fakeAlerts) ListProfiles(context.Context) ([]alerts.Profile, error) { return f.profiles, nil }
func (f fakeAlerts) ListRules(_ context.Context, profileID string) ([]alerts.Rule, error) {
	return f.rules[profileID], nil
}

type fakeVisualDesigns struct {
	byOwner map[string]visualdesign.Record
}

func (f fakeVisualDesigns) Get(_ context.Context, ownerKind visualdesign.OwnerKind, ownerID string) (visualdesign.Record, bool, error) {
	r, ok := f.byOwner[string(ownerKind)+":"+ownerID]
	return r, ok, nil
}

type fakeVisualTemplates struct{ rows []visualtemplate.Template }

func (f fakeVisualTemplates) List(context.Context) ([]visualtemplate.Template, error) {
	return f.rows, nil
}

type fakeVisualAssets struct {
	rows  []visualasset.Asset
	blobs map[string]visualasset.Blob
}

func (f fakeVisualAssets) ListAssets(context.Context) ([]visualasset.Asset, error) {
	return f.rows, nil
}

func (f fakeVisualAssets) GetBlob(_ context.Context, sha256Hex string) (visualasset.Blob, bool, error) {
	b, ok := f.blobs[sha256Hex]
	return b, ok, nil
}

type fakeAudioAssets struct {
	rows  []audioasset.Asset
	blobs map[string]audioasset.Blob
}

func (f fakeAudioAssets) ListAssets(context.Context) ([]audioasset.Asset, error) { return f.rows, nil }

func (f fakeAudioAssets) GetBlob(_ context.Context, sha256Hex string) (audioasset.Blob, bool, error) {
	b, ok := f.blobs[sha256Hex]
	return b, ok, nil
}

type fakeAudioSettings struct {
	settings audio.Settings
	has      bool
}

func (f fakeAudioSettings) GetSettings(context.Context) (audio.Settings, bool, error) {
	return f.settings, f.has, nil
}

type fakeGoals struct {
	goals   []goals.Goal
	widgets []goals.WidgetProfile
}

func (f fakeGoals) ListGoals(context.Context) ([]goals.Goal, error) { return f.goals, nil }
func (f fakeGoals) ListWidgetProfiles(_ context.Context, goalID string) ([]goals.WidgetProfile, error) {
	if goalID != "" {
		t := []goals.WidgetProfile{}
		for _, w := range f.widgets {
			if w.GoalID == goalID {
				t = append(t, w)
			}
		}
		return t, nil
	}
	return f.widgets, nil
}

type fakeMetadataPresets struct{ rows []metadatapreset.Preset }

func (f fakeMetadataPresets) List(context.Context) ([]metadatapreset.Preset, error) {
	return f.rows, nil
}

type fakeStreamSetupProfiles struct{ rows []streamsetup.Profile }

func (f fakeStreamSetupProfiles) List(context.Context) ([]streamsetup.Profile, error) {
	return f.rows, nil
}

type fakeDonationSources struct{ rows []donationsource.Source }

func (f fakeDonationSources) ListSources(context.Context) ([]donationsource.Source, error) {
	return f.rows, nil
}

type fakeUpdatePreferences struct {
	prefs updatersettings.Preferences
	has   bool
}

func (f fakeUpdatePreferences) GetPreferences(context.Context) (updatersettings.Preferences, bool, error) {
	return f.prefs, f.has, nil
}

type fakeStreamSessionSettings struct {
	days int
	has  bool
}

func (f fakeStreamSessionSettings) GetRetentionDays(context.Context) (int, bool, error) {
	return f.days, f.has, nil
}

// emptySources returns a fully-wired Sources with every fake empty -
// the baseline every test in this file starts from and overrides one
// field of, so an unrelated Sources field can never be left nil and
// panic on a call this test does not care about.
func emptySources() Sources {
	return Sources{
		Platforms:          fakePlatforms{},
		Output:             fakeOutput{byPlatform: map[string]output.Settings{}},
		RemoteTarget:       fakeRemoteTarget{byPlatform: map[string]remotetarget.Target{}},
		Accounts:           fakeAccounts{links: map[string][]account.Link{}, integration: map[account.ProviderID]account.IntegrationSettings{}},
		YouTubeRegion:      fakeYouTubeRegion{byAccount: map[string]string{}},
		EngagementSettings: fakeEngagementSettings{byAccount: map[string]engagementsettings.Settings{}},
		OperatorChatPrefs:  fakeOperatorChatPrefs{},
		ChatOverlays: fakeChatOverlays{
			accounts: map[string][]string{}, hidden: map[string][]chatoverlay.HiddenUser{},
			blocked: map[string][]chatoverlay.BlockedTerm{}, activityTypes: map[string][]string{},
		},
		ChatAutomation:        fakeChatAutomation{},
		Alerts:                fakeAlerts{rules: map[string][]alerts.Rule{}},
		VisualDesigns:         fakeVisualDesigns{byOwner: map[string]visualdesign.Record{}},
		VisualTemplates:       fakeVisualTemplates{},
		VisualAssets:          fakeVisualAssets{},
		AudioAssets:           fakeAudioAssets{},
		AudioSettings:         fakeAudioSettings{},
		Goals:                 fakeGoals{},
		MetadataPresets:       fakeMetadataPresets{},
		StreamSetupProfiles:   fakeStreamSetupProfiles{},
		DonationSources:       fakeDonationSources{},
		UpdatePreferences:     fakeUpdatePreferences{},
		StreamSessionSettings: fakeStreamSessionSettings{},
	}
}

func TestExportEmptyDatabaseProducesEmptyConfig(t *testing.T) {
	cfg, err := Export(context.Background(), emptySources())
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if cfg.FormatVersion != FormatVersion {
		t.Errorf("FormatVersion = %d, want %d", cfg.FormatVersion, FormatVersion)
	}
	if len(cfg.Platforms) != 0 || len(cfg.ConnectedAccounts) != 0 || len(cfg.AlertProfiles) != 0 {
		t.Errorf("expected every slice empty on a fresh database, got %+v", cfg)
	}
	if cfg.AudioSettings != nil || cfg.UpdatePreferences != nil || cfg.OperatorChatPreferences != nil {
		t.Errorf("expected every singleton nil when never saved, got %+v", cfg)
	}
}

func TestExportComposesPlatformWithItsOutputSettings(t *testing.T) {
	src := emptySources()
	src.Platforms = fakePlatforms{rows: []platform.Platform{{ID: "pf_1", DisplayName: "Main Twitch"}}}
	src.Output = fakeOutput{byPlatform: map[string]output.Settings{"pf_1": {ServerURL: "rtmp://example/live"}}}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.Platforms) != 1 {
		t.Fatalf("got %d platforms, want 1", len(cfg.Platforms))
	}
	if cfg.Platforms[0].Platform.ID != "pf_1" || cfg.Platforms[0].Output.ServerURL != "rtmp://example/live" {
		t.Errorf("platform composition wrong: %+v", cfg.Platforms[0])
	}
}

func TestExportComposesConnectedAccountWithLinksRegionAndEngagement(t *testing.T) {
	src := emptySources()
	src.Accounts = fakeAccounts{
		rows:        []account.Account{{ID: "acct_1", ProviderID: account.ProviderYouTube}},
		links:       map[string][]account.Link{"acct_1": {{PlatformID: "pf_1"}, {PlatformID: "pf_2"}}},
		integration: map[account.ProviderID]account.IntegrationSettings{},
	}
	src.YouTubeRegion = fakeYouTubeRegion{byAccount: map[string]string{"acct_1": "US"}}
	src.EngagementSettings = fakeEngagementSettings{byAccount: map[string]engagementsettings.Settings{"acct_1": {Enabled: true}}}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.ConnectedAccounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(cfg.ConnectedAccounts))
	}
	got := cfg.ConnectedAccounts[0]
	if len(got.PlatformLinks) != 2 {
		t.Errorf("PlatformLinks = %v, want 2 entries", got.PlatformLinks)
	}
	if got.YouTubeRegion != "US" {
		t.Errorf("YouTubeRegion = %q, want US", got.YouTubeRegion)
	}
	if got.EngagementEnabled == nil || !*got.EngagementEnabled {
		t.Errorf("EngagementEnabled = %v, want true", got.EngagementEnabled)
	}
}

func TestExportComposesChatOverlayWithChildRowsAndVisualDesign(t *testing.T) {
	src := emptySources()
	src.ChatOverlays = fakeChatOverlays{
		profiles:      []chatoverlay.Profile{{ID: "ov_1", PublicSlug: "abc123"}},
		accounts:      map[string][]string{"ov_1": {"acct_1"}},
		hidden:        map[string][]chatoverlay.HiddenUser{"ov_1": {{ProviderUserID: "u1"}}},
		blocked:       map[string][]chatoverlay.BlockedTerm{"ov_1": {{Value: "spam"}}},
		activityTypes: map[string][]string{"ov_1": {"follow"}},
	}
	src.VisualDesigns = fakeVisualDesigns{byOwner: map[string]visualdesign.Record{
		"chat_overlay:ov_1": {ID: "vd_1", OwnerKind: visualdesign.OwnerKindChatOverlay, OwnerID: "ov_1"},
	}}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.ChatOverlays) != 1 {
		t.Fatalf("got %d chat overlays, want 1", len(cfg.ChatOverlays))
	}
	got := cfg.ChatOverlays[0]
	if len(got.AccountIDs) != 1 || len(got.HiddenUsers) != 1 || len(got.BlockedTerms) != 1 || len(got.ActivityTypes) != 1 {
		t.Errorf("chat overlay child rows missing: %+v", got)
	}
	if len(cfg.VisualDesigns) != 1 || cfg.VisualDesigns[0].ID != "vd_1" {
		t.Errorf("visual design not exported for the overlay: %+v", cfg.VisualDesigns)
	}
}

func TestExportComposesAlertProfileWithRulesAndVisualDesign(t *testing.T) {
	src := emptySources()
	src.Alerts = fakeAlerts{
		profiles: []alerts.Profile{{ID: "alp_1", PublicSlug: "def456"}},
		rules:    map[string][]alerts.Rule{"alp_1": {{ID: "alr_1", ProfileID: "alp_1"}}},
	}
	src.VisualDesigns = fakeVisualDesigns{byOwner: map[string]visualdesign.Record{
		"alert_rule:alr_1": {ID: "vd_2", OwnerKind: visualdesign.OwnerKindAlertRule, OwnerID: "alr_1"},
	}}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.AlertProfiles) != 1 || len(cfg.AlertProfiles[0].Rules) != 1 {
		t.Fatalf("alert profile/rule composition wrong: %+v", cfg.AlertProfiles)
	}
	if len(cfg.VisualDesigns) != 1 || cfg.VisualDesigns[0].ID != "vd_2" {
		t.Errorf("visual design not exported for the alert rule: %+v", cfg.VisualDesigns)
	}
}

func TestExportListsWidgetProfilesAcrossEveryGoal(t *testing.T) {
	src := emptySources()
	src.Goals = fakeGoals{
		goals: []goals.Goal{{ID: "goal_1"}, {ID: "goal_2"}},
		widgets: []goals.WidgetProfile{
			{ID: "wp_1", GoalID: "goal_1"},
			{ID: "wp_2", GoalID: "goal_2"},
			{ID: "wp_3", Kind: goals.WidgetProfileKindDashboard},
		},
	}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.Goals) != 2 {
		t.Errorf("got %d goals, want 2", len(cfg.Goals))
	}
	if len(cfg.WidgetProfiles) != 3 {
		t.Errorf("got %d widget profiles, want 3 (including the goal-less dashboard)", len(cfg.WidgetProfiles))
	}
}

func TestExportReadsEverySingletonWhenPresent(t *testing.T) {
	src := emptySources()
	src.AudioSettings = fakeAudioSettings{settings: audio.Settings{Enabled: true}, has: true}
	src.UpdatePreferences = fakeUpdatePreferences{prefs: updatersettings.Preferences{AutoCheck: false}, has: true}
	src.OperatorChatPrefs = fakeOperatorChatPrefs{prefs: operatorchatprefs.Preferences{ShowBadges: true}, hasPrefs: true}
	src.StreamSessionSettings = fakeStreamSessionSettings{days: 30, has: true}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if cfg.AudioSettings == nil || !cfg.AudioSettings.Enabled {
		t.Errorf("AudioSettings = %+v, want Enabled=true", cfg.AudioSettings)
	}
	if cfg.UpdatePreferences == nil || cfg.UpdatePreferences.AutoCheck {
		t.Errorf("UpdatePreferences = %+v, want AutoCheck=false", cfg.UpdatePreferences)
	}
	if cfg.OperatorChatPreferences == nil || !cfg.OperatorChatPreferences.Preferences.ShowBadges {
		t.Errorf("OperatorChatPreferences = %+v, want ShowBadges=true", cfg.OperatorChatPreferences)
	}
	if cfg.StreamSessionRetentionDays == nil || *cfg.StreamSessionRetentionDays != 30 {
		t.Errorf("StreamSessionRetentionDays = %v, want 30", cfg.StreamSessionRetentionDays)
	}
}

// TestExportOmitsStreamSessionRetentionDaysWhenNeverSet proves the
// absent-row-means-default convention (streamsession.Repository's own
// doc comment) survives Export - a never-configured retention
// preference must stay nil in the backup, never a guessed
// DefaultRetentionDays value baked in as if the operator had chosen it.
func TestExportOmitsStreamSessionRetentionDaysWhenNeverSet(t *testing.T) {
	src := emptySources()

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if cfg.StreamSessionRetentionDays != nil {
		t.Errorf("StreamSessionRetentionDays = %v, want nil when never explicitly set", *cfg.StreamSessionRetentionDays)
	}
}

func TestExportListsMetadataPresetsAndDonationSourcesAndChatAutomation(t *testing.T) {
	src := emptySources()
	src.MetadataPresets = fakeMetadataPresets{rows: []metadatapreset.Preset{{ID: "mp_1", Name: "Coding"}}}
	src.StreamSetupProfiles = fakeStreamSetupProfiles{rows: []streamsetup.Profile{{ID: "setup_1", Name: "Gaming"}}}
	src.DonationSources = fakeDonationSources{rows: []donationsource.Source{{ID: "donsrc_1", Label: "Main"}}}
	src.ChatAutomation = fakeChatAutomation{
		schedules: []chatautomation.Schedule{{ID: "sch_1", Name: "Reminder"}},
		commands:  []chatautomation.Command{{ID: "cmd_1", Name: "discord"}},
	}
	src.VisualTemplates = fakeVisualTemplates{rows: []visualtemplate.Template{{ID: "tpl_1", Name: "Neon"}}}
	src.VisualAssets = fakeVisualAssets{rows: []visualasset.Asset{{ID: "asset_1", DisplayName: "logo.png"}}}
	src.AudioAssets = fakeAudioAssets{rows: []audioasset.Asset{{ID: "aud_1", DisplayName: "ding.wav"}}}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.MetadataPresets) != 1 || len(cfg.StreamSetupProfiles) != 1 || len(cfg.DonationSources) != 1 || len(cfg.ChatSchedules) != 1 ||
		len(cfg.ChatCommands) != 1 || len(cfg.VisualTemplates) != 1 || len(cfg.VisualAssets) != 1 || len(cfg.AudioAssets) != 1 {
		t.Errorf("one or more flat-list domains missing from export: %+v", cfg)
	}
}

// A real repository's ListAssets never populates Asset.Blob - it is a
// read-time join a caller must resolve separately (exactly like
// visualasset.Service.resolveBlob already does for every other read
// path). Without Export doing this itself, WriteArchive's own
// collectBlobRefs (which only ever looks at Asset.Blob) would silently
// skip every real asset's actual file content while still including
// its metadata row - a backup that looks complete but has quietly
// lost every image and sound.
func TestExportResolvesVisualAndAudioAssetBlobs(t *testing.T) {
	src := emptySources()
	src.VisualAssets = fakeVisualAssets{
		rows:  []visualasset.Asset{{ID: "asset_1", DisplayName: "logo.png", BlobSHA256: "sha_visual_1"}},
		blobs: map[string]visualasset.Blob{"sha_visual_1": {SHA256: "sha_visual_1", ByteSize: 4096}},
	}
	src.AudioAssets = fakeAudioAssets{
		rows:  []audioasset.Asset{{ID: "aud_1", DisplayName: "ding.wav", BlobSHA256: "sha_audio_1"}},
		blobs: map[string]audioasset.Blob{"sha_audio_1": {SHA256: "sha_audio_1", ByteSize: 2048}},
	}

	cfg, err := Export(context.Background(), src)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(cfg.VisualAssets) != 1 || cfg.VisualAssets[0].Blob == nil || cfg.VisualAssets[0].Blob.SHA256 != "sha_visual_1" {
		t.Fatalf("VisualAssets[0].Blob was not resolved: %+v", cfg.VisualAssets)
	}
	if len(cfg.AudioAssets) != 1 || cfg.AudioAssets[0].Blob == nil || cfg.AudioAssets[0].Blob.SHA256 != "sha_audio_1" {
		t.Fatalf("AudioAssets[0].Blob was not resolved: %+v", cfg.AudioAssets)
	}
}
