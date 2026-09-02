# Twitch outbound chat integration — researched contract

**Research date:** 2026-08-07.

This is the Stage 11A counterpart to [`twitch.md`](twitch.md) (OAuth and
channel metadata) and [`twitch-engagement.md`](twitch-engagement.md)
(EventSub inbound chat/events). Those two documents cover reading; this one
covers **writing** - sending a chat message as the connected account through
Twitch's own Helix API. Kept as its own document for the same reason the
other two are split: a different Twitch subsystem, researched at a different
time, so a future API change to one never forces a re-read of the others.

As with the other two documents, this is a paraphrased summary of official
documentation, not a copy of it. Field names and constants are used because
the code needs the exact identifier, not because the surrounding prose is
quoted at length.

## Official pages inspected

- <https://dev.twitch.tv/docs/chat/> — chat overview, EventSub-for-reading +
  Helix-for-sending positioning.
- <https://dev.twitch.tv/docs/chat/authenticating/> — scope requirements for
  reading (`user:read:chat`) versus writing (`user:write:chat`) chat.
- <https://dev.twitch.tv/docs/chat/send-receive-messages/> — the
  request/response shape for `POST /helix/chat/messages`, including
  `message_id`, `is_sent`, and a `drop_reason` example
  (`{"code":"automod_held","message":"..."}`).
- <https://dev.twitch.tv/docs/chat/irc-migration/> — explicit migration
  guidance: "it is recommended to upgrade your chatbots that are using
  Twitch IRC to use EventSub (for reading chat messages and roomstates) and
  Twitch API (for sending chat messages)," and the exact sentence "the Send
  Chat Message API requires at a minimum the **user:write:chat** [scope]
  from the chatting user" (an app access token additionally needs
  `user:bot`, which this stage deliberately never requests - see "Selected
  token type and scope" below).
- <https://dev.twitch.tv/docs/api/reference/#send-chat-message> — the
  endpoint's own request/response reference: method, URL, request
  query/body parameters, response body, and response codes.
- <https://dev.twitch.tv/docs/authentication/scopes/> — the canonical scope
  list, confirming `user:write:chat` = "Send chat messages to a chatroom,"
  distinct from the IRC-only `chat:edit` = "Send chat messages to a chatroom
  **using an IRC connection**."
- <https://dev.twitch.tv/docs/api/guide> — Helix rate-limit header shape
  (`Ratelimit-Limit`/`Ratelimit-Remaining`/`Ratelimit-Reset`, a token-bucket
  per Client ID + user, refilling within a rolling one-minute window, `429`
  on exhaustion) - identical to what `twitch.md` already documented for
  every other Helix endpoint this application calls.
- <https://dev.twitch.tv/docs/api/reference/#get-shared-chat-session> —
  confirms Shared Chat is a real, current Twitch concept with its own
  dedicated read endpoint.

## Why EventSub + Twitch API instead of IRC

Twitch's own IRC-migration guide states the recommendation directly: use
EventSub for reading chat/roomstate and the Twitch (Helix) API for sending,
migrating away from IRC. This application already made the EventSub choice
for reading in Stage 8A (see `twitch-engagement.md`); Stage 11A completes
the pair by using the Helix `POST /helix/chat/messages` endpoint for
sending, rather than opening a second, IRC-based connection this
application would otherwise have no other reason to hold. Concretely, IRC
would also mean a second credential/connection model, a different message
framing entirely outside this application's existing SSE/HTTP-shaped
architecture, and no `is_sent`/`drop_reason` structured outcome at all - IRC
sends are fire-and-forget with no synchronous delivery confirmation, which
would make this stage's "never claim success without `is_sent: true`"
requirement impossible to honor honestly.

## Selected token type and scope

**User Access Token**, requesting the **`user:write:chat`** scope
(confirmed via three independent official pages: the scopes reference, the
chat/authenticating page, and an exact quoted sentence on the
irc-migration page - see "Official pages inspected" above).

