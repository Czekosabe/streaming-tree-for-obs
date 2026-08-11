import { z } from 'zod';

import { visualDesignDocumentSchema } from './visualdesign-schemas';
import { visualAssetKindSchema } from './visualasset-schemas';
import { visualTemplateTargetSchema } from './visualtemplate-schemas';

/**
 * Zod contracts for the Stage 14B portable package import/preview/
 * export API (`internal/httpapi/visualpackage.go`). See
 * docs/visual-template-packages.md §19/§43 for the full contract.
 */

export const visualTemplatePackagePreviewAssetSchema = z.object({
  packageAssetId: z.string(),
  kind: visualAssetKindSchema,
  mediaType: z.string(),
  sizeBytes: z.number(),
  displayName: z.string(),
  author: z.string(),
  license: z.string(),
  notice: z.string(),
  url: z.string(),
});
export type VisualTemplatePackagePreviewAsset = z.infer<typeof visualTemplatePackagePreviewAssetSchema>;

/** The document here still carries package-local (`pkgasset_...`)
 * asset references, resolved against `assets[].packageAssetId`/`url` -
 * never a real local managed-asset id (docs/visual-template-
 * packages.md §43). Nothing here has been persisted. */
export const visualTemplatePackagePreviewSchema = z.object({
  token: z.string(),
  target: visualTemplateTargetSchema,
  name: z.string(),
  description: z.string(),
  author: z.string(),
  license: z.string(),
  document: visualDesignDocumentSchema,
  assets: z.array(visualTemplatePackagePreviewAssetSchema),
  expiresAt: z.string(),
});
export type VisualTemplatePackagePreview = z.infer<typeof visualTemplatePackagePreviewSchema>;
