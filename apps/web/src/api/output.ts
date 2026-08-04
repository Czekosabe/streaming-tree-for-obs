import { apiGet, apiPut } from '@/lib/api-client';

import {
  outputSettingsSchema,
  type OutputSettings,
  type UpdateOutputSettingsInput,
} from './output-schemas';

/** Thin transport layer. No caching or React concerns live here. */

export async function fetchOutputSettings(
  platformId: string,
  signal?: AbortSignal,
): Promise<OutputSettings> {
  return apiGet(
    `/api/platforms/${encodeURIComponent(platformId)}/output`,
    outputSettingsSchema,
    { signal },
  );
}

export async function updateOutputSettings(
  platformId: string,
  input: UpdateOutputSettingsInput,
): Promise<OutputSettings> {
  return apiPut(
    `/api/platforms/${encodeURIComponent(platformId)}/output`,
    input,
    outputSettingsSchema,
  );
}
