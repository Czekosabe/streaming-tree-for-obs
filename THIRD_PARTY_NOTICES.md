# Third-party notices

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

## Go dependencies

Declared in [`apps/server/go.mod`](apps/server/go.mod) and fetched by the Go
toolchain. The notable direct dependencies are:

| Project | Licence | Purpose |
| --- | --- | --- |
| [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) | BSD-3-Clause | Pure-Go SQLite driver, so the backend builds without a C toolchain |
| [`github.com/99designs/keyring`](https://github.com/99designs/keyring) v1.2.2 | MIT | Uniform interface over the operating system's credential store (Windows Credential Manager, macOS Keychain, Linux Secret Service) |

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

## Trademarks

Twitch, YouTube, Kick and TikTok are trademarks of their respective owners.
Streaming Tree refers to them by name only; no logo, icon or other brand asset
is included in this repository. Platform markers in the interface are plain
text labels.
