import { apiDelete, apiGet, apiPost, apiPut } from '@/lib/api-client';

import {
  configuredPlatformSchema,
  configuredPlatformsResponseSchema,
  platformMetadataSchema,
  providerDefinitionsResponseSchema,
  type ConfiguredPlatform,
  type CreatePlatformInput,
  type PlatformMetadata,
  type ProviderDefinition,
  type SaveMetadataInput,
  type UpdatePlatformInput,
} from './platform-schemas';

/** Thin transport layer. No caching or React concerns live here. */

export async function fetchProviderDefinitions(
  signal?: AbortSignal,
): Promise<ProviderDefinition[]> {
  const response = await apiGet('/api/platform-definitions', providerDefinitionsResponseSchema, {
    signal,
  });
  return response.definitions;
}

export async function fetchPlatforms(signal?: AbortSignal): Promise<ConfiguredPlatform[]> {
  const response = await apiGet('/api/platforms', configuredPlatformsResponseSchema, { signal });
  return response.platforms;
}

export async function fetchPlatform(
  id: string,
  signal?: AbortSignal,
): Promise<ConfiguredPlatform> {
  return apiGet(`/api/platforms/${encodeURIComponent(id)}`, configuredPlatformSchema, { signal });
}

export async function createPlatform(input: CreatePlatformInput): Promise<ConfiguredPlatform> {
  return apiPost('/api/platforms', input, configuredPlatformSchema);
}

export async function updatePlatform(
  id: string,
  input: UpdatePlatformInput,
): Promise<ConfiguredPlatform> {
  return apiPut(`/api/platforms/${encodeURIComponent(id)}`, input, configuredPlatformSchema);
}

export async function deletePlatform(id: string): Promise<void> {
  await apiDelete(`/api/platforms/${encodeURIComponent(id)}`);
}

export async function savePlatformMetadata(
  id: string,
  input: SaveMetadataInput,
): Promise<PlatformMetadata> {
  return apiPut(
    `/api/platforms/${encodeURIComponent(id)}/metadata`,
    input,
    platformMetadataSchema,
  );
}
