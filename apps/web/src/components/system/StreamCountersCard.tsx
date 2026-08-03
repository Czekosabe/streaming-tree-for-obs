import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';
import { cn } from '@/lib/cn';

import { Panel, PanelBody, PanelHeader } from '../ui/Panel';

/**
 * Configured destination counters.
 *
 * These count CONFIGURATION, not runtime state: how many destinations exist and
 * how many are enabled. There is no live/starting/error breakdown, because no
 * streaming engine exists to produce one - inventing those counters next to
 * real saved data would be misleading.
 */
export function StreamCountersCard({ platforms }: { platforms: readonly ConfiguredPlatform[] }) {
  const { t } = useTranslation('dashboard');

  const total = platforms.length;
  const enabled = platforms.filter((platform) => platform.enabled).length;
  const disabled = total - enabled;

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
      <PanelBody className="space-y-3">
        <div className="grid grid-cols-3 gap-2">
          {counters.map((counter) => (
            <div
              key={counter.key}
              role="group"
              aria-label={counter.accessible}
              className="rounded-lg border border-line bg-surface-sunken px-3 py-2.5"
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

        <p className="text-[11px] leading-relaxed text-ink-faint">
          {t('counters.noRuntimeState')}
        </p>
      </PanelBody>
    </Panel>
  );
}
