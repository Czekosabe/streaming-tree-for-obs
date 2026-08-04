/**
 * Static application facts shown in the shell.
 *
 * The ingest address is deliberately NOT duplicated here. It is configurable on
 * the backend, so the real values come from `GET /api/runtime` and are derived
 * from the running configuration rather than from a constant that could drift.
 */
export const APP_INFO = {
  name: 'Streaming Tree for OBS',
  version: '0.1.0',
} as const;
