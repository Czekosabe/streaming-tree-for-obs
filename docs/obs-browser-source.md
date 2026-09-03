# OBS Browser Source contract

This document records what was researched from **official OBS sources
only**, and the resulting recommendations and current contract for every
public overlay/alert/audio/widget route this application serves. It is
deliberately paraphrased - no large passages are copied from OBS's own
documentation. Re-check this document whenever OBS changes Browser Source
behavior; nothing here is guaranteed to stay accurate forever.

**No real OBS installation was used to verify any of this** - see "What was
not tested" at the end of this document.

## Sources inspected

- <https://obsproject.com/kb/browser-source> - the Browser Source
  properties reference.
- <https://obsproject.com/kb/faq-stream-chat> - OBS's own FAQ on
  displaying chat, which explicitly defers to a third-party overlay
  loaded as a Browser Source rather than any native OBS chat feature.
- <https://obsproject.com/kb/stream-tutorial-2-alerts> - the general
  "paste a widget URL into a Browser Source" workflow OBS documents
  for alert/overlay providers.
- <https://github.com/obsproject/obs-browser> - the plugin's own
  README, for the CEF basis, bundling model, and the `window.obsstudio`
  JS API surface.

## What Browser Source actually is

Browser Source is a CEF (Chromium Embedded Framework) web view embedded
directly in an OBS scene, bundled with OBS Studio itself (not a
separate plugin install) on Windows, macOS, and Linux (Ubuntu
PPA/Flatpak). It loads either a URL or a local file, toggled by a
"Local file" property - this project always uses the URL form, never a
local file, so every overlay is served exactly like any other page this
application's frontend already serves.

## Recommended setup

- **URL, not local file.** Point Browser Source at the relevant
  `/overlay/...` route on this application's own origin - never a saved
  local HTML file, so the overlay always reflects live configuration and
  live state.
- **Width/height**: recommend **1920×1080** for a normal horizontal-
  stream layout and **1080×1920** for a vertical-stream layout,
  matching common canvas sizes - but no renderer hard-codes either; each
  responds to whatever viewport OBS actually gives it. OBS's own default
  Browser Source viewport is 800×600, so this must be set explicitly per
  profile.
- **Custom CSS: leave empty.** OBS's own default Browser Source CSS
  already sets a transparent `rgba(0,0,0,0)` background, removes body
  margin, and hides scrollbars - exactly what an overlay needs, and
  precisely why this application never asks an operator to paste any CSS
  at all.
- **FPS**: leave the custom-FPS option off (OBS's own default, capped
  around 30 when enabled). None of these overlays run a continuous
  animation loop while idle - items are static between arrivals - so
  nothing benefits from a higher capture rate, and leaving it off avoids
  OBS re-rendering an idle page needlessly.
- **Page permissions**: leave every permission at OBS's own most
  restrictive default. No overlay this application serves ever calls
  `window.obsstudio` - see "Browser Source permissions the overlays do
  not need" below - so none needs any of OBS's READ_OBS/READ_USER/
  BASIC/ADVANCED/ALL permission tiers the plugin can grant a page.

## Shutdown / refresh checkboxes - trade-offs, not a single right answer

OBS's Browser Source properties expose three related but distinct
controls; the official page describes what each does, not which
combination to use for a specific kind of source. This project's own
recommendation, with the trade-off stated honestly:

- **"Shutdown source when not visible"** (OBS default: **off**). When
  on, CEF fully unloads the page's JS context the moment the source's
  scene is not visible (not just OBS minimized - literally not the
  active scene), and reloads it fresh the next time it becomes
  visible. Every open connection, including an overlay's SSE stream, is
  closed and re-established from scratch on reload.
  - *Trade-off*: turning this **on** saves CPU/GPU while a scene using
    the overlay is off-screen - valuable for a streamer with several
    scenes and the overlay in only one. The cost is a brief, visible
    "re-populate" moment on every scene switch back to it: the reloaded
    page reconnects its SSE stream, whose own first event is a complete
    reset of everything currently visible - so it starts from whatever
    the backend currently retains rather than continuing seamlessly
    (see "How a reload is restored" below for exactly what that first
    event contains and why no separate snapshot fetch is involved).
  - Leaving it **off** (the default) keeps the overlay's state warm
    and its SSE connection open continuously, at the cost of
    background resource use even while off-screen.
  - **Recommendation**: leave off for a single always-visible overlay
    scene (the common case); turn on only if the same overlay profile is
    used in a scene that is rarely active, and the operator accepts the
    brief repopulation moment as a trade for lower idle resource use.
