# Stage 17A — shared audio runtime + text-to-speech foundation

This is the canonical Stage 17A contract, written before any product code in
this milestone. It records the primary-source research this stage's design
decisions rest on, the exact split between Stage 17A and Stage 17B, and the
runtime/protocol contracts every later commit in this milestone implements
against.

Stage 17A explicitly does **not** implement: alert sound assets, alert-rule
sound/volume, per-alert-rule TTS, synchronization with alert playback, local
neural voice models, cloud TTS, or any extension of `internal/domain/visualasset`
to carry audio. See §1.

## 0. Research method

Every external claim below was checked against the primary/official source
listed - Microsoft Learn pages for Windows APIs, the OBS Project knowledge
base and the `obsproject/obs-browser` GitHub repository for OBS behavior.
Where the official sources did not explicitly prove something this stage
needs, that is stated honestly rather than assumed - see §3.4 and §7.3.

## 1. Stage 17 split

**Stage 17A** (this milestone):

- one provider-independent audio/TTS subsystem (`internal/provider/tts`)
- one bounded runtime audio queue (`internal/audio`)
- one real system-TTS provider, Windows-only (research below confirms this
  is the only clean supported desktop path today - see §2)
- one real OBS Browser Source-compatible audio output route
  (`/overlay/audio/{publicSlug}`)
- global, Event-Bus-driven TTS configuration (`internal/domain/audio`) -
  one settings object, not a multi-profile model like alerts

**Stage 17B** (deliberately not implemented here):

- uploaded/persisted alert sound assets
- alert-rule sound selection and volume
- per-alert-rule TTS integration
- synchronization between visual alert playback and rule-owned audio
- any extension of `internal/domain/visualasset`'s managed-asset storage to
  carry audio

**Why the split exists**: Stage 14B's own `internal/domain/visualasset`
package doc comment already states this explicitly - *"Deliberately
excludes: audio assets of any kind (Stage 17 owns the application's one
audio/playback subsystem...)"* (`apps/server/internal/domain/visualasset/asset.go`).
Stage 14B left audio out on purpose so this project ends up with **one**
audio subsystem, not a visual-template sound engine plus a later, separate
TTS engine. Stage 17A is that one subsystem. Stage 17A does not add a
visual-design `audio` layer, does not widen the Stage 14B package v1 schema
to accept audio, and does not add alert sound upload UI - all of that is
Stage 17B's own, later, separately-scoped decision.

Expected status after this task: **Stage 17A = Completed. Stage 17B =
Planned, not started. Stage 17 as a whole = Incomplete.**

## 2. Windows system-TTS path comparison

Two real candidate APIs exist. Both were checked against Microsoft's own
current documentation.

### 2.1 `Windows.Media.SpeechSynthesis` (WinRT)

Source: <https://learn.microsoft.com/en-us/uwp/api/windows.media.speechsynthesis.speechsynthesizer>
(fetched; `updated_at: 2025-11-21`, current versioned docs up to
`winrt-28000`, **not** archived).

Confirmed facts:

- Namespace `Windows.Media.SpeechSynthesis`, requires Windows 10
  (`10.0.10240.0`) or later, `Windows.Foundation.UniversalApiContract`.
- `SynthesizeTextToStreamAsync(string)` returns a `SpeechSynthesisStream`
  (a random-access stream with `ContentType`) - this is playable through a
  `MediaElement` or readable as bytes, not forced to speakers directly.
- `AllVoices` enumerates installed voices; `DefaultVoice`/`Voice`
  select one; `Options` (added 1703/SDK 15063) exposes rate/pitch/volume/
  punctuation-reading controls.
- Every code example on the page is C#/C++/CX/JavaScript targeting a UWP
  XAML app (`MediaElement`) - there is no documented example of calling
  this from an ordinary unpackaged desktop process, and no Go interop of
  any kind exists for WinRT projection.

