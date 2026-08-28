import { Loader2, Save } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { Button } from '@/components/ui/Button';
import { FormField } from '@/components/ui/FormField';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useOutputSettingsQuery, useUpdateOutputSettingsMutation } from '@/hooks/use-output';
import { resolveApiErrorMessage } from '@/lib/api-error-message';

import { validateServerUrlDraft } from './output-validation';

type OutputSettingsSectionProps = {
  platform: ConfiguredPlatform;
};

/**
 * Destination server-address management, embedded in the platform settings
 * dialog alongside `StreamKeySection`.
 *
 * The server address is not a secret - it is cached normally, unlike the
 * stream key. Never joined with the key anywhere in this component: they
 * stay two separate inputs and two separate API calls.
 */
export function OutputSettingsSection({ platform }: OutputSettingsSectionProps) {
  const { t } = useTranslation(['platforms', 'errors']);
  const tErrors = useTranslation('errors').t;

  const settingsQuery = useOutputSettingsQuery(platform.id);
  const updateMutation = useUpdateOutputSettingsMutation();

  const [serverUrl, setServerUrl] = useState('');
  const [autoRestart, setAutoRestart] = useState(true);
  const [localError, setLocalError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [initialized, setInitialized] = useState(false);

  // Populate the form once the current settings load - a plain effect, not a
  // secret-handling concern, since a server address is safe to display and
  // pre-fill (unlike the stream key, which is never preloaded).
  useEffect(() => {
    if (settingsQuery.data !== undefined && !initialized) {
      setServerUrl(settingsQuery.data.serverUrl);
      setAutoRestart(settingsQuery.data.autoRestart);
      setInitialized(true);
    }
  }, [settingsQuery.data, initialized]);

  const busy = updateMutation.isPending;

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) return;

    const validation = validateServerUrlDraft(serverUrl);
    if (!validation.valid) {
      setLocalError(
        validation.violation === 'server-url-missing-host'
          ? t('platforms:validation.serverUrlRequired')
          : t('platforms:validation.serverUrlInvalid'),
      );
      return;
    }

    setLocalError(null);
    setSaved(false);
    updateMutation.mutate(
      { platformId: platform.id, input: { serverUrl: validation.serverUrl, autoRestart } },
      { onSuccess: () => setSaved(true) },
    );
  };

  const submitFailure =
    updateMutation.error !== null ? resolveApiErrorMessage(tErrors, updateMutation.error) : null;

  return (
    <div className="space-y-3 rounded-lg bg-surface-sunken/70 p-3">
      <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {t('platforms:output.sectionTitle')}
      </p>

      <p className="text-[11px] text-ink-faint">{t('platforms:output.explanation')}</p>
      <p className="text-[11px] text-ink-faint">{t('platforms:output.serverKeyDistinction')}</p>

      {submitFailure !== null && (
        <p
          role="alert"
          className="rounded-md border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-[11px] text-status-error"
        >
          {submitFailure}
        </p>
      )}

      <form onSubmit={handleSubmit} noValidate className="space-y-2">
        <FormField label={t('platforms:output.serverUrlLabel')} error={localError ?? undefined}>
          {({ inputId, describedBy }) => (
            <TextInput
              id={inputId}
              aria-describedby={describedBy}
              aria-invalid={localError !== null}
              placeholder={t('platforms:output.serverUrlPlaceholder')}
              value={serverUrl}
              disabled={busy}
              onChange={(event) => {
                setServerUrl(event.target.value);
                setLocalError(null);
                setSaved(false);
              }}
            />
          )}
        </FormField>

        <ToggleSwitch
          label={t('platforms:output.autoRestartLabel')}
          description={t('platforms:output.autoRestartDescription')}
          checked={autoRestart}
          onCheckedChange={(next) => {
            setAutoRestart(next);
            setSaved(false);
          }}
        />

        <div className="flex items-center gap-2">
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={busy}
            icon={
              updateMutation.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Save className="size-3.5" />
              )
            }
          >
            {updateMutation.isPending
              ? t('platforms:output.saving')
              : t('platforms:output.save')}
          </Button>
          {saved && (
            <span aria-live="polite" className="text-[11px] text-status-live">
              {t('platforms:output.saved')}
            </span>
          )}
        </div>
      </form>
    </div>
  );
}
