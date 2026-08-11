import type { ReactNode } from 'react';

import type {
  VisualDesignAnimation,
  VisualDesignAvatarProps,
  VisualDesignBadgeListProps,
  VisualDesignFrame,
  VisualDesignLayerKind,
  VisualDesignMessageFragmentsProps,
  VisualDesignShapeProps,
  VisualDesignTextProps,
} from '@/api/visualdesign-schemas';
import { cn } from '@/lib/cn';

import { AvatarLayer } from './AvatarLayer';
import type { RenderableBadge } from './BadgeListLayer';
import { BadgeListLayer } from './BadgeListLayer';
import { layerEntryAnimationClassName, layerFrameStyle } from './design-style';
import { ImageLayer, type RenderableImageProps } from './ImageLayer';
import type { RenderableFragment } from './MessageFragmentsLayer';
import { MessageFragmentsLayer } from './MessageFragmentsLayer';
import { PlatformIconLayer } from './PlatformIconLayer';
import { ShapeLayer } from './ShapeLayer';
import { TextLayer } from './TextLayer';
import type { VisualBindingContext } from './text-binding';
import { VideoLayer, type RenderableVideoProps } from './VideoLayer';

export type { RenderableBadge } from './BadgeListLayer';
export type { RenderableFragment } from './MessageFragmentsLayer';

/** A resolved-asset lookup keyed by local managed-asset id (Stage 14B
 * task Part 42: "assets: <localAssetId>: {kind, mediaType, url}") -
 * built by the Designer from its own managed-asset library query. A
 * public payload never needs this: every reference there already
 * arrives pre-resolved (`image.url`/`video.url`/`text.fontUrl`). */
export type VisualAssetMap = Record<string, { url: string; mediaType?: string }>;

export type RenderableLayer = {
  id: string;
  kind: VisualDesignLayerKind;
  frame: VisualDesignFrame;
  opacity: number;
  shape?: VisualDesignShapeProps | undefined;
  text?: VisualDesignTextProps | undefined;
  platformIcon?: Record<string, never> | undefined;
  avatar?: VisualDesignAvatarProps | undefined;
  messageFragments?: VisualDesignMessageFragmentsProps | undefined;
  badgeList?: VisualDesignBadgeListProps | undefined;
  image?: RenderableImageProps | undefined;
  video?: RenderableVideoProps | undefined;
  entryAnimation: VisualDesignAnimation;
  exitAnimation: VisualDesignAnimation;
  animationDurationMs: number;
  visible?: boolean | undefined;
};

function resolveAssetUrl(assetId: string | undefined, assetMap: VisualAssetMap | undefined): string | undefined {
  return assetId === undefined ? undefined : assetMap?.[assetId]?.url;
}

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
  messageFragments,
  badges,
  mode,
  prefersReducedMotion,
  chrome,
  assetMap,
}: {
  layer: RenderableLayer;
  scale: number;
  context: VisualBindingContext;
  providerId: string;
  avatarUrl: string | null;
  messageFragments?: readonly RenderableFragment[] | undefined;
  badges?: readonly RenderableBadge[] | undefined;
  mode: 'public' | 'preview';
  prefersReducedMotion: boolean;
  chrome?: ((layer: RenderableLayer, scale: number, children: ReactNode) => ReactNode) | undefined;
  /** Management-only (Stage 14B task Part 42) - undefined on the public
   * route, where every reference already arrives pre-resolved. */
  assetMap?: VisualAssetMap | undefined;
}) {
  const animationClass = layerEntryAnimationClassName(layer.entryAnimation, prefersReducedMotion);

  let content: ReactNode = null;
  if (layer.kind === 'shape' && layer.shape !== undefined) {
    content = <ShapeLayer shape={layer.shape} scale={scale} />;
  } else if (layer.kind === 'text' && layer.text !== undefined) {
    const fontUrl = layer.text.fontUrl ?? resolveAssetUrl(layer.text.fontAssetId, assetMap);
    content = <TextLayer text={layer.text} scale={scale} context={context} mode={mode} fontUrl={fontUrl} />;
  } else if (layer.kind === 'platform_icon') {
    content = <PlatformIconLayer providerId={providerId} />;
  } else if (layer.kind === 'avatar' && layer.avatar !== undefined) {
    content = <AvatarLayer avatar={layer.avatar} avatarUrl={avatarUrl} scale={scale} />;
  } else if (layer.kind === 'message_fragments' && layer.messageFragments !== undefined) {
    const fontUrl = layer.messageFragments.fontUrl ?? resolveAssetUrl(layer.messageFragments.fontAssetId, assetMap);
    content = (
      <MessageFragmentsLayer props={layer.messageFragments} fragments={messageFragments} scale={scale} fontUrl={fontUrl} />
    );
  } else if (layer.kind === 'badge_list' && layer.badgeList !== undefined) {
    content = <BadgeListLayer props={layer.badgeList} badges={badges} />;
  } else if (layer.kind === 'image' && layer.image !== undefined) {
    content = <ImageLayer image={layer.image} assetMap={assetMap} prefersReducedMotion={prefersReducedMotion} />;
  } else if (layer.kind === 'video' && layer.video !== undefined) {
    content = <VideoLayer video={layer.video} assetMap={assetMap} prefersReducedMotion={prefersReducedMotion} />;
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
