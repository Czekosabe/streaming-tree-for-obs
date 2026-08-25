# Stage 20E — Windows system-tray icon

**Research date:** 2026-08-25, against learn.microsoft.com's own current API
reference, before writing `internal/runtime/tray`'s Windows implementation -
this project's own standing "contract before implementation" discipline.

Closing the browser tab does not stop the backend (docs/windows-packaging.md
§1: one Go process, the browser is a separate OS component). A desktop
operator with no visible window and no console (`-H=windowsgui`,
docs/windows-packaging.md §7/§13) then has no way to see the application is
still running, reopen it, or quit it, short of Task Manager. The tray icon
is the one persistent, always-available piece of desktop UI that closes this
gap.

## 1. Audit of the existing lifecycle (before design)

Confirmed by direct source reading:

- **Single-instance detection**: `internal/runtime/singleinstance` -
  `CreateMutexW("Local\StreamingTreeForOBS.SingleInstance")`, Windows-only,
  gated on `buildinfo.Packaged()`. Guarantees at most one real backend
  process, and therefore at most one tray icon, per desktop session.
- **Browser launch**: `internal/runtime/browserlaunch` - `ShellExecuteW`
  with the `"open"` verb against an application-generated loopback URL.
  Reused unchanged for the tray's "Open Streaming Tree"/"Open Logs &
  Diagnostics" actions - the tray never starts a second backend, it only
  ever opens a browser tab against the one already-running process.
- **Console-free release build**: `-H=windowsgui` (`scripts/build-release.ps1`)
  - no console for the main process, and (as of the console-flashing fix
  earlier in this remediation cycle) no console flash for any child process
  either (`internal/runtime/procutil`).
- **Native fatal dialog**: `internal/runtime/nativealert` - `MessageBoxW`,
  plain English, never localized. The tray's own menu text follows this
  same precedent (§6).
