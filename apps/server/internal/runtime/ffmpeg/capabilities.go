package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/runtime/procutil"
)

// Capabilities records what a probed FFmpeg binary can actually do.
//
// Centralized here on purpose: this is the one place that decides what
// "compatible" means, so nothing else re-implements or drifts from these
// checks - see docs/project-overview.md for why capability probing, not an
// exact version match, is the compatibility policy for this dependency.
type Capabilities struct {
	RTMPInput   bool `json:"rtmpInput"`
	RTMPOutput  bool `json:"rtmpOutput"`
	RTMPSOutput bool `json:"rtmpsOutput"`
	FLVMuxer    bool `json:"flvMuxer"`
	// Progress is true only after a real, short invocation produced a
	// "progress=end" line - not merely because -h lists the flag.
	Progress bool `json:"progress"`
}

// Satisfied reports whether every capability this application requires is
// present.
func (c Capabilities) Satisfied() bool {
	return c.RTMPInput && c.RTMPOutput && c.RTMPSOutput && c.FLVMuxer && c.Progress
}

// Missing lists the capabilities that are not satisfied, for a readable error
// message. Never empty when Satisfied() is false.
func (c Capabilities) Missing() []string {
	var missing []string
	if !c.RTMPInput {
		missing = append(missing, "RTMP input")
	}
	if !c.RTMPOutput {
		missing = append(missing, "RTMP output")
	}
	if !c.RTMPSOutput {
		missing = append(missing, "RTMPS output")
	}
	if !c.FLVMuxer {
		missing = append(missing, "FLV muxer")
	}
	if !c.Progress {
		missing = append(missing, "-progress support")
	}
	return missing
}

// probeTimeout bounds every individual probe command.
const probeTimeout = 15 * time.Second

// maxProbeOutputBytes bounds how much of a probe command's output is kept in
// memory, so a hostile or broken binary cannot exhaust memory by printing
// without limit.
const maxProbeOutputBytes = 1 << 20 // 1 MiB

// probeResult is the outcome of fully probing one executable.
type probeResult struct {
	versionOutput string
	protocols     Capabilities // RTMPInput/RTMPOutput/RTMPSOutput only, filled from -protocols
	flvMuxer      bool
	progressWorks bool
}

// probeExecutable runs every check needed to decide whether path is a usable
// FFmpeg binary. It never uses a shell: every command is a direct
// exec.CommandContext invocation with an explicit argument list.
func probeExecutable(ctx context.Context, path string) (probeResult, error) {
	versionOutput, err := runProbe(ctx, path, "-hide_banner", "-version")
	if err != nil {
		return probeResult{}, fmt.Errorf("run %s -version: %w", filepath.Base(path), err)
	}

	protocolsOutput, err := runProbe(ctx, path, "-hide_banner", "-protocols")
	if err != nil {
		return probeResult{}, fmt.Errorf("run %s -protocols: %w", filepath.Base(path), err)
	}

	muxersOutput, err := runProbe(ctx, path, "-hide_banner", "-muxers")
	if err != nil {
		return probeResult{}, fmt.Errorf("run %s -muxers: %w", filepath.Base(path), err)
	}

	progressWorks := probeProgress(ctx, path)

	return probeResult{
		versionOutput: versionOutput,
		protocols:     parseProtocols(protocolsOutput),
		flvMuxer:      parseFLVMuxer(muxersOutput),
		progressWorks: progressWorks,
	}, nil
}

// runProbe executes one bounded, non-interactive, shell-free command.
func runProbe(ctx context.Context, path string, args ...string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, path, args...)
	procutil.HideConsoleWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &out, max: maxProbeOutputBytes}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// probeProgress runs a trivial, near-instant audio-only encode with
// `-progress pipe:1` and confirms a real "progress=end" line was produced.
// This is what proves the binary can actually run a normal encode, not just
// answer -version - a probe that only checked `-h` for the flag's existence
// would not.
func probeProgress(ctx context.Context, path string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	tempDir, err := os.MkdirTemp("", "streaming-tree-ffmpeg-probe-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tempDir)

	outputPath := filepath.Join(tempDir, "probe.wav")

	cmd := exec.CommandContext(probeCtx, path,
		"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono",
		"-t", "0.1",
		"-c:a", "pcm_s16le",
		"-f", "wav",
		"-progress", "pipe:1",
		"-y", outputPath,
	)
	procutil.HideConsoleWindow(cmd)

	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &out, max: maxProbeOutputBytes}
	cmd.Stderr = nil // discarded: -loglevel error keeps it near-silent anyway

	if err := cmd.Run(); err != nil {
		return false
	}

	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimSpace(line) == "progress=end" {
			return true
		}
	}
	return false
}

// parseProtocols reads `ffmpeg -protocols` output and reports which of the
// Input/Output sections list "rtmp" and "rtmps".
func parseProtocols(output string) Capabilities {
	var caps Capabilities
	section := ""

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "Input:":
			section = "input"
			continue
		case "Output:":
			section = "output"
			continue
		}

		switch {
		case section == "input" && trimmed == "rtmp":
			caps.RTMPInput = true
		case section == "output" && trimmed == "rtmp":
			caps.RTMPOutput = true
		case section == "output" && trimmed == "rtmps":
			caps.RTMPSOutput = true
		}
	}

	return caps
}

// parseFLVMuxer reads `ffmpeg -muxers` output and reports whether "flv" is
// listed with muxing (E) support.
func parseFLVMuxer(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == "flv" && strings.Contains(fields[0], "E") {
			return true
		}
	}
	return false
}

// limitedWriter caps how many bytes are retained from a command's output.
type limitedWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // silently discard past the cap; not an error for the child
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
	} else {
		w.buf.Write(p)
	}
	return len(p), nil
}
