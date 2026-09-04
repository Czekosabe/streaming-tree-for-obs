# Development

Building and running Streaming Tree for OBS from source: requirements,
the two-process development workflow, data storage, production builds,
linting/testing, the REST API, and the repository layout.

---

## Requirements

There are two different audiences here, and they need different tools.

### Developer/build requirements

Building Streaming Tree for OBS from source, or running the two-process
development workflow below, needs:

| Tool | Version | Purpose | Needed now? |
| ---- | ------- | ------- | ----------- |
| **Node.js** | 20.19+ or 22.12+ (22 LTS or newer recommended) | running/building the React panel | yes |
| **npm** | 10+ | installing frontend dependencies | yes |
| **Go** | 1.25 or newer | building and running the backend (`go.mod` pins the floor) | yes |
| Inno Setup 6 | — | building the Windows installer (`scripts/build-release.ps1`) | only for producing a release build, see [windows-packaging.md](windows-packaging.md) |

Checking the installed versions:

```bash
node --version
npm --version
go version
```

> **Note about the Node version.** The project is configured so that it also
> works on Node 22.11. If your Node is older than 22.12, an upgrade is still
> recommended: newer frontend tooling (Vite 7/8, jsdom 30) requires Node
> `^20.19 || >=22.12`, and older versions silently skip their native optional
> dependencies. Details are in [`docs/progress.md`](progress.md).

If you do not have Go yet, download it from <https://go.dev/dl/> and run the
installer for your system. It adds `go` to `PATH`; open a **new** terminal
window afterwards.

### Packaged Windows user requirements

Running the **packaged Windows release** (built via
[windows-packaging.md](windows-packaging.md)) needs **none of the
above** - no Node.js, no npm, and no Go installation. Install it, launch it,
and it opens your default browser to the local management UI on its own.

Both audiences still need the following, regardless of how the application
itself was obtained - these are not build tools, they are what the
application actually does its work with:

| Tool | Version | Purpose | Needed now? |
| ---- | ------- | ------- | ----------- |
| OBS Studio | 30+ | the source of the stream | yes, to actually publish something — the backend runs without it |
| MediaMTX | — | receiving the RTMP stream | yes — installed and supervised automatically, see [Local ingest with MediaMTX](connecting-platforms.md#local-ingest-with-mediamtx) |
| FFmpeg | a recent build (4.4+ floor; actual compatibility is capability-probed, not version-matched) | sending each destination branch | yes, to actually start a destination — see [Outgoing streaming with FFmpeg](connecting-platforms.md#outgoing-streaming-with-ffmpeg). The application starts and the rest of the interface works without it, packaged or not. |

---

## Quick start

### Development workflow (from source)

The application consists of two processes, started in **two separate
terminals**.

**Terminal 1 — backend:**

```bash
cd apps/server
go run ./cmd/server
```

**Terminal 2 — frontend:**

```bash
cd apps/web
npm install
npm run dev
```

Then open <http://localhost:5173>.

The panel also works **without the backend running** — the system status section
then shows a clear "Backend unavailable" message and the rest of the interface
keeps working.

This two-process workflow remains fully supported for development and stays
that way for the foreseeable future - Stage 20A's packaged build is
additive, not a replacement for it.

### Packaged Windows release

Stage 20A implements a real single-launch Windows packaging: one Go process
serving the production frontend, no separate frontend process, no Node/npm/Go
installation required for the end user. See
[`docs/windows-packaging.md`](windows-packaging.md) for the full
architecture (production routing, packaged-mode lifecycle, the Inno Setup
installer). Building a local release from source:

```powershell
powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+local"
```

produces an unsigned installer under `build/release/output/` (local build
artifacts only - nothing is published, tagged, or released by this script).
Installing and running it opens your default browser to the local management
UI once the application is ready; "Quit Streaming Tree" in **Settings →
About & Legal** stops it cleanly. The same page's **Updates** panel checks
GitHub for a newer Stable release and, with explicit confirmation, installs
it and restarts - see [`docs/updater.md`](updater.md).

---

## Frontend — install and run

### Installing dependencies

```bash
cd apps/web
npm install
```

Run this once, and again after any dependency change. Dependencies land in
`apps/web/node_modules`, which is not version-controlled.

### Running in development mode

```bash
npm run dev
```

The dev server starts at <http://localhost:5173> and reloads the application on
every code change. Requests to `/api` are proxied to the backend at
`http://127.0.0.1:8080`.

Stop it with `Ctrl + C`.

### Configuration (optional)

The defaults are enough for local work. If the backend runs at a different
address, copy `apps/web/.env.example` to `apps/web/.env.local` and adjust the
values.

> **Never put secrets in frontend `.env` files.** Everything prefixed with
> `VITE_` is compiled into the public JavaScript bundle and is visible to anyone
> who opens the page.

### Typography and static assets

The UI font is the native system-font stack (`--font-sans` in
`apps/web/src/index.css`: `ui-sans-serif, system-ui, -apple-system,
"Segoe UI", Roboto, Helvetica, Arial, sans-serif`) - deliberately never
a remote or bundled web font. This local-first desktop application must
render correctly with no internet access, on a clean OS install, using
only whatever real UI font that platform already ships; it holds itself
to the same "no arbitrary/remote font" principle the public-overlay
visual-design contract already commits to (`docs/visual-designs.md`).
An earlier, undocumented `'Inter'` entry predated this decision and was
never actually loaded anywhere (no `@font-face`, no bundled font file,
no CDN link) - it silently fell through to this same fallback stack on
every machine that didn't happen to already have Inter installed for
an unrelated reason, until a real-browser CI run on headless Linux
caught the one environment where that fallback produced a visibly
different, more cramped layout. Removed for that reason.

The same rule applies to every other static UI asset: the brand
logo/emblem (`src/assets/brand-emblem.png`) and the four provider brand
marks (Twitch/YouTube/Kick/TikTok, inline SVG path data in
`src/components/providers/ProviderBrand.tsx` - see its own doc comment
and `docs/provider-branding.md` for provenance) are bundled through
Vite's normal asset pipeline or embedded directly in the component,
never loaded from an external host. `apps/web/e2e/specs/
typography.spec.ts` proves this contract in a real browser: no font
network request is ever made, the app shell renders with all outbound
internet access blocked, `document.fonts` settles without ever
resolving the declared family to `"Inter"`, and a representative
heading (the exact element a prior CI run found collapsed to a
sub-pixel width) renders with real, non-zero content width.

---

## Go backend — running it

### Running without building an executable

```bash
cd apps/server
go run ./cmd/server
```

The console prints a line confirming that it is listening:

```
level=INFO msg="http server listening" service=streaming-tree-server version=0.1.0 address=127.0.0.1:8080
```

Stop it with `Ctrl + C`. The server shuts down gracefully, waiting for in-flight
requests to finish (up to 10 seconds).

### Checking the health endpoint

```bash
curl http://127.0.0.1:8080/api/health
```

Example response:

```json
{
  "status": "ok",
  "service": "streaming-tree-server",
  "version": "0.1.0",
  "uptimeSeconds": 12.34,
  "time": "2026-08-03T11:36:38Z"
}
```

On Windows without `curl`, use PowerShell:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/health
```

### Configuration through environment variables

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `STREAMING_TREE_HOST` | `127.0.0.1` | Interface to bind to. Loopback only by default, so the server is not exposed to the local network by accident. |
| `STREAMING_TREE_PORT` | `8080` | REST API port. |
| `STREAMING_TREE_ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Comma-separated list of origins accepted by CORS. |
| `STREAMING_TREE_DATA_DIR` | per-user config directory | Application data directory: database, managed MediaMTX and generated configuration. See [Data storage](#data-storage). |
| `STREAMING_TREE_DB_PATH` | — | Full path to the SQLite file. Takes precedence over `STREAMING_TREE_DATA_DIR` for the database only. |
| `STREAMING_TREE_MEDIAMTX_PATH` | — | Full path to a MediaMTX executable you provide. Skips the managed installation. Must report the supported version. |
| `STREAMING_TREE_FFMPEG_PATH` | — | Full path to an FFmpeg executable you provide. Skips the `PATH` search. Must pass every capability probe. See [Outgoing streaming with FFmpeg](connecting-platforms.md#outgoing-streaming-with-ffmpeg). |
| `STREAMING_TREE_MEDIAMTX_AUTOSTART` | `true` | Start MediaMTX when the backend starts. |
| `STREAMING_TREE_MEDIAMTX_AUTO_RESTART` | `true` | Restart MediaMTX automatically after an unexpected exit. |
| `STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` | `127.0.0.1:1935` | Address OBS publishes to. **Loopback only.** |
| `STREAMING_TREE_MEDIAMTX_API_ADDRESS` | `127.0.0.1:9997` | MediaMTX Control API address, read only by the backend. **Loopback only.** |
| `STREAMING_TREE_INGEST_PATH` | `live` | The single path publishing is allowed on. Letters, digits, `-` and `_` only. |
| `STREAMING_TREE_TWITCH_CLIENT_ID` | — | Twitch application Client ID. Always wins over a database-managed value if set. Never a client secret — see [Connected accounts and Twitch metadata](connecting-platforms.md#connected-accounts-and-twitch-metadata). |
| `STREAMING_TREE_YOUTUBE_CLIENT_ID` | — | Google OAuth Desktop-app Client ID. Always wins over a database-managed value if set, independently of the Twitch variable above. Never a client secret — see [Connected accounts and YouTube metadata](connecting-platforms.md#connected-accounts-and-youtube-metadata). |
| `STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE` | `1000` | The Engagement Event Bus's in-memory retained-event capacity. Must be between 100 and 10000; an out-of-range or non-numeric value is a startup error. See [Engagement Event Bus and Twitch chat/events](engagement-architecture.md). |
| `STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE` | `500` | The unified operator-chat projection's in-memory retained-item capacity — independent of the Event Bus's own. Must be between 100 and 5000; an out-of-range or non-numeric value is a startup error. See [Unified operator chat](engagement-architecture.md). |

Booleans accept `true`/`false`, `1`/`0` and `t`/`f`. A typo such as `yes` is a
startup error rather than a silent `false`.

Example — running on a different port:

```bash
# Linux / macOS
STREAMING_TREE_PORT=9000 go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:STREAMING_TREE_PORT="9000"; go run ./cmd/server
```

An invalid value produces a clear error at startup instead of silently falling
back to the default.

### Building an executable

```bash
cd apps/server
go build -o bin/streaming-tree-server ./cmd/server
```

On Windows:

```powershell
go build -o bin/streaming-tree-server.exe ./cmd/server
```

The `bin/` directory is ignored by Git.

---

## Data storage

Platform configuration and stream metadata are stored in a local **SQLite**
database. The driver is `modernc.org/sqlite`, a pure-Go implementation, so the
backend still builds with plain `go build` and needs no C toolchain.

### Where the database lives

The path is resolved in this order:

1. **`STREAMING_TREE_DB_PATH`** — the full path to the file, including its name.
2. **`STREAMING_TREE_DATA_DIR`** — a directory; the file `streaming-tree.db` is
   created inside it.
3. **The default** — the per-user configuration directory reported by Go's
   `os.UserConfigDir()`, plus `StreamingTree/streaming-tree.db`:

| System  | Default location |
| ------- | ---------------- |
| Windows | `%AppData%\StreamingTree\streaming-tree.db` (usually `C:\Users\<you>\AppData\Roaming\StreamingTree\streaming-tree.db`) |
| macOS   | `~/Library/Application Support/StreamingTree/streaming-tree.db` |
| Linux   | `$XDG_CONFIG_HOME/StreamingTree/streaming-tree.db`, or `~/.config/StreamingTree/streaming-tree.db` |

The parent directory is created automatically. The default deliberately lives
**outside the repository**, so a working copy never accumulates a database file,
and `*.db` is ignored by Git in any case.

The resolved path is printed at startup:

```
level=INFO msg="database ready" path=... journal_mode=wal
```

That line contains no credentials. A destination stream key and a connected
account's OAuth token bundle are both stored (in the operating system
credential store, via `SecretStore` - never in SQLite, never in a log line),
but neither the database file nor this startup log ever contains one - see
[Stream key security](project-overview.md#10-stream-key-security) and
[Connected accounts and Twitch metadata](connecting-platforms.md#connected-accounts-and-twitch-metadata).

### Migrations

The schema is created and updated by migrations embedded in the binary. They run
automatically at startup — there is no separate migration command. Each
migration commits together with its bookkeeping row, so a failed migration is
never recorded as applied and is retried next time. Applied migrations are
tracked in the `schema_migrations` table and never run twice.

### Seeded configurations

On a **brand-new** database, four destinations are created, one per supported
platform (Twitch, YouTube, Kick, TikTok). They are **disabled** and carry example
metadata. Because the seed is an ordinary recorded migration, it runs exactly
once: **if you delete a seeded destination, restarting the application will not
bring it back.**

No stream key, token or credential is part of the seed itself - the seeded
rows are disabled placeholders with example metadata only. Destination keys
and connected-account OAuth tokens are accepted and stored later, when you
configure them, and always in the OS credential store rather than in SQLite.

### Using a development database

Point the backend at a throwaway file so your real configuration is untouched:

```bash
# Linux / macOS
STREAMING_TREE_DB_PATH=/tmp/streaming-tree-dev.db go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:STREAMING_TREE_DB_PATH="$env:TEMP\streaming-tree-dev.db"; go run ./cmd/server
```

### Resetting a development database

Stop the backend and delete the file. It is recreated, migrated and re-seeded on
the next start:

```bash
# Linux / macOS
rm -f /tmp/streaming-tree-dev.db /tmp/streaming-tree-dev.db-wal /tmp/streaming-tree-dev.db-shm
```

```powershell
# Windows PowerShell
Remove-Item "$env:TEMP\streaming-tree-dev.db*"
```

WAL mode creates `-wal` and `-shm` sidecar files next to the database; remove
them too.

> ### ⚠ Deleting a database deletes your configuration
>
> The database file **is** your saved data: every configured destination, its
> display name and enabled state, and all stream metadata and tags. Deleting it
> removes all of that permanently, and there is no backup or undo. Make sure you
> are deleting a development database and not the default per-user one.

---

## Production build

### Frontend

```bash
cd apps/web
npm run build
```

The result lands in `apps/web/dist/`. The build runs type checking first, so a
type error stops it.

Previewing the built version:

```bash
npm run preview
```

(`vite.config.ts`'s `preview.proxy` mirrors the dev server's own `/api`
proxy, so `npm run preview` against a real or hermetic backend behaves
the same way `npm run dev` does - see `VITE_DEV_API_PROXY_TARGET`
above.)

**Route-level code splitting.** Every page under `src/pages/` is
lazy-loaded (`React.lazy`, via the small `src/lib/lazy-page.ts` adapter
- every page is a named export, and `lazy` only accepts a default one)
and reached through a `Suspense` boundary scoped to that route's own
`<Outlet>` content (`src/components/layout/RouteLoadingFallback.tsx`) -
never around `ShellLayout` itself, which stays mounted and never
remounts across a route change. Only `DashboardPage` (the first thing a
returning operator sees) and `NotFoundPage` (the tiny catch-all) stay
eager. This dropped the single production JS entry chunk from ~1.28 MB
to ~854 KB (see `docs/progress.md`'s performance-hardening entry for
the full before/after bundle audit); the remaining size is dominated by
`react-dom` itself and other framework/library code every route needs
regardless, not application code, and further splitting it would not
reduce what a fresh load actually has to fetch before rendering. Adding
a new page under `ShellLayout` should follow the same pattern rather
than a plain top-level import in `App.tsx`.

**Live-but-not-instant query data.** A `useQuery` polling a resource
that changes at most every few seconds (an update-check status, a
branch/runtime snapshot) should give TanStack Query a small positive
`staleTime` (seconds, not `0`) even though the data is genuinely live -
`refetchInterval` is what drives real freshness, `staleTime: 0` only
means every newly-mounted observer (a dialog reopened, a route
revisited, a sibling component mounting moments later) re-triggers an
extra fetch for data that is still correct. Two real, measured
duplicate-request defects of exactly this shape were found and fixed
this way - see `src/hooks/use-branches.ts` and `src/hooks/
use-updates.ts`'s own comments for the specific mechanism each one hit.

**Auditing bundle composition**: `ANALYZE=1 npm run build` (PowerShell:
`$env:ANALYZE=1; npm run build`) additionally emits
`dist/bundle-analysis.json` (module/chunk composition data via
`rollup-plugin-visualizer`, a dev-only dependency) - not part of a
normal build.

### Backend

```bash
cd apps/server
go build ./...
```

---

## Lint, typecheck, tests and other checks

Automated checks can and should be run while working. Manual interface testing
is the final stage — see `docs/project-overview.md`, section 14.

**Frontend** (from `apps/web`):

```bash
npm run i18n:check  # translation resource consistency
npm run typecheck   # TypeScript type checking (tsc -b)
npm run lint        # ESLint
npm run test        # unit tests (Vitest), plus a set of rendered-component tests (React Testing Library) covering the Twitch device-flow and YouTube OAuth modals, disconnect/publish confirmations, the Engagement page/connector card/event feed, the Chat page/message/activity/moderation rows, the outbound-chat composer, the chat-overlay renderer/Overlays management page, and the Automation page's schedule/command lists, editors, Send Now confirmation and preview
npm run build       # production build
```

**Real-browser E2E tests** (from `apps/web`, Playwright + a real Chromium):

```bash
npx playwright install chromium   # one-time browser download
npm run test:e2e                  # builds/starts a hermetic backend + Vite dev server, then runs the suite
```

This is a real-browser regression layer on top of the Vitest/RTL suite
above, not a replacement for it - it exists because jsdom cannot prove real
scroll position, focus, CSS stacking-context/paint order, or actual
viewport layout, all of which a prior Stage 20E manual pass found real
defects in. `npm run test:e2e` needs nothing configured: it builds and
runs the same `-tags integration` test server every
`scripts/verify-*.mjs` script already uses
(`apps/server/cmd/testserver`, requiring `go` on `PATH`) against a
fresh temporary data directory, starts the real Vite dev server pointed
at it, and runs every spec in `apps/web/e2e/specs/` against real
Chromium - never the operator's installed application, real credentials,
real OBS, or a production build. It covers: sidebar scroll preservation
across navigation, layout at representative viewport heights, the OBS
connection panel's disclosure/error behavior, brand→Dashboard
navigation, the Platforms/Metadata pages, modal stacking/focus-trapping,
both onboarding outcomes (success and a deterministically forced
failure), a route smoke matrix across every current primary route, and
the typography/static-asset determinism contract (see "Typography and
static assets" above), and route-level code splitting/startup-request
hygiene (see "Route-level code splitting" above) - each with an
automatic console/page-error gate.
It deliberately does **not** replace eventual manual Stage 20E verification of the real
Windows installer, tray, OBS Browser Source, audio devices, or real
provider accounts - see `apps/web/e2e/playwright.config.ts`'s own doc
comment and each spec file for the exact scope. Runs in CI as the `e2e`
job in `cross-platform.yml`.

**Backend** (from `apps/server`):

```bash
go build ./...      # compilation
go vet ./...        # static analysis
go test ./...       # tests
gofmt -l .          # lists files needing formatting (empty output = all good)
```

Backend tests always create their own temporary database in the test's temp
directory, so running them never touches your real one.

**Integration checks** (from the repository root):

```bash
node scripts/verify-persistence.mjs               # SQLite survives a backend restart
node scripts/verify-mediamtx-runtime.mjs          # real MediaMTX install and supervision
node scripts/verify-ffmpeg-branches.mjs           # real FFmpeg + MediaMTX destination branches
node scripts/verify-twitch-account-integration.mjs # Twitch device flow, linking, publish - fake Twitch only
node scripts/verify-youtube-account-integration.mjs # YouTube PKCE flow, linking, broadcast/category, publish - fake Google only
node scripts/verify-twitch-engagement.mjs         # Event Bus + EventSub connector - fake Twitch only
node scripts/verify-operator-chat.mjs             # unified operator chat: projection, preferences, badges/emotes - fake Twitch only
node scripts/verify-chat-overlay.mjs              # OBS Browser Source chat overlay: profiles, public projection, public API - fake Twitch only
node scripts/verify-twitch-outbound-chat.mjs      # manual outbound chat: capability, dispatcher, sending/replies - fake Twitch only
node scripts/verify-chat-automation.mjs           # scheduled messages + chat commands: persistence, gating, placeholders, self-loop protection - fake Twitch only
node scripts/verify-alerts.mjs                    # alert rules/queue: matching, priority, expiration, pause/skip/replay/clear - fake Twitch only
node scripts/verify-alert-advanced-queue.mjs      # Stage 12B grouping and mid-alert preemption - fake Twitch only
node scripts/verify-alert-designer.mjs            # Stage 13A alert visual-design HTTP API and public rendering - fake Twitch only
node scripts/verify-chat-overlay-designer.mjs     # Stage 13B chat overlay visual-design HTTP API and public rendering - fake Twitch only
node scripts/verify-visual-templates.mjs          # Stage 14A visual-template library: built-ins, compatibility, JSON import/export - no fake servers needed
node scripts/verify-visual-template-packages.mjs  # Stage 14B managed assets and portable .streaming-tree-template packages - no fake servers needed
node scripts/verify-youtube-engagement.mjs        # Stage 15A YouTube Live Chat connector, alerts, outbound chat, chat automation - fake Google/YouTube only
node scripts/verify-streamelements-donations.mjs  # Stage 16A StreamElements donations: Astro connector, money, moderation, alerts, operator chat - fake Astro WebSocket only
node scripts/verify-tts-audio.mjs                 # Stage 17A shared audio runtime and TTS: queue, filtering, playback lifecycle, public audio route - fake TTS provider + fake Astro WebSocket only
node scripts/verify-alert-audio.mjs               # Stage 17B persistent alert sound/TTS: managed audio assets, rule-owned playback/arbitration/bounded hold, package v2 audio - fake TTS provider only
node scripts/verify-goals-widgets.mjs             # Stage 18A persistent goals/counters: accumulation, dedupe, baseline management, public goal widgets - fake Twitch/YouTube/StreamElements
node scripts/verify-supporter-widgets.mjs         # Stage 18B supporter/activity widgets: latest/largest/recent/ticker/counters, dashboards, runtime-only privacy - fake Twitch/YouTube/StreamElements
node scripts/verify-packaged-app.mjs              # Stage 20A packaged production runtime: routing, legal routes, single-instance, graceful shutdown - real release build, no fake servers
node scripts/verify-updater.mjs                   # Stage 20B application updater: real Inno Setup install/upgrade/restart cycle, manifest and hash-mismatch rejection - fake GitHub API only, real installers
node scripts/verify-metadata-presets.mjs          # Stage 22 reusable stream metadata presets: CRUD, restart persistence, provider-scoped apply, cross-provider isolation, all-or-nothing atomicity - no fake servers needed, no account ever linked
```

The list above is representative, not exhaustive - the repository has grown
further scripts covering backup/restore, onboarding, stream setup/preflight/
insights, and the Windows/macOS/Linux packaging and updater flows. The
current, complete, canonical set is always:

```bash
ls scripts/verify-*.mjs
```

Packaging-specific scripts (`verify-installer.mjs`, `verify-packaged-app.mjs`,
`verify-macos-package.mjs`, `verify-linux-package.mjs`,
`verify-linux-headless.mjs`, `verify-updater.mjs`) need a full platform
release build first - see the relevant packaging doc in
[`docs/README.md`](README.md).

The persistence script starts the backend against a temporary database,
exercises the whole platform API, restarts the process against the same file and
verifies the data survived.

The MediaMTX script uses a temporary data directory and dynamically chosen
loopback ports. It downloads and checksum-verifies the **real** v1.19.3 binary
through the application's own installation endpoint, waits for readiness, checks
the ingest state, stops and starts the service, restarts the backend and
confirms the binary is reused rather than downloaded again. It takes a few
minutes on the first run and needs network access.

The FFmpeg-branches script needs a **real, compatible FFmpeg on `PATH`**
(or pointed to by `STREAMING_TREE_FFMPEG_PATH`) — it never installs one
itself, and stops with a clear message naming the missing prerequisite
instead of claiming success if none is found. It builds a special
`-tags integration` backend binary whose only difference from the real one
is an in-memory fake credential store (see
`apps/server/cmd/testserver/main.go`), so no fake key it uses can ever reach
your real OS keychain, and that build tag makes the swap impossible to
select by accident in a normal build. Everything else — a synthetic
publisher, the real managed MediaMTX as the local ingest, real independent
branch FFmpeg processes, and two more real MediaMTX instances standing in
for destination platforms — runs entirely on loopback with dynamically
chosen ports. It takes roughly a minute (the restart-limit scenario walks
through real exponential backoff).

The Twitch-account-integration script builds the same `-tags integration`
binary and runs two small in-process fake HTTP servers that reproduce only
the Twitch OAuth (`/device`, `/token`, `/validate`, `/revoke`) and Helix
(`/users`, `/channels`, `/search/categories`) response shapes this
application parses. **It never contacts real Twitch, and no real Twitch
account is ever used or required to run it.** It covers Client ID
configuration, a full device-code authorization, account finalization,
linking, category search, metadata publish (asserting only the verified
fields ever reach the fake server), a forced token expiry and its single
transparent refresh-and-retry, reconnecting, and disconnect/revocation —
finishing with a scan of every captured backend response and log line for
every token the run issued.

The YouTube-account-integration script follows the identical shape
against fake Google OAuth and YouTube Data API servers instead, including
a wrong-CSRF-state callback and explicit multi-channel selection. See
[Connected accounts and YouTube metadata](connecting-platforms.md#connected-accounts-and-youtube-metadata)
for the full list of what it covers.

The Twitch-engagement script adds a third fake: a small, hand-rolled
Twitch EventSub WebSocket server (this project added no new dependency
to get one — see the script's own header comment). It covers the
identity-bound permission-upgrade scope union, exact subscription
creation, event normalization and deduplication, Twitch's official
`session_reconnect` handoff, an ordinary disconnect's data-gap handling,
revocation, restart, disable and disconnect. See
[Engagement Event Bus and Twitch chat/events](engagement-architecture.md)
for the full list of what it covers and what is instead covered by Go
unit tests.

The operator-chat script reuses the same fake OAuth/Helix/EventSub
servers, extended with fake `GET /chat/badges/global` and
`GET /chat/badges` routes. It covers badge channel-then-global
resolution with cache-hit counts, the emote CDN URL, an exact deletion
updating the same item id, a per-user clear and a whole-chat clear each
correctly scoped, every activity type (gift batch vs. recipient, bits
never a donation), preferences/hidden-user/bot-user persistence
surviving a real backend restart while chat content itself resets, and
the SSE stream. See
[Unified operator chat](engagement-architecture.md) for the full list of
what it covers and what is instead covered by Go unit tests.

The chat-overlay script reuses the same fake OAuth/Helix/EventSub
servers again and drives chat through the exact same path operator chat
itself uses, layering public-overlay-specific assertions on top: safe
defaults, a live message reaching a filtered public overlay, every
filter (accounts, hidden users, bots, commands, blocked terms, activity
types), `maxVisibleItems` eviction, deletion/clear scoping, two
independent overlays never sharing state, slug rotation, restart
behavior, and a final scan for leaked chat text, blocked-term values,
hidden-user data, the public slug, or access tokens. See
[OBS Browser Source chat overlay](obs-browser-source.md) for
the full list of what it covers and what is instead covered by Go unit
tests.

The outbound-chat script reuses the same fake OAuth/Helix/EventSub
servers once more, extended with a `POST /chat/messages` fake (switched
between success, dropped, `401`/`403`/`422`/`429`/5xx, and a
transport-destroyed "uncertain" response by the step under test) and a
`refresh_token` grant on the fake OAuth server. It covers the exact
scope union (including the permanent absence of `user:bot`/
`channel:bot`), an identity-mismatched upgrade rejected, a successful
upgrade persisting, a send using the account's own provider user id for
both broadcaster and sender with no `for_source_only`/`pin` ever sent,
reply-parent forwarding, `is_sent:false` surfaced as a stable dropped
error, a single `401` refreshed and retried exactly once, a second
`401` stopping and recovering, every other error status mapped and
never auto-retried, two accounts with fully isolated queues and rate
limits, and a sent message's real EventSub echo appearing exactly once
with no optimistic duplicate. See
[Sending Twitch chat manually](provider-integrations/twitch-outbound-chat.md) for the
full list of what it covers.

The chat-automation script reuses the same fake OAuth/Helix/EventSub
servers and the real outbound-chat dispatcher underneath. It covers a
schedule and a command (with aliases) persisting, preview resolving
`{channelName}`/`{platform}`/`{channelUrl}` and rejecting an unknown
placeholder, a scheduled send waiting while local ingest is not
receiving and never sending a backlog once it starts, the
minimum-chat-activity gate blocking until enough real messages arrive
and resetting after a successful send, Send Now with a per-target
result, a command matching its canonical name and an alias while
ignoring extra arguments and a mid-message `!`, the self-message rule
preventing a reply loop, role and cooldown gating, disabling a
schedule or command stopping it immediately, and a backend restart
preserving definitions while resetting all runtime state with no
missed-run catch-up. A representative subset of the full scenario list
in the task's own specification is covered here; every scenario this
script does not itself exercise (jitter bounds, exhaustive per-role
matching, per-user cooldown independence under concurrency, and the
full HTTP status-code mapping) is instead covered by named Go tests in
`internal/chatautomation` and `internal/httpapi` — see
[`docs/progress.md`](progress.md) for the exact mapping. See
[Scheduled messages and chat commands](engagement-architecture.md)
for the full list of what it covers.

**None of these scripts touch your real database, your managed MediaMTX
installation, your real OS credential store, or a real Twitch/Google
account**, and all remove their temporary directories afterwards.

---

## REST API

All endpoints live under `/api` and return `application/json`.

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `GET` | `/api/health` | Liveness, service name, version and uptime. |
| `GET` | `/api/platform-definitions` | Built-in provider definitions: capabilities, limits and supported option identifiers. |
| `GET` | `/api/platforms` | All configured destinations, ordered, with their provider definition and metadata. |
| `POST` | `/api/platforms` | Create a destination. Responds 201 with a `Location` header. |
| `GET` | `/api/platforms/{id}` | One configured destination. |
| `PUT` | `/api/platforms/{id}` | Replace display name, enabled state and sort order. |
| `DELETE` | `/api/platforms/{id}` | Delete a destination; metadata and tags cascade. Responds 204. |
| `GET` | `/api/platforms/{id}/metadata` | Stored metadata and ordered tags. |
| `PUT` | `/api/platforms/{id}/metadata` | Replace metadata and tags atomically. |
| `GET` | `/api/platforms/{id}/credentials` | Stream-key status: `{configured}`, plus whether the OS credential store is reachable. Never the key itself. |
| `PUT` | `/api/platforms/{id}/credentials/stream-key` | Validate and store a new stream key, replacing any previous one. Body capped at 8 KiB, well below the general 64 KiB limit. |
| `DELETE` | `/api/platforms/{id}/credentials/stream-key` | Delete the stored stream key. Idempotent: deleting an absent key still responds 204. |
| `GET` | `/api/platforms/{id}/output` | A destination's output settings: `{serverUrl, autoRestart}`. Never the stream key. |
| `PUT` | `/api/platforms/{id}/output` | Replace a destination's output settings (full replacement). |
| `GET` | `/api/runtime` | One versioned snapshot: MediaMTX state, ingest state and OBS connection values. |
| `POST` | `/api/runtime/mediamtx/install` | Start a managed installation. Responds 202; 409 if one is already running. |
| `POST` | `/api/runtime/mediamtx/start` | Start the ingest service. 202 accepted, 409 if already running, 422 if missing or incompatible. |
| `POST` | `/api/runtime/mediamtx/stop` | Stop it. Suppresses automatic restart for this stop. |
| `POST` | `/api/runtime/mediamtx/restart` | One controlled stop followed by a start. |
| `GET` | `/api/runtime/ffmpeg` | The resolved FFmpeg dependency: state, source, detected version, minimum version, probed capabilities. Never the executable path. |
| `GET` | `/api/runtime/branches` | Every destination branch's runtime snapshot: state, desired-running, blockers, timestamps, restart count, sanitized last error, real progress. Never a secret or a full destination URL. |
| `POST` | `/api/runtime/branches/{id}/start` | Start one destination. `202` accepted, `200` with `{status:"blocked", blockers}` if ineligible, `409` if already starting/live/restarting. |
| `POST` | `/api/runtime/branches/{id}/stop` | Stop one destination and suppress its automatic restart. `409` if it was not running. |
| `POST` | `/api/runtime/branches/{id}/restart` | One controlled stop followed by a start. |
| `POST` | `/api/runtime/branches/start-enabled` | Start every eligible enabled destination; one ineligible destination never blocks another. Returns a per-destination result. |
| `POST` | `/api/runtime/branches/stop-all` | Stop every running destination. |
| `GET` | `/api/integrations/twitch/config` | Twitch Client ID status: `{configured, source, clientId}` — `clientId` present only when `source` is `"database"`. |
| `PUT` | `/api/integrations/twitch/config` | Save a database-managed Client ID. `409` if an environment override is active, or if changing it while accounts exist. |
| `POST` | `/api/integrations/twitch/device-flow` | Start a Twitch device-authorization attempt. `202` with the attempt snapshot; `409` if one is already active. |
| `GET` | `/api/integrations/twitch/device-flow/{id}` | Poll one attempt's current state. Never contains the device code. |
| `DELETE` | `/api/integrations/twitch/device-flow/{id}` | Cancel an in-progress attempt. |
| `GET` | `/api/connected-accounts` | Every connected account: identity, status, granted scopes, last-validated time. Never a token. |
| `GET` | `/api/connected-accounts/{id}` | One connected account. |
| `DELETE` | `/api/connected-accounts/{id}` | Disconnect: revoke with the provider where possible, then remove locally and cascade any destination link. Responds 204. |
| `POST` | `/api/connected-accounts/{id}/validate` | Validate immediately (instead of waiting for the hourly background check), refreshing the token first if needed. |
| `POST` | `/api/connected-accounts/{id}/reconnect` | Start a new attempt that must resolve to this same account — a device-flow attempt for a Twitch account, an Authorization Code + PKCE attempt for a YouTube one. |
| `GET` | `/api/connected-accounts/{id}/twitch/categories` | Search Twitch categories/games via `?query=`. Requires a healthy linked-or-standalone account. |
| `GET` | `/api/platforms/{id}/connected-account` | The account linked to a destination, or `null`. |
| `PUT` | `/api/platforms/{id}/connected-account` | Link (or replace the link to) an account. Body `{accountId}`. `422` on a provider mismatch. |
| `DELETE` | `/api/platforms/{id}/connected-account` | Unlink, without deleting either side. Responds 204. |
| `GET` | `/api/platforms/{id}/metadata/publish-preview` | What publishing would change right now: remote values (and, for YouTube, the selected broadcast), local values, changed/unchanged/skipped fields, blockers, warnings. |
| `POST` | `/api/platforms/{id}/metadata/publish` | Publish the metadata currently saved in SQLite to the destination's real provider. **No request body** — publishing a draft is not possible. |
| `GET` | `/api/integrations/youtube/config` | YouTube Client ID status — same shape as the Twitch config endpoint above. |
| `PUT` | `/api/integrations/youtube/config` | Save a database-managed YouTube Client ID. Same `409` rules as Twitch's, independent of it. |
| `POST` | `/api/integrations/youtube/oauth-attempts` | Start a YouTube Authorization Code + PKCE attempt. `202` with the attempt snapshot (including the authorization URL to open); `409` if one is already active. |
| `GET` | `/api/integrations/youtube/oauth-attempts/{id}` | Poll one attempt's current state. Never contains the authorization code, PKCE verifier, or CSRF state value. |
| `DELETE` | `/api/integrations/youtube/oauth-attempts/{id}` | Cancel an in-progress attempt and close its temporary loopback listener. |
| `POST` | `/api/integrations/youtube/oauth-attempts/{id}/channel` | Explicitly select one of several owned channels, when the attempt is `awaiting_channel_selection`. Body `{channelId}`. |
| `GET` | `/api/connected-accounts/{id}/youtube/broadcasts` | List the linked channel's active and upcoming live broadcasts. Never ingestion data. |
| `GET` | `/api/connected-accounts/{id}/youtube/categories` | List assignable video categories for the account's effective region. |
| `GET` | `/api/connected-accounts/{id}/youtube/region` | The account's effective category region (saved override, else the channel's own country). |
| `PUT` | `/api/connected-accounts/{id}/youtube/region` | Save an explicit two-letter region override. |
| `GET` | `/api/platforms/{id}/remote-target` | The selected live-broadcast target for a YouTube destination, or `null`. |
| `PUT` | `/api/platforms/{id}/remote-target` | Select a broadcast. Body `{resourceId}`. `422` if it does not belong to the linked channel. |
| `DELETE` | `/api/platforms/{id}/remote-target` | Clear the selection, without touching the account link. Responds 204. |
| `GET` | `/api/engagement/status` | Event Bus status: schema version, buffer capacity, retained count, oldest/newest sequence, active subscribers, and a summary per Twitch connector. No message content. |
| `GET` | `/api/engagement/events` | A bounded snapshot of retained normalized events. Query params `after`/`limit` (capped at 500); reports `gap: true` when `after` refers to an already-evicted sequence. |
| `GET` | `/api/engagement/stream` | Server-Sent Events: live normalized events as they are published. Supports `Last-Event-ID` (or `?after=`) for replay, emits `engagement.gap` when replay is incomplete, and periodic keepalive comments. Bounded concurrent clients. |
| `GET` | `/api/connected-accounts/{id}/engagement` | One Twitch account's connector status plus its capability assessment (required/granted scopes, whether a permission upgrade is required). `422` for a non-Twitch account. |
| `PUT` | `/api/connected-accounts/{id}/engagement` | Enable or disable the connector. Body `{enabled}`. Persists; an enabled connector reconnects automatically after a backend restart. |
| `POST` | `/api/connected-accounts/{id}/engagement/authorize` | Start an identity-bound Device Code Flow requesting the union of the account's existing scopes and the engagement profile. **No request body.** Reuses the Twitch device-flow attempt snapshot shape. |
| `POST` | `/api/connected-accounts/{id}/engagement/restart` | Cancel and restart the connector without changing its persisted enabled setting. **No request body.** |
| `GET` | `/api/connected-accounts/{id}/outbound-chat` | One Twitch account's outbound-chat status: capability (`unsupported`/`permission_required`/`ready`/`error`), required/granted/missing scopes, dispatcher state, queue depth/capacity, last attempt/success times, last stable error code, sanitized retry time, whether sending is currently possible, and a standing Shared Chat disclosure identifier. No credential, no message content. |
| `POST` | `/api/connected-accounts/{id}/outbound-chat/authorize` | Start an identity-bound Device Code Flow requesting the union of the account's existing scopes and `user:write:chat`. **No request body.** Reuses the Twitch device-flow attempt snapshot shape. |
| `POST` | `/api/connected-accounts/{id}/outbound-chat/messages` | Send one chat message (optionally a reply) as this account. Body `{message, replyParentMessageId?}`, capped at 8 KiB. Response `{sent, providerMessageId, sentAt}` — never echoes the message text. |
| `GET` | `/api/operator-chat/status` | Operator-chat projection status: schema version, buffer capacity, retained count, oldest/newest sequence, active subscribers, and a one-way "bus gap ever detected" flag. No message content. |
| `GET` | `/api/operator-chat/items` | A bounded snapshot of retained operator-chat items. Query params `after`/`limit` (capped at 1000), repeatable `accountId`, comma-separated `kinds`, `includeDeleted`; reports `gap: true` when `after` is no longer retrievable. |
| `GET` | `/api/operator-chat/stream` | Server-Sent Events: live operator-chat items (each a complete current-state upsert) as they change. Supports `Last-Event-ID` (or `?after=`) for replay, the same `accountId`/`kinds`/`includeDeleted` filters, emits `operator-chat.gap` when replay is incomplete, and periodic keepalive comments. Bounded concurrent clients. |
| `GET` | `/api/operator-chat/preferences` | Persisted display preferences, or the documented defaults if never saved. |
| `PUT` | `/api/operator-chat/preferences` | Full replacement of every preference field. Unknown fields rejected. |
| `GET` | `/api/operator-chat/account-visibility` | Every connected account with an explicit visibility override. An account absent from this list is visible by default. |
| `PUT` | `/api/operator-chat/account-visibility/{id}` | Set one connected account's chat visibility. Body `{visible}`. `404` for an unknown account. |
| `GET` | `/api/operator-chat/hidden-users` | Every operator-hidden user, identified by provider user id. |
| `POST` | `/api/operator-chat/hidden-users` | Hide a user, idempotently. Body `{providerId, connectedAccountId, providerUserId, label?}`. |
| `DELETE` | `/api/operator-chat/hidden-users/{id}` | Un-hide, by the entry's own id. Responds 204; `404` if it no longer exists. |
| `GET` | `/api/operator-chat/bot-users` | Every operator-marked bot user — a separate list from hidden users. |
| `POST` | `/api/operator-chat/bot-users` | Mark a user as a bot, idempotently. Same body shape as hidden-users. |
| `DELETE` | `/api/operator-chat/bot-users/{id}` | Unmark, by the entry's own id. Responds 204; `404` if it no longer exists. |
| `GET` | `/api/chat-overlays` | Every overlay profile. |
| `POST` | `/api/chat-overlays` | Create an overlay profile with safe defaults and a fresh, unguessable public slug. Responds 200 with the created profile. |
| `GET` | `/api/chat-overlays/{id}` | One overlay profile, including its current public slug. |
| `PUT` | `/api/chat-overlays/{id}` | Full replacement of an overlay's settings. Never accepts or changes `id`, `publicSlug` or `createdAt`. Triggers a live rebuild of the running overlay. |
| `DELETE` | `/api/chat-overlays/{id}` | Delete an overlay profile; its public URL stops serving immediately. Responds 204. |
| `POST` | `/api/chat-overlays/{id}/rotate-public-slug` | Rotate the public slug. The previous URL stops resolving immediately; every other setting is untouched. |
| `GET` | `/api/chat-overlays/{id}/accounts` | The connected accounts selected for this overlay. Empty means every currently available account. |
| `PUT` | `/api/chat-overlays/{id}/accounts` | Replace the account selection. `422` on an unknown account id. |
| `GET` | `/api/chat-overlays/{id}/hidden-users` | This overlay's own hidden-user list — independent of the operator Chat page's own list. |
| `POST` | `/api/chat-overlays/{id}/hidden-users` | Hide a user on this overlay, idempotently. |
| `DELETE` | `/api/chat-overlays/{id}/hidden-users` | Un-hide, identified by `providerId`/`connectedAccountId`/`providerUserId` query parameters (this list has no synthetic per-entry id). |
| `GET` | `/api/chat-overlays/{id}/blocked-terms` | This overlay's own blocked terms. |
| `POST` | `/api/chat-overlays/{id}/blocked-terms` | Add a blocked term (`contains` or `whole_word` match mode), idempotently by normalized value. Bounded to 100 per overlay. |
| `DELETE` | `/api/chat-overlays/{id}/blocked-terms/{termId}` | Remove a blocked term by its own id. Responds 204; `404` if it no longer exists. |
| `GET` | `/api/chat-overlays/{id}/activity-types` | The activity types selected for this overlay. Empty means every type shown. |
| `PUT` | `/api/chat-overlays/{id}/activity-types` | Replace the activity-type selection. |
| `GET` | `/api/public/chat-overlays/{publicSlug}/config` | **Unauthenticated.** Public, presentation-only overlay configuration — no management id, no filter values, no blocked-term text. |
| `GET` | `/api/public/chat-overlays/{publicSlug}/items` | **Unauthenticated.** A bounded snapshot of the overlay's currently visible items, already filtered and presentation-shaped. |
| `GET` | `/api/public/chat-overlays/{publicSlug}/stream` | **Unauthenticated.** Server-Sent Events: `chat-overlay.reset`/`.upsert`/`.remove` as the overlay's visible content changes. An unknown or disabled slug still opens a normal connection and renders an empty overlay, never a hard HTTP error. Bounded concurrent clients per overlay. |
| `GET` | `/api/chat-automation/status` | A combined snapshot: every schedule's and command's runtime state (next-run, last attempt/success, skip reason, sends this hour / match and response counts), plus overall engine status. Never a message body or username. |
| `GET` | `/api/chat-automation/schedules` | Every persisted schedule with its targets, message alternatives and current runtime snapshot. |
| `POST` | `/api/chat-automation/schedules` | Create a schedule. Responds 201 with a `Location` header. `422` on an invalid name/timing/target/template/placeholder. |
| `GET` | `/api/chat-automation/schedules/{id}` | One schedule. |
| `PUT` | `/api/chat-automation/schedules/{id}` | Full replacement of a schedule's definition. Resets its runtime state (next-run, activity counters, rolling send count) cleanly. |
| `DELETE` | `/api/chat-automation/schedules/{id}` | Delete a schedule and its targets/messages. Responds 204. |
| `POST` | `/api/chat-automation/schedules/{id}/send-now` | Send this schedule's template immediately to explicitly selected (or, if omitted, every eligible) targets. Body `{accountIds?}`. One result per target; one failure never blocks another. Never a preview — this sends real messages. |
| `GET` | `/api/chat-automation/commands` | Every persisted command with its aliases, targets and current runtime snapshot. |
| `POST` | `/api/chat-automation/commands` | Create a command. Responds 201 with a `Location` header. `409` on a name/alias already used by another command. |
| `GET` | `/api/chat-automation/commands/{id}` | One command. |
| `PUT` | `/api/chat-automation/commands/{id}` | Full replacement of a command's definition. Aliases update atomically; takes effect immediately. |
| `DELETE` | `/api/chat-automation/commands/{id}` | Delete a command and its aliases/targets. Responds 204. |
| `POST` | `/api/chat-automation/preview` | Render a template locally against one account (and optional platform context). Body `{template, accountId, platformId?}`. Never sends, never persists, never contacts Twitch. |
| `GET` | `/api/alert-event-types` | The real, capability-derived table of which conditions/placeholders apply to each of the 8 supported alert event types. |
| `GET` | `/api/alert-profiles` | Every alert profile. |
| `POST` | `/api/alert-profiles` | Create a profile with safe defaults and a fresh, unguessable public slug. Responds 201 with a `Location` header. |
| `GET` | `/api/alert-profiles/{id}` | One alert profile. |
| `PUT` | `/api/alert-profiles/{id}` | Full replacement of a profile's settings. Never accepts or changes `id`, `publicSlug` or `createdAt`. |
| `DELETE` | `/api/alert-profiles/{id}` | Delete a profile; its runtime stops and its public URL stops serving immediately. Responds 204. |
| `POST` | `/api/alert-profiles/{id}/rotate-public-slug` | Rotate the public slug. The previous URL stops resolving immediately. **No request body.** |
| `GET` | `/api/alert-profiles/{id}/rules` | Every rule on this profile, plus any quantity-range overlap warnings. |
| `POST` | `/api/alert-profiles/{id}/rules` | Create a rule. Responds 201 with a `Location` header. `422` on an unsupported condition for the event type. |
| `GET` | `/api/alert-profiles/{id}/queue` | This profile's bounded management queue status: paused, current alert, queued count/capacity, a bounded list of next-queued alerts, and every counter (enqueued/played/expired/capacity-dropped/manually-skipped/synthetic). |
| `POST` | `/api/alert-profiles/{id}/queue/pause` | Freeze queue progression for this profile. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/resume` | Resume queue progression. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/skip-current` | Remove the current alert immediately; counted as manually skipped, never played. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/replay-previous` | Re-show the single most recent completed/skipped alert. Never creates a real Engagement Event. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/clear` | Remove every not-yet-played queued item; the current alert is untouched. **No request body.** |
| `GET` | `/api/alert-rules/{id}` | One alert rule. |
| `PUT` | `/api/alert-rules/{id}` | Full replacement of a rule's definition. Takes effect immediately, without a backend restart. |
| `DELETE` | `/api/alert-rules/{id}` | Delete a rule. Responds 204. |
| `POST` | `/api/alert-rules/{id}/test` | Create one synthetic alert through the real queue/renderer using this rule's own presentation. Optional body `{scenario?}` for an edge-case fixture. No real Twitch account or Event Bus event involved. |
| `POST` | `/api/alert-rule-preview` | Render a template locally against representative fixture data for an event type. Body `{eventType, template, language?}`. Never sends, never persists, never touches the queue. |
| `GET` | `/api/public/alert-profiles/{publicSlug}/config` | **Unauthenticated.** Public, presentation-only profile configuration — theme/position/text-alignment/language only. |
| `GET` | `/api/public/alert-profiles/{publicSlug}/stream` | **Unauthenticated.** Server-Sent Events: `alert.show`/`.hide`/`.reset`/`.paused`/`.gap` as the current alert changes. A fresh connection's first event is always a complete current-state reset — never the queue's future contents. An unknown or disabled slug still opens a normal connection, never a hard HTTP error. Bounded concurrent clients per profile. |

The `POST` runtime and branch-command endpoints take **no request body**;
sending one is a `400`. They are commands, not resources. `GET /api/health`
does not change meaning here: the backend can be perfectly healthy while
FFmpeg is missing or a branch is in `error`.

Example — the runtime snapshot:

```bash
curl http://127.0.0.1:8080/api/runtime
```

```json
{
  "version": 1,
  "mediaMtx": {
    "supportedVersion": "v1.19.3",
    "installedVersion": "v1.19.3",
    "source": "managed",
    "state": "ready",
    "autoStart": true,
    "autoRestart": true,
    "startedAt": "2026-08-03T16:19:05Z",
    "restartCount": 0,
    "lastError": null
  },
  "ingest": {
    "state": "waiting",
    "path": "live",
    "trackCount": null,
    "tracks": []
  },
  "connection": {
    "serverUrl": "rtmp://127.0.0.1:1935",
    "streamKey": "live",
    "publishUrl": "rtmp://127.0.0.1:1935/live"
  }
}
```

**Runtime state lives only in memory.** It is never written to SQLite and resets
when the backend restarts — it describes what is happening now, not what you
configured. `restartCount` returning to zero after a restart is that working as
intended.

Example — listing configured destinations:

```bash
curl http://127.0.0.1:8080/api/platforms
```

Every error uses one envelope:

```json
{ "error": "not_found", "message": "The requested resource does not exist." }
```

Validation failures add a per-field map plus stable rule identifiers the
frontend localizes:

```json
{
  "error": "validation_failed",
  "message": "Validation failed",
  "fields": { "title": "Title cannot exceed 140 characters." },
  "details": { "title": { "rule": "too_long", "params": { "max": 140 } } }
}
```

Status codes: `400` malformed JSON or an unknown field, `404` missing record,
`405` unsupported method (with `Allow`), `409` conflict, `413` body over 64 KiB,
`415` wrong content type, `422` validation failure, `429` a provider's rate
limit was reached (Twitch endpoints only), `500` internal failure, `502` the
provider could not be reached. No endpoint ever forwards a raw Twitch error
body — every provider-facing failure is mapped to this same stable envelope.

**Provider definitions return semantic identifiers, never translated text.** The
backend sends `public`, `ultra-low`, `topic`; the frontend maps those to English
or Polish. The backend never decides the interface language.

**No endpoint returns a stored stream key, token or credential value - not
even the three credential endpoints above.** `PUT .../stream-key` accepts one
to store, but every credential response, including that one, carries only
`configured`/`available` booleans. Every other endpoint neither accepts nor
returns a credential at all; unknown JSON fields are rejected rather than
silently ignored, so a stray credential field on a non-credential endpoint
produces an error instead of disappearing quietly. New stable error codes for
the credential endpoints: `platform_not_found`, `credential_not_found`,
`credential_store_unavailable` (503), `credential_store_failure` (500) - see
"Stream key security" below.

**Stable error codes for the outbound-chat endpoints:**
`outbound_chat_unsupported` (503, non-Twitch account), `account_not_found`
(404, reusing the same code every other per-account endpoint already
uses), `outbound_chat_permission_required` (422), `account_reconnect_required`
(409, a second consecutive `401` from Twitch), `outbound_chat_queue_full`
(429), `outbound_chat_rate_limited` (429, with a sanitized retry time in
the status endpoint), `outbound_chat_forbidden` (403), `outbound_chat_message_dropped`
(422, Twitch accepted the request but did not deliver the message - its
own drop-reason prose is never included), `outbound_chat_delivery_unknown`
(502, a transport failure with no trustworthy result), `outbound_chat_provider_failure`
(502, a Twitch 5xx), and `outbound_chat_cancelled` (503). None of these
responses, nor the send endpoint's own success response, ever echoes the
sent message text.

**Stable error codes for the chat-automation endpoints:**
`chat_automation_not_found` (404, an unknown schedule or command id),
`chat_automation_account_not_found` (404), `chat_automation_target_required`
(422, a schedule or command saved with no targets), `chat_automation_target_invalid`
(422, an unknown, provider-mismatched or unlinked platform context),
`chat_automation_command_conflict` (409, a command name or alias already
used elsewhere), `chat_automation_invalid` (422, a general validation
failure - name/timing/message-count/cooldown bounds), `chat_automation_placeholder_invalid`
(422, an unknown or malformed `{placeholder}`), `chat_automation_provider_unsupported`
(503, a non-Twitch target), `chat_automation_permission_required` (422,
outbound-chat permission not yet granted for a target account),
`chat_automation_queue_full` (429, the automation quota on the shared
outbound queue is exhausted), `chat_automation_rate_limited` (429),
and `account_reconnect_required` (409, reusing the same code the
outbound-chat endpoints already use for a second consecutive `401`).
None of these responses ever echoes a template, a rendered message, or
a triggering username. A scheduled skip (waiting for the stream, waiting
for chat activity, an unresolved placeholder, an over-length render) is
not an HTTP error at all — it only ever shows up as a `lastSkipReason`
in the status/schedule snapshot, since no HTTP request exists at the
moment a timer decides to skip.

**Stable error codes for the alert endpoints:**
`alert_profile_not_found` (404), `alert_profile_disabled` (409, an action
on a disabled profile's queue), `alert_profile_invalid` (422),
`alert_rule_not_found` (404), `alert_rule_account_not_found` (404, an
unknown account in a rule's filter), `alert_rule_threshold_invalid` (422,
minimum exceeds maximum, or a negative bound), `alert_rule_condition_unsupported`
(422, a condition the event type's own capability does not support — a
quantity threshold on a follow rule, for instance), `alert_template_invalid`
(422, an unknown or malformed `{placeholder}`), `alert_queue_empty` (409,
Skip Current/Replay Previous with nothing to act on), `alert_queue_full`
(429, the profile's queue is at capacity and the new candidate is not a
strictly higher priority than the worst queued item). None of these
responses ever echoes a rendered alert's own text or username.

---

## Directory structure

```
.
├── apps/
│   ├── web/              # Operator panel (React + TypeScript + Vite)
│   │   ├── e2e/            # Real-browser Playwright regression suite (specs/, fixtures.ts)
│   │   └── src/
│   │       ├── api/        # Zod contracts + transport per feature area
│   │       ├── components/ # UI grouped by feature (platforms, chat, overlays,
│   │       │                # alerts, automation, goals, settings, layout, ui...)
│   │       ├── hooks/       # Queries, mutations, SSE client hooks
│   │       ├── i18n/        # Localization: config, en/pl resources, tests
│   │       ├── models/      # UI types, validation, reducers
│   │       └── pages/       # Route views
│   └── server/            # Backend (Go)
│       ├── cmd/server/      # Entry point, graceful shutdown, packaged-mode lifecycle
│       ├── cmd/testserver/  # `-tags integration` twin for fake-provider smoke tests
│       └── internal/
│           ├── domain/        # Persisted models + services, one package per feature
│           ├── runtime/       # Process supervisors (MediaMTX, FFmpeg branches,
│           │                  # engagement connectors, tray, single-instance...)
│           ├── httpapi/       # Router, handlers, middleware
│           ├── provider/      # Per-platform API clients (Twitch, YouTube, ...)
│           ├── secrets/       # OS credential-store abstraction
│           └── storage/sqlite/ # Connection, migrations, repositories
├── config/                # config/README.md — what belongs here (reverse-proxy examples)
├── docs/                  # docs/README.md — the documentation index
├── scripts/               # scripts/verify-*.mjs — canonical integration checks
├── LICENSE / PRIVACY.md / LEGAL.md / THIRD_PARTY_NOTICES.md
└── README.md
```

See [`docs/README.md`](README.md) for what each documentation file covers,
and each `internal/domain/<x>`'s own package for its exact scope — this tree
is intentionally a map, not an index kept in lockstep with every package.

---