- **"Refresh browser when scene becomes active"** (OBS default:
  **off**). Forces a fresh page load every time OBS switches *to* the
  scene containing the source, regardless of the shutdown setting
  above. Same reload/reconnect cost as above, but triggered by every
  scene switch rather than only after being fully unloaded.
  - **Recommendation**: leave off for the same reason - every overlay's
    own reconnect-and-replay behavior (see below) already keeps it
    current without a hard reload on every switch.
- **"Refresh cache of current page"** is a one-shot manual action
  (a button, not a persistent setting) - useful for an operator to
  force a reload after changing a profile's settings, though saving a
  profile already triggers a live reset for connected clients, so a
  manual refresh is a fallback, not a requirement.

## What happens to the SSE connection when OBS unloads the source

Whether triggered by "shutdown when not visible" or an operator simply
closing the browser tab used to preview an overlay outside OBS, an
unload closes the `EventSource` connection exactly like closing any
browser tab does - the backend's own per-overlay subscriber is
released, and no special OBS-specific handling exists or is needed for
this: it is the same "unsubscribe on unmount" behavior every SSE
consumer in this project already has.

## How a reload is restored

Every stream-based overlay route below performs the same two steps on
load, never three: fetch its own public config once (renderer
settings), then open one SSE connection. There is no separate snapshot
fetch from the overlay page itself - the stream's own **first event is
always a complete reset**, carrying everything currently visible for
that profile in one payload (the backend's own projection replays its
retained revisions before the connection is considered live). That one
event is a strict superset of what a separate snapshot request would
have returned, so fetching both would only add a second network round
trip and a real race: a message could arrive, appear in an
independently-fetched snapshot, then be evicted or deleted before the
stream's own replay catches up (or the reverse). Relying on the
stream's own initial reset avoids that class of bug entirely, at the
cost of the overlay staying blank for the handful of milliseconds
between opening the connection and that first event arriving - an
acceptable trade for a Browser Source, which has no loading-spinner UI
to show in that gap anyway.

