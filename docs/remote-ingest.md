# Stage 20D2C - remote OBS ingest and remote overlay capability plane

This document is the governing contract for Stage 20D2C. It is written and
committed before any Stage 20D2C product code, per the milestone's own
requirement. It builds directly on:

- Stage 20D2A (`docs/linux-headless-server.md`) - the headless service
  foundation and the encrypted headless secret store;
- Stage 20D2B (`docs/remote-management.md`) - the authenticated remote
  management/control plane, its session/CSRF/Origin model, and its reverse-
  proxy contract;
- PRE-20D2C - the correction that separated BACKEND AUTH CLASSIFICATION from
  REVERSE-PROXY INTERNET REACHABILITY for the management origin.

Stage 20D2C does not modify D2A or D2B behavior for any deployment that does
not explicitly opt into remote ingest. A desktop build, a plain `--headless`
service, and a `--headless --remote-management` service with remote ingest
left disabled are all unchanged by this stage.

## 1. Primary-source research (this date)

Research date: 2026-08-20. Primary sources only, quoted verbatim below with
their origin. This section will be extended, not rewritten, if later work in
this stage requires further primary-source lookups.

**MediaMTX v1.19.3 (pinned, tag-exact, not rolling docs)** - fetched directly
from `raw.githubusercontent.com/bluenviron/mediamtx/v1.19.3/...`, the exact
tag this project's `internal/runtime/mediamtx.SupportedVersion` pins to:

- `mediamtx.yml` (the shipped reference config for this exact tag):
  - `rtmpEncryption`: "Use the secure protocol variant (RTMPS). Available
    values are `no`, `strict`, `optional`."
  - `rtmpAddress`: "Address of the TCP/RTMP listener. This is needed only
    when encryption is `no` or `optional`."
  - `rtmpsAddress`: "Address of the TCP/RTMPS listener. This is needed only
    when encryption is `strict` or `optional`."
  - `rtmpServerKey` / `rtmpServerCert`: "Path to the server key"/"server
    certificate. This is needed only when encryption is `strict` or
    `optional`."
  - `rtmpTrustedProxies`: "IPs or CIDRs of proxies placed before the RTMP
    server."
  - `authMethod`: "internal" by default; "Internal database: credentials are
    stored in the configuration file."
  - `authInternalUsers` shape: `user`, `pass`, `ips`, `permissions: [{action,
    path}]`.
- `docs/2-features/06-authentication.md` (this exact tag):
  - Hashed password syntax: `sha256:<base64(sha256(password))>` (generated
    reference-equivalent to `openssl dgst -binary -sha256 | openssl base64`)
    or `argon2:<phc-string>`. Verified example:
    `pass: sha256:BdSWkrdV+ZxFBLUQQY7+7uv9RmiSVA8nrPmjGjJtZQQ=`.
  - `ips`: "IPs or networks allowed to use this user. An empty list means any
    IP."
  - `path` under `permissions`: "An empty path means any path. Regular
    expressions can be used by using a tilde as prefix."
  - `user: any` - "'any' means any user, including anonymous ones."
  - `action` accepts (at minimum) `publish`, `read`, `playback`, and `api` -
    the Control API is gated by the same `authInternalUsers` mechanism as
    every other action when `authMethod: internal` is in effect.
  - RTMP/RTMPS credential passing: "Use the `user` and `pass` query
    parameters: `rtmp://localhost/mystream?user=myuser&pass=mypass`" -
    contrasted explicitly against RTSP's `rtsp://user:pass@host` and SRT's
    stream-id-embedded form. **This means the OBS-facing RTMPS URL itself
    carries the plaintext credential in its query string** - a fundamental
    property of the RTMP protocol, not a bug in this configuration. §7 and
    §29 below address the consequences.

**Caddy (current official docs, fetched today)** -
`caddyserver.com/docs/caddyfile/matchers`:
- `path` matcher: "Slashes are significant. For example, `/foo*` will match
  `/foo`, `/foobar`, `/foo/`, and `/foo/bar`, but `/foo/*` will _not_ match
  `/foo` or `/foobar`."

  **This confirms the exact defect PRE-20D2C's own §4 flagged**: the
  corrected `docs/examples/Caddyfile.remote-management` matcher
  `path /overlay/* /api/public/*` does not match a request to the bare
  `/overlay` or `/api/public` (no trailing segment). §6 below corrects this.

**Cookies / RFC 6265** - `rfc-editor.org/rfc/rfc6265` §8.5 ("Weak
Confidentiality"): "Cookies do not provide isolation by port. If a cookie is
readable by a service running on one port, the cookie is also readable by a
service running on another port of the same server." This is the exact
primary-source basis for §9's mandatory hostname-not-port separation between
the management and overlay origins.

**systemd credentials** - already primary-source-verified and committed by
Stage 20D2A in `docs/linux-headless-server.md` (its own research, re-used
here rather than re-derived): `LoadCredential=` (systemd 247+) exposes a file
to the service only via `$CREDENTIALS_DIRECTORY/<name>`
(`/run/credentials/<unit>/<name>`), access restricted at the kernel level to
the service's own user, never propagated as an environment variable to
children. This stage reuses that exact mechanism for RTMPS key/certificate
delivery (§8) rather than inventing a second one.

## 2. Threat model

Three parties interact with a Stage 20D2C deployment over the public
Internet:

1. **The operator's own browser**, authenticated against the D2B management
   plane, configuring and monitoring the service.
2. **OBS**, running on the operator's own streaming machine (which may or may
   not be the same machine as the server), publishing one encrypted,
   authenticated RTMPS stream.
3. **Arbitrary Browser Source clients** - in practice, OBS's own embedded
   browser source rendering a chat/alert/audio/goals/supporter overlay - which
   must be able to fetch a specific overlay's public assets without ever
   authenticating as the operator.

