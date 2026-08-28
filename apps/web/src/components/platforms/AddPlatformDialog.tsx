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

  // Deliberately always '' - never definitions[0]?.id. A real physical
  // Windows manual test found that seeding this from the first loaded
  // provider broke on the very first Add Platform open of a session:
  // useState's initializer runs exactly once, at this component's own
  // mount, which happens well before usePlatformDefinitionsQuery's
  // async result has arrived - so it captured '' anyway (definitions
  // was still []), permanently, since useState never re-runs its
  // initializer when the definitions prop changes later. Once
  // definitions did load, the <select>'s value ('') no longer matched
  // any of its real <option> elements, and a native <select> visually
  // falls back to showing its first option in that situation - so the
  // UI showed "Twitch" while providerId (and therefore validation and
  // the submitted payload) was still ''. A later open worked by
  // accident only because reset() re-evaluated definitions[0]?.id
  // after the query had already resolved. The real fix is structural,
  // not a timing patch: providerId now starts at '' unconditionally
  // and is never auto-populated from loaded data at all - matching the
  // explicit placeholder option below, which has value '' too, so the
  // visible selection and providerId can never disagree, and every
  // destination requires a deliberate platform choice, first open or
  // not.
  const [providerId, setProviderId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [enabled, setEnabled] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const serverFieldErrors = fieldErrorsOf(createPlatform.error);
  const busy = createPlatform.isPending;

  const reset = () => {
    setProviderId('');
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

  // The placeholder is a real option with value '' - the same value
  // providerId starts and resets to - so the native <select> always has
  // a real <option> to match against. This is what makes the controlled-
  // select contract hold: visible selection, providerId, validation, and
  // the submitted payload can never disagree, because there is never an
  // unmatched value for the browser to visually paper over on its own.
  const providerOptions = [
    { value: '', label: t('platforms:addDialog.providerPlaceholder') },
    ...definitions.map((definition) => ({
      value: definition.id,
      // Brand names are proper nouns and stay untranslated.
      label: definition.brandName,
    })),
  ];

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
