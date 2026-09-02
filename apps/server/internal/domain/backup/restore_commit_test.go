package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
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
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// fakeSinks is an in-memory Sinks implementation, mirroring
// emptySources' own fakes but for the write side - used to prove
// applyConfig/clearExisting/Restore's real behavior without a
// database.
type fakeSinks struct {
	platforms   map[string]platform.Platform
	output      map[string]output.Settings
	accounts    map[string]account.Account
	links       map[string][]account.Link
	integration map[account.ProviderID]account.IntegrationSettings
	regions     map[string]string
	engagement  map[string]engagementsettings.Settings

	operatorPrefs      operatorchatprefs.Preferences
	operatorVisibility map[string]bool
	operatorHidden     []operatorchatprefs.UserRef
	operatorBots       []operatorchatprefs.UserRef

	overlays        map[string]chatoverlay.Profile
	overlayAccounts map[string][]string
	overlayHidden   map[string][]chatoverlay.HiddenUser
	overlayBlocked  map[string][]chatoverlay.BlockedTerm
	overlayActivity map[string][]string

	schedules map[string]chatautomation.Schedule
	commands  map[string]chatautomation.Command

	alertProfiles map[string]alerts.Profile
	alertRules    map[string]alerts.Rule

	designs map[string]visualdesign.Record

	templates map[string]visualtemplate.Template

	visualBlobs     map[string]visualasset.Blob
	visualAssets    map[string]visualasset.Asset
	audioAssetBlobs map[string]audioasset.Blob
	audioAssets     map[string]audioasset.Asset

	audioSettings *audio.Settings

	goals   map[string]goals.Goal
	widgets map[string]goals.WidgetProfile

	presets map[string]metadatapreset.Preset

	streamSetups map[string]streamsetup.Profile

	donationSources map[string]donationsource.Source

	updatePrefs *updatersettings.Preferences
}

func newFakeSinks() *fakeSinks {
	return &fakeSinks{
		platforms: map[string]platform.Platform{}, output: map[string]output.Settings{},
		accounts: map[string]account.Account{}, links: map[string][]account.Link{},
		integration: map[account.ProviderID]account.IntegrationSettings{}, regions: map[string]string{},
		engagement: map[string]engagementsettings.Settings{}, operatorVisibility: map[string]bool{},
		overlays: map[string]chatoverlay.Profile{}, overlayAccounts: map[string][]string{},
		overlayHidden: map[string][]chatoverlay.HiddenUser{}, overlayBlocked: map[string][]chatoverlay.BlockedTerm{},
		overlayActivity: map[string][]string{}, schedules: map[string]chatautomation.Schedule{},
		commands: map[string]chatautomation.Command{}, alertProfiles: map[string]alerts.Profile{},
		alertRules: map[string]alerts.Rule{}, designs: map[string]visualdesign.Record{},
		templates: map[string]visualtemplate.Template{}, visualBlobs: map[string]visualasset.Blob{},
		visualAssets: map[string]visualasset.Asset{}, audioAssetBlobs: map[string]audioasset.Blob{},
		audioAssets: map[string]audioasset.Asset{}, goals: map[string]goals.Goal{},
		widgets: map[string]goals.WidgetProfile{}, presets: map[string]metadatapreset.Preset{},
		streamSetups:    map[string]streamsetup.Profile{},
		donationSources: map[string]donationsource.Source{},
	}
}

func (f *fakeSinks) sinks() Sinks {
	return Sinks{
		Platforms: fakePlatformSink{f}, Output: fakeOutputSink{f}, Accounts: fakeAccountSink{f},
		YouTubeRegion: fakeYouTubeRegionSink{f}, EngagementSettings: fakeEngagementSink{f},
		OperatorChatPrefs: fakeOperatorChatPrefsSink{f}, ChatOverlays: fakeChatOverlaySink{f},
		ChatAutomation: fakeChatAutomationSink{f}, Alerts: fakeAlertsSink{f},
		VisualDesigns: fakeVisualDesignSink{f}, VisualTemplates: fakeVisualTemplateSink{f},
		VisualAssets: fakeVisualAssetSink{f}, AudioAssets: fakeAudioAssetSink{f},
		AudioSettings: fakeAudioSettingsSink{f}, Goals: fakeGoalsSink{f},
		MetadataPresets: fakeMetadataPresetSink{f}, StreamSetupProfiles: fakeStreamSetupProfileSink{f},
		DonationSources:   fakeDonationSourceSink{f},
		UpdatePreferences: fakeUpdatePreferencesSink{f},
	}
}

