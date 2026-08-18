# Stage 20D1 — Linux local/desktop packaged runtime + native package verification contract

**Research date:** 2026-08-18.

## 1. Scope: local/desktop only

Stage 20D1 packages Streaming Tree for the same local-desktop model
Windows (Stage 20A) and macOS (Stage 20C1) already ship: OBS/encoder +
Streaming Tree + MediaMTX + operator-provided FFmpeg, all on **the same
machine**, loopback-only. It is explicitly NOT: remote server mode, VPS
mode, LAN management, a headless systemd service, remote OBS ingest, a
public management API, TLS termination, reverse-proxy mode, multi-user
server mode, or remote authentication. Every one of those is Stage
20D2's own, separately-threat-modeled scope - see §9 of the Stage 20D1
governing task, restated here as a hard boundary this document does not
cross. Nothing in this milestone binds any listener to a non-loopback
address, and nothing in it adds authentication, TLS, or a headless
secret-storage fallback.

## 2. Primary-source research performed (2026-08-18)

- **XDG Base Directory Specification** (freedesktop.org, current):
  `XDG_CONFIG_HOME` defaults to `$HOME/.config`, `XDG_DATA_HOME` to
  `$HOME/.local/share`, `XDG_CACHE_HOME` to `$HOME/.cache`.
  `XDG_RUNTIME_DIR` has no default - when set, it "MUST be owned by the
  user, and they MUST be the only one having read and write access to
  it" (mode `0700`), "MUST be created when the user first logs in" and
  removed on complete logout, and "Files in the directory MUST not
  survive reboot or a full logout/login cycle." It is not always set
  (minimal sessions, some CI/container environments), so this contract
  never assumes its presence.
- **Desktop Entry Specification** (freedesktop.org, v1.5, current): a
  `.desktop` file's `Exec` key must be "an executable program optionally
  followed by one or more arguments" and explicitly **cannot** contain
  shell constructs - no pipes, no redirection, no shell command
  evaluation. Field codes (`%f`/`%F`/`%u`/`%U`/`%i`/`%c`/`%k`) are
  optional and at most one of `%f`/`%u`/`%F`/`%U` may appear; Streaming
  Tree takes no file/URL argument at all, so none are used. Quoting (when
  needed) uses double quotes with backslash-escaping of `"`, `` ` ``,
  `$`, and `\`.
- **xdg-open / xdg-utils** (freedesktop.org / Portland project, current):
  the documented exit codes are `0` success, `1` command-line syntax
  error, `2` a referenced file did not exist, `3` a required tool could
  not be found, `4` the action itself failed - `browserlaunch_linux.go`
  (already shipped, audited below) already treats any non-zero exit as a
  nonfatal error, consistent with this.
- **Secret Service D-Bus specification** (freedesktop.org): requires a
  running D-Bus **session** bus and a Secret Service-implementing agent
  (GNOME Keyring, KWallet's Secret Service shim, KeePassXC's Secret
  Service integration, etc.) - a legitimate assumption for an
  interactive desktop session, the exact scope Stage 20D1 targets. A
  headless environment with no session D-Bus and no such agent is
  explicitly Stage 20D2's own unsolved problem, not Stage 20D1's.
- **Package format research** (§4 below) across Debian Policy, AppImage,
  Flatpak, and Snap official documentation.
- **GitHub-hosted Linux runners**: re-confirmed current official labels
  rather than trusted from memory - `ubuntu-latest` (currently Ubuntu
  24.04) for x64, `ubuntu-24.04-arm` for ARM64 - both already the exact
  labels this repository's own `cross-platform.yml` uses today, so no
  new runner label risk is introduced.
- **Go Linux support**: this repository's `go.mod` floors at Go 1.25.0.
  Go's own Linux port has no meaningfully narrow "minimum distro"
  floor beyond a reasonably modern kernel/glibc when CGO is used - and,
  as §6 establishes, Stage 20D1's Linux build uses `CGO_ENABLED=0`,
  producing a fully static binary with **no runtime shared-library
  dependency at all**, sidestepping any distro-glibc-version question
  entirely. No specific minimum distro version is invented or claimed.

## 3. Audit of the real current Linux state (before any design)

Read directly, not from memory:

- `browserlaunch_linux.go` (added in Stage 20C1's own darwin/linux
  split): already real and correct - `exec.Command("xdg-open", url)`,
  fixed argv, no shell, matches §2's xdg-open research exactly. **No
  change needed.**
- `singleinstance_other.go`: still the Stage 20A "always succeeds"
  stub, now scoped `!windows && !darwin` (so Linux still falls through
  to it) - explicitly insufficient for a packaged Linux app. **Needs a
  real `singleinstance_linux.go`.**
- `nativealert_other.go`: still stderr-only, `!windows && !darwin` -
  insufficient for a launcher with no visible terminal. **Needs a real
  `nativealert_linux.go`.**
- `internal/runtime/branch/process_unix.go` and
  `internal/runtime/mediamtx/process_unix.go`: both `//go:build
  !windows`, already fully portable to Linux (Setpgid process-group
  isolation + real `SIGTERM` graceful termination, already proven on
  Linux by `cross-platform.yml`'s own `backend (linux-amd64)`/`(linux-
  arm64)` jobs). **No change needed.**
- `internal/runtime/ffmpeg/resolver.go`: already fully portable - its
  only `runtime.GOOS` branches are the `.exe` suffix and the Windows
  executable-bit check, both already correct for Linux. **No change
  needed.**
- `internal/runtime/mediamtx/platform.go`: `linux/amd64` and
  `linux/arm64` asset entries already present. **No change needed to
  the matrix** - only native verification (§13) is new.
- `internal/secrets/keyring_store.go`: `keyring.SecretServiceBackend`
  already listed unconditionally among the allowed backends. Read
  `github.com/99designs/keyring@v1.2.2/secretservice.go` directly: its
  build tag is `//go:build linux` only - **no `cgo` requirement**,
  unlike the macOS Keychain backend. It talks to D-Bus via
  `github.com/godbus/dbus` and `github.com/gsterjov/go-libsecret`, both
  pure Go. This means Linux release builds can use `CGO_ENABLED=0`
  (§6) and still compile the real Secret Service backend - confirmed
  already exercised today by `cross-platform.yml`'s own `linux-amd64`/
  `linux-arm64` jobs, which already build with `CGO_ENABLED=0`.
