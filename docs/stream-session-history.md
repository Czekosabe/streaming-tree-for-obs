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

## 14. Stream insights (Stage 27 aggregate view)

Merged from the former `docs/stream-insights.md` (final product-
polish/documentation-cleanup pass) - a read-only aggregation over
this same session-history data, adding no new persisted domain and
no new privacy surface of its own.


### 14.0 Selection audit

Real current product gaps were compared, per §44's own four
candidates, after onboarding/metadata presets/backup-restore/
operational history/stream setup profiles/preflight all already ship:

- **(A) Scheduling/planned stream preparation.** Would require a
  wholly new domain (dates, reminders, notification delivery) with no
  existing state to derive from - the biggest new-surface, highest-
  risk option of the four, and not clearly higher creator value than
  the alternative below.
- **(B) Operational insights/statistics derived ONLY from Stage 24
  non-engagement history.** `apps/web/src/pages/HistoryPage.tsx`
  (Stage 24C) already lists individual sessions with retention
  controls, but nothing aggregates across them - no "how much have I
  actually streamed," no per-destination reliability, no session-
  length distribution. `streamsession.Repository.ListSessions` already
  returns every retained `Session`/`Destination` row (`StartedAt`/
  `EndedAt`/`EndReason`/`Outcome`, already fully authorized, already
  structurally excludes engagement content per Stage 24's own
  content-exclusion proof) - genuinely valuable aggregation is
  possible with **zero new persisted state, zero new privacy
  surface, and zero new domain to design from scratch**: a pure,
  read-only computation over data this application already records
  and already shows individually.
- **(C) Recovery/repair assistant.** Materially overlaps Stage 26,
  which already surfaces every real blocker/warning with a pointer to
  its existing corrective action. A dedicated "repair assistant"
  would either duplicate Preflight's own remediation surface or need
  a proactive background-scanning design Preflight does not have -
  lower marginal value now that Preflight exists than it would have
  had before Stage 26.
- **(D) Other candidates found by direct audit.** No other concrete,
  evidence-backed gap of comparable value was found; the History
  page's own individual-session view is otherwise complete for what
  it does.

**Selected: (B).** It is the only option requiring no new persisted
domain, no new privacy boundary decision, and no product-direction
choice left unresolved - the governing task's own §44 already settled
the boundary explicitly (no audience analytics, no chat/donation
persistence, no feasibility-gated provider connectors), and (B) never
approaches either line. No consequential fork remains to report;
implementation proceeds directly.

### 14.1 What this is, and is not

Stream Insights answers "how has my streaming actually gone?" by
aggregating Stage 24's already-recorded session history - never a new
data source, never anything derived from chat/donation/engagement
content (structurally impossible: `streamsession.Session`/
`Destination` never carried any of it, per Stage 24's own content-
exclusion proof, which this stage's own output inherits unchanged).

- **NOT** a new persisted domain - `streaminsights.Service` writes
  nothing; every field is computed at request time from
  `streamsession.Repository.ListSessions`.
- **NOT** audience/viewer analytics - every metric is about the
  application's own operational timeline (when it streamed, for how
  long, to which destinations, with what outcome), never about who
  watched, chatted, or donated.
- **NOT** a replacement for the History page's per-session detail -
  Stream Insights is the aggregate view; History remains the
  individual-session view; neither duplicates the other.

### 14.2 Model

```go
package streaminsights

type Insights struct {
    TotalSessions          int
    TotalStreamingDuration time.Duration // sum of every session's own duration (an open session counts up to "now")
    AverageSessionDuration time.Duration // 0 when TotalSessions == 0
    LongestSession         *SessionSummary
    SessionsByEndReason    map[string]int // streamsession.EndReason string -> count; "" (still open) is a real bucket
    Destinations           []DestinationInsights
}

type SessionSummary struct {
    SessionID string
    StartedAt time.Time
    Duration  time.Duration
}

// DestinationInsights groups every session-destination row across
// history that snapshot-matches one real destination identity.
// Grouping key is PlatformID when non-nil (a currently-existing
// destination); a row whose PlatformID is nil (the destination was
// since deleted) groups by its own ProviderID+DisplayName snapshot
// instead, exactly mirroring how the History page itself already
// displays a deleted destination's row - never silently dropped,
// never misattributed to an unrelated currently-existing destination
// of the same provider.
type DestinationInsights struct {
    PlatformID    *string
    ProviderID    string
    DisplayName   string
    SessionCount  int
    TotalDuration time.Duration
    OutcomeCounts map[string]int // streamsession.Outcome string -> count
}
```

### 14.3 Computation

`Service.Compute(ctx) (Insights, error)` calls
`streamsession.Repository.ListSessions(ctx, -1)` (SQLite's own
documented "no limit" `LIMIT` convention, already the exact table
`PruneSessionsBefore` already bounds by the operator's own retention
setting - no new unbounded-read concern) and folds the result client-
side in Go. A still-open session's own duration is computed against
`time.Now()`, not treated as zero or excluded - an in-progress stream
is real operational time already happening.

### 14.4 HTTP surface

`GET /api/stream-insights` → `Insights`, management-only, same route-
namespace convention as every other Stage 24/25/26 surface. Read-only;
no write route exists or is needed.

### 14.5 Frontend surface

A new "Insights" tab/section on the existing History page
(`HistoryPage.tsx`) - not a new top-level navigation entry, since this
is a different view of the exact same underlying data the page already
owns. Shows total sessions/streaming time/average length, the longest
session, an end-reason breakdown (clean vs. crash-recovered), and a
per-destination reliability table (session count, total time, outcome
breakdown) - reusing the page's own existing `formatTimestamp`/
`toDurationParts`/provider-brand conventions, never a new formatting
scheme.

### 14.6 Security/privacy

`Insights` structurally cannot carry engagement content - every field
is a count, a duration, or an already-authorized identity snapshot
(`ProviderID`/`DisplayName`, already exported by Stage 24 itself).
Mirrors the same structural secret/content-exclusion proof pattern
(`security_test.go`) every prior stage this session established,
adapted to the content-exclusion (not secret-exclusion) axis, matching
`streamsession`'s own existing content-exclusion test's exact denylist
approach.

### 14.7 Testing plan

Backend: zero sessions → zero-valued `Insights` (never a divide-by-
zero), a single completed session, multiple sessions across multiple
destinations aggregating correctly, an open session's duration counted
against "now", a session ending in `unclean_shutdown` bucketed
correctly, a deleted destination (`PlatformID == nil`) grouped by its
own snapshot rather than dropped or merged into an unrelated
destination, longest-session selection, content-exclusion structural
proof. HTTP: the report round-trips, no write route exists.

Frontend: empty state, populated state, per-destination table
rendering, EN/PL, responsive/keyboard.

### 14.8 Completion criteria

Contract shipped (this document); `streaminsights.Service` real and
tested; HTTP surface real; frontend Insights view real; content-
exclusion structural proof real; EN/PL complete; backend/frontend
tests green; all correctly-routed CI terminal and green; tree clean;
`origin/main...HEAD` = `0 0`.
