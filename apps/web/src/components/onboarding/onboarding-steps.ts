import type { ComponentType } from 'react';

import { AccountsStep } from './steps/AccountsStep';
import { DestinationsStep } from './steps/DestinationsStep';
import { ObsConnectionStep } from './steps/ObsConnectionStep';
import { ReadinessStep } from './steps/ReadinessStep';
import { SummaryStep } from './steps/SummaryStep';
import { WelcomeStep } from './steps/WelcomeStep';

export type OnboardingStepId =
  | 'welcome'
  | 'readiness'
  | 'obsConnection'
  | 'destinations'
  | 'accounts'
  | 'summary';

export type OnboardingStepDefinition = {
  id: OnboardingStepId;
  Component: ComponentType;
};

/**
 * The onboarding flow's own step order.
 *
 * A plain ordered array, not a more elaborate framework. 21C inserted
 * readiness/OBS-connection/destinations/accounts between 'welcome' and
 * 'summary'; 21D enriches 'summary' itself without needing to
 * restructure this list's own shape.
 */
export const ONBOARDING_STEPS: readonly OnboardingStepDefinition[] = [
  { id: 'welcome', Component: WelcomeStep },
  { id: 'readiness', Component: ReadinessStep },
  { id: 'obsConnection', Component: ObsConnectionStep },
  { id: 'destinations', Component: DestinationsStep },
  { id: 'accounts', Component: AccountsStep },
  { id: 'summary', Component: SummaryStep },
];
