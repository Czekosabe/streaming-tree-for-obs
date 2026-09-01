# Stage 24 — Stream Session / Operational History

## 0. What this is, and is not

Stage 24 is **stream session / operational history**: a local record of
*when Streaming Tree itself observed a stream session run* - when it
started, when it ended, which destinations participated and with what
coarse outcome. It is a log of the application's own operational
behavior, the same category of thing Stage 20E's diagnostics/support
bundle already records for backend events generally, now scoped
specifically to streaming sessions and given its own durable,
queryable, retained history (unlike Stage 20E's bounded in-memory ring
buffer, which never survives a restart).

Stage 24 is explicitly **not**:

- A record of chat messages, chatter names, donation messages, donor
  names/amounts, membership/Super Chat content, alert payload content,
  or TTS text. This is the same content-exclusion boundary Stage 23's
  backup feature draws around secrets, applied here to a different
  category: this feature must never become a database of what
  viewers said or gave, only of how Streaming Tree itself behaved.
  This is a distinct, independent decision from the previously-
  deferred *engagement-event history* question (a possible future
  stage recording chat/donation events themselves) - Stage 24 does not
  resolve, enable, or foreclose that question; it is simply not what
  Stage 24 is.
- A viewer analytics or growth-tracking feature. No follower/
  subscriber/view counts are recorded here - those already have their
  own home in Stage 18's goals/widgets domain.
- A replacement for Stage 20E's operational diagnostics log. That ring
  buffer remains exactly as it is; Stage 24 does not read from it,
  write to it, or change its retention.

## 1. Session-boundary definition

**A session is a contiguous period during which the local MediaMTX
ingest is actively receiving a publish from OBS** -
`mediamtx.Supervisor.Snapshot().Ingest.State == IngestReceiving`
(`internal/runtime/mediamtx`). This is the definition, not a proxy for
it: OBS publishing to local ingest is the one signal that is true
exactly when a creator is actually streaming, independent of how many
destinations are configured, enabled, or currently erroring.

Destination-branch state (`internal/runtime/branch.Manager`,
`updater.StreamingActive`'s own notion of "streaming active" - see
`internal/updater/guard.go`) is deliberately **not** the session
signal: a branch can be `WaitingForIngest` indefinitely with nothing
ever flowing (a destination toggled on with no OBS connection), and a
branch can only ever reach `StateLive` when ingest is genuinely
receiving in the first place (FFmpeg has nothing to relay otherwise) -
so ingest state is both necessary and sufficient, and branch state
adds no independent boundary information. Branch/platform state is
still used, but only to build the **per-destination participation**
records *within* a session's already-determined bounds (§3), not to
decide where those bounds are.

