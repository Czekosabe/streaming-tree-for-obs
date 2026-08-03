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
toolchain. The notable direct dependency is:

| Project | Licence | Purpose |
| --- | --- | --- |
| [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) | BSD-3-Clause | Pure-Go SQLite driver, so the backend builds without a C toolchain |

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
