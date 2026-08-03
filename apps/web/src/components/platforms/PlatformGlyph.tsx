import { cn } from '@/lib/cn';
import type { PlatformId } from '@/models/platform';

/**
 * Platform marker.
 *
 * Deliberately a coloured text badge rather than a brand logo: the project does
 * not ship third-party trademarks. The hues only differentiate the cards and
 * carry no status meaning.
 */
const GLYPH_CLASSES: Record<PlatformId, string> = {
  twitch: 'border-violet-500/35 bg-violet-500/12 text-violet-300',
  youtube: 'border-red-500/35 bg-red-500/12 text-red-300',
  kick: 'border-emerald-500/35 bg-emerald-500/12 text-emerald-300',
  tiktok: 'border-sky-500/35 bg-sky-500/12 text-sky-300',
};

export function PlatformGlyph({
  id,
  label,
  className,
}: {
  id: PlatformId;
  label: string;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex size-9 shrink-0 items-center justify-center rounded-lg border',
        'text-xs font-bold tracking-wide',
        GLYPH_CLASSES[id],
        className,
      )}
    >
      {label}
    </span>
  );
}
