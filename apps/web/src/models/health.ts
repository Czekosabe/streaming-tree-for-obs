import { z } from 'zod';

/**
 * Contract of `GET /api/health` exposed by the Go backend.
 *
 * The response is parsed with Zod rather than trusted blindly: the backend and
 * the frontend are versioned independently and a shape mismatch must surface as
 * a readable error instead of a runtime crash somewhere in the render tree.
 */
export const healthResponseSchema = z.object({
  status: z.string().min(1),
  service: z.string().min(1),
  version: z.string().min(1),
  /** Seconds since process start. Optional so older builds keep parsing. */
  uptimeSeconds: z.number().nonnegative().optional(),
  /** RFC 3339 timestamp produced by the server. */
  time: z.string().optional(),
});

export type HealthResponse = z.infer<typeof healthResponseSchema>;
