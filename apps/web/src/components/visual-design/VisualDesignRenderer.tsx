import type { ReactNode } from 'react';

import type { VisualDesignCanvas } from '@/api/visualdesign-schemas';

import { useContainScale } from './design-scale';
import type { VisualBindingContext } from './text-binding';
import type { RenderableBadge, RenderableFragment, RenderableLayer } from './VisualLayer';
import { VisualLayer } from './VisualLayer';

export type { RenderableFragment, RenderableBadge } from './VisualLayer';

/**
 * The normalized, owner-independent data a visual design renders
 * against (Stage 13B task Part 16: "dataContext is a normalized
 * visual-binding context, not an alert object and not a Twitch
 * object"). `bindings.*` are already-resolved display strings (i18n
 * labels resolved by the caller before this component ever sees them) -
 * `VisualDesignRenderer` itself never imports react-i18next. Every
 * field an owner has no equivalent for is genuinely `null` (never a
 * fabricated placeholder) - e.g. a chat item's `renderedText`/
 * `groupCount` default (1), or an alert's `timestamp`/`accountLabel`.
 */
export type VisualDesignDataContext = {
  providerId: string;
  avatarUrl: string | null;
  bindings: VisualBindingContext;
  /** Chat-only rich content (Stage 13B, docs/visual-designs.md §21) -
   * undefined/empty for an alert, which has no message_fragments
   * layer kind in practice. */
  messageFragments?: readonly RenderableFragment[] | undefined;
  badges?: readonly RenderableBadge[] | undefined;
};

/**
 * The one shared visual-design renderer (Stage 13A task Part 24/25;
 * Stage 13B task Part 16): used identically by both the Alert Designer/
 * public alert route and the Chat Overlay Designer/public chat overlay
 * route - never a second, forked rendering implementation. Scales
 * `canvas`/`layers` into the wrapper's own live size using the
 * deterministic contain-style transform (docs/visual-designs.md §3),
 * resolves each text layer's binding against `dataContext`, and never
 * renders anything beyond ordinary positioned DOM elements (no canvas/
 * WebGL, no `dangerouslySetInnerHTML`, no arbitrary CSS/URL).
 *
 * `mode="public"` hides any layer whose bound value is absent;
 * `mode="preview"` (editor-only) may show a synthetic missing-data
 * indicator per layer instead - see TextLayer.tsx.
 */
export function VisualDesignRenderer({
  canvas,
  layers,
  dataContext,
  mode,
  prefersReducedMotion,
  chrome,
}: {
  canvas: VisualDesignCanvas;
  layers: RenderableLayer[];
  dataContext: VisualDesignDataContext;
  mode: 'public' | 'preview';
  prefersReducedMotion: boolean;
  chrome?: ((layer: RenderableLayer, scale: number, children: ReactNode) => ReactNode) | undefined;
}) {
  const [wrapperRef, transform] = useContainScale(canvas);

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
            context={dataContext.bindings}
            providerId={dataContext.providerId}
            avatarUrl={dataContext.avatarUrl}
            messageFragments={dataContext.messageFragments}
            badges={dataContext.badges}
            mode={mode}
            prefersReducedMotion={prefersReducedMotion}
            chrome={chrome}
          />
        ))}
      </div>
    </div>
  );
}
