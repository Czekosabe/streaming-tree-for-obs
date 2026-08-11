# Visual template format and library contract (Stage 14A)

This document is the canonical contract for Stage 14A's reusable, portable
visual-design **template**: a named, described, licensed, provider-independent
wrapper around a complete `internal/domain/visualdesign.Document` (the
document contract itself lives in [`visual-designs.md`](visual-designs.md) -
this document never redefines it, only wraps it).

Three distinct schemas are in play here, and this document is careful never
to conflate them:

**A. The visual-design document schema** (`visualdesign.Document`, currently
version 2) - what a single saved design/template actually draws: layers,
bindings, typography, animation. Defined and versioned entirely by
[`visual-designs.md`](visual-designs.md).

**B. The Stage 14A template-interchange schema** (this document,
currently version 1) - the portable wrapper: format discriminator, its own
schema version, target, name, description, author, license, and one embedded
document (A). Defined and versioned entirely here.

**C. The future Stage 14B archive/package schema** - not defined anywhere
yet. Stage 14B will add an archive format (assets, a manifest, its own
versioning) that *contains* one or more Stage 14A templates (B) as its
payload. See §12 for the explicit scope boundary.

**A template's own schema version (B) and the embedded document's own
version (A) are two completely independent counters.** A template schema v1
file may legally embed a visual-design document at version 1 *or* version 2
- see §5.

## 1. What Stage 14A is, in one paragraph

A **reusable template library**: application-owned, immutable **built-in**
templates (§8) plus an operator-owned, persisted **user template** library
(§7), a provider-independent **compatibility** assessment (§9), a **draft-
first** application workflow reusing the existing Alert/Chat Overlay
Designers unchanged (§10), and **asset-free JSON** import/export (§13-§16).
Stage 14A adds no archive format, no asset storage, and no new visual-design
primitive - see §12 for exactly what is deliberately deferred to Stage 14B.

## 2. Why Stage 14 is split into 14A and 14B

The architecture eventually wants template *packages* capable of carrying
real assets (custom images, fonts, sounds). That is a substantially larger
untrusted-input security boundary than anything this project has built so
far: archive extraction, path-traversal protection, symlink/hard-link
rejection, decompression-ratio limits, per-asset and total-package size
limits, asset storage and lifecycle, licence-file handling, image/video/font
decoding and safe presentation, CSP implications for publicly serving a
stored asset, and garbage collection of orphaned assets. None of that can be
designed responsibly as an afterthought bolted onto a first template
implementation.

Stage 14A therefore proves the *reusable-template* concept completely on its
own, safe foundation: real templates, real built-ins, real import/export
semantics, real schema/versioning discipline, real compatibility, and a real
preview/apply workflow - with **no arbitrary packaged bytes anywhere**. Stage
14B will add archives/assets **deliberately**, as its own dedicated task,
once this foundation exists to build on. See §12 for Stage 14B's own
explicit (not-yet-decided) scope list.

## 3. The template file format

A Stage 14A template file is a single, closed, asset-free JSON object:

