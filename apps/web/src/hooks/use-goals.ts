import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  createGoal,
  createWidgetProfile,
  deleteGoal,
  deleteWidgetProfile,
  fetchGoal,
  fetchGoals,
  fetchWidgetProfiles,
  resetGoal,
  rotateWidgetProfileSlug,
  setGoalCurrent,
  updateGoal,
  updateWidgetProfile,
} from '@/api/goals';
import type { Goal, GoalInput, WidgetProfile, WidgetProfileInput } from '@/api/goals-schemas';

export const goalsKeys = {
  goals: () => ['goals'] as const,
  goal: (id: string) => ['goals', id] as const,
  widgetProfiles: (goalId: string) => ['widget-profiles', goalId] as const,
};

function useInvalidateGoals() {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: goalsKeys.goals() });
}

function useInvalidateWidgetProfiles(goalId: string) {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: goalsKeys.widgetProfiles(goalId) });
}

export function useGoalsQuery(): UseQueryResult<Goal[], Error> {
  return useQuery({ queryKey: goalsKeys.goals(), queryFn: ({ signal }) => fetchGoals(signal) });
}

export function useGoalQuery(id: string | null): UseQueryResult<Goal, Error> {
  return useQuery({
    queryKey: goalsKeys.goal(id ?? ''),
    queryFn: ({ signal }) => fetchGoal(id ?? '', signal),
    enabled: id !== null,
  });
}

export function useCreateGoalMutation(): UseMutationResult<Goal, Error, GoalInput> {
  const invalidate = useInvalidateGoals();
  return useMutation({ mutationFn: (input: GoalInput) => createGoal(input), onSuccess: invalidate });
}

export function useUpdateGoalMutation(): UseMutationResult<Goal, Error, { id: string; input: GoalInput }> {
  const invalidate = useInvalidateGoals();
  return useMutation({ mutationFn: ({ id, input }) => updateGoal(id, input), onSuccess: invalidate });
}

export function useDeleteGoalMutation(): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateGoals();
  return useMutation({ mutationFn: (id: string) => deleteGoal(id), onSuccess: invalidate });
}

export function useSetGoalCurrentMutation(): UseMutationResult<Goal, Error, { id: string; current: number }> {
  const invalidate = useInvalidateGoals();
  return useMutation({ mutationFn: ({ id, current }) => setGoalCurrent(id, current), onSuccess: invalidate });
}

export function useResetGoalMutation(): UseMutationResult<Goal, Error, string> {
  const invalidate = useInvalidateGoals();
  return useMutation({ mutationFn: (id: string) => resetGoal(id), onSuccess: invalidate });
}

export function useWidgetProfilesQuery(goalId: string | null): UseQueryResult<WidgetProfile[], Error> {
  return useQuery({
    queryKey: goalsKeys.widgetProfiles(goalId ?? ''),
    queryFn: ({ signal }) => fetchWidgetProfiles(goalId ?? '', signal),
    enabled: goalId !== null,
  });
}

export function useCreateWidgetProfileMutation(
  goalId: string,
): UseMutationResult<WidgetProfile, Error, WidgetProfileInput> {
  const invalidate = useInvalidateWidgetProfiles(goalId);
  return useMutation({ mutationFn: (input: WidgetProfileInput) => createWidgetProfile(input), onSuccess: invalidate });
}

export function useUpdateWidgetProfileMutation(
  goalId: string,
): UseMutationResult<WidgetProfile, Error, { id: string; input: WidgetProfileInput }> {
  const invalidate = useInvalidateWidgetProfiles(goalId);
  return useMutation({ mutationFn: ({ id, input }) => updateWidgetProfile(id, input), onSuccess: invalidate });
}

export function useDeleteWidgetProfileMutation(goalId: string): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateWidgetProfiles(goalId);
  return useMutation({ mutationFn: (id: string) => deleteWidgetProfile(id), onSuccess: invalidate });
}

export function useRotateWidgetProfileSlugMutation(
  goalId: string,
): UseMutationResult<WidgetProfile, Error, string> {
  const invalidate = useInvalidateWidgetProfiles(goalId);
  return useMutation({ mutationFn: (id: string) => rotateWidgetProfileSlug(id), onSuccess: invalidate });
}
