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

## 5. Icon/branding

**Updated 2026-08-25** (Stage 20E branding remediation): a real physical/
manual Windows test found two real defects - the tray icon had no hover
tooltip at all, and the packaged app still used the neutral placeholder
identity from §5's original text below rather than the project's real,
final logo, once the operator provided one. Both are fixed; the original
placeholder-icon limitation this section used to describe no longer applies
and is kept below (§5a) only as history.

The operator-provided source logo
(`assets/branding/streaming-tree-logo.png`, 1024x1024, real alpha
transparency) is the **one** canonical branding asset in this repository -
every icon surface derives from it, never a second copy:
`scripts/generate-branding-assets.go` (`//go:build ignore`, superseding the
old `generate-tray-icon.go`) crops it to the emblem alone (the wordmark text
is illegible at 16x16 and is deliberately excluded from every icon-sized
derivative, per the same reasoning §5a already established) and area-average
downsamples it to a standard multi-size set (16/24/32/48/64/128/256),
writing two files: `internal/runtime/tray/assets/tray.ico` (the tray icon)
and `apps/web/public/favicon.ico` (the browser tab icon - see §5b).

`tray.ico` is then the single source for every other Windows icon surface
too, never regenerated independently for each one:

- **Executable icon** (Explorer, Alt-Tab, the .exe's own Properties dialog):
  `apps/server/cmd/server/rsrc_windows_amd64.syso`, a checked-in Windows
  resource object generated by `go-winres` (github.com/tc-hib/go-winres,
  pure Go, no gcc/`windres` needed - installed and run once, not a runtime
  dependency of the server binary) pointed at `tray.ico`. Go's toolchain
  links any `*.syso` file present in a main package's own directory
  automatically - no `build-release.ps1` change, no new build step. The
  `_windows_amd64` suffix restricts it to `windows/amd64` builds via the
  exact same filename-based build-constraint mechanism every `_windows.go`
  file in this codebase already uses; cross-compiling `GOOS=linux`/`darwin`
  was re-verified unaffected. Also carries `ProductName`/`FileDescription`
  = "Streaming Tree for OBS" in the exe's own version-info resource (visible
  in Explorer's Properties → Details tab) - see
  `apps/server/cmd/server/README-icon.txt` for the exact regeneration
  command and why `FileVersion`/`ProductVersion` are a static placeholder,
  not the real per-build version string.
- **Installer icon** (the compiled `Setup.exe` itself):
  `scripts/installer/streaming-tree.iss`'s `SetupIconFile` directive, also
  pointed directly at `tray.ico`.
- **Start Menu shortcut / "Apps & Features" entry**: no new directive
  needed - both already default to the target executable's own icon, which
  now carries the real artwork via the `.syso` resource above. Verified
  directly: a real (non-hermetic, cleaned up immediately after) silent
  install was performed, the real `%AppData%\...\Start Menu\Programs\...`
  `.lnk` file's `IconLocation` was confirmed to read `,0` (index 0 of the
  target exe's own resource, not a separate `.ico` reference), and its
  rendered icon was extracted and visually confirmed to be the real logo,
  before the same install was silently uninstalled to leave no trace.
- **Taskbar icon**: this application has no traditional taskbar-visible
  native window (the hidden tray-support window is never shown - see §2);
  the closest real analog is the browser tab/window showing the Dashboard,
  covered by the favicon (§5b) rather than a native taskbar icon that does
  not otherwise exist for this architecture.

## 5a. Original placeholder-icon design notes (superseded, kept for history)

Before the operator provided a real logo, no final branding art existed for
this project - see `apps/web/src/components/layout/BrandMark.tsx`'s own doc
comment ("No third-party logo or artwork is used anywhere in the
application"). The tray icon reused BrandMark's own existing neutral
first-party visual identity - the rounded-square gradient from
`--color-accent` (`#8b5cf6`) to `--color-accent-deep` (`#6d28d9`) - rather
than inventing new artwork, but rendered a plain white ring-and-dot glyph
instead of attempting to reproduce BrandMark's Lucide "Network" icon
pixel-for-pixel: a faithful reproduction of a multi-path vector glyph is
illegible at real tray sizes (16x16), and an approximation would have been
dishonest as "the app's icon" rather than a deliberately simple placeholder.
This reasoning (real content only, no invented approximation) carried
forward directly into §5's emblem-vs-wordmark crop decision once the real
logo replaced it.

## 5b. Web favicon

`apps/web/public/favicon.ico` (the same multi-size set `tray.ico` carries)
is linked from `apps/web/index.html` via `<link rel="icon" href="/favicon.ico"
sizes="any" />`, and copied into the production build output automatically
by Vite's own `public/` convention (no build-script change needed) - the
one piece of "if appropriate and straightforward" web-branding consistency
the governing task asked for.

## 5c. The tray tooltip bug: root cause and fix

The tray icon's hover tooltip never appeared - a real bug, found by
re-reading this package's own `NOTIFYICONDATAW` construction against the
documented `NIF_*` flag table, not by guessing. `addIcon()` requests
`NOTIFYICON_VERSION_4` (§2 - the modern, per-monitor-DPI-aware callback
shape) immediately after `NIM_ADD`, but Windows' own documented behavior for
that version is that it **suppresses the standard `szTip` tooltip by
default**, replacing it with a richer pop-up mechanism this package never
implements, unless the icon is also flagged `NIF_SHOWTIP`
(`0x00000080`) - which the original implementation never set. Fixed by
adding `NIF_SHOWTIP` to the flags `NIM_ADD` is issued with
(`addIconFlags()` in `tray_windows.go`, pulled into its own small pure
function specifically so this exact regression is unit-testable -
`TestAddIconFlagsIncludesShowTip`). The tooltip text itself
(`buildinfo.ProductName`, i.e. "Streaming Tree for OBS") was already being
set correctly via `szTip` before this fix - the bug was purely that Windows
was never displaying it.

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

- No Polish localization - see §6.
- No "Copy Dashboard URL" item - see §7.
- No balloon/toast notifications from the tray (e.g. "a new version is
  available") - not requested by the governing task, and the existing
  in-browser update banner (docs/updater.md §32) already covers this need
  when the Dashboard is open.
- The exe's embedded `FileVersion`/`ProductVersion` (§5) are a static
  placeholder, not the real per-build version - see
  `apps/server/cmd/server/README-icon.txt`.
- No `CompanyName` in the exe's version-info resource - deliberate, matches
  `buildinfo.CreatorName`'s own established policy of never asserting a real
  legal/company name (docs/product-identity-legal.md).
