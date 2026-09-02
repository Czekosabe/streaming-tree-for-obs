import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  streamSetupApplyResultSchema,
  streamSetupPreviewSchema,
  streamSetupProfileSchema,
  streamSetupProfilesResponseSchema,
  type SaveStreamSetupInput,
  type StreamSetupApplyResult,
  type StreamSetupPreview,
  type StreamSetupProfile,
} from './stream-setup-schemas';

const NO_BODY = undefined;

export async function fetchStreamSetups(signal?: AbortSignal): Promise<StreamSetupProfile[]> {
  return apiGet('/api/stream-setups', streamSetupProfilesResponseSchema, { signal });
}

export async function createStreamSetup(input: SaveStreamSetupInput): Promise<StreamSetupProfile> {
  return apiPost('/api/stream-setups', input, streamSetupProfileSchema);
}

export async function updateStreamSetup(
  id: string,
  input: SaveStreamSetupInput,
): Promise<StreamSetupProfile> {
  return apiPut(`/api/stream-setups/${encodeURIComponent(id)}`, input, streamSetupProfileSchema);
}

export async function deleteStreamSetup(id: string): Promise<void> {
  await apiDelete(`/api/stream-setups/${encodeURIComponent(id)}`);
}

export async function duplicateStreamSetup(id: string, name: string): Promise<StreamSetupProfile> {
  return apiPost(
    `/api/stream-setups/${encodeURIComponent(id)}/duplicate`,
    { name },
    streamSetupProfileSchema,
  );
}

export async function saveCurrentStreamSetup(
  name: string,
  note: string,
  metadataPresetId: string | null,
): Promise<StreamSetupProfile> {
  return apiPost(
    '/api/stream-setups/save-current',
    { name, note, metadataPresetId },
    streamSetupProfileSchema,
  );
}

/** Never writes anything - a read-only compatibility/change preview (docs/stream-setup-profiles.md §3). */
export async function fetchStreamSetupPreview(
  id: string,
  signal?: AbortSignal,
): Promise<StreamSetupPreview> {
  return apiGet(`/api/stream-setups/${encodeURIComponent(id)}/preview`, streamSetupPreviewSchema, {
    signal,
  });
}

/** Local-only: applies destination membership and, if referenced, the linked metadata preset. Never starts a stream or publishes anything. */
export async function applyStreamSetup(id: string): Promise<StreamSetupApplyResult> {
  return apiPost(
    `/api/stream-setups/${encodeURIComponent(id)}/apply`,
    NO_BODY,
    streamSetupApplyResultSchema,
  );
}
