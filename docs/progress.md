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

---

## 2026-08-03 19:05 — feat(server): add managed MediaMTX dependency

### Status
Completed

### Scope
Locate, download and verify the MediaMTX binary. This entry covers the pinned
version, the platform asset matrix, the binary resolver, the secure installer
and the third-party licence record. Process supervision and the runtime API are
the next commit.

### Changes

**Pinned version**
- `SupportedVersion = "v1.19.3"` is declared once, in
  `internal/runtime/mediamtx/version.go`, and every asset name, install path and
  compatibility check derives from it. No code resolves a `latest` release.
- `ParseVersionOutput` reads `mediamtx --version`, which prints a bare `v1.19.3`.
  Anything unparseable is rejected rather than guessed at, and untrusted output
  is truncated before it reaches a message or a log.
- An incompatible binary is never started by default. It resolves to an
  `mediamtx_incompatible_version` state whose English message names both the
  version found and the version expected.

**Platform matrix**
- Official assets are mapped for windows/amd64, linux/amd64, linux/arm64,
  darwin/amd64 and darwin/arm64. The names were verified against the published
  `checksums.sha256` for v1.19.3 rather than assumed.
- An unmapped OS/architecture produces a stable `mediamtx_unsupported_platform`
  error naming the override variable, instead of a guessed asset name that would
  surface later as a confusing 404.

**Binary resolution**
- Order: `STREAMING_TREE_MEDIAMTX_PATH`, then the managed installation, then
  missing. The system PATH is deliberately **not** searched.
- The override path is made absolute, must exist, must be a regular file, and
  must carry an executable bit on Unix. Its version is probed like any other.
- Managed installations live at
  `<data dir>/runtime/mediamtx/v1.19.3/<os>-<arch>/`, so versions and platforms
  sit side by side and a future upgrade is install-then-switch rather than an
  in-place overwrite of a running binary.
- The resolved filesystem path is kept out of every API response.

**Installer**
- Installation is an explicit action; nothing downloads at startup.
- Sequence: fetch `checksums.sha256`, find the exact entry for the selected
  asset, download the archive while hashing it in one pass, compare, extract
  into a temporary directory, locate the executable and `LICENSE`, set the
  executable bit, run `--version` once to verify, then move the staged directory
  into place with a rename.
- Nothing unverified is ever executed: the checksum is compared before the
  archive is opened, and the binary is only run after extraction has already
  been constrained.
- Bounded throughout: 10-minute install timeout, 60-second checksum timeout,
  5 redirects, 128 MB archive, 64 KB checksum manifest, 256 MB extraction,
  256 archive entries.
- Response bodies are never logged - they are either a checksum manifest or,
  when something upstream fails, an arbitrary error page.

**Archive safety**
- Rejected: absolute entry paths, Windows drive-letter paths, any `..` segment,
  entries resolving outside the extraction root, symlinks, hard links, and
  non-regular entries.
- Links are refused rather than validated. The official archives contain none,
  so accepting them would add attack surface for no benefit.
- `O_EXCL` on every extracted file, so a duplicated archive entry fails loudly
  instead of overwriting the first copy.

**Failure behaviour**
- The temporary directory is removed after success and failure alike.
- An existing installation is moved aside, not deleted, and restored if the
  final rename fails - so a failed reinstall cannot degrade a working setup.
- A failed install never leaves a directory that would look valid to the
  resolver.

**Configuration**
- `STREAMING_TREE_DATA_DIR` now resolves the application data directory in its
  own right, and the database path is derived from it. `STREAMING_TREE_DB_PATH`
  still wins for the database file.
- Added `STREAMING_TREE_MEDIAMTX_PATH`, `..._AUTOSTART`, `..._AUTO_RESTART`,
  `..._RTMP_ADDRESS`, `..._API_ADDRESS` and `STREAMING_TREE_INGEST_PATH`, all
  validated at load time.
- Both listener addresses must be loopback. MediaMTX accepts an unauthenticated
  publisher and its Control API can rewrite its own configuration, so a routable
  bind address is refused outright rather than warned about.
- The RTMP and Control API addresses must differ. The ingest path is restricted
  to letters, digits, `-` and `_`, excluding slashes, relative segments, query
  strings and the MediaMTX wildcard names `all` and `all_others`.
- A malformed boolean is an error, not a silent `false`: silently disabling
  autostart would look like an application bug.

**Third-party licence**
- Added `THIRD_PARTY_NOTICES.md` recording MediaMTX, the pinned version, its MIT
  licence, that it is downloaded from the official release, and where the
  installed `LICENSE` lives. An archive without a licence file is rejected.

**Repository fix found while staging this change**
- `.gitignore` contained an unanchored `mediamtx` rule, added during the
  bootstrap stage to ignore a manually downloaded binary. It matched the new
  source package `apps/server/internal/runtime/mediamtx/`, which would have been
  committed empty - the same failure mode as the unanchored `data/` rule that
  once hid `apps/web/src/data/`.
- The third-party binary rules are now anchored to the repository root
  (`/mediamtx`, `/mediamtx.exe`, `/ffmpeg`, `/ffmpeg.exe`). Managed binaries are
  installed into the per-user data directory and never into the working copy, so
  these rules only guard against a manual download left at the root.
- Verified afterwards that `git status --ignored` reports nothing inside
  `apps/`, `config/`, `docs/` or `scripts/`.

### Files changed
- `apps/server/internal/runtime/mediamtx/` - `version.go`, `platform.go`,
  `errors.go`, `resolver.go`, `archive.go`, `installer.go`, `util.go`, and
  three test files
- `apps/server/internal/config/config.go`, `config_test.go`
- `THIRD_PARTY_NOTICES.md`

### Technical decisions

1. **The system PATH is not searched.** On a developer machine it could pick up
   an unrelated or unsupported build. This application starts the binary as a
   long-lived child process with a generated configuration, so it should only
   ever run a copy it can identify.

2. **The release base URL is a constant, overridable only in Go.**
   `WithReleaseBaseURL` exists so tests can serve fixtures from `httptest`. It
   is not reachable from any HTTP request: the install endpoint will accept no
   body, so a browser cannot influence where a download comes from.

3. **The version is verified twice.** Once on the freshly extracted binary
   before it is installed, and again by the resolver on every startup. The first
   stops a wrong archive from ever landing; the second catches an override or a
   directory replaced behind the application's back.

4. **The asset matrix omits linux/armv6 and armv7**, which upstream does publish.
   They are untested here, and the task scope lists five platforms. An explicit
   unsupported-platform error plus the override variable is more honest than an
   untested mapping.

5. **Staging happens inside the runtime directory**, not the system temp
   directory, so the final publish is a rename on the same filesystem rather
   than a cross-device copy that could fail halfway.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Backend formatting | `gofmt -l .` | Passed - no files need formatting |
| Backend build | `go build ./...` | Passed - 0 errors |
| Backend tests | `go test ./internal/config/... ./internal/runtime/...` | Passed |

Tests added: 8 version and platform tests (pinned value, real `--version`
format, malformed output, hostile output truncation, the five required asset
names verified against the published checksums, unsupported platforms),
12 resolver tests (missing, managed, override precedence, non-existent override,
directory as override, incompatible version, unreadable version, unsupported
platform, versioned install path, metadata parsing) and 18 installer tests
covering asset selection per platform, checksum success, mismatch, missing
entry, malformed manifest, oversized response, six unsafe-archive shapes,
missing executable, missing licence, wrong version, staging cleanup, atomic
install and an existing installation surviving a failed reinstall.

No archive built by a test is ever executed: the version probe is injected.

No manual testing was performed.

### Known limitations
- linux/armv6 and linux/armv7 are published upstream but not mapped here.
- The installer verifies the archive checksum against the official manifest, but
  the manifest itself is trusted because it is fetched over HTTPS from the same
  release. Signature verification is not implemented.
- Nothing supervises MediaMTX yet; that is the next commit.
- Concurrent-install protection lives in the supervisor, so it is not yet
  enforced at this layer.

### Next step
Supervise the process: generate the v1.19.3 configuration, start MediaMTX as a
child process, confirm readiness through its Control API, poll `/v3/paths/list`
for OBS ingest, and expose all of it through a runtime API.

---

## 2026-08-03 21:15 — feat(server): supervise MediaMTX runtime and ingest

### Status
Completed

### Scope
Generate the MediaMTX configuration, run it as a supervised child process,
confirm readiness through its Control API, detect RTMP ingest, and expose all of
it through a public runtime API. Adds the real-binary smoke verification.

### Changes

**Generated configuration**
- `RenderConfig` emits a deterministic v1.19.3 configuration covered by a golden
  test. Every key was verified against the reference configuration shipped in
  the official archive: MediaMTX rejects an unknown key and refuses to start, so
  a typo here would be a startup failure rather than a warning.
- Enabled: RTMP on the configured loopback address, and the Control API on its
  own loopback address. Disabled explicitly: `rtsp`, `hls`, `webrtc`, `srt`,
  `moq`, `metrics`, `pprof`, `playback`. Each opens its own listener by default,
  so none is left to the upstream default.
- Exactly one path accepts publishing, with `source: publisher` and
  `overridePublisher: false` - MediaMTX defaults that to `true`, which would let
  a second publisher silently displace the first. No wildcard path exists.
- `record: false`, no forwarding, no destination, no credential.
- `logStructured: true`, so the supervisor can parse severity and message.
- Written atomically into the runtime directory with restrictive permissions,
  never into the repository.

**Control API client**
- Targets v1.19.3 and keeps its wire models internal; they are not the public
  Streaming Tree schema.
- Readiness uses `/v3/config/global/get`. v1.19.3 has no dedicated instance-info
  endpoint, so a valid global-configuration answer is what proves the API is up
  and the process finished loading. The version is verified from
  `mediamtx --version` at resolve and install time instead, and this is
  documented rather than glossed over.
