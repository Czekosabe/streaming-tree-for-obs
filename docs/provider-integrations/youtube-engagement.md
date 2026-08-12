# YouTube Live Chat engagement - research contract (Stage 15A)

**Research date/time:** 2026-08-12, ~06:00-06:20 UTC.

This document is the canonical, pre-implementation research contract for
Stage 15A: a real YouTube Live Chat engagement connector, mirroring the
role `docs/provider-integrations/twitch-engagement.md` plays for Twitch.
Written **before** any YouTube engagement/chat code exists in this
repository, per this project's own "document the contract before writing
provider code" discipline.

## Sources inspected (official only)

All fetched directly from `developers.google.com` on the research date
above. No unofficial YouTube chat library, scraping project, or
third-party blog/tutorial was consulted for any API contract detail.

- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages/streamList>
- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages/streamList> (re-fetched with a skeptical, quote-only prompt to cross-check the first pass)
- <https://developers.google.com/youtube/v3/live/streaming-live-chat> (contains the full inline `stream_list.proto` source, fetched twice - once for a narrative summary, once asking for a verbatim reproduction, cross-checked against the JSON resource page in §3 below for field-name/type consistency)
- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages/list>
- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages>
- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages/insert>
- <https://developers.google.com/youtube/v3/live/docs/liveBroadcasts/list>
- <https://developers.google.com/youtube/v3/live/docs/liveBroadcasts>
- <https://developers.google.com/youtube/v3/live/authentication>
- <https://developers.google.com/youtube/v3/determine_quota_cost>
- Prior in-repo research already covers YouTube's OAuth/installed-app flow
  in full (`docs/provider-integrations/youtube.md`, 2026-08-05) - not
  re-researched here, only reused (§2).

`liveBroadcasts/list` is used only to confirm the `snippet.liveChatId`
field path already documented informally in `liveBroadcasts` itself;
`videos/list` and `live/guides/auth/installed-apps` were not separately
fetched because §2 below shows the existing, already-implemented Stage
7A/7B OAuth flow already answers every question those pages would have.

## 1. Executive summary / implementation decision

**Transport decision: plain REST polling via `liveChatMessages.list`,
respecting the server-supplied `pollingIntervalMillis` - not the gRPC
`liveChatMessages.streamList` method.** This is a deliberate, documented
deviation from this task's own stated preference for the "current
recommended low-latency transport," made because that preference is
explicitly conditioned on the transport being **practical in Go today**,
and it is not, for reasons detailed in §4. Every other Stage 15A design
decision in this document follows from that choice.

No additional OAuth scope is required: the already-requested
`https://www.googleapis.com/auth/youtube.force-ssl` (`youtube.RequiredScope`,
`internal/provider/youtube/metadata.go`) is accepted for
`liveChatMessages.insert` per §3, and no narrower/broader scope is
documented anywhere as required specifically for `list`/`streamList`
(§3.6). **No reconnect or scope-upgrade flow is needed for Stage 15A.**

## 2. Authentication - reused, not re-researched

Stage 15A reuses the existing Stage 7A/7B OAuth2 Authorization Code +
PKCE + loopback-callback flow (`internal/runtime/youtubeauth`,
`internal/provider/youtube/oauth_client.go`) and the existing
`account.Service.WithFreshToken` single-flight-refresh-then-retry-once
pattern unchanged - see `docs/provider-integrations/youtube.md` for the
full, already-researched contract. Stage 15A adds **no new token store,
no new refresh mechanism, and no new scope**.

## 3. API contract, as researched

### 3.1 `liveChatMessages.streamList` (evaluated, not selected)

`GET`-equivalent **gRPC server-streaming RPC**
`youtube.api.v3.V3DataLiveChatMessageService.StreamList`. Confirmed via
two independent fetches (a narrative summary and a literal-quote
cross-check) that the page genuinely says "gRPC" and "server-streaming
connection" - this is a real, current, documented transport, not a
fetch-tool hallucination. The complete `.proto` source is published
inline (not as a separate downloadable file) on the
`streaming-live-chat` guide page; no official Go client library is
named anywhere - the page only links to `grpc.io`'s generic
code-generation instructions for Go, alongside C++/Java/Python/Node.
The one code sample shown end-to-end is Python
(`pip install grpcio grpcio-tools`, generated `stream_list_pb2*`
modules).

### 3.2 `liveChatMessages.list` (selected transport)

