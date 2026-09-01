# Stage 23 — Safe Configuration Backup & Restore

Canonical contract for Stage 23. Written after a full durable-state
audit of the real current architecture (every SQLite migration under
`apps/server/internal/storage/sqlite/migrations/`, `internal/secrets`,
and the existing `visualpackage` archive-security precedent) - nothing
here is assumed.

## 0. What this is, and is not

A Streaming Tree backup lets a creator recover their configuration
after data loss, or move it to a new machine, **without turning the
backup file into a portable credential vault**. It is:

- **SAFE CONFIGURATION BACKUP** - portable, non-secret application
  configuration plus the managed creator assets that configuration
  references.

It is explicitly **not**:

- a full machine clone including credentials;
- a way to move stream keys or connected-account authorization without
  re-entering/re-authorizing them;
- a merge/import tool (v1 restore is REPLACE, see §7);
- a place Stage 24 operational session history lives (§14).

This product decision (backup files never contain secrets, no
password-encrypted secret-backup mode in v1) is resolved and is not
reopened by this document.

## 1. Durable-state inventory

Every table in every migration (0001-0030) was read directly. Columns:
**Portable** = meaningful to move to another machine. **Secret** =
holds a credential/bearer-capability value directly (not a reference).
**v1** = included in a Stage 23 backup.

