import type { ComponentType } from 'react';

import { SummaryStep } from './steps/SummaryStep';
import { WelcomeStep } from './steps/WelcomeStep';

export type OnboardingStepId = 'welcome' | 'summary';

export type OnboardingStepDefinition = {
  id: OnboardingStepId;
  Component: ComponentType;
};

/**
 * The onboarding flow's own step order.
 *
 * A plain ordered array, not a more elaborate framework - later
 * substages (docs/onboarding.md §6-§7) insert further steps between
 * 'welcome' and 'summary' without needing to restructure this list's
 * own shape.
 */
export const ONBOARDING_STEPS: readonly OnboardingStepDefinition[] = [
  { id: 'welcome', Component: WelcomeStep },
  { id: 'summary', Component: SummaryStep },
];
