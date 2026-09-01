# Stage 21 — first-run onboarding + OBS setup experience

**Research date:** 2026-09-01. Written before any Stage 21 product code,
per this project's own standing "contract before implementation"
discipline. Audited against the actual current source before writing
anything below - not assumed from the governing task's own suggested
endpoint/field names.

Stage 21 development proceeds while Stage 20E physical/manual
verification remains deferred by the operator (see
`docs/manual-verification.md`'s own status section) - the two are
independent. Stage 21 does not touch Stage 20C2 (externally gated) or
require any Stage 20E physical evidence to make progress. Starting
Stage 21 does not change Stage 20's own status; see §0 below.

## 0. Status of Stage 20 while Stage 21 begins

Restated, unchanged by this document:

- **Stage 20C2:** Planned - externally gated on Apple Developer
  signing/notarization credentials the operator does not currently
  have.
- **Stage 20E:** In progress - physical/manual verification deferred
  by the operator. Sessions A/B/C/D/E/F/G/K/L/M remain **Pending -
  operator deferred physical verification**; H/I/J remain **Not
  verified - environment unavailable**. None of this is PASS, N/A, or
  deleted.
- **Stage 20 overall:** Incomplete.

Stage 21 is authorized, additive product development that does not
require any of the above to close first.

## 1. Why Stage 21 exists

Every concept a first-run user needs already exists in the product:
Streaming Tree is a standalone companion application (never an OBS
plugin, never touching OBS's own install), a local ingest engine
(MediaMTX) must be ready before OBS can connect, OBS is pointed at a
local RTMP address, destinations are configured independently of
provider account connections, and overlays have their own public
Browser Source URLs. None of this is new architecture. Stage 21 turns
what already exists into a coherent first-run experience - discovery
and explanation, not new capability.

## 2. Audit of the real current product (before design)

Confirmed by direct source reading:

- **Routing** (`apps/web/src/App.tsx`): a flat `react-router-dom`
  route table. Every management route renders inside `AppShell`
  except the visual Designers and the public overlay routes, which
  opt out of `AppShell` entirely for full-viewport/standalone control
  (`AlertDesignerPage`, `ChatOverlayDesignerPage`, `OverlayChatPage`,
  `PublicAlertPage`, `PublicAudioPage`, `PublicWidgetPage` - each with
  its own doc comment explaining why). `/platforms` and `/metadata`
  are still real placeholder routes (`PlannedPages.tsx` →
  `PlaceholderPage`) - **onboarding must never link to either**; the
  actual destination-CRUD surface lives on the Dashboard itself via
  `AddPlatformDialog`.
- **Persisted preferences precedent** (`internal/domain/
  updatersettings`, `internal/domain/operatorchatprefs`): the
  established pattern for a small, singleton, restart-surviving
  preference is a Go domain package (`model.go`/`service.go`/
  `repository.go`/`errors.go`), a SQLite repository under
  `internal/storage/sqlite`, and a migration creating a
  `CHECK (id = 1)` singleton-row table with `created_at`/`updated_at`,
  written via `INSERT ... ON CONFLICT (id) DO UPDATE`. `Service.
  Preferences()` returns a documented `Default()` when no row has ever
  been written; `ReplacePreferences` is a full-replacement write, never
  a partial patch. This is the model Stage 21's own onboarding-state
  persistence follows (§4).
- **Local ingest / OBS connection data** (`GET /api/runtime`,
  `internal/runtime/mediamtx.Snapshot`): already carries everything
  the OBS Connection Assistant needs - `Ingest.State` (`unavailable`/
  `waiting`/`receiving`/`error`, real, polled from MediaMTX's own
  Control API, no new backend logic needed), and `Connection.
  {ServerURL,StreamKey,PublishURL}`. The local ingest path
  (`Connection.StreamKey`) is **explicitly documented as a route
  identifier, not a secret** (`docs/project-overview.md` §10,
  `apps/web/src/i18n/resources/en/runtime.json`'s own
  `connection.notASecret` string, already shown today in
  `SidebarFooter`) - it needs no reveal/secret UX, only the plain
  `CopyableValue` component already built for exactly this data
  (`apps/web/src/components/runtime/CopyableValue.tsx`, already used
  by `SidebarFooter.tsx` for this exact field). Reused as-is, not
  reimplemented.
- **MediaMTX/FFmpeg readiness**: `useRuntimeQuery()`
  (`hooks/use-runtime.ts`, `mediaMtx.state`/`lastError`) and
  `useFfmpegRuntimeQuery()` (`hooks/use-branches.ts`,
  `api/branch-schemas.ts`'s `FFmpegStatus`) are the real, already-
  polled, already-presented (`RuntimeControls`, `runtime-presentation.
  ts`'s `mediaMtxStateKey`/`mediaMtxTone`) sources of truth. Stage 21
  reuses these hooks and presentation helpers directly; it does not
  duplicate readiness logic in a second place.
- **Destinations**: `usePlatformsQuery()`/`usePlatformDefinitionsQuery()`
  (`hooks/use-platforms.ts`) and `AddPlatformDialog` (`components/
  platforms/AddPlatformDialog.tsx`) are the real, tested creation flow
  already used by the Dashboard. Stage 21 opens the same dialog
  component; it does not build a second destination form.
- **Connected accounts**: `GET /api/connected-accounts`
  (`AccountService.ListAccounts`, `httpapi/accounts.go`) is
  independent of a destination's stream key by architecture (§8.1 of
  `docs/project-overview.md`) - a connected account authorizes
  chat/event/account-aware functionality; a destination's stream key
  is what video is sent with. Already two clearly separate concepts in
  the domain model; onboarding explains the distinction, it does not
  invent it.
- **Creator tools / overlays**: Alerts, the Chat Overlay Designer,
  Goals/widgets, and Audio/TTS are real, shipped features (README's
  own "Project state" summary; none are `planned: true` in
  `nav-items.ts` except `/platforms`/`/metadata`). A chat overlay's
  public Browser Source URL is already built by `OverlayUrlPanel`
  (`components/overlays/OverlayUrlPanel.tsx`,
  `${origin}/overlay/chat/${publicSlug}`) - reused as-is for §9's
  overlay step, never a second URL-construction implementation. The
  remote/D2C overlay capability plane (`RemoteOverlayPanel`) is out of
  scope for onboarding entirely - never surfaced there, matching the
  governing task's own explicit "respect the D2C local/remote overlay
  security design."
- **Existing-user migration signal**: seed migration `0002_seed_
  default_platforms.sql` creates exactly four platforms, always
  `enabled = 0`, and never touches `platform_output_settings` or
  `connected_accounts`. This is a real, non-fragile signal a fresh
  database can be told apart from a used one by, at migration time
  (§4.3) - not "platform count > 0" (which the seed alone already
  satisfies) and not a single brittle heuristic.
- **i18n**: `apps/web/src/i18n/resources/{en,pl}/*.json`, one
  namespace per feature area, validated by `npm run i18n:check`. Stage
  21 adds one new namespace, `onboarding`, in both languages.
- **Settings entry point precedent**: `SettingsPage.tsx`'s own
  About & Legal card (a `Panel` with a `Link` row, icon, heading,
  description, chevron) is the established pattern for "a settings
  page section that navigates to a dedicated route" - reused for the
  onboarding reopen affordance (§10).

## 3. Substage decomposition

- **21A** - persisted onboarding state: backend domain, migration,
  API, frontend schema/hook, existing-user migration rule, dev/test
  seams. No UI yet.
- **21B** - the onboarding route/flow shell: routing, step framework,
  accessibility scaffold, responsive layout, Welcome step, entry/
  reopen affordances (Settings card, Dashboard setup-incomplete
  affordance), skip/complete semantics, Dashboard integration,
  installer→first-launch coherence.
- **21C** - readiness and OBS connection steps: local engine
  readiness (MediaMTX/FFmpeg), the OBS Connection Assistant (real
  server/stream-key values, real ingest-state detection, OBS
  instructions), destinations summary + add action, connected-accounts
  distinction step.
- **21D** - creator tools discovery step + final readiness summary.
- **21E** - hardening: full test suite (frontend/backend/integration/
  packaged-runtime), localization parity, accessibility pass,
  responsive verification, documentation close-out.

Each substage is independently useful and independently verifiable;
none is a placeholder for a later one to retroactively justify.

## 4. 21A - persisted onboarding state

### 4.1 Domain model (`apps/server/internal/domain/onboarding`)

```go
type Status string

const (
    StatusPending   Status = "pending"   // never engaged - auto-show
    StatusCompleted Status = "completed" // finished the flow
    StatusDismissed Status = "dismissed" // explicitly skipped, or a
                                          // migrated pre-Stage-21 install
)

const CurrentSchemaVersion = 1

type State struct {
    Status        Status
    SchemaVersion int
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

func Default() State // Status: StatusPending, SchemaVersion: CurrentSchemaVersion
```

One status value, not independent `completed`/`dismissed` booleans -
matching this codebase's own repeated, deliberate pattern (`mediamtx.
ProcessState`, `branch.State`, `mediamtx.IngestState`): "completed and
dismissed" must be unrepresentable. `SchemaVersion` exists so a future
*major* onboarding revision can deliberately re-offer itself (by
migrating rows with an older `SchemaVersion` back to `StatusPending`)
without nagging every already-onboarded user on an ordinary feature
addition - not exercised in Stage 21 itself, since `CurrentSchemaVersion`
starts at `1` with nothing yet to bump against.

No secret of any kind is ever stored here - matching every other
preferences domain in this codebase.

`Service.State(ctx)` returns the singleton row or `Default()` if none
exists. `Service.SetStatus(ctx, Status)` is a full-replacement write
(never a partial patch), validating the status is one of the three
known values.

### 4.2 Persistence

New migration `0029_onboarding_state.sql`, following `0027_update_
preferences.sql`'s exact singleton-row shape:

```sql
CREATE TABLE onboarding_state (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    status         TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'dismissed')),
    schema_version INTEGER NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
```

### 4.3 Existing-user migration rule

The same migration seeds the singleton row's initial `status` from
real persisted signals, computed once, in SQL, inside the same
migration transaction - never a second later heuristic and never
inferred from the frontend:

```sql
INSERT INTO onboarding_state (id, status, schema_version, created_at, updated_at)
SELECT 1,
    CASE WHEN EXISTS (
        SELECT 1 FROM connected_accounts
        UNION ALL
        SELECT 1 FROM platform_output_settings
        UNION ALL
        SELECT 1 FROM platforms WHERE enabled = 1
        UNION ALL
        SELECT 1 FROM platforms WHERE id NOT IN
            ('pf_seed_twitch', 'pf_seed_youtube', 'pf_seed_kick', 'pf_seed_tiktok')
    ) THEN 'dismissed' ELSE 'pending' END,
    1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now');
```

Rule, stated plainly: any real prior use - a connected account, a
configured output server, an enabled seed platform, or any
user-created platform beyond the four disabled seed rows - marks the
database as belonging to an existing user (`dismissed`: onboarding
stays available, never auto-shown). A database where literally nothing
has ever been touched beyond the one-time seed starts `pending`
(onboarding auto-shows once, on the next load). This runs once, for
every database (fresh or years-old) that applies this migration -
exactly the same "runs once, ever" guarantee every other migration in
this project already has.

### 4.4 API

```
GET /api/onboarding
PUT /api/onboarding   { "status": "pending" | "completed" | "dismissed" }
```

Mirrors `GET/PUT /api/updates`'s own preferences shape: a small,
versioned response (`{"version":1,...State}`), unknown-field rejection
on the PUT body (matching every other write endpoint's convention),
`400` for an unrecognized status string.

### 4.5 Frontend

`apps/web/src/api/onboarding-schemas.ts` (Zod), `apps/web/src/api/
onboarding.ts` (fetch functions), `apps/web/src/hooks/use-onboarding.ts`
(`useOnboardingStateQuery`/`useSetOnboardingStatusMutation`, the same
`useQuery`/`useMutation`+`invalidateQueries` shape every other hook in
`hooks/` already uses).

### 4.6 Development/test seams

No special-cased "test mode" branch in production code. Every
frontend/backend/integration test drives the real `GET`/`PUT` endpoints
against a real (in-memory or hermetic file) SQLite database, exactly
like every other persistence test in this codebase - `npm run dev`
behaves identically to production because there is no separate code
path to diverge. A frontend component test that needs a specific
onboarding status mocks the query response the same way every other
existing page test already mocks its own queries (`msw`/manual mock,
whichever this codebase's existing test setup already uses - confirmed
during implementation, not assumed here).

## 5. 21B - flow shell, entry points, skip/complete semantics

### 5.1 Surface: a dedicated route, not a modal

`/onboarding`, registered in `App.tsx` alongside the Designer/public
overlay routes - **outside** `AppShell`'s sidebar/nav chrome (a
first-run user should not have to parse the full navigation while
being onboarded) but with its own minimal, consistent header (brand
mark, a step indicator, an always-visible "Skip setup" action) and
real page-level headings per step, exactly the accessibility trade-off
`docs/onboarding.md` §2's routing audit above found already used for
the Designers - a dedicated route gives correct heading structure,
natural focus order, and no z-layer/focus-trap complexity a giant
modal would need instead (per the governing task's own explicit
accessibility instruction).

### 5.2 First-run detection and auto-show

On app load, once `GET /api/onboarding` resolves: `status === 'pending'`
navigates to `/onboarding` once; `completed`/`dismissed` never
auto-navigates. Never inferred from `localStorage`, absent
destinations, or MediaMTX state - only the persisted status (§4).

### 5.3 Skip/complete semantics

Completing means the flow was finished; skipping means the operator
explicitly declined it. Neither requires: an active stream, provider
OAuth, overlays, donations, TTS, goals, or any specific destination
count - zero configured destinations is a valid, complete-able state.
Both set a terminal status (`completed`/`dismissed`) via `PUT /api/
onboarding`, and neither can be silently undone by app restart or
routine navigation - only an explicit future "restart setup assistant"
action (not required by Stage 21) could ever move status back to
`pending`.

### 5.4 Reopen entry points

- **Settings**: a new card, styled exactly like the existing About &
  Legal card in `SettingsPage.tsx` (`Panel` → `Link` row, icon,
  heading, description, chevron), navigating to `/onboarding`.
- **Dashboard**: a small, dismissible, non-modal affordance ("Setup
  incomplete - Continue setup") shown only while real readiness gaps
  exist (per §7's summary categories) and status is not `completed` -
  never a large persistent Dashboard region, matching the governing
  task's explicit "do not permanently consume a large area."

### 5.5 Installer → first-launch coherence

No new installer-side state. A fresh per-user install has no database
yet; the first real backend start runs every migration including
0029, landing on `pending` (§4.3), so the app's own already-implemented
first-launch browser-open naturally lands the user on `/onboarding`
with zero installer-specific code. An update/repair preserves the
existing database (`docs/windows-packaging.md` §13-§15, unchanged),
so `onboarding_state`'s already-migrated status survives exactly like
every other table - an update never re-forces onboarding on an
existing user.

## 6. 21C - readiness and OBS connection steps

### 6.1 Welcome step

Explains, briefly: OBS → Streaming Tree for OBS → destinations
(Twitch/YouTube/Kick/TikTok). Never leads with MediaMTX/FFmpeg/port
numbers/Go server terminology (secondary detail only, where relevant
in the readiness step). Explicitly states Streaming Tree is a
standalone companion application, never an OBS plugin, and never
touches the OBS installation - reusing this project's own existing,
already-reviewed wording precedent (`docs/windows-packaging.md` §11's
"never asks 'where is OBS installed'" framing) rather than writing a
new claim from scratch. No fake screenshots, stream stats, or
providers.

### 6.2 Local engine readiness step

Reuses `useRuntimeQuery()` (MediaMTX state/errors) and
`useFfmpegRuntimeQuery()` (FFmpeg capability status) directly - no new
frontend readiness logic. Missing MediaMTX surfaces the existing
`RequestInstall`/`RuntimeControls` action (§2's audit: the one
canonical install action); missing/incompatible FFmpeg surfaces the
existing, already-honest remediation copy this project already ships
elsewhere (`docs/project-overview.md` §7.5's own FFmpeg-resolution
documentation is the source of truth for that copy, not an invented
second explanation).

### 6.3 OBS Connection Assistant step

Reuses `Connection.ServerURL`/`Connection.StreamKey` from `GET /api/
runtime` via the existing `CopyableValue` component (§2's audit - not
a secret, no reveal UX needed, matching the field's own documented
classification). Short, current, verified-before-shipping OBS
instructions (Settings → Stream → Service: Custom → Server/Stream Key
→ Apply) - no obs-websocket requirement, no OBS plugin, no OBS config
file modification. Real ingest-state detection via `Ingest.State`
(`unavailable`/`waiting`/`receiving`/`error`, already polled by
`useRuntimeQuery()` at the interval `pollIntervalFor` already computes)
- "Waiting for OBS...", "OBS connected", using the same
`ingestStateKey`/`ingestTone` presentation helpers `SidebarFooter`
already uses, never a fake always-succeeding "Test connection" button
and never process-name inspection.

### 6.4 Destinations step

Reuses `usePlatformsQuery()` for the summary list (provider, enabled/
disabled, real branch state via the existing branch hooks where
useful) and opens the existing `AddPlatformDialog` for the add action -
no second destination form. Zero, one, or many destinations are all
valid; onboarding completion never requires a specific count.

### 6.5 Connected accounts step

Explains the destination-vs-account distinction using this project's
own existing domain language (§8.1 of `docs/project-overview.md`:
"where the video is sent" vs "chat/event/account-aware functionality"),
lists `GET /api/connected-accounts`, links to the existing account
management UI (`SettingsPage`'s `ConnectedAccountsPanel`/
`YouTubeAccountsPanel`) rather than embedding a second OAuth flow.
Never forces a device-flow/OAuth connection to finish onboarding. Only
providers with a real, shipped account integration are shown as
connectable - never a feasibility-gated future connector (Kick, TikTok)
presented as available.

## 7. 21D - creator tools + final summary

### 7.1 Creator tools discovery step

Concise links to real, shipped features only (Alerts, Chat Overlay
Designer, Goals/widgets, Audio/TTS - confirmed real in §2's audit).
Where a chat overlay profile already exists, `OverlayUrlPanel` is
reused for its "Copy Browser Source URL" action; where none exists yet,
onboarding directs the user to create one first rather than inventing
a URL. Discovery only - not a second configuration surface for every
subsystem.

### 7.2 Final readiness summary

Real-state categories, never fabricated: Application (ready/issue),
OBS ingest (connected/not currently connected, from `Ingest.State`),
Destinations (N configured/enabled/active, from the real platform +
branch state), Connected accounts (N connected). Distinguishes
required-for-streaming vs optional vs currently-inactive - optional
gaps are never rendered as failures. Answers, from real state alone:
"what do I still need before I can stream?"

## 8. Security

Every value onboarding surfaces is either already-public runtime
metadata (ingest state, MediaMTX version) or the local ingest route
identifier already classified as non-secret (§2). Onboarding never
requests, displays, or has access to a destination stream key, an
OAuth token, a remote-management session, or a remote-overlay
capability token - none of those are reachable from the read-only
service interfaces onboarding's own hooks depend on (`RuntimeService`,
`BranchRuntimeService`, `PlatformService`, `AccountService` as already
scoped in `httpapi`), matching this project's existing "the HTTP layer
has no method that returns a secret" architecture (`docs/project-
overview.md` §10.4).

## 9. Accessibility and responsive design

Logical focus order and visible focus per step; each step is a real
heading, not a `div` styled to look like one; status communicated by
text plus icon, never color alone; `aria-live` for the ingest-
connection state change; no keyboard trap; Escape behavior matches
this codebase's existing dialog/page conventions (a route, not a
modal, so no Escape-to-close ambiguity - §5.1). Verified at
2560/1920/1600/1440/1366/1280/1024 and up to 150% zoom, reusing the
existing design primitives (`Panel`, `Button`, `AppShell`-adjacent
layout patterns) rather than inventing new ones, following the same
discipline Stage 20E's own Dashboard responsive-grid remediation
already established.

## 10. Localization

English is canonical; Polish ships at parity, validated by the
existing `npm run i18n:check`. One new namespace, `onboarding`, in
both `apps/web/src/i18n/resources/{en,pl}/`.

## 11. Testing (see §33-§36 of the governing task for the full list)

Frontend: behavior tests, not snapshot tests, covering first-run
detection, the migration rule's effect, skip/reopen, step navigation,
real readiness states (MediaMTX/FFmpeg/ingest, each state), zero/one/
many destinations, the account-vs-destination distinction, copy
actions and clipboard-failure fallback, the final summary, and
English/Polish parity. Backend: `internal/domain/onboarding` unit
tests (default state, persistence, restart survival via a real
repository test, validation, unknown status rejection) plus the
migration's existing-user rule exercised directly against a seeded
database. Integration: extends the appropriate canonical script to
prove fresh state → onboarding API available → readiness loaded →
status persisted → restart → status survives, entirely hermetic, no
real provider stream required. Packaged runtime: extends
`verify-packaged-app.mjs` (or the narrowest appropriate addition) to
prove the route/API exist against the real embedded production
frontend, using the existing hermetic data-dir mechanism - never the
operator's real `%AppData%\StreamingTree`.

## 12. Documentation

This document is Stage 21's own contract. `README.md`/`docs/project-
overview.md` gain a brief, honest "Stage 21 (in progress)" mention once
substantial work lands - never aspirational copy describing unshipped
behavior. `docs/progress.md` records each logical commit as it always
does; this document is updated in place as substages complete (it is
living documentation, not a journal).
