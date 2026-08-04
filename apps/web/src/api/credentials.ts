import { apiDelete, apiGet, apiPut } from '@/lib/api-client';

import { credentialStatusSchema, type CredentialStatus } from './credential-schemas';

/**
 * Thin transport layer for the destination-credential API. No caching or
 * React concerns live here - see `hooks/use-credentials.ts`.
 *
 * There is deliberately no function here that could return a stored stream
 * key's value: the backend never sends one, and no endpoint for reading one
 * exists.
 */

export async function fetchCredentialStatus(
  platformId: string,
  signal?: AbortSignal,
): Promise<CredentialStatus> {
  return apiGet(
    `/api/platforms/${encodeURIComponent(platformId)}/credentials`,
    credentialStatusSchema,
    { signal },
  );
}

export async function setStreamKey(
  platformId: string,
  streamKey: string,
): Promise<CredentialStatus> {
  return apiPut(
    `/api/platforms/${encodeURIComponent(platformId)}/credentials/stream-key`,
    { streamKey },
    credentialStatusSchema,
  );
}

export async function deleteStreamKey(platformId: string): Promise<void> {
  await apiDelete(`/api/platforms/${encodeURIComponent(platformId)}/credentials/stream-key`);
}
