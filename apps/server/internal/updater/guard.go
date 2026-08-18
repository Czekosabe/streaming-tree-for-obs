package updater

import (
	"context"

	"github.com/streaming-tree/server/internal/runtime/branch"
)

// BranchSnapshotSource is the narrow slice of branch.Manager the
// streaming-active guard needs - a small interface rather than the
// concrete type, so this package's own tests can supply a fake without
// running a real branch manager.
type BranchSnapshotSource interface {
	Snapshot(ctx context.Context) ([]branch.Snapshot, error)
}

// activeStates are the branch.State values that, per docs/updater.md
// §17, mean the operator intends the broadcast to remain active -
// deliberately broadened from branch.Manager's own private StopAll
// predicate to name every transitional state explicitly, so the
// updater's notion of "active" never silently drifts if that internal
// predicate ever changes shape.
var activeStates = map[branch.State]bool{
	branch.StateStarting:         true,
	branch.StateLive:             true,
	branch.StateRestarting:       true,
	branch.StateWaitingForIngest: true,
	branch.StateStopping:         true,
}

// StreamingActive reports whether any branch snapshot indicates the
// operator intends the broadcast to remain active - see
// docs/updater.md §17 for the exact rule and its reasoning. A nil or
// empty snapshot list means no configured destination, i.e. not
// active.
func StreamingActive(snapshots []branch.Snapshot) bool {
	for _, s := range snapshots {
		if s.DesiredRunning || activeStates[s.State] {
			return true
		}
	}
	return false
}
