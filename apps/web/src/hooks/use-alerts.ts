import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  clearAlertQueue,
  createAlertProfile,
  createAlertRule,
  deleteAlertProfile,
  deleteAlertRule,
  fetchAlertEventTypes,
  fetchAlertProfile,
  fetchAlertProfiles,
  fetchAlertQueueStatus,
  fetchAlertRule,
  fetchAlertRules,
  fetchPublicAlertProfileConfig,
  pauseAlertQueue,
  previewAlertTemplate,
  replayPreviousAlert,
  resumeAlertQueue,
  rotateAlertProfileSlug,
  skipCurrentAlert,
  testAlertRule,
  updateAlertProfile,
  updateAlertRule,
} from '@/api/alerts';
import type {
  AlertEventTypeCapability,
  AlertProfile,
  AlertProfileInput,
  AlertQueueStatus,
  AlertRule,
  AlertRuleInput,
  AlertRulePreviewInput,
  AlertRulePreviewResponse,
  AlertSummary,
  ListAlertRulesResponse,
  PublicAlertProfileConfig,
} from '@/api/alerts-schemas';

export const alertsKeys = {
  eventTypes: () => ['alert-event-types'] as const,
  profiles: () => ['alert-profiles'] as const,
  profile: (id: string) => ['alert-profiles', id] as const,
  rules: (profileId: string) => ['alert-rules', profileId] as const,
  rule: (id: string) => ['alert-rule', id] as const,
  queue: (profileId: string) => ['alert-queue', profileId] as const,
};

/** How often the queue status view polls while mounted - matches this
 * project's existing chat-automation/engagement precedent. */
const POLL_INTERVAL_MS = 5_000;

export function useAlertEventTypesQuery(): UseQueryResult<AlertEventTypeCapability[], Error> {
  return useQuery({
    queryKey: alertsKeys.eventTypes(),
    queryFn: ({ signal }) => fetchAlertEventTypes(signal),
    staleTime: Infinity,
  });
}

export function useAlertProfilesQuery(): UseQueryResult<AlertProfile[], Error> {
  return useQuery({
    queryKey: alertsKeys.profiles(),
    queryFn: ({ signal }) => fetchAlertProfiles(signal),
  });
}

export function useAlertProfileQuery(id: string | null): UseQueryResult<AlertProfile, Error> {
  return useQuery({
    queryKey: alertsKeys.profile(id ?? ''),
    queryFn: ({ signal }) => fetchAlertProfile(id ?? '', signal),
    enabled: id !== null,
  });
}

function useInvalidateProfiles() {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: alertsKeys.profiles() });
}

export function useCreateAlertProfileMutation(): UseMutationResult<AlertProfile, Error, string> {
  const invalidate = useInvalidateProfiles();
  return useMutation({
    mutationFn: (name: string) => createAlertProfile(name),
    onSuccess: invalidate,
  });
}

export function useUpdateAlertProfileMutation(): UseMutationResult<
  AlertProfile,
  Error,
  { id: string; input: AlertProfileInput }
