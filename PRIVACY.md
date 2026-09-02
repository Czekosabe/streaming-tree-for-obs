# Privacy

Streaming Tree for OBS, created by **Czekosabe**
(<https://github.com/Czekosabe>). Canonical repository:
<https://github.com/Czekosabe/streaming-tree-for-obs>.

This document distinguishes two different things: what the application does
**locally, on your own machine**, and what network activity happens **only
when you explicitly enable a provider integration or click an external
link**. It describes the application as it exists today; see
`docs/product-identity-legal.md` for the audit this document is based on.

## Local application state

- Streaming Tree for OBS is **local-first**. It runs as a local backend
  process plus a local web UI you open in your own browser; there is no
  Streaming Tree for OBS cloud account or first-party server.
- Application configuration and state (destination settings, alert rules,
  goal/widget configuration, and so on) are stored in a local SQLite
  database, in your operating system's standard per-user application-data
  directory.
- Destination stream keys and provider OAuth token bundles (Twitch,
  YouTube) are stored using your **operating system's own credential
  store** - Windows Credential Manager, macOS Keychain, or the Linux
  Secret Service, depending on platform - never in a plain application
  file, and never in the SQLite database.
- A Linux instance started explicitly with `--headless` (Stage 20D2A,
  see [docs/linux-headless-server.md](docs/linux-headless-server.md))
  never opens a desktop Secret Service session - it uses a separate,
  file-based secret store instead, still never plaintext:
  AES-256-GCM-encrypted at rest under a master key you provision
  yourself outside the application, decrypted only in memory for as
  long as a value is actually in use. This mode is entirely opt-in and
  local-machine-only; nothing about it exposes the application to a
  network beyond the same loopback-only boundary every other mode
  already has.
- A Linux instance additionally started with `--remote-management`
  (Stage 20D2B, see
  [docs/remote-management.md](docs/remote-management.md)) is opt-in and
  requires `--headless`. Your single administrator password is never
  stored in plain text: only an Argon2id-hashed verifier is stored, in
  the same encrypted headless secret store described above. Once
  enabled, and once you separately configure your own HTTPS reverse
  proxy in front of it, this instance becomes reachable from wherever
  you choose to expose that proxy - the application itself never binds
  to a network interface beyond loopback, and never opens a port for
  you; remote reachability is entirely a consequence of infrastructure
  you set up yourself outside this application.
- The only thing this application stores in your browser (`localStorage`)
  is your interface language preference. No stream key, token, session
  identifier, CSRF token, administrator password, or other secret is
  ever written to browser storage - a remote-management session is
  held only in a browser cookie your browser itself manages.
- Public OBS overlay routes (chat overlay, alert overlay, audio overlay,
  goal/supporter widgets) are **local application routes**, served by your
  own backend for you to add as an OBS Browser Source. They are not hosted
  by a Streaming Tree for OBS cloud service.
- A Linux instance additionally started with `--remote-ingest` (Stage
  20D2C, see [docs/remote-ingest.md](docs/remote-ingest.md)) is opt-in
  and requires `--remote-management`. The generated publisher credential
  is shown to you exactly once, at the moment you provision or rotate
  it; only its one-way SHA-256 verifier is stored afterward (the same
  encrypted headless secret store described above), and it can never be
  displayed again through any route - only rotated or revoked.
  MediaMTX's own connection logs and Control API never record this
  credential's plaintext value (verified directly against MediaMTX's
  own source for the version this application pins), and this
  application's own Control API client only ever reads path/config
  status, never a connection's own credential-bearing query string.
  Once you configure your own TLS certificate and separately expose the
  RTMPS port you choose, the source IP address of whatever publishes to
  it (normally your own OBS installation) becomes visible to this
  application and to MediaMTX, the same way any server sees the IP of
  whatever connects to it.
