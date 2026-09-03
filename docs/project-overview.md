# Streaming Tree for OBS — project overview

> This document describes the goals, architecture and roadmap of the project.
> Sections marked as **planned** are not implemented yet.
> The current state of the work is recorded in [progress.md](progress.md).

---

## 1. Project name

**Streaming Tree for OBS**

The name describes how the application works: a single stream leaving OBS is the
"trunk", and every destination platform is an independent "branch".

Created by **Czekosabe** (<https://github.com/Czekosabe>). Canonical
repository: <https://github.com/Czekosabe/streaming-tree-for-obs>. The
application's public creator identity, licence status, privacy posture and
creator-support model are defined in
[`docs/product-identity-legal.md`](product-identity-legal.md), and surfaced
in-app via `Settings → About & Legal`.

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
                       │ supervises              │ supervises
                       ▼                         ▼
        ┌──────────────────────┐    ┌──────────────────────────────────┐
  OBS ─▶│  MediaMTX  [DONE]    │──▶ │  FFmpeg (one process per branch) │
  RTMP  │  local RTMP ingest   │    │  ffmpeg #1 ─▶ Twitch      [DONE] │
        │  127.0.0.1:1935/live │    │  ffmpeg #2 ─▶ YouTube     [DONE] │
        └──────────────────────┘    │  ffmpeg #3 ─▶ Kick        [DONE] │
                 ▲                  │  ffmpeg #4 ─▶ TikTok      [DONE] │
                 │ Control API      └──────────────────────────────────┘
                 │ 127.0.0.1:9997
                 │ (backend only, never the browser)
            readiness + ingest status
```

Every arrow above is implemented. The backend supervises MediaMTX and reads
its Control API on loopback; it also supervises one independent FFmpeg
process per enabled, started destination, pulling the same local input.
The browser never contacts MediaMTX or a branch's FFmpeg process directly -
only the backend's REST API reports their state.

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
application's local address, `rtmp://127.0.0.1:1935/live` by default - this is
implemented and working today (see [7.4](#74-the-role-of-mediamtx-implemented)).

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
- it starts and supervises MediaMTX (implemented, §7.4) and starts and
  supervises one FFmpeg process per destination branch (implemented, §7.5),
- it reads a stream key from the system credential store only immediately
  before starting that branch's FFmpeg process (implemented, §7.5, §10) -
  never for a status check, never cached, never logged,
- it enforces failure isolation and an independent bounded-restart policy
  for each FFmpeg branch (implemented, §7.5) - the same principle that
  already governs MediaMTX supervision, applied per destination so one
  branch failing cannot affect another,
- it pushes live state to public overlay/widget routes over Server-Sent
  Events (chat, alerts, audio, goal/supporter widgets - see
  `docs/engagement-architecture.md`),
- it reports its own real, local-only, bounded-sampling host CPU/memory/
  disk usage (`GET /api/system/resources`, `internal/sysresources`) for
  the Dashboard's System Resources card - never network throughput, and
  never anything that leaves this process; no fake/demo numbers are ever
  shown in their place.

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
  quality or process state, for MediaMTX or for any destination branch. That
  now-implemented runtime engine (§7.4, §7.5) keeps its state in memory only,
  deliberately, so configuration and "what is happening right now" can never
  be confused with each other - see §8.1,
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

### 7.5 The role of FFmpeg (implemented)

**Status: implemented.**

For each destination the operator explicitly starts, the backend spawns a
separate FFmpeg process that reads the shared local MediaMTX input and pushes
it to that destination's configured RTMP/RTMPS server, entirely independent
of every other destination's process.

#### Resolution and compatibility

FFmpeg has no single official binary distributor the way MediaMTX does, so
unlike MediaMTX, **this application never downloads it**. It is resolved in
order: an explicit `STREAMING_TREE_FFMPEG_PATH` override, a possible future
bundled location beside the backend (documented, no binary committed today),
then the system `PATH` - the `PATH` fallback is deliberately the opposite of
MediaMTX's resolver, precisely because there is no approved managed source to
prefer over it here.

Compatibility is decided by **probing capabilities**, not matching an exact
version: `ffmpeg -version` parses, RTMP input, RTMP output, RTMPS output, the
FLV muxer, and `-progress` support. A documented minimum version (4.4) is a
floor, not a ceiling - a newer FFmpeg that passes every probe is never
rejected for being newer than this code, and an old or stripped-down build
that fails a probe is incompatible regardless of its version string. The
resolved executable path is never sent to the browser, matching MediaMTX's
own `Path` field convention - only a semantic source identifier.

#### Output configuration

Each destination has its own output settings, stored in SQLite exactly like
its display name or metadata: a server URL (`rtmp://` or `rtmps://`, host
required, no embedded user-info, no fragment) and an automatic-restart
preference. **Never** stored: the stream key, a destination URL containing
it, or any runtime field. This is a deliberately separate concern from the
credential store (§10): the server address is not a secret and is cached and
displayed normally; the stream key is retrieved fresh from the OS credential
store only at the moment a branch actually launches.

#### Branch supervisor

One `Manager` (`internal/runtime/branch`) supervises every destination
through an explicit state machine - `idle`, `blocked`, `waiting_for_ingest`,
`starting`, `live`, `restarting`, `stopping`, `error` - the same design
principle as MediaMTX's own supervisor (§7.4): one value, not a set of
booleans, so "starting and live" is unrepresentable.

It tracks **desired-running** separately from actual process state, because
those diverge for real reasons: an explicit Start means "keep this running";
the local input can disappear temporarily without that desire changing;
FFmpeg can crash independently of anything the operator did; an explicit
Stop must both terminate the process and suppress any future automatic
restart. Eligibility is recomputed on every request, in a fixed order
(platform enabled → output server configured → stream key present →
credential store reachable → compatible FFmpeg present → MediaMTX ready →
ingest actually receiving), and reported as stable, frontend-localized
blocker identifiers rather than one opaque "cannot start" flag.

`live` means FFmpeg has produced **real, advancing `-progress` output** -
never merely that a process was spawned. If the local input disappears, the
affected branch(es) pause (`waiting_for_ingest`) rather than being treated as
crashed, and resume automatically once input returns, but only for a branch
that is still desired-running - an explicit Stop is never silently
overridden. A genuine unexpected exit is retried with the same bounded
exponential-backoff policy MediaMTX uses in spirit (1 s to 30 s, at most 5
attempts in 5 minutes, a stable run resetting the count) - implemented as an
independent policy per branch, not a shared import, so the two supervisors
stay decoupled. **One branch's failure never affects another's process,
state, or restart count.**

A backend restart resets every branch to not-desired-running with a fresh
restart counter - starting a broadcast is always required to be a deliberate,
explicit action, never something a backend restart resumes on its own - while
the output settings themselves persist in SQLite like any other configuration.

#### Secret handling at launch

The stream key is retrieved from the credential store (§10) only immediately
before the process is spawned, via a retrieval method reachable only from
this supervisor - never from a status check, never from the HTTP layer. It is
never written to SQLite, logged, placed in a diagnostic buffer, or returned
by any API response; FFmpeg's own captured stdout/stderr is redacted (the
exact key, the full destination URL, and their URL-escaped variants replaced
with `[REDACTED]`) before it is ever logged or stored anywhere. The one
honestly-documented limitation: no safer FFmpeg CLI mechanism exists for
passing a per-run RTMP destination, so the key is present as a process
command-line argument while that branch runs - visible, on most operating
systems, to another process on the same machine with permission to inspect
process lists. This is accepted only for this local, single-user stage, with
every other channel (logs, errors, API responses, diagnostics) actively
closed.

#### Design constraints kept

- stream copy only in this stage (`-c copy`) - no re-encoding, no adaptive
  bitrate, no transcoding presets; a source codec FLV/RTMP cannot carry fails
  that one branch fast with a clear error instead of silently starting an
  expensive re-encode,
- one process per branch means a separate lifecycle, separate captured
  output, and a separate restart policy - confirmed by real, independent
  process isolation in `scripts/verify-ffmpeg-branches.mjs`, not merely by
  the type system.

---

## 8. The independent branch model

**Status: implemented** (§7.5). Every configured destination is an independent
branch with its own explicit lifecycle:

```
        ┌── blocked ◀────────────────────────────┐
        │      ▲                                 │
 idle ──┼──────┘                                  │
        │                                         │
        └─▶ starting ──▶ live ──▶ stopping ──▶ idle
                 │          │
                 │          ▼
                 │   waiting_for_ingest ──▶ starting (ingest returns)
                 ▼
             restarting ──▶ starting (retry) / error (limit reached)
```

(`State` in `internal/runtime/branch` is one value, not a diagram of
booleans - see §7.5 for the full state list and what each transition means.)

Rules, all implemented and covered by both fast unit tests and the real,
loopback FFmpeg/MediaMTX integration script:

1. **Process isolation.** One branch means one FFmpeg process. A process failure
   does not touch the others.
2. **Error isolation.** One destination's output connection failing moves only
   that branch into the `error` state after its own restart budget is spent.
3. **Independent control.** Branches are started, stopped and restarted
   individually, through their own HTTP endpoints, without interrupting any
   other destination.
4. **Independent restart.** The bounded-exponential-backoff retry policy is
   tracked per branch, with its own backoff, restart count and time window.
5. **Shared source.** All branches read the same local MediaMTX input, so
   adding a destination puts no extra load on OBS.

## 8.1 Four concepts that must not be confused

The domain deliberately separates four things that look similar and behave
completely differently. The fourth, a connected account, was added in stage
7A.

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

### Connected account

A real provider identity Streaming Tree has authorized via OAuth - for
example, a specific Twitch login. It lives in
`internal/domain/account`, entirely independent of any configured platform:
an account can exist unlinked, one account can be linked to more than one
configured platform, and a configured platform has at most one linked
account at a time. A connected account's non-secret record (login, display
name, avatar, status, granted scopes) is a database row; its OAuth token
bundle is a single SecretStore entry, atomically replaced on every
rotation, and is never present in that database row, in an HTTP response,
or in a log line.

A connected account, a configured platform, a stream key, an output
server, and a destination's runtime branch state are five separate facts
that together describe one destination, never conflated into one: a
platform can be fully configured with a stream key and an output server
and have its FFmpeg branch live, entirely independent of whether a Twitch
account happens to be connected and linked to it, and vice versa.
Connecting or linking an account never starts, stops, or otherwise touches
a branch, and linking never validates or replaces a stream key.

**A destination also carries a sixth, YouTube-specific fact: its
remote broadcast target** (`internal/domain/remotetarget`,
`platform_remote_targets` - `platform_id` primary key cascading from
`platforms`, `provider_id`, `resource_type`, `resource_id`,
`display_name`; no token, no stream key, no ingestion field). A connected
YouTube account is not enough to know *which* live broadcast a
destination's metadata should be read from and published to - that is
this sixth fact, selected explicitly, never auto-selected, and never
presented as verifying that the selected broadcast uses the stream key
configured for the same destination. Deliberately a provider-independent
table (any future provider needing the same "which remote resource"
concept reuses it) rather than a YouTube-only column bolted onto
`connected_accounts` or `platforms`.

**A connected account, rather than a destination, carries a seventh
fact: its engagement-connector configuration**
(`internal/domain/engagementsettings`,
`connected_account_engagement_settings` - `account_id` primary key
cascading from `connected_accounts`, one `enabled` boolean, timestamps;
no token, no WebSocket session id, no subscription id). Whether a Twitch
account's inbound EventSub connector is enabled is a persisted
preference, deliberately as small as the fact it records; everything
about the connector's *live* session (its state, subscription counts,
reconnect count, last event/data-gap timestamps) is runtime state, kept
only in memory, described below alongside MediaMTX's and a branch's own
runtime state. A capability-specific scope assessment
(`internal/provider/twitch.AssessEngagementCapability`) is computed on
demand from the account's already-stored granted scopes, not persisted
separately - metadata health (`channel:manage:broadcast`) and engagement-
capability health are independent facts about the same account, never
conflated, so an account can be perfectly healthy for metadata while
still needing an explicit permission upgrade before engagement can
enable at all.

