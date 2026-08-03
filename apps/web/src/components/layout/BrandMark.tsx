import { Network } from 'lucide-react';

import { cn } from '@/lib/cn';

/**
 * Text-only brand mark. No third-party logo or artwork is used anywhere in the
 * application - platforms are represented by short text labels instead.
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-2.5', className)}>
      <span
        aria-hidden="true"
        className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-linear-to-br from-accent to-accent-deep text-white shadow-raised"
      >
        <Network className="size-4" />
      </span>
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