> {
  const invalidate = useInvalidateProfiles();
  return useMutation({
    mutationFn: ({ id, input }) => updateAlertProfile(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteAlertProfileMutation(): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateProfiles();
  return useMutation({
    mutationFn: (id: string) => deleteAlertProfile(id),
    onSuccess: invalidate,
  });
}

export function useRotateAlertProfileSlugMutation(): UseMutationResult<AlertProfile, Error, string> {
  const invalidate = useInvalidateProfiles();
  return useMutation({
    mutationFn: (id: string) => rotateAlertProfileSlug(id),
    onSuccess: invalidate,
  });
}

export function useAlertRulesQuery(profileId: string | null): UseQueryResult<ListAlertRulesResponse, Error> {
  return useQuery({
    queryKey: alertsKeys.rules(profileId ?? ''),
    queryFn: ({ signal }) => fetchAlertRules(profileId ?? '', signal),
    enabled: profileId !== null,
  });
}

export function useAlertRuleQuery(id: string | null): UseQueryResult<AlertRule, Error> {
  return useQuery({
    queryKey: alertsKeys.rule(id ?? ''),
    queryFn: ({ signal }) => fetchAlertRule(id ?? '', signal),
    enabled: id !== null,
  });
}

function useInvalidateRules(profileId: string) {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: alertsKeys.rules(profileId) });
}

export function useCreateAlertRuleMutation(
  profileId: string,
): UseMutationResult<AlertRule, Error, AlertRuleInput> {
  const invalidate = useInvalidateRules(profileId);
  return useMutation({
    mutationFn: (input: AlertRuleInput) => createAlertRule(profileId, input),
    onSuccess: invalidate,
  });
}

export function useUpdateAlertRuleMutation(
  profileId: string,
): UseMutationResult<AlertRule, Error, { id: string; input: AlertRuleInput }> {
  const invalidate = useInvalidateRules(profileId);
  return useMutation({
    mutationFn: ({ id, input }) => updateAlertRule(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteAlertRuleMutation(profileId: string): UseMutationResult<void, Error, string> {
  const invalidate = useInvalidateRules(profileId);
  return useMutation({
    mutationFn: (id: string) => deleteAlertRule(id),
    onSuccess: invalidate,
  });
}

/** Test Rule - creates one real, synthetic alert through the real
 * queue/playback path. Never a preview: it actually plays on the
 * public Browser Source. */
export function useTestAlertRuleMutation(): UseMutationResult<
  AlertSummary,
  Error,
  { id: string; scenario?: string }
> {
  return useMutation({
    mutationFn: ({ id, scenario }) => testAlertRule(id, scenario),
  });
}

/** Local editor preview - never invalidates any cache, since it
 * neither sends nor persists nor queues anything. */
export function useAlertPreviewMutation(): UseMutationResult<
  AlertRulePreviewResponse,
  Error,
  AlertRulePreviewInput
> {
  return useMutation({
    mutationFn: (input: AlertRulePreviewInput) => previewAlertTemplate(input),
  });
}

export function useAlertQueueStatusQuery(profileId: string | null): UseQueryResult<AlertQueueStatus, Error> {
  return useQuery({
    queryKey: alertsKeys.queue(profileId ?? ''),
    queryFn: ({ signal }) => fetchAlertQueueStatus(profileId ?? '', signal),
    enabled: profileId !== null,
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}

function useInvalidateQueue(profileId: string) {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: alertsKeys.queue(profileId) });
}

export function usePauseAlertQueueMutation(profileId: string): UseMutationResult<AlertQueueStatus, Error, void> {
  const invalidate = useInvalidateQueue(profileId);
  return useMutation({ mutationFn: () => pauseAlertQueue(profileId), onSuccess: invalidate });
}

export function useResumeAlertQueueMutation(profileId: string): UseMutationResult<AlertQueueStatus, Error, void> {
  const invalidate = useInvalidateQueue(profileId);
  return useMutation({ mutationFn: () => resumeAlertQueue(profileId), onSuccess: invalidate });
}

export function useSkipCurrentAlertMutation(profileId: string): UseMutationResult<AlertQueueStatus, Error, void> {
  const invalidate = useInvalidateQueue(profileId);
  return useMutation({ mutationFn: () => skipCurrentAlert(profileId), onSuccess: invalidate });
}

export function useReplayPreviousAlertMutation(
  profileId: string,
): UseMutationResult<AlertQueueStatus, Error, void> {
  const invalidate = useInvalidateQueue(profileId);
  return useMutation({ mutationFn: () => replayPreviousAlert(profileId), onSuccess: invalidate });
}

export function useClearAlertQueueMutation(profileId: string): UseMutationResult<AlertQueueStatus, Error, void> {
  const invalidate = useInvalidateQueue(profileId);
  return useMutation({ mutationFn: () => clearAlertQueue(profileId), onSuccess: invalidate });
}

/** Public route only: the profile's own fixed presentation config,
 * fetched once (never polled - the SSE stream carries every live
 * change that matters). */
export function usePublicAlertProfileConfigQuery(
  publicSlug: string | undefined,
): UseQueryResult<PublicAlertProfileConfig, Error> {
  return useQuery({
    queryKey: ['public-alert-profile-config', publicSlug ?? ''],
    queryFn: ({ signal }) => fetchPublicAlertProfileConfig(publicSlug!, signal),
    enabled: publicSlug !== undefined && publicSlug !== '',
    retry: false,
  });
}
