# Stage 25 — Stream Setup Profiles

## 0. What this is, and is not

A stream setup profile is a reusable **local** preparation of Streaming
Tree for a particular kind of show ("Gaming", "Podcast", "Weekly
show"). Applying one prepares local configuration - it never starts a
stream, never publishes provider metadata, and never touches a
credential. It is intentional, reusable **workflow** configuration -
narrower than Stage 23's backup format (a full disaster-recovery/
portability snapshot of everything) and broader than a single Stage 22
metadata preset (only stream content metadata).

Stage 25 is explicitly **not**: a generic arbitrary-configuration
snapshot (that is what Stage 23 already is, for a different purpose);
a replacement for Stage 22 metadata presets (it *references* one,
never re-implements preset storage); a way to activate destination
credentials, connected accounts, alert/overlay/audio/chat-automation
"profiles" that this codebase does not actually have a canonical
"current selection" concept for (§2).

## 1. Contract-first audit result

Before designing the model, every existing "profile-shaped" domain was
read directly to determine whether it already has a real, persisted
"active/current/selected" concept - never inferred, never invented.

| Domain | Real "active" concept? | Evidence |
| --- | --- | --- |
| Destinations (`internal/domain/platform`) | **Yes** - `Platform.Enabled bool` | The exact mechanism `branch.Manager.StartEnabled` already reads to decide which destinations to start. |
| Metadata presets (`internal/domain/metadatapreset`) | N/A - applied, not "selected" | `Service.Apply`/`ApplyPreview` already write local metadata directly; nothing tracks which preset a destination's current metadata came from. |
| Alert profiles (`internal/domain/alerts`) | **No** | `alerts.Manager.Start` loads **every** `Enabled` profile into its own live runtime simultaneously (`internal/alerts/manager.go`) - each has its own OBS Browser Source URL. There is no "current" one; excluding one from a setup profile cannot mean "deactivate" it, only "not part of this show's intended alert set," which this codebase has no mechanism to express today. |
| Chat overlay profiles (`internal/domain/chatoverlay`) | **No** | Same shape as alerts - independent `PublicSlug`-addressed profiles, all live simultaneously when `Enabled`. |
| Goals/dashboard widget profiles (`internal/domain/goals`) | **No** | Same shape again - independent `PublicSlug` widgets, no "currently displayed" concept. |
| Chat automation (`internal/domain/chatautomation`) | **No** | Schedules and commands are flat, independently `Enabled` rows with **no parent grouping object at all** - there is nothing to select between. |
| Connected accounts (`internal/domain/account`) | **No** | `Link{PlatformID, AccountID}` is a direct destination↔account association, not a separate "active account" layer - already implied by which destinations a setup profile includes. |
| Audio/TTS settings (`internal/storage/sqlite/audiosettings_repository.go`) | **No** - true singleton | Exactly one `audio_settings` row (`id = 'singleton'`) always exists. There is no *profile* to select - only one configuration ever exists, so there is nothing for a setup profile to meaningfully reference. |

