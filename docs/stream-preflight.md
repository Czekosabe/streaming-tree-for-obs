# Stage 26 — Stream Preflight & Launch Readiness

Canonical contract for Stage 26. Written after a full audit of the
real current architecture this stage must derive readiness from -
`internal/runtime/branch`, `internal/runtime/ffmpeg`,
`internal/runtime/mediamtx`, `internal/domain/{platform,output,
credential,account,metadatapreset,streamsetup}`, `internal/updater`,
`internal/sysresources`, and the existing frontend Dashboard/status
conventions - nothing here is assumed or invented.

## 0. What this is, and is not

Stream Preflight answers one question immediately before a real
broadcast: **"Is this setup actually ready to stream?"** - by
aggregating state every domain already tracks, never by inventing a
new source of truth. It is:

- **A READINESS AGGREGATOR**, composing existing services (branch
  runtime, FFmpeg resolution, MediaMTX/ingest, destination
  configuration, connected-account health, local metadata validity,
  Stage 25 setup-profile integrity). It reimplements none of their
  business rules.
- **NOT a replacement for Stage 21 onboarding** - onboarding teaches
  first-time setup; preflight evaluates the CURRENT configuration
  immediately before a real stream, any number of times.
- **NOT a scoring system.** No percentage, no gamified metric - a
  deterministic Ready / Ready with warnings / Not ready derived from
  real blocker/warning counts (§5).
- **NEVER an action itself.** Preflight only reports; starting a
  stream remains the existing explicit `POST
  /api/runtime/branches/start-enabled` (or per-branch start) action,
  reused unchanged (§8).

## 1. Audit result: what real state already exists

### 1.1 Branch/ingest state machine (`internal/runtime/branch`)

`branch.State` (`state.go`): `idle | blocked | waiting_for_ingest |
starting | live | restarting | stopping | error`.

`Manager.computeBlockers(ctx, platform.Platform) ([]string,
launchInputs, error)` (`manager.go:333-373`, **unexported**, called
from `StartBranch` before anything is spawned) is the single real
source of "would this destination actually start right now,"
producing zero or more of 8 stable blocker identifiers
(`state.go:47-56`): `platform_disabled`, `output_server_missing`,
`stream_key_missing`, `credential_store_unavailable`,
`ffmpeg_missing`, `ffmpeg_incompatible`, `mediamtx_not_ready`,
`ingest_not_receiving`.

**Confirmed empirically (§33's own question, resolved by reading the
real code, not guessed):** for a destination that has never been
started, `ingest_not_receiving` (OBS/publisher not yet connected to
the local MediaMTX ingest) **is a real blocker** - `StartBranch`
refuses to launch FFmpeg at all while it holds, and leaves
`desiredRunning = false`, so nothing auto-resumes once OBS connects;
the operator must click Start again. This is architecturally correct
(a destination's FFmpeg process reads from the local ingest and
re-publishes it - there is nothing to read yet), and it means the
real intended workflow is: start MediaMTX → start OBS publishing to it
→ THEN start a destination branch. Only AFTER a branch has already
gone `live` at least once does losing the publisher become the
softer, self-healing `waiting_for_ingest` state (`watchExit`,
`manager.go:560-581`) that the reconciliation loop resumes
automatically. **Preflight must reflect this distinction precisely**:
for a destination not currently live, "ingest not receiving" is a
genuine BLOCKER, not an informational "waiting" state.

`computeBlockers` is pure reads (`Outputs.Get`, `Credentials.Status`,
`cachedFFmpeg()`, `Ingest.Snapshot()`) - no side effects, safe to call
outside `StartBranch`. It has no HTTP-exposed equivalent today; §4
below adds one.

### 1.2 FFmpeg resolution (`internal/runtime/ffmpeg`)

`GET /api/runtime/ffmpeg` (`ffmpegStatusResponse`,
`httpapi/branch_runtime.go:39-68`) already reports a public `state`:
`ready | missing | incompatible | error`, cached and refreshed every 5
minutes - never re-probed per request, never exposes the binary path.
`state != "ready"` is exactly the predicate `computeBlockers` already
uses. Preflight reuses this response verbatim; no new FFmpeg check is
written.

### 1.3 MediaMTX / ingest engine (`internal/runtime/mediamtx`)

`GET /api/runtime` (`Snapshot{MediaMTX, Ingest, Connection}`,
`state.go`) already separates the two distinct facts preflight needs:
`MediaMTX.State` (`missing|installing|incompatible|stopped|starting|
ready|stopping|error` - the engine process itself) from `Ingest.State`
(`unavailable|waiting|receiving|error` - whether a publisher is
currently connected). `Ingest.State == waiting` is exactly "engine
running, OBS not connected yet." Preflight reuses this response
verbatim.

### 1.4 Destination/credential/output readiness

