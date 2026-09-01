import { Compass, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import { Button } from '@/components/ui/Button';
import { useOnboardingStateQuery } from '@/hooks/use-onboarding';

/**
 * Small, dismissible Dashboard affordance for an operator who has not
 * completed onboarding (docs/onboarding.md §5.4) - never a large
 * persistent Dashboard region. Dismissing this banner only hides it for
 * the current session (plain component state, reset on reload); it does
 * not set the persisted onboarding status to 'dismissed' - only the
 * assistant's own "Skip setup" action does that. Renders nothing while
 * the state is still loading or once it is 'completed'.
 */
export function OnboardingDashboardBanner() {
  const { t } = useTranslation('onboarding');
  const navigate = useNavigate();
  const stateQuery = useOnboardingStateQuery();
  const [dismissed, setDismissed] = useState(false);

  if (dismissed) return null;
  if (stateQuery.data === undefined) return null;
  if (stateQuery.data.status === 'completed') return null;

  return (
    <div className="mb-4 flex items-center gap-3 rounded-lg border border-line bg-surface-raised px-3 py-2.5 xl:mb-5">
      <span
        aria-hidden="true"
        className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-accent-soft"
      >
        <Compass className="size-4" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-ink">{t('dashboardBanner.heading')}</p>
        <p className="text-xs text-ink-muted">{t('dashboardBanner.body')}</p>
      </div>
      <Button size="sm" variant="primary" onClick={() => void navigate('/onboarding')}>
        {t('dashboardBanner.action')}
      </Button>
      <button
        type="button"
        onClick={() => setDismissed(true)}
        aria-label={t('dashboardBanner.dismiss')}
        title={t('dashboardBanner.dismiss')}
        className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-surface-hover hover:text-ink"
      >
        <X aria-hidden="true" className="size-3.5" />
      </button>
    </div>
  );
}
