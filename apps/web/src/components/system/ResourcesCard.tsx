import { Cpu, Signal } from 'lucide-react';

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
  return (
    <Panel>
      <PanelHeader
        title="System resources"
        description="Placeholder values"
        icon={<Cpu className="size-4" />}
        headingLevel={3}
        actions={<DemoBadge title="Host metrics are not collected by the backend yet" />}
      />
      <PanelBody className="space-y-4">
        {DEMO_RESOURCE_METRICS.map((metric) => (
          <Meter
            key={metric.id}
            label={metric.label}
            value={metric.usagePercent}
            detail={metric.detail}
          />
        ))}

        <div className="flex items-start justify-between gap-3 border-t border-line pt-3">
          <div className="flex items-start gap-2">
            <Signal aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-status-live" />
            <div>
              <p className="text-xs font-medium text-ink">Network: {DEMO_NETWORK_STATUS.label}</p>
              <p className="mt-0.5 text-[11px] text-ink-faint">{DEMO_NETWORK_STATUS.detail}</p>
            </div>
          </div>
          <p className="shrink-0 font-mono text-xs tabular-nums text-ink-muted">
            {DEMO_NETWORK_STATUS.uploadMbps.toFixed(1)} Mb/s
          </p>
        </div>
      </PanelBody>
    </Panel>
  );
}
