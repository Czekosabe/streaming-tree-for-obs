import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import type { VisualDesignCanvas } from '@/api/visualdesign-schemas';

import { useContainScale } from './design-scale';
import { platformDisplayName } from './text-binding';
import type { RenderableLayer } from './VisualLayer';
import { VisualLayer } from './VisualLayer';

export type VisualDesignAlertData = {
  eventType: string;
  providerId: string;
  username: string | null;
  message: string | null;
  quantity: number | null;
  groupCount: number;
  renderedText: string;
  avatarUrl: string | null;
};

/**
 * The one shared visual-design renderer (Stage 13A task Part 24/25):
 * used identically by the Alert Designer's own live preview and the
 * real public Browser Source route (`PublicAlertPage.tsx`) - never a
 * fake/approximate preview implementation. Scales `canvas`/`layers`
 * into the wrapper's own live size using the deterministic
 * contain-style transform (docs/visual-designs.md §3), resolves each
 * text layer's binding against `alert`, and never renders anything
 * beyond ordinary positioned DOM elements (no canvas/WebGL, no
 * `dangerouslySetInnerHTML`, no arbitrary CSS/URL).
 *
 * `mode="public"` hides any layer whose bound value is absent;
 * `mode="preview"` (editor-only) may show a synthetic missing-data
 * indicator per layer instead - see TextLayer.tsx.
 */
export function VisualDesignRenderer({
  canvas,
  layers,
  alert,
  mode,
  prefersReducedMotion,
  chrome,
}: {
  canvas: VisualDesignCanvas;
  layers: RenderableLayer[];
  alert: VisualDesignAlertData;
  mode: 'public' | 'preview';
  prefersReducedMotion: boolean;
  chrome?: ((layer: RenderableLayer, scale: number, children: ReactNode) => ReactNode) | undefined;
}) {
  const { t } = useTranslation('alerts');
  const [wrapperRef, transform] = useContainScale(canvas);

  const context = {
    renderedText: alert.renderedText,
    username: alert.username,
    platformLabel: platformDisplayName(alert.providerId),
    eventTypeLabel: t(`rules.eventType.${alert.eventType}`, { defaultValue: alert.eventType }),
    message: alert.message,
    quantity: alert.quantity,
    groupCount: alert.groupCount,
  };

  // Visible-only, in the caller's own order (both PublicDocument.layers
  // and the Designer's own normalized management layers are already
  // ordered back-to-front - see ToPublic/normalizeLayerOrder).
  const visibleLayers = layers.filter((l) => l.visible !== false);

  return (
    <div ref={wrapperRef} className="relative h-full w-full overflow-hidden" data-testid="visual-design-renderer">
      <div
        style={{
          position: 'absolute',
          left: transform.offsetX,
          top: transform.offsetY,
          width: canvas.width * transform.scale,
          height: canvas.height * transform.scale,
        }}
        data-testid="visual-design-canvas"
      >
        {visibleLayers.map((layer) => (
          <VisualLayer
            key={layer.id}
            layer={layer}
            scale={transform.scale}
            context={context}
            providerId={alert.providerId}
            avatarUrl={alert.avatarUrl}
            mode={mode}
            prefersReducedMotion={prefersReducedMotion}
            chrome={chrome}
          />
        ))}
      </div>
    </div>
  );
}
