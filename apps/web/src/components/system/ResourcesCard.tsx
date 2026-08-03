import { Cpu, Signal } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { DEMO_NETWORK_STATUS, DEMO_RESOURCE_METRICS } from '@/data/demo-system';

import { DemoBadge } from '../ui/DemoBadge';
import { Meter } from '../ui/Meter';
import { Panel, PanelBody, PanelHeader } from '../ui/Panel';

/**
 * Host resources.
 *
 * ALL values here are static demo constants - the backend does not collect host
 * metrics yet. The panel carries a permanent "Demo" badge so the numbers are
 * never mistaken for measurements.
 */
export function ResourcesCard() {
  const { t } = useTranslation(['dashboard', 'common']);

  return (
    <Panel>
      <PanelHeader
        title={t('dashboard:resources.heading')}
        description={t('dashboard:resources.description')}
        icon={<Cpu className="size-4" />}
        headingLevel={3}
        actions={<DemoBadge title={t('dashboard:resources.demoTooltip')} />}
      />
      <PanelBody className="space-y-4">
        {DEMO_RESOURCE_METRICS.map((metric) => (
          <Meter
            key={metric.id}
            label={t(metric.labelKey)}
            value={metric.usagePercent}
            detail={t(metric.detailKey)}
          />
        ))}

        <div className="flex items-start justify-between gap-3 border-t border-line pt-3">
          <div className="flex items-start gap-2">
            <Signal aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-status-live" />
            <div>
              <p className="text-xs font-medium text-ink">{t(DEMO_NETWORK_STATUS.statusKey)}</p>
              <p className="mt-0.5 text-[11px] text-ink-faint">
                {t(DEMO_NETWORK_STATUS.detailKey)}
              </p>
            </div>
          </div>
          <p className="shrink-0 font-mono text-xs tabular-nums text-ink-muted">
            {t('common:units.megabitsPerSecond', {
              value: DEMO_NETWORK_STATUS.uploadMbps.toFixed(1),
            })}
          </p>
        </div>
      </PanelBody>
    </Panel>
  );
}
