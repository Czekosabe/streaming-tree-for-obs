<img src="apps/web/src/assets/brand-emblem.png" alt="" width="72" align="left" />

# Streaming Tree for OBS

A local application that takes **one** stream from OBS and branches it out to
several platforms at once — Twitch, YouTube, Kick, TikTok — without a
subscription service, and without your stream keys ever leaving your machine.

<br clear="left" />

## What is it?

OBS sends one RTMP stream to Streaming Tree, running locally on your own
computer. Streaming Tree receives it once and pushes a copy to every
destination you enable — no re-encoding, and one destination failing never
affects the others.

```
        OBS  ──▶  Streaming Tree  ──┬──▶  Twitch
                                     ├──▶  YouTube
                                     ├──▶  Kick
                                     └──▶  TikTok
```

On top of that streaming core, Streaming Tree is growing into a local
engagement and overlay platform: a unified chat across your connected
accounts, OBS Browser Source overlays, alerts, scheduled messages and
commands, visual designers, text-to-speech, and goal/supporter widgets — all
running locally, with no cloud account and no chat data sent anywhere it
doesn't already go.

## Highlights

- **Local multistream routing** — one FFmpeg process per destination, stream
  copy only, independent start/stop/restart and failure isolation.
- **Connected platform accounts** — real Twitch and YouTube sign-in, with
  channel metadata (title, category, tags) read from and published to the
  platform.
- **Stream metadata presets and setup profiles** — save a title/category/tags
  configuration once and reuse it, or prepare an entire show's destination
  set in one step.
- **Unified engagement and chat** — merged live chat across your connected
  accounts, manual and scheduled outbound messages, and safe chat commands.
- **OBS Browser Source overlays** — chat, alerts, audio, and goal/supporter
  widgets, each served over its own unguessable public URL.
- **Alerts with a visual designer** — a real alert queue with a drag-and-drop
  Alert Overlay Designer and a matching Chat Overlay Designer, sharing one
  reusable, importable/exportable visual-template library.
- **Text-to-speech and alert audio** — a shared audio runtime with cooldowns
  and per-rule sound/TTS.
- **Goals, counters, and supporter widgets** — followers, subscriptions,
  donations, Bits, and a set of activity/session widgets built on top of
  them.
- **External donations** — a StreamElements connector feeding the same
  engagement pipeline as platform events.
- **Configuration backup and restore**, and an **in-app updater** for the
  packaged Windows release.
- **English and Polish** application UI, with English canonical and Polish
  kept to full parity.

Only what's actually implemented is listed above — see
[Current limitations](#current-limitations) for what isn't yet, and
[`docs/README.md`](docs/README.md) for the full, current-state contract
behind every feature.

## Quick start

**Development (two processes, two terminals):**

```bash
# Terminal 1 — backend
cd apps/server && go run ./cmd/server

# Terminal 2 — frontend
cd apps/web && npm install && npm run dev
```

Open <http://localhost:5173>.

**Packaged Windows release** — no Node/npm/Go required to run it, just
install and launch:

```powershell
powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+local"
```

See [`docs/development.md`](docs/development.md) for full setup, and
[`docs/windows-packaging.md`](docs/windows-packaging.md) for the packaged
build itself.

## Connecting OBS

In OBS, open **Settings → Stream**, choose **Custom...**, and set:

| OBS field | Value |
| --- | --- |
| **Server** | `rtmp://127.0.0.1:1935` |
| **Stream Key** | `live` |

Both values (and the current ingest state) are shown in the app itself. The
local stream key `live` is a route name on your own machine, not a secret —
real destination stream keys are a separate concern, stored in your
operating system's credential store. See
[`docs/connecting-platforms.md`](docs/connecting-platforms.md) for the full
setup guide (MediaMTX, FFmpeg, and connecting Twitch/YouTube accounts), and
[`docs/project-overview.md`](docs/project-overview.md#10-stream-key-security)
for the stream-key security contract. Running into a problem? See
[`docs/troubleshooting.md`](docs/troubleshooting.md).

## Platform support

| Platform | Status |
| --- | --- |
| **Windows (x64)** | Primary target. Packaged installer (English/Polski), in-app updater. Unsigned (no Authenticode certificate yet). |
| **macOS** | Unsigned, not notarized `.app`/DMG, CI-verified on Apple Silicon and Intel. No automatic updates yet; no system text-to-speech. |
| **Linux (desktop)** | Unsigned `.deb` for Debian/Ubuntu, CI-verified on x64/ARM64. No automatic updates yet; no system text-to-speech. |
| **Linux (headless/self-hosted)** | Opt-in `--headless` service mode with authenticated remote management and remote OBS ingest, behind your own reverse proxy. |

Full detail, including exactly what is CI-verified today, is in
[`docs/platform-support.md`](docs/platform-support.md).

## Documentation

The root of the documentation is [`docs/README.md`](docs/README.md) — start
there for setup guides, feature contracts, packaging/deployment docs,
provider integration research, and the security/architecture reference.

## Current limitations

- **Kick and TikTok** have no account or chat integration yet (feasibility-
  gated for engagement — see [`docs/README.md`](docs/README.md)).
- **Streamlabs and Ko-fi** donation connectors are not implemented (also
  feasibility-gated).
- **macOS and Linux** builds are unsigned, with no automatic updates and no
  system text-to-speech.
- **Windows releases are unsigned** — no Authenticode certificate yet.
- A consolidated **manual/physical verification pass** for the packaged
  builds is still pending; everything above that is described as
  implemented is proven by automated tests, not yet by a human clicking
  through the packaged app.

## Development

See [`docs/development.md`](docs/development.md) for the full developer
setup: requirements, running each half of the app, the REST API reference,
directory structure, linting/testing, and configuration.

## Privacy, Legal, and Licence

Streaming Tree for OBS is an independent project created by **Czekosabe**
(<https://github.com/Czekosabe>), licensed under the **GNU General Public
License v3 or later** (`GPL-3.0-or-later`).

- [`LICENSE`](LICENSE) — the complete licence text.
- [`PRIVACY.md`](PRIVACY.md) — what's local application state versus network
  activity you explicitly enable.
- [`LEGAL.md`](LEGAL.md) — a concise disclaimer and third-party service
  terms.
- [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) — bundled/dependency
  third-party software notices.

The in-app **About & Legal** page (`Settings → About & Legal`) shows the
same information from within the running application, including offline in
a packaged install.
