package backup

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// clearExisting deletes every currently-configured row in every
// domain this backup replaces, in a child-before-parent order safe
// for this schema's real foreign keys (docs/backup-restore.md §7 step
// "REPLACE CONFIGURATION FROM BACKUP"). Reads the CURRENT state
// through Sources - the exact same read path Export already uses - so
// this never depends on a second, parallel notion of "what exists".
//
// Deliberately does NOT delete managed-asset blob files/rows: the
// existing startup reconciliation (*visualasset.Service.Reconcile,
// already run once at every backend start) already finds and removes
// any blob no asset row references any more - reusing it here avoids
// a second, restore-specific blob-cleanup implementation for a
// low-value cleanup (a few orphaned files) that is not itself a
// correctness or security concern.
func clearExisting(ctx context.Context, src Sources, sink Sinks) error {
	// Visual designs first: no real SQL foreign key ties them to their
	// polymorphic owner (docs/backup-restore.md's own audit, migration
	// 0015's comment), so they must be deleted explicitly rather than
	// relying on an owner's own cascade.
	if overlays, err := src.ChatOverlays.ListProfiles(ctx); err == nil {
		for _, o := range overlays {
			_ = sink.VisualDesigns.Delete(ctx, visualdesign.OwnerKindChatOverlay, o.ID)
		}
	} else {
		return fmt.Errorf("list chat overlays for clear: %w", err)
	}
	if profiles, err := src.Alerts.ListProfiles(ctx); err == nil {
		for _, p := range profiles {
			rules, err := src.Alerts.ListRules(ctx, p.ID)
			if err != nil {
				return fmt.Errorf("list alert rules for clear: %w", err)
			}
			for _, ru := range rules {
				_ = sink.VisualDesigns.Delete(ctx, visualdesign.OwnerKindAlertRule, ru.ID)
			}
		}
	} else {
		return fmt.Errorf("list alert profiles for clear: %w", err)
	}

	// Widget profiles before goals: goals.DeleteGoal refuses while a
	// widget profile still references it (ErrGoalInUse).
	if err := deleteAll(ctx, func() ([]goals.WidgetProfile, error) { return src.Goals.ListWidgetProfiles(ctx, "") },
		func(id string) error { return sink.Goals.DeleteWidgetProfile(ctx, id) },
		func(p goals.WidgetProfile) string { return p.ID }); err != nil {
		return err
	}
	if err := deleteAll(ctx, func() ([]goals.Goal, error) { return src.Goals.ListGoals(ctx) },
		func(id string) error { return sink.Goals.DeleteGoal(ctx, id) },
		func(g goals.Goal) string { return g.ID }); err != nil {
		return err
	}

	if err := deleteAll(ctx, func() ([]donationsource.Source, error) { return src.DonationSources.ListSources(ctx) },
		func(id string) error { return sink.DonationSources.DeleteSource(ctx, id) },
		func(s donationsource.Source) string { return s.ID }); err != nil {
		return err
	}

	if err := deleteAll(ctx, func() ([]chatoverlay.Profile, error) { return src.ChatOverlays.ListProfiles(ctx) },
		func(id string) error { return sink.ChatOverlays.DeleteProfile(ctx, id) },
		func(p chatoverlay.Profile) string { return p.ID }); err != nil {
		return err
	}

	if schedules, err := src.ChatAutomation.ListSchedules(ctx); err == nil {
		for _, s := range schedules {
			_ = sink.ChatAutomation.DeleteSchedule(ctx, s.ID)
		}
	} else {
		return fmt.Errorf("list chat schedules for clear: %w", err)
	}
	if commands, err := src.ChatAutomation.ListCommands(ctx); err == nil {
		for _, c := range commands {
			_ = sink.ChatAutomation.DeleteCommand(ctx, c.ID)
		}
	} else {
		return fmt.Errorf("list chat commands for clear: %w", err)
	}

	if profiles, err := src.Alerts.ListProfiles(ctx); err == nil {
		for _, p := range profiles {
			// Deleting the profile cascades its own rules
			// (alert_rules ON DELETE CASCADE, migration 0013).
			if err := sink.Alerts.DeleteProfile(ctx, p.ID); err != nil {
				return fmt.Errorf("delete alert profile %s: %w", p.ID, err)
			}
		}
	}

	if templates, err := src.VisualTemplates.List(ctx); err == nil {
		for _, t := range templates {
			_ = sink.VisualTemplates.Delete(ctx, t.ID)
		}
	} else {
		return fmt.Errorf("list visual templates for clear: %w", err)
	}

	if visualAssets, err := src.VisualAssets.ListAssets(ctx); err == nil {
		for _, a := range visualAssets {
			_ = sink.VisualAssets.DeleteAsset(ctx, a.ID)
		}
	} else {
		return fmt.Errorf("list visual assets for clear: %w", err)
	}
	if audioAssets, err := src.AudioAssets.ListAssets(ctx); err == nil {
		for _, a := range audioAssets {
			_ = sink.AudioAssets.DeleteAsset(ctx, a.ID)
		}
	} else {
		return fmt.Errorf("list audio assets for clear: %w", err)
	}

	if presets, err := src.MetadataPresets.List(ctx); err == nil {
		for _, p := range presets {
			_ = sink.MetadataPresets.Delete(ctx, p.ID)
		}
	} else {
		return fmt.Errorf("list metadata presets for clear: %w", err)
	}

	// Accounts before platforms: platform_account_links references
	// both, but deleting an account first (cascading its own links) is
	// simplest; either order is safe since both rows are being removed
	// in the same clear pass regardless.
	if err := deleteAll(ctx, func() ([]account.Account, error) { return src.Accounts.ListAccounts(ctx) },
		func(id string) error { return sink.Accounts.DeleteAccount(ctx, id) },
		func(a account.Account) string { return a.ID }); err != nil {
		return err
	}
	if err := deleteAll(ctx, func() ([]platform.Platform, error) { return src.Platforms.List(ctx) },
		func(id string) error { return sink.Platforms.Delete(ctx, id) },
		func(p platform.Platform) string { return p.ID }); err != nil {
		return err
	}

	return nil
}

