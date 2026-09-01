import { z } from 'zod';

/**
 * Zod contracts for the Stage 24 stream session / operational history
 * API (`internal/httpapi/streamsession.go`). See docs/stream-session-
 * history.md for the full contract - notably §0: this never carries
 * chat messages, chatter names, donation content, or any other viewer
 * content, only the application's own operational timeline.
 */

export const streamSessionDestinationSchema = z.object({
  id: z.string(),
  platformId: z.string().nullable(),
  providerId: z.string(),
  displayName: z.string(),
  startedAt: z.string(),
  endedAt: z.string().nullable(),
  open: z.boolean(),
  outcome: z.string(),
});
export type StreamSessionDestination = z.infer<typeof streamSessionDestinationSchema>;

export const streamSessionSchema = z.object({
  id: z.string(),
  startedAt: z.string(),
  endedAt: z.string().nullable(),
  open: z.boolean(),
  endReason: z.string(),
  destinations: z.array(streamSessionDestinationSchema),
});
export type StreamSession = z.infer<typeof streamSessionSchema>;

export const streamSessionListSchema = z.object({
  sessions: z.array(streamSessionSchema),
});

export const streamSessionSettingsSchema = z.object({
  retentionDays: z.number(),
});
export type StreamSessionSettings = z.infer<typeof streamSessionSettingsSchema>;
