import { apiGet } from '@/lib/api-client';
import { systemResourcesSchema, type SystemResourcesSnapshot } from '@/models/system-resources';

/** Transport for `GET /api/system/resources`. No caching/React concerns
 * live here - see hooks/use-system-resources.ts. */
export async function fetchSystemResources(signal?: AbortSignal): Promise<SystemResourcesSnapshot> {
  return apiGet('/api/system/resources', systemResourcesSchema, { signal });
}
