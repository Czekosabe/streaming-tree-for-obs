import { PlugZap } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useFfmpegRuntimeQuery } from '@/hooks/use-branches';
import { useHealthQuery } from '@/hooks/use-health-query';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { ffmpegStateKey, ffmpegTone } from '@/models/branch-presentation';
import type { PlatformStatus } from '@/models/platform';
import { mediaMtxStateKey, mediaMtxTone } from '@/models/runtime-presentation';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';
import { StatusDot } from '../ui/StatusBadge';

/**
 * "Services": the real runtime dependencies this application needs to
 * actually stream - is the backend reachable, is the local ingest engine
 * running, is a compatible FFmpeg available.
 *
 * Named "Services" (previously, misleadingly, "System resources" - a real
 * physical finding: service-dependency health and host CPU/memory/disk
 * usage are not the same concept, and calling this card "System resources"
 * meant the Dashboard both had no real resources card AND mislabelled the
 * one card it did have). Real host resource usage now lives in its own,
 * accurately-named `SystemResourcesCard`.
 *
 * Reuses the exact same `useHealthQuery`/`useRuntimeQuery`/
 * `useFfmpegRuntimeQuery` data every other page already fetches (
 * `BackendHealthCard`, `SidebarFooter`, `StreamsPage`'s FFmpeg panel) -
 * never a second implementation of any of these three real facts.
 */
export function ServicesCard() {
  const { t } = useTranslation(['dashboard', 'runtime']);
  const healthQuery = useHealthQuery();
  const runtimeQuery = useRuntimeQuery();
  const ffmpegQuery = useFfmpegRuntimeQuery();

  const backendTone: PlatformStatus = healthQuery.isPending
    ? 'starting'
    : healthQuery.isSuccess
      ? 'live'
      : 'error';
  const backendLabel = healthQuery.isPending
    ? t('dashboard:backend.pending')
    : healthQuery.isSuccess
      ? t('dashboard:backend.connected')
      : t('dashboard:backend.unavailable');

  const mediaMtxState = runtimeQuery.data?.mediaMtx.state;
  const ingestTone: PlatformStatus = runtimeQuery.isPending
    ? 'starting'
    : mediaMtxState === undefined
      ? 'error'
      : mediaMtxTone(mediaMtxState);
  const ingestLabel = runtimeQuery.isPending
    ? t('runtime:system.checking')
    : mediaMtxState === undefined
      ? t('runtime:system.runtimeUnavailable')
      : t(`runtime:${mediaMtxStateKey(mediaMtxState)}`);

  const ffmpegState = ffmpegQuery.data?.ffmpeg.state;
  const ffmpegToneValue: PlatformStatus = ffmpegQuery.isPending
    ? 'starting'
    : ffmpegState === undefined
      ? 'offline'
      : ffmpegTone(ffmpegState);
  const ffmpegLabel = ffmpegQuery.isPending
    ? t('runtime:system.checking')
    : ffmpegState === undefined
      ? t('runtime:ffmpeg.state.missing')
      : t(`runtime:${ffmpegStateKey(ffmpegState)}`);

  const rows: { label: string; tone: PlatformStatus; value: string }[] = [
    { label: t('dashboard:services.backend'), tone: backendTone, value: backendLabel },
    { label: t('dashboard:services.ingestEngine'), tone: ingestTone, value: ingestLabel },
    { label: t('dashboard:services.ffmpeg'), tone: ffmpegToneValue, value: ffmpegLabel },
  ];

  return (
    <Panel>
      <PanelHeader
        title={t('dashboard:services.heading')}
        description={t('dashboard:services.description')}
        icon={<PlugZap className="size-4" />}
        headingLevel={3}
      />
      <PanelBody className="space-y-2.5">
        {rows.map((row) => (
          <div key={row.label} className="flex items-center justify-between gap-2 text-xs">
            <span className="text-ink-muted">{row.label}</span>
            <span className="flex min-w-0 items-center gap-1.5 truncate font-medium text-ink">
              <StatusDot status={row.tone} />
              <span className="truncate">{row.value}</span>
            </span>
          </div>
        ))}
      </PanelBody>
    </Panel>
  );
}