```json
{
  "format": "streaming-tree-visual-template",
  "schemaVersion": 1,
  "target": "chat",
  "name": "Minimal Dark",
  "description": "A clean, low-contrast dark chat item.",
  "author": "Jane Streamer",
  "license": "CC0-1.0",
  "visualDesign": { "version": 2, "canvas": { "...": "..." }, "layers": [ "..." ] }
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `format` | string, exact literal | Must equal `"streaming-tree-visual-template"` - guards against importing an unrelated JSON file. Any other value is rejected outright. |
| `schemaVersion` | integer | The template-interchange schema version (currently `1` - see §5). |
| `target` | closed enum | `"alert"` or `"chat"` - see §4. |
| `name` | string, 1-80 Unicode code points | Required, never empty. |
| `description` | string, 0-400 Unicode code points | Optional. |
| `author` | string, 0-100 Unicode code points | Optional, plain text only. |
| `license` | string, 0-120 Unicode code points | Optional, plain text only (e.g. `"CC0-1.0"`, `"All rights reserved"` - never validated against a license registry). |
| `visualDesign` | object | A complete `visualdesign.Document` (schema A above) - the exact same wire shape the management visual-design API already uses. |

No other top-level field is accepted - the backend decoder rejects an unknown
field outright (see §14), the same discipline every other write endpoint in
this project already uses.

## 4. Target: a closed, coarse owner category

`target` is `"alert"` or `"chat"` - deliberately coarser than "this exact
alert rule" or "this exact chat overlay." It answers "which Designer/owner
*kind* was this built for," never "which specific rule/overlay." A specific
owner instance's own finer-grained compatibility (does this alert rule's own
event type support every binding the template uses) is a separate, later
check - see §9.

## 5. Template schema version vs. visual-design document version

`schemaVersion` (this document, "B") counts revisions to the **portable
wrapper shape** - if a future Stage 14A change ever needs a new top-level
field or a changed metadata bound, that bump lives here, independently of
anything happening to the embedded document. `visualDesign.version` (see
[`visual-designs.md`](visual-designs.md), "A") counts revisions to the
**document itself** - currently 2, and rising independently of this
wrapper's own version.

Worked example: a Stage 14A template schema v1 file may legally contain a
visual-design document at version 1 (e.g. an alert design exported before
Stage 13B shipped) *or* version 2 (anything saved since). Both are valid
template-schema-v1 files. The template wrapper's own version never needs to
change just because the embedded document's version changed, and vice versa
- see §6 for exactly how an older embedded document is handled.

## 6. Importing an older embedded visual-design version

Stage 14A import is the first genuinely **portable** consumer of a visual-
design document - unlike the management API (which only ever reads what this
same application already wrote), an imported file could have been exported
by an older version of this same application, or hand-authored.

The backend therefore always:

1. reads the embedded document's own `version`,
2. runs it through the exact same `visualdesign.MigrateToCurrentVersion` the
   SQLite `visual_designs` repository already uses on every read (see
   [`visual-designs.md`](visual-designs.md) §11/§19) - a stored (or
   imported) version-1 document is transparently upgraded to version 2,
   losslessly,
3. runs the result through the exact same `visualdesign.Validate` every
   other write path already uses,
4. only a document that is now genuinely at `CurrentVersion` and
   structurally/semantically valid is accepted.

**Rejected outright, never silently reinterpreted:** version `0`, any
version between 0 and the oldest known step, any version newer than
`CurrentVersion`, and any malformed/non-integer version field. A freshly
created or freshly exported template's own embedded document is always at
`CurrentVersion` - Stage 14A never persists or exports an older version.

Implementation: `internal/domain/visualtemplate.NormalizeAndValidateDocument`
is the one shared entry point every creation/import path calls.

## 7. Persisted user template library

One new SQLite table, `visual_templates` (migration
`0017_visual_templates.sql`), holding **user templates only**:

```sql
CREATE TABLE visual_templates (
    id TEXT PRIMARY KEY,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('alert', 'chat')),
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    author TEXT NOT NULL,
    license TEXT NOT NULL,
    template_schema_version INTEGER NOT NULL,
    document_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

- `document_json` always stores the document normalized to
  `visualdesign.CurrentVersion` - the same "typed, fully validated JSON
  column, never raw markup" discipline `visual_designs` already established.
- **Never stored here:** built-in templates (§8, never a row at all), raw
  imported file bytes, a file path, export/preview history, screenshots, or
  any asset.
- No foreign key to any owner (`alert_rules`, `chat_overlays`, or
  `visual_designs`) exists, deliberately - see §11.

