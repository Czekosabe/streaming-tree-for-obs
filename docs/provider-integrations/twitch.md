# Twitch provider integration — researched contract

**Research date: 2026-08-04.** This document records the Twitch API contract
this application's Twitch adapter (`apps/server/internal/provider/twitch`)
was built against. It is a snapshot, not a promise: Twitch can change any of
this, and this document **must be re-reviewed whenever Twitch's official
documentation changes**, before assuming the adapter still matches reality.

Only primary Twitch sources were used. No section below is copied verbatim
from Twitch's documentation at length; each is paraphrased, with a link to
the page it summarizes.

## Official pages inspected

- [Authentication](https://dev.twitch.tv/docs/authentication/)
- [Getting OAuth Access Tokens](https://dev.twitch.tv/docs/authentication/getting-tokens-oauth/) — Device Code Grant Flow
- [Refreshing Access Tokens](https://dev.twitch.tv/docs/authentication/refresh-tokens/)
- [Validating Requests](https://dev.twitch.tv/docs/authentication/validate-tokens/)
- [Revoking Access Tokens](https://dev.twitch.tv/docs/authentication/revoke-tokens/)
- [Register Your App](https://dev.twitch.tv/docs/authentication/register-app/)
- [API Reference](https://dev.twitch.tv/docs/api/reference/) — Get Users, Get Channel Information, Modify Channel Information, Search Categories
- [API Concepts (rate limiting)](https://dev.twitch.tv/docs/api/guide)

## Selected OAuth flow and rationale

**Device Code Grant Flow (DCF)**, as a **public client** (no client secret).

Twitch's own documentation states a public client "cannot use any of the
other flows" and is limited to DCF, but in exchange never needs to hold a
client secret at all — including for refreshing tokens, where the refresh
endpoint explicitly documents `client_secret` as "not required if your
application's client type was set to public." This matches this
application's constraint exactly: it is a local, single-user desktop-style
program with no secure place to hold a confidential-client secret, and DCF's
"go to this URL from any browser, on any device" model requires no local
web server or redirect URI either. Confirmed current and generally
available (no beta gate) as of the research date above.

**No conflict was found** between this task's requirement (no client secret
in the production flow) and current official documentation — the flow was
implemented as planned.

## Required scope

**`channel:manage:broadcast`** only — the scope Modify Channel Information
requires. Get Channel Information and Get Users need no scope (a valid
token is enough). No chat, EventSub, subscription, Bits, moderation or
email scope is requested in this stage; those remain for Stage 8 and later,
which will request additional consent (or prompt a reconnect) only when a
capability that actually needs a new scope is implemented.

## Device flow contract

**Start:** `POST https://id.twitch.tv/oauth2/device`, form-encoded
`client_id` + space-delimited `scopes`. Response: `device_code` (backend-only,
one-time use), `user_code` (safe to show the user), `verification_uri`,
`expires_in` (typically 1800s / 30 minutes), `interval` (typically 5s).

**Poll:** `POST https://id.twitch.tv/oauth2/token`,
`grant_type=urn:ietf:params:oauth:grant-type:device_code` + `device_code` +
`client_id`. Twitch's documented pending response is
`{"status":400,"message":"authorization_pending"}` — a message string, not
the RFC 8628 `error` field other providers use, so this application matches
on `message`, tolerantly. The Device Flow's other standard states (all
observed as the same `{status, message}` shape in practice, per Twitch
support-forum guidance and the RFC 8628 base this flow follows) are
`slow_down` (increase the polling interval), `access_denied` (user declined),
and `expired_token` (the `expires_in` window elapsed — Twitch's own
documentation notes the device may immediately request a fresh code).

**Success:** `access_token`, `expires_in` (~4 hours), `refresh_token`
(one-time use), `scope` (array), `token_type: "bearer"`.

## Token lifecycle

- **Access token lifetime:** ~4 hours.
- **Refresh tokens are single-use and rotate on every refresh.** Twitch's
  own refresh-token page: "your app should safely store the new refresh
  token to use the next time" — confirming a fresh refresh token is issued
  every time and the old one is spent. This application therefore replaces
  the **entire** token bundle atomically on every refresh (see "Token
  storage design" below) — never persisting a new access token next to a
  now-stale refresh token.
- **Public-client refresh tokens expire 30 days after issuance** if unused.
  Once expired, refreshing is impossible and the user must repeat the
  device flow from the start — this application surfaces that as
  `reconnect_required`, not a silent failure.
- **Validation is a hard requirement, not a courtesy check:** Twitch's own
  documentation states an app "must validate the OAuth token when it starts
  and on an hourly basis thereafter," warning of "punitive action" for
  apps that do not. This application's background worker validates every
  connected account on that same schedule.

## Validation

`GET https://id.twitch.tv/oauth2/validate`, `Authorization: OAuth
<token>`. Success (200): `client_id`, `login`, `user_id`, `scopes`,
`expires_in`. Failure (401): `{"status":401,"message":"invalid access
token"}`. This application additionally checks the returned `client_id`
matches the configured Twitch Client ID and that `scopes` still contains
`channel:manage:broadcast`, before trusting the account as healthy.

## Revocation

`POST https://id.twitch.tv/oauth2/revoke`, form-encoded `client_id` +
`token`. Success: 200 with an empty body. Twitch treats an already-invalid
token as a normal 400 ("Invalid token"), which this application treats the
same as a successful revocation (see the disconnect ordering in
`docs/project-overview.md`).

## Helix endpoints used

| Endpoint | Method | Scope | Purpose |
| --- | --- | --- | --- |
| `/helix/users` | GET | none (valid token) | Resolve the authorized user's stable ID, login and display name/avatar. |
| `/helix/channels` | GET | none (valid token) | Read current remote channel metadata for the publish-preview diff. |
| `/helix/channels` | PATCH | `channel:manage:broadcast` | Publish saved local metadata. |
| `/helix/search/categories` | GET | none (valid token) | Category search backing the metadata editor's category picker. |

Every request sends `Authorization: Bearer <token>` and `Client-Id:
<configured client id>`.

## Verified field limits

- **Title:** max 140 characters; Twitch rejects an empty string.
- **Category:** identified by `game_id` (stable) with `game_name` as
  display text — **there is no way to reliably publish a category by name
  alone**, which is exactly why this stage adds a `categoryId` column
  alongside the existing free-text `category` field (see below).
- **Tags:** max 10, each max 25 characters, and Twitch's own documentation
  states a tag "may not be an empty string or contain spaces or special
  characters." This is a real, additional constraint beyond the count/length
  limits this application already validated — it is now checked before a
  Twitch publish (not at local-save time, since local metadata storage
  stays provider-agnostic free text).
- **Language:** `broadcaster_language`, an ISO 639-1 two-letter code.

## Fields Streaming Tree deliberately does NOT publish to Twitch

- **Description.** Twitch's Get/Modify Channel Information has no
  description field at all. `Capabilities.Description` for Twitch was
  already `false` before this stage and stays `false`.
- **Visibility.** No equivalent Twitch concept; a Twitch channel is not
  toggled public/unlisted/private the way a YouTube broadcast is.
  `Capabilities.Visibility` stays `false`.
- **DVR.** No equivalent. Twitch's `delay` field (Partner-only broadcast
  buffering, up to 900 seconds) is a **different concept** from a viewer-side
  DVR/rewind toggle and is not mapped to it. `Capabilities.DVR` stays `false`.
- **Mature content — corrected in this stage.** The provider table
  previously marked `MatureContent: true` for Twitch, approximated before
  any real API was consulted. Twitch's actual API has **no single boolean**
  for this: `content_classification_labels` is a *set* of specific labels
  (drugs/intoxication, gambling, mature-rated game, sexual themes, extreme
  violence, and similar), and `is_branded_content` is an unrelated
  sponsorship-disclosure flag, not a maturity signal at all. Forcing either
  into this application's generic single `matureContent` boolean would not
  be "exact semantic equivalence," which this task explicitly forbids.
  **`Capabilities.MatureContent` is corrected to `false` for Twitch in this
  stage.** Per-label content classification is reserved for a future stage
  that models it as its own multi-select concept, not shoehorned into the
  existing boolean.
- **Latency mode — corrected in this stage.** Also previously `true`
  (approximated). Modify Channel Information has no field controlling
  low-latency mode; that setting lives outside the Helix channel-update API
  this application uses. **`Capabilities.LatencyMode` is corrected to
  `false` for Twitch in this stage.**

These are the two user-visible capability corrections this stage makes
(recorded again in `docs/progress.md`): a Twitch destination's metadata
editor stops rendering the mature-content toggle and latency-mode selector,
because there is no real Twitch API this application calls that they would
actually affect. Any value already saved locally in those fields (for
example the seeded example data) is simply no longer surfaced for Twitch —
it is harmless, unused local data, not deleted.

## Rate limiting

Helix responses carry `Ratelimit-Limit`, `Ratelimit-Remaining` and
`Ratelimit-Reset` (a Unix-epoch reset time) headers. A bucket empties within
a rolling one-minute window; exceeding it returns `429`. This application
parses these headers (tolerantly — an unfamiliar or missing header must not
crash a request) and maps a `429` to the stable `twitch_rate_limited` error
rather than retrying blindly.

## Known API limitations / operational notes

- The device-code polling response format is Twitch-specific
  (`{status, message}`), not the RFC 8628 `error` field shape used by many
  other providers — this adapter parses Twitch's actual shape, not a
  generic OAuth device-flow library's assumed shape.
- Refresh tokens are **one-time use**; a refresh must always be followed by
  persisting the *new* refresh token before considering the operation
  complete, or a subsequent refresh attempt will fail with an already-spent
  token.
- Community reports (Twitch developer forums) describe occasional
  transient `500`/false `429` responses from `/helix/channels`; this
  adapter treats any `5xx` as a retryable, sanitized provider error rather
  than a permanent failure, consistent with how this application already
  treats MediaMTX and FFmpeg failures.

## Areas explicitly reserved for Stage 8

EventSub subscriptions, chat (IRC/EventSub chat), moderation scopes,
subscription/Bits/channel-points scopes, and any capability that needs a
broader token than `channel:manage:broadcast` are **not** implemented here.
The connected-account record and token-bundle storage this stage builds are
designed to be reused as-is by Stage 8 — adding a capability there should
mean requesting additional scope and prompting a reconnect, not redesigning
how a Twitch account is stored.

---

This document records the contract this implementation was built against on
the research date above. Twitch's API and policies can change at any time;
before relying on any detail here for new work, re-check it against the
current official documentation linked above.
