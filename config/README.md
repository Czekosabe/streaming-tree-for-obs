# `config/`

Directory reserved for the configuration of infrastructure components that will
be added in later stages of the project.

## Current state

**This directory still does not contain any working configuration.** The
backend does run both MediaMTX and FFmpeg now, but neither is configured
from a file in this directory - see below.

## What does not belong here

Several things are deliberately **not** kept in this directory:

- **The SQLite database.** It is user data, not configuration, and lives outside
  the repository — by default in the per-user configuration directory. Its
  location is controlled by `STREAMING_TREE_DATA_DIR` and
  `STREAMING_TREE_DB_PATH` and documented in `README.md`.
- **Schema migrations.** They are `.sql` files embedded into the Go binary from
  `apps/server/internal/storage/sqlite/migrations/`. They ship with the code
  rather than being editable configuration, so that the schema and the code that
  reads it can never drift apart.
- **The MediaMTX configuration.** There is no sample or template here. The real
  `mediamtx.yml` is **generated** by the backend on every start, from the
  validated environment configuration, into
  `<application data directory>/runtime/mediamtx.yml`. It is runtime output, not
  something a user edits — manual changes are overwritten on the next start.

  It is generated rather than templated on purpose: it must stay consistent with
  the pinned MediaMTX version and with the addresses the backend validated, and
  MediaMTX refuses to start on an unknown configuration key. A stale template
  committed here would silently diverge from both.
- **The FFmpeg command line.** There is no config file for it either: each
  destination branch's FFmpeg arguments are built entirely in Go
  (`apps/server/internal/runtime/branch/command.go`) from the destination's
  output settings (`internal/domain/output`, stored in SQLite - server URL
  and an automatic-restart flag, never the stream key) and the retrieved
  stream key, joined immediately before the process starts. This stage is
  stream-copy only (`-c copy`, no transcoding options to configure), so
  there is nothing a per-destination encoding profile would currently
  change - see "Planned contents" below for when that changes.
- **Binaries of any kind.** Neither MediaMTX nor FFmpeg is ever committed to
  the repository. MediaMTX is downloaded, on explicit user request, into the
  application data directory. FFmpeg is never downloaded by this application
  at all - it is located on `PATH` or at an operator-provided path; see the
  README's "Outgoing streaming with FFmpeg" section for why. The
  `.gitignore` rules for third-party binaries are anchored to the repository
  root so they cannot accidentally match a source directory.

This directory remains reserved for configuration a user may legitimately
edit. Per-destination encoding profiles (bitrate, resolution, transcoding)
are the natural first occupant, once that feature exists - stream copy only,
with no transcoding, is this stage's deliberate scope.

## Planned contents

| File (planned)         | Purpose                                                                          |
| ---------------------- | -------------------------------------------------------------------------------- |
| `mediamtx.yml`         | MediaMTX configuration: RTMP listener for OBS, paths, local authentication.       |
| `ffmpeg-profiles.json` | Per-destination transcoding profiles (bitrate, resolution, keyframe interval) - only relevant once transcoding itself is implemented; stream copy needs no profile. |
| `server.example.yml`   | Example backend configuration (port, paths, limits).                              |

## Rules

1. Stream keys, OAuth tokens and any other secrets **must never** be stored
   here. As of the credential-store foundation stage, destination stream
   keys live in the operating system credential store (Windows Credential
   Manager, macOS Keychain, or Linux Secret Service) via the `SecretStore`
   abstraction in `apps/server/internal/secrets` - never in a file, and never
   in this directory. A connected account's OAuth token bundle (stage 7A
   for Twitch, stage 7B for YouTube) lives in that same store under its
   own secret type - see the "Stream key security" section of
   `docs/project-overview.md` and `docs/engagement-architecture.md` for
   the full model.
2. Configuration files kept in the repository are templates and defaults only.
   A user's local configuration (`*.local.yml`, `.env`) is ignored by
   `.gitignore`.