- Ingest uses `/v3/paths/list`. Unknown fields are tolerated; `name` and `ready`
  are required, because treating a missing `ready` as `false` would report
  "waiting" during an API fault.
- Bounded: 3-second timeout, 4 MB response limit, loopback only. No shelling out
  to curl, and no MediaMTX command hooks.

**Process supervision**
- Explicit state machine: `missing`, `installing`, `incompatible`, `stopped`,
  `starting`, `ready`, `stopping`, `error`. One value, not a set of booleans, so
  "ready and missing" is unrepresentable.
- Both output streams are drained by their own goroutine. An undrained pipe
  would wedge MediaMTX a few kilobytes into its logging, so this is a
  correctness requirement rather than a nicety. Structured lines are parsed;
  malformed lines are logged raw and never panic. A 100-line ring bounds memory.
- Readiness is never inferred from process creation: MediaMTX exits milliseconds
  later if its configuration is rejected. The Control API must answer first.
- Ports are checked before spawning, so "address already in use" becomes an
  actionable `mediamtx_port_in_use` rather than a readiness timeout. Nothing is
  ever terminated to free a port.
- The child gets a minimal environment and its own process group.
- A generation counter makes late callbacks from a superseded process
  harmless, so a stop during startup cannot be undone by the launch it raced.

**Restart policy**
- Bounded exponential backoff from 1s to 30s, at most 5 restarts in 5 minutes,
  and a 60-second stable run resets both. A crash loop therefore stops with
  `mediamtx_restart_limit_reached` instead of spinning.
- An explicit Stop sets a flag the policy honours, so a deliberate stop is never
  undone. Restart is one controlled stop followed by a start.

**Shutdown**
- The HTTP server drains first, then MediaMTX, so an in-flight runtime request
  cannot restart it on the way out. Workers are awaited, so no goroutine and no
  child process outlives the backend.
- On Unix, SIGTERM then SIGKILL after a grace period - genuinely graceful. On
  Windows the process is terminated immediately: there is no SIGTERM, and
  MediaMTX is a console application with no message loop. `process_windows.go`
  says so plainly rather than claiming graceful shutdown everywhere, and the
  README repeats it.

**Public runtime API**
- `GET /api/runtime` returns one versioned snapshot: MediaMTX state, ingest
  state and connection details. `POST /api/runtime/mediamtx/{install,start,stop,restart}`.
- Install is asynchronous (202) because downloading and verifying ~30 MB far
  exceeds a sensible browser request; progress is observed through the snapshot.
- Command endpoints reject any request body. They are commands, not resources,
  and accepting a body would invite a client to think it could pass a download
  URL or a checksum.
- The response carries no executable path, environment, command line or process
  id, and the MediaMTX Control API is not proxied - the browser never reaches it.
- The backend stays healthy while MediaMTX is missing or failed; `/api/health`
  is unchanged and the platform API keeps working.

### Files changed
- `apps/server/internal/runtime/mediamtx/` - `config.go`, `apiclient.go`,
  `state.go`, `process.go`, `process_unix.go`, `process_windows.go`,
  `supervisor.go`, and four test files including the fake-process harness
- `apps/server/internal/httpapi/runtime.go`, `runtime_test.go`, `router.go`,
  `decode.go`
- `apps/server/cmd/server/main.go`
- `scripts/verify-mediamtx-runtime.mjs`

### Technical decisions

1. **The configuration was validated against the real binary before the code was
   written.** The v1.19.3 archive was downloaded, its bundled `mediamtx.yml`
   inspected for the exact key names, and a candidate configuration started to
   confirm it loads. Guessing would have produced a config MediaMTX refuses,
   discovered only at smoke-test time. This is also how `moq` was confirmed to
   exist in v1.19.3 and `/v3/paths/list` response shape was captured.

2. **Readiness uses the global-configuration endpoint.** There is no
   `/v3/info` in v1.19.3. The alternative - treating a spawned process as ready -
   is exactly the failure this must avoid.

3. **The fake MediaMTX is the test binary re-executed.** `TestMain` checks an
   environment variable and, when set, serves a fake Control API instead of
   running tests. That avoids platform-specific shell scripts entirely. The
   variable travels through an unexported `Options.extraEnv` that production
   never populates, so it cannot weaken the real environment isolation.

4. **A generation counter rather than a mutex held across I/O.** Start, stop and
   restart all touch the same state while a child process is spawning, and
   holding the lock across process creation would serialise the whole
   supervisor. The counter lets a superseded launch detect that it lost.

5. **`applyResolutionLocked` was extracted after the smoke test caught a real
   bug.** The first version cleared the installing state before re-resolving,
   leaving a window where a snapshot said "stopped" with no installed version -
   which the script caught immediately. Resolution now happens before the state
   transition, and the two are applied under one lock.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Backend formatting | `gofmt -l .` | Passed - no files need formatting |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - 6 packages |
| Backend build | `go build ./...` | Passed - 0 errors |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - unchanged by this commit |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps against the real binary |

Tests added: 12 configuration-generation tests (determinism, a golden file,
loopback binding, every unused service disabled, only the configured path, the
publisher source, no takeover, no recording, no destination or credential,
atomic write, no leftover temporary file), 16 Control API tests using real
v1.19.3 response bodies (readiness, unknown fields tolerated, a non-MediaMTX
payload rejected, malformed JSON, non-success status, timeout, oversized
response, waiting, receiving, disappeared path, other paths ignored, missing
required fields rejected), 18 supervisor tests (missing binary does not stop the
backend, start reaches ready through the API, readiness is not assumed from
process creation, concurrent starts refused, explicit stop, stop suppresses
restart, restart, unexpected exit detected, automatic restart counted, restart
budget bounded, backoff capped, ingest waiting and receiving, port in use
without touching the other listener, shutdown reaps the child, output drained,
ring bounded) and 20 runtime HTTP tests.

The smoke script verified, against the real downloaded binary: a clean data
directory reports `missing`; the platform API still works while MediaMTX is
missing; a request body is rejected; managed installation with official checksum
verification succeeds; the installed version is v1.19.3 from the managed source;
readiness is reached; ingest reports `waiting` with no invented track data; the
payload leaks no filesystem path; explicit stop works and is not undone;
a manual start works; after a backend restart the managed binary is reused
without a second download, autostart brings it up, and the restart counter is
back to zero - which is what demonstrates runtime state lives only in memory.

No manual testing was performed.

### Known limitations

- **`waiting -> receiving -> waiting` was NOT verified end-to-end with a real
  RTMP publisher.** Those transitions are covered thoroughly against a fake
  v1.19.3 Control API and against captured real response bodies, and the
  real-binary smoke test runs through readiness and `waiting` only. Writing a
  correct test-only RTMP publisher means a full handshake, AMF0 command chain
  and a valid H.264 sequence header before MediaMTX marks a path ready; that is
  substantial protocol work, and FFmpeg is out of scope for this stage. The task
  explicitly permits this fallback provided it is stated honestly, which it is
  here and in the final report. Real publisher detection is therefore
  **unverified end-to-end**.
- Windows termination is forced, not graceful. Documented in code and README.
- The diagnostic ring holds 100 lines and is not exposed through the API; the
  Logs page remains a later stage.
- Readiness cannot confirm the running process's version through the API,
  because v1.19.3 exposes none. The binary's `--version` is checked instead.
- The port pre-check has a small race: another process could take the port
  between the check and MediaMTX binding it. MediaMTX then fails to bind and the
  readiness timeout records it, so the outcome is correct if less specific.

### Next step
Replace the placeholder OBS panel with real runtime data, add the installation
flow and runtime controls, and turn the Streams placeholder into a local ingest
status page.

---

## 2026-08-03 23:40 — feat(web): show MediaMTX and OBS ingest state

### Status
Completed

### Scope
Replace the placeholder OBS panel with real runtime data, add the installation
flow and runtime controls, and turn the Streams placeholder into a local ingest
status page.

### Changes

**Data layer**
- `src/api/runtime-schemas.ts` holds the Zod contract; `runtime.ts` is the
  transport. The payload is versioned and a mismatched version is rejected with
  a parse error rather than rendered half-understood.
- Process and ingest states are strict enums, because each one drives the
  interface and an unknown value is a contract violation. Descriptive fields
  (source type, tracks) stay tolerant, since MediaMTX may rename them.
- `useRuntimeQuery` polls with an interval chosen from the current state: 1s
  while ready, starting or stopping, 2s while installing, 10s when nothing can
  change without a user action. Background refetching is off, so a hidden tab
  stops polling. Four command mutations invalidate the snapshot on settle -
  including on failure, because a rejected command means the assumed state was
  wrong.
- The browser never contacts the MediaMTX Control API; only the curated backend
  endpoints exist.

**Presentation rules**
- `src/models/runtime-presentation.ts` maps state onto labels, tone and control
  availability as pure exhaustive functions, so a new state cannot be forgotten
  and the rules are testable without rendering.
- `live` tone is reserved for "actually receiving a stream". A merely running
  MediaMTX is not a live transmission, and configured destinations are never
  shown as live.
- The system summary never reports the system operational while MediaMTX is
  missing or failed.

**Sidebar**
- The placeholder panel is gone. It now shows the service state, the ingest
  state, the last error, compact controls, and the real Server and Stream key
  with copy buttons.
- Copy feedback is announced through a live region, and the value stays
  selectable text so it remains usable when clipboard access is denied.
- The stream key is labelled explicitly as a local route name that is not a
  secret and not a destination platform key.

**Streams page**
- No longer a placeholder. Shows supported and installed version, binary source,
  uptime, restart count, autostart and auto-restart flags, ingest state, path,
  source type, connection time, tracks, the last error, the full control set and
  the OBS settings with all three copyable values.
- Outgoing platform branches remain explicitly marked as a later stage.