Everyone else on the Internet is an attacker. The design goal is: possession
of a management session cookie, a remote overlay capability URL, or the
RTMPS ingest credential must each grant exactly the narrow capability they
are for, and nothing else. In particular:

- An attacker with a remote overlay URL must not be able to reach the
  management API, rotate the ingest credential, or read another overlay's
  configuration.
- An attacker who can reach the RTMPS listener without the ingest credential
  must not be able to publish, read, or reach the Control API.
- An attacker with the RTMPS ingest credential (e.g. because the operator's
  OBS machine was compromised) can publish garbage video to the one canonical
  path - a real but bounded risk the operator can always recover from by
  rotating the credential - but must not gain read access, Control API
  access, or any HTTP-level capability.
- A network observer of the RTMPS wire traffic must not be able to recover
  the ingest credential (hence RTMPS, not RTMP - see §7).
- A passive observer of the management or overlay HTTPS traffic must not be
  able to recover the management session cookie or a remote overlay
  capability token from a different hostname's traffic (hostname separation,
  see §9).

## 3. Explicit remote-ingest enablement

A new explicit opt-in, `--remote-ingest` (CLI flag, mirroring the existing
`--headless`/`--remote-management` pattern in `apps/server/cmd/server/main.go`
- see its own `handleEarlyFlags`/`headlessMode`/`remoteManagementCLIFlag`
precedent), gates every behavior in this document. Remote ingest is refused
at startup unless both:

- `--headless` is active, and
- `--remote-management` is active,

exactly mirroring the existing `remoteManagementEnabled && !headlessMode`
refusal in `main.go` (docs/remote-management.md §3). Never inferred from
Linux `GOOS`, systemd, certificate presence, non-loopback interfaces, DNS, or
environment alone - an explicit boolean, read once at startup, like every
other D2A/D2B gate.

Effect on existing modes:

- Desktop mode: unchanged, `--remote-ingest` is not offered/accepted.
- Plain `--headless` (no `--remote-management`): unchanged, loopback-only,
  exactly as D2A left it.
- `--headless --remote-management` without `--remote-ingest`: unchanged,
  exactly the D2B/PRE-20D2C behavior audited and corrected already - the
  MediaMTX RTMP listener remains loopback-only, `rtmpEncryption: "no"`.
- `--headless --remote-management --remote-ingest`: this document's scope.

The Go management HTTP listener itself remains loopback-only in every mode,
including this one - remote reachability continues to be entirely the
reverse proxy's job (unchanged from D2B, re-confirmed by the PRE-20D2C
`docs/platform-support.md` §18 correction). Remote ingest only ever relaxes
the MediaMTX RTMP/RTMPS listener, never the Go listener.

## 4. RTMPS policy and the branch-input contract (the "optional" resolution)

The pinned MediaMTX v1.19.3 `rtmpEncryption` accepts `"no"`, `"strict"`, or
`"optional"`. The governing requirement is that remote ingest must never
leave plaintext RTMP *remotely* available. Naively, `"strict"` (RTMPS only,
one listener) looks like the obvious choice - but this project's own existing
architecture creates a real conflict `"strict"` cannot satisfy:

`apps/server/internal/runtime/branch/manager.go` reads
`snapshot.Connection.PublishURL` as its own FFmpeg branch input, and that
same value (`internal/runtime/mediamtx/supervisor.go`:
`PublishURL: "rtmp://" + s.options.RTMPAddress + "/" + s.options.IngestPath`)
is a **plain `rtmp://` URL** shared between "the URL OBS publishes to" and
"the URL Streaming Tree's own branch FFmpeg processes read from." If the one
RTMP listener became RTMPS-only, branch FFmpeg would need to read via
`rtmps://` and would need either a trusted certificate chain or a
verification bypass - and the governing task explicitly forbids smuggling a
TLS-verification bypass into an Internet-facing code path.

**Resolution: `rtmpEncryption: "optional"`**, which the pinned reference
config confirms opens `rtmpAddress` AND `rtmpsAddress` simultaneously, each
independently configurable:

- `rtmpAddress` stays exactly what it is today - `127.0.0.1:1935` by
  default, unconditionally loopback-validated by the existing
  `validateLoopbackAddress` call in `config.loadMediaMTX` (unchanged, no new
  code needed here). Branch FFmpeg keeps reading plain `rtmp://127.0.0.1:...`
  exactly as it does in every mode today - **zero change to
  `internal/runtime/branch`**.
- `rtmpsAddress` is a *new*, remote-ingest-only listener, bound to a
  configurable non-loopback address only when `--remote-ingest` is active,
  serving RTMPS with the operator-supplied certificate (§8).

