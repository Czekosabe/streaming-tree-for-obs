import { PlatformGlyph } from '@/components/platforms/PlatformGlyph';
import { cn } from '@/lib/cn';
import { providerGlyphClass } from '@/models/provider-labels';

type ProviderMarkSize = 'sm' | 'md' | 'lg' | 'xl';

type ProviderMark = {
  /** Path data copied byte-for-byte from the vendored, unmodified upstream
   * `.svg` file under `apps/web/src/assets/providers/` - those files remain
   * the authoritative provenance record (docs/provider-branding.md);
   * `ProviderBrand.test.tsx` asserts this string matches that file's own
   * `<path d="...">` exactly, so the two can never silently drift apart. */
  path: string;
  /** Official (or, for TikTok, brand-accent) colour the mark is rendered in. */
  hex: string;
  title: string;
};

/**
 * Real streaming-destination brand marks, rendered as inline SVG - not a
 * CSS `mask-image` referencing an external/inlined asset URL. An earlier
 * version of this component used a CSS mask; that adds a real dependency on
 * how the bundler resolves and serves (or inlines) the asset URL, which is
 * exactly the kind of packaging-specific failure mode a plain unit test
 * cannot catch (see `verify-packaged-app.mjs`'s provider-asset check, which
 * exercises the real packaged build instead). Inline SVG has no such
 * dependency: the path geometry is part of the component itself, so
 * "does this render" reduces to "does React render", the same guarantee
 * every other element on the page already has.
 *
 * Path data vendored directly from the Simple Icons project (CC0 markup
 * licence, official-source geometry) - see docs/provider-branding.md for
 * full provenance, per-icon source links, and the one real usage limitation
 * (TikTok). Third-party marks remain the property of their respective
 * owners.
 */
const PROVIDER_MARKS: Record<string, ProviderMark> = {
  twitch: {
    title: 'Twitch',
    hex: '#9146FF',
    path: 'M11.571 4.714h1.715v5.143H11.57zm4.715 0H18v5.143h-1.714zM6 0L1.714 4.286v15.428h5.143V24l4.286-4.286h3.428L22.286 12V0zm14.571 11.143l-3.428 3.428h-3.429l-3 3v-3H6.857V1.714h13.714Z',
  },
  youtube: {
    title: 'YouTube',
    hex: '#FF0000',
    path: 'M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z',
  },
  kick: {
    title: 'Kick',
    hex: '#53FC19',
    path: 'M1.333 0h8v5.333H12V2.667h2.667V0h8v8H20v2.667h-2.667v2.666H20V16h2.667v8h-8v-2.667H12v-2.666H9.333V24h-8Z',
  },
  // TikTok's only official single-colour mark is near-black (#000000),
  // which has no usable contrast against this application's dark theme on
  // ANY accent tile - a real accessibility problem, not a stylistic
  // choice. Rendered in a light neutral tone instead: an accepted
  // light/dark polarity of the same unaltered geometry (TikTok's own real-
  // world usage commonly presents this exact mark in white for dark
  // surfaces), never a hue recolour. The tile it sits on
  // (providerGlyphClass, not the glyph itself) carries TikTok's cyan/pink
  // brand accent. See docs/provider-branding.md §3.
  tiktok: {
    title: 'TikTok',
    hex: '#f4f6fb',
    path: 'M12.525.02c1.31-.02 2.61-.01 3.91-.02.08 1.53.63 3.09 1.75 4.17 1.12 1.11 2.7 1.62 4.24 1.79v4.03c-1.44-.05-2.89-.35-4.2-.97-.57-.26-1.1-.59-1.62-.93-.01 2.92.01 5.84-.02 8.75-.08 1.4-.54 2.79-1.35 3.94-1.31 1.92-3.58 3.17-5.91 3.21-1.43.08-2.86-.31-4.08-1.03-2.02-1.19-3.44-3.37-3.65-5.71-.02-.5-.03-1-.01-1.49.18-1.9 1.12-3.72 2.58-4.96 1.66-1.44 3.98-2.13 6.15-1.72.02 1.48-.04 2.96-.04 4.44-.99-.32-2.15-.23-3.02.37-.63.41-1.11 1.04-1.36 1.75-.21.51-.15 1.07-.14 1.61.24 1.64 1.82 3.02 3.5 2.87 1.12-.01 2.19-.66 2.77-1.61.19-.33.4-.67.41-1.06.1-1.79.06-3.57.07-5.36.01-4.03-.01-8.05.02-12.07z',
  },
};

