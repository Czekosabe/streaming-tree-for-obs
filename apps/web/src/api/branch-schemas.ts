import { z } from 'zod';

/**
 * Zod contracts for the FFmpeg dependency and destination-branch runtime
 * APIs.
 *
 * Neither payload has a field for a stream key, a full destination URL, an
 * executable path, a process id or a command line - the backend never sends
 * any of those; see docs/progress.md for why.
 */

export const FFMPEG_SCHEMA_VERSION = 1;
export const BRANCHES_SCHEMA_VERSION = 1;

export const FFMPEG_STATES = ['missing', 'ready', 'incompatible', 'error'] as const;
export type FFmpegState = (typeof FFMPEG_STATES)[number];

export const FFMPEG_SOURCES = ['override', 'bundled', 'path', 'missing'] as const;
export type FFmpegSource = (typeof FFMPEG_SOURCES)[number];

export const BRANCH_STATES = [
  'idle',
  'blocked',
  'waiting_for_ingest',
  'starting',
  'live',
  'restarting',
  'stopping',
  'error',
] as const;
export type BranchState = (typeof BRANCH_STATES)[number];

const runtimeErrorSchema = z.object({
  code: z.string(),
  message: z.string(),
});

const ffmpegCapabilitiesSchema = z.object({
  rtmpInput: z.boolean(),
  rtmpOutput: z.boolean(),
  rtmpsOutput: z.boolean(),
  flvMuxer: z.boolean(),
  progress: z.boolean(),
});

export const ffmpegStatusSchema = z.object({
  version: z.number().int(),
  ffmpeg: z.object({
    state: z.enum(FFMPEG_STATES),
    source: z.enum(FFMPEG_SOURCES),
    detectedVersion: z.string().optional(),
    minimumVersion: z.string(),
    capabilities: ffmpegCapabilitiesSchema,
    lastError: runtimeErrorSchema.nullable(),
  }),
});

export type FFmpegStatus = z.infer<typeof ffmpegStatusSchema>;
export type FFmpegCapabilities = z.infer<typeof ffmpegCapabilitiesSchema>;

const branchProgressSchema = z
  .object({
    frameCount: z.number().int(),
    fps: z.number(),
    outTimeMs: z.number().int(),
    totalSize: z.number().int(),
    speed: z.number(),
  })
  .nullable();

export const branchSnapshotSchema = z.object({
  platformId: z.string().min(1),
  state: z.enum(BRANCH_STATES),
  desiredRunning: z.boolean(),
  blockers: z.array(z.string()),
  startedAt: z.string().optional(),
  liveAt: z.string().optional(),
  stoppedAt: z.string().optional(),
  restartCount: z.number().int().nonnegative(),
  progress: branchProgressSchema,
  lastError: runtimeErrorSchema.nullable(),
});

export const branchesResponseSchema = z.object({
  version: z.number().int(),
  branches: z.array(branchSnapshotSchema),
});

export type BranchProgress = z.infer<typeof branchProgressSchema>;
export type BranchSnapshot = z.infer<typeof branchSnapshotSchema>;

const branchCommandResponseSchema = z.object({
  status: z.string(),
  blockers: z.array(z.string()).optional(),
});

export type BranchCommandResponse = z.infer<typeof branchCommandResponseSchema>;
export const branchCommandSchema = branchCommandResponseSchema;

export const startEnabledResponseSchema = z.object({
  results: z.array(
    z.object({
      platformId: z.string(),
      accepted: z.boolean(),
      blockers: z.array(z.string()).optional(),
      conflict: z.boolean().optional(),
    }),
  ),
});

export type StartEnabledResponse = z.infer<typeof startEnabledResponseSchema>;
