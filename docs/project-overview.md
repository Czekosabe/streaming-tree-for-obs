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
                       │ supervises              │ supervises
                       ▼                         ▼
        ┌──────────────────────┐    ┌──────────────────────────────────┐
  OBS ─▶│  MediaMTX            │───▶│  FFmpeg (one process per branch) │
  RTMP  │  local RTMP ingest   │    │  ffmpeg #1 ─▶ Twitch             │
        └──────────────────────┘    │  ffmpeg #2 ─▶ YouTube            │
                                    │  ffmpeg #3 ─▶ Kick               │
                                    │  ffmpeg #4 ─▶ TikTok             │
                                    └──────────────────────────────────┘
```

The layers are separated on purpose: the panel never talks directly to MediaMTX
or FFmpeg. All control flows through the Go backend, which is what will later
allow the backend to be moved to a remote server without changing the panel.

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
- it holds platform configuration and metadata,
- it starts and supervises MediaMTX and the FFmpeg processes,
- it reads stream keys from the system credential store at branch start time,
- it enforces failure isolation and the restart policy,
- in later stages it pushes live state over SSE or WebSocket.

Go was chosen for three reasons: distribution as a single binary with no
runtime to install, good support for supervising child processes, and a simple
concurrency model for many independent branches.

### 7.4 Planned role of MediaMTX

**Status: not implemented.**

MediaMTX will act as the local server receiving the stream from OBS. Instead of
writing an RTMP implementation, the application will run MediaMTX as a child
process with a generated configuration and pull a single source stream from it
for all branches.

This is what lets OBS encode the video **once** while every branch shares the
same source.

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
**option vocabulary** (the name of the category field, the available visibility
levels and latency modes).

Consequences adopted in the code:

- a field a platform does not support is **not rendered at all** - it is not
  merely disabled,
- the Zod validation schema is **built dynamically** from the capability table,
  so tag rules do not exist for a platform without tags,
- adding a new platform means describing it, not rebuilding the form.

In the current demo configuration only Twitch has tag support enabled. These
configurations are **approximate and illustrative** - they will be verified when
real API integrations are implemented.

---

## 10. Stream key security

A stream key allows broadcasting on someone's channel, so we treat it like a
password.

Rules in force for this project:

1. **No secrets in the repository.** No keys, no tokens, no `.env` files with
   real values. `.gitignore` blocks environment files and data directories.
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

| Stage | Scope | Status |
| ----- | ----- | ------ |
| 1 | Foundations: repository structure, documentation, React panel, minimal Go backend, `/api/health` endpoint | **Completed** |
| 2 | English and Polish localization of the frontend | **Completed** |
| 3 | Persistent configuration storage (SQLite), full CRUD API for platforms and metadata | Planned |
| 4 | MediaMTX integration: process startup, configuration generation, OBS connection detection | Planned |
| 5 | FFmpeg branches: startup, supervision, restarts, failure isolation | Planned |
| 6 | Live status over SSE or WebSocket instead of polling | Planned |
| 7 | Operating system credential store for stream keys | Planned |
| 8 | Platform API integrations: OAuth, pushing metadata, reading viewer counts | Planned |
| 9 | Log view and diagnostics, diagnostic bundle export | Planned |
| 10 | Application packaging and server mode | Planned |

The order may change, but stages 4 and 5 depend on stage 3, and stage 8 depends
on stage 7.

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
- `gofmt -l .` - backend formatting check.

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