Verdict: **not chosen for Stage 17A.** The API itself is current and
capable, but this project's backend is a plain Go process (`apps/server`),
and Go has no first-party WinRT projection. Using this API would require
either a separate C++/WinRT shim compiled with a second toolchain (breaking
this project's pure-Go, CGO-free build story - see `apps/server/go.mod`'s
own comment about `modernc.org/sqlite` being chosen specifically to avoid
CGO) or an unaudited, immature third-party Go WinRT binding. Rejected per
this task's own explicit instruction not to add a dependency merely because
a snippet exists somewhere.

### 2.2 SAPI / `ISpVoice` (classic COM, Automation object `SAPI.SpVoice`)

Sources (fetched):
- <https://learn.microsoft.com/en-us/previous-versions/windows/desktop/ee125077(v=vs.85)>
  (SAPI 5.4 overview)
- <https://learn.microsoft.com/en-us/previous-versions/windows/desktop/ms723602(v=vs.85)>
  (SpVoice Automation interface reference)

**Honest caveat, stated up front**: both pages are marked
`ms.topic: archived` / `is_archived: true` on Microsoft Learn. SAPI's own
documentation is legacy/archived status, not actively maintained. This is
recorded here deliberately, not glossed over.

What is nonetheless confirmed, directly from these official pages:

- `ISpVoice` is "the Component Object Model (COM) interface" applications
  use to control TTS; `ISpVoice::Speak` synthesizes; `SetRate`/`SetVolume`/
  `SetVoice` change synthesis properties; `GetStatus` polls state during
  async speech; `Skip` "causes the voice to skip forward or backward in
  the input stream" (cancellation).
- The Automation object `SpVoice` (creatable via COM, ProgID
  `"SAPI.SpVoice"`) exposes the same capability set as late-bound
  properties/methods: `Rate`, `Volume`, `Voice`, `AudioOutput`,
  `AudioOutputStream`, `Speak`, `GetVoices`, `GetAudioOutputs`, `Skip`,
  `Pause`, `Resume`, `WaitUntilDone`.
- **Exact quote, decisive for this stage's architecture**: *"Use the
  AudioOutputStream property with other Speech automation objects to
  store audio output in memory (see SpMemoryStream) or in files (see
  SpFileStream)."* - i.e. SAPI's own documented Automation object model
  supports redirecting synthesis to an in-memory byte stream instead of
  the speakers, which is exactly this stage's preferred architecture
  (§6): the backend synthesizes to bounded bytes, the audio output surface
  performs playback.
- The XML/SSML-shaped voice-control tags (a separate, older SAPI page)
  document `Volume` as an integer 0-100 and `Rate`/`AbsSpeed` as an
  integer -10 to 10 - the exact native ranges Stage 17A's own
  provider-independent rate/volume translation wraps (§10.4).
- SAPI ships with every supported Windows release for backward
  compatibility; it is not documented as removed or scheduled for
  removal, only as archived documentation for what remains a present,
  functional OS component.

Verdict: **chosen for Stage 17A.** `SpVoice`/`SpMemoryStream` are
Automation (`IDispatch`-based) COM objects, callable via
`CoCreateInstance`/`IDispatch.Invoke` from any ordinary unpackaged Win32
process - no manifest, no packaging, no C++/WinRT shim. Go has a real,
audited interop path: `github.com/go-ole/go-ole` (MIT licence, no CGO -
"Go bindings for Windows COM using shared libraries instead of cgo" per
its own README; confirmed current release `v1.3.0`; exposes
`CLSIDFromProgID`, `CreateInstance`, and `IDispatch.CallMethod`/
`GetProperty`/`SetProperty`, exactly the shape needed to drive
`SAPI.SpVoice`/`SAPI.SpMemoryStream`/`SAPI.SpAudioFormat`).

### 2.3 Comparison table

