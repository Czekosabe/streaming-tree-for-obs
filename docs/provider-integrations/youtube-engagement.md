# YouTube Live Chat engagement - research contract (Stage 15A)

**Research date/time:** 2026-08-12, ~06:00-06:20 UTC.
**Correction date/time:** 2026-08-12, same day, following the original
~06:00-06:20 UTC research and the ~12:51 UTC Stage 15A closing regression
(Stage 15A transport corrective pass; see `docs/progress.md` for this
pass's own entries for exact timestamps of each step).

This document is the canonical, pre-implementation research contract for
Stage 15A: a real YouTube Live Chat engagement connector, mirroring the
role `docs/provider-integrations/twitch-engagement.md` plays for Twitch.

## 0. Correction notice (read this first)

Stage 15A originally shipped (18 commits, `4ac938a..3325e1e`, closed
2026-08-12 ~12:51 UTC) with its **inbound** receive transport implemented
as **REST polling** via `liveChatMessages.list`, on the stated conclusion
(§4a below, preserved verbatim as a historical record - **not** deleted)
that the gRPC `liveChatMessages.streamList` server-streaming transport
"has no verifiable Go path" in this environment.

**That conclusion was wrong.** It was corrected the same day, a few hours
later, in this pass, after being challenged and re-researched directly
against the live `developers.google.com` pages rather than trusted from
the original research session. The live official documentation:

- Does describe `liveChatMessages.streamList` as a real, current, gRPC
  **server-streaming** RPC intended specifically to replace repeated
  `list` polling for low-latency delivery.
- Does publish a complete, usable `stream_list.proto` (§4b.2) - copied
  verbatim into this repository at
  `apps/server/internal/provider/youtube/streamlistpb/stream_list.proto`.
- Does document Go as one of the languages `grpc.io`'s standard
  `protoc`/`protoc-gen-go`/`protoc-gen-go-grpc` toolchain generates a
  client for, alongside C++, Java, Python, and Node.js.
- Does document the production host (`dns:///youtube.googleapis.com:443`),
  TLS, and OAuth Bearer metadata authentication for the gRPC channel.

The original research's specific factual claims - "no official Go client
exists," "no official, independently-versioned `.proto` artifact exists,"
and "this development environment has no `protoc`" - were each
individually true in isolation (Google indeed does not ship a pre-built Go
package; the proto is indeed published inline in an HTML page, not a
separate download; `protoc` was indeed absent from the environment at the
time) but the **conclusion drawn from them was wrong**: none of those
three facts actually blocks a real, verifiable Go gRPC implementation,
because (a) `grpc.io`'s standard code-generation tooling **is** the
official, documented way every language (including Go) is meant to
consume this proto - there was never supposed to be a separate pre-built
Go package; (b) an inline HTML code sample is still the authoritative
proto source and can be copied and pinned exactly like any other vendored
third-party source, which is what this correction does; and (c) `protoc`
not being pre-installed is an environment/tooling gap, not a fact about
whether the transport is implementable - it was resolved in ~2 minutes by
downloading the official `protoc` release binary and the two Go plugins.
In short: the original research correctly gathered facts but drew an
overly conservative conclusion from them without actually attempting the
generation it declared infeasible.

**Corrected transport decision: gRPC server-streaming `streamList` is now
the production inbound (receive) transport.** REST (`liveChatMessages.
list`) is no longer used for receiving messages in production. REST
remains, unchanged, for `liveChatMessages.insert` (outbound sending) and
all broadcast/channel/video metadata operations (§9 unaffected).

Sections §1, §3.1/§3.2, §4, and §7 below are rewritten to reflect this
correction; §4a is kept as a clearly labeled historical record of the
original (superseded) reasoning, not deleted, per this project's
append-only research-correction discipline. Everything else in this
document (§2 auth reuse, §3.3 resource shape, §3.4 insert, §3.5 broadcast
lookup, §5 event-type mapping, §6 money model, §8 connector states, §9
reply/length capability, §10 Kick) was already transport-independent and
needed no correction.

## Sources inspected (official only)

Original research date (2026-08-12, ~06:00-06:20 UTC), all fetched
directly from `developers.google.com`:

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

Correction re-research (2026-08-12, ~14:00-15:30 UTC), same-day, fetched
again directly from the live pages (not assumed from the session above):

- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages/streamList> - re-confirmed request/response field list, confirmed `maxResults` "range 200-2000" is documented on this REST-parameter reference page but (per the proto itself, §4b.2) is the *shared REST/gRPC parameter reference page*, not proof `maxResults` does anything in the streaming RPC.
- <https://developers.google.com/youtube/v3/live/streaming-live-chat> - re-fetched in full; this time the complete `stream_list.proto` code block was extracted and diffed byte-for-byte against the syntax-highlighted HTML source (not just summarized by an intermediate model) to guarantee no field, comment, or field number was altered or invented. The full Python demo code block (channel target, credentials, request construction, per-message `Recv` loop) was extracted the same way.
- <https://developers.google.com/youtube/v3/live/docs/liveChatMessages/list> - re-fetched; confirmed the page now carries an explicit banner: *"To poll for live chat messages, use the `liveChatMessages.streamList` method. The `streamList` method pushes new messages to the client as they become available, which reduces the need for constant polling and helps to avoid exceeding your quota."* `list` itself is not marked deprecated and remains valid (kept for no other purpose in this codebase, §9).
- <https://grpc.io/docs/languages/go/basics/#generating-client-and-server-code> - confirmed the exact standard `protoc --go_out=... --go-grpc_out=...` invocation and that it requires the `protoc-gen-go`/`protoc-gen-go-grpc` plugins (both pure-Go, installable via `go install`).

## 1. Executive summary / implementation decision

**Corrected transport decision: gRPC server-streaming
`liveChatMessages.streamList` (`youtube.api.v3.
V3DataLiveChatMessageService/StreamList`) is the production inbound
transport**, per §4b. This supersedes the original REST-polling decision
(§4a, superseded but preserved as history).

No additional OAuth scope is required: the already-requested
`https://www.googleapis.com/auth/youtube.force-ssl` (`youtube.RequiredScope`,
`internal/provider/youtube/metadata.go`) is accepted for
`liveChatMessages.insert` per §3.4, and no narrower/broader scope is
documented anywhere as required specifically for `list`/`streamList`
(§3.6). The gRPC channel reuses the exact same OAuth access token, sent as
`authorization: Bearer <token>` request metadata instead of an HTTP
header - same credential, same refresh mechanism, different transport
envelope. **No reconnect or scope-upgrade flow is needed for Stage 15A.**

## 2. Authentication - reused, not re-researched

Stage 15A reuses the existing Stage 7A/7B OAuth2 Authorization Code +
PKCE + loopback-callback flow (`internal/runtime/youtubeauth`,
`internal/provider/youtube/oauth_client.go`) and the existing
`account.Service.WithFreshToken` single-flight-refresh-then-retry-once
pattern unchanged - see `docs/provider-integrations/youtube.md` for the
full, already-researched contract. Stage 15A adds **no new token store,
no new refresh mechanism, and no new scope**. The gRPC connector calls the
same `WithFreshToken` helper the REST-based connector used; only the
metadata attachment point (gRPC call options vs. an HTTP header) differs.

## 3. API contract, as researched

### 3.1 `liveChatMessages.streamList` (selected transport, corrected 2026-08-12)

`youtube.api.v3.V3DataLiveChatMessageService.StreamList` - a **gRPC
server-streaming RPC**: `rpc StreamList(LiveChatMessageListRequest)
returns (stream LiveChatMessageListResponse)`. Confirmed live and current
as of the correction re-research above. See §4b for the complete verified
contract (request/response fields, host, auth, continuation semantics,
error codes) and for exactly what was wrong in the original evaluation.

### 3.2 `liveChatMessages.list` (superseded for receiving; kept for nothing)

`GET https://www.googleapis.com/youtube/v3/liveChat/messages`. This was
Stage 15A's original inbound transport (§4a); it is **no longer used for
receiving** as of this correction. The live documentation page itself now
carries an explicit banner recommending `streamList` instead (see
"Sources inspected" above). This application does not call
`liveChatMessages.list` anywhere after this correction - it was a
receive-only method in this codebase (the connector's baseline+poll
loop); outbound sending has always been a separate method,
`liveChatMessages.insert` (§3.4), which is unaffected and remains REST.

### 3.3 `liveChatMessage` resource shape

Cross-checked between the REST JSON resource page and the proto (same
underlying schema, camelCase JSON vs. snake_case+field-number proto -
consistent on every field checked, which is why both are trusted). This
shape is transport-independent - the gRPC `LiveChatMessage` message and
the REST `liveChatMessage` resource carry the same fields; only the wire
encoding differs (protobuf field numbers vs. JSON keys).

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
ordinary dedup" guidance. This exact comment is preserved verbatim in
the vendored proto (`streamlistpb/stream_list.proto`,
`LiveChatMessage.id` field).

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
`liveChatEnded`, `404 liveChatNotFound`, `429 rateLimitExceeded`. This
method is unaffected by the transport correction - it stays REST (§9).

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
yet" from the field's presence alone (see §7). This lookup is unaffected
by the transport correction - it stays REST (§9), since `streamList`
takes a `liveChatId` as input and does not itself resolve one from a
broadcast.

### 3.6 OAuth scopes for reading

Neither the `list` nor the `streamList` reference page documents an
explicit "Authorization scopes" table (`insert`'s page does, and lists
`youtube`/`youtube.force-ssl` - §3.4). Since reading a broadcast's own
live chat is a strict subset of what `youtube.force-ssl` already grants
for broadcast/video management, and no separate/narrower/broader scope
is documented anywhere for reading, **Stage 15A requests no additional
scope** - answering research question 6/7 (§ "questions to answer")
definitively: yes, the existing scope already covers every Stage 15A
read/write operation, over either transport.

### 3.7 Quota

The general quota-cost page does not break out `liveChatMessages.list`/
`streamList`/`insert` in its shown cost table; it only confirms Live
Streaming API methods share the same **10,000-units/day default project
quota** as the rest of the YouTube Data API. No exact per-call unit cost
is claimed here since none was found in the inspected page - this is
recorded as genuinely undocumented in this snapshot rather than guessed.
The gRPC transport's own quota accounting is likewise undocumented in
exact units; the live documentation's own stated *reason* to prefer
`streamList` over `list` is precisely to reduce quota consumption versus
repeated polling (§0), which this correction takes at face value without
a further unverifiable numeric claim.

## 4a. Original (superseded) reasoning - 2026-08-12 ~06:00 UTC

**Preserved verbatim as a historical record. This reasoning is wrong -
see §0 and §4b. Do not follow it.**

> This project's own explicit instruction is to prefer the current
> official low-latency transport **if it is practical in Go**. It is not,
> in this repository, for three independent, each-individually-sufficient
> reasons:
>
> 1. **No official Go client exists.** The `streaming-live-chat` guide
>    names no Go package; it only links to `grpc.io`'s generic "how to
>    generate a client from a `.proto` with `protoc`" instructions - the
>    same instructions given for C++/Java/Node, none of which this
>    project uses either.
> 2. **No official, independently-versioned `.proto` artifact exists.**
>    The `.proto` source is published only as an inline HTML code block on
>    a documentation page, meant to be hand-copied by a developer, not
>    fetched/pinned as a versioned dependency the way this project's other
>    third-party contracts are (Go modules, `THIRD_PARTY_NOTICES.md`
>    entries with a real upstream release/commit reference).
> 3. **This development environment has no `protoc` (or `protoc-gen-go`/
>    `protoc-gen-go-grpc`) installed**, confirmed directly
>    (`which protoc` → not found). Generating correct `proto2`-syntax Go
>    bindings by hand - without a compiler to verify wire-format
>    correctness (field numbers, `oneof` handling, `optional` presence
>    semantics) - for an API that carries real monetary data (Super
>    Chat/Super Sticker amounts) is exactly the kind of unverifiable
>    guess this project's own research discipline forbids.
>
> Given all three, implementing `streamList` today would mean either (a)
> hand-writing unverifiable low-level protobuf wire encoding/decoding by
> hand in Go, or (b) introducing a third-party Go protobuf/gRPC runtime
> dependency purely to compile a hand-copied, unofficial `.proto` file
> this project cannot cite an authoritative upstream source/version for.
> Neither is acceptable under this project's "no dependency without
> genuine, source-backed necessity" discipline.
>
> **REST polling via `liveChatMessages.list` uses zero new dependencies**
> [...] If a future stage finds an official, versioned,
> independently-fetchable `.proto` artifact (or an official Go client
> ships), this decision should be revisited against the *then-current*
> documentation - this is a time-bound engineering decision, not a
> permanent architectural verdict.

## 4b. Correction: why `streamList` is implementable, and its verified contract

Each of the three original points was factually true in isolation but
did not actually block a real implementation:

1. **"No official Go client exists"** is true but irrelevant: Google
   never ships pre-built per-language client packages for this API -
   `grpc.io`'s standard `protoc` code-generation workflow **is** the
   official, intended path for every listed language, Go included. The
   `streaming-live-chat` guide explicitly lists Go alongside C++, Java,
   Python, and Node.js as a supported code-generation target
   (`https://grpc.io/docs/languages/go/basics/#generating-client-and-server-code`,
   re-confirmed in the correction re-research). This is exactly the same
   relationship every other gRPC API has with Go: there is no separate
   "official Go client" for *any* gRPC service beyond the generated code.
2. **"No official, independently-versioned `.proto` artifact exists"**
   is true but is not a blocker: an inline HTML code sample on an
   official Google documentation page is still an official source. It
   has been vendored into this repository exactly the way any other
   third-party source without a separate release/tag would be: copied
   verbatim (one import line and one `go_package` option added, both
   documented inline in the file itself - see
   `streamlistpb/stream_list.proto`'s header comment), with the fetch
   date and source URL recorded, and re-generated deterministically by a
   maintainer script (`streamlistpb/README.md`) rather than hand-written.
3. **"No `protoc` installed"** was true of the environment, not of the
   task. `protoc` (official release v29.3, from
   `github.com/protocolbuffers/protobuf`'s own GitHub releases) and the
   two required Go plugins, `protoc-gen-go`
   (`google.golang.org/protobuf/cmd/protoc-gen-go`) and
   `protoc-gen-go-grpc` (`google.golang.org/grpc/cmd/protoc-gen-go-grpc`,
   both `go install`-able, pure Go, official Google/gRPC modules), were
   installed in a few minutes. Code generation succeeded on the first
   attempt against the vendored proto, and the generated package builds
   cleanly (`go build ./internal/provider/youtube/streamlistpb/...`).
   The **generated** `.pb.go`/`_grpc.pb.go` files are checked into the
   repository (§7 of the corrective task, `streamlistpb/README.md`); a
   normal `go build`/`go test` never needs `protoc` again - only a future
   maintainer *regenerating* from a changed proto would.

### 4b.1 Verified: `stream_list.proto`, extracted byte-for-byte

The complete proto was extracted from the syntax-highlighted HTML source
of `https://developers.google.com/youtube/v3/live/streaming-live-chat`
(stripping only HTML markup, not summarizing through an intermediate
model, to guarantee no field/comment/number was altered), and diffed
visually against the rendered page. It is vendored unmodified (beyond one
added `import` and one added `go_package` option, both documented in the
file) at `apps/server/internal/provider/youtube/streamlistpb/
stream_list.proto`. Key confirmed facts from the real proto text, not
the earlier prose summary:

- `syntax = "proto2";`, `package youtube.api.v3;`.
- `service V3DataLiveChatMessageService { rpc StreamList(
  LiveChatMessageListRequest) returns (stream
  LiveChatMessageListResponse) {} }` - confirms server-streaming
  (`stream` keyword on the response only, not the request).
- `LiveChatMessageListRequest`: `live_chat_id` (string), `hl` (string),
  `profile_image_size` (uint32), `max_results` (uint32) - **the proto's
  own comment on this field reads "Not used in the streaming RPC."**
  (verbatim), `page_token` (string), `part` (repeated string).
- `LiveChatMessageListResponse`: `kind`, `etag`, `offline_at`,
  `page_info` (`PageInfo{total_results, results_per_page}`),
  `next_page_token`, `items` (repeated `LiveChatMessage`),
  `active_poll_item`. No `polling_interval_millis` field exists on the
  streaming response (that REST-only field, present on
  `liveChatMessages.list`'s JSON response, has no equivalent here,
  confirming the streaming RPC is push-driven, not poll-interval-driven).
- `LiveChatMessage`, `LiveChatMessageAuthorDetails`,
  `LiveChatMessageSnippet` (with its `TypeWrapper.Type` enum:
  `INVALID_TYPE`, `TEXT_MESSAGE_EVENT`, `TOMBSTONE`,
  `FAN_FUNDING_EVENT`, `CHAT_ENDED_EVENT`,
  `SPONSOR_ONLY_MODE_STARTED_EVENT`, `SPONSOR_ONLY_MODE_ENDED_EVENT`,
  `NEW_SPONSOR_EVENT`, `MEMBER_MILESTONE_CHAT_EVENT`,
  `MEMBERSHIP_GIFTING_EVENT`, `GIFT_MEMBERSHIP_RECEIVED_EVENT`,
  `USER_BANNED_EVENT`, `SUPER_CHAT_EVENT`, `SUPER_STICKER_EVENT`,
  `POLL_EVENT`, `GIFT_EVENT`) and a `oneof displayed_content` selecting
  exactly one details message per type - the same set §3.3/§5 already
  document from the REST side, now confirmed byte-for-byte identical on
  the gRPC side (same field semantics, different wire encoding).
- `LiveChatMessage.id`'s own doc comment: *"Note: For giftEvents, the
  same ID may be reused to update the combo count."* - verbatim, same
  caveat as §3.3 already recorded from the REST-side proto summary, now
  confirmed from the real proto text itself.
- `LiveChatGiftDetails` (the `giftEvent` payload): `gift_name`,
  `gift_duration` (`google.protobuf.Duration`), `jewels_amount` (int32),
  `gift_url`, `alt_text`, `language`, `has_visual_effect` (bool),
  `combo_count` (int32) - confirms `giftEvent` carries an explicit
  `combo_count` field precisely because the same message ID is reused to
  update it live (the doc comment above), not a bug or edge case.

### 4b.2 Verified: production wiring (host, TLS, auth)

From the same page's Python demo code block, extracted the same
byte-for-byte way:

- Channel target: `"dns:///youtube.googleapis.com:443"`, opened with
  `grpc.secure_channel(target, grpc.ssl_channel_credentials())` - i.e.
  standard TLS, the `dns:///` scheme being gRPC's normal DNS-resolving
  target syntax (not a special YouTube requirement). This application's
  Go client uses `google.golang.org/grpc`'s equivalent
  (`grpc.NewClient` with `credentials.NewTLS(&tls.Config{})`) against the
  same host.
- Auth metadata: exactly two documented options, `("x-goog-api-key",
  API_KEY)` or `("authorization", "Bearer " + OAUTH_TOKEN)` - this
  application always uses the second (Bearer OAuth token), the same
  credential `internal/domain/account.Service.WithFreshToken` already
  manages for every other YouTube call, attached as gRPC per-call
  metadata instead of an HTTP header.
- Request construction in the demo sets `part=["snippet"]`,
  `live_chat_id`, `max_results=20`, `page_token=next_page_token`, then
  iterates `stub.StreamList(request, metadata=metadata)`, reading
  `response.next_page_token` after each response to resume. This
  application requests `part=["id","snippet","authorDetails"]` (the same
  three parts the REST connector already requested - `id` for
  `DedupeKey`/`ProviderEventID`, `snippet` for content, `authorDetails`
  for identity) and does not set `max_results`, since the proto's own
  comment (§4b.1) states it is unused for this RPC.

### 4b.3 Verified: error semantics

The `streamList` reference page documents `PERMISSION_DENIED` and
`INVALID_ARGUMENT` as gRPC status codes this RPC can return. No further
codes, and no structured `google.rpc.ErrorInfo`/reason-string detail
analogous to the REST error envelope's `errors[].reason` (§3.2's
`classifyAPIError`), are documented for this RPC anywhere in the
inspected pages. Per this project's own "do not guess provider semantics
for an undocumented code" discipline, the Go connector (§7,
`internal/runtime/youtubeengagement`):

- Treats `PERMISSION_DENIED` and `INVALID_ARGUMENT` as the two
  *documented* outcomes, mapped to distinct, honest connector states
  (`error` for `PERMISSION_DENIED` - not something a token refresh or
  retry fixes; a re-resolve-and-retry via `waiting_for_live_chat` for
  `INVALID_ARGUMENT`, treated conservatively as "this liveChatId is no
  longer valid," the closest documented REST analogue being
  `liveChatNotFound`/`liveChatDisabled`, §3.2's superseded but
  structurally-identical handling). This is an explicit, documented
  judgment call, not a confirmed provider guarantee - recorded here
  exactly as this project's own discipline requires for an undocumented
  mapping.
- Additionally handles the standard gRPC codes every client must defend
  against regardless of per-RPC documentation - `UNAUTHENTICATED`
  (token-refresh-and-retry, same as REST 401), `UNAVAILABLE` and
  `DEADLINE_EXCEEDED` (transient, retried with the existing bounded
  exponential backoff), `RESOURCE_EXHAUSTED` (treated like the REST rate
  limit sentinel), and `CANCELED` (this application's own context
  cancellation, never a provider error).
- Never exposes a raw gRPC status/detail string to the frontend - only
  this connector's own existing stable state/error-code vocabulary (§8),
  unchanged by the transport correction.

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
| `giftEvent` | *(unsupported/diagnostic, deliberately)* | - | The newer virtual-gift/"Jewels" type. Per this project's own explicit instruction: never mapped to fiat/Super Chat (jewels are not currency), and the proto's own note that a `giftEvent` message ID "may be reused to update the combo count" means ordinary Event-Bus dedup semantics do not safely apply - left unsupported rather than modeled incorrectly. Confirmed unchanged by this correction: the gRPC `LiveChatGiftDetails.combo_count` field (§4b.1) is the same reused-ID mechanism this row already accounted for. |
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
`amountMicros`/`currency`/`amountDisplayString` (REST JSON naming) /
`amount_micros`/`currency`/`amount_display_string` (proto naming, same
fields, same semantics, confirmed identical between §3.3 and §4b.1).
**No currency conversion, ever** - a threshold rule's currency must match
the event's currency exactly (§ alerts). Overflow/malformed
`amountMicros` values are rejected by the normalizer (event dropped as
malformed, counted, not published) rather than silently truncated or
coerced. Implemented as `internal/domain/engagement.Money` (integer
micros, as designed here) - unaffected by the transport correction.

## 7. Initial-history / live-cutover strategy (corrected: baseline vs. reconnect)

**No explicit history/live boundary marker exists anywhere in the
`streamList` response** (§4b.1) - like the superseded REST response,
`next_page_token` describes *continuation*, not a "this is where live
begins" flag. Per this project's own explicit safety requirement (never
treat provider-returned history as brand-new live engagement), Stage 15A
implements a **baseline-first cutover for a genuinely fresh stream**: the
connector's first response on a stream opened **without** a valid
previously-held continuation token (per connector start/restart/
broadcast-change, or after an invalid/stale continuation - see below) is
consumed entirely to establish a baseline `next_page_token` and is
**never published to the Event Bus** - only responses received *after*
that baseline (i.e., genuinely new since the baseline) are normalized and
published.

