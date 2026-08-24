import { z } from 'zod';

/**
 * Zod shapes for the Stage 20E diagnostics API
 * (`internal/httpapi/logs.go`), mirroring `internal/diagnostics.Entry`'s
 * own JSON tags exactly.
 */

export const LOG_SEVERITIES = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
export type LogSeverity = (typeof LOG_SEVERITIES)[number];

export const logEntrySchema = z.object({
  time: z.string(),
  severity: z.string(),
  subsystem: z.string(),
  message: z.string(),
  // Present only when the backend record carried attributes; values are
  // opaque to the frontend, already redacted server-side.
  attrs: z.record(z.string(), z.unknown()).optional(),
  seq: z.number(),
});
export type LogEntry = z.infer<typeof logEntrySchema>;

export const logsResponseSchema = z.object({
  entries: z.array(logEntrySchema),
  nextCursor: z.number().optional(),
});
export type LogsResponse = z.infer<typeof logsResponseSchema>;
