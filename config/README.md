# `config/`

Directory reserved for the configuration of infrastructure components that will
be added in later stages of the project.

## Current state

**This directory does not contain any working configuration yet.** The current
build does not run MediaMTX or FFmpeg, so there is nothing to configure.

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
- **Binaries of any kind.** MediaMTX is downloaded on explicit user request into
  the application data directory and is never committed. The `.gitignore` rules
  for third-party binaries are anchored to the repository root so they cannot
  accidentally match a source directory.

This directory remains reserved for configuration a user may legitimately edit.
Once FFmpeg destination branches exist, per-platform encoding profiles are the
natural first occupant.

## Planned contents

| File (planned)         | Purpose                                                                          |
| ---------------------- | -------------------------------------------------------------------------------- |
| `mediamtx.yml`         | MediaMTX configuration: RTMP listener for OBS, paths, local authentication.       |
| `ffmpeg-profiles.json` | Per-platform FFmpeg parameter profiles (bitrate, keyframe interval, output format). |
| `server.example.yml`   | Example backend configuration (port, paths, limits).                              |

## Rules

1. Stream keys, OAuth tokens and any other secrets **must never** be stored
   here. As of the credential-store foundation stage, destination stream
   keys live in the operating system credential store (Windows Credential
   Manager, macOS Keychain, or Linux Secret Service) via the `SecretStore`
   abstraction in `apps/server/internal/secrets` - never in a file, and never
   in this directory. See the "Stream key security" section of
   `docs/project-overview.md` and `docs/engagement-architecture.md` for the
   full model, including how this is designed to extend to OAuth tokens.
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
