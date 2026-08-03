# `config/`

Katalog przeznaczony na konfigurację komponentów infrastrukturalnych, które
zostaną dodane w kolejnych etapach projektu.

## Stan obecny

**Katalog nie zawiera jeszcze żadnej działającej konfiguracji.** Aplikacja
w obecnej wersji nie uruchamia MediaMTX ani FFmpeg, więc nie ma czego
konfigurować.

## Planowana zawartość

| Plik (planowany)     | Przeznaczenie                                                             |
| -------------------- | ------------------------------------------------------------------------- |
| `mediamtx.yml`       | Konfiguracja MediaMTX: nasłuch RTMP z OBS, ścieżki, uwierzytelnianie lokalne. |
| `ffmpeg-profiles.json` | Profile parametrów FFmpeg per platforma (bitrate, keyframe interval, format wyjściowy). |
| `server.example.yml` | Przykładowa konfiguracja backendu Go (port, ścieżki, limity).              |

## Zasady

1. W tym katalogu **nie wolno** przechowywać kluczy transmisji, tokenów OAuth
   ani żadnych innych sekretów. Trafią one do systemowego magazynu poświadczeń
   (Windows Credential Manager / macOS Keychain / Secret Service).
2. Pliki konfiguracyjne trzymane w repozytorium to wyłącznie szablony
   i wartości domyślne. Konfiguracja lokalna użytkownika (`*.local.yml`, `.env`)
   jest ignorowana przez `.gitignore`.
3. Każdy plik dodany tutaj musi zostać opisany w `docs/progress.md`.
