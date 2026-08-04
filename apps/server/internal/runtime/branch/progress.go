package branch

import (
	"strconv"
	"strings"
	"time"
)

// progressAccumulator parses FFmpeg's `-progress pipe:1` key=value stream one
// line at a time.
//
// FFmpeg emits one block of keys per update, terminated by a line whose key
// is "progress" and whose value is "continue" or "end" - see
// https://ffmpeg.org/ffmpeg.html, checked while implementing this stage.
// Unknown keys are tolerated (this application does not need every key
// FFmpeg might ever add); a malformed required value is ignored for that
// field rather than aborting the whole block, since one bad line must not
// take down progress reporting for the rest of the run.
type progressAccumulator struct {
	pending Progress
	hasAny  bool
}

// newProgressAccumulator starts a fresh block. FrameCount defaults to -1 so
// "FFmpeg never reported a frame count" (an audio-only branch) stays
// distinguishable from "FFmpeg reported zero frames".
func newProgressAccumulator() *progressAccumulator {
	return &progressAccumulator{pending: Progress{FrameCount: -1}}
}

// maxProgressLineBytes bounds one line, so a malformed or hostile stream
// cannot grow memory without limit.
const maxProgressLineBytes = 4 << 10

// feed processes one line. It returns a completed Progress and true only on
// the line that ends a block (progress=continue or progress=end); every
// other line just accumulates into the pending block.
func (a *progressAccumulator) feed(line string) (Progress, bool) {
	line = strings.TrimSpace(line)
	if line == "" || len(line) > maxProgressLineBytes {
		return Progress{}, false
	}

	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return Progress{}, false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	switch key {
	case "frame":
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			a.pending.FrameCount = n
			a.hasAny = true
		}
	case "fps":
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			a.pending.FPS = n
			a.hasAny = true
		}
	case "out_time_ms":
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			a.pending.OutTimeMs = n
			a.hasAny = true
		}
	case "out_time_us":
		// Present for backward compatibility across FFmpeg versions; only
		// used when out_time_ms was not already set by this same block.
		if a.pending.OutTimeMs == 0 {
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.pending.OutTimeMs = n / 1000
				a.hasAny = true
			}
		}
	case "total_size":
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			a.pending.TotalSize = n
			a.hasAny = true
		}
	case "speed":
		// FFmpeg reports "N/A" before it has a meaningful measurement, and a
		// trailing "x" such as "1.02x" once it does.
		trimmed := strings.TrimSuffix(value, "x")
		if n, err := strconv.ParseFloat(trimmed, 64); err == nil {
			a.pending.Speed = n
			a.hasAny = true
		}
	case "progress":
		if value != "continue" && value != "end" {
			return Progress{}, false
		}
		if !a.hasAny {
			// No field FFmpeg reported this block actually parsed to
			// anything - do not report an all-zero block as if FFmpeg had
			// said something meaningful.
			a.pending = Progress{FrameCount: -1}
			return Progress{}, false
		}
		completed := a.pending
		completed.ObservedAt = time.Now()
		a.pending = Progress{FrameCount: -1}
		a.hasAny = false
		return completed, true
	}

	return Progress{}, false
}

// hasAdvanced reports whether a completed progress block shows real output,
// not just an instantaneous first tick still sitting at zero. A branch must
// not be shown as live from process creation, or from a first progress line
// that reports nothing has actually moved yet.
func (p Progress) hasAdvanced() bool {
	return p.OutTimeMs > 0 || p.TotalSize > 0 || p.FrameCount > 0
}
