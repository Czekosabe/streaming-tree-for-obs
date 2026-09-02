import { z } from 'zod';

/**
 * Zod contract for `GET /api/stream-insights` (docs/stream-
 * insights.md §4).
 */

export const streamInsightsSessionSummarySchema = z.object({
  sessionId: z.string(),
  startedAt: z.string(),
  durationSeconds: z.number(),
});
export type StreamInsightsSessionSummary = z.infer<typeof streamInsightsSessionSummarySchema>;

export const streamInsightsDestinationSchema = z.object({
  platformId: z.string().nullable(),
  providerId: z.string(),
  displayName: z.string(),
  sessionCount: z.number(),
  durationSeconds: z.number(),
  outcomeCounts: z.record(z.string(), z.number()),
});
export type StreamInsightsDestination = z.infer<typeof streamInsightsDestinationSchema>;

export const streamInsightsSchema = z.object({
  totalSessions: z.number(),
  totalDurationSeconds: z.number(),
  averageDurationSeconds: z.number(),
  longestSession: streamInsightsSessionSummarySchema.nullable(),
  sessionsByEndReason: z.record(z.string(), z.number()),
  destinations: z.array(streamInsightsDestinationSchema),
});
export type StreamInsights = z.infer<typeof streamInsightsSchema>;
