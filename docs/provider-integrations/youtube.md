# YouTube provider integration — researched contract

**Research date:** 2026-08-05

This document records what was actually verified against current, official
Google and YouTube documentation before any YouTube code was written for
Stage 7B, mirroring [`twitch.md`](twitch.md)'s own discipline: paraphrased,
not pasted, and revisited whenever Google changes these APIs. Where the
official documentation was ambiguous or silent, that is stated explicitly
rather than filled in with assumed behavior.

## Official pages inspected

- `developers.google.com/identity/protocols/oauth2/native-app` — installed-app
  Authorization Code flow, PKCE, loopback redirect.
- `developers.google.com/identity/protocols/oauth2/web-server#offline` —
  `access_type=offline`, `prompt=consent`, refresh-token issuance, revocation
  endpoint.
- `support.google.com/cloud/answer/15549945` and
  `developers.google.com/identity/protocols/oauth2/production-readiness/overview`
  — OAuth consent screen "Testing" publishing status, the 100-test-user cap,
  and the 7-day token expiration in Testing mode.
- `developers.google.com/youtube/v3/live/docs/liveBroadcasts` and
  `.../liveBroadcasts/list` — broadcast resource shape, `mine`/`broadcastStatus`
  filters, authorization scopes.
- `developers.google.com/youtube/v3/live/guides/migration-guide-default-broadcasts`
  — deprecation of persistent/default broadcasts.
- `developers.google.com/youtube/v3/docs/videos` and `.../videos/update` —
  video resource fields, update semantics.
- `developers.google.com/youtube/v3/docs/videoCategories/list` — category
  listing and region filter.
- `developers.google.com/youtube/v3/docs/channels/list` — `mine=true` channel
  identity.
- `developers.google.com/youtube/v3/determine_quota_cost` — quota costs.
- `developers.google.com/youtube/v3/live/docs/errors` and general web search
  against `developers.google.com` for `quotaExceeded`/`liveStreamingNotEnabled`
  error reasons (the errors page itself did not return full content to the
  fetch tool used; the reason names below are corroborated by Google's own
  syndicated documentation excerpts, not invented).

This document must be re-checked against the live pages above whenever Google
changes these APIs; nothing in this file should be treated as permanently
accurate.

## Selected OAuth flow and rationale

**Authorization Code Flow with PKCE, using Google's Desktop-app / installed-app
pattern with a loopback redirect.**

Google's installed-app guide documents exactly this shape for a native
desktop application: generate a PKCE verifier/challenge, open the system
browser at `https://accounts.google.com/o/oauth2/v2/auth`, receive the
authorization code on a locally-bound HTTP listener, and exchange it at
`https://oauth2.googleapis.com/token`. The guide explicitly recommends
dynamic loopback ports ("query your platform for the relevant loopback IP
address and start an HTTP listener on a random available port") and warns
that `localhost` "may cause issues with client firewalls," which is why this
application binds the callback listener to the loopback **IP** (`127.0.0.1`)
rather than the hostname `localhost`.

Google's documentation also states plainly: **"Custom URI schemes are no
longer supported due to the risk of app impersonation."** This rules out a
custom-scheme redirect outright, independent of this application's own
security requirements.

### Why Twitch's Device Code Flow is not reused

Twitch's flow (a user-code and a verification link the user visits on any
device, with this backend polling a token endpoint) is designed for
constrained/limited-input devices, and Google's closest equivalent — its own
TV/limited-input Device Authorization Flow — is explicitly for that same
class of device. Streaming Tree is a desktop application with a full browser
and keyboard already available; Google's own installed-app documentation
describes the Authorization Code + PKCE + loopback pattern specifically for
this case, not the device flow. Forcing Google's browser-redirect flow into
Twitch's `internal/runtime/deviceflow` polling state machine would also
obscure real differences the task called out: Google's flow is a one-shot
callback, not a poll loop; it can require an extra "which channel"
disambiguation step device flows have no equivalent of; and its token
endpoint parameters differ (`code`, `code_verifier`, `redirect_uri` instead
of `device_code`). A separate, provider-specific attempt manager
(`internal/runtime/youtubeauth`) was built instead — see below.

### Client secret: not required, not stored