func (f *fakeSinks) sources() Sources {
	return Sources{
		Platforms: fakePlatformsListOnly{f}, Output: fakeOutput{byPlatform: f.output},
		Accounts: fakeAccountsListOnly{f}, YouTubeRegion: fakeYouTubeRegion{byAccount: f.regions},
		EngagementSettings: fakeEngagementSettings{byAccount: f.engagement},
		OperatorChatPrefs:  fakeOperatorChatPrefsListOnly{f},
		ChatOverlays:       fakeChatOverlaysListOnly{f}, ChatAutomation: fakeChatAutomationListOnly{f},
		Alerts: fakeAlertsListOnly{f}, VisualDesigns: fakeVisualDesignsGetOnly{f},
		VisualTemplates: fakeVisualTemplatesListOnly{f}, VisualAssets: fakeVisualAssetsListOnly{f},
		AudioAssets: fakeAudioAssetsListOnly{f}, AudioSettings: fakeAudioSettingsGetOnly{f},
		Goals: fakeGoalsListOnly{f}, MetadataPresets: fakeMetadataPresetsListOnly{f},
		StreamSetupProfiles: fakeStreamSetupProfilesListOnly{f},
		DonationSources:     fakeDonationSourcesListOnly{f}, UpdatePreferences: fakeUpdatePreferencesGetOnly{f},
	}
}

// --- write-side (Sinks) adapters --------------------------------------------

type fakePlatformSink struct{ f *fakeSinks }

func (s fakePlatformSink) Create(_ context.Context, p platform.Platform) error {
	s.f.platforms[p.ID] = p
	return nil
}
func (s fakePlatformSink) Delete(_ context.Context, id string) error {
	delete(s.f.platforms, id)
	delete(s.f.output, id)
	return nil
}

type fakeOutputSink struct{ f *fakeSinks }

func (s fakeOutputSink) Update(_ context.Context, platformID string, in output.UpdateInput) (output.Settings, error) {
	settings := output.Settings{ServerURL: in.ServerURL, AutoRestart: in.AutoRestart}
	s.f.output[platformID] = settings
	return settings, nil
}

type fakeAccountSink struct{ f *fakeSinks }

func (s fakeAccountSink) CreateAccount(_ context.Context, a account.Account) error {
	s.f.accounts[a.ID] = a
	return nil
}
func (s fakeAccountSink) DeleteAccount(_ context.Context, id string) error {
	delete(s.f.accounts, id)
	delete(s.f.links, id)
	return nil
}
func (s fakeAccountSink) SetLink(_ context.Context, platformID, accountID string, now time.Time) (account.Link, error) {
	link := account.Link{PlatformID: platformID, AccountID: accountID, CreatedAt: now, UpdatedAt: now}
	s.f.links[accountID] = append(s.f.links[accountID], link)
	return link, nil
}
func (s fakeAccountSink) SetIntegrationSettings(_ context.Context, providerID account.ProviderID, clientID string, now time.Time) (account.IntegrationSettings, error) {
	is := account.IntegrationSettings{ProviderID: providerID, ClientID: clientID, UpdatedAt: now}
	s.f.integration[providerID] = is
	return is, nil
}

type fakeYouTubeRegionSink struct{ f *fakeSinks }

func (s fakeYouTubeRegionSink) SetRegion(_ context.Context, accountID, region string, _ time.Time) error {
	s.f.regions[accountID] = region
	return nil
}

type fakeEngagementSink struct{ f *fakeSinks }

func (s fakeEngagementSink) Set(_ context.Context, es engagementsettings.Settings, _ time.Time) (engagementsettings.Settings, error) {
	s.f.engagement[es.AccountID] = es
	return es, nil
}

type fakeOperatorChatPrefsSink struct{ f *fakeSinks }

