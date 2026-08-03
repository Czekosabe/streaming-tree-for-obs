import { useTranslation } from 'react-i18next';

import { useHealthQuery } from '@/hooks/use-health-query';
import { cn } from '@/lib/cn';
import type { PlatformStatus } from '@/models/platform';
import { useDemoStream } from '@/state/use-demo-stream';

import { StatusBadge } from '../ui/StatusBadge';

/**
 * Aggregated "whole system" indicator shown in the top bar.
 *
 * Priority: a backend that cannot be reached is the most severe condition,
 * then a branch in error, then live/starting branches, then idle.
 */
export function SystemStatusPill({ className }: { className?: string }) {
  const { t } = useTranslation('dashboard');
  const { platforms } = useDemoStream();
  const { isError: backendDown, isPending } = useHealthQuery();

  const liveCount = platforms.filter((platform) => platform.status === 'live').length;
  const errorCount = platforms.filter((platform) => platform.status === 'error').length;
  const startingCount = platforms.filter((platform) => platform.status === 'starting').length;

  let status: PlatformStatus = 'offline';
  let label = t('systemStatus.idle');

  if (isPending) {
    status = 'starting';
    label = t('systemStatus.checking');
  } else if (backendDown) {
    status = 'error';
    label = t('systemStatus.backendUnavailable');
  } else if (errorCount > 0) {
    status = 'error';
    label = t('systemStatus.errors', { count: errorCount });
  } else if (liveCount > 0) {
    status = 'live';
    label = t('systemStatus.live', { count: liveCount });
  } else if (startingCount > 0) {
    status = 'starting';
    label = t('systemStatus.starting', { count: startingCount });
  }

  return (
    <StatusBadge
      status={status}
      label={label}
      className={cn('h-8 px-3 py-0 normal-case', className)}
    />
  );
}
