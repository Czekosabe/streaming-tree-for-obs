# Stage 20D2A — Linux headless service foundation + secure headless secret storage

**Research date:** 2026-08-19.

## 1. The 20D2 split and this milestone's exact scope

Stage 20D2 (Linux headless/self-hosted server mode) is split into three
independently-completable milestones, each with its own threat model:

- **20D2A** (this document): a real, unattended Linux headless service
  foundation and secure headless secret storage - **loopback-only**, no
  remote exposure of any kind. This is infrastructure other stages build
  on, not a remote product.
- **20D2B** (future): the secure remote management/control plane -
  authentication, sessions, CSRF, trusted origins, a reverse-proxy/TLS
  contract, remote-safe shutdown/actions, and a public-overlay exposure
  policy. Nothing in 20D2A implements any of this.
- **20D2C** (future): the remote OBS ingest/data plane - authenticated/
  encrypted ingest, MediaMTX remote-ingest policy, and final combined
  self-hosted deployment validation.

Stage 20D2 as a whole remains **Incomplete** until all three are done.
Stage 20E (logs/diagnostics beyond this milestone's basic operational
logging, final release hardening/signing, final manual/platform
verification) is untouched by this milestone.

**This document defines 20D2A only.** It explicitly does not implement,
and must not be read as implementing, any 20D2B or 20D2C capability.

## 2. The hard network boundary (restated, unconditional)

Every listener this application ever opens - management HTTP, MediaMTX
RTMP ingest, MediaMTX Control API, public overlay routes - remains
**loopback-only** in every mode this milestone introduces, exactly as
strictly as the existing desktop modes. Headless mode does not relax
this even slightly; it exists specifically so an *unattended* process
can run this same loopback-only local application without a graphical
session, not so it can be reached remotely. No login, session cookie,
CSRF token, reverse-proxy trusted-header handling, TLS, remote
shutdown, remote overlay exposure, remote RTMP, RTMPS, SRT, or VPN
integration is implemented here - all of that is 20D2B/20D2C's own,
separately-designed scope.

## 3. Audit of the real current implementation (before any design)

Read directly, not from memory:

- **CLI parsing** (`cmd/server/main.go` `handleEarlyFlags`): a single
  `flag.Parse()` call handles `--version` and the internal updater-
  helper flags (`updater.FlagUpdateHelper` etc.), returning early for
  either. Normal startup proceeds through `run()` otherwise. This is
  the natural place to add a new, equally explicit `--headless` flag -
  parsed once, alongside the existing flags, never inferred from
  environment absence.
- **Packaged-mode identification**: `buildinfo.Packaged()` is set only
  by `-ldflags -X ...packagedFlag=true`, injected only by the three
  release-build scripts. Headless mode is a **separate, additional**
  concern layered on top of an already-packaged Linux build - a
  headless deployment is still `Packaged() == true` (it embeds the
  production frontend and exposes the real shutdown endpoint exactly
  like the desktop build), plus the new explicit `--headless` flag.
- **Browser launch / single-instance / nativealert call sites** (`run()`
  in `main.go`): both `browserlaunch.Open` (twice - the second-launch
  fallback at the top of `run()`, and the post-listener call near the
  bottom) and `nativealert.ShowFatalError` (in `main()`, on a fatal
  `run()` error) are gated on `buildinfo.Packaged()` only today - no
  headless awareness exists yet. Both call sites need a headless check
  added alongside the existing `Packaged()` check.
- **Signal/shutdown behavior**: `signal.NotifyContext(..., os.Interrupt,
  syscall.SIGTERM)` already handles `SIGTERM` gracefully via the exact
  same `shutdownRuntime` closure every platform already uses - **no
  change needed** for headless mode; systemd's own `SIGTERM`-then-
  `SIGKILL`-after-timeout stop sequence already matches what this code
  already does.
