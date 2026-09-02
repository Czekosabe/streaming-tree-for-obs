# Stage 20A — Windows packaging contract

**Research date:** 2026-08-18. Written before any Stage 20A product code, per
this project's own standing "contract before implementation" discipline.

Stage 20A transforms the current developer-only two-process workflow (Go
backend + `npm run dev` Vite server) into a real, installable Windows
desktop application: one Go process serving the production React build,
launched through the user's own default browser. It does **not** introduce
Electron/WebView2/Tauri/Chromium, does **not** implement update checks, and
does **not** produce a public release.

## 0. Audit of the real current runtime (before design)

Confirmed by direct source reading, not assumption:

- **Server lifecycle** (`apps/server/cmd/server/main.go`): `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` is the only shutdown trigger today. A single, already-correct, carefully-ordered graceful-shutdown sequence exists in the `case <-ctx.Done()` branch (branches → device-flow/YouTube-auth/engagement managers → operator chat → chat overlay → outbound chat/chat automation/alerts/audio/goals/supporter-widgets → event bus → account validation worker → MediaMTX supervisor, then `server.Shutdown`). There is no HTTP-triggerable shutdown path today.
- **Listen address**: `internal/config.Config` defaults to `127.0.0.1:8080` (already loopback-only, never `0.0.0.0`). `STREAMING_TREE_PORT`/`STREAMING_TREE_HOST` override it. If the port is already taken, `server.ListenAndServe()` fails, the already-started subsystems are shut down, and the process exits with an error - there is no silent fallback to a different port today.
- **Routing** (`internal/httpapi/router.go` and every `register*Routes` function across the package): every real, registered backend route is either exactly `GET /{$}` (a tiny liveness JSON blob) or begins with `/api/` - confirmed by enumerating every `mux.HandleFunc` pattern in the package. Every public/binary/SSE endpoint (chat overlay config/items/stream, alert config/stream, widget config/stream, public audio stream/bytes/ack, public visual-asset bytes, engagement/operator-chat SSE) already lives under `/api/public/...` or `/api/...` - there is no separate, ambiguous namespace to reconcile. `/api/` itself has its own catch-all returning a JSON 404 for any unmatched sub-path. **There is no existing static-file serving, no embedded frontend, and no SPA-fallback route anywhere in the backend today.**
- **Frontend routing** (`apps/web/src/App.tsx`): every management route (`/`, `/platforms`, `/streams`, `/metadata`, `/engagement`, `/chat`, `/overlays`, `/automation`, `/alerts`, `/audio`, `/goals`, `/settings`, `/settings/about`, `/logs`, the two Designer routes) and every public overlay route (`/overlay/chat/:publicSlug`, `/overlay/alerts/:publicSlug`, `/overlay/audio/:publicSlug`, `/overlay/widgets/:publicSlug`) is a **React Router client-side route** - none of them exist as a backend route. In development, Vite's dev server serves `index.html` for all of these and React Router takes over; production serving must reproduce exactly that fallback behavior for anything outside `/api/`.
- **Vite config** (`apps/web/vite.config.ts`): dev server proxies `/api` to `http://127.0.0.1:8080` (overridable via `VITE_DEV_API_PROXY_TARGET`), so the frontend already only ever calls relative `/api/...` URLs - same-origin API calls already work naturally and require no frontend change for production. Build output: `dist/`, with sourcemaps enabled.
- **Build/browser-launch/CLI infrastructure**: no existing browser-launch helper anywhere in `apps/server`. No `flag`/`os.Args` parsing in either `cmd/server` or `cmd/testserver` - no `--version` today. No `.github/workflows`. No existing build/release/package script under `scripts/`. `.gitignore` already ignores `dist/`, `*.exe`, and Go binaries - nothing packaging-related is committed today.
- **`internal/buildinfo`**: already the single canonical source for `ProductName`, `CreatorName`, `RepositoryURL`, `CreatorURL`, `SupportURL`, `ApplicationLicenseSPDX`/`Name`, `Version` (`"0.1.0"`, hand-maintained), `IsReleaseBuild` (`false`), and `CommitInfo()` (Go's own automatic VCS build-info stamping). This is reused, not replaced.
- **`testserver`/integration-test convention**: `apps/server/cmd/testserver/main.go` is gated by the `integration` build tag - invisible to `go build ./...`/`go vet ./...`/`go test ./...`/a normal `go build ./cmd/server`. This is the established, deliberate precedent for "test-only behavior must not be reachable by an accidental environment variable" (see that file's own doc comment). Stage 20A's packaged-mode/browser-launch-suppression mechanism follows the same discipline.
- **Windows-specific precedent**: `internal/provider/tts/windows.go` (`//go:build windows`) and `internal/provider/tts/stub.go` (`//go:build !windows`) are the established pattern for OS-specific code in this repository - one real Windows implementation, one no-op/fallback for other platforms, so non-Windows builds keep compiling.

## 1. Production runtime model

**One Go process.** The production React build is embedded into the Go
binary via `embed.FS` (see §2). At runtime there is exactly one process:
the Go backend, serving the API, the public overlay data endpoints, and the
production frontend, all on the same loopback origin. The user's default
system browser is a separate, already-installed OS component, not an
application-owned process.

## 2. Frontend packaging model

**Chosen: embed, with a committed placeholder so `go build`/`go test`
never require Node.**

`apps/server/internal/webassets/embedded/` is a new directory containing a
`//go:embed all:embedded` directive. On a clean checkout it contains only a
placeholder file (`.gitkeep`, tracked; everything else in that directory is
git-ignored via a scoped `.gitignore` rule). `go build ./...`/`go test
./...`/`go vet ./...` never require Node, npm, or a Vite build to succeed -
the embed compiles against the placeholder and packaged/production static
serving simply has nothing meaningful to serve until a real release build
runs. The release script (§9) is the only thing that ever overwrites that
directory with the real `apps/web/dist` output before `go build`s the
release binary - so `git status` stays clean for ordinary development, and
the generated frontend is never committed.

Rejected: **committing `dist/`** (defeats reproducibility - a committed
build artifact can silently drift from its own source; every prior stage of
this project has avoided committing generated output). **A sidecar
directory beside the executable** was considered and is architecturally
equally valid per this task's own "one process does not require one
physical file" framing, but embedding was chosen because it gives true
single-file executable distribution (simpler installer staging, no risk of
an installer accidentally shipping a mismatched exe/assets pair, no
"assets missing beside the exe" runtime failure mode) at negligible
executable-size cost (the production frontend bundle is ~1.1 MB gzip
today).

## 3. Development mode preserved

`npm run dev` (Vite + HMR) and `go run ./cmd/server`/`go build ./cmd/server`
remain fully supported exactly as today. Vite continues proxying `/api` to
the Go backend. Nothing about the development workflow changes; Stage 20A
is strictly additive. The embedded placeholder (§2) means a developer who
has never run `npm run build` sees no difference in the dev workflow at
all - the embed only matters for the packaged/production serving path,
which a developer does not use.

## 4. Production routing

Implemented as one new middleware-adjacent handler registered **after**
every `/api/` route and the existing `GET /{$}` liveness route, so it never
shadows them:

- a request path is served from the embedded frontend's build output
  (`index.html`, hashed JS/CSS, other static assets) when a matching file
  exists in the embedded filesystem, with content-type/caching per §4.1;
- any other non-`/api/` path (i.e. every React Router client route, known
  or future) receives `index.html` - the exact SPA-fallback Vite already
  provides in development;
- `/api/...` is **never** touched by this fallback - the existing `/api/`
  catch-all (JSON 404) remains authoritative, unchanged;
- path traversal (`..`, absolute paths, backslashes) is rejected before
  any filesystem lookup, using `path.Clean`+prefix validation against the
  embedded `fs.FS` root - `fs.FS` itself already refuses to resolve
  outside its own root, but the check is defense in depth, not reliance on
  that alone;
- no directory listing (`http.FileServer`'s own directory-listing
  behavior is disabled by never serving a bare directory path - only
  `index.html`/real files/the SPA fallback are ever returned).

### 4.1 Caching

- hashed, content-addressed assets (Vite's own `assets/*-[hash].js`/`.css`
  naming) are served with a long, immutable `Cache-Control` (`public,
  max-age=31536000, immutable`) - safe because the filename itself changes
  whenever the content does;
- `index.html` is served with `Cache-Control: no-cache` so a browser
  always revalidates it - it is the one file whose content can legitimately
  change between releases while its own URL never does.

## 5. Local production origin

Unchanged from today: `127.0.0.1` only, port `8080` by default, configurable
via the existing `STREAMING_TREE_HOST`/`STREAMING_TREE_PORT` environment
variables. Packaged mode does not introduce a new default. If the
configured port is already occupied, the existing hard-failure behavior is
preserved rather than silently rebinding elsewhere - OBS Browser Source
URLs are persisted outside the application and must not be invalidated by
an unannounced port change (see §7 for what packaged mode does with that
failure). Remote-server/non-loopback configuration remains conceptually
possible via the existing environment variables but is explicitly out of
scope for Stage 20A hardening.

## 6. Browser-launch model

**Windows mechanism: `ShellExecuteW` (`shell32.dll`), verb `"open"`,**
confirmed against the official Microsoft Learn API reference
(`learn.microsoft.com/windows/win32/api/shellapi/nf-shellapi-shellexecutew`)
- the documented, standard way to open a URL with its OS-registered
default handler (the default browser, for an `http://` URL). Called via
`golang.org/x/sys/windows` (already an indirect module dependency of this
project, BSD-3-Clause), never via `cmd /c start` or another shell
invocation - no shell is involved, so there is no shell-injection surface,
and the only string ever passed is the application's own generated
loopback URL (`http://127.0.0.1:<port>/`), never anything derived from
user/browser input.

New package `internal/runtime/browserlaunch`: `browserlaunch_windows.go`
(`//go:build windows`) implements it for real; `browserlaunch_other.go`
(`//go:build !windows`) is a documented best-effort fallback (`xdg-open` on
Linux, `open` on macOS, called with a fixed argv - never a shell string) so
non-Windows developer builds keep compiling and behave reasonably, even
though Stage 20A's actual packaging target is Windows-first. Launch failure
is always logged and never fatal - the application remains fully usable by
manually navigating to the printed URL.

**Launched exactly once, only in packaged mode, only after the HTTP
listener is confirmed ready** (after `net.Listen` succeeds, immediately
before entering the request-serving goroutine) - never in development
mode, never speculatively before the server can actually answer. An
integration-test-only suppression mechanism (§13) lets automated tests
exercise the packaged startup path without a real browser window opening
on the CI/dev machine.

## 7. Windows process/console decision

**Chosen: Option B - a normal no-console Windows GUI-subsystem release
binary, with a genuinely usable in-app Quit action and a native startup-
error dialog.** This is the preferred, polished direction per the governing
task, implemented for real rather than merely hiding failures behind a
linker flag:

- the release binary is built with `-ldflags "-H=windowsgui"`, so no
  console window appears when launched from Explorer/Start Menu;
- **development and integration-test binaries remain ordinary console
  applications** (`cmd/server` unchanged; the GUI-subsystem flag is only
  ever passed by the release build script, never baked into the source);
- fatal startup errors (port already occupied by something else, unusable
  data directory, migration failure, unreadable/corrupt embedded static
  assets) are shown via a native Windows message box
  (`user32.dll`'s `MessageBoxW`, called through
  `golang.org/x/sys/windows` exactly like `ShellExecuteW` above - no
  secrets, tokens, stream keys, or raw environment values are ever
  included in the message text, only a short, sanitized description and
  the log file location) instead of silently disappearing;
- an explicit in-app **"Quit Streaming Tree"** action (§8) is the normal
  way to end a running packaged instance, since there is no console window
  to Ctrl+C.

## 8. Application shutdown model

A new, narrow, same-origin-protected endpoint:

```
POST /api/system/shutdown
```

- registered unconditionally (like `/api/health`/`/api/about`), needs no
  service dependency;
- **requires an exact, non-simple JSON body** (`{"confirm":true}`, `
  Content-Type: application/json`) - an ordinary HTML `<form>` cannot
  submit `application/json` as a "simple request" per the Fetch
  standard's own CORS-preflight rules, so a foreign page cannot trigger
  this endpoint merely by having a user click a link or auto-submitting a
  form; a real cross-origin `fetch()` POST with a JSON body triggers a
  CORS preflight, and the existing `withCORS` middleware's origin
  allowlist (`cfg.AllowedOrigins`, defaulting to the loopback dev-server
  origins only) rejects it before the real request is ever sent;
  additionally the handler itself re-validates `Origin`/absence of
  `Origin` is only accepted for same-origin browser navigation, and
  `GET`/any other method is rejected outright (`405`);
- body is bounded (`http.MaxBytesReader`, a few hundred bytes) and
  strictly parsed (unknown fields rejected, matching every other write
  endpoint's convention in this codebase);
- on success, cancels the **same** `context.CancelFunc` `main.go` already
  gets from `signal.NotifyContext` - so the shutdown endpoint reuses the
  exact existing, already-correct `<-ctx.Done()` graceful-shutdown
  sequence (§0) rather than duplicating it. A second call while shutdown
  is already in progress is a safe no-op (the cancel func is idempotent;
  a small internal flag also makes the handler itself return `200`
  immediately for a repeat call instead of trying to cancel twice);
- frontend: a **"Quit Streaming Tree"** action added to the About & Legal
  page (§14), behind an explicit confirmation dialog, calling this
  endpoint and then showing a "Streaming Tree has stopped - you may close
  this browser tab" state rather than trying to auto-close the tab
  (browsers do not reliably allow a script to close a tab it did not
  itself open).

**Closing a browser tab never stops the backend** - there is no
tab-visibility/`beforeunload`-triggered shutdown anywhere in this design;
the backend only stops via this explicit endpoint, an OS signal, or the
existing `<-ctx.Done()` path.

## 9. Second-launch/single-instance behavior

**Windows mechanism: a named mutex via `CreateMutexW`**, confirmed against
the official Microsoft Learn API reference
(`learn.microsoft.com/windows/win32/api/synchapi/nf-synchapi-createmutexw`)
- the documented standard single-instance detection pattern: the first
process creates the named mutex and owns it for its lifetime; a second
process's `CreateMutexW` call with the same name succeeds but
`GetLastError()` returns `ERROR_ALREADY_EXISTS`, which is the reliable
signal that another instance is already running (not "something happens to
answer on the HTTP port," which this design does not use as instance
proof). The mutex name is a fixed, application-specific string (not derived
from user input), created only in packaged mode.

**Behavior:** the second launch detects `ERROR_ALREADY_EXISTS`, opens/
focuses the local management URL via the same `browserlaunch` mechanism as
§6 (a fresh tab to the verified existing instance, since reliably focusing
an *existing* browser tab is not something this application controls), and
exits cleanly with no attempt to start a second backend, bind the port
again, or touch the database. The mutex is process-lifetime-scoped (closed
automatically by Windows when the owning process exits, including a crash)
- no explicit stale-lock recovery code is needed, matching the documented
behavior of `CreateMutexW` itself.

Non-Windows developer builds compile a no-op single-instance check
(`!windows` build tag, same pattern as §6) and are not otherwise affected.

## 10. Version/build metadata model

`internal/buildinfo.Version` remains the one canonical application version.
Release builds inject a real value via `-ldflags "-X
.../buildinfo.releaseVersion=<value> -X
.../buildinfo.releaseCommit=<value>"`; `buildinfo.Version`/`CommitInfo()`
prefer the injected value when present and fall back to the existing
`"0.1.0"`/Go build-info-stamping behavior otherwise - **no** `1.0.0` public
release version is invented by this milestone. Release automation uses an
explicit development/test package version such as `0.1.0-dev+<short
commit>` (see §16 for the installer-required numeric mapping).

**`--version` CLI flag** (`cmd/server`, using the standard library `flag`
package - the first CLI-flag parsing this binary gains): prints product
name, version, commit (when known), and the SPDX licence identifier, then
exits `0` without starting any application service (no database open, no
MediaMTX supervisor, no HTTP listener) - safe to run against an installed
binary as an installer/smoke-test check.

Frontend continues to obtain the version exclusively from `GET /api/about`
(already the case since the prior product-identity milestone) - it never
invents its own version string.

## 11. Product identity in Windows

Reused verbatim from `internal/buildinfo` (§16 of the governing task):
Application = "Streaming Tree for OBS", Publisher/Creator = "Czekosabe",
Licence = `GPL-3.0-or-later`, Repository/Support URLs unchanged. The
Inno Setup script (§12) sets `AppPublisher=Czekosabe` and
`AppPublisherURL=https://github.com/Czekosabe` - never a Git email, Windows
username, or build-machine username. Publisher text is a plain identity
string; it never claims or implies Authenticode verification (§18 - there
is no certificate).

## 12. Windows installer comparison and selection

Compared, from each project's own current official documentation
(2026-08-18):

| | **WiX Toolset** | **NSIS** | **Inno Setup (selected)** |
| --- | --- | --- | --- |
| Output | Real MSI + Burn bundles | EXE | EXE |
| Build tooling | .NET SDK + MSBuild (`dotnet tool install wix`) - **not present on this dev machine**, confirmed by direct check | Standalone compiler, no SDK | Standalone compiler (~1.78 MB), no SDK |
| Licence | Open source + an "Open Source Maintenance Fee" model (FireGiant-stewarded docs) | Free/open source | Free; commercial users "requested" (not required) to buy a licence - acceptable for a GPL open-source project |
| Per-user install | Supported, but configured through more verbose "bundle scope" concepts | Supported, hand-scripted | **First-class, explicitly documented** ("extensive support for both administrative and non-administrative installations") |
| Silent install/uninstall | Supported | Supported (`/S`) | **First-class, explicitly documented** |
| Stable upgrade identity | `UpgradeCode` GUID (MSI-native) | Hand-rolled (registry-key convention scripted manually) | **First-class `AppId` GUID**, declaratively |
| Code-signing hook | Supported via Burn/MSBuild | Supported via post-build step | **First-class `SignTool=` directive**, insertable later without redesign |
| Scripting model | XML + WiX-specific extensions/Burn bootstrapper | Own imperative scripting language | Declarative `[Setup]`/`[Files]`/`[Icons]`/`[Run]` sections + optional Pascal scripting only where needed |
| Learning curve | Steepest - "spans five tutorial sprints," deep Windows Installer protocol knowledge expected | Moderate - lower-level, more manual work for the same guarantees | Lowest for this project's actual needs |
| CI suitability here | Blocked today (no .NET SDK installed) | Fine | Fine - installed via `winget install --id JRSoftware.InnoSetup --scope user` (official GitHub-hosted release, hash-verified by winget itself) |

**Selected: Inno Setup.** It is the only candidate that satisfies every
requirement (per-user install, no elevation, silent install/uninstall,
Start Menu integration, stable `AppId`-based upgrade identity, a real
code-signing hook for later, no build-tool dependency this environment
lacks) with the lowest operational complexity for a solo-maintained GPL
project, matching this codebase's own repeated pattern of preferring the
smallest tool that genuinely satisfies the requirement (`modernc.org/
sqlite` over CGO, `coder/websocket` over heavier alternatives,
`99designs/keyring` after reading its source). Installed via `winget`
(Microsoft's own official package manager) at user scope on 2026-08-18;
hash verified by winget itself against the official `jrsoftware/issrc`
GitHub release.

## 13. Installation scope/location

**Per-user install, no administrator elevation.** Inno Setup's
`PrivilegesRequired=lowest` with `DefaultDirName={autopf}\Streaming Tree
for OBS` resolves to `%LOCALAPPDATA%\Programs\Streaming Tree for OBS` for a
per-user install (Inno Setup's own documented per-user Program Files
constant) - **program files only** (the executable, the embedded frontend
is inside that one executable per §2, so there is nothing else to stage
beside it). **Persistent user/application data is never installed here** -
it continues to live at the existing, unchanged
`os.UserConfigDir()/StreamingTree` location (`internal/config.go`,
`%AppData%\StreamingTree` on Windows) exactly as today. The install
directory is treated as fully replaceable release output; no code ever
writes mutable state into it.

## 14. Upgrade identity

A fixed Inno Setup `AppId` GUID (generated once, committed in the `.iss`
script, never changed across releases) is the stable identity Inno Setup
uses to recognize "this is an upgrade of the same application" rather than
a separate parallel install - the documented, first-class mechanism for
exactly this (§12). Installing a newer version over an older one replaces
the program files; it never touches `%AppData%\StreamingTree`.

## 15. Application-data preservation and uninstall policy

Audited every persisted resource: SQLite (`streaming-tree.db` + WAL/SHM),
managed visual assets (`assets/visual/`), managed audio assets
(`assets/audio/`), the managed MediaMTX installation and its generated
configuration (`runtime/mediamtx/...`), and OS credential-store entries
(Windows Credential Manager, via `internal/secrets`) - **all of it lives
outside the install directory today** (per-user `%AppData%\StreamingTree`
or the OS credential store) and Stage 20A does not change that.

- **Ordinary upgrade** (install a newer version over an existing one):
  preserves all of the above, because the installer only ever touches the
  program-files directory.
- **Ordinary uninstall**: removes only the installed program files
  (`Uninstallable=yes`, standard Inno Setup uninstaller, registered in
  Apps & Features). It **must not** and **does not** delete
  `%AppData%\StreamingTree` or touch any OS credential-store entry - no
  `[UninstallDelete]` entry targets the data directory, and no code runs
  during uninstall that calls into `internal/secrets`. This is a
  deliberate absence, not an oversight: implementing an explicit,
  separate "remove all my data" tool remains a distinct future feature
  requiring its own design and confirmation, per the governing task.
- Testing this (§20) uses a **hermetic, disposable test data directory**
  (`STREAMING_TREE_DATA_DIR` pointed at a temp path) - never the
  operator's real `%AppData%\StreamingTree`.

## 16. Legal/licence documents in the installed application

Closes the limitation the prior licensing milestone deliberately left
open. Every installed distribution includes `LICENSE`,
`THIRD_PARTY_NOTICES.md`, `LEGAL.md`, and `PRIVACY.md`, embedded into the
same Go binary as the frontend (§2 - one `//go:embed` covering a small
`legal/` directory populated by the release script from the repository
root's own canonical files, never a second hand-maintained copy). Served
through a small, fixed allowlist of same-origin routes:

```
GET /legal/license
GET /legal/privacy
GET /legal/legal
GET /legal/third-party-notices
```

- fixed allowlist only - no `{name}` path parameter resolves an arbitrary
  filename;
- `Content-Type: text/plain; charset=utf-8` - plain/preformatted
  rendering, no Markdown execution, no new rendering dependency;
- the About & Legal page's existing "view full document" links (currently
  pointing at GitHub blob URLs, per the prior milestone) switch to these
  local routes so the installed application works fully offline; the
  GitHub repository link remains separately available as "Source code."

## 17. GPL packaging requirements

Unchanged decision (`GPL-3.0-or-later`, Copyright (C) 2026 Czekosabe) -
not reconsidered. The Windows distribution includes the complete,
unmodified `LICENSE` text (§16). `THIRD_PARTY_NOTICES.md` remains a
separate document; nothing in the packaging process adds a GPL header to
vendored Apache-2.0 material (the YouTube `streamlistpb` files) or claims
FFmpeg/MediaMTX are GPL merely because Streaming Tree is.

**Corresponding-source obligation** (confirmed against the FSF's own GPL
FAQ, `gnu.org/licenses/gpl-faq.html`): distributing a GPL-covered binary
requires the complete corresponding source to remain available for as long
as the binary is distributed. Since this project's complete source is
already public at the canonical repository
(`https://github.com/Czekosabe/streaming-tree-for-obs`), this obligation
is already satisfied for any binary built from a commit that exists in
that public history - **the later public release pipeline (not part of
Stage 20A) must ensure every published installer is built from, and
tagged against, a commit that is actually pushed to that public
repository** before or at the moment of publishing, so the corresponding
source for a distributed binary is never only-locally-available.

Stage 20A does not publish a GitHub Release and does not pretend a local
installer build already is one.

## 18. FFmpeg policy

**Kept operator-provided - the existing model is preserved unchanged**,
after re-confirming the reasoning: `THIRD_PARTY_NOTICES.md` already
documents that FFmpeg has no single official binary distributor this
project can verify and download automatically, and that its licence
depends entirely on how a specific build was compiled (LGPL-2.1-or-later
by default, GPL if `--enable-gpl`, sometimes non-free components) -
Streaming Tree cannot make a licensing claim about a binary it does not
control the provenance of. The installer:

- does **not** include `ffmpeg.exe`;
- does **not** download FFmpeg during or after installation;
- the packaged application starts and is fully usable without it (every
  non-stream-output feature works: MediaMTX ingest, connected accounts,
  engagement, alerts, TTS, goals/widgets, About & Legal);
- outgoing-destination UI continues to report FFmpeg's resolved status
  honestly (existing `branchManager.FFmpegStatus()` behavior, unchanged);
- `STREAMING_TREE_FFMPEG_PATH` and PATH-fallback resolution are unchanged.

End-user documentation (§41) explains this plainly: the app installs and
starts without FFmpeg; only outgoing streaming to destination platforms
needs it; where to get a build and how to point the app at it.

## 19. MediaMTX policy

**Unchanged** - the existing managed-install model (explicit user-
requested download of the pinned, checksum-verified official release into
the per-user application-data directory) is preserved. It is not bundled
into the installer: bundling would freeze the pinned version inside the
installer itself, duplicating the existing, independently-versioned,
independently-checksummed managed-install flow for no benefit, and would
contradict `THIRD_PARTY_NOTICES.md`'s own accurate "no third-party binary
is committed to this repository" statement (kept accurate - see §22).

## 20. Authenticode status

Researched from Microsoft's own current documentation model (signtool.exe
+ a code-signing certificate + an RFC 3161 timestamp server is the standard
Authenticode mechanism). **Stage 20A has no production code-signing
certificate.** No certificate is generated, no private key is committed,
and no artifact produced by this milestone is marked or described as
signed - the release script and every piece of documentation state
plainly that local Stage 20A builds are **unsigned**. The Inno Setup
script structures its `[Setup]` section so a future `SignTool=`
directive can be added later without redesigning the installer - signing
integration is prepared for, not implemented.

## 21. Updater boundary

Implemented in Stage 20B - see [`updater.md`](updater.md) for the full
contract (release source, manifest schema, streaming-active guard, the
real Windows external-helper handoff, and privacy). This document's own
scope remains Stage 20A's packaging/runtime foundation; the updater's own
research, design, and closing state live in `updater.md`, not here.

## 22. Automated packaged-runtime test strategy

`scripts/verify-packaged-app.mjs` (script #23, first new integration
script since Stage 18B) exercises the **real** release-built executable
and embedded production frontend - never `go run`, never Vite, never
Node-served assets. It builds (or reuses an already-built) release binary
with `-tags integration`-equivalent test hooks for browser-launch/native-
dialog suppression (§9/§13 below), starts it against a temporary,
hermetic `STREAMING_TREE_DATA_DIR`/loopback test port, and verifies
routing separation, `--version`, `/api/about`, the legal-document routes,
graceful shutdown via the real endpoint, and clean process exit - detailed
scenario list in the script itself and the closing journal entry.

## 23. Automated installer smoke-test strategy

A second script/PowerShell helper drives the actual Inno Setup output
(`ISCC.exe`-produced `.exe`) through Inno Setup's own documented silent-
install flags (`/VERYSILENT /SUPPRESSMSGBOXES /DIR=<hermetic path>`) into
a throwaway, non-default install location - **never** the operator's real
per-user install path - verifies the installed files, runs `--version`
and a full startup/shutdown cycle against the installed executable, then
uninstalls silently and confirms program files are gone while a
deliberately-created test-data-directory file survives. Full scenario
list in the closing journal entry, gated on actually building a working
installer during implementation.

## 24. Known limitations remaining after Stage 20A

- No Authenticode signing (§20) - artifacts are unsigned by design of this
  milestone, pending a real certificate.
- No final application icon/branding polish beyond the safest neutral
  packaging default - a canonical brand mark was not found in the
  repository during this audit and inventing one is explicitly out of
  scope; recorded as pre-public-release polish.
- No remote-server/non-loopback hardening - unchanged scope boundary from
  every prior stage.
- No full diagnostics/log viewer - Stage 20E.
- No real end-user/OBS manual verification - only automated
  fake/hermetic verification, matching every prior stage's own testing
  discipline.
- macOS/Linux packaging is not attempted - Stage 20A is explicitly
  Windows-first; non-Windows builds are kept compiling but are not
  packaged. See [`platform-support.md`](platform-support.md) for the full
  cross-platform contract, current CI-verification status, and the
  Stage 20B-20E roadmap this leads into.

## 25. Release-candidate distribution via CI artifact (Stage 20E)

Physical/manual verification (`docs/manual-verification.md`) needs the
real installer on hardware other than the developer's own machine. To
avoid requiring every tester to have a local Go/Node/Inno Setup
toolchain, `.github/workflows/windows-package.yml`'s existing second
build+verify pass (already run to prove build reproducibility, §22/§23
above) is built under a `0.1.0-manualtest+<shortsha>` version instead
of an anonymous `-dev+ci2` one, and - only after that pass's own
`verify-packaged-app.mjs` and `verify-installer.mjs` runs have both
passed - the resulting installer, its `.sha256` sidecar, and a small
generated `BUILD-INFO.txt` (product, version, full commit SHA, OS,
architecture, unsigned status - no secrets, no logs, no application
data) are uploaded as a GitHub Actions artifact named
`StreamingTreeForOBS-0.1.0-manualtest-<shortsha>-windows-amd64`, with
14-day retention.

This is **not** a production distribution mechanism:

- **Not a GitHub Release.** No tag is created, nothing is published
  outside the workflow run's own Artifacts panel, and the artifact
  expires with Actions' own retention policy rather than being hosted
  indefinitely.
- **Not a trusted updater source.** The Stage 20B updater (§21 above,
  `docs/updater.md`) is unchanged - it only ever checks canonical
  GitHub Releases on the Stable channel, and still never fetches an
  Actions artifact URL of any kind. A manual/test-versioned build
  downloaded this way still reports itself as ineligible for automatic
  updates (`docs/updater.md` §43), exactly as a locally-built
  `-manualtest`/`-dev` version already did before this change.
- **A distribution convenience only**, for maintainers/testers who
  need the exact CI-built, CI-verified candidate on a second machine
  during `docs/manual-verification.md`'s physical sessions.

`macos-package.yml` and `linux-package.yml` upload the equivalent
verified DMG/`.deb` the same way, for the same reason - see those
workflows' own header comments; this is a workflow-only mechanism, not
a change to either platform's packaging implementation.

## 26. Explicit data-purge uninstall option and cooperative shutdown for manual upgrades (Stage 20E)

### Data ownership
Everything this application ever writes lives under one per-user data
directory (`internal/config.resolveDataDir`, `%AppData%\StreamingTree`
by default): `streaming-tree.db` (+ WAL/SHM), `assets/visual`,
`assets/audio`, the managed MediaMTX runtime under `runtime/`, updater
staging under `updates/`, and (headless-only) `secrets.json`. On
Windows, credentials (destination stream keys, connected-account OAuth
token bundles, donation-source tokens, the administrator password, the
remote-ingest publisher password) live in Windows Credential Manager
instead, under `internal/secrets`'s own namespaced key scheme
(`secrets.BuildKey(secretType, subjectID)`, service name
`streaming-tree-for-obs`).

### Ordinary uninstall (default)
Unchanged since Stage 20A: `scripts/installer/streaming-tree.iss` has
no `[UninstallDelete]` entries and never references the data directory
or the credential store - only the program files `[Files]` installed
are ever removed. This remains the default; no operator action is
required to preserve their configuration on an ordinary uninstall.

### Explicit "remove all my data" option
`InitializeUninstall` (in `[Code]`) replaces Inno's default Yes/No
uninstall confirmation with a custom dialog (built via
`CreateCustomForm`, Inno's own documented recommendation) carrying an
**unchecked-by-default** checkbox: "Also remove all Streaming Tree
settings, local data, and saved credentials," with an explicit "this
cannot be undone" warning. The dialog's default (Enter-responsive)
button always acts on the checkbox's own current state, so a stray
Enter press can never trigger destructive deletion. Under
`/VERYSILENT` (`UninstallSilent()`), the dialog is skipped entirely
and the checkbox defaults to unchecked - identical to an interactive
operator leaving it alone - unless `STREAMING_TREE_TEST_PURGE_USER_DATA=1`
is set on the uninstaller's own environment (`ShouldPurgeUserDataForTest`),
which exists solely so an automated test can exercise the destructive
path without a GUI.

**A real production bug found and fixed during this same cycle, after
four separate diagnostic rounds against real Windows CI evidence:**
the original design ran the purge helper declaratively through an
`[UninstallRun]` entry gated on `Check: ShouldPurgeUserData`. Across
four different real captured Inno `/LOG` runs - a command-line-switch
attempt, an environment-variable attempt (correctly reasoned around
Windows relaunching the uninstaller from a TEMP copy of itself, since
a running process cannot delete its own `.exe`, which really did break
a naive Pascal-global approach), a `RunOnceId` removal, and finally
direct `Log()` instrumentation of the real decision points -
`ShouldPurgeUserData`'s own `Log()` call never fired at all, with no
conclusive documented explanation ever found for why Inno never even
evaluated that `Check:` function in this project's real, repeated-
install-cycle CI environment. Rather than continue diagnosing Inno's
own undocumented `[UninstallRun]` evaluation internals, `[UninstallRun]`
was removed entirely: `InitializeUninstall` now calls the purge helper
directly via Pascal Script's own documented `Exec()` function
(`ExpandConstant('{app}\{#MyAppExeName}')`, `-purge-user-data`,
`ewWaitUntilTerminated`), immediately after
`RequestCooperativeShutdownIfRunning` has confirmed the application is
stopped - a simple, synchronous, imperative call inside the exact
function real captured `Log()` ground truth had already confirmed runs
correctly with the correct `PurgeUserDataChecked` value, removing the
whole cross-mechanism question. `{app}` is still valid at this point:
nothing has removed any files yet when `InitializeUninstall` runs.
`MsgBox` (never auto-suppressed by `/SUPPRESSMSGBOXES`, unlike Inno's
own built-in prompts - a real, separately-discovered gotcha) is guarded
by `UninstallSilent()` everywhere it is used in `[Code]`, so an
automated silent test exercising a failure path can never hang waiting
for a click nobody will make.

`-purge-user-data` (`cmd/server/main.go`, thin wrapper around
`internal/userdatapurge.Purge`) refuses to run at all while
`internal/runtime/singleinstance.Acquire()` reports another instance
still running - purging while the application might still have the
database open is never attempted. It then opens the real database (if
present), enumerates every real platform/account/donation-source ID
through the same repositories production code already uses, deletes
each one's credential-store entry via the exact same `secrets.BuildKey`
namespacing (never a wildcard/prefix scan of Windows Credential
Manager), deletes the two fixed-subject secrets (admin password,
remote-ingest publisher password), then removes the whole data
directory. Never touches OBS's own configuration, a user-supplied
FFmpeg binary, or any credential outside this application's own
namespace.

### Cooperative shutdown for manual installer upgrades
A real physical-test finding: launching a newer installer while
Streaming Tree was still running previously required the operator to
kill the process manually via Task Manager before the install could
proceed. Root-caused as a missing IPC bridge, not a hang in any of the
application's own (individually audited, correctly bounded) shutdown
paths: Restart Manager's `CloseApplications` (Inno's default) can only
gracefully close a window that has registered to handle the request,
and nothing in this application's tray window did.

The fix, shared by both Setup and Uninstall:
`internal/runtime/tray/tray_windows.go`'s existing hidden tray window
now also registers a `RegisterWindowMessageW` name
(`StreamingTreeForOBS.RequestGracefulShutdown`) and, on receiving it,
invokes the exact same `OnQuit` callback the tray's own Quit menu item
already uses - one canonical shutdown path, never a second one.
`[Code]`'s `RequestCooperativeShutdownIfRunning` (researched against
jrsoftware.org/ishelp, though one specific claim from that research
turned out wrong - see below) checks whether the application is
running via the documented `CheckForMutexes` function against the
exact same named mutex `internal/runtime/singleinstance` already
creates, resolves the tray window via `FindWindowW`, posts the
registered message via `PostMessageW`, and polls `CheckForMutexes` up
to a bounded 60 seconds. Called from `PrepareToInstall` and from
`InitializeUninstall`. A failure returns a bounded, specific message
directing the operator to use the tray's own Quit item and retry -
never a hang, never Task Manager.

**Deliberately no `AppMutex` directive in `[Setup]`.** It was tried
first, mirroring the same mutex, on the assumption that
`PrepareToInstall` fires before Inno's own `AppMutex` detection - a
documented claim that a real captured Windows CI `/LOG` proved wrong
or incomplete for this project's actual behavior: `AppMutex` is
checked at Setup's own very early startup, before `PrepareToInstall`
(tied to a later wizard page) ever runs. With `AppMutex` configured,
its own native prompt ("Setup has detected that ... is currently
running ... click OK to continue, or Cancel to exit") fired first and,
under `/SUPPRESSMSGBOXES`, defaulted to Cancel and aborted Setup
before the cooperative-shutdown mechanism ever got a chance to run.
Since `RequestCooperativeShutdownIfRunning` already detects a running
instance itself (via the identical mutex) and actually attempts to
resolve it rather than only prompting, `AppMutex` added no benefit and
actively defeated it by firing first - so it stays out.

### Known local-testing limitation
End-to-end install/uninstall/upgrade behavior for this mechanism could
not be reliably exercised on the primary development machine: a
third-party endpoint security product (McAfee, alongside Windows
Defender) appears to interfere with the newly-built, unsigned
installer once it embeds raw `user32.dll` window-messaging imports
(`FindWindowW`/`PostMessageW`) - a pattern security heuristics
legitimately flag. The real Windows CI runner
(`.github/workflows/windows-package.yml`, GitHub-hosted, no
third-party endpoint security software) is the trustworthy validator
for this mechanism, consistent with how every other native Windows
behavior in this project has always been authoritatively verified.
**Revised (§28):** a later round's own repeated local hangs during
install/uninstall cycling were initially also attributed to this same
McAfee/Defender interference, by analogy rather than direct evidence.
A corrective re-investigation found a real, different, evidenced cause
for those specific hangs instead - see §28's own "AV attribution
correction" note. This section's original McAfee/Defender finding
(about `FindWindowW`/`PostMessageW` import-pattern heuristics
specifically) is not itself contradicted by that correction and is left
as originally recorded.

## 27. Installer UX hardening: fresh/update/repair/downgrade detection, optional desktop shortcut, launch-on-finish

A later hardening pass audited the actual `.iss` and Inno Setup's own
current official documentation (`jrsoftware.org/ishelp`) before adding
anything, per this project's own "contract/audit before implementation"
discipline.

### Directory page and previous-location reuse: already correct by Inno's own defaults
`DisableDirPage` and `UsePreviousAppDir` were never set in `[Setup]` -
confirmed (`jrsoftware.org/ishelp/topic_setup_disabledirpage.htm`,
`topic_setup_usepreviousappdir.htm`) that their unset defaults already
do exactly what was required: `DisableDirPage=auto` shows the directory
page on a fresh install and skips it on an update; `UsePreviousAppDir=
yes` reuses the previously-installed location as the default rather
than resetting to `{localappdata}\Programs\{#MyAppName}`. No `.iss`
change was needed for this part - it was already correct.

### Real fresh/update/repair/downgrade detection
`InitializeSetup` now reads the real previously-installed version (if
any) from this application's own stable `AppId`'s registry entry and
compares it against the installer's own version using a narrow, purpose-
built Pascal comparison (`CompareAppVersions`/`CompareVersionCores`/
`SplitVersion`) that mirrors the exact version shape
`internal/buildinfo.go` actually produces (`MAJOR.MINOR.PATCH`, or
`MAJOR.MINOR.PATCH-<label>+<commit>` for a manual/test build) - never a
lexicographic string comparison, and never confusing two different
manual-test builds of the same nominal version for an update/downgrade
of each other (they compare equal - a repair, not an update).

- **Fresh install** (no previous version found): proceeds normally.
- **Update or repair** (installed version <= installer version):
  proceeds normally - no extra gate.
- **Downgrade** (installed version > installer version): interactively,
  shows an explicit confirmation dialog naming both versions and
  requiring the operator to choose to continue; under a silent run
  (`WizardSilent()`), refuses outright (`Result := False`) rather than
  silently downgrading - the built-in updater itself never reaches this
  branch in real use, since it only ever installs a version newer than
  the one it is replacing.
- The Ready-to-Install page (`UpdateReadyMemo`) shows the real
  "Installed version / Installer version / Operation" context (fresh
  install / update / repair / downgrade) requested for the interactive
  path - the Ready page itself is skipped entirely under `/SILENT`/
  `/VERYSILENT`, so this never affects a silent run.

**Superseded by §28.** This round's own first attempt at a fix
concluded the registry root varies with `IsAdminInstallMode()` on an
"admin-capable account" and picked the root accordingly. A corrective
re-investigation (§28) found that explanation did not hold up against
official documentation and, with real `/LOG` evidence, identified a
different, concrete, fixable root cause instead
(`PrivilegesRequiredOverridesAllowed=dialog`, now removed). See §28 for
the corrected design; `UninstallRegRoot()`/`IsAdminInstallMode()` no
longer exist in `[Code]`.

### Start Menu, desktop shortcut, and Launch-on-finish
**Superseded by §28.** This round originally kept the Start Menu
shortcut as a mandatory, unconditional `[Icons]` entry and used a
custom `RegisterPreviousData`/`GetPreviousData` reimplementation of
task-choice persistence. §28 changed both: Start Menu is now also a
standard task (selected by default), and the custom persistence code
was removed as redundant with Inno's own native `UsePreviousTasks`.

A new `[Run]` entry offers "Launch {#MyAppName}" on the final wizard
page using Inno's own `postinstall skipifsilent nowait` flags -
`skipifsilent` is Inno's own documented, native way to skip a `[Run]`
entry entirely under a silent run, which by construction means the
built-in updater's own always-silent invocation
(`internal/updater/helper_windows.go`) never launches a duplicate
process here; no custom `[Code]` was needed to keep the silent updater
path silent.

### The built-in updater already never overrides the install path
Re-audited `internal/updater/helper_windows.go`'s real invocation of the
installer (`proceedWithInstall`, around its own "Step 7" comment): it
already runs `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=...` with no
`/DIR=` of any kind, and its own doc comment already states why -
`UsePreviousAppDir`'s default (confirmed above) already preserves the
existing install location on a same-AppId upgrade without one. No
change was needed here.

### Automated verification
`scripts/verify-installer.mjs` gained a new scenario
(`testVersionDetectionScenario`) that compiles two additional throwaway
test installers (versions `0.1.0` and `0.2.0`) via `ISCC.exe` directly,
reusing the same staged executable `scripts/build-release.ps1` already
built for the main scenarios - no second `go build`/`npm run build`
pass. It drives a real fresh install, a real update, a real silent
downgrade attempt (asserted to fail and leave the registered version
unchanged), and a real same-version repair/reinstall (asserted to
succeed), verifying the real Inno-registered `DisplayVersion` after each
step. **Revised in §28** to assert the canonical root
(`HKEY_CURRENT_USER`) explicitly and assert `HKEY_LOCAL_MACHINE` stays
empty throughout, rather than accepting either - proving the fix, not
merely tolerating the pre-fix behavior.

Shortcut-task behavior is now automated too (§28's own
`testShortcutTasksScenario`), on the GitHub-hosted CI runner's own
disposable per-user Desktop/Start Menu - never the operator's real
machine, which local development/manual testing must still avoid (see
§28).

## 28. Corrective audit: the real cause of the HKLM finding, and the resulting redesign

§27's own `IsAdminInstallMode()`-based registry-root fix was flagged
(by the operator, re-reading §27 against Inno's own official
documentation) as inconsistent with `PrivilegesRequired=lowest`'s own
documented guarantee ("will always run in non administrative install
mode" - `jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm`,
quoted correctly in §27 itself, yet not reconciled with the observed
`HKEY_LOCAL_MACHINE` finding at the time). This section documents the
real, evidence-based re-investigation and its outcome.

### The real root cause
A real local install, captured with Inno's own `/LOG=`, showed:

```
Setup command line: ... /VERYSILENT /SUPPRESSMSGBOXES /NOICONS /DIR=... /LOG=... /ALLUSERS
User privileges: Administrative
Administrative install mode: Yes
Install mode root key: HKEY_LOCAL_MACHINE
```

`/ALLUSERS` was never passed by the test - Inno appended it to its own
internal re-launch. The cause: `PrivilegesRequiredOverridesAllowed=
dialog`, present in `[Setup]` since this file's very first commit
(`1587612`) with no comment ever explaining why, makes Setup default to
administrative install mode whenever the current account is an
Administrators-group member and no dialog can be shown to ask
otherwise (every silent/automated invocation, and even an interactive
one unless the operator notices and deliberately picks "install for
just me"). This directly contradicts the project's own absolute,
repeatedly documented "per-user, no elevation, ever" contract - not
merely a registry-detection inconvenience, a real installer-behavior
bug. `git log` found no justification for the directive ever having
been added deliberately for a specific product reason.

**Fix:** `PrivilegesRequiredOverridesAllowed` removed from `[Setup]`
entirely. Re-tested with the identical `/LOG=` capture, on the
identical admin-capable account that previously showed
`HKEY_LOCAL_MACHINE`:

```
Setup command line: ... /VERYSILENT /SUPPRESSMSGBOXES /NOICONS /DIR=... /LOG=...
User privileges: None
Administrative install mode: No
Install mode root key: HKEY_CURRENT_USER
```

No `/ALLUSERS`. Confirmed twice (a distinguishable never-before-used
test version each time, so no stale registry state could account for
the result).

### Redesigned existing-install detection
`UninstallRegRoot()`/`IsAdminInstallMode()`-based root selection is
removed. `InitializeSetup` now reads **both** `HKEY_CURRENT_USER`
(the one canonical root a correctly-behaving build of this installer
ever writes to) and `HKEY_LOCAL_MACHINE` (checked defensively only -
this project has never published a public release, so any real HKLM
entry found can only be residue from testing this installer itself
before this fix, never a real external user's legitimate legacy
install):

- **Only HKCU has an entry:** the real, canonical fresh/update/
  repair/downgrade path (unchanged from §27's own comparison logic).
- **Only HKLM has an entry:** refuses to proceed with an explicit
  message directing the operator to uninstall the administrative copy
  first (from an elevated "Apps & Features") - never silently creates
  a second, parallel per-user registration alongside it.
- **Both have an entry:** refuses to proceed with an explicit message
  naming both versions - never silently picks one.
- Every refusal is logged (`Log(...)`) for silent-run diagnosis and
  shown via `MsgBox` interactively; a silent run refusing is a clean
  non-zero exit, not a hang.

Verified locally against real registry state for both conflict cases
(HKLM-only, and both-populated simultaneously) - both produced the
correct refusal and the correct logged message.

### Start Menu is now a real, independent task choice
Reviewed per the operator's own explicit challenge: keeping the Start
Menu shortcut mandatory was a preference stated in §27, not a concrete
product/Windows requirement. Changed: `startmenuicon` is now a
`[Tasks]` entry, selected by default; `desktopicon` remains a
`[Tasks]` entry, unselected by default. Neither choice affects the
application's own Apps & Features/uninstall registration.

### Native task persistence replaces the custom §27 mechanism
Re-audited whether §27's own `RegisterPreviousData`/`GetPreviousData`/
`InitializeWizard` reimplementation was actually necessary. Official
documentation (`jrsoftware.org/ishelp/topic_setup_useprevioustasks.htm`):
`UsePreviousTasks` defaults to `"yes"` and already "use[s] the task
settings of the previous installation as the default settings
presented to the user in the wizard" - exactly what the custom code
reimplemented by hand. Removed as redundant; `[Setup]` never overrides
`UsePreviousTasks`, so the native default applies with no `[Code]`
needed.

### AV attribution correction
A later local-testing round in §27 attributed several install/uninstall
hangs to the McAfee/Defender interference already recorded earlier in
this document, by analogy rather than direct evidence from that round
itself. This corrective investigation found a better-evidenced
explanation for those specific hangs: a pre-fix build (with
`PrivilegesRequiredOverridesAllowed=dialog` still present) installed in
administrative mode, and a later uninstall of that same install likely
attempted to silently re-elevate via UAC - an OS-level secure-desktop
consent prompt that no Inno flag (`/SUPPRESSMSGBOXES` included) can
suppress, and that a non-interactive script can never satisfy, which
would hang indefinitely rather than fail. Direct supporting evidence:
after the fix, an uninstall of a genuinely per-user (non-admin-mode)
install completed instantly, with no hang, on the same machine where
prior uninstalls of pre-fix installs had hung repeatedly. This is a
plausible, evidenced explanation, not a certainty - stated as such
rather than re-asserting the earlier McAfee/Defender analogy as fact.
The original §26 McAfee/Defender finding (a different, earlier round,
about `FindWindowW`/`PostMessageW` import-pattern security heuristics
specifically) is not itself contradicted by this and stands as
originally recorded.

### Automated verification
`scripts/verify-installer.mjs`'s `testVersionDetectionScenario` now
asserts the canonical root explicitly: `HKEY_CURRENT_USER` has the
expected `DisplayVersion` and `HKEY_LOCAL_MACHINE` has none, at every
step (fresh install, update, blocked downgrade, repair) - proving the
fix holds, not merely tolerating either root as §27's original version
did.

A new `testShortcutTasksScenario` exercises real Start Menu/desktop
`.lnk` creation and removal - deliberately only ever on the GitHub-
hosted CI runner's own disposable per-user Desktop/Start Menu, per the
operator's own explicit instruction never to do this on a real/local
machine. Covers: default fresh install (Start Menu present, desktop
absent); update with no explicit task flags (previous choices remain
stable, proving `UsePreviousTasks` itself, not just that the code
compiles); explicit desktop-task selection (`/MERGETASKS=desktopicon`,
shortcut created, target resolved via `WScript.Shell` and confirmed to
point at the real installed executable); explicit Start Menu
deselection (`/MERGETASKS=!startmenuicon`, no Start Menu shortcut
created, install itself still correct); and uninstall removing every
installer-owned shortcut. Every shortcut path is built from the exact
literal app/group name, never a wildcard, and is removed again in a
`finally` block on every path including failure.

### What remains manual
Full interactive wizard walkthroughs (seeing the actual Ready-to-Install
memo text, clicking through the shortcut tasks page) remain part of
`docs/manual-verification.md`'s physical Windows sessions - this
section closes the automatable, evidence-based gaps the operator's
corrective audit identified, not the physical gate itself.

## 29. Multilingual installer (English/Polish)

The application's own web UI has offered English/Polish for some time
(its own separate i18next resources under `apps/web/src/i18n/resources`).
The Windows installer had never received the same treatment - this
section documents the installer-language work, using only Inno Setup's
own native mechanisms (no custom Pascal language page, no Inno version
upgrade, no third-party or vendored `.isl` file).

### Supported languages
Exactly English and Polski - `[Languages]`:

```
[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "polish"; MessagesFile: "compiler:Languages\Polish.isl"
```

Both are Inno Setup's own official, compiler-shipped translation files -
`Default.isl` (English, built in) and `Languages\Polish.isl` (Inno's
own official Polish translation, resolved from the installed compiler's
own `Languages\` subdirectory at compile time - confirmed via a real
`ISCC.exe` compile log: `Reading file: ...\Inno Setup 6\Languages\
Polish.isl`). Neither is vendored into this repository. English remains
the canonical/source language; no other language is offered (no German/
Spanish/French/etc. - out of scope).

### Default-language detection and interactive override
`ShowLanguageDialog`, `LanguageDetectionMethod`, and `UsePreviousLanguage`
are all left at Inno's own documented defaults (`yes` / `uilanguage` /
`yes` respectively) - no override was needed for this project's exact
setup (a plain fixed-literal `AppId`, two languages, English listed
first as the eventual fallback). Practical effect: a fresh interactive
install shows Inno's native Select Language dialog, defaulting to
whichever of the two offered languages matches the current Windows UI
language (Polish on a Polish-language Windows session, English
otherwise, including for any other unsupported UI language - never
forced to Polish merely because the installer itself was built by a
Polish-speaking developer). The user can always override the default
and pick either language from the dialog.

### Update/repair language preservation
`UsePreviousLanguage` (native default `yes`) looks up the language an
existing same-`AppId` install already used and pre-selects it as the
new wizard's default, silently skipping the dialog when a command-line
override isn't given. Confirmed via a real local install+update cycle
(a throwaway `AppId`, never the real project's registered one) reading
the real registry value Inno itself records:

```
Inno Setup: Language    REG_SZ    polish
```

A Polish-language install, updated later with no `/LANG` flag at all,
stayed Polish; an English-language install, updated the same way,
stayed English. Both directions verified for real, not assumed from
documentation alone.

### Silent built-in updater behavior
The application's own built-in updater
(`apps/server/internal/updater/helper_windows.go`) always launches the
installer with `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=...`.
This exact flag combination was run for real, timed, against an
existing Polish-language install with no `/LANG` override - it
completed in well under a second, registered `Inno Setup: Language` as
still `polish` (native `UsePreviousLanguage` preservation, not a
dialog), and at no point waited for input. A silent/very-silent
installer invocation never shows the Select Language dialog regardless
of `/LANG` presence, by Inno's own documented `ShowLanguageDialog`
behavior in an unattended run.

### Uninstaller localization
`Uninstall.exe` has no `/LANG=` command-line parameter of its own
(confirmed against `jrsoftware.org/ishelp/topic_uninstcmdline.htm`'s
full parameter list) - it purely inherits the language the matching
install used, read back from the same registry value above. No `[Code]`
was needed for this; it is native Inno behavior. Every custom string
the project's own `InitializeUninstall` dialog shows (caption,
confirmation message, the destructive-purge checkbox, the "cannot be
undone" warning, both buttons, and every failure `MsgBox`) is now
sourced from `[CustomMessages]` via `CustomMessage(...)`/
`FmtMessage(...)`, so it renders fully in whichever language the
install used - confirmed via real side-by-side screenshots in both
languages during development, which also caught and fixed a pre-
existing (not localization-introduced) single-line clipping bug in the
dialog's `Message` control, present in both languages beforehand
(`Message.Height` was never explicitly set).

### Every project-owned string is localized
All `[Tasks]`/`[Icons]`/`[Run]` `Description:`/`GroupDescription:`
values, every `[Code]`-driven `MsgBox` (dual/administrative-install
conflicts, downgrade confirmation, cooperative-shutdown failure,
uninstall shutdown/purge failures), and the entire `UpdateReadyMemo`
text (fresh/update/repair/downgrade operation lines, installed/installer
version labels) route through `[CustomMessages]` - 25 `english.*` keys,
each with a `polish.*` counterpart, no orphan on either side (enforced
by an automated structural check - see below). `{#MyAppName}`/
`{#MyAppVersion}` preprocessor substitutions are baked into both
language values at compile time and never translated; the product name
itself is never translated, per the glossary the operator supplied
(Install→Zainstaluj, Update→Zaktualizuj, Repair/Reinstall→Napraw /
Zainstaluj ponownie, etc.), adapted to fit each actual wizard context
rather than applied as literal machine translation.

### Installer language vs. application UI language are separate
Choosing Polski in Setup does **not** change the web app's own EN/PL UI
language, and vice versa. The installer never writes browser
`localStorage`, never invents registry state to steer the React UI, and
never depends on a specific browser profile - the two settings remain
two independent, unlinked preferences, exactly as before this task. No
existing code linked them prior to this work, and this task introduced
no such linkage.

### Automated verification
`scripts/verify-installer.mjs`'s `testLocalizationScenario`:

- **Structural** (parses the real `.iss` source text, not a compiled
  artifact): `[Languages]` contains exactly `english` → `compiler:
  Default.isl` and `polish` → `compiler:Languages\Polish.isl`, no third
  language; every `english.*` `[CustomMessages]` key has a matching
  `polish.*` key and vice versa (no orphan in either direction).
- **Real language selection**: a real `/LANG=english` and a real
  `/LANG=polish` silent install each register the matching `Inno Setup:
  Language` value.
- **Real update-preservation**: a real update with no `/LANG` flag over
  an existing Polish install stays Polish, and the same over an English
  install stays English (native `UsePreviousLanguage`).
- **Real silent-never-blocks proof**: the exact real updater-compatible
  flags (`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=...`) complete
  in a measured, bounded time over an existing Polish install with no
  `/LANG` override - proof, not assumption, that a second installer
  language never makes the built-in silent updater stop at a language
  dialog.
- Every pre-existing silent-install invocation in this script was
  pinned to `/LANG=english`, so the rest of the regression suite (Start
  Menu/desktop shortcuts, version-detection, ordinary uninstall/
  reinstall, explicit purge, upgrade-while-running) stays deterministic
  and independent of whatever UI language the CI runner itself happens
  to report - no existing assertion (hardcoded-English artifact names
  like `Uninstall Streaming Tree for OBS.lnk` included) was weakened to
  accommodate the second language.

### What remains manual
Real interactive Select Language dialog appearance in both languages,
full Polish wizard-page/diacritics rendering with no clipped controls,
and a real previously-installed-language default surviving a real
interactive update, are recorded as pending physical verification in
`docs/manual-verification.md` - not claimed here without human
evidence.

## 30. System tray icon (Windows)

Merged from the former `docs/windows-tray.md` (final product-polish/
documentation-cleanup pass) - a real, always-available piece of desktop
UI closing the gap a console-free packaged app otherwise leaves: closing
the browser tab does not stop the backend (§1), and with no visible
window and no console, an operator otherwise has no way to see the
application is still running, reopen it, or quit it short of Task
Manager.


Closing the browser tab does not stop the backend (docs/windows-packaging.md
§1: one Go process, the browser is a separate OS component). A desktop
operator with no visible window and no console (`-H=windowsgui`,
docs/windows-packaging.md §7/§13) then has no way to see the application is
still running, reopen it, or quit it, short of Task Manager. The tray icon
is the one persistent, always-available piece of desktop UI that closes this
gap.

### 30.1 Audit of the existing lifecycle (before design)

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

### 30.2 Implementation choice

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

### 30.3 Menu contract

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

### 30.4 Lifecycle and invariants

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

### 30.5 Icon/branding

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

### 30.5a Original placeholder-icon design notes (superseded, kept for history)

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

### 30.5b Web favicon

`apps/web/public/favicon.ico` (the same multi-size set `tray.ico` carries)
is linked from `apps/web/index.html` via `<link rel="icon" href="/favicon.ico"
sizes="any" />`, and copied into the production build output automatically
by Vite's own `public/` convention (no build-script change needed) - the
one piece of "if appropriate and straightforward" web-branding consistency
the governing task asked for.

### 30.5c The tray tooltip bug: root cause and fix

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

### 30.6 Localization

Tray menu text is plain English only, never run through the frontend's
i18n system - this mirrors `internal/runtime/nativealert`'s own existing
precedent (its `MessageBoxW` title/message are plain Go strings too).
Native OS-level UI in this application has never been localized; only the
web frontend is. This was not part of the governing task's explicit
localization requirement for this remediation cycle (which named the
updater eligibility work specifically, docs/updater.md §43) - restated here
as a real, honest limitation rather than an oversight.

### 30.7 What was dropped, and why

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

### 30.8 Test strategy

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

### 30.9 Known limitations

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
