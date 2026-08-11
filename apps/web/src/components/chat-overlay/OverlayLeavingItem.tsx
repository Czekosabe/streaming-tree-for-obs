import { useEffect } from 'react';

import type { PublicChatOverlayConfig } from '@/api/chat-overlay-schemas';
import type { ChatOverlayLeavingItem as LeavingItemEntry } from '@/models/chat-overlay-reducer';

import { OverlayItemContent } from './OverlayItemContent';
import { exitAnimationClassName, exitAnimationFallbackMs } from './overlay-style';

type OverlayLeavingItemProps = {
  entry: LeavingItemEntry;
  config: PublicChatOverlayConfig;
  prefersReducedMotion: boolean;
  onComplete: (id: string) => void;
};

/**
 * One item mid cosmetic exit-animation (expiry or capacity eviction -
 * see models/chat-overlay-reducer.ts's own doc comment; a moderation/
 * clear/settings removal never reaches this component at all, it is
 * applied immediately by the reducer instead). Calls `onComplete` from
 * whichever fires first: the CSS `animationend` event, or a hard
 * fallback `setTimeout` - `animationend` is never the only removal
 * path, so a missed event (a CSS bug, a throttled background tab)
 * cannot leave an item stuck on screen forever. When there is no
 * animation to play at all (`animation: none`, or the viewer's own
 * `prefers-reduced-motion`), completes immediately instead of
 * rendering a frame.
 */
export function OverlayLeavingItem({ entry, config, prefersReducedMotion, onComplete }: OverlayLeavingItemProps) {
  const exitClass = exitAnimationClassName(config.exitAnimation, prefersReducedMotion);
  const itemId = entry.item.id;

  useEffect(() => {
    if (exitClass === '') {
      onComplete(itemId);
      return;
    }
    const timeout = window.setTimeout(() => onComplete(itemId), exitAnimationFallbackMs(config.animationDurationMs));
    return () => window.clearTimeout(timeout);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire once per leaving item id/animation choice, not on every config identity change
  }, [itemId, exitClass, config.animationDurationMs]);

  if (exitClass === '') return null;

  return (
    <div
      className={exitClass}
      data-testid="chat-overlay-leaving-item"
      data-remove-reason={entry.reason}
      onAnimationEnd={() => onComplete(itemId)}
    >
      <OverlayItemContent item={entry.item} config={config} prefersReducedMotion={prefersReducedMotion} />
    </div>
  );
}
