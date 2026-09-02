import { useTranslation } from 'react-i18next';

import { SystemStatusPill } from '@/components/system/SystemStatusPill';
import { StatusDot } from '@/components/ui/StatusBadge';
import { useAccountsQuery } from '@/hooks/use-accounts';
import { useBranchRuntimeQuery } from '@/hooks/use-branches';
import { usePlatformsConfiguredCount } from '@/hooks/use-credentials';
import { usePlatformsQuery } from '@/hooks/use-platforms';
import { useRuntimeQuery } from '@/hooks/use-runtime';
import { ingestStateKey, ingestTone } from '@/models/runtime-presentation';

/**
 * Final step: a real readiness summary (docs/onboarding.md §7.2), then
 * closes the assistant itself.
 *
 * Every category reuses real state already fetched elsewhere in the
 * app - `SystemStatusPill` (the same aggregated indicator the top bar
 * uses) for Application, `useRuntimeQuery` for OBS ingest,
 * `usePlatformsQuery`/`useBranchRuntimeQuery`/`usePlatformsConfiguredCount`
 * for destinations, `useAccountsQuery` for connected accounts. No category
 * is ever rendered as a failure merely for being optional or not yet
 * configured - zero destinations and zero connected accounts are both
 * valid, complete-able states.
 *
 * "Configured" here means the same thing `PlatformCard`'s own "Stored"
 * credential badge means - a destination with a stream key actually saved,
 * not merely a destination card that exists. A destination created but
 * never given a stream key (including a seeded placeholder one) counts
 * toward the total, never toward "configured" - see docs/progress.md,
 * Stage 20E findings batch 1, defect D.
 */
export function SummaryStep() {
  const { t } = useTranslation(['onboarding', 'runtime']);
  const runtimeQuery = useRuntimeQuery();
  const platformsQuery = usePlatformsQuery();
  const branchesQuery = useBranchRuntimeQuery();
  const accountsQuery = useAccountsQuery();

  const ingest = runtimeQuery.data?.ingest;
  const platforms = platformsQuery.data ?? [];
  const branches = branchesQuery.data ?? [];
  const platformIds = platforms.map((platform) => platform.id);
  const { configuredCount } = usePlatformsConfiguredCount(platformIds);
  const enabledCount = platforms.filter((p) => p.enabled).length;
  const activeCount = branches.filter((b) => b.state === 'live').length;
  const accounts = accountsQuery.data ?? [];

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">{t('summary.heading')}</h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('summary.body')}</p>

      <ul className="space-y-2">
        <li className="flex items-center justify-between gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm">
          <span className="text-ink-muted">{t('summary.categories.application')}</span>
          <SystemStatusPill />
        </li>

        {ingest !== undefined && (
          <li className="flex items-center justify-between gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm">
            <span className="text-ink-muted">{t('summary.categories.obsIngest')}</span>
            <span className="flex items-center gap-1.5 font-medium text-ink">
              <StatusDot status={ingestTone(ingest.state)} />
              {t(`runtime:${ingestStateKey(ingest.state)}`)}
            </span>
          </li>
        )}

        <li className="flex items-center justify-between gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm">
          <span className="text-ink-muted">{t('summary.categories.destinations')}</span>
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink">
              {t('summary.destinationsCount', {
                total: platforms.length,
                configured: configuredCount,
                enabled: enabledCount,
                active: activeCount,
              })}
            </span>
            <span className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('summary.optional')}
            </span>
          </span>
        </li>

        <li className="flex items-center justify-between gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2 text-sm">
          <span className="text-ink-muted">{t('summary.categories.accounts')}</span>
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink">
              {t('summary.accountsCount', { count: accounts.length })}
            </span>
            <span className="text-[10px] font-semibold uppercase tracking-widest text-ink-faint">
              {t('summary.optional')}
            </span>
          </span>
        </li>
      </ul>
    </div>
  );
}
