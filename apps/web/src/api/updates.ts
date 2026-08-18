import { apiGet, apiPostNoContent, apiPut } from '@/lib/api-client';
import { updateStatusSchema, type UpdateStatus } from '@/models/updates';

/** Fetches the current updater status from `GET /api/updates/status`. */
export async function fetchUpdateStatus(signal?: AbortSignal): Promise<UpdateStatus> {
  return apiGet('/api/updates/status', updateStatusSchema, { signal });
}

/** Persists the automatic-check preference via `PUT /api/updates/preferences`. */
export async function setAutoCheckPreference(autoCheck: boolean): Promise<UpdateStatus> {
  return apiPut('/api/updates/preferences', { autoCheck }, updateStatusSchema);
}

/** Triggers a manual metadata check - `POST /api/updates/check`, no body. */
export async function checkForUpdate(): Promise<void> {
  await apiPostNoContent('/api/updates/check', undefined);
}

/** Begins downloading the currently-available update - no body. */
export async function downloadUpdate(): Promise<void> {
  await apiPostNoContent('/api/updates/download', undefined);
}

/**
 * Begins the install/restart handoff. The backend requires this exact body
 * shape (mirroring `POST /api/system/shutdown`) - there is no generic
 * action/command parameter.
 */
export async function installUpdate(): Promise<void> {
  await apiPostNoContent('/api/updates/install', { confirm: true });
}
