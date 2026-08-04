import { useTranslation } from 'react-i18next';

import { useHealthQuery } from '@/hooks/use-health-query';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { cn } from '@/lib/cn';
import { summarizeSystem } from '@/models/runtime-presentation';

import { StatusBadge } from '../ui/StatusBadge';

/**
 * Aggregated system indicator in the top bar.
 *
 * It distinguishes backend availability from the ingest component's state, and
 * never calls the system operational while MediaMTX is missing or failed - a
 * dashboard claiming "all good" with no ingest service would be worse than no
 * summary at all.
 *
 * It also never implies that configured destination platforms are live, because
 * nothing is being sent to them.
 */
export function SystemStatusPill({ className }: { className?: string }) {
  const { t } = useTranslation('runtime');
  const health = useHealthQuery();
  const runtime = useRuntimeQuery();

  const loading = health.isPending || runtime.isPending;
  const backendReachable = !health.isError && !runtime.isError;

  const summary = summarizeSystem(runtime.data, backendReachable, loading);

  return (
    <StatusBadge
      status={summary.tone}
      label={t(summary.labelKey)}
      className={cn('h-8 px-3 py-0 normal-case', className)}
    />
  );
}
