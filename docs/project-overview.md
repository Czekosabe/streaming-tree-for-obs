# Streaming Tree for OBS — opis projektu

> Dokument opisuje założenia, architekturę i plan rozwoju projektu.
> Sekcje oznaczone jako **planowane** nie są jeszcze zaimplementowane.
> Aktualny stan prac znajduje się w [progress.md](progress.md).

---

## 1. Nazwa projektu

**Streaming Tree for OBS**

Nazwa opisuje model działania: pojedynczy strumień wychodzący z OBS jest
„pniem", a każda platforma docelowa stanowi niezależną „gałąź" transmisji.

---

## 2. Problem, który rozwiązujemy

Osoba prowadząca transmisję na żywo, która chce nadawać jednocześnie na kilka
platform, napotyka dziś następujące trudności:

1. **Ograniczenia sprzętowe.** OBS potrafi wysyłać wiele wyjść, ale każde z nich
   kosztuje osobne kodowanie lub osobne wysyłanie tego samego strumienia. Przy
   czterech platformach obciążenie procesora lub łącza rośnie kilkukrotnie.
2. **Uzależnienie od usług zewnętrznych.** Komercyjne serwisy do multistreamingu
   wymagają przesłania strumienia na cudzy serwer oraz powierzenia im kluczy
   transmisji, zwykle za abonament.
3. **Rozproszone metadane.** Tytuł, kategoria, tagi i ustawienia widoczności
   trzeba wprowadzać osobno w panelu każdej platformy, w różnych formatach i przy
   różnych limitach.
4. **Brak izolacji awarii.** Jeżeli jedna platforma odrzuci połączenie lub
   zerwie sesję, typowe konfiguracje potrafią zakłócić pozostałe wyjścia.

## 3. Główne założenie

OBS wysyła **jeden** lokalny strumień do aplikacji. Aplikacja odbiera go i
rozgałęzia na dowolną liczbę platform, przy czym:

- rozgałęzianie odbywa się **bez ponownego kodowania obrazu** tam, gdzie jest to
  możliwe (kopiowanie strumienia),
- każda gałąź jest **niezależnym procesem** — awaria jednej nie przerywa innych,
- **klucze transmisji nie opuszczają komputera użytkownika** i nie trafiają do
  repozytorium ani do przeglądarki,
- metadata każdej platformy są opisane **modelem możliwości (capabilities)**, a
  nie wspólnym, sztucznie ujednoliconym formularzem.

## 4. Grupa docelowa

- Twórcy transmisji na żywo nadający równolegle na kilka platform.
- Osoby techniczne, które wolą uruchomić narzędzie lokalnie niż powierzać klucze
  transmisji usłudze zewnętrznej.
- Małe zespoły produkcyjne i operatorzy transmisji wydarzeń, potrzebujący
  czytelnego panelu kontrolnego stanu wszystkich wyjść.
- W przyszłości: użytkownicy, którzy przeniosą router transmisji na własny
  serwer (VPS) i będą sterować nim z przeglądarki.

---

## 5. Zakres pierwszej wersji lokalnej

Wersja 1.0 (lokalna) ma obejmować:

- odbiór jednego strumienia RTMP z OBS na komputerze użytkownika,
- konfigurację listy platform docelowych,
- niezależne uruchamianie i zatrzymywanie każdej gałęzi transmisji,
- podgląd stanu każdej gałęzi (offline / starting / live / error),
- edycję metadanych transmisji zależną od możliwości platformy,
- bezpieczne przechowywanie kluczy transmisji w magazynie systemowym,
- podgląd logów i podstawowej diagnostyki,
- panel operatorski w przeglądarce, uruchamiany lokalnie.

## 6. Poza zakresem pierwszej wersji

Świadomie **nie** wchodzą w zakres wersji 1.0:

- nagrywanie i archiwizacja transmisji,
- transkodowanie do wielu rozdzielczości (ABR),
- czat zbiorczy z wielu platform w jednym oknie,
- statystyki historyczne i analityka widowni,
- system kont, ról i uprawnień,
- automatyczne klipy, powiadomienia, integracje z botami,
- aplikacja mobilna,
- wtyczka działająca wewnątrz OBS.

