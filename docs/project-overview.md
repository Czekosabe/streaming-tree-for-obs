# Streaming Tree for OBS — project overview

> This document describes the goals, architecture and roadmap of the project.
> Sections marked as **planned** are not implemented yet.
> The current state of the work is recorded in [progress.md](progress.md).

---

## 1. Project name

**Streaming Tree for OBS**

The name describes how the application works: a single stream leaving OBS is the
"trunk", and every destination platform is an independent "branch".

---

## 2. The problem we are solving

A live streamer who wants to broadcast to several platforms at once runs into
the following difficulties today:

1. **Hardware cost.** OBS can send multiple outputs, but each one costs either a
   separate encode or a separate upload of the same stream. With four platforms
   the CPU or upstream bandwidth load grows several times over.
2. **Dependence on third-party services.** Commercial multistreaming services
   require sending the stream to someone else's server and handing over the
   stream keys, usually for a subscription fee.
3. **Scattered metadata.** Title, category, tags and visibility settings have to
   be entered separately in each platform's dashboard, in different formats and
   under different limits.
4. **No failure isolation.** If one platform refuses the connection or drops the
   session, typical setups can disturb the remaining outputs.

## 3. Core idea

OBS sends **one** local stream to the application. The application receives it
and branches it out to any number of platforms, with these properties:

- branching happens **without re-encoding the video** wherever possible (stream
  copy),
- every branch is an **independent process** - one failing does not interrupt
  the others,
- **stream keys never leave the user's machine** and are never committed to the
  repository or exposed to the browser,
- each platform's metadata is described by a **capability model** rather than a
  single, artificially unified form.

## 4. Target audience

- Live streamers broadcasting to several platforms in parallel.
- Technically minded people who prefer running a tool locally over entrusting
  their stream keys to an external service.
- Small production teams and event stream operators who need a clear control
  panel showing the state of every output.
- Later: users who move the stream router to their own server (VPS) and control
  it from a browser.

---

## 5. Scope of the first local version

Version 1.0 (local) is intended to cover:

- receiving a single RTMP stream from OBS on the user's machine,
- configuring the list of destination platforms,
- starting and stopping each stream branch independently,
- showing the state of every branch (offline / starting / live / error),
- editing stream metadata according to each platform's capabilities,
- storing stream keys securely in the system credential store,
- viewing logs and basic diagnostics,
- an operator panel in the browser, served locally.

## 6. Out of scope for the first version

Deliberately **not** part of version 1.0:

- recording and archiving streams,
- transcoding to multiple resolutions (ABR),
- an aggregated chat from several platforms in one window,
- historical statistics and audience analytics,
- accounts, roles and permissions,
- automatic clips, notifications, bot integrations,
- a mobile application,
- a plugin running inside OBS.

Several of these — an aggregated chat, alerts, and bot messages in
particular — **are now part of the long-term product vision** (see §16) and
are planned for stages well after version 1.0. Being out of scope for the
first local version is not the same as being abandoned; it means the
streaming router has to exist and work before anything reads from it.

---

## 7. Overall architecture

```
                ┌───────────────────────────────────────────────┐
                │  Operator panel (React + TypeScript)          │
                │  browser, http://localhost:5173               │
                └───────────────────────┬───────────────────────┘
                                        │ REST  (+ SSE/WebSocket later)
                                        ▼
                ┌───────────────────────────────────────────────┐
                │  Backend (Go)                                 │
                │  API, branch state, metadata, process control │
                └──────┬─────────────────────────┬──────────────┘
                       │ supervises              │ supervises (planned)
                       ▼                         ▼
        ┌──────────────────────┐    ┌──────────────────────────────────┐
  OBS ─▶│  MediaMTX  [DONE]    │─ ─▶│  FFmpeg (one process per branch) │
  RTMP  │  local RTMP ingest   │    │  ffmpeg #1 ─▶ Twitch    [PLANNED]│
        │  127.0.0.1:1935/live │    │  ffmpeg #2 ─▶ YouTube            │
        └──────────────────────┘    │  ffmpeg #3 ─▶ Kick               │
                 ▲                  │  ffmpeg #4 ─▶ TikTok             │
                 │ Control API      └──────────────────────────────────┘
                 │ 127.0.0.1:9997
                 │ (backend only, never the browser)
            readiness + ingest status
```