This satisfies the letter of the requirement ("no plaintext RTMP remotely
available" - `rtmpAddress` never leaves loopback, remote-ingest or not) and
the spirit (OBS's actual publish path is RTMPS-only, credential-protected,
in-transit-encrypted) without inventing a second ingest path, a second branch
runtime, or a TLS-bypass. `"strict"` is not used. This is a deliberate
divergence from an initial naive reading of the governing task's "strict
mode" suggestion, justified by the branch-input conflict above; the outcome
- no remotely-reachable plaintext RTMP - is identical.

Both listeners share one MediaMTX process, one `paths:` section, one
canonical ingest path (`opts.IngestPath`, unchanged - no second channel is
introduced, per the governing task's explicit prohibition on inventing
multiple remote channels).

## 5. MediaMTX authentication model

`authMethod: internal` with exactly two `authInternalUsers` entries, added to
the generated config only when `--remote-ingest` is active (a plain
`--headless --remote-management` deployment's generated config is unchanged -
no `authMethod`/`authInternalUsers` keys at all, exactly as today):

1. **Remote publisher identity** - `user: streaming-tree-obs` (fixed,
   non-secret service identity, plaintext - the username is not the secret,
   only `pass` is), `pass: sha256:<verifier>` (§6), `ips: []` (empty - a
   remote publisher's source IP is not known in advance and is not the
   security boundary here, the credential is), `permissions: [{action:
   publish, path: <IngestPath>}]` - publish only, to the one canonical path,
   nothing else. No `read`, no `playback`, no `api` permission is ever
   granted to this identity.
2. **Local internal identity** - `user: any` (MediaMTX's own documented
   wildcard - "any user, including anonymous ones"), `pass:` empty, `ips:
   [127.0.0.1, ::1]` (this is the actual boundary: only a loopback caller may
   use this identity, regardless of what credentials, if any, it presents),
   `permissions: [{action: read, path: <IngestPath>}, {action: api}]` - read
   access to the canonical path (branch FFmpeg) and Control API access (the
   Go backend's own `apiclient.go` polling), and nothing else. No `publish`
   permission is granted to this identity, so a loopback caller cannot
   silently override the remote publisher either.

A request that matches neither entry (wrong credentials, wrong IP for the
`any` entry, wrong path, wrong action) is refused by MediaMTX itself -
default-deny, enforced by MediaMTX's own permission engine, not re-derived by
this project.

This satisfies the governing task's explicit test matrix (§13 of the
governing task): valid credential + correct path -> publish succeeds; missing
credential -> fails; wrong credential -> fails; valid credential + wrong path
-> fails (no permission entry matches); valid remote credential attempting
`read` -> fails (only `publish` is granted); valid remote credential
attempting `api` -> fails (same reason).

## 6. Credential generation, verifier persistence, one-time display

The remote ingest secret is a machine-generated ~256-bit capability, not a
human-chosen password - a different threat model from the D2B administrator
password, which stays Argon2id because it is human-chosen and thus subject to
guessing/dictionary attack in a way a 256-bit random value is not.

- Generate 32 bytes via `crypto/rand`, encode filesystem/URL-safe (base64url,
  no padding - avoids `+`, `/`, `=` needing escaping inside the RTMP query
  string OBS will carry).
- Compute the MediaMTX-native verifier: `sha256:` + base64-standard-encode of
  the raw SHA-256 digest of the generated secret - the exact format
  `docs/2-features/06-authentication.md` documents (§1). A one-way SHA-256
  verifier is acceptable here specifically because the source secret already
  contains 256 random bits; unlike the administrator password, there is no
  human-guessable structure a KDF would need to slow down.
- **Lifecycle**: generate -> return the plaintext secret once, in the
  provision/rotate HTTP response body only (`Cache-Control: no-store`, never
  logged) -> persist only the `sha256:` verifier -> the generated MediaMTX
  config (§4/§5) is regenerated with that verifier, never the plaintext ->
  the plaintext is never retrievable again through any API.
- **Correction (implementation commit, 2026-08-20)**: this section originally
  said the verifier would be stored in SQLite, describing that as "the
  existing project precedent." Auditing the real code before implementing
  (`internal/auth/adminauth.go`) showed this was wrong: the D2B administrator
  password verifier is stored via the `secrets.SecretStore` abstraction (the
  OS keyring on desktop, the AES-256-GCM `HeadlessStore` in headless mode),
  not SQLite - `secrets.BuildKey(secrets.SecretTypeAdminPassword,
  AdminPasswordSubjectID)`. The remote-ingest verifier follows that real
  precedent instead: a new `secrets.SecretTypeRemoteIngestPublisherPassword`
  key, stored and read through the same `SecretStore` interface
  (`internal/auth/remoteingestcredential.go`, mirroring `adminauth.go`'s
  shape exactly), never SQLite. It is one-way and, per the threat model
  above, non-secret on its own even so - recovering the original 256-bit
  secret from a SHA-256 digest is not feasible - but using the store already
  reserved for security-sensitive verifiers is still the correct, consistent
  choice, not merely an acceptable one. No plaintext retention is required or
  added anywhere, including within the store itself - only the verifier is
  ever written.
- Rotation invalidates the previous verifier immediately (config regenerated,
  MediaMTX reloaded - see §11 for the safety gate around this).
- Disable/revoke removes the `authInternalUsers` remote-publisher entry
  entirely from the generated config (regenerated, MediaMTX reloaded) -
  no credential can authenticate afterward, by construction, not by a
  separate "disabled" flag MediaMTX would need to interpret.

## 7. TLS certificate/key delivery

Streaming Tree does not become a certificate authority: no ACME client, no
Let's Encrypt/Certbot integration, no embedded Caddy, no certificate issuance
service is added anywhere in this stage. The operator supplies the RTMPS
server certificate and private key, exactly as they already must for the D2B
Caddy reverse proxy in front of the management/overlay origins (Caddy
obtains its own certificate automatically for those HTTP origins; the RTMPS
listener is a raw TCP/TLS service MediaMTX terminates itself, never proxied
through Caddy - Caddy has no first-class raw-TCP-with-SNI-routing story that
would not add its own complexity and attack surface for no benefit here).

Delivery reuses the exact `LoadCredential=` pattern Stage 20D2A already
established and documented (`docs/linux-headless-server.md`, cited in §1
above) for the master key, rather than inventing a second secret-delivery
mechanism:

```
LoadCredential=streaming-tree-rtmps-key:/etc/streaming-tree/rtmps.key
LoadCredential=streaming-tree-rtmps-cert:/etc/streaming-tree/rtmps.crt
```

exposed to the service only via `$CREDENTIALS_DIRECTORY/streaming-tree-rtmps-key`
and `$CREDENTIALS_DIRECTORY/streaming-tree-rtmps-cert`, read once at startup
when `--remote-ingest` is active, never via an environment variable, a
command-line flag, or a value this application's own config parses as a
secret. The certificate itself is not secret and could in principle be
delivered another way, but is delivered identically to the key for
consistency and because both files must be updated together on renewal.

**Fail closed**: if `--remote-ingest` is active and either credential file is
missing, unreadable, or fails to parse as a valid key/certificate pair,
startup fails with a clear error - the same "fail loudly" philosophy
`config.Load()` and the D2A master-key path already apply.

**Trust model documented for the operator** (not enforced by this
application, since it is inherently the operator's/OBS's own trust
decision): a publicly-trusted CA certificate is the simplest production path,
since OBS's own TLS stack (built on the same trust store as most desktop
software) accepts it with no extra configuration. A private/local CA works
only if its root is imported into the trust store OBS itself consults, which
is subject to OS/packaging-specific constraints outside this project's
control (and, on a sandboxed OBS install such as Flatpak, may not be
possible at all without exporting to the sandbox's own trust store). A bare
self-signed leaf certificate is not a supported production configuration:
if OBS rejects it, that is OBS correctly doing its job, not a bug to route
around with a verification bypass.

## 8. Ingest credential management API

New endpoints, all behind the existing D2B deny-by-default management
surface (`withRemoteManagementSecurity` - session + CSRF + Origin, unchanged,
reused as-is) and available only when `--remote-ingest` is active:

- `GET /api/remote-ingest/status` - configured/not-configured, RTMPS
  hostname:port, canonical path, whether MediaMTX currently reports the path
  receiving.
- `POST /api/remote-ingest/provision` - first-time credential generation.
  409 Conflict if already configured (use rotate instead).
- `POST /api/remote-ingest/rotate` - invalidate the old credential, generate
  a new one. Subject to the streaming-safety gate in §11.
- `POST /api/remote-ingest/revoke` - remove the remote-publisher identity
  entirely. Subject to the same gate.

`provision`/`rotate` responses carry the new plaintext secret exactly once,
with `Cache-Control: no-store`, never logged (§1's own finding about RTMP
query-string credentials makes this doubly important - the application's own
logs must not compound a protocol-level leak risk with an application-level
one). None of these routes ever live under `/api/public/*` - they are exactly
as sensitive as the existing D2B admin-password provisioning surface.

## 9. Streaming-safety policy for credential changes

Rotating or disabling the ingest credential invalidates OBS's current
configuration and can interrupt a live publish. `rotate`/`revoke` return
`409 Conflict` while MediaMTX currently reports the canonical path receiving
a stream (queried the same way `internal/runtime/mediamtx/apiclient.go`
already polls path status for the existing ingest-state projection - no new
MediaMTX query mechanism is introduced) - mirroring this stage's own
streaming-active guard pattern rather than inventing a second one. The
management UI must show an explicit confirmation before allowing rotation
even outside that window, explaining that the operator's OBS configuration
will need updating afterward. A MediaMTX config regeneration + supervised
restart (the existing `internal/runtime/mediamtx.Supervisor` restart path,
not a new mechanism) applies a rotated/revoked credential; this is a
controlled, bounded restart, not a silent one, and is refused entirely while
actively receiving.

## 10. Remote overlay origin - mandatory hostname separation

A second external HTTPS origin, distinct from the D2B management origin, is
introduced for Browser Source capability URLs - e.g.
`https://overlay.example.com` alongside `https://stream.example.com`.

**The required property is hostname separation, not mere web-Origin
inequality.** RFC 6265 §8.5 (§1 above): cookies are not port-scoped, so
`https://stream.example.com` and `https://stream.example.com:8443` are
different web Origins (different CORS/SOP boundary) but the *same* cookie
host - a management session cookie set by the first would still be sent to
the second by the browser. The `__Host-` prefix (docs/remote-management.md
§11) restricts by secure-context + no-Domain + Path=/, none of which touch
port. **Changing only scheme or port between the management and overlay
origin is therefore not a valid configuration and must be rejected outright**
by a new validation function alongside the existing
`ValidateRemoteManagementOrigin`: same normalized-host check (host, not
host:port) between the two configured origins fails closed.

The overlay origin config itself reuses `ValidateRemoteManagementOrigin`'s
own rules (HTTPS, no userinfo/path/query/fragment, exactly one origin, no
wildcard) plus the new cross-check against the management origin's host.

## 11. Backend defense-in-depth for the overlay origin

Per PRE-20D2C's own established discipline (never let the proxy be the only
boundary), the backend gains awareness of forwarded overlay requests,
symmetric to `validateForwardedRequest`/`singleForwardedValue`
(`internal/httpapi/remote_management.go`, reused as-is - the same
single-value, no-comma-list, loopback-peer-required contract, just checked
against the overlay origin's host instead of the management origin's):