---

## 7. Architektura ogólna

```
                ┌───────────────────────────────────────────────┐
                │  Panel operatorski (React + TypeScript)        │
                │  przeglądarka, http://localhost:5173           │
                └───────────────────────┬───────────────────────┘
                                        │ REST  (+ SSE/WebSocket w przyszłości)
                                        ▼
                ┌───────────────────────────────────────────────┐
                │  Backend (Go)                                 │
                │  API, stan gałęzi, metadane, nadzór procesów  │
                └──────┬─────────────────────────┬──────────────┘
                       │ nadzór                  │ nadzór
                       ▼                         ▼
        ┌──────────────────────┐    ┌──────────────────────────────────┐
  OBS ─▶│  MediaMTX            │───▶│  FFmpeg (osobny proces / gałąź)  │
  RTMP  │  lokalny odbiór RTMP │    │  ffmpeg #1 ─▶ Twitch             │
        └──────────────────────┘    │  ffmpeg #2 ─▶ YouTube            │
                                    │  ffmpeg #3 ─▶ Kick               │
                                    │  ffmpeg #4 ─▶ TikTok             │
                                    └──────────────────────────────────┘
```

Warstwy są celowo rozdzielone: panel nie komunikuje się bezpośrednio z MediaMTX
ani z FFmpeg. Całość sterowania przechodzi przez backend Go, co pozwoli
w przyszłości przenieść backend na zdalny serwer bez zmian w panelu.

### 7.1 Rola OBS

OBS pozostaje narzędziem produkcyjnym: sceny, źródła, miksowanie dźwięku,
kodowanie obrazu. Konfiguruje się w nim **jedno** wyjście — Custom / RTMP
wskazujące na lokalny adres aplikacji (docelowo `rtmp://127.0.0.1:1935/live`).

OBS nie wie, na ile platform trafi strumień. Z jego perspektywy istnieje jeden
odbiorca.

### 7.2 Rola frontendu React

Frontend to **panel operatorski**, nie element toru transmisji. Odpowiada za:

- prezentację stanu wszystkich gałęzi transmisji,
- uruchamianie i zatrzymywanie gałęzi (przez API backendu),
- edycję metadanych zależną od możliwości platformy,
- prezentację diagnostyki i logów.

Frontend **nigdy** nie przechowuje kluczy transmisji ani tokenów — również w
`localStorage` czy `sessionStorage`.

### 7.3 Rola backendu Go

Backend jest jedynym miejscem, w którym podejmowane są decyzje:

- udostępnia REST API dla panelu,
- przechowuje konfigurację platform i metadane,
- uruchamia i nadzoruje MediaMTX oraz procesy FFmpeg,
- odczytuje klucze transmisji z magazynu systemowego w chwili uruchomienia gałęzi,
- pilnuje izolacji awarii i polityki ponownego uruchamiania,
- w kolejnych etapach przekazuje stan na żywo przez SSE lub WebSocket.

Wybór Go wynika z trzech przesłanek: dystrybucja jako pojedynczy plik binarny
bez środowiska uruchomieniowego, dobra obsługa nadzoru procesów potomnych oraz
prosty model współbieżności dla wielu niezależnych gałęzi.

### 7.4 Planowana rola MediaMTX

**Status: nie zaimplementowane.**

MediaMTX ma pełnić rolę lokalnego serwera odbierającego strumień z OBS. Zamiast
pisać własną implementację RTMP, aplikacja uruchomi MediaMTX jako proces
potomny z wygenerowaną konfiguracją i będzie z niego pobierać pojedynczy
strumień źródłowy dla wszystkich gałęzi.

Dzięki temu OBS koduje obraz **raz**, a gałęzie korzystają ze wspólnego źródła.

### 7.5 Planowana rola FFmpeg

**Status: nie zaimplementowane.**