- `platform.Platform.Enabled` - the existing selection mechanism,
  unchanged.
- `output.Settings.ServerURL == ""` - "no server configured," already
  the exact predicate `computeBlockers` uses. `output.ValidateServerURL`
  runs at save time; there is no separate persisted "malformed URL"
  flag to read back, so absence is the only readable signal.
- `GET /api/platforms/{id}/credentials` → `{streamKey:{configured},
  store:{available}}` (`httpapi/credentials.go`) - already an
  API-safe boolean pair; the raw stream key is never reachable outside
  `credential.Service.RetrieveForProcessStart`, which preflight never
  calls.

### 1.5 Connected-account health (`internal/domain/account`)

`Account.Status`: `connected | reconnect_required`. **Confirmed by
package-dependency audit**: `branch.Manager` never imports
`internal/domain/account` at all, and `computeBlockers` has no
account check - every real consumer of account status is
metadata-publish/chat/engagement/alerts/goals, never the branch/
FFmpeg/RTMP path. Per governing product policy, this stage encodes
that fact precisely: a `reconnect_required` account is surfaced only
as a WARNING affecting optional metadata/engagement features, **never**
a video-routing blocker.

### 1.6 Local metadata validation (`internal/domain/platform`)

`platform.ValidateMetadata(def platform.ProviderDefinition, in
platform.Metadata) (platform.Metadata, error)` is a pure, local,
capability-table-driven check with no network call - already reused
read-only by `metadatapreset.Service.ApplyPreview`
(`metadatapreset/apply.go`). Preflight calls it directly against each
destination's own currently-stored `Platform.Metadata` (not a preset
candidate) to get "would this provider reject the metadata as
currently saved" - a WARNING (affects Publish only, never Start).

### 1.7 Stage 25 stream setup profile integrity

`streamsetup.Service.Preview(ctx, profileID) (Preview, error)` already
computes, read-only: per-destination `Change` (including
`ChangeMissing` for a deleted destination reference) and
`MetadataPresetMissing` (a deleted preset reference). Preflight reuses
`Preview` directly for whichever profile is selected (§3) - no second
implementation of "does this profile have a broken reference."

### 1.8 Canonical "is a broadcast active" definition

`updater.StreamingActive(snapshots []branch.Snapshot) bool`
(`updater/guard.go`) is the existing, exported, canonical definition:
true if any snapshot's `State` is in `{starting, live, restarting,
waiting_for_ingest, stopping}` **or** its `DesiredRunning` is true.
Preflight reuses this function directly rather than adding a third
copy of the state set (`streamsetup.activeBranchStates` is already a
second, narrower copy without the `DesiredRunning` check - a known,
accepted, pre-existing duplication this stage does not need to add
to).

### 1.9 Disk/resource health - excluded from v1

`GET /api/system/resources` reports raw `CPUPercent`/`MemoryPercent`/
`DiskPercent` with an honest `Unavailable` list - **no backend
threshold concept exists anywhere**. The only threshold-shaped logic
found is a frontend-only display-color convention
(`SystemResourcesCard.tsx`'s 65/85 bar-color cutoffs), never exposed
as an API field or documented as a real product threshold. Per
governing policy ("do not generate a fake metric"), **Stage 26 v1 does
not add a disk/resource blocker or warning** - this data is already
visible on the Dashboard's existing System resources card, and
inventing a pass/fail threshold here would be exactly the kind of
fabricated signal this stage must avoid. Revisit only if a future
stage defines a real, product-owned threshold.

### 1.10 Restart-required state after Stage 23 restore - excluded from v1

`RestoreResult.RestartRequired` is always `true` on a successful
restore, but it is **purely a one-shot HTTP response field** - no
persisted backend flag exists (confirmed: zero matches for a
restart-pending concept anywhere in `apps/server`), and the frontend
only holds it in transient component state that is lost on navigation
or reload. There is currently no way for anything, including
preflight, to later ask "are we running post-restore, unrestarted?".
Adding a persisted flag purely to serve this one preflight check would
be new product surface beyond this stage's own "aggregate existing
state" charter, so **Stage 26 v1 does not include a restart-required
check** - documented here as an honest, acknowledged gap rather than
silently invented or silently omitted without explanation.

### 1.11 Existing frontend conventions to reuse, not duplicate

- `SystemStatusRail`/`ServicesCard` already aggregate backend
  reachability, MediaMTX state, and FFmpeg state via
  `useHealthQuery`/`useRuntimeQuery`/`useFfmpegRuntimeQuery` - Stage 26
  reuses these same hooks.
- `models/branch-presentation.ts`'s `blockerKey` already maps every
  one of the 8 blocker identifiers to an i18n key - reused verbatim
  for blocker display text.
- `useStartEnabledBranchesMutation`/`POST
  /api/runtime/branches/start-enabled` is the existing "Start" action
  (already surfaced via `QuickActionsCard`/`StartEnabledConfirmDialog`)
  - this is the exact action Preflight's own explicit launch button
  reuses (§8), never a new start endpoint.
- State freshness convention across this whole domain is adaptive
  TanStack Query polling (fast while something can change, slow
  otherwise, `refetchIntervalInBackground:false`) - never SSE, never a
  bespoke interval. Preflight composes existing queries plus its own
  new one under the same convention (§9).
- **A real, pre-existing gap found during this audit**:
  `StartEnabledConfirmDialog`'s own "eligible vs. skipped" list is
  built from each platform's cached `GET /api/branches` blockers,
  which are only ever populated once a real start attempt has run
  `computeBlockers` at least once - a never-started platform shows as
  "eligible" even if it would actually fail immediately. This is a
  correctness gap in existing UI, not something Stage 26 inherits
  silently: §4 gives preflight a genuine live-evaluation path so its
  own readiness view does not repeat this mistake.

## 2. Readiness model

```go
package preflight

