# Stage 20D2B — secure remote management / control plane

**Research date:** 2026-08-19.

This document defines Stage 20D2B only: the remote management/control
plane for the Linux headless deployment established in Stage 20D2A. It
explicitly does not implement remote OBS ingest (Stage 20D2C) and does
not expose MediaMTX RTMP, remote overlays, or any new port beyond the
existing loopback-only management HTTP listener.

## 1. Scope boundary (restated, unconditional)

The Streaming Tree backend's own HTTP listener remains **loopback-only**
in every mode, exactly as strict as Stage 20D2A left it. D2B never
changes `STREAMING_TREE_HOST`/`STREAMING_TREE_PORT` semantics, never
binds `0.0.0.0` or `::`, and never opens a new port. Remote reachability
is provided entirely by an **operator-supplied HTTPS reverse proxy on
the same host**, terminating TLS and forwarding to the existing
loopback backend over plain HTTP. Streaming Tree does not become a
general TLS server: no embedded Caddy/nginx, no ACME, no certificate
issuance, no port 80/443 bind of its own.

Not implemented in D2B, all explicitly Stage 20D2C or later: remote
RTMP/RTMPS/SRT ingest, MediaMTX external auth, public MediaMTX
listeners, remote overlay exposure, VPN provisioning, application-
managed firewall rules.

## 2. Primary-source research (this date)

- RFC 9106 (Argon2 memory-hard function): §4's two recommended
  parameter sets for password hashing. The first option (2 GiB memory)
  targets "environments with the possibility of side-channel attacks"
  and dedicated hardware; the second ("if much less memory is
  available") is Argon2id, t=3, p=4 lanes, m=2^16 KiB (64 MiB), 128-bit
  salt, 256-bit tag. This application runs alongside MediaMTX/FFmpeg
  child processes and other services on an operator's general-purpose
  server, not dedicated authentication hardware - the second,
  memory-constrained option is selected and used exactly as specified,
  not an invented or remembered value.
- `pkg.go.dev/golang.org/x/crypto/argon2`: exposes `IDKey(password,
  salt []byte, time, memory uint32, threads uint8, keyLen uint32)
  []byte` for Argon2id specifically (not `Key`, which is Argon2i). Used
  directly; no third-party wrapper.
- MDN's Cookies guide: the `__Host-` prefix requires `Secure`, `Path=/`,
  and no `Domain` attribute - selected for the session cookie, since
  D2B's own topology (one canonical external origin, no subdomain
  cookie sharing ever needed) has no reason not to take the strongest
  available cookie-scoping guarantee. `Max-Age` (not `Expires`)
  confirmed as the current-recommended lifetime mechanism, avoiding
  client/server clock-skew error class `Expires` is documented to be
  more error-prone under.
- MDN's `Sec-Fetch-Site` reference: confirmed Baseline-available since
  March 2023, browser-generated only (a forbidden header - JavaScript
  cannot set or spoof it), absent from older browsers and non-browser
  clients. Used as defense-in-depth only, never the sole CSRF control,
  per its own documented gap (not universally present).
- Caddy's `reverse_proxy` documentation: confirmed directly - "by
  default, the proxy will ignore [`X-Forwarded-*`] values from incoming
  requests, to prevent spoofing" and sets fresh `X-Forwarded-For`/
  `X-Forwarded-Proto`/`X-Forwarded-Host` values itself. This is the
  load-bearing fact behind §11's forwarding-header contract: the
  documented reference proxy topology cannot pass through an
  attacker-supplied forwarded header, so the application only needs to
  trust these headers when they arrive from the loopback peer at all.
- RFC 6454 (Origin header, well-established, not re-fetched this
  session but applied as authoritative): Origin identity is the
  scheme+host+port tuple; a subdomain or different port is a different
  origin even under the same registrable "site".

**PRE-20D2C correction research (2026-08-19), added after Stage 20D2B's
own close:**

