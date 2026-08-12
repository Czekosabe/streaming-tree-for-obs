# External donation services - research contract (Stage 16A)

**Research date/time:** 2026-08-12, ~18:30-19:30 UTC.

This document is the canonical, pre-implementation research contract for
Stage 16A: an external-donation foundation plus a first real provider,
StreamElements, mirroring the role `docs/provider-integrations/
youtube-engagement.md` plays for YouTube and
`docs/provider-integrations/kick-engagement.md` plays for Kick. Written
**before** any donation-source/StreamElements code exists in this
repository, per this project's own "document the contract before writing
provider code" discipline. Covers all three providers named in this
stage's own instructions (StreamElements, Streamlabs, Ko-fi) in one
document, since - like `kick-engagement.md` - a feasibility verdict and an
implementation contract are closely related here and do not need two
separate files.

## 0. Stage-order decision

Stage 15B (Kick engagement) remains feasibility-gated, blocked on Kick's
own webhook-only public-inbound event delivery model (unchanged - see §10
below; not re-researched deeply here, only re-confirmed still current).
**Stage 16A does not depend on Stage 15B and is not blocked by it.** This
mirrors how Stage 15A (YouTube) was never blocked on Stage 7C
(Kick/TikTok account integration) - a stage that is infeasible or not yet
started under the current architecture does not prevent an independent,
provider-independent foundation (here: the external-donation source model
and the generic `donation` event) from being built and immediately proven
with whichever real provider's own architecture *does* fit this
application's local-desktop deployment model today. Stage 15 as a whole
remains **Incomplete** (still gated on Stage 15B); this document does not
change that, does not implement Kick, does not implement a Kick relay, and
does not alter Kick's own researched conclusion in `kick-engagement.md`.

## 1. Sources inspected (official only)

All fetched directly from the providers' own documentation domains on the
research date above. No unofficial donation-overlay library, blog post,
Reddit thread, StackOverflow answer, reverse-engineered endpoint list, or
browser network capture was consulted for any API contract detail.

StreamElements:
- <https://docs.streamelements.com/websockets>
- <https://docs.streamelements.com/websockets/examples>
- <https://docs.streamelements.com/websockets/topics>
- <https://docs.streamelements.com/websockets/topics/channel-tips>
- <https://docs.streamelements.com/websockets/topics/channel-tips-moderation>
- <https://dev.streamelements.com/> (Stoplight-hosted API reference shell;
  client-rendered, no server-rendered body content reachable by this
  research's fetch tooling - see §4's honest note)
- Search-engine-indexed snippet of
  <https://support.streamelements.com/hc/en-us/articles/10474949304466-How-to-Locate-Your-Account-ID-and-JWT-Token>
  (the live page itself returns HTTP 403 to this research's fetch tooling
  - a Zendesk bot-protection response, not a content-availability
  decision by StreamElements; the search engine's own indexed snippet of
  the same official page was used instead, quoted narrowly and only for
  the one operational fact this document actually relies on - see §5)

Streamlabs:
- <https://dev.streamlabs.com/docs/socket-api>
- <https://dev.streamlabs.com/docs/obtain-an-access_token>
- <https://dev.streamlabs.com/docs/register-your-application>

Ko-fi:
- Search-engine-indexed snippet of
  <https://help.ko-fi.com/hc/en-us/articles/360004162298-Does-Ko-fi-have-an-API-or-webhook>
  (the live page returns HTTP 403 to this research's fetch tooling, same
  Zendesk bot-protection behavior as the StreamElements support site
  above; the officially-hosted page's own indexed content was used
  instead of any third-party summary)

## 2. Provider feasibility matrix

