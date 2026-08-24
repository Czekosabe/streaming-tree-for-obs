# Stage 20E — final hardening and manual verification contract

This document is written *before* any Stage 20E product code, per this
stage's own governing task. It defines what Stage 20E builds, why, and
exactly what it does not attempt — grounded in a direct audit of the
real current architecture (recorded in `docs/progress.md`), not in
assumptions carried over from earlier roadmap prose.

Stage 20E owns exactly three things: **diagnostics/logging**, **final
release hardening**, and **final manual/platform verification**. It
does not implement Stage 20C2 (Apple signing/notarization — remains
externally gated), does not publish a public release, and does not
add telemetry of any kind.

## A. Diagnostics/logging architecture

The real current logger (audited directly, `docs/progress.md`) is a
single `log/slog` instance, `slog.NewTextHandler(os.Stdout, ...)`,
identical in headless and desktop mode, with no file sink anywhere.
Headless mode's stdout already reaches journald with systemd's own
real persistence/rotation; desktop mode's stdout is not visible to an
end user at all today (no console window) — which is the actual gap
Stage 20E closes, not a missing *destination* so much as a missing
*operator-visible surface*.

Stage 20E does **not** build a second logging universe. It wraps the
existing single logger with an `slog.Handler` that delegates unchanged
to the real stdout handler (headless/journald behavior is byte-for-byte
unaffected) and additionally captures each record, after redaction,
into a bounded in-memory ring buffer. This ring buffer is what the new
Logs API and support bundle both read from.

**Deliberate scope decision, disclosed honestly**: the ring buffer is
in-memory only, not a rotating file. Rationale:
- Headless mode already has real, durable, rotated log storage via
  journald — an app-level file would duplicate infrastructure the OS
  already provides correctly.
- Desktop mode's actual gap is *visibility*, not *durability* — an
  operator troubleshooting a live problem needs *recent* events, which
  a ring buffer provides without any of file rotation's cross-platform
  path/permission/schema complexity.
- No new on-disk schema means no migration/backward-compatibility
  surface is introduced (relevant to this stage's own final
  install/upgrade/uninstall matrix).
- Trade-off: log history does not survive a process restart. This is
  an accepted limitation, not an oversight — recorded here so it is
  never silently claimed as more than it is.

Ring buffer bound: 2,000 most recent entries (a fixed, generous bound
well within single-digit MB of memory for realistic message sizes,
chosen after auditing that no existing subsystem logs at a volume
anywhere near that in normal operation).

## B. Privacy/redaction rules

Two independent, centralized redaction functions (never one-off
string replacement at call sites), in a new `internal/diagnostics`
package:

- `RedactPath(path string) string` — route-aware. Extends the
  existing (but incomplete) `redactLoggedPath` in
  `internal/httpapi/middleware.go`, which today only covers
  `/api/public/chat-overlays/{slug}`. The centralized version covers
  every path shape that can carry a capability value: chat-overlay,
  alert-profile, audio, and widget public slugs; visual-asset public
  tokens; every `/overlay/{capability}` and `/api/public/*` remote-
  overlay path; remote-overlay management routes
  (`/api/remote-overlay/{domain}/{slug}/...`). `internal/httpapi`
  calls into this one function from both `withLogging` (already
  redacted, now centralized) and `withRecovery` (found unredacted in
  the audit — a real gap this stage closes).
- `RedactText(s string) string` — a defense-in-depth value scanner for
  secret-shaped strings appearing in free-text log messages or error
  detail (long hex/base64 tokens, `?user=...&pass=...`-shaped query
  fragments in any URL text, bearer-token-shaped strings). This is a
  second line of defense; the first is that no code path today logs
  raw secret values at all (confirmed by direct audit and by the
  existing `TestAccessLogNeverContainsOAuthSecrets` regression test) —
  `RedactText` exists so a future call site that *does* interpolate an
  error containing one doesn't silently leak it.

Never logged or captured, by construction: stream keys, OAuth
access/refresh tokens, the admin password verifier, the remote-ingest
plaintext password, session cookies, CSRF tokens, remote-overlay
capability tokens, visual-asset capability tokens, the headless master
key, TLS private key material (only file *paths* ever appear in
config, never key bytes — confirmed by audit).

## C. Support-bundle model

A single deterministic ZIP, generated on explicit operator action
only (never automatic, never uploaded, never phoned home). Contents
are bounded and enumerated explicitly in `internal/diagnostics`, not
assembled by walking arbitrary state:

**Included**: `buildinfo` fields (version, commit, packaged flag),
OS/arch, headless/desktop mode, MediaMTX installed version, FFmpeg
probe/capabilities, high-level subsystem state (MediaMTX state,
remote-ingest receiving/not, branch states — never their configured
destination URLs), recent redacted log entries from the ring buffer,
sanitized configuration metadata (which features are *enabled*, never
their credential values), updater status, and the platform-support
summary from `docs/platform-support.md`'s own vocabulary.

**Excluded, absolutely**: the SQLite database, the OS credential-store
contents, the headless secrets store file, the master key, TLS private
keys, stream keys, OAuth tokens, cookies, CSRF tokens, remote-overlay/
remote-ingest/visual-asset tokens or credential-bearing URLs, chat
message history, donation personal details, and any other user file.
When in doubt about a field, it is omitted — this list is a floor, not
a ceiling, on caution.

