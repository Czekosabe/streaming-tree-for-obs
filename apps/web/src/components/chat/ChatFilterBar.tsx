import { useTranslation } from 'react-i18next';

import type { ConnectedAccount } from '@/api/account-schemas';
import { Button } from '@/components/ui/Button';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';

export type ChatFilterBarProps = {
  accounts: ConnectedAccount[];
  isAccountVisible: (accountId: string) => boolean;
  onSetAccountVisible: (accountId: string, visible: boolean) => void;
  hideBotMessages: boolean;
  onToggleHideBotMessages: (value: boolean) => void;
  onResetAll: () => void;
};

/** Account and bot-message filters - see the Stage 9 task's Part 19.
 * Filtering never mutates backend chat history; account visibility is
 * persisted immediately per toggle since each click is already one
 * complete, atomic operator decision. */
export function ChatFilterBar({
  accounts,
  isAccountVisible,
  onSetAccountVisible,
  hideBotMessages,
  onToggleHideBotMessages,
  onResetAll,
}: ChatFilterBarProps) {
  const { t } = useTranslation('chat');

  return (
    <div className="space-y-3 text-xs">
      {accounts.length > 1 && (
        <div>
          <p className="mb-1.5 font-medium text-ink">{t('filters.accounts')}</p>
          <ul className="space-y-1.5">
            {accounts.map((account) => (
              <li key={account.id}>
                <ToggleSwitch
                  label={account.displayName !== '' ? account.displayName : account.login}
                  checked={isAccountVisible(account.id)}
                  onCheckedChange={(checked) => onSetAccountVisible(account.id, checked)}
                />
              </li>
            ))}
          </ul>
        </div>
      )}

      <ToggleSwitch
        label={t('filters.hideBots')}
        checked={hideBotMessages}
        onCheckedChange={onToggleHideBotMessages}
      />

      <Button variant="ghost" size="sm" onClick={onResetAll}>
        {t('filters.resetAction')}
      </Button>
    </div>
  );
}