Local user template ids are always server-generated: `tpl_` + 16 random
bytes, hex-encoded (`visualtemplate.NewTemplateID`). An imported file's own
JSON never carries a local id field at all (§3's closed field list has none),
and even if a client tried to smuggle one past validation, the unknown-field
rejection (§14) would reject the whole request first.

## 8. Built-in template registry

Built-in templates are **application-owned, immutable, reviewed Go
constructors** (`internal/domain/visualtemplate/builtin.go`,
`DefaultBuiltins()`) - never downloaded, never fetched from any remote
registry, never a `visual_templates` row. Each one passes through the exact
same `visualtemplate.Validate` a user template does, so it is provably a
structurally/semantically valid template using only existing safe
primitives - no external asset, no arbitrary URL, no font outside the
existing closed allowlist. `ValidateBuiltins` runs once at
`visualtemplate.NewService` construction (application startup) and fails
loudly (returning an error, aborting startup) rather than silently admitting
a broken built-in.

Stable id namespace: `builtin_` + target + a short slug (e.g.
`builtin_alert_minimal_dark`, `builtin_chat_neon_accent`) - `ValidateBuiltins`
itself rejects any built-in id that collides with the `tpl_` user-id
namespace, so the two id spaces can never be confused.

Current set - three alert, three chat:

| Target | Built-ins |
| --- | --- |
| `alert` | Minimal Dark, Clean Modern, Neon Accent |
| `chat` | Minimal Dark, Compact, Neon Accent |

Built-in **names** may be presented through localized UI labels; the id
itself is never a provider name or trademark, and no built-in includes a
provider logo.

## 9. Target-specific compatibility

`target` alone (§4) does not guarantee a template can be used by every owner
of that target - a chat template's own text bindings might need an activity
item's `quantity`, unavailable on a message-only overlay context; an alert
template's own `quantity` binding is unavailable for a `follow` rule.

`internal/domain/visualtemplate.Compatibility{Compatible bool, Blockers
[]string}` is the one provider-independent, backend-authoritative result -
the frontend never re-derives it. Stable blocker codes:

| Code | Meaning |
| --- | --- |
| `template_target_mismatch` | The template's own `target` does not match the target being checked against. |
| `alert_binding_unavailable` | The owner-instance check (a specific alert rule's own event type) rejected a binding. |
| `chat_binding_unavailable` | The owner-instance check (a chat overlay) rejected a binding. |
| `unsupported_visual_document` | The embedded document is not at `visualdesign.CurrentVersion`. |
| `visual_document_invalid` | The embedded document fails `visualdesign.Validate`. |

`AssessCompatibility(tpl, forTarget, ownerCheck)` takes an optional
`OwnerBindingCheck` function value - a narrow closure, supplied by
`internal/httpapi` (which already has authenticated access to both
`internal/domain/alerts` and `internal/domain/chatoverlay`), never embedded
inside `internal/domain/visualtemplate` itself. This keeps the template
domain package independent of both owning domains and of any provider
concept, exactly like the shared `visualdesign` package itself
(`visual-designs.md` §7). When no owner instance is given, compatibility is
assessed at the target level only.

The management list endpoint (`GET /api/visual-templates`) accepts optional
`target`/`ownerId` query parameters and, when both are present, resolves a
real owner instance (an alert rule's own `EventType` via
`internal/domain/alerts.ValidateDesignBindingsForEventType`, or a chat
overlay via `internal/domain/chatoverlay.ValidateDesignBindingsForChatOverlay`)
and returns a `compatibility` block per template. **A template is never
mutated to make it compatible** - compatibility is a read-only assessment.

## 10. Template application semantics: draft-first, never automatic

**Using a template never saves the owner's design automatically.** A
template is loaded into the existing Alert/Chat Overlay Designer as an
**unsaved draft**, exactly preserving Stage 13's own explicit-save rule:

1. open the Designer,
2. open Templates,
3. preview a template (§10.1),
4. choose "Use as draft,"
5. the Designer's local draft changes,
6. the normal unsaved-changes indicator appears,
7. the operator may keep editing,
8. only the Designer's own existing explicit Save persists anything.

Opening the gallery, previewing a template, or importing a file **never**:
updates the owner's saved design, changes the public overlay/alert
presentation, emits a chat `chat-overlay.presentation` event, mutates the
current/queued alert, or writes a `visual_designs` row. Only the Designer's
pre-existing Save action does any of that - unchanged from Stage 13.

If the current draft is already dirty when a template is chosen, an
application-styled confirmation explains the draft will be replaced; neither
version is auto-saved. Using a template is one logical undo step (not one
step per layer) where the existing undo/redo history model supports it.

### 10.1 Preview vs. the real overlay

Template preview reuses the exact same `VisualDesignRenderer` component the
Designer's own canvas and the real public route both already use - never a
screenshot, never a second renderer. Preview never saves, never enters the
alert queue, never emits chat SSE, and never makes a provider request; an
alert template previews against the Alert Designer's own existing preview-
fixture context, a chat template against the Chat Designer's own existing
preview-fixture context (`docs/visual-designs.md` §17's own item-card
semantics apply unchanged).

### 10.2 Layer ids on template reuse

A template is a complete design draft; every layer id inside it is already
valid and unique within that one document (duplicate layer ids within one
template are rejected at creation/import time, exactly like a saved design).
Reusing the same template across two *different* owners may safely reuse the
identical layer ids, since layer identity is document-local (per
[`visual-designs.md`](visual-designs.md) §5) - there is no cross-document
global uniqueness requirement, and none is invented here.

## 11. No provenance link back to a template

`visual_designs` gains **no** foreign key to `visual_templates`. A design
created from a template becomes its own fully independent copy the moment
"Use as draft" runs - there is no live reference from a saved design back to
whatever template (if any) it started from. Consequently:

- deleting a user template **never** changes any alert design, current
  alert, queued alert, chat design, or currently-visible chat item that was
  ever created from it,
- deleting a built-in is impossible (§8) and the question does not arise,
- "Save as template" (§10.3) reads the current draft and writes a brand-new,
  independent template row - it never links back to whatever owner the draft
  came from either.

### 10.3 "Save as template"

Both Designers support "Save as template," persisting the **current draft**
(not necessarily the owner's saved design) as a new user template:

1. the draft is validated independently (`visualtemplate.Validate` after
   `NormalizeAndValidateDocument`),
2. compatibility is assessed for informational display, never blocking,
3. the operator supplies name/description/author/license,
4. a normalized **copy** is persisted with a fresh `tpl_` id,
5. the owner's own saved design is untouched, and the Designer's own dirty
   state is untouched.

This lets an operator go design draft -> save a reusable template -> keep
editing the owner, without coupling the two operations.

## 12. What Stage 14A explicitly does not implement

Deliberately absent, deferred to Stage 14B or later, never smuggled in "for
later" via a dead schema field:

- ZIP/TAR or any archive format; no archive extraction of any kind,
- any custom image/GIF/video/audio/font layer or upload path,
- any arbitrary image/video/font URL, local file reference, or `data:` URL,
- blob/asset persistence, an asset directory, thumbnail/screenshot
  persistence, asset deduplication, or asset garbage collection,
- a marketplace, a remote template registry, URL-based import, a community
  browser, or an update checker - every import is a local file the operator
  explicitly selected,
- editing a template's own visual document directly inside a dedicated
  template-only designer (to modify a template: load it into the normal
  Alert/Chat Designer as a draft, edit normally, "Save as template" again).

**Stage 14B (planned, not started by this task)** will define, deliberately
and separately: the final portable archive extension/format and its own
manifest; bundled assets and their managed local storage; image/GIF/video/
font decode-and-present decisions; per-file and total package size limits;
decompression-ratio ("zip bomb") protection; absolute-path and `..`
rejection; symlink/hard-link rejection; duplicate-path/case-collision
handling; MIME/signature verification; asset hashing/deduplication if
justified; a deletion/garbage-collection policy; public asset serving and
its CSP implications; licence-file metadata; archive import/export; and
migration of an older archive version. Stage 14B must also explicitly decide
whether a sound asset belongs to a visual template or to the later
audio/TTS subsystem - not decided here.

## 13. Public/private field discipline

Export (§16) never includes: the local `tpl_` id, `createdAt`/`updatedAt`,
any owner id (`alert_rule`, `chat_overlay`), any `visual_designs` database
id, that design's own persistence revision, a public slug, a secret, a
token, a stream key, or a local filesystem path. Only the §3 field list ever
leaves the backend in an exported file - the same "public/private DTOs never
leak management-only state" discipline `visual-designs.md` §9/§23 already
established for the design document itself.

## 14. Import body limits and validation

Raw JSON only - `Content-Type: application/json`, no multipart, no archive.
Body limit: **128 KiB** (`visualtemplate.MaxImportBytes`) - generous
relative to a document's own 64 KiB bound (`visualdesign.MaxDocumentBytes`),
since the portable wrapper's own metadata is small.

Rejected, all before anything is persisted: an oversized body (413), 
malformed JSON (400), an unknown top-level field (400 - the same strict
`DisallowUnknownFields` decoder every other write endpoint already uses), an
unsupported `format` or `schemaVersion` (422,
`visual_template_version_unsupported`), an invalid `target` or out-of-bounds
metadata (422, `visual_template_invalid`), and an unsupported embedded
visual-design version (422, `visual_template_design_version_unsupported`, see
§6). The raw imported body is never logged (§17).

## 15. Static text is never markup

The existing shared `VisualDesignRenderer` renders every static/bound text
value as plain text - there is no HTML field, no CSS field, no JS field, and
no URL field anywhere in a template or a document (`visual-designs.md` §13).
A static text layer whose content happens to be the literal characters
`<script>` is harmless text and stays text; Stage 14A adds no new rendering
path and therefore introduces no new risk here, and no code in this stage
ever uses `dangerouslySetInnerHTML`.

## 16. Export

`GET /api/visual-templates/{id}/export` returns the exact §3 JSON shape -
canonical, normalized, at the current template schema version and the
current visual-design version - with `Content-Disposition: attachment` and a
**safe** filename: every path separator (`/`, `\`, `:`) and control
character (including CR/LF, which could otherwise inject extra response
headers) is stripped from the operator-authored template name, the result is
length-bounded, and a fixed extension is always appended:
**`.streaming-tree-template.json`** - deliberately not Stage 14B's own
eventual archive extension. Built-in templates may be exported too (there is
nothing sensitive in one). A template exported by this application is
guaranteed to re-import successfully into a clean instance of the same
application, with the portable metadata and visual content matching exactly
- only the local database id and timestamps are expected to differ (§7).

## 17. Privacy and logging

`visual_templates` may contain user template metadata and a visual-design
document. It must never contain: raw imported file bytes, real chat/alert
event content, real usernames, tokens, stream keys, OAuth secrets, a public
slug, or an export file path. Application logs may record a template's own
local id, its source (built-in/user), its target, a validation error code,
an import/export result, and a byte count - never the raw imported JSON, a
static text value, a description/author string beyond what is operationally
necessary, or the visual document itself.

## 18. Stable error codes

| Code | HTTP status | Meaning |
| --- | --- | --- |
| `visual_template_not_found` | 404 | No template (built-in or user) exists with that id. |
| `visual_template_invalid` | 422 | Structural/semantic template validation failed. |
| `visual_template_immutable` | 409 | An update/delete was attempted on a built-in. |
| `visual_template_target_mismatch` | 422 | (Reserved for a template-level target check; compatibility's own mismatch uses the `template_target_mismatch` *blocker* code instead, §9.) |
| `visual_template_version_unsupported` | 422 | The template file's own `schemaVersion` is not the current one. |
| `visual_template_design_version_unsupported` | 422 | The embedded visual-design document's own version could not be migrated/validated (§6). |
| `visual_template_import_too_large` | 413 | The raw imported body exceeded 128 KiB. |
| `visual_template_import_invalid` | (mapped through `visual_template_invalid`/decoder errors) | A generic import-validation failure. |
| `visual_template_builtin_invalid` | (never returned to a client - an invalid built-in fails application startup instead, §8) | |

## 19. API surface

A management/editor surface only - **never** exposed on the public,
unauthenticated OBS Browser Source API. Templates are operator-facing
configuration, not public presentation.

```
GET    /api/visual-templates                    list (built-ins + user), optional ?target=&ownerId= for compatibility
GET    /api/visual-templates/{id}                read one (built-in or user)
POST   /api/visual-templates                     create a user template ("Save as template" or a direct create)
PUT    /api/visual-templates/{id}                update a USER template's own metadata only
DELETE /api/visual-templates/{id}                delete a USER template

POST   /api/visual-templates/import/preview      validate a raw file, no persistence
POST   /api/visual-templates/import              re-validate and persist a new user template
GET    /api/visual-templates/{id}/export         download the portable JSON file
```

`POST`/`PUT` bodies never accept a client-supplied local id or timestamp -
the server always generates both. `PUT` and `DELETE` against a built-in id
return 409 `visual_template_immutable`.

## 20. Implementation map

- `internal/domain/visualtemplate` - the provider-independent domain:
  `template.go` (the `Template`/`Target`/`Source` types and id generation),
  `validation.go` (`Validate`, `NormalizeAndValidateDocument`),
  `compatibility.go` (`AssessCompatibility`, blocker codes),
  `builtin.go` (`DefaultBuiltins`, `ValidateBuiltins`),
  `repository.go` (the `Repository` interface, user templates only),
  `service.go` (the validated façade: `List`/`Get`/`Create`/
  `UpdateMetadata`/`Delete`/`ImportPreview`/`Import`/`Export`),
  `errors.go` (domain sentinel errors). Imports only `visualdesign` and the
  standard library - never Twitch, EventSub, `internal/alerts`,
  `internal/chatoverlay`, or `internal/operatorchat`.
- `internal/storage/sqlite/visualtemplate_repository.go` - the SQLite
  `Repository` implementation, reusing `visualdesign.MarshalDocumentJSON`/
  `UnmarshalDocumentJSON` (a new, shared, exported wire mirror added to the
  `visualdesign` package for this purpose, alongside - not replacing -
  `visualdesign_repository.go`'s own long-standing private mirror).
- `internal/httpapi/visualtemplate.go` - routes, wire DTOs (a management
  DTO with local identity, and a separate portable file DTO, §3), and the
  owner-instance compatibility resolution that calls into
  `internal/domain/alerts`/`internal/domain/chatoverlay` on the template
  domain's behalf (§9).