| | WinRT `SpeechSynthesis` | SAPI `SpVoice` (chosen) |
| --- | --- | --- |
| Still documented by Microsoft | Yes, current (2025) | Archived/legacy, but not removed |
| Usable from an unpackaged desktop process | Not demonstrated in official docs; requires WinRT-from-desktop bridging | Yes - classic COM, no packaging |
| Enumerate installed voices | `AllVoices` | `GetVoices()` |
| Synthesize to a stream instead of speakers | `SynthesizeTextToStreamAsync` returns a stream | `AudioOutputStream` + `SpMemoryStream`, documented explicitly for this |
| Voice identifier stability | `VoiceInformation.Id` | Registry-backed object token ID |
| Rate/volume controls | `Options` object (SDK 15063+) | `Rate` (-10..10), `Volume` (0..100) |
| Cancellation | Not directly documented on this page | `Skip` method, explicitly documented |
| Go interoperability | None (no first-party WinRT projection) | `github.com/go-ole/go-ole`, MIT, no CGO |
| Dependency/licensing | Would need an unaudited third-party binding or a C++ shim | One audited, MIT-licensed, CGO-free Go module |
| Shells out to another executable | No | No - direct in-process COM calls |
| Testable without speakers | Yes (stream output either way) | Yes (`AudioOutputStream` redirect) |

Rejected outright, per this task's own explicit instructions, regardless of
the above: PowerShell speech synthesis, `cmd.exe`/shell execution,
VBScript/JScript files, browser automation, undocumented Windows APIs,
scraping, embedded cloud credentials, or any external online TTS website.

## 3. Platform matrix

A Windows `system` TTS provider is the only real provider Stage 17A ships,
because research (§2) confirms it as the cleanest supported route with a
real, auditable Go dependency. This does not make non-Windows builds fail:

- The provider implementation lives behind Go build tags:
  `internal/provider/tts/system_windows.go` (`//go:build windows`) and
  `internal/provider/tts/system_other.go` (`//go:build !windows`).
- Non-Windows builds compile and run identically; the `system` provider
  reports itself unavailable (`Capabilities().Available == false`,
  `UnavailableReason: "unsupported_platform"`), never a fake success.
- The frontend renders the real capability response; it never claims
  `system` mode works on a platform where the backend says it doesn't.
- `disabled` mode always works, on every platform - this is the safe
  default (§4).

No macOS `say` or Linux shell-command "fake parity" implementation exists
anywhere in this stage - that would violate the "no shell execution" rule
above regardless of platform.

## 4. Provider modes

The settings model has a `ProviderMode` enum: `disabled | system | local |
cloud`. Stage 17A implements only what is real:

- `disabled` - real, the default, works everywhere.
- `system` - real on Windows (§2.2); reports unavailable elsewhere (§3).
- `local` - **rejected by validation**. No local neural engine exists yet;
  saving `local` returns a validation error, not a silently-accepted
  no-op. A future local engine needs its own licensing/model review.
- `cloud` - **rejected by validation**, for the identical reason; a future
  cloud provider needs its own credentials/privacy/network contract.

The management API's capability response tells the frontend exactly which
modes are real (`available: true`) versus not yet implemented
(`available: false`, with a stable reason code) - the frontend never
invents provider support the backend does not report.

## 5. Provider abstraction

`internal/provider/tts` (package doc comment names every wire/COM detail
this hides):

```go
type Provider interface {
    Capabilities() Capabilities
    ListVoices(ctx context.Context) ([]Voice, error)
    Synthesize(ctx context.Context, in SynthesizeInput) (SynthesizeResult, error)
}
```

`SynthesizeInput` carries only provider-independent, already-validated
fields: plain text, a selected voice ID (or empty for system default),
optional language, a normalized rate multiplier, a normalized volume.
`SynthesizeResult` exposes only what the audio runtime needs: a content
type, bounded audio bytes, a duration when reliably known, and a small
sanitized diagnostics string - never a raw COM/HRESULT value. No UI code
and no Event Bus consumer knows SAPI/COM details; only
`internal/provider/tts`'s own Windows file does.

## 6. Shared audio runtime

