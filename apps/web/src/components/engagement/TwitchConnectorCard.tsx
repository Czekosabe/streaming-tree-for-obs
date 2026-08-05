import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConnectedAccount } from '@/api/account-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  useAccountEngagementQuery,
  useAuthorizeEngagementMutation,
  useRestartEngagementMutation,
  useSetEngagementMutation,
} from '@/hooks/use-engagement';
import {
  connectorBlockerKey,
  connectorStateKey,
  connectorStateTone,
} from '@/models/engagement-presentation';

/**
 * One connected Twitch account's engagement-connector status and controls.
 *
 * Deliberately keeps three facts visually distinct, per the stage task:
 * the account is connected, metadata permission is granted (unaffected by
 * anything on this card), engagement permission is granted, the connector
 * is enabled, and EventSub is actually connected - five different facts,
 * never collapsed into one status chip.
 */
export function TwitchConnectorCard({ account }: { account: ConnectedAccount }) {
  const { t } = useTranslation('engagement');
  const { data, isLoading } = useAccountEngagementQuery(account.id);
  const setEngagement = useSetEngagementMutation();
  const authorize = useAuthorizeEngagementMutation();
  const restart = useRestartEngagementMutation();
  const [confirmingDisable, setConfirmingDisable] = useState(false);

  if (isLoading || data === undefined) {
    return (
      <Panel>
        <PanelHeader title={account.displayName || account.login} />
        <PanelBody>
          <p className="text-xs text-ink-faint">{t('connector.loading')}</p>
        </PanelBody>
      </Panel>
    );
  }

  const tone = connectorStateTone(data.state);
  const hasMissingScope = data.permissionUpgradeRequired;

  function handleToggle(nextEnabled: boolean) {
    if (!nextEnabled && data?.state === 'connected') {
      setConfirmingDisable(true);
      return;
    }
    setEngagement.mutate({ accountId: account.id, input: { enabled: nextEnabled } });
  }

  function confirmDisable() {
    setEngagement.mutate({ accountId: account.id, input: { enabled: false } });
    setConfirmingDisable(false);
  }

  return (
    <Panel>
      <PanelHeader
        title={account.displayName || account.login}
        description={t('connector.subtitle', { login: account.login })}
        actions={<StatusBadge status={tone} label={t(connectorStateKey(data.state))} />}
      />
      <PanelBody className="space-y-4">
        <ToggleSwitch
          label={t('connector.enableLabel')}
          description={t('connector.enableDescription')}
          checked={data.enabled}
          onCheckedChange={handleToggle}
        />

        {hasMissingScope && (
          <div className="rounded-lg border border-status-starting/40 bg-status-starting/10 p-3">
            <p className="text-xs font-medium text-status-starting">
              {t('connector.permissionUpgradeRequired')}
            </p>
            <p className="mt-1 text-[11px] text-ink-muted">
              {t('connector.permissionUpgradeExplanation')}
            </p>
            <Button
              size="sm"
              variant="primary"
              className="mt-2"
              onClick={() => authorize.mutate(account.id)}
              disabled={authorize.isPending}
            >
              {t('connector.authorizeAction')}
            </Button>
            {authorize.isSuccess && (
              <p className="mt-2 text-[11px] text-ink-muted">
                {t('connector.authorizeStarted', {
                  userCode: authorize.data.userCode ?? '',
                })}
              </p>
            )}
          </div>
        )}

        {data.blockerCodes !== undefined && data.blockerCodes.length > 0 && !hasMissingScope && (
          <p className="text-[11px] text-status-error">
            {data.blockerCodes
              .map((code) => {
                const key = connectorBlockerKey(code);
                return key === null ? code : t(key);
              })
              .join(', ')}
          </p>
        )}

        <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11px] text-ink-muted">
          <dt>{t('connector.subscriptions')}</dt>
          <dd className="text-right text-ink">
            {data.activeSubscriptionCount}/{data.expectedSubscriptionCount}
          </dd>
          <dt>{t('connector.reconnectCount')}</dt>
          <dd className="text-right text-ink">{data.reconnectCount}</dd>
          {data.lastEventAt !== undefined && (
            <>
              <dt>{t('connector.lastEventAt')}</dt>
              <dd className="text-right text-ink">
                {new Date(data.lastEventAt).toLocaleTimeString()}
              </dd>
            </>
          )}
          {data.lastDataGapAt !== undefined && (
            <>
              <dt>{t('connector.lastDataGapAt')}</dt>
              <dd className="text-right text-status-starting">
                {new Date(data.lastDataGapAt).toLocaleTimeString()}
              </dd>
            </>
          )}
          {data.lastError !== undefined && (
            <>
              <dt>{t('connector.lastError')}</dt>
              <dd className="text-right text-status-error">{data.lastError}</dd>
            </>
          )}
        </dl>

        {data.state === 'error' && (
          <Button
            size="sm"
            onClick={() => restart.mutate(account.id)}
            disabled={restart.isPending}
          >
            {t('connector.restartAction')}
          </Button>
        )}
      </PanelBody>

      <ConfirmDialog
        open={confirmingDisable}
        title={t('connector.disableConfirmTitle')}
        message={t('connector.disableConfirmMessage')}
        confirmLabel={t('connector.disableConfirmAction')}
        onConfirm={confirmDisable}
        onCancel={() => setConfirmingDisable(false)}
        busy={setEngagement.isPending}
      />
    </Panel>
  );
}
