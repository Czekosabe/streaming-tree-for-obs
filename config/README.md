# `config/`

Directory reserved for the configuration of infrastructure components that will
be added in later stages of the project.

## Current state

**This directory does not contain any working configuration yet.** The current
build does not run MediaMTX or FFmpeg, so there is nothing to configure.

## What does not belong here

Two things that arrived with persistent storage are deliberately **not** kept in
this directory:

- **The SQLite database.** It is user data, not configuration, and lives outside
  the repository — by default in the per-user configuration directory. Its
  location is controlled by `STREAMING_TREE_DATA_DIR` and
  `STREAMING_TREE_DB_PATH` and documented in `README.md`.
- **Schema migrations.** They are `.sql` files embedded into the Go binary from
  `apps/server/internal/storage/sqlite/migrations/`. They ship with the code
  rather than being editable configuration, so that the schema and the code that
  reads it can never drift apart.

This directory remains reserved for configuration a user may legitimately edit.

## Planned contents

| File (planned)         | Purpose                                                                          |
| ---------------------- | -------------------------------------------------------------------------------- |
| `mediamtx.yml`         | MediaMTX configuration: RTMP listener for OBS, paths, local authentication.       |
| `ffmpeg-profiles.json` | Per-platform FFmpeg parameter profiles (bitrate, keyframe interval, output format). |
| `server.example.yml`   | Example backend configuration (port, paths, limits).                              |

## Rules

1. Stream keys, OAuth tokens and any other secrets **must never** be stored
   here. They will live in the operating system credential store (Windows
   Credential Manager / macOS Keychain / Secret Service).
2. Configuration files kept in the repository are templates and defaults only.
   A user's local configuration (`*.local.yml`, `.env`) is ignored by
   `.gitignore`.
3. Every file added here must be described in `docs/progress.md`.
