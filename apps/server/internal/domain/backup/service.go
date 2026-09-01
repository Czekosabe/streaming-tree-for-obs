package backup

import (
	"context"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service is the backup domain's own use-case layer: read every
// included domain (Sources) to build a package, and validate/stage/
// commit an uploaded one. Restore's actual commit (23D) is added
// separately from this substage's RestorePreview - nothing in this
// file mutates the real configuration.
type Service struct {
	sources Sources

	visualBlobs AssetBlobSource
	audioBlobs  AssetBlobSource

	staging Staging

	appVersion string
	platform   string
	now        Clock
}

// NewService builds a Service. appVersion/platformName are stamped
// into every package's manifest (docs/backup-restore.md §5) -
// buildinfo.EffectiveVersion() and runtime.GOOS at the call site.
func NewService(sources Sources, visualBlobs, audioBlobs AssetBlobSource, staging Staging, appVersion, platformName string) *Service {
	return &Service{
		sources: sources, visualBlobs: visualBlobs, audioBlobs: audioBlobs, staging: staging,
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