**An eighth fact is not about any single account at all, but about
the operator's own presentation preferences: unified-operator-chat
settings** (`internal/domain/operatorchatprefs`,
`operator_chat_preferences` - a singleton row of presentation toggles;
`operator_chat_account_visibility` - per-account visibility overrides;
`operator_chat_hidden_users`/`operator_chat_bot_users` - operator-
maintained lists identified by the provider's own stable user id,
never a display name). Deliberately as small as engagement settings
above: no message text, no username treated as authoritative identity,
no token, no raw provider event. Everything about actual chat
content - the merged timeline itself - is transient, in-memory-only
projection state, described below alongside the Event Bus's own
runtime state, and is gone on every backend restart exactly like it.

**A ninth fact is a persisted, public-facing presentation profile
rather than the operator's own preferences: chat-overlay profiles** (`internal/domain/chatoverlay`, five tables -
`chat_overlays` itself holding layout/visibility/filter/typography/color/
animation/role-highlighting settings as explicit columns, never a JSON
settings blob; `chat_overlay_accounts`, `chat_overlay_hidden_users`,
`chat_overlay_blocked_terms` and `chat_overlay_activity_types` as its
child tables). An overlay's own hidden-user list is a genuinely separate
table from the operator's own `operator_chat_hidden_users` above - a user may
stay visible to the operator while being hidden from one specific public
overlay. Each overlay's public slug (a separate, higher-entropy value
from its management id) is documented explicitly as an unguessable
locator, not a credential - it is never stored in `internal/secrets`,
and rotating it only ever changes that one column. Like every other
table in this section: no message text, no raw provider event, and
nothing that constitutes actual chat content is ever stored here either
- only the operator's own presentation and filtering choices for that
overlay.

