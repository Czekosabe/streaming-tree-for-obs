import { useEffect, useState } from 'react';

import type { PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import { cn } from '@/lib/cn';
import type { ChatOverlayLeavingItem as LeavingItemEntry } from '@/models/chat-overlay-reducer';

import { OverlayItemContent } from './OverlayItemContent';
import { OverlayLeavingItem } from './OverlayLeavingItem';
import { entryAnimationClassName, overlayContainerStyle } from './overlay-style';

function supportsMatchMedia(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function';
}

/** Tracks the `prefers-reduced-motion` media query - non-essential entry
 * *and* exit animation are both disabled when set (Part 11); a
 * moderation/clear/settings removal is always immediate regardless,
 * animated or not - see models/chat-overlay-reducer.ts's own doc
 * comment. Degrades to "not reduced" when `matchMedia` itself is
 * unavailable (a test environment without it), rather than throwing. */
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

const ALIGNMENT_ITEMS: Record<PublicChatOverlayConfig['horizontalAlignment'], string> = {
  left: 'items-start text-left',
  center: 'items-center text-center',
  right: 'items-end text-right',
};

type RenderEntry =
  | { kind: 'active'; id: string; item: PublicChatOverlayItem }
  | { kind: 'leaving'; id: string; entry: LeavingItemEntry };

/**
 * The Browser Source renderer: a transparent, responsive column of chat
 * messages and activity events, styled entirely from a validated public
 * config. Used both by the real overlay route (pages/OverlayChatPage.tsx,
 * which sizes its own full-viewport wrapper) and the management preview
 * (components/overlays/OverlayPreviewPanel.tsx, fed by local,
 * clearly-synthetic fixtures, inside a bounded box) - this component
 * itself always fills 100% of whatever its parent gives it, never a
 * fixed viewport size of its own, so both call sites get correct
 * behavior for free. Renders nothing but what `items`/`leaving` already
 * contain - no additional filtering, no operator-only data, no
 * dangerouslySetInnerHTML.
 *
 * `leaving` items (see hooks/use-chat-overlay-stream.ts and
 * models/chat-overlay-reducer.ts) are always cosmetic - a moderation
 * deletion, a chat/user clear, or any settings-driven removal is
 * already gone from `items` by the time this component ever sees it,
 * applied immediately with no "leaving" transition. A leaving item is
 * rendered at the same relative position an active item that old would
 * have (see the ordering below), since it is by construction older
 * than every currently active item.
 */
export function ChatOverlayRenderer({
  config,
  items,
  leaving = [],
  onLeavingComplete,
}: {
  config: PublicChatOverlayConfig;
  items: PublicChatOverlayItem[];
  leaving?: LeavingItemEntry[];
  onLeavingComplete?: (id: string) => void;
}) {
  const prefersReducedMotion = usePrefersReducedMotion();
  const entryClass = entryAnimationClassName(config.entryAnimation, prefersReducedMotion);

  const combined: RenderEntry[] = [
    ...leaving.map((entry): RenderEntry => ({ kind: 'leaving', id: entry.item.id, entry })),
    ...items.map((item): RenderEntry => ({ kind: 'active', id: item.id, item })),
  ];
  const ordered = config.stackDirection === 'top_down' ? [...combined].reverse() : combined;

  return (
    <div
      className={cn(
        'flex h-full w-full flex-col overflow-hidden p-3',
        config.stackDirection === 'top_down' ? 'justify-start' : 'justify-end',
        ALIGNMENT_ITEMS[config.horizontalAlignment],
        config.layoutMode === 'vertical' ? 'max-w-full' : 'mx-auto max-w-[720px]',
      )}
      style={overlayContainerStyle(config)}
      data-testid="chat-overlay-root"
    >
      {ordered.map((entry) => {
        if (entry.kind === 'leaving') {
          return (
            <OverlayLeavingItem
              key={entry.id}
              entry={entry.entry}
              config={config}
              prefersReducedMotion={prefersReducedMotion}
              onComplete={onLeavingComplete ?? (() => {})}
            />
          );
        }
        return (
          <div
            key={entry.id}
            className={cn('w-full max-w-full', entryClass)}
            data-testid="chat-overlay-item"
            data-rendering-mode={config.renderingMode}
          >
            <OverlayItemContent item={entry.item} config={config} prefersReducedMotion={prefersReducedMotion} />
          </div>
        );
      })}
    </div>
  );
}
