# Stage 22 — reusable stream metadata presets

**Research date:** 2026-09-01. Written before any Stage 22 product code,
per this project's own standing "contract before implementation"
discipline. Audited against the actual current source before writing
anything below - not assumed from the governing task's own suggested
field names.

Stage 22 proceeds while Stage 20E physical/manual verification remains
deferred by the operator - the two are independent, exactly like Stage
21 before it. Starting Stage 22 does not change Stage 20's own status.

## 0. What a preset is, and is not

A preset is a reusable, named bundle of **stream content metadata** - a
title, description, tags, language, and (where applicable) a provider's
own category - that a creator can apply to one or more configured
destinations without retyping it every time they stream the same kind
of content.

A preset is explicitly **not**: a destination/stream-key configuration,
an OBS profile, a credential bundle, a scheduler, an auto-publisher, a
backup/restore feature, or an engagement-event history feature. Presets
never touch `platform_output_settings` (server URL, auto-restart),
`enabled` state, FFmpeg/branch runtime state, connected-account
credentials, or MediaMTX/OBS configuration - all of that is destination
*transport* configuration, a different concept from stream *content*
(`docs/project-overview.md` §8.1's own "four/nine concepts that must
not be confused" already establishes this boundary; presets are simply
a new reusable view over the *content* half of it).

**Applying a preset never publishes anything remotely.** Local Save and
provider Publish are already two separate, explicit actions in this
codebase (`PUT /api/platforms/{id}/metadata` vs `POST /api/platforms/
{id}/metadata/publish`, backed by `usePublishPreviewQuery`/
`usePublishMetadataMutation` on the frontend) - Stage 22 extends the
*Save* side only. Publishing after applying a preset is exactly the
same existing, unchanged, explicit Publish action a manual edit already
requires.

## 1. Audit of the real current metadata architecture

Confirmed by direct source reading:

- **One unified `Metadata` struct, not per-provider schemas**
  (`internal/domain/platform/model.go`): `Title`, `Description`,
  `Category`, `CategoryID`, `Tags []string`, `Language`, `Visibility`,
  `MatureContent bool`, `DVR bool`, `LatencyMode`, `UpdatedAt`. Every
  provider stores into the *same* columns; a `Capabilities` struct per
  provider gates which fields are meaningful. This is the single most
  important finding for Stage 22's own "common vs provider-specific"
  design question (§6 below) - the existing architecture is already
  capability-gated-common, not per-provider-typed.