**Manual outbound-chat permission adds no new persisted fact of its
own.** It is a third, independently assessed capability
profile on the same connected-account row already described above -
`internal/provider/twitch.AssessOutboundChatCapability`
computes it on demand from the account's already-stored granted
scopes (this time checking for `user:write:chat`), exactly the way
`AssessEngagementCapability` already does for the five inbound scopes.
Metadata health, inbound-engagement health and outbound-chat health
are three independent facts about the same account, never conflated:
an account can be perfectly healthy for metadata and for reading chat
while still needing its own separate permission upgrade before it can
send anything. There is no new SQLite table and no new column - the
capability is computed, not stored, and the queue/dispatcher state
that goes with it is pure runtime state, described below alongside
every other in-memory projection in this section.

### Runtime stream state

Whether the ingest service is running, whether a publisher is connected,
process restart counts, the last runtime error — and, for every configured
destination, its own independent FFmpeg branch state.

**Runtime state lives only in memory**, for MediaMTX and for every branch. It
is never written to the SQLite tables and resets when the backend restarts,
because it describes what is happening right now rather than what the user
configured. No migration in this project adds a runtime column, and none
should.

What is tracked for MediaMTX: whether it is installed, whether its version is
compatible, its process state, readiness, restart count, the last error,
whether a publisher is connected, when the input became available, the source
type and the track identifiers it reports.

