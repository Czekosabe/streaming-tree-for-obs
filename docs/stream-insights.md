# Stage 27 — Stream Insights

Canonical contract for Stage 27, the post-Stage-26 selection audit
required by the governing task's own §44.

## 0. Selection audit

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

## 1. What this is, and is not

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

## 2. Model

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

## 3. Computation

`Service.Compute(ctx) (Insights, error)` calls
`streamsession.Repository.ListSessions(ctx, -1)` (SQLite's own
documented "no limit" `LIMIT` convention, already the exact table
`PruneSessionsBefore` already bounds by the operator's own retention
setting - no new unbounded-read concern) and folds the result client-
side in Go. A still-open session's own duration is computed against
`time.Now()`, not treated as zero or excluded - an in-progress stream
is real operational time already happening.

## 4. HTTP surface

`GET /api/stream-insights` → `Insights`, management-only, same route-
namespace convention as every other Stage 24/25/26 surface. Read-only;
no write route exists or is needed.

## 5. Frontend surface

A new "Insights" tab/section on the existing History page
(`HistoryPage.tsx`) - not a new top-level navigation entry, since this
is a different view of the exact same underlying data the page already
owns. Shows total sessions/streaming time/average length, the longest
session, an end-reason breakdown (clean vs. crash-recovered), and a
per-destination reliability table (session count, total time, outcome
breakdown) - reusing the page's own existing `formatTimestamp`/
`toDurationParts`/provider-brand conventions, never a new formatting
scheme.

## 6. Security/privacy

`Insights` structurally cannot carry engagement content - every field
is a count, a duration, or an already-authorized identity snapshot
(`ProviderID`/`DisplayName`, already exported by Stage 24 itself).
Mirrors the same structural secret/content-exclusion proof pattern
(`security_test.go`) every prior stage this session established,
adapted to the content-exclusion (not secret-exclusion) axis, matching
`streamsession`'s own existing content-exclusion test's exact denylist
approach.

## 7. Testing plan

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

## 8. Completion criteria

Contract shipped (this document); `streaminsights.Service` real and
tested; HTTP surface real; frontend Insights view real; content-
exclusion structural proof real; EN/PL complete; backend/frontend
tests green; all correctly-routed CI terminal and green; tree clean;
`origin/main...HEAD` = `0 0`.