- **Real per-provider capability/limit/option data**
  (`internal/domain/platform/definitions.go`, all four providers read
  directly, not assumed):
  - `Language`: **one shared vocabulary for every provider that
    supports it** - `supportedLanguages = ["en","pl","de","es","fr"]`
    (BCP 47 subtags), reused verbatim by Twitch/YouTube/Kick (TikTok:
    `Language` capability false). A genuinely portable, provider-
    independent concept.
  - `Visibility`: only YouTube supports it today
    (`[public, unlisted, private]`, defined as shared constants in
    `model.go`, not YouTube-specific literals) - Twitch/Kick/TikTok all
    have `Visibility: false`.
  - `LatencyMode`: **false for every real provider today** - the
    option lists are deliberately empty with a doc comment explaining
    why ("a non-empty list here would misleadingly suggest \[the
    provider\] has latency options this application can actually set
    this stage"). Currently inert everywhere; kept in the shared
    struct only for forward compatibility with the existing domain
    model, not invented by this stage.
  - `MatureContent`: true only for Kick; false elsewhere (Twitch/
    YouTube's own real-API research found no generic equivalent -
    already documented in `definitions.go`'s own comments).
  - `DVR`: false for every real provider today (same "not part of this
    stage's real publish path" reasoning as LatencyMode).
  - **`Category`/`CategoryID` are the one genuinely provider-scoped
    concept**: `CategoryRequiresRemoteID` is `true` for Twitch and
    YouTube (a bare category *string* with no matching ID is an
    explicit publish blocker per `docs/provider-integrations/
    twitch.md`/`youtube.md`), unset (`false`) for Kick/TikTok.
    `CategoryFieldType` even differs semantically (`"category"` for
    Twitch/YouTube/Kick, `"topic"` for TikTok). A Twitch game ID and a
    YouTube category ID are different ID spaces entirely - confirmed,
    not assumed.
- **`ValidateMetadata(def ProviderDefinition, in Metadata) (Metadata,
  error)`** (`internal/domain/platform/validation.go`) is the single
  authoritative validation/normalization function every save already
  goes through (`Service.SaveMetadata` calls it directly). Critically:
  **a field the target provider does not support is a hard validation
  error when non-empty, not a silent drop** - Stage 22's own apply
  logic must therefore *project* a preset's fields down to only what a
  target's capability table supports *before* calling this function,
  never call it with the full common payload against an incompatible
  provider and expect it to gracefully ignore anything.
- **`platform.ValidationError`/`FieldViolation`** is already a shared,
  cross-domain mechanism (`account`, `chatoverlay`, `credential`,
  `output` domains and `httpapi/errors.go`'s `writeValidationError` all
  reuse it directly, confirmed by grep) - Stage 22 reuses it too rather
  than inventing a second validation-error shape.
- **The existing preview-then-explicit-action pattern**
  (`GET /api/platforms/{id}/metadata/publish-preview` then
  `POST .../publish`, `usePublishPreviewQuery`/
  `usePublishMetadataMutation`) is the established precedent Stage 22's
  own compatibility-preview-then-apply flow (§7 below) mirrors.
- **`MetadataEditor`/`MetadataForm`** (`apps/web/src/components/
  metadata/`): the current Stream details editor is **one destination
  at a time** (a tabbed `PlatformTabs` selector, one active platform,
  `MetadataForm` keyed by platform id), with an already-built unsaved-
  changes confirm-discard flow (`isDirty`/`toDraft` in
  `metadata-draft.ts`, a `ConfirmDialog` in `MetadataEditor.tsx`) -
  reused directly for Stage 22's own unsaved-edit-vs-Apply conflict UX
  (§13 below), not reimplemented.
- **`SaveMetadataInput`** (`apps/web/src/api/platform-schemas.ts`) is
  the exact frontend draft shape (`title, description, category,
  categoryId, tags, language, visibility, matureContent, dvr,
  latencyMode`) - the field set a preset's own captured content
  mirrors.
- **Bounds already in force** (`internal/domain/platform/
  validation.go`): `CategoryMaxLength = 100`, `CategoryIDMaxLength =
  64`, `TagMinLength = 2`; per-provider `TitleMaxLength`/
  `DescriptionMaxLength`/`MaxTags`/`TagMaxLength` vary (Title: 60-140;
  Description: 0 or 5000 bytes; Tags: 0-500). A preset is not tied to
  one provider, so it stores at the *most generous* real bound across
  all four providers (never truncating meaningful user content at save
  time) and lets the existing, unchanged `ValidateMetadata` enforce the
  real, tighter, per-target-provider limits at apply time - see §9.
- **Migrations**: next available number is `0030` (`0029_onboarding_
  state.sql` is the latest).

## 2. Preset data model

```go
// Package metadatapreset (apps/server/internal/domain/metadatapreset)

type Preset struct {
    ID        string
    Name      string
    // Note is the preset's own optional short annotation (e.g. "for
    // Just Chatting streams") - never the stream description itself,
    // which lives in Common.Description below. Deliberately a
    // different field name to avoid confusing the two "description"
    // concepts.
    Note      string
    Common    CommonMetadata
    // Providers holds provider-scoped category data only, keyed by
    // the exact provider it was captured from. A provider absent from
    // this map simply has no category set for that provider - never
    // inherited from another provider's entry.
    Providers map[platform.ProviderID]ProviderMetadata
    CreatedAt time.Time
    UpdatedAt time.Time
}

// CommonMetadata mirrors platform.Metadata's own capability-gated
// shared fields exactly - everything except Category/CategoryID
// (provider-scoped, see ProviderMetadata) and UpdatedAt (the
// destination's own, not the preset's).
type CommonMetadata struct {
    Title         string
    Description   string
    Tags          []string
    Language      string
    Visibility    string
    MatureContent bool
    DVR           bool
    LatencyMode   string
}

// ProviderMetadata is the one genuinely provider-scoped concept
// (§1's audit) - never applied to a provider other than the exact one
// it is keyed under.
type ProviderMetadata struct {
    Category   string
    CategoryID string
}
```

### Why this split, not a per-provider-everything model

The governing task's own §7 offered `{common, twitch, youtube, kick,
tiktok}` as one *conceptual*, non-binding example. The real schema
audit (§1 above) shows only Category/CategoryID are genuinely provider-
non-portable; Language/Visibility/LatencyMode already use shared,
capability-gated vocabularies token-for-token across every provider
that supports them, and Title/Description/Tags/MatureContent/DVR are
plain user content or booleans with no provider-specific identity at
all. Giving *every* field a per-provider override slot would duplicate
data for no reason and fight the actual, already-capability-driven
domain model (the governing task's own explicit warning against
"an abstraction that fights the actual provider schemas merely for
theoretical elegance"). The chosen model is the smallest one that
correctly keeps the one real provider-scoped concept scoped.

### Duplicate-name policy

Preset names are rejected on exact duplicate (case-insensitive,
`COLLATE NOCASE` unique index) - simple, predictable, and matches how
a human would expect "My Just Chatting Setup" and "my just chatting
setup" to collide. `ErrDuplicateName` maps to `409 Conflict`.

### Bounds (§27)

| Field | Bound | Basis |
| --- | --- | --- |
| Preset name | 100 runes | Comparable to `platform.DisplayNameMaxLength` (80), slightly more generous for a more descriptive preset name |
| Note | 280 runes | A short annotation, not a second description field |
| `Common.Title` | 140 runes | The most generous real per-provider `TitleMaxLength` (Twitch) |
| `Common.Description` | 5000 UTF-8 bytes | The most generous real per-provider limit (YouTube, byte-counted) |
| `Common.Tags` | ≤500 tags, ≤100 runes each, ≤500 combined UTF-8 bytes | The most generous real per-provider tag limits (YouTube) |
| `ProviderMetadata.Category` | 100 runes | `platform.CategoryMaxLength`, reused verbatim |
| `ProviderMetadata.CategoryID` | 64 runes | `platform.CategoryIDMaxLength`, reused verbatim |
| Presets per installation | 200 | A real, generous, non-arbitrary bound - no legitimate creator workflow needs more; prevents unbounded growth |

Storing at the most generous bound never truncates meaningful content
at save time; the existing, unchanged `ValidateMetadata` is what
enforces a specific target provider's real, tighter limits at apply
time (§9) - exactly mirroring how manual editing already works today
(a value valid for YouTube's 5000-byte description is already rejected,
not truncated, if pasted into a provider with no description support
at all).

## 3. Substage decomposition

- **22A** - preset domain/schema/persistence: Go domain package,
  migration, repository, service, backend CRUD API, secret-exclusion
  proof.
- **22B** - preset CRUD and management UI: list/create/rename/delete,
  "Save current as preset", empty state.
- **22C** - provider-aware apply workflow: compatibility preview,
  atomic multi-destination apply, unsaved-edit conflict handling.
- **22D** - Stream details/Dashboard integration and UX hardening:
  wiring into `MetadataEditor`, accessibility/responsive pass.
- **22E** - integration + packaged-runtime verification + docs.

## 4. Persistence

Migration `0030_metadata_presets.sql`, mirroring `platforms`/
`platform_metadata`/`platform_metadata_tags`'s own three-table shape:

```sql
CREATE TABLE metadata_presets (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    language       TEXT NOT NULL DEFAULT '',
    visibility     TEXT NOT NULL DEFAULT '',
    mature_content INTEGER NOT NULL DEFAULT 0 CHECK (mature_content IN (0, 1)),
    dvr            INTEGER NOT NULL DEFAULT 0 CHECK (dvr IN (0, 1)),
    latency_mode   TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_metadata_presets_name ON metadata_presets (name COLLATE NOCASE);

CREATE TABLE metadata_preset_tags (
    preset_id TEXT NOT NULL REFERENCES metadata_presets (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (preset_id, position)
);

CREATE TABLE metadata_preset_provider_overrides (
    preset_id   TEXT NOT NULL REFERENCES metadata_presets (id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT '',
    category_id TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (preset_id, provider_id)
);
```

No seed data - a fresh database starts with zero presets, matching the
governing task's own explicit "no fake data" requirement. Deterministic,
restart-safe, tracked in `schema_migrations` like every other migration.

**Structural secret exclusion (§8/§40):** every column above is a plain
string/boolean/timestamp describing content or a category label/ID.
There is no column, no field, no JSON blob capable of holding a stream
key, an OAuth token, a client secret, or a credential-store key -
proven by the schema itself, not merely by UI omission. §12 below
records the specific test proving this.

## 5. Multi-destination atomic apply

`internal/domain/platform` gains one new method, alongside the
existing single-item `SaveMetadata`:

```go
// SaveMetadataBatch validates and replaces the metadata of every named
// platform atomically - either all succeed or none are persisted.
// Provider publishing is never part of this transaction: Apply only
// ever writes local metadata.
func (s *Service) SaveMetadataBatch(ctx context.Context, updates map[string]Metadata) (map[string]Metadata, error)
```

Implemented with one `BeginTx`/`defer Rollback`/`Commit` spanning every
platform ID, reusing the exact same `insertMetadataRow` helper the
existing single-item `SaveMetadata` already calls inside its own
transaction - the smallest correct transaction boundary extension, not
a rewrite of the existing method.

`metadatapreset.Service` depends on a narrow port (mirroring this
codebase's own established per-consumer-interface convention, e.g.
`httpapi.PlatformService`):

```go
type PlatformMetadataStore interface {
    GetMany(ctx context.Context, ids []string) (map[string]platform.Platform, error)
    SaveMetadataBatch(ctx context.Context, updates map[string]platform.Metadata) (map[string]platform.Metadata, error)
}
```

satisfied by `*platform.Service` - the preset domain never talks to
SQLite directly for destination data, and never duplicates platform
business logic.

## 6. Apply algorithm (compatibility preview + apply)

For each target destination `d` with provider `P`:

1. Look up `P`'s real `platform.ProviderDefinition` (the single
   authoritative capability table - never a second, hand-written one).
2. Build a candidate `platform.Metadata`:
   - For each `CommonMetadata` field, copy it **only if** `P`'s
     capability for that field is `true`; otherwise leave it at its
     zero value. This is the projection step `ValidateMetadata` itself
     requires (§1) - it never receives a value the target does not
     support.
   - If `Providers[P]` exists, copy its `Category`/`CategoryID`;
     otherwise leave both empty. Never borrowed from another provider.
3. Call `platform.ValidateMetadata(def, candidate)` - the exact
   existing function every manual save already uses.
4. Compare the validated candidate against `d`'s current stored
   metadata field-by-field to classify each field as **will change**,
   **unchanged**, or **not supported by this destination** (a Common
   field the target's capability table doesn't expose at all).

`GET /api/metadata-presets/{id}/apply-preview?platformIds=a,b,c`
returns this classification (plus any validation errors) per
destination, without writing anything - the same "preview first"
pattern `publish-preview` already established.

`POST /api/metadata-presets/{id}/apply` (`{"platformIds": [...]}`)
re-validates independently (never trusts the frontend's own preview
computation as authority, matching the existing publish-preview/
publish split) and, **only if every selected destination's candidate
validates successfully**, applies all of them in the one atomic
transaction from §5. If any destination fails validation, the entire
request is rejected with the specific field errors and nothing is
written - the documented all-or-nothing contract §15/§23 both require;
the user deselects the failing destination or edits the preset, then
retries. No partial-apply mode exists in v1.

**After Apply, the preset and the destination's metadata are
independent persisted objects** (§16 of the governing task) - Apply
copies values once; nothing about a destination stays bound to the
preset it came from, so later editing or deleting the preset never
touches already-applied metadata.

## 7. Create-from-current-metadata

No dedicated backend endpoint: the frontend's "Save as preset" action
(from the active destination's own `MetadataForm`) already holds that
destination's current draft/stored `SaveMetadataInput` in memory. It
opens a small dialog asking for a Name (+ optional Note) and POSTs to
the same generic `POST /api/metadata-presets` create endpoint, with the
body's `Common` fields taken from that draft and a single
`Providers[thatDestination'sProviderID]` entry taken from its
Category/CategoryID. This keeps the backend API surface minimal and
generic (no special "capture" endpoint) while still making "prepare a
reusable preset from what I already have configured" the natural,
one-click primary workflow the governing task requires (§10/§11).

## 8. Unsaved-edit conflict (§32)

`MetadataEditor`/`MetadataForm` already track `dirty` via `isDirty`/
`toDraft`. The "Apply preset" action, when triggered from Stream
details with unsaved edits present, reuses the exact same
`ConfirmDialog` pattern `MetadataEditor` already uses for tab-switching
- explicit confirmation that applying will replace the current
unsaved fields, never a silent overwrite. No dialog appears when there
are no conflicting unsaved edits.

## 9. Validation

Every field a preset can carry is bounded server-side (§2's table) at
create/update time. Applying a preset runs the exact same
`ValidateMetadata` every manual edit already runs - never a weaker
path. An old preset that no longer fits a provider's evolved rules is
never corrupted or silently truncated: the apply-preview surfaces the
real validation error, and the user edits the preset or the target
selection.

## 10. Security

Presets never contain: destination stream keys, OAuth access/refresh
tokens, client secrets, remote-management credentials, overlay
capability tokens, donation/webhook secrets, administrator-password
data, or any credential-store key. This is structural (§4's schema has
no such column), not merely a UI omission - `22A`'s own test suite
includes a test that attempts to construct a `Preset`/`CommonMetadata`/
`ProviderMetadata` from a `platform.Platform` (which itself never holds
a credential either, per the existing domain's own documented
boundary) and asserts every exported field name against a denylist of
secret-shaped identifiers, plus a full JSON-marshal round-trip
asserting the output never contains a value sourced from
`internal/secrets`.

## 11. What remains manual

No physical/manual verification is required or claimed for Stage 22's
automated scope, matching Stage 21's own precedent. `docs/manual-
verification.md`'s existing pending Windows sessions are unaffected -
Stage 22 does not touch installer, tray, diagnostics, or ingest
surfaces.
