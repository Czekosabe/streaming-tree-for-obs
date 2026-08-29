import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import { fetchSystemResources } from '@/api/system-resources';
import type { SystemResourcesSnapshot } from '@/models/system-resources';

export const SYSTEM_RESOURCES_QUERY_KEY = ['system-resources'] as const;

/** Matches the backend collector's own sampling cadence
 * (`sysresources.NewCollector`'s 5-second tick) - polling faster would only
 * ever re-fetch the same cached snapshot. */
const SYSTEM_RESOURCES_POLL_INTERVAL_MS = 5_000;

/**
 * Polls `GET /api/system/resources` - real, local host CPU/memory/disk
 * usage, sampled by the backend in the background and cached there; this
 * hook never samples anything itself. A failure here is treated the same
 * as every other optional Dashboard query: `isError` renders an honest
 * unavailable state, never a crash.
 */
export function useSystemResourcesQuery(): UseQueryResult<SystemResourcesSnapshot, Error> {
  return useQuery({
    queryKey: SYSTEM_RESOURCES_QUERY_KEY,
    queryFn: ({ signal }) => fetchSystemResources(signal),
    refetchInterval: SYSTEM_RESOURCES_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  });
}
