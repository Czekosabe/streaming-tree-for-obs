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
chat commands, visual overlay designers, text-to-speech and goal widgets. None
of that exists yet — it is architecture and planning, detailed in
[`docs/engagement-architecture.md`](docs/engagement-architecture.md) — but it
shapes decisions made today, starting with the credential-store foundation
this stage adds.

> ## Project state: local ingest, secure key storage and outgoing FFmpeg streaming all work
>
> Streaming Tree can **receive** a stream from OBS (a supervised, managed
> MediaMTX process), **store a destination's stream key securely** in the
> operating system credential store, and now **send it onward**: one
> independent FFmpeg process per enabled destination, pulling the local
> ingest and pushing to that destination's configured RTMP/RTMPS server with
> plain stream copy (no re-encoding). See
> [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg) and
> [Stream key security](#stream-key-security).
>
> Starting a real broadcast is always an **explicit action** — a destination
> never starts on its own, and a backend restart never resumes one
> automatically.
>
> OAuth-based sign-in, platform metadata publishing, and the wider engagement
> platform (unified chat, overlays, alerts, bot automation) are still
> **planned**. Whatever remains a placeholder is marked with a **Demo**
> badge — the full list is in
> [What is currently demo-only](#what-is-currently-demo-only).

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
| 6 | FFmpeg destination branches (this stage) | **Completed** — see [progress.md](docs/progress.md) |
| 7 | Connected accounts, OAuth, metadata publishing | Planned |
| 8–19 | Engagement Event Bus, unified chat, overlays, alerts, bot automation, visual designers, templates, TTS, goal widgets, additional platform connectors | Planned |
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
`415` wrong content type, `422` validation failure, `500` internal failure.

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

**Integration checks** (from the repository root):

```bash
node scripts/verify-persistence.mjs        # SQLite survives a backend restart
node scripts/verify-mediamtx-runtime.mjs   # real MediaMTX install and supervision
node scripts/verify-ffmpeg-branches.mjs    # real FFmpeg + MediaMTX destination branches
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

**None of these scripts touch your real database, your managed MediaMTX
installation, or your real OS credential store**, and all remove their
temporary directories afterwards.

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
    │   └── runtime.json      # MediaMTX/ingest state, dependency status
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
│   │   │   │   ├── platforms/  # Destination cards, add/settings dialogs, output settings, branch controls
│   │   │   │   ├── runtime/    # Ingest controls, install dialog, copy widget, bulk-start confirmation
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
│       ├── cmd/testserver/     # `-tags integration` twin for the real-FFmpeg smoke test only
│       └── internal/
│           ├── buildinfo/      # Service name and version
│           ├── config/         # Configuration and database path resolution
│           ├── domain/platform/# Provider registry, models, validation, service
│           ├── domain/credential/# Destination stream-key service (OS credential store)
│           ├── domain/output/  # Destination output-settings model, validation, service
│           ├── httpapi/        # Router, handlers, middleware, JSON responses
│           ├── runtime/mediamtx/# Resolver, installer, config, supervisor, API client
│           ├── runtime/ffmpeg/ # Executable resolver and capability probing
│           ├── runtime/branch/ # Per-destination branch supervisor (state machine, restart policy)
│           └── storage/sqlite/ # Connection, migrations, repository
│               └── migrations/ # Embedded .sql schema and seed
│
├── config/                     # No FFmpeg/MediaMTX templates live here - see config/README.md
├── docs/
│   ├── project-overview.md     # Full project description
│   ├── engagement-architecture.md # Later-stage engagement platform architecture
│   └── progress.md             # Work journal
├── scripts/
│   ├── verify-persistence.mjs      # Scripted restart-persistence check
│   ├── verify-mediamtx-runtime.mjs # Real MediaMTX install and supervision check
│   └── verify-ffmpeg-branches.mjs  # Real FFmpeg + MediaMTX destination-branch check
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
| Platform capability tables | An approximate configuration, served by the backend. It has **not** been verified against the real Twitch, YouTube, Kick or TikTok APIs and needs re-checking when real integrations are implemented. |
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
- **The language switcher**, in the top bar and under Settings.

No bitrate, resolution or frame rate is displayed anywhere: the MediaMTX Control
API does not report them, so showing a number would mean inventing it.

### What will be added later

- **SSE or WebSocket** — live status instead of polling.
- **OAuth and platform APIs** — sign-in and metadata publishing, reusing the
  same credential-store abstraction with a different secret type.
- **The engagement and overlay platform** — unified chat, OBS overlays,
  alerts, bot automation and more; architecture only so far, see
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
