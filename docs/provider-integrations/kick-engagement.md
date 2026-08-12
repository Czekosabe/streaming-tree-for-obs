# Kick engagement - feasibility research (Stage 15B gate)

**Research date/time:** 2026-08-12, ~06:10-06:20 UTC.

**Status: research/feasibility only. No Kick provider code exists in this
repository, and none is added by this document.** This is the Stage 15B
gate this project's own roadmap requires be resolved *before* any Kick
implementation work is attempted - see `docs/engagement-architecture.md`
§16/§18 and `README.md`'s roadmap table (Stage 15 row).

## Sources inspected (official only)

All of the following were fetched directly from `docs.kick.com` on the
research date above. No unofficial Kick endpoint list, scraping project,
reverse-engineered WebSocket capture, browser network trace, or
third-party blog/tutorial was consulted for any part of this document.

- <https://docs.kick.com> - documentation homepage.
- <https://docs.kick.com/llms.txt> - the documentation's own machine-
  readable index, used to enumerate every real sub-page rather than
  guessing URLs.
- <https://docs.kick.com/getting-started/kick-apps-setup.md>
- <https://docs.kick.com/getting-started/generating-tokens-oauth2-flow.md>
- <https://docs.kick.com/getting-started/scopes.md>
- <https://docs.kick.com/events/introduction.md>
- <https://docs.kick.com/events/webhook-security.md>
- <https://docs.kick.com/events/subscribe-to-events.md>
- <https://docs.kick.com/events/event-types.md>
- <https://docs.kick.com/apis/chat.md>
- <https://docs.kick.com/apis/public-key.md>

The official `KickEngineering/KickDevDocs` GitHub repository is the
source `docs.kick.com` itself is built from (per the homepage's own
"Contributing" link) - no separate community-reported operational
evidence beyond the rendered documentation was found necessary to reach
the conclusion below, since the documentation's own API contract text is
unambiguous on the one question that actually decides this gate (§2).

## 1. OAuth model

Kick's Authorization Grant flow requires **both** a `client_secret`
*and* PKCE (`code_challenge`/`code_challenge_method: S256`) on the same
flow - `generating-tokens-oauth2-flow.md` lists `client_secret` as a
required parameter on both the authorization-grant token exchange and
the app-access-token endpoint. This is a real point of friction for a
purely local, source-available desktop application (a `client_secret`
compiled into or shipped alongside a downloadable app is not
meaningfully confidential), but it is not by itself a hard blocker - the
existing Twitch/YouTube integrations in this project already handle an
operator-supplied Client ID via Stage 5/7A's own connected-account
model, and a similar operator-supplied-credential pattern could in
principle extend to a Kick `client_secret` if Stage 15B is ever
attempted. **This is a secondary friction point, not the deciding
factor below.**

Redirect URIs must be pre-registered; `http://localhost/...` is
explicitly supported and recommended for local development, with a
documented workaround for a `127.0.0.1` NextJS-side bug. This part of
the flow is compatible with this application's existing loopback-
callback OAuth pattern (Stage 7A/7B).

## 2. Event/webhook transport - the actual gate

This is the determining question, and the official documentation is
unambiguous:

- `events/introduction.md`: *"With webhooks, you can receive instant
  data about actions like follows, subscriptions, gifted subscriptions,
  and chat messages directly to your application."* Setting up a
  subscription requires the developer to *"Enter a publicly accessible
  URL in the textbox. This is where Kick will send POST requests
  containing event payloads."*
- `events/subscribe-to-events.md`: the API-driven subscription endpoint
  (`POST /public/v1/events/subscriptions`) accepts a `method` field
  whose documented value is the enum `webhook` - no alternative
  transport value (`websocket`, `sse`, `poll`, or similar) is documented
  anywhere in the current API reference.
- `apis/chat.md`: there is **no** endpoint to list/poll/read chat
  messages at all. The chat API surface is send-only
  (`POST /public/v1/chat`); the documentation states chat *reading* is
  available only "via event subscriptions" - i.e., only via the webhook
  transport above. Unlike YouTube (which has both a push-oriented
  `streamList` *and* a plain-REST-polling `liveChatMessages.list`
  fallback - see `docs/provider-integrations/youtube-engagement.md`
  §3), **Kick currently provides no non-webhook way to receive chat at
  all.**

**Conclusion: current official Kick events are webhook-only, and
require a publicly (internet-)reachable callback URL configured for the
developer's application. There is no documented client-initiated
WebSocket, SSE, or long-poll alternative, and no REST polling fallback
for chat specifically.**

