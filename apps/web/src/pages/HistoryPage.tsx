import { History as HistoryIcon, Loader2, TrendingUp } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { StreamInsightsDestination } from '@/api/stream-insights-schemas';
import type { StreamSession, StreamSessionDestination } from '@/api/stream-sessions-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { ProviderBrand } from '@/components/providers/ProviderBrand';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import {
  useClearStreamSessionHistoryMutation,
  useSetStreamSessionRetentionDaysMutation,
  useStreamSessionSettingsQuery,
  useStreamSessionsQuery,
} from '@/hooks/use-stream-sessions';
import { useStreamInsightsQuery } from '@/hooks/use-stream-insights';
import { useLanguage } from '@/i18n/use-language';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { formatTimestamp, toDurationParts } from '@/lib/format';

const RETENTION_OPTIONS = [7, 30, 90, 180, 365] as const;

/** "Xh Ym" - a friendlier, coarser format than durationLabel's H:MM:SS
 * clock face, used for cumulative/aggregate durations that can span
 * many hours. */
function hoursMinutesLabel(totalSeconds: number): string {
  const parts = toDurationParts(totalSeconds);
  return `${parts.hours}h ${parts.minutes}m`;
}

function durationLabel(startedAt: string, endMs: number): string {
  const startMs = new Date(startedAt).getTime();
  const duration = toDurationParts(Math.max(0, endMs - startMs) / 1000);
  return duration.unit === 'hoursMinutesSeconds'
    ? `${duration.hours}:${duration.minutes}:${duration.seconds}`
    : `${duration.minutes}:${duration.seconds}`;
}

/** Ticks once a second only while the session is still open, so a
 * pinned in-progress session's duration visibly counts up without
 * polling the backend any harder than useStreamSessionsQuery already
 * does. */
function SessionDuration({ session }: { session: StreamSession }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!session.open) return undefined;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [session.open]);

  const endMs = session.open || session.endedAt === null ? now : new Date(session.endedAt).getTime();
  return <span className="font-mono text-ink">{durationLabel(session.startedAt, endMs)}</span>;
}

const OUTCOME_CLASSES: Record<string, string> = {
  completed: 'border-status-live/40 bg-status-live/12 text-status-live',
  session_ended: 'border-status-live/40 bg-status-live/12 text-status-live',
  error: 'border-status-error/40 bg-status-error/12 text-status-error',
};