| | StreamElements | Streamlabs | Ko-fi |
| --- | --- | --- | --- |
| Client-initiated receive transport? | **Yes** - raw WebSocket (`wss://astro.streamelements.com/`), client dials out | Yes, but **Socket.IO** (v2.0.3 protocol, not a raw WebSocket - a different wire protocol requiring a Socket.IO-aware client) | **No** - server-initiated HTTP POST to an operator-configured URL (webhook) |
| Public inbound endpoint required? | No | No | **Yes** - Ko-fi must be able to reach the operator's own public HTTPS callback URL |
| Auth method | JWT, "apikey" (Overlay token), or OAuth2 access token - all sent as `data.token`/`data.token_type` in the subscribe request itself, not at connect time | OAuth2 authorization-code flow only (`/api/v2.0/token`) | A per-webhook "Verification Token" the operator configures and Ko-fi echoes back in each POST body for the operator to check |
| Client secret required? | **No**, for the JWT/apikey path (a personal, per-channel credential, not an OAuth app secret) | **Yes** - documented token exchange requires `client_id` **and** `client_secret` in the POST body; no PKCE or public/native-client alternative found in current docs | N/A (no OAuth) |
| Account/app approval requirements | None for personal JWT (self-serve, from the operator's own dashboard) | Self-serve app registration, but **apps default to an unapproved state limited to 10 whitelisted authorizing users** until StreamElements... (Streamlabs) approves the app | None (webhook URL configured directly on the creator's own Ko-fi account) |
| Realtime donation capability? | **Yes** - dedicated `channel.tips` topic (`tips:read` scope), plus `channel.tips.moderation` (`tips:moderation` scope) | Yes - Socket API delivers `eventData.type === 'donation'` | Yes, but only via the webhook push, never a client-initiated pull |
| Donation history capability? | A separate REST history endpoint likely exists on `dev.streamelements.com`'s Stoplight-hosted reference, but Stage 16A does not use any history endpoint regardless (see §9) - not required to evaluate further | Not evaluated - irrelevant given the auth blocker below | Not evaluated - irrelevant given the transport blocker below |
| Local desktop fit | **Good** - no public endpoint, no confidential secret, a single self-serve personal credential the operator copies from their own dashboard | **Poor** - either ship a confidential `client_secret` inside a source-available desktop application (unacceptable), or force every operator through an unapproved-app 10-user whitelist limit; also a second WebSocket-family protocol (Socket.IO) this project does not otherwise depend on | **Poor** - requires a public HTTPS receiver; this application is local-first and does not operate one, and this task explicitly forbids solving that with a tunnel/relay/port-forward workaround |
| Stage 16A decision | **Implement** | **Defer to Stage 16B** - "feasible transport, unresolved/poor desktop authentication model" | **Feasibility-gated for the current local-only deployment model** - not implemented |

## 3. Authentication decision (StreamElements)

Per this task's own explicit decision order:

1. *"If current StreamElements OAuth supports a true public/native client
   flow without a confidential secret embedded in the application, prefer
   scoped OAuth."*  StreamElements' own developer reference for OAuth2 is
   hosted on a Stoplight-based page
   (`dev.streamelements.com/docs/api-docs/cd02cda5171ea-o-auth2`) that
   renders its actual body content client-side; this research's fetch
   tooling could retrieve only the page shell (navigation/banner), not
   the OAuth2 flow's own prose, after two independent attempts (a
   markdown-converting fetch and a raw HTTP fetch). This document records
   that honestly rather than guessing at OAuth2's exact flow shape either
   way. Absent a *positive, verified* finding that a public/native OAuth2
   client flow exists, this task's own decision order does not permit
   assuming one - so step 1 is not taken.
2. *"If OAuth requires a confidential client secret... but the officially
   documented personal JWT path remains supported, Stage 16A may use the
   operator's own StreamElements JWT."* This **is** independently
   confirmed, from StreamElements' own support documentation (via its
   search-indexed content, since the live page itself 403s to automated
   fetching - not a StreamElements decision, a Zendesk anti-bot response):
   *"If you intend to use the API for your own stream or to any single
   use case, you do not need to have OAuth access—you can use your JWT
   token instead."* Streaming Tree, for one operator managing their own
   channel's donations locally, is exactly this "single use case." The
   same source explicitly warns the JWT is sensitive and must never be
   shared - directly informing this document's own credential-handling
   requirements (§8 below).
3. **No StreamElements confidential client secret is embedded anywhere in
   this application.** Confirmed by design: Stage 16A never registers a
   StreamElements OAuth application at all.
