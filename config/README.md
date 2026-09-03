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

These rules are timeless, not tied to when a particular subsystem shipped;
the full stage-by-stage history of exactly which migration/table/setting was
added when lives in `docs/progress.md` and Git, not here.

1. **Secrets never live here, in SQLite, or in any file this application
   writes.** A destination stream key, a connected account's OAuth token
   bundle (Twitch, YouTube), and a donation source's credential (e.g. a
   StreamElements personal JWT) all go exclusively through the OS
   credential store (Windows Credential Manager, macOS Keychain, or Linux
   Secret Service) via the `SecretStore` abstraction in
   `apps/server/internal/secrets`, each under its own namespaced secret
   type - never in a file, and never in this directory. See the "Stream
   key security" section of `docs/project-overview.md` and
   `docs/engagement-architecture.md` for the full model.
2. Configuration files kept in the repository are templates and defaults
   only. A user's local configuration (`*.local.yml`, `.env`) is ignored by
   `.gitignore`.
3. Every file added here must be described in `docs/progress.md`.
4. **No exported package - a visual/chat-overlay template, a portable
   archive, or any other format a user can export and share - may ever
   contain a credential.** Template/package export (see
   `docs/visual-templates.md` and `docs/visual-template-packages.md`) is
   scoped to declarative, non-secret content only; a template or package
   that referenced a credential would leak it the moment it was shared. A
   scheduled/automated chat message or command's own response template is
   likewise plain, user-authored, declarative text (a closed placeholder
   language, never a script or an expression) that can never reference a
   credential either - see `docs/engagement-architecture.md`.
5. A Twitch, YouTube (or future provider) **Client ID is not a secret**,
   and is the one piece of provider configuration that lives outside the
   OS credential store - but still not in this directory. Each resolves
   independently from its own environment variable if set
   (`STREAMING_TREE_TWITCH_CLIENT_ID`, `STREAMING_TREE_YOUTUBE_CLIENT_ID`),
   otherwise from its own non-secret SQLite settings row managed through
   the Settings page. **A Client Secret is never accepted by any part of
   this application, anywhere, for any provider** - every supported OAuth
   flow is public-client-shaped with no secret to store; a pasted
   complete Google `credentials.json` file is rejected the same way a
   pasted `clientSecret` field is, never silently stripped of its secret.
   See [`docs/connecting-platforms.md`](../docs/connecting-platforms.md)
   for the account-connection contract.
6. **Every `STREAMING_TREE_TEST_*` environment variable that redirects a
   provider/updater client at a local fake server exists only in this
   repository's own `-tags integration` test binaries** (built via
   `scripts/build-release.ps1 -IntegrationTest`, or the dedicated
   `apps/server/cmd/testserver`), is read directly via `os.Getenv`, and
   is never routed through the shared config loader. A normal `go build
   ./cmd/server` - the exact command that produces every real release -
   cannot recognize any of them, under any circumstances; there is no
   production environment variable that redirects a provider or updater
   client anywhere but its real, fixed production endpoint. Each
   subsystem's own doc lists its exact test variable(s):
   [youtube-engagement.md §9](../docs/provider-integrations/youtube-engagement.md),
   [windows-packaging.md](../docs/windows-packaging.md),
   [updater.md](../docs/updater.md) §1/§15.
7. **Derived runtime state is never persisted here, in SQLite, or
   anywhere on disk - it lives in memory only and resets on every backend
   restart.** This is a hard, consistent line across every subsystem: a
   destination branch's live status, a provider connector's session/
   reconnect state, the Engagement Event Bus, the operator-chat and
   chat-overlay projections, the outbound-chat dispatcher, the chat-
   automation scheduler, the alert queue/current-alert/playback state,
   the audio-playback queue, and every widget's own event-derived
   content (a display name, a donation message, a ticker row) are all
   runtime-only. What *is* persisted for each of these subsystems is
   only their **configuration** (a schedule definition, an alert rule, an
   overlay profile, a goal's target/baseline) - small SQLite tables
   alongside the rest of this application's configuration, not a file in
   this directory. See each subsystem's own doc
   (`docs/engagement-architecture.md`, `docs/goals-widgets.md`,
   `docs/supporter-widgets.md`) for its exact persisted-vs-runtime split.
8. **A managed binary asset's bytes (a visual/audio asset uploaded or
   imported through a template package) are never stored in SQLite or in
   this directory.** They live as content-addressed plain files under
   `<application data directory>/assets/{visual,audio}/`, addressed by
   SHA-256 digest - only their metadata (a logical asset row, reference-
   tracking join rows) is a SQLite table. See
   `docs/visual-template-packages.md` and `docs/alert-audio.md` for the
   full storage/validation/reconciliation contract.
9. The application's own version/commit/packaged-mode identity is set at
   **build time**, never by an environment variable: three unexported
   `internal/buildinfo` package variables are populated only by
   `scripts/build-release.ps1`'s own `-ldflags "-X ..."` flags, empty in
   every ordinary `go build`/`go run`/`go test`. The production frontend
   build and the four legal documents are staged into
   `apps/server/internal/webassets/{embedded,legal}` by that same release
   script immediately before building - both directories are git-ignored
   except for a tracked `.gitkeep` placeholder each, so a clean
   checkout's `go build`/`go test` never require Node and the generated
   content is never committed. See
   [windows-packaging.md](../docs/windows-packaging.md).
