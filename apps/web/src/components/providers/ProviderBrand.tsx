import kickIcon from '@/assets/providers/kick.svg';
import tiktokIcon from '@/assets/providers/tiktok.svg';
import twitchIcon from '@/assets/providers/twitch.svg';
import youtubeIcon from '@/assets/providers/youtube.svg';
import { PlatformGlyph } from '@/components/platforms/PlatformGlyph';
import { cn } from '@/lib/cn';
import { providerGlyphClass } from '@/models/provider-labels';

type ProviderMarkSize = 'sm' | 'md' | 'lg';

type ProviderMark = {
  /** Vendored, unmodified upstream SVG - see docs/provider-branding.md. */
  icon: string;
  /** Official (or, for TikTok, brand-accent) colour the mark is rendered in. */
  hex: string;
};

/**
 * Real streaming-destination brand marks.
 *
 * Vendored directly from the Simple Icons project (CC0 markup licence,
 * official-source geometry) rather than added as an npm dependency, so the
 * production bundle never risks pulling in more than these four files -
 * see docs/provider-branding.md for full provenance, per-icon source
 * links, and the one real usage limitation (TikTok). Third-party marks
 * remain the property of their respective owners.
 */
const PROVIDER_MARKS: Record<string, ProviderMark> = {
  twitch: { icon: twitchIcon, hex: '#9146FF' },
  youtube: { icon: youtubeIcon, hex: '#FF0000' },
  kick: { icon: kickIcon, hex: '#53FC19' },
  // TikTok's own official single-colour mark is black; the tile it sits on
  // (providerGlyphClass, not the glyph itself) carries TikTok's cyan/pink
  // brand accent instead - the glyph geometry is never recoloured. See
  // docs/provider-branding.md §3.
  tiktok: { icon: tiktokIcon, hex: '#0a0a0a' },
};

const TILE_SIZE: Record<ProviderMarkSize, string> = {
  sm: 'size-7 rounded-md',
  md: 'size-9 rounded-lg',
  lg: 'size-11 rounded-lg',
};

const GLYPH_SIZE: Record<ProviderMarkSize, string> = {
  sm: 'size-3.5',
  md: 'size-4',
  lg: 'size-5',
};

type ProviderBrandProps = {
  providerId: string;
  /** Short text shown only for a provider id this build does not
   * recognise - matches the existing neutral-tile fallback behaviour. */
  fallbackLabel: string;
  size?: ProviderMarkSize;
  className?: string;
};

/**
 * One reusable provider-identity tile.
 *
 * Resolves a known provider id (twitch/youtube/kick/tiktok) to its real
 * brand mark on a provider-accented rounded tile, rendered as a CSS
 * mask so the vendored SVG file itself is never parsed as markup and
 * never mutated - only referenced as an image resource, coloured by the
 * mask's `background-color`. Any other id, including a future provider
 * this build has not shipped support for yet, falls back to the existing
 * neutral text tile (`PlatformGlyph`) - never a crash, never a blank tile.
 *
 * The mark is always `aria-hidden`: the visible provider/brand name text
 * every call site already renders next to it remains the one accessible
 * identifier, consistent with the rest of this codebase's status/identity
 * badges.
 */
export function ProviderBrand({ providerId, fallbackLabel, size = 'md', className }: ProviderBrandProps) {
  const mark = PROVIDER_MARKS[providerId];
  const tileClass = cn(
    'flex shrink-0 items-center justify-center border',
    TILE_SIZE[size],
    providerGlyphClass(providerId),
    className,
  );

  if (mark === undefined) {
    return <PlatformGlyph label={fallbackLabel} className={tileClass} />;
  }

  return (
    <span aria-hidden="true" className={tileClass}>
      <span
        className={cn('block', GLYPH_SIZE[size])}
        style={{
          backgroundColor: mark.hex,
          WebkitMaskImage: `url(${mark.icon})`,
          maskImage: `url(${mark.icon})`,
          WebkitMaskRepeat: 'no-repeat',
          maskRepeat: 'no-repeat',
          WebkitMaskPosition: 'center',
          maskPosition: 'center',
          WebkitMaskSize: 'contain',
          maskSize: 'contain',
        }}
      />
    </span>
  );
}

/** Provider ids this build has a real vendored brand mark for. Exported so
 * tests can assert every mark resolves without reaching into the private map. */
export const KNOWN_PROVIDER_BRAND_IDS = Object.keys(PROVIDER_MARKS);
