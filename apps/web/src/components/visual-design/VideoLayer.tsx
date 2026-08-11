import { useState } from 'react';

import type { VisualDesignImageFit } from '@/api/visualdesign-schemas';

/** See ImageLayer's own RenderableImageProps doc comment - identical
 * reasoning, one shape covering both management (`assetId`) and public
 * (`url`) payloads. */
export type RenderableVideoProps = {
  assetId?: string | undefined;
  fit: VisualDesignImageFit;
  loop: boolean;
  url?: string | null | undefined;
  mediaType?: string | undefined;
};

/** Renders a Stage 14B managed video asset - always muted, always
 * `playsInline`, never any user-facing controls, never any volume/
 * poster/track/subtitle input (docs/visual-template-packages.md §20/
 * §40): sound/audio playback is explicitly out of scope for this
 * package - Stage 17 owns the application's one audio subsystem, so a
 * video's own embedded audio track (if any) is never played regardless
 * of what this component receives. Never autoplays under
 * `prefersReducedMotion` (docs/visual-template-packages.md §21). Fails
 * safe (renders nothing) if the reference cannot be resolved or the
 * video fails to load. */
export function VideoLayer({
  video,
  assetMap,
  prefersReducedMotion,
}: {
  video: RenderableVideoProps;
  assetMap?: Record<string, { url: string; mediaType?: string }> | undefined;
  prefersReducedMotion: boolean;
}) {
  const [broken, setBroken] = useState(false);
  const resolved = video.url !== undefined && video.url !== null ? { url: video.url } : video.assetId !== undefined ? assetMap?.[video.assetId] : undefined;
  const url = resolved?.url ?? null;

  if (url === null || broken) return null;

  return (
    <video
      src={url}
      muted
      playsInline
      autoPlay={!prefersReducedMotion}
      loop={video.loop}
      controls={false}
      style={{ width: '100%', height: '100%', objectFit: video.fit }}
      data-testid="visual-design-video"
      onError={() => setBroken(true)}
    />
  );
}
