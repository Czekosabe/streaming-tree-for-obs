package backup

import (
	"context"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// StreamingGuard reports whether this application currently considers
// a broadcast active - restore refuses to run while it does (docs/
// backup-restore.md §7 step 6/§9 of the governing task), reusing
// exactly the same rule the application updater's own "installing is
// blocked while a stream is active" guard already uses
// (updater.StreamingActive), never a second definition of "active".
type StreamingGuard interface {
	Active(ctx context.Context) (bool, error)
}

// Service is the backup domain's own use-case layer: read every
// included domain (Sources) to build a package, and validate/stage/
// commit an uploaded one (Sinks).
type Service struct {
	sources Sources
	sinks   Sinks

	visualBlobs      AssetBlobSource
	audioBlobs       AssetBlobSource
	visualBlobWriter BlobWriter
	audioBlobWriter  BlobWriter

	staging   Staging
	safety    SafetySnapshotStore
	streaming StreamingGuard

	appVersion string
	platform   string
	now        Clock
}

// NewService builds a Service. appVersion/platformName are stamped
// into every package's manifest (docs/backup-restore.md §5) -
// buildinfo.EffectiveVersion() and runtime.GOOS at the call site.
func NewService(
	sources Sources, sinks Sinks,
	visualBlobs, audioBlobs AssetBlobSource,
	visualBlobWriter, audioBlobWriter BlobWriter,
	staging Staging, safety SafetySnapshotStore, streaming StreamingGuard,
	appVersion, platformName string,
) *Service {
	return &Service{
		sources: sources, sinks: sinks,
		visualBlobs: visualBlobs, audioBlobs: audioBlobs,
		visualBlobWriter: visualBlobWriter, audioBlobWriter: audioBlobWriter,
		staging: staging, safety: safety, streaming: streaming,
		appVersion: appVersion, platform: platformName, now: time.Now,
	}
}

// Export reads a coherent Config from every included domain and
// returns one complete, validated backup package.
//
// The coherent-snapshot guarantee (docs/backup-restore.md §6) is the
// caller's responsibility today - Sources' own repository
// implementations each run their own read, and cmd/server wires this
// against the same *sql.DB every other read-only listing in this
// application already uses. A dedicated single-read-transaction
// Sources implementation is a natural 23F hardening step if a real
// concurrency issue is ever observed; nothing about this Service's own
// shape needs to change for that.
func (s *Service) Export(ctx context.Context) ([]byte, error) {
	cfg, err := Export(ctx, s.sources)
	if err != nil {
		return nil, err
	}
	return WriteArchive(cfg, s.appVersion, s.platform, s.now(), s.visualBlobs, s.audioBlobs)
}

// RestorePreview fully validates an uploaded package (docs/backup-
// restore.md §5's whole "never blind extraction" pipeline, via
// ReadArchive) and stages its ORIGINAL raw bytes under a fresh token -
// nothing about the real configuration is touched. The returned
// PreviewSession is a bounded summary only: never raw database
// records, matching the governing task's own explicit preview
// requirement.
func (s *Service) RestorePreview(_ context.Context, data []byte) (PreviewSession, error) {
	validated, err := ReadArchive(data)
	if err != nil {
		return PreviewSession{}, err
	}

	token, err := s.staging.Put(data)
	if err != nil {
		return PreviewSession{}, err
	}

	var assetBytes int64
	for _, a := range validated.Assets {
		assetBytes += int64(len(a.Data))
	}

	counts := countObjects(validated.Config)
	return PreviewSession{
		Token:                             token,
		Manifest:                          validated.Manifest,
		Counts:                            counts,
		AssetCount:                        len(validated.Assets),
		AssetTotalBytes:                   assetBytes,
		ExpiresAt:                         s.now().Add(PreviewTTL),
		ConnectedAccountsRequireReconnect: counts.ConnectedAccounts,
		DestinationsNeedStreamKey:         counts.Platforms,
		DonationSourcesNeedCredential:     counts.DonationSources,
	}, nil
}

// CancelPreview discards a staged restore-preview session's raw
// bytes immediately - idempotent.
func (s *Service) CancelPreview(token string) {
	s.staging.Remove(token)
}

// Restore commits a previously-previewed package (docs/backup-
// restore.md §7's full flow):
//
//  1. load the staged raw bytes for token (ErrNotFound if missing/
//     expired) and re-validate them from scratch via ReadArchive -
//     never trusting RestorePreview's own earlier parse;
//  2. refuse with ErrStreamingActive if a broadcast is active;
//  3. take a pre-restore safety snapshot of the CURRENT configuration
//     (the exact same Export this Service's own Export method uses),
//     so a bad restore can be recovered from through the normal
//     restore flow again, using the snapshot as the source package;
//  4. clear every existing row in every included domain;
//  5. insert everything from the restored package, minting a fresh
//     local id for every object and remapping every cross-domain
//     reference (§4/§5) - never re-using a backup-supplied id as a
//     literal local primary key;
//  6. discard the staged upload.
//
// If step 1 or 2 fails, nothing about the real configuration is
// touched. If step 4/5 fails partway, the safety snapshot from step 3
// remains available - the operator restores it the same way any other
// backup is restored (docs/backup-restore.md §19).
func (s *Service) Restore(ctx context.Context, token string) (RestoreResult, error) {
	data, err := s.staging.Get(token)
	if err != nil {
		return RestoreResult{}, err
	}

	validated, err := ReadArchive(data)
	if err != nil {
		return RestoreResult{}, err
	}

	if s.streaming != nil {
		active, err := s.streaming.Active(ctx)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("check streaming-active guard: %w", err)
		}
		if active {
			return RestoreResult{}, ErrStreamingActive
		}
	}

	if s.safety != nil {
		currentCfg, err := Export(ctx, s.sources)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("read current configuration for safety snapshot: %w", err)
		}
		snapshotData, err := WriteArchive(currentCfg, s.appVersion, s.platform, s.now(), s.visualBlobs, s.audioBlobs)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("build safety snapshot: %w", err)
		}
		if err := s.safety.Save(snapshotData); err != nil {
			return RestoreResult{}, fmt.Errorf("save safety snapshot: %w", err)
		}
	}

	if err := clearExisting(ctx, s.sources, s.sinks); err != nil {
		return RestoreResult{}, fmt.Errorf("clear existing configuration: %w", err)
	}

	assetsByHash := make(map[string][]byte, len(validated.Assets))
	for _, a := range validated.Assets {
		assetsByHash[a.Manifest.SHA256] = a.Data
	}
	if err := applyConfig(ctx, validated.Config, assetsByHash, s.sinks, s.visualBlobWriter, s.audioBlobWriter, s.now); err != nil {
		return RestoreResult{}, fmt.Errorf("apply restored configuration: %w", err)
	}

	s.staging.Remove(token)

	counts := countObjects(validated.Config)
	return RestoreResult{
		Counts:                            counts,
		ConnectedAccountsRequireReconnect: counts.ConnectedAccounts,
		DestinationsNeedStreamKey:         counts.Platforms,
		DonationSourcesNeedCredential:     counts.DonationSources,
		RestartRequired:                   true,
	}, nil
}
