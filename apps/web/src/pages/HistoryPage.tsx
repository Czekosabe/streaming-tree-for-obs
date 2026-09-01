import { History as HistoryIcon, Loader2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { StreamSession, StreamSessionDestination } from '@/api/stream-sessions-schemas';
import { AppShell } from '@/components/layout/AppShell';
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
import { useLanguage } from '@/i18n/use-language';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { formatTimestamp, toDurationParts } from '@/lib/format';

const RETENTION_OPTIONS = [7, 30, 90, 180, 365] as const;

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