Solid arrows are implemented; the dashed arrow to FFmpeg is the next stage. The
backend supervises MediaMTX and reads its Control API on loopback; the browser
never contacts MediaMTX directly.

The layers are separated on purpose: the panel never talks directly to MediaMTX
or FFmpeg. All control flows through the Go backend, which is what will later
allow the backend to be moved to a remote server without changing the panel.

This diagram covers the **streaming path** only. The planned engagement and
overlay platform (§16) is a separate, additive set of connectors and consumers
sitting beside it in the backend, communicating with platforms independently
of the RTMP/FFmpeg path; see
[docs/engagement-architecture.md](engagement-architecture.md) for its own
diagram.

### 7.1 The role of OBS

OBS remains the production tool: scenes, sources, audio mixing, video encoding.
It is configured with **one** output - a Custom / RTMP target pointing at the
application's local address (eventually `rtmp://127.0.0.1:1935/live`).

OBS does not know how many platforms the stream will reach. From its point of
view there is a single recipient.

### 7.2 The role of the React frontend

The frontend is an **operator panel**, not part of the streaming path. It is
responsible for:

- presenting the state of all stream branches,
- starting and stopping branches (through the backend API),
- editing metadata according to platform capabilities,
- presenting diagnostics and logs.

The frontend **never** stores stream keys or tokens - not in `localStorage`, not
in `sessionStorage`, not in application state.

### 7.3 The role of the Go backend

The backend is the only place where decisions are made:

- it exposes the REST API for the panel,
- it holds platform configuration and metadata in a local SQLite database,
- it is the single source of truth for provider capabilities,
- it starts and supervises MediaMTX and the FFmpeg processes,
- it reads stream keys from the system credential store at branch start time,
- it enforces failure isolation and the restart policy,
- in later stages it pushes live state over SSE or WebSocket.

Go was chosen for three reasons: distribution as a single binary with no
runtime to install, good support for supervising child processes, and a simple
concurrency model for many independent branches.

### 7.3.1 Persistent storage (implemented)

**Status: implemented.**

Configuration lives in a local SQLite database, accessed through
`database/sql` with the pure-Go `modernc.org/sqlite` driver, so the backend
still builds and cross-compiles as a single binary without a C toolchain. No ORM
is used.

What the database holds:

- configured destinations (which provider, display name, enabled state, ordering),
- their stream metadata,
- ordered metadata tags,
- the record of which schema migrations have been applied.

What it deliberately does **not** hold:

- **runtime state** — no offline/starting/live status, viewer count, connection
  quality or process state. Runtime state belongs to a streaming engine that
  does not exist yet, and persisting it would make configuration and reality
  indistinguishable,
- **credentials** — no stream key, OAuth token or password. No table has a
  column for one.

Migrations are `.sql` files embedded in the binary and applied automatically at
startup, each inside a transaction together with its bookkeeping row, so a
failed migration is never recorded as applied. A one-time seed creates four
disabled example destinations on a brand-new database; because the seed is an
ordinary recorded migration, deleting a seeded destination is permanent.

The database location, environment variables and reset procedure are documented
in `README.md`.

### 7.4 The role of MediaMTX (implemented)

**Status: implemented.**

MediaMTX is the local server receiving the stream from OBS. Rather than writing
an RTMP implementation, the application runs MediaMTX v1.19.3 as a supervised
child process with a generated configuration.

This is what lets OBS encode the video **once** while every branch will later
share the same source.

#### Managed dependency

MediaMTX is third-party software, not part of this repository and never
committed to it. The application resolves it in a deliberately narrow order:
an explicit `STREAMING_TREE_MEDIAMTX_PATH`, then its own managed installation,
then missing. The system `PATH` is not searched, because the binary is launched
as a long-lived child process and should only ever be a copy the application can
identify.

Installation is an explicit user action. The installer verifies the archive
against the checksum file published with the same official release, refuses
unsafe archive entries, requires the upstream `LICENSE` to be present, confirms
the extracted binary reports the pinned version, and only then moves it into
place atomically. Nothing unverified is ever executed.

The version is pinned to exactly **v1.19.3**. The generated configuration and
the Control API client target that schema, and MediaMTX refuses to start when it
meets an unknown configuration key, so a binary reporting any other version is
reported as incompatible and is not started.

