import { useTranslation } from 'react-i18next';

import type { ConfiguredPlatform } from '@/api/platform-schemas';

import { QuickActionsCard } from './QuickActionsCard';
import { StreamCountersCard } from './StreamCountersCard';

/**
 * Right-hand status column on wide screens.
 *
 * Below the `xl` breakpoint the dashboard grid drops it underneath the main
 * content instead of squeezing it - see `DashboardPage`.
 *
 * `StreamCountersCard` carries the real live/starting/error/offline ring and
 * configuration counts; `QuickActionsCard` surfaces the same canonical
 * start-enabled/stop-all/refresh actions `StreamsPage` already exposes,
 * never a second implementation of them.
 *
 * Deliberately does not show backend connectivity/version/uptime detail:
 * that developer-facing framing ("Backend" / "Go REST API" / Service /
 * Version / Uptime) moved to the Logs & Diagnostics page, which is the
 * right home for it - see `BackendHealthCard`'s own usage in `LogsPage`.
 * It also does not show host CPU/memory/disk/network: the backend does
 * not collect that data, and a permanently-fake "Demo" card was removed
 * rather than kept for visual symmetry (Stage 20E dashboard realignment -
 * see docs/dashboard-design.md).
 */
export function SystemStatusRail({ platforms }: { platforms: readonly ConfiguredPlatform[] }) {
  const { t } = useTranslation('dashboard');

  return (
    <aside aria-label={t('systemStatus.railLabel')} className="space-y-4">
      <StreamCountersCard platforms={platforms} />
      <QuickActionsCard platforms={platforms} />
    </aside>
  );
}