function DestinationChip({ destination }: { destination: StreamSessionDestination }) {
  const { t } = useTranslation('history');
  const classes = destination.open
    ? 'border-status-starting/40 bg-status-starting/12 text-status-starting'
    : (OUTCOME_CLASSES[destination.outcome] ?? 'border-line bg-surface-sunken text-ink-muted');
  const label = destination.open ? t('outcome.live') : t(`outcome.${destination.outcome}`, destination.outcome);

  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] ${classes}`}>
      <span className="font-medium text-ink">{destination.displayName}</span>
      <span className="opacity-80">{label}</span>
    </span>
  );
}

function SessionRow({ session }: { session: StreamSession }) {
  const { t } = useTranslation('history');
  const { locale } = useLanguage();

  return (
    <div className="rounded-lg border border-line bg-surface-sunken px-3 py-2.5" data-testid="history-session-row">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          {session.open && (
            <span className="inline-flex items-center gap-1 rounded-full border border-status-starting/40 bg-status-starting/12 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-status-starting">
              {t('inProgress')}
            </span>
          )}
          <span className="text-xs text-ink-muted">{formatTimestamp(session.startedAt, locale)}</span>
        </div>
        <SessionDuration session={session} />
      </div>
      {session.destinations.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {session.destinations.map((d) => (
            <DestinationChip key={d.id} destination={d} />
          ))}
        </div>
      ) : (
        <p className="mt-2 text-xs text-ink-faint">{t('noDestinations')}</p>
      )}
    </div>
  );
}

function RetentionSettings() {
  const { t } = useTranslation('history');
  const settingsQuery = useStreamSessionSettingsQuery();
  const setRetention = useSetStreamSessionRetentionDaysMutation();
  const clearHistory = useClearStreamSessionHistoryMutation();
  const [confirming, setConfirming] = useState(false);

  return (
    <Panel>
      <PanelHeader title={t('settings.heading')} />
      <PanelBody className="space-y-3">
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-ink-muted" htmlFor="history-retention">
            {t('settings.retentionLabel')}
          </label>
          <SelectInput
            id="history-retention"
            className="h-9 w-48"
            value={String(settingsQuery.data?.retentionDays ?? '')}
            disabled={settingsQuery.data === undefined || setRetention.isPending}
            onChange={(event) => setRetention.mutate(Number(event.target.value))}
            options={RETENTION_OPTIONS.map((days) => ({ value: String(days), label: t('settings.retentionDays', { count: days }) }))}
          />
        </div>

        <Button type="button" variant="danger" onClick={() => setConfirming(true)}>
          {t('settings.clearButton')}
        </Button>
        {clearHistory.isSuccess && (
          <p className="text-xs font-medium text-status-live">{t('settings.clearSuccess')}</p>
        )}
      </PanelBody>

      <ConfirmDialog
        open={confirming}
        title={t('settings.clearConfirmTitle')}
        message={t('settings.clearConfirmMessage')}
        confirmLabel={t('settings.clearButton')}
        destructive
        busy={clearHistory.isPending}
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          clearHistory.mutate();
          setConfirming(false);
        }}
      />
    </Panel>
  );
}

function StatTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-line bg-surface-sunken px-3 py-2.5">
      <p className="text-[11px] font-medium uppercase tracking-wide text-ink-faint">{label}</p>
      <p className="mt-1 text-lg font-semibold text-ink">{value}</p>
    </div>
  );
}

function DestinationInsightRow({ destination }: { destination: StreamInsightsDestination }) {
  const { t } = useTranslation('history');
  const outcomeEntries = Object.entries(destination.outcomeCounts).filter(([, count]) => count > 0);

  return (
    <li className="flex flex-wrap items-center gap-2 rounded-lg border border-line px-3 py-2 text-sm">
      <ProviderBrand
        providerId={destination.providerId}
        fallbackLabel={destination.providerId.slice(0, 2).toUpperCase()}
        size="sm"
      />
      <span className="min-w-0 flex-1 truncate text-ink">{destination.displayName}</span>
      <span className="text-xs text-ink-muted">
        {t('insights.destinationSessions', { count: destination.sessionCount })}
      </span>
      <span className="text-xs text-ink-muted">{hoursMinutesLabel(destination.durationSeconds)}</span>
      {outcomeEntries.length > 0 && (
        <span className="text-xs text-ink-faint">
          {outcomeEntries
            .map(([outcome, count]) =>
              t('insights.outcomeCount', { count, outcome: t(`outcome.${outcome || 'live'}`, outcome) }),
            )
            .join(', ')}
        </span>
      )}
    </li>
  );
}

/**
 * Stage 27: stream insights (docs/stream-session-history.md §14) - a read-only
 * aggregation of Stage 24's own operational history, never a new
 * data source. A different view of the exact same data the session
 * list above already owns, not a duplicate of it.
 */
function InsightsSection() {
  const { t } = useTranslation(['history', 'errors']);
  const tErrors = useTranslation('errors').t;
  const { locale } = useLanguage();
  const insightsQuery = useStreamInsightsQuery();

  return (
    <Panel>
      <PanelHeader title={t('history:insights.heading')} icon={<TrendingUp className="size-4" />} />
      <PanelBody className="space-y-3">
        {insightsQuery.isPending && (
          <div className="flex items-center justify-center gap-2 py-8 text-sm text-ink-muted">
            <Loader2 aria-hidden="true" className="size-4 animate-spin" />
            {t('history:list.loading')}
          </div>
        )}

        {insightsQuery.isError && (
          <p className="py-4 text-center text-sm text-status-error">
            {resolveApiErrorMessage(tErrors, insightsQuery.error)}
          </p>
        )}

        {insightsQuery.isSuccess && insightsQuery.data.totalSessions === 0 && (
          <p className="py-6 text-center text-sm text-ink-muted">{t('history:insights.empty')}</p>
        )}

        {insightsQuery.isSuccess && insightsQuery.data.totalSessions > 0 && (
          <>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              <StatTile
                label={t('history:insights.totalSessions')}
                value={String(insightsQuery.data.totalSessions)}
              />
              <StatTile
                label={t('history:insights.totalDuration')}
                value={hoursMinutesLabel(insightsQuery.data.totalDurationSeconds)}
              />
              <StatTile
                label={t('history:insights.averageDuration')}
                value={hoursMinutesLabel(insightsQuery.data.averageDurationSeconds)}
              />
            </div>

            {insightsQuery.data.longestSession !== null && (
              <p className="text-xs text-ink-muted">
                {t('history:insights.longestSession', {
                  duration: hoursMinutesLabel(insightsQuery.data.longestSession.durationSeconds),
                  date: formatTimestamp(insightsQuery.data.longestSession.startedAt, locale),
                })}
              </p>
            )}

            {insightsQuery.data.destinations.length > 0 && (
              <div className="space-y-1.5">
                <p className="text-xs font-semibold uppercase tracking-wide text-ink-faint">
                  {t('history:insights.destinationsHeading')}
                </p>
                <ul className="space-y-1.5">
                  {insightsQuery.data.destinations.map((destination, index) => (
                    <DestinationInsightRow
                      key={destination.platformId ?? `${destination.providerId}-${destination.displayName}-${index}`}
                      destination={destination}
                    />
                  ))}
                </ul>
              </div>
            )}
          </>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * Stage 24: stream session / operational history (docs/stream-session-
 * history.md). A record of when Streaming Tree observed a stream
 * session run and which destinations participated - never chat
 * messages, chatter names, donation content, or any other viewer
 * content (§0 of the contract).
 */
export function HistoryPage() {
  const { t } = useTranslation(['history', 'errors']);
  const tErrors = useTranslation('errors').t;
  const sessionsQuery = useStreamSessionsQuery();
  const sessions = sessionsQuery.data ?? [];

  return (
    <AppShell title={t('history:page.title')} description={t('history:page.description')}>
      <div className="mx-auto max-w-3xl space-y-4">
        <InsightsSection />

        <Panel>
          <PanelHeader title={t('history:page.title')} icon={<HistoryIcon className="size-4" />} />
          <PanelBody className="space-y-3">
            {sessionsQuery.isPending && (
              <div className="flex items-center justify-center gap-2 py-10 text-sm text-ink-muted">
                <Loader2 aria-hidden="true" className="size-4 animate-spin" />
                {t('history:list.loading')}
              </div>
            )}

            {sessionsQuery.isError && (
              <div className="space-y-2 py-8 text-center">
                <p className="text-sm font-medium text-status-error">{t('history:list.backendUnavailable')}</p>
                <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
                  {resolveApiErrorMessage(tErrors, sessionsQuery.error)}
                </p>
              </div>
            )}

            {sessionsQuery.isSuccess && sessions.length === 0 && (
              <p className="py-8 text-center text-sm text-ink-muted">{t('history:list.empty')}</p>
            )}

            {sessions.length > 0 && (
              <div className="space-y-2">
                {sessions.map((session) => (
                  <SessionRow key={session.id} session={session} />
                ))}
              </div>
            )}
          </PanelBody>
        </Panel>

        <RetentionSettings />
      </div>
    </AppShell>
  );
}