- A **direct loopback** request (no forwarded headers) to `/overlay/*` or
  `/api/public/*` is unaffected - the existing local OBS Browser Source
  contract, unchanged in every mode including this one.
- A **forwarded** request (peer is the loopback proxy, forwarded headers
  present) targeting `/overlay/*` or `/api/public/*` is accepted only when:
  remote overlay exposure is explicitly configured; the forwarded scheme is
  exactly `https`; and the forwarded host exactly equals the configured
  overlay origin's host. A forwarded management hostname attempting to reach
  these paths, an unrecognized forwarded host, or malformed/duplicated
  forwarded headers are all rejected the same way
  `validateForwardedRequest` already rejects them for the management origin.
- Overlays never require an administrator session cookie - this remains a
  completely separate security domain from D2B's management auth, exactly as
  the governing task requires; the capability token (§12) is the entire
  authorization model for a remote overlay request.

## 12. Remote overlay capability tokens - audit and design

Every existing `NewPublicSlug` generator (`internal/domain/{chatoverlay,
audio,alerts,goals}/ids.go`) generates 20 random bytes (160 bits) via
`crypto/rand`, hex-encoded - confirmed identical across all four domains,
each domain's own comment cross-referencing the others as the shared
convention. `internal/domain/visualasset/asset.go`'s `NewPublicToken`
generates 32 bytes (256 bits), hex-encoded, and is already the widest token
in the codebase, explicitly because it is "the only credential-shaped value
this package ever exposes on an unauthenticated public route."

