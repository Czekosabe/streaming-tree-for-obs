import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';

import { ONBOARDING_STEPS } from '@/components/onboarding/onboarding-steps';
import { BrandMark } from '@/components/layout/BrandMark';
import { Button } from '@/components/ui/Button';
import { Panel } from '@/components/ui/Panel';
import { useSetOnboardingStatusMutation } from '@/hooks/use-onboarding';

/**
 * Stage 21 first-run onboarding assistant (docs/onboarding.md §5.1).
 *
 * A dedicated route, not a modal: each step is a real page section with
 * its own heading and natural focus order, avoiding the focus-trap/
 * z-layer complexity a giant modal would need for the same multi-step
 * flow. Deliberately outside AppShell's sidebar/nav chrome - a first-run
 * user should not have to parse the full navigation while being
 * onboarded - but with its own minimal, consistent header.
 */
export function OnboardingPage() {
  const { t } = useTranslation('onboarding');
  const navigate = useNavigate();
  const setStatus = useSetOnboardingStatusMutation();
  const [stepIndex, setStepIndex] = useState(0);
  const headingRef = useRef<HTMLDivElement>(null);

  const step = ONBOARDING_STEPS[stepIndex];
  const isFirstStep = stepIndex === 0;
  const isLastStep = stepIndex === ONBOARDING_STEPS.length - 1;
  const StepComponent = step?.Component;

  // Moves focus to the new step's own content on every step change, so a
  // keyboard/screen-reader user lands where the new heading is rather
  // than staying on the now-stale "Continue" button position.
  useEffect(() => {
    headingRef.current?.focus();
  }, [stepIndex]);

  // Navigates only once the status is actually persisted (`onSuccess`), never
  // unconditionally (`onSettled`): navigating away on a failed save is what
  // previously let the assistant say "Setup is complete" while the Dashboard,
  // reading the real still-unpersisted status, showed "Setup incomplete"
  // right after - see docs/progress.md, Stage 20E findings batch 1, defect E.
  const finish = (status: 'completed' | 'dismissed') => {
    setStatus.mutate(status, {
      onSuccess: () => void navigate('/'),
    });
  };

  const handleContinue = () => {
    if (isLastStep) {
      finish('completed');
      return;
    }
    setStepIndex((index) => Math.min(index + 1, ONBOARDING_STEPS.length - 1));
  };

  const handleBack = () => {
    setStepIndex((index) => Math.max(index - 1, 0));
  };

  const handleSkip = () => finish('dismissed');

  if (StepComponent === undefined) return null;

  return (
    <div className="flex min-h-dvh flex-col bg-canvas">
      <header className="border-b border-line px-4 py-3 sm:px-6">
        <div className="mx-auto flex max-w-2xl items-center justify-between gap-3">
          <BrandMark />
          <div className="flex items-center gap-3">
            <span className="text-xs text-ink-faint">
              {t('header.stepIndicator', { current: stepIndex + 1, total: ONBOARDING_STEPS.length })}
            </span>
            <Button variant="ghost" size="sm" onClick={handleSkip} disabled={setStatus.isPending}>
              {t('header.skip')}
            </Button>
          </div>
        </div>
      </header>

      <main className="flex flex-1 items-start justify-center px-4 py-8 sm:px-6">
        <div className="w-full max-w-2xl space-y-5">
          <Panel raised className="p-5 sm:p-6">
            {/* tabIndex=-1: a programmatic focus target, never a tab stop -
                matching AppShell's own #main-content skip-link convention.
                No outline override here: the app's own global
                :focus-visible rule (index.css) must still show a real
                visible focus ring when this receives focus. */}
            <div ref={headingRef} tabIndex={-1}>
              <StepComponent />
            </div>
          </Panel>

          {setStatus.isError && (
            <p role="alert" className="text-sm text-status-error">
              {t('nav.finishError')}
            </p>
          )}

          <div className="flex items-center justify-between gap-3">
            <Button variant="secondary" onClick={handleBack} disabled={isFirstStep}>
              {t('nav.back')}
            </Button>
            <Button variant="primary" onClick={handleContinue} disabled={setStatus.isPending}>
              {isLastStep ? t('nav.finish') : t('nav.continue')}
            </Button>
          </div>
        </div>
      </main>
    </div>
  );
}
