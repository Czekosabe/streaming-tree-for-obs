import { useEffect, useState } from 'react';

import type { PublicWidgetSnapshot } from '@/api/goals-schemas';
import { cn } from '@/lib/cn';
import { formatAmountMicros } from '@/models/alerts';

function supportsMatchMedia(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function';
}

/** Tracks the `prefers-reduced-motion` media query - mirrors
 * components/alerts/AlertRenderer.tsx's own identical hook. */
function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(
    () => supportsMatchMedia() && window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  );
  useEffect(() => {
    if (!supportsMatchMedia()) return;
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = () => setReduced(query.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);
  return reduced;
}

function formatValue(kind: PublicWidgetSnapshot['goalKind'], value: number, currency: string | undefined): string {
  if (kind === 'donations') {
    const amount = formatAmountMicros(value);
    return currency ? `${amount} ${currency}` : amount;
  }
  return value.toLocaleString();
}

/**
 * The Stage 18A goal-widget renderer: a single, transparent, bounded
 * progress presentation styled entirely from a public snapshot's own
 * `presentation` fields (docs/goals-widgets.md §22-§23) - never
 * arbitrary CSS, never a `visualdesign.Document`. Used both by the real
 * public route (pages/PublicWidgetPage.tsx, which sizes its own
 * full-viewport wrapper) and the management page's own in-editor
 * preview (components/goals/WidgetProfileManager.tsx) - this component
 * always fills 100% of whatever its parent gives it, never a fixed
 * viewport size of its own (mirrors AlertRenderer/
 * ChatOverlayRenderer's identical "one renderer, two call sites"
 * convention).
 */
export function GoalWidgetRenderer({ snapshot }: { snapshot: PublicWidgetSnapshot }) {
  const reducedMotion = usePrefersReducedMotion();
  const { presentation } = snapshot;

  // The visual bar clamps to 100% while the textual current value
  // stays the real, potentially-over-target number (docs/goals-
  // widgets.md §13) - a presentation-only clamp, never a persisted one.
  const fillPercent = Math.min(100, Math.max(0, snapshot.progressBasisPoints / 100));
  const percentLabel = `${Math.round(snapshot.progressBasisPoints) / 100}%`;

  const fontClass =
    presentation.fontFamily === 'serif'
      ? 'font-serif'
      : presentation.fontFamily === 'monospace'
        ? 'font-mono'
        : presentation.fontFamily === 'rounded'
          ? 'font-sans'
          : 'font-sans';

  const alignClass =
    presentation.textAlign === 'left' ? 'items-start text-left' : presentation.textAlign === 'right' ? 'items-end text-right' : 'items-center text-center';

  return (
    <div
      className={cn('flex w-full flex-col gap-2 p-3', fontClass, alignClass, presentation.orientation === 'vertical' && 'max-w-xs')}
      style={{
        backgroundColor: presentation.backgroundColor,
        color: presentation.foregroundColor,
        borderColor: presentation.borderColor,
        borderWidth: 1,
        borderStyle: 'solid',
        borderRadius: presentation.borderRadiusPx,
        opacity: presentation.opacity,
      }}
    >
      <div className="flex w-full items-baseline justify-between gap-2">
        <span className="truncate text-sm font-semibold">{snapshot.title}</span>
        {snapshot.completed && <span className="shrink-0 text-xs font-medium">&#10003;</span>}
      </div>

      {(presentation.showCurrent || presentation.showTarget) && (
        <div className="text-xs tabular-nums opacity-90">
          {presentation.showCurrent && formatValue(snapshot.goalKind, snapshot.current, snapshot.currency)}
          {presentation.showCurrent && presentation.showTarget && ' / '}
          {presentation.showTarget && formatValue(snapshot.goalKind, snapshot.target, snapshot.currency)}
        </div>
      )}

      <div className="h-2 w-full overflow-hidden rounded-full bg-black/20">
        <div
          className={cn('h-full rounded-full', !reducedMotion && 'transition-[width] duration-500 ease-out')}
          style={{ width: `${fillPercent}%`, backgroundColor: presentation.fillColor }}
        />
      </div>

      {presentation.showPercent && <span className="text-[11px] tabular-nums opacity-80">{percentLabel}</span>}
    </div>
  );
}
