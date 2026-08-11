import { z } from 'zod';

/**
 * Zod contracts for the Stage 14B managed visual asset API
 * (`internal/httpapi/visualasset.go`) - a management/editor surface
 * only. See docs/visual-template-packages.md §17/§18 for the full
 * contract this mirrors.
 */

export const visualAssetKindSchema = z.enum(['image', 'video', 'font']);
export type VisualAssetKind = z.infer<typeof visualAssetKindSchema>;

export const visualAssetSourceSchema = z.enum(['upload', 'package']);
export type VisualAssetSource = z.infer<typeof visualAssetSourceSchema>;

/** One managed asset, as returned by every `/api/visual-assets/...`
 * endpoint. `url` is the same safe, app-owned public content URL a
 * Browser Source would use - asset bytes are not sensitive, only the
 * local `id`/reference-count context is management-only (docs/visual-
 * template-packages.md §18/§38). */
export const visualAssetSchema = z.object({
  id: z.string(),
  kind: visualAssetKindSchema,
  mediaType: z.string(),
  sizeBytes: z.number(),
  displayName: z.string(),
  author: z.string(),
  license: z.string(),
  notice: z.string(),
  source: visualAssetSourceSchema,
  url: z.string(),
  referenceCount: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type VisualAsset = z.infer<typeof visualAssetSchema>;

export const visualAssetListSchema = z.array(visualAssetSchema);
