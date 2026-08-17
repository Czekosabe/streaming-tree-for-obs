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
`ProductName`, `CreatorName`, `RepositoryURL`, `CreatorURL`, and
`SupportURL` are defined as Go constants. `GET /api/about`
(`internal/httpapi/about.go`) is the one HTTP contract that exposes them.
The frontend's About & Legal page (`apps/web/src/pages/AboutLegalPage.tsx`)
fetches that endpoint rather than keeping a second, independently
maintained copy of these strings - changing the support URL, for example,
means editing exactly one Go constant, matching §16 of the governing task
for this milestone (no database migration, no saved user configuration, no
per-component literal to hunt down).

The pre-existing frontend `apps/web/src/data/app-info.ts` (`APP_INFO.name`/
`APP_INFO.version`) is a narrower, unrelated concern - the sidebar footer's
own local chrome label - and is left untouched by this milestone.

## 3. Creator support - what it is and is not

- Support is entirely **voluntary**.
- Support **does not unlock any application feature**. Every feature in
  Streaming Tree for OBS is available identically whether or not a user
  ever visits the support link.
- Support **is not the purchase of an application licence**. It has no
  bearing on §5's separate, still-unresolved licence question.
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

## 5. Application licence - audit result

**No application-wide licence file exists in this repository as of this
milestone** (no root `LICENSE`, `LICENSE.*`, or `COPYING` file - confirmed
by direct directory listing; only `THIRD_PARTY_NOTICES.md` and `README.md`
exist at the repository root alongside `apps/`, `config/`, `docs/`, and
`scripts/`). Neither `apps/web/package.json` nor `apps/server/go.mod`
declares a licence field.

The only licence-related text anywhere in this repository concerns
**MediaMTX**, a bundled third-party dependency under its own MIT licence -
this is explicitly *not* Streaming Tree for OBS's own licence and must
never be read as one (README.md's own MediaMTX packaging section already
makes this distinction; see also `THIRD_PARTY_NOTICES.md`).

**This milestone does not select, invent, or guess an application
licence.** No MIT/Apache-2.0/GPL/AGPL/other licence is added to this
repository by this milestone. This is an explicit, deliberately unresolved
**operator decision required before the first public packaged release**
(Stage 20A). `GET /api/about`'s `applicationLicenceStatus` field carries
the stable status code `"unselected"` for exactly this reason, and the
About & Legal UI displays that state honestly rather than hiding it.

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
