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
  fetchWidgetRuntimeStatus,
  resetGoal,
  resetWidgetRuntime,
  rotateWidgetProfileSlug,
  setGoalCurrent,
  updateGoal,
  updateWidgetProfile,
} from '@/api/goals';
import type { Goal, GoalInput, RuntimeStatus, WidgetProfile, WidgetProfileInput } from '@/api/goals-schemas';

export const goalsKeys = {
  goals: () => ['goals'] as const,
  goal: (id: string) => ['goals', id] as const,
  // widgetProfiles: 'all' lists every profile (Widgets/Dashboards tabs -
  // docs/supporter-widgets.md §20); a real goal id lists only that
  // goal's own profiles (the existing Stage 18A per-goal list).
  widgetProfiles: (goalId: string) => ['widget-profiles', goalId] as const,
  widgetProfilesAll: () => ['widget-profiles', 'all'] as const,
  runtimeStatus: (id: string) => ['widget-profile-runtime-status', id] as const,
};

function useInvalidateGoals() {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: goalsKeys.goals() });
}

/** Invalidates every widget-profile list query (both a specific goal's
 * own list and the "all profiles" list Widgets/Dashboards use) - a
 * single widget-profile mutation may affect what either view shows
 * (e.g. creating a dashboard child doesn't change a goal's own list,
 * but every other CRUD action should be visible everywhere). */
function useInvalidateWidgetProfiles() {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: ['widget-profiles'] });
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

/** Lists every widget profile regardless of kind - the Widgets/
 * Dashboards management tabs' own data source (docs/supporter-
 * widgets.md §20), as opposed to useWidgetProfilesQuery's existing
 * per-goal filter. */
export function useAllWidgetProfilesQuery(): UseQueryResult<WidgetProfile[], Error> {
  return useQuery({ queryKey: goalsKeys.widgetProfilesAll(), queryFn: ({ signal }) => fetchWidgetProfiles(undefined, signal) });
}

export function useCreateWidgetProfileMutation(): UseMutationResult<WidgetProfile, Error, WidgetProfileInput> {
  const invalidate = useInvalidateWidgetProfiles();
  return useMutation({ mutationFn: (input: WidgetProfileInput) => createWidgetProfile(input), onSuccess: invalidate });
}

export function useUpdateWidgetProfileMutation(): UseMutationResult<
  WidgetProfile,
  Error,
  { id: string; input: WidgetProfileInput }
> {
  const invalidate = useInvalidateWidgetProfiles();
  return useMutation({ mutationFn: ({ id, input }) => updateWidgetProfile(id, input), onSuccess: invalidate });
}

export function useDeleteWidgetProfileMutation(): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateWidgetProfiles();
  return useMutation({ mutationFn: (id: string) => deleteWidgetProfile(id), onSuccess: invalidate });
}

export function useRotateWidgetProfileSlugMutation(): UseMutationResult<WidgetProfile, Error, string> {
  const invalidate = useInvalidateWidgetProfiles();
  return useMutation({ mutationFn: (id: string) => rotateWidgetProfileSlug(id), onSuccess: invalidate });
}

/** Clears a Stage 18B widget's own runtime-only presentation state
 * (docs/supporter-widgets.md §14) - a manual operator action, entirely
 * separate from persisted configuration. */
export function useResetWidgetRuntimeMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => resetWidgetRuntime(id),
    onSuccess: (_data, id) => void queryClient.invalidateQueries({ queryKey: goalsKeys.runtimeStatus(id) }),
  });
}

export function useWidgetRuntimeStatusQuery(id: string | null): UseQueryResult<RuntimeStatus, Error> {
  return useQuery({
    queryKey: goalsKeys.runtimeStatus(id ?? ''),
    queryFn: ({ signal }) => fetchWidgetRuntimeStatus(id ?? '', signal),
    enabled: id !== null,
    // The runtime status can change from a real event at any moment
    // outside any action this page takes - poll gently while a profile
    // is expanded, mirroring the public route's own 1.5s poll interval.
    refetchInterval: 1500,
  });
}
