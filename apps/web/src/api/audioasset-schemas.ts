import { z } from 'zod';

/**
 * Zod contracts for the Stage 17B managed audio asset API
 * (`internal/httpapi/audioasset.go`) - a management/editor surface
 * only. See docs/alert-audio.md §5/§7 for the full contract this
 * mirrors. Deliberately no content URL field here (unlike
 * `visualAssetSchema`) - an audio asset's bytes are never served
 * directly by local id, only ever indirectly through whichever alert
 * instance's current `internal/audio` item happens to reference it.
 */

export const audioAssetKindSchema = z.enum(['sound']);
export type AudioAssetKind = z.infer<typeof audioAssetKindSchema>;

export const audioAssetSourceSchema = z.enum(['upload', 'package']);
export type AudioAssetSource = z.infer<typeof audioAssetSourceSchema>;

export const audioAssetSchema = z.object({
  id: z.string(),
  kind: audioAssetKindSchema,
  mediaType: z.string(),
  sizeBytes: z.number(),
  durationMs: z.number(),
  displayName: z.string(),
  source: audioAssetSourceSchema,
  referenceCount: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type AudioAsset = z.infer<typeof audioAssetSchema>;

export const audioAssetListSchema = z.array(audioAssetSchema);
