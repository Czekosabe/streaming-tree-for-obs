import { PlatformGlyph } from '@/components/platforms/PlatformGlyph';

/**
 * Provider-independent source marker: an app-owned text glyph, never a
 * third-party brand logo (see PlatformGlyph's own doc comment) plus an
 * optional account label - both purely presentational toggles the public
 * config exposes (`showPlatformIcon`/`showPlatformName`), independent of
 * whether the item itself carries an `accountLabel` at all (only
 * populated server-side when the owning profile's own
 * `showAccountLabel` setting is on).
 */
const PROVIDER_GLYPHS: Record<string, { label: string; className: string }> = {
  twitch: { label: 'TW', className: 'border-[#9146FF]/50 bg-[#9146FF]/15 text-[#c9a4ff]' },
};

export function OverlaySourceMarker({
  providerId,
  accountLabel,
  showIcon,
  showName,
}: {
  providerId: string;
  accountLabel: string | undefined;
  showIcon: boolean;
  showName: boolean;
}) {
  if (!showIcon && !showName) return null;
  const glyph = PROVIDER_GLYPHS[providerId] ?? { label: providerId.slice(0, 2).toUpperCase(), className: '' };

  return (
    <span className="inline-flex shrink-0 items-center gap-1 align-middle">
      {showIcon && (
        <PlatformGlyph
          label={glyph.label}
          className={`size-[1.4em] rounded text-[0.55em] ${glyph.className}`}
        />
      )}
      {showName && <span className="text-[0.75em] opacity-80">{providerId}</span>}
      {accountLabel !== undefined && accountLabel !== '' && (
        <span className="text-[0.75em] opacity-70">({accountLabel})</span>
      )}
    </span>
  );
}
