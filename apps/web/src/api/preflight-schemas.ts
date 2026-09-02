import { z } from 'zod';

/**
 * Zod contract for `GET /api/preflight` (docs/stream-preflight.md §6).
 */

export const preflightSeveritySchema = z.enum(['blocker', 'warning']);
export type PreflightSeverity = z.infer<typeof preflightSeveritySchema>;

export const preflightActionSchema = z.object({
  code: z.string(),
  platformId: z.string().optional(),
});
export type PreflightAction = z.infer<typeof preflightActionSchema>;

export const preflightFindingSchema = z.object({
  code: z.string(),
  severity: preflightSeveritySchema,
  platformId: z.string().optional(),
  action: preflightActionSchema.optional(),
});
export type PreflightFinding = z.infer<typeof preflightFindingSchema>;

export const preflightDestinationSchema = z.object({
  platformId: z.string(),
  providerId: z.string(),
  displayName: z.string(),
  findings: z.array(preflightFindingSchema),
});
export type PreflightDestination = z.infer<typeof preflightDestinationSchema>;

export const preflightStatusSchema = z.enum(['ready', 'ready_with_warnings', 'not_ready']);
export type PreflightStatus = z.infer<typeof preflightStatusSchema>;

export const preflightReportSchema = z.object({
  status: preflightStatusSchema,
  findings: z.array(preflightFindingSchema),
  destinations: z.array(preflightDestinationSchema),
  selectedProfileId: z.string().nullable().optional(),
  streamingActive: z.boolean(),
});
export type PreflightReport = z.infer<typeof preflightReportSchema>;
