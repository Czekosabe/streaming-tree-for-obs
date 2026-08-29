import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { QuickActionsCard } from './QuickActionsCard';
import { ServicesCard } from './ServicesCard';
import { StreamCountersCard } from './StreamCountersCard';
import { SystemResourcesCard } from './SystemResourcesCard';
import { UpcomingFeaturesCard } from './UpcomingFeaturesCard';

/**
 * Right-hand status column on wide screens.
 *
 * Below the `xl` breakpoint the dashboard grid drops it underneath the main
 * content instead of squeezing it - see `DashboardPage`.
 *
 * `StreamCountersCard` carries the real live/starting/error/offline ring and
 * configuration counts. `ServicesCard` is real service-dependency health
 * (backend/ingest engine/FFmpeg) - a distinct concept from host resource
 * usage, and named accordingly (an earlier round of this stage called this
 * same content "System resources", which was a real, reported mislabelling:
 * service health and host CPU/memory/disk usage are not the same thing).
 * `SystemResourcesCard` is the real thing that heading now refers to - real
 * local CPU/memory/disk usage sampled by the backend's
 * `internal/sysresources` collector, never fabricated, never remote
 * telemetry. `QuickActionsCard` surfaces the same canonical start-enabled/
 * stop-all/refresh actions `StreamsPage` already exposes, never a second
 * implementation of them. `UpcomingFeaturesCard` lists the same real
 * planned-feature copy the `/platforms` and `/metadata` placeholder routes
 * already show.
 *
 * Deliberately does not show backend connectivity/version/uptime *detail*
 * (Service/Version/Uptime numbers): that developer-facing framing moved to
 * the Logs & Diagnostics page - see `BackendHealthCard`'s own usage in
 * `LogsPage`. `ServicesCard` shows only real connectivity/health status
 * here, not those raw fields.
 */
export function SystemStatusRail({ platforms }: { platforms: readonly ConfiguredPlatform[] }) {
  const { t } = useTranslation('dashboard');

  return (
    <aside aria-label={t('systemStatus.railLabel')} className="space-y-4">
      <StreamCountersCard platforms={platforms} />
      <ServicesCard />
      <SystemResourcesCard />
      <QuickActionsCard platforms={platforms} />
      <UpcomingFeaturesCard />
    </aside>
  );
}
