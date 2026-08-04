import type { ParseKeys } from 'i18next';
import { Loader2, Save, Trash2 } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { TextInput } from '@/components/ui/TextInput';
import { useCredentialStatusQuery, useDeleteStreamKeyMutation, useSetStreamKeyMutation } from '@/hooks/use-credentials';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { cn } from '@/lib/cn';
import { presentCredentialStatus, type CredentialTone } from '@/models/credential-presentation';
import { STREAM_KEY_MAX_LENGTH } from '@/models/platform-constraints';

import { validateStreamKeyDraft, type StreamKeyViolation } from './credential-validation';

type StreamKeySectionProps = {
  platform: ConfiguredPlatform;
};

const TONE_CLASSES: Record<CredentialTone, string> = {
  neutral: 'border-line text-ink-faint',
  positive: 'border-accent/40 bg-accent/12 text-accent-soft',
  warning: 'border-status-warning/40 bg-status-warning/12 text-status-warning',
};

function messageKeyForViolation(
  violation: StreamKeyViolation,
): ParseKeys<['platforms', 'common', 'errors']> {
  switch (violation) {
    case 'stream-key-required':
      return 'platforms:validation.streamKeyRequired';
    case 'stream-key-too-long':
      return 'platforms:validation.streamKeyTooLong';
    case 'stream-key-invalid':
      return 'platforms:validation.streamKeyInvalid';
  }
}

/**
 * Destination stream-key management, embedded in the platform settings
 * dialog.
 *
 * Shows only configured/missing/unavailable status - never the key itself,
 * never a length or partial value. The input is password-style, is cleared
 * immediately after a successful save, and (because this component is keyed
 * by platform id and unmounts whenever the dialog closes - see
 * `PlatformSettingsDialog`) is also cleared whenever the dialog closes or a
 * different platform is opened.
 */
export function StreamKeySection({ platform }: StreamKeySectionProps) {
  const { t } = useTranslation(['platforms', 'common', 'errors']);
  const tErrors = useTranslation('errors').t;

  const statusQuery = useCredentialStatusQuery(platform.id);
  const setStreamKeyMutation = useSetStreamKeyMutation();
  const deleteStreamKeyMutation = useDeleteStreamKeyMutation();

  const [streamKeyInput, setStreamKeyInput] = useState('');
  const [localError, setLocalError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [justSaved, setJustSaved] = useState(false);
  const [justDeleted, setJustDeleted] = useState(false);

  const busy = setStreamKeyMutation.isPending || deleteStreamKeyMutation.isPending;
  const storeUnavailable = statusQuery.data !== undefined && !statusQuery.data.store.available;
  const configured = statusQuery.data?.streamKey.configured === true;

  const presentation = presentCredentialStatus(statusQuery.data, statusQuery.isLoading);

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) return;

    const validation = validateStreamKeyDraft(streamKeyInput);
    if (validation.violation !== null) {
      setLocalError(t(messageKeyForViolation(validation.violation), { max: STREAM_KEY_MAX_LENGTH }));
      return;
    }

    setLocalError(null);
    setSubmitError(null);
    setJustSaved(false);
    setJustDeleted(false);

    setStreamKeyMutation.mutate(
      { platformId: platform.id, streamKey: validation.streamKey },
      {
        onSuccess: () => {
          // Clear immediately: the value must not linger in this component's
          // state (or a browser autofill/undo history) any longer than
          // submitting it required.
          setStreamKeyInput('');
          setJustSaved(true);
        },
        onError: (error) => {
          // Captured as a plain string, independent of the mutation's own
          // `error`/`variables`, which are cleared below regardless of
          // outcome.
          setSubmitError(resolveApiErrorMessage(tErrors, error));
        },
        onSettled: () => {
          // TanStack Query otherwise keeps this mutation's variables - the
          // stream key itself - in the mutation cache and in this hook's own
          // state until the next call or an unmount. Resetting immediately
          // after every settlement, success or failure, means the secret
          // does not linger there either.
          setStreamKeyMutation.reset();
        },
      },
    );
  };

  const handleDelete = () => {
    deleteStreamKeyMutation.mutate(platform.id, {
      onSuccess: () => {
        setConfirmingDelete(false);
        setJustDeleted(true);
        setJustSaved(false);
      },
    });
  };

  return (
    <div className="space-y-3 rounded-lg border border-line bg-surface-sunken p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
          {t('platforms:credentials.sectionTitle')}
        </p>
        <span
          className={cn(
            'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-semibold',
            TONE_CLASSES[presentation.tone],
          )}
        >
          {t(presentation.labelKey)}
        </span>
      </div>

      <p className="text-[11px] text-ink-faint">{t('platforms:credentials.explanation')}</p>
      <p className="text-[11px] text-ink-faint">{t('platforms:credentials.notVerifiedNote')}</p>
      <p className="text-[11px] text-ink-faint">
        {t('platforms:credentials.localIngestDistinction')}
      </p>

      {storeUnavailable && (
        <p
          role="alert"
          className="rounded-md border border-status-warning/30 bg-status-warning/10 px-2 py-1.5 text-[11px] text-status-warning"
        >
          {t('platforms:credentials.storeUnavailableNote')}
        </p>
      )}

      {submitError !== null && (
        <p
          role="alert"
          className="rounded-md border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-[11px] text-status-error"
        >
          {submitError}
        </p>
      )}

      <form onSubmit={handleSubmit} noValidate className="space-y-2">
        <FormField label={t('platforms:credentials.inputLabel')} error={localError ?? undefined}>
          {({ inputId, describedBy }) => (
            <TextInput
              id={inputId}
              type="password"
              autoComplete="off"
              aria-describedby={describedBy}
              aria-invalid={localError !== null}
              placeholder={t('platforms:credentials.inputPlaceholder')}
              value={streamKeyInput}
              maxLength={STREAM_KEY_MAX_LENGTH}
              disabled={busy || storeUnavailable}
              onChange={(event) => {
                setStreamKeyInput(event.target.value);
                setLocalError(null);
                setSubmitError(null);
                setJustSaved(false);
                setJustDeleted(false);
              }}
            />
          )}
        </FormField>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={busy || storeUnavailable}
            icon={
              setStreamKeyMutation.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Save className="size-3.5" />
              )
            }
          >
            {setStreamKeyMutation.isPending
              ? t('platforms:credentials.saving')
              : t('platforms:credentials.save')}
          </Button>
          <Button
            type="button"
            variant="danger"
            size="sm"
            disabled={busy || !configured}
            onClick={() => setConfirmingDelete(true)}
            icon={<Trash2 className="size-3.5" />}
          >
            {t('platforms:credentials.delete')}
          </Button>
        </div>

        <p aria-live="polite" className="text-[11px] text-ink-faint">
          {justSaved && <span className="text-status-live">{t('platforms:credentials.saved')}</span>}
          {justDeleted && (
            <span className="text-status-live">{t('platforms:credentials.deleted')}</span>
          )}
          {!justSaved && !justDeleted && (
            <>
              {t('platforms:credentials.replaceNote')} {t('platforms:credentials.deleteNote')}
            </>
          )}
        </p>
      </form>

      <ConfirmDialog
        open={confirmingDelete}
        title={t('platforms:credentials.deleteDialog.title')}
        message={t('platforms:credentials.deleteDialog.message', { platform: platform.displayName })}
        confirmLabel={t('platforms:credentials.deleteDialog.confirm')}
        destructive
        busy={deleteStreamKeyMutation.isPending}
        onConfirm={handleDelete}
        onCancel={() => setConfirmingDelete(false)}
      />
    </div>
  );
}
