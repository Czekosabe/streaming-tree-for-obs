import { useTranslation } from 'react-i18next';

import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { useAccountsQuery } from '@/hooks/use-accounts';
import { useChatOverlayAccountsQuery, useSetChatOverlayAccountsMutation } from '@/hooks/use-chat-overlay';

/**
 * Which connected accounts this overlay shows - an empty selection means
 * "all accounts" (Part 4's own documented default). Each toggle saves
 * immediately, mirroring the operator Chat page's own per-account
 * visibility toggle (there is no separate "save" step for this list).
 */
export function OverlayAccountsPanel({ overlayId }: { overlayId: string }) {
  const { t } = useTranslation('overlays');
  const accountsQuery = useAccountsQuery();
  const selectedQuery = useChatOverlayAccountsQuery(overlayId);
  const setAccounts = useSetChatOverlayAccountsMutation(overlayId);

  const selected = new Set(selectedQuery.data ?? []);

  function toggle(accountId: string, checked: boolean) {
    const next = new Set(selected);
    if (checked) next.add(accountId);
    else next.delete(accountId);
    setAccounts.mutate(Array.from(next));
  }

  return (
    <Panel>
      <PanelHeader title={t('accounts.title')} description={t('accounts.description')} />
      <PanelBody className="space-y-2">
        {selected.size === 0 && <p className="text-xs text-ink-faint">{t('accounts.allAccounts')}</p>}
        {(accountsQuery.data ?? []).map((account) => (
          <label key={account.id} className="flex cursor-pointer items-center gap-2 text-sm text-ink">
            <input
              type="checkbox"
              checked={selected.has(account.id)}
              onChange={(event) => toggle(account.id, event.target.checked)}
              className="size-4 rounded border-line accent-accent"
            />
            {account.displayName} <span className="text-xs text-ink-faint">({account.providerId})</span>
          </label>
        ))}
      </PanelBody>
    </Panel>
  );
}
