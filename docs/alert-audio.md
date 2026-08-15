# Stage 17B — persistent alert audio, per-alert-rule TTS, and alert/audio synchronization

This is the canonical Stage 17B contract, written before any product code in
this milestone, exactly as [`docs/audio-tts.md`](audio-tts.md) preceded Stage
17A. It records the primary-source research this milestone's format decision
rests on, resolves every open question Stage 17A and Stage 14B deliberately
left for "Stage 17B's own, later, separately-scoped decision," and the
runtime/protocol contracts every later commit in this milestone implements
against.

Stage 17B closes Stage 17 as a whole. It does **not** start Stage 18
(goal/counter widgets).

## 0. Non-negotiable constraints, restated

- **One audio subsystem.** Stage 17B extends `internal/audio` (the Stage 17A
  shared runtime, its single public Browser Source, its SSE + short-lived
  bytes-URL + ACK protocol, and the Stage 17A `tts.Provider` abstraction). It
  never builds a second playback engine, a second public audio renderer, or a
  second queue.
- **No audio visual-design layer.** `visualdesign.Document` stays at version
  3. Alert audio is presentation *behavior* tied to a rule/template, never a
  geometric canvas layer next to `image`/`video`/`text`.
- **Stage 14A JSON stays visual-only.** The asset-free
  `streaming-tree-visual-template` JSON schema (`schemaVersion: 1`) is
  unchanged and never gains an audio-carrying field.
- **Package v1 stays valid.** Every `.streaming-tree-template` package
  written before this milestone still imports identically.

## 1. Research method

Every external claim below was checked against the primary/official source
listed - the OBS Project knowledge base, the Chromium project's own media
documentation, and MDN's browser-compatibility guides (which cite the
underlying WHATWG/W3C specifications). Where the official sources did not
explicitly prove something this stage needs, that is stated honestly rather
than assumed - see §2.4.

## 2. Persistent audio format research

### 2.1 OBS Browser Source's own documentation

Source: <https://obsproject.com/kb/browser-source> (fetched). Confirms, as
already recorded in `docs/audio-tts.md` §18: Browser Source is CEF-based and
"custom layout, image, video, and even audio tasks" are explicitly listed as
supported general capabilities. **Not documented**: any specific audio
container/codec support list, any autoplay policy, any "Control audio via
OBS" property behavior, or CEF audio-routing/mixing configuration. This gap
is carried forward from Stage 17A unchanged - no new claim is made here.

### 2.2 Chromium's own codec documentation - the decisive finding

Source: <https://www.chromium.org/audio-video/> (fetched), the Chromium
project's own page about its media support, which OBS's own CEF build is
compiled from.

Confirmed facts, load-bearing for this stage's format decision:

- **WAV is supported by default.** The page lists `WAV` as a supported
  container, with audio codecs `PCM 8-bit unsigned integer`, `PCM 16-bit
  signed integer little endian`, `PCM 32-bit float little endian`, and `PCM
  μ-law` - all in Chromium's own baseline, unconditional codec set.
- **MP3 (and AAC) are *not* in that baseline set.** The page places `MP3`
  under a section titled "Proprietary Audio Codecs (Limited to Google
  Chrome)" - gated behind a build-time `ffmpeg_branding` flag that can be set
  to `Chrome` ("includes additional proprietary codecs (MP3, etc.) for use
  with Google Chrome") versus the open-source project's own default
  `Chromium` branding ("builds default set of codecs" - i.e. **without** the
  proprietary additions).
- **This directly answers the question this stage actually needs answered**:
  not "does *Google Chrome* play MP3" (widely true, and irrelevant here), but
  "does the open-source Chromium codebase that CEF - and therefore OBS
  Browser Source - is built from play MP3 *by default*." The primary source
  says no, unless OBS's own CEF build was specifically compiled with
  `ffmpeg_branding=Chrome`, which is not documented anywhere OBS publishes
  and cannot be verified from this project's own vantage point. Claiming MP3
  support would be exactly the "every browser supports this" folklore this
  task's own instructions warn against.

### 2.3 Format decision

**Stage 17B accepts exactly one persistent audio format: WAV (RIFF/WAVE
container, canonical PCM format tag, 16-bit signed little-endian samples, 1
or 2 channels, 8000-192000 Hz sample rate).** Every other container/codec
(MP3, AAC, Ogg/Vorbis, Ogg/Opus, WebM/Opus, FLAC, ADPCM, μ-law, 8-bit PCM,
32-bit float PCM) is explicitly rejected in this stage, even though some of
those (Opus, Vorbis) are themselves royalty-free and Chromium-default per
§2.2 - narrowed further, deliberately, for three independent reasons:

1. **Validation stays trivial and safe.** A canonical-PCM WAV file is fully
   describable by its RIFF/`fmt `/`data` chunk headers - no codec/entropy
   decoding is ever needed to prove the file is well-formed, compute its
   exact duration, or reject a hostile file. Opus/Vorbis validation would
   require parsing an Ogg or WebM container plus understanding codec-specific
   framing, a meaningfully larger safe-parsing surface for no format-choice
   benefit this stage needs.
2. **No new dependency.** 16-bit PCM WAV is exactly what `internal/provider/
   tts`'s own Windows SAPI provider and integration fake already generate and
   serve (`wrapPCMAsWAV`/`wrapFakePCMAsWAV`, both already tested and proven
   working end-to-end) - Stage 17B's asset validator is a *reader* for the
   same shape Stage 17A's provider already *writes*, using only
   `encoding/binary` from the standard library, never a new Go module.
3. **Homogeneity.** Every audio byte this whole subsystem ever serves -
   synthesized speech and persistent alert sounds alike - is the same
   container/codec, so the public bytes route (§7 of `docs/audio-tts.md`,
   unchanged by this stage) never needs format-specific branching.

It is explicitly acceptable, per this task's own instruction, that this
narrows out common real-world "alert sound" distributions (many free sound
packs ship as MP3 or Ogg) - an operator must convert to 16-bit PCM WAV before
uploading. No server-side transcoding is offered (§2.4).

### 2.4 What is explicitly rejected, and why

- **No FFmpeg/ffprobe shell-out**, ever, for audio validation or
  transcoding. Stage 17B must work identically whether or not FFmpeg is
  installed - it validates structure and duration from the WAV header alone.
- **No arbitrary remote audio URL.** Every persistent sound is an uploaded,
  managed, locally-stored asset; a rule references it only by opaque local
  asset ID (§6), exactly like Stage 14B's own `image`/`video`/`font`
  references.
- **No HTML, JavaScript, CSS, SVG, XML-driven active content, playlist
  format (M3U/PLS/etc.), archive-masquerading-as-audio, executable, shell
  script, or filesystem path accepted as a playback reference** - the asset
  validator's signature/chunk parser only ever recognizes the one closed WAV
  shape from §2.3; anything else fails `audio_asset_unsupported` or
  `audio_asset_type_mismatch` before a single byte is trusted.
- **No new Go/npm dependency was needed** for this milestone's audio-format
  validation (a plain RIFF/WAVE chunk parser is under ~150 lines of ordinary
  Go using only `encoding/binary`/`bytes`); `THIRD_PARTY_NOTICES.md` is
  updated only if implementation genuinely needs a new dependency for an
  unrelated concern (§10's package v2 ZIP handling reuses the existing
  `archive/zip` standard-library reader Stage 14B's own `visualpackage`
  already uses - no new dependency there either).

## 3. One audio subsystem (task §4.1)

Persistent sounds and synthesized speech become **two input forms feeding
the same `internal/audio.Manager` queue/promotion/synthesis/playback
pipeline**, distinguished by a new closed `Source` classification on a
queued item (§8), never two separate runtimes:

```go
type Source int

const (
    SourceGlobalTTS     Source = iota // Stage 17A: Event-Bus-driven, eligibility/cooldown/manual-approval gated
    SourceAlertSound                  // Stage 17B: a rule's persistent sound asset, played verbatim
    SourceAlertTTS                    // Stage 17B: a rule's own TTS text template, synthesized via the same Provider
)
```

`internal/audio.Manager` gains exactly one new class of entry point -
`EnqueueAlertAudio` (§8) - alongside its existing Event-Bus subscription and
`TestSpeak`. Nothing about the existing public route
(`/overlay/audio/{publicSlug}`), the SSE event names/payload shapes, the
bytes-URL/Range serving, or the single-renderer-session ACK protocol changes
shape. A Browser Source already pointed at the public audio route requires
zero configuration change to also render alert-owned audio - it is the same
stream.

## 4. Audio assets are not visual-design layers (task §4.2)

Confirmed, no exception: `visualdesign.Document.Version` stays `3`.
`internal/domain/visualdesign` gains no `audio` `LayerKind`, no audio field on
any existing layer, and `visualtemplate.CurrentTemplateSchemaVersion` (the
Stage 14A JSON schema counter, currently `1`) is untouched.

Alert-owned audio configuration is a property of the **alert rule itself**
(§7) and, for template/package purposes, of a new package-level structure
(§10) - never of the rendered visual canvas. An alert rule may reference a
visual design (Stage 13A) and independently reference alert-owned audio
(Stage 17B); the two are sibling concerns joined only by sharing the same
`Rule.ID`, exactly the way a rule's `TextTemplate` (Stage 12A) already
coexists with its optional saved `visualdesign.Record` today without either
one containing the other.

## 5. Managed audio-asset domain (task §4.3)

### 5.1 Package choice

A new package, `internal/domain/audioasset`, mirrors `internal/domain/
visualasset`'s own proven shape (blob/metadata split, content-addressed
storage, reference tracking, delayed orphan GC) rather than either widening
`visualasset` into a vague "any asset" package or duplicating its logic
inline. `visualasset`'s own package doc comment already states its scope
"deliberately excludes... audio assets of any kind (Stage 17 owns the
application's one audio/playback subsystem)" - `audioasset` is that
separately-scoped decision, made now.

