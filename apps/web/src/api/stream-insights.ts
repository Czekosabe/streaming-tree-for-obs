import { apiGet } from '@/lib/api-client';

import { streamInsightsSchema, type StreamInsights } from './stream-insights-schemas';

/** Never writes anything - a read-only aggregation over Stage 24's own operational history (docs/stream-insights.md). */
export async function fetchStreamInsights(signal?: AbortSignal): Promise<StreamInsights> {
  return apiGet('/api/stream-insights', streamInsightsSchema, { signal });
}