**Installation flow**
- An application-styled dialog states the exact version, that it comes from the
  official GitHub release, that the checksum is verified, that MediaMTX is
  third-party MIT software with its licence installed alongside it, and the
  approximate download size. Nothing downloads without that confirmation, and
  `window.confirm` is not used.
- A second click while the request is in flight is ignored.

**Removed placeholders**
- `DEMO_OBS_CONNECTION` and the hard-coded RTMP address in `app-info.ts` are
  gone, along with the now-dead `navigation:obs.*` keys in both languages. The
  address is configurable on the backend, so duplicating it in the frontend
  would let the two drift.
- No bitrate, resolution or frame rate is rendered anywhere: the runtime API
  reports none, and inventing them would make the panel untrustworthy.

**Localization**
- New `runtime` namespace in English and Polish, taking the count to eight.
  Polish plural categories are complete for the track counter.

### Files changed
- Added: `src/api/runtime-schemas.ts`, `runtime.ts`, `src/hooks/use-runtime.ts`,
  `src/models/runtime-presentation.ts`, `src/components/runtime/` (four files),
  `src/pages/StreamsPage.tsx`, `src/i18n/resources/{en,pl}/runtime.json`, and
  four test files
- Modified: `SidebarFooter`, `SystemStatusPill`, `App.tsx`, `PlannedPages.tsx`,
  `data/app-info.ts`, `data/demo-system.ts`, `i18n/config.ts`, `resources.ts`,
  and the `navigation` namespace in both languages

### Technical decisions

1. **The runtime payload is versioned and the version is checked.** Runtime
   state drives destructive-looking controls; rendering a payload this build
   does not understand could show a Start button for a state that does not
   exist. A hard parse error is safer than a half-rendered panel.

2. **State enums are strict, descriptive fields are tolerant.** Every process
   and ingest state has behaviour attached, so an unknown one must fail. A new
   `sourceType` string has no behaviour attached and must not break anything.

3. **Control availability comes from one exhaustive function.** Scattering
   `state === 'ready'` checks across components would eventually enable a
   control the backend rejects. `controlsFor` is a total function over the state
   union, and a test asserts start and stop are never both enabled.

4. **Polling adapts to state rather than running at a fixed rate.** One second
   is right while a publisher may appear, and wasteful against a stopped
   service that cannot change without a user action.

5. **The install dialog closes as soon as the backend accepts the job.** The
   download takes minutes; holding a modal open would block the interface for
   the whole time, and the runtime panels already show `installing`.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 8 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test` | Passed - 16 files, 235 tests |
| Frontend build | `npm run build` | Passed |

102 frontend tests were added: runtime snapshot parsing for every MediaMTX and
ingest state, a missing installed version, a receiving snapshot with tracks,
unknown future fields tolerated, unknown states and malformed payloads rejected,
and a missing `lastError` rejected; state-to-label and state-to-tone mapping;
control availability for every state including the invariant that start and stop
are never both enabled; system-status aggregation across every state, including
that a missing or failed component is never reported as operational and that
"receiving" requires an actual stream; polling-interval selection; runtime cache
invalidation touching only the runtime key; and runtime error localization
including the English fallback for an unmapped code.

No manual testing was performed.

### Known limitations
- There are no component-rendering tests; behaviour is covered through the
  extracted pure modules, consistent with the approach taken in earlier stages.
- The bundle grew to roughly 520 KB (155 KB gzipped) and Vite warns about chunk
  size. Still not worth code-splitting for a locally loaded application.
- The diagnostic ring the backend keeps is not surfaced; the Logs page remains a
  later stage.
- The Streams page renders timestamps as the backend's raw RFC 3339 strings
  rather than formatting them per locale.

### Next step
Document the MediaMTX runtime integration: the pinned version, supported
platforms, installation and checksum verification, all new environment
variables, the loopback security model, the process lifecycle and restart
policy, the runtime endpoints, OBS settings, and troubleshooting.

---

## 2026-08-04 00:30 — docs: document MediaMTX runtime integration

### Status
Completed

### Scope
Document the MediaMTX runtime layer across `README.md`,
`docs/project-overview.md` and `config/README.md`. No code changed.

### Changes

**`README.md`**
- The project-state banner now says local ingest works and outgoing streaming
  does not, rather than the previous "nothing transmits".
- New **Local ingest with MediaMTX** section: the pinned v1.19.3 and why it is
  pinned, the supported OS/architecture matrix and what happens outside it, the
  three-step resolution order and why `PATH` is not searched, the installation
  flow with all nine verification steps, the managed installation layout, how to
  remove only the managed MediaMTX without touching the database, what the
  generated configuration enables and disables, the loopback security model, the
  process lifecycle, and the restart policy with its exact bounds.
- The Windows shutdown difference is stated in a callout rather than buried: it
  is forced, not graceful, and the reason is given.
- New **Connecting OBS** section with the Server and Stream Key table, and a
  prominent callout that the local stream key is a route name and not a secret,
  distinct from a destination platform key.
- Six new environment variables documented, including that a malformed boolean
  is a startup error.
- The API table gained the five runtime endpoints, an example snapshot, the
  no-request-body rule, and a note that runtime state is in-memory only.
- The demo table was rewritten: the Streams page and the OBS panel are no longer
  placeholders, and per-platform live status is recorded as removed. A "what is
  real" list now leads with receiving a stream from OBS.
- Twelve MediaMTX and OBS troubleshooting entries, including port conflicts with
  commands to find the holder on each platform, and an explicit note that
  Streaming Tree never terminates another process to free a port.
- The integration-check section covers both scripts and states that neither
  touches the real database or managed installation.

**`docs/project-overview.md`**
- Section 7.4 changed from "planned" to implemented, covering the managed
  dependency model, the process supervisor and the security boundary.
- The runtime-state part of section 8.1 was rewritten: runtime state now exists,
  lives only in memory, and is listed field by field - alongside what is
  deliberately not tracked, and why.
- New section 8.2 on OBS ingest detection, including why the interface says
  "OBS or another RTMP publisher" and why MediaMTX command hooks are not used.
- The architecture diagram marks what is implemented, shows the Control API on
  loopback and notes that the browser never reaches it.
- Stage 4 marked complete, with the honest note that the
  `waiting -> receiving -> waiting` transition was not verified end to end with
  a real publisher, pointing at the entry that explains it.
- The stream-key section records that the local ingest path is a route
  identifier and is never labelled as a secret.

**`config/README.md`**
- Records that no MediaMTX sample or template lives there, that the real
  configuration is generated into the runtime directory on every start, why it
  is generated rather than templated, and that binaries are never committed.

**`THIRD_PARTY_NOTICES.md`** was added in the first commit of this stage and
needed no change.

### Files changed
- `README.md`
- `docs/project-overview.md`
- `docs/progress.md`
- `config/README.md`

### Technical decisions

1. **The commit split follows the suggested four-way boundary**: managed
   dependency, supervision and runtime API, frontend, documentation.
   Documentation comes last so it describes what was built rather than what was
   intended.

2. **The Windows shutdown limitation is a callout, not a footnote.** An operator
   deciding whether to trust the application with a live stream should not have
   to discover that in source comments.

3. **"The local stream key is not a secret" is its own callout.** The word
   "stream key" means something very specific and very sensitive to a streamer.
   Reusing it for a local route name without saying so plainly would be a
   genuine security-communication failure.

### Automated validation

Re-run after the documentation change to confirm no regression:

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 8 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test` | Passed - 16 files, 235 tests |
| Frontend build | `npm run build` | Passed |
| Backend formatting | `gofmt -l .` | Passed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - 6 packages |
| Backend build | `go build ./...` | Passed |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

No manual testing was performed. No OBS was connected by hand.

### Known limitations
- Nothing checks automatically that the documentation matches the code; a
  renamed environment variable or endpoint must be corrected by hand.
- The documented default database and runtime paths were observed on Windows
  only; the macOS and Linux paths follow documented `os.UserConfigDir()`
  behaviour but were not observed on those systems.
- The English documentation has not been reviewed by a second reader.
- All product limitations from the three previous entries still stand, including
  that the `waiting -> receiving -> waiting` transition is unverified end to end
  with a real RTMP publisher.

### Next step

Stage 5: FFmpeg destination branches.

1. Resolve or manage an FFmpeg binary the same way MediaMTX is handled now:
   pinned version, checksum-verified installation, explicit path override.
2. Start one FFmpeg process per enabled destination, pulling from the local
   MediaMTX path and pushing to the platform's RTMP endpoint, defaulting to
   stream copy so OBS still encodes once.
3. Extend the in-memory runtime model with per-branch state, keeping it out of
   SQLite exactly as the MediaMTX state is today.
4. Give each branch independent start/stop and its own restart policy, so one
   failing destination cannot disturb the others.
5. Read destination stream keys from the operating system credential store,
   which stage 5 or 6 must introduce - they are still not stored anywhere today.

The constraint to carry forward: per-branch runtime state must stay in memory,
and a destination must not be shown as live until an FFmpeg process is genuinely
connected and sending.

## 2026-08-04 08:07 — docs: expand roadmap for engagement and overlays

### Status
Completed

### Scope
Document a product-scope expansion decided before this stage's implementation
work: Streaming Tree's long-term plan now includes a local streaming
engagement and overlay platform (unified chat, OBS overlays, alerts, bot
automation, visual designers, TTS, goal widgets) layered on top of the
streaming router. No code changed and nothing described here is implemented.
This entry exists so the roadmap and architecture documents are in place
before the credential-store foundation (the next entry) is built against them.

### Changes

