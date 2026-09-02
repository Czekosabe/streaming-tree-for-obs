# Connecting OBS and streaming platforms

How to receive a stream from OBS, send it on to a destination, and
connect a real Twitch or YouTube account for metadata publishing.

---

## Local ingest with MediaMTX

Streaming Tree receives the stream from OBS through
[MediaMTX](https://github.com/bluenviron/mediamtx), which it runs as a child
process. MediaMTX is third-party software under the MIT licence — see
[`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md).

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
> [Stream key security](project-overview.md#10-stream-key-security) — and are read only when you
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
2. **A bundled location** beside the backend executable — a real,
   checked resolver step (`internal/runtime/ffmpeg/resolver.go`), kept
   intentionally unused: Windows, macOS, and Linux packaged builds
   (Stages 20A/20C1/20D1) all deliberately never place an FFmpeg binary
   there, since FFmpeg remains operator-provided on every packaged
   platform — so this step currently finds nothing on any of them, by
   design, not because the feature is unbuilt.
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
dialog, or through the API (see [REST API](development.md#rest-api)).

> ### The server URL is not the stream key
>
> The server URL is the address of the destination's RTMP ingest — the
> equivalent of OBS's "Server" field. The stream key is the separate secret
> that authorizes publishing to *your* channel on it, stored exactly as
> described in [Stream key security](project-overview.md#10-stream-key-security). Streaming Tree
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

See [REST API](development.md#rest-api) for the full endpoint list and response shapes.

### Verifying it for real

`scripts/verify-ffmpeg-branches.mjs` exercises this whole feature end to
end against a **real** FFmpeg executable and **real** MediaMTX instances —
a synthetic publisher, the real local ingest, a real branch process, and a
temporary destination MediaMTX standing in for the platform — entirely on
loopback, with no real platform account or credential. See
[Lint, typecheck, tests and other checks](development.md#lint-typecheck-tests-and-other-checks).

---

## Connected accounts and Twitch metadata

Streaming Tree can connect to a real **Twitch** account and use it to read
and explicitly publish that destination's channel metadata (title,
category, language, tags). This was the first of several provider
integrations (stage 7A of the roadmap); YouTube now has its own real
integration too — see
[Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
below (stage 7B). Kick account integration remains deferred/feasibility-
gated (stage 7C, alongside Kick's own stage 15B engagement research).
TikTok account integration is not pursued independently - it is folded
into stage 19's own feasibility gate, which found no official TikTok
LIVE engagement capability for such an account to power, see
[`docs/provider-integrations/tiktok-live.md`](provider-integrations/tiktok-live.md).

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
consumes it, stage 10 added a real, public **OBS Browser Source chat
overlay** that in turn consumes that same operator-chat projection, and
stage 11A added **manual outbound chat sending** — a third, independent
capability profile letting the same connected account send and reply to
real Twitch chat messages — stage 11B added real **scheduled
messages and safe chat commands** on top of that same profile and
dispatcher, and stage 12A added a real **alert engine** consuming the
same Event Bus (persisted alert rules, a matcher, a bounded queue, and a
public Browser Source alert route), which stage 12B then closed out with
real bounded alert grouping and mid-alert preemption — see
[Engagement Event Bus and Twitch chat/events](engagement-architecture.md),
[Unified operator chat](engagement-architecture.md),
[OBS Browser Source chat overlay](obs-browser-source.md),
[Sending Twitch chat manually](provider-integrations/twitch-outbound-chat.md),
[Scheduled messages and chat commands](engagement-architecture.md)
and [Alerts](engagement-architecture.md).
Since then, the visual alert/overlay designer (stage 13), donations
from external services (stage 16A), a shared audio/text-to-speech
runtime (stage 17A), persistent alert audio/per-rule TTS (stage 17B),
and a persistent goals/counters foundation with the full supporter/
activity widget suite (stages 18A/18B) have all shipped as well — see
[`docs/engagement-architecture.md`](engagement-architecture.md).
What remains planned: live viewer counts and broader stream analytics
(not part of any stage scoped so far).

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
[`docs/provider-integrations/twitch.md`](provider-integrations/twitch.md)
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
see [`docs/progress.md`](progress.md) for exactly what was and was not
verified against a real account.

---

## Connected accounts and YouTube metadata

Streaming Tree can connect to a real **YouTube channel** and use it to read
and explicitly publish a selected live broadcast's video metadata (title,
description, category, tags, language, visibility). This is stage 7B of
the roadmap, reusing the same connected-account foundation stage 7A built
for Twitch, adapted for how Google's own OAuth and the YouTube APIs
actually work — see
[`docs/provider-integrations/youtube.md`](provider-integrations/youtube.md)
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

**What this stage does not implement.** Stage 7B (this section) is the
account, broadcast-selection, and metadata foundation only - it does not
itself implement YouTube live-chat ingestion, Super Chat, Super Sticker,
or membership events. That inbound engagement connector was built later,
on top of this foundation, as stage 15A - see
[Engagement Event Bus and YouTube chat/events](engagement-architecture.md).
Twitch was the first provider to feed chat and events onto the
Engagement Event Bus (stage 8A); YouTube is the second (stage 15A) -
both now serve the exact same downstream pipeline: the
provider-independent operator **Chat** page (stage 9) - see
[Unified operator chat](engagement-architecture.md) - the public **OBS
Browser Source chat overlay** built on top of that same chat (stage 10)
- see
[OBS Browser Source chat overlay](obs-browser-source.md) below
and [`docs/obs-browser-source.md`](obs-browser-source.md) for the
OBS-specific research it is built on - outbound manual chat sending
(stage 11A/15A) and real alerts (stage 12A, with Super Chat/Super Sticker
money support added in 15A). Text-to-speech, donation connectors,
automatic broadcast creation, automatic `liveStream` binding, and
automatic stream-key retrieval from YouTube remain unimplemented - see
[`docs/engagement-architecture.md`](engagement-architecture.md).

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
[`docs/provider-integrations/youtube.md`](provider-integrations/youtube.md)
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
see [`docs/progress.md`](progress.md) for exactly what was and was
not verified.

---
