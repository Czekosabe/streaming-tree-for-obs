import { Loader2, Play, RefreshCw, Radio, Server, Square, Tv } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { BranchSnapshot } from '@/api/branch-schemas';
import type { RuntimeSnapshot } from '@/api/runtime-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { CopyableValue } from '@/components/runtime/CopyableValue';
import { RuntimeControls } from '@/components/runtime/RuntimeControls';
import { runtimeErrorMessage } from '@/components/runtime/runtime-error-message';
import { StartEnabledConfirmDialog } from '@/components/runtime/StartEnabledConfirmDialog';
import { Button } from '@/components/ui/Button';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import {
  useFfmpegRuntimeQuery,
  useBranchRuntimeQuery,
  useRestartBranchMutation,
  useStartBranchMutation,
  useStartEnabledBranchesMutation,
  useStopAllBranchesMutation,
  useStopBranchMutation,
} from '@/hooks/use-branches';
import { usePlatformsQuery } from '@/hooks/use-platforms';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { formatBytes, formatSpeed, toDurationParts } from '@/lib/format';
import { useLanguage } from '@/i18n/use-language';
import {
  blockerKey,
  branchControlsFor,
  branchFor,
  branchStateKey,
  branchTone,
  ffmpegStateKey,
  ffmpegTone,
} from '@/models/branch-presentation';
import {
  ingestStateKey,
  ingestTone,
  mediaMtxStateKey,
  mediaMtxTone,
} from '@/models/runtime-presentation';

/** One label/value row. */
function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3 text-xs">
      <span className="text-ink-muted">{label}</span>
      <span className="truncate text-right font-mono text-ink" title={value}>
        {value}
      </span>
    </div>
  );
}

function IngestServicePanel({ snapshot }: { snapshot: RuntimeSnapshot }) {
  const { t } = useTranslation('runtime');
  const { mediaMtx, ingest } = snapshot;

  const lastError = runtimeErrorMessage(t, mediaMtx.lastError);

  const sourceLabel =
    mediaMtx.source === 'managed'
      ? t('mediamtx.sourceManaged')
      : mediaMtx.source === 'override'
        ? t('mediamtx.sourceOverride')
        : t('mediamtx.sourceMissing');

  return (
    <Panel>
      <PanelHeader
        title={t('page.componentHeading')}
        description={t('mediamtx.componentName')}
        icon={<Server className="size-4" />}
        actions={<StatusBadge status={mediaMtxTone(mediaMtx.state)} label={t(mediaMtxStateKey(mediaMtx.state))} />}
      />
      <PanelBody className="space-y-4">
        <div className="space-y-1.5">
          <Row label={t('mediamtx.supportedVersion')} value={mediaMtx.supportedVersion} />
          {mediaMtx.installedVersion !== undefined && mediaMtx.installedVersion !== '' && (
            <Row label={t('mediamtx.installedVersion')} value={mediaMtx.installedVersion} />
          )}
          <Row label={t('mediamtx.source')} value={sourceLabel} />
          {mediaMtx.startedAt !== undefined && mediaMtx.startedAt !== '' && (
            <Row label={t('mediamtx.startedAt')} value={mediaMtx.startedAt} />
          )}
          <Row label={t('mediamtx.restartCount')} value={String(mediaMtx.restartCount)} />
        </div>

        <div className="space-y-1 border-t border-line pt-3 text-[11px] text-ink-faint">
          <p>
            {t('mediamtx.autoStart')}: {mediaMtx.autoStart ? 'on' : 'off'}
          </p>
          <p>
            {t('mediamtx.autoRestart')}: {mediaMtx.autoRestart ? 'on' : 'off'}
          </p>
        </div>

        <div className="border-t border-line pt-3">
          <p className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('page.lastError')}
          </p>
          {lastError === null ? (
            <p className="mt-1 text-xs text-ink-muted">{t('page.noError')}</p>
          ) : (
            <p className="mt-1 rounded-lg border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-xs leading-relaxed text-status-error">
              {lastError}
            </p>
          )}
        </div>

        <div className="border-t border-line pt-3">
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('controls.heading')}
          </p>
          <RuntimeControls mediaMtx={mediaMtx} />
        </div>

        {/* Ingest is only meaningful once the service is running. */}
        <div className="border-t border-line pt-3">
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('ingest.heading')}
          </p>
          <div className="flex items-center gap-2">
            <StatusBadge status={ingestTone(ingest.state)} label={t(ingestStateKey(ingest.state))} />
          </div>

          <div className="mt-2 space-y-1.5">
            <Row label={t('ingest.path')} value={ingest.path} />
            {/* Only values MediaMTX actually reported are shown. */}
            {ingest.sourceType !== undefined && ingest.sourceType !== '' && (
              <Row label={t('ingest.sourceType')} value={ingest.sourceType} />
            )}
            {ingest.connectedAt !== undefined && ingest.connectedAt !== '' && (
              <Row label={t('ingest.connectedAt')} value={ingest.connectedAt} />
            )}
            {ingest.trackCount !== null && (
              <Row
                label={t('ingest.tracks')}
                value={
                  ingest.tracks.length > 0
                    ? ingest.tracks.join(', ')
                    : t('ingest.trackCount', { count: ingest.trackCount })
                }
              />
            )}
          </div>

          <p className="mt-2 text-[11px] leading-relaxed text-ink-faint">
            {t('ingest.publisherNote')}
          </p>
        </div>
      </PanelBody>
    </Panel>
  );
}

