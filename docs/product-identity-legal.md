# Product identity and legal surfaces (pre-Stage-20 milestone)

This is the canonical contract for Streaming Tree for OBS's public
product/creator identity and the About/Legal/Privacy/Support surfaces built
on top of it, written before any of that product code, per this project's
own standing "contract before implementation" discipline (see
`docs/goals-widgets.md`, `docs/supporter-widgets.md`,
`docs/provider-integrations/tiktok-live.md` for precedent).

## 1. Binding product identity

| Field | Value |
| --- | --- |
| Product name | Streaming Tree for OBS |
| Public creator/author identity | **Czekosabe** |
| Canonical source repository | <https://github.com/Czekosabe/streaming-tree-for-obs> |
| Creator GitHub profile | <https://github.com/Czekosabe> |
| Creator support URL (current) | <https://streamelements.com/czekosabe/tip> |

**Czekosabe is the ONLY public creator identity the application ever
displays.** The application never shows, infers, or derives:

- a real first name or surname;
- the local Windows username the application happens to run under;
- the repository-local or global Git commit author email
  (`kacper2280@tlen.pl` in this checkout's own commit history - that value
  is Git metadata about this development environment, not product
  metadata, and must never appear in any user-facing surface, API
  response, or generated document);
- the OS account name;
- any other personal identity, even though such information exists on the
  machine this code happens to run on today.

This rule is enforced structurally, not by convention alone: the single
canonical source of these fields is `internal/buildinfo` (Go constants),
which the frontend's About page fetches via `GET /api/about` - there is no
code path anywhere in this application that reads `os/user`, a Git config
file, or an environment variable to populate a "creator" or "author" field.

## 2. Single canonical source of truth

`apps/server/internal/buildinfo/buildinfo.go` is the one place
`ProductName`, `CreatorName`, `RepositoryURL`, `CreatorURL`, `SupportURL`,
`ApplicationLicenseSPDX`, and `ApplicationLicenseName` are defined as Go
constants. `GET /api/about`
(`internal/httpapi/about.go`) is the one HTTP contract that exposes them.
The frontend's About & Legal page (`apps/web/src/pages/AboutLegalPage.tsx`)
fetches that endpoint rather than keeping a second, independently
maintained copy of these strings - changing the support URL, for example,
means editing exactly one Go constant, matching §16 of the governing task
for this milestone (no database migration, no saved user configuration, no
per-component literal to hunt down).

The pre-existing frontend `apps/web/src/data/app-info.ts` (`APP_INFO.name`/
`APP_INFO.version`) was, at the time this contract was written, a
narrower, unrelated concern - the sidebar footer's own local chrome
label - left untouched by this milestone. **Correction (final
product-polish audit, 2026-09-02):** that file was later removed
entirely during a Stage 20E dashboard-realignment pass (see
`docs/progress.md`) - its hardcoded `version: '0.1.0'` was
found stale, and `SidebarFooter` now reads real build identity from
`useAboutQuery()`/`GET /api/about` the same as this page does, so the
"single canonical source of truth" this section describes now holds
for the sidebar footer too, not just the About & Legal page.

## 3. Creator support - what it is and is not

- Support is entirely **voluntary**.
- Support **does not unlock any application feature**. Every feature in
  Streaming Tree for OBS is available identically whether or not a user
  ever visits the support link.
- Support **is not the purchase of an application licence**. It has no
  bearing on §5's separate application-licence question, resolved
  independently below.
- Support **does not buy priority support** or any support/SLA
  entitlement.
- The transaction happens entirely on an external StreamElements page,
  opened in the user's default browser. Streaming Tree for OBS:
  - never receives payment-card data;
  - never processes the transaction;
  - never receives whatever the user enters on that external page.
- The support URL is **application-owned and fixed** - it is never
  user-configurable, never accepted through a frontend request body, never
  persisted in SQLite, and there is no generic "open arbitrary URL"
  backend API that could be repurposed to point anywhere else.
