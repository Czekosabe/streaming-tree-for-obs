# Stage 20B — application updater contract

**Research date:** 2026-08-18. Written before any Stage 20B product code, per
this project's established discipline (see
[`windows-packaging.md`](windows-packaging.md), written the same way for
Stage 20A).

This document is the canonical design contract for Streaming Tree's
application-update system. It builds directly on
[`windows-packaging.md`](windows-packaging.md) (the packaged Windows
runtime Stage 20A already shipped) and
[`platform-support.md`](platform-support.md) (the cross-platform artifact
model this updater's data shapes must already be compatible with, even
though only Windows x64 is wired up today).

## 1. Release source (fixed, not configurable)

The only production update source is the canonical repository:

- owner: `Czekosabe`
- repository: `streaming-tree-for-obs`
- URL: `https://github.com/Czekosabe/streaming-tree-for-obs`

This is a Go constant, not a setting. There is no production environment
variable, Settings field, or API parameter that can redirect updater
traffic to a different host, repository, or URL — see §15 for why this is
a hard security boundary, not an oversight. Test builds inject a fake
GitHub API through a constructor parameter gated by the `integration`
build tag (the same convention `apps/server/cmd/testserver` already uses
for every other test-only capability), never through a production
environment variable.

## 2. Current official GitHub Releases API research (2026-08-18)

Verified against `docs.github.com`'s current REST API reference and a live
call against a real public repository (`cli/cli`), not memory:

- **Endpoint:** `GET /repos/{owner}/{repo}/releases/latest`. GitHub's own
  documentation states this returns "the most recent non-prerelease,
  non-draft release, sorted by the `created_at` attribute" — draft and
  prerelease releases are excluded automatically by the endpoint itself,
  and ranking is by creation timestamp, not semantic-version comparison.
  This project additionally re-validates `draft`/`prerelease` client-side
  (never trusts the endpoint's filtering alone) and performs its own exact
  semver comparison against the installed version (§9) rather than relying
  on "most recent" meaning "highest version" for our own purposes.
- **Release object fields used:** `id`, `tag_name`, `name`, `draft`,
  `prerelease`, `body`, `published_at`, `assets[]`.
- **Release asset object fields used:** `id`, `name`, `size`,
  `browser_download_url`, `content_type`, `digest`, `url` (the API's own
  asset resource URL, `.../releases/assets/{id}`).
- **Asset digest field:** confirmed live against a real release
  (`cli/cli` `v2.97.0`) — a present asset carries
  `"digest": "sha256:<64 lowercase hex chars>"`. This is a real, current,
  documented field. §14 defines exactly how this project uses it: as an
  **additional cross-check only**, never as the sole integrity source,
  and never security-critical on its own — see that section for why.
- **Accept header:** `application/vnd.github+json` (current recommended
  value).
- **API version header:** `X-GitHub-Api-Version`, current value
  `2026-03-10` at time of writing. The exact value is not hard-coded logic
  — it is sent as a plain header constant, reviewed whenever this contract
  is next revisited, not something the running application ever needs to
  discover dynamically.
- **User-Agent:** GitHub's REST API requires *some* User-Agent header on
  every request (empirically confirmed: a request with no explicit
  User-Agent still succeeded only because `curl` itself always sends a
  default one — Go's `net/http` client similarly sends a default, but this
  project sends an explicit, descriptive one instead of relying on that
  default). Sent value: `StreamingTreeForOBS/<installed-version>
  (+https://github.com/Czekosabe/streaming-tree-for-obs)`.
- **Unauthenticated rate limit:** 60 requests/hour per current official
  documentation, confirmed live via response headers `x-ratelimit-limit:
  60`. Response headers actually present: `x-ratelimit-limit`,
  `x-ratelimit-remaining`, `x-ratelimit-used`, `x-ratelimit-resource`,
  `x-ratelimit-reset`. No OAuth token or PAT is ever used (§15) — the
  updater is always an unauthenticated client, so 60/hour is the real
  operating ceiling and the check schedule (§17) is designed to stay far
  under it (at most ~24-25 checks/day per running instance, one per hour
  while the app is open, plus the rare manual check).
- **ETag/conditional requests:** confirmed live — a real response carries
  a real `ETag` header. GitHub's REST API documents standard HTTP
  conditional-request support; a subsequent request sending
  `If-None-Match: <etag>` receives `304 Not Modified` when nothing
  changed, which this project treats as a normal, successful "no change"
  outcome (§16), never as an error and never as consuming meaningful
  quota beyond the request itself.
- **Redirect behavior for asset downloads:** the `browser_download_url`
  redirects to time-limited, signed storage (not documented as a stable
  permanent URL) — the updater always re-resolves the current release's
  asset list immediately before downloading rather than caching a
  download URL across sessions (§11).

## 3. Stable channel only

Stage 20B implements exactly one channel: **Stable**. No Beta, Nightly,
Canary, Dev, or custom channel exists. A release is eligible for
production update only if the canonical GitHub Release backing it is
`draft == false` and `prerelease == false`, and its own release manifest
(§11) declares `"channel": "stable"`.

## 4. Version format and comparison

Versions are `major.minor.patch` (no pre-release/build-metadata suffix
accepted for Stage 20B Stable). Canonical Git tag format is `v<version>`
(e.g. tag `v0.2.0` for application version `0.2.0`) — this mirrors the
project's already-established version constant
(`internal/buildinfo.Version = "0.1.0"`) without inventing a public 1.0
release; `0.1.0`-style versioning continues.

Normalization and comparison rules, enforced by a small internal
`semver`-style parser (not a third-party dependency — the surface needed
is tiny: parse exactly `\d+\.\d+\.\d+`, optionally strip a leading `v`
from a tag before comparing to the manifest's own unprefixed version
field):

- `v0.2.0` (tag) and `0.2.0` (manifest/application version) are the same
  version once the leading `v` is stripped from the tag.
- Rejected as malformed, not silently accepted: `latest`, `master`,
  `main`, `0.2`, `0.2.0.0`, `v0.2`, `1.0-beta`, `1.0.0-rc1`, empty string,
  anything with non-numeric components.
- Comparison is exact integer comparison of `(major, minor, patch)`
  tuples in that order — never a string/lexicographic comparison (which
  would wrongly rank `"0.10.0" < "0.9.0"`).
- If installed version `==` latest stable version: **up to date**.
- If installed version `>` latest stable version (a downgrade scenario):
  **no update offered** — never a downgrade path in Stage 20B.
- If the fetched release's own tag or manifest version fails to parse:
  the whole release is rejected as invalid metadata (logged, surfaced as
  a nonfatal "check failed" state), never partially trusted.

## 5. Release manifest

Each Stable GitHub Release includes one additional release asset: a
project-controlled JSON manifest, named exactly **`streaming-tree-
release.json`** (a fixed, hard-coded expected asset name — never
discovered by pattern-matching).

Schema (versioned, strict):

```json
{
  "format": "streaming-tree-release",
  "schemaVersion": 1,
  "version": "0.2.0",
  "channel": "stable",
  "artifacts": [
    {
      "os": "windows",
      "arch": "amd64",
      "kind": "installer",
      "name": "StreamingTreeForOBS-Setup-0.2.0.exe",
      "sizeBytes": 12345678,
      "sha256": "<64 lowercase hex characters>"
    }
  ]
}
```

Deliberately **no download URL field anywhere in this manifest.** The
manifest only asserts identity and integrity (exact asset name, size,
SHA-256); the actual downloadable location always comes from matching
that asset name against the assets array of the **same** canonical
GitHub Release response fetched moments earlier (§6) — a release
manifest can never itself become an arbitrary-redirect vector, because it
carries no URL to redirect to.

Strict validation (implemented once, in a pure Go package with no I/O, so
the exact same validator runs both at release-build time — §20 — and at
runtime):

- Unknown top-level or per-artifact fields: **rejected**.
- `format` must equal `"streaming-tree-release"` exactly.
- `schemaVersion` must equal the one integer this build understands (`1`
  for Stage 20B); a higher unknown version is rejected as "not yet
  understood," never guessed at.
- `version` must parse per §4 and must exactly equal the tag's own
  version (after `v`-stripping) — a mismatch is rejected outright, never
  "trust whichever one looks more specific."
- `channel` must equal `"stable"` exactly.
- `artifacts` must be non-empty.
- Duplicate artifact identity (same `os`+`arch`+`kind`) across the array:
  rejected.
- Duplicate artifact `name` across the array: rejected.
- `os` must be one of `windows`/`darwin`/`linux` (§7's closed enum) —
  anything else rejected.
- `arch` must be one of `amd64`/`arm64` — anything else rejected.
- `kind` must be one of the closed set defined in §7 — anything else
  rejected.
- `name` must be a non-empty plain filename (no path separators, no `..`,
  no leading `/` or drive letter) — this becomes part of a filesystem
  path later (§21) and is never trusted blindly.
- `sizeBytes` must be a positive integer, and — for the artifact this
  build actually intends to install — within the hard bound in §12.
- `sha256` must be exactly 64 lowercase hexadecimal characters.

## 6. Release-asset matching

A manifest artifact is matched against an asset in the **same** GitHub
Release response by exact `name` equality only — never substring, never
case-insensitive, never "first `.exe` found," never an asset from a
different release, never an externally-hosted URL, never a link taken
from the release body. On a match, this project prefers binding the
actual download to the asset's own API resource URL (`asset.url`, which
resolves through `Accept: application/octet-stream` per GitHub's
documented asset-download convention) over `browser_download_url` where
both are available, since the API URL is the more explicitly
API-versioned, less redirect-dependent path — but either is acceptable
as long as it originates from this exact release response, never a
cached or reconstructed URL from an earlier check. Exactly one matching
asset is required; zero or more-than-one is treated as invalid release
metadata (nonfatal check failure), never "pick the first."

## 7. Cross-platform artifact identity

Per `platform-support.md` §15, the artifact model is generic from day
one, even though only one concrete combination is wired up to actually
install anything in Stage 20B:

```go
type ArtifactIdentity struct {
    OS   OS   // "windows" | "darwin" | "linux"
    Arch Arch // "amd64" | "arm64"
    Kind Kind // "installer" | "dmg" | "pkg" | "appimage" | "deb" | "rpm"
}
```

`Kind`'s enum already lists the package kinds `platform-support.md`
names as future candidates, even though only `installer` (Windows) is
ever selected or installable in Stage 20B — adding a real macOS/Linux
`Kind` value later (20C/20D) requires no change to manifest parsing,
release-asset matching, or the update-manager state machine, only a new
platform-specific install-handoff implementation behind the same
narrow interface the Windows handoff already implements (§21).

The running application supplies its own identity once, at manager
construction, from `runtime.GOOS`/`runtime.GOARCH` plus a fixed
`KindInstaller` constant for the only platform Stage 20B actually
installs on:

```go
CurrentIdentity() ArtifactIdentity // { OS: "windows", Arch: "amd64", Kind: "installer" } today
```

On any OS/architecture Stage 20B does not yet support installing on, the
manager still performs metadata checks (§17) and can report "update
available," but install/download is reported as unavailable with an
honest reason — never silently offering a Windows installer to a
hypothetical future macOS build, and never crashing because no matching
artifact exists for the current platform.

## 8. Network client

One bounded, shared updater HTTP client (`net/http.Client` with an
explicit timeout, no cookie jar, no redirect-following surprises beyond
the standard library's own bounded default). Every production request:

- uses HTTPS only (`https://api.github.com`, `https://github.com` for
  the resolved download);
- carries an explicit per-request timeout (metadata checks: short,
  single-digit seconds; the installer download: a longer bound, but the
  transfer itself is still governed by the byte-count/size bound in §12,
  not by an unbounded wall-clock timeout alone);
- sends `Accept: application/vnd.github+json`,
  `X-GitHub-Api-Version: 2026-03-10`, and the descriptive `User-Agent`
  from §2;
- sends no `Authorization` header — no PAT, no OAuth token, ever;
- sends no Streaming Tree secret, stream key, chat content, OS username,
  machine name, streamer name, destination metadata, or any
  installation/analytics identifier of any kind. The User-Agent's own
  version number is the only "identifying" data point, and it identifies
  the *software*, not the *person or machine*.
- bounds every response body read (metadata responses: a small fixed
  cap, generously larger than any real GitHub release JSON payload but
  far below anything resembling abuse; the installer download: streamed
  and bounded per §12, never buffered whole in memory);
- never logs a full untrusted response body — only a short, already-
  sanitized status line (HTTP status code, byte count, elapsed time).

## 9. Rate limits and ETag

The manager keeps the most recent `ETag` from a successful `/releases/
latest` response in runtime memory only (never persisted — a fresh
process starts with no cached ETag and simply performs one full
request). Every later automatic or manual check sends
`If-None-Match: <etag>` when one is held. A `304 Not Modified` response
is treated as **successful, no change** — `lastSuccessfulCheckAt` still
advances, no error state is shown. A rate-limited response
(`403`/`429` with rate-limit headers present) is treated as a **nonfatal
check failure**: the manager surfaces a bounded, sanitized "check failed,
try again later" state internally, schedules the next attempt no sooner
than its normal cadence (§17) — never a tight retry loop, never hammering
the API on failure.

## 10. Automatic check schedule

Persisted preference `Automatically check for updates`, default **true**
(§27's persistence pattern). In a packaged **release** build
(`buildinfo.IsReleaseBuild() == true`) with the preference enabled:

- one metadata check a short, randomized/jittered delay after successful
  startup (on the order of tens of seconds, never immediately racing
  startup itself);
- then approximately every 60 minutes while the process keeps running,
  with a small bounded jitter (a few minutes either side) so many
  installations do not all poll GitHub on the exact same wall-clock
  minute.

Automatic checking is **metadata only** — it fetches and validates
release metadata and nothing else. It never downloads an installer,
never installs anything, never stops a stream, never stops OBS ingest,
never restarts the application. A failed automatic check never affects
streaming, never crashes the process, and never blocks any other part of
the application; it is logged at a low severity and simply retried on
the normal schedule.

In a **development build** (`IsReleaseBuild() == false`), no automatic
check ever fires, regardless of the persisted preference — see §24.
Disabling the preference stops automatic checks entirely; **manual**
"Check for updates" remains available either way, in release builds
only.

## 11. Manual check and the update-manager state machine

One backend-owned `updater.Manager`, exposing a single safe status
snapshot the frontend polls (never inventing its own state). Conceptual
states: `disabled` (development build, or after being turned off outside
a release build — see §24) → `idle` → `checking` → `up_to_date` /
`available` → `downloading` → `ready_to_install` → `installing` →
`error` (a recoverable failure state that returns to `idle` on the next
successful check, not a terminal dead end).

A check already in progress is tracked with an internal mutex/flag so
repeated manual "Check for updates" clicks never spawn concurrent
requests — a click while `checking` is already true is a no-op against
the same in-flight check.

Status fields exposed (safe only — see §22 for the exact exclusion list):
`enabled`, `releaseBuild`, `currentVersion`, `autoCheck`, `state`,
`latestVersion`, `updateAvailable`, `lastSuccessfulCheckAt`,
`releaseNotes` (bounded plain text, §12), `publishedAt`,
`downloadProgress`/`totalBytes` (while downloading), `installBlocked`
(bool) + `blockerCode` (while streaming is active, §17-§18),
`lastErrorCode` (a small closed set of stable strings, never a raw
error/exception message).

## 12. Release notes

The GitHub Release `body` is untrusted display text. It is rendered as
bounded **plain text** only:

- never `dangerouslySetInnerHTML`, never any HTML execution;
- no Markdown renderer is introduced for Stage 20B — Markdown syntax
  characters display as literal text, exactly as GitHub sent them;
- no remote image, script, or embedded-URL fetch is ever triggered by
  release-note content;
- line breaks are preserved (rendered as a `white-space: pre-wrap`-style
  block, not collapsed to one line);
- length is capped (a few thousand characters is generous for a real
  release note); a truncated body shows an honest "truncated" indicator
  rather than silently cutting text with no sign anything was omitted.

## 13. Download size bounds

The manifest's `sizeBytes` for the artifact this build intends to
install must be:

- a positive integer;
- below a hard application-defined ceiling. Given the current real
  installer (a Windows NSIS/Inno build embedding the whole production
  frontend and legal documents, currently on the order of tens of
  megabytes) plus reasonable future growth, the hard ceiling is
  **300 MiB** (`300 * 1024 * 1024` bytes) — generous headroom over any
  realistic near-future installer size, documented here so a later
  change to this constant is a deliberate, reviewed decision, not a
  forgotten magic number.
- exactly equal to the real transferred size once the download
  completes — a short/truncated transfer is a failure, not "close
  enough."

If the HTTP response's `Content-Length` is present and already exceeds
the ceiling, the download **aborts before transferring any body bytes**.
If `Content-Length` is absent, the ceiling is still enforced while
streaming — the transfer aborts and the partial file is discarded the
moment the byte count crosses the bound, exactly as if the declared size
had been checked up front. The installer is always streamed straight to
disk (§14) — the whole file is never held in memory.

## 14. SHA-256 verification and the GitHub digest cross-check

SHA-256 verification is mandatory and is computed while streaming the
download to disk (a single pass, no second read of the file needed).
The computed digest must exactly equal the manifest's own `sha256`
field (§5) — lowercase hex, byte-for-byte comparison, no partial or
prefix match.

Where the **same** GitHub asset response also carries a `digest` field
(§2 — confirmed real and current, `sha256:<hex>`), this project uses it
as an **additional cross-check**, exactly as `platform-support.md`'s own
principle of never making an undocumented field security-critical would
suggest even if this field were less well-documented than it is:

- if the GitHub-reported digest is present and matches the manifest's
  own SHA-256 (and both match the actually-downloaded bytes): normal
  success, both checks agree;
- if the GitHub-reported digest is present and **disagrees** with the
  manifest's SHA-256: this is **never silently ignored** — the release
  is treated as inconsistent/untrustworthy metadata and installation is
  refused, even though this is a stricter check than GitHub's own API
  contract requires. A present, mismatching digest is a real anomaly
  worth refusing on, not a warning to click past.
- if the field is absent for a given asset (GitHub's own documentation
  allows for this on some assets), the manifest's own SHA-256 remains
  the sole, sufficient integrity source — absence is handled exactly as
  documented, never treated as suspicious on its own.

No MD5, no SHA-1, anywhere in this design. There is no "hash mismatch
warning, continue anyway" path. A mismatch means: installation
prohibited, the candidate file deleted/quarantined, the currently
running application is completely unaffected and keeps running, and a
safe, non-alarming error state (`hash_mismatch`) is surfaced.

## 15. Why the release source is not configurable

No Settings field, environment variable, or API parameter can ever
change which repository/host the updater talks to in a production
build. This is a deliberate security boundary, not an oversight: an
updater that trusted an operator-editable URL would let a compromised
web page, a malicious browser extension, or a tricked operator redirect
"Check for updates" traffic — and eventually "Install and restart" —
toward an attacker-controlled server. Fixing the source as a Go constant
closes that entire class of attack outright. Test/integration builds
inject a fake GitHub API exclusively through a compile-time constructor
parameter behind the `integration` build tag (mirroring
`apps/server/cmd/testserver`'s own existing test-only-capability
convention) — there is no code path in an ordinary `go build`/release
build that can reach a non-canonical host.

## 16. Verified download staging

Downloads land in an application-owned subtree of the existing per-user
data directory (`internal/config.Config.DataDir`, e.g.
`%AppData%\StreamingTree\updates\` on Windows) — never the install
directory, never the current working directory, never any
frontend-supplied path. A candidate is first written to `artifact.part`;
only after the write completes, the exact size matches, and SHA-256
(§14) verifies, is it atomically renamed to a verified candidate file
name that also encodes its own identity (version + a short content hash
prefix) so a stale candidate from an earlier check can never be
confused with a newer one (§18). No symlink/reparse-point target is
ever followed for the destination path, and every path component is
application-generated — no user- or frontend-supplied path segment is
ever concatenated into a filesystem path.

## 17. Active-stream guard

This is the single most safety-critical rule in this document. Backend
is authoritative; the frontend never independently decides whether an
install is safe.

**Streaming is considered active** if, for any configured destination,
`branch.Manager.Snapshot()` reports `DesiredRunning == true`, **or**
`State` is one of `StateStarting`, `StateLive`, `StateRestarting`,
`StateWaitingForIngest`, or `StateStopping` — deliberately the same
predicate `branch.Manager.StopAll` already uses internally
(`desiredRunning || proc != nil || state == StateBlocked || state ==
StateWaitingForIngest`, broadened slightly to explicitly name every
transitional state rather than relying on `proc != nil` alone), so the
updater's notion of "active" never drifts from the one the branch
runtime itself already uses to decide what needs stopping.

- **Checking metadata:** allowed while streaming.
- **Downloading and verifying:** allowed while streaming (the download
  itself touches nothing about the running application).
- **Installing:** **forbidden** while streaming. The `Install and
  restart` action is rejected outright — backend returns
  `install_blocked_streaming_active` — the moment the guard is true, and
  the frontend explains why rather than the button silently doing
  nothing. Stage 20B never stops a stream automatically and never
  implements "Stop streams and update" — the operator must stop
  streaming manually first, then retry.

## 18. Re-checking the guard at final handoff

The guard above is checked once when `Install and restart` is first
requested — but a stream could start in the gap between that check and
the moment shutdown is actually committed to (operator was idle,
package already verified, then a stream starts). Immediately **before**
committing to the shutdown/handoff sequence (i.e. inside the same
backend call that begins it, not merely at the HTTP-request boundary),
the guard is re-evaluated against the real, current runtime state. If it
is now true, the install is aborted at that point with the same
`install_blocked_streaming_active` outcome — the application keeps
running normally, nothing was shut down.

To close the remaining narrow race (a stream call arriving in the brief
window after the final re-check succeeds but before the shutdown
signal is actually sent), the manager sets a short-lived internal
"update committing" flag for the duration of that final check-then-
shutdown critical section; branch-start requests arriving while that
flag is set are rejected with a clear, honest error rather than being
silently accepted into a process that is about to exit. This is
intentionally the smallest possible mechanism to close the race — not a
general maintenance-mode feature, and it is cleared immediately if the
shutdown does not actually proceed (e.g. the re-check itself failed).

## 19. Installed-context verification

`Install and restart` is only ever enabled when the running executable
is confirmed to be an actual Inno-Setup-installed instance, not merely
"this is Windows" or merely `buildinfo.Packaged()`— a release binary can
theoretically be copied elsewhere and run standalone.

Chosen mechanism (researched via Inno Setup's own current documentation,
§20): Inno Setup **automatically** creates `unins000.exe` and
`unins000.dat` beside the installed executable for every install it
performs — no `[Registry]` or extra `[Files]` entry is needed to produce
them, and no Stage 20B installer-script change is required. At startup,
if `buildinfo.Packaged()` is true, the manager checks whether both files
exist as regular files in the same directory as `os.Executable()`. If
they do, this is treated as a genuine Inno-Setup-installed instance and
installer handoff is enabled (subject to every other gate above). If
they do not, metadata checks still work normally (there is no reason a
copied/portable binary can't tell the operator an update exists), but
`Install and restart` is disabled with an honest status
(`not_installed_context`) rather than guessing where an installer should
target. This marker is immutable product metadata Inno Setup itself
owns and contains no secret.

## 20. Inno Setup research (2026-08-18, official documentation)

- **`/VERYSILENT`**: no installation progress window; error messages and
  the small set of unsuppressible startup prompts (About Setup, disk
  insertion) still appear unless further suppressed, and a required
  reboot proceeds without prompting unless `/NORESTART` is also given.
- **`/SUPPRESSMSGBOXES`**: suppresses ordinary message boxes (applying
  default Yes/No/Abort/Cancel answers), only meaningful combined with
  `/SILENT`/`/VERYSILENT`; five specific dialog types remain
  unsuppressible regardless (none of which this project's own installer
  triggers, since it has no custom pages).
- **`/NORESTART`**: prevents any automatic system restart after a
  successful install or a "Preparing to Install" failure that would
  otherwise request one.
- **`/DIR=`**: overrides the install directory — **deliberately never
  passed** by the Stage 20B updater. Leaving it unset lets Inno Setup's
  own same-`AppId` upgrade behavior (below) preserve the existing
  install location automatically, which is exactly what an in-place
  update needs.
- **`/LOG=`**: writes a fixed-path installer log — used by the helper
  (§21) so a failed silent install leaves a real, inspectable log file
  in the update-staging subtree, never merely a bare exit code.
- **Exit codes** (official, complete list): `0` success; `1` failed to
  initialize; `2` user cancelled before install began; `3`/`4` fatal
  error preparing/during install; `5` user cancelled during install;
  `6` forcefully terminated by a debugger; `7` blocked during
  "Preparing to Install," no restart needed; `8` same, but a restart is
  required. The helper (§21) maps exactly these codes to its own small
  result vocabulary — `0` is the only success code; every other code is
  a distinct, logged failure, never collapsed into one generic
  "something went wrong."
- **`AppId` / same-install upgrade identity**: this project's fixed
  `AppId` (`{{C067013C-D143-49F8-9510-D078482D6DA4}`, unchanged from
  Stage 20A) determines the per-user uninstall registry key name
  (`{AppId}_is1`, under `HKCU\...\Uninstall` for a per-user install) and
  causes Setup to append to the existing uninstall log when the same
  `AppId` is detected — this is the mechanism that makes "run the new
  installer silently" a genuine in-place upgrade rather than a second,
  parallel install. No change to the `.iss` script's `AppId` is made or
  ever should be.
- Inno Setup does not document a way to hot-replace a running
  executable's own file while it is still open — this is exactly why an
  external handoff (§21) exists: the installer only ever runs after the
  application process that owns `streaming-tree-server.exe` has fully
  exited.

## 21. External update handoff

The running application never overwrites itself. The chosen design,
evaluated against the alternatives in §21 of the governing task (a
scheduled task, a Windows service, a Run-key startup hack, or an
ad-hoc generated shell/PowerShell script) and rejected in favor of a
small first-party helper mode of the very same executable:

1. Once `Install and restart` is confirmed and the final guard (§18)
   passes, the running application copies its **own currently-running
   executable** to the update-staging subtree (§16) as a helper copy —
   running the helper from the staging subtree, not the install
   directory, is deliberate: the installer needs to freely replace
   files inside the install directory a moment later, which it cannot
   do if a running process from that same directory still holds the
   executable open.
2. The application launches that helper copy with a small, closed,
   application-generated argument set (see §22 — never an arbitrary
   command, path, or URL) that includes: this process's own PID, the
   verified installer candidate's path, the expected post-install
   version, and the installed executable's own real path (from
   `os.Executable()`, resolved before any handoff begins).
3. The **helper's first action**, while the original application is
   still fully running, is to open a real Windows handle to that PID
   with `SYNCHRONIZE` access (`golang.org/x/sys/windows.OpenProcess`,
   already a direct module dependency since Stage 20A) — binding the
   handle to the correct process object *before* it can exit closes the
   PID-reuse race entirely; once the handle is open, this exact process
   object's termination is unambiguous no matter how long the wait
   takes or what PID gets reused afterward (§23).
4. The running application then begins its own normal, existing
   graceful-shutdown sequence (§24 — nothing new is invented here) and
   exits.
5. The helper calls `WaitForSingleObject` on that handle with a bounded
   timeout (a few minutes — generous for a graceful shutdown, not
   unbounded).
6. On confirmed exit, the helper re-verifies the staged installer's
   SHA-256 one more time (defense in depth against staging-directory
   tampering between the first verification and now) before executing
   anything.
7. The helper launches the real Inno Setup installer with
   `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=<staging>\install.log`
   (never `/DIR=`, per §20) and waits for it to exit, capturing its exit
   code.
8. On exit code `0`, the helper verifies the newly-installed
   executable's own reported version (§25) by running it with
   `--version` and parsing the existing, already-implemented output
   format (`cmd/server`'s own `--version` flag, unchanged).
9. The helper writes a small, bounded, no-secret result record (§26) to
   a fixed location in the update-staging subtree and launches the
   installed application's own real executable (not the helper copy) to
   restart it normally.
10. The helper exits. No helper process is left running.

If the parent does not exit within the bounded wait (step 5), the
helper does **not** proceed to install anything — it records a
`parent_did_not_exit` failure and does not kill the original process
itself (an operator-visible, still-running application is a safe
failure mode; a half-installed one is not).

## 22. Update helper security

Helper mode is detected **before** any normal application startup runs
(before SQLite is opened, before MediaMTX is touched, before providers
start, before the HTTP server binds, before the single-instance mutex is
acquired, before TTS initializes) — a dedicated flag check at the very
top of `main()`, mirroring how `--version` is already checked before
`run()` is ever called. Helper mode performs none of the above; it does
exactly the ten steps in §21 and nothing else.

Arguments accepted are a small, strict, positional/flag set — the
parent's own PID, the verified candidate's own path (inside the
staging subtree only — the helper independently re-validates this),
the expected installed executable path, and the expected post-install
version string. There is no "arbitrary executable to run," "arbitrary
shell command," "arbitrary URL," "arbitrary install flag," or
"arbitrary restart command" parameter — every one of those is instead a
fixed constant inside the helper's own code (the installer flags in
§21 step 7, the `--version` check in step 8), never taken from an
argument. The helper independently re-derives and re-checks every path
it uses rather than trusting the parent's arguments at face value where
that check is cheap (e.g. the candidate path must resolve inside the
known staging subtree; the target executable path must match the
installed-context marker's own directory from §19).

## 23. Parent-process wait

Implemented exactly as described in §21 step 3/5:
`windows.OpenProcess(windows.SYNCHRONIZE, false, parentPID)` called
immediately at helper startup (parent still running at that moment,
since the parent is the one that just launched the helper), followed by
`windows.WaitForSingleObject(handle, boundedTimeoutMillis)`. This is the
standard, Microsoft-documented technique for waiting on a specific
process object rather than polling `tasklist`/sleeping-and-guessing; it
is race-free because the handle is bound to the process object, not to
the numeric PID, at the moment it is opened.

## 24. Graceful shutdown reuse

The update install path never invents a second shutdown implementation.
It calls the exact same `context.CancelFunc` (`shutdownCancel` in
`cmd/server/main.go`, already reused once by `POST /api/system/shutdown`
— see `system.go`) that triggers the single, already-correct
`shutdownRuntime()` sequence: destination branches, device-flow
manager, YouTube auth manager, Twitch/YouTube/StreamElements engagement
managers, operator-chat projection, chat-overlay manager, outbound-chat
manager, chat-automation manager, alerts manager, audio manager, goals
manager, supporter-widgets manager, the Event Bus, the account
validation worker, and finally the MediaMTX supervisor — in that exact
existing order, followed by `server.Shutdown` and process exit. The
updater's own shutdown request carries an internal "reason: update"
marker (used only for the log line and, after restart, for producing an
accurate post-update result message) — it changes no ordering and no
step of the existing sequence.

## 25. Install failure model

A failure **before** the application shuts down (metadata failure,
download failure, hash mismatch, helper-preparation failure): the
existing application keeps running, completely unaffected, with a
clear error state surfaced.

A failure **after** shutdown/handoff has begun: the helper makes a best
safe effort to restart the existing, already-installed application if
its executable is still present and runnable — an update attempt must
never leave the operator with no application at all when the previous
install was never actually removed (Inno Setup replaces files in place
on success; on a failed silent install, per its own documented exit
codes in §20, it does not claim to have completed, so the previous
install's files are expected to remain intact). The helper's result
record captures which of these two categories happened, in a small
closed vocabulary (`ok`, `parent_did_not_exit`,
`reverify_failed`, `installer_failed:<exit-code>`,
`version_mismatch`, `restart_failed`), and no attempt is made to claim a
stronger guarantee (e.g. atomic rollback) than Inno Setup itself
documents providing — this boundary is stated plainly rather than
invented.

## 26. Post-update result and post-install version verification

The helper never trusts the installer's exit code `0` alone as proof
the intended version is now installed. Step 8 (§21) actually runs the
freshly-installed executable with `--version` and parses its existing
output (`buildinfo.ProductName buildinfo.EffectiveVersion()`, already
implemented, unchanged) — if the reported version does not exactly
match the version this update attempt targeted, the result is
`version_mismatch`, not `ok`, even though the installer itself reported
success.

The result record is a small JSON file in the update-staging subtree,
containing only: outcome code, from-version, to-version, a timestamp,
and (only for `installer_failed`) the numeric Inno Setup exit code —
never a secret, never a raw stdout/stderr dump (the installer's own log
file, capped in size, is kept alongside for manual inspection if ever
needed, but is not surfaced through any API). On the next normal
startup, the application reads this file once, surfaces it once (a
one-time "Streaming Tree was updated to `<version>`" or "Update could
not be installed, your existing installation is still available"
message), and deletes it — this is a one-shot transient signal, never
an accumulating history, and explicitly not analytics.

## 27. Automatic-check preference persistence

New minimal settings domain, `internal/domain/updatersettings` (or
folded into a small dedicated `internal/domain/updater` package
alongside the runtime manager's own domain types — decided at
implementation time by whichever keeps the package boundary cleanest,
documented in the implementing commit), following the exact
`operatorchatprefs`/`audio` precedent already established in this
codebase: a singleton row (`internal/storage/sqlite`, next migration
number after the current highest), a `Service.Preferences`/
`Service.ReplacePreferences` pair, `ErrStorage`/`ErrValidation`
sentinels, and a `Default()` returning `AutoCheck: true`. The preference
is configuration only — it carries no identity, no machine data, no
history.

## 28. Update HTTP API

New route group, `internal/httpapi/updater.go`, following this
codebase's existing router-registration convention exactly (method-aware
pattern + a bare-path 405 fallback, only registered when the manager
dependency is non-nil):

- `GET /api/updates/status` — the safe status snapshot from §11.
- `PUT /api/updates/preferences` — `{"autoCheck": bool}`, strict
  `decodeJSON` (unknown fields rejected, same as every other PUT in this
  codebase).
- `POST /api/updates/check` — manual check; no body.
- `POST /api/updates/download` — begins downloading the currently
  available update; no body; rejected if no update is available or one
  is already downloading.
- `POST /api/updates/install` — begins the shutdown/handoff sequence;
  requires the same strict, non-form-submittable JSON body shape the
  existing shutdown endpoint uses (`{"confirm":true}`, §29) — never a
  bare POST with no body, so an ordinary HTML `<form>` cannot trigger it
  for the same reason it cannot trigger `/api/system/shutdown` today.

No endpoint ever accepts or exposes an arbitrary URL, repository name,
asset id used as a redirect target, local filesystem path, executable
path, or shell argument from the frontend.

## 29. Reusing the shutdown endpoint's local-action protection

`POST /api/updates/install` reuses the exact same protection
`POST /api/system/shutdown` already implements (`system.go`): an Origin
allowlist check against the same `cfg.AllowedOrigins`, and a strict
`{"confirm":true}` JSON body (defeating simple cross-origin `<form>`
submission, since a form cannot submit `application/json`). Rather than
inventing a second, slightly-different security scheme, `handleShutdown`'s
Origin-check helper is factored into a small shared function both
handlers call — this is a genuine, warranted refactor (not "cleanup for
its own sake"): two independent, subtly different implementations of the
same security check is a real correctness risk, one implementation used
twice is not. `POST /api/system/shutdown` itself is **not weakened** by
this change — its behavior is byte-for-byte identical before and after,
verified by its existing tests continuing to pass unmodified.

## 30. Status API privacy

Exposed in `GET /api/updates/status`: `enabled`, `releaseBuild`,
`currentVersion`, `autoCheck`, `state`, `latestVersion`,
`updateAvailable`, `lastSuccessfulCheckAt`, bounded `releaseNotes`,
`publishedAt`, `downloadProgress`/`totalBytes`, `installBlocked` +
`blockerCode`, `lastErrorCode`. Never exposed: the update-staging
directory path, the helper executable's path, the installer's local
filesystem path, the GitHub API asset id (not user-facing, no reason to
leak internal identifiers), the resolved download URL, the SHA-256
value (nothing in the UI needs to display a hash), or any machine/OS
identity.

## 31. Frontend UX

A new "Updates" panel is added to `AboutLegalPage.tsx` (the natural
existing home — it already shows version information and hosts the
"Quit Streaming Tree" card, both process-lifecycle concerns), following
the exact `QuitApplicationCard` shape: a `Panel`/`PanelHeader`/
`PanelBody`, a `useQuery` status hook mirroring `useRuntimeQuery`'s own
state-dependent `refetchInterval` idiom (a short interval only while
actively `checking`/`downloading`/`installing`, a longer idle interval
otherwise — never a constant aggressive poll), and a `ConfirmDialog`
(the same generic component `QuitApplicationCard` already uses) for the
final "Streaming Tree will close, install the update, and restart"
confirmation, in both English and Polish.

States surfaced: current version and Stable channel label; the
automatic-check toggle; last successful check time; a
"Check for updates" button (disabled while a check is already running);
and, once an update is available: version, bounded plain-text release
notes, `Update now` / `Later`; during download: progress; once
verified: `Install and restart` (or, if blocked, the real reason —
§17 — never a silently inert button); on a development build: an honest
"Application updates are available in packaged release builds" message
instead of any of the above (§24).

## 32. Global update banner

A new, small, non-blocking banner component, mounted alongside the
existing `SystemStatusPill` (the closest existing "visible on every
page" element — `TopBar.tsx`), shown only once a stable update is
confirmed available. It never blocks streaming, never takes over the
screen, never repeatedly flashes or nags, and never plays sound. Actions:
`Update now` (navigates to the Updates panel) and `Later`. "Later" hides
the banner for that specific version for the remainder of the current
application process/session only — it is not a permanent "skip this
version forever" feature (that would be a separate, deliberate future
product decision, not implied here); the Settings panel continues to
show the update is available regardless, a manual check can surface the
banner again deliberately, and a fresh automatic check after an
application restart may show it again.

## 33. Streaming-blocker UX

When `installBlocked` is true, the UI shows the real reason in plain
language ("Stop the active stream before installing the update.") — the
`Install and restart` action is visibly disabled with that explanation
attached, never silently inert. With multiple active destinations, one
bounded, generic explanation is sufficient — no destination
configuration detail is exposed through this path.

## 34. i18n

A new namespace, `updates`, added to both `apps/web/src/i18n/resources/
en/updates.json` and `.../pl/updates.json`, registered in
`apps/web/src/i18n/config.ts`'s `NAMESPACES` array — the exact existing
convention every other page-level namespace already follows. Every
user-facing updater string ships in both English and Polish before this
milestone closes; no string exists in English only.

## 35. Development-build behavior

In an ordinary `go run ./cmd/server` / `go build` (not a release build,
`IsReleaseBuild() == false`): no startup GitHub check, no hourly check,
no installer download, no helper handoff — ever, regardless of the
persisted `autoCheck` preference, which is read but has no effect unless
`IsReleaseBuild()` is also true. Settings shows the honest "Application
updates are available in packaged release builds" state. Integration
tests enable updater behavior exclusively through explicit,
`integration`-build-tag-gated test wiring that injects a fake GitHub
API and a `IsReleaseBuild`-equivalent test override — never through a
production code path.

## 36. Authenticode status (unchanged, restated honestly)

Windows release artifacts remain **unsigned** — this is unchanged by
Stage 20B, and Stage 20B does not create a self-signed certificate,
does not generate a fake publisher identity, and does not commit a
signing key. The updater's real integrity boundary is: canonical HTTPS
GitHub release (fixed source, §1/§15) + strict release/asset identity
matching (§6) + the project-controlled release manifest (§5) + mandatory
SHA-256 verification (§14) + the optional documented GitHub asset digest
as an additional cross-check (§14). This is explicitly **not** presented
as equivalent to Authenticode publisher authentication — production
code signing remains pre-public-release future work, restated here in
the same honest terms `windows-packaging.md` §20 already uses.

## 37. Privacy

`PRIVACY.md` is updated (§40) to describe, precisely: what the updater
contacts (`api.github.com` and, when a download begins, GitHub's release
asset storage), what leaves the machine (an HTTPS request carrying only
the descriptive User-Agent from §2 — no stream keys, no OAuth tokens, no
chat content, no destination configuration, no machine/OS identity, no
installation ID), what is never sent (any of the above), what is stored
locally (the persisted `autoCheck` boolean preference, the transient
staged installer file and its verification state, the one-shot
post-update result record — none of it shared with any third party
beyond GitHub itself as the download source), and that this entire
system is inert unless the release build's `autoCheck` preference is on
or the operator manually checks.

## 38. Test strategy

- **Manifest schema/validator**: pure unit tests, no I/O — every
  rejection rule in §5 gets its own test case (unknown field, wrong
  schemaVersion, version/tag mismatch, non-stable channel, duplicate
  identity, duplicate name, invalid os/arch/kind, invalid size, invalid
  SHA-256 shape).
- **GitHub client**: tests run against a local `httptest.Server`
  standing in for `api.github.com` — real HTTP round-trips, fake
  responses. Covers: successful `latest` fetch and parse, draft/
  prerelease exclusion, ETag round-trip (a second request with
  `If-None-Match` receiving `304`), rate-limit response handling,
  malformed JSON handling, asset-matching rules (exact name, exactly-one
  match, digest cross-check agreement and mismatch).
- **Update manager / state machine**: unit tests with a fake GitHub
  client and a fake `branch.Manager` snapshot source — covers every
  state transition, the streaming-active guard (allowed while
  checking/downloading, blocked while installing), the final-handoff
  race guard from §18, size-bound enforcement, hash-mismatch handling,
  and the version-comparison/no-downgrade rules from §4.
- **HTTP API**: `httptest`-based handler tests mirroring this
  codebase's existing handler-test convention — status shape, PUT
  preference validation (unknown fields rejected), Origin/JSON-body
  protection on the install endpoint (reusing/extending the existing
  shutdown-endpoint tests' own approach), and confirms
  `POST /api/system/shutdown`'s own existing tests are unaffected by the
  shared-helper refactor in §29.
- **Windows helper**: the parent-process-wait and installer-invocation
  logic is exercised by a real, hermetic **integration** test — a new
  canonical script, `scripts/verify-updater.mjs` (script **24**),
  described in §41 — using a local fake GitHub API server and a real,
  locally-built release installer (never a fake stand-in for the actual
  Inno Setup binary), run in a fully isolated `STREAMING_TREE_DATA_DIR`/
  install-staging location exactly like `verify-packaged-app.mjs` and
  `verify-installer.mjs` already do.
- **Frontend**: component/hook tests for the Updates panel and the
  global banner, following this codebase's existing Vitest/Testing
  Library conventions (see `AlertsPage.test.tsx`/`PublicAlertPage.test.tsx`
  for the established pattern), including a development-build state
  test (no automatic check, honest message shown) and an
  install-blocked-while-streaming UI test.

## 39. Release-pipeline manifest generation

`scripts/build-release.ps1` is extended: after the real Inno Setup
installer is produced (step 8 of the existing script), a new step
computes the manifest artifact entry directly from that real file —
version from the same `-Version` parameter already used for
`-ldflags`, `os`/`arch`/`kind` fixed to `windows`/`amd64`/`installer`,
`name` from the real installer filename Inno Setup actually produced,
`sizeBytes` from the real file's length, `sha256` from the same
`Get-FileHash` mechanism the script already uses for its existing
`.sha256` sidecar file. The manifest is generated by a small first-party
Go tool (reusing the exact same manifest-validator package product code
uses, so the release pipeline and the runtime updater can never disagree
about what a valid manifest looks like) rather than hand-assembled
PowerShell JSON — the release script builds and invokes this tool, then
runs the same validator against its own output before declaring success,
so the pipeline fails loudly if it ever produces a manifest the runtime
updater itself would reject. `-SkipInstaller` also skips manifest
generation (there is no installer to describe). The build fails outright
if the installer is missing, the computed size is invalid, the checksum
cannot be computed, the version is inconsistent, or the generated
manifest fails validation — never a best-effort partial manifest.

Generated release artifacts (the manifest included) remain **git-ignored**,
exactly like the installer and its `.sha256` file already are — only the
generator tool and its tests are committed. This milestone does **not**
create a Git tag, does **not** create a GitHub Release, and does **not**
upload anything — it produces a correct, self-validated local manifest
ready for a future release, the same "local artifacts only" framing
`build-release.ps1`'s own doc comment already uses for the installer
itself.

## 40. Documentation updated by this milestone

`PRIVACY.md` (§37), `README.md` (roadmap row 20B: Planned → in progress
→ closed as implemented; a short "Automatic updates" mention consistent
with the existing Platform support section's own honest tone),
`docs/project-overview.md` §12.1.1 (already titled "Application update
system (Stage 20B, not yet implemented)" — retargeted to describe the
real, now-implemented system), `docs/engagement-architecture.md`'s
roadmap table (20B row: Planned → Completed), `docs/platform-support.md`
§15 (cross-references this document once it exists rather than
duplicating its content), `config/README.md` (any new
`STREAMING_TREE_*`-prefixed environment variable this milestone
introduces — expected to be none in production, per §1/§15; any
integration-only test wiring is documented there instead, matching the
existing convention for `STREAMING_TREE_TEST_NO_UI`).

## 41. Integration test script

A new canonical script, **`scripts/verify-updater.mjs`**, becomes
integration script **24** (the 23 existing scripts remain unchanged and
in their existing order; this is a genuinely new capability, exactly the
same justification Stage 20A used when it added script 23). It exercises,
against real locally-built artifacts and a real, hermetic local fake
GitHub API server: a full "old version running → check → download →
verify → blocked while streaming → unblocked once stopped → install →
helper handoff → real Inno Setup silent upgrade → restarted at the new
version → post-update result surfaced" cycle, plus the manifest-
rejection and hash-mismatch failure paths. It uses only local, isolated
paths (its own `STREAMING_TREE_DATA_DIR`, its own install staging
location) and cleans up every process and temporary directory it
creates, exactly like `verify-packaged-app.mjs` and `verify-installer.mjs`
already do — never touching a real developer install or a real GitHub
repository.

## 42. Known limitations after Stage 20B

- Windows x64 only — no macOS/Linux artifact is ever offered or
  installed, even though the data model already supports naming one
  (§7).
- No code signing (§36) — unchanged from Stage 20A.
- No Beta/Nightly/custom channel (§3) — a future, separate product
  decision if ever pursued.
- No permanent "skip this version" feature (§32) — "Later" is
  session-scoped only.
- No rollback to a previous version if an operator wants to undo a
  successful update — Stage 20B is forward-only.
- No update telemetry/analytics of any kind (§26/§37) — success/failure
  is visible to the operator once, locally, and nowhere else.
- The active-stream guard (§17-§18) blocks *installation*, not
  *download* — an operator can download and verify an update while
  streaming; this is intentional (§17) and not a limitation to fix
  later, restated here only so it is not mistaken for an oversight.