**Blob storage reuse.** `visualasset.FileStore` (`blobstore.go`) is generic
over `io.Reader`/byte-count/SHA-256 - it has no `Kind`/`MediaType` awareness
anywhere in its own implementation. Stage 17B instantiates a **second**
`*visualasset.FileStore`, rooted at a sibling directory (`<app-data>/assets/
audio/`, alongside the existing `<app-data>/assets/visual/`), reusing the
exact same `WriteBlob`/atomic-rename/dedup-by-hash primitive rather than
copying it - this is the one piece of Stage 14B machinery genuinely narrow
and generic enough to share directly, per this task's own explicit
allowance. `audioasset` never imports `visualasset`'s `Kind`/`MediaType`/
`Asset`/`Repository` types, only its exported `FileStore`.

### 5.2 Domain model

```go
package audioasset

type Kind string
const KindSound Kind = "sound" // the only kind Stage 17B defines

type MediaType string
const MediaWAV MediaType = "audio/wav" // the only media type Stage 17B accepts

type Blob struct {
    SHA256      string // hex, primary key
    MediaType   MediaType
    ByteSize    int64
    DurationMS  int64  // computed from the WAV header alone, never estimated
    StorageName string // == SHA256 hex, the FileStore's own blob filename
    PublicToken string // 32 random bytes hex; unused until/unless a future
                        // stage needs direct public byte access - the
                        // Stage 17B renderer always reaches audio bytes
                        // through internal/audio's own existing
                        // /api/public/audio/{slug}/bytes/{token} route
                        // (§8), never a second public asset route
    CreatedAt   time.Time
}

type Asset struct {
    ID          string // "audioasset_" + 16 random bytes hex
    BlobSHA256  string
    Kind        Kind
    DisplayName string // bounded plain text, 200 code points, like visualasset
    Source      string // "upload" (this stage defines no "package" origin
                        // distinct from upload - see §10.4)
    CreatedAt, UpdatedAt time.Time
    Blob        *Blob // resolved on read only, mirrors visualasset.Asset
}
```

### 5.3 Validation

- **Asset ID format**: `audioasset_` + 16 random bytes hex, generated only
  server-side, never accepted as caller input - mirrors `visualasset.
  NewAssetID`'s own convention exactly (distinct prefix, same entropy).
- **Content SHA-256**: computed while streaming into the temp file, exactly
  like `visualasset.FileStore.WriteBlob`; verified against nothing external
  (no signature/checksum is supplied by the uploader - the hash *is* the
  content address, not a claim to be checked against).
- **Deduplication**: content-only, by SHA-256, exactly like `visualasset`
  (§5.1) - two `Asset` rows may share one `Blob` row; `DisplayName` is never
  deduplicated merely because bytes match.
- **Per-file byte limit**: `MaxSoundBytes = 8 MiB`. At 16-bit PCM, even mono
  48 kHz that bounds an uploaded clip to roughly 87 seconds - generous for
  any realistic alert sound, and small enough that a hostile "sound" upload
  cannot meaningfully compete with the existing 8 MiB `defaultMaxAudioBytes`
  Stage 17A already enforces on *synthesized* output.
- **Duration bound**: `MaxSoundDurationMS = 30_000` (30 seconds), computed
  directly from the validated `data` chunk's byte count divided by the
  header's own byte rate - enforceable *because* WAV/PCM duration is exact
  and header-derived, never an estimate, unlike a compressed codec.
- **Metadata text bounds**: `DisplayName` - 1 to 200 Unicode code points,
  same bound family as every other metadata field in this codebase.
- **Content/extension/media-type/signature agreement**: the uploaded file's
  declared `Content-Type` (multipart part), its client-supplied filename
  extension (informational only, never trusted, never used as a storage
  name), and the actual bytes' own RIFF/WAVE header must all agree it is
  `audio/wav`; a mismatch on any axis is rejected with
  `audio_asset_type_mismatch` - the same "independent triple validation"
  discipline `visualasset.VerifyTypeAgreement` already established, applied
  to this stage's own closed WAV signature check instead.