Deliberately **not** requested: `user:bot` or `channel:bot`. Twitch's
irc-migration page states an app access token needs `user:bot` in addition
to `user:write:chat` to send **on behalf of** a user as a separate bot
identity; this application sends **as the connected user themselves**
through a **User Access Token**, which needs no bot-identity scope at all.
Requesting `user:bot`/`channel:bot` would signal "this app impersonates a
bot identity in your channel," which is not what Stage 11A does and not
what this application's product design wants (see the stage task's own
explicit exclusion list).

### A research anomaly, recorded for transparency

One fetch pass of the Send Chat Message reference section returned
`user:manage:chat` instead of `user:write:chat` as the required scope. This
could not be corroborated: `user:manage:chat` does not appear anywhere in
the canonical scopes reference page's full "contains chat" listing (which
does include `user:write:chat`, `user:read:chat`, `user:bot`, `channel:bot`,
and several `moderator:*`/`user:manage:chat_color` scopes - `user:manage:chat`
is absent), and three independently-fetched pages (scopes reference,
chat/authenticating, irc-migration - the last with an exact quoted sentence)
all agree on `user:write:chat`. This is recorded here rather than silently
discarded, consistent with this document's own standing rule to verify
against current documentation rather than trust a single source - if a
future re-check finds `user:manage:chat` genuinely is now correct, treat
that as a real Twitch API change, not a repeat of this anomaly.

## Endpoint and request/response contract

**`POST https://api.twitch.tv/helix/chat/messages`**

Headers: `Authorization: Bearer <user access token>`, `Client-Id: <configured
client id>`, `Content-Type: application/json`.

Request body:

| Field | Required | Notes |
| --- | --- | --- |
| `broadcaster_id` | Yes | The channel to send to. |
| `sender_id` | Yes | "Must match the user ID in the user access token." |
| `message` | Yes | "May contain a maximum of **500 characters**." |
| `reply_parent_message_id` | No | The message ID being replied to. |