- If you separately enable a remote overlay capability for a specific
  overlay profile (Stage 20D2C), the resulting remote Browser Source URL
  is a **capability**: anyone who has that URL can view that specific
  overlay until you rotate or revoke it, the same "possession is the
  capability" model this application's local overlay URLs already use,
  now extended to a second, explicitly opt-in, wider-audience surface.
  It grants no access beyond that one overlay's own public rendering
  data - never your administrator session, never any other overlay,
  never any management capability.

## Network activity you explicitly enable

- When you connect a **Twitch** or **YouTube** account, or configure a
  **StreamElements** donation source, this application communicates
  directly with that provider's own service to authenticate and exchange
  data (chat, events, donations, metadata) - it does not go through any
  Streaming Tree for OBS server, because none exists.
- If you use the managed **MediaMTX** installation option, the backend
  downloads MediaMTX from its documented official source at your request.
  This only happens when you explicitly choose managed installation.
- Outgoing streams (FFmpeg) go directly to the destination platforms you
  configure, using the stream key you provide.

We do not claim "Streaming Tree never connects to the internet" - the
provider integrations above necessarily do, once you enable them. We also
do not claim "no data ever leaves your computer" - the same is true for the
same reason. What we do say precisely: the application itself has **no
first-party cloud service or telemetry pipeline today**, and no network
request happens as a side effect of simply running the application with no
providers configured.

## Telemetry and analytics

There is currently no analytics, crash-reporting, or telemetry dependency
or code path anywhere in this application - confirmed by direct source
audit, not merely undocumented. If this ever changes, it will be a
deliberate, separately documented product decision, not a silent addition.

## Diagnostics and support bundle

Streaming Tree includes an operator-facing diagnostics surface (Stage
20E): a **Logs** page in the management UI, and an on-demand
**support bundle** export, both backed by a bounded, in-memory record
of recent backend log activity - never a second logging system, and
never telemetry (see the section above: nothing here is sent
anywhere automatically).

**What is recorded.** Backend operational events - startup/shutdown,
subsystem state changes (MediaMTX, ingest, destination branches),
HTTP request lines, and recovered errors - each with a timestamp,
severity, and the originating subsystem. This is operational logging
about the application's own behavior, not a record of chat, donation,
or viewer content: chat messages and donation events flow through a
separate, dedicated in-memory pipeline that never passes through this
logging path at all (verified directly - see
`scripts/verify-operator-chat.mjs`).

**Redaction, precisely stated.** Every captured entry is passed
through centralized redaction before it is stored: capability-bearing
URL segments (chat-overlay/alert/audio/widget public slugs,
visual-asset tokens, remote-overlay capability paths) are replaced
with a fixed placeholder, and free-text messages are scanned for
secret-shaped values (long hex/base64 tokens, credential-bearing query
fragments) before capture. Never captured, by construction: stream
keys, OAuth access/refresh tokens, the administrator password or its
verifier, the remote-ingest publisher password, session cookies, CSRF
tokens, remote-overlay or visual-asset capability tokens, the headless
master key, or TLS private key material. We do not claim these logs
contain "nothing personal" without qualification - a connected
account's display name or a destination's platform identifier can
appear in an operational log line describing that account/destination
by name; what we do say precisely is that credentials, tokens, and
message/donation content never can, by the boundaries above.

**Retention.** A fixed 2,000-entry in-memory ring buffer - not a file,
not a database table. History does not survive a process restart; the
oldest entries are silently discarded once the buffer is full, so
memory use never grows without bound no matter how long the
application runs.

**Viewing logs.** The Logs page reads this same bounded buffer through
`GET /api/logs`, filterable by severity, subsystem, and text search.
On a local desktop install this works the same way every other local
management route already does; on a headless remote-management
deployment (Stage 20D2B), it requires the same authenticated session
every other management route requires - never a route reachable
without authentication, and never one that can read an arbitrary file
from the server's filesystem.

