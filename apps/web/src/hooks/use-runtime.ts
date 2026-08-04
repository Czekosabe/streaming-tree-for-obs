import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  installMediaMtx,
  restartMediaMtx,
  startMediaMtx,
  stopMediaMtx,
} from '@/api/runtime';
import { fetchRuntime } from '@/api/runtime';
import type { MediaMtxState, RuntimeSnapshot } from '@/api/runtime-schemas';

/** Query key for the runtime snapshot. */
export const runtimeKeys = {
  runtime: ['runtime'] as const,
};

/**
 * How often the runtime snapshot is refreshed, chosen from the current state.
 *
 * Fast while something is changing or a publisher may connect at any moment,
 * slow when nothing can change without a user action. Polling a stopped or
 * missing MediaMTX every second would be pure waste.
 */
export function pollIntervalFor(state: MediaMtxState | undefined): number {
  switch (state) {
    case 'ready':
    case 'starting':
    case 'stopping':
      // A publisher can appear or disappear at any moment.
      return 1_000;
    case 'installing':
      // A download is running; the user is watching for it to finish.
      return 2_000;
    case 'missing':
    case 'incompatible':
    case 'stopped':
    case 'error':
      // Nothing changes without a user action, so a slow heartbeat is enough
      // to notice an out-of-band change.
      return 10_000;
    default:
      return 5_000;
  }
}

/**
 * Polls the runtime snapshot.
 *
 * Polling pauses while the document is hidden: TanStack Query stops background
 * refetching for a hidden tab by default, so a minimised window does not keep
 * the loop running.
 */
export function useRuntimeQuery(): UseQueryResult<RuntimeSnapshot, Error> {
  return useQuery({
    queryKey: runtimeKeys.runtime,
    queryFn: ({ signal }) => fetchRuntime(signal),
    refetchInterval: (query) => pollIntervalFor(query.state.data?.mediaMtx.state),
    refetchIntervalInBackground: false,
    // Runtime state is always "now"; a cached value is never worth reusing.
    staleTime: 0,
  });
}

/** Shared invalidation: every command changes the snapshot. */
function useRuntimeCommand(
  mutationFn: () => Promise<void>,
): UseMutationResult<void, Error, void> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn,
    onSettled: () => {
      // Refetched on success and on failure alike: a rejected command still
      // tells us the state we assumed was wrong.
      void queryClient.invalidateQueries({ queryKey: runtimeKeys.runtime });
    },
  });
}

export function useInstallMediaMtxMutation(): UseMutationResult<void, Error, void> {
  return useRuntimeCommand(installMediaMtx);
}

export function useStartMediaMtxMutation(): UseMutationResult<void, Error, void> {
  return useRuntimeCommand(startMediaMtx);
}

export function useStopMediaMtxMutation(): UseMutationResult<void, Error, void> {
  return useRuntimeCommand(stopMediaMtx);
}

export function useRestartMediaMtxMutation(): UseMutationResult<void, Error, void> {
  return useRuntimeCommand(restartMediaMtx);
}