| Domain | Storage (migration → table) | Portable | Secret | v1 | Restore behavior |
|---|---|:-:|:-:|:-:|---|
| Platforms/destinations | `0001` `platforms`, `platform_metadata`, `platform_metadata_tags`, `0006` category_id | Yes | No | Yes | Fresh local `id`; `provider_id`/`display_name`/`enabled`/`sort_order`/metadata restored verbatim |
| Destination output settings | `0003` `platform_output_settings` | Yes | No (server URL only, never the key) | Yes | Restored verbatim, remapped to the new platform id |
| Stream key | `internal/secrets`, `SecretTypeDestinationStreamKey` | No | **Yes** | **No** | Restored destination shows `Key: Missing`; never guessed/placeholder |
| Provider integration settings (Twitch Client ID) | `0004` `provider_integration_settings` | Yes | No (public client id) | Yes | Restored verbatim; a `STREAMING_TREE_TWITCH_CLIENT_ID` env override on the target machine still wins, same as today |
| Connected accounts (identity/status) | `0005` `connected_accounts`, `connected_account_scopes`, `platform_account_links` | Yes (identity), No (status) | No | Yes (identity only) | Fresh local `id`; `status` forced to `reconnect_required` regardless of the backup's own value - see §5 |
| OAuth token bundle | `internal/secrets`, `SecretTypeOAuthTokenBundle` | No | **Yes** | **No** | Nothing restored; account is honestly `reconnect_required` |
| YouTube channel region override | `0008` `youtube_channel_settings` | Yes | No | Yes | Remapped to the new account id |
| Engagement connector enable/disable | `0009` `connected_account_engagement_settings` | Yes | No | Yes | Remapped to the new account id |
| YouTube remote broadcast target | `0007` `platform_remote_targets` | Marginal | No (a resource reference, not a token) | Yes | Remapped to the new platform id; a stale `resource_id` simply fails to resolve on next use, same as it would today if the broadcast ended |
| Operator chat preferences | `0010` `operator_chat_preferences` (singleton), `operator_chat_account_visibility`, `operator_chat_hidden_users`, `operator_chat_bot_users` | Yes | No (provider user ids, never names) | Yes | Singleton + remapped account-scoped rows |
| Chat overlay profiles | `0011` `chat_overlays`, `chat_overlay_accounts`, `chat_overlay_hidden_users`, `chat_overlay_blocked_terms`, `chat_overlay_activity_types` | Yes | No (`public_slug` is an "unguessable locator, not a credential" per the domain's own doc comment) | Yes | Fresh local `id`; `public_slug` **preserved verbatim** so existing OBS Browser Sources keep working |
| Chat automation (schedules/commands) | `0012` `chat_schedules`, `chat_schedule_targets`, `chat_schedule_messages`, `chat_commands`, `chat_command_aliases`, `chat_command_targets` | Yes | No | Yes | Fresh local ids; targets remapped to new account/platform ids |
| Alert profiles/rules | `0013`/`0014`/`0019`/`0020` `alert_profiles`, `alert_rules`, `alert_rule_providers`, `alert_rule_accounts` | Yes | No (`public_slug` same as chat overlay) | Yes | Fresh local ids; `public_slug` preserved; accounts/donation-source filters remapped |
| Visual designs (alert/overlay layouts) | `0015`/`0016` `visual_designs` | Yes | No (typed/validated JSON, never markup) | Yes | Fresh local `id`; `owner_id` remapped to the new alert-rule/chat-overlay id |
| Visual templates | `0017` `visual_templates` | Yes | No | Yes | Fresh local `id`; only user-created templates - built-ins are code, never rows |
| Managed visual assets | `0018` `visual_asset_blobs`, `visual_assets`, `visual_design_asset_refs`, `visual_template_asset_refs` + filesystem blob store | Yes | No (`public_token` same "locator, not credential" status) | Yes | Blob content (keyed by its own SHA-256) re-imported through `visualasset.FileStore`; asset `id` fresh, `public_token` preserved |
| Donation sources | `0020` `donation_sources` | Yes (label/channel id), No (credential) | No (this table); **Yes** (its credential) | Yes (table only) | Fresh local `id`; `enabled` forced to `false` - see §9 |
| Donation source credential | `internal/secrets`, `SecretTypeDonationSourceToken` | No | **Yes** | **No** | Nothing restored |
| Audio/TTS settings | `0021` `audio_settings` (singleton) | Yes | No | Yes | Singleton restored verbatim; `public_slug` preserved |
| Managed audio assets | `0022` `audioasset_blobs`, `audioasset_assets`, `alert_rule_audio_asset_refs`, `alert_template_audio_asset_refs` + filesystem blob store | Yes | No | Yes | Same pattern as visual assets |
| Alert rule sound/TTS | `0023` (columns on `alert_rules`) | Yes | No | Yes | Restored with the rule; `sound_asset_id` remapped |
| Template audio preset | `0024` (columns on `visual_templates`) | Yes | No | Yes | Restored with the template |
| Goals | `0025` `goals`, `goal_providers`, `goal_accounts` | Yes | No | Yes | Fresh local `id`; accounts remapped |
| Goal contribution dedupe ledger | `0025` `goal_applied_events` | Yes (needed for correctness) | No (no event content, only provider event keys) | Yes | Restored with its goal, remapped `goal_id`, so a replayed provider event is never double-counted post-restore |
| Widget profiles (goal/supporter/dashboard) | `0026` `widget_profiles`, `widget_profile_providers`, `widget_profile_accounts`, `widget_profile_event_types`, `widget_profile_dashboard_children` | Yes | No (`public_slug` same status) | Yes | Fresh local `id`; `public_slug` preserved; `goal_id`/dashboard children remapped |
| Update preferences | `0027` `update_preferences` (singleton) | Yes | No | Yes | Singleton restored verbatim |
| Remote overlay capabilities | `0028` `remote_overlay_capabilities` | No | **Yes** ("token is the capability itself" per the migration's own comment) | **No** | Nothing restored; any previously-enabled remote overlay must be re-issued explicitly after restore |
| Onboarding state | `0029` `onboarding_state` (singleton) | Marginal | No | **No** (see rationale below) | Not restored - a fresh install computes its own honest pending/dismissed state from what actually exists after restore, exactly like the existing-user migration rule already does (`0029`'s own `INSERT`) |
| Metadata presets | `0030` `metadata_presets`, `metadata_preset_tags`, `metadata_preset_provider_overrides` | Yes | No (structural exclusion proof: `docs/metadata-presets.md` §10/§12) | Yes | Fresh local `id`; provider-scoped, no platform reference to remap |
| Admin password verifier | `internal/secrets`, `SecretTypeAdminPassword` (fixed subject `default`) | No | **Yes** | **No** | Untouched by restore - not object-derived, no row to import |
| Remote-ingest publisher verifier | `internal/secrets`, `SecretTypeRemoteIngestPublisherPassword` (fixed subject `default`) | No | **Yes** | **No** | Untouched by restore |
| Headless master key | Externally provisioned, never in SQLite or the app's own store | No | **Yes** | **No** | Out of scope entirely - the application never persists it |

**Runtime/transient state excluded by definition** (never in any
migration, confirmed by direct audit, so there is nothing to exclude
*from* - it was never a candidate): FFmpeg/branch live state, MediaMTX
ingest live state, SSE subscriber lists, in-flight OAuth/device-code
flows, process IDs, in-memory operator-chat/chat-overlay projections,
in-memory alert/audio queues, updater download/install one-shot state.

## 2. Secret exclusion - structural proof plan

Two independent, testable guarantees (§29):

1. **No secret is ever read.** The backup writer's own dependency graph
   never imports `internal/secrets`, `internal/auth`, or any
   `TokenBundle`/`Credential`/donation-source-credential accessor. This
   is enforced by construction (the exporter is handed only the
   already-non-secret domain repositories - see §4's `PlatformMetadataStore`-
   style narrow-port precedent from Stage 22) and proven by a real
   fixture test (§29) scanning the produced archive's every byte for
   sentinel secret values seeded into `internal/secrets` before export.
2. **No secret-shaped field exists in the payload schema.** Mirrors
   `docs/metadata-presets.md`'s own security test pattern: reflect over
   every exported DTO/struct the backup format defines and assert no
   field name matches a secret-shaped denylist.

## 3. Capability-token audit (§4 of the governing task)

Every `public_slug`/`public_token` in the schema was read against its
own domain's documentation, not guessed from the word alone:

| Value | Where | Classification | Why |
|---|---|---|---|
| `chat_overlays.public_slug` | `0011` | **A** - ordinary identifier | Migration comment: "a separate, high-entropy, rotatable value... an unguessable locator, not a credential" |
| `alert_profiles.public_slug` | `0013` | **A** | Same local-Browser-Source pattern as chat overlays |
| `widget_profiles.public_slug` | `0025`/`0026` | **A** | Same pattern |
| `audio_settings.public_slug` | `0021` | **A** | Same pattern |
| `visual_asset_blobs.public_token` | `0018` | **A** | Same "locator, not credential" framing as the overlay slugs; used only to serve a local, non-secret media file |
| `audioasset_blobs.public_token` | `0022` | **A** | Same |
| `remote_overlay_capabilities.token` | `0028` | **B** - bearer capability | Migration comment states outright: "token is the capability itself"; PRIVACY.md independently describes the resulting remote URL as "a capability: anyone who has that URL can view that specific overlay" over a **wider, non-loopback** audience |

Category A values are preserved verbatim in a v1 backup (this is what
makes restore actually useful - existing OBS Browser Sources keep
working). Category B is never exported; see the inventory row above.

## 4. Restore identity strategy - no ID-collision attack surface

**Finding.** `internal/secrets.BuildKey(secretType, subjectID)` uses
the owning domain object's own persisted `id` as `subjectID` for every
object-scoped secret type (`credential.streamKeyKey` →
`platforms.id`; `account.tokenBundleKey` → `connected_accounts.id`;
`donationsource.credentialKey` → `donation_sources.id`). A backup file
is untrusted external input; if restore ever inserted an imported row
under the **id it arrived with**, a crafted backup naming an id that
happens to already exist in the local OS credential store would cause
the restored object to silently resolve to that pre-existing,
unrelated local secret the moment credential-consuming code looked it
up - without the restore process ever writing anything to the secret
store itself.

**Decision.** Restore **never reuses a backup-supplied `id` as a
literal local primary key**, for any restored object, secret-backed or
not. Every restored row's `id` is regenerated locally through that
domain's own existing `NewID()` generator (the same one `Create`
already uses today - `platform.NewID`, `account.NewID`,
`donationsource.NewID`, and so on), applied uniformly rather than as a
special case only for the three secret-backed domains - one restore
algorithm, not three. An in-memory `oldID → newID` map, built while
importing each domain in dependency order, is used to remap every
internal reference (`platform_account_links.account_id`,
`alert_rule_accounts.connected_account_id`,
`chat_schedule_targets.platform_id`, `widget_profiles.goal_id`, every
similar column named in §1). Content that is *not* a database identity
- `public_slug`/`public_token` values, names, notes, settings - is
preserved verbatim (§3).

Since Stage 23 v1 never exports a secret in the first place, this has
**zero functional cost**: no legitimate backup ever expects a restored
object to find a waiting secret under its old id anyway (§0's own
"Reconnect required"/"Key: Missing" contract already assumes it
won't). It closes the collision class by construction, for every
current and future secret-scoped domain, without relying on anyone
remembering to special-case a new one.

This also settles §6 (same-machine vs. portable restore): because
every restored object always gets a fresh id and always starts
credential-less, there is no scenario where a same-machine restore
could safely skip re-entering credentials that a foreign restore
could not - both are identical. No installation-identity mechanism is
introduced (`grep`-confirmed: none exists anywhere in the codebase
today to build on), and none is needed. Same-machine restore honestly
requires credential re-entry too, same as governing §6 explicitly
permits.

## 5. Package format

A versioned logical archive, reusing `internal/domain/visualpackage`'s
own archive-security implementation directly (same bounds, same
`archive/zip` + stdlib approach, same "never blind extraction"
pipeline: `apps/server/internal/domain/visualpackage/reader.go`,
`pathgrammar.go`) rather than a second, independently-written parser:

- `archive/zip`, read via `zip.NewReader` over the fully-buffered
  (size-bounded) upload - the same approach `visualpackage.ReadArchive`
  uses.
- Entry path grammar reused unmodified in spirit: no absolute path, no
  drive letter, no backslash, no `.`/`..` segment, no control
  character, bounded ASCII segment names, no reserved Windows device
  name, case-insensitive duplicate rejection, regular-file-only
  (symlinks/specials rejected).
- Bounds enforced before any entry is fully read: max archive bytes,
  max entry count, max total uncompressed bytes, max per-entry
  decompression ratio, max manifest bytes.
- Every real archive entry outside `manifest.json` must be a
  manifest-declared object; every manifest-declared object must have a
  real entry - exact cross-reference, no hidden payload.
- Every asset entry's declared size/SHA-256/media type must agree with
  its actually-detected signature.
- No archive path is ever joined directly to a filesystem destination;
  a validated entry name is only ever used as a lookup key into the
  already-validated allowed-entry set.

Package layout:

```
manifest.json          - format version, product identity, created_at,
                          source app version/platform, entry hashes
config.json             - the versioned logical configuration payload
                          (every "Yes" row in §1's table)
assets/<sha256-name>     - managed visual asset blobs actually
                          referenced by config.json
audio/<sha256-name>      - managed audio asset blobs actually
                          referenced by config.json
```

`manifest.json` fields (mirroring `visualpackage.Manifest`'s own
shape): `formatVersion` (int), `product` (fixed string identifying
this application, never user-editable), `createdAt` (RFC 3339),
`sourceAppVersion`, `sourcePlatform`, `configSHA256`, one entry per
asset with its own path/sha256/size. No executable content of any
kind is ever accepted (enforced the same way `visualpackage` already
enforces it: `visualasset`/`audioasset`'s own `VerifyTypeAgreement`
signature detection, never a file-extension guess).

Only app-managed assets actually **referenced by the included
configuration** are packaged (§1's own "included" rows) - never an
arbitrary filesystem crawl, matching `visualpackage`'s own existing
"a package should contain exactly the bytes it needs" rule.

## 6. Backup creation

A read-transaction/coherent-snapshot read across every included table
(SQLite: a single `BEGIN DEFERRED` read transaction spans every
`SELECT`, so no table observes a mutation that happened after another
table was already read), plus the matching set of asset blobs read
from the same coherent view. Backup creation is allowed while
streaming - nothing it reads is runtime state, so there is no
technical reason to block it (governing §15).

Output safety (§16): the user chooses the destination via the existing
local file-save flow this application already uses elsewhere; written
to a temporary file in that directory first, renamed into place only
after the archive is fully written and its own manifest hash
verified - a failed backup never leaves a truncated file that looks
valid. No network transfer of any kind. A `.streaming-tree-backup`
extension, matching `visualpackage`'s own `.streaming-tree-template`
naming convention.

## 7. Restore

**Mode: REPLACE**, not merge (§17) - the only predictable v1 contract,
matching the governing task's own explicit instruction against
ambiguous ID/account/destination/rule/template/asset merge semantics.

Flow (mirrors `visualpackage.Service.ImportPreview`/`Import`'s own
already-proven preview-then-commit shape exactly):

1. Bounded upload (`http.MaxBytesReader`, the same pattern
   `readBoundedPackageBody` already uses for template packages) →
   staged to a temp file, never buffered unbounded.
2. Archive/manifest/hash/cross-reference validation (§5) - nothing
   written yet.
3. `RestorePreview` stages validated assets under a fresh, random,
   time-bounded token (same `PreviewTTL`-style expiry as
   `visualpackage`) and returns a bounded summary: backup product/
   version/created-at, object counts per domain, asset count/size, an
   explicit "credentials excluded" notice, and which features will
   require reconnect/re-entry after restore. No mutation yet.
4. Explicit user confirmation.
5. A **safety snapshot** of the current configuration (the same §1
   non-secret domains, same logical format as a real backup) is
   written to app-owned storage, retaining only the single most recent
   one (bounded, no history).
6. **Streaming-active guard**: reuses `updater.StreamingActive(snapshots)`
   verbatim (`apps/server/internal/updater/guard.go`) against
   `branch.Manager.Snapshot()` - the exact function the updater's own
   install-blocked-while-streaming rule already uses. If active,
   restore is refused with `restore_blocked_streaming_active` (mirrors
   `updater.BlockerStreamingActive`); the operator stops streaming
   first. No force-kill of FFmpeg/MediaMTX.
7. Commit: re-validates independently from the token-scoped staged
   bytes (never trusts step 3's own preview as authority, matching
   `visualpackage`'s own "actual import does not trust preview" rule) -
   existing configuration tables are cleared and every restored row
   inserted with a freshly generated id (§4), inside one transaction.
8. Final integrity check, then the affected runtime subsystems
   (branch manager, MediaMTX config, alert/chat-automation in-memory
   state) are refreshed from the newly restored configuration exactly
   as they already refresh after any ordinary configuration write
   today - no special restart mechanism invented if the existing
   config-write reload path already covers it; audited per-domain
   during 23D.

If restore fails before step 7 commits, current configuration is
provably untouched (nothing was written). If it fails during step 7,
the safety snapshot from step 5 restores the pre-restore state through
the same restore code path, recursively.

## 8. Connected accounts / stream keys / donation sources

A restored connected account's `status` is always forced to
`reconnect_required` regardless of what the backup itself recorded -
never represented as `connected`/healthy merely because its row
exists (governing §11). A restored destination with no local stream
key reads `Key: Missing` through the existing credential-status query
path (no new field invented - `credential.Service` already reports
absence today). A restored donation source's `enabled` is forced to
`false` and its status honestly requires the credential to be
re-entered before it can activate. No provider is contacted during any
of this (§13 of this document / §34 of the governing task).

## 9. Remote management / remote overlay / remote ingest

Excluded entirely per §1: admin password verifier, remote-ingest
publisher verifier, and every `remote_overlay_capabilities` row. A
restore never re-provisions any of these; the operator must explicitly
re-enable remote management/ingest/overlay exposure afterward through
their existing, unchanged security flows. Restore itself is reachable
only through the same non-public route namespace (`/api/...`, not
`/api/public/...`) every other management endpoint already uses, so it
already inherits `withRemoteManagementSecurity` when remote management
is enabled (`apps/server/internal/httpapi/router.go`) - no new auth
mechanism needed, only correct route placement.

## 10. HTTP architecture

Reuses the exact download/upload shape `visualtemplate`/`visualpackage`
already established:

- **Download**: `Content-Type: application/zip`,
  `X-Content-Type-Options: nosniff`,
  `Content-Disposition: attachment; filename="..."` - same as
  `handleExportVisualTemplatePackage`.
- **Upload**: `http.MaxBytesReader(w, r.Body, maxBackupBytes)` before
  any read - same as `readBoundedPackageBody`.
- **Preview asset/staging access**: `Cache-Control: no-store`, staged
  under a random per-session token, deleted on completion/failure/TTL
  expiry.
- Never registered under `/api/public/*` - unreachable from any
  overlay/capability-token surface, per §9.

## 11. Substage decomposition

- **23A** - this contract; durable-state/security inventory (done, this
  document); domain `backup` package skeleton, manifest/format types.
- **23B** - logical export (the read-snapshot + per-domain DTO mapping)
  and package writer, reusing `visualpackage`'s archive primitives.
- **23C** - package reader/validator + `RestorePreview` (staging,
  bounded summary, no mutation).
- **23D** - atomic restore commit: id remapping (§4), safety snapshot,
  streaming-active guard, transactional replace, runtime refresh.
- **23E** - Settings UI: Backup/Restore panel, preview screen,
  destructive-restore confirmation, restore-complete summary
  (needs-attention list).
- **23F** - integration test (`scripts/verify-backup-restore.mjs`),
  security tests (§29-33 of the governing task), packaged-runtime
  extension, PRIVACY.md/README.md/project-overview.md updates.

## 12. Testing plan

Backend: secret-exclusion fixture scan (§29), portable restore across
two independent installations with independent `SecretStore`s (§30),
the secret-collision attack test using a crafted backup naming an id
that already has a local secret under it (§31 - release-blocking),
managed-asset round-trip + malicious-path rejection (§33), streaming-
active restore refusal, safety-snapshot rollback, every malformed/
malicious-package case from the governing task's §23 list (zip-slip,
oversized, decompression bomb, truncated, wrong hashes, unknown
format version, secret-shaped unsupported fields, crafted colliding
ids). Frontend: EN/PL, keyboard/accessibility, empty/preview/restored/
needs-attention states. Integration: a hermetic script in the
`scripts/verify-persistence.mjs` style. Packaged: extend
`scripts/verify-packaged-app.mjs`.