**Support bundle.** Generated only when you explicitly click "Export
support bundle" - never automatically, never uploaded anywhere by this
application. It contains: application version/commit, OS/architecture,
packaged/headless mode, which optional features are enabled (remote
management/ingest/overlay - never their configuration or credentials),
the MediaMTX version and whether FFmpeg was found, high-level
subsystem state (never a configured destination URL), updater status,
and the same redacted recent log entries described above. It never
contains: the
SQLite database, your OS credential-store contents, the headless
encrypted secret-store file, the master key, TLS private keys, stream
keys, OAuth tokens, cookies, CSRF tokens, remote-overlay/remote-ingest
tokens or credential-bearing URLs, or chat/donation history. This
exclusion list is enforced by an automated test that seeds synthetic
secret-shaped values into a real bundle and scans every byte of the
result (`internal/support`'s own
`TestSupportBundleNeverLeaksSecretShapedValues`), not merely stated
here.

## Configuration backup and restore

Streaming Tree includes an operator-facing configuration backup and
restore feature (Stage 23, see
[docs/backup-restore.md](docs/backup-restore.md)) - a single local
file capturing your destinations, output settings, connected-account
list, chat overlays/schedules/commands, alert profiles/rules, visual
templates/designs and their managed images/sounds, audio settings,
goals/dashboard widgets, metadata presets, stream setup profiles
(Stage 25), the donation-source list, and update preferences.

**Created only when you explicitly ask for it.** A backup is generated
only when you click "Download backup" in Settings - never automatically,
never uploaded anywhere by this application, and never scheduled. The
file downloads directly to your own machine through your browser, the
same as any other file download; Streaming Tree keeps no copy of it
after you download it, except a single most-recent internal safety
snapshot taken automatically immediately before you commit a restore
(never before an export), so a restore can be recovered from if you
change your mind - also never uploaded anywhere.

