import type { TFunction } from 'i18next';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { FormField } from '@/components/ui/FormField';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { TextArea } from '@/components/ui/TextInput';
import {
  useAudioPendingQuery,
  useAudioStatusQuery,
  useApproveAudioPendingItemMutation,
  useClearAudioQueueMutation,
  useRejectAudioPendingItemMutation,
  useSkipAudioQueueCurrentMutation,
  useTestSpeakAudioMutation,
} from '@/hooks/use-audio';
import { ApiError } from '@/lib/api-client';

function errorMessage(t: TFunction<'audio'>, error: unknown): string {
  if (error instanceof ApiError && error.code !== null) {
    const key = `errors.${error.code}` as never;
    const translated = t(key, { defaultValue: '' });
    return translated === '' ? t('errors.generic') : translated;
  }
  return t('errors.generic');
}

/**
 * Runtime status, the bounded queue's own counters, pending-approval
 * items (approve/reject), queue commands (skip current/clear), and
 * Test Speak - the operator-facing view of internal/audio.Manager's
 * own runtime state, never a complete historical message list
 * (governing task §46).
 */
export function AudioQueuePanel() {
  const { t } = useTranslation('audio');
  const statusQuery = useAudioStatusQuery();
  const pendingQuery = useAudioPendingQuery();
  const skipMutation = useSkipAudioQueueCurrentMutation();
  const clearMutation = useClearAudioQueueMutation();
  const approveMutation = useApproveAudioPendingItemMutation();
  const rejectMutation = useRejectAudioPendingItemMutation();
  const testSpeakMutation = useTestSpeakAudioMutation();

  const [clearConfirmOpen, setClearConfirmOpen] = useState(false);
  const [testText, setTestText] = useState('');

  const status = statusQuery.data;

  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader title={t('status.title')} />
        <PanelBody className="space-y-3">
          {status !== undefined && (
            <>
              <div className="flex flex-wrap items-center gap-4 text-sm">
                <span className="text-ink">
                  {t('status.renderer')}:{' '}
                  <span className={status.rendererConnected ? 'text-status-live' : 'text-ink-faint'}>
                    {status.rendererConnected ? t('status.rendererConnected') : t('status.rendererDisconnected')}
                  </span>
                </span>
              </div>

              <p className="text-sm text-ink">
                {t('status.current')}:{' '}
                {status.hasCurrentItem ? (
                  <span>
                    {status.currentSynthetic && (
                      <span className="mr-1.5 rounded bg-accent/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-accent-soft">
                        {t('status.currentSynthetic')}
                      </span>
                    )}
                  </span>
                ) : (
                  <span className="text-ink-faint">
                    {status.rendererConnected ? t('status.currentNone') : t('status.waitingForRenderer')}
                  </span>
                )}
              </p>

              {status.inputGap && (
                <p role="alert" className="text-xs text-status-warning">
                  {t('status.inputGapWarning')}
                </p>
              )}

              <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs sm:grid-cols-3">
                {(
                  [
                    ['totalEnqueued', status.totalEnqueued],
                    ['totalPlayed', status.totalPlayed],
                    ['totalExpired', status.totalExpired],
                    ['totalCapacityDropped', status.totalCapacityDropped],
                    ['totalManuallySkipped', status.totalManuallySkipped],
                    ['totalSynthetic', status.totalSynthetic],
                    ['totalRejected', status.totalRejected],
                    ['totalPlaybackFailed', status.totalPlaybackFailed],
                    ['totalSynthesisFailed', status.totalSynthesisFailed],
                    ['totalInterrupted', status.totalInterrupted],
                  ] as const
                ).map(([key, value]) => (
                  <div key={key} className="flex items-baseline justify-between gap-2">
                    <dt className="text-ink-faint">{t(`status.counters.${key}`)}</dt>
                    <dd className="font-mono tabular-nums text-ink">{value}</dd>
                  </div>
                ))}
              </dl>
            </>
          )}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelHeader
          title={t('queue.title')}
          {...(status !== undefined
            ? { description: t('queue.queuedCount', { count: status.readyQueueCount, capacity: status.capacity }) }
            : {})}
          actions={
            <>
              <Button size="sm" onClick={() => skipMutation.mutate()} disabled={skipMutation.isPending}>
                {t('queue.skipAction')}
              </Button>
              <Button size="sm" variant="danger" onClick={() => setClearConfirmOpen(true)}>
                {t('queue.clearAction')}
              </Button>
            </>
          }
        />
      </Panel>

      <Panel>
        <PanelHeader title={t('pending.title')} />
        <PanelBody className="space-y-2">
          {(pendingQuery.data === undefined || pendingQuery.data.length === 0) && (
            <p className="text-sm text-ink-faint">{t('pending.empty')}</p>
          )}
          {pendingQuery.data !== undefined && pendingQuery.data.length > 0 && (
            <ul className="space-y-2">
              {pendingQuery.data.map((item) => (
                <li
                  key={item.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-line bg-surface-sunken px-3 py-2"
                >
                  <p className="min-w-0 truncate text-sm text-ink">{item.text}</p>
                  <div className="flex shrink-0 gap-2">
                    <Button size="sm" variant="success" onClick={() => approveMutation.mutate(item.id)}>
                      {t('pending.approveAction')}
                    </Button>
                    <Button size="sm" variant="danger" onClick={() => rejectMutation.mutate(item.id)}>
                      {t('pending.rejectAction')}
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelHeader title={t('testSpeak.title')} />
        <PanelBody className="space-y-3">
          <p className="text-xs text-ink-faint">{t('testSpeak.hint')}</p>
          <FormField label={t('testSpeak.title')}>
            {({ inputId }) => (
              <TextArea
                id={inputId}
                value={testText}
                placeholder={t('testSpeak.placeholder')}
                onChange={(e) => setTestText(e.target.value)}
              />
            )}
          </FormField>
          {testSpeakMutation.isError && (
            <p role="alert" className="text-sm text-status-error">
              {errorMessage(t, testSpeakMutation.error)}
            </p>
          )}
          <div className="flex justify-end">
            <Button
              variant="primary"
              disabled={testText.trim() === '' || testSpeakMutation.isPending}
              onClick={() =>
                testSpeakMutation.mutate(testText, {
                  onSuccess: () => setTestText(''),
                })
              }
            >
              {t('testSpeak.action')}
            </Button>
          </div>
        </PanelBody>
      </Panel>

      <ConfirmDialog
        open={clearConfirmOpen}
        title={t('queue.clearConfirmTitle')}
        message={t('queue.clearConfirmMessage')}
        confirmLabel={t('queue.clearAction')}
        destructive
        busy={clearMutation.isPending}
        onCancel={() => setClearConfirmOpen(false)}
        onConfirm={() => clearMutation.mutate(undefined, { onSuccess: () => setClearConfirmOpen(false) })}
      />
    </div>
  );
}
