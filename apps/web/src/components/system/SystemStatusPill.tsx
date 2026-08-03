import { useTranslation } from 'react-i18next';

import { useHealthQuery } from '@/hooks/use-health-query';
import { usePlatformsQuery } from '@/hooks/use-platforms';
import { cn } from '@/lib/cn';
import type { PlatformStatus } from '@/models/platform';

import { StatusBadge } from '../ui/StatusBadge';

/**
 * Aggregated system indicator in the top bar.
 *
 * It reports the state of the SYSTEM, not of any transmission: whether the
 * backend answers and how many destinations are configured and enabled. No
 * live/starting state is ever shown, because nothing streams yet.
 */
export function SystemStatusPill({ className }: { className?: string }) {
  const { t } = useTranslation('dashboard');
  const { isError: backendDown, isPending: healthPending } = useHealthQuery();
  const platformsQuery = usePlatformsQuery();

  let status: PlatformStatus = 'offline';
  let label = t('systemStatus.idle');

  if (healthPending || platformsQuery.isPending) {
    status = 'starting';
    label = t('systemStatus.checking');
  } else if (backendDown) {
    status = 'error';
    label = t('systemStatus.backendUnavailable');
  } else if (platformsQuery.isError) {
    status = 'error';
    label = t('systemStatus.configurationUnavailable');
  } else {
    const enabled = (platformsQuery.data ?? []).filter((platform) => platform.enabled).length;
    if (enabled > 0) {
      // Deliberately "enabled", not "live": these destinations are configured
      // and ready, and nothing is transmitting.
      label = t('systemStatus.enabled', { count: enabled });
    }
  }

  return (
    <StatusBadge
      status={status}
      label={label}
      className={cn('h-8 px-3 py-0 normal-case', className)}
    />
  );
}