`GET https://www.googleapis.com/youtube/v3/liveChat/messages`. Required
params: `liveChatId` (string, from a broadcast's `snippet.liveChatId`),
`part` (`id`, `snippet`, `authorDetails`). Optional: `pageToken`,
`maxResults` (200-2000, default 500), `profileImageSize` (16-720,
default 88), `hl`. Response:

```
{
  "kind": "youtube#liveChatMessageListResponse",
  "etag": string,
  "nextPageToken": string,
  "pollingIntervalMillis": uint,
  "offlineAt": datetime,       // present once the underlying stream ended
  "pageInfo": { "totalResults": int, "resultsPerPage": int },
  "items": [ liveChatMessage ],
  "activePollItem": liveChatMessage
}
```

The documentation itself recommends `streamList` specifically to reduce
polling frequency/quota usage versus calling `list` more often than
`pollingIntervalMillis` suggests - honestly recorded as a real tradeoff
of the transport decision in §4, not hidden.

### 3.3 `liveChatMessage` resource shape

Cross-checked between the REST JSON resource page and the proto (same
underlying schema, camelCase JSON vs. snake_case+field-number proto -
consistent on every field checked, which is why both are trusted).

```
liveChatMessage {
  kind, etag, id: string
  snippet: {
    type: enum (see §5 mapping table)
    liveChatId, authorChannelId, publishedAt: string
    hasDisplayContent: bool
    displayMessage: string   // present unless the message is "silent" (tombstone/chatEndedEvent)
    // exactly one of the following, selected by `type`:
    textMessageDetails { messageText }
    fanFundingEventDetails { amountMicros, currency, amountDisplayString, userComment }   // legacy name for Super Chat, effectively superseded (§5)
    superChatDetails { amountMicros, currency, amountDisplayString, userComment, tier }
    superStickerDetails { amountMicros, currency, amountDisplayString, tier, superStickerMetadata { stickerId, altText, language } }
    newSponsorDetails { memberLevelName, isUpgrade }
    memberMilestoneChatDetails { userComment, memberMonth, memberLevelName }
    membershipGiftingDetails { giftMembershipsCount, giftMembershipsLevelName }
    giftMembershipReceivedDetails { memberLevelName, gifterChannelId, associatedMembershipGiftingMessageId }
    userBannedDetails { bannedUserDetails { channelId, channelUrl, displayName, profileImageUrl }, banType, banDurationSeconds }
    pollDetails { metadata { questionText, options [{ optionText, tally }] }, status }
    giftEventDetails { giftMetadata { jewelsAmount, giftName, giftUrl, giftDuration, hasVisualEffect, comboCount, altText, language } }  // the newer virtual-gift/"Jewels" type
  }
  authorDetails: {
    channelId, channelUrl, displayName, profileImageUrl: string
    isVerified, isChatOwner, isChatSponsor, isChatModerator: bool
  }
}
```

**Confirmed: `superStickerMetadata` carries `stickerId`/`altText`/
`language` only - no image URL field anywhere in the schema.** The
proto's own doc comment on `LiveChatMessage.id` states: *"Note: For
giftEvents, the same ID may be reused to update the combo count"* -
i.e. `giftEvent`'s message ID is explicitly **not** always a fresh,
independent identity; per this document's own §6/§8 policy, `giftEvent`
is therefore treated as **not normalized in Stage 15A** (§5), exactly
matching the "leave it unsupported rather than force it through
ordinary dedup" guidance.

### 3.4 `liveChatMessages.insert`

`POST https://www.googleapis.com/youtube/v3/liveChat/messages`. Accepted
scopes: `https://www.googleapis.com/auth/youtube` **or**
`https://www.googleapis.com/auth/youtube.force-ssl` - the scope already
requested by this application covers sending. Body for a text message:

```json
{ "snippet": { "liveChatId": "...", "type": "textMessageEvent", "textMessageDetails": { "messageText": "..." } } }
```

**No reply/parent-message field exists anywhere in the insert contract.**
Confirmed absent from both the request schema and the response resource
- YouTube Live Chat simply has no reply concept via this API. **No
documented maximum message length** - error `400 messageTextInvalid` is
the provider's own validation outcome, not a documented numeric bound.
Documented errors: `400 messageTextInvalid`/`liveChatIdRequired`/
`messageTextRequired`/`typeRequired`, `403 forbidden`/`liveChatDisabled`/
`liveChatEnded`, `404 liveChatNotFound`, `429 rateLimitExceeded`.

### 3.5 `liveBroadcasts` - resolving `liveChatId`

`snippet.liveChatId` (string) on the `liveBroadcast` resource -
*"With this ID, you can use the liveChatMessage resource's methods to
retrieve, insert, or delete chat messages."* `status.lifeCycleStatus`
enum: `created`, `ready`, `testing`, `liveStarting`, `live`, `complete`,
`revoked`, `testStarting`. The documentation does not explicitly state
whether `snippet.liveChatId` is present/absent when chat is disabled for
a broadcast - Stage 15A treats an empty/missing `liveChatId` string as
the honest "no live chat available yet" signal regardless of exactly
why, rather than trying to distinguish "chat disabled" from "not live
yet" from the field's presence alone (see §7).

### 3.6 OAuth scopes for reading

Neither the `list` nor the `streamList` reference page documents an
explicit "Authorization scopes" table (`insert`'s page does, and lists
`youtube`/`youtube.force-ssl` - §3.4). Since reading a broadcast's own
live chat is a strict subset of what `youtube.force-ssl` already grants
for broadcast/video management, and no separate/narrower/broader scope
is documented anywhere for reading, **Stage 15A requests no additional
scope** - answering research question 6/7 (§ "questions to answer")
definitively: yes, the existing scope already covers every Stage 15A
read/write operation.

### 3.7 Quota

The general quota-cost page does not break out `liveChatMessages.list`/
`streamList`/`insert` in its shown cost table; it only confirms Live
Streaming API methods share the same **10,000-units/day default project
quota** as the rest of the YouTube Data API. No exact per-call unit cost
is claimed here since none was found in the inspected page - this is
recorded as genuinely undocumented in this snapshot rather than guessed.

## 4. Why `streamList` (gRPC) was evaluated and not selected

This project's own explicit instruction is to prefer the current
official low-latency transport **if it is practical in Go**. It is not,
in this repository, for three independent, each-individually-sufficient
reasons:

1. **No official Go client exists.** The `streaming-live-chat` guide
   names no Go package; it only links to `grpc.io`'s generic "how to
   generate a client from a `.proto` with `protoc`" instructions - the
   same instructions given for C++/Java/Node, none of which this
   project uses either.
2. **No official, independently-versioned `.proto` artifact exists.**
   The `.proto` source is published only as an inline HTML code block on
   a documentation page, meant to be hand-copied by a developer, not
   fetched/pinned as a versioned dependency the way this project's other
   third-party contracts are (Go modules, `THIRD_PARTY_NOTICES.md`
   entries with a real upstream release/commit reference).
3. **This development environment has no `protoc` (or `protoc-gen-go`/
   `protoc-gen-go-grpc`) installed**, confirmed directly
   (`which protoc` → not found). Generating correct `proto2`-syntax Go
   bindings by hand - without a compiler to verify wire-format
   correctness (field numbers, `oneof` handling, `optional` presence
   semantics) - for an API that carries real monetary data (Super
   Chat/Super Sticker amounts) is exactly the kind of unverifiable
   guess this project's own research discipline forbids. A hand-
   transcribed proto, however carefully cross-checked against two
   independent documentation fetches (§3.1), is still not verified
   against the one authority that actually matters: the real `protoc`
   compiler and Google's real wire format.

Given all three, implementing `streamList` today would mean either (a)
hand-writing unverifiable low-level protobuf wire encoding/decoding by
hand in Go (a much larger, much riskier undertaking than anything else
in Stage 15A, and squarely the kind of code this project's own
dependency discipline says to avoid), or (b) introducing a third-party
Go protobuf/gRPC runtime dependency (`google.golang.org/grpc` +
`google.golang.org/protobuf`) purely to compile a hand-copied,
unofficial `.proto` file this project cannot cite an authoritative
upstream source/version for. Neither is acceptable under this project's
"no dependency without genuine, source-backed necessity" discipline.

**REST polling via `liveChatMessages.list` uses zero new dependencies**
- it is one more JSON GET call through the exact same
`internal/provider/youtube` HTTP client (`Client.doAPI`) every other
YouTube Data API call in this codebase already goes through, with a
request/response shape backed by the same official JSON schema every
other endpoint in this file already trusts. The honest cost, recorded
rather than hidden: higher quota consumption and higher latency
(bounded by `pollingIntervalMillis`, server-recommended and typically on
the order of a few seconds) than `streamList` would offer. If a future
stage finds an official, versioned, independently-fetchable `.proto`
artifact (or an official Go client ships), this decision should be
revisited against the *then-current* documentation - this is a
time-bound engineering decision, not a permanent architectural verdict.

## 5. Event-type mapping table

| YouTube `snippet.type` | Engagement `Type` (Stage 15A) | Money? | Notes |
| --- | --- | --- | --- |
| `textMessageEvent` | `engagement.TypeChatMessage` | no | Ordinary chat, existing shared type - not a new one. |
| `newSponsorEvent` | `engagement.TypeYouTubeMembership` (new) | no | "Member" is YouTube's current term for what the API/proto still calls "sponsor." `isUpgrade`/`memberLevelName` preserved via `Quantity`-adjacent fields (see §6). |
| `memberMilestoneChatEvent` | `engagement.TypeYouTubeMembershipMilestone` (new) | no | Deliberately **not** `TypeYouTubeMembership` - a milestone is an ongoing-member event, never relabeled as a fresh new membership. `memberMonth` preserved. |
| `membershipGiftingEvent` | `engagement.TypeSubscriptionGiftBatch` (reused) | no | Semantically identical to Twitch's existing gift-batch concept (a purchase of N gift memberships) - reused, not duplicated. `giftMembershipsCount` → `Quantity`. |
| `giftMembershipReceivedEvent` | `engagement.TypeGiftedSubscription` (reused) | no | Semantically identical to Twitch's existing "you received a gifted sub" concept - reused. `gifterChannelId` preserved via `ProviderExtra` (bounded, non-content). |
| `superChatEvent` | `engagement.TypeYouTubeSuperChat` (new) | **yes** | Real fiat monetary event - kept distinct from `TypeYouTubeSuperSticker`, never collapsed into Twitch's `TypeBits`. |
| `superStickerEvent` | `engagement.TypeYouTubeSuperSticker` (new) | **yes** | Distinct from Super Chat per §"Preferred semantic direction." Sticker `altText`/`stickerId`/`language` preserved as safe text/`ProviderExtra`; **no image URL invented** (§3.3 confirms none exists). |
| `fanFundingEvent` | *(unsupported/diagnostic)* | - | Legacy pre-rename name for Super Chat; still defined in the schema for backward compatibility but not documented as actively sent by current YouTube. Recognized and counted as an unsupported-type diagnostic (§8) rather than guessed at, since it is not exercised by any current official flow this document could verify. |
| `userBannedEvent` | `engagement.TypeYouTubeModeration` (new) | no | Normalized only to what the API actually proves (a ban/timeout action occurred) - never conflated with a message-deletion event (none exists). |
| `chatEndedEvent` | *(connector lifecycle signal, not an Event Bus event)* | - | Consumed internally by the connector state machine (§7) to transition to `chatEnded`. Never forwarded to the Event Bus, and never treated as equivalent to `stream.offline` - those are separate, real Twitch/YouTube-independent concepts this connector must not conflate. |
| `tombstone` | *(dropped, not forwarded)* | - | The proto/JSON docs describe this as a "silent" message (`hasDisplayContent: false`) marking that some earlier message is no longer displayable - it is not itself a deletion notification with identity of the deleted message content, and is not forwarded to the Event Bus. |
| `sponsorOnlyModeStartedEvent` / `sponsorOnlyModeEndedEvent` | *(connector diagnostic, not an Event Bus event)* | - | Channel-configuration signals, not audience engagement - recorded only as bounded connector diagnostics (§8), never published. |
| `pollEvent` | *(unsupported/diagnostic in Stage 15A)* | - | No current provider-independent "system/poll event" concept exists in `internal/domain/engagement.Type` to extend safely without a wider design decision outside this stage's scope - left as an explicit unsupported-type diagnostic rather than forced into an unrelated type. |
| `giftEvent` | *(unsupported/diagnostic, deliberately)* | - | The newer virtual-gift/"Jewels" type. Per this project's own explicit instruction: never mapped to fiat/Super Chat (jewels are not currency), and the proto's own note that a `giftEvent` message ID "may be reused to update the combo count" means ordinary Event-Bus dedup semantics do not safely apply - left unsupported rather than modeled incorrectly. |
| *(any other/future type)* | *(unsupported/diagnostic)* | - | No panic, no raw payload log - a bounded, rate-limited unsupported-type counter only (§8). |

## 6. Money model

A new provider-independent optional value on `engagement.Event`:

```go
type Money struct {
    AmountMicros int64  // integer, never float
    Currency     string // uppercase ISO-4217-style code, as reported by the provider
    DisplayAmount string // optional, provider-formatted (e.g. "$1.00") - display only, never authoritative
}
```

Populated only for `TypeYouTubeSuperChat`/`TypeYouTubeSuperSticker` from
`amountMicros`/`currency`/`amountDisplayString`. **No currency
conversion, ever** - a threshold rule's currency must match the event's
currency exactly (§ alerts). Overflow/malformed `amountMicros` values
are rejected by the normalizer (event dropped as malformed, counted, not
published) rather than silently truncated or coerced.

*(Implementation note, confirmed by codebase research: `engagement.Event`
already has dormant `Amount *float64`/`Currency string` fields reserved
for exactly this purpose, but currently populated by nothing - see
`internal/domain/alerts/capability.go`'s own `HasAmount` comment. Stage
15A's actual implementation should decide whether to populate those
existing fields directly or introduce the `Money` struct above as a
cleaner replacement during implementation - both are compatible with
this contract; the exact struct shape is an implementation detail
finalized in code, not re-litigated here.)*

## 7. Initial-history / live-cutover strategy

**No explicit history/live boundary marker exists anywhere in the
`liveChatMessages.list` response** (§3.2) - `pollingIntervalMillis` and
`nextPageToken` describe *pagination*, not a "this is where live begins"
flag. Per this project's own explicit safety requirement (never treat
provider-returned history as brand-new live engagement), Stage 15A
implements a **baseline-first cutover**: the connector's first
successful `list` call (per connector start/restart/broadcast-change)
is consumed entirely to establish a baseline `nextPageToken` and is
**never published to the Event Bus** - only messages returned by the
*second and later* calls (i.e., genuinely new since the baseline) are
normalized and published. This mirrors the safest strategy this
document's own governing task explicitly named as acceptable
("baseline the initial response before publishing live events") and
requires no provider guarantee this research could not verify.