**`docs/engagement-architecture.md`** (new) - the full architecture for the
planned engagement platform: terminology, the normalized event model and its
core fields, the connector interface and inbound/outbound capability model,
deduplication and ordering rules, deletion/moderation events, the in-memory
buffer vs. optional persisted history, the operator-chat vs. OBS-overlay
distinction with their separate settings, scheduled bot messages and the
placeholder system for message text (explicitly no arbitrary code execution),
chat commands, automation rules, the alert engine and alert queue, visual
overlay and chat-overlay designers, the template security model (no
JavaScript, no executables, no unrestricted HTML/filesystem/network access,
reusing the archive-safety approach already used for the MediaMTX managed
installation), template packaging and import/export, preview/test events,
text-to-speech and the `TTSProvider` abstraction (no engine bundled), goals
and counters, external donation connectors, privacy and security notes, the
OAuth/credential-store dependency, and a staged implementation order. The
document opens with an explicit "this is planning, not implementation"
notice and repeats it at points where a reader might otherwise assume
something works.

**`docs/project-overview.md`**
- Section 6 (out of scope for v1.0) now notes that several excluded items -
  aggregated chat, alerts, bot messages - are part of the long-term vision
  and points at the new section 16.
- Section 7 (architecture) notes the diagram covers the streaming path only;
  the engagement platform is a separate, additive set of connectors.
- Section 10 (stream key security) notes the `SecretStore` abstraction is
  designed to be reused for OAuth tokens once connected accounts exist.
- Section 13 (roadmap) replaces the previous 10-stage table with the full
  20-stage table: stages 1-4 completed, stage 5 (credential-store
  foundation, this task's implementation target) through stage 20 (logs,
  diagnostics, packaging, remote-server hardening) planned, with a "key
  dependencies" list stating explicitly that stage 6 (FFmpeg) and stage 7
  (OAuth) both need stage 5's credential store, that stage 8 (Event Bus) is
  a prerequisite for every later engagement stage, and that stage 13
  (designers) needs stages 9/10/12 to establish what an overlay renders
  before stage 14 (templates) can define an import/export format for it.
- New section 16, "Engagement and overlay platform (planned)", states plainly
  that the credential-store foundation is a hard prerequisite for the entire
  engagement era, that a connected account (for reading chat) is a different
  concept from a configured destination (for outgoing streaming) even for the
  same provider, and that provider support is planned honestly - Twitch
  first, YouTube and Kick as separate adapters, TikTok only if an official
  stable integration exists, never scraping as a core feature.

**`README.md`**
- New "Long-term vision" paragraph in the introduction, stating plainly that
  none of the engagement platform exists yet and pointing at
  `docs/engagement-architecture.md`.
- New "Roadmap" section (linked from the table of contents) with a condensed
  stage table and links to the full table in `project-overview.md` and the
  detailed architecture document.

### Files changed
- `docs/engagement-architecture.md` (new)
- `docs/project-overview.md`
- `README.md`
- `docs/progress.md`

### Technical decisions

1. **Existing section numbers in `project-overview.md` were preserved.** The
   new engagement content was appended as section 16 and as short
   forward-references inserted into sections 6, 7, 10 and 13, rather than
   renumbering the document. `engagement-architecture.md` and other
   documents link to specific section numbers (for example
   `project-overview.md#13-roadmap`); renumbering would have silently broken
   those anchors.
2. **A dedicated architecture document, not an expanded section 16.** The
   engagement platform touches roughly a dozen concerns (events, chat,
   overlays, alerts, bot automation, templates, TTS, widgets). Folding all of
   it into `project-overview.md` would have doubled that document's length
   with planning detail that has no bearing on what is running today. Section
   16 stays short and points at the dedicated document.
3. **The 20-stage roadmap replaces, rather than merges with, the previous
   10-stage one.** The previous table's "live status over SSE or WebSocket"
   stage does not appear in the new table as its own stage; the underlying
   intent ("later stages push live state over SSE or WebSocket") remains
   documented in section 7 and section 12 without being tied to a specific
   stage number, since the roadmap this task was given does not treat it as
   a separate stage.

### Automated validation

