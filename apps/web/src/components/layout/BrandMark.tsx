import brandEmblem from '@/assets/brand-emblem.png';
import { cn } from '@/lib/cn';

/**
 * The application's own brand mark: the real logo's emblem (cropped
 * from the operator-provided canonical source, never redrawn - see
 * scripts/generate-branding-assets.go) alongside the existing textual
 * lockup. No third-party logo or artwork is used anywhere in the
 * application - platforms are represented by short text labels
 * instead; this is Streaming Tree's own first-party identity.
 *
 * Only the emblem is used here, not the source's full wordmark image:
 * at this ~32px display size the wordmark text would be illegible, so
 * the existing "Streaming Tree / for OBS" text lockup below carries
 * the name instead, exactly like it already did before this asset
 * existed.
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-2.5', className)}>
      <img
        src={brandEmblem}
        alt=""
        className="size-8 shrink-0 rounded-lg object-contain"
      />
      <span className="min-w-0 leading-tight">
        <span className="block truncate text-sm font-semibold tracking-tight text-ink">
          Streaming Tree
        </span>
        <span className="block truncate text-[10px] font-medium uppercase tracking-widest text-ink-faint">
          for OBS
        </span>
      </span>
    </div>
  );
}
