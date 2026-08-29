import { Cpu } from 'lucide-react';
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
 * "System resources": the real services this application depends on to
 * actually stream, not host CPU/memory/disk/network - this application has
 * never collected host telemetry and this stage does not add it (a
 * standing, deliberate non-goal - see docs/dashboard-design.md §5 and
 * `THIRD_PARTY_NOTICES.md`'s neighbouring MediaMTX/FFmpeg entries). What
 * IS real and already fetched elsewhere in this app (`BackendHealthCard`,
 * `SidebarFooter`, `StreamsPage`'s FFmpeg panel) is service-dependency
 * health: is the backend reachable, is the local ingest engine running,
 * is a compatible FFmpeg available. Reusing those exact three real facts
 * here gives this rail position real, honest, deliberate content instead
 * of an empty slot or a fabricated resource-usage number.
 */
export function SystemResourcesCard() {
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
    { label: t('dashboard:resources.backend'), tone: backendTone, value: backendLabel },
    { label: t('dashboard:resources.ingestEngine'), tone: ingestTone, value: ingestLabel },
    { label: t('dashboard:resources.ffmpeg'), tone: ffmpegToneValue, value: ffmpegLabel },
  ];

  return (
    <Panel>
      <PanelHeader
        title={t('dashboard:resources.heading')}
        description={t('dashboard:resources.description')}
        icon={<Cpu className="size-4" />}
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
