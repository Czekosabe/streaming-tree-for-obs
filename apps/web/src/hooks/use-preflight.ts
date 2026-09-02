import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import { fetchPreflight } from '@/api/preflight';
import type { PreflightReport } from '@/api/preflight-schemas';

export const preflightKeys = {
  report: (profileId: string | null) => ['preflight', profileId ?? 'current'] as const,
};

/**
 * Read-only readiness check (docs/stream-preflight.md) - polled only
 * while `enabled` (the Preflight view is actually open), under the
 * same adaptive-polling convention every other runtime/branch query
 * already uses: no SSE, no bespoke interval, and no polling in the
 * background tab.
 */
export function usePreflightQuery(
  profileId: string | null,
  enabled: boolean,
): UseQueryResult<PreflightReport, Error> {
  return useQuery({
    queryKey: preflightKeys.report(profileId),
    queryFn: ({ signal }) => fetchPreflight(profileId, signal),
    enabled,
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
    staleTime: 0,
  });
}