- `internal/updater/handoff_other.go`: build tag is `//go:build
  !windows` (not `!windows && !darwin`) - the Stage 20C1
  `StatePlatformUnsupported` gate built on top of
  `BlockerPlatformUnsupported` therefore **already applies to Linux
  release builds with zero further Go code changes**. This is verified
  natively in §13/§14, not re-implemented.
- `cmd/server/main.go`: `singleinstance.Acquire()`/
  `browserlaunch.Open()`/`nativealert.ShowFatalError()` call sites are
  unchanged since Stage 20A/20C1 - the new Linux adapters plug into the
  exact same interfaces, no wiring changes needed.
- `internal/config.resolveDataDir`: already resolves via
  `os.UserConfigDir()`, which is `$XDG_CONFIG_HOME` (or
  `~/.config`) joined with `AppDirName = "StreamingTree"` on Linux -
  already correct, confirmed by direct source read. **No change
  needed**, only contract documentation (§8).
- `cmd/releasemanifest`: already supports the `-in` flag shipped in
  Stage 20C1 (§10 below reuses it verbatim, no further extension
  needed).

## 4. Package-format decision

**Selected: a `.deb` package (Debian/Ubuntu family), built with
`dpkg-deb`.** Evaluated against AppImage, Flatpak, and Snap:

- **AppImage**: genuinely distro-agnostic and needs no install step,
  but `appimagetool` itself ships *as* an AppImage, which needs FUSE to
  run directly in a container - real, avoidable CI friction - and,
  more importantly, AppImage has no first-class desktop-menu
  integration by default (a `.desktop` entry embedded in the image is
  not automatically registered with the host's application menu without
  a separate integration daemon such as AppImageLauncher, which this
  project cannot assume is installed). Given §8/§9 of the governing task
  require a real, verified desktop-launcher story, this is a real
  functional gap, not a stylistic preference.
