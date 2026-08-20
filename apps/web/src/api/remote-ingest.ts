import { z } from 'zod';

import { apiGet, apiPost } from '@/lib/api-client';

import {
  remoteIngestSecretSchema,
  remoteIngestStatusSchema,
  type RemoteIngestSecret,
  type RemoteIngestStatus,
} from './remote-ingest-schemas';

const revokeResponseSchema = z.object({ status: z.string() });

export async function fetchRemoteIngestStatus(signal?: AbortSignal): Promise<RemoteIngestStatus> {
  return apiGet('/api/remote-ingest/status', remoteIngestStatusSchema, { signal });
}

export async function provisionRemoteIngest(): Promise<RemoteIngestSecret> {
  return apiPost('/api/remote-ingest/provision', undefined, remoteIngestSecretSchema);
}

export async function rotateRemoteIngest(): Promise<RemoteIngestSecret> {
  return apiPost('/api/remote-ingest/rotate', undefined, remoteIngestSecretSchema);
}

export async function revokeRemoteIngest(): Promise<void> {
  await apiPost('/api/remote-ingest/revoke', undefined, revokeResponseSchema);
}
