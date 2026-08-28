# Third-party notices

The licence of Streaming Tree for OBS itself is documented in
[`LICENSE`](LICENSE) (GNU General Public License version 3 or any later
version, `GPL-3.0-or-later`; see [`LEGAL.md`](LEGAL.md)) - this file
covers third-party material only.

Streaming Tree for OBS uses third-party software. This file records what is
used, under which licence, and where the full licence text can be found on a
machine that has the software installed.

No third-party binary is committed to this repository. Dependencies are either
fetched by the language toolchains (Go modules, npm packages) or downloaded on
demand into the per-user application data directory.

---

## MediaMTX

| | |
| --- | --- |
| **Project** | MediaMTX |
| **Upstream** | <https://github.com/bluenviron/mediamtx> |
| **Pinned version** | **v1.19.3** |
| **Licence** | MIT Licence |
| **How it is obtained** | Downloaded on explicit user request from the official GitHub release for v1.19.3, and verified against the `checksums.sha256` file published with that same release. |
| **Where it is installed** | `<application data directory>/runtime/mediamtx/v1.19.3/<os>-<arch>/` |
| **Installed licence file** | `LICENSE`, beside the executable in the directory above |

MediaMTX is a separate program. Streaming Tree runs it as a child process and
communicates with it over a loopback-only Control API; it is not linked into the
Streaming Tree binary and its source is not redistributed here.

The `LICENSE` file shipped in the official archive is preserved during
installation and is never removed. An installation whose archive does not
contain a licence file is rejected.

The exact application data directory depends on the operating system and on the
`STREAMING_TREE_DATA_DIR` environment variable. See the "Data storage" section
of [`README.md`](README.md) for the resolved locations.

---

## FFmpeg

| | |
| --- | --- |
| **Project** | FFmpeg |
| **Upstream** | <https://ffmpeg.org/> |
| **Pinned version** | None - a minimum version is documented as a floor (4.4); actual compatibility is decided by probing capabilities (RTMP input/output, RTMPS output, the FLV muxer, `-progress` support), not by matching an exact version string. See the "Outgoing streaming with FFmpeg" section of [`README.md`](README.md). |
| **How it is obtained** | **Not obtained by this application at all.** Unlike MediaMTX, FFmpeg has no single official binary distributor this project can verify and download automatically - official releases are source only, and every ready-to-run binary comes from a third-party packager. Streaming Tree only locates and probes an executable you already have: an explicit `STREAMING_TREE_FFMPEG_PATH`, a possible future bundled location beside the backend, or the system `PATH`. |
| **Licence** | **Not one fixed licence.** Upstream FFmpeg defaults to LGPL-2.1-or-later, but a specific build commonly enables `--enable-gpl` (and sometimes non-free components), which changes its licence to GPL. Because the executable is entirely operator-provided, this project makes **no claim about the licence of whatever binary you point it at** - that determination belongs to whoever built or distributed that binary. |
| **How to inspect your own build** | Run `ffmpeg -version` and read its `configuration:` line, or check the distributor's own notice. `--enable-gpl` present means GPL; absent means the LGPL default (still subject to whatever its bundled codec libraries require). |

FFmpeg is a separate program. Streaming Tree runs it as one short-lived child
process per active destination branch (`exec.CommandContext`, never a shell)
and communicates with it only through its `-progress` stdout stream and
captured stderr; it is not linked into the Streaming Tree binary, and **no
FFmpeg binary is ever committed to this repository or downloaded by it.**

---

## Go dependencies

Declared in [`apps/server/go.mod`](apps/server/go.mod) and fetched by the Go
toolchain. The notable direct dependencies are:

| Project | Licence | Purpose |
| --- | --- | --- |
| [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) | BSD-3-Clause | Pure-Go SQLite driver, so the backend builds without a C toolchain |
| [`github.com/99designs/keyring`](https://github.com/99designs/keyring) v1.2.2 | MIT | Uniform interface over the operating system's credential store (Windows Credential Manager, macOS Keychain, Linux Secret Service) |
| [`github.com/coder/websocket`](https://github.com/coder/websocket) v1.8.15 | ISC | WebSocket client used by the Stage 8A Twitch EventSub connector (`internal/runtime/twitchengagement`) and, reused unchanged, the Stage 16A StreamElements Astro connector (`internal/provider/streamelements`, `internal/runtime/streamelementsengagement`) |
| [`google.golang.org/grpc`](https://github.com/grpc/grpc-go) v1.83.0 | Apache-2.0 | gRPC client used by the Stage 15A YouTube Live Chat connector's `streamList` server-streaming transport (`internal/provider/youtube`, `internal/runtime/youtubeengagement`) |
| [`google.golang.org/protobuf`](https://github.com/protocolbuffers/protobuf-go) v1.36.12 | BSD-3-Clause | Protocol Buffers runtime for the generated `streamlistpb` client code (`internal/provider/youtube/streamlistpb`) |
| [`github.com/go-ole/go-ole`](https://github.com/go-ole/go-ole) v1.3.0 | MIT | Go bindings for Windows COM Automation, used only by the Stage 17A Windows system text-to-speech provider (`internal/provider/tts/windows.go`, build-tagged `windows`) to call `SAPI.SpVoice`/`SAPI.SpMemoryStream`/`SAPI.SpAudioFormat` |
| [`golang.org/x/sys`](https://pkg.go.dev/golang.org/x/sys) v0.46.0 | BSD-3-Clause | The Go project's own extended platform-syscall package (already an indirect dependency via other modules above); used directly since Stage 20A for the Windows-only `ShellExecuteW`/`CreateMutexW`/`MessageBoxW` calls behind `internal/runtime/browserlaunch`, `internal/runtime/singleinstance`, and `internal/runtime/nativealert` (each build-tagged `windows`, with a no-op/fallback `!windows` counterpart) |

`coder/websocket` (the maintained continuation of the formerly-popular
`nhooyr.io/websocket`) was chosen after checking: actively maintained, pure Go
(no CGO, consistent with `modernc.org/sqlite`'s own reason for existing in this
project), first-class `context.Context` support on every blocking call
(`Dial`, `Read`), a built-in per-connection frame-size limit
(`Conn.SetReadLimit`), transparent handling of the standard WebSocket
ping/pong control frames, and clean, typed close-code/close-reason handling
(`CloseStatus`, `CloseError`) - all directly used by the connector's welcome/
keepalive/reconnect state machine. It connects outbound only, to Twitch's own
EventSub endpoint (or, in the `-tags integration` test binary only, to a local
loopback fake server) - Streaming Tree never runs a WebSocket *server* itself.

`99designs/keyring` was chosen over the more commonly cited alternative
(`zalando/go-keyring`) after reading its source directly: `zalando/go-keyring`
shells out to the `security` command line tool on macOS, which this project's
credential-storage rules explicitly forbid. `99designs/keyring`'s backends for
the three platforms Streaming Tree targets are native bindings instead - Win32
credential APIs on Windows, the macOS Security framework via CGO on macOS, and
D-Bus on Linux - never a shelled-out command. `internal/secrets` restricts it
to exactly those three backends: the library also ships a `pass` backend
(shells out to the external `pass` command) and a `file` backend
(password-encrypted file on disk), and both are excluded, since shelling out
and a file-based fallback are equally forbidden here.

Its transitive dependencies, all MIT or BSD-licensed, are pulled in for those
same three backends:

| Project | Licence | Backend |
| --- | --- | --- |
| [`github.com/99designs/go-keychain`](https://github.com/99designs/go-keychain) | MIT | macOS Keychain (CGO binding to Security.framework) |
| [`github.com/danieljoos/wincred`](https://github.com/danieljoos/wincred) | MIT | Windows Credential Manager (pure syscall binding, no CGO) |
| [`github.com/godbus/dbus`](https://github.com/godbus/dbus) | BSD-2-Clause | Linux Secret Service (D-Bus transport) |
| [`github.com/gsterjov/go-libsecret`](https://github.com/gsterjov/go-libsecret) | MIT | Linux Secret Service (libsecret protocol over D-Bus) |
| [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) | BSD-3-Clause | Terminal handling used by keyring's optional interactive-prompt path (unused by Streaming Tree, which never prompts) |
| [`github.com/dvsekhvalnov/jose2go`](https://github.com/dvsekhvalnov/jose2go) | MIT | Encryption for keyring's password-encrypted-file backend |
| [`github.com/mtibben/percent`](https://github.com/mtibben/percent) | MIT | Path escaping for keyring's password-encrypted-file backend |

The last two are compiled in because keyring's file-based backend has no
platform build constraint, but that backend is never selected: `internal/secrets`
restricts `AllowedBackends` to the three OS-native backends above, so the
file-based (and `pass`-based) backends are dead code in this binary, not a
storage path Streaming Tree can reach.

The macOS Keychain backend requires CGO and the Xcode command line tools to
build, an accepted trade-off: the only CGO-free alternative for macOS would be
shelling out to the `security` command, which is not permitted. This has not
been build-verified on macOS as part of this stage; see `docs/progress.md`.

`google.golang.org/grpc` and `google.golang.org/protobuf` were added in the
Stage 15A transport corrective pass (docs/provider-integrations/
youtube-engagement.md §4b) to replace the connector's original REST-polling
receive transport with the official `liveChatMessages.streamList` gRPC
server-streaming RPC. Both are Google's own official Go implementations of
gRPC and Protocol Buffers respectively - the same libraries any Go gRPC
client for any service would use, not YouTube-specific. Their transitive
dependencies (`golang.org/x/net`, `golang.org/x/text`,
`google.golang.org/genproto/googleapis/rpc`) are all Apache-2.0 or
BSD-licensed, standard-library-adjacent Google modules pulled in solely to
support the gRPC/protobuf runtime, and carry no separate license obligations
beyond their own upstream notices in the Go module cache.

`apps/server/internal/provider/youtube/streamlistpb/stream_list.proto` is
vendored (not authored) from Google's own YouTube Live Streaming API
documentation (`https://developers.google.com/youtube/v3/live/
streaming-live-chat`), whose code samples - including this proto - are
licensed under Apache License 2.0 (`https://developers.google.com/
site-policies#license`). See that file's own header comment for the exact
source and the two small additions (one `import`, one `go_package` option)
made to it, and `streamlistpb/README.md` for the maintainer-only
regeneration procedure. The generated `.pb.go`/`_grpc.pb.go` files are
Apache-2.0-licensed derivative output of that same proto, produced by
Google's own `protoc-gen-go`/`protoc-gen-go-grpc` tools.

`go-ole` was added in Stage 17A (docs/audio-tts.md §22) as the Go
interoperability path for Windows SAPI, chosen after auditing its upstream
source directly: no CGO ("Go bindings for Windows COM using shared libraries
instead of cgo," consistent with `modernc.org/sqlite`'s own reason for
existing in this project), MIT licensed, and its `IDispatch`/`VARIANT`/
`oleutil` surface is exactly what calling a COM Automation object like
`SAPI.SpVoice` from an ordinary unpackaged Win32 process needs - confirmed by
using it to build the real provider, tested against the actual installed
SAPI engine on a development machine (`internal/provider/tts/windows_test.go`).
It is linked directly into the Windows build of `apps/server`; the
non-Windows build (`internal/provider/tts/stub.go`, build-tagged `!windows`)
does not import it at all, so `go-ole` never appears in a Linux/macOS binary.
No voice model, engine, or other SAPI component is bundled by this
project - `internal/provider/tts` only calls into whatever SAPI installation
already exists on the machine running Streaming Tree.

Full licence texts are available in the Go module cache
(`go env GOMODCACHE`) and on each project's page.

---

## npm dependencies

Declared in [`apps/web/package.json`](apps/web/package.json) and fetched by npm.
The notable direct dependencies are React, React Router, TanStack Query, i18next,
Zod, Tailwind CSS and Lucide icons, all under permissive licences (MIT or ISC).

Full licence texts are installed in `apps/web/node_modules/<package>/` after
`npm install`.

---

## Provider brand marks (Twitch, YouTube, Kick, TikTok)

| | |
| --- | --- |
| **Source project** | Simple Icons |
| **Upstream** | <https://github.com/simple-icons/simple-icons> |
| **Package/version used** | `simple-icons` npm package, version `16.28.0` |
| **Licence (this project's own markup/geometry contribution)** | CC0-1.0 - <https://github.com/simple-icons/simple-icons/blob/master/LICENSE.md> |
| **Role** | Four static `.svg` files (`twitch.svg`, `youtube.svg`, `kick.svg`, `tiktok.svg`) copied byte-for-byte, unmodified, into `apps/web/src/assets/providers/`, rendered locally by `ProviderBrand` (`apps/web/src/components/providers/ProviderBrand.tsx`). Not an npm dependency - the package itself is never installed or imported; only the four files were retrieved from it and vendored, so `package.json` carries no entry for it. |

**CC0 covers Simple Icons' own contribution only - it is not a grant of
trademark rights to the underlying marks.** Per Simple Icons' own
DISCLAIMER (<https://github.com/simple-icons/simple-icons/blob/master/DISCLAIMER.md>,
quoted in full in `docs/provider-branding.md`), the marks depicted by
these four files remain the trademarks of Twitch Interactive, Inc.
(Twitch), Google LLC (YouTube), Kick Streaming Pty Ltd (Kick), and
TikTok Pty Ltd (TikTok) respectively. Streaming Tree uses them
nominatively only - to identify, inside the operator's own local
dashboard, which platform a destination the operator configured
connects to - never to imply sponsorship, endorsement, or affiliation,
and never on merchandise or marketing material. Full per-provider
sourcing (exact upstream `source`/`guidelines` URL, retrieval date,
official-vs-fallback determination, and the one real usage limitation
for TikTok) is recorded in `docs/provider-branding.md`, not duplicated
here.

---

## Inno Setup (Windows installer build tool)

| | |
| --- | --- |
| **Project** | Inno Setup |
| **Upstream** | <https://jrsoftware.org/isinfo.php> |
| **Licence** | Inno Setup License (custom, free for any use including commercial) - <https://jrsoftware.org/files/is/license.txt> |
| **Role** | Build-only tool (`scripts/build-release.ps1`, `scripts/installer/streaming-tree.iss`, Stage 20A). Not a Go or npm dependency, not imported by any source file, and not fetched by any package manager - installed separately on the build machine (`winget install --id JRSoftware.InnoSetup`). |

This is a genuinely different question from every dependency above:
Inno Setup's *build-tool* licence (free to use to compile an installer)
is separate from what its *output* actually contains. The installer
`.exe` that `ISCC.exe` produces embeds Inno Setup's own compiled
bootstrap/uninstaller stub code - licensed for exactly this by its own
terms ("compiled installers produced by Inno Setup can be distributed
commercially without restriction"), so redistributing a Streaming Tree
for OBS installer built with it is permitted. That installer otherwise
contains only Streaming Tree for OBS's own release executable (which
itself embeds the production frontend and the four legal documents -
see `docs/windows-packaging.md` §2/§16) - no other third-party binary
is added by packaging.

---

## Trademarks

Twitch, YouTube, Kick and TikTok are trademarks of their respective owners.
Streaming Tree identifies them by name and, as of Stage 20E, by a small
locally-vendored brand mark on destination cards and settings (see
"Provider brand marks" above and `docs/provider-branding.md`) - never a
redrawn, recoloured-beyond-documented-treatment, or hotlinked logo, and
never in a way that implies sponsorship or endorsement. No other
third-party brand asset is included in this repository.
