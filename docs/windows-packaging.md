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

Not implemented, not started. No GitHub Releases check, no update banner,
no download, no install, no restart-after-update, no telemetry. This is
Stage 20B (or later) work.

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
- No updater (§21) - Stage 20B.
- No remote-server/non-loopback hardening - unchanged scope boundary from
  every prior stage.
- No full diagnostics/log viewer - later Stage 20 work.
- No real end-user/OBS manual verification - only automated
  fake/hermetic verification, matching every prior stage's own testing
  discipline.
- macOS/Linux packaging is not attempted - Stage 20A is explicitly
  Windows-first; non-Windows builds are kept compiling but are not
  packaged.
