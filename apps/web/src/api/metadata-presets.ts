import { ApiError, apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  applyPresetResponseSchema,
  applyPreviewResponseSchema,
  type ApplyDestinationPreview,
  type ApplyPresetResult,
} from './metadata-preset-apply-schemas';
import {
  metadataPresetSchema,
  metadataPresetsResponseSchema,
  type MetadataPreset,
  type SavePresetInput,
} from './metadata-preset-schemas';

export async function fetchMetadataPresets(signal?: AbortSignal): Promise<MetadataPreset[]> {
  return apiGet('/api/metadata-presets', metadataPresetsResponseSchema, { signal });
}

export async function fetchMetadataPreset(id: string, signal?: AbortSignal): Promise<MetadataPreset> {
  if (id.length === 0) {
    throw new ApiError('parse', 'A preset id is required.');
  }
  return apiGet(`/api/metadata-presets/${encodeURIComponent(id)}`, metadataPresetSchema, { signal });
}

export async function createMetadataPreset(input: SavePresetInput): Promise<MetadataPreset> {
  return apiPost('/api/metadata-presets', input, metadataPresetSchema);
}

export async function updateMetadataPreset(id: string, input: SavePresetInput): Promise<MetadataPreset> {
  return apiPut(`/api/metadata-presets/${encodeURIComponent(id)}`, input, metadataPresetSchema);
}

export async function deleteMetadataPreset(id: string): Promise<void> {
  await apiDelete(`/api/metadata-presets/${encodeURIComponent(id)}`);
}

/** Never publishes anything - a read-only compatibility check (docs/metadata-presets.md §6). */
export async function fetchApplyPreview(
  presetId: string,
  platformIds: string[],
  signal?: AbortSignal,
): Promise<ApplyDestinationPreview[]> {
  if (platformIds.length === 0) return [];
  const query = new URLSearchParams({ platformIds: platformIds.join(',') });
  return apiGet(
    `/api/metadata-presets/${encodeURIComponent(presetId)}/apply-preview?${query.toString()}`,
    applyPreviewResponseSchema,
    { signal },
  );
}

/** Writes local metadata only, all-or-nothing across every named destination. */
export async function applyMetadataPreset(
  presetId: string,
  platformIds: string[],
): Promise<ApplyPresetResult> {
  return apiPost(
    `/api/metadata-presets/${encodeURIComponent(presetId)}/apply`,
    { platformIds },
    applyPresetResponseSchema,
  );
}
