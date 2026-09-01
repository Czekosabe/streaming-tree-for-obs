import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import { fetchOnboardingState, setOnboardingStatus } from '@/api/onboarding';
import type { OnboardingState, OnboardingStatus } from '@/api/onboarding-schemas';

/** Query key for the onboarding state. */
export const onboardingKeys = {
  state: ['onboarding'] as const,
};

/**
 * Fetches the persisted onboarding state (docs/onboarding.md §4) - the
 * one source of truth for whether the first-run flow should auto-show,
 * never inferred from localStorage or other frontend-only state.
 *
 * Not polled: this is an operator-driven preference, not runtime state
 * that changes on its own.
 */
export function useOnboardingStateQuery(): UseQueryResult<OnboardingState, Error> {
  return useQuery({
    queryKey: onboardingKeys.state,
    queryFn: ({ signal }) => fetchOnboardingState(signal),
  });
}

export function useSetOnboardingStatusMutation(): UseMutationResult<
  OnboardingState,
  Error,
  OnboardingStatus
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: setOnboardingStatus,
    onSuccess: (state) => {
      queryClient.setQueryData(onboardingKeys.state, state);
    },
  });
}
