import { apiGet } from '@/lib/api-client';
import { aboutResponseSchema, type AboutResponse } from '@/models/about';

/** Fetches the fixed product-identity/build metadata from GET /api/about. */
export async function fetchAbout(signal?: AbortSignal): Promise<AboutResponse> {
  return apiGet('/api/about', aboutResponseSchema, { signal });
}
