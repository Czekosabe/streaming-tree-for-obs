package branch

import "testing"

// Captured from a real `ffmpeg ... -progress pipe:1` run (see
// docs/progress.md for how this stage's FFmpeg build was probed).
var sampleProgressBlock = []string{
	"frame=5",
	"fps=0.00",
	"stream_0_0_q=-1.0",
	"bitrate=  82.1kbits/s",
	"total_size=6156",
	"out_time_us=600000",
	"out_time_ms=600000",
	"out_time=00:00:00.600000",
	"dup_frames=0",
	"drop_frames=0",
	"speed=36.4x",
	"progress=end",
}

func feedAll(acc *progressAccumulator, lines []string) (Progress, bool) {
	var last Progress
	var ok bool
	for _, line := range lines {
		last, ok = acc.feed(line)
	}
	return last, ok
}

func TestProgressAccumulatorParsesARealBlock(t *testing.T) {
	acc := newProgressAccumulator()
	got, ok := feedAll(acc, sampleProgressBlock)
	if !ok {
		t.Fatal("a complete, real progress block was not reported as complete")
	}
	if got.FrameCount != 5 {
		t.Errorf("FrameCount = %d, want 5", got.FrameCount)
	}
	if got.OutTimeMs != 600000 {
		t.Errorf("OutTimeMs = %d, want 600000", got.OutTimeMs)
	}
	if got.TotalSize != 6156 {
		t.Errorf("TotalSize = %d, want 6156", got.TotalSize)
	}
	if got.Speed != 36.4 {
		t.Errorf("Speed = %v, want 36.4", got.Speed)
	}
}

func TestProgressAccumulatorToleratesUnknownFields(t *testing.T) {
	acc := newProgressAccumulator()
	lines := []string{
		"frame=1",
		"a_completely_unknown_future_key=42",
		"out_time_ms=1000",
		"progress=continue",
	}
	got, ok := feedAll(acc, lines)
	if !ok {
		t.Fatal("a block with one unknown key was rejected entirely")
	}
	if got.FrameCount != 1 {
		t.Errorf("FrameCount = %d, want 1", got.FrameCount)
	}
}

func TestProgressAccumulatorIgnoresAMalformedValueWithoutAbortingTheBlock(t *testing.T) {
	acc := newProgressAccumulator()
	lines := []string{
		"frame=not-a-number",
		"out_time_ms=2000",
		"progress=continue",
	}
	got, ok := feedAll(acc, lines)
	if !ok {
		t.Fatal("a block with one malformed field was rejected entirely")
	}
	if got.OutTimeMs != 2000 {
		t.Errorf("OutTimeMs = %d, want 2000 (the malformed frame field must not corrupt the rest)", got.OutTimeMs)
	}
}

func TestProgressAccumulatorDefaultsFrameCountToMinusOneWhenNeverReported(t *testing.T) {
	acc := newProgressAccumulator()
	lines := []string{"out_time_ms=1000", "progress=end"}
	got, ok := feedAll(acc, lines)
	if !ok {
		t.Fatal("expected a completed block")
	}
	if got.FrameCount != -1 {
		t.Errorf("FrameCount = %d, want -1 for an audio-only branch", got.FrameCount)
	}
}

func TestProgressAccumulatorRejectsAnEmptyBlock(t *testing.T) {
	acc := newProgressAccumulator()
	_, ok := acc.feed("progress=continue")
	if ok {
		t.Error("an all-empty block (nothing parsed) was reported as complete")
	}
}

func TestProgressAccumulatorIgnoresMalformedLines(t *testing.T) {
	acc := newProgressAccumulator()
	_, ok := acc.feed("this line has no equals sign")
	if ok {
		t.Error("a malformed line was reported as a completed block")
	}
}

func TestProgressAccumulatorBoundsLineLength(t *testing.T) {
	acc := newProgressAccumulator()
	huge := make([]byte, maxProgressLineBytes+100)
	for i := range huge {
		huge[i] = 'a'
	}
	_, ok := acc.feed("frame=" + string(huge))
	if ok {
		t.Error("an oversized line was accepted")
	}
}

func TestHasAdvancedRequiresRealMovement(t *testing.T) {
	zero := Progress{FrameCount: -1}
	if zero.hasAdvanced() {
		t.Error("an all-zero progress block reported as advanced")
	}

	withTime := Progress{OutTimeMs: 1}
	if !withTime.hasAdvanced() {
		t.Error("a block with out_time_ms > 0 did not report as advanced")
	}

	withSize := Progress{TotalSize: 1}
	if !withSize.hasAdvanced() {
		t.Error("a block with total_size > 0 did not report as advanced")
	}

	withFrames := Progress{FrameCount: 1}
	if !withFrames.hasAdvanced() {
		t.Error("a block with frame count > 0 did not report as advanced")
	}
}
