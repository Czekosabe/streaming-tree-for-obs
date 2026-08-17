# TikTok LIVE engagement - feasibility research (Stage 19 gate)

**Research date/time:** 2026-08-17, conducted directly against
`developers.tiktok.com` and TikTok's own legal/community-guidelines pages.

**Status: research/feasibility only. No TikTok LIVE provider code exists in
this repository, and none is added by this document.** This is the Stage 19
gate this project's own roadmap requires be resolved *before* any TikTok LIVE
engagement implementation is attempted - see
[engagement-architecture.md](../engagement-architecture.md) §16/§18 and
`README.md`'s roadmap table (Stage 19 row). This document does **not**
concern TikTok as an RTMP/RTMPS **streaming destination** (already configured
and unaffected, see §F) or a generic TikTok **account login** in the
abstract (see §G) - it concerns specifically whether TikTok currently offers
an official, permitted way for a local desktop application to receive LIVE
engagement events (chat, gifts, and similar).

## Sources inspected (official only)

All of the following were fetched directly from `developers.tiktok.com` (or
TikTok's own legal pages) on the research date above. No unofficial TikTok
LIVE endpoint list, reverse-engineered protocol writeup, browser network
capture, mobile-app decompilation, or third-party "TikTok LIVE API" SaaS
product was consulted for any part of this document's factual conclusions -
several such projects surfaced during search and are named in §H solely to
record that they were recognized and rejected, not used.

- <https://developers.tiktok.com/> - developer portal homepage and full
  product list.
- <https://developers.tiktok.com/doc/overview> - complete documentation
  navigation/table of contents.
- <https://developers.tiktok.com/doc/webhooks-overview> - webhook delivery
  mechanism (callback registration, HTTPS requirement, retry policy).
- <https://developers.tiktok.com/doc/webhooks-events> - the complete,
  enumerated list of every webhook event TikTok currently documents.
- <https://developers.tiktok.com/doc/login-kit-desktop> - Desktop Login Kit
  flow: authorization endpoint, redirect-URI rules, PKCE requirements.
- <https://developers.tiktok.com/doc/oauth-user-access-token-management> -
  the exact authorization-code and refresh-token token-exchange requests.
- <https://developers.tiktok.com/doc/tiktok-api-scopes> - the complete OAuth
  scopes reference.
- <https://developers.tiktok.com/doc/embed-player> - Embed Player capability
  and postMessage event list.
- <https://developers.tiktok.com/doc/changelog> - every changelog entry from
  February 2025 through June 2026 (the most recent entry as of the research
  date).
- <https://www.tiktok.com/legal/page/global/tik-tok-developer-terms-of-service/en>
  and TikTok's Community Guidelines - reverse-engineering/circumvention
  prohibition (§H).

## A. Officially documented capabilities

TikTok's current developer product list (from the portal homepage and
`doc/overview`'s full navigation) consists of: **Login Kit, Share Kit,
Content Posting API, Embed Videos, Webhooks, Data Portability API, Green
Screen Kit, Display API, Research API, Commercial Content API, Monetization
(in-app virtual goods for TikTok Minis), TikTok Minis Platform, TikTok GO
(Dining), and Scopes/legacy references.**

Of these, only two are even plausibly relevant to real-time engagement:

- **Webhooks** - a real, working push-notification mechanism, but (per §B)
  it carries none of the events this project would need.
- **Embed** (Embed Player / Embed Videos / Embed Creator Profiles) - a
  playback/display surface, not an engagement API (per §B and §8 of the
  governing task).

No product named "LIVE API," "LIVE Studio," "Live Streaming," "Webcast," or
equivalent exists anywhere in the current product list or documentation
navigation.

## B. Missing/unavailable capabilities

**No official TikTok LIVE engagement API exists in any form** - not as a
REST polling endpoint, not as a WebSocket, not as a webhook, not as a
server-sent-event stream, and not as part of any other documented product.
Specifically, none of the following are documented anywhere in
`developers.tiktok.com`:

- Receiving LIVE chat/comments in real time.
- Receiving LIVE gifts in real time.
- Receiving LIVE likes as a LIVE-context engagement event.
- Receiving LIVE follows/subscriptions as a LIVE-context engagement event.
- Discovering whether a creator is currently LIVE, or obtaining a LIVE
  room/session identifier.
- Sending LIVE chat as an authenticated third-party app.

**Webhooks (`doc/webhooks-events`).** The complete, enumerated event list is
exactly four events: `authorization.removed` (a user revokes app access),
`video.upload.failed` and `video.publish.completed` (Content/Share Kit video
publishing outcomes), and `portability.download.ready` (Data Portability API
download readiness). **None relate to LIVE streaming, LIVE chat, LIVE
gifts, or LIVE status in any way.** This list is exhaustive per the official
events page, not a keyword-search miss - the entire webhook product covers
video-publish lifecycle and account-authorization lifecycle only.

**Scopes (`doc/tiktok-api-scopes`).** The complete scope list covers basic
user profile (`user.info.basic/.profile/.stats`), video
(`video.list/.publish/.upload`), Data Portability
(`portability.*`), Research (`research.*`), and Local Service/TikTok GO
(`local.*`) scopes only. **No scope of any kind grants access to LIVE data**
- there is no `live.read`, `live.chat`, `live.gifts`, or equivalent scope
anywhere in the reference.

**Changelog (`doc/changelog`).** Every entry from February 28, 2025 through
the most recent entry (June 4, 2026) was inspected. None mentions LIVE,
live streaming, live chat, live gifts, or webcast in any form; the entries
in this window cover Data Portability, Research Tools, TikTok GO launch,
Batch Compliance APIs, and Mini Games. There is no indication a LIVE
engagement product has ever shipped, is in beta, or is announced.

## C. Authentication architecture

**Desktop Login Kit** (`doc/login-kit-desktop`) is real and well documented
for the capabilities it does cover (basic profile, video publishing, Data
Portability): authorization endpoint
`https://www.tiktok.com/v2/auth/authorize/`; redirect URIs must use
`localhost` or `127.0.0.1` with a port (wildcard `*` supported), matching
this project's existing Stage 7B loopback-callback OAuth pattern; PKCE is
required (`code_challenge`/`code_challenge_method=S256`); `state` is
required for CSRF protection.

**Token exchange** (`doc/oauth-user-access-token-management`), the load-
bearing detail: `POST https://open.tiktokapis.com/v2/oauth/token/` with
required parameters `client_key`, `client_secret`, `code`, `grant_type=
authorization_code`, `redirect_uri`, and (desktop/mobile only)
`code_verifier`. **`client_secret` is listed as a required parameter for
the authorization-code exchange, alongside PKCE, not instead of it.**
**Refreshing** a token (`grant_type=refresh_token`) requires exactly the
same `client_key`+`client_secret` pair again - there is no reduced-secret
path for token refresh either. No alternate, secret-free "public client"
token-exchange endpoint or flow is documented anywhere on
`developers.tiktok.com` for desktop/native apps.

## D. Local-desktop compatibility

Even setting aside §B entirely (no LIVE data to fetch at all), the
authentication model itself is incompatible with this project's deployment
architecture:

- Streaming Tree for OBS is local-first, source-available, and distributed
  directly to end users with no Streaming-Tree-operated cloud backend
  (`docs/project-overview.md`, `README.md`).
- A `client_secret` required for every token exchange and every refresh
  cannot be meaningfully confidential once compiled into or shipped
  alongside a downloadable, source-available desktop application - every
  installation would carry the identical value, extractable by any user.
  PKCE mitigates authorization-code interception in transit; it does not
  remove the requirement to present `client_secret` in the token-exchange
  request body, and TikTok's own docs list both as required simultaneously
  (§9/§10 of the governing task's framing - the "PKCE + still-required
  secret" contradiction is real and confirmed above, not assumed).
- OS-backed credential storage (already used in this project for per-user
  OAuth token bundles, see `engagement-architecture.md` §17.1) protects a
  *user's own* secret on that user's machine. It does not change the fact
  that a *vendor-wide* confidential application secret shipped to every
  installation is not actually confidential - this is the same
  distinction already drawn for Streamlabs in
  [external-donations.md](external-donations.md), and it applies
  identically here.
- No alternate public-client/secret-free desktop flow is documented (§C).

**Conclusion: even if TikTok had a LIVE engagement API, the currently
documented Desktop Login Kit auth model alone would already be a hard
architectural blocker for this deployment target**, absent either a
TikTok-provided public-client flow or a fundamental change to this
project's own architecture (a Streaming-Tree-operated backend to broker
tokens) - neither of which exists today and neither of which this document
proposes.

### Webhook local-first evaluation (for completeness)

Applying the same test already used for Kick and Ko-fi
(`kick-engagement.md`, `external-donations.md`): TikTok's webhook delivery
(`doc/webhooks-overview`) requires a callback URL registered on the
Developer Portal that "must require HTTPS," i.e. a publicly reachable
inbound endpoint - the same architecture gate that already defers Kick and
Ko-fi. This is recorded for completeness only; it does not change the
outcome, since (per §B) no LIVE-relevant webhook event exists to receive in
the first place.

## E. Stage 19 decision

**Stage 19: Deferred / feasibility-gated. Not implemented, not started.**
No TikTok LIVE provider code, no TikTok LIVE OAuth flow, no TikTok LIVE
SQLite migration, no TikTok LIVE UI, no fake TikTok LIVE integration
script, and no TikTok Event Bus normalizer exist anywhere in this
repository. Three independent, each-individually-sufficient reasons, all
confirmed against current official documentation on the research date
above:

1. **No official TikTok LIVE engagement event API/scope exists at all**
   (§B) - not gated by auth, not partner-only, not review-only:
   structurally absent from the entire documented product surface.
2. **Embed Player is playback-only** (§8 of the governing task; verified
   directly against `doc/embed-player`): its complete postMessage event
   list is `onPlayerReady`, `onStateChange`, `onCurrentTime`, `onMute`,
   `onVolumeChange`, `onImageChange`, `onPlayerError` - player-state
   telemetry, not engagement data - and it documents no LIVE-stream
   embedding support in the first place (only video/image posts). It is
   not treated as Stage 19 engagement, and Streaming Tree already receives
   the stream itself locally via OBS/MediaMTX, so it has no use here even
   as playback.
3. **Desktop Login Kit's only documented token-exchange path requires a
   confidential `client_secret` alongside PKCE, with no documented
   public-client alternative** (§C/§D) - architecturally incompatible with
   a distributed, source-available, local-only application, independent of
   whether LIVE data existed to protect.

If TikTok ever ships (a) a genuine LIVE engagement event API/scope and (b)
either a public-client desktop token exchange or an equivalent architecture
this project could adopt without embedding a shared vendor secret, this gate
should be re-researched from scratch against TikTok's *then-current*
official documentation - the conclusion above is time-bound to what
`developers.tiktok.com` states as of 2026-08-17, not a permanent verdict
about TikTok generally.

## F. Streaming destination unaffected

TikTok already exists as a configured RTMP/RTMPS streaming destination and
FFmpeg output branch (`docs/project-overview.md`, roadmap Stage 6). That
capability is unrelated to this gate - it is outbound video delivery
Streaming Tree already performs today, not an authenticated TikTok account
or a LIVE engagement connector, and nothing in this document removes or
modifies it.

## G. Stage 7C / TikTok account integration decision

`engagement-architecture.md` §18 and `project-overview.md` §13 both
currently list Stage 7C as "Kick and TikTok account integration -
deferred, capability-gated." Given §B-§E above, a standalone TikTok Login
Kit account integration has **no current downstream product use**: there is
no LIVE engagement data it would unlock, and the desktop token-exchange
model would be incompatible even if there were. Implementing TikTok OAuth
merely to have an authenticated account, with nothing for that account to
power, would be dead infrastructure - the same judgment call already applied
project-wide (e.g. Stage 18B's own "no widget designer without a proven
requirement" decision). **Decision: TikTok account integration remains
folded into Stage 19's own feasibility gate, not pursued as an independent
Stage 7C deliverable, until Stage 19 itself becomes feasible.** This is a
wording/roadmap-consistency correction, not a new blocker - Stage 7C was
already "deferred, capability-gated" for TikTok specifically; this document
makes the linkage to Stage 19 explicit rather than leaving two separately-
worded deferrals that could drift apart. Kick's own Stage 7C status is
unchanged by this document.

## H. Unofficial APIs/libraries considered and rejected

During research, web search results surfaced multiple actively-marketed
"TikTok LIVE API" products and open-source libraries, including (named here
solely for the record - none inspected as an implementation reference, none
added as a dependency, none used to inform any architectural decision
above): `TikTokLive` (Python, isaackogan), `tiktok-live-connector` (Node.js,
zerodytrash), and commercial "managed WebSocket" resellers built on top of
them. All of these operate by reverse-engineering TikTok's private mobile/
web "webcast" protocol, generating request signatures out-of-band, and/or
proxying through third-party signing infrastructure. TikTok's own Developer
Terms of Service prohibit "distributing, copying, modifying, reverse
engineering, decompiling, or otherwise altering the TikTok Developer
Services or TikTok Services," and its Community Guidelines separately state
it does "not allow attempts to hack, reverse-engineer, or otherwise
compromise TikTok's systems." Per this task's own explicit instructions and
this project's own established policy (`kick-engagement.md`,
`external-donations.md`), none of these were used, will be used, or inform
any part of this document's technical conclusions. Their existence
confirms *demand* for TikTok LIVE data; it does not confirm an *official,
permitted* way to obtain it, and does not change §E.

## Decision matrix

| # | Capability | Status |
| - | ---------- | ------ |
| 1 | Desktop OAuth/login | Supported officially (Login Kit Desktop, PKCE + loopback redirect), but see row 12 |
| 2 | Basic user/profile identity | Supported officially (`user.info.basic`/`.profile`/`.stats` scopes) |
| 3 | LIVE room/session discovery | Not found officially - no scope, endpoint, or webhook exposes LIVE status or a session identifier |
| 4 | LIVE chat/comments inbound | Not found officially - absent from webhooks, scopes, and the full product list |
| 5 | LIVE gifts inbound | Not found officially - absent from webhooks, scopes, and the full product list |
| 6 | LIVE subscriptions/support events | Not found officially - absent from webhooks, scopes, and the full product list |
| 7 | LIVE follows (LIVE context) | Not found officially - absent from webhooks, scopes, and the full product list |
| 8 | LIVE status (is a creator currently live) | Not found officially - no documented endpoint or event |
| 9 | Outbound LIVE chat (send as app) | Not found officially - no documented endpoint |
| 10 | Transport type (if any capability existed) | Not applicable - no LIVE transport of any kind is documented (no WebSocket, SSE, polling endpoint, or LIVE-relevant webhook event) |
| 11 | Required public callback/server | Feasibility blocker for Webhooks generally (HTTPS callback required, same class of blocker as Kick/Ko-fi) - moot here since no LIVE event exists to deliver via that transport |
| 12 | Confidential-secret requirement | Feasibility blocker - `client_secret` is a required parameter on both the authorization-code exchange and the refresh-token exchange, alongside PKCE, with no documented public-client alternative |
| 13 | Local desktop compatibility | Architecturally incompatible - distributing a shared vendor `client_secret` to every installation is not confidentiality, independent of row 3-9 |
| 14 | Current implementation decision | Deferred / feasibility-gated - no product code added; re-check required against future official documentation before any implementation begins |

## Re-check protocol

Re-research from scratch against then-current `developers.tiktok.com`
documentation (not this document, and not any cached/summarized version of
it) before starting any TikTok LIVE implementation work, exactly as
`kick-engagement.md` §4.1 already establishes for Kick. A materially
different outcome requires at minimum: (a) a genuine LIVE engagement event
API/scope appearing in the scopes reference or product list, and (b) either
a documented public-client desktop token exchange or a project-level
architecture decision to operate a token-brokering backend - neither
condition met as of this research date.
