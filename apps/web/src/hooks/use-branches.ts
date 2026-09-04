import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  fetchBranches,
  fetchFFmpegStatus,
  restartBranch,
  startBranch,
  startEnabledBranches,
  stopAllBranches,
  stopBranch,
} from '@/api/branches';
import type {
  BranchCommandResponse,
  BranchSnapshot,
  FFmpegStatus,
  StartEnabledResponse,
} from '@/api/branch-schemas';

/** Query keys. FFmpeg status and the branch list are independent resources. */
export const branchKeys = {
  ffmpeg: ['ffmpeg-status'] as const,
  branches: ['branches'] as const,
};

/**
 * How often the branch list is refreshed.
 *
 * Roughly one second while any branch is active or desired-running - a
 * publisher connecting, FFmpeg producing progress, or a restart backoff
 * ticking down can all change state at any moment. Slower once every branch
 * is idle or blocked, since nothing changes there without a user action.
 */
const ACTIVE_BRANCH_STATES: readonly BranchSnapshot['state'][] = [
  'starting',
  'live',
  'restarting',
  'stopping',
  'waiting_for_ingest',
];

export function branchPollIntervalFor(branches: BranchSnapshot[] | undefined): number {
  if (branches === undefined) return 5_000;
  const active = branches.some(
    (b) => b.desiredRunning || ACTIVE_BRANCH_STATES.includes(b.state),
  );
  return active ? 1_000 : 10_000;
}

/** FFmpeg dependency status changes only when the operator acts or the
 * 5-minute backend refresh runs; polled slowly. */
export function useFfmpegRuntimeQuery(): UseQueryResult<FFmpegStatus, Error> {
  return useQuery({
    queryKey: branchKeys.ffmpeg,
    queryFn: ({ signal }) => fetchFFmpegStatus(signal),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });
}

export function useBranchRuntimeQuery(): UseQueryResult<BranchSnapshot[], Error> {
  return useQuery({
    queryKey: branchKeys.branches,
    queryFn: ({ signal }) => fetchBranches(signal),
    refetchInterval: (query) => branchPollIntervalFor(query.state.data),
    refetchIntervalInBackground: false,
    // A real, measured startup-performance defect (not merely a hunch):
    // on Dashboard, `SystemStatusRail` (StreamCountersCard/
    // QuickActionsCard) mounts immediately, before `platformsQuery`
    // resolves, while `PlatformGrid`'s own `PlatformCard`s - the other
    // caller of this same hook - only mount once it has (gated on
    // `platformsQuery.isSuccess`). With `staleTime: 0`, that second,
    // slightly-later wave of observers found the already-fetched data
    // instantly "stale" and each triggered its own extra network
    // request for state that had not actually changed in the
    // intervening tens of milliseconds - confirmed via a real-browser
    // network capture against a real production build, not development
    // duplication. `refetchInterval` above already drives real
    // liveness (as fast as every second while any branch is active);
    // this small positive `staleTime` only prevents a same-page-load
    // remount from re-triggering a fetch the interval will already
    // repeat within a second or two regardless - never a "cached value
    // is fine" argument for this genuinely live data.
    staleTime: 2_000,
  });
}

/** Shared invalidation: every command changes the branch list. */
function useBranchCommand<TData, TVariables>(
  mutationFn: (variables: TVariables) => Promise<TData>,
): UseMutationResult<TData, Error, TVariables> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: branchKeys.branches });
    },
  });
}

export function useStartBranchMutation(): UseMutationResult<BranchCommandResponse, Error, string> {
  return useBranchCommand(startBranch);
}

export function useStopBranchMutation(): UseMutationResult<BranchCommandResponse, Error, string> {
  return useBranchCommand(stopBranch);
}

export function useRestartBranchMutation(): UseMutationResult<BranchCommandResponse, Error, string> {
  return useBranchCommand(restartBranch);
}

export function useStartEnabledBranchesMutation(): UseMutationResult<
  StartEnabledResponse,
  Error,
  void
> {
  return useBranchCommand(startEnabledBranches);
}

export function useStopAllBranchesMutation(): UseMutationResult<BranchCommandResponse, Error, void> {
  return useBranchCommand(stopAllBranches);
}
