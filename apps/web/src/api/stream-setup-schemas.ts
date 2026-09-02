import { z } from 'zod';

import { applyDestinationSchema } from './metadata-preset-apply-schemas';

/**
 * Zod contract for `/api/stream-setups` (docs/stream-setup-profiles.md §10).
 */

export const streamSetupDestinationSchema = z.object({
  platformId: z.string().nullable(),
  providerId: z.string(),
  displayName: z.string(),
});
export type StreamSetupDestination = z.infer<typeof streamSetupDestinationSchema>;

export const streamSetupProfileSchema = z.object({
  id: z.string().min(1),
  name: z.string(),
  note: z.string(),
  destinations: z.array(streamSetupDestinationSchema),
  metadataPresetId: z.string().nullable(),
  metadataPresetName: z.string(),
  metadataPresetMissing: z.boolean(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type StreamSetupProfile = z.infer<typeof streamSetupProfileSchema>;

export const streamSetupProfilesResponseSchema = z.array(streamSetupProfileSchema);

/** Payload accepted by `POST`/`PUT /api/stream-setups[/{id}]`. */
export type SaveStreamSetupInput = {
  name: string;
  note: string;
  destinationIds: string[];
  metadataPresetId: string | null;
};

export const streamSetupDestinationChangeSchema = z.enum([
  'will_enable',
  'will_disable',
  'unchanged',
  'missing',
]);
export type StreamSetupDestinationChange = z.infer<typeof streamSetupDestinationChangeSchema>;

export const streamSetupDestinationPreviewSchema = z.object({
  platformId: z.string(),
  providerId: z.string(),
  displayName: z.string(),
  currentlyEnabled: z.boolean(),
  change: streamSetupDestinationChangeSchema,
  active: z.boolean(),
});
export type StreamSetupDestinationPreview = z.infer<typeof streamSetupDestinationPreviewSchema>;

export const streamSetupPreviewSchema = z.object({
  profile: streamSetupProfileSchema,
  destinations: z.array(streamSetupDestinationPreviewSchema),
  metadataPresetReferenced: z.boolean(),
  metadataPresetMissing: z.boolean(),
  metadataPresetName: z.string(),
  metadataDestinationPreviews: z.array(applyDestinationSchema),
  blocked: z.boolean(),
  blockedDestinationIds: z.array(z.string()),
});
export type StreamSetupPreview = z.infer<typeof streamSetupPreviewSchema>;

export const streamSetupApplyResultSchema = z.object({
  destinationsChanged: z.number(),
  metadataApplied: z.boolean(),
  metadataSkippedReason: z.string().optional(),
});
export type StreamSetupApplyResult = z.infer<typeof streamSetupApplyResultSchema>;
