# Visual design document contract (Stage 13A)

This document is the canonical contract for the shared, provider-independent
**visual design document** introduced in Stage 13A: a versioned, bounded,
declarative layer tree an operator builds in a visual designer, persisted per
owner, and rendered by one shared React component on both the management
preview and the real public OBS Browser Source route.

Stage 13A implements this contract for exactly one owner: an **alert rule**
(the Alert Overlay Designer). Stage 13B is expected to reuse this exact same
document/primitive model for the Chat Overlay Designer, adding only whatever
shared primitives that reuse still requires. Stage 14 (built-in templates,
import/export) is expected to package this same document as its payload
format, plus an archive format this document does not define.

The implementation lives at `apps/server/internal/domain/visualdesign` (the
shared, provider-independent domain), with alert-specific binding-capability
and legacy-draft logic kept beside it in `apps/server/internal/domain/alerts`
(never inside the shared package itself - see §12).

## 1. Schema versioning

Every document carries an explicit integer `version`. The current, and so far
only, version is **1** (`visualdesign.CurrentVersion`).

- A document whose `version` does not equal the currently supported version is
  rejected outright by `Validate` - there is no "best effort" reinterpretation
  of an unrecognized shape.
- Stage 13A defines no migration path between versions, because only one
  version has ever existed. A future version bump gets its own explicit,
  reviewed migration function in the domain package; it is never silently
  inferred from the JSON shape.
- The public payload (`PublicDocument`) carries its own `schemaVersion`,
  copied from the source document's `version` - a public consumer (the
  frontend renderer) can reject or gracefully ignore a version it does not yet
  understand, the same additive-versioning discipline every other public
  payload in this project already follows (compare `PublicAlert`'s own
  `schemaVersion`).

## 2. Coordinate system

All layer geometry is expressed in **integer design units** - a fixed,
abstract coordinate space tied to the document's own `canvas`, never CSS
pixels and never the OBS Browser Source's actual viewport size.

- Canvas bounds: width and height each `320`-`3840` design units
  (`MinCanvasWidth`/`MaxCanvasWidth`, `MinCanvasHeight`/`MaxCanvasHeight`).
- Two built-in presets: **Landscape** (1920×1080) and **Vertical**
  (1080×1920) - `CanvasLandscape`/`CanvasVertical`. A custom size is accepted
  only within the same bounds.
- `canvas.transparent` is always `true` for a real Stage 13A design (the
  field exists for a possible future non-transparent preview context, never
  an arbitrary background color).
- A layer's `frame` (`x`, `y`, `width`, `height`) must remain **fully inside**
  the canvas - `x >= 0`, `y >= 0`, `x + width <= canvas.width`,
  `y + height <= canvas.height`. `NaN`, infinite, negative, or zero
  dimensions are all rejected.
- Minimum layer size: `8 × 8` design units (`MinLayerSize`) on both axes.

## 3. Canvas scaling (renderer contract)

The persisted design is deliberately independent of the real Browser Source's
own pixel dimensions. The shared renderer applies a deterministic,
**contain**-style transform:

```
scale = min(viewportWidth / canvas.width, viewportHeight / canvas.height)
```

The scaled design is then centered inside the actual viewport; the
leftover space on either axis stays transparent. Width and height are always
scaled by the *same* factor - never stretched independently - and the
renderer never writes browser-observed pixel dimensions back into the saved
document. This applies identically to the Alert Designer's own canvas/preview
area and to the real public Browser Source route, since both use the exact
same renderer component (see `docs/engagement-architecture.md`'s own Stage
13A note and `docs/obs-browser-source.md`).

## 4. Layer ordering

`layers` is an ordered list. Each layer carries an explicit `order` integer
(ascending: back to front). The persistence layer (`Service.Save`)
**normalizes** `order` to a dense `0..N-1` sequence, stable-sorted by the
caller's own submitted values, on every save - callers never need to reason
about gaps, and rendering never depends on array/DOM insertion order.

## 5. Layer IDs

