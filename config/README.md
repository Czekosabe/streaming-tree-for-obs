# `config/`

Directory reserved for the configuration of infrastructure components that will
be added in later stages of the project.

## Current state

**This directory does not contain any working configuration yet.** The current
build does not run MediaMTX or FFmpeg, so there is nothing to configure.

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
