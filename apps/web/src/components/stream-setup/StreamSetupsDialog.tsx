import { Check, Copy, Layers, Loader2, Pencil, Play, Plus, Save, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import type { StreamSetupProfile } from '@/api/stream-setup-schemas';
import { Button, IconButton } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Modal } from '@/components/ui/Modal';
import { TextInput } from '@/components/ui/TextInput';
import { useMetadataPresetsQuery } from '@/hooks/use-metadata-presets';
import {
  useDeleteStreamSetupMutation,
  useDuplicateStreamSetupMutation,
  useStreamSetupsQuery,
} from '@/hooks/use-stream-setups';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

import { ProviderBrand } from '../providers/ProviderBrand';
import { ApplyStreamSetupDialog } from './ApplyStreamSetupDialog';
import { SaveCurrentSetupDialog } from './SaveCurrentSetupDialog';
import { StreamSetupFormDialog } from './StreamSetupFormDialog';

type StreamSetupsDialogProps = {
  open: boolean;
  onClose: () => void;
  platforms: readonly ConfiguredPlatform[];
  /** The destination tab currently open in Stream details, if any. */
  activeMetadataId: string | null;
  /** Whether that open tab has unsaved edits right now. */
  activeMetadataDirty: boolean;
};

/**
 * List/create/edit/duplicate/delete/apply surface for stream setup
 * profiles (docs/stream-setup-profiles.md §14/§18) - the Dashboard's
 * entry point into Stage 25. Deliberately one compact dialog rather
 * than a dedicated page: this is a local, personal list of reusable
 * show configurations, the same scale and spirit as the Stage 22
 * metadata-preset manager it sits next to.
 */
