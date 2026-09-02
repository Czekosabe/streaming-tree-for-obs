# Documentation

This is the index for Streaming Tree for OBS's documentation. The root
[`README.md`](../README.md) is the short public pitch; everything else
lives here, grouped by what you're trying to do.

## Getting started

- [`development.md`](development.md) — build/run requirements, the
  two-process dev workflow, data storage, production builds, linting/
  testing, the REST API reference, and the repository layout.
- [`connecting-platforms.md`](connecting-platforms.md) — receiving a
  stream from OBS via MediaMTX, sending it onward with FFmpeg, and
  connecting a real Twitch or YouTube account.
- [`troubleshooting.md`](troubleshooting.md) — common problems and their
  fixes.
- [`onboarding.md`](onboarding.md) — the in-app first-run assistant.

## Architecture and product state

- [`project-overview.md`](project-overview.md) — the canonical
  architecture reference: goals, the streaming-router design, the
  independent branch model, the capability-driven metadata model,
  stream-key security, localization, and the full stage-by-stage
  roadmap table.
- [`engagement-architecture.md`](engagement-architecture.md) — the
  engagement/overlay platform's target design: the normalized event
  model, connectors, chat, alerts, and where each layer currently
  stands. Read its own opening notice before treating any part of it
  as implemented — [`progress.md`](progress.md) is the actual current-
  state authority.
- [`platform-support.md`](platform-support.md) — the honest, current
  cross-platform (Windows/macOS/Linux) support and CI-verification
  matrix.
- [`product-identity-legal.md`](product-identity-legal.md) — the
  canonical contract behind the app's public identity, licence, and
  creator-support presentation.

## Streaming and destinations

- [`metadata-presets.md`](metadata-presets.md) — reusable stream title/
  category/tags/language presets.
- [`stream-setup-profiles.md`](stream-setup-profiles.md) — reusable
  destination + preset bundles for a particular kind of show.
- [`stream-preflight.md`](stream-preflight.md) — the "ready to stream?"
  readiness check shown before going live.
- [`stream-session-history.md`](stream-session-history.md) — the
  operational session/destination history record, and the aggregate
  Insights view built on top of it.

## Engagement, chat, and overlays

- [`obs-browser-source.md`](obs-browser-source.md) — the shared OBS
  Browser Source HTTP/SSE hydration contract every public overlay route
  (chat, alerts, audio, widgets) uses.
- [`provider-integrations/`](provider-integrations/) — researched,
  provider-specific contracts:
  - [`twitch.md`](provider-integrations/twitch.md) — account sign-in and
    channel-metadata publishing.
  - [`twitch-engagement.md`](provider-integrations/twitch-engagement.md) —
    the inbound EventSub chat/events connector.
  - [`twitch-outbound-chat.md`](provider-integrations/twitch-outbound-chat.md) —
    sending chat messages and replies as a connected account.
  - [`youtube.md`](provider-integrations/youtube.md) — account sign-in
    and video/broadcast-metadata publishing.
  - [`youtube-engagement.md`](provider-integrations/youtube-engagement.md) —
    the inbound Live Chat gRPC connector.
  - [`external-donations.md`](provider-integrations/external-donations.md) —
    the StreamElements donations connector, and the feasibility research
    behind Streamlabs/Ko-fi remaining deferred.
  - [`kick-engagement.md`](provider-integrations/kick-engagement.md) /
    [`tiktok-live.md`](provider-integrations/tiktok-live.md) — feasibility-
    gate research for providers with no engagement integration yet.
- [`provider-branding.md`](provider-branding.md) — sourcing and licensing
  record for the vendored provider logo marks.

## Visual design and audio

- [`visual-designs.md`](visual-designs.md) — the shared, provider-
  independent visual-design document schema behind the Alert Overlay
  Designer and Chat Overlay Designer.
- [`visual-templates.md`](visual-templates.md) — the reusable, asset-free
  visual-template library (built-ins, a persisted user gallery, JSON
  import/export).
- [`visual-template-packages.md`](visual-template-packages.md) —
  portable archive template packages with managed images/video/fonts,
  and the archive-safety/asset-serving security model.
- [`audio-tts.md`](audio-tts.md) — the shared audio runtime and text-to-
  speech provider abstraction.
- [`alert-audio.md`](alert-audio.md) — persistent alert sound assets and
  per-alert-rule TTS, built on the audio runtime above.
- [`goals-widgets.md`](goals-widgets.md) — the goals/counters
  accumulation engine and core public OBS goal widgets.
- [`supporter-widgets.md`](supporter-widgets.md) — the further supporter/
  activity widget kinds and multi-widget dashboards built on top of it.

## Data, backup, and updates

- [`backup-restore.md`](backup-restore.md) — safe, non-secret
  configuration backup and restore.
- [`updater.md`](updater.md) — the application updater: release source,
  verification, and the Windows install/restart handoff.

## Windows, macOS, and Linux packaging

- [`windows-packaging.md`](windows-packaging.md) — the Windows Inno
  Setup installer, packaged-mode lifecycle, the system tray icon, and
  the EN/PL installer localization contract.
- [`macos-packaging.md`](macos-packaging.md) — the macOS `.app`/DMG
  build, native lifecycle adapters, and its unsigned/not-notarized
  boundary.
- [`linux-desktop-packaging.md`](linux-desktop-packaging.md) — the
  Debian/Ubuntu `.deb` package.
- [`linux-headless-server.md`](linux-headless-server.md) — the opt-in
  `--headless` unattended service mode and its encrypted secret store.
- [`remote-management.md`](remote-management.md) — the opt-in
  `--remote-management` authenticated control-plane, built on the
  headless foundation above.
- [`remote-ingest.md`](remote-ingest.md) — the opt-in `--remote-ingest`
  authenticated RTMPS ingest and remote-overlay capability plane, built
  on remote management.

## Verification

- [`manual-verification.md`](manual-verification.md) — the real,
  ID-based physical/manual test checklist for every packaged platform.
- [`final-hardening.md`](final-hardening.md) — the diagnostics/logging,
  redaction, and support-bundle contract, plus the final release-
  hardening scope.
- [`ci-reliability.md`](ci-reliability.md) — CI workflow design
  patterns: path filtering, concurrency groups, and monitoring
  discipline.

## Project history

- [`progress.md`](progress.md) — the append-only work journal, and the
  authoritative record of what has actually shipped, in what order, and
  why. When any other document's current-state claim is in doubt, this
  file wins.

Historical per-stage narrative belongs in `progress.md`, not scattered
across the documents above — most of the files listed here describe the
system as it stands today, not a chronological account of how it got
there.