#### Process supervisor

The supervisor owns an explicit state machine — `missing`, `installing`,
`incompatible`, `stopped`, `starting`, `ready`, `stopping`, `error` — rather
than a set of booleans, so contradictory combinations are unrepresentable.

Readiness means the MediaMTX Control API answered correctly, never merely that a
process was spawned: a misconfigured MediaMTX exits milliseconds after starting.
Both output streams are drained concurrently, because a child whose pipe fills
blocks permanently. An unexpected exit triggers bounded exponential backoff with
a cap per time window, so a crash loop stops rather than spinning; an explicit
Stop suppresses restart entirely. Backend shutdown drains HTTP first, then stops
and reaps the child.

A missing or failed MediaMTX never stops the Go API: platform configuration
stays fully readable and writable, and the component reports its own state.

#### Security boundary

Both the RTMP listener and the Control API bind to loopback only, and a
non-loopback address is rejected at startup rather than warned about — MediaMTX
here accepts an unauthenticated publisher and its Control API can rewrite its
own configuration. The browser never contacts the Control API; only the Go
backend does, and there is no proxy route. No runtime path, process environment
or process id is exposed to the interface.

### 7.5 Planned role of FFmpeg

**Status: not implemented.**

For each active platform the backend will start a separate FFmpeg process that
reads the stream from MediaMTX and pushes it to that platform's RTMP endpoint.

Design assumptions:

- stream copy by default (`-c copy`) - no re-encoding,
- re-encoding only where a platform requires different parameters,
- one process per branch means a separate lifecycle, separate logs and a
  separate restart policy.

---

## 8. The independent branch model

Every platform is an independent branch with its own lifecycle:

```
offline ──▶ starting ──▶ live
   ▲            │           │
   │            ▼           ▼
   └────────── error ◀──────┘
```

Rules:

1. **Process isolation.** One branch means one FFmpeg process. A process failure
   does not touch the others.
2. **Error isolation.** One platform rejecting a stream key moves only that
   branch into the `error` state.
3. **Independent control.** Branches can be started and stopped individually
   without interrupting the stream on the remaining platforms.
4. **Independent restart.** The retry policy is configured per branch.
5. **Shared source.** All branches read the same stream from MediaMTX, so adding
   a platform puts no extra load on OBS.

## 8.1 Three concepts that must not be confused

The domain deliberately separates three things that look similar and behave
completely differently.

### Provider definition

A built-in description of an integration type: Twitch, YouTube, Kick, TikTok.
It carries the brand name, the metadata capabilities, the field limits and the
supported option identifiers.

Provider definitions are **compiled into the backend binary**. They are not
database rows, cannot be created or deleted by a user, and are not affected by
platform CRUD. The backend is their single source of truth; the frontend holds
no competing capability table.

### Configured platform

A destination branch the **user** created — for example "provider: Twitch,
display name: Main Twitch channel".

Several destinations may use the same provider, so the provider identifier is
never the primary key. Every configured platform has a stable, random,
backend-generated identifier. A configured platform stores **configuration
only**.

### Runtime stream state

Whether the ingest service is running, whether a publisher is connected, process
restart counts, the last runtime error — and later, per-branch FFmpeg state.

**Runtime state exists now, and lives only in memory.** It is never written to
the SQLite tables and resets when the backend restarts, because it describes
what is happening right now rather than what the user configured. No migration
in this stage added a runtime column, and none should.

What is tracked today: whether MediaMTX is installed, whether its version is
compatible, its process state, readiness, restart count, the last error, whether
a publisher is connected, when the input became available, the source type and
the track identifiers MediaMTX reports.

What is deliberately **not** tracked: bitrate, resolution, frame rate, dropped
frames and viewer counts. The MediaMTX Control API does not report them, so any
number shown would be invented. Per-destination runtime state does not exist
either, because no outgoing streaming engine exists yet — configured
destinations are still presented as configuration only.

### 8.2 OBS ingest detection

While MediaMTX is ready, the backend polls its Control API `/v3/paths/list` and
maps the configured path onto an ingest state: `unavailable` when the service is
not ready, `waiting` when it is ready with no publisher, `receiving` when a
publisher is connected, and `error` when the service runs but its status cannot
be read — which is reported honestly rather than being reported as "waiting".

