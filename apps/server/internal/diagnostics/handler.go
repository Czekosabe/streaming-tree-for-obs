package diagnostics

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
)

// Handler wraps a real slog.Handler, delegating every record to it
// completely unchanged (headless/journald and desktop stdout output
// stay byte-for-byte identical, docs/final-hardening.md §A) while
// additionally capturing a redacted copy of each record into a bounded
// Recorder. This is the one seam that turns the existing single
// logger into an operator-visible diagnostics surface, without
// building a second logging universe.
type Handler struct {
	real     slog.Handler
	recorder *Recorder
}

// NewHandler returns a Handler that delegates to real and captures
// every record it handles into recorder.
func NewHandler(real slog.Handler, recorder *Recorder) *Handler {
	return &Handler{real: real, recorder: recorder}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.real.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	err := h.real.Handle(ctx, record)
	h.capture(record)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{real: h.real.WithAttrs(attrs), recorder: h.recorder}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{real: h.real.WithGroup(name), recorder: h.recorder}
}

// capture builds a redacted Entry from record and adds it to the
// ring buffer. record.Attrs is safe to call here even though real's
// own Handle call (above) may already have iterated it - slog.Record
// is a reusable value, not a single-use stream.
func (h *Handler) capture(record slog.Record) {
	var attrs map[string]any
	if n := record.NumAttrs(); n > 0 {
		attrs = make(map[string]any, n)
		record.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = redactAttrValue(a)
			return true
		})
	}
	h.recorder.Add(Entry{
		Time:      record.Time,
		Severity:  record.Level.String(),
		Subsystem: subsystemFromPC(record.PC),
		Message:   RedactText(record.Message),
		Attrs:     attrs,
	})
}

// redactAttrValue applies the same defense-in-depth text scan to a
// logged attribute's value as RedactText applies to the message -
// an attribute carrying interpolated free text (an upstream error's
// Error() string, in particular) can carry the same secret-shaped
// substrings a message can.
func redactAttrValue(a slog.Attr) any {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return RedactText(v.String())
	default:
		if err, ok := v.Any().(error); ok {
			return RedactText(err.Error())
		}
		return v.Any()
	}
}

// subsystemFromPC derives a short, stable subsystem label from the
// call site's program counter - always populated by slog.Logger's own
// Info/Warn/Error/Debug methods, independent of any handler's
// AddSource option. This gives every captured entry a meaningful
// grouping automatically, without requiring every one of this
// codebase's existing call sites to be edited to attach one.
func subsystemFromPC(pc uintptr) string {
	if pc == 0 {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}
	return packageFromFuncName(fn.Name())
}

// packageFromFuncName extracts the package name from a fully
// qualified Go function name, e.g.
// "github.com/streaming-tree/server/internal/chatoverlay.(*Manager).run"
// becomes "chatoverlay", and "main.main" becomes "main".
func packageFromFuncName(name string) string {
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		return "unknown"
	}
	return name
}
