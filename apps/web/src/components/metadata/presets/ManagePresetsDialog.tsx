import { BookmarkX, Check, Loader2, Pencil, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { MetadataPreset } from '@/api/metadata-preset-schemas';
import { IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Modal } from '@/components/ui/Modal';
import { TextInput } from '@/components/ui/TextInput';
import {
  useDeleteMetadataPresetMutation,
  useMetadataPresetsQuery,
  useUpdateMetadataPresetMutation,
} from '@/hooks/use-metadata-presets';
import { useLanguage } from '@/i18n/use-language';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { formatTimestamp } from '@/lib/format';
import { NAME_MAX_LENGTH } from '@/models/metadata-preset-constraints';

import { ProviderBrand } from '../../providers/ProviderBrand';

type ManagePresetsDialogProps = {
  open: boolean;
  onClose: () => void;
};

/**
 * List/rename/delete surface for saved presets (docs/metadata-presets.md §7).
 *
 * Deliberately compact: no folders, tags, search or sort controls - the
 * governing scope caps preset count at 200 and this is a local, personal
 * list, not a shared library. Applying a preset to a destination is a
 * separate workflow (22C), not part of this dialog.
 */
export function ManagePresetsDialog({ open, onClose }: ManagePresetsDialogProps) {
  const { t } = useTranslation(['metadataPresets', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const { locale } = useLanguage();

  const presetsQuery = useMetadataPresetsQuery();
  const updatePreset = useUpdateMetadataPresetMutation();
  const deletePreset = useDeleteMetadataPresetMutation();

  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [deletingPreset, setDeletingPreset] = useState<MetadataPreset | null>(null);

  const presets = presetsQuery.data ?? [];
  const busy = updatePreset.isPending || deletePreset.isPending;

  const handleClose = () => {
    if (busy) return;
    setEditingId(null);
    setDeletingPreset(null);
    updatePreset.reset();
    deletePreset.reset();
    onClose();
  };

  const startEdit = (preset: MetadataPreset) => {
    setEditingId(preset.id);
    setEditName(preset.name);
    updatePreset.reset();
  };

  const cancelEdit = () => {
    setEditingId(null);
    updatePreset.reset();
  };

  const saveEdit = (preset: MetadataPreset) => {
    const trimmed = editName.trim();
    if (trimmed.length === 0) return;

    updatePreset.mutate(
      {
        id: preset.id,
        input: {
          name: trimmed,
          note: preset.note,
          title: preset.title,
          description: preset.description,
          tags: preset.tags,
          language: preset.language,
          visibility: preset.visibility,
          matureContent: preset.matureContent,
          dvr: preset.dvr,
          latencyMode: preset.latencyMode,
          providers: preset.providers,
        },
      },
      { onSuccess: () => setEditingId(null) },
    );
  };

  const handleDelete = () => {
    if (deletingPreset === null) return;
    deletePreset.mutate(deletingPreset.id, {
      onSuccess: () => setDeletingPreset(null),
    });
  };

  const updateFailure =
    updatePreset.error !== null ? resolveApiErrorMessage(tErrors, updatePreset.error) : null;
  const deleteFailure =
    deletePreset.error !== null ? resolveApiErrorMessage(tErrors, deletePreset.error) : null;

  return (
    <>
      <Modal
        open={open}
        onClose={handleClose}
        title={t('metadataPresets:manage.title')}
        description={t('metadataPresets:manage.description')}
        dismissible={!busy}
      >
        <div className="space-y-3">
          {(updateFailure ?? deleteFailure) !== null && (
            <p
              role="alert"
              className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
            >
              {updateFailure ?? deleteFailure}
            </p>
          )}

          {presetsQuery.isLoading ? (
            <p className="p-3 text-sm text-ink-muted">{t('metadataPresets:manage.loading')}</p>
          ) : presets.length === 0 ? (
            <div className="rounded-lg bg-surface-sunken/70 px-4 py-8 text-center">
              <BookmarkX aria-hidden="true" className="mx-auto size-6 text-ink-faint" />
              <p className="mt-3 text-sm font-medium text-ink">{t('metadataPresets:manage.emptyTitle')}</p>
              <p className="mx-auto mt-1 max-w-sm text-xs text-ink-faint">
                {t('metadataPresets:manage.emptyDescription')}
              </p>
            </div>
          ) : (
            <ul className="divide-y divide-line rounded-lg border border-line">
              {presets.map((preset) => {
                const providerIds = Object.keys(preset.providers);
                const isEditing = editingId === preset.id;

                return (
                  <li key={preset.id} className="p-3">
                    {isEditing ? (
                      <div className="flex items-center gap-2">
                        <TextInput
                          aria-label={t('metadataPresets:fields.nameLabel')}
                          value={editName}
                          maxLength={NAME_MAX_LENGTH}
                          autoFocus
                          disabled={updatePreset.isPending}
                          onChange={(event) => setEditName(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                              event.preventDefault();
                              saveEdit(preset);
                            } else if (event.key === 'Escape') {
                              cancelEdit();
                            }
                          }}
                        />
                        <IconButton
                          label={t('metadataPresets:manage.save')}
                          icon={
                            updatePreset.isPending ? (
                              <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                              <Check className="size-3.5" />
                            )
                          }
                          disabled={updatePreset.isPending || editName.trim().length === 0}
                          onClick={() => saveEdit(preset)}
                        />
                        <IconButton
                          label={t('common:actions.cancel')}
                          icon={<X className="size-3.5" />}
                          disabled={updatePreset.isPending}
                          onClick={cancelEdit}
                        />
                      </div>
                    ) : (
                      <div className="flex items-center gap-3">
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-ink">{preset.name}</p>
                          {preset.note.length > 0 && (
                            <p className="mt-0.5 truncate text-xs text-ink-muted">{preset.note}</p>
                          )}
                          <p className="mt-1 text-[11px] text-ink-faint">
                            {t('metadataPresets:manage.updatedAt', {
                              date: formatTimestamp(preset.updatedAt, locale),
                            })}
                          </p>
                        </div>

                        {providerIds.length > 0 && (
                          <div className="flex shrink-0 items-center gap-1">
                            {providerIds.map((providerId) => (
                              <ProviderBrand
                                key={providerId}
                                providerId={providerId}
                                fallbackLabel={providerId.slice(0, 2).toUpperCase()}
                                size="sm"
                              />
                            ))}
                          </div>
                        )}

                        <div className="flex shrink-0 items-center gap-1">
                          <IconButton
                            label={t('metadataPresets:manage.rename')}
                            icon={<Pencil className="size-3.5" />}
                            disabled={busy}
                            onClick={() => startEdit(preset)}
                          />
                          <IconButton
                            label={t('metadataPresets:manage.delete')}
                            icon={<Trash2 className="size-3.5" />}
                            variant="danger"
                            disabled={busy}
                            onClick={() => setDeletingPreset(preset)}
                          />
                        </div>
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </Modal>

      <ConfirmDialog
        open={deletingPreset !== null}
        title={t('metadataPresets:manage.deleteDialog.title')}
        message={t('metadataPresets:manage.deleteDialog.message', {
          preset: deletingPreset?.name ?? '',
        })}
        confirmLabel={t('metadataPresets:manage.deleteDialog.confirm')}
        destructive
        busy={deletePreset.isPending}
        onConfirm={handleDelete}
        onCancel={() => setDeletingPreset(null)}
      />
    </>
  );
}
