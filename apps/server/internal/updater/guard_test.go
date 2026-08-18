package updater

import (
	"testing"

	"github.com/streaming-tree/server/internal/runtime/branch"
)

func TestStreamingActiveIdleIsNotActive(t *testing.T) {
	snapshots := []branch.Snapshot{
		{PlatformID: "a", State: branch.StateIdle, DesiredRunning: false},
	}
	if StreamingActive(snapshots) {
		t.Fatal("StreamingActive() = true, want false for an idle, not-desired branch")
	}
}

func TestStreamingActiveEmptyIsNotActive(t *testing.T) {
	if StreamingActive(nil) {
		t.Fatal("StreamingActive(nil) = true, want false")
	}
}

func TestStreamingActiveDesiredRunningIsActive(t *testing.T) {
	snapshots := []branch.Snapshot{
		{PlatformID: "a", State: branch.StateIdle, DesiredRunning: true},
	}
	if !StreamingActive(snapshots) {
		t.Fatal("StreamingActive() = false, want true when DesiredRunning is true")
	}
}

func TestStreamingActiveEveryTransitionalState(t *testing.T) {
	transitional := []branch.State{
		branch.StateStarting, branch.StateLive, branch.StateRestarting,
		branch.StateWaitingForIngest, branch.StateStopping,
	}
	for _, state := range transitional {
		snapshots := []branch.Snapshot{{PlatformID: "a", State: state, DesiredRunning: false}}
		if !StreamingActive(snapshots) {
			t.Errorf("StreamingActive() = false for state %q, want true", state)
		}
	}
}

func TestStreamingActiveOneOfManyIsEnough(t *testing.T) {
	snapshots := []branch.Snapshot{
		{PlatformID: "a", State: branch.StateIdle, DesiredRunning: false},
		{PlatformID: "b", State: branch.StateLive, DesiredRunning: true},
	}
	if !StreamingActive(snapshots) {
		t.Fatal("StreamingActive() = false, want true when any branch is active")
	}
}
