# Dashboard visual alignment - design contract (Stage 20E)

**Written:** 2026-08-28, before the Stage 20E visual-realignment implementation
described below. This is the narrow, implementation-grounded contract required
before touching `DashboardPage.tsx` and its children - it exists so the
redesign is judged against the real current architecture and the real current
backend contract, not against an old mockup screenshot.

## 0. What triggered this

The operator described (textually; no image data reached this session) an
older product mockup next to the Dashboard as it exists today, and asked for
the Dashboard to move back toward that direction: a polished streaming
control center, not an internal admin panel. The mockup is a **visual/product
direction reference only**. It is not a data source, and nothing in it
overrides real backend state. Where the mockup shows something the backend
cannot currently produce (viewer counts, live thumbnails, bitrate/resolution/
FPS, host CPU/memory/disk), that is not built - see §5.

## 1. Audit of the real current implementation (2026-08-28)

Read in full before any code changed:

- `DashboardPage.tsx` - already a two-region grid
  (`xl:grid-cols-[minmax(0,1fr)_20rem]`): main column (platform grid +
  metadata workspace) + a fixed 20rem `SystemStatusRail`.
- `AppShell.tsx` / `Sidebar.tsx` / `SidebarFooter.tsx` - the sidebar is
  **already** the brand/nav/OBS-connection left rail the target layout
  calls for: `BrandMark` (real emblem + "Streaming Tree" / "for OBS"),
  `SidebarNav` (real routes, real `planned` badges), and `SidebarFooter`
  (real local-ingest state from `GET /api/runtime` - MediaMTX state,
  OBS/ingest state, server address + stream key with copy buttons; no
  fake bitrate/resolution/FPS, per its own doc comment). This already
  satisfies §20-21 of the governing brief; no ingest-panel rebuild was
  needed, only the Dashboard's own right rail and card grid.