Re-run to confirm this documentation-only change caused no regression:

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 8 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test -- --run` | Passed - 16 files, 235 tests |
| Frontend build | `npm run build` | Passed |
| Backend formatting | `gofmt -l .` | Passed - no files listed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend tests | `go test ./...` | Passed - 6 packages |
| Backend build | `go build ./...` | Passed |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

No manual testing was performed. No OBS was connected by hand.

### Known limitations
- `docs/engagement-architecture.md` is planning, not a specification frozen
  against implementation reality; details will shift once stage 8 (Event Bus)
  is actually built, the same way earlier planning documents were refined
  during implementation.
- The 99designs/keyring library selected for the credential-store foundation
  (next entry) is not yet pinned in `go.mod` as of this entry.
- All product limitations from previous entries still stand.

### Next step

Stage 5: the secure credential-store foundation itself - a `SecretStore`
interface backed by the operating system credential store (Windows Credential
Manager, macOS Keychain, Linux Secret Service via `github.com/99designs/keyring`),
an in-memory fake for tests, a centralized credential key namespace, stream-key
validation, HTTP endpoints to set/check/delete a destination stream key without
ever echoing it back, and platform-settings UI to manage it. FFmpeg itself is
still not implemented; this stage only makes it possible to store the secret
FFmpeg will eventually need.

## 2026-08-04 08:30 — feat(server): add system credential store

### Status
Completed (backend only - the frontend controls to manage a stream key are
the next entry in this stage)

### Scope
The secure credential-store foundation: a `SecretStore` port over the
operating system's own credential store, a credential domain service built on
it, and the HTTP API a future frontend uses to set, check and delete a
destination platform's stream key. No stream key has ever been stored in
SQLite, and nothing here starts FFmpeg or any outgoing stream - this stage
only makes it possible to hold the secret a later stage will need.

### Changes

**`apps/server/internal/secrets`** (new package) - the storage-agnostic
foundation:
- `SecretStore` interface (`Set`/`Get`/`Delete`/`Exists`, all `context`-based)
  and three sentinel errors: `ErrUnavailable`, `ErrNotFound`, `ErrFailure`. No
  driver-specific error, and no secret value, is allowed to cross this
  interface.
- `KeyringStore`, the production implementation, wraps
  `github.com/99designs/keyring` and restricts it to exactly three backends -
  `WinCredBackend`, `KeychainBackend`, `SecretServiceBackend` - excluding the
  library's `pass` backend (shells out to the external `pass` command) and
  `file` backend (a password-encrypted file on disk). Opening the OS backend
  is deferred to first use, so constructing the store at startup never
  prompts and never fails a healthy backend just because no credential store
  happens to be available yet.
- `BuildKey(secretType, subjectID)` centralizes the key-namespace format:
  `<secret-type>:<subject-id>`, for example `destination-stream-key:pf_abc123`.
  The subject ID is always the platform's generated ID, never its display
  name, so a rename can never orphan a secret, and two destinations for the
  same provider always resolve to independent keys because the provider ID is
  never part of the key at all.
- `secretstest.Store` (in `internal/secrets/secretstest`), a concurrency-safe
  in-memory fake with `Unavailable` and `FailNext` fields to simulate an
  unreachable store and an unexpected failure on demand. No production code
  imports it.

**`apps/server/internal/domain/credential`** (new package) - the credential
domain service:
- `ValidateStreamKey` trims incidental surrounding whitespace, rejects an
  empty value, rejects invalid UTF-8, rejects any control character
  (including every line-break form), and rejects a value over 4096 bytes - a
  conservative ceiling, not a claim about any real provider's format, since
  none has been verified. No rejected value is ever included in the returned
  error.
- `Service.Status` reports a stream key's configured state and whether the
  store could be reached to determine it. An unavailable store is not a Go
  error: it is a legitimate, stable status the API must always be able to
  report.
- `Service.SetStreamKey` / `DeleteStreamKey` validate, store and remove a
  key. `DeleteStreamKey` is explicitly idempotent: deleting an absent key
  succeeds.
- `Service.DeletePlatformCredentials` is the platform-deletion cleanup hook
  (see Technical decisions for its ordering and failure policy).
- `Service.RetrieveForProcessStart` is the one method that returns a secret
  value, reserved for the future FFmpeg stage. It is not part of
  `httpapi.CredentialService`, so the HTTP layer cannot reach it even though
  the concrete service has it.

**`apps/server/internal/httpapi`**:
- `credentials.go` (new): `GET /api/platforms/{id}/credentials` (returns
  `{streamKey:{configured}, store:{available}}`), `PUT
  /api/platforms/{id}/credentials/stream-key` (body `{streamKey: "..."}`,
  capped at 8 KiB versus the general 64 KiB limit, unknown fields and
  malformed JSON rejected, never echoes the value back), `DELETE
  .../stream-key` (idempotent, 204, no body). Platform existence is checked
  first on every credential endpoint, answering the stable `platform_not_found`
  code rather than the generic `not_found` platform CRUD uses. New stable
  error codes: `platform_not_found`, `credential_not_found`,
  `credential_store_unavailable` (503), `credential_store_failure` (500,
  cause logged server-side only). Stream-key validation failures reuse the
  existing `validation_failed` / `fields` / `details` envelope, so the
  frontend's existing field-error machinery covers `streamKey` with no new
  response shape.
- `DELETE /api/platforms/{id}` now runs through
  `handleDeletePlatformWithCredentials` when a `CredentialService` is wired:
  it deletes the platform's credential first and only deletes the platform
  row if that succeeds (see Technical decisions). Registered only when
  `Options.Credentials` is non-nil; otherwise the route falls back to the
  original `handleDeletePlatform`, so a router built without credentials
  (existing tests, a future headless mode) behaves exactly as before.
- `decode.go`: `decodeJSON` is now a thin wrapper over
  `decodeJSONWithLimit(w, r, target, limit)`, so the credential endpoints can
  use a smaller body-size ceiling without duplicating the strict-decode logic.

**`apps/server/cmd/server/main.go`**: constructs `secrets.NewKeyringStore()`
and `credential.NewService(...)`, wires it into `httpapi.Options.Credentials`.
Two comments that were about to become false are corrected: the package doc
no longer claims only a health endpoint is exposed and credential storage is
unstarted, and the database-ready log line no longer claims the application
stores no credentials anywhere - it now says where they actually live.

**`apps/server/go.mod`**: adds `github.com/99designs/keyring v1.2.2` as a
direct dependency, plus its transitive dependencies for the three backends
in use (`99designs/go-keychain`, `danieljoos/wincred`, `godbus/dbus`,
`gsterjov/go-libsecret`) and for its always-compiled file-backend code path
that this application never selects at runtime (`dvsekhvalnov/jose2go`,
`mtibben/percent`, `golang.org/x/term`).

**`THIRD_PARTY_NOTICES.md`**: records `99designs/keyring` and its
transitive dependencies, why it was chosen over `zalando/go-keyring` (which
shells out to `security` on macOS - confirmed by reading its source, not
just its documentation), why the `pass` and `file` backends are excluded,
and the macOS CGO/Xcode build requirement this choice accepts.

**`.gitignore`**: `secrets/` and `credentials/` were unanchored and silently
matched the new `apps/server/internal/secrets/` source package - the same
class of bug that previously hid `apps/web/src/data/` and
`apps/server/internal/runtime/mediamtx/` (see the two earlier comments in
this file). Anchored to `/secrets/` and `/credentials/`, with a comment
explaining why, matching the existing pattern for `/data/` and `/mediamtx`.
Caught before this commit by noticing `git status` did not list the new
package as untracked.

### Files changed
- `apps/server/internal/secrets/store.go` (new)
- `apps/server/internal/secrets/keyring_store.go` (new)
- `apps/server/internal/secrets/keyring_store_smoketest_test.go` (new)
- `apps/server/internal/secrets/store_test.go` (new)
- `apps/server/internal/secrets/secretstest/fake.go` (new)
- `apps/server/internal/secrets/secretstest/fake_test.go` (new)
- `apps/server/internal/domain/credential/model.go` (new)
- `apps/server/internal/domain/credential/errors.go` (new)
- `apps/server/internal/domain/credential/validation.go` (new)
- `apps/server/internal/domain/credential/validation_test.go` (new)
- `apps/server/internal/domain/credential/service.go` (new)
- `apps/server/internal/domain/credential/service_test.go` (new)
- `apps/server/internal/httpapi/credentials.go` (new)
- `apps/server/internal/httpapi/credentials_test.go` (new)
- `apps/server/internal/httpapi/decode.go`
- `apps/server/internal/httpapi/router.go`
- `apps/server/cmd/server/main.go`
- `apps/server/go.mod`
- `apps/server/go.sum`
- `THIRD_PARTY_NOTICES.md`
- `.gitignore`
- `docs/progress.md`

### Technical decisions

1. **Status is derived directly from the secret store; no new SQLite table.**
   The credential rules explicitly allow either a small non-secret metadata
   table or deriving status purely from the store. A table would need to stay
   in sync with a store that can change out from under it (a credential
   deleted by hand outside the app, a locked keychain), and the rules are
   explicit that SQLite may never be authoritative for whether a credential
   exists. Deriving status directly from `Exists` removes that synchronization
   problem entirely rather than managing it, at the cost of no "last updated"
   timestamp for now - an acceptable trade for a first stage with a single
   secret type and no UI yet built around that timestamp.

2. **Any error other than "not found" is treated as `ErrUnavailable`, not a
   separate `ErrFailure`, in the production store.** The rules group "no
   Secret Service session", "a locked keychain" and "a permission failure"
   together as one "unavailable" condition the backend must survive and
   report as a stable status - not as three different severities. Attempting
   to distinguish a merely-locked keychain from a genuinely broken one by
   parsing OS-specific denial strings would be fragile and unverifiable in
   this environment (this stage was implemented and tested on Windows only).
   `ErrFailure` stays in the shared vocabulary - the HTTP layer maps it to
   `credential_store_failure` - and is exercised through `secretstest.Store`,
   so that path is tested even though the real store cannot currently
   trigger it. Documented as a known limitation below.

3. **Platform deletion: delete the credential first, then the platform row -
   except when the store is merely unavailable.** SQLite and the OS
   credential store cannot share a transaction, so this is a strict two-step
   sequence rather than a claim of atomicity. If credential cleanup fails
   with an unexpected error (`ErrFailure`), platform deletion aborts entirely:
   the platform and its credential are left exactly as they were, safe to
   retry, rather than ever deleting a platform row while its credential's
   fate is unknown. If the store is simply unreachable (`ErrUnavailable`),
   platform deletion proceeds anyway: blocking all platform CRUD - a feature
   that predates credential storage and must keep working regardless of
   credential-store availability - on a transient outage would be a worse
   regression than the accepted, documented risk of leaving an inert entry
   behind under a platform ID this application will never generate or look
   up again.
   
4. **`ValidateStreamKey` returns `*platform.ValidationError`, reusing the
   platform domain's error type instead of inventing a parallel one.** With
   exactly one validated field, duplicating the whole
   violation/field/rule/params machinery (and the matching `writeValidationError`
   rendering it already has) for one more field was worse than the
   alternative: the credential package depending on platform's error
   vocabulary for this one type. The trade-off is a small, deliberate
   coupling between two domain packages that are otherwise independent; it
   buys byte-for-byte response consistency with every other validation error
   the API already returns, and the frontend's existing field/rule mapping
   needs no new machinery to support `streamKey`.

5. **The credential-cleanup cascade lives in `httpapi`, not in the platform
   or credential domain packages.** Making the platform domain aware of
   credentials, or the reverse, would couple two domains that have no other
   reason to know about each other. `handleDeletePlatformWithCredentials`
   orchestrates both services at the one layer that is already allowed to
   depend on both. `registerPlatformRoutes` registers it only when
   `Options.Credentials` is non-nil, so a router built without a credential
   service (every existing test, and any future headless mode) keeps the
   original single-service delete behavior unchanged.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Backend formatting | `gofmt -l .` | Passed - no files listed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend build | `go build ./...` | Passed |
| Backend tests | `go test ./...` | Passed - 8 packages, 262 tests |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

Frontend checks were not re-run for this entry: no frontend file changed.
They are re-run and recorded in the next entry, which adds the frontend
credential controls.

The optional real-OS-credential-store smoke test
(`TestKeyringStoreAgainstTheRealOSCredentialStore` in
`internal/secrets/keyring_store_smoketest_test.go`) was **not run**. It is
skipped by default and only runs when `STREAMING_TREE_CREDENTIAL_SMOKE_TEST=1`
is already set in the environment; that flag was not set, as instructed. All
other backend tests use `secretstest.Store`, never the real OS credential
store.

Verified on Windows only. The macOS Keychain backend (CGO, requires Xcode
command line tools) and the Linux Secret Service backend (D-Bus) were
reviewed at the source level (see `THIRD_PARTY_NOTICES.md`) but not
build-verified or run on those platforms.

### Known limitations

- No "last updated" timestamp is reported for a configured credential; see
  Technical decisions #1.
- The production `KeyringStore` cannot currently distinguish a genuinely
  broken credential store from one that is merely locked or lacking a
  session; both are reported as `credential_store_unavailable`. See
  Technical decisions #2.
- Not build-verified on macOS or Linux in this stage.
- The frontend has no controls to use this API yet - that is the next entry.
- `Service.RetrieveForProcessStart` has no caller yet; it exists for the
  future FFmpeg destination-branch stage and is exercised only by its own
  unit test today.
- All product limitations from previous entries still stand.

### Next step

Frontend controls for managing a destination stream key: a focused section in
(or reachable from) the platform settings dialog, showing only
configured/missing/not-available status, a password-style input to set a new
key, replace and delete actions with an application-styled delete
confirmation, and English/Polish translations - followed by the documentation
pass that closes out this stage.

## 2026-08-04 10:29 — feat(web): manage destination stream keys

### Status
Completed

### Scope
Frontend controls for the credential-store foundation added in the previous
entry: a stream-key section in the platform settings dialog, a non-sensitive
status indicator on each platform card, and the Zod schemas, API transport,
TanStack Query hooks and English/Polish translations behind them. No stream
key is ever cached, logged, or placed anywhere the backend rules forbid.

### Changes

**`apps/web/src/api/credential-schemas.ts`** (new) - `credentialStatusSchema`:
`{ streamKey: { configured }, store: { available } }`. There is no field for
a value anywhere in this contract.

**`apps/web/src/api/credentials.ts`** (new) - thin transport:
`fetchCredentialStatus`, `setStreamKey`, `deleteStreamKey`, matching the
`GET/PUT/DELETE /api/platforms/{id}/credentials...` routes. No function here
can return a stored key's value - the backend never sends one.

**`apps/web/src/hooks/use-credentials.ts`** (new) - `credentialKeys.status(id)`
(scoped per platform, mirroring `platformKeys`), `useCredentialStatusQuery`,
`useSetStreamKeyMutation`, `useDeleteStreamKeyMutation`. The set mutation is
configured with `gcTime: 0` (see Technical decisions) and its `onSuccess`
caches only the parsed API response, never `variables.streamKey`.
`apps/web/src/hooks/credential-cache.ts` (new) holds the one pure cache
transform (`markStreamKeyDeleted`), mirroring `platform-cache.ts`.

**`apps/web/src/components/platforms/credential-validation.ts`** (new) -
`validateStreamKeyDraft`: client-side mirror of the backend's trim/empty/
control-character/newline/max-length rules, for immediate feedback only; the
backend remains the authority and validates the same rules again. Mirrors
`add-platform-validation.ts`.

**`apps/web/src/models/credential-presentation.ts`** (new) -
`presentCredentialStatus` maps status onto a label key and tone: "Checking",
"Stored", "Missing", or a store-unavailable message - never "Valid",
"Connected" or "Authenticated", since a stored key is never checked against
the platform.

**`apps/web/src/components/platforms/StreamKeySection.tsx`** (new) - the UI:
current status badge, an explanation of where the key is stored and that it
is never shown again, an explicit note that a stored key is not verified,
an explicit note distinguishing this key from the local MediaMTX ingest path
("live") shown elsewhere in the same dialog, a password-style input to set a
new key, a delete action behind `ConfirmDialog` (never `window.confirm`), and
a store-unavailable banner that disables the input rather than pretending the
store is reachable. Embedded in `PlatformSettingsDialog.tsx` as `<StreamKeySection
key={platform.id} platform={platform} />` - keyed by platform id so its input,
status and delete-confirmation state can never leak from one platform to
another, and so it is cleared whenever the dialog closes (the whole subtree,
including this section, unmounts when the parent's `platform` prop becomes
`null` - see the comment at the call site for why that already holds
regardless of an unrelated pre-existing staleness question in the outer
dialog's own local state, which this entry does not touch).

**`apps/web/src/components/platforms/PlatformCard.tsx`**: a third status row,
"Stream key: Stored / Missing / ...", backed by its own
`useCredentialStatusQuery` call - non-sensitive by construction, since the
query result never contains anything but the two booleans.

**Translations** (`en`/`pl`, `platforms.json` and `errors.json`): a new
`credentials` section (status wording, explanation text, delete-dialog copy),
three new `validation.streamKey*` messages, a `card.streamKeyLabel` key, and
four new backend error codes (`platform_not_found`, `credential_not_found`,
`credential_store_unavailable`, `credential_store_failure`) mapped in
`api-error-message.ts` the same way the existing codes are. The Add Platform
dialog's `noCredentialsNote` was reworded: it used to promise credential
storage "in a later stage" - this entry is that stage, so it now points at
where the key is actually managed instead.

**`apps/web/src/lib/field-error-rules.ts`**: three new `streamKey:<rule>`
mappings, reusing the existing `field:rule → message key` pattern rather than
adding a parallel one - this is the frontend half of the backend's decision
(previous entry) to reuse `platform.ValidationError` for stream-key
validation failures.

**`apps/web/src/models/platform-constraints.ts`**: `STREAM_KEY_MAX_LENGTH =
4096`, matching `MaxStreamKeyBytes` in the backend (a character count here
versus a byte count there - see the comment on the constant).

### Files changed
- `apps/web/src/api/credential-schemas.ts` (new)
- `apps/web/src/api/credential-schemas.test.ts` (new)
- `apps/web/src/api/credentials.ts` (new)
- `apps/web/src/hooks/use-credentials.ts` (new)
- `apps/web/src/hooks/use-credentials.test.ts` (new)
- `apps/web/src/hooks/credential-cache.ts` (new)
- `apps/web/src/hooks/credential-cache.test.ts` (new)
- `apps/web/src/components/platforms/credential-validation.ts` (new)
- `apps/web/src/components/platforms/credential-validation.test.ts` (new)
- `apps/web/src/models/credential-presentation.ts` (new)
- `apps/web/src/models/credential-presentation.test.ts` (new)
- `apps/web/src/components/platforms/StreamKeySection.tsx` (new)
- `apps/web/src/components/platforms/PlatformSettingsDialog.tsx`
- `apps/web/src/components/platforms/PlatformCard.tsx`
- `apps/web/src/lib/field-error-rules.ts`
- `apps/web/src/lib/field-error-rules.test.ts`
- `apps/web/src/lib/api-error-message.ts`
- `apps/web/src/lib/api-error-message.test.ts`
- `apps/web/src/models/platform-constraints.ts`
- `apps/web/src/i18n/resources/en/platforms.json`
- `apps/web/src/i18n/resources/pl/platforms.json`
- `apps/web/src/i18n/resources/en/errors.json`
- `apps/web/src/i18n/resources/pl/errors.json`
- `docs/progress.md`

### Technical decisions

1. **The set-stream-key mutation uses `gcTime: 0` and resets itself in
   `onSettled`, regardless of success or failure.** TanStack Query keeps a
   mutation's `variables` - here, the stream key the operator just typed -
   in both the shared mutation cache (visible via React Query Devtools) and
   the hook's own returned state, for the default `gcTime` of five minutes,
   or until the observer resets or a new mutation runs. A secret must not
   linger in either place. `gcTime: 0` makes the underlying cache entry
   eligible for collection as soon as it is unobserved; calling `.reset()`
   in `onSettled` additionally clears the hook's own `variables`/`data`
   immediately, which `gcTime` alone does not guarantee while the component
   stays mounted and the mutation stays "observed". The error message shown
   to the operator is captured into a plain local string in `onError`
   *before* this reset runs, so resetting the mutation cannot make an error
   banner vanish out from under the user.
2. **`StreamKeySection` is a `key`-ed child, not inline state in
   `PlatformSettingsDialog`.** The parent dialog is rendered unconditionally
   by `DashboardPage` with `platform` toggling between an object and `null`;
   its own `useState` values do not reinitialize just because a *different*
   platform is later selected on the same mounted instance, since
   `useState`'s initializer only runs once. Rather than touch that
   pre-existing behavior (out of scope here, and not something this entry's
   automated checks cover), `StreamKeySection` is given `key={platform.id}`
   so React remounts it fresh - clearing its input, status query and
   delete-confirmation state - both when the dialog closes (the surrounding
   `Modal` unmounts entirely, since `PlatformSettingsDialog` returns `null`
   before rendering it whenever `platform` is `null`) and if a different
   platform were ever opened directly.
3. **Credential status is fetched per platform, including once per visible
   card**, rather than batched into the platform-list response. This adds
   one request per rendered card, judged acceptable for a local desktop
   panel with a handful of destinations; `staleTime: 30_000` on the query
   keeps the settings dialog and a card's own indicator from doubling the
   request when both are showing the same platform. No `refetchInterval` is
   set anywhere in this stage: a credential check must not repeatedly touch
   the OS credential store just because a card or dialog is left open.
4. **The Add Platform dialog's credential note was reworded, not removed.**
   It previously promised storage "in a later stage"; leaving that sentence
   unchanged after this entry would have made it quietly false. It now
   points at where the key is actually managed instead.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 8 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test -- --run` | Passed - 21 files, 279 tests |
