import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

import { StatusDot } from '@/components/ui/StatusBadge';
import { useAccountsQuery } from '@/hooks/use-accounts';

/**
 * Step 5: connected accounts (docs/onboarding.md §6.5).
 *
 * Explains the destination-vs-account distinction already established
 * in the domain model (docs/project-overview.md §8.1: a destination's
 * stream key is where the video is sent; a connected account authorizes
 * chat/event/account-aware functionality) and reuses `useAccountsQuery`
 * for the real list - linking to the existing account management UI in
 * Settings rather than embedding a second OAuth flow here.
 */
export function AccountsStep() {
  const { t } = useTranslation('onboarding');
  const accountsQuery = useAccountsQuery();
  const accounts = accountsQuery.data ?? [];

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">
        {t('accounts.heading')}
      </h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('accounts.distinction')}</p>

      {accounts.length === 0 ? (
        <p className="text-sm text-ink-faint">{t('accounts.empty')}</p>
      ) : (
        <ul className="space-y-1.5">
          {accounts.map((account) => (
            <li
              key={account.id}
              className="flex items-center justify-between gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm"
            >
              <span className="min-w-0 truncate text-ink">{account.displayName}</span>
              <span className="flex shrink-0 items-center gap-1.5 text-xs text-ink-muted">
                <StatusDot status={account.status === 'connected' ? 'live' : 'error'} />
                {account.status === 'connected'
                  ? t('accounts.connected')
                  : t('accounts.reconnectRequired')}
              </span>
            </li>
          ))}
        </ul>
      )}

      <Link
        to="/settings"
        className="inline-block text-sm font-medium text-accent-soft underline-offset-2 hover:underline"
      >
        {t('accounts.manageAction')}
      </Link>
    </div>
  );
}