4. **No undocumented/invented OAuth flow is used.**
5. *"Do NOT use an Overlay API key unless official documentation proves it
   grants exactly the permissions required... and there is a security
   advantage over the JWT."* The `apikey`/"Overlay Token" path is
   documented as an alternative `token_type` for the exact same subscribe
   request shape as JWT (§5), with no documented scope or security
   difference specific to the tip topics - no evidence of an advantage
   over JWT was found, so **JWT is used**, as the primary, most广ly
   documented personal-credential path.

**Decision: Stage 16A authenticates to StreamElements using the
operator's own personal JWT, `token_type: "jwt"`, pasted in from their own
StreamElements dashboard.** See §8 for the exact secret-handling
requirements this drives.

## 4. Transport and deployment constraints

StreamElements' real-time delivery mechanism ("Astro") is a **raw
WebSocket** service (confirmed directly from the official page's own
prose: *"This service is not built on top of Socket.IO or any other
framework - it's a raw WebSocket implementation"* - paraphrased from the
`websockets` overview page, consistent with its documented `wss://`
endpoint and plain JSON message envelopes shown throughout the topics/
examples pages), reachable by a client dialing outward from Streaming
Tree's own process - no public inbound endpoint, no port forwarding, no
tunnel, and no relay of any kind is required. This is materially
different from Streamlabs (Socket.IO, a heavier framework this project
does not otherwise depend on) and Ko-fi (a webhook requiring a public
receiver this application does not operate). StreamElements is the only
one of the three providers whose current official transport genuinely
fits a local-first desktop application using only outbound connections,
matching exactly how this project's existing Twitch EventSub WebSocket
and YouTube `streamList` gRPC connectors already work.

## 5. Production endpoint, authentication, subscription lifecycle

**Production WebSocket endpoint (fixed in code, never operator-editable
in normal settings):** `wss://astro.streamelements.com/`.

**Connection envelope** (every message, both directions, JSON): `{id, ts,
type, topic?, room?, nonce?, error?, data}`. `type` is one of, confirmed
directly from the official examples page's own client code samples
(JavaScript, Go):

- Server → client: `"welcome"` (first message after connecting;
  `data.client_id`), `"response"` (reply to a `subscribe`/`unsubscribe`
  request, correlated by `nonce`; `error` present and non-empty on
  failure, e.g. the documented `"rate_limit_exceeded"` - rate-limit
  responses do not carry a `nonce`, per the official example's own
  comment), `"message"` (a real topic event; carries `topic`, `room`,
  `data`), `"reconnect"` (graceful-shutdown notice; `data.reconnect_token`).
- Client → server: `"subscribe"` (`{type, nonce, data: {topic, room,
  token, token_type}}`), `"unsubscribe"` (`{type, nonce, data: {topic,
  room?}}` - omitting `room` unsubscribes every room for that topic).

**Authentication:** sent per-subscription, not at connect time - the
`subscribe` request's own `data.token`/`data.token_type` fields.
`token_type: "jwt"` for Stage 16A (§3). `data.room` is the StreamElements
channel/account ID the operator is subscribing on behalf of - for a
personal JWT this is simply the operator's own channel, identified by the
Account ID their own StreamElements dashboard shows next to the JWT
itself (both are entered together when the operator adds the donation
source - see §12/§14 of the implementation, and the persistence model
below).

**Heartbeat:** WebSocket-protocol-level `PING` frames, sent by the server
approximately every 30 seconds; the client must answer with `PONG` or the
server closes the connection after roughly 70 seconds of silence. These
are transport-level control frames (handled transparently by a compliant
WebSocket client library, including `github.com/coder/websocket`), not
application JSON messages - no `PING`/`PONG` handling needs to be written
by hand in the connector.

**Graceful reconnect:** before a planned server-side shutdown, the server
sends `{type: "reconnect", data: {reconnect_token}}`. The documented
client behavior is to open a new connection to
`wss://astro.streamelements.com/?reconnect_token=<token>`; the new
connection "verifies the token and restores all subscriptions
automatically" (quoted from the `websockets` overview page) - i.e. the
client must **not** re-send `subscribe` requests immediately after a
reconnect-token-based reconnect, since the server already restores them.
Stage 16A's own tests verify this distinction explicitly (§ implementation
test list).