**Grace window against transient disconnects.** OBS reconnecting after
a brief network blip must not fragment one real session into several.
The detector tracks `lastSeenReceivingAt`; a session is only actually
closed once ingest has been continuously non-`Receiving` for longer
than `SessionGraceWindow` (60 seconds - long enough to absorb a normal
reconnect blip, short enough that a genuinely ended session closes
within about a minute, never confused with "the operator went to get
coffee"). The session's own `endedAt` is always set to
`lastSeenReceivingAt` - the last real moment it was actually
receiving - never to the later moment the grace window happened to
expire. No session boundary is ever derived from process uptime,
wall-clock polling cadence, or any timer that does not itself trace
back to a real observed ingest-state transition.

**Polling, not push.** Neither `branch.Manager` nor
`mediamtx.Supervisor` exposes a subscribe/callback/event-bus mechanism
today (confirmed by direct source review) - both are polling-only.
The Stage 24 detector polls `Supervisor.Snapshot()` and
`branch.Manager.Snapshot(ctx)` on its own timer (`PollInterval`, 5
seconds) and diffs against its own last-observed state; it does not
require, and this stage does not add, a push mechanism to either
package.

## 2. App-restart / unclean-shutdown semantics

There is no generic shutdown-hook registry in this codebase
(`cmd/server/main.go`'s `shutdownRuntime` closure calls each
subsystem's own `Shutdown` in a fixed, hand-written order) - Stage 24
adds its own `Manager.Shutdown(ctx)` call into that same sequence,
stopping the poll loop and, if a session is currently open, leaving
its row exactly as it is (`endedAt` still `NULL`) rather than guessing
an end time it does not actually know. This is deliberate: a graceful
shutdown while a session is genuinely still active (the operator quit
Streaming Tree without stopping OBS first) is not meaningfully
different from a crash from this feature's own point of view - in
neither case does the process observe a real ingest-stopped
transition before it goes away.

**Heartbeat.** `lastSeenAt` is updated on every poll tick where ingest
is confirmed `Receiving` - the same instant §1's grace-window clock
uses, deliberately never updated during a grace-window-waiting tick
(ingest not currently receiving, session not yet closed), so it always
means exactly "the last real moment this session was observed
receiving," never "the last time the process happened to be alive."
This is what makes recovery honest rather than guessed.

**Startup recovery.** On `Manager.Start`, before the poll loop begins,
the repository is checked for any session row with `endedAt IS NULL`
(there can be at most one, by construction - see §3). If found, it is
immediately closed: `endedAt = lastSeenAt` (its own last real
heartbeat, never `time.Now()`, never fabricated), `endReason =
"unclean_shutdown"`, and every one of its still-open destination-
participation rows (§3) is closed the same way, at the same
timestamp. The poll loop then starts fresh from the real, current
ingest/branch state exactly as it would on any other tick - if OBS is
still actually publishing at that moment, a **new** session begins
immediately, never treated as a continuation of the recovered one
(continuity was already broken by the restart itself; pretending
otherwise would misrepresent what actually happened).

## 3. Data model

Two tables, migration `0031_stream_sessions.sql` (SQLite, up-only, the
existing convention - see `internal/storage/sqlite/migrations/`).
Timestamps are RFC3339Nano strings, matching every other domain in
this codebase (`platform.FormatTimestamp`/`ParseTimestamp`).

```sql
CREATE TABLE stream_sessions (
  id            TEXT PRIMARY KEY,
  started_at    TEXT NOT NULL,
  ended_at      TEXT,                    -- NULL while the session is open
  last_seen_at  TEXT NOT NULL,           -- heartbeat; also the source of a recovered session's own endedAt
  end_reason    TEXT NOT NULL DEFAULT '', -- '' while open; see §5 once closed
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

-- At most one row with ended_at IS NULL at any time - enforced by the
-- Manager's own single-poll-loop invariant, not by a SQL constraint
-- (SQLite has no partial-unique-index-portable way to express this
-- across every version this project supports; the invariant is proven
-- by the Go-level tests instead, per §7).

CREATE TABLE stream_session_destinations (
  id            TEXT PRIMARY KEY,
  session_id    TEXT NOT NULL REFERENCES stream_sessions(id) ON DELETE CASCADE,
  -- platform_id is SET NULL, never CASCADE, if the destination is later
  -- deleted - deleting a destination must never delete its own history
  -- (governing requirement). provider_id/display_name are a SNAPSHOT
  -- taken when the row is created, never re-resolved from the live
  -- platform row, so a later rename or deletion never rewrites what
  -- this history says happened at the time.
  platform_id   TEXT REFERENCES platforms(id) ON DELETE SET NULL,
  provider_id   TEXT NOT NULL,
  display_name  TEXT NOT NULL,
  started_at    TEXT,                    -- first observed StateLive within this session
  ended_at      TEXT,                    -- last observed StateLive, or the session's own endedAt
  outcome       TEXT NOT NULL DEFAULT '', -- '' while open; see §5 once closed
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
```

A destination that never actually went live during a session (stayed
`WaitingForIngest`/`Blocked`/disabled the whole time) gets no
`stream_session_destinations` row at all - this table is a record of
real participation, not of every platform that merely existed.

## 4. Bounded error classification

Never a raw FFmpeg stderr line or a provider HTTP response body - the
same "operational classification, not diagnostic transcript" rule
Stage 20E's own log redaction already applies. A closed lookup table,
mirroring the `updateBlockerKey`/`updateErrorKey` convention
(`apps/web/src/models/updates-presentation.ts`) on the frontend and a
plain Go string enum on the backend:

`outcome`: `completed` (the destination was live and the session ended
normally), `error` (the branch's own last-known `State` was
`StateError` when it stopped being live), `session_ended` (the
destination was still live when the whole session ended - a graceful
end, not an error, but distinct from `completed` since nothing about
*that destination specifically* signaled it was done).

## 5. Session end reasons

`end_reason`: `ingest_stopped` (the normal case - ingest genuinely
stopped receiving and the grace window elapsed), `unclean_shutdown`
(§2 recovery path).

## 6. Retention

Enabled by default - explicitly authorized by the governing task,
since (unlike an engagement-content history) nothing third-party or
personally sensitive is ever stored here, only the application's own
operational timeline. Default retention: **90 days**, configurable in
Settings, plus an explicit **"Clear history"** action that deletes
every session immediately regardless of age. Pruning runs as a bounded
sweep on the same timer as the poll loop (delete every session whose
`endedAt` is older than the retention window; `ON DELETE CASCADE`
handles the destination rows) - never a separate scheduled job with
its own failure mode to reason about.

## 7. Live/in-progress display and honest empty states

The currently-open session (if any) is served as a distinct "in
progress" entry, never backdated into looking like a finished one -
its own `endedAt`/`outcome` fields read as null/pending in the API and
UI, not silently omitted or defaulted to a misleading value. A
genuinely empty history (a fresh install, or after "Clear history")
renders as an honest empty state, never a fabricated example session.

## 8. HTTP surface

`internal/httpapi` gains a narrow, read-mostly surface:
`GET /api/stream-sessions` (bounded list, newest first, paginated the
same way every other list route in this codebase already bounds
itself), `GET /api/stream-sessions/{id}` (one session with its
destination-participation rows), `DELETE /api/stream-sessions` (Clear
history - requires an explicit confirmation body, mirroring
`requireEmptyBody`/`confirm: true` conventions already used for
destructive routes elsewhere), `GET/PUT
/api/stream-sessions/settings` (retention-days preference). No public/
overlay-facing route - this is a management-only surface, the same
route-namespace boundary that already gates every other non-public
route under `withRemoteManagementSecurity` when remote management is
enabled.

## 9. Frontend

A new **History** page (top-level navigation entry), listing sessions
newest-first with duration, destination chips (provider + snapshot
display name + outcome), and the in-progress session (if any) pinned
at the top with a live-updating duration. A settings panel for
retention days and "Clear history" (destructive, behind
`ConfirmDialog`, mirroring the Stage 23 Backup & Restore panel's own
destructive-confirmation pattern built this session). EN/PL
localization, keyboard/accessible controls, matching every other
surface's existing conventions - no new UI pattern invented where an
existing one already fits.

## 10. Never touches streaming functionality

The poll loop only ever calls the two existing `Snapshot` methods
(already read-only, already called elsewhere in this codebase, e.g.
by whatever serves the current dashboard/status view) and writes to
its own two tables. It never calls any branch-control method, never
starts/stops FFmpeg or MediaMTX, and a failure in this subsystem
(a database error, a panic recovered at the poll-loop boundary) must
never prevent or interrupt an actual stream - the same "diagnostics
must never be able to take down the product" principle Stage 20E's
own logging already commits to.

## 11. Substage decomposition

- **24A** - this contract; migration; `internal/domain/streamsession`
  (or a similarly-named) package: the data model, repository port,
  the poll-loop `Manager` implementing §1/§2, bounded outcome/end-
  reason enums.
- **24B** - HTTP API (§8), retention/prune sweep (§6), `main.go`
  wiring (construction + `Shutdown` registration).
- **24C** - frontend: the History page, live in-progress display,
  retention settings + Clear history, EN/PL.
- **24D** - integration test (a hermetic script exercising a real
  MediaMTX-free simulated ingest transition, or - if that proves
  impractical without a real OBS/MediaMTX - a Go-level integration
  test driving the poll loop against fake `Snapshot` sources on a real
  SQLite database, the same rigor Stage 23's own
  `security_integration_test.go` used), packaged-runtime extension,
  PRIVACY.md update, a content-exclusion proof test (seeds a
  synthetic chat/donation-shaped sentinel somewhere reachable and
  proves it never appears in a session row - the same "prove it, do
  not just state it" standard Stage 23's own secret-exclusion tests
  set).

## 12. Testing plan

Backend: session-boundary determinism (a scripted sequence of fake
`Snapshot` results across many ticks produces exactly the expected
session/destination rows, including a blip shorter than the grace
window never fragmenting a session, and one longer than it always
does); the startup-recovery path (an unclosed row with a stale
heartbeat is closed at that heartbeat's own time, not `time.Now()`);
retention pruning (sessions older than the window are gone, cascade
removes their destination rows, an in-progress session is never
pruned regardless of age); destination-deletion resilience (deleting
a platform leaves its history's snapshot fields intact and does not
cascade-delete any session); the content-exclusion proof (§11 24D).
Frontend: EN/PL, empty/list/in-progress states, Clear-history
confirmation flow. Integration: real poll-loop-against-real-SQLite,
matching Stage 23's own established rigor rather than only unit-level
fakes. Packaged: extend `scripts/verify-packaged-app.mjs`.

## 13. Completion criteria

Every substage in §11 complete and tested per §12; `PRIVACY.md`
updated with an honest, precise description of what this feature
records and (explicitly) does not; no physical/manual OBS pass
required or requested, consistent with every stage since 20E - Stage
20E's own physical/manual verification gate remains a separate,
deferred, operator-owned concern this stage does not touch.
