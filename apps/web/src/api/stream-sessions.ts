import { apiDeleteWithBody, apiGet, apiPut } from '@/lib/api-client';

import {
  streamSessionListSchema,
  streamSessionSchema,
  streamSessionSettingsSchema,
  type StreamSession,
  type StreamSessionSettings,
} from './stream-sessions-schemas';

/** Transport for the Stage 24 stream session / operational history API
 * (`internal/httpapi/streamsession.go`). */

export async function fetchStreamSessions(limit?: number): Promise<StreamSession[]> {
  const path = limit === undefined ? '/api/stream-sessions' : `/api/stream-sessions?limit=${limit}`;
  const { sessions } = await apiGet(path, streamSessionListSchema);
  return sessions;
}

export async function fetchStreamSession(id: string): Promise<StreamSession> {
  return apiGet(`/api/stream-sessions/${id}`, streamSessionSchema);
}

/** Requires {"confirm":true} - mirrors the backend's own convention for
 * a destructive action with no other parameters. */
export async function clearStreamSessionHistory(): Promise<void> {
  await apiDeleteWithBody('/api/stream-sessions', { confirm: true });
}

export async function fetchStreamSessionSettings(): Promise<StreamSessionSettings> {
  return apiGet('/api/stream-sessions/settings', streamSessionSettingsSchema);
}

export async function setStreamSessionRetentionDays(retentionDays: number): Promise<StreamSessionSettings> {
  return apiPut('/api/stream-sessions/settings', { retentionDays }, streamSessionSettingsSchema);
}
