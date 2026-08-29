import { z } from 'zod';

/**
 * Contract of `GET /api/system/resources` exposed by the Go backend
 * (`internal/sysresources`).
 *
 * Every metric is independently optional/nullable: a platform or
 * environment that cannot report one metric still reports the others, and
 * a missing field means "not sampled this tick," never a fabricated zero.
 * `unavailable` names which metric keys, if any, could not be sampled, so
 * the UI can render an honest "unavailable" state per metric instead of
 * guessing from an absent field alone.
 */
export const systemResourcesSchema = z.object({
  cpuPercent: z.number().min(0).max(100).nullish(),
  memoryPercent: z.number().min(0).max(100).nullish(),
  memoryUsedBytes: z.number().nonnegative().nullish(),
  memoryTotalBytes: z.number().nonnegative().nullish(),
  diskPercent: z.number().min(0).max(100).nullish(),
  diskUsedBytes: z.number().nonnegative().nullish(),
  diskTotalBytes: z.number().nonnegative().nullish(),
  unavailable: z.array(z.string()),
  sampledAt: z.string().min(1),
});

export type SystemResourcesSnapshot = z.infer<typeof systemResourcesSchema>;
