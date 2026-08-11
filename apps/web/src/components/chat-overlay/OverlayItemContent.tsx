import { useTranslation } from 'react-i18next';

import type { PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import { VisualDesignRenderer } from '@/components/visual-design/VisualDesignRenderer';

import { chatItemDataContext } from './chat-item-data-context';
import { OverlayActivity } from './OverlayActivity';
import { OverlayMessage } from './OverlayMessage';
import { overlayItemStyle } from './overlay-style';

/**
 * One item's own content - legacy (OverlayMessage/OverlayActivity,
 * styled from the profile's own bubble/font/color settings) or
 * design-driven (the shared VisualDesignRenderer, styled entirely by
 * the saved design's own layers - Stage 13B, docs/visual-designs.md
 * §17). Shared by both ChatOverlayRenderer's own active-item render and
 * OverlayLeavingItem, so the branch is never duplicated. The item's own
 * entry/exit animation wrapper (config.entryAnimation/exitAnimation/
 * animationDurationMs) stays owned by the caller regardless of mode -
 * exactly like AlertRenderer.tsx's own outer wrapper, which also stays
 * uniform across legacy/design-driven content.
 */
export function OverlayItemContent({
  item,
  config,
  prefersReducedMotion,
}: {
  item: PublicChatOverlayItem;
  config: PublicChatOverlayConfig;
  prefersReducedMotion: boolean;
}) {
  const { t } = useTranslation('overlays');

  if (config.renderingMode === 'visual_design' && config.visualDesign) {
    return (
      <div style={{ aspectRatio: `${config.visualDesign.canvas.width} / ${config.visualDesign.canvas.height}`, width: '100%' }}>
        <VisualDesignRenderer
          canvas={config.visualDesign.canvas}
          layers={config.visualDesign.layers}
          dataContext={chatItemDataContext(item, t)}
          mode="public"
          prefersReducedMotion={prefersReducedMotion}
        />
      </div>
    );
  }

  return (
    <div style={overlayItemStyle(config)}>
      {item.kind === 'message' ? <OverlayMessage item={item} config={config} /> : <OverlayActivity item={item} config={config} />}
    </div>
  );
}