func (s fakeOperatorChatPrefsSink) SetPreferences(_ context.Context, p operatorchatprefs.Preferences, _ time.Time) (operatorchatprefs.Preferences, error) {
	s.f.operatorPrefs = p
	return p, nil
}
func (s fakeOperatorChatPrefsSink) SetAccountVisibility(_ context.Context, accountID string, visible bool, _ time.Time) (operatorchatprefs.AccountVisibility, error) {
	s.f.operatorVisibility[accountID] = visible
	return operatorchatprefs.AccountVisibility{AccountID: accountID, Visible: visible}, nil
}
func (s fakeOperatorChatPrefsSink) AddHiddenUser(_ context.Context, ref operatorchatprefs.UserRef, _ time.Time) (operatorchatprefs.UserRef, error) {
	s.f.operatorHidden = append(s.f.operatorHidden, ref)
	return ref, nil
}
func (s fakeOperatorChatPrefsSink) AddBotUser(_ context.Context, ref operatorchatprefs.UserRef, _ time.Time) (operatorchatprefs.UserRef, error) {
	s.f.operatorBots = append(s.f.operatorBots, ref)
	return ref, nil
}

type fakeChatOverlaySink struct{ f *fakeSinks }

func (s fakeChatOverlaySink) CreateProfile(_ context.Context, p chatoverlay.Profile) (chatoverlay.Profile, error) {
	s.f.overlays[p.ID] = p
	return p, nil
}
func (s fakeChatOverlaySink) DeleteProfile(_ context.Context, id string) error {
	delete(s.f.overlays, id)
	delete(s.f.overlayAccounts, id)
	delete(s.f.overlayHidden, id)
	delete(s.f.overlayBlocked, id)
	delete(s.f.overlayActivity, id)
	return nil
}
func (s fakeChatOverlaySink) SetAccounts(_ context.Context, overlayID string, accountIDs []string) error {
	s.f.overlayAccounts[overlayID] = accountIDs
	return nil
}
func (s fakeChatOverlaySink) AddHiddenUser(_ context.Context, ref chatoverlay.HiddenUser, _ time.Time) (chatoverlay.HiddenUser, error) {
	s.f.overlayHidden[ref.OverlayID] = append(s.f.overlayHidden[ref.OverlayID], ref)
	return ref, nil
}
func (s fakeChatOverlaySink) AddBlockedTerm(_ context.Context, term chatoverlay.BlockedTerm, _ time.Time) (chatoverlay.BlockedTerm, error) {
	s.f.overlayBlocked[term.OverlayID] = append(s.f.overlayBlocked[term.OverlayID], term)
	return term, nil
}
func (s fakeChatOverlaySink) SetActivityTypes(_ context.Context, overlayID string, types []string) error {
	s.f.overlayActivity[overlayID] = types
	return nil
}

type fakeChatAutomationSink struct{ f *fakeSinks }

func (s fakeChatAutomationSink) CreateSchedule(_ context.Context, sc chatautomation.Schedule) (chatautomation.Schedule, error) {
	s.f.schedules[sc.ID] = sc
	return sc, nil
}
func (s fakeChatAutomationSink) CreateCommand(_ context.Context, c chatautomation.Command) (chatautomation.Command, error) {
	s.f.commands[c.ID] = c
	return c, nil
}
func (s fakeChatAutomationSink) DeleteSchedule(_ context.Context, id string) error {
	delete(s.f.schedules, id)
	return nil
}
func (s fakeChatAutomationSink) DeleteCommand(_ context.Context, id string) error {
	delete(s.f.commands, id)
	return nil
}

type fakeAlertsSink struct{ f *fakeSinks }

func (s fakeAlertsSink) CreateProfile(_ context.Context, p alerts.Profile) (alerts.Profile, error) {
	s.f.alertProfiles[p.ID] = p
	return p, nil
}
func (s fakeAlertsSink) CreateRule(_ context.Context, r alerts.Rule) (alerts.Rule, error) {
	s.f.alertRules[r.ID] = r
	return r, nil
}
func (s fakeAlertsSink) DeleteProfile(_ context.Context, id string) error {
	delete(s.f.alertProfiles, id)
	for rid, r := range s.f.alertRules {
		if r.ProfileID == id {
			delete(s.f.alertRules, rid)
		}
	}
	return nil
}

type fakeVisualDesignSink struct{ f *fakeSinks }

func (s fakeVisualDesignSink) Save(_ context.Context, ownerKind visualdesign.OwnerKind, ownerID string, doc visualdesign.Document, _ int, newID func() (string, error)) (visualdesign.Record, error) {
	id, err := newID()
	if err != nil {
		return visualdesign.Record{}, err
	}
	rec := visualdesign.Record{ID: id, OwnerKind: ownerKind, OwnerID: ownerID, Document: doc, Revision: 1}
	s.f.designs[string(ownerKind)+":"+ownerID] = rec
	return rec, nil
}
func (s fakeVisualDesignSink) Delete(_ context.Context, ownerKind visualdesign.OwnerKind, ownerID string) error {
	delete(s.f.designs, string(ownerKind)+":"+ownerID)
	return nil
}

