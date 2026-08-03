# Project journal — Streaming Tree for OBS

This file is the permanent journal of work on the project. It records what was
done, why, and what the real state of every feature is.

---

## Journal rules

1. **The file is updated on every logical change.**
2. **An entry is written before the commit it describes is created.**
3. **Every commit must have a corresponding entry.**
4. **The history of earlier entries must not be rewritten or deleted without a
   reason.** A correction to a wrong entry is added as a new entry, not by
   overwriting the old one.
5. **Stream keys, tokens and any other secrets are never written to this file.**
   This includes example values and log excerpts.
6. **A feature is not marked as completed if it is only an interface
   placeholder.** Placeholders belong in the "Known limitations" section.
7. **Manual testing remains the final stage** — performed only after the
   application functionality is complete.
8. **Build, lint and typecheck can and should be run during implementation**,
   and their results are recorded in the "Automated validation" section.

### Working language

From the entry `feat(web): add English and Polish localization` onward, the
code, comments, documentation, commit messages, progress entries and API
documentation are written in **English**. The interface itself remains
bilingual (English and Polish).

Entries written before that point were originally in Polish and were translated
into English in the commit `docs: migrate project documentation to English`.
The translation preserved all facts; nothing was added, removed or softened.

### Entry identification

An entry is identified by the **commit message**, not by a Git hash. Hashes are
not recorded: modifying this file after creating a commit would change the hash
itself, so the record would be stale by definition.

