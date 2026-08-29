import { Cpu } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useLanguage } from '@/i18n/use-language';
import { useSystemResourcesQuery } from '@/hooks/use-system-resources';
import { cn } from '@/lib/cn';
import { formatBytes } from '@/lib/format';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';

/** Threshold-based colouring keeps high utilisation readable at a glance -
 * the same convention the deleted demo-era `Meter` component used, now
 * backed by real values instead of static constants. */
function barClass(value: number): string {
  if (value >= 85) return 'bg-status-error';
  if (value >= 65) return 'bg-status-warning';
  return 'bg-accent';
}

function ResourceRow({
  label,
  unavailableLabel,
  percent,
  detail,
}: {
  label: string;
  unavailableLabel: string;
  percent: number | null | undefined;
  detail?: string | undefined;
}) {
  if (percent === null || percent === undefined) {
    return (
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-xs font-medium text-ink-muted">{label}</span>
        <span className="text-[11px] text-ink-faint">{unavailableLabel}</span>
      </div>
    );
  }

  const clamped = Math.min(100, Math.max(0, Math.round(percent)));

  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-xs font-medium text-ink-muted">{label}</span>
        <span className="font-mono text-xs tabular-nums text-ink">{clamped}%</span>
      </div>
      <div
        role="meter"
        aria-label={label}
        aria-valuenow={clamped}
        aria-valuemin={0}
        aria-valuemax={100}
        className="h-1.5 w-full overflow-hidden rounded-full bg-surface-sunken ring-1 ring-line ring-inset"
      >
        <div
          className={cn('h-full rounded-full transition-[width] duration-500', barClass(clamped))}
          style={{ width: `${clamped}%` }}
        />
      </div>
      {detail !== undefined && detail !== '' && (
        <p className="text-[11px] text-ink-faint">{detail}</p>
      )}
    </div>
  );
}

/**
 * Real, local host resource usage - CPU, memory, and disk usage of this
 * application's own data volume - sampled by the backend's
 * `internal/sysresources` collector every 5 seconds and served over
 * `GET /api/system/resources`. Nothing here is fabricated: a metric this
 * platform/environment cannot report renders an honest "unavailable" row
 * instead of a fake percentage, and each of the three metrics is
 * independent - one being unavailable never hides the other two.
 *
 * This is local-only resource monitoring, not telemetry: nothing sampled
 * here is persisted or sent anywhere beyond this same local HTTP response
 * to this same local browser tab.
 */
export function SystemResourcesCard() {
  const { t } = useTranslation(['dashboard', 'common']);
  const { locale } = useLanguage();
  const query = useSystemResourcesQuery();
  const snap = query.data;

  const usedOfTotal = (used: number | null | undefined, total: number | null | undefined) =>
    used != null && total != null
      ? t('dashboard:resources.usedOfTotal', {
          used: formatBytes(used, locale),
          total: formatBytes(total, locale),
        })
      : undefined;

  return (
    <Panel>
      <PanelHeader
        title={t('dashboard:resources.heading')}
        description={t('dashboard:resources.description')}
        icon={<Cpu className="size-4" />}
        headingLevel={3}
      />
      <PanelBody className="space-y-4">
        {query.isPending && (
          <p className="text-xs text-ink-muted">{t('dashboard:resources.loading')}</p>
        )}
        {query.isError && (
          <p className="text-xs text-status-error">{t('dashboard:resources.unavailable')}</p>
        )}
        {snap !== undefined && (
          <>
            <ResourceRow
              label={t('dashboard:resources.cpu')}
              unavailableLabel={t('dashboard:resources.metricUnavailable')}
              percent={snap.cpuPercent}
            />
            <ResourceRow
              label={t('dashboard:resources.memory')}
              unavailableLabel={t('dashboard:resources.metricUnavailable')}
              percent={snap.memoryPercent}
              detail={usedOfTotal(snap.memoryUsedBytes, snap.memoryTotalBytes)}
            />
            <ResourceRow
              label={t('dashboard:resources.disk')}
              unavailableLabel={t('dashboard:resources.metricUnavailable')}
              percent={snap.diskPercent}
              detail={usedOfTotal(snap.diskUsedBytes, snap.diskTotalBytes)}
            />
          </>
        )}
      </PanelBody>
    </Panel>
  );
}
