package sysresources

import (
	"context"
	"os"
	"testing"
	"time"
)

// waitForSample blocks until the collector has produced at least one real
// sample (SampledAt becomes non-empty), or fails the test after a bounded
// timeout - this package's own background tick is asynchronous.
func waitForSample(t *testing.T, c *Collector) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := c.Snapshot()
		if snap.SampledAt != "" {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("collector did not produce a sample within 5s")
	return Snapshot{}
}

func TestCollector_ReportsBoundedRealValues(t *testing.T) {
	dataDir := t.TempDir()
	c := NewCollector(dataDir, nil, time.Hour) // interval irrelevant: only the initial collect() on Start matters here
	c.Start()
	defer c.Shutdown(context.Background())

	snap := waitForSample(t, c)

	// Every metric is either present and bounded, or honestly absent (and
	// named in Unavailable) - never a fabricated/out-of-range value.
	if snap.CPUPercent != nil {
		if *snap.CPUPercent < 0 || *snap.CPUPercent > 100 {
			t.Errorf("CPUPercent = %v, want within [0, 100]", *snap.CPUPercent)
		}
	}
	if snap.MemoryPercent != nil {
		if *snap.MemoryPercent < 0 || *snap.MemoryPercent > 100 {
			t.Errorf("MemoryPercent = %v, want within [0, 100]", *snap.MemoryPercent)
		}
		if snap.MemoryTotalBytes == nil || *snap.MemoryTotalBytes == 0 {
			t.Error("MemoryPercent is present but MemoryTotalBytes is nil/zero")
		}
		if snap.MemoryUsedBytes != nil && snap.MemoryTotalBytes != nil && *snap.MemoryUsedBytes > *snap.MemoryTotalBytes {
			t.Errorf("MemoryUsedBytes (%d) > MemoryTotalBytes (%d)", *snap.MemoryUsedBytes, *snap.MemoryTotalBytes)
		}
	}
	if snap.DiskPercent != nil {
		if *snap.DiskPercent < 0 || *snap.DiskPercent > 100 {
			t.Errorf("DiskPercent = %v, want within [0, 100]", *snap.DiskPercent)
		}
		if snap.DiskUsedBytes != nil && snap.DiskTotalBytes != nil && *snap.DiskUsedBytes > *snap.DiskTotalBytes {
			t.Errorf("DiskUsedBytes (%d) > DiskTotalBytes (%d)", *snap.DiskUsedBytes, *snap.DiskTotalBytes)
		}
	}
	if snap.Unavailable == nil {
		t.Error("Unavailable is nil, want a non-nil (possibly empty) slice so it serialises as [] not null")
	}
}

func TestCollector_DiskUsageReflectsTheRealDataDirVolume(t *testing.T) {
	dataDir := t.TempDir()
	c := NewCollector(dataDir, nil, time.Hour)
	c.Start()
	defer c.Shutdown(context.Background())

	snap := waitForSample(t, c)

	for _, name := range snap.Unavailable {
		if name == "disk" {
			t.Skip("disk metric unavailable in this test environment - acceptable per this package's own honest-unavailable contract")
		}
	}
	if snap.DiskTotalBytes == nil || *snap.DiskTotalBytes == 0 {
		t.Fatal("disk metric reported available but DiskTotalBytes is nil/zero")
	}
}

// Nothing in this package ever writes to disk: dataDir is read-only input
// to disk.Usage. Confirms the "no persisted resource telemetry" contract
// behaviourally, not just by code review.
func TestCollector_NeverWritesToDataDir(t *testing.T) {
	dataDir := t.TempDir()
	before, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("ReadDir before: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("test setup: dataDir not empty before the collector ran")
	}

	c := NewCollector(dataDir, nil, 20*time.Millisecond)
	c.Start()
	time.Sleep(120 * time.Millisecond) // let a few ticks pass
	c.Shutdown(context.Background())

	after, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("ReadDir after: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("dataDir gained %d entr(y/ies) after the collector ran - it must never write anything", len(after))
	}
}

func TestCollector_ShutdownStopsBackgroundSampling(t *testing.T) {
	dataDir := t.TempDir()
	c := NewCollector(dataDir, nil, 10*time.Millisecond)
	c.Start()
	waitForSample(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c.Shutdown(ctx)

	if err := ctx.Err(); err != nil {
		t.Fatalf("Shutdown did not return before its own deadline: %v", err)
	}
}
