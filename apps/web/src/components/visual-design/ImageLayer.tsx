import { useState } from 'react';

import type { VisualDesignImageFit } from '@/api/visualdesign-schemas';

/** The renderable shape an image layer's payload takes at render time -
 * a superset of both the management shape (`assetId`, no `url`) and the
 * public shape (`url`, no `assetId`) so this one component serves both
 * (Stage 14B task Part 42: "the same VisualDesignRenderer should
 * receive document + resolved asset map + data context... do not fork a
 * second renderer"). */
export type RenderableImageProps = {
  assetId?: string | undefined;
  fit: VisualDesignImageFit;
  alt?: string | undefined;
  url?: string | null | undefined;
  mediaType?: string | undefined;
};

/** Renders a Stage 14B managed image asset - never an arbitrary URL: the
 * `src` only ever comes from an already-resolved, app-owned asset URL
 * (either the public payload's own `url`, or a management-side
 * `assetMap` lookup by local asset id). Fails safe (renders nothing) if
 * the reference cannot be resolved or the image fails to load - an
 * image layer must never crash the overlay. Under `prefersReducedMotion`,
 * a GIF/WebP asset (both potentially animated - docs/visual-template-
 * packages.md §21's conservative "treat every WebP as animated" rule)
 * is hidden entirely rather than guessed at. */
export function ImageLayer({
  image,
  assetMap,
  prefersReducedMotion,
}: {
  image: RenderableImageProps;
  assetMap?: Record<string, { url: string; mediaType?: string }> | undefined;
  prefersReducedMotion: boolean;
}) {
  const [broken, setBroken] = useState(false);
  const resolved = image.url !== undefined && image.url !== null ? { url: image.url, mediaType: image.mediaType } : image.assetId !== undefined ? assetMap?.[image.assetId] : undefined;
  const url = resolved?.url ?? null;
  const mediaType = resolved?.mediaType;
  const potentiallyAnimated = mediaType === 'image/gif' || mediaType === 'image/webp';

  if (url === null || broken || (prefersReducedMotion && potentiallyAnimated)) return null;

  return (
    <img
      src={url}
      alt={image.alt ?? ''}
      style={{ width: '100%', height: '100%', objectFit: image.fit }}
      data-testid="visual-design-image"
      onError={() => setBroken(true)}
    />
  );
}
