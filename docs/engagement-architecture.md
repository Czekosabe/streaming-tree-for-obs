# Engagement and overlay architecture

> **This document is architecture and planning.** It describes a coherent target
> design for features that do **not exist yet**. Nothing described here is
> implemented unless the corresponding stage in the [roadmap](project-overview.md#13-roadmap)
> is marked **Completed**. Do not read any section of this document as a
> statement that the feature works today.
>
> The current, real state of the project is recorded in
> [progress.md](progress.md). When in doubt, that file wins.

---

## 1. Scope

This document plans the second half of the product: everything built on top of
the streaming router that turns Streaming Tree from "one OBS input, several
RTMP outputs" into a local engagement and overlay platform.

In scope for this document:

- a normalized event model that abstracts over Twitch, YouTube, Kick, TikTok
  and non-platform sources (external donation services),
- platform connectors as adapters into that model,
- unified operator chat and OBS Browser Source chat overlays,
- outbound chat: scheduled bot messages and commands,
- alerts, the alert queue, and the rule engine that feeds it,
- visual designers for chat overlays and alert overlays,
- a safe, declarative overlay template format, with import/export,
- text-to-speech as a consumer of the event stream,
- goals, counters and supporter widgets,
- external donation-service connectors,
- privacy, security and credential dependencies for all of the above,
- a staged implementation order.

## 2. Non-goals

Explicitly **not** planned, now or later, unless a future revision of this
document says otherwise:

- **Arbitrary code execution in overlays or templates.** No JavaScript
  execution, no `eval`, no unrestricted HTML, no remote script tags. See
  [§13 Template security](#13-template-security-model).
- **A general-purpose scripting language for bot commands.** Commands use a
  fixed, safe placeholder system (§8), not a template language with
  conditionals, loops or arbitrary expressions.
- **Scraping as a supported integration strategy.** See
  [§16 Provider support honesty](#16-provider-support-honesty).
- **A cloud-hosted version of any of this.** Everything in this document is
  planned for the local application described in
  [project-overview.md](project-overview.md); a remote-server mode is a
  separate, later concern and is not assumed here.
- **Bundling a specific TTS engine or voice model** as part of this
  architecture. §12 defines an abstraction; the actual engine is a future,
  separate decision that must independently clear a licensing review.
- **Promising a donation-service connector before its official integration
  method is verified.** See §15.

## 3. Terminology

| Term | Meaning |
| --- | --- |
| **Provider** | A built-in integration type: Twitch, YouTube, Kick, TikTok, or a non-platform source such as an external donation service. Exists today as `platform.ProviderDefinition` for the streaming side; the engagement side extends the same idea. |
| **Connector** | The adapter code that talks to one provider's real API/protocol and translates its events and actions into the normalized model. Not the same as a *configured destination* (§4): a connector is code, a configured destination is a user's row. |
| **Connected account** | A user's authenticated identity with a provider — distinct from a *configured destination* used for outgoing streaming. The two may reference the same provider without being the same record. **Implemented as of stage 7A** (`internal/domain/account`) for Twitch account lifecycle and metadata publishing, **extended to YouTube as of stage 7B** (account lifecycle, broadcast selection and metadata publishing); this document's engagement uses of it (chat, events) remain planned, stage 8A (Twitch) and stage 15 (YouTube) onward. |
| **Normalized engagement event** | One provider-independent representation of "something happened": a chat message, a follow, a donation, and so on. Defined in §5. |
| **Engagement Event Bus** | The in-process component that receives normalized events from connectors and distributes them to every consumer (§6). |
| **Operator chat** | The internal, full-detail chat view for the streamer/moderator (§7). |
| **OBS chat overlay** | The filtered, public Browser Source view of the same event stream (§7). |
| **Alert** | A short-lived visual/audio notification triggered by a rule matching an event (§9–10). |
| **Widget** | A persistent overlay element driven by accumulated state (a goal, a counter, a "latest supporter" panel) rather than by one event (§14). |
| **Template** | A declarative, versioned package describing the visual design of an overlay (§12–13). |

## 4. Relationship to the existing streaming domain

The existing `platform` domain (provider definitions, configured destinations,
stream metadata — see project-overview.md §8–9) is the **outbound streaming**
side of the product. This document's connectors are a **separate, additive**
concept for **inbound and bidirectional engagement**: reading chat, reading
events, and optionally sending chat messages.

A configured destination (e.g. "Main Twitch channel") and a connected account
for the same provider are not required to be the same thing, though in the
common case a user will want them linked. The data model must allow:

- a destination with no connected account (streaming only, no chat features),
- a connected account with no destination (reading chat/events from a channel
  you are not also streaming to — for example, moderating from an account that
  does not hold the destination's stream key),
- both, linked, which is expected to be the common case for Twitch.

This split exists because the two rely on entirely different credentials
(§17): a destination needs a *stream key*; a connected account needs an
*OAuth token* with chat/event scopes. Conflating them would force every viewer
of this document to reason about two credential lifecycles as if they were one.

> **Factual status update (stage 7A, completed):** the connected-account
> concept described above is no longer purely planned. `internal/domain/
> account` now implements a real, provider-independent connected-account
> foundation - identity, OAuth token storage, linking to a destination,
> validation and reconnect - and `internal/provider/twitch` is a real
> Twitch adapter over it, currently used only for account lifecycle and
> channel-metadata publishing (project-overview.md §8.1, §13). It does
> **not** read chat or events, has no EventSub subscription, and the Event
> Bus below still does not exist. Stage 8 is expected to reuse this same
> account foundation and Twitch adapter for its own authorization rather
> than building a second one - see §6.4's factual note and
> [`docs/provider-integrations/twitch.md`](provider-integrations/twitch.md)
> for the researched Twitch contract this adapter implements.
>
> **Factual status update (stage 7B, completed):** the same
> connected-account foundation now has a second real adapter,
> `internal/provider/youtube`, for account lifecycle, broadcast selection
> and video-metadata publishing - still **not** for reading YouTube live
> chat, Super Chat, or membership events, all of which remain stage 15's
> scope, and the Event Bus still does not exist. Stage 15 (not stage 8) is
> expected to reuse this YouTube adapter for its own authorization -
> see §16's own roadmap table - rather than building a second one, and
> [`docs/provider-integrations/youtube.md`](provider-integrations/youtube.md)
> for the researched Google/YouTube contract this adapter implements.

> **Factual status update (stage 8A, completed):** the Event Bus this
> section referred to as not existing now does - see §5-6 below, both no
> longer purely planned. `internal/provider/twitch` gained a real Twitch
> EventSub WebSocket connector, reusing the same connected-account
> foundation and requesting a second, additive scope profile on the same
> account rather than a competing authorization (§6.4's own factual note
> below has the detail). YouTube's own inbound connector, and the
> unified operator chat/OBS overlay that read the bus this stage builds,
> remain stage 15 and stages 9-10 respectively, still planned. See
> [`docs/provider-integrations/twitch-engagement.md`](provider-integrations/twitch-engagement.md)
> for the researched EventSub contract this connector implements.

## 5. Normalized engagement event model

### 5.1 Design goal

Nothing downstream of the Event Bus — operator chat, overlays, alerts, TTS,
goals — may depend on a provider-specific message shape. Every consumer reads
the same normalized event type. A connector's entire job is producing that
type correctly for its provider.

### 5.2 Core fields

Every normalized event is planned to carry at least:

| Field | Purpose |
| --- | --- |
| `id` | Globally unique **internal** event ID, generated by the bus (not the provider). Used for internal references (queueing, deduplication, deletion pointers). |
| `providerEventId` | The original ID the provider assigned, when it has one. Used for provider-side correlation (e.g. matching a later "message deleted" notice to the message it deletes). |
| `providerId` | Which provider this event came from (`twitch`, `youtube`, `kick`, `tiktok`, or a donation-service identifier). |
| `connectedAccountId` | Which connected account received this event. |
| `destinationId` | The configured destination this maps to, when relevant (optional — see §4). |
| `type` | The normalized event type (§5.4). |
| `platformTimestamp` | When the provider says it happened. |
| `receivedAt` | When the backend received it. Kept separate from `platformTimestamp` because clock skew and delivery latency are real and observable, and conflating them would corrupt ordering (§5.6). |
| `user` | Identity block: platform user ID, display name, avatar URL (nullable — see §5.5), badges, role information (subscriber/moderator/VIP/broadcaster, as the provider exposes it). |
| `messageFragments` | For chat-shaped events: an ordered list of text/emote/mention fragments, not a single pre-rendered string, so overlays can style emotes without re-parsing text. |
| `emotes` | Emote identifiers referenced by the fragments, with enough data to resolve an image (provider, emote ID). |
| `amount` / `currency` | For monetary events: value and ISO 4217 currency code, when the provider expresses one. |
| `quantity` | For countable events without a monetary value (Bits count, gifted-sub count, raid viewer count). |
| `rawProviderType` | The provider's own event/type identifier, kept for debugging and for connector-specific edge cases, but never consumed by generic downstream code. |
| `moderationRef` | For deletion/moderation events: a reference to the `id` (or `providerEventId`) of the event being moderated. |
| `dedupeKey` | A stable key used for deduplication (§5.7). |

Not every field applies to every event type; a follow event has no `amount`,
a chat message has no `quantity`. The type is a superset shape with clear
per-type expectations documented at the connector-capability level (§6.2),
not a union of unrelated payloads a consumer must switch on blindly.

### 5.3 Deliberately absent from the core model

- **No provider-specific nested payload as a first-class field.** If a
  connector needs to carry extra provider detail forward, it goes in a
  clearly-namespaced `providerExtra` bag that generic consumers must not read.
- **No secrets.** An event never carries a token, a stream key, or anything
  from §17. A connector authenticates itself; it does not put credentials in
  the data it publishes.

### 5.4 Planned event types

| Type | Notes |
| --- | --- |
| `chat.message` | A normal chat message. |
| `chat.message_deleted` | References the deleted message via `moderationRef`. |
| `chat.cleared` | The whole channel's chat was cleared. |
| `follow` | |
| `subscription` | A new subscription. |
| `resubscription` | A renewed/continuing subscription, with streak/cumulative-months data where the provider exposes it. |
| `gifted_subscription` | One gifted sub, with the gifter's identity when not anonymous. |
| `subscription_gift_batch` | A gifter giving N subs at once — kept distinct from N individual `gifted_subscription` events so a rule can target "gifted 5 subs" as one occurrence, not five. |
| `bits` / `cheer` | Twitch Bits. |
| `donation` | A monetary donation from a connector that is itself a donation channel (platform-native or external service). |
| `super_chat` | YouTube Super Chat. |
| `super_sticker` | YouTube Super Sticker. |
| `membership` | YouTube channel membership (new). |
| `gifted_membership` | YouTube gifted membership. |
| `raid` | Incoming raid, with source channel and viewer count when available. |
| `channel_point_redemption` | Twitch channel points (or an equivalent provider mechanic). |
| `moderation` | Timeout/ban/unban and similar actions, distinct from `chat.message_deleted`. |
| `stream.online` / `stream.offline` | The provider's own view of the connected account going live/offline — independent of Streaming Tree's own MediaMTX ingest state, which is a local, different concept. |
| `test` | A preview/test event (§11). Structurally identical to a real event of the type it simulates, but flagged so it can never be mistaken for one (§11.3). |

This list is a planning target, not a promise that every provider supports
every type — see §6.2 and §16.

### 5.5 Handling missing data

Providers do not all expose the same fields. An avatar may be unavailable, a
display name may be extremely long, a badge list may be empty. The model
therefore treats optional fields as genuinely optional (nullable), and every
consumer (operator chat, overlay, alert renderer) must have a defined,
non-crashing behaviour for the missing case — this is also why "missing
avatar" and "very long username" are explicitly planned preview/test events
(§11.1): they are not edge cases to discover after the fact, they are cases
designed for from the start.

### 5.6 Event ordering

Two ordering signals exist and must not be conflated:

- **`platformTimestamp`** — the provider's own notion of when it happened.
  Not reliably comparable across providers (clock skew, event batching), and
  not reliably monotonic within one provider's delivery stream.
- **`receivedAt`** plus a monotonically increasing bus sequence number — what
  the Event Bus actually uses to order display in operator chat and overlays,
  because it is the one ordering guarantee the bus itself controls.

Display ordering is planned to use receive order. `platformTimestamp` is
carried for information and for "how long ago" displays, never for sort order
across sources.

### 5.7 Deduplication

A connector may redeliver an event after a reconnect, and in principle two
different connectors could theoretically observe the same underlying action
(not expected in practice for chat, but the model should not assume it never
happens). `dedupeKey` exists for this: a connector computes it deterministically
from provider-stable identifiers (e.g. `providerId + providerEventId`, or a
content hash when no stable ID exists), and the bus drops a second event with
a `dedupeKey` it has already seen within a bounded recent window. The window
is bounded because holding an unbounded dedup set forever is a memory leak,
not because late duplicates are acceptable — a bounded window is a deliberate,
documented trade-off.

### 5.8 Deletion and moderation events

`chat.message_deleted`, `chat.cleared` and `moderation` are first-class event
types, not a mutation of a previously delivered event. Consumers therefore
never need to reach back and edit history in place:

- **Operator chat** shows the deleted message with a visible "deleted" marker
  rather than removing it, because a moderator's job is to see what was said.
- **OBS overlay** removes or visually retracts the referenced message,
  because a public overlay must not keep showing removed content.

This is why `moderationRef` exists on the model (§5.2): both behaviours need
to resolve "which earlier event does this refer to", and they are allowed to
react differently to the same underlying signal.

## 6. Connector interface concept

### 6.1 Adapters, not a shared protocol

The rest of the application depends on the normalized model (§5) and on the
connector interface below — never on Twitch EventSub payload shapes, YouTube
Live Chat message types, Kick's event format, or any future TikTok protocol.
A connector's only job is to translate its provider's reality into that model
and back.

Conceptually:

```
Platform connectors
        |
        v
Normalized Engagement Event Bus
        |
        +--> Operator chat
        +--> OBS chat overlays
        +--> Alert engine
        +--> Alert queue
        +--> TTS queue
        +--> Goals and counters
        +--> Event history (bounded, see §6.5)
        +--> Automation rules
```

### 6.2 Connector capability model

Not every provider supports every event type or every outbound action, and the
architecture must say so structurally rather than let a caller find out at
runtime. A connector is planned to declare a capability set conceptually
similar to:

```
ConnectorCapabilities {
  canReceiveChat:       bool
  canSendChat:          bool
  supportedEventTypes:  [EventType]     // subset of §5.4
  supportsDeletion:     bool            // can report chat.message_deleted
  supportsModeration:   bool
  requiresOAuth:        bool
}
```

This is the same philosophy already used for streaming-metadata capabilities
(project-overview.md §9): rather than one shared feature set every provider is
forced into, each provider declares what it actually does, and the rest of the
system renders and behaves accordingly — an alert rule referencing an event
type a provider cannot produce is inert for that provider, not broken.

### 6.3 Inbound and outbound capabilities

A connector may support any combination of:

- **receiving chat**,
- **sending chat**,
- **both**,
- **neither** (an events-only connector — most external donation services fall
  here: they push donation events but have no "chat" concept at all).

Outbound sending is its own capability, separate from receiving, because
authorization scopes and rate limits usually differ, and because a connector
that reads chat should not be assumed able to post to it.

### 6.4 Connected accounts and OAuth

A connector that requires authentication does so through a **connected
account**: a stored, provider-scoped authorization distinct from a configured
destination's stream key. Connected accounts depend on OAuth token storage,
which depends on the credential-store foundation stage 5 implements (§17) —
the same `SecretStore` abstraction, a different secret type. OAuth flows
themselves (redirect handling, token refresh) are **not** designed in this
document; they belong to the stage that actually adds the first OAuth
connector.

> **Factual status update (stage 7A, completed):** that first OAuth
> connector now exists for Twitch - device-code sign-in, token storage,
> refresh and reconnect, exactly as anticipated above - but scoped only to
> account lifecycle and metadata publishing (project-overview.md §8.1,
> §13; [`docs/provider-integrations/twitch.md`](provider-integrations/twitch.md)).
> It requests only `channel:manage:broadcast`; it has no chat or event
> scope, does not read chat, and is not wired to any Event Bus, because
> that bus (§5 below) still does not exist. A future Twitch engagement
> connector (stage 8) is expected to request additional scopes on the same
> underlying connected account rather than creating a second, competing
> Twitch authorization.

> **Factual status update (stage 7B, completed):** a second OAuth
> connector now exists for YouTube - Authorization Code Flow with PKCE via
> a temporary loopback callback and a real system browser (not a device
> code, deliberately - see
> [`docs/provider-integrations/youtube.md`](provider-integrations/youtube.md)
> for why), token storage, refresh and reconnect, scoped only to account
> lifecycle, broadcast selection and metadata publishing. It requests only
> `https://www.googleapis.com/auth/youtube.force-ssl`; it has no chat or
> event scope, does not read YouTube live chat or Super Chat, and is not
> wired to any Event Bus. A future YouTube engagement connector (stage 15,
> not stage 8) is expected to request additional scopes on the same
> underlying connected account rather than creating a second, competing
> YouTube authorization.

> **Factual status update (stage 8A, completed):** that Twitch engagement
> connector now exists exactly as anticipated above - it requests a
> second, additive scope profile (`user:read:chat`,
> `moderator:read:followers`, `channel:read:subscriptions`, `bits:read`,
> `channel:read:redemptions`) on the *same* connected-account row stage
> 7A created, via an identity-bound upgrade of the existing Device Code
> Flow, never a second Twitch authorization. Metadata health
> (`channel:manage:broadcast`) and engagement-capability health are
> tracked independently: an account missing the newer scopes still
> validates and publishes metadata normally, and is shown as
> "permission upgrade required" for engagement specifically, not marked
> `reconnect_required` outright. `user:write:chat` (outbound chat) is
> still not requested anywhere - that remains stage 11's scope. See
> [`docs/provider-integrations/twitch-engagement.md`](provider-integrations/twitch-engagement.md).

### 6.5 In-memory buffer versus persisted history

The default and only currently planned storage model is an **in-memory,
bounded ring buffer** of recent events per view (operator chat, overlay),
mirroring the precedent already set by MediaMTX runtime state
(project-overview.md §8.1): operational state that describes "what is
happening" lives in memory and resets on restart.

> **Factual status update (stage 8A, completed):** this ring buffer is no
> longer only planned - `internal/engagement.Bus` implements it exactly
> as described here: a fixed-capacity buffer (default 1000 events,
> operator-configurable via `STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE`
> within a validated 100-10000 range), bounded TTL'd deduplication, and a
> complete reset to empty on every backend restart. It is read today only
> by the diagnostic Engagement page and its Server-Sent Events stream
> (`GET /api/engagement/stream`) - operator chat and the OBS overlay
> themselves are still stages 9 and 10, planned. No persisted event
> history exists; the paragraph below remains accurate unchanged.



A **persisted event history** (searchable past chat, historical alert log) is
an explicit **optional, later** extension, not assumed by the rest of this
architecture. If implemented, it is planned as its own opt-in store, separate
from the SQLite configuration tables, because event history has different
volume, retention and privacy characteristics than configuration (§17
"Privacy"). No stage in the current roadmap implements it.

## 7. Unified chat

### 7.1 One event stream, two views

Operator chat and the OBS overlay are **different views of the same
underlying normalized event stream** (§5), not two separate chat systems that
happen to look similar. A message that never reaches the Event Bus cannot
appear in either.

### 7.2 Operator chat

The full, internal view for the streamer or moderator. Planned to show:

- every connected platform's messages, merged,
- platform icon and platform name per message,
- badges and role/moderator information,
- deleted-message status (§5.8) — shown, not hidden,
- system messages (connector connected/disconnected, and similar),
- event messages (follows, subs, etc.) inline with chat, when useful for
  moderation context,
- messages that are hidden from the public overlay by filter rules, still
  visible here — the operator view is deliberately more permissive than the
  public one.

### 7.3 OBS chat overlay

A filtered **public** Browser Source. Planned settings:

- included platforms,
- show/hide: platform icon, textual platform name, avatars, badges,
- hide bots, hide commands (§8.2), hide selected users, hide selected words,
  hide selected event types,
- message lifetime and maximum visible message count,
- entry and exit animation,
- typography: font, text size, outline, shadow,
- message bubble background, spacing, alignment,
- a vertical-stream layout variant,
- highlight rules for moderators, subscribers and supporters.

The overlay is a **rendering and filtering layer** over the same stream
operator chat reads; it holds no separate connection to any provider.

## 8. Outbound chat and bot automation

### 8.1 Scheduled bot messages

Planned settings per scheduled message:

- enabled/disabled,
- one or more target platforms (via connectors that `canSendChat`, §6.3),
- one message for all platforms, or per-platform variants,
- interval, first-send delay,
- allowed streaming hours,
- "only while ingest is receiving" — tying bot activity to the *local* MediaMTX
  ingest state that already exists (project-overview.md §8.2), so a bot never
  posts while nothing is actually being streamed,
- minimum viewer/chat-activity thresholds where the provider exposes them,
- minimum number of viewer messages since the previous bot message (so a bot
  does not talk over a quiet chat),
- global cooldown and per-platform cooldown,
- maximum sends per hour,
- optional randomized delay (so multiple scheduled messages do not all fire in
  lockstep),
- message **groups** with random selection from the group, so repeated sends
  do not read as an obvious loop,
- a preview of the next scheduled execution,
- manual "Send now",
- automatic suspension when the stream ends.

### 8.2 Chat commands

Planned example commands: `!discord`, `!socials`, `!uptime`, `!title`,
`!game`, `!commands`.

Planned settings per command:

- name and aliases,
- response text (placeholder-based, §8.3),
- target platforms,
- per-user cooldown and global cooldown,
- required role (everyone / subscriber / moderator / broadcaster),
- enabled/disabled,
- "reply on the same platform the command was issued on" as the default
  behaviour.

### 8.3 Placeholder system

Bot messages and command responses use a **fixed, safe placeholder
vocabulary**, not a scripting or templating language. Planned placeholders
include `{channelName}`, `{platform}`, `{streamTitle}`, `{streamUptime}`,
`{channelUrl}`. Substitution is a bounded, whitelisted lookup — there is no
arbitrary code execution and no plan to add one (§2).

## 9. Alerts

### 9.1 Alert rules

An alert rule consumes normalized events (§5) and decides whether to enqueue
an alert. Planned categories: follow, subscription, resubscription, gifted
subscription, subscription gift batch, raid, Bits, donation, Super Chat, Super
Sticker, membership, gifted membership, channel-point redemption, and a custom
test category (§11) for designing without waiting for a real event.

Planned rule conditions and outputs:

- event type, provider filters, connected-account filters,
- minimum monetary threshold, and threshold **tiers** (so "small donation" and
  "large donation" can be genuinely different alerts, not the same alert with
  a bigger number — see the worked example below),
- minimum quantity (e.g. Bits count, gift-batch size),
- user-role filters,
- show/hide the platform source on the rendered alert,
- queue priority (§10),
- duration, sound, volume,
- image/GIF/video asset,
- entry/exit animation,
- text template (placeholder-based, same constraint as §8.3 — not a scripting
  language),
- username visibility, message visibility, amount visibility (a streamer may
  want to acknowledge a donation without displaying the exact amount),
- TTS enabled/disabled for this rule (feeds §12).

### 9.2 Worked threshold example

A single "donation" event type is expected to map to **several rules with
different thresholds**, each with its own design, e.g.:

| Tier | Threshold (example) | Typical design intent |
| --- | --- | --- |
| Small donation | below a low threshold | brief, quiet, low-priority |
| Normal donation | mid-range | standard alert |
| Large donation | high threshold | longer, louder, higher priority |
| Exceptional donation | very high threshold | full-screen, highest priority, may interrupt (§10) |

The specific threshold values are a per-user configuration choice, not a
constant this document fixes.

## 10. Alert queue

Alerts do not render immediately on arrival; they enter a queue. Planned
behaviour:

- sequential playback (one alert visible at a time, by default),
- priority ordering (a rule's configured priority, §9.1),
- grouping of similar events arriving close together (e.g. a burst of gifted
  subs collapsing into one "gifted 5 subs" style presentation rather than five
  separate alerts, complementing `subscription_gift_batch`, §5.4),
- expiration of stale queued alerts (an alert that waited too long is dropped
  rather than played long after the moment it was about),
- pause, skip-current, replay-previous, and clear-queue controls,
- a maximum total queue duration, to bound how far behind "now" the queue can
  fall,
- interrupt rules for high-priority events (an "exceptional donation" may be
  allowed to jump the queue; the rule for when that is allowed is itself
  configuration, not a hard-coded exception).

## 11. Preview and test events

### 11.1 Purpose

The visual designers (§14) need realistic-looking data to design against
before any real event has occurred, and need to exercise edge cases
deliberately rather than by waiting for one to happen live. Planned generated
scenarios: follow, subscription, resubscription, one gifted subscription, a
gift batch, Bits, small donation, large donation, Super Chat, membership,
raid, a short chat message, a very long username, a very long message, a
missing avatar, and an unknown/unrecognised platform.

### 11.2 Why "very long username" and "missing avatar" are planned test cases

These are not hypothetical robustness concerns; they are §5.5's optional-field
handling made concrete and testable. A designer must be able to see, on
purpose, what an overlay looks like when a field is absent or extreme, rather
than discovering it during a live broadcast.

### 11.3 Isolation from real data paths

A test/preview event is the same normalized shape as a real one (§5.4, type
`test`, or a real type explicitly flagged as synthetic) but is generated
entirely locally and:

- **never** passes through a real platform connector,
- **never** enters the real alert queue's history/statistics,
- **never** counts toward a goal or counter (§14),
- **never** triggers TTS against real per-user/global cooldown state meant for
  genuine supporters,
- is visibly and structurally distinguishable from a real event, so a
  consumer cannot mistake one for the other even if a UI label were missed.

## 12. Text-to-speech

TTS is planned as a **consumer of the normalized event stream** (§5), not a
capability of any platform connector. Any event type that reaches the bus can
in principle feed TTS, subject to its own configuration.

Planned settings:

- provider mode: disabled / system / local / cloud,
- enabled event types, enabled platforms,
- supporter-only mode, minimum amount, minimum Bits,
- maximum text length,
- per-user cooldown, global cooldown,
- blocked words, URL removal, repeated-character normalization (e.g.
  "soooooo" collapsing before being read aloud),
- command suppression (a chat command should not be read aloud as if it were
  a message),
- queue size, manual approval mode, skip-current, clear-queue,
- voice, language, speed, volume.

A `TTSProvider` abstraction is planned so "system", "local" and "cloud" are
interchangeable implementations behind one interface — mirroring the
`SecretStore` abstraction pattern used elsewhere in this codebase (§17). No
specific engine is selected or bundled by this document; **licensing of any
local voice engine or voice model must be evaluated on its own before
bundling**, exactly as MediaMTX's licence was reviewed before it was bundled
(project-overview.md §7.4).

## 13. Visual designers and templates

### 13.1 Two designers, one underlying model

Two planned visual editors: the **Chat Overlay Designer** and the **Alert
Overlay Designer**. Both operate on layered visual elements over the same kind
of underlying data (chat messages for one, alert events for the other) and are
planned to share the same layer/asset/typography primitives rather than being
built as unrelated tools.

Planned layer/setting primitives: background, platform icon, user avatar,
username, event title, message, amount, image/GIF/video, sound, entry/exit
animation, opacity, position, size, typography, text outline, shadow, border,
corner radius, alignment.

### 13.2 Template security model

This is a hard boundary, not a preference. A template package **must not**
permit:

- arbitrary JavaScript execution,
- executable files of any kind,
- external `<script>` tags or remote script loading,
- unrestricted HTML (only a constrained, whitelisted set of presentational
  elements the designer itself can produce),
- filesystem access from within a rendered overlay,
- unrestricted outbound network requests from a rendered overlay.

A template is a **declarative, data-only description** — positions, sizes,
colours, references to bundled asset files, and references to the fixed
placeholder vocabulary (§8.3) — never a program.

### 13.3 Template package format

A planned template package (`template.json` plus assets) may contain images,
WebP, GIF, video, audio, preview images, and fonts whose redistribution
licence explicitly permits inclusion. It must not contain anything §13.2
forbids.

Planned package handling:

- **built-in default templates**, shipped with the application,
- **import** and **export** of user-modified or third-party templates,
- a **versioned template schema**, with migration of older template versions
  forward when the schema changes,
- **previews** before a template is applied,
- **validation** on import: reject anything violating §13.2 before it is ever
  rendered, not after,
- **archive traversal protection** on import, using the same class of defense
  already implemented and tested for the MediaMTX archive installer
  (`apps/server/internal/runtime/mediamtx/archive.go` — absolute-path
  rejection, `..`-segment rejection, symlink/hard-link rejection, size limits):
  a template package is exactly as untrusted as a downloaded binary archive
  and must be validated with the same rigor,
- **asset-size limits**,
- **licence metadata** carried with the package, so a user importing a
  third-party template can see what they are bound by.

Possible future built-in visual families (naming only, no assets created by
this task): Minimal Dark, Clean Modern, Neon Gaming, Retro Pixel, Fantasy,
Pastel, Horror, Cyberpunk, Vertical Stream, Just Chatting.

## 14. Goals, counters and widgets

Planned overlay widgets, all driven by accumulated normalized-event state
rather than by a single event: follower goal, subscription goal, donation
goal, Bits goal, latest follower, latest subscriber, latest donation, largest
donation, recent supporters list, an event ticker, and platform-specific
counters.

Widgets read the same Event Bus (§5–6) that alerts and TTS do; a goal is
"alerts, integrated over time," not a separately fetched data source.

## 15. External donations

Donations are not assumed to originate only from Twitch, YouTube, Kick or
TikTok. External donation services are planned as their **own connector
category** (§6), producing `donation` events (§5.4) exactly like a
platform-native donation would, so the rest of the system never needs to know
the money came from outside the streaming platform.

Potential future connector categories: StreamElements, Streamlabs, Ko-fi, and
other services that publish an official API or webhook. **No connector for any
of these is promised until its official integration options have been
verified** — this document names candidates, it does not commit to them.

## 16. Provider support honesty

- **Twitch is expected to be the first engagement connector**, both because it
  has the most mature public API for chat/events and because it is already the
  only provider with tag support on the streaming side (project-overview.md
  §9), suggesting the deepest existing familiarity.
- **YouTube and Kick require separate adapters.** Nothing about a Twitch
  connector implementation is assumed to transfer directly; each is planned as
  its own connector against the capability model of §6.2.
- **TikTok LIVE chat/events are implemented only when an official, permitted,
  and sufficiently stable integration exists.** As of this writing no such
  integration is confirmed suitable; TikTok engagement support is listed in
  the roadmap (§18) as conditional, not scheduled.
- **The project does not depend on fragile scraping as a core product
  feature.** A scraping-based integration is not planned; if one were ever
  considered as a stop-gap, it would be clearly labelled experimental and kept
  out of the load-bearing path of any feature.
- **Support matrices, once connectors exist, must distinguish**: verified,
  partial, planned, unavailable, and experimental support — not a single
  yes/no per provider. This mirrors the honesty already required of the
  streaming metadata tables (project-overview.md §9: "these definitions ...
  have not been verified against the real ... APIs").

## 17. Privacy, security and credential dependencies

### 17.1 What this document depends on from the current task

The credential-store foundation implemented in stage 5 (see the
`feat(server): add system credential store` progress entry) is a **hard
prerequisite** for:

- FFmpeg destination stream keys (roadmap stage 6, completed),
- OAuth tokens for connected accounts (§6.4, roadmap stages 7A/7B,
  **completed** for Twitch and YouTube account-lifecycle and
  metadata-publish tokens; the engagement Event Bus's own eventual use of
  the same tokens for chat/event scopes is still stage 8 (Twitch) and
  stage 15 (YouTube), planned),
- any future outbound bot-message credential (if a connector ever needs one
  beyond its OAuth token).

The `SecretStore` interface is secret-type-agnostic, exactly as anticipated:
stage 7A's and 7B's connected-account OAuth token bundles both reuse it
under the same secret type (`oauth-token-bundle:<connected-account-id>`),
the same OS-backed, no-plaintext-fallback guarantees, and the same
atomic-replacement requirement §17.1 always intended - including YouTube's
own refresh-response quirk (Google's refresh omits a new refresh token
more often than not, so the bundle's previous one is preserved rather
than replaced with an empty value) - see project-overview.md §10 for the
implemented shape.

### 17.2 Data sensitivity of engagement data

Chat messages, donor names and donation amounts are **user-generated,
potentially personally identifying content**, not configuration. This
architecture treats them accordingly:

- the default model is an in-memory bounded buffer (§6.5), not a permanent
  log,
- if persisted history is ever added, it is opt-in and separately scoped —
  never silently enabled as a side effect of another feature,
- preview/test events are structurally barred from ever being confused with
  or contributing to real supporter data (§11.3),
- template rendering has no filesystem or network access (§13.2), so a
  malicious or buggy template cannot exfiltrate chat content.

### 17.3 Security posture carried forward

Every security property already established for MediaMTX and platform
configuration is expected to carry forward unchanged for engagement features:

- the browser never talks to a third-party provider or credential store
  directly — only the Go backend does, exactly as the browser never talks to
  the MediaMTX Control API directly (project-overview.md §7.4),
- no secret of any kind (stream key, OAuth token, donation-service webhook
  secret) is ever returned by an API response, stored in the browser, or
  logged,
- connectors run under the same "one failure must not take down the whole
  application" principle already applied to MediaMTX supervision and, since
  stage 6, FFmpeg destination-branch supervision.

## 18. Proposed staged implementation order

This section is the engagement-specific detail behind the roadmap in
[project-overview.md §13](project-overview.md#13-roadmap). Stage numbers match
that table.

| Stage | Scope |
| --- | --- |
| 5 | Secure credential-store foundation (this task's implementation part) |
| 6 | FFmpeg destination branches (consumes the credential store for stream keys) |
| 7A/7B | Connected accounts, OAuth, platform metadata publishing for Twitch and YouTube (consumes the credential store for tokens) |
| 7C | Kick and TikTok account integration — **deferred**, capability-gated, not a prerequisite for stage 8A (see the factual note after this table) |
| 8A | Engagement Event Bus + Twitch inbound connector (first real implementation of §5–6) |
| 8B | Additional Twitch event coverage, reserved only if 8A cannot safely cover the full verified event set |
| 9 | Unified operator chat (§7.2) |
| 10 | OBS chat overlay (§7.3) |
| 11 | Outbound chat, scheduled bot messages and commands (§8) |
| 12 | Alert engine and alert queue (§9–10) |
| 13 | Visual overlay designers (§13.1) |
| 14 | Built-in templates and template import/export (§13.3) |
| 15 | YouTube and Kick engagement connectors (§16), and Kick account integration if not already done in 7C |
| 16 | External donation-service connectors (§15) |
| 17 | TTS and audio queue (§12) |
| 18 | Goals, counters and event widgets (§14) |
| 19 | TikTok LIVE connector, conditional on an official integration existing (§16) |
| 20 | Logs, diagnostics, packaging and remote-server hardening |

> **Roadmap decision (recorded when stage 8A began):** stage 8A starts
> before stage 7C is implemented. 7C (Kick/TikTok accounts) is not a
> dependency of the Event Bus — the bus and its first connector need only
> the Twitch adapter stage 7A already built — while every stage from 9
> onward genuinely cannot begin without the bus. Kick account integration
> is expected to move to stage 15, alongside its own engagement adapter,
> rather than staying a separate earlier stage; TikTok remains conditional
> as already stated. See [progress.md](progress.md) for the entry recording
> this decision and `project-overview.md` §13/§16 for the roadmap table.

Dependencies that constrain this order:

- Stage 6 (FFmpeg) and stage 7A/7B (OAuth) both need stage 5's credential
  store — they need different secret types through the same abstraction.
- Stage 8A (event bus) is a prerequisite for every stage from 9 onward: nothing
  downstream can exist before there is a normalized stream to consume.
- Stage 9 (operator chat) and stage 10 (overlay) both read stage 8's bus; one
  is not a prerequisite for the other, but both need it.
- Stage 11 (outbound/bot) needs connector send-message capability (§6.3),
  which is part of the stage 8 Twitch connector's capability declaration.
- Stage 12 (alerts) needs stage 8's normalized events; the alert queue (§10)
  is not useful without the rule engine that feeds it, so they are one stage.
- Stage 13 (designers) needs a stable overlay data shape to design against,
  which only exists meaningfully once stage 9/10 (chat) and stage 12 (alerts)
  establish what an overlay actually renders.
- Stage 14 (templates) needs stage 13's designer output format to serialize.
- Stage 17 (TTS) and stage 18 (goals/widgets) both consume the stage 8 bus
  directly and do not depend on the designers, so they can in principle move
  earlier if priorities change — they are ordered late here only because they
  are lower-impact than chat and alerts, not because of a hard dependency.

No stage in this table is implemented by the current task. See
[progress.md](progress.md) for what actually exists today.
