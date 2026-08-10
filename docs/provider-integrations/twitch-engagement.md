# Twitch engagement integration — researched contract

**Research date:** 2026-08-05.

This document is the Stage 8A counterpart to
[`twitch.md`](twitch.md): that document covers Twitch's OAuth and channel
**metadata** contract (stage 7A); this one covers Twitch's **EventSub
WebSocket** contract for reading chat and channel events (stage 8A). The two
are deliberately kept separate rather than merged, because they cover
different Twitch subsystems researched at different times, and a future
Twitch API change to one should not force a re-read of the other.

As with `twitch.md`, this is a paraphrased summary of official documentation,
not a copy of it. Field names and constants below are used because the code
needs the exact identifier, not because the surrounding prose is quoted.

## Official pages inspected

- <https://dev.twitch.tv/docs/eventsub/handling-websocket-events/> — WebSocket
  lifecycle: connection URL, welcome timeout, keepalive, reconnect, close
  codes, limits, message envelope.
- <https://dev.twitch.tv/docs/eventsub/manage-subscriptions/> — creating a
  WebSocket-transport subscription, authorization requirements, cost/limits.
- <https://dev.twitch.tv/docs/eventsub/eventsub-subscription-types/> — exact
  type strings, versions, condition fields and scopes for the subscription
  types this stage selected.
- <https://dev.twitch.tv/docs/api/reference/#create-eventsub-subscription> —
  the Helix subscription-creation endpoint.
