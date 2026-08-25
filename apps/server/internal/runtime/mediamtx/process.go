package mediamtx

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/runtime/procutil"
)

// maxLogLineBytes bounds one captured log line, so a runaway process cannot
// grow the buffer without limit.
const maxLogLineBytes = 8 << 10

// diagnosticRingSize is the number of recent MediaMTX log lines kept in memory.
// Small on purpose: this is a diagnostic aid, not the Logs page.
const diagnosticRingSize = 100

// process wraps one MediaMTX child process.
type process struct {
	cmd    *exec.Cmd
	logger *slog.Logger

	// exited is closed once the process has been reaped.
	exited chan struct{}
	// waitErr holds the result of Wait, valid after exited is closed.
	waitErr error

	mu   sync.Mutex
	ring []string
}

// structuredLine is a MediaMTX log line when logStructured is enabled.
type structuredLine struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// startProcess spawns MediaMTX with the generated configuration.
//
// Both output streams are drained by their own goroutine. A child process whose
// stdout pipe fills up blocks forever, so this is not optional: MediaMTX logs
// steadily, and an undrained pipe would wedge it a few kilobytes in.
func startProcess(executablePath, configPath string, extraEnv []string, logger *slog.Logger) (*process, error) {
	cmd := exec.Command(executablePath, configPath)

	// A minimal environment: the child needs none of the backend's variables,
	// and passing them along would widen what a compromised binary could read.
	// extraEnv is empty in production; the supervisor tests use it to tell the
	// test binary to behave as a fake MediaMTX.
	cmd.Env = append(minimalEnv(), extraEnv...)
	configureProcessAttributes(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("attach to stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("attach to stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start MediaMTX: %w", err)
	}
	// Best-effort safety net (real fix on Windows, honest no-op
	// elsewhere for now - see internal/runtime/procutil's own doc
	// comments): ensures MediaMTX cannot outlive this process even on
	// an ungraceful termination that never reaches Shutdown below. A
	// real physical/manual Windows test found exactly this - an
	// orphaned MediaMTX still bound to its RTMP port after the parent
	// process was already gone.
	if jobErr := procutil.AssignToChildJob(cmd); jobErr != nil {
		logger.Warn("could not enroll MediaMTX in the child-process safety net", slog.Any("error", jobErr))
	}

	p := &process{
		cmd:    cmd,
		logger: logger,
		exited: make(chan struct{}),
		ring:   make([]string, 0, diagnosticRingSize),
	}

	var drained sync.WaitGroup
	drained.Add(2)
	go func() {
		defer drained.Done()
		p.drain(stdout, "stdout")
	}()
	go func() {
		defer drained.Done()
		p.drain(stderr, "stderr")
	}()

	go func() {
		// Both pipes must be fully read before Wait, otherwise Wait can close
		// them underneath the readers.
		drained.Wait()
		p.waitErr = cmd.Wait()
		close(p.exited)
	}()

	return p, nil
}

// drain reads one output stream to EOF, recording bounded log lines.
func (p *process) drain(reader io.Reader, stream string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 4096), maxLogLineBytes)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		p.record(line)
		p.log(line, stream)
	}

	// A scanner error (a line over the limit, or a closed pipe) is not fatal:
	// the process is still supervised through its exit status.
	if err := scanner.Err(); err != nil {
		p.logger.Debug("stopped reading MediaMTX output",
			slog.String("stream", stream), slog.Any("error", err))
	}
}

// log forwards a MediaMTX line, parsing the structured form when possible.
func (p *process) log(line, stream string) {
	var structured structuredLine
	// Malformed lines must never panic or be dropped: they are logged raw.
	if err := json.Unmarshal([]byte(line), &structured); err == nil && structured.Message != "" {
		p.logger.Info("mediamtx",
			slog.String("mediamtx_level", structured.Level),
			slog.String("mediamtx_message", structured.Message),
		)
		return
	}

	p.logger.Info("mediamtx", slog.String("stream", stream), slog.String("line", line))
}

// record keeps the line in a bounded ring for diagnostics.
func (p *process) record(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.ring) == diagnosticRingSize {
		copy(p.ring, p.ring[1:])
		p.ring = p.ring[:diagnosticRingSize-1]
	}
	p.ring = append(p.ring, line)
}

// recentLines returns a copy of the diagnostic ring.
func (p *process) recentLines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]string, len(p.ring))
	copy(out, p.ring)
	return out
}

// Exited exposes the completion channel.
func (p *process) Exited() <-chan struct{} { return p.exited }

// stop asks the process to terminate, escalating after a grace period.
//
// See terminate() for the platform-specific behaviour and its honest limits.
func (p *process) stop(grace time.Duration) error {
	select {
	case <-p.exited:
		return nil
	default:
	}

	if err := terminate(p.cmd.Process); err != nil {
		// Escalate straight away if the polite request could not be delivered.
		_ = p.cmd.Process.Kill()
	}

	select {
	case <-p.exited:
		return nil
	case <-time.After(grace):
	}

	// The grace period elapsed; force termination and reap.
	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("force-terminate MediaMTX: %w", err)
	}

	select {
	case <-p.exited:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("MediaMTX did not exit after being force-terminated")
	}
}

// waitFor blocks until the process exits or the context is done.
func (p *process) waitFor(ctx context.Context) error {
	select {
	case <-p.exited:
		return p.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// minimalEnv builds the child environment.
//
// Only the variables a process genuinely needs to run are forwarded; the
// backend's own configuration is not inherited.
func minimalEnv() []string {
	keep := []string{"PATH", "SystemRoot", "TEMP", "TMP", "HOME", "USERPROFILE", "LANG"}

	env := make([]string, 0, len(keep))
	for _, key := range keep {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}