`internal/audio` owns exactly the runtime state listed in the governing
task's own §11 (bounded queue, current item, pending-approval items,
synthetic-test distinction, counters, playback state, skip/clear/approve/
reject, renderer connected/disconnected, synthesis/playback failure
status) and nothing else - never Twitch/YouTube/StreamElements specifics,
never alert matching, never visual designs.

Preferred architecture, confirmed feasible by §2.2's own research: the
backend synthesizes to bounded bytes **just in time**, when an item is
promoted to "current" - never for the whole future queue (§7 for exact
bounds). Generated audio is ephemeral: never written to SQLite, and never
becomes a `visualasset`.

Nothing in the queue is ever persisted. A backend restart starts with an
empty queue and empty cooldowns - see §12 for exactly what small
disallow-list of things must never appear in a database column.

## 7. Bounds

- Queue capacity: default 100, operator-configurable within a bounded
  range (10-500).
- Maximum spoken text length: Unicode code points, not UTF-8 bytes (never
  truncates mid-rune); default 500, configurable 50-2000.
- Maximum synthesized audio bytes per item: 8 MiB (comfortably above a
  500-code-point utterance at any real voice's bitrate; a provider
  exceeding this fails that one item safely).
- Synthesis timeout: 10 seconds per item.
- Per-user cooldown: default 30s, configurable 0-3600s.
- Global cooldown: default 3s, configurable 0-300s.
- Queue item expiry: 5 minutes from enqueue - an item still waiting to
  play after 5 minutes (e.g. no renderer was ever connected) is dropped
  and counted, never spoken late. Chosen deliberately simple (one bound,
  not a per-event-type matrix) per this task's own "avoid overcomplication
  in 17A" instruction.

All bounds are enforced by the settings validator, never trusted from the
frontend alone.

## 8. Exact-currency monetary threshold

Mirrors `internal/domain/alerts.Rule`'s own model exactly (confirmed by
the codebase audit): one optional threshold carrying both a currency code
and integer micros, nilable, inclusive when set. No FX conversion, ever.
An event whose currency does not match the configured threshold currency
is filtered out (never spoken, never compared as if equivalent) -
deterministic, and the same "amounts are never compared across
currencies" rule Stage 16A's own alert integration already established.

## 9. Supporter-only semantics