This is a hard architecture gate for Streaming Tree for OBS, which is
explicitly a local-only desktop application with no server-side
infrastructure of its own (`docs/project-overview.md`, `README.md`) and
never opens an inbound network port to the public internet. Per this
task's own explicit instruction, this is **not** worked around by:

- scraping Kick's website or any undocumented endpoint,
- an undocumented Pusher/WebSocket connection (webhook-security.md
  confirms Kick's own webhook delivery is a signed HTTPS POST, not a
  Pusher-based push the client could instead subscribe to directly),
- requiring the operator to expose a port, configure UPnP, or run
  router-level port forwarding as a product requirement,
- bundling or auto-launching a tunnelling client (ngrok/cloudflared or
  equivalent) as part of the product,
- inventing any Streaming-Tree-operated relay/server infrastructure -
  this project has none today and adding one would be a fundamental,
  undocumented architecture change far outside this task's scope.

**Stage 15B therefore remains feasibility-gated: the current local-only
application has no public webhook receiver, and none is being added by
this task.** If a future official Kick transport (a documented client-
initiated streaming/polling option) ships, this gate should be
re-researched against the *then-current* official documentation before
any implementation begins - the conclusion above is time-bound to what
`docs.kick.com` states as of the research date, not a permanent
architectural verdict about Kick generally.

## 3. Other findings, for completeness (not gating, recorded for a future re-check)

- **Webhook signature verification model**
  (`events/webhook-security.md`): the `Kick-Event-Signature` header
  carries an RSA/SHA-256/PKCS#1v1.5 signature over
  `{message_id}.{timestamp}.{raw_body}`, verified against a public key
  obtainable either as a fixed documented value or dynamically from
  `GET https://api.kick.com/public/v1/public-key`. If Stage 15B is ever
  unblocked, this is the verification model any receiver would need to
  implement.
- **Event types** (`events/event-types.md` via the index; not fetched
  in full detail since the transport gate above already decides this
  stage): the introduction page names follows, subscriptions, gifted
  subscriptions, and chat messages as example event categories.
- **Send-chat endpoint** (`apis/chat.md`): `POST /public/v1/chat`,
  requires the `chat:write` scope, body `{ content: string (max 500
  chars), type: "user" | "bot" }`, with an optional
  `reply_to_message_id` field for threaded replies - notably, unlike
  YouTube's `liveChatMessages.insert` (no reply field at all, per
  `docs/provider-integrations/youtube-engagement.md` §3), Kick's send
  API *does* document reply support. Recorded for Stage 15B's own
  future use; not implemented here.
- **OAuth scopes** (`getting-started/scopes.md`): `user:read`,
  `channel:read`, `channel:write`, `channel:rewards:read`,
  `channel:rewards:write`, `chat:write`, `streamkey:read`,
  `events:subscribe`, `moderation:ban`,
  `moderation:chat_message:manage`, `kicks:read`. `events:subscribe` is
  the scope needed for the webhook-based event model above.
- **Subscription limits** (`events/subscribe-to-events.md`): up to
  10,000 subscriptions per event type per app; an *unverified* app is
  capped at 1,000 subscriptions for `chat.message.sent` specifically -
  a further practical constraint on any future multi-operator relay
  design, irrelevant to the current gate.

## 4. Stage 15B status

**Stage 15B: feasibility-gated, not implemented, not started.** No Kick
provider code, no Kick webhook receiver, no Kick OAuth flow, and no Kick
UI surface exists anywhere in this repository. This document exists
solely to record why, with a primary-source citation trail, so a future
stage does not have to re-derive the same research from scratch - and so
nobody mistakes the *absence* of Kick support for an oversight rather
than a deliberately researched, currently-correct architectural
conclusion.

### 4.1 Re-check, 2026-08-12 (Stage 15A transport corrective pass, §34)

Performed solely to confirm the YouTube `streamList` gRPC correction
elsewhere in this pass (`docs/provider-integrations/
youtube-engagement.md` §0/§4b) does not accidentally invalidate this
document's own gate - it does not; Kick and YouTube are unrelated
providers with unrelated transport questions. Re-checked directly:
`https://github.com/KickEngineering/KickDevDocs/issues/20` ("Websocket-
based Events") is still **open**, labeled `feature`/`planned`, with a
Kick Dev project board status of **Backlog** - unchanged from this
document's original research. No comments, commits, or shipping
timeline have been added since. Webhooks remain Kick's only official
event-delivery transport; no direct WebSocket/desktop-friendly transport
has shipped. **§2's gate, and Stage 15B's feasibility-gated status,
stand unchanged.**