**Conclusion, stated as the actual v1 scope:** only destinations
(§1's `Enabled` mechanism) and an optional metadata preset reference
have a genuine selection concept to build on. Every other domain
audited above is **excluded from Stage 25 v1**, not because it is
unimportant, but because including it would mean inventing an
activation concept this codebase does not have - exactly what the
governing task forbids. This is a deliberate scope decision, stated
here so a future stage does not need to re-derive it.

## 2. Data model

Migration `0033_stream_setup_profiles.sql` (next free number after
Stage 24's `0032`).

```sql
CREATE TABLE stream_setup_profiles (
    id                 TEXT NOT NULL PRIMARY KEY,
    name               TEXT NOT NULL,
    note               TEXT NOT NULL DEFAULT '',
    -- SET NULL, never CASCADE: deleting a metadata preset must not
    -- delete the setup profile that references it (§7 of the
    -- governing task) - the profile survives and reports the
    -- reference as missing (§4).
    metadata_preset_id TEXT REFERENCES metadata_presets (id) ON DELETE SET NULL,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_stream_setup_profiles_name ON stream_setup_profiles (name COLLATE NOCASE);

CREATE TABLE stream_setup_profile_destinations (
    profile_id    TEXT NOT NULL REFERENCES stream_setup_profiles (id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    -- SET NULL, never CASCADE, mirroring Stage 24's own destination-
    -- history pattern exactly: deleting a destination must not delete
    -- the setup profile's own membership row - it survives and
    -- reports "destination missing" (§4). provider_id/display_name
    -- are a snapshot taken when this row is written, purely for
    -- display when platform_id is NULL - never authoritative over the
    -- live platform row while it still exists.
    platform_id   TEXT REFERENCES platforms (id) ON DELETE SET NULL,
    provider_id   TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    PRIMARY KEY (profile_id, position)
);
```

Domain model (`internal/domain/streamsetup`):

```go
type Profile struct {
    ID                string
    Name              string
    Note              string
    Destinations      []Destination // ordered, deterministic
    MetadataPresetID  *string       // nil if none referenced
    CreatedAt, UpdatedAt time.Time
}

type Destination struct {
    PlatformID  *string // nil once the referenced destination is deleted
    ProviderID  string
    DisplayName string
}
```

`Destinations` is the **complete intended-enabled set** for this show
- applying the profile enables exactly these destinations and disables
every other configured destination (§3), matching the governing task's
own worked example precisely.

## 3. Apply semantics

Applying a profile is a **preview-then-confirm** flow, mirroring Stage
22's own `ApplyPreview`/`Apply` shape exactly - never a single blind
write.

**Preview** computes three categories without writing anything:

- *Destinations*: `willEnable` (currently disabled, in the profile),
  `willDisable` (currently enabled, not in the profile),
  `unchanged` (already in the correct state either way).
- *Metadata preset*: if referenced and it still exists, the preview
  reuses Stage 22's own `ApplyPreview` verbatim against the profile's
  destination set - the exact same compatibility/validation
  classification the Stream details page already shows, never a
  second implementation of it. If the referenced preset no longer
  exists, this category reports `missing` (§4).
- *Creator tools*: deliberately absent - §1 found nothing real to
  reference.

**Commit** is two separately-atomic steps, not one cross-domain
transaction - a deliberate, documented tradeoff, the same shape Stage
23's own restore already made and documented honestly rather than
faking a stronger guarantee (`docs/backup-restore.md` §7 step 7):

1. `platform.Service.SetEnabledBatch` (new - §5) enables/disables
   every affected destination in **one** database transaction: either
   every change lands, or none does.
2. If a metadata preset is referenced and still exists, its **existing,
   unchanged** `metadatapreset.Service.Apply` runs against the
   profile's destination set - already its own atomic, all-or-nothing
   transaction across every destination it touches (Stage 22C).

If step 1 fails, nothing changes at all. If step 1 succeeds and step 2
fails (or is skipped because the preset is missing), the destination
membership is now correctly applied - a real, valid, non-corrupt state
- and the metadata step's own outcome is reported separately in the
Apply response, so the operator can see exactly what happened and
retry only what did not.

## 4. Missing references

Neither a missing metadata preset nor a missing destination ever
deletes the setup profile or silently substitutes something else.

- **Metadata preset deleted**: the profile row survives
  (`ON DELETE SET NULL`); `MetadataPresetID` becomes `nil` - reported
  to the operator as "Metadata preset missing," with an explicit
  action to select a different preset or remove the reference. Preview
  treats this category as `missing`, never silently drops it from the
  apply.
- **Destination deleted**: its own `stream_setup_profile_destinations`
  row survives with `platform_id` now `NULL`; the snapshot
  `provider_id`/`display_name` still identify what it used to be.
  Reported as "Destination missing," with an explicit action to remove
  it from the profile. Never recreated automatically, never rebound to
  a different destination merely because the provider matches - ids
  matter, not provider type.

## 5. `platform.Service.SetEnabledBatch` (new)

Mirrors `SaveMetadataBatch`'s own exact shape
(`internal/storage/sqlite/platform_repository.go`): one transaction,
sorted platform-id iteration for deterministic statement order,
`ErrNotFound` if any id does not exist, nothing written if any single
update fails.

```go
// Repository
SetEnabledBatch(ctx context.Context, updates map[string]bool) error

// Service
SetEnabledBatch(ctx context.Context, updates map[string]bool) error
```

This is a small, generically useful addition to the platform domain in
its own right (a batch enable/disable was already awkward to do
atomically before Stage 25), not a Stage-25-only special case.

## 6. Active-stream safety

Blocked entirely - never a partial/forced apply - if **any affected
destination** currently has a branch in `StateStarting`, `StateLive`,
`StateRestarting`, or `StateStopping` (`internal/runtime/branch`'s own
`State` enum, the same states `updater.StreamingActive` already
treats as active). "Affected" is deliberately the union of
`willEnable ∪ willDisable ∪ profile.Destinations` (i.e. every
destination whose `Enabled` flag would change, plus every destination
the referenced metadata preset would write to) - not a blanket
"nothing may be live anywhere" rule, and not a narrower one that could
let a live branch's own enabled flag flip out from under it. The error
message is exactly "Stop streaming before changing the active setup,"
naming which destination(s) are blocking it. Selecting/previewing a
profile in the UI is always allowed while live; only the commit step
is blocked. Streaming Tree never force-kills FFmpeg to unblock an
apply.

## 7. Onboarding

Not part of first-run onboarding (governing task's own explicit
instruction) - a first-time user does not need this concept before
their first stream. A small discovery link appears only once at least
one real setup profile exists, alongside the existing Stage 21
discovery-card conventions - never onboarding bloat.

## 8. Backup/restore integration (Stage 23)

Stream setup profiles are ordinary non-secret creator configuration -
included in Stage 23 backups. `internal/domain/backup.Config` gains
`StreamSetupProfiles []StreamSetupProfileExport`. Restore mints a
fresh id for every profile and every destination-membership row, and
remaps `MetadataPresetID`/`PlatformID` through the exact same `idMap`
every other domain already uses - closing off the identical id-
collision class Stage 23's own §4 already established, applied here
without exception. Restored the same way every other domain is:
platforms and metadata presets are restored **before** stream setup
profiles in `applyConfig`'s own fixed order, so both ids are already
in `idMap` by the time a profile's own references are remapped. A
small, backward-compatible fix rides along: metadata-preset restore
never recorded its own old→new id mapping into `idMap` before Stage
25 (nothing referenced a preset id across domains yet) - it now does,
since Stage 25 is the first thing that needs to.

Stage 24 operational history is **not** added to the backup inventory
merely because this stage also touches backup code - it remains
excluded per its own separate, already-settled contract.

## 9. Security

No stream key, OAuth token, client secret, remote-management/ingest/
overlay credential, or any other secret-shaped value is ever read into
a `Profile`/`Destination` - structurally, not by convention: neither
type has a field that could hold one, proven by the same reflection-
based structural scan Stage 22/23/24 each already established
(`TestProfileStructurallyExcludesSecretShapedFields`). No destination
or connected-account domain object is ever serialized wholesale; only
this package's own explicit DTOs are stored. As with every other
stage's own honest phrasing: this is a guarantee about the *model*,
not an impossible claim that a user could never manually paste a
secret-shaped string into the free-text `Note` field.

## 10. HTTP surface

`POST/GET /api/stream-setup-profiles`, `GET/PUT/DELETE
/api/stream-setup-profiles/{id}`, `POST
/api/stream-setup-profiles/{id}/duplicate`, `POST
/api/stream-setup-profiles/save-current` (captures the currently
enabled destination set into a new named profile; the metadata-preset
reference, if any, is an explicit field in the same request body -
there is no way to detect "which preset is currently applied" from
live metadata alone, so this is never auto-detected), `GET
/api/stream-setup-profiles/{id}/apply-preview`, `POST
/api/stream-setup-profiles/{id}/apply`. Management-only, same route-
namespace boundary as every other non-public route.

## 11. Frontend

A compact `Setup: [Gaming ▾]` control in the Dashboard's own header
actions (`AppShell`'s `actions` slot, alongside the existing "Add
Platform"/"Global Settings" buttons - confirmed as the real, current
control-center location, not a new page), with "Apply setup" and
"Manage" alongside it. Apply shows the preview (§3) behind a
confirmation dialog, mirroring the `ConfirmDialog` destructive-action
pattern already established for Stage 23/24 this same project. "Last
applied: Gaming" is shown as a simple factual record once a profile
has been applied - **no "Modified since apply" drift indicator is
implemented in v1**: destination-membership drift could be tracked
reliably (a live diff against the last-applied profile's own
destination set), but metadata drift cannot (no lineage from an
applied preset back to live metadata exists anywhere in this
codebase), and showing a partial indicator that only accounts for one
of the two would be actively misleading rather than merely incomplete.
This is a deliberate, stated scope decision, not an oversight,
matching the governing task's own explicit "only implement this
distinction if it can be tracked reliably."

## 12. Substage decomposition

- **25A** - this contract; migration; `internal/domain/streamsetup`
  (model, repository port, `Service` with preview/apply/save-current/
  CRUD/duplicate); `platform.Service.SetEnabledBatch`.
- **25B** - HTTP API; `main.go` wiring.
- **25C** - frontend: the Dashboard setup control, manage/apply-
  preview UI, EN/PL.
- **25D** - Stage 23 backup/restore integration and its own round-trip
  tests; final integration test; packaged-runtime extension.

## 13. Testing plan

Backend: zero/create/list/get/update/delete/duplicate; save-current;
apply preview and commit (enable/disable/unchanged classification,
metadata-preset integration, transaction rollback on a destination-
batch failure); missing preset and missing destination handling;
active-stream rejection scoped to affected destinations only; no
provider network call anywhere in apply; no field in `Profile`/
`Destination` is secret-shaped (reflection proof); restart persistence;
backup/restore round trip including id remapping. Frontend: empty
state, create/manage, save-current, apply preview, missing-reference
states, active-stream blocker, delete confirmation, EN/PL,
accessibility. Integration: a real backend against a real temp
database, multiple destinations, a real metadata preset, apply,
restart, verify, backup, restore, verify the profile survives with
remapped references. Packaged: extend `verify-packaged-app.mjs`.

## 14. Completion criteria

Every substage complete and tested per §13; the domain-inclusion/
exclusion table in §1 accurately reflects what shipped; secret boundary
proven; active-stream safety proven; backup/restore integration real;
EN/PL complete; all correctly-routed CI terminal and green; tree clean;
`origin/main...HEAD` = `0 0`. Stage 20E's own physical/manual
verification gate remains independent and is not required.