A closed capability table (`internal/audio/capability.go`, mirroring
`internal/domain/alerts/capability.go`'s own exact pattern) marks each
current `engagement.Type` with a `SupporterFamily bool` fact, derived from
what the normalized event actually is - never inferred from a provider
name or a chat message's own `User.Roles`. Supporter-family types:
`bits`, `subscription`, `resubscription`, `gifted_subscription`,
`subscription_gift_batch`, `youtube.membership`,
`youtube.membership_milestone`, `youtube.super_chat`,
`youtube.super_sticker`, `donation`. `chat.message` and all other current
types are not supporter-family. When the settings' `SupporterOnlyMode` is
enabled, only supporter-family types are eligible for speech, regardless
of the operator's own `EnabledEventTypes` selection; when disabled, the
`EnabledEventTypes` list applies on its own. A future provider adding a
new event type is never spoken until this table gets an explicit new
entry - `CapabilityFor` returns the zero value (not speakable) for an
unknown type, exactly like the alerts package's own established
safe-default convention.

## 10. Text and preprocessing

### 10.1 Utterance builder

One provider-independent builder converts a normalized `engagement.Event`
into plain spoken text, using only fields the event actually carries
(display name, message text, `Money.DisplayAmount`, quantity, membership
level, anonymity) - never inventing data, never letting a provider
generate its own sentence. Output is plain text only: no SSML, HTML,
Markdown, arbitrary XML, remote fetches, or embedded audio tags, even
though SAPI itself accepts XML voice-control tags (§2.2) - Stage 17A never
constructs or accepts them from user/event data (§10.5).

### 10.2 Preprocessing pipeline (fixed order, every step tested)

1. Take the utterance builder's own plain text.
2. Command suppression (drop the item entirely if the source text is a
   recognized command and suppression is enabled) - see §10.3.
3. Remove URL text if enabled (`http://`/`https://`, and `www.`-prefixed
   text without a scheme, treated as URL-like and removed the same way -
   documented and tested explicitly).
4. Blocked-word handling - deterministic, case-insensitive, Unicode-aware
   whole-word matching (never a bare substring match inside an unrelated
   larger word).
5. Repeated-character normalization - collapse any run of the same code
   point longer than 3 down to exactly 3 ("sooooo" -> "sooo"); never
   collapses a normal doubled letter (a run of 2).
6. Whitespace normalization (collapse runs of whitespace, trim ends).
7. Maximum Unicode-code-point length enforcement (§7), preferring a word
   boundary when practical, hard cap is always the code-point count.
8. Reject (drop, count, never enqueue) if the result is empty after every
   step above.

No step uses HTML parsing, network lookups, or a regex with unbounded/
catastrophic backtracking potential. Only the TTS spoken copy is ever
touched by this pipeline - operator chat and the public chat overlay's own
message content are never modified by it.

### 10.3 Command suppression

Reuses the same one-line convention `internal/chatoverlay/filtering.go`'s
own `isCommandMessage` already established (`strings.HasPrefix(strings.TrimSpace(text),
"!")`) rather than `internal/chatautomation`'s own private, dispatcher-
coupled command parser (confirmed too entangled with cooldown/role logic
to reuse safely). A donation message that happens to start with `!` is
still eligible for speech - command suppression only ever applies to
`chat.message` events, never to a supporter-family event.

### 10.4 Speed and volume

Canonical app-level values: `Speed` a float multiplier, bounded
`0.5`-`2.0` (1.0 = normal); `Volume` a float, bounded `0.0`-`1.0`. The
Windows SAPI adapter translates these into SAPI's own native ranges
(`Rate` -10..10, `Volume` 0..100) - confirmed exact native ranges from
§2.2's own research - so the UI and the settings model never expose a
provider-specific numeric range directly. Never `NaN`/`Inf` - the
validator rejects both.

### 10.5 No user-supplied SSML

Even though SAPI documents its own XML voice-control tags, Stage 17A never
accepts `<speak>`/`<audio>`/`<voice>`/`<prosody>` or any other markup from
user, chat, or donation text. Every synthesis call uses SAPI's plain
`Speak` (or WinRT-style plain-text path, moot since WinRT was not chosen)
- never the SSML variant - with the already-sanitized plain-text
utterance.

## 11. Cooldowns

- Stable per-user identity, only when one genuinely exists:
  `providerID + connectedAccountID/sourceID + providerUserID`. Never
  display name, donor email, an email hash, a transaction ID, or a
  donation ID.
- An anonymous event, or one with no stable normalized user ID, uses the
  global cooldown only - no fabricated per-user key.
- Cooldown is reserved once a real event passes filtering and
  preprocessing and is **accepted into the bounded queue or pending-
  approval queue** - never before, and never rolled back merely because a
  pending item is later rejected by the operator (rejection is not a
  do-over; the cooldown reservation is itself anti-spam protection).
- Synthetic Test Speak items never read or write real cooldown state -
  proven by a dedicated test.
- Implemented against an injectable clock (mirrors this project's existing
  fake-clock testing convention), never wall-clock-only.

## 12. Persistence

`internal/domain/audio` persists **settings only** - one row, no
multi-profile model (there is exactly one global audio output, unlike
alerts' many profiles). Every field listed in the governing task's own §12
is persisted; queued text, chat/donor message content, generated audio
bytes, current playback item, cooldown state, and playback history are
**never** persisted - proven by a repository test asserting the migration
creates no such column.

Migration `0021_audio_tts.sql` (next number after `0020_donation_sources.sql`,
confirmed by inspecting `apps/server/internal/storage/sqlite/migrations/`)
follows the same heavily-commented, `CHECK`-constrained pattern
`0020_donation_sources.sql` itself established.

## 13. Public audio output route

`/overlay/audio/{publicSlug}` - a public, no-`AppShell`, presentation-only
React route, registered in `apps/web/src/App.tsx` next to the existing
`/overlay/chat/:publicSlug` and `/overlay/alerts/:publicSlug` routes,
following their identical "renders no application chrome" convention.

The slug uses the exact same discipline
`internal/domain/chatoverlay.NewPublicSlug()` already established (20
random bytes, hex-encoded, "practically unguessable" rather than a
credential, rotatable, rotation invalidates the old URL immediately) -
implemented as its own function in `internal/domain/audio`, not a shared
import, matching this codebase's existing per-domain-package convention
(donationsource/alerts/chatoverlay each define their own ID/slug
generation rather than sharing one).

Never reuses an alert-profile slug. Never requires an OAuth/account
credential in the URL. No arbitrary remote audio URL is ever accepted by
the renderer - the audio URL is always generated by this backend.

## 14. Audio output protocol

Modeled directly on `internal/httpapi/alerts.go`'s own
`handlePublicAlertStream` contract (confirmed by the codebase audit to
already implement exactly the shape this stage needs): **SSE + narrow
POST acknowledgements**, not a new WebSocket server. A WebSocket was
considered and rejected - the project already gets everything this
protocol needs from SSE (the alerts precedent proves it), and introducing
a second public WebSocket *server* surface (as opposed to the existing
outbound WebSocket *client* dependency used for Twitch/StreamElements)
would add a new, separately-securable listening surface for no protocol
benefit here; playback acknowledgement is a simple, infrequent, one-shot
signal that a narrow `POST` handles cleanly.

- `GET /api/public/audio/{slug}/stream` (SSE): current item only - never
  the future queue. Events: `audio.current` (a safe summary: a
  short-lived playback token/URL, volume, sequence), `audio.idle` (no
  active item - "waiting for content", not to be confused with §16's
  renderer-absent state), `audio.reset` (fresh-connection snapshot,
  mirrors `alert.reset`), `audio.gap` (sequence evicted / slow consumer,
  mirrors `alert.gap`), keepalive comments on the same interval
  convention as the alerts stream. `Last-Event-ID` / `?after=` supported
  identically to the alerts stream.
- `GET /api/public/audio/{slug}/bytes/{token}` - serves the current item's
  already-generated audio bytes, `Content-Type` and `Content-Length` set
  from the real synthesis result, `http.ServeContent` reused for Range
  support (the same stdlib mechanism `visualasset`'s own public route
  already uses - confirmed reusable at the mechanics level even though
  the package itself is not, since this route is hand-written against an
  in-memory byte buffer, never a file on a public path). The token is
  short-lived and per-item, never a filesystem path, never a database ID,
  never contains any source event text.
- `POST /api/public/audio/{slug}/ack` - the renderer reports
  `playback_started` / `playback_ended` / `playback_failed`, scoped to
  the exact current playback item and the caller's own active renderer
  session (§15). Carries no user text.

Per-profile connection limiting mirrors `alertStreamLimiter` exactly (a
bounded per-slug SSE client count), reused as its own small type in the
new package rather than imported from `internal/httpapi/alerts.go`
(package-private there).

## 15. Single active renderer

An explicit lease: connecting to the SSE stream establishes a new,
high-entropy, runtime-only session token (never logged, never persisted).
A newer session immediately supersedes the previous one as the sole
authoritative acknowledger; a stale session's ACK for any item is rejected
(§17). The management UI never opens the SSE stream itself and can never
accidentally become a second renderer.

## 16. Renderer disconnect and playback failure

If the active renderer disappears while an item is genuinely playing, the
runtime marks that attempt `playback_unknown` (interrupted) - it is never
silently assumed complete, and it is never auto-replayed from the start
when a renderer reconnects. The operator-facing status exposes this
explicitly. If no renderer is connected at all, new items wait in an
honest `waiting_for_renderer` state, still subject to the same queue
capacity and expiry bounds (§7) - an hour-old chat message never
suddenly speaks the moment OBS is later opened.

## 17. Acknowledgement validation

Every ACK is checked against both the active renderer session and the
exact current item's own identity. A duplicate, stale, or
wrong-item/wrong-session ACK is rejected outright and never advances the
queue, promotes a new "current" item, or otherwise mutates state - proven
by dedicated tests including deliberately replayed/duplicated ACKs.

## 18. Autoplay and real-OBS honesty

Both the OBS Knowledge Base (<https://obsproject.com/kb/browser-source>,
fetched) and the `obsproject/obs-browser` GitHub repository (fetched) were
checked directly. Confirmed: Browser Source is CEF-based and explicitly
documented as supporting "custom layout, image, video, and even audio
tasks." **Not documented anywhere in either official source**: any
autoplay policy, any "Control audio via OBS" property behavior, or CEF
audio-routing/mixing configuration. The KB page does document a "Refresh
browser source when scene becomes active" option, which is a real,
sourced fact relevant to renderer-disconnect handling (§16) - a scene
switch can legitimately restart the renderer's own connection.

Per this task's own explicit instruction, this gap is recorded honestly
rather than guessed at: **no claim is made that OBS's first programmatic
audio play is always accepted, and no claim is made that real OBS mixer
routing has been manually verified.** Automated tests (§21) prove the
HTTP protocol, the audio bytes served, and the renderer state machine -
never real speakers, never real CEF. Final manual product verification
(a real OBS Browser Source added once, confirmed to actually produce
audible speech) remains this project's own later, final manual-
verification pass, exactly like every other provider integration this
project has shipped so far.

## 19. Public privacy boundary

The public audio protocol exposes only what is needed to play the current
item: a playback token/URL, volume, minimal sequencing. It never exposes
the original `engagement.Event`, `ProviderEventID`, account/source IDs,
username, donor email, a donation transaction ID, the chat message
separately from the already-synthesized audio, queue contents, settings,
cooldown state, or any voice-system/filesystem detail. The generated
audio itself inherently contains the spoken result - no metadata beyond
what playing it back already reveals is ever exposed around it.

## 20. Management API

Consistent with this project's existing REST conventions (confirmed via
`internal/httpapi/router.go`'s nil-guarded `Options` field pattern):

- `GET/PUT /api/audio/settings`
- `GET /api/audio/capabilities` (provider availability per mode)
- `GET /api/audio/voices`
- `GET /api/audio/status` (bounded queue/runtme summary, §22 of the
  governing task's own field list)
- `POST /api/audio/queue/skip-current`
- `POST /api/audio/queue/clear`
- `POST /api/audio/pending/{id}/approve`
- `POST /api/audio/pending/{id}/reject`
- `POST /api/audio/rotate-slug`
- `POST /api/audio/test-speak`

Strict unknown-field rejection, bounded bodies, stable error codes, 404
unknown resource, 405 + `Allow`, 422 validation, 500/503 sanitized
internal/provider errors - no Windows HRESULT or raw COM exception text
ever reaches a client. No synthesized bytes appear in any management JSON
response.

## 21. Integration testing

The 19th integration script, `scripts/verify-tts-audio.mjs`, runs the real
`-tags integration` backend against a real integration-build-only fake TTS
provider (behind the same `Provider` interface the real Windows provider
implements) - never bypassing the Event Bus by calling the utterance
builder directly for real-event scenarios. The fake generates a small,
deterministic, valid WAV/PCM buffer in-process (no `ffmpeg` invocation, no
network) - WAV/PCM chosen specifically because its duration and byte
layout are exactly, deterministically knowable without a codec dependency.

## 22. Third-party dependency

`github.com/go-ole/go-ole` v1.3.0 (or the current release at
implementation time), MIT licence, no CGO. Linked directly into the
Windows-only build of `apps/server`; the Windows system-TTS file uses it
to call `SAPI.SpVoice`/`SAPI.SpMemoryStream` via COM Automation. Recorded
in `THIRD_PARTY_NOTICES.md` alongside this project's other direct
dependencies. No voice model is bundled - voices come entirely from
whatever the operator's own Windows installation already has.
