/**
 * Static application facts shown in the shell.
 *
 * The RTMP address below is the address OBS will be pointed at once MediaMTX is
 * integrated. It is a planned default, not a live endpoint - nothing is
 * listening on it in the current stage.
 */
export const APP_INFO = {
  name: 'Streaming Tree for OBS',
  version: '0.1.0',
  /** Planned local ingest endpoint (MediaMTX, not yet running). */
  localIngestUrl: 'rtmp://127.0.0.1:1935/live',
  localIngestKey: 'obs',
} as const;
