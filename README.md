# Streaming Tree for OBS

Lokalna aplikacja, która pozwala wysłać **jeden** strumień z OBS i rozgałęzić go
na wiele platform jednocześnie — Twitch, YouTube, Kick, TikTok.

Nazwa opisuje model działania: strumień z OBS to „pień", a każda platforma to
niezależna „gałąź" transmisji. Awaria jednej gałęzi nie zatrzymuje pozostałych.

> ## Stan projektu: etap fundamentów
>
> **Aplikacja nie transmituje jeszcze niczego.** Ten etap obejmuje strukturę
> projektu, dokumentację, panel operatorski w React oraz minimalny backend w Go
> z endpointem statusowym.
>
> MediaMTX, FFmpeg, logowanie OAuth, integracje z API platform oraz baza danych
> **zostaną dodane w kolejnych etapach**. Wszystkie elementy demonstracyjne są
> oznaczone w interfejsie znacznikiem **Demo** — pełna lista znajduje się
> w sekcji [Co jest obecnie tylko demonstracyjne](#co-jest-obecnie-tylko-demonstracyjne).

Szczegółowy opis projektu: [`docs/project-overview.md`](docs/project-overview.md)
Dziennik prac: [`docs/progress.md`](docs/progress.md)

---

## Spis treści

- [Wymagania](#wymagania)
- [Szybki start](#szybki-start)
- [Frontend — instalacja i uruchomienie](#frontend--instalacja-i-uruchomienie)
- [Backend Go — uruchomienie](#backend-go--uruchomienie)
- [Build produkcyjny](#build-produkcyjny)
- [Lint, typecheck i pozostałe kontrole](#lint-typecheck-i-pozostałe-kontrole)
- [Struktura katalogów](#struktura-katalogów)
- [Co jest obecnie tylko demonstracyjne](#co-jest-obecnie-tylko-demonstracyjne)
- [Bezpieczeństwo kluczy transmisji](#bezpieczeństwo-kluczy-transmisji)
- [Najczęstsze problemy](#najczęstsze-problemy)

---

## Wymagania

| Narzędzie | Wersja | Do czego służy | Wymagane teraz? |
| --------- | ------ | -------------- | --------------- |
| **Node.js** | 20.19+ lub 22.12+ (zalecane 22 LTS lub nowsze) | uruchomienie panelu React | tak |
| **npm** | 10+ | instalacja zależności frontendu | tak |
| **Go** | 1.22 lub nowsze | kompilacja i uruchomienie backendu | tak |
| OBS Studio | 30+ | źródło transmisji | jeszcze nie |
| MediaMTX | — | odbiór strumienia RTMP | jeszcze nie |
| FFmpeg | — | rozsyłanie gałęzi transmisji | jeszcze nie |

Sprawdzenie zainstalowanych wersji:

```bash
node --version
npm --version
go version
```

> **Uwaga dotycząca wersji Node.** Projekt został skonfigurowany tak, aby
> działał również na Node 22.11. Jeżeli jednak masz Node starszy niż 22.12,
> aktualizacja jest zalecana: nowsze narzędzia frontendowe (Vite 7/8) wymagają
> Node `^20.19 || >=22.12`, a przy starszej wersji npm pomija ich zależności
> natywne. Szczegóły w [`docs/progress.md`](docs/progress.md).

Jeżeli nie masz jeszcze Go, pobierz je ze strony <https://go.dev/dl/> i wykonaj
instalację dla swojego systemu. Instalator dodaje `go` do zmiennej `PATH`; po
instalacji otwórz **nowe** okno terminala.

---

## Szybki start

Aplikacja składa się z dwóch procesów, które uruchamia się w **dwóch osobnych
terminalach**.

**Terminal 1 — backend:**

```bash
cd apps/server
go run ./cmd/server
```

**Terminal 2 — frontend:**

```bash
cd apps/web
npm install
npm run dev
```

Następnie otwórz <http://localhost:5173>.

Panel działa również **bez uruchomionego backendu** — w sekcji statusu systemu
pojawi się wtedy czytelny komunikat „Backend unavailable", a reszta interfejsu
pozostanie sprawna.

---

## Frontend — instalacja i uruchomienie

### Instalacja zależności

```bash
cd apps/web
npm install
```

Polecenie wykonuje się raz, a potem po każdej zmianie zależności. Zależności
trafiają do katalogu `apps/web/node_modules`, który nie jest wersjonowany.

### Uruchomienie w trybie deweloperskim

```bash
npm run dev
```

Serwer deweloperski wystartuje pod adresem <http://localhost:5173> i będzie
przeładowywał aplikację po każdej zmianie w kodzie. Zapytania do `/api` są
automatycznie przekazywane do backendu na `http://127.0.0.1:8080`.

Zatrzymanie: `Ctrl + C`.

### Konfiguracja (opcjonalna)

Domyślne ustawienia wystarczają do pracy lokalnej. Jeżeli backend działa pod
innym adresem, skopiuj `apps/web/.env.example` do `apps/web/.env.local`
i dostosuj wartości.

> **Nigdy nie umieszczaj sekretów w plikach `.env` frontendu.** Wszystko
> z przedrostkiem `VITE_` jest wkompilowywane w publiczny pakiet JavaScript
> i widoczne dla każdego, kto otworzy stronę.

---

## Backend Go — uruchomienie

### Uruchomienie bez kompilowania pliku wykonywalnego

```bash
cd apps/server
go run ./cmd/server
```

W konsoli pojawi się log potwierdzający nasłuch:

```
level=INFO msg="http server listening" service=streaming-tree-server version=0.1.0 address=127.0.0.1:8080
```

Zatrzymanie: `Ctrl + C`. Serwer zamyka się w sposób kontrolowany, czekając na
dokończenie trwających żądań (maksymalnie 10 sekund).

### Sprawdzenie endpointu statusowego

```bash
curl http://127.0.0.1:8080/api/health
```

Przykładowa odpowiedź:

```json
{
  "status": "ok",
  "service": "streaming-tree-server",
  "version": "0.1.0",
  "uptimeSeconds": 12.34,
  "time": "2026-08-03T11:36:38Z"
}
```

W systemie Windows bez `curl` można użyć PowerShella:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/health
```

### Konfiguracja przez zmienne środowiskowe

| Zmienna | Domyślnie | Opis |
| ------- | --------- | ---- |
| `STREAMING_TREE_HOST` | `127.0.0.1` | Interfejs nasłuchu. Domyślnie tylko pętla zwrotna, aby nie wystawiać serwera do sieci lokalnej. |
| `STREAMING_TREE_PORT` | `8080` | Port REST API. |
| `STREAMING_TREE_ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Lista dozwolonych origin dla CORS, rozdzielona przecinkami. |

Przykład — uruchomienie na innym porcie:

```bash
# Linux / macOS
STREAMING_TREE_PORT=9000 go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:STREAMING_TREE_PORT="9000"; go run ./cmd/server
```

Niepoprawna wartość powoduje czytelny błąd przy starcie zamiast cichego powrotu
do wartości domyślnej.

### Kompilacja pliku wykonywalnego

```bash
cd apps/server
go build -o bin/streaming-tree-server ./cmd/server
```

W systemie Windows:

```powershell
go build -o bin/streaming-tree-server.exe ./cmd/server
```

Katalog `bin/` jest ignorowany przez Git.

---

## Build produkcyjny

### Frontend

```bash
cd apps/web
npm run build
```

Wynik trafia do `apps/web/dist/`. Build wykonuje najpierw kontrolę typów, więc
błąd typu przerywa budowanie.

Podgląd zbudowanej wersji:

```bash
npm run preview
```

### Backend

```bash
cd apps/server
go build ./...
```

---

## Lint, typecheck i pozostałe kontrole

Kontrole automatyczne można i należy uruchamiać w trakcie pracy. Testy manualne
interfejsu są etapem końcowym — patrz `docs/project-overview.md`, sekcja 13.

**Frontend** (z katalogu `apps/web`):

```bash
npm run lint        # ESLint
npm run typecheck   # kontrola typów TypeScript (tsc -b)
npm run build       # build produkcyjny
```

**Backend** (z katalogu `apps/server`):

```bash
go build ./...      # kompilacja
go vet ./...        # analiza statyczna
gofmt -l .          # lista plików wymagających sformatowania (pusta = wszystko OK)
```

---

## Struktura katalogów

```
.
├── apps/
│   ├── web/                    # Panel operatorski (React + TypeScript + Vite)
│   │   ├── src/
│   │   │   ├── app/            # Konfiguracja TanStack Query
│   │   │   ├── components/
│   │   │   │   ├── layout/     # Powłoka: panel boczny, górny pasek
│   │   │   │   ├── metadata/   # Edytor metadanych z zakładkami platform
│   │   │   │   ├── platforms/  # Karty gałęzi transmisji
│   │   │   │   ├── system/     # Panel statusu systemu i backendu
│   │   │   │   └── ui/         # Elementy bazowe (przyciski, pola, panele)
│   │   │   ├── data/           # DANE DEMONSTRACYJNE
│   │   │   ├── hooks/          # Hooki (m.in. zapytanie o stan backendu)
│   │   │   ├── lib/            # Klient API, pomocnicze funkcje
│   │   │   ├── models/         # Model domeny + schematy Zod
│   │   │   ├── pages/          # Widoki tras
│   │   │   └── state/          # STAN DEMONSTRACYJNY (atrapa)
│   │   └── ...                 # Konfiguracja Vite, TypeScript, ESLint
│   │
│   └── server/                 # Backend (Go)
│       ├── cmd/server/         # Punkt wejścia, kontrolowane zamykanie
│       └── internal/
│           ├── buildinfo/      # Nazwa usługi i wersja
│           ├── config/         # Konfiguracja ze zmiennych środowiskowych
│           └── httpapi/        # Router, handlery, middleware, odpowiedzi JSON
│
├── config/                     # Konfiguracja MediaMTX i FFmpeg (etap przyszły)
├── docs/
│   ├── project-overview.md     # Pełny opis projektu
│   └── progress.md             # Dziennik prac
├── .gitignore
└── README.md
```

---

## Co jest obecnie tylko demonstracyjne

Wszystkie poniższe elementy są oznaczone w interfejsie znacznikiem **Demo** lub
opisem wprost przy kontrolce.

| Element | Rzeczywiste zachowanie |
| ------- | ---------------------- |
| Przyciski **Start / Stop** na kartach platform | Zmieniają wyłącznie stan w pamięci przeglądarki. Nie uruchamiają żadnego procesu i nie wysyłają żadnych danych. |
| Statusy platform (offline / starting / live / error) | Stan początkowy jest zapisany na stałe w kodzie; „starting" przechodzi w „live" po ok. 1,8 s. |
| Liczba widzów, jakość połączenia | Wartości stałe. Żadna platforma nie jest odpytywana. |
| CPU, pamięć, dysk, sieć | Wartości stałe. Backend nie zbiera metryk hosta. |
| Status połączenia OBS | Zawsze „Waiting for OBS". Nic nie nasłuchuje na porcie RTMP. |
| Adres RTMP w panelu bocznym | Adres planowany, nie działający. |
| Zapis metadanych | Trafia wyłącznie do pamięci przeglądarki. Odświeżenie strony przywraca wartości początkowe. |
| Tabele możliwości platform | Konfiguracja przybliżona, przygotowana na potrzeby demonstracji edytora. Wymaga weryfikacji przy wdrażaniu realnych integracji. |
| Podstrony Platforms, Streams, Metadata, Settings, Logs | Widoki informacyjne opisujące planowany zakres. Bez implementacji. |

**Jedyne realne połączenie z backendem** w tym etapie to `GET /api/health`.
Wynik tego zapytania jest prezentowany w karcie „Backend" w prawej kolumnie.

### Co zostanie dodane później

- **MediaMTX** — lokalny serwer odbierający strumień RTMP z OBS.
- **FFmpeg** — po jednym procesie na każdą gałąź transmisji.
- **SQLite** — trwałe przechowywanie konfiguracji platform i metadanych.
- **SSE lub WebSocket** — statusy na żywo zamiast odpytywania.
- **Magazyn poświadczeń systemu** — bezpieczne przechowywanie kluczy transmisji.
- **OAuth i API platform** — logowanie oraz wysyłanie metadanych.

---

## Bezpieczeństwo kluczy transmisji

Klucz transmisji pozwala nadawać na cudzym kanale, więc traktujemy go jak hasło.

- **Repozytorium nie zawiera żadnych sekretów** i nie może ich zawierać.
  `.gitignore` blokuje pliki `.env` oraz katalogi danych.
- **Klucze nie będą przechowywane w przeglądarce** — ani w `localStorage`, ani
  w `sessionStorage`, ani w stanie aplikacji.
- **Docelowym miejscem przechowywania jest magazyn poświadczeń systemu
  operacyjnego** (Windows Credential Manager, macOS Keychain, Secret Service).
- Backend odczyta klucz dopiero w chwili uruchamiania gałęzi i nie zapisze go
  w logach.

Obsługa kluczy transmisji **nie została jeszcze rozpoczęta**.

---

## Najczęstsze problemy

**Panel pokazuje „Backend unavailable".**
Backend nie jest uruchomiony albo działa na innym porcie. Uruchom go w drugim
terminalu (`cd apps/server && go run ./cmd/server`) i użyj przycisku odświeżania
w karcie „Backend". To oczekiwany, w pełni obsłużony stan — panel nie ulega
awarii.

**`go: command not found` lub `'go' nie jest rozpoznawane`.**
Go nie jest zainstalowane lub nie znalazło się w `PATH`. Zainstaluj je ze strony
<https://go.dev/dl/> i otwórz nowe okno terminala.

**`npm install` kończy się błędem „Cannot find native binding".**
Wersja Node jest starsza niż wymagana przez zależności natywne. Zaktualizuj Node
do 22.12+ lub 24 LTS, usuń `apps/web/node_modules` oraz
`apps/web/package-lock.json` i powtórz instalację.

**Port 8080 lub 5173 jest zajęty.**
Backend: uruchom z inną wartością `STREAMING_TREE_PORT` i dopisz nowy adres do
`VITE_DEV_API_PROXY_TARGET` w `apps/web/.env.local`.
Frontend: Vite sam zaproponuje kolejny wolny port; pamiętaj wtedy o dodaniu
nowego origin do `STREAMING_TREE_ALLOWED_ORIGINS`.

**Zmiany w interfejsie nie są widoczne.**
Sprawdź, czy `npm run dev` nadal działa i czy w konsoli przeglądarki nie ma
błędów. W razie potrzeby przeładuj stronę z pominięciem pamięci podręcznej
(`Ctrl + Shift + R`).
