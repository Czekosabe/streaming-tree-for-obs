package ffmpeg

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// Captured from a real `ffmpeg -hide_banner -protocols` run (see
// docs/progress.md for how this stage's FFmpeg build was probed), trimmed to
// the parts the parser looks at.
const sampleProtocolsOutput = `Supported file protocols:
Input:
  async
  concat
  crypto
  data
  ffrtmpcrypt
  file
  ftp
  http
  rtmp
  rtmps
  tcp
Output:
  crypto
  fd
  file
  ftp
  http
  rtmp
  rtmpe
  rtmps
  rtmpt
  tcp
`

const sampleMuxersOutput = `Formats:
 D.. = Demuxing supported
 .E. = Muxing supported
 ..d = Is a device
 --
  E  3g2             3GP2 (3GPP2 file format)
 DE  flv             FLV (Flash Video)
 D   matroska        Matroska
`

func TestParseProtocolsFindsInputAndOutputRTMP(t *testing.T) {
	caps := parseProtocols(sampleProtocolsOutput)

	if !caps.RTMPInput {
		t.Error("RTMPInput = false, want true")
	}
	if !caps.RTMPOutput {
		t.Error("RTMPOutput = false, want true")
	}
	if !caps.RTMPSOutput {
		t.Error("RTMPSOutput = false, want true")
	}
}

func TestParseProtocolsDoesNotConfuseInputOnlyRTMPSWithOutput(t *testing.T) {
	// rtmps appears under Input in the sample but not confusingly elsewhere;
	// this asserts the section boundary is actually respected, not just that
	// the substring "rtmps" appears anywhere in the text.
	output := `Supported file protocols:
Input:
  rtmp
  rtmps
Output:
  rtmp
`
	caps := parseProtocols(output)
	if caps.RTMPSOutput {
		t.Error("RTMPSOutput = true, but rtmps was only listed under Input")
	}
}

func TestParseFLVMuxerFindsMuxingSupport(t *testing.T) {
	if !parseFLVMuxer(sampleMuxersOutput) {
		t.Error("parseFLVMuxer() = false, want true")
	}
}

func TestParseFLVMuxerRejectsDemuxOnlyFLV(t *testing.T) {
	output := " D   flv             FLV (Flash Video)\n"
	if parseFLVMuxer(output) {
		t.Error("a demux-only flv entry was reported as a usable muxer")
	}
}

func TestParseFLVMuxerHandlesAbsence(t *testing.T) {
	output := " DE  matroska        Matroska\n"
	if parseFLVMuxer(output) {
		t.Error("parseFLVMuxer() = true, want false when flv is not listed")
	}
}

func TestCapabilitiesMissingListsEveryGap(t *testing.T) {
	caps := Capabilities{}
	missing := caps.Missing()
	if len(missing) != 5 {
		t.Fatalf("Missing() returned %d items, want 5: %v", len(missing), missing)
	}
}

func TestCapabilitiesSatisfiedRequiresEveryFlag(t *testing.T) {
	full := Capabilities{RTMPInput: true, RTMPOutput: true, RTMPSOutput: true, FLVMuxer: true, Progress: true}
	if !full.Satisfied() {
		t.Error("a fully-capable set was reported unsatisfied")
	}

	partial := full
	partial.Progress = false
	if partial.Satisfied() {
		t.Error("a partially-capable set was reported satisfied")
	}
}

// TestProbeExecutableAgainstARealFFmpegBinary exercises the real probe
// pipeline end to end against whatever FFmpeg is actually on PATH. It is
// skipped, not failed, when none is found: the standard test suite must not
// require a real FFmpeg installation, but when one is present in this
// environment this gives real coverage beyond the injected-fake tests above.
func TestProbeExecutableAgainstARealFFmpegBinary(t *testing.T) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH in this environment")
	}

	result, err := probeExecutable(context.Background(), path)
	if err != nil {
		t.Fatalf("probeExecutable() error = %v", err)
	}

	if !strings.HasPrefix(result.versionOutput, "ffmpeg version") {
		t.Errorf("versionOutput = %q, want it to start with \"ffmpeg version\"", result.versionOutput)
	}
	if !result.progressWorks {
		t.Error("progressWorks = false against a real ffmpeg binary")
	}
}
