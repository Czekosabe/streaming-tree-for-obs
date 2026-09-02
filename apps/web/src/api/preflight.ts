import { apiGet } from '@/lib/api-client';

import { preflightReportSchema, type PreflightReport } from './preflight-schemas';

/** Never writes anything - a read-only readiness aggregation (docs/stream-preflight.md). */
export async function fetchPreflight(
  profileId: string | null,
  signal?: AbortSignal,
): Promise<PreflightReport> {
  const query = profileId !== null ? `?${new URLSearchParams({ profileId }).toString()}` : '';
  return apiGet(`/api/preflight${query}`, preflightReportSchema, { signal });
}