function ObsSettingsPanel({ snapshot }: { snapshot: RuntimeSnapshot }) {
  const { t } = useTranslation('runtime');
  const { connection } = snapshot;

  return (
    <Panel>
      <PanelHeader
        title={t('connection.heading')}
        description={t('connection.instructions')}
        icon={<Radio className="size-4" />}
      />
      <PanelBody className="space-y-3">
        <CopyableValue
          label={t('connection.serverLabel')}
          value={connection.serverUrl}
          copyLabel={t('connection.copyServer')}
        />
        <CopyableValue
          label={t('connection.streamKeyLabel')}
          value={connection.streamKey}
          copyLabel={t('connection.copyStreamKey')}
        />
        <CopyableValue
          label={t('connection.publishUrlLabel')}
          value={connection.publishUrl}
          copyLabel={t('connection.copyServer')}
        />
        <p className="rounded-lg border border-line bg-surface-sunken px-3 py-2 text-[11px] leading-relaxed text-ink-faint">
          {t('connection.notASecret')}
        </p>
      </PanelBody>
    </Panel>
  );
}

function FFmpegDependencyPanel() {
  const { t } = useTranslation('runtime');
  const ffmpegQuery = useFfmpegRuntimeQuery();

  if (ffmpegQuery.data === undefined) return null;
  const { ffmpeg } = ffmpegQuery.data;

  const sourceLabel = {
    override: t('ffmpeg.sourceOverride'),
    bundled: t('ffmpeg.sourceBundled'),
    path: t('ffmpeg.sourcePath'),
    missing: t('ffmpeg.sourceMissing'),
  }[ffmpeg.source];

  const lastError = runtimeErrorMessage(t, ffmpeg.lastError);

  return (
    <Panel>
      <PanelHeader
        title={t('ffmpeg.heading')}
        icon={<Server className="size-4" />}
        actions={<StatusBadge status={ffmpegTone(ffmpeg.state)} label={t(ffmpegStateKey(ffmpeg.state))} />}
      />
      <PanelBody className="space-y-3">
        <div className="space-y-1.5">
          <Row label={t('ffmpeg.source')} value={sourceLabel} />
          <Row label={t('ffmpeg.detectedVersion')} value={ffmpeg.detectedVersion ?? '--'} />
          <Row label={t('ffmpeg.minimumVersion')} value={ffmpeg.minimumVersion} />
        </div>

        <div className="border-t border-line pt-2">
          <p className="mb-1 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('ffmpeg.capabilitiesHeading')}
          </p>
          <ul className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-ink-muted">
            {(Object.keys(ffmpeg.capabilities) as (keyof typeof ffmpeg.capabilities)[]).map((key) => (
              <li key={key} className="flex items-center justify-between gap-2">
                <span>{t(`ffmpeg.capability.${key}`)}</span>
                <span className={ffmpeg.capabilities[key] ? 'text-status-live' : 'text-status-error'}>
                  {ffmpeg.capabilities[key] ? '✓' : '✗'}
                </span>
              </li>
            ))}
          </ul>
        </div>

        {lastError !== null && (
          <p className="rounded-lg border border-status-error/30 bg-status-error/10 px-2 py-1.5 text-xs leading-relaxed text-status-error">
            {lastError}
          </p>
        )}

        {ffmpeg.state === 'missing' && (
          <p className="text-[11px] leading-relaxed text-ink-faint">{t('ffmpeg.notFoundNote')}</p>
        )}
        {ffmpeg.state === 'incompatible' && (
          <p className="text-[11px] leading-relaxed text-ink-faint">{t('ffmpeg.incompatibleNote')}</p>
        )}
      </PanelBody>
    </Panel>
  );
}