| Frontend build | `npm run build` | Passed |
| Backend formatting | `gofmt -l .` | Passed - no files listed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend build | `go build ./...` | Passed |
| Backend tests | `go test ./...` | Passed - 8 packages |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

No manual testing was performed: this entry's UI was not opened in a browser,
per this stage's instruction to skip manual browser/OBS/platform testing.
Typecheck, lint and the test suite verify the code is correct and internally
consistent; they do not confirm the dialog renders or behaves as intended
when actually clicked through.

### Known limitations

- **This UI has not been exercised in a browser.** Automated checks cover
  the pure logic (schemas, validation, cache transforms, presentation
  mapping, error-code mapping) and typecheck/lint/build the component itself,
  but nothing here proves the section renders correctly, that focus and
  keyboard behavior work, or that the delete-confirmation flow feels right
  in practice.
- **No component-rendering test exists for `StreamKeySection` or any other
  dialog in this codebase** - this project's frontend tests are exclusively
  pure-logic (`vitest`, no `@testing-library/react`). Behaviors that are
  genuinely about component interaction - the input clearing after a
  successful save, the delete-confirmation dialog opening and closing, the
  mutation's `.reset()` actually firing - are exercised indirectly (the
  underlying pure functions and cache operations are tested directly) but
  not through a rendered component. Introducing a component-testing
  dependency was judged out of scope for this entry; it would be a
  project-wide testing-strategy decision, not a credential-store one.
- The platform-card credential indicator adds one request per card; see
  Technical decisions #3 for why this was accepted rather than batched.
- All product limitations from previous entries still stand, including that
  FFmpeg destination streaming is still not implemented - a stored key
  cannot yet be used to start an outgoing stream.

### Next step

Documentation: update `README.md`, `docs/project-overview.md`, `config/README.md`
and `THIRD_PARTY_NOTICES.md` (already touched in the previous entry, but not
yet updated to describe the frontend surface added here) to describe the
credential-store feature end to end, and mark stage 5 as completed in the
roadmap. That closes out this stage; stage 6 (FFmpeg destination branches) is
next.

## 2026-08-04 10:36 — docs: document secure credential handling

### Status
Completed

### Scope
Bring `README.md`, `docs/project-overview.md` and `config/README.md` up to
date with what the previous two entries actually built, and mark stage 5
completed in the roadmap. `THIRD_PARTY_NOTICES.md` was already updated in
the `feat(server): add system credential store` entry and needed no further
change - no new third-party dependency was added in the frontend entry. No
code changed.

### Changes

**`README.md`**
- "Stream key security" was rewritten from a forward-looking bullet list
  ("has not been started yet") into an accurate description of what is
  implemented: the OS credential store choice and why, no plaintext
  fallback, the key-namespace scoping rule, "never re-displayed, never
  verified" and the exact wording rule that enforces it, the browser-side
  guarantees including the mutation `gcTime: 0` decision, that the retrieval
  method for FFmpeg exists but is unreachable from the HTTP API and has no
  caller yet, and the explicit distinction from the local MediaMTX ingest
  path.
- The REST API table gained the three credential endpoints; the "no endpoint
  accepts or returns a credential" note was corrected to state precisely what
  is true now (no endpoint **returns** one; one endpoint accepts a new value
  to store) instead of a blanket claim that stopped being accurate the
  moment the PUT endpoint existed.
- "What is currently demo-only" moved credential storage from "what will be
  added later" to "what is real", with an explicit note that reading a
  stored key to start a stream is still not real, since FFmpeg is not
  implemented.
