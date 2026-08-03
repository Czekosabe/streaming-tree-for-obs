import { Activity } from 'lucide-react';

import { cn } from '@/lib/cn';
import type { PlatformStatus, StreamPlatform } from '@/models/platform';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';

const COUNTER_CLASSES: Record<'live' | 'starting' | 'offline' | 'error', string> = {
  live: 'text-status-live',
  starting: 'text-status-starting',
  offline: 'text-status-offline',
  error: 'text-status-error',
};

function Counter({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: keyof typeof COUNTER_CLASSES;
}) {
  return (
    <div className="rounded-lg border border-line bg-surface-sunken px-3 py-2.5">
      <p className={cn('font-mono text-xl leading-none tabular-nums', COUNTER_CLASSES[tone])}>
        {value}
      </p>
      <p className="mt-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
        {label}
      </p>
    </div>
  );
}

function countByStatus(platforms: readonly StreamPlatform[], status: PlatformStatus): number {
  return platforms.filter((platform) => platform.status === status).length;
}

/**
 * Branch counters.
 *
 * These are derived from the DEMO store, so they are real counts of local demo
 * state - not measurements of actual transmissions.
 */
export function StreamCountersCard({ platforms }: { platforms: readonly StreamPlatform[] }) {
  return (
    <Panel>
      <PanelHeader
        title="Stream branches"
        description="Counted from local demo state"
        icon={<Activity className="size-4" />}
        headingLevel={3}
      />
      <PanelBody>
        <div className="grid grid-cols-2 gap-2">
          <Counter label="Live" value={countByStatus(platforms, 'live')} tone="live" />
          <Counter label="Starting" value={countByStatus(platforms, 'starting')} tone="starting" />
          <Counter label="Offline" value={countByStatus(platforms, 'offline')} tone="offline" />
          <Counter label="Error" value={countByStatus(platforms, 'error')} tone="error" />
        </div>
      </PanelBody>
    </Panel>
  );
}
