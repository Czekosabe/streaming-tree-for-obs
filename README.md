# Streaming Tree for OBS

A local application that lets you send **one** stream from OBS and branch it out
to several platforms at once — Twitch, YouTube, Kick, TikTok.

The name describes the model: the stream from OBS is the "trunk", and every
platform is an independent "branch". One branch failing does not stop the
others.

**Long-term vision.** Beyond routing one stream to several platforms, Streaming
Tree is planned to grow into a local streaming engagement and overlay
platform: normalized chat and events from multiple platforms, a unified
operator chat, OBS Browser Source overlays, alerts, scheduled bot messages and
chat commands, visual overlay designers, text-to-speech and goal widgets.
Most of that still does not exist — it is architecture and planning,
detailed in
[`docs/engagement-architecture.md`](docs/engagement-architecture.md) — but it
shapes decisions made today. The foundation is built incrementally: the
credential-store foundation (stage 5), the Twitch and YouTube
connected-account integrations (stages 7A/7B), the first real piece
of the engagement platform itself — a normalized Engagement Event Bus and
a real Twitch inbound connector reading chat and channel events (stage
8A) — a real, unified operator chat consuming that bus (stage 9) — and a
real, public OBS Browser Source chat overlay consuming that same
operator-chat projection (stage 10) — are all completed. Everything still
on top of that (outbound chat, alerts, TTS) remains planned.

