// Package procutil holds small helpers for spawning child processes
// correctly across every platform this project supports.
//
// HideConsoleWindow exists because of a real bug a physical/manual
// Windows test found (Stage 20E): the packaged server binary is built
// with -H=windowsgui (docs/windows-packaging.md §7/§13) specifically so
// it never owns a console window of its own, but every child process it
// spawns via os/exec - FFmpeg's capability probes, MediaMTX's own
// --version probe, the updater's install/restart commands and its
// update-helper - is an ordinary console-subsystem executable. Without
// an explicit hint, Windows allocates a brand new console for a child
// like that and briefly shows it: a console window flashing on screen
// for an instant, with no connection to anything the operator did.
// FFmpeg's own real per-branch publish process and MediaMTX's own
// server process already build their SysProcAttr directly in their
// platform-specific process_windows.go (CREATE_NEW_PROCESS_GROUP, for
// signal-delivery reasons unrelated to this) - HideWindow is added
// there directly rather than through this package, to keep that one
// struct construction in one place.
//
// Every other os/exec.Cmd this project constructs for a child process
// that might run on Windows calls HideConsoleWindow(cmd) immediately
// after construction, before Start/Run/Output/CombinedOutput.
package procutil
