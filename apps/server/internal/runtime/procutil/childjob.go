// Package procutil (see procutil.go) also provides AssignToChildJob -
// see this file's own doc comment for why it exists.
package procutil

import "os/exec"

// AssignToChildJob makes a best-effort attempt to ensure cmd's
// already-started process cannot outlive this server process even if
// it terminates ungracefully - real, verified protection on Windows
// (see childjob_windows.go); a documented no-op everywhere else for
// now (see childjob_other.go for exactly what that does and does not
// claim about other platforms).
//
// This exists because of a real bug a physical/manual Windows retest
// found: MediaMTX (and, by the same mechanism, a branch's own FFmpeg
// process) can be left running, still bound to its RTMP port, after
// the parent streaming-tree-server.exe process is gone - not through
// this application's own graceful shutdown path (which already stops
// every owned child correctly, confirmed by this project's own
// verify-packaged-app.mjs/verify-installer.mjs), but through any
// ungraceful parent termination Windows itself allows to happen
// without any cooperation from the dying process: a crash, an
// unhandled panic, Task Manager "End Task", or anything else that
// closes the process's handles without it having a chance to run its
// own shutdown sequence. Windows has no automatic parent-child process
// lifetime link by default - a child survives its parent's death
// unless something explicit prevents that.
//
// See childjob_windows.go for the real fix (a Windows Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE) and childjob_other.go for why
// this is a no-op everywhere else (a genuine platform difference, not
// a deferred implementation - see that file's own doc comment).
func AssignToChildJob(cmd *exec.Cmd) error {
	return assignToChildJob(cmd)
}
