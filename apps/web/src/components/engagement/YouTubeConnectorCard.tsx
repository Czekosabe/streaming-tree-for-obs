import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ConnectedAccount } from '@/api/account-schemas';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { useAccountEngagementQuery, useRestartEngagementMutation, useSetEngagementMutation } from '@/hooks/use-engagement';
import { connectorBlockerKey, connectorStateKey, connectorStateTone } from '@/models/engagement-presentation';

/**
 * One connected YouTube account's engagement-connector status and controls
 * - Stage 15A's own parallel sibling to TwitchConnectorCard (this codebase's
 * established convention for provider-specific status, see
 * components/settings/YouTubeAccountsPanel.tsx), not a generic component
 * with provider branching: YouTube has no subscription count, no scope
 * upgrade flow (youtube.RequiredScope covers everything from the start -
 * see internal/httpapi/engagement.go's toYouTubeAccountEngagementResponse),
 * and its own YouTube-only fields (selected broadcast, last poll, possible
 * gap count, unsupported-event count) that Twitch's card has no equivalent
 * for.
 */
export function YouTubeConnectorCard({ account }: { account: ConnectedAccount }) {
  const { t } = useTranslation('engagement');
  const { data, isLoading } = useAccountEngagementQuery(account.id);
  const setEngagement = useSetEngagementMutation();
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
        description={t('youtubeConnector.subtitle', { login: account.login })}
        actions={<StatusBadge status={tone} label={t(connectorStateKey(data.state))} />}
      />
      <PanelBody className="space-y-4">
        <ToggleSwitch
          label={t('youtubeConnector.enableLabel')}
          description={t('youtubeConnector.enableDescription')}
          checked={data.enabled}
          onCheckedChange={handleToggle}
        />

        {data.selectedBroadcastId === undefined && data.enabled && (
          <p className="text-[11px] text-status-starting">{t('youtubeConnector.noBroadcastSelected')}</p>
        )}

        {data.blockerCodes !== undefined && data.blockerCodes.length > 0 && (
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
          <dt>{t('connector.reconnectCount')}</dt>
          <dd className="text-right text-ink">{data.reconnectCount}</dd>
          {data.lastPollAt !== undefined && (
            <>
              <dt>{t('youtubeConnector.lastPollAt')}</dt>
              <dd className="text-right text-ink">{new Date(data.lastPollAt).toLocaleTimeString()}</dd>
            </>
          )}
          {data.lastEventAt !== undefined && (
            <>
              <dt>{t('connector.lastEventAt')}</dt>
              <dd className="text-right text-ink">{new Date(data.lastEventAt).toLocaleTimeString()}</dd>
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
          {data.possibleGapCount !== undefined && data.possibleGapCount > 0 && (
            <>
              <dt>{t('youtubeConnector.possibleGapCount')}</dt>
              <dd className="text-right text-status-starting">{data.possibleGapCount}</dd>
            </>
          )}
          {data.unsupportedEventCount !== undefined && data.unsupportedEventCount > 0 && (
            <>
              <dt>{t('youtubeConnector.unsupportedEventCount')}</dt>
              <dd className="text-right text-ink">{data.unsupportedEventCount}</dd>
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
          <Button size="sm" onClick={() => restart.mutate(account.id)} disabled={restart.isPending}>
            {t('connector.restartAction')}
          </Button>
        )}
      </PanelBody>

      <ConfirmDialog
        open={confirmingDisable}
        title={t('youtubeConnector.disableConfirmTitle')}
        message={t('youtubeConnector.disableConfirmMessage')}
        confirmLabel={t('connector.disableConfirmAction')}
        onConfirm={confirmDisable}
        onCancel={() => setConfirmingDisable(false)}
        busy={setEngagement.isPending}
      />
    </Panel>
  );
}
