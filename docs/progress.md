# Dziennik projektu — Streaming Tree for OBS

Plik jest trwałym dziennikiem prac nad projektem. Opisuje, co zostało wykonane,
dlaczego oraz jaki jest rzeczywisty stan każdej funkcji.

---

## Zasady prowadzenia dziennika

1. **Plik jest aktualizowany przy każdej logicznej zmianie.**
2. **Wpis powstaje przed utworzeniem odpowiadającego mu commita.**
3. **Każdy commit musi mieć odpowiadający mu wpis.**
4. **Historia wcześniejszych wpisów nie może być bez powodu przepisywana ani
   usuwana.** Korektę błędnego wpisu dopisujemy jako nowy wpis, nie przez
   nadpisanie starego.
5. **Nie zapisujemy w tym pliku kluczy streamu, tokenów ani innych sekretów.**
   Dotyczy to również przykładowych wartości i fragmentów logów.
6. **Nie oznaczamy funkcji jako ukończonej, jeżeli jest tylko atrapą
   interfejsu.** Atrapy trafiają do sekcji „Znane ograniczenia".
7. **Testy manualne pozostają etapem końcowym** — wykonywane dopiero po
   ukończeniu funkcjonalności aplikacji.
8. **Build, lint oraz typecheck mogą i powinny być uruchamiane podczas
   implementacji**, a ich wynik jest odnotowywany w sekcji „Walidacja
   automatyczna".

### Identyfikacja wpisów

Identyfikatorem wpisu jest **treść komunikatu commita**, a nie hash Git.
Hashy nie zapisujemy: modyfikacja tego pliku po utworzeniu commita zmieniałaby
sam hash, więc zapis byłby z definicji nieaktualny.

