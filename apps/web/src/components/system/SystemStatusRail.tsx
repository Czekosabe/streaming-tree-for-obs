import type { StreamPlatform } from '@/models/platform';

import { BackendHealthCard } from './BackendHealthCard';
import { ResourcesCard } from './ResourcesCard';
import { StreamCountersCard } from './StreamCountersCard';

/**
 * Right-hand status column on wide screens.
 *
 * Below the `xl` breakpoint the dashboard grid drops it underneath the main
 * content instead of squeezing it - see `DashboardPage`.
 */
export function SystemStatusRail({ platforms }: { platforms: readonly StreamPlatform[] }) {
  return (
    <aside aria-label="System status" className="space-y-4">
      <BackendHealthCard />
      <StreamCountersCard platforms={platforms} />
      <ResourcesCard />
    </aside>
  );
}
