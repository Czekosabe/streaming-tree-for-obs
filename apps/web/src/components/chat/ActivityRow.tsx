import { useTranslation } from 'react-i18next';

import type { OperatorChatItem, OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { cn } from '@/lib/cn';
import { activityTypeKey, userDisplayLabel } from '@/models/operator-chat-presentation';

import { ChatSourceLabel } from './ChatSourceLabel';

/**
 * A non-chat engagement event presented inline - visually distinguishable
 * from a chat message row (no bubble/fragment layout, a subdued
 * background), never described as proof the outgoing FFmpeg branch works
 * (a remote stream.online/offline notice is presented as exactly that:
 * remote).
 */
export function ActivityRow({
  item,
  preferences,
  accountLabel,
}: {
  item: OperatorChatItem;
  preferences: OperatorChatPreferences;
  accountLabel: string | null;
}) {
  const { t } = useTranslation('chat');
  const activity = item.activity;
  if (activity === undefined) return null;

  const key = activityTypeKey(activity.activityType);
  const label = key !== null ? t(key) : activity.activityType;
  const name = userDisplayLabel(item.user);
  const anonymous = item.user?.anonymous ?? false;
  const time = preferences.showTimestamps ? new Date(item.receivedAt).toLocaleTimeString() : null;

  return (
    <li
      className={cn(
        'flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 rounded-md bg-surface-raised/60 px-2 py-1',
        preferences.compactMode ? 'text-[11px]' : 'text-xs',
      )}
      data-testid="chat-activity-row"
    >
      <ChatSourceLabel item={item} preferences={preferences} accountLabel={accountLabel} />
      {time !== null && <span className="shrink-0 tabular-nums text-ink-faint">{time}</span>}
      <span className="shrink-0 rounded bg-accent-soft/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-accent-soft">
        {label}
      </span>
      {anonymous ? (
        <span className="italic text-ink-faint">{t('anonymous')}</span>
      ) : (
        name !== '' && <span className="font-medium text-ink">{name}</span>
      )}
      {activity.displayAmount !== undefined && (
        <span className="font-medium text-status-live">{activity.displayAmount}</span>
      )}
      {activity.quantity !== undefined && (
        <span className="text-ink-muted">{activity.quantity}</span>
      )}
    </li>
  );
}