3. Every file added here must be described in `docs/progress.md`.
4. **No secret template belongs here, and no future exported package -
   overlay templates, chat-bot configurations, or anything else a user can
   export and share - may contain a credential.** The planned template
   import/export format (see `docs/engagement-architecture.md`) is scoped to
   declarative, non-secret content; a template that referenced a credential
   would leak it the moment the template was shared.
5. A Twitch, YouTube (or future provider) **Client ID is not a secret** and
   is the one piece of provider configuration that does live outside the OS
   credential store - but still not in this directory. Each resolves
   independently from its own environment variable if set
   (`STREAMING_TREE_TWITCH_CLIENT_ID`, `STREAMING_TREE_YOUTUBE_CLIENT_ID`),
   otherwise from its own non-secret SQLite settings row managed through
   the Settings page - never from a file here, and changing one provider's
   Client ID never affects another's. **A Client Secret is never accepted
   by any part of this application, anywhere, for any provider** - Twitch's
   Device Code Grant Flow and YouTube's Desktop-app Authorization Code
   Flow with PKCE are both public-client flows with no secret to store; a
   pasted complete Google `credentials.json` file is rejected the same way
   a pasted `clientSecret` field is, not silently stripped of its secret.
   The `STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL` / `_API_BASE_URL` and
   `STREAMING_TREE_TEST_YOUTUBE_AUTH_BASE_URL` / `_OAUTH_BASE_URL` /
   `_API_BASE_URL` environment variables that let a test point a provider
   client at a local fake server exist only in the `-tags integration` test
   binary (`apps/server/cmd/testserver`) and are read directly via
   `os.Getenv`, never through the shared config loader - a production build
   cannot recognize them even if they happened to be set. See
   [Connected accounts and Twitch metadata](../README.md#connected-accounts-and-twitch-metadata)
   and
   [Connected accounts and YouTube metadata](../README.md#connected-accounts-and-youtube-metadata).
6. YouTube's OAuth callback listener (a temporary `127.0.0.1` HTTP server
   bound to a dynamically-assigned port, open only for the lifetime of one
   authorization attempt) is **pure runtime state**, exactly like the
   generated MediaMTX configuration above - nothing about it is ever
   written to a file in this directory, or anywhere else on disk.
7. Stage 8A's Twitch EventSub connector follows the exact same rules as
   every provider integration above: `STREAMING_TREE_TEST_TWITCH_EVENTSUB_BASE_URL`
   and `STREAMING_TREE_TEST_TWITCH_EVENTSUB_RECONNECT_HOST` (a local fake
   WebSocket server address, for `scripts/verify-twitch-engagement.mjs`
   only) exist solely in the `-tags integration` test binary, read directly
   via `os.Getenv`, never through the shared config loader or a file here.
   A connector's live WebSocket session, subscription set, reconnect count
   and last error are **runtime state, kept in memory only** - the same
   category as MediaMTX's and a destination branch's own runtime state
   above - never written to a file in this directory. The one thing that
   *is* persisted is a plain enable/disable preference per connected
   account (`connected_account_engagement_settings`, an ordinary SQLite
   table alongside the rest of this application's configuration - not a
   file in this directory, exactly like every other table this project
   has). **Normalized engagement events themselves (chat messages, follows,
   subscriptions, and so on) are never written to SQLite or to any file
   here** - see `docs/engagement-architecture.md` §6.5: the Engagement
   Event Bus is an in-memory-only ring buffer that resets on every backend
   restart, by design.
8. Stage 9's unified operator chat follows the same rule: the operator's
   display preferences, per-account chat visibility, and hidden-user/
   bot-user lists are a small set of ordinary SQLite tables
   (`operator_chat_preferences` and friends) alongside the rest of this
   application's configuration - not a file in this directory. **Chat
   content itself (message text, badges, fragments, the merged timeline)
   is never written to SQLite or to any file anywhere** - the operator-
   chat projection (`internal/operatorchat`) is an in-memory-only ring
   buffer, independently bounded from the Event Bus and reset on every
   backend restart, exactly like it. The Twitch chat-badge image cache
   (`internal/provider/twitch/chatassets`) is likewise in-memory only,
   with a 1-hour TTL - never a file, never a database row. There is
   still no overlay *template* configuration for this directory to hold
   (that remains a later, separate stage) - see rule 9 below for what
   stage 10 itself actually added.
9. Stage 10's chat-overlay profiles follow the same rule again: every
   persisted overlay setting (layout, visibility toggles, filters,
   typography, colors, animation, role highlighting, selected accounts,
   hidden users, blocked terms, activity types, and the public slug)
   is a small set of ordinary SQLite tables
   (`internal/domain/chatoverlay`, migration
   `0011_chat_overlay_profiles.sql`) alongside the rest of this
   application's configuration - not a file in this directory. The
   runtime public projection's own retained-revision capacity
   (`internal/chatoverlay.DefaultRevisionCapacity`, 400) is a fixed Go
   constant, unlike the Event Bus's and operator-chat's own
   environment-configurable buffer sizes above - stage 10 added no new
   environment variable and no new file anywhere in this directory.
   There is still no overlay template/design-file format (that remains
   a later, separate stage, `docs/engagement-architecture.md` §13) - the
   visual settings above are plain per-profile SQLite columns, not an
   exported, shareable package.
10. Stage 11A's manual outbound-chat foundation adds **no new file here
    and no new environment variable**. The in-memory per-account send
    dispatcher (`internal/outboundchat`) is runtime state only - queue
    contents, dispatcher state, recent-send timestamps and the local
    rate-limit window all reset on every backend restart, the same
    category as the branch supervisor's and the Twitch EventSub
    connector's own runtime state above - never written to SQLite or to
    a file in this directory. Its bounds (queue capacity, the 1-second
    local dispatch floor, the 20-per-30-second window) are fixed Go
    constants rather than configurable settings, a deliberate choice
    over adding a new environment variable for values this stage has no
    evidence yet need tuning per deployment. The one thing granted
    (`user:write:chat`) is an additive Twitch OAuth scope, stored in the
    OS credential store exactly like every other scope on the account's
    existing token bundle - not a new secret type, and not a file here.
11. Stage 11B is the first stage in this list to actually add real
    configuration content, not just another "nothing new here" entry:
    persisted schedule and command definitions - names, targets,
    message/response templates, aliases, intervals, delays, jitter,
    thresholds and cooldown bounds (`internal/domain/chatautomation`,
    migration `0012_chat_automation.sql`) - are a small set of ordinary
    SQLite tables (`chat_schedules` and friends) alongside the rest of
    this application's configuration - not a file in this directory,
    and this project's usual rule still holds: **no schedule or command
    definition may contain a credential**, and a message/response
    template is plain, user-authored, declarative text (a closed
    placeholder language, never a script or an expression) - see rule 4
    above and `docs/engagement-architecture.md` §8.3. **Everything this
    stage computes at runtime stays out of SQLite and out of this
    directory**: next-run times, per-schedule/per-account activity
    counters, rolling hourly send counts, and command cooldowns are all
    in-memory only (`internal/chatautomation`), the same category as
    the outbound-chat dispatcher's own runtime state in rule 10 above -
    reset on every backend restart, with no missed-run catch-up. Also
    never persisted anywhere: inbound chat message text, triggering
    usernames, command-use history, or outbound delivery history - see
    `docs/engagement-architecture.md` §17.2. Stage 11B added **no new
    environment variable**.
12. Stage 12A's alert engine follows the same split established above:
    persisted **alert profiles and rules** - profile name, enabled
    state, language, theme/position/text-alignment, queue-capacity/
    expiration bounds, and its public slug; a rule's event type,
    priority, duration, quantity thresholds, visibility toggles, text
    template, animation choice, and provider/account filters
    (`internal/domain/alerts`, migration `0013_alerts.sql`) - are a
    small set of ordinary SQLite tables (`alert_profiles`,
    `alert_rules`, `alert_rule_providers`, `alert_rule_accounts`)
    alongside the rest of this application's configuration - not a file
    in this directory. **Everything the alert engine computes or
    matches at runtime stays out of SQLite and out of this directory**:
    the queue's own contents, the currently playing alert, the single
    replay snapshot, every counter (enqueued/played/expired/capacity-
    dropped/manually-skipped/synthetic), and the public playback
    revision sequence are all in-memory only (`internal/alerts`), the
    same category as the outbound-chat dispatcher's and the automation
    runtime's own state in rules 10-11 above - reset cleanly on every
    backend restart, with no missed-alert replay. **Real matched alert
    events - who followed, subscribed, cheered, or raided, and when -
    are never persisted anywhere**, exactly like inbound chat message
    text and command-use history above; this application keeps no
    supporter history or alert-event log. Local synthetic test-alert
    fixtures (`internal/alerts/testevents.go`) are generated in memory
    at request time and are never written anywhere either. There is
    still no alert *asset or template* file for this directory to hold
    - no uploaded image/GIF/video/sound/font, and no exported template
    package - since Stage 12A's alert presentation is a fixed, closed
    set of profile/rule columns (theme, position, animation choice),
    not a designer output (`docs/engagement-architecture.md` §13).
    Stage 12A added **no new environment variable** beyond what stage
    8A's Twitch EventSub connector already reads in the `-tags
    integration` test binary - the alert engine talks only to the
    already-normalized Engagement Event Bus, never to Twitch directly,
    so it needed no fake-server address of its own.
13. Stage 12B (bounded alert grouping and mid-alert preemption) follows
    the exact same split: four further **persisted** rule columns -
    `allow_grouping`, `group_window_ms`, `interrupt_mode`,
    `interruptible` (migration `0014_alert_grouping_and_
    interruption.sql`) - alongside the rest of `alert_rules`, still not
    a file in this directory. Every new **runtime** counter Stage 12B
    added (`totalGroupedMembers`, `totalGroupsCreated`,
    `totalPreempted`) is in-memory only, the same category as every
    other alert-engine counter in rule 12 above - reset on every
    backend restart, never persisted. No grouped-alert member list, no
    per-event grouping/preemption log, and no new environment variable
    were added either.
14. Stage 13A's shared visual-design engine adds exactly one new
    **persisted** table, `visual_designs` (migration
    `0015_visual_designs.sql`): a versioned JSON document plus an
    integer revision, owner kind/id (only `alert_rule` is an accepted
    owner kind in Stage 13A), and timestamps - one row per alert rule
    that has ever been explicitly saved through the Alert Overlay
    Designer, alongside the rest of `alert_rules` in the same SQLite
    database, still not a file in this directory. A rule with no saved
    design has no row here at all and keeps rendering through Stage
    12A's original fixed presentation. **Everything the Designer's own
    editing session touches stays out of SQLite and out of this
    directory**: the in-memory draft an operator is currently editing,
    its bounded undo/redo history, canvas zoom, and selection state are
    frontend memory only and are discarded on navigating away without
    an explicit Save - opening the Designer never writes a row here by
    itself. At runtime, each alert instance's resolved visual-design
    snapshot (`internal/alerts.Instance.VisualDesign`) is in-memory
    only, the same category as every other alert-engine runtime value
    in rule 12 above, copied once at match time so editing or deleting
    the saved design later never mutates an already-created alert's
    snapshot. There is still no design *asset* file for this directory
    to hold - no uploaded image/GIF/video/sound/font, no design-asset
    directory, and no exported template package or archive - Stage 13A's
    layer kinds (rectangle shape, closed-binding text, platform icon,
    avatar) reference only bounded style values and this application's
    own already-normalized data, never an uploaded or arbitrary-URL
    asset (`docs/visual-designs.md`). Stage 13A added **no new
    environment variable**.
15. Stage 13B (Chat Overlay Designer, Stage 13 as a whole now complete)
    adds no new table and no new file to this directory - it reuses the
    exact same `visual_designs` table item 14 describes, widened by
    migration `0016_visual_design_chat_overlay_owner.sql` (a standard
    SQLite table-rebuild, since a `CHECK` constraint cannot be widened
    in-place) to also accept `owner_kind = 'chat_overlay'` alongside the
    existing `'alert_rule'` - one row per chat overlay that has ever
    been explicitly saved through the Chat Overlay Designer, every
    existing Stage 13A alert-design row surviving the migration with
    its id/JSON/revision/timestamps unchanged. The document schema
    itself moved from version 1 to version 2 in the same migration
    (`internal/domain/visualdesign`'s own in-code, lossless upgrade on
    read - not a second SQL migration) to add the two shared layer
    kinds chat needed; this changes no column, table, or file. A chat
    overlay with no saved design has no row here either and keeps
    rendering through Stage 10's original settings/renderer. The same
    "editing session state never touches SQLite" rule from item 14
    applies identically to the Chat Overlay Designer's own draft/undo/
    redo/selection state. There is still no design *asset* file for
    this directory to hold for chat either - the two new layer kinds
    (`message_fragments`, `badge_list`) reference only already-resolved,
    already-normalized emote/badge image URLs this application already
    trusted before Stage 13B, never an uploaded or arbitrary-URL asset
    (`docs/visual-designs.md` §21). Stage 13B added **no new environment
    variable**.
16. Stage 14A (the reusable visual-template library) adds exactly one
    new **persisted** table, `visual_templates` (migration
    `0017_visual_templates.sql`): a portable metadata set
    (name/description/author/license), a template-format schema
    version, and a versioned JSON document column - one row per
    **user-created** template only. **Built-in templates are never
    persisted at all** - they are reviewed Go constructors
    (`internal/domain/visualtemplate/builtin.go`), validated once at
    application startup, and are never a row in this table, never
    downloaded, and never written to this directory either. Also never
    stored: a raw imported template file's own original bytes (only the
    parsed, validated, normalized document is persisted), a file path,
    export history, or a preview/thumbnail image. There is still no
    template *asset* directory anywhere - Stage 14A adds no new layer
    kind and no asset primitive to the embedded visual-design document,
    and no ZIP/archive format exists yet; see `docs/visual-templates.md`
    for the explicit split from Stage 14B's own portable archive/asset
    format, added later as its own stage (below), which Stage 14A itself
    does not implement. Stage 14A added **no new environment variable**.
17. Stage 14B (managed visual assets and portable archive template
    packages, Stage 14 as a whole now complete) adds four new
    **persisted** tables in one migration (`0018_visual_assets.sql`):
    content-addressed blob metadata, logical asset metadata, and two
    reference-tracking join tables (alert-rule and chat-overlay design
    references to an asset). The blob *bytes* themselves are still never
    stored in SQLite or in this directory - they live as plain files
    under `<application data directory>/assets/visual/`, addressed by
    SHA-256 digest, one file per unique blob regardless of how many
    logical assets or references point at it. A package archive
    (`.streaming-tree-template`) uploaded for preview is staged under a
    random, short-lived token in a sibling
    `<application data directory>/assets/visual/previews/<token>/`
    directory - not the permanent, content-addressed blob store, and not
    this directory - and is deleted on preview cancellation, on preview
    TTL expiry (10 minutes), or unconditionally on the next clean
    application startup; the real import that follows a preview does not
    trust or reuse the preview's staged files, re-validating the
    archive's bytes from scratch. See `docs/visual-template-packages.md`
    for the full manifest, validation, and storage contract. Stage 14B
    added **no new environment variable**.
