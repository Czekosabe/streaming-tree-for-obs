import { ApiError, apiDelete, apiGet, apiPut, kindForStatus, readErrorEnvelope, resolveUrl } from '@/lib/api-client';

import { visualAssetListSchema, visualAssetSchema, type VisualAsset } from './visualasset-schemas';

/**
 * Transport for the Stage 14B managed visual asset API
 * (`internal/httpapi/visualasset.go`) - a management/editor surface
 * only. Upload uses a real `multipart/form-data` request (never JSON),
 * so it does not go through `lib/api-client.ts`'s own JSON-only `send`
 * helper - it reuses that module's error-envelope/status-classification
 * logic directly instead of duplicating it.
 */

const UPLOAD_TIMEOUT_MS = 30_000;

export async function fetchVisualAssets(signal?: AbortSignal): Promise<VisualAsset[]> {
  return apiGet('/api/visual-assets', visualAssetListSchema, { signal });
}

export async function fetchVisualAsset(id: string, signal?: AbortSignal): Promise<VisualAsset> {
  return apiGet(`/api/visual-assets/${id}`, visualAssetSchema, { signal });
}

export async function updateVisualAssetMetadata(
  id: string,
  input: { displayName: string; author: string; license: string; notice: string },
): Promise<VisualAsset> {
  return apiPut(`/api/visual-assets/${id}`, input, visualAssetSchema);
}

/** Rejected with `ApiError.code === 'visual_asset_in_use'` (409) if any
 * saved design/template still references the asset (docs/visual-
 * template-packages.md §15) - never a silent no-op. */
export async function deleteVisualAsset(id: string): Promise<void> {
  return apiDelete(`/api/visual-assets/${id}`);
}

/** Uploads file as a new managed asset - a normal file-picker flow
 * (docs/visual-template-packages.md §17), never a URL import. The
 * backend independently validates the file's real signature; a
 * mismatched/unsupported type is rejected with `ApiError.code ===
 * 'visual_asset_unsupported'`. */
export async function uploadVisualAsset(
  file: File,
  metadata: { displayName?: string; author?: string; license?: string; notice?: string } = {},
): Promise<VisualAsset> {
  const form = new FormData();
  form.append('file', file, file.name);
  if (metadata.displayName !== undefined) form.append('displayName', metadata.displayName);
  if (metadata.author !== undefined) form.append('author', metadata.author);
  if (metadata.license !== undefined) form.append('license', metadata.license);
  if (metadata.notice !== undefined) form.append('notice', metadata.notice);

  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), UPLOAD_TIMEOUT_MS);

  const path = '/api/visual-assets';
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
  const parsed = visualAssetSchema.safeParse(payload);
  if (!parsed.success) {
    throw new ApiError('parse', `Backend response for ${path} did not match the expected shape.`);
  }
  return parsed.data;
}
