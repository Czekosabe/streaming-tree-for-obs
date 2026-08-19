# Platform support and Stage 20 portability contract

This document is the canonical record of what "Streaming Tree for OBS"
supports on which operating system, what is merely *technically possible*
versus actually verified, and the roadmap that will close the gap. It exists
because Stage 20A (see [`windows-packaging.md`](windows-packaging.md))
implemented a real Windows production runtime and installer, and before any
further packaging work begins for another platform this project needs one
place that states, precisely, what is true today.

Research for this document was performed 2026-08-18 against GitHub's own
Actions documentation, Apple's own Developer documentation, the
freedesktop.org XDG Base Directory and Secret Service specifications, and
MediaMTX's own upstream configuration reference. Sources are cited inline
where a specific claim depends on them.

## 1. Vocabulary

These words are used precisely and are not interchangeable:

- **Supported** — a real user can install and run this today, with the
  platform's native packaging, and the experience has had at least some
  real-device/human validation beyond automated CI.
- **Automated-build verified** — an official CI runner for that exact OS/
  architecture compiles and/or tests the shared application core. This is
  evidence the *code* is portable; it is not evidence of a packaged,
  installable, or human-validated experience.
- **Native CI verified** — automated-build verification that ran on the
  real target OS/architecture via a GitHub-hosted runner (as opposed to
  cross-compilation from a different host OS).
- **Cross-compilation verified** — the Go toolchain produced a binary for
  that OS/architecture from a *different* host OS, without ever running or
  testing it there. This is the weakest verification tier and must never be
  described as build-verified for a platform whose production build has
  native, OS-specific dependencies (see §4 on macOS Keychain).
- **Planned** — a real target for future work; nothing platform-specific
  has shipped yet.
- **Experimental** — exists and runs, but with known, undocumented-elsewhere
  rough edges the project is not yet ready to call Planned-complete or
  Supported.