The [Conventional Commits](https://www.conventionalcommits.org/) convention
applies, for example:

- `docs: add initial project documentation`
- `chore: bootstrap project structure`
- `feat(web): add dashboard shell`
- `feat(server): add health endpoint`
- `fix(web): correct platform status rendering`

### Entry format

```
## YYYY-MM-DD HH:MM — type(scope): short commit description

### Status
### Scope
### Changes
### Files changed
### Technical decisions
### Automated validation
### Known limitations
### Next step
```

---

# Entries

## 2026-08-03 11:45 — chore: bootstrap streaming tree project

### Status
Completed

### Scope
The project foundations stage. Creating the repository structure, the
documentation, the React operator panel and a minimal Go backend with a health
endpoint that the frontend connects to. The stage does not cover real
streaming.

### Changes

**Repository structure**
- Created the `apps/web`, `apps/server`, `docs`, `config` layout.
- Added a `.gitignore` covering dependencies, build artifacts, environment
  files, data directories and the MediaMTX and FFmpeg binaries.
- Added `config/README.md` describing the planned contents of the configuration
  directory (currently empty, with no working configuration).

**Documentation**
- Created `docs/project-overview.md` with a full project description: the
  problem, the core idea, the target audience, the scope and exclusions of
  version 1.0, the architecture, the roles of OBS / React / Go / MediaMTX /
  FFmpeg, the independent branch model, the capability-driven metadata model,
  stream key security, the server version, the roadmap and the manual testing
  rule.
- Created this journal together with its rules.
- Created `README.md` with setup instructions for both a technical reader and
  someone new to the project.

**Frontend (`apps/web`)**
- Configured React 19 + TypeScript (strict mode) + Vite 6 + Tailwind CSS 4 +
  React Router 7 + TanStack Query 5 + Zod 4 + Lucide React.
- Built a visual token system (`src/index.css`): dark navy background, lighter
  panels, a violet accent, four semantic status colours, subtle borders and
  shadows, visible focus states and `prefers-reduced-motion` support.
- Implemented the application shell: left navigation panel (text logo, six menu
  items, OBS status, local RTMP address, version number), top bar (title,
  description, `Add Platform`, `Global Settings`, aggregated system status), the
  main content area and the right-hand status column.
- Implemented cards for four platforms (Twitch, YouTube, Kick, TikTok) with
  status, stream title, category, viewer count, connection quality, a
  Start/Stop button and a settings button.
- Implemented the status panel: live / starting / offline / error branch
  counters, the backend state card and the system resources card (CPU, memory,
  disk, network).
- Implemented the platform capability model (`PlatformCapabilities`,
  `PlatformFieldLimits`, `PlatformFieldOptions`) and a tabbed metadata editor
  that renders only the fields the selected platform supports.
- Implemented a tag editor in which every tag is a separate, removable interface
  element. Tags are enabled for Twitch only.
- Based metadata form validation on a Zod schema built dynamically from the
  platform capability table.
- Connected `GET /api/health` through TanStack Query; the result is shown in the
  system status section, and an unavailable backend produces a clear
  "Backend unavailable" state without crashing the interface.
- The Platforms, Streams, Metadata, Settings and Logs pages are presentable
  views describing the planned scope, with no fake widgets.
- Ensured responsiveness: the right column moves below the main content under
  `xl`, cards collapse to a single column on narrow screens, and the navigation
  becomes an off-canvas menu below `lg`.

**Backend (`apps/server`)**
- Created a Go module split into packages: `cmd/server`, `internal/config`,
  `internal/httpapi`, `internal/buildinfo`.
- Implemented `GET /api/health` returning `status`, `service`, `version`,
  `uptimeSeconds` and `time`.
- Added configuration through environment variables (`STREAMING_TREE_HOST`,
  `STREAMING_TREE_PORT`, `STREAMING_TREE_ALLOWED_ORIGINS`) with value validation
  and a clear error at startup.
- Added middleware: panic handling (recovering from a panic instead of killing
  the process), an access log, and CORS with an explicit allow-list of origins.
- Added graceful shutdown on SIGINT/SIGTERM with a timeout for in-flight
  requests to finish.
- Unified the JSON error format (`error`, `message`) and returning 405 together
  with an `Allow` header for an incorrect method.

### Files changed

Documentation and repository configuration:
- `README.md`
- `.gitignore`
- `docs/project-overview.md`
- `docs/progress.md`
- `config/README.md`

Frontend — configuration:
- `apps/web/package.json`, `apps/web/vite.config.ts`, `apps/web/eslint.config.js`
- `apps/web/tsconfig.json`, `tsconfig.app.json`, `tsconfig.node.json`
- `apps/web/index.html`, `apps/web/.env.example`

Frontend — code:
- `apps/web/src/index.css` (visual tokens)
- `apps/web/src/models/platform.ts`, `metadata-schema.ts`, `health.ts`
- `apps/web/src/data/demo-platforms.ts`, `demo-system.ts`, `app-info.ts`
- `apps/web/src/state/` (demo state store)
- `apps/web/src/lib/api-client.ts`, `cn.ts`, `format.ts`
- `apps/web/src/hooks/use-health-query.ts`
- `apps/web/src/components/layout/`, `ui/`, `platforms/`, `system/`, `metadata/`
- `apps/web/src/pages/`
- `apps/web/src/App.tsx`, `main.tsx`

Backend:
- `apps/server/go.mod`
- `apps/server/cmd/server/main.go`
- `apps/server/internal/config/config.go`
- `apps/server/internal/httpapi/router.go`, `health.go`, `middleware.go`, `respond.go`
- `apps/server/internal/buildinfo/buildinfo.go`

### Technical decisions

1. **The `apps/` structure follows the proposal.** No changes were made to the
   directory layout other than adding `config/README.md`, which describes the
   purpose of the empty directory so that Git keeps it and its role is
   unambiguous.

2. **Vite 6 instead of the newest Vite.** The environment runs Node 22.11, while
   current Vite releases (7/8) require Node `^20.19 || >=22.12`. On Node 22.11
   npm skips native optional dependencies because the `engines` field is not
   satisfied, which made Vite 8 fail to start ("Cannot find native binding").
   The newest line that works in this environment was chosen so that build, lint
   and typecheck are genuinely verifiable. After upgrading Node to 22.12+, Vite
   can be raised without changes to the application code.

3. **Tailwind CSS 4 configured in CSS.** Tokens live in the `@theme` directive
   instead of a `tailwind.config.js` file — fewer configuration files and a
   single place defining the palette.

4. **A capability-driven metadata model rather than a shared form.** The Zod
   schema is built by a function from a given platform's capability table and
   limits. As a result, the tag validation rule does not exist for a platform
   without tags, and adding a platform does not require modifying the form.

5. **Unsupported fields are not rendered, rather than disabled.** A disabled
   field would suggest the platform knows the concept but is temporarily not
   offering it.

6. **Demo state isolated in `src/state/` and `src/data/`.** A React context with
   a reducer, explicitly documented as a placeholder. It will eventually be
   replaced by backend data without changes to the presentational components.

7. **Backend response validation on the frontend side.** `GET /api/health` is
   parsed with a Zod schema. A shape mismatch produces a readable message
   instead of an error somewhere in the render tree.

8. **An `/api` proxy in the Vite dev server.** The frontend issues relative
   (same-origin) requests, which simplifies local work. The backend still has
   its own CORS middleware, because in the server version the panel will be
   served from a different origin.

9. **`net/http` without an external router.** The Go 1.22 ServeMux supports
   method-aware patterns (`GET /api/health`), which covers the current needs.
   Having no external dependencies simplifies distributing the binary.

10. **Panic-recovering middleware.** A single handler failing must not stop the
    process — the same principle that will later keep the stream branches
    independent.

11. **CORS with an explicit origin list instead of a wildcard.** The server will
    eventually control real transmissions, so access from any page open in the
    browser is unacceptable.

12. **TypeScript in strict mode with additional flags**
    (`noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `noUnusedLocals`,
    `noUnusedParameters`). The code contains no `any` type; the ESLint rule
    `@typescript-eslint/no-explicit-any` is set to `error`.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Frontend typecheck | `npm run typecheck` (`tsc -b`) | Passed — 0 errors |
| Frontend lint | `npm run lint` (`eslint .`) | Passed — 0 errors, 0 warnings |
| Frontend build | `npm run build` | Passed — 1982 modules, `dist/` generated |
| Backend compilation | `go build ./...` | Passed — 0 errors |
| Backend static analysis | `go vet ./...` | Passed — 0 findings |
| Backend formatting | `gofmt -l .` | Passed — no files need formatting |
| Endpoint contract (scripted) | `GET /api/health` against the running binary | 200, `application/json`, payload matching the Zod schema |
| Error handling (scripted) | `POST /api/health`, `GET /api/<nonexistent>` | 405 with an `Allow: GET` header; 404 with a JSON payload |

Note: the checks marked "scripted" are automated HTTP requests made by a shell
script to verify the API contract. They are not manual interface tests — those
remain the final stage.

Environment note: Go was not installed on the user's system at the time. The
backend checks were run with a portable Go 1.26.5 toolchain unpacked into a
temporary directory, without modifying the system environment. To run the
backend yourself, Go must be installed — see `README.md`.

### Known limitations

**Unimplemented items**
- No real streaming: MediaMTX, FFmpeg and process control do not exist.
- No OAuth sign-in and no integrations with the Twitch, YouTube, Kick or TikTok
  APIs.
- No database — configuration and metadata are not persisted; reloading the page
  restores the initial state.
- No credential store; stream key handling has not been started.
- No SSE/WebSocket — the backend state is polled every 15 seconds.
- The Platforms, Streams, Metadata, Settings and Logs pages have no
  implementation.

**Placeholders (marked with a "Demo" badge in the interface)**
- The Start/Stop buttons only change local state in the browser; after about
  1.8 s the status moves from `starting` to `live`. No process is started.
- The viewer count and connection quality are fixed values.
- The CPU, memory, disk and network metrics are fixed demo values — the backend
  does not collect host metrics.
- The OBS connection status is fixed ("Waiting for OBS"); nothing is listening on
  the RTMP port.
- The local RTMP address in the sidebar is a planned address, not a working one.
- Saving metadata goes only to the browser's memory.
- The platform capability tables are approximate and illustrative; they need
  verification when real integrations are implemented.

**Environment problems**
- Node 22.11 does not satisfy the `engines` requirements of the newest frontend
  tooling. An upgrade to Node 22.12+ or 24 LTS is recommended. The current set of
  versions works correctly on Node 22.11.
- The repository was not initialized as a Git repository at the time this stage
  was carried out, so the commit was not created automatically. The commands to
  run were provided in the stage summary.

### Next step

Stage 2: persistent configuration storage.

1. Adding SQLite to the backend together with schema migrations.
2. A platform and metadata model on the backend side, as the source of truth
   instead of demo data in the browser.
3. REST API: `GET/POST/PUT/DELETE /api/platforms` and
   `GET/PUT /api/platforms/{id}/metadata`, with validation based on the same
   capability table as the frontend.
4. Replacing the demo store in `src/state/` with TanStack Query requests, keeping
   the current presentational components.
5. Keeping the `Backend unavailable` state as a fully handled path.

---

## 2026-08-03 14:55 — feat(web): add English and Polish localization

> From this entry onward the journal is written in English. The historical
> entry above is translated in the following commit
> (`docs: migrate project documentation to English`); its facts are preserved
> unchanged.

### Status
Completed

### Scope
Introduce the internationalization foundation for the frontend: English and
Polish interface resources, a language switcher, a persisted language
preference, localized backend error messages, an automated resource
consistency check and focused tests. No product feature was added or redesigned.

### Changes

**Localization core (`apps/web/src/i18n/`)**
- Added i18next + react-i18next with a static, build-time resource bundle. No
  translation service, API or runtime auto-translation is used anywhere.
- Centralized every language code, namespace name, locale tag and the storage
  key in `config.ts`, so no component contains a literal `'en'` or `'pl'`.
- Added a validated `SupportedLanguage` type with `isSupportedLanguage` and
  `toSupportedLanguage` guards used at every untrusted boundary.
- Configured English as both default and fallback language, `escapeValue: false`
  (React escapes on render), `load: 'languageOnly'` so `pl-PL` resolves to `pl`,
  and a development-only missing-key warning. Production silently falls back to
  English - a raw key is never shown to a user.
- Added an i18next module augmentation so translation keys are checked against
  the English bundle at compile time.

**Language preference**
- Stored under `streaming-tree.language` in localStorage. This remains the only
  value the application persists in the browser.
- The stored value is validated before use; an unsupported or corrupted value
  falls back to English. Reads and writes are wrapped so a disabled or full
  localStorage cannot break the switcher.
- The browser language is deliberately ignored: first launch is always English.
- `changeAppLanguage` updates i18next, the stored preference and
  `<html lang>` together, so the three cannot drift apart.

**Language switcher**
- Added `LanguageSwitcher`, a native `<select>` with endonym labels
  ("English", "Polski") and no flag icons. Native markup gives keyboard
  operation, the platform picker on mobile, and the existing focus ring for
  free.
- Placed in the top bar (all pages) and in a working "Interface language" panel
  on the Settings page. Switching re-renders immediately; no page reload.

**Interface translation**
- Extracted every user-facing string into seven namespaces: `common`,
  `navigation`, `dashboard`, `platforms`, `metadata`, `pages`, `errors`.
- Covered navigation, headings, buttons, status and quality labels, Demo badges
  and their explanations, platform cards, system status panels, resource
  labels, the metadata editor, form labels, placeholders, hints, validation
  messages, loading, empty and "Backend unavailable" states, planned-page
  descriptions, the OBS connection panel, the version footer and the mobile
  menu dialog.
- Proper pluralization for active streams, starting streams, branches in error,
  branch counters, viewer counts and the tag counter. Polish uses its full CLDR
  set (`one`, `few`, `many`, `other`).
- Numbers, compact viewer counts and list joins are locale-aware via `Intl`.
- Sentences are never assembled from translated fragments: each is one entry
  with interpolation.

**Repository fix found while staging this change**
- `.gitignore` contained unanchored `data/`, `logs/` and `build/` rules. The
  `data/` rule silently matched `apps/web/src/data/`, so the demo data modules
  were never committed in `chore: bootstrap streaming tree project` and a fresh
  clone could not build. The rules are now anchored to the repository root
  (`/data/`, `/logs/`, `/build/`) and the two source files are added here.
  `dist/` stays unanchored because build output lives in `apps/web/dist`.

**Not translated, by design**
- User-authored content (stream titles, descriptions, tags), platform brand
  names, URLs, the RTMP address, API identifiers such as service name and
  version, backend error codes, and stream-language endonyms.

**Backend error handling**
- `ApiError` now carries the backend's stable `code` and its English `message`
  from the `{ error, message }` envelope. The API contract itself is unchanged.
- `resolveApiErrorMessage` prefers a localized message mapped from the stable
  code, then the backend's own English message for HTTP failures it explained,
  then a localized transport-level message. Arbitrary server text is shown
  verbatim, never machine-translated.

**Consistency check**
- Added `apps/web/scripts/check-i18n.mjs` and `npm run i18n:check`. English
  defines the canonical structure; the script reports missing keys, extra keys,
  object/value structure mismatches, empty values and plural-form problems,
  printing the full dotted path of each and exiting non-zero.
- The check is plural-aware: plural groups are compared by base key, and each
  language is validated against the categories `Intl.PluralRules` requires for
  it. Without this, Polish `_few`/`_many` entries would be reported as
  mismatches against English `_one`/`_other`.

**Tests**
- Added Vitest with jsdom (no browser automation). 32 tests across three files
  cover: invalid, corrupted and missing stored values falling back to English;
  a valid stored Polish preference being accepted; localStorage failures being
  survivable; only the language key being persisted; language switching
  updating instance, storage and `<html lang>`; unsupported languages being
  ignored; Polish resource lookup; Polish plural categories; fallback to
  English for a missing Polish key; and the parity checker both passing on the
  shipped resources and correctly detecting each class of defect on fixtures.

### Files changed
- `apps/web/src/i18n/` - `config.ts`, `types.ts`, `resources.ts`, `index.ts`,
  `language-storage.ts`, `document-language.ts`, `use-language.ts`,
  `i18next.d.ts`, `resources/{en,pl}/*.json` (7 namespaces per language), and
  three test files.
- `apps/web/scripts/check-i18n.mjs`, `apps/web/scripts/check-i18n.d.mts`
- `apps/web/src/components/` - layout, ui, platforms, system and metadata
  components; new `ui/LanguageSwitcher.tsx` and
  `metadata/use-validation-messages.ts`.
- `apps/web/src/pages/` - all pages; `PlaceholderPage` now accepts real content
  above the placeholder card, used by the Settings language panel.
- `apps/web/src/models/platform.ts`, `metadata-schema.ts`
- `apps/web/src/data/demo-platforms.ts`, `demo-system.ts`
- `apps/web/src/lib/format.ts`, `api-client.ts`, new `api-error-message.ts`
- `apps/web/package.json`, `tsconfig.app.json`, `tsconfig.node.json`,
  `vitest.config.ts`, `src/App.tsx`
- `.gitignore` (anchored the runtime-data rules; see above)

### Technical decisions

1. **Static bundled resources instead of runtime loading.** Both languages
   together are a few kilobytes and the app runs locally, so bundling removes an
   entire class of "translation not loaded yet" states and needs no HTTP backend
   plugin.

2. **The data and model layers carry translation keys, not text.** Demo data,
   the platform capability model and navigation items store keys such as
   `latency.low`. Typed with i18next's `ParseKeys`, a renamed or deleted key
   becomes a compile error instead of text rendered as its own key.

3. **Validation messages are injected into the Zod schema.** `metadata-schema.ts`
   receives an explicit, fully typed `MetadataValidationMessages` object rather
   than a translate callback. The schema module stays free of display language,
   and every message key is checked once, in one hook.

4. **No language detector plugin.** The requirement is that first launch is
   always English, which browser detection would break. Dropping the plugin also
   drops a dependency.

5. **Endonyms for stream languages and for the switcher.** Language names are
   proper nouns and are conventionally shown in their own language, so they are
   not translation resources. This also keeps them out of parity checking.

6. **The consistency check is plain ESM JavaScript.** It runs through `node`
   with no build step and no extra dependency, and its comparison function is
   exported so the test suite asserts on it directly rather than duplicating the
   logic. A hand-written `.d.mts` keeps the TypeScript import type-safe.

7. **jsdom pinned to 26.** jsdom 30 pulls a transitive dependency that requires
   an ESM module from CommonJS, which Node 22.11 cannot do (`require(ESM)` is
   unflagged only from Node 22.12). Same root cause as the Vite 6 pin recorded
   in the previous entry.

8. **`Intl.ListFormat` for the unsupported-fields list.** The list separator and
   final conjunction differ per language, so joining is delegated to `Intl`
   rather than hard-coded.

9. **The `.gitignore` fix is included in this commit rather than a separate
   one.** The affected files (`src/data/`) now carry translation keys and are
   part of this change; committing the localization without them would leave
   `origin/main` unbuildable.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 7 namespaces, no differences against `en` |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test` | Passed - 3 files, 32 tests |
| Frontend build | `npm run build` | Passed - 2035 modules |
| Backend formatting | `gofmt -l .` | Passed - no files need formatting |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - no test files (backend untouched this stage) |
| Backend build | `go build ./...` | Passed - 0 errors |

Two checks failed on the first run and were fixed before completion: the
strict key typing rejected a test that used unknown keys (rewritten to exercise
fallback through the real bundles), and ESLint flagged a type-only import.

No manual UI testing was performed.

### Known limitations
- The English and Polish wording of the platform capability tables remains
  approximate and illustrative, as recorded in the previous entry. Localization
  did not change that.
- Polish plural forms were written by hand and reviewed against CLDR
  categories, but have not been reviewed by a second Polish speaker.
- Adding a third language currently requires editing `SUPPORTED_LANGUAGES`,
  adding a resource directory and adding a locale tag; there is no automated
  scaffolding for it.
- The `starting -> live` demo transition, the fixed viewer counts, the
  placeholder host metrics and the in-memory metadata store are unchanged and
  remain demo behaviour, still marked as such in the interface.
- The consistency check treats any leaf key ending in `_one`, `_few`, `_many`,
  `_other`, `_two` or `_zero` as a plural form. A non-plural key with such a
  suffix would be misclassified; none exists today.
- No Go tests exist yet, so `go test ./...` reports "no test files".

### Next step
Migrate the project documentation to English (`README.md`,
`docs/project-overview.md`, `docs/progress.md`, `config/README.md`), preserving
every historical fact, and document the localization workflow.

---

## 2026-08-03 15:20 — docs: migrate project documentation to English

### Status
Completed

### Scope
Translate the project documentation from Polish to English and document the
localization workflow. English becomes the working language of the project for
code, comments, documentation, commit messages and progress entries. No
application code behaviour was changed in this commit.

### Changes

**`README.md`** — fully rewritten in English, and extended with an
"Interface languages" section covering: the supported interface languages;
English as the source and fallback language; how the language choice is stored
(`streaming-tree.language` in localStorage, validated on read); the translation
directory structure with a description of each namespace; how to add a new
translation key, including the pluralization convention; how to add a future
language as a numbered procedure; how to run `npm run i18n:check` and what it
detects; the rule that user-created stream metadata is never translated
automatically; and the rule that secrets must never appear in translation
resources. The check list, the directory tree and the demo-only table were
updated for the new files, and a troubleshooting entry was added for the case
where a label appears in English while the interface is set to Polish.

**`docs/project-overview.md`** — fully translated, plus a new section 11
"Localization" describing English as the canonical product language, Polish as
the second supported UI language, static resource-based translations, the
absence of any runtime automatic translation, and extensibility to further
languages. The stream key security section now states explicitly that the
language preference is the only value persisted in the browser. The roadmap was
updated: localization is recorded as a completed stage and the later stages were
renumbered accordingly. The manual testing section now lists the full current
check set, including `npm run i18n:check` and `npm run test`.

**`docs/progress.md`** — the journal header, the rules and the historical entry
`chore: bootstrap streaming tree project` were translated into English. All
facts were preserved: the same scope, the same twelve technical decisions, the
same validation table with the same results, the same known limitations
including every placeholder, and the same next step. Nothing was added, removed
or made to sound more complete than it was; the Vite 6 rationale, the portable
Go toolchain note and the "repository was not a Git repository at the time"
note are all still there. A "Working language" subsection was added to the rules
recording when the switch to English took effect and that the historical entry
is a translation.

**`config/README.md`** — fully translated.

**`apps/web/package.json`** — the `description` field was translated.

### Files changed
- `README.md`
- `docs/project-overview.md`
- `docs/progress.md`
- `config/README.md`
- `apps/web/package.json` (description field only)

### Technical decisions

1. **The documentation migration is a separate commit from the localization
   feature.** The first commit changes application behaviour and can be reviewed
   for correctness; this one changes only prose. Splitting them keeps the
   feature diff readable, which a combined ~5000-line commit would not be. This
   matches the commit split suggested for the task.

2. **The historical entry was translated rather than reprinted in Polish or
   replaced by a summary.** Journal rule 4 forbids rewriting history without a
   reason; the task explicitly permits translating it. A translation preserves
   every fact while making the whole journal readable in one language.
   Retaining the Polish text alongside would have doubled the file with no
   added information.

3. **Polish plural examples remain in `README.md`.** They are code samples
   showing the CLDR categories a translator must supply, not prose, so they stay
   in Polish deliberately.

4. **Language endonyms in the documentation are not translated.** "Polski" is a
   proper noun and is written the same way in an English document, matching how
   the switcher renders it.

### Automated validation

Re-run after the documentation change to confirm no regression:

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 7 namespaces, no differences against `en` |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test` | Passed - 3 files, 32 tests |
| Frontend build | `npm run build` | Passed - 2035 modules |
| Backend formatting | `gofmt -l .` | Passed - no files need formatting |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - no test files |
| Backend build | `go build ./...` | Passed - 0 errors |

Environment note: Go 1.26.5 is now installed system-wide, so the backend checks
were run with the user's own toolchain rather than the portable copy used in the
previous stage.

No manual UI testing was performed.

### Known limitations
- The documentation describes the localization workflow but there is no
  automated check that the documentation itself stays in sync with the code; a
  renamed namespace would have to be corrected in `README.md` by hand.
- The English documentation has not been reviewed by a second reader.
- All product limitations recorded in the two previous entries still stand. This
  commit changed no application behaviour.

### Next step

Stage 3: persistent configuration storage, unchanged from the plan recorded in
the first entry.

1. Adding SQLite to the backend together with schema migrations.
2. A platform and metadata model on the backend side, as the source of truth
   instead of demo data in the browser.
3. REST API: `GET/POST/PUT/DELETE /api/platforms` and
   `GET/PUT /api/platforms/{id}/metadata`, with validation based on the same
   capability table as the frontend.
4. Replacing the demo store in `src/state/` with TanStack Query requests, keeping
   the current presentational components.
5. Keeping the `Backend unavailable` state as a fully handled path.

The capability table labels are already translation keys, so moving that data to
the backend must keep sending keys (or capability identifiers) rather than
display text, so the server never decides the interface language.

---

## 2026-08-03 16:10 — feat(server): add SQLite persistence and migrations

### Status
Completed

### Scope
Introduce the persistence foundation: SQLite storage, an embedded migration
runner, the schema for configured platforms and their metadata, the built-in
provider registry, and the domain layer that validates and orchestrates them.
This entry covers storage and domain only; the REST API that exposes them is
the next commit.

### Changes

**Database location and connection (`internal/config`, `internal/storage/sqlite`)**
- Added `STREAMING_TREE_DB_PATH` (full path to the file) and
  `STREAMING_TREE_DATA_DIR` (directory that will hold the default filename).
  `STREAMING_TREE_DB_PATH` wins when both are set.
- Without either, the path is derived from `os.UserConfigDir()` plus
  `StreamingTree/streaming-tree.db`, so the default lands outside the working
  copy and a repository clone never accumulates a database file.
- Relative paths are made absolute at load time.
- The parent directory is created automatically with `0o700`.
- `foreign_keys`, `busy_timeout`, WAL journal mode and `synchronous=NORMAL` are
  requested through the DSN so they apply to every pooled connection, and are
  then read back and verified. A missing `foreign_keys` or `busy_timeout` is a
  startup failure, never a silent warning, because cascading deletes depend on
  it. The journal mode is recorded rather than enforced, since WAL is
  unavailable on some filesystems and is a performance choice, not a
  correctness one.
- Connection pool capped at 4 connections: SQLite serialises writers, so a
  large pool would only increase lock contention.

**Migration runner**
- Migrations are `.sql` files embedded with `go:embed`, named
  `<version>_<name>.sql`. No external migration CLI exists or is needed.
- Order is by numeric version, so it never depends on filesystem iteration.
  Duplicate versions are rejected at load time.
- Each migration runs inside its own transaction together with its
  `schema_migrations` insert, so a failure rolls back the schema change and the
  bookkeeping row alike: a failed migration is never recorded as applied and is
  retried on the next start.
- `schema_migrations` records version, name and applied timestamp.
- Pending migrations run automatically at startup.

**Schema (migration 0001)**
- `platforms`: `id` TEXT primary key, `provider_id`, `display_name`, `enabled`,
  `sort_order`, `created_at`, `updated_at`, with indexes on
  `(sort_order, created_at, id)` for the dashboard ordering and on
  `provider_id` for lookup. The provider index is deliberately non-unique
  because several destinations may share one provider.
- `platform_metadata`: one row per platform, cascading on delete. Every
  optional column is nullable, and NULL specifically means "the provider does
  not support this field", which is a different statement from an empty string
  the user actually left blank.
- `platform_metadata_tags`: separate ordered rows with
  `PRIMARY KEY (platform_id, position)` and a unique index on
  `(platform_id, lower(value))`, so case-insensitive tag uniqueness is enforced
  by the database as well as by the domain layer. Tags cascade on delete.
- Timestamps are RFC 3339 with nanosecond precision in UTC, stored as TEXT
  because that format sorts lexicographically in chronological order.

**Seed (migration 0002)**
- Four configured platforms, one per provider, with stable predefined IDs
  (`pf_seed_twitch` and so on), unique sort orders 0-3, and example metadata
  mirroring what the dashboard previously showed.
- All four are disabled. No runtime state, no stream key, no token, no
  credential is seeded.
- Twitch is the only seeded platform with tags, stored in order.
- Because the seed is an ordinary migration recorded in `schema_migrations`, it
  runs exactly once: deleting a seeded platform is permanent and a restart does
  not bring it back.

**Domain layer (`internal/domain/platform`)**
- `model.go` separates the three concepts explicitly: built-in
  `ProviderDefinition`, user-created `Platform`, and runtime state - which is
  deliberately absent from both.
- `definitions.go` holds the built-in registry for Twitch, YouTube, Kick and
  TikTok, moved here from the frontend so the backend is the single source of
  truth for capabilities. Twitch retains tag support; it is still the only
  provider with it. The definitions carry only semantic identifiers
  (`category`, `topic`, `public`, `ultra-low`), never localized labels.
- `validation.go` validates display names, sort orders and metadata against the
  provider's capability table, limits and option lists. A field the provider
  does not support is rejected when it carries a meaningful value and silently
  reset when empty, so a client that always sends the whole object does not
  need to know the capability table.
- `errors.go` provides typed domain errors (not found, unknown provider,
  conflict, storage) plus a `ValidationError` carrying per-field violations
  with a stable rule identifier, English fallback message and parameters.
- `service.go` owns ID generation, timestamps and the use cases. IDs are
  random 16-byte values prefixed `pf_`, never sequential integers, so they do
  not leak how many destinations exist or invite enumeration.

**Repository (`internal/storage/sqlite/platform_repository.go`)**
- Implements the domain's `Repository` port. Every driver error is converted
  into a domain sentinel, so no SQLite text can reach an HTTP response.
- `List` loads all platforms and all tags in two queries regardless of how many
  destinations exist, avoiding an N+1 pattern.
- `Create` inserts the platform and its metadata row in one transaction, so a
  platform can never exist without exactly one metadata record.
- `SaveMetadata` replaces the metadata row and the whole ordered tag list in a
  single transaction.

**Startup wiring**
- `main.go` opens the database, logs the resolved path and journal mode, runs
  pending migrations and closes the database on every exit path including a
  failed migration. The signal context is created before the database so a
  signal during startup still unwinds cleanly.
- The logged path contains no credentials, because the application stores none.

### Files changed
- `apps/server/go.mod`, `go.sum` (added `modernc.org/sqlite`)
- `apps/server/internal/config/config.go`, `config_test.go`
- `apps/server/internal/storage/sqlite/` - `database.go`, `migrations.go`,
  `platform_repository.go`, `errors.go`, `migrations/0001_initial_schema.sql`,
  `migrations/0002_seed_default_platforms.sql`, and four test files
- `apps/server/internal/domain/platform/` - `model.go`, `definitions.go`,
  `errors.go`, `repository.go`, `service.go`, `validation.go`, `unicode.go`,
  `validation_test.go`
- `apps/server/cmd/server/main.go`

### Technical decisions

1. **`modernc.org/sqlite` rather than `mattn/go-sqlite3`.** It is a pure-Go
   translation of SQLite, so the server still builds with plain `go build` and
   cross-compiles without a C toolchain. The cost is a larger binary and
   slightly lower throughput, neither of which matters for a local single-user
   control panel.

2. **The Go directive moved from 1.22 to 1.25.** `modernc.org/sqlite` requires
   it. The router still only needs 1.22 for method-aware patterns; the comment
   in `go.mod` now records both facts. `README.md` is updated in the
   documentation commit.

3. **No ORM.** `database/sql` with hand-written SQL in the repository layer.
   The query surface is small and explicit, and it keeps the N+1 avoidance in
   `List` visible instead of hidden behind lazy loading.

4. **Provider definitions are code, not rows.** `platforms.provider_id` is
   therefore not a foreign key. Providers ship with the binary, cannot be
   created or deleted by a user, and validating against a table would imply
   otherwise.

5. **NULL means "unsupported", not "empty".** The repository writes NULL for
   every field the provider's capability table disables, and reads NULL back as
   the Go zero value. This keeps the database self-describing: a row shows at a
   glance which fields the provider ever had.

6. **Case-insensitive tag uniqueness is enforced twice.** The domain rejects it
   with a helpful field error; the unique index is the backstop that makes a
   bug in that logic a failed transaction rather than corrupted data. A
   repository test bypasses the domain deliberately to prove the transaction
   rolls the metadata write back too.

7. **Random identifiers with a `pf_` prefix.** Sequential IDs would be a public
   enumeration surface, and the prefix makes an identifier recognisable in logs
   and support requests.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Backend formatting | `gofmt -l .` | Passed - one file reformatted, then clean |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - config, domain and storage packages |
| Backend build | `go build ./...` | Passed - 0 errors |

Tests added in this scope: 8 configuration tests (default path, data-dir
override, `STREAMING_TREE_DB_PATH` precedence, relative-path resolution,
invalid ports, origins, blank values), 6 database tests (parent directory
creation, foreign keys on, busy timeout, journal mode, empty path rejected,
unusable path rejected), 8 migration tests (empty database, idempotence,
recorded metadata, failed migration not recorded and not partially applied,
ordering, seed applied once, deleted seed not recreated, seed tag order, NULL
for unsupported fields) and 15 repository tests (ordering, tag loading, not
found, second configuration per provider, metadata row creation, duplicate ID
conflict, update, cascade deletes, ordered tag save and replacement,
transaction rollback, Unicode preservation, next sort order).

No manual testing was performed.

### Known limitations
- The provider capability tables are still the approximate values carried over
  from the frontend. They have NOT been verified against the real Twitch,
  YouTube, Kick or TikTok APIs, and the code says so.
- No HTTP endpoint exposes any of this yet; that is the next commit.
- `go test ./...` still reports "no test files" for `cmd/server` and
  `internal/buildinfo`.
- WAL mode is requested but not required, so a database on a filesystem that
  refuses WAL falls back silently to another journal mode. The mode actually in
  use is logged at startup.

### Next step
Expose the persisted configuration over REST: provider definitions, platform
CRUD and metadata replacement, with the existing error envelope extended by an
optional per-field map.

---

## 2026-08-03 16:35 — feat(server): add platform configuration API

### Status
Completed

### Scope
Expose the persisted configuration over REST: built-in provider definitions,
full CRUD for configured platforms, and atomic metadata replacement. Adds the
scripted restart-persistence verification that proves the whole path works
across a process restart.

### Changes

**Endpoints**
- `GET /api/platform-definitions` - all four built-in definitions, as semantic
  identifiers and capability data only.
- `GET /api/platforms` - configured platforms ordered by sort order then
  creation time then id, each with its provider definition and metadata
  inlined, so the dashboard renders from one request and needs no per-card
  metadata call.
- `POST /api/platforms` - creates a configuration, responds 201 with a
  `Location` header and the complete record.
- `GET /api/platforms/{id}`, `PUT /api/platforms/{id}`,
  `DELETE /api/platforms/{id}` - read, full replacement of the mutable fields,
  and delete returning 204 with cascading metadata and tag removal.
- `GET /api/platforms/{id}/metadata`, `PUT /api/platforms/{id}/metadata` -
  read and atomic replacement of metadata plus ordered tags.

**Request handling**
- Bodies are capped at 64 KiB and answered with 413 when exceeded.
- Unknown JSON fields are rejected with 400 rather than silently dropped, so a
  client that tries to send a `streamKey` gets an error instead of the field
  quietly disappearing. The rejected value is not echoed back.
- Malformed JSON, a wrong content type and trailing JSON after the object are
  each distinguished from validation failures.
- All write endpoints take full replacement payloads, avoiding the ambiguity of
  partial PATCH semantics.

**Error contract**
- The existing `{error, message}` envelope is preserved everywhere.
- Validation failures add `fields` (one English fallback sentence per field,
  matching the documented shape) and `details` (a stable `rule` plus `params`
  per field), both built from one internal violation list so they cannot drift
  apart. The frontend localizes from `details` and falls back to `fields`.
- Status codes: 400 malformed JSON or unknown field, 404 missing record, 405
  unsupported method with an `Allow` header, 409 genuine conflict, 413
  oversized body, 415 wrong content type, 422 validation failure, 500
  unexpected internal failure.
- Storage failures are logged with their cause and answered with a generic
  message; a test asserts that no response mentions SQLite, SQL keywords,
  constraints or the database file.

**Route matching**
- Every path is registered twice: with its allowed methods, and once bare so a
  wrong verb produces 405 with `Allow` instead of falling through to the
  `/api/` catch-all as 404. Existing health, 404 and 405 behaviour is covered
  by tests to prove it did not regress.

**Scripted persistence verification (`scripts/verify-persistence.mjs`)**
- Starts the real backend against a temporary database, exercises definitions,
  the seed, create, update, metadata save with ordered tags and read-back,
  stops the process, restarts it against the same file, verifies everything
  survived and that the seed did not run again, deletes the created platform,
  confirms the 404, stops the backend and removes the temporary directory.
- It never opens the real user database.

### Files changed
- `apps/server/internal/httpapi/` - new `platforms.go`, `platform_metadata.go`,
  `decode.go`, `errors.go`, `platforms_test.go`; modified `router.go` and
  `respond.go`
- `apps/server/cmd/server/main.go` (service wiring)
- `scripts/verify-persistence.mjs`

### Technical decisions

1. **The commit split follows the suggested boundary.** Storage and domain
   landed first, the HTTP surface second. `main.go` is touched by both because
   the first commit opens and migrates the database while the second injects
   the service into the router; the intermediate state compiles and passes its
   own tests.

2. **`fields` and `details` rather than one map.** The task specifies `fields`
   as a field-to-message map, and separately requires that the frontend
   localize field validation identifiers. An English sentence cannot be
   localized, so `details` carries the stable rule and its parameters
   alongside. Both are derived from the same violation list in one place.

3. **Only the first violation per field is reported.** Forms show one message
   per input, so additional violations for the same field would be discarded by
   the client anyway.

4. **Handler tests run against a real database.** `httptest` drives the real
   router over the real service and repository, each test with its own
   temporary file. Testing through the whole stack is what catches a route,
   serialization or transaction mistake; a mocked service would not.

5. **The provider definition is inlined into every platform response.** It
   costs a few hundred bytes and removes a second round trip plus a
   client-side join for every card.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Backend formatting | `gofmt -l .` | Passed - no files need formatting |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - all packages |
| Backend build | `go build ./...` | Passed - 0 errors |
| Restart persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps, 30 assertions, exit code 0 |

24 handler tests were added covering every endpoint, malformed JSON, unknown
fields, an oversized body, an unknown provider, missing records, validation
field details with parameters, 405 with `Allow` on four paths, JSON content
type, the preserved 404 and health behaviour, no SQLite leakage and no
credential-shaped fields in any payload.

No manual testing was performed.

### Known limitations
- The frontend still uses its own demo platform configuration; it is switched
  over in the next commit, so until then two sources of truth exist in the
  repository.
- There is no pagination or filtering on `GET /api/platforms`. A local control
  panel has a handful of destinations, so it would be unused complexity.
- `PUT /api/platforms/{id}` replaces the mutable fields only; the provider of
  an existing configuration cannot be changed. Deleting and recreating is the
  intended path, since metadata validity depends on the provider.
- The seeded and created configurations still have no runtime state, and the
  API exposes none, because no streaming engine exists.

### Next step
Replace the frontend demo store with real API data: TanStack Query hooks with
Zod validation, an Add Platform dialog, a platform settings editor with
deletion, a metadata editor wired to the API, and removal of the fake
Start/Stop transitions.

---

## 2026-08-03 17:35 — feat(web): persist platforms and metadata

### Status
Completed

### Scope
Replace the frontend's demo platform configuration with real API data. The
backend becomes the only source of truth for provider capabilities and
configured destinations; the fake Start/Stop lifecycle is removed.

### Changes

**Data layer**
- `src/api/platform-schemas.ts` holds Zod contracts for provider definitions,
  configured platforms and metadata; `src/api/platforms.ts` is a thin transport
  layer. Every response is validated before it reaches a component.
- `api-client.ts` gained POST/PUT/DELETE, a parsed error envelope including
  `fields` and `details`, and a failure taxonomy the UI can act on: `network`,
  `timeout`, `http`, `parse`, `validation`, `not-found`, `server`.
- `src/hooks/use-platforms.ts` exposes `usePlatformDefinitionsQuery`,
  `usePlatformsQuery` and create/update/delete/metadata mutations. Cache
  updates live in `platform-cache.ts` as pure functions. Nothing calls
  `window.location.reload()`.

**Provider identifiers to labels**
- `src/models/provider-labels.ts` maps backend identifiers (`category`,
  `topic`, `public`, `ultra-low`) onto translation keys. Every lookup is total:
  an unrecognised identifier returns null and the raw identifier is rendered,
  so a newer backend degrades to "an unfamiliar word" rather than a crash.
- Stream languages are rendered as endonyms; a platform whose provider this
  build does not know renders a clear warning instead of failing.

**Dashboard**
- Cards show configuration only: destination name, brand, saved title, saved
  category, and enabled/disabled. Fake viewer counts and fake connection
  quality are gone; showing invented numbers next to real saved data would be
  misleading.
- Start is disabled with a localized "Streaming engine not implemented"
  explanation. The metadata and settings actions are real.
- The counters card now counts configuration (total / enabled / disabled) and
  states explicitly that no live state is shown.
- Explicit states for loading, empty, load failure and definitions
  unavailable, each with a retry where it makes sense. When the backend is
  down the shell keeps working and the page says the configuration lives in the
  backend database - it never falls back to demo platforms.

**Add Platform and settings**
- A new accessible `Modal` primitive: focus moves in on open and returns to the
  trigger on close, Tab is trapped, Escape closes when safe, background scroll
  is locked.
- `AddPlatformDialog` selects a provider, takes a display name and an enabled
  flag. Duplicate providers are allowed. No stream key is requested, and the
  dialog says so.
- `PlatformSettingsDialog` edits display name, enabled state and sort order,
  and deletes behind `ConfirmDialog`. `window.confirm` is not used anywhere.
- Both show backend field errors next to the field and general failures above
  the form, and block double submits while a request is in flight.

**Metadata editor**
- Tabs come from configured platforms; capabilities, limits and option lists
  come from the backend definition. Save performs a real PUT.
- Switching tabs with unsaved edits asks for confirmation rather than silently
  discarding them. Deleting the selected platform moves the selection.
- Client-side validation still runs for immediate feedback, driven entirely by
  the backend-supplied capability data, with the backend as the authority.

**Removed**
- `src/data/demo-platforms.ts` and the whole `src/state/` demo store. Only
  `demo-system.ts` remains, for host metrics that are still unimplemented and
  still labelled Demo.

### Files changed
- Added: `src/api/`, `src/hooks/use-platforms.ts`, `platform-cache.ts`,
  `use-api-field-errors.ts`, `src/lib/field-error-rules.ts`,
  `src/models/provider-labels.ts`, `platform-constraints.ts`,
  `src/components/ui/Modal.tsx`, `ConfirmDialog.tsx`,
  `src/components/platforms/AddPlatformDialog.tsx`,
  `PlatformSettingsDialog.tsx`, `add-platform-validation.ts`,
  `src/components/metadata/metadata-draft.ts`, and seven test files
- Modified: `PlatformCard`, `PlatformGrid`, `PlatformGlyph`, `MetadataEditor`,
  `MetadataForm`, `PlatformTabs`, `StreamCountersCard`, `SystemStatusPill`,
  `SystemStatusRail`, `DashboardPage`, `App.tsx`, `api-client.ts`,
  `models/platform.ts`, `models/metadata-schema.ts`, and the `en`/`pl`
  `common`, `dashboard`, `metadata` and `platforms` namespaces
- Removed: `src/data/demo-platforms.ts`, `src/state/`

### Technical decisions

1. **Unknown identifiers degrade, never throw.** Zod keeps option identifiers
   as plain strings rather than enums, and the label mappings return null for
   anything unrecognised. A backend that adds an option must not blank the
   dashboard of an older frontend.

2. **Runtime status was removed rather than faked.** The previous cards showed
   a `starting -> live` transition, viewer counts and connection quality, all
   invented. Beside genuinely persisted configuration those would read as real,
   so they are gone. `PlatformStatus` survives only for system-level states
   (checking, backend unavailable).

3. **Validation logic extracted into pure modules.** Add Platform validation,
   the unsaved-changes rule, the cache updates and the field-error mapping are
   plain functions, tested directly. This covers the required behaviour without
   adding a browser automation suite or asserting on markup that refactoring
   would break.

4. **Field errors are localized from `details`, with the English `fields`
   sentence as the fallback.** An unmapped rule still produces a sentence, never
   a rule identifier.

5. **Metadata saves patch the cache; configuration changes refetch.** A metadata
   save cannot affect ordering, so patching is enough. `sortOrder` can reorder
   the whole list, so those mutations invalidate and let the backend decide.

6. **A local mirror of two backend limits** (`platform-constraints.ts`) drives
   `maxLength` and instant feedback. It is documented as a mirror, and the
   backend validates everything again.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 7 namespaces, no differences |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test` | Passed - 12 files, 133 tests |
| Frontend build | `npm run build` | Passed |
| Backend tests | `go test ./...` | Passed - unchanged by this commit |
| Restart persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |

52 frontend tests were added: provider-definition, configured-platform and
metadata Zod parsing including unknown identifiers and a missing provider;
identifier-to-key mapping with unknown and hostile input; capability-driven
validation including Twitch tags, duplicates, limits and unsupported options;
API client failure classification and the validation envelope; localized error
codes with the English fallback; backend field-error mapping; Add Platform form
validation; unsaved-change detection including tag reordering; and cache
behaviour after create, update and delete.

One lint warning was found and fixed: the platform list needed `useMemo` so its
identity did not change on every render.

No manual testing was performed.

### Known limitations
- The production bundle is now ~505 KB (151 KB gzipped) and Vite warns about
  chunk size. It is a local application loaded from disk, so this is not worth
  code-splitting yet.
- Sort order is edited as a number in the settings dialog; there is no
  drag-and-drop reordering.
- The provider of an existing destination cannot be changed, matching the API.
- Host metrics in `demo-system.ts` remain demo values, isolated and labelled.
- Provider capability tables are still the approximate values, now served by the
  backend, and still unverified against real platform APIs.
- The frontend has no component-level rendering tests; behaviour is covered
  through the extracted pure modules instead.

### Next step
Update the documentation for persistent configuration: database locations per
operating system, the new environment variables, migrations, seeding, the API
endpoints, development-database handling and the corrected record of the
`.gitignore` incident.

---

## 2026-08-03 18:05 — docs: document persistent configuration

### Status
Completed

### Scope
Document the persistence layer across `README.md`,
`docs/project-overview.md` and `config/README.md`, and record a correction to an
earlier entry. No code changed in this commit.

### Changes

**`README.md`**
- The project-state banner now says configuration and metadata persist across a
  refresh and a restart, while streaming remains unimplemented.
- New **Data storage** section: SQLite and the CGO-free driver, the path
  resolution order, a table of default locations for Windows, macOS and Linux,
  the startup log line, automatic migrations, the one-time seed and the fact
  that a deleted seeded destination stays deleted, how to use a development
  database, how to reset one including the WAL sidecar files, and a prominent
  warning that deleting a database permanently deletes configuration and
  metadata with no undo.
- New **REST API** section listing all nine endpoints, the error envelope, the
  validation envelope with field details, the status codes, the rule that
  definitions carry semantic identifiers rather than translated text, and the
  rule that no endpoint accepts or returns credentials.
- The two new environment variables were added to the backend configuration
  table.
- The check list gained the backend test note about temporary databases and the
  scripted persistence command.
- The demo-only table was rewritten: Start is disabled, and live status, viewer
  counts and connection quality are recorded as **removed** rather than demo. A
  "What is real" list was added.
- The directory tree and troubleshooting section were updated, including
  "my destinations disappeared" and "a deleted seeded destination did not come
  back".

**`docs/project-overview.md`**
- Section 7.3.1 describes the implemented storage role, what the database holds
  and what it deliberately does not: runtime state and credentials.
- New section 8.1 separates provider definition, configured platform and runtime
  stream state.
- Section 9 records that the capability table now lives in the backend and that
  the frontend keeps no competing copy, and section 9.1 states the localization
  boundary explicitly.
- The roadmap marks stage 3 completed, noting it was marked so only after the
  restart-persistence verification passed. MediaMTX, FFmpeg and credential
  storage remain planned.
- The manual-testing section lists the persistence script; the stream key
  section records that the database has no credential columns.

**`config/README.md`**
- Records what deliberately does not belong there: the database (user data,
  outside the repository) and the migrations (embedded in the binary so the
  schema cannot drift from the code that reads it).

### Correction to an earlier entry

The entry `feat(web): add English and Polish localization` states, under
"Repository fix found while staging this change", that **two** source files were
added after the `.gitignore` fix. That count is wrong.

`git show --diff-filter=A 61b9457 -- apps/web/src/data` shows **three** files
were added: `app-info.ts`, `demo-platforms.ts` and `demo-system.ts`. The "Files
changed" list in the same entry also omits `app-info.ts`.

Per journal rule 4 the historical entry is left as written and corrected here
instead. Nothing else in that entry is affected: the `.gitignore` rules, the
cause and the fix are all recorded accurately, and the omission did not change
what was committed.

### Files changed
- `README.md`
- `docs/project-overview.md`
- `docs/progress.md`
- `config/README.md`

### Technical decisions

1. **The commit split follows the suggested four-way boundary**: persistence,
   API, frontend, documentation. Documentation is last so it describes what was
   actually built rather than what was planned.

2. **The database-deletion warning is prominent rather than a footnote.** The
   file now holds the only copy of a user's configuration, there is no backup,
   and the reset instructions sit directly above it.

3. **The correction is a new entry, not an edit.** Journal rule 4 forbids
   rewriting history without reason. A miscounted file is worth correcting for
   the record but not worth altering a past entry over.

### Automated validation

Re-run after the documentation change to confirm no regression:

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 7 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test` | Passed - 12 files, 133 tests |
| Frontend build | `npm run build` | Passed |
| Backend formatting | `gofmt -l .` | Passed - no files need formatting |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - 4 packages |
| Backend build | `go build ./...` | Passed - 0 errors |
| Restart persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |

No manual testing was performed.

### Known limitations
- Nothing checks automatically that the documentation stays in sync with the
  code; a renamed endpoint or environment variable must be corrected in
  `README.md` by hand.
- The default database locations are documented from the implemented
  `os.UserConfigDir()` behaviour and verified on Windows only. The macOS and
  Linux paths follow the documented Go behaviour but were not observed on those
  systems.
- The English documentation has not been reviewed by a second reader.
- All product limitations from the three previous entries still stand.

### Next step

Stage 4: MediaMTX integration.

1. Bundle or locate the MediaMTX binary and generate its configuration from
   `config/`.
2. Supervise it as a child process with health checks and graceful shutdown.
3. Detect when OBS connects to the local RTMP ingest and report it through the
   API, replacing the placeholder OBS panel.
4. Introduce a runtime-state model that is explicitly separate from the
   configuration tables, kept in memory rather than persisted.

The separation recorded in section 8.1 of the project overview is the constraint
to respect here: runtime state must not leak into the SQLite configuration
schema.