- **Flatpak / Snap**: both are built around a sandboxed runtime/store
  model (Flatpak's own shared runtimes, Snap's confinement and snapd
  daemon) that adds real operational surface - a sandboxing/permission
  model, a runtime dependency, and in Snap's case a background daemon -
  none of which this single self-contained Go binary needs, and both
  are naturally oriented at a store/publish workflow this milestone
  explicitly does not use (no publisher account, no store submission).
- **`.deb`**: this repository's own native Linux CI is already
  Ubuntu-based (`ubuntu-latest`, `ubuntu-24.04-arm`), so `dpkg-deb`,
  `dpkg`, and `apt` are already present with zero added tooling. The
  format has first-class, guaranteed desktop-menu integration via
  `/usr/share/applications/*.desktop`, a real, auditable install/remove
  lifecycle (`dpkg -i` / `dpkg -r`) directly testable in ephemeral CI,
  and (since the binary is `CGO_ENABLED=0`, fully static - §6) needs no
  `Depends` field at all.

Because `.deb` is a Debian/Ubuntu-family format, **the support claim is
correspondingly narrow**: this milestone proves "Debian/Ubuntu package
CI-verified on x64 and ARM64," never generic "Linux supported." A
distribution outside the Debian/Ubuntu family (Fedora/RPM, Arch, etc.)
is out of this milestone's scope entirely - not attempted, not claimed.
Only one package format is built in Stage 20D1, matching the one-format
precedent Windows and macOS each already set.

## 5. Package identity

- **Debian package name:** `streaming-tree-for-obs` (lowercase, hyphen-
  separated, per Debian Policy's package-name character rules).
- **Maintainer field:** `Czekosabe <Czekosabe@users.noreply.github.com>`
  - GitHub's own standard noreply-email convention for the project's
  real public identity (`github.com/Czekosabe`, already
  `buildinfo.CreatorURL`), never the personal Git commit email
  (`kacper2280@tlen.pl`) - the governing task explicitly forbids
  exposing the Git email as product identity.
- **Description / Product:** `Streaming Tree for OBS`.
- **License (documentation, not a `.deb` control field):**
  `GPL-3.0-or-later`, matching `buildinfo.ApplicationLicenseSPDX`
  exactly.
- **Architecture:** Debian's own architecture names, `amd64` and
  `arm64`, which happen to equal Go's own `GOARCH` values for these two
  targets - no translation table needed.

## 6. Build policy: CGO disabled, fully static binary

Linux release builds use `CGO_ENABLED=0` - confirmed sufficient in §3:
the real Secret Service backend needs no CGO. The resulting binary is
fully static (no dynamic dependency on glibc or anything else), so the
`.deb` declares no `Depends` field and can install and run correctly on
any Debian/Ubuntu-family system regardless of installed library
versions. This is a meaningfully simpler build/runtime story than
Windows's MSVC runtime or macOS's CGO-linked Keychain backend, and is
recorded explicitly so a future change to this policy is a deliberate,
reviewed decision.

## 7. Package filesystem layout

```
/usr/bin/streaming-tree-server                                  (0755)
/usr/share/applications/streaming-tree-for-obs.desktop            (0644)
/usr/share/doc/streaming-tree-for-obs/copyright                   (0644)
/usr/share/doc/streaming-tree-for-obs/LEGAL.md                    (0644)
/usr/share/doc/streaming-tree-for-obs/PRIVACY.md                  (0644)
/usr/share/doc/streaming-tree-for-obs/THIRD_PARTY_NOTICES.md      (0644)
```

`copyright` is the repository's own `LICENSE` file content, placed
under Debian's own conventional `copyright` filename so the license is
discoverable exactly where Debian tooling expects it; the other three
legal documents are shipped alongside it under their own names, exactly
mirroring the Windows/macOS convention of shipping all four documents
loose beside the package contents, in addition to the same documents
already served over HTTP from the embedded frontend (`/legal/*`,
unchanged). The production React frontend remains embedded directly in
the Go executable (`internal/webassets`, unchanged) - no sidecar Node/
Vite files, no separate frontend process, identical to Windows/macOS.
No maintainer scripts (`preinst`/`postinst`/`prerm`/`postrm`) are used -
a plain file-copy package needs none, and this avoids any root-executed
custom logic entirely (§16). One accepted, honestly-documented
consequence: a desktop environment's application-menu cache may not
refresh immediately after install without a session restart or a
manual `update-desktop-database` call, since no `postinst` trigger runs
one automatically - a cosmetic limitation, not a functional one (the
binary and its `.desktop` entry are both installed correctly either
way).

## 8. Application data

Unchanged and already correct (§3): `internal/config.resolveDataDir`
already resolves to `$XDG_CONFIG_HOME/StreamingTree` (or
`~/.config/StreamingTree`) via `os.UserConfigDir()` - no migration, no
code change. The package installs only to `/usr/bin` and
`/usr/share/...`, both package-owned and immutable from the
application's own perspective; all mutable state (SQLite, managed
visual/audio assets, the managed MediaMTX installation, Secret Service
entries) lives entirely outside the package, under the user's own home
directory. Removing or upgrading the `.deb` (`dpkg -r`/`dpkg -i`) never
touches `/usr`-external user data.

