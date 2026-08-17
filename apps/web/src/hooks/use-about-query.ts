import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import { fetchAbout } from '@/api/about';
import type { AboutResponse } from '@/models/about';

export const ABOUT_QUERY_KEY = ['about'] as const;

/**
 * Fetches `GET /api/about` once.
 *
 * Unlike the runtime/health queries this never polls: product identity and
 * build metadata cannot change while the backend process is running, so a
 * background refetch would only ever return the exact same value.
 */
export function useAboutQuery(): UseQueryResult<AboutResponse, Error> {
  return useQuery({
    queryKey: ABOUT_QUERY_KEY,
    queryFn: ({ signal }) => fetchAbout(signal),
  });
}
