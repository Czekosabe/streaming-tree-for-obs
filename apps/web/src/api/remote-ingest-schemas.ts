import { z } from 'zod';

/**
 * Zod contracts for the Stage 20D2C remote-ingest credential-management
 * API (`internal/httpapi/remoteingest.go`). Registered on the backend
 * only when `--remote-ingest` is active - every request against these
 * routes on any other deployment returns 404, which the frontend treats
 * as "this feature does not exist here," not an error (see
 * use-remote-ingest.ts).
 */

export const CURRENT_REMOTE_INGEST_SCHEMA_VERSION = 1;

export const remoteIngestStatusSchema = z.object({
  version: z.number(),
  configured: z.boolean(),
  receiving: z.boolean(),
  rtmpsAddress: z.string(),
  ingestPath: z.string(),
});
export type RemoteIngestStatus = z.infer<typeof remoteIngestStatusSchema>;

/**
 * The one-time plaintext publisher secret. Never persisted by any
 * caller of this schema beyond the active response lifecycle (docs/
 * remote-ingest.md §6) - the component that receives this must clear
 * it from React state on unmount/dismiss and never write it to
 * localStorage/sessionStorage/IndexedDB/the URL/a long-lived query
 * cache entry.
 */
export const remoteIngestSecretSchema = z.object({
  version: z.number(),
  secret: z.string(),
});
export type RemoteIngestSecret = z.infer<typeof remoteIngestSecretSchema>;