type fakeVisualTemplateSink struct{ f *fakeSinks }

func (s fakeVisualTemplateSink) Create(_ context.Context, t visualtemplate.Template) (visualtemplate.Template, error) {
	s.f.templates[t.ID] = t
	return t, nil
}
func (s fakeVisualTemplateSink) Delete(_ context.Context, id string) error {
	delete(s.f.templates, id)
	return nil
}

type fakeVisualAssetSink struct{ f *fakeSinks }

func (s fakeVisualAssetSink) CreateBlob(_ context.Context, b visualasset.Blob) error {
	s.f.visualBlobs[b.SHA256] = b
	return nil
}
func (s fakeVisualAssetSink) CreateAsset(_ context.Context, a visualasset.Asset) error {
	s.f.visualAssets[a.ID] = a
	return nil
}
func (s fakeVisualAssetSink) DeleteAsset(_ context.Context, id string) error {
	delete(s.f.visualAssets, id)
	return nil
}

type fakeAudioAssetSink struct{ f *fakeSinks }

func (s fakeAudioAssetSink) CreateBlob(_ context.Context, b audioasset.Blob) error {
	s.f.audioAssetBlobs[b.SHA256] = b
	return nil
}
func (s fakeAudioAssetSink) CreateAsset(_ context.Context, a audioasset.Asset) error {
	s.f.audioAssets[a.ID] = a
	return nil
}
func (s fakeAudioAssetSink) DeleteAsset(_ context.Context, id string) error {
	delete(s.f.audioAssets, id)
	return nil
}

type fakeAudioSettingsSink struct{ f *fakeSinks }

func (s fakeAudioSettingsSink) SetSettings(_ context.Context, settings audio.Settings, _ time.Time) (audio.Settings, error) {
	s.f.audioSettings = &settings
	return settings, nil
}

type fakeGoalsSink struct{ f *fakeSinks }

func (s fakeGoalsSink) CreateGoal(_ context.Context, g goals.Goal) (goals.Goal, error) {
	s.f.goals[g.ID] = g
	return g, nil
}
func (s fakeGoalsSink) CreateWidgetProfile(_ context.Context, p goals.WidgetProfile) (goals.WidgetProfile, error) {
	s.f.widgets[p.ID] = p
	return p, nil
}
func (s fakeGoalsSink) UpdateWidgetProfile(_ context.Context, p goals.WidgetProfile) (goals.WidgetProfile, error) {
	s.f.widgets[p.ID] = p
	return p, nil
}
func (s fakeGoalsSink) DeleteGoal(_ context.Context, id string) error {
	delete(s.f.goals, id)
	return nil
}
func (s fakeGoalsSink) DeleteWidgetProfile(_ context.Context, id string) error {
	delete(s.f.widgets, id)
	return nil
}

type fakeMetadataPresetSink struct{ f *fakeSinks }

func (s fakeMetadataPresetSink) Create(_ context.Context, p metadatapreset.Preset) error {
	s.f.presets[p.ID] = p
	return nil
}
func (s fakeMetadataPresetSink) Delete(_ context.Context, id string) error {
	delete(s.f.presets, id)
	return nil
}

type fakeStreamSetupProfileSink struct{ f *fakeSinks }

func (s fakeStreamSetupProfileSink) Create(_ context.Context, p streamsetup.Profile) error {
	s.f.streamSetups[p.ID] = p
	return nil
}
func (s fakeStreamSetupProfileSink) Delete(_ context.Context, id string) error {
	delete(s.f.streamSetups, id)
	return nil
}

type fakeDonationSourceSink struct{ f *fakeSinks }

func (s fakeDonationSourceSink) CreateSource(_ context.Context, src donationsource.Source) error {
	s.f.donationSources[src.ID] = src
	return nil
}
func (s fakeDonationSourceSink) DeleteSource(_ context.Context, id string) error {
	delete(s.f.donationSources, id)
	return nil
}

type fakeUpdatePreferencesSink struct{ f *fakeSinks }

func (s fakeUpdatePreferencesSink) SetPreferences(_ context.Context, p updatersettings.Preferences, _ time.Time) (updatersettings.Preferences, error) {
	s.f.updatePrefs = &p
	return p, nil
}