**Reconnect** (continuation token still held in runtime memory):
resumes from the last known `nextPageToken` - no re-baseline, no
suppressed messages, since the token already excludes replayed history
by construction. **Fresh connect** (no valid continuation - process
restart, invalid/expired token, broadcast change): re-baselines exactly
as on first connect. **Backend restart never replays history** - runtime
continuation state is never persisted (per this project's existing
privacy/no-content-persistence policy, and per this task's own
instruction), so every restart is a fresh baseline by construction.

This is not a "gapless" guarantee (`pollingIntervalMillis`-spaced
polling can, in principle, miss a message if the provider's own
in-memory buffer between polls is exceeded under extreme chat volume) -
that honest limitation is recorded here rather than overclaimed.

## 8. Connector states, errors, and diagnostics (design summary)

Mirrors `internal/runtime/twitchengagement`'s `State`/`Snapshot` shape
where the underlying reality is actually analogous (disabled, blocked,
connecting, connected, reconnecting, error, stopping), and diverges
where YouTube's polling-with-broadcast-lifecycle reality genuinely
differs from Twitch's push-WebSocket reality: explicit
`waiting_for_broadcast` / `waiting_for_live_chat` states (no selected
broadcast, or a selected broadcast with no `liveChatId` yet - §3.5), and
`chat_ended` (the broadcast's chat closed, a real terminal-for-now
YouTube-specific state distinct from `reconnecting` or `error`).
Bounded, rate-limited counters for reconnects, possible-gap events (a
failed/invalid continuation forcing a re-baseline - §7), and unsupported
provider event types (§5) - never a raw payload, never chat content, per
this project's existing logging/privacy discipline
(`docs/provider-integrations/twitch-engagement.md`'s own privacy
section, reused unchanged as policy).

## 9. Reply and message-length capability (frontend-facing)

Per §3.4: **YouTube reply is unavailable** - the Reply action must never
be offered for a YouTube-authored chat item, and a YouTube send request
carrying a reply-parent must be rejected server-side, never silently
downgraded to an `@mention`-prefixed plain message. **No YouTube-
specific maximum message length is fabricated** - Stage 15A relies on
the provider's own `400 messageTextInvalid` rejection plus this
application's existing generic transport/body-size bound, exactly as
this document's own governing task requires; the existing Twitch
500-code-point limit must not be reused as if it were a YouTube fact.

## 10. Kick feasibility (Stage 15B) - see separate document

Stage 15B's own feasibility research is recorded independently in
`docs/provider-integrations/kick-engagement.md`, since it is a
different provider with a different (and, per that research, currently
blocking) architecture question. Stage 15A implements no Kick code.