- <https://dev.twitch.tv/docs/chat/send-receive-messages/> — chat message
  fragment shape (`channel.chat.message`'s payload).
- <https://dev.twitch.tv/docs/authentication/scopes/> — exact scope names and
  descriptions.
- <https://dev.twitch.tv/docs/authentication/validate-tokens/> — token
  validation cadence and the `/oauth2/validate` response shape (confirms the
  hourly requirement `internal/domain/account`'s existing validation worker,
  built in stage 7A, already implements — no change needed for stage 8A).

## WebSocket lifecycle

- **Production URL:** `wss://eventsub.wss.twitch.tv/ws`. Not overridable in
  the production binary — only the `-tags integration` test binary accepts an
  override, via `STREAMING_TREE_TEST_TWITCH_EVENTSUB_BASE_URL`, read directly
  via `os.Getenv` in `cmd/testserver/main.go`, exactly like the existing
  Twitch OAuth/Helix test overrides from stage 7A.
- **Welcome window:** after connecting, the client has **10 seconds** from
  receiving `session_welcome` to create at least one subscription, or Twitch
  closes the connection with close code `4003` ("connection unused").
- **Keepalive:** Twitch sends `session_keepalive` messages whenever no
  notification has arrived within the negotiated `keepalive_timeout_seconds`
  window (query parameter on connect, default range 10–600s; this
  application does not override it and uses Twitch's default). The keepalive
  timer resets on **either** a notification or a keepalive message — a
  connector must therefore track "time since last message of either kind,"
  not "time since last real event."
- **Standard WebSocket ping/pong** is independent of the keepalive timer and
  is handled by the WebSocket client library, not application code (see
  "WebSocket client library" below).
- **Ordinary disconnection:** Twitch does **not** replay events lost between
  disconnect and reconnect, and every subscription on that connection is
  disabled — a new session must recreate every subscription from scratch.
  This is why the connector marks a **possible data gap** on ordinary
  reconnect (never a false claim of seamless recovery) and why an official
  `session_reconnect` (below) is handled completely differently.
- **`session_reconnect`:** carries a `reconnect_url`. The client must connect
  to it **exactly as given** (no query-parameter changes), keep the *old*
  connection open, wait for `session_welcome` on the *new* connection, and
  only then close the old one. Twitch carries every existing subscription
  forward automatically — the connector must **not** recreate subscriptions
  after this flow, and must **not** mark a data gap, because none occurred.
  Twitch closes with code `4004` if the old connection is not closed in time
  or the new one is never established.
- **Revocation:** a `revocation` message means Twitch stopped delivering a
  specific subscription, for one of: `authorization_revoked`, `user_removed`,
  or `version_removed`. Twitch sends it once per affected subscription, then
  simply stops sending anything for it — there is no further signal.
- **Close codes surfaced to the connector's error state (sanitized, no raw
  frame stored):** `4000` internal server error, `4001` client sent
  disallowed inbound traffic, `4002` client failed ping/pong, `4003`
  connection unused, `4004` reconnect grace period expired, `4005`–`4007`
  network timeout / internal error / invalid reconnect URL.

## Connection and subscription limits (per user token)

- Up to **3** concurrent WebSocket connections with enabled subscriptions per
  user token + Client ID pair (a reconnect via `session_reconnect` does not
  count as an extra connection against this limit).
- Up to **300** enabled subscriptions per connection — far above the 13
  subscriptions stage 8A's one connector creates per account.
- A **total subscription cost ceiling of 10** per user token, summed across
  every enabled subscription's individual cost. Every subscription type this
  stage selected has a documented cost of 0 or 1, so 13 subscriptions well
  under the 10-cost ceiling was confirmed before finalizing the selected set
  (see "Selected subscription types" below — several chat-related types
  share one broadcaster+moderator scope pair and are cost-cheap).

Because Streaming Tree runs **one connector per enabled connected Twitch
account**, and each account has its own token, these limits apply
independently per account — one account's subscription count never affects
another's ceiling.

## Creating a subscription

`POST https://api.twitch.tv/helix/eventsub/subscriptions`, with
`Authorization: Bearer <user access token>`, `Client-Id: <client id>`, JSON
body `{type, version, condition, transport: {method: "websocket",
session_id}}`. **A user access token is required for WebSocket transport —
an app access token is rejected.** This is why stage 8A's connector reuses
the *same* connected-account token bundle stage 7A already stores, rather
than requesting a separate app token. `session_id` comes from the
`session_welcome` message on the connection the subscription should attach
to.

## Selected subscription types

| Type | Version | Condition | Scope |
| --- | --- | --- | --- |
| `channel.chat.message` | `1` | `broadcaster_user_id`, `user_id` (the connected account's own user ID — Streaming Tree is not a bot reading on someone else's behalf) | `user:read:chat` |
| `channel.chat.message_delete` | `1` | `broadcaster_user_id`, `user_id` | `user:read:chat` |
| `channel.chat.clear` | `1` | `broadcaster_user_id`, `user_id` | `user:read:chat` |
| `channel.chat.clear_user_messages` | `1` | `broadcaster_user_id`, `user_id` | `user:read:chat` |
| `channel.follow` | `2` | `broadcaster_user_id`, `moderator_user_id` | `moderator:read:followers` |
| `channel.subscribe` | `1` | `broadcaster_user_id` | `channel:read:subscriptions` |
| `channel.subscription.gift` | `1` | `broadcaster_user_id` | `channel:read:subscriptions` |
| `channel.subscription.message` | `1` | `broadcaster_user_id` | `channel:read:subscriptions` |
| `channel.cheer` | `1` | `broadcaster_user_id` | `bits:read` |
| `channel.raid` | `1` | `to_broadcaster_user_id` (incoming raids only — `from_broadcaster_user_id` is the outgoing direction and is deliberately not subscribed) | none |
| `channel.channel_points_custom_reward_redemption.add` | `1` | `broadcaster_user_id` | `channel:read:redemptions` |
| `stream.online` | `1` | `broadcaster_user_id` | none |
| `stream.offline` | `1` | `broadcaster_user_id` | none |

`channel.follow`'s `moderator_user_id` condition is the account's own user
ID — `moderator:read:followers` is documented as requiring the token's user
to be a moderator (or the broadcaster) of the channel queried, which the
connected account always is for its own channel.

**Version note:** `channel.follow` version 1 exists in Twitch's history but
is deprecated in favor of version 2's explicit `moderator:read:followers`
requirement; only version 2 is used here, matching this document's
requirement to record exact current versions, not remembered ones.

## Scopes

The full "Stage 8 inbound-engagement" scope profile is the union of every
scope above:

```
user:read:chat
moderator:read:followers
channel:read:subscriptions
bits:read
channel:read:redemptions
```

This is a **second, additive scope profile**, separate from stage 7A's
"metadata" profile (`channel:manage:broadcast`) — see
[Part 3's scope-profile design](#scope-profile-design-decision) below. Every
scope above was checked individually against
<https://dev.twitch.tv/docs/authentication/scopes/> — none is bundled with a
broader "read everything" scope, and none of the explicitly excluded scopes
(`user:write:chat`, any moderation-write scope, email, whispers, ads,
analytics, raid-management, or any channel-management scope beyond
`channel:manage:broadcast`) appears anywhere in this list.

## Chat message shape (`channel.chat.message`)

The notification payload carries `message.text` (the complete plain text) and
`message.fragments`, an ordered array where each fragment has a `type`
(`text`, `cheermote`, `emote`, or `mention`) plus type-specific fields
(emote ID/set/owner for `emote`, a Bits prefix/amount for `cheermote`, a
mentioned user's ID/login/name for `mention`). Also present:
`chatter_user_id`/`_login`/`_name`, `color` (hex, may be empty), `badges`
(array of `{set_id, id, info}`), `message_id`, `message_type`, an optional
`reply` object when the message replies to another, and an optional
`channel_points_custom_reward_id` when the message accompanied a reward
redemption. The normalized `engagement.Message` model's fragment types map
directly onto these four, plus an `unknown` fallback for any fragment `type`
Twitch adds in the future that this application does not yet recognize —
never a hard parse failure.

## Duplicate delivery and reconnect

Twitch's own documentation does not describe an explicit duplicate-delivery
guarantee or a required deduplication key the way some other providers'
webhook systems do. Rather than assume delivery is always exactly-once,
Streaming Tree treats `metadata.message_id` (present on every WebSocket
message, including notifications) as a bounded, TTL'd deduplication key —
cheap insurance against a redelivered notification being counted twice,
costing nothing when no redelivery ever happens. See the Event Bus's own
dedup design in `docs/progress.md`'s stage 8A entries for the exact bound
and TTL chosen.

## Known unavoidable data-gap behavior

Because Twitch does not replay events across an ordinary disconnect, any
gap between a connection dying and the reconnected session's subscriptions
being live again is a genuine, permanent gap in what Streaming Tree observed
— not a bug to work around, since there is no API to ask Twitch "what did I
miss." The connector exposes `lastDataGapAt` for exactly this reason: an
honest signal, not a promise of completeness. The **only** case with no gap
is a successful `session_reconnect` handoff, because Twitch itself guarantees
continuity there.

## Areas reserved for later stages

- **Stage 9 (unified operator chat):** consuming the normalized events this
  stage's bus produces in an actual chat UI. Stage 8A's own diagnostic event
  view is explicitly not this.
- **Stage 11A (manual outbound chat, now implemented):** `user:write:chat`
  and sending/replying via the real Send Chat Message API. Not requested
  or used anywhere in this stage 8A connector - it is a third,
  independently assessed capability profile on the same connected
  account, researched and implemented separately. See
  [`twitch-outbound-chat.md`](twitch-outbound-chat.md) for the full
  contract, and the addendum at the end of this document for how the two
  stay independent.
- **Stage 11B (scheduled bot messages, chat commands, now implemented):**
  built on stage 11A's own dispatcher, not this inbound connector.
  This document's own connector is **unchanged** by stage 11B: the
  command engine (`internal/chatautomation`) subscribes to the
  already-normalized Engagement Event Bus this stage produces - the
  same bus stage 9's operator chat and stage 10's overlay already
  consume - never to this connector's WebSocket directly, and never a
  second EventSub connection. See the README's own
  [Scheduled messages and chat commands](../../README.md#scheduled-messages-and-chat-commands)
  section for the full design.
- **Stage 12 (alerts):** rule matching against these events. Not implemented.
- **Badge image resolution, per-message avatar fetching:** stage 8A carries
  badge/emote **IDs** in the normalized model but does not resolve them to
  image URLs or fetch a chatter's avatar per message (avoiding one profile
  request per chat message, as the stage task explicitly required). A future
  stage may add a small, bounded image-catalog cache.
- **`channel.chat.notification`:** deliberately **omitted** in stage 8A.
  Twitch overlays subscription/gift/raid/announcement notices onto this one
  event type with its own `notice_type` field, which would require either
  duplicating semantic events already covered by the dedicated subscriptions
  above (with a *different* provider event ID, defeating deduplication) or a
  carefully-designed non-duplicating `chat.notice` mapping this stage's scope
  does not include. Reserved for a later stage if a real product need for it
  emerges.
- **Newer/beta event types:** Twitch periodically adds new subscription
  types (for example, newer Bits-related events). None is subscribed to here
  merely because it exists; each would need its own scope/version/product
  review before being added, exactly like the excluded list in
  [Part 8 of the stage task](../progress.md).

## Scope-profile design decision

`internal/domain/account`'s `RequiredScopes` map (one fixed scope list per
provider, enforced on every validation pass) is deliberately **not**
widened to include the engagement scopes above. Doing so would make every
existing Twitch account's core health depend on scopes most accounts have
never been asked to grant, directly contradicting the stage task's
requirement that "an account may be healthy for metadata while lacking
engagement scopes." Instead:

- `RequiredScopes[ProviderTwitch]` stays exactly `channel:manage:broadcast`
  (the metadata profile) — unchanged from stage 7A.
- A new, separate **capability assessment** (not wired into
  `account.Service`'s own health/reconnect logic at all) compares an
  account's currently-granted scopes against the engagement profile above,
  independently of metadata health.
- Requesting the engagement profile for an *existing* account reuses the
  Device Code Flow (`internal/runtime/deviceflow`), extended to accept a
  **per-attempt scope override** rather than only the Manager's one
  construction-time scope list — the union of the account's currently
  granted scopes and the engagement profile, so a successful upgrade never
  narrows what the account can already do, and identity-bound so it always
  targets the same underlying Twitch account rather than risking a second,
  competing connection.

## Requirement to re-check

Twitch's EventSub documentation, subscription type versions and scope model
are Twitch's to change without this application's involvement. Any change to
a subscription's version, condition fields, or required scope, any new close
code, any change to the keepalive/reconnect contract, or any change to the
per-token connection/subscription/cost limits invalidates part of this
document and must be re-verified against the official pages listed above
before being relied on again — exactly the same standing rule
`twitch.md` and `youtube.md` already state for their own contracts.

---

## Stage 9 addendum — chat badge and emote presentation

**Research date:** 2026-08-06.

Stage 9 (unified operator chat) needs to render badges and emotes safely,
without a provider request per message. This addendum records what was
researched for that, kept separate from the EventSub contract above since
it is a different Helix subsystem (Chat, not EventSub).

### Sources inspected

- <https://dev.twitch.tv/docs/api/reference/#get-global-chat-badges>
- <https://dev.twitch.tv/docs/api/reference/#get-channel-chat-badges>
- <https://dev.twitch.tv/docs/api/reference/#get-channel-emotes>
- <https://dev.twitch.tv/docs/irc/emotes/>

### Badge-resolution strategy

`GET /helix/chat/badges/global` and `GET /helix/chat/badges?broadcaster_id=`
share one response shape: a list of `{set_id, versions: [{id,
image_url_1x/2x/4x, title, description, click_action, click_url}]}`
entries. Both accept an app or a user access token; the documentation pages
inspected state no scope requirement for either. A chat badge referenced by
an event (`set_id` + `version` from the normalized `Badge`) is resolved by
checking the **channel-specific catalog first, then falling back to the
global catalog** for the same `set_id`/version pair - two parallel-shaped
catalogs with identical keys are otherwise pointless to fetch separately.
**This exact override order is this implementation's own defensible
inference from the API shape, not a sentence quoted from an official page**
- the pages inspected describe the two endpoints independently without
spelling out a merge/override rule in the sections this research could
access, so it is flagged here explicitly for re-verification rather than
presented as confirmed. `subscriber`-set badges are channel-scoped in
practice (every channel has its own subscriber-tier badge images), so the
channel-first order is also the only one that can ever resolve them at all.

### Emote-resolution strategy — no catalog fetch at all

This is the more significant finding: Twitch's own IRC/EventSub emote guide
documents a **fixed CDN URL template** returned alongside the Get Channel/
Global/Set Emotes responses:

```
https://static-cdn.jtvnw.net/emoticons/v2/{{id}}/{{format}}/{{theme_mode}}/{{scale}}
```

`{{id}}` is exactly the emote ID every EventSub chat fragment already
carries (`internal/domain/engagement.Fragment.EmoteID`, already normalized
in stage 8A). Building this URL therefore needs **no Get Channel/Global
Emotes catalog request at all** - not even a cached one - because every
input the template needs is already present on the fragment itself.
`{{format}}` is fixed to `static` (Twitch's own documentation states the
non-template image in an emote payload "is always a static image," meaning
`static` is guaranteed available for every emote, animated or not - a safe,
universal default this application does not need per-emote metadata to
choose). `{{theme_mode}}` is fixed to `dark` (matching this application's
own dark UI, and a value Twitch always accepts regardless of the viewer's
own theme). `{{scale}}` is fixed to `2.0` (a bounded, reasonable rendering
size - see "Presentation details" below).

This means the emote half of Part 11's asset resolver is, deliberately, not
a cache at all: it is a pure, request-free URL-construction function over an
already-validated emote ID, with no network call, no TTL, and nothing that
can go stale.

### Cache behavior (badges only)

Bounded in-memory cache keyed by `"global"` or a broadcaster's provider
user ID, each entry the parsed badge catalog for that scope. TTL: **1
hour** (badge catalogs change rarely - a new subscriber-tier badge or a
Twitch-wide addition is not time-sensitive the way a chat message is), a
generous but bounded max-entry count, single-flight refresh per cache key
(mirroring `internal/domain/account.Service`'s own hand-rolled
per-key-mutex-plus-in-flight-map pattern - see the main Stage 9 progress
entry), strict response-size and timeout limits identical to every other
Helix call this application makes.

### Fallback behavior

A badge that cannot be resolved (cache miss during a cold single-flight
fetch, a fetch failure, an unknown `set_id`/version) is simply omitted from
that user's rendered badge list - the chat message itself is never
discarded or blocked on it. An emote fragment always has a URL (the
template needs nothing but the ID), but the frontend still treats a broken
image load as a safe, expected case and falls back to the fragment's own
plain text rather than hiding it.

### Deliberately unsupported presentation details

- **Animated-vs-static negotiation**: not implemented. Every emote image
  requested uses `format=static`, regardless of whether the emote also has
  an animated version, regardless of the viewer's `prefers-reduced-motion`
  setting (which is moot here, since nothing animated is ever requested).
  This is a real, documented behavior choice, not an oversight: negotiating
  `format=animated` would need the Get Channel/Global Emotes catalog this
  design deliberately avoids fetching, purely to decide a presentation
  detail.
- **Light-mode emote variants**: not implemented; `theme_mode=dark` is
  fixed, matching the application's own single dark theme.
- **Cheermote tier-specific images**: not implemented this stage. A
  cheermote fragment's `CheermotePrefix`/`CheermoteBits` (already
  normalized in stage 8A) are rendered as plain text, not resolved to
  Twitch's own Bits-tier image set - that catalog (`GET
  /helix/bits/cheermotes`) was not researched for this stage and is left
  for a future one if a real product need emerges.
- **Badge click actions/URLs**: `click_action`/`click_url` fields Twitch
  returns per badge version are read but never rendered as a clickable
  element - operator chat is a read-only diagnostic/working view, not an
  interactive Twitch chat client.

---

## Stage 11A addendum — relationship to outbound chat

This document covers **inbound** EventSub reading only. Stage 11A added
manual **outbound** Twitch chat sending and replying, researched and
documented entirely separately in
[`twitch-outbound-chat.md`](twitch-outbound-chat.md) - that document is
the authoritative contract for the Send Chat Message API, `is_sent`/
`drop_reason` behavior, rate limits and retry policy, not this one.

The two stay independent by design, not by accident:

- **Independent scope profiles.** Reading chat needs the five scopes in
  [Scopes](#scopes) above; sending needs only `user:write:chat` -
  requested through its own capability assessment
  (`AssessOutboundChatCapability`), never merged into the engagement
  profile this document defines.
- **Independent health.** An account can be healthy for inbound
  engagement while `permission_required` for outbound chat, or vice
  versa - exactly the same independence stage 8A already established
  between metadata and engagement health (see
  [Scope-profile design decision](#scope-profile-design-decision)
  above).
- **One shared fact.** A message sent through the outbound-chat
  dispatcher returns through *this* document's own `channel.chat.message`
  subscription, like any other message - Stage 11A adds no separate echo
  path and no optimistic local copy. See `twitch-outbound-chat.md` for
  why, and engagement-architecture.md §8.0 for the full design.
