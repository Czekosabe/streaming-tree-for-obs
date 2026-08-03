# Streaming Tree for OBS

A local application that lets you send **one** stream from OBS and branch it out
to several platforms at once — Twitch, YouTube, Kick, TikTok.

The name describes the model: the stream from OBS is the "trunk", and every
platform is an independent "branch". One branch failing does not stop the
others.

> ## Project state: foundations
>
> **The application does not transmit anything yet.** What exists so far is the
> project structure, the documentation, the React operator panel with English
> and Polish interface languages, and a minimal Go backend with a health
> endpoint.
>
> MediaMTX, FFmpeg, OAuth sign-in, platform API integrations and the database
> **will be added in later stages**. Everything that is only a placeholder is
> marked with a **Demo** badge in the interface — the full list is in
> [What is currently demo-only](#what-is-currently-demo-only).

Detailed project description: [`docs/project-overview.md`](docs/project-overview.md)
Work journal: [`docs/progress.md`](docs/progress.md)

---

## Table of contents

- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Frontend — install and run](#frontend--install-and-run)
- [Go backend — running it](#go-backend--running-it)
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
│   │   │   ├── app/            # TanStack Query configuration
│   │   │   ├── components/
│   │   │   │   ├── layout/     # Shell: sidebar, top bar
│   │   │   │   ├── metadata/   # Metadata editor with platform tabs
│   │   │   │   ├── platforms/  # Stream branch cards
│   │   │   │   ├── system/     # System and backend status panels
│   │   │   │   └── ui/         # Base elements (buttons, inputs, panels)
│   │   │   ├── data/           # DEMO DATA
│   │   │   ├── hooks/          # Hooks (including the backend health query)
│   │   │   ├── i18n/           # Localization: config, resources, tests
│   │   │   ├── lib/            # API client, helper functions
│   │   │   ├── models/         # Domain model + Zod schemas
│   │   │   ├── pages/          # Route views
│   │   │   └── state/          # DEMO STATE (placeholder)
│   │   └── ...                 # Vite, TypeScript, ESLint, Vitest configuration
│   │
│   └── server/                 # Backend (Go)
│       ├── cmd/server/         # Entry point, graceful shutdown
│       └── internal/
│           ├── buildinfo/      # Service name and version
│           ├── config/         # Configuration from environment variables
│           └── httpapi/        # Router, handlers, middleware, JSON responses
│
├── config/                     # MediaMTX and FFmpeg configuration (future stage)
├── docs/
│   ├── project-overview.md     # Full project description
│   └── progress.md             # Work journal
├── .gitignore
└── README.md
```

---

## What is currently demo-only

Every item below is marked with a **Demo** badge in the interface, or described
directly next to the control.

| Element | What actually happens |
| ------- | --------------------- |
| **Start / Stop** buttons on platform cards | They only change state in the browser's memory. No process is started and no data is sent. |
| Platform statuses (offline / starting / live / error) | The initial state is hard-coded; "starting" becomes "live" after about 1.8 s. |
| Viewer count, connection quality | Fixed values. No platform is queried. |
| CPU, memory, disk, network | Fixed values. The backend does not collect host metrics. |
| OBS connection status | Always "Waiting for OBS". Nothing is listening on the RTMP port. |
| RTMP address in the sidebar | A planned address; it does not work. |
| Saving metadata | Goes only to the browser's memory. Reloading the page restores the initial values. |
| Platform capability tables | An approximate configuration prepared to demonstrate the editor. It needs verification when real integrations are implemented. |
| Platforms, Streams, Metadata, Logs pages | Informational views describing the planned scope. Not implemented. |

**The only real backend connection** at this stage is `GET /api/health`. Its
result is shown in the "Backend" card in the right-hand column.

The language switcher on the Settings page is **not** a placeholder — it is a
working feature.

### What will be added later

- **MediaMTX** — the local server receiving the RTMP stream from OBS.
- **FFmpeg** — one process per stream branch.
- **SQLite** — persistent storage of platform configuration and metadata.
- **SSE or WebSocket** — live status instead of polling.
- **System credential store** — secure storage of stream keys.
- **OAuth and platform APIs** — sign-in and metadata publishing.

---

## Stream key security

A stream key allows broadcasting on someone's channel, so we treat it like a
password.

- **The repository contains no secrets** and must never contain any.
  `.gitignore` blocks `.env` files and data directories.
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
panel does not crash.

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