function BranchProgressCell({ branch }: { branch: BranchSnapshot | undefined }) {
  const { t } = useTranslation('runtime');
  const { locale } = useLanguage();

  if (branch?.progress === null || branch?.progress === undefined) {
    return <span className="text-ink-faint">{t('branch.noProgressYet')}</span>;
  }

  const { outTimeMs, totalSize, speed } = branch.progress;
  const duration = toDurationParts(outTimeMs / 1000);
  const durationLabel =
    duration.unit === 'hoursMinutesSeconds'
      ? `${duration.hours}:${duration.minutes}:${duration.seconds}`
      : `${duration.minutes}:${duration.seconds}`;

  return (
    <div className="space-y-0.5 text-[11px] text-ink-muted">
      <div>
        {t('branch.outputTime')}: <span className="font-mono text-ink">{durationLabel}</span>
      </div>
      <div>
        {t('branch.outputSize')}:{' '}
        <span className="font-mono text-ink">{formatBytes(totalSize, locale)}</span>
      </div>
      {speed > 0 && (
        <div>
          {t('branch.speed')}: <span className="font-mono text-ink">{formatSpeed(speed, locale)}</span>
        </div>
      )}
    </div>
  );
}

function BranchTablePanel() {
  const { t } = useTranslation(['runtime', 'platforms']);
  const platformsQuery = usePlatformsQuery();
  const branchesQuery = useBranchRuntimeQuery();

  const startMutation = useStartBranchMutation();
  const stopMutation = useStopBranchMutation();
  const restartMutation = useRestartBranchMutation();
  const startEnabledMutation = useStartEnabledBranchesMutation();
  const stopAllMutation = useStopAllBranchesMutation();

  const [confirmingStartEnabled, setConfirmingStartEnabled] = useState(false);
  const [confirmingStopAll, setConfirmingStopAll] = useState(false);

  const platforms = platformsQuery.data ?? [];
  const bulkBusy = startEnabledMutation.isPending || stopAllMutation.isPending;
  const liveBranchCount = platforms.filter(
    (platform) => branchFor(branchesQuery.data, platform.id)?.state === 'live',
  ).length;

  return (
    <Panel>
      <PanelHeader title={t('runtime:branch.heading')} icon={<Tv className="size-4" />} />
      <PanelBody className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled={bulkBusy || platforms.length === 0}
            icon={<Play className="size-3.5" />}
            onClick={() => setConfirmingStartEnabled(true)}
          >
            {t('runtime:branch.startEnabled')}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={bulkBusy}
            icon={<Square className="size-3.5" />}
            onClick={() => {
              // A destination that is not actually live yet (only
              // "starting"/blocked/idle) has nothing a viewer would
              // notice stop - only ask for confirmation when this
              // action would really interrupt a real, currently-
              // sending stream (governing task's own "stop all
              // branches" destructive-action requirement).
              if (liveBranchCount > 0) {
                setConfirmingStopAll(true);
              } else {
                stopAllMutation.mutate();
              }
            }}
          >
            {t('runtime:branch.stopAll')}
          </Button>
        </div>

        {platforms.length === 0 ? (
          <p className="text-xs text-ink-muted">{t('runtime:branch.noDestinations')}</p>
        ) : (
          <div className="space-y-2">
            {platforms.map((platform) => {
              const branch = branchFor(branchesQuery.data, platform.id);
              const state = branch?.state ?? 'idle';
              const controls = branchControlsFor(state);
              const blockers = branch?.blockers ?? [];
              const busy =
                startMutation.isPending || stopMutation.isPending || restartMutation.isPending;

              return (
                <div
                  key={platform.id}
                  className="rounded-lg border border-line bg-surface-sunken p-3"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-ink">{platform.displayName}</p>
                      <StatusBadge status={branchTone(state)} label={t(branchStateKey(state))} className="mt-1" />
                    </div>
                    <div className="flex shrink-0 items-center gap-1.5">
                      {controls.canStart && (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={busy}
                          icon={<Play className="size-3.5" />}
                          onClick={() => startMutation.mutate(platform.id)}
                        >
                          {t('runtime:branch.start')}
                        </Button>
                      )}
                      {controls.canStop && (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={busy}
                          icon={<Square className="size-3.5" />}
                          onClick={() => stopMutation.mutate(platform.id)}
                        >
                          {t('runtime:branch.stop')}
                        </Button>
                      )}
                      {controls.canRestart && (
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={busy}
                          icon={<RefreshCw className="size-3.5" />}
                          onClick={() => restartMutation.mutate(platform.id)}
                        >
                          {t('runtime:branch.restart')}
                        </Button>
                      )}
                    </div>
                  </div>

                  {blockers.length > 0 && (
                    <p className="mt-2 text-[11px] text-ink-faint">
                      {blockers
                        .map((blocker) => {
                          const key = blockerKey(blocker);
                          return key === null ? blocker : t(key);
                        })
                        .join(' · ')}
                    </p>
                  )}

                  {(state === 'live' || state === 'restarting') && (
                    <div className="mt-2 border-t border-line pt-2">
                      <BranchProgressCell branch={branch} />
                    </div>
                  )}

                  {branch?.restartCount !== undefined && branch.restartCount > 0 && (
                    <p className="mt-1 text-[11px] text-ink-faint">
                      {t('runtime:branch.restartCount', { count: branch.restartCount })}
                    </p>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </PanelBody>

      <StartEnabledConfirmDialog
        open={confirmingStartEnabled}
        platforms={platforms}
        branches={branchesQuery.data}
        busy={startEnabledMutation.isPending}
        onConfirm={() =>
          startEnabledMutation.mutate(undefined, {
            onSuccess: () => setConfirmingStartEnabled(false),
          })
        }
        onCancel={() => setConfirmingStartEnabled(false)}
      />

      <ConfirmDialog
        open={confirmingStopAll}
        title={t('runtime:branch.confirmStopAll.title')}
        message={t('runtime:branch.confirmStopAll.body', { count: liveBranchCount })}
        confirmLabel={t('runtime:branch.confirmStopAll.confirm')}
        destructive
        busy={stopAllMutation.isPending}
        onConfirm={() =>
          stopAllMutation.mutate(undefined, {
            onSuccess: () => setConfirmingStopAll(false),
          })
        }
        onCancel={() => setConfirmingStopAll(false)}
      />
    </Panel>
  );
}

/**
 * Streams page: local ingest from OBS, the FFmpeg dependency, and every
 * configured destination branch's real runtime state.
 *
 * The page keeps four distinct facts visually separate, never conflating
 * them: input receiving (ingest), output sending (branch state), a
 * destination being enabled (configuration), and a stream key being stored
 * (credential status, shown in platform settings).
 */
export function StreamsPage() {
  const { t } = useTranslation(['runtime', 'errors']);
  const tErrors = useTranslation('errors').t;
  const runtimeQuery = useRuntimeQuery();

  return (
    <AppShell title={t('runtime:page.title')} description={t('runtime:page.description')}>
      <div className="mx-auto max-w-3xl space-y-4">
        {runtimeQuery.isPending && (
          <Panel>
            <PanelBody className="flex items-center justify-center gap-2 py-12 text-sm text-ink-muted">
              <Loader2 aria-hidden="true" className="size-4 animate-spin" />
              {t('runtime:system.checking')}
            </PanelBody>
          </Panel>
        )}

        {runtimeQuery.isError && (
          <Panel>
            <PanelBody className="space-y-3 py-10 text-center">
              <p className="text-sm font-medium text-status-error">
                {t('runtime:system.runtimeUnavailable')}
              </p>
              <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
                {resolveApiErrorMessage(tErrors, runtimeQuery.error)}
              </p>
              <Button
                variant="primary"
                icon={<RefreshCw className="size-3.5" />}
                onClick={() => void runtimeQuery.refetch()}
              >
                {t('runtime:controls.restart')}
              </Button>
            </PanelBody>
          </Panel>
        )}

        {runtimeQuery.data !== undefined && (
          <>
            <IngestServicePanel snapshot={runtimeQuery.data} />
            <ObsSettingsPanel snapshot={runtimeQuery.data} />
          </>
        )}

        <FFmpegDependencyPanel />
        <BranchTablePanel />
      </div>
    </AppShell>
  );
}
