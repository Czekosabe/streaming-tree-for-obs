# Stage 20C1 — macOS packaged runtime + native package verification contract

**Research date:** 2026-08-18. Written before any Stage 20C1 product code,
per this project's established discipline (see
[`windows-packaging.md`](windows-packaging.md) for Stage 20A and
[`updater.md`](updater.md) for Stage 20B, both written the same way).

## 1. The 20C1 / 20C2 split

`docs/platform-support.md` originally scoped Stage 20C as one bucket:
"macOS desktop portability + packaging + signing + notarization +
automated verification." This document splits it honestly, because the
operator does not own a Mac and has no production Apple Developer
signing/notarization setup available for this project today:

- **Stage 20C1** (this document, this milestone): a real macOS packaged
  runtime, a real `.app` bundle, a real DMG, real native-CI package
  verification on GitHub-hosted Apple Silicon and Intel runners, and the
  macOS-specific lifecycle adapters (browser launch, single instance,
  fatal-startup UX) a packaged app actually needs. **Unsigned. Not
  notarized. No public release.**
- **Stage 20C2** (a distinct future milestone, externally gated):
  Developer ID Application signing, hardened-runtime finalization,
  notarization submission/stapling, a real macOS updater install
  handoff, and public/Beta release readiness. Every one of these
  requires a real Apple Developer Program identity this project does
  not have yet - see §32.

After this milestone: **Stage 20C1 is Completed. Stage 20C2 is Planned,
externally gated on real Apple signing credentials. Stage 20C as a whole
remains Incomplete.** macOS is never described as "fully supported"
after 20C1 - see §9's own vocabulary discipline, inherited from
`docs/platform-support.md` §1.

## 2. Primary-source research performed (2026-08-18)

- **Apple bundle structure**: current Apple Developer documentation on
  bundle layout confirms `Contents/MacOS/` holds the executable
  (`CFBundleExecutable`), `Contents/Resources/` holds static resources,
  and `Info.plist` is the bundle's own metadata dictionary
  (`CFBundleIdentifier`, `CFBundleName`, `CFBundleDisplayName`,
  `CFBundleShortVersionString`, `CFBundleVersion`, and the
  `LSUIElement`/`LSBackgroundOnly` background/agent-app keys).
- **GitHub-hosted macOS runners**: re-verified directly against GitHub's
  own current runner reference (not assumed from the prior milestone's
  own recorded labels) - confirmed current labels: Apple Silicon
  (`macos-latest`, `macos-14`, `macos-15`, `macos-26`), Intel
  (`macos-15-intel`, `macos-26-intel`) - both free for this public
  repository. `macos-15-intel` specifically was cross-checked against
  this repository's own real, already-green CI run history (Stage 20B's
  `cross-platform.yml`), confirming it is a genuine, working label, not
  a stale or deprecated one.
- **Go + macOS deployment target**: this repository's `go.mod` currently
  floors at Go 1.25.0. Go 1.25's own release notes require macOS 12
  Monterey or later - this is the authoritative, non-guessed minimum
  macOS deployment target for any binary this toolchain produces.
- **Developer ID / Gatekeeper / notarization**: current Apple Developer
  documentation confirms distribution outside the Mac App Store requires
  a Developer ID Application certificate with the hardened runtime
  enabled, `codesign` for signing, and submission to Apple's
  notarization service via `notarytool` (the modern replacement for the
  retired `altool`), with the resulting ticket typically stapled to the
  distributed artifact; Gatekeeper checks this at first launch. None of
  this is performed in Stage 20C1 - recorded here only to inform the
  20C2 boundary in §32-§33.
- **DMG packaging**: `hdiutil` is Apple's own standard command-line tool
  for disk-image creation (`hdiutil create`), mounting (`hdiutil
  attach`), and unmounting (`hdiutil detach`) - already present on every
  GitHub-hosted macOS runner, no third-party packaging framework needed.

