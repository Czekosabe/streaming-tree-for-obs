package branch

import (
	"strings"
	"testing"
)

func TestBuildDestinationURLJoinsServerAndKey(t *testing.T) {
	got := buildDestinationURL("rtmp://example.invalid/app", "sk_live_abc")
	if got != "rtmp://example.invalid/app/sk_live_abc" {
		t.Errorf("got %q", got)
	}
}

func TestBuildDestinationURLTrimsATrailingSlash(t *testing.T) {
	got := buildDestinationURL("rtmp://example.invalid/app/", "sk_live_abc")
	if got != "rtmp://example.invalid/app/sk_live_abc" {
		t.Errorf("got %q", got)
	}
}

func TestBuildArgsUsesStreamCopyAndFLV(t *testing.T) {
	args := buildArgs("rtmp://127.0.0.1:1935/live", "rtmp://example.invalid/app/sk_live_abc")

	joined := strings.Join(args, " ")
	for _, want := range []string{"-c copy", "-f flv", "-progress pipe:1", "-nostdin", "-hide_banner"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %v missing %q", args, want)
		}
	}
}

func TestBuildArgsPlacesTheDestinationLast(t *testing.T) {
	destination := "rtmp://example.invalid/app/sk_live_abc"
	args := buildArgs("rtmp://127.0.0.1:1935/live", destination)

	if args[len(args)-1] != destination {
		t.Errorf("last arg = %q, want the destination URL", args[len(args)-1])
	}
}

func TestBuildArgsMapsOptionalVideoAndAudio(t *testing.T) {
	args := buildArgs("rtmp://127.0.0.1:1935/live", "rtmp://example.invalid/app/key")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "0:v?") || !strings.Contains(joined, "0:a?") {
		t.Errorf("args %v should map optional video and audio streams", args)
	}
}
