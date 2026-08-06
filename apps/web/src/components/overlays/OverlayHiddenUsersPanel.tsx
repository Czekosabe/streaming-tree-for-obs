import { Plus, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button, IconButton } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { useAccountsQuery } from '@/hooks/use-accounts';
import {
  useAddChatOverlayHiddenUserMutation,
  useChatOverlayHiddenUsersQuery,
  useRemoveChatOverlayHiddenUserMutation,
} from '@/hooks/use-chat-overlay';
import type { SelectOption } from '@/models/platform';

/**
 * This overlay's own hidden-user list - deliberately separate from the
 * operator Chat page's own hidden-user list (Part 4: a user may be
 * visible to the operator while hidden here, or vice versa).
 */
export function OverlayHiddenUsersPanel({ overlayId }: { overlayId: string }) {
  const { t } = useTranslation('overlays');
  const accountsQuery = useAccountsQuery();
  const hiddenQuery = useChatOverlayHiddenUsersQuery(overlayId);
  const add = useAddChatOverlayHiddenUserMutation(overlayId);
  const remove = useRemoveChatOverlayHiddenUserMutation(overlayId);

  const [connectedAccountId, setConnectedAccountId] = useState('');
  const [providerUserId, setProviderUserId] = useState('');
  const [label, setLabel] = useState('');

  const accountOptions: SelectOption[] = (accountsQuery.data ?? []).map((account) => ({
    value: account.id,
    label: account.displayName,
  }));

  function handleAdd() {
    if (connectedAccountId === '' || providerUserId.trim() === '') return;
    const trimmedLabel = label.trim();
    add.mutate(
      {
        providerId: 'twitch',
        connectedAccountId,
        providerUserId: providerUserId.trim(),
        ...(trimmedLabel === '' ? {} : { label: trimmedLabel }),
      },
      { onSuccess: () => setProviderUserId('') },
    );
  }

  return (
    <Panel>
      <PanelHeader title={t('hiddenUsers.title')} description={t('hiddenUsers.description')} />
      <PanelBody className="space-y-4">
        <ul className="space-y-1.5">
          {(hiddenQuery.data ?? []).map((user) => (
            <li key={`${user.connectedAccountId}-${user.providerUserId}`} className="flex items-center justify-between gap-2 rounded-lg border border-line px-3 py-1.5 text-sm">
              <span className="truncate text-ink">
                {user.label !== undefined && user.label !== '' ? user.label : user.providerUserId}
              </span>
              <IconButton
                label={t('hiddenUsers.removeAction')}
                icon={<X className="size-3.5" />}
                variant="ghost"
                onClick={() =>
                  remove.mutate({ providerId: user.providerId, connectedAccountId: user.connectedAccountId, providerUserId: user.providerUserId })
                }
              />
            </li>
          ))}
          {(hiddenQuery.data ?? []).length === 0 && <p className="text-xs text-ink-faint">{t('hiddenUsers.empty')}</p>}
        </ul>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_1fr_auto] sm:items-end">
          <FormField label={t('hiddenUsers.connectedAccount')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={[{ value: '', label: '' }, ...accountOptions]}
                value={connectedAccountId}
                onChange={(e) => setConnectedAccountId(e.target.value)}
              />
            )}
          </FormField>
          <FormField label={t('hiddenUsers.providerUserId')}>
            {({ inputId }) => (
              <TextInput id={inputId} value={providerUserId} onChange={(e) => setProviderUserId(e.target.value)} />
            )}
          </FormField>
          <FormField label={t('hiddenUsers.label')}>
            {({ inputId }) => <TextInput id={inputId} value={label} onChange={(e) => setLabel(e.target.value)} />}
          </FormField>
          <Button icon={<Plus className="size-3.5" />} onClick={handleAdd} disabled={add.isPending}>
            {t('hiddenUsers.addAction')}
          </Button>
        </div>
      </PanelBody>
    </Panel>
  );
}