## 9. Packaged-mode identification

Unchanged from Windows/macOS: `buildinfo.Packaged()` is true only when
the new `scripts/build-release-linux.sh` injects `packagedFlag=true`
via `-ldflags -X`, the same unexported variable every platform shares.
`runtime.GOOS == "linux"` is never used to infer packaged mode - a
Linux developer still gets the normal `go run`/Vite workflow unchanged.

## 10. Linux browser launch

Already correct (§3) - `browserlaunch_linux.go`'s existing `xdg-open`
call needs no change. Requirements (already met): fixed executable
name, never a shell string; only the application-generated loopback
management URL is ever passed; launch happens only after the server is
actually ready; failure is nonfatal (the backend keeps running); a
second packaged launch triggers the same safe open-URL behavior once
the running first instance is detected (§11); headless CI never
attempts to open a real graphical browser (the existing
`STREAMING_TREE_TEST_NO_UI` seam, already used by Windows/macOS
verification, is reused unchanged - §14 exercises it for real).

## 11. Linux single instance

The current `!windows && !darwin` "always succeeds" stub is explicitly
insufficient for a packaged Linux app. Implemented for real in a new
`internal/runtime/singleinstance/singleinstance_linux.go`: an
exclusive, non-blocking `flock(2)` on a fixed, per-user, application-
owned lock file - directly reusing the same kernel primitive and the
same "closing the fd releases the lock automatically, including on a
crash" property `singleinstance_darwin.go` already relies on (§2's
XDG_RUNTIME_DIR research informs *where* the lock file lives, not the
locking mechanism itself, which is identical to macOS). Location
policy: prefer `$XDG_RUNTIME_DIR/StreamingTree/.instance.lock` when
`XDG_RUNTIME_DIR` is set (session-scoped, mode-0700-owned by the user,
automatically cleared on logout - a natural fit for a live-process
lock); fall back to the same per-user data directory
(`$XDG_CONFIG_HOME/StreamingTree/.instance.lock`, honoring the
`STREAMING_TREE_DATA_DIR` test-isolation override exactly like macOS)
when `XDG_RUNTIME_DIR` is unset or empty - a documented, safe fallback
rather than assuming every environment sets it. No plaintext PID file
is ever used as sole proof of another instance - only a live,
kernel-held `flock` proves a live owner; an unrelated process merely
occupying port 8080 is never mistaken for Streaming Tree, since nothing
about this mechanism inspects the port at all. A second launch that
fails to acquire the lock does not start a second backend and instead
triggers the existing safe open-URL behavior for the first instance's
management URL, exactly like Windows/macOS.

## 12. Linux fatal-startup UX

There is no single universal Linux modal-dialog API the way Win32/
AppKit provide one. Research confirms the realistic desktop-appropriate
options are the `zenity` (GTK) and `kdialog` (KDE) command-line tools -
both common on their respective desktop environments, neither
guaranteed present on every Linux installation, and pulling in a full
GUI toolkit merely to show one fatal-error string is not justified by
the evidence. Implemented in a new
`internal/runtime/nativealert/nativealert_linux.go` as an honestly
**best-effort** chain: if `zenity` is found on `PATH` (via
`exec.LookPath`, never assumed), invoke it with fixed flags
(`--error --title=... --text=...`) and the title/message passed as
literal argv elements - never interpolated into a shell string of any
kind; otherwise, if `kdialog` is found, the same pattern
(`--error MESSAGE --title TITLE`); otherwise fall back to the existing
stderr behavior. This is documented here and in living support docs as
best-effort, not a guaranteed cross-desktop mechanism - never described
as "native" the way the Windows `MessageBoxW`/macOS `NSAlert`
mechanisms are, since neither `zenity` nor `kdialog` is part of the
base OS the way those platform APIs are.

