import { apiDelete, apiPost, apiPostBlob, ApiError, kindForStatus, readErrorEnvelope, resolveUrl } from '@/lib/api-client';

import {
  restoreBackupPreviewSchema,
  restoreBackupResultSchema,
  type RestoreBackupPreview,
  type RestoreBackupResult,
} from './backup-schemas';

/**
 * Transport for the Stage 23 configuration backup/restore API
 * (`internal/httpapi/backup.go`). Export reuses `apiPostBlob` exactly
 * like the Stage 20E support-bundle export (a POST with no body whose
 * response is a binary file); restore-preview's upload is raw
 * archive bytes rather than JSON, so it talks to `fetch` directly and
 * reuses `lib/api-client.ts`'s error-envelope/status-classification
 * logic instead of duplicating it - the same shape `api/visualpackage.ts`
 * already established for the closest existing precedent (a portable
 * package's own preview-then-commit upload flow).
 */

const RESTORE_PREVIEW_TIMEOUT_MS = 30_000;

/** Downloads a fresh backup package. The browser download itself is
 * triggered by the caller via `models/visualtemplate.ts`'s
 * `downloadBlob` helper. */
export async function exportBackup(): Promise<{ blob: Blob; filename: string }> {
  return apiPostBlob('/api/backup/export');
}

/** Fully validates file as a backup package and stages its raw bytes
 * under a fresh preview token - persists nothing. */
export async function previewRestoreBackup(file: File): Promise<RestoreBackupPreview> {
  const bytes = await file.arrayBuffer();
  const path = '/api/backup/restore/preview';
  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), RESTORE_PREVIEW_TIMEOUT_MS);
  let response: Response;
  try {
    response = await fetch(resolveUrl(path), {
      method: 'POST',
      body: bytes,
      signal: controller.signal,
      headers: { Accept: 'application/json', 'Content-Type': 'application/octet-stream' },
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError('timeout', `Request to ${path} timed out.`);
    }
    throw new ApiError('network', `Cannot reach the backend at ${resolveUrl(path)}.`);
  } finally {
    window.clearTimeout(timeoutId);
  }
  if (!response.ok) {
    const envelope = await readErrorEnvelope(response);
    throw new ApiError(kindForStatus(response.status, envelope.code), `Request to ${path} failed with ${response.status}.`, {
      status: response.status,
      ...envelope,
    });
  }
  const payload: unknown = await response.json();
  const parsed = restoreBackupPreviewSchema.safeParse(payload);
  if (!parsed.success) {
    throw new ApiError('parse', 'Backend response for the restore preview did not match the expected shape.');
  }
  return parsed.data;
}

/** Best-effort - discards a staged restore-preview session's bytes
 * early. Safe to call even after the session has already expired. */
export async function cancelRestoreBackupPreview(token: string): Promise<void> {
  await apiDelete(`/api/backup/restore/preview/${token}`).catch(() => undefined);
}

/** Re-validates token's staged bytes from scratch - never trusts a
 * prior preview call. Destructive: replaces the entire current
 * configuration (docs/backup-restore.md §7 "Mode: REPLACE"). */
export async function commitRestoreBackup(token: string): Promise<RestoreBackupResult> {
  return apiPost(`/api/backup/restore/commit/${token}`, undefined, restoreBackupResultSchema);
}