- Design id: `design_` + 16 random bytes, hex-encoded (`NewDesignID`).
- Layer id: `layer_` + 12 random bytes, hex-encoded (`NewLayerID`), minted by
  the frontend when a layer is added and validated for uniqueness within one
  document server-side (`Validate` rejects a duplicate layer id anywhere in
  the same document).
- Layer ids are stable across editing and reordering, and a **duplicated**
  layer always receives a brand-new id - never derived from its position in
  the array.

## 6. Shared primitive types (Stage 13A's bounded set)

Four layer kinds, deliberately small and generic enough to be reused
unchanged by Stage 13B:

| Kind | Purpose |
| --- | --- |
| `shape` | A solid-color rectangle (fill, border, corner radius) - also used as a "background" instead of a separate arbitrary CSS background concept. |
| `text` | Bound, closed-vocabulary text content (see §7) with bounded typography. |
| `platform_icon` | The application's own existing provider glyph mapping - never an arbitrary icon URL. |
| `avatar` | The safe, already-normalized avatar URL already present on a public alert item - never an arbitrary URL, never a fresh per-render provider request. |

No custom media layer kind (uploaded image/video/audio/font) exists yet -
that remains explicitly out of scope until its own storage/security model is
designed (Stage 13B or Stage 14, not assumed here).

Every layer additionally carries: `id`, `name` (management-only), `kind`,
`visible`, `locked` (management-only), `order`, `frame`, `opacity`
(`0`-`1`), `entryAnimation`/`exitAnimation` (the closed, application-owned
`none`/`fade`/`slide_up`/`slide_left`/`scale` vocabulary already used by
alerts and the chat overlay), and a bounded `animationDurationMs`
(`0`-`2000`).

## 7. Alert-specific bindings (kept out of the shared schema)

A `text` layer's content source is the closed `TextBinding` enum:

`static`, `alert_rendered_text`, `username`, `platform`, `event_type`,
`message`, `quantity`, `group_count`.

- `static` uses the layer's own `staticText` (bounded 500 code points,
  never empty).
- `alert_rendered_text` reuses the Stage 12 rule's own already-rendered,
  already-validated text template - the shared document never replaces or
  duplicates that parser.
- The remaining five mirror `internal/alerts`'s own closed placeholder
  vocabulary one-to-one, so both systems describe the same underlying alert
  data.

**Availability is capability-driven, exactly like Stage 12's own
`AvailablePlaceholders`**: `AvailableTextBindings(eventType)` in
`internal/domain/alerts/visualdesign_binding.go` returns the bindings that
actually make sense for a given event type (e.g. `quantity` is unavailable
for `follow`), reading the *same* `CapabilityFor`/`GroupingCapabilityFor`
tables Stage 12's template placeholders already use - never a second,
independently-maintained list that could drift. `Validate
DesignBindingsForEventType` rejects a save containing an unavailable binding
(422), the design-document analogue of `ValidateTemplateForEventType`.

This capability logic deliberately lives **beside** the alert domain
(`internal/domain/alerts`), not inside `internal/domain/visualdesign` itself
- the shared package has no concept of "alert capability" at all, and never
imports `internal/domain/engagement`, `internal/provider/twitch`,
`internal/alerts`, or `internal/operatorchat`.

### Missing-value behavior

Each text layer also carries a closed `missingValueBehavior`: `hide` (the
only behavior a real public alert ever uses - the layer is simply not
rendered when its bound value is absent) or `placeholder` (editor-preview
only, showing an obviously synthetic missing-data indicator that must never
itself become persisted static text). The **public** renderer always treats
an absent bound value as `hide`, regardless of the saved preference - this is
enforced in the one shared React renderer component, not duplicated per
route.

## 8. Style bounds

Every numeric/enum style field is a closed, validated bound - never a raw CSS
string:

| Field | Bound |
| --- | --- |
| Opacity | `0`-`1` |
| Border width | `0`-`32` |
| Corner radius | `0`-`500` |
| Outline width | `0`-`16` |
| Shadow blur | `0`-`64` |
| Shadow offset (x/y) | `-32`-`32` |
| Font size | `8`-`300` |
| Font weight | `100`-`900`, in steps of `100` |
| Line height | `0.8`-`3.0` |
| Letter spacing | `-2`-`20` |
| Font family | closed allowlist: `system-ui`, `sans-serif`, `serif`, `monospace` - never an arbitrary family string, never an uploaded/remote font |
| Color | `#RRGGBB` or `#RRGGBBAA` only (`IsValidColor`) - never a CSS color name, `rgb()`/`hsl()` function, or CSS variable |
| Static text length | `0`-`500` code points |
| Layer name length | `0`-`80` code points |
| Layers per document | `0`-`50` |
| Document size | `64` KiB serialized |

