package diagnostics

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorderBoundedCapacity(t *testing.T) {
	r := NewRecorder()
	for i := 0; i < RingCapacity+500; i++ {
		r.Add(Entry{Time: time.Now(), Severity: "INFO", Message: "entry"})
	}
	got := r.Snapshot(Filter{Limit: MaxLimit})
	if len(got) != RingCapacity {
		t.Fatalf("Snapshot returned %d entries, want exactly %d (the ring's fixed capacity)", len(got), RingCapacity)
	}
}

func TestRecorderSnapshotOrderAndFilters(t *testing.T) {
	r := NewRecorder()
	r.Add(Entry{Time: time.Now(), Severity: "INFO", Subsystem: "chatoverlay", Message: "first"})
	r.Add(Entry{Time: time.Now(), Severity: "ERROR", Subsystem: "mediamtx", Message: "second failure"})
	r.Add(Entry{Time: time.Now(), Severity: "INFO", Subsystem: "chatoverlay", Message: "third"})

	all := r.Snapshot(Filter{})
	if len(all) != 3 {
		t.Fatalf("got %d entries, want 3", len(all))
	}
	if all[0].Message != "third" {
		t.Errorf("newest-first order violated: all[0].Message = %q, want %q", all[0].Message, "third")
	}
	if all[2].Message != "first" {
		t.Errorf("newest-first order violated: all[2].Message = %q, want %q", all[2].Message, "first")
	}

	bySeverity := r.Snapshot(Filter{Severity: "ERROR"})
	if len(bySeverity) != 1 || bySeverity[0].Message != "second failure" {
		t.Errorf("Severity filter returned %+v, want just the ERROR entry", bySeverity)
	}

	bySubsystem := r.Snapshot(Filter{Subsystem: "chatoverlay"})
	if len(bySubsystem) != 2 {
		t.Errorf("Subsystem filter returned %d entries, want 2", len(bySubsystem))
	}

	bySearch := r.Snapshot(Filter{Search: "FAILURE"})
	if len(bySearch) != 1 || bySearch[0].Message != "second failure" {
		t.Errorf("case-insensitive Search filter returned %+v", bySearch)
	}
}

func TestRecorderConcurrentAdd(t *testing.T) {
	r := NewRecorder()
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				r.Add(Entry{Time: time.Now(), Severity: "INFO", Message: "concurrent"})
			}
		}()
	}
	wg.Wait()
	got := r.Snapshot(Filter{Limit: MaxLimit})
	if len(got) != RingCapacity {
		t.Fatalf("after concurrent writes, Snapshot returned %d, want %d", len(got), RingCapacity)
	}
}

func TestHandlerDelegatesUnchangedAndCaptures(t *testing.T) {
	var out bytes.Buffer
	real := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelInfo})
	recorder := NewRecorder()
	h := NewHandler(real, recorder)
	logger := slog.New(h)

	logger.Info("database ready", slog.String("driver", "sqlite"))
	logger.Error("upstream token rejected 9f8e7d6c5b4a3928170615243342516278899aabbccddeeff0011223344")

	if !strings.Contains(out.String(), "database ready") {
		t.Errorf("real handler output missing the delegated record: %q", out.String())
	}
	if !strings.Contains(out.String(), "9f8e7d6c5b4a3928170615243342516278899aabbccddeeff0011223344") {
		t.Errorf("real handler output must stay byte-for-byte unredacted (headless/journald contract): %q", out.String())
	}

	entries := recorder.Snapshot(Filter{Limit: MaxLimit})
	if len(entries) != 2 {
		t.Fatalf("recorder captured %d entries, want 2", len(entries))
	}

	var sawRedacted, sawPlain bool
	for _, e := range entries {
		if strings.Contains(e.Message, "9f8e7d6c5b4a3928170615243342516278899aabbccddeeff0011223344") {
			sawPlain = true
		}
		if strings.Contains(e.Message, "{redacted}") {
			sawRedacted = true
		}
		if e.Message == "database ready" && e.Subsystem == "" {
			t.Errorf("captured entry missing a derived subsystem: %+v", e)
		}
	}
	if sawPlain {
		t.Errorf("captured entries must never contain the raw secret-shaped token")
	}
	if !sawRedacted {
		t.Errorf("captured entries should include the redacted placeholder for the token message")
	}
}

func TestHandlerEnabledDelegates(t *testing.T) {
	real := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := NewHandler(real, NewRecorder())
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("Handler.Enabled should delegate to the real handler's configured level")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Errorf("Handler.Enabled should report true for a level above the real handler's threshold")
	}
}