What is tracked per destination branch (§7.5): its state, whether it is
desired-running, its current blockers, started/live/stopped timestamps, its
restart count, its real FFmpeg `-progress` fields (frame count, fps, output
time, total size, speed), and a sanitized last error. Never tracked, for
either: bitrate, resolution, frame rate, dropped frames or viewer counts that
the underlying process did not itself report, and never a stream key, a full
destination URL, an FFmpeg command line, a process id, or process
environment - inventing or exposing any of those would defeat the point of
reporting real state at all.

What is tracked per Twitch engagement connector
(`internal/runtime/twitchengagement`): an explicit state (`disabled`,
`blocked`, `connecting`, `waiting_for_welcome`, `subscribing`,
`connected`, `reconnecting`, `stopping`, `error` - never independent
booleans for mutually exclusive facts), blocker codes, connected/last-
event/last-keepalive/last-data-gap timestamps, reconnect count, and
active/expected subscription counts. Never tracked: a WebSocket session
id, a reconnect URL, an access token, or a raw provider response -
exposing any of those would defeat the same "no secret in a diagnostic
view" rule stream-key and OAuth-token status already follow. The
Engagement Event Bus itself (`internal/engagement`) is the same kind of
fact at a different scope: a bounded, in-memory-only ring buffer of
normalized events (default capacity 1000, `STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE`-
configurable) that resets to empty on every backend restart, exactly
like MediaMTX's and a branch's own runtime state above - see
[docs/engagement-architecture.md](engagement-architecture.md) §5-6 for
the normalized event model and bus design themselves.

**The unified-operator-chat projection
(`internal/operatorchat`) is the same kind of fact one layer up**: a
second, independently bounded, in-memory-only ring buffer (default
capacity 500, `STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE`-configurable),
holding chat-shaped items derived from the Event Bus's own normalized
events rather than a second copy of provider state. It begins empty on
every backend start - no pre-restart chat history is ever claimed -
and tracks only its own sequence/capacity/subscriber counts and a
one-way "was a gap from the bus ever detected" flag, never a raw
provider payload. This is the clearest illustration in the project so
far of the rule this whole subsection follows: *what happened* (a
persisted, provider-independent fact - here, an operator's display
preferences) and *what is happening right now* (transient, in-memory,
provider-derived - here, the actual chat timeline) are never the same
storage.

