import type { ReactNode } from 'react';

import type {
  VisualDesignAnimation,
  VisualDesignAvatarProps,
  VisualDesignFrame,
  VisualDesignLayerKind,
  VisualDesignShapeProps,
  VisualDesignTextProps,
} from '@/api/visualdesign-schemas';
import { cn } from '@/lib/cn';

import { AvatarLayer } from './AvatarLayer';
import { layerEntryAnimationClassName, layerFrameStyle } from './design-style';
import { PlatformIconLayer } from './PlatformIconLayer';
import { ShapeLayer } from './ShapeLayer';
import { TextLayer } from './TextLayer';
import type { AlertBindingContext } from './text-binding';

export type RenderableLayer = {
  id: string;
  kind: VisualDesignLayerKind;
  frame: VisualDesignFrame;
  opacity: number;
  shape?: VisualDesignShapeProps | undefined;
  text?: VisualDesignTextProps | undefined;
  platformIcon?: Record<string, never> | undefined;
  avatar?: VisualDesignAvatarProps | undefined;
  entryAnimation: VisualDesignAnimation;
  exitAnimation: VisualDesignAnimation;
  animationDurationMs: number;
  visible?: boolean | undefined;
};

/**
 * One positioned layer: converts its design-space frame to scaled
 * pixels, applies its own entry animation (Stage 13A task Part 15 -
 * per-layer, independent of the outer alert's own mount/unmount
 * transition), and dispatches to the right kind-specific component.
 * `chrome` lets the Designer wrap the layer with selection/drag
 * handles without this component (or the public route) ever knowing
 * about editing at all (Part 24/25 - one shared rendering
 * implementation, editing chrome added around it, never forked).
 */
export function VisualLayer({
  layer,
  scale,
  context,
  providerId,
  avatarUrl,
  mode,
  prefersReducedMotion,
  chrome,
}: {
  layer: RenderableLayer;
  scale: number;
  context: AlertBindingContext;
  providerId: string;
  avatarUrl: string | null;
  mode: 'public' | 'preview';
  prefersReducedMotion: boolean;
  chrome?: ((layer: RenderableLayer, scale: number, children: ReactNode) => ReactNode) | undefined;
}) {
  const animationClass = layerEntryAnimationClassName(layer.entryAnimation, prefersReducedMotion);

  let content: ReactNode = null;
  if (layer.kind === 'shape' && layer.shape !== undefined) {
    content = <ShapeLayer shape={layer.shape} scale={scale} />;
  } else if (layer.kind === 'text' && layer.text !== undefined) {
    content = <TextLayer text={layer.text} scale={scale} context={context} mode={mode} />;
  } else if (layer.kind === 'platform_icon') {
    content = <PlatformIconLayer providerId={providerId} />;
  } else if (layer.kind === 'avatar' && layer.avatar !== undefined) {
    content = <AvatarLayer avatar={layer.avatar} avatarUrl={avatarUrl} scale={scale} />;
  }

  const positioned = (
    <div
      style={layerFrameStyle(layer.frame, scale, layer.opacity)}
      className={cn(animationClass)}
      data-testid="visual-design-layer"
      data-layer-id={layer.id}
      data-layer-kind={layer.kind}
    >
      {content}
    </div>
  );

  return chrome !== undefined ? <>{chrome(layer, scale, positioned)}</> : positioned;
}