- `PlatformGrid.tsx` / `PlatformCard.tsx` - the destination-card grid.
  Provider identity was a plain coloured-text tile (`PlatformGlyph` +
  `providerGlyphClass`), by a **deliberate prior decision** ("the project
  does not ship third-party trademarks" - see the old doc comment). That
  decision is superseded by this stage; see §4 and `docs/provider-branding.md`.
  Grid was capped at `sm:grid-cols-2` regardless of viewport width.
- `SystemStatusRail.tsx` - three stacked cards: `BackendHealthCard`
  ("Backend" / "Go REST API" / Service / Version / Uptime - real data,
  but developer-facing framing), `StreamCountersCard` (real configured/
  enabled/disabled counts, but its `noRuntimeState` copy falsely claimed
  "the streaming engine is not implemented yet" - stale; the branch
  runtime engine has existed since an earlier stage, see
  `use-branches.ts` / `BranchControls.tsx`), `ResourcesCard` (100% static
  `DEMO_RESOURCE_METRICS` / `DEMO_NETWORK_STATUS` from `demo-system.ts` -
  fake CPU/memory/disk/network numbers, honestly labelled "Demo" but
  still fake, and the backend does not collect host metrics).
- `MetadataEditor.tsx` / `PlatformTabs.tsx` / `MetadataForm.tsx` - already
  a compact tabbed workspace (Panel + `PlatformTabs` + capability-driven
  form), not the "large engineering-style editor" the brief warned about.
  No structural change needed; only spacing polish and a provider-brand
  icon on each tab (§4).
- `PlaceholderPage.tsx` / `PlannedPages.tsx` - `/platforms` and
  `/metadata` routes are genuinely still placeholder pages for **future,
  narrower** sub-features (per-branch encoding profiles; reusable
  metadata presets/history). Their copy already says the general
  functionality "already works from the Dashboard's platform cards" -
  this is accurate, not stale, and their `planned` nav badges stay.
- `AboutLegalPage.tsx` / `useAboutQuery` / `GET /api/about` - the real,
  already-existing authoritative source for product version/build
  identity (`AboutResponse.version`, `isReleaseBuild`, `commit`,
  `commitDirty`). `apps/web/src/data/app-info.ts` separately hardcodes
  `version: '0.1.0'` and is the only place that string is used
  (`SidebarFooter`'s `navigation:version` line) - stale, fixed in this
  stage (§6).
- `LogsPage.tsx` - the real Stage 20E diagnostics surface
  (`GET /api/logs`, support-bundle export). The natural home for
  `BackendHealthCard`'s Service/Version/Uptime detail once it leaves the
  Dashboard.

## 2. Backend/API data actually available (audited, not assumed)

| Concern | Real source | Notes |
|---|---|---|
| Destination/branch runtime state | `GET /api/branches` (`useBranchRuntimeQuery`) | `idle/blocked/waiting_for_ingest/starting/live/restarting/stopping/error`, blockers, progress (outTimeMs/totalSize/speed) |
| Destination configuration | `GET /api/platforms` | enabled/disabled, display name, sort order, provider id |
| Local ingest / MediaMTX | `GET /api/runtime` | real state only; no bitrate/resolution/FPS is ever reported |
| FFmpeg dependency | `GET /api/ffmpeg-status` | capability flags, detected/minimum version |
| Backend health | `GET /api/health` | status/service/version/uptime |
| Product/build identity | `GET /api/about` | version, isReleaseBuild, commit, commitDirty |
| Host CPU/memory/disk/network | **none** | not collected anywhere on the backend; `ResourcesCard`'s numbers were static constants |
| Viewer counts / stream thumbnails | **none** | never fabricated anywhere in this codebase |

Any Dashboard element not backed by a row in this table is either removed
(`ResourcesCard`) or built from a real row above.

## 3. Target information architecture

The left/main/right hierarchy the brief asked for **already exists** at the
`AppShell` level (sidebar = left rail). What changes is scoped to
`DashboardPage`'s own two regions:

- **Main column**: platform/destination cards (primary content, responsive
  grid, real provider branding - §4), then the Metadata & Settings
  workspace directly below (unchanged structure, spacing polish only).
- **Right rail** (`SystemStatusRail`): becomes a compact, honest
  **Overall Stream Status** summary built from real branch state
  (configured / enabled / live / starting / waiting / error counts,
  reusing the existing green/blue/red/grey semantic vocabulary from
  `StatusBadge`) instead of the old "configured only" counters plus a
  disclaimer that used to be false. The developer-facing Backend card and
  the fake Resources card are removed from this rail entirely (§5).

No new three-region grid, no sidebar rebuild, no new page-level layout
primitive - the existing `AppShell` + `DashboardPage` grid already matches
the target shape once the right rail's content is corrected.

## 4. Provider branding

Covered in full in `docs/provider-branding.md`. Summary: a new
`ProviderBrand` component (`apps/web/src/components/providers/ProviderBrand.tsx`)
resolves Twitch/YouTube/Kick/TikTok to a real, locally-vendored brand mark in
a rounded, provider-accented tile, and falls back to the existing neutral
text tile (`PlatformGlyph`) for any other/unknown provider id - never a
crash, never a blank tile. Adopted in `PlatformCard`, `PlatformSettingsDialog`,
`PlatformTabs`, and `StreamsPage`'s branch table (the surfaces the brief
names). The public, viewer-facing chat/overlay glyphs
(`ChatSourceLabel`, `OverlaySourceMarker`) are deliberately left on their
existing neutral text-glyph treatment - that is a distinct, previously
deliberate decision for content rendered live inside a stream/browser
source, not a Dashboard/product-UI surface, and is out of scope here.

## 5. Removed: fake host-resource metrics

`ResourcesCard.tsx`, its only two consumers `Meter.tsx` and `DemoBadge.tsx`,
and `demo-system.ts`'s `DEMO_RESOURCE_METRICS`/`DEMO_NETWORK_STATUS` are
deleted outright. The backend does not collect host CPU/memory/disk/network
data (confirmed in §2), so there is no honest way to keep this card. No new
telemetry subsystem is built to backfill it - per the brief's own explicit
non-goal.

## 6. Version display

`APP_INFO.version` (`apps/web/src/data/app-info.ts`) is removed. Its one
caller, `SidebarFooter`, now reads the real build identity from
`useAboutQuery()` (`GET /api/about`), the same source `AboutLegalPage`
already uses, formatted with the same `about:product.*` i18n keys
(`versionLabel`/`developmentBuild`/`commit`/`commitDirty`) so a manual/test
build honestly shows its development-build+commit identity instead of a
hardcoded "0.1.0".

## 7. Stale-copy corrections

- `dashboard:counters.noRuntimeState` ("No live state is shown: the
  streaming engine is not implemented yet...") - false since the branch
  runtime engine exists; replaced by a real live/starting/waiting/error
  breakdown.
- `dashboard:counters.description` ("Read from the local database") -
  removed as unnecessary implementation detail for a Dashboard summary
  card.
- `dashboard:backend.*` - the card itself relocates to `LogsPage`
  (diagnostics is the right home for "Go REST API" / Service / Version /
  Uptime); the Dashboard no longer shows this framing at all.

## 8. Non-goals (unchanged from the governing brief)

No fake viewer counts, thumbnails, or resource metrics. No telemetry. No
runtime CDN logo fetching. No deleting real nav features
(Chat/Overlays/Automation/Alerts/Audio/Goals) to match the old mockup. No
Electron/Tauri/native-window rework. No backend changes - every item above
is achievable from data the backend already exposes.
