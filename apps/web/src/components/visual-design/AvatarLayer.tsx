import { useState } from 'react';

import type { VisualDesignAvatarProps } from '@/api/visualdesign-schemas';

import { avatarLayerStyle } from './design-style';

/** Renders the alert's own already-safe, already-normalized avatar URL
 * - defense-in-depth HTTPS-only re-check client-side, a safe fallback
 * (renders nothing) whenever the URL is absent, non-HTTPS, or fails to
 * load, and never any arbitrary URL input (Stage 13A task Part 46).
 * `avatarUrl` is null for every real Stage 12A/12B event type today
 * (no normalization path populates it yet) - this is the honest,
 * expected state, not a bug. */
export function AvatarLayer({
  avatar,
  avatarUrl,
  scale,
}: {
  avatar: VisualDesignAvatarProps;
  avatarUrl: string | null;
  scale: number;
}) {
  const [broken, setBroken] = useState(false);
  const safe = avatarUrl !== null && avatarUrl.startsWith('https://') && !broken;

  if (!safe) return null;

  return (
    <img
      src={avatarUrl}
      alt=""
      style={avatarLayerStyle(avatar, scale)}
      data-testid="visual-design-avatar"
      onError={() => setBroken(true)}
    />
  );
}
