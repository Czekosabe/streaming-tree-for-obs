import { Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChatOverlayProfile } from '@/api/chat-overlay-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { TextInput } from '@/components/ui/TextInput';
import { useCreateChatOverlayMutation, useDeleteChatOverlayMutation } from '@/hooks/use-chat-overlay';
import { cn } from '@/lib/cn';

type OverlayListPanelProps = {
  overlays: ChatOverlayProfile[];
  selectedId: string | null;
  onSelect: (id: string) => void;
};

export function OverlayListPanel({ overlays, selectedId, onSelect }: OverlayListPanelProps) {
  const { t } = useTranslation('overlays');
  const [name, setName] = useState('');
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const create = useCreateChatOverlayMutation();
  const remove = useDeleteChatOverlayMutation();

  function handleCreate() {
    if (name.trim() === '') return;
    create.mutate(name.trim(), {
      onSuccess: (overlay) => {
        setName('');
        onSelect(overlay.id);
      },
    });
  }

  const pendingDelete = overlays.find((overlay) => overlay.id === pendingDeleteId) ?? null;

  return (
    <Panel>
      <PanelHeader title={t('page.title')} description={t('page.description')} />
      <PanelBody className="space-y-4">
        <div className="flex gap-2">
          <TextInput
            aria-label={t('list.createPlaceholder')}
            placeholder={t('list.createPlaceholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
          />
          <Button icon={<Plus className="size-3.5" />} onClick={handleCreate} disabled={create.isPending}>
            {t('list.createAction')}
          </Button>
        </div>

        {overlays.length === 0 && <p className="text-xs text-ink-faint">{t('list.empty')}</p>}

        <ul className="space-y-1.5">
          {overlays.map((overlay) => (
            <li key={overlay.id}>
              <div
                className={cn(
                  'flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-sm transition-colors',
                  overlay.id === selectedId
                    ? 'border-accent/50 bg-accent/10 text-ink'
                    : 'border-line text-ink-muted hover:bg-surface-hover hover:text-ink',
                )}
              >
                <button type="button" className="min-w-0 flex-1 truncate text-left" onClick={() => onSelect(overlay.id)}>
                  {overlay.name}
                  <span
                    className={cn(
                      'ml-2 rounded px-1 py-0 text-[10px] font-semibold uppercase tracking-wide',
                      overlay.enabled ? 'text-status-live' : 'text-ink-faint',
                    )}
                  >
                    {overlay.enabled ? t('list.enabledBadge') : t('list.disabledBadge')}
                  </span>
                </button>
                <IconButton
                  label={t('list.deleteAction')}
                  icon={<Trash2 className="size-3.5" />}
                  variant="ghost"
                  onClick={() => setPendingDeleteId(overlay.id)}
                />
              </div>
            </li>
          ))}
        </ul>
      </PanelBody>

      <ConfirmDialog
        open={pendingDelete !== null}
        title={t('list.deleteConfirmTitle')}
        message={t('list.deleteConfirmBody')}
        confirmLabel={t('list.deleteConfirmAction')}
        destructive
        busy={remove.isPending}
        onCancel={() => setPendingDeleteId(null)}
        onConfirm={() => {
          if (pendingDelete === null) return;
          remove.mutate(pendingDelete.id, { onSuccess: () => setPendingDeleteId(null) });
        }}
      />
    </Panel>
  );
}