160 bits already exceeds this stage's 128-bit minimum, but every existing
`NewPublicSlug`'s own doc comment already says explicitly: "not sufficient
authentication for a server exposed to the public network - a future
remote-server stage must add real authentication before that is safe." This
stage is that stage, and takes that existing comment at face value rather
than reinterpreting it: **existing local `publicSlug` values are not
promoted to remote capabilities merely because their entropy happens to be
adequate.**

**Design: separate, nullable remote capability tokens, independent of the
local `publicSlug`.**

- The local `publicSlug` is unchanged in every respect - same generator, same
  persistence, same rotation endpoint, same direct-loopback behavior. Every
  existing local Browser Source URL keeps working exactly as it does today.
- A new, separate `remoteCapabilityToken` (nullable) exists per overlay
  profile (chat overlay, alert profile, audio, each supporter widget), `NULL`
  until the operator explicitly enables remote access for that specific
  overlay. Generated with 32 random bytes (256 bits, matching
  `visualasset.NewPublicToken`'s own precedent for the codebase's one other
  Internet-facing credential-shaped value) via `crypto/rand`,
  base64url-no-padding encoded (shorter in a URL than hex for the same
  entropy, and already the codebase's established alternate encoding
  elsewhere for URL-embedded values).
- The overlay-origin proxy (§14) and the backend's forwarded-request
  handling (§11) accept the `remoteCapabilityToken` in the URL, in the exact
  position the local `publicSlug` occupies today (`{slug}` in the existing
  route patterns) - a forwarded request's `{slug}` path parameter is looked
  up against the remote-capability index, not the local-publicSlug index.
  **A forwarded request presenting a legacy local `publicSlug` value (even a
  valid one) that does not also match a remote capability token is rejected**
  - this is the concrete mechanism behind "the legacy local publicSlug does
  not grant remote capability."
- Lifecycle per overlay profile: disabled (`NULL`, no remote URL exists) ->
  enable (generate) -> rotate (generate a new value, the old one stops
  matching immediately) -> disable/revoke (`NULL` again, immediately). The
  management API returns the full remote overlay URL to the authenticated
  operator on enable/rotate; PRIVACY.md is updated (§16) to disclose that
  anyone possessing a remote overlay URL can view that overlay until it is
  rotated or revoked - the same "possession is the capability" model this
  project's local `publicSlug`s already document, now extended to a second,
  explicitly-opt-in, wider-audience surface.
- Not logged, not sent to any telemetry (none exists in this project), never
  written to `docs/progress.md`, never included in an error message.

**Implementation simplification (implementation commit, 2026-08-20)**: this
section's own permission - "If a substantially simpler architecture provides
equivalent properties, explain it in docs/remote-ingest.md BEFORE
implementation" - is exercised here. Rather than adding a
`remoteCapabilityToken` column to each of four separate domain
schemas/repositories/services (`chatoverlay`, `alerts`, `audio`, the
`goals`-owned widget profiles), a single shared mapping table,
`remote_overlay_capabilities(token, domain, local_slug, created_at)`, plus
one small domain package (`internal/domain/remoteoverlay`) and one SQLite
repository, provides every property this section requires:

- local `publicSlug` values remain completely untouched in every domain -
  no schema change to `chatoverlay`, `alerts`, `audio`, or `goals` at all;
- a capability row's mere existence for `(domain, local_slug)` is exactly
  "remote access enabled for that profile" - no row means disabled, matching
  the "nullable" model's own semantics without an actual nullable column
  anywhere;
- issuing a fresh token (`Issue`) atomically replaces any previous row for
  that `(domain, local_slug)` - enable and rotate are the same operation
  from the repository's point of view, exactly as
  `internal/remoteingest.Manager.Provision`/`Rotate` already share
  `generateAndApply` for the equivalent reason;
- `Resolve(ctx, domain, token)` is the one lookup a forwarded overlay
  request needs - it returns the real local slug only for a currently-valid
  token, so a revoked or rotated-away token simply fails to resolve;
- one small, uniform HTTP surface
  (`/api/remote-overlay/{domain}/{slug}/{enable,rotate,disable,status}`)
  replaces four near-duplicate per-domain route sets.

This is a genuine simplification, not a scope reduction: every property
this section's original per-domain-column design specified still holds,
implemented with one shared table and one shared package instead of four
parallel ones.

## 13. `/api/public/*` route remote-safety classification

Every current route under `/api/public/*`
(`apps/server/internal/httpapi/{audio,alerts,chatoverlay,public_widgets,
visualasset}.go`), classified per the governing task's A/B/C scheme:

| Route | Class | Notes |
|---|---|---|
| `GET /api/public/chat-overlays/{slug}/config` | A+B | Required for overlay render; read-only; slug-scoped |
| `GET /api/public/chat-overlays/{slug}/items` | A+B | Read-only, slug-scoped |
| `GET /api/public/chat-overlays/{slug}/stream` (SSE) | A+B | Read-only, slug-scoped, long-lived connection |
| `GET /api/public/alert-profiles/{slug}/config` | A+B | Read-only, slug-scoped |
| `GET /api/public/alert-profiles/{slug}/stream` (SSE) | A+B | Read-only, slug-scoped |
| `GET /api/public/widgets/{slug}/config` | A+B | Read-only, slug-scoped (supporter widgets) |
| `GET /api/public/widgets/{slug}/stream` (SSE) | A+B | Read-only, slug-scoped |
| `GET /api/public/audio/{slug}/stream` (SSE) | A+B | Read-only, slug-scoped |
| `GET /api/public/audio/{slug}/bytes/{token}` | A+B | Read-only; doubly-scoped (slug + a separate per-item bytes token issued by `ConnectRenderer`/current-item flow, not attacker-choosable) |
| `POST /api/public/audio/{slug}/ack` | A+B | The one state-changing route in this namespace - but the mutation is scoped to the caller's own renderer-lease token (`ConnectRenderer`'s return value, required alongside `{slug}`) advancing that overlay's own playback queue state; it cannot affect another overlay, another profile, or any global/management setting. Classified safe by the same reasoning D2B already applied to overlay-owned, non-privileged mutations. |
| `GET/HEAD /api/public/visual-assets/{token}` | A+B | Read-only; the 256-bit `token` alone is the entire capability (not additionally slug-scoped) - already the widest existing token, reused as-is under the overlay origin |

No route in this namespace grants cross-profile enumeration (every lookup is
keyed by an unguessable slug/token, never a sequential id), no route mutates
global/management state, and no route requires promotion to a different
authorization model than "possession of the correct capability value(s)." Ten
of the eleven routes are class A+B and gated by the remote-capability-token
substitution described in §12 - none of those ten is reachable remotely via
its legacy local `publicSlug` alone.

**Visual-asset audit (implementation commit, 2026-08-20) - the eleventh route
is honestly different, not identically gated:** `GET/HEAD /api/public/
visual-assets/{token}` was deliberately never wired into §12's capability
substitution. Audited directly (`internal/domain/visualasset/service.go`,
`blobstore.go`, `validation.go`, `internal/httpapi/visualasset.go`):

- The 256-bit `PublicToken` (`visualasset.NewPublicToken`, `crypto/rand`) is
  itself the entire capability - there is no separate "remote" token layered
  on top the way §12 adds one for the other four domains. **Possession of an
  existing local visual-asset token already grants remote access once the
  overlay origin proxies this route at all** - it is not bound to, or gated
  by, whether the *referencing overlay* has ever had remote access enabled
  for itself. This is the honest model, not a claimed cryptographic binding
  that does not exist.