**What a backup never contains, by construction.** Stream keys, OAuth
access/refresh tokens and client secrets, donation-source credentials,
the administrator password or its verifier, the remote-ingest
publisher password, and every remote-overlay capability token. This is
enforced two ways, not merely stated here: a closed, reflection-based
structural scan of the backup's own data type
(`internal/domain/backup`'s own `TestConfigStructurallyExcludesSecretShapedFields`)
proving no field could ever carry one, and a separate integration test
that seeds real secrets into a real credential store and scans a real
exported backup's actual bytes for them
(`TestExportedBackupNeverContainsAnyRealSecretValue`). A restored
connected account, destination, or donation source always comes back
needing to be reconnected or re-entered - never silently marked
healthy because its row exists in the file.

**Restoring is always an explicit, destructive action.** Restoring a
backup replaces your entire current configuration with the one from
the file (never merged with what you already have) and requires you to
review a preview and separately confirm before anything is written.
Every restored object is always given a fresh local identity, never
reusing an identifier from the file itself - closing off, by
construction, any possibility that a backup (including a deliberately
crafted one) could cause a restored object to resolve to a credential
already stored on your machine under a coincidentally matching
identifier (`TestRestoreIntoAnIndependentInstallationNeverAdoptsItsPreExistingSecret`,
explicitly the highest-priority test in this feature's own test
suite). Restoring is blocked entirely while a stream is active, the
same guard the updater's own install step already uses.

## Stream session / operational history

Streaming Tree keeps a local record of when it observed a stream
session run - when it started and ended, and which destinations
participated with a coarse, closed-category outcome (Stage 24, see
[docs/stream-session-history.md](docs/stream-session-history.md)).
This is a record of the application's own operational behavior, the
same category of thing the diagnostics log above already keeps, given
its own durable history instead of a bounded in-memory buffer that
disappears on restart.

**What is recorded.** A session's start/end time, and, for each
destination that actually went live during it, its provider and the
display name you gave that destination at the time, when it went live/
stopped, and a coarse outcome (completed, error, or still live when
the session ended). A destination's name is snapshotted at the moment
it goes live - renaming or deleting the destination later never
rewrites or deletes what history already recorded.

**What is never recorded, by construction.** Chat messages, chatter
names, donation messages, donor names or amounts, membership/Super
Chat content, alert payload content, TTS text, or any other viewer-
supplied content - there is no field anywhere in this feature's data
model that could hold any of it, verified by an automated structural
scan of the feature's own types
(`internal/domain/streamsession`'s own
`TestSessionAndDestinationStructurallyExcludeEngagementContent`), not
merely stated here. This is a distinct, independent decision from a
separate, still-open question about whether a future stage might ever
record engagement events themselves - this feature does not resolve,
enable, or foreclose that question.

**How a session's boundaries are determined.** By whether Streaming
Tree's local ingest is actually receiving a publish from OBS - not by
whether any destination is enabled or configured, and never derived
from how long the application itself has been running. A brief
network reconnect (under about a minute) is treated as the same
session, not a new one.

**Retention.** Kept for 90 days by default (configurable in the
History page), since - unlike an engagement-content history - nothing
third-party or personally identifying is ever stored here, only the
application's own operational timeline. An explicit "Clear history"
action in the History page deletes everything immediately, regardless
of age.

## Updater

Streaming Tree includes an application updater (Stage 20B), active only in
a packaged Windows release build - a development build never checks for
updates, regardless of settings. Packaged macOS (Stage 20C1) and Linux
(Stage 20D1) builds do **not** perform automatic or manual update checks
either: neither platform has an install path for the updater to hand off
to yet, so neither ever contacts GitHub for this purpose at all - both
honestly report updates as unavailable rather than silently polling for
something they could not install. This is an intentional platform-
capability limitation, not a privacy carve-out - see
[docs/macos-packaging.md](docs/macos-packaging.md) §20 and
[docs/linux-desktop-packaging.md](docs/linux-desktop-packaging.md) §20.

**What it contacts.** `api.github.com` for release metadata, and GitHub's
own release-asset storage when a download begins - always the canonical
`Czekosabe/streaming-tree-for-obs` repository, never configurable and never
redirectable by any setting, environment variable, or web page.

**What leaves this machine.** An HTTPS request carrying only a descriptive
User-Agent identifying the application and its own version (e.g.
`StreamingTreeForOBS/0.1.0 (+https://github.com/Czekosabe/
streaming-tree-for-obs)`). Nothing else - no stream key, no OAuth token, no
chat content, no destination configuration, no Windows username, no machine
name, and no installation or analytics identifier of any kind.

**When it runs.** By default, a packaged release build checks for updates
shortly after startup and roughly once an hour thereafter - metadata only,
never a download or install on its own. This can be turned off in Settings
→ About & Legal → Updates at any time; a manual "Check for updates" button
remains available either way.

**What is stored locally.** The automatic-check on/off preference, and,
only while an update is actively downloading or has just been installed, a
small temporary file recording the download's own verification state and,
after a real install attempt, a one-shot local result (success or failure)
shown once and then deleted. None of this is shared with any third party
beyond GitHub itself as the download source, and none of it is telemetry -
see the section above.

**Installing an update is always an explicit action** - the operator must
click "Update now" and then confirm "Install and restart"; nothing
downloads or installs automatically. Installing is blocked while a stream
is active.

## Creator support

Selecting **"Support the creator"** in the About & Legal page opens
<https://streamelements.com/czekosabe/tip> in your default browser.

Streaming Tree for OBS itself:

- does not receive your card or payment credentials;
- does not process the transaction;
- does not receive whatever you enter on that external page.

Any information you enter on that external StreamElements (or successor
payment-provider) page is handled entirely under that service's own
privacy policy and terms - not this application's. We make no claims about
StreamElements' fees, taxation, or financial handling; that is between you
and StreamElements.

## Questions

This is a GPL-licensed open-source community project, not a company with a
dedicated privacy contact. The most reliable way to ask a question or
report a concern is to open an issue on the canonical repository:
<https://github.com/Czekosabe/streaming-tree-for-obs>.
