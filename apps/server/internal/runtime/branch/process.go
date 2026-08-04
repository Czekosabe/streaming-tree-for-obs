package branch

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"
)

// maxLogLineBytes bounds one captured stderr line.
const maxLogLineBytes = 8 << 10

// diagnosticRingSize is the number of recent, already-redacted stderr lines
// kept in memory - a diagnostic aid, not a log viewer.
const diagnosticRingSize = 50

// onProgress is called for every completed progress block FFmpeg reports.
type onProgress func(Progress)

// process wraps one branch's FFmpeg child process.
type process struct {
	cmd      *exec.Cmd
	logger   *slog.Logger
	redactor *Redactor

	exited  chan struct{}
	waitErr error

	mu   sync.Mutex
	ring []string
}

// startProcess spawns FFmpeg for one branch.
//
// executablePath and args must never be logged together in a way that
// reconstructs the full command line - args' last element is the destination
// URL, which carries the stream key. Both stdout (progress) and stderr
// (diagnostic log lines, redacted before they are kept or logged) are
// drained by their own goroutine, exactly like the MediaMTX process wrapper:
// an undrained pipe blocks the child a few kilobytes in.
func startProcess(
	executablePath string, args []string, redactor *Redactor,
	logger *slog.Logger, onProgress onProgress,
) (*process, error) {
	cmd := exec.Command(executablePath, args...)
	cmd.Env = minimalEnv()
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
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	p := &process{
		cmd:      cmd,
		logger:   logger,
		redactor: redactor,
		exited:   make(chan struct{}),
		ring:     make([]string, 0, diagnosticRingSize),
	}

	var drained sync.WaitGroup
	drained.Add(2)
	go func() {
		defer drained.Done()
		p.drainProgress(stdout, onProgress)
	}()
	go func() {
		defer drained.Done()
		p.drainStderr(stderr)
	}()

	go func() {
		drained.Wait()
		p.waitErr = cmd.Wait()
		close(p.exited)
	}()

	return p, nil
}

// drainProgress reads stdout, feeding complete blocks to onProgress. It
// never logs a raw stdout line: -progress output is safe by construction
// (key=value pairs this package defines), but the callback is the only
// consumer, keeping this function itself simple to audit.
func (p *process) drainProgress(reader io.Reader, report onProgress) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024), maxProgressLineBytes)

	acc := newProgressAccumulator()
	for scanner.Scan() {
		if completed, ok := acc.feed(scanner.Text()); ok && report != nil {
			report(completed)
		}
	}
}

// drainStderr reads stderr, redacting and recording each line.
func (p *process) drainStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 4096), maxLogLineBytes)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		redacted := p.redactor.Redact(line)
		p.record(redacted)
		p.logger.Info("ffmpeg", slog.String("line", redacted))
	}
}

func (p *process) record(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.ring) == diagnosticRingSize {
		copy(p.ring, p.ring[1:])
		p.ring = p.ring[:diagnosticRingSize-1]
	}
	p.ring = append(p.ring, line)
}

// lastLines returns a copy of the bounded, already-redacted stderr ring.
func (p *process) lastLines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]string, len(p.ring))
	copy(out, p.ring)
	return out
}

// Exited exposes the completion channel.
func (p *process) Exited() <-chan struct{} { return p.exited }

// stop asks the process to terminate, escalating after a grace period -
// same shape as the MediaMTX process wrapper's stop().
func (p *process) stop(grace time.Duration) error {
	select {
	case <-p.exited:
		return nil
	default:
	}

	if err := terminate(p.cmd.Process); err != nil {
		_ = p.cmd.Process.Kill()
	}

	select {
	case <-p.exited:
		return nil
	case <-time.After(grace):
	}

	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("force-terminate ffmpeg: %w", err)
	}

	select {
	case <-p.exited:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("ffmpeg did not exit after being force-terminated")
	}
}

// minimalEnv builds the child environment - only what FFmpeg genuinely needs
// to run, so the backend's own configuration and secrets are never inherited
// through the environment.
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
