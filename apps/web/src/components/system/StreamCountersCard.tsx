import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { useBranchRuntimeQuery } from '@/hooks/use-branches';
import { cn } from '@/lib/cn';
import { branchFor, branchTone } from '@/models/branch-presentation';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';
import { OverallStatusRing } from './OverallStatusRing';

/**
 * Overall stream status: a real live/starting/error/offline ring built from
 * `GET /api/branches`, plus configured/enabled/disabled counts underneath.
 *
 * The ring reuses `branchTone` - the exact same live/starting/error/offline
 * semantics already used by every branch status badge in this codebase
 * (`BranchControls`, `StreamsPage`) - rather than inventing a second
 * vocabulary here. A destination with no tracked branch yet (before the
 * first fetch resolves) counts as idle/offline, matching `branchFor`'s own
 * documented convention.
 */
export function StreamCountersCard({ platforms }: { platforms: readonly ConfiguredPlatform[] }) {
  const { t } = useTranslation('dashboard');
  const branchesQuery = useBranchRuntimeQuery();

  const total = platforms.length;
  const enabled = platforms.filter((platform) => platform.enabled).length;
  const disabled = total - enabled;

  const tones = platforms.map((platform) =>
    branchTone(branchFor(branchesQuery.data, platform.id)?.state ?? 'idle'),
  );
  const live = tones.filter((tone) => tone === 'live').length;
  const starting = tones.filter((tone) => tone === 'starting').length;
  const errorCount = tones.filter((tone) => tone === 'error').length;
  const offline = tones.filter((tone) => tone === 'offline').length;

  const counters = [
    {
      key: 'configured',
      value: total,
      label: t('counters.configured'),
      accessible: t('counters.configuredAccessible', { count: total }),
      tone: 'text-ink',
    },
    {
      key: 'enabled',
      value: enabled,
      label: t('counters.enabled'),
      accessible: t('counters.enabledAccessible', { count: enabled }),
      tone: 'text-accent-soft',
    },
    {
      key: 'disabled',
      value: disabled,
      label: t('counters.disabled'),
      accessible: t('counters.disabledAccessible', { count: disabled }),
      tone: 'text-status-offline',
    },
  ];

  return (
    <Panel>
      <PanelHeader
        title={t('counters.heading')}
        description={t('counters.description')}
        icon={<Activity className="size-4" />}
        headingLevel={3}
      />
      <PanelBody className="space-y-4">
        <OverallStatusRing live={live} starting={starting} error={errorCount} offline={offline} />

        <div className="grid grid-cols-3 gap-2 border-t border-line pt-3">
          {counters.map((counter) => (
            <div
              key={counter.key}
              role="group"
              aria-label={counter.accessible}
              className="rounded-lg bg-surface-sunken/70 px-3 py-2.5"
            >
              <p
                aria-hidden="true"
                className={cn('font-mono text-xl leading-none tabular-nums', counter.tone)}
              >
                {counter.value}
              </p>
              <p
                aria-hidden="true"
                className="mt-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-faint"
              >
                {counter.label}
              </p>
            </div>
          ))}
        </div>
      </PanelBody>
    </Panel>
  );
}
