import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import { fetchStreamInsights } from '@/api/stream-insights';
import type { StreamInsights } from '@/api/stream-insights-schemas';

export const streamInsightsKeys = {
  insights: ['stream-insights'] as const,
};

export function useStreamInsightsQuery(): UseQueryResult<StreamInsights, Error> {
  return useQuery({
    queryKey: streamInsightsKeys.insights,
    queryFn: ({ signal }) => fetchStreamInsights(signal),
  });
}
