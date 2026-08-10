import { Pause, Play, RotateCcw, SkipForward, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelHeader } from '@/components/ui/Panel';
import {
  useAlertQueueStatusQuery,
  useClearAlertQueueMutation,
  usePauseAlertQueueMutation,
  useReplayPreviousAlertMutation,
  useResumeAlertQueueMutation,
  useSkipCurrentAlertMutation,
} from '@/hooks/use-alerts';

/**
 * Live queue/playback status and controls for one alert profile - Part
 * 35's "current queue state, pause/resume, skip current, replay
 * previous, clear queue". Polls every 5s (see useAlertQueueStatusQuery)
 * so the operator sees the same state a Browser Source viewer would,
 * without needing a second SSE connection just for the management UI.
 */
export function QueuePanel({ profileId }: { profileId: string }) {
  const { t } = useTranslation('alerts');
  const statusQuery = useAlertQueueStatusQuery(profileId);
  const pauseMutation = usePauseAlertQueueMutation(profileId);
  const resumeMutation = useResumeAlertQueueMutation(profileId);
  const skipMutation = useSkipCurrentAlertMutation(profileId);
  const replayMutation = useReplayPreviousAlertMutation(profileId);
  const clearMutation = useClearAlertQueueMutation(profileId);
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false);

  const status = statusQuery.data;
  if (status === undefined) return null;

  return (
    <Panel>
      <PanelHeader
        title={t('queue.title')}
        description={status.paused ? t('queue.paused') : t('queue.running')}
      />
      <div className="space-y-4 p-4 sm:p-5">
        {status.inputGap && (
          <p role="status" className="rounded-lg border border-status-error/40 bg-status-error/10 px-3 py-2 text-xs text-status-error">
            {t('queue.inputGapWarning')}
          </p>
        )}

        <div>
          <p className="text-xs font-medium text-ink-muted">{t('queue.current')}</p>
          {status.current === undefined ? (
            <p className="mt-1 text-sm text-ink-faint">{t('queue.currentNone')}</p>
          ) : (
            <p className="mt-1 text-sm text-ink">
              {status.current.renderedText}
              {status.current.synthetic && (
                <span className="ml-2 rounded-full border border-line px-1.5 py-0.5 text-[10px] text-ink-faint">
                  {t('common.synthetic')}
                </span>
              )}
              {status.current.groupCount > 1 && (
                <span className="ml-2 rounded-full border border-line px-1.5 py-0.5 text-[10px] text-ink-faint">
                  {t('queue.currentGroupCount', { count: status.current.groupCount })}
                </span>
              )}
              <span className="ml-2 rounded-full border border-line px-1.5 py-0.5 text-[10px] text-ink-faint">
                {status.current.interruptible ? t('queue.currentInterruptible') : t('queue.currentNotInterruptible')}
              </span>
            </p>
          )}
        </div>

        <p className="text-xs text-ink-muted">
          {t('queue.queuedCount', { count: status.queuedCount, capacity: status.queueCapacity })}
        </p>

        <div className="flex flex-wrap items-center gap-2">
          {status.paused ? (
            <Button size="sm" icon={<Play className="size-4" />} onClick={() => resumeMutation.mutate()} disabled={resumeMutation.isPending}>
              {t('queue.resumeAction')}
            </Button>
          ) : (
            <Button size="sm" icon={<Pause className="size-4" />} onClick={() => pauseMutation.mutate()} disabled={pauseMutation.isPending}>
              {t('queue.pauseAction')}
            </Button>
          )}
          <Button
            size="sm"
            icon={<SkipForward className="size-4" />}
            onClick={() => skipMutation.mutate()}
            disabled={skipMutation.isPending || status.current === undefined}
          >
            {t('queue.skipAction')}
          </Button>
          <Button
            size="sm"
            icon={<RotateCcw className="size-4" />}
            onClick={() => replayMutation.mutate()}
            disabled={replayMutation.isPending || !status.replayAvailable}
            title={status.replayAvailable ? undefined : t('queue.replayUnavailable')}
          >
            {t('queue.replayAction')}
          </Button>
          <Button
            size="sm"
            variant="danger"
            icon={<Trash2 className="size-4" />}
            onClick={() => setClearConfirmOpen(true)}
            disabled={clearMutation.isPending || status.queuedCount === 0}
          >
            {t('queue.clearAction')}
          </Button>
        </div>

        <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-ink-muted sm:grid-cols-3">
          {(
            [
              ['totalEnqueued', status.totalEnqueued],
              ['totalPlayed', status.totalPlayed],
              ['totalExpired', status.totalExpired],
              ['totalCapacityDropped', status.totalCapacityDropped],
              ['totalManuallySkipped', status.totalManuallySkipped],
              ['totalSynthetic', status.totalSynthetic],
              ['totalGroupedMembers', status.totalGroupedMembers],
              ['totalGroupsCreated', status.totalGroupsCreated],
              ['totalPreempted', status.totalPreempted],
            ] as const
          ).map(([key, value]) => (
            <div key={key} className="flex items-center justify-between gap-2 rounded-lg border border-line px-2 py-1">
              <dt>{t(`queue.counters.${key}`)}</dt>
              <dd className="font-mono tabular-nums text-ink">{value}</dd>
            </div>
          ))}
        </dl>
      </div>

      {clearConfirmOpen && (
        <ConfirmDialog
          open
          title={t('queue.clearConfirmTitle')}
          message={t('queue.clearConfirmMessage')}
          confirmLabel={t('queue.clearAction')}
          destructive
          busy={clearMutation.isPending}
          onCancel={() => setClearConfirmOpen(false)}
          onConfirm={() => clearMutation.mutate(undefined, { onSuccess: () => setClearConfirmOpen(false) })}
        />
      )}
    </Panel>
  );
}