- **Structural validation** (`validateWAV(data []byte) (durationMS int64, err
  error)`): confirms `RIFF`/`WAVE` magic, walks chunks (any chunk order,
  skipping unknown chunks safely by declared size, exactly like a real WAV
  reader must), requires exactly one canonical 16-byte `fmt ` chunk with
  `audioFormat == 1` (PCM; `WAVE_FORMAT_EXTENSIBLE`/`0xFFFE` and any other
  non-1 tag rejected outright - no ADPCM, no float, no μ-law, keeping the
  parser closed and simple per §2.3), `bitsPerSample == 16`, `numChannels ∈
  {1, 2}`, `sampleRate ∈ [8000, 192000]`; requires exactly one `data` chunk
  whose declared size is checked against the actual remaining byte count
  (never trusted alone, mirroring Stage 14B's own "declared size is never
  trusted alone" discipline); rejects a truncated, oversized-per-header, or
  structurally malformed file safely (a parse error, never a panic).

### 5.4 Storage directory and atomic installation

`<app-data>/assets/audio/blobs/<sha256hex>` (no extension, mirroring
`visualasset`'s own convention) via the reused `FileStore`. Same five-step
atomic sequence Stage 14B already established (§14 of
`docs/visual-template-packages.md`): stream to a temp file bounded by
`MaxSoundBytes`, hash while writing, verify size+type+hash together (this
stage's WAV structural check runs here, on the fully-buffered-to-temp-file
bytes, before promotion), `fsync`+close, atomic rename into the
content-addressed blob path, only then write the SQLite rows. Same crash
model: **files first, database second** - a post-crash orphan blob file with
no matching row is safe and collected by startup reconciliation (§5.7); a
database row is never committed for bytes that were never safely placed.

### 5.5 Reference tracking

A single join table, `alert_rule_audio_asset_refs (rule_id, asset_id)`,
mirroring `visual_design_asset_refs`'s own shape - **not** a ref-count
column. An alert-owned template audio preset (§10) that has not yet been
applied to any saved rule is tracked the same way visual-template asset
references already are, via `alert_template_audio_asset_refs (template_id,
asset_id)`. Both tables are rebuilt as a full replacement on every relevant
save (mirroring `visualasset.Service.SetDesignAssetRefs`'s own
full-replacement convention), never incrementally patched.

### 5.6 Deletion

Manual/API delete is rejected with `409 audio_asset_in_use` if either
reference table still has a row for that asset ID - exactly `visualasset`'s
own `ErrInUse` pattern. **Physical blob bytes are never deleted merely
because their last logical reference disappeared during a running server
process** - a currently-queued, currently-playing, or currently-replayable
alert instance may still hold an immutable snapshot (§9) naming that asset's
SHA-256/local ID; deleting the row is safe (a snapshot never re-resolves a
live asset row, only ever a resolved byte reference captured at enqueue
time - see §9), but the physical file is collected only by the next clean
startup's reconciliation pass, mirroring `docs/visual-template-packages.md`
§15's own reasoning applied to audio bytes.

### 5.7 Startup GC

`audioasset.Service.Reconcile(ctx) (ReconcileResult, error)`, called once at
`cmd/server`/`cmd/testserver` startup (same call site pattern as
`visualasset.Service.Reconcile`, a sibling call, not a merge into the
existing one): verifies the audio asset directory exists, detects an
`audioasset_blobs` row whose backing file is missing (reported as a
non-fatal diagnostic, never a startup failure), detects an untracked orphan
blob file (no matching row) and removes it, removes any blob row/file with
zero references in *both* reference tables, never follows a symlink, never
recursively deletes outside the exact managed audio asset root. Startup-only,
not periodic, exactly like Stage 14B's own precedent - no ticker/scheduler is
introduced.

### 5.8 Referenced-but-later-deleted safety

A saved rule's audio configuration stores an **opaque local asset ID**
(§7), resolved to bytes only at enqueue/promotion time (§9), exactly
mirroring how `visualdesign` layers already store an opaque `image`/`video`
asset ID rather than embedding bytes. If an asset is deleted while still
referenced by a saved rule, the deletion itself is rejected (§5.6) - a saved
reference can never become silently broken by an ordinary delete. If an
asset row's backing blob file is ever found missing (§5.7's diagnostic path
- e.g. external interference with the data directory), the rule's audio
playback for that reference fails safely and honestly (`playback_failed`,
§8), never a crash, exactly like a Stage 14B design encountering a missing
visual asset blob "reports an asset error/fallback rather than failing to
load at all."

## 6. Alert-rule audio configuration (task §4.4)

A new, bounded, embedded structure on `alerts.Rule` (`internal/domain/
alerts/model.go`):

```go
type RuleAudio struct {
    SoundEnabled bool
    SoundAssetID string  // required if SoundEnabled; must be a real audioasset.KindSound asset

    SoundVolume float64  // 0.0-1.0, default 1.0 - the rule-owned sound's own
                          // volume multiplier (§6.3)

    TTSEnabled  bool
    TTSTemplate string    // closed {placeholder} grammar (§6.4), required if TTSEnabled
    TTSVolume   float64   // 0.0-1.0, default 1.0 (§6.3)
}
```

`RuleAudio{}` (zero value: both `Enabled` flags false) is "no rule-owned
audio" - the default for every existing rule after migration (§6.5) and for
a newly created rule that never touches these fields.

### 6.1 Mode matrix

| SoundEnabled | TTSEnabled | Behavior |
| --- | --- | --- |
| false | false | No rule-owned audio (Stage 17A global TTS is the only audio path, unaffected) |
| true | false | Persistent sound only |
| false | true | Rule-owned TTS only |
| true | true | Sound, **then** TTS, back-to-back, never overlapped (§6.2) |

### 6.2 Deterministic sound+TTS order

**Sound first, then TTS** - the task's own preferred default, and there is
no implementation-constraint reason found during this audit to choose
otherwise (persistent sounds in comparable products are conventionally a
short "sting" that precedes an announcement, not the reverse, and TTS
duration is inherently more variable/unpredictable, better placed last so a
fixed short sound never gets cut awkwardly mid-sentence by a synthesis
delay). Implemented as an ordered two-item chain sharing one alert-instance
link (§9) - never a single item with internal concatenation, since the
existing `internal/audio` runtime already promotes/synthesizes/serves one
item at a time and reusing that exact mechanism twice in sequence is simpler
and safer than teaching it to splice audio buffers together.

### 6.3 Volume semantics - exact, no double-application

Three independent volume values can affect what an operator hears:

1. **Global audio output volume**: Stage 17A's `domain/audio.Settings.
   Volume` (0.0-1.0) - the operator's own "how loud is the whole audio
   Browser Source" control, applied identically to every item type
   (`SourceGlobalTTS`/`SourceAlertSound`/`SourceAlertTTS` alike) at the
   renderer (`AudioRenderer.tsx` already clamps and sets `audio.volume` from
   the current item's own `volume` field in the SSE payload).
2. **Rule-owned sound volume** (`RuleAudio.SoundVolume`) - applies **only**
   to a `SourceAlertSound` item.
3. **Rule-owned TTS volume** (`RuleAudio.TTSVolume`) - applies **only** to a
   `SourceAlertTTS` item, independent of `SoundVolume` (an operator may want
   a quiet chime but a normal-volume announcement, or vice versa).

**Combination rule, stated exactly**: the `audio.current` SSE payload's own
`volume` field for a given item is `globalVolume * itemOwnVolume`, where
`itemOwnVolume` is `1.0` for `SourceGlobalTTS` (Stage 17A never had a
per-item multiplier; unchanged), `RuleAudio.SoundVolume` for
`SourceAlertSound`, and `RuleAudio.TTSVolume` for `SourceAlertTTS`. This is
a single multiplication, computed once at enqueue/snapshot time (§9) and
carried on the queued item - never recomputed from a live, possibly-since-
changed rule, and never silently multiplied a second time anywhere else
(the renderer only ever reads the one `volume` field the SSE payload already
carries, exactly as today). No provider-level volume is separately applied
to `SourceAlertSound` (persistent sound bytes are served byte-for-byte,
never re-encoded); `SourceAlertTTS` still passes `Volume` into
`SynthesizeInput` exactly like `SourceGlobalTTS` does today (the *provider's
own* rendering of loudness, e.g. SAPI's native volume property), and that
same rule-owned value is what appears in the SSE payload's `volume` field
multiplied by `globalVolume` - a provider never independently re-scales on
top of the browser-side `<audio>.volume` the renderer already applies, this
was already true for `SourceGlobalTTS` and stays true here.

### 6.4 Rule-owned TTS text (task §4.5)

**Reuses `internal/alerts/templates.go`'s existing closed placeholder
system wholesale** - `KnownPlaceholders`, `ParseTemplate`, `Render`,
`Context`, `AvailablePlaceholders(t domain.EventType)`,
`ValidateTemplatePlaceholders`, `ValidateTemplateForEventType` - rather than
inventing a second grammar. This is a deliberate, audited choice: Stage 17A's
own *global* TTS (`internal/audio/utterance.go`) never had a customizable
text template (`BuildUtterance` is a hardcoded per-`engagement.Type` sentence
builder) - alert-rule TTS is the first place in this codebase a *customizable*
spoken-text template exists, and the existing `{name}`-style alert template
grammar (already validated, already tested, already understood by operators
editing a rule's visual `TextTemplate`) is the correct vocabulary to extend,
not `visualdesign.TextBinding` (a structurally different "one bound value
per layer" mechanism, not a freeform string grammar) and not a new
grammar.

`RuleAudio.TTSTemplate` is validated exactly like `Rule.TextTemplate`
already is: `ValidateTemplatePlaceholders` (unknown-name rejection) then
`ValidateTemplateForEventType(template, rule.EventType)` (capability-
availability rejection) at save time. **`{groupCount}` is explicitly
rejected in `TTSTemplate`** regardless of the rule's own grouping
configuration - grouping never restarts currently-playing audio (§9.3), so a
spoken group count would silently go stale the moment a group updates after
speech has already started; forcing this out closes that inconsistency at
validation time rather than documenting it as a known staleness. `{message}`
remains subject to the exact same `ValidateGroupingTemplate` restriction the
visual template already has (never referenced while grouping is enabled for
a `RequiresNoMessage` event type).

At render time, `internal/alerts.buildInstance`/`BuildTestInstance` (the
same function that already renders `TextTemplate` into `Instance.
RenderedText`) additionally renders `RuleAudio.TTSTemplate` into a plain
spoken-text string using the identical `Context`/`Render` call already built
for the visual template - one `Context` value serves both renders, since
both draw from the exact same matched-event fields.

The rendered spoken text then passes through Stage 17A's own shared
preprocessing pipeline (`internal/audio.Preprocess`) exactly as global TTS
text does - URL removal, blocked-word handling, repeated-character
normalization, whitespace normalization, and the Unicode-code-point maximum
- using the *rule's own* enabled toggles where the settings model has an
equivalent (see §6.6 for exactly which toggles apply). Command suppression
never applies (an alert-rule TTS item was never a chat message to begin
with). An empty result after preprocessing (e.g. a template that renders to
only blocked words) causes that `SourceAlertTTS` item to be skipped safely -
never silently replaced with fallback text, and never blocking the rule's
own `SourceAlertSound` item (if also enabled) from still playing.

### 6.5 Backward compatibility

`RuleAudio` embeds as a nilable-by-zero-value struct on `Rule`/`RuleInput`;
every existing persisted rule migrates with `SoundEnabled: false,
TTSEnabled: false` (§11's migration adds columns with `NOT NULL DEFAULT 0`/
empty-string defaults, never requiring a data backfill decision). An old API
client that omits the new `audio` object in a rule create/update request
body gets the zero value - **no existing rule begins producing sound after
migration or after an old client's write**, matching this task's own
explicit requirement.

### 6.6 Relationship to global Event-Bus TTS settings (task §4.5, §4.6)

Global TTS (`SourceGlobalTTS`) is driven by `domain/audio.Settings`:
`EnabledEventTypes`/`SupporterOnlyMode`/`EnabledProviderIDs`/
`EnabledSourceIDs`/threshold/`ManualApproval`/per-user and global cooldowns.
Alert-rule TTS/sound (`SourceAlertSound`/`SourceAlertTTS`) is driven
entirely by the **rule's own** enabled/matched state - an alert rule that
already matched and became the profile's current alert has, by definition,
already passed every filter that rule itself defines (`internal/alerts`'s
own matcher, provider/account filters, thresholds, roles). Therefore,
exactly as directed:

- alert-owned audio is **never** silently dropped because
  `domain/audio.Settings.EnabledEventTypes` doesn't include that event type,
  or `SupporterOnlyMode` would have excluded it, or `EnabledSourceIDs`/
  `EnabledProviderIDs` don't list that source - none of those Stage 17A
  settings are consulted for a `SourceAlertSound`/`SourceAlertTTS` item at
  all;
- it is **never** routed through `domain/audio.Settings.ManualApproval` -
  an alert becoming current on-screen already required no further human
  approval step (the alert engine has its own, separate, already-shipped
  "requires manual approval" concept nowhere in scope here - alert-owned
  audio simply follows whatever the alert itself already did);
- it is **never** delayed by `PerUserCooldownSeconds`/`GlobalCooldownSeconds`
  - those are Stage 17A's own anti-spam protection for chat-driven global
  TTS and have no bearing on an alert that already went through the alert
  engine's own priority/queue/rate characteristics.

**What alert-owned audio *does* still respect from Stage 17A**: the
`domain/audio.Settings.ProviderMode` capability for the TTS half only
(`SourceAlertTTS` needs a real synthesis provider exactly like global TTS
does - if `ProviderMode == disabled` or the provider reports itself
unavailable, `SourceAlertTTS` fails honestly, see §6.7); the shared
text-preprocessing primitives (§6.4); and the shared `MaxTextLengthCodePoints`
bound (a `TTSTemplate`'s rendered output is capped by the same global
maximum as any other spoken text, since it flows through the same
`Preprocess` call) - it does **not** respect `BlockedWords`/`RemoveURLs`/
`NormalizeRepeatedChars` toggles being *disabled* globally in a way that
would let raw blocked words through; if global settings have those safety
toggles off, alert-rule TTS still safely normalizes repeated characters and
whitespace unconditionally, and blocked-word filtering always runs
(matching `internal/audio.Preprocess`'s own behavior: blocked-word removal
"always runs, no-op if empty" - an empty global blocked-word list simply
means nothing is filtered, which is the operator's own explicit choice
either way).

### 6.7 Provider unavailable/disabled behavior

If `domain/audio.Settings.ProviderMode` is `disabled`, or the configured
provider reports `Capabilities().Available == false`:

- **persistent sound playback is entirely unaffected** - `SourceAlertSound`
  never touches the `tts.Provider` at all, so a rule with `SoundEnabled:
  true, TTSEnabled: false` keeps playing its sound normally regardless of
  provider state;
- **rule-owned TTS reports an honest, bounded failure** for that one item
  (`playback_failed` via the same `AckFailed`-shaped internal completion
  Stage 17A already uses for a failed `SourceGlobalTTS` synthesis - see
  `internal/audio/manager.go`'s existing `synthesize` failure path) rather
  than silently skipping or fabricating audio; if sound+TTS are both
  configured, the sound half still plays in full before the TTS half's own
  failure is reported - the chain (§6.2) does not abort the sound item
  merely because the *next* chained item will fail;
- the audio runtime as a whole is **never disabled** merely because
  synthesis is unavailable - this mirrors Stage 17A's own existing
  capability-honesty contract (`docs/audio-tts.md` §4) applied to the new
  alert-owned TTS path.

## 7. Alert-rule audio HTTP surface

Extends the existing alert-rule management API (`internal/httpapi/
alerts.go`) rather than introducing a parallel one - `RuleAudio` is
transported as a nested `audio` object on the existing rule create/update
request/response DTOs, following this project's existing full-replacement
PUT convention (`RuleInput` already mirrors `Rule` 1:1; `audio` becomes one
more field on both).

```json
{
  "audio": {
    "soundEnabled": true,
    "soundAssetId": "audioasset_...",
    "soundVolume": 1.0,
    "ttsEnabled": true,
    "ttsTemplate": "{username} says: {message}",
    "ttsVolume": 0.8
  }
}
```

Validated server-side at save time (never trusted from the client alone,
same discipline as every other rule field): `soundAssetId` must reference an
existing `audioasset.KindSound` asset when `soundEnabled` is true (unknown
ID -> `422 audio_asset_not_found`; wrong kind is structurally impossible
since Stage 17B defines only one kind, but the check exists for forward
safety); `soundVolume`/`ttsVolume` bounded `0.0-1.0`; `ttsTemplate` bounded
by the same length limit `TextTemplate` already has and validated via
`ValidateTemplatePlaceholders`/`ValidateTemplateForEventType`/
`ValidateGroupingTemplate` (§6.4); `ttsTemplate` required non-empty when
`ttsEnabled` is true; `soundAssetId` required when `soundEnabled` is true.
Reference-table replacement (§5.5) happens transactionally alongside the
rule row write, mirroring how `visualasset` reference rows are already
replaced alongside a design/template save.

Managed audio asset upload/list/delete gets its own small, bounded
management surface, following the exact same `Options`-field-plus-nil-guard
router convention every domain already uses (`internal/httpapi/router.go`):

```
POST   /api/audio-assets            multipart upload -> 201 with Asset metadata
GET    /api/audio-assets            -> list
GET    /api/audio-assets/{id}       -> metadata (never local storage path)
DELETE /api/audio-assets/{id}       -> 204, or 409 audio_asset_in_use
```

Multipart upload (never base64-in-JSON), following Stage 14B's own strict
contract: `http.MaxBytesReader` first, `multipart.Reader` streaming, exactly
one binary file part, one bounded `displayName` text field, rejection of any
unrecognized part. No management endpoint ever returns a local filesystem
path; a management response may include the asset's own local ID (used only
by the operator dashboard's own picker, never the public route) and its
validated `durationMs`/`byteSize`/`mediaType`/`displayName`.

## 8. Rule-owned playback and synchronization (task §4.6, §4.7)

### 8.1 Extending `internal/audio`, not replacing it

`internal/audio.Item` (§3's `Source` field) gains exactly the fields needed
to distinguish and resolve the three source kinds:

```go
type Item struct {
    ID         string
    Source     Source          // new: SourceGlobalTTS | SourceAlertSound | SourceAlertTTS
    Text       string          // meaningful only for SourceGlobalTTS/SourceAlertTTS
    AssetID    string          // meaningful only for SourceAlertSound
    Synthetic  bool
    Snapshot   ItemSnapshot
    EnqueuedAt time.Time
    ExpiresAt  time.Time

    // New: present only for SourceAlertSound/SourceAlertTTS.
    AlertLink  *AlertLink
}

// AlertLink ties a queued/current item back to the exact alert instance
// that produced it - the "stable runtime alert/presentation identity"
// this stage's own synchronization contract requires. Never exposed on
// the public route (§8.6); internal only.
type AlertLink struct {
    ProfileID    string
    InstanceID   string // internal/alerts.Instance's own ID
    ChainNext    *Item  // the queued "then TTS" half when sound+TTS (§6.2);
                         // nil for the final (or only) item in the chain
}
```

`ItemSnapshot` gains the resolved, already-multiplied `Volume` (§6.3) and,
for `SourceAlertSound`, the resolved blob bytes/content-type captured at
enqueue time (§9) rather than re-read from the asset store at synthesis
time - this is what makes a queued sound item immune to a concurrent asset
deletion attempt (which is rejected anyway per §5.6, but the immutable
snapshot means even a hypothetical future relaxation of that guard could
never corrupt an already-queued alert's audio).

### 8.2 `synthesize()` branches by `Source`

`Manager.synthesize` (today: always calls `provider.Synthesize`) gains a
type switch:

- `SourceGlobalTTS`/`SourceAlertTTS`: calls `provider.Synthesize` exactly as
  today, using `it.Text/Snapshot.VoiceID/Snapshot.Language/Snapshot.Speed/
  Snapshot.Volume` - `SourceAlertTTS` uses the *global* provider/voice/speed
  (§6.6: "uses the selected global provider/voice/speed foundation") with
  its own rule-owned `Volume` substituted in from `RuleAudio.TTSVolume`'s
  already-multiplied snapshot value.
- `SourceAlertSound`: **never calls `provider.Synthesize`** - resolves bytes
  through a new small injected interface,
  `type AudioAssetResolver interface { ResolveSoundAsset(ctx context.Context, assetID string) (data []byte, contentType string, ok bool) }`,
  supplied via `Manager.Options` (mirroring the existing `SelfUserIDLookup
  func(...)` decoupling pattern - `internal/audio` never imports
  `internal/domain/audioasset` directly, keeping the dependency direction
  the same "runtime depends on an injected function, not a concrete domain
  package" shape already established). The resolved bytes become the
  `SynthesizeResult`-shaped output directly (`ContentType: "audio/wav",
  Audio: data, Duration: <from the asset's own stored DurationMS>`) - no
  synthesis latency, no provider involvement, no `SynthesisTimeout` applies
  (bytes are already on disk; resolution failure is immediate, not a
  timeout).

A stale-result guard identical to today's (compare item ID before
committing a result to `m.current`) still applies to both branches.

