// Package sysresources collects local, read-only host resource snapshots -
// CPU, memory, and disk usage of the application's own data volume - for
// the Dashboard's "System resources" card.
//
// This is local resource monitoring, not remote telemetry: every value is
// sampled from this machine, held only in memory, and served back to the
// same local browser tab that already talks to this backend over
// GET /api/system/resources. Nothing here writes to disk, nothing is kept
// beyond the single most recent sample, and nothing is ever sent to any
// third party or external service - there is no outbound HTTP client in
// this package at all.
package sysresources

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Snapshot is the payload of GET /api/system/resources.
//
// Every metric is an independent pointer, not one all-or-nothing gate: a
// platform or environment that cannot report one metric (e.g. disk usage
// for an unusual filesystem) still reports the others, and the field is
// simply absent (nil) rather than a fabricated zero. Unavailable lists
// which metric keys, if any, could not be sampled this tick, so the
// frontend can render an honest "unavailable" state instead of silently
// treating a missing field as zero usage.
//
// The frontend validates this shape with Zod, so field names and JSON tags
// are part of the API contract - see apps/web/src/models/system-resources.ts.
type Snapshot struct {
	CPUPercent       *float64 `json:"cpuPercent,omitempty"`
	MemoryPercent    *float64 `json:"memoryPercent,omitempty"`
	MemoryUsedBytes  *uint64  `json:"memoryUsedBytes,omitempty"`
	MemoryTotalBytes *uint64  `json:"memoryTotalBytes,omitempty"`
	DiskPercent      *float64 `json:"diskPercent,omitempty"`
	DiskUsedBytes    *uint64  `json:"diskUsedBytes,omitempty"`
	DiskTotalBytes   *uint64  `json:"diskTotalBytes,omitempty"`
	// Unavailable holds "cpu"/"memory"/"disk" for any metric this tick could
	// not sample. Always a non-nil (possibly empty) slice, so it serializes
	// as `[]` rather than `null`.
	Unavailable []string `json:"unavailable"`
	SampledAt   string   `json:"sampledAt"`
}

// Collector periodically samples host resource usage in the background and
// caches the single most recent snapshot - no history, nothing persisted.
// A background collector (rather than sampling directly inside the HTTP
// handler) keeps a request from ever blocking on a syscall, and keeps the
// real sampling cadence bounded and explicit rather than driven by however
// often a browser tab happens to poll.
type Collector struct {
	dataDir  string
	logger   *slog.Logger
	interval time.Duration

	mu       sync.RWMutex
	snapshot Snapshot

	stop chan struct{}
	done chan struct{}
}

// NewCollector builds a collector that samples disk usage for dataDir (the
// application's own data volume, not necessarily the OS/system drive) on
// the given interval. It does nothing until Start is called.
func NewCollector(dataDir string, logger *slog.Logger, interval time.Duration) *Collector {
	return &Collector{
		dataDir:  dataDir,
		logger:   logger,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins background sampling. Safe to call at most once.
func (c *Collector) Start() {
	go c.run()
}

func (c *Collector) run() {
	defer close(c.done)

	c.collect()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// Shutdown stops background sampling and waits for the current sample (if
// any) to finish, up to ctx's own deadline.
func (c *Collector) Shutdown(ctx context.Context) {
	close(c.stop)
	select {
	case <-c.done:
	case <-ctx.Done():
	}
}

// Snapshot returns the most recently sampled values. Safe for concurrent use.
func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot
}

func (c *Collector) collect() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap := Snapshot{
		SampledAt:   time.Now().UTC().Format(time.RFC3339),
		Unavailable: []string{},
	}

	// interval=0 reports the delta since this collector's own previous
	// call, per gopsutil's own documented convention for periodic sampling
	// - the correct, non-blocking way to get a real utilisation percentage
	// on a recurring background tick, rather than blocking this goroutine
	// for a fixed sampling window on every tick. The very first sample
	// after startup is therefore not meaningful (no previous call to diff
	// against) and gopsutil reports 0 for it; that single early reading is
	// an accepted, self-correcting startup transient, not a persistent
	// fabricated value.
	if percents, err := cpu.PercentWithContext(ctx, 0, false); err != nil || len(percents) == 0 {
		if c.logger != nil {
			c.logger.Debug("sysresources: CPU sample unavailable", "error", err)
		}
		snap.Unavailable = append(snap.Unavailable, "cpu")
	} else {
		v := percents[0]
		snap.CPUPercent = &v
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err != nil {
		if c.logger != nil {
			c.logger.Debug("sysresources: memory sample unavailable", "error", err)
		}
		snap.Unavailable = append(snap.Unavailable, "memory")
	} else {
		used, total, pct := vm.Used, vm.Total, vm.UsedPercent
		snap.MemoryUsedBytes = &used
		snap.MemoryTotalBytes = &total
		snap.MemoryPercent = &pct
	}

	if du, err := disk.UsageWithContext(ctx, c.dataDir); err != nil {
		if c.logger != nil {
			c.logger.Debug("sysresources: disk sample unavailable", "dataDir", c.dataDir, "error", err)
		}
		snap.Unavailable = append(snap.Unavailable, "disk")
	} else {
		used, total, pct := du.Used, du.Total, du.UsedPercent
		snap.DiskUsedBytes = &used
		snap.DiskTotalBytes = &total
		snap.DiskPercent = &pct
	}

	c.mu.Lock()
	c.snapshot = snap
	c.mu.Unlock()
}
