import { EyeOff, Bot } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { OperatorChatItem, OperatorChatPreferences } from '@/api/operator-chat-schemas';
import { IconButton } from '@/components/ui/Button';
import { cn } from '@/lib/cn';
import {
  deletionReasonKey,
  isCommandMessage,
  userDisplayLabel,
} from '@/models/operator-chat-presentation';

import { ChatBadgeImage } from './ChatBadgeImage';
import { ChatEmoteImage } from './ChatEmoteImage';
import { ChatSourceLabel } from './ChatSourceLabel';

type MessageRowProps = {
  item: OperatorChatItem;
  preferences: OperatorChatPreferences;
  accountLabel: string | null;
  onHideUser?: (() => void) | undefined;
  onMarkBot?: (() => void) | undefined;
};

/** Chat-shaped item row: identity, badges, ordered fragments, and lifecycle
 * (deleted) state. Never uses dangerouslySetInnerHTML - every fragment is
 * rendered from typed, validated data, never raw HTML. */
export function MessageRow({ item, preferences, accountLabel, onHideUser, onMarkBot }: MessageRowProps) {
  const { t } = useTranslation('chat');
  const message = item.message;
  if (message === undefined) return null;

  const deleted = item.lifecycle.deleted;
  const name = userDisplayLabel(item.user);
  const anonymous = item.user?.anonymous ?? false;
  const isCommand = !deleted && isCommandMessage(message.plainText);
  const reasonKey = deleted ? deletionReasonKey(item.lifecycle.deletionReason ?? '') : null;
  const time = preferences.showTimestamps
    ? new Date(item.receivedAt).toLocaleTimeString()
    : null;

  return (
    <li
      className={cn(
        'group rounded-md border border-transparent px-2 py-1.5 hover:border-line/60',
        preferences.compactMode ? 'text-[11px] leading-snug' : 'text-xs leading-relaxed',
      )}
      data-testid="chat-message-row"
    >
      <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5">
        <ChatSourceLabel item={item} preferences={preferences} accountLabel={accountLabel} />
        {time !== null && <span className="shrink-0 tabular-nums text-ink-faint">{time}</span>}

        {preferences.showBadges &&
          item.user?.badges?.map((badge, index) => (
            <ChatBadgeImage key={`${badge.setId}-${badge.id}-${index}`} badge={badge} />
          ))}

        {anonymous ? (
          <span className="shrink-0 italic text-ink-faint">{t('anonymous')}</span>
        ) : (
          name !== '' && (
            <span
              className="shrink-0 font-semibold"
              style={item.user?.color !== undefined ? { color: item.user.color } : undefined}
            >
              {name}
            </span>
          )
        )}

        {isCommand && (
          <span className="shrink-0 rounded bg-surface-raised px-1 py-0 text-[10px] text-ink-faint">
            {t('command')}
          </span>
        )}
        {item.synthetic && (
          <span className="shrink-0 rounded bg-surface-raised px-1 py-0 text-[10px] text-ink-faint">
            {t('synthetic')}
          </span>
        )}

        {(onHideUser !== undefined || onMarkBot !== undefined) &&
          !anonymous &&
          item.user?.providerUserId !== undefined && (
            <span className="ml-auto hidden shrink-0 items-center gap-1 group-hover:inline-flex">
              {onHideUser !== undefined && (
                <IconButton
                  label={t('filters.hideUserAction')}
                  icon={<EyeOff className="size-3.5" />}
                  variant="ghost"
                  className="size-6"
                  onClick={onHideUser}
                />
              )}
              {onMarkBot !== undefined && (
                <IconButton
                  label={t('filters.markBotAction')}
                  icon={<Bot className="size-3.5" />}
                  variant="ghost"
                  className="size-6"
                  onClick={onMarkBot}
                />
              )}
            </span>
          )}
      </div>

      <div className={cn('mt-0.5 whitespace-pre-wrap wrap-break-word text-ink', deleted && 'italic text-ink-faint line-through')}>
        {message.fragments.length === 0 ? (
          message.plainText
        ) : (
          message.fragments.map((fragment, index) => {
            switch (fragment.type) {
              case 'emote':
                return (
                  <ChatEmoteImage key={index} url={fragment.emoteImageUrl} text={fragment.text} />
                );
              case 'mention':
                return (
                  <span key={index} className="text-accent-soft">
                    {fragment.text}
                  </span>
                );
              default:
                return <span key={index}>{fragment.text}</span>;
            }
          })
        )}
      </div>

      {deleted && (
        <p className="mt-0.5 text-[10px] text-status-error">
          {t('deletedMessagePlaceholder')}
          {reasonKey !== null && ` — ${t(reasonKey)}`}
        </p>
      )}
    </li>
  );
}