- **Data-dir resolution** (`internal/config.resolveDataDir`): defaults
  to `os.UserConfigDir()` (`$XDG_CONFIG_HOME`/`~/.config`) joined with
  `AppDirName`, or the `STREAMING_TREE_DATA_DIR` override. A service
  account has no meaningful "user config directory" the same way an
  interactive user does - headless deployments **must** set
  `STREAMING_TREE_DATA_DIR` explicitly (the override this code already
  supports) rather than requiring any new resolution logic.
- **Mutable state locations** (audited across `internal/storage/sqlite`,
  `internal/domain/visualasset`, `internal/runtime/mediamtx`): all
  already resolve under `cfg.DataDir` or an explicit sub-path of it -
  no location outside `cfg.DataDir` is ever written to at runtime on
  any platform. This means one `StateDirectory` covers everything a
  headless deployment needs to persist.
- **SecretStore** (`internal/secrets`): a clean port
  (`Set`/`Get`/`Delete`/`Exists(ctx, key string, ...)`), already used
  uniformly by `credential.NewService` for both destination stream keys
  and connected-account OAuth token bundles - confirmed by reading
  `store.go` in full. `KeyringStore` (the only production
  implementation today) is constructed once in `main.go`
  (`secrets.NewKeyringStore()`) with no mode branching at all. No
  caller anywhere assumes a specific backend - they only depend on the
  `SecretStore` interface. This means a new backend can be added and
  selected purely in `main.go`'s construction site, with **zero changes
  to any consumer**.
