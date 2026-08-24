// Package support builds the Stage 20E diagnostic support bundle
// (docs/final-hardening.md §C): a single, deterministic, privacy-safe
// ZIP generated on explicit operator action only.
//
// This package deliberately depends on nothing beyond plain data
// (Snapshot) and internal/diagnostics's Recorder - it does not import
// internal/httpapi or any domain-service package, so what a bundle can
// ever contain is fully determined by what its caller chooses to put
// into a Snapshot, not by anything this package can reach on its own.
// The caller (cmd/server) gathers Snapshot's fields from services it
// already has in scope at startup.
package support

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/diagnostics"
)

// Snapshot carries every non-secret fact the support bundle reports.
// Every field here MUST already be safe to hand to the operator in a
// file they might paste into a support request - never a credential,
// token, cookie, or full destination URL. When in doubt about a new
// field, it is omitted (docs/final-hardening.md §C).
type Snapshot struct {
	Version          string `json:"version"`
	Commit           string `json:"commit,omitempty"`
	CommitDirty      bool   `json:"commitDirty,omitempty"`
	Packaged         bool   `json:"packaged"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	GoRuntimeVersion string `json:"goRuntimeVersion"`
	Headless         bool   `json:"headless"`
	RemoteManagement bool   `json:"remoteManagementEnabled"`
	RemoteIngest     bool   `json:"remoteIngestEnabled"`
	RemoteOverlay    bool   `json:"remoteOverlayEnabled"`

	MediaMTXVersion string `json:"mediaMTXVersion,omitempty"`
	FFmpegAvailable bool   `json:"ffmpegAvailable"`
	FFmpegVersion   string `json:"ffmpegVersion,omitempty"`

	// SubsystemStates is a small set of high-level state labels (e.g.
	// "mediamtx": "running", "remote_ingest": "receiving") - never a
	// configured destination URL or credential.
	SubsystemStates map[string]string `json:"subsystemStates,omitempty"`

	UpdaterStatus          string `json:"updaterStatus,omitempty"`
	PlatformSupportSummary string `json:"platformSupportSummary,omitempty"`
}

// SnapshotFunc gathers a fresh Snapshot at bundle-generation time.
type SnapshotFunc func(ctx context.Context) (Snapshot, error)

// Builder implements httpapi.SupportBundleBuilder.
type Builder struct {
	recorder *diagnostics.Recorder
	snapshot SnapshotFunc
	now      func() time.Time
}

// NewBuilder returns a Builder reading recent log entries from
// recorder and other facts from snapshot at generation time.
func NewBuilder(recorder *diagnostics.Recorder, snapshot SnapshotFunc) *Builder {
	return &Builder{recorder: recorder, snapshot: snapshot, now: time.Now}
}

// BuildSupportBundle generates one deterministic ZIP containing
// manifest.json (the Snapshot) and logs.json (recent, already-
// redacted ring-buffer entries). The filename is entirely
// app-controlled, never derived from request input.
func (b *Builder) BuildSupportBundle(ctx context.Context) ([]byte, string, error) {
	snap, err := b.snapshot(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("gather support bundle snapshot: %w", err)
	}

	manifest, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal support bundle manifest: %w", err)
	}

	entries := b.recorder.Snapshot(diagnostics.Filter{Limit: diagnostics.MaxLimit})
	logs, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal support bundle logs: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	if err := writeZipFile(zw, "manifest.json", manifest); err != nil {
		return nil, "", err
	}
	if err := writeZipFile(zw, "logs.json", logs); err != nil {
		return nil, "", err
	}
	if err := zw.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize support bundle zip: %w", err)
	}

	filename := fmt.Sprintf("streaming-tree-support-%s-%s.zip", snap.Version, b.now().UTC().Format("20060102T150405Z"))
	return buf.Bytes(), filename, nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create %s in support bundle zip: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write %s in support bundle zip: %w", name, err)
	}
	return nil
}
