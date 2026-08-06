# OBS Browser Source contract (Stage 10 research)

Research date: 2026-08-06.

This document records what was researched from **official OBS sources
only** before designing the Stage 10 chat overlay, and the resulting
recommendations. It is deliberately short and paraphrased - no large
passages are copied from OBS's own documentation. Re-check this
document whenever OBS changes Browser Source behavior; nothing here is
guaranteed to stay accurate forever.

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
local file, so the overlay is served exactly like any other page this
application's frontend already serves.

## Recommended Stage 10 setup

- **URL, not local file.** Point Browser Source at
  `http://127.0.0.1:5173/overlay/chat/{publicSlug}` in development (or
  the equivalent production origin once a packaging stage exists) -
  never a saved local HTML file, so the overlay always reflects live
  configuration and live chat.
- **Width/height**: recommend **1920×1080** for a normal horizontal-
  stream layout and **1080×1920** for a vertical-stream layout,
  matching common canvas sizes - but the renderer itself does not hard-
  code either; it responds to whatever viewport OBS actually gives it
  (Part 11's own requirement). OBS's own default Browser Source
  viewport is 800×600, so this must be set explicitly per profile.
- **Custom CSS: leave empty.** OBS's own default Browser Source CSS
  already sets a transparent `rgba(0,0,0,0)` background, removes body
  margin, and hides scrollbars - exactly what an overlay needs, and
  precisely why this stage never asks an operator to paste any CSS at
  all (see Part 11's arbitrary-CSS prohibition).
- **FPS**: leave the custom-FPS option off (OBS's own default, capped
  around 30 when enabled). This overlay has no continuous animation
  loop of its own - items are static between arrivals - so there is
  nothing that benefits from a higher capture rate, and leaving it off
  avoids OBS re-rendering an idle page needlessly.
- **Page permissions**: leave every permission at OBS's own most
  restrictive default. This overlay never calls `window.obsstudio` -
  see "What the overlay deliberately does not use" below - so it needs
  none of OBS's READ_OBS/READ_USER/BASIC/ADVANCED/ALL permission tiers
  the plugin can grant a page.

## Shutdown / refresh checkboxes - trade-offs, not a single right answer

OBS's Browser Source properties expose three related but distinct
controls; the official page describes what each does, not which
combination to use for a specific kind of source. This project's own
recommendation, with the trade-off stated honestly:

- **"Shutdown source when not visible"** (OBS default: **off**). When
  on, CEF fully unloads the page's JS context the moment the source's
  scene is not visible (not just OBS minimized - literally not the
  active scene), and reloads it fresh the next time it becomes
  visible. Every open connection, including this overlay's SSE stream,
  is closed and re-established from scratch on reload.
  - *Trade-off*: turning this **on** saves CPU/GPU while a scene using
    the overlay is off-screen - valuable for a streamer with several
    scenes and the overlay in only one. The cost is a brief, visible
    "re-populate" moment on every scene switch back to it: the reloaded
    page reconnects its SSE stream, whose own first event is a complete
    reset of everything currently visible - so it starts from whatever
    the backend currently retains rather than continuing seamlessly
    (see "How the retained projection is restored after a reload"
    below for exactly what that first event contains and why no
    separate snapshot fetch is involved).
  - Leaving it **off** (the default) keeps the overlay's state warm
    and its SSE connection open continuously, at the cost of
    background resource use even while off-screen.
  - **Recommendation**: leave off for a single always-visible chat
    overlay scene (the common case); turn on only if the same overlay
    profile is used in a scene that is rarely active, and the operator
    accepts the brief repopulation moment as a trade for lower idle
    resource use.
- **"Refresh browser when scene becomes active"** (OBS default:
  **off**). Forces a fresh page load every time OBS switches *to* the
  scene containing the source, regardless of the shutdown setting
  above. Same reload/reconnect cost as above, but triggered by every
  scene switch rather than only after being fully unloaded.
  - **Recommendation**: leave off for the same reason - this overlay
    has no reason to want a hard reload on every switch, since its own
    reconnect-and-replay behavior (see below) already keeps it current
    without one.
- **"Refresh cache of current page"** is a one-shot manual action
  (a button, not a persistent setting) - useful for an operator to
  force a reload after changing a profile's settings, though saving a
  profile already triggers a live reset for connected clients (see the
  management page's own "successful Save triggers a public reset"
  behavior) so a manual refresh is a fallback, not a requirement.

## What happens to the SSE connection when OBS unloads the source

Whether triggered by "shutdown when not visible" or an operator simply
closing the browser tab used to preview the overlay outside OBS, an
unload closes the `EventSource` connection exactly like closing any
browser tab does - the backend's own per-overlay subscriber is
released, and no special OBS-specific handling exists or is needed for
this: it is the same "unsubscribe on unmount" behavior every other SSE
consumer in this project already has (see `use-operator-chat-stream.ts`
and this stage's own overlay hook).

## How the retained projection is restored after a reload

**Corrected 2026-08-06** (an earlier draft of this section wrongly
described a separate snapshot fetch the shipped frontend never
performs - see `docs/progress.md`'s Stage 10 corrective-pass entry for
the full investigation).

On reload the overlay page performs exactly two steps, not three:
fetch the public config (`GET /api/public/chat-overlays/{slug}/config`,
a one-time read of the profile's renderer settings), then open a fresh
SSE connection (`GET /api/public/chat-overlays/{slug}/stream`). There
is no separate call to `GET /api/public/chat-overlays/{slug}/items`
from the overlay page itself - the stream's own **first event is
always a complete `chat-overlay.reset`**, carrying every item currently
visible for that profile in one payload
(`internal/chatoverlay.Projection.Subscribe` replays its retained
revisions before the connection is considered live - see
`internal/chatoverlay/projection.go`). That one event is a strict
superset of what a separate `/items` snapshot request would have
returned, so fetching both would only add a second network round trip
and a real race: the snapshot response and the stream's own first
event are not part of one atomic read, so a message could arrive,
appear in the snapshot fetched first, then be evicted or deleted before
the stream's own replay catches up (or the reverse), leaving the
frontend to reconcile two independently-fetched views of the same
mutable state instead of trusting one. Relying on the stream's own
initial reset avoids that class of bug entirely, at the cost of the
overlay staying blank for the handful of milliseconds between opening
the connection and that first event arriving - an acceptable trade for
a Browser Source, which has no loading-spinner UI to show in that gap
anyway.

The reset itself reflects whatever the public overlay projection
currently has visible for that profile - bounded by its own
`max_visible_items` and `message_lifetime_seconds` settings - not a
replay of everything that was ever shown. This is the intended
behavior, not a limitation to work around: an overlay is a live
viewer-facing rendering, not a chat archive.

`GET /api/public/chat-overlays/{slug}/items` still exists and is fully
supported - it answers the same current-visible-set data the stream's
own reset carries, useful for a diagnostic tool, a script (this
project's own `scripts/verify-chat-overlay.mjs` uses it, since a
one-shot Node script has no reason to hold an open SSE connection just
to check current state), or any future direct API consumer that only
needs a point-in-time read rather than live updates. The React overlay
route simply has no reason to call it, given the stream already
provides a complete, race-free initial state on its own.

### Reconnect and `Last-Event-ID`

A reconnect (after a network blip, a scene switch with "shutdown when
not visible" on, or `EventSource`'s own automatic retry) sends the last
sequence number the client actually applied via the `Last-Event-ID`
header, exactly like the operator-chat stream already does. If the
backend's retained revision ring still covers that sequence, the
client receives only the upsert/remove operations it missed, not a
full reset - a normal, low-cost catch-up. If the gap is too large (the
ring evicted revisions the client would need), the backend sends an
explicit `chat-overlay.gap` event followed by a fresh
`chat-overlay.reset` instead of silently under-reporting history - see
"What can be lost after a projection-buffer gap" below.

## What can be lost after a projection-buffer gap

If the overlay's own retained-revision ring buffer has evicted
revisions the reloading client would need to replay from its last-known
sequence (an unusually long disconnect, or an unusually active chat),
the stream instead sends an explicit `gap`/`reset` operation rather
than silently under-reporting history - see the revision-protocol
section of `docs/project-overview.md`'s Stage 10 entry. The reloaded
client always ends up in a *correct* current state; what it cannot do
is show messages that both arrived and were already evicted entirely
between the disconnect and the reconnect. This is the same honest-gap
philosophy Stage 8A and Stage 9 already established for the Engagement
Event Bus and the operator-chat projection, applied one layer further
out.

## Browser Source permissions the overlay does not need

This project's overlay page never references `window.obsstudio`
(scene/transition/streaming state, visibility-change events, or
control functions the obs-browser plugin exposes to a permitted page).
Every OBS-specific permission tier the Browser Source properties
dialog can grant - READ_OBS, READ_USER, BASIC, ADVANCED, ALL - is
therefore left unused. This is a deliberate simplification: the
overlay behaves as an ordinary web page so that the exact same
component also works for the in-app preview (Part 19), for a plain
browser tab, and inside OBS, with no code path that only works inside
CEF.

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
count and lifetime (Part 10), no per-message network request for
badges/emotes (Stage 9's already-resolved asset URLs are reused
verbatim), and no custom FPS capture above OBS's own default.

## What was not tested

**No real OBS installation was used for this research or for any
Stage 10 verification.** Every finding above comes from reading the
official pages listed, not from observing a live Browser Source. The
local integration script (`scripts/verify-chat-overlay.mjs`) exercises
the same HTTP/SSE contract a real Browser Source would consume, from a
plain Node.js HTTP client - it proves the backend's contract is
correct, not that OBS's own CEF renders it identically. Re-verify this
document's recommendations manually the first time this feature is
actually used inside real OBS, and re-check it entirely if OBS changes
Browser Source's documented behavior in a future release.