// --- read-side (Sources) adapters over the same fakeSinks state, used so
// clearExisting can "list what currently exists" against the same fake
// database this test drives -----------------------------------------------

type fakePlatformsListOnly struct{ f *fakeSinks }

func (s fakePlatformsListOnly) List(context.Context) ([]platform.Platform, error) {
	out := make([]platform.Platform, 0, len(s.f.platforms))
	for _, p := range s.f.platforms {
		out = append(out, p)
	}
	return out, nil
}

type fakeAccountsListOnly struct{ f *fakeSinks }

func (s fakeAccountsListOnly) ListAccounts(context.Context) ([]account.Account, error) {
	out := make([]account.Account, 0, len(s.f.accounts))
	for _, a := range s.f.accounts {
		out = append(out, a)
	}
	return out, nil
}
func (s fakeAccountsListOnly) ListLinksByAccount(_ context.Context, accountID string) ([]account.Link, error) {
	return s.f.links[accountID], nil
}
func (s fakeAccountsListOnly) GetIntegrationSettings(_ context.Context, providerID account.ProviderID) (account.IntegrationSettings, bool, error) {
	is, ok := s.f.integration[providerID]
	return is, ok, nil
}

type fakeOperatorChatPrefsListOnly struct{ f *fakeSinks }

func (s fakeOperatorChatPrefsListOnly) GetPreferences(context.Context) (operatorchatprefs.Preferences, bool, error) {
	return s.f.operatorPrefs, true, nil
}
func (s fakeOperatorChatPrefsListOnly) ListAccountVisibility(context.Context) ([]operatorchatprefs.AccountVisibility, error) {
	return nil, nil
}
func (s fakeOperatorChatPrefsListOnly) ListHiddenUsers(context.Context) ([]operatorchatprefs.UserRef, error) {
	return s.f.operatorHidden, nil
}
func (s fakeOperatorChatPrefsListOnly) ListBotUsers(context.Context) ([]operatorchatprefs.UserRef, error) {
	return s.f.operatorBots, nil
}

type fakeChatOverlaysListOnly struct{ f *fakeSinks }

func (s fakeChatOverlaysListOnly) ListProfiles(context.Context) ([]chatoverlay.Profile, error) {
	out := make([]chatoverlay.Profile, 0, len(s.f.overlays))
	for _, o := range s.f.overlays {
		out = append(out, o)
	}
	return out, nil
}
func (s fakeChatOverlaysListOnly) ListAccounts(_ context.Context, overlayID string) ([]string, error) {
	return s.f.overlayAccounts[overlayID], nil
}
func (s fakeChatOverlaysListOnly) ListHiddenUsers(_ context.Context, overlayID string) ([]chatoverlay.HiddenUser, error) {
	return s.f.overlayHidden[overlayID], nil
}
func (s fakeChatOverlaysListOnly) ListBlockedTerms(_ context.Context, overlayID string) ([]chatoverlay.BlockedTerm, error) {
	return s.f.overlayBlocked[overlayID], nil
}
func (s fakeChatOverlaysListOnly) ListActivityTypes(_ context.Context, overlayID string) ([]string, error) {
	return s.f.overlayActivity[overlayID], nil
}

type fakeChatAutomationListOnly struct{ f *fakeSinks }

func (s fakeChatAutomationListOnly) ListSchedules(context.Context) ([]chatautomation.Schedule, error) {
	out := make([]chatautomation.Schedule, 0, len(s.f.schedules))
	for _, sc := range s.f.schedules {
		out = append(out, sc)
	}
	return out, nil
}
func (s fakeChatAutomationListOnly) ListCommands(context.Context) ([]chatautomation.Command, error) {
	out := make([]chatautomation.Command, 0, len(s.f.commands))
	for _, c := range s.f.commands {
		out = append(out, c)
	}
	return out, nil
}

type fakeAlertsListOnly struct{ f *fakeSinks }