Every bound is validated on save (`Validate`); an out-of-bounds value is
**rejected**, never silently clamped.

## 9. Public/private fields

`PublicDocument`/`PublicLayer` (`internal/domain/visualdesign/public.go`) is
the only shape ever sent to a public Browser Source or embedded in a public
`alert.show` SSE payload. `ToPublic(doc)` derives it from a validated
`Document` and:

- **excludes** every layer's `name` and `locked` state, the design's own
  database id, its owner kind/id, and every revision-management field,
- **excludes hidden layers entirely** (a layer with `visible=false` is
  dropped from the list, never merely flagged),
- orders the remaining layers by their own `order` ascending.

The management API (`GET`/`PUT /api/alert-rules/{id}/visual-design`) uses a
separate, richer wire DTO (`internal/httpapi/visualdesign.go`) that *does*
include `name`/`locked` - that surface is operator-only (authenticated by
being on the local management API, never exposed publicly), mirroring every
other management-vs-public DTO split already established in this codebase
(compare `AlertSummary` vs `PublicAlert`).

## 10. Persistence

One SQLite table, `visual_designs` (migration `0015_visual_designs.sql`):

```sql
CREATE TABLE visual_designs (
    id TEXT PRIMARY KEY,
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('alert_rule')),
    owner_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    document_json TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (owner_kind, owner_id)
);
```

