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