Dla każdej aktywnej platformy backend uruchomi osobny proces FFmpeg, który
pobiera strumień z MediaMTX i wysyła go pod adres RTMP danej platformy.

Założenia:

- domyślnie kopiowanie strumienia (`-c copy`) — bez ponownego kodowania,
- ewentualne przekodowanie tylko wtedy, gdy platforma wymaga innych parametrów,
- osobny proces = osobny cykl życia, osobne logi, osobna polityka restartu.

---

## 8. Model niezależnych gałęzi transmisji

Każda platforma to niezależna gałąź o własnym cyklu życia:

```
offline ──▶ starting ──▶ live
   ▲            │           │
   │            ▼           ▼
   └────────── error ◀──────┘
```

Zasady:

1. **Izolacja procesów.** Jedna gałąź = jeden proces FFmpeg. Awaria procesu nie
   dotyka pozostałych.
2. **Izolacja błędów.** Odrzucenie klucza przez jedną platformę przenosi w stan
   `error` wyłącznie tę gałąź.
3. **Niezależne sterowanie.** Gałęzie można uruchamiać i zatrzymywać osobno, bez
   przerywania transmisji na pozostałych platformach.
4. **Niezależny restart.** Polityka ponownych prób jest ustawiana per gałąź.
5. **Wspólne źródło.** Wszystkie gałęzie czytają ten sam strumień z MediaMTX,
   więc dodanie platformy nie obciąża dodatkowo OBS.

## 9. Model metadanych zależny od możliwości platformy

Platformy nie oferują tych samych pól metadanych i nie stosują tych samych
ograniczeń. Zamiast wspólnego formularza, każda platforma deklaruje swoje
możliwości:

```ts
type PlatformCapabilities = {
  title: boolean;
  description: boolean;
  category: boolean;
  tags: boolean;
  language: boolean;
  visibility: boolean;
  matureContent: boolean;
  dvr: boolean;
  latencyMode: boolean;
};
```

Uzupełniają go **limity** (maksymalna długość tytułu, liczba tagów) oraz
**słownik opcji** (nazwa pola kategorii, dostępne poziomy widoczności i tryby
opóźnienia).

Konsekwencje przyjęte w kodzie:

- pole nieobsługiwane przez platformę **nie jest renderowane** — nie jest
  jedynie wyłączone,
- schemat walidacji Zod jest **budowany dynamicznie** z tabeli możliwości, więc
  reguły dotyczące tagów nie działają na platformie bez tagów,
- dodanie nowej platformy polega na dopisaniu jej opisu, a nie na przebudowie
  formularza.

W obecnej, demonstracyjnej konfiguracji obsługę tagów ma włączoną wyłącznie
Twitch. Konfiguracje te są **przybliżone i poglądowe** — zostaną zweryfikowane
przy okazji wdrażania realnych integracji z API.

---

## 10. Bezpieczeństwo kluczy transmisji

Klucz transmisji pozwala nadawać na cudzym kanale, dlatego traktujemy go jak
hasło.

Zasady obowiązujące w projekcie:

1. **Żadnych sekretów w repozytorium.** Ani kluczy, ani tokenów, ani plików
   `.env` z wartościami. `.gitignore` blokuje pliki środowiskowe i katalogi
   danych.
2. **Żadnych sekretów w przeglądarce.** Klucze nie trafiają do `localStorage`,
   `sessionStorage`, cookies ani do stanu aplikacji React. Zmienne `VITE_*` są
   wkompilowywane w publiczny pakiet JavaScript i nigdy nie mogą zawierać
   sekretów.
3. **Magazyn systemowy.** Docelowo klucze będą przechowywane w mechanizmie
   systemu operacyjnego (Windows Credential Manager, macOS Keychain, Secret
   Service w Linuksie), a nie w plikach aplikacji.
4. **Odczyt w ostatniej chwili.** Backend pobiera klucz dopiero w momencie
   uruchamiania gałęzi i przekazuje go procesowi FFmpeg, nie zapisując go w
   logach.