- **Does the server assume a desktop D-Bus session outside secrets?**
  Audited `internal/runtime/branch`, `internal/runtime/mediamtx`,
  `internal/runtime/ffmpeg`, `internal/provider/tts` - none of them
  touch D-Bus, `DISPLAY`, or `WAYLAND_DISPLAY` at all. The *only* D-Bus
  dependency anywhere in the backend is `keyring.SecretServiceBackend`
  itself, reached only when `KeyringStore` is actually constructed and
  only when a secret operation is actually attempted - confirmed by
  reading `github.com/99designs/keyring`'s own `secretservice.go`
  `init()` (already audited in Stage 20D1: "silently fail if dbus isn't
  available"). So the *existing* desktop Secret Service backend already
  degrades to `ErrUnavailable` rather than hanging when no session bus
  exists - but Stage 20D2A must not rely on that degrade-and-report
  behavior for a **service's primary secret backend**, since a service
  whose only secret path is silently broken is exactly the "looks
  healthy while every provider credential is unusable" failure mode
  §22 of the governing task warns against. Headless mode therefore
  selects a completely different backend outright, never falling
  through to `KeyringStore` at all.
- **Can the server run with no `HOME`?** `os.UserConfigDir()` on Linux
  falls back through `$XDG_CONFIG_HOME` before `$HOME` - a service unit
  that sets `STREAMING_TREE_DATA_DIR` explicitly never calls
  `os.UserConfigDir()` in the first place (the override short-circuits
  it in `resolveDataDir`), so a missing `$HOME` is a non-issue for
  headless deployments by construction.
- **Existing Linux single-instance mechanism**
  (`singleinstance_linux.go`, shipped in Stage 20D1): already resolves
  its lock path via `STREAMING_TREE_DATA_DIR` (when set) before falling
  back to `XDG_RUNTIME_DIR` or the desktop config directory - a
  headless deployment that sets `STREAMING_TREE_DATA_DIR` (as it must,
  per above) automatically gets a lock file inside its own
  `StateDirectory`, tied to that exact deployment's state, with zero
  code changes. This directly satisfies §24's requirement of a lock
  "tied to the service runtime/state identity" - already true today,
  confirmed by re-reading that file's own `lockFilePath` function.
- **MediaMTX Control API / RTMP address validation**
  (`internal/config.loadMediaMTX`): **already** calls
  `validateLoopbackAddress` unconditionally on both addresses,
  regardless of platform or mode - confirmed by direct source read.
  Stage 20D2A adds **no new MediaMTX validation code**: this invariant
  was already absolute before this milestone existed.
- **Management HTTP listener address**: `cfg.Host`/`cfg.Port` (from
  `STREAMING_TREE_HOST`/`STREAMING_TREE_PORT`) have **no loopback
  validation at all** today - defaults to `127.0.0.1` but an operator
  (or a misconfigured deployment) could override it to `0.0.0.0` with
  nothing rejecting that. **This is the one real gap Stage 20D2A must
  close**: headless mode must validate `cfg.Address()` is loopback
  before ever calling `net.Listen`, refusing startup with a clear error
  otherwise.
- **FFmpeg resolver** (`internal/runtime/ffmpeg/resolver.go`): already
  fully portable (§3G finding from this milestone's own Stage 20D1
  corrective audit) - `STREAMING_TREE_FFMPEG_PATH` override, then a
  bundled-location check (unused on every current packaged platform),
  then `PATH` search. No change needed for headless mode; the existing
  override and `PATH` mechanisms both already work for a service
  account exactly as they do for a desktop user.
- **Updater**: `internal/updater/handoff_other.go`'s `//go:build
  !windows` gate already makes every non-Windows release build (Linux
  headless included) report `StatePlatformUnsupported` with zero
  further code - confirmed in Stage 20D1's own audit and unchanged.
- **Debian package layout** (`scripts/build-release-linux.sh`, audited
  in full): produces `/usr/bin/streaming-tree-server`,
  `/usr/share/applications/*.desktop`, `/usr/share/doc/...` - no
  maintainer scripts, no systemd unit shipped yet. This milestone adds
  a systemd unit file and, if research supports it, a minimal
  provisioning helper - both to the same package (§14 below).

## 4. Primary-source research (2026-08-19)

- **systemd.service / systemd.exec / systemd.unit** (freedesktop.org,
  current): `Type=simple` (or `notify` if the binary is later taught to
  signal readiness - not attempted in this milestone), `Restart=`
  policy, `RuntimeDirectory=`/`StateDirectory=` (systemd creates and
  owns these, recursively fixing ownership to the configured
  `User=`/`DynamicUser=` on start), `UMask=`, `ExecStart=` requiring a
  fixed absolute path plus fixed arguments (no shell string).
- **systemd credentials** (`systemd.io/CREDENTIALS`, `systemd-creds(1)`,
  current): `LoadCredential=` (plain, introduced systemd 247) exposes a
  file to the service only via `$CREDENTIALS_DIRECTORY/<name>`
  (`/run/credentials/<unit>/<name>` for a system service), access
  restricted at the kernel level to the service's own user, never
  propagated as an environment variable to children.
  `LoadCredentialEncrypted=` (introduced systemd 250) additionally
  encrypts the credential at rest, using a key that can come from
  `/var/lib/systemd/credential.secret` (pure software, no TPM2
  required) and/or a TPM2 chip if present - confirmed directly from
  the official documentation that TPM2 is optional, not mandatory.
- **Debian/Ubuntu systemd version floor** (re-verified, not assumed):
  Ubuntu 22.04 LTS ships systemd 249 - supports plain `LoadCredential=`
  but **not** `LoadCredentialEncrypted=` (needs 250+). Ubuntu 24.04 LTS
  ships systemd 255 - supports both. Debian 12 (Bookworm) ships systemd
  252 - supports both. This is a real, verified version gap, not
  assumed: **Stage 20D2A's required baseline uses plain
  `LoadCredential=`** (systemd 247+, covers Ubuntu 22.04 LTS onward and
  Debian 12 onward - the realistic Debian/Ubuntu-family server floor),
  with `LoadCredentialEncrypted=` documented as an optional, better-
  protected-at-rest enhancement path for hosts running systemd 250+
  (Ubuntu 22.10+, Ubuntu 24.04 LTS, Debian 12+).
- **systemd.exec hardening** (freedesktop.org, current): `NoNewPrivileges=yes`
  (blocks privilege escalation via setuid/setgid/capabilities, no
  functional cost to this application), `ProtectSystem=strict` (mounts
  the whole filesystem read-only except `/dev`, `/proc`, `/sys`, plus
  any explicitly declared `StateDirectory=`/`RuntimeDirectory=`, which
  remain writable) with `ProtectHome=yes` (denies `/home`, `/root`,
  `/run/user` - the service never needs any of them once
  `STREAMING_TREE_DATA_DIR` points at its own `StateDirectory`),
  `PrivateTmp=yes` (an isolated `/tmp` - `internal/runtime/mediamtx`'s
  own staging/extraction always happens under `cfg.DataDir`, never
  `/tmp`, confirmed by source read, so this is free), `UMask=0077`.
  `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6` - deliberately
  keeps `AF_INET`/`AF_INET6` (the application needs real loopback TCP
  sockets and outbound HTTPS to providers) and `AF_UNIX` (D-Bus is not
  used in headless mode at all, but some Go runtime/libc paths use
  `AF_UNIX` internally on Linux; excluding it has caused startup
  failures in other Go services in community reports, so it stays
  allowed rather than guessed away). `RestrictSUIDSGID=yes`,
  `CapabilityBoundingSet=` (empty - the application needs no Linux
  capability at all: it never binds a privileged port, since loopback
  management/RTMP/API ports are all >1024). `PrivateNetwork=true` is
  explicitly **not** used - the application legitimately needs real
  network access (outbound provider HTTPS/WebSocket, loopback
  listeners).
- **`systemd-analyze verify`** (current): the documented static
  correctness checker for a unit file - used in this milestone's own
  CI (§17) as the primary automated proof the unit is syntactically and
  structurally valid, since (per the runner reality-check below) full
  service lifecycle cannot always be exercised.
- **AEAD cryptography** (Go standard library `crypto/aes` +
  `crypto/cipher`, NIST SP 800-38D): AES-256-GCM is a NIST-standardized,
  widely-audited authenticated encryption mode, available in Go's
  standard library with no new third-party dependency. Selected over a
  passphrase-based KDF design because the systemd-credential-provisioned
  master key is already raw, high-entropy 256-bit material - inventing
  a password-derivation step would add complexity and a weaker link
  (a human-memorable passphrase has far less entropy than 256 random
  bits) without a real corresponding benefit for this deployment model.
- **GitHub-hosted Ubuntu runners and systemd** (re-verified during this
  milestone, not assumed): `ubuntu-latest`/`ubuntu-24.04-arm` remain the
  correct current labels (unchanged since Stage 20D1's own research).
  Whether PID 1 is genuinely `systemd` on these runners is verified
  empirically in this milestone's own CI job (§17) via `ps -p 1`,
  `systemctl is-system-running`, and `systemd-analyze --version` -
  never assumed from the presence of `systemctl` on `PATH` alone.

## 5. Explicit headless mode

A new `--headless` boolean flag, parsed once in the existing
`handleEarlyFlags`/`flag.Parse()` call alongside `--version` and the
updater-helper flags - never inferred from `runtime.GOOS`, a missing
`DISPLAY`, or being launched by systemd. One core binary; no forked
build. Desktop packaged behavior and the ordinary `go run` development
workflow are both completely unchanged when `--headless` is absent.
When present:

- automatic browser launch never happens, on either the primary-launch
  or second-instance-detected code path;
- `nativealert.ShowFatalError` (which would try `zenity`/`kdialog` or
  fall back to stderr) is never called - a headless fatal error is a
  structured `slog` error line to stderr (captured by journald under a
  real systemd unit) followed by a nonzero exit, nothing more;
- the loopback-only bind validation in §6 below is enforced;
- the headless `SecretStore` backend (§8) is selected instead of
  `KeyringStore`;
- `--version` is completely unaffected (it returns before any of this
  is reached, exactly as today).

An unrecognized flag (including a misspelled one) fails via Go's own
`flag` package's existing error-and-exit behavior - unchanged, and
already clear.

## 6. Headless loopback bind validation

`internal/config.loadMediaMTX` already unconditionally validates both
MediaMTX addresses as loopback (§3 finding) - no change there. The one
real gap is the management HTTP listener. A new exported function,
`config.ValidateHeadlessListenAddress(cfg Config) error`, reuses the
exact same (already well-tested) loopback-checking logic the MediaMTX
validation already uses, applied to `cfg.Address()`. `main.go`'s
`run()` calls it immediately after `config.Load()` succeeds, only when
`--headless` was given, before any listener of any kind is created -
rejecting `0.0.0.0`, `::`, a specific LAN address, or a specific public
address with the same clear, typed startup error every other
`config.Load()` failure already produces (never silently reinterpreted
as loopback). Desktop/dev builds are unaffected - this validation only
ever runs in headless mode.

## 7. Service identity: DynamicUser + StateDirectory

Evaluated a fixed system user (created/removed via `.deb` maintainer
scripts) against systemd's `DynamicUser=yes`. Selected **`DynamicUser=yes`
with `StateDirectory=streaming-tree`**: per current systemd
documentation, `StateDirectory=` under `DynamicUser=` is created below
`/var/lib/private/`, with systemd itself transparently remapping the
recycled dynamic UID back onto the same directory's ownership on every
start - so persistence across restarts is a documented, systemd-
maintained property, not something this project has to solve. This
avoids maintainer-script user creation/removal entirely (no `useradd`/
`deluser` in the package, no UID collision risk, no orphaned system
user left behind by an uninstalled package) - a materially simpler and
more auditable package than a fixed-UID alternative, and consistent
with "no privileged helper" and "minimal, fixed, auditable" maintainer
scripts. The desktop packaged application and the headless unit both
ship in the *same* `.deb` (§14) but never share a runtime identity: the
desktop app always runs as the interactive desktop user; the headless
unit always runs as its own dynamic service identity. Neither the
application process nor the MediaMTX/FFmpeg children it spawns ever run
as root, and no capability is retained (`CapabilityBoundingSet=` empty,
§4).

## 8. Secure headless secret storage

Desktop Linux keeps using `KeyringStore`/Secret Service unchanged - no
migration, no shared code path with the new backend beyond the common
`SecretStore` interface both satisfy. A new
`internal/secrets/headlessstore.go` implements a second `SecretStore`:

- **Format**: a single JSON file (default
  `<StateDirectory>/secrets.json`) holding `{"format": "streaming-tree-
  headless-secrets", "version": 1, "entries": {"<key>":
  "<base64(nonce||ciphertext)>"}}`. One AEAD instance
  (`crypto/cipher.NewGCM` over `crypto/aes.NewCipher` with the 256-bit
  master key) serves every entry. Each `Set` generates a fresh random
  12-byte nonce (`crypto/rand`) and seals with the entry's own key
  string as AEAD associated data - binding each ciphertext to the exact
  key it is stored under, so swapping ciphertext between two entries
  (even with a valid master key) fails authentication rather than
  silently succeeding.
- **Whole-file rewrite under a mutex + real `flock`**: every `Set`/
  `Delete` re-reads, mutates, and atomically rewrites the whole file
  (write to a temp file in the same directory, `fsync`, rename) while
  holding both an in-process `sync.Mutex` and a real `flock(2)` on the
  file for the duration - safe for concurrent callers within the one
  process this application always is, and safe against a second
  process (a stale/orphaned instance) touching the same file
  concurrently.
- **Master key provenance**: read once, at process start, from
  `$CREDENTIALS_DIRECTORY/streaming-tree-master-key` (populated by the
  unit's `LoadCredential=streaming-tree-master-key:<path>` directive,
  §4/§9) - **never** from an `Environment=` value, a command-line flag,
  or a config file the application itself parses as a secret. The key
  file itself, wherever the operator places it for systemd to load, is
  exactly 32 raw bytes; anything else (wrong length, unreadable, or the
  credential absent entirely) is a startup error, not a silent
  fallback.
- **Corruption/tamper handling**: malformed JSON, a wrong `format`/
  `version` field, a nonce of the wrong length, or a GCM authentication
  failure (wrong key, corrupted ciphertext, or tampered file) all
  return `secrets.ErrFailure` - the existing sentinel this codebase's
  own doc comment already describes as "an unexpected failure from a
  reachable store," reused rather than inventing a parallel error
  vocabulary the rest of the codebase (`credential.Service`, the HTTP
  layer's existing error mapping) does not know about. A missing master
  credential at construction time returns `secrets.ErrUnavailable` -
  the same sentinel `KeyringStore` already returns for "no usable
  backend" - so the existing `credential_store_failure`/
  `credential_store_unavailable` HTTP-layer handling needs **zero
  changes** to support the new backend.
- **Plaintext handling**: decrypted values are returned to the caller
  (exactly like `KeyringStore.Get` already does) and never separately
  logged, written to a temp file, or cached beyond the caller's own use
  - the store itself holds no plaintext at rest, ever, and the process
  memory holding a decrypted value is released back to the Go garbage
  collector as soon as the caller is done with it (Go provides no
  guaranteed memory-scrubbing primitive; this matches the same honest
  limitation every other secret-handling code in this codebase already
  accepts, not a new gap).

## 9. Master-key provisioning flow

Required baseline (systemd 247+, §4): the operator generates a random
32-byte key file once (a small provisioning helper, `scripts/
provision-headless-master-key.sh`, wraps `head -c32 /dev/urandom` with
a fixed, no-shell-interpolation write, atomic rename, and `chmod 0600`)
at an operator-chosen path outside any package-owned directory (e.g.
`/etc/streaming-tree/master.key`), then references that path from the
unit's `LoadCredential=` directive before ever starting the service.
No master key is generated automatically by package installation, and
none is ever printed to a persistent log. Documented explicitly:

- **First provisioning**: run the helper once, note the key file path,
  reference it from the unit (or a systemd drop-in), `daemon-reload`,
  then start the service.
- **Backup**: back up the key file itself (outside the package, outside
  `StateDirectory`) *and* the `StateDirectory`'s `secrets.json`
  separately - both are required together to recover provider secrets;
  documented as an explicit, non-optional pairing.
- **Recovery**: restoring only `secrets.json` without the matching key
  file (or vice versa) leaves every entry permanently undecryptable -
  **stated plainly: losing the master key makes existing encrypted
  provider secrets unrecoverable, by design** (the whole point of
  authenticated encryption is that there is no back door); the operator
  must re-provision each provider's credentials from scratch through
  the normal application UI/API if this happens.
- **Deliberate reset**: delete `secrets.json` (or generate a new master
  key and start from an empty store) - the application's own existing
  "credential unavailable" UX already handles a provider whose secret
  is missing; no special reset tooling is required.

An optional, documented (not required) enhancement path notes that
`LoadCredentialEncrypted=` can wrap the *provisioning* of that same
32-byte file for at-rest protection of the key file itself on systemd
250+/Ubuntu 24.04-class hosts - a defense-in-depth layer around key
delivery, not a replacement for the AES-256-GCM envelope protecting the
provider secrets themselves.

## 10. Backend selection is mode-driven, not GOOS-driven

`main.go`'s single `secrets.NewKeyringStore()` construction site becomes
a small conditional: `--headless` selects the new
`secrets.NewHeadlessStore(...)`; its absence keeps
`secrets.NewKeyringStore()` exactly as today, on every platform - a
normal Linux desktop package continues using Secret Service
unconditionally, even though the binary is capable of headless mode.
Tests prove both branches select the intended concrete type without
starting a real service.

## 11. No secret-store migration in 20D2A

Desktop Secret Service contents are never automatically migrated into
the new headless store - the two modes may run on entirely different
machines and under different user identities, and a plaintext export/
import path is explicitly not built. A future encrypted export/import
format is left as documented future work, not attempted here.

## 12. Headless data/runtime layout

`StateDirectory=streaming-tree` (systemd-managed, `/var/lib/private/
streaming-tree` under `DynamicUser=`) holds everything `cfg.DataDir`
already resolves to on every platform: the SQLite database, managed
visual/audio assets, the managed MediaMTX installation, and
`secrets.json`. `RuntimeDirectory=streaming-tree`
(`/run/streaming-tree`) is available for any genuinely transient
artifact, though the existing single-instance lock already prefers
`STREAMING_TREE_DATA_DIR` over `XDG_RUNTIME_DIR` when the former is set
(§3), so in practice the lock file also lives inside `StateDirectory`
for a headless deployment - one location to back up, not two. Nothing
under `/usr` (package-owned) is ever written to at runtime, on any
platform, confirmed unchanged by this milestone's own audit.

## 13. Headless startup contract: fail closed

Per §22 of the governing task: a headless service whose mandatory
secret backend cannot be initialized must not report itself healthy
while every configured provider credential is silently unusable.
**Decision: startup fails completely** if `--headless` is given and the
master credential cannot be loaded (missing, wrong length, or the
`CREDENTIALS_DIRECTORY` mechanism itself unavailable) - the same "fail
loudly at startup" philosophy `config.Load()` already applies to every
other structurally-unusable configuration. This is a deliberate
divergence from the desktop `KeyringStore` path (which *does* start
successfully and reports `ErrUnavailable` lazily per-operation, because
an interactive desktop user can reasonably unlock a keychain after
launch) - a headless service has no equivalent "unlock later" moment,
so a broken secret backend at startup is treated as a genuine
configuration error, not a degraded-but-running state.

## 14. Debian package / systemd integration

The existing `streaming-tree-for-obs` `.deb` is extended (not
duplicated) with:

```
/usr/bin/streaming-tree-server                                    (unchanged)
/usr/share/applications/streaming-tree-for-obs.desktop             (unchanged)
/usr/share/doc/streaming-tree-for-obs/...                          (unchanged)
/lib/systemd/system/streaming-tree.service                         (new)
/usr/share/streaming-tree/provision-headless-master-key.sh         (new)
```

No maintainer scripts are added: `dpkg` itself installs the unit file
inert - not enabled, not started, exactly matching "installation does
not silently enable or start the service." An operator who wants the
headless service explicitly runs `systemctl daemon-reload` (required
after any new unit file appears, per systemd's own documented
behavior) and then `systemctl enable --now streaming-tree.service`
themselves, after provisioning the master key (§9). Ordinary package
removal (`dpkg -r`) never touches `StateDirectory` (systemd, not
`dpkg`, owns that path's lifecycle) and never deletes the operator's
separately-provisioned master key file (outside any package-owned
path) - both survive a reinstall. A future explicit `--purge`-style
policy for deliberately destroying `StateDirectory` is left
undocumented/unimplemented in 20D2A, since no maintainer scripts exist
yet to hook it into.

## 15. Service enablement UX (documented operator sequence)

1. `sudo dpkg -i StreamingTreeForOBS-<version>-linux-<arch>.deb`
2. `sudo /usr/share/streaming-tree/provision-headless-master-key.sh
   /etc/streaming-tree/master.key`
3. Create a systemd drop-in (or edit a local copy of the unit) pointing
   `LoadCredential=` at that path, if the shipped unit's own default
   path does not already match.
4. `sudo systemctl daemon-reload`
5. `sudo systemctl enable --now streaming-tree.service`
6. `systemctl status streaming-tree.service` / `journalctl -u
   streaming-tree.service -f`
7. `sudo systemctl disable --now streaming-tree.service` to stop.
8. Back up `/var/lib/private/streaming-tree` and the master key file
   together (§9).
9. `sudo dpkg -r streaming-tree-for-obs` removes the package without
   destroying `StateDirectory` or the master key file.

Every command above reflects the actual selected design and is
exercised for real in this milestone's own native CI (§17) as far as
the runner reality-check (§16) allows.

## 16. Native CI reality check

Directly inspected the real GitHub-hosted Ubuntu runner in this
milestone's own CI job before designing the test strategy: `ps -p 1`,
`systemctl is-system-running`, and `systemd-analyze --version` are run
first, and their real output (not an assumption) determines what is
claimed. If PID 1 is genuinely `systemd` and the system reports a
started/running-degraded state, the real service lifecycle
(`daemon-reload`, `enable --now`, `status`, real HTTP/API verification
against the running service, `disable --now`, log inspection) is
exercised for real and reported as native-CI-verified. If it is not
(GitHub's own hosted-runner images have historically not always run
systemd as PID 1 inside the execution environment), the milestone
instead relies on `systemd-analyze verify` for static unit correctness
plus separate process-level headless-mode tests (flag parsing, bind
validation, secret-store behavior, graceful `SIGTERM` shutdown) that do
not require PID-1 service management - and states plainly, wherever
this happens, that full systemd service-lifecycle management was not
exercised natively, rather than presenting `systemd-analyze verify`
output as if it were live lifecycle proof.

**Confirmed finding (2026-08-19, both architectures):** on the current
`ubuntu-latest` and `ubuntu-24.04-arm` GitHub-hosted runners, PID 1
genuinely is `systemd`, and the real service lifecycle path above
(`daemon-reload`/`enable --now`/a real `systemctl status ... active
(running)` assertion/`disable --now`) executed and passed for real -
not merely `systemd-analyze verify`. This is stronger evidence than
this section originally expected to be able to claim; it was
discovered empirically (a real CI failure investigation surfaced it,
not a deliberate test of the fallback path) rather than assumed at
contract-writing time - see the closing entry in `docs/progress.md`
for the exact run/commit evidence.

## 17. Native headless CI workflow

`.github/workflows/linux-headless.yml` - `contents: read` only, no
secrets, no artifact publication, no GitHub Release, no Git tag.
Matrix: `ubuntu-latest` (x64), `ubuntu-24.04-arm` (ARM64) - the same
current labels already used elsewhere. Triggers: `workflow_dispatch`
and `push` to `main` with path filtering over the shared Go core, the
new headless-specific Go files, `scripts/build-release-linux.sh` (the
unit file and provisioning helper are added to its output), the new
verification helper, and the workflow file itself.

## 18. Headless verification helper

`scripts/verify-linux-headless.mjs` - a platform-specific CI
verification helper, **not** canonical integration script #25. The
canonical local/Windows count remains **24**. Verification surfaces are
now documented as: 24 canonical local integration scripts + the macOS
package CI helper + the Linux desktop-package CI helper + the new Linux
headless-service CI helper. Exercises (to the extent §16 allows): flag
parsing (`--headless` accepted, desktop behavior unchanged without it),
no browser-launch/no `zenity`/`kdialog` invocation in headless mode,
management-listener loopback rejection (`0.0.0.0`, `::`, a LAN
address), MediaMTX RTMP/API remaining loopback, the real headless
`SecretStore` (write/read/restart-persistence/wrong-key-rejection/
tamper-rejection/missing-credential-failure, all against a real file on
disk, never a mock), real `SIGTERM` graceful shutdown, `systemd-analyze
verify` against the shipped unit, and - when genuine systemd PID-1
lifecycle is available - the real enable/start/status/stop/disable
sequence with a real socket check proving no listener is reachable from
anywhere but loopback. Socket state is inspected with real platform
tools (e.g. `ss`/`/proc/net/tcp*`), never trusted from configuration
values alone.

## 19. Explicit 20D2B/20D2C exclusions

Not implemented, not started, and not to be inferred as "coming next
inside this same package" from anything in this document: authentication,
session cookies, CSRF protection, trusted-proxy/reverse-proxy header
handling, TLS/HTTPS termination, a remote-safe shutdown model, remote
overlay exposure or `publicSlug` redesign, remote/authenticated RTMP
ingest, RTMPS, SRT, external JWT auth, or a Linux in-app updater install
path (Linux remains `platform_unsupported`, unchanged - no `apt`/`dpkg`/
`sudo`/`pkexec` is ever invoked by the running application itself, in
any mode). An operator who reaches the loopback UI today via their own
independently-managed SSH port forward is using external transport
they set up themselves - not a Stage 20D2A product guarantee, and
nothing in this milestone is designed around that workflow.