- **Shutdown/cancel path**: `cmd/server/main.go`'s `ctx, stop :=
  signal.NotifyContext(...)` is the **one** cancellation trigger every
  other shutdown path already converges on - the web UI's `POST
  /api/system/shutdown` (`internal/httpapi/system.go`) and the updater's
  install handoff (`updater.Options.OnHandoffBegun`) both simply call this
  same `stop`. The tray's "Quit Streaming Tree" reuses it identically
  (§4) - there is no second shutdown implementation anywhere in this
  application.
- **Updater handoff**: `updater.Manager.Status(ctx).State` already
  distinguishes every state the tray's "Check for updates" item needs to
  honestly reflect (`docs/updater.md` §11, extended by this remediation
  cycle's §43) - the tray reads this same state, never a second updater.
- **Icon/resource setup**: none exists. No `.ico` file anywhere in the
  repository, no resource embedded into the Windows executable, no
  `IconFilename` in the Inno Setup script's `[Icons]` section (Start Menu
  shortcuts use the executable's own default icon). See §5.
- **Cross-platform build constraints**: this project's established
  per-platform pattern (`internal/runtime/singleinstance`,
  `internal/runtime/browserlaunch`, `internal/runtime/nativealert`,
  `internal/updater/handoff_windows.go`/`handoff_other.go`) is one real
  Windows implementation (`_windows.go`) plus an honest no-op fallback
  (`_other.go`), so every other platform keeps compiling unchanged.
  `internal/runtime/tray` follows this exactly.

## 2. Implementation choice

**Raw Win32 syscalls only** (`Shell32.dll`/`User32.dll` via
`syscall.NewLazyDLL`/`NewProc`/`.Call`, exactly the mechanism
`golang.org/x/sys/windows` itself thinly wraps, and the same one
`internal/runtime/browserlaunch`/`nativealert`/`singleinstance` already
use) - explicitly **not** a third-party systray library, **not** CGO, and
**not** Electron/WebView2/Tauri or a separate helper process. This keeps
the tray auditable within this codebase's own existing dependency
footprint rather than adding a new, unaudited GUI dependency for one
feature.

Every struct layout, numeric constant, and call sequence used
(`NOTIFYICONDATAW`'s exact field order, `NOTIFYICON_VERSION_4`'s modern
per-monitor-DPI-aware callback shape, the documented
`SetForegroundWindow`/`TrackPopupMenu`/`PostMessage(WM_NULL)` menu-dismissal
fix, `CreateIconFromResourceEx`'s required `dwVer = 0x00030000`) was
researched against learn.microsoft.com's current API reference before being
written - a wrong struct layout here is a memory-safety bug, not a compile
error. One documented API, `LookupIconIdFromDirectoryEx`, did not behave as
documented against a real single-frame PNG-compressed `.ico` in practice (it
returned an out-of-range "best fit" id); frame selection is done directly in
Go instead (`selectClosestFrame` in `tray_windows.go`), which is simpler,
fully within this project's own control, and verified working by
`TestLoadIconFromICOBytesRealAsset` against the real embedded icon.

A real, hidden, never-shown top-level window (`WS_EX_TOOLWINDOW`, no
`WS_VISIBLE`, `ShowWindow` never called) is used to receive the tray's
callback messages - not a true `HWND_MESSAGE` message-only window, because
`TrackPopupMenu`'s own documented dismissal fix requires
`SetForegroundWindow` on a real top-level window, which a message-only
window's own documented semantics (no z-order, not enumerable) conflict
with.

The message loop runs on its own goroutine with `runtime.LockOSThread()`
held for its entire lifetime (window handles and message queues are
thread-affine in Win32) - every Win32 UI call this package makes happens on
that one locked thread; `Handle.Stop()` (callable from any goroutine) posts
a private message to request shutdown and blocks until that thread has
actually finished tearing down, so `Stop()` returning is a real guarantee,
not a best-effort signal.

## 3. Menu contract

Exactly the menu the governing task specified, rebuilt fresh every time it
is about to be shown (status text and update-eligibility can both have
changed since the last time):

1. **Open Streaming Tree** - `browserlaunch.Open` against the Dashboard URL.
2. **Open Logs & Diagnostics** - `browserlaunch.Open` against the Dashboard
   URL + `logs` (the frontend's canonical `/logs` route).
3. *(separator)*
4. **Status** (grayed, not clickable) - `"Ingest: Not installed"` /
   `"Ingest: Waiting"` / `"Ingest: Receiving"` / `"Ingest: Not ready"` /
   `"Ingest: Error"`, read directly from the in-process
   `mediamtx.Supervisor.Snapshot()` the rest of the backend already uses -
   never a second ingest-state computation, never an HTTP round-trip to its
   own API.
5. *(separator)*
6. **Check for updates** - delegates to `updater.Manager.CheckNow`; grayed
   (disabled, never clickable-but-refused) exactly when
   `Status(ctx).State` is `disabled`, `manual_build`, or
   `platform_unsupported` (docs/updater.md §11/§35/§43) - the same three
   permanent, non-actionable states the web Updates panel already treats
   this way.
7. *(separator)*
8. **Quit Streaming Tree** - calls `cmd/server/main.go`'s own
   `signal.NotifyContext` `stop` function directly (see §1) - the exact
   same graceful-shutdown path the web UI's Quit action and the updater's
   install handoff both already use.

No "Start/Stop All Streams" item, as instructed. The optional "Copy
Dashboard URL" item was considered and dropped - see §7.

## 4. Lifecycle and invariants

- **One tray icon per desktop instance**: guaranteed transitively by
  `singleinstance.Acquire()` (§1) - a second launch never reaches the tray-
  creation code at all, it detects the first instance and exits.
- **Never in `--headless` mode**: gated on `!headlessMode` in
  `cmd/server/main.go`, the same condition (plus `!cfg.TestNoUI`, so
  automated test/CI runs never spawn a real native tray icon/message loop)
  that already gates the real browser launch.
- **No zombie icon after failure or shutdown**: `Handle.Stop()` is called
  from `shutdownRuntime` (`cmd/server/main.go`), the one function every
  shutdown path already converges through - web UI Quit, tray Quit,
  Ctrl+C/SIGTERM, and the updater's install handoff all call it. `Stop()`
  itself blocks until the tray's own `NIM_DELETE`/`DestroyMenu`/
  `DestroyIcon`/`DestroyWindow`/`UnregisterClassW` teardown sequence has
  actually completed (§2) before returning, and a failed `tray.Run` (e.g. a
  real Win32 API failure) leaves `trayHandle` `nil`, so `shutdownRuntime`
  simply skips it - there is nothing to leak in that case either.
- **Web Quit removes it / tray Quit terminates cleanly / updater shutdown
  removes it**: all three are the same one fact, restated for each trigger
  - every one of them calls the same `stop()`, which unblocks `<-ctx.Done()`
  in `run()`, which calls `shutdownRuntime`, which calls `trayHandle.Stop()`
  unconditionally, first, before any other manager's own shutdown.
- **Explorer-restart resilience**: the tray also listens for the documented
  `"TaskbarCreated"` broadcast message (`RegisterWindowMessageW`) and
  re-adds itself when it arrives - the documented signal that Explorer
  itself restarted and silently dropped every previously-added tray icon.
- **Tray support itself causes no console flash**: every `os/exec` call
  this package could plausibly need was avoided entirely (no child process
  is ever spawned by the tray itself); the icon is loaded from an embedded
  `.ico` via `CreateIconFromResourceEx`, not an external tool.

## 5. Icon/branding: an honest limitation

No final branding art exists for this project - see
`apps/web/src/components/layout/BrandMark.tsx`'s own doc comment ("No
third-party logo or artwork is used anywhere in the application"). The tray
icon (`internal/runtime/tray/assets/tray.ico`, generated by
`scripts/generate-tray-icon.go`) reuses BrandMark's own existing neutral
first-party visual identity - the rounded-square gradient from
`--color-accent` (`#8b5cf6`) to `--color-accent-deep` (`#6d28d9`) - rather
than inventing new artwork, but renders a plain white ring-and-dot glyph
instead of attempting to reproduce BrandMark's Lucide "Network" icon
pixel-for-pixel: a faithful reproduction of a multi-path vector glyph is
illegible at real tray sizes (16x16 on a real Windows notification area, the
size this package actually requests - see `selectClosestFrame`), and an
approximation would be dishonest as "the app's icon" rather than a
deliberately simple placeholder. **This should be replaced with real,
designed branding art once that exists** - `scripts/generate-tray-icon.go`
is a one-off, `//go:build ignore` tool with no other purpose than producing
this placeholder, and is no longer needed once real art replaces
`tray.ico`.