type Severity string
const (
    SeverityBlocker Severity = "blocker" // prevents the intended stream from working
    SeverityWarning Severity = "warning" // may work, needs attention
    SeverityInfo    Severity = "info"    // useful, non-blocking
)

// Finding is one concrete, actionable readiness fact.
type Finding struct {
    Code       string   // stable identifier, e.g. "stream_key_missing"
    Severity   Severity
    PlatformID string   // "" for a finding not scoped to one destination
    Action     *Action  // nil if nothing to do (already healthy)
}

// Action points at an existing corrective route/action - preflight
// never duplicates a configuration form.
type Action struct {
    Code string // "add_stream_key" | "open_destination_settings" |
                // "start_mediamtx" | "install_ffmpeg" |
                // "reconnect_account" | "fix_metadata" |
                // "repair_setup_profile" | "start_enabled_branches"
    PlatformID string // "" when not destination-scoped
}

type Report struct {
    Status              Status // "ready" | "ready_with_warnings" | "not_ready"
    Findings            []Finding
    Destinations        []DestinationReadiness
    SelectedProfileID    *string // the Stage 25 profile this report evaluated, if any
    StreamingActive      bool    // updater.StreamingActive(snapshots) - live-state note, see §7
}

type DestinationReadiness struct {
    PlatformID  string
    ProviderID  string
    DisplayName string
    Findings    []Finding // this destination's own blockers/warnings/info
}
```

`Status` is computed deterministically, never a score (§0):
`not_ready` if any `SeverityBlocker` finding exists; else
`ready_with_warnings` if any `SeverityWarning` exists; else `ready`.

## 3. Profile-aware preflight

Stage 25 deliberately never persists a "currently active/applied
profile" concept (its own §18/§11 decision - no drift indicator, no
active-profile pointer). Preflight therefore accepts an OPTIONAL
`profileID` query parameter, chosen transiently in the frontend (a
selector local to the Preflight view, never persisted):

- **`profileID` given**: preflight evaluates exactly the destinations
  `streamsetup.Service.Preview(ctx, profileID)` reports (its
  `PlatformID`s, skipping `ChangeMissing` entries which surface their
  own `repair_setup_profile` finding instead), plus surfaces
  `MetadataPresetMissing` as its own warning.
- **`profileID` omitted**: preflight evaluates every currently-`Enabled`
  destination - the same set `StartEnabledBranches` would act on. A
  profile is never required to stream (governing invariant, unchanged).

## 4. `branch.Manager.EvaluateReadiness` (new, read-only)

`computeBlockers` is already pure reads with no side effects. A thin
exported wrapper is added rather than a second implementation:

```go
// EvaluateReadiness computes the same blockers StartBranch would
// encounter for platformID right now, without starting anything.
func (m *Manager) EvaluateReadiness(ctx context.Context, platformID string) ([]string, error) {
    p, err := m.opts.Platforms.Get(ctx, platformID)
    if err != nil {
        return nil, err
    }
    blockers, _, err := m.computeBlockers(ctx, p)
    return blockers, err
}
```

This is the authoritative source for every `SeverityBlocker` finding
in §2 - preflight never re-derives platform/output/credential/FFmpeg/
ingest readiness itself, it only asks `branch.Manager` for the truth
it already computes internally, now made externally reachable and
live (fixing the exact staleness gap §1.11 identifies in the existing
Start-confirmation dialog, as a side effect, though that dialog itself
is out of this stage's scope to change).

## 5. Backend architecture

`internal/domain/preflight.Service` composes narrow, existing ports -
never a parallel truth for any domain it reads from (governing §41):

```go
type BranchPort interface {
    EvaluateReadiness(ctx context.Context, platformID string) ([]string, error)
    Snapshot(ctx context.Context) ([]branch.Snapshot, error)
}
type PlatformPort interface {
    List(ctx context.Context) ([]platform.Platform, error)
}
type AccountPort interface {
    // per-account Status, scoped to the accounts linked to a
    // preflighted destination - optional-feature warning only (§1.5)
}
type MetadataPort interface {
    // read-only ValidateMetadata pass per destination (§1.6)
}
type StreamSetupPort interface {
    Preview(ctx context.Context, profileID string) (streamsetup.Preview, error)
}
```

`Service.Evaluate(ctx, profileID *string) (Report, error)` resolves
the destination set (§3), calls `BranchPort.EvaluateReadiness` per
destination for blockers, folds in account/metadata warnings, folds
in Stage 25 profile-integrity findings when `profileID` is given, and
computes `Status` deterministically (§2). No writes anywhere in this
path.

## 6. HTTP surface

`GET /api/preflight?profileId={optional}` → `Report` (§2), management-
only (never under `/api/public/*`), same route-namespace convention as
Stage 24/25.

## 7. Live-stream semantics

When `updater.StreamingActive(snapshots)` is true, `Report.StreamingActive`
is set and the frontend labels the view "Pre-stream check unavailable
while streaming" (governing §39) rather than presenting a confusing
"not ready" for destinations that are, in fact, already live -
preflight never duplicates Stage 24 History or diagnostics; it links
to them instead of re-rendering their data.

## 8. Explicit launch, never automatic

Preflight computes and displays `Report` only. The frontend's own
"Start" action is the EXISTING `useStartEnabledBranchesMutation` /
`POST /api/runtime/branches/start-enabled` call, reused unchanged -
preflight adds no new start endpoint and never calls it on the
operator's behalf. If `Status == "not_ready"`, Start stays available
but the operator gets a clear list of what will fail and why (some
destinations, e.g. ones with no blockers, may still be worth starting
even when a different destination is not ready - the existing
per-destination/all-destinations start actions are unchanged).
Applying a Stage 25 setup profile or publishing metadata are never
triggered from this view either - Local Save, provider Publish, and
Start remain three separate explicit actions, unchanged invariant.

## 9. Frontend surface

A fourth Dashboard action button ("Preflight"), alongside "Add
Platform"/"Stream Setups"/"Global Settings" (`DashboardPage.tsx`'s
existing `actions` slot), opening a compact panel/dialog - not a
permanent giant block on the Dashboard itself. Composes the existing
`useHealthQuery`/`useRuntimeQuery`/`useFfmpegRuntimeQuery` plus a new
`usePreflightQuery` under the same adaptive-polling convention (§1.11)
- no SSE, no bespoke interval. Reuses `blockerKey` from
`branch-presentation.ts` for every blocker's display text.

## 10. Security

`Report`/`Finding` never contain a stream key, an OAuth token, or a
client secret - only `configured`/`available`-shaped booleans, the
same DTO discipline `credentialStatusResponse` already established.
Structural proof (mirroring `streamsetup`/`metadatapreset`'s own
`security_test.go` pattern) covers `Report`/`Finding`/`Action`/
`DestinationReadiness`.

## 11. Substage decomposition

- **26A**: `internal/domain/preflight` domain package,
  `branch.Manager.EvaluateReadiness`, unit tests.
- **26B**: HTTP API + `main.go` wiring.
- **26C**: frontend Preflight panel/dialog + i18n (EN/PL).

## 12. Testing plan

Backend: all-ready, missing FFmpeg, MediaMTX not ready, ingest not
receiving on a never-started destination (blocker) vs. an
already-live destination that lost ingest (non-blocking per §1.1),
missing stream key, disabled platform, optional-account warning,
metadata-validation warning, broken Stage 25 profile reference
(missing destination / missing preset), no profile given → currently-
enabled set, multiple destinations with one blocker among many,
`StreamingActive` semantics, no-secret DTO structural proof, no
provider network call anywhere in this path.

Frontend: ready / ready-with-warnings / not-ready states, per-
destination findings with action links, profile-aware vs.
current-config mode, live-stream label swap, explicit Start reuse (no
auto-start), keyboard/responsive/EN-PL.

Integration: hermetic backend, deterministic severity across every
combination above, confirm `EvaluateReadiness` matches what a real
`StartBranch` call would have produced for the same platform/state.

## 13. Completion criteria

Contract shipped (this document); `branch.Manager.EvaluateReadiness`
real and tested; readiness model/severity rules real; profile-aware
preflight real; actionable remediation links real; explicit-launch
reuse proven (no new start path, no auto-start, no auto-publish); no
fake scoring; secret boundary proven; EN/PL complete; backend/
frontend/integration tests green; all correctly-routed CI terminal and
green; tree clean; `origin/main...HEAD` = `0 0`.
