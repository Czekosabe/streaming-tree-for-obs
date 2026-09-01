import { Check, Loader2 } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { SaveMetadataInput } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { TextArea, TextInput } from '@/components/ui/TextInput';
import { useCreateMetadataPresetMutation } from '@/hooks/use-metadata-presets';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { NAME_MAX_LENGTH, NOTE_MAX_LENGTH } from '@/models/metadata-preset-constraints';

type SavePresetDialogProps = {
  open: boolean;
  onClose: () => void;
  /** The provider the current form's category/categoryId belongs to -
   * stored as that provider's own scoped entry, never blended into
   * another provider's data (docs/metadata-presets.md §1/§6). */
  providerId: string;
  /** The current form draft - whatever is on screen right now,
   * whether already saved or not, matching "turn what I'm looking at
   * into a reusable preset" (docs/metadata-presets.md §7). */
  draft: SaveMetadataInput;
};

/**
 * "Save as preset" - the primary create-a-preset workflow
 * (docs/metadata-presets.md §7). No dedicated backend "capture"
 * endpoint: this dialog builds the same generic create request every
 * other preset creation uses, from the caller's own current draft.
 */
export function SavePresetDialog({ open, onClose, providerId, draft }: SavePresetDialogProps) {
  const { t } = useTranslation(['metadataPresets', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const createPreset = useCreateMetadataPresetMutation();

  const [name, setName] = useState('');
  const [note, setNote] = useState('');
  const [saved, setSaved] = useState(false);

  const busy = createPreset.isPending;

  // Reset the form's own local state each time the dialog opens fresh,
  // so a previous save's leftover name/note never bleeds into the next.
  useEffect(() => {
    if (open) {
      setName('');
      setNote('');
      setSaved(false);
      createPreset.reset();
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

    createPreset.mutate(
      {
        name: name.trim(),
        note,
        title: draft.title,
        description: draft.description,
        tags: draft.tags,
        language: draft.language,
        visibility: draft.visibility,
        matureContent: draft.matureContent,
        dvr: draft.dvr,
        latencyMode: draft.latencyMode,
        providers:
          draft.category.trim().length > 0 || draft.categoryId.trim().length > 0
            ? { [providerId]: { category: draft.category, categoryId: draft.categoryId } }
            : {},
      },
      {
        onSuccess: () => {
          setSaved(true);
          window.setTimeout(() => {
            onClose();
          }, 900);
        },
      },
    );
  };

  const generalError = createPreset.error !== null ? resolveApiErrorMessage(tErrors, createPreset.error) : null;

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={t('metadataPresets:save.title')}
      description={t('metadataPresets:save.description')}
      dismissible={!busy}
      size="sm"
      footer={
        <>
          <Button type="button" onClick={handleClose} disabled={busy}>
            {t('common:actions.cancel')}
          </Button>
          <Button
            type="submit"
            form="save-preset-form"
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
            {saved ? t('metadataPresets:save.saved') : busy ? t('metadataPresets:save.saving') : t('metadataPresets:save.submit')}
          </Button>
        </>
      }
    >
      <form id="save-preset-form" onSubmit={handleSubmit} noValidate className="space-y-4">
        {generalError !== null && (
          <p role="alert" className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error">
            {generalError}
          </p>
        )}

        <FormField
          label={t('metadataPresets:fields.nameLabel')}
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
              placeholder={t('metadataPresets:fields.namePlaceholder')}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </FormField>

        <FormField
          label={t('metadataPresets:fields.noteLabel')}
          hint={t('metadataPresets:fields.noteHint')}
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
              placeholder={t('metadataPresets:fields.notePlaceholder')}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </FormField>
      </form>
    </Modal>
  );
}
