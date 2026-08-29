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
themselves drawn from official material. **None of the four is an asset
downloaded directly from the provider's own site by this project** - every
one is the Simple Icons fallback, not an "official asset" in the sense of
having been fetched from `brand.twitch.tv`/`about.kick.com`/etc. by this
project itself. That distinction is recorded explicitly per provider below
so it is never overstated later.

**Upstream package:** `simple-icons` npm package, version `16.28.0`,
retrieved 2026-08-28 (`npm view`/`npm install simple-icons@16.28.0` into a
scratch directory - not added to this repository's own `package.json`).
**Licence of Simple Icons' own contribution:** CC0-1.0 - see the package's
own `LICENSE.md`. CC0 covers Simple Icons' standardized SVG markup/geometry
extraction only; it is **not** a grant of trademark rights to the
underlying marks - see `DISCLAIMER.md`, quoted in full at the bottom of
this document. **Retrieval method for all four:** the `.svg` file under
Simple Icons' own `icons/` directory was copied byte-for-byte into this
repository, with no redrawing, no recolouring, and no path editing; only
its `<title>` element was left intact. No file was resized, repackaged, or
otherwise altered beyond that direct copy.

### Twitch
- **Local file:** `apps/web/src/assets/providers/twitch.svg`
- **Upstream source:** Simple Icons `twitch` icon, `simple-icons@16.28.0`
- **Official asset or fallback:** Simple Icons fallback (not fetched from
  Twitch's own brand portal by this project)
- **Retrieved:** 2026-08-28
- **Official brand-guideline source:** <https://brand.twitch.tv> (also
  Simple Icons' own recorded `source`/`guidelines` field for this icon)
- **Modified beyond the copy described above:** No
- **Colour/treatment rule applied:** Rendered as an inline `<svg><path>`
  (see §4) with its `fill` set to Twitch's official hex `#9146FF`; no other
  recolouring or distortion

### YouTube
- **Local file:** `apps/web/src/assets/providers/youtube.svg`
- **Upstream source:** Simple Icons `youtube` icon, `simple-icons@16.28.0`
- **Official asset or fallback:** Simple Icons fallback
- **Retrieved:** 2026-08-28
- **Official brand-guideline source:**
  <https://www.youtube.com/howyoutubeworks/resources/brand-resources/#logos-icons-and-colors>
  (Simple Icons' own recorded `source`/`guidelines` field for this icon)
- **Modified beyond the copy described above:** No
- **Colour/treatment rule applied:** Rendered as an inline `<svg><path>`
  (see §4) with its `fill` set to YouTube's official hex `#FF0000`; no
  other recolouring or distortion

### Kick
- **Local file:** `apps/web/src/assets/providers/kick.svg`
- **Upstream source:** Simple Icons `kick` icon, `simple-icons@16.28.0`
- **Official asset or fallback:** Simple Icons fallback
- **Retrieved:** 2026-08-28
- **Official brand-guideline source:** <https://about.kick.com/brand>
  (Kick's own official brand toolkit, confirmed to exist via search on
  2026-08-28; Simple Icons' own recorded `source` field for this icon is
  the bare `https://kick.com` domain, not the brand-toolkit subpage - this
  project did not independently re-verify Simple Icons' geometry against
  Kick's own toolkit download, since doing so would require the binary
  download/verification capability this session does not have; recorded
  here as an honest limitation, not glossed over)
- **Modified beyond the copy described above:** No
- **Colour/treatment rule applied:** Rendered as an inline `<svg><path>`
  (see §4) with its `fill` set to Kick's official brand green `#53FC19`;
  no other recolouring or distortion

### TikTok
- **Local file:** `apps/web/src/assets/providers/tiktok.svg`
- **Upstream source:** Simple Icons `tiktok` icon, `simple-icons@16.28.0`
- **Official asset or fallback:** Simple Icons fallback
- **Retrieved:** 2026-08-28
- **Official brand-guideline source:** <https://developers.tiktok.com>
  (TikTok's own developer brand-and-use guidance); Simple Icons' own
  recorded `source` field for this icon is the bare `https://tiktok.com`
  domain
- **Modified beyond the copy described above:** No - the glyph geometry
  itself is untouched
- **Colour/treatment rule applied:** TikTok's only official single-colour
  mark is near-black (`#000000`). Rendered against this application's dark
  theme, that colour has no usable contrast on any accent tile - a real
  accessibility problem, not only an aesthetic one. The glyph is instead
  rendered, as an inline `<svg><path fill="#f4f6fb">` (see §4), in a light
  neutral tone, the accepted light/dark polarity of the same unaltered
  geometry (TikTok's own real-world usage commonly presents this exact
  mark in white for dark surfaces); this is not a hue recolour. The
  surrounding **tile background** (not the glyph) separately carries
  TikTok's cyan/pink brand accent (`#25F4EE`/`#FE2C55`) as a decorative
  gradient. See §3 below for why this split treatment exists.

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

- The TikTok glyph's geometry is rendered exactly as Simple Icons ships
  it - undistorted, unstretched, no path edited. Its officially-documented
  single colour is near-black (`#000000`); against this application's dark
  theme that colour is not legible on any tile background this app uses,
  so the glyph is instead rendered in a light neutral tone (`#f4f6fb`) - an
  accepted light/dark polarity of the same unaltered mark, not a hue
  recolour into an unofficial colour. This was caught and fixed during
  this stage's own accessibility/contrast re-verification pass, not
  assumed correct from the start.
- The **tile background** behind it (not the glyph geometry) separately
  uses TikTok's well-known brand accent colours (cyan `#25F4EE` / pink
  `#FE2C55`) as a decorative gradient, so the destination is still
  visually recognizable and colourful in the card grid.
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
`size` scale (`sm`/`md`/`lg`/`xl`) is the only per-call variation, plus a
`bare` flag that renders just the glyph with no surrounding tile (used for
a large, low-opacity brand watermark inside a destination card's header
band).

**Rendering technique - inline SVG, not a CSS mask.** An earlier version
of this component rendered the mark as a `mask-image: url(...)` CSS
property referencing the vendored asset via the bundler's own resolved
URL. A physical Windows finding reported destination cards showing flat
coloured squares instead of recognizable marks; while the exact cause in
that specific packaged build was not conclusively isolated, a CSS mask
adds a real dependency on how the bundler/server resolves and serves (or
inlines) that asset URL in the packaged build - a class of failure a
component-level unit test cannot see, since it never exercises the real
build/packaging pipeline. The component now hardcodes each mark's exact
`<path d="...">` data (copied byte-for-byte from the vendored `.svg`
files, verified equal to them by `ProviderBrand.test.tsx`'s own sync
check) and renders it as a real inline `<svg><path fill="...">` element.
This removes the dependency entirely: there is no URL to resolve, no
external resource to fetch, and "does this render" reduces to "does React
render," the same guarantee every other element on the page already has.
`scripts/verify-packaged-app.mjs` additionally fetches the real
production JS bundle from the real running packaged server and confirms
each provider's official/contrast-corrected hex literal is present in
it - proof against the actual shipped artifact, not only a unit test.

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
