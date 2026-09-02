package preflight

import (
	"context"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/updater"
)

// BranchPort is the narrow slice of branch.Manager this domain
// depends on - EvaluateReadiness reused unchanged, never a second
// implementation of "would this destination actually start" (docs/
// stream-preflight.md §4).
type BranchPort interface {
	EvaluateReadiness(ctx context.Context, platformID string) ([]string, error)
	Snapshot(ctx context.Context) ([]branch.Snapshot, error)
}

// PlatformPort is the narrow slice of platform.Service this domain
// depends on.
type PlatformPort interface {
	List(ctx context.Context) ([]platform.Platform, error)
}

// AccountPort is the narrow slice of account.Service this domain
// depends on - read-only, and only ever used to classify a warning,
// never a blocker (docs/stream-preflight.md §1.5).
type AccountPort interface {
	GetLink(ctx context.Context, platformID string) (account.Link, bool, error)
	GetAccount(ctx context.Context, id string) (account.Account, error)
}

// StreamSetupPort is the narrow slice of streamsetup.Service this
// domain depends on - Preview reused unchanged, never a second
// implementation of "does this profile have a broken reference"
// (docs/stream-preflight.md §1.7).
type StreamSetupPort interface {
	Preview(ctx context.Context, profileID string) (streamsetup.Preview, error)
}

// Service composes the ports above into one Report - it writes
// nothing anywhere.
type Service struct {
	branches  BranchPort
	platforms PlatformPort
	accounts  AccountPort
	setups    StreamSetupPort
}

// NewService builds a Service.
func NewService(branches BranchPort, platforms PlatformPort, accounts AccountPort, setups StreamSetupPort) *Service {
	return &Service{branches: branches, platforms: platforms, accounts: accounts, setups: setups}
}

// Evaluate computes a Report. profileID, when non-nil, preflights
// exactly the destinations the named Stage 25 setup profile
// references; when nil, it preflights every currently-Enabled
// destination (docs/stream-preflight.md §3) - a profile is never
// required to stream.
func (s *Service) Evaluate(ctx context.Context, profileID *string) (Report, error) {
	allPlatforms, err := s.platforms.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("list platforms: %w", err)
	}
	byID := make(map[string]platform.Platform, len(allPlatforms))
	for _, p := range allPlatforms {
		byID[p.ID] = p
	}

	var destIDs []string
	var profileFindings []Finding
	if profileID != nil {
		preview, err := s.setups.Preview(ctx, *profileID)
		if err != nil {
			return Report{}, fmt.Errorf("preview stream setup profile: %w", err)
		}
		for _, d := range preview.Destinations {
			if d.Change == streamsetup.ChangeMissing {
				profileFindings = append(profileFindings, Finding{
					Code: "setup_destination_missing", Severity: SeverityWarning,
					Action: &Action{Code: ActionRepairSetupProfile},
				})
				continue
			}
			destIDs = append(destIDs, d.PlatformID)
		}
		if preview.MetadataPresetMissing {
			profileFindings = append(profileFindings, Finding{
				Code: "setup_metadata_preset_missing", Severity: SeverityWarning,
				Action: &Action{Code: ActionRepairSetupProfile},
			})
		}
	} else {
		for _, p := range allPlatforms {
			if p.Enabled {
				destIDs = append(destIDs, p.ID)
			}
		}
	}

	findings := append([]Finding{}, profileFindings...)
	destinations := make([]DestinationReadiness, 0, len(destIDs))

	for _, id := range destIDs {
		p, ok := byID[id]
		if !ok {
			continue
		}
		dr := DestinationReadiness{PlatformID: id, ProviderID: string(p.ProviderID), DisplayName: p.DisplayName}

		blockers, err := s.branches.EvaluateReadiness(ctx, id)
		if err != nil {
			return Report{}, fmt.Errorf("evaluate destination readiness for %s: %w", id, err)
		}
		for _, code := range blockers {
			f := Finding{Code: code, Severity: SeverityBlocker, PlatformID: id, Action: actionForBlocker(code, id)}
			dr.Findings = append(dr.Findings, f)
			findings = append(findings, f)
		}

		if def, ok := platform.Definition(p.ProviderID); ok {
			if _, err := platform.ValidateMetadata(def, p.Metadata); err != nil {
				f := Finding{
					Code: "metadata_invalid", Severity: SeverityWarning, PlatformID: id,
					Action: &Action{Code: ActionFixMetadata, PlatformID: id},
				}
				dr.Findings = append(dr.Findings, f)
				findings = append(findings, f)
			}
		}

		link, hasLink, err := s.accounts.GetLink(ctx, id)
		if err != nil {
			return Report{}, fmt.Errorf("get account link for %s: %w", id, err)
		}
		if hasLink {
			acc, err := s.accounts.GetAccount(ctx, link.AccountID)
			if err != nil {
				return Report{}, fmt.Errorf("get linked account for %s: %w", id, err)
			}
			if acc.Status == account.StatusReconnectRequired {
				f := Finding{
					Code: "account_reconnect_required", Severity: SeverityWarning, PlatformID: id,
					Action: &Action{Code: ActionReconnectAccount, PlatformID: id},
				}
				dr.Findings = append(dr.Findings, f)
				findings = append(findings, f)
			}
		}

		destinations = append(destinations, dr)
	}

	snapshots, err := s.branches.Snapshot(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("snapshot branches: %w", err)
	}

	return Report{
		Status: computeStatus(findings), Findings: findings, Destinations: destinations,
		SelectedProfileID: profileID, StreamingActive: updater.StreamingActive(snapshots),
	}, nil
}

func computeStatus(findings []Finding) Status {
	hasBlocker, hasWarning := false, false
	for _, f := range findings {
		switch f.Severity {
		case SeverityBlocker:
			hasBlocker = true
		case SeverityWarning:
			hasWarning = true
		}
	}
	switch {
	case hasBlocker:
		return StatusNotReady
	case hasWarning:
		return StatusReadyWithWarnings
	default:
		return StatusReady
	}
}

// actionForBlocker maps a branch.Blocker* identifier to an existing
// corrective action, or nil when there is genuinely nothing to click
// (docs/stream-preflight.md §1.1/§2).
func actionForBlocker(code, platformID string) *Action {
	switch code {
	case branch.BlockerPlatformDisabled, branch.BlockerOutputServerMissing:
		return &Action{Code: ActionOpenDestinationSettings, PlatformID: platformID}
	case branch.BlockerStreamKeyMissing:
		return &Action{Code: ActionAddStreamKey, PlatformID: platformID}
	case branch.BlockerFFmpegMissing, branch.BlockerFFmpegIncompatible:
		return &Action{Code: ActionInstallFFmpeg}
	case branch.BlockerMediaMTXNotReady:
		return &Action{Code: ActionStartMediaMTX}
	default:
		// credential_store_unavailable (OS-level), ingest_not_receiving
		// (the operator must start OBS themselves) - nothing in-app to
		// point at.
		return nil
	}
}