Google's installed-app guide states the client secret "is not applicable" to
several installed-client types, and its overall installed-app flow is built
around PKCE providing the proof-of-possession a secret would otherwise
provide. This application's Google Cloud OAuth client is configured as a
**Desktop app** client type, and the operator pastes only its Client ID (see
[Client configuration](#client-configuration) below). No client secret field
exists anywhere in this application's configuration surface, request
schemas, or storage — matching the same policy already enforced for Twitch.
This satisfies the task's stop condition: current documentation supports the
no-secret desktop + PKCE flow, so no conflict was hit and no workaround was
needed.

## Client configuration

`STREAMING_TREE_YOUTUBE_CLIENT_ID`, resolved with the same precedence as
Twitch's Client ID (environment, then a `provider_integration_settings` row
scoped to `provider_id = 'youtube'`, then missing) — this reuses the existing
provider-scoped table and `account.Service` integration-config logic
unchanged; only a second `ProviderID` value (`"youtube"`) and a second
`envClientIDs` entry were added.

Only the Client ID is accepted. A pasted Client Secret, a pasted complete
`credentials.json` (Google's downloadable OAuth client file, which contains
`client_secret` and `redirect_uris` fields the JSON decoder here has no
struct field for), an access token, a refresh token, or an authorization
code are all rejected the same structural way Twitch's config endpoint
rejects them: the request struct has exactly one field (`clientId`), and
`decodeJSON`'s `DisallowUnknownFields()` rejects anything else with a stable
`unknown_field` error before the value is ever inspected.

## OAuth scopes

**`https://www.googleapis.com/auth/youtube.force-ssl`** — the same single
scope for every operation this stage needs.

Verified per operation against each method's own "Authorization" section:

| Operation | Method | Scopes documented as sufficient |
| --- | --- | --- |
| Channel identity | `channels.list(mine=true)` | `youtube.readonly`, `youtube`, or `youtube.force-ssl` |
| List broadcasts | `liveBroadcasts.list` | `youtube.readonly`, `youtube`, or `youtube.force-ssl` |
| Read broadcast/video | `videos.list` | same read scopes |
| Update video metadata | `videos.update` | `youtube.force-ssl` (documented as covering "edit ... your YouTube videos") |
| List categories | `videoCategories.list` | no authorization required at all, but this application always calls it as an authenticated user for consistency and rate-limit accounting |

`youtube.force-ssl` covers both the read and write operations above, so it
is the only scope requested — narrower than the general-purpose
`https://www.googleapis.com/auth/youtube` scope, and far narrower than
anything email/profile/Drive/Analytics/monetization-related. No
`liveBroadcasts.update` call is made in this stage (see
[Metadata mapping](#metadata-mapping-and-multi-call-publishing) — broadcast
lifecycle fields like `enableDvr`/`latencyPreference` are not published this
stage), so no broader Live Streaming API write scope was evaluated.

### access_type and prompt

Every authorization request sends `access_type=offline` (required to receive
a refresh token at all) **and** `prompt=consent`. Google's own web-server
guide states a refresh token "is only returned on the first authorization"
otherwise; since this application must be able to obtain a fresh refresh
token on every reconnect (including after a disconnect that revoked the
previous one), `prompt=consent` is sent unconditionally rather than only on
a detected "no refresh token" case.

## Loopback callback design

- Bound to `127.0.0.1` (the loopback **IP**, not the hostname `localhost` —
  matching Google's own noted firewall caveat) on a port chosen by the OS
  (`:0`), read back from the listener before building the authorization URL.
- `[::1]` is not implemented in this stage — the task allows it only "if
  implemented and tested carefully," and IPv4 loopback alone is sufficient
  for every environment this application targets.
- The listener serves exactly one path (`/callback`) and exists only for the
  lifetime of one attempt; it is closed on success, denial, cancellation,
  expiration, or backend shutdown.
- It runs as its own bare `http.Server`, never registered on the main API
  mux, and carries **no** logging middleware — not even the main API's own
  path-only access logger — so there is no code path that could ever log a
  query string containing an authorization code or state value.
- The success/failure page it serves back to the browser is a static,
  hand-written HTML string with no template interpolation of anything from
  the request: it never echoes the code, state, or any query parameter back
  into the page.
- A second request to the same callback path (a duplicate code submission,
  a browser retry, or a stray request after the attempt is already
  terminal) receives the same harmless static page and has no effect on
  attempt state — the code is consumed exactly once, on a first-write-wins
  basis guarded by the attempt's own state machine.

## Token lifecycle

- **Exchange:** `POST https://oauth2.googleapis.com/token` with
  `grant_type=authorization_code`, `code`, `code_verifier`, `client_id`,
  `redirect_uri` — no `client_secret` field is ever sent.
- **Response:** `access_token`, `expires_in`, `refresh_token`, `scope`,
  `token_type`. `id_token` is never requested (no `openid`/`email`/`profile`
  scope is in the request), so none is expected or parsed.
- **Refresh:** same endpoint, `grant_type=refresh_token`, `refresh_token`,
  `client_id` — again no client secret. The response "typically" omits a new
  `refresh_token`; when that happens, this application's adapter preserves
  the previously-stored refresh token rather than treating its absence as an
  error or storing an empty string — the task's explicit requirement, and the
  one place this stage's `RefreshToken` implementation deliberately does not
  mirror Twitch's (Twitch always rotates and returns a fresh refresh token on
  every refresh; Google usually does not).
- **Testing-mode limitation:** while this application's own Google Cloud
  project is left in OAuth consent screen "Testing" status (the default for
  a newly created project, and likely the state most self-hosted operators
  will leave it in), Google expires both the authorization and any refresh
  token it issued **seven days** after consent, regardless of the
  `access_type`/duration requested — confirmed via Google's own support
  documentation on the Testing publishing status and its 100-test-user cap.
  This is unrelated to this application's own refresh logic: even a
  correctly-implemented refresh will fail with `invalid_grant` once Google
  itself has expired the underlying grant, and the only fix is
  reconnecting. The Settings page surfaces this as a standing notice (see
  README) rather than something this application can detect or predict.
- **Revocation:** `POST https://oauth2.googleapis.com/revoke` with `token=`
  (this application sends the **refresh token** when one exists, since
  Google's own documentation states revoking a token that has a
  corresponding refresh/access pair revokes the linked one too, and revoking
  the refresh token is the more complete action); `Content-Type:
  application/x-www-form-urlencoded`. A 200 response is success. Google's
  documented failure mode for an already-invalid token is a plain error
  response (no special "already revoked" success shape the way Twitch's
  revoke endpoint has); this adapter treats a 400-class response whose body
  indicates an already-invalid/unknown token as successful revocation too,
  by the same disconnect-ordering reasoning `docs/provider-integrations/
  twitch.md` already documents, so a token Google has already expired
  (Testing-mode's 7-day cliff, for instance) never blocks a local disconnect.

## Validation policy

Google does not document a periodic re-validation requirement the way
Twitch's documentation mandates hourly checks. Rather than copy Twitch's
`defaultValidationInterval = 1 * time.Hour` for a provider with no such
requirement (and no need to spend `tokeninfo` calls that often), YouTube
accounts are validated:

- once, right after authorization completes (part of finalization, like
  every provider);
- once per backend startup, in the same non-blocking background worker
  Twitch already uses (`account.Service.StartValidationWorker`), just with
  a longer, YouTube-specific interval;
- on explicit user request (`POST /api/connected-accounts/{id}/validate`);
- transparently before a provider write, whenever the cached health is
  stale - `account.Service.WithFreshToken`'s existing refresh-and-retry-once
  logic already covers this without a YouTube-specific addition.

`account.Service`'s background validation loop is shared across every
provider's accounts (one ticker, one `ListAccounts` sweep,
`internal/domain/account/service.go`), and its interval is a single
`Service`-wide value driven by Twitch's hard hourly requirement - there is
no real need for YouTube to piggyback on a shorter interval of its own, and
splitting the loop per-provider to give YouTube a deliberately longer one
would add real complexity for no documented benefit: Google issues no
periodic-validation requirement to violate, and an hourly `tokeninfo` call
per YouTube account (a 1-unit-equivalent-cost, unauthenticated-tier Google
endpoint, not a YouTube Data API quota unit at all) is not meaningful load
under this application's realistic single-operator usage. YouTube accounts
are therefore validated by the same hourly worker Twitch already requires,
which is a stricter cadence than Google requires (none), not a violation of
anything - "no busy-looping on an invalid token" is what the task actually
forbids, and a fixed hourly sweep across a small account list is not that.

**Token validation endpoint:** `GET https://oauth2.googleapis.com/tokeninfo
?access_token=<token>` - confirmed via Google's own API reference to return
`aud` (the client ID the token was issued to), `scope` (space-separated
granted scopes), and `expires_in`. An invalid or expired token produces a
non-200 response with an error body; this adapter treats any non-200 as
"not valid" (mirroring Twitch's own `ValidateToken` contract) rather than
distinguishing every possible Google error shape.

## Testing-mode and verification limitations

- OAuth consent screens left in **Testing** status: 100 listed test users
  maximum, and every authorization/refresh token issued expires in 7 days
  regardless of what the application requests.
- Apps requesting only non-sensitive scopes (this application requests none
  of Google's "sensitive" or "restricted" scope categories with
  `youtube.force-ssl` alone being borderline — see Google's own scope
  classification, which was not exhaustively re-derived here) may not need
  full verification to leave Testing, but **this application does not detect
  or report the project's actual publishing/verification status** — it can
  only detect the practical *symptom* (a refresh failing with
  `invalid_grant` sooner than a healthy 6-month-old grant should) and
  surface `account_reconnect_required`, never a claim about *why*.
- No app-verification flow, user-cap enforcement, or "sensitive scope
  review" state is implemented or simulated by this application; the
  Settings page states the limitation as a standing, honest notice instead.

## Account/channel identity behavior

`GET https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true`,
authorized as the user. Google's own documentation confirms `mine=true`
"can only be used in a properly authorized request" and "return[s] channels
owned by the authenticated user," but — as recorded above — **does not
explicitly document whether a Brand Account can cause more than one channel
to be returned**. Rather than assume either "always exactly one" or "Brand
Accounts always return several," this application handles all three cases
defensively:

- **Zero channels:** `youtube_channel_not_found` — the attempt is finished
  as an error; no account is created.
- **Exactly one channel:** finalized automatically, exactly like Twitch's
  single-identity flow.
- **More than one channel:** the attempt moves to
  `awaiting_channel_selection`; the frontend must show every returned
  channel (ID, title, thumbnail — never using data outside those fields) and
  the operator must explicitly choose one before finalization proceeds.

The channel's `id` (its stable YouTube channel ID) is `provider_user_id`;
`snippet.title` is used for both `login` and `displayName` (YouTube channels
have no separate stable "handle" field guaranteed present in every account's
`channels.list` response — an `@handle`, when the API happens to expose one
on the resource, is not treated as a required or load-bearing identity
field here); `snippet.thumbnails` supplies the avatar URL.

## Broadcast discovery and the "persistent" deprecation

This is the one place this stage's design **deliberately diverges from the
task's own suggested wording**, because real research contradicted it:
Google deprecated default/persistent broadcasts and streams on 2020-09-01.
`liveBroadcasts.list?broadcastType=persistent` is documented to return **no
results** post-deprecation; the `broadcastType` filter's only meaningful
values today are `all` and `event`. "Active," "upcoming," and "completed"
are values of a **different** filter, `broadcastStatus`, orthogonal to
`broadcastType`. This application therefore lists broadcasts with
`mine=true&broadcastType=event` and separately requests
`broadcastStatus=active` and `broadcastStatus=upcoming` (never `completed`,
since a finished broadcast cannot receive this application's live metadata
publish anyway), merging and de-duplicating the two result sets by
broadcast ID. No "persistent" broadcast category is offered in the UI: it
does not exist as a selectable resource on a channel enabled after
2020-09-01, which is true for effectively every channel a new Streaming
Tree operator would be connecting.

## Remote broadcast target

`platform_remote_targets` (migration `0007`), exactly the shape the task
proposed: `platform_id` primary key with `ON DELETE CASCADE` to `platforms`,
`provider_id`, `resource_type` (always `"live_broadcast"` this stage),
`resource_id` (the YouTube broadcast ID), `display_name`, timestamps. No
token, stream key, or ingestion field. Selecting a target requires an
already-linked, healthy YouTube account, and the chosen broadcast ID is
verified (by re-fetching it through `liveBroadcasts.list(id=...)`) to
actually belong to the linked channel before the row is written — a
provider-mismatch or "not owned by this channel" response is rejected, not
silently accepted.

## Metadata mapping and multi-call publishing

Verified per official field, capability corrected against the previous
approximate `platform.definitions.go` entry:

| Field | API resource.property | Writable | Verified limit / notes |
| --- | --- | --- | --- |
| Title | `videos.snippet.title` | Yes (`videos.update`) | 100 characters (UTF-8, no `<`/`>`) |
| Description | `videos.snippet.description` | Yes | 5000 **bytes** (UTF-8, no `<`/`>`) — enforced as a byte count, not a rune count, unlike every other length limit in this application's `platform.Limits`, which count runes; this is called out explicitly in code as the one field that differs |
| Category | `videos.snippet.categoryId` | Yes, and **required** whenever `snippet` is included in an update | ID from `videoCategories.list`, region-scoped (see below) |
| Tags | `videos.snippet.tags` | Yes | Combined length of all tags together (including the implied separators) capped at 500 characters total — not a per-tag or a tag-count limit the way Twitch's are |
| Language | `videos.snippet.defaultLanguage` | Yes | BCC-47 language tag; this application reuses its existing `supportedLanguages` list rather than YouTube's full language catalog, exactly like the pre-existing (approximate) definition already did |
| Visibility | `videos.status.privacyStatus` | Yes | `public` / `unlisted` / `private` — same three values already modeled |
| Mature content | *(no verified equivalent)* | — | `status.selfDeclaredMadeForKids` is a **child-directed disclosure**, a legal/COPPA classification, not a generic "mature content" flag — publishing to it as if it meant the same thing would misrepresent a compliance-relevant field. Corrected to unsupported. |
| DVR | *(no verified equivalent this stage)* | — | `liveBroadcasts.contentDetails.enableDvr` exists, but is a **broadcast**, not a **video**, property, is documented as unchangeable once the broadcast is `testing`/`live`, and this stage deliberately does not implement any `liveBroadcasts.update` call at all (see below). Corrected to unsupported rather than half-implemented. |
| Latency mode | *(no verified equivalent this stage)* | — | Same reasoning as DVR: `contentDetails.latencyPreference` is a broadcast property with lifecycle restrictions this stage does not implement writes for. Corrected to unsupported. |

`platform.definitions.go`'s YouTube entry was updated to `MatureContent:
false, DVR: false, LatencyMode: false` and `LatencyOptions: []string{}` to
match — the same style of correction already made for Twitch in Stage 7A,
and, per the task, made **only** to the YouTube entry; Kick and TikTok are
untouched.

### Why no `liveBroadcasts.update` call exists this stage

The metadata this stage publishes (title, description, category, tags,
language, visibility) all lives on the **video** resource bound to the
broadcast, reachable through `videos.update` alone. A `liveBroadcasts.update`
call would only be needed for the broadcast-lifecycle fields this stage
explicitly does not support (DVR, latency) — so, honestly, **this stage's
publish path is a single non-atomic-but-single API write** (`videos.update`),
not the two-call sequence the task anticipated as the general case. The
task's multi-call-publish requirements (partial-success reporting, ordering,
idempotent retry) are implemented in the publish service regardless, so a
future stage that does add a `liveBroadcasts.update` write (DVR/latency) can
reuse that machinery without a redesign — but this stage never actually
issues more than one write per publish.

### Safe read-modify-write

Every `videos.update` call is preceded by a `videos.list(part=snippet,status,
id=<id>)` read of the **current remote resource**, immediately before the
write. Google's own documentation is explicit and was directly quoted
during this research: *"If you are submitting an update request, and your
request does not specify a value for a property that already has a value,
the property's existing value will be deleted."* This application's publish
path therefore builds its `snippet`/`status` request bodies by copying every
mutable field from the just-fetched resource and overwriting only the
fields this application actually manages (title, description, categoryId,
tags, defaultLanguage, privacyStatus) — every other mutable property
(`madeForKids`, `selfDeclaredMadeForKids`, `publishAt`, etc.) is echoed back
unchanged rather than omitted, so a value this application does not manage
is never silently cleared.

### Category region

`videoCategories.list` requires exactly one of `id` or `regionCode` per
Google's own parameter documentation. This application resolves the
"effective region" as: (1) the connected channel's country, when
`channels.list`'s response actually includes one (the official
documentation excerpt available during this research did not confirm
`snippet.country`'s presence/reliability, so this is treated as
best-effort, never assumed); (2) otherwise, an explicit region the operator
must choose — there is no silent fallback to, for instance, the interface
language, which is a UI setting with no necessary relationship to a
YouTube category region. The chosen region (an ISO 3166-1 alpha-2 code) is
stored per connected account in `youtube_channel_settings` (migration
`0008`), shown in the UI, and changing it re-fetches the category list.

## Quota behavior

Confirmed costs (`developers.google.com/youtube/v3/determine_quota_cost`):
`videos.list` = 1, `videos.update` = 50, `channels.list` = 1,
`videoCategories.list` = 1. `liveBroadcasts.list`'s exact unit cost was not
present in the fetched quota table; it is treated conservatively as
equivalent to `videos.list` (1 unit) for the purposes of this document, and
this is flagged here as an unconfirmed exact figure rather than presented
as verified. The default project quota is documented as 10,000 units/day
combined for all endpoints other than `search.list`/`videos.insert` (neither
of which this application ever calls). A publish (one `videos.list` +
one `videos.update`) costs roughly 51 units; a category search plus a
broadcast list refresh costs a few more — nowhere near the daily ceiling
under normal, human-paced use, but explicit publish and preview actions
(never automatic/background publishing) keep it that way deliberately.

## API endpoints used

| Purpose | Method |
| --- | --- |
| Authorization | `GET https://accounts.google.com/o/oauth2/v2/auth` |
| Token exchange / refresh | `POST https://oauth2.googleapis.com/token` |
| Token validation | `GET https://oauth2.googleapis.com/tokeninfo` |
| Revocation | `POST https://oauth2.googleapis.com/revoke` |
| Channel identity | `GET https://www.googleapis.com/youtube/v3/channels` |
| Broadcast listing/reading | `GET https://www.googleapis.com/youtube/v3/liveBroadcasts` |
| Video reading | `GET https://www.googleapis.com/youtube/v3/videos` |
| Video metadata update | `PUT https://www.googleapis.com/youtube/v3/videos` |
| Category listing | `GET https://www.googleapis.com/youtube/v3/videoCategories` |

## Error mapping

`errors.reason` values confirmed present in Google's own Live Streaming API
error documentation and general API error corpus: `liveStreamingNotEnabled`
(403 — the channel has not enabled live streaming), `quotaExceeded` (403),
`rateLimitExceeded` (403). A `401` with `invalid_grant`-shaped content from
the token endpoint means the refresh token itself is no longer usable
(revoked, or Testing-mode's 7-day cliff) and maps to
`account_reconnect_required`, never a generic "unavailable."

## Known limitations

- No real Google account, Google Cloud project, or network request to
  Google/YouTube was used anywhere in this stage's implementation or
  automated tests — see the closing report.
- `channels.list`'s `snippet.country` reliability is unconfirmed by the
  documentation excerpts available during this research; the region-fallback
  behavior above treats it as best-effort for exactly that reason.
- `liveBroadcasts.list`'s exact quota cost is unconfirmed (see
  [Quota behavior](#quota-behavior)).
- Persistent/default broadcasts are not offered, because Google deprecated
  them in 2020, not because this application chose to omit them.
- `[::1]` (IPv6 loopback) is not implemented for the callback listener.

## Fields deliberately not published

Description of *why*, not just *what*, since Twitch's document already
established this style: `madeForKids`/`selfDeclaredMadeForKids` (a
COPPA/child-directed compliance flag, not equivalent to Twitch's absent
"mature content" boolean or to any generic maturity rating this
application's `Capabilities.MatureContent` field represents), broadcast
`contentDetails.enableDvr` and `contentDetails.latencyPreference` (broadcast
lifecycle-restricted properties this stage's single-call `videos.update`
publish path does not touch), `publishAt` (scheduled-publish timing, no UI
for it exists), and anything under `status.uploadStatus`/`processingDetails`
(read-only, YouTube-managed state).

## Areas reserved for Stage 15

YouTube live-chat ingestion and sending, Super Chat events, membership
events, and any YouTube engagement connector remain entirely unimplemented
by this stage — `docs/engagement-architecture.md` still marks Stage 15 (and
the Event Bus itself, Stage 8) planned, and nothing here changes that.
Automatic broadcast creation, automatic `liveStream` binding, and automatic
stream-key retrieval from YouTube are also explicitly out of scope for this
stage and are not implemented.

## Requirement to re-check this contract

Everything above reflects Google and YouTube's documentation as read on
2026-08-05. Google is known to change OAuth consent-screen policy, quota
allocations, and API field behavior without this application's knowledge;
this document — and the code it describes — must be re-verified against the
live documentation pages listed above before being trusted again after any
significant elapsed time, exactly as `twitch.md` already requires for
Twitch.