## 13. Desktop entry

A real `.desktop` file, matching the Desktop Entry Specification
research in §2 exactly:

```ini
[Desktop Entry]
Type=Application
Name=Streaming Tree for OBS
Exec=/usr/bin/streaming-tree-server
Terminal=false
Categories=AudioVideo;
```

`Exec` is a fixed absolute path with no arguments and no field codes -
Streaming Tree takes no file/URL argument, so none of `%f`/`%u`/`%F`/
`%U` are used, and (per §2's research) the key could not safely contain
a shell string even if one were wanted. `Terminal=false` since the
application is a GUI-adjacent background server controlled through the
browser, not a terminal program. No `Autostart` entry is installed (the
user launches it explicitly, mirroring the Windows/macOS "no autostart
by default" decision). No `Icon` key is set: no canonical project icon
exists yet (confirmed by the same audit `docs/product-identity-legal.md`
already performed for Windows/macOS) - inventing one now, or using an
OBS/Twitch/YouTube/Kick/TikTok mark, is explicitly out of scope; a
missing `Icon` key is valid per the specification and desktop
environments fall back to a generic icon. Validated in CI with
`desktop-file-validate` when available on the runner (§14).

## 14. Quit / graceful shutdown

Unchanged: the existing protected `POST /api/system/shutdown` endpoint
and its shared `context.CancelFunc` shutdown path are reused exactly as
they are on every other platform - no Linux-specific shutdown
architecture. Closing a browser tab never quits the application.

## 15. Secret Service

Kept as the real desktop credential backend (§3, §6) - no plaintext
fallback, no `secrets.json`, no unencrypted env-file fallback, no
hardcoded master password. Stage 20D1 is desktop/local mode, where a
graphical-session D-Bus Secret Service is a legitimate assumption;
Stage 20D2 headless secret storage remains a separate, unsolved
problem this milestone does not touch. The native package CI (§17)
starts a hermetic, ephemeral D-Bus session bus plus an ephemeral
Secret Service implementation (`gnome-keyring-daemon --unlock`-free
test mode or an equivalent minimal implementation available on the
runner) strictly to prove the existing opt-in credential-store smoke
test passes against a *real* Secret Service - never against any real
user credential, requiring no committed secret, and torn down
unconditionally at the end of the job. If this cannot be made reliably
hermetic on the runner, the CI job says so plainly rather than faking a
pass - see §17 for the exact, honestly-scoped claim.

## 16. Package installation requires no runtime root

Installing a `.deb` with `dpkg`/`apt` requires elevated privileges - an
intrinsic property of the package-manager format, not something this
project introduces. The **application itself** always runs as the
normal desktop user: no setuid binary, no root daemon, no privileged
helper process, no `sudo` call from Streaming Tree itself, no root-
owned user database or configuration, and no background service
installed for desktop mode (§17). This distinction - install-time
package-manager privilege vs. strictly unprivileged runtime - is the
same one already accepted for Windows's per-user Inno Setup installer
and is recorded explicitly here.

## 17. No systemd service in 20D1

Stage 20D1 is an interactive local desktop application, launched by the
user and exited through the existing Quit action - exactly like the
Windows/macOS packaged apps. No `systemd` service or user service unit
is installed or enabled. Server/service lifecycle ownership is entirely
Stage 20D2's own, separately-designed scope.

## 18. MediaMTX and FFmpeg on Linux

MediaMTX: the asset matrix already lists real `linux/amd64`/
`linux/arm64` entries (§3); the native package CI (§17 below - package
verification) proves platform asset selection, archive/executable-
permission handling, configured-path resolution, and process start/
stop/cleanup semantics for real, on native Linux, exactly like the
macOS milestone did. Loopback-only RTMP/Control-API policy is
completely unchanged - nothing in this milestone touches that boundary.

FFmpeg: remains entirely operator-provided, unchanged. No bundling, no
downloading a third-party build, no automatic `apt`/`dnf`/`pacman`
install. The packaged app starts and is fully usable without it;
outgoing branch streaming remains unavailable until the operator
supplies a compatible executable. README guidance for Linux stays
honest: point at the operator's own distribution's package manager or
the official FFmpeg downloads page, never an unaudited third-party
static-build distributor.

## 19. System TTS

Not implemented in Stage 20D1, same honest limitation as macOS:
`tts/stub.go` already reports `Capabilities.Available == false` on
Linux. No shell-out to `espeak`/`festival`/`spd-say` as a packaging
shortcut. A real native Linux TTS provider is separate future feature
work.

## 20. Linux updater behavior

Confirmed by direct source audit (§3): `internal/updater/handoff_other.go`
is gated `//go:build !windows`, so the `StatePlatformUnsupported` gate
Stage 20C1 built already applies to a Linux release build with **zero
further Go code changes** - `Manager.Start` never begins automatic
polling, and `CheckNow`/`Download`/`Install` all refuse immediately
with `ErrPlatformUnsupported`, exactly like macOS. This milestone does
not implement a Linux updater install handoff: no automatic `apt`/`dnf`
invocation, no `sudo` call, no privilege escalation, no unattended
package-manager command of any kind - a safe, unprivileged Linux update
handoff (most plausibly: download a new `.deb` and hand it to `pkexec
dpkg -i` or equivalent, with the same manifest-verified integrity
model) is a real, understandable future architectural option, but is
explicitly deferred to a later, separately-justified release-hardening
decision rather than implemented speculatively here. Settings shows the
same honest "Automatic updates are not yet available on this platform"
state already shipped for macOS - no frontend change needed (§24).

## 21. Artifact naming and release manifest

Continuing Stage 20C1's convention exactly:

```
StreamingTreeForOBS-<version>-linux-amd64.deb
StreamingTreeForOBS-<version>-linux-arm64.deb
```

`cmd/releasemanifest`'s existing `-in` mechanism (shipped and tested in
Stage 20C1, corrected in its own contract this same milestone - see the
Stage 20C1 reconciliation entry) is reused verbatim, with no schema or
CLI change: each of `scripts/build-release.ps1`,
`scripts/build-release-macos.sh`, and the new
`scripts/build-release-linux.sh` describes exactly one artifact per
invocation and can fold into an existing manifest via `-in`. A five-
artifact manifest (`windows/amd64/installer` +
`darwin/arm64/dmg` + `darwin/amd64/dmg` + `linux/amd64/deb` +
`linux/arm64/deb`) is proven with real Go tests: `ArtifactFor` resolves
each identity to exactly the right artifact, with no fuzzy or cross-
architecture match and the existing duplicate-identity/duplicate-name
rejection intact. `manifest.Kind` already defines `KindDeb` (anticipated
since Stage 20B alongside `KindInstaller`/`KindDMG`/`KindPKG`/
`KindAppImage`/`KindRPM`) - no enum change needed, only real usage for
the first time. The manifest → exact GitHub Release
asset name → size → SHA-256 security model is completely unchanged.

## 22. Linux release build script

`scripts/build-release-linux.sh` - a Linux-only bash script mirroring
`scripts/build-release-macos.sh`'s own structure: verifies a real Linux
host and a supported architecture (`x86_64→amd64`, `aarch64→arm64`)
before doing anything else, validates a strict version string, verifies
required tools (`go`, `npm`, `dpkg-deb`), builds the frontend with
`npm ci`, stages the embedded frontend/legal directories exactly like
the other two scripts, builds the Go executable natively with
`CGO_ENABLED=0` (§6), injects the same canonical version/commit/
packaged-mode `-ldflags` metadata every platform uses, assembles the
`.deb` staging layout (§7), writes `DEBIAN/control`, builds the package
with `dpkg-deb --build`, computes a SHA-256 digest, and - only for a
strict `major.minor.patch` version - invokes `cmd/releasemanifest`
(with an optional `-in` pass-through). Never installs anything via
`apt`, never publishes, never signs, never creates a Git tag. Generated
artifacts remain git-ignored, matching `build/release{,-macos}/`.

## 23. Native Linux package CI workflow

`.github/workflows/linux-package.yml` - a package-verification gate,
not a release workflow, mirroring `macos-package.yml`'s own shape
exactly: `contents: read` only, no secrets, no artifact upload of the
built `.deb`. Matrix over `ubuntu-latest` (amd64) and `ubuntu-24.04-arm`
(arm64) - the same labels `cross-platform.yml` already uses. Triggers:
`workflow_dispatch` and `push` to `main` with path filtering scoped to
the shared Go core, the shared frontend, the Linux packaging scripts,
the shared release-manifest code, the legal documents, and the workflow
file itself. Runs the real build-and-verify cycle **twice** per
architecture in one job (two independent rebuild+reverify passes, each
with its own version string), satisfying the "verified at least twice"
requirement in one execution, exactly like the macOS workflow already
does.

## 24. Linux package verification helper

`scripts/verify-linux-package.mjs` - a platform-specific CI
verification helper, explicitly **not** canonical local integration
script #25; the canonical count remains **24**. Documented separately
as: 24 canonical local/Windows integration scripts + the macOS package
CI helper + the Linux package CI helper. Mirrors
`verify-macos-package.mjs`'s own step/pass/fail/expect style, proving
(on each native architecture): the real `.deb` exists and its control
metadata (`dpkg-deb --info`) reports the correct package name/
architecture/version; the package installs for real via `dpkg -i`
(using CI's own `sudo`, since installation intrinsically needs it -
§16); the installed binary starts as a normal, unprivileged user;
`--version` reports real packaged metadata; `/usr/share/doc/
streaming-tree-for-obs/{copyright,LEGAL.md,PRIVACY.md,
THIRD_PARTY_NOTICES.md}` are present and readable; the `.desktop` entry
is installed and validates; health/About/production-frontend/SPA/
legal-route serving all work; TTS/updater honestly report
unavailability; a real second-launch attempt is detected by the real
`flock`-based mechanism; graceful shutdown actually exits the process;
`dpkg -r` removes the package cleanly without touching the test user's
application data; and no Streaming Tree/MediaMTX/D-Bus-test process or
temporary directory is left behind. Run to a clean pass **at least
twice per architecture** before this milestone is considered package-
verified.

## 25. Package security

Packaging paths exist only in build tooling
(`scripts/build-release-linux.sh`, the verification helper, and the CI
workflow that invokes them) - never in a product HTTP API, exactly like
Windows/macOS. No API accepts an arbitrary filesystem path, installs or
removes a package, executes an arbitrary binary, or runs a shell
command assembled from request input.

## 26. Manual verification limitation

No operator-owned Linux desktop manual test was performed for this
milestone (this document does not claim one occurred unless it
genuinely did). Native GitHub-hosted Ubuntu CI is real native
execution, meaningfully stronger evidence than cross-compilation, but
it is not equivalent to a human's real GNOME/KDE desktop session, real
OBS Browser Source rendering on a real Linux machine, real audio-device
behavior, or a human clicking through `dpkg -i`/the application menu.
Browser-launch is proven through the native runtime/test seam
(`STREAMING_TREE_TEST_NO_UI`), not a human GUI session, unless a real
desktop session is genuinely used later.

## 27. Known limitations after Stage 20D1

- Debian/Ubuntu family only - no RPM/Arch/other distro family package,
  and no claim of one.
- No Linux in-app updater install path - `platform_unsupported`,
  matching macOS.
- No Linux system TTS.
- No final application icon (same pre-public-release polish gap as
  Windows/macOS).
- Desktop-menu refresh after install may require a session restart or a
  manual `update-desktop-database` call, since no maintainer script
  triggers one automatically (§7).
- No public/Beta release of any kind - no GitHub Release, no Git tag.
- No operator-owned physical Linux desktop manual test (§26).
- Stage 20D2 (headless/remote/server mode) is a fully separate, not-yet-
  designed threat model - nothing in Stage 20D1 assumes or prepares its
  specific security mechanisms beyond preserving the existing loopback-
  only boundary unconditionally.
