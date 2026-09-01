import type { ComponentType } from 'react';

import { AccountsStep } from './steps/AccountsStep';
import { CreatorToolsStep } from './steps/CreatorToolsStep';
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
  | 'creatorTools'
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
 * 'summary'; 21D inserted creator-tools discovery and enriched
 * 'summary' itself with a real per-category readiness view - all
 * without needing to restructure this list's own shape.
 */
export const ONBOARDING_STEPS: readonly OnboardingStepDefinition[] = [
  { id: 'welcome', Component: WelcomeStep },
  { id: 'readiness', Component: ReadinessStep },
  { id: 'obsConnection', Component: ObsConnectionStep },
  { id: 'destinations', Component: DestinationsStep },
  { id: 'accounts', Component: AccountsStep },
  { id: 'creatorTools', Component: CreatorToolsStep },
  { id: 'summary', Component: SummaryStep },
];