- **No support popup, startup nag, dismissible banner, countdown, donation
  goal, donation amount field, payment form, embedded checkout, iframe, or
  QR code exists or is planned by this milestone.** The support action is
  a single, quiet external link inside the About page - see §13 of the
  governing task for this milestone, honored exactly.
- **This is a completely separate concern from the Stage 16 external-
  donation system.** `internal/domain/donationsource`, the StreamElements
  Astro connector, donation Event Bus events, alerts, TTS, goals, and
  supporter widgets are the *streamer's own* engagement pipeline for
  *their own* viewers' donations - see
  `docs/provider-integrations/external-donations.md`. A person who clicks
  "Support the creator" in their own copy of Streaming Tree for OBS is
  supporting Czekosabe, the application's developer, on Czekosabe's own
  StreamElements page - this can never become, feed, or be confused with
  an engagement donation event inside *their* application, and no code in
  this milestone connects the two.
- No support-button click analytics, conversion tracking, unique
  identifiers, referral query strings, or tracking pixels exist. A click is
  a plain browser navigation to a fixed external HTTPS URL - nothing about
  it is logged beyond what a normal `<a target="_blank">` already does.

## 4. Future-proofing the support destination

Today's canonical support destination is StreamElements. If this ever
changes (e.g. to a Czekosabe-owned landing page, GitHub Sponsors, or
another provider), the only required change is
`buildinfo.SupportURL` - no other file, test, or documentation should hold
a second literal copy of this URL (aside from this contract document and
`PRIVACY.md`/`LEGAL.md`, which describe the *current* value and are
expected to be updated alongside the constant if it ever changes). This
milestone does not implement that future landing page and does not create
any other support-provider account.

## 5. Application licence

### 5.1 Original audit (2026-08-17) - historical context

At the time this document was first written, **no application-wide licence
file existed in this repository** (no root `LICENSE`, `LICENSE.*`, or
`COPYING` file - confirmed by direct directory listing; only
`THIRD_PARTY_NOTICES.md` and `README.md` existed at the repository root
alongside `apps/`, `config/`, `docs/`, and `scripts/`). Neither
`apps/web/package.json` nor `apps/server/go.mod` declared a licence field.

The only licence-related text anywhere in the repository concerned
**MediaMTX**, a bundled third-party dependency under its own MIT licence -
explicitly *not* Streaming Tree for OBS's own licence and never to be read
as one (README.md's own MediaMTX packaging section already made this
distinction; see also `THIRD_PARTY_NOTICES.md`).

That original milestone deliberately did not select, invent, or guess an
application licence - it recorded this as an explicit, unresolved
**operator decision required before the first public packaged release**.
This was correct at the time and is preserved here as historical record,
not rewritten - see §5.2 for the decision that has since resolved it.

### 5.2 Operator decision (2026-08-18)

**The operator has selected an application licence.** This is no longer
unresolved.

