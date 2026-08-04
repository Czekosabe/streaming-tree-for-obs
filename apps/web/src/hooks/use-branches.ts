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
    // Runtime state is always "now"; a cached value is never worth reusing.
    staleTime: 0,
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