- `owner_kind`/`owner_id` is a deliberately **polymorphic** pair rather than a
  single-table foreign key - the one new persistence convention this feature
  introduces (every other "one row owns many child rows" relationship in this
  codebase instead uses a direct, single-purpose foreign key column). This is
  a conscious choice, not an oversight: Stage 13A accepts only
  `owner_kind = 'alert_rule'` (`AcceptedOwnerKinds`, enforced again at the
  database layer via `CHECK`), but the column shape is written so a future
  owner kind (Stage 13B's chat overlays) can share this exact table without a
  schema change.
- `document_json` is a versioned JSON blob rather than one column per field -
  the one deliberate exception to this project's general "never a settings
  blob" rule, justified by the shape of the data (a dynamic, variant-typed
  layer tree, unlike `alert_rules`' own fixed column set) and never a
  relaxation of the underlying safety principle: every write is fully
  `Validate`-d against the typed Go struct before it is ever persisted, and
  every read is fully parsed back into that same typed struct before it can
  reach a renderer - malformed JSON can never reach a renderer.
- Because `owner_id` is polymorphic, cascading delete (a deleted alert rule's
  own design must not outlive it) cannot be a SQL foreign key. It is an
  explicit application-level call instead: `internal/alerts.Manager.
  DeleteRule` calls `visualDesignSvc.Delete` as part of deleting a rule.

## 11. Migration behavior (between future document versions)

Not yet needed (only version 1 exists), but the contract for when it is: a
new version gets an explicit `migrateVX toVY(doc)` function in
`internal/domain/visualdesign`, applied on read when an older stored version
is encountered, never a schema-version bump silently reinterpreting old field
names. `Validate` always validates the *current* version's own rules after
any such migration runs.

## 12. Backward compatibility

Every alert rule that predates Stage 13A (i.e. every Stage 12 rule) has no
saved visual design, and **must continue rendering exactly through the Stage
12 fixed renderer** - no migration ever runs against existing rules, and no
existing rule's public appearance changes merely because Stage 13A shipped.

- `GET /api/alert-rules/{id}/visual-design` for a rule with no saved design
  returns a **freshly generated, never-persisted** draft
  (`domain.GenerateLegacyDraft`) approximating that rule's current fixed
  presentation (profile position/text-align/theme, the rule's own
  duration/animations) as closely as practical - not a pixel-perfect
  reproduction, an honest best effort. Opening the designer performs no
  write.
- The design becomes real, and the rule becomes design-driven, **only** after
  an explicit `PUT` (Save).
- `DELETE /api/alert-rules/{id}/visual-design` ("Reset to legacy
  presentation") detaches the saved design and returns the rule to the Stage
  12 fixed renderer - it never deletes the rule itself, and is idempotent.
- `internal/alerts.PublicAlert` carries an explicit, additive
  `renderingMode` discriminator (`"legacy"` or `"visual_design"`) rather than
  inferring legacy-ness from an absent field - every existing Stage 12
  integration script and frontend consumer keeps working unchanged, since the
  new field is purely additive.

## 13. Security model

- **No arbitrary HTML/CSS/JS ever**: static text and every data-bound value
  are always rendered as plain text (never `dangerouslySetInnerHTML`); no
  style string, class name, HTML fragment, SVG markup, CSS variable, `url()`,
  `data:` URL, or `javascript:` URL is ever accepted anywhere in the document.
  Every style value is a validated, typed, bounded primitive (§8).
- **No arbitrary object paths / expression language**: a text binding is one
  of eight closed enum values (§7) - never `event.user.name`-style path
  syntax, never a template/expression language.
- **No arbitrary URLs**: `platform_icon` uses only the application's own
  glyph mapping; `avatar` uses only the already-normalized, already-safe
  avatar URL a public alert item already carries (HTTPS-only, approved
  hosts, no `data:`/`javascript:` URL, bounded dimensions, safe fallback) -
  the design itself never stores a URL of any kind.
- **No uploaded/remote assets**: no image, video, audio, or font upload path
  exists in Stage 13A. Custom media remains explicitly out of scope until its
  own storage/security model is designed (Stage 13B or Stage 14).
- **Bounded everything**: layer count, document byte size, static text
  length, layer name length, and every numeric style field are all closed,
  validated bounds (§8) - no unbounded array or string anywhere in the
  document.
- **Public/private separation never weakens**: §9's exclusions (layer names,
  locked state, hidden layers, every management/revision field) are enforced
  once, structurally, by `ToPublic` - never re-implemented per call site.

## 14. Stage 13A scope

Implemented: the versioned document (§1-11), the Alert Overlay Designer
(drag/resize/numeric editing, property panel, layer ordering, show/hide/lock,
duplicate/delete, bounded undo/redo, canvas zoom/fit, snapping, deterministic
preview fixtures), immutable per-alert-instance design snapshots (§12's
"Policy A" extended to designs - see `docs/engagement-architecture.md`'s own
Stage 13A note), and the public/management HTTP API.

Not implemented (see the Stage 13A task's own explicit non-goals):
Chat Overlay Designer, template import/export, archive packaging, a built-in
template gallery, AI generation of any kind, an asset marketplace, custom
JS/HTML/CSS, executable expressions, arbitrary data paths/URLs, uploaded
fonts/audio/images/GIFs/video, filesystem browsing, sound playback, TTS,
goals/counters, donation connectors, further engagement providers, arbitrary
SVG/shaders/CSS filters, OBS WebSocket integration, or any form of browser
automation/manual OBS testing.

## 15. Stage 13B relationship

Stage 13B is expected to build the **Chat Overlay Designer** directly on top
of this exact same `visualdesign.Document`/layer model - a second owner kind
(e.g. `chat_overlay`) added to `AcceptedOwnerKinds`, reusing the identical
canvas/layer/style/animation primitives this document already defines. Any
further shared primitive genuinely required to complete that reuse (for
example, a layer kind neither designer currently needs alone) is explicitly
Stage 13B's own scope to add - this document deliberately does not
pre-guess what that will be.

## 16. Stage 14 relationship

Stage 14 (built-in templates, template import/export) is expected to treat
this same `Document` shape as its **template payload** - the thing a
template package contains one or more of. Stage 14 is responsible for its own
archive/packaging format (asset bundling, metadata, versioning of the archive
itself, migration of an imported older template package) on top of this
payload; none of that is defined here, and none of it is implemented by
Stage 13A.
