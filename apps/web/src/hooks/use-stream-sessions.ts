import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import {
  clearStreamSessionHistory,
  fetchStreamSession,
  fetchStreamSessions,
  fetchStreamSessionSettings,
  setStreamSessionRetentionDays,
} from '@/api/stream-sessions';
import type { StreamSession, StreamSessionSettings } from '@/api/stream-sessions-schemas';

export const streamSessionKeys = {
  list: (limit?: number) => ['stream-sessions', limit ?? null] as const,
  detail: (id: string) => ['stream-sessions', 'detail', id] as const,
  settings: () => ['stream-sessions', 'settings'] as const,
};

/** Refetches periodically so a currently-open session's live duration
 * and the arrival of newly-finished sessions stay current without a
 * manual refresh - the same "operator reads a live-ish snapshot"
 * expectation the Logs page already sets, just on a slower cadence
 * appropriate to how often a session actually starts/ends. */
const STREAM_SESSIONS_REFETCH_INTERVAL_MS = 15_000;

export function useStreamSessionsQuery(limit?: number): UseQueryResult<StreamSession[], Error> {
  return useQuery({
    queryKey: streamSessionKeys.list(limit),
    queryFn: () => fetchStreamSessions(limit),
    refetchInterval: STREAM_SESSIONS_REFETCH_INTERVAL_MS,
  });
}

export function useStreamSessionQuery(id: string): UseQueryResult<StreamSession, Error> {
  return useQuery({
    queryKey: streamSessionKeys.detail(id),
    queryFn: () => fetchStreamSession(id),
  });
}

export function useClearStreamSessionHistoryMutation(): UseMutationResult<void, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: clearStreamSessionHistory,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stream-sessions'] });
    },
  });
}

export function useStreamSessionSettingsQuery(): UseQueryResult<StreamSessionSettings, Error> {
  return useQuery({
    queryKey: streamSessionKeys.settings(),
    queryFn: fetchStreamSessionSettings,
  });
}

export function useSetStreamSessionRetentionDaysMutation(): UseMutationResult<StreamSessionSettings, Error, number> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (days) => setStreamSessionRetentionDays(days),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: streamSessionKeys.settings() });
    },
  });
}
