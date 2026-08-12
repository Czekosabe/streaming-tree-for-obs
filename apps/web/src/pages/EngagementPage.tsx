import { useTranslation } from 'react-i18next';

import { AppShell } from '@/components/layout/AppShell';
import { RecentEventsFeed } from '@/components/engagement/RecentEventsFeed';
import { TwitchConnectorCard } from '@/components/engagement/TwitchConnectorCard';
import { YouTubeConnectorCard } from '@/components/engagement/YouTubeConnectorCard';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { useAccountsQuery } from '@/hooks/use-accounts';
import { useEngagementStatusQuery } from '@/hooks/use-engagement';

/**
 * Stage 8A diagnostic and operational view for the Engagement Event Bus and
 * the Twitch inbound connector.
 *
 * This page is explicitly NOT the final unified operator chat (stage 9) or
 * an OBS overlay (stage 10) - see RecentEventsFeed's own doc comment. It
 * exists to make the Event Bus and connector state genuinely observable:
 * whether the bus is running, whether a connector is connected, and a
 * bounded recent-event feed for verification.
 */
export function EngagementPage() {
  const { t } = useTranslation(['pages', 'engagement']);
  const status = useEngagementStatusQuery();
  const accounts = useAccountsQuery();

  const twitchAccounts = (accounts.data ?? []).filter((account) => account.providerId === 'twitch');
  const youtubeAccounts = (accounts.data ?? []).filter((account) => account.providerId === 'youtube');

  return (
    <AppShell
      title={t('pages:engagement.title')}
      description={t('pages:engagement.description')}
    >
      <div className="mx-auto max-w-3xl space-y-4">
        <Panel>
          <PanelHeader title={t('engagement:status.title')} description={t('engagement:status.description')} />
          <PanelBody>
            {status.isLoading || status.data === undefined ? (
              <p className="text-xs text-ink-faint">{t('engagement:status.loading')}</p>
            ) : (
              <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs sm:grid-cols-4">
                <dt className="text-ink-faint">{t('engagement:status.retained')}</dt>
                <dd className="text-ink">
                  {status.data.retainedCount}/{status.data.bufferCapacity}
                </dd>
                <dt className="text-ink-faint">{t('engagement:status.oldestSequence')}</dt>
                <dd className="text-ink">{status.data.oldestSequence}</dd>
                <dt className="text-ink-faint">{t('engagement:status.newestSequence')}</dt>
                <dd className="text-ink">{status.data.newestSequence}</dd>
                <dt className="text-ink-faint">{t('engagement:status.activeSubscribers')}</dt>
                <dd className="text-ink">{status.data.activeSubscribers}</dd>
              </dl>
            )}
          </PanelBody>
        </Panel>

        {twitchAccounts.length === 0 && youtubeAccounts.length === 0 ? (
          <Panel>
            <PanelBody>
              <p className="text-xs text-ink-faint">{t('engagement:connector.noAccounts')}</p>
            </PanelBody>
          </Panel>
        ) : (
          <>
            {twitchAccounts.map((account) => <TwitchConnectorCard key={account.id} account={account} />)}
            {youtubeAccounts.map((account) => <YouTubeConnectorCard key={account.id} account={account} />)}
          </>
        )}

        <RecentEventsFeed />
      </div>
    </AppShell>
  );
}
