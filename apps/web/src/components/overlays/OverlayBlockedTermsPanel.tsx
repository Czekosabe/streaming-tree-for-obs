import { Plus, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChatOverlayMatchMode } from '@/api/chat-overlay-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import {
  useAddChatOverlayBlockedTermMutation,
  useChatOverlayBlockedTermsQuery,
  useRemoveChatOverlayBlockedTermMutation,
} from '@/hooks/use-chat-overlay';
import type { SelectOption } from '@/models/platform';

/**
 * This overlay's blocked terms - a match hides the complete message, never
 * a partial censor (Part 7), and this list itself is the only place the
 * configured term value is ever shown (the public API never returns it -
 * see internal/httpapi's own doc comment).
 */
export function OverlayBlockedTermsPanel({ overlayId }: { overlayId: string }) {
  const { t } = useTranslation('overlays');
  const termsQuery = useChatOverlayBlockedTermsQuery(overlayId);
  const add = useAddChatOverlayBlockedTermMutation(overlayId);
  const remove = useRemoveChatOverlayBlockedTermMutation(overlayId);

  const [value, setValue] = useState('');
  const [matchMode, setMatchMode] = useState<ChatOverlayMatchMode>('contains');

  const matchModeOptions: SelectOption[] = [
    { value: 'contains', label: t('blockedTerms.matchModeContains') },
    { value: 'whole_word', label: t('blockedTerms.matchModeWholeWord') },
  ];

  function handleAdd() {
    if (value.trim() === '') return;
    add.mutate({ value: value.trim(), matchMode }, { onSuccess: () => setValue('') });
  }

  return (
    <Panel>
      <PanelHeader title={t('blockedTerms.title')} description={t('blockedTerms.description')} />
      <PanelBody className="space-y-4">
        <ul className="space-y-1.5">
          {(termsQuery.data ?? []).map((term) => (
            <li key={term.id} className="flex items-center justify-between gap-2 rounded-lg border border-line px-3 py-1.5 text-sm">
              <span className="truncate text-ink">
                {term.value} <span className="text-xs text-ink-faint">({t(`blockedTerms.matchMode${term.matchMode === 'contains' ? 'Contains' : 'WholeWord'}`)})</span>
              </span>
              <IconButton label={t('blockedTerms.removeAction')} icon={<X className="size-3.5" />} variant="ghost" onClick={() => remove.mutate(term.id)} />
            </li>
          ))}
          {(termsQuery.data ?? []).length === 0 && <p className="text-xs text-ink-faint">{t('blockedTerms.empty')}</p>}
        </ul>

        {add.error !== null && (
          <p className="text-xs text-status-error">{t('blockedTerms.limitReached')}</p>
        )}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[2fr_1fr_auto] sm:items-end">
          <FormField label={t('blockedTerms.valuePlaceholder')}>
            {({ inputId }) => <TextInput id={inputId} value={value} onChange={(e) => setValue(e.target.value)} />}
          </FormField>
          <FormField label={t('blockedTerms.matchModeContains')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={matchModeOptions}
                value={matchMode}
                onChange={(e) => setMatchMode(e.target.value as ChatOverlayMatchMode)}
              />
            )}
          </FormField>
          <Button icon={<Plus className="size-3.5" />} onClick={handleAdd} disabled={add.isPending}>
            {t('blockedTerms.addAction')}
          </Button>
        </div>
      </PanelBody>
    </Panel>
  );
}
