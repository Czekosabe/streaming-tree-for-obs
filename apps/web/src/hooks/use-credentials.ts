import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import { deleteStreamKey, fetchCredentialStatus, setStreamKey } from '@/api/credentials';
import type { CredentialStatus } from '@/api/credential-schemas';

import { markStreamKeyDeleted } from './credential-cache';

/** Query keys, scoped per platform so one platform's status can never be
 * invalidated or overwritten by another's. */
export const credentialKeys = {
  status: (platformId: string) => ['platform-credentials', platformId] as const,
};

/**
 * A destination's stream-key status: configured or not, and whether the
 * credential store could be reached.
 *
 * Cached briefly (`staleTime`) so the settings dialog and the platform card's
 * indicator can both read this without doubling the request, but never
 * polled: a status check must not repeatedly touch the OS credential store
 * just because a dialog happens to be open.
 */
export function useCredentialStatusQuery(platformId: string): UseQueryResult<CredentialStatus, Error> {
  return useQuery({
    queryKey: credentialKeys.status(platformId),
    queryFn: ({ signal }) => fetchCredentialStatus(platformId, signal),
    staleTime: 30_000,
  });
}

export function useSetStreamKeyMutation(): UseMutationResult<
  CredentialStatus,
  Error,
  { platformId: string; streamKey: string }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ platformId, streamKey }: { platformId: string; streamKey: string }) =>
      setStreamKey(platformId, streamKey),
    // TanStack Query otherwise keeps a settled mutation - including its
    // variables, which here is the stream key the operator just typed - in
    // the mutation cache for the default gcTime (five minutes), visible via
    // React Query Devtools. A secret must not linger there, so this mutation
    // is garbage-collected as soon as it settles instead.
    gcTime: 0,
    onSuccess: (status, variables) => {
      // Only the status shape the backend returned is cached - never
      // `variables.streamKey`.
      queryClient.setQueryData(credentialKeys.status(variables.platformId), status);
    },
  });
}

export function useDeleteStreamKeyMutation(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (platformId: string) => deleteStreamKey(platformId),
    onSuccess: (_result, platformId) => {
      queryClient.setQueryData<CredentialStatus>(credentialKeys.status(platformId), (current) =>
        markStreamKeyDeleted(current),
      );
    },
  });
}