**Corrected distinction (not present in the original REST-polling
design, because REST's request/response shape made it implicit): a
reconnect that already holds a valid `next_page_token` from the same
still-live stream must NOT re-baseline its first resumed response.**
Discarding a reconnect's first response merely because it is "the first
response since dialing" would silently drop real, already-continuing
chat on every transient reconnect - the connector distinguishes these two
cases explicitly by whether it is resuming with a previously-captured
token (continuation: publish immediately) or starting fresh with no
usable token (baseline: suppress the first response only). This
distinction is exercised directly by dedicated tests (§28-30 of the
corrective task) precisely because it is easy to get backwards.

**Stale/invalid continuation**: if a reconnect attempt using the last
known `next_page_token` is rejected by the provider (mapped from
`INVALID_ARGUMENT`, §4b.3 - the closest documented signal, since no
distinct "your continuation token expired" gRPC code is documented), the
connector discards that token, marks a possible-gap diagnostic (§8),
and opens a fresh stream that re-baselines exactly like a first connect -
never replaying whatever backlog a fresh stream's first response might
contain as new live events.

**Backend restart never replays history** - runtime continuation state
(`next_page_token`) is held only in process memory, never persisted to
SQLite (per this project's existing privacy/no-content-persistence policy
and per this task's own explicit instruction), so every process restart
is a fresh baseline by construction, exactly as the superseded REST
design already established.

This is not a "gapless" guarantee: a stream loss followed by a successful
reconnect with a still-valid token is gapless by the provider's own
continuation contract, but a stream loss whose continuation token is
later rejected as stale necessarily loses whatever was said between the
disconnect and the reconnect (recorded as a possible-gap diagnostic, never
silently hidden) - an honest limitation, not a regression from the
superseded REST design's own equivalent limitation (a `list` call
returning a `nextPageToken` Google's servers no longer recognized).

## 8. Connector states, errors, and diagnostics (design summary)

Preserves `internal/runtime/twitchengagement`'s `State`/`Snapshot` shape
where the underlying reality is actually analogous (disabled, blocked,
connecting, connected, reconnecting, error, stopping), and the
YouTube-specific states this document's original research already
established for the broadcast-lifecycle reality both transports share:
explicit `waiting_for_broadcast` / `waiting_for_live_chat` states (no
selected broadcast, or a selected broadcast with no `liveChatId` yet -
§3.5), and `chat_ended` (the broadcast's chat closed - conveyed by a
`chatEndedEvent` message or the response's own `offline_at`/`offlineAt`
field on either transport, a real terminal-for-now YouTube-specific state
distinct from `reconnecting` or `error`). **No new operator-facing state
was introduced by the transport correction** (§31 of the corrective
task) - gRPC channel state, the continuation token, and HTTP/2 internals
are never exposed past this connector's own boundary, exactly as the
REST design's continuation token and poll timer never were.

Bounded, rate-limited counters for reconnects, possible-gap events (a
failed/invalid continuation forcing a re-baseline - §7), and unsupported
provider event types (§5) - never a raw payload, never chat content, and
now additionally never gRPC metadata, the OAuth bearer token, or a
protobuf message dump (`%+v` on a generated message is never logged) -
per this project's existing logging/privacy discipline
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
`liveChatMessages.insert` stays REST (§3.4) - the transport correction
only ever concerned the inbound receive method.

## 10. Kick feasibility (Stage 15B) - see separate document

Stage 15B's own feasibility research is recorded independently in
`docs/provider-integrations/kick-engagement.md`, since it is a
different provider with a different (and, per that research, currently
blocking) architecture question. Stage 15A implements no Kick code.
Re-checked briefly during this correction (§34 of the corrective task) to
confirm the YouTube correction does not accidentally invalidate Stage
15B's own result - see `docs/provider-integrations/kick-engagement.md`
for that re-check's own record; it does not change Stage 15B's
feasibility-gated status.
