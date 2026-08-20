import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';

import { disableRemoteOverlay, enableRemoteOverlay, fetchRemoteOverlayStatus, rotateRemoteOverlay } from '@/api/remote-overlay';
import type { RemoteOverlayDomain, RemoteOverlayStatus, RemoteOverlayUrlResponse } from '@/api/remote-overlay-schemas';
import { ApiError } from '@/lib/api-client';

export const remoteOverlayKeys = {
  status: (domain: RemoteOverlayDomain, localSlug: string) => ['remote-overlay', domain, localSlug] as const,
};

/**
 * A 404 means no remote overlay origin is configured on this
 * deployment - the feature does not exist here, not an error the
 * panel should surface. `isRemoteOverlayUnavailable` is what
 * `RemoteOverlayPanel` checks before rendering anything.
 */
export function useRemoteOverlayStatusQuery(
  domain: RemoteOverlayDomain,
  localSlug: string,
): UseQueryResult<RemoteOverlayStatus, Error> {
  return useQuery({
    queryKey: remoteOverlayKeys.status(domain, localSlug),
    queryFn: ({ signal }) => fetchRemoteOverlayStatus(domain, localSlug, signal),
    enabled: localSlug !== '',
    retry: (failureCount, error) => {
      if (error instanceof ApiError && error.kind === 'not-found') return false;
      return failureCount < 2;
    },
  });
}

export function isRemoteOverlayUnavailable(error: Error | null): boolean {
  return error instanceof ApiError && error.kind === 'not-found';
}

export function useEnableRemoteOverlayMutation(
  domain: RemoteOverlayDomain,
  localSlug: string,
): UseMutationResult<RemoteOverlayUrlResponse, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => enableRemoteOverlay(domain, localSlug),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: remoteOverlayKeys.status(domain, localSlug) });
    },
  });
}

export function useRotateRemoteOverlayMutation(
  domain: RemoteOverlayDomain,
  localSlug: string,
): UseMutationResult<RemoteOverlayUrlResponse, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => rotateRemoteOverlay(domain, localSlug),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: remoteOverlayKeys.status(domain, localSlug) });
    },
  });
}

export function useDisableRemoteOverlayMutation(
  domain: RemoteOverlayDomain,
  localSlug: string,
): UseMutationResult<void, Error, void> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => disableRemoteOverlay(domain, localSlug),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: remoteOverlayKeys.status(domain, localSlug) });
    },
  });
}
