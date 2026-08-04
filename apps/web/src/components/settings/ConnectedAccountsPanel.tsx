import { Loader2, Plug, RefreshCw, Save, Unplug } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConnectedAccount } from '@/api/account-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { TextInput } from '@/components/ui/TextInput';
import {
  useAccountsQuery,
  useDisconnectAccountMutation,
  useIntegrationConfigQuery,
  useSetIntegrationConfigMutation,
  useValidateAccountMutation,
} from '@/hooks/use-accounts';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { cn } from '@/lib/cn';
import { useLanguage } from '@/i18n/use-language';
import { accountStatusKey, accountStatusTone } from '@/models/account-presentation';

import { TwitchDeviceFlowModal } from './TwitchDeviceFlowModal';

const TONE_CLASSES: Record<'live' | 'error' | 'offline' | 'starting', string> = {
  live: 'border-status-live/40 bg-status-live/12 text-status-live',
  error: 'border-status-error/40 bg-status-error/12 text-status-error',
  offline: 'border-line text-ink-faint',
  starting: 'border-status-starting/40 bg-status-starting/12 text-status-starting',
};

function IntegrationConfigForm() {
  const { t } = useTranslation(['accounts', 'errors']);
  const tErrors = useTranslation('errors').t;
  const configQuery = useIntegrationConfigQuery();
  const setConfig = useSetIntegrationConfigMutation();

  const [clientId, setClientId] = useState('');
  const [saved, setSaved] = useState(false);

  if (configQuery.data === undefined) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-ink-muted">
        <Loader2 aria-hidden="true" className="size-4 animate-spin" />
      </div>
    );
  }

  const config = configQuery.data;
  const isEnvironment = config.source === 'environment';
  const value = clientId !== '' ? clientId : (config.clientId ?? '');

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isEnvironment || setConfig.isPending) return;
    setSaved(false);
    setConfig.mutate(
      { clientId: value },
      {
        onSuccess: () => {
          setClientId('');
          setSaved(true);
        },
      },
    );
  };

  const sourceNote = {
    environment: t('accounts:integration.clientIdSourceEnvironment'),
    database: t('accounts:integration.clientIdSourceDatabase'),
    missing: t('accounts:integration.clientIdSourceMissing'),
  }[config.source];

  return (
    <form onSubmit={handleSubmit} noValidate className="space-y-3">
      <p className="text-[11px] text-ink-faint">{t('accounts:integration.description')}</p>

      {setConfig.error !== null && (
        <p role="alert" className="rounded-md border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-[11px] text-status-error">
          {resolveApiErrorMessage(tErrors, setConfig.error)}
        </p>
      )}

      <FormField label={t('accounts:integration.clientIdLabel')} hint={sourceNote}>
        {({ inputId, describedBy }) => (
          <TextInput
            id={inputId}
            aria-describedby={describedBy}
            value={value}
            disabled={isEnvironment || setConfig.isPending}
            placeholder={t('accounts:integration.clientIdPlaceholder')}
            onChange={(event) => {
              setClientId(event.target.value);
              setSaved(false);
            }}
          />
        )}
      </FormField>

      <p className="text-[11px] text-ink-faint">{t('accounts:integration.noSecretNote')}</p>
      {isEnvironment ? null : configQuery.data.configured === false ? null : (
        <p className="text-[11px] text-ink-faint">{t('accounts:integration.lockedNote')}</p>
      )}

      {!isEnvironment && (
        <div className="flex items-center gap-2">
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={setConfig.isPending || value.trim() === ''}
            icon={setConfig.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
          >
            {setConfig.isPending ? t('accounts:integration.saving') : t('accounts:integration.save')}
          </Button>
          {saved && <span className="text-[11px] text-status-live">{t('accounts:integration.saved')}</span>}
        </div>
      )}
    </form>
  );
}

