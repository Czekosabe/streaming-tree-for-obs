package branch

import "strings"

// rwTimeoutMicros bounds FFmpeg's generic AVIO timeout (-rw_timeout,
// microseconds), applied to the output side.
//
// No RTMP-specific reconnect or timeout option exists in this application's
// probed FFmpeg build (checked via `ffmpeg -h protocol=rtmp` while
// implementing this stage - see docs/progress.md); -rw_timeout is the
// generic AVIO option that does apply regardless of protocol. It bounds a
// hung network write; the restart policy (manager.go) is what actually
// recovers the branch afterward.
const rwTimeoutMicros = "15000000" // 15 seconds

// buildDestinationURL joins a configured server address with a stream key.
//
// Built as late as practical, immediately before starting a process, and
// never logged, stored, or returned from any exported function beyond the
// caller that starts the process - see manager.go's launch step.
func buildDestinationURL(serverURL, streamKey string) string {
	return strings.TrimRight(serverURL, "/") + "/" + streamKey
}

// buildArgs constructs the FFmpeg argument list for one branch.
//
// destinationURL is the only argument that carries a secret (the stream
// key). Nothing in this package logs the full argument list - see
// process.go, which redacts before it ever reaches a log line.
//
// Stream copy only: this stage never transcodes. -map 0:v? and -map 0:a?
// each carry the "?" optional-stream marker, so a branch whose source has no
// video (or no audio) still starts rather than failing to find a stream to
// map. A source codec the FLV/RTMP output cannot carry without transcoding
// is not handled specially here - it is FFmpeg's own -c copy failing fast
// with a clear "codec not currently supported in container" error, which
// this application treats as a normal, immediate exit, not something to
// crash-loop against; see CodeUnsupportedCodec.
func buildArgs(inputURL, destinationURL string) []string {
	return []string{
		"-hide_banner",
		"-nostdin",
		"-loglevel", "warning",
		"-i", inputURL,
		"-map", "0:v?",
		"-map", "0:a?",
		"-c", "copy",
		"-f", "flv",
		"-rw_timeout", rwTimeoutMicros,
		"-progress", "pipe:1",
		destinationURL,
	}
}
