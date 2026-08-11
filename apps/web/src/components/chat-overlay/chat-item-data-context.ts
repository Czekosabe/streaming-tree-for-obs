import type { ParseKeys, TFunction } from 'i18next';

import type { PublicChatOverlayItem } from '@/api/chat-overlay-schemas';
import type { VisualDesignDataContext } from '@/components/visual-design/VisualDesignRenderer';
import { platformDisplayName } from '@/components/visual-design/text-binding';

type OverlaysKey = ParseKeys<'overlays'>;

/** Mirrors OverlayActivity.tsx's own ACTIVITY_TYPE_KEYS exactly - the
 * one place a chat activity type is translated for the *legacy*
 * renderer; reused here so a design-driven `event_type` binding shows
 * the identical label a legacy overlay would. */
const ACTIVITY_TYPE_KEYS: Record<string, OverlaysKey> = {
  follow: 'renderer.activityType.follow',
  subscription: 'renderer.activityType.subscription',
  resubscription: 'renderer.activityType.resubscription',
  gifted_subscription: 'renderer.activityType.gifted_subscription',
  subscription_gift_batch: 'renderer.activityType.subscription_gift_batch',
  bits: 'renderer.activityType.bits',
  raid: 'renderer.activityType.raid',
  channel_point_redemption: 'renderer.activityType.channel_point_redemption',
};

/** Builds a design-driven chat item's own VisualDesignDataContext
 * (Stage 13B, docs/visual-designs.md §20) - the chat-side analogue of
 * AlertRenderer.tsx's own inline dataContext construction. Never
 * fabricates a value: an absent field (no message, no activity
 * quantity, no account label, an anonymous user) stays genuinely
 * `null`/absent so a bound layer hides safely rather than showing
 * "undefined". */
export function chatItemDataContext(item: PublicChatOverlayItem, t: TFunction<'overlays'>): VisualDesignDataContext {
  const anonymous = item.user?.anonymous === true;
  const username = anonymous || item.user === undefined ? null : (item.user.displayName ?? null);
  const message = item.deleted ? null : (item.message?.plainText ?? null);
  const activityLabelKey = item.activity !== undefined ? ACTIVITY_TYPE_KEYS[item.activity.activityType] : undefined;
  const eventType =
    item.activity === undefined
      ? null
      : activityLabelKey !== undefined
        ? t(activityLabelKey)
        : item.activity.activityType;
  const quantity = item.activity?.quantity ?? null;

  return {
    providerId: item.providerId,
    avatarUrl: anonymous ? null : (item.user?.avatarUrl ?? null),
    bindings: {
      renderedText: null,
      username,
      platform: platformDisplayName(item.providerId),
      eventType,
      message,
      quantity,
      groupCount: 1,
      timestamp: new Date(item.occurredAt).toLocaleTimeString(),
      accountLabel: item.accountLabel ?? null,
    },
    messageFragments: item.deleted ? undefined : item.message?.fragments,
    badges: anonymous ? undefined : item.user?.badges,
  };
}
