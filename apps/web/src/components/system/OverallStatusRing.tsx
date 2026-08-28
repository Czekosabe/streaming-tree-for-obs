import { useTranslation } from 'react-i18next';

import { StatusDot } from '@/components/ui/StatusBadge';

type OverallStatusRingProps = {
  live: number;
  starting: number;
  error: number;
  offline: number;
};

/** Resolved CSS colour for each segment - kept in one place so the ring's
 * `conic-gradient` and the legend dots it sits beside always agree. Values
 * are this project's own semantic status tokens (index.css), not arbitrary
 * hex - the same green/blue/red/grey vocabulary every status badge uses. */
const SEGMENT_COLOR = {
  live: 'var(--color-status-live)',
  starting: 'var(--color-status-starting)',
  error: 'var(--color-status-error)',
  offline: 'var(--color-status-offline)',
} as const;

/**
 * Real destination-state summary as a donut ring: a lightweight decorative
 * `conic-gradient` (no chart dependency) with the real active-destination
 * count centred inside, and a plain-text legend beside it carrying the same
 * counts so the information is never only conveyed by the ring's colour or
 * proportions. The ring itself is `aria-hidden`; the legend is the
 * accessible source of truth.
 */
export function OverallStatusRing({ live, starting, error, offline }: OverallStatusRingProps) {
  const { t } = useTranslation('dashboard');
  const total = live + starting + error + offline;

  if (total === 0) {
    return <p className="text-[11px] text-ink-faint">{t('counters.runtimeIdle')}</p>;
  }

  // Cumulative degree boundaries for each segment, in a fixed, stable order
  // (live, starting, error, offline) so a segment with 0 count simply has
  // zero width rather than needing to be conditionally omitted.
  const liveDeg = (live / total) * 360;
  const startingDeg = liveDeg + (starting / total) * 360;
  const errorDeg = startingDeg + (error / total) * 360;

  const gradient = `conic-gradient(${SEGMENT_COLOR.live} 0deg ${liveDeg}deg, ${SEGMENT_COLOR.starting} ${liveDeg}deg ${startingDeg}deg, ${SEGMENT_COLOR.error} ${startingDeg}deg ${errorDeg}deg, ${SEGMENT_COLOR.offline} ${errorDeg}deg 360deg)`;

  const legendItems = [
    { key: 'live' as const, count: live, label: t('counters.runtimeLive', { count: live }) },
    {
      key: 'starting' as const,
      count: starting,
      label: t('counters.runtimeStarting', { count: starting }),
    },
    { key: 'error' as const, count: error, label: t('counters.runtimeError', { count: error }) },
    {
      key: 'offline' as const,
      count: offline,
      label: t('counters.configuredOffline', { count: offline }),
    },
  ].filter((item) => item.count > 0);

  return (
    <div className="flex items-center gap-4">
      <div
        aria-hidden="true"
        className="relative flex size-24 shrink-0 items-center justify-center rounded-full"
        style={{ background: gradient }}
      >
        <div className="flex size-18 flex-col items-center justify-center rounded-full bg-surface">
          <span className="font-mono text-2xl leading-none font-bold text-ink tabular-nums">
            {live}
          </span>
          <span className="mt-0.5 text-[9px] font-semibold uppercase tracking-widest text-ink-faint">
            {t('counters.ringLiveLabel')}
          </span>
        </div>
      </div>

      <ul className="min-w-0 flex-1 space-y-1.5">
        {legendItems.map((item) => (
          <li key={item.key} className="flex items-center gap-2 text-xs text-ink-muted">
            <StatusDot status={item.key} />
            <span className="truncate">{item.label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
