import { Loader2, Plus } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ProviderDefinition } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { Modal } from '@/components/ui/Modal';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useApiFieldErrors } from '@/hooks/use-api-field-errors';
import { useCreatePlatformMutation } from '@/hooks/use-platforms';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { DISPLAY_NAME_MAX_LENGTH } from '@/models/platform-constraints';

import { validateAddPlatform } from './add-platform-validation';

type AddPlatformDialogProps = {
  open: boolean;
  onClose: () => void;
  definitions: readonly ProviderDefinition[];
};

/**
 * Creates a configured destination.
 *
 * Several destinations may use the same provider, so the provider select is
 * never filtered by what already exists. No stream key is requested here: the
 * API does not accept one and credentials will live in the OS credential store.
 */
export function AddPlatformDialog({ open, onClose, definitions }: AddPlatformDialogProps) {
  const { t } = useTranslation(['platforms', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;
  const fieldErrorsOf = useApiFieldErrors();
  const createPlatform = useCreatePlatformMutation();

  const firstProvider = definitions[0]?.id ?? '';
  const [providerId, setProviderId] = useState(firstProvider);
  const [displayName, setDisplayName] = useState('');
  const [enabled, setEnabled] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const serverFieldErrors = fieldErrorsOf(createPlatform.error);
  const busy = createPlatform.isPending;

  const reset = () => {
    setProviderId(firstProvider);
    setDisplayName('');
    setEnabled(false);
    setLocalError(null);
    createPlatform.reset();
  };

  const handleClose = () => {
    if (busy) return;
    reset();
    onClose();
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    // Guards against a double submit from a fast second Enter press.
    if (busy) return;

    const validation = validateAddPlatform({ providerId, displayName });
    if (!validation.valid) {
      switch (validation.violation) {
        case 'display-name-required':
          setLocalError(t('platforms:validation.displayNameRequired'));
          break;
        case 'display-name-too-long':
          setLocalError(
            t('platforms:validation.displayNameTooLong', { max: DISPLAY_NAME_MAX_LENGTH }),
          );
          break;
        default:
          setLocalError(t('platforms:validation.providerRequired'));
          break;
      }
      return;
    }

    setLocalError(null);
    createPlatform.mutate(
      { providerId, displayName: validation.displayName, enabled },
      {
        onSuccess: () => {
          reset();
          onClose();
        },
      },
    );
  };

  // A validation rejection is shown per field; anything else is a general
  // failure that belongs above the form.
  const generalError =
    createPlatform.error !== null && Object.keys(serverFieldErrors).length === 0
      ? resolveApiErrorMessage(tErrors, createPlatform.error)
      : null;

  const providerOptions = definitions.map((definition) => ({
    value: definition.id,
    // Brand names are proper nouns and stay untranslated.
    label: definition.brandName,
  }));

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={t('platforms:addDialog.title')}
      description={t('platforms:addDialog.description')}
      dismissible={!busy}
      footer={
        <>
          <Button type="button" onClick={handleClose} disabled={busy}>
            {t('common:actions.cancel')}
          </Button>
          <Button
            type="submit"
            form="add-platform-form"
            variant="primary"
            disabled={busy}
            icon={
              busy ? <Loader2 className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />
            }
          >
            {busy ? t('platforms:addDialog.creating') : t('platforms:addDialog.submit')}
          </Button>
        </>
      }
    >
      <form id="add-platform-form" onSubmit={handleSubmit} noValidate className="space-y-4">
        {generalError !== null && (
          <p
            role="alert"
            className="rounded-lg border border-status-error/30 bg-status-error/10 px-3 py-2 text-xs text-status-error"
          >
            {generalError}
          </p>
        )}

        <FormField label={t('platforms:addDialog.providerLabel')} error={serverFieldErrors.providerId}>
          {({ inputId, describedBy }) => (
            <SelectInput
              id={inputId}
              aria-describedby={describedBy}
              options={providerOptions}
              value={providerId}
              disabled={busy}
              onChange={(event) => setProviderId(event.target.value)}
            />
          )}
        </FormField>

        <FormField
          label={t('platforms:addDialog.displayNameLabel')}
          hint={t('platforms:addDialog.displayNameHint')}
          error={localError ?? serverFieldErrors.displayName}
          counter={`${displayName.trim().length} / ${DISPLAY_NAME_MAX_LENGTH}`}
        >
          {({ inputId, describedBy }) => (
            <TextInput
              id={inputId}
              aria-describedby={describedBy}
              aria-invalid={localError !== null || serverFieldErrors.displayName !== undefined}
              value={displayName}
              maxLength={DISPLAY_NAME_MAX_LENGTH}
              disabled={busy}
              placeholder={t('platforms:addDialog.displayNamePlaceholder')}
              onChange={(event) => {
                setDisplayName(event.target.value);
                setLocalError(null);
              }}
            />
          )}
        </FormField>

        <div className="rounded-lg border border-line bg-surface-sunken p-3">
          <ToggleSwitch
            label={t('platforms:settings.enabledLabel')}
            description={t('platforms:settings.enabledDescription')}
            checked={enabled}
            onCheckedChange={setEnabled}
          />
        </div>

        <p className="text-[11px] leading-relaxed text-ink-faint">
          {t('platforms:addDialog.noCredentialsNote')}
        </p>
      </form>
    </Modal>
  );
}