export function StreamSetupsDialog({
  open,
  onClose,
  platforms,
  activeMetadataId,
  activeMetadataDirty,
}: StreamSetupsDialogProps) {
  const { t } = useTranslation(['streamSetups', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const setupsQuery = useStreamSetupsQuery();
  const metadataPresetsQuery = useMetadataPresetsQuery();
  const deleteSetup = useDeleteStreamSetupMutation();
  const duplicateSetup = useDuplicateStreamSetupMutation();

  const [formOpen, setFormOpen] = useState(false);
  const [editingProfile, setEditingProfile] = useState<StreamSetupProfile | null>(null);
  const [saveCurrentOpen, setSaveCurrentOpen] = useState(false);
  const [applyingProfile, setApplyingProfile] = useState<StreamSetupProfile | null>(null);
  const [deletingProfile, setDeletingProfile] = useState<StreamSetupProfile | null>(null);
  const [duplicatingProfile, setDuplicatingProfile] = useState<StreamSetupProfile | null>(null);
  const [duplicateName, setDuplicateName] = useState('');

  const setups = setupsQuery.data ?? [];
  const metadataPresets = metadataPresetsQuery.data ?? [];
  const busy = deleteSetup.isPending || duplicateSetup.isPending;

  const handleClose = () => {
    if (busy) return;
    setDeletingProfile(null);
    setDuplicatingProfile(null);
    deleteSetup.reset();
    duplicateSetup.reset();
    onClose();
  };

  const openCreate = () => {
    setEditingProfile(null);
    setFormOpen(true);
  };

  const openEdit = (profile: StreamSetupProfile) => {
    setEditingProfile(profile);
    setFormOpen(true);
  };

  const startDuplicate = (profile: StreamSetupProfile) => {
    setDuplicatingProfile(profile);
    setDuplicateName(t('streamSetups:manage.copyName', { name: profile.name }));
    duplicateSetup.reset();
  };

  const confirmDuplicate = () => {
    if (duplicatingProfile === null || duplicateName.trim().length === 0) return;
    duplicateSetup.mutate(
      { id: duplicatingProfile.id, name: duplicateName.trim() },
      { onSuccess: () => setDuplicatingProfile(null) },
    );
  };

  const handleDelete = () => {
    if (deletingProfile === null) return;
    deleteSetup.mutate(deletingProfile.id, { onSuccess: () => setDeletingProfile(null) });
  };

  const deleteFailure = deleteSetup.error !== null ? resolveApiErrorMessage(tErrors, deleteSetup.error) : null;
  const duplicateFailure =
    duplicateSetup.error !== null ? resolveApiErrorMessage(tErrors, duplicateSetup.error) : null;

  return (
    <>
      <Modal
        open={open}
        onClose={handleClose}
        title={t('streamSetups:manage.title')}
        description={t('streamSetups:manage.description')}
        dismissible={!busy}
        footer={
          <>
            <Button type="button" icon={<Save className="size-3.5" />} onClick={() => setSaveCurrentOpen(true)}>
              {t('streamSetups:manage.saveCurrent')}
            </Button>
            <Button type="button" variant="primary" icon={<Plus className="size-3.5" />} onClick={openCreate}>
              {t('streamSetups:manage.new')}
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          {(deleteFailure ?? duplicateFailure) !== null && (
            <p
              role="alert"
              className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
            >
              {deleteFailure ?? duplicateFailure}
            </p>
          )}

          {setupsQuery.isLoading ? (
            <p className="p-3 text-sm text-ink-muted">{t('streamSetups:manage.loading')}</p>
          ) : setups.length === 0 ? (
            <div className="rounded-lg bg-surface-sunken/70 px-4 py-8 text-center">
              <Layers aria-hidden="true" className="mx-auto size-6 text-ink-faint" />
              <p className="mt-3 text-sm font-medium text-ink">{t('streamSetups:manage.emptyTitle')}</p>
              <p className="mx-auto mt-1 max-w-sm text-xs text-ink-faint">
                {t('streamSetups:manage.emptyDescription')}
              </p>
            </div>
          ) : (
            <ul className="divide-y divide-line rounded-lg border border-line">
              {setups.map((profile) => {
                const isDuplicating = duplicatingProfile?.id === profile.id;

                return (
                  <li key={profile.id} className="p-3">
                    {isDuplicating ? (
                      <div className="flex items-center gap-2">
                        <TextInput
                          aria-label={t('streamSetups:manage.duplicateNameLabel')}
                          value={duplicateName}
                          maxLength={100}
                          autoFocus
                          disabled={duplicateSetup.isPending}
                          onChange={(event) => setDuplicateName(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                              event.preventDefault();
                              confirmDuplicate();
                            } else if (event.key === 'Escape') {
                              setDuplicatingProfile(null);
                            }
                          }}
                        />
                        <IconButton
                          label={t('streamSetups:manage.save')}
                          icon={
                            duplicateSetup.isPending ? (
                              <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                              <Check className="size-3.5" />
                            )
                          }
                          disabled={duplicateSetup.isPending || duplicateName.trim().length === 0}
                          onClick={confirmDuplicate}
                        />
                        <IconButton
                          label={t('common:actions.cancel')}
                          icon={<X className="size-3.5" />}
                          disabled={duplicateSetup.isPending}
                          onClick={() => setDuplicatingProfile(null)}
                        />
                      </div>
                    ) : (
                      <div className="flex items-center gap-3">
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-ink">{profile.name}</p>
                          {profile.note.length > 0 && (
                            <p className="mt-0.5 truncate text-xs text-ink-muted">{profile.note}</p>
                          )}
                          {profile.metadataPresetMissing && (
                            <p className="mt-1 text-[11px] font-medium text-status-warning">
                              {t('streamSetups:manage.presetMissing')}
                            </p>
                          )}
                        </div>

                        {profile.destinations.length > 0 && (
                          <div className="flex shrink-0 items-center gap-1">
                            {profile.destinations.map((destination, index) => (
                              <ProviderBrand
                                key={`${destination.platformId ?? 'missing'}-${index}`}
                                providerId={destination.providerId}
                                fallbackLabel={destination.providerId.slice(0, 2).toUpperCase()}
                                size="sm"
                              />
                            ))}
                          </div>
                        )}

                        <div className="flex shrink-0 items-center gap-1">
                          <IconButton
                            label={t('streamSetups:manage.apply')}
                            icon={<Play className="size-3.5" />}
                            disabled={busy}
                            onClick={() => setApplyingProfile(profile)}
                          />
                          <IconButton
                            label={t('streamSetups:manage.edit')}
                            icon={<Pencil className="size-3.5" />}
                            disabled={busy}
                            onClick={() => openEdit(profile)}
                          />
                          <IconButton
                            label={t('streamSetups:manage.duplicate')}
                            icon={<Copy className="size-3.5" />}
                            disabled={busy}
                            onClick={() => startDuplicate(profile)}
                          />
                          <IconButton
                            label={t('streamSetups:manage.delete')}
                            icon={<Trash2 className="size-3.5" />}
                            variant="danger"
                            disabled={busy}
                            onClick={() => setDeletingProfile(profile)}
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
        open={deletingProfile !== null}
        title={t('streamSetups:manage.deleteDialog.title')}
        message={t('streamSetups:manage.deleteDialog.message', { setup: deletingProfile?.name ?? '' })}
        confirmLabel={t('streamSetups:manage.deleteDialog.confirm')}
        destructive
        busy={deleteSetup.isPending}
        onConfirm={handleDelete}
        onCancel={() => setDeletingProfile(null)}
      />

      <StreamSetupFormDialog
        open={formOpen}
        onClose={() => setFormOpen(false)}
        platforms={platforms}
        metadataPresets={metadataPresets}
        editing={editingProfile}
      />

      <SaveCurrentSetupDialog
        open={saveCurrentOpen}
        onClose={() => setSaveCurrentOpen(false)}
        metadataPresets={metadataPresets}
      />

      {applyingProfile !== null && (
        <ApplyStreamSetupDialog
          open
          onClose={() => setApplyingProfile(null)}
          profile={applyingProfile}
          activeMetadataId={activeMetadataId}
          activeMetadataDirty={activeMetadataDirty}
        />
      )}
    </>
  );
}
