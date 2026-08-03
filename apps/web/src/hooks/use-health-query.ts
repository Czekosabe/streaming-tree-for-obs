import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import { apiGet } from '@/lib/api-client';
import { healthResponseSchema, type HealthResponse } from '@/models/health';

export const HEALTH_QUERY_KEY = ['health'] as const;

/** Interval between automatic backend health probes. */
const HEALTH_POLL_INTERVAL_MS = 15_000;

/**
 * Polls `GET /api/health`.
 *
 * A failure here is an expected, non-fatal state: the user may simply not have
 * started the Go backend. Consumers render a "Backend unavailable" panel from
 * `isError` instead of letting the error bubble up.
 */
export function useHealthQuery(): UseQueryResult<HealthResponse, Error> {
  return useQuery({
    queryKey: HEALTH_QUERY_KEY,
    queryFn: ({ signal }) => apiGet('/api/health', healthResponseSchema, { signal }),
    refetchInterval: HEALTH_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}
