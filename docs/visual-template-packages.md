# Secure visual template packages, managed assets, and custom visual primitives (Stage 14B)

This document is the canonical contract for Stage 14B, written and committed
before any package or asset code is implemented, exactly as
[`docs/visual-templates.md`](visual-templates.md) preceded Stage 14A's own
template code. It defines the portable archive package format, the
manifest schema, archive/asset validation, managed asset storage, the
custom image/video/font visual primitives those assets back, and how all of
this fits into the existing Stage 13/14A visual-design/template system.

## 1. What Stage 14B is, in one paragraph

Stage 14A's template library is asset-free by construction: a template is
one JSON file containing a `visualdesign.Document` built only from the six
existing layer kinds (shape/text/platform_icon/avatar/message_fragments/
badge_list), none of which reference any binary content. Stage 14B adds two
new layer kinds - `image` and `video` - plus an optional custom-font
reference for text-capable layers, and the managed asset storage and
portable archive package format an operator needs to actually use custom
pictures, video clips, and fonts safely. This is the **first** stage that
accepts untrusted binary input from an operator (a manual asset upload) or
from a third party (an imported package), so its design principle,
restated from the task that authorized this work, is load-bearing for
every section below:

> This is the first project stage that intentionally accepts **untrusted
> binary visual assets**. Treat every imported package and uploaded file as
> hostile input. The design stays: declarative, data-only, non-executable,
> local, bounded, inspectable, independently validated before rendering. No
> imported package may provide JavaScript, HTML, CSS, SVG, executable
> files, DLLs, scripts, shell commands, browser extensions, arbitrary
> filesystem paths, arbitrary URL fields, or remote script/style/font/
> media references.

## 2. Sound and audio are explicitly out of scope

Stage 14B does **not** implement visual-template sound or audio playback,
for one deliberate reason: Stage 17 owns TTS and the audio queue, and this
project wants **one** application audio/playback subsystem, not a template
sound engine now and a second, unrelated TTS/audio engine later. Package v1
therefore rejects audio assets, sound layers, and arbitrary audio files
outright - there is no `audio` asset kind. A video asset may contain an
audio track internally (most real-world WebM/MP4 clips do), but the
renderer **always** renders package video muted, with no controls and no
audio output, regardless of what the container holds.

**Factual status update (stage 17A, completed):** Stage 17A shipped that
one audio/playback subsystem - a provider-independent TTS `Provider`, a
bounded runtime audio queue, and a public audio Browser Source route (see
[audio-tts.md](audio-tts.md)) - and, per its own §1, deliberately did
**not** decide whether alert sounds can reference the same managed
blob/storage infrastructure through a separate, audio-safe domain.
Persistent alert sound assets, per-alert-rule TTS, and any audio
extension of this package format remain Stage 17B's own, later,
separately-scoped decision; nothing in Stage 14B or Stage 17A should be
read as having made it. The
same statement appears in
[`docs/visual-templates.md`](visual-templates.md) and
[`docs/engagement-architecture.md`](engagement-architecture.md) so the
boundary is visible from every entry point.

## 3. Four independently versioned schemas

Stage 14B raises the schema count from two to four. Each has its own
version counter; none is reused for another concern.

| # | Schema | Version field | Current value | Scope |
| - | ------ | -------------- | -------------- | ----- |
| A | `visualdesign.Document` | `Version` | 3 (raised from 2 by this stage) | The shared, provider-independent layer tree - what a design/template/package ultimately renders. |
| B | Stage 14A template interchange (`streaming-tree-visual-template`) | `schemaVersion` | 1 (unchanged) | A single asset-free JSON file wrapping one Document. |
| C | Stage 14B package manifest (`streaming-tree-template-package`) | `schemaVersion` | 1 (new) | The archive-level manifest describing a `.streaming-tree-template` package's contents. |
| D | Managed-asset SQLite persistence | migration number | new migrations, this stage | A local implementation detail - never a portable file format, never shipped inside a package. |

