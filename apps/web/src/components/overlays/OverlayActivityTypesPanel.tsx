import type { ParseKeys } from 'i18next';
import { useTranslation } from 'react-i18next';

import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { useChatOverlayActivityTypesQuery, useSetChatOverlayActivityTypesMutation } from '@/hooks/use-chat-overlay';

type OverlaysKey = ParseKeys<'overlays'>;

const ACTIVITY_TYPES: { value: string; labelKey: OverlaysKey }[] = [
  { value: 'follow', labelKey: 'activityTypes.follow' },
  { value: 'subscription', labelKey: 'activityTypes.subscription' },
  { value: 'resubscription', labelKey: 'activityTypes.resubscription' },
  { value: 'gifted_subscription', labelKey: 'activityTypes.gifted_subscription' },
  { value: 'subscription_gift_batch', labelKey: 'activityTypes.subscription_gift_batch' },
  { value: 'bits', labelKey: 'activityTypes.bits' },
  { value: 'raid', labelKey: 'activityTypes.raid' },
  { value: 'channel_point_redemption', labelKey: 'activityTypes.channel_point_redemption' },
];

/** Which activity types this overlay shows - empty means every type
 * (Part 4's own documented default), mirroring OverlayAccountsPanel's
 * own immediate-save behavior. */
export function OverlayActivityTypesPanel({ overlayId }: { overlayId: string }) {
  const { t } = useTranslation('overlays');
  const query = useChatOverlayActivityTypesQuery(overlayId);
  const setTypes = useSetChatOverlayActivityTypesMutation(overlayId);

  const selected = new Set(query.data ?? []);

  function toggle(type: string, checked: boolean) {
    const next = new Set(selected);
    if (checked) next.add(type);
    else next.delete(type);
    setTypes.mutate(Array.from(next));
  }

  return (
    <Panel>
      <PanelHeader title={t('activityTypes.title')} description={t('activityTypes.description')} />
      <PanelBody className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {ACTIVITY_TYPES.map(({ value, labelKey }) => (
          <label key={value} className="flex cursor-pointer items-center gap-2 text-sm text-ink">
            <input
              type="checkbox"
              checked={selected.has(value)}
              onChange={(event) => toggle(value, event.target.checked)}
              className="size-4 rounded border-line accent-accent"
            />
            {t(labelKey)}
          </label>
        ))}
      </PanelBody>
    </Panel>
  );
}
