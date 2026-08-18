import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import {
  checkForUpdate,
  downloadUpdate,
  fetchUpdateStatus,
  installUpdate,
  setAutoCheckPreference,
} from '@/api/updates';
import type { UpdateState, UpdateStatus } from '@/models/updates';

export const updateKeys = {
  status: ['updates', 'status'] as const,
};

/**
 * How often the update status is refreshed, chosen from the current state -
 * mirrors `pollIntervalFor` in `hooks/use-runtime.ts` exactly. Fast while
 * something is actively changing (a download in progress, a check just
 * fired), a slow heartbeat otherwise - checking for updates itself is
 * already a background hourly concern owned by the backend
 * (docs/updater.md §10), not something this poll needs to drive.
 */
export function updatePollIntervalFor(state: UpdateState | undefined): number {
  switch (state) {
    case 'checking':
    case 'downloading':
    case 'installing':
      return 1_000;
    default:
      return 30_000;
  }
}

/** Polls `GET /api/updates/status`. */
export function useUpdateStatusQuery(): UseQueryResult<UpdateStatus, Error> {
  return useQuery({
    queryKey: updateKeys.status,
    queryFn: ({ signal }) => fetchUpdateStatus(signal),
    refetchInterval: (query) => updatePollIntervalFor(query.state.data?.state),
    refetchIntervalInBackground: false,
    staleTime: 0,
  });
}

function useUpdateCommand<TArgs = void>(
  mutationFn: (args: TArgs) => Promise<unknown>,
): UseMutationResult<unknown, Error, TArgs> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: updateKeys.status });
    },
  });
}

export function useSetAutoCheckMutation(): UseMutationResult<unknown, Error, boolean> {
  return useUpdateCommand((autoCheck: boolean) => setAutoCheckPreference(autoCheck));
}

export function useCheckForUpdateMutation(): UseMutationResult<unknown, Error, void> {
  return useUpdateCommand(() => checkForUpdate());
}

export function useDownloadUpdateMutation(): UseMutationResult<unknown, Error, void> {
  return useUpdateCommand(() => downloadUpdate());
}

export function useInstallUpdateMutation(): UseMutationResult<unknown, Error, void> {
  return useUpdateCommand(() => installUpdate());
}
