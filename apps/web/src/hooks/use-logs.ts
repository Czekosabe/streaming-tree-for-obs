import {
  useInfiniteQuery,
  useMutation,
  type InfiniteData,
  type UseInfiniteQueryResult,
  type UseMutationResult,
} from '@tanstack/react-query';

import { fetchLogs, fetchSupportBundle } from '@/api/logs';
import type { LogsResponse } from '@/api/logs-schemas';
import { downloadBlob } from '@/models/visualtemplate';

/** One page's worth of log entries, bounded by the backend's own limit. */
export const LOGS_PAGE_LIMIT = 100;

export type LogsFilter = {
  severity?: string;
  subsystem?: string;
  search?: string;
};

/** Query key for a filtered logs stream. Filters are part of the key
 * so switching severity/subsystem/search starts a fresh page sequence
 * instead of showing a stale page under a different filter's cache
 * entry. */
export const logsKeys = {
  logs: (filters: LogsFilter) =>
    ['logs', filters.severity ?? '', filters.subsystem ?? '', filters.search ?? ''] as const,
};

/**
 * Fetches recent, already-redacted log entries, newest first, paged
 * backward with the "load older" cursor the backend returns.
 *
 * Deliberately not auto-polling (governing task's own "no fake live
 * terminal" requirement): an operator troubleshooting a specific past
 * event wants a stable, filterable snapshot they can read, not text
 * scrolling out from under them while they read it. Refresh is an
 * explicit action - see the page's own refresh button.
 */
export function useLogsQuery(
  filters: LogsFilter,
): UseInfiniteQueryResult<InfiniteData<LogsResponse>, Error> {
  return useInfiniteQuery({
    queryKey: logsKeys.logs(filters),
    queryFn: ({ pageParam, signal }) =>
      fetchLogs({ ...filters, before: pageParam, limit: LOGS_PAGE_LIMIT, signal }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
}

/** Generates the support bundle and triggers a real browser download
 * of it - an explicit operator action only, never automatic. */
export function useSupportBundleMutation(): UseMutationResult<void, Error, void> {
  return useMutation({
    mutationFn: async () => {
      const { blob, filename } = await fetchSupportBundle();
      downloadBlob(blob, filename);
    },
  });
}