func (s fakeAlertsListOnly) ListProfiles(context.Context) ([]alerts.Profile, error) {
	out := make([]alerts.Profile, 0, len(s.f.alertProfiles))
	for _, p := range s.f.alertProfiles {
		out = append(out, p)
	}
	return out, nil
}
func (s fakeAlertsListOnly) ListRules(_ context.Context, profileID string) ([]alerts.Rule, error) {
	out := []alerts.Rule{}
	for _, r := range s.f.alertRules {
		if r.ProfileID == profileID {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeVisualDesignsGetOnly struct{ f *fakeSinks }

func (s fakeVisualDesignsGetOnly) Get(_ context.Context, ownerKind visualdesign.OwnerKind, ownerID string) (visualdesign.Record, bool, error) {
	r, ok := s.f.designs[string(ownerKind)+":"+ownerID]
	return r, ok, nil
}

type fakeVisualTemplatesListOnly struct{ f *fakeSinks }

func (s fakeVisualTemplatesListOnly) List(context.Context) ([]visualtemplate.Template, error) {
	out := make([]visualtemplate.Template, 0, len(s.f.templates))
	for _, t := range s.f.templates {
		out = append(out, t)
	}
	return out, nil
}

type fakeVisualAssetsListOnly struct{ f *fakeSinks }

func (s fakeVisualAssetsListOnly) ListAssets(context.Context) ([]visualasset.Asset, error) {
	out := make([]visualasset.Asset, 0, len(s.f.visualAssets))
	for _, a := range s.f.visualAssets {
		out = append(out, a)
	}
	return out, nil
}

func (s fakeVisualAssetsListOnly) GetBlob(_ context.Context, sha256Hex string) (visualasset.Blob, bool, error) {
	b, ok := s.f.visualBlobs[sha256Hex]
	return b, ok, nil
}

type fakeAudioAssetsListOnly struct{ f *fakeSinks }

func (s fakeAudioAssetsListOnly) ListAssets(context.Context) ([]audioasset.Asset, error) {
	out := make([]audioasset.Asset, 0, len(s.f.audioAssets))
	for _, a := range s.f.audioAssets {
		out = append(out, a)
	}
	return out, nil
}

func (s fakeAudioAssetsListOnly) GetBlob(_ context.Context, sha256Hex string) (audioasset.Blob, bool, error) {
	b, ok := s.f.audioAssetBlobs[sha256Hex]
	return b, ok, nil
}

type fakeAudioSettingsGetOnly struct{ f *fakeSinks }

func (s fakeAudioSettingsGetOnly) GetSettings(context.Context) (audio.Settings, bool, error) {
	if s.f.audioSettings == nil {
		return audio.Settings{}, false, nil
	}
	return *s.f.audioSettings, true, nil
}

type fakeGoalsListOnly struct{ f *fakeSinks }

func (s fakeGoalsListOnly) ListGoals(context.Context) ([]goals.Goal, error) {
	out := make([]goals.Goal, 0, len(s.f.goals))
	for _, g := range s.f.goals {
		out = append(out, g)
	}
	return out, nil
}
func (s fakeGoalsListOnly) ListWidgetProfiles(_ context.Context, goalID string) ([]goals.WidgetProfile, error) {
	out := []goals.WidgetProfile{}
	for _, w := range s.f.widgets {
		if goalID == "" || w.GoalID == goalID {
			out = append(out, w)
		}
	}
	return out, nil
}

type fakeMetadataPresetsListOnly struct{ f *fakeSinks }

func (s fakeMetadataPresetsListOnly) List(context.Context) ([]metadatapreset.Preset, error) {
	out := make([]metadatapreset.Preset, 0, len(s.f.presets))
	for _, p := range s.f.presets {
		out = append(out, p)
	}
	return out, nil
}

type fakeStreamSetupProfilesListOnly struct{ f *fakeSinks }

func (s fakeStreamSetupProfilesListOnly) List(context.Context) ([]streamsetup.Profile, error) {
	out := make([]streamsetup.Profile, 0, len(s.f.streamSetups))
	for _, p := range s.f.streamSetups {
		out = append(out, p)
	}
	return out, nil
}

type fakeDonationSourcesListOnly struct{ f *fakeSinks }

func (s fakeDonationSourcesListOnly) ListSources(context.Context) ([]donationsource.Source, error) {
	out := make([]donationsource.Source, 0, len(s.f.donationSources))
	for _, src := range s.f.donationSources {
		out = append(out, src)
	}
	return out, nil
}

type fakeUpdatePreferencesGetOnly struct{ f *fakeSinks }

func (s fakeUpdatePreferencesGetOnly) GetPreferences(context.Context) (updatersettings.Preferences, bool, error) {
	if s.f.updatePrefs == nil {
		return updatersettings.Preferences{}, false, nil
	}
	return *s.f.updatePrefs, true, nil
}

// --- tests -------------------------------------------------------------

func TestApplyConfigRemapsCrossDomainReferences(t *testing.T) {
	sinks := newFakeSinks()
	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms: []PlatformExport{
			{Platform: platform.Platform{ID: "pf_old_1", DisplayName: "Main"}},
		},
		ConnectedAccounts: []ConnectedAccountExport{
			{Account: account.Account{ID: "acct_old_1", ProviderID: account.ProviderTwitch}, PlatformLinks: []string{"pf_old_1"}},
		},
		AlertProfiles: []AlertProfileExport{
			{
				Profile: alerts.Profile{ID: "alp_old_1", Name: "Main alerts"},
				Rules:   []alerts.Rule{{ID: "alr_old_1", Name: "Follow", Accounts: []string{"acct_old_1"}}},
			},
		},
	}

	err := applyConfig(context.Background(), cfg, map[string][]byte{}, sinks.sinks(), fakeBlobWriter{}, fakeBlobWriter{}, func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("applyConfig() error = %v", err)
	}

	if len(sinks.platforms) != 1 {
		t.Fatalf("got %d platforms, want 1", len(sinks.platforms))
	}
	var newPlatformID string
	for id := range sinks.platforms {
		newPlatformID = id
	}
	if newPlatformID == "pf_old_1" {
		t.Error("platform id was not remapped - restore must never reuse a backup-supplied id")
	}

	if len(sinks.accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(sinks.accounts))
	}
	var newAccountID string
	for id, a := range sinks.accounts {
		newAccountID = id
		if a.Status != account.StatusReconnectRequired {
			t.Errorf("restored account status = %q, want reconnect_required", a.Status)
		}
	}
	if newAccountID == "acct_old_1" {
		t.Error("account id was not remapped")
	}

	links := sinks.links[newAccountID]
	if len(links) != 1 || links[0].PlatformID != newPlatformID {
		t.Errorf("account link was not remapped to the new platform id: %+v (want platform %s)", links, newPlatformID)
	}

	if len(sinks.alertRules) != 1 {
		t.Fatalf("got %d alert rules, want 1", len(sinks.alertRules))
	}
	for _, r := range sinks.alertRules {
		if len(r.Accounts) != 1 || r.Accounts[0] != newAccountID {
			t.Errorf("alert rule Accounts = %v, want [%s] (remapped)", r.Accounts, newAccountID)
		}
		if r.ProfileID == "alp_old_1" {
			t.Error("alert rule ProfileID was not remapped")
		}
	}
}

type fakeBlobWriter struct{}

func (fakeBlobWriter) WriteBlob(r io.Reader, maxBytes int64) (string, int64, error) {
	sum := sha256.Sum256(nil)
	data, _ := io.ReadAll(r)
	if len(data) > 0 {
		sum = sha256.Sum256(data)
	}
	return hex.EncodeToString(sum[:]), int64(len(data)), nil
}

func TestClearExistingRemovesEveryIncludedDomain(t *testing.T) {
	sinks := newFakeSinks()
	sinks.platforms["pf_1"] = platform.Platform{ID: "pf_1"}
	sinks.accounts["acct_1"] = account.Account{ID: "acct_1"}
	sinks.alertProfiles["alp_1"] = alerts.Profile{ID: "alp_1"}
	sinks.alertRules["alr_1"] = alerts.Rule{ID: "alr_1", ProfileID: "alp_1"}
	sinks.overlays["ov_1"] = chatoverlay.Profile{ID: "ov_1"}
	sinks.goals["goal_1"] = goals.Goal{ID: "goal_1"}
	sinks.widgets["wp_1"] = goals.WidgetProfile{ID: "wp_1", GoalID: "goal_1"}
	sinks.presets["mp_1"] = metadatapreset.Preset{ID: "mp_1"}
	sinks.streamSetups["setup_1"] = streamsetup.Profile{ID: "setup_1"}
	sinks.donationSources["donsrc_1"] = donationsource.Source{ID: "donsrc_1"}

	if err := clearExisting(context.Background(), sinks.sources(), sinks.sinks()); err != nil {
		t.Fatalf("clearExisting() error = %v", err)
	}

	if len(sinks.platforms) != 0 || len(sinks.accounts) != 0 || len(sinks.alertProfiles) != 0 ||
		len(sinks.alertRules) != 0 || len(sinks.overlays) != 0 || len(sinks.goals) != 0 ||
		len(sinks.widgets) != 0 || len(sinks.presets) != 0 || len(sinks.streamSetups) != 0 ||
		len(sinks.donationSources) != 0 {
		t.Errorf("clearExisting left rows behind: %+v", sinks)
	}
}

// TestApplyConfigRemapsStreamSetupProfileReferences proves the fix to
// the bug this stage found: metadata-preset restore previously never
// recorded its own old->new id into ids, so a stream setup profile -
// the first thing to reference a metadata preset id across domains -
// would have restored with a dangling, un-remapped preset reference.
// Also proves a destination's PlatformID remaps to the platform's own
// new id, and that an already-missing destination/preset reference
// (nil PlatformID / nil MetadataPresetID with only the name snapshot
// carried) survives restore unchanged rather than being dropped or
// resurrected.
func TestApplyConfigRemapsStreamSetupProfileReferences(t *testing.T) {
	sinks := newFakeSinks()
	pid := "pf_old_1"
	presetID := "mp_old_1"
	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms: []PlatformExport{
			{Platform: platform.Platform{ID: "pf_old_1", DisplayName: "Main"}},
		},
		MetadataPresets: []metadatapreset.Preset{
			{ID: "mp_old_1", Name: "Gaming preset"},
		},
		StreamSetupProfiles: []streamsetup.Profile{
			{
				ID: "setup_old_1", Name: "Gaming",
				Destinations: []streamsetup.Destination{
					{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main"},
					{PlatformID: nil, ProviderID: "youtube", DisplayName: "Deleted destination"},
				},
				MetadataPresetID: &presetID, MetadataPresetName: "Gaming preset",
			},
			{
				ID: "setup_old_2", Name: "Already missing preset",
				MetadataPresetID: nil, MetadataPresetName: "Long-gone preset",
			},
		},
	}

	err := applyConfig(context.Background(), cfg, map[string][]byte{}, sinks.sinks(), fakeBlobWriter{}, fakeBlobWriter{}, func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("applyConfig() error = %v", err)
	}

	if len(sinks.platforms) != 1 || len(sinks.presets) != 1 || len(sinks.streamSetups) != 2 {
		t.Fatalf("got %d platforms, %d presets, %d stream setups, want 1, 1, 2", len(sinks.platforms), len(sinks.presets), len(sinks.streamSetups))
	}
	var newPlatformID, newPresetID string
	for id := range sinks.platforms {
		newPlatformID = id
	}
	for id := range sinks.presets {
		newPresetID = id
	}

	var gaming, missingPreset streamsetup.Profile
	for _, p := range sinks.streamSetups {
		if p.Name == "Gaming" {
			gaming = p
		} else {
			missingPreset = p
		}
	}

	if gaming.ID == "setup_old_1" {
		t.Error("stream setup profile id was not remapped")
	}
	if gaming.MetadataPresetID == nil || *gaming.MetadataPresetID != newPresetID {
		t.Errorf("MetadataPresetID = %v, want the remapped preset id %q", gaming.MetadataPresetID, newPresetID)
	}
	if len(gaming.Destinations) != 2 {
		t.Fatalf("got %d destinations, want 2", len(gaming.Destinations))
	}
	if gaming.Destinations[0].PlatformID == nil || *gaming.Destinations[0].PlatformID != newPlatformID {
		t.Errorf("Destinations[0].PlatformID = %v, want the remapped platform id %q", gaming.Destinations[0].PlatformID, newPlatformID)
	}
	if gaming.Destinations[1].PlatformID != nil {
		t.Errorf("Destinations[1].PlatformID = %v, want nil (was already missing before backup)", *gaming.Destinations[1].PlatformID)
	}
	if gaming.Destinations[1].DisplayName != "Deleted destination" {
		t.Errorf("Destinations[1].DisplayName = %q, want the snapshot preserved", gaming.Destinations[1].DisplayName)
	}

	if missingPreset.MetadataPresetID != nil {
		t.Errorf("MetadataPresetID = %v, want nil (was already missing before backup)", *missingPreset.MetadataPresetID)
	}
	if missingPreset.MetadataPresetName != "Long-gone preset" {
		t.Errorf("MetadataPresetName = %q, want the snapshot preserved", missingPreset.MetadataPresetName)
	}
	if !missingPreset.MetadataPresetMissing() {
		t.Error("MetadataPresetMissing() = false, want true")
	}
}
