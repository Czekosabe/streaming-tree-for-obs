package backup

import (
	"context"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service is the backup domain's own use-case layer: read every
// included domain (Sources), build the package, and hand back its
// bytes. Restore's own use cases (RestorePreview/Restore) are added in
// 23C/23D - this substage covers backup creation only.
type Service struct {
	sources Sources

	visualBlobs AssetBlobSource
	audioBlobs  AssetBlobSource

	appVersion string
	platform   string
	now        Clock
}

// NewService builds a Service. appVersion/platformName are stamped
// into every package's manifest (docs/backup-restore.md §5) -
// buildinfo.EffectiveVersion() and runtime.GOOS at the call site.
func NewService(sources Sources, visualBlobs, audioBlobs AssetBlobSource, appVersion, platformName string) *Service {
	return &Service{
		sources: sources, visualBlobs: visualBlobs, audioBlobs: audioBlobs,
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
