import { useTranslation } from 'react-i18next';

import { ServicesCard } from '@/components/system/ServicesCard';
import { RuntimeControls } from '@/components/runtime/RuntimeControls';
import { useRuntimeQuery } from '@/hooks/use-runtime';

/**
 * Step 2: local streaming engine readiness (docs/onboarding.md §6.2).
 *
 * Reuses `ServicesCard` (the Dashboard's own real backend/ingest-engine/
 * FFmpeg health card) and `RuntimeControls` (the same install/start/
 * stop/restart actions `SidebarFooter` already exposes) directly - no
 * new readiness logic, no second health check, no fabricated state.
 */
export function ReadinessStep() {
  const { t } = useTranslation('onboarding');
  const runtimeQuery = useRuntimeQuery();
  const mediaMtx = runtimeQuery.data?.mediaMtx;

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight text-ink">
        {t('readiness.heading')}
      </h2>
      <p className="text-sm leading-relaxed text-ink-muted">{t('readiness.body')}</p>

      <ServicesCard />

      {mediaMtx !== undefined && (
        <div>
          <RuntimeControls mediaMtx={mediaMtx} />
        </div>
      )}
    </div>
  );
}