function AccountRow({ account }: { account: ConnectedAccount }) {
  const { t } = useTranslation(['accounts', 'errors']);
  const tErrors = useTranslation('errors').t;
  const { locale } = useLanguage();

  const validateMutation = useValidateAccountMutation();
  const disconnectMutation = useDisconnectAccountMutation();

  const [reconnecting, setReconnecting] = useState(false);
  const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);

  const lastValidatedLabel =
    account.lastValidatedAt === undefined
      ? t('accounts:account.neverValidated')
      : t('accounts:account.lastValidated', {
          time: new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(
            new Date(account.lastValidatedAt),
          ),
        });

  return (
    <div className="rounded-lg border border-line bg-surface-sunken p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2.5">
          {account.avatarUrl !== undefined && account.avatarUrl !== '' && (
            <img
              src={account.avatarUrl}
              alt=""
              aria-hidden="true"
              className="size-8 shrink-0 rounded-full border border-line object-cover"
            />
          )}
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-ink">{account.displayName}</p>
            <p className="truncate text-[11px] text-ink-faint">@{account.login}</p>
          </div>
        </div>
        <span
          className={cn(
            'shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-semibold',
            TONE_CLASSES[accountStatusTone(account.status)],
          )}
        >
          {t(accountStatusKey(account.status))}
        </span>
      </div>

      {account.status === 'reconnect_required' && (
        <p className="mt-2 text-[11px] text-status-warning">{t('accounts:account.reconnectRequiredNote')}</p>
      )}

      <div className="mt-2 space-y-1 text-[11px] text-ink-faint">
        <p>
          {t('accounts:account.scopesLabel')}: {account.scopes.length > 0 ? account.scopes.join(', ') : '--'}
        </p>
        <p>{lastValidatedLabel}</p>
      </div>

      {validateMutation.error !== null && (
        <p role="alert" className="mt-2 text-[11px] text-status-error">
          {resolveApiErrorMessage(tErrors, validateMutation.error)}
        </p>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={validateMutation.isPending}
          icon={
            validateMutation.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <RefreshCw className="size-3.5" />
            )
          }
          onClick={() => validateMutation.mutate(account.id)}
        >
          {validateMutation.isPending ? t('accounts:account.validating') : t('accounts:account.validate')}
        </Button>
        <Button type="button" variant="secondary" size="sm" onClick={() => setReconnecting(true)}>
          {t('accounts:account.reconnect')}
        </Button>
        <Button
          type="button"
          variant="danger"
          size="sm"
          icon={<Unplug className="size-3.5" />}
          onClick={() => setConfirmingDisconnect(true)}
        >
          {t('accounts:account.disconnect')}
        </Button>
      </div>

      <TwitchDeviceFlowModal
        open={reconnecting}
        reconnectAccountId={account.id}
        onClose={() => setReconnecting(false)}
        onAuthorized={() => setReconnecting(false)}
      />

      <ConfirmDialog
        open={confirmingDisconnect}
        title={t('accounts:account.disconnectDialog.title', { login: account.login })}
        message={t('accounts:account.disconnectDialog.message')}
        confirmLabel={t('accounts:account.disconnectDialog.confirm')}
        destructive
        busy={disconnectMutation.isPending}
        onConfirm={() =>
          disconnectMutation.mutate(account.id, { onSuccess: () => setConfirmingDisconnect(false) })
        }
        onCancel={() => setConfirmingDisconnect(false)}
      />
    </div>
  );
}

/**
 * Connected Accounts section of the Settings page: Twitch Client ID
 * configuration, the Connect action, and the list of connected accounts
 * with their health and per-account actions.
 */
export function ConnectedAccountsPanel() {
  const { t } = useTranslation('accounts');
  const configQuery = useIntegrationConfigQuery();
  const accountsQuery = useAccountsQuery();

  const [connecting, setConnecting] = useState(false);

  const canConnect = configQuery.data?.configured === true;
  const accounts = accountsQuery.data ?? [];

  return (
    <Panel>
      <PanelHeader
        title={t('integration.heading')}
        icon={<Plug className="size-4" />}
      />
      <PanelBody className="space-y-4">
        <IntegrationConfigForm />

        <div className="border-t border-line pt-4">
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('account.heading')}
          </p>

          {!canConnect && (
            <p className="mb-2 text-[11px] text-ink-faint">{t('connect.notConfiguredNote')}</p>
          )}

          <Button
            type="button"
            variant="primary"
            size="sm"
            disabled={!canConnect}
            icon={<Plug className="size-3.5" />}
            onClick={() => setConnecting(true)}
          >
            {t('connect.button')}
          </Button>

          <div className="mt-3 space-y-2">
            {accounts.length === 0 ? (
              <p className="text-xs text-ink-muted">{t('account.empty')}</p>
            ) : (
              accounts.map((account) => <AccountRow key={account.id} account={account} />)
            )}
          </div>
        </div>
      </PanelBody>

      <TwitchDeviceFlowModal open={connecting} onClose={() => setConnecting(false)} onAuthorized={() => setConnecting(false)} />
    </Panel>
  );
}