- Two new troubleshooting entries: a credential-store-unavailable status
  message, and what happens to a stream key if a platform is deleted while
  the store happens to be unreachable (see project-overview.md §10 point 9
  for the underlying policy).

**`docs/project-overview.md`**
- Section 10 (stream key security) rules 3 and 4 were rewritten from
  future tense to present tense, matching what stages 5's two commits
  actually built, and five new rules were added: never re-displayed or
  verified, the centralized key-namespace format, and the platform-deletion
  ordering and its one accepted exception. Cross-references point at the
  specific progress.md entries that explain the reasoning in full rather
  than repeating it here.
- Section 13 (roadmap): stage 5 marked **Completed**; the note after the
  table now references both commits that implement it and records the two
  honest limitations (Windows-only verification of the OS-backed store;
  no browser-rendered verification of the frontend controls, since this
  project's test suite has no component-rendering harness).
- Section 16: the credential-store-is-a-hard-prerequisite point was
  reworded from "this task implements" to "implemented in stage 5", and the
  closing sentence now states plainly that stage 5 is the only one of the
  twenty completed so far.

**`config/README.md`**: rule 1 (no secrets in this directory) rewritten from
a future promise into a statement of the actual mechanism now in place, with
a pointer at where the full model is documented. A new rule 4 states
explicitly that no future exported template package (overlay templates, bot
configurations, anything a user can share) may contain a credential - the
same principle rule 1 states for this directory, extended to the planned
template-export format before that format exists, so it is a constraint from
the start rather than a retrofit.

### Files changed
- `README.md`
- `docs/project-overview.md`
- `config/README.md`
- `docs/progress.md`

### Technical decisions

1. **`THIRD_PARTY_NOTICES.md` was left untouched in this entry.** It already
   records `99designs/keyring` and its transitive dependencies accurately,
   added in the `feat(server): add system credential store` entry; nothing
   in the frontend entry introduced a new third-party dependency. Editing it
   here with nothing to say would have been noise.
2. **Every documentation change in this entry cross-references the specific
   progress.md entry that explains a decision, rather than re-explaining
   the reasoning in each file.** The full reasoning (why this library, why
   this deletion ordering, why `gcTime: 0`) already lives in the two
   implementation entries; repeating it in README.md and project-overview.md
   would create three places that could drift out of sync with each other.

### Automated validation

Re-run to confirm this documentation-only change caused no regression:

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 8 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test -- --run` | Passed - 21 files, 279 tests |
| Frontend build | `npm run build` | Passed |
| Backend formatting | `gofmt -l .` | Passed - no files listed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend build | `go build ./...` | Passed |
| Backend tests | `go test ./...` | Passed - 8 packages |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

No manual testing was performed. The optional real-OS-credential-store smoke
test remains unrun in this entry too, for the same reason recorded in the
`feat(server): add system credential store` entry: it requires an explicit
opt-in environment variable that was not set.

### Known limitations
- All limitations recorded in the two implementation entries for this stage
  still stand: Windows-only verification of the OS-backed credential store,
  and no browser-rendered verification of the frontend controls.
- Nothing checks automatically that documentation matches code; a renamed
  endpoint or error code must be corrected by hand, same as every previous
  documentation entry in this journal.

### Next step

Stage 6: FFmpeg destination branches. The credential store built in this
stage exists specifically to be read here - `RetrieveForProcessStart` has no
caller yet. Per the stream-key security rules (§10 point 4), the value it
returns must not be logged, must not appear in a formatted error, and its
exposure risk if a safer transport than process-list-visible command-line
arguments is unavailable must be assessed as part of that stage, not assumed
away here.

## 2026-08-04 11:34 — fix(docs): correct stage 5 project status

### Status
Completed

### Scope
Stage 6 (FFmpeg destination branches) begins with a documentation audit: the
stage 5 entries corrected `README.md`'s and `project-overview.md`'s major
claims about the credential store, but a few statements were left stale
because they lived in sections that stage 5 did not otherwise touch. This
entry corrects them before any stage 6 code is written, so stage 6's own
documentation changes start from an accurate baseline rather than compounding
old drift. No code changed.

### Changes

**`README.md`**
- The project-state banner said credential storage was still planned; it now
  states plainly that a stream key can be stored securely today, and that
  nothing reads a stored key yet (that is this stage).
- The Connecting OBS callout said real platform stream keys are "not yet
  handled at all" and "will live in" the credential store; both are stage 5
  facts that were already false. It now says they are stored there today, and
  that nothing reads a stored key yet - accurate on both counts as of the
  start of this stage.
- The Requirements table said Go 1.22+, while `go.mod` has required 1.25
  since the SQLite persistence stage. Corrected. OBS and MediaMTX were marked
  "not yet" needed, contradicting the fact that local ingest has worked since
  stage 4; both are now marked accurately, with MediaMTX noted as installed
  and supervised automatically.
- The translation directory listing predated the `runtime` namespace added
  during the MediaMTX runtime stage. Added.

**`docs/project-overview.md`**
- §7.1 said OBS points at the local address "eventually" - it has pointed
  there since stage 4. Corrected to state plainly that this is implemented.
- §7.3 listed "it starts and supervises MediaMTX and the FFmpeg processes"
  and "it reads stream keys from the system credential store at branch start
  time" as present-tense facts about the backend. Only the MediaMTX half of
  the first claim, and none of the second, is true before this stage: FFmpeg
  supervision and reading a stored key both begin in stage 6, which had not
  started when this entry was written. Both bullets now distinguish what is
  implemented from what stage 6 will add.

Points 8, 9 and 10 of the requested audit (runtime-state wording, further
"eventually" wording, and roadmap/stage-number consistency) were checked and
found already accurate: the runtime-state section already distinguishes
MediaMTX runtime (exists) from per-destination runtime (does not), no other
stale "eventually" phrasing exists in `docs/project-overview.md`, and every
stage 5/6 reference across `README.md`, `docs/project-overview.md` and
`docs/engagement-architecture.md` already agrees with the 20-stage roadmap.
No further edit was needed for those three points.

### Files changed
- `README.md`
- `docs/project-overview.md`
- `docs/progress.md`

### Technical decisions

1. **Wording still says FFmpeg supervision and key retrieval are planned, not
   implemented.** This commit is a correction pass that runs before any stage
   6 code exists in the working tree. Describing FFmpeg support as
   implemented here - ahead of the commits that actually add it - would
   create the exact kind of drift this audit exists to remove. The
   corresponding present-tense update happens in the stage's closing
   documentation commit, once the real integration verification has passed.

### Automated validation

Re-run to confirm this documentation-only change caused no regression:

| Check | Command | Result |
| ----- | ------- | ------ |
| Translation consistency | `npm run i18n:check` | Passed - 2 languages, 8 namespaces |
| Frontend typecheck | `npm run typecheck` | Passed - 0 errors |
| Frontend lint | `npm run lint` | Passed - 0 errors, 0 warnings |
| Frontend tests | `npm run test -- --run` | Passed - 21 files, 279 tests |
| Frontend build | `npm run build` | Passed |
| Backend formatting | `gofmt -l .` | Passed - no files listed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend build | `go build ./...` | Passed |
| Backend tests | `go test ./...` | Passed - 8 packages |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

No manual testing was performed.

### Known limitations
- All limitations recorded in prior entries still stand.
- This entry does not touch `docs/engagement-architecture.md` or
  `config/README.md`: neither contained a stage 5/6 inaccuracy, per the
  cross-file consistency check above.

### Next step

FFmpeg research (executable resolution, compatibility probing, licensing),
then the output-configuration schema and API that stage 6's branch
supervision depends on.

## 2026-08-04 11:51 — feat(server): add FFmpeg output configuration

### Status
Completed

### Scope
The first half of stage 6: FFmpeg executable resolution and capability
probing, and the non-secret output-configuration (destination RTMP/RTMPS
server address, restart preference) each platform needs before a branch can
be started. No process is started yet - that is the next entry. No stream key
is read, stored differently, or exposed anywhere in this entry.

### FFmpeg research (before writing the resolver)

Checked directly rather than relied on memory, per this stage's instructions:

- **`https://ffmpeg.org/download.html`** (fetched during this entry): FFmpeg
  publishes **source releases only**; executable builds come from independent
  third-party distributors the project does not vouch for. Currently
  maintained release branches listed on that page: **9.0, 8.1, 8.0, 7.1, 6.1,
  5.1, 4.4**. This directly confirms the task's instruction not to implement
  a managed downloader the way MediaMTX has one: there is no single
  "official" binary to pin a checksum against.
- **`https://ffmpeg.org/legal.html`** (fetched during this entry): FFmpeg is
  LGPL 2.1-or-later **by default**, but becomes GPL the moment any
  GPL-covered optional component is compiled in - `--enable-gpl` is what
  turns that on, and `--enable-version3` upgrades either license to version 3.
  There is no license text FFmpeg is available under other than these; no
  commercial licensing exists.
- **`https://ffmpeg.org/ffmpeg.html`** (fetched during this entry): confirmed
  `-progress url` emits periodic `key=value` lines ending in
  `progress=continue` or `progress=end`; confirmed `-nostdin` disables
  interactive stdin handling for background use; found no RTMP-specific
  reconnect/timeout option in the documented CLI reference.
- **Locally detected FFmpeg** (probed directly, both manually and through the
  resolver written in this entry): `ffmpeg version 8.1-full_build-www.gyan.dev`,
  a Windows build from gyan.dev, built with `--enable-gpl --enable-version3`
  among many other flags - so this specific binary is **GPL v3-or-later**, not
  the LGPL default, and must not be described as such. `-protocols` lists
  `rtmp` under both Input and Output, and `rtmps` under Output; `-muxers`
  lists `flv` with muxing support; a real `-progress pipe:1` invocation (a
  0.1-second `anullsrc` encode) produced a genuine `progress=end` line. RTMP
  network-layer options were checked via `ffmpeg -h protocol=rtmp` and
  `-h protocol=tcp`: no RTMP-specific reconnect flag exists, but `tcp`
  (the transport RTMP runs over) accepts a generic `-timeout` (microseconds),
  and the generic AVIO `-rw_timeout` option applies regardless of protocol -
  this is the bounded-timeout mechanism the branch process will use once
  process launch is implemented (next entry), not an RTMP-specific one,
  because no RTMP-specific one exists in this FFmpeg's documented options.

### Compatibility policy

`internal/runtime/ffmpeg.MinimumVersion = "4.4"` - the oldest branch the
download page above still lists as maintained, checked at the time this was
written, not remembered. This is a floor, not the real gate: `Capabilities`
probing (RTMP input, RTMP output, RTMPS output, the FLV muxer, and a **real**
`-progress` invocation, not merely `-h` listing the flag) is authoritative.
A binary below the floor is rejected without probing further; a binary at or
above it is rejected only if a capability probe actually fails - a newer
release than anything named in this entry is accepted as long as it passes.
A version string with no numeric meaning (a git/dev build such as
`N-112233-gabcdef1234`) is not, by itself, a rejection reason either - it is
still shown to the user as the detected version, and compatibility is decided
by capability probing alone in that case.

### Executable resolution order

1. `STREAMING_TREE_FFMPEG_PATH` (absolute path required; a relative one is
   resolved against the working directory at startup).
2. A bundled location beside the running backend executable
   (`<dir of the backend binary>/ffmpeg[.exe]`) - a convention for a future
   packaged distribution. Nothing in this stage places a binary there, and
   nothing is committed to the repository; nothing chooses this path unless a
   regular file already exists there.
3. The system `PATH`.
4. Missing.

Unlike the MediaMTX resolver, PATH **is** searched here - the task's explicit
instruction, because there is no approved managed source to prefer over it in
this stage. Every candidate from every step, wherever it came from, is still
probed for every required capability before it is ever trusted; none is
assumed compatible merely because it was found.

`Resolution.Path` is a Go-level field only, exactly like the MediaMTX
resolver's own `Resolution.Path` - it is never part of any JSON response.
Source is reported as one of `override` / `bundled` / `path` / `missing`.

### Why no managed downloader was implemented

The task forbids it, and the research above independently confirms why it
would be the wrong call for this stage specifically: FFmpeg has no single
official executable to pin a checksum against the way MediaMTX does (its
GitHub releases page publishes one asset per platform, officially, which is
what stage 4's installer already verifies). Any FFmpeg binary source is a
third party the project has not reviewed, so this stage resolves and probes
whatever the operator already has instead of choosing a distributor for them.

### Output-settings schema and migration

New table `platform_output_settings` (migration `0003_platform_output_settings.sql`):
`platform_id` (primary key, `REFERENCES platforms(id) ON DELETE CASCADE`),
`server_url` (`TEXT NOT NULL DEFAULT ''` - empty means "not configured", not
absent), `auto_restart` (`INTEGER NOT NULL DEFAULT 1`), `created_at`,
`updated_at`. No stream key, no full destination URL, no runtime state (PID,
restart count, live/error status) - all forbidden explicitly by this stage's
instructions, and runtime state stays in memory exactly as MediaMTX's does.

The migration backfills a default (empty `server_url`, `auto_restart = 1`)
row for every platform that already exists, so no seeded destination becomes
silently "ready to stream" as a side effect of running it. A newly created
platform gets its default row in the **same SQL transaction** as its
`platforms` and `platform_metadata` rows - `PlatformRepository.Create` (SQLite
layer) now also calls `insertDefaultOutputSettingsRow`, so the invariant
"every platform has output settings" holds without the `platform` domain
package needing to know the `output` domain package, or the concept of output
settings, exists at all. Deleting a platform cascades to its output-settings
row via the same foreign key that already cascades metadata and tags -
verified with a dedicated test - and does not touch the credential-cleanup
ordering from stage 5 in any way, since they are entirely separate concerns
(one SQL row, one OS credential store entry).

### Server-URL validation

`internal/domain/output.ValidateServerURL`: trims surrounding whitespace;
empty (after trim) is valid and means "not configured" - the field is
optional, so a destination can exist without one, and clearing it is a
legitimate action, not an error. A non-empty value must be `rtmp://` or
`rtmps://`, with a required host, a port that parses as 1-65535 when present,
no userinfo, no `#` fragment, and (an explicit, tested decision) **no `?`
query string** - no verified provider integration needs one for the base
server address, and rejecting it keeps the field unambiguous; if a real
provider integration is later found to need one, this is the one place that
changes. A path is allowed (`/app` or `/app/instance`), since providers
commonly use one. Control characters and embedded line breaks are rejected.
Maximum length 2048 bytes. Reuses `*platform.ValidationError` for the
response shape, the same trade-off made for the credential stream-key field
in stage 5.

### Output-settings API

- `GET /api/platforms/{id}/output` → `{serverUrl, autoRestart}` (plus
  `updatedAt`).
- `PUT /api/platforms/{id}/output` → full replacement, same body shape.

Both check platform existence first (reusing `requirePlatform` from the
credential handlers) and answer `platform_not_found` for a missing platform,
`validation_failed` with a `serverUrl` field for a bad address, and otherwise
the existing 400/404/405/413/415/422/500 machinery already used by the
platform and credential endpoints - no new decode or error-envelope code was
needed.

### Files changed
- `apps/server/internal/runtime/ffmpeg/errors.go` (new)
- `apps/server/internal/runtime/ffmpeg/version.go` (new)
- `apps/server/internal/runtime/ffmpeg/version_test.go` (new)
- `apps/server/internal/runtime/ffmpeg/capabilities.go` (new)
- `apps/server/internal/runtime/ffmpeg/capabilities_test.go` (new)
- `apps/server/internal/runtime/ffmpeg/resolver.go` (new)
- `apps/server/internal/runtime/ffmpeg/resolver_test.go` (new)
- `apps/server/internal/domain/output/model.go` (new)
- `apps/server/internal/domain/output/errors.go` (new)
- `apps/server/internal/domain/output/validation.go` (new)
- `apps/server/internal/domain/output/validation_test.go` (new)
- `apps/server/internal/domain/output/repository.go` (new)
- `apps/server/internal/domain/output/service.go` (new)
- `apps/server/internal/domain/output/service_test.go` (new)
- `apps/server/internal/storage/sqlite/migrations/0003_platform_output_settings.sql` (new)
- `apps/server/internal/storage/sqlite/output_repository.go` (new)
- `apps/server/internal/storage/sqlite/output_repository_test.go` (new)
- `apps/server/internal/storage/sqlite/platform_repository.go`
- `apps/server/internal/httpapi/output.go` (new)
- `apps/server/internal/httpapi/output_test.go` (new)
- `apps/server/internal/httpapi/router.go`
- `apps/server/internal/config/config.go`
- `apps/server/internal/config/config_test.go`
- `apps/server/cmd/server/main.go`

### Technical decisions

1. **`MinimumVersion` is a documented floor, capability probing is the real
   gate.** Recorded above with the exact source checked and when.
2. **The FFmpeg dependency gets its own package (`internal/runtime/ffmpeg`)
   rather than extending `internal/runtime/mediamtx`.** The two dependencies
   have almost nothing in common beyond both being external executables: one
   has a managed, checksummed install; the other is resolved wherever it
   already is and probed for capabilities. Sharing a package would have
   forced one of the two models to bend toward the other for no real benefit.
3. **Output settings are their own domain package, not fields added to
   `platform`.** Mirrors the reasoning already recorded for `credential` in
   stage 5: `platform` stays about configuration a user directly edits
   through the existing dialog; `output` is a distinct concept with its own
   validation and its own future consumer (the branch manager, next entry).
4. **A query string in the server URL is rejected, not merely "handled".**
   The task required an explicit, tested decision either way; this one was
   chosen because no verified provider needs one and it keeps the field's
   contents unambiguous - see the validation section above.

### Automated validation

| Check | Command | Result |
| ----- | ------- | ------ |
| Backend formatting | `gofmt -l .` | Passed - no files listed |
| Backend static analysis | `go vet ./...` | Passed - 0 findings |
| Backend build | `go build ./...` | Passed |
| Backend tests | `go test ./...` | Passed - 10 packages |
| SQLite persistence | `node scripts/verify-persistence.mjs` | Passed - 14 steps |
| MediaMTX runtime | `node scripts/verify-mediamtx-runtime.mjs` | Passed - 17 steps |

Frontend checks were not re-run: no frontend file changed in this entry. They
resume in the entry that adds the frontend for this stage.

The FFmpeg capability-probe tests run twice: once entirely against an
injected fake probe (the standard suite, no real FFmpeg required), and once
(`TestProbeExecutableAgainstARealFFmpegBinary`, skipped rather than failed
when none is found) against whatever real `ffmpeg` is on `PATH` in this
environment - which is the 8.1 build described above, and passed for real.

### Known limitations
- FFmpeg process launch, branch supervision, and reading a stored key at
  start time are not implemented yet - that is the next entry.
- The bundled-location convention (`<backend dir>/ffmpeg[.exe]`) has no
  packaging step that ever populates it in this stage; it is resolution-order
  plumbing for a later stage.
- `-rw_timeout` as the bounded-network-timeout mechanism is a research
  finding recorded here for the next entry to use; it is not wired into any
  process command yet, since no process is started in this entry.
- All limitations recorded in previous entries still stand.

### Next step

Branch process supervision: the FFmpeg command itself (stream copy, FLV over
RTMP/RTMPS, `-progress` parsing), the per-branch state machine, eligibility
and blockers, secret retrieval at process-start time only, restart policy and
ingest-loss handling, and the branch runtime HTTP API.