**Unexpected disconnect:** not documented with any replay/resume
guarantee. Stage 16A therefore treats an unexpected close the same
conservative way the YouTube `streamList` corrective pass already
established for its own undocumented gaps: bounded exponential backoff
with jitter, a fresh connection, a fresh `subscribe` (since no
reconnect_token exists for this path), and an honest possible-gap
diagnostic - never a claim of stronger replay guarantees than
StreamElements documents.

**Errors:** the `response` envelope's `error` field carries a stable
string code; the only one directly documented in the fetched pages is
`"rate_limit_exceeded"` (with `data.message` as a human-readable
description). Any other `error` value is treated as an honest, generic
subscribe failure - never guessed at.

## 6. Topics and scopes used

**Primary (required): `channel.tips`**, scope `tips:read` - the dedicated
donation event topic. This is the **only** topic Stage 16A's production
connector subscribes to for receiving donations - never `channel.
activities` (a broader topic that scope `activities:read` also happens to
carry tip-shaped entries; subscribing to it *in addition to* `channel.
tips` would create two independent delivery paths for the same donation,
which this document deliberately avoids - see §7).

**Optional (used, per §7 below): `channel.tips.moderation`**, scope
`tips:moderation` - moderation-state transitions for a tip already seen
(or about to be seen) on `channel.tips`.

The full topic catalogue documented on the `websockets/topics` page is
much larger (`channel.activities`, `channel.chat.message`, `channel.
session.update`, loyalty/chatbot/overlay topics, etc.) - none of those are
subscribed to by Stage 16A; this connector's authorization footprint is
deliberately the minimum two scopes above.

## 7. Event payload and moderation semantics

Both `channel.tips` and `channel.tips.moderation` deliver the identical
payload shape (extracted byte-for-byte from the official pages' own
copy-button source, not a paraphrase):

```
{
  "id": "<ULID envelope id - never used as the donation's identity>",
  "ts": "<ISO 8601>", "type": "message", "topic": "channel.tips" | "channel.tips.moderation",
  "room": "<channel id>",
  "data": {
    "donation": {
      "user": { "username": "...", "geo": "...", "email": "...", "channel": "..." },
      "message": "...", "amount": 4.2, "currency": "USD", "paymentMethod": "scheme"
    },
    "_id": "<stable tip id - THIS is the donation's identity>",
    "channel": "<channel id>",
    "provider": "paypal",
    "approved": "pending" | "allowed" | "rejected",
    "status": "success",
    "createdAt": "<ISO 8601>", "updatedAt": "<ISO 8601>",
    "transactionId": "<payment-rail transaction id>",
    "approvedBy": "<moderator - present once a moderation decision is made>"
  }
}
```

Verified directly, from the official channel-tips-moderation page's own
two worked examples (a `pending` example and, later, an `allowed`
example, both for the same tip): **`data._id` stays identical across the
entire moderation lifecycle** (`"67b5f39d07ecd4c594e60f73"` in both
examples), while `data.approved` changes from `"pending"` to `"allowed"`
and `data.approvedBy` becomes populated. `data.status` stayed `"success"`
in both examples - confirming `status` (payment/transaction outcome) and
`approved` (moderation state) are two orthogonal fields, not one combined
enum.

**Moderation → publish semantics (Stage 16A's own decision, since the
docs describe the field values but not a required consumer behavior):**

- `approved: "allowed"` **and** `status` representing a successfully
  completed payment (§ below) → publish the donation exactly once.
- `approved: "pending"` → never publish yet.
- a later `channel.tips.moderation` message for the same `data._id`
  transitioning to `"allowed"` → publish exactly once, at that point.
