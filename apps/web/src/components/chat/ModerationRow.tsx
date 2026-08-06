import { useTranslation } from 'react-i18next';

import type { OperatorChatItem, OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { cn } from '@/lib/cn';
import { moderationActionKey } from '@/models/operator-chat-presentation';

/**
 * A moderation or system row - visually and semantically distinct from
 * both a chat message and an activity item: a centered divider-style line,
 * never mistaken for something a viewer said.
 */
export function ModerationRow({
  item,
  preferences,
}: {
  item: OperatorChatItem;
  preferences: OperatorChatPreferences;
}) {
  const { t } = useTranslation('chat');
  const moderation = item.moderation;
  if (moderation === undefined) return null;

  const key = moderationActionKey(moderation.action);
  const label = key !== null ? t(key) : moderation.action;
  const time = preferences.showTimestamps ? new Date(item.receivedAt).toLocaleTimeString() : null;

  return (
    <li
      className={cn(
        'flex items-center justify-center gap-1.5 py-1 text-center text-[11px] italic text-ink-faint',
      )}
      data-testid="chat-moderation-row"
    >
      {time !== null && <span className="tabular-nums">{time}</span>}
      <span>{label}</span>
    </li>
  );
}