### 8.3 Arbitration: alert-owned audio outranks global TTS (task §4.6)

`internal/audio.Queue` gains no general priority field (global TTS items
stay plain FIFO among themselves, unchanged) - arbitration is implemented
narrowly, only where the contract requires it:

- **`EnqueueAlertAudio` always preempts a currently-playing/synthesizing
  `SourceGlobalTTS` item.** When an alert becomes current (§9) and its
  `RuleAudio` produces at least one item, `Manager.EnqueueAlertAudio` checks
  `m.current`: if non-nil and `m.current.item.Source == SourceGlobalTTS`,
  that item is cancelled exactly as an explicit skip already cancels the
  current item internally (its in-flight synthesis context is cancelled,
  `totalManuallySkipped`-equivalent bookkeeping is **not** incremented since
  this is not an operator skip - a new counter,
  `TotalInterruptedByAlert`, is incremented instead so the two are never
  conflated in `Status`), and the alert-owned item is promoted immediately
  in its place. **The interrupted global TTS item is discarded outright,
  never resumed, never re-queued** - matching this project's own existing
  "never resume mid-item" convention already established for renderer
  disconnect (§16 of `docs/audio-tts.md`).
- **A currently-playing/synthesizing `SourceAlertSound`/`SourceAlertTTS`
  item from a *different, still-current* alert is never interrupted by a
  new `SourceGlobalTTS` arrival** - global TTS items are always inserted at
  the back of the ordinary FIFO ready queue and simply wait their turn like
  today, exactly as if a higher-priority queue item already existed; only
  the *reverse* direction (alert preempts global TTS) is special-cased.