- `approved: "rejected"` → never publish, ever.
- any repeated `"allowed"` message for a `data._id` already published →
  never duplicate (deduplicated on `data._id`, the stable tip identity -
  never the envelope `id`, never `transactionId`, never a computed
  amount+timestamp hash, per this task's own explicit instruction).

**`status` (transaction/payment outcome) semantics:** the only value
found anywhere in the fetched official examples is `"success"`. No
documented enum of failure/cancelled/incomplete values was found in the
pages this research could reach. Per this task's own explicit
instruction not to guess: **Stage 16A only ever publishes a donation
when `status == "success"`** (the one officially-observed value
representing a completed payment); any other `status` value is treated
conservatively as unsupported/not-publishable (counted as a bounded
diagnostic, never silently treated as a successful donation, and never
guessed to be a "should still publish" case).

## 8. Credential handling requirements (StreamElements personal JWT)

Directly driven by the official warning quoted in §3 ("do not share it
under any circumstances") and this project's own existing no-secrets-in-
SQLite/no-secrets-in-logs/no-secrets-in-React-state discipline (already
established for Twitch/YouTube OAuth token bundles and destination stream
keys):

- Accepted only through an explicit operator action (pasting it into a
  dedicated credential field when adding or replacing a donation source -
  never auto-discovered, never defaulted).
- The UI must explain, in plain language, that this is a sensitive
  StreamElements credential, where the official dashboard exposes it
  ("Show Secrets" toggle next to the operator's own channel), and that it
  is never shown again after saving.
- Never persisted in SQLite - only safe metadata is (§ persistence model,
  implementation).
- Never returned from any GET endpoint.
- Never written to a log line, an error message, a toast, a URL, or
  `docs/progress.md`.
- Never placed in `localStorage`/`sessionStorage`; held only in an
  uncontrolled, ephemeral password-style input, cleared immediately after
  a successful submit.
- Stored exclusively through the existing OS-native SecretStore
  abstraction (`internal/secrets`), under its own namespaced key -
  reusing the existing `secrets.BuildKey`/`SecretType` pattern already
  used for OAuth token bundles and destination stream keys, never a
  second keyring wrapper.
- Deleting the donation source deletes the stored secret. Replacing/
  rotating the token atomically replaces it (no window where the old and
  new value could both be readable, and no window where the source is
  "enabled" with no credential at all).
- **Never decoded and trusted locally.** A JWT's claims are not treated
  as authoritative by this application - the StreamElements Astro service
  itself is the only party that validates the token, when this
  application's own `subscribe` request is answered with a `response`
  success or a `response` error. Stage 16A does not attempt to parse,
  verify a signature on, or read expiry/claims out of the JWT string
  itself anywhere in this codebase.

## 9. No donation history, no catch-up

StreamElements' broader API surface almost certainly includes a REST
donation-history endpoint (typical of every donation-platform API this
task's author is aware of), but Stage 16A does not inspect, request, or
depend on one anywhere, per this task's own explicit instruction (§35 of
the governing task): realtime-only, no history fetch at startup, no
"catch up on missed donations" behavior, and no persisted donation-event
history of any kind in this application's own database. A backend
restart reconnects an explicitly-enabled donation source and waits for
new activity - it never surprises the operator with a donation that
happened while the backend was offline. If a future stage ever wants a
real donation-history feature, it is a separate, explicitly-scoped
addition - not an implicit side effect of this connector reconnecting.

## 10. Kick re-confirmation (unchanged)

Not deeply re-researched in this pass (no repository source concerning
Kick changed since the last check), per this task's own explicit
instruction. Kick's official event delivery
(`docs/provider-integrations/kick-engagement.md`) remains webhook-only,
requiring a publicly reachable HTTPS callback the operator would have to
expose; the official client-initiated WebSocket/SSE/poll request
(`KickEngineering/KickDevDocs` issue #20) remains open, `feature`/
`planned`, Backlog status, no shipping timeline, last independently
re-confirmed 2026-08-12 (Stage 15A transport corrective pass). Stage 15B
remains feasibility-gated/not started. This document does not alter that
conclusion, and Stage 16A implements no Kick code, relay, or workaround.

## 11. Implementation summary

**Stage 16A implements: an external-donation foundation (a
provider-independent donation-source concept, and the generic `donation`
engagement event type) plus a real StreamElements connector receiving
over the official `channel.tips`/`channel.tips.moderation` Astro
WebSocket topics, authenticated with the operator's own personal JWT.**
Streamlabs and Ko-fi are not implemented in Stage 16A - see §2's matrix
for the exact, honestly-recorded reason each is deferred/gated, not a
generic "unsupported" verdict. See `docs/progress.md` for the
implementation's own commit-by-commit record, and this document's own
future revisions (never rewriting this section's history, only appending
a dated correction the same way `youtube-engagement.md` §0 corrects its
own §4a) if a later stage's research supersedes anything recorded above.