Obowiązuje konwencja [Conventional Commits](https://www.conventionalcommits.org/),
na przykład:

- `docs: add initial project documentation`
- `chore: bootstrap project structure`
- `feat(web): add dashboard shell`
- `feat(server): add health endpoint`
- `fix(web): correct platform status rendering`

### Format wpisu

```
## YYYY-MM-DD HH:MM — typ(scope): krótki opis commita

### Status
### Zakres
### Wprowadzone zmiany
### Zmienione pliki
### Decyzje techniczne
### Walidacja automatyczna
### Znane ograniczenia
### Następny krok
```

---

# Wpisy

## 2026-08-03 11:45 — chore: bootstrap streaming tree project

### Status
Ukończone

### Zakres
Etap fundamentów projektu. Utworzenie struktury repozytorium, dokumentacji,
panelu operatorskiego w React oraz minimalnego backendu w Go wraz z endpointem
statusowym, z którym łączy się frontend. Etap nie obejmuje realnego
streamowania.

### Wprowadzone zmiany

**Struktura repozytorium**
- Utworzono układ `apps/web`, `apps/server`, `docs`, `config`.
- Dodano `.gitignore` obejmujący zależności, artefakty budowania, pliki
  środowiskowe, katalogi danych oraz binaria MediaMTX i FFmpeg.
- Dodano `config/README.md` opisujący planowaną zawartość katalogu
  konfiguracyjnego (obecnie pusty, bez działającej konfiguracji).

**Dokumentacja**
- Utworzono `docs/project-overview.md` z pełnym opisem projektu: problem,
  założenia, grupa docelowa, zakres i wyłączenia wersji 1.0, architektura, role
  OBS / React / Go / MediaMTX / FFmpeg, model niezależnych gałęzi, model
  metadanych oparty na możliwościach platformy, bezpieczeństwo kluczy, wersja
  serwerowa, plan rozwoju, zasada testów manualnych.
- Utworzono ten dziennik wraz z zasadami prowadzenia.
- Utworzono `README.md` z instrukcją uruchomienia dla osoby technicznej i dla
  osoby wdrażającej się w projekt.

**Frontend (`apps/web`)**
- Skonfigurowano React 19 + TypeScript (tryb strict) + Vite 6 + Tailwind CSS 4 +
  React Router 7 + TanStack Query 5 + Zod 4 + Lucide React.
- Zbudowano system tokenów wizualnych (`src/index.css`): ciemne granatowe tło,
  jaśniejsze panele, fioletowy akcent, cztery semantyczne kolory statusów,
  delikatne obramowania i cienie, widoczne stany focus, obsługa
  `prefers-reduced-motion`.
- Zaimplementowano powłokę aplikacji: lewy panel nawigacyjny (logo tekstowe,
  sześć pozycji menu, status OBS, lokalny adres RTMP, numer wersji), górny pasek
  (tytuł, opis, `Add Platform`, `Global Settings`, zagregowany status systemu),
  treść główną oraz prawą kolumnę statusu.
- Zaimplementowano karty czterech platform (Twitch, YouTube, Kick, TikTok) ze
  statusem, tytułem transmisji, kategorią, liczbą widzów, jakością połączenia,
  przyciskiem Start/Stop oraz przyciskiem ustawień.
- Zaimplementowano panel statusu: liczniki gałęzi live / starting / offline /
  error, karta stanu backendu oraz karta zasobów systemowych (CPU, pamięć, dysk,
  sieć).
- Zaimplementowano model możliwości platform (`PlatformCapabilities`,
  `PlatformFieldLimits`, `PlatformFieldOptions`) oraz edytor metadanych
  z zakładkami, renderujący wyłącznie pola obsługiwane przez wybraną platformę.
- Zaimplementowano edytor tagów, w którym każdy tag jest osobnym, usuwalnym
  elementem interfejsu. Tagi są aktywne wyłącznie dla Twitcha.
- Walidację formularza metadanych oparto na schemacie Zod budowanym dynamicznie
  z tabeli możliwości platformy.
- Podłączono `GET /api/health` przez TanStack Query; wynik jest prezentowany
  w sekcji statusu systemu, a niedostępny backend daje czytelny stan
  „Backend unavailable" bez awarii interfejsu.
- Podstrony Platforms, Streams, Metadata, Settings i Logs to estetyczne widoki
  informujące o planowanym zakresie, bez pozorowanych widżetów.
- Zapewniono responsywność: prawa kolumna przechodzi pod treść główną poniżej
  `xl`, karty układają się w jedną kolumnę na wąskich ekranach, a nawigacja
  zmienia się w wysuwane menu poniżej `lg`.

**Backend (`apps/server`)**
- Utworzono moduł Go z podziałem na pakiety: `cmd/server`, `internal/config`,
  `internal/httpapi`, `internal/buildinfo`.
- Zaimplementowano `GET /api/health` zwracające `status`, `service`, `version`,
  `uptimeSeconds` oraz `time`.
- Dodano konfigurację przez zmienne środowiskowe (`STREAMING_TREE_HOST`,
  `STREAMING_TREE_PORT`, `STREAMING_TREE_ALLOWED_ORIGINS`) z walidacją wartości
  i czytelnym błędem przy starcie.
- Dodano middleware: obsługę paniki (odzyskiwanie z panic zamiast wyłączenia
  procesu), log dostępowy oraz CORS z jawną listą dozwolonych origin.
- Dodano kontrolowane zamykanie serwera na SIGINT/SIGTERM z limitem czasu na
  dokończenie żądań.
- Ujednolicono format błędów JSON (`error`, `message`) oraz zwracanie 405 wraz
  z nagłówkiem `Allow` dla nieprawidłowej metody.

### Zmienione pliki

Dokumentacja i konfiguracja repozytorium:
- `README.md`
- `.gitignore`
- `docs/project-overview.md`
- `docs/progress.md`
- `config/README.md`

Frontend — konfiguracja:
- `apps/web/package.json`, `apps/web/vite.config.ts`, `apps/web/eslint.config.js`
- `apps/web/tsconfig.json`, `tsconfig.app.json`, `tsconfig.node.json`
- `apps/web/index.html`, `apps/web/.env.example`

Frontend — kod:
- `apps/web/src/index.css` (tokeny wizualne)
- `apps/web/src/models/platform.ts`, `metadata-schema.ts`, `health.ts`
- `apps/web/src/data/demo-platforms.ts`, `demo-system.ts`, `app-info.ts`
- `apps/web/src/state/` (magazyn stanu demonstracyjnego)
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

### Decyzje techniczne

1. **Struktura `apps/` zgodna z propozycją.** Nie wprowadzono zmian w układzie
   katalogów poza dodaniem `config/README.md`, który opisuje przeznaczenie
   pustego katalogu, aby Git go zachował i aby jego rola była jednoznaczna.

2. **Vite 6 zamiast najnowszego Vite.** Środowisko ma Node 22.11, a bieżące
   wydania Vite (7/8) wymagają Node `^20.19 || >=22.12`. Przy Node 22.11 npm
   pomija opcjonalne zależności natywne z powodu niespełnionego pola `engines`,
   przez co Vite 8 nie startował („Cannot find native binding"). Wybrano
   najnowszą linię działającą w tym środowisku, aby build, lint i typecheck były
   faktycznie weryfikowalne. Po aktualizacji Node do 22.12+ można podnieść Vite
   bez zmian w kodzie aplikacji.

3. **Tailwind CSS 4 z konfiguracją w CSS.** Tokeny w dyrektywie `@theme`
   zamiast pliku `tailwind.config.js` — mniej plików konfiguracyjnych i jedno
   miejsce definiujące paletę.

4. **Model metadanych oparty na możliwościach platformy, nie na wspólnym
   formularzu.** Schemat Zod jest budowany funkcją z tabeli możliwości i limitów
   danej platformy. Dzięki temu reguła walidacji tagów nie istnieje dla
   platformy bez tagów, a dodanie platformy nie wymaga modyfikacji formularza.

5. **Pola nieobsługiwane nie są renderowane, a nie wyłączane.** Wyłączone pole
   sugerowałoby, że platforma zna dane pojęcie, ale chwilowo go nie udostępnia.

6. **Stan demonstracyjny odseparowany w `src/state/` i `src/data/`.** Kontekst
   Reacta z reducerem, wyraźnie opisany jako atrapa. Docelowo zostanie zastąpiony
   danymi z backendu bez zmian w komponentach prezentacyjnych.

7. **Walidacja odpowiedzi backendu po stronie frontendu.** `GET /api/health`
   jest parsowane schematem Zod. Niezgodność kształtu daje czytelny komunikat
   zamiast błędu w drzewie renderowania.

8. **Proxy `/api` w serwerze deweloperskim Vite.** Frontend wykonuje zapytania
   względne (same-origin), co upraszcza pracę lokalną. Backend mimo to ma własny
   middleware CORS, ponieważ w wersji serwerowej panel będzie serwowany z innego
   origin.

9. **`net/http` bez zewnętrznego routera.** ServeMux z Go 1.22 obsługuje wzorce
   z metodą (`GET /api/health`), co pokrywa obecne potrzeby. Brak zależności
   zewnętrznych upraszcza dystrybucję binarium.

10. **Middleware odzyskujący z panic.** Awaria pojedynczego handlera nie może
    zatrzymać procesu — ta sama zasada, która później zapewni niezależność
    gałęzi transmisji.

11. **CORS z jawną listą origin zamiast wildcardu.** Serwer będzie docelowo
    sterował realnymi transmisjami, więc dostęp z dowolnej strony otwartej
    w przeglądarce jest niedopuszczalny.

12. **TypeScript w trybie strict z dodatkowymi flagami** (`noUncheckedIndexedAccess`,
    `exactOptionalPropertyTypes`, `noUnusedLocals`, `noUnusedParameters`).
    W kodzie nie występuje typ `any`; reguła ESLint `@typescript-eslint/no-explicit-any`
    ustawiona na `error`.

### Walidacja automatyczna

| Kontrola | Polecenie | Wynik |
| -------- | --------- | ----- |
| Typecheck frontendu | `npm run typecheck` (`tsc -b`) | Przeszła — 0 błędów |
| Lint frontendu | `npm run lint` (`eslint .`) | Przeszła — 0 błędów, 0 ostrzeżeń |
| Build frontendu | `npm run build` | Przeszła — 1982 moduły, `dist/` wygenerowany |
| Kompilacja backendu | `go build ./...` | Przeszła — 0 błędów |
| Analiza statyczna backendu | `go vet ./...` | Przeszła — 0 uwag |
| Formatowanie backendu | `gofmt -l .` | Przeszła — brak plików do sformatowania |
| Kontrakt endpointu (skryptowa) | `GET /api/health` na uruchomionym binarium | 200, `application/json`, ładunek zgodny ze schematem Zod |
| Obsługa błędów (skryptowa) | `POST /api/health`, `GET /api/nieistniejacy` | 405 z nagłówkiem `Allow: GET`; 404 z ładunkiem JSON |

Uwaga: kontrole oznaczone jako „skryptowa" to automatyczne zapytania HTTP
wykonane skryptem powłoki w celu weryfikacji kontraktu API. Nie są to testy
manualne interfejsu — te pozostają etapem końcowym.

Uwaga środowiskowa: Go nie jest zainstalowane w systemie użytkownika. Kontrole
backendu wykonano przenośnym zestawem narzędzi Go 1.26.5 rozpakowanym do
katalogu tymczasowego, bez modyfikowania środowiska systemowego. Aby uruchamiać
backend samodzielnie, należy zainstalować Go — patrz `README.md`.

### Znane ograniczenia

**Elementy niezaimplementowane**
- Brak realnego streamowania: MediaMTX, FFmpeg i sterowanie procesami nie
  istnieją.
- Brak logowania OAuth i integracji z API Twitcha, YouTube, Kicka i TikToka.
- Brak bazy danych — konfiguracja i metadane nie są utrwalane; odświeżenie
  strony przywraca stan początkowy.
- Brak magazynu poświadczeń; obsługa kluczy transmisji nie została rozpoczęta.
- Brak SSE/WebSocket — stan backendu jest odpytywany co 15 sekund.
- Podstrony Platforms, Streams, Metadata, Settings i Logs nie mają
  implementacji.

**Atrapy (oznaczone w interfejsie znacznikiem „Demo")**
- Przyciski Start/Stop zmieniają wyłącznie lokalny stan w przeglądarce; po
  ok. 1,8 s status przechodzi ze `starting` na `live`. Żaden proces nie jest
  uruchamiany.
- Liczba widzów i jakość połączenia to wartości stałe.
- Metryki CPU, pamięci, dysku i sieci to stałe wartości demonstracyjne —
  backend nie zbiera metryk hosta.
- Status połączenia OBS jest stały („Waiting for OBS"); nic nie nasłuchuje na
  porcie RTMP.
- Lokalny adres RTMP w panelu bocznym to adres planowany, nie działający.
- Zapis metadanych trafia wyłącznie do pamięci przeglądarki.
- Tabele możliwości platform są przybliżone i poglądowe; wymagają weryfikacji
  przy wdrażaniu realnych integracji.

**Problemy środowiskowe**
- Node 22.11 nie spełnia wymagań `engines` najnowszych narzędzi frontendowych.
  Zalecana aktualizacja do Node 22.12+ lub 24 LTS. Obecny zestaw wersji działa
  poprawnie na Node 22.11.
- Repozytorium nie było zainicjalizowane jako repozytorium Git w chwili
  wykonywania tego etapu, więc commit nie został utworzony automatycznie.
  Polecenia do wykonania znajdują się w podsumowaniu etapu.

### Następny krok

Etap 2: trwałe przechowywanie konfiguracji.

1. Dodanie SQLite w backendzie wraz z migracjami schematu.
2. Model platform i metadanych po stronie backendu, jako źródło prawdy zamiast
   danych demonstracyjnych w przeglądarce.
3. REST API: `GET/POST/PUT/DELETE /api/platforms` oraz
   `GET/PUT /api/platforms/{id}/metadata`, z walidacją opartą na tej samej
   tabeli możliwości co frontend.
4. Zastąpienie magazynu demonstracyjnego w `src/state/` zapytaniami TanStack
   Query, z zachowaniem obecnych komponentów prezentacyjnych.
5. Utrzymanie stanu `Backend unavailable` jako w pełni obsłużonej ścieżki.

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