RTMP does not identify the publishing application. The interface therefore says
"OBS or another RTMP publisher" and never asserts that a generic publisher is
OBS. The source type MediaMTX reports (for example `rtmpConn`) is shown as-is,
because it identifies the protocol rather than the program.

MediaMTX command hooks (`runOnOnline`, `runOnOffline`) are deliberately not
used: they run shell commands, and polling a loopback HTTP API is both simpler
to reason about and safer.

---

## 9. Capability-driven metadata model

Platforms do not offer the same metadata fields and do not apply the same
limits. Instead of one shared form, each platform declares its capabilities:

```ts
type PlatformCapabilities = {
  title: boolean;
  description: boolean;
  category: boolean;
  tags: boolean;
  language: boolean;
  visibility: boolean;
  matureContent: boolean;
  dvr: boolean;
  latencyMode: boolean;
};
```

This is complemented by **limits** (maximum title length, number of tags) and an
**option vocabulary** (the type of the category field, the available visibility
levels and latency modes).

This table now lives in the Go backend and is served by
`GET /api/platform-definitions`. The frontend receives booleans, numbers and
option identifiers; it maintains no capability table of its own.

Consequences adopted in the code:

- a field a platform does not support is **not rendered at all** - it is not
  merely disabled,
- the validation schema is **built dynamically** from the capability table on
  both sides, so tag rules do not exist for a platform without tags,
- the backend validates every save against the same table and is the authority;
  the frontend's copy of the rules exists only for immediate feedback,
- adding a new platform means describing it in the registry, not rebuilding the
  form.

Only Twitch has tag support enabled. These definitions are **approximate and
illustrative**: they have **not** been verified against the real Twitch,
YouTube, Kick or TikTok APIs and will be re-checked when real integrations are
implemented.

### 9.1 The localization boundary

The backend never decides the interface language. Provider definitions carry
**semantic identifiers only** — `twitch`, `topic`, `public`, `ultra-low`, `tags`
— and never localized prose such as "Public" or "Publiczny".

The frontend maps those identifiers onto English and Polish translation
resources. Every mapping is total: an identifier this build does not recognise
is rendered as-is rather than crashing the dashboard, so a newer backend
degrades gracefully.

The one exception is brand names (Twitch, YouTube, Kick, TikTok), which are
proper nouns and identical in every language.

User-authored metadata — titles, descriptions, tags — is stored exactly as
entered and is never translated.

---

## 10. Stream key security

A stream key allows broadcasting on someone's channel, so we treat it like a
password.

Rules in force for this project:

1. **No secrets in the repository.** No keys, no tokens, no `.env` files with
   real values. `.gitignore` blocks environment files, database files and data
   directories.

   The SQLite database likewise stores no credentials: no table has a column for
   a stream key, token or password, and no API payload carries one. Write
   endpoints reject unknown JSON fields, so a stray credential field fails
   loudly instead of being silently dropped.

   The generated MediaMTX configuration contains no credential either, and the
   local ingest path (`live` by default) is a **route identifier, not a secret**.
   It is deliberately never labelled as one in the interface, so it cannot be
   confused with a destination platform stream key.
2. **No secrets in the browser.** Keys never go into `localStorage`,
   `sessionStorage`, cookies or React state. `VITE_*` variables are compiled
   into the public JavaScript bundle and must never contain secrets. The only
   value the application persists in the browser is the interface language
   preference.
3. **System credential store.** Keys will eventually be held by an operating
   system mechanism (Windows Credential Manager, macOS Keychain, Secret Service
   on Linux) rather than in application files.
4. **Read at the last moment.** The backend fetches a key only when starting a
   branch and hands it to the FFmpeg process without writing it to the logs.
5. **Masked in diagnostics.** Logs and diagnostic exports must have sensitive
   values stripped.
6. **No secrets in documentation**, including `docs/progress.md` and the
   translation resources.

The same `SecretStore` abstraction backing destination stream keys is designed
to be reused, with a different secret type, for OAuth tokens once connected
accounts exist (§16, engagement-architecture.md §17.1) - one credential-store
foundation, not one bespoke mechanism per feature.

## 11. Localization

The interface is bilingual; the product itself is developed in English.

### 11.1 English is the canonical product language

