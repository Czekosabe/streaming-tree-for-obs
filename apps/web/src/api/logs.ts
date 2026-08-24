import { apiGet, apiPostBlob } from '@/lib/api-client';

import { logsResponseSchema, type LogsResponse } from './logs-schemas';

/**
 * Transport for the Stage 20E diagnostics API
 * (`internal/httpapi/logs.go`). `GET /api/logs` is bounded/filtered
 * server-side; this file never requests more than the backend's own
 * `MaxLimit`. `POST /api/diagnostics/support-bundle` is an explicit,
 * operator-triggered action only - no call site here ever invokes it
 * automatically.
 */

export type FetchLogsOptions = {
  severity?: string;
  subsystem?: string;
  search?: string;
  limit?: number;
  before?: number | undefined;
  signal?: AbortSignal;
};

export async function fetchLogs(options: FetchLogsOptions = {}): Promise<LogsResponse> {
  const params = new URLSearchParams();
  if (options.severity !== undefined && options.severity !== '') {
    params.set('severity', options.severity);
  }
  if (options.subsystem !== undefined && options.subsystem !== '') {
    params.set('subsystem', options.subsystem);
  }
  if (options.search !== undefined && options.search !== '') {
    params.set('search', options.search);
  }
  if (options.limit !== undefined) params.set('limit', String(options.limit));
  if (options.before !== undefined) params.set('before', String(options.before));

  const query = params.toString();
  const path = query ? `/api/logs?${query}` : '/api/logs';
  return apiGet(path, logsResponseSchema, { signal: options.signal });
}

const SUPPORT_BUNDLE_TIMEOUT_MS = 30_000;

/** Generates and downloads the support bundle - always a fresh
 * server-side generation, never cached. */
export async function fetchSupportBundle(): Promise<{ blob: Blob; filename: string }> {
  return apiPostBlob('/api/diagnostics/support-bundle', { timeoutMs: SUPPORT_BUNDLE_TIMEOUT_MS });
}