## 3. Architecture policy (unchanged from Stage 20A/20B)

One application core, preserved. This milestone does not create
"Streaming Tree for Mac" as a separate product. The shared Go core
(`apps/server`) and shared React frontend (`apps/web`) are unchanged;
macOS-specific code lives behind `darwin`-tagged files in the same three
narrow runtime-adapter packages Windows already established
(`internal/runtime/browserlaunch`, `internal/runtime/singleinstance`,
`internal/runtime/nativealert`), plus a new macOS release-build script
and CI workflow - never `runtime.GOOS == "darwin"` scattered through
domain code.

## 4. Target architectures

Both **darwin/arm64** (primary, modern) and **darwin/amd64** (Intel,
packaged as long as native GitHub-hosted Intel CI remains practical -
confirmed practical per §2) are targeted. No universal/fat binary is
built in 20C1 - two separate, architecture-specific packages are
simpler and already fit the artifact-identity model
`internal/updater/manifest` shipped in Stage 20B
(`Identity{OS, Arch, Kind}`), which was deliberately built multi-
platform-ready from the start (`docs/platform-support.md` §15).
Artifact identities introduced by this milestone:

- `darwin / arm64 / dmg`
- `darwin / amd64 / dmg`

## 5. Package format decision

**A normal `.app` bundle distributed inside a `.dmg`.** Evaluated
against a bare `.app` + `.zip` and against a `.pkg` installer:

- No native window framework, system service, or privileged installer
  is needed - the application is a background HTTP server plus the
  user's own browser, exactly like the Windows packaged app.
- User data already lives entirely outside the bundle (§22) - there is
  nothing for a `.pkg`'s privileged install step to legitimately do that
  a plain drag-to-Applications `.app` does not already do.
- A DMG is the conventional macOS experience for exactly this shape of
  application, and Stage 20B's updater artifact model already treats a
  package as one opaque, name-and-hash-verified `Kind` - a DMG fits that
  model directly, with no need to invent a second kind for this
  milestone.

The DMG itself is a simple layout - `Streaming Tree for OBS.app` plus an
`Applications` symlink, exactly the standard "drag to Applications"
pattern - no custom background image or branded DMG chrome. That
polish, like Windows's own unresolved icon/branding gap
(`windows-packaging.md`), is explicitly out of scope here.

## 6. Stable bundle identity

**Bundle identifier: `io.github.czekosabe.streaming-tree-for-obs`.**
Apple's own bundle-identifier convention is a reverse-DNS string using
only alphanumerics, hyphens, and periods; this project has no owned
domain, so the reverse-DNS form is built from the canonical public
GitHub identity (`github.com/Czekosabe/streaming-tree-for-obs`) instead
of a domain that does not exist - the same reasoning Windows's own fixed
Inno Setup `AppId` GUID used (a stable identity that does not depend on
owning infrastructure this project does not have). This identifier is
fixed for the lifetime of the project, generated once, never derived
from a machine username, a Git email, a temporary version string, or an
architecture - both the Apple Silicon and Intel packages share the
exact same `CFBundleIdentifier`; only the artifact's own OS/Arch/Kind
identity (§4) distinguishes them, never the bundle identity itself.

## 7. App bundle layout

```
Streaming Tree for OBS.app/
  Contents/
    Info.plist
    MacOS/
      streaming-tree-server        (the real Go executable, CGO-enabled)
    Resources/
      LICENSE
      PRIVACY.md
      LEGAL.md
      THIRD_PARTY_NOTICES.md
```

The production React frontend remains embedded directly in the Go
executable via the exact same `//go:embed` mechanism Stage 20A already
established (`internal/webassets`) - no sidecar Node/Vite runtime files,
no separate frontend process, identical to the Windows packaged
architecture. The four legal documents are staged both ways, exactly
mirroring Windows's own redundancy: loose inside `Contents/Resources`
for distribution clarity, and embedded (`webassets.Legal()`) so the
existing `/legal/*` HTTP routes continue serving them identically on
every platform.