5. **Maskowanie w diagnostyce.** Logi i eksporty diagnostyczne muszą mieć
   usunięte wartości wrażliwe.
6. **Nie zapisujemy sekretów w dokumentacji**, w tym w `docs/progress.md`.

## 11. Przyszła wersja serwerowa

Pierwsza wersja działa w całości lokalnie, ale architektura jest przygotowana na
przeniesienie routera transmisji na zdalny serwer:

- panel komunikuje się z backendem wyłącznie przez REST (a w przyszłości
  SSE/WebSocket) — nigdy bezpośrednio z MediaMTX ani z FFmpeg,
- adres API jest konfigurowalny po stronie frontendu,
- backend ma jawną, wąską listę dozwolonych źródeł (CORS) zamiast wildcardu,
- port i interfejs nasłuchu są konfigurowane zmiennymi środowiskowymi, domyślnie
  z ograniczeniem do pętli zwrotnej.

Wersja serwerowa będzie dodatkowo wymagać: uwierzytelnienia panelu, transportu
TLS oraz przemyślanego modelu przechowywania sekretów po stronie serwera. Żaden
z tych elementów nie jest jeszcze zaimplementowany.

---

## 12. Plan rozwoju projektu

| Etap | Zakres | Status |
| ---- | ------ | ------ |
| 1 | Fundamenty: struktura repozytorium, dokumentacja, panel React, minimalny backend Go, endpoint `/api/health` | **Ukończony** |
| 2 | Trwałe przechowywanie konfiguracji platform (SQLite), pełne CRUD API dla platform i metadanych | Planowany |
| 3 | Integracja z MediaMTX: uruchamianie procesu, generowanie konfiguracji, wykrywanie połączenia z OBS | Planowany |
| 4 | Gałęzie FFmpeg: uruchamianie, nadzór, restarty, izolacja awarii | Planowany |
| 5 | Statusy na żywo przez SSE lub WebSocket zamiast odpytywania | Planowany |
| 6 | Magazyn poświadczeń systemu operacyjnego dla kluczy transmisji | Planowany |
| 7 | Integracje z API platform: OAuth, wysyłanie metadanych, odczyt liczby widzów | Planowany |
| 8 | Widok logów i diagnostyka, eksport pakietu diagnostycznego | Planowany |
| 9 | Pakowanie aplikacji i tryb serwerowy | Planowany |

Kolejność może ulec zmianie, ale etapy 3 i 4 są zależne od etapu 2, a etap 7 od
etapu 6.

## 13. Zasada testów manualnych

**Testy manualne są etapem końcowym i wykonuje się je dopiero po ukończeniu
funkcjonalności aplikacji.**

Uzasadnienie: dopóki większość toru transmisji stanowią atrapy, testowanie
ręczne sprawdzałoby wyłącznie zachowanie danych demonstracyjnych i dawałoby
złudne poczucie gotowości.

W trakcie implementacji obowiązują natomiast kontrole automatyczne, które można
i należy uruchamiać na bieżąco:

- `npm run build` — build produkcyjny frontendu,
- `npm run lint` — analiza statyczna ESLint,
- `npm run typecheck` — kontrola typów TypeScript,
- `go build ./...` — kompilacja backendu,
- `go vet ./...` — analiza statyczna backendu,
- `gofmt -l .` — kontrola formatowania.

## 14. Uczciwość opisu stanu prac

W dokumentacji i w interfejsie obowiązuje zasada: **funkcja niezaimplementowana
nie jest przedstawiana jako gotowa.**

W praktyce oznacza to, że:

- dane demonstracyjne są oznaczone znacznikiem „Demo" w interfejsie oraz
  komentarzem w kodzie,
- przyciski, które nie wykonują realnej operacji, jasno to komunikują,
- podstrony bez implementacji pokazują informację o planowanym zakresie zamiast
  pozorowanych widżetów,
- wpis w `docs/progress.md` nie oznacza funkcji jako ukończonej, jeżeli jest ona
  wyłącznie atrapą interfejsu.
