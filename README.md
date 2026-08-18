# Streaming Tree for OBS

A local application that lets you send **one** stream from OBS and branch it out
to several platforms at once — Twitch, YouTube, Kick, TikTok.

The name describes the model: the stream from OBS is the "trunk", and every
platform is an independent "branch". One branch failing does not stop the
others.

**Long-term vision.** Beyond routing one stream to several platforms, Streaming
Tree is planned to grow into a local streaming engagement and overlay
platform: normalized chat and events from multiple platforms, a unified
operator chat, OBS Browser Source overlays, alerts, scheduled bot messages and
chat commands, visual overlay designers, text-to-speech and goal widgets. A
substantial part of that is real today, not just architecture: a normalized
Engagement Event Bus and a real Twitch inbound connector (stage 8A), a real,
unified operator chat consuming that bus (stage 9), a real, public OBS Browser
Source chat overlay consuming that same operator-chat projection (stage 10),
real manual outbound Twitch chat sending and replying (stage 11A), real
scheduled bot messages and safe chat commands built on that same foundation
(stage 11B), and real **alert rules and a real alert queue** consuming that
same Event Bus (stage 12A) — persisted alert profiles/rules, a
provider-independent matching engine, a bounded in-memory queue with
priority/expiration/pause/resume/skip/replay/clear, local synthetic test
alerts, presented on its own public OBS Browser Source route — plus,
closing out the alert queue itself, real **bounded alert grouping** and
real, opt-in **mid-alert preemption** (stage 12B), a real, shared,
provider-independent **visual-design engine** with a real **Alert Overlay
Designer** editor for that same alert presentation (stage 13A), and a
matching real **Chat Overlay Designer** reusing that same engine for the
chat overlay (stage 13B) — the visual designers are now complete as a
whole — a real **reusable visual-template library** (built-ins, a
persisted user template gallery, and asset-free JSON import/export;
stage 14A) shared by both Designers, and real **portable archive
template packages with bundled assets** — managed images, video, and
custom fonts, safe archive validation, and package preview/import/
export (stage 14B; Stage 14 as a whole is now complete), see
[`docs/visual-template-packages.md`](docs/visual-template-packages.md) —
and a real second **YouTube inbound connector** (stage 15A) publishing
Live Chat, Super Chat, Super Sticker and membership events onto that same
Event Bus, over YouTube's official `streamList` gRPC push transport, with
every downstream consumer above (operator chat, chat overlay, outbound
sending, alerts) serving YouTube events identically to Twitch's — see
[Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents) —
and a real external-donation connector (stage 16A): a provider-independent
`donationsource` domain and a real **StreamElements** Astro WebSocket
connector, publishing real donations onto that same Event Bus with exact
integer-micros money and full reuse of operator chat/chat overlay/alerts —
see [`docs/provider-integrations/external-donations.md`](docs/provider-integrations/external-donations.md) —
and a real **shared audio runtime and text-to-speech foundation**
(stage 17A): a provider-independent `Provider` abstraction with a real
Windows SAPI implementation, a shared bounded audio queue consuming that
same Event Bus (cooldowns, manual approval, per-source/per-currency/
per-bits filtering, text preprocessing), and a real public OBS Browser
Source audio route — see [`docs/audio-tts.md`](docs/audio-tts.md) —
and, on top of that exact same runtime, real **persistent alert sound
assets and per-alert-rule TTS** (stage 17B; Stage 17 as a whole is now
complete): a managed audio-asset library (16-bit PCM WAV only),
rule-owned sound/TTS with deterministic arbitration against the global
TTS queue and a bounded visual hold, and a Stage 14B package manifest
v2 extension carrying that configuration through a portable template
package — see [`docs/alert-audio.md`](docs/alert-audio.md) — and, as
this project's newest addition, a real **persistent goals/counters
foundation and supporter/activity widget suite** (stages 18A/18B; Stage
18 as a whole is now complete): a provider-independent accumulation
engine consuming that same Event Bus at current position, four core
goal families (followers, subscriptions, donations, Bits) with a
deterministic, provider-independent contribution table,
operator-supplied baseline/current management (this application never
claims to know a provider's own complete historical total), durable
per-goal duplicate protection, real public OBS goal widgets, and, on
top of that same foundation, eight further widget kinds (latest
follower/subscriber/donation, largest donation, a recent-supporters
list, an event ticker, richer session counters, and bounded
multi-widget dashboards) whose own event-derived content is
deliberately runtime-only and clears on every backend restart — all
sharing one generic Browser Source route — see
[`docs/goals-widgets.md`](docs/goals-widgets.md) and
[`docs/supporter-widgets.md`](docs/supporter-widgets.md).
**Still planned**: a **Kick**
engagement connector (feasibility-gated — see
[`docs/provider-integrations/kick-engagement.md`](docs/provider-integrations/kick-engagement.md)),
TikTok LIVE support (feasibility-gated — see
[`docs/provider-integrations/tiktok-live.md`](docs/provider-integrations/tiktok-live.md)),
additional external donation-service connectors (Streamlabs,
Ko-fi — both feasibility-gated, stage 16B), and Stage 20's remaining
work (20C2's macOS signing/notarization/updater handoff, 20D's Linux
portability, and 20E's final hardening — Stage 20A's own Windows
production runtime/installer, 20B's application updater, and 20C1's
unsigned macOS packaged runtime are already complete, see
[`docs/windows-packaging.md`](docs/windows-packaging.md),
[`docs/updater.md`](docs/updater.md), and
[`docs/macos-packaging.md`](docs/macos-packaging.md)) — detailed in
[`docs/engagement-architecture.md`](docs/engagement-architecture.md), which
also shapes decisions made today about what is built first. The foundation
was built incrementally: the credential-store foundation (stage 5), the
Twitch and YouTube connected-account integrations (stages 7A/7B), then each
engagement piece above in order (stages 8A through 18B).

> ## Project state: local ingest, outgoing FFmpeg streaming, Twitch + YouTube accounts, real Twitch and YouTube inbound Event Bus connectors, a real unified operator chat, a real OBS Browser Source chat overlay, real manual Twitch and YouTube chat sending, real scheduled messages/chat commands, a real alert engine with Super Chat/Super Sticker money support, real Alert/Chat Overlay Designers, and real portable visual-template packages with managed assets all work
>
> Streaming Tree can **receive** a stream from OBS (a supervised, managed
> MediaMTX process), **store a destination's stream key securely** in the
> operating system credential store, **send it onward** (one independent
> FFmpeg process per enabled destination, plain stream copy, no
> re-encoding), and connect a real **Twitch** account (device-code
> sign-in) or a real **YouTube** channel (Authorization Code + PKCE
> sign-in, via a temporary loopback callback and a real system browser) —
> neither ever requests or stores a client secret — to **read and
> explicitly publish that destination's title, category and other
> platform metadata**. A YouTube destination additionally needs an
> explicitly selected live broadcast before it can publish. A connected
> Twitch account can also, after an explicit additional-permission step,
> **enable a real EventSub WebSocket connector** that normalizes chat
> messages, follows, subscriptions, gifts, cheers, raids, channel-point
> redemptions and remote stream online/offline events onto an in-memory
> **Engagement Event Bus**, viewable live on a diagnostic **Engagement**
> page and now also presented as a real, merged, working **Chat** page —
> badges, emotes, message deletion/clearing, activity events, filters and
> autoscroll all real. That same chat, filtered and re-shaped for a public
> audience, can now be pointed at as a real **OBS Browser Source** — the
> **Overlays** page manages any number of persisted overlay profiles, each
> with its own unguessable public URL, visual settings and filters, served
> over a public HTTP + Server-Sent Events API with no application chrome.
> A connected Twitch account can also, after its own explicit
> additional-permission step, **send real chat messages and replies as
> that account** from the Chat page's composer — a bounded, per-account
> dispatcher and a real Twitch Send Chat Message adapter, with no
> separate bot identity, no optimistic local echo (the sent message
> appears the same way any other message does, once Twitch's own EventSub
> delivers it back), and honest rate-limited/dropped/delivery-unknown
> states rather than a false "sent" claim. That same dispatcher now also
> drives real **scheduled messages** (interval, first delay, randomized
> jitter, message-group alternatives, only-while-streaming and
> minimum-chat-activity gating, a per-schedule hourly cap, and a manual
> Send Now override) and real **safe chat commands** (a fixed `!` prefix,
> aliases, per-role gating, global/per-user cooldowns, and a closed,
> declarative placeholder language) — managed from a new **Automation**
> page, with a hard rule that the account's own sent messages can never
> re-trigger a command. That same Event Bus now also drives real **alerts**:
> the **Alerts** page manages any number of independent alert profiles, each
> with its own public OBS Browser Source URL, and rules that match real
> Twitch follows, subscriptions, resubscriptions, gifted subs, gift-sub
> batches, Bits and raids and channel-point redemptions — with provider/
> account filters, quantity thresholds, priority, a closed placeholder
> template, and a bounded presentation. A bounded in-memory queue plays
> one alert at a time per profile, with expiration, pause/resume, skip,
> replay and clear, plus **local synthetic test alerts** that exercise
> the exact same queue and renderer without ever touching a real Twitch
> account or event. A shared, provider-independent **visual-design
> engine** now also drives a real, bounded **Alert Overlay Designer**
> (drag/resize/property-panel editing, saved per rule) and a matching
> **Chat Overlay Designer** (saved per overlay, one repeated item card
> reusing Stage 10's own filtering/lifecycle unchanged) — both closed,
> bounded editors, never free-form CSS or markup — sharing a real
> **reusable visual-template library** (built-in templates, a persisted
> user template gallery, backend-authoritative compatibility, and
> asset-free JSON import/export, never automatically saving an owner's
> design; stage 14A), extended by real **portable archive template
> packages** (`.streaming-tree-template`, ZIP-only) and real **managed
> visual assets** — uploaded images, video, and custom WOFF2 fonts,
> content/signature-validated, content-addressed and deduplicated,
> served publicly over an unguessable per-asset token, with safe
> archive validation (never blind extraction) and a two-step package
> preview/import flow that never trusts its own preview (stage 14B;
> Stage 14 as a whole is now complete). See
> [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg),
> [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata),
> [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata),
> [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents),
> [Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents),
> [Unified operator chat](#unified-operator-chat),
> [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay),
> [Sending Twitch chat manually](#sending-twitch-chat-manually),
> [Scheduled messages and chat commands](#scheduled-messages-and-chat-commands),
> [Alerts](#alerts)
> and [Stream key security](#stream-key-security).
>
> Starting a real broadcast is always an **explicit action** — a destination
> never starts on its own, and a backend restart never resumes one
> automatically. The same is true of publishing metadata: saving locally and
> publishing to the platform are two separate, both-explicit actions, for
> both Twitch and YouTube. Enabling either the Twitch or the YouTube
> engagement connector, and upgrading a Twitch account's permission to
> send chat, are equally explicit — restoring an enabled connector
> automatically on the next backend start only ever applies to one you
> already enabled yourself. Manual sending is always operator-initiated; a
> schedule or command only ever runs once you have explicitly created and
> enabled it, and no missed run is ever replayed after a restart. Real
> alert-event history is never persisted either — the queue, the current
> alert and every counter are runtime-only and reset cleanly on restart,
> exactly like the automation runtime above.
>
> A connected **YouTube** account can, once a live broadcast with an
> active Live Chat is selected, likewise **enable a real engagement
> connector** — inbound Live Chat over YouTube's official `streamList`
> gRPC push transport (not polling), reusing the exact same connected
> account and OAuth scope, no separate engagement identity. Ordinary chat,
> Super Chat, Super Sticker and channel memberships/milestones all flow
> onto the same Engagement Event Bus the Twitch connector uses, and are
> served by the exact same unified Chat page, operator-chat activities,
> scheduled messages/commands and alerts — Super Chat/Super Sticker carry
> a real monetary amount (integer micros, never a float; an alert's
> currency must match the event's exactly, with no FX conversion ever).
> Outbound plain-text YouTube chat sending works the same way Twitch's
> does, through the same dispatcher; YouTube has no reply concept, so a
> reply is rejected outright rather than silently downgraded. See
> [Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents)
> for the full detail.
>
> Stage 16A adds a real external-donation connector the same way: a
> provider-independent `donationsource` domain (deliberately separate from
> `connected_accounts` — a StreamElements personal JWT has no OAuth shape)
> and a real Astro WebSocket connector publishing donations onto the same
> Event Bus, with exact integer-micros money, moderation-aware
> pending/allowed/rejected handling, and full reuse of operator chat and
> alerts. See [`docs/provider-integrations/external-donations.md`](docs/provider-integrations/external-donations.md).
>
> Kick/TikTok account integration (stage 7C, feasibility-gated alongside
> Kick's own stage 15B and TikTok's own stage 19 engagement research),
> additional donation providers (Streamlabs, Ko-fi — stage 16B,
> feasibility-gated), and everything else still built **on top of** the
> operator chat, outbound chat, alert engine, visual-design/template/
> package engine, shared audio/TTS runtime, and persistent
> goals/supporter-widgets foundation — Stage 20's remaining work
> (20C2's macOS signing/notarization/updater handoff, 20D's Linux
> portability, and 20E's final hardening; Stage 20A's own Windows
> production runtime/installer, 20B's application updater, and 20C1's
> unsigned macOS packaged runtime are already complete) — is still
> **planned**. Whatever remains a placeholder is marked with a
> **Demo** badge — the full list is in
> [What is currently demo-only](#what-is-currently-demo-only).

Detailed project description: [`docs/project-overview.md`](docs/project-overview.md)
Work journal: [`docs/progress.md`](docs/progress.md)

---

## Table of contents

- [Roadmap](#roadmap)
- [Requirements](#requirements)
- [Quick start](#quick-start)
- [Frontend — install and run](#frontend--install-and-run)
- [Go backend — running it](#go-backend--running-it)
- [Data storage](#data-storage)
- [Local ingest with MediaMTX](#local-ingest-with-mediamtx)
- [Connecting OBS](#connecting-obs)
- [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg)
- [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata)
- [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
- [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents)
- [Unified operator chat](#unified-operator-chat)
- [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)
- [Sending Twitch chat manually](#sending-twitch-chat-manually)
- [Scheduled messages and chat commands](#scheduled-messages-and-chat-commands)
- [Alerts](#alerts)
- [Text-to-speech and audio](#text-to-speech-and-audio)
- [Persistent goals and supporter widgets (Stage 18A/18B)](#persistent-goals-and-supporter-widgets-stage-18a18b)
- [REST API](#rest-api)
- [Production build](#production-build)
- [Lint, typecheck, tests and other checks](#lint-typecheck-tests-and-other-checks)
- [Interface languages](#interface-languages)
- [Directory structure](#directory-structure)
- [What is currently demo-only](#what-is-currently-demo-only)
- [Stream key security](#stream-key-security)
- [About, privacy and legal](#about-privacy-and-legal)
- [Common problems](#common-problems)

---

## Roadmap

| Stage | Scope | Status |
| ----- | ----- | ------ |
| 1–4 | Foundations, localization, SQLite configuration, MediaMTX ingest | **Completed** |
| 5 | Secure credential-store foundation | **Completed** |
| 6 | FFmpeg destination branches | **Completed** |
| 7A | Connected-account foundation and a first provider integration: Twitch device-code sign-in, account lifecycle, and explicit metadata publishing | **Completed** — see [progress.md](docs/progress.md) |
| 7B | YouTube account integration: Authorization Code + PKCE sign-in, channel selection, broadcast selection, and explicit metadata publishing | **Completed** — see [progress.md](docs/progress.md) |
| 7C | Kick and TikTok account integration | Deferred — capability-gated, not a prerequisite for Stage 8; Kick's own engagement feasibility was researched in Stage 15B and found feasibility-gated. TikTok's own account integration is now folded into Stage 19's feasibility gate rather than pursued independently, see [tiktok-live.md](docs/provider-integrations/tiktok-live.md) |
| 8A | Engagement Event Bus and a real Twitch inbound connector | **Completed** — see [progress.md](docs/progress.md) |
| 8B | Additional Twitch event coverage, reserved only if 8A cannot safely cover the full verified event set | Planned, conditional |
| 9 | Unified operator chat: a real, merged Twitch chat view across connected accounts | **Completed** — see [progress.md](docs/progress.md) |
| 10 | OBS Browser Source chat overlay: persisted overlay profiles, a public per-overlay projection, a public HTTP/SSE API, a frontend renderer and the Overlays management page | **Completed** — see [progress.md](docs/progress.md) |
| 11A | Manual outbound Twitch chat: additive send-permission profile, a real Send Chat Message adapter, an in-memory per-account dispatcher, manual sending and replies from the Chat page | **Completed** — see [progress.md](docs/progress.md) |
| 11B | Scheduled messages and safe chat commands, built on the same dispatcher: interval/jitter/activity/rate gating, message groups, aliases, roles, cooldowns, a closed placeholder language, and the Automation page | **Completed** — see [progress.md](docs/progress.md) |
| 12A | Alert rules and queue: persisted alert profiles/rules, a provider-independent matching engine, a bounded in-memory alert queue (priority, expiration, pause/resume/skip/replay/clear), local synthetic test alerts, a fixed (non-designer) alert presentation, and a public OBS Browser Source alert route | **Completed** — see [progress.md](docs/progress.md) |
| 12B | Mid-alert preemption and bounded alert grouping, deliberately deferred out of 12A | **Completed** — see [progress.md](docs/progress.md); Stage 12 as a whole is now complete |
| 13A | Shared, provider-independent visual-design document, its persistence/HTTP API, immutable per-alert snapshotting, and the Alert Overlay Designer editor UI | **Completed** — see [progress.md](docs/progress.md) |
| 13B | Chat Overlay Designer, reusing 13A's shared document/renderer for chat overlays | **Completed** — see [progress.md](docs/progress.md); Stage 13 as a whole is now complete |
| 14A | Reusable visual-template library: built-in templates, a persisted user template library, target/owner compatibility, and asset-free JSON import/export, built on Stage 13's own document format | **Completed** — see [progress.md](docs/progress.md) |
| 14B | Portable archive template packages, managed visual assets, and safe custom image/video/font primitives, see [visual-template-packages.md](docs/visual-template-packages.md) | **Completed** — see [progress.md](docs/progress.md); Stage 14 as a whole is now complete |
| 15A | YouTube engagement connector: Live Chat received over the official `streamList` gRPC server-streaming transport, reusing operator chat/chat overlay/alerts/outbound chat unchanged, plus the first real monetary alert capability (Super Chat/Super Sticker) | **Completed** — see [progress.md](docs/progress.md) |
| 15B | Kick engagement connector | Deferred — feasibility-gated: Kick's currently-documented event delivery is webhook-only, requiring a public inbound endpoint this deployment target does not offer, see [kick-engagement.md](docs/provider-integrations/kick-engagement.md); Stage 15 as a whole is **not** complete |
| 16A | External donation foundation and a real StreamElements donations connector: a provider-independent `donationsource` domain, a real Astro WebSocket connector, exact integer-micros money conversion, and full reuse of the existing Event Bus/operator chat/alerts pipeline, see [external-donations.md](docs/provider-integrations/external-donations.md) | **Completed** — see [progress.md](docs/progress.md) |
| 16B | Additional external donation providers (Streamlabs, Ko-fi) | Deferred — feasibility-gated: Streamlabs' documented OAuth token exchange requires a confidential client secret with no public-client alternative found; Ko-fi is webhook-only, requiring a public inbound endpoint this deployment target does not offer; see [external-donations.md](docs/provider-integrations/external-donations.md); Stage 16 as a whole is **not** complete |
| 17A | Shared audio runtime and text-to-speech foundation: a provider-independent `Provider` abstraction, a real Windows SAPI implementation, a shared bounded audio queue consuming the Event Bus, and a public OBS Browser Source audio route, see [audio-tts.md](docs/audio-tts.md) | **Completed** — see [progress.md](docs/progress.md) |
| 17B | Persistent alert sounds, per-rule TTS, visual-template audio assets, see [alert-audio.md](docs/alert-audio.md) | **Completed** — see [progress.md](docs/progress.md); Stage 17 as a whole is now complete |
| 18A | Persistent goals/counters foundation: a provider-independent accumulation engine, four core goal families (followers, subscriptions, donations, Bits), operator baseline/current management, and real public OBS goal widgets, see [goals-widgets.md](docs/goals-widgets.md) | **Completed** — see [progress.md](docs/progress.md) |
| 18B | Latest follower/subscriber/donation, largest donation, a recent-supporters list, an event ticker, richer session counters, and bounded multi-widget dashboards, see [supporter-widgets.md](docs/supporter-widgets.md) | **Completed** — see [progress.md](docs/progress.md); Stage 18 as a whole is now complete |
| 19 | TikTok LIVE connector, **only if** an official, permitted, sufficiently stable integration exists | **Deferred** — feasibility-gated: no official TikTok LIVE engagement event API/scope exists, Embed Player is playback-only, and Desktop Login Kit's token exchange requires a confidential client secret with no public-client alternative found, see [tiktok-live.md](docs/provider-integrations/tiktok-live.md); Stage 19 is **not** implemented |
| 20A | Production runtime and Windows packaging foundation: embedded production frontend, packaged-mode lifecycle (browser launch, single-instance detection, protected graceful shutdown), release-injectable version metadata, and a per-user Inno Setup installer including the four legal documents, see [windows-packaging.md](docs/windows-packaging.md) | **Completed** |
| 20B | Application update system (GitHub Releases check, update UI, real Windows installer/updater handoff), see [updater.md](docs/updater.md) | **Completed** |
| 20C1 | macOS packaged runtime: unsigned `.app`/DMG, real macOS lifecycle adapters (browser launch, single-instance via `flock`, native NSAlert fatal-startup UX), and native macOS CI package verification, see [macos-packaging.md](docs/macos-packaging.md) | **Completed** |
| 20C2 | macOS Developer ID signing, hardened runtime, notarization/stapling, updater install handoff, and public/Beta readiness, see [macos-packaging.md](docs/macos-packaging.md) | Planned — externally gated on real Apple Developer credentials |
| 20D1 | Linux local/desktop runtime and packaging, see [platform-support.md](docs/platform-support.md) | Planned |
| 20D2 | Linux headless/self-hosted server mode and remote security, see [platform-support.md](docs/platform-support.md) | Planned |
| 20E | Logs, diagnostics, and final release hardening/manual verification not covered by 20A-20D | Planned |

The full table with dependencies is in
[`docs/project-overview.md`](docs/project-overview.md#13-roadmap). The
engagement era (stages 8–19) is architected in detail in
[`docs/engagement-architecture.md`](docs/engagement-architecture.md) — read
that document's opening notice before treating any part of it as implemented.

---

## Platform support

Full detail, including exactly what is and isn't automated-CI-verified
today, is in [`docs/platform-support.md`](docs/platform-support.md). Short
version:

- **Windows (x64)** — the packaged desktop app is implemented and this is
  the primary, actively-used target: one Go process, a production React
  build, and your own default browser (see
  [`docs/windows-packaging.md`](docs/windows-packaging.md)). The
  application updater is implemented (Stage 20B, see
  [`docs/updater.md`](docs/updater.md)) - a Stable-only, explicit-action
  update flow against the canonical GitHub repository, with mandatory
  SHA-256 verification and a real Windows installer/restart handoff.
  Release artifacts remain honestly unsigned (no Authenticode certificate
  yet).
- **macOS** — Stage 20C1 (**Completed**, see
  [`docs/macos-packaging.md`](docs/macos-packaging.md)): a real,
  **unsigned and not notarized** `.app` bundle inside a DMG, built and
  verified natively on both Apple Silicon and Intel GitHub-hosted CI
  runners (real `.app`/`Info.plist` structure, real CGO-enabled build
  against the macOS Keychain backend, a real `flock`-based single-
  instance mechanism, a real NSAlert fatal-startup-error bridge, and a
  real DMG mount/copy/run/unmount cycle) - but still no code signing, no
  notarization, no public release, and no real-hardware verification
  (the maintainer does not own a Mac; native CI is real Apple hardware
  but not a substitute for a human's Finder/Gatekeeper/OBS experience).
  The updater honestly reports automatic updates as unavailable on
  macOS rather than a false "up to date". System text-to-speech is
  unavailable today on macOS; the application reports this honestly
  rather than faking it. Stage 20C2 (signing, notarization, updater
  install handoff, public readiness) is planned, externally gated on
  real Apple Developer credentials.
- **Linux (desktop/local)** — planned, not packaged. Native GitHub-hosted
  CI compiles and tests the shared core on Linux x64 and Linux ARM64. No
  `.deb`/`.rpm`/AppImage/Flatpak/Snap exists yet. System text-to-speech is
  unavailable today on Linux, same honest limitation as macOS.
- **Linux (headless/self-hosted server)** — a planned *future* architecture
  target, not a currently-supported deployment mode. The current
  management API and local MediaMTX RTMP listener are deliberately
  loopback-only with no authentication; **do not expose port 8080 (or
  MediaMTX's RTMP/API ports) to a LAN or the public internet** — remote
  access requires a dedicated future security/hardening stage (Stage 20D2)
  that does not exist yet.

---

## Requirements

There are two different audiences here, and they need different tools.

### Developer/build requirements

Building Streaming Tree for OBS from source, or running the two-process
development workflow below, needs:

| Tool | Version | Purpose | Needed now? |
| ---- | ------- | ------- | ----------- |
| **Node.js** | 20.19+ or 22.12+ (22 LTS or newer recommended) | running/building the React panel | yes |
| **npm** | 10+ | installing frontend dependencies | yes |
| **Go** | 1.25 or newer | building and running the backend (`go.mod` pins the floor) | yes |
| Inno Setup 6 | — | building the Windows installer (`scripts/build-release.ps1`) | only for producing a release build, see [windows-packaging.md](docs/windows-packaging.md) |

Checking the installed versions:

```bash
node --version
npm --version
go version
```

> **Note about the Node version.** The project is configured so that it also
> works on Node 22.11. If your Node is older than 22.12, an upgrade is still
> recommended: newer frontend tooling (Vite 7/8, jsdom 30) requires Node
> `^20.19 || >=22.12`, and older versions silently skip their native optional
> dependencies. Details are in [`docs/progress.md`](docs/progress.md).

If you do not have Go yet, download it from <https://go.dev/dl/> and run the
installer for your system. It adds `go` to `PATH`; open a **new** terminal
window afterwards.

### Packaged Windows user requirements

Running the **packaged Windows release** (built via
[windows-packaging.md](docs/windows-packaging.md)) needs **none of the
above** - no Node.js, no npm, and no Go installation. Install it, launch it,
and it opens your default browser to the local management UI on its own.

Both audiences still need the following, regardless of how the application
itself was obtained - these are not build tools, they are what the
application actually does its work with:

| Tool | Version | Purpose | Needed now? |
| ---- | ------- | ------- | ----------- |
| OBS Studio | 30+ | the source of the stream | yes, to actually publish something — the backend runs without it |
| MediaMTX | — | receiving the RTMP stream | yes — installed and supervised automatically, see [Local ingest with MediaMTX](#local-ingest-with-mediamtx) |
| FFmpeg | a recent build (4.4+ floor; actual compatibility is capability-probed, not version-matched) | sending each destination branch | yes, to actually start a destination — see [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg). The application starts and the rest of the interface works without it, packaged or not. |

---

## Quick start

### Development workflow (from source)

The application consists of two processes, started in **two separate
terminals**.

**Terminal 1 — backend:**

```bash
cd apps/server
go run ./cmd/server
```

**Terminal 2 — frontend:**

```bash
cd apps/web
npm install
npm run dev
```

Then open <http://localhost:5173>.

The panel also works **without the backend running** — the system status section
then shows a clear "Backend unavailable" message and the rest of the interface
keeps working.

This two-process workflow remains fully supported for development and stays
that way for the foreseeable future - Stage 20A's packaged build is
additive, not a replacement for it.

### Packaged Windows release

Stage 20A implements a real single-launch Windows packaging: one Go process
serving the production frontend, no separate frontend process, no Node/npm/Go
installation required for the end user. See
[`docs/windows-packaging.md`](docs/windows-packaging.md) for the full
architecture (production routing, packaged-mode lifecycle, the Inno Setup
installer). Building a local release from source:

```powershell
powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+local"
```

produces an unsigned installer under `build/release/output/` (local build
artifacts only - nothing is published, tagged, or released by this script).
Installing and running it opens your default browser to the local management
UI once the application is ready; "Quit Streaming Tree" in **Settings →
About & Legal** stops it cleanly. The same page's **Updates** panel checks
GitHub for a newer Stable release and, with explicit confirmation, installs
it and restarts - see [`docs/updater.md`](docs/updater.md).

---

## Frontend — install and run

### Installing dependencies

```bash
cd apps/web
npm install
```

Run this once, and again after any dependency change. Dependencies land in
`apps/web/node_modules`, which is not version-controlled.

### Running in development mode

```bash
npm run dev
```

The dev server starts at <http://localhost:5173> and reloads the application on
every code change. Requests to `/api` are proxied to the backend at
`http://127.0.0.1:8080`.

Stop it with `Ctrl + C`.

### Configuration (optional)

The defaults are enough for local work. If the backend runs at a different
address, copy `apps/web/.env.example` to `apps/web/.env.local` and adjust the
values.

> **Never put secrets in frontend `.env` files.** Everything prefixed with
> `VITE_` is compiled into the public JavaScript bundle and is visible to anyone
> who opens the page.

---

## Go backend — running it

### Running without building an executable

```bash
cd apps/server
go run ./cmd/server
```

The console prints a line confirming that it is listening:

```
level=INFO msg="http server listening" service=streaming-tree-server version=0.1.0 address=127.0.0.1:8080
```

Stop it with `Ctrl + C`. The server shuts down gracefully, waiting for in-flight
requests to finish (up to 10 seconds).

### Checking the health endpoint

```bash
curl http://127.0.0.1:8080/api/health
```

Example response:

```json
{
  "status": "ok",
  "service": "streaming-tree-server",
  "version": "0.1.0",
  "uptimeSeconds": 12.34,
  "time": "2026-08-03T11:36:38Z"
}
```

On Windows without `curl`, use PowerShell:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/health
```

### Configuration through environment variables

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `STREAMING_TREE_HOST` | `127.0.0.1` | Interface to bind to. Loopback only by default, so the server is not exposed to the local network by accident. |
| `STREAMING_TREE_PORT` | `8080` | REST API port. |
| `STREAMING_TREE_ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Comma-separated list of origins accepted by CORS. |
| `STREAMING_TREE_DATA_DIR` | per-user config directory | Application data directory: database, managed MediaMTX and generated configuration. See [Data storage](#data-storage). |
| `STREAMING_TREE_DB_PATH` | — | Full path to the SQLite file. Takes precedence over `STREAMING_TREE_DATA_DIR` for the database only. |
| `STREAMING_TREE_MEDIAMTX_PATH` | — | Full path to a MediaMTX executable you provide. Skips the managed installation. Must report the supported version. |
| `STREAMING_TREE_FFMPEG_PATH` | — | Full path to an FFmpeg executable you provide. Skips the `PATH` search. Must pass every capability probe. See [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg). |
| `STREAMING_TREE_MEDIAMTX_AUTOSTART` | `true` | Start MediaMTX when the backend starts. |
| `STREAMING_TREE_MEDIAMTX_AUTO_RESTART` | `true` | Restart MediaMTX automatically after an unexpected exit. |
| `STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` | `127.0.0.1:1935` | Address OBS publishes to. **Loopback only.** |
| `STREAMING_TREE_MEDIAMTX_API_ADDRESS` | `127.0.0.1:9997` | MediaMTX Control API address, read only by the backend. **Loopback only.** |
| `STREAMING_TREE_INGEST_PATH` | `live` | The single path publishing is allowed on. Letters, digits, `-` and `_` only. |
| `STREAMING_TREE_TWITCH_CLIENT_ID` | — | Twitch application Client ID. Always wins over a database-managed value if set. Never a client secret — see [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata). |
| `STREAMING_TREE_YOUTUBE_CLIENT_ID` | — | Google OAuth Desktop-app Client ID. Always wins over a database-managed value if set, independently of the Twitch variable above. Never a client secret — see [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata). |
| `STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE` | `1000` | The Engagement Event Bus's in-memory retained-event capacity. Must be between 100 and 10000; an out-of-range or non-numeric value is a startup error. See [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents). |
| `STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE` | `500` | The unified operator-chat projection's in-memory retained-item capacity — independent of the Event Bus's own. Must be between 100 and 5000; an out-of-range or non-numeric value is a startup error. See [Unified operator chat](#unified-operator-chat). |

Booleans accept `true`/`false`, `1`/`0` and `t`/`f`. A typo such as `yes` is a
startup error rather than a silent `false`.

Example — running on a different port:

```bash
# Linux / macOS
STREAMING_TREE_PORT=9000 go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:STREAMING_TREE_PORT="9000"; go run ./cmd/server
```

An invalid value produces a clear error at startup instead of silently falling
back to the default.

### Building an executable

```bash
cd apps/server
go build -o bin/streaming-tree-server ./cmd/server
```

On Windows:

```powershell
go build -o bin/streaming-tree-server.exe ./cmd/server
```

The `bin/` directory is ignored by Git.

---

## Data storage

Platform configuration and stream metadata are stored in a local **SQLite**
database. The driver is `modernc.org/sqlite`, a pure-Go implementation, so the
backend still builds with plain `go build` and needs no C toolchain.

### Where the database lives

The path is resolved in this order:

1. **`STREAMING_TREE_DB_PATH`** — the full path to the file, including its name.
2. **`STREAMING_TREE_DATA_DIR`** — a directory; the file `streaming-tree.db` is
   created inside it.
3. **The default** — the per-user configuration directory reported by Go's
   `os.UserConfigDir()`, plus `StreamingTree/streaming-tree.db`:

| System  | Default location |
| ------- | ---------------- |
| Windows | `%AppData%\StreamingTree\streaming-tree.db` (usually `C:\Users\<you>\AppData\Roaming\StreamingTree\streaming-tree.db`) |
| macOS   | `~/Library/Application Support/StreamingTree/streaming-tree.db` |
| Linux   | `$XDG_CONFIG_HOME/StreamingTree/streaming-tree.db`, or `~/.config/StreamingTree/streaming-tree.db` |

The parent directory is created automatically. The default deliberately lives
**outside the repository**, so a working copy never accumulates a database file,
and `*.db` is ignored by Git in any case.

The resolved path is printed at startup:

```
level=INFO msg="database ready" path=... journal_mode=wal
```

That line contains no credentials. A destination stream key and a connected
account's OAuth token bundle are both stored (in the operating system
credential store, via `SecretStore` - never in SQLite, never in a log line),
but neither the database file nor this startup log ever contains one - see
[Stream key security](#stream-key-security) and
[Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata).

### Migrations

The schema is created and updated by migrations embedded in the binary. They run
automatically at startup — there is no separate migration command. Each
migration commits together with its bookkeeping row, so a failed migration is
never recorded as applied and is retried next time. Applied migrations are
tracked in the `schema_migrations` table and never run twice.

### Seeded configurations

On a **brand-new** database, four destinations are created, one per supported
platform (Twitch, YouTube, Kick, TikTok). They are **disabled** and carry example
metadata. Because the seed is an ordinary recorded migration, it runs exactly
once: **if you delete a seeded destination, restarting the application will not
bring it back.**

No stream key, token or credential is part of the seed itself - the seeded
rows are disabled placeholders with example metadata only. Destination keys
and connected-account OAuth tokens are accepted and stored later, when you
configure them, and always in the OS credential store rather than in SQLite.

### Using a development database

Point the backend at a throwaway file so your real configuration is untouched:

```bash
# Linux / macOS
STREAMING_TREE_DB_PATH=/tmp/streaming-tree-dev.db go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:STREAMING_TREE_DB_PATH="$env:TEMP\streaming-tree-dev.db"; go run ./cmd/server
```

### Resetting a development database

Stop the backend and delete the file. It is recreated, migrated and re-seeded on
the next start:

```bash
# Linux / macOS
rm -f /tmp/streaming-tree-dev.db /tmp/streaming-tree-dev.db-wal /tmp/streaming-tree-dev.db-shm
```

```powershell
# Windows PowerShell
Remove-Item "$env:TEMP\streaming-tree-dev.db*"
```

WAL mode creates `-wal` and `-shm` sidecar files next to the database; remove
them too.

> ### ⚠ Deleting a database deletes your configuration
>
> The database file **is** your saved data: every configured destination, its
> display name and enabled state, and all stream metadata and tags. Deleting it
> removes all of that permanently, and there is no backup or undo. Make sure you
> are deleting a development database and not the default per-user one.

---

## Local ingest with MediaMTX

Streaming Tree receives the stream from OBS through
[MediaMTX](https://github.com/bluenviron/mediamtx), which it runs as a child
process. MediaMTX is third-party software under the MIT licence — see
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

### Pinned version

Only **MediaMTX v1.19.3** is supported. The version is pinned in one place in
the backend, and nothing resolves a "latest" release at runtime.

This matters because the generated configuration and the Control API client
target that exact schema, and MediaMTX refuses to start when it meets an unknown
configuration key. A binary reporting any other version is reported as
**incompatible** and is **not started**.

### Supported operating systems and architectures

| System | Architecture | Managed installation |
| --- | --- | --- |
| Windows | x86-64 (`amd64`) | yes |
| Linux | x86-64 (`amd64`) | yes |
| Linux | ARM64 (`arm64`) | yes |
| macOS | Intel (`amd64`) | yes |
| macOS | Apple Silicon (`arm64`) | yes |

Anything else — 32-bit Linux, ARMv6/ARMv7, FreeBSD, Windows on ARM — has no
managed installation. The interface says so clearly, and you can still point
`STREAMING_TREE_MEDIAMTX_PATH` at a compatible v1.19.3 binary you provide.

### How the binary is found

1. **`STREAMING_TREE_MEDIAMTX_PATH`** — an explicit path you set. Relative paths
   are made absolute, the file must exist and be executable, and its version is
   verified like any other. Reported as source `override`. **Streaming Tree
   never deletes or overwrites this file.**
2. **The managed installation** — what the application downloaded itself.
   Reported as source `managed`.
3. **Missing** — nothing usable was found.

The system `PATH` is deliberately **not** searched. Streaming Tree runs this
binary as a long-lived child process with a generated configuration, so it only
ever runs a copy it can identify.

### Installing MediaMTX

Installation is always an **explicit action** — nothing is downloaded when the
application starts.

In the interface, the sidebar and the **Streams** page show an **Install
MediaMTX** button whenever it is missing. The dialog states the exact version,
that it comes from the official GitHub release, that the checksum is verified,
and that MediaMTX is third-party software with its own licence.

Or through the API:

```bash
curl -X POST http://127.0.0.1:8080/api/runtime/mediamtx/install
```

The installer:

1. selects the official asset for your OS and architecture,
2. downloads `checksums.sha256` from the same release,
3. finds the entry for exactly that asset — no entry means no install,
4. downloads the archive over HTTPS, hashing it as it streams,
5. **discards it on any checksum mismatch**,
6. extracts into a temporary directory, rejecting absolute paths, `..`
   segments, symlinks, hard links and anything escaping the extraction root,
7. requires both the executable and the `LICENSE` file to be present,
8. runs the extracted binary once to confirm it reports v1.19.3,
9. moves the finished installation into place with an atomic rename.

Nothing unverified is ever executed. Temporary files are removed after success
and failure alike, and a failed reinstall leaves an existing working
installation untouched. A second install request while one is running returns
`409 Conflict`.

### Where it is installed

```
<application data directory>/
└── runtime/
    ├── mediamtx.yml                    generated configuration
    └── mediamtx/
        └── v1.19.3/
            └── <os>-<arch>/
                ├── mediamtx(.exe)
                ├── LICENSE             preserved from the official archive
                └── installation.json   version, asset name, SHA-256, timestamp
```

Version and platform are separate path segments, so future versions can sit side
by side. **No MediaMTX binary is ever committed to this repository**, and the
managed installation lives outside your working copy.

### Removing only the managed MediaMTX

Stop the backend and delete the `runtime/mediamtx` directory. Your platform
configuration and metadata are in `streaming-tree.db` and are **not** affected.

```bash
# Linux / macOS
rm -rf ~/.config/StreamingTree/runtime/mediamtx
```

```powershell
# Windows PowerShell
Remove-Item -Recurse "$env:AppData\StreamingTree\runtime\mediamtx"
```

The application then reports MediaMTX as missing and offers to install it again.

### Generated configuration

The backend regenerates `runtime/mediamtx.yml` every time it starts MediaMTX.
It is **generated output, not a file you edit** — manual changes are overwritten.

It enables RTMP and the Control API on their loopback addresses, and explicitly
disables RTSP, HLS, WebRTC, SRT, MoQ, metrics, pprof and playback. Each of those
opens its own listener by default, so none is left to the upstream default.
Exactly one path accepts publishing, with `overridePublisher: false` so a second
publisher cannot silently displace the first. Recording is off, and no
destination or credential appears anywhere in the file.

### Security model

- **Both listeners are loopback-only and this is enforced.** A non-loopback
  address is rejected at startup, not warned about. MediaMTX here accepts an
  unauthenticated publisher, and its Control API can rewrite its own
  configuration, so neither may be reachable from the network.
- **The browser never talks to the MediaMTX Control API.** Only the Go backend
  does. There is no proxy route.
- **The installation endpoint accepts no request body**, so no client can supply
  a download URL or a checksum.
- **No runtime path, process environment or process id is sent to the browser.**

### Process lifecycle and restart policy

The service moves through explicit states: `missing`, `installing`,
`incompatible`, `stopped`, `starting`, `ready`, `stopping`, `error`.

`ready` means the MediaMTX **Control API answered correctly** — not merely that
a process was spawned. A process that starts and immediately exits because a
port is taken never reports ready.

After an **unexpected** exit and with automatic restart enabled, MediaMTX is
restarted with exponential backoff from 1 s to 30 s, at most **5 times in 5
minutes**. Exceeding that stops the retries with an explanatory error instead of
spinning in a crash loop; 60 seconds of stable running resets both the backoff
and the counter.

**An explicit Stop is never undone by the restart policy.**

When the backend shuts down it drains HTTP first, then stops MediaMTX and waits
for it, so no child process is left behind.

> **Shutdown differs by platform.** On Linux and macOS MediaMTX is asked to stop
> with `SIGTERM` and only force-terminated if it has not exited within the grace
> period — a genuinely graceful shutdown. **On Windows it is terminated
> immediately**, because Windows has no `SIGTERM` and MediaMTX is a console
> application with no message loop to close. That is safe here: MediaMTX holds
> no unflushed persistent state, only listeners and in-memory sessions.

---

## Connecting OBS

Start the backend, install MediaMTX if needed, and wait for the service to
report **Running**. Then in OBS open **Settings → Stream** and choose
**Custom...**:

| OBS field | Value |
| --- | --- |
| **Server** | `rtmp://127.0.0.1:1935` |
| **Stream Key** | `live` |

Both values are shown in the sidebar and on the **Streams** page with copy
buttons, and both are derived from your configuration — if you change
`STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` or `STREAMING_TREE_INGEST_PATH`, the
displayed values change with them.

> ### The local stream key is not a secret
>
> `live` is a **route name** on your own machine — it tells MediaMTX which path
> you are publishing to. It is not a password, and it is not a destination
> platform stream key. It is safe to show in a screenshot or a support request.
>
> Real platform stream keys are an entirely separate concept. They are stored
> securely in the operating system credential store — see
> [Stream key security](#stream-key-security) — and are read only when you
> explicitly start that destination's outgoing branch, described next.

Once OBS starts streaming, the ingest status changes from **Waiting for OBS or
another RTMP publisher** to **Receiving an RTMP stream**, and the detected
tracks appear.

RTMP does not identify the publishing application, so Streaming Tree accepts any
RTMP publisher and never claims with certainty that it is OBS.

**Receiving the stream and sending it onward are two separate steps.** OBS
connecting here only makes the local ingest available; nothing goes out to
any platform until you explicitly start that destination — see the next
section.

---

## Outgoing streaming with FFmpeg

Once OBS is publishing to the local ingest, Streaming Tree can send that
stream onward to each configured destination independently, using
[FFmpeg](https://ffmpeg.org/) — one process per destination ("branch"),
pulling the shared local input and pushing to that destination's own
RTMP/RTMPS server.

### Why there is no managed FFmpeg download

MediaMTX has one official, checksummed GitHub release this application can
verify and install automatically (see
[Local ingest with MediaMTX](#local-ingest-with-mediamtx)). **FFmpeg has no
equivalent single official binary distributor** — official builds are source
only; every ready-to-run Windows/macOS/Linux binary comes from a third-party
packager with its own build configuration and licensing implications.
Silently downloading and running one of those on your behalf would mean
trusting a third party this project has not reviewed, so **Streaming Tree
never downloads FFmpeg**. You provide it, from whatever source you already
trust (your OS package manager, or a build you reviewed yourself), and this
application only ever *locates and probes* it.

### How the FFmpeg executable is found

1. **`STREAMING_TREE_FFMPEG_PATH`** — an explicit path you set. Relative
   paths are made absolute; the file must exist, be a regular file, and be
   executable. Reported as source `override`.
2. **A future bundled location** beside the backend executable — documented
   as a convention for a later packaged build; **no binary is bundled or
   committed today**, so this step currently finds nothing.
3. **The system `PATH`.** Unlike MediaMTX, FFmpeg has no single
   application-managed installation to prefer over it — searching `PATH` is
   the correct fallback here, precisely because there is no approved managed
   source to prefer instead. Reported as source `path`.
4. **Missing** — nothing usable was found. The backend keeps running;
   destinations simply report the `ffmpeg_missing` blocker until one is
   available.

**The resolved executable path is never sent to the browser** — only a
semantic source identifier (`override` / `bundled` / `path` / `missing`),
exactly like MediaMTX's own resolution.

### Compatibility policy: capability probing, not exact version matching

Streaming Tree does not pin one exact FFmpeg release the way it pins
MediaMTX. Instead it documents a **minimum supported version as a floor**
(currently 4.4) and probes the actual capabilities every branch needs:
`ffmpeg -version` parses cleanly, RTMP input, RTMP output, RTMPS output, the
FLV muxer, and `-progress` support. A binary that passes every probe is
compatible **regardless of how new it is** — a newer release is never
rejected merely for being newer than this code. A binary that fails any
probe is incompatible even if its reported version looks recent. This
matches how the real, local FFmpeg builds used while developing this stage
report themselves, and avoids treating "this exact string" as a proxy for
"has the features this application actually uses."

### Configuring a destination's output server

Each destination has its own **output settings**, separate from its stream
key: an **RTMP/RTMPS server URL** (`rtmp://` or `rtmps://`, host and
optional port required, no embedded credentials, no fragment) and an
**automatic-restart** toggle. Configure it in the platform's settings
dialog, or through the API (see [REST API](#rest-api)).

> ### The server URL is not the stream key
>
> The server URL is the address of the destination's RTMP ingest — the
> equivalent of OBS's "Server" field. The stream key is the separate secret
> that authorizes publishing to *your* channel on it, stored exactly as
> described in [Stream key security](#stream-key-security). Streaming Tree
> never joins them into one field in the interface, and the stored server
> URL alone is never enough to publish anywhere — it needs the key, which is
> retrieved only at the moment a branch actually starts.

Streaming Tree does not guess a provider's address format for you: a
provider definition may ship a verified default server URL, but if one has
not been confirmed against that platform's current documentation, the field
is left empty rather than filled with a guess.

### Starting and stopping a destination

Each destination's outgoing branch is started and stopped **independently
and explicitly** — there is no automatic start, on ingest arriving or
otherwise, in this stage. A branch becomes eligible to start only once
every one of these holds, in order, and the platform card / Streams page
explain whichever is missing:

1. the platform is enabled,
2. an output server URL is configured,
3. a stream key is stored,
4. the OS credential store is reachable,
5. a compatible FFmpeg was found,
6. the local MediaMTX ingest is ready,
7. OBS (or another publisher) is actually connected.

Starting a destination is a real, deliberate action that begins real
outgoing network transmission — the interface never disguises this as a
quiet background toggle, and starting more than one destination at once
(the **Start enabled destinations** bulk control) shows a confirmation
listing exactly which destinations will start, which are skipped and why,
and that outgoing bandwidth increases per active destination.

**Stream copy only.** No destination is transcoded in this stage: FFmpeg is
run with `-c copy`, so CPU cost stays low and quality is unchanged, but a
source codec FLV/RTMP cannot carry without transcoding makes that one
branch fail fast with a clear, sanitized error rather than silently starting
an expensive re-encode.

### Branch lifecycle and restart policy

Each branch has its own explicit state — `idle`, `blocked`,
`waiting_for_ingest`, `starting`, `live`, `restarting`, `stopping`, `error`
— tracked only in memory, never in SQLite. **`live` means FFmpeg has
reported real, advancing progress**, not merely that a process was spawned.

If OBS disconnects while a branch is running, that branch pauses
(`waiting_for_ingest`) rather than crash-looping against a missing input,
and resumes automatically once the input returns — but **only** for a
branch you explicitly started and have not explicitly stopped since. An
explicit **Stop** always wins: it clears the desire to run and is never
silently undone.

If a branch's own process fails unexpectedly (its destination connection
drops, for instance), it restarts with bounded exponential backoff (1 s up
to 30 s, at most 5 attempts in 5 minutes); exceeding that stops retrying
with a sanitized error instead of looping forever. **One destination
failing never affects another** — each has its own process, its own
backoff, and its own error state.

A backend restart resets every branch to `idle`/not-desired-running and its
restart counter to zero — **it never resumes a broadcast on its own** — while
the output settings themselves (server URL, automatic-restart preference)
persist in SQLite exactly like the rest of your configuration.

### Stream-key exposure on the command line — an honest limitation

The stream key is retrieved from the OS credential store only immediately
before a branch's FFmpeg process is spawned, and is never written to
SQLite, logged, placed in an error message, or returned by any API
response. FFmpeg's CLI was checked for a safer way to pass a per-run RTMP
destination (`ffmpeg -h protocol=rtmp`, `-h protocol=tcp`) and none exists
for this use case — no environment-variable or file-based alternative that
FFmpeg's RTMP output itself supports. **The destination URL, including the
key, is therefore passed as an FFmpeg command-line argument**, which on most
operating systems is visible to other processes owned by the same user (for
example, in a process list). This is accepted for this local, single-user
stage only with these mitigations: it is never logged by this application,
FFmpeg's own captured output is redacted before any logging or storage,
no API response ever contains a destination URL that includes the key, and
this application makes no claim of complete process-level secrecy.

### FFmpeg dependency and branch runtime endpoints

```bash
curl http://127.0.0.1:8080/api/runtime/ffmpeg
curl http://127.0.0.1:8080/api/runtime/branches
```

See [REST API](#rest-api) for the full endpoint list and response shapes.

### Verifying it for real

`scripts/verify-ffmpeg-branches.mjs` exercises this whole feature end to
end against a **real** FFmpeg executable and **real** MediaMTX instances —
a synthetic publisher, the real local ingest, a real branch process, and a
temporary destination MediaMTX standing in for the platform — entirely on
loopback, with no real platform account or credential. See
[Lint, typecheck, tests and other checks](#lint-typecheck-tests-and-other-checks).

---

## Connected accounts and Twitch metadata

Streaming Tree can connect to a real **Twitch** account and use it to read
and explicitly publish that destination's channel metadata (title,
category, language, tags). This was the first of several provider
integrations (stage 7A of the roadmap); YouTube now has its own real
integration too — see
[Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
below (stage 7B). Kick account integration remains deferred/feasibility-
gated (stage 7C, alongside Kick's own stage 15B engagement research).
TikTok account integration is not pursued independently - it is folded
into stage 19's own feasibility gate, which found no official TikTok
LIVE engagement capability for such an account to power, see
[`docs/provider-integrations/tiktok-live.md`](docs/provider-integrations/tiktok-live.md).

**A connected account is not the same thing as a destination's stream
key.** They are separate facts about a destination, tracked and shown
separately: whether the destination is configured, whether a stream key is
stored, whether an output server is configured, whether a Twitch account is
connected and linked to it, whether the local ingest is receiving, whether
its FFmpeg branch is sending, and whether its metadata is in sync with
Twitch. Connecting a Twitch account never starts, stops, or otherwise
touches a destination's FFmpeg branch, and linking an account never
validates or replaces a stream key.

**What this stage (7A) does and does not implement.** This stage is the
account and metadata foundation: sign-in, account lifecycle, linking, and
explicit metadata publishing — not chat or events. Twitch's own chat and
channel events (EventSub) are a **later, separate stage, and that stage is
now real**: stage 8A added a genuine EventSub WebSocket connector reading
chat messages, follows, subscriptions, gifts, cheers, raids, channel-point
redemptions and remote stream status onto an in-memory Engagement Event
Bus, stage 9 added a real, working **unified operator chat page** that
consumes it, stage 10 added a real, public **OBS Browser Source chat
overlay** that in turn consumes that same operator-chat projection, and
stage 11A added **manual outbound chat sending** — a third, independent
capability profile letting the same connected account send and reply to
real Twitch chat messages — stage 11B added real **scheduled
messages and safe chat commands** on top of that same profile and
dispatcher, and stage 12A added a real **alert engine** consuming the
same Event Bus (persisted alert rules, a matcher, a bounded queue, and a
public Browser Source alert route), which stage 12B then closed out with
real bounded alert grouping and mid-alert preemption — see
[Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents),
[Unified operator chat](#unified-operator-chat),
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay),
[Sending Twitch chat manually](#sending-twitch-chat-manually),
[Scheduled messages and chat commands](#scheduled-messages-and-chat-commands)
and [Alerts](#alerts).
Since then, the visual alert/overlay designer (stage 13), donations
from external services (stage 16A), a shared audio/text-to-speech
runtime (stage 17A), persistent alert audio/per-rule TTS (stage 17B),
and a persistent goals/counters foundation with the full supporter/
activity widget suite (stages 18A/18B) have all shipped as well — see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).
What remains planned: live viewer counts and broader stream analytics
(not part of any stage scoped so far).

### Registering a Twitch application and configuring a Client ID

1. Go to the [Twitch Developer Console](https://dev.twitch.tv/console/apps)
   and register a new application. Set its OAuth Redirect URL to
   `https://localhost` (unused by the flow this application performs, but
   Twitch requires one) and its Client Type to **Public**.
2. Copy the generated **Client ID**. Streaming Tree **never asks for,
   accepts, or stores a Client Secret** — the Settings page's Connected
   Accounts panel and every related API endpoint reject one outright (an
   unrecognized `clientSecret` field is a `400`), and the OAuth flow used
   (below) is a public-client flow that has no secret to send, including on
   refresh.
3. Provide the Client ID one of two ways:
   - **Environment variable** `STREAMING_TREE_TWITCH_CLIENT_ID` — always
     wins if set. The Settings page shows its source as "environment" and
     will not let you edit it there.
   - **Settings page**, when no environment variable is set — saved to
     SQLite (not a secret; it is public per Twitch's own client-type
     model), shown with source "database", and editable there.

   Changing a database-managed Client ID while any Twitch account is
   connected is rejected (`409`) — a different application can mean
   different or revoked tokens for existing accounts. Disconnect every
   Twitch account first, or set it to the exact same value (always
   allowed).

### Connecting an account — Device Code Flow

Streaming Tree uses Twitch's **Device Code Grant Flow**, the flow Twitch
documents for a public client with no way to keep a secret (as this
desktop-style local application is). Clicking **Connect Twitch** in
Settings:

1. asks the backend to start an authorization attempt with Twitch;
2. shows a short **user code** and a link to Twitch's activation page;
3. you open that link on any device, sign in, and enter the code;
4. the backend polls Twitch in the background (never faster than Twitch's
   own requested interval) until you finish, the code expires, or you
   cancel;
5. once authorized, the backend validates the token, confirms it was
   issued to the configured Client ID, confirms the required permission was
   granted, and fetches your Twitch login and display name for the account
   list.

The **device code** itself never reaches the browser — only the user code
(safe to display and copy) and the verification link do; there is no field
for it anywhere in the frontend's data model, because the backend's own API
response has no such field to send. Only one Twitch authorization attempt
may be in progress at a time.

The one permission requested is `channel:manage:broadcast` — the minimum
Twitch scope that allows reading and updating channel information. Nothing
broader (chat, subscriptions, Bits, moderation, email) is ever requested at
this stage.

### Account health, validation and reconnecting

A connected account is periodically re-validated against Twitch (at the
hourly interval Twitch's own documentation requires) and can be checked on
demand with **Check now**. If Twitch reports the token invalid, Streaming
Tree attempts one documented refresh (Twitch's refresh tokens rotate on
every use — the previous refresh token stops working the moment a new one
is issued, and Streaming Tree stores the new access and refresh token
together, atomically, so a partial failure never leaves a mismatched pair)
and re-validates the result. If that also fails, the account is marked
**Reconnect required**: publishing and category search stop working for it
until you click **Reconnect**, which repeats the device-flow authorization
for that same account. The same single-refresh-then-retry rule applies
transparently to every ordinary Twitch call this application makes (a
category search or a publish that hits an expired token retries exactly
once with a freshly refreshed token before giving up).

**Disconnect** revokes the account's token with Twitch where possible, then
removes it locally, then removes any destination link that pointed at it.
Twitch reporting the token as already invalid counts as a successful
revocation; a transient network failure leaves the account exactly as it
was so you can safely retry.

### Linking an account to a destination

Open a Twitch destination's settings and choose a connected account in its
own **Connected Twitch account** section — deliberately separate from the
stream-key section above it, since they are different credentials for
different purposes. One account can be linked to more than one destination
(useful if you configure the same channel as more than one destination
entry); a destination has at most one linked account, and linking a
different one replaces the link explicitly.

### Category selection, local Save, and publishing to Twitch

For a Twitch destination, the metadata editor's category field becomes a
search box backed by Twitch's real category/game search (needs a linked,
healthy account). Selecting a result stores both the display name and
Twitch's own stable category ID; typing over it without selecting a new
result leaves a stale ID, which blocks publishing until you search and
select again rather than guessing which category you meant.

**Save and Publish are two separate, both-explicit actions.** Save stores
metadata locally in Streaming Tree's own database, exactly as it always
has. **Publish to Twitch** sends the metadata **currently saved** to your
real Twitch channel — it is disabled, with an explanation, whenever the
form has unsaved edits, so you never publish a draft you have not saved.
Before publishing, a preview shows what would change: the current values on
Twitch, your saved local values, which fields would actually change, and
any reason publishing is currently blocked (no account linked, the account
needs reconnecting, no category selected, Twitch unreachable, Twitch's rate
limit reached). Publishing itself sits behind a confirmation dialog.

Only fields with a verified, real Twitch API equivalent are ever sent:
**title, category, language and tags** — via Twitch's real Modify Channel
Information endpoint. Twitch's channel API has **no** field for stream
description, a generic "mature content" flag, DVR, or a client-side latency
mode; sending real values for those fields to Twitch would either be
silently dropped by Twitch or misrepresent something Twitch does not
actually let this application control, so this application never sends
them and says so plainly in the publish preview instead. See
[`docs/provider-integrations/twitch.md`](docs/provider-integrations/twitch.md)
for the fully researched capability table, including exactly which fields
were previously guessed and have now been corrected.

Publishing **never** starts or stops a destination's FFmpeg branch, never
changes a stream key, and is never triggered automatically by saving
locally — it is always a separate, explicit click.

### Verifying it for real

`scripts/verify-twitch-account-integration.mjs` exercises this whole
feature end to end against the real backend and two small local fake
Twitch servers that reproduce only the response shapes this application
actually parses — device-code authorization, account finalization,
linking, category search, publishing, a forced token expiry and its
single-flight refresh, reconnecting, and disconnect/revocation — entirely
on loopback, with **no real Twitch account, application, or network
request to Twitch involved**. An optional, separate real-Twitch smoke test
is described in the task history but was not run as part of this stage —
see [`docs/progress.md`](docs/progress.md) for exactly what was and was not
verified against a real account.

---

## Connected accounts and YouTube metadata

Streaming Tree can connect to a real **YouTube channel** and use it to read
and explicitly publish a selected live broadcast's video metadata (title,
description, category, tags, language, visibility). This is stage 7B of
the roadmap, reusing the same connected-account foundation stage 7A built
for Twitch, adapted for how Google's own OAuth and the YouTube APIs
actually work — see
[`docs/provider-integrations/youtube.md`](docs/provider-integrations/youtube.md)
for the fully researched contract.

**A connected account, a selected broadcast, and a destination's stream
key are three separate facts**, tracked and shown separately: whether the
destination is configured, whether a stream key is stored, whether an
output server is configured, whether a YouTube channel is connected and
linked to it, whether a live broadcast is selected for it, whether the
local ingest is receiving, whether its FFmpeg branch is sending, and
whether its metadata is in sync with YouTube. Connecting a YouTube channel
or selecting a broadcast never starts, stops, or otherwise touches a
destination's FFmpeg branch, never validates or replaces a stream key, and
Streaming Tree never verifies that a selected broadcast is actually bound
to the stream key configured below it — that binding lives entirely on
YouTube's side.

**What this stage does not implement.** Stage 7B (this section) is the
account, broadcast-selection, and metadata foundation only - it does not
itself implement YouTube live-chat ingestion, Super Chat, Super Sticker,
or membership events. That inbound engagement connector was built later,
on top of this foundation, as stage 15A - see
[Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents).
Twitch was the first provider to feed chat and events onto the
Engagement Event Bus (stage 8A); YouTube is the second (stage 15A) -
both now serve the exact same downstream pipeline: the
provider-independent operator **Chat** page (stage 9) - see
[Unified operator chat](#unified-operator-chat) - the public **OBS
Browser Source chat overlay** built on top of that same chat (stage 10)
- see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay) below
and [`docs/obs-browser-source.md`](docs/obs-browser-source.md) for the
OBS-specific research it is built on - outbound manual chat sending
(stage 11A/15A) and real alerts (stage 12A, with Super Chat/Super Sticker
money support added in 15A). Text-to-speech, donation connectors,
automatic broadcast creation, automatic `liveStream` binding, and
automatic stream-key retrieval from YouTube remain unimplemented - see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).

### Registering a Google Cloud project and configuring a Client ID

1. Create a project in the [Google Cloud console](https://console.cloud.google.com/),
   then enable **YouTube Data API v3** for it (APIs & Services → Library).
2. Under APIs & Services → Credentials, create an OAuth client of type
   **Desktop app**. Google does not require (and this application never
   sends) a client secret for this client type.
3. Copy the generated **Client ID**. Streaming Tree **never asks for,
   accepts, or stores a Client Secret**, and rejects a pasted complete
   `credentials.json` file outright rather than silently extracting the
   secret from it — the Settings page's YouTube panel and every related API
   endpoint accept only a bare Client ID (an unrecognized field is a `400`).
4. Provide the Client ID one of two ways:
   - **Environment variable** `STREAMING_TREE_YOUTUBE_CLIENT_ID` — always
     wins if set. The Settings page shows its source as "environment" and
     will not let you edit it there.
   - **Settings page**, when no environment variable is set — saved to
     SQLite (not a secret), shown with source "database", and editable
     there.

   Changing a database-managed Client ID while any YouTube account is
   connected is rejected (`409`) — the same policy as Twitch's Client ID,
   and independent of it (changing one never affects the other).

**Testing-mode limitation.** A newly created Google Cloud project's OAuth
consent screen defaults to **Testing** publishing status, under which
Google expires every authorization and refresh token it issues after
**seven days**, regardless of what this application requests. This is a
Google-side limitation Streaming Tree cannot detect or work around — only
notice the symptom (a channel unexpectedly needing to be reconnected) and
surface it as **Reconnect required**, the same as any other refresh
failure. The Settings page shows a standing notice about this.

### Connecting a channel — Authorization Code Flow with PKCE

Streaming Tree uses Google's **Authorization Code Flow with PKCE**, the
flow Google documents for a Desktop-app OAuth client — not Twitch's
device-code flow, and not Google's own TV/limited-input device flow either
(that exists for a different class of device; this is a desktop
application with a full browser and keyboard already available). Clicking
**Connect YouTube** in Settings:

1. asks the backend to start an authorization attempt: it generates a
   random attempt ID, a high-entropy PKCE verifier, its S256 challenge, a
   random CSRF state value, and binds a temporary HTTP listener to
   `127.0.0.1` on a port the operating system picks;
2. shows an **Open Google authorization** button — clicking it opens
   Google's real sign-in and consent page in your system browser; nothing
   opens automatically;
3. you sign in and approve access on Google's own page;
4. Google redirects your browser back to the temporary loopback listener,
   which the backend closes right after handling that one request; the
   backend then exchanges the authorization code for a token directly with
   Google — no client secret is sent, ever;
5. if the Google account owns more than one YouTube channel, Streaming
   Tree shows every channel it found and asks you to pick one explicitly —
   it never silently picks the first one;
6. once finalized, the backend validates the token, confirms the required
   permission was granted, and records the channel's title and thumbnail
   for the account list.

The **authorization code**, the **PKCE verifier**, and the **CSRF state
value** never reach the frontend at all — there is no field for any of
them anywhere in the frontend's data model, because the backend's own API
response has no such field to send. Only one YouTube authorization attempt
may be in progress at a time.

The one permission requested is
`https://www.googleapis.com/auth/youtube.force-ssl` — the narrowest scope
that covers reading channel/broadcast/video data and updating video
metadata. Nothing broader (email, Google profile, Drive, Analytics,
monetization, chat) is ever requested at this stage, and the connected-
account identity is the YouTube channel ID — Streaming Tree never stores
or displays the Google account's email address.

### Account health, validation and reconnecting

A connected YouTube account is validated against Google (`GET
https://oauth2.googleapis.com/tokeninfo`) right after authorization, once
per backend startup, and can be checked on demand with **Check now** — no
official Google requirement mandates hourly re-validation the way Twitch's
own documentation does, so this application does not poll Google that
often for YouTube. If validation fails, Streaming Tree attempts one
documented refresh; Google's refresh response typically **omits** a new
refresh token, in which case the previously stored one is preserved rather
than lost (Twitch, by contrast, always rotates its refresh token on every
use — the two providers are handled according to their own actual
behavior, not a shared assumption). If refresh also fails (Google reports
`invalid_grant` — typically a revoked grant, or the Testing-mode seven-day
expiry above), the account is marked **Reconnect required**: publishing,
broadcast listing, and category listing stop working for it until you
click **Reconnect**, which repeats the Authorization Code + PKCE flow for
that same channel identity — authorizing a *different* channel during a
reconnect is rejected rather than silently swapping which channel the
account represents.

**Disconnect** revokes the account's token with Google where possible,
then removes it locally, then removes any destination link **and any
selected-broadcast target** that pointed at it. Google reporting the token
as already invalid counts as a successful revocation; a transient network
failure leaves the account exactly as it was so you can safely retry.

### Linking a channel and selecting a broadcast

Open a YouTube destination's settings and choose a connected channel in
its own **Connected YouTube channel** section, then choose a live
broadcast in the separate **Selected broadcast** section below it — both
deliberately separate from the stream-key section, since all three are
different facts. The broadcast selector lists only your channel's
**active** and **upcoming** broadcasts (never a "persistent" one — Google
deprecated those in 2020) and never auto-selects one; if a previously
selected broadcast can no longer be found, the section says so plainly
rather than silently clearing it. Creating a broadcast happens in YouTube
Studio — Streaming Tree does not create one for you.

### Category selection, region, local Save, and publishing to YouTube

For a YouTube destination, the metadata editor's category field becomes a
dropdown backed by YouTube's real category list for an explicit **region**
(YouTube categories are region-scoped, not a text search the way Twitch's
are). The effective region defaults to the connected channel's own country
when YouTube reports one; otherwise you choose a region explicitly — there
is no silent fallback to the interface language, which is an unrelated
setting. Selecting a category stores both its display name and YouTube's
own stable category ID.

**Save and Publish are two separate, both-explicit actions**, exactly like
Twitch. Save stores metadata locally in Streaming Tree's own database.
**Publish to YouTube** sends the metadata **currently saved** to your
selected broadcast's underlying video — disabled, with an explanation,
whenever the form has unsaved edits or no broadcast is selected. Before
publishing, a preview shows the selected broadcast, the current values on
YouTube, your saved local values, which fields would actually change, and
any reason publishing is blocked (no channel linked, the channel needs
reconnecting, no broadcast selected, live streaming not enabled for the
channel, no category region set, no category selected, YouTube
unreachable, YouTube's quota exceeded) — plus standing warnings such as the
Testing-mode seven-day note and that the selected broadcast and the stored
stream key are not verified as belonging together.

Only fields with a verified, real YouTube Data API equivalent are ever
sent: **title, description, category, tags, language, and visibility** —
via a safe read-modify-write against the video's real `videos.update`
endpoint (Google's own API deletes any mutable property a submitted part
omits, so Streaming Tree always re-fetches the current resource
immediately before writing and only overwrites the fields it actually
manages). YouTube's real API has **no** generic "mature content" flag (its
closest field, made-for-kids, is a COPPA child-directed disclosure, not a
maturity rating), and this stage does not write DVR or latency-mode
settings either (both are broadcast-lifecycle properties a future stage
may add). See
[`docs/provider-integrations/youtube.md`](docs/provider-integrations/youtube.md)
for the fully researched capability table.

Publishing **never** starts or stops a destination's FFmpeg branch, never
changes a stream key, never creates a broadcast, and is never triggered
automatically by saving locally.

### Verifying it for real

`scripts/verify-youtube-account-integration.mjs` exercises this whole
feature end to end against the real backend and two small local fake
Google servers that reproduce only the response shapes this application
actually parses — Authorization Code + PKCE authorization (including a
wrong-CSRF-state callback and explicit multi-channel selection), account
finalization, linking, broadcast selection, category/region, publishing, a
forced token expiry and its single-flight refresh (including Google's
omitted-refresh-token response), restart persistence, reconnecting, and
disconnect/revocation — entirely on loopback, with **no real Google
account, Google Cloud project, or network request to Google/YouTube
involved**. No real-Google smoke test exists or was run for this stage —
see [`docs/progress.md`](docs/progress.md) for exactly what was and was
not verified.

---

## Engagement Event Bus and Twitch chat/events

Streaming Tree can normalize a connected Twitch account's chat messages
and channel events (follows, subscriptions, gifts, cheers, raids,
channel-point redemptions, and remote stream online/offline) onto an
in-memory **Engagement Event Bus**, and stream them live to a new
diagnostic **Engagement** page in the interface. This is stage 8A of the
roadmap — the foundation later stages build the unified operator chat
(stage 9), the OBS Browser Source overlay (stage 10), manual outbound
chat sending (stage 11A), scheduled bot messages and chat commands
(stage 11B), and the alert engine (stage 12A) on top of. See
[`docs/engagement-architecture.md`](docs/engagement-architecture.md) for
the full target design and
[`docs/provider-integrations/twitch-engagement.md`](docs/provider-integrations/twitch-engagement.md)
for the fully researched Twitch EventSub contract.

**What this stage does not implement.** This stage is *inbound* only:
reading chat and events, never sending anything to Twitch.
Kick/TikTok engagement are still unimplemented (YouTube's own
engagement connector is real as of Stage 15A, and TTS is real as of
Stage 17A — see
[Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents)
below) — see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md). A
real, unified operator chat consuming this Event Bus is implemented —
see [Unified operator chat](#unified-operator-chat) below — a real,
public OBS Browser Source overlay consuming that chat's own projection
in turn is also implemented — see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay) —
manually sending/replying as the connected account is implemented too,
as its own independent capability profile — see
[Sending Twitch chat manually](#sending-twitch-chat-manually) — real
scheduled messages and safe chat commands are implemented on top of
that same profile — see
[Scheduled messages and chat commands](#scheduled-messages-and-chat-commands) —
and a real alert engine (matching, queue, fixed presentation) now
consumes this same Event Bus too — see [Alerts](#alerts).
The diagnostic Engagement page added in this stage is explicitly
**not** the operator chat, an overlay, the outbound-chat composer, or
the Automation page — it exists to make the Event Bus and the Twitch
connector's state genuinely observable, and stays a separate page from
Chat, Overlays, and the composer/Automation page built on top of
outbound chat.

### The Engagement Event Bus

The Event Bus (`internal/engagement`) is a concurrency-safe, in-process
component: a bounded ring buffer of recently published normalized events
(default capacity 1000, configurable via
`STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE`), bounded deduplication against
redelivered provider notifications, and live delivery to every connected
subscriber — the same Server-Sent Events endpoint the diagnostic
Engagement page reads from, never a direct connection to Twitch. Neither
the operator Chat page nor the OBS chat overlay reads this endpoint
directly; both read through the operator-chat projection instead (see
[Unified operator chat](#unified-operator-chat) and
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)).
**It is in-memory only.** No normalized event, and no chat message, is
ever written to SQLite; the entire buffer resets to empty on every
backend restart, exactly like MediaMTX's own runtime state.

A slow subscriber can never block event publication or another
subscriber: if a subscriber's own buffered channel is full when a new
event arrives, that subscriber is dropped with an explicit signal rather
than allowed to make the whole bus wait on it.

### Enabling Twitch chat and events — an explicit permission upgrade

A Twitch account connected under
[Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata)
above already exists with only the metadata scope
(`channel:manage:broadcast`). Reading chat and events needs five
additional, narrowly-scoped permissions: `user:read:chat`,
`moderator:read:followers`, `channel:read:subscriptions`, `bits:read`
and `channel:read:redemptions` — never `user:write:chat` (sending chat
messages), which belongs to a separate, independently assessed
outbound-chat capability profile instead — see
[Sending Twitch chat manually](#sending-twitch-chat-manually). Reading
and sending are deliberately never bundled into one upgrade request.

Clicking **Authorize engagement access** on the Engagement page starts a
new Device Code Flow attempt, reusing the exact same flow the initial
Twitch connection used, requesting the **union** of the account's
current scopes and the five above — it can only add permission, never
remove any the account already has. The newly authorized identity must
match the existing connected account exactly; a different Twitch login
is rejected rather than silently creating a second, competing
connection. The previous, working token stays in place until the
upgrade completes successfully, then is atomically replaced.

**Metadata health and engagement-permission health are tracked
independently.** An account missing the engagement scopes remains fully
healthy for metadata publishing — it is never marked "Reconnect
required" merely because an optional capability was never authorized.

### The Twitch EventSub connector

Once the engagement scopes are granted, toggling **Enable Twitch chat
and events** on the Engagement page opens one supervised WebSocket
connection to Twitch's EventSub endpoint for that account and creates
the selected subscriptions (chat messages, message deletion, chat
clear, user-message clear, follows, subscriptions, subscription gifts
and gift batches, resubscription messages, cheers, incoming raids,
channel-point redemptions, and remote stream online/offline — thirteen
subscription types in total). Enabling or disabling is always an
explicit action, exactly like starting or stopping a destination branch;
an enabled connector reconnects automatically after a backend restart,
a disabled one does not.

The connector's own state is shown plainly: connecting, waiting for
Twitch's welcome message, subscribing, connected, reconnecting,
stopping, blocked (missing permission or configuration), or error
(for example, after Twitch revokes authorization) — never collapsed
into a single "on/off" indicator. **This is a distinct fact from
whether OBS is streaming, whether the local ingest is receiving, or
whether a destination's FFmpeg branch is sending** — a connected,
subscribed EventSub connector says nothing about whether this
application's own outgoing stream to Twitch is live, and vice versa.

Twitch does not replay events lost during an ordinary connection loss.
When that happens, the connector reconnects with bounded backoff,
recreates its subscriptions, and honestly marks a **possible data gap**
rather than claiming seamless recovery. Twitch's own official
`session_reconnect` handoff (a graceful migration to a new connection,
distinct from an ordinary loss) is handled without recreating
subscriptions and without a data-gap marker, exactly as Twitch's own
documentation describes.

### Normalized events and the diagnostic Engagement page

Every event — from any provider, in future stages — is normalized to
the same versioned shape before reaching the bus: a monotonically
increasing sequence number, a stable internal ID, the provider and
connected account it came from, a normalized type (`chat.message`,
`chat.message_deleted`, `chat.cleared`, `moderation`, `follow`,
`subscription`, `resubscription`, `gifted_subscription`,
`subscription_gift_batch`, `bits`, `raid`, `channel_point_redemption`,
`stream.online`, `stream.offline`), ordered chat message fragments
(text, emote, cheermote, mention), a user identity block (never
inventing an avatar or color the provider did not itself report, never
fabricating an identity for an anonymous gift or cheer), and — where
applicable — an amount, currency, or quantity. A gift-batch event
("gifted 5 subs") and each individual gifted-subscription recipient
event are kept as genuinely separate events, never collapsed into one.

The Engagement page shows the bus's own status (retained event count,
buffer capacity, oldest/newest sequence), a card per connected Twitch
account's connector, and a bounded, plain-text recent-events feed fed
live over Server-Sent Events — no message bubbles, no theming, no
animation, explicitly not styled as the finished chat overlay a later
stage will build.

### Verifying it for real

`scripts/verify-twitch-engagement.mjs` exercises this whole feature
end to end against the real backend, fake Twitch OAuth and Helix
servers, and a small hand-rolled fake Twitch EventSub WebSocket server
(Node has no built-in WebSocket server, and this project added no new
npm dependency to get one) — the permission-upgrade scope union,
subscription creation, event normalization and deduplication across
many event types, Twitch's official `session_reconnect` handoff (no
resubscription, no data gap), an ordinary disconnect (a data gap
recorded, subscriptions recreated), authorization revocation, restart,
disable, and disconnect — entirely on loopback, with **no real Twitch
account or network request to Twitch involved**. A representative
subset of scenarios is covered by Go unit tests instead of this script
(malformed/oversized-frame handling, keepalive-timeout-triggered
reconnects, and others needing precise timing control a fake clock
provides more reliably than a real WebSocket exchange) — see
[`docs/progress.md`](docs/progress.md) for exactly which.

---

## Engagement Event Bus and YouTube chat/events

Stage 15A extends the same Engagement Event Bus a connected YouTube
account's Live Chat: text messages, Super Chat, Super Sticker, channel
memberships and membership milestones — onto the exact same bus the
Twitch connector above publishes to. Every downstream consumer (the
unified operator chat, the OBS chat overlay, manual/scheduled/command
outbound sending, and alerts) serves YouTube events identically to
Twitch's, through the exact same shared code — never a second,
YouTube-only copy of any of them. See
[`docs/provider-integrations/youtube-engagement.md`](docs/provider-integrations/youtube-engagement.md)
for the fully researched YouTube Live Chat contract this connector
implements.

**No separate permission upgrade.** Unlike Twitch, a connected YouTube
account (see
[Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
above) already carries the one scope
(`https://www.googleapis.com/auth/youtube.force-ssl`) that covers
reading chat, Super Chat/Super Sticker, membership events, and sending
chat, all at once — enabling engagement on the Engagement page never
prompts for additional authorization.

**A real gRPC push stream, not polling.** YouTube's Live Chat API's
official `liveChatMessages.streamList` method is a gRPC server-streaming
RPC — a long-lived push connection, not a client polling on an interval
(see the linked research document's own §4b for the full verified
contract: the vendored `.proto`, the generated Go client, the production
host, and the OAuth metadata handshake). A connector must first have a
**broadcast selected** (see
[Linking a channel and selecting a broadcast](#linking-a-channel-and-selecting-a-broadcast)
above) with an active live chat; without one it reports
`waiting_for_broadcast` or `waiting_for_live_chat` — an honest "not
ready yet" state, never an error. Outbound sending
(`liveChatMessages.insert`) and all broadcast/channel/video metadata
calls stay REST — only receiving moved to gRPC.

**The first response never counts as live.** A genuinely fresh stream's
first response can carry recent chat history — this connector treats
that first response as a silent baseline (its continuation token is
kept, nothing in it is ever published) and only publishes messages from
responses *after* that baseline. A transient reconnect that still holds
a valid continuation token is different: its first resumed response is
treated as live immediately, never re-baselined, so an ordinary network
blip never silently drops real chat. This baseline behavior applies
again after every *fresh* connect: an explicit connector restart, a
broadcast change, or a full backend restart, all re-baseline rather than
replaying history as if it had just happened.

**A first real monetary value.** Super Chat and Super Sticker carry a
genuine paid amount — this application's first real money value
anywhere (`internal/domain/engagement.Money`): an integer
micros amount (never a float), an uppercased currency code, and the
provider's own formatted display string. There is no currency
conversion anywhere in this codebase and there will not be one — an
alert's amount threshold only ever matches the exact same currency,
never a numerically-larger amount in a different one.

**Replying is not offered.** YouTube's send API has no reply/parent-
message concept at all — the Chat page's Reply action never appears
on a YouTube message, and a send request that still tries to carry a
reply reference for a YouTube account is rejected outright by the
backend, never silently sent as a plain message instead.

### Verifying it for real

`scripts/verify-youtube-engagement.mjs` exercises this whole feature
end to end against the real backend and a fake Google OAuth/YouTube
API server (extending the same fake-server pattern
`scripts/verify-youtube-account-integration.mjs` already established)
— baseline-first cutover (seeded chat history never leaks onto the
bus), a real chat message/Super Chat/Super Sticker each normalizing
correctly and reaching both the Event Bus and the operator-chat
projection, a real Super Chat triggering a real monetary alert with
correct currency matching (no FX conversion), outbound sending with a
rejected reply attempt, chat-automation self-loop protection (keyed on
the stable channel id), and both an explicit connector restart and a
full backend restart never replaying history — entirely on loopback,
with **no real Google/YouTube account or network request to Google
involved**.

---

## Unified operator chat

Stage 9 adds a real, working **Chat** page (`/chat`): merged, live
Twitch chat across every connected account whose engagement connector
is enabled, distinct from the Engagement page's connector diagnostics.
Chat = the daily working view; Engagement = "is the connector actually
healthy." Neither replaces the other.

**What this stage (9) did not implement, and what has changed since.**
At stage 9, sending chat, chat commands, scheduled bot messages,
alerts, TTS, remote moderation actions (bans/timeouts/message deletion
sent *to* Twitch), and YouTube/Kick/TikTok chat all remained exactly as
planned. Alerts shipped in stage 12A and TTS in stage 17A; remote
moderation actions and YouTube/Kick/TikTok chat sending are still
unimplemented — but stage 11A added real **manual**
sending and replying, from a composer built into this same Chat page,
and stage 11B added real **scheduled messages and chat commands** on
top of that same foundation, managed from a separate Automation page —
see [Sending Twitch chat manually](#sending-twitch-chat-manually) and
[Scheduled messages and chat commands](#scheduled-messages-and-chat-commands).
A message appearing in operator chat is never proof this application's
own outgoing FFmpeg branch works; that is an unrelated, separately
verified fact. The public OBS Browser Source overlay built on top of
this same projection **is** implemented (stage 10) — see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay).

### The operator-chat projection

`internal/operatorchat` subscribes to the same Engagement Event Bus and
converts normalized events into a chat-shaped, lifecycle-aware public
item model — never the other way around; the projection never mutates
the Event Bus's own retained history, and it never imports the Twitch
provider package. It is **in-memory only**, independently bounded from
the Event Bus (default capacity 500, configurable via
`STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE`, 100–5000) and begins empty
on every backend start — nothing about this stage claims pre-restart
chat history.

Every revision — a brand-new message/activity/moderation/system item,
or a lifecycle update to an existing one (a message becoming deleted) —
is a complete "upsert" carrying its own monotonically increasing
sequence, replayed identically by the bounded snapshot endpoint and the
live SSE stream. A message becoming deleted updates the **same** item
id in place, with its original content preserved and a visible deleted
marker — it is never silently removed and never produces a second row.
A deletion referencing a message no longer retained produces a small,
honest moderation item instead of inventing content. A whole-chat clear
or a per-user clear ("clear this user's messages," which needs no
reference to any specific prior message) is scoped exactly to the
provider/account/user it targets — never a different account, never a
different user.

### Merged accounts, badges and emotes

A user may have more than one connected Twitch account; the merged
timeline never guesses which one "the" source is — every item carries
its own connected-account id, and the account label is shown only when
there is more than one account contributing (never a name nobody needs
to disambiguate). Twitch chat badges are resolved from the
channel-specific catalog first, then the global one, through a
bounded, TTL'd (1 hour), single-flight cache — see
[`docs/provider-integrations/twitch-engagement.md`](docs/provider-integrations/twitch-engagement.md)'s
Stage 9 addendum for the full research this is built on, including
where that channel-then-global order is this project's own defensible
inference rather than an explicitly documented Twitch rule. Emote
images are built as a pure URL from the fragment's own emote id — no
catalog fetch, no cache, nothing that can go stale. A badge or emote
that cannot be resolved is simply omitted or falls back to its plain
text; the chat message itself is never discarded or blocked on it.

### Filters, settings and privacy

The Chat page filters by connected account (persisted per account) and
by explicitly-hidden or bot-marked users (a small "hide this
user"/"mark as bot" action per message, backed by its own persisted
list — identified by the provider's own stable user id, never a
display name someone can change, and never a heuristic guessing "bot"
from a username). Display preferences (platform icon/name, account
label, badges, timestamps, activity events, deleted messages, command
messages, compact mode) persist in SQLite and apply immediately while
being edited, saved only on an explicit action. **None of this is chat
content**: no message text, no username treated as authoritative
identity, no token, and no raw provider event is ever persisted — see
the migration's own scope note in
`apps/server/internal/storage/sqlite/migrations/0010_operator_chat_preferences.sql`.
The timeline auto-scrolls while at the bottom, pauses the moment an
operator scrolls up, and offers an explicit "Jump to latest" control
with an unseen count rather than silently stealing the viewport.

### Verifying it for real

`scripts/verify-operator-chat.mjs` exercises the whole stack end to end
against the real backend and the same kind of fake Twitch OAuth/Helix/
EventSub servers `verify-twitch-engagement.mjs` uses (extended with
fake `GET /chat/badges/global` and `GET /chat/badges` routes) — badge
channel-then-global resolution with cache-hit counts, the emote CDN
URL, an exact deletion updating the same item id, a per-user clear and
a whole-chat clear each correctly scoped, every activity type
(including the gift batch staying distinct from its recipient, and
bits never labeled a donation), preferences/hidden-user/bot-user
persistence surviving a real backend restart while chat content itself
resets to empty, and the SSE stream — entirely on loopback, with **no
real Twitch account or network request to Twitch involved**. A
representative subset of scenarios (a second connected account merging
into the timeline, a deliberately forced projection-side gap) is
covered by Go unit tests instead — see
[`docs/progress.md`](docs/progress.md) for exactly which.

---

## OBS Browser Source chat overlay

Stage 10 adds a real, public **OBS Browser Source chat overlay**: any
number of persisted overlay profiles, each rendering a filtered,
presentation-shaped view of the same merged Twitch chat the operator
**Chat** page shows, served over its own unauthenticated public HTTP +
Server-Sent Events API for OBS's Browser Source (or a plain browser tab)
to consume directly — no application chrome, no sidebar, no operator
login. Manage overlays on the **Overlays** page (`/overlays`); each
overlay's own public URL points at `/overlay/chat/{publicSlug}`. See
[`docs/obs-browser-source.md`](docs/obs-browser-source.md) for the
underlying OBS Browser Source research (setup, recommended dimensions,
the shutdown/refresh checkbox trade-off) this feature is built on.

**What this stage does not implement.** Kick/TikTok overlay
support is still unimplemented (a public audio overlay route shipped
in stage 17A — see [Text-to-speech and audio](#text-to-speech-and-audio)).
YouTube chat *does* reach the overlay,
same as Twitch's — this stage's own filtering/lifecycle/moderation
stayed entirely authoritative when Stage 15A added the YouTube connector
underneath it, exactly like the operator Chat page above (see
[Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents)).
Asset-free JSON
template import/export for a chat visual design exists (Stage 14A, via
the Chat Overlay Designer's own Templates gallery — see below); a
portable *archive* template package with managed image/video/font
assets now exists too (Stage 14B — see
[`docs/visual-template-packages.md`](docs/visual-template-packages.md)).
See [`docs/engagement-architecture.md`](docs/engagement-architecture.md).

### Chat Overlay Designer (Stage 13B)

Stage 13B adds a real, bounded **visual designer** for the chat overlay
on top of the same shared, provider-independent visual-design document
Stage 13A introduced for alerts (`internal/domain/visualdesign`) —
never a second document format or a second React renderer. Open it from
the Overlays page via **Open Designer**
(`/overlays/{overlayId}/designer`); it reuses the Alert Overlay
Designer's own generic editing chrome (drag/resize/snap, numeric
geometry, layer ordering/lock/duplicate/delete, undo/redo, zoom/fit,
typography/color/animation panels) unchanged, differing only in which
layer kinds and text bindings are offered.

A chat visual design represents **one repeated item card** — a single
message or activity event — never the whole overlay: Stage 10's own
filtering, lifecycle, moderation, capacity and stack-ordering logic
(`internal/chatoverlay`) stays entirely authoritative in both legacy and
design-driven rendering. The same saved design is instantiated once per
currently-visible item with that item's own safe data. Two new shared
layer kinds beyond Stage 13A's four (rectangle shape, text, platform
icon, avatar) support rich chat content safely: `message_fragments`
(pre-normalized text/emote/mention fragments, resolved emote images,
never raw HTML) and `badge_list` (a bounded list of already-resolved
badge images) — bumping the document schema to version 2
(`docs/visual-designs.md` documents the lossless v1→v2 migration; every
Stage 13A alert design keeps loading and rendering identically).

One overlay may have at most one saved design (no design → the
original Stage 10 fixed renderer, unchanged; a saved design → that
design drives every message/activity item). Opening the Designer
generates a deterministic, legacy-approximating draft entirely in the
frontend — it persists nothing and changes nothing about the live
overlay until an explicit Save. **Reset to legacy** deletes the saved
design and returns the overlay to the original fixed renderer
immediately; deleting the overlay itself cascades its saved design.
Unlike an alert (which snapshots its design once per queued instance,
so a replay always looks the way it did when it fired), a chat design
is **current-profile presentation** — saving a new one updates every
currently-visible item and every future item, without duplicating,
resurrecting, or reordering anything; the public overlay learns about
this over the same SSE stream via an additive `chat-overlay.presentation`
event that never changes the meaning of any existing Stage 10 event or
endpoint.

### Visual Template Library (Stage 14A)

Both the Alert Overlay Designer and the Chat Overlay Designer share a
**Templates** button opening the same reusable template gallery
(`components/visual-templates/TemplateGallery.tsx`) — never two
separate gallery implementations. A template wraps a complete visual
design document with portable metadata (name, description, author,
license) and belongs to exactly one target, `alert` or `chat`.
**Built-in templates** (three per target — Minimal Dark, Clean
Modern/Compact, Neon Accent) are application-owned, immutable, reviewed
Go constructors, never downloaded and never a database row; **My
templates** are operator-owned, persisted in a new `visual_templates`
SQLite table (migration `0017`), and may be renamed or deleted.

Using a template **never saves the owner's design automatically** —
choosing "Use as draft" loads it into the Designer's own existing
unsaved-draft state (one undo step), exactly preserving Stage 13's own
explicit-Save rule; only the Designer's pre-existing Save button ever
writes a `visual_designs` row. **Save as template** does the reverse:
it persists the *current draft* as a new reusable template without
touching the owner's own saved design, so the two operations stay
fully independent. Deleting a template can never affect any design
already created from it — there is no foreign key or live reference
from a saved design back to a template it may once have come from.

**Compatibility** is assessed by the backend, never the frontend: a
template scoped to the current owner (a specific alert rule's own
event type, or a specific chat overlay) reports whether it can
genuinely be used, with a stable reason code (e.g. "designed for a
different target," "uses a binding unavailable for this alert rule") —
an incompatible template's own "Use as draft" stays disabled rather
than silently producing a design that can never render anything for
that owner.

**JSON import/export, asset-free.** A template exports as a single,
closed, portable JSON file (`.streaming-tree-template.json`) containing
only the fields above plus the embedded document — no image, video,
audio, font, or archive of any kind. Import is a two-step flow: select
a file, see a backend-validated preview (name/description/target/a
real rendered preview/compatibility), then explicitly confirm — nothing
is persisted merely by selecting a file. An older exported document
(schema version 1, from before Stage 13B) is transparently migrated to
the current version on import; an unknown/future/malformed version is
rejected outright, never silently reinterpreted. This asset-free JSON
format remains unchanged and fully supported after Stage 14B - a
template whose document references a managed asset is exported as a
portable archive package instead (`.streaming-tree-template`, see
[`docs/visual-template-packages.md`](docs/visual-template-packages.md)).
See [`docs/visual-templates.md`](docs/visual-templates.md) for the
JSON format's own full contract.

### Persisted overlay profiles

Each overlay profile (`internal/domain/chatoverlay`, five SQLite tables
added by migration `0011`) stores its own layout, visibility toggles,
filters, typography, colors, animation and role-highlighting settings as
explicit, individually validated columns — never a settings JSON blob.
An overlay has its own management id and a separate, higher-entropy
**public slug** (160 bits via `crypto/rand`) that can be rotated
independently at any time, immediately invalidating the old public URL.
An overlay's own hidden-user list is deliberately separate from the
operator Chat page's hidden-user list — a user can stay visible to the
operator while being hidden from one specific public overlay.

### The public overlay projection

`internal/chatoverlay` is a second, independent consumer of the
operator-chat projection's own revision stream (stage 9's
`internal/operatorchat`) — it never subscribes to the Engagement Event
Bus directly, so none of stage 9's lifecycle, deduplication or badge/
emote-resolution logic is duplicated. For every overlay it keeps its own
filtered, bounded, in-memory current-item view plus a separate revision
ring (fixed capacity, not configurable) for live Server-Sent Events
replay. Moderation and system items never reach any public overlay,
regardless of settings; a deleted message is either removed outright or
replaced with a placeholder that never carries the original text,
depending on the overlay's own setting. A settings change triggers an
immediate rebuild and a public reset — visible on a connected Browser
Source within moments of clicking Save.

### The public and management APIs

`GET /api/public/chat-overlays/{publicSlug}/config`, `/items` and
`/stream` require no authentication (the public slug itself is the only
thing standing between the URL and its content, exactly like every other
public overlay tool) and never answer an unknown or disabled slug with a
hard error — a Browser Source instead gets an empty, transparent overlay,
matching how a live broadcast should degrade. The management API
(`/api/chat-overlays/...`) creates, edits, deletes and rotates overlays,
and manages each overlay's own accounts, hidden users, blocked terms and
activity-type selection. The frontend renderer
(`apps/web/src/components/chat-overlay/`) is shared, unchanged, between
the real public route (`/overlay/chat/:publicSlug`, with no `<AppShell>`
anywhere in its render tree) and the Overlays management page's own live
preview panel.

### Hydration, live updates and exit animation

The overlay route fetches `/config` once, then opens the public SSE
stream — its own **first event is always a complete reset** of every
item currently visible, so the route never merges a separate
`/items` snapshot fetch with the stream: doing so would race two
independently-fetched views of the same mutable state against each
other. `/items` still exists and stays fully supported for a script,
a diagnostic tool, or any other direct API consumer that only needs a
one-shot read — the React route simply has no reason to call it. See
[`docs/obs-browser-source.md`](docs/obs-browser-source.md) for the full
reasoning and the reconnect/`Last-Event-ID`/gap behavior.

Since Stage 13B, `/config` additively reports `renderingMode`
(`"legacy"` or `"visual_design"`) plus the current safe public design
when design-driven. Saving or deleting a design while the page is open
emits a `chat-overlay.presentation` revision on the same SSE
stream/ring-buffer/replay mechanism every other overlay event already
uses; the route refetches `/config` on that event and re-renders every
currently-visible item under the new presentation — existing item
state itself is untouched, so nothing is duplicated or resurrected. A
pre-Stage-13B client that never recognizes the new event name or fields
keeps working unchanged, since both are purely additive.

Removing an item from the overlay carries one of two safety classes,
never left to guesswork on the frontend: a **cosmetic** removal
(natural message-lifetime expiry, or the oldest item evicted once
`maxVisibleItems` is exceeded) may use the profile's own configured,
bounded exit animation; every other removal — a moderator deleting a
message, a chat or per-user clear, or any settings change that hides an
item (a newly blocked term, a newly hidden user, a filter toggle, an
account deselected, the overlay disabled or deleted) — is **immediate**,
never animated, and never carries the removed item's own text or any
other content in its own payload. `prefers-reduced-motion` disables
exit animation the same way it already disables entry animation.
Exit animation is one of a fixed, safe enum (`none`/`fade`/`slide_up`/
`slide_left`/`scale`) validated server-side — never arbitrary CSS, a
keyframe string, or an easing function from the backend. The Overlays
management page's own preview panel runs this same reducer and
renderer against local synthetic fixtures (never published to Twitch,
operator chat, or a public stream) and has two buttons demonstrating
the split honestly: "Simulate expiry" plays the draft's own configured
exit animation, and "Simulate moderation removal" always applies on
the same tick with no animation, exactly like a real moderation
deletion — it is a fixed demonstration, not a general visual designer.

### Verifying it for real

`scripts/verify-chat-overlay.mjs` exercises the whole stack end to end
against the real backend and the same kind of fake Twitch OAuth/Helix/
EventSub servers the other engagement scripts use — safe defaults, a
live message reaching a filtered public overlay, every filter (accounts,
hidden users, bots, commands, blocked terms, activity types), capacity/
expiry eviction, deletion/clear scoping, slug rotation, restart behavior,
and a final scan confirming no chat text, blocked-term value, hidden-user
data or public slug ever appears in a log line — entirely on loopback,
with **no real Twitch account or OBS installation involved**. See
[`docs/progress.md`](docs/progress.md) for exactly what it covers.
`scripts/verify-chat-overlay-designer.mjs` does the same for Stage 13B's
own Designer/visual-design layer: save/revision-conflict/delete, a real
fake-Twitch message resolving a saved design's own bindings, filtering
and moderation staying authoritative and immediate under a design, a
live save updating visible items with no duplication, reconnect replay
of the presentation change, and design survival across a restart.
`scripts/verify-visual-templates.mjs` covers Stage 14A's own template
library - unlike every other engagement script, it needs no fake
Twitch server at all, since template management and alert-rule/chat-
overlay creation never require a connected account.

---

## Sending Twitch chat manually

Stage 11A adds a real, manual outbound-chat foundation: a connected
Twitch account can, after its own explicit additional-permission step,
send chat messages and replies to its own channel as itself, from a
composer built into the Chat page. There is **no separate bot
identity** — every sent message is attributed to the same Twitch
account already connected under
[Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata),
using the real
[Send Chat Message](https://dev.twitch.tv/docs/api/reference/#send-chat-message)
API — never IRC, never a scraped or automated browser session. See
[`docs/provider-integrations/twitch-outbound-chat.md`](docs/provider-integrations/twitch-outbound-chat.md)
for the fully researched contract this is built on.

**What this stage does not implement.** A separate bot account,
announcements, whispers, pinned messages, and remote moderation
actions sent *to* Twitch remain unimplemented — see
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).
Scheduled/randomized messages, message groups, chat commands,
cooldowns and placeholder substitution — planned as stage 11B when
this section was first written — are now real, built directly on top
of this same dispatcher; see
[Scheduled messages and chat commands](#scheduled-messages-and-chat-commands)
below. This stage itself is deliberately scoped to **manual,
operator-initiated** sending only.

### A third, independent capability profile

Reading chat (stage 8A's `user:read:chat` and its four siblings) and
sending chat (`user:write:chat`) are two separate, independently
assessed capability profiles on the same connected account — alongside
the original metadata profile from stage 7A
(`channel:manage:broadcast`). An account can be healthy for metadata
and inbound engagement while still needing its own separate
outbound-chat permission upgrade, shown as its own capability state
(unsupported / permission required / ready / error) wherever the
composer or its status is shown. Authorizing outbound chat reuses the
same identity-bound Device Code Flow every other permission upgrade in
this application uses: it requests the **union** of the account's
current scopes plus `user:write:chat` (never narrowing, never
requesting `user:bot` or `channel:bot`, which this application does
not use), rejects a completion from a different Twitch identity, and
atomically replaces the token bundle only once the upgrade succeeds.

### The in-memory outbound-chat dispatcher

`internal/outboundchat` keeps one bounded, in-memory, per-account send
queue (`internal/outboundchat/dispatcher.go`) — reset on every backend
restart, never written to SQLite, exactly like the Engagement Event
Bus and the operator-chat projection above. Only one send is ever
in flight per account at a time, order is preserved, and a queue at
capacity fails a new send explicitly rather than growing without
bound. A local rate limiter caps how often a single account can start
a new send (no more than one dispatch per second, and no more than 20
within a rolling 30-second window) independently of any other
account's own limiter, and a real Twitch `429`/rate-limit response
paces the next send using Twitch's own reported reset time rather than
guessing. **A send is never automatically retried** except for a
single, transparent refresh-and-retry on exactly one `401`, the same
rule every other Twitch call in this application already follows — a
`403`, `422`, `429`, `5xx`, or an uncertain transport failure (the
request may or may not have reached Twitch) is always surfaced
honestly rather than silently retried, since retrying an uncertain
send risks a real duplicate message.

### The Chat page composer

The Chat page's composer appears only for a Twitch-capable connected
account (with an account selector when more than one qualifies),
labeled **"Send as \<display name\>"** — never "bot" — with a live
character counter matching the backend's own 500-Unicode-code-point
limit exactly (a single emoji counts as one code point, the same way
the backend counts Unicode runes, not UTF-16 units). **A sent message
is never optimistically appended to the timeline.** The composer only
ever shows a confirmed send outcome; the message itself reappears in
the merged timeline the same way any other message does, once
Twitch's own EventSub delivery echoes it back through the existing
Engagement Event Bus and operator-chat projection described above —
if inbound engagement is not enabled for that account, the composer
says so plainly instead of implying a local echo is coming. A
persistent notice discloses that Twitch's own Shared Chat feature may
distribute a sent message to other channels in the same session,
without ever claiming a session is currently active (this application
has no way to know that). Every non-success outcome — permission
required, backend unavailable, rate-limited (with a formatted retry
time when Twitch provided one), dropped by Twitch (for example held by
AutoMod), or delivery-unknown — is shown with its own explanation, and
only a confirmed send clears the composer; every other outcome leaves
the typed text in place to edit and resend.

### Replies

A **Reply** action appears on an eligible operator-chat item — a real,
non-deleted Twitch message with a known provider message id, never an
activity, a moderation row, a deleted placeholder, or a non-Twitch
item — and locks the composer to that message's own connected account,
showing a small preview with a cancel control. The reply target is
cleared only after a confirmed send; a validation, drop, or
rate-limit failure preserves both the typed text and the active reply
target so the operator can just fix and resend. A sent message's own
provider message id (needed to let *others* reply to it later) was
added to the operator-chat item model as a narrowly-scoped field —
deliberately never added to the public overlay DTO, since a public
Browser Source viewer has no legitimate use for it.

### Verifying it for real

`scripts/verify-twitch-outbound-chat.mjs` exercises the whole feature
end to end against the real backend and the same kind of fake Twitch
OAuth/Helix/EventSub servers the other engagement scripts use,
extended with a `POST /chat/messages` fake and a `refresh_token` grant
— the exact scope union (including the absence of `user:bot`/
`channel:bot`), an identity-mismatched upgrade rejected, a successful
upgrade persisting, a send using the account's own provider user id
for both broadcaster and sender with no `for_source_only`/`pin` ever
sent, reply-parent forwarding, a `200` response with `is_sent:false`
surfaced as a stable dropped error (Twitch's own drop-reason prose
never exposed), a single `401` transparently refreshed and retried
exactly once, a second `401` stopping rather than looping, every other
error status mapped and never auto-retried (with `429` exposing a
sanitized retry time), two connected accounts sending with fully
isolated queues and rate limits, and a sent message's real EventSub
echo appearing exactly once with no optimistic duplicate — entirely on
loopback, with **no real Twitch account or network request to Twitch
involved**. See [`docs/progress.md`](docs/progress.md) for exactly
what it covers.

---

## Scheduled messages and chat commands

Stage 11B adds the automation layer on top of stage 11A's outbound-chat
foundation: **scheduled bot messages** sent on a timer, and **safe chat
commands** that reply to a `!command` typed by a real viewer. Both
reuse the exact same per-account dispatcher, permission profile and
Twitch Send Chat Message call stage 11A introduced — there is no
second outbound pipeline, no separate bot identity, and no new Twitch
scope. Everything here is managed from its own **Automation** page,
separate from Chat and Overlays.

**What this stage (11B) did not implement, and what has changed
since.** At stage 11B, alerts, donations, Bits/sub alert rendering,
sounds, TTS, goals/counters, a visual overlay designer, Kick/TikTok
outbound chat, a separate bot account, IRC, whispers, announcements,
pinned messages, and any remote moderation action (bans, timeouts,
message deletion) all remained exactly as planned. Alerts and Bits/sub
alert rendering shipped in stage 12A, a visual overlay designer in
stage 13, donations in stage 16A, and TTS in stage 17A; goals/counters,
Kick/TikTok outbound chat, a separate bot account, IRC, whispers,
announcements, pinned messages, and remote moderation actions are
still unimplemented. YouTube scheduled messages and chat commands
*are* implemented, as of Stage 15A, through this exact same
dispatcher — see
[Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents).
See
[`docs/engagement-architecture.md`](docs/engagement-architecture.md).
Nothing in this stage can execute arbitrary code: there is no
scripting language, no regular expressions, no arbitrary HTTP
webhooks, and no shell/filesystem/SQL access anywhere in a schedule or
command definition — only a closed set of fixed behaviors and a closed
placeholder language, described below.

### Scheduled messages

A schedule has one or more target accounts (each with an optional
destination-metadata context, used only for placeholders like
`{streamTitle}`), one or more message alternatives (chosen at random
each send, deliberately avoiding an immediate repeat when there is
more than one), a fixed interval (60 seconds to 24 hours), an optional
first-send delay, and an optional extra random jitter added on top of
the interval — never subtracted from it, so a schedule never fires
early. Two independent, optional gates can suppress an otherwise-due
send without ever queuing it for later: **only while this
application's own local ingest is actively receiving** (MediaMTX's
local publish state — never Twitch's `stream.online`, never the
outgoing FFmpeg branches, never a viewer count) and a **minimum number
of real human chat messages** seen since that schedule's own previous
successful send, counted per target account and reset to zero after
each successful send. A **maximum sends per hour** ceiling additionally
caps actual successful automated sends, independently of Twitch's own
rate limit, which still applies on top of it. All scheduling state —
next-run time, per-account activity counters, rolling send counts — is
kept **in memory only, exactly like the outbound-chat dispatcher
itself**: a backend restart resets it cleanly, never replays a missed
run, and never sends a backlog of "catch-up" messages; every enabled
schedule simply gets a fresh next-run time after its configured first
delay. A **Send now** action bypasses the interval, first delay and
chat-activity gate for one explicit, confirmed, one-off send to
explicitly chosen targets — it still honors the streaming-only gate by
default, still goes through the same placeholder rendering and
dispatcher/provider limits as every other send, and is never described
as a test or a preview, because it sends a real message.

### Chat commands

A command matches a single, fixed `!` prefix — never configurable,
never a slash command — followed by its canonical name or one of its
aliases, case-insensitively, as the *first* token of an otherwise
plain chat message; anything typed after it is ignored, and a `!`
appearing mid-message or doubled (`!!name`) never matches. Command
names and aliases are short, plain, lowercase, ASCII-only, and must be
**globally unique** across every command in this application, so a
match is never ambiguous. Each command can require a minimum viewer
role (everyone, subscriber, VIP, moderator or broadcaster — matched
against what the normalized chat event itself reports, never inferred
from a username), and can enforce a global cooldown and a separate
per-user cooldown, both reset on every backend restart just like
schedule state. **A message from the same connected account that would
send the reply can never trigger that command itself** — matched by
comparing Twitch provider user IDs, never by trying to recognize the
application's own previously-sent message IDs — which is what makes it
safe for a command's own response to legitimately start with `!`
without ever causing a reply loop. A command response is never
retried; if it cannot enter the dispatcher within a few seconds of the
triggering message (queue full, permission not ready, etc.), it is
dropped rather than sent late.

### The placeholder language

Both schedules and commands render their message template through the
same small, closed placeholder language: `{channelName}`, `{platform}`
and `{channelUrl}` are always available; `{streamTitle}` and
`{streamUptime}` are available only when the target has enough local
context to resolve them (a linked destination's saved metadata for the
title, this application's own local ingest start time for the uptime —
never a value fetched from Twitch at send time). There are no
conditionals, functions, loops or custom formats — just literal text
and `{name}` substitutions, with `{{` and `}}` as the only way to write
a literal brace. An unknown placeholder name is rejected when the
schedule or command is saved; a known placeholder that simply cannot
be resolved for a particular send (no destination context configured,
for example) is skipped rather than sent with a literal `{streamTitle}`
in it. A **Preview** action renders a template locally against a
chosen account, with no network request to Twitch and nothing sent —
it exists purely to show the operator what a message will actually
look like, including its live character count against the same
500-code-point provider limit every other send already respects.

### The automation runtime and its API

`internal/chatautomation` owns one centralized, concurrency-safe
runtime (a poll-based scheduler plus a single Engagement Event Bus
subscription shared between the command matcher and the schedule
activity counters — never a dedicated goroutine per schedule or per
command, and never a second, redundant EventSub connection) sitting on
top of the same `internal/outboundchat` dispatcher stage 11A built;
persisted schedule and command *definitions* live in SQLite, exactly
like every other configuration in this application, while all runtime
state (next-run times, cooldowns, activity counters, rolling send
counts) stays in memory only. A create, edit, enable/disable or delete
takes effect immediately, without a backend restart. The
`/api/chat-automation` REST API (see [REST API](#rest-api) below)
exposes schedules, commands, a stateless preview, manual send-now and
a combined status snapshot — the snapshot and every log line describe
*what* is happening (states, counts, skip reasons) but never persist
or log an actual message body, template or triggering username.

### Verifying it for real

`scripts/verify-chat-automation.mjs` exercises the whole feature
end to end against the real backend, the real Engagement Event Bus and
the real outbound-chat dispatcher, using the same kind of fake Twitch
OAuth/Helix/EventSub servers the other engagement scripts use — a
schedule and its targets/messages persisting, a command and its
aliases persisting, preview resolving `{channelName}`/`{platform}`/
`{channelUrl}` and rejecting an unknown placeholder, a scheduled send
waiting while local ingest is not receiving and never sending a
backlog once it is, a minimum-chat-activity gate blocking until enough
real messages arrive and resetting after a successful send, Send Now
working with a per-target result, a command matching its canonical
name and an alias while ignoring arguments and a mid-message `!`, the
self-message rule preventing a reply loop, role and cooldown gating,
disabling a schedule or command stopping it immediately, and a backend
restart preserving definitions while resetting all runtime state with
no missed-run catch-up — entirely on loopback, with **no real Twitch
account or network request to Twitch involved**. See
[`docs/progress.md`](docs/progress.md) for exactly what it covers,
including the deeper timing/role/cooldown scenarios covered instead by
named Go tests.

---

## Alerts

Stage 12A adds a real **alert engine** on top of the same normalized
Engagement Event Bus stage 8A built: persisted alert profiles and rules, a
provider-independent matching engine, a bounded in-memory alert queue, and
a fixed (not yet a free-form designer) alert presentation, served on its
own public OBS Browser Source route. Stage 12B then closed out the queue
itself with real **bounded alert grouping** (collapsing a burst of
compatible near-simultaneous alerts into one, with a truthfully aggregated
count/quantity) and real, opt-in **mid-alert preemption** (a
strictly-higher-priority alert immediately replacing one already playing,
with no resume of the interrupted one). Managed from a new **Alerts**
page, separate from Chat, Overlays and Automation.

Stage 13A then added a real, bounded **visual designer** on top of that
same fixed-presentation alert engine: a shared, provider-independent
visual-design document (`internal/domain/visualdesign`, schema version
2 since Stage 13B — see below), persisted one-per-rule with
optimistic-concurrency revisions, a
shared React renderer used identically by the Designer's own canvas and
the public Browser Source route, and a new **Alert Overlay Designer**
page (`/alerts/rules/{ruleId}/designer`) for drag/resize/property-panel
editing of a bounded set of layer kinds (rectangle shape, closed-binding
text, platform icon, avatar). Every existing Stage 12 rule keeps
rendering through the original fixed renderer unless and until an
operator explicitly opens the Designer and saves — opening the page
alone persists nothing. See
[`docs/visual-designs.md`](docs/visual-designs.md) for the full document
contract.

**What this stage does not implement.** Uploaded/remote images, GIFs,
video, sounds, fonts, custom CSS, or arbitrary HTML/JS were not part of
Stage 13A — its own layer kinds and typography/animation options are
closed, bounded enums, never free-form CSS or markup, and Stage 14A's
own template library (below) added no new primitive either. Managed
images/video/fonts and a portable *archive* template package were later
added deliberately, as their own dedicated stage — see
[`docs/visual-template-packages.md`](docs/visual-template-packages.md)
(Stage 14B). No
new Twitch scope and no new EventSub subscription type were added for
12A, 12B or 13A: alerts only ever match events already reaching the
Event Bus, and the alert engine never talks to Twitch directly.
A real external donation-service connector (StreamElements), a shared
audio/text-to-speech runtime, and a persistent goals/counters
foundation with the full public supporter/activity widget suite were
added as their own dedicated stages — see
[`docs/provider-integrations/external-donations.md`](docs/provider-integrations/external-donations.md)
(Stage 16A), [Text-to-speech and audio](#text-to-speech-and-audio)
(Stage 17A), and [Persistent goals and supporter widgets (Stage 18A/18B)](#persistent-goals-and-supporter-widgets-stage-18a18b).
Real alert-event history, queue
contents, and every counter (including the grouping/preemption ones)
are **runtime-only** — never persisted — exactly like the automation
runtime above. Saved visual designs and their revisions **are**
persisted (SQLite, migration `0015_visual_designs.sql`); the Designer's
own undo/redo history and any unsaved draft are frontend memory only,
never sent to the backend until Save.

Stage 13B then extended this same shared visual-design foundation to
the chat overlay: see
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)
below for the **Chat Overlay Designer**.

### Genuinely supported alert events

What the real Twitch normalization code actually produces:
**follow, subscription, resubscription, gifted subscription, gift-sub
batch, Bits, raid, and channel-point redemption.** What the real
YouTube normalization code produces (Stage 15A): **membership,
membership milestone, Super Chat, and Super Sticker** — the last two
carry this application's first real monetary threshold (an inclusive
integer-micros amount range, exact-currency-match only, never a
currency conversion). Stage 16A adds a real **donation** event type from
a connected StreamElements donation source, reusing that exact same
monetary-threshold model (integer micros, exact-currency-match, no FX);
a donation is never auto-grouped (each one is individually meaningful),
though the existing priority/preemption model still lets a large
donation jump the queue. Chat messages and moderation events never
become alerts on any provider.

### Alert profiles

An alert profile is an independent OBS Browser Source destination — its
own public URL, its own queue, its own fixed theme/position/text-alignment
choice, and its own queue-capacity/expiration bounds (1–500 items,
5–3600 seconds). Any number of profiles can exist; one profile's queue or
a slow Browser Source never blocks another's. Like an overlay profile, the
public URL uses a separate, high-entropy, rotatable slug — never the
profile's own management id, never a credential, and rotating it
invalidates the old URL immediately.

### Alert rules

A rule belongs to one profile and matches one event type. It can filter by
provider and by specific connected account (empty means "any"), set an
inclusive minimum/maximum quantity threshold (Bits and gift-sub-batch
tiers, for example, can be made non-overlapping this way — the Rules panel
warns, without ever silently suppressing either side, if two tiers on the
same profile *do* overlap), a priority (0–100), a duration (1–30 seconds),
which fields to show (platform/username/message/quantity — only the ones
that field genuinely exist for that event type), a closed placeholder
template (`{username}`, `{platform}`, `{eventType}`, `{quantity}`,
`{message}`, `{rewardTitle}`, `{groupCount}` — only where the event type
actually supports it), and bounded entry/exit animations reusing this
application's own existing overlay animation classes — never an
arbitrary CSS class, never a backend-supplied stylesheet. Every field the
rule editor shows is capability-driven: a condition that does not make
sense for the selected event type (a quantity threshold on a follow rule,
for instance) is not just hidden — the backend rejects it outright if
sent anyway.

A rule may also opt in to two independent behaviors. **Group similar
alerts**, shown only for the 3 event types with a genuinely safe grouping
strategy (Bits and gift-sub batch, by same actor and truthfully summed
quantity; channel-point redemption, by same actor and the same specific
reward), with a configurable window (1–30 seconds, fixed once the first
member arrives — a later member never extends it) — unavailable
elsewhere, and rejected by the backend if forced on anyway. Enabling it
forces "show message" off and forbids a `{message}` placeholder, since a
truthful grouped alert can never show one arbitrary member's own message.
**Interruption** is always available and is two separate, independent
toggles: whether *this* rule's own alert may be interrupted by a later,
higher-priority one (default: yes), and whether *this* rule's alerts may
themselves interrupt a lower-priority one already playing (default: no).
Both default to Stage 12A's own original non-interrupting behavior.

### The alert queue

Exactly **one alert plays at a time per profile**. A bounded, in-memory
queue (never persisted) orders pending alerts by priority, then
first-in-first-out within equal priority — never by database row order,
and multiple matching rules for the same event always enqueue
independently rather than "first rule wins." A queued alert older than
the profile's own maximum age is discarded, never played, the moment it
would otherwise be promoted. **Pause** freezes queue progression — the
currently playing alert always finishes normally, but the next one never
promotes until **Resume**. **Skip Current** removes the playing alert
immediately (counted separately from a normal completion) and advances
unless paused. **Replay Previous** re-shows the single most recently
completed or skipped alert, without ever recreating a real Engagement
Event or affecting any real counter. **Clear Queue** removes only
not-yet-played items — the currently playing alert is untouched, and it is
a separate, distinct action from Skip Current. Disabling a profile hides
its current alert and empties its queue immediately; re-enabling never
replays whatever arrived while it was disabled.

**Grouping** (Stage 12B) only ever merges *still-queued* alerts — the
currently playing one is never touched. A newly matched event from the
same rule (the exact same saved rule version — editing a rule starts a
fresh group), the same connected account, and the same actor merges into
an existing compatible group if one is open and not yet at its bounded
member limit, incrementing its `groupCount` and, where the event type
allows it, truthfully summing its quantity — a merge never creates a new
queue entry or takes a new capacity slot. **Preemption** (Stage 12B) only
ever fires for a strictly higher-priority incoming candidate whose own
rule opts in to interrupting, against a current alert whose own rule
allows being interrupted — equal priority never preempts, a paused queue
never preempts, a replayed alert never preempts, and a synthetic Test
Rule preview may only ever preempt another synthetic preview, never a
real alert. A preempted alert is hidden immediately with no resume of its
remaining duration and becomes the one available replay snapshot; the
incoming alert shows immediately with its own full, fresh duration.

### Local synthetic test alerts

**Test Rule**, on any saved rule, creates one alert through the exact same
queue and public renderer a real match would use — using that rule's real
presentation settings, regardless of its own provider/account/quantity
filters — with **no real Twitch account and no real Engagement Event Bus
event** involved. Every test alert is explicitly marked `synthetic` end to
end (queue status, the public stream payload), counted in its own separate
counter, and a genuine `Synthetic` Engagement Event is always ignored by
real rule matching — a preview can never be mistaken for, or pollute the
counters of, a real supporter event.

### The fixed alert presentation

A rule's duration, visibility toggles, template and animation choice, plus
a profile's theme/position/text-alignment, are the whole of what Stage 12A
lets you configure — never arbitrary coordinates, layers, uploaded
media, custom fonts, or custom CSS/HTML/JS (Stage 13's job). Animations
reuse the same bounded, application-owned classes the chat overlay
already uses, respect `prefers-reduced-motion`, and always complete via a
hard fallback timer rather than relying solely on an `animationend` event
that might never fire. No new asset upload path exists for alerts: only
this application's own existing safe provider/platform glyph mapping and
an already-available normalized avatar URL are ever used — never a
fresh, per-alert Twitch API call, never hotlinking an arbitrary
event-supplied URL. The renderer works correctly with no avatar,
an anonymous user, no message, and no quantity.

### The public OBS Browser Source alert route

Each profile's alert renders at its own `/overlay/alerts/{publicSlug}`
route — transparent, no application chrome, no queue contents, no account
diagnostics, and the exact same React renderer component the Alerts
page's own rule-editor preview uses, so what you preview is what OBS
actually shows. It hydrates by fetching a small public config, then
opening a Server-Sent Events stream whose first event is always a
complete current-state reset (the current alert if one exists, and the
paused flag) — **never the queue's future contents**, which stay
management-only. Reconnecting with `Last-Event-ID` resumes without a gap
when possible; an unbridgeable gap sends an explicit reset rather than
silently skipping alerts. Like the chat overlay before it, the public
slug is an unguessable local locator, not a credential or an
authentication token, and is never logged.

### Verifying it for real

`scripts/verify-alerts.mjs` exercises the whole feature end to end
against the real backend, the real Engagement Event Bus, and the same
kind of fake Twitch OAuth/Helix/EventSub servers the other engagement
scripts use — profile and rule persistence (including two non-overlapping
Bits tiers), the public config/stream never leaking management data, a
real fake-Twitch follow notification reaching the public stream as a
non-synthetic alert, account filtering, Bits-tier selection with no
cross-tier triggering, raid/redemption/subscription/resubscription/gift
events staying distinct, Test Rule going through the real queue,
strict priority ordering independent of insertion order, expiration,
the pause policy, skip, replay (never creating a real Event), clear, the
deterministic capacity policy, profile disable/enable isolation, two
profiles staying isolated, slug rotation, and a full backend restart
that preserves profiles/rules while resetting every runtime counter with
no replay — entirely on loopback, with **no real Twitch account or OBS
Browser Source involved**. See [`docs/progress.md`](docs/progress.md) for
exactly what it covers, including the scenarios covered instead by named
Go tests.

`scripts/verify-alert-advanced-queue.mjs` does the same for Stage 12B
specifically: the grouping/interruption rejection matrix, real Bits and
channel-point-redemption events merging correctly (and never merging
across a different actor or a different reward), real-event preemption
with the public stream's hide-then-show sequence asserted directly (hide
carries only the outgoing alert's id and reason, never its rendered
content), equal-priority and non-interruptible protection, paused-queue/
synthetic/replay preemption guards, and a full backend restart that
preserves every new rule field while resetting the new counters — run at
least twice per change. See [`docs/progress.md`](docs/progress.md) for
exactly which scenarios this covers versus a specific named Go/frontend
test instead.

`scripts/verify-alert-designer.mjs` does the same for Stage 13A: the
visual-design draft/save/revision-conflict/delete HTTP API, a
representative validation-rejection matrix, a real fake-Twitch event
rendering the saved design end to end on the public stream with no
editor-only field ever leaking, snapshot immutability when a design is
saved while an alert is current/queued/replayed, Test Rule and grouped/
preempted alerts each keeping their own correct design, Reset to
legacy, rule-deletion cascade, and design survival across a backend
restart — run at least twice per change. See
[`docs/progress.md`](docs/progress.md) for exactly which scenarios this
covers versus a specific named Go/frontend test instead.

`scripts/verify-chat-overlay-designer.mjs` does the same for Stage 13B:
the chat-overlay visual-design HTTP API sharing the same wire shapes as
the alert one; owner-kind independence (an alert design and a chat
design never disturb each other); a real fake-Twitch message resolving
a saved design's own username/message bindings while still carrying its
real content; the data-needs override that never lets a legacy
`showAvatar:false` toggle starve a design that genuinely needs the
avatar; blocked-term/hidden-user filtering and immediate moderation
removal staying authoritative under a design; a live save updating
every currently-visible item with no duplication or resurrection,
followed by a `chat-overlay.presentation` event; reconnect replay of
that event via a real `Last-Event-ID`; Reset to legacy and overlay-
deletion cascade; and design survival across a backend restart — run at
least twice per change. See [`docs/progress.md`](docs/progress.md) for
exactly which scenarios this covers versus a specific named Go/frontend
test instead.

`scripts/verify-visual-templates.mjs` does the same for Stage 14A: the
built-in registry (at least 3 alert, at least 3 chat, none ever a
SQLite row) validated at real backend startup; compatibility scoped to
a real alert rule and a real chat overlay, including a target-mismatch
blocker and an event-type-unavailable blocker; "Save as template"
never touching the owner's own saved design; import-preview migrating
an embedded version-1 document to the current version while persisting
nothing; a client-supplied local id, an unknown field, an unsupported
template version, an unsupported future design version, and an
oversized body all rejected; a full export → delete → re-import
semantic round trip; built-in immutability; and survival across a
backend restart — run at least twice per change, needing no fake
Twitch server at all. See [`docs/progress.md`](docs/progress.md) for
exactly which scenarios this covers versus a specific named Go/frontend
test instead.

---

## Text-to-speech and audio

Stage 17A adds a real **shared audio runtime and text-to-speech
foundation**: a provider-independent `Provider` abstraction
(`internal/provider/tts`), a real Windows SAPI implementation
(`SAPI.SpVoice`/`SpMemoryStream` via COM Automation, using
`github.com/go-ole/go-ole`), a bounded runtime audio queue
(`internal/audio`) consuming the same Engagement Event Bus every other
stage does, and a public OBS Browser Source audio route. Managed from
a new **Audio** page, separate from Alerts, Chat, Overlays and
Automation.

Only one global audio configuration exists — unlike alerts' many
profiles — covering: provider mode (`disabled`/`system`/`local`/`cloud`;
only `disabled` and `system` are real, `local`/`cloud` are rejected by
validation until a real engine exists), voice selection, speed/volume,
which event types trigger speech (or supporter-only mode, reusing the
same closed capability-table pattern alerts already established),
per-source/per-provider filtering, an exact-currency monetary
threshold (never compared across currencies, mirroring alerts' own
rule), a Bits threshold, blocked words, URL removal, manual-approval
mode, cooldowns (global and stable per-user, never a fabricated
identity for an anonymous event), queue capacity, and max spoken text
length.

Every eligible event goes through a fixed text-preprocessing pipeline
(command suppression, URL removal, blocked-word filtering, repeated-
character normalization, whitespace normalization, max-length
enforcement) before being enqueued; synthesis happens **just in
time**, only once an item is promoted to "current" and a renderer is
actually connected — never for the whole queue up front. Generated
audio is ephemeral: never written to SQLite, never becomes a
template/visual asset, and the queue itself never survives a backend
restart.

The public route (`/overlay/audio/{publicSlug}`) uses the same SSE +
narrow-POST-acknowledgement protocol the alert overlay established:
`audio.current`/`audio.idle`/`audio.reset`/`audio.gap` events carrying
only a short-lived playback token/URL, volume and sequencing — never
the original event, username, donor email, or message text — plus a
single-active-renderer lease (a new connection immediately supersedes
the previous one; a stale session's acknowledgement is rejected) and
honest `waiting_for_renderer`/interrupted states rather than silent
auto-replay. See [`docs/audio-tts.md`](docs/audio-tts.md) for the full
researched contract, including why SAPI was chosen over WinRT and the
OBS Browser Source audio-support research.

**What this stage does not implement.** Persistent alert sound
assets, per-alert-rule TTS/sound, synchronization with alert playback,
a local or cloud TTS engine (both rejected by validation, not silently
accepted), and any audio extension of the Stage 14B template-asset
format — all deliberately deferred to Stage 17B (since shipped — see
below). Non-Windows builds compile and run identically; the `system`
provider honestly reports itself unavailable there rather than faking
success.

### Persistent alert audio and per-rule TTS (Stage 17B)

Built directly on that same shared audio runtime — never a second
playback engine. A managed **audio asset** domain
(`internal/domain/audioasset`) accepts exactly one closed format, 16-bit
PCM WAV (RIFF/WAVE, mono or stereo, 8–192 kHz sample rate), verified by
an independent structural signature parser — never a filename
extension, never a caller-declared media type alone, never FFmpeg/
ffprobe. Blobs are content-addressed and deduplicated in a second
`FileStore` instance, a sibling of Stage 14B's own managed-visual-asset
store.

An alert rule may now configure a persistent **sound** (chosen from
that managed library, with its own volume) and/or rule-owned **TTS**
(reusing the exact same `{placeholder}` grammar/renderer the rule's
own visual text template already uses — never a second grammar), each
validated the same way every other rule field already is. Playback is
synchronized with the shared `internal/audio` queue: a rule-owned
item always preempts a currently-playing global-TTS item outright, but
never preempts another alert instance's own current audio; a
configured sound plays, then TTS, automatically on natural completion
(never on grouping, since grouping only ever merges still-queued
instances); and the alert stays visible for a bounded extra hold
(capped at 15 seconds) while its own linked audio is still genuinely
playing, rather than hiding the instant its own fixed duration elapses.

The Stage 14B package format also gained an optional **manifest schema
v2**: two new sibling objects, `alertAudio` and `audioAssets`, carrying
a template's own sound/TTS preset and its bundled WAV file through a
portable `.streaming-tree-template` archive — legal only for an
alert-target package, validated through the exact same managed-asset
validator a manual upload uses. A purely visual template still exports
as schema v1, byte-shape-identical to before this stage; v2 is written
only when a template actually carries a configured audio preset.
Applying such a template in the Alert Designer updates the visual
draft and the alert-audio draft together, as one combined, one-undo-
step action — never auto-saving the rule.

See [`docs/alert-audio.md`](docs/alert-audio.md) for the full contract.

### Verifying it for real

`scripts/verify-tts-audio.mjs` exercises the whole feature end to end
against the real backend, a real fake StreamElements Astro connector
(reused as the one real Event-Bus-triggering donation source, since
donations are already supporter-family-eligible), and a real
integration-build-only deterministic fake TTS provider (valid
WAV/PCM, no ffmpeg, no network, no dependency on installed voices) —
settings persistence, the runtime queue never surviving a restart,
Test Speak through the real preprocessing pipeline and bounded queue,
source/currency/threshold filtering through the real Event Bus,
manual approval (pending/approve/reject, proving a rejected item is
never synthesized), just-in-time promotion requiring a connected
renderer, the generated URL leaking no source text, playback
acknowledgement lifecycle (including stale/duplicate/wrong-session
rejection and session supersession), renderer disconnect while
playing, skip-current cancelling an in-flight synthesis, forced
synthesis failure/oversize isolating one item, provider-unavailable
and unknown-voice honesty, slug rotation, route strictness, backend
shutdown cancelling in-flight synthesis, and a full PII/secret scan of
every audio-surface payload plus the raw SQLite file — run at least
twice per change. See [`docs/progress.md`](docs/progress.md) for
exactly which scenarios this covers versus a specific named Go test
instead.

`scripts/verify-alert-audio.mjs` exercises Stage 17B the same
representative-subset way: managed audio-asset upload/list/delete-
guard, rule-audio persistence and validation, the bounded visual hold
with zero renderer ever connected, the real sound-then-TTS chain over
the real public audio/alert SSE streams (ordered items, correct
per-item combined volume, no alert/rule identifier or spoken text ever
in the public payload), arbitration against a real global-TTS item,
package v2 audio import/export (a fresh local asset id every time, a
chat-target package carrying `alertAudio` rejected before any asset is
staged, a plain v1 package unaffected), and a final SQLite scan — also
run at least twice per change.

---

## Persistent goals and supporter widgets (Stage 18A/18B)

Stage 18A adds a persistent, provider-independent goal/counter
accumulation engine on top of the same Engagement Event Bus every other
consumer above already reads, plus real public OBS goal widgets. Full
contract: [`docs/goals-widgets.md`](docs/goals-widgets.md).

**The fundamental rule: observed progress, not a provider total.** A
goal's own `current` value means *"events this application has observed
since this goal's own configured baseline"* — never a Twitch/YouTube/
StreamElements-reported canonical total. Streaming Tree has no provider
API client that fetches "current follower count" or "lifetime donation
amount," and none is introduced by this milestone. An operator who
already knows their real current standing sets that as the goal's own
baseline at creation time; the application never fabricates it.

**Four closed goal kinds** — followers, subscriptions, donations, Bits —
each fed by one provider-independent contribution table over the
normalized event model (never scattered contribution logic across
HTTP/frontend/provider code). Selected, deliberate decisions from that
table:

- A follow event contributes `+1`. There is no normalized unfollow
  event today, so a follower goal is honestly "observed follows since
  baseline," never "current follower count."
- A subscription/gifted-subscription event contributes `+1`; a
  resubscription or membership milestone does **not** (a continuing
  subscription being reaffirmed, not a new one). A gift-batch summary
  event never contributes on its own — only its own individual gift-
  recipient events do, so a batch of 5 plus its 5 recipients count as
  exactly 5, never 10 and never 0.
- A donation goal has exactly one configured currency; only a same-
  currency event contributes, in exact integer micros — never a float,
  never FX-converted, matching this project's own standing money rule.
- A Bits goal accumulates the event's own exact integer quantity —
  never converted to money.

**Persistence, dedupe, and concurrency.** Goal configuration and
accumulated state both survive a backend restart (unlike the alert/TTS
runtime queues, which are deliberately transient). The runtime manager
subscribes to the Event Bus at current position only — never replaying
retained history into a goal, so a restart can never double-apply an
already-observed event. A durable, per-goal dedupe ledger (keyed on a
real provider event id when one exists, the connector's own delivery id
otherwise) protects against a provider redelivering the same
notification after a restart; contribution application is one atomic
transaction (an idempotent dedupe-ledger insert plus an atomic SQL
`current + amount` update), proven exact under concurrent contributions.

**Manual baseline management.** Because this application cannot
discover a provider's complete historical total, every goal supports an
explicit **Set current value** and **Reset to baseline** action — data-
layer only, never publishing a fake event, mutating provider state, or
triggering an alert/TTS.

**Public OBS goal widgets.** One goal may have several public widget
profiles, each its own rotatable, high-entropy Browser Source URL under
one generic `/overlay/widgets/{publicSlug}` route. The public stream
sends only the current goal snapshot (title, current/target, basis-
point progress, completion, and the widget's own bounded presentation
fields) — never a database id, account/source id, provider event id, or
any user identity. A goal reaching its target keeps accumulating past
it with no clamp on the stored value; only the rendered progress bar
clamps visually.

**Stage 18B: supporter/activity widgets, richer counters, and bounded
dashboards.** Built directly on the same `WidgetProfile` model above,
widened from one kind (`goal`) to nine. Full contract:
[`docs/supporter-widgets.md`](docs/supporter-widgets.md).

- **Latest follower / latest subscriber / latest donation** — the most
  recent matching event's own display name (or "Anonymous"), provider,
  and time; a latest subscriber specifically means the latest *new*
  subscriber/member — a resubscription or a gift-batch summary never
  overwrites it, only a genuinely new subscription, an individual gift
  recipient, or a new YouTube membership does.
- **Largest donation** — requires one configured comparison currency;
  compares exact integer micros, never FX-converted; a strictly larger
  amount replaces the current session's winner, an exactly equal amount
  never does.
- **Recent supporters** and **event ticker** — bounded, newest-first
  lists (1–20 and 1–50 items respectively) built from two independently
  closed event-family tables, so a gift-batch summary can never
  duplicate its own already-counted individual recipients.
- **Session counters** — one of eight closed metrics (follows, new
  subscriptions, resubscriptions, gifted subscriptions, raids, Bits
  quantity, support-event count, exact-currency support amount)
  observed since the backend started or was last reset — explicitly
  never confused with a Stage 18A goal's own persistent total.
- **Dashboards** — a bounded grid (1–4 columns, 1–8 children) composing
  *existing* widget profiles by reference, never copying their state; a
  dashboard can never contain another dashboard, and deleting a widget
  still referenced by one is rejected until it is removed first.

**The event-derived content above is deliberately runtime-only.**
Unlike a Stage 18A goal's own numeric `current`, a Stage 18B widget's
own display names, donation messages, recent-supporter rows, and ticker
entries are never written to SQLite — this project has always kept
chat/engagement *content* out of persistent storage, and Stage 18B
preserves that boundary exactly. Every such widget's own content starts
empty again after a backend restart, or after an explicit manual reset
— only its own bounded *configuration* (kind, filters, style, dashboard
composition) survives. A donation message is shown publicly only when a
widget's own `showMessage` setting is explicitly enabled (off by
default). The public route, DTO shape, and 1.5-second poll-and-diff
stream mechanism are all identical to Stage 18A's own — one generic
`/overlay/widgets/{publicSlug}` route serves every kind, including a
dashboard composing its own children's snapshots inline.

### Verifying it for real

`scripts/verify-goals-widgets.mjs` (the 21st integration script) drives
real events through the real Twitch EventSub fake, the real fake
YouTube `streamList` gRPC server, and the real fake StreamElements
Astro server — never a separate fake goals event source — proving the
full pipeline end to end: real provider fake → real connector → real
normalization → real Event Bus → the real goals runtime → real SQLite →
the real public widget SSE stream. Covers the gift-batch no-double-
count proof, exact Bits/money accumulation, cross-currency rejection,
duplicate-delivery dedupe, provider/account filters against real linked
accounts, multiple goals matching one real event, manual Set current/
Reset never bumping the configuration revision, widget slug rotation,
goal-in-use delete rejection, full backend-restart persistence, and a
raw-SQLite privacy scan — run at least twice per change.

`scripts/verify-supporter-widgets.mjs` (the 22nd integration script)
extends the same real-fakes-only approach to every Stage 18B kind: the
latest-subscriber "new only" rule, the largest-donation tie rule, the
recent-supporters/event-ticker no-batch-duplication proof, exact
session-counter metrics and cross-currency rejection, dashboard
composition with zero internal id ever exposed publicly, nested-
dashboard rejection, dashboard delete-protection, runtime reset, a full
backend-restart proof that configuration survives while every runtime-
only projection resets to empty, and a raw-SQLite scan proving no
observed display name, donation message, or provider-private field was
ever written to disk.

---

## REST API

All endpoints live under `/api` and return `application/json`.

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `GET` | `/api/health` | Liveness, service name, version and uptime. |
| `GET` | `/api/platform-definitions` | Built-in provider definitions: capabilities, limits and supported option identifiers. |
| `GET` | `/api/platforms` | All configured destinations, ordered, with their provider definition and metadata. |
| `POST` | `/api/platforms` | Create a destination. Responds 201 with a `Location` header. |
| `GET` | `/api/platforms/{id}` | One configured destination. |
| `PUT` | `/api/platforms/{id}` | Replace display name, enabled state and sort order. |
| `DELETE` | `/api/platforms/{id}` | Delete a destination; metadata and tags cascade. Responds 204. |
| `GET` | `/api/platforms/{id}/metadata` | Stored metadata and ordered tags. |
| `PUT` | `/api/platforms/{id}/metadata` | Replace metadata and tags atomically. |
| `GET` | `/api/platforms/{id}/credentials` | Stream-key status: `{configured}`, plus whether the OS credential store is reachable. Never the key itself. |
| `PUT` | `/api/platforms/{id}/credentials/stream-key` | Validate and store a new stream key, replacing any previous one. Body capped at 8 KiB, well below the general 64 KiB limit. |
| `DELETE` | `/api/platforms/{id}/credentials/stream-key` | Delete the stored stream key. Idempotent: deleting an absent key still responds 204. |
| `GET` | `/api/platforms/{id}/output` | A destination's output settings: `{serverUrl, autoRestart}`. Never the stream key. |
| `PUT` | `/api/platforms/{id}/output` | Replace a destination's output settings (full replacement). |
| `GET` | `/api/runtime` | One versioned snapshot: MediaMTX state, ingest state and OBS connection values. |
| `POST` | `/api/runtime/mediamtx/install` | Start a managed installation. Responds 202; 409 if one is already running. |
| `POST` | `/api/runtime/mediamtx/start` | Start the ingest service. 202 accepted, 409 if already running, 422 if missing or incompatible. |
| `POST` | `/api/runtime/mediamtx/stop` | Stop it. Suppresses automatic restart for this stop. |
| `POST` | `/api/runtime/mediamtx/restart` | One controlled stop followed by a start. |
| `GET` | `/api/runtime/ffmpeg` | The resolved FFmpeg dependency: state, source, detected version, minimum version, probed capabilities. Never the executable path. |
| `GET` | `/api/runtime/branches` | Every destination branch's runtime snapshot: state, desired-running, blockers, timestamps, restart count, sanitized last error, real progress. Never a secret or a full destination URL. |
| `POST` | `/api/runtime/branches/{id}/start` | Start one destination. `202` accepted, `200` with `{status:"blocked", blockers}` if ineligible, `409` if already starting/live/restarting. |
| `POST` | `/api/runtime/branches/{id}/stop` | Stop one destination and suppress its automatic restart. `409` if it was not running. |
| `POST` | `/api/runtime/branches/{id}/restart` | One controlled stop followed by a start. |
| `POST` | `/api/runtime/branches/start-enabled` | Start every eligible enabled destination; one ineligible destination never blocks another. Returns a per-destination result. |
| `POST` | `/api/runtime/branches/stop-all` | Stop every running destination. |
| `GET` | `/api/integrations/twitch/config` | Twitch Client ID status: `{configured, source, clientId}` — `clientId` present only when `source` is `"database"`. |
| `PUT` | `/api/integrations/twitch/config` | Save a database-managed Client ID. `409` if an environment override is active, or if changing it while accounts exist. |
| `POST` | `/api/integrations/twitch/device-flow` | Start a Twitch device-authorization attempt. `202` with the attempt snapshot; `409` if one is already active. |
| `GET` | `/api/integrations/twitch/device-flow/{id}` | Poll one attempt's current state. Never contains the device code. |
| `DELETE` | `/api/integrations/twitch/device-flow/{id}` | Cancel an in-progress attempt. |
| `GET` | `/api/connected-accounts` | Every connected account: identity, status, granted scopes, last-validated time. Never a token. |
| `GET` | `/api/connected-accounts/{id}` | One connected account. |
| `DELETE` | `/api/connected-accounts/{id}` | Disconnect: revoke with the provider where possible, then remove locally and cascade any destination link. Responds 204. |
| `POST` | `/api/connected-accounts/{id}/validate` | Validate immediately (instead of waiting for the hourly background check), refreshing the token first if needed. |
| `POST` | `/api/connected-accounts/{id}/reconnect` | Start a new attempt that must resolve to this same account — a device-flow attempt for a Twitch account, an Authorization Code + PKCE attempt for a YouTube one. |
| `GET` | `/api/connected-accounts/{id}/twitch/categories` | Search Twitch categories/games via `?query=`. Requires a healthy linked-or-standalone account. |
| `GET` | `/api/platforms/{id}/connected-account` | The account linked to a destination, or `null`. |
| `PUT` | `/api/platforms/{id}/connected-account` | Link (or replace the link to) an account. Body `{accountId}`. `422` on a provider mismatch. |
| `DELETE` | `/api/platforms/{id}/connected-account` | Unlink, without deleting either side. Responds 204. |
| `GET` | `/api/platforms/{id}/metadata/publish-preview` | What publishing would change right now: remote values (and, for YouTube, the selected broadcast), local values, changed/unchanged/skipped fields, blockers, warnings. |
| `POST` | `/api/platforms/{id}/metadata/publish` | Publish the metadata currently saved in SQLite to the destination's real provider. **No request body** — publishing a draft is not possible. |
| `GET` | `/api/integrations/youtube/config` | YouTube Client ID status — same shape as the Twitch config endpoint above. |
| `PUT` | `/api/integrations/youtube/config` | Save a database-managed YouTube Client ID. Same `409` rules as Twitch's, independent of it. |
| `POST` | `/api/integrations/youtube/oauth-attempts` | Start a YouTube Authorization Code + PKCE attempt. `202` with the attempt snapshot (including the authorization URL to open); `409` if one is already active. |
| `GET` | `/api/integrations/youtube/oauth-attempts/{id}` | Poll one attempt's current state. Never contains the authorization code, PKCE verifier, or CSRF state value. |
| `DELETE` | `/api/integrations/youtube/oauth-attempts/{id}` | Cancel an in-progress attempt and close its temporary loopback listener. |
| `POST` | `/api/integrations/youtube/oauth-attempts/{id}/channel` | Explicitly select one of several owned channels, when the attempt is `awaiting_channel_selection`. Body `{channelId}`. |
| `GET` | `/api/connected-accounts/{id}/youtube/broadcasts` | List the linked channel's active and upcoming live broadcasts. Never ingestion data. |
| `GET` | `/api/connected-accounts/{id}/youtube/categories` | List assignable video categories for the account's effective region. |
| `GET` | `/api/connected-accounts/{id}/youtube/region` | The account's effective category region (saved override, else the channel's own country). |
| `PUT` | `/api/connected-accounts/{id}/youtube/region` | Save an explicit two-letter region override. |
| `GET` | `/api/platforms/{id}/remote-target` | The selected live-broadcast target for a YouTube destination, or `null`. |
| `PUT` | `/api/platforms/{id}/remote-target` | Select a broadcast. Body `{resourceId}`. `422` if it does not belong to the linked channel. |
| `DELETE` | `/api/platforms/{id}/remote-target` | Clear the selection, without touching the account link. Responds 204. |
| `GET` | `/api/engagement/status` | Event Bus status: schema version, buffer capacity, retained count, oldest/newest sequence, active subscribers, and a summary per Twitch connector. No message content. |
| `GET` | `/api/engagement/events` | A bounded snapshot of retained normalized events. Query params `after`/`limit` (capped at 500); reports `gap: true` when `after` refers to an already-evicted sequence. |
| `GET` | `/api/engagement/stream` | Server-Sent Events: live normalized events as they are published. Supports `Last-Event-ID` (or `?after=`) for replay, emits `engagement.gap` when replay is incomplete, and periodic keepalive comments. Bounded concurrent clients. |
| `GET` | `/api/connected-accounts/{id}/engagement` | One Twitch account's connector status plus its capability assessment (required/granted scopes, whether a permission upgrade is required). `422` for a non-Twitch account. |
| `PUT` | `/api/connected-accounts/{id}/engagement` | Enable or disable the connector. Body `{enabled}`. Persists; an enabled connector reconnects automatically after a backend restart. |
| `POST` | `/api/connected-accounts/{id}/engagement/authorize` | Start an identity-bound Device Code Flow requesting the union of the account's existing scopes and the engagement profile. **No request body.** Reuses the Twitch device-flow attempt snapshot shape. |
| `POST` | `/api/connected-accounts/{id}/engagement/restart` | Cancel and restart the connector without changing its persisted enabled setting. **No request body.** |
| `GET` | `/api/connected-accounts/{id}/outbound-chat` | One Twitch account's outbound-chat status: capability (`unsupported`/`permission_required`/`ready`/`error`), required/granted/missing scopes, dispatcher state, queue depth/capacity, last attempt/success times, last stable error code, sanitized retry time, whether sending is currently possible, and a standing Shared Chat disclosure identifier. No credential, no message content. |
| `POST` | `/api/connected-accounts/{id}/outbound-chat/authorize` | Start an identity-bound Device Code Flow requesting the union of the account's existing scopes and `user:write:chat`. **No request body.** Reuses the Twitch device-flow attempt snapshot shape. |
| `POST` | `/api/connected-accounts/{id}/outbound-chat/messages` | Send one chat message (optionally a reply) as this account. Body `{message, replyParentMessageId?}`, capped at 8 KiB. Response `{sent, providerMessageId, sentAt}` — never echoes the message text. |
| `GET` | `/api/operator-chat/status` | Operator-chat projection status: schema version, buffer capacity, retained count, oldest/newest sequence, active subscribers, and a one-way "bus gap ever detected" flag. No message content. |
| `GET` | `/api/operator-chat/items` | A bounded snapshot of retained operator-chat items. Query params `after`/`limit` (capped at 1000), repeatable `accountId`, comma-separated `kinds`, `includeDeleted`; reports `gap: true` when `after` is no longer retrievable. |
| `GET` | `/api/operator-chat/stream` | Server-Sent Events: live operator-chat items (each a complete current-state upsert) as they change. Supports `Last-Event-ID` (or `?after=`) for replay, the same `accountId`/`kinds`/`includeDeleted` filters, emits `operator-chat.gap` when replay is incomplete, and periodic keepalive comments. Bounded concurrent clients. |
| `GET` | `/api/operator-chat/preferences` | Persisted display preferences, or the documented defaults if never saved. |
| `PUT` | `/api/operator-chat/preferences` | Full replacement of every preference field. Unknown fields rejected. |
| `GET` | `/api/operator-chat/account-visibility` | Every connected account with an explicit visibility override. An account absent from this list is visible by default. |
| `PUT` | `/api/operator-chat/account-visibility/{id}` | Set one connected account's chat visibility. Body `{visible}`. `404` for an unknown account. |
| `GET` | `/api/operator-chat/hidden-users` | Every operator-hidden user, identified by provider user id. |
| `POST` | `/api/operator-chat/hidden-users` | Hide a user, idempotently. Body `{providerId, connectedAccountId, providerUserId, label?}`. |
| `DELETE` | `/api/operator-chat/hidden-users/{id}` | Un-hide, by the entry's own id. Responds 204; `404` if it no longer exists. |
| `GET` | `/api/operator-chat/bot-users` | Every operator-marked bot user — a separate list from hidden users. |
| `POST` | `/api/operator-chat/bot-users` | Mark a user as a bot, idempotently. Same body shape as hidden-users. |
| `DELETE` | `/api/operator-chat/bot-users/{id}` | Unmark, by the entry's own id. Responds 204; `404` if it no longer exists. |
| `GET` | `/api/chat-overlays` | Every overlay profile. |
| `POST` | `/api/chat-overlays` | Create an overlay profile with safe defaults and a fresh, unguessable public slug. Responds 200 with the created profile. |
| `GET` | `/api/chat-overlays/{id}` | One overlay profile, including its current public slug. |
| `PUT` | `/api/chat-overlays/{id}` | Full replacement of an overlay's settings. Never accepts or changes `id`, `publicSlug` or `createdAt`. Triggers a live rebuild of the running overlay. |
| `DELETE` | `/api/chat-overlays/{id}` | Delete an overlay profile; its public URL stops serving immediately. Responds 204. |
| `POST` | `/api/chat-overlays/{id}/rotate-public-slug` | Rotate the public slug. The previous URL stops resolving immediately; every other setting is untouched. |
| `GET` | `/api/chat-overlays/{id}/accounts` | The connected accounts selected for this overlay. Empty means every currently available account. |
| `PUT` | `/api/chat-overlays/{id}/accounts` | Replace the account selection. `422` on an unknown account id. |
| `GET` | `/api/chat-overlays/{id}/hidden-users` | This overlay's own hidden-user list — independent of the operator Chat page's own list. |
| `POST` | `/api/chat-overlays/{id}/hidden-users` | Hide a user on this overlay, idempotently. |
| `DELETE` | `/api/chat-overlays/{id}/hidden-users` | Un-hide, identified by `providerId`/`connectedAccountId`/`providerUserId` query parameters (this list has no synthetic per-entry id). |
| `GET` | `/api/chat-overlays/{id}/blocked-terms` | This overlay's own blocked terms. |
| `POST` | `/api/chat-overlays/{id}/blocked-terms` | Add a blocked term (`contains` or `whole_word` match mode), idempotently by normalized value. Bounded to 100 per overlay. |
| `DELETE` | `/api/chat-overlays/{id}/blocked-terms/{termId}` | Remove a blocked term by its own id. Responds 204; `404` if it no longer exists. |
| `GET` | `/api/chat-overlays/{id}/activity-types` | The activity types selected for this overlay. Empty means every type shown. |
| `PUT` | `/api/chat-overlays/{id}/activity-types` | Replace the activity-type selection. |
| `GET` | `/api/public/chat-overlays/{publicSlug}/config` | **Unauthenticated.** Public, presentation-only overlay configuration — no management id, no filter values, no blocked-term text. |
| `GET` | `/api/public/chat-overlays/{publicSlug}/items` | **Unauthenticated.** A bounded snapshot of the overlay's currently visible items, already filtered and presentation-shaped. |
| `GET` | `/api/public/chat-overlays/{publicSlug}/stream` | **Unauthenticated.** Server-Sent Events: `chat-overlay.reset`/`.upsert`/`.remove` as the overlay's visible content changes. An unknown or disabled slug still opens a normal connection and renders an empty overlay, never a hard HTTP error. Bounded concurrent clients per overlay. |
| `GET` | `/api/chat-automation/status` | A combined snapshot: every schedule's and command's runtime state (next-run, last attempt/success, skip reason, sends this hour / match and response counts), plus overall engine status. Never a message body or username. |
| `GET` | `/api/chat-automation/schedules` | Every persisted schedule with its targets, message alternatives and current runtime snapshot. |
| `POST` | `/api/chat-automation/schedules` | Create a schedule. Responds 201 with a `Location` header. `422` on an invalid name/timing/target/template/placeholder. |
| `GET` | `/api/chat-automation/schedules/{id}` | One schedule. |
| `PUT` | `/api/chat-automation/schedules/{id}` | Full replacement of a schedule's definition. Resets its runtime state (next-run, activity counters, rolling send count) cleanly. |
| `DELETE` | `/api/chat-automation/schedules/{id}` | Delete a schedule and its targets/messages. Responds 204. |
| `POST` | `/api/chat-automation/schedules/{id}/send-now` | Send this schedule's template immediately to explicitly selected (or, if omitted, every eligible) targets. Body `{accountIds?}`. One result per target; one failure never blocks another. Never a preview — this sends real messages. |
| `GET` | `/api/chat-automation/commands` | Every persisted command with its aliases, targets and current runtime snapshot. |
| `POST` | `/api/chat-automation/commands` | Create a command. Responds 201 with a `Location` header. `409` on a name/alias already used by another command. |
| `GET` | `/api/chat-automation/commands/{id}` | One command. |
| `PUT` | `/api/chat-automation/commands/{id}` | Full replacement of a command's definition. Aliases update atomically; takes effect immediately. |
| `DELETE` | `/api/chat-automation/commands/{id}` | Delete a command and its aliases/targets. Responds 204. |
| `POST` | `/api/chat-automation/preview` | Render a template locally against one account (and optional platform context). Body `{template, accountId, platformId?}`. Never sends, never persists, never contacts Twitch. |
| `GET` | `/api/alert-event-types` | The real, capability-derived table of which conditions/placeholders apply to each of the 8 supported alert event types. |
| `GET` | `/api/alert-profiles` | Every alert profile. |
| `POST` | `/api/alert-profiles` | Create a profile with safe defaults and a fresh, unguessable public slug. Responds 201 with a `Location` header. |
| `GET` | `/api/alert-profiles/{id}` | One alert profile. |
| `PUT` | `/api/alert-profiles/{id}` | Full replacement of a profile's settings. Never accepts or changes `id`, `publicSlug` or `createdAt`. |
| `DELETE` | `/api/alert-profiles/{id}` | Delete a profile; its runtime stops and its public URL stops serving immediately. Responds 204. |
| `POST` | `/api/alert-profiles/{id}/rotate-public-slug` | Rotate the public slug. The previous URL stops resolving immediately. **No request body.** |
| `GET` | `/api/alert-profiles/{id}/rules` | Every rule on this profile, plus any quantity-range overlap warnings. |
| `POST` | `/api/alert-profiles/{id}/rules` | Create a rule. Responds 201 with a `Location` header. `422` on an unsupported condition for the event type. |
| `GET` | `/api/alert-profiles/{id}/queue` | This profile's bounded management queue status: paused, current alert, queued count/capacity, a bounded list of next-queued alerts, and every counter (enqueued/played/expired/capacity-dropped/manually-skipped/synthetic). |
| `POST` | `/api/alert-profiles/{id}/queue/pause` | Freeze queue progression for this profile. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/resume` | Resume queue progression. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/skip-current` | Remove the current alert immediately; counted as manually skipped, never played. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/replay-previous` | Re-show the single most recent completed/skipped alert. Never creates a real Engagement Event. **No request body.** |
| `POST` | `/api/alert-profiles/{id}/queue/clear` | Remove every not-yet-played queued item; the current alert is untouched. **No request body.** |
| `GET` | `/api/alert-rules/{id}` | One alert rule. |
| `PUT` | `/api/alert-rules/{id}` | Full replacement of a rule's definition. Takes effect immediately, without a backend restart. |
| `DELETE` | `/api/alert-rules/{id}` | Delete a rule. Responds 204. |
| `POST` | `/api/alert-rules/{id}/test` | Create one synthetic alert through the real queue/renderer using this rule's own presentation. Optional body `{scenario?}` for an edge-case fixture. No real Twitch account or Event Bus event involved. |
| `POST` | `/api/alert-rule-preview` | Render a template locally against representative fixture data for an event type. Body `{eventType, template, language?}`. Never sends, never persists, never touches the queue. |
| `GET` | `/api/public/alert-profiles/{publicSlug}/config` | **Unauthenticated.** Public, presentation-only profile configuration — theme/position/text-alignment/language only. |
| `GET` | `/api/public/alert-profiles/{publicSlug}/stream` | **Unauthenticated.** Server-Sent Events: `alert.show`/`.hide`/`.reset`/`.paused`/`.gap` as the current alert changes. A fresh connection's first event is always a complete current-state reset — never the queue's future contents. An unknown or disabled slug still opens a normal connection, never a hard HTTP error. Bounded concurrent clients per profile. |

The `POST` runtime and branch-command endpoints take **no request body**;
sending one is a `400`. They are commands, not resources. `GET /api/health`
does not change meaning here: the backend can be perfectly healthy while
FFmpeg is missing or a branch is in `error`.

Example — the runtime snapshot:

```bash
curl http://127.0.0.1:8080/api/runtime
```

```json
{
  "version": 1,
  "mediaMtx": {
    "supportedVersion": "v1.19.3",
    "installedVersion": "v1.19.3",
    "source": "managed",
    "state": "ready",
    "autoStart": true,
    "autoRestart": true,
    "startedAt": "2026-08-03T16:19:05Z",
    "restartCount": 0,
    "lastError": null
  },
  "ingest": {
    "state": "waiting",
    "path": "live",
    "trackCount": null,
    "tracks": []
  },
  "connection": {
    "serverUrl": "rtmp://127.0.0.1:1935",
    "streamKey": "live",
    "publishUrl": "rtmp://127.0.0.1:1935/live"
  }
}
```

**Runtime state lives only in memory.** It is never written to SQLite and resets
when the backend restarts — it describes what is happening now, not what you
configured. `restartCount` returning to zero after a restart is that working as
intended.

Example — listing configured destinations:

```bash
curl http://127.0.0.1:8080/api/platforms
```

Every error uses one envelope:

```json
{ "error": "not_found", "message": "The requested resource does not exist." }
```

Validation failures add a per-field map plus stable rule identifiers the
frontend localizes:

```json
{
  "error": "validation_failed",
  "message": "Validation failed",
  "fields": { "title": "Title cannot exceed 140 characters." },
  "details": { "title": { "rule": "too_long", "params": { "max": 140 } } }
}
```

Status codes: `400` malformed JSON or an unknown field, `404` missing record,
`405` unsupported method (with `Allow`), `409` conflict, `413` body over 64 KiB,
`415` wrong content type, `422` validation failure, `429` a provider's rate
limit was reached (Twitch endpoints only), `500` internal failure, `502` the
provider could not be reached. No endpoint ever forwards a raw Twitch error
body — every provider-facing failure is mapped to this same stable envelope.

**Provider definitions return semantic identifiers, never translated text.** The
backend sends `public`, `ultra-low`, `topic`; the frontend maps those to English
or Polish. The backend never decides the interface language.

**No endpoint returns a stored stream key, token or credential value - not
even the three credential endpoints above.** `PUT .../stream-key` accepts one
to store, but every credential response, including that one, carries only
`configured`/`available` booleans. Every other endpoint neither accepts nor
returns a credential at all; unknown JSON fields are rejected rather than
silently ignored, so a stray credential field on a non-credential endpoint
produces an error instead of disappearing quietly. New stable error codes for
the credential endpoints: `platform_not_found`, `credential_not_found`,
`credential_store_unavailable` (503), `credential_store_failure` (500) - see
"Stream key security" below.

**Stable error codes for the outbound-chat endpoints:**
`outbound_chat_unsupported` (503, non-Twitch account), `account_not_found`
(404, reusing the same code every other per-account endpoint already
uses), `outbound_chat_permission_required` (422), `account_reconnect_required`
(409, a second consecutive `401` from Twitch), `outbound_chat_queue_full`
(429), `outbound_chat_rate_limited` (429, with a sanitized retry time in
the status endpoint), `outbound_chat_forbidden` (403), `outbound_chat_message_dropped`
(422, Twitch accepted the request but did not deliver the message - its
own drop-reason prose is never included), `outbound_chat_delivery_unknown`
(502, a transport failure with no trustworthy result), `outbound_chat_provider_failure`
(502, a Twitch 5xx), and `outbound_chat_cancelled` (503). None of these
responses, nor the send endpoint's own success response, ever echoes the
sent message text.

**Stable error codes for the chat-automation endpoints:**
`chat_automation_not_found` (404, an unknown schedule or command id),
`chat_automation_account_not_found` (404), `chat_automation_target_required`
(422, a schedule or command saved with no targets), `chat_automation_target_invalid`
(422, an unknown, provider-mismatched or unlinked platform context),
`chat_automation_command_conflict` (409, a command name or alias already
used elsewhere), `chat_automation_invalid` (422, a general validation
failure - name/timing/message-count/cooldown bounds), `chat_automation_placeholder_invalid`
(422, an unknown or malformed `{placeholder}`), `chat_automation_provider_unsupported`
(503, a non-Twitch target), `chat_automation_permission_required` (422,
outbound-chat permission not yet granted for a target account),
`chat_automation_queue_full` (429, the automation quota on the shared
outbound queue is exhausted), `chat_automation_rate_limited` (429),
and `account_reconnect_required` (409, reusing the same code the
outbound-chat endpoints already use for a second consecutive `401`).
None of these responses ever echoes a template, a rendered message, or
a triggering username. A scheduled skip (waiting for the stream, waiting
for chat activity, an unresolved placeholder, an over-length render) is
not an HTTP error at all — it only ever shows up as a `lastSkipReason`
in the status/schedule snapshot, since no HTTP request exists at the
moment a timer decides to skip.

**Stable error codes for the alert endpoints:**
`alert_profile_not_found` (404), `alert_profile_disabled` (409, an action
on a disabled profile's queue), `alert_profile_invalid` (422),
`alert_rule_not_found` (404), `alert_rule_account_not_found` (404, an
unknown account in a rule's filter), `alert_rule_threshold_invalid` (422,
minimum exceeds maximum, or a negative bound), `alert_rule_condition_unsupported`
(422, a condition the event type's own capability does not support — a
quantity threshold on a follow rule, for instance), `alert_template_invalid`
(422, an unknown or malformed `{placeholder}`), `alert_queue_empty` (409,
Skip Current/Replay Previous with nothing to act on), `alert_queue_full`
(429, the profile's queue is at capacity and the new candidate is not a
strictly higher priority than the worst queued item). None of these
responses ever echoes a rendered alert's own text or username.

---

## Production build

### Frontend

```bash
cd apps/web
npm run build
```

The result lands in `apps/web/dist/`. The build runs type checking first, so a
type error stops it.

Previewing the built version:

```bash
npm run preview
```

### Backend

```bash
cd apps/server
go build ./...
```

---

## Lint, typecheck, tests and other checks

Automated checks can and should be run while working. Manual interface testing
is the final stage — see `docs/project-overview.md`, section 14.

**Frontend** (from `apps/web`):

```bash
npm run i18n:check  # translation resource consistency
npm run typecheck   # TypeScript type checking (tsc -b)
npm run lint        # ESLint
npm run test        # unit tests (Vitest), plus a set of rendered-component tests (React Testing Library) covering the Twitch device-flow and YouTube OAuth modals, disconnect/publish confirmations, the Engagement page/connector card/event feed, the Chat page/message/activity/moderation rows, the outbound-chat composer, the chat-overlay renderer/Overlays management page, and the Automation page's schedule/command lists, editors, Send Now confirmation and preview
npm run build       # production build
```

**Backend** (from `apps/server`):

```bash
go build ./...      # compilation
go vet ./...        # static analysis
go test ./...       # tests
gofmt -l .          # lists files needing formatting (empty output = all good)
```

Backend tests always create their own temporary database in the test's temp
directory, so running them never touches your real one.

**Integration checks** (from the repository root):

```bash
node scripts/verify-persistence.mjs               # SQLite survives a backend restart
node scripts/verify-mediamtx-runtime.mjs          # real MediaMTX install and supervision
node scripts/verify-ffmpeg-branches.mjs           # real FFmpeg + MediaMTX destination branches
node scripts/verify-twitch-account-integration.mjs # Twitch device flow, linking, publish - fake Twitch only
node scripts/verify-youtube-account-integration.mjs # YouTube PKCE flow, linking, broadcast/category, publish - fake Google only
node scripts/verify-twitch-engagement.mjs         # Event Bus + EventSub connector - fake Twitch only
node scripts/verify-operator-chat.mjs             # unified operator chat: projection, preferences, badges/emotes - fake Twitch only
node scripts/verify-chat-overlay.mjs              # OBS Browser Source chat overlay: profiles, public projection, public API - fake Twitch only
node scripts/verify-twitch-outbound-chat.mjs      # manual outbound chat: capability, dispatcher, sending/replies - fake Twitch only
node scripts/verify-chat-automation.mjs           # scheduled messages + chat commands: persistence, gating, placeholders, self-loop protection - fake Twitch only
node scripts/verify-alerts.mjs                    # alert rules/queue: matching, priority, expiration, pause/skip/replay/clear - fake Twitch only
node scripts/verify-alert-advanced-queue.mjs      # Stage 12B grouping and mid-alert preemption - fake Twitch only
node scripts/verify-alert-designer.mjs            # Stage 13A alert visual-design HTTP API and public rendering - fake Twitch only
node scripts/verify-chat-overlay-designer.mjs     # Stage 13B chat overlay visual-design HTTP API and public rendering - fake Twitch only
node scripts/verify-visual-templates.mjs          # Stage 14A visual-template library: built-ins, compatibility, JSON import/export - no fake servers needed
node scripts/verify-visual-template-packages.mjs  # Stage 14B managed assets and portable .streaming-tree-template packages - no fake servers needed
node scripts/verify-youtube-engagement.mjs        # Stage 15A YouTube Live Chat connector, alerts, outbound chat, chat automation - fake Google/YouTube only
node scripts/verify-streamelements-donations.mjs  # Stage 16A StreamElements donations: Astro connector, money, moderation, alerts, operator chat - fake Astro WebSocket only
node scripts/verify-tts-audio.mjs                 # Stage 17A shared audio runtime and TTS: queue, filtering, playback lifecycle, public audio route - fake TTS provider + fake Astro WebSocket only
node scripts/verify-alert-audio.mjs               # Stage 17B persistent alert sound/TTS: managed audio assets, rule-owned playback/arbitration/bounded hold, package v2 audio - fake TTS provider only
node scripts/verify-goals-widgets.mjs             # Stage 18A persistent goals/counters: accumulation, dedupe, baseline management, public goal widgets - fake Twitch/YouTube/StreamElements
node scripts/verify-supporter-widgets.mjs         # Stage 18B supporter/activity widgets: latest/largest/recent/ticker/counters, dashboards, runtime-only privacy - fake Twitch/YouTube/StreamElements
node scripts/verify-packaged-app.mjs              # Stage 20A packaged production runtime: routing, legal routes, single-instance, graceful shutdown - real release build, no fake servers
node scripts/verify-updater.mjs                   # Stage 20B application updater: real Inno Setup install/upgrade/restart cycle, manifest and hash-mismatch rejection - fake GitHub API only, real installers
```

A separate helper, `scripts/verify-installer.mjs`, smoke-tests the real
Inno Setup installer's silent install/uninstall cycle - see
[windows-packaging.md](docs/windows-packaging.md) §23. It requires a full
release build (`scripts/build-release.ps1`, installer included) and is not
part of the numbered integration-script sequence above.

The persistence script starts the backend against a temporary database,
exercises the whole platform API, restarts the process against the same file and
verifies the data survived.

The MediaMTX script uses a temporary data directory and dynamically chosen
loopback ports. It downloads and checksum-verifies the **real** v1.19.3 binary
through the application's own installation endpoint, waits for readiness, checks
the ingest state, stops and starts the service, restarts the backend and
confirms the binary is reused rather than downloaded again. It takes a few
minutes on the first run and needs network access.

The FFmpeg-branches script needs a **real, compatible FFmpeg on `PATH`**
(or pointed to by `STREAMING_TREE_FFMPEG_PATH`) — it never installs one
itself, and stops with a clear message naming the missing prerequisite
instead of claiming success if none is found. It builds a special
`-tags integration` backend binary whose only difference from the real one
is an in-memory fake credential store (see
`apps/server/cmd/testserver/main.go`), so no fake key it uses can ever reach
your real OS keychain, and that build tag makes the swap impossible to
select by accident in a normal build. Everything else — a synthetic
publisher, the real managed MediaMTX as the local ingest, real independent
branch FFmpeg processes, and two more real MediaMTX instances standing in
for destination platforms — runs entirely on loopback with dynamically
chosen ports. It takes roughly a minute (the restart-limit scenario walks
through real exponential backoff).

The Twitch-account-integration script builds the same `-tags integration`
binary and runs two small in-process fake HTTP servers that reproduce only
the Twitch OAuth (`/device`, `/token`, `/validate`, `/revoke`) and Helix
(`/users`, `/channels`, `/search/categories`) response shapes this
application parses. **It never contacts real Twitch, and no real Twitch
account is ever used or required to run it.** It covers Client ID
configuration, a full device-code authorization, account finalization,
linking, category search, metadata publish (asserting only the verified
fields ever reach the fake server), a forced token expiry and its single
transparent refresh-and-retry, reconnecting, and disconnect/revocation —
finishing with a scan of every captured backend response and log line for
every token the run issued.

The YouTube-account-integration script follows the identical shape
against fake Google OAuth and YouTube Data API servers instead, including
a wrong-CSRF-state callback and explicit multi-channel selection. See
[Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata)
for the full list of what it covers.

The Twitch-engagement script adds a third fake: a small, hand-rolled
Twitch EventSub WebSocket server (this project added no new dependency
to get one — see the script's own header comment). It covers the
identity-bound permission-upgrade scope union, exact subscription
creation, event normalization and deduplication, Twitch's official
`session_reconnect` handoff, an ordinary disconnect's data-gap handling,
revocation, restart, disable and disconnect. See
[Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents)
for the full list of what it covers and what is instead covered by Go
unit tests.

The operator-chat script reuses the same fake OAuth/Helix/EventSub
servers, extended with fake `GET /chat/badges/global` and
`GET /chat/badges` routes. It covers badge channel-then-global
resolution with cache-hit counts, the emote CDN URL, an exact deletion
updating the same item id, a per-user clear and a whole-chat clear each
correctly scoped, every activity type (gift batch vs. recipient, bits
never a donation), preferences/hidden-user/bot-user persistence
surviving a real backend restart while chat content itself resets, and
the SSE stream. See
[Unified operator chat](#unified-operator-chat) for the full list of
what it covers and what is instead covered by Go unit tests.

The chat-overlay script reuses the same fake OAuth/Helix/EventSub
servers again and drives chat through the exact same path operator chat
itself uses, layering public-overlay-specific assertions on top: safe
defaults, a live message reaching a filtered public overlay, every
filter (accounts, hidden users, bots, commands, blocked terms, activity
types), `maxVisibleItems` eviction, deletion/clear scoping, two
independent overlays never sharing state, slug rotation, restart
behavior, and a final scan for leaked chat text, blocked-term values,
hidden-user data, the public slug, or access tokens. See
[OBS Browser Source chat overlay](#obs-browser-source-chat-overlay) for
the full list of what it covers and what is instead covered by Go unit
tests.

The outbound-chat script reuses the same fake OAuth/Helix/EventSub
servers once more, extended with a `POST /chat/messages` fake (switched
between success, dropped, `401`/`403`/`422`/`429`/5xx, and a
transport-destroyed "uncertain" response by the step under test) and a
`refresh_token` grant on the fake OAuth server. It covers the exact
scope union (including the permanent absence of `user:bot`/
`channel:bot`), an identity-mismatched upgrade rejected, a successful
upgrade persisting, a send using the account's own provider user id for
both broadcaster and sender with no `for_source_only`/`pin` ever sent,
reply-parent forwarding, `is_sent:false` surfaced as a stable dropped
error, a single `401` refreshed and retried exactly once, a second
`401` stopping and recovering, every other error status mapped and
never auto-retried, two accounts with fully isolated queues and rate
limits, and a sent message's real EventSub echo appearing exactly once
with no optimistic duplicate. See
[Sending Twitch chat manually](#sending-twitch-chat-manually) for the
full list of what it covers.

The chat-automation script reuses the same fake OAuth/Helix/EventSub
servers and the real outbound-chat dispatcher underneath. It covers a
schedule and a command (with aliases) persisting, preview resolving
`{channelName}`/`{platform}`/`{channelUrl}` and rejecting an unknown
placeholder, a scheduled send waiting while local ingest is not
receiving and never sending a backlog once it starts, the
minimum-chat-activity gate blocking until enough real messages arrive
and resetting after a successful send, Send Now with a per-target
result, a command matching its canonical name and an alias while
ignoring extra arguments and a mid-message `!`, the self-message rule
preventing a reply loop, role and cooldown gating, disabling a
schedule or command stopping it immediately, and a backend restart
preserving definitions while resetting all runtime state with no
missed-run catch-up. A representative subset of the full scenario list
in the task's own specification is covered here; every scenario this
script does not itself exercise (jitter bounds, exhaustive per-role
matching, per-user cooldown independence under concurrency, and the
full HTTP status-code mapping) is instead covered by named Go tests in
`internal/chatautomation` and `internal/httpapi` — see
[`docs/progress.md`](docs/progress.md) for the exact mapping. See
[Scheduled messages and chat commands](#scheduled-messages-and-chat-commands)
for the full list of what it covers.

**None of these scripts touch your real database, your managed MediaMTX
installation, your real OS credential store, or a real Twitch/Google
account**, and all remove their temporary directories afterwards.

---

## Interface languages

The interface is available in **English** and **Polish**.

**English is the source and fallback language.** Every string is written in
English first; Polish is a translation of it. If a Polish entry were ever
missing, the interface falls back to English — a user never sees a raw
translation key.

The project uses [i18next](https://www.i18next.com/) with static, version
-controlled resource files. **No online translation API, browser translation
service, AI translation service or runtime automatic translation is used
anywhere.**

### Switching the language

The language switcher sits in the top bar on every page, and also under
**Settings → Interface language**. Switching applies immediately, without
reloading the page, and updates the `<html lang>` attribute.

### How the choice is stored

The selected language is saved in `localStorage` under the key
**`streaming-tree.language`**, and that is the only value the application stores
in the browser. It is validated on every read: an unsupported or corrupted value
falls back to English. On first launch the interface is always English — the
browser language is deliberately not detected.

Stream keys, tokens, credentials, platform configuration and stream metadata are
**never** stored in `localStorage`.

### Translation directory structure

```
apps/web/src/i18n/
├── config.ts                 # languages, namespaces, locales, storage key
├── types.ts                  # SupportedLanguage type and guards
├── index.ts                  # i18next instance and changeAppLanguage()
├── language-storage.ts       # reading/writing the preference
├── document-language.ts      # <html lang> synchronization
├── use-language.ts           # useLanguage() hook
├── i18next.d.ts              # compile-time key checking
└── resources/
    ├── en/                   # canonical source language
    │   ├── common.json       # shared labels, demo badge, units, durations
    │   ├── navigation.json   # sidebar, menu, OBS panel, version footer
    │   ├── dashboard.json    # dashboard, system status, backend, resources
    │   ├── platforms.json    # platform cards, statuses, quality, field options
    │   ├── metadata.json     # metadata editor, form labels, validation
    │   ├── pages.json        # page titles and planned-feature descriptions
    │   ├── errors.json       # backend error messages and code mappings
    │   ├── runtime.json      # MediaMTX/ingest state, dependency status
    │   ├── accounts.json     # Twitch integration, device flow, account link, publish
    │   ├── engagement.json   # Event Bus status, connector state, diagnostic event feed
    │   ├── chat.json         # unified operator chat, manual sending/replying and outbound-chat status
    │   ├── overlays.json     # OBS Browser Source overlay management and live preview
    │   └── automation.json   # scheduled messages, chat commands, placeholders, runtime status
    └── pl/                   # Polish translation, same structure
```

### Adding a new translation key

1. Add the key to the appropriate namespace in `resources/en/`. English defines
   the canonical structure.
2. Add the same key to `resources/pl/`.
3. Use it in a component: `const { t } = useTranslation('dashboard');` then
   `t('backend.heading')`.
4. Run `npm run i18n:check` and `npm run typecheck`.

Keys are type-checked against the English bundle, so a typo or a key removed
from English becomes a compile error rather than text rendered as its own key.

For countable values use pluralization instead of composing a sentence:

```jsonc
// en
"live_one": "{{count}} active stream",
"live_other": "{{count}} active streams"
```

```jsonc
// pl — Polish needs the full CLDR set
"live_one": "{{count}} aktywna transmisja",
"live_few": "{{count}} aktywne transmisje",
"live_many": "{{count}} aktywnych transmisji",
"live_other": "{{count}} aktywnej transmisji"
```

Never build a sentence by concatenating translated fragments — use one complete
entry with interpolation.

### Adding a future language

1. Create `apps/web/src/i18n/resources/<code>/` and copy the English namespace
   files into it.
2. Translate the values, using the plural categories that language requires
   (`npm run i18n:check` reports which ones are missing).
3. Register the language in `apps/web/src/i18n/config.ts`: add the code to
   `SUPPORTED_LANGUAGES`, its endonym to `LANGUAGE_LABELS` and its BCP 47 tag to
   `LANGUAGE_LOCALES`.
4. Add the imports to `apps/web/src/i18n/resources.ts`.
5. Run `npm run i18n:check`, `npm run typecheck` and `npm run test`.

The switcher picks up the new language automatically — it is rendered from
`SUPPORTED_LANGUAGES`.

### Checking translation consistency

```bash
cd apps/web
npm run i18n:check
```

English defines the canonical key structure. The script reports, with the full
path of each problem and a non-zero exit code:

- a key present in English but missing in another language,
- a key present in another language but missing in English,
- incompatible structures (an object where a string is expected, or vice versa),
- empty or whitespace-only values,
- missing or unexpected plural forms for that language.

It is plural-aware: Polish `_few` and `_many` entries are not reported as
mismatches against English `_one` and `_other`.

### What is not translated

- **User-created stream metadata** — titles, descriptions, tags and display
  names you type in are stored and shown verbatim, and are never translated
  automatically.
- **Platform brand names** — Twitch, YouTube, Kick, TikTok.
- **URLs and the RTMP address.**
- **API identifiers** — service name, version, backend error codes.
- **Stream language names** — shown as endonyms ("English", "Polski") the way
  the platforms themselves present them.

### Secrets and translation resources

Translation files are ordinary, version-controlled source files. **Stream keys,
tokens and any other secrets must never be placed in them**, exactly as with the
rest of the repository.

---

## Directory structure

```
.
├── apps/
│   ├── web/                    # Operator panel (React + TypeScript + Vite)
│   │   ├── scripts/            # check-i18n.mjs — translation consistency check
│   │   ├── src/
│   │   │   ├── api/            # Zod contracts + transport for the platform, account, engagement, operator-chat, chat-overlay, outbound-chat and chat-automation API
│   │   │   ├── app/            # TanStack Query configuration
│   │   │   ├── components/
│   │   │   │   ├── automation/ # Automation page panels: schedule/command lists, editors, Send Now confirmation, placeholder helper, preview (Stage 11B)
│   │   │   │   ├── chat/       # Message/activity/moderation rows, filter bar, settings panel, badge/emote images, the outbound-chat composer (Stage 11A)
│   │   │   │   ├── chat-overlay/ # The public overlay renderer tree (Stage 10) - shared by the public route and the Overlays preview panel
│   │   │   │   ├── engagement/ # Twitch connector card, bounded recent-events feed
│   │   │   │   ├── layout/     # Shell: sidebar, top bar
│   │   │   │   ├── metadata/   # Metadata editor with platform tabs, Twitch/YouTube category pickers, publish panel
│   │   │   │   ├── overlays/   # Overlays management page panels: list, editor, URL, settings, accounts, hidden users, blocked terms, activity types, setup, preview (Stage 10)
│   │   │   │   ├── platforms/  # Destination cards, add/settings dialogs, output settings, branch controls, account link, broadcast selection
│   │   │   │   ├── runtime/    # Ingest controls, install dialog, copy widget, bulk-start confirmation
│   │   │   │   ├── settings/   # Connected Accounts panel, Twitch device-flow modal, YouTube accounts panel and OAuth modal
│   │   │   │   ├── system/     # System and backend status panels
│   │   │   │   └── ui/         # Base elements (buttons, inputs, panels, modal)
│   │   │   ├── data/           # DEMO DATA (host metrics only)
│   │   │   ├── hooks/          # Queries, mutations, cache helpers, the engagement, operator-chat and chat-overlay SSE client hooks, outbound-chat status/send/authorize hooks, chat-automation status/schedule/command/preview hooks
│   │   │   ├── i18n/           # Localization: config, resources, tests
│   │   │   ├── lib/            # API client, error mapping, helpers
│   │   │   ├── models/         # UI types, validation, identifier/state-to-label mappings, the operator-chat and chat-overlay reducers, autoscroll state machine, overlay preview fixtures, chat-automation placeholder/bounds helpers
│   │   │   ├── pages/          # Route views, including EngagementPage, ChatPage, OverlaysPage, AutomationPage and the public OverlayChatPage (no application shell)
│   │   │   └── test/           # Rendered-component test harness (Testing Library provider wrapper)
│   │   └── ...                 # Vite, TypeScript, ESLint, Vitest configuration
│   │
│   └── server/                 # Backend (Go)
│       ├── cmd/server/         # Entry point, graceful shutdown
│       ├── cmd/testserver/     # `-tags integration` twin for the real-FFmpeg and fake-provider smoke tests only
│       └── internal/
│           ├── buildinfo/      # Service name and version
│           ├── config/         # Configuration and database path resolution
│           ├── domain/account/ # Connected-account model, token bundle, service (provider-independent)
│           ├── domain/engagement/       # Normalized engagement-event model (Stage 8A)
│           ├── domain/engagementsettings/ # Per-account engagement-connector enable/disable preference
│           ├── domain/operatorchatprefs/ # Persisted operator-chat preferences, account visibility, hidden/bot-user lists (Stage 9)
│           ├── domain/chatoverlay/ # Persisted chat-overlay profiles: settings, accounts, hidden users, blocked terms, activity types (Stage 10)
│           ├── domain/chatautomation/ # Persisted schedule and command definitions: targets, messages/aliases, validation (Stage 11B)
│           ├── domain/platform/# Provider registry, models, validation, service
│           ├── domain/credential/# Destination stream-key service (OS credential store)
│           ├── domain/output/  # Destination output-settings model, validation, service
│           ├── domain/remotetarget/ # Remote broadcast/target association (YouTube)
│           ├── engagement/     # The Engagement Event Bus (ring buffer, dedup, subscriptions)
│           ├── operatorchat/   # The unified operator-chat projection (Stage 9) - provider-independent, in-memory only
│           ├── chatoverlay/    # The public per-overlay chat projection (Stage 10) - consumes operatorchat's own revision stream, not the Event Bus directly
│           ├── outboundchat/   # Provider-independent send model, validation, in-memory per-account dispatcher (Stage 11A) - never imports the Twitch provider package
│           ├── chatautomation/ # Scheduler + command engine runtime, placeholders, dispatch quota, wiring (Stage 11B) - only ever calls outboundchat, never the Twitch client directly
│           ├── httpapi/        # Router, handlers, middleware, JSON responses
│           ├── provider/twitch/# Twitch OAuth + Helix + EventSub client, adapter, metadata/engagement services, the Send Chat Message adapter (Stage 11A)
│           │   └── chatassets/ # Twitch chat badge (cached) and emote (pure URL) resolution (Stage 9)
│           ├── provider/youtube/# YouTube OAuth (PKCE) + Data API client, adapter, metadata service
│           ├── runtime/deviceflow/# Device-authorization attempt state machine
│           ├── runtime/youtubeauth/# YouTube Authorization Code + PKCE loopback-callback attempt manager
│           ├── runtime/twitchengagement/ # Per-account Twitch EventSub WebSocket connector supervisor
│           ├── runtime/mediamtx/# Resolver, installer, config, supervisor, API client
│           ├── runtime/ffmpeg/ # Executable resolver and capability probing
│           ├── runtime/branch/ # Per-destination branch supervisor (state machine, restart policy)
│           └── storage/sqlite/ # Connection, migrations, repository
│               └── migrations/ # Embedded .sql schema and seed
│
├── config/                     # No FFmpeg/MediaMTX templates live here - see config/README.md
├── docs/
│   ├── project-overview.md     # Full project description
│   ├── engagement-architecture.md # Engagement platform architecture (operator chat implemented as of stage 9, the OBS chat overlay as of stage 10, manual outbound chat as of stage 11A, scheduled messages and chat commands as of stage 11B)
│   ├── obs-browser-source.md   # Researched OBS Browser Source contract and Stage 10 recommendations
│   ├── audio-tts.md            # Researched Windows SAPI TTS contract, the shared audio runtime/queue design, and the public audio overlay protocol (Stage 17A)
│   ├── alert-audio.md          # Persistent alert sound assets, per-alert-rule TTS, synchronization, and package v2 audio (Stage 17B)
│   ├── goals-widgets.md        # Persistent goals/counters foundation and public OBS goal widgets (Stage 18A)
│   ├── supporter-widgets.md    # Supporter/activity widgets, richer session counters, and bounded dashboards (Stage 18B)
│   ├── provider-integrations/
│   │   ├── twitch.md           # Researched Twitch metadata API contract: flow, scopes, capabilities, limits
│   │   ├── twitch-engagement.md # Researched Twitch EventSub WebSocket contract (Stage 8A) + chat badge/emote contract (Stage 9)
│   │   ├── twitch-outbound-chat.md # Researched Twitch Send Chat Message API contract (Stage 11A) + the Stage 11B automation layer built on top of it
│   │   ├── youtube.md          # Researched Google/YouTube API contract
│   │   ├── youtube-engagement.md # Researched YouTube Live Chat streamList gRPC contract (Stage 15A)
│   │   ├── kick-engagement.md  # Researched Kick engagement feasibility (Stage 15B) - webhook-only, deferred
│   │   └── external-donations.md # Researched StreamElements/Streamlabs/Ko-fi donation feasibility + the real StreamElements Astro contract (Stage 16A)
│   └── progress.md             # Work journal
├── scripts/
│   ├── verify-persistence.mjs      # Scripted restart-persistence check
│   ├── verify-mediamtx-runtime.mjs # Real MediaMTX install and supervision check
│   ├── verify-ffmpeg-branches.mjs  # Real FFmpeg + MediaMTX destination-branch check
│   ├── verify-twitch-account-integration.mjs # Twitch device flow, linking, publish - fake Twitch only
│   ├── verify-youtube-account-integration.mjs # YouTube PKCE flow, linking, publish - fake Google only
│   ├── verify-twitch-engagement.mjs # Event Bus + EventSub connector - fake Twitch only
│   ├── verify-operator-chat.mjs    # Unified operator chat: projection, preferences, badges/emotes - fake Twitch only
│   ├── verify-chat-overlay.mjs     # OBS Browser Source chat overlay: profiles, public projection, public API - fake Twitch only
│   ├── verify-twitch-outbound-chat.mjs # Manual outbound chat: capability, dispatcher, sending/replies - fake Twitch only
│   ├── verify-chat-automation.mjs  # Scheduled messages + chat commands: persistence, gating, placeholders, self-loop protection - fake Twitch only
│   ├── verify-alerts.mjs           # Alert rules/queue: matching, priority, expiration, pause/skip/replay/clear - fake Twitch only
│   ├── verify-alert-advanced-queue.mjs # Stage 12B grouping and mid-alert preemption - fake Twitch only
│   ├── verify-alert-designer.mjs   # Stage 13A alert visual-design HTTP API and public rendering - fake Twitch only
│   ├── verify-chat-overlay-designer.mjs # Stage 13B chat overlay visual-design HTTP API and public rendering - fake Twitch only
│   ├── verify-visual-templates.mjs # Stage 14A visual-template library: built-ins, compatibility, JSON import/export - no fake servers needed
│   ├── verify-visual-template-packages.mjs # Stage 14B managed assets and portable .streaming-tree-template packages - no fake servers needed
│   ├── verify-youtube-engagement.mjs # Stage 15A YouTube Live Chat connector, alerts, outbound chat, chat automation - fake Google/YouTube only
│   ├── verify-streamelements-donations.mjs # Stage 16A StreamElements donations: Astro connector, money, moderation, alerts, operator chat - fake Astro WebSocket only
│   ├── verify-tts-audio.mjs        # Stage 17A shared audio runtime and TTS: queue, filtering, playback lifecycle, public audio route - fake TTS provider + fake Astro WebSocket only
│   ├── verify-alert-audio.mjs      # Stage 17B persistent alert sound/TTS: managed audio assets, rule-owned playback/arbitration/bounded hold, package v2 audio - fake TTS provider only
│   ├── verify-goals-widgets.mjs    # Stage 18A persistent goals/counters: accumulation, dedupe, baseline management, public goal widgets - fake Twitch/YouTube/StreamElements
│   └── verify-supporter-widgets.mjs # Stage 18B supporter/activity widgets: latest/largest/recent/ticker/counters, dashboards, runtime-only privacy - fake Twitch/YouTube/StreamElements
├── .gitignore
├── THIRD_PARTY_NOTICES.md      # MediaMTX, FFmpeg and other third-party dependencies
└── README.md
```

---

## What is currently demo-only

Every item below is marked with a **Demo** badge in the interface, or described
directly next to the control.

| Element | What actually happens |
| ------- | --------------------- |
| Per-destination viewer counts, connection quality, "Authenticated"/"Verified by platform" status | **Not shown anywhere.** Streaming Tree never contacts a platform to confirm a stream is live there; the interface only ever reports what FFmpeg itself reported (real progress fields) or a plain "Sending" / "Output active" wording. |
| CPU, memory, disk, network | Fixed demo values, clearly badged. The backend does not collect host metrics. |
| Platform capability tables | Twitch's and YouTube's tables are now verified against their real APIs — see [`docs/provider-integrations/twitch.md`](docs/provider-integrations/twitch.md) and [`docs/provider-integrations/youtube.md`](docs/provider-integrations/youtube.md). Kick and TikTok remain an approximate configuration, **not** verified against their real APIs, and need re-checking when their own account integration is implemented (stage 7C). |
| Kick and TikTok account connection and metadata publishing | **Not implemented.** Only Twitch and YouTube have a real provider integration at this stage; the destination-settings account section for these providers shows an honest "not implemented yet" state instead of a working selector. |
| Kick/TikTok engagement, Streamlabs/Ko-fi donations | **Not implemented anywhere.** A real, unified operator chat is implemented as of stage 9, a real, public OBS Browser Source chat overlay built on top of it as of stage 10, real *manual* outbound chat sending/replying as of stage 11A, real *scheduled messages and chat commands* as of stage 11B, a real alert engine as of stage 12A/12B, a real, shared visual-design engine with a real **Alert Overlay Designer** (stage 13A) and a real **Chat Overlay Designer** reusing that same engine (stage 13B) — Stage 13 as a whole is complete — a real, shared **visual-template library** (built-ins, a persisted user gallery, asset-free JSON import/export) reused by both Designers (stage 14A), real **portable archive template packages with managed visual assets** (images, video, custom fonts; stage 14B) — Stage 14 as a whole is now complete — a real second **YouTube inbound connector** (stage 15A: Live Chat, Super Chat, Super Sticker, membership events, all served by that exact same pipeline, over YouTube's official `streamList` gRPC transport), a real **external-donation connector** (stage 16A: a provider-independent `donationsource` domain plus a real **StreamElements** Astro WebSocket connector, exact integer-micros money, moderation-aware handling, served by that exact same pipeline), a real **shared audio runtime and text-to-speech foundation** (stage 17A: a real Windows SAPI provider, a bounded audio queue consuming that same Event Bus, and a public OBS Browser Source audio route), real **persistent alert sound assets and per-alert-rule TTS** on top of that exact same runtime (stage 17B: a managed audio-asset library, rule-owned sound/TTS with deterministic arbitration and a bounded visual hold, and a Stage 14B package manifest v2 audio extension) — Stage 17 as a whole is now complete — and a real **persistent goals/counters foundation and full supporter/activity widget suite** (stages 18A/18B: a provider-independent accumulation engine, four core goal families, operator baseline/current management, real public OBS goal widgets, and eight further widget kinds — latest follower/subscriber/donation, largest donation, a recent-supporters list, an event ticker, richer session counters, and bounded multi-widget dashboards, all runtime-only for their own event-derived content) (see [Unified operator chat](#unified-operator-chat), [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay), [Sending Twitch chat manually](#sending-twitch-chat-manually), [Scheduled messages and chat commands](#scheduled-messages-and-chat-commands), [Alerts](#alerts), [Visual Template Library (Stage 14A)](#visual-template-library-stage-14a), [Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents), [Text-to-speech and audio](#text-to-speech-and-audio), [Persistent goals and supporter widgets](#persistent-goals-and-supporter-widgets-stage-18a18b) and [`docs/provider-integrations/external-donations.md`](docs/provider-integrations/external-donations.md)). Everything built on top of that engine and not yet listed as real above (Kick/TikTok engagement, Streamlabs/Ko-fi donations) remains planned; see [`docs/engagement-architecture.md`](docs/engagement-architecture.md). |
| Platforms, Metadata, Logs pages | Informational views describing the planned scope. Not implemented. |

### What is real

- **Receiving a stream from OBS**, through a supervised MediaMTX process.
- **Installing MediaMTX**, with official checksum verification.
- **Start, Stop and Restart of the local ingest service.**
- **Live ingest detection** — waiting versus receiving, with the source type and
  detected tracks reported by MediaMTX.
- **The Server and Stream Key values**, derived from the running configuration.
- **Adding, editing and deleting destinations**, stored in SQLite.
- **Editing and saving stream metadata**, including ordered Twitch tags.
- **Storing, replacing, checking the status of and deleting a destination's
  stream key**, in the operating system credential store - see
  [Stream key security](#stream-key-security).
- **Sending a destination's stream onward with real FFmpeg** - one
  independent process per enabled destination, pulling the local ingest and
  pushing stream-copied RTMP/RTMPS to that destination's configured server,
  with real Start/Stop/Restart, real per-branch state and real progress -
  see [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg).
- **Everything stored survives a browser refresh and a backend restart** -
  including a stream key, which survives in the OS credential store
  independently of both. Per-branch *runtime* state (live/error/restart
  count) deliberately does **not** survive a backend restart - see the
  branch lifecycle section above for why.
- **Connecting a real Twitch account** via device-code sign-in, with
  no client secret ever requested or stored, account validation/refresh/
  reconnect/disconnect, and linking an account to a destination - see
  [Connected accounts and Twitch metadata](#connected-accounts-and-twitch-metadata).
- **Searching real Twitch categories and publishing real channel metadata**
  (title, category, language, tags) to Twitch, behind an explicit publish
  action separate from the existing local Save.
- **Connecting a real YouTube channel** via Authorization Code + PKCE
  sign-in through a real system browser and a temporary loopback callback,
  with no client secret ever requested or stored, explicit multi-channel
  selection, account validation/refresh/reconnect/disconnect, linking a
  channel to a destination, and selecting a live broadcast for it - see
  [Connected accounts and YouTube metadata](#connected-accounts-and-youtube-metadata).
- **Listing real YouTube broadcasts and categories and publishing real
  video metadata** (title, description, category, tags, language,
  visibility) to a selected YouTube broadcast, behind an explicit publish
  action separate from the existing local Save.
- **The language switcher**, in the top bar and under Settings.
- **A real Twitch EventSub WebSocket connector** reading chat messages,
  moderation, follows, subscriptions, gifts, cheers, incoming raids,
  channel-point redemptions and remote stream online/offline, normalized
  onto an in-memory Engagement Event Bus, with real enable/disable
  (persisted, restored automatically after a backend restart), a real
  identity-bound permission-upgrade flow, and Twitch's own official
  `session_reconnect` handoff handled without a false data-gap marker -
  see [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents).
- **A real Server-Sent Events stream** (`GET /api/engagement/stream`) with
  replay via `Last-Event-ID` and an explicit gap signal for evicted
  history, and the diagnostic Engagement page that consumes it live.
- **A real, unified operator Chat page** (`/chat`) merging live Twitch
  chat across every connected account with an enabled connector: real
  ordered fragments, resolved Twitch badges and emote images, message
  deletion and chat/user clearing reflected in place, activity events
  inline, account/kind/bot/hidden-user filtering, persisted display
  preferences, and autoscroll with a jump-to-latest control - see
  [Unified operator chat](#unified-operator-chat).
- **A real, public OBS Browser Source chat overlay** (`/overlays` to
  manage, `/overlay/chat/{publicSlug}` to view/embed) — persisted overlay
  profiles with their own filters, visual settings and an unguessable,
  rotatable public URL; a public per-overlay projection consuming the
  operator-chat projection above; a public unauthenticated HTTP + SSE
  API; and a live preview panel on the management page reusing the exact
  same renderer - see
  [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay).
- **Real manual Twitch chat sending and replying**, as the connected
  account itself, through Twitch's real Send Chat Message API - a
  third, independent per-account permission upgrade; an in-memory,
  bounded, per-account send queue with local and provider rate-limit
  handling; a Chat page composer with no optimistic local echo; and a
  Reply action locked to the message's own connected account - see
  [Sending Twitch chat manually](#sending-twitch-chat-manually).
- **Real scheduled messages and safe chat commands**, built on that
  same dispatcher - a centralized in-memory scheduler with drift-free
  interval/jitter timing, only-while-streaming and minimum-chat-
  activity gating, message-group alternatives, and a manual Send Now
  override; a command engine matching a fixed `!` prefix with aliases,
  per-role gating, global/per-user cooldowns, and a hard rule that the
  sending account's own messages can never re-trigger a command; a
  closed, declarative placeholder language shared by both; and an
  Automation page to manage them - see
  [Scheduled messages and chat commands](#scheduled-messages-and-chat-commands).
- **A real alert engine**, consuming that same Event Bus — persisted alert
  profiles and rules; a provider-independent matcher supporting all 8 real
  Twitch alert-capable event types with provider/account filters,
  capability-driven quantity thresholds and visibility toggles; a bounded
  in-memory queue with priority ordering, expiration, pause/resume, skip,
  replay and clear; local synthetic test alerts that exercise the exact
  same queue and renderer with no real Twitch account or event involved;
  and a real, public, unauthenticated OBS Browser Source alert route —
  see [Alerts](#alerts).
- **A real, shared, provider-independent visual-design engine, Alert
  Overlay Designer and Chat Overlay Designer** (stages 13A/13B, Stage 13
  now complete) — a versioned, bounded layer-tree document persisted one
  per alert rule or one per chat overlay with optimistic-concurrency
  revisions; one shared React renderer used identically by both
  Designers' own canvas and the real public alert/overlay routes;
  drag/resize/numeric editing, layer ordering, show/hide/lock,
  duplicate/delete, bounded undo/redo, zoom/fit and deterministic
  preview scenarios; every existing alert rule or chat overlay keeps
  rendering through its original fixed/legacy presentation until an
  operator explicitly opens the Designer and saves, and a chat overlay's
  own Stage 10 filtering/lifecycle/moderation stays entirely
  authoritative regardless of rendering mode — see
  [Alerts](#alerts), [OBS Browser Source chat overlay](#obs-browser-source-chat-overlay)
  and [`docs/visual-designs.md`](docs/visual-designs.md).
- **A real, shared, reusable visual-template library** (stage 14A,
  built on the same document from the bullet above) — built-in,
  immutable templates (three per target) alongside a persisted,
  operator-owned template gallery; backend-authoritative
  target/owner-instance compatibility with stable reason codes; a
  strict draft-first application model (using a template only ever
  updates the Designer's own unsaved draft - the owner's saved design
  changes only through the Designer's own pre-existing Save); "Save as
  template" from the current draft, independent of the owner's own
  save state; and closed, asset-free JSON import (backend-validated
  preview, then explicit confirm) and export, with an older exported
  document version migrated transparently - see
  [`docs/visual-templates.md`](docs/visual-templates.md).
- **A real YouTube inbound engagement connector** (stage 15A) — Live
  Chat received over YouTube's official `streamList` gRPC server-
  streaming transport (a long-lived push connection, not polling),
  reusing the connected YouTube account's existing OAuth scope with no
  separate engagement identity; ordinary chat, Super Chat, Super
  Sticker, new-member and member-milestone events all normalized onto
  the exact same Engagement Event Bus the Twitch connector uses, and
  served by the exact same unified Chat page, operator-chat activities,
  OBS chat overlay, manual/scheduled/command outbound sending, and
  alerts — Super Chat/Super Sticker carry this application's first real
  monetary value (integer micros, uppercased currency, no FX
  conversion) — see
  [Engagement Event Bus and YouTube chat/events](#engagement-event-bus-and-youtube-chatevents).
- **A real external-donation connector** (stage 16A) — a
  provider-independent `donationsource` domain and a real
  **StreamElements** Astro WebSocket connector, publishing real
  donations onto that same Event Bus with exact integer-micros money
  and full reuse of operator chat/chat overlay/alerts — see
  [`docs/provider-integrations/external-donations.md`](docs/provider-integrations/external-donations.md).
- **A real shared audio runtime and text-to-speech foundation**
  (stage 17A) — a provider-independent `Provider` abstraction with a
  real Windows SAPI implementation; a bounded audio queue consuming
  that same Event Bus with cooldowns, manual approval, per-source/
  per-currency/per-Bits filtering and text preprocessing; and a real,
  public, unauthenticated OBS Browser Source audio route — see
  [Text-to-speech and audio](#text-to-speech-and-audio).
- **Real persistent alert sound assets and per-alert-rule TTS** (stage
  17B, Stage 17 as a whole now complete) — built on that exact same
  audio runtime: a managed audio-asset library (16-bit PCM WAV only),
  rule-owned sound/TTS with deterministic arbitration against the
  global TTS queue, a bounded visual hold so an alert stays visible
  while its own audio is still playing, and a Stage 14B package
  manifest v2 extension carrying that configuration through a portable
  template package — see
  [Persistent alert audio and per-rule TTS (Stage 17B)](#persistent-alert-audio-and-per-rule-tts-stage-17b).
- **A real persistent goals/counters foundation and the full
  supporter/activity widget suite** (stages 18A/18B, Stage 18 as a
  whole now complete) — a provider-independent accumulation engine
  consuming that same Event Bus at current position; four core goal
  families (followers, subscriptions, donations, Bits) with a
  deterministic, provider-independent contribution table; operator-
  supplied baseline/current management (this application never claims
  to know a provider's own complete historical total); durable per-goal
  duplicate protection; real public OBS goal widgets; and, on top of
  that same foundation, eight further widget kinds (latest follower/
  subscriber/donation, largest donation, a recent-supporters list, an
  event ticker, richer session counters, and bounded multi-widget
  dashboards) whose own event-derived content is runtime-only, all
  sharing one generic Browser Source route — see
  [Persistent goals and supporter widgets (Stage 18A/18B)](#persistent-goals-and-supporter-widgets-stage-18a18b).

No bitrate, resolution or frame rate is displayed anywhere: the MediaMTX Control
API does not report them, so showing a number would mean inventing it.

### What will be added later

- **Kick account integration** — sign-in and metadata publishing, reusing
  the same connected-account foundation Twitch's and YouTube's
  integrations now provide - deferred, capability-gated (stage 7C).
  Kick's own engagement adapter (stage 15B) remains separately
  feasibility-gated - see
  [`docs/provider-integrations/kick-engagement.md`](docs/provider-integrations/kick-engagement.md).
- **TikTok account integration** is not pursued as an independent item -
  it is folded into stage 19's own feasibility gate, which found no
  official TikTok LIVE engagement capability for such an account to
  power - see
  [`docs/provider-integrations/tiktok-live.md`](docs/provider-integrations/tiktok-live.md).
- **Additional donation providers (Streamlabs, Ko-fi)** remain
  feasibility-gated — Streamlabs' documented OAuth token exchange
  requires a confidential client secret with no public-client
  alternative found; Ko-fi is webhook-only and needs a public inbound
  endpoint this deployment target does not offer — see
  [`docs/provider-integrations/external-donations.md`](docs/provider-integrations/external-donations.md).
  (Portable archive template packages and managed template assets
  shipped in Stage 14B; a real external-donation connector,
  StreamElements, shipped in Stage 16A; a shared audio/text-to-speech
  runtime shipped in Stage 17A; persistent alert sound assets and
  per-alert-rule TTS shipped in Stage 17B — Stage 17 as a whole is now
  complete; and the full persistent goals/counters foundation plus
  every supporter/activity widget kind shipped across Stages 18A/18B —
  Stage 18 as a whole is now complete.)
- **A log viewer** — the backend keeps a small diagnostic buffer already.

---

## Stream key security

A stream key allows broadcasting on someone's channel, so we treat it like a
password. This section describes the destination-credential foundation; how
that key is actually used to start an outgoing stream is described in
[Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg), including
its one honestly-documented limitation (command-line exposure).

- **The repository contains no secrets** and must never contain any.
  `.gitignore` blocks `.env` files, database files and data directories - and
  is anchored at the repository root everywhere it lists a directory name, so
  a guard rule can never accidentally match a source directory.
- **The SQLite database stores no credentials, and never will.** No table has
  a column for a stream key, token or password, and no API payload for
  platform or metadata endpoints carries one. Whether a credential is
  configured is derived directly from the OS credential store on every
  request, not cached in a database row that could go stale.
- **A stream key is stored in the operating system credential store**:
  Windows Credential Manager, macOS Keychain, or Linux Secret Service, via
  [`github.com/99designs/keyring`](https://github.com/99designs/keyring) -
  chosen specifically because none of its backends for these three platforms
  shell out to an external command (see `THIRD_PARTY_NOTICES.md`). **There is
  no plaintext fallback.** If the credential store cannot be reached, the API
  reports that plainly and leaves the value unstored; it never writes it to a
  file instead.
- **The key is scoped by the destination's generated ID, not its display
  name or provider.** Renaming a destination cannot orphan its key, and two
  destinations configured for the same provider always get independent keys.
- **Once saved, a key cannot be viewed again through this application.**
  There is no "show saved key" control anywhere. Replacing overwrites the
  previous value; deleting removes it and prevents outgoing streaming to that
  destination until a new key is added. Both actions are described in the
  platform settings dialog itself.
- **A "Stored" status is not a claim that the key is valid.** Streaming Tree
  never contacts a platform to verify a key, so the interface only ever says
  "Stored" or "Missing" (or that secure storage is unavailable) - never
  "Valid", "Connected" or "Authenticated".
- **Keys are not stored in the browser.** Not in `localStorage`, not in
  `sessionStorage`, not in application state beyond what a submit requires,
  and not in TanStack Query's cache - only a `{configured, available}` status
  is ever cached. The mutation that submits a new key is configured with a
  zero garbage-collection time and resets itself immediately after every
  attempt, so it does not linger in React Query Devtools either. The only
  value this application stores in the browser at all is the interface
  language preference.
- **The backend reads a key only when explicitly starting that destination's
  outgoing FFmpeg process**, never while merely checking status, and never
  logs it or includes it in a formatted error. The retrieval method for that
  is not reachable through the HTTP API at all: the interface the web panel
  talks to simply has no method that returns a value. It **is** passed to
  FFmpeg as part of a command-line argument, since no safer mechanism exists
  in FFmpeg's own CLI for this - see
  [Outgoing streaming with FFmpeg](#outgoing-streaming-with-ffmpeg) for the
  honest explanation of that one limitation and its mitigations.
- **This is unrelated to the local ingest path.** The `live` stream key OBS
  uses to publish to this machine (see [Connecting OBS](#connecting-obs)) is
  a route name on a loopback-only server, not a secret, and is never confused
  with a destination's credential anywhere in the interface.

The full architecture - key-namespace format, validation rules, the HTTP
contract, and the platform-deletion cleanup ordering - is in
[`docs/project-overview.md`](docs/project-overview.md#10-stream-key-security)
and the implementation notes in
[`docs/progress.md`](docs/progress.md).

---

## About, privacy and legal

Streaming Tree for OBS is an independent project created by **Czekosabe**
(<https://github.com/Czekosabe>). The application's own in-app **About &
Legal** page (`Settings → About & Legal`) shows product identity, the
current build/version state, the application licence, and a voluntary
creator-support link, and links to the canonical documents - served from a
packaged installation's own embedded copies (`/legal/license`,
`/legal/privacy`, `/legal/legal`, `/legal/third-party-notices`, see
[windows-packaging.md](docs/windows-packaging.md) §16), so this works fully
offline, not only from source:

- [`LICENSE`](LICENSE) - the complete, authoritative licence text.
- [`PRIVACY.md`](PRIVACY.md) - what is local application state versus
  network activity the user explicitly enables.
- [`LEGAL.md`](LEGAL.md) - a concise disclaimer, the application-licence
  section below, and third-party service terms.
- [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) - bundled/dependency
  third-party software notices.
- [`docs/product-identity-legal.md`](docs/product-identity-legal.md) - the
  canonical contract these surfaces were built from.

### Licence

Streaming Tree for OBS's own first-party application code is licensed
under the **GNU General Public License version 3 or any later version**
(`GPL-3.0-or-later`), Copyright (C) 2026 Czekosabe. The complete licence
text is [`LICENSE`](LICENSE) at the repository root; see
[`LEGAL.md`](LEGAL.md) for a plain-language summary (commercial use is
permitted; distributed modified versions remain subject to the same
licence terms). Third-party components this project bundles or depends on
keep their own licences, documented separately in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) - they are not
relicensed by Streaming Tree for OBS's own choice of licence.

---

## Common problems

**The panel shows "Backend unavailable".**
The backend is not running, or is running on a different port. Start it in a
second terminal (`cd apps/server && go run ./cmd/server`) and use the refresh
button in the "Backend" card. This is an expected, fully handled state — the
panel does not crash. Your configuration is safe: it lives in the backend
database, which is why the dashboard cannot show it while the backend is down.

**My destinations disappeared.**
The backend is probably using a different database than before. Check the
`path=` value in the startup log and whether `STREAMING_TREE_DB_PATH` or
`STREAMING_TREE_DATA_DIR` is set in that terminal.

**A seeded destination I deleted did not come back.**
That is intended. The seed runs once, on a brand-new database, and is recorded
like any other migration.

**The platform settings dialog says "Secure storage unavailable" for the
stream key.** The operating system credential store could not be reached:
common causes are a Linux session with no Secret Service running, a locked
macOS Keychain, or a permission failure. The rest of the application is
unaffected - SQLite, MediaMTX and everything else keep working - but a stream
key cannot be saved until the store becomes available. This is not polled
automatically; reopen the dialog after fixing the underlying cause to check
again.

**I deleted a destination while the credential store was unavailable - is its
stream key still out there?** Possibly. Platform deletion does not block on a
credential store it cannot reach (see "Stream key security"), so a key set
earlier, when the store was reachable, may still exist under that platform's
old ID. It is inert: the ID is never reused and nothing in this application
can look it up again. If this matters to you, use your OS credential manager
directly to remove any leftover entry under the `streaming-tree-for-obs`
service name.

**`go: command not found` or `'go' is not recognized`.**
Go is not installed or is not on `PATH`. Install it from <https://go.dev/dl/>
and open a new terminal window.

**`npm install` fails with "Cannot find native binding".**
Your Node version is older than the native dependencies require. Upgrade Node to
22.12+ or 24 LTS, delete `apps/web/node_modules` and
`apps/web/package-lock.json`, then install again.

**Port 8080 or 5173 is already in use.**
Backend: start it with a different `STREAMING_TREE_PORT` and add the new address
to `VITE_DEV_API_PROXY_TARGET` in `apps/web/.env.local`.
Frontend: Vite will offer the next free port; remember to add the new origin to
`STREAMING_TREE_ALLOWED_ORIGINS`.

**Interface changes are not visible.**
Check that `npm run dev` is still running and that there are no errors in the
browser console. If needed, reload the page bypassing the cache
(`Ctrl + Shift + R`).

**A label shows in English while the interface is set to Polish.**
That is the fallback working: the Polish entry is missing. Run
`npm run i18n:check` — it prints the exact path of every missing key.

### MediaMTX and OBS

**"MediaMTX is not installed yet."**
Expected on a fresh setup. Use the **Install MediaMTX** button in the sidebar or
on the **Streams** page. Nothing is downloaded until you confirm.

**"The MediaMTX binary found is not the supported version."**
Only v1.19.3 is supported, and an unsupported build is never started because the
generated configuration targets that exact schema. Either remove
`STREAMING_TREE_MEDIAMTX_PATH` and use the managed installation, or point it at
a v1.19.3 binary. If a managed installation is stale, delete
`runtime/mediamtx` and reinstall.

**"The downloaded file did not match the official checksum."**
Nothing was installed — the archive was discarded. Retry; if it keeps happening,
suspect a proxy or security product rewriting downloads. Never work around this
by installing manually from an unverified source.

**"There is no official MediaMTX release for this operating system..."**
Your OS/architecture is outside the supported matrix. Obtain a v1.19.3 binary
yourself and set `STREAMING_TREE_MEDIAMTX_PATH` to it.

**"The configured port is already used by another application."**
Something else holds 1935 or 9997. Streaming Tree **never terminates another
process to free a port**. Stop the other application, or set
`STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` / `STREAMING_TREE_MEDIAMTX_API_ADDRESS`
to free ports. Remember to update OBS if you change the RTMP port.

Finding the holder:

```bash
# Linux / macOS
lsof -i :1935
```

```powershell
# Windows PowerShell
Get-NetTCPConnection -LocalPort 1935 | Select-Object OwningProcess
```

**"MediaMTX failed repeatedly and will not be restarted automatically."**
The crash-loop guard tripped: five failures within five minutes. Automatic
restarts stop deliberately so the loop does not run forever. Look at the backend
log for the MediaMTX output, fix the cause, then press **Start**.

**"MediaMTX started but did not become ready in time."**
The process launched but its Control API never answered. Usually the Control API
port is blocked or occupied by something that accepts connections without
answering correctly. Check `STREAMING_TREE_MEDIAMTX_API_ADDRESS`.

**OBS is connected but the panel still says "Waiting for OBS".**
Check that OBS uses **Custom...** with exactly the Server and Stream Key shown
in the panel — a mismatched stream key publishes to a path this configuration
does not allow. Also confirm OBS is actually streaming, not just configured, and
that the service reports **Running**.

**Ingest says "Status unavailable".**
MediaMTX is running but the backend cannot read its Control API. Restarting the
service usually clears it.

**MediaMTX keeps running after I close the backend.**
It should not: shutdown stops and reaps it. If it happens, note how the backend
was terminated — a `SIGKILL` to the backend gives it no chance to clean up — and
end the `mediamtx` process manually.

### FFmpeg and destination branches

**A destination shows the blocker "FFmpeg is not available."**
No compatible FFmpeg was found. Install one from a source you trust and make
sure it is on `PATH`, or set `STREAMING_TREE_FFMPEG_PATH` to it, then restart
the backend (FFmpeg is only re-probed periodically or at startup). Streaming
Tree never installs FFmpeg for you — see
[Why there is no managed FFmpeg download](#outgoing-streaming-with-ffmpeg).

**A destination shows the blocker "The available FFmpeg is missing a
required capability."**
The located FFmpeg failed at least one capability probe (RTMP input/output,
RTMPS output, the FLV muxer, or `-progress` support) even though it parses
`-version` fine. Most general-purpose FFmpeg builds pass all of these;
check whether yours was built with RTMP support disabled.

**A destination fails immediately with an "unsupported codec" error.**
FLV/RTMP cannot carry every codec, and this stage never transcodes. Change
the source (in OBS) to a codec FLV can carry — H.264 video, AAC audio are
the safe, universally supported choice — rather than expecting Streaming
Tree to silently re-encode.

**A destination keeps restarting and then shows "FFmpeg failed repeatedly
and will not be restarted automatically."**
The same crash-loop guard as MediaMTX's, applied per destination: five
failures within five minutes. Check the destination's `lastError` on the
Streams page, fix the underlying cause (commonly: the destination server is
unreachable, or the port/URL is wrong), then press **Start** again — the
restart counter resets on a fresh explicit start.

**A destination is stuck on "Waiting for input."**
This is expected whenever OBS is not currently publishing to the local
ingest — the branch is deliberately paused, not failing. It resumes on its
own once OBS reconnects, as long as you have not pressed **Stop** since.

**I configured an output server URL but saving it fails validation.**
Only `rtmp://` and `rtmps://` are accepted, a host is required, the port (if
present) must be valid, and the URL may not contain user-info (`user@host`),
a `#fragment`, or control characters. A path (like `/app`) is fine — many
providers use one. The stream key never belongs in this field at all; it has
its own field.

**Is my stream key visible anywhere I should worry about?**
See [Stream-key exposure on the command line](#outgoing-streaming-with-ffmpeg)
for the one honestly-documented limitation: it is briefly present as an
FFmpeg process argument while that destination is running, which on most
operating systems a process list on the *same machine* could observe. It is
never logged, never in an API response, and never on disk outside the OS
credential store.

### Twitch account integration

**"Configure a Twitch Client ID above before connecting an account."**
No Client ID is configured yet, or it failed validation. Register an
application at the
[Twitch Developer Console](https://dev.twitch.tv/console/apps) and either
set `STREAMING_TREE_TWITCH_CLIENT_ID` or paste it into the Settings page —
see [Registering a Twitch application](#connected-accounts-and-twitch-metadata).

**The Client ID field in Settings is disabled and I can't change it.**
It is set by the `STREAMING_TREE_TWITCH_CLIENT_ID` environment variable,
which always wins over anything saved in the database. Unset it (and
restart the backend) if you want to manage the Client ID from Settings
instead.

**Saving a new Client ID fails with a conflict.**
A database-managed Client ID cannot be changed while any Twitch account is
still connected, since a different application can mean invalidated
tokens for existing accounts. Disconnect every Twitch account first.

**"Authorization was denied on Twitch."**
You (or whoever completed the device-code flow) chose not to authorize the
application on Twitch's own page. Click **Connect Twitch** again to start a
fresh attempt.

**"This code expired before it was used."**
The user code has a limited lifetime. Start a new attempt and complete it
more quickly, or check that the device you used to open the verification
link actually reached Twitch (network issues on that device look the same
as simply not finishing in time).

**"The authorization did not grant every required permission."**
Twitch's own authorization page let you decline part of what was requested.
Reconnect and make sure the full permission is granted; Streaming Tree only
ever asks for one scope (`channel:manage:broadcast`), so there is nothing
to selectively decline without breaking metadata publishing.

**An account shows "Reconnect required."**
Twitch could not confirm the account's access on the last check (the token
could not be validated and the automatic refresh also failed — commonly
because the refresh token expired from 30 days of disuse, or the account's
authorization was revoked directly on Twitch). Click **Reconnect** to
re-authorize the same account; nothing about the account's identity or any
destination links needs to be re-entered.

**"Secure storage is currently unavailable" on a Twitch action.**
The same operating-system credential store used for stream keys also holds
Twitch token bundles, and it could not be reached — see "Secure storage
unavailable" above for common causes. Connected accounts and their links
are unaffected in SQLite; only the token bundle-dependent actions
(validate, category search, publish) are blocked until the store is
reachable again.

**"Twitch could not be reached" / a publish or category search fails
intermittently.** A transient network issue talking to Twitch, or Twitch
itself being unavailable. Nothing local was changed; try again.

**"Twitch's rate limit was reached; try again shortly."**
Twitch's own API rate limit (visible in its `Ratelimit-*` response headers)
was hit. This is Twitch-side, not a Streaming Tree limit; wait a short
while and retry.

**The Publish button is disabled and says to select a category first.**
The saved category text has no matching Twitch category ID — either it was
typed by hand without picking a search result, or an older save predates
the category picker. Open the metadata editor, use the category search box,
and select a real result; that stores both the display name and the ID
publishing actually needs.

**Publishing is disabled with a note about unsaved changes.**
Save your local edits first. Publish always sends exactly what is currently
saved in Streaming Tree's database, never an in-progress, unsaved draft —
this is deliberate, not a bug.

### YouTube account integration

**"Configure a YouTube Client ID above before connecting a channel."**
No Client ID is configured yet. Create a Google Cloud project, enable
YouTube Data API v3, create a Desktop-app OAuth client, and either set
`STREAMING_TREE_YOUTUBE_CLIENT_ID` or paste the Client ID into the
Settings page — see
[Registering a Google Cloud project](#connected-accounts-and-youtube-metadata).

**The Client ID field in Settings is disabled and I can't change it.**
It is set by the `STREAMING_TREE_YOUTUBE_CLIENT_ID` environment variable,
which always wins over anything saved in the database — independent of
Twitch's own Client ID variable.

**Saving a new Client ID fails with a conflict.**
A database-managed YouTube Client ID cannot be changed while any YouTube
account is still connected. Disconnect every YouTube account first.

**"Authorization was denied on Google."**
You chose not to approve access on Google's own consent page. Click
**Connect YouTube** again to start a fresh attempt.

**"This attempt expired before it was completed."**
The authorization attempt has a bounded lifetime. Start a new attempt and
complete the Google sign-in more promptly.

**A channel-selection screen appears after signing in.**
The Google account you authorized owns more than one YouTube channel.
Streaming Tree never guesses which one you meant — pick the correct
channel explicitly from the list shown.

**"The authorized channel does not match the account being reconnected."**
During a reconnect, a different YouTube channel was authorized than the
one this connected account represents. Reconnect must authorize the exact
same channel; if you meant to connect a different channel, disconnect this
one first and connect the other as a new account.

**An account shows "Reconnect required."**
Google could not confirm the account's access on the last check. This is
often expected if your Google Cloud project's OAuth consent screen is
still in **Testing** publishing status — Google expires authorization
after seven days in that state regardless of what Streaming Tree
requests. Click **Reconnect** to re-authorize the same channel.

**"Secure storage is currently unavailable" on a YouTube action.**
The same operating-system credential store used for stream keys and
Twitch tokens also holds YouTube token bundles, and it could not be
reached — see "Secure storage unavailable" above. Connected accounts,
links, and selected broadcasts are unaffected in SQLite; only token-
dependent actions (validate, broadcast/category listing, publish) are
blocked until the store is reachable again.

**"YouTube could not be reached" / a publish or listing fails
intermittently.** A transient network issue talking to Google/YouTube, or
the API itself being unavailable. Nothing local was changed; try again.

**"YouTube's API quota was exceeded; try again later."**
Your Google Cloud project's daily YouTube Data API quota (10,000 units by
default) was exhausted. This is Google-side, not a Streaming Tree limit;
it resets daily.

**"Live streaming is not enabled for this channel."**
The connected YouTube channel has not enabled live streaming in YouTube
Studio. Enable it there, then retry.

**The broadcast selector is empty.**
No active or upcoming broadcast was found for the linked channel. Create
and schedule one in YouTube Studio — Streaming Tree does not create a
broadcast for you.

**The Publish button is disabled and says to select a broadcast or
category first.** Select a live broadcast in the destination's own
**Selected broadcast** section, and/or open the metadata editor's category
field and pick a real region-scoped result — both are required before
publishing, and neither is guessed automatically.

**Publishing is disabled with a note about unsaved changes.**
Save your local edits first, exactly like Twitch — Publish always sends
what is currently saved, never an in-progress draft.

### Twitch engagement

**"Additional Twitch permission is required" on the Engagement page.**
The connected Twitch account has only the metadata scope
(`channel:manage:broadcast`); reading chat and events needs five more,
narrowly-scoped permissions. Click **Authorize engagement access** to
start the upgrade — your existing stream key and metadata publishing are
completely unaffected while you do.

**The upgrade shows a new code/consent step even though the account is
already connected.** That is expected: the upgrade reuses the same
Device Code Flow as the initial connection, requesting the union of the
account's current scopes plus the engagement ones. Complete it the same
way you completed the original connection.

**"The authorized identity does not match" during the upgrade.** A
different Twitch login completed the device-code activation than the one
already connected. The upgrade must authorize the *same* account;
disconnect and reconnect as a new account instead if you actually meant
to switch identities.

**The Enable toggle is disabled or shows "Blocked."** Either the
permission upgrade above has not been completed yet, or the account
itself needs reconnecting for an unrelated reason (see "An account shows
'Reconnect required'" under Twitch account integration above) — the
connector's own state and blocker code explain which.

**A connector shows "Reconnecting" repeatedly, or a "possible data gap"
timestamp appeared.** Twitch does not replay events lost during an
ordinary connection loss; the connector reconnects automatically with
bounded backoff and recreates its subscriptions, and is honest about the
gap rather than pretending nothing was missed. This is expected
behavior, not an error — check the connector's own reconnect count and
last-event timestamp to see whether it has recovered.

**A connector shows "Error" and does not reconnect on its own.** Most
commonly, Twitch revoked the authorization directly (on Twitch's own
site) or removed the subscription version this application uses. Use
**Restart connector**, and if that also fails, disconnect and reconnect
the underlying Twitch account.

**The recent-events feed says "Disconnected" or never shows anything.**
The Server-Sent Events connection to the backend dropped, or the
connector itself is not `connected` yet — check the connector card's own
state first; the feed only ever shows what the backend's Event Bus
actually received.

**Does disabling engagement affect my stream key or metadata publishing?**
No. A connected account's engagement connector, its metadata-publishing
capability, and a destination's stream key are three separate facts —
see [Engagement Event Bus and Twitch chat/events](#engagement-event-bus-and-twitch-chatevents).
Enabling or disabling the connector never starts, stops, or otherwise
touches a destination's FFmpeg branch.