const TILE_SIZE: Record<ProviderMarkSize, string> = {
  sm: 'size-7 rounded-md',
  md: 'size-9 rounded-lg',
  lg: 'size-11 rounded-lg',
  xl: 'size-14 rounded-xl',
};

const GLYPH_SIZE: Record<ProviderMarkSize, string> = {
  sm: 'size-3.5',
  md: 'size-4',
  lg: 'size-5',
  xl: 'size-7',
};

type ProviderBrandProps = {
  providerId: string;
  /** Short text shown only for a provider id this build does not
   * recognise - matches the existing neutral-tile fallback behaviour. */
  fallbackLabel: string;
  size?: ProviderMarkSize;
  className?: string;
  /** Renders only the glyph - no tile background/border. Used for a large,
   * low-opacity brand watermark inside a card's decorative header area,
   * where a bordered tile would be the wrong visual weight. Has no effect
   * on the neutral-fallback path, which always needs its tile to remain
   * legible text. */
  bare?: boolean;
};

/**
 * One reusable provider-identity tile.
 *
 * Resolves a known provider id (twitch/youtube/kick/tiktok) to its real
 * brand mark - real SVG geometry, rendered inline, filled with the
 * official (or, for TikTok, contrast-corrected) colour directly as an SVG
 * attribute - on a provider-accented rounded tile. Any other id, including
 * a future provider this build has not shipped support for yet, falls back
 * to the existing neutral text tile (`PlatformGlyph`) - never a crash,
 * never a blank tile, never an unstyled/uncoloured square.
 *
 * The mark is always `aria-hidden`: the visible provider/brand name text
 * every call site already renders next to it remains the one accessible
 * identifier, consistent with the rest of this codebase's status/identity
 * badges.
 */
export function ProviderBrand({
  providerId,
  fallbackLabel,
  size = 'md',
  className,
  bare = false,
}: ProviderBrandProps) {
  const mark = PROVIDER_MARKS[providerId];

  if (mark === undefined) {
    const tileClass = cn(
      'flex shrink-0 items-center justify-center border',
      TILE_SIZE[size],
      providerGlyphClass(providerId),
      className,
    );
    return <PlatformGlyph label={fallbackLabel} className={tileClass} />;
  }

  const glyph = (
    <svg
      viewBox="0 0 24 24"
      className={cn('block', GLYPH_SIZE[size], bare && className)}
      aria-hidden="true"
      focusable="false"
    >
      <path d={mark.path} fill={mark.hex} />
    </svg>
  );

  if (bare) return glyph;

  const tileClass = cn(
    'flex shrink-0 items-center justify-center border',
    TILE_SIZE[size],
    providerGlyphClass(providerId),
    className,
  );

  return (
    <span aria-hidden="true" className={tileClass}>
      {glyph}
    </span>
  );
}

/** Provider ids this build has a real vendored brand mark for. Exported so
 * tests can assert every mark resolves without reaching into the private map. */
export const KNOWN_PROVIDER_BRAND_IDS = Object.keys(PROVIDER_MARKS);

/** Exported for `ProviderBrand.test.tsx`'s vendored-file sync check only. */
export const PROVIDER_MARK_PATHS: Record<string, string> = Object.fromEntries(
  Object.entries(PROVIDER_MARKS).map(([id, mark]) => [id, mark.path]),
);
