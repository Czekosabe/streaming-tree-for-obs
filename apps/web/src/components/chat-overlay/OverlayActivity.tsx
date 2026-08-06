import type { ParseKeys } from 'i18next';
import { useTranslation } from 'react-i18next';

import type { PublicChatOverlayConfig, PublicChatOverlayItem } from '@/api/chat-overlay-schemas';

import { OverlaySourceMarker } from './OverlaySourceMarker';

type OverlaysKey = ParseKeys<'overlays'>;

/** Translation key per known activity type - an unrecognized type renders
 * its raw identifier rather than a missing-key warning, mirroring
 * operator-chat-presentation.ts's own activityTypeKey. */
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

export function OverlayActivity({
  item,
  config,
}: {
  item: PublicChatOverlayItem;
  config: PublicChatOverlayConfig;
}) {
  const { t } = useTranslation('overlays');
  const activity = item.activity;
  if (activity === undefined) return null;

  const labelKey = Object.prototype.hasOwnProperty.call(ACTIVITY_TYPE_KEYS, activity.activityType)
    ? ACTIVITY_TYPE_KEYS[activity.activityType]
    : undefined;
  const label = labelKey !== undefined ? t(labelKey) : activity.activityType;
  const displayName = item.user?.anonymous === true || item.user === undefined ? null : item.user.displayName;

  return (
    <div className="flex items-center gap-1.5">
      <OverlaySourceMarker
        providerId={item.providerId}
        accountLabel={item.accountLabel}
        showIcon={config.showPlatformIcon}
        showName={config.showPlatformName}
      />
      <p className="italic opacity-90">
        {displayName !== null && displayName !== '' ? (
          <span className="font-semibold not-italic">{displayName} </span>
        ) : null}
        {label}
        {activity.quantity !== undefined && ` × ${activity.quantity}`}
        {activity.amount !== undefined &&
          ` (${activity.amount}${activity.currency !== undefined ? ` ${activity.currency}` : ''})`}
      </p>
    </div>
  );
}
