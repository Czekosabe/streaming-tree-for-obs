# OBS Browser Source contract (Stage 10 research)

Research date: 2026-08-06.

This document records what was researched from **official OBS sources
only** before designing the Stage 10 chat overlay, and the resulting
recommendations. It is deliberately short and paraphrased - no large
passages are copied from OBS's own documentation. Re-check this
document whenever OBS changes Browser Source behavior; nothing here is
guaranteed to stay accurate forever.

> **Factual status update (stage 12A, completed):** the Stage 12A alert
> route (`/overlay/alerts/{publicSlug}`) reuses this document's research
> and recommendations **unchanged** - it is the same class of Browser
> Source, served the same way, with the same shutdown/refresh trade-offs
> and the same CEF differences. See
> [Stage 12A: the alert Browser Source route](#stage-12a-the-alert-browser-source-route)
> near the end of this document for what is specific to it. **No real
> OBS installation was used for Stage 12A's own verification either** -
> see "What was not tested" at the end of this document.
>
> **Factual status update (stage 12B, completed):** bounded alert
> grouping and mid-alert preemption changed nothing about the OBS-level
> contract this document researched - only the alert route's own SSE
> payload shape gained a `groupCount` field and a dedicated
> `alert.hide` event. See the note at the end of
> [Stage 12A: the alert Browser Source route](#stage-12a-the-alert-browser-source-route)
> for the detail. Still **no real OBS installation was used** for Stage
> 12B's own verification either.
>
> **Factual status update (stage 13A, completed):** the shared visual-
> design engine and Alert Overlay Designer changed nothing about the
> OBS-level Browser Source contract either - the alert route's URL,
> transparent background, and lack of any OBS-specific permission are
> all unchanged. OBS itself does not know or care whether an alert is
> legacy-rendered or design-driven. The only change is that a
> design-driven alert's `alert.show` payload additionally carries a
> `renderingMode` discriminator (`"legacy"` or `"visual_design"`) and,
> when design-driven, a complete safe public visual-design snapshot
> (never a mere revision reference, so a reconnecting Browser Source can
> always reproduce the exact alert it missed without a design-fetch
> race). See the note at the end of
> [Stage 12A: the alert Browser Source route](#stage-12a-the-alert-browser-source-route)
> for the detail. Still **no real OBS installation was used** for Stage
> 13A's own verification either.
>
> **Factual status update (stage 13B, completed - Stage 13 as a whole is
> now complete):** the Chat Overlay Designer changed nothing about this
> document's own OBS-level research either - the chat overlay route's
> URL (`/overlay/chat/{publicSlug}`), transparent background, and lack
> of any OBS-specific permission are all unchanged, and OBS itself does
> not know or care whether an overlay is legacy-rendered or
> design-driven. Unlike an alert, whose entire presentation-plus-content
> arrives in one `alert.show` event, a chat overlay page stays open
> indefinitely while chat continues, so a saved design can change while
> the Browser Source is already displaying it. `GET .../config`
> additively gains `renderingMode` (`"legacy"` | `"visual_design"`) and,
> when design-driven, the current safe public design; a new
> `chat-overlay.presentation` SSE event (carrying no item content, only
> a sequence number) tells an already-connected Browser Source its
> config is stale and to refetch it, riding the exact same
> ring-buffer/replay/`Last-Event-ID`/gap mechanism this document already
> describes below for every other `chat-overlay.*` event - no new
> reconnect behavior, no new gap-recovery path. See
> [`docs/visual-designs.md`](visual-designs.md) §25 for the full
> protocol. Still **no real OBS installation was used** for Stage 13B's
> own verification either.

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

## Stage 12A: the alert Browser Source route

The alert route (`/overlay/alerts/{publicSlug}`) is a **second,
independent instance of the exact same Browser Source contract** this
document researched for the chat overlay - same URL-not-local-file
guidance, same transparent-background/no-custom-CSS recommendation,
same FPS/permissions guidance, same shutdown/refresh trade-offs, and
the same CEF differences. Nothing about *how OBS itself is configured*
changes for an alert source versus a chat overlay one.

What differs is only the **application-level** hydration/reset shape,
mirroring the chat overlay's own pattern one layer over: the alert page
fetches `GET /api/public/alert-profiles/{slug}/config` once (theme,
position, text-alignment, language - never management data), then opens
`GET /api/public/alert-profiles/{slug}/stream`, whose **first event is
always a complete `alert.reset`** - the current alert if one exists, and
the paused flag - never the queue's future contents, which stay
management-only (`internal/alerts/projection.go` reimplements the same
bounded-ring/subscription contract `internal/chatoverlay`'s own
projection already established, not a new mechanism). A reconnect sends
`Last-Event-ID` exactly like the chat overlay; an unbridgeable gap sends
an explicit `alert.gap` followed by a fresh `alert.reset`, never a
silent skip. Since exactly one alert plays at a time per profile (§10 of
`docs/engagement-architecture.md`), there is no analogue of the chat
overlay's own `max_visible_items`/`message_lifetime_seconds` bounds to
configure - the "how much is visible at once" question does not apply
to a single-slot alert renderer the way it does to a scrolling chat
overlay.

The alert renderer needs **no OBS-specific permission** either -
`window.obsstudio` is never referenced, for the same reason the chat
overlay never references it (see "Browser Source permissions the
overlay does not need" above) - and the exact same React component
(`AlertRenderer`) renders both the public route and the Alerts
management page's own local, instant, queue-free editor preview, so
what an operator previews while editing a rule is exactly what OBS
will show.

**Stage 12B (bounded grouping and mid-alert preemption)** changed
nothing about this OBS-level contract either - still the same single
Browser Source, transparent background, no custom CSS, no new
permission. It only added two **application-level** SSE additions on
top of the same `alert.reset`/`alert.show`/`alert.gap` shape above: a
grouped alert's `alert.show` payload carries a `groupCount` (1 for an
ungrouped alert), and a preempted alert now sends a dedicated
`alert.hide` event - carrying only the outgoing alert's id and a
stable reason, never its prior rendered content - immediately followed
by the interrupting alert's own `alert.show`, with no artificial delay
for an exit animation. Since OBS's Browser Source composites whatever
the page renders each frame, this is invisible to OBS itself; it is
purely how the React renderer decides what to show next.

**Stage 13A (shared visual-design engine and Alert Overlay Designer)**
changed nothing about this OBS-level contract either - still the same
single Browser Source, transparent background, no new permission. The
only application-level addition is the `renderingMode` discriminator
and, for a design-driven alert, a complete safe public visual-design
snapshot embedded directly in that alert's own `alert.show` payload
(canvas dimensions, ordered layers, bounded style/typography/animation
values - never layer names, lock state, or any editor-only field). The
existing `AlertRenderer` component still renders both the public route
and the Alerts management page's own local editor preview identically,
branching internally on `renderingMode` rather than becoming two
components - what an operator sees in the Designer's own preview or the
rule editor's preview is exactly what OBS will show. The renderer
scales the persisted design to whatever viewport OBS's Browser Source
actually reports (contain-style, aspect-preserving, centered - see
[visual-designs.md](visual-designs.md)'s coordinate-system section) so
a saved 1920x1080 design still renders correctly at other configured
Browser Source dimensions; this was verified with automated viewport-
size assertions in the frontend test suite, not inside real OBS.

**Stage 13B (Chat Overlay Designer, Stage 13 as a whole now complete)**
changed nothing about this OBS-level contract either - still the same
single chat Browser Source (`/overlay/chat/{publicSlug}`), transparent
background, no new permission. The application-level additions are the
same shape as 13A's, mapped onto the chat overlay's own SSE stream
instead: an additive `renderingMode` field on `GET .../config`, a
complete safe public visual-design snapshot when design-driven, and a
new `chat-overlay.presentation` event (no item content, just a sequence
number) telling an already-open Browser Source to refetch `.../config`
because presentation changed - riding the same ring-buffer/replay/gap
mechanism described above, never a new one. A chat design's own item
canvas is deliberately much smaller than an alert's full-screen one (a
960x280 design-unit "chat item" preset - see
[visual-designs.md](visual-designs.md) §17), instantiated once per
currently-visible item inside the same `ChatOverlayRenderer` that
already owns stacking/lifecycle/entry-exit animation; OBS itself never
sees more than one composited page either way. The existing chat
overlay React components still render both the public route and the
Overlays management page's own preview identically, branching
internally on `renderingMode`.

## What was not tested

**No real OBS installation was used for this research, for any Stage 10
verification, or for Stage 12A's, 12B's, 13A's or 13B's own
verification.** Every finding above comes from reading the official
pages listed, not from observing a live Browser Source. The local
integration scripts (`scripts/verify-chat-overlay.mjs`,
`scripts/verify-alerts.mjs`, `scripts/verify-alert-advanced-queue.mjs`,
`scripts/verify-alert-designer.mjs`,
`scripts/verify-chat-overlay-designer.mjs`) exercise the same HTTP/SSE
contract a real Browser Source would consume, from a plain Node.js
HTTP client - they prove the backend's contract is correct, not that
OBS's own CEF renders either overlay identically. Re-verify this
document's recommendations manually the first time either feature is
actually used inside real OBS, and re-check it entirely if OBS changes
Browser Source's documented behavior in a future release.