This application always sets `broadcaster_id` and `sender_id` to the same
value: the connected account's own `ProviderUserID` - this stage sends only
to the connected broadcaster's own channel, as themselves, never to an
arbitrary channel or as an arbitrary sender chosen by the browser (see the
stage task's own security requirements).

Response body: `data` is a one-element array; each element carries
`message_id` (string) and `is_sent` (boolean), plus `drop_reason` (an
object with `code` and a human-readable `message`) when `is_sent` is
`false`. The reference page's own worked example returns exactly
`{"data":[{"message_id":"abc-123-def","is_sent":true}]}` for a successful
send - `drop_reason` is absent entirely on success, not present-and-empty.

**Maximum message length: 500 Unicode characters**, enforced by Twitch
(`400 Bad Request` above the limit) and mirrored by this application's own
backend validation so a too-long message never reaches Twitch at all.

## Reply behavior

`reply_parent_message_id` is optional, forwarded verbatim, and Twitch's own
documentation gives no additional constraint on it beyond "the ID of the
message to reply to." This application only ever populates it with a
message ID it already holds because the operator selected an existing
Twitch-sourced message from their own connected account's chat (see the
stage task's own reply-selection requirements) - never a value typed
freely into a text field.

## `is_sent` and `drop_reason` behavior

A `200 OK` HTTP response is **not**, by itself, proof the message reached
chat. The response body's own `is_sent` boolean is the actual outcome:
`is_sent: true` is a real send; `is_sent: false` means Twitch's own chat
backend (not the Helix API layer) rejected or dropped the message - the
worked drop example returned by the send-receive-messages guide is
AutoMod holding a message for review (`drop_reason.code: "automod_held"`).
This application treats `is_sent: false` as a **stable, non-retryable
"dropped" outcome**, never as a successful send, and never automatically
retries it (see "Uncertain-outcome and retry policy" below - AutoMod
holding a message is exactly as final, from this stage's perspective, as
any other stable drop reason; there is no product requirement in Stage
11A to poll for a later AutoMod resolution). `drop_reason.message` (the
human-readable prose) is parsed only to confirm the shape is well-formed;
its text is never persisted, returned to the frontend, or logged - only
`drop_reason.code`, a stable machine identifier, ever leaves the parsing
layer.

## HTTP error mapping

From the endpoint's own documented response codes:

| Status | Documented cause(s) |
| --- | --- |
| `400` | Missing `broadcaster_id`/`sender_id`/`message`, or `message` exceeds the length limit. |
| `401` | `sender_id` doesn't match the token's user, missing/invalid `Authorization`, token lacks `user:write:chat`, invalid token, or Client ID mismatch. |
| `403` | The user is banned/timed out in that chat room, or otherwise lacks permission to send. |
| `404` | The broadcaster (channel) was not found. |
| `420` | "Enhance Your Calm" - the user is sending messages too quickly (Twitch's **chat-backend** rate limit, distinct from the standard Helix `429`; see "Twitch chat-backend rate limits" below). |

This is mapped to this application's own stable outbound-chat error codes -
see the Stage 11A HTTP API for the exact table - never Twitch's own status
text or prose forwarded verbatim.

## API rate-limit headers

Standard Helix token-bucket headers apply, identical to every other Helix
endpoint `twitch.md` already documents: `Ratelimit-Limit`,
`Ratelimit-Remaining`, `Ratelimit-Reset` (Unix-epoch reset time), refilling
within a rolling one-minute window, `429` on exhaustion. The general API
guide states no Send-Chat-Message-specific override to this shape, so this
application parses these headers the same tolerant way it already parses
them for metadata/EventSub-subscription calls.

## Twitch chat-backend rate limits

Separate from the Helix-wide `429`, Twitch's chat backend enforces its own
per-user "sending too fast" limit, surfaced as HTTP **`420`** ("Enhance
Your Calm") on the Send Chat Message endpoint specifically - not a status
code this application's HTTP client encounters anywhere else in this
codebase. Twitch does not publish an exact numeric per-second/per-minute
figure for this limit in the pages inspected; this application therefore
does not attempt to predict it and instead relies on (a) its own
conservative local rate limiting as a safety ceiling (see the stage task's
dispatcher requirements) and (b) treating any received `420` exactly like a
`429` - a stable, sanitized, non-retried rate-limited outcome exposing a
retry hint where Twitch's response provides one, never blindly retried by
this application itself.

## Shared Chat behavior

A **Shared Chat session** lets several broadcasters' channels share one
merged chat. Twitch documents a dedicated read endpoint for it (`Get Shared
Chat Session`), confirming the feature is real and current. This stage's
own product requirement (the stage task's own asserted premise, which this
research did not find grounds to contradict) is that a message sent with a
**User Access Token** may be distributed to every channel participating in
a Shared Chat session the target channel is part of, and that
`for_source_only` - a field intended to restrict a send to the originating
channel only - is documented elsewhere as usable only with certain token
types, not confirmed as available to a plain User Access Token send in the
material this research pass could reliably access (see "A note on
`for_source_only`" immediately below). Because this application has **no
reliable way to detect whether Shared Chat is currently active** for a
given channel in this stage, and no requirement to build that detection,
the honest choice is disclosure, not a false claim of restriction: the
composer shows a persistent warning that a send may be distributed across
a Shared Chat session and that this application cannot restrict it, without
asserting Shared Chat is presently active (see the stage task's own
`for_source_only` and Shared Chat requirements) - and this application never
sends `for_source_only` under any circumstance.

### A note on `for_source_only`

The single most reliable verbatim extraction of the Send Chat Message
Request Body table this research pass obtained lists exactly two optional
fields alongside the required `message`: `reply_parent_message_id` and
nothing else - `for_source_only` did not appear in that table. Other fetch
passes of the same (very large) reference page returned inconsistent or
truncated results for this specific field, most likely a limitation of
paginated/summarized fetching against a large single-page reference rather
than a confirmed absence of the field from Twitch's real documentation.
This is recorded honestly as an open point rather than asserted either way:
**this application's implementation never sends `for_source_only` in any
request, regardless of which reading is eventually confirmed correct** -
omitting it is safe whether the field exists (Twitch's own documented
default behavior for a User Access Token send applies) or does not
(there is nothing to omit). Re-verify this specific field directly against
<https://dev.twitch.tv/docs/api/reference/#send-chat-message> before ever
introducing `for_source_only` support in a future stage.

## Uncertain-outcome and retry policy

Chat sends are **not safely idempotent** - retrying a send that may have
already reached Twitch risks a real, visible duplicate message in the
broadcaster's own chat. This application's policy:

- **Never automatically retried:** any `5xx`, a transport-level failure
  (connection reset, DNS failure), a timeout, a malformed/non-Twitch-shaped
  "success" response, `403`, `422`, `429`, `420`, or `is_sent: false`. Each
  becomes a stable, honest outcome - "dropped" for `is_sent: false`,
  "rate limited" for `429`/`420`, "provider failure" for `5xx`/transport
  errors, and an explicit **"delivery outcome unknown"** result specifically
  for the case where the request may have reached Twitch's servers but no
  trustworthy response was ever received (a timeout or a connection reset
  mid-response) - never silently treated as either a success or a clean
  failure.
- **Exactly one exception:** a clearly-received `401` may trigger the
  existing single-flight token-refresh path this application already uses
  for every other Twitch call, followed by exactly one retried send - safe
  specifically because a `401` is Twitch explicitly rejecting the *first*
  attempt outright (never processed as a chat message), not an uncertain
  outcome. A second `401` after the refreshed retry stops immediately and
  surfaces `reconnect_required`-shaped failure, exactly like every other
  Twitch adapter call in this application already does.

## Stage 11B: scheduled messages and chat commands reuse this same contract

Stage 11B (scheduled bot messages, safe chat commands, placeholders,
cooldowns) added **no new Twitch scope, no new endpoint, and no second
outbound pipeline**. Every automated send - scheduled or
command-triggered - is rendered locally (the closed placeholder
language in `internal/chatautomation/placeholders.go`) and then handed
to the exact same `internal/outboundchat` dispatcher and Send Chat
Message adapter this document already describes; `internal/chatautomation`
never calls the Twitch client directly. Concretely, that means every
constraint recorded above still applies unchanged to an automated send:
the same `user:write:chat` scope (still no `user:bot`/`channel:bot`),
the same one-send-in-flight-per-account queue and local rate limiter,
the same no-automatic-retry-on-uncertain-outcome policy, and the same
Shared Chat disclosure. Stage 11B's own scheduler and command-cooldown
limits (interval, jitter, activity gating, hourly cap, per-user/global
cooldowns) sit **above** this contract as an automation-behavior
control, not a replacement for it - the dispatcher's local rate limiter
and Twitch's own `429` response remain the authoritative safety ceiling
either way.

## Exact non-goals for this document/stage

- No IRC connection of any kind.
- No app access token, no `client_secret`, no `user:bot`/`channel:bot`.
- No `for_source_only` in any outgoing request (see above).
- No pinning (`pin`), no announcements, no whispers, no moderation actions -
  none of these are part of the Send Chat Message contract this document
  covers, and none is implemented in Stage 11A.
- No attempt to detect or predict Twitch's exact chat-backend rate-limit
  numbers - Twitch does not publish them for this endpoint in the pages
  inspected, and this application does not guess.
- No scheduled/automatic sending, no command engine, no placeholder
  substitution - this document covers the **transport contract** only; the
  product boundary around what triggers a send is Stage 11A/11B's own
  concern, described in `docs/progress.md`, not this contract document.

## Requirement to re-check

Exactly like `twitch.md` and `twitch-engagement.md` already state for their
own contracts: Twitch's Send Chat Message endpoint, its required scope, its
response shape, its error codes, its rate-limit behavior, and its Shared
Chat interaction are Twitch's to change without this application's
involvement. Any change to any of these invalidates part of this document
and must be re-verified against the official pages listed above before
being relied on again - especially the `for_source_only`/Shared Chat point
flagged above as not fully resolved by this research pass.

---

This document records the contract this implementation was built against on
the research date above. Twitch's API and policies can change at any time;
before relying on any detail here for new work, re-check it against the
current official documentation linked above.
