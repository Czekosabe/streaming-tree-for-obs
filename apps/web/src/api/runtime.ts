import { z } from 'zod';

import { ApiError, apiGet, apiPost } from '@/lib/api-client';

import {
  runtimeSnapshotSchema,
  RUNTIME_SCHEMA_VERSION,
  type RuntimeSnapshot,
} from './runtime-schemas';

// The commands answer with a small acknowledgement; the authoritative state
// always comes from the next GET /api/runtime.
const runtimeCommandSchema = z.object({ status: z.string() });

/**
 * Transport for the runtime API.
 *
 * The MediaMTX Control API is never contacted from the browser: only these
 * curated backend endpoints exist, and the backend alone talks to MediaMTX.
 */

/** Commands take no body; the backend rejects one outright. */
const NO_BODY = undefined;

export async function fetchRuntime(signal?: AbortSignal): Promise<RuntimeSnapshot> {
  const snapshot = await apiGet('/api/runtime', runtimeSnapshotSchema, { signal });

  if (snapshot.version !== RUNTIME_SCHEMA_VERSION) {
    // A different schema version means the backend and this build disagree
    // about the payload. Rendering it anyway would show wrong or missing state.
    throw new ApiError(
      'parse',
      `The backend returned runtime schema version ${snapshot.version}, but this build understands ${RUNTIME_SCHEMA_VERSION}.`,
    );
  }

  return snapshot;
}

export async function installMediaMtx(): Promise<void> {
  await apiPost('/api/runtime/mediamtx/install', NO_BODY, runtimeCommandSchema);
}

export async function startMediaMtx(): Promise<void> {
  await apiPost('/api/runtime/mediamtx/start', NO_BODY, runtimeCommandSchema);
}

export async function stopMediaMtx(): Promise<void> {
  await apiPost('/api/runtime/mediamtx/stop', NO_BODY, runtimeCommandSchema);
}

export async function restartMediaMtx(): Promise<void> {
  await apiPost('/api/runtime/mediamtx/restart', NO_BODY, runtimeCommandSchema);
}
