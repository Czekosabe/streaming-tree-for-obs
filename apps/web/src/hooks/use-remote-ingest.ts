import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import {
  fetchRemoteIngestStatus,
  provisionRemoteIngest,
  revokeRemoteIngest,
  rotateRemoteIngest,
} from '@/api/remote-ingest';
import type { RemoteIngestSecret, RemoteIngestStatus } from '@/api/remote-ingest-schemas';
import { ApiError } from '@/lib/api-client';

export const remoteIngestKeys = {
  status: ['remote-ingest', 'status'] as const,
};

/**
 * A 404 means `--remote-ingest` is not active on this deployment - the
 * feature does not exist here, not an error condition the panel should
 * surface. `useRemoteIngestAvailable` below is what components check
 * before rendering anything.
 */
export function useRemoteIngestStatusQuery(): UseQueryResult<RemoteIngestStatus, Error> {
  return useQuery({
    queryKey: remoteIngestKeys.status,
    queryFn: ({ signal }) => fetchRemoteIngestStatus(signal),
    retry: (failureCount, error) => {
      if (error instanceof ApiError && error.kind === 'not-found') return false;
      return failureCount < 2;
    },
  });
}

/** True once the status query has confirmed the route exists on this deployment. */
export function isRemoteIngestUnavailable(error: Error | null): boolean {
  return error instanceof ApiError && error.kind === 'not-found';
}

export function useProvisionRemoteIngestMutation(): UseMutationResult<RemoteIngestSecret, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => provisionRemoteIngest(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: remoteIngestKeys.status });
    },
  });
}

export function useRotateRemoteIngestMutation(): UseMutationResult<RemoteIngestSecret, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => rotateRemoteIngest(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: remoteIngestKeys.status });
    },
  });
}

export function useRevokeRemoteIngestMutation(): UseMutationResult<void, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => revokeRemoteIngest(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: remoteIngestKeys.status });
    },
  });
}