- **Deferred** — investigated and explicitly not pursued now, for a
  documented reason (mirrors the vocabulary already used for Stage 15B/16B/
  19's feasibility gates).
- **Unsupported** — will not run, or runs with a feature honestly reported
  as unavailable rather than faked.
- **Not verified** — no automated or manual check of any kind has been
  performed for this claim.

A component is never called Supported merely because Go *can* compile it.

## 2. The one-core architecture rule

There is one application: a shared Go core (`apps/server`) plus a shared
React frontend (`apps/web`), with a small number of narrow OS/runtime
adapters (`internal/runtime/browserlaunch`, `internal/runtime/
singleinstance`, `internal/runtime/nativealert`, `internal/provider/tts`,
`internal/runtime/branch` process control, `internal/runtime/mediamtx`
process control) selected at compile time via Go build tags, plus
platform-specific *packaging* on top (installer scripts, CI jobs). This
project does not fork into "Streaming Tree Windows" / "Streaming Tree
macOS" / "Streaming Tree Server" - provider/event/chat/alert/widget/storage
domains, the HTTP API, and the frontend are shared and platform-independent
by construction. Every platform-facing package audited in §3 already
follows this pattern from earlier stages; no refactor was required to
establish it.

## 3. Stage 20A Windows-coupling audit result

Before writing this contract, the actual Stage 20A source was inspected
end-to-end: `apps/server/cmd/server/main.go`, `internal/buildinfo`,
`internal/webassets`, the three runtime adapter packages, the HTTP
production/legal/shutdown routes (`internal/httpapi/production.go`,
`legal.go`, `system.go`), `internal/secrets/keyring_store.go`, `internal/
provider/tts`, `internal/runtime/mediamtx/platform.go`, `internal/runtime/
ffmpeg/resolver.go`, `internal/runtime/branch/process_{windows,unix}.go`,
`go.mod`/`go.sum`, `scripts/build-release.ps1`, and `scripts/installer/
streaming-tree.iss`.

Result: **no unnecessary Windows coupling was found**, so no refactor was
performed. Specifically:

- `main.go` only ever calls the three runtime adapters through their
  package-level functions (`browserlaunch.Open`, `singleinstance.Acquire`,
  `nativealert.ShowFatalError`); it never branches on `runtime.GOOS` itself
  and never imports a Windows-only package directly. Its one `syscall.
  SIGTERM` reference is portable (defined on every Go-supported OS).
- `internal/buildinfo` and `internal/webassets` are pure Go with no OS-
  conditional code at all - `internal/webassets` is a plain `embed.FS`
  wrapper, and `go.mod`'s only direct dependencies with any OS-specific
  code (`github.com/99designs/keyring`, `github.com/go-ole/go-ole`,
  `golang.org/x/sys`) are all already scoped behind build tags or, in
  go-ole's case, only imported from `internal/provider/tts/windows.go`.
- Every Windows-only package already has a real `!windows` counterpart
  written in an earlier stage, not added by this milestone:
  `browserlaunch_other.go` (shells to `open`/`xdg-open`), `singleinstance_
  other.go` (always succeeds - a real cross-platform single-instance
  mechanism is deferred to a future packaged non-Windows target),
  `nativealert_other.go` (writes to stderr, since non-Windows builds never
  use the console-free release mode), `tts/stub.go` (honestly reports
  `Capabilities.Available == false`, never fakes a `say`/shell-command
  provider), and `runtime/branch/process_unix.go` (SIGTERM instead of an
  immediate kill, since non-Windows FFmpeg can shut down gracefully).
- `internal/httpapi/production.go`, `legal.go`, and `system.go` (the
  production static/SPA host, the `/legal/*` allowlist, and the protected
  shutdown endpoint) contain no platform branching at all; their only
  "windows" references are doc comments pointing at `windows-packaging.md`.
  The one backslash-rejection check in `production.go` is a security
  hardening measure (rejecting a Windows-style path separator in a request
  path regardless of which OS is actually serving it), not platform
  coupling.
- `internal/secrets/keyring_store.go` is fully portable: it lists all three
  allowed backends (`WinCredBackend`, `KeychainBackend`,
  `SecretServiceBackend`) unconditionally, relying on `keyring.Open` to
  silently skip whichever is unavailable on the current OS, and it already
  defers opening the real backend to first use rather than at construction
  - see §5 for why that matters for a future headless Linux target.
- `internal/runtime/mediamtx/platform.go` already carries a release-asset
  matrix for `windows/amd64`, `linux/amd64`, `linux/arm64`, `darwin/amd64`,
  and `darwin/arm64` - this predates this milestone and already anticipates
  the roadmap below.
- `internal/runtime/ffmpeg/resolver.go` uses `runtime.GOOS` only for two
  narrow, correct reasons: choosing the `.exe` suffix for a hypothetical
  future bundled copy, and skipping the Unix executable-bit check on
  Windows (which has none).
- `scripts/build-release.ps1` and `scripts/installer/streaming-tree.iss`
  are Windows-only tooling by design (PowerShell + Inno Setup) and are
  left exactly as they are - per this milestone's own scope, a non-Windows
  package is 20C/20D's decision, not this one's.

`cmd/server` was confirmed to actually cross-compile for `linux/amd64` and
`linux/arm64` (`CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build ./cmd/
server`, both exit 0, verified locally on 2026-08-18) - see §7 for why the
equivalent `darwin` check is deliberately *not* treated as verification.

## 4. Windows (x64) — primary target

**Status: Supported.**

Windows x64 is where Stage 20A actually shipped: one Go process serving a
production React build, launching the user's own default browser via
`ShellExecuteW`, a `CreateMutexW`-based single-instance guard, a
`MessageBoxW` fatal-startup dialog on the console-free release binary, a
protected `POST /api/system/shutdown` endpoint, release-injected version/
commit metadata, and a per-user Inno Setup installer bundling the four
legal documents. Full detail is in
[`windows-packaging.md`](windows-packaging.md); this section only restates
current status, it does not change it.

| Component | Status |
| --- | --- |
| Go build/test | Supported |
| Frontend production build/serving | Supported |
| SQLite persistence | Supported |
| Provider connectors (Twitch/YouTube/StreamElements) | Supported |
| MediaMTX managed install | Supported |
| FFmpeg (operator-provided, never bundled) | Supported |
| OS credential store (Windows Credential Manager) | Supported |
| Packaged browser launch | Supported |
| Single-instance detection | Supported |
| Fatal-startup-error UX | Supported |
| Quit (protected shutdown) | Supported |
| System TTS (SAPI) | Supported |
| Package format (Inno Setup, per-user) | Supported |
| Code signing (Authenticode) | Not implemented - honestly unsigned, `SignTool=` hook prepared inert |
| Automated CI (native Windows runner) | Automated-build verified - see §6 |
| Application updater | Supported - Stage 20B, see [updater.md](updater.md) |
| Real OBS/provider manual verification | Not verified by this project beyond developer use - out of scope for automated CI |

## 5. macOS — Stage 20C1 Completed

**Status: Stage 20C1 Completed** (see
[macos-packaging.md](macos-packaging.md) for the full contract). A real,
**unsigned and not notarized** `.app` bundle inside a DMG is built and
verified natively on both Apple Silicon and Intel GitHub-hosted CI
runners. Stage 20C2 (Developer ID signing, hardened runtime, notarization,
stapling, updater install handoff, public/Beta readiness) remains
Planned, externally gated on real Apple Developer credentials this
project does not have. The operator does not own a Mac; nothing here
claims manual hardware/Finder/Gatekeeper/OBS verification.

### 5.1 Architecture preference

Both Apple Silicon (`darwin/arm64`, the preferred modern target) and Intel
(`darwin/amd64`) are packaged. GitHub's currently-documented hosted
runners offer an Intel macOS label (`macos-15-intel` / `macos-26-intel` as
of this research date, alongside Apple-Silicon `macos-latest`/`macos-15`/
`macos-14`), cross-checked against this repository's own real, already-
green CI history for that label - Intel remains natively CI-verifiable at
no extra tooling cost. No universal/fat binary is built; each architecture
is a separate, independently identified release artifact (docs/
macos-packaging.md §4).

### 5.2 Per-architecture component status

| Component | `darwin/arm64` | `darwin/amd64` |
| --- | --- | --- |
| Go build (CGO-enabled) | Native CI verified (see §6) | Native CI verified (see §6) |
| Frontend build | Automated-build verified (platform-independent Node build, not architecture-specific) | same |
| SQLite (CGO-free `modernc.org/sqlite`) | Native CI verified | Native CI verified |
| Provider connectors | Automated-build verified (platform-independent HTTP/gRPC code) | same |
| MediaMTX managed install | Asset matrix has `darwin-arm64`/`darwin-amd64` entries (§3); managed-install/process lifecycle exercised as far as the existing test architecture allows | same |
| FFmpeg (operator-provided) | Resolver logic is portable; the packaged app starts and runs without FFmpeg, streaming is unavailable until the operator supplies one | same |
| OS credential store (Keychain, requires CGO) | Native CI verified — see §5.3, this requires the *real* runner, cross-compilation cannot verify it | Native CI verified |
| Packaged browser launch | Real `browserlaunch_darwin.go` (uses `open`), native-CI-verified via the `STREAMING_TREE_TEST_NO_UI` seam | same |
| Single-instance | Real `flock`-based mechanism (`singleinstance_darwin.go`), native-CI-verified with two real processes | same |
| Fatal-startup UX | Real NSAlert/Cgo bridge (`nativealert_darwin.go`/`.m`); compiled and linked by native CI, never invoked live in CI (would block on a modal with no user present) | same |
| Quit | Real shared HTTP shutdown endpoint, native-CI-verified | same |
| System TTS | **Unavailable today** - `tts/stub.go` honestly reports `Capabilities.Available == false`; no macOS `AVSpeechSynthesizer`/`say` provider exists. This is an honest limitation, not a bug. | same |
| Updater | Recognized as a real release build, but automatic polling never starts (`platform_unsupported` state) - no macOS install path exists yet (Stage 20C2) | same |
| Package format | Real `.app` bundle inside a DMG (`hdiutil`), native-CI-verified mount/copy/run/unmount cycle, twice per architecture | same |
| Signing / notarization | **Not implemented** - no Apple Developer account exists for this project yet (Stage 20C2) | Not implemented |
| Automated CI | Native CI verified: build + `go vet` + `go test` + full package build/verify, twice per architecture (`.github/workflows/macos-package.yml`) | Native CI verified |
| Manual hardware/UX/OBS verification | Not verified - operator owns no Mac | Not verified |

### 5.3 Why the macOS Keychain build gate matters

`github.com/99designs/keyring`'s Keychain backend
(`keychain.go` inside that module) carries the build constraint `//go:build
darwin && cgo`, and the underlying `github.com/99designs/go-keychain`
binding literally does `import "C"` against `CoreFoundation`/`Security`
frameworks. This was confirmed by direct inspection of both modules' source
during this milestone's audit (§3).

A concrete, reproduced finding from this audit: cross-compiling with
`GOOS=darwin GOARCH=arm64 CGO_ENABLED=0` from this Windows development
machine **succeeds with exit code 0** - but silently produces a binary
missing the real Keychain backend entirely, because Go excludes any file
containing `import "C"` when CGO is disabled, and the `darwin && cgo`
constraint on `keychain.go` means nothing else in the package even
references the missing symbols. A binary built this way would look
correct and would still run, but `keyring.Open` would simply never offer
`KeychainBackend` as an available option, and no static analysis or
successful-exit-code check would reveal that.

This is exactly why this document does not accept a CGO-disabled Windows-
host cross-compile as evidence of macOS credential-store portability, and
why the macOS CI job (§6) is required to build with CGO enabled on a real
macOS runner - proving the production binary compiles against the real
platform backend, not a substitute.

### 5.4 macOS packaging - implemented in Stage 20C1, signing/notarization still research-only

Package format, browser launch, single-instance, and fatal-startup UX are
now real, implemented, native-CI-verified code - see
[macos-packaging.md](macos-packaging.md) for the full contract. Per
Apple's own current developer documentation, still research-only and
explicitly NOT implemented in Stage 20C1: distribution outside the Mac
App Store requires signing with a Developer ID Application certificate
with the hardened runtime enabled, followed by submission to Apple's
notarization service via `notarytool` (the modern replacement for the
retired `altool`); Gatekeeper checks the notarization ticket (typically
"stapled" to the artifact) at first launch and otherwise blocks or warns.
None of this requires Xcode itself for a Go binary, but it does require an
Apple Developer Program membership, a real Apple ID with 2FA, and access to
`codesign`/`notarytool` (available via Xcode Command Line Tools) - all of
it is Stage 20C2's own scope, externally gated on credentials this project
does not have. macOS system TTS exists via `AVSpeechSynthesizer`/the `say`
command, but building a real provider for it is separate future feature
work, not part of Stage 20C1/20C2.

### 5.5 macOS support-claim progression

This project will not describe macOS as "supported" merely because CI goes
green. The intended progression is: **Planned** → **Automated-build/test
verified** (the cross-platform portability baseline: native CI compiles
and tests the shared core and proves the Keychain build gate) →
**Unsigned package verified** (Stage 20C1, current state: a real,
unsigned, not-notarized `.app`/DMG built and verified twice per
architecture on native CI, including real single-instance/browser-launch/
DMG-lifecycle proof) → **Beta / automated-test supported** (once Stage
20C2 produces a real signed, notarized, installable package) → **Fully
supported** after real-device/human validation of launch, browser-open,
TTS-absence messaging, OBS Browser Source rendering, and Gatekeeper
behavior. GitHub-hosted macOS runners execute on real Apple hardware/OS,
which is meaningfully stronger than cross-compilation, but they still do
not replace a human clicking through the installed app.

## 6. GitHub Actions cross-platform CI baseline

A new workflow, `.github/workflows/cross-platform.yml`, was added by this
milestone as a **portability gate**, not a release workflow: it verifies
the shared application core compiles and its platform-neutral tests pass
on each officially available target OS/architecture. It does not build an
installer, does not run the 23 integration scripts, does not publish
anything, and uses no secrets.

### 6.1 Runner labels used (current, 2026-08-18, per GitHub's own runner
reference documentation)

