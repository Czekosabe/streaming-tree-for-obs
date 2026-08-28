# Provider brand assets - sourcing and provenance (Stage 20E)

**Written:** 2026-08-28. Covers the four streaming-destination provider
marks shown on the operator Dashboard, Add Platform, destination settings,
metadata tabs, and the Streams page: Twitch, YouTube, Kick, TikTok.

Third-party brand names and logos referenced here remain the trademarks of
their respective owners (Twitch Interactive, Inc.; Google LLC / YouTube;
Kick Streaming Pty Ltd; TikTok Pty Ltd). Nothing in this document or in the
application implies sponsorship, endorsement, or affiliation. Every mark is
used solely to identify, within the operator's own dashboard, which
platform a destination the operator configured actually connects to -
nominative use, never decoration, never marketing.

## 1. Why vendored SVGs instead of a runtime dependency or a CDN

- No logo is ever fetched at runtime. The production build is fully
  self-contained and works offline, matching every other asset in this
  project (see `BrandMark`'s own emblem, MediaMTX's locally-installed
  binary, etc.).
- Rather than adding `simple-icons` as an npm dependency (its unpacked
  package is ~16 MB across 3,400+ per-brand SVG files, of which this
  project needs exactly four), the four needed `.svg` files were copied
  directly into this repository under
  `apps/web/src/assets/providers/`. This keeps the production bundle
  from ever risking pulling in the full catalog and avoids a large
  devDependency for four static files. This is the "vendor the SVG
  directly" path the governing task explicitly allowed as an alternative
  to a package dependency.

## 2. Source and provenance, per mark

All four were retrieved from the Simple Icons project - an open-source,
standardized-geometry icon set used here as the explicit "fallback" source
named in the governing task, because reliably extracting an official
binary/vector asset from each brand's own portal through this session's
available tooling (text-oriented web fetch, no binary download/verification
step) was not something this session could do to the same
provenance/integrity standard as a pinned, versioned, checksummable
package. Simple Icons' own per-icon `source` field points at each brand's
official brand/press page, confirming the geometry and colour were
themselves drawn from official material.

| Provider | Local file | Simple Icons slug | Official hex | Simple Icons `source` | Brand guidelines |
|---|---|---|---|---|---|
| Twitch | `apps/web/src/assets/providers/twitch.svg` | `twitch` | `#9146FF` | <https://brand.twitch.tv> | <https://brand.twitch.tv> |
| YouTube | `apps/web/src/assets/providers/youtube.svg` | `youtube` | `#FF0000` | <https://www.youtube.com/howyoutubeworks/resources/brand-resources/#logos-icons-and-colors> | same |
| Kick | `apps/web/src/assets/providers/kick.svg` | `kick` | `#53FC19` | <https://kick.com> | <https://about.kick.com/brand> (official Kick brand toolkit, confirmed to exist via search on 2026-08-28; not independently re-verified against Simple Icons' geometry) |
| TikTok | `apps/web/src/assets/providers/tiktok.svg` | `tiktok` | `#000000` (official single-colour mark) | <https://tiktok.com> | <https://developers.tiktok.com> |

- **Upstream package:** `simple-icons` npm package, version `16.28.0`
  (retrieved 2026-08-28).
- **License:** CC0-1.0 (the icon *dataset/markup* - see the package's own
  `LICENSE.md`). CC0 covers Simple Icons' own contribution (standardized
  SVG markup/geometry extraction); it is **not** a grant of trademark
  rights to the underlying marks - see `DISCLAIMER.md`, quoted in full at
  the bottom of this document.
- **Retrieval method:** `npm view`/`npm install simple-icons@16.28.0` into
  a scratch directory, then the four `.svg` files under its `icons/`
  directory were copied verbatim (no redrawing, no recolouring, no path
  editing) into this repository. Their `<title>` elements were left
  intact.

## 3. TikTok - the one real usage limitation