## D. Log retention/bounds

2,000-entry ring buffer (§A). No unbounded memory or disk growth is
possible by construction — the buffer is a fixed-capacity circular
structure, never a growing slice.

## E. Remote-management authorization for diagnostics

`GET /api/logs` and the support-bundle endpoint live under the same
authenticated management surface every other Stage 20D2B route already
uses — session + the existing CSRF/Origin machinery for the bundle
generation (an unsafe, state-nothing-but-resource-creating action),
plain session auth for read-only log retrieval, matching existing
route conventions in `internal/httpapi`. Neither route is ever
registered under `/api/public/*`. Local loopback desktop use continues
to work exactly as every other local-only route already does (no
remote-management origin configured means no additional check applies,
matching the existing `withRemoteManagementSecurity`/no-op-when-
disabled pattern already established in Stage 20D2B).

## F. Startup/failure diagnostics

Audited per-platform in `docs/progress.md`. Actionable startup
failures (port in use, MediaMTX unavailable/corrupt, FFmpeg missing,
SQLite open/migration failure, credential-store unavailable, headless
master key missing, admin verifier missing, RTMPS cert/key failure,
production frontend artifact missing) are reviewed for whether the
operator is told *what* failed and *where to look next* — never a
secret in the message. Desktop native alerts stay concise; full detail
belongs in diagnostics/logs. Headless failures remain visible through
service logs (journald), unchanged.

## G. Release-candidate automated gate

Full existing regression (frontend + backend + all 24 canonical
integration scripts) plus this stage's own new diagnostics/support-
bundle tests, run before any manual verification begins. Any shared
Go/frontend change requires a green `cross-platform.yml` run with no
partial-green acceptance, per this stage's own governing rule.

## H. Final dependency/security audit

A one-time, honestly-recorded audit using `govulncheck` (Go) and
`npm audit` (frontend), classifying every finding as exploitable/
relevant, not reachable, dev-only, or upstream/no-fix — never claiming
"zero vulnerabilities" from a single scanner's raw number. Recorded in
full in `docs/progress.md`.

## I. Final packaging matrix

Windows, macOS (unsigned, per 20C1), Linux desktop, and Linux headless
— each already has local build tooling; this stage adds the one
missing native CI gate (Windows, §21 finding) and re-verifies the
other three only if this stage's own source changes affect their
packaged runtime inputs.

## J. Updater compatibility gate

`verify-updater.mjs` re-run against any build this stage's own changes
touch. The release-manifest generator (`cmd/releasemanifest`) is
exercised in a dry run against representative real artifacts — never
against a public GitHub Release.

## K. Final browser/manual UX verification

A source-level UX audit (placeholder pages, stale planned/not-
implemented text, broken links, missing loading/error states,
untranslated strings) precedes the manual gate — release-blocking
polish only, no redesign.

## L. Final OBS verification

Real, physical OBS — local RTMP ingest and Browser Source overlays —
is manually verified at the single consolidated gate (§37 of the
governing task), never claimed from FFmpeg-based automated proof
alone.

## M. Platform-by-platform physical/manual evidence rules

This document reuses `docs/platform-support.md`'s own existing
vocabulary verbatim: **Supported**, **Automated-build verified**,
**Native CI verified**, **Cross-compilation verified**, **Planned**,
**Experimental**, **Deferred**, **Unsupported**, **Not verified**. A
native CI run is not a physical/manual test; a physical test on one
platform/architecture never upgrades another's status. `docs/manual-
verification.md` records exactly which physical hardware was actually
available and used.

## N. What can and cannot be called "Supported"

A platform's "Supported" label requires the *combination* its existing
stage already earned (native CI evidence) plus, where the operator's
hardware makes it feasible, a real physical manual pass recorded in
`docs/manual-verification.md`. Where hardware is unavailable, the
platform's status is not downgraded from what its own completed stage
already established — it simply does not gain a NEW physical-
verification claim it never had.

## O. Stage 20C2 external-signing limitation

Unchanged, restated for this document's own completeness: Stage 20C2
(Apple Developer signing/notarization) remains Planned — externally
gated on real Apple Developer credentials the operator does not
currently have. Stage 20E does not implement ad-hoc signing, does not
fabricate a Developer ID substitute, and does not claim Gatekeeper/
public-distribution readiness for macOS regardless of manual test
outcomes on Mac hardware.

## P. Public-release/tag policy

Stage 20E proves release *readiness*. It does not create a Git tag,
does not create a GitHub Release, does not publish artifacts, and does
not point the updater at fabricated production release metadata. A
separate, explicit, future operator instruction is required to publish
anything.

## Q. Final Stage 20 completion semantics

Stage 20E is marked Completed only after: automated regression is
green; required CI is green; diagnostics/support-bundle work
(including its own redaction self-audit) is complete; release
hardening (dependency audit, identity/version audit, release-manifest
dry run, packaging matrix) is complete; the single consolidated manual
verification gate has run and every release-blocking failure it found
has been fixed and retested. Stage 20 as a whole remains **Incomplete**
even after Stage 20E completes, because Stage 20C2 remains externally
gated — this is not a defect Stage 20E can close.