| | |
| --- | --- |
| Selected licence | GNU General Public License version 3, or (at the recipient's option) any later version |
| SPDX expression | `GPL-3.0-or-later` |
| Copyright identity | Copyright (C) 2026 Czekosabe |
| Canonical full text | [`LICENSE`](../LICENSE) (repository root, verbatim official GNU text, byte-identical to `https://www.gnu.org/licenses/gpl-3.0.txt` as fetched on 2026-08-18) |

**Reason:** the operator wants distributed forks and modified,
redistributed versions of Streaming Tree for OBS to remain open under a
strong copyleft licence, rather than permitting a closed, proprietary
redistribution of this codebase.

**What this decision does NOT mean** (explicitly, to prevent
misdescription elsewhere in this project's docs/UI):

- **Commercial use and commercial distribution are permitted.** The GPL
  is not a "non-commercial" licence, and this project adds no extra
  restriction on top of it - no "personal use only," no "no resale," no
  "permission required for companies," and no other custom restriction
  layered onto the standard GPL terms.
- **Private, un-distributed modification does not require publishing
  source to the world.** The GPL's source-availability/copyleft
  obligations attach to *conveying* (distributing) the program or a
  modified version of it - not to modifying your own private copy and
  never sharing it.
- **This is the ordinary GNU GPL, not the GNU Affero GPL (AGPL).** It
  carries no separate obligation to offer source to users who merely
  interact with the program over a network (there is no Streaming-Tree-
  operated network service for that distinction to apply to in any case
  - see `PRIVACY.md`).
- **Creator support (§3 above) is entirely independent of this licence
  decision.** Supporting Czekosabe voluntarily is not a licence purchase
  and has no bearing on the rights this licence already grants for free.

**Ownership audit (before applying the licence):** `git shortlog -sne
--all` and `git log --format='%an <%ae>' | sort | uniq` show exactly one
real contributor identity across the entire repository history - the
same email (`kacper2280@tlen.pl`) under two different local Git
author-name spellings ("Czekosabe" and "kacper2280"), i.e. the same
person/repository operator, never a distinct third-party human
contributor. There is no genuine other contributor whose separately-
copyrightable work would need independent relicensing consent, so this
milestone proceeded without a blocker.

**Third-party compatibility audit:** the application licence governs
Streaming Tree for OBS's own first-party source only. Every third-party
component actually combined into a distributed build was checked against
`THIRD_PARTY_NOTICES.md`, `apps/server/go.mod`, and
`apps/web/package.json`:

- Go dependencies actually linked into the backend binary: MIT (`99designs/
  keyring` and its MIT transitive backends), BSD-3-Clause (`modernc.org/
  sqlite`, `google.golang.org/protobuf`, `golang.org/x/term`),
  BSD-2-Clause (`godbus/dbus`), ISC (`coder/websocket`), Apache-2.0
  (`google.golang.org/grpc` and its Apache-2.0/BSD transitive modules).
- npm dependencies actually bundled into the frontend build: MIT/ISC
  (React, React Router, TanStack Query, i18next, Zod, Tailwind CSS,
  Lucide icons, per `THIRD_PARTY_NOTICES.md`).
- The vendored/generated YouTube `streamlistpb` material
  (`apps/server/internal/provider/youtube/streamlistpb/`) is Apache-2.0,
  carrying its own pre-existing attribution header naming Google as the
  source - **left completely untouched by this milestone**, no GPL header
  was added to it, and it is not claimed to be authored or relicensed by
  Czekosabe.
- **Apache License 2.0 compatibility with GPLv3 was verified directly
  against the Free Software Foundation's own license-list page
  (`https://www.gnu.org/licenses/license-list.html#apache2`, fetched
  2026-08-18): "a free software license, compatible with version 3 of
  the GNU GPL"** (explicitly *not* compatible with GPLv2, which is
  irrelevant here since this project selected GPLv3-or-later, never
  GPLv2). This directly confirms `grpc-go` and the vendored YouTube
  material can be combined into this GPLv3-or-later-covered project
  while keeping their own Apache-2.0 notices intact.
- **MediaMTX and FFmpeg remain separately licensed and are not claimed as
  GPL.** Both run as separate child processes (MediaMTX via a loopback
  Control API, FFmpeg via `exec.CommandContext` and stdout/stderr only),
  neither is linked into the Streaming Tree binary, and neither is
  redistributed by this repository - ordinary "mere aggregation" of
  independent programs, not a combined/derivative work, so the chosen
  GPL licence does not relicense them and their own notices in
  `THIRD_PARTY_NOTICES.md` remain authoritative.

No incompatibility was found. Nothing was papered over.

### 5.3 Primary sources consulted (2026-08-18)

- GNU/FSF's own `https://www.gnu.org/licenses/gpl-3.0.txt` - the exact,
  complete, verbatim GPLv3 text now at `LICENSE` (674 lines, byte-for-byte
  identical to the fetched copy - diffed directly, not merely visually
  compared).
- SPDX's own `https://spdx.org/licenses/GPL-3.0-or-later.html` - confirmed
  `GPL-3.0-or-later` as the correct, current, non-deprecated identifier
  for "GPL v3.0 or later," distinct from `GPL-3.0-only`.
- FSF's `https://www.gnu.org/licenses/license-list.html#apache2` -
  Apache-2.0/GPLv3 compatibility (§5.2 above).
- FSF's `https://www.gnu.org/licenses/why-affero-gpl.html` - the GPL/AGPL
  network-use distinction (§5.2 above).

## 6. Version / build identity

No release-version injection pipeline exists yet (`apps/server/internal/
buildinfo`'s own `Version = "0.1.0"` is a hand-maintained internal
identifier, not a real semver release, and the Vite frontend build has no
equivalent commit/version stamping). Per this milestone's own governing
instructions, the About page does not present `0.1.0` as if it were a real
release - `buildinfo.IsReleaseBuild` is `false` today, and the UI shows an
honest "Development build" identity while that remains the case.

Go's own automatic VCS build-info stamping (`runtime/debug.ReadBuildInfo`,
available for any `go build`/`go install` run inside a git checkout, no
`-ldflags` setup required) reliably provides a real commit hash and a
"working tree had uncommitted changes at build time" flag - this is real,
verified backend-only information the frontend cannot determine on its
own, so `GET /api/about` exposes it (`commit`, `commitDirty`) as a
secondary detail alongside the "Development build" label. Stage 20A will
separately establish full release-version injection; this milestone only
prepares/reuses the `buildinfo` abstraction and does not begin packaging.