## 8. Application icon

No canonical project icon exists (confirmed by the same audit
`docs/product-identity-legal.md` already performed for Windows). No
`.icns` is invented for this milestone, no third-party logo is used, and
no OBS/Twitch/YouTube mark is used. The unsigned automated package ships
with the system default application icon - recorded as pre-public-
release polish, exactly like Windows's own unresolved icon gap, and
does not block 20C1.

## 9. Packaged-mode identification

Unchanged from Stage 20A: `buildinfo.Packaged()` reports true only when
`scripts/build-release-macos.sh` (or its Windows counterpart) injects
`packagedFlag=true` via `-ldflags -X`, exactly the same unexported
`internal/buildinfo` variable both platforms share. `runtime.GOOS ==
"darwin"` is never used to infer packaged mode - a macOS developer
still gets the normal `go run`/Vite two-process workflow unchanged.

## 10. macOS browser launch

`internal/runtime/browserlaunch/browserlaunch_other.go` already handled
macOS correctly (`exec.Command("open", url)` - a fixed executable name,
never a shell string, matching Apple's own documented "open with the
default handler" mechanism) but was mixed into one generic `!windows`
file shared with Linux. Split into `browserlaunch_darwin.go`
(`//go:build darwin`) and `browserlaunch_linux.go`
(`//go:build linux`, plus a narrower `!windows && !darwin && !linux`
catch-all) purely for clarity and to let each platform's own adapter
evolve independently without touching the others - the actual `open`
mechanism itself is unchanged, since it was already correct. Same
requirements as Windows: launches only after the server is actually
ready to accept connections, failure is nonfatal (the application keeps
running), and a second packaged launch triggers the identical safe
open-URL behavior once the running first instance is detected (§11).

## 11. macOS single instance

The prior "always succeeds" `!windows` stub is explicitly insufficient
for a packaged macOS app (docs/platform-support.md §9/§25 already
flagged this). Implemented for real in
`internal/runtime/singleinstance/singleinstance_darwin.go`: an
exclusive, advisory `flock(2)` on a fixed, application-owned lock file
inside the same per-user data directory `internal/config` already
resolves (`~/Library/Application Support/StreamingTree/.instance.lock`)
- `flock` is the standard POSIX/BSD primitive for exactly this purpose,
and its defining property (the one Windows's `CreateMutexW` mechanism
also has) is that the OS kernel releases the lock automatically the
moment the owning process exits or crashes, for any reason - there is no
stale-lock state that can permanently block a future launch, and no
plaintext PID file is trusted as the sole proof of another instance
(a PID file alone cannot prove the process holding that PID is actually
Streaming Tree, only that *some* process has that number - `flock`
proves live ownership directly, at the kernel level, with no such
ambiguity). A second launch that fails to acquire the lock does not
start a second backend, does not bind the HTTP port again, and instead
opens the existing management URL exactly like Windows's own second-
launch behavior.

## 12. macOS fatal-startup UX

Finder-launched applications have no visible terminal, so the existing
stderr-only `!windows` fallback (correct for a source/dev build, which
always has a console) is insufficient for a packaged macOS release.
Implemented for real in
`internal/runtime/nativealert/nativealert_darwin.go`: a narrow, minimal
Cgo bridge to `NSAlert` (AppKit), the standard Apple-native modal-alert
mechanism, showing a bounded, plain-text title/message with no HTML, no
AppleScript, and no shell command ever assembled from the error text -
the message string is passed directly as Objective-C string data, never
interpolated into an executable command of any kind. Source/developer
builds are unaffected (they never build with the packaged, console-free
release flag in the first place, exactly like Windows). Non-darwin,
non-Windows builds keep their existing stderr fallback unchanged.

## 13. Dock / background-app behavior

**Decision: `LSUIElement = true`** (agent/accessory application - no
Dock icon, no menu bar by default). Streaming Tree's actual UI is
always the user's own browser tab; the packaged process itself owns no
native application window, exactly matching the Windows packaged
product, where the process similarly exposes no native window of its
own. A normal Dock-visible app with no window to show or activate would
be confusing (clicking its Dock icon would do nothing meaningful); an
agent app avoids that while the application remains fully controllable
through its own web UI, including the existing explicit "Quit Streaming
Tree" action (§14) - the user always retains a clear way to quit.

## 14. Quit / graceful shutdown

Unchanged: the existing protected `POST /api/system/shutdown` endpoint
and its `context.CancelFunc` shutdown path are reused exactly as they
are - no second, Mac-specific shutdown architecture is created. The
packaged macOS application stops the same shared runtime in the same
order Windows already established: destination branches, the device-
flow manager, engagement connectors, the operator-chat projection, the
chat-overlay/outbound-chat/chat-automation/alerts/audio/goals/
supporter-widgets managers, the Event Bus, and finally MediaMTX -
`cmd/server/main.go`'s own `shutdownRuntime` closure is not duplicated,
only reached from one more platform. Closing the browser tab does not
quit the application, on any platform.

## 15. Application data

Unchanged and already correct: `internal/config.resolveDataDir` already
uses `os.UserConfigDir()`, which resolves to `~/Library/Application
Support` on macOS, joined with the same `AppDirName = "StreamingTree"`
constant every platform shares - confirmed by direct source audit, no
migration or rename performed merely because packaging now exists. The
`.app` bundle is a replaceable distribution artifact; the DMG is a
read-only distribution container, never a data store. Ordinary
replacement or removal of the `.app` (dragging a new version over the
old one, or dragging it to the Trash) never touches SQLite, managed
visual/audio assets, the managed MediaMTX installation, or Keychain
entries - all of it lives outside the bundle, exactly like Windows's own
install-directory/AppData separation.

## 16. Keychain

Unchanged: `internal/secrets/keyring_store.go` already lists
`keyring.KeychainBackend` unconditionally among its allowed backends
(confirmed by direct source audit, unchanged since Stage 20A/20B). The
real Keychain backend (`github.com/99designs/go-keychain`, gated
`//go:build darwin && cgo` inside `github.com/99designs/keyring` itself)
requires CGO - the macOS release build script (§30) and the macOS
package-verification CI workflow (§34) both build with `CGO_ENABLED=1`
(never disabled to dodge this real dependency), and the CI workflow
proves the *production* binary actually compiles against the real
backend, exactly the same discipline the cross-platform portability
baseline milestone already established for `cross-platform.yml`'s own
macOS jobs. No real OAuth token or stream key is ever written to a CI
runner's Keychain; ordinary tests continue using the existing fake
secret store.

## 17. MediaMTX on macOS

Unchanged: `internal/runtime/mediamtx/platform.go`'s asset matrix
already lists real, checksum-verified release assets for
`darwin/amd64` and `darwin/arm64` (confirmed present since before this
milestone). Stage 20C1's own native macOS CI package-verification
workflow (§34/§38) exercises this path for real as far as safely
possible - platform asset selection, archive/executable-permission
handling, configured-path resolution, and process start/stop/cleanup
semantics - using the existing managed-install mechanism, never a
network-disabled fake substituted in its place. The loopback-only RTMP/
Control-API policy is completely unchanged; nothing in this milestone
touches that boundary.

## 18. FFmpeg on macOS

Unchanged: FFmpeg remains entirely operator-provided, on every
platform. This milestone does not bundle FFmpeg, does not install
Homebrew, and does not download a third-party FFmpeg build from any
distributor this project has not audited. `internal/runtime/ffmpeg/
resolver.go` was already fully portable before this milestone (its only
`runtime.GOOS` uses are the `.exe` suffix choice and the Unix
executable-bit check, both already correct for macOS). The packaged
macOS app starts and is fully usable without FFmpeg; outgoing branch
streaming remains unavailable until the operator supplies a compatible
executable.

## 19. System TTS

Unchanged and honest: macOS system TTS is **not implemented** in this
milestone. `internal/provider/tts/stub.go` already reports
`Capabilities.Available == false` on every non-Windows build, including
macOS, with no fake `say`/`AVSpeechSynthesizer` shortcut. A real native
macOS TTS provider remains a separate, future feature milestone, not
part of packaging.

## 20. Updater behavior on macOS in 20C1

Stage 20B's updater installs updates on Windows x64 only. This milestone
does **not** make the macOS packaged build behave as though its updater
is complete. Audited: `updater.Manager.Start` currently begins the
automatic hourly check loop whenever `ReleaseBuild && AutoCheck`, with
no platform-capability gate - on a macOS release build this would begin
real, hourly, rate-limit-consuming GitHub polling for an update the
application could never install, and would eventually show "Update
available" with no way to act on it besides a confusing, permanently-
blocked "Install and restart" button.

**Fixed for real** (not merely documented): a new `State` value,
`StatePlatformUnsupported`, distinct from `StateDisabled` (which
specifically means "not a release build," a different condition).
`Manager.Start` now checks `Handoff.Available()`'s blocker code before
ever beginning automatic scheduling; when it is exactly
`BlockerPlatformUnsupported` (the non-Windows `UnsupportedHandoff`'s own
permanent answer, never the Windows "not currently installed" answer),
the manager sets `StatePlatformUnsupported` and the automatic loop never
starts, regardless of the persisted `AutoCheck` preference. `CheckNow`/
`Download`/`Install` all refuse immediately in this state, mirroring the
existing `ErrDisabled` pattern with a new `ErrPlatformUnsupported`
sentinel. Settings shows an honest "Automatic updates are not yet
available on this platform" message instead of a "you're up to date"
state that never actually checked anything. No DMG is ever downloaded
automatically on macOS in 20C1; no install action is ever exposed as
usable. Windows behavior is completely unchanged - this gate only ever
activates for a `Handoff` that reports itself permanently unsupported,
which `WindowsHandoff` never does.

## 21. Cross-platform artifact naming

The cross-platform portability baseline milestone deliberately deferred
renaming the Windows installer until a second platform actually
introduced a release artifact (`docs/platform-support.md` §16) - Stage
20C1 is that point. New stable naming convention, applied consistently:

```
StreamingTreeForOBS-<version>-windows-amd64-setup.exe
StreamingTreeForOBS-<version>-darwin-arm64.dmg
StreamingTreeForOBS-<version>-darwin-amd64.dmg
```

Every release artifact name now identifies product, version, OS,
architecture, and package kind where necessary. Renaming the Windows
installer is safe precisely because Stage 20B's updater always resolves
a download through release-manifest metadata (exact artifact name,
exact size, exact SHA-256, matched against the same GitHub Release's
own assets array) - never a hard-coded "find this one `.exe`" rule; no
updater code depends on the old literal filename anywhere. This rename
is applied to `scripts/installer/streaming-tree.iss`, and the full
Windows release/update regression (`build-release.ps1`,
`verify-packaged-app.mjs`, `verify-installer.mjs`, `verify-updater.mjs`)
is re-run afterward to prove the rename did not disturb Windows
behavior (§43 of the governing task).

## 22. Multi-artifact release manifest

Stage 20B's manifest schema already modeled a list of artifacts,
`OS`/`Arch`/`Kind`/`Name`/`SizeBytes`/`SHA256` each - `schemaVersion`
does not change for this milestone; the existing schema already
supports multiple entries, this milestone is the first to actually
populate more than one. `cmd/releasemanifest` (Stage 20B's single-
artifact generator) still describes exactly one artifact per
invocation - Windows and macOS builds never run on the same machine, so
a single invocation accepting several artifact descriptors at once
could never actually be satisfied by one real release pipeline anyway.
Instead, a new optional `-in <existing-manifest.json>` flag was added:
when given, that manifest's own artifacts are carried over and the
newly-described one is appended (and the whole result re-validated);
omitting `-in` behaves exactly as it did in Stage 20B (a fresh, single-
artifact manifest), so `scripts/build-release.ps1`'s own existing
invocation needed no change. This is how one canonical manifest gets
assembled across separate per-platform build invocations - never a
second, per-platform manifest format; the final public GitHub Release
will eventually carry exactly one `streaming-tree-release.json`
covering every platform's artifact. No GitHub Release is created in
20C1.

## 23. macOS release build script

`scripts/build-release-macos.sh` - runs only on macOS (verifies the
Darwin host and a supported architecture before doing anything else),
mirrors `scripts/build-release.ps1`'s own structure and safety
discipline: validates a strict version string, verifies required tools
(Go, npm, `hdiutil`), builds the frontend with the existing lockfile
discipline (`npm ci`), builds the Go executable natively with
`CGO_ENABLED=1`, injects the exact same canonical version/commit/
packaged-mode `-ldflags` metadata Windows already uses, assembles the
real `.app` bundle (`Info.plist`, the executable, the four legal
documents), creates the DMG via `hdiutil`, computes a SHA-256 digest,
and emits release-artifact metadata compatible with the canonical
manifest generator - self-validating its own output before declaring
success, exactly like the Windows script's own manifest-generation step
already does. It never installs Homebrew, never installs Node at
runtime (Node/npm are expected to already be present, exactly like the
Windows script's own expectation of a pre-installed toolchain), never
publishes anything, never signs with any identity (real or fake), never
notarizes, and never creates a Git tag. Generated artifacts remain
git-ignored, identical to the Windows build's own `build/release/`
convention.

## 24. Build tools

Only Apple/macOS tools already present on GitHub's hosted runners are
used: `bash`, `plutil` (for `Info.plist` validation), `hdiutil`, and
standard system tooling. `codesign` is used only for inspection where
useful (e.g. confirming the produced binary's architecture), never for
real signing in this milestone. No third-party packaging framework and
no downloaded DMG-builder tool is introduced.

## 25. Unsigned build truth

Stage 20C1 packages are **unsigned and not notarized** - stated
explicitly, everywhere this matters (README, `docs/platform-support.md`,
this document). No self-signed certificate is created and presented as
if it were a real Developer ID; no certificate or private key is ever
committed to the repository; the operator is never asked for Apple
credentials during this milestone. The build is structurally ready for
Stage 20C2 to insert real signing (the release script already produces
a clean, unambiguous `.app` a future signing step can target directly),
but actual signing is entirely out of scope here.

## 26. Stage 20C2 boundary

Stage 20C2 will require, at minimum: a real Apple Developer Program /
Developer ID Application identity; real `codesign` signing with the
hardened runtime enabled; any entitlements that prove necessary once
signing is actually attempted; `notarytool` submission and result
validation; stapling; a real Gatekeeper assessment of the signed,
notarized package; a real macOS updater install handoff (Stage 20B's
own `Handoff` interface already anticipates this - a future
`DarwinHandoff` implements the same interface `WindowsHandoff` does);
and an explicit public/Beta distribution policy. **None of this is
implemented, attempted, or claimed complete in Stage 20C1.** The
operator is not asked for any of it now.

## 27. Native macOS package CI workflow

`.github/workflows/macos-package.yml` - a package-verification gate,
not a release workflow. No publication, no secrets, no Developer ID, no
notarization, no package upload via `actions/upload-artifact` (an
unsigned `.dmg` is never made downloadable through CI - built, tested,
and discarded when the ephemeral runner is destroyed; only safe
filenames/hashes appear in logs). `permissions: contents: read` only.
Verifies both `macos-latest` (Apple Silicon) and `macos-15-intel`
(Intel) - the current official labels confirmed in §2 - building and
testing the real package natively on each. Triggers:
`workflow_dispatch` and `push` to `main` with path filtering scoped to
the shared Go production runtime, the web frontend, macOS packaging
files, the shared release-manifest code, and the workflow file itself -
broad enough that a shared core change is never silently unverified,
narrow enough that an unrelated documentation-only change does not
burn macOS runner time for no reason.

## 28. macOS package verification helper

`scripts/verify-macos-package.mjs` - a **platform-specific CI
verification helper**, explicitly not counted as canonical local
integration script #25. The canonical local integration-script count
remains **24** (Stage 20B's own count, unchanged) - this helper only
ever runs meaningfully on macOS, inside `macos-package.yml`, and is
documented separately from the 24 canonical Windows/local scripts,
exactly the way this milestone's own governing task requires. It proves
(on each native architecture): the real `.app` bundle exists with a
correct, parseable `Info.plist` (`plutil -lint`), the correct stable
bundle identifier and version fields, an executable matching the
runner's own architecture, real packaged build metadata via
`--version`, all four legal documents present under `Resources`, the
executable actually starts and serves real health/About/production-
frontend/SPA-route/legal-route responses from loopback only, the
creator/licence/support identity is unchanged (`Czekosabe`,
`GPL-3.0-or-later`, the existing StreamElements URL), TTS is honestly
reported unavailable, the updater reports the new platform-unsupported
state rather than a false "up to date" and never exposes a usable
install action, `Quit` triggers the real shared shutdown path and the
process actually exits, a genuine second-launch attempt is detected by
the real `flock`-based single-instance mechanism rather than starting a
second backend, the DMG itself mounts/contains the correct `.app`/
unmounts cleanly, a copy of the app in a hermetic temporary
"Applications-like" directory starts and stays healthy with its data
still outside the bundle, and no Streaming Tree process or mounted disk
image is left behind afterward. Run for real, to a clean pass, **at
least twice per architecture** before this milestone is considered
package-verified, per the governing task's own requirement.

## 29. Package security

Packaging paths exist only in build tooling (`scripts/
build-release-macos.sh`, the verification helper, and the CI workflow
that invokes them) - never in a product HTTP API. No API accepts an
arbitrary filesystem path, mounts or executes a DMG, executes an
arbitrary binary, opens an arbitrary file, or runs a shell command
assembled from request input, on any platform. The web frontend remains
an ordinary local HTTP UI, unchanged by this milestone.

## 30. Manual verification limitation

The operator owns no physical Mac. **No operator-owned physical Mac
manual test was performed for this milestone, and none is claimed.**
GitHub-hosted native macOS CI is real native execution on real Apple
hardware/OS - meaningfully stronger evidence than cross-compilation -
but it is not equivalent to a human's real Finder UX, Dock UX,
Gatekeeper first-launch experience (doubly relevant once 20C2's
unsigned-vs-notarized distinction actually matters to a real user), OBS
Browser Source rendering on a real Mac, real audio-device behavior, or
real creator/operator Keychain use. This limitation is stated plainly
rather than implied away by a green CI badge.

## 31. Known limitations after Stage 20C1

- Unsigned, not notarized (§25) - Stage 20C2.
- No macOS updater install path (§20) - Stage 20C2's own `DarwinHandoff`.
- No public/Beta release of any kind - no GitHub Release, no Git tag.
- No final application icon - pre-public-release polish, same boundary
  Windows already has.
- No real end-user/OBS/Gatekeeper manual verification (§30) - only
  automated native-CI verification, matching every prior stage's own
  testing discipline.
- No universal/fat binary - two separate architecture-specific packages
  instead, deliberately (§4).
