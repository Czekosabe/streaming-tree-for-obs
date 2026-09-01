import { z } from 'zod';

/**
 * Zod contract for `/api/metadata-presets` (docs/metadata-presets.md).
 *
 * Mirrors platform-schemas.ts's own SaveMetadataInput shape for the
 * shared ("common") fields; `providers` is a map keyed by provider id,
 * holding only a category/categoryId pair that is never applied
 * outside the exact provider it is keyed under.
 */

export const providerMetadataSchema = z.object({
  category: z.string(),
  categoryId: z.string(),
});
export type ProviderMetadata = z.infer<typeof providerMetadataSchema>;

export const metadataPresetSchema = z.object({
  id: z.string().min(1),
  name: z.string(),
  note: z.string(),
  title: z.string(),
  description: z.string(),
  tags: z.array(z.string()),
  language: z.string(),
  visibility: z.string(),
  matureContent: z.boolean(),
  dvr: z.boolean(),
  latencyMode: z.string(),
  providers: z.record(z.string(), providerMetadataSchema),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type MetadataPreset = z.infer<typeof metadataPresetSchema>;

export const metadataPresetsResponseSchema = z.array(metadataPresetSchema);

/** Payload accepted by `POST`/`PUT /api/metadata-presets[/{id}]`. */
export type SavePresetInput = {
  name: string;
  note: string;
  title: string;
  description: string;
  tags: string[];
  language: string;
  visibility: string;
  matureContent: boolean;
  dvr: boolean;
  latencyMode: string;
  providers: Record<string, ProviderMetadata>;
};
