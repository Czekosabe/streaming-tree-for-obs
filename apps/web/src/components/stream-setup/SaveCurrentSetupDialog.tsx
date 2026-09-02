import { Check, Loader2 } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { MetadataPreset } from '@/api/metadata-preset-schemas';
import { Button } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextArea, TextInput } from '@/components/ui/TextInput';
import { useSaveCurrentStreamSetupMutation } from '@/hooks/use-stream-setups';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { NAME_MAX_LENGTH, NOTE_MAX_LENGTH } from '@/models/stream-setup-constraints';

type SaveCurrentSetupDialogProps = {
  open: boolean;
  onClose: () => void;
  metadataPresets: readonly MetadataPreset[];
};

const NO_PRESET = '';

/**
 * "Save current setup" - captures the currently-enabled destination
 * set into a new named profile (docs/stream-setup-profiles.md §14).
 * No destination picker here: the backend derives the set from real
 * current `Platform.Enabled` state at save time.
 */
export function SaveCurrentSetupDialog({ open, onClose, metadataPresets }: SaveCurrentSetupDialogProps) {
  const { t } = useTranslation(['streamSetups', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const saveCurrent = useSaveCurrentStreamSetupMutation();

  const [name, setName] = useState('');
  const [note, setNote] = useState('');
  const [metadataPresetId, setMetadataPresetId] = useState<string>(NO_PRESET);
  const [saved, setSaved] = useState(false);

  const busy = saveCurrent.isPending;

  useEffect(() => {
    if (open) {
      setName('');
      setNote('');
      setMetadataPresetId(NO_PRESET);
      setSaved(false);
      saveCurrent.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const handleClose = () => {
    if (busy) return;
    onClose();
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy || name.trim().length === 0) return;

    saveCurrent.mutate(
      {
        name: name.trim(),
        note,
        metadataPresetId: metadataPresetId === NO_PRESET ? null : metadataPresetId,
      },
      {
        onSuccess: () => {
          setSaved(true);
          window.setTimeout(() => onClose(), 900);
        },
      },
    );
  };

  const generalError = saveCurrent.error !== null ? resolveApiErrorMessage(tErrors, saveCurrent.error) : null;

  const presetOptions = [
    { value: NO_PRESET, label: t('streamSetups:fields.noPreset') },
    ...metadataPresets.map((preset) => ({ value: preset.id, label: preset.name })),
  ];

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={t('streamSetups:saveCurrent.title')}
      description={t('streamSetups:saveCurrent.description')}
      dismissible={!busy}
      size="sm"
      footer={
        <>
          <Button type="button" onClick={handleClose} disabled={busy}>
            {t('common:actions.cancel')}
          </Button>
          <Button
            type="submit"
            form="save-current-setup-form"
            variant="primary"
            disabled={busy || name.trim().length === 0}
            icon={
              busy ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : saved ? (
                <Check className="size-3.5" />
              ) : undefined
            }
          >
            {saved
              ? t('streamSetups:saveCurrent.saved')
              : busy
                ? t('streamSetups:saveCurrent.saving')
                : t('streamSetups:saveCurrent.submit')}
          </Button>
        </>
      }
    >
      <form id="save-current-setup-form" onSubmit={handleSubmit} noValidate className="space-y-4">
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
      </form>
    </Modal>
  );
}
