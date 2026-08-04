import { z } from 'zod';

/**
 * Zod contract for the destination output-settings API.
 *
 * Deliberately has no field for a stream key: the server address alone is
 * never enough to start a broadcast, and this contract must make that
 * structurally obvious rather than merely documented.
 */
export const outputSettingsSchema = z.object({
  serverUrl: z.string(),
  autoRestart: z.boolean(),
  updatedAt: z.string(),
});

export type OutputSettings = z.infer<typeof outputSettingsSchema>;

/** Payload accepted by `PUT /api/platforms/{id}/output`. */
export type UpdateOutputSettingsInput = {
  serverUrl: string;
  autoRestart: boolean;
};