- **Two alert-owned items arriving at nearly the same time (multiple
  profiles) are ordered by enqueue call order** - `EnqueueAlertAudio` runs
  under `Manager`'s own single mutex, exactly like every other Manager
  mutation; whichever `profileRuntime`'s promotion happened first (itself
  already deterministic - the alerts `Manager`'s own single Event-Bus
  consumption loop processes one event at a time, and per-profile
  promotion inside one `tick()` call is deterministic map/slice iteration
  order made deterministic by sorting profile IDs, see §8.5) calls
  `EnqueueAlertAudio` first and is promoted first; the second call finds
  `m.current` already occupied by alert-owned audio and is appended to the
  **back of the ready queue** like an ordinary item (never itself preempts
  another alert's audio - only global TTS is ever preempted). This is a
  total, deterministic order: enqueue-call order, which is itself
  deterministic because both the alerts `Manager`'s own tick loop and
  `internal/audio.Manager`'s mutex make it so - "whichever goroutine wins"
  never applies.
- **Never simultaneous.** There is exactly one physical audio renderer
  (§3); this project never claims two items play at once. A second
  alert-owned item genuinely waits in the ordinary ready queue, subject to
  its own `ItemExpiry` (§9.4) like anything else.

### 8.4 A new coordinating call site: `internal/alerts` calls into `internal/audio`

`internal/alerts.Manager` gains an optional, nil-able dependency,
`AudioLink AlertAudioLink` (mirrors the existing nil-guarded optional-
dependency pattern this codebase already uses everywhere an HTTP `Options`
field may be nil):

```go
// AlertAudioLink is internal/alerts's own narrow view of what it needs
// from internal/audio - never the reverse: internal/audio never imports
// internal/alerts, avoiding a dependency cycle and keeping internal/audio
// genuinely provider-independent of any one event-producing subsystem,
// exactly like it already is with respect to Twitch/YouTube/StreamElements.
type AlertAudioLink interface {
    EnqueueAlertAudio(profileID, instanceID string, items []audio.AlertAudioRequest)
    CancelAlertAudio(instanceID string)
    // AlertAudioState reports whether instanceID's linked audio has
    // started, is still playing, has ended, has failed, or was never
    // requested (no rule-owned audio configured) - polled by the
    // alerts tick loop for the bounded-hold behavior (§8.5).
    AlertAudioState(instanceID string) AlertAudioState
}
```

`internal/alerts`'s own `startCurrentLocked`/`completeCurrentLocked`/
`preemptCurrentLocked`/`skipCurrent`/`clearQueue`/`replayPrevious` gain calls
to this interface at the exact points the contract requires (§8.5-§8.7). The
alerts package builds the `[]audio.AlertAudioRequest` slice (sound item then
TTS item per §6.2, using the `Instance`'s own already-snapshotted fields -
never re-reading a live `Rule`) and hands it to `internal/audio` as an
opaque unit; `internal/audio` never needs to understand what an "alert
rule" or "profile" is beyond the two opaque ID strings used for the
`AlertLink` (§8.1) and for `AlertAudioState` lookups.

### 8.5 Normal completion and bounded visual hold

`internal/alerts.profileRuntime.tick(now)` (today: `if current != nil &&
now >= currentDeadline { completeCurrentLocked(...) }`) gains, only when
`current.audioLinked` (i.e. this instance actually requested rule-owned
audio):

```
if now >= currentDeadline:
    state := audioLink.AlertAudioState(current.ID)
    switch state {
    case NeverRequested, Ended, Failed:
        completeCurrentLocked(now, HideReasonCompleted)
    case Started, Playing:
        if now >= currentDeadline + MaxAudioHoldMS:
            // Hard bound reached - never freezes the queue indefinitely.
            audioLink.CancelAlertAudio(current.ID)
            completeCurrentLocked(now, HideReasonCompleted)
        // else: keep current visible, re-check next tick (20ms poll,
        // identical cadence to internal/audio's own poll loop)
    case NoRenderer:
        // No usable audio renderer at all - proceed on normal timing,
        // never wait for audio that can structurally never arrive.
        completeCurrentLocked(now, HideReasonCompleted)
    }
```

`MaxAudioHoldMS = 15_000` (15 seconds) - a deliberately generous but finite
bound, well above `audioasset.MaxSoundDurationMS` (30s hold would exceed a
sound's own max length in an edge case, so 15s covers the overwhelming
majority of real sound+short-TTS combinations while still guaranteeing the
queue can never actually freeze; a pathological rule combining a
near-max-length sound with a long TTS template may occasionally have its
audio truncated by this bound rather than the visual hold extending
indefinitely - an explicit, documented trade-off, not an oversight).
`playback_failed`, renderer disappearance mid-play (Stage 17A's own existing
`playback_unknown`/interrupted state, §16 of `docs/audio-tts.md`), and "no
renderer was ever connected" are all folded into paths that resume normal
alert timing rather than holding - **the alert queue can never deadlock
waiting on audio**, satisfying the task's own explicit requirement.

`AlertAudioState` is computed from `internal/audio.Manager`'s own already-
tracked `playbackState` (§2 of the Stage 17A implementation map) for the
item(s) linked to that instance ID via `AlertLink` - no new state machine,
a read-only projection of state `internal/audio` already maintains.

### 8.6 Public payload - no new leak surface

The public `audio.current` SSE payload gains no new field - `{itemId,
bytesUrl, contentType, volume}` stays exactly as Stage 17A defined it (§14 of
`docs/audio-tts.md`). `AlertLink`/`Source`/asset ID are internal-only,
attached to `Item`, never serialized to the public snapshot
(`PublicCurrentSnapshot`/`toPublicCurrent` continues to read only the same
four fields regardless of `Source`). The public alert SSE payload (`alert.
show`) likewise gains no audio-specific field - the alert Browser Source and
the audio Browser Source remain two genuinely separate pages/pipelines,
linked only by the shared, honest, best-effort timing behavior of §8.5, never
by a payload cross-reference. This is the honest boundary §9 below states
explicitly.

## 9. Synchronization contract, defined honestly (task §4.7)

**What is claimed:**

- Rule-owned audio is created from the exact same authoritative transition
  that makes one specific `Instance` current (`startCurrentLocked`) -
  `internal/alerts` calls `EnqueueAlertAudio` from inside that same locked
  transition, using that same `Instance`'s own already-immutable snapshot
  fields, never from a raw Event-Bus event handled independently of the
  alert queue.
- Both are linked by the stable `AlertLink{ProfileID, InstanceID}` identity
  (§8.1) for the lifetime of that one alert instance.
- Skip/clear/preemption/cancellation of an alert cancels its linked audio
  immediately (§9.1-§9.3) - linked audio can never outlive the alert
  instance that owns it by more than the bounded hold window (§8.5), and
  never bleeds into a later, unrelated alert (a new `AlertLink.InstanceID`
  is minted per instance; `internal/audio` never matches audio to the
  "next" alert merely because one happens to be current).
- Replay (§9.4) uses the replayed alert's own immutable audio snapshot,
  never a live re-read of the rule.
- A rule edit after enqueue never mutates an already-queued/current/
  replayed alert's audio, exactly like the existing visual-design snapshot
  guarantee (§4 of the Stage 17A implementation map, extended identically
  in §9.5 below).

**What is explicitly *not* claimed**, matching this task's own required
honesty and Stage 17A's own established convention (`docs/audio-tts.md`
§18):

