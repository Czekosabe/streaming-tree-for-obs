import { z } from 'zod';

/**
 * Zod contract for `GET /api/runtime`.
 *
 * The payload is versioned, so a backend that changes the shape in an
 * incompatible way is rejected outright rather than rendering half a panel.
 *
 * State values are kept as plain enums because the interface must react to each
 * one; an unknown state is a contract violation, not something to degrade past.
 * Optional descriptive fields (source type, tracks) are tolerant, because
 * MediaMTX may add or rename them between releases.
 */

/** Schema version this build understands. */
export const RUNTIME_SCHEMA_VERSION = 1;

export const MEDIAMTX_STATES = [
  'missing',
  'installing',
  'incompatible',
  'stopped',
  'starting',
  'ready',
  'stopping',
  'error',
] as const;

export const INGEST_STATES = ['unavailable', 'waiting', 'receiving', 'error'] as const;

export const BINARY_SOURCES = ['managed', 'override', 'missing'] as const;

export type MediaMtxState = (typeof MEDIAMTX_STATES)[number];
export type IngestState = (typeof INGEST_STATES)[number];
export type BinarySource = (typeof BINARY_SOURCES)[number];

const runtimeErrorSchema = z.object({
  /** Stable identifier the frontend localizes. */
  code: z.string(),
  /** English fallback for a code this build has no mapping for. */
  message: z.string(),
});

export const mediaMtxSnapshotSchema = z.object({
  supportedVersion: z.string().min(1),
  installedVersion: z.string().optional(),
  source: z.enum(BINARY_SOURCES),
  state: z.enum(MEDIAMTX_STATES),
  autoStart: z.boolean(),
  autoRestart: z.boolean(),
  startedAt: z.string().optional(),
  restartCount: z.number().int().nonnegative(),
  // Explicitly nullable rather than optional, so "no error" is distinguishable
  // from "the backend forgot the field".
  lastError: runtimeErrorSchema.nullable(),
});

export const ingestSnapshotSchema = z.object({
  state: z.enum(INGEST_STATES),
  path: z.string(),
  /**
   * MediaMTX source kind, e.g. "rtmpConn". It identifies the protocol, not the
   * application: RTMP does not prove the publisher is OBS.
   */
  sourceType: z.string().optional(),
  connectedAt: z.string().optional(),
  trackCount: z.number().int().nonnegative().nullable(),
  tracks: z.array(z.string()),
});

export const connectionSnapshotSchema = z.object({
  serverUrl: z.string().min(1),
  /** The MediaMTX path. A route identifier, not a secret. */
  streamKey: z.string().min(1),
  publishUrl: z.string().min(1),
});

export const runtimeSnapshotSchema = z.object({
  version: z.number().int(),
  mediaMtx: mediaMtxSnapshotSchema,
  ingest: ingestSnapshotSchema,
  connection: connectionSnapshotSchema,
});

export type RuntimeError = z.infer<typeof runtimeErrorSchema>;
export type MediaMtxSnapshot = z.infer<typeof mediaMtxSnapshotSchema>;
export type IngestSnapshot = z.infer<typeof ingestSnapshotSchema>;
export type ConnectionSnapshot = z.infer<typeof connectionSnapshotSchema>;
export type RuntimeSnapshot = z.infer<typeof runtimeSnapshotSchema>;
