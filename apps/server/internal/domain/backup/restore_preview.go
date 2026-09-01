package backup

import "time"

// PreviewTTL is how long a staged restore-preview session stays valid
// before it must be re-uploaded - mirrors
// internal/domain/visualpackage's own PreviewTTL exactly (10 minutes),
// the same "long enough for someone to read a page, not so long stale
// staged data lingers" reasoning.
const PreviewTTL = 10 * time.Minute

// ObjectCounts is a bounded, named summary of what a package contains
// - never raw database records (docs/backup-restore.md §18's "bounded
// summary").
type ObjectCounts struct {
	Platforms         int `json:"platforms"`
	ConnectedAccounts int `json:"connectedAccounts"`
	ChatOverlays      int `json:"chatOverlays"`
	ChatSchedules     int `json:"chatSchedules"`
	ChatCommands      int `json:"chatCommands"`
	AlertProfiles     int `json:"alertProfiles"`
	AlertRules        int `json:"alertRules"`
	VisualTemplates   int `json:"visualTemplates"`
	VisualAssets      int `json:"visualAssets"`
	AudioAssets       int `json:"audioAssets"`
	Goals             int `json:"goals"`
	WidgetProfiles    int `json:"widgetProfiles"`
	MetadataPresets   int `json:"metadataPresets"`
	DonationSources   int `json:"donationSources"`
}

// countObjects builds ObjectCounts from a validated Config - the exact
// domains docs/backup-restore.md §1 marks "Yes".
func countObjects(cfg Config) ObjectCounts {
	counts := ObjectCounts{
		Platforms:         len(cfg.Platforms),
		ConnectedAccounts: len(cfg.ConnectedAccounts),
		ChatOverlays:      len(cfg.ChatOverlays),
		ChatSchedules:     len(cfg.ChatSchedules),
		ChatCommands:      len(cfg.ChatCommands),
		AlertProfiles:     len(cfg.AlertProfiles),
		VisualTemplates:   len(cfg.VisualTemplates),
		VisualAssets:      len(cfg.VisualAssets),
		AudioAssets:       len(cfg.AudioAssets),
		Goals:             len(cfg.Goals),
		WidgetProfiles:    len(cfg.WidgetProfiles),
		MetadataPresets:   len(cfg.MetadataPresets),
		DonationSources:   len(cfg.DonationSources),
	}
	for _, p := range cfg.AlertProfiles {
		counts.AlertRules += len(p.Rules)
	}
	return counts
}

// PreviewSession is a validated, staged package waiting for explicit
// confirmation (docs/backup-restore.md §7/§18) - nothing has been
// written to the real configuration yet, and nothing will be until a
// matching Restore(token) call re-validates and commits it
// independently (§7 step 7 - "never trusts the preview as authority").
type PreviewSession struct {
	Token           string       `json:"token"`
	Manifest        Manifest     `json:"manifest"`
	Counts          ObjectCounts `json:"counts"`
	AssetCount      int          `json:"assetCount"`
	AssetTotalBytes int64        `json:"assetTotalBytes"`
	ExpiresAt       time.Time    `json:"expiresAt"`
	// ConnectedAccountsRequireReconnect is always the connected-account
	// count itself, never zero unless there truly are none - every
	// restored account needs reconnecting, full stop (docs/backup-
	// restore.md §8). Named separately from Counts.ConnectedAccounts so
	// the frontend never has to duplicate that "always requires
	// attention" rule itself.
	ConnectedAccountsRequireReconnect int `json:"connectedAccountsRequireReconnect"`
	// DestinationsNeedStreamKey is always Counts.Platforms - no backup
	// ever carries a stream key (docs/backup-restore.md §12).
	DestinationsNeedStreamKey int `json:"destinationsNeedStreamKey"`
	// DonationSourcesNeedCredential is always Counts.DonationSources -
	// no backup ever carries a donation-source credential either.
	DonationSourcesNeedCredential int `json:"donationSourcesNeedCredential"`
}