> ## Project state: local ingest, outgoing FFmpeg streaming, Twitch + YouTube accounts, a real Twitch inbound Event Bus connector, a real unified operator chat and a real OBS Browser Source chat overlay all work
>
> Streaming Tree can **receive** a stream from OBS (a supervised, managed
> MediaMTX process), **store a destination's stream key securely** in the
> operating system credential store, **send it onward** (one independent
> FFmpeg process per enabled destination, plain stream copy, no
> re-encoding), and connect a real **Twitch** account (device-code
> sign-in) or a real **YouTube** channel (Authorization Code + PKCE
> sign-in, via a temporary loopback callback and a real system browser) —
> neither ever requests or stores a client secret — to **read and
> explicitly publish that destination's title, category and other
> platform metadata**. A YouTube destination additionally needs an
> explicitly selected live broadcast before it can publish. A connected
> Twitch account can also, after an explicit additional-permission step,
> **enable a real EventSub WebSocket connector** that normalizes chat
> messages, follows, subscriptions, gifts, cheers, raids, channel-point
> redemptions and remote stream online/offline events onto an in-memory
> **Engagement Event Bus**, viewable live on a diagnostic **Engagement**
> page and now also presented as a real, merged, working **Chat** page —
> badges, emotes, message deletion/clearing, activity events, filters and
> autoscroll all real. That same chat, filtered and re-shaped for a public
> audience, can now be pointed at as a real **OBS Browser Source** — the
> **Overlays** page manages any number of persisted overlay profiles, each
> with its own unguessable public URL, visual settings and filters, served
> over a public HTTP + Server-Sent Events API with no application chrome.
> See
> [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg),
> [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata),
> [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata),
> [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents),
> [Unified operator chat](#unified-operator-chat),
> [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)
> and [Stream key security](#stream-key-security).
>
> Starting a real broadcast is always an **explicit action** — a destination
> never starts on its own, and a backend restart never resumes one
> automatically. The same is true of publishing metadata: saving locally and
> publishing to the platform are two separate, both-explicit actions, for
> both Twitch and YouTube. Enabling the Twitch engagement connector is
> equally explicit, and restoring it automatically on the next backend
> start only ever applies to a connector you already enabled yourself.
>
> Kick/TikTok account integration, YouTube live-chat and Super Chat, and
> everything else still built **on top of** the operator chat — outbound
> chat and bot messages, alerts, TTS — are still **planned**. Whatever
> remains a placeholder is marked with a **Demo** badge — the full list is
> in [What is currently demo-only](#what-is-currently-demo-only).

Detailed project description: [`docs/project-overview.md`](docs/project-overview.md)
Work journal: [`docs/progress.md`](docs/progress.md)

---

## Table of contents

- [Roadmap](#roadmap)
- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Frontend — install and run](#frontend--install-and-run)
- [Go backend — running it](#go-backend--running-it)
- [Data storage](#data-storage)
- [Local ingest with MediaMTX](#local-ingest-with-mediamtx)
- [Connecting OBS](#connecting-obs)
- [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg)
- [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata)
- [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
- [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents)
- [Unified operator chat](#unified-operator-chat)
- [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)
- [REST API](#rest-api)
- [Production build](#production-build)
- [Lint, typecheck, tests and other checks](#lint-typecheck-tests-and-other-checks)
- [Interface languages](#interface-languages)
- [Directory structure](#directory-structure)
- [What is currently demo-only](#what-is-currently-demo-only)
- [Stream key security](#stream-key-security)
- [Common problems](#common-problems)

---

## Roadmap

| Stage | Scope | Status |
| ----- | ----- | ------ |
| 1–4 | Foundations, localization, SQLite configuration, MediaMTX ingest | **Completed** |
| 5 | Secure credential-store foundation | **Completed** |
| 6 | FFmpeg destination branches | **Completed** |
| 7A | Connected-account foundation and a first provider integration: Twitch device-code sign-in, account lifecycle, and explicit metadata publishing | **Completed** — see [progress.md](docs/progress.md) |
| 7B | YouTube account integration: Authorization Code + PKCE sign-in, channel selection, broadcast selection, and explicit metadata publishing | **Completed** — see [progress.md](docs/progress.md) |
| 7C | Kick and TikTok account integration | Deferred — capability-gated, not a prerequisite for Stage 8; Kick may land together with its engagement adapter in stage 15, TikTok remains conditional on a stable official integration |
| 8A | Engagement Event Bus and a real Twitch inbound connector | **Completed** — see [progress.md](docs/progress.md) |
| 8B | Additional Twitch event coverage, reserved only if 8A cannot safely cover the full verified event set | Planned, conditional |
| 9 | Unified operator chat: a real, merged Twitch chat view across connected accounts | **Completed** — see [progress.md](docs/progress.md) |
| 10 | OBS Browser Source chat overlay: persisted overlay profiles, a public per-overlay projection, a public HTTP/SSE API, a frontend renderer and the Overlays management page (this stage) | **Completed** — see [progress.md](docs/progress.md) |
| 11–19 | Outbound chat, alerts, bot automation, visual designers, templates, TTS, goal widgets, YouTube/Kick engagement connectors, external donations | Planned |
| 20 | Logs, diagnostics, packaging, remote-server hardening | Planned |

The full table with dependencies is in
[`docs/project-overview.md`](docs/project-overview.md#13-roadmap). The
engagement era (stages 8–19) is architected in detail in
[`docs/engagement-architecture.md`](docs/engagement-architecture.md) — read
that document's opening notice before treating any part of it as implemented.

---

## Requirements

| Tool | Version | Purpose | Needed now? |
| ---- | ------- | ------- | ----------- |
| **Node.js** | 20.19+ or 22.12+ (22 LTS or newer recommended) | running the React panel | yes |
| **npm** | 10+ | installing frontend dependencies | yes |
| **Go** | 1.25 or newer | building and running the backend (`go.mod` pins the floor) | yes |
| OBS Studio | 30+ | the source of the stream | yes, to actually publish something — the backend runs without it |
| MediaMTX | — | receiving the RTMP stream | yes — installed and supervised automatically, see [Local ingest with MediaMTX](#local-ingest-with-mediamtx) |
| FFmpeg | a recent build (4.4+ floor; actual compatibility is capability-probed, not version-matched) | sending each destination branch | yes, to actually start a destination — see [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg). The backend runs and the rest of the interface works without it. |

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
| `STREAMING_TREE_DATA_DIR` | per-user config directory | Application data directory: database, managed MediaMTX and generated configuration. See [Data storage](#data-storage). |
| `STREAMING_TREE_DB_PATH` | — | Full path to the SQLite file. Takes precedence over `STREAMING_TREE_DATA_DIR` for the database only. |
| `STREAMING_TREE_MEDIAMTX_PATH` | — | Full path to a MediaMTX executable you provide. Skips the managed installation. Must report the supported version. |
| `STREAMING_TREE_FFMPEG_PATH` | — | Full path to an FFmpeg executable you provide. Skips the `PATH` search. Must pass every capability probe. See [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg). |
| `STREAMING_TREE_MEDIAMTX_AUTOSTART` | `true` | Start MediaMTX when the backend starts. |
| `STREAMING_TREE_MEDIAMTX_AUTO_RESTART` | `true` | Restart MediaMTX automatically after an unexpected exit. |
| `STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` | `127.0.0.1:1935` | Address OBS publishes to. **Loopback only.** |
| `STREAMING_TREE_MEDIAMTX_API_ADDRESS` | `127.0.0.1:9997` | MediaMTX Control API address, read only by the backend. **Loopback only.** |
| `STREAMING_TREE_INGEST_PATH` | `live` | The single path publishing is allowed on. Letters, digits, `-` and `_` only. |
| `STREAMING_TREE_TWITCH_CLIENT_ID` | — | Twitch application Client ID. Always wins over a database-managed value if set. Never a client secret — see [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata). |
| `STREAMING_TREE_YOUTUBE_CLIENT_ID` | — | Google OAuth Desktop-app Client ID. Always wins over a database-managed value if set, independently of the Twitch variable above. Never a client secret — see [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata). |
| `STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE` | `1000` | The Engagement Event Bus's in-memory retained-event capacity. Must be between 100 and 10000; an out-of-range or non-numeric value is a startup error. See [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents). |
| `STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE` | `500` | The unified operator-chat projection's in-memory retained-item capacity — independent of the Event Bus's own. Must be between 100 and 5000; an out-of-range or non-numeric value is a startup error. See [Unified operator chat](#unified-operator-chat). |

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
[Stream key security](#stream-key-security) and
[Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata).

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

## Local ingest with MediaMTX

Streaming Tree receives the stream from OBS through
[MediaMTX](https://github.com/bluenviron/mediamtx), which it runs as a child
process. MediaMTX is third-party software under the MIT licence — see
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

### Pinned version

Only **MediaMTX v1.19.3** is supported. The version is pinned in one place in
the backend, and nothing resolves a "latest" release at runtime.

This matters because the generated configuration and the Control API client
target that exact schema, and MediaMTX refuses to start when it meets an unknown
configuration key. A binary reporting any other version is reported as
**incompatible** and is **not started**.

### Supported operating systems and architectures

| System | Architecture | Managed installation |
| --- | --- | --- |
| Windows | x86-64 (`amd64`) | yes |
| Linux | x86-64 (`amd64`) | yes |
| Linux | ARM64 (`arm64`) | yes |
| macOS | Intel (`amd64`) | yes |
| macOS | Apple Silicon (`arm64`) | yes |

Anything else — 32-bit Linux, ARMv6/ARMv7, FreeBSD, Windows on ARM — has no
managed installation. The interface says so clearly, and you can still point
`STREAMING_TREE_MEDIAMTX_PATH` at a compatible v1.19.3 binary you provide.

### How the binary is found

1. **`STREAMING_TREE_MEDIAMTX_PATH`** — an explicit path you set. Relative paths
   are made absolute, the file must exist and be executable, and its version is
   verified like any other. Reported as source `override`. **Streaming Tree
   never deletes or overwrites this file.**
2. **The managed installation** — what the application downloaded itself.
   Reported as source `managed`.
3. **Missing** — nothing usable was found.

The system `PATH` is deliberately **not** searched. Streaming Tree runs this
binary as a long-lived child process with a generated configuration, so it only
ever runs a copy it can identify.

### Installing MediaMTX

Installation is always an **explicit action** — nothing is downloaded when the
application starts.

In the interface, the sidebar and the **Streams** page show an **Install
MediaMTX** button whenever it is missing. The dialog states the exact version,
that it comes from the official GitHub release, that the checksum is verified,
and that MediaMTX is third-party software with its own licence.

Or through the API:

```bash
curl -X POST http://127.0.0.1:8080/api/runtime/mediamtx/install
```

The installer:

1. selects the official asset for your OS and architecture,
2. downloads `checksums.sha256` from the same release,
3. finds the entry for exactly that asset — no entry means no install,
4. downloads the archive over HTTPS, hashing it as it streams,
5. **discards it on any checksum mismatch**,
6. extracts into a temporary directory, rejecting absolute paths, `..`
   segments, symlinks, hard links and anything escaping the extraction root,
7. requires both the executable and the `LICENSE` file to be present,
8. runs the extracted binary once to confirm it reports v1.19.3,
9. moves the finished installation into place with an atomic rename.

Nothing unverified is ever executed. Temporary files are removed after success
and failure alike, and a failed reinstall leaves an existing working
installation untouched. A second install request while one is running returns
`409 Conflict`.

### Where it is installed

```
<application data directory>/
└── runtime/
    ├── mediamtx.yml                    generated configuration
    └── mediamtx/
        └── v1.19.3/
            └── <os>-<arch>/
                ├── mediamtx(.exe)
                ├── LICENSE             preserved from the official archive
                └── installation.json   version, asset name, SHA-256, timestamp
```

Version and platform are separate path segments, so future versions can sit side
by side. **No MediaMTX binary is ever committed to this repository**, and the
managed installation lives outside your working copy.

### Removing only the managed MediaMTX

Stop the backend and delete the `runtime/mediamtx` directory. Your platform
configuration and metadata are in `streaming-tree.db` and are **not** affected.

```bash
# Linux / macOS
rm -rf ~/.config/StreamingTree/runtime/mediamtx
```

```powershell
# Windows PowerShell
Remove-Item -Recurse "$env:AppData\StreamingTree\runtime\mediamtx"
```

The application then reports MediaMTX as missing and offers to install it again.

### Generated configuration

The backend regenerates `runtime/mediamtx.yml` every time it starts MediaMTX.
It is **generated output, not a file you edit** — manual changes are overwritten.

It enables RTMP and the Control API on their loopback addresses, and explicitly
disables RTSP, HLS, WebRTC, SRT, MoQ, metrics, pprof and playback. Each of those
opens its own listener by default, so none is left to the upstream default.
Exactly one path accepts publishing, with `overridePublisher: false` so a second
publisher cannot silently displace the first. Recording is off, and no
destination or credential appears anywhere in the file.

### Security model

- **Both listeners are loopback-only and this is enforced.** A non-loopback
  address is rejected at startup, not warned about. MediaMTX here accepts an
  unauthenticated publisher, and its Control API can rewrite its own
  configuration, so neither may be reachable from the network.
- **The browser never talks to the MediaMTX Control API.** Only the Go backend
  does. There is no proxy route.
- **The installation endpoint accepts no request body**, so no client can supply
  a download URL or a checksum.
- **No runtime path, process environment or process id is sent to the browser.**

### Process lifecycle and restart policy

The service moves through explicit states: `missing`, `installing`,
`incompatible`, `stopped`, `starting`, `ready`, `stopping`, `error`.

`ready` means the MediaMTX **Control API answered correctly** — not merely that
a process was spawned. A process that starts and immediately exits because a
port is taken never reports ready.

After an **unexpected** exit and with automatic restart enabled, MediaMTX is
restarted with exponential backoff from 1 s to 30 s, at most **5 times in 5
minutes**. Exceeding that stops the retries with an explanatory error instead of
spinning in a crash loop; 60 seconds of stable running resets both the backoff
and the counter.

**An explicit Stop is never undone by the restart policy.**

When the backend shuts down it drains HTTP first, then stops MediaMTX and waits
for it, so no child process is left behind.

> **Shutdown differs by platform.** On Linux and macOS MediaMTX is asked to stop
> with `SIGTERM` and only force-terminated if it has not exited within the grace
> period — a genuinely graceful shutdown. **On Windows it is terminated
> immediately**, because Windows has no `SIGTERM` and MediaMTX is a console
> application with no message loop to close. That is safe here: MediaMTX holds
> no unflushed persistent state, only listeners and in-memory sessions.

---

## Connecting OBS

Start the backend, install MediaMTX if needed, and wait for the service to
report **Running**. Then in OBS open **Settings → Stream** and choose
**Custom...**:

| OBS field | Value |
| --- | --- |
| **Server** | `rtmp://127.0.0.1:1935` |
| **Stream Key** | `live` |

Both values are shown in the sidebar and on the **Streams** page with copy
buttons, and both are derived from your configuration — if you change
`STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` or `STREAMING_TREE_INGEST_PATH`, the
displayed values change with them.

> ### The local stream key is not a secret
>
> `live` is a **route name** on your own machine — it tells MediaMTX which path
> you are publishing to. It is not a password, and it is not a destination
> platform stream key. It is safe to show in a screenshot or a support request.
>
> Real platform stream keys are an entirely separate concept. They are stored
> securely in the operating system credential store — see
> [Stream key security](#stream-key-security) — and are read only when you
> explicitly start that destination's outgoing branch, described next.

Once OBS starts streaming, the ingest status changes from **Waiting for OBS or
another RTMP publisher** to **Receiving an RTMP stream**, and the detected
tracks appear.

RTMP does not identify the publishing application, so Streaming Tree accepts any
RTMP publisher and never claims with certainty that it is OBS.

**Receiving the stream and sending it onward are two separate steps.** OBS
connecting here only makes the local ingest available; nothing goes out to
any platform until you explicitly start that destination — see the next
section.

---

## Outgoing streaming with FFmpeg

Once OBS is publishing to the local ingest, Streaming Tree can send that
stream onward to each configured destination independently, using
[FFmpeg](https://ffmpeg.org/) — one process per destination ("branch"),
pulling the shared local input and pushing to that destination's own
RTMP/RTMPS server.

### Why there is no managed FFmpeg download

MediaMTX has one official, checksummed GitHub release this application can
verify and install automatically (see
[Local ingest with MediaMTX](#local-ingest-with-mediamtx)). **FFmpeg has no
equivalent single official binary distributor** — official builds are source
only; every ready-to-run Windows/macOS/Linux binary comes from a third-party
packager with its own build configuration and licensing implications.
Silently downloading and running one of those on your behalf would mean
trusting a third party this project has not reviewed, so **Streaming Tree
never downloads FFmpeg**. You provide it, from whatever source you already
trust (your OS package manager, or a build you reviewed yourself), and this
application only ever *locates and probes* it.

### How the FFmpeg executable is found

1. **`STREAMING_TREE_FFMPEG_PATH`** — an explicit path you set. Relative
   paths are made absolute; the file must exist, be a regular file, and be
   executable. Reported as source `override`.
2. **A future bundled location** beside the backend executable — documented
   as a convention for a later packaged build; **no binary is bundled or
   committed today**, so this step currently finds nothing.
3. **The system `PATH`.** Unlike MediaMTX, FFmpeg has no single
   application-managed installation to prefer over it — searching `PATH` is
   the correct fallback here, precisely because there is no approved managed
   source to prefer instead. Reported as source `path`.
4. **Missing** — nothing usable was found. The backend keeps running;
   destinations simply report the `ffmpeg_missing` blocker until one is
   available.

**The resolved executable path is never sent to the browser** — only a
semantic source identifier (`override` / `bundled` / `path` / `missing`),
exactly like MediaMTX's own resolution.

### Compatibility policy: capability probing, not exact version matching

Streaming Tree does not pin one exact FFmpeg release the way it pins
MediaMTX. Instead it documents a **minimum supported version as a floor**
(currently 4.4) and probes the actual capabilities every branch needs:
`ffmpeg -version` parses cleanly, RTMP input, RTMP output, RTMPS output, the
FLV muxer, and `-progress` support. A binary that passes every probe is
compatible **regardless of how new it is** — a newer release is never
rejected merely for being newer than this code. A binary that fails any
probe is incompatible even if its reported version looks recent. This
matches how the real, local FFmpeg builds used while developing this stage
report themselves, and avoids treating "this exact string" as a proxy for
"has the features this application actually uses."

### Configuring a destination's output server

Each destination has its own **output settings**, separate from its stream
key: an **RTMP/RTMPS server URL** (`rtmp://` or `rtmps://`, host and
optional port required, no embedded credentials, no fragment) and an
**automatic-restart** toggle. Configure it in the platform's settings
dialog, or through the API (see [REST API](#rest-api)).

> ### The server URL is not the stream key
>
> The server URL is the address of the destination's RTMP ingest — the
> equivalent of OBS's "Server" field. The stream key is the separate secret
> that authorizes publishing to *your* channel on it, stored exactly as
> described in [Stream key security](#stream-key-security). Streaming Tree
> never joins them into one field in the interface, and the stored server
> URL alone is never enough to publish anywhere — it needs the key, which is
> retrieved only at the moment a branch actually starts.

Streaming Tree does not guess a provider's address format for you: a
provider definition may ship a verified default server URL, but if one has
not been confirmed against that platform's current documentation, the field
is left empty rather than filled with a guess.

### Starting and stopping a destination

Each destination's outgoing branch is started and stopped **independently
and explicitly** — there is no automatic start, on ingest arriving or
otherwise, in this stage. A branch becomes eligible to start only once
every one of these holds, in order, and the platform card / Streams page
explain whichever is missing:

1. the platform is enabled,
2. an output server URL is configured,
3. a stream key is stored,
4. the OS credential store is reachable,
5. a compatible FFmpeg was found,
6. the local MediaMTX ingest is ready,
7. OBS (or another publisher) is actually connected.

Starting a destination is a real, deliberate action that begins real
outgoing network transmission — the interface never disguises this as a
quiet background toggle, and starting more than one destination at once
(the **Start enabled destinations** bulk control) shows a confirmation
listing exactly which destinations will start, which are skipped and why,
and that outgoing bandwidth increases per active destination.

**Stream copy only.** No destination is transcoded in this stage: FFmpeg is
run with `-c copy`, so CPU cost stays low and quality is unchanged, but a
source codec FLV/RTMP cannot carry without transcoding makes that one
branch fail fast with a clear, sanitized error rather than silently starting
an expensive re-encode.

### Branch lifecycle and restart policy

Each branch has its own explicit state — `idle`, `blocked`,
`waiting_for_ingest`, `starting`, `live`, `restarting`, `stopping`, `error`
— tracked only in memory, never in SQLite. **`live` means FFmpeg has
reported real, advancing progress**, not merely that a process was spawned.

If OBS disconnects while a branch is running, that branch pauses
(`waiting_for_ingest`) rather than crash-looping against a missing input,
and resumes automatically once the input returns — but **only** for a
branch you explicitly started and have not explicitly stopped since. An
explicit **Stop** always wins: it clears the desire to run and is never
silently undone.

If a branch's own process fails unexpectedly (its destination connection
drops, for instance), it restarts with bounded exponential backoff (1 s up
to 30 s, at most 5 attempts in 5 minutes); exceeding that stops retrying
with a sanitized error instead of looping forever. **One destination
failing never affects another** — each has its own process, its own
backoff, and its own error state.

A backend restart resets every branch to `idle`/not-desired-running and its
restart counter to zero — **it never resumes a broadcast on its own** — while
the output settings themselves (server URL, automatic-restart preference)
persist in SQLite exactly like the rest of your configuration.

### Stream-key exposure on the command line — an honest limitation

The stream key is retrieved from the OS credential store only immediately
before a branch's FFmpeg process is spawned, and is never written to
SQLite, logged, placed in an error message, or returned by any API
response. FFmpeg's CLI was checked for a safer way to pass a per-run RTMP
destination (`ffmpeg -h protocol=rtmp`, `-h protocol=tcp`) and none exists
for this use case — no environment-variable or file-based alternative that
FFmpeg's RTMP output itself supports. **The destination URL, including the
key, is therefore passed as an FFmpeg command-line argument**, which on most
operating systems is visible to other processes owned by the same user (for
example, in a process list). This is accepted for this local, single-user
stage only with these mitigations: it is never logged by this application,
FFmpeg's own captured output is redacted before any logging or storage,
no API response ever contains a destination URL that includes the key, and
this application makes no claim of complete process-level secrecy.

### FFmpeg dependency and branch runtime endpoints

```bash
curl http://127.0.0.1:8080/api/runtime/ffmpeg
curl http://127.0.0.1:8080/api/runtime/branches
```

See [REST API](#rest-api) for the full endpoint list and response shapes.

### Verifying it for real

`scripts/verify-ffmpeg-branches.mjs` exercises this whole feature end to
end against a **real** FFmpeg executable and **real** MediaMTX instances —
a synthetic publisher, the real local ingest, a real branch process, and a
temporary destination MediaMTX standing in for the platform — entirely on
loopback, with no real platform account or credential. See
[Lint, typecheck, tests and other checks](#lint-typecheck-tests-and-other-checks).

---

## Connected accounts and Twitch metadata

Streaming Tree can connect to a real **Twitch** account and use it to read
and explicitly publish that destination's channel metadata (title,
category, language, tags). This was the first of several provider
integrations (stage 7A of the roadmap); YouTube now has its own real
integration too — see
[Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
below (stage 7B). Kick and TikTok account integration are still planned
(stage 7C).

**A connected account is not the same thing as a destination's stream
key.** They are separate facts about a destination, tracked and shown
separately: whether the destination is configured, whether a stream key is
stored, whether an output server is configured, whether a Twitch account is
connected and linked to it, whether the local ingest is receiving, whether
its FFmpeg branch is sending, and whether its metadata is in sync with
Twitch. Connecting a Twitch account never starts, stops, or otherwise
touches a destination's FFmpeg branch, and linking an account never
validates or replaces a stream key.

**What this stage (7A) does and does not implement.** This stage is the
account and metadata foundation: sign-in, account lifecycle, linking, and
explicit metadata publishing — not chat or events. Twitch's own chat and
channel events (EventSub) are a **later, separate stage, and that stage is
now real**: stage 8A added a genuine EventSub WebSocket connector reading
chat messages, follows, subscriptions, gifts, cheers, raids, channel-point
redemptions and remote stream status onto an in-memory Engagement Event
Bus, stage 9 added a real, working **unified operator chat page** that
consumes it, and stage 10 added a real, public **OBS Browser Source chat
overlay** that in turn consumes that same operator-chat projection — see
[Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents),
[Unified operator chat](#unified-operator-chat) and
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay). What
remains planned, unaffected by any of that: outbound Twitch chat and bot
messages, the alert engine, text-to-speech, donations from external
services, viewer counts, and analytics — see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).

### Registering a Twitch application and configuring a Client ID

1. Go to the [Twitch Developer Console](https://dev.twitch.tv/console/apps)
   and register a new application. Set its OAuth Redirect URL to
   `https://localhost` (unused by the flow this application performs, but
   Twitch requires one) and its Client Type to **Public**.
2. Copy the generated **Client ID**. Streaming Tree **never asks for,
   accepts, or stores a Client Secret** — the Settings page's Connected
   Accounts panel and every related API endpoint reject one outright (an
   unrecognized `clientSecret` field is a `400`), and the OAuth flow used
   (below) is a public-client flow that has no secret to send, including on
   refresh.
3. Provide the Client ID one of two ways:
   - **Environment variable** `STREAMING_TREE_TWITCH_CLIENT_ID` — always
     wins if set. The Settings page shows its source as "environment" and
     will not let you edit it there.
   - **Settings page**, when no environment variable is set — saved to
     SQLite (not a secret; it is public per Twitch's own client-type
     model), shown with source "database", and editable there.

   Changing a database-managed Client ID while any Twitch account is
   connected is rejected (`409`) — a different application can mean
   different or revoked tokens for existing accounts. Disconnect every
   Twitch account first, or set it to the exact same value (always
   allowed).

### Connecting an account — Device Code Flow

Streaming Tree uses Twitch's **Device Code Grant Flow**, the flow Twitch
documents for a public client with no way to keep a secret (as this
desktop-style local application is). Clicking **Connect Twitch** in
Settings:

1. asks the backend to start an authorization attempt with Twitch;
2. shows a short **user code** and a link to Twitch's activation page;
3. you open that link on any device, sign in, and enter the code;
4. the backend polls Twitch in the background (never faster than Twitch's
   own requested interval) until you finish, the code expires, or you
   cancel;
5. once authorized, the backend validates the token, confirms it was
   issued to the configured Client ID, confirms the required permission was
   granted, and fetches your Twitch login and display name for the account
   list.

The **device code** itself never reaches the browser — only the user code
(safe to display and copy) and the verification link do; there is no field
for it anywhere in the frontend's data model, because the backend's own API
response has no such field to send. Only one Twitch authorization attempt
may be in progress at a time.

The one permission requested is `channel:manage:broadcast` — the minimum
Twitch scope that allows reading and updating channel information. Nothing
broader (chat, subscriptions, Bits, moderation, email) is ever requested at
this stage.

### Account health, validation and reconnecting

A connected account is periodically re-validated against Twitch (at the
hourly interval Twitch's own documentation requires) and can be checked on
demand with **Check now**. If Twitch reports the token invalid, Streaming
Tree attempts one documented refresh (Twitch's refresh tokens rotate on
every use — the previous refresh token stops working the moment a new one
is issued, and Streaming Tree stores the new access and refresh token
together, atomically, so a partial failure never leaves a mismatched pair)
and re-validates the result. If that also fails, the account is marked
**Reconnect required**: publishing and category search stop working for it
until you click **Reconnect**, which repeats the device-flow authorization
for that same account. The same single-refresh-then-retry rule applies
transparently to every ordinary Twitch call this application makes (a
category search or a publish that hits an expired token retries exactly
once with a freshly refreshed token before giving up).

**Disconnect** revokes the account's token with Twitch where possible, then
removes it locally, then removes any destination link that pointed at it.
Twitch reporting the token as already invalid counts as a successful
revocation; a transient network failure leaves the account exactly as it
was so you can safely retry.

### Linking an account to a destination

Open a Twitch destination's settings and choose a connected account in its
own **Connected Twitch account** section — deliberately separate from the
stream-key section above it, since they are different credentials for
different purposes. One account can be linked to more than one destination
(useful if you configure the same channel as more than one destination
entry); a destination has at most one linked account, and linking a
different one replaces the link explicitly.

### Category selection, local Save, and publishing to Twitch

For a Twitch destination, the metadata editor's category field becomes a
search box backed by Twitch's real category/game search (needs a linked,
healthy account). Selecting a result stores both the display name and
Twitch's own stable category ID; typing over it without selecting a new
result leaves a stale ID, which blocks publishing until you search and
select again rather than guessing which category you meant.

**Save and Publish are two separate, both-explicit actions.** Save stores
metadata locally in Streaming Tree's own database, exactly as it always
has. **Publish to Twitch** sends the metadata **currently saved** to your
real Twitch channel — it is disabled, with an explanation, whenever the
form has unsaved edits, so you never publish a draft you have not saved.
Before publishing, a preview shows what would change: the current values on
Twitch, your saved local values, which fields would actually change, and
any reason publishing is currently blocked (no account linked, the account
needs reconnecting, no category selected, Twitch unreachable, Twitch's rate
limit reached). Publishing itself sits behind a confirmation dialog.

Only fields with a verified, real Twitch API equivalent are ever sent:
**title, category, language and tags** — via Twitch's real Modify Channel
Information endpoint. Twitch's channel API has **no** field for stream
description, a generic "mature content" flag, DVR, or a client-side latency
mode; sending real values for those fields to Twitch would either be
silently dropped by Twitch or misrepresent something Twitch does not
actually let this application control, so this application never sends
them and says so plainly in the publish preview instead. See
[`docs/provider-integrations/twitch.md`](docs/provider-integrations/twitch.md)
for the fully researched capability table, including exactly which fields
were previously guessed and have now been corrected.

Publishing **never** starts or stops a destination's FFmpeg branch, never
changes a stream key, and is never triggered automatically by saving
locally — it is always a separate, explicit click.

### Verifying it for real

`scripts/verify-twitch-account-integration.mjs` exercises this whole
feature end to end against the real backend and two small local fake
Twitch servers that reproduce only the response shapes this application
actually parses — device-code authorization, account finalization,
linking, category search, publishing, a forced token expiry and its
single-flight refresh, reconnecting, and disconnect/revocation — entirely
on loopback, with **no real Twitch account, application, or network
request to Twitch involved**. An optional, separate real-Twitch smoke test
is described in the task history but was not run as part of this stage —
see [`docs/progress.md`](docs/progress.md) for exactly what was and was not
verified against a real account.

---

## Connected accounts and YouTube metadata

Streaming Tree can connect to a real **YouTube channel** and use it to read
and explicitly publish a selected live broadcast's video metadata (title,
description, category, tags, language, visibility). This is stage 7B of
the roadmap, reusing the same connected-account foundation stage 7A built
for Twitch, adapted for how Google's own OAuth and the YouTube APIs
actually work — see
[`docs/provider-integrations/youtube.md`](docs/provider-integrations/youtube.md)
for the fully researched contract.

**A connected account, a selected broadcast, and a destination's stream
key are three separate facts**, tracked and shown separately: whether the
destination is configured, whether a stream key is stored, whether an
output server is configured, whether a YouTube channel is connected and
linked to it, whether a live broadcast is selected for it, whether the
local ingest is receiving, whether its FFmpeg branch is sending, and
whether its metadata is in sync with YouTube. Connecting a YouTube channel
or selecting a broadcast never starts, stops, or otherwise touches a
destination's FFmpeg branch, never validates or replaces a stream key, and
Streaming Tree never verifies that a selected broadcast is actually bound
to the stream key configured below it — that binding lives entirely on
YouTube's side.

**What this stage does not implement.** YouTube live-chat ingestion and
Super Chat/membership events are not implemented - Twitch is currently
the only live provider source feeding chat and events anywhere in this
application. The provider-independent operator **Chat** page itself
*is* implemented (stage 9) - see
[Unified operator chat](#unified-operator-chat). The public **OBS
Browser Source chat overlay** built on top of that same chat is also now
implemented (stage 10) - see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay) below
and [`docs/obs-browser-source.md`](docs/obs-browser-source.md) for the
OBS-specific research it is built on. Outbound chat, alerts,
text-to-speech, donations, automatic broadcast creation, automatic
`liveStream` binding, and automatic stream-key retrieval from YouTube
remain unimplemented - see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).
This stage is the account, broadcast-selection, and metadata
foundation those still-planned features will build on later, not an
implementation of them.

### Registering a Google Cloud project and configuring a Client ID

1. Create a project in the [Google Cloud console](https://console.cloud.google.com/),
   then enable **YouTube Data API v3** for it (APIs & Services → Library).
2. Under APIs & Services → Credentials, create an OAuth client of type
   **Desktop app**. Google does not require (and this application never
   sends) a client secret for this client type.
3. Copy the generated **Client ID**. Streaming Tree **never asks for,
   accepts, or stores a Client Secret**, and rejects a pasted complete
   `credentials.json` file outright rather than silently extracting the
   secret from it — the Settings page's YouTube panel and every related API
   endpoint accept only a bare Client ID (an unrecognized field is a `400`).
4. Provide the Client ID one of two ways:
   - **Environment variable** `STREAMING_TREE_YOUTUBE_CLIENT_ID` — always
     wins if set. The Settings page shows its source as "environment" and
     will not let you edit it there.
   - **Settings page**, when no environment variable is set — saved to
     SQLite (not a secret), shown with source "database", and editable
     there.

   Changing a database-managed Client ID while any YouTube account is
   connected is rejected (`409`) — the same policy as Twitch's Client ID,
   and independent of it (changing one never affects the other).

**Testing-mode limitation.** A newly created Google Cloud project's OAuth
consent screen defaults to **Testing** publishing status, under which
Google expires every authorization and refresh token it issues after
**seven days**, regardless of what this application requests. This is a
Google-side limitation Streaming Tree cannot detect or work around — only
notice the symptom (a channel unexpectedly needing to be reconnected) and
surface it as **Reconnect required**, the same as any other refresh
failure. The Settings page shows a standing notice about this.

### Connecting a channel — Authorization Code Flow with PKCE

Streaming Tree uses Google's **Authorization Code Flow with PKCE**, the
flow Google documents for a Desktop-app OAuth client — not Twitch's
device-code flow, and not Google's own TV/limited-input device flow either
(that exists for a different class of device; this is a desktop
application with a full browser and keyboard already available). Clicking
**Connect YouTube** in Settings:

1. asks the backend to start an authorization attempt: it generates a
   random attempt ID, a high-entropy PKCE verifier, its S256 challenge, a
   random CSRF state value, and binds a temporary HTTP listener to
   `127.0.0.1` on a port the operating system picks;
2. shows an **Open Google authorization** button — clicking it opens
   Google's real sign-in and consent page in your system browser; nothing
   opens automatically;
3. you sign in and approve access on Google's own page;
4. Google redirects your browser back to the temporary loopback listener,
   which the backend closes right after handling that one request; the
   backend then exchanges the authorization code for a token directly with
   Google — no client secret is sent, ever;
5. if the Google account owns more than one YouTube channel, Streaming
   Tree shows every channel it found and asks you to pick one explicitly —
   it never silently picks the first one;
6. once finalized, the backend validates the token, confirms the required
   permission was granted, and records the channel's title and thumbnail
   for the account list.

The **authorization code**, the **PKCE verifier**, and the **CSRF state
value** never reach the frontend at all — there is no field for any of
them anywhere in the frontend's data model, because the backend's own API
response has no such field to send. Only one YouTube authorization attempt
may be in progress at a time.

The one permission requested is
`https://www.googleapis.com/auth/youtube.force-ssl` — the narrowest scope
that covers reading channel/broadcast/video data and updating video
metadata. Nothing broader (email, Google profile, Drive, Analytics,
monetization, chat) is ever requested at this stage, and the connected-
account identity is the YouTube channel ID — Streaming Tree never stores
or displays the Google account's email address.

### Account health, validation and reconnecting

A connected YouTube account is validated against Google (`GET
https://oauth2.googleapis.com/tokeninfo`) right after authorization, once
per backend startup, and can be checked on demand with **Check now** — no
official Google requirement mandates hourly re-validation the way Twitch's
own documentation does, so this application does not poll Google that
often for YouTube. If validation fails, Streaming Tree attempts one
documented refresh; Google's refresh response typically **omits** a new
refresh token, in which case the previously stored one is preserved rather
than lost (Twitch, by contrast, always rotates its refresh token on every
use — the two providers are handled according to their own actual
behavior, not a shared assumption). If refresh also fails (Google reports
`invalid_grant` — typically a revoked grant, or the Testing-mode seven-day
expiry above), the account is marked **Reconnect required**: publishing,
broadcast listing, and category listing stop working for it until you
click **Reconnect**, which repeats the Authorization Code + PKCE flow for
that same channel identity — authorizing a *different* channel during a
reconnect is rejected rather than silently swapping which channel the
account represents.

**Disconnect** revokes the account's token with Google where possible,
then removes it locally, then removes any destination link **and any
selected-broadcast target** that pointed at it. Google reporting the token
as already invalid counts as a successful revocation; a transient network
failure leaves the account exactly as it was so you can safely retry.

### Linking a channel and selecting a broadcast

Open a YouTube destination's settings and choose a connected channel in
its own **Connected YouTube channel** section, then choose a live
broadcast in the separate **Selected broadcast** section below it — both
deliberately separate from the stream-key section, since all three are
different facts. The broadcast selector lists only your channel's
**active** and **upcoming** broadcasts (never a "persistent" one — Google
deprecated those in 2020) and never auto-selects one; if a previously
selected broadcast can no longer be found, the section says so plainly
rather than silently clearing it. Creating a broadcast happens in YouTube
Studio — Streaming Tree does not create one for you.

### Category selection, region, local Save, and publishing to YouTube

For a YouTube destination, the metadata editor's category field becomes a
dropdown backed by YouTube's real category list for an explicit **region**
(YouTube categories are region-scoped, not a text search the way Twitch's
are). The effective region defaults to the connected channel's own country
when YouTube reports one; otherwise you choose a region explicitly — there
is no silent fallback to the interface language, which is an unrelated
setting. Selecting a category stores both its display name and YouTube's
own stable category ID.

**Save and Publish are two separate, both-explicit actions**, exactly like
Twitch. Save stores metadata locally in Streaming Tree's own database.
**Publish to YouTube** sends the metadata **currently saved** to your
selected broadcast's underlying video — disabled, with an explanation,
whenever the form has unsaved edits or no broadcast is selected. Before
publishing, a preview shows the selected broadcast, the current values on
YouTube, your saved local values, which fields would actually change, and
any reason publishing is blocked (no channel linked, the channel needs
reconnecting, no broadcast selected, live streaming not enabled for the
channel, no category region set, no category selected, YouTube
unreachable, YouTube's quota exceeded) — plus standing warnings such as the
Testing-mode seven-day note and that the selected broadcast and the stored
stream key are not verified as belonging together.

Only fields with a verified, real YouTube Data API equivalent are ever
sent: **title, description, category, tags, language, and visibility** —
via a safe read-modify-write against the video's real `videos.update`
endpoint (Google's own API deletes any mutable property a submitted part
omits, so Streaming Tree always re-fetches the current resource
immediately before writing and only overwrites the fields it actually
manages). YouTube's real API has **no** generic "mature content" flag (its
closest field, made-for-kids, is a COPPA child-directed disclosure, not a
maturity rating), and this stage does not write DVR or latency-mode
settings either (both are broadcast-lifecycle properties a future stage
may add). See
[`docs/provider-integrations/youtube.md`](docs/provider-integrations/youtube.md)
for the fully researched capability table.

Publishing **never** starts or stops a destination's FFmpeg branch, never
changes a stream key, never creates a broadcast, and is never triggered
automatically by saving locally.

### Verifying it for real

`scripts/verify-youtube-account-integration.mjs` exercises this whole
feature end to end against the real backend and two small local fake
Google servers that reproduce only the response shapes this application
actually parses — Authorization Code + PKCE authorization (including a
wrong-CSRF-state callback and explicit multi-channel selection), account
finalization, linking, broadcast selection, category/region, publishing, a
forced token expiry and its single-flight refresh (including Google's
omitted-refresh-token response), restart persistence, reconnecting, and
disconnect/revocation — entirely on loopback, with **no real Google
account, Google Cloud project, or network request to Google/YouTube
involved**. No real-Google smoke test exists or was run for this stage —
see [`docs/progress.md`](docs/progress.md) for exactly what was and was
not verified.

---

## Engagement Event Bus and Twitch chat/events

Streaming Tree can normalize a connected Twitch account's chat messages
and channel events (follows, subscriptions, gifts, cheers, raids,
channel-point redemptions, and remote stream online/offline) onto an
in-memory **Engagement Event Bus**, and stream them live to a new
diagnostic **Engagement** page in the interface. This is stage 8A of the
roadmap — the foundation later stages build the unified operator chat
(stage 9), the OBS Browser Source overlay (stage 10), outbound chat and
bot messages (stage 11), and the alert engine (stage 12) on top of. See
[`docs/engagement-architecture.md`](docs/engagement-architecture.md) for
the full target design and
[`docs/provider-integrations/twitch-engagement.md`](docs/provider-integrations/twitch-engagement.md)
for the fully researched Twitch EventSub contract.

**What this stage does not implement.** Sending Twitch chat messages,
chat commands, scheduled bot messages, alert rules or rendering, TTS,
YouTube live chat, and Kick/TikTok engagement are all still
unimplemented — see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md). A
real, unified operator chat consuming this Event Bus is implemented —
see [Unified operator chat](#unified-operator-chat) below — and a real,
public OBS Browser Source overlay consuming that chat's own projection
in turn is also implemented — see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay). The
diagnostic Engagement page added in this stage is explicitly **not**
the operator chat or an overlay — it exists to make the Event Bus and
the Twitch connector's state genuinely observable, and stays a
separate page from both Chat and Overlays.

### The Engagement Event Bus

The Event Bus (`internal/engagement`) is a concurrency-safe, in-process
component: a bounded ring buffer of recently published normalized events
(default capacity 1000, configurable via
`STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE`), bounded deduplication against
redelivered provider notifications, and live delivery to every connected
subscriber — the same Server-Sent Events endpoint the diagnostic
Engagement page reads from, never a direct connection to Twitch. Neither
the operator Chat page nor the OBS chat overlay reads this endpoint
directly; both read through the operator-chat projection instead (see
[Unified operator chat](#unified-operator-chat) and
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)).
**It is in-memory only.** No normalized event, and no chat message, is
ever written to SQLite; the entire buffer resets to empty on every
backend restart, exactly like MediaMTX's own runtime state.

A slow subscriber can never block event publication or another
subscriber: if a subscriber's own buffered channel is full when a new
event arrives, that subscriber is dropped with an explicit signal rather
than allowed to make the whole bus wait on it.

### Enabling Twitch chat and events — an explicit permission upgrade

A Twitch account connected under
[Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata)
above already exists with only the metadata scope
(`channel:manage:broadcast`). Reading chat and events needs five
additional, narrowly-scoped permissions: `user:read:chat`,
`moderator:read:followers`, `channel:read:subscriptions`, `bits:read`
and `channel:read:redemptions` — never `user:write:chat` (sending chat
messages), which belongs to a later stage this one does not implement.

Clicking **Authorize engagement access** on the Engagement page starts a
new Device Code Flow attempt, reusing the exact same flow the initial
Twitch connection used, requesting the **union** of the account's
current scopes and the five above — it can only add permission, never
remove any the account already has. The newly authorized identity must
match the existing connected account exactly; a different Twitch login
is rejected rather than silently creating a second, competing
connection. The previous, working token stays in place until the
upgrade completes successfully, then is atomically replaced.

**Metadata health and engagement-permission health are tracked
independently.** An account missing the engagement scopes remains fully
healthy for metadata publishing — it is never marked "Reconnect
required" merely because an optional capability was never authorized.

### The Twitch EventSub connector

Once the engagement scopes are granted, toggling **Enable Twitch chat
and events** on the Engagement page opens one supervised WebSocket
connection to Twitch's EventSub endpoint for that account and creates
the selected subscriptions (chat messages, message deletion, chat
clear, user-message clear, follows, subscriptions, subscription gifts
and gift batches, resubscription messages, cheers, incoming raids,
channel-point redemptions, and remote stream online/offline — thirteen
subscription types in total). Enabling or disabling is always an
explicit action, exactly like starting or stopping a destination branch;
an enabled connector reconnects automatically after a backend restart,
a disabled one does not.

The connector's own state is shown plainly: connecting, waiting for
Twitch's welcome message, subscribing, connected, reconnecting,
stopping, blocked (missing permission or configuration), or error
(for example, after Twitch revokes authorization) — never collapsed
into a single "on/off" indicator. **This is a distinct fact from
whether OBS is streaming, whether the local ingest is receiving, or
whether a destination's FFmpeg branch is sending** — a connected,
subscribed EventSub connector says nothing about whether this
application's own outgoing stream to Twitch is live, and vice versa.

Twitch does not replay events lost during an ordinary connection loss.
When that happens, the connector reconnects with bounded backoff,
recreates its subscriptions, and honestly marks a **possible data gap**
rather than claiming seamless recovery. Twitch's own official
`session_reconnect` handoff (a graceful migration to a new connection,
distinct from an ordinary loss) is handled without recreating
subscriptions and without a data-gap marker, exactly as Twitch's own
documentation describes.

### Normalized events and the diagnostic Engagement page

Every event — from any provider, in future stages — is normalized to
the same versioned shape before reaching the bus: a monotonically
increasing sequence number, a stable internal ID, the provider and
connected account it came from, a normalized type (`chat.message`,
`chat.message_deleted`, `chat.cleared`, `moderation`, `follow`,
`subscription`, `resubscription`, `gifted_subscription`,
`subscription_gift_batch`, `bits`, `raid`, `channel_point_redemption`,
`stream.online`, `stream.offline`), ordered chat message fragments
(text, emote, cheermote, mention), a user identity block (never
inventing an avatar or color the provider did not itself report, never
fabricating an identity for an anonymous gift or cheer), and — where
applicable — an amount, currency, or quantity. A gift-batch event
("gifted 5 subs") and each individual gifted-subscription recipient
event are kept as genuinely separate events, never collapsed into one.

The Engagement page shows the bus's own status (retained event count,
buffer capacity, oldest/newest sequence), a card per connected Twitch
account's connector, and a bounded, plain-text recent-events feed fed
live over Server-Sent Events — no message bubbles, no theming, no
animation, explicitly not styled as the finished chat overlay a later
stage will build.

### Verifying it for real

`scripts/verify-twitch-engagement.mjs` exercises this whole feature
end to end against the real backend, fake Twitch OAuth and Helix
servers, and a small hand-rolled fake Twitch EventSub WebSocket server
(Node has no built-in WebSocket server, and this project added no new
npm dependency to get one) — the permission-upgrade scope union,
subscription creation, event normalization and deduplication across
many event types, Twitch's official `session_reconnect` handoff (no
resubscription, no data gap), an ordinary disconnect (a data gap
recorded, subscriptions recreated), authorization revocation, restart,
disable, and disconnect — entirely on loopback, with **no real Twitch
account or network request to Twitch involved**. A representative
subset of scenarios is covered by Go unit tests instead of this script
(malformed/oversized-frame handling, keepalive-timeout-triggered
reconnects, and others needing precise timing control a fake clock
provides more reliably than a real WebSocket exchange) — see
[`docs/progress.md`](docs/progress.md) for exactly which.

---

## Unified operator chat

Stage 9 adds a real, working **Chat** page (`/chat`): merged, live
Twitch chat across every connected account whose engagement connector
is enabled, distinct from the Engagement page's connector diagnostics.
Chat = the daily working view; Engagement = "is the connector actually
healthy." Neither replaces the other.

**What this stage does not implement.** Sending chat, chat commands,
scheduled bot messages, alerts, TTS, remote moderation actions
(bans/timeouts/message deletion sent *to* Twitch), and YouTube/Kick/TikTok
chat all remain exactly as planned before this stage — a message
appearing in operator chat is never proof this application's own
outgoing FFmpeg branch works; that is an unrelated, separately verified
fact. The public OBS Browser Source overlay built on top of this same
projection **is** implemented (stage 10) — see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay).

### The operator-chat projection

`internal/operatorchat` subscribes to the same Engagement Event Bus and
converts normalized events into a chat-shaped, lifecycle-aware public
item model — never the other way around; the projection never mutates
the Event Bus's own retained history, and it never imports the Twitch
provider package. It is **in-memory only**, independently bounded from
the Event Bus (default capacity 500, configurable via
`STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE`, 100–5000) and begins empty
on every backend start — nothing about this stage claims pre-restart
chat history.

Every revision — a brand-new message/activity/moderation/system item,
or a lifecycle update to an existing one (a message becoming deleted) —
is a complete "upsert" carrying its own monotonically increasing
sequence, replayed identically by the bounded snapshot endpoint and the
live SSE stream. A message becoming deleted updates the **same** item
id in place, with its original content preserved and a visible deleted
marker — it is never silently removed and never produces a second row.
A deletion referencing a message no longer retained produces a small,
honest moderation item instead of inventing content. A whole-chat clear
or a per-user clear ("clear this user's messages," which needs no
reference to any specific prior message) is scoped exactly to the
provider/account/user it targets — never a different account, never a
different user.

### Merged accounts, badges and emotes

A user may have more than one connected Twitch account; the merged
timeline never guesses which one "the" source is — every item carries
its own connected-account id, and the account label is shown only when
there is more than one account contributing (never a name nobody needs
to disambiguate). Twitch chat badges are resolved from the
channel-specific catalog first, then the global one, through a
bounded, TTL'd (1 hour), single-flight cache — see
[`docs/provider-integrations/twitch-engagement.md`](docs/provider-integrations/twitch-engagement.md)'s
Stage 9 addendum for the full research this is built on, including
where that channel-then-global order is this project's own defensible
inference rather than an explicitly documented Twitch rule. Emote
images are built as a pure URL from the fragment's own emote id — no
catalog fetch, no cache, nothing that can go stale. A badge or emote
that cannot be resolved is simply omitted or falls back to its plain
text; the chat message itself is never discarded or blocked on it.

### Filters, settings and privacy

The Chat page filters by connected account (persisted per account) and
by explicitly-hidden or bot-marked users (a small "hide this
user"/"mark as bot" action per message, backed by its own persisted
list — identified by the provider's own stable user id, never a
display name someone can change, and never a heuristic guessing "bot"
from a username). Display preferences (platform icon/name, account
label, badges, timestamps, activity events, deleted messages, command
messages, compact mode) persist in SQLite and apply immediately while
being edited, saved only on an explicit action. **None of this is chat
content**: no message text, no username treated as authoritative
identity, no token, and no raw provider event is ever persisted — see
the migration's own scope note in
`apps/server/internal/storage/sqlite/migrations/0010_operator_chat_preferences.sql`.
The timeline auto-scrolls while at the bottom, pauses the moment an
operator scrolls up, and offers an explicit "Jump to latest" control
with an unseen count rather than silently stealing the viewport.

### Verifying it for real

`scripts/verify-operator-chat.mjs` exercises the whole stack end to end
against the real backend and the same kind of fake Twitch OAuth/Helix/
EventSub servers `verify-twitch-engagement.mjs` uses (extended with
fake `GET /chat/badges/global` and `GET /chat/badges` routes) — badge
channel-then-global resolution with cache-hit counts, the emote CDN
URL, an exact deletion updating the same item id, a per-user clear and
a whole-chat clear each correctly scoped, every activity type
(including the gift batch staying distinct from its recipient, and
bits never labeled a donation), preferences/hidden-user/bot-user
persistence surviving a real backend restart while chat content itself
resets to empty, and the SSE stream — entirely on loopback, with **no
real Twitch account or network request to Twitch involved**. A
representative subset of scenarios (a second connected account merging
into the timeline, a deliberately forced projection-side gap) is
covered by Go unit tests instead — see
[`docs/progress.md`](docs/progress.md) for exactly which.

---

## OBS Browser Source chat overlay

Stage 10 adds a real, public **OBS Browser Source chat overlay**: any
number of persisted overlay profiles, each rendering a filtered,
presentation-shaped view of the same merged Twitch chat the operator
**Chat** page shows, served over its own unauthenticated public HTTP +
Server-Sent Events API for OBS's Browser Source (or a plain browser tab)
to consume directly — no application chrome, no sidebar, no operator
login. Manage overlays on the **Overlays** page (`/overlays`); each
overlay's own public URL points at `/overlay/chat/{publicSlug}`. See
[`docs/obs-browser-source.md`](docs/obs-browser-source.md) for the
underlying OBS Browser Source research (setup, recommended dimensions,
the shutdown/refresh checkbox trade-off) this feature is built on.

**What this stage does not implement.** A visual overlay designer,
exportable/importable overlay templates, alerts, TTS, and YouTube/Kick/
TikTok overlay support are all still unimplemented — only Twitch chat
reaches any overlay, exactly like the operator Chat page above. See
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).

### Persisted overlay profiles

Each overlay profile (`internal/domain/chatoverlay`, five SQLite tables
added by migration `0011`) stores its own layout, visibility toggles,
filters, typography, colors, animation and role-highlighting settings as
explicit, individually validated columns — never a settings JSON blob.
An overlay has its own management id and a separate, higher-entropy
**public slug** (160 bits via `crypto/rand`) that can be rotated
independently at any time, immediately invalidating the old public URL.
An overlay's own hidden-user list is deliberately separate from the
operator Chat page's hidden-user list — a user can stay visible to the
operator while being hidden from one specific public overlay.

### The public overlay projection

`internal/chatoverlay` is a second, independent consumer of the
operator-chat projection's own revision stream (stage 9's
`internal/operatorchat`) — it never subscribes to the Engagement Event
Bus directly, so none of stage 9's lifecycle, deduplication or badge/
emote-resolution logic is duplicated. For every overlay it keeps its own
filtered, bounded, in-memory current-item view plus a separate revision
ring (fixed capacity, not configurable) for live Server-Sent Events
replay. Moderation and system items never reach any public overlay,
regardless of settings; a deleted message is either removed outright or
replaced with a placeholder that never carries the original text,
depending on the overlay's own setting. A settings change triggers an
immediate rebuild and a public reset — visible on a connected Browser
Source within moments of clicking Save.

### The public and management APIs

`GET /api/public/chat-overlays/{publicSlug}/config`, `/items` and
`/stream` require no authentication (the public slug itself is the only
thing standing between the URL and its content, exactly like every other
public overlay tool) and never answer an unknown or disabled slug with a
hard error — a Browser Source instead gets an empty, transparent overlay,
matching how a live broadcast should degrade. The management API
(`/api/chat-overlays/...`) creates, edits, deletes and rotates overlays,
and manages each overlay's own accounts, hidden users, blocked terms and
activity-type selection. The frontend renderer
(`apps/web/src/components/chat-overlay/`) is shared, unchanged, between
the real public route (`/overlay/chat/:publicSlug`, with no `<AppShell>`
anywhere in its render tree) and the Overlays management page's own live
preview panel.

### Verifying it for real

`scripts/verify-chat-overlay.mjs` exercises the whole stack end to end
against the real backend and the same kind of fake Twitch OAuth/Helix/
EventSub servers the other engagement scripts use — safe defaults, a
live message reaching a filtered public overlay, every filter (accounts,
hidden users, bots, commands, blocked terms, activity types), capacity/
expiry eviction, deletion/clear scoping, slug rotation, restart behavior,
and a final scan confirming no chat text, blocked-term value, hidden-user
data or public slug ever appears in a log line — entirely on loopback,
with **no real Twitch account or OBS installation involved**. See
[`docs/progress.md`](docs/progress.md) for exactly what it covers.

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
| `POST` | `/api/chat-overlays` | Create an overlay profile with safe defaults and a fresh, unguessable public slug. Responds 201 with a `Location` header. |
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
npm run test        # unit tests (Vitest), plus a set of rendered-component tests (React Testing Library) covering the Twitch device-flow and YouTube OAuth modals, disconnect/publish confirmations, the Engagement page/connector card/event feed, the Chat page/message/activity/moderation rows, and the chat-overlay renderer/Overlays management page
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
```

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
[Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
for the full list of what it covers.

The Twitch-engagement script adds a third fake: a small, hand-rolled
Twitch EventSub WebSocket server (this project added no new dependency
to get one — see the script's own header comment). It covers the
identity-bound permission-upgrade scope union, exact subscription
creation, event normalization and deduplication, Twitch's official
`session_reconnect` handoff, an ordinary disconnect's data-gap handling,
revocation, restart, disable and disconnect. See
[Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents)
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
[Unified operator chat](#unified-operator-chat) for the full list of
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
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay) for
the full list of what it covers and what is instead covered by Go unit
tests.

**None of these scripts touch your real database, your managed MediaMTX
installation, your real OS credential store, or a real Twitch/Google
account**, and all remove their temporary directories afterwards.

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
    │   ├── errors.json       # backend error messages and code mappings
    │   ├── runtime.json      # MediaMTX/ingest state, dependency status
    │   ├── accounts.json     # Twitch integration, device flow, account link, publish
    │   ├── engagement.json   # Event Bus status, connector state, diagnostic event feed
    │   └── chat.json         # unified operator chat: kinds, activity/moderation labels, filters, settings, autoscroll
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
│   │   │   ├── api/            # Zod contracts + transport for the platform, account, engagement, operator-chat and chat-overlay API
│   │   │   ├── app/            # TanStack Query configuration
│   │   │   ├── components/
│   │   │   │   ├── chat/       # Message/activity/moderation rows, filter bar, settings panel, badge/emote images
│   │   │   │   ├── chat-overlay/ # The public overlay renderer tree (Stage 10) - shared by the public route and the Overlays preview panel
│   │   │   │   ├── engagement/ # Twitch connector card, bounded recent-events feed
│   │   │   │   ├── layout/     # Shell: sidebar, top bar
│   │   │   │   ├── metadata/   # Metadata editor with platform tabs, Twitch/YouTube category pickers, publish panel
│   │   │   │   ├── overlays/   # Overlays management page panels: list, editor, URL, settings, accounts, hidden users, blocked terms, activity types, setup, preview (Stage 10)
│   │   │   │   ├── platforms/  # Destination cards, add/settings dialogs, output settings, branch controls, account link, broadcast selection
│   │   │   │   ├── runtime/    # Ingest controls, install dialog, copy widget, bulk-start confirmation
│   │   │   │   ├── settings/   # Connected Accounts panel, Twitch device-flow modal, YouTube accounts panel and OAuth modal
│   │   │   │   ├── system/     # System and backend status panels
│   │   │   │   └── ui/         # Base elements (buttons, inputs, panels, modal)
│   │   │   ├── data/           # DEMO DATA (host metrics only)
│   │   │   ├── hooks/          # Queries, mutations, cache helpers, the engagement, operator-chat and chat-overlay SSE client hooks
│   │   │   ├── i18n/           # Localization: config, resources, tests
│   │   │   ├── lib/            # API client, error mapping, helpers
│   │   │   ├── models/         # UI types, validation, identifier/state-to-label mappings, the operator-chat and chat-overlay reducers, autoscroll state machine, overlay preview fixtures
│   │   │   ├── pages/          # Route views, including EngagementPage, ChatPage, OverlaysPage and the public OverlayChatPage (no application shell)
│   │   │   └── test/           # Rendered-component test harness (Testing Library provider wrapper)
│   │   └── ...                 # Vite, TypeScript, ESLint, Vitest configuration
│   │
│   └── server/                 # Backend (Go)
│       ├── cmd/server/         # Entry point, graceful shutdown
│       ├── cmd/testserver/     # `-tags integration` twin for the real-FFmpeg and fake-provider smoke tests only
│       └── internal/
│           ├── buildinfo/      # Service name and version
│           ├── config/         # Configuration and database path resolution
│           ├── domain/account/ # Connected-account model, token bundle, service (provider-independent)
│           ├── domain/engagement/       # Normalized engagement-event model (Stage 8A)
│           ├── domain/engagementsettings/ # Per-account engagement-connector enable/disable preference
│           ├── domain/operatorchatprefs/ # Persisted operator-chat preferences, account visibility, hidden/bot-user lists (Stage 9)
│           ├── domain/chatoverlay/ # Persisted chat-overlay profiles: settings, accounts, hidden users, blocked terms, activity types (Stage 10)
│           ├── domain/platform/# Provider registry, models, validation, service
│           ├── domain/credential/# Destination stream-key service (OS credential store)
│           ├── domain/output/  # Destination output-settings model, validation, service
│           ├── domain/remotetarget/ # Remote broadcast/target association (YouTube)
│           ├── engagement/     # The Engagement Event Bus (ring buffer, dedup, subscriptions)
│           ├── operatorchat/   # The unified operator-chat projection (Stage 9) - provider-independent, in-memory only
│           ├── chatoverlay/    # The public per-overlay chat projection (Stage 10) - consumes operatorchat's own revision stream, not the Event Bus directly
│           ├── httpapi/        # Router, handlers, middleware, JSON responses
│           ├── provider/twitch/# Twitch OAuth + Helix + EventSub client, adapter, metadata/engagement services
│           │   └── chatassets/ # Twitch chat badge (cached) and emote (pure URL) resolution (Stage 9)
│           ├── provider/youtube/# YouTube OAuth (PKCE) + Data API client, adapter, metadata service
│           ├── runtime/deviceflow/# Device-authorization attempt state machine
│           ├── runtime/youtubeauth/# YouTube Authorization Code + PKCE loopback-callback attempt manager
│           ├── runtime/twitchengagement/ # Per-account Twitch EventSub WebSocket connector supervisor
│           ├── runtime/mediamtx/# Resolver, installer, config, supervisor, API client
│           ├── runtime/ffmpeg/ # Executable resolver and capability probing
│           ├── runtime/branch/ # Per-destination branch supervisor (state machine, restart policy)
│           └── storage/sqlite/ # Connection, migrations, repository
│               └── migrations/ # Embedded .sql schema and seed
│
├── config/                     # No FFmpeg/MediaMTX templates live here - see config/README.md
├── docs/
│   ├── project-overview.md     # Full project description
│   ├── engagement-architecture.md # Engagement platform architecture (operator chat implemented as of stage 9, the OBS chat overlay as of stage 10)
│   ├── obs-browser-source.md   # Researched OBS Browser Source contract and Stage 10 recommendations
│   ├── provider-integrations/
│   │   ├── twitch.md           # Researched Twitch metadata API contract: flow, scopes, capabilities, limits
│   │   ├── twitch-engagement.md # Researched Twitch EventSub WebSocket contract (Stage 8A) + chat badge/emote contract (Stage 9)
│   │   └── youtube.md          # Researched Google/YouTube API contract
│   └── progress.md             # Work journal
├── scripts/
│   ├── verify-persistence.mjs      # Scripted restart-persistence check
│   ├── verify-mediamtx-runtime.mjs # Real MediaMTX install and supervision check
│   ├── verify-ffmpeg-branches.mjs  # Real FFmpeg + MediaMTX destination-branch check
│   ├── verify-twitch-account-integration.mjs # Twitch device flow, linking, publish - fake Twitch only
│   ├── verify-youtube-account-integration.mjs # YouTube PKCE flow, linking, publish - fake Google only
│   ├── verify-twitch-engagement.mjs # Event Bus + EventSub connector - fake Twitch only
│   ├── verify-operator-chat.mjs    # Unified operator chat: projection, preferences, badges/emotes - fake Twitch only
│   └── verify-chat-overlay.mjs     # OBS Browser Source chat overlay: profiles, public projection, public API - fake Twitch only
├── .gitignore
├── THIRD_PARTY_NOTICES.md      # MediaMTX, FFmpeg and other third-party dependencies
└── README.md
```

---

## What is currently demo-only

Every item below is marked with a **Demo** badge in the interface, or described
directly next to the control.

| Element | What actually happens |
| ------- | --------------------- |
| Per-destination viewer counts, connection quality, "Authenticated"/"Verified by platform" status | **Not shown anywhere.** Streaming Tree never contacts a platform to confirm a stream is live there; the interface only ever reports what FFmpeg itself reported (real progress fields) or a plain "Sending" / "Output active" wording. |
| CPU, memory, disk, network | Fixed demo values, clearly badged. The backend does not collect host metrics. |
| Platform capability tables | Twitch's and YouTube's tables are now verified against their real APIs — see [`docs/provider-integrations/twitch.md`](docs/provider-integrations/twitch.md) and [`docs/provider-integrations/youtube.md`](docs/provider-integrations/youtube.md). Kick and TikTok remain an approximate configuration, **not** verified against their real APIs, and need re-checking when their own account integration is implemented (stage 7C). |
| Kick and TikTok account connection and metadata publishing | **Not implemented.** Only Twitch and YouTube have a real provider integration at this stage; the destination-settings account section for these providers shows an honest "not implemented yet" state instead of a working selector. |
| Outbound chat/bot messages, alerts, TTS, YouTube live chat, Super Chat, membership events, Kick/TikTok engagement, a visual overlay designer, overlay templates | **Not implemented anywhere.** A real, unified operator chat is implemented as of stage 9 and a real, public OBS Browser Source chat overlay built on top of it as of stage 10 (see [Unified operator chat](#unified-operator-chat) and [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)) — everything still built on top of *those* (outbound chat, alerts, TTS) remains planned; see [`docs/engagement-architecture.md`](docs/engagement-architecture.md). |
| Platforms, Metadata, Logs pages | Informational views describing the planned scope. Not implemented. |

### What is real

- **Receiving a stream from OBS**, through a supervised MediaMTX process.
- **Installing MediaMTX**, with official checksum verification.
- **Start, Stop and Restart of the local ingest service.**
- **Live ingest detection** — waiting versus receiving, with the source type and
  detected tracks reported by MediaMTX.
- **The Server and Stream Key values**, derived from the running configuration.
- **Adding, editing and deleting destinations**, stored in SQLite.
- **Editing and saving stream metadata**, including ordered Twitch tags.
- **Storing, replacing, checking the status of and deleting a destination's
  stream key**, in the operating system credential store - see
  [Stream key security](#stream-key-security).
- **Sending a destination's stream onward with real FFmpeg** - one
  independent process per enabled destination, pulling the local ingest and
  pushing stream-copied RTMP/RTMPS to that destination's configured server,
  with real Start/Stop/Restart, real per-branch state and real progress -
  see [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg).
- **Everything stored survives a browser refresh and a backend restart** -
  including a stream key, which survives in the OS credential store
  independently of both. Per-branch *runtime* state (live/error/restart
  count) deliberately does **not** survive a backend restart - see the
  branch lifecycle section above for why.
- **Connecting a real Twitch account** via device-code sign-in, with
  no client secret ever requested or stored, account validation/refresh/
  reconnect/disconnect, and linking an account to a destination - see
  [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata).
- **Searching real Twitch categories and publishing real channel metadata**
  (title, category, language, tags) to Twitch, behind an explicit publish
  action separate from the existing local Save.
- **Connecting a real YouTube channel** via Authorization Code + PKCE
  sign-in through a real system browser and a temporary loopback callback,
  with no client secret ever requested or stored, explicit multi-channel
  selection, account validation/refresh/reconnect/disconnect, linking a
  channel to a destination, and selecting a live broadcast for it - see
  [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata).
- **Listing real YouTube broadcasts and categories and publishing real
  video metadata** (title, description, category, tags, language,
  visibility) to a selected YouTube broadcast, behind an explicit publish
  action separate from the existing local Save.
- **The language switcher**, in the top bar and under Settings.
- **A real Twitch EventSub WebSocket connector** reading chat messages,
  moderation, follows, subscriptions, gifts, cheers, incoming raids,
  channel-point redemptions and remote stream online/offline, normalized
  onto an in-memory Engagement Event Bus, with real enable/disable
  (persisted, restored automatically after a backend restart), a real
  identity-bound permission-upgrade flow, and Twitch's own official
  `session_reconnect` handoff handled without a false data-gap marker -
  see [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents).
- **A real Server-Sent Events stream** (`GET /api/engagement/stream`) with
  replay via `Last-Event-ID` and an explicit gap signal for evicted
  history, and the diagnostic Engagement page that consumes it live.
- **A real, unified operator Chat page** (`/chat`) merging live Twitch
  chat across every connected account with an enabled connector: real
  ordered fragments, resolved Twitch badges and emote images, message
  deletion and chat/user clearing reflected in place, activity events
  inline, account/kind/bot/hidden-user filtering, persisted display
  preferences, and autoscroll with a jump-to-latest control - see
  [Unified operator chat](#unified-operator-chat).
- **A real, public OBS Browser Source chat overlay** (`/overlays` to
  manage, `/overlay/chat/{publicSlug}` to view/embed) — persisted overlay
  profiles with their own filters, visual settings and an unguessable,
  rotatable public URL; a public per-overlay projection consuming the
  operator-chat projection above; a public unauthenticated HTTP + SSE
  API; and a live preview panel on the management page reusing the exact
  same renderer - see
  [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay).

No bitrate, resolution or frame rate is displayed anywhere: the MediaMTX Control
API does not report them, so showing a number would mean inventing it.

### What will be added later

- **Kick and TikTok account integration** — sign-in and metadata
  publishing for the remaining providers, reusing the same connected-account
  foundation Twitch's and YouTube's integrations now provide - deferred,
  capability-gated (stage 7C; Kick may land together with its own
  engagement adapter in stage 15).
- **Outbound chat, scheduled bot messages, the alert engine, TTS, goal
  widgets, a visual overlay designer and overlay templates** and the rest
  of the engagement and overlay platform - architecture only so far, see
  [`docs/engagement-architecture.md`](docs/engagement-architecture.md).
- **A log viewer** — the backend keeps a small diagnostic buffer already.

---

## Stream key security

A stream key allows broadcasting on someone's channel, so we treat it like a
password. This section describes the destination-credential foundation; how
that key is actually used to start an outgoing stream is described in
[Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg), including
its one honestly-documented limitation (command-line exposure).

- **The repository contains no secrets** and must never contain any.
  `.gitignore` blocks `.env` files, database files and data directories - and
  is anchored at the repository root everywhere it lists a directory name, so
  a guard rule can never accidentally match a source directory.
- **The SQLite database stores no credentials, and never will.** No table has
  a column for a stream key, token or password, and no API payload for
  platform or metadata endpoints carries one. Whether a credential is
  configured is derived directly from the OS credential store on every
  request, not cached in a database row that could go stale.
- **A stream key is stored in the operating system credential store**:
  Windows Credential Manager, macOS Keychain, or Linux Secret Service, via
  [`github.com/99designs/keyring`](https://github.com/99designs/keyring) -
  chosen specifically because none of its backends for these three platforms
  shell out to an external command (see `THIRD_PARTY_NOTICES.md`). **There is
  no plaintext fallback.** If the credential store cannot be reached, the API
  reports that plainly and leaves the value unstored; it never writes it to a
  file instead.
- **The key is scoped by the destination's generated ID, not its display
  name or provider.** Renaming a destination cannot orphan its key, and two
  destinations configured for the same provider always get independent keys.
- **Once saved, a key cannot be viewed again through this application.**
  There is no "show saved key" control anywhere. Replacing overwrites the
  previous value; deleting removes it and prevents outgoing streaming to that
  destination until a new key is added. Both actions are described in the
  platform settings dialog itself.
- **A "Stored" status is not a claim that the key is valid.** Streaming Tree
  never contacts a platform to verify a key, so the interface only ever says
  "Stored" or "Missing" (or that secure storage is unavailable) - never
  "Valid", "Connected" or "Authenticated".
- **Keys are not stored in the browser.** Not in `localStorage`, not in
  `sessionStorage`, not in application state beyond what a submit requires,
  and not in TanStack Query's cache - only a `{configured, available}` status
  is ever cached. The mutation that submits a new key is configured with a
  zero garbage-collection time and resets itself immediately after every
  attempt, so it does not linger in React Query Devtools either. The only
  value this application stores in the browser at all is the interface
  language preference.
- **The backend reads a key only when explicitly starting that destination's
  outgoing FFmpeg process**, never while merely checking status, and never
  logs it or includes it in a formatted error. The retrieval method for that
  is not reachable through the HTTP API at all: the interface the web panel
  talks to simply has no method that returns a value. It **is** passed to
  FFmpeg as part of a command-line argument, since no safer mechanism exists
  in FFmpeg's own CLI for this - see
  [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg) for the
  honest explanation of that one limitation and its mitigations.
- **This is unrelated to the local ingest path.** The `live` stream key OBS
  uses to publish to this machine (see [Connecting OBS](#connecting-obs)) is
  a route name on a loopback-only server, not a secret, and is never confused
  with a destination's credential anywhere in the interface.

The full architecture - key-namespace format, validation rules, the HTTP
contract, and the platform-deletion cleanup ordering - is in
[`docs/project-overview.md`](docs/project-overview.md#10-stream-key-security)
and the implementation notes in
[`docs/progress.md`](docs/progress.md).

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

**The platform settings dialog says "Secure storage unavailable" for the
stream key.** The operating system credential store could not be reached:
common causes are a Linux session with no Secret Service running, a locked
macOS Keychain, or a permission failure. The rest of the application is
unaffected - SQLite, MediaMTX and everything else keep working - but a stream
key cannot be saved until the store becomes available. This is not polled
automatically; reopen the dialog after fixing the underlying cause to check
again.

**I deleted a destination while the credential store was unavailable - is its
stream key still out there?** Possibly. Platform deletion does not block on a
credential store it cannot reach (see "Stream key security"), so a key set
earlier, when the store was reachable, may still exist under that platform's
old ID. It is inert: the ID is never reused and nothing in this application
can look it up again. If this matters to you, use your OS credential manager
directly to remove any leftover entry under the `streaming-tree-for-obs`
service name.

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

### MediaMTX and OBS

**"MediaMTX is not installed yet."**
Expected on a fresh setup. Use the **Install MediaMTX** button in the sidebar or
on the **Streams** page. Nothing is downloaded until you confirm.

**"The MediaMTX binary found is not the supported version."**
Only v1.19.3 is supported, and an unsupported build is never started because the
generated configuration targets that exact schema. Either remove
`STREAMING_TREE_MEDIAMTX_PATH` and use the managed installation, or point it at
a v1.19.3 binary. If a managed installation is stale, delete
`runtime/mediamtx` and reinstall.

**"The downloaded file did not match the official checksum."**
Nothing was installed — the archive was discarded. Retry; if it keeps happening,
suspect a proxy or security product rewriting downloads. Never work around this
by installing manually from an unverified source.

**"There is no official MediaMTX release for this operating system..."**
Your OS/architecture is outside the supported matrix. Obtain a v1.19.3 binary
yourself and set `STREAMING_TREE_MEDIAMTX_PATH` to it.

**"The configured port is already used by another application."**
Something else holds 1935 or 9997. Streaming Tree **never terminates another
process to free a port**. Stop the other application, or set
`STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` / `STREAMING_TREE_MEDIAMTX_API_ADDRESS`
to free ports. Remember to update OBS if you change the RTMP port.

Finding the holder:

```bash
# Linux / macOS
lsof -i :1935
```

```powershell
# Windows PowerShell
Get-NetTCPConnection -LocalPort 1935 | Select-Object OwningProcess
```

**"MediaMTX failed repeatedly and will not be restarted automatically."**
The crash-loop guard tripped: five failures within five minutes. Automatic
restarts stop deliberately so the loop does not run forever. Look at the backend
log for the MediaMTX output, fix the cause, then press **Start**.

**"MediaMTX started but did not become ready in time."**
The process launched but its Control API never answered. Usually the Control API
port is blocked or occupied by something that accepts connections without
answering correctly. Check `STREAMING_TREE_MEDIAMTX_API_ADDRESS`.

**OBS is connected but the panel still says "Waiting for OBS".**
Check that OBS uses **Custom...** with exactly the Server and Stream Key shown
in the panel — a mismatched stream key publishes to a path this configuration
does not allow. Also confirm OBS is actually streaming, not just configured, and
that the service reports **Running**.

**Ingest says "Status unavailable".**
MediaMTX is running but the backend cannot read its Control API. Restarting the
service usually clears it.

**MediaMTX keeps running after I close the backend.**
It should not: shutdown stops and reaps it. If it happens, note how the backend
was terminated — a `SIGKILL` to the backend gives it no chance to clean up — and
end the `mediamtx` process manually.

### FFmpeg and destination branches

**A destination shows the blocker "FFmpeg is not available."**
No compatible FFmpeg was found. Install one from a source you trust and make
sure it is on `PATH`, or set `STREAMING_TREE_FFMPEG_PATH` to it, then restart
the backend (FFmpeg is only re-probed periodically or at startup). Streaming
Tree never installs FFmpeg for you — see
[Why there is no managed FFmpeg download](#outgoing-streaming-with-ffmpeg).

**A destination shows the blocker "The available FFmpeg is missing a
required capability."**
The located FFmpeg failed at least one capability probe (RTMP input/output,
RTMPS output, the FLV muxer, or `-progress` support) even though it parses
`-version` fine. Most general-purpose FFmpeg builds pass all of these;
check whether yours was built with RTMP support disabled.

**A destination fails immediately with an "unsupported codec" error.**
FLV/RTMP cannot carry every codec, and this stage never transcodes. Change
the source (in OBS) to a codec FLV can carry — H.264 video, AAC audio are
the safe, universally supported choice — rather than expecting Streaming
Tree to silently re-encode.

**A destination keeps restarting and then shows "FFmpeg failed repeatedly
and will not be restarted automatically."**
The same crash-loop guard as MediaMTX's, applied per destination: five
failures within five minutes. Check the destination's `lastError` on the
Streams page, fix the underlying cause (commonly: the destination server is
unreachable, or the port/URL is wrong), then press **Start** again — the
restart counter resets on a fresh explicit start.

**A destination is stuck on "Waiting for input."**
This is expected whenever OBS is not currently publishing to the local
ingest — the branch is deliberately paused, not failing. It resumes on its
own once OBS reconnects, as long as you have not pressed **Stop** since.

**I configured an output server URL but saving it fails validation.**
Only `rtmp://` and `rtmps://` are accepted, a host is required, the port (if
present) must be valid, and the URL may not contain user-info (`user@host`),
a `#fragment`, or control characters. A path (like `/app`) is fine — many
providers use one. The stream key never belongs in this field at all; it has
its own field.

**Is my stream key visible anywhere I should worry about?**
See [Stream-key exposure on the command line](#outgoing-streaming-with-ffmpeg)
for the one honestly-documented limitation: it is briefly present as an
FFmpeg process argument while that destination is running, which on most
operating systems a process list on the *same machine* could observe. It is
never logged, never in an API response, and never on disk outside the OS
credential store.

### Twitch account integration

**"Configure a Twitch Client ID above before connecting an account."**
No Client ID is configured yet, or it failed validation. Register an
application at the
[Twitch Developer Console](https://dev.twitch.tv/console/apps) and either
set `STREAMING_TREE_TWITCH_CLIENT_ID` or paste it into the Settings page —
see [Registering a Twitch application](#connected-accounts-and-twitch-metadata).

**The Client ID field in Settings is disabled and I can't change it.**
It is set by the `STREAMING_TREE_TWITCH_CLIENT_ID` environment variable,
which always wins over anything saved in the database. Unset it (and
restart the backend) if you want to manage the Client ID from Settings
instead.

**Saving a new Client ID fails with a conflict.**
A database-managed Client ID cannot be changed while any Twitch account is
still connected, since a different application can mean invalidated
tokens for existing accounts. Disconnect every Twitch account first.

**"Authorization was denied on Twitch."**
You (or whoever completed the device-code flow) chose not to authorize the
application on Twitch's own page. Click **Connect Twitch** again to start a
fresh attempt.

**"This code expired before it was used."**
The user code has a limited lifetime. Start a new attempt and complete it
more quickly, or check that the device you used to open the verification
link actually reached Twitch (network issues on that device look the same
as simply not finishing in time).

**"The authorization did not grant every required permission."**
Twitch's own authorization page let you decline part of what was requested.
Reconnect and make sure the full permission is granted; Streaming Tree only
ever asks for one scope (`channel:manage:broadcast`), so there is nothing
to selectively decline without breaking metadata publishing.

**An account shows "Reconnect required."**
Twitch could not confirm the account's access on the last check (the token
could not be validated and the automatic refresh also failed — commonly
because the refresh token expired from 30 days of disuse, or the account's
authorization was revoked directly on Twitch). Click **Reconnect** to
re-authorize the same account; nothing about the account's identity or any
destination links needs to be re-entered.

**"Secure storage is currently unavailable" on a Twitch action.**
The same operating-system credential store used for stream keys also holds
Twitch token bundles, and it could not be reached — see "Secure storage
unavailable" above for common causes. Connected accounts and their links
are unaffected in SQLite; only the token bundle-dependent actions
(validate, category search, publish) are blocked until the store is
reachable again.

**"Twitch could not be reached" / a publish or category search fails
intermittently.** A transient network issue talking to Twitch, or Twitch
itself being unavailable. Nothing local was changed; try again.

**"Twitch's rate limit was reached; try again shortly."**
Twitch's own API rate limit (visible in its `Ratelimit-*` response headers)
was hit. This is Twitch-side, not a Streaming Tree limit; wait a short
while and retry.

**The Publish button is disabled and says to select a category first.**
The saved category text has no matching Twitch category ID — either it was
typed by hand without picking a search result, or an older save predates
the category picker. Open the metadata editor, use the category search box,
and select a real result; that stores both the display name and the ID
publishing actually needs.

**Publishing is disabled with a note about unsaved changes.**
Save your local edits first. Publish always sends exactly what is currently
saved in Streaming Tree's database, never an in-progress, unsaved draft —
this is deliberate, not a bug.

### YouTube account integration

**"Configure a YouTube Client ID above before connecting a channel."**
No Client ID is configured yet. Create a Google Cloud project, enable
YouTube Data API v3, create a Desktop-app OAuth client, and either set
`STREAMING_TREE_YOUTUBE_CLIENT_ID` or paste the Client ID into the
Settings page — see
[Registering a Google Cloud project](#connected-accounts-and-youtube-metadata).

**The Client ID field in Settings is disabled and I can't change it.**
It is set by the `STREAMING_TREE_YOUTUBE_CLIENT_ID` environment variable,
which always wins over anything saved in the database — independent of
Twitch's own Client ID variable.

**Saving a new Client ID fails with a conflict.**
A database-managed YouTube Client ID cannot be changed while any YouTube
account is still connected. Disconnect every YouTube account first.

**"Authorization was denied on Google."**
You chose not to approve access on Google's own consent page. Click
**Connect YouTube** again to start a fresh attempt.

**"This attempt expired before it was completed."**
The authorization attempt has a bounded lifetime. Start a new attempt and
complete the Google sign-in more promptly.

**A channel-selection screen appears after signing in.**
The Google account you authorized owns more than one YouTube channel.
Streaming Tree never guesses which one you meant — pick the correct
channel explicitly from the list shown.

**"The authorized channel does not match the account being reconnected."**
During a reconnect, a different YouTube channel was authorized than the
one this connected account represents. Reconnect must authorize the exact
same channel; if you meant to connect a different channel, disconnect this
one first and connect the other as a new account.

**An account shows "Reconnect required."**
Google could not confirm the account's access on the last check. This is
often expected if your Google Cloud project's OAuth consent screen is
still in **Testing** publishing status — Google expires authorization
after seven days in that state regardless of what Streaming Tree
requests. Click **Reconnect** to re-authorize the same channel.

**"Secure storage is currently unavailable" on a YouTube action.**
The same operating-system credential store used for stream keys and
Twitch tokens also holds YouTube token bundles, and it could not be
reached — see "Secure storage unavailable" above. Connected accounts,
links, and selected broadcasts are unaffected in SQLite; only token-
dependent actions (validate, broadcast/category listing, publish) are
blocked until the store is reachable again.

**"YouTube could not be reached" / a publish or listing fails
intermittently.** A transient network issue talking to Google/YouTube, or
the API itself being unavailable. Nothing local was changed; try again.

**"YouTube's API quota was exceeded; try again later."**
Your Google Cloud project's daily YouTube Data API quota (10,000 units by
default) was exhausted. This is Google-side, not a Streaming Tree limit;
it resets daily.

**"Live streaming is not enabled for this channel."**
The connected YouTube channel has not enabled live streaming in YouTube
Studio. Enable it there, then retry.

**The broadcast selector is empty.**
No active or upcoming broadcast was found for the linked channel. Create
and schedule one in YouTube Studio — Streaming Tree does not create a
broadcast for you.

**The Publish button is disabled and says to select a broadcast or
category first.** Select a live broadcast in the destination's own
**Selected broadcast** section, and/or open the metadata editor's category
field and pick a real region-scoped result — both are required before
publishing, and neither is guessed automatically.

**Publishing is disabled with a note about unsaved changes.**
Save your local edits first, exactly like Twitch — Publish always sends
what is currently saved, never an in-progress draft.

### Twitch engagement

**"Additional Twitch permission is required" on the Engagement page.**
The connected Twitch account has only the metadata scope
(`channel:manage:broadcast`); reading chat and events needs five more,
narrowly-scoped permissions. Click **Authorize engagement access** to
start the upgrade — your existing stream key and metadata publishing are
completely unaffected while you do.

**The upgrade shows a new code/consent step even though the account is
already connected.** That is expected: the upgrade reuses the same
Device Code Flow as the initial connection, requesting the union of the
account's current scopes plus the engagement ones. Complete it the same
way you completed the original connection.

**"The authorized identity does not match" during the upgrade.** A
different Twitch login completed the device-code activation than the one
already connected. The upgrade must authorize the *same* account;
disconnect and reconnect as a new account instead if you actually meant
to switch identities.

**The Enable toggle is disabled or shows "Blocked."** Either the
permission upgrade above has not been completed yet, or the account
itself needs reconnecting for an unrelated reason (see "An account shows
'Reconnect required'" under Twitch account integration above) — the
connector's own state and blocker code explain which.

**A connector shows "Reconnecting" repeatedly, or a "possible data gap"
timestamp appeared.** Twitch does not replay events lost during an
ordinary connection loss; the connector reconnects automatically with
bounded backoff and recreates its subscriptions, and is honest about the
gap rather than pretending nothing was missed. This is expected
behavior, not an error — check the connector's own reconnect count and
last-event timestamp to see whether it has recovered.

**A connector shows "Error" and does not reconnect on its own.** Most
commonly, Twitch revoked the authorization directly (on Twitch's own
site) or removed the subscription version this application uses. Use
**Restart connector**, and if that also fails, disconnect and reconnect
the underlying Twitch account.

**The recent-events feed says "Disconnected" or never shows anything.**
The Server-Sent Events connection to the backend dropped, or the
connector itself is not `connected` yet — check the connector card's own
state first; the feed only ever shows what the backend's Event Bus
actually received.

**Does disabling engagement affect my stream key or metadata publishing?**
No. A connected account's engagement connector, its metadata-publishing
capability, and a destination's stream key are three separate facts —
see [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents).
Enabling or disabling the connector never starts, stops, or otherwise
touches a destination's FFmpeg branch.
