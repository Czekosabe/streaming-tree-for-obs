import { ApiError, apiDelete, apiGet, kindForStatus, readErrorEnvelope, resolveUrl } from '@/lib/api-client';

import { audioAssetListSchema, audioAssetSchema, type AudioAsset } from './audioasset-schemas';

/**
 * Transport for the Stage 17B managed audio asset API
 * (`internal/httpapi/audioasset.go`) - a management/editor surface
 * only. Upload uses a real `multipart/form-data` request (never JSON),
 * mirroring `api/visualasset.ts`'s own identical upload shape.
 */

const UPLOAD_TIMEOUT_MS = 30_000;

export async function fetchAudioAssets(signal?: AbortSignal): Promise<AudioAsset[]> {
  return apiGet('/api/audio-assets', audioAssetListSchema, { signal });
}

export async function fetchAudioAsset(id: string, signal?: AbortSignal): Promise<AudioAsset> {
  return apiGet(`/api/audio-assets/${id}`, audioAssetSchema, { signal });
}

/** Rejected with `ApiError.code === 'audio_asset_in_use'` (409) if any
 * saved alert rule or template still references the asset - never a
 * silent no-op. */
export async function deleteAudioAsset(id: string): Promise<void> {
  return apiDelete(`/api/audio-assets/${id}`);
}

/** Uploads file as a new managed audio asset - a normal file-picker
 * flow, never a URL import. The backend independently validates the
 * file's real signature (16-bit PCM WAV only); a mismatched/
 * unsupported type is rejected with `ApiError.code ===
 * 'audio_asset_unsupported'`. */
export async function uploadAudioAsset(file: File, displayName?: string): Promise<AudioAsset> {
  const form = new FormData();
  form.append('file', file, file.name);
  if (displayName !== undefined) form.append('displayName', displayName);

  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), UPLOAD_TIMEOUT_MS);

  const path = '/api/audio-assets';
  let response: Response;
  try {
    response = await fetch(resolveUrl(path), {
      method: 'POST',
      body: form,
      signal: controller.signal,
      headers: { Accept: 'application/json' },
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError('timeout', `Uploading to ${path} timed out.`);
    }
    throw new ApiError('network', `Cannot reach the backend at ${resolveUrl(path)}.`);
  } finally {
    window.clearTimeout(timeoutId);
  }

  if (!response.ok) {
    const envelope = await readErrorEnvelope(response);
    throw new ApiError(kindForStatus(response.status, envelope.code), `Upload to ${path} failed with ${response.status}.`, {
      status: response.status,
      ...envelope,
    });
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new ApiError('parse', `Backend response for ${path} was not valid JSON.`);
  }
  const parsed = audioAssetSchema.safeParse(payload);
  if (!parsed.success) {
    throw new ApiError('parse', `Backend response for ${path} did not match the expected shape.`);
  }
  return parsed.data;
}
