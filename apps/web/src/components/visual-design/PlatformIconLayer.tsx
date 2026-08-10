import { providerGlyphClass } from '@/models/provider-labels';

/** Uses only the application-owned provider glyph mapping - never an
 * arbitrary icon URL, never a copyrighted platform logo asset (Stage
 * 13A task Part 47). */
export function PlatformIconLayer({ providerId }: { providerId: string }) {
  return (
    <div
      className={`flex h-full w-full items-center justify-center rounded ${providerGlyphClass(providerId)}`}
      data-testid="visual-design-platform-icon"
    >
      <span className="text-[0.6em] font-semibold uppercase">{providerId.slice(0, 2)}</span>
    </div>
  );
}