## 7. Documents this contract governs

- `PRIVACY.md` - what data lives where, and what network activity is
  provider-initiated versus explicitly triggered by the user.
- `LEGAL.md` - concise product identity, licence status, third-party
  notices pointer, independent-project disclaimer, third-party service
  availability, user responsibility, and creator-support terms. Not a
  Terms of Service document (see §8 of the governing task for this
  milestone) and not represented as reviewed by a lawyer.
- The in-app **About & Legal** surface (`Settings → About & Legal`,
  `apps/web/src/pages/AboutLegalPage.tsx`), which summarizes both of the
  above and links to them rather than duplicating their full text.

## 8. Stage 20A distribution boundary - implemented (2026-08-18)

At the time this section was first written, it recorded a future
requirement ahead of implementation: "every future public packaged
distribution must include, at minimum, `LICENSE`, `THIRD_PARTY_NOTICES.md`,
and any additional required notice." **Stage 20A has since implemented
this for real** - see [windows-packaging.md](windows-packaging.md) §16 for
the complete architecture. In summary: `LICENSE`, `THIRD_PARTY_NOTICES.md`,
`LEGAL.md`, and `PRIVACY.md` are all embedded into the single release
executable (`internal/webassets`) and served through a fixed, closed
route allowlist (`/legal/license`, `/legal/privacy`, `/legal/legal`,
`/legal/third-party-notices`), plus staged as loose files beside the
executable by the installer - so the installed application can show its
own licence fully offline, satisfying this requirement independent of
GitHub/internet access.

The Windows installer's own `AppPublisher`/`AppPublisherURL` metadata
(Inno Setup `[Setup]` section) reuses exactly the same `Czekosabe`/
`https://github.com/Czekosabe` identity established in §1 above - never a
second, independently-maintained copy.

Stage 20A does not publish a public GitHub Release; the corresponding-
source obligation for any binary actually distributed by a future public
release remains the later release pipeline's own responsibility (already
noted in §5.2 above) - already satisfied today for any locally-built
binary, since this project's complete source is already public at the
canonical repository.

## 9. Contribution policy - audit result, not a blocker

No `CONTRIBUTING.md`, CLA, or DCO exists in this repository. The
ownership audit in §5.2 found exactly one real contributor across the
entire Git history, so there is no current governance gap this milestone
must close. This is recorded as a **future project-governance
consideration** for whenever external contributions become a real
possibility, not a blocker for the GPL licence decision - introducing a
CLA or copyright-assignment agreement is explicitly not warranted merely
because a licence was selected. If a contribution policy is ever added,
the expected minimal rule is that contributions are expected to be
compatible with the project's `GPL-3.0-or-later` licence and to be the
contributor's own work or otherwise legally redistributable, identified
by their normal Git identity - no legal name demand, no copyright
assignment.
