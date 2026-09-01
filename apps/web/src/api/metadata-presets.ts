import { ApiError, apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

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