**The public chat-overlay projection (`internal/chatoverlay`) is a
second, independent consumer of this same rule, one layer further out
again**: for every overlay profile it holds its own filtered, bounded,
in-memory-only current-item view plus a separate revision ring (a fixed
capacity, not environment-configurable, unlike the Event Bus's and
operator-chat's own buffer sizes above) for live Server-Sent Events
replay. It deliberately does **not** subscribe to the Engagement Event
Bus directly - it consumes `internal/operatorchat`'s own already-
lifecycle-correct revision stream instead, so none of the operator-chat
projection's own deduplication, deletion or moderation-filtering logic
is duplicated a second time. Like every runtime projection in this section, it begins
empty on every backend start and tracks only its own sequence/capacity/
subscriber counts, never a raw provider payload.

**The outbound-chat dispatcher (`internal/outboundchat`) is
runtime state of the same kind, at the opposite end of the pipeline**:
one bounded, in-memory-only send queue per connected account (not
persisted, not shared with any other account's queue), tracking only
its own state (idle/queued/sending/rate\_limited/stopping/error), queue
depth/capacity, last-attempt/last-success timestamps, a stable last
error code and a sanitized retry time - never a message's own text,
never a Twitch response body or header, never the OAuth token used to
send it. It begins empty on every backend restart exactly like the
Event Bus and the two chat projections above; nothing about a queued
or in-flight send survives a restart, and no outbound message is ever
persisted anywhere once sent, matching this project's existing rule
that actual chat content - inbound or outbound - is never written to
SQLite.

**Hydration protocol.** The frontend overlay route fetches the public
config once, then opens the public SSE stream and relies entirely on
that stream's own first event - always a complete `reset` of the
current visible set - for its initial state. It never merges a
separate `GET /api/public/chat-overlays/{slug}/items` request with the
stream: that endpoint's response and the stream's own reset are two
independently-timed reads of the same mutable projection, so combining
them would introduce a real race (an item could change visibility
between the two reads) for no benefit, since the reset already carries
a strict superset of what the snapshot endpoint would answer. `/items`
remains a fully supported, separate read for a script or diagnostic
tool that only needs a point-in-time value - see
[`docs/obs-browser-source.md`](obs-browser-source.md) for the complete
reasoning and the reconnect/`Last-Event-ID`/gap behavior.

**Removal reason and the cosmetic/immediate safety split.** Every
`remove` revision carries a stable, closed-enum reason
(`expired`/`capacity_evicted`/`message_deleted`/`chat_cleared`/
`user_messages_cleared`/`unknown`) - never the removed item's own
message text, and never any detail about which blocked term or hidden
user caused it. Only two of those reasons are safe to animate on the
frontend: `expired` (natural message-lifetime expiry) and
`capacity_evicted` (the oldest item evicted once `maxVisibleItems` is
exceeded) - genuinely cosmetic removals with nothing to hide. Every
other reason is treated as an immediate removal: no exit animation, no
retained "leaving" copy of the item, applied on the same render pass
the revision arrives on. A settings/privacy change that hides
previously-visible items (a newly blocked term, a newly hidden user, a
filter toggle, a narrowed account selection, the overlay being disabled
or deleted) never travels as an individual `remove` at all in this
design - it travels as the same full projection `reset` a `Configure`
rebuild already produces, which the frontend always applies
immediately and in full, exactly like the initial hydration reset
above; a reset never triggers a mass exit-animation moment for every
item that happened to still be visible before it.

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

One field the capability table alone could not express is
`categoryRequiresRemoteId`, which a provider may declare, meaning its category is not
free text but a selection from the provider's own catalog (Twitch: a
game/category ID from its Search Categories endpoint). Stored metadata then
carries both `category` (display text, shown even for a provider this
build does not recognize) and an independent, nullable `categoryId` (the
provider's stable identifier, never guessed from the text). A destination
whose saved category text has no matching ID is a publish blocker, not a
best-effort guess at which remote category the text meant.

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

**Twitch's and YouTube's definitions were both verified against their
real APIs** - see
[`docs/provider-integrations/twitch.md`](provider-integrations/twitch.md)
and
[`docs/provider-integrations/youtube.md`](provider-integrations/youtube.md)
for the researched contracts. Twitch turned out to have **no** field for
description, visibility, a generic mature-content flag, DVR, or a
client-side latency mode on the real Modify Channel Information endpoint;
YouTube turned out to have **no** generic mature-content flag either (its
closest field, `selfDeclaredMadeForKids`, is a COPPA child-directed
disclosure, not a maturity rating) and no DVR or latency-mode write path
this stage's single-call, video-only publish reaches - both corrections
replaced an earlier approximation that had assumed all three existed.
YouTube does have real tag support, unlike Twitch's per-tag/count model:
its limit is a combined byte budget across every tag together, which
needed a second `Limits` field (`TagsCombinedMaxLength`) rather than
reusing Twitch's `MaxTags`/`TagMaxLength` as-is. Kick and TikTok's
definitions remain **approximate and illustrative**: they have **not**
been verified against their real APIs and will be re-checked when their
own account integration is implemented (stage 7C).

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
3. **System credential store.** A destination stream key is held by an
   operating system mechanism - Windows Credential Manager, macOS Keychain, or
   Linux Secret Service - via the `SecretStore` interface in
   `apps/server/internal/secrets`, never in an application file. There is no
   plaintext fallback: if the store cannot be reached, the key is left
   unstored and the API reports a stable `credential_store_unavailable`
   status rather than writing it anywhere else. The production implementation
   (`KeyringStore`) wraps `github.com/99designs/keyring`, restricted to
   exactly those three backends - the library's `pass` and `file` backends
   (an external command and a password-encrypted file, respectively) are
   excluded, since both are exactly the kind of fallback this rule forbids.
   See `docs/progress.md`, entry `feat(server): add system credential store`,
   for why this library was chosen over the alternative that was rejected
   after reading its source.
4. **Read at the last moment.** `credential.Service.RetrieveForProcessStart`
   exists for this purpose and is not reachable through the HTTP API - the
   interface `internal/httpapi` depends on has no method that returns a
   secret value, so the web panel cannot obtain one even indirectly. Its one
   caller is the branch manager (§7.5), which calls it only immediately
   before spawning that destination's FFmpeg process - never for a status
   check, never cached, never called again while the process keeps running.
5. **Masked in diagnostics.** Logs and diagnostic exports must have sensitive
   values stripped.
6. **No secrets in documentation**, including `docs/progress.md` and the
   translation resources.
7. **Never re-displayed, never verified.** Once stored, a key cannot be read
   back through this application - there is no "show saved key" control.
   Replacing overwrites the previous value; deleting removes it. The
   interface reports only "Stored", "Missing", or that secure storage is
   unavailable - never "Valid", "Connected" or "Authenticated", since nothing
   in this stage contacts a platform to check a key.
8. **The credential key namespace is centralized**, not left to each call
   site: `secrets.BuildKey(secretType, subjectID)` produces
   `<secret-type>:<subject-id>`, for example `destination-stream-key:pf_abc123`.
   `subjectID` is always a platform's generated ID, never its display name, so
   a rename can never orphan a key, and two destinations configured for the
   same provider always resolve to independent keys - the provider ID is
   never part of the key at all.
9. **Platform deletion and its credential are not one atomic operation** -
   SQLite and the OS credential store cannot share a transaction. The
   credential is deleted first; the platform row is only removed once that
   succeeds, except when the store is merely unreachable (not failing), in
   which case platform deletion proceeds rather than blocking ordinary CRUD
   on a transient outage. See `docs/progress.md` for the full reasoning and
   the accepted, documented risk this trade-off carries.

The same `SecretStore` abstraction backing destination stream keys also
backs a connected account's OAuth token bundle, for both Twitch and
YouTube, under the same secret type
(`oauth-token-bundle:<connected-account-id>`) and key namespace, subject
to every rule above: no plaintext fallback, never re-displayed through
the API, never in a log line. Unlike a stream key, an OAuth token bundle
is refreshed and rotated automatically by
`internal/domain/account.Service`, and is stored as one atomically-replaced
unit (access token, refresh token, token type, expiry together) rather than
independently-replaceable pieces, since a partial rotation failure leaving a
mismatched access/refresh pair would be worse than the old pair simply
staying in place. One credential-store foundation, not one bespoke
mechanism per feature — see §8.1's "Connected account" and
[`docs/provider-integrations/twitch.md`](provider-integrations/twitch.md).

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

## 12.1 Windows packaging and the application updater

Both are implemented; this overview does not restate their contracts.

- **Packaging**: a per-user Inno Setup installer with no elevation, no
  Node/npm/Go required at the end-user's machine, the production React
  frontend and the four legal documents embedded directly into a single
  Go executable (`//go:embed`) served from one loopback origin - no
  Electron or other bundled-browser-engine layer. MediaMTX keeps its
  existing managed-installation model (downloaded on request, checksum-
  verified); FFmpeg remains operator-provided, never bundled. Unsigned -
  no production Authenticode certificate exists yet. See
  [windows-packaging.md](windows-packaging.md) for the complete contract
  (installer comparison and choice, packaged-mode lifecycle, GPL
  packaging obligations, and known limitations).
- **Updater**: checks the fixed, canonical `Czekosabe/streaming-tree-for-obs`
  GitHub Releases repository only - never configurable, never a branch or
  arbitrary artifact. Strict `major.minor.patch` versioning, Stable-only,
  never a downgrade. A project-controlled release manifest plus mandatory
  SHA-256 verification is the integrity boundary (not code-signing, stated
  honestly). Checking/downloading are allowed while streaming; installing
  is not. See [updater.md](updater.md) for the complete contract (the
  external-helper handoff, the active-stream guard, the failure model, and
  known limitations) and [PRIVACY.md](../PRIVACY.md) for what the updater
  does and does not send.

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
| 5 | Secure credential-store foundation: OS-backed secret storage for destination stream keys, required before real FFmpeg output and any OAuth connector | **Completed** |
| 6 | FFmpeg destination branches: resolution/compatibility probing, output settings, per-branch supervision, restarts, failure isolation | **Completed** |
| 7A | Connected-account foundation and a first provider integration: Twitch device-code sign-in, account lifecycle (validate/refresh/reconnect/disconnect), destination linking, and explicit channel-metadata publishing | **Completed** |
| 7B | YouTube account integration: Authorization Code Flow with PKCE via a loopback callback, multi-channel selection, a provider-independent remote-broadcast-target association, and explicit video-metadata publishing, reusing the same connected-account foundation | **Completed** |
| 7C | Kick and TikTok account integration | Deferred — capability-gated, not a prerequisite for stage 8. Kick's own engagement feasibility was researched in stage 15B and found feasibility-gated (webhook-only event delivery, no public inbound endpoint available) - Kick account integration remains deferred alongside it. TikTok's own account integration is now folded into stage 19's feasibility gate rather than pursued independently - stage 19 found no official LIVE engagement capability for such an account to power (see [tiktok-live.md](provider-integrations/tiktok-live.md)) |
| 8A | Engagement Event Bus and a real Twitch inbound connector (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 8B | Additional Twitch event coverage, reserved only if stage 8A cannot safely cover the full verified event set | Planned, conditional |
| 9 | Unified operator chat: a real, merged Twitch chat page consuming the Engagement Event Bus, provider-independent projection, persisted preferences, Twitch badge/emote resolution (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 10 | OBS chat overlay: persisted overlay profiles, a public per-overlay projection over the operator-chat projection, a public HTTP/SSE API and a management page (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 11A | Manual outbound Twitch chat: a third, independent send-permission profile, a real Send Chat Message adapter, an in-memory per-account dispatcher, and manual sending/replying from the Chat page (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 11B | Scheduled bot messages and chat commands, built on the same dispatcher stage 11A introduced: interval/jitter/streaming/activity/rate gating, message groups, command roles/aliases/cooldowns, a closed placeholder language, and the Automation page (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 12A | Alert engine and alert queue: persisted alert profiles/rules, a provider-independent matcher over the same Event Bus, a bounded in-memory queue (priority, expiration, pause/resume/skip/replay/clear), local synthetic test alerts, a fixed (non-designer) presentation, and a public OBS Browser Source alert route, plus the Alerts management page (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 12B | Mid-alert preemption and bounded alert grouping, deliberately deferred out of 12A | **Completed** — stage 12 as a whole is now complete |
| 13A | Shared, provider-independent visual-design document, persistence/HTTP API, immutable per-alert snapshotting, shared React renderer, and the Alert Overlay Designer editor (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 13B | Chat Overlay Designer, reusing 13A's shared document/renderer for one repeated chat item card, with Stage 10's own filtering/lifecycle staying authoritative | **Completed** — stage 13 as a whole is now complete |
| 14A | Reusable visual-template library: built-in templates, a persisted user template library, target/owner-instance compatibility, and asset-free JSON import/export, built on stage 13's document format (see [engagement-architecture.md](engagement-architecture.md)) | **Completed** |
| 14B | Portable archive template packages, managed template assets, and safe custom image/video/font primitives (see [visual-template-packages.md](visual-template-packages.md)) | **Completed** — stage 14 as a whole is now complete |
| 15A | YouTube engagement connector (Live Chat received over the official `streamList` gRPC server-streaming transport), integrated into the existing operator chat / Event Bus / alerts / outbound-chat pipelines unchanged, and a first real monetary alert capability (Super Chat/Super Sticker, integer-micros money, no currency conversion) | **Completed** |
| 15B | Kick engagement connector | Deferred — feasibility-gated: Kick's currently-documented event delivery is webhook-only, requiring a public inbound HTTPS endpoint this local-first deployment target does not offer, and no scraping/tunneling/relay workaround is acceptable (see [kick-engagement.md](provider-integrations/kick-engagement.md)). Stage 15 as a whole is **not** complete until this is resolved or explicitly re-scoped |
| 16A | External donation foundation and a real StreamElements donations connector: a provider-independent `donationsource` domain (deliberately separate from `connected_accounts`), a real Astro WebSocket connector, exact integer-micros money conversion, moderation-aware (pending/allowed/rejected) publish semantics, and full reuse of the existing Event Bus/operator chat/alerts pipeline (see [external-donations.md](provider-integrations/external-donations.md)) | **Completed** |
| 16B | Additional external donation providers (Streamlabs, Ko-fi) | Deferred — feasibility-gated: Streamlabs' documented OAuth2 token exchange requires a confidential `client_secret` with no public/native-client alternative found, and unapproved apps are capped at 10 whitelisted users; Ko-fi is webhook-only, requiring a public inbound HTTPS endpoint this local-first deployment target does not offer (see [external-donations.md](provider-integrations/external-donations.md)). Stage 16 as a whole is **not** complete until this is resolved or explicitly re-scoped |
| 17A | Shared audio runtime and text-to-speech foundation: a provider-independent `Provider` abstraction, a real Windows SAPI implementation, a bounded audio queue consuming the same Event Bus (cooldowns, manual approval, per-source/per-currency/per-Bits filtering, text preprocessing), and a public OBS Browser Source audio route (see [audio-tts.md](audio-tts.md)) | **Completed** |
| 17B | Persistent alert sound assets, per-alert-rule TTS/sound, synchronization with alert playback, and any audio extension of the Stage 14B template-asset format (see [alert-audio.md](alert-audio.md)) | **Completed** — stage 17 as a whole is now complete |
| 18A | Persistent goals/counters foundation and core public OBS goal widgets (followers, subscriptions, donations, Bits), see [goals-widgets.md](goals-widgets.md) | **Completed** |
| 18B | Latest follower/subscriber/donation, largest donation, recent supporters, event ticker, richer session counters, and bounded multi-widget dashboards, see [supporter-widgets.md](supporter-widgets.md) | **Completed** — stage 18 as a whole is now complete |
| 19 | TikTok LIVE connector, **only if** an official, permitted, sufficiently stable integration exists | **Deferred** — feasibility-gated: no official TikTok LIVE engagement event API/scope exists, Embed Player is playback-only, and Desktop Login Kit's token exchange requires a confidential `client_secret` with no public-client alternative found (see [tiktok-live.md](provider-integrations/tiktok-live.md)). Stage 19 is **not** implemented until this is resolved or a future official integration is confirmed |
| 20A | Production runtime and Windows packaging foundation: the embedded production frontend, packaged-mode lifecycle (browser launch, single-instance detection, protected graceful shutdown, native fatal-startup-error dialog), release-injectable version metadata, and a per-user Inno Setup installer with the four legal documents included (see [windows-packaging.md](windows-packaging.md)) | **Completed** |
| 20B | Application update system: GitHub Releases check, update UI, download/verification, real Windows external-installer handoff (§12.1.1, see [updater.md](updater.md)); already uses the cross-platform artifact-identity concept defined in [platform-support.md](platform-support.md) §15, with Windows x64 as the only platform it currently serves | **Completed** |
| 20C1 | macOS packaged runtime: unsigned `.app`/DMG, real lifecycle adapters, native macOS CI package verification (see [macos-packaging.md](macos-packaging.md)) | **Completed** |
| 20C2 | macOS Developer ID signing, hardened runtime, notarization/stapling, updater install handoff, public/Beta readiness (see [macos-packaging.md](macos-packaging.md)) | Planned - externally gated on real Apple Developer credentials |
| 20D1 | Linux local/desktop runtime and packaging: a real `.deb` for the Debian/Ubuntu family, native x64/ARM64 CI package verification (see [linux-desktop-packaging.md](linux-desktop-packaging.md)) | **Completed** |
| 20D2A | Linux headless service foundation: loopback-only unattended systemd operation, secure encrypted headless secret storage (see [linux-headless-server.md](linux-headless-server.md)) | **Completed** |
| 20D2B | Secure remote management/control plane: single-administrator authentication, sessions, CSRF, TLS/reverse-proxy contract (see [remote-management.md](remote-management.md)) | **Completed** |
| 20D2C | Remote OBS ingest/data plane: MediaMTX-native authenticated/encrypted RTMPS ingest, a shared remote-overlay capability-token system across all five public overlay domains, and a native Linux CI harness proving the full contract through a genuinely isolated network namespace (see [remote-ingest.md](remote-ingest.md)) | **Completed** — stage 20D2 (and stage 20D) as a whole is now complete |
| 20E | Logs, diagnostics, and final release hardening/manual verification not covered by 20A-20D (see [final-hardening.md](final-hardening.md)) | **In progress** — diagnostics/logging backend, redaction, support bundle, frontend Logs UI, dependency/license/version/release-manifest audits, UX polish pass, native Windows package-verification CI gate, full automated regression (backend/frontend/24 canonical scripts), and a bounded resource-stability soak check are all complete; the single consolidated manual/physical verification gate (see [manual-verification.md](manual-verification.md)) is still pending. Stage 20 as a whole remains **Incomplete** regardless of 20E's own outcome, because 20C2 remains Planned — externally gated on real Apple Developer credentials |
| 21 | First-run onboarding + OBS setup experience (see [onboarding.md](onboarding.md)) | **Completed** — automated scope; no physical/manual pass performed yet |
| 22 | Reusable stream metadata presets, applied to one or several destinations at once (see [metadata-presets.md](metadata-presets.md)) | **Completed** |
| 23 | Safe configuration backup and restore: a full disaster-recovery/portability snapshot distinct from stage 22's own narrower metadata presets (see [backup-restore.md](backup-restore.md)) | **Completed** |
| 24 | Stream session / operational history: when a session ran and which destinations participated, with a coarse closed-category outcome - never chat, donation, or other engagement content (see [stream-session-history.md](stream-session-history.md)) | **Completed** |
| 25 | Stream setup profiles: a reusable local preparation of destinations and an optional metadata-preset reference for a particular kind of show, distinct from stage 23's full-snapshot backup and stage 22's own preset storage (see [stream-setup-profiles.md](stream-setup-profiles.md)) | **Completed** |
| 26 | Stream preflight and launch readiness: a single derived readiness check over the existing branch/FFmpeg/MediaMTX/metadata state before going live, surfaced on the Dashboard (see [stream-preflight.md](stream-preflight.md)) | **Completed** |
| 27 | Stream insights: aggregate stats (session count, total/average duration, longest session, per-destination outcome breakdown) computed on demand from stage 24's existing session-history store, with no new persisted data category (see [stream-session-history.md](stream-session-history.md) §14) | **Completed** |

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
- `node scripts/verify-ffmpeg-branches.mjs` - scripted verification that
  real, independent FFmpeg destination branches start, report real progress,
  isolate failures, survive ingest loss, respect an explicit stop, restart
  within their bounded policy, and that a backend restart resets their
  runtime state while their output settings persist - against a real FFmpeg
  executable and real MediaMTX instances, entirely on loopback.
- `node scripts/verify-twitch-account-integration.mjs` - scripted
  verification of Twitch account connection, linking, category search,
  metadata publishing, refresh-on-401 and disconnect/revocation, against
  the real backend and two small local fake Twitch servers - no real
  Twitch account or network request to Twitch is ever involved.

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

## 16. Engagement and overlay platform

Streaming Tree is also a **local streaming engagement and overlay
platform**, built on top of the router above: normalized chat/events
from multiple platforms, a unified operator chat, OBS Browser Source
overlays, outbound chat with scheduled bot messages and commands, an
alert engine and queue with visual overlay designers and a safe
portable template/package format, text-to-speech and persistent alert
audio, and goal/counter/supporter widgets. Every one of those pieces is
implemented and running today, over a single normalized Engagement
Event Bus that every consuming subsystem shares - never a
provider-specific or feature-specific parallel copy of chat, alerts, or
outbound sending.

The full architecture - the normalized event model, the connector
interface and capability model, deduplication and ordering rules, the
operator-chat vs. overlay distinction, bot automation, the alert and
queue design, the template security model, TTS, and per-provider
capability status - is documented separately in
[docs/engagement-architecture.md](engagement-architecture.md), so this
overview is not doubled in length by detail that has no bearing on
architecture. Exact per-provider/feature completion and deferral status
(e.g. Kick and TikTok engagement, additional donation providers) is
tracked in §13's roadmap table above, not repeated here.

One thing worth stating plainly, because it shapes decisions throughout
this whole platform: **the credential-store foundation (§10) is a hard
prerequisite for it.** A destination stream key, every connected
account's OAuth token bundle, and a donation source's own credential all
depend on the exact same `SecretStore` abstraction, distinguished only
by secret type - never a second, parallel secret mechanism for
engagement-platform credentials.

