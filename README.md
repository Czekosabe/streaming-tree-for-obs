# Streaming Tree for OBS

A local application that lets you send **one** stream from OBS and branch it out
to several platforms at once — Twitch, YouTube, Kick, TikTok.

The name describes the model: the stream from OBS is the "trunk", and every
platform is an independent "branch". One branch failing does not stop the
others.

> ## Project state: persistent configuration
>
> **The application does not transmit anything yet.** What exists so far is the
> project structure, the documentation, the React operator panel with English
> and Polish interface languages, and a Go backend that stores platform
> configuration and stream metadata in a local SQLite database.
>
> You can add, edit and delete destinations and save their metadata, and it all
> survives a browser refresh and a backend restart. **Streaming itself is not
> implemented**: MediaMTX, FFmpeg, OAuth sign-in and platform API integrations
> **will be added in later stages**, so destinations are configuration only and
> the Start button is disabled.
>
> Whatever is still only a placeholder is marked with a **Demo** badge in the
> interface — the full list is in
> [What is currently demo-only](#what-is-currently-demo-only).

Detailed project description: [`docs/project-overview.md`](docs/project-overview.md)
Work journal: [`docs/progress.md`](docs/progress.md)

---

## Table of contents

- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Frontend — install and run](#frontend--install-and-run)
- [Go backend — running it](#go-backend--running-it)
- [Data storage](#data-storage)
- [REST API](#rest-api)
- [Production build](#production-build)
- [Lint, typecheck, tests and other checks](#lint-typecheck-tests-and-other-checks)
- [Interface languages](#interface-languages)
- [Directory structure](#directory-structure)
- [What is currently demo-only](#what-is-currently-demo-only)
- [Stream key security](#stream-key-security)
- [Common problems](#common-problems)

---

## Requirements

| Tool | Version | Purpose | Needed now? |
| ---- | ------- | ------- | ----------- |
| **Node.js** | 20.19+ or 22.12+ (22 LTS or newer recommended) | running the React panel | yes |
| **npm** | 10+ | installing frontend dependencies | yes |
| **Go** | 1.22 or newer | building and running the backend | yes |
| OBS Studio | 30+ | the source of the stream | not yet |
| MediaMTX | — | receiving the RTMP stream | not yet |
| FFmpeg | — | distributing the stream branches | not yet |

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
> dependencies. Details are in [`docs/progress.md`](docs/progress.md).

If you do not have Go yet, download it from <https://go.dev/dl/> and run the
installer for your system. It adds `go` to `PATH`; open a **new** terminal
window afterwards.

---

## Quick start

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
| `STREAMING_TREE_DATA_DIR` | per-user config directory | Directory that will hold the database file. See [Data storage](#data-storage). |
| `STREAMING_TREE_DB_PATH` | — | Full path to the SQLite file. Takes precedence over `STREAMING_TREE_DATA_DIR`. |

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

That line contains no credentials, because the application stores none.

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

No stream key, token or credential is seeded, stored or accepted anywhere.

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
`415` wrong content type, `422` validation failure, `500` internal failure.

**Provider definitions return semantic identifiers, never translated text.** The
backend sends `public`, `ultra-low`, `topic`; the frontend maps those to English
or Polish. The backend never decides the interface language.

**No endpoint accepts or returns a stream key, token or credential.** Unknown
JSON fields are rejected rather than silently ignored, so a stray credential
field produces an error instead of disappearing quietly.

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
npm run test        # unit tests (Vitest)
npm run build       # production build
```

**Backend** (from `apps/server`):

```bash
go build ./...      # compilation
go vet ./...        # static analysis
go test ./...       # tests
gofmt -l .          # lists files needing formatting (empty output = all good)
```

Backend tests always create their own temporary database in the test's temp
directory, so running them never touches your real one.

**Persistence check** (from the repository root):

```bash
node scripts/verify-persistence.mjs
```

This starts the backend against a temporary database, exercises the whole
platform API, restarts the process against the same file, verifies the data
survived, deletes what it created and removes the temporary directory. It never
opens your real database. It needs Go and Node on `PATH` and uses port 8199.

---

## Interface languages

The interface is available in **English** and **Polish**.

**English is the source and fallback language.** Every string is written in
English first; Polish is a translation of it. If a Polish entry were ever
missing, the interface falls back to English — a user never sees a raw
translation key.

The project uses [i18next](https://www.i18next.com/) with static, version
-controlled resource files. **No online translation API, browser translation
service, AI translation service or runtime automatic translation is used
anywhere.**

### Switching the language

The language switcher sits in the top bar on every page, and also under
**Settings → Interface language**. Switching applies immediately, without
reloading the page, and updates the `<html lang>` attribute.

### How the choice is stored

The selected language is saved in `localStorage` under the key
**`streaming-tree.language`**, and that is the only value the application stores
in the browser. It is validated on every read: an unsupported or corrupted value
falls back to English. On first launch the interface is always English — the
browser language is deliberately not detected.

Stream keys, tokens, credentials, platform configuration and stream metadata are
**never** stored in `localStorage`.

### Translation directory structure

```
apps/web/src/i18n/
├── config.ts                 # languages, namespaces, locales, storage key
├── types.ts                  # SupportedLanguage type and guards
├── index.ts                  # i18next instance and changeAppLanguage()
├── language-storage.ts       # reading/writing the preference
├── document-language.ts      # <html lang> synchronization
├── use-language.ts           # useLanguage() hook
├── i18next.d.ts              # compile-time key checking
└── resources/
    ├── en/                   # canonical source language
    │   ├── common.json       # shared labels, demo badge, units, durations
    │   ├── navigation.json   # sidebar, menu, OBS panel, version footer
    │   ├── dashboard.json    # dashboard, system status, backend, resources
    │   ├── platforms.json    # platform cards, statuses, quality, field options
    │   ├── metadata.json     # metadata editor, form labels, validation
    │   ├── pages.json        # page titles and planned-feature descriptions
    │   └── errors.json       # backend error messages and code mappings
    └── pl/                   # Polish translation, same structure
```

### Adding a new translation key

1. Add the key to the appropriate namespace in `resources/en/`. English defines
   the canonical structure.
2. Add the same key to `resources/pl/`.
3. Use it in a component: `const { t } = useTranslation('dashboard');` then
   `t('backend.heading')`.
4. Run `npm run i18n:check` and `npm run typecheck`.

Keys are type-checked against the English bundle, so a typo or a key removed
from English becomes a compile error rather than text rendered as its own key.

For countable values use pluralization instead of composing a sentence:

```jsonc
// en
"live_one": "{{count}} active stream",
"live_other": "{{count}} active streams"
```

```jsonc
// pl — Polish needs the full CLDR set
"live_one": "{{count}} aktywna transmisja",
"live_few": "{{count}} aktywne transmisje",
"live_many": "{{count}} aktywnych transmisji",
"live_other": "{{count}} aktywnej transmisji"
```

Never build a sentence by concatenating translated fragments — use one complete
entry with interpolation.

### Adding a future language

1. Create `apps/web/src/i18n/resources/<code>/` and copy the English namespace
   files into it.
2. Translate the values, using the plural categories that language requires
   (`npm run i18n:check` reports which ones are missing).
3. Register the language in `apps/web/src/i18n/config.ts`: add the code to
   `SUPPORTED_LANGUAGES`, its endonym to `LANGUAGE_LABELS` and its BCP 47 tag to
   `LANGUAGE_LOCALES`.
4. Add the imports to `apps/web/src/i18n/resources.ts`.
5. Run `npm run i18n:check`, `npm run typecheck` and `npm run test`.

The switcher picks up the new language automatically — it is rendered from
`SUPPORTED_LANGUAGES`.

### Checking translation consistency

```bash
cd apps/web
npm run i18n:check
```

English defines the canonical key structure. The script reports, with the full
path of each problem and a non-zero exit code:

- a key present in English but missing in another language,
- a key present in another language but missing in English,
- incompatible structures (an object where a string is expected, or vice versa),
- empty or whitespace-only values,
- missing or unexpected plural forms for that language.

It is plural-aware: Polish `_few` and `_many` entries are not reported as
mismatches against English `_one` and `_other`.

### What is not translated

- **User-created stream metadata** — titles, descriptions, tags and display
  names you type in are stored and shown verbatim, and are never translated
  automatically.
- **Platform brand names** — Twitch, YouTube, Kick, TikTok.
- **URLs and the RTMP address.**
- **API identifiers** — service name, version, backend error codes.
- **Stream language names** — shown as endonyms ("English", "Polski") the way
  the platforms themselves present them.

### Secrets and translation resources

Translation files are ordinary, version-controlled source files. **Stream keys,
tokens and any other secrets must never be placed in them**, exactly as with the
rest of the repository.

---

## Directory structure

```
.
├── apps/
│   ├── web/                    # Operator panel (React + TypeScript + Vite)
│   │   ├── scripts/            # check-i18n.mjs — translation consistency check
│   │   ├── src/
│   │   │   ├── api/            # Zod contracts + transport for the platform API
│   │   │   ├── app/            # TanStack Query configuration
│   │   │   ├── components/
│   │   │   │   ├── layout/     # Shell: sidebar, top bar
│   │   │   │   ├── metadata/   # Metadata editor with platform tabs
│   │   │   │   ├── platforms/  # Destination cards, add and settings dialogs
│   │   │   │   ├── system/     # System and backend status panels
│   │   │   │   └── ui/         # Base elements (buttons, inputs, panels, modal)
│   │   │   ├── data/           # DEMO DATA (host metrics only)
│   │   │   ├── hooks/          # Queries, mutations, cache helpers
│   │   │   ├── i18n/           # Localization: config, resources, tests
│   │   │   ├── lib/            # API client, error mapping, helpers
│   │   │   ├── models/         # UI types, validation, identifier mappings
│   │   │   └── pages/          # Route views
│   │   └── ...                 # Vite, TypeScript, ESLint, Vitest configuration
│   │
│   └── server/                 # Backend (Go)
│       ├── cmd/server/         # Entry point, graceful shutdown
│       └── internal/
│           ├── buildinfo/      # Service name and version
│           ├── config/         # Configuration and database path resolution
│           ├── domain/platform/# Provider registry, models, validation, service
│           ├── httpapi/        # Router, handlers, middleware, JSON responses
│           └── storage/sqlite/ # Connection, migrations, repository
│               └── migrations/ # Embedded .sql schema and seed
│
├── config/                     # MediaMTX and FFmpeg configuration (future stage)
├── docs/
│   ├── project-overview.md     # Full project description
│   └── progress.md             # Work journal
├── scripts/
│   └── verify-persistence.mjs  # Scripted restart-persistence check
├── .gitignore
└── README.md
```

---

## What is currently demo-only

Every item below is marked with a **Demo** badge in the interface, or described
directly next to the control.

| Element | What actually happens |
| ------- | --------------------- |
| **Start** button on platform cards | Disabled. There is no streaming engine, so nothing can be started. |
| Live / starting status, viewer counts, connection quality | **Removed.** These were invented values and would be misleading next to genuinely saved configuration. Cards report configuration only: configured, and enabled or disabled. |
| CPU, memory, disk, network | Fixed demo values. The backend does not collect host metrics. |
| OBS connection status | Always "Waiting for OBS". Nothing is listening on the RTMP port. |
| RTMP address in the sidebar | A planned address; it does not work. |
| Platform capability tables | An approximate configuration, now served by the backend. It has **not** been verified against the real Twitch, YouTube, Kick or TikTok APIs and needs re-checking when real integrations are implemented. |
| Platforms, Streams, Metadata, Logs pages | Informational views describing the planned scope. Not implemented. |

### What is real

- **Adding, editing and deleting destinations** through the API, stored in
  SQLite.
- **Editing and saving stream metadata**, including ordered Twitch tags.
- **Everything above survives a browser refresh and a backend restart.**
- **Provider capabilities** come from the backend, which is the single source of
  truth for them.
- **The language switcher**, in the top bar and under Settings.

### What will be added later

- **MediaMTX** — the local server receiving the RTMP stream from OBS.
- **FFmpeg** — one process per stream branch.
- **SSE or WebSocket** — live status instead of polling.
- **System credential store** — secure storage of stream keys.
- **OAuth and platform APIs** — sign-in and metadata publishing.

---

## Stream key security

A stream key allows broadcasting on someone's channel, so we treat it like a
password.

- **The repository contains no secrets** and must never contain any.
  `.gitignore` blocks `.env` files, database files and data directories.
- **The SQLite database stores no credentials.** No table has a column for a
  stream key, token or password, and no API payload carries one. Write endpoints
  reject unknown JSON fields, so a stray credential field produces an error
  rather than being silently dropped.
- **Keys will not be stored in the browser** — not in `localStorage`, not in
  `sessionStorage`, not in application state. The only value stored locally is
  the interface language preference.
- **The target location is the operating system credential store** (Windows
  Credential Manager, macOS Keychain, Secret Service).
- The backend will read a key only when starting a branch, and will not write it
  to the logs.

Stream key handling **has not been started yet**.

---

## Common problems

**The panel shows "Backend unavailable".**
The backend is not running, or is running on a different port. Start it in a
second terminal (`cd apps/server && go run ./cmd/server`) and use the refresh
button in the "Backend" card. This is an expected, fully handled state — the
panel does not crash. Your configuration is safe: it lives in the backend
database, which is why the dashboard cannot show it while the backend is down.

**My destinations disappeared.**
The backend is probably using a different database than before. Check the
`path=` value in the startup log and whether `STREAMING_TREE_DB_PATH` or
`STREAMING_TREE_DATA_DIR` is set in that terminal.

**A seeded destination I deleted did not come back.**
That is intended. The seed runs once, on a brand-new database, and is recorded
like any other migration.

**`go: command not found` or `'go' is not recognized`.**
Go is not installed or is not on `PATH`. Install it from <https://go.dev/dl/>
and open a new terminal window.

**`npm install` fails with "Cannot find native binding".**
Your Node version is older than the native dependencies require. Upgrade Node to
22.12+ or 24 LTS, delete `apps/web/node_modules` and
`apps/web/package-lock.json`, then install again.

**Port 8080 or 5173 is already in use.**
Backend: start it with a different `STREAMING_TREE_PORT` and add the new address
to `VITE_DEV_API_PROXY_TARGET` in `apps/web/.env.local`.
Frontend: Vite will offer the next free port; remember to add the new origin to
`STREAMING_TREE_ALLOWED_ORIGINS`.

**Interface changes are not visible.**
Check that `npm run dev` is still running and that there are no errors in the
browser console. If needed, reload the page bypassing the cache
(`Ctrl + Shift + R`).

**A label shows in English while the interface is set to Polish.**
That is the fallback working: the Polish entry is missing. Run
`npm run i18n:check` — it prints the exact path of every missing key.