Document schema B (Stage 14A's JSON template file) is unchanged by this
stage and remains valid for asset-free templates only (§13). Document
version A moves from 2 to 3 because two new closed layer kinds and one new
optional font reference need representing; see §7 for the migration
contract.

## 4. Package archive format

- **Container:** ZIP only. No TAR, 7z, RAR, or self-extracting archive is
  accepted.
- **User-facing extension:** `.streaming-tree-template` - a naming
  convenience only. Security never depends on the extension or on any
  browser-supplied `Content-Type`/MIME value; the backend validates
  contents independently every time (§8-§9).
- **Archive root contents:** exactly `manifest.json`, `template.json`, and
  zero or more entries under `assets/`. No other root file, no executable
  metadata, no nested archive of any kind is accepted anywhere in the
  archive.

## 5. Package manifest v1 (`manifest.json`)

Strict JSON, unknown fields rejected. Top-level shape:

```json
{
  "format": "streaming-tree-template-package",
  "schemaVersion": 1,
  "templatePath": "template.json",
  "assets": [
    {
      "id": "pkgasset_a1b2c3d4",
      "path": "assets/pkgasset_a1b2c3d4.png",
      "kind": "image",
      "mediaType": "image/png",
      "sha256": "…64 hex chars…",
      "sizeBytes": 12345,
      "displayName": "Corner badge",
      "author": "",
      "license": "",
      "notice": ""
    }
  ]
}
```

- `format` must equal `streaming-tree-template-package` exactly.
- `schemaVersion` must equal a version this build actually supports (1
  today); an unrecognized version is rejected with
  `visual_template_package_version_unsupported`, never guessed at or
  silently coerced.
- `templatePath` must equal the literal string `template.json`.
- `assets` is a bounded array (§10); each entry carries only the eight
  fields above. There is deliberately **no** URL field, no source-download
  URL, no local absolute path, no OS path, no application database id, no
  public token, and no required timestamp - a manifest asset entry
  describes archive-local content, never a live reference to anything
  outside the archive.
- `displayName`/`author`/`license`/`notice` are bounded plain text (§16) -
  never URLs, never HTML.

## 6. Package logical asset IDs are archive-local, never trusted as database IDs

A manifest asset `id` is an archive-local logical identifier in the ASCII
form `pkgasset_<short-random-or-deterministic-id>` - bounded, ASCII,
independent of archive entry ordering, unique only within that one package.
It is never persisted as, or treated as equivalent to, a local database
primary key.

- **On import:** the backend generates fresh local asset IDs
  (`asset_<random>`, §17) for every accepted package asset, and rewrites
  every package-local asset reference inside the imported `template.json`
  document to the corresponding new local ID before that document is ever
  persisted. A package-supplied `id` is never written into `visual_assets`.
- **On export:** local asset IDs are remapped to freshly generated,
  deterministic package-local IDs, and the exported document copy is
  rewritten to reference those package-local IDs instead. The stored
  template/design is never mutated by an export.

Semantic round-trip equality matters (§20); the specific local or
package-local ID values used along the way do not.

## 7. Archive internal paths - a narrow ASCII grammar, not arbitrary filenames

An archive entry's path is untrusted input, never a filesystem destination.
Valid entry paths match one of exactly:

- `manifest.json`
- `template.json`
- `assets/<segment>` where `<segment>` is a bounded ASCII filename with an
  extension matching the entry's own validated content (§11).

An entry is rejected, and the whole package is rejected, if its path:

- is absolute, or begins with `/` or `\`,
- carries a Windows drive prefix (`C:`, …) or is UNC-shaped (`\\…`),
- contains **any** backslash at all,
- contains a `.` or `..` path segment, or an empty segment (`//`),
- contains a NUL byte or other control character,
- uses an ambiguous or non-canonical path-separator encoding,
- falls outside the exact allowed root structure above,
- has a trailing dot or trailing space on any segment (Windows path
  ambiguity),
- matches a Windows reserved device name (`CON`, `PRN`, `AUX`, `NUL`,
  `COM1`-`COM9`, `LPT1`-`LPT9`) in any segment, case-insensitively,
- duplicates another entry's path after normalization, including a
  case-insensitive duplicate.

The untrusted archive path is **never** joined to a real filesystem
directory and written to (§9). `internal/runtime/mediamtx/archive.go`'s own
`safeEntryPath` rejection logic (absolute/drive-letter/`..`-segment checks)
is a useful conceptual precedent, but this package's own reader implements
its own, stricter, purpose-built grammar rather than reusing that file's
extraction technique directly - see §9 for why.

## 8. Symlinks, hard links, and special files are always rejected

An archive entry is accepted only if it is a regular file, or (if the
writer chooses to emit one at all) the literal `assets/` directory entry.
Rejected outright: a symlink, a hard-link-like special entry, a device
node, a socket, a FIFO, any entry whose external file attributes encode
Unix "special" mode bits, and any other unknown special mode. The package
reader does not require explicit directory entries to exist at all - a
well-formed archive may omit them entirely.

## 9. Archive validation pipeline - never blind extraction

The following is the one anti-pattern this stage's reader must never
implement, regardless of precedent elsewhere in the codebase:

```
zip entry path
  -> filepath.Join(destination, entry.Name)
  -> write
```

Instead, the package reader validates the archive as a whole before
extracting anything from it:

1. open the archive and read its central directory only,
2. validate every entry's metadata (path grammar §7, mode §8, declared
   size) against the complete allowed entry set - reject the whole package
   on the first violation,
