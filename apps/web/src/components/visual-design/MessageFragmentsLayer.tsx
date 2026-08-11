import type { VisualDesignMessageFragmentsProps } from '@/api/visualdesign-schemas';
import { useManagedFont } from '@/hooks/use-managed-font';

import { messageFragmentsContainerStyle, messageFragmentsEmoteStyle, messageFragmentsTextStyle } from './design-style';

export type RenderableFragment = {
  type: 'text' | 'emote' | 'mention';
  text: string;
  emoteImageUrl?: string | undefined;
};

/**
 * Renders an item's own already-normalized, already-ordered message
 * fragments (Stage 13B, docs/visual-designs.md §21) - plain text as
 * text, mention as text/presentation, an already-resolved safe emote
 * image where available. Never re-parses raw IRC/EventSub payload,
 * never `dangerouslySetInnerHTML`, never makes a provider request at
 * render time. An unknown/unrecognized fragment type (defensively,
 * should never arrive past Zod validation) falls back to its own safe
 * text.
 */
export function MessageFragmentsLayer({
  props,
  fragments,
  scale,
  fontUrl,
}: {
  props: VisualDesignMessageFragmentsProps;
  fragments: readonly RenderableFragment[] | undefined;
  scale: number;
  fontUrl?: string | undefined;
}) {
  const customFontFamily = useManagedFont(props.fontAssetId, fontUrl) ?? undefined;
  if (fragments === undefined || fragments.length === 0) return null;

  return (
    <div style={messageFragmentsContainerStyle(props)} data-testid="visual-design-message-fragments">
      {fragments.map((fragment, index) => {
        const key = `${index}-${fragment.type}`;
        if (fragment.type === 'emote' && fragment.emoteImageUrl !== undefined && fragment.emoteImageUrl.startsWith('https://')) {
          return (
            <img
              key={key}
              src={fragment.emoteImageUrl}
              alt={fragment.text}
              style={messageFragmentsEmoteStyle(props, scale)}
              data-testid="visual-design-fragment-emote"
            />
          );
        }
        return (
          <span key={key} style={messageFragmentsTextStyle(props, scale, customFontFamily)} data-testid="visual-design-fragment-text">
            {fragment.text}
          </span>
        );
      })}
    </div>
  );
}
