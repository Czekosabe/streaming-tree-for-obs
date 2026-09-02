import { Check, Loader2 } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import type { MetadataPreset } from '@/api/metadata-preset-schemas';
import type { StreamSetupProfile } from '@/api/stream-setup-schemas';
import { Button } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextArea, TextInput } from '@/components/ui/TextInput';
import { useCreateStreamSetupMutation, useUpdateStreamSetupMutation } from '@/hooks/use-stream-setups';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { NAME_MAX_LENGTH, NOTE_MAX_LENGTH } from '@/models/stream-setup-constraints';

import { ProviderBrand } from '../providers/ProviderBrand';

type StreamSetupFormDialogProps = {
  open: boolean;
  onClose: () => void;
  platforms: readonly ConfiguredPlatform[];
  metadataPresets: readonly MetadataPreset[];
  /** null creates a new profile; otherwise the profile being edited. */
  editing: StreamSetupProfile | null;
};

const NO_PRESET = '';

/**
 * Create/edit form for one stream setup profile (docs/stream-setup-
 * profiles.md §13/§14): name, note, the intended destination set, and
 * an optional metadata preset reference. Never touches credentials,
 * never starts anything - saving only writes this profile's own row.
 */
export function StreamSetupFormDialog({
  open,
  onClose,
  platforms,
  metadataPresets,
  editing,
}: StreamSetupFormDialogProps) {
  const { t } = useTranslation(['streamSetups', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const createSetup = useCreateStreamSetupMutation();
  const updateSetup = useUpdateStreamSetupMutation();

  const [name, setName] = useState('');
  const [note, setNote] = useState('');
  const [destinationIds, setDestinationIds] = useState<string[]>([]);
  const [metadataPresetId, setMetadataPresetId] = useState<string>(NO_PRESET);

  const isEditing = editing !== null;
  const mutation = isEditing ? updateSetup : createSetup;
  const busy = mutation.isPending;

  // Reset the form to the editing target (or a blank slate) each time
  // the dialog opens fresh, so a previous session's leftover draft
  // never bleeds into the next.
  useEffect(() => {
    if (!open) return;
    if (editing !== null) {
      setName(editing.name);
      setNote(editing.note);
      setDestinationIds(editing.destinations.flatMap((d) => (d.platformId !== null ? [d.platformId] : [])));
      setMetadataPresetId(editing.metadataPresetId ?? NO_PRESET);
    } else {
      setName('');
      setNote('');
      setDestinationIds([]);
      setMetadataPresetId(NO_PRESET);
    }
    createSetup.reset();
    updateSetup.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, editing]);

  const handleClose = () => {
    if (busy) return;
    onClose();
  };

  const toggleDestination = (id: string, checked: boolean) => {
    setDestinationIds((current) => (checked ? [...current, id] : current.filter((x) => x !== id)));
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy || name.trim().length === 0) return;

    const input = {
      name: name.trim(),
      note,
      destinationIds,
      metadataPresetId: metadataPresetId === NO_PRESET ? null : metadataPresetId,
    };

    if (editing !== null) {
      updateSetup.mutate({ id: editing.id, input }, { onSuccess: () => onClose() });
    } else {
      createSetup.mutate(input, { onSuccess: () => onClose() });
    }
  };

  const generalError = mutation.error !== null ? resolveApiErrorMessage(tErrors, mutation.error) : null;

  const presetOptions = [
    { value: NO_PRESET, label: t('streamSetups:fields.noPreset') },
    ...metadataPresets.map((preset) => ({ value: preset.id, label: preset.name })),
  ];

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={isEditing ? t('streamSetups:form.editTitle') : t('streamSetups:form.createTitle')}
      description={t('streamSetups:form.description')}
      dismissible={!busy}
      footer={
        <>
          <Button type="button" onClick={handleClose} disabled={busy}>
            {t('common:actions.cancel')}
          </Button>
          <Button
            type="submit"
            form="stream-setup-form"
            variant="primary"
            disabled={busy || name.trim().length === 0}
            icon={busy ? <Loader2 className="size-3.5 animate-spin" /> : <Check className="size-3.5" />}
          >
            {busy ? t('streamSetups:form.saving') : t('streamSetups:form.submit')}
          </Button>
        </>
      }
    >
      <form id="stream-setup-form" onSubmit={handleSubmit} noValidate className="space-y-4">
        {generalError !== null && (
          <p
            role="alert"
            className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
          >
            {generalError}
          </p>
        )}

        <FormField
          label={t('streamSetups:fields.nameLabel')}
          counter={`${name.trim().length} / ${NAME_MAX_LENGTH}`}
        >
          {({ inputId, describedBy }) => (
            <TextInput
              id={inputId}
              aria-describedby={describedBy}
              value={name}
              maxLength={NAME_MAX_LENGTH}
              disabled={busy}
              autoFocus
              placeholder={t('streamSetups:fields.namePlaceholder')}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </FormField>

        <FormField
          label={t('streamSetups:fields.noteLabel')}
          hint={t('streamSetups:fields.noteHint')}
          counter={`${note.length} / ${NOTE_MAX_LENGTH}`}
        >
          {({ inputId, describedBy }) => (
            <TextArea
              id={inputId}
              aria-describedby={describedBy}
              value={note}
              maxLength={NOTE_MAX_LENGTH}
              disabled={busy}
              rows={2}
              placeholder={t('streamSetups:fields.notePlaceholder')}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </FormField>

        <FormField label={t('streamSetups:fields.presetLabel')} hint={t('streamSetups:fields.presetHint')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              value={metadataPresetId}
              disabled={busy}
              options={presetOptions}
              onChange={(event) => setMetadataPresetId(event.target.value)}
            />
          )}
        </FormField>

        <fieldset className="space-y-2">
          <legend className="text-sm font-medium text-ink">{t('streamSetups:fields.destinationsLabel')}</legend>
          {platforms.length === 0 ? (
            <p className="text-xs text-ink-muted">{t('streamSetups:form.noDestinations')}</p>
          ) : (
            <ul className="space-y-1.5">
              {platforms.map((platform) => (
                <li key={platform.id}>
                  <label className="flex cursor-pointer items-center gap-2 rounded-lg border border-line px-3 py-2 text-sm text-ink">
                    <input
                      type="checkbox"
                      checked={destinationIds.includes(platform.id)}
                      disabled={busy}
                      onChange={(event) => toggleDestination(platform.id, event.target.checked)}
                      className="size-4 rounded border-line accent-accent"
                    />
                    <ProviderBrand
                      providerId={platform.providerId}
                      fallbackLabel={platform.providerId.slice(0, 2).toUpperCase()}
                      size="sm"
                    />
                    <span className="min-w-0 flex-1 truncate">{platform.displayName}</span>
                  </label>
                </li>
              ))}
            </ul>
          )}
        </fieldset>
      </form>
    </Modal>
  );
}
