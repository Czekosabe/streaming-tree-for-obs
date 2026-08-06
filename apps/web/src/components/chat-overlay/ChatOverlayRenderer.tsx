import { useEffect, useState } from 'react';

import type { PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import { cn } from '@/lib/cn';

import { OverlayActivity } from './OverlayActivity';
import { OverlayMessage } from './OverlayMessage';
import { entryAnimationClassName, overlayContainerStyle, overlayItemStyle } from './overlay-style';

function supportsMatchMedia(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function';
}

/** Tracks the `prefers-reduced-motion` media query - non-essential entry
 * animation is disabled when set, per Part 11; a moderation removal
 * still completes immediately regardless (the renderer never animates an
 * item out, see overlay-style.ts's own doc comment). Degrades to "not
 * reduced" when `matchMedia` itself is unavailable (a test environment
 * without it), rather than throwing. */
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

/**
 * The Browser Source renderer: a transparent, responsive column of chat
 * messages and activity events, styled entirely from a validated public
 * config. Used both by the real overlay route (pages/OverlayChatPage.tsx,
 * which sizes its own full-viewport wrapper) and the management preview
 * (components/overlays/OverlayPreviewPanel.tsx, fed by local,
 * clearly-synthetic fixtures, inside a bounded box) - this component
 * itself always fills 100% of whatever its parent gives it, never a
 * fixed viewport size of its own, so both call sites get correct
 * behavior for free. Renders nothing but what `items` already contains -
 * no additional filtering, no operator-only data, no
 * dangerouslySetInnerHTML.
 */
export function ChatOverlayRenderer({
  config,
  items,
}: {
  config: PublicChatOverlayConfig;
  items: PublicChatOverlayItem[];
}) {
  const prefersReducedMotion = usePrefersReducedMotion();
  const entryClass = entryAnimationClassName(config.entryAnimation, prefersReducedMotion);

  const ordered = config.stackDirection === 'top_down' ? [...items].reverse() : items;

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
      {ordered.map((item) => (
        <div
          key={item.id}
          className={cn('w-full max-w-full', entryClass)}
          style={overlayItemStyle(config)}
          data-testid="chat-overlay-item"
        >
          {item.kind === 'message' ? (
            <OverlayMessage item={item} config={config} />
          ) : (
            <OverlayActivity item={item} config={config} />
          )}
        </div>
      ))}
    </div>
  );
}