| Target | Runner label | Notes |
| --- | --- | --- |
| Windows x64 | `windows-latest` | Currently maps to Windows Server 2025-based image; free for this public repository |
| Linux x64 | `ubuntu-latest` | Currently maps to Ubuntu 24.04-based image; free for this public repository |
| Linux ARM64 | `ubuntu-24.04-arm` | Native ARM64 hosted runner, free for public repositories; included because MediaMTX itself ships ARM64 releases (§3) |
| macOS Apple Silicon | `macos-latest` (currently `macos-15`, M-series) | Free for this public repository |
| macOS Intel | `macos-15-intel` | Free for this public repository; explicit Intel label rather than a "latest" alias since GitHub does not offer an Intel "latest" alias |

This repository (`Czekosabe/streaming-tree-for-obs`) is public
(`"private": false`, confirmed via the unauthenticated GitHub REST API
during this milestone), so every one of these hosted runner types,
including macOS, runs at no cost under GitHub's public-repository Actions
policy.

### 6.2 Job content

Per platform: `gofmt -l .` (fails the job on any diff), `go vet ./...`,
`go test -count=1 ./...`, `go build ./...`. On macOS specifically, `CGO_
ENABLED` is left at its default (enabled) rather than forced to `0`, so the
real Keychain backend in `internal/secrets` actually compiles - see §5.3.
On Windows and Linux, `CGO_ENABLED=0` is used for `go build`/`go test`,
matching this project's existing CGO-free posture (SQLite is `modernc.org/
sqlite`, a pure-Go driver) - the real Windows Credential Manager and Linux
Secret Service backends in `github.com/99designs/keyring` do not require
CGO. A single Linux x64 job additionally runs the frontend checks
(`npm ci`, `i18n:check`, `typecheck`, `lint`, `test -- --run`, `build`) -
not duplicated across every OS, since the frontend build itself is
platform-independent Node tooling and there is no platform-specific reason
to repeat it four times. No job runs the opt-in real-credential-store
smoke test (`keyring_store_smoketest_test.go` is guarded behind its own
opt-in build tag/env var and is not exercised in CI); ordinary tests
continue to use the existing fake secret store.

### 6.3 Permissions and triggers

The workflow declares `permissions: contents: read` only - no `write`,
`packages`, or `id-token` scope, since it never publishes anything.
Triggers are `pull_request`, `push` to `main`, and `workflow_dispatch`.
No schedule trigger was added: this is a portability gate meant to run
when code changes, not a recurring job burning macOS/ARM64 runner minutes
on a timer with no corresponding change to review.

### 6.4 Actual run result

Recorded in the closing journal entry (`docs/progress.md`) with the real
workflow run ID(s), per-job pass/fail outcome, and any fix made in
response to a genuine failure - this document states the intended design;
the journal entry states what the real first run actually did.

## 7. Local cross-compilation checks performed

From this Windows development machine, `GOOS=linux GOARCH=amd64
CGO_ENABLED=0 go build ./cmd/server` and `GOOS=linux GOARCH=arm64 CGO_
ENABLED=0 go build ./cmd/server` were both run and both exited 0 (2026-08-
18). This is recorded as **Cross-compilation verified** for Linux x64/
ARM64 build-time portability only - it is not runtime verification, and it
predates and is superseded by the Native CI verification in §6 for the
same targets.

A `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/server` was also
attempted and also exited 0 - but per §5.3, this result is **deliberately
not recorded as any form of macOS verification**, because it silently
excludes the real Keychain backend. The only meaningful macOS compile gate
this project uses is the native, CGO-enabled macOS CI job in §6.

## 8. Linux desktop / local mode

**Status: Stage 20D1 Completed** (see
[linux-desktop-packaging.md](linux-desktop-packaging.md) for the full
contract). Concept: OBS/encoder, Streaming Tree, MediaMTX, and FFmpeg
all run on the same Linux machine, preserving today's loopback-only
model exactly as it is on Windows - this is not the remote server mode
in §9. A real, **unsigned** `.deb` package for the Debian/Ubuntu family
is built and verified natively on both x64 and ARM64 GitHub-hosted CI
runners - only that family is claimed, never generic "Linux supported".

| Component | Status |
| --- | --- |
| Go build (`linux/amd64`, `linux/arm64`, `CGO_ENABLED=0`) | Native CI verified (§6) |
| Frontend build | Automated-build verified (platform-independent) |
| SQLite | Native CI verified |
| Browser launch | Real `browserlaunch_linux.go` (`xdg-open`, fixed argv), native-CI-verified via the `STREAMING_TREE_TEST_NO_UI` seam |
| Single-instance | Real `flock`-based mechanism (`singleinstance_linux.go`, preferring `XDG_RUNTIME_DIR`), native-CI-verified with two real processes |
| Fatal-startup UX | Best-effort `zenity`/`kdialog` chain (`nativealert_linux.go`), falling back to stderr - honestly documented as best-effort, not a guaranteed cross-desktop mechanism |
| MediaMTX managed install | Asset matrix covers `linux-amd64`/`linux-arm64` (§3); managed-install/process lifecycle exercised as far as the existing test architecture allows |
| FFmpeg (operator-provided) | Resolver logic portable; the packaged app starts and runs without FFmpeg |
| Secret Service credential store | Native CI verified for the build gate; a real ephemeral D-Bus session + `gnome-keyring-daemon` now also exercises the existing opt-in credential-store smoke test for real, non-fatally skipped if the tooling is unavailable on the runner |
| System TTS | **Unavailable today** - same honest `stub.go` behavior as macOS, no Linux provider exists |
| Updater | Recognized as a real release build, but automatic polling never starts (`platform_unsupported` state) - no Linux install path exists yet |
| Package format | Real `.deb` (Debian/Ubuntu family), native-CI-verified `dpkg -i`/`dpkg -r` install-and-remove lifecycle, twice per architecture |
| Signing | **Not implemented** - no Linux release signing exists at any stage yet |
| Manual hardware/UX/OBS verification | Not verified - no operator-owned physical Linux desktop test was performed |

The current absence of Linux TTS is stated plainly rather than hidden: the
feature gap is real, and the application already reports it honestly at
runtime via `Capabilities.Available == false` rather than silently
degrading or pretending a provider exists.

## 9. Linux headless / self-hosted server mode

**Status: Stage 20D2A Completed** (see [linux-headless-server.md](linux-headless-server.md)
for the full contract) **- still loopback-only, still not remote.**
Stage 20D2 as a whole is split into three parts, each with its own
threat model: 20D2A (this section - a real unattended-service
foundation, loopback-only), 20D2B (the future secure remote management/
control plane), and 20D2C (the future remote OBS ingest/data plane).
Only 20D2A is implemented; 20D2B/20D2C remain exactly as unimplemented
as this section originally described the whole of 20D2.

**What 20D2A actually implements**, real and native-CI-verified: an
explicit `--headless` CLI flag (never inferred from `runtime.GOOS`/
`DISPLAY`); a real systemd unit (`DynamicUser=yes` +
`StateDirectory=`/`RuntimeDirectory=`, hardened per current
`systemd.exec` documentation, `Restart=on-failure`, a fixed
`ExecStart=` with no shell string); a small provisioning helper for a
real 32-byte master key delivered to the service exclusively via
systemd's own `LoadCredential=` mechanism (never `Environment=`, never
a command-line argument); a real AES-256-GCM encrypted headless secret
store (§11) selected only in headless mode, never on a normal Linux
desktop package; and headless-mode-only startup validation that
**actively rejects** a non-loopback management bind
(`0.0.0.0`/`::`/a LAN address) rather than merely documenting the
restriction as a future requirement.

**What remains exactly as unimplemented as before** - all Stage
20D2B/20D2C scope, untouched by 20D2A:

- **Management authentication, sessions, CSRF, trusted origins.** The
  management API still has no authentication at all - it still relies
  entirely on being reachable only from the same machine (loopback),
  now actively enforced rather than merely conventional.
- **TLS / reverse-proxy contract and trusted proxy headers.** Still
  undesigned.
- **Rate limiting and secure remote shutdown.** `POST /api/system/
  shutdown` is still protected only by an exact-JSON-body check plus
  loopback-only reachability - still not a remote-safe design.
- **Public overlay exposure.** Overlay/alert/widget routes are still
  reachable by anyone who can reach the loopback port - still not
  remote-safe.
- **Remote OBS ingest and ingest authentication/transport security.**
  See §10 - Stage 20D2C's own scope, untouched.

This **still cannot** be obtained by simply setting a bind address like
`STREAMING_TREE_HOST=0.0.0.0` - in headless mode, doing so is now an
explicit, actively-rejected startup error rather than merely an
unsupported configuration.

## 10. Remote OBS ingest is a separate security boundary

MediaMTX's RTMP listener and Control API are deliberately loopback-only
today, and this milestone does not weaken that. Per MediaMTX's own
upstream configuration reference (`mediamtx.yml`, checked during this
milestone's research), the project already ships optional building blocks
for a future authenticated/encrypted ingest path - `rtmpEncryption`
(`no`/`strict`/`optional`) with `rtmpServerKey`/`rtmpServerCert` for RTMPS,
an internal user database, external HTTP auth, or JWT/JWKS auth for the
Control API, `rtmpTrustedProxies` and `apiTrustedProxies` for IP-based
trust, and per-user IP allowlisting - but selecting and wiring any of this
is explicitly deferred to a dedicated future server-security stage, not
decided here. Binding an unauthenticated RTMP listener to `0.0.0.0` is
explicitly rejected as a way to "fix" remote ingest; a safe model
(authenticated RTMP/RTMPS, SRT if justified, a VPN/private-network
deployment expectation, or an explicit MediaMTX auth mechanism) needs its
own threat-model milestone. The MediaMTX Control API must remain private
and must never become remotely exposed as a side effect of any future
change.

## 11. Headless secret storage - solved by Stage 20D2A

`internal/secrets` supports four backends now: the three real OS-native
keyrings (§3 - Windows Credential Manager, macOS Keychain, Linux Secret
Service, selected on every platform exactly as before) plus a new
`HeadlessStore` (`internal/secrets/headlessstore.go`), selected only
when `--headless` is explicitly given. Per the freedesktop.org Secret
Service specification, Secret Service is a D-Bus API backed by desktop-
session daemons (GNOME Keyring, KWallet) that depends on an active
D-Bus session bus - something a headless systemd service with no
graphical login session typically does not have, confirmed during
Stage 20D2A's own research. `HeadlessStore` never attempts to open one:
it is a single AES-256-GCM-encrypted JSON file, keyed by a 32-byte
master key delivered exclusively via systemd's `LoadCredential=`
mechanism (never a plaintext `secrets.json`, never an unencrypted
config file, never an env-file fallback, never a hardcoded password -
see [linux-headless-server.md](linux-headless-server.md) §8/§9 for the
full envelope/nonce/AAD/corruption-handling design and §17-§21 of the
Stage 20D2A governing task for the security requirements it was built
against). Every entry gets a fresh random nonce and is bound to its own
key via AEAD associated data; tampering, truncation, an unknown
envelope version, or a wrong master key are all detected and rejected,
proven by 23 focused Go tests. Losing the master key makes existing
encrypted secrets permanently unrecoverable - stated plainly, not
hidden, since that is the correct, expected behavior of real
authenticated encryption with no back door.

`internal/secrets/keyring_store.go` still defers opening the real
desktop backend until first use rather than at construction - unchanged
and still exactly correct for the desktop path; `HeadlessStore`, by
contrast, deliberately fails closed at construction (Stage 20D2A's own
explicit decision, [linux-headless-server.md](linux-headless-server.md)
§13): a headless service whose mandatory secret backend cannot
initialize must not report itself healthy while every configured
provider credential is silently unusable, unlike an interactive desktop
user who can reasonably unlock a keychain after launch.

## 12. MediaMTX platform matrix

| Platform | Managed install status |
| --- | --- |
| `windows/amd64` | Supported (Stage 20A) |
| `linux/amd64` | Native-CI-verified managed-install/process lifecycle (Stage 20D1, `scripts/verify-linux-package.mjs`) |
| `linux/arm64` | Native-CI-verified managed-install/process lifecycle (Stage 20D1) |
| `darwin/amd64` | Native-CI-verified managed-install/process lifecycle (Stage 20C1, `scripts/verify-macos-package.mjs`) |
| `darwin/arm64` | Native-CI-verified managed-install/process lifecycle (Stage 20C1) |

"Native-CI-verified" means the real managed download/install/start/stop
lifecycle was exercised on real native CI runners for that platform -
not merely that the asset matrix resolves an entry for it. No platform
here has had this verified by an operator's own physical hardware.

## 13. FFmpeg platform policy

Unchanged everywhere: FFmpeg is always operator-provided and never
bundled, on every platform, now and in the future - this project does not
control FFmpeg's build provenance or licensing and does not take on that
responsibility by shipping a copy. `internal/runtime/ffmpeg/resolver.go`
is already fully portable (§3); only the operator-facing install guidance
for non-Windows platforms remains to be written, as part of a future
20C/20D milestone.

## 14. TTS platform matrix

| Platform | Status |
| --- | --- |
| Windows | Supported - real SAPI provider (`tts/windows.go`) |
| macOS | Unavailable today - honestly reported via `Capabilities.Available == false`, no provider exists |
| Linux | Unavailable today - same honest stub, no provider exists |

No CI job or test on any platform is permitted to fake
`Capabilities.Available = true` to claim parity; native macOS/Linux TTS is
separate future feature work, not part of this baseline.

## 15. Stage 20B updater: cross-platform artifact-identity constraint

Stage 20B (the application updater) is now **implemented** - see
[updater.md](updater.md) for the full contract. This section, originally
written as a forward-looking constraint before implementation, now
records that the constraint was actually honored: the shipped
`internal/updater/manifest.Identity{OS, Arch, Kind}` model was built
exactly this way from the start, so Windows x64 (`windows/amd64/
installer`, the only platform the updater actually installs anything on
today) required no special-casing that a future platform would need to be
carved back out of.

The update-check contract identifies a downloadable artifact by at least:
**OS** (`windows`/`darwin`/`linux`), **architecture** (`amd64`/`arm64`),
**package/artifact kind** (the shipped enum already lists `installer`,
`dmg`, `pkg`, `appimage`, `deb`, `rpm`), **version**, a **SHA-256**
digest, and a download resolved only from the same trusted GitHub
Release's own assets array (never an arbitrary URL accepted from the
frontend - the release manifest itself carries no download-URL field at
all, by design). `windows/amd64/installer`, `darwin/arm64/dmg`,
`darwin/amd64/dmg`, `linux/amd64/deb`, and `linux/arm64/deb` are all now
real, chosen, and actually produced identities (Stages 20A/20C1/20D1).
**Only `windows/amd64/installer` is auto-installable through the
updater's own handoff today** - macOS and Linux release builds report
`platform_unsupported` and never attempt an install, even though their
artifacts exist and are named (see [macos-packaging.md](macos-packaging.md)
§20, [linux-desktop-packaging.md](linux-desktop-packaging.md) §20). A
future Linux RPM/Arch-family package, if ever added, would reuse this
same already-multi-platform-ready identity shape without any manifest
schema change.

## 16. Current cross-platform artifact naming

Stage 20C1 is the point this document's own §16 (as originally written)
said the Windows name should be revisited: macOS is now a second
platform that produces a real release artifact, so every artifact name
encodes product, version, OS, and architecture together.
`scripts/installer/streaming-tree.iss` now produces
`StreamingTreeForOBS-<version>-windows-amd64-setup.exe` (e.g.
`StreamingTreeForOBS-0.1.0-windows-amd64-setup.exe`, renamed from the
prior `StreamingTreeForOBS-Setup-<version>.exe`), and
`scripts/build-release-macos.sh` (docs/macos-packaging.md §23) produces
`StreamingTreeForOBS-<version>-darwin-arm64.dmg` and
`StreamingTreeForOBS-<version>-darwin-amd64.dmg`. The rename is safe
because Stage 20B's updater always resolves a download through release-
manifest metadata (exact name, exact size, exact SHA-256, matched
against the same GitHub Release's own assets array) - no updater code
anywhere depends on the old literal filename; `scripts/build-release.ps1`
discovers the installer's real filename dynamically
(`Get-ChildItem -Filter '*.exe'`) rather than hard-coding it, so nothing
there needed to change. `scripts/verify-installer.mjs` and
`scripts/verify-updater.mjs` were re-run against the renamed artifact as
part of Stage 20C1's own closing regression. Stage 20D1 followed the
same shape for real: `scripts/build-release-linux.sh` (docs/linux-
desktop-packaging.md §22) produces
`StreamingTreeForOBS-<version>-linux-amd64.deb` and
`StreamingTreeForOBS-<version>-linux-arm64.deb`.

## 17. Roadmap (Stage 20, expanded)

| Stage | Scope | Status |
| --- | --- | --- |
| 20A | Windows production runtime and installer (see [`windows-packaging.md`](windows-packaging.md)) | **Completed** |
| 20B | Application updater (GitHub Releases check, update UI, download/verification, real Windows installer/updater handoff, see [updater.md](updater.md)) - uses the cross-platform artifact-identity concept in §15; Windows x64 remains the only platform it actually serves | **Completed** |
| 20C1 | macOS packaged runtime, unsigned `.app`/DMG, native macOS CI package verification (see [macos-packaging.md](macos-packaging.md)) | **Completed** |
| 20C2 | macOS Developer ID signing, hardened runtime, notarization, stapling, updater install handoff, public/Beta readiness | Planned - externally gated on real Apple Developer credentials |
| 20D | Linux platform support, split into: | Incomplete |
| 20D1 | Linux local/desktop runtime and packaging: a real `.deb` for the Debian/Ubuntu family, native x64/ARM64 CI package verification (§8, see [linux-desktop-packaging.md](linux-desktop-packaging.md)) | **Completed** |
| 20D2A | Linux headless service foundation: loopback-only unattended systemd operation, secure encrypted headless secret storage (§9, §11, see [linux-headless-server.md](linux-headless-server.md)) | **Completed** |
| 20D2B | Secure remote management/control plane: authentication, sessions, CSRF, TLS/reverse-proxy contract, remote-safe shutdown, public-overlay exposure policy (§9) | Planned |
| 20D2C | Remote OBS ingest/data plane: authenticated/encrypted ingest, MediaMTX remote-ingest policy, final combined self-hosted validation (§10) | Planned |
| 20E | Logs/diagnostics, final release hardening, and final manual/platform verification | Planned |

Stage 20 as a whole remains **Incomplete**: 20A, 20B, 20C1, 20D1, and
20D2A are Completed; 20C2, 20D2B, 20D2C, and 20E remain Planned. 20C2
is externally gated on real Apple Developer signing/notarization
credentials this project does not have - it is not
blocked on any further engineering decision.

## 18. What the cross-platform portability baseline milestone explicitly did not do (historical)

This section is historical: it records the explicit exclusions of the
*cross-platform portability baseline* milestone specifically (the one
that first wrote this document, before Stage 20B or Stage 20C1
existed). It is kept for the record rather than deleted, but several of
these items have since been implemented by later milestones - reading
it as still describing the *current* state would now be wrong. Kept
current below, one line per original claim:

- No macOS package (`.app`/`.dmg`/`.pkg`) - **since implemented**:
  Stage 20C1 shipped a real, unsigned, not-notarized `.app`/DMG, native-
  CI-verified on both architectures (see [macos-packaging.md](macos-packaging.md)).
- No Linux package (`.deb`/`.rpm`/AppImage/Flatpak/Snap/systemd service)
  - **partially since implemented**: Stage 20D1 shipped a real,
  unsigned `.deb` for the Debian/Ubuntu family, native-CI-verified on
  both architectures (see [linux-desktop-packaging.md](linux-desktop-packaging.md));
  no RPM/Arch/other distro-family package, and no systemd service,
  exist or are planned for 20D1.
- No code signing, no notarization submission, no Apple Developer
  account or certificate request - **still true**: this is Stage 20C2's
  own scope, externally gated on real Apple Developer credentials this
  project does not have.
- No Stage 20B updater code - **since implemented**: Stage 20B shipped
  the real GitHub Releases update system (see [updater.md](updater.md)).
- No GitHub Release, no Git tag - **still true**: no stage through
  20D1 has published a public release.
- No binding of the management API to a non-loopback address, no
  weakening of MediaMTX's loopback-only RTMP/Control-API policy - **still
  true and unconditional**: every stage through 20C1 preserved this,
  and Stage 20D1 (Linux local/desktop mode) preserves it too; only a
  future, separately-designed Stage 20D2 remote/headless mode could
  ever revisit it, under its own distinct threat model.
- No remote authentication system, no TLS termination, no headless
  secret-storage fallback of any kind - **still true**: these remain
  Stage 20D2's own unsolved scope, not addressed by 20C1 or by Stage
  20D1's local/desktop-only Linux work.
- No macOS/Linux TTS implementation - **still true**: both remain
  honestly reported as unavailable (`Capabilities.Available == false`),
  with a real native provider left as separate future feature work on
  either platform.