- **No sample-accurate or frame-accurate synchronization.** The visual alert
  route and the audio route are separate pages, separate SSE connections,
  separate network round trips, and (per Stage 17A's own honest §18) an
  unverified real-OBS autoplay/mixer path. "Synchronized" here means:
  linked by identity and lifecycle, started from the same transition,
  cancelled together - never "the waveform's first sample plays on the
  exact video frame the alert appears."
- **No guaranteed zero-latency simultaneous start.** `EnqueueAlertAudio`
  runs synchronously inside the same locked transition that shows the
  alert, but the audio item still goes through the *same* just-in-time
  promotion/synthesis-or-resolve path every other item does (§8.2) - a
  `SourceAlertTTS` item's synthesis takes real, variable time (bounded by
  `SynthesisTimeout`, unchanged); a `SourceAlertSound` item resolves
  near-instantly (disk read, no synthesis) but still crosses the same
  promote -> ready -> renderer-fetches-bytes-URL -> renderer plays pipeline
  the public route already has. The alert visual can and often will render
  measurably before its linked audio's first audible sample.

### 9.1 Grouping never restarts current audio

`internal/alerts`'s own grouping (`tryGroupLocked`) only ever mutates a
still-**queued** instance in place (§2 of the alerts implementation map:
"grouping only ever operates on still-queued items... it never touches
`pr.current`"). Since rule-owned audio is only ever enqueued at the moment
an instance becomes *current* (§9, first bullet), a queued instance that
later absorbs a group member has never yet had `EnqueueAlertAudio` called
for it at all - there is nothing to restart. When that grouped instance
eventually *is* promoted to current, its audio is built once, from its
then-current `GroupCount`/rendered fields, exactly like any other instance -
this needs no special-case code, it falls out of the existing "audio is
enqueued only at promotion" rule directly. `{groupCount}` remains rejected
from `TTSTemplate` regardless (§6.4), so even a hypothetical future change
to when audio is built could never make a rendered TTS phrase reference a
count that then changes underneath it.

### 9.2 Preemption

`preemptCurrentLocked(inst, now, newID)` (today: `completeCurrentLocked` then
`startCurrentLocked`) gains, at the `completeCurrentLocked` call inside it,
`audioLink.CancelAlertAudio(oldInstance.ID)` **before** the new instance's
own `startCurrentLocked`/`EnqueueAlertAudio` runs - old audio is cancelled
first, new instance becomes authoritative, new instance's own audio starts
under the exact same normal start path as any other promotion (no special
"preempted" audio path). Interrupted audio (sound or TTS, wherever it was in
its chain) is discarded, never resumed - matching §8.3's own "never resume"
rule for the global-TTS-interrupted-by-alert case, applied symmetrically
here for alert-interrupted-by-alert.

### 9.3 Skip/Clear

`skipCurrent`/an operator "Clear Queue" action both call
`audioLink.CancelAlertAudio(current.ID)` at the same point they already
clear/complete `pr.current` - cancellation is unconditional and immediate,
never waiting for the bounded hold window (§8.5's hold only ever applies to
*normal* duration-expiry completion, never to an explicit operator skip/
clear, which always wins immediately per the task's own requirement).
Clearing the *ready queue* (not current) also removes any not-yet-promoted
alert-owned items still waiting their turn, exactly like clearing removes
any other queued item.

### 9.4 Replay

`replayPrevious()` clones `pr.lastCompleted` (§2 of the alerts implementation
map) - since that clone already carries whatever `RenderedText`/
`VisualDesign` snapshot the *original* instance had, the audio equivalent
follows identically: the replayed `Instance` carries the **same audio
snapshot fields** (resolved asset bytes reference, rendered TTS text,
resolved volumes) the original instance had at its own original enqueue
time, never re-resolved from the rule as it exists now. `EnqueueAlertAudio`
is called again for the replayed instance (a fresh `AlertLink.InstanceID`,
since replay already mints a new instance ID per existing behavior) using
those frozen snapshot values - a rule edit made between the original play
and the replay is never adopted by the replay.

### 9.5 Rule edits after enqueue never mutate an in-flight instance

`buildInstance`/`BuildTestInstance` copy `RuleAudio` (resolved: asset bytes
reference + computed volumes + rendered TTS text, exactly like `Instance.
VisualDesign`/`RenderedText` are already copied, §4 of the alerts
implementation map) onto the `Instance` at match/test time, cached the same
way `profileRuntime.designs` already resolves once per rule-CRUD-reload and
is copied per-instance thereafter - an edit to the rule's `RuleAudio` after
that copy has zero effect on any already-built `Instance`, exactly matching
the existing visual-design snapshot guarantee.

### 9.6 Synthetic Test Rule

`TestRule`/`BuildTestInstance` build audio through the **exact same**
`EnqueueAlertAudio` call path a real matched alert uses (task's own explicit
requirement) - a Test Rule with `RuleAudio.SoundEnabled`/`TTSEnabled` set
plays its configured sound/TTS through the real `internal/audio` runtime,
proving the operator's own configuration end to end. It still does not
publish a fake Engagement Event (unchanged from Stage 12A) and still does
not consult `domain/audio.Settings`' Event-Bus eligibility gates (§6.6) -
`Synthetic: true` on the resulting `internal/audio.Item` is set exactly like
`TestSpeak`'s own item already is, so a Test Rule's alert-owned audio is
excluded from lifetime counters the same honest way (`TotalSynthetic`,
unaffected by this stage).

## 10. Package/template audio (task §4.10)

### 10.1 Manifest schema C moves from v1 to v2

`visualpackage.CurrentManifestSchemaVersion` moves from `1` to `2`.

- **v1 packages remain fully, identically importable** - `ReadArchive`/
  `Import` branch on the manifest's own declared `schemaVersion`; a `v1`
  manifest is parsed by the exact existing v1 path, unchanged in any way, and
  produces an audio-free template exactly as it always has.
- **v1 export remains valid** for a purely visual template with no
  `RuleAudio`-equivalent template preset (§10.2) - `ExportTemplate`
  continues to write a `schemaVersion: 1` manifest whenever the template
  carries no alert-audio preset, so an operator's existing purely-visual
  template library exports byte-shape-identically to before this stage.
- **v2 is written only when the template being exported carries an
  alert-audio preset** (§10.2) - explicit and versioned, never silently
  upgrading a visual-only template's own export format.
- **An unrecognized future version (3+) is rejected** exactly like an
  unrecognized version already is today (`visual_template_package_version_
  unsupported`) - never guessed at, never coerced.

### 10.2 The optional `alertAudio` manifest object

Legal **only** in a `schemaVersion: 2` manifest, and **only** when the
contained `template.json`'s own `target` is `alert` (task's own explicit
requirement - a `chat`-target package containing this object is rejected
outright with a new stable error, `visual_template_package_audio_target_
invalid`, before any asset is even staged):

```json
{
  "format": "streaming-tree-template-package",
  "schemaVersion": 2,
  "templatePath": "template.json",
  "assets": [ /* unchanged v1 shape, image/video/font entries as before */ ],
  "alertAudio": {
    "soundEnabled": true,
    "soundAssetId": "pkgaudio_a1b2c3d4",
    "soundVolume": 1.0,
    "ttsEnabled": true,
    "ttsTemplate": "{username} says: {message}",
    "ttsVolume": 0.8
  },
  "audioAssets": [
    {
      "id": "pkgaudio_a1b2c3d4",
      "path": "audio/pkgaudio_a1b2c3d4.wav",
      "mediaType": "audio/wav",
      "sha256": "…64 hex chars…",
      "sizeBytes": 654321,
      "durationMs": 4200,
      "displayName": "Coin chime"
    }
  ]
}
```

- `alertAudio` is entirely optional (a v2 package may still carry zero audio
  - e.g. a template author who only wants to bump schema version for some
  future unrelated v2 field would still be valid, though this stage defines
  no such other field yet). When present, `alertAudio.soundEnabled`/
  `ttsEnabled`/bounds are validated with the **exact same** validator §7
  uses for a live rule's `audio` object - never a second, weaker validator.
- `audioAssets` is a **separate, sibling array** from the existing `assets`
  array (visual assets) - kept structurally distinct rather than widening
  `assets[].kind` to include `sound`, since audio assets have their own
  bound family (§5.3: duration, `MaxSoundBytes`) and their own package-local
  ID namespace (`pkgaudio_` prefix, disjoint from `pkgasset_`, so a
  manifest-parsing bug can never accidentally cross-resolve a visual
  reference against an audio entry or vice versa).
- `alertAudio.soundAssetId`, when present, **must** reference an entry in
  `audioAssets` (cross-checked bidirectionally, mirroring §9 of `docs/
  visual-template-packages.md`'s existing visual-asset cross-check
  exactly: every referenced audio asset ID must exist in `audioAssets`, and
  every `audioAssets` entry must be referenced by `alertAudio` - a package
  never contains audio bytes it doesn't use).
- Archive path grammar (§7 of `docs/visual-template-packages.md`) gains one
  new legal root pattern, `audio/<segment>`, validated by the identical
  `validateAssetPath` machinery already used for `assets/<segment>` (bounded
  ASCII filename, extension agreement, no traversal/absolute/drive/backslash/
  reserved-name/trailing-dot-or-space, case-insensitive duplicate
  rejection) - no new path-grammar code, one more accepted prefix.
- Bounds table (§10 of `docs/visual-template-packages.md`) gains: max single
  audio asset `8 MiB` (matching `audioasset.MaxSoundBytes`, §5.3), max audio
  assets per package `4` (an alert template needs at most one sound; four is
  a generous ceiling against a future multi-preset feature without inviting
  abuse today), and the existing aggregate bounds (max total uncompressed
  bytes, max entries, max decompression ratio) already cover a package that
  happens to also carry audio without needing a separate aggregate audio
  bound.

### 10.3 Audio imported through the exact same validator as manual uploads

A package's `audio/<segment>` entry is streamed through **the same**
`audioasset` structural/type-agreement validator §5.3 defines for a manual
upload - never a separately-implemented, potentially weaker package-path
validator. `ReadArchive`'s existing streamed-bytes/hash/decompression-ratio
protections (§9 of `docs/visual-template-packages.md`, reused verbatim) wrap
the audio entries exactly as they already wrap image/video/font entries;
only the final signature/structural check branches by declared `kind`
(visual asset validator for `assets[]`, audio asset validator for
`audioAssets[]`).

### 10.4 Package-local ID rewrite

Import: fresh local `audioasset_` IDs are generated for every accepted
`audioAssets` entry (via `audioasset.Service.Upload(..., Source: "package")`
- Stage 17B's own `Source` value distinct from `"upload"`, mirroring
`visualasset.SourcePackage`/`SourceUpload`'s own distinction exactly),
building an `idMap[pkgaudioID] = realAssetID`, then the imported template's
own stored `alertAudio.soundAssetId`-equivalent field is rewritten to the
real local ID before persistence - a package-supplied `pkgaudio_` ID is
never written into `alert_rule_audio_asset_refs`/`audioasset` tables.
Export: the reverse remap to deterministic `pkgaudio_%04d` IDs, sorted by
local ID, mirroring §6 of `docs/visual-template-packages.md` exactly.

### 10.5 Template-level audio preset persistence

`visualtemplate.Template` gains one new, optional, embedded field,
`AlertAudio *alerts.RuleAudioPreset` (a template-scoped variant of §6's
`RuleAudio` shape, referencing a *template-owned* asset reference rather
than a live rule - kept as its own small type so `visualtemplate` never
needs to import `internal/domain/alerts` for the shared `RuleAudio` struct
itself, avoiding a new cross-domain dependency; the two shapes are kept
field-identical by convention, exactly like `alerts.Capability`/`audio.
Capability` are already deliberately parallel-but-independent, §6 of the
alerts implementation map). `nil` for every existing template (built-in or
previously-imported) - migrated with no behavior change. Reference rows live
in `alert_template_audio_asset_refs` (§5.5).

### 10.6 Applying a template/package stays draft-first

Applying an alert template (JSON or package) changes the Alert Designer's
**unsaved visual draft** (existing behavior, unchanged) **and** its
**unsaved alert-audio draft** together, in the same client-side "apply"
action - never auto-saving the rule. The Designer workspace's existing
dirty-draft-replacement confirmation (`ConfirmDialog` before replacing a
dirty draft, §6 of the Stage 14 implementation map) is extended to describe
both halves ("this will replace your unsaved visual design and audio
configuration"); undo restores both coherently (one combined undo step,
mirroring `onUseAsDraft`'s existing "one undo step" comment exactly - visual
and audio drafts are pushed/popped together, never independently, so undo
can never leave a visual draft and an audio draft that were never a matched
pair). The rule's own separate, pre-existing Save action remains the only
thing that ever persists either half - applying a template/package, by
itself, still saves nothing.

### 10.7 Export/JSON interaction

- An audio-bearing template (via `AlertAudio` referencing a real asset)
  **cannot** be exported through Stage 14A's asset-free JSON path -
  `ErrRequiresPackageExport`'s existing check
  (`len(t.Document.AssetReferences()) > 0`) gains an additional condition,
  `|| t.AlertAudio != nil && t.AlertAudio.SoundAssetID != ""` - the same
  stable error code, the same frontend "package required" messaging
  (`templateHasAssets`-equivalent extended to also check for a configured
  audio preset), reusing the established mechanism rather than inventing a
  parallel one.
- **A TTS-only preset (no sound asset, only `ttsEnabled`/`ttsTemplate`) also
  requires package export**, per this task's own preferred answer -
  `TTSTemplate`/`TTSEnabled`/`TTSVolume` are Stage 17B alert-audio semantics
  with no representation in the Stage 14A JSON schema at all (that schema
  has no `audio` field of any kind), so `ErrRequiresPackageExport`'s
  condition is really `t.AlertAudio != nil && (t.AlertAudio.SoundEnabled ||
  t.AlertAudio.TTSEnabled)` - any configured alert-audio preset, sound or
  TTS or both, forces package export. Stage 14A JSON stays **permanently,
  unconditionally visual-only** - it never gains an audio field, now or
  later in this stage.
- Built-in Stage 14A templates (`DefaultBuiltins()`) remain audio-free -
  Stage 17B adds no built-in template carrying a preset.

## 11. Migration

Highest existing migration, confirmed by direct inspection of
`apps/server/internal/storage/sqlite/migrations/`, is `0021_audio_tts.sql`
(Stage 17A). **Stage 17B's own migration(s) begin at `0022`** - never
assumed from memory, always re-confirmed immediately before writing the
file, since this codebase's own convention (`internal/domain/audio`'s own
Stage 17A design doc did the same check against Stage 14B's migrations)
is to verify this fact fresh each time rather than trust an older document.
Exact table/column list and migration file count are decided at
implementation time once the domain models above are finalized in code, but
at minimum: `alert_rule_audio_asset_refs`, `alert_template_audio_asset_refs`,
new `alert_rules` columns for the six `RuleAudio` fields (§6), new
`audioasset_blobs`/`audioasset_assets` tables (§5.2) - each heavily
`CHECK`-constrained, following `0021_audio_tts.sql`'s own commented,
constrained style exactly.

## 12. Persistence and snapshot semantics (task §4.9)

**Persisted**: rule audio configuration (`RuleAudio`, on `alert_rules`),
managed audio asset metadata + blobs (`audioasset_*` tables, §5), template
audio presets (§10.5).

**Never persisted**, exactly mirroring Stage 17A's own existing list: the
runtime audio queue's contents, generated TTS bytes (rule-owned TTS is
exactly as ephemeral as global TTS - never written to SQLite, never becomes
a managed asset itself), active playback tokens, renderer session tokens,
current playback state, cooldown runtime state (moot for alert-owned audio,
which has none, §6.6), and the alert runtime queue itself (already true,
unchanged by this stage). `AlertLink`/`Source`/chain state (§8.1) are
runtime-only `internal/audio.Item` fields, gone on restart exactly like
every other `Item` field already is.

Snapshot immutability (§9.5) is the enforcement mechanism that makes "if
persistent sound bytes are physically GC'd later, existing in-memory queued
snapshots for this process must remain safe" true: a queued/current/replay-
pending `Item`'s `ItemSnapshot` already carries the *resolved bytes*
(§8.1), not a live asset-ID lookup performed at synthesis time - so even in
the (already-prevented-by-the-reference-guard, §5.6) hypothetical where an
asset's blob disappeared mid-process, an already-enqueued item is
unaffected, because it captured its bytes at enqueue time, not at play time.

## 13. Security (task §4.11)

Every Stage 14B archive defense (§4.11's own enumerated list; the source
enumeration is `docs/visual-template-packages.md` §§7-10) applies unchanged
to a v2 package containing `audio/` entries - ZIP-only, no blind extraction,
no absolute/drive/backslash/`..`/reserved-device-name paths, no symlink/
hardlink/special files, case-insensitive duplicate rejection, bounded entry
count/compressed/uncompressed bytes, decompression-ratio protection, strict
manifest decoding, hash verification, no nested archives, no arbitrary URLs,
no executable content - because §10's package-v2 implementation reuses
`visualpackage.ReadArchive`'s existing pipeline wholesale, adding only the
`audio/` path-grammar branch and the `audioAssets`/`alertAudio` manifest
fields, never a second, parallel archive reader.

For audio content specifically: the closed WAV-only format set (§2.3);
independent signature + declared media-type + extension agreement (§5.3,
§10.3); bounded bytes (`MaxSoundBytes`, §5.3) and bounded duration
(`MaxSoundDurationMS`, §5.3); malformed/truncated files rejected safely by
`validateWAV`'s own bounded chunk walk (never a panic, never an unbounded
read); no browser-supplied local path is ever persisted as a storage name
(§5.4, content-addressed by SHA-256 exactly like `visualasset`); no upload
filename becomes a storage path (mirrors §5.4); no token/path/secret ever
appears in an error message or log line for this stage's own new surfaces
(mirrors §23 of `docs/visual-template-packages.md` - operational logs may
include byte count, a local asset ID, a SHA-256 when genuinely useful for
diagnosing one failure, and a stable error code; never asset bytes, never a
local filesystem path).

## 14. Stable error codes

At minimum, in addition to the existing Stage 12A/13A/14A/14B/17A sets:

`audio_asset_not_found`, `audio_asset_invalid`, `audio_asset_unsupported`,
`audio_asset_too_large`, `audio_asset_type_mismatch`, `audio_asset_in_use`,
`audio_rule_config_invalid`, `audio_rule_asset_not_found`,
`audio_rule_tts_template_invalid`,
`visual_template_package_audio_target_invalid`,
`visual_template_package_audio_asset_missing`,
`visual_template_package_audio_asset_unreferenced`,
`visual_template_package_audio_asset_hash_mismatch`,
`visual_template_package_audio_asset_type_mismatch`,
`visual_template_package_audio_asset_unsupported`.

`visual_template_requires_package_export`/`visual_template_assets_missing`
(existing, Stage 14B) are reused as-is per §10.7's extended condition,
never duplicated under a new name.

## 15. Integration testing

The 20th integration script, `scripts/verify-alert-audio.mjs`, runs the real
`-tags integration` backend against a real fake TTS provider (the same
`tts.FakeProvider` Stage 17A's own 19th script already established, never a
second fake) and real, locally-generated valid/invalid WAV fixtures built
in-process by the script itself (no committed binary fixture files, mirroring
how the fake TTS provider generates deterministic PCM in-process rather than
shipping a binary asset) - never bypassing the alert engine or the audio
manager by calling internal functions directly. It exercises the real HTTP
management API for both alert rules and audio assets, the real public alert
SSE stream, and the real public audio SSE stream/bytes/ack protocol
together, proving the two are genuinely linked through the running system,
not merely unit-tested in isolation.

## 16. What Stage 17B explicitly does not implement

- No arbitrary audio mixing, multi-track timelines, DSP, fades, or audio
  filters.
- No user-provided SSML - rule-owned TTS text still only ever reaches the
  provider as plain text (§6.4), identically to global TTS (§10.5 of
  `docs/audio-tts.md`).
- No second TTS provider configuration per alert rule - rule-owned TTS
  always uses the one global provider/voice/speed foundation (§6.6).
- No goal/counter widgets (Stage 18) and no new engagement connector
  (Stages 15B/16B/19) - entirely out of scope, unaffected by this milestone.
- No Stage 20 packaging/updater work.