English is the source language for everything: the interface, the code,
comments, documentation, commit messages and progress entries. Every new string
is written in English first, and English is what the rest of the project is
reviewed against.

### 11.2 Polish is the second supported interface language

Polish is a full translation of the English resources, maintained to parity with
them. It is a translation of the product, not a second source of truth: a string
that does not exist in English must not exist in Polish either.

English is also the **fallback language**. If a Polish entry is ever missing, the
user sees the English text - never a raw translation key.

### 11.3 Static, version-controlled resources

Translations are JSON files under `apps/web/src/i18n/resources/<language>/`,
split into namespaces by feature area. They are reviewed like any other source
file and bundled at build time.

### 11.4 No runtime automatic translation

The project does not use an online translation API, a browser translation
service, an AI translation service or any form of runtime automatic
translation. Every string is a resource written and reviewed by a person.

Content authored by the user - stream titles, descriptions, tags - is rendered
verbatim and is never translated. The same applies to platform brand names,
URLs, the RTMP address, API identifiers and backend error codes.

### 11.5 Extensibility to further languages

Adding a language means adding a resource directory, registering the language
code and its locale tag, and translating the English bundle. The consistency
check (`npm run i18n:check`) then validates the new language against English,
including the plural categories that language actually requires. The procedure
is documented in `README.md`.

## 12. Future server version

The first version runs entirely locally, but the architecture is prepared for
moving the stream router to a remote server:

- the panel talks to the backend only over REST (and later SSE/WebSocket) -
  never directly to MediaMTX or FFmpeg,
- the API address is configurable on the frontend side,
- the backend has an explicit, narrow allow-list of origins (CORS) instead of a
  wildcard,
- the listening port and interface are configured through environment
  variables, restricted to the loopback interface by default.

The server version will additionally require panel authentication, TLS
transport and a considered model for storing secrets server-side. None of these
is implemented yet.

---

## 13. Roadmap

The roadmap now covers two eras of the product: the **streaming router**
(stages 1–7) and the **engagement and overlay platform** (stages 8–19), plus
ongoing hardening (stage 20). §16 explains why the second era exists and how
it is architected; this table only tracks status and dependencies.

| Stage | Scope | Status |
| ----- | ----- | ------ |
| 1 | Foundations: repository structure, documentation, React panel, minimal Go backend, `/api/health` endpoint | **Completed** |
| 2 | English and Polish localization of the frontend | **Completed** |
| 3 | Persistent configuration storage (SQLite), full CRUD API for platforms and metadata | **Completed** |
| 4 | MediaMTX integration: managed dependency, process supervision, configuration generation, OBS ingest detection | **Completed** |
| 5 | Secure credential-store foundation: OS-backed secret storage for destination stream keys, required before real FFmpeg output and any OAuth connector | Planned |
| 6 | FFmpeg destination branches: startup, supervision, restarts, failure isolation | Planned |
| 7 | Connected accounts, OAuth, platform metadata publishing | Planned |
| 8 | Engagement Event Bus and Twitch connector (see [engagement-architecture.md](engagement-architecture.md)) | Planned |
| 9 | Unified operator chat | Planned |
| 10 | OBS chat overlay | Planned |
| 11 | Outbound chat, scheduled bot messages and commands | Planned |
| 12 | Alert engine and alert queue | Planned |
| 13 | Visual overlay designers | Planned |
| 14 | Built-in templates and template import/export | Planned |
| 15 | YouTube and Kick engagement connectors | Planned |
| 16 | External donation-service connectors | Planned |
| 17 | TTS and audio queue | Planned |
| 18 | Goals, counters and event widgets | Planned |
| 19 | TikTok LIVE connector, **only if** an official, permitted, sufficiently stable integration exists | Planned (conditional) |
| 20 | Logs, diagnostics, packaging and remote-server hardening | Planned |

Key dependencies:

- Stage 6 (FFmpeg) and stage 7 (OAuth) both need stage 5's credential store —
  destination stream keys and OAuth tokens are different secret types behind
  the same storage abstraction.
- Stage 8 (Event Bus) is a prerequisite for every stage from 9 onward.
- Stage 11 (outbound/bot) needs connector send-message capability, declared as
  part of a connector's capability set from stage 8 onward.
- Stage 12 (alerts) needs stage 8's normalized events.
- Stage 13 (designers) needs a stable overlay shape, which only exists once
  stages 9/10 (chat) and 12 (alerts) establish what an overlay renders.