A reset reflects only whatever the public projection currently has
visible - bounded by its own retention settings, never a replay of
everything ever shown. This is the intended behavior: an overlay is a
live viewer-facing rendering, not a chat archive. The equivalent
one-shot HTTP snapshot endpoint (e.g. `GET
/api/public/chat-overlays/{slug}/items`) still exists and is fully
supported for a diagnostic tool or a script that has no reason to hold
an open SSE connection - this project's own `scripts/verify-chat-
overlay.mjs` uses it - but no React overlay route calls it, since its
own stream already provides a complete, race-free initial state.

### Reconnect and `Last-Event-ID`

A reconnect (after a network blip, a scene switch with "shutdown when
not visible" on, or `EventSource`'s own automatic retry) sends the last
sequence number the client actually applied via the `Last-Event-ID`
header. If the backend's retained revision ring still covers that
sequence, the client receives only the operations it missed, not a
full reset - a normal, low-cost catch-up. If the gap is too large (the
ring evicted revisions the client would need), the backend sends an
explicit gap event followed by a fresh reset instead of silently
under-reporting history. The reloaded client always ends up in a
*correct* current state; what it cannot do is show items that both
arrived and were already evicted between the disconnect and the
reconnect - the same honest-gap philosophy applies to every stream
below.

## Browser Source permissions the overlays do not need

No overlay page this project serves ever references `window.obsstudio`
(scene/transition/streaming state, visibility-change events, or
control functions the obs-browser plugin exposes to a permitted page).
Every OBS-specific permission tier the Browser Source properties
dialog can grant - READ_OBS, READ_USER, BASIC, ADVANCED, ALL - is
therefore left unused. This is a deliberate simplification: every
overlay behaves as an ordinary web page so that the exact same
component also works for the in-app management-page preview, for a
plain browser tab, and inside OBS, with no code path that only works
inside CEF.

## Known CEF differences from an ordinary browser

CEF is Chromium-based, so standard web platform behavior (fetch,
EventSource, CSS, `prefers-reduced-motion`) is expected to work the
same way it does in a recent desktop Chrome build. The two practical
differences this project's design accounts for:

- Browser Source has no visible browser chrome, address bar, or
  default page background - the transparent default described above
  is a Browser-Source-specific default, not something an ordinary
  `<iframe>` or browser tab gives you automatically (the in-app preview
  therefore explicitly sets the same transparent styling itself,
  rather than relying on an environment default that only exists
  inside OBS).
- A Browser Source's `prefers-reduced-motion` value is whatever the
  host OS reports to CEF; this project does not detect or override it,
  it only respects whatever value the browser context reports, exactly
  as it would in any other browser context.

## Performance with many Browser Sources

The official page inspected does not publish a specific figure for how
many concurrent Browser Sources are safe, so no number is asserted
here. This project's own contribution to keeping any single overlay
instance cheap: no animation loop while idle, a bounded visible-item
count and lifetime where applicable, no per-message network request for
already-resolved badge/emote/asset URLs, and no custom FPS capture
above OBS's own default.

## Current contract per route

Every route below shares everything above (URL-not-local-file,
transparent background, no custom CSS, no OBS-specific permission,
same shutdown/refresh trade-offs). What differs is only the
**application-level** hydration/reset shape.

### Chat overlay - `/overlay/chat/{publicSlug}`

Fetches `GET /api/public/chat-overlays/{slug}/config` once (layout,
visibility, filters, typography, colors, animation, and - when the
overlay has a saved visual design - a `renderingMode` discriminator
plus the current safe public design snapshot), then opens `GET
/api/public/chat-overlays/{slug}/stream`. The stream's first event is
always a complete `chat-overlay.reset`. A `chat-overlay.presentation`
event (no item content, only a sequence number) tells an already-open
Browser Source its config went stale and to refetch it, riding the
exact same ring-buffer/replay/gap mechanism described above - never a
separate reconnect path. See [visual-designs.md](visual-designs.md) §25
for the full design-driven protocol.

### Alert - `/overlay/alerts/{publicSlug}`

Fetches `GET /api/public/alert-profiles/{slug}/config` once (theme,
position, text-alignment, language - never management data), then opens
`GET /api/public/alert-profiles/{slug}/stream`, whose first event is
always a complete `alert.reset` (the current alert if one exists, and
the paused flag - never the queue's future contents, which stay
management-only). Since exactly one alert plays at a time per profile,
there is no analogue of the chat overlay's own visible-item-count/
lifetime bounds to configure. A grouped alert's `alert.show` payload
carries a `groupCount` (1 for an ungrouped alert); a preempted alert
sends a dedicated `alert.hide` event (only the outgoing alert's id and
a stable reason, never its prior rendered content) immediately followed
by the interrupting alert's own `alert.show`, with no artificial delay
for an exit animation. When design-driven, `alert.show` additionally
carries the `renderingMode` discriminator and a complete safe public
visual-design snapshot (never a mere revision reference, so a
reconnecting Browser Source can always reproduce the exact alert it
missed without a design-fetch race) - canvas dimensions, ordered
layers, bounded style/typography/animation values, never layer names,
lock state, or any editor-only field. The renderer scales the persisted
design to whatever viewport OBS's Browser Source actually reports
(contain-style, aspect-preserving, centered - see
[visual-designs.md](visual-designs.md)'s coordinate-system section), so
a saved 1920×1080 design still renders correctly at other configured
dimensions. The exact same `AlertRenderer` component renders both this
public route and the Alerts management page's own local, instant,
queue-free editor preview, so what an operator previews while editing a
rule is exactly what OBS will show.

### Audio - `/overlay/audio/{publicSlug}`

Reuses the same guidance above, with one genuinely new consideration
the other routes do not have: **the source must actually be able to
make sound.** Both the OBS Knowledge Base and the `obsproject/
obs-browser` GitHub repository confirm Browser Source is CEF-based and
explicitly documented as supporting "custom layout, image, video, and
even audio tasks" - but neither official source documents an autoplay
policy, a "Control audio via OBS" property behavior, or CEF's own
audio-routing/mixing configuration. That gap is recorded honestly here
rather than guessed at: **this document makes no claim that OBS's
first programmatic audio play is always accepted, and no claim that
real OBS mixer routing has been manually verified** (see
[audio-tts.md](audio-tts.md) §18 for the full research and the same
caveat recorded at the product-design level).

The audio page opens `GET /api/public/audio/{slug}/stream` directly (no
separate config fetch - there is no theme/position to hydrate for an
audio-only source), whose events are `audio.reset` (fresh-connection
snapshot), `audio.current` (a safe summary: a short-lived playback
token/URL, content type, volume - never the original event, username,
donor email, or message text), `audio.idle` (no active item) and
`audio.gap` (evicted/slow-consumer, mirrors `alert.gap`), with
`Last-Event-ID` replay exactly like the other routes. Unlike the chat
and alert renderers, the audio renderer is a single `<audio>` element
with no visible layout at all - the Browser Source can be sized
arbitrarily small, since nothing is ever drawn to it. The renderer
establishes itself as the sole active playback session on connect (see
[audio-tts.md](audio-tts.md) §15); a second Browser Source pointed at
the same slug immediately supersedes the first rather than doubling
audio, and reports `playback_started`/`playback_ended`/
`playback_failed` back through a narrow `POST .../ack` rather than a
second SSE stream. A rule-owned sound/TTS item flows through this exact
same route, event shape, and single-renderer-session/ack protocol -
never a second audio route or public payload shape (see
[alert-audio.md](alert-audio.md) §8 for the synchronization contract on
top of this route).

### Goal / supporter widgets - `/overlay/widgets/{publicSlug}`

Simpler than the routes above in one deliberate way: it carries **only
the current snapshot, never a delta sequence or event history** (see
[goals-widgets.md](goals-widgets.md) §19). A chat-overlay or alert
client needs `Last-Event-ID` replay because it must reconstruct a
*sequence* of items it may have missed; a goal widget never has that
problem, since the only snapshot that ever matters is the latest one,
and a fresh connection always receives exactly that. `GET
/api/public/widgets/{slug}/stream` therefore sends one `widget.reset`
event immediately on connect, and another whenever a lightweight
internal poll (~1.5s) detects the underlying goal or widget profile
actually changed - no `Last-Event-ID` handling and no `widget.gap`
event, since there is nothing to gap on when every message is already
complete. Like every route above, an unknown or disabled slug never
answers with a hard HTTP error - it opens a normal connection and sends
a safe, empty default snapshot instead. Every widget kind (goal,
latest follower/subscriber/donation, largest donation, recent
supporters, event ticker, session counter, dashboard) is served by
this exact same route, DTO shape, and poll-and-diff mechanism - a
dashboard's own public snapshot simply composes its children's own
snapshots inline, still one `widget.reset` per change, still no
delta/replay machinery of any kind (see
[supporter-widgets.md](supporter-widgets.md) §10).

## What was not tested

**No real OBS installation was used for any of this research or for any
of this project's own overlay verification.** Every finding above comes
from reading the official pages listed, not from observing a live
Browser Source. This project's local integration scripts
(`scripts/verify-chat-overlay.mjs`, `scripts/verify-alerts.mjs`,
`scripts/verify-alert-advanced-queue.mjs`,
`scripts/verify-alert-designer.mjs`,
`scripts/verify-chat-overlay-designer.mjs`,
`scripts/verify-visual-template-packages.mjs`,
`scripts/verify-tts-audio.mjs`, `scripts/verify-alert-audio.mjs`,
`scripts/verify-goals-widgets.mjs`, `scripts/verify-supporter-widgets.mjs`)
exercise the same HTTP/SSE contract a real Browser Source would
consume, from a plain Node.js HTTP client - they prove the backend's
contract is correct, not that OBS's own CEF decodes an image/video
asset, seeks a video via Range requests, renders a custom `FontFace`,
or actually produces audible speech through its own mixer, identically
to a headless test environment. Re-verify this document's
recommendations manually the first time any of these routes is
actually used inside real OBS, and re-check it entirely if OBS changes
Browser Source's documented behavior in a future release.