3. identify `manifest.json`, `template.json`, and every `assets/…` entry
   logically, by validated path, never by iteration order,
4. stream each accepted asset entry through the size/hash/type pipeline
   (§10-§11) with a bounded reader - never load an entire entry into memory
   at once,
5. write each verified asset only to an application-generated temporary
   filename (never derived from the entry's own archive path),
6. promote verified files into the managed asset store (§17) under an
   application-generated, content-addressed path,
7. only after every asset is verified and staged does the reader parse and
   validate `template.json` itself and rewrite its package-local asset
   references to local IDs (§6).

The untrusted archive path is used only as a lookup key against the
already-validated allowed-entry set; it never becomes, or contributes to,
a real filesystem destination path.

## 10. Bounds

Initial policy, enforced on both declared metadata **and** actual streamed
bytes - a ZIP header's declared size is never trusted alone:

| Bound | Value |
| ----- | ----- |
| Max compressed package body | 96 MiB |
| Max total uncompressed bytes | 128 MiB |
| Max archive entries | 64 |
| Max assets | 32 |
| Max `manifest.json` size | 64 KiB |
| Max `template.json` size | 128 KiB |
| Max single image asset | 16 MiB |
| Max single video asset | 64 MiB |
| Max single font asset | 8 MiB |
| Max decompression ratio, per entry and in aggregate | 100:1 |
| Max asset metadata text field (`displayName`/`author`/`license`/`notice`) | 200 code points each |

A conservative, narrower policy is always acceptable if a real technical
reason requires it; this table is not broadened casually. Reading uses
counted, streaming reads throughout - the reader never allocates a buffer
sized from a ZIP header's `UncompressedSize64` field, and never reads a
whole package into one unbounded byte slice.

## 11. Allowed visual asset types

Only these families are accepted in Stage 14B v1:

| Kind | Formats | Signature check |
| ---- | ------- | ---------------- |
| `image` | PNG, JPEG, GIF (87a/89a), WebP | PNG signature; JPEG SOI marker; `GIF87a`/`GIF89a` header; RIFF container + `WEBP` fourcc |
| `video` | WebM, MP4 | WebM EBML header; MP4 `ftyp` box |
| `font` | WOFF2 only | `wOF2` signature |

Explicitly excluded, and rejected even if a manifest or upload claims one
of these kinds: SVG/SVGZ (active/XML content, unnecessary for this stage),
BMP, TIFF, ICO, PSD, PDF, HTML, CSS, JavaScript, XML, any executable
format, DLL, any archive format, any audio format, TTF, OTF, and WOFF
(version 1). A narrower list than this table is acceptable if a real
technical incompatibility is found during implementation; the list is not
broadened beyond it without a new, separately justified requirement.

**Independent triple validation.** A file's extension, its manifest/
upload-declared `mediaType`, and its own magic-byte signature must all
agree. A mismatch on any axis is rejected with
`visual_asset_type_mismatch` (or the package-scoped equivalent, §21) -
never resolved by trusting whichever of the three looks most convenient. A
file beginning with a known executable, script, or archive signature is
rejected even if every other field claims it is an image.

**Image dimensions.** Width ≤ 8192, height ≤ 8192, total pixels ≤ 32
megapixels, read via safe header/metadata decoding only (`image.
DecodeConfig` for PNG/JPEG/GIF) - never full-frame decoding merely to
validate a file. WebP dimension validation uses a small, reviewed
header-only reader; a full image-processing dependency is not added purely
for this check.

**Video.** A visual primitive only. The renderer fixes: autoplay (subject
to the reduced-motion policy below), muted, `playsInline`, no controls, an
optional closed `loop` boolean, and no user-configurable volume or audio
output - a video's own embedded audio track, if any, is never played.
Validation checks container signature, extension/mediaType agreement, and
size bounds only; it never shells out to FFmpeg/ffprobe, since the
asset/template system must keep working even when an operator has not
installed FFmpeg. A structurally valid container with an undecodable codec
fails safely at render time (the layer is hidden/shows a nonfatal
media-load state) rather than crashing the overlay; this project does not
claim codec-level OBS compatibility has been manually verified (§22).

**Animated images and reduced motion.** GIF and animated WebP remain plain
`image` assets - there is no separate animated-image code path. Under
`prefers-reduced-motion`, video never autoplays, and a known-animatable
image asset (GIF, or any WebP - see below) uses a deterministic
reduced-motion fallback, preferring to hide the layer if no safe static
first-frame representation is available. Reliable animation detection for
WebP without a full decoder is not implemented in this stage, so the
conservative rule is: **treat every WebP asset as potentially animated for
reduced-motion purposes**, exactly like GIF, rather than silently ignoring
the accessibility requirement for the subset that happens to be static.
Thumbnails/first frames are never generated via FFmpeg.

**Fonts.** WOFF2 only. A font's own internal family name is never trusted
as CSS input; the application assigns a deterministic, internal renderer
family name per managed font asset and loads it via the browser `FontFace`
API (or an equivalent app-controlled mechanism) - `font-family: <arbitrary
user CSS>` is never persisted or emitted. A visual design persists a
system fallback font from the existing closed `FontFamily` allowlist plus
an optional managed font asset reference; if the custom font fails to
load, the renderer falls back to the saved system font and the overlay
stays readable.

## 12. Visual-design document version 3

`internal/domain/visualdesign` gains, in version 3:

- two new closed layer kinds, `image` and `video`, each carrying only a
  managed asset reference plus a small closed set of presentation fields
  (fit enum `contain`/`cover` for both; an additional closed `loop`
  boolean for video),
- an optional managed custom-font asset reference on `TextProps` and
  `MessageFragmentsProps`, alongside (never replacing) the existing
  `FontFamily` system-fallback enum.

Deliberately **not** added: an arbitrary `object-fit` string, a CSS class,
raw HTML, a URL field of any kind, a source-set, a filter/shader/blend-mode
field, or an event callback. The existing frame/opacity/order/animation
model continues to control layout and transitions for these two new kinds
exactly as it already does for the other six.

An asset reference stored in a document is an **opaque managed asset ID**
- never a filesystem path, `http://`/`https://` URL, `file://` URL, `blob:`
URL, or `data:` URL. Two independent validation layers apply: (1) generic
structural validation in `visualdesign.Validate` (the reference is a
well-formed local ID, the fit/loop fields are within their closed
enums/bounds), and (2) asset-existence/kind-match validation in the owning
service, which confirms the referenced ID actually exists in the managed
asset store and is the kind (`image`/`video`/`font`) the layer expects. For
a portable package, references are package-local (§6) and are validated
against the manifest before ever being rewritten to local IDs.

`MigrateToCurrentVersion` gains an explicit Version2→Version3 step,
chained after the existing Version1→Version2 step so a Version1 document
still migrates correctly all the way to Version3. The 2→3 step is also
lossless and relabel-only: no Version2 document could ever have populated
an `image`/`video` layer or a font-asset reference (they did not exist
yet), so migration never invents an asset reference - every existing
alert/chat design keeps rendering identically after migration, proven the
same way Version1→Version2 already is
(`visualdesign/migration_test.go`).

## 13. Managed asset persistence (local implementation detail, schema D)

Conceptual model, implemented as ordinary SQLite migrations:

```
visual_asset_blobs           (sha256 PK, media_type, byte_size, storage_name, public_token, created_at)
visual_assets                (id PK, blob_sha256 FK, kind, display_name, author, license, notice, source, created_at, updated_at)
visual_design_asset_refs     (design_id, asset_id)
visual_template_asset_refs   (template_id, asset_id)
```

- **Blob/metadata separation.** Immutable binary bytes live in
  `visual_asset_blobs`, addressed by their own SHA-256, never in a SQLite
  column - no asset blob column is added to SQLite; bytes live only in the
  filesystem-backed asset store (§14).
- **Deduplication is content-only.** Identical bytes (same SHA-256) share
  one blob row, since a package may repeat an asset, a design and a
  template may reference the same bytes, and repeated imports commonly
  carry identical content. Metadata is **not** deduplicated by content
  alone: two logical `visual_assets` rows with identical bytes may carry a
  different `display_name`/`author`/`license`/`notice` and are never
  silently merged into one logical asset just because their blobs match.
- **Local asset IDs.** `asset_<random>`, generated only by the server,
  never accepted as caller input, never equal to a package-supplied
  logical ID (§6). Frontend management surfaces may receive a local asset
  ID; the public Browser Source surface never does (§18).
- **Reference tracking.** `visual_design_asset_refs`/
  `visual_template_asset_refs` record which saved designs/templates
  actually use which logical asset, used both for the "in use" deletion
  guard (§15) and for future safe garbage collection.

## 14. Asset store location and atomic installation

Conceptual location: `<app-data>/assets/visual/`, a sibling of the
existing `<app-data>/runtime/` convention (`internal/config.Config.
DataDir`, `internal/runtime/mediamtx/resolver.go`'s own `filepath.Join
(dataDir, "runtime")`). Only application-generated filenames are ever
used - the recommended blob filename is the asset's own SHA-256 hex; an
imported archive's own path, or a browser-supplied full local path from a
manual upload, is never preserved as a storage path.

Atomic installation, in order:

1. stream the incoming bytes into an app-owned temporary file, bounded by
   the relevant size limit (§10),
2. compute SHA-256 while reading,
3. verify size, type, and hash together,
4. `fsync`/close the temporary file,
5. atomically rename it into its final content-addressed blob location,
6. only then create or update the corresponding SQLite rows.

This gives a safe crash model: **files first, database second.** If the
process dies after step 5 but before step 6 commits, the blob file is an
orphan with no matching database row - existing configuration remains
entirely correct, and the next clean-startup reconciliation pass (§16) may
remove the orphan. A database row referencing bytes that were never safely
placed is never committed.

## 15. Deletion, blob garbage collection, and runtime snapshot safety

A logical asset (`visual_assets` row) may be deleted immediately once
safe:

- **Manual/API delete** is rejected with `409` and stable error
  `visual_asset_in_use` if any persisted design or template still
  references it (via the reference-tracking tables, §13). The management
  API may expose a reference count; it never exposes *which* private owner
  (which alert rule, which chat overlay) uses an asset through a public
  endpoint.

Physical blob bytes are a different, more conservative story. Stage 12/13
alert/chat snapshots may still be displaying an old design - including one
that referenced an asset whose *last persisted reference* has just been
removed - for the remaining lifetime of an already-queued or already-live
alert. Therefore: **a blob is never physically deleted merely because its
last logical reference disappeared during a running server process.**
Logical deletion may happen immediately; physical blob garbage collection
happens only on a later clean startup/maintenance pass, once no runtime
alert snapshot from the *previous* process can still exist. This mirrors
the same reasoning that already governs in-memory alert snapshot lifetime
elsewhere in the codebase, applied here to on-disk blob bytes instead.

## 16. Startup asset-store reconciliation

Before normal use, on every startup:

- verify the managed asset directories exist and are safe to use,
- remove any expired preview-staging session (§19) older than its TTL,
- detect a `visual_asset_blobs` row whose backing file is missing and
  report it as a safe, non-fatal diagnostic - the relevant design/template
  reports an asset error/fallback rather than failing to load at all,
- detect an untracked orphan blob file (no matching database row) and
  remove it,
- remove any blob row/file that has zero references in *both* reference-
  tracking tables,
- never follow a symlink, and never recursively delete anything outside
  the exact managed asset/preview roots.

A broken individual asset must never prevent the rest of the database from
being read.

## 17. Manual asset upload

An operator must be able to use a custom asset without hand-building a
ZIP. Management-only upload flow: choose a local file through a normal
file picker (no URL import, no clipboard-executable handling, no
drag/drop-only affordance - a picker always exists), the backend validates
it with the exact same binary validator package import uses (§9-§11),
optional bounded metadata (`displayName`/`author`/`license`/`notice`) is
attached, the result is stored as a managed asset, and it becomes
selectable in either Designer.

Upload transport uses a strict multipart contract: `http.MaxBytesReader`
first, `multipart.Reader` streaming (never unbounded `ParseMultipartForm`
temp-file spilling), exactly one binary file part, bounded metadata text
fields, and rejection of any unrecognized part. An equally strict
alternative streaming contract is acceptable if it proves clearer during
implementation; an unbounded or filename/path-trusting contract is not.

## 18. Public asset serving

A managed asset's local ID, blob hash, and filesystem path are never
exposed on a public Browser Source route. Each immutable blob instead
receives a random, high-entropy `public_token`, generated locally and
never accepted from an imported package or any other caller input, served
conceptually at:

```
GET /api/public/visual-assets/{publicToken}
```

Response headers: the exact validated `Content-Type`, `Content-Length`,
`X-Content-Type-Options: nosniff`, and `Cache-Control: public,
max-age=31536000, immutable` (a blob token's content never changes once
issued). Video benefits from HTTP byte-range support; a correct, bounded
single-range implementation is preferred - `200` for a full request, `206`
with `Content-Range`/`Accept-Ranges: bytes` for a valid single
`Range: bytes=…` request, `416` for an invalid/unsatisfiable range - over
an incorrect or partial one. No directory listing is ever possible. An
unknown token returns a normal `404`; a wrong method returns the project's
existing `405` + `Allow` convention, never a raw filesystem or parser
error.

A public token behaves like this project's existing public overlay slugs -
an unguessable locator for immutable presentation content, not an account
credential - and is redacted from logs the same way (§23). `visualdesign.
ToPublic`/the public presentation resolution step is what converts a
managed asset reference into this safe, app-owned public URL; the
persisted document itself never contains a URL (§12).

**Public/private DTO split.** A management response may expose a local
asset ID, its metadata, and a management-only asset content URL. A public
document exposes only the safe app-owned content URL and the visual
properties a layer needs (fit, loop) - never a local asset ID, hash,
package ID, local filename, author/license/notice metadata (unless a later
explicit feature intentionally surfaces attribution publicly, which this
stage does not), path, original upload filename, or database timestamp.

## 19. Package import preview - two-step, exact

Stage 14A's own "preview before import" pattern is reused, extended to
cover assets:

1. the operator uploads a package to an import-preview endpoint,
2. the backend fully validates the archive, manifest, template document,
   and every asset (§9-§12), staging verified asset bytes under
   `<app-data>/tmp/template-previews/<random-token>/` - **not**
   persistence; nothing is written to the normal template/asset tables,
3. a temporary preview session is exposed, scoped to that random,
   high-entropy preview token,
4. the UI shows template metadata, target, the asset list with each
   asset's kind/media type/size, license/author/notice attribution, a
   compatibility assessment, and a real `VisualDesignRenderer` preview
   using the staged assets' preview-scoped URLs,
5. the operator explicitly confirms Import,
6. the **actual** import re-uploads and fully re-validates the original
   package bytes from scratch - a preview token is never trusted as proof
   that a later "confirm" request's bytes are identical to what was
   previewed.

Preview-staging rules: bounded concurrent sessions, a bounded total staged
byte count, a TTL of roughly 10 minutes, best-effort deletion on confirm or
cancel, deletion on expiration, and unconditional removal of every leftover
preview session on the next startup (§16). No staged entry ever uses the
archive-supplied filesystem path as its own path. The preview route is a
management-API route, never reachable from the public OBS Browser Source
surface.

## 20. Package import and export persistence semantics

**Import.** A successful import creates exactly: one new user-owned
visual template, one or more managed logical assets (deduplicated at the
blob level, §13), and the template→asset reference rows tying them
together. It does **not** alter any alert rule, alter any chat overlay,
save any owner visual design, emit a public alert/chat presentation event,
or automatically apply the template as a draft. After import, the template
appears in *My templates* exactly like a Stage 14A JSON import; the
operator may choose *Use as draft*, and the Designer's own Save remains the
only action that ever activates an owner design - Stage 14A's draft-first
semantics are unchanged (`docs/visual-templates.md` §10).

Deleting an imported template does **not** cascade-delete its assets - an
asset may be reused elsewhere, and there is no provenance coupling between
a template and the assets it once imported (mirroring §11 of
`docs/visual-templates.md`: no provenance link, ever). An asset may be
deleted explicitly later, once the reference-tracking guard (§15) allows
it; an unused imported asset row surviving until an operator deletes it is
expected, and preferred over surprise deletion.

**Save as template with assets.** Both Designers' existing "Save as
template" action gains asset support: every asset reference in the
document being saved is validated, the template document is persisted, and
template→asset reference rows are created - no blob bytes are copied, and
the owner design being saved from is never changed by this action.

**Export.** "Export package" is added for both built-in and user
templates. An asset-free template exports as a valid package with zero
assets. An asset-backed template's package contains the manifest,
`template.json`, and **exactly** the assets the document transitively
references - never every asset in the operator's library. Local managed
asset IDs are remapped to package-local logical IDs (§6) and the exported
document copy is rewritten accordingly; the stored template itself is
never mutated by an export. Construction prefers determinism where the ZIP
library allows it (assets sorted by a stable key, entries written in a
stable path order, stable archive timestamps rather than wall-clock where
supported) - equivalent metadata/document/assets should produce
semantically equivalent packages; byte-for-byte determinism is not claimed
unless it is actually tested and verified.

**Filename.** A safe filename is derived from the template's own name,
with the fixed `.streaming-tree-template` extension: slash, backslash,
colon, control characters, and CR/LF are stripped or replaced, length is
bounded, no user-controlled path is ever used, and no value is placed
anywhere that could enable `Content-Disposition` header injection.

**Round trip.** Export → delete the template → import the resulting
package → new local template/assets must be semantically equal to the
original: local template ID, local asset IDs, timestamps, and blob public
tokens are all expected to differ; asset bytes must match by SHA-256; every
other portable field must match exactly.

## 21. Stage 14A JSON compatibility

Stage 14A's asset-free `.streaming-tree-template.json` format is
unchanged and remains fully supported for templates that reference no
managed asset:

- **JSON export** of a template whose document references any managed
  asset fails with the stable error `visual_template_requires_package_export`;
  the frontend explains that the template contains assets and must be
  exported as a package instead. An asset-free template exports as JSON
  exactly as it always has.
- **JSON import** of a standalone template file is unchanged for an
  asset-free document. A JSON template whose document references a
  managed asset is rejected with the stable error
  `visual_template_assets_missing` - asset references are never resolved
  by coincidence merely because a same-named or same-ID local asset
  happens to already exist; a JSON file simply has no channel for
  transporting the bytes an asset reference depends on.

## 22. OBS Browser Source contract

Browser Source URLs are unchanged by this stage. No new OBS permission is
introduced, and `window.obsstudio` is never used. A custom asset URL is an
ordinary app-owned, same-origin HTTP resource, resolved exactly like every
other public overlay resource today. `docs/obs-browser-source.md` gets a
factual Stage 14B note, added only after implementation: image/video/font
behavior is covered by normal browser/jsdom/integration test paths, but
codec-level video compatibility inside the real OBS CEF browser source
remains manually unverified until a final manual test pass - this document
does not claim otherwise.

## 23. Privacy and logging

Never logged: asset bytes, a raw package body, preview contents, static
design/template text, a local public token, an archive content dump, or an
original local upload path. Operational logs may include: package byte
count, entry count, asset count, a local asset ID, an asset kind, a media
type, a SHA-256 hash when genuinely useful for diagnosing a specific
failure, and a stable validation error code. A public token is never
logged, matching the existing public overlay slug redaction convention.

## 24. Stable error codes

At minimum, in addition to Stage 14A's existing set
(`docs/visual-templates.md` §18):

`visual_asset_not_found`, `visual_asset_invalid`,
`visual_asset_unsupported`, `visual_asset_too_large`,
`visual_asset_type_mismatch`, `visual_asset_in_use`,
`visual_template_package_invalid`, `visual_template_package_too_large`,
`visual_template_package_version_unsupported`,
`visual_template_package_entry_invalid`,
`visual_template_package_too_many_entries`,
`visual_template_package_decompression_limit`,
`visual_template_package_asset_missing`,
`visual_template_package_asset_unreferenced`,
`visual_template_package_asset_hash_mismatch`,
`visual_template_package_asset_type_mismatch`,
`visual_template_package_asset_unsupported`,
`visual_template_requires_package_export`,
`visual_template_assets_missing`,
`visual_template_package_preview_expired`.

Archive-parsing/entry-grammar failures and owner-binding compatibility
failures use separate stable error families rather than being mixed
together, matching Stage 14A's own existing compatibility-vs-validation
split. A raw ZIP/parser/filesystem error is never surfaced to a caller
directly.

## 25. What Stage 14B explicitly does not implement

- No template/visual-layer sound or audio playback of any kind (§2) - that
  is Stage 17's decision to make, on its own audio subsystem.
- No update checking, GitHub networking, release download, installer
  launch, or restart logic - that is the Stage 20 target documented in
  `docs/project-overview.md` §12.1.1, written alongside this stage but not
  built by it.
- No SVG, HTML, CSS, JavaScript, or any executable/archive/audio asset
  kind, in a package or via manual upload (§11).
- No public exposure of asset attribution metadata (author/license/notice)
  on the public overlay surface - management-only for this stage.
- No content-delivery network, no remote asset fetching of any kind - every
  managed asset is stored and served locally, same-origin, by this
  application.
- **No `Content-Security-Policy` header on any page or response.** This was
  audited as part of Stage 14B and deliberately deferred rather than shipped
  half-right: no CSP exists anywhere in this application today (audited
  across `internal/httpapi`), and a narrow, source-backed policy cannot be
  written honestly right now, because the public alert and chat overlay
  pages this stage's own image/video/font assets render on are the same
  pages Stage 13B's `message_fragments`/`badge_list` layers already load
  third-party emote and badge images onto from multiple external providers
  (Twitch's own CDN today, with 7TV/BTTV/FFZ-style providers a realistic
  future addition per `internal/provider/twitch/chatassets`) - none of
  which this codebase currently enumerates as a closed, stable allowlist.
  Writing a CSP `img-src`/`media-src` directive today would mean either
  guessing at that host list (risking silent breakage the moment a
  provider's CDN hostname changes) or falling back to a wildcard broad
  enough to defeat the policy's own purpose - both rejected as a fake or
  incomplete CSP. This stage's own new surface (managed images/video,
  custom fonts, package import) is same-origin only and adds no new
  external host, so it does not make the existing gap any worse. Tracked
  as a named hardening item for Stage 20 (`docs/project-overview.md` §13
  roadmap: "Logs, diagnostics, packaging and remote-server hardening"),
  once every trusted external image host across every provider
  integration can be enumerated and pinned in one pass instead of
  piecemeal per stage.