- Stage 14 (templates) needs stage 13's designer output format.
- Stage 17 (TTS) and stage 18 (goals/widgets) consume stage 8's bus directly
  and do not depend on the designers.

The full dependency reasoning and the normalized event model behind stages
8–19 are in [docs/engagement-architecture.md](engagement-architecture.md).

Stage 3 was marked completed only after all automated checks passed, including
the scripted verification that configuration and metadata survive a backend
restart.

Stage 4 was marked completed only after all automated checks passed, including a
scripted verification that installs and supervises the real MediaMTX binary. One
limitation is recorded honestly: the `waiting -> receiving -> waiting` ingest
transition is covered against a fake Control API and captured real responses,
but was **not** verified end to end with a real RTMP publisher. See the
`feat(server): supervise MediaMTX runtime and ingest` entry in
[progress.md](progress.md).

Stage 5, like every stage before it, is marked completed only once its
automated checks pass — see [progress.md](progress.md) for the entry recording
that.

## 14. The manual testing rule

**Manual testing is the final stage and is performed only after the application
functionality is complete.**

Rationale: as long as most of the streaming path consists of placeholders,
manual testing would only exercise demo data and create a false sense of
readiness.

During implementation the following automated checks apply instead, and should
be run continuously:

- `npm run i18n:check` - translation resource consistency,
- `npm run typecheck` - TypeScript type checking,
- `npm run lint` - ESLint static analysis,
- `npm run test` - frontend unit tests,
- `npm run build` - frontend production build,
- `go build ./...` - backend compilation,
- `go vet ./...` - backend static analysis,
- `go test ./...` - backend tests,
- `gofmt -l .` - backend formatting check,
- `node scripts/verify-persistence.mjs` - scripted verification that
  configuration and metadata survive a backend restart, run against a temporary
  database,
- `node scripts/verify-mediamtx-runtime.mjs` - scripted verification that the
  real MediaMTX binary is installed, checksum-verified, supervised and reused
  across a backend restart, run against a temporary data directory on
  dynamically chosen ports.

## 15. Honesty about the state of the work

Both the documentation and the interface follow one rule: **an unimplemented
feature is never presented as finished.**

In practice this means:

- demo data is marked with a "Demo" badge in the interface and with a comment in
  the code,
- buttons that perform no real operation say so clearly,
- unimplemented pages show a description of the planned scope instead of fake
  widgets,
- an entry in `docs/progress.md` does not mark a feature as completed if it is
  only an interface placeholder.

## 16. Engagement and overlay platform (planned)

**Status: planned. Nothing in this section is implemented.**

The product's long-term scope is larger than a streaming router. Streaming
Tree is also planned to become a **local streaming engagement and overlay
platform**: normalized chat and events from multiple platforms, a unified
operator chat, OBS Browser Source overlays, outbound chat and scheduled bot
messages, alerts and an alert queue, visual overlay designers with a safe
template format, text-to-speech, and goal/counter widgets.

The full architecture — the normalized event model, the connector interface
and capability model, deduplication and ordering rules, the operator-chat vs.
overlay distinction, bot automation, the alert and queue design, the template
security model, TTS, and the staged implementation order — is documented
separately in **[docs/engagement-architecture.md](engagement-architecture.md)**,
so this overview is not doubled in length by planning detail that has no
bearing on what is running today.

Three things are worth stating plainly here, because they shape decisions this
task makes now:

1. **The credential-store foundation this task implements (§10) is a hard
   prerequisite for this entire second era of the product.** FFmpeg
   destination stream keys, OAuth tokens for connected accounts, and any
   future outbound-bot credential all depend on the same `SecretStore`
   abstraction, distinguished only by secret type.
2. **A connected account (for reading chat/events) is a different concept
   from a configured destination (for outgoing streaming), even for the same
   provider**, because they depend on different credential types with
   different lifecycles. See engagement-architecture.md §4.
3. **Provider support is planned honestly**: Twitch first, YouTube and Kick
   as separate adapters, and TikTok only if and when an official, stable
   integration exists — never via scraping as a core feature. See
   engagement-architecture.md §16.

This section will be updated, and marked accordingly, only as each roadmap
stage from §13 is actually completed - not before.
