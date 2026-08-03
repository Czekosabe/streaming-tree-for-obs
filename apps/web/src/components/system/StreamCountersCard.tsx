import type { ParseKeys } from 'i18next';
import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/cn';
import type { PlatformStatus, StreamPlatform } from '@/models/platform';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';

type CounterTone = 'live' | 'starting' | 'offline' | 'error';

const COUNTER_CLASSES: Record<CounterTone, string> = {
  live: 'text-status-live',
  starting: 'text-status-starting',
  offline: 'text-status-offline',
  error: 'text-status-error',
};

/**
 * Each counter carries a short visible label and a pluralized accessible label,
 * so a screen reader announces "2 live branches" rather than "2, Live".
 */
const COUNTERS: readonly {
  tone: CounterTone;
  status: PlatformStatus;
  labelKey: ParseKeys<'dashboard'>;
  accessibleKey: ParseKeys<'dashboard'>;
}[] = [
  {
    tone: 'live',
    status: 'live',
    labelKey: 'counters.live',
    accessibleKey: 'counters.liveAccessible',
  },
  {
    tone: 'starting',
    status: 'starting',
    labelKey: 'counters.starting',
    accessibleKey: 'counters.startingAccessible',
  },
  {
    tone: 'offline',
    status: 'offline',
    labelKey: 'counters.offline',
    accessibleKey: 'counters.offlineAccessible',
  },
  {
    tone: 'error',
    status: 'error',
    labelKey: 'counters.error',
    accessibleKey: 'counters.errorAccessible',
  },
];

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
  const { t } = useTranslation('dashboard');

  return (
    <Panel>
      <PanelHeader
        title={t('counters.heading')}
        description={t('counters.description')}
        icon={<Activity className="size-4" />}
        headingLevel={3}
      />
      <PanelBody>
        <div className="grid grid-cols-2 gap-2">
          {COUNTERS.map((counter) => {
            const value = countByStatus(platforms, counter.status);
            return (
              <div
                key={counter.tone}
                className="rounded-lg border border-line bg-surface-sunken px-3 py-2.5"
                aria-label={t(counter.accessibleKey, { count: value })}
                role="group"
              >
                <p
                  aria-hidden="true"
                  className={cn(
                    'font-mono text-xl leading-none tabular-nums',
                    COUNTER_CLASSES[counter.tone],
                  )}
                >
                  {value}
                </p>
                <p
                  aria-hidden="true"
                  className="mt-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-faint"
                >
                  {t(counter.labelKey)}
                </p>
              </div>
            );
          })}
        </div>
      </PanelBody>
    </Panel>
  );
}
