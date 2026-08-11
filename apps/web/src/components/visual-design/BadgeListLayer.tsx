import type { VisualDesignBadgeListProps } from '@/api/visualdesign-schemas';

import { badgeListContainerStyle, badgeListImageStyle } from './design-style';

export type RenderableBadge = {
  setId: string;
  id: string;
  imageUrl1x?: string | undefined;
  imageUrl2x?: string | undefined;
  imageUrl4x?: string | undefined;
};

/**
 * Renders an item's own already-resolved public badge image DTOs
 * (Stage 13B, docs/visual-designs.md §21) - bounded count, safe HTTPS
 * image URLs only, never a provider request in the renderer, never an
 * arbitrary URL stored in the design itself.
 */
export function BadgeListLayer({
  props,
  badges,
}: {
  props: VisualDesignBadgeListProps;
  badges: readonly RenderableBadge[] | undefined;
}) {
  if (badges === undefined || badges.length === 0) return null;
  const bounded = badges.slice(0, props.maxCount);

  return (
    <div style={badgeListContainerStyle(props)} data-testid="visual-design-badge-list">
      {bounded.map((badge) => {
        const url = badge.imageUrl2x ?? badge.imageUrl1x;
        if (url === undefined || !url.startsWith('https://')) return null;
        return (
          <img
            key={`${badge.setId}/${badge.id}`}
            src={url}
            alt={badge.setId}
            style={badgeListImageStyle(props)}
            data-testid="visual-design-badge"
          />
        );
      })}
    </div>
  );
}