- Caddy's "Caddyfile matchers" documentation: confirmed directly -
  *"If the matcher token is omitted, it is the same as a wildcard
  matcher (`*`)"* - the exact fact that disproved the original
  reference configuration's implicit claim that a bare `reverse_proxy`
  only forwarded intended paths. Also confirmed the `path` matcher's
  multi-pattern syntax (`path <paths...>`, "Multiple paths will be
  OR'ed together" - the real official example given is `@assets path
  /js/* /css/* /images/*`, directly analogous to this document's own
  `@excludedLocalOnlySurface { path /overlay/* /api/public/* }`).
- Caddy's "Directives" documentation: confirmed directly that Caddy
  does **not** execute directives in the literal textual order written
  in a Caddyfile - *"a default ordering is hard-coded into Caddy"* -
  and that `handle` is sorted before `respond`, which is sorted before
  `reverse_proxy`, in that built-in order. This is why §20's corrected
  configuration is correct regardless of which `handle` block appears
  first in the file.
- Caddy's `handle` directive documentation: confirmed directly - *"when
  multiple `handle` directives appear in sequence, only the first
  matching `handle` block will be evaluated"* (mutual exclusivity), and
  the documentation's own worked example is exactly this milestone's
  own shape: *"Handle requests in `/foo/` with the static file server,
  and other requests with the reverse proxy."* This is the official,
  idiomatic pattern selected for §20's fix - not an invented one.

## 3. Explicit remote-management mode (opt-in)

A new boolean setting, `--remote-management` (mirroring the existing
`--headless` CLI convention in `cmd/server/main.go`), gates every
behavior in this document. It is never inferred from `--headless`
alone, from `GOOS`, or from any environment heuristic. It may only be
set true when `--headless` is also true (checked explicitly at
startup; remote management outside headless mode is refused - D2B is
Linux-headless-deployment functionality, per §47's own restatement
below). Ordinary desktop (Windows/macOS/Linux packaged) and existing
D2A headless-without-remote-management deployments are byte-for-byte
unaffected: no new middleware runs, no login screen appears, no
session/CSRF state is ever constructed, when this flag is false.

## 4. Security-critical deployment configuration (local-only, never
API-mutable)

A new `internal/config` struct, `RemoteManagementConfig`, holds:

- `Enabled bool` - the `--remote-management` flag itself.
- `ExternalOrigin string` - the one canonical external HTTPS origin
  (§6).
- Proxy-trust is implicit, not a separate configurable field: the
  application trusts forwarded headers only from a direct TCP peer at
  a loopback address (§8) - there is no "trusted proxy list" to
  misconfigure, by construction.

All three are read from environment variables at process startup
(`STREAMING_TREE_REMOTE_MANAGEMENT`,
`STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN`), exactly like every other
`internal/config` value - the existing environment-variable model
already satisfies "not mutable through the remote-management UI/API"
by construction, since no HTTP handler in this codebase ever writes to
process environment or re-invokes `config.Load()`. No new persisted-
settings table is needed for this boundary. The shipped systemd unit
sets these via its own `Environment=` lines, editable only by an
operator with `systemctl edit` (root), never by any HTTP route.

## 5. Failure-closed startup

When `--remote-management` is set, `run()` validates, before creating
any listener:

1. `--headless` is also set (else: fatal, remote management requires
   headless mode).
2. `ExternalOrigin` parses as `https://host[:port]` exactly (§6) - no
   fallback to `http://`.
3. The headless `SecretStore` (already fail-closed per Stage 20D2A)
   constructed successfully - remote management depends on it for both
   the admin password verifier and every provider secret.
4. An administrator password verifier is already provisioned (§9) -
   remote management never starts with no way to authenticate, and
   never silently falls back to an unauthenticated remote mode.

Any failure here returns an error from `run()`, logged and exited
nonzero, exactly like every other Stage 20D2A fail-closed condition -
no new error-handling pattern.

## 6. Canonical external management origin

`STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN` must be exactly one origin:
scheme `https` (mandatory), a host, an optional explicit port, no
userinfo, no path, no query, no fragment. Validated with `net/url`:
parse, then reject if `Scheme != "https"`, `User != nil`,
`Path/RawPath` not empty/`"/"`... actually Path must be empty (no
prefix) - `RawQuery != ""`, `Fragment != ""`, or `Host` empty. No
wildcard, no list (a single string, not `AllowedOrigins`'s existing
comma-separated slice shape - deliberately a different, stricter type
so the two are never confused). This is the value every Origin check
(§7) and every cookie/CSRF decision compares against - never derived
from an incoming `Host` header.

## 7. Origin and CSRF enforcement

A new `internal/httpapi` middleware, `withRemoteManagementSecurity`,
applied only when remote management is enabled, wraps every route not
on the public allowlist (§10):

1. **Session**: a valid, non-expired session cookie is required, or
   401.
2. **CSRF** (unsafe methods only - POST/PUT/PATCH/DELETE): the
   `X-CSRF-Token` header must match the session's own token
   (constant-time compare), or 403. GET/HEAD never require it.
3. **Origin**: for any request bearing an `Origin` header, it must
   exactly equal the configured `ExternalOrigin` - unlike the existing
   loopback-oriented `checkLocalActionOrigin` (which allows an absent
   Origin through for a non-browser local client), remote management
   is deliberately stricter: a state-changing request with **no**
   Origin header is also rejected (403) unless it is one of the
   narrow, explicitly-listed non-browser exceptions this milestone
   defines (none currently - every real D2B client is the browser
   SPA).
4. **`Sec-Fetch-Site`** (defense in depth, when present): `cross-site`
   is rejected regardless of the above three passing - never the sole
   check, per the header's own documented absence on older/non-browser
   clients.

This is one shared middleware, not 50 individual per-route checks -
satisfying the governing task's own "deny-by-default... do not
manually remember to protect 80 individual endpoints" instruction.

## 8. Forwarding-header contract

The application trusts `X-Forwarded-Proto`/`X-Forwarded-Host`/
`X-Forwarded-For` **only when the direct TCP peer
(`http.Request.RemoteAddr`) is loopback** - the only topology D2B
supports is exactly one same-host reverse-proxy hop, so a non-loopback
direct peer can never legitimately be the trusted proxy, and its
forwarded headers (if any) are simply ignored (not merged, not
partially trusted). When the peer is loopback:

- Exactly one value expected per header; multiple comma-separated
  values, or the header repeated, is rejected outright (`400`) rather
  than "take the first"/"take the last" - an unambiguous single-hop
  contract has no legitimate reason to ever see more than one.
- `X-Forwarded-Proto` must be exactly `https` - `http` (or any other
  value) fails the request closed; there is no insecure fallback.
- `X-Forwarded-Host` must exactly equal `ExternalOrigin`'s own
  host[:port] - a mismatch fails closed.
- Confirmed via Caddy's own documentation (§2) that the reference proxy
  configuration in §14 always overwrites, never passes through, a
  client-supplied value for these three headers - the loopback-peer
  gate above is defense in depth on top of that, not a substitute for
  it, since this application cannot itself verify what a third-party
  proxy binary does at runtime.

Client-IP derivation for rate limiting (§12) uses this same validated
`X-Forwarded-For` value when present and the peer is loopback,
otherwise `RemoteAddr` directly (the desktop/local-headless case,
where there is no proxy).

## 9. Single-administrator authentication model

One administrator identity, no username (the login form has a password
field only). No registration, no RBAC, no OAuth/social login, no TOTP/
WebAuthn, no password-reset email - all explicitly out of scope, this
is a single-operator product.

### 9.1 Password storage

A new `internal/auth` package, `HashPassword`/`VerifyPassword`, using
`golang.org/x/crypto/argon2.IDKey` with RFC 9106's second recommended
parameter set (§2): `time=3, memory=64*1024 (KiB), threads=4,
keyLen=32`. Salt: 16 random bytes from `crypto/rand` per hash, never
deterministic, never reused. Verifier format (self-describing, so a
future parameter upgrade never breaks an existing stored value):

```
argon2id$v=19$m=65536,t=3,p=4$<base64 salt>$<base64 hash>
```

Parsing rejects: an unknown algorithm token, an unsupported `v=`,
non-numeric or absurd `m`/`t`/`p` values (bounded to sane ranges before
ever calling `argon2.IDKey`, so a corrupted/malicious verifier can
never itself trigger a resource-exhausting derivation), malformed
base64, wrong salt length, wrong hash length. Comparison uses
`crypto/subtle.ConstantTimeCompare`. The verifier is stored via the
**existing** `secrets.SecretStore` (a new `SecretTypeAdminPassword`
constant, one fixed key - no per-instance subject ID, since there is
exactly one administrator) - the smallest defensible model per the
governing task's own instruction: no new persistence mechanism, reuses
Stage 20D2A's already-audited encrypted-at-rest headless store
verbatim. Never returned by any API response, never logged, never
copied into frontend state.

### 9.2 Provisioning (local-only, fail-closed if absent)

A new server CLI mode, `--provision-admin-password` (handled in
`handleEarlyFlags`, the same pattern as `-update-helper`), reads a
password from stdin (interactive: a hidden-input prompt with
confirmation; non-interactive/test: a single line, no TTY required so
CI can drive it), never from a command-line argument or environment
variable, hashes it immediately, and writes the verifier through the
same `secrets.HeadlessStore` construction `run()` itself uses -
requiring the same `LoadHeadlessMasterKey()` (`$CREDENTIALS_DIRECTORY`)
and `STREAMING_TREE_DATA_DIR` inputs. Since a manually-invoked process
does not naturally have `$CREDENTIALS_DIRECTORY` set (only systemd's
own `LoadCredential=` populates it for a real unit start), a new
wrapper script, `scripts/provision-admin-password.sh`, invokes this
mode via `systemd-run` with the **exact same** `LoadCredential=`/
`DynamicUser=yes`/`StateDirectory=streaming-tree`/`Environment=
STREAMING_TREE_DATA_DIR=%S/streaming-tree` properties the shipped unit
itself declares - reusing the real production identity and state path
rather than inventing a parallel one, and requiring no new Go-side
special-casing of "am I being provisioned or actually running".
Overwriting an existing verifier requires `--force`, mirroring
`provision-headless-master-key.sh`'s own established convention.
Startup fails closed (§5) if remote management is enabled and no
verifier is provisioned.

## 10. Sessions

Opaque, server-side, in-memory only - never a JWT, per the governing
task's own explicit instruction. A new `internal/auth.SessionStore`:

- Session ID: 32 random bytes from `crypto/rand`, base64url-encoded
  (256 bits of entropy - far beyond any practical guessing budget).
- CSRF token: a second, independent 32 random bytes, generated fresh
  per session, never derived from the session ID.
- Tracked per session: ID (map key, never logged - see §16), issued
  time, last-activity time, absolute-expiry time, CSRF token. No IP/
  User-Agent/analytics metadata - the governing task explicitly
  forbids building an analytics identifier.
- **Idle timeout: 30 minutes.** **Absolute lifetime: 12 hours.** Both
  chosen as defensible, conservative values for a single-operator
  remote-management surface (long enough for a real working session,
  short enough that a forgotten open tab does not stay authenticated
  indefinitely); recorded here as the contract, not left implicit.
- A restart empties the session set (in-memory only) - documented,
  accepted consequence, not a bug: a fresh login is required after
  every service restart.
- A local admin-password reset invalidates every active session
  immediately (§9.2's `--force` path clears the session store when
  wired through `run()`... concretely: the store is only constructed
  at process start, so a reset performed while the service is stopped
  already achieves this by construction; a reset is not supported
  while the service is running in this milestone - simpler and no
  weaker, since the operator action is inherently local/offline
  already).
- Cleanup: a lazily-swept map (checked on every session lookup, no
  separate background goroutine/ticker needed at this scale) evicts
  expired entries so memory stays bounded by the number of genuinely
  active sessions, not the number that ever existed.

## 11. Session cookie

One cookie, name `__Host-streaming-tree-session` (§2's `__Host-`
research applied): `Secure`, `HttpOnly`, `SameSite=Strict` (the SPA
never depends on a cross-site top-level navigation carrying the
session - login is always a same-origin fetch from the already-loaded
login page, so `Strict` costs nothing and is the strictest available
option, matching the governing task's own preference absent contrary
evidence), `Path=/`, no `Domain` attribute (required by `__Host-`
itself), `Max-Age` set to the absolute-lifetime value (12 hours) -
never a "session cookie with no Max-Age" (that would outlive the
server's own absolute-expiry tracking in a browser that restores
tabs). Value is the opaque session ID and nothing else - never the
password, hash, CSRF token, or any provider token.

## 12. Login rate limiting

A new `internal/auth.LoginLimiter`: per-client-IP token-bucket-style
bound (5 failed attempts per IP per 5-minute window) plus a smaller
global secondary bound (30 failed attempts per 5-minute window across
all IPs) to blunt a distributed attempt without creating an easy
denial-of-service against the single legitimate administrator IP.
Client IP is derived per §8's contract. Entries expire via the same
lazy-sweep-on-access pattern as sessions - bounded memory, no
unbounded per-attacker history. `429` with a computed `Retry-After`
when a bound is hit. Never logs the attempted password (§13/§14). A
successful login does not, itself, reset the per-IP failure counter
early (avoids an attacker using a cheap "probe with the right password
occasionally" trick to keep their own budget topped up) - it simply
lets the window expire naturally like any other entry.

## 13. Logging boundaries

Permitted, bounded log events: `remote login failed`, `remote login
succeeded`, `rate limit activated`, `session expired`, `logout`,
`remote shutdown requested`. Never logged, anywhere, under any
circumstance: the password, the password hash/verifier, a session ID
(a bearer credential - logging it is equivalent to logging a password),
a CSRF token, the raw `Cookie`/`Authorization` header, a provider
token/stream key, or a full request-header dump on an auth failure. If
request correlation is ever needed, a separate non-secret random
request ID (already independent of any of the above) is used - not
implemented in this milestone since nothing here yet needs it.

## 14. Security headers (management routes only)

Applied by the same `withRemoteManagementSecurity` middleware, only
when remote management is enabled, and only for non-static-asset
management responses (never applied to `/api/public/*` or overlay
routes - §17 - which must keep rendering inside OBS Browser Source
unmodified):

- `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'`
  (audited against the real production Vite build output before
  shipping - no `unsafe-inline` added without first confirming the
  built bundle needs it).
- `X-Content-Type-Options: nosniff`.
- `Referrer-Policy: same-origin`.
- `Cache-Control: no-store` on `/api/auth/*` responses specifically
  (session/CSRF-bearing) - other authenticated management API
  responses are audited case-by-case rather than blanket-disabled,
  since the existing production static-asset caching strategy (long-
  cache hashed assets, `no-cache` on `index.html`) is already correct
  and must not be blindly overridden.

## 15. Public route allowlist (deny-by-default boundary)

When remote management is enabled, `withRemoteManagementSecurity`
requires authentication for every request **except**:

- `GET/HEAD /api/health` (minimal liveness, already minimal - §16's
  own audit found no change needed).
- `GET/POST /api/auth/*` (the new login/session-bootstrap/logout
  surface itself - see §18).
- Any request whose path is **not** under `/api/` (the embedded
  frontend's static assets and SPA `index.html`, including the login
  page shell itself) - `/legal/*` remains public, unchanged, exactly
  like every prior stage.
- `/api/public/*` - the **existing**, pre-established public-overlay/
  widget prefix. Not newly invented for D2B: this repository's router
  already separates every public overlay/widget route under this exact
  prefix (chat overlays, alert profiles, visual-asset content, audio
  output, widgets). D2B does not add remote reachability to these
  routes (§17) - the proxy configuration in §20 does not forward them
  - but the backend-side middleware still recognizes the prefix so a
  future, deliberate D2C decision to proxy them does not require
  touching this middleware's own classification logic again.

Everything else under `/api/` - `/api/about`, `/api/platforms`,
`/api/runtime/*`, `/api/accounts/*`, every engagement/chat-overlay/
outbound-chat/chat-automation/alert/visual-template/visual-asset/
visual-package/donation-source/audio/audio-asset/goals/system/updates
route - requires authentication. This is the deny-by-default guarantee
the governing task requires: a future route added anywhere under
`/api/` (outside the two narrow public prefixes) is automatically
protected without anyone needing to remember to wrap it.

## 16. Health endpoint audit

`GET /api/health` (read directly, `internal/httpapi/health.go`)
already returns only `status`, `service`, `version`, `uptimeSeconds`,
`time` - no provider configuration, no account names, no stream
destinations, no filesystem path, no secret-store status, no session
information. No change needed; recorded as an explicit audit result,
not assumed.

## 17. Public overlays - D2B policy (audit only, no exposure) - PRE-
20D2C correction: proxy exposure boundary

Two different questions must not be conflated, and an earlier version
of this section conflated them:

- **Backend auth classification**: does the Go backend's own
  authentication middleware gate this route? `/api/public/*` is
  deliberately *not* gated (§15) - this is the existing local-overlay
  contract (`internal/domain/chatoverlay`/`alerts`/`audio`/`goals`),
  unchanged by D2B, and it must stay that way: an OBS Browser Source
  running locally has no session cookie and was never meant to
  authenticate.
- **Reverse-proxy Internet reachability**: does the operator's own
  reverse proxy forward this path from the public Internet to the
  backend at all? This is an entirely separate question, decided by
  the proxy configuration, not by the backend's auth middleware.

The original reference Caddy configuration (§20) was a bare
`reverse_proxy 127.0.0.1:8080` with no matcher. Confirmed directly
against Caddy's own current documentation (research date 2026-08-19):
*"If the matcher token is omitted, it is the same as a wildcard
matcher (`*`)"* - a bare `reverse_proxy` forwards **every** path,
including `/overlay/*` and `/api/public/*`. That directly contradicted
this document's own claim, in this same section, that D2B "does not
expose `/overlay/*` or `/api/public/*` through the reverse-proxy
example in §20." The claim was wrong; the code (an intentionally
unauthenticated backend route) was always correct. **This section and
§20 are now corrected, not the backend.**

The fix does not touch backend authentication - adding a session
requirement to `/api/public/*` would incorrectly conflate the two
security models above and break every local OBS Browser Source. The
fix is entirely in the reverse-proxy layer: §20's corrected reference
configuration explicitly excludes `/overlay/*` and `/api/public/*`
from the proxied surface (a Caddy `handle` block matching those two
path prefixes, responding `404` before the catch-all `reverse_proxy`
handle block ever runs - `handle` blocks are mutually exclusive and
Caddy sorts them by its own built-in directive order regardless of
textual position, so this is correct regardless of which block is
written first). `docs/examples/Caddyfile.remote-management` carries
the real, complete, corrected file; `scripts/verify-linux-remote-
management.mjs`'s own ephemeral-TLS-proxy test harness implements the
identical policy and proves it with real HTTP requests through the
proxy boundary, not merely a string check against the Caddyfile.

`publicSlug` values (`internal/domain/chatoverlay`, `internal/domain/
alerts`, `internal/domain/audio`, `internal/domain/goals`'s widget
profiles) are generated as high-entropy random tokens at creation time
(audited: existing generation already uses `crypto/rand`-backed
identifiers of a length providing a strong capability-token entropy
boundary for the existing loopback-only threat model). D2B does not
change generation and does not rotate existing slugs. With the
corrected reference proxy configuration, `/overlay/*` and
`/api/public/*` are genuinely not reachable through the D2B management
origin - not merely undocumented-but-actually-open, as the prior,
uncorrected reference configuration would have made them. The future
D2C rule, recorded now rather than left implicit: **remote overlays
must not share the authenticated management origin** - a separate
capability origin (e.g. `overlay.example.com`) is the preferred future
architecture, so a compromised or leaked overlay page can never read
the management session cookie (different origin entirely), and
management session cookies (`__Host-`-scoped, `Path=/`, no `Domain`)
are never sent to any other origin regardless. This document does not
implement that separate origin - it is explicit Stage 20D2C scope.
Existing slugs are not claimed strong enough for unconditional future
Internet exposure merely because they were sufficient for a loopback-
only threat model; a dedicated entropy/rotation/revocation review is
explicit required-before-D2C future work, not assumed satisfied here.

## 18. Authentication API

`GET /api/auth/session` - bootstrap: 200 with `{"authenticated":
true, "csrfToken": "..."}` if a valid session cookie is present, 200
with `{"authenticated": false}` otherwise (never 401 for this specific
bootstrap check - the frontend calls it unconditionally on load to
decide whether to render the login page or the app shell).

`POST /api/auth/login` - strict JSON, `{"password": "..."}` only,
bounded body size, unknown fields rejected, `Content-Type:
application/json` required (mirrors the existing `decodeJSONWithLimit`
convention). Origin-checked per §7 even though no session exists yet
(the check does not require authentication, only a valid Origin).
Rate-limited per §12. Constant-time verification via `VerifyPassword`.
On success: a fresh session and CSRF token are generated (never reused
across logins), the `__Host-` cookie is set, and the response body
carries the CSRF token (so the frontend never has to make a second
round trip). On failure: `401`, a generic message (no distinction
between "wrong password" and any other state, since there is only one
account - nothing to enumerate).

`POST /api/auth/logout` - requires an existing valid session + CSRF
(it is a state-changing action), deletes the server-side session,
clears the cookie (`Max-Age=0`).

## 19. Remote-safe shutdown

`POST /api/system/shutdown`'s existing Origin check
(`checkLocalActionOrigin`, `local_action.go`) is preserved unchanged
for the local/desktop/D2A-only case. When remote management is
enabled, the route additionally passes through
`withRemoteManagementSecurity` (§7) like every other non-public
`/api/` route - session + CSRF + strict Origin, on top of, not instead
of, the existing check. No second shutdown implementation: the same
`context.CancelFunc` path Stage 20A/20D2A already established is
reused verbatim. The frontend keeps its existing explicit confirmation
step. `Restart=on-failure` (the shipped unit's own directive) does not
restart a clean, zero-exit-code shutdown - `on-failure` only triggers
after a non-zero exit or a signal-terminated/timed-out stop, both of
which the existing graceful `<-ctx.Done()` path already avoids;
audited, no unit change needed.

## 20. Reverse-proxy reference configuration (Caddy)

```caddyfile
stream.example.com {
    @excludedLocalOnlySurface {
        path /overlay/* /api/public/*
    }
    handle @excludedLocalOnlySurface {
        respond 404
    }

    handle {
        reverse_proxy 127.0.0.1:8080
    }
}
```

A real, standalone copy of this exact configuration lives at
`docs/examples/Caddyfile.remote-management` - not merely a doc-comment
snippet, so an operator can copy the actual file.

Caddy terminates HTTPS (automatic certificate management is Caddy's
own concern, external to this application), overwrites `X-Forwarded-
For`/`X-Forwarded-Proto`/`X-Forwarded-Host` with real values per §2's
confirmed default behavior, and proxies only to the loopback backend -
never to the MediaMTX Control API or RTMP listener, both of which stay
entirely unreferenced by the proxy configuration. The `@excludedLocal
OnlySurface` `handle` block (§17's own PRE-20D2C correction) refuses
`/overlay/*` and `/api/public/*` with a bare `404` before the catch-all
`handle` block's own `reverse_proxy` ever runs - `handle` blocks are
mutually exclusive and Caddy sorts them by its own built-in directive
order regardless of the order they are written in the file (confirmed
directly against Caddy's own current documentation, research date
2026-08-19), so this is correct independent of block ordering in the
file. The application is not installed, configured, or started by this
repository's own tooling; this block is operator-applied guidance,
documented in `docs/linux-headless-server.md`'s own provisioning-
sequence style.

## 21. Operator provisioning sequence

Every command below reflects the actual, tested design in this
document - none is aspirational or untested:

1. Install the package (`sudo dpkg -i streaming-tree-for-obs_*.deb`) -
   the unit is installed disabled, remote management is not enabled.
2. Provision the headless secret-store master key (Stage 20D2A,
   unchanged): `sudo scripts/provision-headless-master-key.sh
   /etc/streaming-tree/master.key`.
3. Provision the administrator password:
   `sudo scripts/provision-admin-password.sh`.
4. Configure the reverse proxy on this same host (§20 /
   `docs/examples/Caddyfile.remote-management`), pointing at
   `127.0.0.1:8080` (or the configured port) and terminating HTTPS for
   the chosen domain.
5. Enable remote management via a systemd drop-in (never editing the
   package-owned unit file directly): `sudo systemctl edit
   streaming-tree.service`, adding
   `STREAMING_TREE_REMOTE_MANAGEMENT=true` and
   `STREAMING_TREE_REMOTE_MANAGEMENT_ORIGIN=https://<your-domain>`.
6. `sudo systemctl daemon-reload && sudo systemctl enable --now
   streaming-tree.service`.
7. Inspect status/logs: `systemctl status streaming-tree.service`,
   `journalctl -u streaming-tree.service`.
8. Sign in at `https://<your-domain>` with the provisioned
   administrator password.
9. To stop/disable: `sudo systemctl disable --now
   streaming-tree.service`.
10. To back up: stop the service cleanly, back up the state directory
    together with the master-key file (Stage 20D2A's own backup
    contract, unchanged - losing either one alone makes encrypted
    secrets, including the administrator password verifier,
    unrecoverable).
11. To remove the package without destroying persistent state:
    ordinary `sudo dpkg -r streaming-tree-for-obs` (not `--purge`)
    preserves the state directory and the master-key/administrator-
    password material, exactly like Stage 20D2A's own existing
    removal contract.

## 22. Non-scope (restated)

No remote overlay exposure, no remote RTMP/RTMPS/SRT, no MediaMTX
external auth, no multi-hop/CDN proxy chain, no application-managed
firewall, no embedded TLS/ACME, no username/registration/RBAC/OAuth/
TOTP/WebAuthn, no password-reset email, no JWT, no persistent bearer-
token database. Windows/macOS/Linux-desktop packaged builds and plain
`--headless` (without `--remote-management`) are functionally
unchanged - the shared `internal/auth` code compiles everywhere but
activates nowhere outside the explicit opt-in.