## 6. Localization

Tray menu text is plain English only, never run through the frontend's
i18n system - this mirrors `internal/runtime/nativealert`'s own existing
precedent (its `MessageBoxW` title/message are plain Go strings too).
Native OS-level UI in this application has never been localized; only the
web frontend is. This was not part of the governing task's explicit
localization requirement for this remediation cycle (which named the
updater eligibility work specifically, docs/updater.md §43) - restated here
as a real, honest limitation rather than an oversight.

## 7. What was dropped, and why

An optional "Copy Dashboard URL" clipboard item was implemented and then
removed. Its correct implementation requires converting a `uintptr` (from
`GlobalLock`'s own return value) to `unsafe.Pointer` across two separate
Go statements - `go vet`'s `unsafeptr` check only recognizes this pattern
as safe when it appears directly inside a call to the raw `syscall.Syscall`
family, not through `syscall.LazyProc.Call` (the mechanism this whole
package otherwise consistently uses, matching this codebase's existing
Windows-syscall style). Restructuring just this one optional feature to use
raw `syscall.Syscall` instead would have introduced a second, inconsistent
calling convention into the file for a feature the governing task itself
marked optional ("if simple") - it turned out not to be simple without that
inconsistency, so it was dropped rather than compromising either
correctness (a `go vet`-failing build) or consistency.

## 8. Test strategy

Real, on-platform tests (`tray_windows_test.go`, Windows CI only):
`loadIconFromICOBytes` is exercised against the real embedded `IconICO`
bytes and confirmed to produce a real, destroyable `HICON` - not a mock, a
genuine Win32 call sequence proven to work; a table of malformed inputs
(empty, truncated header, truncated directory) confirms it fails safely
rather than panicking or reading out of bounds. `copyUTF16`'s truncation
and NUL-termination behavior is unit-tested directly. The full click-
handling/menu-building/message-loop machinery is not unit-testable in the
ordinary sense (it requires a real desktop session and a human or scripted
click), and is instead covered by the Stage 20E manual-verification
checklist (`docs/manual-verification.md`) once a new installer is built
from this commit.

## 9. Known limitations

- No real, designed branding art - see §5.
- No Polish localization - see §6.
- No "Copy Dashboard URL" item - see §7.
- No balloon/toast notifications from the tray (e.g. "a new version is
  available") - not requested by the governing task, and the existing
  in-browser update banner (docs/updater.md §32) already covers this need
  when the Dashboard is open.
