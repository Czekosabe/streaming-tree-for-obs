import { ApiError, apiGet, apiPut } from '@/lib/api-client';

import {
  onboardingStateSchema,
  ONBOARDING_SCHEMA_VERSION,
  type OnboardingState,
  type OnboardingStatus,
} from './onboarding-schemas';

function assertSchemaVersion(state: OnboardingState): OnboardingState {
  if (state.version !== ONBOARDING_SCHEMA_VERSION) {
    // A different schema version means the backend and this build disagree
    // about the payload. Rendering it anyway would risk auto-showing (or
    // hiding) onboarding based on a misread status.
    throw new ApiError(
      'parse',
      `The backend returned onboarding schema version ${state.version}, but this build understands ${ONBOARDING_SCHEMA_VERSION}.`,
    );
  }
  return state;
}

export async function fetchOnboardingState(signal?: AbortSignal): Promise<OnboardingState> {
  const state = await apiGet('/api/onboarding', onboardingStateSchema, { signal });
  return assertSchemaVersion(state);
}

export async function setOnboardingStatus(status: OnboardingStatus): Promise<OnboardingState> {
  const state = await apiPut('/api/onboarding', { status }, onboardingStateSchema);
  return assertSchemaVersion(state);
}
