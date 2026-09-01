import { z } from 'zod';

import { platformMetadataSchema } from './platform-schemas';

/**
 * Zod contract for the Stage 22C apply endpoints
 * (docs/metadata-presets.md §6):
 * `GET /api/metadata-presets/{id}/apply-preview?platformIds=...` and
 * `POST /api/metadata-presets/{id}/apply`.
 */

export const fieldStatusSchema = z.enum(['will_change', 'unchanged', 'not_supported']);
export type FieldStatus = z.infer<typeof fieldStatusSchema>;

export const applyFieldSchema = z.object({
  field: z.string(),
  status: fieldStatusSchema,
});
export type ApplyField = z.infer<typeof applyFieldSchema>;

export const applyDestinationSchema = z.object({
  platformId: z.string(),
  providerId: z.string(),
  valid: z.boolean(),
  fields: z.array(applyFieldSchema),
  errors: z.record(z.string(), z.string()).optional(),
});
export type ApplyDestinationPreview = z.infer<typeof applyDestinationSchema>;

export const applyPreviewResponseSchema = z.array(applyDestinationSchema);

export const applyPresetResponseSchema = z.object({
  platforms: z.record(z.string(), platformMetadataSchema),
});
export type ApplyPresetResult = z.infer<typeof applyPresetResponseSchema>;
