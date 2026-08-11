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
  database layer via `CHECK`). The `visual_designs` table itself, its
  `document_json` shape, and every Go type are already owner-agnostic and
  need no structural change for a second owner kind - but SQLite's `CHECK
  (owner_kind IN ('alert_rule'))` is a literal, closed list, not a foreign
  key against an extensible lookup table, so accepting a second owner kind
  still requires its own migration to widen that literal list (see Stage
  13B's `0016_visual_design_chat_overlay_owner.sql`, which rebuilds the
  table with the wider CHECK and copies every existing row across
  unchanged - SQLite cannot `ALTER TABLE` a `CHECK` constraint directly).
  This document previously claimed the table could accept a new owner kind
  "without a schema change" - that was incorrect, corrected here.
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

Stage 13B builds the **Chat Overlay Designer** directly on top of this exact
same `visualdesign.Document`/layer model: a second owner kind (`chat_overlay`)
added to `AcceptedOwnerKinds` plus its own migration widening the
`visual_designs.owner_kind` `CHECK` constraint (§10/§18), the document version
bumped from 1 to 2 to add the two new shared layer kinds and two new text
bindings chat needs (§19), and chat-specific binding-capability/data-needs
logic kept beside the chat domain, never inside this shared package (§20/§22).
See §17-§25 below for the full Stage 13B contract.

## 16. Stage 14 relationship

Stage 14 (built-in templates, template import/export) is expected to treat
this same `Document` shape as its **template payload** - the thing a
template package contains one or more of. Stage 14 is responsible for its own
archive/packaging format (asset bundling, metadata, versioning of the archive
itself, migration of an imported older template package) on top of this
payload; none of that is defined here, and none of it is implemented by
Stage 13A or Stage 13B.

## 17. Chat item/card design semantics (Stage 13B)

A chat visual design is fundamentally different in shape from an alert one:
an alert canvas represents one whole-screen presentation shown once at a
time, while a public chat overlay shows **several independently-lived items
at once** (a scrolling/stacking list, each with its own entry, lifetime, and
possible moderation removal). Stage 13B's document therefore describes the
visual presentation of **one repeated chat-overlay item/card**, not the
overlay as a whole:

```
ChatOverlayRenderer (internal/chatoverlay's existing Stage 10 projection)
    |
    +-- item A -> VisualDesignRenderer(document, dataContext for A)
    +-- item B -> VisualDesignRenderer(document, dataContext for B)
    +-- item C -> VisualDesignRenderer(document, dataContext for C)
```

The same one saved design is instantiated once per currently-visible item,
each with its own safe, normalized `dataContext` (§20). Everything about
**which** items exist and for how long - filtering (account selection,
hidden users, blocked terms, bot/command filtering, activity-type
selection), the visible-item list itself, `maxVisibleItems`, message
lifetime, capacity eviction, stack direction, and every moderation-lifecycle
removal (message deletion, chat clear, user-messages clear) - remains
entirely owned by the existing Stage 10 `internal/chatoverlay` projection
(`Projection`/`resolvedSettings`/`evaluate`), completely unchanged and never
reimplemented here. The chat visual-design document has no field that could
ever control any of that; a document cannot set its own `maxVisibleItems` or
lifetime, and none of its layers can resurrect, reorder, or hide a specific
item independently of the projection's own decision.

## 18. Chat-overlay ownership (Stage 13B)

Exactly like Stage 13A's alert-rule ownership model (§18 of the Stage 13A
task, "one saved visual design belongs to one alert rule"): one saved
visual design belongs to **exactly one chat-overlay profile**. A profile
may have no saved design (renders through the Stage 10 fixed/legacy
renderer) or exactly one (design-driven item rendering) - never many
profiles sharing one mutable design object. Reusable designs/templates
remain Stage 14's job.

`OwnerKindChatOverlay = "chat_overlay"` is added to `AcceptedOwnerKinds`.
Deleting a chat-overlay profile deletes its saved design through an explicit
application-level call, mirroring `internal/alerts.Manager.DeleteRule`'s own
cascade for alert rules exactly (§10's own reasoning: a polymorphic
`owner_id` cannot be a SQL foreign key, so cascade-on-owner-delete is always
an explicit call, never implicit).

## 19. Document version 2 (Stage 13B)

Stage 13B needs two new closed `TextBinding` values (`timestamp`,
`account_label`) and two new closed `LayerKind` values (`message_fragments`,
`badge_list` - see §21) that a version-1 reader has no way to safely
interpret. Per this document's own §11 migration policy, this is a genuine
schema-vocabulary change, not merely a new owner kind reusing the existing
vocabulary (owner-kind expansion alone, per §18, needed no version bump) -
so `CurrentVersion` becomes **2**, with an explicit `MigrateToCurrentVersion`
function in `internal/domain/visualdesign`.

The migration itself is intentionally trivial and lossless: a stored
version-1 document's wire shape is byte-for-byte identical to a version-2
one (every version-1 field still exists, unchanged, in version 2; only the
set of values `LayerKind`/`TextBinding` accept grew wider). A version-1
document, by construction, never used a value that only exists in version 2
- so migrating it is exactly "parse it as today, then relabel `Version =
2`," with no field renamed, reinterpreted, or dropped. Every existing Stage
13A alert design therefore loads and renders **identically** after
migration - proven by a dedicated test that a pre-migration v1 fixture and
its post-migration v2 result produce the same `PublicDocument`. Migration
runs once, on read, inside the SQLite repository (`fromJSONDocument` +
`MigrateToCurrentVersion`); every new `Save` from either Designer always
writes at `CurrentVersion` going forward. `Validate` continues to reject any
document whose `Version` is not `CurrentVersion` at the point it is
validated - a stale-version *write* is still rejected outright, exactly as
before; only a stale-version *stored row being read back* is transparently
upgraded.

## 20. Shared vocabulary, owner-specific capability (Stage 13B)

`TextBinding` stays one single closed enum in the shared package - Stage
13B does **not** duplicate it. Five existing values are reused verbatim for
chat, with their real-world meaning documented once, here, rather than
per-owner:

| Binding | Alert meaning | Chat meaning |
| --- | --- | --- |
| `static` | operator-authored fixed text | operator-authored fixed text |
| `username` | the alert actor's display name | the message/activity item's user display name |
| `platform` | the alert's own provider | the item's own provider |
| `event_type` | the alert rule's own event type (`follow`, `bits`, ...) | the activity item's own `activityType` (`follow`, `bits`, ...) - same underlying vocabulary, reused rather than inventing a second `activity_type` binding for the identical concept |
| `quantity` | bits/gift-count/redemption quantity | the activity item's own `Activity.Quantity`, where present |

Two values are new, chat-only:

- `timestamp`: the item's own `OccurredAt`, formatted by the shared renderer
  using a fixed, safe format - never a user-suppliable format string.
- `account_label`: the item's own `AccountLabel`, when the owning profile's
  account-label setting resolved one (§22).

One value is **alert-only and never legal for a chat-overlay design**:
`alert_rendered_text` (Stage 12's own rendered text template has no chat
equivalent - a chat message's own text is the `message` binding, or the
`message_fragments` layer kind for rich rendering, §21). Two new layer kinds
(§21) are chat-only in practice today (nothing stops an alert design from
using them structurally, but no alert item ever has fragments/badges to
bind, so the alert binding-capability table never offers them).

This is enforced exactly like Stage 13A's own alert capability check,
**beside** the owning domain, never inside the shared package:

- `internal/domain/alerts/visualdesign_binding.go`'s existing
  `AvailableTextBindings(EventType)`/`ValidateDesignBindingsForEventType`
  additionally reject `timestamp`/`account_label` for every event type (no
  alert item has either), and continue to never offer `alert_rendered_text`
  to chat by construction (chat never calls this function at all).
- A new `internal/domain/chatoverlay/visualdesign_binding.go` defines
  `AvailableTextBindings(itemKind ChatItemKind) []visualdesign.TextBinding`
  (item kind, not event type - see §20.1) and
  `ValidateDesignBindingsForItemKind(doc, itemKind) error`, rejecting
  `alert_rendered_text` outright for every chat item kind and gating
  `event_type`/`quantity` to activity items only (a message item has
  neither).

Neither domain package imports the other; `internal/domain/visualdesign`
imports neither.

### 20.1 Message items vs activity items

A chat visual design is one document, reused for **both** of Stage 10's own
item kinds (`co.KindMessage`, `co.KindActivity`) - Stage 13B does not create
a separate persisted design per item kind or per Twitch event type (that
would be template/preset complexity Stage 14's own job, not this stage's).
`AvailableTextBindings(itemKind)` therefore returns the binding set valid
for *either* kind when validating a save (a design commonly contains both a
`message` layer and an `event_type`/`quantity` layer, since different
layers naturally apply to different real items) - what actually renders for
a specific item at request time is governed by §22's missing-value/hide
behavior, not by validation-time rejection.

## 21. New shared primitives: message fragments and badges (Stage 13B)

Stage 13A's four layer kinds (§6) cannot represent two things Stage 10's
real renderer already supports and Stage 13B must not regress (task Part
12): richly-formatted message text (ordinary text mixed with resolved emote
images and mentions) and a bounded row of badge images. Both are genuinely
new **shared** primitives - reused unchanged by any future owner, not
chat-specific in the type system, exactly like the original four:

- **`message_fragments`**: renders the item's own already-normalized,
  already-ordered message fragments (plain text, resolved emote image,
  mention) - never re-parses raw IRC/EventSub payload, never makes a
  provider request at render time, never accepts `dangerouslySetInnerHTML`.
  An unknown/unrecognized fragment type falls back to its own safe text.
  Payload (`MessageFragmentsProps`): the same bounded typography fields
  `TextProps` already has (font family/size/weight/line height/letter
  spacing/color/alignment - a plain struct field subset, not embedding
  `TextProps` itself, since `binding`/`staticText`/outline/shadow do not
  apply to a fragment stream) plus a bounded `EmoteSize` (8-128 design
  units, matching font-size-adjacent bounds).
- **`badge_list`**: renders the item's own already-resolved public badge
  image DTOs (`1x`/`2x`/`4x` URLs the backend already resolves via
  `chatassets`/badge lookup, exactly the same safe URLs Stage 10's own
  renderer already uses) - never a provider request in the renderer, never
  an arbitrary URL stored in the design itself. Payload (`BadgeListProps`):
  bounded `MaxCount` (1-20), `BadgeSize` (8-128 design units), `Gap`
  (0-32 design units).

Both kinds carry no binding field at all (unlike `text`) - there is exactly
one thing each kind can ever show (the item's own message fragments, or the
item's own badges), so there is nothing to choose between. Both hide safely
when their item has no fragments/badges to show (§22), following the same
`MissingHide`-only-in-public rule §7 already established for text layers.

## 22. Chat data-needs model (Stage 13B)

Stage 10's existing renderer strips several optional public fields
(avatar, badges, account label) when the owning profile's own legacy
`show*` toggle is off (§10's own `buildItem`/`buildUser` in
`internal/chatoverlay/lifecycle.go`) - correct behavior for the legacy
renderer, but wrong for a design-driven one: if a saved design contains an
`avatar` layer while the legacy `ShowAvatar` toggle happens to be off, the
server must not silently strip the very field the active design needs, or
the Designer's own promise ("what you build is what renders") would be a
lie.

`internal/domain/chatoverlay/visualdesign_dataneeds.go` defines a small,
typed, server-side assessment:

```go
type ChatDataNeeds struct {
    Avatar       bool
    Badges       bool
    AccountLabel bool
}

func DeriveDataNeeds(doc visualdesign.Document) ChatDataNeeds
```

`DeriveDataNeeds` walks the (already-validated) document's own layers once
- an `avatar` layer sets `Avatar`, a `badge_list` layer sets `Badges`, a
text layer bound to `account_label` sets `AccountLabel` - and is **derived
only from the saved design**, never from a legacy toggle. When a profile is
design-driven, `internal/chatoverlay`'s own `resolvedSettings` carries this
assessment (`resolvedSettings.designDataNeeds`, populated by
`DefaultSettingsResolver.Resolve`) and `buildUser`/`buildItem` populate a
field whenever **either** the legacy toggle **or** the active design's own
data-needs says to - so an active design's layers are always honestly
populated, while a legacy-mode overlay's behavior is completely unchanged
(nil `designDataNeeds` when no design is saved, exactly the same
`buildUser` logic as today).

This mechanism only ever **adds** fields already governed by privacy/
filtering that happen upstream of it - it can never bypass a blocked term,
a hidden user, moderation removal, or account selection, all of which are
decided by `passesStaticFilters`/`evaluate` before `buildItem` ever runs
(§17's own "filtering stays entirely Stage 10's job").

## 23. Legacy mode and the generated draft (Stage 13B)

Exactly like §12's alert backward-compatibility guarantee: every existing
chat-overlay profile has no saved visual design and **must continue
rendering exactly through the Stage 10 fixed renderer** - no migration ever
attaches a design to an existing profile, and no existing profile's public
appearance changes merely because Stage 13B shipped.

- `GET /api/chat-overlays/{id}/visual-design` for a profile with no saved
  design returns a **freshly generated, never-persisted** draft
  (`chatoverlaydomain.GenerateLegacyDraft`) approximating that profile's
  current fixed presentation (bubble/background, username, message region,
  platform icon, avatar/badges/timestamp/account-label if their legacy
  toggles are already on) as closely as practical - an honest best effort,
  not a pixel-perfect reproduction (mirroring §12's own alert draft's own
  documented honesty). Opening the Designer performs no write and emits no
  public presentation change.
- The design becomes real, and the profile becomes design-driven, only
  after an explicit `PUT` (Save).
- `DELETE /api/chat-overlays/{id}/visual-design` ("Reset to legacy
  presentation") detaches the saved design and returns the profile to the
  Stage 10 fixed renderer - idempotent, never deletes the profile itself.
- While a design is active, the profile's own legacy visual-presentation
  columns (font, colors, bubble, animations, and similar) remain stored
  completely unchanged - never overwritten or deleted by saving a design -
  so Reset to legacy restores exactly the presentation that was active
  before the design was ever saved. Only the profile's own
  filter/lifecycle columns (§17's own list) stay authoritative in *both*
  rendering modes; the legacy *visual* columns are read only by the legacy
  renderer, and only when no design is saved.

## 24. Current-presentation semantics (Stage 13B) vs alert snapshot semantics (Stage 13A)

This is a deliberate, load-bearing difference from Stage 13A, not an
oversight:

- An **alert** design is **snapshotted per alert instance** (§12/§22 of the
  Stage 13A task): Stage 12 already snapshots presentation settings when an
  alert instance is created, and Stage 13A extends that same guarantee to
  designs, because a queued or replayed alert must keep showing exactly the
  presentation it had when it was created, forever, regardless of later
  edits.
- A **chat-overlay** design is **current profile presentation**, not a
  per-item snapshot. A chat item's own *content* (its text, user, badges)
  is fixed at the moment operator-chat created it - exactly like an alert's
  content - but which **visual design** renders that content is not part of
  the item at all; it is resolved fresh, at render time, from whatever the
  owning profile's current saved design is. Saving a new chat design
  therefore updates every currently-visible item's presentation
  immediately, including items that were already on screen before the
  save - there is no chat equivalent of an alert's queued/replayed
  snapshot to preserve, since nothing about a visible chat item is ever
  "replayed" the way a finished alert is.

Concretely: saving a new chat design never mutates, duplicates, resurrects,
or reorders any chat item (§17); it only changes which layer tree
`VisualDesignRenderer` uses to draw the *same* still-current item list -
enforced by reusing the existing `Projection.Configure`/`Manager.Rebuild`
rebuild path (a full, safe re-derivation from whatever operator-chat still
retains, the same mechanism every other settings change already uses,
§25).

## 25. Public presentation update protocol (Stage 13B)

The public chat-overlay page fetches `GET .../config` once and then follows
an SSE stream (`GET .../stream`) - unlike an alert, whose entire
presentation-plus-content arrives in one `alert.show` event, a chat design
can now change while that page stays open indefinitely. Two additive,
race-safe mechanisms, layered on the existing protocol without replacing
it:

1. **`GET /api/public/chat-overlays/{slug}/config`** gains two additive
   fields: `renderingMode` (`"legacy"` | `"visual_design"`, mirroring
   `PublicAlert`'s own discriminator) and, when design-driven, the full
   safe `PublicDocument` (`visualDesign`, mirroring the same field name
   already used on `PublicAlert`). A legacy overlay's config response is
   byte-for-byte unchanged from Stage 10 aside from the additive
   `renderingMode: "legacy"` field - no existing Stage 10 client or
   integration script parses config as anything but an open, additive
   object, so this cannot break one.
2. A new SSE event, **`chat-overlay.presentation`**, carries no item
   content at all - just a sequence number, participating in the exact
   same monotonic revision stream/replay/gap mechanism every other
   `chat-overlay.*` event already uses (`Projection`'s own ring buffer). It
   is emitted exactly once per successful design Save or Delete
   (`Manager.NotifyPresentationChanged`), and tells an already-connected
   client "your presentation config is now stale - refetch `GET .../config`
   before trusting it further." The frontend reducer's own item state
   (upsert/remove/reset) is completely unaffected by this event - it is
   purely a "go re-fetch config" signal, never a substitute for one.

Saving or deleting a chat design also triggers the *existing*
`Manager.Rebuild` path (§24) so already-visible items' own optional fields
(avatar/badges/account label) are correctly re-derived against the new
`designDataNeeds` (§22) - this produces its own ordinary `chat-overlay.reset`
revision, immediately followed by the new `chat-overlay.presentation`
revision, both through the same sequence counter. A client that reconnects
after a retained gap replays every revision it missed, including a
`chat-overlay.presentation` one, so it never ends up with a stale design
paired against fresh item state; a client whose gap could not be satisfied
(`chat-overlay.gap`) always re-fetches `GET .../config` as part of its
existing reset handling regardless, which already carries the current
presentation. No snapshot/config race is possible: config is always fetched
fresh after any signal that it might be stale, never assumed current from a
value cached before the page connected.