TikTok's own brand policy is materially stricter than the other three
("You may not use TikTok logos, icons, symbols, or designs without prior
written permission" - developers.tiktok.com guidance, confirmed via search
on 2026-08-28) and its full multicolour treatment is a layered
cyan/red-pink/white composition that Simple Icons does not provide as a
multicolour asset - only the single official black glyph.

This project's use is nominative (identifying, inside an operator's own
local dashboard, which of their own configured destinations is a TikTok
destination - directly analogous to how every third-party multistreaming
tool shows destination platform marks), not merchandising or marketing use.
To stay honest about the limitation rather than silently redrawing the
mark in unofficial colours:

- The TikTok glyph itself is rendered exactly as Simple Icons ships it -
  the official single-colour (black) mark, undistorted, unrecoloured.
- The **tile background** behind it (not the glyph geometry) uses TikTok's
  well-known brand accent colours (cyan `#25F4EE` / pink `#FE2C55`) as a
  decorative gradient, so the destination is still visually recognizable
  and colourful in the card grid, without altering the trademarked glyph
  itself.
- If TikTok's terms are ever found to prohibit even this nominative,
  non-commercial identification use inside a local operator tool, the
  fallback is the same neutral text-tile treatment (`PlatformGlyph`) every
  unrecognized provider already gets - not a redrawn or invented logo.

## 4. Component and reuse

`apps/web/src/components/providers/ProviderBrand.tsx` is the one reusable
component: given a `providerId`, it renders a rounded tile with the real
vendored mark (decorative, `aria-hidden`, since the visible provider name
text next to it is the accessible identifier - matching every other
provider-identity surface in this codebase) and a provider-specific accent
background, or falls back to the pre-existing neutral text tile
(`PlatformGlyph`) for any provider id it does not recognize. It never
stretches, skews, or recolours the vendored glyph geometry itself. A small
`size` scale (`sm`/`md`/`lg`) is the only per-call variation.

Adopted on: `PlatformCard` (Dashboard grid), `PlatformSettingsDialog`
(provider identity row), `PlatformTabs` (Metadata workspace tab strip), and
`StreamsPage`'s destination branch table. The Add Platform provider
`<select>` cannot render an icon inside a native `<option>`, so it keeps its
existing text-only options.

**Deliberately not adopted:** `ChatSourceLabel` and `OverlaySourceMarker`
(the public, viewer-facing chat/browser-source overlay glyphs). Their own
doc comments predate this stage and record a deliberate choice to never
show a third-party brand logo inside content rendered live inside a
stream/OBS browser source. That is a distinct product surface from the
operator's own Dashboard and was left exactly as it was.

## 5. Bundle-size check

Each vendored SVG is a few hundred bytes of path data (see §2's file list);
four files add well under 5 KB total to the production bundle - no
meaningful size delta, and nowhere near the ~16 MB the full `simple-icons`
package would represent if added as a dependency and not properly
tree-shaken. No devDependency was added to `package.json` for this.

## 6. Simple Icons DISCLAIMER.md (quoted in full, retrieved 2026-08-28)

> Simple Icons asks that its users read this disclaimer fully before
> including an icon in their project.
>
> ## Licenses, Copyrights & Trademarks
>
> Simple Icons is released under CC0 - though that doesn't mean to imply
> that all icons within the project are also CC0. Please see individual
> licenses where available.
>
> Simple Icons provides data on the license under which icons are
> available. We ask users to carefully consider this when using an icon.
>
> If an icon includes a registered trademark (®) or trademark symbol (™)
> the recommendations outlined in the Simple Icons Contributing
> Guidelines are followed to decide whether to include the symbol or not.
>
> Simple Icons cannot be held responsible for any legal activity raised by
> a brand, or users of the package. We ask that our users seek the correct
> permissions to use the icons relevant to their project.
>
> ## Brand Guidelines
>
> Simple Icons provides a link to a brand's branding guidelines (or
> similar) if the brand provides one. We ask our users read these
> guidelines and ensure their usage of the brand's icon is in accordance
> with them.

Full text: <https://github.com/simple-icons/simple-icons/blob/master/DISCLAIMER.md>
(package version 16.28.0, retrieved 2026-08-28).