// deleteAll is a small generic helper: list every current row through
// list, then delete each one by its own id through del - used for
// every domain whose clear step is this exact shape.
func deleteAll[T any](_ context.Context, list func() ([]T, error), del func(id string) error, idOf func(T) string) error {
	rows, err := list()
	if err != nil {
		return fmt.Errorf("list rows for clear: %w", err)
	}
	for _, row := range rows {
		if err := del(idOf(row)); err != nil {
			return fmt.Errorf("delete row %s for clear: %w", idOf(row), err)
		}
	}
	return nil
}

// applyConfig inserts every object in cfg, minting a fresh local id
// for every restored row via each domain's own real NewID generator
// (docs/backup-restore.md §4 - never a backup-supplied id used as a
// literal local primary key) and remapping every cross-domain
// reference through ids as it goes, in dependency order. Assumes
// clearExisting has already run - restore is REPLACE, not merge.
//
// assets holds the validated asset bytes ReadArchive already checked
// (keyed by sha256) - never taken from cfg itself, since Config (the
// config.json payload) carries only metadata, never raw bytes.
func applyConfig(ctx context.Context, cfg Config, assets map[string][]byte, sink Sinks, visualBlobs, audioBlobs BlobWriter, now func() time.Time) error {
	ids := idMap{}

	// --- platforms + output settings ------------------------------------
	for _, pe := range cfg.Platforms {
		newID, err := platform.NewID()
		if err != nil {
			return fmt.Errorf("generate platform id: %w", err)
		}
		ids[pe.Platform.ID] = newID
		t := now().UTC()
		p := pe.Platform
		p.ID = newID
		p.CreatedAt, p.UpdatedAt = t, t
		if err := sink.Platforms.Create(ctx, p); err != nil {
			return fmt.Errorf("restore platform: %w", err)
		}
		if _, err := sink.Output.Update(ctx, newID, output.UpdateInput{
			ServerURL: pe.Output.ServerURL, AutoRestart: pe.Output.AutoRestart,
		}); err != nil {
			return fmt.Errorf("restore platform output settings: %w", err)
		}
	}

	// --- provider integration settings (Twitch/YouTube client id) ------
	for _, s := range cfg.ProviderIntegrationSettings {
		if _, err := sink.Accounts.SetIntegrationSettings(ctx, s.ProviderID, s.ClientID, now()); err != nil {
			return fmt.Errorf("restore provider integration settings: %w", err)
		}
	}

	// --- connected accounts ---------------------------------------------
	// Status is ALWAYS forced to reconnect_required regardless of what
	// the backup recorded (docs/backup-restore.md §8/§11) - a restored
	// account is never represented as healthy merely because its row
	// exists, since its OAuth token bundle was never exported.
	for _, ae := range cfg.ConnectedAccounts {
		newID, err := account.NewID()
		if err != nil {
			return fmt.Errorf("generate account id: %w", err)
		}
		ids[ae.Account.ID] = newID
		t := now().UTC()
		a := ae.Account
		a.ID = newID
		a.Status = account.StatusReconnectRequired
		a.LastValidatedAt = nil
		a.CreatedAt, a.UpdatedAt = t, t
		if err := sink.Accounts.CreateAccount(ctx, a); err != nil {
			return fmt.Errorf("restore connected account: %w", err)
		}
		for _, oldPlatformID := range ae.PlatformLinks {
			if _, err := sink.Accounts.SetLink(ctx, ids.remap(oldPlatformID), newID, t); err != nil {
				return fmt.Errorf("restore platform account link: %w", err)
			}
		}
		if ae.YouTubeRegion != "" {
			if err := sink.YouTubeRegion.SetRegion(ctx, newID, ae.YouTubeRegion, t); err != nil {
				return fmt.Errorf("restore youtube region: %w", err)
			}
		}
		if ae.EngagementEnabled != nil {
			if _, err := sink.EngagementSettings.Set(ctx, engagementsettings.Settings{
				AccountID: newID, Enabled: *ae.EngagementEnabled,
			}, t); err != nil {
				return fmt.Errorf("restore engagement settings: %w", err)
			}
		}
	}

	// --- donation sources (never enabled automatically - no credential
	// ever transfers) -----------------------------------------------------
	for _, s := range cfg.DonationSources {
		newID, err := donationsource.NewID()
		if err != nil {
			return fmt.Errorf("generate donation source id: %w", err)
		}
		ids[s.ID] = newID
		t := now().UTC()
		s.ID = newID
		s.Enabled = false
		s.CreatedAt, s.UpdatedAt = t, t
		if err := sink.DonationSources.CreateSource(ctx, s); err != nil {
			return fmt.Errorf("restore donation source: %w", err)
		}
	}

	// --- chat overlays -----------------------------------------------------
	for _, oe := range cfg.ChatOverlays {
		newID, err := chatoverlay.NewID()
		if err != nil {
			return fmt.Errorf("generate chat overlay id: %w", err)
		}
		ids[oe.Profile.ID] = newID
		t := now().UTC()
		p := oe.Profile
		p.ID = newID
		// PublicSlug is preserved verbatim (docs/backup-restore.md §3) -
		// an "unguessable locator, not a credential", so existing OBS
		// Browser Sources keep working after restore.
		p.CreatedAt, p.UpdatedAt = t, t
		if _, err := sink.ChatOverlays.CreateProfile(ctx, p); err != nil {
			return fmt.Errorf("restore chat overlay: %w", err)
		}
		if len(oe.AccountIDs) > 0 {
			if err := sink.ChatOverlays.SetAccounts(ctx, newID, ids.remapAll(oe.AccountIDs)); err != nil {
				return fmt.Errorf("restore chat overlay accounts: %w", err)
			}
		}
		for _, hu := range oe.HiddenUsers {
			hu.OverlayID = newID
			hu.ConnectedAccountID = ids.remap(hu.ConnectedAccountID)
			if _, err := sink.ChatOverlays.AddHiddenUser(ctx, hu, t); err != nil {
				return fmt.Errorf("restore chat overlay hidden user: %w", err)
			}
		}
		for _, bt := range oe.BlockedTerms {
			bt.OverlayID = newID
			if _, err := sink.ChatOverlays.AddBlockedTerm(ctx, bt, t); err != nil {
				return fmt.Errorf("restore chat overlay blocked term: %w", err)
			}
		}
		if len(oe.ActivityTypes) > 0 {
			if err := sink.ChatOverlays.SetActivityTypes(ctx, newID, oe.ActivityTypes); err != nil {
				return fmt.Errorf("restore chat overlay activity types: %w", err)
			}
		}
	}

	// --- chat automation ---------------------------------------------------
	for _, s := range cfg.ChatSchedules {
		s.Targets = remapChatAutomationTargets(s.Targets, ids)
		t := now().UTC()
		s.CreatedAt, s.UpdatedAt = t, t
		if _, err := sink.ChatAutomation.CreateSchedule(ctx, s); err != nil {
			return fmt.Errorf("restore chat schedule: %w", err)
		}
	}
	for _, c := range cfg.ChatCommands {
		c.Targets = remapChatAutomationTargets(c.Targets, ids)
		t := now().UTC()
		c.CreatedAt, c.UpdatedAt = t, t
		if _, err := sink.ChatAutomation.CreateCommand(ctx, c); err != nil {
			return fmt.Errorf("restore chat command: %w", err)
		}
	}

	// --- alert profiles + rules ---------------------------------------------
	for _, ape := range cfg.AlertProfiles {
		newProfileID, err := alerts.NewProfileID()
		if err != nil {
			return fmt.Errorf("generate alert profile id: %w", err)
		}
		ids[ape.Profile.ID] = newProfileID
		t := now().UTC()
		prof := ape.Profile
		prof.ID = newProfileID
		// PublicSlug preserved verbatim, same reasoning as chat overlays.
		prof.CreatedAt, prof.UpdatedAt = t, t
		if _, err := sink.Alerts.CreateProfile(ctx, prof); err != nil {
			return fmt.Errorf("restore alert profile: %w", err)
		}

		for _, ru := range ape.Rules {
			newRuleID, err := alerts.NewRuleID()
			if err != nil {
				return fmt.Errorf("generate alert rule id: %w", err)
			}
			ids[ru.ID] = newRuleID
			ru.ID = newRuleID
			ru.ProfileID = newProfileID
			ru.Accounts = ids.remapAll(ru.Accounts)
			ru.Audio.SoundAssetID = ids.remap(ru.Audio.SoundAssetID)
			ru.CreatedAt, ru.UpdatedAt = t, t
			if _, err := sink.Alerts.CreateRule(ctx, ru); err != nil {
				return fmt.Errorf("restore alert rule: %w", err)
			}
		}
	}

	// --- visual designs (owner ids remapped to the new alert-rule/
	// chat-overlay ids just minted above) ------------------------------------
	for _, d := range cfg.VisualDesigns {
		newOwnerID := ids.remap(d.OwnerID)
		if _, err := sink.VisualDesigns.Save(ctx, d.OwnerKind, newOwnerID, d.Document, 0, visualdesign.NewDesignID); err != nil {
			return fmt.Errorf("restore visual design: %w", err)
		}
	}

	// --- visual templates ---------------------------------------------------
	for _, tmpl := range cfg.VisualTemplates {
		newID, err := visualtemplate.NewTemplateID()
		if err != nil {
			return fmt.Errorf("generate visual template id: %w", err)
		}
		ids[tmpl.ID] = newID
		t := now().UTC()
		tmpl.ID = newID
		if tmpl.AlertAudio != nil {
			tmpl.AlertAudio.SoundAssetID = ids.remap(tmpl.AlertAudio.SoundAssetID)
		}
		tmpl.CreatedAt, tmpl.UpdatedAt = t, t
		if _, err := sink.VisualTemplates.Create(ctx, tmpl); err != nil {
			return fmt.Errorf("restore visual template: %w", err)
		}
	}

	// --- managed visual/audio assets -----------------------------------------
	for _, a := range cfg.VisualAssets {
		if a.Blob == nil {
			continue
		}
		data, ok := assets[a.Blob.SHA256]
		if !ok {
			return fmt.Errorf("%w: visual asset %s references a blob not present in the package", ErrAssetMissing, a.ID)
		}
		sha, size, err := visualBlobs.WriteBlob(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("write visual asset blob: %w", err)
		}
		token, err := visualasset.NewPublicToken()
		if err != nil {
			return fmt.Errorf("generate visual asset public token: %w", err)
		}
		t := now().UTC()
		if err := sink.VisualAssets.CreateBlob(ctx, visualasset.Blob{
			SHA256: sha, MediaType: a.Blob.MediaType, ByteSize: size, StorageName: sha,
			PublicToken: token, CreatedAt: t,
		}); err != nil {
			return fmt.Errorf("restore visual asset blob row: %w", err)
		}
		newID, err := visualasset.NewAssetID()
		if err != nil {
			return fmt.Errorf("generate visual asset id: %w", err)
		}
		ids[a.ID] = newID
		if err := sink.VisualAssets.CreateAsset(ctx, visualasset.Asset{
			ID: newID, BlobSHA256: sha, Kind: a.Kind, DisplayName: a.DisplayName,
			Author: a.Author, License: a.License, Notice: a.Notice, Source: a.Source,
			CreatedAt: t, UpdatedAt: t,
		}); err != nil {
			return fmt.Errorf("restore visual asset: %w", err)
		}
	}

	for _, a := range cfg.AudioAssets {
		if a.Blob == nil {
			continue
		}
		data, ok := assets[a.Blob.SHA256]
		if !ok {
			return fmt.Errorf("%w: audio asset %s references a blob not present in the package", ErrAssetMissing, a.ID)
		}
		sha, size, err := audioBlobs.WriteBlob(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("write audio asset blob: %w", err)
		}
		token, err := audioasset.NewPublicToken()
		if err != nil {
			return fmt.Errorf("generate audio asset public token: %w", err)
		}
		t := now().UTC()
		if err := sink.AudioAssets.CreateBlob(ctx, audioasset.Blob{
			SHA256: sha, MediaType: a.Blob.MediaType, ByteSize: size, DurationMS: a.Blob.DurationMS,
			StorageName: sha, PublicToken: token, CreatedAt: t,
		}); err != nil {
			return fmt.Errorf("restore audio asset blob row: %w", err)
		}
		newID, err := audioasset.NewAssetID()
		if err != nil {
			return fmt.Errorf("generate audio asset id: %w", err)
		}
		ids[a.ID] = newID
		if err := sink.AudioAssets.CreateAsset(ctx, audioasset.Asset{
			ID: newID, BlobSHA256: sha, Kind: a.Kind, DisplayName: a.DisplayName, Source: a.Source,
			CreatedAt: t, UpdatedAt: t,
		}); err != nil {
			return fmt.Errorf("restore audio asset: %w", err)
		}
	}

	// --- audio settings (singleton) -------------------------------------------
	if cfg.AudioSettings != nil {
		s := *cfg.AudioSettings
		s.EnabledSourceIDs = ids.remapAll(s.EnabledSourceIDs)
		t := now().UTC()
		s.CreatedAt, s.UpdatedAt = t, t
		if _, err := sink.AudioSettings.SetSettings(ctx, s, t); err != nil {
			return fmt.Errorf("restore audio settings: %w", err)
		}
	}

	// --- goals + widget profiles -----------------------------------------------
	for _, g := range cfg.Goals {
		newID, err := goals.NewGoalID()
		if err != nil {
			return fmt.Errorf("generate goal id: %w", err)
		}
		ids[g.ID] = newID
		t := now().UTC()
		g.ID = newID
		g.Accounts = ids.remapAll(g.Accounts)
		g.CreatedAt, g.UpdatedAt, g.StartedAt = t, t, t
		if _, err := sink.Goals.CreateGoal(ctx, g); err != nil {
			return fmt.Errorf("restore goal: %w", err)
		}
	}
	// Pass 1: create every widget profile with its Children cleared -
	// a dashboard's children reference OTHER widget profiles by id,
	// which may not have a new local id yet (export order is
	// created_at/id, not dependency order), so Children is always
	// patched in pass 2 below, once every sibling profile in this
	// backup has a real new id.
	dashboards := make([]goals.WidgetProfile, 0)
	for _, w := range cfg.WidgetProfiles {
		newID, err := goals.NewWidgetProfileID()
		if err != nil {
			return fmt.Errorf("generate widget profile id: %w", err)
		}
		ids[w.ID] = newID
		if len(w.Children) > 0 {
			dashboards = append(dashboards, w)
		}
		t := now().UTC()
		w.ID = newID
		w.GoalID = ids.remap(w.GoalID)
		w.Accounts = ids.remapAll(w.Accounts)
		w.Children = nil
		// PublicSlug preserved verbatim, same reasoning as every other
		// public-facing locator.
		w.CreatedAt, w.UpdatedAt = t, t
		if _, err := sink.Goals.CreateWidgetProfile(ctx, w); err != nil {
			return fmt.Errorf("restore widget profile: %w", err)
		}
	}
	// Pass 2: patch each dashboard's own Children back in, now
	// remapped to the real new ids every sibling widget profile above
	// was just given.
	for _, w := range dashboards {
		newID := ids.remap(w.ID)
		children := make([]goals.DashboardChild, len(w.Children))
		for i, c := range w.Children {
			c.WidgetProfileID = ids.remap(c.WidgetProfileID)
			children[i] = c
		}
		w.ID = newID
		w.GoalID = ids.remap(w.GoalID)
		w.Accounts = ids.remapAll(w.Accounts)
		w.Children = children
		if _, err := sink.Goals.UpdateWidgetProfile(ctx, w); err != nil {
			return fmt.Errorf("restore dashboard widget children: %w", err)
		}
	}

	// --- metadata presets ---------------------------------------------------
	for _, p := range cfg.MetadataPresets {
		newID, err := metadatapreset.NewID()
		if err != nil {
			return fmt.Errorf("generate metadata preset id: %w", err)
		}
		t := now().UTC()
		p.ID = newID
		p.CreatedAt, p.UpdatedAt = t, t
		if err := sink.MetadataPresets.Create(ctx, p); err != nil {
			return fmt.Errorf("restore metadata preset: %w", err)
		}
	}

	// --- operator chat preferences (singleton + child lists) -----------------
	if cfg.OperatorChatPreferences != nil {
		t := now().UTC()
		prefs := cfg.OperatorChatPreferences.Preferences
		prefs.CreatedAt, prefs.UpdatedAt = t, t
		if _, err := sink.OperatorChatPrefs.SetPreferences(ctx, prefs, t); err != nil {
			return fmt.Errorf("restore operator chat preferences: %w", err)
		}
		for _, v := range cfg.OperatorChatPreferences.AccountVisibility {
			if _, err := sink.OperatorChatPrefs.SetAccountVisibility(ctx, ids.remap(v.AccountID), v.Visible, t); err != nil {
				return fmt.Errorf("restore operator chat account visibility: %w", err)
			}
		}
		for _, u := range cfg.OperatorChatPreferences.HiddenUsers {
			u.ConnectedAccountID = ids.remap(u.ConnectedAccountID)
			if _, err := sink.OperatorChatPrefs.AddHiddenUser(ctx, u, t); err != nil {
				return fmt.Errorf("restore operator chat hidden user: %w", err)
			}
		}
		for _, u := range cfg.OperatorChatPreferences.BotUsers {
			u.ConnectedAccountID = ids.remap(u.ConnectedAccountID)
			if _, err := sink.OperatorChatPrefs.AddBotUser(ctx, u, t); err != nil {
				return fmt.Errorf("restore operator chat bot user: %w", err)
			}
		}
	}

	// --- update preferences (singleton) ---------------------------------------
	if cfg.UpdatePreferences != nil {
		t := now().UTC()
		prefs := *cfg.UpdatePreferences
		prefs.CreatedAt, prefs.UpdatedAt = t, t
		if _, err := sink.UpdatePreferences.SetPreferences(ctx, prefs, t); err != nil {
			return fmt.Errorf("restore update preferences: %w", err)
		}
	}

	return nil
}

func remapChatAutomationTargets(targets []chatautomation.Target, ids idMap) []chatautomation.Target {
	out := make([]chatautomation.Target, len(targets))
	for i, t := range targets {
		out[i] = chatautomation.Target{
			AccountID:  ids.remap(t.AccountID),
			PlatformID: ids.remap(t.PlatformID),
		}
	}
	return out
}