- This is judged acceptable, not merely convenient, because: `PublicBlobByToken`
  performs a single keyed lookup with no sequential id ever exposed
  (`ErrNotFound` uniformly for "wrong token" and "right token, blob
  vanished" - no near-miss signal); `OpenBlob` is called only with the
  server-computed `SHA256` value already resolved from that lookup, never
  with request-supplied text, so `filepath.Join(blobsDir, sha256Hex)` cannot
  be steered by a caller - path traversal is structurally impossible, not
  merely filtered; `MediaType` is a closed, seven-value set (PNG/JPEG/GIF/
  WebP/WebM/MP4/WOFF2) with **no HTML/SVG/script-capable type ever
  accepted**, and `VerifyTypeAgreement` cross-checks the upload's real
  binary signature (`DetectSignature`) against both the declared type and
  the file extension - an attacker cannot upload an SVG/HTML payload by
  mislabeling it, because the stored `Content-Type` this route later serves
  is never caller-controlled at serve time, only at a validated upload time;
  Range support is the standard library's own `http.ServeContent` (206/416),
  not a hand-rolled implementation; the one route that could enumerate every
  token, `GET /api/visual-assets`, requires an authenticated management
  session (never `/api/public/*`), so a remote, unauthenticated caller with
  no token at all cannot discover any.
- Net effect for the overlay-origin proxy (§14): `/api/public/visual-assets/*`
  is included in the overlay host's allowlist on the strength of this
  audit, not because it was mechanically swept in with the other ten routes.

## 14. Caddy reverse-proxy policy for both origins

`docs/examples/Caddyfile.self-hosted` (new file, §17) supersedes
`docs/examples/Caddyfile.remote-management` as the canonical combined
reference for a Stage 20D2C deployment; the D2B-only file remains for a
deployment that stops at D2B (management only, no remote ingest/overlay).

**Management host** (`stream.example.com`): unchanged from the corrected
PRE-20D2C policy, with the exact-root gap PRE-20D2C's own §1 research above
just confirmed, closed:

```caddyfile
@excludedLocalOnlySurface {
	path /overlay /overlay/* /api/public /api/public/*
}
handle @excludedLocalOnlySurface {
	respond 404
}
handle {
	reverse_proxy 127.0.0.1:8080
}
```

(`/overlay` and `/api/public` added alongside the existing `/overlay/*` and
`/api/public/*` - Caddy's own documentation, quoted in §1, confirms
`/foo/*` does not match the bare `/foo`, so the wildcard-only matcher left a
real, if narrow, gap: a request to exactly `/overlay` or exactly
`/api/public`, with no trailing segment, would have fallen through to the
catch-all proxy handle. Both roots currently 404 from the backend's own SPA
fallback/router in practice, but the proxy boundary must not depend on that
backend behavior remaining true - PRE-20D2C's own core lesson.)

**Overlay host** (`overlay.example.com`): proxies only what a Browser Source
genuinely requests - `/overlay/*` (the SPA overlay routes themselves) and the
eleven `/api/public/*` routes classified in §13, plus the hashed
`/assets/*` bundle the production Vite build serves (verified against the
real build output, not assumed - see §17's implementation note). Every other
path, including every authenticated management route, `/api/auth/*`,
`/legal/*`, and the bare SPA management shell, is unreachable - unknown paths
receive `404`, the same fail-closed default as the management host, never a
"forward everything and rely on backend middleware" catch-all.

**Implementation status (implementation commit, 2026-08-20)**:
`docs/examples/Caddyfile.self-hosted` now exists as the real, standalone
combined reference file described above - not merely a doc-comment sketch.
Its overlay-host `@overlaySurface` matcher is exactly `path /overlay/*
/assets/* /api/public/*`, verified directly against
`internal/httpapi/production.go`'s own request handling (the SPA-fallback
handler answers any non-asset-like path under `/overlay/*` with `index.html`,
identically to every client-side route) and against the real embedded build
output (`internal/webassets/embedded/assets/`) rather than assumed. One
single-page-app bundle serves both the management UI and every public
overlay - there is no separate "overlay-only" bundle to proxy instead, which
is safe because the bundle is client-side code with no embedded secret and
the real security boundary is the backend API surface, the same reasoning
Stage 20D2B already applied to the management origin's own bundle.
`docs/examples/Caddyfile.remote-management` (the D2B-only file) remains
unchanged and still shipped for a deployment that stops at D2B.

## 15. Native CI remote-network test model

The D2C native verification must prove real remote-network behavior, not
localhost-to-localhost behavior relabeled as "remote." This requires:

- A real isolated client/server network boundary on native Linux CI (a Linux
  network namespace + veth pair, or the closest equivalent both GitHub-hosted
  architectures actually support), with non-loopback addresses between the
  two sides.
- A CI-only ephemeral test CA issuing server certificates for deterministic
  test hostnames (e.g. `manage.test`, `overlay.test`, `ingest.test`), trusted
  by the test client, never a production credential and never a `-k`/
  `--insecure`/verification-disabled positive-path test.
- A deterministic synthetic RTMPS publisher (real FFmpeg, not a hand-rolled
  RTMP client) run from the isolated client namespace against the real
  MediaMTX RTMPS listener, proving the full accept/reject matrix from §5 at
  the protocol level.
- End-to-end proof through to a real (test-scoped) destination branch and a
  local sink, reusing the existing branch integration infrastructure rather
  than a second, parallel branch runtime.
- Two independent build-and-verify passes per architecture before this
  stage's evidence is accepted, each from isolated temporary state, no pass
  reusing another pass's ingest credential or remote capability token.

This is deliberately deferred to its own implementation/CI commits later in
this stage rather than designed exhaustively in this contract document -
concretely landing it depends on the exact shape of the credential-management
and MediaMTX-config code this document specifies, which does not exist yet.

## 16. PRIVACY.md, docs/remote-management.md, and MediaMTX credential-logging audit

**Status (implementation commit, 2026-08-20): done, both items.**

- `PRIVACY.md`: disclose remote overlay URLs as viewer-grade capabilities
  (possession = access, until rotated/revoked); disclose that RTMPS
  publisher source IPs are visible to the server/MediaMTX; disclose that
  the generated ingest credential is shown once and stored only as a
  verifier afterward. Done - see the two new bullets under "Local
  application state."
- `docs/remote-management.md`: corrected the "different port, same cookie"
  wording this document's own §10/§1 research showed was too broad if it
  implied full web-Origin isolation; the correction was recorded append-only
  in `docs/progress.md` (the `fix(docs): harden the D2B Caddy exclusion
  matcher and correct the cookie/origin wording` entry, earlier this stage),
  without rewriting the historical PRE-20D2C entry.

**MediaMTX credential-logging audit (primary source, 2026-08-20)** - the
load-bearing question this section's own governing task raised: because
RTMP authentication carries `user`/`pass` in the publish URL's query string
(§1's own confirmed finding), does anything in this project's actual
architecture ever log or expose that plaintext value? Verified directly
against the pinned `v1.19.3` tag's real source, not assumed:

- `internal/servers/rtmp/conn.go`: the read path logs `"is reading from
  path '%s', %s"` using only `res.Path.Name()` - never the query string. The
  publish path has no equivalent connection-established log line at all.
  Connection-lifecycle logs (`"opened"`, `"closed: %v"`) never reference the
  query string either.
- `internal/auth/error.go` / `manager.go` / `log_and_delay_error.go`: every
  authentication-failure error message is generic ("authentication failed",
  "failed to authenticate: <wrapped>") - none embeds the attempted
  username, password, or query string. `LogAndDelayError` logs only that
  generic error text.
- **The one place MediaMTX genuinely does carry the raw query string is its
  own Control API**: `conn.go` sets `c.query = c.rconn.URL.RawQuery` on the
  connection object, which feeds MediaMTX's own `/v3/rtmpconns/list`
  endpoint - so a caller of *that specific endpoint* would see the
  plaintext credential. This project's own `internal/runtime/mediamtx/
  apiclient.go` never calls it: the client only ever calls
  `/v3/config/global/get` and `/v3/paths/list`, and the narrow `pathSource`/
  `pathItem` structs it deserializes into have no field for a connection's
  query string at all - Go's JSON decoder silently drops any such field even
  if a future MediaMTX release added it to that response. A defensive
  comment was added directly on `APIClient` warning against ever adding an
  `rtmpconns` call (or any endpoint that might surface a connection's query
  string) without redacting it first.
- Net conclusion, stated honestly rather than merely hoped: at `logLevel:
  info` (this project's own configured level, unconditionally), MediaMTX
  does not log the ingest credential anywhere in normal operation, and this
  project's own Control API usage structurally cannot see it either. No
  code change was required to achieve this - the existing, narrower
  `apiclient.go` API surface (chosen before this stage existed, for
  unrelated reasons) already had this property; this audit confirms it
  rather than introduces it, and the new guard comment keeps it that way
  going forward.

## 17. Final combined self-hosted acceptance criteria

Stage 20D2C is not complete until, on native Linux CI, on both `linux-amd64`
and `linux-arm64`, across two independent passes each:

- the real `.deb` package installs, provisions master key + admin password +
  RTMPS credential, and starts under systemd;
- the management HTTPS origin, overlay HTTPS origin, and RTMPS ingest
  listener each behave exactly as specified in §5/§10/§11/§13/§14;
- a synthetic FFmpeg RTMPS publish is accepted only with the correct
  credential and canonical path, rejected in every other case in the §5
  matrix;
- ingest state transitions waiting -> receiving -> waiting correctly;
- a real destination branch starts from the remote-published stream and
  reaches a local sink;
- credential rotation/revocation, including a full service restart, behaves
  per §6/§9, with no plaintext credential ever recoverable after
  provisioning;
- remote overlay enable/rotate/disable behaves per §12, with the legacy
  local `publicSlug` never granting remote access;
- the cookie-separation property from §10 is proven by actually inspecting
  which hostname a real browser-equivalent HTTP client sends the management
  cookie to, not merely by asserting the two configured hostnames differ as
  strings;
- MediaMTX's Control API, RTMP/RTMPS ports beyond the one intended listener,
  and the Go management port are all unreachable from the isolated remote
  client network;
- package removal and service teardown leave no orphaned process, network
  namespace, veth interface, or trust-store modification.

## 18. Explicit Stage 20E deferrals

Out of scope for Stage 20D2C, unchanged from the governing task's own list:
public GitHub Releases; macOS signing/notarization; final user-facing manual
hardware validation (a real human running real OBS against this stage's
RTMPS listener); final release hardening/diagnostic polish;
application-managed firewall configuration; embedded ACME/certificate
issuance; general VPN provisioning; RBAC/multi-user accounts; arbitrary
public MediaMTX protocols (RTSP/HLS/WebRTC/SRT/MoQ remain disabled, unchanged
from every prior stage). Actual human OBS configuration against a real
RTMPS endpoint is explicitly Stage 20E's manual validation, never claimed as
"tested in OBS" by this stage's automated FFmpeg-based proof.

## 19. Operator provisioning sequence (systemd)

Extends `docs/remote-management.md`'s own operator provisioning sequence -
read that first. Remote ingest and remote overlay exposure are each enabled
the same way D2B's own `--remote-management` already is: a systemd drop-in
adding `Environment=` lines, never by editing the package-owned unit file
directly (`scripts/systemd/streaming-tree.service` ships with neither
feature enabled, exactly like it ships with `--remote-management` disabled).

1. Complete D2B's own sequence first (master key, administrator password,
   `--remote-management` drop-in, the management reverse-proxy site block).
2. Provision the RTMPS certificate/key at an operator-chosen path outside
   any package-owned directory (e.g. `/etc/streaming-tree/rtmps.key` /
   `rtmps.crt`), with the same discipline §8 requires: root-owned, `0600`
   for the key.
3. `sudo systemctl edit streaming-tree.service` and add:

   ```ini
   [Service]
   LoadCredential=streaming-tree-rtmps-key:/etc/streaming-tree/rtmps.key
   LoadCredential=streaming-tree-rtmps-cert:/etc/streaming-tree/rtmps.crt
   Environment=STREAMING_TREE_REMOTE_INGEST=true
   Environment=STREAMING_TREE_REMOTE_INGEST_RTMPS_ADDRESS=0.0.0.0:1936
   Environment=STREAMING_TREE_REMOTE_INGEST_TLS_KEY_PATH=%d/streaming-tree-rtmps-key
   Environment=STREAMING_TREE_REMOTE_INGEST_TLS_CERT_PATH=%d/streaming-tree-rtmps-cert
   ```

   `%d` is systemd's own specifier for the credentials directory - "the
   value of the `$CREDENTIALS_DIRECTORY` environment variable if
   available" (`systemd.unit(5)`'s own specifier table, current official
   documentation, fetched directly). `Environment=` itself resolves
   specifiers rather than performing shell expansion, and this project's
   own shipped unit already relies on exactly that: `Environment=
   STREAMING_TREE_DATA_DIR=%S/streaming-tree` (the real, already-working
   line in `scripts/systemd/streaming-tree.service`) uses the identical
   mechanism with the `%S` (state directory) specifier - direct, concrete
   precedent from this repository's own already-verified unit, not merely
   an external doc lookup.
4. To also enable remote overlay exposure, add to the same drop-in:

   ```ini
   Environment=STREAMING_TREE_REMOTE_INGEST_OVERLAY_ORIGIN=https://overlay.your-domain.example.com
   ```

   (a different hostname from the management origin's own - §10's own
   mandatory requirement, enforced at startup either way).
5. Configure the overlay reverse-proxy site block from
   `docs/examples/Caddyfile.self-hosted`'s own second site block, alongside
   the management site block from step 1.
6. Configure the host/cloud firewall to allow the chosen RTMPS port
   (`1936` in the example above) - this application never does so itself
   (§10 of the original governing task; unchanged).
7. `sudo systemctl daemon-reload && sudo systemctl restart streaming-tree.service`.
8. Through the authenticated management UI (`RemoteIngestPanel`), provision
   the publisher credential - the one-time secret is shown exactly once;
   configure OBS's custom RTMP service with the RTMPS server address and
   this credential (`docs/remote-ingest.md` §28's own UX requirements).
9. Through each overlay's own management surface (`RemoteOverlayPanel`),
   enable remote access for the specific overlays that need it - never all
   of them by default.
